// Package hotpage — tests du templating par compte (N°35-b).
package hotpage

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// TestPersonalizeMarkers — chaque marqueur est substitué par sa valeur.
func TestPersonalizeMarkers(t *testing.T) {
	cfg := PortalConfig{
		TenantName: "Cyber Espace SC",
		APIBase:    "https://mikcloud.onrender.com",
		WifiSlug:   "cyber-espace-sc",
		WifiURL:    "https://mikcloud.ftci.fr/wifi/cyber-espace-sc",
		JoinURL:    "https://mikcloud.ftci.fr/join/abcdef1234567890",
		WaveLink:   "https://pay.wave.com/m/M_xxx/c/ci/",
		LogoURL:    "data:image/png;base64,xyz",
		BannerURL:  "https://r2.example.com/banners/cyber.jpg",
	}
	content := `<title>{{MIKCLOUD_TENANT_NAME}}</title>
<a href="{{MIKCLOUD_WIFI_URL}}">WiFi</a>
<form action="{{MIKCLOUD_JOIN_URL}}">Join</form>
<a href="{{MIKCLOUD_WAVE_LINK}}/amount/100/">Wave</a>
<img src="{{MIKCLOUD_LOGO_URL}}"/>
<img src="{{MIKCLOUD_BANNER_URL}}"/>
<base data-api="{{MIKCLOUD_API_BASE}}"/>`
	out := Personalize(content, cfg)
	for _, want := range []string{
		`<title>Cyber Espace SC</title>`,
		`href="https://mikcloud.ftci.fr/wifi/cyber-espace-sc"`,
		`action="https://mikcloud.ftci.fr/join/abcdef1234567890"`,
		`href="https://pay.wave.com/m/M_xxx/c/ci//amount/100/"`,
		`src="data:image/png;base64,xyz"`,
		`src="https://r2.example.com/banners/cyber.jpg"`,
		`data-api="https://mikcloud.onrender.com"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("marqueur absent : %q", want)
		}
	}
	// Aucun marqueur {{MIKCLOUD_*}} ne doit rester (hors CONFIG_JSON testé ailleurs).
	if strings.Contains(out, "{{MIKCLOUD_") {
		t.Errorf("marqueur non substitué restant :\n%s", out)
	}
}

// TestPersonalizeLogoBlock — N°135 : le bloc logo porte le logo DU CLIENT
// quand il est configuré, sinon l'initiale du tenant — jamais un asset par
// défaut (le logo historique du template était celui d'un autre client).
func TestPersonalizeLogoBlock(t *testing.T) {
	// Avec logo : <img> du client + repli initiale masqué.
	cfg := PortalConfig{TenantName: "ProMax WIFI", LogoURL: "data:image/png;base64,abc"}
	out := Personalize(`<div class="logo-wrap">{{MIKCLOUD_LOGO_BLOCK}}</div>`, cfg)
	if !strings.Contains(out, `<img src="data:image/png;base64,abc"`) {
		t.Errorf("img du logo client absent : %s", out)
	}
	if !strings.Contains(out, `alt="Logo ProMax WIFI"`) {
		t.Errorf("alt du logo absent : %s", out)
	}
	if !strings.Contains(out, `<span class="logo-fallback" style="display:none;">P</span>`) {
		t.Errorf("repli initiale masqué absent : %s", out)
	}
	// Sans logo : initiale visible (majuscule, même en minuscules au nom),
	// AUCUN <img> — c'est l'initiale DU tenant qui signe le portail.
	out = Personalize(`<div class="logo-wrap">{{MIKCLOUD_LOGO_BLOCK}}</div>`, PortalConfig{TenantName: "promax wifi"})
	if !strings.Contains(out, `<div class="logo-wrap"><span class="logo-fallback">P</span></div>`) {
		t.Errorf("initiale du tenant absente du bloc : %s", out)
	}
	if strings.Contains(out, "<img") {
		t.Errorf("aucun <img> ne doit être rendu sans logo configuré : %s", out)
	}
	// Nom vide : repli neutre « W » (WiFi).
	out = Personalize(`{{MIKCLOUD_LOGO_BLOCK}}`, PortalConfig{})
	if !strings.Contains(out, `<span class="logo-fallback">W</span>`) {
		t.Errorf("repli W attendu pour un tenant sans nom : %s", out)
	}
	// Nom accentué : l'initiale reste une lettre unicode majuscule.
	out = Personalize(`{{MIKCLOUD_LOGO_BLOCK}}`, PortalConfig{TenantName: "éclair Net"})
	if !strings.Contains(out, `<span class="logo-fallback">É</span>`) {
		t.Errorf("initiale accentuée attendue : %s", out)
	}
}

// TestPersonalizeLogoBlockInjection — un logo ou un nom malveillant ne peut
// pas sortir du contexte attribut du bloc logo (échappement HTML strict).
func TestPersonalizeLogoBlockInjection(t *testing.T) {
	cfg := PortalConfig{
		TenantName: `X" onmouseover="alert(1)`,
		LogoURL:    `data:image/png;base64,x" onerror="alert(2)`,
	}
	out := Personalize(`{{MIKCLOUD_LOGO_BLOCK}}`, cfg)
	if strings.Contains(out, `" onerror="alert(2)`) {
		t.Errorf("injection via logoUrl non neutralisée : %s", out)
	}
	if strings.Contains(out, `" onmouseover="alert(1)`) {
		t.Errorf("injection via tenantName non neutralisée : %s", out)
	}
	if !strings.Contains(out, `&#34;`) {
		t.Errorf("guillemets non échappés en &#34; : %s", out)
	}
}

