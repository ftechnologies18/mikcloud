// Package api — endpoints de l'agent MikCloud (routeur -> cloud, HTTP-poll sortant).
//
// Contrat :
//
//	POST /agent/register?token=…  (identity, model, version, uptime)  → inscription
//	GET  /agent/cmd?token=…                                            → script .rsc ou "# mikcloud nop"
//	POST /agent/result?token=…    (cmd, status, …)                     → rapport d'exécution
//
// Côté console (auth JWT) :
//
//	GET  /api/routers/{id}/provision     → état du provisionning (token stocké haché)
//	POST /api/routers/{id}/rotate-token  → nouveau token + script complet
//	POST /api/routers/{id}/refresh       → file une commande read_state
package api

import (
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"mikcloud/hotspot-api/internal/agent"
	"mikcloud/hotspot-api/internal/model"
)

// OnlineWindow — fenêtre pendant laquelle un routeur agent est considéré en ligne.
const OnlineWindow = 3 * time.Minute

// registerAgentRoutes — routes publiques (token d'agent) + routes console.
// Les chemins /agent/* sont hors préfixe /api/ : le middleware JWT ne les touche pas.
func (a *API) registerAgentRoutes(mux *http.ServeMux) {
	// Agent (auth par token de routeur)
	mux.HandleFunc("POST /agent/register", a.handleAgentRegister)
	mux.HandleFunc("GET /agent/cmd", a.handleAgentCmd)
	mux.HandleFunc("POST /agent/result", a.handleAgentResult)

	// Console (auth JWT via middleware /api/)
	mux.HandleFunc("GET /api/routers/{id}/provision", a.handleRouterProvision)
	mux.HandleFunc("POST /api/routers/{id}/rotate-token", a.handleRouterRotateToken)
	mux.HandleFunc("POST /api/routers/{id}/refresh", a.handleRouterRefresh)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// agentBaseURL — URL publique du backend pour les scripts :
// env MIKCLOUD_BASE_URL > X-Forwarded-Proto + Host > https://Host (http si local).
func agentBaseURL(r *http.Request) string {
	if b := strings.TrimSpace(os.Getenv("MIKCLOUD_BASE_URL")); b != "" {
		return strings.TrimRight(b, "/")
	}
	scheme := "https"
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	} else if r.TLS == nil && (strings.HasPrefix(r.Host, "localhost") || strings.HasPrefix(r.Host, "127.")) {
		scheme = "http"
	}
	return scheme + "://" + r.Host
}

// parseAgentForm — parse tolérant : query URL + corps brut (RouterOS n'envoie pas
// toujours un Content-Type form-urlencoded).
func parseAgentForm(r *http.Request) url.Values {
	vals := url.Values{}
	for k, vs := range r.URL.Query() {
		for _, v := range vs {
			vals.Add(k, v)
		}
	}
	body := make([]byte, 0, 1<<16)
	buf := make([]byte, 4096)
	for {
		n, err := r.Body.Read(buf)
		body = append(body, buf[:n]...)
		if err != nil || len(body) > 1<<20 {
			break
		}
	}
	if len(body) > 0 {
		if bv, err := url.ParseQuery(string(body)); err == nil {
			for k, vs := range bv {
				for _, v := range vs {
					vals.Add(k, v)
				}
			}
		}
	}
	return vals
}

// routerByToken — retrouve le routeur agent par token (haché). Sous verrou.
func routerByToken(db *model.DB, token string) *model.Router {
	h := agent.HashToken(token)
	for i := range db.Routers {
		if db.Routers[i].Mode == "agent" && db.Routers[i].AgentTokenHash == h {
			return &db.Routers[i]
		}
	}
	return nil
}

// touchAgent — marque le routeur en ligne (sous verrou).
func touchAgent(r *model.Router) {
	r.Status = "online"
	r.LastSeen = model.NowISO()
}

