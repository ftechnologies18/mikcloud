// FamilyGuard — génération du script RouterOS de contrôle parental.
// Extrait du monolithe agent.go (N°88) — même package, contenu inchangé.
package agent

import (
	"mikcloud/hotspot-api/internal/model"
	"strings"
)

// FamilyGuardMarker — commentaire des règles FILTER posées par FamilyGuard
// (N°82 — couvre-feu internet du WiFi public). Même contrat d'idempotence :
// seules les règles portant ce marqueur sont retirées puis recréées — les
// règles du gérant sont préservées.
const FamilyGuardMarker = "mikcloud-familyguard"

// FamilyGuardActiveFromPayload — état demandé par la commande familyguard
// (couvre-feu en cours ou non). L'état est calculé PAR LE CLOUD au moment de
// la mise en file (UTC == heure d'Abidjan, sans DST) : l'horloge routeur
// n'est jamais consultée (un routeur sans NTP verrait le couvre-feu partir
// à la mauvaise heure via le paramètre natif time=). Le payload normalise
// au repli prudent false.

// FamilyGuardActiveFromPayload — état demandé par la commande familyguard
// (couvre-feu en cours ou non). L'état est calculé PAR LE CLOUD au moment de
// la mise en file (UTC == heure d'Abidjan, sans DST) : l'horloge routeur
// n'est jamais consultée (un routeur sans NTP verrait le couvre-feu partir
// à la mauvaise heure via le paramètre natif time=). Le payload normalise
// au repli prudent false.
func FamilyGuardActiveFromPayload(p map[string]any) bool {
	b, _ := p["active"].(bool)
	return b
}

// AntiVpnMarker — commentaire des règles FILTER posées par AntiVPN (N°88 —
// bloque-VPN du WiFi public). Même contrat d'idempotence : seules les
// règles portant ce marqueur sont retirées puis recréées — les règles du
// gérant sont préservées.

// buildFamilyGuard — N°82 : couvre-feu internet du WiFi public (FamilyGuard).
//
// Principe : pendant la fenêtre programmée par le gérant (ex. 22:00 → 06:00),
// UNE règle FILTER par serveur hotspot coupe l'internet des clients —
// chain=forward ciblée sur l'INTERFACE du hotspot, lue SUR le routeur
// (foreach /ip hotspot find — s'adapte à toute topologie, pattern N°81),
// posée en tête de chaîne (place-before=0, au-dessus d'un éventuel fasttrack
// d'établies : les connexions EN COURS sont coupées immédiatement, pas seulement
// les nouvelles), action=reject reject-with=icmp-network-unreachable (échec
// immédiat côté appareil — pas de navigateur qui tourne dans le vide).
//
// La page du portail captif reste accessible (chain=input, servie par le
// routeur) : les vouchers restent validables pendant le couvre-feu, seul
// l'internet est coupé. Le réseau du gérant (LAN, hors interface hotspot)
// et le trafic propre du routeur (chain=output : check-in agent, DNS) ne
// sont JAMAIS touchés ; /ip firewall nat et /ip dns non plus (SafeWiFi N°80
// reste maître du port 53).
//
// L'ÉTAT (active=true/false) est calculé PAR LE CLOUD au moment de la mise
// en file (l'horloge de référence, en UTC == heure d'Abidjan) : le script
// ne consulte JAMAIS l'horloge routeur (un routeur sans NTP verrait le
// couvre-feu partir à la mauvaise heure). La bascule s'applique au check-in
// suivant (≤ 45 s console ouverte, ≤ 180 s en veille).
//
//	active=false : retire les règles marquées (retour à l'état antérieur) ;
//	active=true  : les repose — exactement 1 règle par serveur hotspot.
//
// Idempotent : seules les règles marquées "mikcloud-familyguard" sont
// remplacées. Le rapport échoe le nombre de règles marquées présentes APRÈS
// application ET le nombre de serveurs hotspots trouvés (vérité routeur —
// le cloud ne pose la signature que si rules == 1 × hotspots, 0 sinon).
//
// 0 Mo de RAM routeur (règle sans état), 0 FCFA d'infrastructure —
// compatible MIPS 128 Mo (RB951Ui-2HnD).
func (b Builder) buildFamilyGuard(cmd model.Command) string {
	active := FamilyGuardActiveFromPayload(cmd.Payload)
	okVar := "ok" + idSafe(cmd.ID)
	var sb strings.Builder
	sb.WriteString(header(cmd))
	sb.WriteString(":local " + okVar + " true\n")
	sb.WriteString(":do {\n  /ip firewall filter remove [find comment=\"" + FamilyGuardMarker + "\"]\n} on-error={}\n")
	sb.WriteString(":local fgn 0\n")
	if active {
		sb.WriteString(":foreach fgh in=[/ip hotspot find] do={\n")
		sb.WriteString("  :set fgn ($fgn + 1)\n")
		sb.WriteString("  :local fgi [/ip hotspot get $fgh interface]\n")
		sb.WriteString("  :do {\n    /ip firewall filter add chain=forward place-before=0 in-interface=$fgi action=reject reject-with=icmp-network-unreachable comment=\"" + FamilyGuardMarker + "\"\n  } on-error={ :set " + okVar + " false }\n")
		sb.WriteString("}\n")
	}
	// Rapport — vérité routeur : le compte de règles marquées présentes
	// après application ET le nombre de serveurs hotspots trouvés
	// (valeurs DYNAMIQUES côté routeur, pattern fetchResultData).
	sb.WriteString(":local fgr [:len [/ip firewall filter find comment=\"" + FamilyGuardMarker + "\"]]\n")
	ok := `/tool fetch url="` + strings.TrimRight(b.BaseURL, "/") + `/agent/result?token=` + urlEscape(b.Token) +
		`" http-method=post http-data=("cmd=` + urlEscape(cmd.ID) + `&status=ok&rules=". $fgr . "&hs=". $fgn) output=none`
	ko := b.reportLine(cmd.ID, false, map[string]string{"message": "echec des regles du couvre-feu sur le routeur"})
	sb.WriteString(":if ($" + okVar + ") do={\n  " + ok + "\n} else={\n  " + ko + "\n}\n")
	return sb.String()
}

