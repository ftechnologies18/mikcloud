// Tests du package agent — fonctions pures uniquement (aucune connexion
// réseau, aucun appel sortant du routeur).
//
// Couverture :
//   - tokens : NewToken (base64url 32 car.), HashToken (SHA-256 hex 64,
//     déterministe), Preview ;
//   - échappement RouterOS : rosEscape neutralise \, ", $, retours à la
//     ligne/tabs (injection d'expression), rosScriptValue (script imbriqué),
//     urlEscape ;
//   - assainissement : SanitizeName (alphabet réduit, 48 car.), idSafe,
//     rosMinutes (formats durée RouterOS) ;
//   - VersionAtLeast : garde TLS RouterOS ≥ 7.19, suffixes d'édition,
//     version illisible → tolérante (fail-open documenté) ;
//   - scripts : InstallScript (marqueurs attendus, nom neutralisé),
//     NopScript, ScriptFor → buildVoucherBatch / buildUserAdd (injection
//     neutralisée dans nom/mot de passe/commentaire), kind inconnu → erreur ;
//   - lecture de payload : plStr, plInt64, plBool, plProfile, plUserList,
//     plStrList (formes JSON et mémoire).
package agent

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"testing"

	"mikcloud/hotspot-api/internal/model"
)

func TestNewTokenAndHash(t *testing.T) {
	tok, err := NewToken()
	if err != nil {
		t.Fatalf("NewToken impossible : %v", err)
	}
	if len(tok) != 32 { // 24 octets → 32 caractères base64url
		t.Fatalf("longueur du token = %d, attendu 32", len(tok))
	}
	if _, err := base64.RawURLEncoding.DecodeString(tok); err != nil {
		t.Fatalf("le token doit être base64url : %v", err)
	}
	tok2, _ := NewToken()
	if tok == tok2 {
		t.Fatal("deux tokens successifs doivent différer")
	}

	// HashToken : SHA-256 hexadécimal, déterministe.
	h1, h2 := HashToken(tok), HashToken(tok)
	if h1 != h2 {
		t.Fatal("HashToken doit être déterministe")
	}
	if len(h1) != 64 {
		t.Fatalf("longueur du hash = %d, attendu 64 (SHA-256 hex)", len(h1))
	}
	sum := sha256.Sum256([]byte(tok))
	if h1 != hex.EncodeToString(sum[:]) {
		t.Fatal("HashToken doit être exactement sha256(token) hexadécimal")
	}
	if HashToken("autre") == h1 {
		t.Fatal("des tokens différents doivent donner des hash différents")
	}
}

func TestPreview(t *testing.T) {
	if got := Preview("ABCDEFGH"); got != "ABCD…" {
		t.Fatalf("Preview = %q, attendu %q", got, "ABCD…")
	}
	if got := Preview("abc"); got != "abc" {
		t.Fatalf("token court renvoyé tel quel, obtenu %q", got)
	}
}

func TestRosEscapeNeutralizesInjection(t *testing.T) {
	cas := []struct{ in, want string }{
		{`abc"def`, `abc\"def`},       // guillemet : casse la chaîne
		{`pa$$w0rd`, `pa\$\$w0rd`},    // $ : interpolation de variables RouterOS
		{`back\slash`, `back\\slash`}, // antislash
		{"ligne1\nligne2", "ligne1 ligne2"},
		{"retour\rchariot", "retour chariot"},
		{"tab\there", "tab here"},
	}
	for _, c := range cas {
		if got := rosEscape(c.in); got != c.want {
			t.Fatalf("rosEscape(%q) = %q, attendu %q", c.in, got, c.want)
		}
	}
	// Tentative d'injection complète : le résultat ne contient plus AUCUN
	// guillemet non échappé ni commande exécutable dans la chaîne.
	malveillant := "\"; /ip hotspot user add name=hax password=pwn; $()"
	esc := rosEscape(malveillant)
	if strings.ContainsAny(esc, "\"$\n\r") && !strings.Contains(esc, `\"`) {
		t.Fatalf("injection non neutralisée : %q", esc)
	}
	if strings.Contains(esc, "\n") {
		t.Fatal("les retours à la ligne doivent être remplacés")
	}
}

func TestRosScriptValue(t *testing.T) {
	script := ":if ($user = \"x\") do={}"
	got := rosScriptValue(script)
	if strings.Count(got, `$`) != strings.Count(got, `\$`) {
		t.Fatalf("chaque $ doit être échappé : %q", got)
	}
	if strings.Contains(got, `\"`) == false {
		t.Fatalf("chaque guillemet doit être échappé : %q", got)
	}
	if strings.Contains(got, `\\$`) {
		t.Fatal("double échappement interdit")
	}
}

