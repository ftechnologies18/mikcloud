package api

// Tests N°106 — mode bridage (quota data) : convergence du scheduler
// mikcloud-quota au check-in (pattern watcher N°77), validations du CRUD
// profils (quotaMode/throttleRate) et flag « session bridée » du rapport
// read_state (paramètre throttle= : noms des files mikthrottle- présentes).

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"mikcloud/hotspot-api/internal/agent"
	"mikcloud/hotspot-api/internal/model"
)

// newQuotaAPI — API minimale pour les tests de convergence (même patron
// que newWatchAPI).
func newQuotaAPI() *API {
	return &API{attn: map[string]time.Time{}}
}

// quotaRouter — routeur agent antérieur au N°106 (QuotaSchedOK absent = faux).
func quotaRouter() *model.Router {
	return &model.Router{ID: "r-quota", AccountID: "acc-quota", Name: "Throttly", Mode: "agent", Version: "7.20 (stable)", SchedulerSec: agentSleepSec}
}

// TestQuotaEnsureQueuedWhenThrottleProfile — la convergence ne file
// quota_ensure QUE si le compte possède au moins un profil en mode
// throttle : un site qui ne bridle jamais ne consomme rien.
func TestQuotaEnsureQueuedWhenThrottleProfile(t *testing.T) {
	a := newQuotaAPI()
	db := &model.DB{}
	router := quotaRouter()

	// Aucun profil throttle → silence absolu (économie N°75 entière).
	db.Profiles = append(db.Profiles, model.Profile{ID: "p-1", AccountID: "acc-quota", Name: "cut-1h", QuotaMode: model.QuotaModeCut})
	a.ensureQuotaThrottleLocked(db, router)
	if findQueuedByKind(db, model.CmdQuotaEnsure) != nil {
		t.Fatal("aucun profil throttle : quota_ensure ne doit PAS être filé")
	}

	// Un profil throttle du compte → mise en file.
	db.Profiles = append(db.Profiles, model.Profile{ID: "p-2", AccountID: "acc-quota", Name: "soft-1h", QuotaMode: model.QuotaModeThrottle})
	a.ensureQuotaThrottleLocked(db, router)
	if findQueuedByKind(db, model.CmdQuotaEnsure) == nil {
		t.Fatal("profil throttle présent : quota_ensure doit être en file")
	}

	// Déjà en file → pas de doublon.
	a.ensureQuotaThrottleLocked(db, router)
	n := 0
	for _, c := range db.Commands {
		if c.Kind == model.CmdQuotaEnsure {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("quota_ensure dupliqué : %d commandes, attendu 1", n)
	}

	// En vol (« sent ») → pas de doublon non plus.
	for i := range db.Commands {
		db.Commands[i].Status = "sent"
	}
	a.ensureQuotaThrottleLocked(db, router)
	for _, c := range db.Commands {
		if c.Kind == model.CmdQuotaEnsure && c.Status == "queued" {
			t.Fatal("déploiement en vol : aucun nouveau quota_ensure ne doit être filé")
		}
	}

	// Drapeau posé → silence définitif.
	router.QuotaSchedOK = true
	db.Commands = nil
	a.ensureQuotaThrottleLocked(db, router)
	if findQueuedByKind(db, model.CmdQuotaEnsure) != nil {
		t.Fatal("QuotaSchedOK posé : aucun quota_ensure ne doit être re-filé")
	}

	// Un profil throttle d'un AUTRE compte ne compte pas (isolation).
	router.QuotaSchedOK = false
	db.Commands = nil
	db.Profiles = []model.Profile{{ID: "p-x", AccountID: "acc-autre", Name: "soft-x", QuotaMode: model.QuotaModeThrottle}}
	a.ensureQuotaThrottleLocked(db, router)
	if findQueuedByKind(db, model.CmdQuotaEnsure) != nil {
		t.Fatal("profil throttle d'un autre compte : aucun quota_ensure attendu")
	}

	// Un routeur simulé n'est pas concerné.
	db.Profiles = append(db.Profiles, model.Profile{ID: "p-2", AccountID: "acc-quota", Name: "soft-1h", QuotaMode: model.QuotaModeThrottle})
	router.Mode = "simulated"
	a.ensureQuotaThrottleLocked(db, router)
	if findQueuedByKind(db, model.CmdQuotaEnsure) != nil {
		t.Fatal("routeur simulé : aucun quota_ensure ne doit être filé")
	}
}

// TestQuotaEnsureScriptShape — le script servi déploie le scheduler
// mikcloud-quota : remove-then-add idempotent, tick 20 s échappé dans
// on-event, rapport standard au cloud.
func TestQuotaEnsureScriptShape(t *testing.T) {
	b := agent.Builder{BaseURL: "https://cloud.exemple", Token: "qt0ken-abcdefghijklmnop"}
	script, err := b.ScriptFor(model.Command{ID: "c-q1", Kind: model.CmdQuotaEnsure, Payload: map[string]any{}})
	if err != nil {
		t.Fatalf("ScriptFor(quota_ensure) : %v", err)
	}
	for _, want := range []string{
		`name="` + agent.QuotaSchedName + `"`,
		`interval=` + "20" + `s`,
		`start-time=startup`,
		// Le tick (variables $…) est échappé pour l'attribut on-event="…".
		`\$qn2`,
		`mikthrottle-`,
		`/agent/result?token=`,
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("quota_ensure : %q absent du script", want)
		}
	}
	// Le remove précède l'add (remove-then-add idempotent).
	if idxRemove, idxAdd := strings.Index(script, `/system scheduler remove`), strings.Index(script, `/system scheduler add`); idxRemove < 0 || idxAdd < 0 || idxRemove > idxAdd {
		t.Fatalf("ordre remove-then-add non respecté (remove=%d, add=%d)", idxRemove, idxAdd)
	}
}

