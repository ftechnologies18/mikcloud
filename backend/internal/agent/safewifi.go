// SafeWiFi — génération du script RouterOS de filtrage DNS (DNS chiffré et IPv6 coupés, N°85).
// Extrait du monolithe agent.go (N°88) — même package, contenu inchangé.
package agent

import (
	"mikcloud/hotspot-api/internal/model"
	"strings"
)

// SafeWifiMarker — commentaire des règles NAT posées par SafeWiFi (N°80 —
// protection DNS du WiFi public). Même contrat d'idempotence : seules les
// règles portant ce marqueur sont retirées puis recréées — les règles du
// gérant sont préservées.
const SafeWifiMarker = "mikcloud-safewifi"

// Résolveurs filtrants publics (anycast, gratuits) par niveau N°80 :
//
//      threats : Quad9 — malwares, phishing, arnaques ;
//      family  : AdGuard Family — + contenus adultes, publicités.

// Résolveurs filtrants publics (anycast, gratuits) par niveau N°80 :
//
//	threats : Quad9 — malwares, phishing, arnaques ;
//	family  : AdGuard Family — + contenus adultes, publicités.
const (
	SafeWifiDNSQuad9         = "9.9.9.9"
	SafeWifiDNSAdGuardFamily = "94.140.14.15"
)

// SafeWifiResolver — adresse du résolveur filtrant d'un niveau. Toute valeur
// inconnue retombe sur Quad9 : en cas de doute, le filtrage minimal protège.

// SafeWifiResolver — adresse du résolveur filtrant d'un niveau. Toute valeur
// inconnue retombe sur Quad9 : en cas de doute, le filtrage minimal protège.
func SafeWifiResolver(level string) string {
	if level == model.SafeWifiFamily {
		return SafeWifiDNSAdGuardFamily
	}
	return SafeWifiDNSQuad9
}

// SafeWifiDoHList — nom de l'address-list (v4 ET v6) des endpoints DoH
// publics bloqués depuis le WiFi public quand un filtrage est actif (N°85).

// SafeWifiDoHList — nom de l'address-list (v4 ET v6) des endpoints DoH
// publics bloqués depuis le WiFi public quand un filtrage est actif (N°85).
const SafeWifiDoHList = "mikcloud-safewifi-doh"

// SafeWifiHotspotDnsPort — N°93 : port du servlet DNS NATIF du hotspot
// RouterOS. Les règles dynamiques du hotspot redirigent elles-mêmes le DNS
// de SES clients vers ce port (« chain=hotspot action=redirect
// to-ports=64872 dst-port=53 », manuel « Hotspot customisation », section
// Firewall customizations), et le filtre dynamique hs-input l'accepte
// PRÉ-authentification (ports 64872-64875 : services locaux
// d'authentification et de proxy). C'est ce mécanisme natif qui rend le DNS
// — donc la détection du portail captif par l'OS du client — possible avant
// le login : le servlet résout les noms LOCAUX (dns-name du profil, p.ex.
// « login » Mikhmon) comme les domaines publics.
const SafeWifiHotspotDnsPort = "64872"

// SafeWifiDoHIPv4 — endpoints DoH (DNS-over-HTTPS, tcp/443) des grands
// résolveurs publics, toutes variantes confondues (filtrées ou non) : quand
// un niveau de filtrage est actif, ces adresses sont injoignables depuis le
// WiFi public — les navigateurs en mode « DNS sécurisé » automatique et les
// systèmes configurés en DoH retombent alors sur le DNS simple (port 53 →
// redirigé → filtré). Y compris le DoH du résolveur du niveau choisi : son
// chemin parallèle échapperait à la vérification du cloud. Jeu du chat et
// de la souris assumé et documenté : un endpoint DoH exotique hors liste
// reste joignable ; un VPN contourne toute protection DNS (limite MVP).

