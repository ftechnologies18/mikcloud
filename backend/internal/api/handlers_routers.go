// handlers_routers.go — gestion des routeurs (CRUD, test, stats).

package api

import (
	"encoding/json"
	"math/rand"
	"net/http"
	"sort"
	"strings"
	"time"

	"mikcloud/hotspot-api/internal/agent"
	"mikcloud/hotspot-api/internal/model"
	"mikcloud/hotspot-api/internal/routeros"
	"mikcloud/hotspot-api/internal/store"
)

func (a *API) handleRoutersList(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	a.store.Lock()
	db := a.store.Data()
	rs := []model.Router{}
	// « Utilisateurs » de la carte routeur = comptes NOMINATIFS (kind !=
	// voucher) — un ticket en stock ou vendu n'est pas un utilisateur.
	userCount := map[string]int{}
	for _, u := range db.HotspotUsers {
		if u.AccountID == acc && u.Kind != "voucher" {
			userCount[u.RouterID]++
		}
	}
	sessCount := map[string]int{}
	for _, s := range db.Sessions {
		if s.AccountID == acc {
			sessCount[s.RouterID]++
		}
	}
	for _, rr := range db.Routers {
		if rr.AccountID == acc {
			rs = append(rs, rr)
		}
	}
	a.store.Unlock()
	sort.Slice(rs, func(i, j int) bool { return rs[i].CreatedAt > rs[j].CreatedAt })
	out := make([]model.Router, 0, len(rs))
	for _, rr := range rs {
		rr.HotspotUsers = userCount[rr.ID]
		rr.ActiveSessions = sessCount[rr.ID]
		out = append(out, sanitizeRouter(rr))
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *API) handleRouterCreate(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	var req struct {
		Name            string `json:"name"`
		Host            string `json:"host"`
		Port            int    `json:"port"`
		Username        string `json:"username"`
		Password        string `json:"password"`
		Mode            string `json:"mode"`
		HotspotLoginUrl string `json:"hotspotLoginUrl"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	name := strings.TrimSpace(req.Name)
	host := strings.TrimSpace(req.Host)
	if name == "" {
		writeErr(w, http.StatusBadRequest, "Nom du routeur requis")
		return
	}
	hotspotLoginUrl, ok := normalizeHotspotLoginUrl(req.HotspotLoginUrl)
	if !ok {
		writeErr(w, http.StatusBadRequest, "URL de connexion hotspot invalide (http(s) attendu)")
		return
	}
	mode := req.Mode
	if mode == "" {
		mode = "simulated"
	}
	if mode != "simulated" && mode != "real" && mode != "agent" {
		writeErr(w, http.StatusBadRequest, "Mode invalide (simulated, real ou agent)")
		return
	}
	// P3 — compte expiré : écritures métier refusées (lecture seule).
	if !a.guardAccountWrite(w, r) {
		return
	}
	router := model.Router{
		ID: model.NewID("r-"), AccountID: acc, Name: name, Host: host, Mode: mode,
		Username: strings.TrimSpace(req.Username), Password: req.Password,
		HotspotLoginUrl: hotspotLoginUrl,
		Status:          "online", CreatedAt: model.NowISO(),
	}

	var agentToken string
	switch mode {
	case "agent":
		// Mode agent : aucune adresse IP requise (connexions 100 % sortantes,
		// compatibles CGNAT/Starlink). En ligne dès le premier check-in.
		tok, err := agent.NewToken()
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "Génération du token agent impossible")
			return
		}
		agentToken = tok
		router.AgentTokenHash = agent.HashToken(tok)
		router.TokenPreview = agent.Preview(tok)
		router.Status = "offline"
	case "real":
		port := req.Port
		if port == 0 {
			port = 8728
		}
		if port < 1 || port > 65535 {
			writeErr(w, http.StatusBadRequest, "Port invalide")
			return
		}
		if host == "" {
			writeErr(w, http.StatusBadRequest, "Adresse du routeur requise")
			return
		}
		router.Port = port
		username := strings.TrimSpace(req.Username)
		if username == "" {
			username = "admin"
		}
		router.Username = username
		res, err := routeros.TestRouter(host, port, username, req.Password)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "Connexion impossible : "+err.Error())
			return
		}
		router.Version = res.Version
	default: // simulated
		if host == "" {
			host = "simulé"
		}
		port := req.Port
		if port == 0 {
			port = 8728
		}
		username := strings.TrimSpace(req.Username)
		if username == "" {
			username = "admin"
		}
		router.Host = host
		router.Port = port
		router.Username = username
		versions := []string{"7.14.3", "7.15.2", "7.13.5", "7.12.1"}
		router.Version = versions[rand.Intn(len(versions))]
		router.UptimeSec = int64(3600 + rand.Intn(72*3600))
		router.CPULoad = 5 + rand.Intn(30)
	}
	a.store.Lock()
	for _, rr := range a.store.Data().Routers {
		if rr.AccountID == acc && strings.EqualFold(rr.Name, name) {
			a.store.Unlock()
			writeErr(w, http.StatusBadRequest, "Ce nom de routeur existe déjà")
			return
		}
	}
	// P3 — plafond de routeurs du plan Essentiel : la période couvre
	// RouterSlots routeurs (0 = non plafonné : bêta, illimité, plateforme).
	guardView := a.subscriptionGuardStateLocked(acc)
	if !guardAccountRouterLimit(w, guardView, accountRouterCount(a.store.Data(), acc)) {
		a.store.Unlock()
		return
	}
	a.store.Data().Routers = append(a.store.Data().Routers, router)
	msg := "Routeur " + router.Name + " ajouté"
	if mode == "agent" {
		msg += " (mode agent — en ligne au premier check-in)"
		// P1 (audit Mikhmon) — F6/F8 : le premier read_state est enfilé
		// dès la création : le premier check-in rapporte télémétrie,
		// carte/disque, interfaces et sessions. Chaque résultat en
		// enfile un suivant (handleAgentResult) → télémétrie continue.
		queueCommandLocked(a.store.Data(), acc, router.ID, model.CmdReadState, map[string]any{})
	}
	a.logActivityBy(r, a.store.Data(), acc, "router", msg)
	a.store.Save()
	a.store.Unlock()

	data, _ := json.Marshal(sanitizeRouter(router))
	var out map[string]any
	_ = json.Unmarshal(data, &out)
	if agentToken != "" {
		out["agentToken"] = agentToken
		out["installScript"] = agent.InstallScript(agentBaseURL(r), agentToken, name)
		out["message"] = "Copiez le script et collez-le dans Terminal (Winbox) — le routeur passera « En ligne » au premier check-in (≤ 45 s)."
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *API) handleRouterUpdate(w http.ResponseWriter, r *http.Request) {
	// P3 — compte expiré : écritures métier refusées (lecture seule).
	if !a.guardAccountWrite(w, r) {
		return
	}
	acc := accountScope(r)
	id := r.PathValue("id")
	var req struct {
		Name            *string `json:"name"`
		Host            *string `json:"host"`
		Port            *int    `json:"port"`
		Username        *string `json:"username"`
		Password        *string `json:"password"`
		Mode            *string `json:"mode"`
		HotspotLoginUrl *string `json:"hotspotLoginUrl"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	a.store.Lock()
	cur := findRouterScoped(a.store.Data(), id, acc)
	if cur == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Routeur introuvable")
		return
	}
	updated := *cur
	a.store.Unlock()

	if req.Name != nil {
		updated.Name = strings.TrimSpace(*req.Name)
	}
	if updated.Name == "" {
		writeErr(w, http.StatusBadRequest, "Nom du routeur requis")
		return
	}
	if req.HotspotLoginUrl != nil {
		hlu, ok := normalizeHotspotLoginUrl(*req.HotspotLoginUrl)
		if !ok {
			writeErr(w, http.StatusBadRequest, "URL de connexion hotspot invalide (http(s) attendu)")
			return
		}
		updated.HotspotLoginUrl = hlu
	}
	if req.Host != nil {
		updated.Host = strings.TrimSpace(*req.Host)
	}
	if req.Port != nil {
		updated.Port = *req.Port
	}
	if req.Username != nil {
		updated.Username = strings.TrimSpace(*req.Username)
	}
	if req.Password != nil && *req.Password != "" {
		updated.Password = *req.Password
	}
	prevMode := updated.Mode
	if req.Mode != nil {
		if *req.Mode != "simulated" && *req.Mode != "real" && *req.Mode != "agent" {
			writeErr(w, http.StatusBadRequest, "Mode invalide (simulated, real ou agent)")
			return
		}
		updated.Mode = *req.Mode
	}
	if updated.Port < 1 || updated.Port > 65535 {
		updated.Port = 8728
	}
	if updated.Mode != "agent" && updated.Host == "" {
		writeErr(w, http.StatusBadRequest, "Adresse du routeur requise")
		return
	}

	var agentToken string
	if updated.Mode == "agent" {
		if prevMode != "agent" || updated.AgentTokenHash == "" {
			tok, err := agent.NewToken()
			if err != nil {
				writeErr(w, http.StatusInternalServerError, "Génération du token agent impossible")
				return
			}
			agentToken = tok
			updated.AgentTokenHash = agent.HashToken(tok)
			updated.TokenPreview = agent.Preview(tok)
			updated.LastSeen = ""
			updated.Status = "offline"
		}
	} else {
		if prevMode == "agent" {
			updated.AgentTokenHash = ""
			updated.TokenPreview = ""
			updated.LastSeen = ""
		}
		// Test de connectivité (best effort) : met à jour le statut.
		if updated.Mode == "real" {
			if res, err := routeros.TestRouter(updated.Host, updated.Port, updated.Username, updated.Password); err != nil {
				updated.Status = "offline"
			} else {
				updated.Status = "online"
				if res.Version != "" {
					updated.Version = res.Version
				}
			}
		} else {
			updated.Status = "online"
		}
	}

	a.store.Lock()
	cur = findRouterScoped(a.store.Data(), id, acc)
	if cur == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Routeur introuvable")
		return
	}
	for _, rr := range a.store.Data().Routers {
		if rr.ID != id && rr.AccountID == acc && strings.EqualFold(rr.Name, updated.Name) {
			a.store.Unlock()
			writeErr(w, http.StatusBadRequest, "Ce nom de routeur existe déjà")
			return
		}
	}
	*cur = updated
	a.logActivityBy(r, a.store.Data(), acc, "router", "Routeur "+updated.Name+" modifié")
	a.store.Save()
	a.store.Unlock()
	a.invalidateGateway(id)

	data, _ := json.Marshal(sanitizeRouter(updated))
	var out map[string]any
	_ = json.Unmarshal(data, &out)
	if agentToken != "" {
		out["agentToken"] = agentToken
		out["installScript"] = agent.InstallScript(agentBaseURL(r), agentToken, updated.Name)
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *API) handleRouterDelete(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	id := r.PathValue("id")
	a.store.Lock()
	db := a.store.Data()
	cur := findRouterScoped(db, id, acc)
	if cur == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Routeur introuvable")
		return
	}
	name := cur.Name
	routers := db.Routers[:0]
	for _, rr := range db.Routers {
		if rr.ID != id {
			routers = append(routers, rr)
		}
	}
	db.Routers = routers
	users := db.HotspotUsers[:0]
	for _, u := range db.HotspotUsers {
		if u.RouterID != id {
			users = append(users, u)
		}
	}
	db.HotspotUsers = users
	sessions := db.Sessions[:0]
	for _, s := range db.Sessions {
		if s.RouterID != id {
			sessions = append(sessions, s)
		}
	}
	db.Sessions = sessions
	commands := db.Commands[:0]
	for _, c := range db.Commands {
		if c.RouterID != id {
			commands = append(commands, c)
		}
	}
	db.Commands = commands
	a.logActivityBy(r, db, acc, "router", "Routeur "+name+" supprimé")
	a.store.Save()
	a.store.Unlock()
	a.invalidateGateway(id)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *API) handleRouterTest(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	id := r.PathValue("id")
	a.store.Lock()
	cur := findRouterScoped(a.store.Data(), id, acc)
	if cur == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Routeur introuvable")
		return
	}
	routerCopy := *cur
	a.store.Unlock()

	if routerCopy.Mode == "agent" {
		online := false
		last := "jamais"
		if t, err := time.Parse(time.RFC3339, routerCopy.LastSeen); err == nil {
			online = time.Since(t) < OnlineWindow
			last = routerCopy.LastSeen
		}
		msg := "Agent connecté (dernier check-in : " + last + ")"
		if !online {
			msg = "Agent injoignable : aucun check-in récent. Recollez le script d'installation si le scheduler a été supprimé."
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok": online, "message": msg, "latencyMs": 0, "version": routerCopy.Version,
		})
		return
	}

	gw := a.gatewayFor(routerCopy)
	res, err := gw.TestConnection()
	if err != nil {
		a.store.Lock()
		if rr := findRouterScoped(a.store.Data(), id, acc); rr != nil {
			rr.Status = "offline"
		}
		a.store.Save()
		a.store.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{
			"ok": false, "message": "Connexion échouée : " + err.Error(), "latencyMs": 0, "version": "",
		})
		return
	}
	a.store.Lock()
	if rr := findRouterScoped(a.store.Data(), id, acc); rr != nil {
		rr.Status = "online"
		if routerCopy.Mode == "real" && res.Version != "" {
			rr.Version = res.Version
		}
	}
	a.store.Save()
	a.store.Unlock()
	msg := "Connexion réussie"
	if routerCopy.Mode == "simulated" {
		msg = "Connexion réussie (routeur simulé)"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "message": msg, "latencyMs": res.LatencyMs, "version": res.Version,
	})
}

