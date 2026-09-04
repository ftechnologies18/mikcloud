// Package api — actions utilisateurs (F4/F5, audit Mikhmon) : reset stats,
// prolongation (extend), export CSV, actions groupées (bulk) et nettoyage
// des utilisateurs expirés (cleanup).
package api

import (
	"fmt"
	"mikcloud/hotspot-api/internal/agent"
	"mikcloud/hotspot-api/internal/model"
	"mikcloud/hotspot-api/internal/store"
	"net/http"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// F4 — Actions utilisateurs : reset stats / prolonger / export CSV
// ---------------------------------------------------------------------------

// handleUserResetStats — POST /api/users/{id}/reset-stats : remet à zéro les
// compteurs cloud (bytesIn/bytesOut/uptimeUsedSec) ; en mode agent, une
// commande user_reset applique la même remise à zéro sur le routeur.
func (a *API) handleUserResetStats(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	id := r.PathValue("id")
	a.store.Lock()
	db := a.store.Data()
	u := findUserScoped(db, id, acc)
	if u == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Utilisateur introuvable")
		return
	}
	username := u.Username
	u.BytesIn = 0
	u.BytesOut = 0
	u.UptimeUsedSec = 0
	u.UsedAt = "" // retour à « jamais connecté »
	if u.Status == "used" {
		u.Status = "active"
	}
	var routerCopy *model.Router
	if rr := findRouterScoped(db, u.RouterID, acc); rr != nil {
		c := *rr
		routerCopy = &c
	}
	if routerCopy != nil && routerCopy.Mode == "agent" {
		queueCommandLocked(db, routerCopy.AccountID, routerCopy.ID, model.CmdUserReset, map[string]any{
			"name": agent.SanitizeName(username),
		})
	}
	a.logActivityBy(r, db, acc, "user", "Statistiques de "+username+" réinitialisées")
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleUserExtend — POST /api/users/{id}/extend {days} : nouvelle expiration
// = max(maintenant, expiration actuelle) + days. Un utilisateur « expired »
// repasse « active » (et l'agent le ré-active sur le routeur).
func (a *API) handleUserExtend(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	id := r.PathValue("id")
	var req struct {
		Days int `json:"days"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	if req.Days < 1 || req.Days > 3650 {
		writeErr(w, http.StatusBadRequest, "Le nombre de jours doit être compris entre 1 et 3650")
		return
	}
	now := time.Now().UTC()

	a.store.Lock()
	db := a.store.Data()
	u := findUserScoped(db, id, acc)
	if u == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Utilisateur introuvable")
		return
	}
	if u.Kind == "voucher" && u.UsedAt == "" && u.ExpiresAt == "" {
		// Ancrage 1er login : un ticket jamais connecté n'a pas d'échéance —
		// la validité du stock se règle via le profil, pas par prolongation.
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, "Ticket jamais connecté : sa validité démarrera au premier login (selon son profil). Pour l'ajuster, modifiez le profil.")
		return
	}
	// Base : l'expiration future est conservée, une expiration passée repart
	// de maintenant (jamais dans le passé).
	base := now
	if u.ExpiresAt != "" {
		if exp, err := time.Parse(time.RFC3339, u.ExpiresAt); err == nil && exp.After(base) {
			base = exp
		}
	}
	u.ExpiresAt = base.Add(time.Duration(req.Days) * 24 * time.Hour).Format(time.RFC3339)
	wasExpired := false
	if u.Status == "expired" {
		wasExpired = true
		u.Status = "active"
		u.Enforced = true // rien à pousser : l'utilisateur redevient actif
	}
	var routerCopy *model.Router
	if rr := findRouterScoped(db, u.RouterID, acc); rr != nil {
		c := *rr
		routerCopy = &c
	}
	// Mode agent : si l'utilisateur était expiré, il a été désactivé/supprimé
	// sur le routeur — le ré-activer (user_set disabled=false).
	if wasExpired && routerCopy != nil && routerCopy.Mode == "agent" {
		name := agent.SanitizeName(u.Username)
		queueCommandLocked(db, routerCopy.AccountID, routerCopy.ID, model.CmdUserSet, map[string]any{
			"oldName": name, "name": name, "disabled": false,
		})
	}
	updated := *u
	a.logActivityBy(r, db, acc, "user", fmt.Sprintf("Utilisateur %s prolongé de %d j", u.Username, req.Days))
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, updated)
}

// handleUsersBulk — POST /api/users/bulk {ids, action, days} : actions
// groupées sur une sélection d'utilisateurs (sélection multiple côté UI).
// Actions : enable, disable, delete, extend, reset-stats — sémantique
// identique aux handlers unitaires (userSetStatus, handleUserDelete,
// handleUserExtend, handleUserResetStats) :
//   - routeur agent  : mutation cloud immédiate + commandes en file
//     (user_remove regroupées par routeur — jusqu'à 200 noms par commande) ;
//   - real/simulated : appels passerelle directs (le miroir cloud est tenu à
//     jour par la passerelle elle-même) ;
//   - orphelin       : mutation cloud seule (routeur introuvable).
//
// Réponse {ok, processed, failed} — failed compte les introuvables et échecs.
func (a *API) handleUsersBulk(w http.ResponseWriter, r *http.Request) {
	// P3 — compte suspendu/expiré : écritures refusées.
	if !a.guardAccountWrite(w, r) {
		return
	}
	acc := accountScope(r)
	var req struct {
		IDs    []string `json:"ids"`
		Action string   `json:"action"`
		Days   int      `json:"days"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	if len(req.IDs) == 0 {
		writeErr(w, http.StatusBadRequest, "Aucun utilisateur sélectionné")
		return
	}
	if len(req.IDs) > 200 {
		writeErr(w, http.StatusBadRequest, "Sélection trop importante (200 utilisateurs maximum)")
		return
	}
	switch req.Action {
	case "enable", "disable", "delete", "extend", "reset-stats":
	default:
		writeErr(w, http.StatusBadRequest, "Action inconnue")
		return
	}
	// Cohérent avec DELETE /api/users/{id} : la suppression exige le rôle 2.
	if req.Action == "delete" {
		if c := claimsFrom(r); c == nil || roleRank(c.Role) < 2 {
			writeErr(w, http.StatusForbidden, "Accès refusé — rôle insuffisant pour cette action")
			return
		}
	}
	if req.Action == "extend" && (req.Days < 1 || req.Days > 3650) {
		writeErr(w, http.StatusBadRequest, "Le nombre de jours doit être compris entre 1 et 3650")
		return
	}
	now := time.Now().UTC()

	type gwTask struct {
		id  string
		rid string
	}
	type target struct {
		user   model.HotspotUser
		router *model.Router
	}
	a.store.Lock()
	db := a.store.Data()
	processed, failed := 0, 0
	gwTasks := []gwTask{}
	gwRouters := map[string]model.Router{} // copies pour la phase B
	agentRemove := map[string][]string{}   // routerID -> noms (delete, agent)
	removedIDs := map[string]bool{}        // sessions à purger (delete)

	// prolonge — même règle que handleUserExtend : base = expiration future
	// conservée, sinon maintenant ; un expiré repasse « active » (+ user_set
	// agent pour le réactiver sur le routeur).
	prolonge := func(u *model.HotspotUser, rc *model.Router) {
		if u.Kind == "voucher" && u.UsedAt == "" && u.ExpiresAt == "" {
			return // jamais connecté : validité ancrée au 1er login — rien à prolonger
		}
		base := now
		if u.ExpiresAt != "" {
			if exp, err := time.Parse(time.RFC3339, u.ExpiresAt); err == nil && exp.After(base) {
				base = exp
			}
		}
		u.ExpiresAt = base.Add(time.Duration(req.Days) * 24 * time.Hour).Format(time.RFC3339)
		if u.Status == "expired" {
			u.Status = "active"
			u.Enforced = true // rien à pousser : l'utilisateur redevient actif
			if rc != nil && rc.Mode == "agent" {
				name := agent.SanitizeName(u.Username)
				queueCommandLocked(db, rc.AccountID, rc.ID, model.CmdUserSet, map[string]any{
					"oldName": name, "name": name, "disabled": false,
				})
			}
		}
	}

	// Phase A — snapshot des cibles + mutations cloud (agents et orphelins).
	targets := make([]target, 0, len(req.IDs))
	seen := map[string]bool{}
	for _, id := range req.IDs {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		u := findUserScoped(db, id, acc)
		if u == nil {
			failed++
			continue
		}
		var rc *model.Router
		if rr := findRouterScoped(db, u.RouterID, acc); rr != nil {
			c := *rr
			rc = &c
			gwRouters[c.ID] = c
		}
		targets = append(targets, target{user: *u, router: rc})
	}
	for _, tg := range targets {
		u := findUserScoped(db, tg.user.ID, acc)
		if u == nil { // disparu entre-temps (course interne)
			failed++
			continue
		}
		rc := tg.router
		switch {
		case rc != nil && rc.Mode == "agent":
			name := agent.SanitizeName(u.Username)
			switch req.Action {
			case "enable", "disable":
				if req.Action == "enable" {
					u.Status = "active"
				} else {
					u.Status = "disabled"
				}
				queueCommandLocked(db, rc.AccountID, rc.ID, model.CmdUserSet, map[string]any{
					"oldName": name, "name": name, "disabled": req.Action == "disable",
				})
				processed++
			case "delete":
				a.removeUserByID(u.ID)
				agentRemove[rc.ID] = append(agentRemove[rc.ID], name)
				removedIDs[u.ID] = true
				processed++
			case "extend":
				prolonge(u, rc)
				processed++
			case "reset-stats":
				u.BytesIn, u.BytesOut, u.UptimeUsedSec = 0, 0, 0
				u.UsedAt = "" // retour à « jamais connecté »
				if u.Status == "used" {
					u.Status = "active"
				}
				queueCommandLocked(db, rc.AccountID, rc.ID, model.CmdUserReset, map[string]any{"name": name})
				processed++
			}
		case rc == nil:
			// Orphelin : mutation cloud seule, comme les handlers unitaires.
			switch req.Action {
			case "enable":
				u.Status = "active"
			case "disable":
				u.Status = "disabled"
			case "delete":
				a.removeUserByID(u.ID)
				removedIDs[u.ID] = true
			case "extend":
				prolonge(u, nil)
			case "reset-stats":
				u.BytesIn, u.BytesOut, u.UptimeUsedSec = 0, 0, 0
				u.UsedAt = "" // retour à « jamais connecté »
				if u.Status == "used" {
					u.Status = "active"
				}
			}
			processed++
		default:
			// real/simulated : la passerelle décide (phase B) — sauf
			// extend/reset-stats qui ne poussent rien (statut cloud suffit).
			switch req.Action {
			case "extend":
				prolonge(u, rc)
				processed++
			case "reset-stats":
				u.BytesIn, u.BytesOut, u.UptimeUsedSec = 0, 0, 0
				u.UsedAt = "" // retour à « jamais connecté »
				if u.Status == "used" {
					u.Status = "active"
				}
				processed++
			case "enable", "disable", "delete":
				gwTasks = append(gwTasks, gwTask{id: u.ID, rid: rc.ID})
			}
		}
	}
	for rid, names := range agentRemove {
		if rc := findRouterScoped(db, rid, acc); rc != nil {
			queueCommandLocked(db, rc.AccountID, rid, model.CmdUserRemove, map[string]any{"names": names})
		}
	}
	names := make([]string, 0, len(targets))
	for _, tg := range targets {
		names = append(names, tg.user.Username)
	}
	shown, suffix := names, ""
	if len(names) > 10 {
		shown, suffix = names[:10], fmt.Sprintf(" … (+%d autres)", len(names)-10)
	}
	a.logActivityBy(r, db, acc, "user", fmt.Sprintf("Action groupée « %s » sur %d utilisateur(s) : %s%s",
		req.Action, len(targets), strings.Join(shown, ", "), suffix))
	a.store.Save()
	a.store.Unlock()

	// Phase B — routeurs real/simulated : appels passerelle hors verrou (la
	// passerelle tient le miroir cloud à jour elle-même).
	for _, task := range gwTasks {
		rc, ok := gwRouters[task.rid]
		if !ok {
			failed++
			continue
		}
		gw := a.gatewayFor(rc)
		var err error
		switch req.Action {
		case "enable":
			_, err = gw.EnableUser(task.id)
		case "disable":
			_, err = gw.DisableUser(task.id)
		case "delete":
			err = gw.RemoveUser(task.id)
			if err == nil {
				removedIDs[task.id] = true
			}
		}
		if err != nil {
			failed++
			continue
		}
		processed++
	}

	// Phase C — purge des sessions des utilisateurs supprimés.
	if req.Action == "delete" && len(removedIDs) > 0 {
		a.store.Lock()
		db = a.store.Data()
		sessions := db.Sessions[:0]
		for _, s := range db.Sessions {
			if !removedIDs[s.UserID] {
				sessions = append(sessions, s)
			}
		}
		db.Sessions = sessions
		a.store.Save()
		a.store.Unlock()
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "processed": processed, "failed": failed})
}

// handleUsersExport — GET /api/users/export : CSV de la liste utilisateurs
// avec les MÊMES filtres que handleUsersList (kind/search/status/profileId).
func (a *API) handleUsersExport(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	now := time.Now().UTC()
	a.store.Lock()
	db := a.store.Data()
	store.Tick(db, now)  // statuts à jour (même comportement que la liste)
	a.enforceExpired(db) // F1 : enforcement au passage
	users := filterUsers(db, acc, r.URL.Query(), now)
	a.store.Save()
	a.store.Unlock()

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\"mikcloud-utilisateurs.csv\"")
	// BOM UTF-8 : Excel reconnaît l'encodage.
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})
	// Colonnes trafic : RouterOS bytes-in = upload, bytes-out = download
	// (doc officielle HotSpot) — libellés explicites côté client, sans
	// ambiguïté de point de vue ; l'ordre des colonnes est inchangé.
	_, _ = w.Write([]byte("Utilisateur;Mot de passe;Profil;Statut;Routeur;Créé le;Expire le;" +
		"Upload (Mo);Download (Mo);Prix;Revendeur;Commentaire\r\n"))
	for _, u := range users {
		_, _ = w.Write([]byte(fmt.Sprintf("%s;%s;%s;%s;%s;%s;%s;%d;%d;%d;%s;%s\r\n",
			csvField(u.Username), csvField(u.Password), csvField(u.ProfileName), csvField(u.Status),
			csvField(u.RouterName), csvField(u.CreatedAt), csvField(u.ExpiresAt),
			u.BytesIn/1_048_576, u.BytesOut/1_048_576, u.Price,
			csvField(u.ResellerName), csvField(u.Comment))))
	}
}

