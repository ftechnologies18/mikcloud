// Spécifications d'entités (tables, clés, hash) pour la synchro PostgreSQL.
// Extrait du monolithe pg.go (N°88) — même package, contenu inchangé.
package store

import (
	"database/sql"
	"encoding/json"
	"hash/fnv"
	"mikcloud/hotspot-api/internal/model"
	"mikcloud/hotspot-api/internal/secretbox"
)

// ---------------------------------------------------------------------------
// Specs concrètes des 10 tables entité.
// IMPORTANT : l'ordre des colonnes doit rester strictement identique entre
// cols, scan et args.
// ---------------------------------------------------------------------------

// accountSpec — comptes clients SaaS (isolation multi-tenant).
var accountSpec = entitySpec[model.Account]{
	table: "accounts",
	// N°98 — colonne usage en fin de liste (Hotspot vs HomeNet) : l'ordre
	// cols/scan/args reste aligné, et l'ALTER idempotent du schéma garantit
	// la colonne sur les bases préexistantes (défaut « hotspot »).
	cols: []string{"id", "name", "status", "created_at", "email", "phone", "country", "city", "usage"},
	idOf: func(x *model.Account) string { return x.ID },
	scan: func(r *sql.Rows) (model.Account, error) {
		var x model.Account
		err := r.Scan(&x.ID, &x.Name, &x.Status, &x.CreatedAt, &x.Email, &x.Phone, &x.Country, &x.City, &x.Usage)
		return x, err
	},
	args: func(x *model.Account) []any {
		return []any{x.ID, x.Name, x.Status, x.CreatedAt, x.Email, x.Phone, x.Country, x.City, x.Usage}
	},
	hashOf: hashEntity[model.Account],
}

var adminSpec = entitySpec[model.AdminUser]{
	table: "admin_users",
	cols:  []string{"id", "name", "username", "role", "password_hash", "salt", "created_at", "account_id", "password_set_by_user", "env_password_hash", "session_epoch", "totp_secret", "totp_enabled"},
	idOf:  func(u *model.AdminUser) string { return u.ID },
	scan: func(r *sql.Rows) (model.AdminUser, error) {
		var u model.AdminUser
		err := r.Scan(&u.ID, &u.Name, &u.Username, &u.Role, &u.PasswordHash, &u.Salt, &u.CreatedAt, &u.AccountID, &u.PasswordSetByUser, &u.EnvPasswordHash, &u.SessionEpoch, &u.TOTPSecret, &u.TOTPEnabled)
		// N°75 — le secret 2FA est stocké chiffré (AES-256-GCM) :
		// lecture = déchiffrement (passthrough si valeur antérieure,
		// migration assurée par migrateSealSecretColumns).
		u.TOTPSecret = secretbox.Decrypt(u.TOTPSecret)
		return u, err
	},
	args: func(u *model.AdminUser) []any {
		return []any{u.ID, u.Name, u.Username, u.Role, u.PasswordHash, u.Salt, u.CreatedAt, u.AccountID, u.PasswordSetByUser, u.EnvPasswordHash, u.SessionEpoch, secretbox.Encrypt(u.TOTPSecret), u.TOTPEnabled}
	},
	// N°75 — empreinte QUI COUVRE LE SECRET 2FA : le modèle porte
	// json:"-" sur TOTPSecret (le secret ne sort jamais des réponses API),
	// mais hashEntity marshalle en JSON — sans cette ombre, un changement
	// de secret ne changeait PAS l'empreinte → AUCUN upsert → le secret
	// n'était persisté que par effet de bord du flip TOTPEnabled (un
	// redémarrage entre /2fa/setup et /2fa/activate le perdait).
	hashOf: adminUserHash,
}

// adminUserHashShadow — ombre de AdminUser EXPOSANT TOTPSecret au marshal
// (uniquement pour l'empreinte de synchro — jamais sérialisée ailleurs).

// adminUserHashShadow — ombre de AdminUser EXPOSANT TOTPSecret au marshal
// (uniquement pour l'empreinte de synchro — jamais sérialisée ailleurs).
type adminUserHashShadow struct {
	model.AdminUser
	TOTPSecret string `json:"totpSecret,omitempty"`
}

func adminUserHash(u *model.AdminUser) uint64 {
	b, err := json.Marshal(adminUserHashShadow{AdminUser: *u, TOTPSecret: u.TOTPSecret})
	if err != nil {
		return 0
	}
	h := fnv.New64a()
	h.Write(b)
	return h.Sum64()
}

