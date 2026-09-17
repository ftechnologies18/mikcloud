// Persistance PostgreSQL (Neon) — schéma relationnel + synchronisation différentielle.
//
// Architecture : la mémoire reste le moteur de calcul (les handlers continuent
// de travailler sur *model.DB sous verrou), PostgreSQL devient la source de
// vérité durable entre redémarrages. À chaque Save(), seules les lignes
// ajoutées, modifiées ou supprimées sont écrites (une transaction par sauvegarde),
// ce qui reste léger même avec le polling 5 s du tableau de bord.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// PG — backend PostgreSQL : pool de connexions + empreintes (hash FNV-1a) de la
// dernière synchronisation réussie, par table, pour calculer les différences.
type PG struct {
	db     *sql.DB
	hashes map[string]map[string]uint64
	// N°130 — syncMu exclut le cache d'empreintes : le syncreur de fond
	// (goroutine de sauvegarde asynchrone) lit/écrit p.hashes dans Sync,
	// tandis qu'un Reload (admin) ou un boot le reconstruit via
	// rebuildHashes. Aucune contention de parcours — les deux opérations
	// sont rares et jamais sur le chemin des requêtes.
	syncMu sync.Mutex
	// N°71 — instrumentation santé de la synchro (syncstats.go) et mode du
	// keep-alive mémorisé pour le diagnostic (vide = keep-alive désactivé).
	stats  syncStats
	kaMode string

	// lastWrite — horodatage (unix, atomic) du dernier contact CONFIRMÉ avec
	// Neon (ping d'ouverture, Load, Sync, ping du keep-alive). Base de la
	// décision du keep-alive (Phase C) : si une écriture réelle est passée
	// depuis < 4 min, Neon est déjà éveillé et un ping serait du gaspillage.
	lastWrite atomic.Int64
	// kaStop — canal d'arrêt de la goroutine keep-alive (fermé par Close).
	kaStop chan struct{}
}

// OpenPG ouvre le pool, attend que la base réponde (cold start Neon) et crée le
// schéma si nécessaire (DDL idempotent).

// OpenPG ouvre le pool, attend que la base réponde (cold start Neon) et crée le
// schéma si nécessaire (DDL idempotent).
func OpenPG(databaseURL string) (*PG, error) {
	// N°75 — TLS STRICT : sslmode=verify-full par défaut (validation de la
	// chaîne de certification + correspondance du nom d'hôte). L'ancien
	// « require » chiffrait le transport mais acceptait N'IMPORTE QUEL
	// certificat : un attaquant positionné sur le segment réseau
	// Render→Neon pouvait se faire passer pour la base (usurpation de
	// l'endpoint). Échappatoires volontaires : un sslmode= explicite dans
	// l'URL, ou la variable MIKCLOUD_PG_SSLMODE (urgence opérationnelle
	// sans re-déploiement — ex. souci de chaîne de certification de
	// l'hébergeur). L'image de production embarque ca-certificates
	// (Dockerfile) — prérequis des racines publiques.
	if !strings.Contains(databaseURL, "sslmode=") {
		mode := strings.TrimSpace(os.Getenv("MIKCLOUD_PG_SSLMODE"))
		if mode == "" {
			mode = "verify-full"
		}
		sep := "?"
		if strings.Contains(databaseURL, "?") {
			sep = "&"
		}
		databaseURL += sep + "sslmode=" + mode
	}

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("ouverture PostgreSQL : %w", err)
	}
	// Réglages adaptés à Neon (compute suspendu après inactivité) :
	// peu de connexions, recyclées avant que Neon ne les coupe.
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(2)
	db.SetConnMaxIdleTime(3 * time.Minute)
	db.SetConnMaxLifetime(30 * time.Minute)

	// Ping avec retry : le premier réveil d'une base Neon peut prendre quelques secondes.
	var lastErr error
	for attempt := 1; attempt <= 10; attempt++ {
		if lastErr = db.Ping(); lastErr == nil {
			break
		}
		log.Printf("pg: tentative %d/10 — base pas encore prête (%v), nouvelle tentative dans 1 s…", attempt, lastErr)
		time.Sleep(time.Second)
	}
	if lastErr != nil {
		db.Close()
		return nil, fmt.Errorf("PostgreSQL injoignable : %w", lastErr)
	}

	p := &PG{db: db, hashes: map[string]map[string]uint64{}}
	p.touchDB() // le ping d'ouverture est un contact confirmé
	if err := p.ensureSchema(); err != nil {
		db.Close()
		return nil, err
	}
	return p, nil
}