// SafeWifiDoHIPv4 — endpoints DoH (DNS-over-HTTPS, tcp/443) des grands
// résolveurs publics, toutes variantes confondues (filtrées ou non) : quand
// un niveau de filtrage est actif, ces adresses sont injoignables depuis le
// WiFi public — les navigateurs en mode « DNS sécurisé » automatique et les
// systèmes configurés en DoH retombent alors sur le DNS simple (port 53 →
// redirigé → filtré). Y compris le DoH du résolveur du niveau choisi : son
// chemin parallèle échapperait à la vérification du cloud. Jeu du chat et
// de la souris assumé et documenté : un endpoint DoH exotique hors liste
// reste joignable ; un VPN contourne toute protection DNS (limite MVP).
var SafeWifiDoHIPv4 = []string{
	// Cloudflare (1.1.1.1 / 1.0.0.1 / cloudflare-dns.com)
	"1.1.1.1", "1.0.0.1", "104.16.248.249", "104.16.249.249",
	// Google (dns.google)
	"8.8.8.8", "8.8.4.4",
	// Quad9 (filtré ET non filtré)
	"9.9.9.9", "149.112.112.112", "9.9.9.10", "149.112.112.10",
	// AdGuard (default, family, non filtré)
	"94.140.14.14", "94.140.15.15", "94.140.14.15", "94.140.15.16",
	"94.140.14.140", "94.140.14.141",
	// OpenDNS (+ FamilyShield)
	"208.67.222.222", "208.67.220.220", "208.67.222.123", "208.67.220.123",
	// CleanBrowsing, Yandex, Comodo, DNS.SB
	"185.228.168.9", "185.228.169.9",
	"77.88.8.8", "77.88.8.1",
	"8.26.56.26", "8.20.247.20",
	"185.222.222.222", "45.11.45.11",
}

// SafeWifiDoHIPv6 — mêmes endpoints DoH, en IPv6 : le NAT SafeWiFi est IPv4
// (/ip firewall nat) — le DNS IPv6 ne le traverse pas. Quand un filtrage est
// actif, le DNS IPv6 (tcp+udp 53) et le DoT/DoH IPv6 sont COUPÉS depuis le
// WiFi public : l'appareil retombe sur le chemin IPv4, redirigé → filtré.
// Contre-mesure best-effort (on-error silencieux) : un routeur sans pile
// IPv6 n'a rien à couper — il n'a pas non plus d'échappatoire IPv6.

// SafeWifiDoHIPv6 — mêmes endpoints DoH, en IPv6 : le NAT SafeWiFi est IPv4
// (/ip firewall nat) — le DNS IPv6 ne le traverse pas. Quand un filtrage est
// actif, le DNS IPv6 (tcp+udp 53) et le DoT/DoH IPv6 sont COUPÉS depuis le
// WiFi public : l'appareil retombe sur le chemin IPv4, redirigé → filtré.
// Contre-mesure best-effort (on-error silencieux) : un routeur sans pile
// IPv6 n'a rien à couper — il n'a pas non plus d'échappatoire IPv6.
var SafeWifiDoHIPv6 = []string{
	"2606:4700:4700::1111", "2606:4700:4700::1001", // Cloudflare
	"2001:4860:4860::8888", "2001:4860:4860::8844", // Google
	"2620:fe::fe", "2620:fe::9", // Quad9
	"2a10:50c0::ad1:ff", "2a10:50c0::ad2:ff", // AdGuard
	"2620:119:35::35", "2620:119:53::53", // OpenDNS
}

// SafeWifiFilterRulesPerHotspot — règles FILTER IPv4 posées par serveur
// hotspot quand un filtrage est actif (N°85) : DoT (tcp/853) + DoH (tcp/443
// vers la liste SafeWifiDoHList). La vérification du retour compte ces
// règles — le compte attendu dépend donc du nombre de hotspots RAPPORTÉ.

