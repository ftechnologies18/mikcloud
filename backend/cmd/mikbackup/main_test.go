// main_test.go — tests du chiffrement de mikbackup (S4) : aller-retour,
// clé erronée, fichier altéré. Les accès base (export/restore-check) sont
// exercés en production par l'opérateur via le runbook — hors unit tests.

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testFile(t *testing.T) (*backupFile, string) {
	t.Helper()
	bk := &backupFile{
		Version:     1,
		GeneratedAt: "2026-09-03T03:00:00Z",
		Tables: map[string]backupTable{
			"admin_users": {Rows: []string{`{"id":"usr-1","username":"probe"}`}},
			"accounts":    {Rows: []string{`{"id":"acc-1"}`, `{"id":"acc-2"}`}},
			"vide":        {},
		},
	}
	key := strings.Repeat("ab", 32) // 64 hex → 32 octets
	return bk, key
}

// TestBackupCipherRoundtrip — chiffrer puis déchiffrer rend le contenu
// exact (authenticité GCM).
func TestBackupCipherRoundtrip(t *testing.T) {
	bk, key := testFile(t)
	path := filepath.Join(t.TempDir(), "backup.enc")

	if err := writeEncrypted(path, key, bk); err != nil {
		t.Fatalf("écriture : %v", err)
	}
	got, err := readEncrypted(path, key)
	if err != nil {
		t.Fatalf("lecture : %v", err)
	}
	if got.GeneratedAt != bk.GeneratedAt || len(got.Tables) != len(bk.Tables) {
		t.Fatalf("contenu altéré : %+v", got)
	}
	if len(got.Tables["accounts"].Rows) != 2 || got.Tables["accounts"].Rows[1] != `{"id":"acc-2"}` {
		t.Fatalf("lignes modifiées : %+v", got.Tables["accounts"])
	}
}

// TestBackupWrongKeyRejected — la mauvaise clé est rejetée (GCM auth tag).
func TestBackupWrongKeyRejected(t *testing.T) {
	bk, key := testFile(t)
	path := filepath.Join(t.TempDir(), "backup.enc")
	if err := writeEncrypted(path, key, bk); err != nil {
		t.Fatalf("écriture : %v", err)
	}
	if _, err := readEncrypted(path, strings.Repeat("cd", 32)); err == nil {
		t.Fatal("une clé erronée doit être rejetée")
	}
}

// TestBackupTamperRejected — un fichier modifié est rejeté.
func TestBackupTamperRejected(t *testing.T) {
	bk, key := testFile(t)
	path := filepath.Join(t.TempDir(), "backup.enc")
	if err := writeEncrypted(path, key, bk); err != nil {
		t.Fatalf("écriture : %v", err)
	}
	raw, _ := os.ReadFile(path)
	lines := strings.SplitN(strings.TrimSpace(string(raw)), "\n", 2)
	// Corromps le dernier caractère du base64.
	lines[1] = lines[1][:len(lines[1])-1] + "A"
	if err := os.WriteFile(path, []byte(lines[0]+"\n"+lines[1]+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readEncrypted(path, key); err == nil {
		t.Fatal("un fichier altéré doit être rejeté")
	}
}

// TestBackupKeyValidation — clé non hex ou de mauvaise longueur refusée.
func TestBackupKeyValidation(t *testing.T) {
	bk, _ := testFile(t)
	path := filepath.Join(t.TempDir(), "backup.enc")
	if err := writeEncrypted(path, "court", bk); err == nil {
		t.Fatal("clé trop courte : doit être refusée")
	}
	if err := writeEncrypted(path, strings.Repeat("zz", 32), bk); err == nil {
		t.Fatal("clé non hexadécimale : doit être refusée")
	}
}