// queueCommandLocked — dépose une commande en file (sous verrou ; Save à charge
// de l'appelant). Déduplique les read_state déjà en attente. La commande porte
// l'identifiant du compte du routeur (isolation multi-tenant).
func queueCommandLocked(db *model.DB, acc, routerID, kind string, payload map[string]any) *model.Command {
	if kind == model.CmdReadState {
		for i := range db.Commands {
			if db.Commands[i].RouterID == routerID && db.Commands[i].Kind == model.CmdReadState && db.Commands[i].Status == "queued" {
				return &db.Commands[i]
			}
		}
	}
	cmd := model.Command{
		ID:        model.NewID("c-"),
		RouterID:  routerID,
		AccountID: acc,
		Kind:      kind,
		Payload:   payload,
		Status:    "queued",
		CreatedAt: model.NowISO(),
	}
	db.Commands = append(db.Commands, cmd)
	return &db.Commands[len(db.Commands)-1]
}

// profileRef — construit la référence compacte d'un profil pour les payloads.
func profileRef(p model.Profile) map[string]any {
	return map[string]any{
		"name":              agent.SanitizeName(p.Name),
		"rateLimit":         p.RateLimit,
		"sessionTimeoutMin": p.SessionTimeoutMin,
		"sharedUsers":       p.SharedUsers,
	}
}

// purgeOldCommands — supprime les commandes terminées de plus de 7 jours (sous verrou).
func purgeOldCommands(db *model.DB) {
	lim := time.Now().Add(-7 * 24 * time.Hour).Format(time.RFC3339)
	kept := db.Commands[:0]
	for _, c := range db.Commands {
		if (c.Status == "done" || c.Status == "error") && c.DoneAt != "" && c.DoneAt < lim {
			continue
		}
		kept = append(kept, c)
	}
	db.Commands = kept
}

// parseRosUptime — parse une durée RouterOS ("3w1d02:15:30", "02:15:30", "2h30m", "45s").
func parseRosUptime(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	if i := strings.LastIndex(s, ":"); i >= 0 {
		seg := s
		if j := strings.LastIndexAny(s[:i], "wd"); j >= 0 {
			seg = s[j+1:]
		}
		parts := strings.Split(seg, ":")
		vals := make([]int, 0, 3)
		ok := true
		for _, p := range parts {
			n, err := strconv.Atoi(strings.TrimSpace(p))
			if err != nil {
				ok = false
				break
			}
			vals = append(vals, n)
		}
		if ok && len(vals) >= 2 {
			var hms int64
			if len(vals) == 3 {
				hms = int64(vals[0])*3600 + int64(vals[1])*60 + int64(vals[2])
			} else {
				hms = int64(vals[0])*60 + int64(vals[1])
			}
			prefix := s
			if k := strings.Index(s, seg); k > 0 {
				prefix = s[:k]
			}
			return hms + parseRosUptimeSuffix(prefix)
		}
	}
	return parseRosUptimeSuffix(s)
}

// parseRosUptimeSuffix — suffixes w/d/h/m/s ("3w1d", "2h30m", "45s").
func parseRosUptimeSuffix(s string) int64 {
	var total int64
	num := 0
	has := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= '0' && c <= '9' {
			num = num*10 + int(c-'0')
			has = true
			continue
		}
		if !has {
			continue
		}
		switch c {
		case 'w':
			total += int64(num) * 7 * 86400
		case 'd':
			total += int64(num) * 86400
		case 'h':
			total += int64(num) * 3600
		case 'm':
			total += int64(num) * 60
		case 's':
			total += int64(num)
		}
		num = 0
		has = false
	}
	return total
}

// ---------------------------------------------------------------------------
// POST /agent/register
// ---------------------------------------------------------------------------