// SafeWifiFilterRulesPerHotspot — règles FILTER IPv4 posées par serveur
// hotspot quand un filtrage est actif (N°85) : DoT (tcp/853) + DoH (tcp/443
// vers la liste SafeWifiDoHList). La vérification du retour compte ces
// règles — le compte attendu dépend donc du nombre de hotspots RAPPORTÉ.
const SafeWifiFilterRulesPerHotspot = 2

// SafeWifiNatRules — règles NAT IPv4 posées par un niveau actif : 2 dst-nat
// (redirection vers le résolveur filtrant, N°80/N°85) + 2 redirect
// pré-authentification vers le servlet DNS natif du hotspot (N°93). La
// vérification du retour compte ces règles — via SafeWifiRulesExpected.
const SafeWifiNatRules = 4

// SafeWifiRulesExpected — compte TOTAL d'objets marqués attendu côté routeur
// pour un niveau actif : SafeWifiNatRules règles NAT + les entrées de la
// liste DoH v4 + SafeWifiFilterRulesPerHotspot règles par serveur hotspot.
// Miroir exact du comptage du script (nat + filter + address-list, IPv4
// uniquement — l'IPv6 est best-effort, non comptée).
func SafeWifiRulesExpected(hotspots int) int {
	return SafeWifiNatRules + len(SafeWifiDoHIPv4) + SafeWifiFilterRulesPerHotspot*hotspots
}

// SafeWifiLevelFromPayload — niveau d'une commande safewifi, normalisé
// ("" ou valeur inconnue → off : le script ne pose alors aucune règle).

// SafeWifiLevelFromPayload — niveau d'une commande safewifi, normalisé
// ("" ou valeur inconnue → off : le script ne pose alors aucune règle).
func SafeWifiLevelFromPayload(p map[string]any) string {
	if s, _ := p["level"].(string); model.ValidSafeWifiLevel(s) {
		return s
	}
	return model.SafeWifiOff
}

// ShieldMarker — commentaire des règles FILTER posées par Shield (N°81 —
// bouclier réseau du WiFi public). Même contrat d'idempotence : seules
// les règles portant ce marqueur sont retirées puis recréées — les
// règles du gérant sont préservées.

