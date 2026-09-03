// Tests du package main — middlewares HTTP de bordure uniquement
// (CORS, anti brute-force, extraction IP client).
//
// Ces fonctions vivent dans main.go (hors packages internes) : elles sont
// testées ici sans jamais exécuter main() ni toucher au réseau réel — les
// requêtes passent par httptest.ResponseRecorder.
//
// Couverture :
//   - corsMiddleware : origine autorisée réfléchie + Vary, origine inconnue
//     sans en-tête, préflight OPTIONS → 204, mode dev ouvert (« * »), mode
//     production sans ALLOWED_ORIGIN → fail-closed (audit P1 #16) ;
//   - authRateLimit : 12 req/min sur /api/auth/*, 5 req/min sur
//     /api/reseller/login (P0), routes non limitées, 429 avec Retry-After,
//     compteur par IP ;
//   - clientIP : X-Forwarded-For (PREMIER hop = IP client réelle posée par le
//     proxy de confiance Render — sondes production S1), repli RemoteAddr.
package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// req — construit une requête de test avec en-têtes optionnels.
func req(method, target string, headers map[string]string) *http.Request {
	r := httptest.NewRequest(method, target, nil)
	r.RemoteAddr = "203.0.113.7:55555"
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	return r
}

func TestCorsMiddlewareReflectsAllowedOrigin(t *testing.T) {
	t.Setenv("ALLOWED_ORIGIN", "https://console.example.ci, https://secours.example.ci")
	t.Setenv("DATABASE_URL", "")
	handler := corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Origine de la liste blanche → réfléchie (pas « * ») + Vary: Origin.
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req("GET", "/api/dashboard", map[string]string{"Origin": "https://secours.example.ci"}))
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://secours.example.ci" {
		t.Fatalf("origine autorisée doit être réfléchie, obtenu %q", got)
	}
	if got := rec.Header().Get("Vary"); got != "Origin" {
		t.Fatalf("Vary: Origin attendu, obtenu %q", got)
	}

	// Origine inconnue → AUCUN en-tête CORS (navigateur refuse).
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req("GET", "/api/dashboard", map[string]string{"Origin": "https://pirate.example"}))
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("origine hors liste ne doit pas être reflétée, obtenu %q", got)
	}

	// Préflight → 204 direct.
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req("OPTIONS", "/api/dashboard", map[string]string{"Origin": "https://secours.example.ci"}))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("préflight OPTIONS doit répondre 204, obtenu %d", rec.Code)
	}
}

func TestCorsMiddlewareDevOpenAndProdFailClosed(t *testing.T) {
	// Dev local (sans ALLOWED_ORIGIN ni DATABASE_URL) : ouvert « * ».
	t.Setenv("ALLOWED_ORIGIN", "")
	t.Setenv("DATABASE_URL", "")
	dev := corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	rec := httptest.NewRecorder()
	dev.ServeHTTP(rec, req("GET", "/", map[string]string{"Origin": "https://nimporte.example"}))
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("mode dev sans config doit rester ouvert (*), obtenu %q", got)
	}

	// Production (DATABASE_URL défini) SANS ALLOWED_ORIGIN : fail-closed —
	// aucun en-tête CORS n'est émis. NB : la variable n'est jamais utilisée
	// pour une connexion ici, corsMiddleware ne lit que sa présence.
	t.Setenv("DATABASE_URL", "postgres://rien:aucune-connexion@invalide/db")
	prod := corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	rec = httptest.NewRecorder()
	prod.ServeHTTP(rec, req("GET", "/", map[string]string{"Origin": "https://nimporte.example"}))
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("production sans ALLOWED_ORIGIN doit être fail-closed, obtenu %q", got)
	}
}