// TestPersonalizeServicesBlock — N°137 : la section « Nos Services » rend les
// services DU TENANT (≤ 6) et se MASQUE entièrement sans services
// configurés (repli neutre — les 4 services historiques du template
// étaient ceux du site pilote, même chasse que le logo N°135).
func TestPersonalizeServicesBlock(t *testing.T) {
	// Avec services : les <li> du tenant, icône + libellé.
	cfg := PortalConfig{Services: []PortalService{
		{Icon: "fa-print", Label: "Impression & photocopie"},
		{Label: "Recharge électrique"}, // icône vide → coche neutre fa-check
	}}
	tpl := `<div class="mt-4" id="mikcloud-services-wrap"{{MIKCLOUD_SERVICES_ATTR}}><ul id="mikcloud-services-list">{{MIKCLOUD_SERVICES_BLOCK}}</ul></div>`
	out := Personalize(tpl, cfg)
	if !strings.Contains(out, `<li class="service-list-item"><div class="service-icon-box"><i class="fas fa-print"></i></div><span>Impression &amp; photocopie</span></li>`) {
		t.Errorf("li du service (icône fa-print) absent : %s", out)
	}
	if !strings.Contains(out, `<i class="fas fa-check"></i>`) {
		t.Errorf("icône vide doit retomber sur fa-check : %s", out)
	}
	if strings.Contains(out, "display:none") {
		t.Errorf("avec des services configurés, la section ne doit PAS être masquée : %s", out)
	}
	// Sans services : bloc vide + ATTR masquant (section retirée, jamais vide).
	out = Personalize(tpl, PortalConfig{})
	if !strings.Contains(out, `id="mikcloud-services-wrap" style="display:none"`) {
		t.Errorf("sans services, le wrap doit être masqué par l'ATTR : %s", out)
	}
	if !strings.Contains(out, `id="mikcloud-services-list"></ul>`) {
		t.Errorf("sans services, le bloc doit être vide : %s", out)
	}
}

// TestPersonalizeServicesBlockInjection — un libellé ou une icône malveillants
// ne peuvent pas sortir de leurs contextes (contenu HTML / attribut class).
func TestPersonalizeServicesBlockInjection(t *testing.T) {
	cfg := PortalConfig{Services: []PortalService{
		{Icon: `fa-x" onload="alert(1)`, Label: `<script>alert(2)</script>`},
	}}
	out := Personalize(`<ul>{{MIKCLOUD_SERVICES_BLOCK}}</ul>`, cfg)
	if strings.Contains(out, "<script>") {
		t.Errorf("label non échappé : %s", out)
	}
	if strings.Contains(out, `" onload="`) {
		t.Errorf("icône non échappée dans l'attribut class : %s", out)
	}
	if !strings.Contains(out, "&lt;script&gt;") {
		t.Errorf("label doit être échappé en &lt;script&gt; : %s", out)
	}
}

