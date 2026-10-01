package api

// Tests N°201 — parent-queue résolu PAR ROUTEUR (fin de l'incident Zikisso) :
//   - résolution pure : qosQueueLiveOnRouter/parentQueueForRouter (régimes
//     vide / custom / managée vivante / managée absente) ;
//   - fan-out multi-box : profile_set et user_add ne référencent la file
//     managée mikcloud-qos QUE sur les box où elle existe (QoS activée +
//     sig convergée) ;
//   - auto-guérison : la CONVERGENCE d'une box (retour vérifié du
//     queue_ensure) réaligne les profils du compte qui référencent la file
//     managée, SUR CETTE BOX (arbitrage N°104 intact : pas d'écriture du
//     champ account-level en multi-box).

import (
	"net/http"
	"net/url"
	"testing"

	"mikcloud/hotspot-api/internal/agent"
	"mikcloud/hotspot-api/internal/model"
	"mikcloud/hotspot-api/internal/store"
)

// TestQoSParentQueueResolution — la règle à l'unité : trois régimes de
// parentQueueForRouter + la vérité qosQueueLiveOnRouter.
func TestQoSParentQueueResolution(t *testing.T) {
	live := &model.Router{ID: "r-live", Mode: "agent", QoSEnabled: true, QoSSig: "sig-live-00000001"}
	pending := &model.Router{ID: "r-pend", Mode: "agent", QoSEnabled: true, QoSSig: ""} // activée, pas encore convergée
	off := &model.Router{ID: "r-off", Mode: "agent"}
	real := &model.Router{ID: "r-real", Mode: "real", QoSEnabled: true, QoSSig: "sig"}

	if !qosQueueLiveOnRouter(live) {
		t.Fatal("activée + convergée : la file existe")
	}
	for _, r := range []*model.Router{pending, off, real, nil} {
		if qosQueueLiveOnRouter(r) {
			t.Fatalf("file déclarée vivante à tort : %+v", r)
		}
	}

	// Profil SANS file parent : clé présente à vide (parent-queue=none côté
	// routeur — sémantique historique inchangée).
	plain := model.Profile{ID: "pr-plain", Name: "PLAIN", ParentQueue: ""}
	if pq, ok := parentQueueForRouter(plain, off); !ok || pq != "" {
		t.Fatalf("profil sans file : attendu (\"\", true), obtenu (%q, %v)", pq, ok)
	}
	// File CUSTOM : le choix du gérant passe tel quel, même sur une box QoS
	// désactivée (le cloud ne connaît pas l'existence des files custom).
	custom := model.Profile{ID: "pr-cust", Name: "CUST", ParentQueue: "ma-file-agg"}
	if pq, ok := parentQueueForRouter(custom, off); !ok || pq != "ma-file-agg" {
		t.Fatalf("file custom : attendu (custom, true), obtenu (%q, %v)", pq, ok)
	}
	// File managée : vivante sur la box → référencée.
	managed := model.Profile{ID: "pr-mg", Name: "MG", ParentQueue: agent.QoSQueueName}
	if pq, ok := parentQueueForRouter(managed, live); !ok || pq != agent.QoSQueueName {
		t.Fatalf("file managée vivante : attendu (mikcloud-qos, true), obtenu (%q, %v)", pq, ok)
	}
	// File managée : absente de la box (désactivée OU non convergée) → clé
	// OMISE — c'est LE correctif Zikisso.
	for _, r := range []*model.Router{pending, off, real, nil} {
		if pq, ok := parentQueueForRouter(managed, r); ok || pq != "" {
			t.Fatalf("file managée absente : attendu (\"\", false) pour %+v, obtenu (%q, %v)", r, pq, ok)
		}
	}
}

// seededProfileSetPayloads — relève les payloads profile_set EN FILE pour un
// profil (par nom) et un routeur donnés (sous verrou).
func profileSetPayloads(t *testing.T, st *store.Store, name, routerID string) []map[string]any {
	t.Helper()
	var out []map[string]any
	st.Lock()
	defer st.Unlock()
	for _, c := range st.Data().Commands {
		if c.Kind != model.CmdProfileSet || c.RouterID != routerID {
			continue
		}
		if c.Status != "queued" && c.Status != "sent" {
			continue
		}
		if n, _ := c.Payload["name"].(string); n == name {
			out = append(out, c.Payload)
		}
	}
	return out
}

