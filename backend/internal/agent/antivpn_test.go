package agent

// Tests N°88 — script AntiVPN (bloque-VPN du WiFi public). Le contrat :
//   - niveau on : retrait idempotent IPv4 PUIS exactement 4 règles filter
//     par serveur hotspot (GRE, ESP, ports UDP, ports TCP), ciblées sur
//     l'interface lue SUR le routeur (foreach /ip hotspot find), posées
//     en tête de chaîne (place-before=0), marquées mikcloud-antivpn ;
//     miroir IPv6 best-effort (on-error silencieux, non compté) ;
//   - le port 53 (DNS — SafeWiFi N°80 reste maître), le NTP (123) et
//     l'UDP 443 (appels WhatsApp, QUIC) ne sont JAMAIS touchés ;
//   - niveau off (ou inconnu) : retrait seul — aucune règle posée ;
//   - le rapport échoe le compte de règles marquées IPv4 PRÉSENTES après
//     application ET le nombre de serveurs hotspots trouvés (vérité
//     routeur, valeurs dynamiques).

import (
	"strings"
	"testing"

	"mikcloud/hotspot-api/internal/model"
)

// TestAntiVpnScriptOn — le bloque-VPN actif pose exactement 4 règles
// filter IPv4 par serveur hotspot (GRE + ESP + ports UDP + ports TCP)
// après le retrait idempotent, plus le miroir IPv6 best-effort.
func TestAntiVpnScriptOn(t *testing.T) {
	b := Builder{BaseURL: "https://api.example", Token: "tok"}
	cmd := model.Command{ID: "c-av", Kind: model.CmdAntiVpn, Payload: map[string]any{"level": model.AntiVpnOn}}
	s := b.buildAntiVpn(cmd)

	if !strings.Contains(s, `remove [find comment="`+AntiVpnMarker+`"]`) {
		t.Error("le retrait idempotent des règles marquées manque")
	}
	if !strings.Contains(s, `:foreach h in=[/ip hotspot find] do={`) {
		t.Error("l'énumération dynamique des serveurs hotspots manque (vérité routeur)")
	}
	if !strings.Contains(s, `:local avi [/ip hotspot get $h interface]`) {
		t.Error("la lecture de l'interface du hotspot manque")
	}
	// 4 règles forward IPv4 : GRE, ESP (50), ports UDP, ports TCP.
	if got := strings.Count(s, `/ip firewall filter add chain=forward place-before=0 in-interface=$avi action=drop`); got != 4 {
		t.Errorf("%d règles forward IPv4, attendu 4", got)
	}
	// GRE et ESP posés en IPv4 ET en IPv6 (miroir best-effort).
	if got := strings.Count(s, `action=drop protocol=gre comment="`+AntiVpnMarker+`"`); got != 2 {
		t.Errorf("%d règles GRE (v4+v6), attendu 2", got)
	}
	if got := strings.Count(s, `action=drop protocol=50 comment="`+AntiVpnMarker+`"`); got != 2 {
		t.Errorf("%d règles ESP (v4+v6), attendu 2", got)
	}
	for _, want := range []string{"dst-port=" + AntiVpnUDPPorts, "dst-port=" + AntiVpnTCPPorts} {
		if !strings.Contains(s, want) {
			t.Errorf("ports de tunnels manquants : %s", want)
		}
	}
	// Miroir IPv6 best-effort : 4 règles /ipv6, échec silencieux.
	if got := strings.Count(s, `/ipv6 firewall filter add chain=forward place-before=0 in-interface=$avi`); got != 4 {
		t.Errorf("%d règles forward IPv6 best-effort, attendu 4", got)
	}
	// Jamais touchés : DNS (53 — SafeWiFi N°80 maître), NTP, UDP 443
	// (appels WhatsApp/QUIC) — au niveau des constantes ET du script.
	if strings.Contains(AntiVpnUDPPorts, "443") || strings.Contains(AntiVpnTCPPorts, "443") {
		t.Error("l'UDP/TCP 443 ne doit jamais faire partie des listes de ports (appels WhatsApp, QUIC)")
	}
	if strings.Contains(s, "dst-port=53") {
		t.Error("le port 53 ne doit jamais être touché (SafeWiFi N°80 reste maître du DNS)")
	}
	if strings.Contains(s, "dst-port=123") {
		t.Error("le NTP (123) ne doit jamais être touché")
	}
	// Les tables du gérant ne sont pas touchées : ni nat ni dns.
	if strings.Contains(s, "/ip firewall nat") || strings.Contains(s, "/ip dns") {
		t.Error("le bloque-VPN ne doit toucher NI la table nat NI /ip dns (SafeWiFi N°80 reste maître du port 53)")
	}
	// Rapport dynamique : règles présentes + hotspots trouvés.
	if !strings.Contains(s, `&rules=". $avr . "&hs=". $avn`) {
		t.Error("le rapport doit échoer rules ET hs DYNAMIQUES (vérité routeur)")
	}
}

