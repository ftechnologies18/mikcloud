package api

// Tests N°98 — Phase 1 Hotspot/HomeNet (plomberie invisible) :
//   - contrat d'inscription publique : N°98 refusait « homenet » (la
//     coquille n'existait pas) — N°101 (Phase 3) l'OUVRE, les DEUX usages
//     sont acceptés (absent = défaut hotspot, valeur inconnue = refusée) ;
//   - la session transporte l'usage : login, register, /api/auth/me ;
//   - la garde requireUsage : les endpoints PRODUIT hotspot répondent 404
//     aux comptes homenet (créés par la console plateforme), les endpoints
//     PARTAGÉS (dashboard, routeurs, sessions, réglages) restent ouverts —
//     même token, zéro re-login ;
//   - exemption plateforme : la session support (impersonation d'un compte
//     homenet par l'admin) passe partout ;
//   - bascule admin PUT /api/admin/accounts/{id}/usage : effet immédiat
//     (même token), idempotence, refus du compte principal, des valeurs
//     inconnues et des non-admin ;
//   - normalisation : un compte sans usage (ère pré-colonne) est lu comme
//     hotspot, au store (Reload → migrateMultiTenant) comme à la garde
//     (repli défensif sans rechargement).

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"mikcloud/hotspot-api/internal/auth"
	"mikcloud/hotspot-api/internal/model"
)

// uniquePhone — S5 dédoublonne email/WhatsApp : chaque inscription de test
// porte un numéro distinct (compteur global, 10 chiffres).
var uniquePhoneCounter atomic.Int64

func uniquePhone() string {
	return fmt.Sprintf("070707%04d", uniquePhoneCounter.Add(1))
}

// registerUsageBody — corps d'inscription publique avec usage EXPLICITE
// (le helper registerAccount teste déjà le contrat « champ absent »).
func registerUsageBody(username, usage string) map[string]string {
	body := map[string]string{
		"name":     "Gérant " + username,
		"username": username,
		"password": "mot-de-passe-8+",
		"email":    username + "@example.ci",
		"phone":    uniquePhone(),
		"country":  "CI",
		"city":     "Abidjan",
	}
	if usage != "" {
		body["usage"] = usage
	}
	return body
}

// TestRegisterUsageBothOpen — N°101 (Phase 3) inverse le contrat N°98 :
// l'inscription publique accepte les DEUX usages. Champ absent → défaut
// hotspot (les clients existants ne changent pas d'un octet) ; homenet →
// 201 avec usage transporté par la session (la coquille N°100 et les
// features maison existent) ; valeur inconnue → 400.
func TestRegisterUsageBothOpen(t *testing.T) {
	ts := newTestServer(t)

	// Champ absent → 201, compte hotspot (les clients existants ne
	// changent pas d'un octet).
	status, out := doJSON(t, ts, "POST", "/api/auth/register", "", registerUsageBody("usage-absent", ""))
	if status != http.StatusCreated {
		t.Fatalf("inscription sans usage : statut %d, corps %v", status, out)
	}
	user, _ := out["user"].(map[string]any)
	if user["usage"] != model.AccountUsageHotspot {
		t.Fatalf("usage par défaut = %v, attendu %q", user["usage"], model.AccountUsageHotspot)
	}

	// « hotspot » explicite → 201.
	if status, _ := doJSON(t, ts, "POST", "/api/auth/register", "", registerUsageBody("usage-hs", model.AccountUsageHotspot)); status != http.StatusCreated {
		t.Fatalf("inscription hotspot explicite : statut %d, attendu 201", status)
	}

	// N°101 — « homenet » → 201 : l'inscription publique du foyer est
	// OUVERTE (Phase 3 : dashboard maison + appareils + protection
	// re-skinée existent, l'atterrissage client suit l'usage).
	status, out = doJSON(t, ts, "POST", "/api/auth/register", "", registerUsageBody("usage-hn", model.AccountUsageHomeNet))
	if status != http.StatusCreated {
		t.Fatalf("inscription homenet : statut %d, attendu 201 (ouverte depuis la Phase 3)", status)
	}
	user, _ = out["user"].(map[string]any)
	if user["usage"] != model.AccountUsageHomeNet {
		t.Fatalf("usage homenet = %v, attendu %q", user["usage"], model.AccountUsageHomeNet)
	}

	// Valeur inconnue → 400.
	if status, _ := doJSON(t, ts, "POST", "/api/auth/register", "", registerUsageBody("usage-x", "cybercafe")); status != http.StatusBadRequest {
		t.Fatalf("usage inconnu : statut %d, attendu 400", status)
	}
}

