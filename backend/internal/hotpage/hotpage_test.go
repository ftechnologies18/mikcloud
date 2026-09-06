// Package hotpage — tests du template embarqué et de la signature (N°35).
package hotpage

import (
	"strings"
	"testing"
)

// TestDefaultFilesNonEmpty — le template doit contenir au minimum login.html,
// status.html et md5.js (les fichiers critiques du portail MikroTik). Ce test
// échoue si le dossier template/ est vide au build (CI détecterait).
func TestDefaultFilesNonEmpty(t *testing.T) {
	files := DefaultFiles()
	if len(files) < 3 {
		t.Fatalf("DefaultFiles() = %d fichiers, attendu ≥ 3 (login.html, status.html, md5.js au minimum)", len(files))
	}
}

// TestDefaultFilesOrder — login.html est TOUJOURS en dernier (atomicité du
// déploiement : la page critique pré-auth n'est écrasée qu'une fois les
// assets en place). md5.js précède les pages HTML qui le référencent.
func TestDefaultFilesOrder(t *testing.T) {
	files := DefaultFiles()
	if len(files) == 0 {
		t.Fatal("DefaultFiles() vide")
	}
	last := files[len(files)-1]
	if last != "login.html" {
		t.Errorf("login.html doit fermer la marche (dernier fichier), ordre actuel : dernier=%q", last)
	}
	// md5.js doit apparaître avant login.html.
	md5Idx, loginIdx := -1, -1
	for i, f := range files {
		if f == "md5.js" {
			md5Idx = i
		}
		if f == "login.html" {
			loginIdx = i
		}
	}
	if md5Idx < 0 {
		t.Error("md5.js absent du template — requis par login.html pour CHAP-MD5")
	}
	if loginIdx < 0 {
		t.Error("login.html absent du template — page critique du portail")
	}
	if md5Idx >= 0 && loginIdx >= 0 && md5Idx > loginIdx {
		t.Errorf("md5.js (idx %d) doit précéder login.html (idx %d)", md5Idx, loginIdx)
	}
}

// TestDefaultFilesNoDup — un fichier ne doit pas apparaître deux fois (la sig
// serait instable et le déploiement gaspillerait un fetch).
func TestDefaultFilesNoDup(t *testing.T) {
	files := DefaultFiles()
	seen := map[string]bool{}
	for _, f := range files {
		if seen[f] {
			t.Errorf("fichier en double : %q", f)
		}
		seen[f] = true
	}
}

// TestRawFile — le fichier login.html existe et contient le marqueur CHAP
// (preuve que le template de référence est bien embarqué).
func TestRawFile(t *testing.T) {
	login := RawFile("login.html")
	if login == "" {
		t.Fatal("RawFile(login.html) vide — template non embarqué")
	}
	if !strings.Contains(login, "chap-id") && !strings.Contains(login, "doLogin") {
		t.Error("login.html ne contient ni 'chap-id' ni 'doLogin' — template incorrect ?")
	}
}

// TestRawFileSecurity — pas de remontée de dossier (sécurité défense en
// profondeur : l'appelant passe déjà un path relatif, mais le package doit
// refuser toute tentative de ../).
func TestRawFileSecurity(t *testing.T) {
	if RawFile("../go.mod") != "" {
		t.Error("RawFile(../go.mod) doit retourner '' (remontée refusée)")
	}
	if RawFile("/etc/passwd") != "" {
		t.Error("RawFile(/etc/passwd) doit retourner '' (chemin absolu refusé)")
	}
	if RawFile("") != "" {
		t.Error("RawFile('') doit retourner '' (chemin vide refusé)")
	}
}

// TestFileBinary — les assets binaires (png, woff2) sont servis via File([]byte).
func TestFileBinary(t *testing.T) {
	// logo.png existe dans le template de référence (testé à l'audit).
	b, ok := File("img/logo.png")
	if !ok {
		t.Fatal("File(img/logo.png) = nil, attendu []byte — asset binaire non servi")
	}
	if len(b) < 1000 {
		t.Errorf("File(img/logo.png) = %d octets, attendu > 1000 (logo.png ~288 Ko à l'audit)", len(b))
	}
	// Signature PNG : 89 50 4E 47 0D 0A 1A 0A
	if len(b) < 8 || b[0] != 0x89 || b[1] != 0x50 || b[2] != 0x4E || b[3] != 0x47 {
		t.Error("File(img/logo.png) — en-tête PNG invalide")
	}
}

// TestHasFile — cohérence entre HasFile et File/RawFile.
func TestHasFile(t *testing.T) {
	cases := map[string]bool{
		"login.html":            true,
		"status.html":           true,
		"md5.js":                true,
		"css/bootstrap.min.css": true,
		"img/logo.png":          true,
		"nonexistent.html":      false,
		"../go.mod":             false,
		"":                      false,
	}
	for path, want := range cases {
		if got := HasFile(path); got != want {
			t.Errorf("HasFile(%q) = %v, attendu %v", path, got, want)
		}
	}
}

// TestSigStable — la sig est stable pour deux listes identiques (ordres
// différents), différente pour deux listes différentes.
func TestSigStable(t *testing.T) {
	a := []string{"login.html", "status.html", "md5.js"}
	b := []string{"md5.js", "login.html", "status.html"} // même ensemble, ordre différent
	c := []string{"login.html", "status.html"}           // ensemble différent
	sa, sb, sc := Sig(a), Sig(b), Sig(c)
	if sa != sb {
		t.Errorf("Sig instable : a=%q b=%q (doivent être égaux — même ensemble)", sa, sb)
	}
	if sa == sc {
		t.Errorf("Sig collision : a=%q c=%q (doivent différer — ensembles différents)", sa, sc)
	}
	if len(sa) != 16 {
		t.Errorf("Sig longueur = %d, attendu 16 (cf. walledGardenSig)", len(sa))
	}
}

// TestSigEmpty — la sig d'une liste vide ne panique pas.
func TestSigEmpty(t *testing.T) {
	s := Sig(nil)
	if len(s) != 16 {
		t.Errorf("Sig(nil) longueur = %d, attendu 16", len(s))
	}
}
