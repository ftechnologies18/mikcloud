// Chargement de l'état depuis PostgreSQL (Load + paramètres).
// Extrait du monolithe pg.go (N°88) — même package, contenu inchangé.
package store

import (
	"database/sql"
	"fmt"
	"mikcloud/hotspot-api/internal/model"
)

// ---------------------------------------------------------------------------
// Chargement au démarrage
// ---------------------------------------------------------------------------

// Load relit toute la base et reconstruit *model.DB. Retourne found=false si la
// base est vide (l'appelant construira l'état de mise en service BuildEmptyState
// — zéro démo — puis déclenchera la synchronisation initiale).
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
		LineQuality:       []model.LineQualityDay{},
		NotifSettings:     map[string]model.NotificationSettings{},
		NotifLog:          []model.NotificationLog{},
		BillingRequests:   []model.BillingRequest{},
		WifiSites:         []model.WifiSite{},
		WifiGuests:        []model.WifiGuest{},
		Devices:           []model.Device{},
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
		{"sell_sessions", func() error { return loadInto(p, &db.SellSessions, sellSessionSpec) }},
		{"password_resets", func() error { return loadInto(p, &db.PasswordResets, passwordResetSpec) }},
		{"transactions", func() error { return loadInto(p, &db.Transactions, transactionSpec) }},
		{"sessions", func() error { return loadInto(p, &db.Sessions, sessionSpec) }},
		{"devices", func() error { return loadInto(p, &db.Devices, deviceSpec) }},
		{"activity", func() error { return loadInto(p, &db.Activity, activitySpec) }},
		{"sales", func() error { return loadInto(p, &db.Sales, saleSpec) }},
		{"commands", func() error { return loadInto(p, &db.Commands, commandSpec) }},
		{"voucher_templates", func() error { return loadInto(p, &db.Templates, templateSpec) }},
		{"user_logs", func() error { return loadInto(p, &db.UserLogs, userLogSpec) }},
		{"ip_bindings", func() error { return loadInto(p, &db.IPBindings, ipBindingSpec) }},
		{"scheduler_tasks", func() error { return loadInto(p, &db.SchedulerTasks, schedulerTaskSpec) }},
		{"traffic", func() error { return loadInto(p, &db.Traffic, trafficSpec) }},
		{"line_quality", func() error { return loadInto(p, &db.LineQuality, lineQualitySpec) }},
		{"notif_settings", func() error { return p.loadNotifSettings(db) }},
		{"notif_log", func() error { return loadInto(p, &db.NotifLog, notifLogSpec) }},
		{"billing_requests", func() error { return loadInto(p, &db.BillingRequests, billingRequestSpec) }},
		{"geniuspay_subs", func() error { return loadInto(p, &db.GeniusPaySubs, geniusPaySubSpec) }},
		{"purge_tombstones", func() error { return loadInto(p, &db.PurgeTombstones, purgeTombstoneSpec) }},
		{"join_links", func() error { return loadInto(p, &db.JoinLinks, joinLinkSpec) }},
		{"registration_requests", func() error { return loadInto(p, &db.RegistrationRequests, registrationRequestSpec) }},
		{"wifi_sites", func() error { return loadInto(p, &db.WifiSites, wifiSiteSpec) }},
		{"wifi_guests", func() error { return loadInto(p, &db.WifiGuests, wifiGuestSpec) }},
		{"promo_events", func() error { return loadInto(p, &db.PromoEvents, promoEventSpec) }},
		{"chat_conversations", func() error { return loadInto(p, &db.ChatConversations, chatConversationSpec) }},
		{"chat_messages", func() error { return loadInto(p, &db.ChatMessages, chatMessageSpec) }},
		{"settings", func() error { return p.loadSettings(db) }},
	}
	for _, st := range steps {
		if err := st.query(); err != nil {
			return nil, false, fmt.Errorf("lecture %s : %w", st.name, err)
		}
	}

	// Sécurité P0 #6 — chiffre au repos les mots de passe routeur encore en
	// clair (base antérieure au correctif), AVANT que les handlers ne s'exécutent.
	p.migrateSealRouterPasswords()
	// N°75 — même passe pour les secrets de notification et 2FA (tokens
	// bots, clé Resend, mot de passe SMTP, secret TOTP).
	p.migrateSealSecretColumns()

	// Les tris applicatifs sont faits en Go (sort.Slice dans les handlers),
	// l'ordre de lecture n'a donc aucune importance.

	found = len(db.Users) > 0 || len(db.Accounts) > 0 || len(db.Routers) > 0 ||
		len(db.Profiles) > 0 || len(db.HotspotUsers) > 0 || len(db.Resellers) > 0 || len(db.Sales) > 0
	if !found {
		p.touchDB() // les tables ont bien été lues (base vide de mise en service)
		return nil, false, nil
	}

	// Le cache d'empreintes reflète l'état chargé.
	p.rebuildHashes(db)
	p.touchDB()
	return db, true, nil
}

// loadNotifSettings lit TOUTES les lignes notif_settings (une par compte SaaS)
// et remplit db.NotifSettings. stock_alert_state (TEXT) est désérialisé en map
// par le spec (pattern commandSpec).

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