// Close ferme le pool (et arrête le keep-alive éventuel).

// Close ferme le pool (et arrête le keep-alive éventuel).
func (p *PG) Close() error {
	if p.kaStop != nil {
		close(p.kaStop)
		p.kaStop = nil
	}
	return p.db.Close()
}

// ---------------------------------------------------------------------------
// Keep-alive Neon (Phase C « Speed App UX »)
// ---------------------------------------------------------------------------

// touchDB — note le dernier contact confirmé avec Neon. Appelé après tout
// échange réussi : ping d'ouverture, Load (boot / admin/reload), Sync (Save).
// Les lectures du dashboard sont servies depuis la MÉMOIRE et ne touchent pas
// Neon : sans cette trace, le keep-alive ne saurait pas que la base reçoit
// déjà du trafic réel.

// ---------------------------------------------------------------------------
// Keep-alive Neon (Phase C « Speed App UX »)
// ---------------------------------------------------------------------------

// touchDB — note le dernier contact confirmé avec Neon. Appelé après tout
// échange réussi : ping d'ouverture, Load (boot / admin/reload), Sync (Save).
// Les lectures du dashboard sont servies depuis la MÉMOIRE et ne touchent pas
// Neon : sans cette trace, le keep-alive ne saurait pas que la base reçoit
// déjà du trafic réel.
func (p *PG) touchDB() {
	p.lastWrite.Store(time.Now().Unix())
}

// keepAliveQuietWindow — au-delà de ce silence, Neon risque la suspension
// (autosuspend par défaut du plan gratuit : 5 min) ; le keep-alive pinge.
// Placé à 4 min pour laisser une marge de sécurité sous les 5 min.

// keepAliveQuietWindow — au-delà de ce silence, Neon risque la suspension
// (autosuspend par défaut du plan gratuit : 5 min) ; le keep-alive pinge.
// Placé à 4 min pour laisser une marge de sécurité sous les 5 min.
const keepAliveQuietWindow = 4 * time.Minute

// StartKeepAlive — Phase C « Speed App UX » : supprime le cold start Neon
// (~0,5-1 s payé par la première mutation) pendant les périodes calmes.
//
// Pourquoi pas un ping bête 24/7 :
//   - quand au moins un agent est en ligne, son check-in (45 s) déclenche
//     Save() → Sync() → BEGIN/COMMIT : Neon reçoit déjà du trafic continu ;
//   - le garde-fou lastWrite saute donc tout ping superflu : la goroutine ne
//     parle à Neon QUE si aucune activité réelle n'est passée depuis 4 min —
//     le coût réel se limite aux périodes où la base serait de toute façon
//     endormie alors que des usagers peuvent arriver.
//
// Modes (variable d'environnement NEON_KEEPALIVE, défaut « business ») :
//   - business : fenêtre d'activité usagers (05:00–24:00 UTC ≈ Abidjan,
//     UTC+0). La nuit, Neon retrouve son autosuspend — le plafond gratuit
//     (191,9 CU-h/mois) reste largement couvert (≈ 142 CU-h au pire).
//   - on (« 24/7 » accepté) : maintien permanent (≈ 180 CU-h/mois).
//   - off : désactivé (aucune goroutine).

