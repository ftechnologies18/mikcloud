// Moteur de synchro différentielle FNV-1a vers PostgreSQL (Sync, tables génériques, upsert/delete).
// Extrait du monolithe pg.go (N°88) — même package, contenu inchangé.
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"mikcloud/hotspot-api/internal/model"
	"strconv"
	"strings"
	"time"
)

// maxRowsPerStatement — bornes d'insertion multi-lignes (limite PostgreSQL : 65535 paramètres).
const maxRowsPerStatement = 200

// syncTimeout — N°74 — borne de durée d'UNE synchronisation Save(). Le
// diagnostic de robustesse a montré que Sync s'exécute SOUS LE VERROU GLOBAL
// du store SANS AUCUN timeout SQL : un Neon gelé (compute en réveil lent,
// partition réseau, transaction bloquée) tenait le mutex indéfiniment —
// toutes les requêtes, y compris le health check Render (GET /), se
// bloquaient → crash-loop du service. BeginTx(ctx) borne TOUTE la durée de
// vie de la transaction (statements + commit) : l'expiration annule la
// transaction en base ET libère l'appelant, qui retente au Save suivant (les
// empreintes ne sont rafraîchies qu'après succès — aucun delta perdu). 20 s =
// ~8× le temps mesuré en production (2,6 s) ; seul un incident réel l'atteint.

// syncTimeout — N°74 — borne de durée d'UNE synchronisation Save(). Le
// diagnostic de robustesse a montré que Sync s'exécute SOUS LE VERROU GLOBAL
// du store SANS AUCUN timeout SQL : un Neon gelé (compute en réveil lent,
// partition réseau, transaction bloquée) tenait le mutex indéfiniment —
// toutes les requêtes, y compris le health check Render (GET /), se
// bloquaient → crash-loop du service. BeginTx(ctx) borne TOUTE la durée de
// vie de la transaction (statements + commit) : l'expiration annule la
// transaction en base ET libère l'appelant, qui retente au Save suivant (les
// empreintes ne sont rafraîchies qu'après succès — aucun delta perdu). 20 s =
// ~8× le temps mesuré en production (2,6 s) ; seul un incident réel l'atteint.
const syncTimeout = 20 * time.Second

// PG — backend PostgreSQL : pool de connexions + empreintes (hash FNV-1a) de la
// dernière synchronisation réussie, par table, pour calculer les différences.

// ---------------------------------------------------------------------------
// Synchronisation différentielle (N°130 : appelée par le syncreur de fond,
// sur un SNAPSHOT CloneDeep, HORS verrou global)
// ---------------------------------------------------------------------------

// Sync compare l'état mémoire aux empreintes de la dernière synchronisation
// réussie et applique les différences en une transaction :
// upserts des lignes nouvelles/modifiées, suppressions des disparues.
//
// N°133 — Save() appelle ce diff COMPLET (les 33 tables différentielles) ;
// les chemins de lecture pollés appellent SyncTables (diff ciblé sur les
// seules tables marquées sales par le moteur, cf. tables.go).
func (p *PG) Sync(db *model.DB) (err error) {
	return p.syncPlan(db, nil)
}

// SyncTables — N°133 — variante CIBLÉE de Sync : seules les tables listées
// sont re-hashées puis différentiellement synchronisées ; les autres
// conservent leurs empreintes (reportées telles quelles dans le cache après
// commit — cf. syncPlan). La table settings reste TOUJOURS écrite : elle est
// hors diff d'empreintes (une ligne par compte, upsert direct) et porte
// last_tick/last_sweep — la fraîcheur du moteur de simulation ne doit pas
// dépendre du ciblage.
//
// Une liste vide déclenche le diff complet (défaut sûr : l'appelant qui
// n'aurait rien su marquer retombe sur le comportement de Sync).
func (p *PG) SyncTables(db *model.DB, tables map[string]bool) (err error) {
	if len(tables) == 0 {
		return p.syncPlan(db, nil)
	}
	return p.syncPlan(db, tables)
}

// syncStep — UNE table de la synchro différentielle, encapsulée dans une
// clôture pour effacer le type concret des lignes (syncTable est générique :
// les méthodes Go ne peuvent pas introduire de paramètres de type — c'est une
// restriction du langage, d'où la liste de clôtures ci-dessous).
type syncStep struct {
	name string
	run  func(ctx context.Context, tx *sql.Tx, hashes, pending map[string]map[string]uint64, delta *syncDelta) error
}

