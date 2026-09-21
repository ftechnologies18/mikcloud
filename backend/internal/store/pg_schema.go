// Schéma PostgreSQL (DDL ensureSchema) et migrations de scellement des colonnes secrètes.
// Extrait du monolithe pg.go (N°88) — même package, contenu inchangé.
package store

import (
	"fmt"
	"log"
	"mikcloud/hotspot-api/internal/secretbox"
)

// ---------------------------------------------------------------------------
// Schéma (idempotent)
// ---------------------------------------------------------------------------

// rlsStatements — N°166 — un « ALTER TABLE ... ENABLE ROW LEVEL SECURITY »
// par table du registre syncKnownTables. Généré DU registre pour qu'une table
// future soit couverte automatiquement (garde TestRLSStatementsCoverRegistry).
func rlsStatements() []string {
	out := make([]string, 0, len(syncKnownTables))
	for name := range syncKnownTables {
		out = append(out, `ALTER TABLE `+name+` ENABLE ROW LEVEL SECURITY`)
	}
	return out
}

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
                        created_at          TEXT NOT NULL,
                        exp_mode            TEXT NOT NULL DEFAULT 'notify',
                        grace_period_min    INTEGER NOT NULL DEFAULT 0,
                        lock_user           BOOLEAN NOT NULL DEFAULT FALSE,
                        selling_price       INTEGER NOT NULL DEFAULT 0,
                        lock_first_device   BOOLEAN NOT NULL DEFAULT FALSE,
                        address_pool        TEXT NOT NULL DEFAULT '',
                        parent_queue        TEXT NOT NULL DEFAULT '',
                        validity_min        INTEGER NOT NULL DEFAULT 0
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
                        data_quota_mb   BIGINT NOT NULL DEFAULT 0,
                        time_limit_min  BIGINT NOT NULL DEFAULT 0
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
                        time_limit_min BIGINT NOT NULL DEFAULT 0,
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
                        created_at    TEXT NOT NULL,
                        pin_hash      TEXT NOT NULL DEFAULT ''
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
                        id         TEXT PRIMARY KEY,
                        type       TEXT NOT NULL,
                        message    TEXT NOT NULL,
                        at         TEXT NOT NULL,
                        actor_id   TEXT NOT NULL DEFAULT '',
                        actor_name TEXT NOT NULL DEFAULT ''
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
		// F (signup enrichi) — contact propriétaire + segmentation géographique.
		`ALTER TABLE accounts ADD COLUMN IF NOT EXISTS email   TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE accounts ADD COLUMN IF NOT EXISTS phone   TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE accounts ADD COLUMN IF NOT EXISTS country TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE accounts ADD COLUMN IF NOT EXISTS city    TEXT NOT NULL DEFAULT ''`,
		// N°98 — usage du compte (Hotspot vs HomeNet, Phase 1 du
		// produit bi-mode) : « hotspot » par défaut — tout le parc
		// existant reste sur le produit historique, la colonne ne
		// change RIEN tant que personne ne la pose à « homenet »
		// (seuls la console plateforme et l'API admin le peuvent).
		// Même mécanique idempotente que N°47 : sans l'ALTER, le
		// SELECT différentiel de la nouvelle colonne ne boote pas
		// (SQLSTATE 42703) sur les bases préexistantes.
		`ALTER TABLE accounts ADD COLUMN IF NOT EXISTS usage TEXT NOT NULL DEFAULT 'hotspot'`,
		// N°101 — appareils des foyers HomeNet : registre des bails DHCP
		// (identité stable = MAC, upsert par routeur) + nom affecté par la
		// famille + état de pause dîner (désiré côté cloud, convergé par la
		// commande agent device_pause). Remplie UNIQUEMENT pour les routeurs
		// agent de comptes homenet — la table des comptes hotspot reste vide
		// (cadenceur du check-in, cf. ensureHomeDevicesLocked côté api).
		`CREATE TABLE IF NOT EXISTS devices (
                                id           TEXT PRIMARY KEY,
                                account_id   TEXT NOT NULL DEFAULT '',
                                router_id    TEXT NOT NULL DEFAULT '',
                                router_name  TEXT NOT NULL DEFAULT '',
                                mac          TEXT NOT NULL DEFAULT '',
                                name         TEXT NOT NULL DEFAULT '',
                                hostname     TEXT NOT NULL DEFAULT '',
                                ip           TEXT NOT NULL DEFAULT '',
                                status       TEXT NOT NULL DEFAULT '',
                                expires      TEXT NOT NULL DEFAULT '',
                                lease_at     TEXT NOT NULL DEFAULT '',
                                created_at   TEXT NOT NULL DEFAULT '',
                                paused       BOOLEAN NOT NULL DEFAULT FALSE,
                                paused_until TEXT NOT NULL DEFAULT ''
                )`,
		`CREATE INDEX IF NOT EXISTS idx_devices_router  ON devices (router_id)`,
		`CREATE INDEX IF NOT EXISTS idx_devices_account ON devices (account_id)`,
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
		// N°103 — qualité de ligne : une ligne par (routeur, jour,
		// interface). Histogrammes en TEXT canonique « c0,…,c15 »
		// (fusionnés incrémentalement — cf. model/linequality.go).
		`CREATE TABLE IF NOT EXISTS line_quality (
                        id         TEXT PRIMARY KEY,
                        account_id TEXT NOT NULL DEFAULT '',
                        router_id  TEXT NOT NULL,
                        day        TEXT NOT NULL,
                        iface      TEXT NOT NULL,
                        samples    INTEGER NOT NULL,
                        rx_max_bps BIGINT NOT NULL,
                        tx_max_bps BIGINT NOT NULL,
                        rx_hist    TEXT NOT NULL DEFAULT '',
                        tx_hist    TEXT NOT NULL DEFAULT '',
                        updated_at TEXT NOT NULL
                )`,
		`CREATE INDEX IF NOT EXISTS idx_line_quality_account ON line_quality (account_id)`,
		`CREATE INDEX IF NOT EXISTS idx_line_quality_router  ON line_quality (router_id)`,
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
                        email_provider      TEXT NOT NULL DEFAULT '', -- '' | smtp | resend (N°67)
                        resend_api_key      TEXT NOT NULL DEFAULT '',
                        resend_from         TEXT NOT NULL DEFAULT '',
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
		// Facturation (verrou du cycle) — demandes de souscription /
		// renouvellement (file actionnable de la console plateforme).
		`CREATE TABLE IF NOT EXISTS billing_requests (
                        id           TEXT PRIMARY KEY,
                        account_id   TEXT NOT NULL DEFAULT '',
                        plan_id      TEXT NOT NULL DEFAULT '',
                        plan_name    TEXT NOT NULL DEFAULT '',
                        amount_fcfa  INTEGER NOT NULL DEFAULT 0,
                        period_label TEXT NOT NULL DEFAULT '',
                        router_count INTEGER NOT NULL DEFAULT 0,
                        ref          TEXT NOT NULL DEFAULT '',
                        status       TEXT NOT NULL DEFAULT 'pending',
                        created_at   TEXT NOT NULL,
                        resolved_at  TEXT NOT NULL DEFAULT '',
                        resolved_by  TEXT NOT NULL DEFAULT '',
                        note         TEXT NOT NULL DEFAULT '',
                        paid_via     TEXT NOT NULL DEFAULT ''
                )`,
		`CREATE INDEX IF NOT EXISTS idx_billing_requests_account ON billing_requests (account_id)`,
		`CREATE INDEX IF NOT EXISTS idx_billing_requests_status  ON billing_requests (status)`,
		`CREATE INDEX IF NOT EXISTS idx_billing_requests_ref     ON billing_requests (ref)`,
		// Abonnement récurrent par carte (Stripe via GeniusPay) : état local des
		// prélèvements automatiques (webhook subscription.* / resynchronisation).
		`CREATE TABLE IF NOT EXISTS geniuspay_subs (
                        uuid            TEXT PRIMARY KEY,
                        account_id      TEXT NOT NULL DEFAULT '',
                        plan_id         TEXT NOT NULL DEFAULT '',
                        plan_name       TEXT NOT NULL DEFAULT '',
                        cycle           TEXT NOT NULL DEFAULT '',
                        amount_fcfa     INTEGER NOT NULL DEFAULT 0,
                        slots           INTEGER NOT NULL DEFAULT 0,
                        status          TEXT NOT NULL DEFAULT 'pending',
                        customer_name   TEXT NOT NULL DEFAULT '',
                        customer_email  TEXT NOT NULL DEFAULT '',
                        phone           TEXT NOT NULL DEFAULT '',
                        next_billing    TEXT NOT NULL DEFAULT '',
                        last_invoice_at TEXT NOT NULL DEFAULT '',
                        last_renewal_at TEXT NOT NULL DEFAULT '',
                        created_at      TEXT NOT NULL,
                        updated_at      TEXT NOT NULL DEFAULT '',
                        cancelled_at    TEXT NOT NULL DEFAULT ''
                )`,
		`CREATE INDEX IF NOT EXISTS idx_geniuspay_subs_account ON geniuspay_subs (account_id)`,
		// Audit purge/résurgence — tombstones de purge : la purge admin
		// supprime du cloud mais PAS des routeurs réels ; ces marqueurs
		// bloquent le ré-import des usernames purgés par la synchro agent
		// pendant 30 jours (PurgeTombstoneTTL) ou jusqu'à levée explicite.
		`CREATE TABLE IF NOT EXISTS purge_tombstones (
                        id         TEXT PRIMARY KEY,
                        account_id TEXT NOT NULL DEFAULT '',
                        username   TEXT NOT NULL DEFAULT '',
                        purged_at  TEXT NOT NULL DEFAULT '',
                        expires_at TEXT NOT NULL DEFAULT ''
                )`,
		`CREATE INDEX IF NOT EXISTS idx_purge_tombstones_account ON purge_tombstones (account_id)`,
		`CREATE INDEX IF NOT EXISTS idx_purge_tombstones_user    ON purge_tombstones (username)`,
		// N°27 — inscriptions publiques par QR : liens d'invitation (token
		// serveur révocable, compteur d'usages, expiration) + demandes en
		// attente de validation par le gérant.
		`CREATE TABLE IF NOT EXISTS join_links (
                        id              TEXT PRIMARY KEY,
                        account_id      TEXT NOT NULL DEFAULT '',
                        name            TEXT NOT NULL DEFAULT '',
                        token           TEXT NOT NULL DEFAULT '',
                        profile_id      TEXT NOT NULL DEFAULT '',
                        profile_name    TEXT NOT NULL DEFAULT '',
                        router_id       TEXT NOT NULL DEFAULT '',
                        router_name     TEXT NOT NULL DEFAULT '',
                        auto_validate   BOOLEAN NOT NULL DEFAULT FALSE,
                        max_uses        INTEGER NOT NULL DEFAULT 0,
                        uses            INTEGER NOT NULL DEFAULT 0,
                        expires_at      TEXT NOT NULL DEFAULT '',
                        revoked         BOOLEAN NOT NULL DEFAULT FALSE,
                        created_by      TEXT NOT NULL DEFAULT '',
                        created_by_name TEXT NOT NULL DEFAULT '',
                        created_at      TEXT NOT NULL
                )`,
		`CREATE INDEX IF NOT EXISTS idx_join_links_account ON join_links (account_id)`,
		`CREATE INDEX IF NOT EXISTS idx_join_links_token  ON join_links (token)`,
		`CREATE TABLE IF NOT EXISTS registration_requests (
                        id               TEXT PRIMARY KEY,
                        account_id       TEXT NOT NULL DEFAULT '',
                        link_id          TEXT NOT NULL DEFAULT '',
                        link_name        TEXT NOT NULL DEFAULT '',
                        full_name        TEXT NOT NULL DEFAULT '',
                        phone            TEXT NOT NULL DEFAULT '',
                        desired_username TEXT NOT NULL DEFAULT '',
                        password         TEXT NOT NULL DEFAULT '',
                        message          TEXT NOT NULL DEFAULT '',
                        status           TEXT NOT NULL DEFAULT 'pending',
                        rejection_reason TEXT NOT NULL DEFAULT '',
                        reviewed_by      TEXT NOT NULL DEFAULT '',
                        reviewed_by_name TEXT NOT NULL DEFAULT '',
                        reviewed_at      TEXT NOT NULL DEFAULT '',
                        user_id          TEXT NOT NULL DEFAULT '',
                        created_ip       TEXT NOT NULL DEFAULT '',
                        created_mac      TEXT NOT NULL DEFAULT '',
                        created_at       TEXT NOT NULL
                )`,
		`CREATE INDEX IF NOT EXISTS idx_registration_requests_account ON registration_requests (account_id)`,
		`CREATE INDEX IF NOT EXISTS idx_registration_requests_status ON registration_requests (status)`,
		// N°28 — WiFi jetable : sites publics (slug GLOBALEMENT unique) +
		// registre marketing visiteurs (1 ligne = 1 code délivré).
		`CREATE TABLE IF NOT EXISTS wifi_sites (
                        id          TEXT PRIMARY KEY,
                        account_id  TEXT NOT NULL DEFAULT '',
                        name        TEXT NOT NULL DEFAULT '',
                        slug        TEXT NOT NULL DEFAULT '',
                        router_id   TEXT NOT NULL DEFAULT '',
                        router_name TEXT NOT NULL DEFAULT '',
                        profile_id   TEXT NOT NULL DEFAULT '',
                        profile_name TEXT NOT NULL DEFAULT '',
                        free_time_min BIGINT NOT NULL DEFAULT 0,
                        free_data_mb  BIGINT NOT NULL DEFAULT 0,
                        marketing_opt_in BOOLEAN NOT NULL DEFAULT FALSE,
                        daily_per_phone INTEGER NOT NULL DEFAULT 1,
                        daily_per_mac   INTEGER NOT NULL DEFAULT 1,
                        daily_cap       INTEGER NOT NULL DEFAULT 100,
                        active    BOOLEAN NOT NULL DEFAULT FALSE,
                        created_at TEXT NOT NULL DEFAULT ''
                )`,
		`CREATE INDEX IF NOT EXISTS idx_wifi_sites_account ON wifi_sites (account_id)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_wifi_sites_slug ON wifi_sites (slug)`,
		`CREATE TABLE IF NOT EXISTS wifi_guests (
                        id         TEXT PRIMARY KEY,
                        account_id TEXT NOT NULL DEFAULT '',
                        site_id    TEXT NOT NULL DEFAULT '',
                        site_name  TEXT NOT NULL DEFAULT '',
                        phone      TEXT NOT NULL DEFAULT '',
                        opt_in     BOOLEAN NOT NULL DEFAULT FALSE,
                        voucher_id TEXT NOT NULL DEFAULT '',
                        code       TEXT NOT NULL DEFAULT '',
                        day        TEXT NOT NULL DEFAULT '',
                        created_at TEXT NOT NULL DEFAULT '',
                        claim_cmd_id TEXT NOT NULL DEFAULT '',
                        mac        TEXT NOT NULL DEFAULT '',
                        ip         TEXT NOT NULL DEFAULT '',
                        opt_in_at  TEXT NOT NULL DEFAULT ''
                )`,
		`CREATE INDEX IF NOT EXISTS idx_wifi_guests_account ON wifi_guests (account_id)`,
		`CREATE INDEX IF NOT EXISTS idx_wifi_guests_site   ON wifi_guests (site_id)`,
		`CREATE INDEX IF NOT EXISTS idx_wifi_guests_phone  ON wifi_guests (phone)`,
		// N°56 — analytics du portail hospitalité : journal des
		// impressions/clics par promo. La déduplication (un appareil =
		// une ligne par promo/jour/type) est portée par l'ID
		// DÉTERMINISTE de la ligne (PK) — pas besoin d'index unique
		// supplémentaire ; les index couvrent les lectures de stats.
		`CREATE TABLE IF NOT EXISTS promo_events (
                        id         TEXT PRIMARY KEY,
                        account_id TEXT NOT NULL DEFAULT '',
                        promo_id   TEXT NOT NULL DEFAULT '',
                        kind       TEXT NOT NULL DEFAULT '',
                        client_key TEXT NOT NULL DEFAULT '',
                        day        TEXT NOT NULL DEFAULT '',
                        created_at TEXT NOT NULL DEFAULT ''
                )`,
		`CREATE INDEX IF NOT EXISTS idx_promo_events_account ON promo_events (account_id, day)`,
		`CREATE INDEX IF NOT EXISTS idx_promo_events_promo   ON promo_events (account_id, promo_id)`,
		// N°47 — colonne ajoutée sur la table EXISTANTE : le CREATE TABLE IF NOT
		// EXISTS ne fait rien sur une base déjà initialisée, et le chargement
		// différentiel SELECT la colonne ⇒ sans ALTER, le store ne boot plus
		// (SQLSTATE 42703 « column does not exist », déploiement Render
		// update_failed constaté sur f85629e). Idempotent, sans risque pour le
		// code antérieur (listes de colonnes explicites).
		`ALTER TABLE wifi_guests ADD COLUMN IF NOT EXISTS claim_cmd_id TEXT NOT NULL DEFAULT ''`,
		// N°50 — garde-fous anti-abus WiFi jetable : plafond par appareil sur
		// les sites, empreintes MAC/IP sur le registre. Même mécanique que
		// N°47/N°49 : les tables pré-existantes ont besoin de l'ALTER.
		`ALTER TABLE wifi_sites ADD COLUMN IF NOT EXISTS daily_per_mac INTEGER NOT NULL DEFAULT 1`,
		`ALTER TABLE wifi_guests ADD COLUMN IF NOT EXISTS mac TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE wifi_guests ADD COLUMN IF NOT EXISTS ip TEXT NOT NULL DEFAULT ''`,
		// N°69 — preuve d'opt-in marketing : horodatage du consentement
		// explicite (interrupteur du claim / endpoint consent). Même
		// mécanique que N°47/N°50 : les bases pré-existantes ont besoin
		// de l'ALTER, sinon le SELECT différentiel ne boote pas
		// (SQLSTATE 42703).
		`ALTER TABLE wifi_guests ADD COLUMN IF NOT EXISTS opt_in_at TEXT NOT NULL DEFAULT ''`,
		// N°49 — QR de connexion WiFi : SSID (+ mot de passe WPA
		// optionnel) du réseau du hotspot, encodés dans l'affiche
		// imprimable (format universel WIFI:). Même mécanique que
		// N°47 : les tables pré-existantes ont besoin de l'ALTER.
		`ALTER TABLE wifi_sites ADD COLUMN IF NOT EXISTS wifi_ssid TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE wifi_sites ADD COLUMN IF NOT EXISTS wifi_password TEXT NOT NULL DEFAULT ''`,
		// Audit purge — réglage par compte : import automatique des
		// utilisateurs créés hors MikCloud (défaut ON — compatibilité).
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS auto_import_router_users BOOLEAN NOT NULL DEFAULT TRUE`,
		// N°46 — bouton « S'inscrire » du portail captif, réglage par
		// compte (défaut ON — compatibilité zéro-migration : les lignes
		// existantes passent à TRUE explicite au premier Save).
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS join_button BOOLEAN NOT NULL DEFAULT TRUE`,
		// N°64 — date du dernier balayage PÉRIODIQUE de rétention (purge
		// des journaux à 90 j, goroutine 1 h) : valeur GLOBALE du moteur,
		// portée par la première ligne non vide comme last_tick. Preuve
		// d'audit (GET / lastSweepAt) qui survit aux redémarrages.
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS last_sweep TIMESTAMPTZ`,
		// N°7 — rôles équipe + audit : acteur des actions du journal, et
		// renommage du rôle historique « admin » → « platform_admin » (les
		// tokens existants portant « admin » restent acceptés côté API).
		`ALTER TABLE activity ADD COLUMN IF NOT EXISTS actor_id TEXT NOT NULL DEFAULT ''`,
		// N°151 — acquit de la cloche par utilisateur (read-state serveur).
		`ALTER TABLE admin_users ADD COLUMN IF NOT EXISTS activity_seen_at TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE activity ADD COLUMN IF NOT EXISTS actor_name TEXT NOT NULL DEFAULT ''`,
		`UPDATE admin_users SET role = 'platform_admin' WHERE role = 'admin'`,
		// N°8 — Mode Vente : PIN revendeur + traçabilité des remises.
		`ALTER TABLE resellers     ADD COLUMN IF NOT EXISTS pin_hash TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE hotspot_users ADD COLUMN IF NOT EXISTS sold_at TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE hotspot_users ADD COLUMN IF NOT EXISTS sold_via TEXT NOT NULL DEFAULT ''`,
		// N°19 — dépôt-vente : mode de paiement revendeur + plafond de créance + marqueur stock à crédit.
		`ALTER TABLE resellers     ADD COLUMN IF NOT EXISTS payment_mode TEXT NOT NULL DEFAULT 'prepaid'`,
		`ALTER TABLE resellers     ADD COLUMN IF NOT EXISTS debt_ceiling INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE hotspot_users ADD COLUMN IF NOT EXISTS credit_sale BOOLEAN NOT NULL DEFAULT FALSE`,
		// N°66 — limite d'appareils simultanés (Mode Vente) par revendeur :
		// 0 = illimité (défaut, token stateless historique) ; N ≥ 1 = registre
		// sell_sessions + jti embarqué dans le JWT (éviction de l'appareil
		// connecté depuis le plus longtemps au-delà de la limite).
		`ALTER TABLE resellers ADD COLUMN IF NOT EXISTS max_devices INTEGER NOT NULL DEFAULT 0`,
		`CREATE TABLE IF NOT EXISTS sell_sessions (
                        id          TEXT PRIMARY KEY,
                        account_id  TEXT NOT NULL,
                        reseller_id TEXT NOT NULL,
                        issued_at   TEXT NOT NULL,
                        last_seen   TEXT NOT NULL,
                        user_agent  TEXT NOT NULL,
                        ip          TEXT NOT NULL
                )`,
		`CREATE INDEX IF NOT EXISTS idx_sell_sessions_reseller ON sell_sessions (reseller_id)`,
		// N°68 — « Mot de passe oublié ? » : liens de réinitialisation
		// e-mail (token stocké HASHÉ en SHA-256, expiration 60 min,
		// usage unique). Nouvelle table additive — aucun impact sur les
		// versions antérieures du backend.
		`CREATE TABLE IF NOT EXISTS password_resets (
                        id         TEXT PRIMARY KEY,
                        account_id TEXT NOT NULL,
                        user_id    TEXT NOT NULL,
                        token_hash TEXT NOT NULL,
                        expires_at TEXT NOT NULL,
                        used_at    TEXT NOT NULL DEFAULT '',
                        created_at TEXT NOT NULL,
                        created_ip TEXT NOT NULL DEFAULT ''
                )`,
		`CREATE INDEX IF NOT EXISTS idx_password_resets_account ON password_resets (account_id)`,
		`CREATE INDEX IF NOT EXISTS idx_password_resets_token  ON password_resets (token_hash)`,
		// N°127 — assistant conversationnel public : conversations
		// visiteur ↔ bot ↔ support (la FAQ du landing est devenue
		// un chatbot, transmission à un humain depuis la console).
		`CREATE TABLE IF NOT EXISTS chat_conversations (
                        id         TEXT PRIMARY KEY,
                        token_hash TEXT NOT NULL,
                        lang       TEXT NOT NULL DEFAULT 'fr',
                        status     TEXT NOT NULL DEFAULT 'bot',
                        created_ip TEXT NOT NULL DEFAULT '',
                        created_at TEXT NOT NULL,
                        updated_at TEXT NOT NULL,
                        unread     INTEGER NOT NULL DEFAULT 0
                )`,
		`CREATE INDEX IF NOT EXISTS idx_chat_conversations_status ON chat_conversations (status, updated_at)`,
		`CREATE TABLE IF NOT EXISTS announcements (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			body TEXT NOT NULL DEFAULT '',
			level TEXT NOT NULL DEFAULT 'info',
			audience TEXT NOT NULL DEFAULT 'all',
			created_at TEXT NOT NULL,
			publish_at TEXT NOT NULL DEFAULT '',
			created_by TEXT NOT NULL DEFAULT '',
			created_by_name TEXT NOT NULL DEFAULT '',
			expires_at TEXT NOT NULL DEFAULT '',
			emailed_at TEXT NOT NULL DEFAULT '',
			email_pending BOOLEAN NOT NULL DEFAULT FALSE,
			emailed_count INTEGER NOT NULL DEFAULT 0
		)`,
		// N°165 — annonces programmées : date de diffusion + e-mail différé
		// (migration douce pour les bases créées avant, idempotente).
		`ALTER TABLE announcements ADD COLUMN IF NOT EXISTS publish_at TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE announcements ADD COLUMN IF NOT EXISTS email_pending BOOLEAN NOT NULL DEFAULT FALSE`,
		`CREATE TABLE IF NOT EXISTS chat_messages (
                        id              TEXT PRIMARY KEY,
                        conversation_id TEXT NOT NULL,
                        sender          TEXT NOT NULL,
                        body            TEXT NOT NULL,
                        at              TEXT NOT NULL
                )`,
		`CREATE INDEX IF NOT EXISTS idx_chat_messages_conversation ON chat_messages (conversation_id)`,
		// N°67 — Resend (API HTTP https://resend.com) comme fournisseur
		// alternatif du canal e-mail : le provider choisit entre SMTP
		// direct (défaut, '') et l'API Resend (clé secrète par compte).
		`ALTER TABLE notif_settings ADD COLUMN IF NOT EXISTS email_provider TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE notif_settings ADD COLUMN IF NOT EXISTS resend_api_key TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE notif_settings ADD COLUMN IF NOT EXISTS resend_from TEXT NOT NULL DEFAULT ''`,
		// N (rapprochement doux) — utilisateur absent du dernier read_state du
		// routeur (supprimé dans Winbox) : badge + action de resynchronisation.
		`ALTER TABLE hotspot_users ADD COLUMN IF NOT EXISTS missing_on_router BOOLEAN NOT NULL DEFAULT FALSE`,
		// QR vouchers : page de login du portail captive du routeur.
		`ALTER TABLE routers ADD COLUMN IF NOT EXISTS hotspot_login_url TEXT NOT NULL DEFAULT ''`,
		// Migrations douces pour les bases créées avant l'ajout des champs
		// agent/Wave/commandes (idempotentes, donc sans risque au premier déploiement).
		`ALTER TABLE routers ADD COLUMN IF NOT EXISTS agent_token_hash TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE routers ADD COLUMN IF NOT EXISTS token_preview TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE routers ADD COLUMN IF NOT EXISTS last_seen TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS wave_link TEXT NOT NULL DEFAULT ''`,
		// Paiement en ligne (GeniusPay -> Wave) : référence de transaction
		// marchande enregistrée sur la demande pour l'appariement webhook.
		`ALTER TABLE billing_requests ADD COLUMN IF NOT EXISTS gateway_ref TEXT NOT NULL DEFAULT ''`,
		// Répercussion des frais (stratégie validée) : base net cible + moyen
		// de paiement actif de la demande (wave | card).
		`ALTER TABLE billing_requests ADD COLUMN IF NOT EXISTS base_amount_fcfa INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE billing_requests ADD COLUMN IF NOT EXISTS pay_method TEXT NOT NULL DEFAULT ''`,
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
		// N°45 — bannière du portail captif (data URL ≤ 500 Ko ou URL https R2).
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS banner_url TEXT NOT NULL DEFAULT ''`,
		// N°55 — mode hospitalité du portail (style, bienvenue, promos et
		// réseaux sociaux en JSON ; pattern colonnes à-plats du repo).
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS portal_style TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS portal_welcome TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS portal_promos TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS portal_socials TEXT NOT NULL DEFAULT ''`,
		// N°136 — slides du carrousel commercial du portail captif (JSON
		// ["url",…] ≤ 3 URLs https ; pattern colonnes à-plats du repo).
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS portal_slides TEXT NOT NULL DEFAULT ''`,
		// N°137 — services de l'établissement du portail captif (JSON [{icon,label}]
		// ≤ 6, section « Nos Services » du mode commercial).
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS portal_services TEXT NOT NULL DEFAULT ''`,
		// N°138 — messages du bandeau animé sous le logo du portail captif
		// (JSON ["msg",…] ≤ 5, effet Typed.js du login.html).
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS portal_ticker TEXT NOT NULL DEFAULT ''`,
		// N°139 — numéro WhatsApp SUPPORT du portail captif (JSON
		// {"number","label"} — lien wa.me du footer login/logout/error).
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS portal_whatsapp TEXT NOT NULL DEFAULT ''`,
		// N°56 — clé publique du portail (analytics pré-auth) : générée
		// une fois par compte côté Go (ensureSettings), simple TEXT.
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS portal_key TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS expiry_policy_mode TEXT NOT NULL DEFAULT 'keep'`,
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS expiry_policy_after_days INTEGER NOT NULL DEFAULT 30`,
		// N°65 — rétention du journal utilisateurs PAR COMPTE (30/60/90 j,
		// défaut 90 : la colonne reporte la valeur effective au premier Save).
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS log_retention_days INTEGER NOT NULL DEFAULT 90`,
		`ALTER TABLE routers ADD COLUMN IF NOT EXISTS board_name TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE routers ADD COLUMN IF NOT EXISTS free_hdd_mb INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE routers ADD COLUMN IF NOT EXISTS total_hdd_mb INTEGER NOT NULL DEFAULT 0`,
		// N°75 — veille adaptative : intervalle courant du scheduler MikCloud
		// sur ce routeur (0 = antérieur au N°75 → 45 s implicites).
		`ALTER TABLE routers ADD COLUMN IF NOT EXISTS scheduler_sec INTEGER NOT NULL DEFAULT 0`,
		// N°77 — veilleur d'invités : scheduler mikcloud-watch confirmé déployé
		// sur ce routeur (posé au retour « ok » de watcher_ensure uniquement —
		// pattern walled-garden : un échec re-file au check-in suivant).
		`ALTER TABLE routers ADD COLUMN IF NOT EXISTS watcher_ok BOOLEAN NOT NULL DEFAULT FALSE`,
		// N°80 — SafeWiFi : niveau de filtrage DNS du WiFi public
		// (off/threats/family ; "" = antérieur au N°80 → off implicite
		// sans commande), signature de la config appliquée et horodatage
		// de la dernière confirmation (pattern walled-garden N°29/N°49).
		`ALTER TABLE routers ADD COLUMN IF NOT EXISTS safewifi_level TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE routers ADD COLUMN IF NOT EXISTS safewifi_sig TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE routers ADD COLUMN IF NOT EXISTS safewifi_applied_at TEXT NOT NULL DEFAULT ''`,
		// N°81 — Shield : bouclier réseau du WiFi public (off/on ; "" =
		// antérieur au N°81 → off implicite sans commande), signature de
		// la config appliquée et horodatage de la dernière confirmation
		// (pattern SafeWiFi N°80).
		`ALTER TABLE routers ADD COLUMN IF NOT EXISTS shield_level TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE routers ADD COLUMN IF NOT EXISTS shield_sig TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE routers ADD COLUMN IF NOT EXISTS shield_applied_at TEXT NOT NULL DEFAULT ''`,
		// N°82 — FamilyGuard : couvre-feu internet du WiFi public
		// (fenêtre canonique "1|22:00|06:00|1111111" ; "" = jamais
		// utilisé → aucune commande), signature de la config appliquée
		// et horodatage de la dernière confirmation (pattern N°80/N°81).
		`ALTER TABLE routers ADD COLUMN IF NOT EXISTS familyguard_spec TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE routers ADD COLUMN IF NOT EXISTS familyguard_sig TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE routers ADD COLUMN IF NOT EXISTS familyguard_applied_at TEXT NOT NULL DEFAULT ''`,
		// N°88 — AntiVPN : bloque-VPN du WiFi public (off/on ; "" =
		// antérieur au N°88 → off implicite sans commande), signature de
		// la config appliquée et horodatage de la dernière confirmation
		// (pattern SafeWiFi N°80 / Shield N°81).
		`ALTER TABLE routers ADD COLUMN IF NOT EXISTS antivpn_level TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE routers ADD COLUMN IF NOT EXISTS antivpn_sig TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE routers ADD COLUMN IF NOT EXISTS antivpn_applied_at TEXT NOT NULL DEFAULT ''`,
		// N°97 — docteur pool IP : capacité (PoolCap), hôtes occupant une
		// IP (PoolHosts, authentifiés + zombies), ranges affichés et date
		// du dernier diagnostic (cf. internal/api/agent_pool.go).
		`ALTER TABLE routers ADD COLUMN IF NOT EXISTS pool_cap INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE routers ADD COLUMN IF NOT EXISTS pool_hosts INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE routers ADD COLUMN IF NOT EXISTS pool_ranges TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE routers ADD COLUMN IF NOT EXISTS pool_doctor_at TEXT NOT NULL DEFAULT ''`,
		// N°99 — auto-réparation opt-in du pool : switch du gérant
		// (PoolAuto) + marqueur d'auto-réparation en attente du
		// check-in (PoolAutoPending, cf. internal/api/agent_pool.go).
		`ALTER TABLE routers ADD COLUMN IF NOT EXISTS pool_auto BOOLEAN NOT NULL DEFAULT FALSE`,
		`ALTER TABLE routers ADD COLUMN IF NOT EXISTS pool_auto_pending BOOLEAN NOT NULL DEFAULT FALSE`,
		// N°101 — pause dîner HomeNet : signature de l'ensemble pause appliqué
		// sur ce routeur (hash de la liste des MAC en pause, sel dp-v1 — posée
		// au retour « ok » VÉRIFIÉ de la commande device_pause, cf. models.go).
		`ALTER TABLE routers ADD COLUMN IF NOT EXISTS pause_sig TEXT NOT NULL DEFAULT ''`,
		// N°103 — qualité de ligne (mesure passive du débit FAI) : interface
		// WAN détectée par read_state (route par défaut active) + capacité
		// ligne déclarée par le gérant, PAR ROUTEUR (chaque site a son FAI).
		`ALTER TABLE routers ADD COLUMN IF NOT EXISTS wan_iface     TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE routers ADD COLUMN IF NOT EXISTS line_down_bps BIGINT NOT NULL DEFAULT 0`,
		`ALTER TABLE routers ADD COLUMN IF NOT EXISTS line_up_bps   BIGINT NOT NULL DEFAULT 0`,
		// N°104 — QoS Manager : plafond agrégat du hotspot (file
		// mikcloud-qos convergée au check-in, pattern walled_garden).
		`ALTER TABLE routers ADD COLUMN IF NOT EXISTS qos_enabled     BOOLEAN NOT NULL DEFAULT FALSE`,
		`ALTER TABLE routers ADD COLUMN IF NOT EXISTS qos_target      TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE routers ADD COLUMN IF NOT EXISTS qos_max_up_bps   BIGINT NOT NULL DEFAULT 0`,
		`ALTER TABLE routers ADD COLUMN IF NOT EXISTS qos_max_down_bps BIGINT NOT NULL DEFAULT 0`,
		`ALTER TABLE routers ADD COLUMN IF NOT EXISTS qos_sig         TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE routers ADD COLUMN IF NOT EXISTS qos_applied_at  TEXT NOT NULL DEFAULT ''`,
		// N°97 — état anti-spam alerte pool (routerID → high|full), JSON
		// sérialisé comme stock_alert_state.
		`ALTER TABLE notif_settings ADD COLUMN IF NOT EXISTS pool_alert_state TEXT NOT NULL DEFAULT ''`,
		// N°97-ter — re-diagnostic immédiat des routeurs dont le diagnostic
		// N°97 a abouti SANS capacité (PoolCap=0 + PoolDoctorAt posé) : le
		// parseur N°97-ter lit aussi les pools des serveurs DHCP des
		// interfaces hotspot (cas réel ProMax WIFI : profil sans pool,
		// DHCP du bridge). La condition '' <> '' sur les jamais-diagnostiqués
		// les laisse tranquilles ; un routeur réellement sans pool
		// re-diagnostiquera à chaque redémarrage cloud (rare, une commande
		// de lecture).
		`UPDATE routers SET pool_doctor_at = '' WHERE pool_cap = 0 AND pool_doctor_at <> ''`,
		// N°108 — le docteur N°97 lisait address-pool sur le PROFIL
		// hotspot (propriété INEXISTANTE — elle vit sur /ip hotspot,
		// le SERVEUR) : capacité sous-estimée sur les topologies
		// « pool serveur ≠ pool DHCP » et extension qui n'a jamais
		// touché le vrai fournisseur d'adresses (constat production
		// ProMax WIFI). La sémantique du rapport change (serveurs à
		// 8 champs, DHCP à 4, liste profils retirée) : re-diagnostic
		// de tous les routeurs agent au premier check-in suivant ce
		// démarrage — lecture seule, une commande par routeur par
		// démarrage cloud (rare), pattern de convergence du parc.
		`UPDATE routers SET pool_doctor_at = '' WHERE mode = 'agent' AND pool_doctor_at <> ''`,
		// Sécurité S6 — détection d'identité routeur dupliquée (conflit
		// inter-comptes, cf. internal/api/agent_handlers.go).
		`ALTER TABLE routers ADD COLUMN IF NOT EXISTS identity_conflict BOOLEAN NOT NULL DEFAULT FALSE`,
		// N°29 — signature de la config walled-garden d'inscription publique
		// déjà appliquée sur ce routeur agent (cf. agent_handlers.go).
		`ALTER TABLE routers ADD COLUMN IF NOT EXISTS walled_garden_sig TEXT NOT NULL DEFAULT ''`,
		// N°49 — horodatage de la dernière application confirmée du
		// walled-garden (auto-réparation périodique, cf. agent_handlers.go
		// ensureWalledGardenLocked / walledGardenFresh).
		`ALTER TABLE routers ADD COLUMN IF NOT EXISTS walled_garden_applied_at TEXT NOT NULL DEFAULT ''`,
		// N°35 — signature du portail captif déjà déployé avec succès sur
		// ce routeur agent (cf. agent_handlers.go ensureHotspotFilesLocked).
		// Pattern identique à walled_garden_sig : posée au retour « ok »,
		// vidée sur échec, comparée à chaque check-in pour re-file auto.
		`ALTER TABLE routers ADD COLUMN IF NOT EXISTS hotspot_files_sig TEXT NOT NULL DEFAULT ''`,
		// N°33 — MAC de l'appareil d'inscription publique (page login du
		// routeur, ?mac=) : anti-abus par appareil derrière le NAT du hotspot.
		`ALTER TABLE registration_requests ADD COLUMN IF NOT EXISTS created_mac TEXT NOT NULL DEFAULT ''`,
		// Quota de données par voucher (« 5 Go = 500 F ») : Mo, 0 = illimité.
		`ALTER TABLE hotspot_users ADD COLUMN IF NOT EXISTS data_quota_mb BIGINT NOT NULL DEFAULT 0`,
		`ALTER TABLE batches       ADD COLUMN IF NOT EXISTS data_quota_mb BIGINT NOT NULL DEFAULT 0`,
		// Parité Mikhmon : address-pool / parent-queue du profil, validité
		// fine (minutes) et Time Limit par lot (limit-uptime RouterOS).
		`ALTER TABLE profiles      ADD COLUMN IF NOT EXISTS address_pool  TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE profiles      ADD COLUMN IF NOT EXISTS parent_queue  TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE profiles      ADD COLUMN IF NOT EXISTS validity_min  INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE hotspot_users ADD COLUMN IF NOT EXISTS time_limit_min BIGINT NOT NULL DEFAULT 0`,
		`ALTER TABLE batches       ADD COLUMN IF NOT EXISTS time_limit_min BIGINT NOT NULL DEFAULT 0`,
		// N°106 — mode bridage (quota data) : profil (cut/throttle + débit de
		// bridage), flag « session bridée » (file mikthrottle- présente sur le
		// routeur) et scheduler mikcloud-quota confirmé déployé (pattern watcher
		// N°77). Migrations idempotentes — boot Render.
		`ALTER TABLE profiles      ADD COLUMN IF NOT EXISTS quota_mode    TEXT NOT NULL DEFAULT 'cut'`,
		`ALTER TABLE profiles      ADD COLUMN IF NOT EXISTS throttle_rate TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE sessions      ADD COLUMN IF NOT EXISTS throttled     BOOLEAN NOT NULL DEFAULT FALSE`,
		`ALTER TABLE routers       ADD COLUMN IF NOT EXISTS quota_sched_ok BOOLEAN NOT NULL DEFAULT FALSE`,
		// N°113 — génération du script du tick mikcloud-quota confirmée sur
		// le routeur (0 = pré-N°113 : le check-in re-file quota_ensure pour
		// servir le tick v2 — ancrage EN TÊTE de liste, cf. agent.QuotaTickVersion).
		`ALTER TABLE routers       ADD COLUMN IF NOT EXISTS quota_sched_ver INTEGER NOT NULL DEFAULT 0`,
		// Abonnement SaaS (formules Essentiel 1 250 F/mois/routeur et
		// Illimité 12 000 F/an, routeurs illimités) : état par compte dans settings.
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS sub_plan_id      TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS sub_status       TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS sub_period_start TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS sub_period_end   TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS sub_last_amount  INTEGER NOT NULL DEFAULT 0`,
		// P2/P3 (console plateforme) — quotas Essentiel (routeurs couverts) et
		// traçabilité du paiement marqué par la plateforme.
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS sub_router_slots INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS sub_last_paid_at TEXT NOT NULL DEFAULT ''`,
		// Migrations multi-tenant : colonne account_id sur toutes les tables métier.
		`ALTER TABLE admin_users   ADD COLUMN IF NOT EXISTS account_id TEXT NOT NULL DEFAULT ''`,
		// Changement de mot de passe par l'utilisateur (POST /api/auth/password) :
		// password_set_by_user protège le mot de passe contre l'override ADMIN_PASSWORD
		// tant que la variable ne change pas ; env_password_hash mémorise le dernier
		// mot de passe env appliqué pour détecter un changement d'intention opérateur.
		`ALTER TABLE admin_users   ADD COLUMN IF NOT EXISTS password_set_by_user BOOLEAN NOT NULL DEFAULT FALSE`,
		`ALTER TABLE admin_users   ADD COLUMN IF NOT EXISTS env_password_hash TEXT NOT NULL DEFAULT ''`,
		// Sécurité S1-A3 — époque de session : compteur de révocation par
		// utilisateur. Incrémenté à chaque opération sensible (mot de passe
		// changé/réinitialisé, rôle modifié) ; tout token dont le claim « ver »
		// diffère de la valeur stockée est refusé immédiatement par le middleware.
		`ALTER TABLE admin_users   ADD COLUMN IF NOT EXISTS session_epoch INTEGER NOT NULL DEFAULT 0`,
		// Sécurité S4 — 2FA TOTP : secret base32 (jamais sérialisé en
		// JSON) et état activé. Valeurs par défaut neutres : aucun
		// utilisateur existant n'est changé par la migration.
		`ALTER TABLE admin_users   ADD COLUMN IF NOT EXISTS totp_secret  TEXT    NOT NULL DEFAULT ''`,
		`ALTER TABLE admin_users   ADD COLUMN IF NOT EXISTS totp_enabled BOOLEAN NOT NULL DEFAULT FALSE`,
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
		// I (paramètres plateforme) — config globale du SaaS (compte principal
		// uniquement) : nom affiché + gestion des inscriptions.
		`ALTER TABLE settings      ADD COLUMN IF NOT EXISTS platform_name        TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE settings      ADD COLUMN IF NOT EXISTS platform_register_open BOOLEAN NOT NULL DEFAULT TRUE`,
		`ALTER TABLE settings      ADD COLUMN IF NOT EXISTS platform_register_key  TEXT NOT NULL DEFAULT ''`,
		// La ligne singleton settings historique (id INTEGER 1) devient la ligne du
		// compte principal : id = account_id, une ligne par compte.
		`ALTER TABLE settings ALTER COLUMN id TYPE TEXT`,
		// Backfill : toutes les données de l'ère mono-tenant vont au compte principal.
		// Les administrateurs plateforme (admin/platform_admin) restent SANS
		// compte client — opérateurs du SaaS, pas clients.
		`UPDATE admin_users   SET account_id = 'acc-main' WHERE account_id = '' AND role NOT IN ('platform_admin','admin')`,
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
		// v2 — anti-partage : verrou « 1er appareil » par profil (liaison MAC
		// appliquée par le script on-login généré côté agent).
		`ALTER TABLE profiles ADD COLUMN IF NOT EXISTS lock_first_device BOOLEAN NOT NULL DEFAULT FALSE`,
		// v2 — profil « Staff » par défaut : présent sur CHAQUE compte. La marque
		// staff_seeded (colonne DB du compte, hors miroir Go) garantit le passage
		// unique : un Staff supprimé par le client n'est pas ressuscité au boot.
		`ALTER TABLE accounts ADD COLUMN IF NOT EXISTS staff_seeded BOOLEAN NOT NULL DEFAULT FALSE`,
		`INSERT INTO profiles (id, name, rate_limit, session_timeout_min, shared_users, validity_days, price, data_quota_mb, created_at, account_id, exp_mode, grace_period_min, lock_user, selling_price, lock_first_device)
                        SELECT 'p-staff-' || a.id, 'Staff', '10M/10M', 43200, 2, 30, 0, 0,
                               TO_CHAR(NOW() AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
                               a.id, 'notify', 0, FALSE, 0, FALSE
                        FROM accounts a
                        WHERE a.staff_seeded = FALSE
                          AND NOT EXISTS (SELECT 1 FROM profiles p WHERE p.account_id = a.id AND LOWER(p.name) = 'staff')`,
		`UPDATE accounts SET staff_seeded = TRUE WHERE staff_seeded = FALSE`,
	}
	// N°166 — RLS systématique : sur un hébergeur mutualisé doté d'une API
	// Data (Supabase : PostgREST + clé publishable), une table sans RLS
	// est LISIBLE par les rôles anon/authenticated. ENABLE ROW LEVEL
	// SECURITY sans politique = aucune ligne visible pour eux ; le compte
	// applicatif (propriétaire des tables) contourne RLS — zéro effet sur
	// le backend, ordre inerte sur Neon. Idempotent : rejoué à chaque
	// boot, la passe protège aussi les tables restaurées d'un dump sans
	// RLS (migration du 1er octobre) et toute table future.
	stmts = append(stmts, rlsStatements()...)
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
// base est vide (l'appelant construira l'état de mise en service BuildEmptyState
// — zéro démo — puis déclenchera la synchronisation initiale).

