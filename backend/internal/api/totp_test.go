// totp_test.go — tests de la 2FA TOTP (S4) : vecteurs officiels RFC 6238
// pour la génération des codes, puis intégration HTTP bout en bout du cycle
// setup → login sans code → login mauvais code → activate → login avec code
// → me (statut) → disable → retour sans code.

package api

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestTOTPCodeRFC6238 — vecteurs officiels du RFC 6238 (annexe B, HMAC-SHA1)
// : le secret base32 correspond à l'ASCII « 12345678901234567890 » ; les
// codes à 6 chiffres sont les 6 derniers chiffres des codes 8 chiffres du
// RFC.
func TestTOTPCodeRFC6238(t *testing.T) {
	secret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	cases := []struct {
		unix int64
		want string
	}{
		{59, "287082"},          // 94287082
		{1111111109, "081804"},  // 07081804
		{1111111111, "050471"},  // 14050471
		{1234567890, "005924"},  // 89005924
		{2000000000, "279037"},  // 69279037
		{20000000000, "353130"}, // 65353130
	}
	for _, c := range cases {
		got, err := totpCodeAt(secret, c.unix)
		if err != nil {
			t.Fatalf("T=%d : %v", c.unix, err)
		}
		if got != c.want {
			t.Fatalf("T=%d : attendu %s, obtenu %s", c.unix, c.want, got)
		}
	}
}

// TestTOTPVerifyWindow — la fenêtre ±1 accepte le code de la période
// précédente et suivante, mais pas au-delà.
func TestTOTPVerifyWindow(t *testing.T) {
	secret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	cur := time.Now().Unix()
	// Code de la période suivante (+1) : accepté.
	next, _ := totpCodeAt(secret, cur+totpPeriod)
	if !verifyTOTP(secret, next) {
		t.Fatalf("code période +1 doit être accepté (%s)", next)
	}
	// Code de la période -1 : accepté.
	prev, _ := totpCodeAt(secret, cur-totpPeriod)
	if !verifyTOTP(secret, prev) {
		t.Fatalf("code période -1 doit être accepté (%s)", prev)
	}
	// Code trop ancien (-2 périodes) : refusé.
	old, _ := totpCodeAt(secret, cur-2*totpPeriod)
	if verifyTOTP(secret, old) {
		t.Fatalf("code période -2 doit être refusé (%s)", old)
	}
	// Code mal formé : refusé.
	if verifyTOTP(secret, "abc") || verifyTOTP(secret, "12345") || verifyTOTP(secret, "1234567") {
		t.Fatal("codes mal formés doivent être refusés")
	}
}

// TestTwoFactorFlowE2E — cycle complet sur la surface HTTP.
func TestTwoFactorFlowE2E(t *testing.T) {
	ts := newTestServer(t)
	token, _, _ := registerAccount(t, ts, "gerant-2fa", "")

	// 1. setup : secret + URL otpauth cohérente.
	status, out := doJSON(t, ts, "POST", "/api/auth/2fa/setup", token, nil)
	if status != http.StatusOK {
		t.Fatalf("setup : statut %d %v", status, out)
	}
	secret, _ := out["secret"].(string)
	otpauth, _ := out["otpauth"].(string)
	if len(secret) != 32 { // 20 octets → base32 sans padding = 32 caractères
		t.Fatalf("secret inattendu : %q (%d car.)", secret, len(secret))
	}
	if !strings.HasPrefix(otpauth, "otpauth://totp/MikCloud:") || !strings.Contains(otpauth, "secret="+secret) {
		t.Fatalf("URL otpauth inattendue : %s", otpauth)
	}

	// 2. activate avec le code courant : 200 — le secret « en attente » ne
	// déclenche PAS encore la 2FA (aucun verrouillage avant activation
	// volontaire) ; c'est l'activation qui rend le login exigeant.
	code, err := totpCodeAt(secret, time.Now().Unix())
	if err != nil {
		t.Fatal(err)
	}
	status, out = doJSON(t, ts, "POST", "/api/auth/2fa/activate", token, map[string]string{"code": code})
	if status != http.StatusOK || out["ok"] != true {
		t.Fatalf("activate : attendu 200/ok, obtenu %d %v", status, out)
	}

	// 3. login sans code : 401 + code machine totp_required.
	status, out = doJSON(t, ts, "POST", "/api/auth/login", "", map[string]string{
		"username": "gerant-2fa", "password": "mot-de-passe-8+",
	})
	if status != http.StatusUnauthorized || out["code"] != "totp_required" {
		t.Fatalf("login sans code : attendu 401/totp_required, obtenu %d %v", status, out)
	}

	// 4. login avec mauvais code : 400 générique (aucun oracle).
	status, out = doJSON(t, ts, "POST", "/api/auth/login", "", map[string]string{
		"username": "gerant-2fa", "password": "mot-de-passe-8+", "code": "000000",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("login mauvais code : attendu 400, obtenu %d %v", status, out)
	}

	// 5. me : statut totpEnabled exposé.
	status, out = doJSON(t, ts, "GET", "/api/auth/me", token, nil)
	if status != http.StatusOK {
		t.Fatalf("me : statut %d", status)
	}
	if u, _ := out["user"].(map[string]any); u == nil || u["totpEnabled"] != true {
		t.Fatalf("me : totpEnabled attendu, obtenu %v", out)
	}

	// 6. login avec le code correct : 200 + token.
	status, out = doJSON(t, ts, "POST", "/api/auth/login", "", map[string]string{
		"username": "gerant-2fa", "password": "mot-de-passe-8+", "code": code,
	})
	if status != http.StatusOK {
		t.Fatalf("login avec code : attendu 200, obtenu %d %v", status, out)
	}

	// 7. disable avec mauvais mot de passe : 400.
	status, _ = doJSON(t, ts, "POST", "/api/auth/2fa/disable", token, map[string]string{"password": "mauvais-mot-de-passe-1"})
	if status != http.StatusBadRequest {
		t.Fatalf("disable mauvais mdp : attendu 400, obtenu %d", status)
	}

	// 8. disable avec le bon mot de passe : 200, puis login sans code repasse.
	status, _ = doJSON(t, ts, "POST", "/api/auth/2fa/disable", token, map[string]string{"password": "mot-de-passe-8+"})
	if status != http.StatusOK {
		t.Fatalf("disable : attendu 200, obtenu %d", status)
	}
	status, _ = doJSON(t, ts, "POST", "/api/auth/login", "", map[string]string{
		"username": "gerant-2fa", "password": "mot-de-passe-8+",
	})
	if status != http.StatusOK {
		t.Fatalf("login post-disable : attendu 200, obtenu %d", status)
	}
}
