// Tests du package auth — JWT HS256 maison + hachage de mots de passe.
//
// Couverture :
//   - Sign/Verify : aller-retour avec claims complets, structure du token
//     (3 segments, en-tête HS256) ;
//   - rejets : token expiré, mauvais secret, payload altéré, format invalide,
//     base64/json illisible ;
//   - mots de passe : HashPassword (bcrypt coût 12, sel ignoré), CheckPassword
//     (bcrypt + migration ancien hash hexadécimal sha256(salt||password)),
//     IsLegacyHash, NewSalt.
package auth

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// claimsT — claims de référence pour les tests.
func claimsT() Claims {
	now := time.Now().Unix()
	return Claims{Sub: "usr-123", Name: "Awa Diop", Role: "owner", Acc: "acc-abc", Iat: now, Exp: now + 3600}
}

func TestSignVerifyRoundtrip(t *testing.T) {
	const secret = "secret-de-test-mikcloud"
	c := claimsT()
	tok := Sign(secret, c)

	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatalf("le token doit avoir 3 segments, obtenu %d", len(parts))
	}
	// L'en-tête DOIT déclarer HS256 (l'algorithme n'est pas lu au verify —
	// la vérification est toujours HMAC-SHA256 : aucun risque d'« alg:none »).
	hdr, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatalf("en-tête illisible : %v", err)
	}
	if !strings.Contains(string(hdr), "HS256") {
		t.Fatalf("en-tête inattendu : %s", hdr)
	}

	got, err := Verify(secret, tok)
	if err != nil {
		t.Fatalf("Verify d'un token frais a échoué : %v", err)
	}
	if got.Sub != c.Sub || got.Name != c.Name || got.Role != c.Role || got.Acc != c.Acc {
		t.Fatalf("claimsaltérés : %+v ≠ %+v", got, c)
	}
	if got.Iat != c.Iat || got.Exp != c.Exp {
		t.Fatalf("horodatages altérés : iat=%d exp=%d ≠ %d/%d", got.Iat, got.Exp, c.Iat, c.Exp)
	}
}

func TestNewClaims(t *testing.T) {
	before := time.Now().Unix()
	c := NewClaims("usr-1", "Gérant", "owner", "acc-9")
	after := time.Now().Unix()
	if c.Exp-c.Iat != int64(TokenTTL.Seconds()) {
		t.Fatalf("durée de vie = %d s, attendu %d", c.Exp-c.Iat, int64(TokenTTL.Seconds()))
	}
	if c.Iat < before || c.Iat > after {
		t.Fatalf("iat %d hors fenêtre [%d,%d]", c.Iat, before, after)
	}
}

func TestVerifyExpired(t *testing.T) {
	const secret = "secret-de-test-mikcloud"
	c := claimsT()
	c.Exp = time.Now().Unix() - 60 // expiré depuis 1 min
	tok := Sign(secret, c)
	if _, err := Verify(secret, tok); err == nil {
		t.Fatal("un token expiré doit être refusé")
	}
}

func TestVerifyWrongSecret(t *testing.T) {
	c := claimsT()
	tok := Sign("autre-secret", c)
	if _, err := Verify("secret-de-test-mikcloud", tok); err == nil {
		t.Fatal("un token signé avec un autre secret doit être refusé")
	}
}

func TestVerifyTamperedPayload(t *testing.T) {
	const secret = "secret-de-test-mikcloud"
	tok := Sign(secret, claimsT())
	parts := strings.Split(tok, ".")

	// Payload altéré ( élévation de rôle) signé par l'ANCIENNE signature.
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("payload illisible : %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(payload, &m); err != nil {
		t.Fatalf("payload non JSON : %v", err)
	}
	m["role"] = "platform_admin"
	forged, _ := json.Marshal(m)
	evil := parts[0] + "." + base64.RawURLEncoding.EncodeToString(forged) + "." + parts[2]
	if _, err := Verify(secret, evil); err == nil {
		t.Fatal("un payload altéré sans re-signature doit être refusé")
	}

	// Signature elle-même corrompue (1 caractère changé).
	sig := []byte(parts[2])
	if sig[0] == 'A' {
		sig[0] = 'B'
	} else {
		sig[0] = 'A'
	}
	if _, err := Verify(secret, parts[0]+"."+parts[1]+"."+string(sig)); err == nil {
		t.Fatal("une signature corrompue doit être refusée")
	}
}