// TestLoginMeCarryUsage — l'usage voyage dans la session : register, login
// ET /api/auth/me renvoient la même clé « usage » (la coquille Phase 2
// lira l'une ou l'autre).
func TestLoginMeCarryUsage(t *testing.T) {
	ts := newTestServer(t)

	status, out := doJSON(t, ts, "POST", "/api/auth/register", "", registerUsageBody("session-usage", ""))
	if status != http.StatusCreated {
		t.Fatalf("inscription : statut %d, corps %v", status, out)
	}
	user, _ := out["user"].(map[string]any)
	if user["usage"] != model.AccountUsageHotspot {
		t.Fatalf("register : usage = %v, attendu %q", user["usage"], model.AccountUsageHotspot)
	}

	status, out = doJSON(t, ts, "POST", "/api/auth/login", "", map[string]string{
		"username": "session-usage",
		"password": "mot-de-passe-8+",
	})
	if status != http.StatusOK {
		t.Fatalf("login : statut %d, corps %v", status, out)
	}
	token, _ := out["token"].(string)
	user, _ = out["user"].(map[string]any)
	if user["usage"] != model.AccountUsageHotspot {
		t.Fatalf("login : usage = %v, attendu %q", user["usage"], model.AccountUsageHotspot)
	}

	status, out = doJSON(t, ts, "GET", "/api/auth/me", token, nil)
	if status != http.StatusOK {
		t.Fatalf("me : statut %d", status)
	}
	user, _ = out["user"].(map[string]any)
	if user["usage"] != model.AccountUsageHotspot {
		t.Fatalf("me : usage = %v, attendu %q", user["usage"], model.AccountUsageHotspot)
	}
}

// adminLogin — token de l'admin plateforme bootstrappé par ADMIN_PASSWORD
// (username « admin » — cf. applyAdminOverride).
func adminLogin(t *testing.T, ts *httptest.Server) string {
	t.Helper()
	status, out := doJSON(t, ts, "POST", "/api/auth/login", "", map[string]string{
		"username": "admin",
		"password": "admin-test-1234",
	})
	if status != http.StatusOK {
		t.Fatalf("login admin : statut %d, corps %v", status, out)
	}
	token, _ := out["token"].(string)
	return token
}

// createAdminAccount — POST /api/admin/accounts avec un usage donné ;
// retourne (accountID, token du owner créé). Vérifie AUSSI que le login du
// owner transporte l'usage demandé.
func createAdminAccount(t *testing.T, ts *httptest.Server, adminToken, username, usage string) (string, string) {
	t.Helper()
	body := map[string]string{
		"name":     "Compte " + username,
		"username": username,
		"password": "mot-de-passe-8+",
		"email":    username + "@example.ci",
		"phone":    uniquePhone(),
		"country":  "CI",
		"city":     "Abidjan",
	}
	if usage != "" {
		body["usage"] = usage
	}
	status, out := doJSON(t, ts, "POST", "/api/admin/accounts", adminToken, body)
	if status != http.StatusCreated {
		t.Fatalf("création admin %s (usage=%s) : statut %d, corps %v", username, usage, status, out)
	}
	acc, _ := out["account"].(map[string]any)
	accID, _ := acc["id"].(string)

	// Usage inconnu → refusé côté création admin aussi.
	status, out = doJSON(t, ts, "POST", "/api/auth/login", "", map[string]string{
		"username": username,
		"password": "mot-de-passe-8+",
	})
	if status != http.StatusOK {
		t.Fatalf("login du owner %s : statut %d, corps %v", username, status, out)
	}
	token, _ := out["token"].(string)
	user, _ := out["user"].(map[string]any)
	want := usage
	if want == "" {
		want = model.AccountUsageHotspot
	}
	if user["usage"] != want {
		t.Fatalf("login du owner %s : usage = %v, attendu %q", username, user["usage"], want)
	}
	return accID, token
}

