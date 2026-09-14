package model

import (
	"testing"
	"time"
)

// Tests N°101 — logique pure du registre d'appareils : normalisation MAC,
// pause effective (illimitée, bornée, expirée), nom affecté borné.

func TestNormalizeMAC(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"AA:BB:CC:DD:EE:FF", "AA:BB:CC:DD:EE:FF"},
		{"aa:bb:cc:dd:ee:ff", "AA:BB:CC:DD:EE:FF"},
		{"  Aa:Bb:cC:Dd:eE:Ff  ", "AA:BB:CC:DD:EE:FF"},
		{"", ""},
		{"AA:BB:CC:DD:EE", ""},       // trop court
		{"AA:BB:CC:DD:EE:FF:00", ""}, // trop long
		{"AA-BB-CC-DD-EE-FF", ""},    // séparateur inconnu
		{"GG:BB:CC:DD:EE:FF", ""},    // non hexa
		{"AA:BB:CC:DD:EE:F", ""},     // 11 chiffres
	}
	for _, c := range cases {
		if got := NormalizeMAC(c.in); got != c.want {
			t.Errorf("NormalizeMAC(%q) = %q, attendu %q", c.in, got, c.want)
		}
	}
}

func TestDevicePauseActiveAt(t *testing.T) {
	now := time.Date(2026, 3, 14, 19, 0, 0, 0, time.UTC)

	// Pause illimitée : PausedUntil vide = effective pour toujours.
	d := Device{Paused: true, PausedUntil: ""}
	if !d.PauseActiveAt(now) || !d.PauseActiveAt(now.Add(100*time.Hour)) {
		t.Fatal("pause illimitée : doit être effective maintenant et dans 100 h")
	}

	// Pause bornée : effective avant l'échéance, levée après.
	until := now.Add(30 * time.Minute).Format(time.RFC3339)
	d = Device{Paused: true, PausedUntil: until}
	if !d.PauseActiveAt(now.Add(time.Minute)) {
		t.Fatal("pause bornée : effective une minute avant l'échéance")
	}
	if d.PauseActiveAt(now.Add(31 * time.Minute)) {
		t.Fatal("pause bornée : levée après l'échéance (l'ensemble désiré l'excise)")
	}

	// Pause expirée : Paused peut rester posé (le check-in le lève), la
	// lecture EFFECTIVE dit déjà « rétabli » — l'UI ne ment jamais.
	d = Device{Paused: true, PausedUntil: now.Add(-time.Minute).Format(time.RFC3339)}
	if d.PauseActiveAt(now) {
		t.Fatal("pause expirée : l'état effectif doit être rétabli")
	}

	// Jamais demandée.
	d = Device{Paused: false, PausedUntil: until}
	if d.PauseActiveAt(now) {
		t.Fatal("sans pause demandée : aucun état effectif")
	}

	// Échéance illisible : repli prudent — pause ILLIMITÉE (couper trop
	// longtemps se répare par un clic ; l'inverse mentirait).
	d = Device{Paused: true, PausedUntil: "pas-une-date"}
	if !d.PauseActiveAt(now) {
		t.Fatal("pausedUntil illisible : repli illimité (prudent)")
	}
}

func TestSanitizeDeviceName(t *testing.T) {
	if got := SanitizeDeviceName("  TV du salon  "); got != "TV du salon" {
		t.Fatalf("trim : %q", got)
	}
	long := make([]rune, 80)
	for i := range long {
		long[i] = 'é'
	}
	if got := SanitizeDeviceName(string(long)); len([]rune(got)) != DeviceNameMax {
		t.Fatalf("borne : %d runes, attendu %d", len([]rune(got)), DeviceNameMax)
	}
	// Les accents restent : un nom de famille s'écrit avec (registre cloud,
	// jamais envoyé au routeur).
	if got := SanitizeDeviceName("Tél de mama"); got != "Tél de mama" {
		t.Fatalf("accents conservés : %q", got)
	}
}

func TestDeviceOnline(t *testing.T) {
	if !DeviceOnline(DeviceLeaseBound) {
		t.Fatal("bound = en ligne")
	}
	for _, s := range []string{DeviceLeaseWaiting, DeviceLeaseOffered, DeviceLeaseExpired, DeviceLeaseGone, ""} {
		if DeviceOnline(s) {
			t.Fatalf("statut %q doit s'afficher hors ligne", s)
		}
	}
}
