// Shield — génération du script RouterOS anti-piratage WiFi.
// Extrait du monolithe agent.go (N°88) — même package, contenu inchangé.
package agent

import (
	"mikcloud/hotspot-api/internal/model"
	"strings"
)

// ShieldMarker — commentaire des règles FILTER posées par Shield (N°81 —
// bouclier réseau du WiFi public). Même contrat d'idempotence : seules
// les règles portant ce marqueur sont retirées puis recréées — les
// règles du gérant sont préservées.
const ShieldMarker = "mikcloud-shield"

// N°81 — ports bloqués pour les clients du WiFi public :
//   - administration du routeur (tcp) : ftp, ssh, telnet, winbox, api ;
//   - api en udp (8728/8729 répondent aussi en udp) ;
//   - propagation de malwares (SMB/NetBIOS, tcp et udp) — les vecteurs
//     de mouvement latéral classiques des réseaux partagés.

// N°81 — ports bloqués pour les clients du WiFi public :
//   - administration du routeur (tcp) : ftp, ssh, telnet, winbox, api ;
//   - api en udp (8728/8729 répondent aussi en udp) ;
//   - propagation de malwares (SMB/NetBIOS, tcp et udp) — les vecteurs
//     de mouvement latéral classiques des réseaux partagés.
const (
	ShieldAdminTCPPorts   = "21,22,23,8291,8728,8729"
	ShieldAdminUDPPorts   = "8728,8729"
	ShieldMalwareTCPPorts = "135,137,138,139,445"
	ShieldMalwareUDPPorts = "137,138,139"
)

// ShieldLevelFromPayload — niveau d'une commande shield, normalisé
// ("" ou valeur inconnue → off : le script ne pose alors aucune règle).

// ShieldLevelFromPayload — niveau d'une commande shield, normalisé
// ("" ou valeur inconnue → off : le script ne pose alors aucune règle).
func ShieldLevelFromPayload(p map[string]any) string {
	if s, _ := p["level"].(string); model.ValidShieldLevel(s) {
		return s
	}
	return model.ShieldOff
}

// FamilyGuardMarker — commentaire des règles FILTER posées par FamilyGuard
// (N°82 — couvre-feu internet du WiFi public). Même contrat d'idempotence :
// seules les règles portant ce marqueur sont retirées puis recréées — les
// règles du gérant sont préservées.

// buildShield — N°81 : bouclier réseau du WiFi public (Shield).
//
// Principe : cinq règles FILTER par serveur hotspot du routeur, posées
// en TÊTE de chaîne (place-before=0) et ciblées sur l'INTERFACE du
// hotspot — le réseau du gérant (LAN) et le trafic propre du routeur
// (chain=output : check-in agent, DNS sortant) ne sont JAMAIS touchés :
//
//	input   ×2 : ports d'administration (winbox, ssh, telnet, api, ftp)
//	            inaccessibles DEPUIS le WiFi public ;
//	forward ×3 : connexions invalides, SMB et NetBIOS bloqués pour les
//	            appareils des clients (vecteurs de propagation des
//	            malwares sur réseau partagé) et vers le réseau du gérant.
//
//	L'interface du hotspot est lue SUR le routeur (foreach /ip hotspot
//	find) : le script s'adapte à toute topologie (wlan1,
//	bridge-hotspot…) et suit un renommage d'interface à la prochaine
//	réparation 6 h (vérité routeur, pattern fetchResultData).
//
//	off : retire les règles marquées (retour à l'état antérieur).
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
func (b Builder) buildShield(cmd model.Command) string {
	level := ShieldLevelFromPayload(cmd.Payload)
	okVar := "ok" + idSafe(cmd.ID)
	var sb strings.Builder
	sb.WriteString(header(cmd))
	sb.WriteString(":local " + okVar + " true\n")
	sb.WriteString(":do {\n  /ip firewall filter remove [find comment=\"" + ShieldMarker + "\"]\n} on-error={}\n")
	sb.WriteString(":local shn 0\n")
	if level != model.ShieldOff {
		sb.WriteString(":foreach h in=[/ip hotspot find] do={\n")
		sb.WriteString("  :set shn ($shn + 1)\n")
		sb.WriteString("  :local shi [/ip hotspot get $h interface]\n")
		sb.WriteString("  :do {\n    /ip firewall filter add chain=input place-before=0 in-interface=$shi action=drop protocol=tcp dst-port=" + ShieldAdminTCPPorts +
			" comment=\"" + ShieldMarker + "\"\n  } on-error={ :set " + okVar + " false }\n")
		sb.WriteString("  :do {\n    /ip firewall filter add chain=input place-before=0 in-interface=$shi action=drop protocol=udp dst-port=" + ShieldAdminUDPPorts +
			" comment=\"" + ShieldMarker + "\"\n  } on-error={ :set " + okVar + " false }\n")
		sb.WriteString("  :do {\n    /ip firewall filter add chain=forward place-before=0 in-interface=$shi action=drop connection-state=invalid comment=\"" + ShieldMarker + "\"\n  } on-error={ :set " + okVar + " false }\n")
		sb.WriteString("  :do {\n    /ip firewall filter add chain=forward place-before=0 in-interface=$shi action=drop protocol=tcp dst-port=" + ShieldMalwareTCPPorts +
			" comment=\"" + ShieldMarker + "\"\n  } on-error={ :set " + okVar + " false }\n")
		sb.WriteString("  :do {\n    /ip firewall filter add chain=forward place-before=0 in-interface=$shi action=drop protocol=udp dst-port=" + ShieldMalwareUDPPorts +
			" comment=\"" + ShieldMarker + "\"\n  } on-error={ :set " + okVar + " false }\n")
		sb.WriteString("}\n")
	}
	// Rapport — vérité routeur : le compte de règles marquées présentes
	// après application ET le nombre de serveurs hotspots trouvés (valeurs
	// DYNAMIQUES calculées côté routeur, pattern fetchResultData : le
	// cloud ne croit que ce que le routeur rapporte).
	sb.WriteString(":local shr [:len [/ip firewall filter find comment=\"" + ShieldMarker + "\"]]\n")
	ok := `/tool fetch url="` + strings.TrimRight(b.BaseURL, "/") + `/agent/result?token=` + urlEscape(b.Token) +
		`" http-method=post http-data=("cmd=` + urlEscape(cmd.ID) + `&status=ok&rules=". $shr . "&hs=". $shn) output=none`
	ko := b.reportLine(cmd.ID, false, map[string]string{"message": "echec des regles du bouclier sur le routeur"})
	sb.WriteString(":if ($" + okVar + ") do={\n  " + ok + "\n} else={\n  " + ko + "\n}\n")
	return sb.String()
}

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