// TestAntiVpnScriptOff — le niveau off (et toute valeur inconnue) ne
// RETIRE que les règles marquées : aucune règle posée, retour à l'état
// antérieur.
func TestAntiVpnScriptOff(t *testing.T) {
	b := Builder{BaseURL: "https://api.example", Token: "tok"}
	for _, level := range []string{model.AntiVpnOff, "", "niveau-inconnu"} {
		cmd := model.Command{ID: "c-av", Kind: model.CmdAntiVpn, Payload: map[string]any{"level": level}}
		s := b.buildAntiVpn(cmd)
		if strings.Contains(s, "firewall filter add") {
			t.Errorf("niveau %q : off ne doit poser AUCUNE règle", level)
		}
		if !strings.Contains(s, `remove [find comment="`+AntiVpnMarker+`"]`) {
			t.Errorf("niveau %q : le retrait des règles marquées manque", level)
		}
		// Le rapport reste dynamique (rules=0 attendu côté routeur).
		if !strings.Contains(s, `&rules=". $avr . "&hs=". $avn`) {
			t.Errorf("niveau %q : le rapport doit rester dynamique", level)
		}
	}
}

// TestAntiVpnLevelFromPayload — normalisation du niveau (repli prudent :
// toute valeur inconnue retombe sur off — le script ne pose alors rien).
func TestAntiVpnLevelFromPayload(t *testing.T) {
	if got := AntiVpnLevelFromPayload(map[string]any{"level": model.AntiVpnOn}); got != model.AntiVpnOn {
		t.Errorf("on → %q, attendu %q", got, model.AntiVpnOn)
	}
	if got := AntiVpnLevelFromPayload(map[string]any{"level": model.AntiVpnOff}); got != model.AntiVpnOff {
		t.Errorf("off → %q, attendu %q", got, model.AntiVpnOff)
	}
	if got := AntiVpnLevelFromPayload(map[string]any{"level": "n'importe-quoi"}); got != model.AntiVpnOff {
		t.Errorf("inconnu → %q, attendu %q", got, model.AntiVpnOff)
	}
	if got := AntiVpnLevelFromPayload(nil); got != model.AntiVpnOff {
		t.Errorf("payload nil → %q, attendu %q", got, model.AntiVpnOff)
	}
}

// TestAntiVpnRulesPerHotspotMirror — la constante du compte (source
// unique importée par le cloud pour la vérification du retour) vaut
// exactement le nombre de règles IPv4 posées par hotspot dans le script.
func TestAntiVpnRulesPerHotspotMirror(t *testing.T) {
	if AntiVpnRulesPerHotspot != 4 {
		t.Errorf("AntiVpnRulesPerHotspot = %d, attendu 4 (GRE + ESP + UDP + TCP)", AntiVpnRulesPerHotspot)
	}
	b := Builder{BaseURL: "https://api.example", Token: "tok"}
	cmd := model.Command{ID: "c-av", Kind: model.CmdAntiVpn, Payload: map[string]any{"level": model.AntiVpnOn}}
	s := b.buildAntiVpn(cmd)
	if got := strings.Count(s, `/ip firewall filter add chain=forward place-before=0 in-interface=$avi action=drop`); got != AntiVpnRulesPerHotspot {
		t.Errorf("script = %d règles IPv4 par hotspot, constante = %d : le miroir est cassé", got, AntiVpnRulesPerHotspot)
	}
}
