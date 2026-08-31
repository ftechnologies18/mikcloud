// Package api — purge des données par catégories (Paramètres → « Purge des
// données »).
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
	"strings"

	"mikcloud/hotspot-api/internal/model"
)

// Scopes de purge (identifiants stables consommés par le frontend).
const (
	PurgeScopeSimulatedRouters = "simulated_routers" // + cascade : utilisateurs, sessions, trafic, commandes, bindings, schedulers
	PurgeScopeHotspotUsers     = "hotspot_users"     // utilisateurs hotspot restants
	PurgeScopeProfiles         = "profiles"          // profils / forfaits
	PurgeScopeBatches          = "batches"           // lots de vouchers + leurs vouchers
	PurgeScopeResellers        = "resellers"         // revendeurs + transactions
	PurgeScopeSales            = "sales"             // ventes
	PurgeScopeSessions         = "sessions"          // sessions actives restantes
	PurgeScopeLogs             = "logs"              // journaux : utilisateurs, activité, notifications, commandes agent
	PurgeScopeTemplates        = "templates"         // gabarits de tickets
	PurgeScopeAll              = "all"               // toutes les catégories ci-dessus
)

// purgeScopeSet — les 9 catégories purgables (« all » mis à part).
var purgeScopeSet = []string{
	PurgeScopeSimulatedRouters, PurgeScopeHotspotUsers, PurgeScopeProfiles,
	PurgeScopeBatches, PurgeScopeResellers, PurgeScopeSales,
	PurgeScopeSessions, PurgeScopeLogs, PurgeScopeTemplates,
}