// TestProfileQuotaModeValidation — le CRUD profils refuse un mode bridage
// sans débit ni quota (config à moitié posée), accepte la forme complète,
// et bascule proprement cut ↔ throttle.
func TestProfileQuotaModeValidation(t *testing.T) {
	ts := newTestServer(t)
	status, out := doJSON(t, ts, "POST", "/api/auth/login", "", map[string]string{
		"username": "admin", "password": "admin-test-1234",
	})
	if status != http.StatusOK {
		t.Fatalf("login admin : statut %d (%v)", status, out)
	}
	token, _ := out["token"].(string)
	if token == "" {
		t.Fatal("token admin absent")
	}

	// Throttle sans débit → 400.
	status, out = doJSON(t, ts, "POST", "/api/profiles", token, map[string]any{
		"name": "soft-1h", "dataQuotaMb": 1024, "quotaMode": "throttle",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("throttle sans débit : statut %d attendu 400 (%v)", status, out)
	}

	// Throttle sans quota data → 400.
	status, out = doJSON(t, ts, "POST", "/api/profiles", token, map[string]any{
		"name": "soft-1h", "dataQuotaMb": 0, "quotaMode": "throttle", "throttleRate": "512k/512k",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("throttle sans quota : statut %d attendu 400 (%v)", status, out)
	}

	// Mode inconnu → 400.
	status, out = doJSON(t, ts, "POST", "/api/profiles", token, map[string]any{
		"name": "soft-1h", "quotaMode": "explose",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("mode inconnu : statut %d attendu 400 (%v)", status, out)
	}

	// Forme complète → 201, champs persistés.
	status, out = doJSON(t, ts, "POST", "/api/profiles", token, map[string]any{
		"name": "soft-1h", "dataQuotaMb": 1024, "quotaMode": "throttle", "throttleRate": "512k/512k",
		"sessionTimeoutMin": 60,
	})
	if status != http.StatusCreated {
		t.Fatalf("profil throttle complet : statut %d (%v)", status, out)
	}
	profID, _ := out["id"].(string)
	if out["quotaMode"] != model.QuotaModeThrottle || out["throttleRate"] != "512k/512k" {
		t.Fatalf("champs N°106 non sérialisés : quotaMode=%v throttleRate=%v", out["quotaMode"], out["throttleRate"])
	}

	// Bascule throttle → cut toujours permise (retour arrière franc).
	status, _ = doJSON(t, ts, "PUT", "/api/profiles/"+profID, token, map[string]any{
		"quotaMode": "cut",
	})
	if status != http.StatusOK {
		t.Fatalf("bascule cut : statut %d", status)
	}

	// Re-bascule cut → throttle avec le QUOTA HÉRITÉ (1024 Mo, non effacé
	// par le retour en cut) : l'état futur est valide → 200. Seul le débit
	// doit être fourni.
	status, out = doJSON(t, ts, "PUT", "/api/profiles/"+profID, token, map[string]any{
		"quotaMode": "throttle", "throttleRate": "1M",
	})
	if status != http.StatusOK {
		t.Fatalf("throttle avec quota hérité : statut %d attendu 200 (%v)", status, out)
	}
	if out["quotaMode"] != model.QuotaModeThrottle || out["throttleRate"] != "1M" {
		t.Fatalf("état futur non appliqué : %v / %v", out["quotaMode"], out["throttleRate"])
	}

	// Passage throttle avec quota RAMENÉ À 0 dans la même requête : l'état
	// futur est invalide (bridage sans quota) → 400.
	status, _ = doJSON(t, ts, "PUT", "/api/profiles/"+profID, token, map[string]any{
		"quotaMode": "throttle", "throttleRate": "1M", "dataQuotaMb": 0,
	})
	if status != http.StatusBadRequest {
		t.Fatalf("throttle avec quota 0 : statut %d attendu 400", status)
	}

	// État final sain : cut (retour franc toujours permis).
	status, out = doJSON(t, ts, "PUT", "/api/profiles/"+profID, token, map[string]any{
		"quotaMode": "cut", "dataQuotaMb": 2048,
	})
	if status != http.StatusOK {
		t.Fatalf("retour cut final : statut %d", status)
	}
	if out["quotaMode"] != model.QuotaModeCut {
		t.Fatalf("état final attendu cut, obtenu %v", out["quotaMode"])
	}
}

// TestReadStateThrottleFlag — le rapport read_state pose Session.Throttled
// depuis le paramètre throttle= (noms des files mikthrottle-, vérité
// routeur) ; un rapport vide (routeur sans bridage) laisse le drapeau faux.
func TestReadStateThrottleFlag(t *testing.T) {
	a := newQuotaAPI()
	db := &model.DB{}
	router := quotaRouter()

	// Un utilisateur connu du cloud (pour l'appariement user).
	db.HotspotUsers = append(db.HotspotUsers, model.HotspotUser{
		ID: "u-1", AccountID: "acc-quota", Username: "ticket-1", ProfileName: "soft-1h", RouterID: "r-quota",
	})

	vals := url.Values{}
	vals.Set("version", "7.20 (stable)")
	vals.Set("uptime", "12m")
	vals.Set("cpu", "5")
	vals.Set("freemem", "100000000")
	vals.Set("totalmem", "200000000")
	vals.Set("board", "RB941")
	vals.Set("freehdd", "10")
	vals.Set("totalhdd", "20")
	vals.Set("total", "1")
	vals.Set("start", "0")
	vals.Set("count", "500")
	vals.Set("out", "1")
	vals.Set("users", "ticket-1|soft-1h|false;")
	vals.Set("stotal", "2")
	vals.Set("hosts", "2")
	vals.Set("sessions", "ticket-1|10.5.50.7|3m|1000|2000;ticket-2|10.5.50.8|1m|10|20;")
	// La file mikthrottle-ticket-1 existe sur le routeur.
	vals.Set("throttle", "ticket-1,")
	vals.Set("trunc", "false")

	a.applyReadState(db, router, vals)

	if len(db.Sessions) != 2 {
		t.Fatalf("2 sessions attendues, obtenu %d", len(db.Sessions))
	}
	found := false
	for _, s := range db.Sessions {
		if s.Username == "ticket-1" {
			found = true
			if !s.Throttled {
				t.Fatal("ticket-1 doit être marqué bridée (file mikthrottle-ticket-1 présente)")
			}
		}
		if s.Username == "ticket-2" && s.Throttled {
			t.Fatal("ticket-2 ne doit pas être marqué bridée (aucune file)")
		}
	}
	if !found {
		t.Fatal("session ticket-1 absente du rapport appliqué")
	}

	// Rapport suivant SANS bridage (file retirée — reset-counters) : le
	// drapeau retombe (l'appariement FIFO conserve l'identité, l'état est
	// recalculé depuis la vérité routeur).
	vals.Set("throttle", "")
	a.applyReadState(db, router, vals)
	for _, s := range db.Sessions {
		if s.Username == "ticket-1" && s.Throttled {
			t.Fatal("après retrait de la file : le drapeau doit retomber")
		}
	}
}