// TestPersonalizeConfigJSON — le marqueur CONFIG_JSON est remplacé par un
// objet JSON valide, parsable par encoding/json côté client.
func TestPersonalizeConfigJSON(t *testing.T) {
	cfg := PortalConfig{
		TenantName: "Test",
		APIBase:    "https://api.example",
		WifiSlug:   "test-slug",
		Offers: []PortalOffer{
			{Name: "1h", PriceFcfa: 100, ValidityMin: 60, WaveURL: "https://wave/amount/100/"},
		},
	}
	content := `<script type="application/json" id="mikcloud-config">{{MIKCLOUD_CONFIG_JSON}}</script>`
	out := Personalize(content, cfg)
	// Extraire le JSON du bloc <script>.
	start := strings.Index(out, ">") + 1
	end := strings.LastIndex(out, "</script>")
	if start <= 0 || end <= start {
		t.Fatalf("bloc script introuvable : %s", out)
	}
	jsonStr := out[start:end]
	var parsed PortalConfig
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		t.Fatalf("JSON invalide : %v\n%s", err, jsonStr)
	}
	if parsed.TenantName != "Test" || parsed.WifiSlug != "test-slug" {
		t.Errorf("JSON mal parsé : %+v", parsed)
	}
	if len(parsed.Offers) != 1 || parsed.Offers[0].PriceFcfa != 100 {
		t.Errorf("offers mal sérialisées : %+v", parsed.Offers)
	}
}

// TestPersonalizeSecurityScriptInjection — un nom de tenant contenant
// « </script> » ne doit PAS casser le bloc <script type="application/json">.
// encoding/json échappe « < » en « \u003c » par défaut → la séquence
// « </script> » devient « \u003c/script\u003e » dans le JSON, inoffensive.
func TestPersonalizeSecurityScriptInjection(t *testing.T) {
	cfg := PortalConfig{
		TenantName: `Evil</script><script>alert(1)</script>`,
	}
	content := `<script type="application/json" id="mikcloud-config">{{MIKCLOUD_CONFIG_JSON}}</script>`
	out := Personalize(content, cfg)
	// Le bloc <script type="application/json"> ne doit pas être fermé prématurément
	// par le contenu malveillant. On vérifie qu'il n'y a qu'un seul « </script> »
	// (celui légitime de fermeture) ET qu'aucun « <script> » n'apparaît dans le JSON.
	if strings.Count(out, "<script>") != 0 {
		t.Errorf("injection <script> détectée :\n%s", out)
	}
	// Le </script> légitime de fermeture doit être le seul.
	if strings.Count(out, "</script>") != 1 {
		t.Errorf("plusieurs </script> détectés :\n%s", out)
	}
	// Le nom malveillant doit être échappé en \u003c (pas de < brut dans le JSON).
	if strings.Contains(out, "Evel</script>") || strings.Contains(out, "Evil<") {
		t.Errorf("nom malveillant non échappé :\n%s", out)
	}
}

// TestPersonalizeSecurityHTMLInjection — un nom de tenant contenant des
// chevrons dans un contexte HTML (pas JSON) doit être échappé par html.EscapeString.
func TestPersonalizeSecurityHTMLInjection(t *testing.T) {
	cfg := PortalConfig{
		TenantName: `<img src=x onerror=alert(1)>`,
	}
	out := Personalize(`<title>{{MIKCLOUD_TENANT_NAME}}</title>`, cfg)
	if strings.Contains(out, "<img src=x") {
		t.Errorf("injection HTML non neutralisée :\n%s", out)
	}
	if !strings.Contains(out, "&lt;img") {
		t.Errorf("nom non échappé en &lt;... :\n%s", out)
	}
}

