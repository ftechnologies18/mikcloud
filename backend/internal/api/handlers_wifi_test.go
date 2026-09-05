// Tests N°27 « WiFi Jetable » : flux public complet (claim idempotent,
// plafonds téléphone/site, garde abonnement, validations) + console
// (CRUD, slug unique, isolation multi-tenant, registre/CSV).
//
// Méthode : surface HTTP réelle (httptest.Server sur api.Handler(), store
// JSON éphémère) ; routeur SIMULÉ — le push gw.AddUser insère réellement le
// voucher dans le store, le flux de claim est donc testé de bout en bout.
package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mikcloud/hotspot-api/internal/model"
	"mikcloud/hotspot-api/internal/store"
)

// seedWifiEnv — routeur simulé + profil « WiFi Offert » (prix 0) pour un compte.
func seedWifiEnv(t *testing.T, st *store.Store, accID string) (string, string) {
	t.Helper()
	st.Lock()
	defer st.Unlock()
	db := st.Data()
	routerID, profileID := "rt-wifi-test", "pf-wifi-test"
	db.Routers = append(db.Routers, model.Router{
		ID: routerID, AccountID: accID, Name: "Routeur Maquis", Host: "10.5.50.1",
		Mode: "simulated", Status: "online",
		HotspotLoginUrl: "http://10.5.50.1/login", CreatedAt: model.NowISO(),
	})
	db.Profiles = append(db.Profiles, model.Profile{
		ID: profileID, AccountID: accID, Name: "WiFi Offert",
		SessionTimeoutMin: 30, SharedUsers: 1, ValidityDays: 1, Price: 0,
		CreatedAt: model.NowISO(),
	})
	return routerID, profileID
}

// seedWifiSite — site WiFi jetable complet.
func seedWifiSite(t *testing.T, st *store.Store, accID, slug, routerID, profileID string, active bool, perPhone, cap int) model.WifiSite {
	t.Helper()
	st.Lock()
	defer st.Unlock()
	db := st.Data()
	site := model.WifiSite{
		ID: "ws-" + slug, AccountID: accID, Name: "Maquis Chez Fofi", Slug: slug,
		RouterID: routerID, RouterName: "Routeur Maquis",
		ProfileID: profileID, ProfileName: "WiFi Offert",
		FreeTimeMin: 30, FreeDataMb: 100,
		MarketingOptIn: true, DailyPerPhone: perPhone, DailyCap: cap,
		Active: active, CreatedAt: model.NowISO(),
	}
	db.WifiSites = append(db.WifiSites, site)
	return site
}

// newWifiTestServer — serveur + store (l'éphemère est nécessaire au semis
// direct des routeurs/profils/sites, comme dans les tests Lots).
func newWifiTestServer(t *testing.T) (*httptest.Server, *store.Store) {
	t.Helper()
	t.Setenv("DATABASE_URL", "")
	t.Setenv("ADMIN_PASSWORD", "admin-test-1234")
	t.Setenv("ADMIN_USERNAME", "")
	t.Setenv("ALLOWED_ORIGIN", "")
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatalf("store.New impossible : %v", err)
	}
	ts := startTestServer(st)
	t.Cleanup(ts.Close)
	return ts, st
}

func TestWifiSiteInfoPublic(t *testing.T) {
	ts, st := newWifiTestServer(t)
	_, accID, _ := registerAccount(t, ts, "gerant-wifi-info", "")
	routerID, profileID := seedWifiEnv(t, st, accID)
	seedWifiSite(t, st, accID, "maquis-fofi", routerID, profileID, true, 1, 100)

	status, out := doJSON(t, ts, "GET", "/api/wifi/site/maquis-fofi", "", nil)
	if status != http.StatusOK {
		t.Fatalf("info site : statut %d, corps %v", status, out)
	}
	if name, _ := out["name"].(string); name != "Maquis Chez Fofi" {
		t.Fatalf("nom du site inattendu : %v", out["name"])
	}
	if active, _ := out["active"].(bool); !active {
		t.Fatal("le site doit être actif")
	}
	if tm, _ := out["freeTimeMin"].(float64); tm != 30 {
		t.Fatalf("quota temps affiché = %v, voulu 30", out["freeTimeMin"])
	}
	// Aucune fuite : pas d'identifiant de routeur ni de profil dans la réponse.
	for _, leak := range []string{"routerId", "profileId", "siteId"} {
		if _, ok := out[leak]; ok {
			t.Fatalf("fuite de donnée sensible %q dans la réponse publique", leak)
		}
	}

	// Slug inconnu → 404 propre.
	if status, _ := doJSON(t, ts, "GET", "/api/wifi/site/inconnu", "", nil); status != http.StatusNotFound {
		t.Fatalf("slug inconnu : statut %d, voulu 404", status)
	}
}