// syncSteps — plan COMPLET de la synchro différentielle (ordre stable,
// identique à l'ancien Sync monolithique). Les tables non marquées par un
// SyncTables sont simplement sautées : leurs empreintes sont reportées.
func syncSteps(db *model.DB) []syncStep {
	return []syncStep{
		{accountSpec.table, func(ctx context.Context, tx *sql.Tx, hashes, pending map[string]map[string]uint64, delta *syncDelta) error {
			return syncTable(ctx, tx, hashes, pending, accountSpec, db.Accounts, delta)
		}},
		{adminSpec.table, func(ctx context.Context, tx *sql.Tx, hashes, pending map[string]map[string]uint64, delta *syncDelta) error {
			return syncTable(ctx, tx, hashes, pending, adminSpec, db.Users, delta)
		}},
		{routerSpec.table, func(ctx context.Context, tx *sql.Tx, hashes, pending map[string]map[string]uint64, delta *syncDelta) error {
			return syncTable(ctx, tx, hashes, pending, routerSpec, db.Routers, delta)
		}},
		{profileSpec.table, func(ctx context.Context, tx *sql.Tx, hashes, pending map[string]map[string]uint64, delta *syncDelta) error {
			return syncTable(ctx, tx, hashes, pending, profileSpec, db.Profiles, delta)
		}},
		{hotspotUserSpec.table, func(ctx context.Context, tx *sql.Tx, hashes, pending map[string]map[string]uint64, delta *syncDelta) error {
			return syncTable(ctx, tx, hashes, pending, hotspotUserSpec, db.HotspotUsers, delta)
		}},
		{batchSpec.table, func(ctx context.Context, tx *sql.Tx, hashes, pending map[string]map[string]uint64, delta *syncDelta) error {
			return syncTable(ctx, tx, hashes, pending, batchSpec, db.Batches, delta)
		}},
		{resellerSpec.table, func(ctx context.Context, tx *sql.Tx, hashes, pending map[string]map[string]uint64, delta *syncDelta) error {
			return syncTable(ctx, tx, hashes, pending, resellerSpec, db.Resellers, delta)
		}},
		{sellSessionSpec.table, func(ctx context.Context, tx *sql.Tx, hashes, pending map[string]map[string]uint64, delta *syncDelta) error {
			return syncTable(ctx, tx, hashes, pending, sellSessionSpec, db.SellSessions, delta)
		}},
		{passwordResetSpec.table, func(ctx context.Context, tx *sql.Tx, hashes, pending map[string]map[string]uint64, delta *syncDelta) error {
			return syncTable(ctx, tx, hashes, pending, passwordResetSpec, db.PasswordResets, delta)
		}},
		{chatConversationSpec.table, func(ctx context.Context, tx *sql.Tx, hashes, pending map[string]map[string]uint64, delta *syncDelta) error {
			return syncTable(ctx, tx, hashes, pending, chatConversationSpec, db.ChatConversations, delta)
		}},
		{chatMessageSpec.table, func(ctx context.Context, tx *sql.Tx, hashes, pending map[string]map[string]uint64, delta *syncDelta) error {
			return syncTable(ctx, tx, hashes, pending, chatMessageSpec, db.ChatMessages, delta)
		}},
		{transactionSpec.table, func(ctx context.Context, tx *sql.Tx, hashes, pending map[string]map[string]uint64, delta *syncDelta) error {
			return syncTable(ctx, tx, hashes, pending, transactionSpec, db.Transactions, delta)
		}},
		{sessionSpec.table, func(ctx context.Context, tx *sql.Tx, hashes, pending map[string]map[string]uint64, delta *syncDelta) error {
			return syncTable(ctx, tx, hashes, pending, sessionSpec, db.Sessions, delta)
		}},
		{deviceSpec.table, func(ctx context.Context, tx *sql.Tx, hashes, pending map[string]map[string]uint64, delta *syncDelta) error {
			return syncTable(ctx, tx, hashes, pending, deviceSpec, db.Devices, delta)
		}},
		{activitySpec.table, func(ctx context.Context, tx *sql.Tx, hashes, pending map[string]map[string]uint64, delta *syncDelta) error {
			return syncTable(ctx, tx, hashes, pending, activitySpec, db.Activity, delta)
		}},
		{saleSpec.table, func(ctx context.Context, tx *sql.Tx, hashes, pending map[string]map[string]uint64, delta *syncDelta) error {
			return syncTable(ctx, tx, hashes, pending, saleSpec, db.Sales, delta)
		}},
		{commandSpec.table, func(ctx context.Context, tx *sql.Tx, hashes, pending map[string]map[string]uint64, delta *syncDelta) error {
			return syncTable(ctx, tx, hashes, pending, commandSpec, db.Commands, delta)
		}},
		{templateSpec.table, func(ctx context.Context, tx *sql.Tx, hashes, pending map[string]map[string]uint64, delta *syncDelta) error {
			return syncTable(ctx, tx, hashes, pending, templateSpec, db.Templates, delta)
		}},
		{userLogSpec.table, func(ctx context.Context, tx *sql.Tx, hashes, pending map[string]map[string]uint64, delta *syncDelta) error {
			return syncTable(ctx, tx, hashes, pending, userLogSpec, db.UserLogs, delta)
		}},
		{ipBindingSpec.table, func(ctx context.Context, tx *sql.Tx, hashes, pending map[string]map[string]uint64, delta *syncDelta) error {
			return syncTable(ctx, tx, hashes, pending, ipBindingSpec, db.IPBindings, delta)
		}},
		{schedulerTaskSpec.table, func(ctx context.Context, tx *sql.Tx, hashes, pending map[string]map[string]uint64, delta *syncDelta) error {
			return syncTable(ctx, tx, hashes, pending, schedulerTaskSpec, db.SchedulerTasks, delta)
		}},
		{trafficSpec.table, func(ctx context.Context, tx *sql.Tx, hashes, pending map[string]map[string]uint64, delta *syncDelta) error {
			return syncTable(ctx, tx, hashes, pending, trafficSpec, db.Traffic, delta)
		}},
		// N°103 — agrégats quotidiens de qualité de ligne (mesure FAI).
		{lineQualitySpec.table, func(ctx context.Context, tx *sql.Tx, hashes, pending map[string]map[string]uint64, delta *syncDelta) error {
			return syncTable(ctx, tx, hashes, pending, lineQualitySpec, db.LineQuality, delta)
		}},
		{notifSettingsSpec.table, func(ctx context.Context, tx *sql.Tx, hashes, pending map[string]map[string]uint64, delta *syncDelta) error {
			notifRows := make([]model.NotificationSettings, 0, len(db.NotifSettings))
			for _, v := range db.NotifSettings {
				notifRows = append(notifRows, v)
			}
			return syncTable(ctx, tx, hashes, pending, notifSettingsSpec, notifRows, delta)
		}},
		{notifLogSpec.table, func(ctx context.Context, tx *sql.Tx, hashes, pending map[string]map[string]uint64, delta *syncDelta) error {
			return syncTable(ctx, tx, hashes, pending, notifLogSpec, db.NotifLog, delta)
		}},
		{billingRequestSpec.table, func(ctx context.Context, tx *sql.Tx, hashes, pending map[string]map[string]uint64, delta *syncDelta) error {
			return syncTable(ctx, tx, hashes, pending, billingRequestSpec, db.BillingRequests, delta)
		}},
		{purgeTombstoneSpec.table, func(ctx context.Context, tx *sql.Tx, hashes, pending map[string]map[string]uint64, delta *syncDelta) error {
			return syncTable(ctx, tx, hashes, pending, purgeTombstoneSpec, db.PurgeTombstones, delta)
		}},
		{joinLinkSpec.table, func(ctx context.Context, tx *sql.Tx, hashes, pending map[string]map[string]uint64, delta *syncDelta) error {
			return syncTable(ctx, tx, hashes, pending, joinLinkSpec, db.JoinLinks, delta)
		}},
		{registrationRequestSpec.table, func(ctx context.Context, tx *sql.Tx, hashes, pending map[string]map[string]uint64, delta *syncDelta) error {
			return syncTable(ctx, tx, hashes, pending, registrationRequestSpec, db.RegistrationRequests, delta)
		}},
		{wifiSiteSpec.table, func(ctx context.Context, tx *sql.Tx, hashes, pending map[string]map[string]uint64, delta *syncDelta) error {
			return syncTable(ctx, tx, hashes, pending, wifiSiteSpec, db.WifiSites, delta)
		}},
		{wifiGuestSpec.table, func(ctx context.Context, tx *sql.Tx, hashes, pending map[string]map[string]uint64, delta *syncDelta) error {
			return syncTable(ctx, tx, hashes, pending, wifiGuestSpec, db.WifiGuests, delta)
		}},
		{promoEventSpec.table, func(ctx context.Context, tx *sql.Tx, hashes, pending map[string]map[string]uint64, delta *syncDelta) error {
			return syncTable(ctx, tx, hashes, pending, promoEventSpec, db.PromoEvents, delta)
		}},
		// N°71 — geniuspay_subs intègre la synchro différentielle : la spec
		// existait (table chargée au boot) mais échappait à Sync — les abonnements
		// carte créés en mémoire (avec a.store.Save() !) disparaissaient donc au
		// redémarrage. La clé primaire « uuid » (≠ « id ») est désormais portée
		// par la machinerie générique (cols[0] = cible ON CONFLICT, cf. upsertRows).
		{geniusPaySubSpec.table, func(ctx context.Context, tx *sql.Tx, hashes, pending map[string]map[string]uint64, delta *syncDelta) error {
			return syncTable(ctx, tx, hashes, pending, geniusPaySubSpec, db.GeniusPaySubs, delta)
		}},
	}
}

