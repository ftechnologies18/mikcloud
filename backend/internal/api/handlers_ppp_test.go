// Tests N°293 — PPPoE Phase A : vertical HTTP de bout en bout (surface
// publique réelle, store JSON éphémère) — création console → commande agent
// en file → application du rapport agent → parité read_state.
package api

import (
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"mikcloud/hotspot-api/internal/agent"
	"mikcloud/hotspot-api/internal/model"
	"mikcloud/hotspot-api/internal/store"
)

// seedPppAgentRouter — un routeur MODE AGENT avec token connu (hash stocké).
func seedPppAgentRouter(t *testing.T, st *store.Store, accID, id, name, token string) {
	t.Helper()
	st.Lock()
	st.Data().Routers = append(st.Data().Routers, model.Router{
		ID: id, AccountID: accID, Name: name, Mode: "agent", Status: "online",
		AgentTokenHash: agent.HashToken(token), CreatedAt: model.NowISO(),
	})
	st.Save()
	st.Unlock()
}

// postAgentResult — POST /agent/result (formulaire, comme le routeur).
// Les espaces sont émis « %20 » et NON « + » : le parseur tolérant du cloud
// (parseTolerantQuery, PathUnescape — les « + » restent littéraux pour les
// clés base64 des rapports wg) ne les convertirait pas.
func postAgentResult(t *testing.T, ts *httptest.Server, vals url.Values) (int, map[string]any) {
	t.Helper()
	body := strings.ReplaceAll(vals.Encode(), "+", "%20")
	resp, err := ts.Client().Post(ts.URL+"/agent/result", "application/x-www-form-urlencoded", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /agent/result : %v", err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

// TestPppSecretVerticalCreateSetRemove — cycle de vie complet d'un abonné
// PPPoE : création (201, commande ppp_secret_add en file, State=pending) →
// confirmation agent (State=active + LastSeenOnRouter) → modification
// (deltas + commande set) → suppression (pending → retrait registre à la
// confirmation) → échec rapporté (State=error + message, registre intact).
func TestPppSecretVerticalCreateSetRemove(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	ownerToken, accID, _ := registerAccount(t, ts, "ppp-vrt", "")
	seedPppAgentRouter(t, st, accID, "rt-ppp1", "POP Cocody", "agent-tok-ppp")

	// 1. Création — 201, commande en file, unicité par routeur.
	s1, out := doJSON(t, ts, "POST", "/api/routers/rt-ppp1/ppp/secrets", ownerToken, map[string]string{
		"name": "Abdou@FAI.CI", "password": "pw-abdou", "profile": "ABONNE-10M", "comment": "Abonné Diallo",
	})
	if s1 != 201 {
		t.Fatalf("création : statut %d, corps %v", s1, out)
	}
	secret, _ := out["secret"].(map[string]any)
	if secret == nil || secret["id"] == "" {
		t.Fatalf("création : secret attendu dans la réponse, %v", out)
	}
	secID, _ := secret["id"].(string)
	if got, _ := secret["name"].(string); got != "abdou@fai.ci" {
		t.Fatalf("création : nom normalisé attendu %q, %q", "abdou@fai.ci", got)
	}
	if got, _ := secret["state"].(string); got != "pending" {
		t.Fatalf("création : état pending attendu, %q", got)
	}
	// Doublon refusé (unicité PAR ROUTEUR, insensible à la casse).
	if s, _ := doJSON(t, ts, "POST", "/api/routers/rt-ppp1/ppp/secrets", ownerToken, map[string]string{
		"name": "ABDOU@FAI.CI", "profile": "default",
	}); s != 400 {
		t.Fatalf("doublon : 400 attendu, %d", s)
	}
	// Nom dangereux refusé (défense au bord console).
	if s, _ := doJSON(t, ts, "POST", "/api/routers/rt-ppp1/ppp/secrets", ownerToken, map[string]string{
		"name": `bad"name$`, "profile": "default",
	}); s != 400 {
		t.Fatalf("nom dangereux : 400 attendu, %d", s)
	}

	// 2. Le script de la commande contient le marqueur et le nom échappé.
	st.Lock()
	var addCmd *model.Command
	for i := range st.Data().Commands {
		if st.Data().Commands[i].Kind == model.CmdPppSecretAdd && st.Data().Commands[i].Status == "queued" {
			addCmd = &st.Data().Commands[i]
		}
	}
	if addCmd == nil {
		t.Fatal("commande ppp_secret_add attendue en file")
	}
	b := agent.Builder{BaseURL: "https://cloud.test", Token: "t"}
	script, err := b.ScriptFor(*addCmd)
	if err != nil || !strings.Contains(script, `name="abdou@fai.ci"`) || !strings.Contains(script, "mikcloud-ppp") {
		t.Fatalf("script add inattendu : %q (err %v)", script, err)
	}
	cmdID := addCmd.ID
	st.Unlock()

	// 3. Confirmation agent → active + LastSeenOnRouter + ErrorMsg vide.
	if s, _ := postAgentResult(t, ts, url.Values{
		"token": {"agent-tok-ppp"}, "cmd": {cmdID}, "status": {"ok"},
	}); s != 200 {
		t.Fatalf("rapport agent add : statut %d", s)
	}
	st.Lock()
	sec := model.FindPppSecretScoped(st.Data(), secID, accID)
	if sec == nil || sec.State != model.PppStateActive || sec.LastSeenOnRouter == "" {
		t.Fatalf("confirmation add : active+lastSeen attendus, %+v", sec)
	}
	st.Unlock()

	// 4. PATCH (profil + suspension) — cloud immédiat + commande set.
	if s, out2 := doJSON(t, ts, "PATCH", "/api/ppp/secrets/"+secID, ownerToken, map[string]any{
		"profile": "ABONNE-5M", "disabled": true,
	}); s != 200 {
		t.Fatalf("patch : statut %d, corps %v", s, out2)
	}
	st.Lock()
	sec = model.FindPppSecretScoped(st.Data(), secID, accID)
	if sec == nil || sec.Profile != "ABONNE-5M" || !sec.Disabled || sec.State != model.PppStatePending {
		t.Fatalf("patch immédiat : deltas+pending attendus, %+v", sec)
	}
	var setCmd *model.Command
	for i := range st.Data().Commands {
		if st.Data().Commands[i].Kind == model.CmdPppSecretSet && st.Data().Commands[i].Status == "queued" {
			setCmd = &st.Data().Commands[i]
		}
	}
	if setCmd == nil {
		t.Fatal("commande ppp_secret_set attendue en file")
	}
	if _, has := setCmd.Payload["password"]; has {
		t.Fatal("patch : le mot de passe non pointé ne doit PAS partir au routeur")
	}
	setID := setCmd.ID
	st.Unlock()

	// Confirmation du set : deltas réappliqués depuis le payload + active.
	if s, _ := postAgentResult(t, ts, url.Values{
		"token": {"agent-tok-ppp"}, "cmd": {setID}, "status": {"ok"},
	}); s != 200 {
		t.Fatalf("rapport agent set : statut %d", s)
	}
	st.Lock()
	sec = model.FindPppSecretScoped(st.Data(), secID, accID)
	if sec == nil || sec.State != model.PppStateActive || sec.Profile != "ABONNE-5M" || !sec.Disabled {
		t.Fatalf("confirmation set : active + deltas attendus, %+v", sec)
	}
	st.Unlock()

	// 5. Échec rapporté → State=error + message, registre INTACT.
	if s, _ := doJSON(t, ts, "PATCH", "/api/ppp/secrets/"+secID, ownerToken, map[string]any{
		"comment": "essai",
	}); s != 200 {
		t.Fatalf("patch 2 : statut %d", s)
	}
	st.Lock()
	for i := range st.Data().Commands {
		if st.Data().Commands[i].Kind == model.CmdPppSecretSet && st.Data().Commands[i].Status == "queued" {
			setID = st.Data().Commands[i].ID
		}
	}
	st.Unlock()
	if s, _ := postAgentResult(t, ts, url.Values{
		"token": {"agent-tok-ppp"}, "cmd": {setID}, "status": {"error"}, "message": {"profil absent sur le routeur"},
	}); s != 200 {
		t.Fatalf("rapport agent set en échec : statut %d", s)
	}
	st.Lock()
	sec = model.FindPppSecretScoped(st.Data(), secID, accID)
	if sec == nil || sec.State != model.PppStateError || !strings.Contains(sec.ErrorMsg, "profil absent") {
		t.Fatalf("échec : error+message attendus, %+v", sec)
	}
	st.Unlock()

	// 6. DELETE → pending puis confirmation → ligne RETIRÉE du registre.
	if s, _ := doJSON(t, ts, "DELETE", "/api/ppp/secrets/"+secID, ownerToken, nil); s != 200 {
		t.Fatalf("delete : statut %d", s)
	}
	st.Lock()
	var rmCmd *model.Command
	for i := range st.Data().Commands {
		if st.Data().Commands[i].Kind == model.CmdPppSecretRemove && st.Data().Commands[i].Status == "queued" {
			rmCmd = &st.Data().Commands[i]
		}
	}
	if rmCmd == nil {
		t.Fatal("commande ppp_secret_remove attendue en file (registre NON retiré avant confirmation)")
	}
	if model.FindPppSecretScoped(st.Data(), secID, accID) == nil {
		t.Fatal("discipline N°291 : le registre ne doit PAS être retiré avant la confirmation agent")
	}
	rmID := rmCmd.ID
	st.Unlock()
	if s, _ := postAgentResult(t, ts, url.Values{
		"token": {"agent-tok-ppp"}, "cmd": {rmID}, "status": {"ok"},
	}); s != 200 {
		t.Fatalf("rapport agent remove : statut %d", s)
	}
	st.Lock()
	if model.FindPppSecretScoped(st.Data(), secID, accID) != nil {
		t.Fatal("confirmation remove : la ligne doit être RETIRÉE du registre")
	}
	st.Unlock()
}

// TestPppParityReadState — parité : le paramètre ppp= du chunk FINAL du
// read_state rafraîchit LastSeenOnRouter et lève pending→active ; absent du
// rapport (script ancien) → AUCUNE déduction ; autre routeur → non touché.
func TestPppParityReadState(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	_, accID, _ := registerAccount(t, ts, "ppp-parity", "")
	seedPppAgentRouter(t, st, accID, "rt-par", "POP Yopougon", "agent-tok-par")
	st.Lock()
	old := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	st.Data().PppSecrets = append(st.Data().PppSecrets,
		model.PppSecret{ID: "pp-seen", AccountID: accID, RouterID: "rt-par", Name: "vu@fai.ci",
			State: model.PppStateActive, LastSeenOnRouter: old, CreatedAt: old, UpdatedAt: old},
		model.PppSecret{ID: "pp-pend", AccountID: accID, RouterID: "rt-par", Name: "nouveau@fai.ci",
			State: model.PppStatePending, CreatedAt: old, UpdatedAt: old},
		model.PppSecret{ID: "pp-autre", AccountID: accID, RouterID: "rt-AUTRE", Name: "vu@fai.ci",
			State: model.PppStateActive, LastSeenOnRouter: old, CreatedAt: old, UpdatedAt: old},
	)
	st.Save()
	st.Unlock()

	// Chunk final mono-fenêtre (total=0 users → final direct) AVEC ppp=.
	vals := url.Values{
		"token": {"agent-tok-par"}, "cmd": {"c-read-1"}, "status": {"ok"},
		"total": {"0"}, "start": {"0"}, "count": {"500"}, "out": {"0"},
		"users": {""}, "sessions": {""}, "stotal": {"0"},
		"ppp": {"vu@fai.ci|false;nouveau@fai.ci|true;"},
	}
	a := New(st, testJWTSecret)
	db := st.Data()
	st.Lock()
	router := routerByToken(db, "agent-tok-par")
	if router == nil {
		t.Fatal("routeur agent attendu")
	}
	final, _ := a.applyReadState(db, router, vals)
	st.Unlock()
	if !final {
		t.Fatal("chunk final attendu")
	}
	st.Lock()
	defer st.Unlock()
	sec := model.FindPppSecretScoped(db, "pp-seen", accID)
	if sec == nil || sec.LastSeenOnRouter == old {
		t.Fatalf("parité : LastSeenOnRouter rafraîchi attendu, %+v", sec)
	}
	sec = model.FindPppSecretScoped(db, "pp-pend", accID)
	if sec == nil || sec.State != model.PppStateActive {
		t.Fatalf("parité : pending→active attendu, %+v", sec)
	}
	// Absent du rapport → RIEN (pas de destruction, pas de badge).
	if sec := model.FindPppSecretScoped(db, "pp-autre", accID); sec == nil || sec.LastSeenOnRouter != old {
		t.Fatalf("parité : le secret d'un autre routeur ne doit pas bouger, %+v", sec)
	}
	// Paramètre ABSENT (script ancien) → comportement historique inchangé.
	// (le verrou est déjà porté par ce bloc — pas de re-Lock : deadlock)
	a2 := New(st, testJWTSecret)
	db.PppSecrets[0].LastSeenOnRouter = old
	vals2 := url.Values{
		"token": {"agent-tok-par"}, "cmd": {"c-read-2"}, "status": {"ok"},
		"total": {"0"}, "start": {"0"}, "count": {"500"}, "out": {"0"},
		"users": {""}, "sessions": {""}, "stotal": {"0"},
	}
	router = routerByToken(db, "agent-tok-par")
	a2.applyReadState(db, router, vals2)
	if db.PppSecrets[0].LastSeenOnRouter != old {
		t.Fatal("paramètre ppp absent : aucune déduction attendue (comportement historique)")
	}
}
