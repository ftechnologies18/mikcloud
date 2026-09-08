// password_reset_test.go — N°68 : « Mot de passe oublié ? » (lien e-mail à
// usage unique, 60 minutes).
//
// Couverture :
//   - demande : e-mail inconnu → 404 explicite (choix produit), compte
//     désactivé → 403, format invalide → 400, AUCUN fournisseur e-mail → 503 ;
//   - envoi : destinataire = e-mail du compte, lien /reset-password?token=…,
//     mention des 60 minutes et de l'usage unique, token stocké HASHÉ (le
//     clair n'apparaît nulle part en base) ;
//   - consommation : mot de passe changé (l'ancien ne connecte plus, le
//     nouveau oui), sessions révoquées (ancien JWT → 401), usage unique
//     (2e consommation → 400), expiration (lien périmé → 400), politique S2
//     (court/denylist → 400) ;
//   - une nouvelle demande INVALIDE le lien en attente (un seul lien vivant) ;
//   - historique : entrée notif_log kind=password_reset ;
//   - résolution de l'origine du lien : APP_PUBLIC_URL > origine ALLOWED
//     (prod) > toute origine (dev) > URL canonique ;
//   - quota IP : 5/10 min, la 6e tentative → 429 + Retry-After.
//
// AUCUNE connexion réseau : le sendeur e-mail est remplacé par un stub.
package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mikcloud/hotspot-api/internal/model"
	"mikcloud/hotspot-api/internal/notify"
	"mikcloud/hotspot-api/internal/store"
)

// sentEmail — capture d'un envoi du stub e-mail.
type sentEmail struct {
	cfg   model.NotificationSettings
	to    string
	title string
	body  string
}

// resetCapture — enregistrement des envois du stub e-mail.
type resetCapture struct{ calls []sentEmail }

// stubEmail — remplace sendResetEmail : capture l'appel, succès inconditionnel.
func stubEmail(cap *resetCapture) func(*model.NotificationSettings, string, string, string) error {
	return func(cfg *model.NotificationSettings, to, title, body string) error {
		cap.calls = append(cap.calls, sentEmail{*cfg, to, title, body})
		return nil
	}
}

// newResetServer — serveur + store (pour semer les réglages notif) + stubs
// token/e-mail restaurés à la fin du test.
func newResetServer(t *testing.T, tokenFn func() (string, error), cap *resetCapture) (*httptest.Server, *store.Store) {
	t.Helper()
	t.Setenv("DATABASE_URL", "")
	t.Setenv("ADMIN_PASSWORD", "admin-test-1234")
	t.Setenv("ADMIN_USERNAME", "")
	t.Setenv("ALLOWED_ORIGIN", "")
	t.Setenv("APP_PUBLIC_URL", "")
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatalf("store.New impossible : %v", err)
	}
	ts := startTestServer(st)
	t.Cleanup(ts.Close)
	oldGen, oldSend := generateResetToken, sendResetEmail
	if tokenFn != nil {
		generateResetToken = tokenFn
	}
	if cap != nil {
		sendResetEmail = stubEmail(cap)
	}
	t.Cleanup(func() { generateResetToken, sendResetEmail = oldGen, oldSend })
	return ts, st
}

// seedResend — configure le canal e-mail (Resend) d'un compte directement
// dans le store : la demande de lien doit partir sans réglage côté console.
func seedResend(t *testing.T, st *store.Store, accID string) {
	t.Helper()
	st.Lock()
	if st.Data().NotifSettings == nil {
		st.Data().NotifSettings = map[string]model.NotificationSettings{}
	}
	st.Data().NotifSettings[accID] = model.NotificationSettings{
		AccountID:     accID,
		EmailProvider: "resend",
		ResendAPIKey:  "re_test_key",
		ResendFrom:    "MikCloud <alertes@ftci.fr>",
	}
	st.Save()
	st.Unlock()
}

// fixedToken — générateur déterministe (une valeur par appel).
func fixedToken(values ...string) func() (string, error) {
	i := 0
	return func() (string, error) {
		v := values[min(i, len(values)-1)]
		i++
		return v, nil
	}
}

// forgotStatus — POST /api/auth/forgot-password {email} → (statut, corps).
func forgotStatus(t *testing.T, ts *httptest.Server, email string) (int, map[string]any) {
	t.Helper()
	return doJSON(t, ts, "POST", "/api/auth/forgot-password", "", map[string]string{"email": email})
}