// syncPlan — moteur commun de Sync (diff complet) et SyncTables (diff ciblé) :
// UNE transaction (bornée par syncTimeout), les empreintes fraîches posées
// dans « pending » puis basculées APRÈS le Commit (atomicité N°130).
//
// N°133 — mode ciblé (only != nil) : les tables non marquées sont sautées et
// leurs empreintes COURANTES sont reportées dans pending (le swap post-commit
// ne peut donc pas perdre de cache) ; la volumétrie santé (syncDelta) ne
// compte que les tables réellement re-hashées.
func (p *PG) syncPlan(db *model.DB, only map[string]bool) (err error) {
	// N°71 — instrumentation santé : le defer alimente les compteurs exposés
	// par GET /api/admin/sync-status (tentatives/succès/échecs, durée,
	// volumétrie du delta). Aucun verrou supplémentaire sur le chemin
	// critique : les compteurs ont leur micro-verrou (syncstats.go) et le
	// syncreur de fond appelle Sync hors du verrou global du store.
	start := time.Now()
	delta := syncDelta{}
	defer func() {
		if err != nil {
			p.stats.recordFailure(err, time.Since(start))
			return
		}
		p.stats.recordSuccess(delta, time.Since(start))
	}()
	// N°130 — exclusion avec rebuildHashes (boot/Reload) qui réécrit le
	// cache d'empreintes : un seul Sync à la fois (le syncreur de fond est
	// la seule source d'appel en production).
	p.syncMu.Lock()
	defer p.syncMu.Unlock()
	// N°74 — contexte borné : un Neon gelé ne peut plus tenir la
	// synchronisation indéfiniment (cf. syncTimeout) — l'incident se
	// résout en une erreur retournée, retentée par le syncreur.
	ctx, cancel := context.WithTimeout(context.Background(), syncTimeout)
	defer cancel()
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("pg sync (begin) : %w", err)
	}
	defer tx.Rollback() // no-op si Commit réussit

	// N°130 — ATOMICITÉ du cache d'empreintes : les empreintes fraîches
	// sont calculées dans « pending » et ne REMPLACENT p.hashes qu'APRÈS
	// le Commit. L'ancien code rafraîchissait les empreintes table par
	// table AVANT le commit : un échec à mi-parcours (rollback) laissait
	// le cache croire synchronisées des lignes jamais écrites — le diff
	// suivant ne les revoyait plus (perte silencieuse). Avec la sauvegarde
	// asynchrone (retry automatique), ce cas serait devenu un bug actif :
	// le retry repartait d'un cache pollué. Pending→commit→swap garantit
	// qu'un échec retente les VRAIES différences.
	pending := make(map[string]map[string]uint64, len(p.hashes)+4)
	if only != nil {
		// N°133 — ciblé : les tables non marquées conservent leurs
		// empreintes (aucun re-hash, aucune écriture pour elles).
		for t, h := range p.hashes {
			pending[t] = h
		}
	}
	for _, st := range syncSteps(db) {
		if only != nil && !only[st.name] {
			continue
		}
		if err := st.run(ctx, tx, p.hashes, pending, &delta); err != nil {
			return err
		}
	}
	if err := p.syncSettings(ctx, tx, db); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("pg sync (commit) : %w", err)
	}
	// Écriture confirmée : le cache d'empreintes bascule ATOMIQUEMENT sur
	// les empreintes de CETTE synchronisation (cf. pending ci-dessus).
	p.hashes = pending
	p.touchDB() // écriture confirmée — le keep-alive saute ses pings inutiles
	return nil
}

