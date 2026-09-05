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

// ---------------------------------------------------------------------------
// N°29 — walled-garden d'inscription publique (runbook N°27-D automatisé)
// ---------------------------------------------------------------------------

// walledGardenDomains — les noms d'hôtes à rendre joignables SANS
// authentification depuis le WiFi du hotspot : l'origine PAGE (console —
// héberge /join/{token}, encodée dans le QR) et l'hôte API (appelé par le
// navigateur du visiteur depuis la page). Sources : MICKLOUD_BASE_URL,
// APP_PUBLIC_URL, ALLOWED_ORIGIN (CORS = exactement les origines navigateur)
// et l'hôte de la requête agent courante (l'API telle que ce déploiement
// l'expose). Dé-dupliqués, triés (signature stable), 10 max — un déploiement
// standard en produit 2 (page + API).
func walledGardenDomains(r *http.Request) []string {
	hosts := make([]string, 0, 4)
	add := func(raw string) {
		if h := agent.SanitizeWGDomain(normalizeWGHost(raw)); h != "" && wgHostUsable(h) {
			hosts = append(hosts, h)
		}
	}
	add(os.Getenv("MIKCLOUD_BASE_URL"))
	add(os.Getenv("APP_PUBLIC_URL"))
	for _, o := range strings.Split(os.Getenv("ALLOWED_ORIGIN"), ",") {
		add(strings.TrimSpace(o))
	}
	if r != nil {
		add(r.Host)
	}
	seen := make(map[string]bool, len(hosts))
	out := make([]string, 0, len(hosts))
	for _, h := range hosts {
		if !seen[h] {
			seen[h] = true
			out = append(out, h)
		}
	}
	sort.Strings(out)
	if len(out) > 10 {
		out = out[:10]
	}
	return out
}

// wgHostUsable — filtre les hôtes inutiles voire nuisibles dans un
// walled-garden hotspot (complément N°31-b) : boucle locale, RFC1918,
// link-local, mDNS et 0.0.0.0 ne sont PAS joignables depuis un client du
// WiFi — les autoriser ne protège aucun flux réel et pollue la table
// walled-garden du gérant (constat prod : « localhost:3000 » issu des
// origines de dev de ALLOWED_ORIGIN). Les hôtes publics restent éligibles,
// port numérique compris (le Host HTTP l'inclut sur les ports non standard).
func wgHostUsable(h string) bool {
	if h == "" {
		return false
	}
	host := h
	if i := strings.IndexByte(host, ':'); i >= 0 {
		host = host[:i] // port retiré pour l'évaluation (un FQDN ne porte pas « : »)
	}
	if host == "" {
		return false // IPv6 abrégée (« ::1 ») — non exprimable en dst-host
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") {
		return false
	}
	if o := strings.Split(host, "."); len(o) == 4 { // candidat IPv4
		nums := make([]int, 4)
		ok := true
		for i, p := range o {
			n, err := strconv.Atoi(p)
			if err != nil || p == "" || len(p) > 3 {
				ok = false
				break
			}
			nums[i] = n
		}
		if ok {
			a, b := nums[0], nums[1]
			if a == 0 || a == 10 || a == 127 || (a == 169 && b == 254) || (a == 172 && b >= 16 && b <= 31) || (a == 192 && b == 168) {
				return false
			}
		}
	}
	return true
}

// normalizeWGHost — extrait l'hôte brut d'une origine/URL/hôte : préfixe de
// schéma, userinfo et chemin retirés, ports par défaut (80/443) retirés.
// La validation fine du jeu de caractères est faite par agent.SanitizeWGDomain.
func normalizeWGHost(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if i := strings.LastIndexByte(s, '@'); i >= 0 { // userinfo parasites
		s = s[i+1:]
	}
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSuffix(s, ":443")
	s = strings.TrimSuffix(s, ":80")
	return s
}

// walledGardenSig — signature courte et stable d'une configuration de
// domaines (hash du join trié) : elle distingue « déjà appliqué sur ce
// routeur » d'« à (re)appliquer » sans table supplémentaire.
func walledGardenSig(domains []string) string {
	return agent.HashToken(strings.Join(domains, "|"))[:16]
}

