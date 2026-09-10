package api

// Tests N°75 — veille adaptative des agents : le cloud pilote le pas du
// scheduler MikCloud (45 s actif ↔ 180 s veille) via des commandes
// scheduler_set ordinaires. Les pièges couverts :
//   - le cadenceur read_state (2 min, N°74) file TOUJOURS une commande au
//     moment de décider → les commandes de BALAYAGE ne doivent pas réveiller
//     (ping-pong 45/180 infini sinon) ;
//   - un routeur en veille ne doit pas être marqué « hors ligne » entre deux
//     check-ins (EffectiveOfflineAfter = 3 × le pas) ;
//   - la vérité du pas vient du RAPPORT routeur (intervalSec échoé), pas du
//     payload émis.

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"mikcloud/hotspot-api/internal/agent"
	"mikcloud/hotspot-api/internal/model"
)

// newSleepAPI — API minimale avec la carte d'attention initialisée.
func newSleepAPI() *API {
	return &API{attn: map[string]time.Time{}}
}

// sleepRouter — routeur agent avec l'état de pas demandé.
func sleepRouter(schedulerSec int) *model.Router {
	return &model.Router{ID: "r-sleep", AccountID: "acc-sleep", Name: "Sleepy", Mode: "agent", Version: "7.20 (stable)", SchedulerSec: schedulerSec}
}

// TestAgentSleepIdleRouterSlowsDown — sans attention ni commandes
// actionnables, un routeur à 45 s (état historique 0) reçoit une bascule en
// veille (180 s) ; l'état connu à 180 s n'en re-file plus (convergence).
func TestAgentSleepIdleRouterSlowsDown(t *testing.T) {
	a := newSleepAPI()
	db := &model.DB{}

	// Historique (SchedulerSec 0 = 45 s implicites) : bascule veille en file.
	router := sleepRouter(0)
	a.ensureSchedulerIntervalLocked(db, router)
	cmd := findQueuedByKind(db, model.CmdSchedulerSet)
	if cmd == nil {
		t.Fatal("routeur inactif à 45 s : une commande scheduler_set (veille) doit être en file")
	}
	if iv, _ := cmd.Payload["intervalSec"].(int); iv != agentSleepSec {
		t.Fatalf("intervalle en file = %v, attendu %d (veille)", cmd.Payload["intervalSec"], agentSleepSec)
	}

	// Le rapport routeur pose la vérité : plus de divergence, plus de file.
	router.SchedulerSec = agentSleepSec
	db.Commands = nil
	a.ensureSchedulerIntervalLocked(db, router)
	if findQueuedByKind(db, model.CmdSchedulerSet) != nil {
		t.Fatal("état connu = 180 s : AUCUNE nouvelle bascule ne doit être en file (convergence)")
	}
}

// TestAgentSleepAttentionWakesUp — une requête console (marqueur de compte)
// ou un invité sur le portail (marqueur de routeur) ramène le pas à 45 s ;
// l'attention expirée (10 min) rend la veille.
func TestAgentSleepAttentionWakesUp(t *testing.T) {
	a := newSleepAPI()
	db := &model.DB{}
	router := sleepRouter(agentSleepSec)

	// Attention console (compte).
	a.markAttention("acc:" + router.AccountID)
	a.ensureSchedulerIntervalLocked(db, router)
	cmd := findQueuedByKind(db, model.CmdSchedulerSet)
	if cmd == nil {
		t.Fatal("attention console : la bascule rapide (45 s) doit être en file")
	}
	if iv, _ := cmd.Payload["intervalSec"].(int); iv != agentFastSec {
		t.Fatalf("intervalle en file = %v, attendu %d (rapide)", cmd.Payload["intervalSec"], agentFastSec)
	}

	// Attention portail (routeur direct) — même effet.
	db.Commands = nil
	router.SchedulerSec = agentSleepSec
	a.markAttention("rt:" + router.ID)
	a.ensureSchedulerIntervalLocked(db, router)
	if findQueuedByKind(db, model.CmdSchedulerSet) == nil {
		t.Fatal("attention portail : la bascule rapide doit être en file")
	}

	// Attention PÉRIMÉE (marqueur de +11 min) : retour à la veille.
	db.Commands = nil
	router.SchedulerSec = agentSleepSec
	old := time.Now().UTC().Add(-11 * time.Minute)
	a.attnMu.Lock()
	a.attn["acc:"+router.AccountID] = old
	a.attn["rt:"+router.ID] = old
	a.attnMu.Unlock()
	a.ensureSchedulerIntervalLocked(db, router)
	if findQueuedByKind(db, model.CmdSchedulerSet) != nil {
		t.Fatal("attention expirée : aucune bascule ne doit être en file (déjà en veille)")
	}
	if got := a.desiredAgentIntervalSec(db, router); got != agentSleepSec {
		t.Fatalf("attention expirée : pas désiré = %d, attendu %d (veille)", got, agentSleepSec)
	}
}

