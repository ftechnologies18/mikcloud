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

// putJoinButton — PUT /api/settings avec joinButton (formes plates + nested,
// même corps défensif que le front) et retourne le statut + les settings.
func putJoinButton(t *testing.T, ts *httptest.Server, token string, enabled bool) (int, map[string]any) {
	t.Helper()
	body := map[string]any{
		"joinButton": enabled,
		"tenant":     map[string]any{"joinButton": enabled},
	}
	return doJSON(t, ts, "PUT", "/api/settings", token, body)
}

// TestSettingsJoinButton — N°46 : le réglage du bouton « S'inscrire » du
// portail captif est persisté via PUT /api/settings (plats + nested), la
// valeur nil reste « inchangé » (défaut effectif ON), et GET reflète l'état.
func TestSettingsJoinButton(t *testing.T) {
	ts := newTestServer(t)
	token, _, _ := registerAccount(t, ts, "gerant-join", "")

	// Défaut : le champ n'a jamais été touché → absent du JSON (nil =
	// omitempty, défaut effectif ON côté portail). On ne vérifie que
	// l'absence de valeur explicite fausse.
	_, out := doJSON(t, ts, "GET", "/api/settings", token, nil)
	if tenant, ok := out["tenant"].(map[string]any); ok {
		if v, exists := tenant["joinButton"]; exists && v != false {
			t.Errorf("joinButton par défaut doit être absent (nil) ou false, obtenu : %v", v)
		}
	}

	// Désactivation explicite → 200, persistée.
	status, out := putJoinButton(t, ts, token, false)
	if status != http.StatusOK {
		t.Fatalf("joinButton=false doit passer (200), obtenu %d : %v", status, out)
	}
	if tenant, ok := out["tenant"].(map[string]any); !ok || tenant["joinButton"] != false {
		t.Errorf("joinButton=false non persistée : %v", out["tenant"])
	}

	// PUT sans le champ (ex. le front enregistre la bannière seule) → le
	// réglage reste inchangé (false), le nil ne le réactive pas.
	_, out = putBanner(t, ts, token, "https://r2.example.com/x.jpg")
	if tenant, ok := out["tenant"].(map[string]any); !ok || tenant["joinButton"] != false {
		t.Errorf("joinButton doit rester false après un PUT sans le champ : %v", out["tenant"])
	}

	// Réactivation → 200, persistée, et visible par un GET suivant.
	status, out = putJoinButton(t, ts, token, true)
	if status != http.StatusOK {
		t.Fatalf("joinButton=true doit passer (200), obtenu %d : %v", status, out)
	}
	if tenant, ok := out["tenant"].(map[string]any); !ok || tenant["joinButton"] != true {
		t.Errorf("joinButton=true non persistée : %v", out["tenant"])
	}
	_, out = doJSON(t, ts, "GET", "/api/settings", token, nil)
	if tenant, ok := out["tenant"].(map[string]any); !ok || tenant["joinButton"] != true {
		t.Errorf("GET doit refléter joinButton=true : %v", out["tenant"])
	}
}
