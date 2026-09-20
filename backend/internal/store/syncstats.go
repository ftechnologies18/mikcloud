// syncstats.go — instrumentation « santé de la persistance » (N°71).
//
// La synchro différentielle FNV-1a (pg.go) ne laissait AUCUNE trace
// observable : un échec n'existait que dans les journaux (une ligne de log),
// et rien ne disait à l'opérateur si Neon recevait bien les deltas, à quel
// rythme, ni combien de lignes voyageaient. Ces compteurs alimentent
// GET /api/admin/sync-status (console plateforme, onglet Maintenance) :
//   - sync   : tentatives / succès / échecs (chaîne d'échecs consécutifs),
//     durée et volumétrie (lignes upsertées / supprimées) du
//     DERNIER delta réussi, dernière erreur ;
//   - neon   : dernier contact confirmé (ping d'ouverture, Load, Sync,
//     keep-alive) et mode du keep-alive ;
//   - tables : lignes MÉMOIRE vs lignes RÉPLIQUÉES (taille du cache
//     d'empreintes) — une dérive qui persiste signale une synchro
//     qui n'aboutit plus alors que l'état continue d'évoluer.
//
// Tout est READ-ONLY : aucune mutation de l'état, aucun verrou nouveau sur
// le chemin critique (Save/Sync) — les compteurs se contentent d'un
// micro-verrou pris alors que le verrou global du store est déjà tenu.
package store

import (
	"sync"
	"time"

	"mikcloud/hotspot-api/internal/model"
)

// syncDelta — volumétrie d'UNE synchro : remplie par syncTable au fil de la
// transaction (uniquement pour les lignes réellement écrites), consommée par
// le defer d'instrumentation de PG.Sync.
type syncDelta struct {
	changed int
	removed int
}

// syncStats — compteurs de la synchro différentielle PostgreSQL.
//
// Verrou DÉDIÉ, jamais tenu pendant une transaction SQL : recordSuccess et
// recordFailure s'exécutent à la sortie de PG.Sync (donc sous le verrou
// global du store, déjà tenu par Save), et snapshot() est appelé par
// Store.SyncHealth qui prend ce même verrou global AVANT de lire — l'ordre
// store.mu → stats.mu est donc toujours le même : aucun interblocement
// possible, aucune contention mesurable.
type syncStats struct {
	mu               sync.Mutex
	attempts         int64     // Sync() appelés depuis le démarrage
	successes        int64     // transactions committées
	failures         int64     // échecs (begin/table/commit — Neon injoignable…)
	consecutiveFails int64     // échecs consécutifs — 0 = flux nominal
	lastSuccessAt    time.Time // dernière transaction committée
	lastSuccessMs    int64     // durée de cette dernière synchro
	lastChangedRows  int       // lignes upsertées par cette dernière synchro
	lastRemovedRows  int       // lignes supprimées par cette dernière synchro
	lastError        string    // dernière erreur (brute, tronquée si géante)
	lastErrorAt      time.Time
}

// maxSyncErrorLen — borne de mémorisation de la dernière erreur : un message
// d'erreur SQL peut embarquer des requêtes entières ; 500 caractères suffisent
// au diagnostic sans gonfler l'état ni la réponse.
const maxSyncErrorLen = 500

// recordSuccess — transaction committée : delta et durée du SUCCÈS le plus
// récent, chaîne d'échecs remise à zéro.
func (s *syncStats) recordSuccess(delta syncDelta, took time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.attempts++
	s.successes++
	s.consecutiveFails = 0
	s.lastSuccessAt = time.Now().UTC()
	s.lastSuccessMs = took.Milliseconds()
	s.lastChangedRows = delta.changed
	s.lastRemovedRows = delta.removed
}

// recordFailure — synchro échouée : l'état mémoire reste la vérité, la
// prochaine sauvegarde retentera (cf. Store.Save) — on trace la cause.
func (s *syncStats) recordFailure(err error, took time.Duration) {
	if err == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.attempts++
	s.failures++
	s.consecutiveFails++
	s.lastError = err.Error()
	if len(s.lastError) > maxSyncErrorLen {
		s.lastError = s.lastError[:maxSyncErrorLen] + "…"
	}
	s.lastErrorAt = time.Now().UTC()
}

