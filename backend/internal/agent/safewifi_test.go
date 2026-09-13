package agent

// Tests N°80/N°85 — script SafeWiFi (protection DNS du WiFi public par
// redirection, durcie : tête de table NAT, DoT/DoH coupés, IPv6 neutralisé).
// Le contrat :
//   - niveau actif : retrait idempotent PUIS exactement deux règles dst-nat
//     (udp + tcp) en TÊTE de table (place-before=0) vers LE résolveur du
//     niveau, marquées mikcloud-safewifi ; la liste DoH v4 complète ; et par
//     serveur hotspot : DoT (tcp/853) et DoH (tcp/443 → liste) coupés, plus
//     les coupures IPv6 (DNS, DoT, DoH) best-effort ;
//   - /ip dns n'est JAMAIS touché : le DNS propre du routeur (check-in
//     agent) ne doit pas dépendre de la disponibilité du résolveur filtrant ;
//   - niveau off (ou inconnu) : retrait seul — aucune règle posée ;
//   - le rapport échoe le compte d'objets marqués IPv4 PRÉSENTS après
//     application ET le nombre de serveurs hotspots (vérité routeur).

import (
	"strings"
	"testing"

	"mikcloud/hotspot-api/internal/model"
)

// TestSafeWifiScriptActiveLevels — menaces et famille posent exactement deux
// règles dst-nat (udp+tcp) EN TÊTE de table vers le résolveur du niveau,
// la liste DoH v4 complète, et les blocages anti-contournement par hotspot.
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
		if got := strings.Count(s, `place-before=0 action=dst-nat`); got != 2 {
			t.Errorf("niveau %s : %d règles dst-nat en tête de table (place-before=0), attendu 2 — une règle dstnat antérieure ne doit JAMAIS passer devant (N°85)", tc.level, got)
		}
		if !strings.Contains(s, `protocol=udp dst-port=53 comment="`+SafeWifiMarker+`"`) {
			t.Errorf("niveau %s : la règle udp marquée manque", tc.level)
		}
		if !strings.Contains(s, `protocol=tcp dst-port=53 comment="`+SafeWifiMarker+`"`) {
			t.Errorf("niveau %s : la règle tcp marquée manque", tc.level)
		}
		// N°85 — DoT coupé (tcp/853) depuis chaque interface hotspot : le
		// « DNS privé » d'Android ne doit plus contourner la redirection.
		if !strings.Contains(s, `action=drop protocol=tcp dst-port=853 comment="`+SafeWifiMarker+`"`) {
			t.Errorf("niveau %s : le blocage DoT (tcp/853) manque — le « DNS privé » Android contournerait la redirection", tc.level)
		}
		// N°85 — DoH coupé (tcp/443 vers la liste des endpoints connus) : le
		// « DNS sécurisé » automatique des navigateurs doit retomber sur le 53.
		if !strings.Contains(s, `action=drop protocol=tcp dst-port=443 dst-address-list=`+SafeWifiDoHList) {
			t.Errorf("niveau %s : le blocage DoH (tcp/443 → %s) manque — le « DNS sécurisé » des navigateurs contournerait la redirection", tc.level, SafeWifiDoHList)
		}
		// N°85 — chaque endpoint DoH v4 est bien dans la liste.
		for _, ip := range SafeWifiDoHIPv4 {
			if !strings.Contains(s, `address-list add list="`+SafeWifiDoHList+`" address=`+ip) {
				t.Errorf("niveau %s : l'endpoint DoH %s manque dans la liste", tc.level, ip)
			}
		}
		// N°85 — IPv6 : le DNS IPv6 (tcp+udp 53) est coupé depuis l'interface
		// hotspot (le NAT est IPv4 : sans coupure, un appareil dual-stack
		// résoudrait hors de portée des règles).
		if !strings.Contains(s, `/ipv6 firewall filter add chain=forward place-before=0 in-interface=$swi action=drop protocol=tcp dst-port=53`) {
			t.Errorf("niveau %s : la coupure DNS IPv6 (tcp/53) manque", tc.level)
		}
		if !strings.Contains(s, `/ipv6 firewall filter add chain=forward place-before=0 in-interface=$swi action=drop protocol=udp dst-port=53`) {
			t.Errorf("niveau %s : la coupure DNS IPv6 (udp/53) manque", tc.level)
		}
		if !strings.Contains(s, `/ipv6 firewall filter add chain=forward place-before=0 in-interface=$swi action=drop protocol=tcp dst-port=853`) {
			t.Errorf("niveau %s : la coupure DoT IPv6 (tcp/853) manque", tc.level)
		}
		if !strings.Contains(s, `/ipv6 firewall filter add chain=forward place-before=0 in-interface=$swi action=drop protocol=tcp dst-port=443 dst-address-list=`+SafeWifiDoHList) {
			t.Errorf("niveau %s : la coupure DoH IPv6 (tcp/443 → %s) manque", tc.level, SafeWifiDoHList)
		}
		if strings.Contains(s, "/ip dns set") {
			t.Errorf("niveau %s : le script ne doit PAS toucher /ip dns (disponibilité du check-in agent)", tc.level)
		}
		if !strings.Contains(s, `&rules=". $swr . "&hs=". $swn`) {
			t.Errorf("niveau %s : le rapport doit échoer le compte d'objets ET le nombre de hotspots DYNAMIQUES (vérité routeur, pattern Shield N°81)", tc.level)
		}
	}
}

