package model_test

// Tests N°82 — FamilyGuard : logique pure de la fenêtre du couvre-feu.
// C'est le cœur du module : bornes incluses/exclues, passage de minuit,
// sémantique des jours (jour de DÉBUT de la fenêtre), validation stricte
// et aller-retour du spec canonique.

import (
	"testing"
	"time"

	"mikcloud/hotspot-api/internal/model"
)

// fgTime — construit une heure UTC (le cloud calcule l'état en UTC ==
// heure d'Abidjan GMT, sans DST).
func fgTime(day time.Weekday, hh, mm int) time.Time {
	// 2026-09-14 est un lundi — base stable pour tous les jours de la semaine.
	base := time.Date(2026, time.September, 13, 0, 0, 0, 0, time.UTC) // dimanche
	delta := (int(day) - int(base.Weekday()) + 7) % 7
	return base.AddDate(0, 0, delta).Add(time.Duration(hh)*time.Hour + time.Duration(mm)*time.Minute)
}

// TestFamilyGuardActiveAtIntraday — fenêtre intra-jour (08:00 → 12:00,
// tous les jours) : le début est inclus, la fin EXCLUE, hors fenêtre
// inactif, et le jour non coché inactive même dans la plage horaire.
func TestFamilyGuardActiveAtIntraday(t *testing.T) {
	cfg := model.FamilyGuardConfig{Enabled: true, Start: "08:00", End: "12:00", Days: "1111111"}
	cases := []struct {
		day  time.Weekday
		hh   int
		mm   int
		want bool
	}{
		{time.Monday, 7, 59, false}, // juste avant
		{time.Monday, 8, 0, true},   // début inclus
		{time.Monday, 9, 30, true},  // au milieu
		{time.Monday, 11, 59, true}, // dernière minute
		{time.Monday, 12, 0, false}, // fin EXCLUE
		{time.Saturday, 10, 0, true},
		{time.Wednesday, 5, 0, false},
	}
	for _, c := range cases {
		if got := cfg.ActiveAt(fgTime(c.day, c.hh, c.mm)); got != c.want {
			t.Errorf("intraday %s %02d:%02d = %v, attendu %v", c.day, c.hh, c.mm, got, c.want)
		}
	}

	// Jour non coché : la plage horaire ne suffit pas.
	cfg.Days = "0000010" // samedi uniquement
	if cfg.ActiveAt(fgTime(time.Monday, 9, 0)) {
		t.Error("lundi non coché : la fenêtre ne doit pas être active")
	}
	if !cfg.ActiveAt(fgTime(time.Saturday, 9, 0)) {
		t.Error("samedi coché + 09:00 : la fenêtre doit être active")
	}
}

// TestFamilyGuardActiveAtMidnight — fenêtre franchissant minuit
// (22:00 → 06:00, tous les jours) : la portion du matin appartient à la
// fenêtre PARTIE LA VEILLE.
func TestFamilyGuardActiveAtMidnight(t *testing.T) {
	cfg := model.FamilyGuardConfig{Enabled: true, Start: "22:00", End: "06:00", Days: "1111111"}
	cases := []struct {
		day  time.Weekday
		hh   int
		mm   int
		want bool
	}{
		{time.Monday, 21, 59, false}, // juste avant
		{time.Monday, 22, 0, true},   // début inclus
		{time.Monday, 23, 30, true},  // portion du soir
		{time.Tuesday, 0, 30, true},  // après minuit = fenêtre partie lundi
		{time.Tuesday, 5, 59, true},  // dernière minute du matin
		{time.Tuesday, 6, 0, false},  // fin EXCLUE
		{time.Sunday, 2, 0, true},    // dimanche 02:00 = fenêtre partie samedi
	}
	for _, c := range cases {
		if got := cfg.ActiveAt(fgTime(c.day, c.hh, c.mm)); got != c.want {
			t.Errorf("minuit %s %02d:%02d = %v, attendu %v", c.day, c.hh, c.mm, got, c.want)
		}
	}
}