// ensureWalledGardenLocked — sous verrou : si la configuration walled-garden
// courante diffère de celle déjà appliquée sur le routeur (et qu'aucune
// commande n'est en vol), file la mise à jour — elle est servie dans CE
// check-in. C'est le point N°29 pour les routeurs DÉJÀ EN LIGNE lors du
// déploiement : aucun recollage manuel, chaque agent se met à niveau tout
// seul à son premier check-in (≤ 45 s). La signature n'est posée qu'au
// retour « ok » (handleAgentResult) : un échec est retenté au check-in
// suivant, un changement de config re-file automatiquement.
func ensureWalledGardenLocked(db *model.DB, router *model.Router, domains []string) {
	if len(domains) == 0 {
		return
	}
	sig := walledGardenSig(domains)
	if router.WalledGardenSig == sig {
		return // déjà appliqué avec cette configuration exacte
	}
	for i := range db.Commands {
		c := &db.Commands[i]
		if c.RouterID == router.ID && c.Kind == model.CmdWalledGarden && (c.Status == "queued" || c.Status == "sent") {
			return // une mise à jour est déjà en vol
		}
	}
	queueCommandLocked(db, router.AccountID, router.ID, model.CmdWalledGarden, map[string]any{
		"domains": domains,
		"sig":     sig,
	})
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

// staleSentReadKinds — kinds dont la re-exécution est sans effet de bord
// (idempotents) : seuls ceux-là sont repris s'ils restent « sent » sans
// rapport (les écritures non idempotentes ne sont jamais re-exécutées).
// Audit N°31 : walled_garden rejoint la liste — le bloc est conçu
// idempotent (remove+add des seules règles marquées mikcloud-wg). Sans
// cela, UN rapport perdu (blip réseau entre l'import et le fetch de
// rapport, reboot en cours de check-in…) laissait la commande « sent » à
// jamais : ensureWalledGardenLocked la croyait « en vol » et ne la
// re-filait jamais — walled-garden jamais appliqué, en silence.
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
	model.CmdWalledGarden:  true, // N°31 : idempotent (marqueur mikcloud-wg)
}

// staleSentLimit — au-delà de cette ancienneté sans rapport, une commande
// idempotente « sent » est considérée perdue et repart en file.
const staleSentLimit = 10 * time.Minute

// requeueStaleReadsLocked — remet en file les commandes IDEMPOTENTES
// « sent » zombies — lectures ET walled_garden (sous verrou ; Save à charge
// de l'appelant, comme le reste du flux).
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
		log.Printf("agent/cmd: commandes idempotentes « sent » sans rapport reprises en file (routeur %s)", routerID)
	}
}

// touchAgent — marque le routeur en ligne (sous verrou).
func touchAgent(r *model.Router) {
	r.Status = "online"
	r.LastSeen = model.NowISO()
}

// ---------------------------------------------------------------------------
// Sécurité S6 — détection d'identité routeur dupliquée (anti-fermage d'essai)
// ---------------------------------------------------------------------------
// L'agent déclare son empreinte RouterOS (System Identity + board-name) au
// register (script d'installation : /system identity get name + /system
// resource get board-name). Un client sous paywall (guard P3) peut créer un
// nouveau compte d'essai et y re-provisionner le MÊME routeur physique : le
// nouveau script écrase l'ancien scheduler, l'ancien compte devient muet.
// Le dédoublonnage des coordonnées (S5, handlers_auth.go) bloque les
// coordonnées réutilisées, pas les identités inventées. Ce garde-fou ferme
// la boucle :
//
//   - POST /agent/register : l'empreinte déclarée est comparée aux routeurs
//     ACTIFS (LastSeen < 24 h) des AUTRES comptes. Conflit → 409 code
//     « router_identity_conflict » + flag persistant IdentityConflict.
//   - GET /agent/cmd : un routeur flaggé ne reçoit AUCUNE commande tant que
//     le porteur de l'empreinte reste actif ; le flag se lève automatiquement
//     dès que le porteur disparaît (suppression du routeur fantôme par le
//     support — impersonation) ou dort plus de 24 h — le check-in reprend
//     alors normalement.
//
// Exclusions assumées : identités génériques (« mikrotik », défaut RouterOS,
// ou vide — des milliers d'appareils non renommés la portent) et MÊME compte
// (re-register, rotate-token, doublon logique = gestion interne du client,
// pas de l'abus plateforme). Fenêtre de 24 h : compromis documenté contre le
// faux positif « routeur revendu » — le support débloque en supprimant le
// routeur fantôme de l'ancien compte, ou l'attente naturelle sort son
// LastSeen de la fenêtre. Limite assumée : l'identity est forgeable par qui
// contrôle le routeur — la barrière vise le fermage de masse paresseux et
// rend TOUTE tentative visible dans le journal d'activité (traçabilité).