// TestAgentSleepSweepCommandsDoNotWake — le PIÈGE du cadenceur : un
// read_state en file (il l'est quasi toujours au moment de décider, N°74) ne
// doit PAS compter comme commande actionable — sinon ping-pong 45/180 à
// chaque check-in. Une commande métier (user_add), elle, réveille.
func TestAgentSleepSweepCommandsDoNotWake(t *testing.T) {
	a := newSleepAPI()
	db := &model.DB{}
	router := sleepRouter(agentSleepSec)

	for _, kind := range []string{model.CmdReadState, model.CmdWalledGarden, model.CmdHotspotFiles} {
		db.Commands = []model.Command{{ID: "c-x", RouterID: router.ID, Kind: kind, Status: "queued"}}
		if got := a.desiredAgentIntervalSec(db, router); got != agentSleepSec {
			t.Fatalf("commande de balayage %s : pas désiré = %d, attendu %d (veille — pas de ping-pong)", kind, got, agentSleepSec)
		}
	}

	db.Commands = []model.Command{{ID: "c-u", RouterID: router.ID, Kind: model.CmdUserAdd, Status: "queued"}}
	if got := a.desiredAgentIntervalSec(db, router); got != agentFastSec {
		t.Fatalf("commande métier en file : pas désiré = %d, attendu %d (rapide)", got, agentFastSec)
	}

	// La même commande pour un AUTRE routeur ne réveille pas celui-ci.
	db.Commands = []model.Command{{ID: "c-o", RouterID: "r-autre", Kind: model.CmdUserAdd, Status: "queued"}}
	if got := a.desiredAgentIntervalSec(db, router); got != agentSleepSec {
		t.Fatalf("commande d'un autre routeur : pas désiré = %d, attendu %d (veille)", got, agentSleepSec)
	}
}

