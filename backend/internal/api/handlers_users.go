// handlers_users.go — gestion des utilisateurs hotspot (CRUD, statut, suppression).

package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"mikcloud/hotspot-api/internal/agent"
	"mikcloud/hotspot-api/internal/model"
	"mikcloud/hotspot-api/internal/store"
)

func (a *API) handleUsersList(w http.ResponseWriter, r *http.Request) {
	a.usersList(w, r, "")
}

// maskResellerCodes — la console gérant ne révèle JAMAIS les codes des tickets
// attribués à un revendeur (anti-« vente en direct » : le gérant ne peut plus
// dicter ni copier depuis les listes un ticket qui ne lui appartient pas — la
// vente passerait sur le dos du revendeur : créance dépôt-vente ou stock payé
// décompté chez LUI à la connexion du client, cash encaissé par le gérant).
// Le filtre `search` continue de tourner sur les VRAIS codes côté serveur
// (vérifier un ticket papier qui revient au comptoir reste possible) ; le
// seul canal de sortie des codes est l'impression tracée
// POST /api/vouchers/print — et le revendeur voit ses codes dans sa PWA
// Mode Vente (/api/sell/*). À appeler après filterUsers (copies), hors store.
func maskResellerCodes(users []model.HotspotUser) {
	for i := range users {
		if users[i].ResellerID != "" {
			users[i].Username = "••••••"
			users[i].Password = ""
		}
	}
}

// maskResellerCode — variante unitaire : masque la COPIE renvoyée au client
// (PUT /api/users/{id}, enable/disable, extend) sans toucher au stockage —
// la réponse d'une action ne doit pas devenir un canal de sortie du code.
func maskResellerCode(u model.HotspotUser) model.HotspotUser {
	if u.ResellerID != "" {
		u.Username = "••••••"
		u.Password = ""
	}
	return u
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
	maskResellerCodes(filtered)

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
	kind := req.Kind
	if kind == "" {
		kind = "regular"
	}
	u, cmdID, status, msg := a.createHotspotUser(r, acc, kind, req.Username, req.Password, req.ProfileID, req.RouterID, req.Comment)
	if status != 0 {
		writeErr(w, status, msg)
		return
	}
	if cmdID != "" {
		// Mode agent : persistance immédiate + commande user_add en file.
		data, _ := json.Marshal(u)
		var out map[string]any
		_ = json.Unmarshal(data, &out)
		out["queued"] = true
		out["commandId"] = cmdID
		writeJSON(w, http.StatusOK, out)
		return
	}
	writeJSON(w, http.StatusOK, u)
}

