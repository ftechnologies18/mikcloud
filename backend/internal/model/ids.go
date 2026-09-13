// Identifiants et codes aléatoires (ID préfixés, codes vouchers, MAC, horodatage ISO).
// Extrait du monolithe models.go (N°88) — même package, contenu inchangé.
package model

import (
	"crypto/rand"
	"encoding/hex"
	"math/big"
	"strings"
	"time"
)

// CodeCharset — alphabet sans caractères ambigus (pas de 0/1/I/L/O) pour les vouchers.
const CodeCharset = "23456789ABCDEFGHJKMNPQRSTUVWXYZ"

// NewID génère un identifiant court lisible (ex. "u-9f3c1a2b4d5e").

// NewID génère un identifiant court lisible (ex. "u-9f3c1a2b4d5e").
func NewID(prefix string) string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return prefix + hex.EncodeToString([]byte(time.Now().UTC().Format("150405.000000000")))
	}
	return prefix + hex.EncodeToString(b)
}

// RandomCode génère un code alphanumérique de n caractères sans caractères ambigus.

// RandomCode génère un code alphanumérique de n caractères sans caractères ambigus.
func RandomCode(n int) string {
	var sb strings.Builder
	max := big.NewInt(int64(len(CodeCharset)))
	for i := 0; i < n; i++ {
		idx, err := rand.Int(rand.Reader, max)
		if err != nil {
			idx = big.NewInt(0)
		}
		sb.WriteByte(CodeCharset[idx.Int64()])
	}
	return sb.String()
}

// Presets de charset pour les codes de vouchers — inspirés du User Manager
// MikroTik (« abcd », « ABCD », « aBcD », « 5ab2C34d », « 5AB2C34D », « 5aB2c34D »).
// Tous les alphabets excluent les caractères ambigus (0/1/I/L/O) : les codes
// restent lisibles sur un ticket imprimé ou lus à voix haute par un revendeur.

// Presets de charset pour les codes de vouchers — inspirés du User Manager
// MikroTik (« abcd », « ABCD », « aBcD », « 5ab2C34d », « 5AB2C34D », « 5aB2c34D »).
// Tous les alphabets excluent les caractères ambigus (0/1/I/L/O) : les codes
// restent lisibles sur un ticket imprimé ou lus à voix haute par un revendeur.
const (
	CharsetDefault = ""    // chiffres + majuscules sûres (CodeCharset, recommandé)
	CharsetLower   = "abc" // minuscules            — preset « abcd »
	CharsetUpper   = "ABC" // majuscules            — preset « ABCD »
	CharsetLetters = "aBc" // lettres min + maj     — preset « aBcD »
	CharsetDigLow  = "5ab" // chiffres + minuscules — preset « 5ab2c34d »
	CharsetDigUp   = "5AB" // chiffres + majuscules — preset « 5AB2C34D »
	CharsetDigMix  = "5aB" // chiffres + lettres    — preset « 5aB2c34D »
	// Parité Mikhmon : jeu « 1234 » (chiffres purs, très demandé pour les
	// codes courts revendus par les revendeurs). Alphabet = digitSafe (sans
	// 0/1) pour conserver la règle « zéro caractère ambigu » de MikCloud.
	CharsetNum = "num" // chiffres purs          — preset « 1234 »
)

const (
	lowerSafe = "abcdefghijkmnpqrstuvwxyz" // sans l, o
	upperSafe = "ABCDEFGHJKMNPQRSTUVWXYZ"  // sans I, L, O
	digitSafe = "23456789"                 // sans 0, 1
)

// CharsetAlphabets associe chaque preset à son alphabet (sans ambiguïtés).

// CharsetAlphabets associe chaque preset à son alphabet (sans ambiguïtés).
var CharsetAlphabets = map[string]string{
	CharsetDefault: digitSafe + upperSafe,
	CharsetLower:   lowerSafe,
	CharsetUpper:   upperSafe,
	CharsetLetters: lowerSafe + upperSafe,
	CharsetDigLow:  digitSafe + lowerSafe,
	CharsetDigUp:   digitSafe + upperSafe,
	CharsetDigMix:  digitSafe + lowerSafe + upperSafe,
	CharsetNum:     digitSafe,
}

// RandomCodeFrom génère un code de n caractères dans l'alphabet du preset
// demandé (charset vide ou inconnu → alphabet MikCloud par défaut).

// RandomCodeFrom génère un code de n caractères dans l'alphabet du preset
// demandé (charset vide ou inconnu → alphabet MikCloud par défaut).
func RandomCodeFrom(n int, charset string) string {
	alphabet, ok := CharsetAlphabets[charset]
	if !ok || alphabet == "" {
		alphabet = CodeCharset
	}
	var sb strings.Builder
	max := big.NewInt(int64(len(alphabet)))
	for i := 0; i < n; i++ {
		idx, err := rand.Int(rand.Reader, max)
		if err != nil {
			idx = big.NewInt(0)
		}
		sb.WriteByte(alphabet[idx.Int64()])
	}
	return sb.String()
}

// RandomMAC génère une adresse MAC aléatoire "AA:BB:CC:DD:EE:FF".

// RandomMAC génère une adresse MAC aléatoire "AA:BB:CC:DD:EE:FF".
func RandomMAC() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	parts := make([]string, 6)
	for i, x := range b {
		parts[i] = strings.ToUpper(hex.EncodeToString([]byte{x}))
	}
	return strings.Join(parts, ":")
}

// NowISO retourne l'heure courante au format RFC3339 (UTC).

// NowISO retourne l'heure courante au format RFC3339 (UTC).
func NowISO() string { return time.Now().UTC().Format(time.RFC3339) }

// ---------------------------------------------------------------------------
// Types métier (réponses JSON strictement conformes au contrat TS)
// ---------------------------------------------------------------------------

// AccountMainID — identifiant littéral du compte principal (plateforme). Le
// compte principal porte TOUJOURS cet ID (migrations déterministes) : il hérite
// des données de l'ère mono-tenant et ne peut pas être désactivé.
