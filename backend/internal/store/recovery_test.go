// recovery_test.go — N°164 : tests du boot résilient.
//
// Les invariants testés sont les GARANTIES de la revue d'implémentation :
//   - la fusion ne perd NI l'historique de la base NI les écritures de la
//     fenêtre dégradée (union, mémoire gagnante sur collision) ;
//   - un username purgé (tombstone) ne ressuscite pas par la fusion ;
//   - un boot sans PostgreSQL ne fait plus crasher le service ET Close ne
//     bloque pas (le syncreur n'étant jamais démarré en mode dégradé).
package store

import (
	"testing"
	"time"

	"mikcloud/hotspot-api/internal/model"
)

// TestMergeRecoveredUnion — la fusion est une UNION par clé primaire : les
// lignes base absentes de la mémoire reviennent, les lignes créées pendant
// la fenêtre dégradée restent, et la MÉMOIRE gagne les collisions (une ligne
// modifiée pendant la fenêtre n'est pas écrasée par la version gelée).
func TestMergeRecoveredUnion(t *testing.T) {
	dst := &model.DB{
		Accounts: []model.Account{
			{ID: "acc-1", Name: "mem-modifie"}, // collision : version MÉMOIRE
			{ID: "acc-2", Name: "mem-seul"},    // créé pendant la fenêtre
		},
		Routers: []model.Router{{ID: "rt-1"}},
	}
	src := &model.DB{
		Accounts: []model.Account{
			{ID: "acc-1", Name: "base-gelée"},   // ne doit PAS écraser
			{ID: "acc-3", Name: "base-seule"},   // doit être restauré
		},
		Routers: []model.Router{{ID: "rt-1"}, {ID: "rt-2"}},
	}

	n := mergeRecovered(dst, src)
	if n != 2 {
		t.Fatalf("lignes restaurées attendues : 2 (acc-3, rt-2) — obtenu %d", n)
	}
	if len(dst.Accounts) != 3 {
		t.Fatalf("3 comptes attendus après fusion, obtenu %d", len(dst.Accounts))
	}
	if len(dst.Routers) != 2 {
		t.Fatalf("2 routeurs attendus après fusion, obtenu %d", len(dst.Routers))
	}
	byID := map[string]string{}
	for _, a := range dst.Accounts {
		byID[a.ID] = a.Name
	}
	if byID["acc-1"] != "mem-modifie" {
		t.Fatalf("collision : la mémoire doit garder la main — acc-1 = %q", byID["acc-1"])
	}
	if byID["acc-2"] != "mem-seul" || byID["acc-3"] != "base-seule" {
		t.Fatalf("union incomplète : acc-2=%q acc-3=%q", byID["acc-2"], byID["acc-3"])
	}
}

// TestMergeRecoveredTombstones — anti-résurgence : un username purgé avant
// l'incident (tombstone vivant côté base) ne doit PAS revenir par la fusion,
// même si sa ligne traîne encore dans la base (synchro de la purge jamais
// passée). Les autres utilisateurs sont restaurés normalement.
func TestMergeRecoveredTombstones(t *testing.T) {
	dst := &model.DB{
		HotspotUsers:    []model.HotspotUser{{ID: "hu-1", Username: "vivant"}},
		PurgeTombstones: []model.PurgeTombstone{{ID: "pt-1", Username: "Purge"}},
	}
	src := &model.DB{
		HotspotUsers: []model.HotspotUser{
			{ID: "hu-1", Username: "vivant"},        // présent des deux côtés
			{ID: "hu-2", Username: "purge"},         // tombstoné (casse différente)
			{ID: "hu-3", Username: "restaure"},      // doit revenir
		},
		PurgeTombstones: []model.PurgeTombstone{{ID: "pt-2", Username: "autre"}},
	}

	n := mergeRecovered(dst, src)
	if n != 2 { // hu-3 (restauré) + tombstone pt-2 (union des tombstones)
		t.Fatalf("ajouts attendus : 2 — obtenu %d", n)
	}
	for _, u := range dst.HotspotUsers {
		if u.Username == "purge" {
			t.Fatal("un username tombstoné a resurgi par la fusion")
		}
	}
	found := false
	for _, u := range dst.HotspotUsers {
		if u.ID == "hu-3" {
			found = true
		}
	}
	if !found {
		t.Fatal("hu-3 (non tombstoné) n'a pas été restauré par la fusion")
	}
}

