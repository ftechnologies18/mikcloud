// password_reset.go — N°68 : « Mot de passe oublié ? » de l'écran de connexion.
//
// Parcours complet, 100 % public (aucun token JWT — le secret voyage dans le
// lien e-mail) :
//  1. POST /api/auth/forgot-password {email} — l'utilisateur saisit l'e-mail
//     enregistré à la création de SON compte. Si l'e-mail correspond à un
//     compte : génération d'un token aléatoire 256 bits, stocké HASHÉ
//     (SHA-256), expiration 60 minutes, puis e-mail de réinitialisation via
//     le canal e-mail du compte (Resend ou SMTP — N°67) ou, à défaut, celui
//     du compte principal (plateforme). Sinon : réponse explicite « aucun
//     compte associé » (choix produit assumé : l'utilisateur est GUIDÉ, pas
//     laissé dans le doute — le quota IP borne le sondage d'adresses).
//  2. POST /api/auth/reset-password {token, password} — consommation du lien
//     : usage UNIQUE (UsedAt), expiration stricte, politique S2 (10
//     caractères + denylist + ≠ username), révocation de TOUTES les sessions
//     (SessionEpoch++), journal d'activité.
//
// Sécurité :
//   - le token en clair n'est JAMAIS persisté (une fuite de la base ne permet
//     aucune réutilisation) ;
//   - une nouvelle demande INVALIDE les liens en attente du même compte (un
//     seul lien vivant par compte) ;
//   - quota IP (5/10 min + 20/24 h, même limiteur que l'inscription) : les
//     e-mails de réinitialisation sont des envois réels — le flood est coupé
//     avant Resend ;
//   - l'origine du lien (frontend) est validée : APP_PUBLIC_URL > origine de
//     la requête SI elle figure dans ALLOWED_ORIGIN > URL canonique (jamais
//     une origine inconnue — anti-phishing du lien) ;
//   - l'envoi réseau se fait HORS verrou du store (même règle que le moniteur
//     de notifications).
package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"mikcloud/hotspot-api/internal/auth"
	"mikcloud/hotspot-api/internal/model"
	"mikcloud/hotspot-api/internal/notify"
)

// passwordResetTTL — durée de validité du lien (60 minutes : assez pour
// relancer son client mail, assez court pour borner la fenêtre d'attaque si
// l'e-mail est intercepté).
const passwordResetTTL = 60 * time.Minute

// passwordResetRetention — fenêtre d'audit : les lignes (utilisées ou
// expirées) sont purgées après 24 h — le registre reste borné sans cron.
const passwordResetRetention = 24 * time.Hour

// defaultFrontendURL — origine canonique du frontend (dernier repli quand ni
// APP_PUBLIC_URL ni l'origine ALLOWED de la requête ne sont disponibles —
// requête forgée sans Origin : le lien pointe sur le vrai site, jamais celui
// de l'appelant).
const defaultFrontendURL = "https://mikcloud.ftci.fr"

// generateResetToken — token de lien (32 octets aléatoires en base64url).
// Variable de package : les tests l'écrasent par une valeur déterministe.
var generateResetToken = func() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// sendResetEmail — envoi de l'e-mail (indirection pour les tests : aucun
// réseau en CI).
var sendResetEmail = notify.SendEmailTo

// hashResetToken — SHA-256 hex du token (seule forme persistée).
func hashResetToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// passwordResetLinkBase — origine du frontend pour construire le lien
// /reset-password. Ordre de résolution :
//  1. APP_PUBLIC_URL (env, si définie) ;
//  2. l'origine de la requête SI elle figure dans ALLOWED_ORIGIN (en prod,
//     c'est l'URL Vercel du frontend — validée, donc jamais une origine
//     forgée par un attaquant qui viserait un faux site de réinitialisation) ;
//     en dev local (pas de DATABASE_URL), toute origine est acceptée ;
//  3. l'URL canonique du frontend.
func passwordResetLinkBase(r *http.Request) string {
	if v := strings.TrimRight(strings.TrimSpace(os.Getenv("APP_PUBLIC_URL")), "/"); v != "" {
		return v
	}
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin != "" && (strings.HasPrefix(origin, "https://") || strings.HasPrefix(origin, "http://")) {
		if os.Getenv("DATABASE_URL") == "" {
			return strings.TrimRight(origin, "/")
		}
		for _, o := range strings.Split(os.Getenv("ALLOWED_ORIGIN"), ",") {
			if strings.TrimSpace(o) == origin {
				return origin
			}
		}
	}
	return defaultFrontendURL
}

