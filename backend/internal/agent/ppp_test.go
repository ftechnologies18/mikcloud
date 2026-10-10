// Tests N°293 — PPPoE Phase A : validations + forme des scripts .rsc.
// La forme est CONTRACTUELLE (le routeur exécute ce qui est généré) : chaque
// assertion est un incident évité (injection, guillemets, valeurs non
// échappées, marqueur mikcloud-ppp, idempotence repair, pagination).
package agent

import (
	"strconv"
	"strings"
	"testing"

	"mikcloud/hotspot-api/internal/model"
)

func TestValidPppName(t *testing.T) {
	good := []string{
		"abdou", "abonne-01", "abdou@fai.ci", "famille.diallo@orange.ci",
		"pop2_abonne_3", "a", strings.Repeat("a", 64), "ab-._@", "1234",
	}
	for _, s := range good {
		if !model.ValidPppName(s) {
			t.Errorf("ValidPppName(%q) = false, attendu true", s)
		}
	}
	bad := []string{
		"",                      // vide
		strings.Repeat("a", 65), // trop long
		"Abdou@fai.ci",          // majuscule (normaliser d'abord — NormalizePppName)
		"deux mots",             // espace
		`guillemet"`,            // injection
		`back\slash`,            // injection
		"dollar$u",              // expression RouterOS
		"pipe|semi;colon",       // séparateurs du rapport de parité
		"accentué",              // non-ASCII
		" newline\n",            // contrôle
	}
	for _, s := range bad {
		if model.ValidPppName(s) {
			t.Errorf("ValidPppName(%q) = true, attendu false", s)
		}
	}
}

func TestNormalizePppName(t *testing.T) {
	if got := model.NormalizePppName("  Abdou@FAI.CI "); got != "abdou@fai.ci" {
		t.Errorf("NormalizePppName = %q, attendu %q", got, "abdou@fai.ci")
	}
	if !model.ValidPppName(model.NormalizePppName("Abdou@FAI.CI")) {
		t.Error("le nom normalisé doit passer la validation")
	}
}

func TestValidPppProfileName(t *testing.T) {
	good := []string{"default", "ABONNE-10M", "forfait.fibre_2", "a", strings.Repeat("x", 64)}
	for _, s := range good {
		if !model.ValidPppProfileName(s) {
			t.Errorf("ValidPppProfileName(%q) = false, attendu true", s)
		}
	}
	bad := []string{"", strings.Repeat("x", 65), "10 MB", `inj"ect`, `back\slash`, "$expr", "pipe|semi"}
	for _, s := range bad {
		if model.ValidPppProfileName(s) {
			t.Errorf("ValidPppProfileName(%q) = true, attendu false", s)
		}
	}
}

// bPPPOK — builder de test standard.
func bPPPOK() Builder { return Builder{BaseURL: "https://api.mikcloud.ftci.fr", Token: "tok"} }

func TestScriptForPppKinds(t *testing.T) {
	b := bPPPOK()
	kinds := []string{
		model.CmdPppReadSecrets, model.CmdPppReadActive, model.CmdPppSecretAdd,
		model.CmdPppSecretSet, model.CmdPppSecretRemove, model.CmdPppKick,
	}
	for _, kind := range kinds {
		cmd := model.Command{ID: "cmd-ab1", Kind: kind, Payload: map[string]any{
			"name": "abdou@fai.ci", "password": "pw", "profile": "default",
		}}
		s, err := b.ScriptFor(cmd)
		if err != nil {
			t.Fatalf("ScriptFor(%s) : %v", kind, err)
		}
		if !strings.HasPrefix(s, "# mikcloud cmd cmd-ab1 "+kind) {
			t.Errorf("header manquant pour %s", kind)
		}
		if !strings.Contains(s, "/agent/result?token=tok") {
			t.Errorf("rapport result manquant pour %s", kind)
		}
	}
}

