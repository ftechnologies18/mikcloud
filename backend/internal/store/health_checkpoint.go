// health_checkpoint.go — N°181 : carte Santé PERSISTANTE (Paramètres
// plateforme → Santé de la persistance).
//
// PROBLÈME : les compteurs de la carte Santé (syncstats.go) ne vivaient qu'en
// mémoire — chaque redéploiement Render (ou redémarrage) les remettait à
// zéro : « Aucune [synchro] depuis le démarrage » affiché juste après un
// déploiement alors que la persistance tournait depuis des jours, chaîne
// d'échecs consécutifs et dernière erreur oubliées, aucune trace du nombre
// de redémarrages du service. Avec l'architecture améliorée (Supabase +
// boot résilient N°164 + syncreur asynchrone N°130), la carte peut désormais
// s'appuyer sur la base elle-même.
//
// PRODUIT : UNE ligne (id='global') dans la table health_checkpoint :
//   - les compteurs différentiels (tentatives/succès/échecs, chaîne
//     consécutive, volumétrie du dernier delta, dernière erreur) repris au
//     boot par adoptHealthCheckpoint puis rafraîchis à CHAQUE résultat de
//     synchro — les compteurs deviennent CUMULATIFS, la chaîne d'échecs et
//     l'âge de la dernière synchro restent lisibles à travers un restart
//     (l'alerte « échec > 5 min » du runbook §8 garde son sens) ;
//   - l'historique de démarrages (boot_count, last_boot_at) : la carte
//     affiche le nombre cumulé de démarrages, redéploiements compris.
//
// DISCIPLINE (miroir de syncstats.go) : le point de contrôle est un upsert
// SÉPARÉ, exécuté APRÈS le record* de la synchro, hors transaction
// différentielle — la synchro ne peut pas échouer À CAUSE de la carte qui
// l'observe. Si la base est injoignable, l'écriture échoue silencieusement
// (journal borné 1/h) et repartira au prochain succès : l'état mémoire reste
// la vérité temps réel, la ligne base est un point de contrôle durable —
// au pire, un crash pendant une indisponibilité perd les échecs de la
// fenêtre (le prochain succès réécrira l'état cohérent).
package store

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"time"
)

// healthCheckpointID — la table ne porte qu'UNE ligne (constante) : l'état
// santé est GLOBAL au service, pas par compte.
const healthCheckpointID = "global"

// healthCpTimeout — borne de l'upsert du point de contrôle : un pool gelé ne
// doit jamais retenir le syncreur (le Close final non plus). 5 s = large
// marge au-dessus d'un upsert monoligne (~10 ms mesurés en production).
const healthCpTimeout = 5 * time.Second

// healthCheckpoint — miroir Go de la ligne unique health_checkpoint. Les
// horodatages sont en RFC3339 TEXT, comme tout le schéma maison (aller-retour
// strictement identique, cf. ensureSchema).
type healthCheckpoint struct {
	Attempts         int64
	Successes        int64
	Failures         int64
	ConsecutiveFails int64
	LastSuccessAt    string // RFC3339, vide = jamais réussi
	LastSuccessMs    int64
	LastChangedRows  int
	LastRemovedRows  int
	LastError        string
	LastErrorAt      string
	BootCount        int64
	LastBootAt       string
}

// loadHealthCheckpoint — relit la ligne de point de contrôle. Retourne nil si
// absente (premier démarrage après déploiement, base de mise en service) ou
// illisible (l'historique repart à zéro — jamais bloquant).
func (p *PG) loadHealthCheckpoint() *healthCheckpoint {
	row := p.db.QueryRow(`SELECT attempts, successes, failures, consecutive_fails,
                last_success_at, last_success_ms, last_changed_rows, last_removed_rows,
                last_error, last_error_at, boot_count, last_boot_at
                FROM health_checkpoint WHERE id = $1`, healthCheckpointID)
	h := &healthCheckpoint{}
	err := row.Scan(&h.Attempts, &h.Successes, &h.Failures, &h.ConsecutiveFails,
		&h.LastSuccessAt, &h.LastSuccessMs, &h.LastChangedRows, &h.LastRemovedRows,
		&h.LastError, &h.LastErrorAt, &h.BootCount, &h.LastBootAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		log.Printf("store: carte Santé — point de contrôle illisible (%v) — historique repart à zéro", err)
		return nil
	}
	return h
}

