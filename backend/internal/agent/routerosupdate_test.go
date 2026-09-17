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
		// N°132 — régression N°125 : la continuation firmware doit OUVRIR une
		// nouvelle chaîne (guillemet AVANT &fwCurrent). Sans lui, `.$fwCurrent=`
		// est une erreur de syntaxe RouterOS qui avortait l'import du script :
		// check muet à jamais, commande « sent » zombie reprise en boucle.
		`."&fwCurrent=". $fwCur ."&fwStaged=". $fwStg ."&fwAuto=". $fwAuto) output=none`,
		// N°132 — auto-upgrade vit sous /system routerboard settings (le
		// chemin N°125 échouait en silence sur le vrai matériel).
		`:set fwAuto [/system routerboard settings get auto-upgrade]`,
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("routeros_check : %q absent du script :\n%s", want, script)
		}
	}
	// Chaque lecture périphérique est isolée dans son :do on-error
	// (N°125 : 4 champs paquet + 3 champs firmware = 7).
	isolated := strings.Count(script, `} on-error={ }`)
	if isolated < 7 {
		t.Fatalf("routeros_check : lectures isolées attendues (≥ 7), trouvées %d :\n%s", isolated, script)
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
	// N°125 — ORDRE : auto-upgrade posé AVANT l'install (le même redémarrage
	// applique RouterOS ET firmware).
	iAuto := strings.Index(script, "/system routerboard settings set auto-upgrade=yes")
	if iAuto < 0 || iAuto > iInstall {
		t.Fatalf("routeros_update : auto-upgrade doit précéder l'install (auto=%d, install=%d) :\n%s", iAuto, iInstall, script)
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

// TestRouterboardFirmwareScriptShape — N°125 — l'appliquage du firmware :
// lectures current/upgrade-firmware isolées, garde anti-redémarrage-inutile
// (rien à appliquer → ok SANS reboot), auto-upgrade + staging dans :do
// on-error, rapport ok AVANT /system reboot (pattern F10), branche d'échec.
func TestRouterboardFirmwareScriptShape(t *testing.T) {
	b := Builder{BaseURL: "https://cloud.example", Token: "tok-fw"}
	script, err := b.ScriptFor(model.Command{ID: "c-fw1", Kind: model.CmdRouterboardFirmware})
	if err != nil {
		t.Fatalf("routerboard_firmware : %v", err)
	}
	for _, want := range []string{
		"# mikcloud cmd c-fw1 routerboard_firmware\n",
		`:do { :set fwCur [/system routerboard get current-firmware] } on-error={ }`,
		`:do { :set fwStg [/system routerboard get upgrade-firmware] } on-error={ }`,
		// Garde : le reboot n'a lieu QUE si un firmware attend réellement.
		`:if ($fwStg != "" && $fwStg != $fwCur) do={ :set fwNeed true }`,
		"/system routerboard settings set auto-upgrade=yes",
		"/system routerboard upgrade",
		"/system reboot",
		// Rapport dynamique : fwCurrent/fwStaged/applied (garde côté routeur).
		`&status=ok&action=firmware&fwCurrent=". $fwCur ."&fwStaged=". $fwStg ."&applied=". $fwNeed`,
		// Branche d'échec : staging impossible → rapport error explicite.
		"firmware_routeurboard_impossible_sur_le_routeur",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("routerboard_firmware : %q absent du script :\n%s", want, script)
		}
	}
	// ORDRE — le rapport ok précède le reboot (le fetch est bloquant : le
	// cloud voit « done » avant que le routeur ne coupe).
	iReport, iReboot := strings.Index(script, "/agent/result?token="), strings.Index(script, "/system reboot")
	if iReport < 0 || iReboot < 0 || iReport > iReboot {
		t.Fatalf("routerboard_firmware : le rapport ok doit précéder le reboot (rapport=%d, reboot=%d) :\n%s", iReport, iReboot, script)
	}
	// Le reboot est SOUS-GARDE : il vit dans la branche fwNeed du rapport ok —
	// le staging (auto-upgrade + upgrade) précède le rapport, qui précède le
	// reboot. Un firmware absent (fwStg vide) ne déclenche JAMAIS le reboot.
	iGuard := strings.Index(script, `:if ($fwStg != "" && $fwStg != $fwCur)`)
	iStage := strings.Index(script, "/system routerboard upgrade")
	if iGuard < 0 || iStage < iGuard || iReboot < iStage {
		t.Fatalf("routerboard_firmware : ordre garde → staging → reboot attendu (garde=%d, staging=%d, reboot=%d) :\n%s", iGuard, iStage, iReboot, script)
	}
	// Lectures isolées (un CHR sans /system routerboard ne tue pas la commande).
	if isolated := strings.Count(script, `} on-error={ }`); isolated < 2 {
		t.Fatalf("routerboard_firmware : lectures isolées attendues (≥ 2), trouvées %d :\n%s", isolated, script)
	}
}

// TestRouterOSScriptsQuoteParity — N°132 — garde anti-régression du bug
// N°125 : chaque ligne http-data= des scripts de mise à jour doit porter un
// nombre PAIR de guillemets. La concaténation dynamique (« &cle=". $var . »)
// exige que CHAQUE fragment ouvre ET ferme sa chaîne : un guillemet ouvrant
// manquant (le `.&fwCurrent=` de N°125) est une erreur de syntaxe RouterOS
// qui avorte l'import du fichier — la commande restait « sent » sans
// rapport à jamais (reprise zombie 10 min en boucle), la vérification
// n'aboutissait plus et la mise à jour de flotte perdait ses cibles.
func TestRouterOSScriptsQuoteParity(t *testing.T) {
	b := Builder{BaseURL: "https://cloud.example", Token: "tok-parity"}
	for _, cmd := range []model.Command{
		{ID: "c-q1", Kind: model.CmdRouterOSCheck},
		{ID: "c-q2", Kind: model.CmdRouterOSUpdate, Payload: map[string]any{"latest": "7.24.4"}},
		{ID: "c-q3", Kind: model.CmdRouterboardFirmware},
	} {
		script, err := b.ScriptFor(cmd)
		if err != nil {
			t.Fatalf("%s : %v", cmd.Kind, err)
		}
		for i, line := range strings.Split(script, "\n") {
			if !strings.Contains(line, "http-data=") {
				continue
			}
			if n := strings.Count(line, `"`); n%2 != 0 {
				t.Fatalf("%s ligne %d : %d guillemets (IMPAIR) — concaténation dynamique cassée :\n%s", cmd.Kind, i+1, n, line)
			}
		}
	}
}
