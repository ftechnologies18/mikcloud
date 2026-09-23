// Tests N°183 — sonde du canal d'images Cloudflare R2 (media_health.go) et
// distinction 503/404 de handleMediaGet.
//
// La sonde réseau est injectée (r2VerifyCall) : ces tests ne touchent JAMAIS
// Cloudflare ; ils vérifient les verdicts (valid/invalid/unknown/non
// configuré), le cache 5 min (un seul appel réseau par fenêtre, même en
// rafale de sync-status) et le contrat HTTP de l'endpoint public /api/media.
package api

import (
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"mikcloud/hotspot-api/internal/auth"
	"mikcloud/hotspot-api/internal/model"
)

// stubR2Probe — remplace l'appel réseau par une réponse déterministe et
// compte les invocations ; restaure tout à la fin du test.
func stubR2Probe(t *testing.T, body string, err error) *int32 {
	t.Helper()
	var calls int32
	orig := r2VerifyCall
	r2VerifyCall = func(token string) ([]byte, error) {
		atomic.AddInt32(&calls, 1)
		if err != nil {
			return nil, err
		}
		return []byte(body), nil
	}
	t.Cleanup(func() {
		r2VerifyCall = orig
		r2ProbeMu.Lock()
		r2ProbeCache = nil
		r2ProbeMu.Unlock()
	})
	return &calls
}

// resetR2Cache — vide le cache de sonde entre deux sous-cas.
func resetR2Cache(t *testing.T) {
	t.Helper()
	r2ProbeMu.Lock()
	r2ProbeCache = nil
	r2ProbeMu.Unlock()
}

// TestMediaHealthUnconfigured — sans R2_ACCOUNT_ID/R2_API_TOKEN : bloc
// configured=false, AUCUN appel réseau (le verdict ne dépend pas de
// Cloudflare).
func TestMediaHealthUnconfigured(t *testing.T) {
	calls := stubR2Probe(t, `{"success":true}`, nil)
	t.Setenv("R2_ACCOUNT_ID", "")
	t.Setenv("R2_API_TOKEN", "")

	h := r2MediaHealthSnapshot()
	if h.Configured {
		t.Fatal("configured doit être false sans variables R2")
	}
	if h.TokenStatus != "" {
		t.Fatalf("tokenStatus attendu vide, obtenu %q", h.TokenStatus)
	}
	if h.Bucket != "" || h.CheckedAt != "" {
		t.Fatalf("aucun contexte attendu hors configuration : %+v", h)
	}
	if n := atomic.LoadInt32(calls); n != 0 {
		t.Fatalf("aucun appel Cloudflare attendu hors configuration, %d effectué(s)", n)
	}
}

// TestMediaHealthVerdicts — les trois verdicts configurés : valid, invalid,
// unknown (erreur réseau). Le compartiment lu dans la config est remonté.
func TestMediaHealthVerdicts(t *testing.T) {
	t.Setenv("R2_ACCOUNT_ID", "acc123")
	t.Setenv("R2_API_TOKEN", "jeton-test")
	t.Setenv("R2_BUCKET", "mikcloud-media")

	cases := []struct {
		name string
		body string
		err  error
		want string
	}{
		{"valide", `{"success":true,"result":{"status":"active","id":"…"}}`, nil, "valid"},
		{"invalide", `{"success":false,"errors":[{"code":1000,"message":"Invalid API Token"}]}`, nil, "invalid"},
		{"réseauKO", "", fmt.Errorf("timeout"), "unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stubR2Probe(t, tc.body, tc.err)
			resetR2Cache(t)
			h := r2MediaHealthSnapshot()
			if !h.Configured {
				t.Fatal("configured doit être true (variables posées)")
			}
			if h.TokenStatus != tc.want {
				t.Fatalf("tokenStatus attendu %q, obtenu %q", tc.want, h.TokenStatus)
			}
			if h.Bucket != "mikcloud-media" {
				t.Fatalf("bucket attendu mikcloud-media, obtenu %q", h.Bucket)
			}
			if h.CheckedAt == "" {
				t.Fatal("checkedAt attendu (RFC3339)")
			}
		})
	}
}

