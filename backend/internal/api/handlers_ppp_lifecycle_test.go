// Tests N°294 — PPPoE : cycle de vie de l'abonné (suspension auto ExpMode→
// disable, renouvellement F4 + réactivation, IP statique phase B, récurrent
// + rappels phase C, renfort tunnel temps réel + provisioning assisté).
//
// Discipline des tests du dépôt : surface HTTP RÉELLE (doJSON) pour les
// endpoints, passage commun enforceExpired appelé SOUS VERROU (comme le fait
// chaque lecture console), indirections (deliverNotif / dispatchEmailTask /
// pppLiveFetch / pppLiveKick) remplacées par des exécuteurs synchrones —
// AUCUN réseau, AUCUNE goroutine orpheline.
package api

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"mikcloud/hotspot-api/internal/model"
	"mikcloud/hotspot-api/internal/store"
)

// errDialFake — erreur de dial factice (comparable partout dans les tests).
var errDialFake = errors.New("dial factice : réseau injoignable")

// pppSeedSecret — injecte un abonné PPPoE directement dans le store.
func pppSeedSecret(t *testing.T, st *store.Store, s model.PppSecret) {
	t.Helper()
	st.Lock()
	if s.ID == "" {
		s.ID = model.NewID("pp-")
	}
	if s.Service == "" {
		s.Service = model.PppService
	}
	if s.State == "" {
		s.State = model.PppStateActive
	}
	if s.CreatedAt == "" {
		s.CreatedAt = model.NowISO()
	}
	s.UpdatedAt = model.NowISO()
	st.Data().PppSecrets = append(st.Data().PppSecrets, s)
	st.Save()
	st.Unlock()
}

// pppEnforce — exécute le passage commun sous verrou (mêmes conditions que
// chaque lecture console) et persiste.
func pppEnforce(t *testing.T, st *store.Store) {
	t.Helper()
	api := New(st, testJWTSecret)
	st.Lock()
	touched := store.NewTableSet()
	api.enforceExpired(st.Data(), touched)
	st.SaveTables(touched.Names()...)
	st.Unlock()
}

// pppCountCommands — commandes d'un kind données (prédicat libre) en file.
func pppCountCommands(st *store.Store, routerID, kind, secretID string, wantDisabled *bool) int {
	st.Lock()
	defer st.Unlock()
	n := 0
	for i := range st.Data().Commands {
		c := &st.Data().Commands[i]
		if c.RouterID != routerID || c.Kind != kind || c.Status != "queued" {
			continue
		}
		if secretID != "" {
			if sid, _ := c.Payload["secretId"].(string); sid != secretID {
				continue
			}
		}
		if wantDisabled != nil {
			if dis, _ := c.Payload["disabled"].(bool); dis != *wantDisabled {
				continue
			}
		}
		n++
	}
	return n
}

