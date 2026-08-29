package api

// N°7 — Équipe & rôles : gestion des membres du compte (console opérateurs).
//
// Modèle : owner (propriétaire, tout) > manager (gérant : tout le compte sauf
// équipe et réglages/billing) > operator (vendeur : ventes/vouchers/sessions,
// lecture seule ailleurs). Le super-admin plateforme (platform_admin, ex « admin »)
// est traité owner sur le compte consulté et ne peut être créé/édité que par
// un autre super-admin.
//
// Invariants servis (chaque mutation est auditée avec l'acteur — logActivityBy) :
//   - un compte garde TOUJOURS ≥ 1 owner actif (ni auto-rétrogradation, ni
//     suppression, ni démotion du dernier owner) ;
//   - un membre ne peut pas se supprimer lui-même ;
//   - les mots de passe sont stockés en bcrypt (auth.HashPassword), jamais renvoyés.

import (
	"net/http"
	"regexp"
	"strings"

	"mikcloud/hotspot-api/internal/auth"
	"mikcloud/hotspot-api/internal/model"
)

// teamMember — vue publique d'un membre (jamais de hash/salt).
type teamMember struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Username  string `json:"username"`
	Role      string `json:"role"`
	CreatedAt string `json:"createdAt"`
}

var usernamePattern = regexp.MustCompile(`^[a-z0-9._-]{3,32}$`)

func toTeamMember(u *model.AdminUser) teamMember {
	return teamMember{ID: u.ID, Name: u.Name, Username: u.Username, Role: u.Role, CreatedAt: u.CreatedAt}
}

