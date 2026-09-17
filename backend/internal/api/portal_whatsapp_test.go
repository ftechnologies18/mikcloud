// Package api — tests du numéro WhatsApp SUPPORT du portail captif (N°139) :
// le gérant remplace le numéro du support MikCloud affiché dans le footer du
// portail (login, logout, error) par SON numéro (format international,
// chiffres seuls 8-15 + libellé d'affichage optionnel ≤ 30 car.).
//
// Couvre :
//   - portalWhatsappInfo : décodage défensif du JSON tenant (invalide/vide/
//     mal formé → nil, format revalidé via hotpage.WhatsappNumber — défense
//     en profondeur contre une ligne héritée d'un appel API direct) ;
//   - PUT /api/settings : matrice des validations portalWhatsapp (8-15
//     chiffres après nettoyage, label ≤ 30 car., number vide = retour au
//     support MikCloud) ;
//   - servage /portal/{token}/{login,logout,error}.html : les marqueurs
//     WHATSAPP_HREF/LABEL embarquent le numéro DU TENANT (chemin routeur,
//     config figée au déploiement) ;
//   - GET /api/wifi/site/{slug}/portal : le fetch live (N°48) porte AUSSI le
//     numéro (chemin hybride, appliqué par le bloc 11 de applyConfig).
package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mikcloud/hotspot-api/internal/hotpage"
	"mikcloud/hotspot-api/internal/model"
)

// TestPortalWhatsappInfo — matrice du décodeur défensif : un JSON invalide,
// vide, un objet sans number exploitable ou un numéro hors [8, 15] chiffres
// donnent nil (le portail garde le numéro du support MikCloud, jamais cassé) ;
// le nettoyage (espaces, +, -) et le trim du label sont appliqués ici même
// si l'API les a déjà faits (défense en profondeur).
func TestPortalWhatsappInfo(t *testing.T) {
	cases := []struct {
		name string
		json string
		want *hotpage.PortalWhatsapp
	}{
		{"vide", "", nil},
		{"JSON invalide", `{"number":`, nil},
		{"liste au lieu d'objet", `["2250708091012"]`, nil},
		{"number vide", `{"number":""}`, nil},
		{"number absent", `{"label":"Support"}`, nil},
		{"7 chiffres", `{"number":"1234567"}`, nil},
		{"16 chiffres", `{"number":"1234567890123456"}`, nil},
		{"numéro simple", `{"number":"2250708091012"}`, &hotpage.PortalWhatsapp{Number: "2250708091012"}},
		{"+ et espaces nettoyés", `{"number":"+225 07 08 09 10 12"}`, &hotpage.PortalWhatsapp{Number: "2250708091012"}},
		{"tirets et parenthèses nettoyés", `{"number":"(225)-0708.091012"}`, &hotpage.PortalWhatsapp{Number: "2250708091012"}},
		{"label trimé", `{"number":"2250708091012","label":"  07 08 09 10 12  "}`, &hotpage.PortalWhatsapp{Number: "2250708091012", Label: "07 08 09 10 12"}},
		{"number lettres filtrées puis trop court", `{"number":"22507abc"}`, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := portalWhatsappInfo(model.Tenant{PortalWhatsapp: tc.json})
			if tc.want == nil {
				if got != nil {
					t.Fatalf("portalWhatsappInfo(%q) = %v, attendu nil", tc.json, got)
				}
				return
			}
			if got == nil {
				t.Fatalf("portalWhatsappInfo(%q) = nil, attendu %v", tc.json, tc.want)
			}
			if got.Number != tc.want.Number || got.Label != tc.want.Label {
				t.Fatalf("portalWhatsappInfo(%q) = %+v, attendu %+v", tc.json, got, tc.want)
			}
		})
	}
}

// putWhatsapp — PUT /api/settings avec portalWhatsapp (formes plates +
// nested, même corps défensif que le front) et retourne le statut + les
// settings.
func putWhatsapp(t *testing.T, ts *httptest.Server, token string, wa map[string]any) (int, map[string]any) {
	t.Helper()
	body := map[string]any{
		"portalWhatsapp": wa,
		"tenant":         map[string]any{"portalWhatsapp": wa},
	}
	return doJSON(t, ts, "PUT", "/api/settings", token, body)
}

// whatsappOf — extrait tenant.portalWhatsapp de la réponse PUT : le champ
// voyage comme CHAÎNE JSON (pattern N°55). Retourne nil si absent/vide.
func whatsappOf(t *testing.T, out map[string]any) *hotpage.PortalWhatsapp {
	t.Helper()
	tenant, ok := out["tenant"].(map[string]any)
	if !ok {
		return nil
	}
	raw, ok := tenant["portalWhatsapp"].(string)
	if !ok || raw == "" {
		return nil
	}
	var wa hotpage.PortalWhatsapp
	if err := json.Unmarshal([]byte(raw), &wa); err != nil {
		t.Fatalf("portalWhatsapp n'est pas un JSON valide : %q (%v)", raw, err)
	}
	return &wa
}