// TestPersonalizeNoMarkers — un contenu sans marqueur est retourné tel quel.
func TestPersonalizeNoMarkers(t *testing.T) {
	content := `<html><body>Hello</body></html>`
	out := Personalize(content, PortalConfig{TenantName: "X"})
	if out != content {
		t.Errorf("contenu sans marqueur modifié : %q", out)
	}
}

// TestPersonalizeEmpty — contenu vide → retourne vide (pas de panic).
func TestPersonalizeEmpty(t *testing.T) {
	if out := Personalize("", PortalConfig{}); out != "" {
		t.Errorf("Personalize vide = %q, attendu ''", out)
	}
}

// TestPersonalizeMissingFields — un PortalConfig à champs vides produit des
// marqueurs substitués par des chaînes vides (pas d'erreur, pas de marqueur
// laissé tel quel).
func TestPersonalizeMissingFields(t *testing.T) {
	content := `<a href="{{MIKCLOUD_WIFI_URL}}">{{MIKCLOUD_TENANT_NAME}}</a><img src="{{MIKCLOUD_BANNER_URL}}"/>`
	out := Personalize(content, PortalConfig{})
	if strings.Contains(out, "{{MIKCLOUD_") {
		t.Errorf("marqueur non substitué :\n%s", out)
	}
	if !strings.Contains(out, `href=""`) {
		t.Errorf("URL vide attendue, out : %s", out)
	}
	if !strings.Contains(out, `src=""`) {
		t.Errorf("bannière vide attendue (omitempty côté JSON, chaîne vide côté marqueur), out : %s", out)
	}
}

// TestPersonalizeBannerURLInjection — une bannière malveillante (tentative de
// sortie d'attribut HTML) est échappée : les guillemets deviennent &#34; et ne
// peuvent pas casser l'attribut src ni injecter d'événements.
func TestPersonalizeBannerURLInjection(t *testing.T) {
	cfg := PortalConfig{
		BannerURL: `https://evil/x.jpg" onerror="alert(1)`,
	}
	out := Personalize(`<img src="{{MIKCLOUD_BANNER_URL}}"/>`, cfg)
	if strings.Contains(out, `" onerror="`) {
		t.Errorf("injection non échappée dans la bannière : %s", out)
	}
	if !strings.Contains(out, `&#34;`) {
		t.Errorf("guillemets non échappés HTML : %s", out)
	}
}

// TestPersonalizeOffersInJSON — les offres sont bien sérialisées dans le JSON.
func TestPersonalizeOffersInJSON(t *testing.T) {
	cfg := PortalConfig{
		TenantName: "Test",
		Offers: []PortalOffer{
			{Name: "1h", PriceFcfa: 100, ValidityMin: 60, DataQuotaMb: 0, WaveURL: "https://w/1"},
			{Name: "24h", PriceFcfa: 300, ValidityMin: 1440, DataQuotaMb: 1024, WaveURL: "https://w/2"},
		},
	}
	out := Personalize(`{{MIKCLOUD_CONFIG_JSON}}`, cfg)
	if !strings.Contains(out, `"1h"`) || !strings.Contains(out, `"24h"`) {
		t.Errorf("offres manquantes dans le JSON :\n%s", out)
	}
	if !strings.Contains(out, `"priceFcfa":300`) {
		t.Errorf("prix manquant :\n%s", out)
	}
}

// TestConfigJSONJoinEnabledAlwaysExplicit — N°46 : le champ joinEnabled doit
// être TOUJOURS explicite dans le bloc config JSON, même à false. Sans
// omitempty ce test est trivial, mais il gèle le contrat : si quelqu'un
// rajoute `omitempty` sur JoinEnabled, un réglage console « désactivé » serait
// OMIS du JSON → la page lirait undefined (≠ false) et réactiverait le bouton
// « S'inscrire » malgré le gérant.
func TestConfigJSONJoinEnabledAlwaysExplicit(t *testing.T) {
	for _, tc := range []struct {
		enabled bool
		want    string
	}{
		{true, `"joinEnabled":true`},
		{false, `"joinEnabled":false`},
	} {
		out := Personalize(`{{MIKCLOUD_CONFIG_JSON}}`, PortalConfig{JoinEnabled: tc.enabled})
		if !strings.Contains(out, tc.want) {
			t.Errorf("joinEnabled=%v : %q attendu dans le JSON, obtenu :\n%s", tc.enabled, tc.want, out)
		}
	}
}

