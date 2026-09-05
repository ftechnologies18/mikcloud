// handlers_join.go — N°27 — inscriptions publiques par QR code (campus,
// écoles, administration, entreprise).
//
// Principe : le gérant crée un LIEN d'invitation depuis la console (onglet
// Inscriptions, rubrique Utilisateurs) ; la console l'encode en QR (URL
// /join/{token}). La page publique, SANS authentification, recueille
// l'identité de l'utilisateur et son couple « nom d'utilisateur & mot de
// passe » — mode RÉGULIER, distinct des vouchers (restés verrouillés
// username = password côté génération). La demande atterrit dans la file du
// gérant ; à la VALIDATION, l'utilisateur hotspot est créé par le même cœur
// que la console (createHotspotUser) : la validité démarre à l'APPROBATION,
// la file agent user_add s'enchaîne, le tombstone éventuel est levé.
//
// Sécurité :
//   - token de 32 caractères (alphabet sans ambiguïtés, ~155 bits) ;
//   - whitelist middleware restreinte à /api/join/{token} — PAS
//     /api/join-links (JWT console requis) ;
//   - rate-limit dédié main.go (10 req/min/IP) + quotas anti-abus CUMULÉS
//     sur la SOUMISSION (signupLimiter réutilisé : 5/10 min, 20/24 h) :
//     par IP et — N°33 — par MAC d'appareil (?mac= de la page login du
//     routeur), seule clé réellement discriminante derrière le NAT partagé
//     du hotspot (une seule IP publique pour tous les clients du lieu) ;
//   - honeypot « website » : un bot qui le remplit reçoit un succès factice
//     (rien n'est créé) — aucun indice sur le filtre ;
//   - N°33 anti-abus kiosque : au plus 1 compte AUTO-VALIDÉ par numéro de
//     téléphone et 24 h, par compte (joinKioskPhoneDailyMax) — sans ce
//     plafond, le dédoublonnage téléphone ne couvrait que les demandes
//     « pending » et le même numéro pouvait créer un compte à répétition ;
//   - GET public minimal : nom du lien, organisation, état, expiration,
//     places restantes — JAMAIS le catalogue de profils ;
//   - le mot de passe choisi n'est conservé que le temps de la décision :
//     VIDÉ à l'approbation comme au refus ; VIDÉ aussi (N°33) pour les
//     demandes pending de plus de 30 jours ; demandes refusées purgées à
//     30 jours (sweepStaleRegistrations, hook enforceExpired) ;
//   - N°33 politique mot de passe publique : 8 caractères min., denylist
//     S2 (mots de passe les plus courants), ≠ nom d'utilisateur.
package api

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"mikcloud/hotspot-api/internal/model"
)

// joinTokenLength — 32 caractères de l'alphabet sans ambiguïtés : le token
// est LE secret d'accès à la page publique (imprimé en QR, non devinable).
const joinTokenLength = 32

// registrationRetention — durée de conservation d'une demande REFUSÉE avant
// purge automatique (minimisation des données personnelles).
const registrationRetention = 30 * 24 * time.Hour

// joinMinPasswordLen — longueur minimale d'un mot de passe CHOISI sur la page
// publique (N°33). La console S2 exige 10 (comptes métier) ; le formulaire
// public est saisi sur un clavier de téléphone par des clients de passage :
// 8 caractères reste au-dessus des 6 d'origine (l'espace de recherche passe
// de ~3·10^7 à ~10^14 sur alphabet 62) sans friction excessive. La denylist
// S2 (mots de passe les plus courants, variantes FR) et l'interdiction
// « identique au nom d'utilisateur » s'appliquent aussi.
const joinMinPasswordLen = 8

// joinKioskPhoneDailyMax — N°33 anti-abus kiosque : nombre maximal de comptes
// AUTO-VALIDÉS par numéro de téléphone et fenêtre de 24 h, PAR COMPTE (tous
// liens kiosque confondus). Sans ce plafond, le dédoublonnage « phone_pending
// » ne couvrait que la file d'attente : en mode kiosque la demande passe
// directement « approved », un même numéro pouvait donc créer un compte
// toutes les quelques secondes (gratuité répétée). 1/24 h couvre l'usage
// légitime (une personne s'inscrit une fois) sans gêner personne.
const joinKioskPhoneDailyMax = 1