// identityConflictWindow — fenêtre de récence du porteur de l'empreinte :
// un routeur qui n'a plus check-in depuis plus de 24 h n'est plus considéré
// actif (l'appareil physique a vraisemblablement quitté ce compte).
const identityConflictWindow = 24 * time.Hour

// normalizeRouterIdent — normalisation d'empreinte : trim + minuscules.
func normalizeRouterIdent(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// genericRouterIdentity — identité non discriminante (défaut RouterOS ou
// vide) : jamais de conflit sur une empreinte générique.
func genericRouterIdentity(ident string) bool {
	return ident == "" || ident == "mikrotik"
}

// identityHolderLocked — renvoie le routeur qui PORTE déjà cette empreinte
// (identity + modèle, normalisées) sur un AUTRE compte, vu ACTIF récemment
// (LastSeen < identityConflictWindow) ; nil sinon. Appelable sous verrou
// store. Garde interne : empreinte non discriminante → nil.
func identityHolderLocked(db *model.DB, router *model.Router, ident, mod string) *model.Router {
	if genericRouterIdentity(ident) || mod == "" {
		return nil
	}
	now := time.Now().UTC()
	for i := range db.Routers {
		other := &db.Routers[i]
		if other.ID == router.ID || other.AccountID == router.AccountID {
			continue // même routeur (re-register) ou même compte (gestion interne)
		}
		if normalizeRouterIdent(other.Host) != ident || normalizeRouterIdent(other.BoardName) != mod {
			continue // empreinte différente
		}
		seen, err := time.Parse(time.RFC3339, other.LastSeen)
		if err != nil || now.Sub(seen) >= identityConflictWindow {
			continue // porteur jamais check-in ou endormi hors fenêtre
		}
		return other
	}
	return nil
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
			// Audit N°31 — le refus est rendu VISIBLE du gérant (le journal
			// d'activité est sa seule fenêtre côté console : sans lui, un
			// routeur < 7.19 semble « en ligne » mais reste muet à jamais).
			registerVer := router.Version
			a.logActivity(db, router.AccountID, "router",
				"Agent « "+name+" » refusé : RouterOS "+registerVer+" < 7.19 requis (validation TLS stricte) — mettez à jour le routeur (System → Packages) puis réinstallez l'agent")
			a.store.Save()
			a.store.Unlock()
			log.Printf("agent/register: REFUS RouterOS %q (< 7.19, TLS requis) — routeur « %s »", router.Version, name)
			writeErrCode(w, http.StatusUpgradeRequired, "routeros_too_old",
				"RouterOS 7.19 ou plus récent requis (validation TLS stricte). Version déclarée : "+router.Version+". Mettez à jour le routeur (System → Packages) puis recollez le script d'installation.", nil)
			return
		}
	}
	// S6 — empreinte de l'appareil : l'identity COURANTE est enregistrée à
	// chaque register (avant : seulement si Host vide — un renommage RouterOS
	// n'était jamais répercuté), le modèle board-name complète l'empreinte.
	identL := normalizeRouterIdent(vals.Get("identity"))
	modL := normalizeRouterIdent(vals.Get("model"))
	if identL != "" {
		router.Host = strings.TrimSpace(vals.Get("identity"))
	}
	if modL != "" {
		router.BoardName = modL
	}
	// S6 — détection de conflit : l'empreinte déclarée est-elle déjà portée
	// par un routeur actif d'un autre compte ? (fermage d'essai : même appareil
	// physique re-provisionné sur un compte frais). Refus + flag persistant ;
	// le check-in (cmd) restera bloqué tant que le conflit vit.
	if holder := identityHolderLocked(db, router, identL, modL); holder != nil {
		router.IdentityConflict = true
		a.logActivity(db, router.AccountID, "router",
			"Inscription agent REFUSÉE : identité « "+vals.Get("identity")+" » ("+modL+") déjà active sur un autre compte — contact du support requis")
		a.store.Save()
		name, hName := router.Name, holder.Name
		a.store.Unlock()
		log.Printf("agent/register: REFUS conflit identité %q/%q — routeur « %s » déjà porté par « %s » (autre compte)", identL, modL, name, hName)
		writeErrCode(w, http.StatusConflict, "router_identity_conflict",
			"Identité de routeur déjà active sur un autre compte — contactez le support MikCloud si vous êtes le propriétaire légitime de cet appareil", nil)
		return
	}
	// S6 — register sans conflit (re-installation, redémarrage…) : le flag
	// éventuel d'un conflit précédent est levé.
	router.IdentityConflict = false
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

	// Sécurité S6 — un routeur marqué en conflit d'identité (empreinte déjà
	// active sur un autre compte, cf. handleAgentRegister) ne reçoit AUCUNE
	// commande : le fermage d'essai ne doit rien produire (ni read_state, ni
	// écritures sur le routeur). Le conflit est RE-VÉRIFIÉ à chaque check-in :
	// dès que le porteur disparaît (suppression par le support) ou dort
	// (LastSeen > 24 h), le flag se lève automatiquement et le service normal
	// reprend. Le script agent n'affiche pas ce refus (réponse texte hors
	// contrat 200) : le routeur reste simplement sans instruction.
	if router.IdentityConflict {
		if holder := identityHolderLocked(db, router, normalizeRouterIdent(router.Host), normalizeRouterIdent(router.BoardName)); holder != nil {
			a.store.Save() // persiste le touchAgent du check-in
			name, hName := router.Name, holder.Name
			a.store.Unlock()
			log.Printf("agent/cmd: REFUS conflit identité — aucune commande pour « %s » (empreinte portée par « %s », autre compte)", name, hName)
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte("# mikcloud: identite de routeur deja active sur un autre compte - contactez le support\n"))
			return
		}
		// Conflit disparu : levée du flag, tracée, le check-in continue.
		router.IdentityConflict = false
		a.logActivity(db, router.AccountID, "router", "Conflit d'identité levé — agent « "+router.Name+" » réactivé automatiquement")
		a.store.Save()
	}

	// P0 (audit Mikhmon) — F1 : l'agent reçoit l'enforcement des
	// expirations à son check-in (les commandes déposées ici sont servies
	// dans le MÊME check-in, juste après).
	a.enforceExpired(db)

	// N°29 — walled-garden d'inscription publique : si la configuration
	// (domaines page+API) a changé ou n'a jamais été appliquée sur ce
	// routeur, la commande walled_garden rejoint la file FIFO ci-dessous —
	// servie dans CE check-in, y compris pour les routeurs déjà en ligne.
	ensureWalledGardenLocked(db, router, walledGardenDomains(r))

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
		if cmd.Kind == model.CmdWalledGarden {
			// N°29 — configuration appliquée et CONFIRMÉE par le routeur :
			// la signature est posée ici (et seulement ici) — un échec sera
			// retenté au check-in suivant, un changement de config re-file.
			if sig, _ := cmd.Payload["sig"].(string); sig != "" {
				router.WalledGardenSig = sig
			}
			a.logActivity(db, router.AccountID, "router", "Walled-garden d'inscription publique appliqué sur «"+router.Name+"»")
		} else {
			a.logActivity(db, router.AccountID, "router", "Commande "+cmd.Kind+" exécutée sur «"+router.Name+"»")
		}
		queueCommandLocked(db, router.AccountID, router.ID, model.CmdReadState, map[string]any{})
	default:
		a.logActivity(db, router.AccountID, "router", "Commande "+cmd.Kind+" ÉCHOUÉE sur «"+router.Name+"» ("+vals.Get("message")+")")
	}
	purgeOldCommands(db)
	a.store.Save()
	a.store.Unlock()

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
