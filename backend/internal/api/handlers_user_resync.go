// Package api — resynchronisation utilisateur « absent du routeur » (N).
//
// Rapprochement doux : quand un utilisateur actif du cloud n'est plus listé
// par le read_state du routeur (supprimé directement dans Winbox, commande
// échouée…), il est marqué MissingOnRouter (badge « absent du routeur » dans
// la console). RIEN n'est supprimé automatiquement — l'opérateur tranche :
//
//	POST /api/users/{id}/resync  {action: "recreate" | "forget"}
//
//   - recreate : renvoie l'utilisateur au routeur (user_add en file pour un
//     routeur agent, ajout direct pour un routeur réel) avec ses identifiants,
//     son profil et son quota d'origine ; le badge se lève au read_state
//     suivant ;
//   - forget : retire l'utilisateur du cloud (il n'existe déjà plus sur le
//     routeur — le cloud se met en accord avec la réalité terrain).

package api

import (
	"net/http"
	"strings"

	"mikcloud/hotspot-api/internal/agent"
	"mikcloud/hotspot-api/internal/model"
)

// handleUserResync — POST /api/users/{id}/resync {action: "recreate"|"forget"}.
func (a *API) handleUserResync(w http.ResponseWriter, r *http.Request) {
	// P3 — compte expiré : écritures métier refusées (lecture seule).
	if !a.guardAccountWrite(w, r) {
		return
	}
	acc := accountScope(r)
	id := r.PathValue("id")

	var req struct {
		Action string `json:"action"` // recreate | forget
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	action := strings.ToLower(strings.TrimSpace(req.Action))
	if action != "recreate" && action != "forget" {
		writeErr(w, http.StatusBadRequest, "Action inconnue (recreate | forget)")
		return
	}

	a.store.Lock()
	db := a.store.Data()
	var user *model.HotspotUser
	var router *model.Router
	for i := range db.HotspotUsers {
		if db.HotspotUsers[i].ID == id && db.HotspotUsers[i].AccountID == acc {
			user = &db.HotspotUsers[i]
			break
		}
	}
	if user == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Utilisateur introuvable")
		return
	}
	for i := range db.Routers {
		if db.Routers[i].ID == user.RouterID && db.Routers[i].AccountID == acc {
			router = &db.Routers[i]
			break
		}
	}

	// Oublier : le retirer du cloud (il n'existe déjà plus côté routeur).
	if action == "forget" {
		username := user.Username
		db.HotspotUsers = append(db.HotspotUsers[:indexOfHotspotUser(db, id)], db.HotspotUsers[indexOfHotspotUser(db, id)+1:]...)
		// Sessions rattachées closes (un utilisateur sans entrée n'a plus de sens).
		kept := db.Sessions[:0]
		for _, s := range db.Sessions {
			if s.UserID != id {
				kept = append(kept, s)
			}
		}
		db.Sessions = kept
		a.logActivityBy(r, db, acc, "user", "Utilisateur "+username+" retiré du cloud (absent du routeur — resynchronisation « oublier »)")
		a.store.Save()
		a.store.Unlock()
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}

	// Recréer : renvoyer l'utilisateur au routeur avec ses attributs d'origine.
	if router == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Routeur d'origine introuvable — impossible de recréer l'utilisateur")
		return
	}
	username := user.Username
	password := user.Password
	profileName := user.ProfileName
	comment := user.Comment
	quota := user.DataQuotaMb
	// Le profil est résolu pour la référence complète (débit, timeout, shared).
	var profile *model.Profile
	for i := range db.Profiles {
		if db.Profiles[i].AccountID == acc && (db.Profiles[i].ID == user.ProfileID || db.Profiles[i].Name == user.ProfileName) {
			profile = &db.Profiles[i]
			break
		}
	}
	routerMode := router.Mode
	routerID, routerAcc := router.ID, router.AccountID

	if routerMode == "agent" {
		payload := map[string]any{
			"name":     agent.SanitizeName(username),
			"password": password,
			"comment":  comment,
		}
		if profile != nil {
			payload["profile"] = profileRef(*profile)
		} else {
			payload["profile"] = map[string]any{"name": agent.SanitizeName(profileName)}
		}
		if quota > 0 {
			payload["limitBytesTotal"] = quota * 1048576
		}
		queueCommandLocked(db, routerAcc, routerID, model.CmdUserAdd, payload)
		// Le badge se lèvera au read_state suivant (l'utilisateur réapparaîtra).
		user.MissingOnRouter = false
		a.logActivityBy(r, db, acc, "user", "Utilisateur "+username+" renvoyé au routeur (resynchronisation « recréer »)")
		a.store.Save()
		a.store.Unlock()
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true, "queued": true})
		return
	}

	// Routeur réel (API directe) : ajout immédiat.
	routerCopy := *router
	a.store.Unlock()
	gw := a.gatewayFor(routerCopy)
	uCopy := *user
	uCopy.Username = agent.SanitizeName(uCopy.Username)
	if err := gw.AddUser(&uCopy); err != nil {
		writeErr(w, http.StatusBadRequest, "Recréation impossible : "+err.Error())
		return
	}
	a.store.Lock()
	for i := range db.HotspotUsers {
		if db.HotspotUsers[i].ID == id {
			db.HotspotUsers[i].MissingOnRouter = false
			break
		}
	}
	a.logActivityBy(r, a.store.Data(), acc, "user", "Utilisateur "+username+" recréé sur le routeur (resynchronisation)")
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// indexOfHotspotUser — index de l'utilisateur (préconditions vérifiées par l'appelant).
func indexOfHotspotUser(db *model.DB, id string) int {
	for i := range db.HotspotUsers {
		if db.HotspotUsers[i].ID == id {
			return i
		}
	}
	return -1
}
