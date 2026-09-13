// AntiVPN (N°88) — génération du script RouterOS bloquant les VPN et tunnels standards depuis le WiFi public.
// Extrait du monolithe agent.go (N°88) — même package, contenu inchangé.
package agent

import (
	"mikcloud/hotspot-api/internal/model"
	"strings"
)

// AntiVpnMarker — commentaire des règles FILTER posées par AntiVPN (N°88 —
// bloque-VPN du WiFi public). Même contrat d'idempotence : seules les
// règles portant ce marqueur sont retirées puis recréées — les règles du
// gérant sont préservées.
const AntiVpnMarker = "mikcloud-antivpn"

// N°88 — canaux des tunnels VPN coupés pour les clients du WiFi public.
// Choix L4 délibéré (MVP honnête — pas de DPI sur un routeur 128 Mo) :
//   - protocoles : GRE (47, tunnels PPTP/data) et ESP (50, IPsec/IKEv2 —
//     le « Ajouter un VPN » natif Android/iOS) ;
//   - UDP : 500/4500 (IKE + NAT-T), 1701 (L2TP), 1194 (OpenVPN),
//     51820 (WireGuard), 2408 (Cloudflare WARP) ;
//   - TCP : 1723 (PPTP), 1194 (OpenVPN), 9001/9030 (Tor ORPort/DirPort).
//
// Ce que le module ne touche JAMAIS : le port 53 (SafeWiFi N°80 reste
// maître du DNS), le NTP (123) et l'UDP 443 (appels WhatsApp — critiques
// en Côte d'Ivoire — et QUIC) : la limite résiduelle (un tunnel camouflé
// en HTTPS pur, ex. certains clients obfusqués) est écrite noir sur
// blanc dans la footnote du module — un MVP ne vend pas de DPI.

// N°88 — canaux des tunnels VPN coupés pour les clients du WiFi public.
// Choix L4 délibéré (MVP honnête — pas de DPI sur un routeur 128 Mo) :
//   - protocoles : GRE (47, tunnels PPTP/data) et ESP (50, IPsec/IKEv2 —
//     le « Ajouter un VPN » natif Android/iOS) ;
//   - UDP : 500/4500 (IKE + NAT-T), 1701 (L2TP), 1194 (OpenVPN),
//     51820 (WireGuard), 2408 (Cloudflare WARP) ;
//   - TCP : 1723 (PPTP), 1194 (OpenVPN), 9001/9030 (Tor ORPort/DirPort).
//
// Ce que le module ne touche JAMAIS : le port 53 (SafeWiFi N°80 reste
// maître du DNS), le NTP (123) et l'UDP 443 (appels WhatsApp — critiques
// en Côte d'Ivoire — et QUIC) : la limite résiduelle (un tunnel camouflé
// en HTTPS pur, ex. certains clients obfusqués) est écrite noir sur
// blanc dans la footnote du module — un MVP ne vend pas de DPI.
const (
	AntiVpnUDPPorts = "500,4500,1701,1194,51820,2408"
	AntiVpnTCPPorts = "1723,1194,9001,9030"
)

// AntiVpnLevelFromPayload — niveau d'une commande antivpn, normalisé
// ("" ou valeur inconnue → off : le script ne pose alors aucune règle).

// AntiVpnLevelFromPayload — niveau d'une commande antivpn, normalisé
// ("" ou valeur inconnue → off : le script ne pose alors aucune règle).
func AntiVpnLevelFromPayload(p map[string]any) string {
	if s, _ := p["level"].(string); model.ValidAntiVpnLevel(s) {
		return s
	}
	return model.AntiVpnOff
}

// AntiVpnRulesPerHotspot — règles FILTER IPv4 posées par serveur hotspot
// quand le bloque-VPN est actif (N°88) : GRE + ESP + ports UDP + ports
// TCP. La vérification du retour compte ces règles — le compte attendu
// dépend donc du nombre de hotspots RAPPORTÉ (pattern Shield N°81 : le
// cloud importe cette constante, source unique).

// AntiVpnRulesPerHotspot — règles FILTER IPv4 posées par serveur hotspot
// quand le bloque-VPN est actif (N°88) : GRE + ESP + ports UDP + ports
// TCP. La vérification du retour compte ces règles — le compte attendu
// dépend donc du nombre de hotspots RAPPORTÉ (pattern Shield N°81 : le
// cloud importe cette constante, source unique).
const AntiVpnRulesPerHotspot = 4

// ---------------------------------------------------------------------------
// Tokens
// ---------------------------------------------------------------------------

