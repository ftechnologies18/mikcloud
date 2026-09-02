// Tests HTTP de bout en bout du package api — surface publique réelle
// (httptest.Server sur api.Handler(), store en mode JSON dans t.TempDir()).
//
// Couverture :
//   - GET /{$} (healthcheck), route /api/xyz inconnue → 404 JSON ;
//   - protection : route protégée sans token → 401 JSON, bearer invalide → 401 ;
//   - flux d'authentification complet : inscription (REGISTER_KEY, fail-closed)
//     → connexion → GET /api/auth/me avec le JWT renvoyé ;
//   - requireRole : owner passe, manager bloqué (403) sur une route rang 3,
//     super-admin plateforme traité comme owner ;
//   - helpers purs : queryInt, csvField, normalizeHotspotLoginUrl,
//     sanitizeVoucherComment, quotaNote, subscriptionStatus, planAmount,
//     roleRank.
//
// AUCUNE connexion réseau réelle (pas de PostgreSQL, pas d'envoi sortant) :
// le store vit dans un répertoire temporaire et DATABASE_URL est vidée.
package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"mikcloud/hotspot-api/internal/auth"
	"mikcloud/hotspot-api/internal/model"
	"mikcloud/hotspot-api/internal/secretbox"
	"mikcloud/hotspot-api/internal/store"
)

const testJWTSecret = "jwt-secret-de-test-api"

func TestMain(m *testing.M) {
	os.Unsetenv("CREDENTIALS_KEY")
	if err := secretbox.Init(testJWTSecret); err != nil {
		panic("secretbox.Init impossible : " + err.Error())
	}
	os.Exit(m.Run())
}

// startTestServer — serveur httptest exposant le mux complet de l'API.
func startTestServer(st *store.Store) *httptest.Server {
	return httptest.NewServer(New(st, testJWTSecret).Handler())
}

// newTestServer — serveur HTTP de test complet : store JSON éphémère,
// admin plateforme depuis ADMIN_PASSWORD (identifiants connus du test).
func newTestServer(t *testing.T) *httptest.Server {
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
	return ts
}

// doJSON — requête JSON sur le serveur de test (réponse décodée en map).
func doJSON(t *testing.T, ts *httptest.Server, method, path, token string, body any) (int, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encodage du corps impossible : %v", err)
		}
	}
	req, err := http.NewRequest(method, ts.URL+path, &buf)
	if err != nil {
		t.Fatalf("requête impossible : %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("%s %s : %v", method, path, err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

// registerAccount — inscrit un compte via la surface HTTP (REGISTER_KEY posée
// par l'appelant) et retourne (token, accountID, userID).
func registerAccount(t *testing.T, ts *httptest.Server, username, key string) (string, string, string) {
	t.Helper()
	status, out := doJSON(t, ts, "POST", "/api/auth/register", "", map[string]string{
		"name":     "Gérant " + username,
		"username": username,
		"password": "mot-de-passe-8+",
		"key":      key,
		"email":    username + "@example.ci",
		"phone":    "0707070707",
		"country":  "CI",
		"city":     "Abidjan",
	})
	if status != http.StatusCreated {
		t.Fatalf("inscription %s : statut %d, corps %v", username, status, out)
	}
	token, _ := out["token"].(string)
	if token == "" {
		t.Fatal("l'inscription doit renvoyer un token")
	}
	user, _ := out["user"].(map[string]any)
	accID, _ := user["accountId"].(string)
	usrID, _ := user["id"].(string)
	return token, accID, usrID
}

func TestHealthEndpoint(t *testing.T) {
	ts := newTestServer(t)
	req, _ := http.NewRequest("GET", ts.URL+"/", nil)
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("GET / : %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /{$} doit répondre 200, obtenu %d", resp.StatusCode)
	}
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("réponse non JSON : %v", err)
	}
	if ok, _ := out["ok"].(bool); !ok {
		t.Fatalf("healthcheck doit renvoyer ok:true, obtenu %v", out)
	}
	if out["service"] != "mikcloud-hotspot-api" {
		t.Fatalf("service inattendu : %v", out["service"])
	}
}

