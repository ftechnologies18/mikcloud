// sell_devices_test.go — N°66 : limite d'appareils simultanés par compte
// revendeur (Mode Vente).
//
// Couverture :
//   - création console avec limite (maxDevices) + exposition dans la réponse
//     sanitisée et le compteur activeDevices de la liste ;
//   - éviction FIFO : au-delà de la limite, l'appareil connecté depuis le plus
//     longtemps reçoit 401 sur /api/sell/* (fin de session PWA), les suivants
//     restent valides ; le registre reste borné à la limite ;
//   - baisse de limite (PUT) : trim immédiat — les surnuméraires sont évincés
//     sans attendre un nouveau login ;
//   - activation de la limite (0 → N) : un token historique SANS jti est
//     révoqué (401 « Session réinitialisée ») — le login suivant régularise
//     l'appareil ;
//   - illimité (0, défaut) : comportement stateless historique intact —
//     aucun enregistrement de session, tokens multiples simultanés valides ;
//   - validation : maxDevices négatif ou > 20 → 400.
//
// AUCUNE connexion réseau : store JSON éphémère + httptest.Server (surface
// publique réelle), helpers partagés du package.
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"mikcloud/hotspot-api/internal/auth"
	"mikcloud/hotspot-api/internal/model"
)

// sellLogin — login PIN Mode Vente (POST /api/reseller/login) → token.
func sellLogin(t *testing.T, ts *httptest.Server, username, pin string) string {
	t.Helper()
	status, out := doJSON(t, ts, "POST", "/api/reseller/login", "", map[string]string{
		"username": username, "pin": pin,
	})
	if status != http.StatusOK {
		t.Fatalf("login %s : statut %d (%v)", username, status, out)
	}
	token, _ := out["token"].(string)
	if token == "" {
		t.Fatalf("login %s : token manquant (%v)", username, out)
	}
	return token
}

