// handlers_user_detail.go — N°197 — carte « détails de connexion » du voucher
// ou de l'utilisateur régulier derrière une session active de /app/sessions :
// le bouton d'action par ligne ouvre une carte qui montre la DERNIÈRE
// connexion, le TOTAL des données consommées par le ticket/l'utilisateur
// (miroir cloud du limit-bytes-total routeur), l'adresse MAC de l'appareil
// et sa MARQUE PROBABLE (préfixe OUI IEEE), plus l'historique récent (F3)
// et la traçabilité de vente quand le ticket vient d'un revendeur.
//
// GET /api/users/{id}/connection-detail?routerId=… — gérant/propriétaire
// (rang 2). {id} = ID du HotspotUser OU username exact : le repli par nom
// couvre les sessions des routeurs RÉELS dont l'utilisateur a été créé
// directement dans Winbox (userId vide côté session, utilisateur absent du
// registre cloud — la carte affiche alors la connexion et le journal, sans
// la section ticket : elle est honnête sur ce qu'elle ne sait pas).
// routerId (optionnel, celui de la ligne de session cliquée) départage les
// homonymes : un username peut vivre sur plusieurs routeurs du compte.
package api

import (
	"net/http"
	"sort"
	"strings"
	"time"

	"mikcloud/hotspot-api/internal/model"
)

// recentLogsLimit — nombre d'événements du journal servis dans la carte
// (au-delà, la vue Journal reste l'outil de référence).
const recentLogsLimit = 10

// loginWindowDays — fenêtre du compteur « connexions » de la carte.
const loginWindowDays = 30

// userConnectionDetail — réponse agrégée de la carte : TOUT ce que le cloud
// sait de la connexion d'un utilisateur hotspot, calculé en un seul appel
// (le dialogue n'a plus qu'à afficher).
type userConnectionDetail struct {
	// User — le ticket/utilisateur du registre cloud, statut RÉSOLU à jour
	// (online/used/active/expired/disabled), mot de passe JAMAIS servi (la
	// carte n'en a pas besoin — c'est LE secret d'un compte régulier).
	User *model.HotspotUser `json:"user"`
	// LiveSessions — sessions actives à l'instant (miroir de GET /api/sessions :
	// store + interrogation des routeurs réels ; un profil shared-users > 1
	// peut en porter plusieurs).
	LiveSessions []model.Session `json:"liveSessions"`
	// LastLoginAt — dernière connexion : session live la plus récente, sinon
	// dernier log login, sinon première connexion (UsedAt) ; "" si inconnue.
	LastLoginAt string `json:"lastLoginAt"`
	// LoginCount30d — logins observés sur les 30 derniers jours (F3).
	LoginCount30d int `json:"loginCount30d"`
	// RecentLogs — derniers événements du journal utilisateur (F3), récents d'abord.
	RecentLogs []model.UserLog `json:"recentLogs"`
	// MAC — meilleure adresse MAC connue : session live la plus récente,
	// sinon dernier log qui en porte une ; "" quand le routeur ne la
	// rapporte pas (script agent antérieur au N°197).
	MAC string `json:"mac"`
	// DeviceBrand — marque PROBABLE de l'appareil (préfixe OUI IEEE) ; ""
	// si la MAC est inconnue ou hors table.
	DeviceBrand string `json:"deviceBrand"`
}