var routerSpec = entitySpec[model.Router]{
	table: "routers",
	cols: []string{"id", "name", "host", "port", "username", "password", "mode", "status",
		"version", "uptime_sec", "cpu_load", "hotspot_users", "active_sessions", "created_at",
		"hotspot_login_url", "agent_token_hash", "token_preview", "last_seen", "account_id",
		"board_name", "free_hdd_mb", "total_hdd_mb", "identity_conflict", "walled_garden_sig", "walled_garden_applied_at", "hotspot_files_sig", "scheduler_sec", "watcher_ok",
		"safewifi_level", "safewifi_sig", "safewifi_applied_at",
		"shield_level", "shield_sig", "shield_applied_at",
		"familyguard_spec", "familyguard_sig", "familyguard_applied_at",
		"antivpn_level", "antivpn_sig", "antivpn_applied_at",
		"pool_cap", "pool_hosts", "pool_ranges", "pool_doctor_at",
		"pool_auto", "pool_auto_pending", "pause_sig",
		"wan_iface", "line_down_bps", "line_up_bps"},
	idOf: func(x *model.Router) string { return x.ID },
	scan: func(r *sql.Rows) (model.Router, error) {
		var x model.Router
		err := r.Scan(&x.ID, &x.Name, &x.Host, &x.Port, &x.Username, &x.Password, &x.Mode, &x.Status,
			&x.Version, &x.UptimeSec, &x.CPULoad, &x.HotspotUsers, &x.ActiveSessions, &x.CreatedAt,
			&x.HotspotLoginUrl, &x.AgentTokenHash, &x.TokenPreview, &x.LastSeen, &x.AccountID,
			&x.BoardName, &x.FreeHddMb, &x.TotalHddMb, &x.IdentityConflict, &x.WalledGardenSig, &x.WalledGardenAppliedAt, &x.HotspotFilesSig, &x.SchedulerSec, &x.WatcherOK,
			&x.SafeWifiLevel, &x.SafeWifiSig, &x.SafeWifiAppliedAt,
			&x.ShieldLevel, &x.ShieldSig, &x.ShieldAppliedAt,
			&x.FamilyGuardSpec, &x.FamilyGuardSig, &x.FamilyGuardAppliedAt,
			&x.AntiVpnLevel, &x.AntiVpnSig, &x.AntiVpnAppliedAt,
			&x.PoolCap, &x.PoolHosts, &x.PoolRanges, &x.PoolDoctorAt, &x.PoolAuto, &x.PoolAutoPending, &x.PauseSig,
			&x.WanIface, &x.LineDownBps, &x.LineUpBps)
		// Sécurité P0 #6 — le mot de passe routeur est stocké chiffré
		// (AES-256-GCM) : lecture = déchiffrement (passthrough si valeur
		// antérieure au correctif, migration assurée par
		// migrateSealRouterPasswords au démarrage).
		x.Password = secretbox.Decrypt(x.Password)
		return x, err
	},
	args: func(x *model.Router) []any {
		// Sécurité P0 #6 — écriture = chiffrement. L'empreinte de
		// synchronisation (hashOf) reste calculée sur l'état mémoire clair,
		// donc la valeur chiffrée (nonce aléatoire) ne provoque aucune
		// réécriture en boucle : seules les VRAIES modifications resynchronisent.
		return []any{x.ID, x.Name, x.Host, x.Port, x.Username, secretbox.Encrypt(x.Password), x.Mode, x.Status,
			x.Version, x.UptimeSec, x.CPULoad, x.HotspotUsers, x.ActiveSessions, x.CreatedAt,
			x.HotspotLoginUrl, x.AgentTokenHash, x.TokenPreview, x.LastSeen, x.AccountID,
			x.BoardName, x.FreeHddMb, x.TotalHddMb, x.IdentityConflict, x.WalledGardenSig, x.WalledGardenAppliedAt, x.HotspotFilesSig, x.SchedulerSec, x.WatcherOK,
			x.SafeWifiLevel, x.SafeWifiSig, x.SafeWifiAppliedAt,
			x.ShieldLevel, x.ShieldSig, x.ShieldAppliedAt,
			x.FamilyGuardSpec, x.FamilyGuardSig, x.FamilyGuardAppliedAt,
			x.AntiVpnLevel, x.AntiVpnSig, x.AntiVpnAppliedAt,
			x.PoolCap, x.PoolHosts, x.PoolRanges, x.PoolDoctorAt, x.PoolAuto, x.PoolAutoPending, x.PauseSig,
			x.WanIface, x.LineDownBps, x.LineUpBps}
	},
	hashOf: hashEntity[model.Router],
}

// migrateSealRouterPasswords — passe de démarrage (idempotente) : chiffre
// TOUTES les valeurs de routers.password encore en clair (base créée avant le
// correctif P0 #6). La mémoire reste claire ; seules les lignes DB concernées
// sont réécrites, une seule fois.

