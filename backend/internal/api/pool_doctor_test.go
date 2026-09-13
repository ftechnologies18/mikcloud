package api

// Tests N°97 — docteur pool IP : parsing des ranges (capacité), convergence
// de l'auto-diagnostic au check-in, application du rapport, endpoint
// console (agent/simulated/real) et alerte moniteur.

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"mikcloud/hotspot-api/internal/agent"
	"mikcloud/hotspot-api/internal/model"
)

// ─── Calcul de capacité ───

func TestParsePoolCapacityRanges(t *testing.T) {
	pools := map[string]string{
		"hs-pool":   "10.5.50.2-10.5.50.254",
		"dhcp-pool": "192.168.88.10-192.168.88.254",
	}
	cap, ranges := ParsePoolCapacity(pools, []string{"hs-pool"})
	if cap != 253 {
		t.Fatalf("capacité range simple = %d, attendu 253", cap)
	}
	if ranges != "10.5.50.2-10.5.50.254" {
		t.Fatalf("ranges affichés = %q", ranges)
	}
	// Pool NON référencé : ignoré (capacité ne compte que le reachable).
	cap, _ = ParsePoolCapacity(pools, []string{"dhcp-pool"})
	if cap != 245 {
		t.Fatalf("capacité dhcp-pool = %d, attendu 245", cap)
	}
	// Multi-range + CIDR + dédoublonnage d'un même pool référencé 2 fois.
	pools["wide"] = "10.77.0.10-10.77.7.254,10.5.50.0/24"
	cap, _ = ParsePoolCapacity(pools, []string{"wide", "wide"})
	if cap != 2037+256 {
		t.Fatalf("capacité wide multi-range = %d, attendu %d", cap, 2037+256)
	}
	// Références absurdes : none, (unknown), pool inconnu → 0.
	cap, _ = ParsePoolCapacity(pools, []string{"none", "(unknown)", "ghost"})
	if cap != 0 {
		t.Fatalf("références invalides : capacité %d, attendu 0", cap)
	}
}

func TestRangeCapacityMalformed(t *testing.T) {
	for _, bad := range []string{"10.5.50.2", "a.b.c.d-e.f.g.h", "10.5.50.0/33", "10.5.50.0/31", "", "10.5.50.9-10.5.50.2"} {
		if n := rangeCapacity(bad); n != 0 {
			t.Errorf("rangeCapacity(%q) = %d, attendu 0", bad, n)
		}
	}
	if n := rangeCapacity("10.77.0.0/21"); n != 2048 {
		t.Errorf("rangeCapacity CIDR /21 = %d, attendu 2048", n)
	}
}

// ─── Auto-diagnostic du check-in ───

// poolDoctorRouter — routeur agent jamais diagnostiqué (PoolCap nul).
func poolDoctorRouter() *model.Router {
	return &model.Router{ID: "r-pd", AccountID: "acc-pd", Name: "Pooly", Mode: "agent", Version: "7.20 (stable)"}
}