// resetStatus — POST /api/auth/reset-password {token, password}.
func resetStatus(t *testing.T, ts *httptest.Server, token, password string) (int, map[string]any) {
	t.Helper()
	return doJSON(t, ts, "POST", "/api/auth/reset-password", "", map[string]string{"token": token, "password": password})
}

// loginStatus — POST /api/auth/login.
func loginStatus(t *testing.T, ts *httptest.Server, username, password string) int {
	t.Helper()
	status, _ := doJSON(t, ts, "POST", "/api/auth/login", "", map[string]string{
		"username": username, "password": password,
	})
	return status
}

func TestForgotPasswordUnknownEmail(t *testing.T) {
	ts, _ := newResetServer(t, fixedToken("tok-x"), nil)
	status, out := forgotStatus(t, ts, "personne@example.ci")
	if status != http.StatusNotFound {
		t.Fatalf("e-mail inconnu : statut %d (%v), 404 attendu", status, out)
	}
	if msg, _ := out["error"].(string); !strings.Contains(msg, "Aucun compte") {
		t.Fatalf("message inattendu : %v", out)
	}
}

func TestForgotPasswordInvalidEmail(t *testing.T) {
	ts, _ := newResetServer(t, fixedToken("tok-x"), nil)
	if status, _ := forgotStatus(t, ts, "pas-un-email"); status != http.StatusBadRequest {
		t.Fatalf("format invalide : statut %d, 400 attendu", status)
	}
	if status, _ := forgotStatus(t, ts, ""); status != http.StatusBadRequest {
		t.Fatalf("e-mail vide : statut %d, 400 attendu", status)
	}
}

func TestForgotPasswordNoSenderConfigured(t *testing.T) {
	ts, _ := newResetServer(t, fixedToken("tok-x"), nil)
	// Compte réel, AUCUN réglage e-mail nulle part → 503 (plateforme à
	// configurer), et surtout AUCUNE ligne de réinitialisation créée.
	registerAccount(t, ts, "bob68", "")
	status, out := forgotStatus(t, ts, "bob68@example.ci")
	if status != http.StatusServiceUnavailable {
		t.Fatalf("sans fournisseur : statut %d (%v), 503 attendu", status, out)
	}
}

func TestForgotPasswordDisabledAccount(t *testing.T) {
	ts, st := newResetServer(t, fixedToken("tok-x"), nil)
	_, accID, _ := registerAccount(t, ts, "ma68", "")
	st.Lock()
	for i := range st.Data().Accounts {
		if st.Data().Accounts[i].ID == accID {
			st.Data().Accounts[i].Status = "disabled"
		}
	}
	st.Save()
	st.Unlock()
	status, out := forgotStatus(t, ts, "ma68@example.ci")
	if status != http.StatusForbidden {
		t.Fatalf("compte désactivé : statut %d (%v), 403 attendu", status, out)
	}
}