// TestSettingsPortalWhatsappValidation — matrice des validations
// portalWhatsapp sur PUT /api/settings (même discipline que le ticker N°138).
func TestSettingsPortalWhatsappValidation(t *testing.T) {
	ts := newTestServer(t)
	token, _, _ := registerAccount(t, ts, "gerant-wa", "")

	// Défaut : le champ n'a jamais été touché → absent du JSON (omitempty).
	status, out := doJSON(t, ts, "GET", "/api/settings", token, nil)
	if status != http.StatusOK {
		t.Fatalf("GET settings : %d", status)
	}
	if wa := whatsappOf(t, out); wa != nil {
		t.Errorf("portalWhatsapp doit être absent par défaut, obtenu %+v", wa)
	}

	// Numéro + libellé valides → 200, persistés (number nettoyé, label trimé).
	if status, out = putWhatsapp(t, ts, token, map[string]any{"number": "+225 07 08 09 10 12", "label": " 07 08 09 10 12 "}); status != http.StatusOK {
		t.Fatalf("numéro valide doit passer (200), obtenu %d : %v", status, out)
	}
	if wa := whatsappOf(t, out); wa == nil || wa.Number != "2250708091012" || wa.Label != "07 08 09 10 12" {
		t.Errorf("portalWhatsapp non persisté/nettoyé : %+v", wa)
	}

	// GET reflète l'état persisté.
	if status, out = doJSON(t, ts, "GET", "/api/settings", token, nil); status != http.StatusOK {
		t.Fatalf("GET settings : %d", status)
	}
	if wa := whatsappOf(t, out); wa == nil || wa.Number != "2250708091012" {
		t.Errorf("GET ne reflète pas portalWhatsapp : %+v", wa)
	}

	// 7 chiffres → 400 (wa.me exige un numéro international complet).
	if status, _ = putWhatsapp(t, ts, token, map[string]any{"number": "1234567", "label": ""}); status != http.StatusBadRequest {
		t.Errorf("numéro de 7 chiffres doit être refusé (400), obtenu %d", status)
	}

	// 16 chiffres → 400 (E.164 : 15 chiffres max).
	if status, _ = putWhatsapp(t, ts, token, map[string]any{"number": "1234567890123456", "label": ""}); status != http.StatusBadRequest {
		t.Errorf("numéro de 16 chiffres doit être refusé (400), obtenu %d", status)
	}

	// Lettres dans le numéro → 400 (nettoyé en numéro trop court).
	if status, _ = putWhatsapp(t, ts, token, map[string]any{"number": "22507ABCDEF", "label": ""}); status != http.StatusBadRequest {
		t.Errorf("numéro avec lettres doit être refusé (400), obtenu %d", status)
	}

	// Libellé > 30 caractères → 400 (le footer tient une ligne lisible).
	if status, _ = putWhatsapp(t, ts, token, map[string]any{"number": "2250708091012", "label": strings.Repeat("a", 31)}); status != http.StatusBadRequest {
		t.Errorf("libellé > 30 car. doit être refusé (400), obtenu %d", status)
	}

	// Number vide → 200, numéro retiré (retour au support MikCloud ;
	// omitempty : la clé disparaît du JSON).
	if status, out = putWhatsapp(t, ts, token, map[string]any{"number": "", "label": ""}); status != http.StatusOK {
		t.Fatalf("number vide doit passer (200), obtenu %d : %v", status, out)
	}
	if wa := whatsappOf(t, out); wa != nil {
		t.Errorf("portalWhatsapp doit être retiré (absent), obtenu %+v", wa)
	}

	// Forme imbriquée seule (sans champ plat) → appliquée (repli du corps
	// défensif : le plat prime, le nested sert de repli).
	body := map[string]any{
		"tenant": map[string]any{"portalWhatsapp": map[string]any{"number": "2250102030405", "label": "Le gérant"}},
	}
	if status, out = doJSON(t, ts, "PUT", "/api/settings", token, body); status != http.StatusOK {
		t.Fatalf("forme nested doit passer (200), obtenu %d : %v", status, out)
	}
	if wa := whatsappOf(t, out); wa == nil || wa.Number != "2250102030405" || wa.Label != "Le gérant" {
		t.Errorf("forme nested non appliquée : %+v", wa)
	}
}

