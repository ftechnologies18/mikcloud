// Package api — scheduler + alimentation routeur (F10, audit Mikhmon) :
// CRUD des tâches (commandes agent scheduler_add/set/remove, lecture
// read_scheduler mise en cache comme les outils F9) et reboot/shutdown
// (cloud direct en simulé, commande agent sinon) — CONTRACT-V2 §0.
package api

import (
	"mikcloud/hotspot-api/internal/model"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// Validation du nom et de l'intervalle d'une tâche scheduler (contrat F10).
var (
	schedulerNamePat  = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
	schedulerInterval = regexp.MustCompile(`^[0-9]+[smhdw]$`)
)

// schedulerRow — une tâche scheduler rapportée par read_scheduler (contrat F10).
type schedulerRow struct {
	Name     string `json:"name"`
	Interval string `json:"interval"`
	OnEvent  string `json:"onEvent"`
	Disabled bool   `json:"disabled"`
}

func parseSchedulerRows(rows [][]string) []schedulerRow {
	out := make([]schedulerRow, 0, len(rows))
	for _, e := range rows {
		out = append(out, schedulerRow{
			Name: field(e, 0), Interval: field(e, 1),
			Disabled: field(e, 2) == "true", OnEvent: field(e, 3),
		})
	}
	return out
}

// ---------------------------------------------------------------------------
// F10 — Scheduler + reboot/shutdown
// ---------------------------------------------------------------------------

// handleSchedulerGet — GET /api/routers/{id}/scheduler → {queued, data, updatedAt}
// (réponse UNIFIÉE) : simulated → SchedulerTask[] du cloud ; agent → mécanique
// F9 avec read_scheduler (data = [{name, interval, onEvent, disabled}]).
func (a *API) handleSchedulerGet(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	id := r.PathValue("id")

	a.store.Lock()
	db := a.store.Data()
	rr := findRouterScoped(db, id, acc)
	if rr == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Routeur introuvable")
		return
	}
	if rr.Mode == "real" {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, realModeUnsupported)
		return
	}
	if rr.Mode == "simulated" {
		tasks := []model.SchedulerTask{}
		for _, t := range db.SchedulerTasks {
			if t.AccountID == acc && t.RouterID == id {
				tasks = append(tasks, t)
			}
		}
		a.store.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{"queued": false, "data": tasks, "updatedAt": model.NowISO()})
		return
	}
	// mode agent — cache puis file.
	now := time.Now().UTC()
	if fresh := freshToolCommand(db, id, model.CmdReadScheduler, now); fresh != nil {
		raw, _ := fresh.Result["data"].(string)
		rows := parseSchedulerRows(splitAgentList(raw))
		doneAt := fresh.DoneAt
		a.store.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{"queued": false, "data": rows, "updatedAt": doneAt})
		return
	}
	if !pendingToolCommand(db, id, model.CmdReadScheduler) {
		queueCommandLocked(db, acc, id, model.CmdReadScheduler, map[string]any{})
		a.store.Save()
	}
	a.store.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"queued": true, "data": []schedulerRow{}, "updatedAt": ""})
}