// joinKioskPhoneDailyWindow — fenêtre glissante du plafond ci-dessus.
const joinKioskPhoneDailyWindow = 24 * time.Hour

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// findJoinLinkByToken — recherche GLOBALE par token : la page publique n'a
// ni JWT ni compte, le token désigne lui-même le compte du lien. À appeler
// sous verrou.
func findJoinLinkByToken(db *model.DB, token string) *model.JoinLink {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil
	}
	for i := range db.JoinLinks {
		if db.JoinLinks[i].Token == token {
			return &db.JoinLinks[i]
		}
	}
	return nil
}

// findJoinLinkScoped — recherche par id, scopée au compte (console).
func findJoinLinkScoped(db *model.DB, id, acc string) *model.JoinLink {
	for i := range db.JoinLinks {
		if db.JoinLinks[i].ID == id && db.JoinLinks[i].AccountID == acc {
			return &db.JoinLinks[i]
		}
	}
	return nil
}

// findRegistrationScoped — recherche par id, scopée au compte (console).
func findRegistrationScoped(db *model.DB, id, acc string) *model.RegistrationRequest {
	for i := range db.RegistrationRequests {
		if db.RegistrationRequests[i].ID == id && db.RegistrationRequests[i].AccountID == acc {
			return &db.RegistrationRequests[i]
		}
	}
	return nil
}

// joinLinkState — état dérivé d'un lien : revoked > expired > exhausted >
// active (affichage console + décision page publique).
func joinLinkState(l model.JoinLink, now time.Time) string {
	if l.Revoked {
		return "revoked"
	}
	if l.ExpiresAt != "" {
		if t, err := time.Parse(time.RFC3339, l.ExpiresAt); err == nil && t.Before(now) {
			return "expired"
		}
	}
	if l.MaxUses > 0 && l.Uses >= l.MaxUses {
		return "exhausted"
	}
	return "active"
}

// normalizePhone — conserve le « + » de tête et les chiffres ; exige 8 à 15
// chiffres (format E.164 souple — marché cible Afrique de l'Ouest inclus).
func normalizePhone(raw string) (string, bool) {
	p := strings.TrimSpace(raw)
	p = strings.ReplaceAll(p, " ", "")
	p = strings.ReplaceAll(p, "-", "")
	p = strings.ReplaceAll(p, ".", "")
	digits := 0
	for i, c := range p {
		if c >= '0' && c <= '9' {
			digits++
			continue
		}
		if i == 0 && c == '+' {
			continue
		}
		return "", false
	}
	if digits < 8 || digits > 15 {
		return "", false
	}
	return p, true
}

// validJoinUsername — 3 à 32 caractères parmi lettres, chiffres, . _ -
// (conservateur : le nom devient un login RouterOS affiché sur le portail).
func validJoinUsername(s string) bool {
	if len(s) < 3 || len(s) > 32 {
		return false
	}
	for _, c := range s {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '.', c == '_', c == '-':
		default:
			return false
		}
	}
	return true
}

// usernameSuggestion — première variante libre du nom demandé (jean → jean2,
// jean3…) sous le même verrou que la vérification d'unicité.
func usernameSuggestion(db *model.DB, acc, base string) string {
	for i := 2; i < 60; i++ {
		candidate := base + strconv.Itoa(i)
		if !usernameTaken(db, acc, candidate) {
			return candidate
		}
	}
	return ""
}