// TestPppAutoSuspension — suspension auto (ExpMode « disable », défaut) :
// échéance dépassée → AutoSuspended + commande {disabled:true, auto:true} ;
// ExpMode « none » → jamais suspendu ; idempotence (Enforced) — le passage
// suivant ne re-file RIEN.
func TestPppAutoSuspension(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	_, accID, _ := registerAccount(t, ts, "ppp-susp", "")
	seedPppAgentRouter(t, st, accID, "rt-ppp-s", "POP Suspension", "tok-s")

	past := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339)
	pppSeedSecret(t, st, model.PppSecret{ID: "pp-susp1", AccountID: accID, RouterID: "rt-ppp-s",
		Name: "abdou@fai.ci", Profile: "default", ExpiresAt: past})
	pppSeedSecret(t, st, model.PppSecret{ID: "pp-susp2", AccountID: accID, RouterID: "rt-ppp-s",
		Name: "illimite@fai.ci", Profile: "default", ExpiresAt: past, ExpMode: model.PppExpModeNone})

	pppEnforce(t, st)

	st.Lock()
	s1 := model.FindPppSecretScoped(st.Data(), "pp-susp1", accID)
	s2 := model.FindPppSecretScoped(st.Data(), "pp-susp2", accID)
	if s1 == nil || s2 == nil {
		t.Fatal("secrets semés introuvables")
	}
	if !s1.AutoSuspended || !s1.Enforced || s1.Disabled {
		t.Errorf("suspension auto attendue sur pp-susp1 : autoSuspended=%v enforced=%v disabled=%v", s1.AutoSuspended, s1.Enforced, s1.Disabled)
	}
	if s2.AutoSuspended || s2.Enforced {
		t.Errorf("ExpMode none : pp-susp2 ne doit pas être suspendu (auto=%v enforced=%v)", s2.AutoSuspended, s2.Enforced)
	}
	st.Unlock()

	yes := true
	if n := pppCountCommands(st, "rt-ppp-s", model.CmdPppSecretSet, "pp-susp1", &yes); n != 1 {
		t.Errorf("commande disable auto attendue pour pp-susp1, trouvée %d", n)
	}
	if n := pppCountCommands(st, "rt-ppp-s", model.CmdPppSecretSet, "pp-susp2", &yes); n != 0 {
		t.Errorf("aucune commande attendue pour pp-susp2 (none), trouvée %d", n)
	}

	// Idempotence : un second passage ne re-file rien (Enforced posé).
	before := pppCountCommands(st, "rt-ppp-s", model.CmdPppSecretSet, "pp-susp1", &yes)
	pppEnforce(t, st)
	if after := pppCountCommands(st, "rt-ppp-s", model.CmdPppSecretSet, "pp-susp1", &yes); after != before {
		t.Errorf("le passage suivant ne doit rien re-filer (%d → %d)", before, after)
	}
}