func (a *API) handleRouterStats(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	id := r.PathValue("id")
	a.store.Lock()
	cur := findRouterScoped(a.store.Data(), id, acc)
	if cur == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Routeur introuvable")
		return
	}
	routerCopy := *cur
	a.store.Unlock()

	if routerCopy.Mode == "agent" {
		// Mode agent : télémétrie en cache, mise à jour par read_state.
		a.store.Lock()
		active := 0
		for _, s := range a.store.Data().Sessions {
			if s.RouterID == id && s.AccountID == acc {
				active++
			}
		}
		a.store.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{
			"cpuLoad": routerCopy.CPULoad, "memUsedPct": 0,
			"freeMemoryMb": 0, "totalMemoryMb": 0,
			"uptimeSec": routerCopy.UptimeSec, "version": routerCopy.Version,
			"activeSessions": active,
		})
		return
	}

	gw := a.gatewayFor(routerCopy)
	stats, err := gw.SystemStats()
	if err != nil {
		writeErr(w, http.StatusBadRequest, "Statistiques indisponibles : "+err.Error())
		return
	}
	activeSessions := 0
	if routerCopy.Mode == "real" {
		if live, lerr := gw.ListSessions(); lerr == nil {
			activeSessions = len(live)
		}
	} else {
		a.store.Lock()
		for _, s := range a.store.Data().Sessions {
			if s.RouterID == id && s.AccountID == acc {
				activeSessions++
			}
		}
		a.store.Save()
		a.store.Unlock()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"cpuLoad":        stats.CPULoad,
		"memUsedPct":     stats.MemUsedPct,
		"freeMemoryMb":   stats.FreeMemoryMb,
		"totalMemoryMb":  stats.TotalMemoryMb,
		"uptimeSec":      stats.UptimeSec,
		"version":        stats.Version,
		"activeSessions": activeSessions,
	})
}

