package agent

// Tests N°80/N°85/N°93/N°95 — script SafeWiFi (protection DNS du WiFi
// public par redirection, durcie : tête de table NAT, DoT/DoH coupés,
// IPv6 neutralisé, bouclier pré-auth vers le servlet DNS natif du
// hotspot, ordre des règles IMPOSÉ par move et RAPPORTÉ). Le contrat :
//   - niveau actif : retrait idempotent PUIS exactement quatre règles
//     NAT en TÊTE de table (place-before=0) marquées mikcloud-safewifi —
//     2 boucliers pré-auth (udp + tcp) vers le servlet DNS natif (64872)
//     ÉMIS EN PREMIER + 2 dst-nat (udp + tcp) vers LE résolveur du
//     niveau ; la liste DoH v4 complète ; et par serveur hotspot : DoT
//     (tcp/853) et DoH (tcp/443 → liste) coupés, plus les coupures IPv6
//     (DNS, DoT, DoH) best-effort ;
//   - le bouclier pré-auth est la correction N°93 : sans lui, le DNS d'un
//     client NON authentifié partait en forward vers l'IP externe du
//     résolveur → rejeté par hs-unauth → plus de détection de portail ;
//   - l'ORDRE bouclier AU-DESSUS des dst-nat est la correction N°95 : le
//     post-mortem CYBER-ESPACE SC a démenti « place-before=0 empile en
//     ordre inverse » (la table réelle conserve l'ordre d'émission) — le
//     script émet les boucliers d'abord, IMPOSE l'ordre par un move
//     explicite vers la première dst-nat et RAPPORTE la disposition
//     réelle (layout) ;
//   - /ip dns n'est JAMAIS touché : le DNS propre du routeur (check-in
//     agent) ne doit pas dépendre de la disponibilité du résolveur filtrant ;
//   - niveau off (ou inconnu) : retrait seul — aucune règle posée ;
//   - le rapport échoe le compte d'objets marqués IPv4 PRÉSENTS après
//     application, le nombre de serveurs hotspots ET la disposition NAT
//     (vérité routeur).

import (
	"strings"
	"testing"

	"mikcloud/hotspot-api/internal/model"
)

