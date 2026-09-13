package agent

// Tests N°82 — script FamilyGuard (couvre-feu internet du WiFi public).
// Le contrat :
//   - active : retrait idempotent PUIS exactement 1 règle filter par
//     serveur hotspot (chain=forward, reject immédiat), ciblée sur
//     l'interface lue SUR le routeur (foreach /ip hotspot find), posée en
//     tête de chaîne (place-before=0, au-dessus d'un éventuel fasttrack),
//     marquée mikcloud-familyguard ;
//   - /ip firewall nat et /ip dns ne sont JAMAIS touchés (SafeWiFi N°80
//     reste maître du port 53, le portail captif reste servi) ;
//   - active=false (hors fenêtre, désactivé ou payload inconnu) : retrait
//     seul — aucune règle posée ;
//   - le rapport échoe le compte de règles marquées PRÉSENTES après
//     application ET le nombre de serveurs hotspots trouvés (vérité
//     routeur, valeurs dynamiques).

import (
	"strings"
	"testing"

	"mikcloud/hotspot-api/internal/model"
)

// TestFamilyGuardScriptActive — le couvre-feu en cours pose exactement 1
// règle filter par serveur hotspot, après le retrait idempotent, sans
// toucher nat ni dns.
func TestFamilyGuardScriptActive(t *testing.T) {
	b := Builder{BaseURL: "https://api.example", Token: "tok"}
	cmd := model.Command{ID: "c-fg", Kind: model.CmdFamilyGuard, Payload: map[string]any{"active": true, "spec": "1|22:00|06:00|1111111"}}
	s := b.buildFamilyGuard(cmd)

	if !strings.Contains(s, `remove [find comment="`+FamilyGuardMarker+`"]`) {
		t.Error("le retrait idempotent des règles marquées manque")
	}
	if !strings.Contains(s, `:foreach fgh in=[/ip hotspot find] do={`) {
		t.Error("l'énumération dynamique des serveurs hotspots manque (vérité routeur)")
	}
	if !strings.Contains(s, `:local fgi [/ip hotspot get $fgh interface]`) {
		t.Error("la lecture de l'interface du hotspot manque")
	}
	if got := strings.Count(s, `chain=forward place-before=0 in-interface=$fgi action=reject reject-with=icmp-network-unreachable`); got != 1 {
		t.Errorf("%d règle forward reject, attendu 1 (par serveur hotspot)", got)
	}
	if !strings.Contains(s, `comment="`+FamilyGuardMarker+`"`) {
		t.Error("le marqueur mikcloud-familyguard manque")
	}
	// L'échec est IMMÉDIAT côté appareil (reject), pas un drop silencieux.
	if !strings.Contains(s, "reject-with=icmp-network-unreachable") {
		t.Error("le reject avec réponse ICMP manque")
	}
	// Les tables du gérant ne sont pas touchées : ni nat ni dns.
	if strings.Contains(s, "/ip firewall nat") || strings.Contains(s, "/ip dns") {
		t.Error("le couvre-feu ne doit toucher NI la table nat NI /ip dns (SafeWiFi N°80 reste maître du port 53)")
	}
	// Rapport dynamique : règles présentes + hotspots trouvés.
	if !strings.Contains(s, `&rules=". $fgr . "&hs=". $fgn`) {
		t.Error("le rapport doit échoer rules ET hs DYNAMIQUES (vérité routeur)")
	}
	// Le script ne consulte JAMAIS l'horloge routeur (arbitrage N°82 :
	// le cloud calcule l'état, en UTC == heure d'Abidjan).
	if strings.Contains(s, "/system clock") || strings.Contains(s, "time=") {
		t.Error("le script ne doit pas consulter l'horloge routeur (le cloud est la référence horaire)")
	}
}

// TestFamilyGuardScriptInactive — hors fenêtre (active=false, payload
// absent ou inconnu) le script ne RETIRE que les règles marquées : aucune
// règle posée, retour à l'état antérieur.
func TestFamilyGuardScriptInactive(t *testing.T) {
	b := Builder{BaseURL: "https://api.example", Token: "tok"}
	payloads := []map[string]any{
		{"active": false, "spec": "1|22:00|06:00|1111111"},
		{"spec": "1|22:00|06:00|1111111"}, // active absent → repli prudent false
		nil,
	}
	for _, p := range payloads {
		cmd := model.Command{ID: "c-fg", Kind: model.CmdFamilyGuard, Payload: p}
		s := b.buildFamilyGuard(cmd)
		if strings.Contains(s, "firewall filter add") {
			t.Errorf("payload %v : hors fenêtre ne doit poser AUCUNE règle", p)
		}
		if !strings.Contains(s, `remove [find comment="`+FamilyGuardMarker+`"]`) {
			t.Errorf("payload %v : le retrait des règles marquées manque", p)
		}
		if !strings.Contains(s, `&rules=". $fgr . "&hs=". $fgn`) {
			t.Errorf("payload %v : le rapport doit rester dynamique", p)
		}
	}
}

// TestFamilyGuardActiveFromPayload — normalisation de l'état (repli
// prudent : toute valeur absente ou non booléenne retombe sur false —
// le script ne pose alors rien).
func TestFamilyGuardActiveFromPayload(t *testing.T) {
	if !FamilyGuardActiveFromPayload(map[string]any{"active": true}) {
		t.Error("active=true → doit être vrai")
	}
	if FamilyGuardActiveFromPayload(map[string]any{"active": false}) {
		t.Error("active=false → doit être faux")
	}
	if FamilyGuardActiveFromPayload(map[string]any{"active": "true"}) {
		t.Error("active=\"true\" (string) → repli prudent faux")
	}
	if FamilyGuardActiveFromPayload(nil) {
		t.Error("payload nil → repli prudent faux")
	}
}