// PurgeStats — compteurs LIVE affichés dans l'UI (GET /api/admin/purge/stats).
// Les utilisateurs/sessions attachés à des routeurs simulés ne sont PAS
// comptés dans leurs catégories : ils partent avec le routeur (cascade).
type PurgeStats struct {
	SimulatedRouters int `json:"simulatedRouters"`
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
	Profiles     int `json:"profiles"`
	Batches      int `json:"batches"`
	Resellers    int `json:"resellers"`
	Transactions int `json:"transactions"`
	Sales        int `json:"sales"`
	Sessions     int `json:"sessions"`
	Logs         int `json:"logs"`
	Templates    int `json:"templates"`
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
	for _, u := range db.HotspotUsers {
		if simIDs[u.RouterID] {
			continue
		}
		stats.HotspotUsers++
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

// handlePurge — POST /api/admin/purge : purge par catégories.
// Corps : {"scopes": ["simulated_routers", "hotspot_users", …]}
// (le scope « all » sélectionne les 9 catégories ; {"scope":"all"} accepté).
func (a *API) handlePurge(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Scopes []string `json:"scopes"`
		Scope  string   `json:"scope"` // compat mono-scope
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
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
			a.purgeScopes(w, r, nil) // nil → « all »
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
		writeErr(w, http.StatusBadRequest, "Aucune catégorie sélectionnée")
		return
	}
	a.purgeScopes(w, r, normalized)
}

// purgeScopes — moteur commun (handlePurge + handleWipe). scopes nil ou vide
// → « all ». Sous verrou global ; ne touche JAMAIS les routeurs réels
// (agent/real), les comptes, l'équipe, les réglages ni l'abonnement ; ne
// régénère RIEN. Les slices sont réaffectées vides (non-nil) pour que la
// synchro différentielle PostgreSQL supprime les lignes correspondantes.
func (a *API) purgeScopes(w http.ResponseWriter, r *http.Request, scopes []string) {
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

	a.store.Lock()
	db := a.store.Data()
	var counts PurgeCounts

	// 1. Routeurs SIMULÉS + cascade complète (utilisateurs, sessions, trafic,
	//    commandes, IP bindings, schedulers). Les routeurs réels (agent/real)
	//    sont systématiquement préservés.
	if has(PurgeScopeSimulatedRouters) {
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
		}
	}

	// 2. Utilisateurs hotspot RESTANTS (les sessions qui leur sont liées sont
	//    closes avec eux — une session sans utilisateur n'a plus de sens).
	if has(PurgeScopeHotspotUsers) {
		orphan := map[string]bool{}
		for _, u := range db.HotspotUsers {
			orphan[u.ID] = true
		}
		counts.HotspotUsers += len(db.HotspotUsers)
		db.HotspotUsers = []model.HotspotUser{}
		if len(orphan) > 0 && len(db.Sessions) > 0 {
			keptSessions := db.Sessions[:0]
			for _, s := range db.Sessions {
				if orphan[s.UserID] {
					counts.Sessions++
					continue
				}
				keptSessions = append(keptSessions, s)
			}
			db.Sessions = keptSessions
		}
	}

	// 3. Profils / forfaits.
	if has(PurgeScopeProfiles) {
		counts.Profiles = len(db.Profiles)
		db.Profiles = []model.Profile{}
	}

	// 4. Lots de vouchers + les vouchers générés par ces lots.
	if has(PurgeScopeBatches) {
		batchIDs := map[string]bool{}
		for _, b := range db.Batches {
			batchIDs[b.ID] = true
		}
		counts.Batches = len(db.Batches)
		db.Batches = []model.Batch{}
		if len(batchIDs) > 0 && len(db.HotspotUsers) > 0 {
			keptUsers := db.HotspotUsers[:0]
			for _, u := range db.HotspotUsers {
				if u.Kind == "voucher" && batchIDs[u.BatchID] {
					counts.HotspotUsers++
					continue
				}
				keptUsers = append(keptUsers, u)
			}
			db.HotspotUsers = keptUsers
		}
	}

	// 5. Revendeurs + leurs transactions (les vouchers déjà générés sont
	//    conservés : ce sont des identifiants valides du compte).
	if has(PurgeScopeResellers) {
		counts.Resellers = len(db.Resellers)
		db.Resellers = []model.Reseller{}
		counts.Transactions = len(db.Transactions)
		db.Transactions = []model.Transaction{}
	}

	// 6. Ventes.
	if has(PurgeScopeSales) {
		counts.Sales = len(db.Sales)
		db.Sales = []model.Sale{}
	}

	// 7. Sessions actives restantes.
	if has(PurgeScopeSessions) {
		counts.Sessions += len(db.Sessions)
		db.Sessions = []model.Session{}
	}

	// 8. Journaux : utilisateurs (login/logout…), activité, notifications,
	//    commandes agent (file d'attente incluse).
	if has(PurgeScopeLogs) {
		counts.Logs = len(db.UserLogs) + len(db.Activity) + len(db.NotifLog) + len(db.Commands)
		db.UserLogs = []model.UserLog{}
		db.Activity = []model.Activity{}
		db.NotifLog = []model.NotificationLog{}
		db.Commands = []model.Command{}
	}

	// 9. Gabarits de tickets.
	if has(PurgeScopeTemplates) {
		counts.Templates = len(db.Templates)
		db.Templates = []model.VoucherTemplate{}
	}

	// Journal d'activité — écrit APRÈS la purge : si la catégorie journaux
	// était purgée, cette ligne est la première du journal neuf (traçabilité).
	summary := purgeSummaryFR(counts, all)
	a.logActivityBy(r, db, accountScope(r), "system", summary)
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
func purgeSummaryFR(c PurgeCounts, all bool) string {
	var parts []string
	if c.Routers > 0 {
		parts = append(parts, fmt.Sprintf("%d routeur(s) simulé(s)", c.Routers))
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
	if len(parts) == 0 {
		return "Purge des données : rien à supprimer (données déjà absentes)"
	}
	prefix := "Purge des données — "
	if all {
		prefix = "Purge totale des données — "
	}
	return prefix + strings.Join(parts, ", ")
}