// ---------------------------------------------------------------------------
// F6 — Trafic temps réel
// ---------------------------------------------------------------------------

// handleRouterTraffic — GET /api/routers/{id}/traffic → RouterTraffic.
// store.Tick est appelé avant lecture : la simulation vit au rythme des polls
// du front (5 s) même si personne d'autre ne consulte les sessions/dashboard.
// Routeur agent sans mesure : enveloppe vide {routerId, interfaces: [], history: []}.
func (a *API) handleRouterTraffic(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	id := r.PathValue("id")
	now := time.Now().UTC()

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
	store.Tick(db, now)
	out := model.RouterTraffic{
		RouterID: id, AccountID: acc,
		Interfaces: []model.IfaceTraffic{}, History: []model.TrafficPoint{},
	}
	for i := range db.Traffic {
		if db.Traffic[i].RouterID != id {
			continue
		}
		// Copie défensive (le rendu JSON se fait hors verrou).
		out = db.Traffic[i]
		out.Interfaces = append([]model.IfaceTraffic(nil), db.Traffic[i].Interfaces...)
		out.History = append([]model.TrafficPoint(nil), db.Traffic[i].History...)
		if out.Interfaces == nil {
			out.Interfaces = []model.IfaceTraffic{}
		}
		if out.History == nil {
			out.History = []model.TrafficPoint{}
		}
		break
	}
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, out)
}
