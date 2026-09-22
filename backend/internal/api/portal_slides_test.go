// Package api — tests des slides du carrousel commercial du portail captif
// (N°136) : le gérant remplace les 3 visuels génériques pub1/2/3 par SES
// images (≤ 3 URLs https via R2 /api/media).
//
// Couvre :
//   - portalSlidesList : décodage défensif du JSON tenant (invalides/vides
//     filtrés, plafond 3 re-vérifié — défense en profondeur contre une ligne
//     héritée d'un appel API direct) ;
//   - PUT /api/settings : matrice des validations portalSlides (≤ 3 URLs
//     https ≤ 300 car., entrées vides ignorées, liste vide = retour aux
//     images par défaut) ;
//   - servage /portal/{token}/login.html : la config JSON embarque
//     portalSlides (chemin routeur, config figée au déploiement) ;
//   - GET /api/wifi/site/{slug}/portal : le fetch live (N°48) du portail
//     déployé porte AUSSI les slides (chemin hybride, appliqué par
//     applySlides côté template).
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
)

// TestPortalSlidesList — matrice du décodeur défensif : un JSON invalide,
// vide ou une liste sans aucune URL https donne nil (le portail garde ses 3
// images génériques, jamais cassée) ; le plafond 3 est re-vérifié ici même
// si l'API l'a déjà validé (défense en profondeur).
func TestPortalSlidesList(t *testing.T) {
	cases := []struct {
		name string
		json string
		want []string
	}{
		{"vide", "", nil},
		{"JSON invalide", `{"a":1}`, nil},
		{"liste vide", `[]`, nil},
		{"objet au lieu de liste", `{"url":"https://a"}`, nil},
		{"une URL", `["https://r2.example.com/s1.jpg"]`, []string{"https://r2.example.com/s1.jpg"}},
		{"deux URLs", `["https://r2.example.com/s1.jpg","https://r2.example.com/s2.jpg"]`, []string{"https://r2.example.com/s1.jpg", "https://r2.example.com/s2.jpg"}},
		{"http filtré", `["http://insecure.example/s.jpg","https://r2.example.com/s.jpg"]`, []string{"https://r2.example.com/s.jpg"}},
		{"javascript filtré", `["javascript:alert(1)"]`, nil},
		{"vide filtré", `["","  ","https://r2.example.com/s.jpg"]`, []string{"https://r2.example.com/s.jpg"}},
		{"espaces trimés", `[" https://r2.example.com/s.jpg "]`, []string{"https://r2.example.com/s.jpg"}},
		{"plafond 3 re-vérifié", `["https://a.example/1.jpg","https://b.example/2.jpg","https://c.example/3.jpg","https://d.example/4.jpg"]`, []string{"https://a.example/1.jpg", "https://b.example/2.jpg", "https://c.example/3.jpg"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := portalSlidesList(tc.json)
			if tc.want == nil {
				if got != nil {
					t.Fatalf("portalSlidesList(%q) = %v, attendu nil", tc.json, got)
				}
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("portalSlidesList(%q) = %v, attendu %v", tc.json, got, tc.want)
			}
		})
	}
}

// putSlides — PUT /api/settings avec portalSlides (formes plates + nested,
// même corps défensif que le front) et retourne le statut + les settings.
func putSlides(t *testing.T, ts *httptest.Server, token string, slides []string) (int, map[string]any) {
	t.Helper()
	body := map[string]any{
		"portalSlides": slides,
		"tenant":       map[string]any{"portalSlides": slides},
	}
	return doJSON(t, ts, "PUT", "/api/settings", token, body)
}

