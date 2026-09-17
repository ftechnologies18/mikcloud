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
	// La sig est posée à la valeur courante (fichiers + branding — N°135)
	// → rien à déployer.
	router.HotspotFilesSig = hotspotFilesSig(hotpage.DefaultFiles(), db, router)

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

// TestEnsureHotspotFilesLockedBrandingChange — N°135 : un changement de
// branding du compte (logo posé en console) change la sig → la commande
// hotspot_files est re-filée au check-in suivant. Garde-fou du bug
// d'origine : la sig v1 ne couvrait que les fichiers du template — un logo
// posé en console n'atteignait JAMAIS le portail déployé sans un
// « Re-déployer » manuel (et le fetch live ne le rattrapait que pour les
// routeurs liés à un site WiFi actif).
func TestEnsureHotspotFilesLockedBrandingChange(t *testing.T) {
	st, _ := newTestServerWithStore(t)
	st.Lock()
	db := st.Data()
	db.Accounts = append(db.Accounts, model.Account{ID: "acc-brand", Name: "Cyber Brand"})
	db.Routers = append(db.Routers, model.Router{
		ID:             "r-brand",
		AccountID:      "acc-brand",
		Name:           "BrandRouter",
		Mode:           "agent",
		Status:         "online",
		AgentTokenHash: agent.HashToken("tok-brand"),
	})
	router := &db.Routers[len(db.Routers)-1]
	// Portail « déployé » : sig posée à la valeur courante (fichiers + branding).
	router.HotspotFilesSig = hotspotFilesSig(hotpage.DefaultFiles(), db, router)
	ensureHotspotFilesLocked(db, router)
	if n := countHotspotCmds(db, router.ID); n != 0 {
		st.Unlock()
		t.Fatalf("sig à jour : 0 commande attendue, %d trouvées", n)
	}
	// Le gérant pose le logo de SON établissement en console (carte Vouchers).
	s := db.SettingsByAccount["acc-brand"]
	s.Tenant.LogoURL = "data:image/png;base64,LOGOPROMAX"
	db.SettingsByAccount["acc-brand"] = s
	ensureHotspotFilesLocked(db, router)
	n := countHotspotCmds(db, router.ID)
	st.Unlock()
	if n != 1 {
		t.Fatalf("changement de logo : 1 commande de re-déploiement attendue, %d trouvées", n)
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

	// GET /portal/tok-portal-ok/img/pub1.jpg → 200 + image/jpeg (N°135 —
	// img/logo.png, le logo du site pilote, n'est plus dans le template).
	resp3, err := http.Get(ts.URL + "/portal/tok-portal-ok/img/pub1.jpg")
	if err != nil {
		t.Fatalf("GET pub1.jpg : %v", err)
	}
	defer resp3.Body.Close()
	if resp3.StatusCode != http.StatusOK {
		t.Fatalf("GET pub1.jpg : statut %d", resp3.StatusCode)
	}
	if ct := resp3.Header.Get("Content-Type"); !strings.HasPrefix(ct, "image/jpeg") {
		t.Errorf("Content-Type pub1.jpg = %q, attendu image/jpeg*", ct)
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
