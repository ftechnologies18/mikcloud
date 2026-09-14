package api

// Tests N°103 — Qualité de ligne (mesure passive du débit FAI), phase 1 :
//   - script read_state : la détection WAN (route par défaut active,
//     « reachable via <iface> ») est rapportée par le paramètre wan ;
//   - rapport agent : WanIface posé sur le routeur (et un silence — wan vide —
//     n'efface jamais la vérité précédente) ;
//   - télémétrie : chaque fenêtre de mesure alimente les agrégats quotidiens
//     (AccumulateLineQuality depuis applyAgentTraffic) ;
//   - endpoint GET /api/routers/{id}/line-quality : WAN, jours triés du plus
//     récent, enveloppe mesurée (jours éclos qualifiés uniquement — le jour en
//     cours ne compte pas, un jour creux non plus), capacité déclarée, débit
//     live de l'interface WAN ;
//   - capacité déclarée : PUT /api/routers/{id} (bornes 0-10 Gbps, 0 = effacer).

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"mikcloud/hotspot-api/internal/agent"
	"mikcloud/hotspot-api/internal/model"
)

// TestReadStateScriptReportsWAN — le script généré porte la détection WAN :
// lecture de la route par défaut active + paramètre &wan= dans le rapport.
func TestReadStateScriptReportsWAN(t *testing.T) {
	b := agent.Builder{BaseURL: "https://cloud.example", Token: "tok"}
	script, err := b.ScriptFor(model.Command{ID: "c-wan", Kind: model.CmdReadState})
	if err != nil {
		t.Fatalf("script read_state : %v", err)
	}
	for _, want := range []string{
		`[/ip route find where dst-address="0.0.0.0/0"]`, // routes par défaut
		`[:find $gs "via "]`,                             // interface de la passerelle ACTIVE
		`:set rwan $w`,                                   // extraction du nom
		`."&wan=". $rwan`,                                // rapport au cloud
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("script read_state N°103 : fragment %q absent du script généré", want)
		}
	}
}

// TestApplyReadStateSetsWanIface — le rapport agent pose l'interface WAN ;
// un rapport SANS wan (routeur antérieur, route publiée absente) laisse la
// valeur précédente intacte.
func TestApplyReadStateSetsWanIface(t *testing.T) {
	a := &API{}
	db := &model.DB{}
	router := &model.Router{ID: "r-wan", Mode: "agent"}
	a.applyReadState(db, router, url.Values{"wan": {"pppoe-out1"}})
	if router.WanIface != "pppoe-out1" {
		t.Fatalf("WanIface = %q, attendu pppoe-out1", router.WanIface)
	}
	// Silence : pas d'effacement (la vérité d'un rapport récent ne doit pas
	// céder à l'absence de signal d'un rapport ancien).
	a.applyReadState(db, router, url.Values{})
	if router.WanIface != "pppoe-out1" {
		t.Fatalf("wan absent du rapport : WanIface doit rester pppoe-out1, obtenu %q", router.WanIface)
	}
	// Valeur parasite surdimensionnée : ignorée.
	a.applyReadState(db, router, url.Values{"wan": {strings.Repeat("x", 40)}})
	if router.WanIface != "pppoe-out1" {
		t.Fatalf("wan parasite : WanIface doit rester pppoe-out1, obtenu %q", router.WanIface)
	}
}