// slidesOf — extrait tenant.portalSlides de la réponse PUT : le champ voyage
// comme CHAÎNE JSON (pattern N°55 des listes du tenant — portalPromos,
// portalSocials — le front fait JSON.parse de la même façon). Retourne nil
// si absent/vide.
func slidesOf(t *testing.T, out map[string]any) []string {
	t.Helper()
	tenant, ok := out["tenant"].(map[string]any)
	if !ok {
		return nil
	}
	raw, ok := tenant["portalSlides"].(string)
	if !ok || raw == "" {
		return nil
	}
	var list []string
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		t.Fatalf("portalSlides n'est pas un JSON d'URLs valide : %q (%v)", raw, err)
	}
	return list
}

// TestSettingsPortalSlidesValidation — matrice des validations portalSlides
// sur PUT /api/settings (même discipline que la bannière N°45).
func TestSettingsPortalSlidesValidation(t *testing.T) {
	ts := newTestServer(t)
	token, _, _ := registerAccount(t, ts, "gerant-slides", "")

	// Défaut : le champ n'a jamais été touché → absent du JSON (omitempty).
	status, out := doJSON(t, ts, "GET", "/api/settings", token, nil)
	if status != http.StatusOK {
		t.Fatalf("GET settings : %d", status)
	}
	if s := slidesOf(t, out); s != nil {
		t.Errorf("portalSlides doit être absent par défaut, obtenu %v", s)
	}

	// 3 URLs https valides → 200, persistées dans l'ordre.
	want := []string{"https://pub-abc.r2.dev/slides/s1.jpg", "https://pub-abc.r2.dev/slides/s2.jpg", "https://pub-abc.r2.dev/slides/s3.jpg"}
	if status, out = putSlides(t, ts, token, want); status != http.StatusOK {
		t.Fatalf("3 URLs valides doivent passer (200), obtenu %d : %v", status, out)
	}
	if got := slidesOf(t, out); !reflect.DeepEqual(got, want) {
		t.Errorf("portalSlides non persistées : %v, attendu %v", got, want)
	}

	// GET reflète l'état persisté.
	if status, out = doJSON(t, ts, "GET", "/api/settings", token, nil); status != http.StatusOK {
		t.Fatalf("GET settings : %d", status)
	}
	if got := slidesOf(t, out); !reflect.DeepEqual(got, want) {
		t.Errorf("GET ne reflète pas portalSlides : %v, attendu %v", got, want)
	}

	// 4 URLs → 400 (le carrousel du portail a exactement 3 slots).
	if status, _ = putSlides(t, ts, token, []string{"https://a.example/1.jpg", "https://a.example/2.jpg", "https://a.example/3.jpg", "https://a.example/4.jpg"}); status != http.StatusBadRequest {
		t.Errorf("4 slides doivent être refusées (400), obtenu %d", status)
	}

	// Schémas interdits → 400 (le portail exige https, pas de http clair ni
	// de javascript:).
	for _, bad := range []string{"http://insecure.example/s.jpg", "javascript:alert(1)", "/img/relative.jpg"} {
		if status, _ = putSlides(t, ts, token, []string{bad}); status != http.StatusBadRequest {
			t.Errorf("slide %q doit être refusée (400), obtenu %d", bad, status)
		}
	}

	// URL > 300 car. → 400 (même règle que les images de promos N°55).
	long := "https://r2.example.com/" + strings.Repeat("a", 300) + ".jpg"
	if status, _ = putSlides(t, ts, token, []string{long}); status != http.StatusBadRequest {
		t.Errorf("URL > 300 car. doit être refusée (400), obtenu %d", status)
	}

	// Entrées vides ignorées (la console n'en envoie pas, l'API tolère).
	if status, out = putSlides(t, ts, token, []string{"", "https://pub-abc.r2.dev/slides/keep.jpg", "  "}); status != http.StatusOK {
		t.Fatalf("entrées vides ignorées doivent passer (200), obtenu %d : %v", status, out)
	}
	if got := slidesOf(t, out); !reflect.DeepEqual(got, []string{"https://pub-abc.r2.dev/slides/keep.jpg"}) {
		t.Errorf("entrées vides non ignorées : %v", got)
	}

	// Liste vide → 200, slides retirées (retour aux images génériques du
	// template ; omitempty : la clé disparaît du JSON).
	if status, out = putSlides(t, ts, token, []string{}); status != http.StatusOK {
		t.Fatalf("liste vide doit passer (200), obtenu %d : %v", status, out)
	}
	if got := slidesOf(t, out); got != nil {
		t.Errorf("portalSlides doit être retirée (absente), obtenu %v", got)
	}
}