// loadSettings lit TOUTES les lignes settings (une par compte SaaS) et remplit
// db.SettingsByAccount ; les lignes sans account_id (non migrées) sont ignorées.
// last_tick (global, moteur de simulation) est repris de la première ligne non vide.
func (p *PG) loadSettings(db *model.DB) error {
	rows, err := p.db.Query(
		`SELECT account_id, tenant_name, tenant_currency, tenant_timezone, plan_name, plan_max_routers, plan_max_users, wave_link,
                        dns_name, logo_url, banner_url, expiry_policy_mode, expiry_policy_after_days,
                        sub_plan_id, sub_status, sub_period_start, sub_period_end, sub_last_amount,
                        sub_router_slots, sub_last_paid_at, last_tick, last_sweep,
                        platform_name, platform_register_open, platform_register_key, auto_import_router_users,
                        join_button, portal_style, portal_welcome, portal_promos, portal_socials, portal_key,
                        log_retention_days
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
			waveLink, dnsName, logoURL, bannerURL      string
			expiryMode                                 string
			expiryAfterDays                            int
			subPlanID, subStatus                       string
			subPeriodStart, subPeriodEnd               string
			subLastAmount                              int
			subRouterSlots                             int
			subLastPaidAt                              string
			lastTick                                   sql.NullTime
			// N°64 — date du dernier balayage périodique (global).
			lastSweep sql.NullTime
			// Audit purge - reglage d'import automatique (defaut ON).
			autoImport bool
			// N°46 - reglage du bouton « S'inscrire » du portail (defaut ON).
			joinButton bool
			// N°55 - mode hospitalité du portail (style/bienvenue/promos/socials).
			portalStyle, portalWelcome, portalPromos, portalSocials string
			// N°56 - clé publique du portail (analytics pré-auth).
			portalKey string
			// N°65 - rétention du journal par compte (30/60/90 j, défaut 90).
			logRetentionDays int
			// I (paramètres plateforme) — uniquement sur le compte principal.
			platformName         string
			platformRegisterOpen bool
			platformRegisterKey  string
		)
		if err := rows.Scan(&accID, &tenantName, &tenantCurrency, &tenantTimezone,
			&planName, &planMaxRouters, &planMaxUsers, &waveLink,
			&dnsName, &logoURL, &bannerURL, &expiryMode, &expiryAfterDays,
			&subPlanID, &subStatus, &subPeriodStart, &subPeriodEnd, &subLastAmount,
			&subRouterSlots, &subLastPaidAt, &lastTick, &lastSweep,
			&platformName, &platformRegisterOpen, &platformRegisterKey, &autoImport,
			&joinButton, &portalStyle, &portalWelcome, &portalPromos, &portalSocials, &portalKey, &logRetentionDays); err != nil {
			return err
		}
		if accID == "" {
			continue // ligne historique non rattachée à un compte → ignorée
		}
		settings := model.Settings{
			Tenant: model.Tenant{
				Name: tenantName, Currency: tenantCurrency, Timezone: tenantTimezone,
				WaveLink: waveLink, DNSName: dnsName, LogoURL: logoURL,
				BannerURL:        bannerURL,
				ExpiryPolicyMode: expiryMode, ExpiryPolicyAfterDays: expiryAfterDays,
				PortalStyle: portalStyle, PortalWelcome: portalWelcome,
				PortalPromos: portalPromos, PortalSocials: portalSocials,
				PortalKey: portalKey,
			},
			Plan: model.Plan{Name: planName, MaxRouters: planMaxRouters, MaxUsers: planMaxUsers},
			Subscription: model.Subscription{
				PlanID: subPlanID, Status: subStatus,
				PeriodStart: subPeriodStart, PeriodEnd: subPeriodEnd, LastAmountFcfa: subLastAmount,
				RouterSlots: subRouterSlots, LastPaidAt: subLastPaidAt,
			},
		}
		// I (paramètres plateforme) — la config globale vit sur le compte principal.
		if accID == model.AccountMainID {
			settings.Platform = &model.PlatformConfig{
				Name:         platformName,
				RegisterOpen: platformRegisterOpen,
				RegisterKey:  platformRegisterKey,
			}
		}
		// Audit purge - valeur lue EXPLICITE (colonne NOT NULL) : le
		// reglage survit aux redemarrages (le pointeur est pose).
		settings.AutoImportRouterUsers = &autoImport
		// N°46 - meme pattern : valeur lue EXPLICITE (colonne NOT NULL
		// DEFAULT TRUE), le reglage du bouton « S'inscrire » survit aux
		// redemarrages.
		settings.Tenant.JoinButton = &joinButton
		// N°65 - même pattern : valeur lue EXPLICITE (colonne NOT NULL
		// DEFAULT 90), la rétention du journal survit aux redémarrages.
		settings.Tenant.LogRetentionDays = &logRetentionDays
		db.SettingsByAccount[accID] = settings
		if lastTick.Valid && db.LastTick.IsZero() {
			db.LastTick = lastTick.Time
		}
		// N°64 — même règle que last_tick : global, pris sur la
		// première ligne non vide (le balayage n'est pas par compte).
		if lastSweep.Valid && db.LastSweep.IsZero() {
			db.LastSweep = lastSweep.Time
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
