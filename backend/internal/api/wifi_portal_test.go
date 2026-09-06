// Package api — tests du endpoint public /api/wifi/site/{slug}/portal (N°35-c).
//
// Couvre :
//   - handleWifiPortal : 200 + PortalConfig valide pour slug existant ;
//   - handleWifiPortal : 404 code site_unknown pour slug inexistant ;
//   - CORS : Access-Control-Allow-Origin: * pour /api/wifi/site/* (origines
//     non listées dans ALLOWED_ORIGIN — simulateur de portail routeur) ;
//   - CORS : reflète l'origine listée dans ALLOWED_ORIGIN (frontend Vercel) ;
//   - CORS : N'envoie PAS Access-Control-Allow-Origin pour les routes API
//     privées (/api/dashboard) avec une origine non listée (sécurité S1 fail-closed).
package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mikcloud/hotspot-api/internal/hotpage"
	"mikcloud/hotspot-api/internal/model"
)

// seedWifiSite — crée un site WiFi jetable lié à un compte + routeur, et
// retourne le slug. Réutilise le pattern de seedRouterWithAccount (N°35-b).
func seedWifiSitePortal(t *testing.T, st interface {
	Lock()
	Unlock()
	Save()
	Data() *model.DB
}, slug, tenantName, waveLink string, withJoin bool, profilePrice int) {
	t.Helper()
	st.Lock()
	defer st.Unlock()
	db := st.Data()
	accID := "acc-wifi-" + slug
	db.Accounts = append(db.Accounts, model.Account{ID: accID, Name: tenantName})
	db.SettingsByAccount[accID] = model.Settings{
		Tenant: model.Tenant{
			Name:     tenantName,
			Currency: "XOF",
			Timezone: "Africa/Abidjan",
			WaveLink: waveLink,
		},
	}
	db.Routers = append(db.Routers, model.Router{
		ID:        "r-wifi-" + slug,
		AccountID: accID,
		Name:      "Router " + slug,
		Mode:      "agent",
		Status:    "online",
	})
	db.WifiSites = append(db.WifiSites, model.WifiSite{
		ID:        "ws-" + slug,
		AccountID: accID,
		Name:      "WiFi " + slug,
		Slug:      slug,
		RouterID:  "r-wifi-" + slug,
		Active:    true,
	})
	if withJoin {
		db.JoinLinks = append(db.JoinLinks, model.JoinLink{
			ID:        "jl-" + slug,
			AccountID: accID,
			Name:      "Lien " + slug,
			Token:     "jointok-" + slug,
			RouterID:  "r-wifi-" + slug,
		})
	}
	if profilePrice > 0 {
		db.Profiles = append(db.Profiles, model.Profile{
			ID:           "p-" + slug,
			AccountID:    accID,
			Name:         "Offre " + slug,
			Price:        profilePrice,
			ValidityDays: 1,
		})
	}
	st.Save()
}

