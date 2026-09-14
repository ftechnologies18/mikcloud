package api

// Tests N°104 — QoS Manager, bout en bout :
//   - recommandation : déclarée > mesurée > rien (jamais inventée) ;
//   - PUT /qos : validation (CIDR, bornes), état désiré posé, queue_ensure
//     enfilé immédiatement ;
//   - convergence : check-in sert queue_ensure → rapport honnête (relecture
//     conforme) → signature posée + profils rattachés (parent-queue) ;
//   - rapport menteur (limites divergentes) : pas de signature ;
//   - monitoring : queue_read cadencé, dérive vidée → re-file ;
//   - désactivation : queue_remove + profils détachés, retrait confirmé ;
//   - état par défaut : aucune commande pour un routeur qui n'a jamais
//     ouvert la carte (économie de veille N°75).

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"mikcloud/hotspot-api/internal/agent"
	"mikcloud/hotspot-api/internal/model"
)

// TestQoSRecommended — capacité déclarée prioritaire, mesure en repli, rien
// sans base honnête.
func TestQoSRecommended(t *testing.T) {
	now := time.Now().UTC()
	day := func(n int) string { return model.LineQualityDayKey(now.AddDate(0, 0, -n)) }

	// 1) Aucune base : aucune recommandation.
	db := &model.DB{}
	r := &model.Router{ID: "r-rec", WanIface: "ether1"}
	rec := qosRecommended(db, r, now)
	if rec.Source != "none" || rec.MaxDownBps != 0 {
		t.Fatalf("sans base : %+v", rec)
	}

	// 2) Mesurée (3 jours qualifiés sur le WAN) : max = 95 %, burst = capacité.
	db.LineQuality = []model.LineQualityDay{
		{RouterID: "r-rec", Day: day(1), Iface: "ether1", Samples: 300, RxMaxBps: 100_000_000, TxMaxBps: 19_000_000},
		{RouterID: "r-rec", Day: day(2), Iface: "ether1", Samples: 300, RxMaxBps: 96_000_000, TxMaxBps: 18_000_000},
		{RouterID: "r-rec", Day: day(3), Iface: "ether1", Samples: 300, RxMaxBps: 90_000_000, TxMaxBps: 17_000_000},
		{RouterID: "r-rec", Day: day(4), Iface: "ether1", Samples: 10, RxMaxBps: 999_000_000},  // creux : ignoré
		{RouterID: "r-rec", Day: day(1), Iface: "bridge", Samples: 300, RxMaxBps: 999_000_000}, // hors WAN
	}
	rec = qosRecommended(db, r, now)
	if rec.Source != "measured" || rec.CapacityDownBps != 100_000_000 || rec.CapacityUpBps != 19_000_000 {
		t.Fatalf("mesurée : %+v", rec)
	}
	if rec.MaxDownBps != 95_000_000 || rec.MaxUpBps != 18_050_000 {
		t.Fatalf("max 95 %% : %+v", rec)
	}
	if rec.BurstDownBps != 100_000_000 { // burst = capacité (max×20/19)
		t.Fatalf("burst : %+v", rec)
	}

	// 3) Déclarée : prime sur la mesure (l'étiquette borne le burst).
	r.LineDownBps, r.LineUpBps = 110_000_000, 20_000_000
	rec = qosRecommended(db, r, now)
	if rec.Source != "declared" || rec.CapacityDownBps != 110_000_000 {
		t.Fatalf("déclarée : %+v", rec)
	}
	if rec.MaxDownBps != 104_500_000 || rec.MaxUpBps != 19_000_000 {
		t.Fatalf("max 95 %% de la déclarée : %+v", rec)
	}
}

