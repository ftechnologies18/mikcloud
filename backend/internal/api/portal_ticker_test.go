// Package api — tests du bandeau animé sous le logo du portail captif
// (N°138) : le gérant remplace les 3 messages historiques de l'effet
// Typed.js par SES messages (≤ 5, texte brut 1-80 car.).
//
// Couvre :
//   - portalTickerList : décodage défensif du JSON tenant (invalides/vides
//     filtrés, plafond 5 et longueur 80 re-vérifiés — défense en profondeur
//     contre une ligne héritée d'un appel API direct) ;
//   - PUT /api/settings : matrice des validations portalTicker (≤ 5 messages
//     1-80 car., entrées vides ignorées, liste vide = retour aux 3 messages
//     par défaut du template) ;
//   - servage /portal/{token}/login.html : le marqueur TICKER_JSON embarque
//     les messages DU TENANT dans l'init de Typed.js (chemin routeur, config
//     figée au déploiement) ;
//   - GET /api/wifi/site/{slug}/portal : le fetch live (N°48) porte AUSSI
//     les messages (chemin hybride, appliqué par le bloc 10 de applyConfig).
package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"mikcloud/hotspot-api/internal/hotpage"
	"mikcloud/hotspot-api/internal/model"
)

// TestPortalTickerList — matrice du décodeur défensif : un JSON invalide,
// vide ou une liste sans aucun message exploitable donne nil (le portail
// garde ses 3 messages historiques, jamais cassé) ; le plafond 5 et la
// longueur 80 sont re-vérifiés ici même si l'API les a déjà validés
// (défense en profondeur).
func TestPortalTickerList(t *testing.T) {
	cases := []struct {
		name string
		json string
		want []string
	}{
		{"vide", "", nil},
		{"JSON invalide", `{"a":1}`, nil},
		{"liste vide", `[]`, nil},
		{"objet au lieu de liste", `{"msg":"salut"}`, nil},
		{"un message", `["Wifi de la maison !"]`, []string{"Wifi de la maison !"}},
		{"trois messages", `["Fibre 100 Mbps","Ouvert 7j/7","Payez par Wave"]`, []string{"Fibre 100 Mbps", "Ouvert 7j/7", "Payez par Wave"}},
		{"vide filtré", `["","  ","Internet rapide"]`, []string{"Internet rapide"}},
		{"espaces trimés", `[" Fibre 100 Mbps "]`, []string{"Fibre 100 Mbps"}},
		{"plafond 5 re-vérifié", `["1","2","3","4","5","6"]`, []string{"1", "2", "3", "4", "5"}},
		{"81 caractères filtré", `["` + strings.Repeat("a", 81) + `","OK"]`, []string{"OK"}},
		{"80 caractères accepté", `["` + strings.Repeat("a", 80) + `"]`, []string{strings.Repeat("a", 80)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := portalTickerList(model.Tenant{PortalTicker: tc.json})
			if tc.want == nil {
				if got != nil {
					t.Fatalf("portalTickerList(%q) = %v, attendu nil", tc.json, got)
				}
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("portalTickerList(%q) = %v, attendu %v", tc.json, got, tc.want)
			}
		})
	}
}

// putTicker — PUT /api/settings avec portalTicker (formes plates + nested,
// même corps défensif que le front) et retourne le statut + les settings.
func putTicker(t *testing.T, ts *httptest.Server, token string, msgs []string) (int, map[string]any) {
	t.Helper()
	body := map[string]any{
		"portalTicker": msgs,
		"tenant":       map[string]any{"portalTicker": msgs},
	}
	return doJSON(t, ts, "PUT", "/api/settings", token, body)
}

// tickerOf — extrait tenant.portalTicker de la réponse PUT : le champ voyage
// comme CHAÎNE JSON (pattern N°55 des listes du tenant). Retourne nil si
// absent/vide.
func tickerOf(t *testing.T, out map[string]any) []string {
	t.Helper()
	tenant, ok := out["tenant"].(map[string]any)
	if !ok {
		return nil
	}
	raw, ok := tenant["portalTicker"].(string)
	if !ok || raw == "" {
		return nil
	}
	var list []string
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		t.Fatalf("portalTicker n'est pas un JSON de messages valide : %q (%v)", raw, err)
	}
	return list
}