func TestWifiClaimFlowIdempotenceAndCaps(t *testing.T) {
	ts, st := newWifiTestServer(t)
	_, accID, _ := registerAccount(t, ts, "gerant-wifi-claim", "")
	routerID, profileID := seedWifiEnv(t, st, accID)
	seedWifiSite(t, st, accID, "cafe-treich", routerID, profileID, true, 1, 100)

	claim := func(phone string) (int, map[string]any) {
		return doJSON(t, ts, "POST", "/api/wifi/site/cafe-treich/claim", "", map[string]any{
			"phone": phone, "optIn": true,
		})
	}

	// Premier claim : un code est délivré avec le lien de connexion.
	status, out := claim("2250707080909")
	if status != http.StatusOK {
		t.Fatalf("claim 1 : statut %d, corps %v", status, out)
	}
	code1, _ := out["code"].(string)
	if len(code1) != 5 {
		t.Fatalf("code délivré invalide : %q", code1)
	}
	loginURL, _ := out["loginUrl"].(string)
	if !strings.Contains(loginURL, "username="+code1) || !strings.Contains(loginURL, "password="+code1) {
		t.Fatalf("lien de connexion invalide : %q", loginURL)
	}
	if strings.HasPrefix(loginURL, "https") && !strings.Contains(loginURL, "10.5.50.1") {
		t.Fatalf("lien inattendu : %q", loginURL)
	}
	if dup, _ := out["duplicate"].(bool); dup {
		t.Fatal("le premier claim ne doit pas être un duplicata")
	}
	// Quotas gratuits poussés au ticket.
	if tm, _ := out["timeLimitMin"].(float64); tm != 30 {
		t.Fatalf("limit-uptime = %v, voulu 30", out["timeLimitMin"])
	}
	if dm, _ := out["dataQuotaMb"].(float64); dm != 100 {
		t.Fatalf("limit-bytes = %v, voulu 100", out["dataQuotaMb"])
	}
	// Le ticket existe réellement (push simulateur) : user = mdp = code.
	st.Lock()
	found := false
	for _, u := range st.Data().HotspotUsers {
		if u.Username == code1 && u.Password == code1 && u.BatchID != "" && strings.HasPrefix(u.BatchID, "W") {
			found = true
		}
	}
	st.Unlock()
	if !found {
		t.Fatalf("voucher %q introuvable dans le store (lot W attendu)", code1)
	}

	// Idempotence : même téléphone ⇒ MÊME code, aucune seconde émission.
	status, out = claim("2250707080909")
	if status != http.StatusOK {
		t.Fatalf("claim 2 (idempotent) : statut %d", status)
	}
	if dup, _ := out["duplicate"].(bool); !dup {
		t.Fatal("le re-claim doit être marqué duplicate")
	}
	if c, _ := out["code"].(string); c != code1 {
		t.Fatalf("re-claim a changé le code : %q ≠ %q", c, code1)
	}

	// Autre téléphone : nouvelle émission (perPhone=1 respecté car code1 encore actif).
	status, out = claim("2250707111222")
	if status != http.StatusOK {
		t.Fatalf("claim téléphone 2 : statut %d", status)
	}
	if c, _ := out["code"].(string); c == code1 {
		t.Fatal("deux téléphones différents doivent recevoir des codes différents")
	}

	// Registre marketing : 2 entrées, opt-in tracé.
	st.Lock()
	guests := len(st.Data().WifiGuests)
	optIns := 0
	for _, g := range st.Data().WifiGuests {
		if g.OptIn {
			optIns++
		}
	}
	st.Unlock()
	if guests != 2 || optIns != 2 {
		t.Fatalf("registre : %d entrées (%d opt-in), voulu 2 (2)", guests, optIns)
	}

	// Plafond site : budget 2 → le troisième téléphone est refusé.
	st.Lock()
	for i := range st.Data().WifiSites {
		if st.Data().WifiSites[i].Slug == "cafe-treich" {
			st.Data().WifiSites[i].DailyCap = 2
		}
	}
	st.Unlock()
	if status, out = claim("2250707333444"); status != http.StatusTooManyRequests {
		t.Fatalf("cap site : statut %d, voulu 429 (corps %v)", status, out)
	}
	if code, _ := out["code"].(string); code == "site_cap" {
		t.Log("code machine site_cap bien renvoyé")
	} else {
		t.Fatalf("code machine attendu site_cap, reçu %v", out["code"])
	}
}

