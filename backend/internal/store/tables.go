// tables.go — N°133 — P1 structurel de l'audit performance : tables marquées
// « sales » à l'écriture, pour une synchronisation différentielle CIBLÉE.
//
// Contexte (audit N°130, cause B résiduelle) : la sauvegarde asynchrone a
// sorti la transaction Neon du verrou global, mais CHAQUE synchro re-hashe
// toujours les 33 tables différentielles COMPLÈTES (json.Marshal + FNV par
// ligne) — sur le 0,1 vCPU Render, ~8 500 lignes (3 130 hotspot_users +
// 5 000 user_logs plafonnés + le reste) soit ~1,4-2,8 s de CPU par flush,
// alors que l'état de croisière d'un parc AGENT ne change que de 6 lignes
// (2 routeurs télémétrie + 4 settings last_tick).
//
// Principe : les chemins de LECTURE pollés (dashboard 15 s, sessions 10 s,
// listes, check-in agent 45 s) marquent précisément les tables qu'ils ont
// réellement modifiées (Tick + enforcement retournent leur empreinte) et
// demandent une sauvegarde CIBLÉE via Store.SaveTables : la synchro ne
// re-hashe alors QUE ces tables, les autres conservent leurs empreintes.
// TOUT autre appelant continue d'utiliser Store.Save() = diff complet —
// comportement inchangé et sûre par défaut (une mutation oubliée dans le
// marquage ne peut donc venir QUE d'un chemin de lecture étroitement
// audité, jamais d'un handler métier).
//
// Le mode JSON (développement/E2E) ignore le ciblage : pas de diff possible,
// SaveTables retombe sur l'écriture complète synchrone (comportement
// strictement identique à Save).
package store

import "sort"

// Noms canoniques des tables de la synchro différentielle — source de
// vérité : les specs de pg_specs.go et la volumétrie de liveTableRows
// (syncstats.go). Le test TestSyncKnownTablesConcordance verrouille
// l'alignement des trois listes (constantes ↔ specs ↔ santé).
const (
	TableAccounts             = "accounts"
	TableAdminUsers           = "admin_users"
	TableRouters              = "routers"
	TableProfiles             = "profiles"
	TableHotspotUsers         = "hotspot_users"
	TableBatches              = "batches"
	TableResellers            = "resellers"
	TableSellSessions         = "sell_sessions"
	TablePasswordResets       = "password_resets"
	TableTransactions         = "transactions"
	TableSessions             = "sessions"
	TableActivity             = "activity"
	TableSales                = "sales"
	TableCommands             = "commands"
	TableVoucherTemplates     = "voucher_templates"
	TableUserLogs             = "user_logs"
	TableIPBindings           = "ip_bindings"
	TableSchedulerTasks       = "scheduler_tasks"
	TableTraffic              = "traffic"
	TableLineQuality          = "line_quality"
	TableNotifSettings        = "notif_settings"
	TableNotifLog             = "notif_log"
	TableBillingRequests      = "billing_requests"
	TablePurgeTombstones      = "purge_tombstones"
	TableJoinLinks            = "join_links"
	TableRegistrationRequests = "registration_requests"
	TableWifiSites            = "wifi_sites"
	TableWifiGuests           = "wifi_guests"
	TablePromoEvents          = "promo_events"
	TableGeniusPaySubs        = "geniuspay_subs"
	TableChatConversations    = "chat_conversations"
	TableChatMessages         = "chat_messages"
	TableDevices              = "devices"
	// N°152 — annonces de la plateforme (collection globale, diff simple).
	TableAnnouncements = "announcements"
	TableSettings      = "settings" // hors diff d'empreintes (syncSettings)
)

// syncKnownTables — registre des noms acceptés par SaveTables/SyncTables :
// filtre les coquilles éventuelles (un nom inconnu ne correspond à aucune
// spec — il serait silencieusement ignoré par la synchro).
var syncKnownTables = map[string]bool{
	TableAccounts:             true,
	TableAdminUsers:           true,
	TableRouters:              true,
	TableProfiles:             true,
	TableHotspotUsers:         true,
	TableBatches:              true,
	TableResellers:            true,
	TableSellSessions:         true,
	TablePasswordResets:       true,
	TableTransactions:         true,
	TableSessions:             true,
	TableActivity:             true,
	TableSales:                true,
	TableCommands:             true,
	TableVoucherTemplates:     true,
	TableUserLogs:             true,
	TableIPBindings:           true,
	TableSchedulerTasks:       true,
	TableTraffic:              true,
	TableLineQuality:          true,
	TableNotifSettings:        true,
	TableNotifLog:             true,
	TableBillingRequests:      true,
	TablePurgeTombstones:      true,
	TableJoinLinks:            true,
	TableRegistrationRequests: true,
	TableWifiSites:            true,
	TableWifiGuests:           true,
	TablePromoEvents:          true,
	TableGeniusPaySubs:        true,
	TableChatConversations:    true,
	TableChatMessages:         true,
	TableDevices:              true,
	TableAnnouncements:        true,
	TableSettings:             true,
}

// TableSet — ensemble de noms de tables marquées sales, rempli au fil des
// mutations par le moteur (Tick/applyExpiry/enforcement). Un pointeur NIL
// est un récepteur valide et absorbe tous les appels (Mark/Has no-op) :
// les fonctions du moteur peuvent ainsi être appelées avec nil (Sweep,
// tests) sans garde supplémentaire.
type TableSet struct {
	m map[string]struct{}
}

// NewTableSet — ensemble pré-rempli (les noms vides ou inconnus sont ignorés).
func NewTableSet(names ...string) *TableSet {
	ts := &TableSet{}
	ts.Mark(names...)
	return ts
}

// Mark — ajoute des tables à l'ensemble (idempotent, nil-safe, filtre les
// noms vides ou hors registre).
func (t *TableSet) Mark(names ...string) {
	if t == nil || len(names) == 0 {
		return
	}
	if t.m == nil {
		t.m = make(map[string]struct{}, len(names))
	}
	for _, n := range names {
		if n == "" || !syncKnownTables[n] {
			continue
		}
		t.m[n] = struct{}{}
	}
}

// Has — la table est-elle marquée ? (nil → faux)
func (t *TableSet) Has(name string) bool {
	if t == nil {
		return false
	}
	_, ok := t.m[name]
	return ok
}

// Empty — aucun marquage ? (nil → vrai)
func (t *TableSet) Empty() bool {
	return t == nil || len(t.m) == 0
}

// Names — noms triés (ordre déterministe : journaux et tests stables).
func (t *TableSet) Names() []string {
	if t == nil || len(t.m) == 0 {
		return nil
	}
	out := make([]string, 0, len(t.m))
	for n := range t.m {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