// syncSettings écrit une ligne par compte de SettingsByAccount (upsert par
// id = account_id) et supprime les lignes orphelines : une ligne settings
// suit le cycle de vie de son compte (suppression de compte client, retrait
// du compte principal…). last_tick (valeur globale du moteur de simulation)
// est répliquée sur chaque ligne.
//
// N°130 — l'élagage mémoire des réglages orphelins (ci-dessous) s'applique
// au SNAPSHOT : la synchro travaille sur une photographie CloneDeep, l'état
// vivant garde ces entrées en mémoire (charge négligeable — quelques structs
// par compte disparu, re-purgés au prochain boot via loadSettings) tandis
// que PostgreSQL reste correctement nettoyé.
func (p *PG) syncSettings(ctx context.Context, tx *sql.Tx, db *model.DB) error {
	accExists := map[string]bool{}
	for i := range db.Accounts {
		accExists[db.Accounts[i].ID] = true
	}
	// Le compte principal est TOUJOURS préservé : il porte la config
	// plateforme (Settings.Platform — nom du SaaS, inscriptions) même s'il
	// n'a pas de ligne dans la table accounts (base de mise en service vide).
	accExists[model.AccountMainID] = true
	// Élagage mémoire : les réglages d'un compte disparu (rechargés au boot
	// par loadSettings depuis des lignes orphelines) sont retirés de l'état —
	// l'upsert ci-dessous ne doit PAS les réécrire.
	for accID := range db.SettingsByAccount {
		if !accExists[accID] {
			delete(db.SettingsByAccount, accID)
		}
	}
	// Orphelins : toute ligne dont le compte n'existe PLUS dans l'état
	// mémoire (table accounts) est supprimée — une ligne settings suit le
	// cycle de vie de son compte (suppression de compte client, retrait du
	// compte principal…).
	rows, err := tx.QueryContext(ctx, `SELECT DISTINCT account_id FROM settings WHERE account_id <> ''`)
	if err != nil {
		return fmt.Errorf("pg sync settings (lecture orphelins) : %w", err)
	}
	present := []string{}
	for rows.Next() {
		var acc string
		if err := rows.Scan(&acc); err != nil {
			rows.Close()
			return fmt.Errorf("pg sync settings (scan orphelins) : %w", err)
		}
		present = append(present, acc)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("pg sync settings (orphelins) : %w", err)
	}
	for _, acc := range present {
		if !accExists[acc] {
			if _, err := tx.ExecContext(ctx, `DELETE FROM settings WHERE account_id = $1`, acc); err != nil {
				return fmt.Errorf("pg sync settings (suppression orphelin %s) : %w", acc, err)
			}
		}
	}

	lastTick := sql.NullTime{Time: db.LastTick, Valid: !db.LastTick.IsZero()}
	// N°64 — date du balayage périodique, même logique de persistance.
	lastSweep := sql.NullTime{Time: db.LastSweep, Valid: !db.LastSweep.IsZero()}
	for accID, s := range db.SettingsByAccount {
		// I (paramètres plateforme) — la config globale ne vit que sur le
		// compte principal ; les autres lignes écrivent les valeurs neutres.
		var platName string
		var platOpen bool
		var platKey string
		if s.Platform != nil {
			platName, platOpen, platKey = s.Platform.Name, s.Platform.RegisterOpen, s.Platform.RegisterKey
		}
		_, err := tx.ExecContext(ctx,
			`INSERT INTO settings (id, account_id, tenant_name, tenant_currency, tenant_timezone, plan_name, plan_max_routers, plan_max_users, wave_link,
                               dns_name, logo_url, banner_url, expiry_policy_mode, expiry_policy_after_days,
                               sub_plan_id, sub_status, sub_period_start, sub_period_end, sub_last_amount,
                               sub_router_slots, sub_last_paid_at, last_tick, last_sweep,
                               platform_name, platform_register_open, platform_register_key, auto_import_router_users, join_button,
                               portal_style, portal_welcome, portal_promos, portal_socials, portal_key,
                               log_retention_days)
                         VALUES ($1, $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28, $29, $30, $31, $32, $33)
                         ON CONFLICT (id) DO UPDATE SET
                           account_id                = EXCLUDED.account_id,
                           tenant_name               = EXCLUDED.tenant_name,
                           tenant_currency           = EXCLUDED.tenant_currency,
                           tenant_timezone           = EXCLUDED.tenant_timezone,
                           plan_name                 = EXCLUDED.plan_name,
                           plan_max_routers          = EXCLUDED.plan_max_routers,
                           plan_max_users            = EXCLUDED.plan_max_users,
                           wave_link                 = EXCLUDED.wave_link,
                           dns_name                  = EXCLUDED.dns_name,
                           logo_url                  = EXCLUDED.logo_url,
                           banner_url                = EXCLUDED.banner_url,
                           expiry_policy_mode        = EXCLUDED.expiry_policy_mode,
                           expiry_policy_after_days  = EXCLUDED.expiry_policy_after_days,
                           sub_plan_id               = EXCLUDED.sub_plan_id,
                           sub_status                = EXCLUDED.sub_status,
                           sub_period_start          = EXCLUDED.sub_period_start,
                           sub_period_end            = EXCLUDED.sub_period_end,
                           sub_last_amount           = EXCLUDED.sub_last_amount,
                           sub_router_slots          = EXCLUDED.sub_router_slots,
                           sub_last_paid_at          = EXCLUDED.sub_last_paid_at,
                           last_tick                 = EXCLUDED.last_tick,
                           last_sweep                = EXCLUDED.last_sweep,
                           platform_name             = EXCLUDED.platform_name,
                           platform_register_open    = EXCLUDED.platform_register_open,
                           platform_register_key    = EXCLUDED.platform_register_key,
                           auto_import_router_users = EXCLUDED.auto_import_router_users,
                           join_button              = EXCLUDED.join_button,
                           portal_style             = EXCLUDED.portal_style,
                           portal_welcome           = EXCLUDED.portal_welcome,
                           portal_promos            = EXCLUDED.portal_promos,
                           portal_socials           = EXCLUDED.portal_socials,
                           portal_key               = EXCLUDED.portal_key,
                           log_retention_days       = EXCLUDED.log_retention_days`,
			accID, s.Tenant.Name, s.Tenant.Currency, s.Tenant.Timezone,
			s.Plan.Name, s.Plan.MaxRouters, s.Plan.MaxUsers,
			s.Tenant.WaveLink, s.Tenant.DNSName, s.Tenant.LogoURL, s.Tenant.BannerURL,
			s.Tenant.ExpiryPolicyMode, s.Tenant.ExpiryPolicyAfterDays,
			s.Subscription.PlanID, s.Subscription.Status, s.Subscription.PeriodStart,
			s.Subscription.PeriodEnd, s.Subscription.LastAmountFcfa,
			s.Subscription.RouterSlots, s.Subscription.LastPaidAt, lastTick, lastSweep,
			platName, platOpen, platKey, s.ImportAutoEnabled(), s.Tenant.JoinButtonEnabled(),
			s.Tenant.PortalStyle, s.Tenant.PortalWelcome, s.Tenant.PortalPromos, s.Tenant.PortalSocials,
			s.Tenant.PortalKey, s.Tenant.LogRetentionDaysEffective())
		if err != nil {
			return fmt.Errorf("pg sync settings (%s) : %w", accID, err)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Outillage générique : specs de tables, diff, upserts multi-lignes
// (fonctions libres : les méthodes Go ne peuvent pas introduire de paramètres
// de type — c'est une restriction du langage)
// ---------------------------------------------------------------------------

// entitySpec — description d'une table : colonnes (cols[0] est TOUJOURS la
// clé primaire — « id » partout, « uuid » pour geniuspay_subs), extraction
// d'id, lecture et écriture d'une ligne.

// ---------------------------------------------------------------------------
// Outillage générique : specs de tables, diff, upserts multi-lignes
// (fonctions libres : les méthodes Go ne peuvent pas introduire de paramètres
// de type — c'est une restriction du langage)
// ---------------------------------------------------------------------------

// entitySpec — description d'une table : colonnes (cols[0] est TOUJOURS la
// clé primaire — « id » partout, « uuid » pour geniuspay_subs), extraction
// d'id, lecture et écriture d'une ligne.
type entitySpec[T any] struct {
	table  string
	cols   []string
	idOf   func(*T) string
	scan   func(*sql.Rows) (T, error)
	args   func(*T) []any
	hashOf func(*T) uint64
}

// loadInto — SELECT des colonnes explicites → tranche typée.

// loadInto — SELECT des colonnes explicites → tranche typée.
func loadInto[T any](p *PG, out *[]T, spec entitySpec[T]) error {
	rows, err := p.db.Query(`SELECT ` + strings.Join(spec.cols, ", ") + ` FROM ` + spec.table)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		v, err := spec.scan(rows)
		if err != nil {
			return err
		}
		*out = append(*out, v)
	}
	return rows.Err()
}

// syncTable — différentiel : détecte ajouts/modifications (comparaison
// d'empreintes) et disparitions (id absents), applique le tout. N°130 :
// les empreintes fraîches sont posées dans « pending » (et non plus
// directement dans le cache) — Sync ne bascule le cache qu'après le Commit
// (un échec sera retenté sur les VRAIES différences).
func syncTable[T any](ctx context.Context, tx *sql.Tx, hashes, pending map[string]map[string]uint64, spec entitySpec[T], rows []T, delta *syncDelta) error {
	cached := hashes[spec.table]
	if cached == nil {
		cached = map[string]uint64{}
	}

	seen := make(map[string]struct{}, len(rows))
	// N°78-bis — les empreintes calculées pour la détection de changements
	// sont RÉUTILISÉES pour le rafraîchissement du cache : l'ancien code
	// re-marshalait chaque ligne une 2ᵉ fois (json.Marshal + FNV) après les
	// écritures — sur le 0,1 vCPU Render, chaque synchro payait deux fois le
	// prix d'un parc de 3 500+ utilisateurs hotspot (~2,8 s → ~1,4 s).
	fresh := make(map[string]uint64, len(rows))
	var changed []T
	for i := range rows {
		id := spec.idOf(&rows[i])
		seen[id] = struct{}{}
		h := spec.hashOf(&rows[i])
		fresh[id] = h
		if old, ok := cached[id]; !ok || old != h {
			changed = append(changed, rows[i])
		}
	}
	var removed []string
	for id := range cached {
		if _, ok := seen[id]; !ok {
			removed = append(removed, id)
		}
	}

	if len(changed) > 0 {
		if err := upsertRows(ctx, tx, spec, changed); err != nil {
			return err
		}
		delta.changed += len(changed) // N°71 — volumétrie (comptée si écrite)
	}
	if len(removed) > 0 {
		if err := deleteRows(ctx, tx, spec.table, spec.cols[0], removed); err != nil {
			return err
		}
		delta.removed += len(removed) // N°71 — volumétrie (comptée si écrite)
	}

	// Empreintes de CETTE table prêtes pour le commit — Sync les basculera
	// dans p.hashes uniquement si la transaction entière passe.
	pending[spec.table] = fresh
	return nil
}

// hashEntity — empreinte FNV-1a de la sérialisation JSON (l'ordre des champs
// d'une struct est stable en Go → déterministe).

// hashEntity — empreinte FNV-1a de la sérialisation JSON (l'ordre des champs
// d'une struct est stable en Go → déterministe).
func hashEntity[T any](v *T) uint64 {
	b, err := json.Marshal(v)
	if err != nil {
		return 0
	}
	h := fnv.New64a()
	h.Write(b)
	return h.Sum64()
}

// deleteRows — DELETE ... WHERE <clé> IN (…) par blocs de 500. La clé est
// cols[0] de la spec (« id » partout, « uuid » pour geniuspay_subs — N°71).

// deleteRows — DELETE ... WHERE <clé> IN (…) par blocs de 500. La clé est
// cols[0] de la spec (« id » partout, « uuid » pour geniuspay_subs — N°71).
func deleteRows(ctx context.Context, tx *sql.Tx, table, key string, ids []string) error {
	for start := 0; start < len(ids); start += 500 {
		end := min(start+500, len(ids))
		chunk := ids[start:end]
		ph := make([]string, len(chunk))
		args := make([]any, len(chunk))
		for i, id := range chunk {
			ph[i] = "$" + strconv.Itoa(i+1)
			args[i] = id
		}
		q := `DELETE FROM ` + table + ` WHERE ` + key + ` IN (` + strings.Join(ph, ",") + `)`
		if _, err := tx.ExecContext(ctx, q, args...); err != nil {
			return fmt.Errorf("suppression %s : %w", table, err)
		}
	}
	return nil
}

// upsertRows — INSERT ... ON CONFLICT (id) DO UPDATE par blocs multi-lignes
// (une seule requête pour jusqu'à 200 lignes → un seul aller-retour réseau).

// upsertRows — INSERT ... ON CONFLICT (id) DO UPDATE par blocs multi-lignes
// (une seule requête pour jusqu'à 200 lignes → un seul aller-retour réseau).
func upsertRows[T any](ctx context.Context, tx *sql.Tx, spec entitySpec[T], rows []T) error {
	n := len(spec.cols)
	// Clause SET de l'upsert (toutes les colonnes sauf la clé).
	sets := make([]string, 0, n-1)
	for _, c := range spec.cols[1:] {
		sets = append(sets, c+` = EXCLUDED.`+c)
	}
	setClause := strings.Join(sets, ", ")

	for start := 0; start < len(rows); start += maxRowsPerStatement {
		end := min(start+maxRowsPerStatement, len(rows))
		chunk := rows[start:end]

		var sb strings.Builder
		args := make([]any, 0, len(chunk)*n)
		k := 0
		sb.WriteString(`INSERT INTO ` + spec.table + ` (` + strings.Join(spec.cols, ", ") + `) VALUES `)
		for i := range chunk {
			if i > 0 {
				sb.WriteByte(',')
			}
			sb.WriteByte('(')
			for j := 0; j < n; j++ {
				if j > 0 {
					sb.WriteByte(',')
				}
				k++
				sb.WriteString("$" + strconv.Itoa(k))
			}
			sb.WriteByte(')')
			args = append(args, spec.args(&chunk[i])...)
		}
		// N°71 — cible de conflit = cols[0] (clé primaire : « id » pour 28
		// tables, « uuid » pour geniuspay_subs) au lieu du « id » en dur.
		sb.WriteString(` ON CONFLICT (` + spec.cols[0] + `) DO UPDATE SET ` + setClause)

		if _, err := tx.ExecContext(ctx, sb.String(), args...); err != nil {
			return fmt.Errorf("upsert %s : %w", spec.table, err)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Specs concrètes des 10 tables entité.
// IMPORTANT : l'ordre des colonnes doit rester strictement identique entre
// cols, scan et args.
// ---------------------------------------------------------------------------

// accountSpec — comptes clients SaaS (isolation multi-tenant).

// rebuildHashes — reconstruit le cache d'empreintes à partir d'un état mémoire
// (après un Load ou un seed initial). N°130 : sous syncMu — un Reload (admin)
// peut recalibrer le cache pendant que le syncreur de fond synchronise l'état
// précédent ; l'exclusion évite la course sur p.hashes.
func (p *PG) rebuildHashes(db *model.DB) {
	p.syncMu.Lock()
	defer p.syncMu.Unlock()
	p.hashes = map[string]map[string]uint64{
		accountSpec.table:             hashRows(db.Accounts, accountSpec),
		adminSpec.table:               hashRows(db.Users, adminSpec),
		routerSpec.table:              hashRows(db.Routers, routerSpec),
		profileSpec.table:             hashRows(db.Profiles, profileSpec),
		hotspotUserSpec.table:         hashRows(db.HotspotUsers, hotspotUserSpec),
		batchSpec.table:               hashRows(db.Batches, batchSpec),
		resellerSpec.table:            hashRows(db.Resellers, resellerSpec),
		sellSessionSpec.table:         hashRows(db.SellSessions, sellSessionSpec),
		passwordResetSpec.table:       hashRows(db.PasswordResets, passwordResetSpec),
		transactionSpec.table:         hashRows(db.Transactions, transactionSpec),
		sessionSpec.table:             hashRows(db.Sessions, sessionSpec),
		activitySpec.table:            hashRows(db.Activity, activitySpec),
		saleSpec.table:                hashRows(db.Sales, saleSpec),
		commandSpec.table:             hashRows(db.Commands, commandSpec),
		templateSpec.table:            hashRows(db.Templates, templateSpec),
		userLogSpec.table:             hashRows(db.UserLogs, userLogSpec),
		ipBindingSpec.table:           hashRows(db.IPBindings, ipBindingSpec),
		schedulerTaskSpec.table:       hashRows(db.SchedulerTasks, schedulerTaskSpec),
		trafficSpec.table:             hashRows(db.Traffic, trafficSpec),
		lineQualitySpec.table:         hashRows(db.LineQuality, lineQualitySpec),
		notifLogSpec.table:            hashRows(db.NotifLog, notifLogSpec),
		billingRequestSpec.table:      hashRows(db.BillingRequests, billingRequestSpec),
		purgeTombstoneSpec.table:      hashRows(db.PurgeTombstones, purgeTombstoneSpec),
		joinLinkSpec.table:            hashRows(db.JoinLinks, joinLinkSpec),
		registrationRequestSpec.table: hashRows(db.RegistrationRequests, registrationRequestSpec),
		wifiSiteSpec.table:            hashRows(db.WifiSites, wifiSiteSpec),
		wifiGuestSpec.table:           hashRows(db.WifiGuests, wifiGuestSpec),
		promoEventSpec.table:          hashRows(db.PromoEvents, promoEventSpec),
		geniusPaySubSpec.table:        hashRows(db.GeniusPaySubs, geniusPaySubSpec),
		// N°133 — parité avec Sync : les tables chat et devices manquaient au
		// cache reconstruit au boot — le premier flush les considérait
		// intégralement « changées » (empreintes absentes) et re-upsertait
		// toutes leurs lignes pour rien. Elles sont chargées au boot comme
		// les autres : leurs empreintes sont posées comme les autres.
		chatConversationSpec.table: hashRows(db.ChatConversations, chatConversationSpec),
		chatMessageSpec.table:      hashRows(db.ChatMessages, chatMessageSpec),
		deviceSpec.table:           hashRows(db.Devices, deviceSpec),
	}
	notifRows := make([]model.NotificationSettings, 0, len(db.NotifSettings))
	for _, v := range db.NotifSettings {
		notifRows = append(notifRows, v)
	}
	p.hashes[notifSettingsSpec.table] = hashRows(notifRows, notifSettingsSpec)
}

// hashRows — empreintes indexées par id.

// hashRows — empreintes indexées par id.
func hashRows[T any](rows []T, spec entitySpec[T]) map[string]uint64 {
	m := make(map[string]uint64, len(rows))
	for i := range rows {
		m[spec.idOf(&rows[i])] = spec.hashOf(&rows[i])
	}
	return m
}