func TestUrlEscape(t *testing.T) {
	cas := []struct{ in, want string }{
		{"abc-DEF_1.2~", "abc-DEF_1.2~"}, // non réservés : inchangés
		{"espace ici", "espace_ici"},
		{"a&b=c/d", "a%26b%3Dc%2Fd"},
		{"100%", "100%25"},
	}
	for _, c := range cas {
		if got := urlEscape(c.in); got != c.want {
			t.Fatalf("urlEscape(%q) = %q, attendu %q", c.in, got, c.want)
		}
	}
}

func TestSanitizeName(t *testing.T) {
	if got := SanitizeName("  Session-Jour.1  "); got != "Session-Jour.1" {
		t.Fatalf("caractères licites et trim attendus, obtenu %q", got)
	}
	// Injection : guillemets, espaces, parenthèses → tout devient "-".
	if got := SanitizeName(`bad"; /ip hotspot user add name=hax`); strings.ContainsAny(got, `"; /`) {
		t.Fatalf("caractères dangereux conservés : %q", got)
	}
	// Accents → "-" (l'alphabet routeur est restreint volontairement).
	if got := SanitizeName("café"); got != "caf-" {
		t.Fatalf("accent non remplacé : %q", got)
	}
	// Limite 48 caractères.
	if got := SanitizeName(strings.Repeat("a", 60)); len(got) != 48 {
		t.Fatalf("troncature à 48 attendue, obtenu %d", len(got))
	}
}

func TestIdSafe(t *testing.T) {
	if got := idSafe("cmd-12_ab!34"); got != "cmd12ab34" {
		t.Fatalf("idSafe = %q, attendu %q", got, "cmd12ab34")
	}
	if got := idSafe("$$$"); got != "" {
		t.Fatalf("aucun caractère licite → vide, obtenu %q", got)
	}
}

func TestRosMinutes(t *testing.T) {
	cas := map[int]string{
		0: "", -5: "",
		90: "90m", 60: "1h", 120: "2h",
		1440: "1d", 2880: "2d", 43200: "30d",
		1441: "1441m", // non multiple : minutes brutes
	}
	for in, want := range cas {
		if got := rosMinutes(in); got != want {
			t.Fatalf("rosMinutes(%d) = %q, attendu %q", in, got, want)
		}
	}
}

func TestVersionAtLeast(t *testing.T) {
	cas := []struct {
		v     string
		major int
		minor int
		want  bool
	}{
		{"7.19.6 (stable)", 7, 19, true},
		{"7.20beta4", 7, 19, true},
		{"7.19rc1", 7, 19, true},
		{"7.19", 7, 19, true},
		{"7.18.2", 7, 19, false},
		{"6.49.13", 7, 19, false},
		{"8.0", 7, 19, true},
		{"7.6", 7, 19, false},
		// Version illisible → tolérante (le REGISTER fait le tri, cf. doc).
		{"", 7, 19, true},
		{"legacy", 7, 19, true},
	}
	for _, c := range cas {
		if got := VersionAtLeast(c.v, c.major, c.minor); got != c.want {
			t.Fatalf("VersionAtLeast(%q,%d,%d) = %v, attendu %v", c.v, c.major, c.minor, got, c.want)
		}
	}
}

func TestInstallScriptMarkers(t *testing.T) {
	script := InstallScript("https://cloud.mikcloud.example/", "tok-en-secret", "Routeur Cocody")
	for _, marqueur := range []string{
		SchedulerName,                    // scheduler créé/recréé
		ScriptFilename,                   // fichier de commandes
		"tok-en-secret",                  // token inliné
		"https://cloud.mikcloud.example", // slash final retiré
		"/agent/register?token=",         // inscription
		"/agent/cmd?token=",              // check-in
		":do {",                          // corps enveloppé dans UN seul bloc
		"interval=45s",                   // fréquence de poll
	} {
		if !strings.Contains(script, marqueur) {
			t.Fatalf("marqueur absent de l'InstallScript : %q", marqueur)
		}
	}
	// Sécurité P0 #5 : AUCUN repli check-certificate=no ACTIF. Les lignes de
	// commentaire (#) qui documentent l'absence du repli sont ignorées.
	for _, ligne := range strings.Split(script, "\n") {
		l := strings.TrimSpace(ligne)
		if strings.HasPrefix(l, "#") {
			continue
		}
		if strings.Contains(l, "check-certificate=no") {
			t.Fatal("l'InstallScript ne doit jamais désactiver la validation TLS")
		}
	}
	// Nom de routeur avec retour à la ligne : neutralisé (pas d'injection
	// dans le commentaire .rsc) mais le nom reste lisible.
	withNewline := InstallScript("https://cloud.example", "tok", "nom\ninjected")
	if strings.Contains(withNewline, "nom\ninjected") {
		t.Fatal("le retour à la ligne du nom doit devenir une espace")
	}
	if !strings.Contains(withNewline, "nom injected") {
		t.Fatal("le nom doit rester lisible après neutralisation")
	}
}

