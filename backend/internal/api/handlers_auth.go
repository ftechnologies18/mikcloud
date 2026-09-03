// handlers_auth.go — connexion, inscription, changement de mot de passe, session courante.

package api

import (
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"mikcloud/hotspot-api/internal/auth"
	"mikcloud/hotspot-api/internal/model"
	"mikcloud/hotspot-api/internal/store"
)

// dummyPasswordHash — hash bcrypt factice (sécurité S2) : utilisé quand
// l'identifiant soumis est inconnu, pour exécuter quand même une comparaison
// bcrypt — le temps de réponse d'un utilisateur inconnu égale celui d'un
// mauvais mot de passe (aucun oracle de timing sur l'énumération
// d'identifiants).
var dummyPasswordHash = auth.HashPassword("mikcloud-dummy-verification", "")

func (a *API) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		// Code — code TOTP à 6 chiffres (sécurité S4), requis uniquement
		// si l'utilisateur a activé la 2FA. Ignoré sinon.
		Code string `json:"code"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		writeErr(w, http.StatusBadRequest, "Nom d'utilisateur et mot de passe requis")
		return
	}
	a.store.Lock()
	var id, name, username, role, salt, hash, accID string
	var accName, accStatus string
	var epoch int
	var user *model.AdminUser
	for i := range a.store.Data().Users {
		u := &a.store.Data().Users[i]
		if strings.EqualFold(u.Username, req.Username) {
			id, name, username, role, salt, hash = u.ID, u.Name, u.Username, u.Role, u.Salt, u.PasswordHash
			accID = u.AccountID
			epoch = u.SessionEpoch
			user = u
			break
		}
	}
	if accID != "" {
		for i := range a.store.Data().Accounts {
			acc := &a.store.Data().Accounts[i]
			if acc.ID == accID {
				accName, accStatus = acc.Name, acc.Status
				break
			}
		}
	}
	a.store.Unlock()
	// Sécurité S2 — journal d'échec (cf. auth_audit.go) : la réponse reste
	// générique, la raison fine n'existe que côté serveur. Identifiant
	// inconnu : comparaison factice pour égaliser le temps de réponse.
	if id == "" {
		_ = auth.CheckPassword(req.Password, "", dummyPasswordHash)
		a.logAuthFailure(r, "console", req.Username, "unknown_user")
		writeErr(w, http.StatusBadRequest, "Identifiants invalides")
		return
	}
	if !auth.CheckPassword(req.Password, salt, hash) {
		a.logAuthFailure(r, "console", req.Username, "bad_password")
		writeErr(w, http.StatusBadRequest, "Identifiants invalides")
		return
	}
	// Sécurité S4 — 2FA TOTP : si activée, le code est requis. Sans code,
	// 401 + code machine « totp_required » (l'écran de connexion demande
	// alors la saisie) ; code erroné → réponse générique (aucun oracle) +
	// journal de raison fine côté serveur. Le contrôle arrive APRÈS la
	// vérification du mot de passe : aucun contour du mot de passe.
	if user != nil && user.TOTPEnabled {
		if strings.TrimSpace(req.Code) == "" {
			writeErrCode(w, http.StatusUnauthorized, "totp_required", "Code d'authentification à deux facteurs requis", nil)
			return
		}
		if !verifyTOTP(user.TOTPSecret, req.Code) {
			a.logAuthFailure(r, "console", req.Username, "bad_totp")
			writeErr(w, http.StatusBadRequest, "Identifiants invalides")
			return
		}
	}
	// Compte désactivé : le login est refusé même avec des identifiants valides.
	if accID != "" && accStatus == "disabled" {
		a.logAuthFailure(r, "console", req.Username, "disabled")
		writeErr(w, http.StatusForbidden, "Compte désactivé — contactez le support")
		return
	}
	// Migration transparente : ancien hash SHA-256 → bcrypt au premier login.
	if user != nil && auth.IsLegacyHash(hash) {
		a.store.Lock()
		user.PasswordHash = auth.HashPassword(req.Password, "")
		user.Salt = ""
		a.store.Save()
		a.store.Unlock()
	}
	// N°7 — audit : trace la connexion réussie (acteur = qui se connecte).
	a.store.Lock()
	a.logActivityBy(r, a.store.Data(), accID, "system", "Connexion de "+username+" («"+role+"»)")
	a.store.Save()
	a.store.Unlock()
	token := auth.Sign(a.secret, auth.NewClaims(id, name, role, accID, epoch))
	writeJSON(w, http.StatusOK, map[string]any{
		"token": token,
		"user": map[string]any{
			"id": id, "name": name, "username": username, "role": role,
			"accountId": accID, "accountName": accName,
			"totpEnabled": user != nil && user.TOTPEnabled,
		},
	})
}

// handlePasswordChange — POST /api/auth/password : l'utilisateur connecté
// modifie SON PROPRE mot de passe. Exige le mot de passe actuel (une session
// laissée ouverte ne suffit pas), la politique S2 (10 caractères minimum +
// denylist, cf. password_policy.go), différent de l'actuel.
// Le flag PasswordSetByUser protège le nouveau mot de passe contre l'override
// ADMIN_PASSWORD au prochain démarrage/reload (tant que l'opérateur ne change
// pas la variable). Sécurité S1-A3 : l'époque de session est incrémentée —
// TOUTES les sessions de l'utilisateur (y compris celle qui effectue la
// modification) sont révoquées immédiatement ; l'utilisateur se reconnecte
// avec le nouveau mot de passe.
func (a *API) handlePasswordChange(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r)
	if claims == nil {
		writeErr(w, http.StatusUnauthorized, "Token invalide ou expiré")
		return
	}
	var req struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	if req.CurrentPassword == "" || req.NewPassword == "" {
		writeErr(w, http.StatusBadRequest, "Mot de passe actuel et nouveau mot de passe requis")
		return
	}
	// Sécurité S2 — politique centralisée (username non encore chargé ici :
	// le contrôle ≠ identifiant est ignoré, longueur et denylist tiennent).
	if msg := passwordPolicyViolation(req.NewPassword, ""); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if req.NewPassword == req.CurrentPassword {
		writeErr(w, http.StatusBadRequest, "Le nouveau mot de passe doit être différent de l'actuel")
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
	// Message unique pour utilisateur inconnu et mot de passe incorrect (pas d'oracle).
	if user == nil || !auth.CheckPassword(req.CurrentPassword, user.Salt, user.PasswordHash) {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, "Mot de passe actuel incorrect")
		return
	}
	user.PasswordHash = auth.HashPassword(req.NewPassword, "") // bcrypt : sel intégré
	user.Salt = ""
	user.PasswordSetByUser = true
	user.SessionEpoch++ // S1-A3 — révoque TOUTES les sessions (dont la courante)
	a.logActivityBy(r, db, user.AccountID, "system", "Mot de passe modifié par "+user.Username)
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ---------------------------------------------------------------------------
// Dédoublonnage email/WhatsApp (sécurité S5 — anti-fermage d'essai)
// ---------------------------------------------------------------------------

// accountContactTaken — parcourt les comptes existants et dit si l'email ou le
// numéro WhatsApp demandé est déjà porté par un AUTRE compte. Comparaisons :
//   - email : trim + insensible à la casse (majuscules/minuscules d'un même
//     objet ne créent pas un compte neuf) ;
//   - WhatsApp : comparaison exacte des chiffres (la normalisation E.164 sans
//     « + » est faite en amont, 8–15 chiffres).
//
// excludeID laisse passer le compte lui-même (édition future du contact).
// Retourne (emailPris, whatsappPris). Limite assumée : « 0701020304 » et
// « +225 07 01 02 03 04 » saisis différemment restent deux formes distinctes —
// un fraudeur acharné peut jouer dessus, il reste borné par le quota
// d'inscription par IP (signup_abuse.go).
func accountContactTaken(accounts []model.Account, email, phone, excludeID string) (bool, bool) {
	e := strings.ToLower(strings.TrimSpace(email))
	p := strings.TrimSpace(phone)
	if e == "" && p == "" {
		return false, false
	}
	emailTaken, phoneTaken := false, false
	for i := range accounts {
		acc := &accounts[i]
		if acc.ID == excludeID {
			continue
		}
		if e != "" && strings.EqualFold(strings.TrimSpace(acc.Email), e) {
			emailTaken = true
		}
		if p != "" && acc.Phone == p {
			phoneTaken = true
		}
	}
	return emailTaken, phoneTaken
}

// handleRegister — inscription SaaS : crée un COMPTE isolé (Account), son
// propriétaire (owner) et ses réglages par défaut, puis connecte immédiatement.
// Si REGISTER_KEY est définie (bêta privée), la clé doit être fournie. Le
// nouveau compte démarre vide : il ne voit aucune donnée des autres comptes.
func (a *API) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string `json:"name"`
		Username string `json:"username"`
		Password string `json:"password"`
		Key      string `json:"key"`
		Email    string `json:"email"`
		Phone    string `json:"phone"`   // WhatsApp de préférence, format E.164 sans +
		Country  string `json:"country"` // code ISO alpha-2 (CI, SN, NG…) ou "other"
		City     string `json:"city"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	// Sécurité S3 — quota d'inscription par IP (cf. signup_abuse.go) :
	// toute tentative — même invalide — consomme le quota, les fermes de
	// comptes d'essai sont coupées avant de toucher au registre. 429 +
	// Retry-After, même contrat que le verrou PIN (S2).
	if ok, retry := a.signup.allow(clientIP(r)); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
		writeErr(w, http.StatusTooManyRequests, "Trop de tentatives d'inscription — réessayez plus tard")
		return
	}
	// Contrôle d'inscription (I — paramètres plateforme) : priorité env
	// REGISTER_KEY > config DB (registerGate). Ouvert + clé vide = libre.
	// Sécurité P1 #10 — fail-closed : une inscription FERMÉE sans clé
	// configurée ne laisse plus personne passer. Avant ce correctif,
	// « fermée + clé vide » rendait expectedKey vide : toute requête sans
	// clé (req.Key == "" == expectedKey) était acceptée — le mode fermé
	// n'était qu'illusion. Le flux OUVERT (essai public 90 jours, mode
	// produit actuel) est strictement inchangé.
	if open, expectedKey := a.registerGate(); !open {
		if expectedKey == "" || req.Key != expectedKey {
			writeErr(w, http.StatusForbidden, "Inscription fermée — clé d'invitation requise")
			return
		}
	}
	username := strings.ToLower(strings.TrimSpace(req.Username))
	if len(username) < 3 || len(username) > 32 {
		writeErr(w, http.StatusBadRequest, "Le nom d'utilisateur doit faire entre 3 et 32 caractères")
		return
	}
	for _, c := range username {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
			writeErr(w, http.StatusBadRequest, "Le nom d'utilisateur n'accepte que a-z, 0-9, tirets et tirets bas")
			return
		}
	}
	// Sécurité S2 — politique centralisée (10 caractères, denylist, ≠ username).
	if msg := passwordPolicyViolation(req.Password, username); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = username
	}

	// F (signup enrichi) — validation email + WhatsApp + pays.
	email := strings.TrimSpace(req.Email)
	if email == "" {
		writeErr(w, http.StatusBadRequest, "L'email est requis")
		return
	}
	if !emailRegexp.MatchString(email) {
		writeErr(w, http.StatusBadRequest, "Format d'email invalide")
		return
	}
	// WhatsApp : on garde uniquement les chiffres, 8 à 15 (format E.164 sans +).
	phone := digitsOnly.ReplaceAllString(req.Phone, "")
	if len(phone) < 8 || len(phone) > 15 {
		writeErr(w, http.StatusBadRequest, "Numéro WhatsApp invalide (8 à 15 chiffres)")
		return
	}
	country := strings.ToLower(strings.TrimSpace(req.Country))
	if country == "" {
		writeErr(w, http.StatusBadRequest, "Le pays est requis")
		return
	}
	city := strings.TrimSpace(req.City)

	a.store.Lock()
	db := a.store.Data()
	// Unicité GLOBALE des usernames console (toutes consoles confondues).
	for i := range db.Users {
		if strings.EqualFold(db.Users[i].Username, username) {
			a.store.Unlock()
			writeErr(w, http.StatusConflict, "Ce nom d'utilisateur est déjà pris")
			return
		}
	}
	// Sécurité S5 — dédoublonnage email/WhatsApp : un même email ou un même
	// numéro WhatsApp ne peut créer qu'UN SEUL compte. Sans ce verrou, un
	// client dont l'essai est tombé sous le paywall (guard P3) relance un essai
	// de 90 jours à l'infini en changeant juste nom et username — le fermage
	// « manuel » le plus courant du marché cible. La raison fine (email vs
	// WhatsApp) est renvoyée pour que l'utilisateur légitime corrige son
	// formulaire ; le quota anti-abus (signup_abuse.go) borne déjà le sondage
	// d'adresses. Comptes désactivés inclus : un client banni ne revient pas
	// avec ses coordonnées.
	if emailTaken, phoneTaken := accountContactTaken(db.Accounts, email, phone, ""); emailTaken || phoneTaken {
		a.store.Unlock()
		if emailTaken {
			writeErr(w, http.StatusConflict, "Un compte existe déjà avec cet email")
		} else {
			writeErr(w, http.StatusConflict, "Un compte existe déjà avec ce numéro WhatsApp")
		}
		return
	}
	acc := model.Account{
		ID:        model.NewID("acc-"),
		Name:      name,
		Status:    "active",
		CreatedAt: model.NowISO(),
		Email:     email,
		Phone:     phone,
		Country:   country,
		City:      city,
	}
	db.Accounts = append(db.Accounts, acc)
	u := model.AdminUser{
		ID:           model.NewID("usr-"),
		AccountID:    acc.ID,
		Name:         name,
		Username:     username,
		Role:         "owner",
		PasswordHash: auth.HashPassword(req.Password, ""),
		CreatedAt:    model.NowISO(),
	}
	db.Users = append(db.Users, u)
	if db.SettingsByAccount == nil {
		db.SettingsByAccount = map[string]model.Settings{}
	}
	now := time.Now().UTC()
	db.SettingsByAccount[acc.ID] = model.Settings{
		Tenant: model.Tenant{
			Name: name, Currency: "XOF", Timezone: "Africa/Abidjan",
			ExpiryPolicyMode: "keep", ExpiryPolicyAfterDays: 30,
		},
		Plan: model.Plan{Name: "Essai", MaxRouters: "1", MaxUsers: "Illimité"},
		Subscription: model.Subscription{
			PlanID:      "essai",
			Status:      "active",
			PeriodStart: now.Format(time.RFC3339),
			PeriodEnd:   now.AddDate(0, 3, 0).Format(time.RFC3339),
			RouterSlots: 1,
		},
	}
	// P0 (audit Mikhmon) — chaque nouveau compte démarre avec les 3 modèles
	// de vouchers par défaut (contrat F2).
	db.Templates = append(db.Templates, store.SeedTemplatesFor(acc.ID)...)
	// v2 — chaque compte démarre aussi avec le profil « Staff » (accès personnel).
	db.Profiles = append(db.Profiles, store.SeedProfilesFor(acc.ID)...)
	a.logActivityBy(r, db, acc.ID, "compte", "Nouveau compte créé : "+acc.Name)
	a.store.Save()
	a.store.Unlock()

	token := auth.Sign(a.secret, auth.NewClaims(u.ID, u.Name, u.Role, acc.ID, u.SessionEpoch))
	writeJSON(w, http.StatusCreated, map[string]any{
		"token": token,
		"user": map[string]any{
			"id": u.ID, "name": u.Name, "username": u.Username, "role": u.Role,
			"accountId": acc.ID, "accountName": acc.Name,
		},
	})
}