func TestWifiClaimPhoneCapAfterExpiry(t *testing.T) {
	ts, st := newWifiTestServer(t)
	_, accID, _ := registerAccount(t, ts, "gerant-wifi-cap", "")
	routerID, profileID := seedWifiEnv(t, st, accID)
	seedWifiSite(t, st, accID, "salon-koko", routerID, profileID, true, 1, 100)

	claim := func(phone string) (int, map[string]any) {
		return doJSON(t, ts, "POST", "/api/wifi/site/salon-koko/claim", "", map[string]any{
			"phone": phone, "optIn": false,
		})
	}
	if status, _ := claim("2250505060606"); status != http.StatusOK {
		t.Fatalf("claim initial : statut %d", status)
	}
	// Le voucher du jour expire (quota temps consommé côté moteur) → le
	// re-claim ne renvoie PLUS le même code : plafond téléphone atteint.
	st.Lock()
	for i := range st.Data().HotspotUsers {
		st.Data().HotspotUsers[i].UptimeUsedSec = st.Data().HotspotUsers[i].TimeLimitMin * 60
	}
	st.Unlock()
	if status, out := claim("2250505060606"); status != http.StatusTooManyRequests {
		t.Fatalf("phone_cap après expiration : statut %d, voulu 429 (corps %v)", status, out)
	}
}

func TestWifiClaimValidations(t *testing.T) {
	ts, st := newWifiTestServer(t)
	_, accID, _ := registerAccount(t, ts, "gerant-wifi-valid", "")
	routerID, profileID := seedWifiEnv(t, st, accID)
	seedWifiSite(t, st, accID, "buvette-yop", routerID, profileID, false, 1, 100)

	// Téléphone invalide → 400.
	if status, _ := doJSON(t, ts, "POST", "/api/wifi/site/buvette-yop/claim", "", map[string]any{
		"phone": "12", "optIn": false,
	}); status != http.StatusBadRequest {
		t.Fatalf("téléphone invalide : statut %d, voulu 400", status)
	}
	// Slug inconnu → 404.
	if status, _ := doJSON(t, ts, "POST", "/api/wifi/site/nullepart/claim", "", map[string]any{
		"phone": "2250707080909", "optIn": false,
	}); status != http.StatusNotFound {
		t.Fatalf("slug inconnu : statut %d, voulu 404", status)
	}
	// Site inactif (bascule OFF) → 403 site_inactive.
	status, out := doJSON(t, ts, "POST", "/api/wifi/site/buvette-yop/claim", "", map[string]any{
		"phone": "2250707080909", "optIn": false,
	})
	if status != http.StatusForbidden {
		t.Fatalf("site inactif : statut %d, voulu 403", status)
	}
	if code, _ := out["code"].(string); code != "site_inactive" {
		t.Fatalf("code machine attendu site_inactive, reçu %v", out["code"])
	}
}