func TestNopScript(t *testing.T) {
	if got := NopScript(); got != "# mikcloud nop\n" {
		t.Fatalf("NopScript = %q", got)
	}
}

func TestScriptForVoucherBatch(t *testing.T) {
	b := Builder{BaseURL: "https://cloud.mikcloud.example/", Token: "tok-agent"}
	cmd := model.Command{ID: "cmd-77", Kind: model.CmdVoucherBatch, Payload: map[string]any{
		"batch":           "LOT-2024-12",
		"comment":         `lot spécial "urgent"`,
		"limitBytesTotal": 5242880,
		"limitUptimeMin":  1440,
		"profile": map[string]any{
			"name": "Pass 24h", "rateLimit": "5M/5M",
			"sessionTimeoutMin": 1440, "sharedUsers": 2,
		},
		"users": []map[string]any{
			{"name": "SC-0001", "password": `p@ss"1`},
			{"name": `SC-0002"; /ip hotspot user add name=hax`, "password": "$interpol"},
		},
	}}
	script, err := b.ScriptFor(cmd)
	if err != nil {
		t.Fatalf("ScriptFor impossible : %v", err)
	}
	// Marqueurs attendus.
	for _, marqueur := range []string{
		"# mikcloud cmd cmd-77 " + model.CmdVoucherBatch,
		`name="SC-0001"`,
		`password="p@ss\"1"`, // guillemet du mot de passe échappé
		`profile="Pass 24h"`,
		"limit-uptime=1d",           // 1440 min → 1d
		"limit-bytes-total=5242880", // quota de données
		`comment="lot spécial \"urgent\" · mikcloud:LOT-2024-12"`,
		"/agent/result?token=tok-agent",
	} {
		if !strings.Contains(script, marqueur) {
			t.Fatalf("marqueur absent du script de lot : %q", marqueur)
		}
	}
	// Le nom malveillant est assaini : plus aucun guillemet/point-virgule.
	if strings.Contains(script, `SC-0002"; /ip hotspot user add name=hax`) {
		t.Fatal("injection dans le nom de voucher non neutralisée")
	}
	if strings.Contains(script, `password="$interpol"`) {
		t.Fatal("$ non échappé dans le mot de passe (interpolation RouterOS)")
	}
}

func TestScriptForUserAdd(t *testing.T) {
	b := Builder{BaseURL: "https://cloud.example", Token: "tk"}
	cmd := model.Command{ID: "cmd-9", Kind: model.CmdUserAdd, Payload: map[string]any{
		"name": "client.regulier", "password": "mdp", "server": "all",
		"limitBytesTotal": 1073741824, "comment": "client au comptoir",
		"profile": map[string]any{"name": "Staff", "sessionTimeoutMin": 43200},
	}}
	script, err := b.ScriptFor(cmd)
	if err != nil {
		t.Fatalf("ScriptFor impossible : %v", err)
	}
	for _, marqueur := range []string{
		`/ip hotspot user add name="client.regulier" password="mdp"`,
		`server="all"`,
		"limit-bytes-total=1073741824",
		`comment="client au comptoir"`,
		"limit-uptime=30d", // hérité du session-timeout du profil (43200 min)
	} {
		if !strings.Contains(script, marqueur) {
			t.Fatalf("marqueur absent du script user_add : %q", marqueur)
		}
	}
}

func TestScriptForUnknownKind(t *testing.T) {
	b := Builder{BaseURL: "https://cloud.example", Token: "tk"}
	if _, err := b.ScriptFor(model.Command{ID: "cmd-x", Kind: "kind-bidon"}); err == nil {
		t.Fatal("un kind inconnu doit produire une erreur")
	}
}

