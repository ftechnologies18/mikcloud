package store

import (
	"strings"
	"testing"
)

// TestRLSStatementsCoverRegistry — N°166 — chaque table du registre doit
// porter exactement une instruction « ALTER TABLE ... ENABLE ROW LEVEL
// SECURITY » bien formée : sur un hébergeur mutualisé doté d'une API Data
// (Supabase : PostgREST + clé publishable), une table oubliée serait
// lisible par les rôles anon/authenticated.
func TestRLSStatementsCoverRegistry(t *testing.T) {
	stmts := rlsStatements()
	if len(stmts) != len(syncKnownTables) {
		t.Fatalf("RLS : %d instructions pour %d tables enregistrées", len(stmts), len(syncKnownTables))
	}
	const head = "ALTER TABLE "
	const tail = " ENABLE ROW LEVEL SECURITY"
	seen := make(map[string]bool, len(stmts))
	for _, s := range stmts {
		if !strings.HasPrefix(s, head) || !strings.HasSuffix(s, tail) || len(s) <= len(head)+len(tail) {
			t.Fatalf("instruction RLS mal formée : %q", s)
		}
		name := strings.TrimSuffix(strings.TrimPrefix(s, head), tail)
		if !syncKnownTables[name] {
			t.Fatalf("instruction RLS sur une table hors registre : %q", name)
		}
		if seen[name] {
			t.Fatalf("doublon RLS pour la table %q", name)
		}
		seen[name] = true
	}
	for name := range syncKnownTables {
		if !seen[name] {
			t.Fatalf("table %q du registre sans instruction RLS", name)
		}
	}
}