// createHotspotUser — cœur partagé de création d'un utilisateur hotspot
// (console + validation des inscriptions publiques N°27) : recherche du
// couple profil/routeur scopé compte, unicité du nom (avec génération
// automatique éventuelle), mot de passe auto, validité (régulier : dès la
// création ; voucher : ancrée au 1er login), puis persistance + file agent
// (user_add, tombstone levé) ou passage passerelle. Retourne (utilisateur,
// id de commande agent — vide sinon, statut HTTP d'erreur — 0 = succès,
// message d'erreur). Le verrou store est pris/rendu en interne.
func (a *API) createHotspotUser(r *http.Request, acc, kind, username, password, profileID, routerID, comment string) (model.HotspotUser, string, int, string) {
	now := time.Now().UTC()

	// Validation + construction (sous verrou)
	a.store.Lock()
	db := a.store.Data()
	profile := findProfileScoped(db, strings.TrimSpace(profileID), acc)
	if profile == nil {
		a.store.Unlock()
		return model.HotspotUser{}, "", http.StatusBadRequest, "Profil introuvable"
	}
	router := findRouterScoped(db, strings.TrimSpace(routerID), acc)
	if router == nil {
		a.store.Unlock()
		return model.HotspotUser{}, "", http.StatusBadRequest, "Routeur introuvable"
	}
	routerCopy := *router
	if kind != "regular" && kind != "voucher" {
		a.store.Unlock()
		return model.HotspotUser{}, "", http.StatusBadRequest, "Type d'utilisateur invalide (regular ou voucher)"
	}
	username = strings.TrimSpace(username)
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
		return model.HotspotUser{}, "", http.StatusBadRequest, "Ce nom d'utilisateur existe déjà"
	}
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
		Comment:   strings.TrimSpace(comment),
		CreatedAt: model.NowISO(),
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
		// Audit purge — création VOLONTAIRE : le tombstone éventuel (purge
		// antérieure de ce nom) est LEVÉ — l'opérateur réclame l'identifiant,
		// la purge ne doit plus le bloquer (ni ici, ni sur le routeur).
		liftPurgeTombstone(db, acc, u.Username)
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
		return u, cmdID, 0, ""
	}

	gw := a.gatewayFor(routerCopy)
	if err := gw.AddUser(&u); err != nil {
		return u, "", http.StatusBadRequest, "Création impossible : " + err.Error()
	}

	a.store.Lock()
	// Audit purge — même levée de tombstone pour la création passerelle.
	liftPurgeTombstone(a.store.Data(), acc, agent.SanitizeName(u.Username))
	a.logActivityBy(r, a.store.Data(), acc, "user", "Utilisateur "+u.Username+" créé")
	a.store.Save()
	a.store.Unlock()
	return u, "", 0, ""
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
	// Garde propriété — le code d'un ticket attribué à un revendeur n'est pas
	// modifiable depuis la console : réécrire username/password permettrait de
	// contourner le masquage des listes (poser un code connu puis le dicter au
	// comptoir = vente en direct aux dépens du revendeur). La voie propre pour
	// récupérer un ticket reste le retour de stock (recrédite le revendeur).
	// Un écho strict du username inchangé est toléré (clients qui renvoient
	// le formulaire complet) ; le commentaire/profil/statut restent éditables
	// (outils de gestion et anti-fraude du gérant).
	if cur.ResellerID != "" {
		usernameEcho := req.Username != nil && strings.TrimSpace(*req.Username) == cur.Username
		if (req.Username != nil && !usernameEcho) || (req.Password != nil && *req.Password != "") {
			a.store.Unlock()
			writeErrCode(w, http.StatusForbidden, "reseller_voucher_locked",
				"Ticket attribué à "+cur.ResellerName+" : le code n'est pas modifiable depuis la console (effectuez une reprise ou un retour de stock pour le récupérer)",
				map[string]any{"resellerId": cur.ResellerID, "resellerName": cur.ResellerName})
			return
		}
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
		writeJSON(w, http.StatusOK, maskResellerCode(u))
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
	writeJSON(w, http.StatusOK, maskResellerCode(u))
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
		writeJSON(w, http.StatusOK, maskResellerCode(*updated))
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
	writeJSON(w, http.StatusOK, maskResellerCode(*updated))
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
	// N°23 (W1) — même philosophie que la garde de code (N°22) : un ticket
	// attribué à un revendeur n'est pas destructible depuis la console —
	// sa suppression effacerait le stock du revendeur ET la preuve de la
	// créance. La voie propre : reprise par le gérant (N°23) ou retour de
	// stock par le revendeur (N°20).
	if cur.ResellerID != "" {
		a.store.Unlock()
		writeErrCode(w, http.StatusForbidden, "reseller_voucher_locked",
			"Ticket attribué à "+cur.ResellerName+" : suppression impossible (effectuez une reprise pour le récupérer, ou faites-le retourner par le revendeur)",
			map[string]any{"resellerId": cur.ResellerID, "resellerName": cur.ResellerName})
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

// filterUsers — filtrage scopé au compte avec les MÊMES règles que la liste
// (kind/search/status/profileId + routerId pour l'export) ; le statut filtré
// et renvoyé est le statut RÉSOLU (ResolvedStatus — 5 états, dont « online »).
// Tri par création décroissante. À appeler sous verrou.
func filterUsers(db *model.DB, acc string, q url.Values, now time.Time) []model.HotspotUser {
	kind := q.Get("kind")
	search := strings.ToLower(strings.TrimSpace(q.Get("search")))
	status := q.Get("status")
	profileID := q.Get("profileId")
	routerID := strings.TrimSpace(q.Get("routerId"))
	// N°23 (W3/W4) — détenteur du stock : direct (gérant) ou revendeur
	// (alloué). Additif : absent = aucun effet (contrat liste préservé).
	holder := q.Get("holder")
	online := onlineSessions(db, now) // sessions live (routeurs vus < 3 min)

	filtered := []model.HotspotUser{}
	for i := range db.HotspotUsers {
		u := &db.HotspotUsers[i]
		if u.AccountID != acc {
			continue
		}
		if kind != "" && u.Kind != kind {
			continue
		}
		if profileID != "" && u.ProfileID != profileID {
			continue
		}
		if routerID != "" && u.RouterID != routerID {
			continue
		}
		switch holder {
		case "reseller":
			if u.ResellerID == "" {
				continue
			}
		case "direct":
			if u.ResellerID != "" {
				continue
			}
		}
		st := model.ResolvedStatus(u, online[onlineKey(u)], now)
		if status != "" && st != status {
			continue
		}
		if search != "" {
			hay := strings.ToLower(u.Username + " " + u.Comment + " " + u.ResellerName + " " + u.ProfileName + " " + u.BatchID)
			if !strings.Contains(hay, search) {
				continue
			}
		}
		uc := *u
		uc.Status = st
		uc.Disabled = u.Status == "disabled" // miroir du statut stocké (toggle UI)
		filtered = append(filtered, uc)
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].CreatedAt > filtered[j].CreatedAt })
	return filtered
}
