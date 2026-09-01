// Package api — handlers de la vague P0 (audit Mikhmon) : templates de vouchers
// (F2), journal utilisateurs (F3), actions utilisateurs (F4 : reset stats /
// prolonger / export CSV, F5 : nettoyage des expirés) et moteur d'enforcement
// de l'expiration cloud vers les routeurs agents (F1).
//
// Les routes sont enregistrées dans Handler() (handlers.go, bloc « P0 »).
package api

import (
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"mikcloud/hotspot-api/internal/agent"
	"mikcloud/hotspot-api/internal/model"
	"mikcloud/hotspot-api/internal/store"
)

// ---------------------------------------------------------------------------
// F1 — Enforcement de l'expiration vers les routeurs
// ---------------------------------------------------------------------------

// enforceExpired — applique au routeur l'expiration de chaque utilisateur
// « expired » non encore appliqué (Enforced == false) :
//
//   - profil expMode « remove »  → commande agent user_remove ;
//   - profil expMode « notify »  → commande agent user_set {disabled:true} ;
//   - routeurs simulated/real    → rien à pousser : le statut cloud suffit
//     (l'utilisateur reste en historique — cf. contrat F1).
//
// Enforced est marqué dans TOUS les modes (l'opération n'est exécutée qu'une
// fois par expiration ; un « extend » ultérieur repasse le statut « active »).
// À appeler sous verrou, après store.Tick (applyExpiry) ; le Save est à charge
// de l'appelant. Les commandes sont servies à l'agent à son prochain check-in.
func (a *API) enforceExpired(db *model.DB) {
	expMode := make(map[string]string, len(db.Profiles))
	for _, p := range db.Profiles {
		m := p.ExpMode
		if m == "" {
			m = "notify"
		}
		expMode[p.ID] = m
	}
	routers := make(map[string]*model.Router, len(db.Routers))
	for i := range db.Routers {
		routers[db.Routers[i].ID] = &db.Routers[i]
	}
	for i := range db.HotspotUsers {
		u := &db.HotspotUsers[i]
		if u.Status != "expired" || u.Enforced {
			continue
		}
		u.Enforced = true
		rr := routers[u.RouterID]
		if rr == nil || rr.Mode != "agent" {
			continue // simulated/real : le statut cloud suffit
		}
		name := agent.SanitizeName(u.Username)
		if expMode[u.ProfileID] == "remove" {
			queueCommandLocked(db, rr.AccountID, rr.ID, model.CmdUserRemove, map[string]any{
				"names": []string{name},
			})
		} else {
			queueCommandLocked(db, rr.AccountID, rr.ID, model.CmdUserSet, map[string]any{
				"oldName": name, "name": name, "disabled": true,
			})
		}
	}
}

// ---------------------------------------------------------------------------
// Filtre partagé des listes d'utilisateurs (liste JSON + export CSV F4)
// ---------------------------------------------------------------------------

// onlineSessions — ensemble des clés « routerID|username » ayant une session
// live, restreint aux routeurs vus depuis moins de 3 minutes (garde : au-delà,
// les sessions cloud peuvent être figées — on ne promet plus « en ligne »).
// À appeler sous verrou.
func onlineSessions(db *model.DB, now time.Time) map[string]bool {
	seen := make(map[string]bool, len(db.Routers))
	for i := range db.Routers {
		if t, err := time.Parse(time.RFC3339, db.Routers[i].LastSeen); err == nil && now.Sub(t) <= 3*time.Minute {
			seen[db.Routers[i].ID] = true
		}
	}
	online := make(map[string]bool, len(db.Sessions))
	for _, s := range db.Sessions {
		if seen[s.RouterID] {
			online[s.RouterID+"|"+strings.ToLower(s.Username)] = true
		}
	}
	return online
}

