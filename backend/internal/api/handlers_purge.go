// Package api — purge des données par catégories (Paramètres plateforme →
// « Purge des données »).
//
// FUSION (anti-redondance) : la purge GLOBALE et la purge CIBLÉE par compte
// partagent UN SEUL moteur (purgeScopes) et UN SEUL endpoint d'exécution
// (POST /api/admin/purge, accountId optionnel). Portée = accountId vide
// (tous les comptes) ou un compte précis (les autres ne sont jamais touchés).
//
// Historique : l'ancien POST /api/admin/reset régénérait le seed de
// démonstration (BuildSeed) — c'était la cause du RETOUR des données de test
// et de la disparition des routeurs réels en production. Il a été supprimé :
// aucun endpoint ne régénère de données démo. La purge, elle, ne touche JAMAIS
// les routeurs réels (mode agent/real), ni les comptes, l'équipe, les réglages
// et l'abonnement — elle ne régénère rien non plus.
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"mikcloud/hotspot-api/internal/agent"
	"mikcloud/hotspot-api/internal/model"
)

// Scopes de purge (identifiants stables consommés par le frontend).
// La grille est UNIFIÉE : les 10 catégories sont disponibles dans les deux
// portées (globale et ciblée par compte).
const (
	PurgeScopeSimulatedRouters = "simulated_routers" // + cascade : utilisateurs, sessions, trafic, commandes, bindings, schedulers, lots, ventes
	PurgeScopeVouchers         = "vouchers"          // tickets seuls (kind=voucher) — les LOTS sont conservés
	PurgeScopeHotspotUsers     = "hotspot_users"     // comptes client hotspot restants (hors tickets)
	PurgeScopeProfiles         = "profiles"          // profils / forfaits
	PurgeScopeBatches          = "batches"           // lots de vouchers + leurs vouchers restants
	PurgeScopeResellers        = "resellers"         // revendeurs + transactions
	PurgeScopeSales            = "sales"             // ventes
	PurgeScopeSessions         = "sessions"          // sessions actives restantes
	PurgeScopeLogs             = "logs"              // journaux : utilisateurs, activité, notifications, commandes agent
	PurgeScopeTemplates        = "templates"         // gabarits de tickets
	PurgeScopeAll              = "all"               // toutes les catégories ci-dessus
)

// purgeScopeSet — les 10 catégories purgables (« all » mis à part).
var purgeScopeSet = []string{
	PurgeScopeSimulatedRouters, PurgeScopeVouchers, PurgeScopeHotspotUsers,
	PurgeScopeProfiles, PurgeScopeBatches, PurgeScopeResellers, PurgeScopeSales,
	PurgeScopeSessions, PurgeScopeLogs, PurgeScopeTemplates,
}

// PurgeStats — compteurs LIVE affichés dans l'UI (GET /api/admin/purge/stats,
// portée globale — tous les comptes confondus). Les utilisateurs/sessions
// attachés à des routeurs simulés ne sont PAS comptés dans leurs catégories :
// ils partent avec le routeur (cascade).
type PurgeStats struct {
	SimulatedRouters int `json:"simulatedRouters"`
	Vouchers         int `json:"vouchers"` // tickets (kind=voucher)
	HotspotUsers     int `json:"hotspotUsers"`
	Profiles         int `json:"profiles"`
	Batches          int `json:"batches"`
	Resellers        int `json:"resellers"`
	Transactions     int `json:"transactions"`
	Sales            int `json:"sales"`
	Sessions         int `json:"sessions"`
	Logs             int `json:"logs"`
	Templates        int `json:"templates"`
	// RealRouters — routeurs réels (agent/real) : informatif, JAMAIS purgés.
	RealRouters int `json:"realRouters"`
}

// PurgeCounts — quantités réellement supprimées (réponse de POST /api/admin/purge).
type PurgeCounts struct {
	Routers      int `json:"routers"`
	HotspotUsers int `json:"hotspotUsers"`
	Vouchers     int `json:"vouchers"` // purge ciblée uniquement (tickets kind=voucher)
	Profiles     int `json:"profiles"`
	Batches      int `json:"batches"`
	Resellers    int `json:"resellers"`
	Transactions int `json:"transactions"`
	Sales        int `json:"sales"`
	Sessions     int `json:"sessions"`
	Logs         int `json:"logs"`
	Templates    int `json:"templates"`
	// Audit purge/résurgence :
	Tombstones     int `json:"tombstones"`     // marqueurs anti-ré-import posés (TTL 30 j)
	RouterRemovals int `json:"routerRemovals"` // comptes commandés en suppression SUR LES ROUTEURS (P2, opt-in)
}