// TestQoSParentQueueMultiBoxFanOut — le cœur du correctif sur la surface
// HTTP : un compte à DEUX box agents (une QoS vivante, une muette), un profil
// rattaché à la file managée — les commandes ne référencent la file QUE sur
// la box où elle existe. C'est exactement la topologie de l'incident Zikisso
// (WIFI Zikisso QoS désactivée + profil partagé « 3-HEURES » rattaché).
func TestQoSParentQueueMultiBoxFanOut(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	token, accID, _ := registerAccount(t, ts, "n200-gerant", "")
	const tokA = "n200-t0ken-boxaaaa"
	const tokB = "n200-t0ken-boxbbbb"
	seedAgentRouter(t, st, accID, "r-boxa", "BOX A (QoS vivante)", tokA)
	seedAgentRouter(t, st, accID, "r-boxb", "BOX B (QoS muette)", tokB)
	st.Lock()
	for i := range st.Data().Routers {
		if st.Data().Routers[i].ID == "r-boxa" {
			st.Data().Routers[i].QoSEnabled = true
			st.Data().Routers[i].QoSSig = "sig-live-00000001"
		}
	}
	st.Save()
	st.Unlock()

	// 1) Profil rattaché à la file managée (choix du gérant, parité Mikhmon).
	status, out := doJSON(t, ts, "POST", "/api/profiles", token, map[string]any{
		"name": "3-HEURES", "rateLimit": "3M/3M", "sessionTimeoutMin": 180, "parentQueue": agent.QoSQueueName,
	})
	if status != http.StatusOK && status != http.StatusCreated {
		t.Fatalf("création profil : %d %v", status, out)
	}
	profileID, _ := out["id"].(string)

	// profile_set pour BOX A : la clé parentQueue EST présente (la file y existe).
	pa := profileSetPayloads(t, st, "3-HEURES", "r-boxa")
	if len(pa) != 1 {
		t.Fatalf("BOX A : 1 profile_set attendu, %d", len(pa))
	}
	if pq, _ := pa[0]["parentQueue"].(string); pq != agent.QoSQueueName {
		t.Fatalf("BOX A : parentQueue=%q attendu %q", pq, agent.QoSQueueName)
	}
	// profile_set pour BOX B : la clé parentQueue est OMISE (la file n'y
	// existe pas — l'add du profil aurait échoué silencieusement avant N°201).
	pb := profileSetPayloads(t, st, "3-HEURES", "r-boxb")
	if len(pb) != 1 {
		t.Fatalf("BOX B : 1 profile_set attendu, %d", len(pb))
	}
	if _, ok := pb[0]["parentQueue"]; ok {
		t.Fatalf("BOX B : la clé parentQueue doit être OMISE (file absente), payload=%v", pb[0])
	}

	// 2) user_add sur la BOX B : la référence profil embarquée n'emporte PAS
	// la file managée (c'est le chemin exact des vagues de réparation qui
	// bouclaient sur WIFI Zikisso).
	status, out = doJSON(t, ts, "POST", "/api/users", token, map[string]any{
		"username": "client-zikisso", "password": "pass1234", "profileId": profileID, "routerId": "r-boxb",
	})
	if status != http.StatusOK {
		t.Fatalf("user_add BOX B : %d %v", status, out)
	}
	userAddB := findQueuedPayload(t, st, model.CmdUserAdd, "r-boxb")
	prof, _ := userAddB["profile"].(map[string]any)
	if prof == nil {
		t.Fatalf("user_add BOX B : référence profil absente, payload=%v", userAddB)
	}
	if _, ok := prof["parentQueue"]; ok {
		t.Fatalf("user_add BOX B : la référence profil ne doit PAS porter parentQueue (file absente), profile=%v", prof)
	}

	// 3) user_add sur la BOX A : la référence profil EMPORTE la file (elle
	// y existe) — aucun détour du côté sain.
	status, out = doJSON(t, ts, "POST", "/api/users", token, map[string]any{
		"username": "client-sain", "password": "pass1234", "profileId": profileID, "routerId": "r-boxa",
	})
	if status != http.StatusOK {
		t.Fatalf("user_add BOX A : %d %v", status, out)
	}
	userAddA := findQueuedPayload(t, st, model.CmdUserAdd, "r-boxa")
	profA, _ := userAddA["profile"].(map[string]any)
	if pq, _ := profA["parentQueue"].(string); pq != agent.QoSQueueName {
		t.Fatalf("user_add BOX A : parentQueue=%q attendu %q", pq, agent.QoSQueueName)
	}

	// 4) File CUSTOM sur la BOX B : le choix du gérant passe tel quel (le
	// correctif ne filtre QUE la file managée).
	status, out = doJSON(t, ts, "POST", "/api/profiles", token, map[string]any{
		"name": "CUSTOM-PQ", "rateLimit": "2M/2M", "parentQueue": "ma-file-agg",
	})
	if status != http.StatusOK && status != http.StatusCreated {
		t.Fatalf("création profil custom : %d %v", status, out)
	}
	pc := profileSetPayloads(t, st, "CUSTOM-PQ", "r-boxb")
	if len(pc) != 1 {
		t.Fatalf("profil custom BOX B : 1 profile_set attendu, %d", len(pc))
	}
	if pq, _ := pc[0]["parentQueue"].(string); pq != "ma-file-agg" {
		t.Fatalf("profil custom BOX B : parentQueue=%q attendu %q (le choix du gérant passe)", pq, "ma-file-agg")
	}

	// 5) Sans file parent du tout : la clé est PRÉSENTE à vide (le set aligne
	// parent-queue=none — sémantique historique conservée).
	status, out = doJSON(t, ts, "POST", "/api/profiles", token, map[string]any{
		"name": "SANS-PQ", "rateLimit": "1M/1M",
	})
	if status != http.StatusOK && status != http.StatusCreated {
		t.Fatalf("création profil sans file : %d %v", status, out)
	}
	ps := profileSetPayloads(t, st, "SANS-PQ", "r-boxb")
	if len(ps) != 1 {
		t.Fatalf("profil sans file BOX B : 1 profile_set attendu, %d", len(ps))
	}
	pq, ok := ps[0]["parentQueue"].(string)
	if !ok || pq != "" {
		t.Fatalf("profil sans file : clé parentQueue attendue PRÉSENTE à vide (parent-queue=none), obtenu (%q, %v)", pq, ok)
	}
}