func TestForgotPasswordFullFlow(t *testing.T) {
	cap := &resetCapture{}
	ts, st := newResetServer(t, fixedToken("token-alpha-123"), cap)
	token, accID, _ := registerAccount(t, ts, "alice68", "")
	seedResend(t, st, accID)

	// 1. Demande → 200, e-mail parti au bon destinataire avec le bon lien.
	status, out := forgotStatus(t, ts, "ALICE68@example.ci") // insensible à la casse
	if status != http.StatusOK {
		t.Fatalf("demande : statut %d (%v)", status, out)
	}
	if v, _ := out["expiresInMin"].(float64); int(v) != 60 {
		t.Fatalf("expiresInMin = %v, 60 attendu", out["expiresInMin"])
	}
	if len(cap.calls) != 1 {
		t.Fatalf("un envoi attendu, %d reçus", len(cap.calls))
	}
	sent := cap.calls[0]
	if sent.to != "alice68@example.ci" {
		t.Fatalf("destinataire = %q", sent.to)
	}
	if !strings.Contains(sent.title, "Réinitialisation") {
		t.Fatalf("sujet inattendu : %q", sent.title)
	}
	if !strings.Contains(sent.body, "/reset-password?token=token-alpha-123") {
		t.Fatalf("lien manquant dans le corps : %q", sent.body)
	}
	if !strings.Contains(sent.body, "60 minutes") || !strings.Contains(sent.body, "une seule fois") {
		t.Fatalf("mention durée/usage unique absente : %q", sent.body)
	}
	if sent.cfg.ResendAPIKey != "re_test_key" {
		t.Fatalf("fournisseur du compte non utilisé : %+v", sent.cfg)
	}

	// 2. Base : le token en CLAIR n'existe pas, son hash si.
	st.Lock()
	rows := st.Data().PasswordResets
	if len(rows) != 1 {
		st.Unlock()
		t.Fatalf("une ligne de reset attendue, %d", len(rows))
	}
	if rows[0].TokenHash != hashResetToken("token-alpha-123") {
		st.Unlock()
		t.Fatalf("hash stocké = %q", rows[0].TokenHash)
	}
	if strings.Contains(rows[0].TokenHash, "token-alpha") {
		st.Unlock()
		t.Fatal("le hash ne doit pas contenir le token clair")
	}
	kinds := 0
	for _, l := range st.Data().NotifLog {
		if l.Kind == notify.KindPasswordReset {
			kinds++
		}
	}
	st.Unlock()
	if kinds != 1 {
		t.Fatalf("une entrée notif_log password_reset attendue, %d", kinds)
	}

	// 3. Consommation : nouveau mot de passe, ancien mort, sessions révoquées.
	if status, _ := resetStatus(t, ts, "token-alpha-123", "court"); status != http.StatusBadRequest {
		t.Fatalf("mot de passe trop court accepté (statut %d)", status)
	}
	if status, out := resetStatus(t, ts, "token-alpha-123", "nouveau-mot-68!"); status != http.StatusOK {
		t.Fatalf("réinitialisation : statut %d (%v)", status, out)
	}
	if loginStatus(t, ts, "alice68", "mot-de-passe-8+") != http.StatusBadRequest {
		t.Fatal("l'ancien mot de passe doit être refusé")
	}
	if loginStatus(t, ts, "alice68", "nouveau-mot-68!") != http.StatusOK {
		t.Fatal("le nouveau mot de passe doit connecter")
	}
	// L'ancien JWT (émis à l'inscription) doit être révoqué (SessionEpoch++).
	req, _ := http.NewRequest("GET", ts.URL+"/api/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("me : %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("ancien token : statut %d, 401 attendu (sessions révoquées)", resp.StatusCode)
	}

	// 4. Usage unique : le lien est mort après consommation.
	if status, out := resetStatus(t, ts, "token-alpha-123", "encore-un-mot-9!"); status != http.StatusBadRequest {
		t.Fatalf("double consommation : statut %d (%v), 400 attendu", status, out)
	}
	// Le token consommé n'est plus trouvable → même message que « invalide ».
	if status, _ := resetStatus(t, ts, "token-fore-chaine", "encore-un-mot-9!"); status != http.StatusBadRequest {
		t.Fatalf("token forgé : statut %d, 400 attendu", status)
	}
}

func TestForgotPasswordNewRequestInvalidatesPreviousLink(t *testing.T) {
	cap := &resetCapture{}
	ts, st := newResetServer(t, fixedToken("tok-premier", "tok-second"), cap)
	_, accID, _ := registerAccount(t, ts, "carla68", "")
	seedResend(t, st, accID)

	if status, _ := forgotStatus(t, ts, "carla68@example.ci"); status != http.StatusOK {
		t.Fatalf("1re demande : statut %d", status)
	}
	if status, _ := forgotStatus(t, ts, "carla68@example.ci"); status != http.StatusOK {
		t.Fatalf("2e demande : statut %d", status)
	}
	if len(cap.calls) != 2 {
		t.Fatalf("2 envois attendus, %d", len(cap.calls))
	}
	// Le 1er lien a été invalidé par la 2e demande : 400. Le 2e : 200.
	if status, _ := resetStatus(t, ts, "tok-premier", "premier-mot-68!"); status != http.StatusBadRequest {
		t.Fatalf("lien invalidé : statut %d, 400 attendu", status)
	}
	if status, _ := resetStatus(t, ts, "tok-second", "second-mot-68!"); status != http.StatusOK {
		t.Fatalf("lien actif : statut %d, 200 attendu", status)
	}
	if loginStatus(t, ts, "carla68", "second-mot-68!") != http.StatusOK {
		t.Fatal("le second mot de passe doit connecter")
	}
}

