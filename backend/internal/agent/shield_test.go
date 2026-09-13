package agent

// Tests N°81 — script Shield (bouclier réseau du WiFi public). Le contrat :
//   - niveau on : retrait idempotent PUIS exactement 5 règles filter par
//     serveur hotspot (2 input + 3 forward), ciblées sur l'interface lue
//     SUR le routeur (foreach /ip hotspot find), posées en tête de chaîne
//     (place-before=0), marquées mikcloud-shield ;
//   - /ip firewall nat et /ip dns ne sont JAMAIS touchés (le gérant garde
//     ses règles NAT ; SafeWiFi N°80 reste maître du port 53) ;
//   - niveau off (ou inconnu) : retrait seul — aucune règle posée ;
//   - le rapport échoe le compte de règles marquées PRÉSENTES après
//     application ET le nombre de serveurs hotspots trouvés (vérité
//     routeur, valeurs dynamiques).

import (
	"strings"
	"testing"

	"mikcloud/hotspot-api/internal/model"
)

// TestShieldScriptOn — le bouclier actif pose exactement 5 règles filter
// par serveur hotspot (admin input ×2, invalid + SMB/NetBIOS forward ×3),
// après le retrait idempotent, sans toucher nat ni dns.
func TestShieldScriptOn(t *testing.T) {
	b := Builder{BaseURL: "https://api.example", Token: "tok"}
	cmd := model.Command{ID: "c-sh", Kind: model.CmdShield, Payload: map[string]any{"level": model.ShieldOn}}
	s := b.buildShield(cmd)

	if !strings.Contains(s, `remove [find comment="`+ShieldMarker+`"]`) {
		t.Error("le retrait idempotent des règles marquées manque")
	}
	if !strings.Contains(s, `:foreach h in=[/ip hotspot find] do={`) {
		t.Error("l'énumération dynamique des serveurs hotspots manque (vérité routeur)")
	}
	if !strings.Contains(s, `:local shi [/ip hotspot get $h interface]`) {
		t.Error("la lecture de l'interface du hotspot manque")
	}
	// 2 règles input (tcp admin + udp api) ciblées sur l'interface hotspot.
	if got := strings.Count(s, `chain=input place-before=0 in-interface=$shi action=drop`); got != 2 {
		t.Errorf("%d règles input, attendu 2", got)
	}
	// 3 règles forward (invalid + tcp malveillants + udp netbios).
	if got := strings.Count(s, `chain=forward place-before=0 in-interface=$shi action=drop`); got != 3 {
		t.Errorf("%d règles forward, attendu 3", got)
	}
	// Ports d'administration : winbox/api/ssh/telnet/ftp bloqués depuis le WiFi.
	for _, want := range []string{"dst-port=" + ShieldAdminTCPPorts, "dst-port=" + ShieldAdminUDPPorts} {
		if !strings.Contains(s, want) {
			t.Errorf("ports d'administration manquants : %s", want)
		}
	}
	// Vecteurs malveillants : SMB + NetBIOS bloqués pour les clients.
	for _, want := range []string{"dst-port=" + ShieldMalwareTCPPorts, "dst-port=" + ShieldMalwareUDPPorts} {
		if !strings.Contains(s, want) {
			t.Errorf("ports malveillants manquants : %s", want)
		}
	}
	if !strings.Contains(s, "connection-state=invalid") {
		t.Error("la drop des connexions invalides manque")
	}
	// Les tables du gérant ne sont pas touchées : ni nat ni dns.
	if strings.Contains(s, "/ip firewall nat") || strings.Contains(s, "/ip dns") {
		t.Error("le bouclier ne doit toucher NI la table nat NI /ip dns (SafeWiFi N°80 reste maître du port 53)")
	}
	// Rapport dynamique : règles présentes + hotspots trouvés.
	if !strings.Contains(s, `&rules=". $shr . "&hs=". $shn`) {
		t.Error("le rapport doit échoer rules ET hs DYNAMIQUES (vérité routeur)")
	}
}

// TestShieldScriptOff — le niveau off (et toute valeur inconnue) ne RETIRE
// que les règles marquées : aucune règle posée, retour à l'état antérieur.
func TestShieldScriptOff(t *testing.T) {
	b := Builder{BaseURL: "https://api.example", Token: "tok"}
	for _, level := range []string{model.ShieldOff, "", "niveau-inconnu"} {
		cmd := model.Command{ID: "c-sh", Kind: model.CmdShield, Payload: map[string]any{"level": level}}
		s := b.buildShield(cmd)
		if strings.Contains(s, "firewall filter add") {
			t.Errorf("niveau %q : off ne doit poser AUCUNE règle", level)
		}
		if !strings.Contains(s, `remove [find comment="`+ShieldMarker+`"]`) {
			t.Errorf("niveau %q : le retrait des règles marquées manque", level)
		}
		// Le rapport reste dynamique (rules=0 attendu côté routeur).
		if !strings.Contains(s, `&rules=". $shr . "&hs=". $shn`) {
			t.Errorf("niveau %q : le rapport doit rester dynamique", level)
		}
	}
}

// TestShieldLevelFromPayload — normalisation du niveau (repli prudent :
// toute valeur inconnue retombe sur off — le script ne pose alors rien).
func TestShieldLevelFromPayload(t *testing.T) {
	if got := ShieldLevelFromPayload(map[string]any{"level": model.ShieldOn}); got != model.ShieldOn {
		t.Errorf("on → %q, attendu %q", got, model.ShieldOn)
	}
	if got := ShieldLevelFromPayload(map[string]any{"level": model.ShieldOff}); got != model.ShieldOff {
		t.Errorf("off → %q, attendu %q", got, model.ShieldOff)
	}
	if got := ShieldLevelFromPayload(map[string]any{"level": "n'importe-quoi"}); got != model.ShieldOff {
		t.Errorf("inconnu → %q, attendu %q", got, model.ShieldOff)
	}
	if got := ShieldLevelFromPayload(nil); got != model.ShieldOff {
		t.Errorf("payload nil → %q, attendu %q", got, model.ShieldOff)
	}
}