// handleSchedulerCreate — POST /api/routers/{id}/scheduler {name, interval, onEvent} :
// simulated → SchedulerTask (201) ; agent → commande scheduler_add → {queued:true}.
func (a *API) handleSchedulerCreate(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	id := r.PathValue("id")
	var req struct {
		Name     string `json:"name"`
		Interval string `json:"interval"`
		OnEvent  string `json:"onEvent"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" || len(name) > 48 || !schedulerNamePat.MatchString(name) {
		writeErr(w, http.StatusBadRequest, "Nom de tâche invalide (48 caractères max, sans espaces : A-Za-z0-9._-)")
		return
	}
	interval := strings.TrimSpace(req.Interval)
	if !schedulerInterval.MatchString(interval) {
		writeErr(w, http.StatusBadRequest, "Intervalle invalide (format RouterOS : 30s, 5m, 1h, 2d, 1w)")
		return
	}
	onEvent := strings.TrimSpace(req.OnEvent)
	if onEvent == "" || len(onEvent) > 4096 {
		writeErr(w, http.StatusBadRequest, "On-event requis (4096 caractères max)")
		return
	}

	a.store.Lock()
	db := a.store.Data()
	rr := findRouterScoped(db, id, acc)
	if rr == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Routeur introuvable")
		return
	}
	if rr.Mode == "real" {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, realModeUnsupported)
		return
	}
	if rr.Mode == "agent" {
		cmd := queueCommandLocked(db, acc, id, model.CmdSchedulerAdd, map[string]any{
			"name": name, "interval": interval, "onEvent": onEvent,
		})
		a.store.Save()
		cmdID := cmd.ID
		a.store.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{
			"queued": true, "commandId": cmdID,
			"message": "Commande envoyée — création au prochain check-in du routeur (≤ 45 s)",
		})
		return
	}
	// simulated — source cloud (un nom unique par routeur, comme RouterOS).
	for i := range db.SchedulerTasks {
		if db.SchedulerTasks[i].AccountID == acc && db.SchedulerTasks[i].RouterID == id && db.SchedulerTasks[i].Name == name {
			a.store.Unlock()
			writeErr(w, http.StatusBadRequest, "Une tâche portant ce nom existe déjà sur ce routeur")
			return
		}
	}
	task := model.SchedulerTask{
		ID: model.NewID("sch-"), AccountID: acc, RouterID: id,
		Name: name, Interval: interval, OnEvent: onEvent,
		Disabled: false, CreatedAt: model.NowISO(),
	}
	db.SchedulerTasks = append(db.SchedulerTasks, task)
	a.logActivityBy(r, db, acc, "router", "Tâche planifiée «"+name+"» ("+interval+") créée sur «"+rr.Name+"»")
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusCreated, task)
}

// handleSchedulerToggle — POST /api/routers/{id}/scheduler-toggle {name, disabled} :
// simulated → update cloud by name ; agent → commande scheduler_set.
func (a *API) handleSchedulerToggle(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	id := r.PathValue("id")
	var req struct {
		Name     string `json:"name"`
		Disabled bool   `json:"disabled"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" || len(name) > 48 {
		writeErr(w, http.StatusBadRequest, "Nom de tâche requis (48 caractères max)")
		return
	}

	a.store.Lock()
	db := a.store.Data()
	rr := findRouterScoped(db, id, acc)
	if rr == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Routeur introuvable")
		return
	}
	if rr.Mode == "real" {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, realModeUnsupported)
		return
	}
	if rr.Mode == "agent" {
		queueCommandLocked(db, acc, id, model.CmdSchedulerSet, map[string]any{
			"name": name, "disabled": req.Disabled,
		})
		a.store.Save()
		a.store.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{
			"queued":  true,
			"message": "Commande envoyée — appliquée au prochain check-in du routeur (≤ 45 s)",
		})
		return
	}
	var task *model.SchedulerTask
	for i := range db.SchedulerTasks {
		if db.SchedulerTasks[i].AccountID == acc && db.SchedulerTasks[i].RouterID == id && db.SchedulerTasks[i].Name == name {
			task = &db.SchedulerTasks[i]
			break
		}
	}
	if task == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Tâche introuvable")
		return
	}
	task.Disabled = req.Disabled
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleSchedulerRemove — POST /api/routers/{id}/scheduler-remove {name} :
// simulated → delete cloud by name ; agent → commande scheduler_remove.
func (a *API) handleSchedulerRemove(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	id := r.PathValue("id")
	var req struct {
		Name string `json:"name"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" || len(name) > 48 {
		writeErr(w, http.StatusBadRequest, "Nom de tâche requis (48 caractères max)")
		return
	}

	a.store.Lock()
	db := a.store.Data()
	rr := findRouterScoped(db, id, acc)
	if rr == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Routeur introuvable")
		return
	}
	if rr.Mode == "real" {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, realModeUnsupported)
		return
	}
	if rr.Mode == "agent" {
		queueCommandLocked(db, acc, id, model.CmdSchedulerRemove, map[string]any{"name": name})
		a.store.Save()
		a.store.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{
			"queued":  true,
			"message": "Commande envoyée — suppression au prochain check-in du routeur (≤ 45 s)",
		})
		return
	}
	idx := -1
	for i := range db.SchedulerTasks {
		if db.SchedulerTasks[i].AccountID == acc && db.SchedulerTasks[i].RouterID == id && db.SchedulerTasks[i].Name == name {
			idx = i
			break
		}
	}
	if idx < 0 {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Tâche introuvable")
		return
	}
	db.SchedulerTasks = append(db.SchedulerTasks[:idx], db.SchedulerTasks[idx+1:]...)
	a.logActivityBy(r, db, acc, "router", "Tâche planifiée «"+name+"» supprimée de «"+rr.Name+"»")
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleRouterReboot — POST /api/routers/{id}/reboot :
//   - simulated → uptimeSec=0 + sessions du routeur purgées (UserLog logout
//     pour chacune, cohérent F3) + Activity → {ok:true} ;
//   - agent     → commande reboot → {queued:true, message} ;
//   - real      → 400.
func (a *API) handleRouterReboot(w http.ResponseWriter, r *http.Request) {
	a.handleRouterPower(w, r, "reboot")
}

// handleRouterShutdown — POST /api/routers/{id}/shutdown : idem reboot.
func (a *API) handleRouterShutdown(w http.ResponseWriter, r *http.Request) {
	a.handleRouterPower(w, r, "shutdown")
}

func (a *API) handleRouterPower(w http.ResponseWriter, r *http.Request, action string) {
	acc := accountScope(r)
	id := r.PathValue("id")

	a.store.Lock()
	db := a.store.Data()
	rr := findRouterScoped(db, id, acc)
	if rr == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Routeur introuvable")
		return
	}
	if rr.Mode == "real" {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, realModeUnsupported)
		return
	}
	if rr.Mode == "agent" {
		kind := model.CmdReboot
		if action == "shutdown" {
			kind = model.CmdShutdown
		}
		cmd := queueCommandLocked(db, acc, id, kind, nil)
		a.store.Save()
		cmdID := cmd.ID
		a.store.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{
			"queued": true, "commandId": cmdID,
			"message": "Commande envoyée — " + action + " au prochain check-in du routeur (≤ 45 s)",
		})
		return
	}

	// simulated — redémarrage immédiat : uptime remis à zéro, sessions
	// coupées (journalisées logout), trafic conservé (compteurs matériels).
	rr.UptimeSec = 0
	rr.ActiveSessions = 0
	now := time.Now().UTC()
	kept := db.Sessions[:0]
	for _, s := range db.Sessions {
		if s.RouterID == id {
			logRouterUserEvent(db, rr, s, "logout", now)
			continue
		}
		kept = append(kept, s)
	}
	db.Sessions = kept
	verb := "redémarré"
	if action == "shutdown" {
		verb = "éteint"
	}
	a.logActivityBy(r, db, acc, "router", "Routeur "+rr.Name+" "+verb+" (commande console)")
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
