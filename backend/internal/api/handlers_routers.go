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
		out["installScript"] = agent.InstallScript(agentBaseURL(r), agentToken, name, walledGardenDomains(r)...)
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
		// N°103 — capacité ligne DÉCLARÉE (bits/s, 0 = effacer la
		// déclaration). WanIface n'est PAS modifiable ici : c'est la
		// vérité routeur (read_state), jamais une saisie console.
		LineDownBps *int64 `json:"lineDownBps"`
		LineUpBps   *int64 `json:"lineUpBps"`
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
	// N°103 — capacité ligne déclarée : bornée 10 Gbps (une saisie au-delà
	// est une erreur d'unité — Mbps entrés en bps ou faute de frappe) ;
	// négatif refusé (0 est l'effacement, pas une capacité).
	if req.LineDownBps != nil {
		if *req.LineDownBps < 0 || *req.LineDownBps > 10_000_000_000 {
			writeErr(w, http.StatusBadRequest, "Capacité descendante invalide (0 à 10 Gbps en bits/s)")
			return
		}
		updated.LineDownBps = *req.LineDownBps
	}
	if req.LineUpBps != nil {
		if *req.LineUpBps < 0 || *req.LineUpBps > 10_000_000_000 {
			writeErr(w, http.StatusBadRequest, "Capacité montante invalide (0 à 10 Gbps en bits/s)")
			return
		}
		updated.LineUpBps = *req.LineUpBps
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
		out["installScript"] = agent.InstallScript(agentBaseURL(r), agentToken, updated.Name, walledGardenDomains(r)...)
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
	// N°101 — le registre des appareils suit le routeur (les noms affectés
	// ne survivent pas à la box qui les a découverts).
	devices := db.Devices[:0]
	for _, d := range db.Devices {
		if d.RouterID != id {
			devices = append(devices, d)
		}
	}
	db.Devices = devices
	// N°103 — les agrégats de qualité de ligne suivent le routeur (la
	// mesure FAI d'une box retirée n'a plus d'objet).
	lq := db.LineQuality[:0]
	for _, d := range db.LineQuality {
		if d.RouterID != id {
			lq = append(lq, d)
		}
	}
	db.LineQuality = lq
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
	// N°133 — marquage ciblé : le tick d'un simulateur marque routers
	// (+ traffic/line_quality pour ce routeur) — sauvegarde ciblée au
	// lieu du diff complet à chaque poll de trafic.
	touched := store.NewTableSet()
	store.Tick(db, now, touched)
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
	a.store.SaveTables(touched.Names()...)
	a.store.Unlock()
	writeJSON(w, http.StatusOK, out)
}

// ---------------------------------------------------------------------------
// N°103 — Qualité de ligne (mesure passive du débit FAI)
// ---------------------------------------------------------------------------

// lineQualityWindow — fenêtre d'observation de l'enveloppe mesurée : 14 jours
// (deux semaines complètes de fréquentation — semaine + week-end — avant de
// faire confiance à la mesure ; moins serait un carnet de commande faussé par
// un seul soir de match).
const lineQualityWindow = 14

// lineQualityMinDaySamples — échantillons minimum pour qu'un jour compte dans
// l'enveloppe : 50 fenêtres de ~2 min ≈ 1 h 40 d'observation — un jour quasi
// vide (redéploiement cloud, routeur en veille profonde) n'apporte rien.
const lineQualityMinDaySamples = 50

// lineQualityConfidentDays — jours QUALIFIÉS minimum pour estimer la mesure
// « exploitable » : 3 jours distincts d'observation sérieuse.
const lineQualityConfidentDays = 3