// normalizeJoinMac — N°33 : adresse MAC de l'appareil de l'inscrit, fournie
// par la page de login du routeur (lien « S'inscrire » vers
// /join/{token}?mac=$(mac-esc)). Formats acceptés : séparateurs « : » ou « - »
// ou aucun, casse libre ; normalisée en majuscules avec « : ». Une MAC vide,
// absente ou INVALIDE renvoie "" (durcissement optionnel : on ne bloque
// jamais un vrai client sur un champ qu'il ne contrôle pas).
func normalizeJoinMac(raw string) string {
	p := strings.TrimSpace(raw)
	if p == "" {
		return ""
	}
	hexDigits := 0
	var b strings.Builder
	for _, c := range p {
		switch {
		case (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F'):
			hexDigits++
			if hexDigits > 12 {
				return ""
			}
			b.WriteRune(unicode.ToUpper(c))
			if hexDigits%2 == 0 && hexDigits < 12 {
				b.WriteByte(':')
			}
		case c == ':' || c == '-':
			// séparateur ignoré — la sortie est toujours normalisée
		default:
			return ""
		}
	}
	if hexDigits != 12 {
		return ""
	}
	return b.String()
}

// joinPasswordViolation — politique mot de passe de la page publique (N°33) :
// retourne le message d'erreur français si le mot de passe est refusé, ""
// s'il est acceptable. Miroir allégé de passwordPolicyViolation (console) :
// longueur 8 (vs 10 — clients de passage sur clavier mobile), même denylist
// S2, même interdiction « identique au nom d'utilisateur » ; la borne bcrypt
// n'entre pas en jeu (les mots de passe hotspot sont stockés tels quels pour
// RouterOS, plafonnés à 64 par la validation d'entrée).
func joinPasswordViolation(password, username string) string {
	if n := len(password); n < joinMinPasswordLen {
		return "Le mot de passe doit faire entre 8 et 64 caractères"
	}
	if username != "" && strings.EqualFold(password, username) {
		return "Le mot de passe ne doit pas être identique au nom d'utilisateur"
	}
	if _, banned := passwordDenylist[strings.ToLower(password)]; banned {
		return "Ce mot de passe figure parmi les plus utilisés et est interdit — choisissez-en un unique"
	}
	return ""
}

// joinLinkView — lien tel que renvoyé à la console, avec état dérivé.
type joinLinkView struct {
	model.JoinLink
	State string `json:"state"`
}

// ---------------------------------------------------------------------------
// Page publique (whitelist middleware — le token fait l'authentification)
// ---------------------------------------------------------------------------

// handleJoinInfo — GET /api/join/{token} : informations minimales pour
// afficher la page publique (nom du lien, organisation, état, expiration,
// places restantes, profil pré-attribué si kiosque). JAMAIS le catalogue de
// profils ni d'autres données du compte.
func (a *API) handleJoinInfo(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	a.store.Lock()
	db := a.store.Data()
	link := findJoinLinkByToken(db, token)
	if link == nil {
		a.store.Unlock()
		writeErrCode(w, http.StatusNotFound, "join_link_unknown", "Lien d'inscription inconnu", nil)
		return
	}
	l := *link // copie — le pointeur ne survit pas au verrou
	accName := ""
	for i := range db.Accounts {
		if db.Accounts[i].ID == l.AccountID {
			accName = db.Accounts[i].Name
			break
		}
	}
	a.store.Unlock()

	state := joinLinkState(l, time.Now().UTC())
	out := map[string]any{
		"name":         l.Name,
		"organization": accName,
		"state":        state,
	}
	if state == "active" {
		if l.ExpiresAt != "" {
			out["expiresAt"] = l.ExpiresAt
		}
		if l.MaxUses > 0 {
			remaining := l.MaxUses - l.Uses
			if remaining < 0 {
				remaining = 0
			}
			out["remaining"] = remaining
		}
		if l.AutoValidate && l.ProfileName != "" {
			out["autoValidate"] = true
			out["profileName"] = l.ProfileName
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// handleJoinSubmit — POST /api/join/{token} : soumission du formulaire
// public. Toute tentative consomme le quota anti-abus par IP ET par MAC
// (N°33 — la MAC vient de la page login du routeur via ?mac= ; elle distingue
// les appareils derrière le NAT partagé du hotspot, où l'IP publique est la
// MÊME pour tout le monde). Honeypot → succès factice. Lien kiosque
// (autoValidate + profil + routeur pré-attribués) → création immédiate via
// createHotspotUser, plafonnée par numéro (joinKioskPhoneDailyMax) ; sinon
// demande « pending » pour le gérant.
func (a *API) handleJoinSubmit(w http.ResponseWriter, r *http.Request) {
	if ok, retry := a.join.allow(clientIP(r)); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
		writeErr(w, http.StatusTooManyRequests, "Trop de tentatives — réessayez plus tard")
		return
	}
	token := r.PathValue("token")
	var req struct {
		FullName string `json:"fullName"`
		Phone    string `json:"phone"`
		Username string `json:"username"`
		Password string `json:"password"`
		Message  string `json:"message"`
		Website  string `json:"website"` // honeypot — champ invisible du formulaire
		Mac      string `json:"mac"`     // N°33 — appareil (page login du routeur), optionnel
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	mac := normalizeJoinMac(req.Mac)
	if mac != "" {
		// Second quota cumulé par APPAREIL : derrière le NAT du hotspot tous
		// les clients partagent la même IP publique — la MAC est la seule
		// clé qui isole réellement un fermier de comptes sur place.
		if ok, retry := a.join.allow("mac:" + mac); !ok {
			w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
			writeErr(w, http.StatusTooManyRequests, "Trop de tentatives — réessayez plus tard")
			return
		}
	}
	// Honeypot — un bot qui remplit le champ caché reçoit un succès factice :
	// rien n'est créé, aucun indice sur le filtre.
	if strings.TrimSpace(req.Website) != "" {
		writeJSON(w, http.StatusOK, map[string]any{"status": "pending"})
		return
	}
	fullName := strings.TrimSpace(req.FullName)
	if len(fullName) < 2 || len(fullName) > 80 {
		writeErr(w, http.StatusBadRequest, "Le nom complet doit faire entre 2 et 80 caractères")
		return
	}
	phone, okPhone := normalizePhone(req.Phone)
	if !okPhone {
		writeErr(w, http.StatusBadRequest, "Numéro de téléphone invalide (8 à 15 chiffres)")
		return
	}
	username := strings.TrimSpace(req.Username)
	if !validJoinUsername(username) {
		writeErr(w, http.StatusBadRequest, "Le nom d'utilisateur doit faire 3 à 32 caractères (lettres, chiffres, . _ -)")
		return
	}
	// N°33 — politique publique : 8 caractères min., denylist S2, ≠ nom.
	if msg := joinPasswordViolation(req.Password, username); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	message := strings.TrimSpace(req.Message)
	if len(message) > 300 {
		message = message[:300]
	}
	now := time.Now().UTC()

	// Validation d'ensemble (sous verrou de lecture) — TOUTE décision
	// d'écriture est re-vérifiée sous verrou plus bas.
	a.store.Lock()
	db := a.store.Data()
	link := findJoinLinkByToken(db, token)
	if link == nil {
		a.store.Unlock()
		writeErrCode(w, http.StatusNotFound, "join_link_unknown", "Lien d'inscription inconnu", nil)
		return
	}
	lcopy := *link
	if state := joinLinkState(lcopy, now); state != "active" {
		a.store.Unlock()
		writeErrCode(w, http.StatusConflict, "join_link_closed", "Ce lien d'inscription n'est plus actif", nil)
		return
	}
	acc := lcopy.AccountID
	if usernameTaken(db, acc, username) {
		sugg := usernameSuggestion(db, acc, username)
		a.store.Unlock()
		extra := map[string]any{}
		if sugg != "" {
			extra["suggestion"] = sugg
		}
		writeErrCode(w, http.StatusConflict, "username_taken", "Ce nom d'utilisateur est déjà pris", extra)
		return
	}
	for i := range db.RegistrationRequests {
		if db.RegistrationRequests[i].AccountID == acc &&
			db.RegistrationRequests[i].Status == "pending" &&
			db.RegistrationRequests[i].Phone == phone {
			a.store.Unlock()
			writeErrCode(w, http.StatusConflict, "phone_pending", "Une demande avec ce numéro est déjà en attente de validation", nil)
			return
		}
	}
	autoValidate := lcopy.AutoValidate && lcopy.ProfileID != "" && lcopy.RouterID != ""
	if autoValidate {
		// N°33 — plafond anti-abus kiosque : les demandes auto-validées ne
		// passent JAMAIS par « pending » (le dédoublonnage ci-dessus est
		// aveugle pour elles) — sans plafond, le même numéro pouvait créer
		// un compte à chaque soumission. Comptage des APPROUVÉES de la
		// fenêtre glissante, par compte (tous liens kiosque confondus).
		approvedToday := 0
		for i := range db.RegistrationRequests {
			q := &db.RegistrationRequests[i]
			if q.AccountID != acc || q.Status != "approved" || q.Phone != phone {
				continue
			}
			stamp := q.CreatedAt
			if q.ReviewedAt != "" {
				stamp = q.ReviewedAt
			}
			if t, err := time.Parse(time.RFC3339, stamp); err == nil && now.Sub(t) < joinKioskPhoneDailyWindow {
				approvedToday++
			}
		}
		if approvedToday >= joinKioskPhoneDailyMax {
			a.store.Unlock()
			writeErrCode(w, http.StatusConflict, "phone_limit", "Un compte a déjà été créé avec ce numéro dans les dernières 24 heures", nil)
			return
		}
	}
	a.store.Unlock()

	if autoValidate {
		// Lien kiosque — création immédiate par le cœur partagé (validité
		// démarre maintenant ; file agent user_add sur routeur agent).
		comment := "Inscription publique : " + fullName + " · " + phone
		u, cmdID, status, msg := a.createHotspotUser(r, acc, "regular", username, req.Password, lcopy.ProfileID, lcopy.RouterID, comment)
		if status != 0 {
			writeErr(w, status, msg)
			return
		}
		a.store.Lock()
		db = a.store.Data()
		if lk := findJoinLinkByToken(db, token); lk != nil {
			lk.Uses++
		}
		reg := model.RegistrationRequest{
			ID: model.NewID("reg-"), AccountID: acc,
			LinkID: lcopy.ID, LinkName: lcopy.Name,
			FullName: fullName, Phone: phone,
			DesiredUsername: u.Username, Password: "",
			Message: message,
			Status:  "approved", UserID: u.ID,
			ReviewedByName: "auto", ReviewedAt: model.NowISO(),
			CreatedIP: clientIP(r), CreatedMac: mac, CreatedAt: model.NowISO(),
		}
		db.RegistrationRequests = append(db.RegistrationRequests, reg)
		a.logActivityBy(r, db, acc, "registration", "Inscription auto-validée (lien « "+lcopy.Name+" ») : "+fullName+" → "+u.Username)
		a.store.Save()
		a.store.Unlock()
		out := map[string]any{"status": "approved", "username": u.Username, "password": u.Password}
		if cmdID != "" {
			out["queued"] = true
		}
		writeJSON(w, http.StatusOK, out)
		return
	}

	// File d'attente standard — re-vérification du lien sous verrou
	// d'écriture (expiration, révocation ou saturation entre-temps).
	a.store.Lock()
	db = a.store.Data()
	link = findJoinLinkByToken(db, token)
	if link == nil || joinLinkState(*link, now) != "active" {
		a.store.Unlock()
		writeErrCode(w, http.StatusConflict, "join_link_closed", "Ce lien d'inscription n'est plus actif", nil)
		return
	}
	reg := model.RegistrationRequest{
		ID: model.NewID("reg-"), AccountID: link.AccountID,
		LinkID: link.ID, LinkName: link.Name,
		FullName: fullName, Phone: phone,
		DesiredUsername: username, Password: req.Password,
		Message:   message,
		Status:    "pending",
		CreatedIP: clientIP(r), CreatedMac: mac, CreatedAt: model.NowISO(),
	}
	db.RegistrationRequests = append(db.RegistrationRequests, reg)
	link.Uses++
	a.logActivityBy(r, db, link.AccountID, "registration", "Nouvelle demande d'inscription : "+fullName+" ("+phone+") via « "+link.Name+" »")
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"status": "pending"})
}

// ---------------------------------------------------------------------------
// Console — gestion des liens + file de validation (manager et plus)
// ---------------------------------------------------------------------------

// handleJoinLinksList — GET /api/join-links : liens du compte, plus récents
// d'abord, avec état dérivé (active | revoked | expired | exhausted).
func (a *API) handleJoinLinksList(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	a.store.Lock()
	db := a.store.Data()
	now := time.Now().UTC()
	views := []joinLinkView{}
	for _, l := range db.JoinLinks {
		if l.AccountID == acc {
			views = append(views, joinLinkView{JoinLink: l, State: joinLinkState(l, now)})
		}
	}
	a.store.Unlock()
	sort.Slice(views, func(i, j int) bool { return views[i].CreatedAt > views[j].CreatedAt })
	writeJSON(w, http.StatusOK, map[string]any{"items": views})
}

// handleJoinLinkCreate — POST /api/join-links {name, profileId?, routerId?,
// autoValidate, maxUses, expiresAt?}. autoValidate (kiosque) exige un profil
// ET un routeur pré-attribués. Token unique de 32 caractères.
func (a *API) handleJoinLinkCreate(w http.ResponseWriter, r *http.Request) {
	if !a.guardAccountWrite(w, r) {
		return
	}
	sub, actorName := "", ""
	if c := claimsFrom(r); c != nil {
		sub, actorName = c.Sub, c.Name
	}
	acc := accountScope(r)
	var req struct {
		Name         string `json:"name"`
		ProfileID    string `json:"profileId"`
		RouterID     string `json:"routerId"`
		AutoValidate bool   `json:"autoValidate"`
		MaxUses      int    `json:"maxUses"`
		ExpiresAt    string `json:"expiresAt"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" || len(name) > 60 {
		writeErr(w, http.StatusBadRequest, "Le nom du lien est requis (60 caractères max)")
		return
	}
	maxUses := req.MaxUses
	if maxUses < 0 || maxUses > 100000 {
		writeErr(w, http.StatusBadRequest, "Limite d'usages invalide (0 = illimité)")
		return
	}
	expiresAt := strings.TrimSpace(req.ExpiresAt)
	if expiresAt != "" {
		t, err := time.Parse(time.RFC3339, expiresAt)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "Date d'expiration invalide")
			return
		}
		if !t.After(time.Now().UTC()) {
			writeErr(w, http.StatusBadRequest, "La date d'expiration doit être dans le futur")
			return
		}
	}
	now := time.Now().UTC()
	profileID := strings.TrimSpace(req.ProfileID)
	routerID := strings.TrimSpace(req.RouterID)

	a.store.Lock()
	db := a.store.Data()
	profileName, routerName := "", ""
	if profileID != "" {
		if p := findProfileScoped(db, profileID, acc); p != nil {
			profileName = p.Name
		} else {
			a.store.Unlock()
			writeErr(w, http.StatusBadRequest, "Profil introuvable")
			return
		}
	}
	if routerID != "" {
		if rr := findRouterScoped(db, routerID, acc); rr != nil {
			routerName = rr.Name
		} else {
			a.store.Unlock()
			writeErr(w, http.StatusBadRequest, "Routeur introuvable")
			return
		}
	}
	if req.AutoValidate && (profileName == "" || routerName == "") {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, "La validation automatique exige un profil et un routeur pré-attribués")
		return
	}
	token := ""
	for i := 0; i < 20 && token == ""; i++ {
		candidate := model.RandomCode(joinTokenLength)
		if findJoinLinkByToken(db, candidate) == nil {
			token = candidate
		}
	}
	if token == "" {
		a.store.Unlock()
		writeErr(w, http.StatusInternalServerError, "Génération du lien impossible — réessayez")
		return
	}
	l := model.JoinLink{
		ID: model.NewID("jl-"), AccountID: acc, Name: name, Token: token,
		ProfileID: profileID, ProfileName: profileName,
		RouterID: routerID, RouterName: routerName,
		AutoValidate: req.AutoValidate, MaxUses: maxUses,
		ExpiresAt: expiresAt,
		CreatedBy: sub, CreatedByName: actorName, CreatedAt: model.NowISO(),
	}
	db.JoinLinks = append(db.JoinLinks, l)
	a.logActivityBy(r, db, acc, "registration", "Lien d'inscription « "+name+" » créé (QR dans l'onglet Inscriptions)")
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, joinLinkView{JoinLink: l, State: joinLinkState(l, now)})
}

// handleJoinLinkUpdate — PUT /api/join-links/{id} {revoked bool} : révoquer /
// réactiver un lien (effet IMMÉDIAT sur les pages publiques ouvertes).
func (a *API) handleJoinLinkUpdate(w http.ResponseWriter, r *http.Request) {
	if !a.guardAccountWrite(w, r) {
		return
	}
	acc := accountScope(r)
	var req struct {
		Revoked *bool `json:"revoked"`
	}
	if err := decodeBody(r, &req); err != nil || req.Revoked == nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide (revoked attendu)")
		return
	}
	a.store.Lock()
	db := a.store.Data()
	l := findJoinLinkScoped(db, r.PathValue("id"), acc)
	if l == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Lien introuvable")
		return
	}
	l.Revoked = *req.Revoked
	out := joinLinkView{JoinLink: *l, State: joinLinkState(*l, time.Now().UTC())}
	action := "révoqué"
	if !l.Revoked {
		action = "réactivé"
	}
	a.logActivityBy(r, db, acc, "registration", "Lien d'inscription « "+l.Name+" » "+action)
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, out)
}

// handleJoinLinkDelete — DELETE /api/join-links/{id} : supprime le lien (les
// demandes déjà soumises conservent le nom du lien pour la traçabilité).
func (a *API) handleJoinLinkDelete(w http.ResponseWriter, r *http.Request) {
	if !a.guardAccountWrite(w, r) {
		return
	}
	acc := accountScope(r)
	a.store.Lock()
	db := a.store.Data()
	idx := -1
	for i := range db.JoinLinks {
		if db.JoinLinks[i].ID == r.PathValue("id") && db.JoinLinks[i].AccountID == acc {
			idx = i
			break
		}
	}
	if idx < 0 {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Lien introuvable")
		return
	}
	name := db.JoinLinks[idx].Name
	db.JoinLinks = append(db.JoinLinks[:idx], db.JoinLinks[idx+1:]...)
	a.logActivityBy(r, db, acc, "registration", "Lien d'inscription « "+name+" » supprimé")
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleRegistrationsList — GET /api/registrations?status= : demandes du
// compte (les plus récentes d'abord, plafonnées) + compteurs par statut
// (badge « en attente » de la console).
func (a *API) handleRegistrationsList(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	a.store.Lock()
	db := a.store.Data()
	counts := map[string]int{"pending": 0, "approved": 0, "rejected": 0}
	items := []model.RegistrationRequest{}
	for _, q := range db.RegistrationRequests {
		if q.AccountID != acc {
			continue
		}
		if _, known := counts[q.Status]; known {
			counts[q.Status]++
		}
		if status == "" || q.Status == status {
			items = append(items, q)
		}
	}
	a.store.Unlock()
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt > items[j].CreatedAt })
	const maxItems = 300
	if len(items) > maxItems {
		items = items[:maxItems]
	}
	writeJSON(w, http.StatusOK, map[string]any{"counts": counts, "items": items})
}

// handleRegistrationApprove — POST /api/registrations/{id}/approve
// {profileId, routerId, username, password?} : crée l'utilisateur hotspot par
// le MÊME cœur que la console (createHotspotUser) — validité démarrant à
// l'approbation, file agent user_add, tombstone levé, journal. Le mot de
// passe choisi est VIDÉ de la demande quoi qu'il arrive. Mode de connexion :
// « Nom d'utilisateur & Mot de passe » (deux codes distincts au choix —
// distinct des vouchers, verrouillés username = password).
func (a *API) handleRegistrationApprove(w http.ResponseWriter, r *http.Request) {
	if !a.guardAccountWrite(w, r) {
		return
	}
	acc := accountScope(r)
	var req struct {
		ProfileID string `json:"profileId"`
		RouterID  string `json:"routerId"`
		Username  string `json:"username"`
		Password  string `json:"password"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	username := strings.TrimSpace(req.Username)
	if !validJoinUsername(username) {
		writeErr(w, http.StatusBadRequest, "Le nom d'utilisateur doit faire 3 à 32 caractères (lettres, chiffres, . _ -)")
		return
	}
	// Mot de passe OPTIONNEL (vide = généré par le serveur). S'il est choisi,
	// même politique que la page publique (N°33 — 8 min., denylist, ≠ nom).
	if len(req.Password) > 0 {
		if msg := joinPasswordViolation(req.Password, username); msg != "" {
			writeErr(w, http.StatusBadRequest, msg)
			return
		}
	}
	profileID := strings.TrimSpace(req.ProfileID)
	routerID := strings.TrimSpace(req.RouterID)

	a.store.Lock()
	db := a.store.Data()
	q := findRegistrationScoped(db, r.PathValue("id"), acc)
	if q == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Demande introuvable")
		return
	}
	if q.Status != "pending" {
		a.store.Unlock()
		writeErr(w, http.StatusConflict, "Demande déjà traitée")
		return
	}
	qc := *q // copie pour l'après-verrou
	if findProfileScoped(db, profileID, acc) == nil {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, "Profil introuvable")
		return
	}
	if findRouterScoped(db, routerID, acc) == nil {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, "Routeur introuvable")
		return
	}
	if usernameTaken(db, acc, username) {
		sugg := usernameSuggestion(db, acc, username)
		a.store.Unlock()
		extra := map[string]any{}
		if sugg != "" {
			extra["suggestion"] = sugg
		}
		writeErrCode(w, http.StatusConflict, "username_taken", "Ce nom d'utilisateur est déjà pris", extra)
		return
	}
	a.store.Unlock()

	comment := "Inscription publique : " + qc.FullName + " · " + qc.Phone
	u, cmdID, status, msg := a.createHotspotUser(r, acc, "regular", username, req.Password, profileID, routerID, comment)
	if status != 0 {
		writeErr(w, status, msg)
		return
	}

	a.store.Lock()
	db = a.store.Data()
	q = findRegistrationScoped(db, r.PathValue("id"), acc)
	if q != nil {
		actorID, actorName := "", ""
		if c := claimsFrom(r); c != nil {
			actorID, actorName = c.Sub, c.Name
		}
		q.Status = "approved"
		q.ReviewedBy = actorID
		q.ReviewedByName = actorName
		q.ReviewedAt = model.NowISO()
		q.UserID = u.ID
		q.DesiredUsername = u.Username // nom final (sanitisé côté agent éventuel)
		q.Password = ""                // minimisation — l'identifiant final vit dans HotspotUsers
		q.RejectionReason = ""
		a.logActivityBy(r, db, acc, "registration", "Inscription validée : "+q.FullName+" → utilisateur "+u.Username+" (« Nom d'utilisateur & Mot de passe »)")
		reqOut := *q
		a.store.Save()
		a.store.Unlock()
		out := map[string]any{"request": reqOut, "user": u}
		if cmdID != "" {
			out["queued"] = true
		}
		writeJSON(w, http.StatusOK, out)
		return
	}
	// Course extrême (demande purgée entre-temps) : l'utilisateur est créé,
	// la trace reste dans le journal d'activité.
	a.store.Save()
	a.store.Unlock()
	out := map[string]any{"user": u}
	if cmdID != "" {
		out["queued"] = true
	}
	writeJSON(w, http.StatusOK, out)
}

