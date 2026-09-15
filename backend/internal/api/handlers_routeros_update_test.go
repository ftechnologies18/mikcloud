package api

// Tests N°115 — mise à jour RouterOS depuis MikCloud :
//   - check simulé : réponse immédiate (disponible / à jour après update) ;
//   - update simulée : version posée, uptime à zéro, sessions coupées,
//     activité journalisée ;
//   - check agent : mise en file + DÉDUP (le second clic récupère la même
//     commande), rapport normalisé (state/status/latestVersion/…) au
//     POST /agent/result ;
//   - update agent : mise en file + dédup stricte, journal du lancement au
//     rapport ok, confirmation de version au read_state suivant (journal
//     N°115) ;
//   - payload hostile : version cible invalide refusée (400).

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"mikcloud/hotspot-api/internal/agent"
	"mikcloud/hotspot-api/internal/model"
	"mikcloud/hotspot-api/internal/store"
)

// rosTestRig — compte + routeur du mode demandé, version posée.
func rosTestRig(t *testing.T, mode, version string) (*store.Store, *httptest.Server, string, string) {
	t.Helper()
	st, ts := newTestServerWithStore(t)
	token, _, _ := registerAccount(t, ts, "ros-gerant-"+mode, "")
	status, out := doJSON(t, ts, "POST", "/api/routers", token, map[string]any{
		"name": "SITE ROS " + mode, "mode": mode, "host": "10.77.7.1",
	})
	if status != http.StatusOK {
		t.Fatalf("création routeur %s : %d %v", mode, status, out)
	}
	routerID, _ := out["id"].(string)
	if routerID == "" {
		t.Fatal("création routeur : id absent")
	}
	st.Lock()
	for i := range st.Data().Routers {
		if st.Data().Routers[i].ID == routerID {
			st.Data().Routers[i].Version = version
		}
	}
	st.Unlock()
	return st, ts, token, routerID
}

// rosAgentToken — pose un token agent connu sur le routeur et le retourne.
func rosAgentToken(t *testing.T, st *store.Store, routerID string) string {
	t.Helper()
	tok := "agt-ros-test"
	st.Lock()
	for i := range st.Data().Routers {
		if st.Data().Routers[i].ID == routerID {
			st.Data().Routers[i].Mode = "agent"
			st.Data().Routers[i].AgentTokenHash = agent.HashToken(tok)
			st.Data().Routers[i].Status = "online"
		}
	}
	st.Unlock()
	return tok
}

