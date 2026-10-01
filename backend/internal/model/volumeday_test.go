package model

// Tests N°199 — accumulateur journalier de volume de données :
//   - attribution jour/heure AU FUSEAU DU COMPTE (America/New_York) ;
//   - bascule de jour local (minuit New-Yorkais) : deux lignes ;
//   - repli UTC pour un compte inconnu ;
//   - histogramme horaire : parse tolérant, bornes, somme 0..h ;
//   - rétention 730 jours (borne incluse) ;
//   - résolution du fuseau du compte (model.AccountTimezone).

import (
	"testing"
	"time"
)

// volDB — base avec un compte à New York et un compte sans réglage.
func volDB() *DB {
	return &DB{SettingsByAccount: map[string]Settings{
		"acc-ny": {Tenant: Tenant{Timezone: "America/New_York"}},
	}}
}

func TestAccumulateVolumeDayTimezoneAndRollover(t *testing.T) {
	db := volDB()

	// 2026-07-14 04:30 UTC = 00:30 à New York (EDT, UTC-4) → jour local
	// « 2026-07-14 », heure 0.
	now := time.Date(2026, 7, 14, 4, 30, 0, 0, time.UTC)
	AccumulateVolumeDay(db, "acc-ny", "rt-1", 100, 50, now)
	if len(db.VolumeDays) != 1 {
		t.Fatalf("1 ligne attendue, obtenu %d", len(db.VolumeDays))
	}
	row := db.VolumeDays[0]
	if row.Day != "2026-07-14" {
		t.Fatalf("Day = %q, attendu 2026-07-14 (jour AU FUSEAU DU COMPTE)", row.Day)
	}
	if row.ID != VolumeDayID("acc-ny", "rt-1", "2026-07-14") {
		t.Fatalf("ID = %q, attendu la clé naturelle %q", row.ID, VolumeDayID("acc-ny", "rt-1", "2026-07-14"))
	}
	if row.BytesIn != 100 || row.BytesOut != 50 {
		t.Fatalf("totaux = %d/%d, attendu 100/50", row.BytesIn, row.BytesOut)
	}
	if got := VolumeHoursSumThrough(row.Hours, 0); got != 150 {
		t.Fatalf("heures[0] = %d, attendu 150", got)
	}

	// 05:15 UTC = 01:15 NY → même jour, heure 1 : la MÊME ligne s'enrichit.
	AccumulateVolumeDay(db, "acc-ny", "rt-1", 200, 100, now.Add(45*time.Minute))
	if len(db.VolumeDays) != 1 {
		t.Fatalf("toujours 1 ligne attendue, obtenu %d", len(db.VolumeDays))
	}
	row = db.VolumeDays[0]
	if row.BytesIn != 300 || row.BytesOut != 150 {
		t.Fatalf("totaux = %d/%d, attendu 300/150", row.BytesIn, row.BytesOut)
	}
	if got := VolumeHoursSumThrough(row.Hours, 0); got != 150 {
		t.Fatalf("heures[0] = %d, attendu 150 (inchangée)", got)
	}
	if got := VolumeHoursSumThrough(row.Hours, 1); got != 450 {
		t.Fatalf("heures[0..1] = %d, attendu 450", got)
	}

	// Bascule de jour local : 2026-07-15 04:30 UTC = 00:30 NY le 15 → NOUVELLE
	// ligne, la veille reste intacte.
	AccumulateVolumeDay(db, "acc-ny", "rt-1", 10, 5, now.Add(24*time.Hour))
	if len(db.VolumeDays) != 2 {
		t.Fatalf("2 lignes attendues après bascule de jour, obtenu %d", len(db.VolumeDays))
	}
	if db.VolumeDays[0].BytesIn != 300 || db.VolumeDays[0].BytesOut != 150 {
		t.Fatalf("ligne de la veille mutée : %d/%d", db.VolumeDays[0].BytesIn, db.VolumeDays[0].BytesOut)
	}
	if db.VolumeDays[1].Day != "2026-07-15" {
		t.Fatalf("Day de la nouvelle ligne = %q, attendu 2026-07-15", db.VolumeDays[1].Day)
	}

	// Deltas négatifs : clampés par direction — l'agrégat reste MONOTONE.
	AccumulateVolumeDay(db, "acc-ny", "rt-1", -500, 7, now.Add(24*time.Hour))
	if db.VolumeDays[1].BytesIn != 10 || db.VolumeDays[1].BytesOut != 12 {
		t.Fatalf("clamp attendu (10/12), obtenu %d/%d", db.VolumeDays[1].BytesIn, db.VolumeDays[1].BytesOut)
	}

	// Compte inconnu → UTC : 2026-07-14 01:00 UTC est encore le 13 à New York
	// (21:00) — le jour retenu prouve le repli UTC.
	AccumulateVolumeDay(db, "acc-inconnu", "rt-1", 7, 3, time.Date(2026, 7, 14, 1, 0, 0, 0, time.UTC))
	last := db.VolumeDays[len(db.VolumeDays)-1]
	if last.AccountID != "acc-inconnu" || last.Day != "2026-07-14" {
		t.Fatalf("compte inconnu : (%s, %s), attendu (acc-inconnu, 2026-07-14 UTC — pas le 13 NY)", last.AccountID, last.Day)
	}

	// Identifiants vides : ignorés, aucune ligne.
	before := len(db.VolumeDays)
	AccumulateVolumeDay(db, "", "rt-1", 5, 5, now)
	AccumulateVolumeDay(db, "acc-ny", "", 5, 5, now)
	AccumulateVolumeDay(db, "acc-ny", "rt-1", 0, 0, now)
	if len(db.VolumeDays) != before {
		t.Fatalf("identifiants vides / delta nul : %d lignes, attendu %d", len(db.VolumeDays), before)
	}
}

