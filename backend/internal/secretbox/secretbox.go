// Package secretbox — chiffrement au repos des secrets sensibles (audit P0 #6).
//
// Périmètre : les identifiants d'accès aux routeurs (model.Router.Password,
// stockés en clair dans PostgreSQL et db.json avant ce correctif). Une fuite
// de base de données (dump, snapshot, compromission de l'hébergeur DB) ne
// doit jamais livrer les mots de passe RouterOS des clients.
//
// Principe : AES-256-GCM (authentifié), clé 32 octets dérivée —
//   - soit de CREDENTIALS_KEY (hex 64) si elle est définie (rotation
//     possible sans toucher au secret JWT) ;
//   - sinon de JWT_SECRET via HKDF-SHA256 (sel « mikcloud-secretbox »,
//     info « mikcloud/credentials/v1 » — domaine distinct de toute autre
//     dérivation). En production JWT_SECRET est déjà obligatoire (P0 #1).
//
// Format stocké : "enc:v1:" + base64url(nonce[12] || ciphertext+tag).
// Décodage tolérant : une valeur SANS préfixe (données antérieures au
// correctif) est renvoyée telle quelle — la migration se fait à l'écriture
// et par la passe de démarrage ; une valeur chiffrée invalide est renvoyée
// vide (jamais le ciphertext brut).
//
// Ce paquet ne dépend que de la stdlib + x/crypto/hkdf (déjà requise).
package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"golang.org/x/crypto/hkdf"
)

const (
	// Prefix — marqueur de version du format chiffré (le « v1 » permet une
	// évolution future sans ambiguïté).
	Prefix = "enc:v1:"

	// keyInfo / keySalt — domaine de dérivation HKDF (RFC 5869) : sépare
	// strictement cette clé de toute autre clé dérivée du même secret.
	keyInfo = "mikcloud/credentials/v1"
	keySalt = "mikcloud-secretbox"

	nonceSize = 12 // taille standard AES-GCM
)

var (
	gcm      cipher.AEAD
	ready    = false
	errNoKey = errors.New("secretbox : aucune source de clé (CREDENTIALS_KEY ou JWT_SECRET)")
)

// Init résout la clé de chiffrement et prépare l'AES-GCM. À appeler UNE fois
// au démarrage, avant toute lecture/écriture du store. En production
// (DATABASE_URL définie), l'absence totale de source de clé est fatale —
// même politique fail-fast que JWT_SECRET (P0 #1).
func Init(jwtSecret string) error {
	if key := os.Getenv("CREDENTIALS_KEY"); strings.TrimSpace(key) != "" {
		raw, err := hex.DecodeString(strings.TrimSpace(key))
		if err != nil || len(raw) != 32 {
			return fmt.Errorf("CREDENTIALS_KEY doit être 64 caractères hexadécimaux (256 bits), reçue %d octets valides", len(raw))
		}
		return start(raw)
	}
	if jwtSecret != "" {
		// HKDF-SHA256 : extract (sel) puis expand (info). Un domaine distinct
		// garantit qu'un attaquant ne peut pas recycler une autre dérivation
		// du même secret.
		r := hkdf.New(sha256.New, []byte(jwtSecret), []byte(keySalt), []byte(keyInfo))
		raw := make([]byte, 32)
		if _, err := io.ReadFull(r, raw); err != nil {
			return fmt.Errorf("dérivation de la clé impossible : %w", err)
		}
		return start(raw)
	}
	if os.Getenv("DATABASE_URL") != "" {
		return errNoKey
	}
	// Développement local sans aucun secret : clé éphémère (les credentials
	// chiffrés ne survivent pas au redémarrage — tolérable en dev, et
	// signalé bruyamment).
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return fmt.Errorf("clé éphémère impossible : %w", err)
	}
	log.Println("AVERTISSEMENT : ni CREDENTIALS_KEY ni JWT_SECRET — clé de chiffrement ÉPHÉMÈRE (dev uniquement)")
	return start(raw)
}

func start(key []byte) error {
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	g, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	gcm = g
	ready = true
	return nil
}

// IsEncrypted — true si la valeur porte le marqueur du format actuel.
func IsEncrypted(s string) bool {
	return strings.HasPrefix(s, Prefix)
}

// Encrypt chiffre une valeur au repos. Chaîne vide → chaîne vide. La valeur
// renvoyée commence par Prefix ; en mémoire (models, API) le mot de passe
// reste EN CLAIR : le chiffrement n'existe qu'aux frontières de persistance
// (pg.go / store.go).
func Encrypt(plain string) string {
	if plain == "" || !ready {
		return plain
	}
	if IsEncrypted(plain) {
		return plain // déjà chiffré (idempotent — évite le double scellage)
	}
	nonce := make([]byte, nonceSize)
	if _, err := rand.Read(nonce); err != nil {
		// Sans nonce aléatoire il n'y a pas de chiffrement sûr : on refuse
		// d'écrire une valeur faussement protégée.
		log.Printf("secretbox: nonce impossible (%v) — valeur NON chiffrée refusée", err)
		return ""
	}
	return Prefix + base64.RawURLEncoding.EncodeToString(append(nonce, gcm.Seal(nil, nonce, []byte(plain), nil)...))
}

// Decrypt déchiffre une valeur lue du stockage. Une valeur sans préfixe
// (données antérieures au correctif) est renvoyée telle quelle — migration
// transparente. Une valeur chiffrée CORROMPUE (clé changée, troncature,
// altération) est renvoyée vide : jamais le ciphertext brut comme mot de
// passe (une connexion RouterOS échouera proprement).
func Decrypt(stored string) string {
	if stored == "" || !ready || !IsEncrypted(stored) {
		return stored
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(stored, Prefix))
	if err != nil || len(raw) <= nonceSize {
		log.Printf("secretbox: valeur chiffrée illisible — mot de passe vide renvoyé (connexion routeur à re-saisir)")
		return ""
	}
	plain, err := gcm.Open(nil, raw[:nonceSize], raw[nonceSize:], nil)
	if err != nil {
		log.Printf("secretbox: déchiffrement impossible (clé modifiée ?) — mot de passe vide renvoyé")
		return ""
	}
	return string(plain)
}
