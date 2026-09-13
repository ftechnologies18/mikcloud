package agent

// Tests N°80 — script SafeWiFi (protection DNS du WiFi public par
// redirection). Le contrat :
//   - niveau actif : retrait idempotent PUIS exactement deux règles dst-nat
//     (udp + tcp) vers LE résolveur du niveau, marquées mikcloud-safewifi ;
//   - /ip dns n'est JAMAIS touché : le DNS propre du routeur (check-in
//     agent) ne doit pas dépendre de la disponibilité du résolveur filtrant ;
//   - niveau off (ou inconnu) : retrait seul — aucune règle posée ;
//   - le rapport échoe le compte de règles marquées PRÉSENTES après
//     application (vérité routeur, valeur dynamique).

import (
	"strings"
	"testing"

	"mikcloud/hotspot-api/internal/model"
)

// TestSafeWifiScriptActiveLevels — menaces et famille posent exactement deux
// règles dst-nat (udp+tcp) vers le résolveur du niveau, après le retrait
// idempotent, sans toucher /ip dns.
func TestSafeWifiScriptActiveLevels(t *testing.T) {
	b := Builder{BaseURL: "https://api.example", Token: "tok"}
	for _, tc := range []struct {
		level string
		dns   string
	}{
		{model.SafeWifiThreats, SafeWifiDNSQuad9},
		{model.SafeWifiFamily, SafeWifiDNSAdGuardFamily},
	} {
		cmd := model.Command{ID: "c-sw", Kind: model.CmdSafeWifi, Payload: map[string]any{"level": tc.level}}
		s := b.buildSafeWifi(cmd)
		if !strings.Contains(s, `remove [find comment="`+SafeWifiMarker+`"]`) {
			t.Errorf("niveau %s : le retrait idempotent des règles marquées manque", tc.level)
		}
		if got := strings.Count(s, `action=dst-nat to-addresses=`+tc.dns+` to-ports=53`); got != 2 {
			t.Errorf("niveau %s : %d règles dst-nat vers %s, attendu 2 (udp+tcp)", tc.level, got, tc.dns)
		}
		if !strings.Contains(s, `protocol=udp dst-port=53 comment="`+SafeWifiMarker+`"`) {
			t.Errorf("niveau %s : la règle udp marquée manque", tc.level)
		}
		if !strings.Contains(s, `protocol=tcp dst-port=53 comment="`+SafeWifiMarker+`"`) {
			t.Errorf("niveau %s : la règle tcp marquée manque", tc.level)
		}
		if strings.Contains(s, "/ip dns set") {
			t.Errorf("niveau %s : le script ne doit PAS toucher /ip dns (disponibilité du check-in agent)", tc.level)
		}
		if !strings.Contains(s, `&rules=". $swr`) {
			t.Errorf("niveau %s : le rapport doit échoer le compte de règles DYNAMIQUE (vérité routeur)", tc.level)
		}
	}
}

// TestSafeWifiScriptOff — le niveau off (et toute valeur inconnue) ne RETIRE
// que les règles marquées : aucune règle posée, retour à l'état antérieur.
func TestSafeWifiScriptOff(t *testing.T) {
	b := Builder{BaseURL: "https://api.example", Token: "tok"}
	for _, level := range []string{model.SafeWifiOff, "", "niveau-inconnu"} {
		cmd := model.Command{ID: "c-sw", Kind: model.CmdSafeWifi, Payload: map[string]any{"level": level}}
		s := b.buildSafeWifi(cmd)
		if strings.Contains(s, "firewall nat add") {
			t.Errorf("niveau %q : off ne doit poser AUCUNE règle", level)
		}
		if !strings.Contains(s, `remove [find comment="`+SafeWifiMarker+`"]`) {
			t.Errorf("niveau %q : le retrait des règles marquées manque", level)
		}
	}
}

// TestSafeWifiResolver — mapping niveau → résolveur, avec repli prudent :
// toute valeur inconnue retombe sur Quad9 (le filtrage minimal protège).
func TestSafeWifiResolver(t *testing.T) {
	if got := SafeWifiResolver(model.SafeWifiThreats); got != SafeWifiDNSQuad9 {
		t.Errorf("SafeWifiResolver(threats) = %q, attendu %q", got, SafeWifiDNSQuad9)
	}
	if got := SafeWifiResolver(model.SafeWifiFamily); got != SafeWifiDNSAdGuardFamily {
		t.Errorf("SafeWifiResolver(family) = %q, attendu %q", got, SafeWifiDNSAdGuardFamily)
	}
	if got := SafeWifiResolver("n'importe-quoi"); got != SafeWifiDNSQuad9 {
		t.Errorf("SafeWifiResolver(inconnu) = %q, attendu le repli %q", got, SafeWifiDNSQuad9)
	}
}
