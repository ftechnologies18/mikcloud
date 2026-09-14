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
	"mikcloud/hotspot-api/internal/agent"
	"mikcloud/hotspot-api/internal/model"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// OnlineWindow — fenêtre pendant laquelle un routeur agent est considéré en ligne.
const OnlineWindow = 3 * time.Minute

// registerAgentRoutes — routes publiques (token d'agent) + routes console.
// Les chemins /agent/* sont hors préfixe /api/ : le middleware JWT ne les touche pas.

// registerAgentRoutes — routes publiques (token d'agent) + routes console.
// Les chemins /agent/* sont hors préfixe /api/ : le middleware JWT ne les touche pas.
func (a *API) registerAgentRoutes(mux *http.ServeMux) {
	// Agent (auth par token de routeur)
	mux.HandleFunc("POST /agent/register", a.handleAgentRegister)
	mux.HandleFunc("GET /agent/cmd", a.handleAgentCmd)
	mux.HandleFunc("POST /agent/result", a.handleAgentResult)

	// N°35 — portail captif : le routeur agent /tool fetch ses fichiers
	// personnalisés ici. Hors /api/ (pas de JWT) — l'auth est le token
	// agent (haché côté cloud, 192 bits). TLS strict hérité.
	// {path...} capte aussi /portal/{token}/ (path="") → 404 propre.
	mux.HandleFunc("GET /portal/{token}/{path...}", a.handlePortalFile)

	// Console (auth JWT via middleware /api/)
	mux.HandleFunc("GET /api/routers/{id}/provision", a.handleRouterProvision)
	mux.HandleFunc("POST /api/routers/{id}/rotate-token", a.handleRouterRotateToken)
	mux.HandleFunc("POST /api/routers/{id}/refresh", a.handleRouterRefresh)
	mux.HandleFunc("POST /api/routers/{id}/import", a.handleRouterImport)
	// N°35-d — portail captif : re-déploiement forcé + aperçu HTML (console).
	mux.HandleFunc("POST /api/routers/{id}/redeploy-portal", a.handleRouterRedeployPortal)
	mux.HandleFunc("GET /api/routers/{id}/portal-preview", a.handleRouterPortalPreview)
	// N°49 — walled-garden : réparation forcée (console gérant).
	mux.HandleFunc("POST /api/routers/{id}/repair-walled-garden", a.handleRouterRepairWalledGarden)
	// N°80 — SafeWiFi : niveau de protection DNS du WiFi public (console).
	mux.HandleFunc("PUT /api/routers/{id}/safewifi", a.handleRouterSetSafeWifi)
	// N°81 — Shield : bouclier réseau du WiFi public (console).
	mux.HandleFunc("PUT /api/routers/{id}/shield", a.handleRouterSetShield)
	// N°82 — FamilyGuard : couvre-feu internet du WiFi public (console).
	mux.HandleFunc("PUT /api/routers/{id}/familyguard", a.handleRouterSetFamilyGuard)
	// N°88 — AntiVPN : bloque-VPN du WiFi public (console).
	mux.HandleFunc("PUT /api/routers/{id}/antivpn", a.handleRouterSetAntiVpn)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// agentBaseURL — URL publique du backend pour les scripts :
// env MIKCLOUD_BASE_URL > X-Forwarded-Proto + Host > https://Host (http si local).

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

// ensureWatcherLocked — N°77 — veilleur d'invités : garantit que le scheduler
// mikcloud-watch est déployé sur ce routeur (check-in 20 s pendant qu'un hôte
// non autorisé est présent — un invité est SUR le portail, son claim est servi
// en ≤ 20 s au lieu d'attendre le réveil du scheduler principal, jusqu'à 180 s
// de veille N°75). Pattern walled-garden/scheduler_set : le drapeau
// Router.WatcherOK n'est posé qu'au retour « ok » de la commande — jamais à la
// mise en file. Faux/absent = routeur antérieur au N°77, échec, ou veilleur
// effacé à la main → re-file au check-in suivant (auto-réparation). À appeler
// sous le verrou du store depuis handleAgentCmd.

// ensureReadStateDue — N°74 — cadenceur de la télémétrie read_state. Enfile
// un read_state pour CE routeur si : (1) aucun n'est déjà en file ou en vol
// (statuts queued/sent), ET (2) le dernier appliqué date de plus de
// readStateMinInterval (ou aucun depuis le boot : premier check-in). À
// appeler sous le verrou du store, depuis handleAgentCmd — la commande est
// servie dans LE MÊME check-in (elle rejoint la FIFO juste avant le service).
func (a *API) ensureReadStateDue(db *model.DB, router *model.Router) {
	for i := range db.Commands {
		c := &db.Commands[i]
		if c.RouterID == router.ID && c.Kind == model.CmdReadState &&
			(c.Status == "queued" || c.Status == "sent") {
			return // déjà en file ou en vol (chunks compris) : rien à faire
		}
	}
	last, ok := a.readStateDone[router.ID]
	// N°76 — cadence adaptée à la taille du parc : un cycle paginé de N
	// chunks coûte N scripts servis — l'intervalle minimum est multiplié
	// par N (un parc de 3 500 users se réconcilie toutes les ~14 min :
	// même régime egress que N°75, réconciliation enfin réelle).
	interval := readStateMinInterval
	if n := a.readStateChunks[router.ID]; n > 1 {
		interval = readStateMinInterval * time.Duration(n)
	}
	if ok && time.Since(last) < interval {
		return // pas encore dû
	}
	queueCommandLocked(db, router.AccountID, router.ID, model.CmdReadState, map[string]any{})
}

// ---------------------------------------------------------------------------
// GET /agent/cmd — défilement de la file et génération du script
// ---------------------------------------------------------------------------

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

	// N°35 — portail captif : si l'ensemble des fichiers du portail a
	// changé (nouveau template, nouvelle liste d'assets) ou n'a jamais
	// été déployé sur ce routeur, la commande hotspot_files rejoint la
	// file FIFO ci-dessous. Le contenu est personnalisé au moment du
	// SERVE (GET /portal/{token}/{path}) ; ici on ne file que la liste
	// des chemins à déployer. Zéro intervention humaine : le gérant
	// change sa config dans la console, l'agent re-déploie tout seul au
	// prochain check-in (≤ 45 s) si la sig a changé.
	ensureHotspotFilesLocked(db, router)

	// Reprise : une commande de lecture « sent » sans rapport depuis plus de
	// 10 min est un zombie (blip réseau, rejet historique du rapport…) —
	// remise en file pour re-exécution au check-in courant. Lectures
	// idempotentes uniquement : jamais les écritures (double exécution).
	requeueStaleReadsLocked(db, router.ID)

	// N°74 — télémétrie cadencée : un read_state est enfilé si (et seulement
	// si) le dernier appliqué date de plus de readStateMinInterval et qu'aucun
	// n'est déjà en file/en vol. Avant : la boucle re-enfilait à CHAQUE
	// résultat (45 s, 24 h/24) — le plus gros poste de bande passante agents
	// pour des snapshots que personne ne consultait la nuit.
	a.ensureReadStateDue(db, router)

	// N°75 — veille adaptative : décide du pas du scheduler (45 s si le
	// routeur est sous attention — console ouverte sur son compte, invité
	// sur son portail, commandes en attente — sinon 240 s de veille) et
	// enfile la bascule si l'état connu diverge. La commande rejoint la
	// FIFO de CE check-in.
	a.ensureSchedulerIntervalLocked(db, router)

	// N°77 — veilleur d'invités : converge le parc existant en UN check-in
	// (WatcherOK absent = routeur antérieur au N°77 → watcher_ensure en
	// file, servi dans CE check-in) puis se tait tant que le retour « ok »
	// n'est pas démenti. Le veilleur rend le claim ≤ 20 s pendant la
	// fenêtre invité SANS réveiller la veille (0 octet émis sans invité).
	a.ensureWatcherLocked(db, router)

	// N°80 — SafeWiFi : converge la protection DNS du WiFi public — rien
	// pour un routeur qui n'a jamais ouvert la carte (économie N°75
	// préservée), re-file automatique au changement de niveau, et
	// auto-réparation périodique (safeWifiRefresh, pattern N°49).
	a.ensureSafeWifiLocked(db, router)

	// N°81 — Shield : converge le bouclier réseau du WiFi public — même
	// contrat (silence si jamais utilisé, re-file au changement,
	// auto-réparation 6 h).
	a.ensureShieldLocked(db, router)

	// N°82 — FamilyGuard : converge le couvre-feu internet du WiFi public
	// — l'ÉTAT désiré (dans la fenêtre ou non) est recalculé à CHAQUE
	// check-in : à chaque frontière de fenêtre (22:00, 06:00…) la
	// signature change et la bascule rejoint la file, servie dans CE
	// check-in (≤ 45 s console ouverte, ≤ 180 s en veille).
	a.ensureFamilyGuardLocked(db, router)

	// N°88 — AntiVPN : converge le bloque-VPN du WiFi public — même
	// contrat (silence si jamais utilisé, re-file au changement,
	// auto-réparation 6 h).
	a.ensureAntiVpnLocked(db, router)

	// N°97 — docteur pool IP : auto-DIAGNOSTIC (lecture seule) quand la
	// capacité n'a jamais été mesurée ou n'est plus fraîche — alimente
	// PoolCap/PoolHosts pour l'alerte « pool plein » aux heures de pointe.
	// Les corrections (recyclage/extension) restent des gestes explicites
	// du gérant via POST /api/routers/{id}/pool-doctor.
	a.ensurePoolDoctorLocked(db, router)

	// N°101 — Phase 3 HomeNet : l'inventaire des appareils (bails DHCP,
	// cadence 2 min — hotspots exclus) puis la convergence de la pause dîner
	// (l'ensemble désiré est recalculé à CHAQUE check-in : une pause qui
	// expire change la signature et se lève d'elle-même, pattern N°82).
	a.ensureHomeDevicesLocked(db, router)
	a.ensureDevicePauseLocked(db, router)

	// N°104 — QoS Manager : converge la file agrégat du hotspot
	// (queue_ensure tant que la signature n'est pas posée/vérifiée,
	// queue_remove après une désactivation, monitoring queue_read
	// 30 min) — rien pour un routeur qui n'a jamais ouvert la carte
	// (économie de veille N°75 préservée).
	a.ensureQoSLocked(db, router)

	// File FIFO : commandes en attente (max 10 par check-in).
	//
	// N°77 — PRIORITÉ AUX ACTIONNABLES : un parc de 3 500 users enfile
	// jusqu'à 10 chunks read_state d'un coup (N°76) — TOUJOURS plus
	// anciens qu'un claim fraîchement posé (ils sont créés au début du
	// cycle) : l'ancien FIFO pur leur donnait les 10 slots du check-in et
	// le claim attendait UN CHECK-IN DE PLUS (jusqu'à +180 s en veille)
	// puis s'exécutait derrière ~30 s de scripts de lecture. Nouvel
	// ordre : actionnables (écritures métier, claim, outils console,
	// veilleur) → chunks read_state (idempotents, cadencés, ré-enfilés
	// par la boucle zombie) → walled_garden/hotspot_files (fermeture
	// habituelle : une ligne avortée ne doit pas tuer ce qui la suit).
	prio := []model.Command{}
	reads := []model.Command{}
	deferred := []model.Command{}
	for i := range db.Commands {
		if db.Commands[i].RouterID != router.ID || db.Commands[i].Status != "queued" {
			continue
		}
		switch db.Commands[i].Kind {
		case model.CmdReadState:
			reads = append(reads, db.Commands[i])
		case model.CmdWalledGarden, model.CmdHotspotFiles, model.CmdSafeWifi, model.CmdShield, model.CmdFamilyGuard, model.CmdAntiVpn, model.CmdPoolDoctor, model.CmdDevicePause, model.CmdQueueEnsure, model.CmdQueueRemove:
			deferred = append(deferred, db.Commands[i])
		default:
			prio = append(prio, db.Commands[i])
		}
	}
	sort.Slice(prio, func(i, j int) bool { return prio[i].CreatedAt < prio[j].CreatedAt })
	sort.Slice(reads, func(i, j int) bool { return reads[i].CreatedAt < reads[j].CreatedAt })
	// Au sein des différés, walled_garden avant hotspot_files (le walled-garden
	// doit être en place pour que le portail puisse appeler l'API cloud pré-auth),
	// puis safewifi (N°80), shield (N°81), familyguard (N°82) et antivpn
	// (N°88) en fermeture, pool_doctor (N°97, lecture seule) en toute fin :
	// ces protections ne dépendent d'aucune autre commande — par ordre de
	// vague : 29 < 35 < 80 < 81 < 82 < 88 < 97).
	deferredWave := func(k string) int {
		switch k {
		case model.CmdWalledGarden:
			return 29
		case model.CmdHotspotFiles:
			return 35
		case model.CmdSafeWifi:
			return 80
		case model.CmdShield:
			return 81
		case model.CmdFamilyGuard:
			return 82
		case model.CmdAntiVpn:
			return 88
		}
		// N°101 — device_pause ferme la vague (la pause ne dépend d'aucune
		// autre commande, aucune écriture ne l'attend derrière).
		if k == model.CmdDevicePause {
			return 101
		}
		// N°104 — la file agrégat ferme TOUT : le rattachement des profils
		// (parent-queue) part au retour VÉRIFIÉ du queue_ensure — jamais
		// en concurrence avec une autre vague.
		if k == model.CmdQueueEnsure || k == model.CmdQueueRemove {
			return 104
		}
		return 97 // CmdPoolDoctor (diagnostic pur : fermeture, ne bloque rien)
	}
	sort.SliceStable(deferred, func(i, j int) bool {
		wi, wj := deferredWave(deferred[i].Kind), deferredWave(deferred[j].Kind)
		if wi != wj {
			return wi < wj
		}
		return deferred[i].CreatedAt < deferred[j].CreatedAt
	})
	queued := append(append(prio, reads...), deferred...)
	if len(queued) > 10 {
		queued = queued[:10]
	}

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
	// N°31-c — battement de cœur du script walled_garden (« status=started ») :
	// preuve de LIVRAISON du fichier, pas un résultat final. La commande
	// reste « sent » — le rapport réel ou la reprise zombie N°31 tranchera.
	if status == "started" {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true, "heartbeat": true})
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
		if len(vs) == 0 || k == "token" {
			continue
		}
		// N°76 — les listes brutes (users d'un chunk de read_state,
		// sessions) sont consommées par applyReadState AVANT ceci : ne
		// pas les persister dans Result (l'historique de commandes
		// gonflait de ~17 Ko par chunk — des Mo/jour de resynchronisation
		// Neon pour un parc de 3 500 users). Seuls les compteurs restent.
		if k == "users" || k == "sessions" {
			continue
		}
		res[k] = vs[0]
	}
	cmd.Result = res
	// P1 (audit Mikhmon) — F8 : le rapport ping arrive en valeurs formulaire
	// (chaînes) ; le front attend des nombres + lossPct → normalisation ici.
	if ok && cmd.Kind == model.CmdPing {
		normalizePingResult(cmd.Result)
	}

	switch {
	case cmd.Kind == model.CmdReadState && ok:
		// N°76 — cycle paginé : les chunks intermédiaires ne comptent
		// ni pour la cadence (readStateDone au FINAL seulement — sinon
		// le cadenceur re-enfilerait pendant le cycle) ni pour le
		// journal (7 lignes/cycle pour un parc de 3 500 users = bruit).
		final, synced := a.applyReadState(db, router, vals)
		if final {
			a.readStateDone[router.ID] = time.Now().UTC()
			if a.readStateChunks == nil {
				a.readStateChunks = map[string]int{}
			}
			chunks := 1
			if tot, ok := parseReportInt(vals.Get("total")); ok && tot > 0 {
				chunks = (tot + agent.ReadChunkSize - 1) / agent.ReadChunkSize
				if chunks < 1 {
					chunks = 1
				}
				if chunks > agent.MaxReadChunks {
					chunks = agent.MaxReadChunks // cadence bornée même sur total malformé
				}
			}
			a.readStateChunks[router.ID] = chunks // cadence adaptée à la taille du parc
		}
		if synced {
			a.logActivity(db, router.AccountID, "router", "Routeur «"+router.Name+"» synchronisé ("+
				strconv.Itoa(router.ActiveSessions)+" session(s) active(s), "+strconv.Itoa(router.HotspotUsers)+" utilisateur(s))")
		}
		// N°74 — télémétrie cadencée : plus de re-enfilement inconditionnel
		// ici (l'ancienne boucle servait un read_state à CHAQUE check-in,
		// 24 h/24). Le cadenceur du check-in suivant (ensureReadStateDue,
		// handleAgentCmd) re-file dès que l'intervalle minimum est écoulé ;
		// les commandes d'écriture ci-dessous re-enfilent TOUJOURS
		// immédiatement (fraîcheur post-action préservée).
	case cmd.Kind == model.CmdImportHotspot && ok:
		summary, more := a.applyImportHotspot(db, router, *cmd, vals)
		if more {
			a.logActivity(db, router.AccountID, "router", "Import depuis «"+router.Name+"» : "+summary+" (suite programmée…)")
		} else {
			a.logActivity(db, router.AccountID, "router", "Import depuis «"+router.Name+"» terminé : "+summary)
		}
	case cmd.Kind == model.CmdPoolDoctor && ok:
		// N°97 — docteur pool IP : le rapport pose PoolCap/PoolRanges/
		// PoolHosts/PoolDoctorAt (vérité routeur). Un échec est silencieux
		// côté file : l'auto-diagnostiqueur du check-in re-file au cycle
		// suivant tant que PoolCap reste nul.
		summary := a.applyPoolDoctor(db, router, vals)
		a.logActivity(db, router.AccountID, "router", "Routeur «"+router.Name+"» — "+summary)
	case cmd.Kind == model.CmdReadDhcp && ok:
		// N°101 — inventaire HomeNet : le rapport des bails DHCP nourrit le
		// registre des appareils UNIQUEMENT pour un compte homenet (le clic
		// DHCP de la console hotspot reste du cache outil F9 : aucune ligne
		// d'appareil ne naît d'un parc public). La commande reste « done »
		// avec son Result["data"] : l'outil F9 y relit son cache 120 s —
		// les DEUX consommateurs du même rapport, zéro conflit.
		if accountUsageLocked(db, router.AccountID) == model.AccountUsageHomeNet {
			created := a.applyDeviceLeases(db, router, vals)
			if a.devicesDone == nil {
				a.devicesDone = map[string]time.Time{}
			}
			a.devicesDone[router.ID] = time.Now().UTC()
			if created > 0 {
				a.logActivity(db, router.AccountID, "router", strconv.Itoa(created)+" appareil(s) découvert(s) sur «"+router.Name+"» (bail DHCP)")
			}
		}
	case cmd.Kind == model.CmdDevicePause && ok:
		// N°101 — pause dîner : la signature n'est posée que si le COMPTE de
		// règles marquées rapporté correspond à l'ensemble envoyé ET si la
		// version envoyée est toujours désirée (vérité routeur + anti-
		// péremption, pattern SafeWiFi N°80/N°93). Un échec reste sans
		// signature : le convergeur du check-in re-file tant que ça diverge.
		if applied := a.applyDevicePauseResult(db, router, cmd, vals); applied > 0 {
			a.logActivity(db, router.AccountID, "router", "Pause d'appareils appliquée sur «"+router.Name+"» ("+strconv.Itoa(applied)+" appareil(s) coupé(s))")
		}
	case ok:
		if cmd.Kind == model.CmdWalledGarden {
			// N°29 — configuration appliquée et CONFIRMÉE par le routeur :
			// la signature est posée ici (et seulement ici) — un échec sera
			// retenté au check-in suivant, un changement de config re-file.
			// N°49 — l'horodatage accompagne la signature : il porte la
			// cadence d'auto-réparation (walledGardenFresh).
			if sig, _ := cmd.Payload["sig"].(string); sig != "" {
				router.WalledGardenSig = sig
			}
			router.WalledGardenAppliedAt = model.NowISO()
			a.logActivity(db, router.AccountID, "router", "Walled-garden d'inscription publique appliqué sur «"+router.Name+"»")
		} else if cmd.Kind == model.CmdHotspotFiles {
			// N°35 — portail captif déployé et CONFIRMÉ par le routeur :
			// la signature est posée ici (et seulement ici). Pattern
			// identique à walled_garden : un échec est retenté au check-in
			// suivant, un changement de template re-file automatiquement.
			if sig := hotspotFilesSigFromPayload(cmd.Payload); sig != "" {
				router.HotspotFilesSig = sig
			}
			files := agent.HotspotFilesFromPayload(cmd.Payload)
			a.logActivity(db, router.AccountID, "router", "Portail captif déployé sur «"+router.Name+"» ("+
				strconv.Itoa(len(files))+" fichier(s) — login.html, status.html, assets)")
		} else if cmd.Kind == model.CmdSchedulerSet {
			// N°75 — veille adaptative : le pas du scheduler est CONFIRMÉ
			// par le routeur (le RAPPORT échoe l'intervalle appliqué —
			// vérité routeur, pas le payload émis ; pattern walled-garden).
			// Un échec est retenté au check-in suivant.
			if iv, err := strconv.Atoi(vals.Get("intervalSec")); err == nil && iv > 0 {
				if iv < 45 {
					iv = 45
				}
				if iv > 900 {
					iv = 900
				}
				if iv != router.SchedulerSec {
					router.SchedulerSec = iv
					pas := "rapide (45 s)"
					if iv >= agentSleepSec {
						pas = "veille (" + strconv.Itoa(iv) + " s)"
					}
					a.logActivity(db, router.AccountID, "router", "Cadence agent de «"+router.Name+"» : mode "+pas)
				}
			}
		} else if cmd.Kind == model.CmdWatcherEnsure {
			// N°77 — veilleur déployé et CONFIRMÉ par le routeur :
			// le drapeau n'est posé qu'ici (pattern walled-garden /
			// scheduler_set — vérité routeur uniquement). Un échec
			// reste WatcherOK=false → re-file au check-in suivant.
			if !router.WatcherOK {
				router.WatcherOK = true
				a.logActivity(db, router.AccountID, "router", "Veilleur d'invités déployé sur «"+router.Name+"» — claim du portail servi en ≤ 20 s")
			}
		} else if cmd.Kind == model.CmdSafeWifi {
			// N°80 — protection appliquée et CONFIRMÉE par le
			// routeur : la signature n'est posée que si le COMPTE
			// d'objets marqués rapporté correspond au niveau
			// attendu (N°93 : 4 règles NAT — 2 dst-nat vers le
			// résolveur filtrant + 2 boucliers pré-auth vers le
			// servlet DNS natif — + les entrées de la liste DoH v4
			// + 2 règles FILTER par serveur hotspot rapporté — 0
			// sinon ; vérité routeur, pattern scheduler_set ET
			// pattern Shield N°81 : le compte dépend du parc réel
			// du routeur) ET, depuis le N°95, si la DISPOSITION des
			// règles NAT marquées rapportée est exacte : boucliers
			// pré-auth AU-DESSUS des dst-nat (SafeWifiNatLayout).
			// Le post-mortem CYBER-ESPACE SC du N°93 a prouvé qu'un
			// compte conforme peut cacher un ordre inversé —
			// boucliers sous les dst-nat — qui tue le portail captif
			// en silence. Et uniquement si le niveau rapporté
			// correspond TOUJOURS au niveau courant : un gérant
			// qui change d'avis pendant le vol ne doit pas voir
			// un niveau périmé figé — le check-in suivant re-file
			// la différence.
			level := agent.SafeWifiLevelFromPayload(cmd.Payload)
			want := 0
			wantLayout := ""
			if level != model.SafeWifiOff {
				if hs, ok := parseReportInt(vals.Get("hs")); ok {
					want = agent.SafeWifiRulesExpected(hs)
				} else {
					want = -1 // hs illisible : vérification impossible → pas de sig
				}
				wantLayout = agent.SafeWifiNatLayout
			}
			if got, ok := parseReportInt(vals.Get("rules")); ok && got == want && vals.Get("layout") == wantLayout {
				if level == router.SafeWifiLevelEffective() {
					if sig, _ := cmd.Payload["sig"].(string); sig != "" {
						router.SafeWifiSig = sig
					}
					router.SafeWifiAppliedAt = model.NowISO()
				}
				a.logActivity(db, router.AccountID, "router", "Protection WiFi public ("+safeWifiLevelLabel(level)+") appliquée sur «"+router.Name+"»")
			}
		} else if cmd.Kind == model.CmdShield {
			// N°81 — bouclier appliqué et CONFIRMÉ par le routeur : la
			// signature n'est posée que si le COMPTE de règles marquées
			// rapporté correspond à 5 règles × le nombre de serveurs
			// hotspots RAPPORTÉ (le script énumère les interfaces sur le
			// routeur — vérité routeur, pattern N°80), et uniquement si le
			// niveau rapporté est TOUJOURS courant.
			level := agent.ShieldLevelFromPayload(cmd.Payload)
			want := 0
			if level != model.ShieldOff {
				if hs, ok := parseReportInt(vals.Get("hs")); ok {
					want = shieldRulesPerHotspot * hs
				} else {
					want = -1 // hs illisible : vérification impossible → pas de sig
				}
			}
			if got, ok := parseReportInt(vals.Get("rules")); ok && got == want {
				if level == router.ShieldLevelEffective() {
					if sig, _ := cmd.Payload["sig"].(string); sig != "" {
						router.ShieldSig = sig
					}
					router.ShieldAppliedAt = model.NowISO()
				}
				a.logActivity(db, router.AccountID, "router", "Bouclier réseau ("+shieldLevelLabel(level)+") appliqué sur «"+router.Name+"»")
			}
		} else if cmd.Kind == model.CmdFamilyGuard {
			// N°82 — couvre-feu appliqué et CONFIRMÉ par le
			// routeur : la signature n'est posée que si le COMPTE
			// de règles marquées rapporté correspond à 1 règle ×
			// le nombre de serveurs hotspots RAPPORTÉ (en
			// couvre-feu actif), ou 0 (levé), ET uniquement si le
			// spec rapporté est TOUJOURS celui du routeur et si
			// l'ÉTAT désiré est toujours courant — une frontière
			// de fenêtre franchie pendant le vol ne fige pas un
			// état périmé : le check-in suivant re-file la
			// bascule (pattern « niveau toujours courant » N°80).
			spec, _ := cmd.Payload["spec"].(string)
			active := agent.FamilyGuardActiveFromPayload(cmd.Payload)
			want := 0
			if active {
				if hs, ok := parseReportInt(vals.Get("hs")); ok {
					want = familyGuardRulesPerHotspot * hs
				} else {
					want = -1 // hs illisible : vérification impossible → pas de sig
				}
			}
			if got, ok := parseReportInt(vals.Get("rules")); ok && got == want {
				if cur, okCfg := model.ParseFamilyGuardSpec(router.FamilyGuardSpec); okCfg &&
					router.FamilyGuardSpec == spec && cur.ActiveAt(time.Now().UTC()) == active {
					if sig, _ := cmd.Payload["sig"].(string); sig != "" {
						router.FamilyGuardSig = sig
					}
					router.FamilyGuardAppliedAt = model.NowISO()
				}
				label := "levé"
				if active {
					label = "actif — internet coupé"
				}
				a.logActivity(db, router.AccountID, "router", "Couvre-feu internet ("+label+") appliqué sur «"+router.Name+"»")
			}
		} else if cmd.Kind == model.CmdAntiVpn {
			// N°88 — bloque-VPN appliqué et CONFIRMÉ par le routeur : la
			// signature n'est posée que si le COMPTE de règles marquées
			// rapporté correspond à 4 règles × le nombre de serveurs
			// hotspots RAPPORTÉ (0 si off), et uniquement si le niveau
			// rapporté est TOUJOURS courant (pattern N°80/81 : un gérant
			// qui change d'avis pendant le vol ne doit pas voir un état
			// périmé figé).
			level := agent.AntiVpnLevelFromPayload(cmd.Payload)
			want := 0
			if level != model.AntiVpnOff {
				if hs, ok := parseReportInt(vals.Get("hs")); ok {
					want = agent.AntiVpnRulesPerHotspot * hs
				} else {
					want = -1 // hs illisible : vérification impossible → pas de sig
				}
			}
			if got, ok := parseReportInt(vals.Get("rules")); ok && got == want {
				if level == router.AntiVpnLevelEffective() {
					if sig, _ := cmd.Payload["sig"].(string); sig != "" {
						router.AntiVpnSig = sig
					}
					router.AntiVpnAppliedAt = model.NowISO()
				}
				a.logActivity(db, router.AccountID, "router", "Bloque-VPN ("+antiVpnLevelLabel(level)+") appliqué sur «"+router.Name+"»")
			}
		} else if cmd.Kind == model.CmdQueueEnsure {
			// N°104 — QoS appliquée et CONFIRMÉE par le routeur : la
			// signature n'est posée que si la RELECTURE (target,
			// max-limit, types, disabled) correspond bit à bit au
			// payload — chaque côté normalise en bps, le formatage
			// RouterOS (« 17M » vs « 17000000 ») ne compte plus — et
			// uniquement si l'état désiré est TOUJOURS courant (un
			// gérant qui change les limites pendant le vol ne doit
			// pas voir figé un état périmé, pattern N°80).
			if qosEnsureVerified(cmd, vals.Get("data")) {
				if sig, _ := cmd.Payload["sig"].(string); sig != "" && router.QoSEnabled && sig == qosSig(router) {
					prev := router.QoSSig
					router.QoSSig = sig
					router.QoSAppliedAt = model.NowISO()
					// Rattachement des profils (files dynamiques
					// filles du plafond) : uniquement à la
					// PREMIÈRE application d'une config — la file
					// existe déjà (retour vérifié), l'ordre est
					// garanti par la causalité.
					if prev != sig {
						a.qosAttachProfilesLocked(db, router, agent.QoSQueueName)
					}
				}
				a.logActivity(db, router.AccountID, "router", "QoS appliquée sur «"+router.Name+"» — file "+agent.QoSQueueName+" vérifiée (plafond agrégat du hotspot)")
			}
		} else if cmd.Kind == model.CmdQueueRemove {
			// N°104 — retrait confirmé : le rapport échoe le nombre de
			// files mikcloud-qos RESTANTES (0 attendu — vérité routeur,
			// pas la foi en un remove silencieux). Le marqueur
			// QoSAppliedAt est levé : plus rien à retirer.
			for _, e := range splitAgentList(vals.Get("data")) {
				if len(e) >= 2 && e[0] == "removed" {
					if left, okL := parseReportInt(e[1]); okL && left == 0 {
						router.QoSAppliedAt = ""
						router.QoSSig = ""
						a.logActivity(db, router.AccountID, "router", "QoS retirée de «"+router.Name+"» — file "+agent.QoSQueueName+" supprimée")
					}
					break
				}
			}
		} else if cmd.Kind == model.CmdQueueRead {
			// N°104 — monitoring : une file manquante ou divergente
			// (limites retouchées en Winbox, file désactivée) vide la
			// signature — le check-in suivant re-file queue_ensure
			// (auto-réparation, pattern N°49). Le rapport alimente AUSSI
			// le cache de la carte (Result["data"], mécanique outils F9).
			if router.QoSEnabled {
				matching := false
				for _, row := range parseQueueRows(splitAgentList(vals.Get("data"))) {
					if qosRowMatches(row, router.QoSTarget, router.QoSMaxUpBps, router.QoSMaxDownBps) {
						matching = true
					}
				}
				if !matching && router.QoSSig != "" {
					router.QoSSig = ""
					a.logActivity(db, router.AccountID, "router", "QoS divergente sur «"+router.Name+"» (file absente ou modifiée localement) — re-application automatique au prochain check-in")
				}
			}
		} else {
			a.logActivity(db, router.AccountID, "router", "Commande "+cmd.Kind+" exécutée sur «"+router.Name+"»")
		}
		// N°76 — fraîcheur post-écriture : via la garde de cycle (un
		// read_state paginé en cours est DÉJÀ la synchronisation — le
		// casser désordonnerait l'accumulateur ; cf.
		// queueReadStateFreshLocked).
		queueReadStateFreshLocked(db, router)
	default:
		a.logActivity(db, router.AccountID, "router", "Commande "+cmd.Kind+" ÉCHOUÉE sur «"+router.Name+"» ("+vals.Get("message")+")")
	}
	purgeOldCommands(db)
	a.store.Save()
	a.store.Unlock()

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