// TestApplyAgentTrafficFeedsLineQuality — chaque fenêtre de mesure alimente
// l'agrégat du jour : échantillon, max par direction, histogramme relu en p95.
// La PREMIÈRE mesure (pas de référence → débits 0) ne compte PAS.
func TestApplyAgentTrafficFeedsLineQuality(t *testing.T) {
	db := &model.DB{}
	router := &model.Router{ID: "r-lq2", AccountID: "acc-lq2", Mode: "agent", WanIface: "ether1"}
	t0 := time.Date(2026, 9, 14, 10, 0, 0, 0, time.UTC)

	// Fenêtre 1 : référence posée (aucun échantillon de ligne).
	applyAgentTraffic(db, router, "ether1:1000000:500000;", t0)
	if len(db.LineQuality) != 0 {
		t.Fatalf("première mesure : aucun échantillon attendu, %d lignes", len(db.LineQuality))
	}
	// Fenêtre 2 (120 s plus tard) : 150 Mo reçus → 10 Mbps ; 30 Mo émis → 2 Mbps.
	applyAgentTraffic(db, router, "ether1:151000000:30500000;", t0.Add(120*time.Second))
	if len(db.LineQuality) != 1 {
		t.Fatalf("1 ligne d'agrégat attendue, %d", len(db.LineQuality))
	}
	lq := db.LineQuality[0]
	if lq.Iface != "ether1" || lq.Day != "2026-09-14" || lq.Samples != 1 {
		t.Fatalf("agrégat mal scopé : %+v", lq)
	}
	if lq.RxMaxBps != 10_000_000 || lq.TxMaxBps != 2_000_000 {
		t.Fatalf("débits de fenêtre : rx=%d tx=%d, attendus 10000000/2000000", lq.RxMaxBps, lq.TxMaxBps)
	}
	if got := model.LineQualityHistPercentile(lq.RxHist, 95); got != 10_000_000 {
		t.Fatalf("p95 rx = %d, attendu 10000000 (seau 5-10M)", got)
	}
}