func (a *API) handleAgentRegister(w http.ResponseWriter, r *http.Request) {
	vals := parseAgentForm(r)
	token := vals.Get("token")
	if token == "" {
		writeErr(w, http.StatusUnauthorized, "Token agent manquant")
		return
	}
	a.store.Lock()
	db := a.store.Data()
	router := routerByToken(db, token)
	if router == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Token agent inconnu")
		return
	}
	touchAgent(router)
	if v := strings.TrimSpace(vals.Get("version")); v != "" {
		router.Version = v
	}
	if ident := strings.TrimSpace(vals.Get("identity")); ident != "" && router.Host == "" {
		router.Host = ident
	}
	a.logActivity(db, router.AccountID, "router", "Routeur «"+router.Name+"» connecté à MikCloud (agent inscrit)")
	a.store.Save()
	name := router.Name
	a.store.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "router": name, "intervalSec": 45, "script": agent.ScriptFilename,
	})
}

// ---------------------------------------------------------------------------
// GET /agent/cmd — défilement de la file et génération du script
// ---------------------------------------------------------------------------

func (a *API) handleAgentCmd(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if token == "" {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("# mikcloud: token manquant\n"))
		return
	}
	base := agentBaseURL(r)

	a.store.Lock()
	db := a.store.Data()
	router := routerByToken(db, token)
	if router == nil {
		a.store.Unlock()
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("# mikcloud: token inconnu\n"))
		return
	}
	touchAgent(router)

	// File FIFO : commandes en attente (max 10 par check-in)
	queued := []model.Command{}
	for i := range db.Commands {
		if db.Commands[i].RouterID == router.ID && db.Commands[i].Status == "queued" {
			queued = append(queued, db.Commands[i])
			if len(queued) >= 10 {
				break
			}
		}
	}
	sort.Slice(queued, func(i, j int) bool { return queued[i].CreatedAt < queued[j].CreatedAt })

	b := agent.Builder{BaseURL: base, Token: token}
	var chunks []string
	sentIDs := []string{}
	for _, cmd := range queued {
		script, err := b.ScriptFor(cmd)
		if err != nil {
			for i := range db.Commands {
				if db.Commands[i].ID == cmd.ID {
					db.Commands[i].Status = "error"
					db.Commands[i].Result = map[string]any{"message": err.Error()}
					db.Commands[i].DoneAt = model.NowISO()
				}
			}
			continue
		}
		chunks = append(chunks, script)
		sentIDs = append(sentIDs, cmd.ID)
	}
	nowISO := model.NowISO()
	for _, id := range sentIDs {
		for i := range db.Commands {
			if db.Commands[i].ID == id {
				db.Commands[i].Status = "sent"
				db.Commands[i].SentAt = nowISO
			}
		}
	}
	purgeOldCommands(db)
	a.store.Save()
	a.store.Unlock()

	w.WriteHeader(http.StatusOK)
	if len(chunks) == 0 {
		_, _ = w.Write([]byte(agent.NopScript()))
		return
	}
	_, _ = w.Write([]byte(strings.Join(chunks, "\n")))
}

// ---------------------------------------------------------------------------
// POST /agent/result — application des résultats
// ---------------------------------------------------------------------------

