// Package api — tests du templating par compte au servage (N°35-b).
//
// Couvre :
//   - buildPortalConfig : résolution tenant, wifi slug, join url, offres ;
//   - handlePortalFile avec templating : login.html servi contient le bloc
//     <script id="mikcloud-config"> avec le JSON du compte ;
//   - handlePortalFile sans templating : les assets binaires (logo.png) sont
//     servis tels quels (pas de substitution) ;
//   - sécurité : un tenant nom malveillant est échappé dans le JSON.
package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"mikcloud/hotspot-api/internal/agent"
	"mikcloud/hotspot-api/internal/hotpage"
	"mikcloud/hotspot-api/internal/model"
)

// seedRouterWithAccount — crée un routeur agent + un compte + des settings +
// (optionnel) un site WiFi + un lien join + des profils, et retourne le token.
// Le token est hashé côté store (comme en production).
func seedRouterWithAccount(t *testing.T, st interface {
	Lock()
	Unlock()
	Save()
	Data() *model.DB
}, token string, opts struct {
	tenantName  string
	wifiSlug    string
	joinActive  bool
	profileName string
	profilePrc  int
	waveLink    string
	bannerUrl   string // N°45 — bannière du portail (data URL ou https)
}) string {
	t.Helper()
	st.Lock()
	defer st.Unlock()
	db := st.Data()
	accID := "acc-portal-" + token
	db.Accounts = append(db.Accounts, model.Account{ID: accID, Name: opts.tenantName})
	// Settings avec le tenant name + wave link.
	db.SettingsByAccount[accID] = model.Settings{
		Tenant: model.Tenant{
			Name:      opts.tenantName,
			Currency:  "XOF",
			Timezone:  "Africa/Abidjan",
			WaveLink:  opts.waveLink,
			BannerURL: opts.bannerUrl,
		},
	}
	// Routeur agent.
	db.Routers = append(db.Routers, model.Router{
		ID:             "r-portal-" + token,
		AccountID:      accID,
		Name:           "Router " + token,
		Mode:           "agent",
		Status:         "online",
		AgentTokenHash: agent.HashToken(token),
	})
	// Site WiFi lié au routeur.
	if opts.wifiSlug != "" {
		db.WifiSites = append(db.WifiSites, model.WifiSite{
			ID:        "ws-" + token,
			AccountID: accID,
			Name:      "WiFi " + token,
			Slug:      opts.wifiSlug,
			RouterID:  "r-portal-" + token,
			Active:    true,
		})
	}
	// Lien join lié au routeur.
	if opts.joinActive {
		db.JoinLinks = append(db.JoinLinks, model.JoinLink{
			ID:        "jl-" + token,
			AccountID: accID,
			Name:      "Lien " + token,
			Token:     "jointok-" + token,
			RouterID:  "r-portal-" + token,
		})
	}
	// Profil à prix.
	if opts.profileName != "" {
		db.Profiles = append(db.Profiles, model.Profile{
			ID:           "p-" + token,
			AccountID:    accID,
			Name:         opts.profileName,
			Price:        opts.profilePrc,
			ValidityDays: 1,
		})
	}
	st.Save()
	return token
}

// TestBuildPortalConfigTenant — le tenant name et l'API base sont résolus.
func TestBuildPortalConfigTenant(t *testing.T) {
	st, _ := newTestServerWithStore(t)
	token := seedRouterWithAccount(t, st, "tok-cfg-tenant", struct {
		tenantName  string
		wifiSlug    string
		joinActive  bool
		profileName string
		profilePrc  int
		waveLink    string
		bannerUrl   string
	}{tenantName: "Mon Cyber", wifiSlug: "", joinActive: false, profileName: "", profilePrc: 0, waveLink: ""})

	st.Lock()
	db := st.Data()
	router := routerByToken(db, token)
	if router == nil {
		t.Fatal("routeur non trouvé")
	}
	// Requête factice pour résoudre l'API base.
	req, _ := http.NewRequest("GET", "https://api.test/portal/"+token+"/login.html", nil)
	cfg := buildPortalConfig(db, router, req)
	st.Unlock()

	if cfg.TenantName != "Mon Cyber" {
		t.Errorf("TenantName = %q, attendu %q", cfg.TenantName, "Mon Cyber")
	}
	if cfg.APIBase == "" {
		t.Error("APIBase vide")
	}
	if cfg.WifiSlug != "" {
		t.Errorf("WifiSlug = %q, attendu vide", cfg.WifiSlug)
	}
	if cfg.JoinURL != "" {
		t.Errorf("JoinURL = %q, attendu vide", cfg.JoinURL)
	}
	if len(cfg.Offers) != 0 {
		t.Errorf("Offers = %d, attendu 0", len(cfg.Offers))
	}
}

