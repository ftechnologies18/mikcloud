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
	"strings"
	"testing"
	"time"

	"mikcloud/hotspot-api/internal/agent"
	"mikcloud/hotspot-api/internal/model"
	"mikcloud/hotspot-api/internal/store"
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

// TestParseQueueRowsDynamic — N°106 — le drapeau dynamique : rapporté par le
// script (9e colonne) ou déduit du nom entre chevrons (rapport ancien) ;
// une file statique reste statique, jamais d'étiquette fantôme.
func TestParseQueueRowsDynamic(t *testing.T) {
	rows := parseQueueRows(splitAgentList(
		"queue|mikcloud-qos|192.168.10.0/24|19M/95M|pcq-upload-default/pcq-download-default|false|1/2|3M/4M|false;" +
			"queue|<D-u1>|192.168.10.0/24|5M/5M|default-small/default|false|1/2|1M/2M|true;" +
			"queue|HOTSPOT-Total|192.168.10.0/24|13M/75M|pcq-upload-default/pcq-download-default|false|7/9|2M/40M|false;"))
	if len(rows) != 3 {
		t.Fatalf("3 files attendues, %d", len(rows))
	}
	if rows[0].Dynamic || rows[2].Dynamic {
		t.Fatalf("files statiques étiquetées dynamiques : %+v / %+v", rows[0], rows[2])
	}
	if !rows[1].Dynamic {
		t.Fatalf("file dynamique rapportée (9e colonne) : %+v", rows[1])
	}
	// Rapport ancien (8 colonnes, sans le drapeau) : l'heuristique des
	// chevrons sauve l'étiquette.
	old := parseQueueRows(splitAgentList(
		"queue|<D-u2>|192.168.10.0/24|5M/5M|default-small/default|false|1/2|1M/2M;"))
	if len(old) != 1 || !old[0].Dynamic {
		t.Fatalf("heuristique chevrons : %+v", old)
	}
}