// SyncStatsSnapshot — photographie JSON des compteurs (contrat de
// GET /api/admin/sync-status, camelCase).
type SyncStatsSnapshot struct {
	Attempts         int64  `json:"attempts"`
	Successes        int64  `json:"successes"`
	Failures         int64  `json:"failures"`
	ConsecutiveFails int64  `json:"consecutiveFailures"`
	LastSuccessAt    string `json:"lastSuccessAt,omitempty"` // RFC3339 — absent = jamais réussi
	LastSuccessMs    int64  `json:"lastSuccessMs"`
	LastChangedRows  int    `json:"lastChangedRows"`
	LastRemovedRows  int    `json:"lastRemovedRows"`
	LastError        string `json:"lastError,omitempty"`
	LastErrorAt      string `json:"lastErrorAt,omitempty"`
}

// snapshot — copie cohérente des compteurs (micro-verrou).
func (s *syncStats) snapshot() *SyncStatsSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	snap := &SyncStatsSnapshot{
		Attempts:         s.attempts,
		Successes:        s.successes,
		Failures:         s.failures,
		ConsecutiveFails: s.consecutiveFails,
		LastSuccessMs:    s.lastSuccessMs,
		LastChangedRows:  s.lastChangedRows,
		LastRemovedRows:  s.lastRemovedRows,
	}
	if !s.lastSuccessAt.IsZero() {
		snap.LastSuccessAt = isoUTC(s.lastSuccessAt)
	}
	if s.lastError != "" {
		snap.LastError = s.lastError
		snap.LastErrorAt = isoUTC(s.lastErrorAt)
	}
	return snap
}