// rosReportResult — POST /agent/result, corps BRUT comme le routeur le fait
// réellement (http-data concaténé : espaces LITTÉRALES, pas d'encodage
// formulaire — PostForm transformerait « New version… » en « New+version… »
// que parseTolerantQuery ne décode pas).
func rosReportResult(t *testing.T, ts *httptest.Server, token, cmdID string, extra url.Values) {
	t.Helper()
	body := "token=" + token + "&cmd=" + cmdID
	for k, vs := range extra {
		for _, v := range vs {
			body += "&" + k + "=" + v
		}
	}
	resp, err := ts.Client().Post(ts.URL+"/agent/result", "application/x-www-form-urlencoded", strings.NewReader(body))
	if err != nil {
		t.Fatalf("rapport agent : %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("rapport agent : statut %d", resp.StatusCode)
	}
}

// TestRouterOSCheckSimulated — réponse immédiate et déterministe.
func TestRouterOSCheckSimulated(t *testing.T) {
	_, ts, token, routerID := rosTestRig(t, "simulated", "7.15.2")

	status, out := doJSON(t, ts, "POST", "/api/routers/"+routerID+"/routeros-check", token, nil)
	if status != http.StatusOK {
		t.Fatalf("check simulé : %d %v", status, out)
	}
	if q, _ := out["queued"].(bool); q {
		t.Fatal("check simulé : réponse immédiate attendue")
	}
	if out["state"] != routerOSStateAvailable || out["latestVersion"] != simRouterOSLatest || out["installedVersion"] != "7.15.2" {
		t.Fatalf("check simulé : %+v", out)
	}
	if out["channel"] != "stable" || out["status"] != "New version is available: "+simRouterOSLatest {
		t.Fatalf("check simulé (status/canal) : %+v", out)
	}
}

// TestRouterOSUpdateSimulated — application immédiate : version, uptime,
// sessions coupées, activité.
func TestRouterOSUpdateSimulated(t *testing.T) {
	st, ts, token, routerID := rosTestRig(t, "simulated", "7.14.3")

	status, out := doJSON(t, ts, "POST", "/api/routers/"+routerID+"/routeros-update", token, map[string]any{"latest": "7.18.1"})
	if status != http.StatusOK {
		t.Fatalf("update simulée : %d %v", status, out)
	}
	if out["version"] != "7.18.1" {
		t.Fatalf("update simulée : version attendue 7.18.1, %+v", out)
	}
	st.Lock()
	var rr model.Router
	for i := range st.Data().Routers {
		if st.Data().Routers[i].ID == routerID {
			rr = st.Data().Routers[i]
		}
	}
	st.Unlock()
	if rr.Version != "7.18.1" || rr.UptimeSec != 0 {
		t.Fatalf("update simulée : état routeur %+v", rr)
	}
	var logged bool
	st.Lock()
	for _, a := range st.Data().Activity {
		if strings.Contains(a.Message, "mis à jour : 7.14.3 → 7.18.1") {
			logged = true
		}
	}
	st.Unlock()
	if !logged {
		t.Fatal("update simulée : activité de mise à jour absente")
	}

	// Seconde mise à jour SANS cible → dernière stable simulée, puis re-check :
	// à jour désormais (la boucle démo se referme).
	status, out = doJSON(t, ts, "POST", "/api/routers/"+routerID+"/routeros-update", token, nil)
	if status != http.StatusOK || out["version"] != simRouterOSLatest {
		t.Fatalf("update sans cible : %d %+v", status, out)
	}
	status, out = doJSON(t, ts, "POST", "/api/routers/"+routerID+"/routeros-check", token, nil)
	if status != http.StatusOK || out["state"] != routerOSStateLatest {
		t.Fatalf("re-check après update : %d %+v", status, out)
	}
}

// TestRouterOSCheckAgentQueuedAndNormalized — mode agent : file + dédup +
// rapport normalisé relu par le poll du front (GET /api/commands/{id}).
func TestRouterOSCheckAgentQueuedAndNormalized(t *testing.T) {
	st, ts, token, routerID := rosTestRig(t, "agent", "7.16.2")
	agt := rosAgentToken(t, st, routerID)

	status, out := doJSON(t, ts, "POST", "/api/routers/"+routerID+"/routeros-check", token, nil)
	if status != http.StatusOK {
		t.Fatalf("check agent : %d %v", status, out)
	}
	cmdID, _ := out["commandId"].(string)
	if q, _ := out["queued"].(bool); !q || cmdID == "" {
		t.Fatalf("check agent : file attendue, %+v", out)
	}

	// Dédup : le second clic récupère LA MÊME commande (pas d'accumulation).
	status, out = doJSON(t, ts, "POST", "/api/routers/"+routerID+"/routeros-check", token, nil)
	if status != http.StatusOK || out["commandId"] != cmdID {
		t.Fatalf("dédup check : %d %+v", status, out)
	}
	st.Lock()
	n := 0
	for _, c := range st.Data().Commands {
		if c.Kind == model.CmdRouterOSCheck && c.RouterID == routerID {
			n++
		}
	}
	st.Unlock()
	if n != 1 {
		t.Fatalf("dédup check : %d commandes en jeu, 1 attendu", n)
	}

	// Rapport routeur (status RouterOS v7 réel) → commande done + résultat
	// normalisé, relu par le poll du front.
	rosReportResult(t, ts, agt, cmdID, url.Values{
		"status":    {"ok"},
		"rosStatus": {"New version is available: 7.19.4"},
		"latest":    {"7.19.4"},
		"installed": {"7.16.2"},
		"channel":   {"stable"},
	})
	status, out = doJSON(t, ts, "GET", "/api/commands/"+cmdID, token, nil)
	if status != http.StatusOK || out["status"] != "done" {
		t.Fatalf("poll commande : %d %+v", status, out)
	}
	res, _ := out["result"].(map[string]any)
	if res == nil || res["state"] != routerOSStateAvailable || res["latestVersion"] != "7.19.4" ||
		res["installedVersion"] != "7.16.2" || res["channel"] != "stable" {
		t.Fatalf("résultat normalisé : %+v", res)
	}
	if res["status"] != "New version is available: 7.19.4" {
		t.Fatalf("status brut préservé : %+v", res)
	}
}

// TestRouterOSUpdateAgentFlow — mode agent : file + dédup stricte + journal
// du lancement au rapport ok + confirmation de version au read_state
// suivant (la boucle complète N°115).
func TestRouterOSUpdateAgentFlow(t *testing.T) {
	st, ts, token, routerID := rosTestRig(t, "agent", "7.16.2")
	agt := rosAgentToken(t, st, routerID)

	status, out := doJSON(t, ts, "POST", "/api/routers/"+routerID+"/routeros-update", token, map[string]any{"latest": "7.19.4"})
	if status != http.StatusOK {
		t.Fatalf("update agent : %d %v", status, out)
	}
	cmdID, _ := out["commandId"].(string)
	if q, _ := out["queued"].(bool); !q || cmdID == "" {
		t.Fatalf("update agent : file attendue, %+v", out)
	}

	// Dédup stricte : jamais deux installations en parallèle.
	status, out = doJSON(t, ts, "POST", "/api/routers/"+routerID+"/routeros-update", token, map[string]any{"latest": "7.19.4"})
	if status != http.StatusOK || out["commandId"] != cmdID {
		t.Fatalf("dédup update : %d %+v", status, out)
	}
	if al, _ := out["already"].(bool); !al {
		t.Fatalf("dédup update : marque « déjà en cours » attendue, %+v", out)
	}

	// Rapport ok (parti AVANT l'installation, pattern reboot) → journal du
	// lancement, read_state re-enfilé (fraîcheur post-écriture).
	rosReportResult(t, ts, agt, cmdID, url.Values{"status": {"ok"}, "action": {"install"}, "latest": {"7.19.4"}})
	st.Lock()
	var launchLogged, readQueued bool
	for _, a := range st.Data().Activity {
		if strings.Contains(a.Message, "Mise à jour RouterOS lancée") && strings.Contains(a.Message, "7.19.4") {
			launchLogged = true
		}
	}
	for _, c := range st.Data().Commands {
		if c.Kind == model.CmdReadState && c.RouterID == routerID && c.Status == "queued" {
			readQueued = true
		}
	}
	st.Unlock()
	if !launchLogged {
		t.Fatal("rapport update : journal du lancement absent")
	}
	if !readQueued {
		t.Fatal("rapport update : read_state de fraîcheur non re-enfilé")
	}

	// Le read_state post-redémarrage rapporte la NOUVELLE version → journal
	// de confirmation N°115 + version du routeur rafraîchie.
	var readCmdID string
	st.Lock()
	for _, c := range st.Data().Commands {
		if c.Kind == model.CmdReadState && c.RouterID == routerID && c.Status == "queued" {
			readCmdID = c.ID
		}
	}
	st.Unlock()
	if readCmdID == "" {
		t.Fatal("read_state de fraîcheur introuvable")
	}
	rosReportResult(t, ts, agt, readCmdID, url.Values{
		"status": {"ok"}, "version": {"7.19.4"}, "uptime": {"2m"}, "cpu": {"7"},
		"total": {"0"}, "start": {"0"}, "count": {"500"}, "stotal": {"0"}, "hosts": {"0"},
	})
	st.Lock()
	var confirmed bool
	for _, a := range st.Data().Activity {
		if strings.Contains(a.Message, "RouterOS de «") && strings.Contains(a.Message, "7.16.2 → 7.19.4") {
			confirmed = true
		}
	}
	var rr model.Router
	for i := range st.Data().Routers {
		if st.Data().Routers[i].ID == routerID {
			rr = st.Data().Routers[i]
		}
	}
	st.Unlock()
	if !confirmed {
		t.Fatal("confirmation de version (journal N°115) absente")
	}
	if rr.Version != "7.19.4" {
		t.Fatalf("version routeur : %q attendu 7.19.4", rr.Version)
	}
}

// TestRouterOSUpdatePayloadRejected — version cible hostile refusée avant
// toute mise en file (défense en profondeur : la valeur est embarquée dans
// le script .rsc du rapport de lancement).
func TestRouterOSUpdatePayloadRejected(t *testing.T) {
	_, ts, token, routerID := rosTestRig(t, "agent", "7.16.2")
	for _, bad := range []string{`7.19.4"; :reboot`, "not a version!", strings.Repeat("7", 40)} {
		status, _ := doJSON(t, ts, "POST", "/api/routers/"+routerID+"/routeros-update", token, map[string]any{"latest": bad})
		if status != http.StatusBadRequest {
			t.Fatalf("version hostile %q : statut %d attendu 400", bad, status)
		}
	}
}

// TestNormalizeRouterOSCheck — dérivation de l'état : libellés v7 réels,
// repli sur la comparaison de versions, états error/unknown honnêtes.
func TestNormalizeRouterOSCheck(t *testing.T) {
	cases := []struct {
		name  string
		in    map[string]any
		state string
	}{
		{"à jour", map[string]any{"rosStatus": "System is already up to date", "latest": "7.19.4", "installed": "7.19.4"}, routerOSStateLatest},
		{"disponible", map[string]any{"rosStatus": "New version is available: 7.19.4", "latest": "7.19.4", "installed": "7.16.2"}, routerOSStateAvailable},
		{"erreur", map[string]any{"rosStatus": "Error: could not resolve download.mikrotik.com"}, routerOSStateError},
		{"repli comparaison", map[string]any{"rosStatus": "un libellé inconnu du build", "latest": "7.19.4", "installed": "7.16.2"}, routerOSStateAvailable},
		{"repli identique", map[string]any{"rosStatus": "", "latest": "7.19.4", "installed": "7.19.4"}, routerOSStateLatest},
		{"inconnu", map[string]any{"rosStatus": "Something odd", "latest": "", "installed": ""}, routerOSStateUnknown},
		{"vide", map[string]any{}, routerOSStateUnknown},
	}
	for _, c := range cases {
		res := map[string]any{}
		for k, v := range c.in {
			res[k] = v
		}
		normalizeRouterOSCheck(res)
		if res["state"] != c.state {
			t.Fatalf("%s : état %q attendu %q (résultat %+v)", c.name, res["state"], c.state, res)
		}
		// Bornage du status brut (défense : 200 chars de junk ne gonflent pas
		// l'historique des commandes).
		if s, _ := res["status"].(string); len(s) > 160 {
			t.Fatalf("%s : status non borné (%d)", c.name, len(s))
		}
	}
}