// TestBuildPortalConfigFull — wifi slug, join url, offres et wave link sont
// résolus quand le routeur a ces entités liées.
func TestBuildPortalConfigFull(t *testing.T) {
	// Forcer APP_PUBLIC_URL pour que les URL absolues soient construites.
	t.Setenv("APP_PUBLIC_URL", "https://mikcloud.ftci.fr")
	st, _ := newTestServerWithStore(t)
	token := seedRouterWithAccount(t, st, "tok-cfg-full", struct {
		tenantName  string
		wifiSlug    string
		joinActive  bool
		profileName string
		profilePrc  int
		waveLink    string
		bannerUrl   string
	}{tenantName: "Cyber Full", wifiSlug: "cyber-full", joinActive: true, profileName: "1h", profilePrc: 100, waveLink: "https://pay.wave.com/m/M_x/c/ci/"})

	st.Lock()
	db := st.Data()
	router := routerByToken(db, token)
	req, _ := http.NewRequest("GET", "https://api.test/portal/"+token+"/login.html", nil)
	cfg := buildPortalConfig(db, router, req)
	st.Unlock()

	if cfg.TenantName != "Cyber Full" {
		t.Errorf("TenantName = %q", cfg.TenantName)
	}
	if cfg.WifiSlug != "cyber-full" {
		t.Errorf("WifiSlug = %q, attendu cyber-full", cfg.WifiSlug)
	}
	if cfg.WifiURL != "https://mikcloud.ftci.fr/wifi/cyber-full" {
		t.Errorf("WifiURL = %q", cfg.WifiURL)
	}
	if !strings.Contains(cfg.JoinURL, "/join/jointok-tok-cfg-full") {
		t.Errorf("JoinURL = %q", cfg.JoinURL)
	}
	if cfg.WaveLink != "https://pay.wave.com/m/M_x/c/ci/" {
		t.Errorf("WaveLink = %q", cfg.WaveLink)
	}
	if len(cfg.Offers) != 1 {
		t.Fatalf("Offers = %d, attendu 1", len(cfg.Offers))
	}
	o := cfg.Offers[0]
	if o.Name != "1h" || o.PriceFcfa != 100 {
		t.Errorf("offer = %+v", o)
	}
	if o.WaveURL != "https://pay.wave.com/m/M_x/c/ci/amount/100/" {
		t.Errorf("WaveURL = %q", o.WaveURL)
	}
}