var profileSpec = entitySpec[model.Profile]{
	table: "profiles",
	cols:  []string{"id", "name", "rate_limit", "session_timeout_min", "shared_users", "validity_days", "price", "data_quota_mb", "created_at", "account_id", "exp_mode", "grace_period_min", "lock_user", "selling_price", "lock_first_device", "address_pool", "parent_queue", "validity_min"},
	idOf:  func(x *model.Profile) string { return x.ID },
	scan: func(r *sql.Rows) (model.Profile, error) {
		var x model.Profile
		err := r.Scan(&x.ID, &x.Name, &x.RateLimit, &x.SessionTimeoutMin, &x.SharedUsers,
			&x.ValidityDays, &x.Price, &x.DataQuotaMb, &x.CreatedAt, &x.AccountID,
			&x.ExpMode, &x.GracePeriodMin, &x.LockUser, &x.SellingPrice, &x.LockFirstDevice,
			&x.AddressPool, &x.ParentQueue, &x.ValidityMin)
		return x, err
	},
	args: func(x *model.Profile) []any {
		return []any{x.ID, x.Name, x.RateLimit, x.SessionTimeoutMin, x.SharedUsers,
			x.ValidityDays, x.Price, x.DataQuotaMb, x.CreatedAt, x.AccountID,
			x.ExpMode, x.GracePeriodMin, x.LockUser, x.SellingPrice, x.LockFirstDevice,
			x.AddressPool, x.ParentQueue, x.ValidityMin}
	},
	hashOf: hashEntity[model.Profile],
}

var hotspotUserSpec = entitySpec[model.HotspotUser]{
	table: "hotspot_users",
	cols: []string{"id", "kind", "username", "password", "profile_id", "profile_name",
		"router_id", "router_name", "status", "batch_id", "reseller_id", "reseller_name",
		"comment", "bytes_in", "bytes_out", "uptime_used_sec", "created_at", "expires_at", "used_at", "price", "data_quota_mb", "account_id",
		"selling_price", "enforced", "sold_at", "sold_via", "missing_on_router", "time_limit_min", "credit_sale"},
	idOf: func(x *model.HotspotUser) string { return x.ID },
	scan: func(r *sql.Rows) (model.HotspotUser, error) {
		var x model.HotspotUser
		err := r.Scan(&x.ID, &x.Kind, &x.Username, &x.Password, &x.ProfileID, &x.ProfileName,
			&x.RouterID, &x.RouterName, &x.Status, &x.BatchID, &x.ResellerID, &x.ResellerName,
			&x.Comment, &x.BytesIn, &x.BytesOut, &x.UptimeUsedSec, &x.CreatedAt, &x.ExpiresAt, &x.UsedAt, &x.Price, &x.DataQuotaMb, &x.AccountID,
			&x.SellingPrice, &x.Enforced, &x.SoldAt, &x.SoldVia, &x.MissingOnRouter, &x.TimeLimitMin, &x.CreditSale)
		return x, err
	},
	args: func(x *model.HotspotUser) []any {
		return []any{x.ID, x.Kind, x.Username, x.Password, x.ProfileID, x.ProfileName,
			x.RouterID, x.RouterName, x.Status, x.BatchID, x.ResellerID, x.ResellerName,
			x.Comment, x.BytesIn, x.BytesOut, x.UptimeUsedSec, x.CreatedAt, x.ExpiresAt, x.UsedAt, x.Price, x.DataQuotaMb, x.AccountID,
			x.SellingPrice, x.Enforced, x.SoldAt, x.SoldVia, x.MissingOnRouter, x.TimeLimitMin, x.CreditSale}
	},
	hashOf: hashEntity[model.HotspotUser],
}

var batchSpec = entitySpec[model.Batch]{
	table: "batches",
	cols:  []string{"id", "profile_id", "profile_name", "router_id", "router_name", "count", "unit_price", "total_cost", "data_quota_mb", "time_limit_min", "channel", "reseller_id", "reseller_name", "created_at", "account_id"},
	idOf:  func(x *model.Batch) string { return x.ID },
	scan: func(r *sql.Rows) (model.Batch, error) {
		var x model.Batch
		err := r.Scan(&x.ID, &x.ProfileID, &x.ProfileName, &x.RouterID, &x.RouterName,
			&x.Count, &x.UnitPrice, &x.TotalCost, &x.DataQuotaMb, &x.TimeLimitMin, &x.Channel, &x.ResellerID, &x.ResellerName, &x.CreatedAt, &x.AccountID)
		return x, err
	},
	args: func(x *model.Batch) []any {
		return []any{x.ID, x.ProfileID, x.ProfileName, x.RouterID, x.RouterName,
			x.Count, x.UnitPrice, x.TotalCost, x.DataQuotaMb, x.TimeLimitMin, x.Channel, x.ResellerID, x.ResellerName, x.CreatedAt, x.AccountID}
	},
	hashOf: hashEntity[model.Batch],
}