func TestVolumeHoursHelpers(t *testing.T) {
	// Versement simple à une heure donnée.
	hist := VolumeHoursAdd("", 5, 1000)
	if got := VolumeHoursSumThrough(hist, 5); got != 1000 {
		t.Fatalf("somme 0..5 = %d, attendu 1000", got)
	}
	if got := VolumeHoursSumThrough(hist, 4); got != 0 {
		t.Fatalf("somme 0..4 = %d, attendu 0 (le seau 5 est au-delà)", got)
	}

	// Forme corrompue : tolérante (champs non numériques → 0).
	hist = VolumeHoursAdd("abc,200", 1, 50)
	if got := VolumeHoursSumThrough(hist, 1); got != 250 {
		t.Fatalf("forme corrompue : somme 0..1 = %d, attendu 250 (0 + 200 + 50)", got)
	}

	// Heures hors bornes : ignorées au versement, clampées à la lecture.
	hist = VolumeHoursAdd(VolumeHoursAdd("", 23, 10), 24, 99)
	if got := VolumeHoursSumThrough(hist, 23); got != 10 {
		t.Fatalf("heure 24 doit être ignorée : somme = %d, attendu 10", got)
	}
	if got := VolumeHoursSumThrough("1,2,3", -1); got != 0 {
		t.Fatalf("uptoHour négatif : somme = %d, attendu 0", got)
	}
	if got := VolumeHoursSumThrough("1,2,3", 99); got != 6 {
		t.Fatalf("uptoHour > 23 : somme = %d, attendu 6 (clamp à 23)", got)
	}
	if got := VolumeHoursSumThrough("", 23); got != 0 {
		t.Fatalf("forme vide : somme = %d, attendu 0", got)
	}

	// Delta nul ou négatif au versement : forme recanonisée, inchangée en
	// valeur (aucun seau ne bouge).
	hist = VolumeHoursAdd("10,20", 7, -5)
	if got := VolumeHoursSumThrough(hist, 23); got != 30 {
		t.Fatalf("delta négatif : somme = %d, attendu 30 (inchangée)", got)
	}
}

func TestPruneVolumeDays(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	db := &DB{}
	mk := func(id string, daysAgo int) VolumeDay {
		return VolumeDay{ID: id, Day: now.AddDate(0, 0, -daysAgo).Format("2006-01-02")}
	}
	db.VolumeDays = []VolumeDay{mk("vd-800", 800), mk("vd-731", 731), mk("vd-730", 730), mk("vd-729", 729), mk("vd-0", 0)}
	if n := PruneVolumeDays(db, now); n != 2 {
		t.Fatalf("2 lignes purgées attendues (800 et 731 jours), obtenu %d", n)
	}
	if len(db.VolumeDays) != 3 {
		t.Fatalf("3 lignes restantes attendues, obtenu %d", len(db.VolumeDays))
	}
	if db.VolumeDays[0].ID != "vd-730" {
		t.Fatalf("la ligne de 730 jours (borne INCLUSE) doit rester, la première restante est %s", db.VolumeDays[0].ID)
	}
	if n := PruneVolumeDays(db, now); n != 0 {
		t.Fatalf("purge idempotente : %d lignes purgées, attendu 0", n)
	}
}

func TestAccountTimezone(t *testing.T) {
	db := &DB{SettingsByAccount: map[string]Settings{
		"acc-ny":  {Tenant: Tenant{Timezone: "America/New_York"}},
		"acc-bad": {Tenant: Tenant{Timezone: "Mars/Olympus"}},
	}}
	if got := AccountTimezone(db, "acc-ny"); got.String() != "America/New_York" {
		t.Fatalf("fuseau = %q, attendu America/New_York", got.String())
	}
	if got := AccountTimezone(db, "acc-bad"); got != time.UTC {
		t.Fatalf("fuseau invalide : repli UTC attendu, obtenu %q", got.String())
	}
	if got := AccountTimezone(db, "acc-sans-reglage"); got != time.UTC {
		t.Fatalf("compte inconnu : repli UTC attendu, obtenu %q", got.String())
	}
	if got := AccountTimezone(&DB{}, "quiconque"); got != time.UTC {
		t.Fatalf("base vide : repli UTC attendu, obtenu %q", got.String())
	}
}
