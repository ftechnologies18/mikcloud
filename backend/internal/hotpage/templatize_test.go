// Package hotpage — tests du templating par compte (N°35-b).
package hotpage

import (
	"encoding/json"
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
	}
	content := `<title>{{MIKCLOUD_TENANT_NAME}}</title>
<a href="{{MIKCLOUD_WIFI_URL}}">WiFi</a>
<form action="{{MIKCLOUD_JOIN_URL}}">Join</form>
<a href="{{MIKCLOUD_WAVE_LINK}}/amount/100/">Wave</a>
<img src="{{MIKCLOUD_LOGO_URL}}"/>
<base data-api="{{MIKCLOUD_API_BASE}}"/>`
	out := Personalize(content, cfg)
	for _, want := range []string{
		`<title>Cyber Espace SC</title>`,
		`href="https://mikcloud.ftci.fr/wifi/cyber-espace-sc"`,
		`action="https://mikcloud.ftci.fr/join/abcdef1234567890"`,
		`href="https://pay.wave.com/m/M_xxx/c/ci//amount/100/"`,
		`src="data:image/png;base64,xyz"`,
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
	content := `<a href="{{MIKCLOUD_WIFI_URL}}">{{MIKCLOUD_TENANT_NAME}}</a>`
	out := Personalize(content, PortalConfig{})
	if strings.Contains(out, "{{MIKCLOUD_") {
		t.Errorf("marqueur non substitué :\n%s", out)
	}
	if !strings.Contains(out, `href=""`) {
		t.Errorf("URL vide attendue, out : %s", out)
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