// TestQoSPutValidation — CIDR requis/canonisé, bornes des limites, état
// désiré posé + commande immédiate.
func TestQoSPutValidation(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	token, _, _ := registerAccount(t, ts, "qos-gerant", "")
	status, out := doJSON(t, ts, "POST", "/api/routers", token, map[string]any{"name": "SITE QOS", "mode": "agent"})
	if status != http.StatusOK {
		t.Fatalf("création routeur : %d %v", status, out)
	}
	routerID, _ := out["id"].(string)

	// CIDR invalide → 400.
	for _, bad := range []string{"192.168.10.0", "not-a-cidr", "fd00::/8"} {
		status, out = doJSON(t, ts, "PUT", "/api/routers/"+routerID+"/qos", token, map[string]any{
			"enabled": true, "target": bad, "maxUpBps": 20_000_000, "maxDownBps": 95_000_000,
		})
		if status != http.StatusBadRequest {
			t.Fatalf("cible %q : statut %d attendu 400", bad, status)
		}
	}
	// Limites hors bornes → 400 (1 Mbps à 10 Gbps).
	for _, lim := range []int64{500_000, 11_000_000_000} {
		status, _ = doJSON(t, ts, "PUT", "/api/routers/"+routerID+"/qos", token, map[string]any{
			"enabled": true, "target": "192.168.10.0/24", "maxUpBps": lim, "maxDownBps": 95_000_000,
		})
		if status != http.StatusBadRequest {
			t.Fatalf("limite %d : statut %d attendu 400", lim, status)
		}
	}
	// Activée sans cible → 400.
	status, _ = doJSON(t, ts, "PUT", "/api/routers/"+routerID+"/qos", token, map[string]any{
		"enabled": true, "maxUpBps": 20_000_000, "maxDownBps": 95_000_000,
	})
	if status != http.StatusBadRequest {
		t.Fatalf("sans cible : statut %d attendu 400", status)
	}

	// Valide : état posé, CIDR canonisé, queue_ensure enfilé immédiatement.
	status, out = doJSON(t, ts, "PUT", "/api/routers/"+routerID+"/qos", token, map[string]any{
		"enabled": true, "target": "192.168.10.0/24", "maxUpBps": 20_000_000, "maxDownBps": 95_000_000,
	})
	if status != http.StatusOK {
		t.Fatalf("PUT qos : %d %v", status, out)
	}
	st.Lock()
	var rr model.Router
	for i := range st.Data().Routers {
		if st.Data().Routers[i].ID == routerID {
			rr = st.Data().Routers[i]
		}
	}
	ensure := 0
	for _, c := range st.Data().Commands {
		if c.Kind == model.CmdQueueEnsure && (c.Status == "queued" || c.Status == "sent") {
			ensure++
		}
	}
	st.Unlock()
	if !rr.QoSEnabled || rr.QoSTarget != "192.168.10.0/24" || rr.QoSMaxUpBps != 20_000_000 || rr.QoSMaxDownBps != 95_000_000 || rr.QoSSig != "" {
		t.Fatalf("état désiré : %+v", rr)
	}
	if ensure != 1 {
		t.Fatalf("queue_ensure enfilée immédiatement : %d commande(s)", ensure)
	}
}

