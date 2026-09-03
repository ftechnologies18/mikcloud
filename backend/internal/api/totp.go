// totp.go — authentification à deux facteurs TOTP (sécurité S4 — P1 de
// l'audit pré-lancement commercial).
//
// Implémentation RFC 6238 (TOTP = HOTP RFC 4226 sur un compteur temporel) :
// HMAC-SHA1, période de 30 s, 6 chiffres, fenêtre de vérification ±1
// période (tolérance au décalage d'horloge des téléphones). Le secret fait
// 160 bits (20 octets aléatoires) encodés en base32 — saisie manuelle ou
// URL otpauth:// dans toute application authenticator. La comparaison des
// codes est constant-time (crypto/subtle).
//
// Cycle de vie (volontairement simple, auditable, sans dépendance) :
//   - POST /api/auth/2fa/setup    (session JWT) : génère un secret, le
//     stocke SANS l'activer — retourne le secret + l'URL otpauth ;
//   - POST /api/auth/2fa/activate (session JWT) : vérifie un premier code
//     contre le secret en attente puis active la 2FA ;
//   - POST /api/auth/2fa/disable  (session JWT + mot de passe) : désactive
//     et efface le secret ;
//   - POST /api/auth/login : si l'utilisateur a activé la 2FA, le champ
//     « code » est requis — sans code, 401 + code machine « totp_required »
//     (l'écran de connexion demande alors le code) ; code erroné → 400
//     générique « Identifiants invalides » (aucun oracle) + journal S2
//     (raison fine bad_totp côté serveur).
//
// Le secret TOTP n'est JAMAIS sérialisé en JSON (tag json:"-" sur le champ
// du modèle). La 2FA n'est pas écrasée par les réinitialisations de mot de
// passe : elle reste une protection indépendante du canal mot de passe.
// Limite v1 assumée : un utilisateur qui perd son téléphone doit faire
// effacer son secret par l'opérateur plateforme (procédure du runbook
// secrets) — il n'existe pas de codes de secours àUsage unique.
//
// Anti brute-force : /api/auth/* est déjà bornée à 12 req/min par IP
// (middleware S1-A2) ; l'espace du code est de 10^6 par fenêtre de 30 s —
// sous 12 essais/min, la probabilité de trouver un code valide dans la
// fenêtre de ±1 reste négligeable (< 0,004 % par fenêtre).
package api

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"mikcloud/hotspot-api/internal/auth"
	"mikcloud/hotspot-api/internal/model"
)

const (
	// totpPeriod — période de rotation des codes (RFC 6238 recommande 30 s).
	totpPeriod int64 = 30

	// totpDigits — longueur du code (standard des authenticators).
	totpDigits = 6

	// totpWindow — tolérance : le code de la période courante et des
	// périodes ±1 est accepté (décalage d'horloge, latence de saisie).
	totpWindow int64 = 1

	// totpSecretBytes — entropie du secret : 160 bits (RFC 4226 §4).
	totpSecretBytes = 20
)

var totpB32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// newTOTPSecret — secret aléatoire en base32 (sans padding, format des
// authenticators).
func newTOTPSecret() (string, error) {
	raw := make([]byte, totpSecretBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return totpB32.EncodeToString(raw), nil
}

// totpCodeAt — code 6 chiffres attendu à l'instant unix donné.
func totpCodeAt(secretB32 string, unix int64) (string, error) {
	key, err := totpB32.DecodeString(strings.ToUpper(strings.ReplaceAll(secretB32, " ", "")))
	if err != nil {
		return "", fmt.Errorf("secret base32 invalide")
	}
	counter := uint64(unix / totpPeriod)
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], counter)
	mac := hmac.New(sha1.New, key)
	mac.Write(buf[:])
	sum := mac.Sum(nil)
	// Troncature dynamique RFC 4226 §5.3.
	offset := sum[len(sum)-1] & 0x0f
	bin := uint64(sum[offset]&0x7f)<<24 |
		uint64(sum[offset+1])<<16 |
		uint64(sum[offset+2])<<8 |
		uint64(sum[offset+3])
	return fmt.Sprintf("%0*d", totpDigits, bin%1_000_000), nil
}

