// Package api — tests des validations bannière du portail (N°45) sur
// PUT /api/settings. La bannière (Tenant.BannerURL) est affichée en tête de
// la page de login du portail captif : le contrat n'accepte qu'une data URL
// image ≤ 500 Ko ou une URL https:// (Cloudflare R2) — tout le reste est
// refusé en 400, la valeur vide retire la bannière.
package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// putBanner — PUT /api/settings avec bannerUrl (formes plates + nested, même
// corps défensif que le front) et retourne le statut + les settings renvoyés.
func putBanner(t *testing.T, ts *httptest.Server, token, banner string) (int, map[string]any) {
	t.Helper()
	body := map[string]any{
		"bannerUrl": banner,
		"tenant":    map[string]any{"bannerUrl": banner},
	}
	return doJSON(t, ts, "PUT", "/api/settings", token, body)
}

// TestSettingsBannerValidation — matrice des validations bannerUrl.
func TestSettingsBannerValidation(t *testing.T) {
	ts := newTestServer(t)
	token, _, _ := registerAccount(t, ts, "gerant-banner", "")

	// data URL image valide (petit PNG 1×1) → 200, persistée.
	smallDataURL := "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="
	status, out := putBanner(t, ts, token, smallDataURL)
	if status != http.StatusOK {
		t.Fatalf("data URL valide doit passer (200), obtenu %d : %v", status, out)
	}
	if tenant, ok := out["tenant"].(map[string]any); !ok || tenant["bannerUrl"] != smallDataURL {
		t.Errorf("bannerUrl non persistée : %v", out["tenant"])
	}

	// URL https (Cloudflare R2 / hébergeur externe) → 200, persistée.
	r2URL := "https://pub-abc123.r2.dev/banners/cyber.jpg"
	status, out = putBanner(t, ts, token, r2URL)
	if status != http.StatusOK {
		t.Fatalf("URL https doit passer (200), obtenu %d : %v", status, out)
	}
	if tenant, ok := out["tenant"].(map[string]any); !ok || tenant["bannerUrl"] != r2URL {
		t.Errorf("bannerUrl https non persistée : %v", out["tenant"])
	}

	// Schémas interdits → 400 (le portail exige https ; pas de http clair,
	// pas de javascript: ni d'URL relative).
	for _, bad := range []string{"http://insecure.example/b.jpg", "javascript:alert(1)", "/img/relative.jpg", "ftp://files.example/b.jpg"} {
		if status, _ := putBanner(t, ts, token, bad); status != http.StatusBadRequest {
			t.Errorf("bannerUrl %q doit être refusée (400), obtenu %d", bad, status)
		}
	}

	// data URL non image → 400.
	if status, _ := putBanner(t, ts, token, "data:text/html;base64,PHNjcmlwdD4="); status != http.StatusBadRequest {
		t.Errorf("data:text/html doit être refusée (400), obtenu %d", status)
	}

	// data URL image > 500 Ko → 400.
	huge := "data:image/jpeg;base64," + strings.Repeat("A", 500*1024)
	if status, _ := putBanner(t, ts, token, huge); status != http.StatusBadRequest {
		t.Errorf("data URL > 500 Ko doit être refusée (400), obtenu %d", status)
	}

	// Vide → 200, bannière retirée (omitempty : la clé disparaît du JSON).
	status, out = putBanner(t, ts, token, "")
	if status != http.StatusOK {
		t.Fatalf("bannière vide doit passer (200), obtenu %d", status)
	}
	tenant, _ := out["tenant"].(map[string]any)
	if v, exists := tenant["bannerUrl"]; exists && v != "" {
		t.Errorf("bannerUrl doit être retirée (absente/vide), obtenu : %v", v)
	}
}