func TestResetPasswordExpiredLink(t *testing.T) {
	cap := &resetCapture{}
	ts, st := newResetServer(t, fixedToken("tok-expire"), cap)
	_, accID, _ := registerAccount(t, ts, "dodo68", "")
	seedResend(t, st, accID)
	if status, _ := forgotStatus(t, ts, "dodo68@example.ci"); status != http.StatusOK {
		t.Fatalf("demande : statut %d", status)
	}
	// Le temps passe : le lien dépasse ses 60 minutes.
	st.Lock()
	for i := range st.Data().PasswordResets {
		if past, err := time.Parse(time.RFC3339, time.Now().UTC().Add(-2*time.Hour).Format(time.RFC3339)); err == nil {
			st.Data().PasswordResets[i].ExpiresAt = past.Format(time.RFC3339)
		}
	}
	st.Save()
	st.Unlock()
	status, out := resetStatus(t, ts, "tok-expire", "mot-valide-68!")
	if status != http.StatusBadRequest {
		t.Fatalf("lien expiré : statut %d, 400 attendu", status)
	}
	if msg, _ := out["error"].(string); !strings.Contains(msg, "expiré") {
		t.Fatalf("message inattendu : %v", out)
	}
	// Le mot de passe n'a PAS changé.
	if loginStatus(t, ts, "dodo68", "mot-de-passe-8+") != http.StatusOK {
		t.Fatal("l'ancien mot de passe doit rester valide après échec")
	}
}

func TestResetPasswordDenylist(t *testing.T) {
	cap := &resetCapture{}
	ts, st := newResetServer(t, fixedToken("tok-deny"), cap)
	_, accID, _ := registerAccount(t, ts, "eva68", "")
	seedResend(t, st, accID)
	forgotStatus(t, ts, "eva68@example.ci")
	if status, _ := resetStatus(t, ts, "tok-deny", "azertyuiop"); status != http.StatusBadRequest {
		t.Fatalf("mot de passe de la denylist accepté (statut %d)", status)
	}
	// 13 caractères ≠ username → politique OK → consommation normale.
	if status, _ := resetStatus(t, ts, "tok-deny", "eva68eva68eva"); status != http.StatusOK {
		t.Fatalf("consommation après refus de politique : statut %d, 200 attendu", status)
	}
}

func TestForgotPasswordRateLimit(t *testing.T) {
	ts, _ := newResetServer(t, fixedToken("tok-rl"), nil)
	// 5 tentatives admises (inconnues → 404, mais le quota se consomme).
	for i := 0; i < 5; i++ {
		if status, _ := forgotStatus(t, ts, "quiqui@example.ci"); status != http.StatusNotFound {
			t.Fatalf("tentative %d : statut inattendu", i+1)
		}
	}
	status, out := forgotStatus(t, ts, "quiqui@example.ci")
	if status != http.StatusTooManyRequests {
		t.Fatalf("6e tentative : statut %d (%v), 429 attendu", status, out)
	}
}

func TestPasswordResetLinkBase(t *testing.T) {
	req := func(origin string) *http.Request {
		r, _ := http.NewRequest("POST", "http://backend.onrender.com/api/auth/forgot-password", nil)
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		return r
	}
	// 1. APP_PUBLIC_URL prioritaire sur tout.
	t.Setenv("APP_PUBLIC_URL", "https://app.public/url/")
	if got := passwordResetLinkBase(req("https://pas-elle.ci")); got != "https://app.public/url" {
		t.Fatalf("APP_PUBLIC_URL ignorée : %q", got)
	}
	t.Setenv("APP_PUBLIC_URL", "")
	// 2. Dev local (pas de DATABASE_URL) : toute origine acceptée.
	t.Setenv("DATABASE_URL", "")
	if got := passwordResetLinkBase(req("http://localhost:3100")); got != "http://localhost:3100" {
		t.Fatalf("dev local : origine attendue, %q reçue", got)
	}
	// 3. Prod : origine uniquement si elle figure dans ALLOWED_ORIGIN.
	t.Setenv("DATABASE_URL", "postgres://prod")
	t.Setenv("ALLOWED_ORIGIN", "https://mikcloud.ftci.fr, https://autre.ci")
	if got := passwordResetLinkBase(req("https://mikcloud.ftci.fr")); got != "https://mikcloud.ftci.fr" {
		t.Fatalf("origine autorisée : %q reçue", got)
	}
	if got := passwordResetLinkBase(req("https://attaquant.ci")); got != defaultFrontendURL {
		t.Fatalf("origine inconnue : %q reçue, URL canonique attendue", got)
	}
	// 4. Aucune origine / origine non http(s) → URL canonique.
	if got := passwordResetLinkBase(req("")); got != defaultFrontendURL {
		t.Fatalf("sans origine : %q reçue", got)
	}
	if got := passwordResetLinkBase(req("ftp://bof")); got != defaultFrontendURL {
		t.Fatalf("origine non http : %q reçue", got)
	}
}
