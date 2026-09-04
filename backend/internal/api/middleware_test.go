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
	st, ts := newTestServerWithStore(t)
	ownerToken, accID, _ := registerAccount(t, ts, "proprio-mw", "")

	// Utilisateurs RÉELS pour les rôles forgés (S1-A3 : le middleware refuse
	// tout porteur de token inconnu du store ; le rôle « reseller », hors
	// AdminUser, reste exempté de ce contrôle).
	seedUser(t, st, "usr-m", accID, "usr-m", model.RoleManager)
	seedUser(t, st, "usr-p", "", "usr-p", model.RolePlatformAdmin)
	// V4 (audit revendeurs) : le Mode Vente exige en plus l'existence du
	// revendeur — « usr-r » doit donc exister pour que son token réponde 200.
	seedSellReseller(t, st, "usr-r", accID, "Usr R", "prepaid", 0)

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
			auth.Sign(testJWTSecret, auth.NewClaims("usr-r", "R", "reseller", accID, 0)), http.StatusForbidden},
		{"revendeur autorisé sur /api/sell/me", http.MethodGet, "/api/sell/me",
			auth.Sign(testJWTSecret, auth.NewClaims("usr-r", "R", "reseller", accID, 0)), http.StatusOK},
		{"revendeur inexistant révoqué (V4)", http.MethodGet, "/api/sell/me",
			auth.Sign(testJWTSecret, auth.NewClaims("res-fantome", "F", "reseller", accID, 0)), http.StatusForbidden},
		{"manager bloqué sur requireRole(3)", http.MethodGet, "/api/team",
			auth.Sign(testJWTSecret, auth.NewClaims("usr-m", "M", model.RoleManager, accID, 0)), http.StatusForbidden},
		{"owner passe sur requireRole(3)", http.MethodGet, "/api/team", ownerToken, http.StatusOK},
		{"plateforme traité comme owner", http.MethodGet, "/api/team",
			auth.Sign(testJWTSecret, auth.NewClaims("usr-p", "P", model.RolePlatformAdmin, "", 0)), http.StatusOK},
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
	// S1-A3 : le porteur doit exister dans le store.
	seedUser(t, st, "usr-plat", "", "usr-plat", model.RolePlatformAdmin)
	plateforme := auth.Sign(testJWTSecret, auth.NewClaims("usr-plat", "Plateforme", model.RolePlatformAdmin, accID, 0))
	if status, _ := doJSON(t, ts, http.MethodGet, "/api/dashboard", plateforme, nil); status != http.StatusOK {
		t.Fatalf("plateforme exempté de la suspension → 200 attendu, obtenu %d", status)
	}
}

// TestSessionRevocation — S1-A3 : un token dont l'époque de session (claim
// « ver ») ne correspond plus à SessionEpoch est refusé immédiatement ; un
// porteur supprimé du store l'est aussi — sans attendre les 24 h du token.
func TestSessionRevocation(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	token, accID, usrID := registerAccount(t, ts, "proprio-revoc-mw", "")

	if status, _ := doJSON(t, ts, http.MethodGet, "/api/auth/me", token, nil); status != http.StatusOK {
		t.Fatalf("session valable avant révocation, obtenu %d", status)
	}

	// Révocation : incrément de l'époque de session (comme le fait un
	// changement de mot de passe ou une réinitialisation par l'owner).
	st.Lock()
	for i := range st.Data().Users {
		if st.Data().Users[i].ID == usrID {
			st.Data().Users[i].SessionEpoch++
			break
		}
	}
	st.Save()
	st.Unlock()

	status, body := doJSON(t, ts, http.MethodGet, "/api/auth/me", token, nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("token à l'époque périmée doit être refusé (401), obtenu %d", status)
	}
	if msg, _ := body["error"].(string); msg != "Session révoquée — reconnectez-vous" {
		t.Fatalf("message de révocation attendu, obtenu %v", body)
	}

	// Un token réémis avec la NOUVELLE époque repasse.
	fresh := auth.Sign(testJWTSecret, auth.NewClaims(usrID, "Gérant", "owner", accID, 1))
	if status, _ := doJSON(t, ts, http.MethodGet, "/api/auth/me", fresh, nil); status != http.StatusOK {
		t.Fatalf("token à l'époque courante doit passer, obtenu %d", status)
	}

	// Suppression du membre : même un token à jour devient invalide.
	st.Lock()
	users := st.Data().Users[:0]
	for _, u := range st.Data().Users {
		if u.ID != usrID {
			users = append(users, u)
		}
	}
	st.Data().Users = users
	st.Save()
	st.Unlock()

	status, body = doJSON(t, ts, http.MethodGet, "/api/auth/me", fresh, nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("token d'un porteur supprimé doit être refusé (401), obtenu %d", status)
	}
	if msg, _ := body["error"].(string); msg != "Compte utilisateur supprimé — reconnectez-vous" {
		t.Fatalf("message de suppression attendu, obtenu %v", body)
	}
}