// sellStockStatus — statut HTTP de GET /api/sell/stock pour un token donné.
func sellStockStatus(t *testing.T, ts *httptest.Server, token string) int {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/sell/stock", nil)
	if err != nil {
		t.Fatalf("requête impossible : %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("stock : %v", err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

// resellersList — GET /api/resellers (réponse = tableau JSON).
func resellersList(t *testing.T, ts *httptest.Server, token string) []map[string]any {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/resellers", nil)
	if err != nil {
		t.Fatalf("requête impossible : %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("liste revendeurs : %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("liste revendeurs : statut %d", resp.StatusCode)
	}
	var list []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("décodage liste revendeurs : %v", err)
	}
	return list
}

// countLiveSellSessions — sessions du registre pour un revendeur.
func countLiveSellSessions(st interface {
	Lock()
	Unlock()
	Data() *model.DB
}, resellerID string) int {
	st.Lock()
	defer st.Unlock()
	n := 0
	for _, s := range st.Data().SellSessions {
		if s.ResellerID == resellerID {
			n++
		}
	}
	return n
}

// TestSellDeviceLimitEvictsOldest — limite 2 : le 3e login déconnecte le 1er
// appareil (401), la baisse à 1 déconnecte ensuite le 2e — trim immédiat.
func TestSellDeviceLimitEvictsOldest(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	token, _, _ := registerAccount(t, ts, "gerant-limite", "")

	status, out := doJSON(t, ts, "POST", "/api/resellers", token, map[string]any{
		"name": "Limite", "username": "limite", "pin": "1234", "maxDevices": 2,
	})
	if status != http.StatusOK {
		t.Fatalf("création revendeur : statut %d (%v)", status, out)
	}
	if got, _ := out["maxDevices"].(float64); int(got) != 2 {
		t.Fatalf("maxDevices attendu 2, obtenu %v", out["maxDevices"])
	}
	resID, _ := out["id"].(string)

	// Trois logins : le registre ne garde que les 2 plus récents.
	t1 := sellLogin(t, ts, "limite", "1234")
	t2 := sellLogin(t, ts, "limite", "1234")
	t3 := sellLogin(t, ts, "limite", "1234")

	// Le 1er appareil est évincé (401) ; les deux suivants vivent.
	if got := sellStockStatus(t, ts, t1); got != http.StatusUnauthorized {
		t.Fatalf("appareil 1 évincé : statut %d attendu 401", got)
	}
	for name, tok := range map[string]string{"appareil 2": t2, "appareil 3": t3} {
		if got := sellStockStatus(t, ts, tok); got != http.StatusOK {
			t.Fatalf("%s : statut %d attendu 200", name, got)
		}
	}
	if got := countLiveSellSessions(st, resID); got != 2 {
		t.Fatalf("registre : 2 sessions attendues, obtenu %d", got)
	}

	// Console : le compteur activeDevices reflète les appareils connectés.
	active := -1
	for _, r := range resellersList(t, ts, token) {
		if id, _ := r["id"].(string); id == resID {
			active = int(r["activeDevices"].(float64))
		}
	}
	if active != 2 {
		t.Fatalf("activeDevices attendu 2, obtenu %d", active)
	}

	// Baisse de limite 2 → 1 : trim immédiat — l'appareil 3 (plus récent)
	// survit, l'appareil 2 est évincé.
	status, out = doJSON(t, ts, "PUT", "/api/resellers/"+resID, token, map[string]any{"maxDevices": 1})
	if status != http.StatusOK {
		t.Fatalf("baisse de limite : statut %d (%v)", status, out)
	}
	if got, _ := out["maxDevices"].(float64); int(got) != 1 {
		t.Fatalf("maxDevices après baisse attendu 1, obtenu %v", out["maxDevices"])
	}
	if got := sellStockStatus(t, ts, t2); got != http.StatusUnauthorized {
		t.Fatalf("appareil 2 après baisse : statut %d attendu 401", got)
	}
	if got := sellStockStatus(t, ts, t3); got != http.StatusOK {
		t.Fatalf("appareil 3 après baisse : statut %d attendu 200", got)
	}
}

// TestSellDeviceActivationRevokesLegacyTokens — l'activation de la limite
// (0 → 1) révoque les tokens en vol sans jti (401 « Session réinitialisée ») ;
// le login suivant régularise l'appareil dans le registre.
func TestSellDeviceActivationRevokesLegacyTokens(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	token, accID, _ := registerAccount(t, ts, "gerant-activation", "")

	status, out := doJSON(t, ts, "POST", "/api/resellers", token, map[string]any{
		"name": "Illimité", "username": "free", "pin": "4321",
	})
	if status != http.StatusOK {
		t.Fatalf("création revendeur : statut %d (%v)", status, out)
	}
	resID, _ := out["id"].(string)

	// Token stateless historique (aucune limite → aucun registre).
	t0 := sellLogin(t, ts, "free", "4321")
	if got := sellStockStatus(t, ts, t0); got != http.StatusOK {
		t.Fatalf("illimité : statut %d attendu 200", got)
	}
	if got := countLiveSellSessions(st, resID); got != 0 {
		t.Fatalf("illimité : registre vide attendu, obtenu %d", got)
	}

	// Activation de la limite : le token sans jti est révoqué…
	status, out = doJSON(t, ts, "PUT", "/api/resellers/"+resID, token, map[string]any{"maxDevices": 1})
	if status != http.StatusOK {
		t.Fatalf("activation limite : statut %d (%v)", status, out)
	}
	if got := sellStockStatus(t, ts, t0); got != http.StatusUnauthorized {
		t.Fatalf("token sans jti après activation : statut %d attendu 401", got)
	}

	// …et le login suivant régularise l'appareil.
	t1 := sellLogin(t, ts, "free", "4321")
	if got := sellStockStatus(t, ts, t1); got != http.StatusOK {
		t.Fatalf("login après activation : statut %d attendu 200", got)
	}
	if got := countLiveSellSessions(st, resID); got != 1 {
		t.Fatalf("registre après régularisation : 1 session attendue, obtenu %d", got)
	}

	// Le token FORGÉ sans jti (même PIN, pré-N°66) reste refusé.
	forged := auth.Sign(testJWTSecret, auth.NewClaims(resID, "Illimité", "reseller", accID, 0))
	if got := sellStockStatus(t, ts, forged); got != http.StatusUnauthorized {
		t.Fatalf("token forgé sans jti : statut %d attendu 401", got)
	}
}

// TestSellDeviceUnlimitedStateless — maxDevices 0 (défaut) : plusieurs
// appareils simultanés, registre vide, comportement historique intact.
func TestSellDeviceUnlimitedStateless(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	token, _, _ := registerAccount(t, ts, "gerant-libre", "")

	status, out := doJSON(t, ts, "POST", "/api/resellers", token, map[string]any{
		"name": "Libre", "username": "libre", "pin": "5678",
	})
	if status != http.StatusOK {
		t.Fatalf("création revendeur : statut %d (%v)", status, out)
	}
	resID, _ := out["id"].(string)

	tokens := make([]string, 0, 3)
	for range 3 {
		tokens = append(tokens, sellLogin(t, ts, "libre", "5678"))
	}
	for i, tok := range tokens {
		if got := sellStockStatus(t, ts, tok); got != http.StatusOK {
			t.Fatalf("appareil %d illimité : statut %d attendu 200", i+1, got)
		}
	}
	if got := countLiveSellSessions(st, resID); got != 0 {
		t.Fatalf("illimité : registre vide attendu, obtenu %d", got)
	}
}

// TestSellDeviceLimitValidation — bornes de la limite : négatif et > 20 → 400.
func TestSellDeviceLimitValidation(t *testing.T) {
	_, ts := newTestServerWithStore(t)
	token, _, _ := registerAccount(t, ts, "gerant-bornes", "")

	for _, bad := range []int{-1, 21} {
		status, out := doJSON(t, ts, "POST", "/api/resellers", token, map[string]any{
			"name": "Bornes", "username": "bornes", "pin": "9999", "maxDevices": bad,
		})
		if status != http.StatusBadRequest {
			t.Fatalf("maxDevices %d : statut %d attendu 400 (%v)", bad, status, out)
		}
	}
	// Même garde en édition.
	status, out := doJSON(t, ts, "POST", "/api/resellers", token, map[string]any{
		"name": "Bornes", "username": "bornes", "pin": "9999", "maxDevices": 3,
	})
	if status != http.StatusOK {
		t.Fatalf("création revendeur : statut %d (%v)", status, out)
	}
	resID, _ := out["id"].(string)
	status, out = doJSON(t, ts, "PUT", "/api/resellers/"+resID, token, map[string]any{"maxDevices": 99})
	if status != http.StatusBadRequest {
		t.Fatalf("baisse maxDevices 99 : statut %d attendu 400 (%v)", status, out)
	}
}