// TestSafeWifiScriptOff — le niveau off (et toute valeur inconnue) ne RETIRE
// que les objets marqués : aucune règle, aucune liste, retour à l'état
// antérieur — ni règle orpheline ni entrée d'address-list résiduelle.
func TestSafeWifiScriptOff(t *testing.T) {
	b := Builder{BaseURL: "https://api.example", Token: "tok"}
	for _, level := range []string{model.SafeWifiOff, "", "niveau-inconnu"} {
		cmd := model.Command{ID: "c-sw", Kind: model.CmdSafeWifi, Payload: map[string]any{"level": level}}
		s := b.buildSafeWifi(cmd)
		if strings.Contains(s, "firewall nat add") {
			t.Errorf("niveau %q : off ne doit poser AUCUNE règle NAT", level)
		}
		if strings.Contains(s, "firewall filter add") {
			t.Errorf("niveau %q : off ne doit poser AUCUNE règle FILTER", level)
		}
		if strings.Contains(s, "address-list add") {
			t.Errorf("niveau %q : off ne doit poser AUCUNE entrée d'address-list", level)
		}
		if !strings.Contains(s, `remove [find comment="`+SafeWifiMarker+`"]`) {
			t.Errorf("niveau %q : le retrait des règles marquées manque", level)
		}
		if !strings.Contains(s, `remove [find list="`+SafeWifiDoHList+`"]`) {
			t.Errorf("niveau %q : le retrait de la liste DoH manque — des entrées orphelines resteraient sur le routeur", level)
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

// TestSafeWifiRulesExpected — miroir exact du comptage du script (N°85) :
// 2 NAT + liste DoH v4 + 2 règles FILTER par hotspot. Toute évolution de la
// liste DoH change le compte attendu → le sel sw-v2 doit aussi bouger.
func TestSafeWifiRulesExpected(t *testing.T) {
	if got := SafeWifiRulesExpected(1); got != 2+len(SafeWifiDoHIPv4)+2 {
		t.Errorf("SafeWifiRulesExpected(1) = %d, attendu %d", got, 2+len(SafeWifiDoHIPv4)+2)
	}
	if got := SafeWifiRulesExpected(3); got != 2+len(SafeWifiDoHIPv4)+6 {
		t.Errorf("SafeWifiRulesExpected(3) = %d, attendu %d", got, 2+len(SafeWifiDoHIPv4)+6)
	}
	if SafeWifiRulesExpected(0) == SafeWifiRulesExpected(1) {
		t.Error("le compte attendu doit dépendre du nombre de hotspots (pattern Shield N°81)")
	}
	if len(SafeWifiDoHIPv4) == 0 || len(SafeWifiDoHIPv6) == 0 {
		t.Error("les listes DoH v4/v6 ne doivent jamais être vides — le durcissement N°85 disparaîtrait silencieusement")
	}
}