// TestLoginTemplateJoinButtonLogic — N°46 : le template embarqué login.html
// porte la logique dynamique du bouton d'inscription :
//   - condition cfg.joinEnabled !== false (undefined = activé, rétrocompat) ;
//   - remplacement du bouton Mikhmon par « S'inscrire » quand activé + lien ;
//   - retrait du bouton (btnQr.remove()) quand désactivé ou sans lien —
//     le reliquat Mikhmon « Scanner un QR Code » ne doit JAMAIS rester.
func TestLoginTemplateJoinButtonLogic(t *testing.T) {
	raw, ok := File("login.html")
	if !ok {
		t.Fatal("login.html introuvable dans le template embarqué")
	}
	body := string(raw)
	for _, want := range []string{
		"cfg.joinEnabled !== false && cfg.joinUrl",
		`btnQr.textContent = "S'inscrire"`,
		"btnQr.remove()",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("logique N°46 absente de login.html : %q", want)
		}
	}
	// Le onclick laksa19 (lien externe sans fonction métier) ne doit plus
	// survivre dans le script hybride : la seule occurrence du domaine doit
	// être dans le bouton statique initial, que applyConfig remplace ou retire.
	if !strings.Contains(body, "S'inscrire") {
		t.Error("le libellé « S'inscrire » doit rester présent dans login.html")
	}
}

// TestTickerJSON — N°138 : le marqueur TICKER_JSON rend le tableau JSON des
// messages DU TENANT (insérable tel quel dans le contexte JS de l'init
// Typed.js) et retombe sur les 3 messages HISTORIQUES du template sans
// configuration (repli neutre — messages WiFi génériques).
func TestTickerJSON(t *testing.T) {
	// Avec messages du tenant : tableau JSON dans l'ordre.
	cfg := PortalConfig{Ticker: []string{"Fibre 100 Mbps", "Ouvert 7j/7"}}
	out := Personalize(`strings: {{MIKCLOUD_TICKER_JSON}},`, cfg)
	if out != `strings: ["Fibre 100 Mbps","Ouvert 7j/7"],` {
		t.Errorf("TICKER_JSON mal substitué : %s", out)
	}
	// Sans messages : les 3 messages historiques du template.
	out = Personalize(`strings: {{MIKCLOUD_TICKER_JSON}},`, PortalConfig{})
	if out != `strings: ["Wifi haut débit !","Disponible 24H/24","Payez facilement par Wave !"],` {
		t.Errorf("repli historique absent : %s", out)
	}
}

// TestTickerJSONInjection — un message contenant </script> ou des guillemets
// ne peut pas sortir du contexte JS du template (encoding/json échappe <, >,
// & par défaut) et le JSON substitué reste parsable — round-trip exact des
// messages du tenant.
func TestTickerJSONInjection(t *testing.T) {
	msgs := []string{`</script><script>alert(1)</script>`, `Say "hello" \o/`}
	out := Personalize(`<script>var x = {{MIKCLOUD_TICKER_JSON}};</script>`, PortalConfig{Ticker: msgs})
	if strings.Contains(out, "</script><script>") {
		t.Errorf("injection </script> non neutralisée : %s", out)
	}
	start := strings.Index(out, "var x = ") + len("var x = ")
	end := strings.Index(out[start:], ";")
	if start <= 0 || end <= 0 {
		t.Fatalf("substitution introuvable : %s", out)
	}
	var got []string
	if err := json.Unmarshal([]byte(out[start:start+end]), &got); err != nil {
		t.Fatalf("le JSON substitué n'est pas parsable : %v (%s)", err, out)
	}
	if !reflect.DeepEqual(got, msgs) {
		t.Errorf("messages non round-trip : %v", got)
	}
}