// TestPortalServePersonalized — GET /portal/{token}/login.html contient le
// bloc <script id="mikcloud-config"> avec le JSON du compte.
func TestPortalServePersonalized(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	t.Setenv("APP_PUBLIC_URL", "https://mikcloud.ftci.fr")
	seedRouterWithAccount(t, st, "tok-personalized", struct {
		tenantName  string
		wifiSlug    string
		joinActive  bool
		profileName string
		profilePrc  int
		waveLink    string
		bannerUrl   string
	}{tenantName: "Cyber Personnalisé", wifiSlug: "cyber-perso", joinActive: true, profileName: "24h", profilePrc: 300, waveLink: ""})

	resp, err := http.Get(ts.URL + "/portal/tok-personalized/login.html")
	if err != nil {
		t.Fatalf("GET login.html : %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("statut %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)
	// Le bloc <script id="mikcloud-config"> doit être présent.
	if !strings.Contains(bodyStr, `id="mikcloud-config"`) {
		t.Error("bloc <script id=\"mikcloud-config\"> absent du login.html servi")
	}
	// Le JSON du compte doit contenir le tenant name « Cyber Personnalisé ».
	if !strings.Contains(bodyStr, "Cyber Personnalisé") {
		t.Error("tenant name absent du login.html servi")
	}
	// Le JSON doit contenir le slug WiFi.
	if !strings.Contains(bodyStr, "cyber-perso") {
		t.Error("wifi slug absent du login.html servi")
	}
	// Le JSON doit contenir l'offre 24h à 300 F.
	if !strings.Contains(bodyStr, `"24h"`) || !strings.Contains(bodyStr, `"priceFcfa":300`) {
		t.Error("offre 24h/300F absente du login.html servi")
	}
	// Le marqueur {{MIKCLOUD_CONFIG_JSON}} ne doit PLUS être présent (substitué).
	if strings.Contains(bodyStr, "{{MIKCLOUD_CONFIG_JSON}}") {
		t.Error("marqueur non substitué dans le login.html servi")
	}
	// Vérifier que le JSON est valide en l'extrayant du bloc.
	start := strings.Index(bodyStr, `id="mikcloud-config">`) + len(`id="mikcloud-config">`)
	end := strings.Index(bodyStr[start:], "</script>")
	if start <= 0 || end <= 0 {
		t.Fatal("bloc config introuvable")
	}
	jsonStr := bodyStr[start : start+end]
	var cfg hotpage.PortalConfig
	if err := json.Unmarshal([]byte(jsonStr), &cfg); err != nil {
		t.Fatalf("JSON invalide : %v\n%s", err, jsonStr)
	}
	if cfg.TenantName != "Cyber Personnalisé" {
		t.Errorf("TenantName = %q", cfg.TenantName)
	}
}

// TestPortalServeBanner — la bannière du tenant (N°45) est propagée dans la
// config JSON du login.html servi : buildPortalConfig lit settings.Tenant.BannerURL
// et le bloc <script id="mikcloud-config"> embarque "bannerUrl" tel quel.
func TestPortalServeBanner(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	seedRouterWithAccount(t, st, "tok-banner", struct {
		tenantName  string
		wifiSlug    string
		joinActive  bool
		profileName string
		profilePrc  int
		waveLink    string
		bannerUrl   string
	}{tenantName: "Cyber Bannière", wifiSlug: "", joinActive: false, profileName: "", profilePrc: 0, waveLink: "", bannerUrl: "https://r2.example.com/cyber/banner.jpg"})

	resp, err := http.Get(ts.URL + "/portal/tok-banner/login.html")
	if err != nil {
		t.Fatalf("GET login.html : %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("statut %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	start := strings.Index(string(body), `id="mikcloud-config">`) + len(`id="mikcloud-config">`)
	end := strings.Index(string(body)[start:], "</script>")
	if start <= 0 || end <= 0 {
		t.Fatal("bloc config introuvable")
	}
	var cfg hotpage.PortalConfig
	if err := json.Unmarshal([]byte(string(body)[start:start+end]), &cfg); err != nil {
		t.Fatalf("JSON config invalide : %v", err)
	}
	if cfg.BannerURL != "https://r2.example.com/cyber/banner.jpg" {
		t.Errorf("BannerURL = %q, attendu l'URL https du tenant", cfg.BannerURL)
	}
}

// TestPortalServeBinaryNotTemplated — les assets binaires (logo.png) sont
// servis tels quels, PAS passés par Personalize (qui casserait les octets).
func TestPortalServeBinaryNotTemplated(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	seedRouterWithAccount(t, st, "tok-binary", struct {
		tenantName  string
		wifiSlug    string
		joinActive  bool
		profileName string
		profilePrc  int
		waveLink    string
		bannerUrl   string
	}{tenantName: "Binary Test", wifiSlug: "", joinActive: false, profileName: "", profilePrc: 0, waveLink: ""})

	resp, err := http.Get(ts.URL + "/portal/tok-binary/img/logo.png")
	if err != nil {
		t.Fatalf("GET logo.png : %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("statut %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	// En-tête PNG : 89 50 4E 47 0D 0A 1A 0A — preuve que le binaire est intact.
	if len(body) < 8 || body[0] != 0x89 || body[1] != 0x50 || body[2] != 0x4E || body[3] != 0x47 {
		t.Errorf("en-tête PNG invalide — binaire altéré par Personalize ? premiers octets : % x", body[:8])
	}
	// L'asset ne doit PAS contenir le tenant name (il n'y a aucun marqueur à
	// substituer dans un PNG, mais on vérifie que Personalize n'a pas été appliqué).
	if strings.Contains(string(body), "Binary Test") {
		t.Error("le binaire contient le tenant name — Personalize a été appliqué à tort")
	}
}

// TestPortalServeSecurityTenantInjection — un tenant name malveillant ne
// casse pas le bloc <script type="application/json"> (échappement JSON) et ne
// crée pas de balise <script> exécutable.
func TestPortalServeSecurityTenantInjection(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	seedRouterWithAccount(t, st, "tok-inject", struct {
		tenantName  string
		wifiSlug    string
		joinActive  bool
		profileName string
		profilePrc  int
		waveLink    string
		bannerUrl   string
	}{tenantName: `</script><script>alert(1)</script>`, wifiSlug: "", joinActive: false, profileName: "", profilePrc: 0, waveLink: ""})

	resp, err := http.Get(ts.URL + "/portal/tok-inject/login.html")
	if err != nil {
		t.Fatalf("GET : %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)
	// Aucune balise <script> exécutable d'injection ne doit apparaître. Le JSON
	// échappé contient \u003cscript\u003ealert(1)\u003c/script\u003e (inoffensif :
	// c'est du texte dans un bloc application/json, jamais interprété comme JS).
	// On cherche une balise HTML RÉELLE <script> contenant alert(1) — qui ne
	// doit JAMAIS exister dans le body.
	if strings.Contains(bodyStr, "<script>alert(1)") {
		t.Errorf("balise <script> exécutable d'injection détectée :\n%s", bodyStr)
	}
	// Le bloc <script type="application/json" id="mikcloud-config"> ne doit pas
	// être fermé prématurément par le tenant name. On compte les occurrences de
	// la chaîne littérale `</script>` (HTML réel, pas échappé) et on vérifie
	// qu'elles correspondent au nombre attendu du template (les </script> de
	// fin de chaque <script> du template : Typed/Swiper + le nôtre).
	// Si l'injection avait réussi, il y aurait un </script> supplémentaire
	// (celui qui ferme le faux <script>injecté).
	// Plus simple : vérifier que `<script>alert(1)</script>` (balise réelle) n'existe pas.
	if strings.Contains(bodyStr, "<script>alert(1)</script>") {
		t.Errorf("injection <script>alert(1)</script> exécutable détectée :\n%s", bodyStr)
	}
	// Le tenant name malveillant DOIT être présent dans le JSON (échappé).
	if !strings.Contains(bodyStr, `\u003c/script\u003e`) {
		t.Error("le tenant name malveillant n'est pas échappé en \\u003c/script\\u003e dans le JSON")
	}
}
