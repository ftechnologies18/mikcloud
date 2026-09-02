// handlers_users.go — gestion des utilisateurs hotspot (CRUD, statut, suppression).

package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"mikcloud/hotspot-api/internal/agent"
	"mikcloud/hotspot-api/internal/model"
	"mikcloud/hotspot-api/internal/store"
)

func (a *API) handleUsersList(w http.ResponseWriter, r *http.Request) {
	a.usersList(w, r, "")
}

func (a *API) usersList(w http.ResponseWriter, r *http.Request, kindOverride string) {
	acc := accountScope(r)
	q := r.URL.Query()
	if kindOverride != "" {
		q.Set("kind", kindOverride)
	}
	page := queryInt(r, "page", 1, 1, 1_000_000)
	pageSize := queryInt(r, "pageSize", 15, 1, 200)

	now := time.Now().UTC()
	a.store.Lock()
	db := a.store.Data()
	// P0 (audit Mikhmon) : fait vivre la simulation (statuts d'expiration à
	// jour via applyExpiry) puis applique l'enforcement routeur (F1).
	store.Tick(db, now)
	a.enforceExpired(db)
	filtered := filterUsers(db, acc, q, now)
	a.store.Save()
	a.store.Unlock()

	total := len(filtered)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data":     filtered[start:end],
		"total":    total,
		"page":     page,
		"pageSize": pageSize,
	})
}