// writeHealthCheckpoint — pose l'état frais de la carte (photographie des
// compteurs + contexte de démarrage) dans la ligne unique. BEST-EFFORT :
// un échec est journalisé (borné 1/h) mais ne remonte JAMAIS à la synchro —
// le prochain résultat de synchro retentera.
func (p *PG) writeHealthCheckpoint() {
	snap := p.stats.snapshot()
	now := time.Now().UTC()
	ctx, cancel := context.WithTimeout(context.Background(), healthCpTimeout)
	defer cancel()
	_, err := p.db.ExecContext(ctx, `INSERT INTO health_checkpoint
                (id, attempts, successes, failures, consecutive_fails,
                 last_success_at, last_success_ms, last_changed_rows, last_removed_rows,
                 last_error, last_error_at, boot_count, last_boot_at, updated_at)
                VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
                ON CONFLICT (id) DO UPDATE SET
                 attempts          = EXCLUDED.attempts,
                 successes         = EXCLUDED.successes,
                 failures          = EXCLUDED.failures,
                 consecutive_fails = EXCLUDED.consecutive_fails,
                 last_success_at   = EXCLUDED.last_success_at,
                 last_success_ms   = EXCLUDED.last_success_ms,
                 last_changed_rows = EXCLUDED.last_changed_rows,
                 last_removed_rows = EXCLUDED.last_removed_rows,
                 last_error        = EXCLUDED.last_error,
                 last_error_at     = EXCLUDED.last_error_at,
                 boot_count        = EXCLUDED.boot_count,
                 last_boot_at      = EXCLUDED.last_boot_at,
                 updated_at        = EXCLUDED.updated_at`,
		healthCheckpointID, snap.Attempts, snap.Successes, snap.Failures, snap.ConsecutiveFails,
		snap.LastSuccessAt, snap.LastSuccessMs, snap.LastChangedRows, snap.LastRemovedRows,
		snap.LastError, snap.LastErrorAt, p.healthBootCount, p.healthBootAt, isoUTC(now))
	if err != nil {
		// Pendant une indisponibilité base, CHAQUE synchro échouée tente
		// ce point de contrôle : le journal reste borné (patron keep-alive).
		if time.Since(p.healthCpErrLog) > time.Hour {
			p.healthCpErrLog = time.Now()
			log.Printf("store: carte Santé — point de contrôle non persisté (%v) — repris au prochain succès", err)
		}
		return
	}
	p.healthCheckpointAt.Store(now.Unix())
}

// adoptHealthCheckpoint — N°181 — appelé UNE fois par process au moment où la
// persistance devient disponible (boot normal après un Load réussi, ou
// récupération N°164 au retour de la base) :
//   - reprend l'historique de la carte depuis la base (les compteurs
//     deviennent cumulatifs, la chaîne d'échecs et la dernière erreur
//     survivent au restart) ;
//   - compte CE démarrage (boot_count + 1) et l'inscrit immédiatement —
//     un crash avant la première synchro laisse quand même la trace du boot.
//
// Garde healthAdopted : la boucle de récupération peut rejouer OpenPG+Load,
// chaque tentative ne doit compter qu'UN démarrage par process.
func (s *Store) adoptHealthCheckpoint(pg *PG) {
	if !s.healthAdopted.CompareAndSwap(false, true) {
		return
	}
	h := pg.loadHealthCheckpoint()
	if h != nil {
		pg.stats.restore(h)
		pg.healthRestored = true
		pg.healthBootCount = h.BootCount + 1
	} else {
		pg.healthBootCount = 1
	}
	pg.healthBootAt = isoUTC(time.Now())
	pg.writeHealthCheckpoint()
	if h != nil {
		log.Printf("store: carte Santé — historique repris de la base (%d tentatives, %d succès, %d échecs, chaîne %d) — démarrage n°%d",
			h.Attempts, h.Successes, h.Failures, h.ConsecutiveFails, pg.healthBootCount)
	} else {
		log.Printf("store: carte Santé — premier point de contrôle (démarrage n°1)")
	}
}
