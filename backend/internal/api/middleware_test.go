// Tests du middleware d'authentification et des gardes d'autorisation
// (surface HTTP réelle via httptest).
//
// Couverture :
//   - matrice authMiddleware : sans token → 401, bearer quelconque → 401,
//     token valable → 200, routes publiques (login) toujours accessibles ;
//   - liste blanche du rôle revendeur : refus hors /api/sell/*, accès à
//     /api/sell/me ;
//   - requireRole : manager (rang 2) bloqué sur une route rang 3, owner et
//     super-admin plateforme traités comme owner (rang 3) ;
//   - suspension d'abonnement (P5) : au-delà de PeriodEnd + 30 jours de
//     grâce, routes métier → 402 account_suspended, seules /api/auth/me et
//     /api/subscription restent ouvertes.
package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"mikcloud/hotspot-api/internal/auth"
	"mikcloud/hotspot-api/internal/model"
	"mikcloud/hotspot-api/internal/store"
)

// newTestServerWithStore — comme newTestServer mais retourne aussi le store
// (pour manipuler l'état : abonnement suspendu).
func newTestServerWithStore(t *testing.T) (*store.Store, *httptest.Server) {
	t.Helper()
	t.Setenv("DATABASE_URL", "")
	t.Setenv("ADMIN_PASSWORD", "admin-test-1234")
	t.Setenv("ADMIN_USERNAME", "")
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatalf("store.New impossible : %v", err)
	}
	ts := startTestServer(st)
	t.Cleanup(ts.Close)
	return st, ts
}

func TestAuthMiddlewareMatrix(t *testing.T) {
	_, ts := newTestServerWithStore(t)
	ownerToken, accID, _ := registerAccount(t, ts, "proprio-mw", "")

	cas := []struct {
		nom    string
		method string
		path   string
		token  string
		status int
	}{
		// POST corps vide → 400 « corps invalide » : la route publique RÉPOND
		// (elle n'est pas bloquée par le middleware d'authentification).
		{"route publique login sans token", http.MethodPost, "/api/auth/login", "", http.StatusBadRequest},
		{"route protégée sans token", http.MethodGet, "/api/dashboard", "", http.StatusUnauthorized},
		{"bearer quelconque", http.MethodGet, "/api/dashboard", "abc.def.ghi", http.StatusUnauthorized},
		{"token owner valable", http.MethodGet, "/api/auth/me", ownerToken, http.StatusOK},
		{"token owner sur route métier", http.MethodGet, "/api/vouchers", ownerToken, http.StatusOK},
		{"revendeur bloqué hors Mode Vente", http.MethodGet, "/api/dashboard",
			auth.Sign(testJWTSecret, auth.NewClaims("usr-r", "R", "reseller", accID)), http.StatusForbidden},
		{"revendeur autorisé sur /api/sell/me", http.MethodGet, "/api/sell/me",
			auth.Sign(testJWTSecret, auth.NewClaims("usr-r", "R", "reseller", accID)), http.StatusOK},
		{"manager bloqué sur requireRole(3)", http.MethodGet, "/api/team",
			auth.Sign(testJWTSecret, auth.NewClaims("usr-m", "M", model.RoleManager, accID)), http.StatusForbidden},
		{"owner passe sur requireRole(3)", http.MethodGet, "/api/team", ownerToken, http.StatusOK},
		{"plateforme traité comme owner", http.MethodGet, "/api/team",
			auth.Sign(testJWTSecret, auth.NewClaims("usr-p", "P", model.RolePlatformAdmin, "")), http.StatusOK},
	}
	for _, c := range cas {
		status, _ := doJSON(t, ts, c.method, c.path, c.token, nil)
		if status != c.status {
			t.Fatalf("%s : %s → %d, attendu %d", c.nom, c.path, status, c.status)
		}
	}
}

func TestSuspendedAccountLockdown(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	ownerToken, accID, _ := registerAccount(t, ts, "proprio-susp", "")

	// Abonnement échué depuis 45 jours : au-delà de la grâce de 30 jours →
	// compte suspendu (verrou lu à chaque requête, sans muter le stockage).
	st.Lock()
	st.Data().SettingsByAccount[accID] = model.Settings{
		Subscription: model.Subscription{
			PlanID:    "essentiel",
			Status:    "expired",
			PeriodEnd: time.Now().UTC().Add(-45 * 24 * time.Hour).Format(time.RFC3339),
		},
	}
	st.Save()
	st.Unlock()

	cas := []struct {
		nom    string
		path   string
		status int
	}{
		{"route métier bloquée", "/api/dashboard", http.StatusPaymentRequired},
		{"lecture vouchers bloquée", "/api/vouchers", http.StatusPaymentRequired},
		{"identification toujours ouverte", "/api/auth/me", http.StatusOK},
		{"abonnement (paiement) toujours ouvert", "/api/subscription", http.StatusOK},
		{"réglages toujours ouverts", "/api/settings", http.StatusOK},
	}
	for _, c := range cas {
		status, body := doJSON(t, ts, http.MethodGet, c.path, ownerToken, nil)
		if status != c.status {
			t.Fatalf("%s : %s → %d, attendu %d (%v)", c.nom, c.path, status, c.status, body)
		}
	}
	// Le refus porte le code machine account_suspended (réaction front adaptée).
	_, body := doJSON(t, ts, http.MethodGet, "/api/dashboard", ownerToken, nil)
	if body["code"] != "account_suspended" {
		t.Fatalf("code machine attendu account_suspended, obtenu %v", body["code"])
	}

	// Le super-admin plateforme n'est JAMAIS soumis au verrou (exemption support).
	// Régression couverte : l'exemption s'évalue sur les claims du token DANS le
	// middleware (le contexte n'y est pas encore posé — cf. isPlatformAdminClaims).
	plateforme := auth.Sign(testJWTSecret, auth.NewClaims("usr-plat", "Plateforme", model.RolePlatformAdmin, accID))
	if status, _ := doJSON(t, ts, http.MethodGet, "/api/dashboard", plateforme, nil); status != http.StatusOK {
		t.Fatalf("plateforme exempté de la suspension → 200 attendu, obtenu %d", status)
	}
}
