// Package auth — JWT HS256 implémenté à la main (stdlib uniquement) + hash de mots de passe.
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// Claims — revendications du token : {sub, name, role, iat, exp}.
type Claims struct {
	Sub  string `json:"sub"`
	Name string `json:"name"`
	Role string `json:"role"`
	Iat  int64  `json:"iat"`
	Exp  int64  `json:"exp"`
}

// TokenTTL — durée de validité du token (24 h).
const TokenTTL = 24 * time.Hour

// NewClaims construit des claims frais.
func NewClaims(sub, name, role string) Claims {
	now := time.Now().Unix()
	return Claims{Sub: sub, Name: name, Role: role, Iat: now, Exp: now + int64(TokenTTL.Seconds())}
}

// Sign produit un JWT HS256 : base64url(header).base64url(payload).base64url(hmac).
func Sign(secret string, c Claims) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload, _ := json.Marshal(c)
	body := header + "." + base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return body + "." + sig
}

// Verify contrôle la signature et l'expiration d'un JWT.
func Verify(secret, token string) (*Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("format de token invalide")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(parts[0] + "." + parts[1]))
	want := mac.Sum(nil)
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, errors.New("signature illisible")
	}
	if subtle.ConstantTimeCompare(sig, want) != 1 {
		return nil, errors.New("signature invalide")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, errors.New("contenu du token illisible")
	}
	var c Claims
	if err := json.Unmarshal(payload, &c); err != nil {
		return nil, errors.New("contenu du token invalide")
	}
	if time.Now().Unix() > c.Exp {
		return nil, errors.New("token expiré")
	}
	return &c, nil
}

// NewSalt — OBSOLÈTE avec bcrypt (le sel est intégré au hash), conservé
// uniquement pour la compatibilité des appels historiques.
func NewSalt() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// HashPassword — bcrypt coût 12 (le sel est intégré au hash : le paramètre
// salt est ignoré, gardé pour la compatibilité des signatures d'appel).
func HashPassword(password, _ string) string {
	b, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return ""
	}
	return string(b)
}

// CheckPassword vérifie un mot de passe contre un hash bcrypt ($2a/$2b/$2y)
// ou un ancien hash hexadécimal sha256(salt||password) — migration transparente.
func CheckPassword(password, salt, expectedHash string) bool {
	if strings.HasPrefix(expectedHash, "$2") {
		return bcrypt.CompareHashAndPassword([]byte(expectedHash), []byte(password)) == nil
	}
	h := sha256.Sum256([]byte(salt + password))
	return subtle.ConstantTimeCompare([]byte(hex.EncodeToString(h[:])), []byte(expectedHash)) == 1
}

// IsLegacyHash — true si le hash n'est pas bcrypt (à re-hasher au prochain login).
func IsLegacyHash(hash string) bool {
	return !strings.HasPrefix(hash, "$2")
}