// TestSafeWifiScriptActiveLevels — menaces et famille posent exactement
// quatre règles NAT en TÊTE de table (2 dst-nat vers le résolveur du niveau
// + 2 boucliers pré-auth vers le servlet DNS natif), la liste DoH v4
// complète, et les blocages anti-contournement par hotspot.
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
		if got := strings.Count(s, `action=dst-nat to-addresses=`+tc.dns+` to-ports=53`); got != 4 {
			t.Errorf("niveau %s : %d occurrences dst-nat vers %s, attendu 4 (2 règles udp+tcp × primaire+repli N°159 — une seule s'exécute sur le routeur)", tc.level, got, tc.dns)
		}
		if got := strings.Count(s, `place-before=0 action=dst-nat`); got != 2 {
			t.Errorf("niveau %s : %d règles dst-nat en tête de table (place-before=0), attendu 2 — une règle dstnat antérieure ne doit JAMAIS passer devant (N°85)", tc.level, got)
		}
		// N°93 — bouclier pré-authentification : le port 53 des clients
		// hotspot NON authentifiés part vers le servlet DNS natif (64872)
		// via le matcher NATIF du hotspot — udp ET tcp, en tête de table.
		if got := strings.Count(s, `hotspot=from-client,!auth action=redirect to-ports=`+SafeWifiHotspotDnsPort); got != 4 {
			t.Errorf("niveau %s : %d occurrences redirect pré-auth (matcher hotspot), attendu 4 (udp+tcp × primaire+repli N°159) — sans ce bouclier le portail captif n'est plus détectable avant le login (régression N°85 corrigée N°93)", tc.level, got)
		}
		if got := strings.Count(s, `place-before=0 hotspot=from-client,!auth action=redirect`); got != 2 {
			t.Errorf("niveau %s : %d boucliers pré-auth en tête de table, attendu 2 — posés via place-before=0, ordre final garanti par le move N°95", tc.level, got)
		}
		// N°95 — ordre DÉTERMINISTE : le post-mortem CYBER-ESPACE SC a démenti
		// la théorie « place-before=0 empile en ordre inverse » (la table
		// réelle conserve l'ordre d'émission → boucliers SOUS les dst-nat →
		// DNS pré-login capté → ERR_NAME_NOT_RESOLVED sur le dns-name du
		// portail). Le script émet les boucliers AVANT les dst-nat ET impose
		// l'ordre par un move explicite : chaque bouclier est déplacé DEVANT
		// la première dst-nat (ancre swtgt), les autres dst-nat suivent.
		if io, ic := strings.Index(s, `hotspot=from-client,!auth action=redirect to-ports=`+SafeWifiHotspotDnsPort), strings.Index(s, `action=dst-nat to-addresses=`+tc.dns); io < 0 || ic < 0 || io > ic {
			t.Errorf("niveau %s : les boucliers pré-auth doivent être émis AVANT les dst-nat (index redirect=%d, dst-nat=%d) — l'ordre d'émission est l'ordre de table observé (post-mortem N°93)", tc.level, io, ic)
		}
		if !strings.Contains(s, `:set swtgt $r`) {
			t.Errorf("niveau %s : la recherche de la première dst-nat (ancre du réordonnancement) manque", tc.level)
		}
		if got := strings.Count(s, `/ip firewall nat move numbers=$r destination=$swtgt`); got != 2 {
			t.Errorf("niveau %s : %d blocs move de réordonnancement, attendu 2 (boucliers puis dst-nat restantes) — sans eux l'ordre resterait une hypothèse (post-mortem N°93)", tc.level, got)
		}
		// N°93 — garde de disponibilité : échec de pose dans la famille
		// NAT → retrait complet des règles marquées de la famille (le
		// portail reste servi par le servlet natif) + échec rapporté.
		if !strings.Contains(s, `:if (!$swnat) do={`) {
			t.Errorf("niveau %s : la garde de retour arrière de la famille NAT manque — un échec de pose laisserait des dst-nat orphelines tuer le portail", tc.level)
		}
		if !strings.Contains(s, `:set swnat false`) {
			t.Errorf("niveau %s : les ajouts NAT doivent alimenter swnat (échec → retrait de la famille)", tc.level)
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
		if !strings.Contains(s, `&rules=". $swr . "&hs=". $swn . "&layout=". $swlay)`) {
			t.Errorf("niveau %s : le rapport doit échoer le compte d'objets, le nombre de hotspots ET la disposition NAT (layout, vérité routeur N°95)", tc.level)
		}
		if !strings.Contains(s, `:set swlay ($swlay . "D")`) || !strings.Contains(s, `:set swlay ($swlay . "R")`) {
			t.Errorf("niveau %s : le calcul de la disposition NAT (layout) manque — le cloud ne peut pas vérifier l'ordre réel des règles", tc.level)
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

// TestSafeWifiRulesExpected — miroir exact du comptage du script (N°93) :
// 4 NAT (2 dst-nat + 2 boucliers pré-auth) + liste DoH v4 + 2 règles FILTER
// par hotspot. Toute évolution de la liste DoH ou du nombre de règles
// change le compte attendu → le sel doit aussi bouger (garde-fou N°48).
func TestSafeWifiRulesExpected(t *testing.T) {
	if got := SafeWifiRulesExpected(1); got != SafeWifiNatRules+len(SafeWifiDoHIPv4)+2 {
		t.Errorf("SafeWifiRulesExpected(1) = %d, attendu %d", got, SafeWifiNatRules+len(SafeWifiDoHIPv4)+2)
	}
	if got := SafeWifiRulesExpected(3); got != SafeWifiNatRules+len(SafeWifiDoHIPv4)+6 {
		t.Errorf("SafeWifiRulesExpected(3) = %d, attendu %d", got, SafeWifiNatRules+len(SafeWifiDoHIPv4)+6)
	}
	if SafeWifiNatRules != 4 {
		t.Errorf("SafeWifiNatRules = %d, attendu 4 (2 dst-nat + 2 boucliers pré-auth N°93) — le miroir du comptage serait faux", SafeWifiNatRules)
	}
	if SafeWifiNatLayout != "RRDD" {
		t.Errorf("SafeWifiNatLayout = %q, attendu \"RRDD\" — les boucliers pré-auth doivent précéder les dst-nat (post-mortem N°93 : un ordre inversé tue le portail captif en silence)", SafeWifiNatLayout)
	}
	if len(SafeWifiNatLayout) != SafeWifiNatRules {
		t.Errorf("len(SafeWifiNatLayout) = %d, attendu SafeWifiNatRules = %d — une lettre par règle NAT marquée, sinon le miroir du rapport est faux", len(SafeWifiNatLayout), SafeWifiNatRules)
	}
	if SafeWifiRulesExpected(0) == SafeWifiRulesExpected(1) {
		t.Error("le compte attendu doit dépendre du nombre de hotspots (pattern Shield N°81)")
	}
	if len(SafeWifiDoHIPv4) == 0 || len(SafeWifiDoHIPv6) == 0 {
		t.Error("les listes DoH v4/v6 ne doivent jamais être vides — le durcissement N°85 disparaîtrait silencieusement")
	}
}

// TestSafeWifiScriptResilient — N°159 — ajout RÉSILIENT + traçage par étape :
// les 4 règles NAT et les 2 règles FILTER par hotspot sont posées ancrées
// (place-before=0) PUIS retentées SANS ancre si elle est rejetée (constat
// production 19/09 : post-boot, « echec_des_regles_de_protection » en boucle
// sur CYBER/ProMax — le WiFi restait sans filtrage DNS) ; le rapport
// d'échec embarque l'étape atteinte ($step).
func TestSafeWifiScriptResilient(t *testing.T) {
	b := Builder{BaseURL: "https://api.example", Token: "tok"}
	cmd := model.Command{ID: "c-sw2", Kind: model.CmdSafeWifi, Payload: map[string]any{"level": model.SafeWifiThreats}}
	s := b.buildSafeWifi(cmd)

	if !strings.Contains(s, `:local step "start"`) {
		t.Error("la variable de traçage $step manque")
	}
	for _, step := range []string{"nat-r-udp", "nat-r-tcp", "nat-d-udp", "nat-d-tcp", "nat-move", "list-v4", "filter-dot", "filter-doh"} {
		if !strings.Contains(s, `:set step "`+step+`"`) {
			t.Errorf("l'étape %q manque (diagnostic sans accès console)", step)
		}
	}
	// Famille NAT : 4 primaires ancrées + 4 replis sans ancre (chain=dstnat
	// directement suivi de action= — sans place-before intercalé).
	if got := strings.Count(s, "place-before=0"); got != 10 {
		t.Errorf("%d formes primaires place-before=0, attendu 10 (4 NAT + 2 filter + 4 IPv6 best-effort)", got)
	}
	if got := strings.Count(s, "/ip firewall nat add chain=dstnat action="); got != 2 {
		t.Errorf("%d replis dst-nat sans ancre, attendu 2", got)
	}
	if got := strings.Count(s, "/ip firewall nat add chain=dstnat hotspot=from-client,!auth"); got != 2 {
		t.Errorf("%d replis redirect sans ancre, attendu 2", got)
	}
	if got := strings.Count(s, "/ip firewall filter add chain=forward in-interface=$swi"); got != 2 {
		t.Errorf("%d replis filter sans ancre, attendu 2 (DoT + DoH)", got)
	}
	// Rapport d'échec dynamique : l'étape atteinte voyage dans le fetch.
	if !strings.Contains(s, `&status=error&message=echec_des_regles_de_protection_sur_le_routeur&step=" . $step`) {
		t.Error("le rapport d'échec doit embarquer $step (étape fautive)")
	}
}