// TestAdminCreateUsageValidation — la création plateforme valide l'usage :
// valeur inconnue → 400 ; champ absent → hotspot (défaut historique).
func TestAdminCreateUsageValidation(t *testing.T) {
	ts := newTestServer(t)
	adminToken := adminLogin(t, ts)

	body := map[string]string{
		"name":     "Compte bizarre",
		"username": "compte-bizarre",
		"password": "mot-de-passe-8+",
		"email":    "bizarre@example.ci",
		"phone":    uniquePhone(),
		"country":  "CI",
		"usage":    "cybercafe",
	}
	if status, _ := doJSON(t, ts, "POST", "/api/admin/accounts", adminToken, body); status != http.StatusBadRequest {
		t.Fatalf("création admin usage inconnu : statut %d, attendu 400", status)
	}

	// Champ absent → défaut hotspot.
	accID, ownerToken := createAdminAccount(t, ts, adminToken, "compte-sans-usage", "")
	if status, _ := doJSON(t, ts, "GET", "/api/vouchers", ownerToken, nil); status != http.StatusOK {
		t.Fatalf("compte sans usage : GET /api/vouchers statut %d, attendu 200 (défaut hotspot)", status)
	}
	_ = accID
}

// TestUsageGuardBlocksHotspotProductForHomeNet — LE cœur de la Phase 1 : un
// compte homenet reçoit 404 sur les endpoints produit hotspot et GARDE les
// endpoints partagés — avec le MÊME token, sans re-login.
func TestUsageGuardBlocksHotspotProductForHomeNet(t *testing.T) {
	ts := newTestServer(t)
	adminToken := adminLogin(t, ts)
	_, hnToken := createAdminAccount(t, ts, adminToken, "gerant-maison", model.AccountUsageHomeNet)

	blocked := []struct {
		label string
		path  string
	}{
		{"vouchers", "/api/vouchers"},
		{"profils", "/api/profiles"},
		{"utilisateurs hotspot", "/api/users"},
		{"revendeurs", "/api/resellers"},
		{"modèles", "/api/templates"},
		{"transactions", "/api/transactions"},
		{"rapports", "/api/reports"},
		{"WiFi jetable (sites)", "/api/wifi/sites"},
		{"inscriptions publiques", "/api/registrations"},
		{"journaux utilisateurs", "/api/user-logs"},
	}
	for _, tc := range blocked {
		if status, _ := doJSON(t, ts, "GET", tc.path, hnToken, nil); status != http.StatusNotFound {
			t.Fatalf("%s : statut %d, attendu 404 pour un compte homenet", tc.label, status)
		}
	}

	open := []struct {
		label string
		path  string
	}{
		{"dashboard (partagé)", "/api/dashboard"},
		{"routeurs (partagé)", "/api/routers"},
		{"sessions (partagé)", "/api/sessions"},
		{"réglages (partagé)", "/api/settings"},
		{"me (partagé)", "/api/auth/me"},
	}
	for _, tc := range open {
		if status, _ := doJSON(t, ts, "GET", tc.path, hnToken, nil); status != http.StatusOK {
			t.Fatalf("%s : statut %d, attendu 200 (familles partagées)", tc.label, status)
		}
	}

	// Contrôle symétrique : un compte HOTSPOT garde ses endpoints produit.
	_, hsToken := createAdminAccount(t, ts, adminToken, "gerant-cyber", model.AccountUsageHotspot)
	if status, _ := doJSON(t, ts, "GET", "/api/vouchers", hsToken, nil); status != http.StatusOK {
		t.Fatalf("compte hotspot : GET /api/vouchers statut %d, attendu 200", status)
	}
}

// TestUsageGuardExemptsPlatformAdmin — la session support (impersonation)
// passe la garde : l'opérateur du SaaS debug un compte homenet SANS être
// coupé de la console produit (même choix que guardAccountWrite : le garde
// sépare les CLIENTS, pas l'opérateur).
func TestUsageGuardExemptsPlatformAdmin(t *testing.T) {
	ts := newTestServer(t)
	adminToken := adminLogin(t, ts)
	hnAccID, _ := createAdminAccount(t, ts, adminToken, "maison-support", model.AccountUsageHomeNet)

	status, out := doJSON(t, ts, "POST", "/api/admin/accounts/"+hnAccID+"/impersonate", adminToken, nil)
	if status != http.StatusOK {
		t.Fatalf("impersonation : statut %d, corps %v", status, out)
	}
	supportToken, _ := out["token"].(string)
	if supportToken == "" {
		t.Fatal("impersonation : token manquant")
	}
	if status, _ := doJSON(t, ts, "GET", "/api/vouchers", supportToken, nil); status != http.StatusOK {
		t.Fatalf("session support sur compte homenet : GET /api/vouchers statut %d, attendu 200 (exemption plateforme)", status)
	}
}