// buildSafeWifi — N°80 : protection DNS du WiFi public (SafeWiFi) —
// durcie N°85 : le DNS filtré devient le SEUL chemin de sortie ; corrigée
// N°93 : le portail captif redevient détectable avant le login.
//
// Principe : deux règles NAT dst-nat réécrivent TOUT le port 53 transitant
// par le routeur (requêtes des clients vers n'importe quel résolveur, y
// compris celles adressées au routeur lui-même) vers le résolveur filtrant
// du niveau. Le DNS PROPRE du routeur n'est pas touché (son trafic part en
// chain=output, hors dstnat) : le check-in agent et la résolution locale
// restent intacts quel que soit l'état du résolveur filtrant — la
// disponibilité du site prime sur la stricteté du filtrage, et /ip dns
// n'est JAMAIS modifié.
//
// Durcissement N°85 — le constat terrain qui l'impose : un site adulte
// restait accessible sur un routeur « Protection familles » active. Trois
// échappatoires à fermer, une par une :
//
//  1. ORDRE DES RÈGLES — les règles NAT se posaient en FIN de table : une
//     règle dstnat antérieure (redirect DNS hérité d'une config Mikhmon ou
//     d'un tutoriel hotspot) interceptait le port 53 AVANT MikCloud,
//     silencieusement — la signature ne comptait que la PRÉSENCE des
//     règles marquées (rules=2), jamais leur EFFECTIVITÉ. Désormais
//     place-before=0 : tête de table, rien ne passe devant (miroir Shield
//     N°81 / FamilyGuard N°82). Bump du sel sw-v1 → sw-v2 : tout le parc
//     reçoit la nouvelle forme au check-in suivant, sans intervention.
//
//  2. DNS CHIFFRÉ — DoT (tcp/853, « DNS privé » Android) est bloqué depuis
//     l'interface hotspot ; DoH (tcp/443 vers les endpoints publics connus,
//     liste SafeWifiDoHList) est bloqué de même : l'appareil retombe sur le
//     DNS simple → redirigé → filtré. Limite résiduelle documentée : un
//     endpoint DoH exotique hors liste reste joignable, un VPN contourne
//     toute protection DNS (DPI hors de portée d'un routeur 128 Mo).
//
//  3. IPv6 — le NAT SafeWiFi est IPv4 : un appareil dual-stack résolvait
//     en IPv6, hors de portée des règles. Désormais DNS IPv6 (tcp+udp 53),
//     DoT et DoH IPv6 sont coupés depuis l'interface hotspot : repli IPv4,
//     redirigé → filtré. Best-effort (on-error silencieux) : un routeur
//     sans pile IPv6 n'a ni règles à poser ni échappatoire à fermer.
//
// Correctif N°93 — la régression que le N°85 a introduite sur le terrain :
// le portail captif de CYBER-ESPACE SC (routeur migré Mikhmon) ne
// s'affichait PLUS après le déploiement. Mécanisme exact (documenté par le
// manuel « Hotspot customisation », section Firewall customizations) : le
// hotspot possède ses PROPRES règles dynamiques NAT — un jump
// « chain=dstnat hotspot=from-client → hotspot » puis un redirect natif du
// port 53 vers son servlet DNS interne (64872), servlet accepté
// PRÉ-authentification par le filtre hs-input. Nos dst-nat en tête de
// table (N°85) préemptaient ce redirect pour TOUT le monde : le DNS d'un
// client NON authentifié partait en forward vers l'IP EXTERNE du résolveur
// filtrant → rejeté par le filtre hs-unauth (tout ce qui n'est pas
// walled-garden) → plus de résolution pré-login → la détection de portail
// captif de l'OS (connectivitycheck.gstatic.com, captive.apple.com…)
// échouait silencieusement → le portail ne s'affichait JAMAIS ; sur un
// profil avec dns-name (config Mikhmon typique), le nom local était de
// surcroît indésolvable chez le résolveur public.
//
// Le correctif restitue le comportement natif pour les clients NON
// authentifiés SANS rouvrir l'échappatoire N°85 : DEUX règles redirect
// « hotspot=from-client,!auth » (le matcher NATIF du hotspot, utilisé par
// ses propres règles dynamiques) renvoient le port 53 pré-login vers le
// servlet 64872 — posées APRÈS les dst-nat dans le script, donc
// AU-DESSUS d'elles dans la table (place-before=0 empile en ordre
// inverse). Le servlet répond lui-même (noms locaux + domaines publics,
// upstream non filtré — un invité ne peut de toute façon rien ouvrir hors
// walled-garden avant le login) ; dès l'authentification, le matcher ne
// matche plus et le DNS transite par les dst-nat → résolveur filtrant :
// la promesse N°85 est intégralement conservée pour tout ce qui est
// authentifié, ainsi que pour le LAN du gérant et tout DNS externe. Garde
// de disponibilité : si la famille NAT échoue à la pose (p.ex. matcher
// absent d'une version exotique), le script RETIRE les règles marquées de
// la famille — le portail reste servi par le servlet natif (doctrine N°80
// : la disponibilité prime) et le rapport échoue → re-file au check-in
// suivant. Bump du sel sw-v2 → sw-v3 : tout le parc corrige au check-in
// suivant.
//
//	off      : retire les règles et listes marquées (retour à l'état antérieur) ;
//	threats  : Quad9 (malwares, phishing, arnaques) ;
//	family   : AdGuard Family (+ contenus adultes, publicités).
//
// Idempotent : seuls les objets marqués "mikcloud-safewifi" sont remplacés
// (NAT + FILTER v4, listes DoH v4/v6, règles IPv6), ceux du gérant sont
// conservés. Le rapport échoe le compte d'objets marqués IPv4 présents
// APRÈS application (nat + filter + address-list — vérité routeur : le
// cloud ne pose la signature que si ce compte est exact) ET le nombre de
// serveurs hotspots trouvés (pattern Shield N°81 : le compte attendu en
// dépend).
func (b Builder) buildSafeWifi(cmd model.Command) string {
	level := SafeWifiLevelFromPayload(cmd.Payload)
	okVar := "ok" + idSafe(cmd.ID)
	var sb strings.Builder
	sb.WriteString(header(cmd))
	sb.WriteString(":local " + okVar + " true\n")
	// Retraits idempotents — IPv4 (NAT, FILTER, liste DoH)…
	sb.WriteString(":do {\n  /ip firewall nat remove [find comment=\"" + SafeWifiMarker + "\"]\n} on-error={}\n")
	sb.WriteString(":do {\n  /ip firewall filter remove [find comment=\"" + SafeWifiMarker + "\"]\n} on-error={}\n")
	sb.WriteString(":do {\n  /ip firewall address-list remove [find list=\"" + SafeWifiDoHList + "\"]\n} on-error={}\n")
	// …et IPv6 (best-effort : sans pile IPv6, rien à retirer, rien à fermer).
	sb.WriteString(":do {\n  /ipv6 firewall filter remove [find comment=\"" + SafeWifiMarker + "\"]\n} on-error={}\n")
	sb.WriteString(":do {\n  /ipv6 firewall address-list remove [find list=\"" + SafeWifiDoHList + "\"]\n} on-error={}\n")
	sb.WriteString(":local swn 0\n")
	// N°93 — santé de la famille NAT : false si l'une des quatre règles
	// manque à la pose → retrait complet de la famille (le portail
	// captif reste servi par le servlet DNS natif du hotspot, doctrine
	// N°80 : disponibilité prime) + okVar false → rapport d'échec →
	// re-file au check-in suivant.
	sb.WriteString(":local swnat true\n")
	if level != model.SafeWifiOff {
		dns := SafeWifiResolver(level)
		// 1. NAT en TÊTE de table (N°85) : une règle dstnat antérieure —
		// redirect DNS hérité d'une config Mikhmon/tutoriel — ne peut plus
		// prendre le port 53 avant les règles MikCloud. Ajoutées en
		// PREMIER, elles se retrouvent SOUS le bouclier pré-auth posé
		// juste après (place-before=0 empile en ordre inverse : le
		// dernier ajouté est le plus haut) — l'ordre final voulu est
		// bouclier PUIS dst-nat.
		sb.WriteString(":do {\n  /ip firewall nat add chain=dstnat place-before=0 action=dst-nat to-addresses=" + dns +
			" to-ports=53 protocol=udp dst-port=53 comment=\"" + SafeWifiMarker + "\"\n} on-error={ :set swnat false }\n")
		sb.WriteString(":do {\n  /ip firewall nat add chain=dstnat place-before=0 action=dst-nat to-addresses=" + dns +
			" to-ports=53 protocol=tcp dst-port=53 comment=\"" + SafeWifiMarker + "\"\n} on-error={ :set swnat false }\n")
		// 1-bis. N°93 — bouclier pré-authentification : DEUX règles
		// redirect matcher hotspot=from-client,!auth (le matcher NATIF
		// du hotspot, utilisé par ses propres règles dynamiques —
		// manuel « Hotspot customisation ») renvoyant le port 53 des
		// clients NON authentifiés vers le servlet DNS interne 64872,
		// accepté pré-login par hs-input. Posées APRÈS les dst-nat
		// ci-dessus → AU-DESSUS dans la table : le DNS pré-login
		// redevient natif (résolution + détection du portail captif),
		// tandis que tout client AUTHENTIFIÉ — le matcher ne matche
		// plus — conserve le chemin dst-nat → résolveur filtrant
		// (promesse N°85 intacte), de même que le LAN du gérant et
		// tout DNS externe.
		sb.WriteString(":do {\n  /ip firewall nat add chain=dstnat place-before=0 hotspot=from-client,!auth action=redirect to-ports=" + SafeWifiHotspotDnsPort +
			" protocol=udp dst-port=53 comment=\"" + SafeWifiMarker + "\"\n} on-error={ :set swnat false }\n")
		sb.WriteString(":do {\n  /ip firewall nat add chain=dstnat place-before=0 hotspot=from-client,!auth action=redirect to-ports=" + SafeWifiHotspotDnsPort +
			" protocol=tcp dst-port=53 comment=\"" + SafeWifiMarker + "\"\n} on-error={ :set swnat false }\n")
		// N°93 — garde de disponibilité : échec de pose dans la
		// famille NAT (p.ex. matcher absent d'une version RouterOS
		// exotique) → retrait complet des règles marquées de la
		// famille : le portail reste servi par le servlet natif
		// (aucune dst-nat résiduelle ne préempte son redirect),
		// l'échec est rapporté et la commande re-file au check-in
		// suivant — jamais d'état à moitié posé qui tuerait le
		// portail.
		sb.WriteString(":if (!$swnat) do={\n  :do {\n    /ip firewall nat remove [find comment=\"" + SafeWifiMarker + "\"]\n  } on-error={}\n  :set " + okVar + " false\n}\n")
		// 2a. Liste DoH IPv4 — endpoints publics connus (bloqués en tcp/443).
		for _, ip := range SafeWifiDoHIPv4 {
			sb.WriteString(":do {\n  /ip firewall address-list add list=\"" + SafeWifiDoHList + "\" address=" + ip +
				"\n} on-error={ :set " + okVar + " false }\n")
		}
		// 2b. Liste DoH IPv6 (best-effort).
		for _, ip := range SafeWifiDoHIPv6 {
			sb.WriteString(":do {\n  /ipv6 firewall address-list add list=\"" + SafeWifiDoHList + "\" address=" + ip +
				"\n} on-error={}\n")
		}
		// 2c/3. Par serveur hotspot : DoT et DoH coupés (v4), DNS IPv6, DoT et
		// DoH IPv6 coupés (v6, best-effort) — l'interface est lue SUR le
		// routeur (pattern N°81 : s'adapte à toute topologie et suit un
		// renommage d'interface à la réparation 6 h suivante).
		sb.WriteString(":foreach h in=[/ip hotspot find] do={\n")
		sb.WriteString("  :set swn ($swn + 1)\n")
		sb.WriteString("  :local swi [/ip hotspot get $h interface]\n")
		sb.WriteString("  :do {\n    /ip firewall filter add chain=forward place-before=0 in-interface=$swi action=drop protocol=tcp dst-port=853 comment=\"" + SafeWifiMarker +
			"\"\n  } on-error={ :set " + okVar + " false }\n")
		sb.WriteString("  :do {\n    /ip firewall filter add chain=forward place-before=0 in-interface=$swi action=drop protocol=tcp dst-port=443 dst-address-list=" + SafeWifiDoHList +
			" comment=\"" + SafeWifiMarker + "\"\n  } on-error={ :set " + okVar + " false }\n")
		sb.WriteString("  :do {\n    /ipv6 firewall filter add chain=forward place-before=0 in-interface=$swi action=drop protocol=tcp dst-port=53 comment=\"" + SafeWifiMarker +
			"\"\n  } on-error={}\n")
		sb.WriteString("  :do {\n    /ipv6 firewall filter add chain=forward place-before=0 in-interface=$swi action=drop protocol=udp dst-port=53 comment=\"" + SafeWifiMarker +
			"\"\n  } on-error={}\n")
		sb.WriteString("  :do {\n    /ipv6 firewall filter add chain=forward place-before=0 in-interface=$swi action=drop protocol=tcp dst-port=853 comment=\"" + SafeWifiMarker +
			"\"\n  } on-error={}\n")
		sb.WriteString("  :do {\n    /ipv6 firewall filter add chain=forward place-before=0 in-interface=$swi action=drop protocol=tcp dst-port=443 dst-address-list=" + SafeWifiDoHList +
			" comment=\"" + SafeWifiMarker + "\"\n  } on-error={}\n")
		sb.WriteString("}\n")
	}
	// Rapport — vérité routeur : le compte d'objets marqués IPv4 présents
	// après application (NAT + FILTER + liste DoH — valeur DYNAMIQUE
	// calculée côté routeur, pattern fetchResultData : le cloud ne croit que
	// ce que le routeur rapporte) ET le nombre de serveurs hotspots trouvés
	// (pattern Shield N°81 : le compte attendu en dépend).
	sb.WriteString(":local swr ([:len [/ip firewall nat find comment=\"" + SafeWifiMarker + "\"]] + [:len [/ip firewall filter find comment=\"" + SafeWifiMarker +
		"\"]] + [:len [/ip firewall address-list find list=\"" + SafeWifiDoHList + "\"]])\n")
	ok := `/tool fetch url="` + strings.TrimRight(b.BaseURL, "/") + `/agent/result?token=` + urlEscape(b.Token) +
		`" http-method=post http-data=("cmd=` + urlEscape(cmd.ID) + `&status=ok&rules=". $swr . "&hs=". $swn) output=none`
	ko := b.reportLine(cmd.ID, false, map[string]string{"message": "echec des regles de protection sur le routeur"})
	sb.WriteString(":if ($" + okVar + ") do={\n  " + ok + "\n} else={\n  " + ko + "\n}\n")
	return sb.String()
}

