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
		parseTolerantQuery(vals, string(body))
	}
	return vals
}

// parseTolerantQuery — parse « k=v&k2=v2 » d'un corps BRUT RouterOS.
// Contrairement à url.ParseQuery (qui rejette tout le corps dès qu'une
// valeur contient un « ; » brut — et avale cette paire), on découpe sur « & »
// uniquement, on coupe sur la PREMIÈRE « = » et on décode le pourcentage de
// façon tolérante (valeur brute conservée si l'échappement est invalide).
// Les « ; » et « | » des valeurs sont préservés : ce sont les séparateurs du
// protocole agent (users=a|b|false;…), relus par splitAgentList côté cloud.
func parseTolerantQuery(vals url.Values, query string) {
	for query != "" {
		pair := query
		if i := strings.IndexByte(query, '&'); i >= 0 {
			pair, query = query[:i], query[i+1:]
		} else {
			query = ""
		}
		if pair == "" {
			continue
		}
		key, value := pair, ""
		if i := strings.IndexByte(pair, '='); i >= 0 {
			key, value = pair[:i], pair[i+1:]
		}
		if key == "" {
			continue
		}
		if dec, err := url.PathUnescape(key); err == nil {
			key = dec
		}
		if dec, err := url.PathUnescape(value); err == nil {
			value = dec
		}
		vals.Add(key, value)
	}
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

// staleSentReadKinds — kinds de LECTURE dont la re-exécution est sans
// effet de bord (idempotents) : seuls ceux-là sont repris s'ils restent
// « sent » sans rapport (les écritures ne sont jamais re-exécutées).
var staleSentReadKinds = map[string]bool{
	model.CmdReadState:     true,
	model.CmdReadDhcp:      true,
	model.CmdReadHosts:     true,
	model.CmdReadCookies:   true,
	model.CmdReadLog:       true,
	model.CmdReadScheduler: true,
	model.CmdReadResources: true,
	model.CmdImportHotspot: true,
	model.CmdPing:          true,
}

// staleSentLimit — au-delà de cette ancienneté sans rapport, une commande
// de lecture « sent » est considérée perdue et repart en file.
const staleSentLimit = 10 * time.Minute

// requeueStaleReadsLocked — remet en file les lectures « sent » zombies
// (sous verrou ; Save à charge de l'appelant, comme le reste du flux).
func requeueStaleReadsLocked(db *model.DB, routerID string) {
	lim := time.Now().Add(-staleSentLimit).Format(time.RFC3339)
	changed := false
	for i := range db.Commands {
		c := &db.Commands[i]
		if c.RouterID == routerID && c.Status == "sent" && staleSentReadKinds[c.Kind] && c.SentAt != "" && c.SentAt < lim {
			c.Status = "queued"
			c.SentAt = ""
			changed = true
		}
	}
	if changed {
		log.Printf("agent/cmd: commandes de lecture « sent » sans rapport reprises en file (routeur %s)", routerID)
	}
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
		"lockFirstDevice":   p.LockFirstDevice,
		// Parité Mikhmon : pools/queues RouterOS portés par le profil
		// (chaque user_add / voucher_batch aligne le profil sur le cloud).
		"addressPool": p.AddressPool,
		"parentQueue": p.ParentQueue,
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
		// Sécurité P0 #5 — TLS strict : pas d'inscription sous RouterOS 7.19.
		// L'agent installer reçoit un échec (« inscription impossible » dans
		// son journal local) ; le message détaille la marche à suivre.
		if !agent.VersionAtLeast(router.Version, 7, 19) {
			name := router.Name
			a.store.Unlock()
			log.Printf("agent/register: REFUS RouterOS %q (< 7.19, TLS requis) — routeur « %s »", router.Version, name)
			writeErrCode(w, http.StatusUpgradeRequired, "routeros_too_old",
				"RouterOS 7.19 ou plus récent requis (validation TLS stricte). Version déclarée : "+router.Version+". Mettez à jour le routeur (System → Packages) puis recollez le script d'installation.", nil)
			return
		}
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

	// Sécurité P0 #5 — TLS strict côté agent : un agent qui s'est déclaré
	// sous RouterOS 7.19 (pas de certificats racine → impossible de valider
	// le certificat du cloud) ne reçoit AUCUNE commande. Version inconnue →
	// tolérée : elle remonte au premier read_state et la garde s'appliquera
	// au check-in suivant — on ne coupe jamais un parc legacy à l'aveugle.
	if !agent.VersionAtLeast(router.Version, 7, 19) {
		name := router.Name
		a.store.Unlock()
		log.Printf("agent/cmd: REFUS RouterOS %q (< 7.19, TLS requis) — aucune commande livrée à « %s »", router.Version, name)
		w.WriteHeader(http.StatusUpgradeRequired)
		_, _ = w.Write([]byte("# mikcloud: RouterOS 7.19+ requis (validation TLS stricte) — mettez a jour le routeur\n"))
		return
	}

	// P0 (audit Mikhmon) — F1 : l'agent reçoit l'enforcement des
	// expirations à son check-in (les commandes déposées ici sont servies
	// dans le MÊME check-in, juste après).
	a.enforceExpired(db)

	// Reprise : une commande de lecture « sent » sans rapport depuis plus de
	// 10 min est un zombie (blip réseau, rejet historique du rapport…) —
	// remise en file pour re-exécution au check-in courant. Lectures
	// idempotentes uniquement : jamais les écritures (double exécution).
	requeueStaleReadsLocked(db, router.ID)

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
