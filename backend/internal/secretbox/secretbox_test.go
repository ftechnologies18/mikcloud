// Tests du package secretbox — chiffrement AES-256-GCM au repos des
// identifiants routeur (audit P0 #6).
//
// Couverture :
//   - Init : dérivation depuis JWT_SECRET (HKDF) et depuis CREDENTIALS_KEY
//     (hex 64), rejets de clé invalide ;
//   - Encrypt/Decrypt : aller-retour, préfixe « enc:v1: », nonce aléatoire
//     (deux chiffrements diffèrent), idempotence (pas de double scellage),
//     chaîne vide ;
//   - Decrypt tolérant : valeur sans préfixe renvoyée telle quelle
//     (données antérieures au correctif), valeur chiffrée corrompue → ""
//     (jamais le ciphertext brut) ;
//   - mauvaise clé (secret JWT changé) → déchiffrement impossible → "".
package secretbox

import (
	"encoding/base64"
	"encoding/hex"
	"os"
	"strings"
	"testing"
)

const testSecret = "jwt-secret-de-test-secretbox"

// TestMain — initialise la clé UNE fois pour tout le paquet (variable globale
// gcm). Aucune clé d'environnement ne doit détourner la dérivation.
func TestMain(m *testing.M) {
	os.Unsetenv("CREDENTIALS_KEY")
	os.Unsetenv("DATABASE_URL")
	if err := Init(testSecret); err != nil {
		panic("Init de test impossible : " + err.Error())
	}
	os.Exit(m.Run())
}

func TestEncryptDecryptRoundtrip(t *testing.T) {
	const motDePasse = `p@ssw0rd"difficile $avec métacaractères`
	enc := Encrypt(motDePasse)
	if !strings.HasPrefix(enc, Prefix) {
		t.Fatalf("la valeur chiffrée doit porter le préfixe %s : %s", Prefix, enc)
	}
	if !IsEncrypted(enc) {
		t.Fatal("IsEncrypted doit reconnaître la sortie d'Encrypt")
	}
	if got := Decrypt(enc); got != motDePasse {
		t.Fatalf("aller-retour cassé : %q ≠ %q", got, motDePasse)
	}
	// Le nonce est aléatoire : deux chiffrements du même texte diffèrent.
	if enc2 := Encrypt(motDePasse); enc2 == enc {
		t.Fatal("deux chiffrements du même texte doivent différer (nonce aléatoire)")
	}
}

func TestEncryptIdempotentAndEmpty(t *testing.T) {
	once := Encrypt("abc")
	if again := Encrypt(once); again != once {
		t.Fatal("Encrypt doit être idempotent sur une valeur déjà chiffrée (pas de double scellage)")
	}
	if e := Encrypt(""); e != "" {
		t.Fatalf("chaîne vide → chaîne vide, obtenu %q", e)
	}
	if d := Decrypt(""); d != "" {
		t.Fatalf("déverrouillage d'une chaîne vide → vide, obtenu %q", d)
	}
}

func TestDecryptPassthroughLegacy(t *testing.T) {
	// Valeur SANS préfixe = donnée antérieure au correctif : renvoyée telle
	// quelle (la migration se fait à l'écriture).
	for _, legacy := range []string{"admin", "mot-de-passe simple", "enc:v2:future"} {
		if got := Decrypt(legacy); got != legacy {
			t.Fatalf("passage sans préfixe altéré : %q → %q", legacy, got)
		}
	}
}

func TestDecryptCorrupt(t *testing.T) {
	valid := Encrypt("secret-routeur")[len(Prefix):] // payload base64url valide

	cas := map[string]string{
		"base64 illisible":  Prefix + "%%%pas-du-base64%%%",
		"trop court":        Prefix + base64.RawURLEncoding.EncodeToString([]byte("nonce!")), // ≤ 12 octets
		"ciphertext altéré": Prefix + alterB64(t, valid),
		"charge utile vide": Prefix,
		"bloc tronqué":      Prefix + valid[:len(valid)-5],
	}
	for nom, valeur := range cas {
		if got := Decrypt(valeur); got != "" {
			t.Fatalf("%s : une valeur chiffrée corrompue doit renvoyer \"\" (jamais le ciphertext), obtenu %q", nom, got)
		}
	}
}

// alterB64 retourne le même payload base64url avec UN octet du ciphertext
// inversé (altération détectée par le tag GCM).
func alterB64(t *testing.T, b64 string) string {
	t.Helper()
	raw, err := base64.RawURLEncoding.DecodeString(b64)
	if err != nil {
		t.Fatalf("payload de référence illisible : %v", err)
	}
	raw[len(raw)-1] ^= 0xFF
	return base64.RawURLEncoding.EncodeToString(raw)
}

func TestWrongKeyRejected(t *testing.T) {
	enc := Encrypt("mot-de-passe-routeur")
	// Rotation du secret JWT : la clé dérivée change, l'ancien ciphertext
	// devient indéchiffrable → chaîne vide (connexion routeur à re-saisir).
	if err := Init("secret-jwt-ROTATION"); err != nil {
		t.Fatalf("re-Init impossible : %v", err)
	}
	if got := Decrypt(enc); got != "" {
		t.Fatalf("avec une autre clé, Decrypt doit renvoyer \"\" (jamais le ciphertext), obtenu %q", got)
	}
	// Restauration de la clé du TestMain pour les tests suivants.
	if err := Init(testSecret); err != nil {
		t.Fatalf("restauration de la clé impossible : %v", err)
	}
	if got := Decrypt(enc); got != "mot-de-passe-routeur" {
		t.Fatalf("après restauration de la clé, le déchiffrement doit refonctionner, obtenu %q", got)
	}
}

func TestInitWithCredentialsKey(t *testing.T) {
	cle := hex.EncodeToString(make([]byte, 32)) // 64 hex valides
	t.Setenv("CREDENTIALS_KEY", cle)
	if err := Init(testSecret); err != nil {
		t.Fatalf("CREDENTIALS_KEY hex 64 doit être acceptée : %v", err)
	}
	t.Setenv("CREDENTIALS_KEY", "zz-pas-du-hex")
	if err := Init(testSecret); err == nil {
		t.Fatal("une CREDENTIALS_KEY non hexadécimale doit être refusée")
	}
	t.Setenv("CREDENTIALS_KEY", "abcd") // 2 octets ≠ 32
	if err := Init(testSecret); err == nil {
		t.Fatal("une CREDENTIALS_KEY trop courte doit être refusée")
	}
	// Restauration pour les tests suivants.
	os.Unsetenv("CREDENTIALS_KEY")
	if err := Init(testSecret); err != nil {
		t.Fatalf("restauration de la clé impossible : %v", err)
	}
}