// TestPppRenewReactivatesAutoSuspended — F4 : le renouvellement prolonge
// (base = max(maintenant, échéance)), réactive un suspendu AUTO et file le
// re-enable (avec purge des disables auto encore en file) ; une suspension
// MANUELLE reste (décision humaine respectée).
func TestPppRenewReactivatesAutoSuspended(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	ownerToken, accID, _ := registerAccount(t, ts, "ppp-renew", "")
	seedPppAgentRouter(t, st, accID, "rt-ppp-r", "POP Renew", "tok-r")

	past := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)
	pppSeedSecret(t, st, model.PppSecret{ID: "pp-rn1", AccountID: accID, RouterID: "rt-ppp-r",
		Name: "auto@fai.ci", Profile: "default", ExpiresAt: past, Disabled: true, AutoSuspended: true})
	pppSeedSecret(t, st, model.PppSecret{ID: "pp-rn2", AccountID: accID, RouterID: "rt-ppp-r",
		Name: "manuel@fai.ci", Profile: "default", ExpiresAt: past, Disabled: true, AutoSuspended: false})

	// 1. Renouvellement du suspendu AUTO : 200, réactivation + re-enable.
	status, out := doJSON(t, ts, "POST", "/api/ppp/secrets/pp-rn1/renew", ownerToken, map[string]any{"days": 30})
	if status != 200 {
		t.Fatalf("renew : statut %d, corps %v", status, out)
	}
	st.Lock()
	s1 := model.FindPppSecretScoped(st.Data(), "pp-rn1", accID)
	if s1 == nil {
		st.Unlock()
		t.Fatal("pp-rn1 introuvable")
	}
	if s1.Disabled || s1.AutoSuspended || !s1.Enforced {
		t.Errorf("pp-rn1 doit être réactivé (disabled=%v auto=%v enforced=%v)", s1.Disabled, s1.AutoSuspended, s1.Enforced)
	}
	exp1, err := time.Parse(time.RFC3339, s1.ExpiresAt)
	st.Unlock()
	if err != nil || !exp1.After(time.Now().UTC().Add(29*24*time.Hour)) {
		t.Errorf("pp-rn1 : échéance future attendue (~+30 j), %v", s1.ExpiresAt)
	}
	no := false
	if n := pppCountCommands(st, "rt-ppp-r", model.CmdPppSecretSet, "pp-rn1", &no); n != 1 {
		t.Errorf("commande re-enable attendue pour pp-rn1, trouvée %d", n)
	}

	// 2. Renouvellement du suspendu MANUEL : le disable reste.
	if status, out := doJSON(t, ts, "POST", "/api/ppp/secrets/pp-rn2/renew", ownerToken, map[string]any{"days": 30}); status != 200 {
		t.Fatalf("renew manuel : statut %d, %v", status, out)
	}
	st.Lock()
	s2 := model.FindPppSecretScoped(st.Data(), "pp-rn2", accID)
	okDisabled := s2 != nil && s2.Disabled && !s2.AutoSuspended
	st.Unlock()
	if !okDisabled {
		t.Error("pp-rn2 (suspendu manuellement) doit rester suspendu après renouvellement")
	}
	if n := pppCountCommands(st, "rt-ppp-r", model.CmdPppSecretSet, "pp-rn2", &no); n != 0 {
		t.Errorf("aucune commande re-enable attendue pour pp-rn2, trouvée %d", n)
	}

	// 3. days invalides → 400.
	if status, _ := doJSON(t, ts, "POST", "/api/ppp/secrets/pp-rn1/renew", ownerToken, map[string]any{"days": 0}); status != 400 {
		t.Errorf("days 0 : 400 attendu, %d", status)
	}

	// 4. Un disable AUTO encore EN FILE est purgé par le renouvellement
	// (le routeur ne doit jamais exécuter une suspension périmée).
	st.Lock()
	queueCommandLocked(st.Data(), accID, "rt-ppp-r", model.CmdPppSecretSet,
		map[string]any{"secretId": "pp-rn1", "name": "auto@fai.ci", "disabled": true, "auto": true})
	st.Save()
	st.Unlock()
	yes := true
	if n := pppCountCommands(st, "rt-ppp-r", model.CmdPppSecretSet, "pp-rn1", &yes); n != 1 {
		t.Fatalf("disable auto en file attendu avant renouvellement, trouvé %d", n)
	}
	if status, out := doJSON(t, ts, "POST", "/api/ppp/secrets/pp-rn1/renew", ownerToken, map[string]any{"days": 30}); status != 200 {
		t.Fatalf("renew avec disable en file : statut %d, %v", status, out)
	}
	if n := pppCountCommands(st, "rt-ppp-r", model.CmdPppSecretSet, "pp-rn1", &yes); n != 0 {
		t.Errorf("le disable auto en file doit être purgé, restant %d", n)
	}
}

// TestPppAutoRenewRecurring — phase C : à l'échéance, AutoRenew prolonge
// (base = max(maintenant, échéance) + RenewDays) au lieu de suspendre,
// réarme le rappel et trace l'activité ; aucun encaissement n'est déclenché.
func TestPppAutoRenewRecurring(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	_, accID, _ := registerAccount(t, ts, "ppp-rec", "")
	seedPppAgentRouter(t, st, accID, "rt-ppp-c", "POP Recurrent", "tok-c")

	past := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339)
	pppSeedSecret(t, st, model.PppSecret{ID: "pp-rec1", AccountID: accID, RouterID: "rt-ppp-c",
		Name: "prepaye@fai.ci", Profile: "default", ExpiresAt: past, AutoRenew: true, RenewDays: 30})

	pppEnforce(t, st)

	st.Lock()
	s := model.FindPppSecretScoped(st.Data(), "pp-rec1", accID)
	if s == nil {
		st.Unlock()
		t.Fatal("pp-rec1 introuvable")
	}
	exp, err := time.Parse(time.RFC3339, s.ExpiresAt)
	okWindow := err == nil && exp.After(time.Now().UTC().Add(29*24*time.Hour)) && exp.Before(time.Now().UTC().Add(31*24*time.Hour))
	if !okWindow {
		t.Errorf("échéance prolongée attendue (~+30 j), obtenue %v (err %v)", s.ExpiresAt, err)
	}
	notSuspended := !s.Disabled && !s.AutoSuspended && s.Enforced
	activityAuto := false
	for _, a := range st.Data().Activity {
		if strings.Contains(a.Message, "renouvelé automatiquement") && strings.Contains(a.Message, "prepaye@fai.ci") {
			activityAuto = true
		}
	}
	st.Unlock()
	if !notSuspended {
		t.Error("récurrent : pas de suspension attendue (disabled/auto/enforced)")
	}
	if !activityAuto {
		t.Error("activité « renouvelé automatiquement » attendue")
	}
	suspendu := true
	if cmds := pppCountCommands(st, "rt-ppp-c", model.CmdPppSecretSet, "pp-rec1", &suspendu); cmds != 0 {
		t.Errorf("aucune commande disable attendue en récurrent, trouvée %d", cmds)
	}
}