// TestWifiPortalOK — GET /api/wifi/site/{slug}/portal retourne 200 + PortalConfig
// valide avec le tenant name, le slug, et l'API base.
func TestWifiPortalOK(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	seedWifiSitePortal(t, st, "cyber-test", "Cyber Test Tenant", "", false, 0)

	resp, err := http.Get(ts.URL + "/api/wifi/site/cyber-test/portal")
	if err != nil {
		t.Fatalf("GET : %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("statut %d, attendu 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `"tenantName":"Cyber Test Tenant"`) {
		t.Errorf("tenant name absent : %s", body)
	}
	if !strings.Contains(string(body), `"wifiSlug":"cyber-test"`) {
		t.Errorf("wifi slug absent : %s", body)
	}
	if !strings.Contains(string(body), `"apiBase":`) {
		t.Errorf("apiBase absent : %s", body)
	}
}

// TestWifiPortal404 — slug inexistant → 404 code site_unknown.
func TestWifiPortal404(t *testing.T) {
	_, ts := newTestServerWithStore(t)

	resp, err := http.Get(ts.URL + "/api/wifi/site/slug-inexistant/portal")
	if err != nil {
		t.Fatalf("GET : %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("statut %d, attendu 404", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "site_unknown") {
		t.Errorf("code site_unknown absent : %s", body)
	}
}

// TestWifiPortalFull — avec join link + offre + wave link → config complète.
func TestWifiPortalFull(t *testing.T) {
	t.Setenv("APP_PUBLIC_URL", "https://mikcloud.ftci.fr")
	st, ts := newTestServerWithStore(t)
	seedWifiSitePortal(t, st, "cyber-full", "Cyber Full", "https://pay.wave.com/m/M_x/c/ci/", true, 300)

	resp, err := http.Get(ts.URL + "/api/wifi/site/cyber-full/portal")
	if err != nil {
		t.Fatalf("GET : %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)
	if !strings.Contains(bodyStr, `"joinUrl":"https://mikcloud.ftci.fr/join/jointok-cyber-full"`) {
		t.Errorf("join URL absente : %s", bodyStr)
	}
	if !strings.Contains(bodyStr, `"wifiUrl":"https://mikcloud.ftci.fr/wifi/cyber-full"`) {
		t.Errorf("wifi URL absente : %s", bodyStr)
	}
	if !strings.Contains(bodyStr, `"priceFcfa":300`) {
		t.Errorf("offre 300F absente : %s", bodyStr)
	}
	if !strings.Contains(bodyStr, `"waveUrl":"https://pay.wave.com/m/M_x/c/ci/amount/300/"`) {
		t.Errorf("wave URL pré-construit absent : %s", bodyStr)
	}
	// N°46 — le fetch live expose joinEnabled (défaut effectif ON).
	if !strings.Contains(bodyStr, `"joinEnabled":true`) {
		t.Errorf("joinEnabled true absent de la config live : %s", bodyStr)
	}
}

// TestWifiPortalJoinButtonDisabled — N°46 : quand le réglage console
// tenant.joinButton est posé à false, la config LIVE (fetch du portail au
// chargement) expose "joinEnabled":false — le réglage s'applique SANS
// re-déploiement sur les portails déjà déployés.
func TestWifiPortalJoinButtonDisabled(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	seedWifiSitePortal(t, st, "cyber-join-off", "Cyber Join Off", "", true, 100)

	// Poser le réglage console : bouton d'inscription désactivé.
	st.Lock()
	db := st.Data()
	off := false
	for i := range db.WifiSites {
		if db.WifiSites[i].Slug == "cyber-join-off" {
			acc := db.WifiSites[i].AccountID
			settings := db.SettingsByAccount[acc]
			settings.Tenant.JoinButton = &off
			db.SettingsByAccount[acc] = settings
		}
	}
	st.Save()
	st.Unlock()

	resp, err := http.Get(ts.URL + "/api/wifi/site/cyber-join-off/portal")
	if err != nil {
		t.Fatalf("GET : %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `"joinEnabled":false`) {
		t.Errorf(`"joinEnabled":false absent de la config live : %s`, body)
	}
}

// Tests CORS du portail captif : gérés dans main_test.go (TestCorsMiddlewareWifiPublicOpen,
// TestCorsMiddlewareWifiSitesConsoleProtected) — corsMiddleware vit dans package main.
// Ici, on teste l'endpoint /api/wifi/site/{slug}/portal sans dépendre de CORS
// (les tests backend ne passent pas par le corsMiddleware, qui est appliqué
// dans main.go en production). La validation CORS est couverte par main_test.go.

// TestPortalConfigForSite — unitaire : buildPortalConfigForSite peuplé correctement
// (wifi slug toujours présent, join URL si routeur lié a un join actif, offres).
func TestPortalConfigForSite(t *testing.T) {
	t.Setenv("APP_PUBLIC_URL", "https://mikcloud.ftci.fr")
	st, _ := newTestServerWithStore(t)
	st.Lock()
	defer st.Unlock()
	db := st.Data()
	accID := "acc-unit"
	db.Accounts = append(db.Accounts, model.Account{ID: accID, Name: "Unit"})
	db.SettingsByAccount[accID] = model.Settings{Tenant: model.Tenant{Name: "Unit", WaveLink: "https://w/"}}
	db.Routers = append(db.Routers, model.Router{ID: "r1", AccountID: accID, Mode: "agent"})
	site := model.WifiSite{ID: "ws1", AccountID: accID, Slug: "unit-slug", RouterID: "r1", Active: true}
	db.WifiSites = append(db.WifiSites, site)
	db.JoinLinks = append(db.JoinLinks, model.JoinLink{ID: "jl1", AccountID: accID, Token: "tkn1", RouterID: "r1"})
	db.Profiles = append(db.Profiles, model.Profile{ID: "p1", AccountID: accID, Name: "1h", Price: 100, ValidityDays: 1})

	router := &db.Routers[0]
	req := httptest.NewRequest("GET", "https://api.test/api/wifi/site/unit-slug/portal", nil)
	cfg := buildPortalConfigForSite(db, &site, router, req)

	if cfg.TenantName != "Unit" {
		t.Errorf("TenantName = %q", cfg.TenantName)
	}
	if cfg.WifiSlug != "unit-slug" {
		t.Errorf("WifiSlug = %q", cfg.WifiSlug)
	}
	if cfg.WifiURL != "https://mikcloud.ftci.fr/wifi/unit-slug" {
		t.Errorf("WifiURL = %q", cfg.WifiURL)
	}
	if !strings.Contains(cfg.JoinURL, "/join/tkn1") {
		t.Errorf("JoinURL = %q", cfg.JoinURL)
	}
	if len(cfg.Offers) != 1 || cfg.Offers[0].PriceFcfa != 100 {
		t.Errorf("Offers = %+v", cfg.Offers)
	}
	if cfg.Offers[0].WaveURL != "https://w/amount/100/" {
		t.Errorf("WaveURL = %q", cfg.Offers[0].WaveURL)
	}
	// Vérifie le type : doit être hotpage.PortalConfig (pas un map).
	var _ hotpage.PortalConfig = cfg
}

// TestPortalConfigForSiteNoRouter — si le routeur a été supprimé (cas théorique),
// buildPortalConfigForSite ne panique pas et renvoie un cfg sans joinURL.
func TestPortalConfigForSiteNoRouter(t *testing.T) {
	st, _ := newTestServerWithStore(t)
	st.Lock()
	defer st.Unlock()
	db := st.Data()
	accID := "acc-norouter"
	db.Accounts = append(db.Accounts, model.Account{ID: accID, Name: "NoRouter"})
	db.SettingsByAccount[accID] = model.Settings{Tenant: model.Tenant{Name: "NoRouter"}}
	site := model.WifiSite{ID: "ws-nr", AccountID: accID, Slug: "no-router-slug", RouterID: "r-inexistant", Active: true}
	db.WifiSites = append(db.WifiSites, site)
	req := httptest.NewRequest("GET", "https://api.test/api/wifi/site/no-router-slug/portal", nil)
	cfg := buildPortalConfigForSite(db, &site, nil, req)
	if cfg.WifiSlug != "no-router-slug" {
		t.Errorf("WifiSlug = %q", cfg.WifiSlug)
	}
	if cfg.JoinURL != "" {
		t.Errorf("JoinURL = %q, attendu vide (pas de routeur)", cfg.JoinURL)
	}
}