// prunePasswordResetsLocked — purge paresseuse (appelée sous verrou à chaque
// demande) : les lignes de plus de 24 h — consommées, expirées ou en attente —
// n'ont plus de valeur (l'expiration est de 60 min), le registre reste borné.
func prunePasswordResetsLocked(db *model.DB) {
	if len(db.PasswordResets) == 0 {
		return
	}
	cutoff := time.Now().UTC().Add(-passwordResetRetention)
	fresh := db.PasswordResets[:0]
	for _, pr := range db.PasswordResets {
		created, err := time.Parse(time.RFC3339, pr.CreatedAt)
		if err == nil && created.Before(cutoff) {
			continue
		}
		fresh = append(fresh, pr)
	}
	db.PasswordResets = fresh
}

// handleForgotPassword — POST /api/auth/forgot-password {email}.
func (a *API) handleForgotPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	email := strings.TrimSpace(strings.ToLower(req.Email))
	if email == "" || !emailRegexp.MatchString(email) {
		writeErr(w, http.StatusBadRequest, "Adresse e-mail invalide")
		return
	}
	// Quota IP (S3, même limiteur que l'inscription) : chaque tentative —
	// valide ou non — consomme le quota, le flood d'e-mails réels est coupé
	// avant d'atteindre Resend/SMTP.
	if ok, retry := a.reset.allow(clientIP(r)); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
		writeErr(w, http.StatusTooManyRequests, "Trop de demandes de réinitialisation — réessayez plus tard")
		return
	}

	// ---- Phase 1 (verrou) : compte, propriétaire, fournisseur e-mail, token.
	a.store.Lock()
	db := a.store.Data()
	prunePasswordResetsLocked(db)
	var acc *model.Account
	for i := range db.Accounts {
		if strings.EqualFold(strings.TrimSpace(db.Accounts[i].Email), email) {
			acc = &db.Accounts[i]
			break
		}
	}
	// L'utilisateur a demandé un signal explicite : e-mail inconnu → 404
	// (le quota IP borne le sondage d'adresses, le fermage est ailleurs).
	if acc == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Aucun compte n'est associé à cet e-mail")
		return
	}
	if acc.Status == "disabled" {
		a.store.Unlock()
		writeErr(w, http.StatusForbidden, "Compte désactivé — contactez le support")
		return
	}
	// Le mot de passe réinitialisé est celui du PROPRIÉTAIRE du compte (le
	// porteur de l'e-mail d'inscription) — les membres d'équipe sans e-mail
	// passent par leur gérant (parcours SaaS standard).
	var owner *model.AdminUser
	for i := range db.Users {
		u := &db.Users[i]
		if u.AccountID == acc.ID && u.Role == model.RoleOwner {
			owner = u
			break
		}
	}
	if owner == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Aucun compte n'est associé à cet e-mail")
		return
	}
	// Fournisseur e-mail : réglages du compte demandeur, sinon ceux du compte
	// principal (plateforme — en production, Resend y est configuré depuis
	// N°67 : les clients n'ont RIEN à régler pour que le lien parte).
	var senderCfg model.NotificationSettings
	senderOK := false
	if cfg, ok := db.NotifSettings[acc.ID]; ok && notify.EmailCredentialsOK(&cfg) {
		senderCfg, senderOK = cfg, true
	} else if cfg, ok := db.NotifSettings[model.AccountMainID]; ok && notify.EmailCredentialsOK(&cfg) {
		senderCfg, senderOK = cfg, true
	}
	if !senderOK {
		a.store.Unlock()
		writeErr(w, http.StatusServiceUnavailable, "Envoi d'e-mail indisponible — contactez le support")
		return
	}
	token, err := generateResetToken()
	if err != nil {
		a.store.Unlock()
		writeErr(w, http.StatusInternalServerError, "Génération du lien impossible — réessayez")
		return
	}
	now := time.Now().UTC()
	// Une nouvelle demande invalide les liens en attente du compte : UN SEUL
	// lien vivant (l'ancien e-mail, s'il arrive après, mène à un lien mort).
	fresh := db.PasswordResets[:0]
	for _, pr := range db.PasswordResets {
		if pr.AccountID != acc.ID {
			fresh = append(fresh, pr)
		}
	}
	db.PasswordResets = fresh
	db.PasswordResets = append(db.PasswordResets, model.PasswordReset{
		ID:        model.NewID("pr-"),
		AccountID: acc.ID,
		UserID:    owner.ID,
		TokenHash: hashResetToken(token),
		ExpiresAt: now.Add(passwordResetTTL).Format(time.RFC3339),
		CreatedAt: now.Format(time.RFC3339),
		CreatedIP: clientIP(r),
	})
	a.logActivityBy(r, db, acc.ID, "system", "Demande de réinitialisation de mot de passe («"+owner.Username+"»)")
	a.store.Save()
	accEmail, accName, ownerName, ownerUsername := acc.Email, acc.Name, owner.Name, owner.Username
	a.store.Unlock()

	// ---- Phase 2 (hors verrou) : envoi réseau.
	link := passwordResetLinkBase(r) + "/reset-password?token=" + url.QueryEscape(token)
	title := "MikCloud — Réinitialisation de votre mot de passe"
	var body strings.Builder
	body.WriteString("Bonjour " + ownerName + ",\n\n")
	body.WriteString("Vous (ou quelqu'un utilisant cette adresse) avez demandé la réinitialisation du mot de passe du compte MikCloud « " + accName + " » (identifiant : " + ownerUsername + ").\n\n")
	body.WriteString("Choisissez votre nouveau mot de passe via ce lien, valable " + minutesLabel(passwordResetTTL) + " et utilisable une seule fois :\n\n")
	body.WriteString(link + "\n\n")
	body.WriteString("Si vous n'êtes pas à l'origine de cette demande, ignorez simplement cet e-mail : votre mot de passe actuel reste inchangé.\n\n")
	body.WriteString("— MikCloud")
	err = sendResetEmail(&senderCfg, accEmail, title, body.String())

	// ---- Phase 3 (verrou) : trace d'historique (même format que N°67).
	entry := notify.LogEntry(&senderCfg, "email", notify.KindPasswordReset, title, "", err)
	a.store.Lock()
	entry.ID = model.NewID("n-")
	entry.At = model.NowISO()
	entry.Body = "Demande de « mot de passe oublié » pour " + ownerUsername
	db = a.store.Data()
	db.NotifLog = append(db.NotifLog, entry)
	a.store.Save()
	a.store.Unlock()

	if err != nil {
		writeErr(w, http.StatusBadGateway, "E-mail non envoyé — réessayez plus tard")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "expiresInMin": int(passwordResetTTL.Minutes())})
}