// TestPppReminderDispatch — phase C : le rappel d'échéance part quand
// maintenant ≥ échéance − RemindDays, dédupliqué par RemindedAt, et NE part
// PAS quand aucun canal n'est configuré (pas de notification perdue).
func TestPppReminderDispatch(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	ownerToken, accID, _ := registerAccount(t, ts, "ppp-rapp", "")
	seedPppAgentRouter(t, st, accID, "rt-ppp-m", "POP Rappel", "tok-m")

	in3h := time.Now().UTC().Add(3 * time.Hour).Format(time.RFC3339)
	pppSeedSecret(t, st, model.PppSecret{ID: "pp-rap1", AccountID: accID, RouterID: "rt-ppp-m",
		Name: "bientot@fai.ci", Profile: "default", ExpiresAt: in3h, RemindDays: 3})

	// Sans canal configuré : pas de rappel marqué, pas d'envoi.
	pppEnforce(t, st)
	st.Lock()
	s := model.FindPppSecretScoped(st.Data(), "pp-rap1", accID)
	remindedBefore := s.RemindedAt
	st.Unlock()
	if remindedBefore != "" {
		t.Fatal("sans canal configuré, le rappel ne doit pas être marqué envoyé")
	}

	// Canal Telegram configuré + exécuteurs déterministes (goroutine + WaitGroup).
	st.Lock()
	cfg := store.GetOrCreateNotifSettings(st.Data(), accID)
	cfg.Enabled = true
	cfg.TelegramEnabled = true
	cfg.TelegramBotToken = "tok-test"
	cfg.TelegramChatID = "42"
	store.SetNotifSettings(st.Data(), cfg)
	st.Save()
	st.Unlock()

	oldDispatch := dispatchEmailTask
	oldDeliver := deliverNotif
	t.Cleanup(func() { dispatchEmailTask = oldDispatch; deliverNotif = oldDeliver })
	// Exécuteur DÉTERMINISTE mais réel : goroutine + WaitGroup — la tâche
	// re-verrouille le store pour consigner NotifLog (comme en prod) ; elle ne
	// doit JAMAIS partager le goroutine du passage commun (sous verrou).
	var wg sync.WaitGroup
	dispatchEmailTask = func(fn func()) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fn()
		}()
	}
	var mu sync.Mutex
	var captured []string
	deliverNotif = func(cfg *model.NotificationSettings, platform *model.NotificationSettings, kind, title, body, onlyChannel string) []model.NotificationLog {
		mu.Lock()
		defer mu.Unlock()
		captured = append(captured, kind+"|"+title)
		return []model.NotificationLog{{Channel: "telegram", Status: "sent", Kind: kind, Title: title}}
	}

	pppEnforce(t, st)
	wg.Wait() // les envois (et leur consignation) doivent être terminés
	mu.Lock()
	capturedNow := append([]string(nil), captured...)
	mu.Unlock()
	st.Lock()
	s = model.FindPppSecretScoped(st.Data(), "pp-rap1", accID)
	reminded := s.RemindedAt
	st.Unlock()
	if reminded == "" {
		t.Fatal("le rappel doit être marqué envoyé (RemindedAt posé)")
	}
	if len(capturedNow) != 1 || !strings.Contains(capturedNow[0], "ppp_reminder") || !strings.Contains(capturedNow[0], "bientot@fai.ci") {
		t.Fatalf("rappel attendu exactement une fois avec le nom de l'abonné, obtenu %v", capturedNow)
	}

	// Dédup : un second passage ne renvoie PAS (RemindedAt posé).
	pppEnforce(t, st)
	wg.Wait()
	mu.Lock()
	capturedAfter := len(captured)
	mu.Unlock()
	if capturedAfter != 1 {
		t.Errorf("dédup attendu : 1 envoi, obtenu %d", capturedAfter)
	}

	// Un abonné suspendu ne reçoit pas de rappel.
	pppSeedSecret(t, st, model.PppSecret{ID: "pp-rap2", AccountID: accID, RouterID: "rt-ppp-m",
		Name: "suspendu@fai.ci", Profile: "default", ExpiresAt: in3h, RemindDays: 3, Disabled: true})
	pppEnforce(t, st)
	wg.Wait()
	mu.Lock()
	capturedFinal := len(captured)
	mu.Unlock()
	if capturedFinal != 1 {
		t.Errorf("aucun rappel pour un suspendu, obtenu %d envois", capturedFinal)
	}
	_ = ownerToken
}