// TestAgentSleepDedupInFlight — une bascule déjà en file/en vol n'est pas
// doublée.
func TestAgentSleepDedupInFlight(t *testing.T) {
	a := newSleepAPI()
	db := &model.DB{}
	router := sleepRouter(0)
	a.ensureSchedulerIntervalLocked(db, router)
	a.ensureSchedulerIntervalLocked(db, router)
	n := 0
	for _, c := range db.Commands {
		if c.Kind == model.CmdSchedulerSet {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("bascule dupliquée : %d commandes scheduler_set, attendu 1", n)
	}
}

// TestEffectiveOfflineAfter — un routeur en veille n'est pas « hors ligne »
// entre deux check-ins : seuil = max(réglage compte, 3 × pas).
func TestEffectiveOfflineAfter(t *testing.T) {
	cas := []struct {
		name         string
		cfg          int
		schedulerSec int
		want         time.Duration
	}{
		{"historique 45 s, défaut 135", 0, 0, 135 * time.Second},
		{"réglage strict, routeur actif", 120, 45, 135 * time.Second}, // max(120, 3×45)
		{"veille 180 s → 9 min", 135, 180, 540 * time.Second},
		{"réglage large prime (2 h)", 7200, 180, 7200 * time.Second},
	}
	for _, c := range cas {
		r := model.Router{SchedulerSec: c.schedulerSec}
		if got := r.EffectiveOfflineAfter(c.cfg); got != c.want {
			t.Fatalf("%s : EffectiveOfflineAfter(%d, %d) = %v, attendu %v", c.name, c.cfg, c.schedulerSec, got, c.want)
		}
	}
	// Un check-in vieux de 2 min sur un routeur en veille : EN LIGNE.
	r := sleepRouter(agentSleepSec)
	if d := r.EffectiveOfflineAfter(135); 2*time.Minute >= d {
		t.Fatalf("2 min sans check-in en veille devrait rester « en ligne » (seuil %v)", d)
	}
}

// TestAgentSleepEndToEnd — flux complet au travers du serveur HTTP : un
// routeur endormi sans attention reçoit scheduler_set(180) dans SON check-in,
// le rapport « ok + intervalSec=180 » pose Router.SchedulerSec, et une
// requête console authentifiée ramène la bascule rapide au check-in suivant.
func TestAgentSleepEndToEnd(t *testing.T) {
	st, ts := newTestServerWithStore(t)

	// Compte + token console (le porteur est lié au compte : chaque requête
	// authentifiée marque l'attention du compte — N°75).
	consoleToken, accID, _ := registerAccount(t, ts, "gerant-sleep-e2e", "")

	// Routeur agent du compte (token d'agent connu).
	const tok = "sl33p-t0ken-abcdefghijklmnop"
	st.Lock()
	st.Data().Routers = append(st.Data().Routers, model.Router{
		ID: "r-e2e", AccountID: accID, Name: "E2E Sleep", Mode: "agent", Status: "online",
		Version: "7.20 (stable)", AgentTokenHash: agent.HashToken(tok), SchedulerSec: agentFastSec,
		LastSeen: model.NowISO(),
	})
	st.Unlock()

	// 1) Check-in sans attention : la bascule veille est DANS le script.
	body := agentCheckIn(t, ts, tok)
	if !strings.Contains(body, "interval=180s") {
		t.Fatalf("check-in sans attention : le script doit porter la bascule veille (interval=180s) : %q", preview(body, 300))
	}
	cmdID := sleepCmdID(t, body)

	// 2) Rapport ok + intervalSec=180 → vérité routeur posée.
	agentReport(t, ts, tok, cmdID, url.Values{"status": {"ok"}, "intervalSec": {"180"}})
	st.Lock()
	got := st.Data().Routers[len(st.Data().Routers)-1].SchedulerSec
	st.Unlock()
	if got != agentSleepSec {
		t.Fatalf("après rapport ok : SchedulerSec = %d, attendu %d", got, agentSleepSec)
	}

	// 3) Requête console authentifiée (le marqueur d'attention du compte)
	//    puis check-in : bascule RAPIDE.
	if status, _ := doJSON(t, ts, "GET", "/api/dashboard", consoleToken, nil); status != http.StatusOK {
		t.Fatalf("dashboard console : statut %d", status)
	}
	body2 := agentCheckIn(t, ts, tok)
	if !strings.Contains(body2, "interval=45s") {
		t.Fatalf("check-in après attention console : le script doit porter la bascule rapide (interval=45s) : %q", preview(body2, 300))
	}
}

// --- helpers ---------------------------------------------------------------

func findQueuedByKind(db *model.DB, kind string) *model.Command {
	for i := range db.Commands {
		if db.Commands[i].Kind == kind && db.Commands[i].Status == "queued" {
			return &db.Commands[i]
		}
	}
	return nil
}

func agentCheckIn(t *testing.T, ts *httptest.Server, token string) string {
	t.Helper()
	resp, err := ts.Client().Get(ts.URL + "/agent/cmd?token=" + token)
	if err != nil {
		t.Fatalf("check-in : %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("check-in : statut %d", resp.StatusCode)
	}
	b := make([]byte, 0, 64*1024)
	tmp := make([]byte, 4096)
	for {
		n, rerr := resp.Body.Read(tmp)
		b = append(b, tmp[:n]...)
		if rerr != nil {
			break
		}
	}
	return string(b)
}

// sleepCmdID — relit l'ID de la commande scheduler_set depuis le commentaire
// d'audit du script (« # mikcloud cmd {id} scheduler_set »).
func sleepCmdID(t *testing.T, script string) string {
	t.Helper()
	for _, line := range strings.Split(script, "\n") {
		if strings.Contains(line, "mikcloud cmd ") && strings.Contains(line, model.CmdSchedulerSet) {
			parts := strings.Fields(strings.TrimSpace(line))
			// parts: # mikcloud cmd <id> scheduler_set
			if len(parts) >= 4 {
				return parts[3]
			}
		}
	}
	t.Fatal("aucune commande scheduler_set dans le script du check-in")
	return ""
}

func agentReport(t *testing.T, ts *httptest.Server, token, cmdID string, extra url.Values) {
	t.Helper()
	form := url.Values{"token": {token}, "cmd": {cmdID}}
	for k, vs := range extra {
		form[k] = vs
	}
	resp, err := ts.Client().PostForm(ts.URL+"/agent/result", form)
	if err != nil {
		t.Fatalf("rapport agent : %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("rapport agent : statut %d", resp.StatusCode)
	}
}

func preview(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