var resellerSpec = entitySpec[model.Reseller]{
	table: "resellers",
	cols:  []string{"id", "name", "username", "phone", "credit", "vouchers_sold", "revenue", "status", "created_at", "account_id", "pin_hash", "payment_mode", "debt_ceiling", "max_devices"},
	idOf:  func(x *model.Reseller) string { return x.ID },
	scan: func(r *sql.Rows) (model.Reseller, error) {
		var x model.Reseller
		err := r.Scan(&x.ID, &x.Name, &x.Username, &x.Phone, &x.Credit, &x.VouchersSold, &x.Revenue, &x.Status, &x.CreatedAt, &x.AccountID, &x.PinHash, &x.PaymentMode, &x.DebtCeiling, &x.MaxDevices)
		return x, err
	},
	args: func(x *model.Reseller) []any {
		return []any{x.ID, x.Name, x.Username, x.Phone, x.Credit, x.VouchersSold, x.Revenue, x.Status, x.CreatedAt, x.AccountID, x.PinHash, x.PaymentMode, x.DebtCeiling, x.MaxDevices}
	},
	hashOf: hashEntity[model.Reseller],
}

// sellSessionSpec — N°66 : registre des sessions PIN Mode Vente (revendeurs
// soumis à la limite d'appareils simultanés).

// sellSessionSpec — N°66 : registre des sessions PIN Mode Vente (revendeurs
// soumis à la limite d'appareils simultanés).
var sellSessionSpec = entitySpec[model.SellSession]{
	table: "sell_sessions",
	cols:  []string{"id", "account_id", "reseller_id", "issued_at", "last_seen", "user_agent", "ip"},
	idOf:  func(x *model.SellSession) string { return x.ID },
	scan: func(r *sql.Rows) (model.SellSession, error) {
		var x model.SellSession
		err := r.Scan(&x.ID, &x.AccountID, &x.ResellerID, &x.IssuedAt, &x.LastSeen, &x.UserAgent, &x.IP)
		return x, err
	},
	args: func(x *model.SellSession) []any {
		return []any{x.ID, x.AccountID, x.ResellerID, x.IssuedAt, x.LastSeen, x.UserAgent, x.IP}
	},
	hashOf: hashEntity[model.SellSession],
}

// passwordResetSpec — N°68 : liens de réinitialisation de mot de passe
// (token hashé SHA-256, TTL 60 min, usage unique).

