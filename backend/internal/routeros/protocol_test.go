// Tests du package routeros — encodage/décodage du protocole binaire
// RouterOS (port 8728) UNIQUEMENT. Aucun test réseau/TCP : les fonctions
// de bas niveau sont exercées sur des buffers mémoire.
//
// Couverture :
//   - encodage des longueurs (writeLength/readLength) : aller-retours sur
//     toute la gamme 1 octet → 3 octets, bornes exactes 0x7F/0x80/0x3FFF… ;
//   - phrases (writeSentence/readSentence) : mots + terminateur vide, sans
//     connexion (Client alimenté avec des bufio sur un bytes.Buffer) ;
//   - parsePairs (=clé=valeur, clé sans valeur, clé=valeur avec =) ;
//   - parseUptime (durées RouterOS « 1w2d3h4m5s ») et parseInt64.
package routeros

import (
	"bufio"
	"bytes"
	"testing"
)

func TestLengthRoundtrip(t *testing.T) {
	valeurs := []int{
		0, 1, 5, 0x7E, 0x7F, // 1 octet
		0x80, 0x81, 0xFF, 0x3FFF, // 2 octets
		0x4000, 0xFFFF, 0x1FFFFF, // 3 octets
		0x200000, 0xFFFFFFF, // 4 octets
	}
	for _, n := range valeurs {
		var buf bytes.Buffer
		w := bufio.NewWriter(&buf)
		if err := writeLength(w, n); err != nil {
			t.Fatalf("writeLength(%d) : %v", n, err)
		}
		if err := w.Flush(); err != nil {
			t.Fatalf("flush : %v", err)
		}
		got, err := readLength(bufio.NewReader(&buf))
		if err != nil {
			t.Fatalf("readLength(%d) : %v", n, err)
		}
		if got != n {
			t.Fatalf("aller-retour longueur : %d → %d", n, got)
		}
	}
	// Bornes d'encodage : < 0x80 = 1 octet, 0x80 = 2 octets (0x81 0x00).
	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)
	_ = writeLength(w, 0x7F)
	_ = writeLength(w, 0x80)
	_ = w.Flush()
	encoded := buf.Bytes()
	if len(encoded) != 3 || encoded[0] != 0x7F || encoded[1] != 0x81 || encoded[2] != 0x00 {
		t.Fatalf("encodage des bornes incorrect : % x", encoded)
	}
}

func TestSentenceRoundtripWithoutConnection(t *testing.T) {
	var buf bytes.Buffer
	c := &Client{
		reader: bufio.NewReader(&buf),
		writer: bufio.NewWriter(&buf),
	}
	mots := []string{"/ip/hotspot/user/print", "=name=SC-0001", "?disabled=false", "commentaire avec espaces"}
	if err := c.writeSentence(mots...); err != nil {
		t.Fatalf("writeSentence : %v", err)
	}
	if err := c.writer.Flush(); err != nil {
		t.Fatalf("flush : %v", err)
	}
	got, err := c.readSentence()
	if err != nil {
		t.Fatalf("readSentence : %v", err)
	}
	if len(got) != len(mots) {
		t.Fatalf("phrase relue : %d mots, attendu %d (%v)", len(got), len(mots), got)
	}
	for i := range mots {
		if got[i] != mots[i] {
			t.Fatalf("mot %d : %q ≠ %q", i, got[i], mots[i])
		}
	}
	// Le mot vide marque la fin de phrase : le buffer est épuisé après.
	if _, err := c.readSentence(); err == nil {
		t.Fatal("après le terminateur, une lecture doit atteindre la fin du buffer (erreur attendue)")
	}
}

func TestParsePairs(t *testing.T) {
	m := parsePairs([]string{
		"=name=SC-0001",
		"=disabled=false",
		"=key=sans=séparateurs=problème", // seul le premier = sépare
		"=flag",                          // clé sans valeur → ""
		"pas-un-attribute",               // ignoré (pas de = initial)
	})
	if m["name"] != "SC-0001" || m["disabled"] != "false" || m["flag"] != "" {
		t.Fatalf("paires mal analysées : %v", m)
	}
	if m["key"] != "sans=séparateurs=problème" {
		t.Fatalf("valeur contenant des = doit être conservée : %q", m["key"])
	}
	if _, ok := m["pas-un-attribute"]; ok {
		t.Fatal("un mot sans = initial ne doit pas être pris comme attribut")
	}
}

func TestParseUptime(t *testing.T) {
	const (
		jour = int64(24 * 3600)
		heur = 3600
		minu = 60
	)
	cas := map[string]int64{
		"1w2d3h4m5s": 7*jour + 2*jour + 3*heur + 4*minu + 5,
		"45s":        45,
		"10m":        10 * minu,
		"2h":         2 * heur,
		"3d":         3 * jour,
		"1w":         7 * jour,
		"1w2d":       9 * jour,
		"":           0,
		"abc":        0,  // unités inconnues → 0
		"5x30s":      30, // x réinitialise le nombre en cours
	}
	for in, want := range cas {
		if got := parseUptime(in); got != want {
			t.Fatalf("parseUptime(%q) = %d, attendu %d", in, got, want)
		}
	}
}

func TestParseInt64(t *testing.T) {
	cas := map[string]int64{
		"42": 42, "  7 ": 7, "0": 0,
		"abc": 0, "": 0, "12x": 0,
	}
	for in, want := range cas {
		if got := parseInt64(in); got != want {
			t.Fatalf("parseInt64(%q) = %d, attendu %d", in, got, want)
		}
	}
}