func (a *API) handleAgentResult(w http.ResponseWriter, r *http.Request) {
	vals := parseAgentForm(r)
	token := vals.Get("token")
	cmdID := vals.Get("cmd")
	status := vals.Get("status")
	if token == "" || cmdID == "" {
		writeErr(w, http.StatusBadRequest, "Paramètres manquants (token, cmd)")
		return
	}
	ok := status == "ok"

	a.store.Lock()
	db := a.store.Data()
	router := routerByToken(db, token)
	if router == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Token agent inconnu")
		return
	}
	touchAgent(router)

	idx := -1
	for i := range db.Commands {
		if db.Commands[i].ID == cmdID && db.Commands[i].RouterID == router.ID {
			idx = i
			break
		}
	}
	if idx < 0 {
		a.store.Unlock()
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true, "unknown": true})
		return
	}
	cmd := &db.Commands[idx]
	cmd.Status = map[bool]string{true: "done", false: "error"}[ok]
	cmd.DoneAt = model.NowISO()
	res := map[string]any{}
	for k, vs := range vals {
		if len(vs) > 0 && k != "token" {
			res[k] = vs[0]
		}
	}
	cmd.Result = res

	switch {
	case cmd.Kind == model.CmdReadState && ok:
		a.applyReadState(db, router, vals)
		a.logActivity(db, router.AccountID, "router", "Routeur «"+router.Name+"» synchronisé ("+
			strconv.Itoa(router.ActiveSessions)+" session(s) active(s), "+strconv.Itoa(router.HotspotUsers)+" utilisateur(s))")
	case ok:
		a.logActivity(db, router.AccountID, "router", "Commande "+cmd.Kind+" exécutée sur «"+router.Name+"»")
		queueCommandLocked(db, router.AccountID, router.ID, model.CmdReadState, map[string]any{})
	default:
		a.logActivity(db, router.AccountID, "router", "Commande "+cmd.Kind+" ÉCHOUÉE sur «"+router.Name+"» ("+vals.Get("message")+")")
	}
	purgeOldCommands(db)
	a.store.Save()
	a.store.Unlock()

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// applyReadState — applique la télémétrie + sync users/sessions d'un routeur agent.
func (a *API) applyReadState(db *model.DB, router *model.Router, vals url.Values) {
	if v := strings.TrimSpace(vals.Get("version")); v != "" {
		if len(v) > 32 {
			v = v[:32]
		}
		router.Version = v
	}
	if up := parseRosUptime(vals.Get("uptime")); up > 0 {
		router.UptimeSec = up
	}
	if cpu, err := strconv.Atoi(vals.Get("cpu")); err == nil && cpu >= 0 && cpu <= 100 {
		router.CPULoad = cpu
	}

	// Users : "name|profile|disabled;…"
	userEntries := splitAgentList(vals.Get("users"))
	router.HotspotUsers = len(userEntries)

	// Sync des utilisateurs créés hors MikCloud (inconnus du cloud)
	known := map[string]bool{}
	for i := range db.HotspotUsers {
		if db.HotspotUsers[i].RouterID == router.ID {
			known[strings.ToLower(db.HotspotUsers[i].Username)] = true
		}
	}
	for _, e := range userEntries {
		name := agent.SanitizeName(e[0])
		if name == "" || name == "-" || known[strings.ToLower(name)] {
			continue
		}
		profName := ""
		disabled := false
		if len(e) > 1 {
			profName = e[1]
		}
		if len(e) > 2 && e[2] == "true" {
			disabled = true
		}
		db.HotspotUsers = append(db.HotspotUsers, model.HotspotUser{
			ID: model.NewID("u-"), AccountID: router.AccountID, Kind: "regular", Username: name,
			ProfileName: profName, RouterID: router.ID, RouterName: router.Name,
			Status:    map[bool]string{true: "disabled", false: "active"}[disabled],
			CreatedAt: model.NowISO(),
		})
	}

	// Sessions actives : "user|ip|uptime;…"
	sessEntries := splitAgentList(vals.Get("sessions"))
	router.ActiveSessions = len(sessEntries)
	userIDs := map[string]string{}
	for i := range db.HotspotUsers {
		if db.HotspotUsers[i].RouterID == router.ID {
			userIDs[strings.ToLower(db.HotspotUsers[i].Username)] = db.HotspotUsers[i].ID
		}
	}
	live := []model.Session{}
	for _, e := range sessEntries {
		s := model.Session{ID: model.NewID("s-"), AccountID: router.AccountID, Username: e[0], RouterID: router.ID, RouterName: router.Name}
		if len(e) > 1 {
			s.IP = e[1]
		}
		if len(e) > 2 {
			s.UptimeSec = parseRosUptime(e[2])
		}
		if id, ok := userIDs[strings.ToLower(e[0])]; ok {
			s.UserID = id
			for i := range db.HotspotUsers {
				if db.HotspotUsers[i].ID == id {
					s.ProfileName = db.HotspotUsers[i].ProfileName
					break
				}
			}
		}
		s.StartedAt = model.NowISO()
		live = append(live, s)
	}
	kept := db.Sessions[:0]
	for _, s := range db.Sessions {
		if s.RouterID != router.ID {
			kept = append(kept, s)
		}
	}
	db.Sessions = append(kept, live...)
}

