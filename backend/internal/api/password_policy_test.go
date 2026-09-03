// password_policy_test.go — tests de la politique de mots de passe (S2).

package api

import (
	"strings"
	"testing"
)

func TestPasswordPolicy(t *testing.T) {
	cases := []struct {
		nom      string
		password string
		username string
		wantErr  bool
	}{
		{"ok : 10 caractères exactement", "1234abcdef", "", false},
		{"ok : passphrase longue", "Phrase secrète robuste 2026 !", "", false},
		{"ok : 10 runes accentuées", "éééééééééé", "", false},
		{"ok : mot de passe test maison", "mot-de-passe-8+", "gerant1", false},
		{"refusé : 9 caractères", "1234abcde", "", true},
		{"refusé : vide", "", "", true},
		{"refusé : denylist (10 runes)", "azertyuiop", "", true},
		{"refusé : denylist insensible à la casse", "PASSWORD123", "", true},
		{"refusé : identique au username", "Proprio42xyz", "proprio42xyz", true},
		{"refusé : 73 octets", strings.Repeat("a", 73), "", true},
		{"ok : 72 octets", strings.Repeat("a", 72), "", false},
		{"ok : username vide → contrôle ignoré", "long-et-unique", "", false},
	}
	for _, c := range cases {
		t.Run(c.nom, func(t *testing.T) {
			msg := passwordPolicyViolation(c.password, c.username)
			if c.wantErr && msg == "" {
				t.Fatalf("mot de passe %q accepté alors qu'il doit être refusé", c.password)
			}
			if !c.wantErr && msg != "" {
				t.Fatalf("mot de passe %q refusé : %s", c.password, msg)
			}
		})
	}
}

// TestPasswordPolicyMessages — les messages sont lisibles et orientés
// utilisateur (ils sont renvoyés tels quels dans writeErr 400).
func TestPasswordPolicyMessages(t *testing.T) {
	if msg := passwordPolicyViolation("court", ""); !strings.Contains(msg, "10") {
		t.Fatalf("message de longueur attendu : %q", msg)
	}
	if msg := passwordPolicyViolation("azertyuiop", ""); !strings.Contains(msg, "interdit") {
		t.Fatalf("message denylist attendu : %q", msg)
	}
	if msg := passwordPolicyViolation("TotoLeBoss1", "totoleboss1"); !strings.Contains(msg, "nom d'utilisateur") {
		t.Fatalf("message username attendu : %q", msg)
	}
}