// handleTeamList — GET /api/team : membres du compte du token (owner).
func (a *API) handleTeamList(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	a.store.Lock()
	defer a.store.Unlock()
	out := []teamMember{}
	for i := range a.store.Data().Users {
		u := &a.store.Data().Users[i]
		if u.AccountID == acc || (u.Role == model.RolePlatformAdmin || u.Role == "admin") {
			out = append(out, toTeamMember(u))
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// handleTeamCreate — POST /api/team {name, username, password, role} (owner).
func (a *API) handleTeamCreate(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	var req struct {
		Name     string `json:"name"`
		Username string `json:"username"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Username = strings.ToLower(strings.TrimSpace(req.Username))
	if req.Name == "" || len(req.Name) > 80 {
		writeErr(w, http.StatusBadRequest, "Le nom est requis (80 caractères max)")
		return
	}
	if !usernamePattern.MatchString(req.Username) {
		writeErr(w, http.StatusBadRequest, "Identifiant invalide : 3-32 caractères (a-z, 0-9, . _ -)")
		return
	}
	if len(req.Password) < 8 {
		writeErr(w, http.StatusBadRequest, "Mot de passe : 8 caractères minimum")
		return
	}
	// Rôles attribuables par un owner : operator / manager / owner.
	// platform_admin ne se crée que côté plateforme (jamais via l'API compte).
	if req.Role != model.RoleOperator && req.Role != model.RoleManager && req.Role != model.RoleOwner {
		writeErr(w, http.StatusBadRequest, "Rôle invalide (operator, manager ou owner)")
		return
	}

	a.store.Lock()
	db := a.store.Data()
	// Identifiants GLOBALEMENT uniques (le login cherche par username sans compte).
	for i := range db.Users {
		if strings.EqualFold(db.Users[i].Username, req.Username) {
			a.store.Unlock()
			writeErr(w, http.StatusConflict, "Cet identifiant est déjà pris")
			return
		}
	}
	u := model.AdminUser{
		ID:           model.NewID("usr-"),
		AccountID:    acc,
		Name:         req.Name,
		Username:     req.Username,
		Role:         req.Role,
		PasswordHash: auth.HashPassword(req.Password, ""),
		CreatedAt:    model.NowISO(),
	}
	db.Users = append(db.Users, u)
	a.logActivityBy(r, db, acc, "team",
		"Membre «"+req.Username+"» ajouté à l'équipe («"+req.Role+"»)")
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusCreated, toTeamMember(&u))
}

// handleTeamUpdate — PUT /api/team/{id} {name?, role?, password?} (owner).
// Garde-fous : pas d'auto-changement de rôle ; ≥ 1 owner par compte ;
// membres platform_admin non éditables par un simple owner.
func (a *API) handleTeamUpdate(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	claims := claimsFrom(r)
	id := r.PathValue("id")
	var req struct {
		Name     *string `json:"name"`
		Role     *string `json:"role"`
		Password *string `json:"password"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	if req.Name != nil {
		n := strings.TrimSpace(*req.Name)
		if n == "" || len(n) > 80 {
			writeErr(w, http.StatusBadRequest, "Nom invalide (1-80 caractères)")
			return
		}
	}
	if req.Role != nil && *req.Role != model.RoleOperator && *req.Role != model.RoleManager && *req.Role != model.RoleOwner {
		writeErr(w, http.StatusBadRequest, "Rôle invalide (operator, manager ou owner)")
		return
	}
	if req.Password != nil && len(*req.Password) < 8 {
		writeErr(w, http.StatusBadRequest, "Mot de passe : 8 caractères minimum")
		return
	}

	a.store.Lock()
	db := a.store.Data()
	var target *model.AdminUser
	for i := range db.Users {
		if db.Users[i].ID == id {
			target = &db.Users[i]
			break
		}
	}
	if target == nil || target.AccountID != acc {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Membre introuvable")
		return
	}
	// Un owner de compte ne touche pas un super-admin plateforme.
	if (target.Role == model.RolePlatformAdmin || target.Role == "admin") && !isPlatformAdmin(r) {
		a.store.Unlock()
		writeErr(w, http.StatusForbidden, "Ce membre est géré par la plateforme MikCloud")
		return
	}
	// Interdit : changer son propre rôle (auto-rétrogradation/surélévation).
	if claims != nil && target.ID == claims.Sub && req.Role != nil && *req.Role != target.Role {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, "Vous ne pouvez pas changer votre propre rôle")
		return
	}
	// Invariant : ≥ 1 owner par compte après changement.
	if req.Role != nil && *req.Role != model.RoleOwner && target.Role == model.RoleOwner {
		owners := 0
		for i := range db.Users {
			if db.Users[i].AccountID == acc && db.Users[i].Role == model.RoleOwner {
				owners++
			}
		}
		if owners <= 1 {
			a.store.Unlock()
			writeErr(w, http.StatusBadRequest, "Impossible : ce compte doit garder au moins un propriétaire")
			return
		}
	}

	changes := []string{}
	if req.Name != nil && *req.Name != target.Name {
		target.Name = strings.TrimSpace(*req.Name)
		changes = append(changes, "nom")
	}
	if req.Role != nil && *req.Role != target.Role {
		old := target.Role
		target.Role = *req.Role
		changes = append(changes, "rôle «"+old+"» → «"+*req.Role+"»")
		// Un mot de passe (ré)initialisé par le patron devient protégé : la
		// variable ADMIN_PASSWORD ne doit pas l'écraser au prochain démarrage.
		target.PasswordSetByUser = true
	}
	if req.Password != nil {
		target.PasswordHash = auth.HashPassword(*req.Password, "")
		target.Salt = ""
		target.PasswordSetByUser = true
		changes = append(changes, "mot de passe réinitialisé")
	}
	if len(changes) == 0 {
		a.store.Unlock()
		writeJSON(w, http.StatusOK, toTeamMember(target))
		return
	}
	a.logActivityBy(r, db, acc, "team",
		"Membre «"+target.Username+"» modifié ("+strings.Join(changes, ", ")+")")
	a.store.Save()
	member := toTeamMember(target)
	a.store.Unlock()
	writeJSON(w, http.StatusOK, member)
}

// handleTeamDelete — DELETE /api/team/{id} (owner). Interdit : soi-même,
// dernier owner, membres platform_admin (sauf super-admin).
func (a *API) handleTeamDelete(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	claims := claimsFrom(r)
	id := r.PathValue("id")
	if claims != nil && id == claims.Sub {
		writeErr(w, http.StatusBadRequest, "Vous ne pouvez pas supprimer votre propre compte")
		return
	}
	a.store.Lock()
	db := a.store.Data()
	idx := -1
	for i := range db.Users {
		if db.Users[i].ID == id {
			idx = i
			break
		}
	}
	if idx < 0 || db.Users[idx].AccountID != acc {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Membre introuvable")
		return
	}
	target := db.Users[idx]
	if (target.Role == model.RolePlatformAdmin || target.Role == "admin") && !isPlatformAdmin(r) {
		a.store.Unlock()
		writeErr(w, http.StatusForbidden, "Ce membre est géré par la plateforme MikCloud")
		return
	}
	if target.Role == model.RoleOwner {
		owners := 0
		for i := range db.Users {
			if db.Users[i].AccountID == acc && db.Users[i].Role == model.RoleOwner {
				owners++
			}
		}
		if owners <= 1 {
			a.store.Unlock()
			writeErr(w, http.StatusBadRequest, "Impossible : ce compte doit garder au moins un propriétaire")
			return
		}
	}
	db.Users = append(db.Users[:idx], db.Users[idx+1:]...)
	a.logActivityBy(r, db, acc, "team", "Membre «"+target.Username+"» retiré de l'équipe")
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