// splitAgentList — découpe "a|b|c;d|e|f;…" en entrées.
func splitAgentList(raw string) [][]string {
	out := [][]string{}
	if strings.TrimSpace(raw) == "" {
		return out
	}
	for _, item := range strings.Split(raw, ";") {
		if strings.TrimSpace(item) == "" {
			continue
		}
		parts := strings.Split(item, "|")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		out = append(out, parts)
	}
	return out
}

// ---------------------------------------------------------------------------
// Console : provisionning / rotation / refresh
// ---------------------------------------------------------------------------

// handleRouterProvision — état du provisionning agent d'un routeur.
// Le token n'est pas stocké en clair : après fermeture de la fenêtre de création,
// il faut "Régénérer le script" (rotation) pour en obtenir un nouveau.
func (a *API) handleRouterProvision(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	id := r.PathValue("id")
	a.store.Lock()
	cur := findRouterScoped(a.store.Data(), id, acc)
	if cur == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Routeur introuvable")
		return
	}
	rc := *cur
	a.store.Unlock()

	if rc.Mode != "agent" {
		writeErr(w, http.StatusBadRequest, "Ce routeur n'est pas en mode agent")
		return
	}
	online := false
	if t, err := time.Parse(time.RFC3339, rc.LastSeen); err == nil {
		online = time.Since(t) < OnlineWindow
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"mode":         "agent",
		"tokenPreview": rc.TokenPreview,
		"provisioned":  rc.AgentTokenHash != "",
		"lastSeen":     rc.LastSeen,
		"online":       online,
		"scheduler":    agent.SchedulerName,
		"scriptFile":   agent.ScriptFilename,
		"note":         "Token stocké haché : utilisez rotate-token pour obtenir un nouveau script",
	})
}

// handleRouterRotateToken — régénère le token et renvoie le script complet.
func (a *API) handleRouterRotateToken(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	id := r.PathValue("id")
	token, err := agent.NewToken()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Génération de token impossible")
		return
	}
	a.store.Lock()
	cur := findRouterScoped(a.store.Data(), id, acc)
	if cur == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Routeur introuvable")
		return
	}
	if cur.Mode != "agent" {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, "Ce routeur n'est pas en mode agent")
		return
	}
	cur.AgentTokenHash = agent.HashToken(token)
	cur.TokenPreview = agent.Preview(token)
	cur.LastSeen = ""
	cur.Status = "offline" // l'ancien agent (token périmé) ne check-inera plus
	a.logActivity(a.store.Data(), cur.AccountID, "router", "Token agent régénéré pour «"+cur.Name+"»")
	a.store.Save()
	name := cur.Name
	a.store.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"agentToken":    token,
		"installScript": agent.InstallScript(agentBaseURL(r), token, name),
	})
}

// handleRouterRefresh — file une commande read_state pour un routeur agent.
func (a *API) handleRouterRefresh(w http.ResponseWriter, r *http.Request) {
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
	if cur.Mode != "agent" {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, "Le rafraîchissement agent ne s'applique qu'au mode agent")
		return
	}
	cmd := queueCommandLocked(db, cur.AccountID, id, model.CmdReadState, map[string]any{})
	a.store.Save()
	cmdID := cmd.ID
	a.store.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"queued":    true,
		"commandId": cmdID,
		"message":   "Synchronisation en attente du prochain check-in du routeur (≤ 45 s)",
	})
}