// passwordResetSpec — N°68 : liens de réinitialisation de mot de passe
// (token hashé SHA-256, TTL 60 min, usage unique).
var passwordResetSpec = entitySpec[model.PasswordReset]{
	table: "password_resets",
	cols:  []string{"id", "account_id", "user_id", "token_hash", "expires_at", "used_at", "created_at", "created_ip"},
	idOf:  func(x *model.PasswordReset) string { return x.ID },
	scan: func(r *sql.Rows) (model.PasswordReset, error) {
		var x model.PasswordReset
		err := r.Scan(&x.ID, &x.AccountID, &x.UserID, &x.TokenHash, &x.ExpiresAt, &x.UsedAt, &x.CreatedAt, &x.CreatedIP)
		return x, err
	},
	args: func(x *model.PasswordReset) []any {
		return []any{x.ID, x.AccountID, x.UserID, x.TokenHash, x.ExpiresAt, x.UsedAt, x.CreatedAt, x.CreatedIP}
	},
	hashOf: hashEntity[model.PasswordReset],
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

// deviceSpec — N°101 : appareils des foyers HomeNet (bails DHCP + noms
// affectés + pause dîner). L'ordre cols/scan/args reste strictement aligné.
var deviceSpec = entitySpec[model.Device]{
	table: "devices",
	cols:  []string{"id", "account_id", "router_id", "router_name", "mac", "name", "hostname", "ip", "status", "expires", "lease_at", "created_at", "paused", "paused_until"},
	idOf:  func(x *model.Device) string { return x.ID },
	scan: func(r *sql.Rows) (model.Device, error) {
		var x model.Device
		err := r.Scan(&x.ID, &x.AccountID, &x.RouterID, &x.RouterName, &x.MAC, &x.Name, &x.Hostname,
			&x.IP, &x.Status, &x.Expires, &x.LeaseAt, &x.CreatedAt, &x.Paused, &x.PausedUntil)
		return x, err
	},
	args: func(x *model.Device) []any {
		return []any{x.ID, x.AccountID, x.RouterID, x.RouterName, x.MAC, x.Name, x.Hostname,
			x.IP, x.Status, x.Expires, x.LeaseAt, x.CreatedAt, x.Paused, x.PausedUntil}
	},
	hashOf: hashEntity[model.Device],
}

var activitySpec = entitySpec[model.Activity]{
	table: "activity",
	cols:  []string{"id", "type", "message", "at", "account_id", "actor_id", "actor_name"},
	idOf:  func(x *model.Activity) string { return x.ID },
	scan: func(r *sql.Rows) (model.Activity, error) {
		var x model.Activity
		err := r.Scan(&x.ID, &x.Type, &x.Message, &x.At, &x.AccountID, &x.ActorID, &x.ActorName)
		return x, err
	},
	args: func(x *model.Activity) []any {
		return []any{x.ID, x.Type, x.Message, x.At, x.AccountID, x.ActorID, x.ActorName}
	},
	hashOf: hashEntity[model.Activity],
}

// billingRequestSpec — demandes de souscription / renouvellement (facturation).

// billingRequestSpec — demandes de souscription / renouvellement (facturation).
var billingRequestSpec = entitySpec[model.BillingRequest]{
	table: "billing_requests",
	cols:  []string{"id", "account_id", "plan_id", "plan_name", "amount_fcfa", "base_amount_fcfa", "pay_method", "period_label", "router_count", "ref", "gateway_ref", "status", "created_at", "resolved_at", "resolved_by", "note", "paid_via"},
	idOf:  func(x *model.BillingRequest) string { return x.ID },
	scan: func(r *sql.Rows) (model.BillingRequest, error) {
		var x model.BillingRequest
		err := r.Scan(&x.ID, &x.AccountID, &x.PlanID, &x.PlanName, &x.AmountFcfa, &x.BaseAmountFcfa, &x.PayMethod, &x.PeriodLabel,
			&x.RouterCount, &x.Ref, &x.GatewayRef, &x.Status, &x.CreatedAt, &x.ResolvedAt, &x.ResolvedBy, &x.Note, &x.PaidVia)
		return x, err
	},
	args: func(x *model.BillingRequest) []any {
		return []any{x.ID, x.AccountID, x.PlanID, x.PlanName, x.AmountFcfa, x.BaseAmountFcfa, x.PayMethod, x.PeriodLabel,
			x.RouterCount, x.Ref, x.GatewayRef, x.Status, x.CreatedAt, x.ResolvedAt, x.ResolvedBy, x.Note, x.PaidVia}
	},
	hashOf: hashEntity[model.BillingRequest],
}

// wifiSiteSpec — N°28 : sites WiFi jetables (slug unique global).

// wifiSiteSpec — N°28 : sites WiFi jetables (slug unique global).
var wifiSiteSpec = entitySpec[model.WifiSite]{
	table: "wifi_sites",
	cols:  []string{"id", "account_id", "name", "slug", "router_id", "router_name", "profile_id", "profile_name", "free_time_min", "free_data_mb", "marketing_opt_in", "daily_per_phone", "daily_per_mac", "daily_cap", "wifi_ssid", "wifi_password", "active", "created_at"},
	idOf:  func(x *model.WifiSite) string { return x.ID },
	scan: func(r *sql.Rows) (model.WifiSite, error) {
		var x model.WifiSite
		err := r.Scan(&x.ID, &x.AccountID, &x.Name, &x.Slug, &x.RouterID, &x.RouterName,
			&x.ProfileID, &x.ProfileName, &x.FreeTimeMin, &x.FreeDataMb,
			&x.MarketingOptIn, &x.DailyPerPhone, &x.DailyPerMac, &x.DailyCap,
			&x.WifiSSID, &x.WifiPassword, &x.Active, &x.CreatedAt)
		return x, err
	},
	args: func(x *model.WifiSite) []any {
		return []any{x.ID, x.AccountID, x.Name, x.Slug, x.RouterID, x.RouterName,
			x.ProfileID, x.ProfileName, x.FreeTimeMin, x.FreeDataMb,
			x.MarketingOptIn, x.DailyPerPhone, x.DailyPerMac, x.DailyCap,
			x.WifiSSID, x.WifiPassword, x.Active, x.CreatedAt}
	},
	hashOf: hashEntity[model.WifiSite],
}

// wifiGuestSpec — N°28 : registre marketing/anti-abus des visiteurs WiFi.

// wifiGuestSpec — N°28 : registre marketing/anti-abus des visiteurs WiFi.
var wifiGuestSpec = entitySpec[model.WifiGuest]{
	table: "wifi_guests",
	cols:  []string{"id", "account_id", "site_id", "site_name", "phone", "opt_in", "voucher_id", "code", "day", "created_at", "claim_cmd_id", "mac", "ip", "opt_in_at"},
	idOf:  func(x *model.WifiGuest) string { return x.ID },
	scan: func(r *sql.Rows) (model.WifiGuest, error) {
		var x model.WifiGuest
		err := r.Scan(&x.ID, &x.AccountID, &x.SiteID, &x.SiteName, &x.Phone, &x.OptIn,
			&x.VoucherID, &x.Code, &x.Day, &x.CreatedAt, &x.ClaimCmdID, &x.Mac, &x.IP, &x.OptInAt)
		return x, err
	},
	args: func(x *model.WifiGuest) []any {
		return []any{x.ID, x.AccountID, x.SiteID, x.SiteName, x.Phone, x.OptIn,
			x.VoucherID, x.Code, x.Day, x.CreatedAt, x.ClaimCmdID, x.Mac, x.IP, x.OptInAt}
	},
	hashOf: hashEntity[model.WifiGuest],
}

// promoEventSpec — N°56 : journal analytics du portail hospitalité
// (impressions/clics par promo). Append-only avec ID déterministe : le même
// (compte, promo, type, appareil, jour) réécrit la MÊME ligne (upsert no-op),
// la diff syncTable la voit inchangée — re-POSTer un événement identique ne
// coûte ni ligne ni écriture Neon.

// promoEventSpec — N°56 : journal analytics du portail hospitalité
// (impressions/clics par promo). Append-only avec ID déterministe : le même
// (compte, promo, type, appareil, jour) réécrit la MÊME ligne (upsert no-op),
// la diff syncTable la voit inchangée — re-POSTer un événement identique ne
// coûte ni ligne ni écriture Neon.
var promoEventSpec = entitySpec[model.PromoEvent]{
	table: "promo_events",
	cols:  []string{"id", "account_id", "promo_id", "kind", "client_key", "day", "created_at"},
	idOf:  func(x *model.PromoEvent) string { return x.ID },
	scan: func(r *sql.Rows) (model.PromoEvent, error) {
		var x model.PromoEvent
		err := r.Scan(&x.ID, &x.AccountID, &x.PromoID, &x.Kind, &x.ClientKey, &x.Day, &x.CreatedAt)
		return x, err
	},
	args: func(x *model.PromoEvent) []any {
		return []any{x.ID, x.AccountID, x.PromoID, x.Kind, x.ClientKey, x.Day, x.CreatedAt}
	},
	hashOf: hashEntity[model.PromoEvent],
}

// geniusPaySubSpec — abonnements récurrents carte (Stripe via GeniusPay).

// geniusPaySubSpec — abonnements récurrents carte (Stripe via GeniusPay).
var geniusPaySubSpec = entitySpec[model.GeniusPaySub]{
	table: "geniuspay_subs",
	cols:  []string{"uuid", "account_id", "plan_id", "plan_name", "cycle", "amount_fcfa", "slots", "status", "customer_name", "customer_email", "phone", "next_billing", "last_invoice_at", "last_renewal_at", "created_at", "updated_at", "cancelled_at"},
	idOf:  func(x *model.GeniusPaySub) string { return x.UUID },
	scan: func(r *sql.Rows) (model.GeniusPaySub, error) {
		var x model.GeniusPaySub
		err := r.Scan(&x.UUID, &x.AccountID, &x.PlanID, &x.PlanName, &x.Cycle, &x.AmountFcfa, &x.Slots,
			&x.Status, &x.CustomerName, &x.CustomerEmail, &x.Phone, &x.NextBilling, &x.LastInvoiceAt,
			&x.LastRenewalAt, &x.CreatedAt, &x.UpdatedAt, &x.CancelledAt)
		return x, err
	},
	args: func(x *model.GeniusPaySub) []any {
		return []any{x.UUID, x.AccountID, x.PlanID, x.PlanName, x.Cycle, x.AmountFcfa, x.Slots,
			x.Status, x.CustomerName, x.CustomerEmail, x.Phone, x.NextBilling, x.LastInvoiceAt,
			x.LastRenewalAt, x.CreatedAt, x.UpdatedAt, x.CancelledAt}
	},
	hashOf: hashEntity[model.GeniusPaySub],
}

// purgeTombstoneSpec — marqueurs anti-résurgence posés par la purge admin
// (audit purge) : bloquent le ré-import des usernames purgés par la synchro
// agent. Expiration gérée côté applicatif (expires_at, PurgeTombstoneTTL) :
// les lignes expirées sont élaguées de l'état mémoire par purgeExpired
// (handlers_purge.go) — la diff syncTable supprime alors les lignes Neon.

// purgeTombstoneSpec — marqueurs anti-résurgence posés par la purge admin
// (audit purge) : bloquent le ré-import des usernames purgés par la synchro
// agent. Expiration gérée côté applicatif (expires_at, PurgeTombstoneTTL) :
// les lignes expirées sont élaguées de l'état mémoire par purgeExpired
// (handlers_purge.go) — la diff syncTable supprime alors les lignes Neon.
var purgeTombstoneSpec = entitySpec[model.PurgeTombstone]{
	table: "purge_tombstones",
	cols:  []string{"id", "account_id", "username", "purged_at", "expires_at"},
	idOf:  func(x *model.PurgeTombstone) string { return x.ID },
	scan: func(r *sql.Rows) (model.PurgeTombstone, error) {
		var x model.PurgeTombstone
		err := r.Scan(&x.ID, &x.AccountID, &x.Username, &x.PurgedAt, &x.ExpiresAt)
		return x, err
	},
	args: func(x *model.PurgeTombstone) []any {
		return []any{x.ID, x.AccountID, x.Username, x.PurgedAt, x.ExpiresAt}
	},
	hashOf: hashEntity[model.PurgeTombstone],
}

// joinLinkSpec — N°27 : liens d'inscription publique (QR) — token stocké
// côté serveur, révocable, compteur d'usages, expiration.

// joinLinkSpec — N°27 : liens d'inscription publique (QR) — token stocké
// côté serveur, révocable, compteur d'usages, expiration.
var joinLinkSpec = entitySpec[model.JoinLink]{
	table: "join_links",
	cols:  []string{"id", "account_id", "name", "token", "profile_id", "profile_name", "router_id", "router_name", "auto_validate", "max_uses", "uses", "expires_at", "revoked", "created_by", "created_by_name", "created_at"},
	idOf:  func(x *model.JoinLink) string { return x.ID },
	scan: func(r *sql.Rows) (model.JoinLink, error) {
		var x model.JoinLink
		err := r.Scan(&x.ID, &x.AccountID, &x.Name, &x.Token, &x.ProfileID, &x.ProfileName, &x.RouterID, &x.RouterName, &x.AutoValidate, &x.MaxUses, &x.Uses, &x.ExpiresAt, &x.Revoked, &x.CreatedBy, &x.CreatedByName, &x.CreatedAt)
		return x, err
	},
	args: func(x *model.JoinLink) []any {
		return []any{x.ID, x.AccountID, x.Name, x.Token, x.ProfileID, x.ProfileName, x.RouterID, x.RouterName, x.AutoValidate, x.MaxUses, x.Uses, x.ExpiresAt, x.Revoked, x.CreatedBy, x.CreatedByName, x.CreatedAt}
	},
	hashOf: hashEntity[model.JoinLink],
}

// registrationRequestSpec — N°27 : demandes d'inscription publique en
// attente de validation (mot de passe vidé à l'approbation comme au refus).

// registrationRequestSpec — N°27 : demandes d'inscription publique en
// attente de validation (mot de passe vidé à l'approbation comme au refus).
var registrationRequestSpec = entitySpec[model.RegistrationRequest]{
	table: "registration_requests",
	cols:  []string{"id", "account_id", "link_id", "link_name", "full_name", "phone", "desired_username", "password", "message", "status", "rejection_reason", "reviewed_by", "reviewed_by_name", "reviewed_at", "user_id", "created_ip", "created_mac", "created_at"},
	idOf:  func(x *model.RegistrationRequest) string { return x.ID },
	scan: func(r *sql.Rows) (model.RegistrationRequest, error) {
		var x model.RegistrationRequest
		err := r.Scan(&x.ID, &x.AccountID, &x.LinkID, &x.LinkName, &x.FullName, &x.Phone, &x.DesiredUsername, &x.Password, &x.Message, &x.Status, &x.RejectionReason, &x.ReviewedBy, &x.ReviewedByName, &x.ReviewedAt, &x.UserID, &x.CreatedIP, &x.CreatedMac, &x.CreatedAt)
		return x, err
	},
	args: func(x *model.RegistrationRequest) []any {
		return []any{x.ID, x.AccountID, x.LinkID, x.LinkName, x.FullName, x.Phone, x.DesiredUsername, x.Password, x.Message, x.Status, x.RejectionReason, x.ReviewedBy, x.ReviewedByName, x.ReviewedAt, x.UserID, x.CreatedIP, x.CreatedMac, x.CreatedAt}
	},
	hashOf: hashEntity[model.RegistrationRequest],
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

// lineQualitySpec — N°103 — agrégats quotidiens de qualité de ligne (mesure
// passive du débit FAI). id synthétique « lq-… » : une ligne par (routeur,
// jour, interface) ; les histogrammes restent des colonnes TEXT (forme
// canonique « c0,…,c15 » produite et relue par le modèle — pas de JSON, un
// simple séparateur virgule, cf. model/linequality.go).
var lineQualitySpec = entitySpec[model.LineQualityDay]{
	table: "line_quality",
	cols:  []string{"id", "account_id", "router_id", "day", "iface", "samples", "rx_max_bps", "tx_max_bps", "rx_hist", "tx_hist", "updated_at"},
	idOf:  func(x *model.LineQualityDay) string { return x.ID },
	scan: func(r *sql.Rows) (model.LineQualityDay, error) {
		var x model.LineQualityDay
		err := r.Scan(&x.ID, &x.AccountID, &x.RouterID, &x.Day, &x.Iface, &x.Samples,
			&x.RxMaxBps, &x.TxMaxBps, &x.RxHist, &x.TxHist, &x.UpdatedAt)
		return x, err
	},
	args: func(x *model.LineQualityDay) []any {
		return []any{x.ID, x.AccountID, x.RouterID, x.Day, x.Iface, x.Samples,
			x.RxMaxBps, x.TxMaxBps, x.RxHist, x.TxHist, x.UpdatedAt}
	},
	hashOf: hashEntity[model.LineQualityDay],
}

// notifSettingsSpec — réglages de notification par compte. id = account_id.
// stock_alert_state est sérialisé en JSON dans une colonne TEXT (” = nil).

// notifSettingsSpec — réglages de notification par compte. id = account_id.
// stock_alert_state est sérialisé en JSON dans une colonne TEXT (” = nil).
var notifSettingsSpec = entitySpec[model.NotificationSettings]{
	table: "notif_settings",
	cols: []string{"id", "enabled", "telegram_enabled", "telegram_bot_token", "telegram_chat_id",
		"whatsapp_enabled", "whatsapp_token", "whatsapp_phone_id", "whatsapp_to",
		"email_enabled", "email_provider", "resend_api_key", "resend_from",
		"smtp_host", "smtp_port", "smtp_user", "smtp_pass", "email_to",
		"offline_after_sec", "low_stock_threshold", "daily_report", "report_hour",
		"last_report_date", "stock_alert_state", "pool_alert_state", "account_id"},
	idOf: func(x *model.NotificationSettings) string { return x.AccountID },
	scan: func(r *sql.Rows) (model.NotificationSettings, error) {
		var x model.NotificationSettings
		var stockState string
		var poolState string
		err := r.Scan(&x.AccountID, &x.Enabled, &x.TelegramEnabled, &x.TelegramBotToken, &x.TelegramChatID,
			&x.WhatsAppEnabled, &x.WhatsAppToken, &x.WhatsAppPhoneID, &x.WhatsAppTo,
			&x.EmailEnabled, &x.EmailProvider, &x.ResendAPIKey, &x.ResendFrom,
			&x.SMTPHost, &x.SMTPPort, &x.SMTPUser, &x.SMTPPass, &x.EmailTo,
			&x.OfflineAfterSec, &x.LowStockThreshold, &x.DailyReport, &x.ReportHour,
			&x.LastReportDate, &stockState, &poolState, &x.AccountID)
		if err != nil {
			return x, err
		}
		if stockState != "" {
			_ = json.Unmarshal([]byte(stockState), &x.StockAlertState)
		}
		if poolState != "" {
			_ = json.Unmarshal([]byte(poolState), &x.PoolAlertState)
		}
		// N°75 — secrets de notification chiffrés au repos : lecture =
		// déchiffrement (passthrough si valeur antérieure au correctif,
		// migration assurée par migrateSealSecretColumns).
		x.TelegramBotToken = secretbox.Decrypt(x.TelegramBotToken)
		x.WhatsAppToken = secretbox.Decrypt(x.WhatsAppToken)
		x.ResendAPIKey = secretbox.Decrypt(x.ResendAPIKey)
		x.SMTPPass = secretbox.Decrypt(x.SMTPPass)
		return x, nil
	},
	args: func(x *model.NotificationSettings) []any {
		stockState := ""
		if x.StockAlertState != nil {
			if b, err := json.Marshal(x.StockAlertState); err == nil {
				stockState = string(b)
			}
		}
		poolState := ""
		if x.PoolAlertState != nil {
			if b, err := json.Marshal(x.PoolAlertState); err != nil {
				poolState = ""
			} else {
				poolState = string(b)
			}
		}
		// N°75 — écriture = chiffrement des secrets de notification
		// (tokens bots Telegram/WhatsApp, clé Resend, mot de passe
		// SMTP). L'empreinte (hashOf) reste calculée sur l'état mémoire
		// clair — comme les mots de passe routeur, la valeur chiffrée
		// (nonce aléatoire) ne provoque aucune réécriture en boucle.
		return []any{x.AccountID, x.Enabled, x.TelegramEnabled, secretbox.Encrypt(x.TelegramBotToken), x.TelegramChatID,
			x.WhatsAppEnabled, secretbox.Encrypt(x.WhatsAppToken), x.WhatsAppPhoneID, x.WhatsAppTo,
			x.EmailEnabled, x.EmailProvider, secretbox.Encrypt(x.ResendAPIKey), x.ResendFrom,
			x.SMTPHost, x.SMTPPort, x.SMTPUser, secretbox.Encrypt(x.SMTPPass), x.EmailTo,
			x.OfflineAfterSec, x.LowStockThreshold, x.DailyReport, x.ReportHour,
			x.LastReportDate, stockState, poolState, x.AccountID}
	},
	hashOf: hashEntity[model.NotificationSettings],
}

// notifLogSpec — historique des notifications envoyées (par compte).

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
