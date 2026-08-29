// Persistance PostgreSQL (Neon) — schéma relationnel + synchronisation différentielle.
//
// Architecture : la mémoire reste le moteur de calcul (les handlers continuent
// de travailler sur *model.DB sous verrou), PostgreSQL devient la source de
// vérité durable entre redémarrages. À chaque Save(), seules les lignes
// ajoutées, modifiées ou supprimées sont écrites (une transaction par sauvegarde),
// ce qui reste léger même avec le polling 5 s du tableau de bord.
package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"log"
	"strconv"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // pilote « pgx » pour database/sql

	"mikcloud/hotspot-api/internal/model"
)

// maxRowsPerStatement — bornes d'insertion multi-lignes (limite PostgreSQL : 65535 paramètres).
const maxRowsPerStatement = 200

// PG — backend PostgreSQL : pool de connexions + empreintes (hash FNV-1a) de la
// dernière synchronisation réussie, par table, pour calculer les différences.
type PG struct {
	db     *sql.DB
	hashes map[string]map[string]uint64
}

// OpenPG ouvre le pool, attend que la base réponde (cold start Neon) et crée le
// schéma si nécessaire (DDL idempotent).
func OpenPG(databaseURL string) (*PG, error) {
	// Neon exige TLS ; on ajoute sslmode=require si l'URL n'en précise pas.
	if !strings.Contains(databaseURL, "sslmode=") {
		sep := "?"
		if strings.Contains(databaseURL, "?") {
			sep = "&"
		}
		databaseURL += sep + "sslmode=require"
	}

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("ouverture PostgreSQL : %w", err)
	}
	// Réglages adaptés à Neon (compute suspendu après inactivité) :
	// peu de connexions, recyclées avant que Neon ne les coupe.
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(2)
	db.SetConnMaxIdleTime(3 * time.Minute)
	db.SetConnMaxLifetime(30 * time.Minute)

	// Ping avec retry : le premier réveil d'une base Neon peut prendre quelques secondes.
	var lastErr error
	for attempt := 1; attempt <= 10; attempt++ {
		if lastErr = db.Ping(); lastErr == nil {
			break
		}
		log.Printf("pg: tentative %d/10 — base pas encore prête (%v), nouvelle tentative dans 1 s…", attempt, lastErr)
		time.Sleep(time.Second)
	}
	if lastErr != nil {
		db.Close()
		return nil, fmt.Errorf("PostgreSQL injoignable : %w", lastErr)
	}

	p := &PG{db: db, hashes: map[string]map[string]uint64{}}
	if err := p.ensureSchema(); err != nil {
		db.Close()
		return nil, err
	}
	return p, nil
}

// Close ferme le pool.
func (p *PG) Close() error { return p.db.Close() }

// ---------------------------------------------------------------------------
// Schéma (idempotent)
// ---------------------------------------------------------------------------