// TestQoSEndToEnd — le parcours doré : activation → check-in → relecture
// honnête → signature + rattachement des profils → monitoring → dérive →
// réparation → désactivation → retrait confirmé.
func TestQoSEndToEnd(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	token, accID, _ := registerAccount(t, ts, "qos-e2e-gerant", "")
	const tok = "q0s-t0ken-abcdefghijklm"
	seedAgentRouter(t, st, accID, "r-qos", "SITE E2E", tok)
	// Un profil hotspot du compte (la cible du rattachement parent-queue).
	st.Lock()
	st.Data().Profiles = append(st.Data().Profiles, model.Profile{
		ID: "pr-qos", AccountID: accID, Name: "1JOUR", RateLimit: "5M/5M",
	})
	st.Save()
	st.Unlock()

	// 1) Activation : cible + limites (95 % d'une ligne 20M/100M).
	status, out := doJSON(t, ts, "PUT", "/api/routers/r-qos/qos", token, map[string]any{
		"enabled": true, "target": "192.168.10.0/24", "maxUpBps": 19_000_000, "maxDownBps": 95_000_000,
	})
	if status != http.StatusOK {
		t.Fatalf("PUT qos : %d %v", status, out)
	}
	var prof model.Profile

	// 2) Check-in : queue_ensure servie (deferred, vague 104 — fermeture).
	body := agentCheckIn(t, ts, tok)
	cmdID := deviceCmdID(t, body, model.CmdQueueEnsure)
	if cmdID == "" {
		t.Fatalf("le check-in doit servir queue_ensure :\n%s", preview(body, 600))
	}

	// 3) Rapport HONNÊTE : relecture conforme (bps bruts ↔ formatés — le
	// cloud normalise chaque côté avant de comparer).
	agentReport(t, ts, tok, cmdID, url.Values{"status": {"ok"},
		"data": {"queue|mikcloud-qos|192.168.10.0/24|19M/95M|pcq-upload-default/pcq-download-default|false;"}})
	st.Lock()
	var rr model.Router
	for i := range st.Data().Routers {
		if st.Data().Routers[i].ID == "r-qos" {
			rr = st.Data().Routers[i]
		}
	}
	// Le compte de mise en service porte ses profils par défaut (ex. Staff) :
	// le rattachement est UN profile_set PAR PROFIL (l'état complet de chacun
	// converge vers le routeur) — le compte du test en a donc 2 (Staff + 1JOUR).
	profileSet := 0
	profilesTotal := 0
	profilesWired := 0
	for _, c := range st.Data().Commands {
		if c.Kind == model.CmdProfileSet && (c.Status == "queued" || c.Status == "sent") {
			profileSet++
		}
	}
	for i := range st.Data().Profiles {
		p := &st.Data().Profiles[i]
		if p.AccountID != accID {
			continue
		}
		profilesTotal++
		if p.ParentQueue == agent.QoSQueueName {
			profilesWired++
		}
		if p.ID == "pr-qos" {
			prof = *p
		}
	}
	st.Unlock()
	if rr.QoSSig == "" || rr.QoSAppliedAt == "" {
		t.Fatalf("relecture conforme : signature+horodatage attendus, %+v", rr)
	}
	if profilesWired != profilesTotal || profilesTotal == 0 {
		t.Fatalf("rattachement : %d/%d profils rattachés", profilesWired, profilesTotal)
	}
	if profileSet != profilesTotal {
		t.Fatalf("le rattachement doit filer UN profile_set PAR PROFIL (%d attendu(s)), %d obtenu(s)", profilesTotal, profileSet)
	}

	// 4) Rapport MENTEUR (limites divergentes) : jamais de signature.
	st.Lock()
	for i := range st.Data().Routers {
		if st.Data().Routers[i].ID == "r-qos" {
			st.Data().Routers[i].QoSSig = ""
			st.Data().Routers[i].QoSAppliedAt = ""
		}
	}
	st.Save()
	st.Unlock()
	body = agentCheckIn(t, ts, tok)
	cmdID = deviceCmdID(t, body, model.CmdQueueEnsure)
	if cmdID == "" {
		t.Fatalf("sig vide : re-file attendu :\n%s", preview(body, 400))
	}
	agentReport(t, ts, tok, cmdID, url.Values{"status": {"ok"},
		"data": {"queue|mikcloud-qos|192.168.10.0/24|8M/30M|pcq-upload-default/pcq-download-default|false;"}})
	st.Lock()
	for i := range st.Data().Routers {
		if st.Data().Routers[i].ID == "r-qos" {
			rr = st.Data().Routers[i]
		}
	}
	st.Unlock()
	if rr.QoSSig != "" {
		t.Fatalf("relecture divergente : la signature ne doit PAS être posée, %+v", rr)
	}

	// 5) Ré-application honnête, puis monitoring : le check-in suivant enfile
	// queue_read (cadence 30 min, aucune lecture done).
	agentReport(t, ts, tok, cmdID, url.Values{"status": {"ok"},
		"data": {"queue|mikcloud-qos|192.168.10.0/24|19000000/95000000|pcq-upload-default/pcq-download-default|false;"}})
	body = agentCheckIn(t, ts, tok)
	readID := deviceCmdID(t, body, model.CmdQueueRead)
	if readID == "" {
		t.Fatalf("sig fraîche : le monitoring queue_read doit être enfilé :\n%s", preview(body, 500))
	}
	// 6) Dérive rapportée (file absente) : signature vidée → re-file.
	agentReport(t, ts, tok, readID, url.Values{"status": {"ok"}, "data": {""}})
	st.Lock()
	for i := range st.Data().Routers {
		if st.Data().Routers[i].ID == "r-qos" {
			rr = st.Data().Routers[i]
		}
	}
	st.Unlock()
	if rr.QoSSig != "" {
		t.Fatalf("dérive détectée : la signature doit être vidée (re-file), %+v", rr)
	}

	// 7) Désactivation : profils détachés + queue_remove + profile_set de
	// détachement, retrait confirmé au retour.
	status, out = doJSON(t, ts, "DELETE", "/api/routers/r-qos/qos", token, nil)
	if status != http.StatusOK {
		t.Fatalf("DELETE qos : %d %v", status, out)
	}
	st.Lock()
	removeQueued := 0
	detachSet := 0
	for _, c := range st.Data().Commands {
		if c.Status != "queued" && c.Status != "sent" {
			continue
		}
		switch c.Kind {
		case model.CmdQueueRemove:
			removeQueued++
		case model.CmdProfileSet:
			// Détachement seulement : les profile_set d'attachement
			// (parentQueue=mikcloud-qos) peuvent rester « sent » — le
			// filtre porte la sémantique, pas le statut.
			if pq, _ := c.Payload["parentQueue"].(string); pq == "" {
				detachSet++
			}
		}
	}
	profilesTotal = 0
	for i := range st.Data().Profiles {
		p := &st.Data().Profiles[i]
		if p.AccountID != accID {
			continue
		}
		profilesTotal++
		if p.ID == "pr-qos" {
			prof = *p
		}
	}
	st.Unlock()
	if removeQueued != 1 {
		t.Fatalf("queue_remove enfilée : %d", removeQueued)
	}
	if prof.ParentQueue != "" {
		t.Fatalf("désactivation : le profil doit être détaché, ParentQueue = %q", prof.ParentQueue)
	}
	if detachSet != profilesTotal {
		t.Fatalf("détachement : UN profile_set par profil (%d attendu(s)), %d", profilesTotal, detachSet)
	}
	body = agentCheckIn(t, ts, tok)
	rmID := deviceCmdID(t, body, model.CmdQueueRemove)
	if rmID == "" {
		t.Fatalf("le check-in doit servir queue_remove :\n%s", preview(body, 500))
	}
	agentReport(t, ts, tok, rmID, url.Values{"status": {"ok"}, "data": {"removed|0;"}})
	st.Lock()
	for i := range st.Data().Routers {
		if st.Data().Routers[i].ID == "r-qos" {
			rr = st.Data().Routers[i]
		}
	}
	st.Unlock()
	if rr.QoSAppliedAt != "" {
		t.Fatalf("retrait confirmé : QoSAppliedAt doit être levé, %+v", rr)
	}

	// 8) Silence : jamais activée + désactivée → plus AUCUNE commande QoS
	// (économie de veille N°75).
	body = agentCheckIn(t, ts, tok)
	for _, kind := range []string{model.CmdQueueEnsure, model.CmdQueueRead, model.CmdQueueRemove} {
		if id := deviceCmdID(t, body, kind); id != "" {
			t.Fatalf("état silencieux : %s ne doit plus être filé :\n%s", kind, preview(body, 400))
		}
	}
}