// NewToken génère un token d'agent (32 caractères base64url, 192 bits d'entropie).

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
func (b Builder) buildAntiVpn(cmd model.Command) string {
	level := AntiVpnLevelFromPayload(cmd.Payload)
	okVar := "ok" + idSafe(cmd.ID)
	var sb strings.Builder
	sb.WriteString(header(cmd))
	sb.WriteString(":local " + okVar + " true\n")
	sb.WriteString(":do {\n  /ip firewall filter remove [find comment=\"" + AntiVpnMarker + "\"]\n} on-error={}\n")
	sb.WriteString(":do {\n  /ipv6 firewall filter remove [find comment=\"" + AntiVpnMarker + "\"]\n} on-error={}\n")
	sb.WriteString(":local avn 0\n")
	if level != model.AntiVpnOff {
		sb.WriteString(":foreach h in=[/ip hotspot find] do={\n")
		sb.WriteString("  :set avn ($avn + 1)\n")
		sb.WriteString("  :local avi [/ip hotspot get $h interface]\n")
		sb.WriteString("  :do {\n    /ip firewall filter add chain=forward place-before=0 in-interface=$avi action=drop protocol=gre comment=\"" + AntiVpnMarker +
			"\"\n  } on-error={ :set " + okVar + " false }\n")
		sb.WriteString("  :do {\n    /ip firewall filter add chain=forward place-before=0 in-interface=$avi action=drop protocol=50 comment=\"" + AntiVpnMarker +
			"\"\n  } on-error={ :set " + okVar + " false }\n")
		sb.WriteString("  :do {\n    /ip firewall filter add chain=forward place-before=0 in-interface=$avi action=drop protocol=udp dst-port=" + AntiVpnUDPPorts +
			" comment=\"" + AntiVpnMarker + "\"\n  } on-error={ :set " + okVar + " false }\n")
		sb.WriteString("  :do {\n    /ip firewall filter add chain=forward place-before=0 in-interface=$avi action=drop protocol=tcp dst-port=" + AntiVpnTCPPorts +
			" comment=\"" + AntiVpnMarker + "\"\n  } on-error={ :set " + okVar + " false }\n")
		// IPv6 best-effort (pattern N°85) : sans pile IPv6, rien à couper —
		// on-error silencieux, non compté au rapport.
		sb.WriteString("  :do {\n    /ipv6 firewall filter add chain=forward place-before=0 in-interface=$avi action=drop protocol=gre comment=\"" + AntiVpnMarker +
			"\"\n  } on-error={}\n")
		sb.WriteString("  :do {\n    /ipv6 firewall filter add chain=forward place-before=0 in-interface=$avi action=drop protocol=50 comment=\"" + AntiVpnMarker +
			"\"\n  } on-error={}\n")
		sb.WriteString("  :do {\n    /ipv6 firewall filter add chain=forward place-before=0 in-interface=$avi action=drop protocol=udp dst-port=" + AntiVpnUDPPorts +
			" comment=\"" + AntiVpnMarker + "\"\n  } on-error={}\n")
		sb.WriteString("  :do {\n    /ipv6 firewall filter add chain=forward place-before=0 in-interface=$avi action=drop protocol=tcp dst-port=" + AntiVpnTCPPorts +
			" comment=\"" + AntiVpnMarker + "\"\n  } on-error={}\n")
		sb.WriteString("}\n")
	}
	// Rapport — vérité routeur : le compte de règles marquées IPv4
	// présentes après application ET le nombre de serveurs hotspots
	// trouvés (valeurs DYNAMIQUES calculées côté routeur, pattern
	// fetchResultData / Shield N°81 : le cloud ne croit que ce que le
	// routeur rapporte).
	sb.WriteString(":local avr [:len [/ip firewall filter find comment=\"" + AntiVpnMarker + "\"]]\n")
	ok := `/tool fetch url="` + strings.TrimRight(b.BaseURL, "/") + `/agent/result?token=` + urlEscape(b.Token) +
		`" http-method=post http-data=("cmd=` + urlEscape(cmd.ID) + `&status=ok&rules=". $avr . "&hs=". $avn) output=none`
	ko := b.reportLine(cmd.ID, false, map[string]string{"message": "echec du bloque-vpn sur le routeur"})
	sb.WriteString(":if ($" + okVar + ") do={\n  " + ok + "\n} else={\n  " + ko + "\n}\n")
	return sb.String()
}

// buildReadState — v5 (N°76) : télémétrie + rapport PAGINÉ des utilisateurs.
// Motivation : v4 (N°75) bornait le rapport à 500 users et gelait TOUTE
// déduction au-delà (badge « absent du routeur » ni posé ni levé) — les faux
// badges posés avant N°75 sur un parc de 3 478 users (cap v2 : 150) restaient
// prisonniers à vie : le rapport était tronqué en PERMANENCE, la
// réconciliation ne tournait plus jamais. v5 découpe le parc en fenêtres :
//   - total=<nb total d'users sur le routeur> : compteur EXACT rapporté par
//     chaque chunk — le cloud connaît le vrai parc dès le 1er ;
//   - start/count : fenêtre [start, start+count) du parc, inlinés par Go
//     depuis le payload de la commande ; out=<entrées émises> ;
//   - sessions : rapportées UNIQUEMENT par le chunk final (start+count >=
//     total) — les chunks intermédiaires n'alourdissent pas leur POST pour
//     rien ; stotal (total de sessions actives) est rapporté par tous ;
//   - trunc=true|false : il RESTE des chunks (start+count < total) —
//     informatif (le cloud décide sur total/start/count, pas sur le drapeau).
//
// v4 (N°75) : board/freehdd/totalhdd en Mo côté script ; ifaces (8 running,
// compteurs cumulés → débits par diff côté cloud). v2 : le gel honnête des
// déductions sur rapport incomplet reste LA règle (chunk perdu = cycle
// abandonné sans déduction, la complétude est vérifiée avant d'appliquer).