// TestPppStaticAddressEndpoints — phase B : IP statique à la création
// (unicité PAR ROUTEUR — 409), PATCH avec remise au pool ("" →
// remoteAddress présent dans le payload), validations (IP invalide → 400).
func TestPppStaticAddressEndpoints(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	ownerToken, accID, _ := registerAccount(t, ts, "ppp-ip", "")
	seedPppAgentRouter(t, st, accID, "rt-ppp-ip", "POP IP", "tok-ip")

	s1, out := doJSON(t, ts, "POST", "/api/routers/rt-ppp-ip/ppp/secrets", ownerToken, map[string]string{
		"name": "static@fai.ci", "profile": "default", "staticAddress": "10.10.0.25",
	})
	if s1 != 201 {
		t.Fatalf("création avec IP statique : statut %d, %v", s1, out)
	}
	sec, _ := out["secret"].(map[string]any)
	if got, _ := sec["staticAddress"].(string); got != "10.10.0.25" {
		t.Errorf("staticAddress attendu 10.10.0.25, obtenu %q", got)
	}
	// Collision sur le MÊME routeur → 409.
	if status, _ := doJSON(t, ts, "POST", "/api/routers/rt-ppp-ip/ppp/secrets", ownerToken, map[string]string{
		"name": "autre@fai.ci", "profile": "default", "staticAddress": "10.10.0.25",
	}); status != 409 {
		t.Errorf("collision IP : 409 attendu, %d", status)
	}
	// IP invalide → 400.
	if status, _ := doJSON(t, ts, "POST", "/api/routers/rt-ppp-ip/ppp/secrets", ownerToken, map[string]string{
		"name": "bad@fai.ci", "profile": "default", "staticAddress": "10.0.00.1",
	}); status != 400 {
		t.Errorf("IP invalide : 400 attendu, %d", status)
	}

	secID, _ := sec["id"].(string)
	// PATCH : remise au pool ("" présent) → payload remoteAddress="".
	status, out2 := doJSON(t, ts, "PATCH", "/api/ppp/secrets/"+secID, ownerToken, map[string]any{"staticAddress": ""})
	if status != 200 {
		t.Fatalf("PATCH remise pool : statut %d, %v", status, out2)
	}
	st.Lock()
	found := false
	for i := range st.Data().Commands {
		c := &st.Data().Commands[i]
		if c.Kind == model.CmdPppSecretSet && c.Status == "queued" {
			if sid, _ := c.Payload["secretId"].(string); sid == secID {
				if v, ok := c.Payload["remoteAddress"].(string); ok && v == "" {
					found = true
				}
			}
		}
	}
	s := model.FindPppSecretScoped(st.Data(), secID, accID)
	clearOK := s != nil && s.StaticAddress == ""
	st.Unlock()
	if !found {
		t.Error("payload remoteAddress=\"\" attendu (remise au pool)")
	}
	if !clearOK {
		t.Error("staticAddress cloud attendu vidé")
	}
	// PATCH collision avec un autre abonné → 409.
	doJSON(t, ts, "POST", "/api/routers/rt-ppp-ip/ppp/secrets", ownerToken, map[string]string{
		"name": "autre@fai.ci", "profile": "default", "staticAddress": "10.10.0.30",
	})
	if status, _ := doJSON(t, ts, "PATCH", "/api/ppp/secrets/"+secID, ownerToken, map[string]any{"staticAddress": "10.10.0.30"}); status != 409 {
		t.Errorf("PATCH collision IP : 409 attendu, %d", status)
	}
}