// TestRouterQueueDeleteLegacy — N°106 — le ménage à distance, bout en bout :
// une file legacy (HOTSPOT-Total posée à la main avant le QoS Manager) avec
// des profils qui la référencent → DELETE /queues/{name} → détachement +
// queue_remove NOMMÉ enfilé → check-in sert un script visant CETTE file (et
// aucune autre) → rapport removed|0 → journal honnête, état QoS INTACT (le
// ménage d'une file legacy ne débranche pas la QoS du routeur) → le cache
// de la table est périmé (cutoff) → re-lecture → table sans la file.
func TestRouterQueueDeleteLegacy(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	token, accID, _ := registerAccount(t, ts, "qos-clean-gerant", "")
	const tok = "q0s-t0ken-legacy"
	seedAgentRouter(t, st, accID, "r-clean", "SITE CLEAN", tok)

	// QoS active et APPLIQUÉE (la file agrégat vit sur le routeur) + un
	// profil qui référence la file legacy (état d'avant le manager) + un
	// cache frais de la table des files montrant l'agrégat ET la legacy.
	wantSig := ""
	st.Lock()
	for i := range st.Data().Routers {
		if st.Data().Routers[i].ID == "r-clean" {
			r := &st.Data().Routers[i]
			r.QoSEnabled = true
			r.QoSTarget = "192.168.10.0/24"
			r.QoSMaxUpBps = 19_000_000
			r.QoSMaxDownBps = 95_000_000
			r.QoSAppliedAt = model.NowISO()
			r.QoSSig = qosSig(r) // sig RÉELLE : pas de queue_ensure parasite au check-in
			wantSig = r.QoSSig
		}
	}
	st.Data().Profiles = append(st.Data().Profiles, model.Profile{
		ID: "pr-legacy", AccountID: accID, Name: "1JOUR", RateLimit: "5M/5M", ParentQueue: "HOTSPOT-Total",
	})
	st.Data().Commands = append(st.Data().Commands, model.Command{
		ID: "c-qr-old", RouterID: "r-clean", AccountID: accID, Kind: model.CmdQueueRead,
		Status: "done", DoneAt: model.NowISO(),
		Result: map[string]any{"data": "queue|mikcloud-qos|192.168.10.0/24|19M/95M|pcq-upload-default/pcq-download-default|false|1/2|3M/4M|false;queue|HOTSPOT-Total|192.168.10.0/24|13M/75M|pcq-upload-default/pcq-download-default|false|7/9|2M/40M|false;"},
	})
	st.Save()
	st.Unlock()

	// Le cache frais se sert tel quel (2 files).
	status, out := doJSON(t, ts, "GET", "/api/routers/r-clean/qos", token, nil)
	if status != http.StatusOK {
		t.Fatalf("GET qos : %d %v", status, out)
	}
	queues, _ := out["queues"].(map[string]any)
	if queues["queued"] != false {
		t.Fatalf("cache frais : %v", queues)
	}
	if rows, _ := queues["data"].([]any); len(rows) != 2 {
		t.Fatalf("cache frais : 2 files attendues, %d", len(rows))
	}

	// Le geste du gérant : DELETE /api/routers/{id}/queues/HOTSPOT-Total.
	status, out = doJSON(t, ts, "DELETE", "/api/routers/r-clean/queues/HOTSPOT-Total", token, nil)
	if status != http.StatusOK {
		t.Fatalf("DELETE queue legacy : %d %v", status, out)
	}
	st.Lock()
	removeQueued := 0
	rmName := ""
	detachSet := 0
	for _, c := range st.Data().Commands {
		if c.Status != "queued" && c.Status != "sent" {
			continue
		}
		if c.Kind == model.CmdQueueRemove {
			removeQueued++
			rmName, _ = c.Payload["name"].(string)
		}
		if c.Kind == model.CmdProfileSet {
			if pq, _ := c.Payload["parentQueue"].(string); pq == "" {
				detachSet++
			}
		}
	}
	var prof model.Profile
	for i := range st.Data().Profiles {
		if st.Data().Profiles[i].ID == "pr-legacy" {
			prof = st.Data().Profiles[i]
		}
	}
	st.Unlock()
	if removeQueued != 1 || rmName != "HOTSPOT-Total" {
		t.Fatalf("queue_remove nommé enfilé : %d commande(s), name=%q", removeQueued, rmName)
	}
	if prof.ParentQueue != "" {
		t.Fatalf("le profil doit être détaché de HOTSPOT-Total, ParentQueue=%q", prof.ParentQueue)
	}
	if detachSet < 1 {
		t.Fatalf("détachement : au moins UN profile_set (celui du profil legacy), %d", detachSet)
	}

	// Check-in : le script servi vise HOTSPOT-Total, JAMAIS mikcloud-qos.
	body := agentCheckIn(t, ts, tok)
	rmID := deviceCmdID(t, body, model.CmdQueueRemove)
	if rmID == "" {
		t.Fatalf("le check-in doit servir queue_remove :\n%s", preview(body, 500))
	}
	rmBlock := cmdScriptBlock(t, body, rmID)
	if !strings.Contains(rmBlock, `name="HOTSPOT-Total"`) {
		t.Fatalf("le script doit viser HOTSPOT-Total :\n%s", preview(rmBlock, 400))
	}
	if strings.Contains(rmBlock, `name="mikcloud-qos"`) {
		t.Fatalf("le retrait ne doit PAS toucher mikcloud-qos :\n%s", preview(rmBlock, 400))
	}

	// Rapport : disparition prouvée → l'état QoS reste INTACT.
	agentReport(t, ts, tok, rmID, url.Values{"status": {"ok"}, "data": {"removed|0;"}})
	// Déterminisme : NowISO est à la seconde — on garantit lecture ancienne
	// < retrait < lecture fraîche en décalant les DoneAt en store (les
	// comparaisons du cutoff sont strictes : égalité = cache périmé).
	shiftCmdDoneAt(t, st, "c-qr-old", -1)
	st.Lock()
	var rr model.Router
	for i := range st.Data().Routers {
		if st.Data().Routers[i].ID == "r-clean" {
			rr = st.Data().Routers[i]
		}
	}
	st.Unlock()
	if !rr.QoSEnabled || rr.QoSSig != wantSig || rr.QoSAppliedAt == "" {
		t.Fatalf("état QoS intact après ménage legacy : %+v", rr)
	}

	// Le cache d'avant le retrait est PÉRIMÉ (cutoff) : GET /qos enfile
	// une nouvelle lecture au lieu de resservir la table avec la file
	// supprimée — la vérité routeur, pas un cliché antérieur au geste.
	status, out = doJSON(t, ts, "GET", "/api/routers/r-clean/qos", token, nil)
	if status != http.StatusOK {
		t.Fatalf("GET qos après retrait : %d %v", status, out)
	}
	queues, _ = out["queues"].(map[string]any)
	if queues["queued"] != true {
		t.Fatalf("cache périmé : une re-lecture doit être en attente, %v", queues)
	}

	// La re-lecture servie puis rapportée : la table revient SANS la
	// file legacy — la preuve complète du ménage à distance.
	body = agentCheckIn(t, ts, tok)
	readID := deviceCmdID(t, body, model.CmdQueueRead)
	if readID == "" {
		t.Fatalf("le check-in doit servir queue_read :\n%s", preview(body, 400))
	}
	agentReport(t, ts, tok, readID, url.Values{"status": {"ok"},
		"data": {"queue|mikcloud-qos|192.168.10.0/24|19M/95M|pcq-upload-default/pcq-download-default|false|1/2|3M/4M|false;"}})
	// Déterminisme (suite) : la lecture fraîche doit être POSTÉRIEURE au
	// retrait — on la décale d'une seconde en store.
	shiftCmdDoneAt(t, st, readID, 1)
	status, out = doJSON(t, ts, "GET", "/api/routers/r-clean/qos", token, nil)
	if status != http.StatusOK {
		t.Fatalf("GET qos final : %d %v", status, out)
	}
	queues, _ = out["queues"].(map[string]any)
	rows, _ := queues["data"].([]any)
	if queues["queued"] != false || len(rows) != 1 {
		t.Fatalf("table rafraîchie : 1 file (l'agrégat) attendue, %v", queues)
	}
	if only, _ := rows[0].(map[string]any); only["name"] != "mikcloud-qos" {
		t.Fatalf("seule l'agrégat doit rester : %v", only)
	}
}