// doJSONList — variante de doJSON pour les réponses TABLEAU JSON (la liste
// des comptes renvoie []accountRow, pas un objet).
func doJSONList(t *testing.T, ts *httptest.Server, method, path, token string) (int, []map[string]any) {
	t.Helper()
	req, err := http.NewRequest(method, ts.URL+path, nil)
	if err != nil {
		t.Fatalf("requête impossible : %v", err)
	}
	req.Header.Set("Accept-Encoding", "identity")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("%s %s : %v", method, path, err)
	}
	defer resp.Body.Close()
	var rows []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
		t.Fatalf("réponse non décodable en tableau : %v", err)
	}
	return resp.StatusCode, rows
}

// TestAdminAccountUsageFlip — PUT /api/admin/accounts/{id}/usage : bascule
// valide (effet immédiat, MÊME token), idempotence, valeurs inconnues,
// compte inconnu, compte principal protégé, non-admin refusé.
func TestAdminAccountUsageFlip(t *testing.T) {
	ts := newTestServer(t)
	adminToken := adminLogin(t, ts)
	accID, hnToken := createAdminAccount(t, ts, adminToken, "maison-bascule", model.AccountUsageHomeNet)

	// Valeur inconnue → 400.
	if status, _ := doJSON(t, ts, "PUT", "/api/admin/accounts/"+accID+"/usage", adminToken, map[string]string{"usage": "cybercafe"}); status != http.StatusBadRequest {
		t.Fatalf("usage inconnu : statut %d, attendu 400", status)
	}
	// Compte inconnu → 404.
	if status, _ := doJSON(t, ts, "PUT", "/api/admin/accounts/acc-fantome/usage", adminToken, map[string]string{"usage": model.AccountUsageHotspot}); status != http.StatusNotFound {
		t.Fatalf("compte inconnu : statut %d, attendu 404", status)
	}
	// Compte principal → 400 (données de l'ère mono-tenant = hotspot).
	if status, _ := doJSON(t, ts, "PUT", "/api/admin/accounts/"+model.AccountMainID+"/usage", adminToken, map[string]string{"usage": model.AccountUsageHomeNet}); status != http.StatusBadRequest {
		t.Fatalf("compte principal : statut %d, attendu 400", status)
	}
	// Non-admin (owner du compte) → 403.
	if status, _ := doJSON(t, ts, "PUT", "/api/admin/accounts/"+accID+"/usage", hnToken, map[string]string{"usage": model.AccountUsageHotspot}); status != http.StatusForbidden {
		t.Fatalf("bascule par un client : statut %d, attendu 403", status)
	}

	// Bascule homenet → hotspot : effet IMMÉDIAT pour le token DÉJÀ émis
	// (la garde relit l'usage à chaque requête, pas au login).
	if status, out := doJSON(t, ts, "PUT", "/api/admin/accounts/"+accID+"/usage", adminToken, map[string]string{"usage": model.AccountUsageHotspot}); status != http.StatusOK {
		t.Fatalf("bascule vers hotspot : statut %d, corps %v", status, out)
	}
	if status, _ := doJSON(t, ts, "GET", "/api/vouchers", hnToken, nil); status != http.StatusOK {
		t.Fatalf("après bascule hotspot (même token) : GET /api/vouchers statut %d, attendu 200", status)
	}

	// Idempotence : re-PUT la même valeur → 200.
	if status, _ := doJSON(t, ts, "PUT", "/api/admin/accounts/"+accID+"/usage", adminToken, map[string]string{"usage": model.AccountUsageHotspot}); status != http.StatusOK {
		t.Fatalf("PUT idempotent : statut %d, attendu 200", status)
	}

	// Retour vers homenet → re-blocage immédiat.
	if status, _ := doJSON(t, ts, "PUT", "/api/admin/accounts/"+accID+"/usage", adminToken, map[string]string{"usage": model.AccountUsageHomeNet}); status != http.StatusOK {
		t.Fatalf("bascule vers homenet : statut %d, attendu 200", status)
	}
	if status, _ := doJSON(t, ts, "GET", "/api/vouchers", hnToken, nil); status != http.StatusNotFound {
		t.Fatalf("après retour homenet (même token) : GET /api/vouchers statut %d, attendu 404", status)
	}

	// La liste console plateforme porte la colonne usage.
	status, rows := doJSONList(t, ts, "GET", "/api/admin/accounts", adminToken)
	if status != http.StatusOK {
		t.Fatalf("liste des comptes : statut %d", status)
	}
	found := false
	for _, row := range rows {
		if row["id"] == accID {
			found = true
			if row["usage"] != model.AccountUsageHomeNet {
				t.Fatalf("liste des comptes : usage = %v, attendu %q", row["usage"], model.AccountUsageHomeNet)
			}
		}
	}
	if !found {
		t.Fatalf("liste des comptes : ligne du compte %s introuvable", accID)
	}
}