// migrateSealRouterPasswords — passe de démarrage (idempotente) : chiffre
// TOUTES les valeurs de routers.password encore en clair (base créée avant le
// correctif P0 #6). La mémoire reste claire ; seules les lignes DB concernées
// sont réécrites, une seule fois.
func (p *PG) migrateSealRouterPasswords() {
	rows, err := p.db.Query(`SELECT id, password FROM routers WHERE password <> ''`)
	if err != nil {
		log.Printf("secretbox: migration des mots de passe routeur impossible (%v) — retentée à l'écriture suivante", err)
		return
	}
	type plain struct {
		id string
		pw string
	}
	var todo []plain
	for rows.Next() {
		var t plain
		if err := rows.Scan(&t.id, &t.pw); err != nil {
			rows.Close()
			return
		}
		if t.pw != "" && !secretbox.IsEncrypted(t.pw) {
			todo = append(todo, t)
		}
	}
	rows.Close()
	for _, t := range todo {
		enc := secretbox.Encrypt(t.pw)
		if enc == "" || !secretbox.IsEncrypted(enc) {
			continue // refus d'écrire un pseudo-chiffré (cf. secretbox.Encrypt)
		}
		if _, err := p.db.Exec(`UPDATE routers SET password = $1 WHERE id = $2`, enc, t.id); err != nil {
			log.Printf("secretbox: chiffrement du routeur %s différé (%v)", t.id, err)
			continue
		}
		log.Printf("secretbox: mot de passe routeur %s chiffré au repos", t.id)
	}
	if len(todo) > 0 {
		log.Printf("secretbox: migration P0 #6 terminée — %d mot(s) de passe routeur chiffré(s)", len(todo))
	}
}