func TestClientIPForwardedFor(t *testing.T) {
	cas := []struct {
		nom  string
		xff  string
		rem  string
		want string
	}{
		{"XFF simple", "198.51.100.23", "10.0.0.1:1", "198.51.100.23"},
		{"premier hop (contrat Render)", "1.2.3.4, 5.6.7.8, 9.10.11.12", "10.0.0.1:1", "1.2.3.4"},
		{"XFF avec espaces", " 1.2.3.4 , 5.6.7.8 ", "10.0.0.1:1", "1.2.3.4"},
		{"sans XFF → RemoteAddr", "", "192.0.2.44:8080", "192.0.2.44"},
	}
	for _, c := range cas {
		r := req("GET", "/api/auth/login", nil)
		r.RemoteAddr = c.rem
		if c.xff != "" {
			r.Header.Set("X-Forwarded-For", c.xff)
		}
		if got := clientIP(r); got != c.want {
			t.Fatalf("%s : clientIP = %q, attendu %q", c.nom, got, c.want)
		}
	}
}

func TestAuthRateLimitBursts(t *testing.T) {
	// Burst rapide dans la fenêtre d'une minute (aucun sleep : les requêtes
	// partent en rafale, très loin d'un reset de fenêtre).
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	limited := authRateLimit(next)

	exhaust := func(path string, limit int) {
		t.Helper()
		for i := 1; i <= limit; i++ {
			rec := httptest.NewRecorder()
			limited.ServeHTTP(rec, req("POST", path, nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("%s : la requête %d sur %s doit passer (fenêtre neuve), obtenu %d", path, i, path, rec.Code)
			}
		}
		// Une de plus → 429 avec Retry-After.
		rec := httptest.NewRecorder()
		limited.ServeHTTP(rec, req("POST", path, nil))
		if rec.Code != http.StatusTooManyRequests {
			t.Fatalf("%s : la requête %d doit être limitée (429), obtenu %d", path, limit+1, rec.Code)
		}
		if rec.Header().Get("Retry-After") != "60" {
			t.Fatalf("Retry-After: 60 attendu, obtenu %q", rec.Header().Get("Retry-After"))
		}
	}
	exhaust("/api/auth/login", 12)    // /api/auth/* : 12 req/min
	exhaust("/api/reseller/login", 5) // PIN revendeur : 5 req/min (P0)

	// Les fenêtres sont par scope+IP : /api/routers tombe dans le scope
	// global (S1-A2, 120 req/min) — 30 requêtes restent loin de la limite.
	for i := 0; i < 30; i++ {
		rec := httptest.NewRecorder()
		limited.ServeHTTP(rec, req("GET", "/api/routers", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("route sous la limite globale doit passer, obtenu %d", rec.Code)
		}
	}

	// Un autre IP a sa propre fenêtre sur /api/reseller/login.
	autreIP := req("POST", "/api/reseller/login", nil)
	autreIP.RemoteAddr = "198.51.100.99:1234"
	rec := httptest.NewRecorder()
	limited.ServeHTTP(rec, autreIP)
	if rec.Code != http.StatusOK {
		t.Fatalf("une autre IP démarre une fenêtre neuve, obtenu %d", rec.Code)
	}
}

// TestAuthRateLimitCountsPerScope — /api/auth/* et /api/reseller/login ont
// des scopes distincts : épuiser /api/auth/login n'affecte pas le login PIN.
func TestAuthRateLimitCountsPerScope(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	limited := authRateLimit(next)
	for i := 0; i < 12; i++ {
		rec := httptest.NewRecorder()
		limited.ServeHTTP(rec, req("POST", "/api/auth/register", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("requête %d sur /api/auth/register doit passer, obtenu %d", i+1, rec.Code)
		}
	}
	rec := httptest.NewRecorder()
	limited.ServeHTTP(rec, req("POST", "/api/reseller/login", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("le scope revendeur est indépendant du scope auth, obtenu %d", rec.Code)
	}
	// La limite /api/auth/* est aussi déclenchée depuis register.
	rec = httptest.NewRecorder()
	limited.ServeHTTP(rec, req("POST", "/api/auth/register", nil))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("13e requête sur /api/auth/* doit être limitée, obtenu %d (%s)",
			rec.Code, strconv.Quote(rec.Body.String()))
	}
}

// TestSecurityHeaders — S1-A4 : les cinq en-têtes de sécurité sont posés sur
// toutes les réponses (y compris les erreurs du handler aval).
func TestSecurityHeaders(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	h := securityHeaders(next)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req("GET", "/api/dashboard", nil))
	want := map[string]string{
		"X-Content-Type-Options":    "nosniff",
		"Strict-Transport-Security": "max-age=31536000; includeSubDomains",
		"X-Frame-Options":           "DENY",
		"Referrer-Policy":           "no-referrer",
		"Cache-Control":             "no-store",
	}
	for k, v := range want {
		if got := rec.Header().Get(k); got != v {
			t.Fatalf("en-tête %s = %q, attendu %q", k, got, v)
		}
	}
}

// TestLimitBodyRejectsOversized — S1-A1 : un corps dont le Content-Length
// dépasse le plafond reçoit un 413 immédiat (le corps n'est jamais lu) ;
// un corps dans la limite passe.
func TestLimitBodyRejectsOversized(t *testing.T) {
	passed := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { passed = true })
	h := limitBody(next)

	// Corps dans la limite → passe.
	r := req("POST", "/api/vouchers/generate", nil)
	r.Body = io.NopCloser(strings.NewReader(`{"count":10}`))
	r.ContentLength = int64(len(`{"count":10}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if !passed || rec.Code != http.StatusOK {
		t.Fatalf("corps sous la limite doit passer, obtenu %d (passed=%v)", rec.Code, passed)
	}

	// Corps au-delà de la limite → 413, le handler aval n'est JAMAIS atteint.
	passed = false
	big := make([]byte, maxBodyBytes+1)
	r = req("POST", "/api/vouchers/generate", nil)
	r.Body = io.NopCloser(bytes.NewReader(big))
	r.ContentLength = int64(len(big))
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("corps au-delà de la limite doit répondre 413, obtenu %d", rec.Code)
	}
	if passed {
		t.Fatal("le handler aval ne doit pas être atteint pour un corps trop grand")
	}
	if !strings.Contains(rec.Body.String(), "trop volumineux") {
		t.Fatalf("le 413 doit porter un message JSON, obtenu %q", rec.Body.String())
	}
}

// TestRateLimitGlobalAPI — S1-A2 : toute route /api/* hors auth/revendeur est
// limitée à 120 requêtes/minute par IP ; les scopes durs d'authentification
// restent indépendants ; /agent/* et les routes hors /api/ restent hors limiteur.
func TestRateLimitGlobalAPI(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	limited := authRateLimit(next)

	// 120 requêtes sur /api/dashboard passent, la 121e est limitée.
	for i := 1; i <= 120; i++ {
		rec := httptest.NewRecorder()
		limited.ServeHTTP(rec, req("GET", "/api/dashboard", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("requête %d sur /api/dashboard doit passer (limite 120), obtenu %d", i, rec.Code)
		}
	}
	rec := httptest.NewRecorder()
	limited.ServeHTTP(rec, req("GET", "/api/dashboard", nil))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("121e requête sur /api/dashboard doit être limitée (429), obtenu %d", rec.Code)
	}

	// Le scope auth garde sa propre fenêtre dure (12/min).
	for i := 0; i < 12; i++ {
		rec := httptest.NewRecorder()
		limited.ServeHTTP(rec, req("POST", "/api/auth/login", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("requête %d sur /api/auth/login doit passer, obtenu %d", i+1, rec.Code)
		}
	}
	rec = httptest.NewRecorder()
	limited.ServeHTTP(rec, req("POST", "/api/auth/login", nil))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("13e requête sur /api/auth/login doit être limitée (429), obtenu %d", rec.Code)
	}

	// Les routes hors /api/ (poll agent, healthcheck) restent hors périmètre.
	for i := 0; i < 200; i++ {
		rec := httptest.NewRecorder()
		limited.ServeHTTP(rec, req("POST", "/agent/cmd", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("/agent/cmd doit rester hors limiteur, obtenu %d à la requête %d", rec.Code, i+1)
		}
	}
}