// ---------------------------------------------------------------------------
// F5 — Nettoyage des utilisateurs expirés
// ---------------------------------------------------------------------------

// handleUsersCleanup — POST /api/users/cleanup {mode:"expired"} : supprime du
// cloud TOUS les utilisateurs « expired » du compte ; en mode agent, une
// commande user_remove par paquet de 50 noms est déposée par routeur.
func (a *API) handleUsersCleanup(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	var req struct {
		Mode string `json:"mode"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	if req.Mode != "expired" {
		writeErr(w, http.StatusBadRequest, "Mode de nettoyage invalide (expired)")
		return
	}
	now := time.Now().UTC()
	a.store.Lock()
	db := a.store.Data()
	store.Tick(db, now) // applique les expirations en attente avant le nettoyage

	// Collecte des utilisateurs expirés du compte, groupés par routeur.
	type target struct {
		id, username string
	}
	byRouter := map[string][]string{} // routerID → usernames (non sanitizés)
	names := map[string]bool{}        // usernames cibles
	targets := []target{}
	for _, u := range db.HotspotUsers {
		if u.AccountID != acc || u.Status != "expired" {
			continue
		}
		targets = append(targets, target{id: u.ID, username: u.Username})
		names[u.Username] = true
		byRouter[u.RouterID] = append(byRouter[u.RouterID], agent.SanitizeName(u.Username))
	}
	if len(targets) == 0 {
		a.store.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "removed": 0})
		return
	}

	// Suppression cloud.
	targetIDs := map[string]bool{}
	for _, t := range targets {
		targetIDs[t.id] = true
	}
	kept := db.HotspotUsers[:0]
	for _, u := range db.HotspotUsers {
		if targetIDs[u.ID] {
			continue
		}
		kept = append(kept, u)
	}
	db.HotspotUsers = kept
	// Les sessions des utilisateurs supprimés sont abandonnées (purge Tick).
	keptSessions := db.Sessions[:0]
	for _, s := range db.Sessions {
		if names[s.Username] {
			continue
		}
		keptSessions = append(keptSessions, s)
	}
	db.Sessions = keptSessions

	// Commandes agent : user_remove par paquets de 50 noms et par routeur.
	for routerID, list := range byRouter {
		rr := findRouterScoped(db, routerID, acc)
		if rr == nil || rr.Mode != "agent" {
			continue
		}
		for start := 0; start < len(list); start += 50 {
			end := start + 50
			if end > len(list) {
				end = len(list)
			}
			queueCommandLocked(db, rr.AccountID, rr.ID, model.CmdUserRemove, map[string]any{
				"names": list[start:end],
			})
		}
	}

	removed := len(targets)
	a.logActivityBy(r, db, acc, "user", fmt.Sprintf("Nettoyage manuel : %d utilisateurs expirés supprimés", removed))
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "removed": removed})
}