func TestWifiClaimSuspendedAccount(t *testing.T) {
	ts, st := newWifiTestServer(t)
	_, accID, _ := registerAccount(t, ts, "gerant-wifi-susp", "")
	routerID, profileID := seedWifiEnv(t, st, accID)
	seedWifiSite(t, st, accID, "resto-cocody", routerID, profileID, true, 1, 100)
	// Abonnement expiré : PeriodEnd hier (PlanID non vide → statut calculé).
	st.Lock()
	st.Data().SettingsByAccount[accID] = model.Settings{
		Tenant:       model.Tenant{Name: "Resto Cocody", Currency: "XOF", Timezone: "Africa/Abidjan"},
		Subscription: model.Subscription{PlanID: "essentiel", Status: "active", PeriodEnd: time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)},
	}
	st.Unlock()

	status, out := doJSON(t, ts, "POST", "/api/wifi/site/resto-cocody/claim", "", map[string]any{
		"phone": "2250707080909", "optIn": false,
	})
	if status != http.StatusPaymentRequired {
		t.Fatalf("compte expiré : statut %d, voulu 402 (corps %v)", status, out)
	}
	if code, _ := out["code"].(string); code != "subscription_expired" {
		t.Fatalf("code machine attendu subscription_expired, reçu %v", out["code"])
	}
	// La page publique marque le site suspendu (message clair au visiteur).
	if status, out := doJSON(t, ts, "GET", "/api/wifi/site/resto-cocody", "", nil); status != http.StatusOK {
		t.Fatalf("info site (suspendu) : statut %d", status)
	} else if susp, _ := out["suspended"].(bool); !susp {
		t.Fatal("la page publique doit voir le site suspendu")
	}
}