// handlePurgeStats — GET /api/admin/purge/stats : compteurs par catégorie.
func (a *API) handlePurgeStats(w http.ResponseWriter, r *http.Request) {
	if !isPlatformAdmin(r) {
		writeErr(w, http.StatusForbidden, "Réservé aux administrateurs de la plateforme")
		return
	}
	a.store.Lock()
	stats := computePurgeStats(a.store.Data())
	a.store.Unlock()
	writeJSON(w, http.StatusOK, stats)
}

// computePurgeStats — compteurs de l'état courant (à appeler sous verrou).
func computePurgeStats(db *model.DB) PurgeStats {
	stats := PurgeStats{}
	simIDs := map[string]bool{}
	for _, rr := range db.Routers {
		if rr.Mode == "simulated" {
			simIDs[rr.ID] = true
			stats.SimulatedRouters++
		} else {
			stats.RealRouters++
		}
	}
	// Les utilisateurs/sessions des routeurs simulés partent avec eux
	// (cascade) — seuls les RESTANTS sont proposés dans leurs catégories.
	// Tickets (kind=voucher) et comptes client sont comptés séparément :
	// grille unifiée avec la purge ciblée par compte.
	for _, u := range db.HotspotUsers {
		if simIDs[u.RouterID] {
			continue
		}
		if u.Kind == "voucher" {
			stats.Vouchers++
		} else {
			stats.HotspotUsers++
		}
	}
	for _, s := range db.Sessions {
		if simIDs[s.RouterID] {
			continue
		}
		stats.Sessions++
	}
	stats.Profiles = len(db.Profiles)
	stats.Batches = len(db.Batches)
	stats.Resellers = len(db.Resellers)
	stats.Transactions = len(db.Transactions)
	stats.Sales = len(db.Sales)
	stats.Logs = len(db.UserLogs) + len(db.Activity) + len(db.NotifLog) + len(db.Commands)
	stats.Templates = len(db.Templates)
	return stats
}

