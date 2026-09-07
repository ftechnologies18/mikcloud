// retention.go — N°64 — balayage PÉRIODIQUE de rétention (privacy/audit).
//
// AVANT : la purge des journaux utilisateurs (90 j, F3), le moteur
// d'expiration et les nettoyages associés ne tournaient qu'au fil des
// LECTURES console (store.Tick en tête des handlers — balayage paresseux).
// Un compte dormant, jamais consulté, conservait donc ses journaux
// connexion au-delà de la rétention annoncée : la garantie « 90 jours
// maximum » n'était vraie que pour les comptes actifs.
//
// DEPUIS : une goroutine (main.go) exécute ce balayage toutes les heures,
// avec un rattrapage immédiat au démarrage (le service Render redémarre
// souvent — chaque boot nettoie ce qui doit l'être). Le passage reprend
// EXACTEMENT le cœur commun des handlers, sans la progression de la
// simulation (sessions/uptime/télémétrie restent au rythme des polls) :
//
//	store.Sweep     — applyExpiry : expirations vouchers + politique
//	                  « remove » + purge UserLogs > 90 j + plafond 5 000,
//	                  date le passage (db.LastSweep) ;
//	enforceExpired  — commandes agent des expirations (Enforced),
//	                  réparation limit-uptime, lots morts (N°26),
//	                  inscriptions stalées 30 j (N°27) ;
//	Save            — suppression RÉELLE côté PostgreSQL (syncTable).
//
// Garantie vérifiable en audit : GET / expose lastSweepAt (persistée en
// base, colonne settings.last_sweep) et le log service trace chaque purge
// non vide.
package api

import (
	"log"
	"time"

	"mikcloud/hotspot-api/internal/store"
)

// retentionSweepInterval — période du balayage de fond. La rétention
// (90 j, cf. store.userLogRetention) se purge donc à l'heure près ; une
// heure laisse largement le temps à un Save PostgreSQL complet entre
// deux passages et reste insignifiant côté charge.
const retentionSweepInterval = time.Hour

// RunRetentionSweepForever — boucle du balayage (lancée en goroutine par
// main.go) : rattrapage immédiat au démarrage, puis passage périodique.
func (a *API) RunRetentionSweepForever() {
	a.RunRetentionSweep()
	for range time.Tick(retentionSweepInterval) {
		a.RunRetentionSweep()
	}
}

// RunRetentionSweep — un passage du balayage : verrou → moteur commun →
// Save. Aucune requête HTTP ne dépend de ce passage (il ne fait qu'avancer
// l'état que les prochaines lectures verront déjà à jour).
func (a *API) RunRetentionSweep() {
	now := time.Now().UTC()
	a.store.Lock()
	db := a.store.Data()
	purged := store.Sweep(db, now)
	a.enforceExpired(db) // même passage commun que les lectures console
	a.store.Save()
	a.store.Unlock()
	if purged > 0 {
		log.Printf("rétention (90 j) : %d entrée(s) du journal utilisateurs purgée(s)", purged)
	}
}