// TestRouterLineQualityEndpoint — la carte complète : jours triés du plus
// récent au plus ancien, enveloppe mesurée sur les jours ÉCLOS qualifiés
// (le jour en cours et les jours creux exclus), capacité déclarée, live WAN.
func TestRouterLineQualityEndpoint(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	token, accID, _ := registerAccount(t, ts, "lq-gerant", "")

	// Routeur agent avec WAN détecté + capacité déclarée + trois jours éclos
	// qualifiés dans l'agrégat + un jour creux (ne doit pas qualifier).
	status, out := doJSON(t, ts, "POST", "/api/routers", token, map[string]any{
		"name": "SITE LQ", "mode": "agent",
	})
	if status != http.StatusOK { // le contrat existant répond 200 (script d'installation inclus)
		t.Fatalf("création routeur : statut %d corps %v", status, out)
	}
	routerID, _ := out["id"].(string)
	now := time.Now().UTC()
	st.Lock()
	for i := range st.Data().Routers {
		rr := &st.Data().Routers[i]
		if rr.ID == routerID {
			rr.WanIface = "ether1"
			rr.LineDownBps = 110_000_000
			rr.LineUpBps = 20_000_000
		}
	}
	st.Data().LineQuality = []model.LineQualityDay{
		{ID: "lq-today", RouterID: routerID, AccountID: accID, Day: model.LineQualityDayKey(now), Iface: "ether1",
			Samples: 200, RxMaxBps: 120_000_000, TxMaxBps: 25_000_000, RxHist: "0,0,0,0,0,0,0,0,0,0,0,2,0,0,0,0"},
		{ID: "lq-d1", RouterID: routerID, AccountID: accID, Day: model.LineQualityDayKey(now.AddDate(0, 0, -1)), Iface: "ether1",
			Samples: 300, RxMaxBps: 96_000_000, TxMaxBps: 18_000_000, RxHist: "0,0,0,0,0,0,0,0,0,15,270,15,0,0,0,0"},
		{ID: "lq-d2", RouterID: routerID, AccountID: accID, Day: model.LineQualityDayKey(now.AddDate(0, 0, -2)), Iface: "ether1",
			Samples: 250, RxMaxBps: 80_000_000, TxMaxBps: 19_000_000, RxHist: "0,0,0,0,0,0,0,0,0,15,223,12,0,0,0,0"},
		{ID: "lq-d3", RouterID: routerID, AccountID: accID, Day: model.LineQualityDayKey(now.AddDate(0, 0, -3)), Iface: "ether1",
			Samples: 280, RxMaxBps: 70_000_000, TxMaxBps: 12_000_000, RxHist: "0,0,0,0,0,0,0,0,0,10,270,0,0,0,0,0"},
		// Jour creux (samples < 50) : visible dans days, exclu de l'enveloppe.
		{ID: "lq-d4", RouterID: routerID, AccountID: accID, Day: model.LineQualityDayKey(now.AddDate(0, 0, -4)), Iface: "ether1",
			Samples: 12, RxMaxBps: 40_000_000, TxMaxBps: 5_000_000, RxHist: "0,0,0,0,0,0,0,0,0,0,1,0,0,0,0,0"},
		// Interface locale : hors WAN, jamais servie par l'endpoint.
		{ID: "lq-lan", RouterID: routerID, AccountID: accID, Day: model.LineQualityDayKey(now.AddDate(0, 0, -1)), Iface: "bridge",
			Samples: 300, RxMaxBps: 999_000_000, TxMaxBps: 999_000_000, RxHist: "0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,1"},
	}
	st.Data().Traffic = []model.RouterTraffic{{
		ID: routerID, RouterID: routerID, AccountID: accID, UpdatedAt: now.Format(time.RFC3339Nano),
		Interfaces: []model.IfaceTraffic{
			{Name: "ether1", RxBps: 42_000_000, TxBps: 9_000_000},
			{Name: "bridge", RxBps: 1_000_000, TxBps: 1_000_000},
		},
	}}
	st.Save()
	st.Unlock()

	status, out = doJSON(t, ts, "GET", "/api/routers/"+routerID+"/line-quality", token, nil)
	if status != http.StatusOK {
		t.Fatalf("line-quality : statut %d corps %v", status, out)
	}
	if out["wanIface"] != "ether1" {
		t.Fatalf("wanIface = %v, attendu ether1", out["wanIface"])
	}
	cfg, _ := out["configured"].(map[string]any)
	if cfg["downBps"] != float64(110_000_000) || cfg["upBps"] != float64(20_000_000) {
		t.Fatalf("capacité déclarée : %v", cfg)
	}
	days, _ := out["days"].([]any)
	if len(days) != 5 {
		t.Fatalf("5 jours attendus (aujourd'hui + 4 éclos), %d obtenus", len(days))
	}
	first, _ := days[0].(map[string]any)
	if first["day"] != model.LineQualityDayKey(now) {
		t.Fatalf("jour le plus récent d'abord, obtenu %v", first["day"])
	}
	measured, _ := out["measured"].(map[string]any)
	// Enveloppe = jours éclos qualifiés : max des max (d1 96M/18M) — le jour en
	// cours (120M) et le jour creux (40M) sont exclus, bridge ignoré.
	if measured["downBps"] != float64(96_000_000) || measured["upBps"] != float64(19_000_000) {
		t.Fatalf("enveloppe mesurée : %v", measured)
	}
	// p95 (résolution = borne du seau) : d1 — 15 échantillons à 75-100M
	// (= 5 % exact, l'excess), le reste à 50-75M → 75M ; d2 — 12 à 75-100M
	// (5 % de 250) → 75M ; d3 — max 70M, tout le monde vit dans (50M, 75M]
	// → 75M. Le p95 régulier (75M) se détache du max (96M) : c'est le but.
	if measured["p95DownBps"] != float64(75_000_000) {
		t.Fatalf("p95 descendant mesuré : %v, attendu 75000000", measured["p95DownBps"])
	}
	if measured["days"] != float64(3) || measured["confident"] != true {
		t.Fatalf("jours qualifiés/confiance : %v", measured)
	}
	live, _ := out["live"].(map[string]any)
	if live["rxBps"] != float64(42_000_000) || live["txBps"] != float64(9_000_000) {
		t.Fatalf("débit live WAN : %v", live)
	}
}