// handleRouterLineQuality — GET /api/routers/{id}/line-quality : la carte
// « Qualité de ligne » du site. Réponse :
//
//	wanIface      — interface WAN détectée (read_state ; ether1 en simulé) ;
//	configured    — capacité DÉCLARÉE par le gérant (0 = non renseignée) ;
//	days          — jusqu'à 14 jours d'agrégats de l'interface WAN (jour le
//	                plus récent d'abord) : max et p95 par direction ;
//	measured      — enveloppe OBSERVÉE sur la fenêtre : le débit le plus
//	                élevé jamais vu en fenêtre ~2 min (plancher honnête de la
//	                capacité — une ligne peu chargée n'est jamais « mesurée » à
//	                son étiquette) + p95 le plus élevé (enveloppe régulière,
//	                insensible aux pics isolés) + days/confident ;
//	live          — débit courant de l'interface WAN (db.Traffic, même
//	                fraîcheur que l'onglet Trafic).
//
// Simulé comme agent : le moteur d'échantillonnage est le même (AccumulateLineQuality),
// seul le producteur diffère (tickTraffic vs read_state). Mode real : 400 comme
// toutes les lectures télémétriques (le mode direct n'est pas supporté).
func (a *API) handleRouterLineQuality(w http.ResponseWriter, r *http.Request) {
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
	routerCopy := *rr
	// Le simulé vit au rythme des polls de la console (pattern handleRouterTraffic).
	if routerCopy.Mode == "simulated" {
		touched := store.NewTableSet()
		store.Tick(db, now, touched)
		// N°133 — le tick d'un routeur simulé marque routers (+ traffic/
		// line_quality pour ce simulateur) : sauvegarde ciblée, le reste
		// du parc n'est pas re-hashé pour cette carte qualité de ligne.
		a.store.SaveTables(touched.Names()...)
		if rr = findRouterScoped(db, id, acc); rr != nil {
			routerCopy = *rr
		}
	}

	type dayOut struct {
		Day      string `json:"day"`
		Samples  int    `json:"samples"`
		RxMaxBps int64  `json:"rxMaxBps"`
		TxMaxBps int64  `json:"txMaxBps"`
		RxP95Bps int64  `json:"rxP95Bps"`
		TxP95Bps int64  `json:"txP95Bps"`
	}
	days := []dayOut{}
	wan := routerCopy.WanIface
	// Agrégats de l'interface WAN, triés du plus récent au plus ancien, bornés
	// à la fenêtre d'observation. Le jour courant compte SANS seuil de samples
	// (c'est le jour en cours de se remplir) ; seuls les jours ÉCLOS comptent
	// dans l'enveloppe mesurée (samples ≥ 50).
	var today string
	if wan != "" {
		today = model.LineQualityDayKey(now)
		rows := []*model.LineQualityDay{}
		for i := range db.LineQuality {
			lq := &db.LineQuality[i]
			if lq.RouterID != id || lq.Iface != wan {
				continue
			}
			if lq.Day < now.AddDate(0, 0, -lineQualityWindow).Format("2006-01-02") {
				continue
			}
			rows = append(rows, lq)
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i].Day > rows[j].Day })
		for _, lq := range rows {
			days = append(days, dayOut{
				Day: lq.Day, Samples: lq.Samples,
				RxMaxBps: lq.RxMaxBps, TxMaxBps: lq.TxMaxBps,
				RxP95Bps: model.LineQualityHistPercentile(lq.RxHist, 95),
				TxP95Bps: model.LineQualityHistPercentile(lq.TxHist, 95),
			})
		}
	}
	// Enveloppe mesurée : jours éclos qualifiés uniquement — règle UNIQUE,
	// partagée avec la recommandation QoS (lineQualityEnvelope, N°104).
	measuredDown, measuredUp, qualifying := lineQualityEnvelope(db, id, wan, now)
	// p95 : enveloppe régulière (insensible aux pics isolés), mêmes jours.
	measuredP95Down, measuredP95Up := int64(0), int64(0)
	for _, d := range days {
		if d.Day == today || d.Samples < lineQualityMinDaySamples {
			continue
		}
		if d.RxP95Bps > measuredP95Down {
			measuredP95Down = d.RxP95Bps
		}
		if d.TxP95Bps > measuredP95Up {
			measuredP95Up = d.TxP95Bps
		}
	}
	// Débit courant de l'interface WAN (même source que l'onglet Trafic).
	live := map[string]any{"rxBps": int64(0), "txBps": int64(0), "at": ""}
	if wan != "" {
		for i := range db.Traffic {
			if db.Traffic[i].RouterID != id {
				continue
			}
			tr := &db.Traffic[i]
			for _, it := range tr.Interfaces {
				if it.Name == wan {
					live["rxBps"] = it.RxBps
					live["txBps"] = it.TxBps
					live["at"] = tr.UpdatedAt
				}
			}
			break
		}
	}
	a.store.Save() // le Tick du simulé a pu avancer les compteurs
	a.store.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"routerId": id,
		"wanIface": wan,
		"configured": map[string]int64{
			"downBps": routerCopy.LineDownBps,
			"upBps":   routerCopy.LineUpBps,
		},
		"days": days,
		"measured": map[string]any{
			"downBps":    measuredDown,
			"upBps":      measuredUp,
			"p95DownBps": measuredP95Down,
			"p95UpBps":   measuredP95Up,
			"days":       qualifying,
			"confident":  qualifying >= lineQualityConfidentDays,
		},
		"live": live,
	})
}