func TestPayloadReaders(t *testing.T) {
	// plInt64 : tolère float64 (JSON relu), int64, int (mémoire).
	payload := map[string]any{"f": float64(7), "i64": int64(9), "i": 11, "bad": "12"}
	for k, want := range map[string]int64{"f": 7, "i64": 9, "i": 11, "absent": 0, "bad": 0} {
		if got := plInt64(payload, k); got != want {
			t.Fatalf("plInt64(%q) = %d, attendu %d", k, got, want)
		}
	}
	if plStr(payload, "bad") != "12" || plStr(payload, "absent") != "" || plStr(nil, "x") != "" {
		t.Fatal("plStr : comportement inattendu")
	}
	if plBool(map[string]any{"v": true}, "v") != true || plBool(map[string]any{"v": "oui"}, "v") != false {
		t.Fatal("plBool : seuls les booléens sont acceptés")
	}
	if !plHas(payload, "bad") || plHas(payload, "absent") {
		t.Fatal("plHas : détection de présence incorrecte")
	}

	// plProfile : tolère la forme JSON (float64) ET mémoire (int).
	jsonForme := map[string]any{"profile": map[string]any{
		"name": "P", "rateLimit": "2M/2M", "sessionTimeoutMin": float64(60),
		"sharedUsers": float64(3), "lockFirstDevice": true,
	}}
	p := plProfile(jsonForme, "profile")
	if p.Name != "P" || p.RateLimit != "2M/2M" || p.SessionTimeoutMin != 60 || p.SharedUsers != 3 || !p.LockFirstDevice {
		t.Fatalf("plProfile (forme JSON) incorrect : %+v", p)
	}
	if !p.HasTimeout || !p.HasShared || !p.HasRate {
		t.Fatalf("présence des clés mal détectée : %+v", p)
	}
	memoireForme := map[string]any{"profile": map[string]any{
		"name": "Q", "sessionTimeoutMin": 120, // int Go en mémoire
	}}
	p = plProfile(memoireForme, "profile")
	if p.SessionTimeoutMin != 120 {
		t.Fatalf("plProfile (forme mémoire) : sessionTimeoutMin = %d, attendu 120 (incident 31/08)", p.SessionTimeoutMin)
	}
	// Clé absente → profil vide, sans panique.
	if p := plProfile(map[string]any{}, "profile"); p.Name != "" || p.HasRate {
		t.Fatal("payload sans profil → ProfileRef vide attendu")
	}

	// plUserList : formes []any (JSON) et []map[string]any (mémoire).
	listeJSON := map[string]any{"users": []any{
		map[string]any{"name": "U1", "password": "p1"},
		"pas-une-map",
	}}
	users := plUserList(listeJSON, "users")
	if len(users) != 1 || users[0].Name != "U1" || users[0].Password != "p1" {
		t.Fatalf("plUserList (JSON) incorrect : %+v", users)
	}
	listeMemoire := map[string]any{"users": []map[string]any{{"name": "U2"}}}
	if users := plUserList(listeMemoire, "users"); len(users) != 1 || users[0].Name != "U2" {
		t.Fatalf("plUserList (mémoire) incorrect : %+v", users)
	}

	// plStrList : mêmes deux formes.
	if l := plStrList(map[string]any{"l": []any{"a", "b"}}, "l"); len(l) != 2 || l[1] != "b" {
		t.Fatalf("plStrList (JSON) incorrect : %v", l)
	}
	if l := plStrList(map[string]any{"l": []string{"x"}}, "l"); len(l) != 1 || l[0] != "x" {
		t.Fatalf("plStrList (mémoire) incorrect : %v", l)
	}
}

func TestSanitizeWGDomain(t *testing.T) {
	cas := []struct{ in, want string }{
		{"Mikcloud.Ftci.fr", "mikcloud.ftci.fr"},         // normalisation casse
		{"  api.example.com  ", "api.example.com"},       // trim
		{"hop.example.com:8443", "hop.example.com:8443"}, // port non standard OK
		{"x.example.com:443x", ""},                       // port non numérique → refus
		{"x.example.com:", ""},                           // port vide → refus
		{"bad host.example", ""},                         // espace → refus
		{`evil"; /system reboot`, ""},                    // injection → refus
		{"", ""},                                         // vide → refus
		{strings.Repeat("a", 254), ""},                   // > 253 → refus
	}
	for _, c := range cas {
		if got := SanitizeWGDomain(c.in); got != c.want {
			t.Fatalf("SanitizeWGDomain(%q) = %q, attendu %q", c.in, got, c.want)
		}
	}
}