// cmdScriptBlock — extrait le segment du script d'un check-in dédié à UNE
// commande (entre son commentaire d'audit et le suivant) : les assertions
// portent sur CE que fait CETTE commande, pas sur le reste du script
// (concaténation des commandes servies ensemble).
func cmdScriptBlock(t *testing.T, script, cmdID string) string {
	t.Helper()
	marker := "# mikcloud cmd " + cmdID + " "
	start := strings.Index(script, marker)
	if start < 0 {
		t.Fatalf("commande %s absente du script", cmdID)
	}
	rest := script[start:]
	if next := strings.Index(rest[len(marker):], "# mikcloud cmd "); next >= 0 {
		rest = rest[:len(marker)+next]
	}
	return rest
}

// shiftCmdDoneAt — décale le DoneAt d'une commande en store de n secondes
// (déterminisme des comparaisons strictes quand NowISO tombe à la seconde).
func shiftCmdDoneAt(t *testing.T, st *store.Store, cmdID string, seconds int) {
	t.Helper()
	st.Lock()
	defer st.Unlock()
	for i := range st.Data().Commands {
		if st.Data().Commands[i].ID != cmdID {
			continue
		}
		tt, err := time.Parse(time.RFC3339, st.Data().Commands[i].DoneAt)
		if err != nil {
			t.Fatalf("DoneAt illisible (%s) : %v", cmdID, err)
		}
		st.Data().Commands[i].DoneAt = tt.Add(time.Duration(seconds) * time.Second).Format(time.RFC3339)
		return
	}
	t.Fatalf("commande %s introuvable pour le décalage DoneAt", cmdID)
}