func (a *API) handleUserCreate(w http.ResponseWriter, r *http.Request) {
	// P3 — compte expiré : écritures métier refusées (lecture seule).
	if !a.guardAccountWrite(w, r) {
		return
	}
	acc := accountScope(r)
	var req struct {
		Username  string `json:"username"`
		Password  string `json:"password"`
		ProfileID string `json:"profileId"`
		RouterID  string `json:"routerId"`
		Comment   string `json:"comment"`
		Kind      string `json:"kind"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	now := time.Now().UTC()
	nowISO := model.NowISO()

	// Validation + construction (sous verrou)
	a.store.Lock()
	db := a.store.Data()
	profile := findProfileScoped(db, strings.TrimSpace(req.ProfileID), acc)
	if profile == nil {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, "Profil introuvable")
		return
	}
	router := findRouterScoped(db, strings.TrimSpace(req.RouterID), acc)
	if router == nil {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, "Routeur introuvable")
		return
	}
	routerCopy := *router
	kind := req.Kind
	if kind == "" {
		kind = "regular"
	}
	if kind != "regular" && kind != "voucher" {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, "Type d'utilisateur invalide (regular ou voucher)")
		return
	}
	username := strings.TrimSpace(req.Username)
	if username == "" {
		for i := 0; i < 50; i++ {
			candidate := "user-" + model.RandomCode(5)
			if !usernameTaken(db, acc, candidate) {
				username = candidate
				break
			}
		}
	} else if usernameTaken(db, acc, username) {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, "Ce nom d'utilisateur existe déjà")
		return
	}
	password := req.Password
	if password == "" {
		password = model.RandomCode(6)
	}
	// Validité ancrée au 1er login (vouchers) : expiresAt reste vide tant que
	// le ticket n'a jamais été connecté — le stock non vendu n'expire pas.
	expiresAt := ""
	if kind != "voucher" {
		expiresAt = now.Add(time.Duration(profile.ValidityMinutes()) * time.Minute).Format(time.RFC3339)
	}
	u := model.HotspotUser{
		ID: model.NewID("u-"), AccountID: acc, Kind: kind, Username: username, Password: password,
		ProfileID: profile.ID, ProfileName: profile.Name,
		RouterID: routerCopy.ID, RouterName: routerCopy.Name,
		Status: "active", BatchID: "", ResellerID: "", ResellerName: "",
		Comment:   strings.TrimSpace(req.Comment),
		CreatedAt: nowISO,
		ExpiresAt: expiresAt,
		UsedAt:    "", Price: profile.Price, DataQuotaMb: int64(profile.DataQuotaMb),
	}
	a.store.Unlock()

	if routerCopy.Mode == "agent" {
		// Mode agent : persistance immédiate + commande user_add en file.
		u.Username = agent.SanitizeName(u.Username)
		u.ProfileName = agent.SanitizeName(u.ProfileName)
		a.store.Lock()
		db = a.store.Data()
		db.HotspotUsers = append(db.HotspotUsers, u)
		userPayload := map[string]any{
			"name": u.Username, "password": u.Password,
			"profile": profileRef(*profile), "comment": u.Comment,
		}
		if profile.DataQuotaMb > 0 {
			userPayload["limitBytesTotal"] = profile.DataQuotaMb * 1048576
		}
		cmd := queueCommandLocked(db, routerCopy.AccountID, routerCopy.ID, model.CmdUserAdd, userPayload)
		a.logActivityBy(r, db, acc, "user", "Utilisateur "+u.Username+" créé (en attente du routeur, commande "+cmd.ID+")")
		a.store.Save()
		cmdID := cmd.ID
		a.store.Unlock()
		data, _ := json.Marshal(u)
		var out map[string]any
		_ = json.Unmarshal(data, &out)
		out["queued"] = true
		out["commandId"] = cmdID
		writeJSON(w, http.StatusOK, out)
		return
	}

	gw := a.gatewayFor(routerCopy)
	if err := gw.AddUser(&u); err != nil {
		writeErr(w, http.StatusBadRequest, "Création impossible : "+err.Error())
		return
	}

	a.store.Lock()
	a.logActivityBy(r, a.store.Data(), acc, "user", "Utilisateur "+u.Username+" créé")
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, u)
}

func (a *API) handleUserUpdate(w http.ResponseWriter, r *http.Request) {
	// P3 — compte expiré : écritures métier refusées (lecture seule).
	if !a.guardAccountWrite(w, r) {
		return
	}
	acc := accountScope(r)
	id := r.PathValue("id")
	var req struct {
		Username  *string `json:"username"`
		Password  *string `json:"password"`
		ProfileID *string `json:"profileId"`
		Comment   *string `json:"comment"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	now := time.Now().UTC()

	a.store.Lock()
	db := a.store.Data()
	cur := findUserScoped(db, id, acc)
	if cur == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Utilisateur introuvable")
		return
	}
	u := *cur
	oldUsername := cur.Username
	router := findRouterScoped(db, u.RouterID, acc)
	var routerCopy *model.Router
	if router != nil {
		c := *router
		routerCopy = &c
	}
	a.store.Unlock()

	if req.Username != nil {
		username := strings.TrimSpace(*req.Username)
		if username == "" {
			writeErr(w, http.StatusBadRequest, "Nom d'utilisateur requis")
			return
		}
		a.store.Lock()
		taken := false
		for i := range a.store.Data().HotspotUsers {
			other := &a.store.Data().HotspotUsers[i]
			if other.AccountID == acc && other.ID != id && other.Username == username {
				taken = true
				break
			}
		}
		a.store.Unlock()
		if taken {
			writeErr(w, http.StatusBadRequest, "Ce nom d'utilisateur existe déjà")
			return
		}
		u.Username = username
	}
	passwordChanged := false
	if req.Password != nil && *req.Password != "" {
		u.Password = *req.Password
		passwordChanged = true
	}
	if req.ProfileID != nil && strings.TrimSpace(*req.ProfileID) != "" {
		a.store.Lock()
		p := findProfileScoped(a.store.Data(), strings.TrimSpace(*req.ProfileID), acc)
		a.store.Unlock()
		if p == nil {
			writeErr(w, http.StatusBadRequest, "Profil introuvable")
			return
		}
		u.ProfileID = p.ID
		u.ProfileName = p.Name
		u.Price = p.Price
		if u.Kind == "voucher" && u.UsedAt == "" {
			u.ExpiresAt = "" // jamais connecté : la validité du profil s'ancrera au 1er login
		} else {
			u.ExpiresAt = now.Add(time.Duration(p.ValidityMinutes()) * time.Minute).Format(time.RFC3339)
		}
	}
	if req.Comment != nil {
		u.Comment = strings.TrimSpace(*req.Comment)
	}

	if routerCopy != nil && routerCopy.Mode == "agent" {
		// Mode agent : persistance + commande user_set en file.
		a.store.Lock()
		payload := map[string]any{
			"oldName": agent.SanitizeName(oldUsername),
			"name":    agent.SanitizeName(u.Username),
		}
		if passwordChanged {
			payload["password"] = u.Password
		}
		if p := findProfileScoped(a.store.Data(), u.ProfileID, acc); p != nil {
			payload["profile"] = profileRef(*p)
		}
		if existing := findUserScoped(a.store.Data(), id, acc); existing != nil {
			*existing = u
		}
		cmd := queueCommandLocked(a.store.Data(), routerCopy.AccountID, routerCopy.ID, model.CmdUserSet, payload)
		a.logActivityBy(r, a.store.Data(), acc, "user", "Utilisateur "+u.Username+" modifié (en attente du routeur, commande "+cmd.ID+")")
		a.store.Save()
		a.store.Unlock()
		writeJSON(w, http.StatusOK, u)
		return
	}

	if routerCopy != nil {
		gw := a.gatewayFor(*routerCopy)
		if err := gw.SetUser(&u, oldUsername, passwordChanged); err != nil {
			writeErr(w, http.StatusBadRequest, "Mise à jour impossible : "+err.Error())
			return
		}
	} else {
		a.store.Lock()
		if existing := findUserScoped(a.store.Data(), id, acc); existing != nil {
			*existing = u
		}
		a.store.Unlock()
	}

	a.store.Lock()
	a.logActivityBy(r, a.store.Data(), acc, "user", "Utilisateur "+u.Username+" modifié")
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, u)
}