// handleRegistrationReject — POST /api/registrations/{id}/reject {reason} :
// refuse la demande (motif conservé pour l'historique) et VIDE le mot de
// passe choisi.
func (a *API) handleRegistrationReject(w http.ResponseWriter, r *http.Request) {
	if !a.guardAccountWrite(w, r) {
		return
	}
	acc := accountScope(r)
	var req struct {
		Reason string `json:"reason"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" || len(reason) > 300 {
		writeErr(w, http.StatusBadRequest, "Le motif du refus est requis (300 caractères max)")
		return
	}
	a.store.Lock()
	db := a.store.Data()
	q := findRegistrationScoped(db, r.PathValue("id"), acc)
	if q == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Demande introuvable")
		return
	}
	if q.Status != "pending" {
		a.store.Unlock()
		writeErr(w, http.StatusConflict, "Demande déjà traitée")
		return
	}
	actorID, actorName := "", ""
	if c := claimsFrom(r); c != nil {
		actorID, actorName = c.Sub, c.Name
	}
	q.Status = "rejected"
	q.RejectionReason = reason
	q.ReviewedBy = actorID
	q.ReviewedByName = actorName
	q.ReviewedAt = model.NowISO()
	q.Password = "" // minimisation — un refus n'a plus besoin du secret choisi
	a.logActivityBy(r, db, acc, "registration", "Inscription refusée : "+q.FullName+" — "+reason)
	out := *q
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, out)
}

// handleRegistrationDelete — DELETE /api/registrations/{id} : purge manuelle
// d'une demande (les refusées sont aussi purgées automatiquement à 30 j).
func (a *API) handleRegistrationDelete(w http.ResponseWriter, r *http.Request) {
	if !a.guardAccountWrite(w, r) {
		return
	}
	acc := accountScope(r)
	a.store.Lock()
	db := a.store.Data()
	idx := -1
	for i := range db.RegistrationRequests {
		if db.RegistrationRequests[i].ID == r.PathValue("id") && db.RegistrationRequests[i].AccountID == acc {
			idx = i
			break
		}
	}
	if idx < 0 {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Demande introuvable")
		return
	}
	db.RegistrationRequests = append(db.RegistrationRequests[:idx], db.RegistrationRequests[idx+1:]...)
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// sweepStaleRegistrations — minimisation des données personnelles au-delà de
// la rétention (30 jours), hook enforceExpired, à appeler sous verrou (le Save
// est à charge de l'appelant) :
//   - demandes REFUSÉES : purgées (statut final, plus aucune valeur) ;
//   - demandes PENDING jamais tranchées : le MOT DE PASSE CLAIR est vidé
//     (N°33) — une demande oubliée ne doit pas conserver un secret
//     indéfiniment ; la demande reste visible du gérant, qui peut encore
//     l'approuver (le serveur génère alors un mot de passe) ou la refuser.
func sweepStaleRegistrations(db *model.DB) int {
	now := time.Now().UTC()
	kept := db.RegistrationRequests[:0]
	removed := 0
	for i := range db.RegistrationRequests {
		q := &db.RegistrationRequests[i]
		if q.CreatedAt != "" {
			if t, err := time.Parse(time.RFC3339, q.CreatedAt); err == nil && now.Sub(t) > registrationRetention {
				switch {
				case q.Status == "rejected":
					removed++
					continue
				case q.Status == "pending":
					q.Password = "" // secret vidé, demande conservée
				}
			}
		}
		kept = append(kept, *q)
	}
	db.RegistrationRequests = kept
	return removed
}