// TestQoSGetEndpoint — la carte : état, recommandation, files (cache frais du
// queue_read servies parsées — le formatage RouterOS devient des bps).
func TestQoSGetEndpoint(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	token, accID, _ := registerAccount(t, ts, "qos-get-gerant", "")
	const tok = "q0s-t0ken-getendpoint"
	seedAgentRouter(t, st, accID, "r-get", "SITE GET", tok)

	st.Lock()
	for i := range st.Data().Routers {
		if st.Data().Routers[i].ID == "r-get" {
			st.Data().Routers[i].QoSEnabled = true
			st.Data().Routers[i].QoSTarget = "192.168.10.0/24"
			st.Data().Routers[i].QoSMaxUpBps = 19_000_000
			st.Data().Routers[i].QoSMaxDownBps = 95_000_000
			st.Data().Routers[i].LineDownBps = 100_000_000
			st.Data().Routers[i].LineUpBps = 20_000_000
		}
	}
	// Cache frais d'un queue_read (rapport déjà appliqué).
	st.Data().Commands = append(st.Data().Commands, model.Command{
		ID: "c-qr1", RouterID: "r-get", AccountID: accID, Kind: model.CmdQueueRead,
		Status: "done", DoneAt: model.NowISO(),
		Result: map[string]any{"data": "queue|mikcloud-qos|192.168.10.0/24|19M/95M|pcq-upload-default/pcq-download-default|false|123456/654321|9.5Mbps/72Mbps;queue|<D-u1>|192.168.10.0/24|5M/5M|default-small/default|false|1000/2000|1.2Mbps/2.4Mbps;"},
	})
	st.Save()
	st.Unlock()

	status, out := doJSON(t, ts, "GET", "/api/routers/r-get/qos", token, nil)
	if status != http.StatusOK {
		t.Fatalf("GET qos : %d %v", status, out)
	}
	statusOut, _ := out["status"].(map[string]any)
	if statusOut["enabled"] != true || statusOut["target"] != "192.168.10.0/24" {
		t.Fatalf("status : %v", statusOut)
	}
	rec, _ := out["recommendation"].(map[string]any)
	if rec["source"] != "declared" || rec["maxDownBps"] != float64(95_000_000) {
		t.Fatalf("recommandation : %v", rec)
	}
	queues, _ := out["queues"].(map[string]any)
	if queues["queued"] != false {
		t.Fatalf("cache frais : pas de re-lecture à la demande, %v", queues)
	}
	rows, _ := queues["data"].([]any)
	if len(rows) != 2 {
		t.Fatalf("2 files attendues (agrégat + dynamique), %d", len(rows))
	}
	first, _ := rows[0].(map[string]any)
	if first["name"] != "mikcloud-qos" || first["maxDownBps"] != float64(95_000_000) ||
		first["rateDownBps"] != float64(72_000_000) || first["bytesUp"] != float64(123456) {
		t.Fatalf("file agrégat parsée : %v", first)
	}
}