// minutesLabel — « 60 minutes » (libellé du corps d'e-mail, en français —
// contractuel avec l'i18n du front qui affiche la même durée).
func minutesLabel(d time.Duration) string {
	return strconv.Itoa(int(d.Minutes())) + " minutes"
}

// handleResetPassword — POST /api/auth/reset-password {token, password}.
// Consomme le lien : usage unique, expiration stricte, politique S2,
// révocation de toutes les sessions.
func (a *API) handleResetPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	token := strings.TrimSpace(req.Token)
	if token == "" {
		writeErr(w, http.StatusBadRequest, "Lien de réinitialisation manquant")
		return
	}
	if req.Password == "" {
		writeErr(w, http.StatusBadRequest, "Nouveau mot de passe requis")
		return
	}
	hash := hashResetToken(token)
	a.store.Lock()
	db := a.store.Data()
	prunePasswordResetsLocked(db)
	var row *model.PasswordReset
	for i := range db.PasswordResets {
		if db.PasswordResets[i].TokenHash == hash {
			row = &db.PasswordResets[i]
			break
		}
	}
	// Token inconnu = lien déjà consommé ET purgé, jamais demandé, ou forgé :
	// réponse unique (aucun oracle sur l'état du registre).
	if row == nil {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, "Lien invalide ou déjà utilisé — demandez un nouveau lien")
		return
	}
	if row.UsedAt != "" {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, "Ce lien a déjà été utilisé — demandez un nouveau lien")
		return
	}
	if exp, err := time.Parse(time.RFC3339, row.ExpiresAt); err == nil && time.Now().UTC().After(exp) {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, "Ce lien a expiré — demandez un nouveau lien")
		return
	}
	var user *model.AdminUser
	for i := range db.Users {
		if db.Users[i].ID == row.UserID {
			user = &db.Users[i]
			break
		}
	}
	if user == nil {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, "Lien invalide ou déjà utilisé — demandez un nouveau lien")
		return
	}
	// Politique S2 centralisée (10 caractères + denylist + ≠ identifiant).
	if msg := passwordPolicyViolation(req.Password, user.Username); msg != "" {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	user.PasswordHash = auth.HashPassword(req.Password, "") // bcrypt : sel intégré
	user.Salt = ""
	user.PasswordSetByUser = true // protège contre l'override ADMIN_PASSWORD
	user.SessionEpoch++           // S1-A3 — révoque TOUTES les sessions
	row.UsedAt = model.NowISO()   // usage unique : le lien meurt ici
	a.logActivityBy(r, db, user.AccountID, "system", "Mot de passe réinitialisé via le lien e-mail («"+user.Username+"»)")
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