// TestPppApiCredsLifecycle — phase B : creds API du renfort (stockage sealed
// existant) : PUT → GET (JAMAIS de mot de passe en réponse) → DELETE ; le
// db.json persisté ne contient JAMAIS le mot de passe en clair (secretbox).
func TestPppApiCredsLifecycle(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("ADMIN_PASSWORD", "admin-test-1234")
	t.Setenv("ADMIN_USERNAME", "")
	dir := t.TempDir()
	st, err := store.New(dir)
	if err != nil {
		t.Fatalf("store.New : %v", err)
	}
	ts := startTestServer(st)
	t.Cleanup(ts.Close)
	ownerToken, accID, _ := registerAccount(t, ts, "ppp-creds", "")
	seedPppAgentRouter(t, st, accID, "rt-ppp-cr", "POP Creds", "tok-cr")

	// GET initial : non configuré.
	status, out := doJSON(t, ts, "GET", "/api/routers/rt-ppp-cr/ppp/api-creds", ownerToken, nil)
	if status != 200 || out["configured"] != false {
		t.Fatalf("GET initial : statut %d, %v", status, out)
	}
	// PUT : 200.
	if status, out = doJSON(t, ts, "PUT", "/api/routers/rt-ppp-cr/ppp/api-creds", ownerToken, map[string]string{
		"username": "api-gest", "password": "pw-api-secret",
	}); status != 200 {
		t.Fatalf("PUT creds : statut %d, %v", status, out)
	}
	// GET : configuré, username visible, mot de passe ABSENT.
	if status, out = doJSON(t, ts, "GET", "/api/routers/rt-ppp-cr/ppp/api-creds", ownerToken, nil); status != 200 || out["configured"] != true {
		t.Fatalf("GET après PUT : statut %d, %v", status, out)
	}
	if _, leak := out["password"]; leak {
		t.Fatalf("le mot de passe ne doit JAMAIS sortir en réponse : %v", out)
	}
	if got, _ := out["username"].(string); got != "api-gest" {
		t.Errorf("username attendu api-gest, obtenu %q", got)
	}
	// Le db.json persisté ne porte PAS le mot de passe en clair (secretbox).
	st.Save()
	raw, err := os.ReadFile(filepath.Join(dir, "db.json"))
	if err != nil {
		t.Fatalf("lecture db.json : %v", err)
	}
	if strings.Contains(string(raw), "pw-api-secret") {
		t.Fatal("fuite : le mot de passe API est en clair dans db.json (secretbox attendu)")
	}
	// DELETE : 200 + non configuré.
	if status, _ = doJSON(t, ts, "DELETE", "/api/routers/rt-ppp-cr/ppp/api-creds", ownerToken, nil); status != 200 {
		t.Fatalf("DELETE creds : statut %d", status)
	}
	if status, out = doJSON(t, ts, "GET", "/api/routers/rt-ppp-cr/ppp/api-creds", ownerToken, nil); status != 200 || out["configured"] != false {
		t.Fatalf("GET après DELETE : statut %d, %v", status, out)
	}
	// Validation : username hostile → 400.
	if status, _ = doJSON(t, ts, "PUT", "/api/routers/rt-ppp-cr/ppp/api-creds", ownerToken, map[string]string{
		"username": `bad"user`, "password": "pw",
	}); status != 400 {
		t.Errorf("username hostile : 400 attendu, %d", status)
	}
}