// buildAntiVpn — N°88 : bloque-VPN du WiFi public (AntiVPN).
//
// Principe : QUATRE règles FILTER par serveur hotspot du routeur, posées
// en TÊTE de chaîne (place-before=0, au-dessus d'un éventuel fasttrack :
// les tunnels EN COURS sont coupés immédiatement, pas seulement les
// nouveaux) et ciblées sur l'INTERFACE du hotspot, lue SUR le routeur
// (foreach /ip hotspot find — s'adapte à toute topologie et suit un
// renommage d'interface à la réparation 6 h suivante, pattern N°81) :
//
//	GRE (47) : tunnels PPTP et encapsulations GRE ;
//	ESP (50) : IPsec/IKEv2 natif (« Ajouter un VPN » Android/iOS) ;
//	UDP      : 500/4500 (IKE, NAT-T), 1701 (L2TP), 1194 (OpenVPN),
//	           51820 (WireGuard), 2408 (Cloudflare WARP) ;
//	TCP      : 1723 (PPTP), 1194 (OpenVPN), 9001/9030 (Tor).
//
// Ce que le module ne touche JAMAIS : le port 53 (SafeWiFi N°80 reste
// maître du DNS), le NTP (123) et l'UDP 443 (appels WhatsApp — critiques
// en Côte d'Ivoire — et QUIC). La limite résiduelle (un tunnel camouflé
// en HTTPS pur) est documentée dans la footnote du module : un MVP ne
// vend pas de DPI.
//
// IPv6 : miroir best-effort (pattern N°85) — mêmes coupures en /ipv6
// firewall filter, on-error silencieux : un routeur sans pile IPv6 n'a
// ni règles à poser ni échappatoire à fermer. Non comptées au rapport.
//
//	off : retire les règles marquées (retour à l'état antérieur).
//
// Idempotent : seules les règles marquées "mikcloud-antivpn" sont
// remplacées, celles du gérant sont conservées. Le rapport échoe le
// compte de règles marquées IPv4 présentes APRÈS application ET le
// nombre de serveurs hotspots trouvés — le cloud ne pose la signature
// que si rules == 4 × hotspots (pattern Shield N°81).
