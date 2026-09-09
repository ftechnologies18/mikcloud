// Tests N°72 — compteur de bande passante sortante : classification des
// chemins, reset journalier UTC, et comptage réel par le middleware (le
// rapport sync-status compte la requête qui le demande).
package api

import (
	"net/http"
	"testing"
	"time"

	"mikcloud/hotspot-api/internal/auth"
	"mikcloud/hotspot-api/internal/model"
)

// TestEgressCategory — les cinq canaux du décompte N°72. Miroir exact du
// découpage public/console de l'allowlist d'authentification : « /api/wifi/
// site/ » (invités) ne matche PAS « /api/wifi/sites » ni « /api/wifi/
// guests » (console — le slash final tranche).
func TestEgressCategory(t *testing.T) {
	for _, tc := range []struct {
		path string
		want string
	}{
		{"/agent/cmd", "agents"},
		{"/agent/result", "agents"},
		{"/agent/register", "agents"},
		{"/portal/ab12cd34/login.html", "portail"},
		{"/portal/ab12cd34/css/bootstrap.min.css", "portail"},
		{"/api/portal/track", "portail"},
		{"/api/wifi/site/mon-slug", "portail"},
		{"/api/wifi/site/mon-slug/claim", "portail"},
		{"/api/wifi/site/mon-slug/status", "portail"},
		{"/api/wifi/site/mon-slug/portal", "portail"},
		{"/api/wifi/sites", "console"},
		{"/api/wifi/guests", "console"},
		{"/api/media/acc1/2026/ab.jpg", "medias"},
		{"/api/users", "console"},
		{"/api/admin/sync-status", "console"},
		{"/api/dashboard", "console"},
		{"/", "autre"},
		{"/sw.js", "autre"},
		{"/nimporte/quoi", "autre"},
	} {
		if got := egressCategory(tc.path); got != tc.want {
			t.Fatalf("%s : catégorie %q attendue, obtenue %q", tc.path, tc.want, got)
		}
	}
}

// TestEgressStatsDailyReset — compteurs du jour, ordre canonique et
// remise à zéro au changement de jour UTC (la fenêtre de facturation
// Render est calée sur le mois calendaire UTC).
func TestEgressStatsDailyReset(t *testing.T) {
	e := newEgressStats()
	day1 := time.Date(2026, 9, 9, 10, 0, 0, 0, time.UTC)
	day2 := day1.Add(24 * time.Hour)

	e.startRequest("agents", day1)
	e.addBytes("agents", 2048, day1)
	e.startRequest("portail", day1)
	e.addBytes("portail", 0, day1) // zéro octet : ignoré silencieusement

	s1 := e.snapshot(day1)
	if s1.Day != "2026-09-09" {
		t.Fatalf("jour attendu 2026-09-09, obtenu %q", s1.Day)
	}
	if s1.TotalRequests != 2 || s1.TotalBytes != 2048 {
		t.Fatalf("jour 1 : 2 requêtes / 2048 octets attendus, obtenu %d / %d", s1.TotalRequests, s1.TotalBytes)
	}
	if len(s1.Categories) != 5 {
		t.Fatalf("5 catégories canoniques attendues, obtenu %d", len(s1.Categories))
	}
	if s1.Categories[0].Name != "agents" || s1.Categories[4].Name != "autre" {
		t.Fatalf("ordre canonique attendu (agents…autre), obtenu %q…%q", s1.Categories[0].Name, s1.Categories[4].Name)
	}
	if s1.Categories[0].Requests != 1 || s1.Categories[0].Bytes != 2048 {
		t.Fatalf("agents : 1 requête / 2048 octets attendus, obtenu %d / %d", s1.Categories[0].Requests, s1.Categories[0].Bytes)
	}
	if s1.Categories[1].Requests != 1 || s1.Categories[1].Bytes != 0 {
		t.Fatalf("portail : 1 requête / 0 octet attendus, obtenu %d / %d", s1.Categories[1].Requests, s1.Categories[1].Bytes)
	}

	// Changement de jour : tout repart de zéro, le jour est actualisé.
	s2 := e.snapshot(day2)
	if s2.Day != "2026-09-10" {
		t.Fatalf("jour attendu 2026-09-10, obtenu %q", s2.Day)
	}
	if s2.TotalRequests != 0 || s2.TotalBytes != 0 {
		t.Fatalf("jour 2 : compteurs remis à zéro attendus, obtenu %d / %d", s2.TotalRequests, s2.TotalBytes)
	}
	e.addBytes("console", 100, day2)
	s3 := e.snapshot(day2)
	if s3.TotalBytes != 100 || s3.Categories[3].Bytes != 100 {
		t.Fatalf("jour 2 après activité : 100 octets attendus, obtenu total=%d console=%d", s3.TotalBytes, s3.Categories[3].Bytes)
	}
}

// TestEgressMiddlewareCountsByCategory — au travers du serveur réel : la
// santé GET / écrit des octets en « autre », la requête sync-status est
// comptée en « console » (startRequest AVANT le handler : la requête en
// cours figure dans son propre rapport).
func TestEgressMiddlewareCountsByCategory(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	seedUser(t, st, "usr-plat-eg", "", "usr-plat-eg", model.RolePlatformAdmin)
	platform := auth.Sign(testJWTSecret, auth.NewClaims("usr-plat-eg", "Plateforme", model.RolePlatformAdmin, "", 0))

	// Une requête publique hors API (catégorie « autre »), en clair.
	resp, body := doGzipReq(t, ts, http.MethodGet, "/", "", "identity", "")
	if resp.StatusCode != http.StatusOK || body == "" {
		t.Fatalf("santé attendue 200 avec corps, obtenu %d (%q)", resp.StatusCode, body)
	}

	status, out := doJSON(t, ts, "GET", "/api/admin/sync-status", platform, nil)
	if status != http.StatusOK {
		t.Fatalf("200 attendu, obtenu %d", status)
	}
	bw, ok := out["bandwidth"].(map[string]any)
	if !ok {
		t.Fatalf("bloc bandwidth absent : %v", out["bandwidth"])
	}
	cats, ok := bw["categories"].([]any)
	if !ok || len(cats) != 5 {
		t.Fatalf("5 catégories attendues, obtenu %v", cats)
	}
	findCat := func(name string) map[string]any {
		for _, e := range cats {
			if m, ok := e.(map[string]any); ok && m["name"] == name {
				return m
			}
		}
		return nil
	}
	autre := findCat("autre")
	if autre == nil || autre["bytes"].(float64) <= 0 {
		t.Fatalf("la santé GET / doit avoir écrit des octets en « autre » : %v", autre)
	}
	console := findCat("console")
	if console == nil || console["requests"].(float64) < 1 {
		t.Fatalf("la requête sync-status en cours doit être comptée en « console » : %v", console)
	}
	if bw["totalRequests"].(float64) < 2 {
		t.Fatalf("au moins 2 requêtes comptées attendues, obtenu %v", bw["totalRequests"])
	}
}
