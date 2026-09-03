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
	"sort"
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
	Vouchers     int `json:"vouchers"` // purge ciblée uniquement (tickets kind=voucher)
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
// Purge CIBLÉE par compte (zone sensible — console plateforme)
//
// Complément CHIRURGICAL de la purge globale : l'administrateur plateforme
// choisit UN compte client et les catégories d'éléments à supprimer pour
// CE compte uniquement (vouchers, utilisateurs hotspot, ventes, journaux…).
// Les autres comptes ne sont JAMAIS touchés. Mêmes garanties que la purge
// globale : ne touche jamais les routeurs réels (agent/real), les comptes,
// l'équipe, les réglages, l'abonnement ni la facturation ; ne régénère RIEN ;
// synchro différentielle PostgreSQL via réaffectation de slices (non-nil).
// ---------------------------------------------------------------------------

// PurgeScopeVouchers — tickets uniquement (hotspot users kind=voucher) :
// les LOTS (métadonnées) sont conservés ; le scope « batches » supprime
// lots + vouchers restants. Distinction propre à la purge ciblée — la
// purge globale conserve sa sémantique historique (batches = cascade).
const PurgeScopeVouchers = "vouchers"

// purgeAccountScopeSet — catégories purgables sur UN compte ciblé
// (« all » mis à part). Sur-ensemble de purgeScopeSet (+ vouchers).
var purgeAccountScopeSet = append([]string{PurgeScopeVouchers}, purgeScopeSet...)

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

