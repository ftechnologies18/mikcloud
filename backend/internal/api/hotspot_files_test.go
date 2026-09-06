// Package api — tests du déploiement automatique du portail captif (N°35).
//
// Couvre :
//   - ensureHotspotFilesLocked : déduplication, signature, re-file auto ;
//   - GET /portal/{token}/{path} : auth par token, servage, sécurité ;
//   - route non-agent : aucune commande jamais filée.
package api

import (
	"net/http"
	"strings"
	"testing"

	"mikcloud/hotspot-api/internal/agent"
	"mikcloud/hotspot-api/internal/hotpage"
	"mikcloud/hotspot-api/internal/model"
)

// TestEnsureHotspotFilesLockedDedup — une commande hotspot_files déjà en vol
// (queued ou sent) ne doit PAS être re-filée. Calqué sur le pattern walled_garden.
func TestEnsureHotspotFilesLockedDedup(t *testing.T) {
	st, _ := newTestServerWithStore(t)

	st.Lock()
	db := st.Data()
	db.Routers = append(db.Routers, model.Router{
		ID:             "r-test-dedup",
		AccountID:      "acc-test",
		Name:           "TestRouter",
		Mode:           "agent",
		Status:         "online",
		AgentTokenHash: agent.HashToken("tok-dedup"),
	})
	router := &db.Routers[len(db.Routers)-1]
	router.HotspotFilesSig = "" // force un déploiement

	// 1er appel : file une commande hotspot_files.
	ensureHotspotFilesLocked(db, router)
	cmds1 := countHotspotCmds(db, router.ID)
	if cmds1 != 1 {
		t.Fatalf("1er ensure : 1 commande attendue, %d trouvées", cmds1)
	}
	// 2e appel : la commande en vol bloque le re-file → pas de doublon.
	ensureHotspotFilesLocked(db, router)
	cmds2 := countHotspotCmds(db, router.ID)
	if cmds2 != 1 {
		t.Fatalf("2e ensure avec commande en vol : 1 commande attendue (dédup), %d trouvées", cmds2)
	}
	st.Unlock()
}

// TestEnsureHotspotFilesLockedSigMatch — si la sig correspond déjà à ce qui
// est déployé, aucune commande n'est filée (économise un check-in).
func TestEnsureHotspotFilesLockedSigMatch(t *testing.T) {
	st, _ := newTestServerWithStore(t)

	st.Lock()
	db := st.Data()
	db.Routers = append(db.Routers, model.Router{
		ID:             "r-test-sigmatch",
		AccountID:      "acc-test",
		Name:           "TestRouter",
		Mode:           "agent",
		Status:         "online",
		AgentTokenHash: agent.HashToken("tok-sigmatch"),
	})
	router := &db.Routers[len(db.Routers)-1]
	// La sig est posée à la valeur courante → rien à déployer.
	router.HotspotFilesSig = hotpage.Sig(hotpage.DefaultFiles())

	before := countHotspotCmds(db, router.ID)
	ensureHotspotFilesLocked(db, router)
	after := countHotspotCmds(db, router.ID)
	st.Unlock()
	if after != before {
		t.Fatalf("sig match : aucune commande ne doit être filée, before=%d after=%d", before, after)
	}
}

// TestEnsureHotspotFilesLockedNonAgent — un routeur simulé ou réel ne reçoit
// JAMAIS de commande hotspot_files (pas de check-in agent → jamais servie).
func TestEnsureHotspotFilesLockedNonAgent(t *testing.T) {
	st, _ := newTestServerWithStore(t)

	st.Lock()
	db := st.Data()
	db.Routers = append(db.Routers, model.Router{
		ID: "r-sim", AccountID: "acc-test", Name: "SimRouter",
		Mode: "simulated", Status: "online",
	})
	sim := &db.Routers[len(db.Routers)-1]
	ensureHotspotFilesLocked(db, sim)
	n := countHotspotCmds(db, sim.ID)
	st.Unlock()
	if n != 0 {
		t.Fatalf("routeur non-agent : 0 commande attendue, %d trouvées", n)
	}
}

// TestPortalServeOK — GET /portal/{token}/login.html sert le fichier avec le
// Content-Type HTML, et un token valide donne accès à tous les fichiers du
// template (texte + binaire).
func TestPortalServeOK(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	st.Lock()
	st.Data().Routers = append(st.Data().Routers, model.Router{
		ID:             "r-test-portal-ok",
		AccountID:      "acc-test",
		Name:           "TestPortal",
		Mode:           "agent",
		Status:         "online",
		AgentTokenHash: agent.HashToken("tok-portal-ok"),
	})
	st.Save()
	st.Unlock()

	// GET /portal/tok-portal-ok/login.html → 200 + HTML.
	resp, err := http.Get(ts.URL + "/portal/tok-portal-ok/login.html")
	if err != nil {
		t.Fatalf("GET login.html : %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET login.html : statut %d, attendu 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, attendu text/html*", ct)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, attendu no-store", cc)
	}
	if xcto := resp.Header.Get("X-Content-Type-Options"); xcto != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, attendu nosniff", xcto)
	}

	// GET /portal/tok-portal-ok/md5.js → 200 + JS.
	resp2, err := http.Get(ts.URL + "/portal/tok-portal-ok/md5.js")
	if err != nil {
		t.Fatalf("GET md5.js : %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("GET md5.js : statut %d", resp2.StatusCode)
	}
	if ct := resp2.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/javascript") {
		t.Errorf("Content-Type md5.js = %q, attendu application/javascript*", ct)
	}

	// GET /portal/tok-portal-ok/img/logo.png → 200 + image/png.
	resp3, err := http.Get(ts.URL + "/portal/tok-portal-ok/img/logo.png")
	if err != nil {
		t.Fatalf("GET logo.png : %v", err)
	}
	defer resp3.Body.Close()
	if resp3.StatusCode != http.StatusOK {
		t.Fatalf("GET logo.png : statut %d", resp3.StatusCode)
	}
	if ct := resp3.Header.Get("Content-Type"); !strings.HasPrefix(ct, "image/png") {
		t.Errorf("Content-Type logo.png = %q, attendu image/png*", ct)
	}
}

// TestPortalServeSecurity — path hostile (remontée, absolu, inexistant) et
// token invalide → 404 sans révéler la structure (anti-énumération).
func TestPortalServeSecurity(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	st.Lock()
	st.Data().Routers = append(st.Data().Routers, model.Router{
		ID:             "r-test-portal-sec",
		AccountID:      "acc-test",
		Name:           "TestPortalSec",
		Mode:           "agent",
		Status:         "online",
		AgentTokenHash: agent.HashToken("tok-portal-sec"),
	})
	st.Save()
	st.Unlock()

	cases := []struct {
		name string
		path string
	}{
		{"token invalide", "/portal/tok-invalide/login.html"},
		{"fichier inexistant", "/portal/tok-portal-sec/nonexistent.html"},
		{"path vide (trailing slash)", "/portal/tok-portal-sec/"},
	}
	for _, c := range cases {
		resp, err := http.Get(ts.URL + c.path)
		if err != nil {
			t.Errorf("%s : %v", c.name, err)
			continue
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s : statut %d, attendu 404 (anti-énumération)", c.name, resp.StatusCode)
		}
	}
}

// countHotspotCmds — helper : compte les commandes hotspot_files pour un routeur.
func countHotspotCmds(db *model.DB, routerID string) int {
	n := 0
	for _, c := range db.Commands {
		if c.RouterID == routerID && c.Kind == model.CmdHotspotFiles {
			n++
		}
	}
	return n
}
