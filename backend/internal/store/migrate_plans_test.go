// migrate_plans_test.go — N°122 : migration idempotente des identifiants de
// formule historiques (« essentiel »/« illimite ») vers les formules
// segmentées du mode du compte (Hotspot/HomeNet).
package store

import (
	"testing"

	"mikcloud/hotspot-api/internal/model"
)

func TestMigrateUsageScopedPlans(t *testing.T) {
	db := &model.DB{
		Accounts: []model.Account{
			{ID: "acc-hot", Usage: model.AccountUsageHotspot},
			{ID: "acc-home", Usage: model.AccountUsageHomeNet},
			{ID: "acc-vide"}, // usage vide → hotspot (produit historique)
		},
		SettingsByAccount: map[string]model.Settings{
			"acc-hot":  {Subscription: model.Subscription{PlanID: "essentiel", Status: "active"}},
			"acc-home": {Subscription: model.Subscription{PlanID: "illimite", Status: "active"}},
			"acc-vide": {Subscription: model.Subscription{PlanID: "essentiel", Status: "active"}},
		},
		BillingRequests: []model.BillingRequest{
			{ID: "breq-1", AccountID: "acc-home", PlanID: "essentiel", Status: "pending"},
			{ID: "breq-2", AccountID: "acc-hot", PlanID: "illimite", Status: "done"},
		},
	}

	if !migrateUsageScopedPlans(db) {
		t.Fatal("la migration initiale doit signaler un changement")
	}

	veut := map[string]string{
		"acc-hot":  "hotspot-mensuel", // essentiel + hotspot
		"acc-home": "homenet-annuel",  // illimite + homenet
		"acc-vide": "hotspot-mensuel", // essentiel + usage vide → hotspot
	}
	for accID, want := range veut {
		if got := db.SettingsByAccount[accID].Subscription.PlanID; got != want {
			t.Fatalf("abonnement %s : PlanID = %q, attendu %q", accID, got, want)
		}
	}
	// Libellé compat rafraîchi (formule segmentée, plafond cohérent).
	if p := db.SettingsByAccount["acc-home"].Plan; p.Name != "MikCloud HomeNet Annuel" || p.MaxRouters != "Illimité" {
		t.Fatalf("libellé compat acc-home = %+v", db.SettingsByAccount["acc-home"].Plan)
	}
	if p := db.SettingsByAccount["acc-hot"].Plan; p.Name != "MikCloud Hotspot Mensuel" || p.MaxRouters != "Par routeur" {
		t.Fatalf("libellé compat acc-hot = %+v", db.SettingsByAccount["acc-hot"].Plan)
	}
	// Demandes de facturation : résolues au mode du compte.
	if got := db.BillingRequests[0].PlanID; got != "homenet-mensuel" {
		t.Fatalf("demande breq-1 : PlanID = %q, attendu homenet-mensuel", got)
	}
	if got := db.BillingRequests[1].PlanID; got != "hotspot-annuel" {
		t.Fatalf("demande breq-2 : PlanID = %q, attendu hotspot-annuel", got)
	}

	// Idempotence : un second passage ne change rien.
	if migrateUsageScopedPlans(db) {
		t.Fatal("la migration doit être idempotente (second passage sans changement)")
	}

	// Les essais et les identifiants segmentés ne sont jamais touchés.
	db2 := &model.DB{
		Accounts: []model.Account{{ID: "a1", Usage: model.AccountUsageHomeNet}},
		SettingsByAccount: map[string]model.Settings{
			"a1": {Subscription: model.Subscription{PlanID: "essai", Status: "active"}},
		},
	}
	if migrateUsageScopedPlans(db2) {
		t.Fatal("un essai ne doit pas être migré")
	}
	s := db2.SettingsByAccount["a1"]
	s.Subscription.PlanID = "homenet-annuel"
	db2.SettingsByAccount["a1"] = s
	if migrateUsageScopedPlans(db2) {
		t.Fatal("un identifiant segmenté ne doit pas être re-migré")
	}
}