// TestRouterLineQualityWithoutWan — WAN non détecté : la réponse reste honnête
// (aucune interface rattachée, aucune mesure), jamais d'erreur.
func TestRouterLineQualityWithoutWan(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	token, _, _ := registerAccount(t, ts, "lq-nowan", "")
	status, out := doJSON(t, ts, "POST", "/api/routers", token, map[string]any{
		"name": "SITE NOWAN", "mode": "agent",
	})
	if status != http.StatusOK { // le contrat existant répond 200 (script d'installation inclus)
		t.Fatalf("création routeur : statut %d corps %v", status, out)
	}
	routerID, _ := out["id"].(string)
	st.Lock()
	st.Data().LineQuality = []model.LineQualityDay{{
		ID: "lq-x", RouterID: routerID, Day: "2026-09-14", Iface: "ether1",
		Samples: 100, RxMaxBps: 50_000_000, RxHist: "0,0,0,0,0,0,0,0,0,0,1,0,0,0,0,0",
	}}
	st.Save()
	st.Unlock()

	status, out = doJSON(t, ts, "GET", "/api/routers/"+routerID+"/line-quality", token, nil)
	if status != http.StatusOK {
		t.Fatalf("line-quality sans WAN : statut %d corps %v", status, out)
	}
	if out["wanIface"] != "" {
		t.Fatalf("wanIface = %v, attendu vide (non détecté)", out["wanIface"])
	}
	days, _ := out["days"].([]any)
	if len(days) != 0 {
		t.Fatalf("aucune interface rattachée : days doit être vide, %d entrées", len(days))
	}
	measured, _ := out["measured"].(map[string]any)
	if measured["days"] != float64(0) || measured["confident"] != false {
		t.Fatalf("mesure sans WAN : %v", measured)
	}
}

// TestRouterUpdateLineCapacity — capacité déclarée par le gérant : mise à jour
// (bits/s), effacement (0), bornes (négatif et > 10 Gbps refusés).
func TestRouterUpdateLineCapacity(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	token, _, _ := registerAccount(t, ts, "lq-cap-gerant", "")
	status, out := doJSON(t, ts, "POST", "/api/routers", token, map[string]any{
		"name": "SITE CAP", "mode": "agent",
	})
	if status != http.StatusOK { // le contrat existant répond 200 (script d'installation inclus)
		t.Fatalf("création routeur : statut %d corps %v", status, out)
	}
	routerID, _ := out["id"].(string)

	// Déclaration : Orange CI 110M/20M — PAR ROUTEUR (chaque site a son FAI).
	status, out = doJSON(t, ts, "PUT", "/api/routers/"+routerID, token, map[string]any{
		"lineDownBps": 110_000_000, "lineUpBps": 20_000_000,
	})
	if status != http.StatusOK {
		t.Fatalf("déclaration capacité : statut %d corps %v", status, out)
	}
	st.Lock()
	var got model.Router
	for _, rr := range st.Data().Routers {
		if rr.ID == routerID {
			got = rr
		}
	}
	st.Unlock()
	if got.LineDownBps != 110_000_000 || got.LineUpBps != 20_000_000 {
		t.Fatalf("capacité persistée : %+v", got)
	}

	// Effacement : 0 = plus aucune déclaration.
	status, _ = doJSON(t, ts, "PUT", "/api/routers/"+routerID, token, map[string]any{
		"lineDownBps": 0, "lineUpBps": 0,
	})
	if status != http.StatusOK {
		t.Fatalf("effacement capacité : statut %d", status)
	}
	st.Lock()
	for _, rr := range st.Data().Routers {
		if rr.ID == routerID {
			got = rr
		}
	}
	st.Unlock()
	if got.LineDownBps != 0 || got.LineUpBps != 0 {
		t.Fatalf("effacement capacité : %+v", got)
	}

	// Bornes : négatif et > 10 Gbps refusés (erreur d'unité la plus courante).
	for _, bad := range []int64{-5, 11_000_000_000} {
		status, out = doJSON(t, ts, "PUT", "/api/routers/"+routerID, token, map[string]any{
			"lineDownBps": bad,
		})
		if status != http.StatusBadRequest {
			t.Fatalf("capacité %d : statut %d attendu 400, corps %v", bad, status, out)
		}
	}
}