// buildShield — N°81 : bouclier réseau du WiFi public (Shield).
//
// Principe : cinq règles FILTER par serveur hotspot du routeur, posées
// en TÊTE de chaîne (place-before=0) et ciblées sur l'INTERFACE du
// hotspot — le réseau du gérant (LAN) et le trafic propre du routeur
// (chain=output : check-in agent, DNS sortant) ne sont JAMAIS touchés :
//
//      input   ×2 : ports d'administration (winbox, ssh, telnet, api, ftp)
//                  inaccessibles DEPUIS le WiFi public ;
//      forward ×3 : connexions invalides, SMB et NetBIOS bloqués pour les
//                  appareils des clients (vecteurs de propagation des
//                  malwares sur réseau partagé) et vers le réseau du gérant.
//
//      L'interface du hotspot est lue SUR le routeur (foreach /ip hotspot
//      find) : le script s'adapte à toute topologie (wlan1,
//      bridge-hotspot…) et suit un renommage d'interface à la prochaine
//      réparation 6 h (vérité routeur, pattern fetchResultData).
//
//      off : retire les règles marquées (retour à l'état antérieur).
//
// Idempotent : seules les règles marquées "mikcloud-shield" sont
// remplacées, celles du gérant sont conservées. Le rapport échoe le
// nombre de règles marquées présentes APRÈS application ET le nombre
// de serveurs hotspots trouvés — le cloud ne pose la signature que si
// rules == 5 × hotspots (2 input + 3 forward par hotspot).
//
// Limites documentées (MVP) : le blindage s'applique au trafic IPv4
// traversant le routeur — l'isolation L2 de deux appareils d'un même
// pont (client-à-client sans traverser le routeur) relève du réglage
// du pont (use-ip-firewall, coûteux sur MIPS), hors de portée d'un MVP
// non intrusif ; l'administration reste possible depuis MikCloud
// (agent, connexions sortantes) et depuis le réseau local du gérant.
