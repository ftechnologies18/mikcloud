// Tests N°26 — lots éteints : un lot dont TOUS les tickets sont expirés
// disparaît du système :
//   - tickets supprimés + ligne de lot supprimée ;
//   - tombstones anti-résurrection posés (les tickets restent sur le routeur) ;
//   - commande user_remove enfilée pour le routeur AGENT (rien pour un
//     routeur simulé) ;
//   - sessions des tickets supprimés abandonnées ;
//   - lot MIXTE (au moins un ticket non expiré) conservé ;
//   - lot avec ticket REVENDEUR conservé (N°23/W1 — trace du stock confié) ;
//   - ventes/transactions intacts ; sweep idempotent.
//
// Aucune connexion réseau : model.DB construite à la main + sweepDeadBatches
// direct (fonction pure sur l'état passé en argument).
package api

import (
	"testing"

	"mikcloud/hotspot-api/internal/model"
)

func sweepTicket(id, batchID, name, status, routerID, resellerID string) model.HotspotUser {
	return model.HotspotUser{
		ID: id, AccountID: "acc-1", Kind: "voucher",
		Username: name, Password: name,
		Status: status, BatchID: batchID,
		RouterID: routerID, ResellerID: resellerID,
		ProfileID: "p-1", ProfileName: "1h",
		CreatedAt: model.NowISO(),
	}
}

func TestSweepDeadBatches(t *testing.T) {
	db := &model.DB{
		Routers: []model.Router{
			{ID: "r-agent", AccountID: "acc-1", Name: "Agent", Mode: "agent"},
			{ID: "r-sim", AccountID: "acc-1", Name: "Simulé", Mode: "simulated"},
		},
		HotspotUsers: []model.HotspotUser{
			// B1 — tous expirés, détenteur direct, routeur AGENT → disparaît.
			sweepTicket("t-1a", "B1", "a1", "expired", "r-agent", ""),
			sweepTicket("t-1b", "B1", "a2", "expired", "r-agent", ""),
			sweepTicket("t-1c", "B1", "a3", "expired", "r-agent", ""),
			// B2 — mixte (un « used ») → conservé.
			sweepTicket("t-2a", "B2", "b1", "expired", "r-agent", ""),
			sweepTicket("t-2b", "B2", "b2", "used", "r-agent", ""),
			// B3 — tous expirés MAIS un ticket revendeur (N°23/W1) → conservé.
			sweepTicket("t-3a", "B3", "c1", "expired", "r-agent", ""),
			sweepTicket("t-3b", "B3", "c2", "expired", "r-agent", "res-9"),
			// B4 — tous expirés, routeur SIMULÉ → disparaît, aucune commande.
			sweepTicket("t-4a", "B4", "d1", "expired", "r-sim", ""),
		},
		Batches: []model.Batch{
			{ID: "B1", AccountID: "acc-1", Count: 3},
			{ID: "B2", AccountID: "acc-1", Count: 2},
			{ID: "B3", AccountID: "acc-1", Count: 2},
			{ID: "B4", AccountID: "acc-1", Count: 1},
		},
		Sessions: []model.Session{
			{ID: "s-1", AccountID: "acc-1", Username: "a1", RouterID: "r-agent"},
			{ID: "s-2", AccountID: "acc-1", Username: "b2", RouterID: "r-agent"},
		},
		Sales: []model.Sale{
			{ID: "sale-1", AccountID: "acc-1", BatchID: "B1", Amount: 300, Count: 3},
		},
	}

	n := sweepDeadBatches(db)
	if n != 2 {
		t.Fatalf("lots supprimés : 2 attendus (B1 + B4), obtenu %d", n)
	}

	// Tickets restants : B2 (2) + B3 (2) — B1 et B4 partis.
	if got := len(db.HotspotUsers); got != 4 {
		t.Fatalf("tickets restants : 4 attendus, obtenu %d", got)
	}
	for _, u := range db.HotspotUsers {
		if u.BatchID == "B1" || u.BatchID == "B4" {
			t.Fatalf("ticket %s du lot éteint encore présent", u.ID)
		}
	}

	// Lignes de lot restantes : B2 + B3 uniquement.
	if got := len(db.Batches); got != 2 {
		t.Fatalf("lots restants : 2 attendus (B2 + B3), obtenu %d", got)
	}
	for _, b := range db.Batches {
		if b.ID == "B1" || b.ID == "B4" {
			t.Fatalf("lot éteint %s encore présent", b.ID)
		}
	}

	// Tombstones : 4 posés (a1, a2, a3, d1), tous du compte, minuscules.
	if got := len(db.PurgeTombstones); got != 4 {
		t.Fatalf("tombstones : 4 attendus, obtenu %d", got)
	}
	seen := map[string]bool{}
	for _, tb := range db.PurgeTombstones {
		if tb.AccountID != "acc-1" {
			t.Fatalf("tombstone hors compte : %+v", tb)
		}
		seen[tb.Username] = true
	}
	for _, want := range []string{"a1", "a2", "a3", "d1"} {
		if !seen[want] {
			t.Fatalf("tombstone manquant pour %q (obtenu %v)", want, seen)
		}
	}

	// Commandes routeur : UNE CmdUserRemove pour r-agent (3 noms), AUCUNE pour r-sim.
	removes := 0
	for _, c := range db.Commands {
		if c.Kind != model.CmdUserRemove {
			t.Fatalf("commande inattendue : %s", c.Kind)
		}
		removes++
		if c.RouterID != "r-agent" {
			t.Fatalf("user_remove hors routeur agent : %+v", c)
		}
		names, ok := c.Payload["names"].([]string)
		if !ok || len(names) != 3 {
			t.Fatalf("payload user_remove : 3 noms attendus, obtenu %v", c.Payload["names"])
		}
	}
	if removes != 1 {
		t.Fatalf("commandes user_remove : 1 attendue, obtenue %d", removes)
	}

	// Sessions : celle du ticket supprimé (a1) abandonnée, l'autre conservée.
	if got := len(db.Sessions); got != 1 || db.Sessions[0].Username != "b2" {
		t.Fatalf("sessions : [b2] attendu, obtenu %v", db.Sessions)
	}

	// Comptabilité intacte : la vente du lot B1 reste la trace du revenu.
	if got := len(db.Sales); got != 1 {
		t.Fatalf("ventes : 1 attendue, obtenu %d", got)
	}

	// Journal d'activité : 1 entrée pour le compte.
	if got := len(db.Activity); got != 1 {
		t.Fatalf("activité : 1 entrée attendue, obtenue %d", got)
	}

	// Idempotence : un second passage ne supprime plus rien, ne duplique
	// aucun tombstone ni commande.
	if n := sweepDeadBatches(db); n != 0 {
		t.Fatalf("second passage : 0 lot attendu, obtenu %d", n)
	}
	if got := len(db.PurgeTombstones); got != 4 {
		t.Fatalf("tombstones après second passage : 4 attendus, obtenu %d", got)
	}
	if got := len(db.Commands); got != 1 {
		t.Fatalf("commandes après second passage : 1 attendue, obtenue %d", got)
	}
}
