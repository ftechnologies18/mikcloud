package api

// Tests N°149 — le journal d'activité (la cloche et la vue Journal) respecte
// le RBAC : les catégories billing (montants, prélèvements) et team
// (membres, rôles) ne partent qu'au propriétaire du compte — le gérant,
// défini produit « tout le compte SAUF équipe et réglages/billing », ne les
// voit plus. Défense en profondeur côté serveur : l'UI masque déjà les vues
// Équipe et réglages, mais /api/activity restait requireRole(2) SANS filtre
// par catégorie — un gérant voyait « Prélèvement carte confirmé — 25 000
// FCFA » dans sa cloche.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"mikcloud/hotspot-api/internal/model"
	"mikcloud/hotspot-api/internal/store"
)

// seedActivityForRBAC — pose un journal mélangé sur le compte : une entrée
// billing récente, une entrée team plus récente encore, et des entrées
// visibles de tout gérant (router, system).
func seedActivityForRBAC(st *store.Store, accID string) {
	st.Lock()
	db := st.Data()
	model.AppendActivity(db, model.Activity{AccountID: accID, Type: "router", Message: "Routeur «Yopougon» hors ligne — sans check-in depuis 4m0s"})
	model.AppendActivity(db, model.Activity{AccountID: accID, Type: "system", Message: "Session support ouverte — la plateforme consulte la console de ce compte"})
	model.AppendActivity(db, model.Activity{AccountID: accID, Type: "billing", Message: "Prélèvement carte confirmé — 25 000 FCFA (abonnement annuel)"})
	model.AppendActivity(db, model.Activity{AccountID: accID, Type: "team", Message: "Membre « assistant » ajouté à l'équipe («manager»)"})
	st.Save()
	st.Unlock()
}

// activityTypesOf — décode la réponse de GET /api/activity et collecte les
// types renvoyés.
func activityTypesOf(t *testing.T, ts *httptest.Server, token string, limit int) map[string]bool {
	t.Helper()
	acts := activityListOf(t, ts, token, limit)
	types := map[string]bool{}
	for _, a := range acts {
		types[a.Type] = true
	}
	return types
}

// activityListOf — GET /api/activity décodé en []model.Activity (la réponse
// est un TABLEAU — doJSON décode en map, inutilisable ici).
func activityListOf(t *testing.T, ts *httptest.Server, token string, limit int) []model.Activity {
	t.Helper()
	req, err := http.NewRequest("GET", ts.URL+"/api/activity?limit="+strconv.Itoa(limit), nil)
	if err != nil {
		t.Fatalf("requête impossible : %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept-Encoding", "identity")
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("GET /api/activity : %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("GET /api/activity : statut %d", resp.StatusCode)
	}
	var acts []model.Activity
	if err := json.NewDecoder(resp.Body).Decode(&acts); err != nil {
		t.Fatalf("décodage /api/activity : %v", err)
	}
	return acts
}

// TestActivityRBACManagerBlindToBillingTeam — le cœur du N°149 : un gérant
// (manager, rang 2) reçoit router/system mais JAMAIS billing ni team ; le
// propriétaire (owner, rang 3) voit tout.
func TestActivityRBACManagerBlindToBillingTeam(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	ownerToken, accID, _ := registerAccount(t, ts, "rbac-owner", "")
	seedActivityForRBAC(st, accID)

	// Le propriétaire voit les quatre catégories.
	types := activityTypesOf(t, ts, ownerToken, 50)
	for _, want := range []string{"router", "system", "billing", "team"} {
		if !types[want] {
			t.Fatalf("owner : l'entrée %q doit être visible, obtenu %v", want, types)
		}
	}

	// Ajout d'un gérant à l'équipe (POST /api/team, owner — cette action
	// journalise elle-même une entrée team).
	status, _ := doJSON(t, ts, "POST", "/api/team", ownerToken, map[string]string{
		"name": "Assistant", "username": "rbac.manager", "password": "mot-de-passe-1", "role": "manager",
	})
	if status != 201 {
		t.Fatalf("création du gérant : statut %d", status)
	}

	// Login du gérant puis lecture du journal.
	loginStatus, out := doJSON(t, ts, "POST", "/api/auth/login", "", map[string]string{
		"username": "rbac.manager", "password": "mot-de-passe-1",
	})
	if loginStatus != 200 {
		t.Fatalf("login gérant : statut %d", loginStatus)
	}
	managerToken, _ := out["token"].(string)
	if managerToken == "" {
		t.Fatal("login gérant : token absent")
	}

	types = activityTypesOf(t, ts, managerToken, 50)
	if types["billing"] {
		t.Fatal("manager : les entrées billing ne doivent PAS partir (montants et prélèvements = propriétaire)")
	}
	if types["team"] {
		t.Fatal("manager : les entrées team ne doivent PAS partir (membres et rôles = propriétaire)")
	}
	if !types["router"] || !types["system"] {
		t.Fatalf("manager : router et system doivent rester visibles (transitions, support), obtenu %v", types)
	}
}

// TestActivityRBACLimitAppliesAfterFilter — la limite ne compte que les
// entrées VISIBLES : un gérant qui demande limit=3 reçoit 3 entrées
// lisibles même quand des entrées billing plus récentes sont filtrées.
func TestActivityRBACLimitAppliesAfterFilter(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	ownerToken, accID, _ := registerAccount(t, ts, "rbac-limit-owner", "")
	seedActivityForRBAC(st, accID)

	status, _ := doJSON(t, ts, "POST", "/api/team", ownerToken, map[string]string{
		"name": "Gerante", "username": "rbac.limit", "password": "mot-de-passe-2", "role": "manager",
	})
	if status != 201 {
		t.Fatalf("création du gérant : statut %d", status)
	}
	loginStatus, out := doJSON(t, ts, "POST", "/api/auth/login", "", map[string]string{
		"username": "rbac.limit", "password": "mot-de-passe-2",
	})
	if loginStatus != 200 {
		t.Fatalf("login gérant : statut %d", loginStatus)
	}
	managerToken, _ := out["token"].(string)

	// Le journal porte 6 entrées (2 seed router/system + billing + team +
	// « Membre ajouté » + « Nouveau compte créé » à l'inscription) ; le
	// gérant avec limit=3 doit recevoir EXACTEMENT 3 entrées visibles.
	acts := activityListOf(t, ts, managerToken, 3)
	if len(acts) != 3 {
		t.Fatalf("limit=3 après filtre RBAC : 3 entrées attendues, %d obtenues (la limite doit compter les entrées VISIBLES)", len(acts))
	}
	for _, a := range acts {
		if a.Type == "billing" || a.Type == "team" {
			t.Fatalf("entrée %q ne doit jamais partir à un gérant", a.Type)
		}
	}
}