func TestBuildPppSecretAddShape(t *testing.T) {
	b := bPPPOK()
	cmd := model.Command{ID: "c-add1", Kind: model.CmdPppSecretAdd, Payload: PppSecretAddPayloadFrom(
		"abdou@fai.ci", "s3cr3t!", "ABONNE-10M", "Abonné Diallo", "", false, false)}
	s := b.buildPppSecretAdd(cmd)
	for _, want := range []string{
		`/ppp/secret/add name="abdou@fai.ci"`,
		`password="s3cr3t!"`,
		`profile="ABONNE-10M"`,
		"service=pppoe",
		`comment="mikcloud-ppp Abonné Diallo"`,
		"okcadd1", // variable locale unique (ok + idSafe)
	} {
		if !strings.Contains(s, want) {
			t.Errorf("buildPppSecretAdd : %q absent", want)
		}
	}
	if strings.Contains(s, "disabled=yes") {
		t.Error("buildPppSecretAdd : disabled=yes ne doit pas apparaître pour disabled=false")
	}
	// disabled → disabled=yes
	cmd2 := model.Command{ID: "c-add2", Kind: model.CmdPppSecretAdd, Payload: PppSecretAddPayloadFrom(
		"abdou@fai.ci", "", "default", "", "", true, false)}
	s2 := b.buildPppSecretAdd(cmd2)
	if !strings.Contains(s2, "disabled=yes") {
		t.Error("buildPppSecretAdd : disabled=yes attendu pour un secret suspendu")
	}
	if strings.Contains(s2, `password="`) {
		t.Error("buildPppSecretAdd : pas de password= quand le mot de passe est vide")
	}
}

func TestBuildPppSecretAddRepairIdempotent(t *testing.T) {
	b := bPPPOK()
	// repair → garde [:len [find name=…]] = 0 : présent = ok sans retouche.
	cmd := model.Command{ID: "c-rep1", Kind: model.CmdPppSecretAdd, Payload: PppSecretAddPayloadFrom(
		"abdou@fai.ci", "pw", "default", "", "", false, true)}
	s := b.buildPppSecretAdd(cmd)
	for _, want := range []string{
		`[:len [/ppp/secret find name="abdou@fai.ci"]] = 0`,
		"/ppp/secret/add name=\"abdou@fai.ci\"",
		"mikcloud-ppp",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("buildPppSecretAdd repair : %q absent", want)
		}
	}
	// Sans repair : add direct (pas de garde de présence — l'unicité est
	// déjà garantie côté cloud, un doublon doit être rapporté en erreur).
	cmd2 := model.Command{ID: "c-rep2", Kind: model.CmdPppSecretAdd, Payload: PppSecretAddPayloadFrom(
		"abdou@fai.ci", "pw", "default", "", "", false, false)}
	s2 := b.buildPppSecretAdd(cmd2)
	if strings.Contains(s2, "[:len [/ppp/secret find") {
		t.Error("buildPppSecretAdd sans repair : la garde de présence ne doit pas être émise")
	}
}

func TestBuildPppSecretSetSelective(t *testing.T) {
	b := bPPPOK()
	// Set SÉLECTIF : seules les propriétés pointées sont envoyées.
	pw := "n3wpass"
	dis := true
	cmd := model.Command{ID: "c-set1", Kind: model.CmdPppSecretSet, Payload: PppSecretSetPayloadFrom(
		&pw, nil, nil, nil, &dis)}
	s := b.buildPppSecretSet(cmd)
	for _, want := range []string{
		`/ppp/secret/set [find name=""]`, // pas de nom fourni : find générique (le cloud envoie toujours name)
		`password="n3wpass"`,
		"disabled=yes",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("buildPppSecretSet : %q absent", want)
		}
	}
	if strings.Contains(s, "profile=") || strings.Contains(s, "comment=") || strings.Contains(s, "disabled=no") {
		t.Error("buildPppSecretSet : des propriétés non pointées ont été envoyées")
	}
	// Avec nom + disabled=false explicite → disabled=no (réactivation).
	cmd2 := model.Command{ID: "c-set2", Kind: model.CmdPppSecretSet,
		Payload: map[string]any{"name": "abdou@fai.ci", "disabled": false}}
	s2 := b.buildPppSecretSet(cmd2)
	if !strings.Contains(s2, `/ppp/secret/set [find name="abdou@fai.ci"]`) {
		t.Errorf("buildPppSecretSet : find name attendu, script %q", s2)
	}
	if !strings.Contains(s2, "disabled=no") {
		t.Error("buildPppSecretSet : disabled=no attendu pour réactivation")
	}
	// Commentaire présent → préfixe marqueur mikcloud-ppp.
	cm := "Abonné réactivé"
	cmd3 := model.Command{ID: "c-set3", Kind: model.CmdPppSecretSet, Payload: PppSecretSetPayloadFrom(
		nil, nil, &cm, nil, nil)}
	s3 := b.buildPppSecretSet(cmd3)
	if !strings.Contains(s3, `comment="mikcloud-ppp Abonné réactivé"`) {
		t.Errorf("buildPppSecretSet : marqueur mikcloud-ppp attendu sur le commentaire, script %q", s3)
	}
}