// TestQoSParentQueueConvergenceRepublish — auto-guérison multi-box : la BOX B
// active sa QoS, la file converge (retour vérifié du queue_ensure) → les
// profils qui référencent la file managée sont RÉALIGNÉS sur cette box, sans
// qu'aucun champ account-level ne bouge (arbitrage N°104 : l'attach
// automatique ne court pas en multi-box — ici on ne republie que le choix
// DÉJÀ pris par le gérant).
func TestQoSParentQueueConvergenceRepublish(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	token, accID, _ := registerAccount(t, ts, "n200-conv", "")
	const tokA = "n200-cnvg-boxaaaa"
	const tokB = "n200-cnvg-boxbbbb"
	seedAgentRouter(t, st, accID, "r-cboxa", "BOX A", tokA)
	seedAgentRouter(t, st, accID, "r-cboxb", "BOX B", tokB)
	st.Lock()
	for i := range st.Data().Routers {
		if st.Data().Routers[i].ID == "r-cboxa" {
			st.Data().Routers[i].QoSEnabled = true
			st.Data().Routers[i].QoSSig = "sig-live-00000001"
		}
	}
	st.Save()
	st.Unlock()

	// Profil rattaché (choix du gérant) : profile_set part sur A AVEC la
	// file, sur B SANS (elle n'y existe pas encore).
	status, out := doJSON(t, ts, "POST", "/api/profiles", token, map[string]any{
		"name": "1JOUR-CONV", "rateLimit": "4M/4M", "parentQueue": agent.QoSQueueName,
	})
	if status != http.StatusOK && status != http.StatusCreated {
		t.Fatalf("création profil : %d %v", status, out)
	}

	// La BOX B active sa QoS.
	status, out = doJSON(t, ts, "PUT", "/api/routers/r-cboxb/qos", token, map[string]any{
		"enabled": true, "target": "10.20.0.0/24", "maxUpBps": 10_000_000, "maxDownBps": 50_000_000,
	})
	if status != http.StatusOK {
		t.Fatalf("PUT qos BOX B : %d %v", status, out)
	}

	// Check-in BOX B : le profile_set initial (sans file) part, le
	// queue_ensure (deferred, vague 104) part en fermeture.
	body := agentCheckIn(t, ts, tokB)
	cmdID := deviceCmdID(t, body, model.CmdQueueEnsure)
	if cmdID == "" {
		t.Fatalf("le check-in BOX B doit servir queue_ensure :\n%s", preview(body, 600))
	}

	// Rapport honnête : la file existe désormais sur la box.
	agentReport(t, ts, tokB, cmdID, url.Values{"status": {"ok"},
		"data": {"queue|mikcloud-qos|10.20.0.0/24|10M/50M|pcq-upload-default/pcq-download-default|false;"}})

	st.Lock()
	var sigB string
	for i := range st.Data().Routers {
		if st.Data().Routers[i].ID == "r-cboxb" {
			sigB = st.Data().Routers[i].QoSSig
		}
	}
	st.Unlock()
	if sigB == "" {
		t.Fatal("convergence BOX B : la signature doit être posée (relecture conforme)")
	}

	// La REPUBLICATION : un NOUVEAU profile_set pour BOX B, cette fois AVEC
	// la file managée (elle existe désormais sur la box).
	pb := profileSetPayloads(t, st, "1JOUR-CONV", "r-cboxb")
	if len(pb) < 2 {
		t.Fatalf("BOX B : le profile_set initial + la republication attendus (≥ 2), %d obtenu(s)", len(pb))
	}
	last := pb[len(pb)-1]
	if pq, _ := last["parentQueue"].(string); pq != agent.QoSQueueName {
		t.Fatalf("republication BOX B : parentQueue=%q attendu %q (la file existe désormais)", pq, agent.QoSQueueName)
	}
	// La BOX A n'a rien reçu de plus (son profile_set initial, déjà avec la
	// file, suffit — pas de fan-out parasite).
	pa := profileSetPayloads(t, st, "1JOUR-CONV", "r-cboxa")
	if len(pa) != 1 {
		t.Fatalf("BOX A : exactement 1 profile_set attendu (pas de republication parasite), %d", len(pa))
	}

	// Le champ account-level n'a pas bougé en multi-box : le profil portait
	// DÉJÀ la file (choix du gérant), l'attach automatique ne l'a pas
	// réécrit — et les profils SANS file ne se sont pas fait rattacher.
	st.Lock()
	var plain model.Profile
	for i := range st.Data().Profiles {
		if st.Data().Profiles[i].AccountID == accID && st.Data().Profiles[i].ParentQueue == "" {
			plain = st.Data().Profiles[i]
			break
		}
	}
	st.Unlock()
	if plain.ID == "" {
		t.Fatal("précondition : le compte doit avoir un profil sans file (défaut de mise en service)")
	}
	if pcs := profileSetPayloads(t, st, plain.Name, "r-cboxb"); len(pcs) != 0 {
		t.Fatalf("profil sans file : la convergence ne doit PAS le republier (rien à réaligner), %d commande(s)", len(pcs))
	}
}

// findQueuedPayload — première commande du kind donné en file pour le
// routeur (payload nu — le test inspecte la commande telle qu'elle partira).
func findQueuedPayload(t *testing.T, st *store.Store, kind, routerID string) map[string]any {
	t.Helper()
	st.Lock()
	defer st.Unlock()
	for _, c := range st.Data().Commands {
		if c.Kind == kind && c.RouterID == routerID && (c.Status == "queued" || c.Status == "sent") {
			return c.Payload
		}
	}
	t.Fatalf("aucune commande %s en file pour %s", kind, routerID)
	return nil
}