// onlineKey — clé de session live d'un utilisateur (voir onlineSessions).
func onlineKey(u *model.HotspotUser) string {
	return u.RouterID + "|" + strings.ToLower(u.Username)
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

// ---------------------------------------------------------------------------
// F2 — Modèles (templates) de vouchers
// ---------------------------------------------------------------------------

// templateFormats — formats d'impression supportés (contrat F2).
var templateFormats = map[string]bool{"a4": true, "58mm": true, "80mm": true}

func (a *API) handleTemplatesList(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	a.store.Lock()
	templates := []model.VoucherTemplate{}
	for _, t := range a.store.Data().Templates {
		if t.AccountID == acc {
			templates = append(templates, t)
		}
	}
	a.store.Unlock()

	// Tri : modèle par défaut d'abord, puis plus récent.
	sort.Slice(templates, func(i, j int) bool {
		if templates[i].IsDefault != templates[j].IsDefault {
			return templates[i].IsDefault
		}
		return templates[i].CreatedAt > templates[j].CreatedAt
	})
	writeJSON(w, http.StatusOK, templates)
}

func (a *API) handleTemplateCreate(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	var req struct {
		Name      string `json:"name"`
		Format    string `json:"format"`
		BodyHTML  string `json:"bodyHtml"`
		IsDefault bool   `json:"isDefault"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	name := strings.TrimSpace(req.Name)
	if len(name) < 1 || len(name) > 60 {
		writeErr(w, http.StatusBadRequest, "Le nom du modèle doit faire entre 1 et 60 caractères")
		return
	}
	format := strings.TrimSpace(req.Format)
	if !templateFormats[format] {
		writeErr(w, http.StatusBadRequest, "Format invalide (a4, 58mm ou 80mm)")
		return
	}
	if len(req.BodyHTML) > 20000 {
		writeErr(w, http.StatusBadRequest, "Le corps du modèle doit faire au plus 20 000 caractères")
		return
	}
	body := model.SanitizeTemplateHTML(req.BodyHTML)

	a.store.Lock()
	db := a.store.Data()
	if req.IsDefault {
		for i := range db.Templates {
			if db.Templates[i].AccountID == acc {
				db.Templates[i].IsDefault = false
			}
		}
	}
	tpl := model.VoucherTemplate{
		ID: model.NewID("tpl-"), AccountID: acc, Name: name, Format: format,
		BodyHTML: body, IsDefault: req.IsDefault, CreatedAt: model.NowISO(),
	}
	db.Templates = append(db.Templates, tpl)
	a.logActivityBy(r, db, acc, "voucher", "Modèle de voucher «"+name+"» créé")
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusCreated, tpl)
}

func (a *API) handleTemplateUpdate(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	id := r.PathValue("id")
	var req struct {
		Name      *string `json:"name"`
		Format    *string `json:"format"`
		BodyHTML  *string `json:"bodyHtml"`
		IsDefault *bool   `json:"isDefault"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	if req.Name != nil && (len(strings.TrimSpace(*req.Name)) < 1 || len(strings.TrimSpace(*req.Name)) > 60) {
		writeErr(w, http.StatusBadRequest, "Le nom du modèle doit faire entre 1 et 60 caractères")
		return
	}
	if req.Format != nil && !templateFormats[strings.TrimSpace(*req.Format)] {
		writeErr(w, http.StatusBadRequest, "Format invalide (a4, 58mm ou 80mm)")
		return
	}
	if req.BodyHTML != nil && len(*req.BodyHTML) > 20000 {
		writeErr(w, http.StatusBadRequest, "Le corps du modèle doit faire au plus 20 000 caractères")
		return
	}

	a.store.Lock()
	db := a.store.Data()
	var tpl *model.VoucherTemplate
	for i := range db.Templates {
		if db.Templates[i].ID == id && db.Templates[i].AccountID == acc {
			tpl = &db.Templates[i]
			break
		}
	}
	if tpl == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Modèle introuvable")
		return
	}
	if req.Name != nil {
		tpl.Name = strings.TrimSpace(*req.Name)
	}
	if req.Format != nil {
		tpl.Format = strings.TrimSpace(*req.Format)
	}
	if req.BodyHTML != nil {
		tpl.BodyHTML = model.SanitizeTemplateHTML(*req.BodyHTML)
	}
	if req.IsDefault != nil {
		if *req.IsDefault {
			for i := range db.Templates {
				if db.Templates[i].AccountID == acc && db.Templates[i].ID != id {
					db.Templates[i].IsDefault = false
				}
			}
		}
		tpl.IsDefault = *req.IsDefault
	}
	updated := *tpl
	a.logActivityBy(r, db, acc, "voucher", "Modèle de voucher «"+updated.Name+"» modifié")
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, updated)
}