// isoUTC — RFC3339 UTC, chaîne vide pour le zéro.
func isoUTC(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// NeonHealth — vue contact/keep-alive (mode PostgreSQL uniquement).
type NeonHealth struct {
	LastContactAt string `json:"lastContactAt,omitempty"` // dernier échange confirmé avec Neon
	KeepAliveMode string `json:"keepAliveMode"`           // business | on | off
}

// TableHealth — volumétrie d'une table : lignes mémoire (Rows) et lignes
// répliquées (Mirrored = taille du cache d'empreintes de la dernière synchro
// réussie ; omis en mode JSON local, où il n'a pas de sens).
type TableHealth struct {
	Table    string `json:"table"`
	Rows     int    `json:"rows"`
	Mirrored int    `json:"mirrored,omitempty"`
}

// PersistenceDegraded — N°164 : état du boot résilient. Présent dès que le
// mode PostgreSQL est décidé : Degraded=true tant qu'aucun Load n'a réussi
// (boot en mémoire seule + récupération en cours), puis un bloc de souvenir
// avec RecoveredAt (dernière récupération réussie).
type PersistenceDegraded struct {
	Degraded      bool   `json:"degraded"`
	Since         string `json:"since,omitempty"`       // début du mode dégradé (RFC3339)
	RecoveredAt   string `json:"recoveredAt,omitempty"` // dernière récupération réussie
	RecoveryTries int64  `json:"recoveryTries"`         // tentatives de récupération cumulées
	LastError     string `json:"lastError,omitempty"`   // dernière erreur de récupération
}

// SyncHealth — photographie de la persistance (partie de la réponse de
// GET /api/admin/sync-status ; le bloc agents est calculé par l'API, qui
// possède les constantes de fraîcheur des check-ins).
//
// Mode : "postgresql" (production Render + Neon) ou "json" (développement,
// fichier local atomique — Sync et Neon absents).
type SyncHealth struct {
	Mode     string               `json:"mode"`
	Sync     *SyncStatsSnapshot   `json:"sync,omitempty"`
	Neon     *NeonHealth          `json:"neon,omitempty"`
	Tables   []TableHealth        `json:"tables"`
	Degraded *PersistenceDegraded `json:"degraded,omitempty"` // N°164 — boot résilient
}

// SyncHealth — prend le verrou global le temps de la photographie (compteurs
// et cache d'empreintes sont maintenus par Save/Sync sous ce même verrou).
func (s *Store) SyncHealth() SyncHealth {
	s.mu.Lock()
	defer s.mu.Unlock()
	h := SyncHealth{Mode: "json", Tables: liveTableRows(s.db)}
	if !s.pgActive.Load() {
		return h // mode JSON (développement/E2E) : pas de bloc dégradé
	}
	h.Mode = "postgresql"
	// N°164 — boot résilient : dégradé (pool pas encore installé) ou
	// souvenir de la dernière récupération.
	if s.pgDegraded.Load() {
		d := &PersistenceDegraded{Degraded: true, RecoveryTries: s.pgRecoverTries.Load()}
		if since := s.pgDegradedSince.Load(); since > 0 {
			d.Since = isoUTC(time.Unix(since, 0))
		}
		if e := s.pgRecoverErr.Load(); e != nil {
			d.LastError = *e
		}
		h.Degraded = d
		return h // sync/neon/empreintes : aucun sens tant que rien n'est installé
	}
	if at := s.pgRecoveredAt.Load(); at > 0 {
		h.Degraded = &PersistenceDegraded{RecoveredAt: isoUTC(time.Unix(at, 0))}
	}
	if s.pg == nil {
		return h
	}
	h.Sync = s.pg.stats.snapshot()
	ka := s.pg.kaMode
	if ka == "" {
		ka = "off"
	}
	h.Neon = &NeonHealth{
		KeepAliveMode: ka,
		LastContactAt: isoUTC(time.Unix(s.pg.lastWrite.Load(), 0)),
	}
	for i := range h.Tables {
		if cached, ok := s.pg.hashes[h.Tables[i].Table]; ok {
			h.Tables[i].Mirrored = len(cached)
		}
	}
	return h
}

// liveTableRows — volumétrie mémoire par table. La liste suit EXACTEMENT les
// tables de la synchro différentielle (Sync + rebuildHashes, pg.go) plus la
// table settings (une ligne par compte, hors diff d'empreintes) : toute table
// ajoutée à la synchro doit être ajoutée ici — le test
// TestLiveTableRowsConcordance vérifie le compte total.
func liveTableRows(db *model.DB) []TableHealth {
	return []TableHealth{
		{Table: "accounts", Rows: len(db.Accounts)},
		{Table: "admin_users", Rows: len(db.Users)},
		{Table: "routers", Rows: len(db.Routers)},
		{Table: "profiles", Rows: len(db.Profiles)},
		{Table: "hotspot_users", Rows: len(db.HotspotUsers)},
		{Table: "batches", Rows: len(db.Batches)},
		{Table: "resellers", Rows: len(db.Resellers)},
		{Table: "sell_sessions", Rows: len(db.SellSessions)},
		{Table: "password_resets", Rows: len(db.PasswordResets)},
		{Table: "transactions", Rows: len(db.Transactions)},
		{Table: "sessions", Rows: len(db.Sessions)},
		{Table: "activity", Rows: len(db.Activity)},
		{Table: "sales", Rows: len(db.Sales)},
		{Table: "commands", Rows: len(db.Commands)},
		{Table: "voucher_templates", Rows: len(db.Templates)},
		{Table: "user_logs", Rows: len(db.UserLogs)},
		{Table: "ip_bindings", Rows: len(db.IPBindings)},
		{Table: "scheduler_tasks", Rows: len(db.SchedulerTasks)},
		{Table: "traffic", Rows: len(db.Traffic)},
		{Table: "line_quality", Rows: len(db.LineQuality)},
		{Table: "notif_settings", Rows: len(db.NotifSettings)},
		{Table: "notif_log", Rows: len(db.NotifLog)},
		{Table: "billing_requests", Rows: len(db.BillingRequests)},
		{Table: "purge_tombstones", Rows: len(db.PurgeTombstones)},
		{Table: "join_links", Rows: len(db.JoinLinks)},
		{Table: "registration_requests", Rows: len(db.RegistrationRequests)},
		{Table: "wifi_sites", Rows: len(db.WifiSites)},
		{Table: "wifi_guests", Rows: len(db.WifiGuests)},
		{Table: "promo_events", Rows: len(db.PromoEvents)},
		{Table: "geniuspay_subs", Rows: len(db.GeniusPaySubs)},
		// N°133 — parité avec la synchro différentielle (chat N°127, HomeNet N°101).
		{Table: "chat_conversations", Rows: len(db.ChatConversations)},
		{Table: "chat_messages", Rows: len(db.ChatMessages)},
		{Table: "devices", Rows: len(db.Devices)},
		// N°152 — annonces de la plateforme (parité synchro différentielle).
		{Table: "announcements", Rows: len(db.Announcements)},
		{Table: "settings", Rows: len(db.SettingsByAccount)},
	}
}