// migrateSealSecretColumns — N°75 — passe de démarrage (idempotente) :
// chiffre les secrets de notification (tokens bots Telegram/WhatsApp, clé
// Resend, mot de passe SMTP) et les secrets 2FA (admin_users.totp_secret)
// encore stockés en clair (base antérieure au correctif). Sans cette passe,
// les valeurs existantes resteraient en clair à vie : l'empreinte de synchro
// ne voit aucune différence entre clair et chiffré (elle porte l'état mémoire
// clair des deux côtés) — seules les NOUVELLES écritures chiffreraient.

// migrateSealSecretColumns — N°75 — passe de démarrage (idempotente) :
// chiffre les secrets de notification (tokens bots Telegram/WhatsApp, clé
// Resend, mot de passe SMTP) et les secrets 2FA (admin_users.totp_secret)
// encore stockés en clair (base antérieure au correctif). Sans cette passe,
// les valeurs existantes resteraient en clair à vie : l'empreinte de synchro
// ne voit aucune différence entre clair et chiffré (elle porte l'état mémoire
// clair des deux côtés) — seules les NOUVELLES écritures chiffreraient.
func (p *PG) migrateSealSecretColumns() {
	for _, c := range []struct{ table, col, label string }{
		{"notif_settings", "telegram_bot_token", "token Telegram"},
		{"notif_settings", "whatsapp_token", "token WhatsApp"},
		{"notif_settings", "resend_api_key", "clé Resend"},
		{"notif_settings", "smtp_pass", "mot de passe SMTP"},
		{"admin_users", "totp_secret", "secret 2FA"},
	} {
		rows, err := p.db.Query(fmt.Sprintf("SELECT id, %s FROM %s WHERE %s <> ''", c.col, c.table, c.col))
		if err != nil {
			log.Printf("secretbox: lecture %s.%s impossible (%v) — retentée à l'écriture suivante", c.table, c.col, err)
			continue
		}
		type plain struct {
			id  string
			val string
		}
		var todo []plain
		for rows.Next() {
			var t plain
			if err := rows.Scan(&t.id, &t.val); err != nil {
				rows.Close()
				break
			}
			if t.val != "" && !secretbox.IsEncrypted(t.val) {
				todo = append(todo, t)
			}
		}
		rows.Close()
		done := 0
		for _, t := range todo {
			enc := secretbox.Encrypt(t.val)
			if enc == "" || !secretbox.IsEncrypted(enc) {
				continue // refus d'écrire un pseudo-chiffré (cf. secretbox.Encrypt)
			}
			if _, err := p.db.Exec(fmt.Sprintf("UPDATE %s SET %s = $1 WHERE id = $2", c.table, c.col), enc, t.id); err != nil {
				log.Printf("secretbox: chiffrement %s de %s différé (%v)", c.label, t.id, err)
				continue
			}
			done++
		}
		if done > 0 {
			log.Printf("secretbox: N°75 — %d %s chiffré(s) au repos (%s.%s)", done, c.label, c.table, c.col)
		}
	}
}