// TestAccountUsageNormalization — un compte de l'ère pré-colonne (usage
// vide) est lu comme HOTSPOT : (1) repli défensif de la garde sur un état
// injecté SANS rechargement ; (2) normalisation du store au Reload
// (migrateMultiTenant — la valeur est persistée au Save suivant).
func TestAccountUsageNormalization(t *testing.T) {
	st, ts := newTestServerWithStore(t)

	// 1) Repli défensif de la garde : compte usage vide injecté APRÈS le
	// boot (jamais passé par migrateMultiTenant).
	st.Lock()
	st.Data().Accounts = append(st.Data().Accounts, model.Account{
		ID: "acc-vide", Name: "Pré-colonne", Status: "active",
		CreatedAt: model.NowISO(), Usage: "",
	})
	st.Unlock()
	seedUser(t, st, "usr-vide", "acc-vide", "vieux-compte", model.RoleOwner)
	token := auth.Sign(testJWTSecret, auth.NewClaims("usr-vide", "vieux-compte", model.RoleOwner, "acc-vide", 0))
	if status, _ := doJSON(t, ts, "GET", "/api/vouchers", token, nil); status != http.StatusOK {
		t.Fatalf("compte pré-colonne (usage vide) : GET /api/vouchers statut %d, attendu 200 (repli hotspot)", status)
	}

	// 2) Normalisation du store : compte usage vide PERSISTÉ, puis Reload
	// → migrateMultiTenant pose « hotspot » (et le diff PG le persistrait
	// au Save suivant — ici mode JSON).
	st.Lock()
	st.Data().Accounts = append(st.Data().Accounts, model.Account{
		ID: "acc-vide2", Name: "Pré-colonne 2", Status: "active",
		CreatedAt: model.NowISO(), Usage: "",
	})
	st.Save()
	st.Unlock()
	if _, err := st.Reload(); err != nil {
		t.Fatalf("Reload impossible : %v", err)
	}
	st.Lock()
	normalized := "(introuvable)"
	for i := range st.Data().Accounts {
		if st.Data().Accounts[i].ID == "acc-vide2" {
			normalized = st.Data().Accounts[i].Usage
			break
		}
	}
	st.Unlock()
	if normalized != model.AccountUsageHotspot {
		t.Fatalf("normalisation store : usage = %q, attendu %q", normalized, model.AccountUsageHotspot)
	}
}

// TestUsageGuardHelpers — unitaires purs des helpers : normalizeAccountUsage
// et accountUsageLocked (compte inconnu → hotspot défensif).
func TestUsageGuardHelpers(t *testing.T) {
	if normalizeAccountUsage("") != model.AccountUsageHotspot {
		t.Fatal("usage vide → hotspot attendu")
	}
	if normalizeAccountUsage("nimporte-quoi") != model.AccountUsageHotspot {
		t.Fatal("usage inconnu → hotspot attendu")
	}
	if normalizeAccountUsage(model.AccountUsageHomeNet) != model.AccountUsageHomeNet {
		t.Fatal("usage homenet → homenet attendu")
	}
	db := &model.DB{
		Accounts: []model.Account{{ID: "acc-a", Usage: model.AccountUsageHomeNet}},
	}
	if got := accountUsageLocked(db, "acc-a"); got != model.AccountUsageHomeNet {
		t.Fatalf("accountUsageLocked(acc-a) = %q, attendu homenet", got)
	}
	if got := accountUsageLocked(db, "acc-inconnu"); got != model.AccountUsageHotspot {
		t.Fatalf("accountUsageLocked(inconnu) = %q, attendu hotspot (défensif)", got)
	}
}