func TestPoolDoctorEnsureQueuesWhenNeverDiagnosed(t *testing.T) {
	a := newWatchAPI()
	db := &model.DB{}
	router := poolDoctorRouter()

	a.ensurePoolDoctorLocked(db, router)
	cmd := findQueuedByKind(db, model.CmdPoolDoctor)
	if cmd == nil {
		t.Fatal("routeur jamais diagnostiqué : pool_doctor doit être en file")
	}
	// DIAGNOSTIC PUR : le check-in ne modifie jamais la configuration.
	if recycle, _ := cmd.Payload["recycle"].(bool); recycle {
		t.Error("l'auto-diagnostic ne doit pas recycler")
	}
	if extend, _ := cmd.Payload["extend"].(bool); extend {
		t.Error("l'auto-diagnostic ne doit pas étendre")
	}

	// En vol → pas de doublon.
	for i := range db.Commands {
		db.Commands[i].Status = "sent"
	}
	a.ensurePoolDoctorLocked(db, router)
	n := 0
	for _, c := range db.Commands {
		if c.Kind == model.CmdPoolDoctor {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("en vol : %d commandes pool_doctor, attendu 1", n)
	}
}

func TestPoolDoctorEnsureFreshStaysSilent(t *testing.T) {
	a := newWatchAPI()
	db := &model.DB{}
	router := poolDoctorRouter()
	router.PoolCap = 254
	router.PoolDoctorAt = model.NowISO()

	a.ensurePoolDoctorLocked(db, router)
	if len(db.Commands) != 0 {
		t.Fatal("diagnostic frais : aucune commande attendue")
	}

	// N°97-ter : diagnostic ABOUTI mais capacité nulle (hotspot sans pool
	// identifiable) → fraîcheur sur PoolDoctorAt SEUL : silence, sinon la
	// file et le journal sont inondés (une commande par check-in, ~180/h
	// avec le veilleur — constat production ProMax WIFI).
	router.PoolCap = 0
	db.Commands = nil
	a.ensurePoolDoctorLocked(db, router)
	if len(db.Commands) != 0 {
		t.Fatal("diagnostic frais sans capacité : AUCUNE commande attendue (anti-boucle N°97-ter)")
	}

	// Diagnostiqué il y a 8 jours (> refresh 7 j) → re-file.
	router.PoolDoctorAt = time.Now().UTC().Add(-8 * 24 * time.Hour).Format(time.RFC3339)
	a.ensurePoolDoctorLocked(db, router)
	if findQueuedByKind(db, model.CmdPoolDoctor) == nil {
		t.Fatal("diagnostic périmé : pool_doctor doit être re-filé")
	}
}

func TestPoolDoctorEnsureIgnoresSimulated(t *testing.T) {
	a := newWatchAPI()
	db := &model.DB{}
	router := poolDoctorRouter()
	router.Mode = "simulated"
	router.PoolCap = 0

	a.ensurePoolDoctorLocked(db, router)
	if len(db.Commands) != 0 {
		t.Fatal("routeur simulé : aucune commande agent attendue")
	}
}

// ─── Application du rapport ───

func TestApplyPoolDoctorReport(t *testing.T) {
	a := newWatchAPI()
	db := &model.DB{}
	router := poolDoctorRouter()

	vals := url.Values{}
	vals.Set("recycled", "yes")
	vals.Set("extended", "yes")
	vals.Set("hosts", "230")
	vals.Set("active", "120")
	vals.Set("pools", "hs-pool|10.5.50.2-10.5.50.254;dhcp|192.168.88.10-192.168.88.254")
	vals.Set("servers", "hotspot1|default|bridge-hotspot|5m|10m|2m")
	vals.Set("profiles", "default|hs-pool|2;vip|none|1")
	vals.Set("dhcp", "dhcp1|bridge-hotspot|hs-pool;dhcp-lan|ether2|dhcp")

	summary := a.applyPoolDoctor(db, router, vals)

	if router.PoolCap != 253 {
		t.Fatalf("PoolCap = %d, attendu 253 (pool référencé par le profil)", router.PoolCap)
	}
	if router.PoolHosts != 230 {
		t.Fatalf("PoolHosts = %d, attendu 230", router.PoolHosts)
	}
	if router.PoolRanges != "10.5.50.2-10.5.50.254" {
		t.Fatalf("PoolRanges = %q", router.PoolRanges)
	}
	if router.PoolDoctorAt == "" {
		t.Fatal("PoolDoctorAt doit être posé")
	}
	if usage := poolUsagePct(router); usage != 90 {
		t.Fatalf("usage = %d, attendu 90 (230/253)", usage)
	}
	for _, frag := range []string{"230/253", "90", "recyclage + extension"} {
		if !strings.Contains(summary, frag) {
			t.Errorf("résumé %q absent de %q", frag, summary)
		}
	}
}

// ─── Endpoint console ───

func TestRouterPoolDoctorEndpointAgentQueued(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	ownerToken, accID, _ := registerAccount(t, ts, "gerant-pool-ag", "")

	st.Lock()
	st.Data().Routers = append(st.Data().Routers, model.Router{
		ID: "r-pool-ag", AccountID: accID, Name: "Pool Agent", Mode: "agent", Status: "online",
	})
	st.Save()
	st.Unlock()

	// Recyclage seul.
	status, out := doJSON(t, ts, "POST", "/api/routers/r-pool-ag/pool-doctor", ownerToken, map[string]any{})
	if status != http.StatusOK {
		t.Fatalf("statut %d (%v), attendu 200", status, out)
	}
	if out["queued"] != true {
		t.Fatalf("réponse %v : queued attendu", out)
	}

	st.Lock()
	var cmd *model.Command
	for i := range st.Data().Commands {
		if st.Data().Commands[i].Kind == model.CmdPoolDoctor {
			cmd = &st.Data().Commands[i]
		}
	}
	st.Unlock()
	if cmd == nil {
		t.Fatal("commande pool_doctor non en file")
	}
	if recycle, _ := cmd.Payload["recycle"].(bool); !recycle {
		t.Error("endpoint console : recycle=true attendu par défaut")
	}
	if extend, _ := cmd.Payload["extend"].(bool); extend {
		t.Error("extend doit être opt-in (faux par défaut)")
	}

	// Extension opt-in.
	status, out = doJSON(t, ts, "POST", "/api/routers/r-pool-ag/pool-doctor", ownerToken, map[string]any{"extend": true})
	if status != http.StatusOK {
		t.Fatalf("statut %d (%v), attendu 200", status, out)
	}
	st.Lock()
	var extCmd *model.Command
	for i := range st.Data().Commands {
		if st.Data().Commands[i].Kind == model.CmdPoolDoctor && st.Data().Commands[i].Payload["extend"] == true {
			extCmd = &st.Data().Commands[i]
		}
	}
	st.Unlock()
	if extCmd == nil {
		t.Fatal("commande pool_doctor extend=true non en file")
	}
}

func TestRouterPoolDoctorEndpointSimulated(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	ownerToken, accID, _ := registerAccount(t, ts, "gerant-pool-sim", "")

	st.Lock()
	st.Data().Routers = append(st.Data().Routers, model.Router{
		ID: "r-pool-sim", AccountID: accID, Name: "Pool Simulé", Mode: "simulated", Status: "online", ActiveSessions: 100,
	})
	st.Save()
	st.Unlock()

	status, out := doJSON(t, ts, "POST", "/api/routers/r-pool-sim/pool-doctor", ownerToken, map[string]any{})
	if status != http.StatusOK {
		t.Fatalf("statut %d (%v), attendu 200", status, out)
	}
	if out["ok"] != true {
		t.Fatalf("réponse %v : ok attendu", out)
	}
	if cap := int(out["poolCap"].(float64)); cap != 254 {
		t.Fatalf("poolCap simulé = %v, attendu 254", out["poolCap"])
	}
	// hôtes = sessions + 40 % de zombies → 100 + 40.
	if hosts := int(out["poolHosts"].(float64)); hosts != 140 {
		t.Fatalf("poolHosts simulé = %v, attendu 140", out["poolHosts"])
	}
	if pct := int(out["usagePct"].(float64)); pct != 55 {
		t.Fatalf("usagePct simulé = %v, attendu 55", out["usagePct"])
	}
}

func TestRouterPoolDoctorEndpointRealMode(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	ownerToken, accID, _ := registerAccount(t, ts, "gerant-pool-real", "")

	st.Lock()
	st.Data().Routers = append(st.Data().Routers, model.Router{
		ID: "r-pool-real", AccountID: accID, Name: "Pool Direct", Mode: "real", Status: "online",
	})
	st.Save()
	st.Unlock()

	status, out := doJSON(t, ts, "POST", "/api/routers/r-pool-real/pool-doctor", ownerToken, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("statut %d (%v), attendu 400 (mode real)", status, out)
	}
}

func TestRouterPoolDoctorEndpointUnknownRouter(t *testing.T) {
	ts := newTestServer(t)
	token, _, _ := registerAccount(t, ts, "gerant-pool-404", "cle-x")
	status, _ := doJSON(t, ts, "POST", "/api/routers/r-inconnu/pool-doctor", token, nil)
	if status != http.StatusNotFound {
		t.Fatalf("statut %d, attendu 404", status)
	}
}

// read_state rapporte hosts → PoolHosts (test de bout en bout du paramètre).
func TestReadStateReportsPoolHosts(t *testing.T) {
	a := newWatchAPI()
	db := &model.DB{}
	router := poolDoctorRouter()

	vals := url.Values{}
	vals.Set("total", "3")
	vals.Set("start", "0")
	vals.Set("count", "500")
	vals.Set("stotal", "42")
	vals.Set("hosts", "217")
	vals.Set("users", "")
	a.applyReadState(db, router, vals)

	if router.PoolHosts != 217 {
		t.Fatalf("PoolHosts = %d, attendu 217 (rafraîchi par read_state)", router.PoolHosts)
	}
	if router.ActiveSessions != 42 {
		t.Fatalf("ActiveSessions = %d, attendu 42", router.ActiveSessions)
	}
}

// Script du docteur servi par le check-in : la file le délivre bien
// (ScriptFor câblé).
func TestPoolDoctorScriptForDispatched(t *testing.T) {
	b := agent.Builder{BaseURL: "https://cloud.example", Token: "tk"}
	script, err := b.ScriptFor(model.Command{ID: "c-x", Kind: model.CmdPoolDoctor, Payload: map[string]any{}})
	if err != nil {
		t.Fatalf("ScriptFor(pool_doctor) : %v", err)
	}
	if script == "" {
		t.Fatal("script vide")
	}
}

// TestApplyPoolDoctorProMaxCase — données RÉELLES de production (ProMax WIFI,
// 13/09 22:06) : profils hotspot SANS address-pool, capacité portée par le
// serveur DHCP du bridge (Hotspot-Pool sur Bridge-Hotspot). Avant N°97-ter :
// PoolCap=0 et re-diagnostic à chaque check-in (boucle 20 s) ; après : la
// capacité du DHCP de l'interface hotspot est comptée.
func TestApplyPoolDoctorProMaxCase(t *testing.T) {
	a := newWatchAPI()
	db := &model.DB{}
	router := poolDoctorRouter()

	vals := url.Values{}
	vals.Set("recycled", "no")
	vals.Set("extended", "no")
	vals.Set("hosts", "149")
	vals.Set("active", "61")
	vals.Set("pools", "Hotspot-Pool|192.168.100.10-192.168.100.250;Prive-Pool|192.168.88.10-192.168.88.250;")
	vals.Set("servers", "ProMax|ProMax_Hotspot|Bridge-Hotspot|none|30m|none;")
	vals.Set("profiles", "default||;ProMax_Hotspot||;")
	vals.Set("dhcp", "dhcp1|Bridge-Hotspot|Hotspot-Pool;dhcp-lan|ether2|Prive-Pool;")

	a.applyPoolDoctor(db, router, vals)

	if router.PoolCap != 241 {
		t.Fatalf("PoolCap = %d, attendu 241 (Hotspot-Pool 192.168.100.10-250 via le DHCP du bridge)", router.PoolCap)
	}
	if router.PoolRanges != "192.168.100.10-192.168.100.250" {
		t.Fatalf("PoolRanges = %q, attendu le range du DHCP hotspot", router.PoolRanges)
	}
	if usage := poolUsagePct(router); usage != 61 {
		t.Fatalf("usage = %d, attendu 61 (149/241)", usage)
	}
	// Le pool du DHCP d'une interface NON hotspot (ether2 → Prive-Pool)
	// ne doit PAS être compté : capacité du bridge LAN hors du périmètre.
	if strings.Contains(router.PoolRanges, "192.168.88") {
		t.Fatalf("le pool de l'interface non-hotspot ne doit pas apparaître : %q", router.PoolRanges)
	}
}