// TestPppLiveEndpoints — phase B : temps réel via tunnel avec les
// indirections remplacées (aucun réseau). Gates : sans tunnel / sans creds →
// 409 explicites ; dial échoué → 502 ; chemins heureux → 200.
func TestPppLiveEndpoints(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	ownerToken, accID, _ := registerAccount(t, ts, "ppp-live", "")
	seedPppAgentRouter(t, st, accID, "rt-ppp-lv", "POP Live", "tok-lv")

	// Gate : ni tunnel ni creds → 409.
	pppSeedSecret(t, st, model.PppSecret{ID: "pp-lv1", AccountID: accID, RouterID: "rt-ppp-lv",
		Name: "live@fai.ci", Profile: "default"})
	if status, out := doJSON(t, ts, "POST", "/api/routers/rt-ppp-lv/ppp/live/active", ownerToken, nil); status != 409 {
		t.Fatalf("gate sans tunnel : 409 attendu, %d (%v)", status, out)
	}

	// Tunnel + creds posés directement (état sealed — la console passe par PUT).
	st.Lock()
	for i := range st.Data().Routers {
		if st.Data().Routers[i].ID == "rt-ppp-lv" {
			st.Data().Routers[i].WgIPv4 = "10.8.0.9"
			st.Data().Routers[i].Username = "api-gest"
			st.Data().Routers[i].Password = "pw-api"
		}
	}
	st.Save()
	st.Unlock()

	oldFetch, oldKick := pppLiveFetch, pppLiveKick
	t.Cleanup(func() { pppLiveFetch, pppLiveKick = oldFetch, oldKick })
	pppLiveFetch = func(ipv4, user, pass string) ([]map[string]string, error) {
		if ipv4 != "10.8.0.9" || user != "api-gest" || pass != "pw-api" {
			t.Errorf("creds attendues (10.8.0.9/api-gest), obtenues (%s/%s)", ipv4, user)
		}
		return []map[string]string{
			{".id": "*1", "name": "live@fai.ci", "service": "pppoe", "caller-id": "AA:BB:CC:DD:EE:01", "address": "10.10.0.25", "uptime": "3d21h14m56s"},
		}, nil
	}
	pppLiveKick = func(ipv4, user, pass, name string) error {
		if name != "live@fai.ci" {
			t.Errorf("kick : nom attendu live@fai.ci, obtenu %q", name)
		}
		return nil
	}

	// Sessions live : 200, ligne mappée.
	status, out := doJSON(t, ts, "POST", "/api/routers/rt-ppp-lv/ppp/live/active", ownerToken, nil)
	if status != 200 {
		t.Fatalf("live/active : statut %d, %v", status, out)
	}
	rows, _ := out["data"].([]any)
	if len(rows) != 1 {
		t.Fatalf("1 session attendue, %v", out["data"])
	}
	r0, _ := rows[0].(map[string]any)
	if r0["name"] != "live@fai.ci" || r0["address"] != "10.10.0.25" {
		t.Errorf("ligne live inattendue : %v", r0)
	}
	// Kick-live : 200.
	if status, out = doJSON(t, ts, "POST", "/api/ppp/secrets/pp-lv1/kick-live", ownerToken, nil); status != 200 {
		t.Fatalf("kick-live : statut %d, %v", status, out)
	}
	// Erreur dial → 502 (honnête, creds jamais dans le message).
	pppLiveFetch = func(string, string, string) ([]map[string]string, error) {
		return nil, errDialFake
	}
	if status, out = doJSON(t, ts, "POST", "/api/routers/rt-ppp-lv/ppp/live/active", ownerToken, nil); status != 502 {
		t.Fatalf("dial échoué : 502 attendu, %d (%v)", status, out)
	}
	if msg, _ := out["message"].(string); strings.Contains(msg, "pw-api") {
		t.Error("les credentials ne doivent jamais apparaître dans les messages d'erreur")
	}
	// Kick en échec → 502 avec le recours agent rappelé.
	pppLiveKick = func(string, string, string, string) error { return errDialFake }
	if status, out = doJSON(t, ts, "POST", "/api/ppp/secrets/pp-lv1/kick-live", ownerToken, nil); status != 502 {
		t.Fatalf("kick échoué : 502 attendu, %d (%v)", status, out)
	}
}