func TestWalledGardenScript(t *testing.T) {
	b := Builder{BaseURL: "https://cloud.example", Token: "tok-agent"}
	cmd := model.Command{ID: "c-wg01", Kind: model.CmdWalledGarden, Payload: map[string]any{
		"domains": []any{"Mikcloud.Ftci.fr", "api.example.com:8443"},
		"sig":     "abcd1234abcd1234",
	}}
	script, err := b.ScriptFor(cmd)
	if err != nil {
		t.Fatalf("ScriptFor(walled_garden) : %v", err)
	}
	for _, marqueur := range []string{
		"# mikcloud cmd c-wg01 walled_garden",
		`:local okcwg01 true`,
		`/ip hotspot walled-garden remove [find comment~"` + WalledGardenMarker + `"]`,
		`/ip hotspot walled-garden ip remove [find comment~"` + WalledGardenMarker + `"]`,
		`/ip hotspot walled-garden add action=allow dst-host="mikcloud.ftci.fr" comment="` + WalledGardenMarker + ` page"`,
		`/ip hotspot walled-garden add action=allow dst-host="api.example.com:8443" comment="` + WalledGardenMarker + ` page"`,
		`/ip hotspot walled-garden ip add action=allow protocol=udp dst-port=53 comment="` + WalledGardenMarker + ` dns"`,
		`/ip hotspot walled-garden ip add action=allow protocol=tcp dst-port=53 comment="` + WalledGardenMarker + ` dns"`,
		"domains=2", // rapport : 2 règles page/api posées
	} {
		if !strings.Contains(script, marqueur) {
			t.Fatalf("marqueur absent du script walled_garden : %q", marqueur)
		}
	}

	// Injection : un domaine hostile est REFUSÉ par l'assainisseur → il ne
	// peut ni apparaître dans le script ni y exécuter quoi que ce soit.
	hostile := model.Command{ID: "c-wg02", Kind: model.CmdWalledGarden, Payload: map[string]any{
		"domains": []any{`evil"; /system reboot x`},
	}}
	s2, err := b.ScriptFor(hostile)
	if err != nil {
		t.Fatalf("ScriptFor(walled_garden hostile) : %v", err)
	}
	if strings.Contains(s2, "/system reboot") || strings.Contains(s2, `evil"`) {
		t.Fatal("le domaine hostile doit être neutralisé par SanitizeWGDomain")
	}
	if !strings.Contains(s2, "domains=0") {
		t.Fatal("aucune règle valide → rapport domains=0 attendu")
	}
}

func TestWalledGardenInstallBlock(t *testing.T) {
	// Domaines annoncés → le bloc est présent, corps MULTI-lignes (règle du
	// parseur console), règles page + DNS, marqueur d'idempotence.
	with := InstallScript("https://cloud.example", "tok", "Routeur A", "a.example", "b.example")
	for _, marqueur := range []string{
		`/ip hotspot walled-garden remove [find comment~"` + WalledGardenMarker + `"]`,
		`/ip hotspot walled-garden add action=allow dst-host="a.example" comment="` + WalledGardenMarker + ` page"`,
		`/ip hotspot walled-garden add action=allow dst-host="b.example" comment="` + WalledGardenMarker + ` page"`,
		`/ip hotspot walled-garden ip add action=allow protocol=udp dst-port=53`,
		"on-error={}",
	} {
		if !strings.Contains(with, marqueur) {
			t.Fatalf("marqueur absent de l'InstallScript (walled-garden) : %q", marqueur)
		}
	}
	// Le bloc d'installation n'utilise JAMAIS de corps one-line (fragile au
	// collage console) : chaque add walled-garden est sur sa propre ligne.
	for _, ligne := range strings.Split(with, "\n") {
		l := strings.TrimSpace(ligne)
		if strings.HasPrefix(l, "/ip hotspot walled-garden add") && strings.Contains(l, ":do {") {
			t.Fatal("corps one-line détecté dans le bloc walled-garden de l'InstallScript")
		}
	}
	// Aucun domaine annoncé → aucun bloc (script inchangé pour ce cas).
	without := InstallScript("https://cloud.example", "tok", "Routeur B")
	if strings.Contains(without, "walled-garden add") {
		t.Fatal("sans domaine annoncé, l'InstallScript ne doit pas contenir de règles walled-garden")
	}
}