// StartKeepAlive — Phase C « Speed App UX » : supprime le cold start Neon
// (~0,5-1 s payé par la première mutation) pendant les périodes calmes.
//
// Pourquoi pas un ping bête 24/7 :
//   - quand au moins un agent est en ligne, son check-in (45 s) déclenche
//     Save() → Sync() → BEGIN/COMMIT : Neon reçoit déjà du trafic continu ;
//   - le garde-fou lastWrite saute donc tout ping superflu : la goroutine ne
//     parle à Neon QUE si aucune activité réelle n'est passée depuis 4 min —
//     le coût réel se limite aux périodes où la base serait de toute façon
//     endormie alors que des usagers peuvent arriver.
//
// Modes (variable d'environnement NEON_KEEPALIVE, défaut « business ») :
//   - business : fenêtre d'activité usagers (05:00–24:00 UTC ≈ Abidjan,
//     UTC+0). La nuit, Neon retrouve son autosuspend — le plafond gratuit
//     (191,9 CU-h/mois) reste largement couvert (≈ 142 CU-h au pire).
//   - on (« 24/7 » accepté) : maintien permanent (≈ 180 CU-h/mois).
//   - off : désactivé (aucune goroutine).
func (p *PG) StartKeepAlive(mode string) {
	if mode == "24/7" {
		mode = "on"
	}
	if mode != "on" && mode != "business" {
		log.Printf("pg keep-alive : mode %q ignoré (valeurs : off|on|business)", mode)
		return
	}
	p.kaMode = mode // N°71 — mémorisé pour le diagnostic santé (sync-status)
	if p.kaStop != nil {
		return // déjà démarré
	}
	p.kaStop = make(chan struct{})
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		lastFailLog := time.Time{}
		for {
			select {
			case <-p.kaStop:
				return
			case now := <-ticker.C:
				// N°74 — le tick est protégé : une panique du keep-alive ne
				// doit pas tuer sa propre goroutine à vie (Neon s'endormait
				// alors définitivement sans que rien ne le signale).
				func() {
					defer func() {
						if r := recover(); r != nil {
							log.Printf("pg keep-alive : panique récupérée : %v", r)
						}
					}()
					if mode == "business" && !inBusinessHours(now.UTC()) {
						return
					}
					if now.Sub(time.Unix(p.lastWrite.Load(), 0)) < keepAliveQuietWindow {
						return // activité réelle suffisante : Neon est éveillé
					}
					ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					err := p.db.PingContext(ctx)
					cancel()
					if err != nil {
						// Un ping peut échouer sur un compute en cours de réveil ;
						// le tick suivant réessaie. Log borné (1/h) pour ne pas
						// noyer les journaux Render si Neon est indisponible.
						if time.Since(lastFailLog) > time.Hour {
							lastFailLog = time.Now()
							log.Printf("pg keep-alive : ping échoué (%v) — nouvelle tentative dans 1 min", err)
						}
						return
					}
					p.touchDB()
				}()
			}
		}
	}()
	window := ""
	if mode == "business" {
		window = " (fenêtre 05:00–24:00 UTC)"
	}
	log.Printf("pg keep-alive actif (mode %s%s) : Neon maintenu éveillé en l'absence d'activité réelle", mode, window)
}

// inBusinessHours — fenêtre d'activité usagers en UTC (Abidjan = UTC+0) :
// de 05:00 à 23:59. En dehors, le mode « business » laisse Neon s'endormir
// (économie free tier — aucun usager visé à ces heures).

// inBusinessHours — fenêtre d'activité usagers en UTC (Abidjan = UTC+0) :
// de 05:00 à 23:59. En dehors, le mode « business » laisse Neon s'endormir
// (économie free tier — aucun usager visé à ces heures).
func inBusinessHours(t time.Time) bool {
	return t.Hour() >= 5
}

// ---------------------------------------------------------------------------
// Schéma (idempotent)
// ---------------------------------------------------------------------------

// ensureSchema crée les tables et index si absents. Les horodatages sont
// conservés en TEXT (RFC3339) pour garantir un aller-retour strictement
// identique avec le modèle Go ; last_tick seul est en TIMESTAMPTZ (time.Time).