// handlePurge — POST /api/admin/purge : purge par catégories, portée UNIFIÉE.
// Corps : {"scopes": ["simulated_routers", "hotspot_users", …],
//
//	       "accountId": "acc-…"  (OPTIONNEL)}
//	- accountId ABSENT/VIDE → purge GLOBALE : les catégories sont supprimées
//	  sur TOUS les comptes (comportement historique) ;
//	- accountId RENSEIGNÉ → purge CIBLÉE : seules les données de CE compte sont
//	  supprimées (les autres ne sont jamais touchés ; 404 si inconnu).
//
// (le scope « all » sélectionne les 10 catégories ; {"scope":"all"} accepté ;
// l'ancien endpoint POST /api/admin/purge/account est FUSIONNÉ ici).
func (a *API) handlePurge(w http.ResponseWriter, r *http.Request) {
	var body struct {
		AccountID string   `json:"accountId"`
		Scopes    []string `json:"scopes"`
		Scope     string   `json:"scope"` // compat mono-scope
		// Purge TOTALE (opt-in, P2) : supprimer aussi les comptes SUR LES
		// ROUTEURS réels (commandes user_remove à l'agent). Exige confirm ==
		// "SUPPRIMER" — double garde (client + serveur) : action irréversible
		// qui déconnecte immédiatement les clients concernés.
		AlsoRouter bool   `json:"alsoRouter"`
		Confirm    string `json:"confirm"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	if body.AlsoRouter && strings.TrimSpace(body.Confirm) != "SUPPRIMER" {
		writeErr(w, http.StatusBadRequest, "Purge totale : confirmation « SUPPRIMER » requise")
		return
	}
	accID := strings.TrimSpace(body.AccountID)
	scopes := body.Scopes
	if len(scopes) == 0 && strings.TrimSpace(body.Scope) != "" {
		scopes = []string{body.Scope}
	}
	// Normalisation + validation (défensif : doublons, casse, inconnus).
	seen := map[string]bool{}
	normalized := make([]string, 0, len(scopes))
	for _, s := range scopes {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if s == PurgeScopeAll {
			a.purgeScopes(w, r, accID, nil, body.AlsoRouter) // nil → « all »
			return
		}
		valid := false
		for _, known := range purgeScopeSet {
			if s == known {
				valid = true
				break
			}
		}
		if !valid {
			writeErr(w, http.StatusBadRequest, "Catégorie de purge inconnue : "+s)
			return
		}
		if !seen[s] {
			seen[s] = true
			normalized = append(normalized, s)
		}
	}
	if len(normalized) == 0 {
		// Purge destructive : exiger une sélection EXPLICITE (ou « all ») —
		// plus sûr qu'un scopes absent qui voudrait dire « tout ».
		writeErr(w, http.StatusBadRequest, "Aucune catégorie sélectionnée")
		return
	}
	a.purgeScopes(w, r, accID, normalized, body.AlsoRouter)
}

// purgeScopes — moteur UNIQUE de la purge (handlePurge + handleWipe).
// accID == "" → portée GLOBALE (tous les comptes) ; accID != "" → portée
// CIBLÉE, restreinte au compte (404 si inconnu). scopes nil ou vide → « all ».
// Sous verrou global ; ne touche JAMAIS les routeurs réels (agent/real), les
// comptes, l'équipe, les réglages ni l'abonnement ; ne régénère RIEN. Les
// slices sont réaffectées vides (non-nil) pour que la synchro différentielle
// PostgreSQL supprime les lignes correspondantes.
//
// Audit purge/résurgence : chaque username purgé reçoit un TOMBSTONE (TTL 30
// jours) qui bloque son ré-import par la synchro agent (applyReadState) —
// sinon le routeur réel, qui garde ses /ip hotspot user, les ferait
// ressusciter à la première synchro. La file de commandes qui recréerait les
// entités purgées est annulée (cancelQueuedForPurged). alsoRouter (opt-in,
// exige la confirmation « SUPPRIMER » côté handlePurge) enfile en plus des
// commandes user_remove pour les routeurs AGENT : purge TOTALE.
func (a *API) purgeScopes(w http.ResponseWriter, r *http.Request, accID string, scopes []string, alsoRouter bool) {
	if !isPlatformAdmin(r) {
		writeErr(w, http.StatusForbidden, "Réservé aux administrateurs de la plateforme")
		return
	}
	all := len(scopes) == 0
	has := func(scope string) bool {
		if all {
			return true
		}
		for _, s := range scopes {
			if s == scope {
				return true
			}
		}
		return false
	}
	global := accID == ""
	// match — filtre de portée : TOUTES les entités (globale) ou uniquement
	// celles du compte ciblé (les identifiants de compte sont un filtre, pas
	// une suppression : le compte lui-même et son équipe ne sont jamais purgés).
	match := func(acc string) bool {
		return global || acc == accID
	}

	a.store.Lock()
	db := a.store.Data()
	var counts PurgeCounts

	// Audit purge/résurgence — collecte des usernames purgés :
	//   - purgedUsers : (compte → usernames lower) → tombstones anti-ré-import ;
	//   - purgedAgent : (routeur agent → usernames d'origine) → commandes
	//     user_remove de la purge TOTALE (alsoRouter). Un routeur ABSENT de
	//     db.Routers au moment de la note (simulé, déjà retiré par le scope 1)
	//     ne peut pas être commandé — il disparaît avec ses comptes.
	purgedUsers := map[string]map[string]bool{}
	purgedAgent := map[string][]string{}
	noteUser := func(u model.HotspotUser) {
		name := agent.SanitizeName(u.Username)
		if name == "" {
			return
		}
		m := purgedUsers[u.AccountID]
		if m == nil {
			m = map[string]bool{}
			purgedUsers[u.AccountID] = m
		}
		m[strings.ToLower(name)] = true
		for i := range db.Routers {
			if db.Routers[i].ID == u.RouterID {
				if db.Routers[i].Mode == "agent" {
					purgedAgent[u.RouterID] = append(purgedAgent[u.RouterID], name)
				}
				break
			}
		}
	}

	// Compte cible doit exister en portée ciblée (nom repris dans le bilan).
	accName := ""
	if !global {
		for i := range db.Accounts {
			if db.Accounts[i].ID == accID {
				accName = db.Accounts[i].Name
				break
			}
		}
		if accName == "" {
			a.store.Unlock()
			writeErr(w, http.StatusNotFound, "Compte introuvable")
			return
		}
	}

	// 1. Routeurs SIMULÉS + cascade complète (utilisateurs, sessions, trafic,
	//    commandes, IP bindings, schedulers, lots, ventes attachés). Les
	//    routeurs réels (agent/real) sont systématiquement préservés.
	if has(PurgeScopeSimulatedRouters) {
		simIDs := map[string]bool{}
		kept := db.Routers[:0]
		for _, rr := range db.Routers {
			if rr.Mode == "simulated" && match(rr.AccountID) {
				simIDs[rr.ID] = true
				counts.Routers++
				continue
			}
			kept = append(kept, rr)
		}
		db.Routers = kept
		if len(simIDs) > 0 {
			drop := func(id string) bool { return simIDs[id] }
			// Utilisateurs/tickets du routeur : comptés dans leur catégorie
			// (tickets séparés des comptes client — grille unifiée).
			keptUsers := db.HotspotUsers[:0]
			for _, u := range db.HotspotUsers {
				if drop(u.RouterID) {
					noteUser(u)
					if u.Kind == "voucher" {
						counts.Vouchers++
					} else {
						counts.HotspotUsers++
					}
					continue
				}
				keptUsers = append(keptUsers, u)
			}
			db.HotspotUsers = keptUsers
			db.Sessions = dropByRouterID(db.Sessions, drop, func(s model.Session) string { return s.RouterID }, &counts.Sessions)
			db.Traffic = dropByRouterID(db.Traffic, drop, func(t model.RouterTraffic) string { return t.RouterID }, nil)
			db.Commands = dropByRouterID(db.Commands, drop, func(c model.Command) string { return c.RouterID }, nil)
			db.IPBindings = dropByRouterID(db.IPBindings, drop, func(b model.IPBinding) string { return b.RouterID }, nil)
			db.SchedulerTasks = dropByRouterID(db.SchedulerTasks, drop, func(t model.SchedulerTask) string { return t.RouterID }, nil)
			db.Batches = dropByRouterID(db.Batches, drop, func(b model.Batch) string { return b.RouterID }, &counts.Batches)
			db.Sales = dropByRouterID(db.Sales, drop, func(s model.Sale) string { return s.RouterID }, &counts.Sales)
		}
	}

	// 2. Tickets RESTANTS (kind=voucher) — les LOTS restent. Les sessions
	//    liées aux tickets supprimés sont closes (une session sans
	//    utilisateur n'a plus de sens).
	if has(PurgeScopeVouchers) {
		dropUser := map[string]bool{}
		keptUsers := make([]model.HotspotUser, 0, len(db.HotspotUsers))
		for _, u := range db.HotspotUsers {
			if match(u.AccountID) && u.Kind == "voucher" {
				dropUser[u.ID] = true
				noteUser(u)
				counts.Vouchers++
				continue
			}
			keptUsers = append(keptUsers, u)
		}
		db.HotspotUsers = keptUsers
		if len(dropUser) > 0 {
			keptSessions := db.Sessions[:0]
			for _, s := range db.Sessions {
				if dropUser[s.UserID] {
					counts.Sessions++
					continue
				}
				keptSessions = append(keptSessions, s)
			}
			db.Sessions = keptSessions
		}
	}

	// 3. Comptes client hotspot RESTANTS (hors tickets) — les sessions qui
	//    leur sont liées sont closes avec eux.
	if has(PurgeScopeHotspotUsers) {
		dropUser := map[string]bool{}
		keptUsers := make([]model.HotspotUser, 0, len(db.HotspotUsers))
		for _, u := range db.HotspotUsers {
			if match(u.AccountID) && u.Kind != "voucher" {
				dropUser[u.ID] = true
				noteUser(u)
				counts.HotspotUsers++
				continue
			}
			keptUsers = append(keptUsers, u)
		}
		db.HotspotUsers = keptUsers
		if len(dropUser) > 0 {
			keptSessions := db.Sessions[:0]
			for _, s := range db.Sessions {
				if dropUser[s.UserID] {
					counts.Sessions++
					continue
				}
				keptSessions = append(keptSessions, s)
			}
			db.Sessions = keptSessions
		}
	}

	// 4. Profils / forfaits.
	if has(PurgeScopeProfiles) {
		kept := db.Profiles[:0]
		for _, p := range db.Profiles {
			if match(p.AccountID) {
				counts.Profiles++
				continue
			}
			kept = append(kept, p)
		}
		db.Profiles = kept
	}

	// 5. Lots de vouchers + leurs vouchers RESTANTS (cascade — les tickets
	//    déjà retirés par le scope « vouchers » ne comptent pas deux fois).
	if has(PurgeScopeBatches) {
		batchIDs := map[string]bool{}
		keptBatches := db.Batches[:0]
		for _, b := range db.Batches {
			if match(b.AccountID) {
				batchIDs[b.ID] = true
				counts.Batches++
				continue
			}
			keptBatches = append(keptBatches, b)
		}
		db.Batches = keptBatches
		if len(batchIDs) > 0 {
			keptUsers := db.HotspotUsers[:0]
			for _, u := range db.HotspotUsers {
				if match(u.AccountID) && u.Kind == "voucher" && batchIDs[u.BatchID] {
					noteUser(u)
					counts.Vouchers++
					continue
				}
				keptUsers = append(keptUsers, u)
			}
			db.HotspotUsers = keptUsers
		}
	}

	// 6. Revendeurs + leurs transactions (les vouchers déjà générés sont
	//    conservés : ce sont des identifiants valides du compte).
	//    V3 (audit revendeurs) — la purge couvre AUSSI les transactions
	//    ORPHELINES : dont le ResellerID ne référence plus aucun revendeur
	//    (reliques d'un DELETE /api/resellers/{id} antérieur à la cascade
	//    V2). Elles survivaient à toutes les purges, y compris « all »,
	//    ce qui rendait l'annonce « et leurs N transaction(s) » mensongère.
	//    Portée ciblée : les revendeurs des AUTRES comptes (liveRes, global)
	//    continuent de protéger leurs propres transactions.
	if has(PurgeScopeResellers) {
		resIDs := map[string]bool{}  // revendeurs purgés à l'instant
		liveRes := map[string]bool{} // revendeurs restants (tous comptes)
		keptResellers := db.Resellers[:0]
		for _, res := range db.Resellers {
			if match(res.AccountID) {
				resIDs[res.ID] = true
				counts.Resellers++
				continue
			}
			liveRes[res.ID] = true
			keptResellers = append(keptResellers, res)
		}
		db.Resellers = keptResellers
		keptTx := db.Transactions[:0]
		for _, t := range db.Transactions {
			if match(t.AccountID) && (resIDs[t.ResellerID] || !liveRes[t.ResellerID]) {
				counts.Transactions++
				continue
			}
			keptTx = append(keptTx, t)
		}
		db.Transactions = keptTx
	}

	// 7. Ventes.
	if has(PurgeScopeSales) {
		kept := db.Sales[:0]
		for _, s := range db.Sales {
			if match(s.AccountID) {
				counts.Sales++
				continue
			}
			kept = append(kept, s)
		}
		db.Sales = kept
	}

	// 8. Sessions actives restantes.
	if has(PurgeScopeSessions) {
		kept := db.Sessions[:0]
		for _, s := range db.Sessions {
			if match(s.AccountID) {
				counts.Sessions++
				continue
			}
			kept = append(kept, s)
		}
		db.Sessions = kept
	}

	// 9. Journaux : utilisateurs (login/logout…), activité, notifications,
	//    commandes agent (file d'attente incluse).
	if has(PurgeScopeLogs) {
		keptLogs := db.UserLogs[:0]
		for _, l := range db.UserLogs {
			if match(l.AccountID) {
				counts.Logs++
				continue
			}
			keptLogs = append(keptLogs, l)
		}
		db.UserLogs = keptLogs
		keptActivity := db.Activity[:0]
		for _, act := range db.Activity {
			if match(act.AccountID) {
				counts.Logs++
				continue
			}
			keptActivity = append(keptActivity, act)
		}
		db.Activity = keptActivity
		keptNotif := db.NotifLog[:0]
		for _, nl := range db.NotifLog {
			if match(nl.AccountID) {
				counts.Logs++
				continue
			}
			keptNotif = append(keptNotif, nl)
		}
		db.NotifLog = keptNotif
		keptCommands := db.Commands[:0]
		for _, c := range db.Commands {
			if match(c.AccountID) {
				counts.Logs++
				continue
			}
			keptCommands = append(keptCommands, c)
		}
		db.Commands = keptCommands
	}

	// 10. Gabarits de tickets.
	if has(PurgeScopeTemplates) {
		kept := db.Templates[:0]
		for _, tp := range db.Templates {
			if match(tp.AccountID) {
				counts.Templates++
				continue
			}
			kept = append(kept, tp)
		}
		db.Templates = kept
	}

	// P0 (audit purge/résurgence) — tombstones anti-ré-import, annulation
	// des commandes en file qui recréeraient les entités purgées, et purge
	// TOTALE optionnelle (commandes user_remove aux routeurs agent). Tout
	// est sous verrou ; le Save() ci-dessous persiste tombstones et file.
	counts.Tombstones = writePurgeTombstones(db, purgedUsers, time.Now())
	counts.RouterRemovals = cancelQueuedForPurged(db, purgedUsers)
	if alsoRouter {
		counts.RouterRemovals += queueRemovalsForPurgedAgent(db, purgedAgent)
	}

	// Journal d'activité — écrit APRÈS la purge : si la catégorie journaux
	// était purgée, cette ligne est la première du journal neuf (traçabilité).
	// Portée globale → journal du compte de l'admin plateforme ; portée
	// ciblée → journal du COMPTE CIBLÉ (« par la plateforme »).
	summary := purgeSummaryFR(accName, counts, all)
	if counts.Tombstones > 0 {
		summary += fmt.Sprintf(" — ré-import depuis les routeurs bloqué 30 jours pour %d identifiant(s)", counts.Tombstones)
		if counts.RouterRemovals > 0 {
			summary += fmt.Sprintf(", %d compte(s) en cours de suppression sur les routeurs (commandes agent en file)", counts.RouterRemovals)
		}
	}
	logAccount := accID
	if global {
		logAccount = accountScope(r)
	}
	a.logActivityBy(r, db, logAccount, "system", summary)
	a.store.Save()
	a.store.Unlock()
	a.clearGateways()

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "summary": summary, "purged": counts})
}

// dropByRouterID — retire les éléments attachés aux routeurs à supprimer
// (cascade générique) ; n compteur optionnel. Retourne une slice non-nil vide
// quand tout a été retiré (synchro PG différentielle).
func dropByRouterID[T any](items []T, drop func(id string) bool, routerID func(T) string, n *int) []T {
	kept := make([]T, 0, len(items))
	for _, it := range items {
		if drop(routerID(it)) {
			if n != nil {
				*n++
			}
			continue
		}
		kept = append(kept, it)
	}
	return kept
}

// purgeSummaryFR — libellé du bilan pour le journal d'activité (français).
// accName == "" → portée globale ; sinon bilan de la purge ciblée du compte.
func purgeSummaryFR(accName string, c PurgeCounts, all bool) string {
	var parts []string
	if c.Routers > 0 {
		parts = append(parts, fmt.Sprintf("%d routeur(s) simulé(s)", c.Routers))
	}
	if c.Vouchers > 0 {
		parts = append(parts, fmt.Sprintf("%d voucher(s)", c.Vouchers))
	}
	if c.HotspotUsers > 0 {
		parts = append(parts, fmt.Sprintf("%d utilisateur(s) hotspot", c.HotspotUsers))
	}
	if c.Profiles > 0 {
		parts = append(parts, fmt.Sprintf("%d profil(s)", c.Profiles))
	}
	if c.Batches > 0 {
		parts = append(parts, fmt.Sprintf("%d lot(s) de vouchers", c.Batches))
	}
	if c.Resellers > 0 {
		parts = append(parts, fmt.Sprintf("%d revendeur(s)", c.Resellers))
	}
	if c.Transactions > 0 {
		parts = append(parts, fmt.Sprintf("%d transaction(s)", c.Transactions))
	}
	if c.Sales > 0 {
		parts = append(parts, fmt.Sprintf("%d vente(s)", c.Sales))
	}
	if c.Sessions > 0 {
		parts = append(parts, fmt.Sprintf("%d session(s)", c.Sessions))
	}
	if c.Logs > 0 {
		parts = append(parts, fmt.Sprintf("%d entrée(s) de journaux", c.Logs))
	}
	if c.Templates > 0 {
		parts = append(parts, fmt.Sprintf("%d gabarit(s)", c.Templates))
	}
	empty := " : rien à supprimer (données déjà absentes)"
	if accName == "" {
		if len(parts) == 0 {
			return "Purge des données" + empty
		}
		prefix := "Purge des données — "
		if all {
			prefix = "Purge totale des données — "
		}
		return prefix + strings.Join(parts, ", ")
	}
	if len(parts) == 0 {
		return "Purge ciblée du compte «" + accName + "»" + empty
	}
	prefix := "Purge ciblée du compte «" + accName + "» (par la plateforme) — "
	if all {
		prefix = "Purge totale du compte «" + accName + "» (par la plateforme) — "
	}
	return prefix + strings.Join(parts, ", ")
}

// ---------------------------------------------------------------------------
// Nettoyage CHIRURGICALE des données de démonstration (POST /api/admin/purge-demo)
// ---------------------------------------------------------------------------

// seedResellerIDs — les 5 revendeurs créés par l'ancien seed de démonstration
// (BuildSeed, supprimé du code) : identifiants littéraux à faible entropie,
// distincts des revendeurs réels créés par l'application
// (model.NewID("res-") — suffixe aléatoire).
var seedResellerIDs = map[string]bool{
	"res-1": true, "res-2": true, "res-3": true, "res-4": true, "res-5": true,
}

// handlePurgeDemo — POST /api/admin/purge-demo : suppression DÉFINITIVE des
// artefacts hérités de l'ancien seed de démonstration, SANS toucher aux
// données réelles :
//
//   - routeurs SIMULÉS + cascade complète (l'ancien seed ne créait que des
//     routeurs simulés : tous les utilisateurs/tickets/lots/ventes/sessions/
//     journaux de démo y sont rattachés et partent avec eux) ;
//   - revendeurs « res-1 »…« res-5 » (Aya Koné, Kouassi Kouamé, …) + leurs
//     transactions simulées — les revendeurs réels (identifiants aléatoires)
//     ne sont JAMAIS touchés ;
//   - lots et ventes émis par ces routeurs (le registre comptable de démo).
//
// Ne touche JAMAIS : routeurs réels (agent), comptes, équipe, profils,
// réglages, abonnement, gabarits. Ne régénère RIEN. Sous verrou global :
// la synchro différentielle PostgreSQL propage les suppressions.
func (a *API) handlePurgeDemo(w http.ResponseWriter, r *http.Request) {
	if !isPlatformAdmin(r) {
		writeErr(w, http.StatusForbidden, "Réservé aux administrateurs de la plateforme")
		return
	}
	a.store.Lock()
	db := a.store.Data()
	var counts PurgeCounts

	// 1. Routeurs simulés + cascade (mêmes garanties que la catégorie
	//    « simulated_routers » de la purge par catégories).
	simIDs := map[string]bool{}
	kept := db.Routers[:0]
	for _, rr := range db.Routers {
		if rr.Mode == "simulated" {
			simIDs[rr.ID] = true
			counts.Routers++
			continue
		}
		kept = append(kept, rr)
	}
	db.Routers = kept
	if len(simIDs) > 0 {
		drop := func(id string) bool { return simIDs[id] }
		db.HotspotUsers = dropByRouterID(db.HotspotUsers, drop, func(u model.HotspotUser) string { return u.RouterID }, &counts.HotspotUsers)
		db.Sessions = dropByRouterID(db.Sessions, drop, func(s model.Session) string { return s.RouterID }, &counts.Sessions)
		db.Traffic = dropByRouterID(db.Traffic, drop, func(t model.RouterTraffic) string { return t.RouterID }, nil)
		db.Commands = dropByRouterID(db.Commands, drop, func(c model.Command) string { return c.RouterID }, nil)
		db.IPBindings = dropByRouterID(db.IPBindings, drop, func(b model.IPBinding) string { return b.RouterID }, nil)
		db.SchedulerTasks = dropByRouterID(db.SchedulerTasks, drop, func(t model.SchedulerTask) string { return t.RouterID }, nil)
		keptBatches := db.Batches[:0]
		for _, b := range db.Batches {
			if drop(b.RouterID) {
				counts.Batches++
				continue
			}
			keptBatches = append(keptBatches, b)
		}
		db.Batches = keptBatches
		keptSales := db.Sales[:0]
		for _, s := range db.Sales {
			if drop(s.RouterID) {
				counts.Sales++
				continue
			}
			keptSales = append(keptSales, s)
		}
		db.Sales = keptSales
	}

	// 2. Revendeurs de démonstration (res-1…res-5) + leurs transactions.
	keptResellers := db.Resellers[:0]
	for _, res := range db.Resellers {
		if seedResellerIDs[res.ID] {
			counts.Resellers++
			continue
		}
		keptResellers = append(keptResellers, res)
	}
	db.Resellers = keptResellers
	keptTx := db.Transactions[:0]
	for _, t := range db.Transactions {
		if seedResellerIDs[t.ResellerID] {
			counts.Transactions++
			continue
		}
		keptTx = append(keptTx, t)
	}
	db.Transactions = keptTx

	// Journal d'activité écrit APRÈS le nettoyage (traçabilité).
	summary := purgeDemoSummaryFR(counts)
	a.logActivityBy(r, db, accountScope(r), "system", summary)
	a.store.Save()
	a.store.Unlock()
	a.clearGateways()

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "summary": summary, "purged": counts})
}

// purgeDemoSummaryFR — bilan lisible pour le journal d'activité et le toast.
func purgeDemoSummaryFR(c PurgeCounts) string {
	var parts []string
	if c.Routers > 0 {
		parts = append(parts, fmt.Sprintf("%d routeur(s) simulé(s)", c.Routers))
	}
	if c.HotspotUsers > 0 {
		parts = append(parts, fmt.Sprintf("%d utilisateur(s)/ticket(s) de démo", c.HotspotUsers))
	}
	if c.Batches > 0 {
		parts = append(parts, fmt.Sprintf("%d lot(s) de démo", c.Batches))
	}
	if c.Sales > 0 {
		parts = append(parts, fmt.Sprintf("%d vente(s) de démo", c.Sales))
	}
	if c.Sessions > 0 {
		parts = append(parts, fmt.Sprintf("%d session(s) de démo", c.Sessions))
	}
	if c.Resellers > 0 {
		parts = append(parts, fmt.Sprintf("%d revendeur(s) de démo", c.Resellers))
	}
	if c.Transactions > 0 {
		parts = append(parts, fmt.Sprintf("%d transaction(s) de démo", c.Transactions))
	}
	if len(parts) == 0 {
		return "Nettoyage des données de démonstration : rien à supprimer (base déjà propre)"
	}
	return "Nettoyage des données de démonstration — " + strings.Join(parts, ", ")
}

// ---------------------------------------------------------------------------
// Compteurs par compte (GET /api/admin/purge/accounts — alimente le
// sélecteur de PORTÉE de la purge fusionnée) ; l'EXÉCUTION, elle, passe par
// POST /api/admin/purge avec accountId (moteur unique purgeScopes).
// ---------------------------------------------------------------------------

// AccountPurgeStats — compteurs par élément pour UN compte
// (GET /api/admin/purge/accounts — zone sensible de la console plateforme).
// Mêmes règles que PurgeStats : les entités attachées aux routeurs simulés
// ne sont PAS comptées dans leur catégorie (elles partent en cascade avec
// le routeur).
type AccountPurgeStats struct {
	SimulatedRouters int `json:"simulatedRouters"`
	HotspotUsers     int `json:"hotspotUsers"` // comptes client (kind != voucher)
	Vouchers         int `json:"vouchers"`     // tickets (kind == voucher)
	Profiles         int `json:"profiles"`
	Batches          int `json:"batches"`
	Resellers        int `json:"resellers"`
	Transactions     int `json:"transactions"`
	Sales            int `json:"sales"`
	Sessions         int `json:"sessions"`
	Logs             int `json:"logs"`
	Templates        int `json:"templates"`
}

// AccountPurgeRow — ligne de la liste des comptes purgables.
type AccountPurgeRow struct {
	ID     string            `json:"id"`
	Name   string            `json:"name"`
	Owner  string            `json:"owner"`
	Status string            `json:"status"`
	Stats  AccountPurgeStats `json:"stats"`
}

// handlePurgeAccountsStats — GET /api/admin/purge/accounts : tous les
// comptes avec leurs compteurs par élément (alimente le sélecteur et les
// cases à cocher de la zone sensible).
func (a *API) handlePurgeAccountsStats(w http.ResponseWriter, r *http.Request) {
	if !isPlatformAdmin(r) {
		writeErr(w, http.StatusForbidden, "Réservé aux administrateurs de la plateforme")
		return
	}
	a.store.Lock()
	db := a.store.Data()

	// Propriétaire = premier utilisateur (owner ou admin) du compte —
	// même règle d'affichage que GET /api/admin/accounts.
	owners := map[string]string{}
	for i := range db.Users {
		u := &db.Users[i]
		if u.Role != "owner" && u.Role != "admin" {
			continue
		}
		if _, seen := owners[u.AccountID]; !seen {
			owners[u.AccountID] = u.Username
		}
	}

	stats := map[string]*AccountPurgeStats{}
	rows := make([]AccountPurgeRow, 0, len(db.Accounts))
	for i := range db.Accounts {
		acc := db.Accounts[i]
		stats[acc.ID] = &AccountPurgeStats{}
		rows = append(rows, AccountPurgeRow{ID: acc.ID, Name: acc.Name, Owner: owners[acc.ID], Status: acc.Status, Stats: *stats[acc.ID]})
	}
	idx := map[string]int{}
	for i := range rows {
		idx[rows[i].ID] = i
	}
	// Un seul passage par collection (les comptes fantômes — entités
	// orphelines de compte supprimé — sont ignorés : non listés).
	for _, rr := range db.Routers {
		i, ok := idx[rr.AccountID]
		if !ok {
			continue
		}
		if rr.Mode == "simulated" {
			rows[i].Stats.SimulatedRouters++
		}
	}
	// Routeurs simulés du compte : leurs utilisateurs/sessions partent en
	// cascade — exclus des compteurs de leur catégorie (règle globale).
	simByAccount := map[string]map[string]bool{}
	for _, rr := range db.Routers {
		if rr.Mode != "simulated" {
			continue
		}
		set, ok := simByAccount[rr.AccountID]
		if !ok {
			set = map[string]bool{}
			simByAccount[rr.AccountID] = set
		}
		set[rr.ID] = true
	}
	onSim := func(acc, routerID string) bool {
		set, ok := simByAccount[acc]
		return ok && set[routerID]
	}
	for _, u := range db.HotspotUsers {
		i, ok := idx[u.AccountID]
		if !ok || onSim(u.AccountID, u.RouterID) {
			continue
		}
		if u.Kind == "voucher" {
			rows[i].Stats.Vouchers++
		} else {
			rows[i].Stats.HotspotUsers++
		}
	}
	for _, s := range db.Sessions {
		if i, ok := idx[s.AccountID]; ok && !onSim(s.AccountID, s.RouterID) {
			rows[i].Stats.Sessions++
		}
	}
	for _, p := range db.Profiles {
		if i, ok := idx[p.AccountID]; ok {
			rows[i].Stats.Profiles++
		}
	}
	for _, b := range db.Batches {
		if i, ok := idx[b.AccountID]; ok {
			rows[i].Stats.Batches++
		}
	}
	for _, res := range db.Resellers {
		if i, ok := idx[res.AccountID]; ok {
			rows[i].Stats.Resellers++
		}
	}
	for _, t := range db.Transactions {
		if i, ok := idx[t.AccountID]; ok {
			rows[i].Stats.Transactions++
		}
	}
	for _, s := range db.Sales {
		if i, ok := idx[s.AccountID]; ok {
			rows[i].Stats.Sales++
		}
	}
	for _, l := range db.UserLogs {
		if i, ok := idx[l.AccountID]; ok {
			rows[i].Stats.Logs++
		}
	}
	for _, act := range db.Activity {
		if i, ok := idx[act.AccountID]; ok {
			rows[i].Stats.Logs++
		}
	}
	for _, nl := range db.NotifLog {
		if i, ok := idx[nl.AccountID]; ok {
			rows[i].Stats.Logs++
		}
	}
	for _, c := range db.Commands {
		if i, ok := idx[c.AccountID]; ok {
			rows[i].Stats.Logs++
		}
	}
	for _, tp := range db.Templates {
		if i, ok := idx[tp.AccountID]; ok {
			rows[i].Stats.Templates++
		}
	}
	a.store.Unlock()

	sort.Slice(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })
	writeJSON(w, http.StatusOK, rows)
}
