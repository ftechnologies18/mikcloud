// Package api — tombstones de purge (audit purge/résurgence).
//
// PROBLÈME : la purge admin supprime du cloud (mémoire + Neon) mais PAS des
// routeurs réels — décision de sécurité assumée. Le read_state agent qui suit
// (≤ 45 s) ré-importait alors TOUT ce que le routeur garde encore : la purge
// semblait ne rien faire (résurgence), et un voucher purgé revenait en
// « utilisateur régulier » fantôme, sans lot ni vente.
//
// SOLUTION : chaque username purgé reçoit un TOMBSTONE (marqueur daté, TTL 30
// jours) que applyReadState consulte AVANT tout ré-import. Le tombstone :
//   - BLOQUE le ré-import (users, sessions, journaux, ventes auto) ;
//   - EXPIRE seul (30 j) — la découverte Winbox redevient possible ;
//   - SE LÈVE à la création volontaire du même username (liftPurgeTombstone).
//
// Effets annexes couverts : la file de commandes en attente qui recréerait
// les entités purgées est annulée (cancelQueuedForPurged) ; les commandes
// user_add déjà ENVOYÉES restent exécutées par l'agent, mais leur read_state
// suivant est filtré par les tombstones — aucun effet de bord cloud.
package api

import (
	"strings"
	"time"

	"mikcloud/hotspot-api/internal/model"
)

// purgeTombSet — ensemble des usernames tombstonés (non expirés) d'un compte.
// Élège au passage les tombstones EXPIRÉS du compte (état mémoire) : la diff
// syncTable supprimera les lignes Neon correspondantes au Save suivant.
// À appeler sous verrou.
func purgeTombSet(db *model.DB, accID string, now time.Time) map[string]bool {
	kept := db.PurgeTombstones[:0]
	set := map[string]bool{}
	for _, t := range db.PurgeTombstones {
		if t.AccountID != accID {
			kept = append(kept, t)
			continue
		}
		if exp, err := time.Parse(time.RFC3339, t.ExpiresAt); err == nil && now.After(exp) {
			continue // expiré : élagué — l'import redevient possible
		}
		set[t.Username] = true
		kept = append(kept, t)
	}
	db.PurgeTombstones = kept
	return set
}

// importAutoEnabled — valeur EFFECTIVE du réglage d'import automatique d'un
// compte (Settings.AutoImportRouterUsers, nil = ON : comportement historique
// de découverte des comptes créés hors MikCloud). À appeler sous verrou.
func importAutoEnabled(db *model.DB, accID string) bool {
	if s, ok := db.SettingsByAccount[accID]; ok {
		return s.ImportAutoEnabled()
	}
	return true
}

// liftPurgeTombstone — lève le tombstone d'un (compte, username) : appelé à la
// CRÉATION VOLONTAIRE d'un utilisateur dans MikCloud. L'opérateur réclame le
// nom : la purge ne doit plus le bloquer, ni côté cloud (recréation) ni côté
// routeur (la commande user_add doit pouvoir aboutir et être reflétée).
// À appeler sous verrou.
func liftPurgeTombstone(db *model.DB, accID, username string) {
	low := strings.ToLower(username)
	if low == "" || len(db.PurgeTombstones) == 0 {
		return
	}
	kept := db.PurgeTombstones[:0]
	for _, t := range db.PurgeTombstones {
		if t.AccountID == accID && t.Username == low {
			continue // levé
		}
		kept = append(kept, t)
	}
	db.PurgeTombstones = kept
}

// writePurgeTombstones — pose (ou rafraîchit) un tombstone par (compte,
// username) purgé ; retourne le nombre de tombstones écrits. Les usernames
// arrivent en minuscules (clés de purgedUsers). À appeler sous verrou.
func writePurgeTombstones(db *model.DB, purgedUsers map[string]map[string]bool, now time.Time) int {
	if len(purgedUsers) == 0 {
		return 0
	}
	nowISO := now.UTC().Format(time.RFC3339)
	expISO := now.Add(model.PurgeTombstoneTTL).UTC().Format(time.RFC3339)
	idx := map[string]int{}
	for i := range db.PurgeTombstones {
		t := &db.PurgeTombstones[i]
		idx[t.AccountID+"|"+t.Username] = i
	}
	n := 0
	for accID, names := range purgedUsers {
		for name := range names {
			if i, ok := idx[accID+"|"+name]; ok {
				db.PurgeTombstones[i].PurgedAt = nowISO // re-purgé : TTL repart
				db.PurgeTombstones[i].ExpiresAt = expISO
				n++
				continue
			}
			db.PurgeTombstones = append(db.PurgeTombstones, model.PurgeTombstone{
				ID: model.NewID("pt-"), AccountID: accID, Username: name,
				PurgedAt: nowISO, ExpiresAt: expISO,
			})
			n++
		}
	}
	return n
}

// cancelQueuedForPurged — retire de la file les commandes qui recréeraient
// les entités purgées : user_add d'un username purgé, et voucher_batch dont
// le compte vient de perdre ses vouchers (l'exécution écrirait des comptes
// que le cloud vient d'effacer). Les commandes déjà ENVOYÉES (sent) sont
// conservées : l'agent les exécutera, et le read_state suivant — filtré par
// les tombstones — empêchera toute résurrection côté cloud. À appeler sous
// verrou. Retourne le nombre de commandes annulées.
func cancelQueuedForPurged(db *model.DB, purgedUsers map[string]map[string]bool) int {
	if len(purgedUsers) == 0 {
		return 0
	}
	kept := db.Commands[:0]
	cancelled := 0
	for _, c := range db.Commands {
		if c.Status == "queued" && purgedUsers[c.AccountID] != nil {
			names := purgedUsers[c.AccountID]
			if c.Kind == model.CmdVoucherBatch {
				cancelled++ // les vouchers du batch viennent d'être purgés
				continue
			}
			if c.Kind == model.CmdUserAdd {
				if name, _ := c.Payload["name"].(string); name != "" && names[strings.ToLower(name)] {
					cancelled++
					continue
				}
			}
		}
		kept = append(kept, c)
	}
	db.Commands = kept
	return cancelled
}

// queueRemovalsForPurgedAgent — PURGE TOTALE (opt-in) : enfile une commande
// user_remove pour chaque routeur AGENT qui détenait des comptes purgés
// (lots de 50, miroir du nettoyage d'expirés — payload {"names": [...]}).
// Les routeurs REAL (passerelle) ne peuvent pas être commandés après coup :
// la purge a déjà effacé les identifiants cloud que la passerelle adresse ;
// leurs comptes restants sont visibles via Router.UnknownOnRouter (adoption
// ou retrait manuel). Les routeurs SIMULÉS disparaissent avec le scope.
// À appeler sous verrou. Retourne le nombre de usernames commandés.
func queueRemovalsForPurgedAgent(db *model.DB, purgedAgent map[string][]string) int {
	if len(purgedAgent) == 0 {
		return 0
	}
	total := 0
	for routerID, names := range purgedAgent {
		if len(names) == 0 {
			continue
		}
		for i := range db.Routers {
			if db.Routers[i].ID != routerID {
				continue
			}
			rr := &db.Routers[i]
			for start := 0; start < len(names); start += 50 {
				end := min(start+50, len(names))
				queueCommandLocked(db, rr.AccountID, rr.ID, model.CmdUserRemove, map[string]any{"names": names[start:end]})
			}
			total += len(names)
			break
		}
	}
	return total
}