func TestVerifyMalformed(t *testing.T) {
	const secret = "secret-de-test-mikcloud"
	cas := map[string]string{
		"chaîne vide":            "",
		"sans signature":         "aaa.bbb",
		"quatre segments":        "a.b.c.d",
		"signature non base64":   "eyJ9.eyJ9.%%%%",
		"payload non base64":     "eyJ9.%%%%.eyJ9",
		"payload non JSON":       "eyJ9." + base64.RawURLEncoding.EncodeToString([]byte("pas-du-json")) + ".eyJ9",
		"header.payload valable": "eyJ9." + base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"x"}`)) + ".eyJ9",
	}
	for name, tok := range cas {
		if _, err := Verify(secret, tok); err == nil {
			t.Fatalf("%s : un token malformé doit être refusé", name)
		}
	}
}

func TestHashPasswordBcrypt(t *testing.T) {
	h := HashPassword("mot-de-passe-sûr", "sel-ignoré")
	if !strings.HasPrefix(h, "$2") {
		t.Fatalf("le hash doit être bcrypt ($2…), obtenu %.10s", h)
	}
	// Le paramètre salt est ignoré (compatibilité d'appel) : deux appels avec
	// des sels différents produisent deux hash vérifiables du même mot de passe.
	h2 := HashPassword("mot-de-passe-sûr", "autre-sel")
	if !CheckPassword("mot-de-passe-sûr", "peu-importe", h) {
		t.Fatal("CheckPassword doit valider le bon mot de passe (hash 1)")
	}
	if !CheckPassword("mot-de-passe-sûr", "", h2) {
		t.Fatal("CheckPassword doit valider le bon mot de passe (hash 2)")
	}
	if CheckPassword("mauvais", "", h) {
		t.Fatal("CheckPassword doit refuser un mauvais mot de passe")
	}
	if h == h2 {
		t.Fatal("bcrypt embarque un sel : deux hash du même mot de passe diffèrent")
	}
}

func TestCheckPasswordLegacySHA256(t *testing.T) {
	// Ancien format pré-bcrypt : hex(sha256(salt||password)).
	salt := "sel-ancien"
	sum := sha256.Sum256([]byte(salt + "ancien-mdp"))
	legacy := hex.EncodeToString(sum[:])

	if !CheckPassword("ancien-mdp", salt, legacy) {
		t.Fatal("le hash legacy doit être validé avec le bon sel")
	}
	if CheckPassword("ancien-mdp", "autre-sel", legacy) {
		t.Fatal("le hash legacy doit échouer avec un mauvais sel")
	}
	if CheckPassword("mauvais", salt, legacy) {
		t.Fatal("le hash legacy doit échouer avec un mauvais mot de passe")
	}
	if !IsLegacyHash(legacy) {
		t.Fatal("un hash hexadécimal doit être signalé legacy")
	}
	if IsLegacyHash(HashPassword("x", "")) {
		t.Fatal("un hash bcrypt ne doit PAS être signalé legacy")
	}
}

func TestNewSalt(t *testing.T) {
	s1, s2 := NewSalt(), NewSalt()
	if len(s1) != 32 { // 16 octets → 32 caractères hex
		t.Fatalf("longueur du sel = %d, attendu 32", len(s1))
	}
	if _, err := hex.DecodeString(s1); err != nil {
		t.Fatalf("le sel doit être hexadécimal : %v", err)
	}
	if s1 == s2 {
		t.Fatal("deux sels successifs doivent différer")
	}
}