// TestPortalServeWhatsapp — le numéro DU TENANT est servi dans les TROIS
// pages qui portent le lien support (marqueurs WHATSAPP_HREF/LABEL, chemin
// routeur : le routeur agent fetch /portal/{token}/{page}.html au
// déploiement, le numéro est figé dedans) ET dans la config JSON
// (portalWhatsapp, consommée par le bloc 11 de applyConfig au fetch live).
func TestPortalServeWhatsapp(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	seedRouterWithAccount(t, st, "tok-wa", struct {
		tenantName  string
		wifiSlug    string
		joinActive  bool
		profileName string
		profilePrc  int
		waveLink    string
		bannerUrl   string
	}{tenantName: "Cyber WhatsApp", wifiSlug: "", joinActive: false, profileName: "", profilePrc: 0, waveLink: ""})

	// Poser le numéro du tenant directement dans le store (la voie PUT est
	// couverte par TestSettingsPortalWhatsappValidation). Valeur de map non
	// adressable → read-modify-write de la Settings complète.
	st.Lock()
	{
		db := st.Data()
		s := db.SettingsByAccount["acc-portal-tok-wa"]
		s.Tenant.PortalWhatsapp = `{"number":"2250708091012","label":"07 08 09 10 12"}`
		db.SettingsByAccount["acc-portal-tok-wa"] = s
	}
	st.Save()
	st.Unlock()

	for _, page := range []string{"login.html", "logout.html", "error.html"} {
		resp, err := http.Get(ts.URL + "/portal/tok-wa/" + page)
		if err != nil {
			t.Fatalf("GET %s : %v", page, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s : statut %d", page, resp.StatusCode)
		}
		if !strings.Contains(string(body), `href="https://wa.me/2250708091012"`) {
			t.Errorf("%s : le lien support ne porte pas le numéro du tenant (marqueur WHATSAPP_HREF non substitué)", page)
		}
		if !strings.Contains(string(body), `07 08 09 10 12`) {
			t.Errorf("%s : le libellé du tenant absent (marqueur WHATSAPP_LABEL non substitué)", page)
		}
		if strings.Contains(string(body), "{{MIKCLOUD_WHATSAPP_HREF}}") || strings.Contains(string(body), "{{MIKCLOUD_WHATSAPP_LABEL}}") {
			t.Errorf("%s : marqueur WHATSAPP non substitué dans la page servie", page)
		}
		if strings.Contains(string(body), "https://wa.me/2250150491807") || strings.Contains(string(body), "01 5049 1807") {
			t.Errorf("%s : le lien du support MikCloud ne doit plus être servi quand le tenant a le sien", page)
		}
	}

	// La config JSON de login.html embarque AUSSI portalWhatsapp (chemin
	// fetch live, bloc 11 de applyConfig).
	resp, err := http.Get(ts.URL + "/portal/tok-wa/login.html")
	if err != nil {
		t.Fatalf("GET login.html : %v", err)
	}
	defer resp.Body.Close()
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
	if cfg.Whatsapp == nil || cfg.Whatsapp.Number != "2250708091012" || cfg.Whatsapp.Label != "07 08 09 10 12" {
		t.Errorf("Whatsapp = %+v, attendu {2250708091012 07 08 09 10 12}", cfg.Whatsapp)
	}
}

// TestPortalServeWhatsappDefaults — sans numéro configuré, les pages servies
// gardent le numéro du support MikCloud (repli historique — le support
// plateforme, comme le crédit FTCI du footer) et la config JSON omet
// portalWhatsapp (omitempty).
func TestPortalServeWhatsappDefaults(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	seedRouterWithAccount(t, st, "tok-wa-def", struct {
		tenantName  string
		wifiSlug    string
		joinActive  bool
		profileName string
		profilePrc  int
		waveLink    string
		bannerUrl   string
	}{tenantName: "Cyber Defauts WA", wifiSlug: "", joinActive: false, profileName: "", profilePrc: 0, waveLink: ""})

	for _, page := range []string{"login.html", "logout.html", "error.html"} {
		resp, err := http.Get(ts.URL + "/portal/tok-wa-def/" + page)
		if err != nil {
			t.Fatalf("GET %s : %v", page, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s : statut %d", page, resp.StatusCode)
		}
		if !strings.Contains(string(body), `href="https://wa.me/2250150491807"`) {
			t.Errorf("%s : sans numéro configuré, le repli support MikCloud doit être servi", page)
		}
		if !strings.Contains(string(body), "01 5049 1807") {
			t.Errorf("%s : le libellé du repli (01 5049 1807) doit être servi", page)
		}
	}
	resp, err := http.Get(ts.URL + "/portal/tok-wa-def/login.html")
	if err != nil {
		t.Fatalf("GET login.html : %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if strings.Contains(string(body), `"portalWhatsapp":`) {
		t.Error("sans numéro configuré, la config JSON ne doit pas porter portalWhatsapp (omitempty)")
	}
}

// TestWifiPortalWhatsapp — le fetch live du portail déployé (N°48 :
// GET /api/wifi/site/{slug}/portal) porte AUSSI le numéro : un portail déjà
// déployé qui recharge sa config en direct affiche le numéro du gérant sans
// redéploiement (le bloc 11 de applyConfig remplace href et libellé).
func TestWifiPortalWhatsapp(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	seedWifiSitePortal(t, st, "cyber-wa", "Cyber WA Site", "", false, 0)

	st.Lock()
	{
		db := st.Data()
		s := db.SettingsByAccount["acc-wifi-cyber-wa"]
		s.Tenant.PortalWhatsapp = `{"number":"2250102030405"}`
		db.SettingsByAccount["acc-wifi-cyber-wa"] = s
	}
	st.Save()
	st.Unlock()

	resp, err := http.Get(ts.URL + "/api/wifi/site/cyber-wa/portal")
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
	if cfg.Whatsapp == nil || cfg.Whatsapp.Number != "2250102030405" || cfg.Whatsapp.Label != "" {
		t.Errorf("Whatsapp = %+v, attendu {2250102030405 }", cfg.Whatsapp)
	}
}