func TestUnknownAPIRouteReturns404JSON(t *testing.T) {
	ts := newTestServer(t)
	// Sans token : le middleware d'auth s'applique AVANT le routage → 401.
	if status, _ := doJSON(t, ts, "GET", "/api/xyz", "", nil); status != http.StatusUnauthorized {
		t.Fatalf("route inconnue sans token → 401 (auth avant routage), obtenu %d", status)
	}
	// Avec un token valable : le mux route vers le fallback /api/ → 404 JSON.
	token, _, _ := registerAccount(t, ts, "route-inconnue", "")
	status, out := doJSON(t, ts, "GET", "/api/xyz", token, nil)
	if status != http.StatusNotFound {
		t.Fatalf("route inconnue doit répondre 404, obtenu %d", status)
	}
	if out["error"] != "Route introuvable" {
		t.Fatalf("le 404 doit être un JSON {error}, obtenu %v", out)
	}
}

func TestProtectedRouteWithoutToken(t *testing.T) {
	ts := newTestServer(t)
	status, out := doJSON(t, ts, "GET", "/api/dashboard", "", nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("route protégée sans token doit répondre 401, obtenu %d", status)
	}
	if msg, _ := out["error"].(string); msg == "" {
		t.Fatalf("le 401 doit porter un JSON {error}, obtenu %v", out)
	}
}

func TestGarbageBearerTokenRejected(t *testing.T) {
	ts := newTestServer(t)
	status, _ := doJSON(t, ts, "GET", "/api/auth/me", "ceci-n-est-pas-un-jwt", nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("bearer invalide doit répondre 401, obtenu %d", status)
	}
	// Même un JWT bien formé mais signé par un autre secret est refusé.
	foreign := auth.Sign("autre-secret", auth.NewClaims("usr-x", "X", "owner", "acc-x"))
	status, _ = doJSON(t, ts, "GET", "/api/auth/me", foreign, nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("token signé par un autre secret doit répondre 401, obtenu %d", status)
	}
}