// TestMergeRecoveredMapsAndClocks — cartes par compte complétées sans
// écrasement, horloges globales conservées au plus récent.
func TestMergeRecoveredMapsAndClocks(t *testing.T) {
	memTick := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	baseTick := time.Date(2026, 9, 20, 4, 0, 0, 0, time.UTC)
	dst := &model.DB{
		SettingsByAccount: map[string]model.Settings{
			"acc-1": {Tenant: model.Tenant{Name: "mem"}},
		},
		LastTick:  memTick,
		LastSweep: baseTick,
	}
	src := &model.DB{
		SettingsByAccount: map[string]model.Settings{
			"acc-1": {Tenant: model.Tenant{Name: "base"}}, // ne doit PAS écraser
			"acc-2": {Tenant: model.Tenant{Name: "base2"}}, // doit être complétée
		},
		NotifSettings: map[string]model.NotificationSettings{
			"acc-2": {AccountID: "acc-2"},
		},
		LastTick:  baseTick,
		LastSweep: memTick,
	}

	mergeRecovered(dst, src)
	if dst.SettingsByAccount["acc-1"].Tenant.Name != "mem" {
		t.Fatal("la carte settings a été écrasée par la version base")
	}
	if _, ok := dst.SettingsByAccount["acc-2"]; !ok {
		t.Fatal("le compte acc-2 (settings base) n'a pas été fusionné")
	}
	if _, ok := dst.NotifSettings["acc-2"]; !ok {
		t.Fatal("les réglages de notification acc-2 n'ont pas été fusionnés")
	}
	if !dst.LastTick.Equal(memTick) || !dst.LastSweep.Equal(memTick) {
		t.Fatal("les horloges globales doivent garder la valeur la plus récente")
	}
}

// TestDegradedBootAndClose — LE test de régression de l'incident du 20/09 :
// une base injoignable au démarrage ne doit PLUS tuer le service (ancien
// comportement : erreur de New → log.Fatalf en production). Le store démarre
// dégradé (mémoire seule, persistance suspendue), Save() ne panique pas, et
// Close() rend la main SANS attendre un syncreur jamais démarré (le bug
// naïf : <-s.syncDone bloque à jamais).
func TestDegradedBootAndClose(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://mikcloud:test@127.0.0.1:9/none?sslmode=disable")
	// Port 9 (discard) : connexion refusée immédiatement à chaque tentative —
	// OpenPG borne ses retries à 10 × 1 s.

	s, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("le boot dégradé ne doit PAS échouer (regression N°164) : %v", err)
	}
	if !s.pgActive.Load() {
		t.Fatal("le mode PostgreSQL doit être décidé (pgActive) même dégradé")
	}
	if !s.pgDegraded.Load() {
		t.Fatal("le store doit se déclarer dégradé")
	}
	if s.Data() == nil {
		t.Fatal("l'état mémoire doit exister en mode dégradé")
	}
	// La persistance est suspendue : Save() marque sans écrire ni paniquer.
	s.Save()
	s.SaveTables(TableAccounts)

	// Close doit rendre la main rapidement (gardé par un timeout explicite :
	// l'ancien close sur syncDone non fermé bloquait indéfiniment).
	done := make(chan error, 1)
	go func() { done <- s.Close() }()
	select {
	case <-done:
		// OK — c'est l'invariant.
	case <-time.After(20 * time.Second):
		t.Fatal("Close() bloque en mode dégradé (syncDone jamais fermé)")
	}
	// Idempotence du double Close.
	if err := s.Close(); err != nil {
		t.Fatalf("double Close doit être silencieux : %v", err)
	}
}