// TestRouterQueueDeleteGuards — N°106 — les gardes-fous : mikcloud-qos
// refusé (son retrait passe par « Désactiver la QoS »), dynamiques refusés,
// charset hostile refusé, routeur inconnu 404, non authentifié 401, et le
// geste convergent (re-clic pendant la convergence : pas d'accumulation).
func TestRouterQueueDeleteGuards(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	token, accID, _ := registerAccount(t, ts, "qos-guard-gerant", "")
	const tok = "q0s-t0ken-guards"
	seedAgentRouter(t, st, accID, "r-guard", "SITE GUARD", tok)

	// mikcloud-qos : refusé — la désactivation QoS pilote AUSSI l'état cloud.
	status, out := doJSON(t, ts, "DELETE", "/api/routers/r-guard/queues/mikcloud-qos", token, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("mikcloud-qos : statut %d attendu 400 (%v)", status, out)
	}
	// File de bridage quota (N°106 parallèle) : refusée — pilotée par le
	// scheduler mikcloud-quota du routeur, la supprimer est vain.
	status, _ = doJSON(t, ts, "DELETE", "/api/routers/r-guard/queues/mikthrottle-D-u1", token, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("mikthrottle : statut %d attendu 400", status)
	}
	// File dynamique (« <D-u1> » encodé) : refusée.
	status, _ = doJSON(t, ts, "DELETE", "/api/routers/r-guard/queues/%3CD-u1%3E", token, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("dynamique : statut %d attendu 400", status)
	}
	// Charset hostile (guillemet, tube, point-virgule) : refusé.
	for _, bad := range []string{"a%22b", "a%7Cb", "a%3Bb", "%20", "%09x"} {
		status, _ = doJSON(t, ts, "DELETE", "/api/routers/r-guard/queues/"+bad, token, nil)
		if status != http.StatusBadRequest {
			t.Fatalf("nom hostile %q : statut %d attendu 400", bad, status)
		}
	}
	// Routeur inconnu : 404.
	status, _ = doJSON(t, ts, "DELETE", "/api/routers/r-inconnu/queues/HOTSPOT-Total", token, nil)
	if status != http.StatusNotFound {
		t.Fatalf("routeur inconnu : statut %d attendu 404", status)
	}
	// Non authentifié : 401.
	status, _ = doJSON(t, ts, "DELETE", "/api/routers/r-guard/queues/HOTSPOT-Total", "", nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("sans token : statut %d attendu 401", status)
	}

	// Routeur simulé : geste no-op convergent (rien de réel à retirer),
	// aucune commande enfilée — la démo ne s'habille pas d'un mensonge.
	// (Semé en store : le plan d'essai couvre 1 seul routeur.)
	st.Lock()
	st.Data().Routers = append(st.Data().Routers, model.Router{
		ID: "r-sim", AccountID: accID, Name: "SITE DEMO", Mode: "simulated", Status: "online",
	})
	st.Save()
	st.Unlock()
	status, out = doJSON(t, ts, "DELETE", "/api/routers/r-sim/queues/HOTSPOT-Total", token, nil)
	if status != http.StatusOK {
		t.Fatalf("simulé : %d %v", status, out)
	}

	// Convergence idempotente : re-clic pendant le vol → pas de doublon.
	status, _ = doJSON(t, ts, "DELETE", "/api/routers/r-guard/queues/HOTSPOT-Total", token, nil)
	if status != http.StatusOK {
		t.Fatalf("premier DELETE agent : %d", status)
	}
	status, _ = doJSON(t, ts, "DELETE", "/api/routers/r-guard/queues/HOTSPOT-Total", token, nil)
	if status != http.StatusOK {
		t.Fatalf("re-clic pendant le vol : %d", status)
	}
	st.Lock()
	sameName := 0
	for _, c := range st.Data().Commands {
		if c.Kind == model.CmdQueueRemove && (c.Status == "queued" || c.Status == "sent") &&
			agent.QueueRemoveTarget(c.Payload) == "HOTSPOT-Total" {
			sameName++
		}
	}
	st.Unlock()
	if sameName != 1 {
		t.Fatalf("pas d'accumulation : 1 queue_remove en vol attendu, %d", sameName)
	}

	// Un retrait d'un AUTRE nom coexiste (deux files legacy distinctes :
	// deux gestes ponctuels, tous deux convergent).
	status, _ = doJSON(t, ts, "DELETE", "/api/routers/r-guard/queues/GLOBAL-Internet", token, nil)
	if status != http.StatusOK {
		t.Fatalf("second nom : %d", status)
	}
	st.Lock()
	other := 0
	for _, c := range st.Data().Commands {
		if c.Kind == model.CmdQueueRemove && (c.Status == "queued" || c.Status == "sent") &&
			agent.QueueRemoveTarget(c.Payload) == "GLOBAL-Internet" {
			other++
		}
	}
	st.Unlock()
	if other != 1 {
		t.Fatalf("coexistence des noms : 1 commande GLOBAL-Internet attendue, %d", other)
	}
}

