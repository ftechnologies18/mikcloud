// Package api — tests des endpoints console /api/routers/{id}/redeploy-portal
// et /api/routers/{id}/portal-preview (N°35-d).
package api

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"mikcloud/hotspot-api/internal/model"
)

// seedRouterInAccount — ajoute un routeur agent au compte fourni, avec une
// sig déjà posée (simule un déploiement déjà effectué). Retourne le routerID.
func seedRouterInAccount(t *testing.T, st interface {
	Lock()
	Unlock()
	Save()
	Data() *model.DB
}, accID, routerName string) (routerID string) {
	t.Helper()
	st.Lock()
	defer st.Unlock()
	db := st.Data()
	routerID = "r-portal-" + routerName
	db.Routers = append(db.Routers, model.Router{
		ID:              routerID,
		AccountID:       accID,
		Name:            routerName,
		Mode:            "agent",
		Status:          "online",
		HotspotFilesSig: "abcd1234abcd1234", // simule un déploiement déjà effectué
	})
	st.Save()
	return routerID
}

// TestRedeployPortalOK — POST /api/routers/{id}/redeploy-portal vide la sig
// et trace l'activité. Réponse 200 avec ok:true.
func TestRedeployPortalOK(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	ownerToken, accID, _ := registerAccount(t, ts, "gerant-redeploy-ok", "")
	routerID := seedRouterInAccount(t, st, accID, "Router OK")

	// Vérifie la sig avant.
	st.Lock()
	before := findRouterScoped(st.Data(), routerID, accID)
	sigBefore := ""
	if before != nil {
		sigBefore = before.HotspotFilesSig
	}
	st.Unlock()
	if sigBefore == "" {
		t.Fatal("sig avant devrait être non vide (seed)")
	}

	// Requête POST.
	req, _ := http.NewRequest("POST", ts.URL+"/api/routers/"+routerID+"/redeploy-portal", nil)
	req.Header.Set("Authorization", "Bearer "+ownerToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST : %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("statut %d, attendu 200, body : %s", resp.StatusCode, body)
	}

	// Vérifie la sig après : doit être vide.
	st.Lock()
	after := findRouterScoped(st.Data(), routerID, accID)
	sigAfter := ""
	if after != nil {
		sigAfter = after.HotspotFilesSig
	}
	st.Unlock()
	if sigAfter != "" {
		t.Errorf("sig après re-déploiement = %q, attendu vide (force le re-file)", sigAfter)
	}
}

// TestRedeployPortalNotFound — routeur inexistant → 404.
func TestRedeployPortalNotFound(t *testing.T) {
	_, ts := newTestServerWithStore(t)
	ownerToken, _, _ := registerAccount(t, ts, "gerant-redeploy-404", "")

	req, _ := http.NewRequest("POST", ts.URL+"/api/routers/r-inexistant/redeploy-portal", nil)
	req.Header.Set("Authorization", "Bearer "+ownerToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST : %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("statut %d, attendu 404", resp.StatusCode)
	}
}

// TestRedeployPortalNotAgent — routeur en mode simulated → 400 code not_agent.
func TestRedeployPortalNotAgent(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	ownerToken, accID, _ := registerAccount(t, ts, "gerant-redeploy-sim", "")
	st.Lock()
	st.Data().Routers = append(st.Data().Routers, model.Router{
		ID: "r-sim-test", AccountID: accID, Name: "SimRouter",
		Mode: "simulated", Status: "online",
	})
	st.Save()
	st.Unlock()

	req, _ := http.NewRequest("POST", ts.URL+"/api/routers/r-sim-test/redeploy-portal", nil)
	req.Header.Set("Authorization", "Bearer "+ownerToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST : %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("statut %d, attendu 400", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "not_agent") {
		t.Errorf("code not_agent absent : %s", body)
	}
}

// TestPortalPreviewOK — GET /api/routers/{id}/portal-preview renvoie le HTML
// personnalisé de login.html avec Content-Type text/html.
func TestPortalPreviewOK(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	ownerToken, accID, _ := registerAccount(t, ts, "gerant-preview-ok", "")
	routerID := seedRouterInAccount(t, st, accID, "Router Preview")

	req, _ := http.NewRequest("GET", ts.URL+"/api/routers/"+routerID+"/portal-preview", nil)
	req.Header.Set("Authorization", "Bearer "+ownerToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET : %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("statut %d, attendu 200, body : %s", resp.StatusCode, body)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, attendu text/html*", ct)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, attendu no-store", cc)
	}
	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)
	// Le HTML doit contenir le bloc <script id="mikcloud-config">.
	if !strings.Contains(bodyStr, `id="mikcloud-config"`) {
		t.Error("bloc config absent de l'aperçu")
	}
	// Le HTML doit contenir le tenant name « Gérant gerant-preview-ok » (config
	// inlinée — ensureSettings prend le nom du compte).
	if !strings.Contains(bodyStr, "Gérant gerant-preview-ok") {
		t.Errorf("tenant name absent de l'aperçu")
	}
	// Le marqueur {{MIKCLOUD_CONFIG_JSON}} ne doit PLUS être présent (substitué).
	if strings.Contains(bodyStr, "{{MIKCLOUD_CONFIG_JSON}}") {
		t.Error("marqueur non substitué dans l'aperçu")
	}
}

// TestPortalPreviewNotFound — routeur inexistant → 404.
func TestPortalPreviewNotFound(t *testing.T) {
	_, ts := newTestServerWithStore(t)
	ownerToken, _, _ := registerAccount(t, ts, "gerant-preview-404", "")

	req, _ := http.NewRequest("GET", ts.URL+"/api/routers/r-inexistant/portal-preview", nil)
	req.Header.Set("Authorization", "Bearer "+ownerToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET : %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("statut %d, attendu 404", resp.StatusCode)
	}
}

// TestPortalPreviewNotAgent — routeur en mode real → 400 code not_agent.
func TestPortalPreviewNotAgent(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	ownerToken, accID, _ := registerAccount(t, ts, "gerant-preview-real", "")
	st.Lock()
	st.Data().Routers = append(st.Data().Routers, model.Router{
		ID: "r-real-test", AccountID: accID, Name: "RealRouter",
		Mode: "real", Status: "online",
	})
	st.Save()
	st.Unlock()

	req, _ := http.NewRequest("GET", ts.URL+"/api/routers/r-real-test/portal-preview", nil)
	req.Header.Set("Authorization", "Bearer "+ownerToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET : %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("statut %d, attendu 400", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "not_agent") {
		t.Errorf("code not_agent absent : %s", body)
	}
}

// TestPortalPreviewCSP — l'aperçu porte une CSP avec frame-ancestors 'self'.
func TestPortalPreviewCSP(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	ownerToken, accID, _ := registerAccount(t, ts, "gerant-preview-csp", "")
	routerID := seedRouterInAccount(t, st, accID, "Router CSP")

	req, _ := http.NewRequest("GET", ts.URL+"/api/routers/"+routerID+"/portal-preview", nil)
	req.Header.Set("Authorization", "Bearer "+ownerToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET : %v", err)
	}
	defer resp.Body.Close()
	csp := resp.Header.Get("Content-Security-Policy")
	if !strings.Contains(csp, "frame-ancestors 'self'") {
		t.Errorf("CSP sans frame-ancestors 'self' : %q", csp)
	}
}