// verifyTOTP — accepte le code de la période courante et des périodes
// adjacentes ; comparaison constant-time.
func verifyTOTP(secretB32, code string) bool {
	code = strings.TrimSpace(code)
	if len(code) != totpDigits {
		return false
	}
	now := time.Now().Unix()
	for _, d := range []int64{0, -1, 1} {
		if d > totpWindow || d < -totpWindow {
			continue
		}
		want, err := totpCodeAt(secretB32, now+d*totpPeriod)
		if err != nil {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(want), []byte(code)) == 1 {
			return true
		}
	}
	return false
}

// otpauthURL — URI de pairage standard (saisie manuelle ou QR dans
// l'authenticator).
func otpauthURL(secret, username string) string {
	label := url.PathEscape("MikCloud") + ":" + url.PathEscape(username)
	return fmt.Sprintf("otpauth://totp/%s?secret=%s&issuer=MikCloud&algorithm=SHA1&digits=6&period=30", label, secret)
}

// ---------------------------------------------------------------------------
// Endpoints
// ---------------------------------------------------------------------------

// handleTOTPSetup — POST /api/auth/2fa/setup : génère un nouveau secret
// (état « en attente »), retourne le secret + URL otpauth. Un setup
// précédent non activé est écrasé.
func (a *API) handleTOTPSetup(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r)
	if claims == nil {
		writeErr(w, http.StatusUnauthorized, "Token invalide ou expiré")
		return
	}
	secret, err := newTOTPSecret()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Erreur interne")
		return
	}
	a.store.Lock()
	db := a.store.Data()
	found := false
	for i := range db.Users {
		if db.Users[i].ID == claims.Sub {
			db.Users[i].TOTPSecret = secret
			found = true
			break
		}
	}
	if found {
		a.store.Save()
	}
	a.store.Unlock()
	if !found {
		writeErr(w, http.StatusUnauthorized, "Token invalide ou expiré")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"secret":  secret,
		"otpauth": otpauthURL(secret, claims.Name),
	})
}

// handleTOTPActivate — POST /api/auth/2fa/activate : vérifie un premier
// code contre le secret en attente, puis active la 2FA.
func (a *API) handleTOTPActivate(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r)
	if claims == nil {
		writeErr(w, http.StatusUnauthorized, "Token invalide ou expiré")
		return
	}
	var req struct {
		Code string `json:"code"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	a.store.Lock()
	db := a.store.Data()
	var user *model.AdminUser
	for i := range db.Users {
		if db.Users[i].ID == claims.Sub {
			user = &db.Users[i]
			break
		}
	}
	if user == nil {
		a.store.Unlock()
		writeErr(w, http.StatusUnauthorized, "Token invalide ou expiré")
		return
	}
	if user.TOTPSecret == "" || !verifyTOTP(user.TOTPSecret, req.Code) {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, "Code invalide — vérifiez votre application d'authentification")
		return
	}
	user.TOTPEnabled = true
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "totpEnabled": true})
}

// handleTOTPDisable — POST /api/auth/2fa/disable : exige le mot de passe
// courant (une session volée ne suffit pas), désactive et efface le secret.
func (a *API) handleTOTPDisable(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r)
	if claims == nil {
		writeErr(w, http.StatusUnauthorized, "Token invalide ou expiré")
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	if req.Password == "" {
		writeErr(w, http.StatusBadRequest, "Le mot de passe est requis")
		return
	}
	a.store.Lock()
	db := a.store.Data()
	var user *model.AdminUser
	for i := range db.Users {
		if db.Users[i].ID == claims.Sub {
			user = &db.Users[i]
			break
		}
	}
	// Message unique pour utilisateur inconnu et mot de passe incorrect.
	if user == nil || !auth.CheckPassword(req.Password, user.Salt, user.PasswordHash) {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, "Mot de passe incorrect")
		return
	}
	user.TOTPEnabled = false
	user.TOTPSecret = ""
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "totpEnabled": false})
}
