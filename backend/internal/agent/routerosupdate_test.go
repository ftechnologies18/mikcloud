package agent

// Tests N°115 — mise à jour RouterOS : forme des scripts générés (check +
// install) et assainissement des versions embarquées.

import (
	"strconv"
	"strings"
	"testing"

	"mikcloud/hotspot-api/internal/model"
)

// TestRouterOSCheckScriptShape — la vérification : lancement asynchrone
// (check-for-updates), poll borné avec garde typeof-num (find ne se compare
// JAMAIS nu), lectures isolées (:do on-error — un champ absent sur un build
// exotique ne tue pas la commande), rapport dynamique pattern ping F8.
func TestRouterOSCheckScriptShape(t *testing.T) {
	b := Builder{BaseURL: "https://cloud.example", Token: "tok-ros"}
	script, err := b.ScriptFor(model.Command{ID: "c-ros1", Kind: model.CmdRouterOSCheck})
	if err != nil {
		t.Fatalf("routeros_check : %v", err)
	}
	for _, want := range []string{
		"# mikcloud cmd c-ros1 routeros_check\n",
		"/system package update check-for-updates",
		`:while ($i < ` + strconv.Itoa(RouterOSCheckPollMax) + `) do={`,
		`:local p [:find $rosStat "heck"]`,
		`[:typeof $p] = "num"`,
		`:set rosStat [/system package update get status]`,
		`:set rosLatest [/system package update get latest-version]`,
		`:set rosInst [/system package update get installed-version]`,
		`:set rosChan [/system package update get channel]`,
		// Rapport dynamique : la clé rosStatus (le status RouterOS brut) ne
		// doit JAMAIS s'appeler status (clé du protocole ok/error du rapport).
		`&status=ok&rosStatus=". $rosStat ."&latest=". $rosLatest ."&installed=". $rosInst ."&channel=". $rosChan`,
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("routeros_check : %q absent du script :\n%s", want, script)
		}
	}
	// Chaque lecture périphérique est isolée dans son :do on-error.
	isolated := strings.Count(script, `} on-error={ }`)
	if isolated < 4 {
		t.Fatalf("routeros_check : lectures isolées attendues (≥ 4), trouvées %d :\n%s", isolated, script)
	}
	// Branche d'échec : rapport error explicite.
	if !strings.Contains(script, `status=error`) || !strings.Contains(script, `verification_de_mise_a_jour_impossible_sur_le_routeur`) {
		t.Fatalf("routeros_check : branche d'échec absente :\n%s", script)
	}
}

// TestRouterOSUpdateScriptShape — l'installation : rapport ok AVANT
// l'exécution (pattern reboot F10 — le téléchargement puis le redémarrage
// couperont le routeur, le rapport part le premier), install dans :do
// on-error qui rapporte l'échec APRÈS coup (le routeur ne redémarre pas
// quand le téléchargement échoue).
func TestRouterOSUpdateScriptShape(t *testing.T) {
	b := Builder{BaseURL: "https://cloud.example", Token: "tok-ros"}
	script, err := b.ScriptFor(model.Command{
		ID:      "c-ros2",
		Kind:    model.CmdRouterOSUpdate,
		Payload: map[string]any{"latest": "7.18.3"},
	})
	if err != nil {
		t.Fatalf("routeros_update : %v", err)
	}
	for _, want := range []string{
		"# mikcloud cmd c-ros2 routeros_update\n",
		"action=install",
		"latest=7.18.3",
		"/system package update install",
		`:log warning "mikcloud: mise a jour RouterOS impossible"`,
		"installation_impossible_sur_le_routeur", // message échappé par reportLine → urlEscape (espaces → underscores)
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("routeros_update : %q absent du script :\n%s", want, script)
		}
	}
	// ORDRE — le rapport ok précède l'installation (le fetch est bloquant :
	// le cloud voit « done » avant que le téléchargement ne commence).
	iReport, iInstall := strings.Index(script, "/agent/result?token="), strings.Index(script, "/system package update install")
	if iReport < 0 || iInstall < 0 || iReport > iInstall {
		t.Fatalf("routeros_update : le rapport ok doit précéder l'install (rapport=%d, install=%d) :\n%s", iReport, iInstall, script)
	}
	// Le rapport d'échec, lui, suit l'install (il ne part QUE si le
	// téléchargement échoue — sinon le routeur redémarre et le script meurt).
	iErr := strings.LastIndex(script, "status=error")
	if iErr < iInstall {
		t.Fatalf("routeros_update : le rapport d'échec doit suivre l'install :\n%s", script)
	}
}

// TestRouterOSUpdatePayloadSanitized — une version cible hostile (guillemet,
// saut de ligne) ne traverse jamais le générateur : le rapport de lancement
// embarque la version ASSAINIE (défense en profondeur — le handler valide
// déjà la forme, urlEscape neutralise le reste).
func TestRouterOSUpdatePayloadSanitized(t *testing.T) {
	b := Builder{BaseURL: "https://cloud.example", Token: "tok-ros"}
	script, err := b.ScriptFor(model.Command{
		ID:      "c-ros3",
		Kind:    model.CmdRouterOSUpdate,
		Payload: map[string]any{"latest": `7.18.3"; :log warning "pwned`},
	})
	if err != nil {
		t.Fatalf("routeros_update : %v", err)
	}
	if strings.Contains(script, `pwned`) || strings.Contains(script, `"; `) {
		t.Fatalf("routeros_update : payload hostile non assaini :\n%s", script)
	}
	if !strings.Contains(script, "latest=7.18.3") {
		t.Fatalf("routeros_update : le noyau de version doit rester :\n%s", script)
	}
}

// TestSanitizeRouterOSVersion — noyau numérique préservé, suffixes d'édition
// coupés, junk tronqué à la première impureté, borne 32.
func TestSanitizeRouterOSVersion(t *testing.T) {
	cases := map[string]string{
		"7.18.3":              "7.18.3",
		"7.19beta5":           "7.19beta5",
		"6.49.18 (long-term)": "6.49.18",
		"7.20.1\nINJECTION":   "7.20.1",
		`7.1"; :reboot`:       "7.1",
		"":                    "",
		"   ":                 "",
		"x":                   "",
	}
	for in, want := range cases {
		if got := sanitizeRouterOSVersion(in); got != want {
			t.Fatalf("sanitizeRouterOSVersion(%q) = %q, attendu %q", in, got, want)
		}
	}
}