// TestNormalizeCIDRList — N°110 — le contrat des cibles multiples : 1 à 4
// CIDR IPv4 séparés par des virgules, canonisés et dédoublonnés ; un seul
// élément invalide rejette TOUTE la liste (le gérant corrige sa saisie —
// jamais de demi-file sur un sous-réseau de moins).
func TestNormalizeCIDRList(t *testing.T) {
	ok := []struct{ in, want string }{
		{"192.168.10.0/24", "192.168.10.0/24"},                                                                             // simple : rétrocompatible
		{" 192.168.10.0/24 ", "192.168.10.0/24"},                                                                           // espaces tolérés
		{"192.168.10.0/24,10.77.0.0/21", "192.168.10.0/24,10.77.0.0/21"},                                                   // le cas ProMax
		{" 10.77.0.0/21 , 192.168.10.0/24 ", "10.77.0.0/21,192.168.10.0/24"},                                               // ordre de saisie conservé
		{"10.77.0.10/21,192.168.10.5/24", "10.77.0.0/21,192.168.10.0/24"},                                                  // canonisation des préfixes réseau
		{"192.168.10.0/24,192.168.10.0/24", "192.168.10.0/24"},                                                             // doublon dédoublonné
		{"10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,100.64.0.0/10", "10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,100.64.0.0/10"}, // 4 éléments
	}
	for _, c := range ok {
		if got, valid := normalizeCIDRList(c.in); !valid || got != c.want {
			t.Errorf("normalizeCIDRList(%q) = (%q, %v), attendu (%q, true)", c.in, got, valid, c.want)
		}
	}
	bad := []string{
		"",                              // vide
		",",                             // que des séparateurs
		"192.168.10.0/24,,10.77.0.0/21", // élément vide en milieu
		"192.168.10.0/24,fd00::/8",      // IPv6 dans la liste
		"192.168.10.0/24,not-a-cidr",    // élément invalide
		"192.168.10.0",                  // sans préfixe (régle inchangée)
		"10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,100.64.0.0/10,169.254.0.0/16", // 5 : trop
	}
	for _, in := range bad {
		if got, valid := normalizeCIDRList(in); valid {
			t.Errorf("normalizeCIDRList(%q) = (%q, true), attendu invalide", in, got)
		}
	}
}