// TestMediaHealthCache — le verdict est en cache : une rafale de
// sync-status ne produit qu'UN appel Cloudflare par fenêtre de 5 min.
func TestMediaHealthCache(t *testing.T) {
	t.Setenv("R2_ACCOUNT_ID", "acc123")
	t.Setenv("R2_API_TOKEN", "jeton-test")
	t.Setenv("R2_BUCKET", "mikcloud-media")
	calls := stubR2Probe(t, `{"success":true}`, nil)
	resetR2Cache(t)

	for i := 0; i < 5; i++ {
		if h := r2MediaHealthSnapshot(); h.TokenStatus != "valid" {
			t.Fatalf("itération %d : verdict attendu valid, obtenu %q", i, h.TokenStatus)
		}
	}
	if n := atomic.LoadInt32(calls); n != 1 {
		t.Fatalf("1 appel Cloudflare attendu pour 5 lectures (cache), %d effectué(s)", n)
	}
}

// TestSyncStatusMediaBlock — le bloc « media » est servi par
// GET /api/admin/sync-status : présent en mode JSON (la sonde ne dépend PAS
// de la persistance), lisible par l'admin plateforme, caché pour personne
// d'autre (garde existante inchangée).
func TestSyncStatusMediaBlock(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	seedUser(t, st, "usr-plat-media", "", "usr-plat-media", model.RolePlatformAdmin)
	t.Setenv("R2_ACCOUNT_ID", "acc123")
	t.Setenv("R2_API_TOKEN", "jeton-test")
	stubR2Probe(t, `{"success":true}`, nil)
	resetR2Cache(t)

	platform := auth.Sign(testJWTSecret, auth.NewClaims("usr-plat-media", "Plateforme", model.RolePlatformAdmin, "", 0))
	status, out := doJSON(t, ts, "GET", "/api/admin/sync-status", platform, nil)
	if status != http.StatusOK {
		t.Fatalf("admin plateforme : 200 attendu, obtenu %d", status)
	}
	media, ok := out["media"].(map[string]any)
	if !ok {
		t.Fatalf("bloc media absent de sync-status : %v", out["media"])
	}
	if cfg, _ := media["configured"].(bool); !cfg {
		t.Fatalf("media.configured attendu true (variables posées) : %v", media)
	}
	if st, _ := media["tokenStatus"].(string); st != "valid" {
		t.Fatalf("media.tokenStatus attendu valid, obtenu %v", media["tokenStatus"])
	}
}

// TestMediaGetUnconfigured503 — N°183 : lecture SANS configuration → 503
// media_unconfigured (et non plus un 404 « Image introuvable » qui masquait
// l'état) ; AVEC configuration mais clé inconnue du format → 404. La
// réponse 503 ne part jamais en fetch R2 (aucun appel réseau).
func TestMediaGetUnconfigured503(t *testing.T) {
	ts := newTestServer(t)
	t.Setenv("R2_ACCOUNT_ID", "")
	t.Setenv("R2_API_TOKEN", "")
	calls := stubR2Probe(t, `{"success":true}`, nil)

	// Clé de format VALIDE mais canal non configuré : 503 explicite.
	url := ts.URL + "/api/media/media/acc-1/2026/0123456789abcdef0123456789abcdef.jpg"
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("requête impossible : %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("non configuré : 503 attendu, obtenu %d", resp.StatusCode)
	}
	body := make([]byte, 512)
	n, _ := resp.Body.Read(body)
	if !strings.Contains(string(body[:n]), "media_unconfigured") {
		t.Fatalf("code media_unconfigured attendu dans le corps : %s", string(body[:n]))
	}

	// Configuré (compte factice) mais clé hors contrat : 404 inchangé.
	t.Setenv("R2_ACCOUNT_ID", "acc123")
	t.Setenv("R2_API_TOKEN", "jeton-test")
	resp2, err := http.Get(ts.URL + "/api/media/autre-chose.jpg")
	if err != nil {
		t.Fatalf("requête impossible : %v", err)
	}
	defer func() { _ = resp2.Body.Close() }()
	if resp2.StatusCode != http.StatusNotFound {
		t.Fatalf("clé hors contrat : 404 attendu, obtenu %d", resp2.StatusCode)
	}
	if n := atomic.LoadInt32(calls); n != 0 {
		t.Fatalf("aucun appel réseau attendu (la clé est rejetée avant tout fetch), %d effectué(s)", n)
	}
}
