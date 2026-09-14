package agent

// Tests N°104 — QoS Manager : parseur de débits RouterOS (les deux dialectes
// « 17M » et « 17000000 », la relecture arrive formatée, l'émission part en
// bps bruts), ratios dérivés (burst/seuil), et forme des trois scripts
// (create-or-set + relecture, lecture des files, retrait propre).

import (
	"strings"
	"testing"

	"mikcloud/hotspot-api/internal/model"
)

func TestRosRateBps(t *testing.T) {
	cas := []struct {
		in   string
		want int64
	}{
		{"17M", 17_000_000},
		{"12.5M", 12_500_000},
		{"12,5M", 12_500_000}, // virgule décimale (locale)
		{"512k", 512_000},
		{"512K", 512_000}, // casse indifférente
		{"1.5G", 1_500_000_000},
		{"17000000", 17_000_000}, // entier brut
		{"10.5Mbps", 10_500_000}, // stats : suffixe bps
		{"0", 0},
		{"", 0},
		{"unlimited", 0},
		{"abc", 0},
		{"-5M", 0}, // négatif : donnée absurde → 0
	}
	for _, c := range cas {
		if got := RosRateBps(c.in); got != c.want {
			t.Errorf("RosRateBps(%q) = %d, attendu %d", c.in, got, c.want)
		}
	}
}

func TestRosRateBpsDual(t *testing.T) {
	up, down := RosRateBpsDual("17M/95M")
	if up != 17_000_000 || down != 95_000_000 {
		t.Fatalf("RosRateBpsDual(17M/95M) = (%d, %d), attendu (17000000, 95000000)", up, down)
	}
	// Paire brute (les limites relues peuvent être des entiers).
	up, down = RosRateBpsDual("20000000/95000000")
	if up != 20_000_000 || down != 95_000_000 {
		t.Fatalf("paire brute : (%d, %d)", up, down)
	}
	// Valeur unique : upload seul, download 0.
	up, down = RosRateBpsDual("8M")
	if up != 8_000_000 || down != 0 {
		t.Fatalf("valeur unique : (%d, %d)", up, down)
	}
}

func TestQoSDerived(t *testing.T) {
	// max 95 % de la capacité → burst = capacité = max×20/19 ; seuil = 80 %.
	maxUp, maxDown := int64(19_000_000), int64(95_000_000)
	bUp, bDown, tUp, tDown := QoSDerived(maxUp, maxDown)
	if bUp != 20_000_000 || bDown != 100_000_000 {
		t.Fatalf("burst = (%d, %d), attendu (20000000, 100000000)", bUp, bDown)
	}
	if tUp != 15_200_000 || tDown != 76_000_000 {
		t.Fatalf("seuil = (%d, %d), attendu (15200000, 76000000)", tUp, tDown)
	}
}

func TestBuildQueueEnsureScript(t *testing.T) {
	b := Builder{BaseURL: "https://cloud.example", Token: "tok"}
	script, err := b.ScriptFor(model.Command{ID: "c-qe", Kind: model.CmdQueueEnsure, Payload: map[string]any{
		"target": "192.168.10.0/24", "sig": "abc123",
		"maxUpBps": int64(20_000_000), "maxDownBps": int64(95_000_000),
		"burstUpBps": int64(21_000_000), "burstDownBps": int64(100_000_000),
		"thrUpBps": int64(16_000_000), "thrDownBps": int64(76_000_000),
	}})
	if err != nil {
		t.Fatalf("script queue_ensure : %v", err)
	}
	for _, want := range []string{
		`/queue simple find where name="mikcloud-qos"`,                 // create-or-set idempotent
		`/queue simple set $mkq target=192.168.10.0/24`,                // branche set
		`/queue simple add name="mikcloud-qos" target=192.168.10.0/24`, // branche add
		`max-limit=20000000/95000000`,                                  // bps BRUTS (upload d'abord)
		`queue=pcq-upload-default/pcq-download-default`,                // types PCQ par défaut
		`burst-limit=21000000/100000000`,
		`burst-threshold=16000000/76000000`,
		`burst-time=10s/10s`,
		`disabled=no`,                                 // une file désactivée ne bride rien
		`[:tostr [/queue simple get $mkq max-limit]]`, // RELECTURE de vérification
		`&data=". $rdata`,                             // rapport par le canal standard
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("script queue_ensure : fragment %q absent :\n%s", want, trunc700(script))
		}
	}
}