// TestSettingsPortalTickerValidation — matrice des validations portalTicker
// sur PUT /api/settings (même discipline que les slides N°136).
func TestSettingsPortalTickerValidation(t *testing.T) {
	ts := newTestServer(t)
	token, _, _ := registerAccount(t, ts, "gerant-ticker", "")

	// Défaut : le champ n'a jamais été touché → absent du JSON (omitempty).
	status, out := doJSON(t, ts, "GET", "/api/settings", token, nil)
	if status != http.StatusOK {
		t.Fatalf("GET settings : %d", status)
	}
	if s := tickerOf(t, out); s != nil {
		t.Errorf("portalTicker doit être absent par défaut, obtenu %v", s)
	}

	// 3 messages valides → 200, persistés dans l'ordre (trimés).
	want := []string{"Fibre 100 Mbps chez vous !", "Ouvert 7j/7 de 8h à 22h", "Payez vos tickets par Wave"}
	if status, out = putTicker(t, ts, token, want); status != http.StatusOK {
		t.Fatalf("3 messages valides doivent passer (200), obtenu %d : %v", status, out)
	}
	if got := tickerOf(t, out); !reflect.DeepEqual(got, want) {
		t.Errorf("portalTicker non persisté : %v, attendu %v", got, want)
	}

	// GET reflète l'état persisté.
	if status, out = doJSON(t, ts, "GET", "/api/settings", token, nil); status != http.StatusOK {
		t.Fatalf("GET settings : %d", status)
	}
	if got := tickerOf(t, out); !reflect.DeepEqual(got, want) {
		t.Errorf("GET ne reflète pas portalTicker : %v, attendu %v", got, want)
	}

	// 6 messages → 400 (le bandeau anime au plus 5 messages).
	six := []string{"1", "2", "3", "4", "5", "6"}
	if status, _ = putTicker(t, ts, token, six); status != http.StatusBadRequest {
		t.Errorf("6 messages doivent être refusés (400), obtenu %d", status)
	}

	// Message > 80 caractères → 400 (le bandeau tient une ligne lisible).
	long := strings.Repeat("a", 81)
	if status, _ = putTicker(t, ts, token, []string{long}); status != http.StatusBadRequest {
		t.Errorf("message > 80 car. doit être refusé (400), obtenu %d", status)
	}

	// Entrées vides ignorées (la console n'en envoie pas, l'API tolère).
	if status, out = putTicker(t, ts, token, []string{"", "Garde ce message", "  "}); status != http.StatusOK {
		t.Fatalf("entrées vides ignorées doivent passer (200), obtenu %d : %v", status, out)
	}
	if got := tickerOf(t, out); !reflect.DeepEqual(got, []string{"Garde ce message"}) {
		t.Errorf("entrées vides non ignorées : %v", got)
	}

	// Liste vide → 200, messages retirés (retour aux 3 messages par défaut
	// du template ; omitempty : la clé disparaît du JSON).
	if status, out = putTicker(t, ts, token, []string{}); status != http.StatusOK {
		t.Fatalf("liste vide doit passer (200), obtenu %d : %v", status, out)
	}
	if got := tickerOf(t, out); got != nil {
		t.Errorf("portalTicker doit être retiré (absent), obtenu %v", got)
	}
}