func (a *API) handleTemplateDelete(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	id := r.PathValue("id")
	a.store.Lock()
	db := a.store.Data()
	accTemplates := 0
	idx := -1
	for i := range db.Templates {
		if db.Templates[i].AccountID != acc {
			continue
		}
		accTemplates++
		if db.Templates[i].ID == id {
			idx = i
		}
	}
	if idx < 0 {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Modèle introuvable")
		return
	}
	if accTemplates <= 1 {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, "Impossible de supprimer le dernier modèle du compte")
		return
	}
	name := db.Templates[idx].Name
	db.Templates = append(db.Templates[:idx], db.Templates[idx+1:]...)
	a.logActivityBy(r, db, acc, "voucher", "Modèle de voucher «"+name+"» supprimé")
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ---------------------------------------------------------------------------
// F3 — Journal utilisateurs (login / logout / expire / kick)
// ---------------------------------------------------------------------------

// filterUserLogs — filtres partagés liste JSON + export CSV (sous verrou).
func filterUserLogs(db *model.DB, acc string, q url.Values) []model.UserLog {
	search := strings.ToLower(strings.TrimSpace(q.Get("search")))
	routerID := strings.TrimSpace(q.Get("routerId"))
	action := strings.TrimSpace(q.Get("action"))

	logs := []model.UserLog{}
	for _, l := range db.UserLogs {
		if l.AccountID != acc {
			continue
		}
		if action != "" && l.Action != action {
			continue
		}
		if routerID != "" && l.RouterID != routerID {
			continue
		}
		if search != "" {
			hay := strings.ToLower(l.Username + " " + l.IP)
			if !strings.Contains(hay, search) {
				continue
			}
		}
		logs = append(logs, l)
	}
	sort.Slice(logs, func(i, j int) bool { return logs[i].At > logs[j].At })
	return logs
}

func (a *API) handleUserLogsList(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	page := queryInt(r, "page", 1, 1, 1_000_000)
	pageSize := queryInt(r, "pageSize", 20, 1, 100)

	a.store.Lock()
	logs := filterUserLogs(a.store.Data(), acc, r.URL.Query())
	a.store.Unlock()

	total := len(logs)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data":     logs[start:end],
		"total":    total,
		"page":     page,
		"pageSize": pageSize,
	})
}

// handleUserLogsExport — CSV du journal utilisateurs (séparateur « ; », BOM).
func (a *API) handleUserLogsExport(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	a.store.Lock()
	logs := filterUserLogs(a.store.Data(), acc, r.URL.Query())
	a.store.Unlock()

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\"mikcloud-journal-utilisateurs.csv\"")
	// BOM UTF-8 : Excel reconnaît l'encodage.
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})
	_, _ = w.Write([]byte("Date;Utilisateur;Action;Routeur;IP;MAC\r\n"))
	for _, l := range logs {
		_, _ = w.Write([]byte(fmt.Sprintf("%s;%s;%s;%s;%s;%s\r\n",
			csvField(l.At), csvField(l.Username), csvField(l.Action),
			csvField(l.RouterName), csvField(l.IP), csvField(l.MAC))))
	}
}

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
	_, _ = w.Write([]byte("Utilisateur;Mot de passe;Profil;Statut;Routeur;Créé le;Expire le;" +
		"Data entrée (Mo);Data sortie (Mo);Prix;Revendeur;Commentaire\r\n"))
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