// TestPortalServeSlides — les slides du tenant sont propagées dans la config
// JSON du login.html servi (chemin routeur : le routeur agent fetch
// /portal/{token}/login.html au déploiement, la config est figée dedans).
func TestPortalServeSlides(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	seedRouterWithAccount(t, st, "tok-slides", struct {
		tenantName  string
		wifiSlug    string
		joinActive  bool
		profileName string
		profilePrc  int
		waveLink    string
		bannerUrl   string
	}{tenantName: "Cyber Slides", wifiSlug: "", joinActive: false, profileName: "", profilePrc: 0, waveLink: ""})

	// Poser les slides du tenant directement dans le store (la voie PUT est
	// couverte par TestSettingsPortalSlidesValidation). Valeur de map non
	// adressable → read-modify-write de la Settings complète.
	slidesJSON := `["https://pub-abc.r2.dev/slides/pub-ete.jpg","https://pub-abc.r2.dev/slides/pub-wifi.jpg"]`
	st.Lock()
	{
		db := st.Data()
		s := db.SettingsByAccount["acc-portal-tok-slides"]
		s.Tenant.PortalSlides = slidesJSON
		db.SettingsByAccount["acc-portal-tok-slides"] = s
	}
	st.Save()
	st.Unlock()

	resp, err := http.Get(ts.URL + "/portal/tok-slides/login.html")
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
	want := []string{"https://pub-abc.r2.dev/slides/pub-ete.jpg", "https://pub-abc.r2.dev/slides/pub-wifi.jpg"}
	if !reflect.DeepEqual(cfg.Slides, want) {
		t.Errorf("Slides = %v, attendu %v", cfg.Slides, want)
	}
	// Le JSON brut porte bien la clé portalSlides (appliquée par applySlides
	// côté template, après échappement HTML).
	if !strings.Contains(string(body), `"portalSlides":`) {
		t.Error("clé portalSlides absente du JSON servi")
	}
}

// TestWifiPortalSlides — le fetch live du portail déployé (N°48 :
// GET /api/wifi/site/{slug}/portal) porte AUSSI les slides : un portail déjà
// déployé qui recharge sa config en direct affiche les nouvelles images sans
// redéploiement (le template applySlides compare sa signature d'état).
func TestWifiPortalSlides(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	seedWifiSitePortal(t, st, "cyber-slides", "Cyber Slides Site", "", false, 0)

	slidesJSON := `["https://pub-abc.r2.dev/slides/site-1.jpg","https://pub-abc.r2.dev/slides/site-2.jpg","https://pub-abc.r2.dev/slides/site-3.jpg"]`
	st.Lock()
	{
		db := st.Data()
		s := db.SettingsByAccount["acc-wifi-cyber-slides"]
		s.Tenant.PortalSlides = slidesJSON
		db.SettingsByAccount["acc-wifi-cyber-slides"] = s
	}
	st.Save()
	st.Unlock()

	resp, err := http.Get(ts.URL + "/api/wifi/site/cyber-slides/portal")
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
	want := []string{"https://pub-abc.r2.dev/slides/site-1.jpg", "https://pub-abc.r2.dev/slides/site-2.jpg", "https://pub-abc.r2.dev/slides/site-3.jpg"}
	if !reflect.DeepEqual(cfg.Slides, want) {
		t.Errorf("Slides = %v, attendu %v", cfg.Slides, want)
	}
}