func TestWifiConsoleCRUDAndIsolation(t *testing.T) {
	ts, st := newWifiTestServer(t)
	tokenA, accA, _ := registerAccount(t, ts, "gerant-wifi-a", "")
	// Second compte : numéro WhatsApp différent (déduplication à l'inscription).
	statusB, outB := doJSON(t, ts, "POST", "/api/auth/register", "", map[string]string{
		"name": "Gérant B", "username": "gerant-wifi-b", "password": "mot-de-passe-8+",
		"key": "", "email": "gerant-wifi-b@example.ci", "phone": "0505050505",
		"country": "CI", "city": "Abidjan",
	})
	if statusB != http.StatusCreated {
		t.Fatalf("inscription B : statut %d, corps %v", statusB, outB)
	}
	tokenB, _ := outB["token"].(string)
	routerID, profileID := seedWifiEnv(t, st, accA)

	// Console protégée : sans token → 401.
	if status, _ := doJSON(t, ts, "GET", "/api/wifi/sites", "", nil); status != http.StatusUnauthorized {
		t.Fatalf("sites sans token : statut %d, voulu 401", status)
	}

	create := func(token, name string) (int, map[string]any) {
		return doJSON(t, ts, "POST", "/api/wifi/sites", token, map[string]any{
			"name": name, "routerId": routerID, "profileId": profileID,
			"freeTimeMin": 45, "freeDataMb": 200, "marketingOptIn": true,
			"dailyPerPhone": 1, "dailyCap": 50, "active": true,
		})
	}
	// Création + slug dérivé du nom (accents supprimés).
	status, out := create(tokenA, "Maquis Chez Fofi")
	if status != http.StatusOK {
		t.Fatalf("création site : statut %d, corps %v", status, out)
	}
	slug, _ := out["slug"].(string)
	if slug != "maquis-chez-fofi" {
		t.Fatalf("slug dérivé = %q, voulu maquis-chez-fofi", slug)
	}
	// Unicité GLOBALE du slug : même nom → suffixe.
	status, out2 := create(tokenA, "Maquis Chez Fofi")
	if status != http.StatusOK {
		t.Fatalf("création 2 : statut %d", status)
	}
	if s2, _ := out2["slug"].(string); s2 == slug {
		t.Fatalf("slug dupliqué : %q", s2)
	}
	// Le gérant peut ajuster les quotas (ajustement demandé) : PUT.
	siteID, _ := out["id"].(string)
	status, out = doJSON(t, ts, "PUT", "/api/wifi/sites/"+siteID, tokenA, map[string]any{
		"name": "Maquis Chez Fofi", "routerId": routerID, "profileId": profileID,
		"freeTimeMin": 60, "freeDataMb": 500, "marketingOptIn": true,
		"dailyPerPhone": 2, "dailyCap": 200, "active": true,
	})
	if status != http.StatusOK {
		t.Fatalf("update quotas : statut %d, corps %v", status, out)
	}
	if tm, _ := out["freeTimeMin"].(float64); tm != 60 {
		t.Fatalf("quota temps ajusté = %v, voulu 60", out["freeTimeMin"])
	}
	// Bascule 1 clic : active=false → le claim public est refusé.
	status, _ = doJSON(t, ts, "PUT", "/api/wifi/sites/"+siteID, tokenA, map[string]any{
		"name": "Maquis Chez Fofi", "routerId": routerID, "profileId": profileID,
		"freeTimeMin": 60, "freeDataMb": 500, "marketingOptIn": true,
		"dailyPerPhone": 2, "dailyCap": 200, "active": false,
	})
	if status != http.StatusOK {
		t.Fatalf("toggle off : statut %d", status)
	}
	if status, _ := doJSON(t, ts, "POST", "/api/wifi/site/maquis-chez-fofi/claim", "", map[string]any{
		"phone": "2250707080909", "optIn": false,
	}); status != http.StatusForbidden {
		t.Fatalf("claim site désactivé : statut %d, voulu 403", status)
	}
	// Isolation : B ne voit AUCUN site de A, ne peut ni le modifier ni le supprimer.
	status, out = doJSON(t, ts, "GET", "/api/wifi/sites", tokenB, nil)
	if status != http.StatusOK {
		t.Fatalf("liste B : statut %d", status)
	}
	if sites, _ := out["sites"].([]any); len(sites) != 0 {
		t.Fatalf("isolation rompue : B voit %d sites de A", len(sites))
	}
	if status, _ := doJSON(t, ts, "PUT", "/api/wifi/sites/"+siteID, tokenB, map[string]any{
		"name": "Pirate", "routerId": routerID, "profileId": profileID,
		"freeTimeMin": 0, "freeDataMb": 0, "dailyPerPhone": 1, "dailyCap": 1, "active": true,
	}); status != http.StatusNotFound {
		t.Fatalf("update par B : statut %d, voulu 404", status)
	}
	if status, _ := doJSON(t, ts, "DELETE", "/api/wifi/sites/"+siteID, tokenB, nil); status != http.StatusNotFound {
		t.Fatalf("delete par B : statut %d, voulu 404", status)
	}

	// Flux complet A : activation → claim → registre + CSV.
	doJSON(t, ts, "PUT", "/api/wifi/sites/"+siteID, tokenA, map[string]any{
		"name": "Maquis Chez Fofi", "routerId": routerID, "profileId": profileID,
		"freeTimeMin": 60, "freeDataMb": 500, "marketingOptIn": true,
		"dailyPerPhone": 2, "dailyCap": 200, "active": true,
	})
	if status, out := doJSON(t, ts, "POST", "/api/wifi/site/maquis-chez-fofi/claim", "", map[string]any{
		"phone": "2250102030405", "optIn": true,
	}); status != http.StatusOK {
		t.Fatalf("claim après activation : statut %d (corps %v)", status, out)
	}
	status, out = doJSON(t, ts, "GET", "/api/wifi/guests?siteId="+siteID, tokenA, nil)
	if status != http.StatusOK {
		t.Fatalf("registre : statut %d", status)
	}
	if guests, _ := out["guests"].([]any); len(guests) != 1 {
		t.Fatalf("registre : %d entrées, voulu 1", len(guests))
	}
	// Export CSV.
	req, _ := http.NewRequest("GET", ts.URL+"/api/wifi/guests?export=csv", nil)
	req.Header.Set("Authorization", "Bearer "+tokenA)
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("export CSV : %v", err)
	}
	defer resp.Body.Close()
	buf := make([]byte, 4096)
	n, _ := resp.Body.Read(buf)
	csv := string(buf[:n])
	if resp.StatusCode != http.StatusOK || !strings.Contains(csv, "2250102030405") {
		t.Fatalf("export CSV invalide : statut %d, corps %q", resp.StatusCode, csv)
	}
	// Suppression A : site + registre effacés.
	if status, _ := doJSON(t, ts, "DELETE", "/api/wifi/sites/"+siteID, tokenA, nil); status != http.StatusOK {
		t.Fatalf("delete par A : statut %d", status)
	}
	st.Lock()
	left := 0
	for _, g := range st.Data().WifiGuests {
		if g.SiteID == siteID {
			left++
		}
	}
	st.Unlock()
	if left != 0 {
		t.Fatalf("suppression : %d entrées de registre restantes", left)
	}
}