func TestBuildPppSecretRemoveShape(t *testing.T) {
	b := bPPPOK()
	cmd := model.Command{ID: "c-rm1", Kind: model.CmdPppSecretRemove,
		Payload: map[string]any{"name": "abdou@fai.ci"}}
	s := b.buildPppSecretRemove(cmd)
	if !strings.Contains(s, `/ppp/secret/remove [find name="abdou@fai.ci"]`) {
		t.Errorf("buildPppSecretRemove : ligne remove attendue, script %q", s)
	}
}

func TestBuildPppKickShape(t *testing.T) {
	b := bPPPOK()
	cmd := model.Command{ID: "c-k1", Kind: model.CmdPppKick,
		Payload: map[string]any{"name": "abdou@fai.ci"}}
	s := b.buildPppKick(cmd)
	if !strings.Contains(s, `/ppp/active/remove [find name="abdou@fai.ci"]`) {
		t.Errorf("buildPppKick : ligne kick attendue, script %q", s)
	}
	// Le kick n'est PAS une suppression de secret.
	if strings.Contains(s, "/ppp/secret/remove") {
		t.Error("buildPppKick : le kick ne doit jamais toucher /ppp/secret")
	}
}

func TestBuildPppReadSecretsPagination(t *testing.T) {
	b := bPPPOK()
	cmd := model.Command{ID: "c-rs1", Kind: model.CmdPppReadSecrets,
		Payload: map[string]any{"start": 500, "count": 500}}
	s := b.buildPppReadSecrets(cmd)
	for _, want := range []string{
		"/ppp/secret find",
		"/ppp/secret get $s name",
		"/ppp/secret get $s profile",
		"/ppp/secret get $s disabled",
		"/ppp/secret get $s service",
		"/ppp/secret get $s comment",
		"&total=", // total exact rapporté
		"&start=500&count=500",
		"&data=",
		"&trunc=",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("buildPppReadSecrets : %q absent", want)
		}
	}
	// Placeholders tous substitués.
	if strings.Contains(s, "@@") {
		t.Error("buildPppReadSecrets : placeholders @@ restants")
	}
	// Count borné à ReadChunkSize même sur payload absurdé.
	cmd2 := model.Command{ID: "c-rs2", Kind: model.CmdPppReadSecrets,
		Payload: map[string]any{"start": 0, "count": 99999}}
	s2 := b.buildPppReadSecrets(cmd2)
	if !strings.Contains(s2, "count="+strconv.Itoa(ReadChunkSize)) {
		t.Errorf("buildPppReadSecrets : count doit être borné à %d", ReadChunkSize)
	}
}

func TestBuildPppReadActiveShape(t *testing.T) {
	b := bPPPOK()
	cmd := model.Command{ID: "c-ra1", Kind: model.CmdPppReadActive}
	s := b.buildPppReadActive(cmd)
	for _, want := range []string{
		"/ppp/active find",
		"/ppp/active get $a name",
		"/ppp/active get $a service",
		"/ppp/active get $a caller-id",
		"/ppp/active get $a address",
		"/ppp/active get $a uptime",
		":do {",   // boucle protégée on-error
		"&total=", // compteur exact même au-delà du plafond
		"&data=",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("buildPppReadActive : %q absent", want)
		}
	}
	if !strings.Contains(s, "< "+strconv.Itoa(PppActiveReportCap)) {
		t.Errorf("buildPppReadActive : plafond %d attendu dans le script", PppActiveReportCap)
	}
}

