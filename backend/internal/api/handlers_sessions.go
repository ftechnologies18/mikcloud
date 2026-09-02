// handlers_sessions.go — sessions actives (liste, kick).

package api

import (
	"net/http"
	"sort"
	"time"

	"mikcloud/hotspot-api/internal/agent"
	"mikcloud/hotspot-api/internal/model"
	"mikcloud/hotspot-api/internal/store"
)

func (a *API) handleSessionsList(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	now := time.Now().UTC()
	a.store.Lock()
	db := a.store.Data()
	store.Tick(db, now) // fait vivre la simulation (tous comptes)
	sessions := []model.Session{}
	for _, s := range db.Sessions {
		if s.AccountID == acc {
			sessions = append(sessions, s)
		}
	}
	realRouters := []model.Router{}
	for _, rr := range db.Routers {
		if rr.Mode == "real" && rr.AccountID == acc {
			realRouters = append(realRouters, rr)
		}
	}
	a.store.Save()
	a.store.Unlock()

	// Fusion des sessions live des routeurs réels
	for _, rr := range realRouters {
		gw := a.gatewayFor(rr)
		if live, err := gw.ListSessions(); err == nil {
			sessions = append(sessions, live...)
		}
	}
	sort.Slice(sessions, func(i, j int) bool { return sessions[i].StartedAt > sessions[j].StartedAt })
	writeJSON(w, http.StatusOK, sessions)
}

func (a *API) handleSessionKick(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	id := r.PathValue("id")
	a.store.Lock()
	db := a.store.Data()
	idx := -1
	for i := range db.Sessions {
		if db.Sessions[i].ID == id && db.Sessions[i].AccountID == acc {
			idx = i
			break
		}
	}
	username := ""
	var routerCopy *model.Router
	if idx >= 0 {
		username = db.Sessions[idx].Username
		if rr := findRouterScoped(db, db.Sessions[idx].RouterID, acc); rr != nil {
			c := *rr
			routerCopy = &c
		}
	}
	realRouters := []model.Router{}
	for _, rr := range db.Routers {
		if rr.Mode == "real" && rr.AccountID == acc {
			realRouters = append(realRouters, rr)
		}
	}
	a.store.Unlock()

	if routerCopy != nil && routerCopy.Mode == "agent" {
		// Mode agent : retrait local de la session + commande kick en file.
		if idx >= 0 {
			a.store.Lock()
			db = a.store.Data()
			sessions := db.Sessions[:0]
			for _, s := range db.Sessions {
				if s.ID != id {
					sessions = append(sessions, s)
				}
			}
			db.Sessions = sessions
			queueCommandLocked(db, routerCopy.AccountID, routerCopy.ID, model.CmdKick, map[string]any{"user": agent.SanitizeName(username)})
			a.logActivityBy(r, db, acc, "session", "Session de "+username+" fermée (kick, en attente du routeur)")
			a.store.Save()
			a.store.Unlock()
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	if routerCopy != nil {
		gw := a.gatewayFor(*routerCopy)
		if err := gw.KickSession(id); err != nil {
			writeErr(w, http.StatusBadRequest, "Impossible de fermer la session : "+err.Error())
			return
		}
	} else if idx >= 0 {
		// routeur disparu : suppression directe
		a.store.Lock()
		db = a.store.Data()
		sessions := db.Sessions[:0]
		for _, s := range db.Sessions {
			if s.ID != id {
				sessions = append(sessions, s)
			}
		}
		db.Sessions = sessions
		a.store.Unlock()
	} else {
		// peut-être une session live d'un routeur réel
		kicked := false
		for _, rr := range realRouters {
			gw := a.gatewayFor(rr)
			if err := gw.KickSession(id); err == nil {
				kicked = true
				break
			}
		}
		if !kicked {
			writeErr(w, http.StatusNotFound, "Session introuvable")
			return
		}
	}

	a.store.Lock()
	msg := "Session fermée (kick)"
	if username != "" {
		msg = "Session de " + username + " fermée (kick)"
	}
	a.logActivityBy(r, a.store.Data(), acc, "session", msg)
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