// handleUserConnectionDetail — agrégat serveur de la carte (lecture seule :
// aucun Tick — l'état servi est exactement celui du dernier poll de la vue
// Sessions, jamais une mutation déguisée en GET).
func (a *API) handleUserConnectionDetail(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	key := strings.TrimSpace(r.PathValue("id"))
	if key == "" {
		writeErr(w, http.StatusBadRequest, "Identifiant manquant")
		return
	}
	routerHint := strings.TrimSpace(r.URL.Query().Get("routerId"))
	now := time.Now().UTC()
	windowStart := now.AddDate(0, 0, -loginWindowDays).Format(time.RFC3339)

	// — Résolution et collecte sous verrou —
	a.store.Lock()
	db := a.store.Data()
	username := key // repli honnête : hors registre cloud tant que rien ne prouve le contraire
	var user *model.HotspotUser
	for i := range db.HotspotUsers {
		u := &db.HotspotUsers[i]
		if u.AccountID == acc && u.ID == key {
			user = u
			break
		}
	}
	if user == nil {
		// Par username (insensible à la casse, miroir des rapports agent) :
		// le hint routeur de la ligne cliquée départage les homonymes ;
		// à défaut, le plus récemment actif gagne (dernier log, sinon
		// dernière utilisation).
		lastSeen := map[string]string{}
		for _, l := range db.UserLogs {
			if l.AccountID == acc && l.UserID != "" && l.At > lastSeen[l.UserID] {
				lastSeen[l.UserID] = l.At
			}
		}
		var best, fallback *model.HotspotUser
		bestRank := ""
		for i := range db.HotspotUsers {
			u := &db.HotspotUsers[i]
			if u.AccountID != acc || !strings.EqualFold(u.Username, key) {
				continue
			}
			if routerHint != "" && u.RouterID == routerHint {
				if best == nil || u.UsedAt > best.UsedAt {
					best = u
				}
				continue
			}
			rank := lastSeen[u.ID]
			if u.UsedAt > rank {
				rank = u.UsedAt
			}
			if fallback == nil || rank > bestRank {
				fallback, bestRank = u, rank
			}
		}
		if best == nil {
			best = fallback
		}
		user = best
	}

	var userOut, userRaw *model.HotspotUser
	var userID, userRouterID string
	if user != nil {
		userID = user.ID
		username = user.Username
		userRouterID = user.RouterID
		raw := *user
		userRaw = &raw
		// Copie servie : mot de passe vide (LE secret d'un compte régulier —
		// la carte n'en a jamais besoin).
		copyU := raw
		copyU.Password = ""
		userOut = &copyU
	}
	// Le rattachement par nom des sessions/logs au userId vide se borne au
	// routeur de l'utilisateur résolu (homonymes d'un autre point d'accès
	// exclus) ; Winbox-only : le hint routeur de la ligne cliquée tient lieu.
	scopeRouter := userRouterID
	if scopeRouter == "" {
		scopeRouter = routerHint
	}

	// Sessions live du store.
	live := []model.Session{}
	for _, s := range db.Sessions {
		if s.AccountID != acc || !sessionBelongsTo(s, userID, username, scopeRouter) {
			continue
		}
		live = append(live, s)
	}

	// Journal (F3) : par ID quand connu, ET par nom borné au routeur — les
	// logs d'une session au userId vide (routeur réel, import postérieur)
	// restent rattachés au bon utilisateur.
	logs := []model.UserLog{}
	for _, l := range db.UserLogs {
		if l.AccountID != acc || !logBelongsToUser(l, userID, username, scopeRouter) {
			continue
		}
		logs = append(logs, l)
	}
	sort.Slice(logs, func(i, j int) bool { return logs[i].At > logs[j].At })

	// Routeurs réels du compte : interrogation APRÈS libération du verrou
	// (ListSessions prend le sien — miroir de handleSessionsList).
	realRouters := []model.Router{}
	for _, rr := range db.Routers {
		if rr.Mode == "real" && rr.AccountID == acc {
			realRouters = append(realRouters, rr)
		}
	}
	a.store.Unlock()

	for _, rr := range realRouters {
		gw := a.gatewayFor(rr)
		sess, err := gw.ListSessions()
		if err != nil {
			continue
		}
		for _, s := range sess {
			if sessionBelongsTo(s, userID, username, scopeRouter) {
				live = append(live, s)
			}
		}
	}
	sort.Slice(live, func(i, j int) bool { return live[i].StartedAt > live[j].StartedAt })

	if userOut == nil && len(live) == 0 && len(logs) == 0 {
		// Rien dans ce compte : utilisateur inconnu (l'isolation multi-tenant
		// prime — aucun indice sur son existence ailleurs).
		writeErr(w, http.StatusNotFound, "Utilisateur introuvable")
		return
	}

	// Statut RÉSOLU, sessions réelles fusionnées comprises (un Winbox-only
	// connecté n'a pas de section ticket mais sa carte reste cohérente).
	if userOut != nil {
		userOut.Status = model.ResolvedStatus(userRaw, len(live) > 0, now)
	}

	// Dernière connexion : session live la plus récente > dernier login
	// journalisé > première utilisation du ticket.
	lastLogin := ""
	for _, s := range live {
		if s.StartedAt > lastLogin {
			lastLogin = s.StartedAt
		}
	}
	loginCount := 0
	for _, l := range logs {
		if l.Action == "login" {
			if l.At > lastLogin {
				lastLogin = l.At
			}
			if l.At >= windowStart {
				loginCount++
			}
		}
	}
	if userOut != nil && userOut.UsedAt > lastLogin {
		lastLogin = userOut.UsedAt
	}
	if len(logs) > recentLogsLimit {
		logs = logs[:recentLogsLimit]
	}

	// Meilleure MAC : session live la plus récente qui en porte une, sinon
	// le dernier log qui en porte une (sessions passées d'un routeur réel).
	mac := ""
	for _, s := range live {
		if s.MAC != "" {
			mac = s.MAC
			break
		}
	}
	if mac == "" {
		for _, l := range logs {
			if l.MAC != "" {
				mac = l.MAC
				break
			}
		}
	}
	if mac != "" {
		if n := model.NormalizeMAC(mac); n != "" {
			mac = n
		}
	}

	writeJSONCacheable(w, r, http.StatusOK, userConnectionDetail{
		User:          userOut,
		LiveSessions:  live,
		LastLoginAt:   lastLogin,
		LoginCount30d: loginCount,
		RecentLogs:    logs,
		MAC:           mac,
		DeviceBrand:   macVendor(mac),
	})
}

// sessionBelongsTo — la session appartient-elle à l'utilisateur résolu ?
// Par ID quand la session le porte (agent/simulé, user connu du cloud) ;
// sinon par username insensible à la casse BORNÉ au routeur de portée (le
// homonyme d'un autre point d'accès reste à son propriétaire). Un userId et
// un username vides ne font JAMAIS matcher.
func sessionBelongsTo(s model.Session, userID, username, scopeRouter string) bool {
	if userID != "" && s.UserID == userID {
		return true
	}
	if username == "" || s.Username == "" || !strings.EqualFold(s.Username, username) {
		return false
	}
	return scopeRouter == "" || s.RouterID == scopeRouter
}

// logBelongsToUser — le log (F3) se rattache-t-il à l'utilisateur ? Par ID
// ET par nom borné au routeur : les sessions d'un routeur réel peuvent
// porter un userId vide (utilisateur créé dans Winbox puis importé — le log
// écrit AVANT l'import ne connaissait que le nom).
func logBelongsToUser(l model.UserLog, userID, username, scopeRouter string) bool {
	if userID != "" && l.UserID == userID {
		return true
	}
	if username == "" || l.Username == "" || !strings.EqualFold(l.Username, username) {
		return false
	}
	return scopeRouter == "" || l.RouterID == scopeRouter
}