// errDialFake — voir déclaration en tête.

// TestPppProvisioningScript — phase B : génération du livrable .rsc
// idempotent (gardes [:len] = 0), validations strictes des paramètres.
func TestPppProvisioningScript(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	ownerToken, accID, _ := registerAccount(t, ts, "ppp-prov", "")
	seedPppAgentRouter(t, st, accID, "rt-ppp-pv", "POP Provisioning", "tok-pv")

	status, out := doJSON(t, ts, "GET",
		"/api/routers/rt-ppp-pv/ppp/provisioning-script?interface=ether2&service=mikcloud&profile=mikcloud-ppp&poolStart=10.10.0.10&poolEnd=10.10.0.30&dns=8.8.8.8,1.1.1.1",
		ownerToken, nil)
	if status != 200 {
		t.Fatalf("provisioning : statut %d, %v", status, out)
	}
	script, _ := out["script"].(string)
	for _, want := range []string{
		`/ip pool`,
		`ranges="10.10.0.10-10.10.0.30"`,
		`service-name="mikcloud"`,
		`interface=ether2`,
		`default-profile="mikcloud-ppp"`,
		`dns-server=8.8.8.8,1.1.1.1`,
		"[:len [find name=\"mikcloud-ppp-pool\"]] = 0", // garde idempotence
	} {
		if !strings.Contains(script, want) {
			t.Errorf("script provisioning : %q absent", want)
		}
	}
	// Validations : interface absente, plage inversée, DNS invalide → 400.
	cases := []struct{ nom, qs string }{
		{"sans interface", "service=mikcloud&poolStart=10.10.0.10&poolEnd=10.10.0.30"},
		{"plage inversée", "interface=ether2&poolStart=10.10.0.30&poolEnd=10.10.0.10"},
		{"DNS invalide", "interface=ether2&poolStart=10.10.0.10&poolEnd=10.10.0.30&dns=abc"},
		{"interface hostile", "interface=ether2;bad&poolStart=10.10.0.10&poolEnd=10.10.0.30"},
	}
	for _, c := range cases {
		if status, _ := doJSON(t, ts, "GET", "/api/routers/rt-ppp-pv/ppp/provisioning-script?"+c.qs, ownerToken, nil); status != 400 {
			t.Errorf("%s : 400 attendu, %d", c.nom, status)
		}
	}
	// Routeur inconnu → 404 (périmètre compte).
	if status, _ := doJSON(t, ts, "GET", "/api/routers/rt-inconnu/ppp/provisioning-script?interface=ether2&poolStart=10.10.0.10&poolEnd=10.10.0.30", ownerToken, nil); status != 404 {
		t.Errorf("routeur inconnu : 404 attendu, %d", status)
	}
}
