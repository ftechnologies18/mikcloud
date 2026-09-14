// DevicePause — N°101 — pause dîner : coupe l'internet d'appareils précis
// du foyer (règles FILTER par MAC, marquées mikcloud-pause).
package agent

import (
	"mikcloud/hotspot-api/internal/model"
	"strings"
)

// DevicePauseMarker — commentaire des règles FILTER posées par la pause
// dîner (N°101). Même contrat d'idempotence que FamilyGuard/Shield : seules
// les règles portant ce marqueur sont retirées puis recréées — les règles du
// gérant sont préservées.
const DevicePauseMarker = "mikcloud-pause"

// DevicePauseMacsFromPayload — liste des MAC à couper, portée par la
// commande device_pause (chaîne « MAC;MAC;… », séparateurs « ; »). Le cloud
// la construit à la mise en file : c'est l'ENSEMBLE DÉSIRÉ complet (pas un
// delta) — l'application est remove-then-add, toujours convergente.
func DevicePauseMacsFromPayload(p map[string]any) []string {
	raw, _ := p["macs"].(string)
	out := []string{}
	for _, part := range strings.Split(raw, ";") {
		if mac := model.NormalizeMAC(part); mac != "" {
			out = append(out, mac)
		}
	}
	return out
}

// DevicePauseSigFromPayload — signature de la version envoyée (le cloud la
// compare à la version désirée au retour du rapport : un parent qui change
// d'avis pendant le vol ne doit pas voir figé un état périmé — pattern
// SafeWiFi N°80).
func DevicePauseSigFromPayload(p map[string]any) string {
	sig, _ := p["sig"].(string)
	return sig
}

// buildDevicePause — N°101 : pause dîner (coupe internet d'appareils).
//
// Principe : UNE règle FILTER PAR APPAREIL en pause, chain=forward,
// src-mac-address=<MAC>, action=drop, posée en TÊTE de chaîne
// (place-before=0 — au-dessus d'un éventuel fasttrack des connexions
// établies : les téléchargements EN COURS sont coupés immédiatement, pas
// seulement les nouvelles). Le match par MAC (et non par IP) survit aux
// renouvellements de bail DHCP — l'appareil qui redemande une adresse reste
// coupé ; le trafic PROPRE du routeur (chain=output : check-in agent, DNS)
// et le LAN entre appareils de la maison ne sont jamais touchés, seul
// l'INTERNET de l'appareil visé est coupé.
//
// IPv6 : miroir best-effort (pattern N°85/88) — mêmes règles en /ipv6
// firewall filter, on-error silencieux par règle : un routeur sans pile
// IPv6 n'a ni règle à poser ni échappatoire à fermer ; les règles IPv6 ne
// comptent PAS au rapport (seule la vérité IPv4 signe la convergence).
//
//	macs vide : retire les règles marquées (toutes les pauses levées) ;
//	macs      : les repose — exactement 1 règle IPv4 par MAC.
//
// Idempotent : seules les règles marquées "mikcloud-pause" sont remplacées.
// Le rapport échoe le compte de règles marquées IPv4 présentes APRÈS
// application — le cloud ne pose la signature que si rules == len(macs)
// (pattern FamilyGuard N°82 : vérité routeur, jamais présomption).
//
// 0 Mo de RAM routeur (règles sans état), compatible MIPS 128 Mo.
func (b Builder) buildDevicePause(cmd model.Command) string {
	macs := DevicePauseMacsFromPayload(cmd.Payload)
	okVar := "ok" + idSafe(cmd.ID)
	var sb strings.Builder
	sb.WriteString(header(cmd))
	sb.WriteString(":local " + okVar + " true\n")
	// Retrait idempotent IPv4 + IPv6 (les règles marquées uniquement).
	sb.WriteString(":do {\n  /ip firewall filter remove [find comment=\"" + DevicePauseMarker + "\"]\n} on-error={}\n")
	sb.WriteString(":do {\n  /ipv6 firewall filter remove [find comment=\"" + DevicePauseMarker + "\"]\n} on-error={}\n")
	// Une règle par MAC en pause (IPv4 signe la vérité, IPv6 best-effort).
	for _, mac := range macs {
		sb.WriteString(":do {\n  /ip firewall filter add chain=forward place-before=0 src-mac-address=\"" + rosEscape(mac) +
			"\" action=drop comment=\"" + DevicePauseMarker + "\"\n} on-error={ :set " + okVar + " false }\n")
		sb.WriteString(":do {\n  /ipv6 firewall filter add chain=forward place-before=0 src-mac-address=\"" + rosEscape(mac) +
			"\" action=drop comment=\"" + DevicePauseMarker + "\"\n} on-error={}\n")
	}
	// Rapport — vérité routeur : le compte de règles marquées IPv4 présentes
	// après application (les IPv6 ne comptent pas — best-effort documenté).
	sb.WriteString(":local dpr [:len [/ip firewall filter find comment=\"" + DevicePauseMarker + "\"]]\n")
	ok := `/tool fetch url="` + strings.TrimRight(b.BaseURL, "/") + `/agent/result?token=` + urlEscape(b.Token) +
		`" http-method=post http-data=("cmd=` + urlEscape(cmd.ID) + `&status=ok&rules=". $dpr) output=none`
	ko := b.reportLine(cmd.ID, false, map[string]string{"message": "echec de la pause d'appareils sur le routeur"})
	sb.WriteString(":if ($" + okVar + ") do={\n  " + ok + "\n} else={\n  " + ko + "\n}\n")
	return sb.String()
}