// TestQoSMultiTarget — N°110 — le parcours doré du cas ProMax : PUT avec la
// liste des deux sous-réseaux → état désiré + queue_ensure au payload
// complet → rapport honnête (la relecture RouterOS peut RÉORDONNER la
// liste) → signature posée quand même : la vérification compare des
// ENSEMBLES, la file couvre exactement ce qui a été demandé.
func TestQoSMultiTarget(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	token, accID, _ := registerAccount(t, ts, "qos-multi-gerant", "")
	const tok = "q0s-t0ken-multi"
	seedAgentRouter(t, st, accID, "r-multi", "SITE MULTI", tok)

	// 1) Activation avec les deux sous-réseaux (pool étendu N°108 actif).
	status, out := doJSON(t, ts, "PUT", "/api/routers/r-multi/qos", token, map[string]any{
		"enabled": true, "target": "192.168.10.0/24,10.77.0.0/21",
		"maxUpBps": 19_000_000, "maxDownBps": 104_500_000,
	})
	if status != http.StatusOK {
		t.Fatalf("PUT qos multi-cibles : %d %v", status, out)
	}
	st.Lock()
	var target string
	var payload map[string]any
	for i := range st.Data().Routers {
		if st.Data().Routers[i].ID == "r-multi" {
			target = st.Data().Routers[i].QoSTarget
		}
	}
	for _, c := range st.Data().Commands {
		if c.Kind == model.CmdQueueEnsure && (c.Status == "queued" || c.Status == "sent") {
			payload = c.Payload
		}
	}
	st.Unlock()
	if target != "192.168.10.0/24,10.77.0.0/21" {
		t.Fatalf("cible stockée : %q", target)
	}
	if got, _ := payload["target"].(string); got != "192.168.10.0/24,10.77.0.0/21" {
		t.Fatalf("payload queue_ensure : cible %q", got)
	}

	// 2) Rapport honnête avec la liste RÉORDONNÉE par RouterOS : la vérité
	// est l'ensemble couvert, pas l'ordre des caractères.
	cmdID := ""
	st.Lock()
	for _, c := range st.Data().Commands {
		if c.Kind == model.CmdQueueEnsure && (c.Status == "queued" || c.Status == "sent") {
			cmdID = c.ID
		}
	}
	st.Unlock()
	agentReport(t, ts, tok, cmdID, url.Values{"status": {"ok"},
		"data": {"queue|mikcloud-qos|10.77.0.0/21,192.168.10.0/24|19M/104.5M|pcq-upload-default/pcq-download-default|false;"}})
	st.Lock()
	var sig string
	for i := range st.Data().Routers {
		if st.Data().Routers[i].ID == "r-multi" {
			sig = st.Data().Routers[i].QoSSig
		}
	}
	st.Unlock()
	if sig == "" {
		t.Fatalf("relecture réordonnée : la signature doit être posée (comparaison par ensemble)")
	}

	// 3) Une cible qui COUVRE MOINS (un seul des deux sous-réseaux) reste un
	// mensonge : sig vidée en store → check-in re-file queue_ensure →
	// rapport incomplet → la signature ne doit PAS revenir.
	st.Lock()
	for i := range st.Data().Routers {
		if st.Data().Routers[i].ID == "r-multi" {
			st.Data().Routers[i].QoSSig = ""
		}
	}
	st.Save()
	st.Unlock()
	body := agentCheckIn(t, ts, tok)
	driftID := deviceCmdID(t, body, model.CmdQueueEnsure)
	if driftID == "" {
		t.Fatalf("sig vide : re-file attendu :\n%s", preview(body, 400))
	}
	agentReport(t, ts, tok, driftID, url.Values{"status": {"ok"},
		"data": {"queue|mikcloud-qos|192.168.10.0/24|19M/104.5M|pcq-upload-default/pcq-download-default|false;"}})
	st.Lock()
	sig = ""
	for i := range st.Data().Routers {
		if st.Data().Routers[i].ID == "r-multi" {
			sig = st.Data().Routers[i].QoSSig
		}
	}
	st.Unlock()
	if sig != "" {
		t.Fatalf("cible incomplète : la signature ne doit PAS être posée")
	}
}

// TestQoSPutMultiTargetInvalid — la moindre cible invalide dans la liste
// rejette le PUT entier (400) : jamais de file à demi-posée.
func TestQoSPutMultiTargetInvalid(t *testing.T) {
	_, ts := newTestServerWithStore(t)
	token, _, _ := registerAccount(t, ts, "qos-multi-bad", "")
	status, out := doJSON(t, ts, "POST", "/api/routers", token, map[string]any{"name": "SITE BAD", "mode": "agent"})
	if status != http.StatusOK {
		t.Fatalf("création routeur : %d %v", status, out)
	}
	routerID, _ := out["id"].(string)
	for _, bad := range []string{
		"192.168.10.0/24,,10.77.0.0/21",                     // élément vide
		"192.168.10.0/24,fd00::/8",                          // IPv6
		"192.168.10.0/24,abc",                               // injection
		"1.0.0.0/8,2.0.0.0/8,3.0.0.0/8,4.0.0.0/8,5.0.0.0/8", // 5 éléments
	} {
		status, _ = doJSON(t, ts, "PUT", "/api/routers/"+routerID+"/qos", token, map[string]any{
			"enabled": true, "target": bad, "maxUpBps": 19_000_000, "maxDownBps": 104_500_000,
		})
		if status != http.StatusBadRequest {
			t.Fatalf("cible %q : statut %d attendu 400", bad, status)
		}
	}
}