func TestBuildQueueReadScript(t *testing.T) {
	b := Builder{BaseURL: "https://cloud.example", Token: "tok"}
	script, err := b.ScriptFor(model.Command{ID: "c-qr", Kind: model.CmdQueueRead})
	if err != nil {
		t.Fatalf("script queue_read : %v", err)
	}
	for _, want := range []string{
		`:foreach qe in=[/queue simple find] do={`, // TOUTES les files (dynamiques incluses)
		`:if ($qn < 60) do={`,                      // même borne que read_resources
		`[:tostr [/queue simple get $qe name]]`,
		`[:tostr [/queue simple get $qe max-limit]]`,
		":do { :set qbs [:tostr [/queue simple get $qe bytes]] } on-error={ :set qbs \"\" }", // stats best-effort
		":do { :set qrt [:tostr [/queue simple get $qe rate]] } on-error={ :set qrt \"\" }",
		":do { :set qdy [:tostr [/queue simple get $qe dynamic]] } on-error={ :set qdy \"false\" }", // N°106 : drapeau dynamique
		`"queue|" . $qnm`, // forme du rapport
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("script queue_read : fragment %q absent :\n%s", want, trunc700(script))
		}
	}
}

func TestBuildQueueRemoveScript(t *testing.T) {
	b := Builder{BaseURL: "https://cloud.example", Token: "tok"}
	script, err := b.ScriptFor(model.Command{ID: "c-qrm", Kind: model.CmdQueueRemove})
	if err != nil {
		t.Fatalf("script queue_remove : %v", err)
	}
	for _, want := range []string{
		// Détacher d'abord les profils qui référencent la file…
		`/ip hotspot user profile set [find where parent-queue="mikcloud-qos"] parent-queue=none`,
		// …puis retirer la file…
		`/queue simple remove [find where name="mikcloud-qos"]`,
		// …et PROUVER la disparition (compte restant, vérité routeur).
		`[/queue simple print count-only where name="mikcloud-qos"]`,
		`"removed|" . [:tostr $mkleft]`,
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("script queue_remove : fragment %q absent :\n%s", want, trunc700(script))
		}
	}
}

// TestValidQueueName — N°106 — le contrat du nom de file, partagé handler et
// builder : charset strict, bornes, pas de dynamiques (chevrons).
func TestValidQueueName(t *testing.T) {
	for _, ok := range []string{"HOTSPOT-Total", "GLOBAL-Internet", "mikcloud-qos", "queue 1", "a", strings.Repeat("n", 64), "File_02.v7"} {
		if !ValidQueueName(ok) {
			t.Errorf("ValidQueueName(%q) = false, attendu true", ok)
		}
	}
	for _, bad := range []string{
		"", " ", "  double", "double  ", strings.Repeat("n", 65),
		"<D-SC-A21B>",   // dynamique RouterOS : jamais supprimable
		`mauvais"quote`, // guillemet : injection
		"slash/interdit", "pipe|interdit", "pt-virg;interdit", "dollar$interdit",
		"accentué", "tab\tinterdit",
	} {
		if ValidQueueName(bad) {
			t.Errorf("ValidQueueName(%q) = true, attendu false", bad)
		}
	}
}

// TestBuildQueueRemoveNamedScript — N°106 — le ménage à distance : payload
// « name » → le script v CETTE file (détachement, retrait, preuve), et ne
// mentionne PLUS mikcloud-qos — le gérant retire ce qu'il voit dans la
// table, jamais autre chose.
func TestBuildQueueRemoveNamedScript(t *testing.T) {
	b := Builder{BaseURL: "https://cloud.example", Token: "tok"}
	script, err := b.ScriptFor(model.Command{ID: "c-qrm2", Kind: model.CmdQueueRemove,
		Payload: map[string]any{"name": "HOTSPOT-Total"}})
	if err != nil {
		t.Fatalf("script queue_remove nommé : %v", err)
	}
	for _, want := range []string{
		`/ip hotspot user profile set [find where parent-queue="HOTSPOT-Total"] parent-queue=none`,
		`/queue simple remove [find where name="HOTSPOT-Total"]`,
		`[/queue simple print count-only where name="HOTSPOT-Total"]`,
		`"removed|" . [:tostr $mkleft]`,
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("script queue_remove nommé : fragment %q absent :\n%s", want, trunc700(script))
		}
	}
	if strings.Contains(script, "mikcloud-qos") {
		t.Fatalf("le retrait nommé ne doit PAS viser mikcloud-qos :\n%s", trunc700(script))
	}
}

// TestBuildQueueRemoveInvalidName — un payload corrompu fait ÉCHOUER la
// commande (elle sera marquée error côté file) : jamais de repli vers une
// autre file — ce serait supprimer la MAUVAISE.
func TestBuildQueueRemoveInvalidName(t *testing.T) {
	b := Builder{BaseURL: "https://cloud.example", Token: "tok"}
	for _, bad := range []string{"<D-u1>", `x"y`, "a;b"} {
		if _, err := b.ScriptFor(model.Command{ID: "c-bad", Kind: model.CmdQueueRemove,
			Payload: map[string]any{"name": bad}}); err == nil {
			t.Fatalf("nom %q : une erreur était attendue", bad)
		}
	}
}

// trunc700 — extrait lisible en cas d'échec (miroir de preview, côté agent).
func trunc700(s string) string {
	if len(s) > 700 {
		return s[:700] + "…"
	}
	return s
}