func TestRegisterLoginMeFlow(t *testing.T) {
	t.Setenv("REGISTER_KEY", "cle-invitation-test")
	ts := newTestServer(t)

	// Inscription fermée sans clé : fail-closed (audit P1 #10).
	status, out := doJSON(t, ts, "POST", "/api/auth/register", "", map[string]string{
		"name": "Sans clé", "username": "sanscle", "password": "mot-de-passe-8+",
		"email": "sc@example.ci", "phone": "0101010101", "country": "CI",
	})
	if status != http.StatusForbidden {
		t.Fatalf("inscription sans clé doit répondre 403, obtenu %d (%v)", status, out)
	}
	// Mauvaise clé : refus aussi.
	status, _ = doJSON(t, ts, "POST", "/api/auth/register", "", map[string]string{
		"name": "Mauvaise clé", "username": "mauvaise", "password": "mot-de-passe-8+",
		"key": "mauvaise-cle", "email": "mk@example.ci", "phone": "0101010101", "country": "CI",
	})
	if status != http.StatusForbidden {
		t.Fatalf("inscription avec mauvaise clé doit répondre 403, obtenu %d", status)
	}

	// Inscription avec la bonne clé → 201 + token + rôle owner.
	token, accID, usrID := registerAccount(t, ts, "gerant1", "cle-invitation-test")
	status, out = doJSON(t, ts, "GET", "/api/auth/me", token, nil)
	if status != http.StatusOK {
		t.Fatalf("GET /api/auth/me avec le JWT d'inscription doit répondre 200, obtenu %d (%v)", status, out)
	}
	user, _ := out["user"].(map[string]any)
	if user["username"] != "gerant1" || user["role"] != "owner" || user["accountId"] != accID {
		t.Fatalf("identité inattendue : %v", user)
	}

	// Connexion avec les mêmes identifiants → nouveau token valable.
	status, out = doJSON(t, ts, "POST", "/api/auth/login", "", map[string]string{
		"username": "gerant1", "password": "mot-de-passe-8+",
	})
	if status != http.StatusOK {
		t.Fatalf("connexion : statut %d (%v)", status, out)
	}
	loginToken, _ := out["token"].(string)
	if loginToken == "" {
		t.Fatal("la connexion doit renvoyer un token")
	}
	status, out = doJSON(t, ts, "GET", "/api/auth/me", loginToken, nil)
	if status != http.StatusOK {
		t.Fatalf("/api/auth/me après login : statut %d", status)
	}
	if user, _ = out["user"].(map[string]any); user["id"] != usrID {
		t.Fatalf("identité après login inattendue : %v", user)
	}
	// Identifiants invalides → 400 (message unique, pas d'oracle).
	status, _ = doJSON(t, ts, "POST", "/api/auth/login", "", map[string]string{
		"username": "gerant1", "password": "faux-mot-de-passe",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("mauvais mot de passe doit répondre 400, obtenu %d", status)
	}
}

func TestRequireRoleMatrix(t *testing.T) {
	ts := newTestServer(t)
	ownerToken, accID, _ := registerAccount(t, ts, "proprio", "")

	// Forge de tokens pour les autres rôles (même secret de test).
	manager := auth.Sign(testJWTSecret, auth.NewClaims("usr-mgr", "Manager", model.RoleManager, accID))
	reseller := auth.Sign(testJWTSecret, auth.NewClaims("usr-res", "Revendeur", "reseller", accID))
	plateforme := auth.Sign(testJWTSecret, auth.NewClaims("usr-plat", "Plateforme", model.RolePlatformAdmin, ""))

	// GET /api/team est requireRole(3) : le owner passe.
	if status, _ := doJSON(t, ts, "GET", "/api/team", ownerToken, nil); status != http.StatusOK {
		t.Fatalf("owner sur route rang 3 doit passer, obtenu %d", status)
	}
	// Le manager (rang 2) est bloqué : 403 rôle insuffisant.
	status, out := doJSON(t, ts, "GET", "/api/team", manager, nil)
	if status != http.StatusForbidden {
		t.Fatalf("manager sur route rang 3 doit répondre 403, obtenu %d", status)
	}
	if !strings.Contains(fmt.Sprint(out["error"]), "rôle insuffisant") {
		t.Fatalf("message d'erreur inattendu : %v", out)
	}
	// Le token revendeur (rang 0) est arrêté AVANT par la liste blanche du
	// middleware : hors /api/sell/* → 403 « Accès réservé au Mode Vente ».
	status, out = doJSON(t, ts, "GET", "/api/team", reseller, nil)
	if status != http.StatusForbidden {
		t.Fatalf("revendeur hors Mode Vente doit répondre 403, obtenu %d", status)
	}
	if fmt.Sprint(out["error"]) != "Accès réservé au Mode Vente" {
		t.Fatalf("message d'erreur inattendu : %v", out)
	}
	// Le super-admin plateforme est traité comme owner : console plateforme OK.
	if status, _ := doJSON(t, ts, "GET", "/api/admin/overview", plateforme, nil); status != http.StatusOK {
		t.Fatalf("plateforme sur route rang 3 doit passer, obtenu %d", status)
	}
	// PUT /api/settings (requireRole 3) : owner modifie, manager est refusé.
	if status, _ := doJSON(t, ts, "PUT", "/api/settings", ownerToken, map[string]any{}); status != http.StatusOK {
		t.Fatalf("PUT /api/settings par le owner doit passer, obtenu %d", status)
	}
	if status, _ := doJSON(t, ts, "PUT", "/api/settings", manager, map[string]any{}); status != http.StatusForbidden {
		t.Fatalf("PUT /api/settings par le manager doit répondre 403, obtenu %d", status)
	}
}

// ---------------------------------------------------------------------------
// Helpers purs — comportements réels, cas limites inclus
// ---------------------------------------------------------------------------

func TestQueryInt(t *testing.T) {
	build := func(query string) *http.Request {
		r := httptest.NewRequest("GET", "/api/test"+query, nil)
		return r
	}
	cas := []struct {
		nom              string
		query            string
		def, min, max, v int
	}{
		{"absent → défaut", "", 10, 1, 100, 10},
		{"valeur valide", "?limit=42", 10, 1, 100, 42},
		{"non numérique → défaut", "?limit=abc", 10, 1, 100, 10},
		{"sous le minimum → défaut", "?limit=0", 10, 1, 100, 10},
		{"négatif → défaut", "?limit=-5", 10, 1, 100, 10},
		{"au-dessus du max → borné", "?limit=999", 10, 1, 100, 100},
		{"borne max exacte", "?limit=100", 10, 1, 100, 100},
	}
	for _, c := range cas {
		if got := queryInt(build(c.query), "limit", c.def, c.min, c.max); got != c.v {
			t.Fatalf("%s : queryInt = %d, attendu %d", c.nom, got, c.v)
		}
	}
}

func TestCSVField(t *testing.T) {
	cas := []struct{ in, want string }{
		{"simple", "simple"},
		{"avec espace", "avec espace"},
		{"a;b", `"a;b"`},                      // séparateur → guillemets
		{`dis "x"`, `"dis ""x"""`},            // guillemet doublé + enveloppe
		{"ligne1\nligne2", `"ligne1 ligne2"`}, // \n → espace + enveloppe
		{"a\rb", `"ab"`},                      // \r → supprimé + enveloppe
		{"", ""},
	}
	for _, c := range cas {
		if got := csvField(c.in); got != c.want {
			t.Fatalf("csvField(%q) = %q, attendu %q", c.in, got, c.want)
		}
	}
}

func TestNormalizeHotspotLoginURL(t *testing.T) {
	cas := []struct {
		in    string
		want  string
		valid bool
	}{
		{"", "", true}, // vide = QR texte, valide
		{"  ", "", true},
		{"http://10.5.50.1/login", "http://10.5.50.1/login", true},
		{"10.5.50.1/login", "http://10.5.50.1/login", true},                                        // schéma ajouté
		{"https://portail.example.ci/login?next=/x#top", "https://portail.example.ci/login", true}, // query+fragment retirés
		{"https://portail.example.ci", "https://portail.example.ci", true},
		{"ftp://10.5.50.1/login", "", false}, // schéma non http(s)
		{"://pas-de-host", "", false},
	}
	for _, c := range cas {
		got, ok := normalizeHotspotLoginUrl(c.in)
		if ok != c.valid || got != c.want {
			t.Fatalf("normalizeHotspotLoginUrl(%q) = (%q,%v), attendu (%q,%v)", c.in, got, ok, c.want, c.valid)
		}
	}
}

func TestSanitizeVoucherComment(t *testing.T) {
	if got := sanitizeVoucherComment("  lot du matin  "); got != "lot du matin" {
		t.Fatalf("trim attendu, obtenu %q", got)
	}
	// Retours à la ligne et tabulations → espaces (une seule ligne).
	if got := sanitizeVoucherComment("ligne1\nligne2\r\nligne3\tfin"); got != "ligne1 ligne2  ligne3 fin" {
		t.Fatalf("retours à la ligne non neutralisés : %q", got)
	}
	// Caractères de contrôle (< 0x20) → espaces.
	if got := sanitizeVoucherComment("a\x01b"); got != "a b" {
		t.Fatalf("caractère de contrôle non neutralisé : %q", got)
	}
	// Troncature à 64 runes (comptées en runes, pas en octets).
	long := strings.Repeat("é", 80) // 160 octets, 80 runes
	if got := sanitizeVoucherComment(long); len([]rune(got)) != 64 {
		t.Fatalf("troncature à 64 runes attendue, obtenu %d", len([]rune(got)))
	}
	// Les 64 runes conservées sont les 64 premières.
	if got := sanitizeVoucherComment("ab" + strings.Repeat("c", 100)); !strings.HasPrefix(got, "ab") {
		t.Fatalf("le début du commentaire doit être conservé : %q", got)
	}
}

func TestQuotaNote(t *testing.T) {
	cas := []struct {
		in   int
		want string
	}{
		{0, ""},
		{-5, ""},
		{512, " — quota 512 Mo"},
		{1024, " — quota 1 Go"},
		{5120, " — quota 5 Go"},
		{1500, " — quota 1500 Mo"}, // non multiple de 1024 : reste en Mo
	}
	for _, c := range cas {
		if got := quotaNote(c.in); got != c.want {
			t.Fatalf("quotaNote(%d) = %q, attendu %q", c.in, got, c.want)
		}
	}
}

func TestSubscriptionStatus(t *testing.T) {
	now := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	rfc := time.RFC3339
	cas := []struct {
		nom  string
		sub  model.Subscription
		want string
	}{
		{"bêta (pas de formule)", model.Subscription{}, "none"},
		{"période en cours", model.Subscription{PlanID: "essentiel", PeriodEnd: now.Add(24 * time.Hour).Format(rfc)}, "active"},
		{"période échue récemment → expiré", model.Subscription{PlanID: "essentiel", PeriodEnd: now.Add(-24 * time.Hour).Format(rfc)}, "expired"},
		{"au-delà de la grâce de 30 j → suspendu", model.Subscription{PlanID: "essentiel", PeriodEnd: now.Add(-31 * 24 * time.Hour).Format(rfc)}, "suspended"},
		{"exactement à la fin de la grâce → encore expiré (bascule stricte After)", model.Subscription{PlanID: "illimite", PeriodEnd: now.Add(-30 * 24 * time.Hour).Format(rfc)}, "expired"},
		{"PeriodEnd illisible → actif (pas de blocage indu)", model.Subscription{PlanID: "essentiel", PeriodEnd: "n'importe quoi"}, "active"},
		{"PeriodEnd vide → actif (non expirant)", model.Subscription{PlanID: "illimite"}, "active"},
	}
	for _, c := range cas {
		if got := subscriptionStatus(c.sub, now); got != c.want {
			t.Fatalf("%s : subscriptionStatus = %q, attendu %q", c.nom, got, c.want)
		}
	}
}

func TestPlanAmount(t *testing.T) {
	essentiel := model.SaasPlan{ID: "essentiel", PriceFcfa: 1250, PerRouter: true}
	illimite := model.SaasPlan{ID: "illimite", PriceFcfa: 12000}
	cas := []struct {
		nom    string
		plan   model.SaasPlan
		router int
		want   int
	}{
		{"Essentiel 1 routeur", essentiel, 1, 1250},
		{"Essentiel 3 routeurs", essentiel, 3, 3750},
		{"Essentiel 0 routeur → plancher 1", essentiel, 0, 1250},
		{"Illimité forfait quel que soit le parc", illimite, 17, 12000},
	}
	for _, c := range cas {
		if got := planAmount(c.plan, c.router); got != c.want {
			t.Fatalf("%s : planAmount = %d, attendu %d", c.nom, got, c.want)
		}
	}
}

func TestRoleRank(t *testing.T) {
	cas := map[string]int{
		model.RoleManager:       2,
		model.RoleOwner:         3,
		model.RolePlatformAdmin: 3,
		"admin":                 3, // rôle plateforme historique
		"reseller":              0, // Mode Vente : hors hiérarchie console
		"operator":              0, // rôle retiré : défensif
		"inconnu":               0,
	}
	for role, want := range cas {
		if got := roleRank(role); got != want {
			t.Fatalf("roleRank(%q) = %d, attendu %d", role, got, want)
		}
	}
}