func (a *API) handleMe(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r)
	if claims == nil {
		writeErr(w, http.StatusUnauthorized, "Token invalide ou expiré")
		return
	}
	a.store.Lock()
	db := a.store.Data()
	id, name, username, role := claims.Sub, claims.Name, claims.Sub, claims.Role
	// Le compte du TOKEN est prioritaire : il porte la session support
	// (impersonation — le claim « acc » pointe sur le compte consulté,
	// pas sur le compte d'origine de l'admin plateforme).
	accID := accountScope(r)
	isPlatform := role == model.RolePlatformAdmin || role == "admin"
	for i := range db.Users {
		u := &db.Users[i]
		if u.ID == claims.Sub {
			id, name, username, role = u.ID, u.Name, u.Username, u.Role
			isPlatform = role == model.RolePlatformAdmin || role == "admin"
			if claims.Acc != "" {
				accID = claims.Acc
			} else {
				accID = u.AccountID
			}
			break
		}
	}
	// Token émis avant la migration multi-tenant : repli sur le compte
	// principal pour les utilisateurs CLIENTS uniquement — l'admin
	// plateforme n'a plus de compte client propre (accountId vide).
	if accID == model.AccountMainID {
		found := false
		for i := range db.Accounts {
			if db.Accounts[i].ID == model.AccountMainID {
				found = true
				break
			}
		}
		if !found && isPlatform {
			accID = ""
		}
	} else if accID == "" && !isPlatform {
		accID = model.AccountMainID
	}
	accName := ""
	for i := range db.Accounts {
		if db.Accounts[i].ID == accID {
			accName = db.Accounts[i].Name
			break
		}
	}
	a.store.Unlock()
	// Sécurité S4 — statut 2FA exposé à l'écran de réglages.
	totpEnabled := false
	a.store.Lock()
	for i := range db.Users {
		if db.Users[i].ID == claims.Sub {
			totpEnabled = db.Users[i].TOTPEnabled
			break
		}
	}
	a.store.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"user": map[string]any{
			"id": id, "name": name, "username": username, "role": role,
			"accountId": accID, "accountName": accName,
			"totpEnabled": totpEnabled,
		},
	})
}

// ---------------------------------------------------------------------------
// Dashboard
// ---------------------------------------------------------------------------

// F (signup enrichi) — validations email + WhatsApp (chiffres uniquement).
var (
	emailRegexp = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
	digitsOnly  = regexp.MustCompile(`[^\d]`)
)