// TestPortalServeTicker — les messages DU TENANT sont servis dans l'init
// Typed.js du login.html (marqueur TICKER_JSON, chemin routeur : le routeur
// agent fetch /portal/{token}/login.html au déploiement, les messages sont
// figés dedans) ET dans la config JSON (portalTicker, consommée par le bloc
// 10 de applyConfig au fetch live).
func TestPortalServeTicker(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	seedRouterWithAccount(t, st, "tok-ticker", struct {
		tenantName  string
		wifiSlug    string
		joinActive  bool
		profileName string
		profilePrc  int
		waveLink    string
		bannerUrl   string
	}{tenantName: "Cyber Ticker", wifiSlug: "", joinActive: false, profileName: "", profilePrc: 0, waveLink: ""})

	// Poser les messages du tenant directement dans le store (la voie PUT est
	// couverte par TestSettingsPortalTickerValidation). Valeur de map non
	// adressable → read-modify-write de la Settings complète.
	tickerJSON := `["Fibre optique 100 Mbps","Ouvert 7j/7 de 8h à 22h"]`
	st.Lock()
	{
		db := st.Data()
		s := db.SettingsByAccount["acc-portal-tok-ticker"]
		s.Tenant.PortalTicker = tickerJSON
		db.SettingsByAccount["acc-portal-tok-ticker"] = s
	}
	st.Save()
	st.Unlock()

	resp, err := http.Get(ts.URL + "/portal/tok-ticker/login.html")
	if err != nil {
		t.Fatalf("GET login.html : %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("statut %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	// 1. Le marqueur de l'init Typed.js porte les messages du tenant
	// (tableau JSON inline, non échappé HTML — il vit dans un contexte JS).
	if !strings.Contains(string(body), `strings: ["Fibre optique 100 Mbps","Ouvert 7j/7 de 8h à 22h"],`) {
		t.Error("l'init Typed.js ne porte pas les messages du tenant (marqueur TICKER_JSON non substitué ou incorrect)")
	}
	if strings.Contains(string(body), "{{MIKCLOUD_TICKER_JSON}}") {
		t.Error("marqueur TICKER_JSON non substitué dans le login.html servi")
	}
	// 2. La config JSON embarque AUSSI portalTicker (chemin fetch live).
	start := strings.Index(string(body), `id="mikcloud-config">`) + len(`id="mikcloud-config">`)
	end := strings.Index(string(body)[start:], "</script>")
	if start <= 0 || end <= 0 {
		t.Fatal("bloc config introuvable")
	}
	var cfg hotpage.PortalConfig
	if err := json.Unmarshal([]byte(string(body)[start:start+end]), &cfg); err != nil {
		t.Fatalf("JSON config invalide : %v", err)
	}
	want := []string{"Fibre optique 100 Mbps", "Ouvert 7j/7 de 8h à 22h"}
	if !reflect.DeepEqual(cfg.Ticker, want) {
		t.Errorf("Ticker = %v, attendu %v", cfg.Ticker, want)
	}
}

// TestPortalServeTickerDefaults — sans messages configurés, le login.html
// servi garde les 3 messages HISTORIQUES du template (repli neutre — messages
// WiFi génériques) et la config JSON omet portalTicker (omitempty).
func TestPortalServeTickerDefaults(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	seedRouterWithAccount(t, st, "tok-ticker-def", struct {
		tenantName  string
		wifiSlug    string
		joinActive  bool
		profileName string
		profilePrc  int
		waveLink    string
		bannerUrl   string
	}{tenantName: "Cyber Defauts", wifiSlug: "", joinActive: false, profileName: "", profilePrc: 0, waveLink: ""})

	resp, err := http.Get(ts.URL + "/portal/tok-ticker-def/login.html")
	if err != nil {
		t.Fatalf("GET login.html : %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("statut %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `strings: ["Wifi haut débit !","Disponible 24H/24","Payez facilement par Wave !"],`) {
		t.Error("sans messages configurés, l'init Typed.js doit retomber sur les 3 messages historiques du template")
	}
	if strings.Contains(string(body), `"portalTicker":`) {
		t.Error("sans messages configurés, la config JSON ne doit pas porter portalTicker (omitempty)")
	}
}

// TestWifiPortalTicker — le fetch live du portail déployé (N°48 :
// GET /api/wifi/site/{slug}/portal) porte AUSSI les messages : un portail
// déjà déployé qui recharge sa config en direct affiche les nouveaux
// messages sans redéploiement (le bloc 10 de applyConfig compare sa
// signature d'état puis reset() l'instance Typed.js).
func TestWifiPortalTicker(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	seedWifiSitePortal(t, st, "cyber-ticker", "Cyber Ticker Site", "", false, 0)

	tickerJSON := `["Fibre optique chez vous !","Rechargez et surfez"]`
	st.Lock()
	{
		db := st.Data()
		s := db.SettingsByAccount["acc-wifi-cyber-ticker"]
		s.Tenant.PortalTicker = tickerJSON
		db.SettingsByAccount["acc-wifi-cyber-ticker"] = s
	}
	st.Save()
	st.Unlock()

	resp, err := http.Get(ts.URL + "/api/wifi/site/cyber-ticker/portal")
	if err != nil {
		t.Fatalf("GET : %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("statut %d, attendu 200", resp.StatusCode)
	}
	var cfg hotpage.PortalConfig
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		t.Fatalf("JSON invalide : %v", err)
	}
	want := []string{"Fibre optique chez vous !", "Rechargez et surfez"}
	if !reflect.DeepEqual(cfg.Ticker, want) {
		t.Errorf("Ticker = %v, attendu %v", cfg.Ticker, want)
	}
}
