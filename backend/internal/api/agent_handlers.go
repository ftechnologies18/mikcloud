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
//	POST /api/routers/{id}/import        → file un import complet (profils + utilisateurs)
package api

import (
	"fmt"
	"log"
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
	mux.HandleFunc("POST /api/routers/{id}/import", a.handleRouterImport)
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
	if kind == model.CmdReadState || kind == model.CmdImportHotspot {
		for i := range db.Commands {
			if db.Commands[i].RouterID == routerID && db.Commands[i].Kind == kind && db.Commands[i].Status == "queued" {
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
		log.Printf("agent/register: token inconnu (préfixe %s…) — identité déclarée %q", agent.Preview(token), vals.Get("identity"))
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
	log.Printf("agent/register: routeur « %s » inscrit (identity=%q version=%q)", name, vals.Get("identity"), vals.Get("version"))

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
		log.Printf("agent/cmd: token inconnu (préfixe %s…) — 404 renvoyé à %s", agent.Preview(token), r.RemoteAddr)
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("# mikcloud: token inconnu\n"))
		return
	}
	touchAgent(router)

	// P0 (audit Mikhmon) — F1 : l'agent reçoit l'enforcement des
	// expirations à son check-in (les commandes déposées ici sont servies
	// dans le MÊME check-in, juste après).
	a.enforceExpired(db)

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
	// P1 (audit Mikhmon) — F8 : le rapport ping arrive en valeurs formulaire
	// (chaînes) ; le front attend des nombres + lossPct → normalisation ici.
	if ok && cmd.Kind == model.CmdPing {
		normalizePingResult(cmd.Result)
	}

	switch {
	case cmd.Kind == model.CmdReadState && ok:
		a.applyReadState(db, router, vals)
		a.logActivity(db, router.AccountID, "router", "Routeur «"+router.Name+"» synchronisé ("+
			strconv.Itoa(router.ActiveSessions)+" session(s) active(s), "+strconv.Itoa(router.HotspotUsers)+" utilisateur(s))")
		// P1 (audit Mikhmon) — F6/F8 : télémétrie CONTINUE. Le read_state
		// suivant est enfilé dès maintenant (dédupliqué par
		// queueCommandLocked) : chaque check-in (≤ 45 s) rapporte un état
		// frais — trafic, carte, disque, sessions.
		queueCommandLocked(db, router.AccountID, router.ID, model.CmdReadState, map[string]any{})
	case cmd.Kind == model.CmdImportHotspot && ok:
		summary, more := a.applyImportHotspot(db, router, *cmd, vals)
		if more {
			a.logActivity(db, router.AccountID, "router", "Import depuis «"+router.Name+"» : "+summary+" (suite programmée…)")
		} else {
			a.logActivity(db, router.AccountID, "router", "Import depuis «"+router.Name+"» terminé : "+summary)
		}
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
//
// P1 (audit Mikhmon) :
//   - F8 : board / freehdd / totalhdd (Mo, divisés côté script) →
//     Router.BoardName / FreeHddMb / TotalHddMb ;
//   - F6 : ifaces=name:rx:tx;… → diff des compteurs cumulés avec l'état
//     précédent (db.Traffic) → débits par interface + point d'historique ;
//   - F3 : diff des sessions avant/après remplacement → UserLogs login/logout
//     (un utilisateur déjà présent n'est pas re-journalisé).
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
	// F8 — carte + disque (Mo ; division octets→Mo faite côté script).
	if v := strings.TrimSpace(vals.Get("board")); v != "" && len(v) <= 64 {
		router.BoardName = v
	}
	if v, err := strconv.Atoi(strings.TrimSpace(vals.Get("freehdd"))); err == nil && v >= 0 && v <= 1<<20 {
		router.FreeHddMb = v
	}
	if v, err := strconv.Atoi(strings.TrimSpace(vals.Get("totalhdd"))); err == nil && v >= 0 && v <= 1<<20 {
		router.TotalHddMb = v
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

	// F3 — capture des usernames présents AVANT remplacement (diff).
	prevByUser := map[string]model.Session{}
	for _, s := range db.Sessions {
		if s.RouterID == router.ID {
			if _, ok := prevByUser[s.Username]; !ok {
				prevByUser[s.Username] = s
			}
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

	// F3 — nouvelles sessions → login ; disparues → logout.
	now := time.Now().UTC()
	liveNames := map[string]bool{}
	for i := range live {
		liveNames[live[i].Username] = true
		if _, ok := prevByUser[live[i].Username]; !ok {
			logRouterUserEvent(db, router, live[i], "login", now)
		}
	}
	for _, s := range prevByUser {
		if !liveNames[s.Username] {
			logRouterUserEvent(db, router, s, "logout", now)
		}
	}

	kept := db.Sessions[:0]
	for _, s := range db.Sessions {
		if s.RouterID != router.ID {
			kept = append(kept, s)
		}
	}
	db.Sessions = append(kept, live...)

	// F6 — trafic : diff des compteurs cumulés par interface.
	applyAgentTraffic(db, router, vals.Get("ifaces"), now)
}

// logRouterUserEvent — journalise un login/logout détecté par diff de sessions
// (F3, applyReadState — mode agent). MAC inconnue du rapport : laissée vide.
func logRouterUserEvent(db *model.DB, router *model.Router, s model.Session, action string, now time.Time) {
	if db.UserLogs == nil {
		db.UserLogs = []model.UserLog{}
	}
	db.UserLogs = append(db.UserLogs, model.UserLog{
		ID:         model.NewID("ul-"),
		AccountID:  router.AccountID,
		UserID:     s.UserID,
		Username:   s.Username,
		Action:     action,
		RouterID:   router.ID,
		RouterName: router.Name,
		IP:         s.IP,
		MAC:        s.MAC,
		At:         now.UTC().Format(time.RFC3339),
	})
}

// parseIfaceCounters — décode « name:rx:tx;… » (compteurs cumulés en octets,
// rapport read_state v2). Les entrées malformées sont ignorées.
func parseIfaceCounters(raw string) []model.IfaceTraffic {
	out := []model.IfaceTraffic{}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return out
	}
	for _, item := range strings.Split(raw, ";") {
		if strings.TrimSpace(item) == "" {
			continue
		}
		parts := strings.Split(item, ":")
		if len(parts) != 3 || strings.TrimSpace(parts[0]) == "" {
			continue
		}
		rx, err1 := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
		tx, err2 := strconv.ParseInt(strings.TrimSpace(parts[2]), 10, 64)
		if err1 != nil || err2 != nil || rx < 0 || tx < 0 {
			continue
		}
		out = append(out, model.IfaceTraffic{Name: strings.TrimSpace(parts[0]), RxBytes: rx, TxBytes: tx})
	}
	return out
}

// applyAgentTraffic — F6 : calcule les débits par interface à partir du diff
// des compteurs cumulés (« delta octets × 8 / delta temps », RxBps/TxBps sont
// en bits/s — aligné sur le seed et formatBitsPerSec du front) :
//   - première mesure  → compteurs stockés, débits 0 (pas de référence) ;
//   - interface inconnue ou compteur décroissant (reboot) → débit 0 ;
//   - sinon            → RxBps/TxBps + TrafficPoint (somme des interfaces)
//     ajouté à History (cap 60).
//
// UpdatedAt est stocké en RFC3339Nano : deux read_state peuvent être très
// rapprochés (test/poll rapide) — la précision sub-seconde évite un delta nul.
func applyAgentTraffic(db *model.DB, router *model.Router, raw string, now time.Time) {
	ifaces := parseIfaceCounters(raw)
	if len(ifaces) == 0 {
		return
	}
	var tr *model.RouterTraffic
	for i := range db.Traffic {
		if db.Traffic[i].RouterID == router.ID {
			tr = &db.Traffic[i]
			break
		}
	}
	if tr == nil {
		if db.Traffic == nil {
			db.Traffic = []model.RouterTraffic{}
		}
		db.Traffic = append(db.Traffic, model.RouterTraffic{
			ID: router.ID, RouterID: router.ID, AccountID: router.AccountID,
			UpdatedAt:  now.UTC().Format(time.RFC3339Nano),
			Interfaces: ifaces,
			History:    []model.TrafficPoint{},
		})
		return // première mesure : référence posée, débits à 0
	}

	dt := 0.0
	if prev, err := time.Parse(time.RFC3339, tr.UpdatedAt); err == nil {
		if d := now.Sub(prev).Seconds(); d > 0 {
			dt = d
		}
	}
	prevIface := make(map[string]model.IfaceTraffic, len(tr.Interfaces))
	for _, it := range tr.Interfaces {
		prevIface[it.Name] = it
	}
	out := make([]model.IfaceTraffic, 0, len(ifaces))
	var sumRx, sumTx int64
	for _, it := range ifaces {
		rxBps, txBps := int64(0), int64(0)
		if dt > 0 {
			if p, ok := prevIface[it.Name]; ok {
				if it.RxBytes > p.RxBytes {
					rxBps = int64(float64(it.RxBytes-p.RxBytes) * 8 / dt)
				}
				if it.TxBytes > p.TxBytes {
					txBps = int64(float64(it.TxBytes-p.TxBytes) * 8 / dt)
				}
			}
		}
		out = append(out, model.IfaceTraffic{
			Name: it.Name, RxBytes: it.RxBytes, TxBytes: it.TxBytes, RxBps: rxBps, TxBps: txBps,
		})
		sumRx += rxBps
		sumTx += txBps
	}
	tr.Interfaces = out
	tr.History = append(tr.History, model.TrafficPoint{
		T: now.UTC().Format(time.RFC3339), RxBps: sumRx, TxBps: sumTx,
	})
	if len(tr.History) > 60 {
		tr.History = tr.History[len(tr.History)-60:]
	}
	tr.UpdatedAt = now.UTC().Format(time.RFC3339Nano)
}

// normalizePingResult — F8 : le rapport agent arrive en valeurs formulaire
// (chaînes « sent=4&received=4&minMs=12… ») ; le front (toPingStats) attend
// des NOMBRES + lossPct. Conversion ici, à l'arrivée du rapport, pour que
// GET /api/commands/{id} serve un result directement consommable.
func normalizePingResult(res map[string]any) {
	if res == nil {
		return
	}
	for _, k := range []string{"sent", "received", "minMs", "avgMs", "maxMs"} {
		switch v := res[k].(type) {
		case string:
			if n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil {
				res[k] = n
			}
		case float64:
			res[k] = int64(v)
		}
	}
	sent, okSent := res["sent"].(int64)
	recv, okRecv := res["received"].(int64)
	if okSent && okRecv && sent > 0 {
		loss := (sent - recv) * 100 / sent
		if loss < 0 {
			loss = 0
		}
		res["lossPct"] = loss
	}
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
	a.logActivityBy(r, a.store.Data(), cur.AccountID, "router", "Token agent régénéré pour «"+cur.Name+"»")
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

// handleRouterImport — file un import complet (profils + utilisateurs) d'un
// routeur agent vers le cloud. Utilisé après la première connexion d'un
// routeur qui gérait déjà son hotspot (Mikhmon, configuration manuelle…).
func (a *API) handleRouterImport(w http.ResponseWriter, r *http.Request) {
	// P3 — compte expiré : écritures métier refusées (l'import crée des
	// utilisateurs hotspot — lecture seule).
	if !a.guardAccountWrite(w, r) {
		return
	}
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
		writeErr(w, http.StatusBadRequest, "L'import ne s'applique qu'aux routeurs en mode agent")
		return
	}
	if cur.Status != "online" {
		a.store.Unlock()
		writeErr(w, http.StatusConflict, "Le routeur doit être en ligne pour importer (le check-in de l'agent alimente l'import)")
		return
	}
	cmd := queueCommandLocked(db, cur.AccountID, id, model.CmdImportHotspot,
		map[string]any{"start": 0, "count": agent.ImportChunkSize})
	a.store.Save()
	cmdID := cmd.ID
	a.store.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"queued":    true,
		"commandId": cmdID,
		"message":   "Import en attente du prochain check-in du routeur (≤ 45 s)",
	})
}

// parseRosDurationMin — parse une durée RouterOS en minutes :
// "08:00:00" → 480, "1d" → 1440, "1d02:00:00" → 1560, "1h30m" → 90, "45s" → 1.
// Retourne 0 si la valeur est vide/illisible.
func parseRosDurationMin(s string) int {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 0
	}
	// Format horloge RouterOS : [Nd]H:MM:SS (ou MM:SS).
	if strings.Contains(s, ":") {
		days := 0
		if idx := strings.Index(s, "d"); idx >= 0 {
			if d, err := strconv.Atoi(strings.TrimSpace(s[:idx])); err == nil {
				days = d
			}
			s = s[idx+1:]
		}
		parts := strings.Split(s, ":")
		h, m, sec := 0, 0, 0
		if len(parts) >= 3 {
			h, _ = strconv.Atoi(parts[0])
			m, _ = strconv.Atoi(parts[1])
			sec, _ = strconv.Atoi(parts[2])
		} else if len(parts) == 2 {
			m, _ = strconv.Atoi(parts[0])
			sec, _ = strconv.Atoi(parts[1])
		}
		total := days*1440 + h*60 + m
		if total == 0 && sec > 0 {
			total = 1
		}
		return total
	}
	// Format suffixé : 1d2h3m4s.
	total := 0
	num := ""
	for _, c := range s {
		if c >= '0' && c <= '9' {
			num += string(c)
			continue
		}
		n := 0
		if num != "" {
			n, _ = strconv.Atoi(num)
			num = ""
		}
		switch c {
		case 'd':
			total += n * 1440
		case 'h':
			total += n * 60
		case 'm':
			total += n
		case 's':
			if n > 0 && total == 0 {
				total = 1 // arrondi : moins d'une minute → 1 min
			}
		}
	}
	return total
}

// applyImportHotspot — applique le résultat d'un chunk d'import (profils +
// utilisateurs existants sur le routeur).
//
// Règles :
//   - le cloud reste MAÎTRE : une entrée déjà connue n'est jamais écrasée
//     (les profils matchent par AccountID+nom ; les utilisateurs par
//     username DU COMPTE — voir usernameTaken) ;
//   - l'import est paginé : si le chunk n'était pas le dernier, le suivant
//     est enfilé ici (même mécanique que read_state) ;
//   - les utilisateurs importés sont "regular", statut actif/disabled, avec
//     leur profil (nom) et leur commentaire (tracé Mikhmon éventuel).
func (a *API) applyImportHotspot(db *model.DB, router *model.Router, cmd model.Command, vals url.Values) (summary string, moreQueued bool) {
	now := model.NowISO()
	acc := router.AccountID

	// 1) Profils : name|rate-limit|shared-users|session-timeout;…
	profAdded := 0
	for _, e := range splitAgentList(vals.Get("profiles")) {
		name := agent.SanitizeName(e[0])
		if name == "" || name == "-" {
			continue
		}
		exists := false
		for i := range db.Profiles {
			if db.Profiles[i].AccountID == acc && strings.EqualFold(db.Profiles[i].Name, name) {
				exists = true
				break
			}
		}
		if exists {
			continue
		}
		rate := "1M/1M"
		if len(e) > 1 && strings.TrimSpace(e[1]) != "" {
			rate = strings.TrimSpace(e[1])
		}
		shared := 1
		if len(e) > 2 {
			if n, err := strconv.Atoi(e[2]); err == nil && n > 0 {
				shared = n
			}
		}
		st := 60
		if len(e) > 3 {
			if m := parseRosDurationMin(e[3]); m > 0 {
				st = m
			}
		}
		db.Profiles = append(db.Profiles, model.Profile{
			ID: model.NewID("p-"), AccountID: acc, Name: name, RateLimit: rate,
			SessionTimeoutMin: st, SharedUsers: shared, ValidityDays: 1,
			Price: 0, DataQuotaMb: 0, CreatedAt: now,
			ExpMode: "notify",
		})
		profAdded++
	}

	// 2) Utilisateurs : name|profile|disabled|comment|limit-bytes-total;…
	userAdded := 0
	knownUser := map[string]bool{}
	for i := range db.HotspotUsers {
		if db.HotspotUsers[i].AccountID == acc {
			knownUser[strings.ToLower(db.HotspotUsers[i].Username)] = true
		}
	}
	for _, e := range splitAgentList(vals.Get("users")) {
		name := agent.SanitizeName(e[0])
		if name == "" || name == "-" || knownUser[strings.ToLower(name)] {
			continue
		}
		profName := ""
		if len(e) > 1 {
			profName = agent.SanitizeName(e[1])
		}
		disabled := len(e) > 2 && (e[2] == "true" || e[2] == "yes")
		comment := ""
		if len(e) > 3 && e[3] != "-" {
			comment = e[3]
		}
		db.HotspotUsers = append(db.HotspotUsers, model.HotspotUser{
			ID: model.NewID("u-"), AccountID: acc, Kind: "regular",
			Username: name, ProfileName: profName,
			RouterID: router.ID, RouterName: router.Name,
			Status:    map[bool]string{true: "disabled", false: "active"}[disabled],
			Comment:   comment,
			CreatedAt: now,
		})
		knownUser[strings.ToLower(name)] = true
		userAdded++
	}

	// 3) Pagination : si le routeur a plus d'utilisateurs que ce chunk,
	//    file le chunk suivant (dédupliqué par queueCommandLocked).
	if total, err := strconv.Atoi(vals.Get("total")); err == nil && total > 0 {
		start := int(plPayloadInt(cmd.Payload, "start"))
		count := int(plPayloadInt(cmd.Payload, "count"))
		if count <= 0 {
			count = agent.ImportChunkSize
		}
		if start+count < total {
			queueCommandLocked(db, acc, router.ID, model.CmdImportHotspot,
				map[string]any{"start": start + count, "count": count})
			moreQueued = true
		}
	}

	summary = fmt.Sprintf("%d nouveau(x) profil(s), %d utilisateur(s) importé(s)", profAdded, userAdded)
	return summary, moreQueued
}

// plPayloadInt — lit un entier du payload d'une commande (JSON reloadé →
// float64, créé en mémoire → int).
func plPayloadInt(payload map[string]any, key string) int64 {
	if payload == nil {
		return 0
	}
	switch v := payload[key].(type) {
	case float64:
		return int64(v)
	case int:
		return int64(v)
	case int64:
		return v
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(v))
		return int64(n)
	default:
		return 0
	}
}