// handlePurgeAccount — POST /api/admin/purge/account : purge ciblée sur UN
// compte. Corps : {"accountId": "acc-…", "scopes": ["vouchers", "sales", …]}
// (le scope « all » sélectionne toutes les catégories ciblables du compte).
// Les catégories sont les mêmes identifiants que la purge globale, plus
// « vouchers » (tickets sans les lots). Les autres comptes ne sont jamais
// touchés ; les garanties structurelles de la purge globale s'appliquent.
func (a *API) handlePurgeAccount(w http.ResponseWriter, r *http.Request) {
	if !isPlatformAdmin(r) {
		writeErr(w, http.StatusForbidden, "Réservé aux administrateurs de la plateforme")
		return
	}
	var body struct {
		AccountID string   `json:"accountId"`
		Scopes    []string `json:"scopes"`
		Scope     string   `json:"scope"` // compat mono-scope
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	accID := strings.TrimSpace(body.AccountID)
	if accID == "" {
		writeErr(w, http.StatusBadRequest, "Compte cible manquant (accountId)")
		return
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
			normalized = nil // « all » → toutes les catégories ciblables
			break
		}
		valid := false
		for _, known := range purgeAccountScopeSet {
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
	if len(normalized) == 0 && len(scopes) > 0 {
		// « all » ou aucune catégorie exploitable : distinguer les deux cas.
		all := false
		for _, s := range scopes {
			if strings.TrimSpace(s) == PurgeScopeAll {
				all = true
				break
			}
		}
		if !all {
			writeErr(w, http.StatusBadRequest, "Aucune catégorie sélectionnée")
			return
		}
	}
	a.purgeAccountScopes(w, r, accID, normalized)
}

// purgeAccountScopes — moteur de la purge ciblée (sous verrou global).
// scopes nil ou vide → « all » pour le compte. Cascade identique à la purge
// globale, RESTREINTE au compte : les utilisateurs/sessions des routeurs
// simulés partent avec eux ; purger les lots retire leurs vouchers restants ;
// purger les revendeurs retire leurs transactions ; purger les utilisateurs
// ou les tickets ferme les sessions liées. La ligne d'activité est écrite
// APRÈS la purge sur le compte ciblé (traçabilité, première ligne du journal
// neuf quand « logs » était purgé).
func (a *API) purgeAccountScopes(w http.ResponseWriter, r *http.Request, accID string, scopes []string) {
	a.store.Lock()
	db := a.store.Data()

	// Le compte cible doit exister (les comptes et l'équipe ne sont jamais
	// purgés : la cible n'est qu'un filtre).
	accName := ""
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

	dbm := db
	var counts PurgeCounts

	// 1. Routeurs SIMULÉS du compte + cascade complète (même garanties que
	//    la catégorie globale « simulated_routers ») : utilisateurs, sessions,
	//    trafic, commandes, IP bindings, schedulers, lots, ventes.
	if has(PurgeScopeSimulatedRouters) {
		simIDs := map[string]bool{}
		kept := dbm.Routers[:0]
		for _, rr := range dbm.Routers {
			if rr.AccountID == accID && rr.Mode == "simulated" {
				simIDs[rr.ID] = true
				counts.Routers++
				continue
			}
			kept = append(kept, rr)
		}
		dbm.Routers = kept
		if len(simIDs) > 0 {
			drop := func(id string) bool { return simIDs[id] }
			dbm.HotspotUsers = dropByRouterID(dbm.HotspotUsers, drop, func(u model.HotspotUser) string { return u.RouterID }, &counts.HotspotUsers)
			dbm.Sessions = dropByRouterID(dbm.Sessions, drop, func(s model.Session) string { return s.RouterID }, &counts.Sessions)
			dbm.Traffic = dropByRouterID(dbm.Traffic, drop, func(t model.RouterTraffic) string { return t.RouterID }, nil)
			dbm.Commands = dropByRouterID(dbm.Commands, drop, func(c model.Command) string { return c.RouterID }, nil)
			dbm.IPBindings = dropByRouterID(dbm.IPBindings, drop, func(b model.IPBinding) string { return b.RouterID }, nil)
			dbm.SchedulerTasks = dropByRouterID(dbm.SchedulerTasks, drop, func(t model.SchedulerTask) string { return t.RouterID }, nil)
			keptBatches := dbm.Batches[:0]
			for _, b := range dbm.Batches {
				if drop(b.RouterID) {
					counts.Batches++
					continue
				}
				keptBatches = append(keptBatches, b)
			}
			dbm.Batches = keptBatches
			keptSales := dbm.Sales[:0]
			for _, s := range dbm.Sales {
				if drop(s.RouterID) {
					counts.Sales++
					continue
				}
				keptSales = append(keptSales, s)
			}
			dbm.Sales = keptSales
		}
	}

	// 2. Tickets du compte (kind=voucher) — les LOTS restent. Les sessions
	//    liées aux tickets supprimés sont closes (une session sans
	//    utilisateur n'a plus de sens).
	if has(PurgeScopeVouchers) {
		dropUser := map[string]bool{}
		keptUsers := make([]model.HotspotUser, 0, len(dbm.HotspotUsers))
		for _, u := range dbm.HotspotUsers {
			if u.AccountID == accID && u.Kind == "voucher" {
				dropUser[u.ID] = true
				counts.Vouchers++
				continue
			}
			keptUsers = append(keptUsers, u)
		}
		dbm.HotspotUsers = keptUsers
		if len(dropUser) > 0 {
			keptSessions := dbm.Sessions[:0]
			for _, s := range dbm.Sessions {
				if dropUser[s.UserID] {
					counts.Sessions++
					continue
				}
				keptSessions = append(keptSessions, s)
			}
			dbm.Sessions = keptSessions
		}
	}

	// 3. Utilisateurs hotspot du compte (kind != voucher) + fermeture de
	//    leurs sessions (même sémantique que la catégorie globale).
	if has(PurgeScopeHotspotUsers) {
		dropUser := map[string]bool{}
		keptUsers := make([]model.HotspotUser, 0, len(dbm.HotspotUsers))
		for _, u := range dbm.HotspotUsers {
			if u.AccountID == accID && u.Kind != "voucher" {
				dropUser[u.ID] = true
				counts.HotspotUsers++
				continue
			}
			keptUsers = append(keptUsers, u)
		}
		dbm.HotspotUsers = keptUsers
		if len(dropUser) > 0 {
			keptSessions := dbm.Sessions[:0]
			for _, s := range dbm.Sessions {
				if dropUser[s.UserID] {
					counts.Sessions++
					continue
				}
				keptSessions = append(keptSessions, s)
			}
			dbm.Sessions = keptSessions
		}
	}

	// 4. Profils / forfaits du compte.
	if has(PurgeScopeProfiles) {
		kept := dbm.Profiles[:0]
		for _, p := range dbm.Profiles {
			if p.AccountID == accID {
				counts.Profiles++
				continue
			}
			kept = append(kept, p)
		}
		dbm.Profiles = kept
	}

	// 5. Lots du compte + leurs vouchers RESTANTS (cascade — les tickets
	//    déjà retirés par le scope « vouchers » ne comptent pas deux fois).
	if has(PurgeScopeBatches) {
		batchIDs := map[string]bool{}
		keptBatches := dbm.Batches[:0]
		for _, b := range dbm.Batches {
			if b.AccountID == accID {
				batchIDs[b.ID] = true
				counts.Batches++
				continue
			}
			keptBatches = append(keptBatches, b)
		}
		dbm.Batches = keptBatches
		if len(batchIDs) > 0 {
			keptUsers := dbm.HotspotUsers[:0]
			for _, u := range dbm.HotspotUsers {
				if u.AccountID == accID && u.Kind == "voucher" && batchIDs[u.BatchID] {
					counts.Vouchers++
					continue
				}
				keptUsers = append(keptUsers, u)
			}
			dbm.HotspotUsers = keptUsers
		}
	}

	// 6. Revendeurs du compte + leurs transactions (mêmes garanties que la
	//    catégorie globale : les vouchers déjà générés restent valides).
	if has(PurgeScopeResellers) {
		resIDs := map[string]bool{}
		keptResellers := dbm.Resellers[:0]
		for _, res := range dbm.Resellers {
			if res.AccountID == accID {
				resIDs[res.ID] = true
				counts.Resellers++
				continue
			}
			keptResellers = append(keptResellers, res)
		}
		dbm.Resellers = keptResellers
		if len(resIDs) > 0 {
			keptTx := dbm.Transactions[:0]
			for _, t := range dbm.Transactions {
				if t.AccountID == accID && resIDs[t.ResellerID] {
					counts.Transactions++
					continue
				}
				keptTx = append(keptTx, t)
			}
			dbm.Transactions = keptTx
		}
	}

	// 7. Ventes du compte.
	if has(PurgeScopeSales) {
		kept := dbm.Sales[:0]
		for _, s := range dbm.Sales {
			if s.AccountID == accID {
				counts.Sales++
				continue
			}
			kept = append(kept, s)
		}
		dbm.Sales = kept
	}

	// 8. Sessions actives restantes du compte.
	if has(PurgeScopeSessions) {
		kept := dbm.Sessions[:0]
		for _, s := range dbm.Sessions {
			if s.AccountID == accID {
				counts.Sessions++
				continue
			}
			kept = append(kept, s)
		}
		dbm.Sessions = kept
	}

	// 9. Journaux du compte : utilisateurs, activité, notifications,
	//    commandes agent (file d'attente incluse).
	if has(PurgeScopeLogs) {
		keptLogs := dbm.UserLogs[:0]
		for _, l := range dbm.UserLogs {
			if l.AccountID == accID {
				counts.Logs++
				continue
			}
			keptLogs = append(keptLogs, l)
		}
		dbm.UserLogs = keptLogs
		keptActivity := dbm.Activity[:0]
		for _, act := range dbm.Activity {
			if act.AccountID == accID {
				counts.Logs++
				continue
			}
			keptActivity = append(keptActivity, act)
		}
		dbm.Activity = keptActivity
		keptNotif := dbm.NotifLog[:0]
		for _, nl := range dbm.NotifLog {
			if nl.AccountID == accID {
				counts.Logs++
				continue
			}
			keptNotif = append(keptNotif, nl)
		}
		dbm.NotifLog = keptNotif
		keptCommands := dbm.Commands[:0]
		for _, c := range dbm.Commands {
			if c.AccountID == accID {
				counts.Logs++
				continue
			}
			keptCommands = append(keptCommands, c)
		}
		dbm.Commands = keptCommands
	}

	// 10. Gabarits de tickets du compte.
	if has(PurgeScopeTemplates) {
		kept := dbm.Templates[:0]
		for _, tp := range dbm.Templates {
			if tp.AccountID == accID {
				counts.Templates++
				continue
			}
			kept = append(kept, tp)
		}
		dbm.Templates = kept
	}

	// Journal d'activité — écrit APRÈS la purge, SUR LE COMPTE CIBLÉ
	// (traçabilité côté client : « par la plateforme ») ; si la catégorie
	// journaux était purgée, cette ligne est la première du journal neuf.
	summary := purgeAccountSummaryFR(accName, counts, all)
	a.logActivityBy(r, db, accID, "system", summary)
	a.store.Save()
	a.store.Unlock()
	a.clearGateways()

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "summary": summary, "purged": counts})
}

// purgeAccountSummaryFR — bilan lisible de la purge ciblée (journal + toast).
func purgeAccountSummaryFR(accName string, c PurgeCounts, all bool) string {
	var parts []string
	if c.Routers > 0 {
		parts = append(parts, fmt.Sprintf("%d routeur(s) simulé(s)", c.Routers))
	}
	if c.HotspotUsers > 0 {
		parts = append(parts, fmt.Sprintf("%d utilisateur(s) hotspot", c.HotspotUsers))
	}
	if c.Vouchers > 0 {
		parts = append(parts, fmt.Sprintf("%d voucher(s)", c.Vouchers))
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
		return "Purge ciblée du compte «" + accName + "» : rien à supprimer (données déjà absentes)"
	}
	prefix := "Purge ciblée du compte «" + accName + "» (par la plateforme) — "
	if all {
		prefix = "Purge totale du compte «" + accName + "» (par la plateforme) — "
	}
	return prefix + strings.Join(parts, ", ")
}
