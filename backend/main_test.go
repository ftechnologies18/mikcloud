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
//   - clientIP : X-Forwarded-For (dernier hop uniquement — les IP antérieures
//     sont forgeables), repli RemoteAddr.
package main

import (
	"net/http"
	"net/http/httptest"
	"strconv"
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
		{"dernier hop uniquement", "1.2.3.4, 5.6.7.8, 9.10.11.12", "10.0.0.1:1", "9.10.11.12"},
		{"XFF avec espaces", " 1.2.3.4 , 5.6.7.8 ", "10.0.0.1:1", "5.6.7.8"},
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

	// Les fenêtres sont par route+IP : /api/routers n'est PAS limitée.
	for i := 0; i < 30; i++ {
		rec := httptest.NewRecorder()
		limited.ServeHTTP(rec, req("GET", "/api/routers", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("route hors périmètre du limiteur doit passer, obtenu %d", rec.Code)
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