func (a *API) handleUserEnable(w http.ResponseWriter, r *http.Request) {
	a.userSetStatus(w, r, "enable")
}

func (a *API) handleUserDisable(w http.ResponseWriter, r *http.Request) {
	a.userSetStatus(w, r, "disable")
}

func (a *API) userSetStatus(w http.ResponseWriter, r *http.Request, action string) {
	acc := accountScope(r)
	id := r.PathValue("id")
	a.store.Lock()
	db := a.store.Data()
	cur := findUserScoped(db, id, acc)
	if cur == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Utilisateur introuvable")
		return
	}
	username := cur.Username
	router := findRouterScoped(db, cur.RouterID, acc)
	var routerCopy *model.Router
	if router != nil {
		c := *router
		routerCopy = &c
	}
	a.store.Unlock()

	var updated *model.HotspotUser
	if routerCopy != nil && routerCopy.Mode == "agent" {
		// Mode agent : persistance + commande user_set (disabled) en file.
		a.store.Lock()
		if u := findUserScoped(a.store.Data(), id, acc); u != nil {
			if action == "enable" {
				u.Status = "active"
			} else {
				u.Status = "disabled"
			}
			c := *u
			updated = &c
		}
		if updated != nil {
			queueCommandLocked(a.store.Data(), routerCopy.AccountID, routerCopy.ID, model.CmdUserSet, map[string]any{
				"oldName":  agent.SanitizeName(username),
				"name":     agent.SanitizeName(username),
				"disabled": action == "disable",
			})
			verb := "activé"
			if action == "disable" {
				verb = "désactivé"
			}
			a.logActivityBy(r, a.store.Data(), acc, "user", "Utilisateur "+username+" "+verb+" (en attente du routeur)")
			a.store.Save()
		}
		a.store.Unlock()
		if updated == nil {
			writeErr(w, http.StatusNotFound, "Utilisateur introuvable")
			return
		}
		writeJSON(w, http.StatusOK, updated)
		return
	}
	if routerCopy != nil {
		gw := a.gatewayFor(*routerCopy)
		var err error
		if action == "enable" {
			updated, err = gw.EnableUser(id)
		} else {
			updated, err = gw.DisableUser(id)
		}
		if err != nil {
			writeErr(w, http.StatusBadRequest, "Opération impossible : "+err.Error())
			return
		}
	} else {
		a.store.Lock()
		if u := findUserScoped(a.store.Data(), id, acc); u != nil {
			if action == "enable" {
				u.Status = "active"
			} else {
				u.Status = "disabled"
			}
			c := *u
			updated = &c
		}
		a.store.Unlock()
		if updated == nil {
			writeErr(w, http.StatusNotFound, "Utilisateur introuvable")
			return
		}
	}

	a.store.Lock()
	verb := "activé"
	if action == "disable" {
		verb = "désactivé"
	}
	a.logActivityBy(r, a.store.Data(), acc, "user", "Utilisateur "+username+" "+verb)
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, updated)
}

func (a *API) handleUserDelete(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	id := r.PathValue("id")
	a.store.Lock()
	db := a.store.Data()
	cur := findUserScoped(db, id, acc)
	if cur == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Utilisateur introuvable")
		return
	}
	username := cur.Username
	router := findRouterScoped(db, cur.RouterID, acc)
	var routerCopy *model.Router
	if router != nil {
		c := *router
		routerCopy = &c
	}
	a.store.Unlock()

	if routerCopy != nil && routerCopy.Mode == "agent" {
		// Mode agent : suppression immédiate côté cloud + commande user_remove en file.
		a.store.Lock()
		a.removeUserByID(id)
		queueCommandLocked(a.store.Data(), routerCopy.AccountID, routerCopy.ID, model.CmdUserRemove,
			map[string]any{"names": []string{agent.SanitizeName(username)}})
		a.logActivityBy(r, a.store.Data(), acc, "user", "Utilisateur "+username+" supprimé (en attente du routeur)")
		a.store.Save()
		a.store.Unlock()
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}

	if routerCopy != nil {
		gw := a.gatewayFor(*routerCopy)
		if err := gw.RemoveUser(id); err != nil {
			writeErr(w, http.StatusBadRequest, "Suppression impossible : "+err.Error())
			return
		}
	} else {
		a.store.Lock()
		a.removeUserByID(id)
		a.store.Unlock()
	}

	a.store.Lock()
	db = a.store.Data()
	sessions := db.Sessions[:0]
	for _, s := range db.Sessions {
		if s.UserID != id {
			sessions = append(sessions, s)
		}
	}
	db.Sessions = sessions
	a.logActivityBy(r, db, acc, "user", "Utilisateur "+username+" supprimé")
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *API) removeUserByID(id string) {
	db := a.store.Data()
	for i := range db.HotspotUsers {
		if db.HotspotUsers[i].ID == id {
			db.HotspotUsers = append(db.HotspotUsers[:i], db.HotspotUsers[i+1:]...)
			return
		}
	}
}