// TestFamilyGuardActiveAtMidnightDays — sémantique des jours sur une
// fenêtre nocturne : « vendredi » coché = la fenêtre DÉMARRE vendredi
// 22:00 et couvre samedi 00:00 → 06:00, même si le samedi n'est pas coché.
func TestFamilyGuardActiveAtMidnightDays(t *testing.T) {
	cfg := model.FamilyGuardConfig{Enabled: true, Start: "22:00", End: "06:00", Days: "0000100"} // vendredi
	if !cfg.ActiveAt(fgTime(time.Friday, 23, 0)) {
		t.Error("vendredi 23:00 (jour de début coché) : doit être actif")
	}
	if !cfg.ActiveAt(fgTime(time.Saturday, 1, 0)) {
		t.Error("samedi 01:00 (matin de la fenêtre partie vendredi) : doit être actif")
	}
	if cfg.ActiveAt(fgTime(time.Saturday, 23, 0)) {
		t.Error("samedi 23:00 (samedi non coché) : ne doit PAS être actif")
	}
	if cfg.ActiveAt(fgTime(time.Thursday, 23, 0)) {
		t.Error("jeudi 23:00 (jeudi non coché) : ne doit PAS être actif")
	}
}

// TestFamilyGuardDisabledAndInvalid — désactivé = jamais actif ; heures
// invalides ou égales = jamais actif (défense, la validation amont rejette).
func TestFamilyGuardDisabledAndInvalid(t *testing.T) {
	cfg := model.FamilyGuardConfig{Enabled: false, Start: "22:00", End: "06:00", Days: "1111111"}
	for _, day := range []time.Weekday{time.Monday, time.Thursday, time.Sunday} {
		for hh := 0; hh < 24; hh++ {
			if cfg.ActiveAt(fgTime(day, hh, 0)) {
				t.Fatalf("désactivé : ne doit jamais être actif (%s %02d:00)", day, hh)
			}
		}
	}
	bad := model.FamilyGuardConfig{Enabled: true, Start: "06:00", End: "06:00", Days: "1111111"}
	if bad.ActiveAt(fgTime(time.Monday, 6, 30)) {
		t.Error("début == fin : fenêtre nulle, ne doit jamais être active")
	}
	malformed := model.FamilyGuardConfig{Enabled: true, Start: "24:00", End: "06:00", Days: "1111111"}
	if malformed.ActiveAt(fgTime(time.Monday, 23, 0)) {
		t.Error("heure mal formée : ne doit jamais être active")
	}
}

// TestFamilyGuardSpecRoundTrip — aller-retour du spec canonique, et rejet
// des formes invalides (défense en profondeur du parse).
func TestFamilyGuardSpecRoundTrip(t *testing.T) {
	cfg := model.FamilyGuardConfig{Enabled: true, Start: "22:00", End: "06:00", Days: "1111111"}
	spec := cfg.SpecString()
	if spec != "1|22:00|06:00|1111111" {
		t.Fatalf("spec = %q, attendu la forme canonique", spec)
	}
	back, ok := model.ParseFamilyGuardSpec(spec)
	if !ok || back != cfg {
		t.Fatalf("aller-retour échoué : %+v (ok=%v)", back, ok)
	}

	disabled := model.FamilyGuardConfig{Enabled: false, Start: "08:00", End: "12:00", Days: "1000000"}
	if s := disabled.SpecString(); s != "0|08:00|12:00|1000000" {
		t.Fatalf("spec désactivé = %q", s)
	}
	if _, ok := model.ParseFamilyGuardSpec(s0(disabled)); !ok {
		t.Fatal("un spec désactivé mais bien formé doit parser")
	}

	for _, invalid := range []string{
		"",                        // jamais utilisé
		"1|22:00|06:00",           // jours manquants
		"1|22:00|06:00|111111",    // 6 jours
		"1|22:00|06:00|11111111",  // 8 jours
		"1|22:00|06:00|0000000",   // aucun jour actif
		"1|25:00|06:00|1111111",   // heure invalide
		"1|22:00|06:60|1111111",   // minute invalide
		"1|06:00|06:00|1111111",   // fenêtre nulle
		"1|2200|06:00|1111111",    // format HH:MM cassé
		"1|22:00|06:00|1111111|x", // 5 segments
	} {
		if _, ok := model.ParseFamilyGuardSpec(invalid); ok {
			t.Errorf("spec invalide accepté : %q", invalid)
		}
	}

	// Enabled non canonique ("2") : parse en « désactivé » — seule la
	// console écrit des specs, toujours canoniques (défense tolérante).
	if c, ok := model.ParseFamilyGuardSpec("2|22:00|06:00|1111111"); !ok || c.Enabled {
		t.Errorf("enabled non canonique : ok=%v enabled=%v — attendu ok=true, enabled=false", ok, c.Enabled)
	}
}

// s0 — forme canonique d'une config désactivée (helper de lisibilité).
func s0(c model.FamilyGuardConfig) string { return c.SpecString() }