func TestBuildPppRosEscapeValues(t *testing.T) {
	b := bPPPOK()
	// Nom avec @ (légitime) et commentaire avec guillemet + dollar (hostile) :
	// rosEscape neutralise " \ $ — jamais de rupture de citation.
	cmd := model.Command{ID: "c-esc", Kind: model.CmdPppSecretAdd, Payload: PppSecretAddPayloadFrom(
		"abdou@fai.ci", `pw"doll$ar`, "default", `Cadeau "Noël" $spécial`, "", false, false)}
	s := b.buildPppSecretAdd(cmd)
	if strings.Contains(s, `comment="mikcloud-ppp Cadeau "Noël"`) {
		t.Error("buildPppSecretAdd : guillemet non échappé dans le commentaire (injection possible)")
	}
	if !strings.Contains(s, `Cadeau \"Noël\" \$spécial`) {
		t.Errorf("buildPppSecretAdd : échappement rosEscape attendu, script %q", s)
	}
	if !strings.Contains(s, `password="pw\"doll\$ar"`) {
		t.Errorf("buildPppSecretAdd : échappement du mot de passe attendu, script %q", s)
	}
}

// TestBuildPppStaticAddress — N°294 (phase B) : remote-address dans les
// builders. add avec IP → remote-address="…" ; set avec pointeur vers "" →
// remote-address="" (REMISE sur le pool du profil — le champ présent est le
// contrat) ; set sans remoteAddress → aucune propriété envoyée ; IP hostile
// ou invalide → neutralisée par pppSafeIP (jamais d'injection).
func TestBuildPppStaticAddress(t *testing.T) {
	b := bPPPOK()
	cmd := model.Command{ID: "c-ip1", Kind: model.CmdPppSecretAdd, Payload: PppSecretAddPayloadFrom(
		"abdou@fai.ci", "pw", "default", "", "10.10.0.25", false, false)}
	s := b.buildPppSecretAdd(cmd)
	if !strings.Contains(s, `remote-address="10.10.0.25"`) {
		t.Errorf("buildPppSecretAdd : remote-address attendu, script %q", s)
	}
	// add sans IP statique → aucune propriété remote-address.
	cmd0 := model.Command{ID: "c-ip0", Kind: model.CmdPppSecretAdd, Payload: PppSecretAddPayloadFrom(
		"abdou@fai.ci", "pw", "default", "", "", false, false)}
	s0 := b.buildPppSecretAdd(cmd0)
	if strings.Contains(s0, "remote-address") {
		t.Errorf("buildPppSecretAdd : remote-address ne doit pas apparaître sans IP statique, script %q", s0)
	}
	// set : pointeur vers "" → remote-address="" (retour au pool).
	empty := ""
	cmd2 := model.Command{ID: "c-ip2", Kind: model.CmdPppSecretSet, Payload: PppSecretSetPayloadFrom(
		nil, nil, nil, &empty, nil)}
	s2 := b.buildPppSecretSet(cmd2)
	if !strings.Contains(s2, `remote-address=""`) {
		t.Errorf("buildPppSecretSet : remote-address=\"\" attendu (remise pool), script %q", s2)
	}
	// set : remoteAddress absent → aucune propriété remote-address.
	dis := false
	cmd3 := model.Command{ID: "c-ip3", Kind: model.CmdPppSecretSet, Payload: PppSecretSetPayloadFrom(
		nil, nil, nil, nil, &dis)}
	s3 := b.buildPppSecretSet(cmd3)
	if strings.Contains(s3, "remote-address") {
		t.Errorf("buildPppSecretSet : remote-address ne doit pas apparaître sans pointeur, script %q", s3)
	}
	// IP invalide/hostile au payload → neutralisée (jamais d'injection).
	cmd4 := model.Command{ID: "c-ip4", Kind: model.CmdPppSecretSet,
		Payload: map[string]any{"name": "abdou@fai.ci", "remoteAddress": `10.0.0.1" evil`}}
	s4 := b.buildPppSecretSet(cmd4)
	if strings.Contains(s4, "evil") {
		t.Errorf("buildPppSecretSet : IP hostile non neutralisée, script %q", s4)
	}
	if !strings.Contains(s4, `remote-address=""`) {
		t.Errorf("buildPppSecretSet : IP invalide doit devenir chaîne vide, script %q", s4)
	}
}