// ensureSchema crée les tables et index si absents. Les horodatages sont
// conservés en TEXT (RFC3339) pour garantir un aller-retour strictement
// identique avec le modèle Go ; last_tick seul est en TIMESTAMPTZ (time.Time).
func (p *PG) ensureSchema() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS admin_users (
                        id            TEXT PRIMARY KEY,
                        name          TEXT NOT NULL,
                        username      TEXT NOT NULL,
                        role          TEXT NOT NULL,
                        password_hash TEXT NOT NULL,
                        salt          TEXT NOT NULL,
                        created_at    TEXT NOT NULL
                )`,
		`CREATE TABLE IF NOT EXISTS routers (
                        id              TEXT PRIMARY KEY,
                        name            TEXT NOT NULL,
                        host            TEXT NOT NULL,
                        port            INTEGER NOT NULL,
                        username        TEXT NOT NULL,
                        password        TEXT NOT NULL,
                        mode            TEXT NOT NULL,
                        status          TEXT NOT NULL,
                        version         TEXT NOT NULL,
                        uptime_sec      BIGINT NOT NULL,
                        cpu_load        INTEGER NOT NULL,
                        hotspot_users   INTEGER NOT NULL,
                        active_sessions INTEGER NOT NULL,
                        created_at      TEXT NOT NULL,
                        agent_token_hash TEXT NOT NULL DEFAULT '',
                        token_preview   TEXT NOT NULL DEFAULT '',
                        last_seen       TEXT NOT NULL DEFAULT ''
                )`,
		`CREATE TABLE IF NOT EXISTS profiles (
                        id                  TEXT PRIMARY KEY,
                        name                TEXT NOT NULL,
                        rate_limit          TEXT NOT NULL,
                        session_timeout_min INTEGER NOT NULL,
                        shared_users        INTEGER NOT NULL,
                        validity_days       INTEGER NOT NULL,
                        price               INTEGER NOT NULL,
                        data_quota_mb       INTEGER NOT NULL,
                        created_at          TEXT NOT NULL
                )`,
		`CREATE TABLE IF NOT EXISTS hotspot_users (
                        id              TEXT PRIMARY KEY,
                        kind            TEXT NOT NULL,
                        username        TEXT NOT NULL,
                        password        TEXT NOT NULL,
                        profile_id      TEXT NOT NULL,
                        profile_name    TEXT NOT NULL,
                        router_id       TEXT NOT NULL,
                        router_name     TEXT NOT NULL,
                        status          TEXT NOT NULL,
                        batch_id        TEXT NOT NULL,
                        reseller_id     TEXT NOT NULL,
                        reseller_name   TEXT NOT NULL,
                        comment         TEXT NOT NULL,
                        bytes_in        BIGINT NOT NULL,
                        bytes_out       BIGINT NOT NULL,
                        uptime_used_sec BIGINT NOT NULL,
                        created_at      TEXT NOT NULL,
                        expires_at      TEXT NOT NULL,
                        used_at         TEXT NOT NULL,
                        price           INTEGER NOT NULL,
                        data_quota_mb   BIGINT NOT NULL DEFAULT 0
                )`,
		`CREATE INDEX IF NOT EXISTS idx_hotspot_users_batch    ON hotspot_users (batch_id)`,
		`CREATE INDEX IF NOT EXISTS idx_hotspot_users_router   ON hotspot_users (router_id)`,
		`CREATE INDEX IF NOT EXISTS idx_hotspot_users_reseller ON hotspot_users (reseller_id)`,
		`CREATE INDEX IF NOT EXISTS idx_hotspot_users_status   ON hotspot_users (status)`,
		`CREATE INDEX IF NOT EXISTS idx_hotspot_users_created  ON hotspot_users (created_at)`,
		`CREATE TABLE IF NOT EXISTS batches (
                        id            TEXT PRIMARY KEY,
                        profile_id    TEXT NOT NULL,
                        profile_name  TEXT NOT NULL,
                        router_id     TEXT NOT NULL,
                        router_name   TEXT NOT NULL,
                        count         INTEGER NOT NULL,
                        unit_price    INTEGER NOT NULL,
                        total_cost    INTEGER NOT NULL,
                        data_quota_mb BIGINT NOT NULL DEFAULT 0,
                        channel       TEXT NOT NULL,
                        reseller_id   TEXT NOT NULL,
                        reseller_name TEXT NOT NULL,
                        created_at    TEXT NOT NULL
                )`,
		`CREATE INDEX IF NOT EXISTS idx_batches_router  ON batches (router_id)`,
		`CREATE INDEX IF NOT EXISTS idx_batches_created ON batches (created_at)`,
		`CREATE TABLE IF NOT EXISTS resellers (
                        id            TEXT PRIMARY KEY,
                        name          TEXT NOT NULL,
                        username      TEXT NOT NULL,
                        phone         TEXT NOT NULL,
                        credit        INTEGER NOT NULL,
                        vouchers_sold INTEGER NOT NULL,
                        revenue       INTEGER NOT NULL,
                        status        TEXT NOT NULL,
                        created_at    TEXT NOT NULL
                )`,
		`CREATE TABLE IF NOT EXISTS transactions (
                        id            TEXT PRIMARY KEY,
                        type          TEXT NOT NULL,
                        reseller_id   TEXT NOT NULL,
                        reseller_name TEXT NOT NULL,
                        amount        INTEGER NOT NULL,
                        note          TEXT NOT NULL,
                        at            TEXT NOT NULL
                )`,
		`CREATE INDEX IF NOT EXISTS idx_transactions_reseller ON transactions (reseller_id)`,
		`CREATE INDEX IF NOT EXISTS idx_transactions_at       ON transactions (at)`,
		`CREATE TABLE IF NOT EXISTS sessions (
                        id           TEXT PRIMARY KEY,
                        user_id      TEXT NOT NULL,
                        username     TEXT NOT NULL,
                        profile_name TEXT NOT NULL,
                        router_id    TEXT NOT NULL,
                        router_name  TEXT NOT NULL,
                        ip           TEXT NOT NULL,
                        mac          TEXT NOT NULL,
                        started_at   TEXT NOT NULL,
                        uptime_sec   BIGINT NOT NULL,
                        bytes_in     BIGINT NOT NULL,
                        bytes_out    BIGINT NOT NULL
                )`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_router ON sessions (router_id)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_user   ON sessions (user_id)`,
		`CREATE TABLE IF NOT EXISTS activity (
                        id      TEXT PRIMARY KEY,
                        type    TEXT NOT NULL,
                        message TEXT NOT NULL,
                        at      TEXT NOT NULL
                )`,
		`CREATE INDEX IF NOT EXISTS idx_activity_at ON activity (at)`,
		`CREATE TABLE IF NOT EXISTS sales (
                        id            TEXT PRIMARY KEY,
                        amount        INTEGER NOT NULL,
                        profile_name  TEXT NOT NULL,
                        count         INTEGER NOT NULL,
                        channel       TEXT NOT NULL,
                        reseller_name TEXT NOT NULL,
                        router_id     TEXT NOT NULL,
                        router_name   TEXT NOT NULL,
                        batch_id      TEXT NOT NULL,
                        at            TEXT NOT NULL
                )`,
		`CREATE INDEX IF NOT EXISTS idx_sales_at     ON sales (at)`,
		`CREATE INDEX IF NOT EXISTS idx_sales_router ON sales (router_id)`,
		`CREATE TABLE IF NOT EXISTS commands (
                        id         TEXT PRIMARY KEY,
                        router_id  TEXT NOT NULL,
                        kind       TEXT NOT NULL,
                        payload    TEXT NOT NULL DEFAULT '', -- JSON sérialisé ('' = absent)
                        status     TEXT NOT NULL,
                        result     TEXT NOT NULL DEFAULT '', -- JSON sérialisé ('' = absent)
                        created_at TEXT NOT NULL,
                        sent_at    TEXT NOT NULL DEFAULT '',
                        done_at    TEXT NOT NULL DEFAULT ''
                )`,
		`CREATE INDEX IF NOT EXISTS idx_commands_router  ON commands (router_id)`,
		`CREATE INDEX IF NOT EXISTS idx_commands_status  ON commands (status)`,
		`CREATE TABLE IF NOT EXISTS accounts (
                        id         TEXT PRIMARY KEY,
                        name       TEXT NOT NULL,
                        status     TEXT NOT NULL DEFAULT 'active',
                        created_at TEXT NOT NULL
                )`,
		`CREATE INDEX IF NOT EXISTS idx_accounts_status ON accounts (status)`,
		// P0/P1 (audit Mikhmon) — nouvelles collections.
		`CREATE TABLE IF NOT EXISTS voucher_templates (
                        id         TEXT PRIMARY KEY,
                        account_id TEXT NOT NULL DEFAULT '',
                        name       TEXT NOT NULL,
                        format     TEXT NOT NULL,
                        body_html  TEXT NOT NULL,
                        is_default BOOLEAN NOT NULL DEFAULT FALSE,
                        created_at TEXT NOT NULL
                )`,
		`CREATE INDEX IF NOT EXISTS idx_voucher_templates_account ON voucher_templates (account_id)`,
		`CREATE TABLE IF NOT EXISTS user_logs (
                        id          TEXT PRIMARY KEY,
                        account_id  TEXT NOT NULL DEFAULT '',
                        user_id     TEXT NOT NULL,
                        username    TEXT NOT NULL,
                        action      TEXT NOT NULL,
                        router_id   TEXT NOT NULL,
                        router_name TEXT NOT NULL,
                        ip          TEXT NOT NULL,
                        mac         TEXT NOT NULL,
                        at          TEXT NOT NULL
                )`,
		`CREATE INDEX IF NOT EXISTS idx_user_logs_account ON user_logs (account_id)`,
		`CREATE INDEX IF NOT EXISTS idx_user_logs_at      ON user_logs (at)`,
		`CREATE TABLE IF NOT EXISTS ip_bindings (
                        id         TEXT PRIMARY KEY,
                        account_id TEXT NOT NULL DEFAULT '',
                        router_id  TEXT NOT NULL,
                        mac        TEXT NOT NULL,
                        address    TEXT NOT NULL,
                        comment    TEXT NOT NULL,
                        type       TEXT NOT NULL,
                        disabled   BOOLEAN NOT NULL DEFAULT FALSE,
                        created_at TEXT NOT NULL
                )`,
		`CREATE INDEX IF NOT EXISTS idx_ip_bindings_account ON ip_bindings (account_id)`,
		`CREATE INDEX IF NOT EXISTS idx_ip_bindings_router  ON ip_bindings (router_id)`,
		`CREATE TABLE IF NOT EXISTS scheduler_tasks (
                        id         TEXT PRIMARY KEY,
                        account_id TEXT NOT NULL DEFAULT '',
                        router_id  TEXT NOT NULL,
                        name       TEXT NOT NULL,
                        interval   TEXT NOT NULL,
                        on_event   TEXT NOT NULL,
                        disabled   BOOLEAN NOT NULL DEFAULT FALSE,
                        created_at TEXT NOT NULL
                )`,
		`CREATE INDEX IF NOT EXISTS idx_scheduler_tasks_account ON scheduler_tasks (account_id)`,
		`CREATE INDEX IF NOT EXISTS idx_scheduler_tasks_router  ON scheduler_tasks (router_id)`,
		// F6 : une ligne par routeur (id = router_id), interfaces/historique en JSON.
		`CREATE TABLE IF NOT EXISTS traffic (
                        id         TEXT PRIMARY KEY, -- = router_id
                        account_id TEXT NOT NULL DEFAULT '',
                        router_id  TEXT NOT NULL,
                        updated_at TEXT NOT NULL,
                        interfaces TEXT NOT NULL DEFAULT '[]', -- JSON []IfaceTraffic
                        history    TEXT NOT NULL DEFAULT '[]'  -- JSON []TrafficPoint
                )`,
		`CREATE INDEX IF NOT EXISTS idx_traffic_account ON traffic (account_id)`,
		`CREATE TABLE IF NOT EXISTS settings (
                        id               TEXT PRIMARY KEY, -- = account_id : une ligne par compte SaaS
                        account_id       TEXT NOT NULL DEFAULT '',
                        tenant_name      TEXT NOT NULL,
                        tenant_currency  TEXT NOT NULL,
                        tenant_timezone  TEXT NOT NULL,
                        plan_name        TEXT NOT NULL,
                        plan_max_routers TEXT NOT NULL,
                        plan_max_users   TEXT NOT NULL,
                        wave_link        TEXT NOT NULL DEFAULT '',
                        last_tick        TIMESTAMPTZ
                )`,
		`CREATE TABLE IF NOT EXISTS notif_settings (
                        id                  TEXT PRIMARY KEY, -- = account_id : une ligne par compte SaaS
                        enabled             BOOLEAN NOT NULL DEFAULT FALSE,
                        telegram_enabled    BOOLEAN NOT NULL DEFAULT FALSE,
                        telegram_bot_token  TEXT NOT NULL DEFAULT '',
                        telegram_chat_id    TEXT NOT NULL DEFAULT '',
                        whatsapp_enabled    BOOLEAN NOT NULL DEFAULT FALSE,
                        whatsapp_token      TEXT NOT NULL DEFAULT '',
                        whatsapp_phone_id   TEXT NOT NULL DEFAULT '',
                        whatsapp_to         TEXT NOT NULL DEFAULT '',
                        email_enabled       BOOLEAN NOT NULL DEFAULT FALSE,
                        smtp_host           TEXT NOT NULL DEFAULT '',
                        smtp_port           INTEGER NOT NULL DEFAULT 0,
                        smtp_user           TEXT NOT NULL DEFAULT '',
                        smtp_pass           TEXT NOT NULL DEFAULT '',
                        email_to            TEXT NOT NULL DEFAULT '',
                        offline_after_sec   INTEGER NOT NULL DEFAULT 135,
                        low_stock_threshold INTEGER NOT NULL DEFAULT 25,
                        daily_report        BOOLEAN NOT NULL DEFAULT FALSE,
                        report_hour         INTEGER NOT NULL DEFAULT 20,
                        last_report_date    TEXT NOT NULL DEFAULT '',
                        stock_alert_state   TEXT NOT NULL DEFAULT '', -- JSON sérialisé ('' = absent)
                        account_id          TEXT NOT NULL DEFAULT ''
                )`,
		`CREATE TABLE IF NOT EXISTS notif_log (
                        id         TEXT PRIMARY KEY,
                        channel    TEXT NOT NULL,
                        kind       TEXT NOT NULL,
                        title      TEXT NOT NULL,
                        body       TEXT NOT NULL DEFAULT '',
                        status     TEXT NOT NULL,
                        error      TEXT NOT NULL DEFAULT '',
                        at         TEXT NOT NULL,
                        account_id TEXT NOT NULL DEFAULT ''
                )`,
		`CREATE INDEX IF NOT EXISTS idx_notif_log_account ON notif_log (account_id)`,
		`CREATE INDEX IF NOT EXISTS idx_notif_log_at      ON notif_log (at)`,
		// QR vouchers : page de login du portail captive du routeur.
		`ALTER TABLE routers ADD COLUMN IF NOT EXISTS hotspot_login_url TEXT NOT NULL DEFAULT ''`,
		// Migrations douces pour les bases créées avant l'ajout des champs
		// agent/Wave/commandes (idempotentes, donc sans risque au premier déploiement).
		`ALTER TABLE routers ADD COLUMN IF NOT EXISTS agent_token_hash TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE routers ADD COLUMN IF NOT EXISTS token_preview TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE routers ADD COLUMN IF NOT EXISTS last_seen TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS wave_link TEXT NOT NULL DEFAULT ''`,
		// P0/P1 (audit Mikhmon) — migrations des champs des tables existantes.
		`ALTER TABLE profiles ADD COLUMN IF NOT EXISTS exp_mode TEXT NOT NULL DEFAULT 'notify'`,
		`ALTER TABLE profiles ADD COLUMN IF NOT EXISTS grace_period_min INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE profiles ADD COLUMN IF NOT EXISTS lock_user BOOLEAN NOT NULL DEFAULT FALSE`,
		`ALTER TABLE profiles ADD COLUMN IF NOT EXISTS selling_price INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE hotspot_users ADD COLUMN IF NOT EXISTS selling_price INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE hotspot_users ADD COLUMN IF NOT EXISTS enforced BOOLEAN NOT NULL DEFAULT FALSE`,
		`ALTER TABLE sales ADD COLUMN IF NOT EXISTS cost INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE sales ADD COLUMN IF NOT EXISTS selling INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS dns_name TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS logo_url TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS expiry_policy_mode TEXT NOT NULL DEFAULT 'keep'`,
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS expiry_policy_after_days INTEGER NOT NULL DEFAULT 30`,
		`ALTER TABLE routers ADD COLUMN IF NOT EXISTS board_name TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE routers ADD COLUMN IF NOT EXISTS free_hdd_mb INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE routers ADD COLUMN IF NOT EXISTS total_hdd_mb INTEGER NOT NULL DEFAULT 0`,
		// Quota de données par voucher (« 5 Go = 500 F ») : Mo, 0 = illimité.
		`ALTER TABLE hotspot_users ADD COLUMN IF NOT EXISTS data_quota_mb BIGINT NOT NULL DEFAULT 0`,
		`ALTER TABLE batches       ADD COLUMN IF NOT EXISTS data_quota_mb BIGINT NOT NULL DEFAULT 0`,
		// Migrations multi-tenant : colonne account_id sur toutes les tables métier.
		`ALTER TABLE admin_users   ADD COLUMN IF NOT EXISTS account_id TEXT NOT NULL DEFAULT ''`,
		// Changement de mot de passe par l'utilisateur (POST /api/auth/password) :
		// password_set_by_user protège le mot de passe contre l'override ADMIN_PASSWORD
		// tant que la variable ne change pas ; env_password_hash mémorise le dernier
		// mot de passe env appliqué pour détecter un changement d'intention opérateur.
		`ALTER TABLE admin_users   ADD COLUMN IF NOT EXISTS password_set_by_user BOOLEAN NOT NULL DEFAULT FALSE`,
		`ALTER TABLE admin_users   ADD COLUMN IF NOT EXISTS env_password_hash TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE routers       ADD COLUMN IF NOT EXISTS account_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE profiles      ADD COLUMN IF NOT EXISTS account_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE hotspot_users ADD COLUMN IF NOT EXISTS account_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE batches       ADD COLUMN IF NOT EXISTS account_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE resellers     ADD COLUMN IF NOT EXISTS account_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE transactions  ADD COLUMN IF NOT EXISTS account_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE sessions      ADD COLUMN IF NOT EXISTS account_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE activity      ADD COLUMN IF NOT EXISTS account_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE sales         ADD COLUMN IF NOT EXISTS account_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE commands      ADD COLUMN IF NOT EXISTS account_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE settings      ADD COLUMN IF NOT EXISTS account_id TEXT NOT NULL DEFAULT ''`,
		// La ligne singleton settings historique (id INTEGER 1) devient la ligne du
		// compte principal : id = account_id, une ligne par compte.
		`ALTER TABLE settings ALTER COLUMN id TYPE TEXT`,
		// Backfill : toutes les données de l'ère mono-tenant vont au compte principal.
		`UPDATE admin_users   SET account_id = 'acc-main' WHERE account_id = ''`,
		`UPDATE routers       SET account_id = 'acc-main' WHERE account_id = ''`,
		`UPDATE profiles      SET account_id = 'acc-main' WHERE account_id = ''`,
		`UPDATE hotspot_users SET account_id = 'acc-main' WHERE account_id = ''`,
		`UPDATE batches       SET account_id = 'acc-main' WHERE account_id = ''`,
		`UPDATE resellers     SET account_id = 'acc-main' WHERE account_id = ''`,
		`UPDATE transactions  SET account_id = 'acc-main' WHERE account_id = ''`,
		`UPDATE sessions      SET account_id = 'acc-main' WHERE account_id = ''`,
		`UPDATE activity      SET account_id = 'acc-main' WHERE account_id = ''`,
		`UPDATE sales         SET account_id = 'acc-main' WHERE account_id = ''`,
		`UPDATE commands      SET account_id = 'acc-main' WHERE account_id = ''`,
		`UPDATE settings SET account_id = 'acc-main', id = 'acc-main' WHERE account_id = ''`,
		// Index par compte (filtre d'isolation systématique).
		`CREATE INDEX IF NOT EXISTS idx_admin_users_account   ON admin_users (account_id)`,
		`CREATE INDEX IF NOT EXISTS idx_routers_account       ON routers (account_id)`,
		`CREATE INDEX IF NOT EXISTS idx_profiles_account      ON profiles (account_id)`,
		`CREATE INDEX IF NOT EXISTS idx_hotspot_users_account ON hotspot_users (account_id)`,
		`CREATE INDEX IF NOT EXISTS idx_batches_account       ON batches (account_id)`,
		`CREATE INDEX IF NOT EXISTS idx_resellers_account     ON resellers (account_id)`,
		`CREATE INDEX IF NOT EXISTS idx_transactions_account  ON transactions (account_id)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_account      ON sessions (account_id)`,
		`CREATE INDEX IF NOT EXISTS idx_activity_account      ON activity (account_id)`,
		`CREATE INDEX IF NOT EXISTS idx_sales_account         ON sales (account_id)`,
		`CREATE INDEX IF NOT EXISTS idx_commands_account      ON commands (account_id)`,
		`CREATE INDEX IF NOT EXISTS idx_settings_account      ON settings (account_id)`,
	}
	for _, q := range stmts {
		if _, err := p.db.Exec(q); err != nil {
			return fmt.Errorf("création du schéma : %w (requête : %.80s)", err, q)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Chargement au démarrage
// ---------------------------------------------------------------------------

// Load relit toute la base et reconstruit *model.DB. Retourne found=false si la
// base est vide (l'appelant déclenchera le seed démo + synchronisation initiale).
func (p *PG) Load() (db *model.DB, found bool, err error) {
	db = &model.DB{
		Accounts:          []model.Account{},
		SettingsByAccount: map[string]model.Settings{},
		Users:             []model.AdminUser{},
		Routers:           []model.Router{},
		Profiles:          []model.Profile{},
		HotspotUsers:      []model.HotspotUser{},
		Batches:           []model.Batch{},
		Resellers:         []model.Reseller{},
		Transactions:      []model.Transaction{},
		Sessions:          []model.Session{},
		Activity:          []model.Activity{},
		Sales:             []model.Sale{},
		Commands:          []model.Command{},
		Templates:         []model.VoucherTemplate{},
		UserLogs:          []model.UserLog{},
		IPBindings:        []model.IPBinding{},
		SchedulerTasks:    []model.SchedulerTask{},
		Traffic:           []model.RouterTraffic{},
		NotifSettings:     map[string]model.NotificationSettings{},
		NotifLog:          []model.NotificationLog{},
	}

	steps := []struct {
		name  string
		query func() error
	}{
		{"accounts", func() error { return loadInto(p, &db.Accounts, accountSpec) }},
		{"admin_users", func() error { return loadInto(p, &db.Users, adminSpec) }},
		{"routers", func() error { return loadInto(p, &db.Routers, routerSpec) }},
		{"profiles", func() error { return loadInto(p, &db.Profiles, profileSpec) }},
		{"hotspot_users", func() error { return loadInto(p, &db.HotspotUsers, hotspotUserSpec) }},
		{"batches", func() error { return loadInto(p, &db.Batches, batchSpec) }},
		{"resellers", func() error { return loadInto(p, &db.Resellers, resellerSpec) }},
		{"transactions", func() error { return loadInto(p, &db.Transactions, transactionSpec) }},
		{"sessions", func() error { return loadInto(p, &db.Sessions, sessionSpec) }},
		{"activity", func() error { return loadInto(p, &db.Activity, activitySpec) }},
		{"sales", func() error { return loadInto(p, &db.Sales, saleSpec) }},
		{"commands", func() error { return loadInto(p, &db.Commands, commandSpec) }},
		{"voucher_templates", func() error { return loadInto(p, &db.Templates, templateSpec) }},
		{"user_logs", func() error { return loadInto(p, &db.UserLogs, userLogSpec) }},
		{"ip_bindings", func() error { return loadInto(p, &db.IPBindings, ipBindingSpec) }},
		{"scheduler_tasks", func() error { return loadInto(p, &db.SchedulerTasks, schedulerTaskSpec) }},
		{"traffic", func() error { return loadInto(p, &db.Traffic, trafficSpec) }},
		{"notif_settings", func() error { return p.loadNotifSettings(db) }},
		{"notif_log", func() error { return loadInto(p, &db.NotifLog, notifLogSpec) }},
		{"settings", func() error { return p.loadSettings(db) }},
	}
	for _, st := range steps {
		if err := st.query(); err != nil {
			return nil, false, fmt.Errorf("lecture %s : %w", st.name, err)
		}
	}

	// Les tris applicatifs sont faits en Go (sort.Slice dans les handlers),
	// l'ordre de lecture n'a donc aucune importance.

	found = len(db.Users) > 0 || len(db.Accounts) > 0 || len(db.Routers) > 0 ||
		len(db.Profiles) > 0 || len(db.HotspotUsers) > 0 || len(db.Resellers) > 0 || len(db.Sales) > 0
	if !found {
		return nil, false, nil
	}

	// Le cache d'empreintes reflète l'état chargé.
	p.rebuildHashes(db)
	return db, true, nil
}

// loadNotifSettings lit TOUTES les lignes notif_settings (une par compte SaaS)
// et remplit db.NotifSettings. stock_alert_state (TEXT) est désérialisé en map
// par le spec (pattern commandSpec).
func (p *PG) loadNotifSettings(db *model.DB) error {
	var rows []model.NotificationSettings
	if err := loadInto(p, &rows, notifSettingsSpec); err != nil {
		return err
	}
	db.NotifSettings = map[string]model.NotificationSettings{}
	for _, s := range rows {
		db.NotifSettings[s.AccountID] = s
	}
	return nil
}

// loadSettings lit TOUTES les lignes settings (une par compte SaaS) et remplit
// db.SettingsByAccount ; les lignes sans account_id (non migrées) sont ignorées.
// last_tick (global, moteur de simulation) est repris de la première ligne non vide.
func (p *PG) loadSettings(db *model.DB) error {
	rows, err := p.db.Query(
		`SELECT account_id, tenant_name, tenant_currency, tenant_timezone, plan_name, plan_max_routers, plan_max_users, wave_link,
                        dns_name, logo_url, expiry_policy_mode, expiry_policy_after_days, last_tick
                 FROM settings`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			accID                                      string
			tenantName, tenantCurrency, tenantTimezone string
			planName, planMaxRouters, planMaxUsers     string
			waveLink, dnsName, logoURL                 string
			expiryMode                                 string
			expiryAfterDays                            int
			lastTick                                   sql.NullTime
		)
		if err := rows.Scan(&accID, &tenantName, &tenantCurrency, &tenantTimezone,
			&planName, &planMaxRouters, &planMaxUsers, &waveLink,
			&dnsName, &logoURL, &expiryMode, &expiryAfterDays, &lastTick); err != nil {
			return err
		}
		if accID == "" {
			continue // ligne historique non rattachée à un compte → ignorée
		}
		db.SettingsByAccount[accID] = model.Settings{
			Tenant: model.Tenant{
				Name: tenantName, Currency: tenantCurrency, Timezone: tenantTimezone,
				WaveLink: waveLink, DNSName: dnsName, LogoURL: logoURL,
				ExpiryPolicyMode: expiryMode, ExpiryPolicyAfterDays: expiryAfterDays,
			},
			Plan: model.Plan{Name: planName, MaxRouters: planMaxRouters, MaxUsers: planMaxUsers},
		}
		if lastTick.Valid && db.LastTick.IsZero() {
			db.LastTick = lastTick.Time
		}
	}
	return rows.Err()
}

// ---------------------------------------------------------------------------
// Synchronisation différentielle (appelée par Store.Save, sous verrou)
// ---------------------------------------------------------------------------

// Sync compare l'état mémoire aux empreintes de la dernière synchronisation
// réussie et applique les différences en une transaction :
// upserts des lignes nouvelles/modifiées, suppressions des disparues.
func (p *PG) Sync(db *model.DB) error {
	tx, err := p.db.Begin()
	if err != nil {
		return fmt.Errorf("pg sync (begin) : %w", err)
	}
	defer tx.Rollback() // no-op si Commit réussit

	if err := syncTable(tx, p.hashes, accountSpec, db.Accounts); err != nil {
		return err
	}
	if err := syncTable(tx, p.hashes, adminSpec, db.Users); err != nil {
		return err
	}
	if err := syncTable(tx, p.hashes, routerSpec, db.Routers); err != nil {
		return err
	}
	if err := syncTable(tx, p.hashes, profileSpec, db.Profiles); err != nil {
		return err
	}
	if err := syncTable(tx, p.hashes, hotspotUserSpec, db.HotspotUsers); err != nil {
		return err
	}
	if err := syncTable(tx, p.hashes, batchSpec, db.Batches); err != nil {
		return err
	}
	if err := syncTable(tx, p.hashes, resellerSpec, db.Resellers); err != nil {
		return err
	}
	if err := syncTable(tx, p.hashes, transactionSpec, db.Transactions); err != nil {
		return err
	}
	if err := syncTable(tx, p.hashes, sessionSpec, db.Sessions); err != nil {
		return err
	}
	if err := syncTable(tx, p.hashes, activitySpec, db.Activity); err != nil {
		return err
	}
	if err := syncTable(tx, p.hashes, saleSpec, db.Sales); err != nil {
		return err
	}
	if err := syncTable(tx, p.hashes, commandSpec, db.Commands); err != nil {
		return err
	}
	if err := syncTable(tx, p.hashes, templateSpec, db.Templates); err != nil {
		return err
	}
	if err := syncTable(tx, p.hashes, userLogSpec, db.UserLogs); err != nil {
		return err
	}
	if err := syncTable(tx, p.hashes, ipBindingSpec, db.IPBindings); err != nil {
		return err
	}
	if err := syncTable(tx, p.hashes, schedulerTaskSpec, db.SchedulerTasks); err != nil {
		return err
	}
	if err := syncTable(tx, p.hashes, trafficSpec, db.Traffic); err != nil {
		return err
	}
	notifRows := make([]model.NotificationSettings, 0, len(db.NotifSettings))
	for _, v := range db.NotifSettings {
		notifRows = append(notifRows, v)
	}
	if err := syncTable(tx, p.hashes, notifSettingsSpec, notifRows); err != nil {
		return err
	}
	if err := syncTable(tx, p.hashes, notifLogSpec, db.NotifLog); err != nil {
		return err
	}
	if err := p.syncSettings(tx, db); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("pg sync (commit) : %w", err)
	}
	return nil
}

// syncSettings écrit une ligne par compte de SettingsByAccount (upsert par
// id = account_id). Les lignes settings ne sont JAMAIS supprimées ici : elles
// suivent le cycle de vie des comptes (spec accounts) et une ligne orpheline
// est tolérée. last_tick (valeur globale du moteur de simulation) est répliquée
// sur chaque ligne.
func (p *PG) syncSettings(tx *sql.Tx, db *model.DB) error {
	lastTick := sql.NullTime{Time: db.LastTick, Valid: !db.LastTick.IsZero()}
	for accID, s := range db.SettingsByAccount {
		_, err := tx.Exec(
			`INSERT INTO settings (id, account_id, tenant_name, tenant_currency, tenant_timezone, plan_name, plan_max_routers, plan_max_users, wave_link,
                                dns_name, logo_url, expiry_policy_mode, expiry_policy_after_days, last_tick)
                         VALUES ($1, $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
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
                           expiry_policy_mode        = EXCLUDED.expiry_policy_mode,
                           expiry_policy_after_days  = EXCLUDED.expiry_policy_after_days,
                           last_tick                 = EXCLUDED.last_tick`,
			accID, s.Tenant.Name, s.Tenant.Currency, s.Tenant.Timezone,
			s.Plan.Name, s.Plan.MaxRouters, s.Plan.MaxUsers,
			s.Tenant.WaveLink, s.Tenant.DNSName, s.Tenant.LogoURL,
			s.Tenant.ExpiryPolicyMode, s.Tenant.ExpiryPolicyAfterDays, lastTick)
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
// clé primaire « id »), extraction d'id, lecture et écriture d'une ligne.
type entitySpec[T any] struct {
	table  string
	cols   []string
	idOf   func(*T) string
	scan   func(*sql.Rows) (T, error)
	args   func(*T) []any
	hashOf func(*T) uint64
}

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
// d'empreintes) et disparitions (id absents), applique le tout, puis rafraîchit
// le cache UNIQUEMENT en cas de succès (un échec sera retenté au Save suivant).
func syncTable[T any](tx *sql.Tx, hashes map[string]map[string]uint64, spec entitySpec[T], rows []T) error {
	cached := hashes[spec.table]
	if cached == nil {
		cached = map[string]uint64{}
		hashes[spec.table] = cached
	}

	seen := make(map[string]struct{}, len(rows))
	var changed []T
	for i := range rows {
		id := spec.idOf(&rows[i])
		seen[id] = struct{}{}
		h := spec.hashOf(&rows[i])
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
		if err := upsertRows(tx, spec, changed); err != nil {
			return err
		}
	}
	if len(removed) > 0 {
		if err := deleteRows(tx, spec.table, removed); err != nil {
			return err
		}
	}

	// Cache rafraîchi uniquement après succès des écritures.
	for id := range cached {
		delete(cached, id)
	}
	for i := range rows {
		cached[spec.idOf(&rows[i])] = spec.hashOf(&rows[i])
	}
	return nil
}

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

// deleteRows — DELETE ... WHERE id IN (…) par blocs de 500.
func deleteRows(tx *sql.Tx, table string, ids []string) error {
	for start := 0; start < len(ids); start += 500 {
		end := min(start+500, len(ids))
		chunk := ids[start:end]
		ph := make([]string, len(chunk))
		args := make([]any, len(chunk))
		for i, id := range chunk {
			ph[i] = "$" + strconv.Itoa(i+1)
			args[i] = id
		}
		q := `DELETE FROM ` + table + ` WHERE id IN (` + strings.Join(ph, ",") + `)`
		if _, err := tx.Exec(q, args...); err != nil {
			return fmt.Errorf("suppression %s : %w", table, err)
		}
	}
	return nil
}

// upsertRows — INSERT ... ON CONFLICT (id) DO UPDATE par blocs multi-lignes
// (une seule requête pour jusqu'à 200 lignes → un seul aller-retour réseau).
func upsertRows[T any](tx *sql.Tx, spec entitySpec[T], rows []T) error {
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
		sb.WriteString(` ON CONFLICT (id) DO UPDATE SET ` + setClause)

		if _, err := tx.Exec(sb.String(), args...); err != nil {
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
var accountSpec = entitySpec[model.Account]{
	table: "accounts",
	cols:  []string{"id", "name", "status", "created_at"},
	idOf:  func(x *model.Account) string { return x.ID },
	scan: func(r *sql.Rows) (model.Account, error) {
		var x model.Account
		err := r.Scan(&x.ID, &x.Name, &x.Status, &x.CreatedAt)
		return x, err
	},
	args: func(x *model.Account) []any {
		return []any{x.ID, x.Name, x.Status, x.CreatedAt}
	},
	hashOf: hashEntity[model.Account],
}

var adminSpec = entitySpec[model.AdminUser]{
	table: "admin_users",
	cols:  []string{"id", "name", "username", "role", "password_hash", "salt", "created_at", "account_id", "password_set_by_user", "env_password_hash"},
	idOf:  func(u *model.AdminUser) string { return u.ID },
	scan: func(r *sql.Rows) (model.AdminUser, error) {
		var u model.AdminUser
		err := r.Scan(&u.ID, &u.Name, &u.Username, &u.Role, &u.PasswordHash, &u.Salt, &u.CreatedAt, &u.AccountID, &u.PasswordSetByUser, &u.EnvPasswordHash)
		return u, err
	},
	args: func(u *model.AdminUser) []any {
		return []any{u.ID, u.Name, u.Username, u.Role, u.PasswordHash, u.Salt, u.CreatedAt, u.AccountID, u.PasswordSetByUser, u.EnvPasswordHash}
	},
	hashOf: hashEntity[model.AdminUser],
}

var routerSpec = entitySpec[model.Router]{
	table: "routers",
	cols: []string{"id", "name", "host", "port", "username", "password", "mode", "status",
		"version", "uptime_sec", "cpu_load", "hotspot_users", "active_sessions", "created_at",
		"hotspot_login_url", "agent_token_hash", "token_preview", "last_seen", "account_id",
		"board_name", "free_hdd_mb", "total_hdd_mb"},
	idOf: func(x *model.Router) string { return x.ID },
	scan: func(r *sql.Rows) (model.Router, error) {
		var x model.Router
		err := r.Scan(&x.ID, &x.Name, &x.Host, &x.Port, &x.Username, &x.Password, &x.Mode, &x.Status,
			&x.Version, &x.UptimeSec, &x.CPULoad, &x.HotspotUsers, &x.ActiveSessions, &x.CreatedAt,
			&x.HotspotLoginUrl, &x.AgentTokenHash, &x.TokenPreview, &x.LastSeen, &x.AccountID,
			&x.BoardName, &x.FreeHddMb, &x.TotalHddMb)
		return x, err
	},
	args: func(x *model.Router) []any {
		return []any{x.ID, x.Name, x.Host, x.Port, x.Username, x.Password, x.Mode, x.Status,
			x.Version, x.UptimeSec, x.CPULoad, x.HotspotUsers, x.ActiveSessions, x.CreatedAt,
			x.HotspotLoginUrl, x.AgentTokenHash, x.TokenPreview, x.LastSeen, x.AccountID,
			x.BoardName, x.FreeHddMb, x.TotalHddMb}
	},
	hashOf: hashEntity[model.Router],
}

var profileSpec = entitySpec[model.Profile]{
	table: "profiles",
	cols:  []string{"id", "name", "rate_limit", "session_timeout_min", "shared_users", "validity_days", "price", "data_quota_mb", "created_at", "account_id", "exp_mode", "grace_period_min", "lock_user", "selling_price"},
	idOf:  func(x *model.Profile) string { return x.ID },
	scan: func(r *sql.Rows) (model.Profile, error) {
		var x model.Profile
		err := r.Scan(&x.ID, &x.Name, &x.RateLimit, &x.SessionTimeoutMin, &x.SharedUsers,
			&x.ValidityDays, &x.Price, &x.DataQuotaMb, &x.CreatedAt, &x.AccountID,
			&x.ExpMode, &x.GracePeriodMin, &x.LockUser, &x.SellingPrice)
		return x, err
	},
	args: func(x *model.Profile) []any {
		return []any{x.ID, x.Name, x.RateLimit, x.SessionTimeoutMin, x.SharedUsers,
			x.ValidityDays, x.Price, x.DataQuotaMb, x.CreatedAt, x.AccountID,
			x.ExpMode, x.GracePeriodMin, x.LockUser, x.SellingPrice}
	},
	hashOf: hashEntity[model.Profile],
}

var hotspotUserSpec = entitySpec[model.HotspotUser]{
	table: "hotspot_users",
	cols: []string{"id", "kind", "username", "password", "profile_id", "profile_name",
		"router_id", "router_name", "status", "batch_id", "reseller_id", "reseller_name",
		"comment", "bytes_in", "bytes_out", "uptime_used_sec", "created_at", "expires_at", "used_at", "price", "data_quota_mb", "account_id",
		"selling_price", "enforced"},
	idOf: func(x *model.HotspotUser) string { return x.ID },
	scan: func(r *sql.Rows) (model.HotspotUser, error) {
		var x model.HotspotUser
		err := r.Scan(&x.ID, &x.Kind, &x.Username, &x.Password, &x.ProfileID, &x.ProfileName,
			&x.RouterID, &x.RouterName, &x.Status, &x.BatchID, &x.ResellerID, &x.ResellerName,
			&x.Comment, &x.BytesIn, &x.BytesOut, &x.UptimeUsedSec, &x.CreatedAt, &x.ExpiresAt, &x.UsedAt, &x.Price, &x.DataQuotaMb, &x.AccountID,
			&x.SellingPrice, &x.Enforced)
		return x, err
	},
	args: func(x *model.HotspotUser) []any {
		return []any{x.ID, x.Kind, x.Username, x.Password, x.ProfileID, x.ProfileName,
			x.RouterID, x.RouterName, x.Status, x.BatchID, x.ResellerID, x.ResellerName,
			x.Comment, x.BytesIn, x.BytesOut, x.UptimeUsedSec, x.CreatedAt, x.ExpiresAt, x.UsedAt, x.Price, x.DataQuotaMb, x.AccountID,
			x.SellingPrice, x.Enforced}
	},
	hashOf: hashEntity[model.HotspotUser],
}

var batchSpec = entitySpec[model.Batch]{
	table: "batches",
	cols:  []string{"id", "profile_id", "profile_name", "router_id", "router_name", "count", "unit_price", "total_cost", "data_quota_mb", "channel", "reseller_id", "reseller_name", "created_at", "account_id"},
	idOf:  func(x *model.Batch) string { return x.ID },
	scan: func(r *sql.Rows) (model.Batch, error) {
		var x model.Batch
		err := r.Scan(&x.ID, &x.ProfileID, &x.ProfileName, &x.RouterID, &x.RouterName,
			&x.Count, &x.UnitPrice, &x.TotalCost, &x.DataQuotaMb, &x.Channel, &x.ResellerID, &x.ResellerName, &x.CreatedAt, &x.AccountID)
		return x, err
	},
	args: func(x *model.Batch) []any {
		return []any{x.ID, x.ProfileID, x.ProfileName, x.RouterID, x.RouterName,
			x.Count, x.UnitPrice, x.TotalCost, x.DataQuotaMb, x.Channel, x.ResellerID, x.ResellerName, x.CreatedAt, x.AccountID}
	},
	hashOf: hashEntity[model.Batch],
}

var resellerSpec = entitySpec[model.Reseller]{
	table: "resellers",
	cols:  []string{"id", "name", "username", "phone", "credit", "vouchers_sold", "revenue", "status", "created_at", "account_id"},
	idOf:  func(x *model.Reseller) string { return x.ID },
	scan: func(r *sql.Rows) (model.Reseller, error) {
		var x model.Reseller
		err := r.Scan(&x.ID, &x.Name, &x.Username, &x.Phone, &x.Credit, &x.VouchersSold, &x.Revenue, &x.Status, &x.CreatedAt, &x.AccountID)
		return x, err
	},
	args: func(x *model.Reseller) []any {
		return []any{x.ID, x.Name, x.Username, x.Phone, x.Credit, x.VouchersSold, x.Revenue, x.Status, x.CreatedAt, x.AccountID}
	},
	hashOf: hashEntity[model.Reseller],
}

var transactionSpec = entitySpec[model.Transaction]{
	table: "transactions",
	cols:  []string{"id", "type", "reseller_id", "reseller_name", "amount", "note", "at", "account_id"},
	idOf:  func(x *model.Transaction) string { return x.ID },
	scan: func(r *sql.Rows) (model.Transaction, error) {
		var x model.Transaction
		err := r.Scan(&x.ID, &x.Type, &x.ResellerID, &x.ResellerName, &x.Amount, &x.Note, &x.At, &x.AccountID)
		return x, err
	},
	args: func(x *model.Transaction) []any {
		return []any{x.ID, x.Type, x.ResellerID, x.ResellerName, x.Amount, x.Note, x.At, x.AccountID}
	},
	hashOf: hashEntity[model.Transaction],
}

var sessionSpec = entitySpec[model.Session]{
	table: "sessions",
	cols:  []string{"id", "user_id", "username", "profile_name", "router_id", "router_name", "ip", "mac", "started_at", "uptime_sec", "bytes_in", "bytes_out", "account_id"},
	idOf:  func(x *model.Session) string { return x.ID },
	scan: func(r *sql.Rows) (model.Session, error) {
		var x model.Session
		err := r.Scan(&x.ID, &x.UserID, &x.Username, &x.ProfileName, &x.RouterID, &x.RouterName,
			&x.IP, &x.MAC, &x.StartedAt, &x.UptimeSec, &x.BytesIn, &x.BytesOut, &x.AccountID)
		return x, err
	},
	args: func(x *model.Session) []any {
		return []any{x.ID, x.UserID, x.Username, x.ProfileName, x.RouterID, x.RouterName,
			x.IP, x.MAC, x.StartedAt, x.UptimeSec, x.BytesIn, x.BytesOut, x.AccountID}
	},
	hashOf: hashEntity[model.Session],
}

var activitySpec = entitySpec[model.Activity]{
	table: "activity",
	cols:  []string{"id", "type", "message", "at", "account_id"},
	idOf:  func(x *model.Activity) string { return x.ID },
	scan: func(r *sql.Rows) (model.Activity, error) {
		var x model.Activity
		err := r.Scan(&x.ID, &x.Type, &x.Message, &x.At, &x.AccountID)
		return x, err
	},
	args: func(x *model.Activity) []any {
		return []any{x.ID, x.Type, x.Message, x.At, x.AccountID}
	},
	hashOf: hashEntity[model.Activity],
}

var saleSpec = entitySpec[model.Sale]{
	table: "sales",
	cols:  []string{"id", "amount", "profile_name", "count", "channel", "reseller_name", "router_id", "router_name", "batch_id", "at", "account_id", "cost", "selling"},
	idOf:  func(x *model.Sale) string { return x.ID },
	scan: func(r *sql.Rows) (model.Sale, error) {
		var x model.Sale
		err := r.Scan(&x.ID, &x.Amount, &x.ProfileName, &x.Count, &x.Channel, &x.ResellerName,
			&x.RouterID, &x.RouterName, &x.BatchID, &x.At, &x.AccountID, &x.Cost, &x.SellingTotal)
		return x, err
	},
	args: func(x *model.Sale) []any {
		return []any{x.ID, x.Amount, x.ProfileName, x.Count, x.Channel, x.ResellerName,
			x.RouterID, x.RouterName, x.BatchID, x.At, x.AccountID, x.Cost, x.SellingTotal}
	},
	hashOf: hashEntity[model.Sale],
}

// commandSpec — payload et result sont sérialisés en JSON dans des colonnes
// TEXT (” = absent/nil). Les maps Go rechargées reprennent exactement la même
// forme JSON (clés triées), les empreintes restent donc cohérentes.
var commandSpec = entitySpec[model.Command]{
	table: "commands",
	cols:  []string{"id", "router_id", "kind", "payload", "status", "result", "created_at", "sent_at", "done_at", "account_id"},
	idOf:  func(x *model.Command) string { return x.ID },
	scan: func(r *sql.Rows) (model.Command, error) {
		var x model.Command
		var payload, result string
		err := r.Scan(&x.ID, &x.RouterID, &x.Kind, &payload, &x.Status, &result, &x.CreatedAt, &x.SentAt, &x.DoneAt, &x.AccountID)
		if err != nil {
			return x, err
		}
		if payload != "" {
			_ = json.Unmarshal([]byte(payload), &x.Payload)
		}
		if result != "" {
			_ = json.Unmarshal([]byte(result), &x.Result)
		}
		return x, nil
	},
	args: func(x *model.Command) []any {
		payload, result := "", ""
		if x.Payload != nil {
			if b, err := json.Marshal(x.Payload); err == nil {
				payload = string(b)
			}
		}
		if x.Result != nil {
			if b, err := json.Marshal(x.Result); err == nil {
				result = string(b)
			}
		}
		return []any{x.ID, x.RouterID, x.Kind, payload, x.Status, result, x.CreatedAt, x.SentAt, x.DoneAt, x.AccountID}
	},
	hashOf: hashEntity[model.Command],
}

// ---------------------------------------------------------------------------
// P0/P1 (audit Mikhmon) — specs des nouvelles collections
// ---------------------------------------------------------------------------

var templateSpec = entitySpec[model.VoucherTemplate]{
	table: "voucher_templates",
	cols:  []string{"id", "account_id", "name", "format", "body_html", "is_default", "created_at"},
	idOf:  func(x *model.VoucherTemplate) string { return x.ID },
	scan: func(r *sql.Rows) (model.VoucherTemplate, error) {
		var x model.VoucherTemplate
		err := r.Scan(&x.ID, &x.AccountID, &x.Name, &x.Format, &x.BodyHTML, &x.IsDefault, &x.CreatedAt)
		return x, err
	},
	args: func(x *model.VoucherTemplate) []any {
		return []any{x.ID, x.AccountID, x.Name, x.Format, x.BodyHTML, x.IsDefault, x.CreatedAt}
	},
	hashOf: hashEntity[model.VoucherTemplate],
}

var userLogSpec = entitySpec[model.UserLog]{
	table: "user_logs",
	cols:  []string{"id", "account_id", "user_id", "username", "action", "router_id", "router_name", "ip", "mac", "at"},
	idOf:  func(x *model.UserLog) string { return x.ID },
	scan: func(r *sql.Rows) (model.UserLog, error) {
		var x model.UserLog
		err := r.Scan(&x.ID, &x.AccountID, &x.UserID, &x.Username, &x.Action, &x.RouterID, &x.RouterName, &x.IP, &x.MAC, &x.At)
		return x, err
	},
	args: func(x *model.UserLog) []any {
		return []any{x.ID, x.AccountID, x.UserID, x.Username, x.Action, x.RouterID, x.RouterName, x.IP, x.MAC, x.At}
	},
	hashOf: hashEntity[model.UserLog],
}

var ipBindingSpec = entitySpec[model.IPBinding]{
	table: "ip_bindings",
	cols:  []string{"id", "account_id", "router_id", "mac", "address", "comment", "type", "disabled", "created_at"},
	idOf:  func(x *model.IPBinding) string { return x.ID },
	scan: func(r *sql.Rows) (model.IPBinding, error) {
		var x model.IPBinding
		err := r.Scan(&x.ID, &x.AccountID, &x.RouterID, &x.MAC, &x.Address, &x.Comment, &x.Type, &x.Disabled, &x.CreatedAt)
		return x, err
	},
	args: func(x *model.IPBinding) []any {
		return []any{x.ID, x.AccountID, x.RouterID, x.MAC, x.Address, x.Comment, x.Type, x.Disabled, x.CreatedAt}
	},
	hashOf: hashEntity[model.IPBinding],
}

var schedulerTaskSpec = entitySpec[model.SchedulerTask]{
	table: "scheduler_tasks",
	cols:  []string{"id", "account_id", "router_id", "name", "interval", "on_event", "disabled", "created_at"},
	idOf:  func(x *model.SchedulerTask) string { return x.ID },
	scan: func(r *sql.Rows) (model.SchedulerTask, error) {
		var x model.SchedulerTask
		err := r.Scan(&x.ID, &x.AccountID, &x.RouterID, &x.Name, &x.Interval, &x.OnEvent, &x.Disabled, &x.CreatedAt)
		return x, err
	},
	args: func(x *model.SchedulerTask) []any {
		return []any{x.ID, x.AccountID, x.RouterID, x.Name, x.Interval, x.OnEvent, x.Disabled, x.CreatedAt}
	},
	hashOf: hashEntity[model.SchedulerTask],
}

// trafficSpec — une ligne par routeur (id = router_id) ; interfaces et history
// sont sérialisées en JSON (même mécanique que commandSpec : ” = vide).
var trafficSpec = entitySpec[model.RouterTraffic]{
	table: "traffic",
	cols:  []string{"id", "account_id", "router_id", "updated_at", "interfaces", "history"},
	idOf:  func(x *model.RouterTraffic) string { return x.ID },
	scan: func(r *sql.Rows) (model.RouterTraffic, error) {
		var x model.RouterTraffic
		var ifaces, hist string
		if err := r.Scan(&x.ID, &x.AccountID, &x.RouterID, &x.UpdatedAt, &ifaces, &hist); err != nil {
			return x, err
		}
		if ifaces != "" && ifaces != "[]" {
			_ = json.Unmarshal([]byte(ifaces), &x.Interfaces)
		}
		if hist != "" && hist != "[]" {
			_ = json.Unmarshal([]byte(hist), &x.History)
		}
		if x.Interfaces == nil {
			x.Interfaces = []model.IfaceTraffic{}
		}
		if x.History == nil {
			x.History = []model.TrafficPoint{}
		}
		return x, nil
	},
	args: func(x *model.RouterTraffic) []any {
		ifaces, hist := "[]", "[]"
		if x.Interfaces != nil {
			if b, err := json.Marshal(x.Interfaces); err == nil {
				ifaces = string(b)
			}
		}
		if x.History != nil {
			if b, err := json.Marshal(x.History); err == nil {
				hist = string(b)
			}
		}
		return []any{x.ID, x.AccountID, x.RouterID, x.UpdatedAt, ifaces, hist}
	},
	hashOf: hashEntity[model.RouterTraffic],
}

// notifSettingsSpec — réglages de notification par compte. id = account_id.
// stock_alert_state est sérialisé en JSON dans une colonne TEXT (” = nil).
var notifSettingsSpec = entitySpec[model.NotificationSettings]{
	table: "notif_settings",
	cols: []string{"id", "enabled", "telegram_enabled", "telegram_bot_token", "telegram_chat_id",
		"whatsapp_enabled", "whatsapp_token", "whatsapp_phone_id", "whatsapp_to",
		"email_enabled", "smtp_host", "smtp_port", "smtp_user", "smtp_pass", "email_to",
		"offline_after_sec", "low_stock_threshold", "daily_report", "report_hour",
		"last_report_date", "stock_alert_state", "account_id"},
	idOf: func(x *model.NotificationSettings) string { return x.AccountID },
	scan: func(r *sql.Rows) (model.NotificationSettings, error) {
		var x model.NotificationSettings
		var stockState string
		err := r.Scan(&x.AccountID, &x.Enabled, &x.TelegramEnabled, &x.TelegramBotToken, &x.TelegramChatID,
			&x.WhatsAppEnabled, &x.WhatsAppToken, &x.WhatsAppPhoneID, &x.WhatsAppTo,
			&x.EmailEnabled, &x.SMTPHost, &x.SMTPPort, &x.SMTPUser, &x.SMTPPass, &x.EmailTo,
			&x.OfflineAfterSec, &x.LowStockThreshold, &x.DailyReport, &x.ReportHour,
			&x.LastReportDate, &stockState, &x.AccountID)
		if err != nil {
			return x, err
		}
		if stockState != "" {
			_ = json.Unmarshal([]byte(stockState), &x.StockAlertState)
		}
		return x, nil
	},
	args: func(x *model.NotificationSettings) []any {
		stockState := ""
		if x.StockAlertState != nil {
			if b, err := json.Marshal(x.StockAlertState); err == nil {
				stockState = string(b)
			}
		}
		return []any{x.AccountID, x.Enabled, x.TelegramEnabled, x.TelegramBotToken, x.TelegramChatID,
			x.WhatsAppEnabled, x.WhatsAppToken, x.WhatsAppPhoneID, x.WhatsAppTo,
			x.EmailEnabled, x.SMTPHost, x.SMTPPort, x.SMTPUser, x.SMTPPass, x.EmailTo,
			x.OfflineAfterSec, x.LowStockThreshold, x.DailyReport, x.ReportHour,
			x.LastReportDate, stockState, x.AccountID}
	},
	hashOf: hashEntity[model.NotificationSettings],
}

// notifLogSpec — historique des notifications envoyées (par compte).
var notifLogSpec = entitySpec[model.NotificationLog]{
	table: "notif_log",
	cols:  []string{"id", "channel", "kind", "title", "body", "status", "error", "at", "account_id"},
	idOf:  func(x *model.NotificationLog) string { return x.ID },
	scan: func(r *sql.Rows) (model.NotificationLog, error) {
		var x model.NotificationLog
		err := r.Scan(&x.ID, &x.Channel, &x.Kind, &x.Title, &x.Body, &x.Status, &x.Error, &x.At, &x.AccountID)
		return x, err
	},
	args: func(x *model.NotificationLog) []any {
		return []any{x.ID, x.Channel, x.Kind, x.Title, x.Body, x.Status, x.Error, x.At, x.AccountID}
	},
	hashOf: hashEntity[model.NotificationLog],
}

// rebuildHashes — reconstruit le cache d'empreintes à partir d'un état mémoire
// (après un Load ou un seed initial).
func (p *PG) rebuildHashes(db *model.DB) {
	p.hashes = map[string]map[string]uint64{
		accountSpec.table:       hashRows(db.Accounts, accountSpec),
		adminSpec.table:         hashRows(db.Users, adminSpec),
		routerSpec.table:        hashRows(db.Routers, routerSpec),
		profileSpec.table:       hashRows(db.Profiles, profileSpec),
		hotspotUserSpec.table:   hashRows(db.HotspotUsers, hotspotUserSpec),
		batchSpec.table:         hashRows(db.Batches, batchSpec),
		resellerSpec.table:      hashRows(db.Resellers, resellerSpec),
		transactionSpec.table:   hashRows(db.Transactions, transactionSpec),
		sessionSpec.table:       hashRows(db.Sessions, sessionSpec),
		activitySpec.table:      hashRows(db.Activity, activitySpec),
		saleSpec.table:          hashRows(db.Sales, saleSpec),
		commandSpec.table:       hashRows(db.Commands, commandSpec),
		templateSpec.table:      hashRows(db.Templates, templateSpec),
		userLogSpec.table:       hashRows(db.UserLogs, userLogSpec),
		ipBindingSpec.table:     hashRows(db.IPBindings, ipBindingSpec),
		schedulerTaskSpec.table: hashRows(db.SchedulerTasks, schedulerTaskSpec),
		trafficSpec.table:       hashRows(db.Traffic, trafficSpec),
		notifLogSpec.table:      hashRows(db.NotifLog, notifLogSpec),
	}
	notifRows := make([]model.NotificationSettings, 0, len(db.NotifSettings))
	for _, v := range db.NotifSettings {
		notifRows = append(notifRows, v)
	}
	p.hashes[notifSettingsSpec.table] = hashRows(notifRows, notifSettingsSpec)
}

// hashRows — empreintes indexées par id.
func hashRows[T any](rows []T, spec entitySpec[T]) map[string]uint64 {
	m := make(map[string]uint64, len(rows))
	for i := range rows {
		m[spec.idOf(&rows[i])] = spec.hashOf(&rows[i])
	}
	return m
}
