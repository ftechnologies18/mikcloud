// routes.go — table de routage HTTP complète de l'API (contrat : docs/CONTRACT-V2.md).

package api

import (
	"net/http"
	"sync"

	"mikcloud/hotspot-api/internal/model"
	"mikcloud/hotspot-api/internal/routeros"
	"mikcloud/hotspot-api/internal/store"
	"mikcloud/hotspot-api/internal/telemetry"
)

// API — registre des routes + dépendances.
type API struct {
	store   *store.Store
	secret  string
	gwMu    sync.Mutex
	gws     map[string]routeros.Gateway
	pinLock *pinLimiter          // sécurité S2 — verrouillage PIN revendeur par compte
	signup  *signupLimiter       // sécurité S3 — quota d'inscription par IP
	vitals  *telemetry.Collector // B2 — Core Web Vitals (nil = collecte désactivée)
}

// New construit l'API.
func New(s *store.Store, jwtSecret string) *API {
	return &API{store: s, secret: jwtSecret, gws: map[string]routeros.Gateway{}, pinLock: newPinLimiter(), signup: newSignupLimiter()}
}

// Handler — mux complet, protégé par le middleware d'authentification.
func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /{$}", a.handleHealth)

	// Agent MikCloud (routeur -> cloud, HTTP-poll sortant) + provisionning console
	a.registerAgentRoutes(mux)

	// Notifications (réglages canaux, test, historique)
	a.registerNotifRoutes(mux)

	// Auth
	mux.HandleFunc("POST /api/auth/login", a.handleLogin)
	mux.HandleFunc("POST /api/auth/register", a.handleRegister)
	mux.HandleFunc("GET /api/auth/me", a.handleMe)
	mux.HandleFunc("POST /api/auth/password", a.handlePasswordChange)
	// Sécurité S4 — 2FA TOTP (pairage, activation, désactivation).
	mux.HandleFunc("POST /api/auth/2fa/setup", a.handleTOTPSetup)
	mux.HandleFunc("POST /api/auth/2fa/activate", a.handleTOTPActivate)
	mux.HandleFunc("POST /api/auth/2fa/disable", a.handleTOTPDisable)

	// N°7 — équipe & rôles (owner uniquement ; le super-admin plateforme est
	// traité owner sur le compte consulté).
	mux.HandleFunc("GET /api/team", a.requireRole(3, a.handleTeamList))
	mux.HandleFunc("POST /api/team", a.requireRole(3, a.handleTeamCreate))
	mux.HandleFunc("PUT /api/team/{id}", a.requireRole(3, a.handleTeamUpdate))
	mux.HandleFunc("DELETE /api/team/{id}", a.requireRole(3, a.handleTeamDelete))

	// N°8 — Mode Vente (PWA revendeur, token scopé role=reseller).
	mux.HandleFunc("POST /api/reseller/login", a.handleResellerLogin)
	mux.HandleFunc("GET /api/sell/me", a.requireReseller(a.handleSellMe))
	mux.HandleFunc("GET /api/sell/stock", a.requireReseller(a.handleSellStock))
	mux.HandleFunc("POST /api/sell/{id}/sold", a.requireReseller(a.handleSellSold))
	// N°20 — retour de stock initié par le revendeur (rendre des tickets au gérant).
	mux.HandleFunc("POST /api/sell/return", a.requireReseller(a.handleSellReturn))
	// N°21 — transfert de stock entre revendeurs (Mode Vente, fusion UX avec
	// le retour : même sélection de tickets, destination pair au lieu du gérant).
	mux.HandleFunc("GET /api/sell/peers", a.requireReseller(a.handleSellPeers))
	mux.HandleFunc("POST /api/sell/transfer", a.requireReseller(a.handleSellTransfer))
	mux.HandleFunc("GET /api/sell/day-report", a.requireReseller(a.handleSellDayReport))
	// P3-d — export comptable « journal de caisse » (CSV Excel, date passée admise).
	mux.HandleFunc("GET /api/sell/day-report.csv", a.requireReseller(a.handleSellDayReportCSV))

	// Dashboard
	mux.HandleFunc("GET /api/dashboard", a.handleDashboard)

	// Routeurs
	mux.HandleFunc("GET /api/routers", a.handleRoutersList)
	mux.HandleFunc("POST /api/routers", a.requireRole(2, a.handleRouterCreate))
	mux.HandleFunc("PUT /api/routers/{id}", a.requireRole(2, a.handleRouterUpdate))
	mux.HandleFunc("DELETE /api/routers/{id}", a.requireRole(2, a.handleRouterDelete))
	mux.HandleFunc("POST /api/routers/{id}/test", a.requireRole(2, a.handleRouterTest))
	mux.HandleFunc("GET /api/routers/{id}/stats", a.handleRouterStats)

	// Profils
	mux.HandleFunc("GET /api/profiles", a.handleProfilesList)
	mux.HandleFunc("POST /api/profiles", a.requireRole(2, a.handleProfileCreate))
	mux.HandleFunc("PUT /api/profiles/{id}", a.requireRole(2, a.handleProfileUpdate))
	mux.HandleFunc("DELETE /api/profiles/{id}", a.requireRole(2, a.handleProfileDelete))

	// Utilisateurs hotspot
	mux.HandleFunc("GET /api/users", a.handleUsersList)
	mux.HandleFunc("POST /api/users", a.handleUserCreate)
	mux.HandleFunc("PUT /api/users/{id}", a.handleUserUpdate)
	mux.HandleFunc("DELETE /api/users/{id}", a.requireRole(2, a.handleUserDelete))
	mux.HandleFunc("POST /api/users/{id}/enable", a.handleUserEnable)
	mux.HandleFunc("POST /api/users/{id}/disable", a.handleUserDisable)

	// Vouchers
	mux.HandleFunc("POST /api/vouchers/generate", a.handleVouchersGenerate)
	mux.HandleFunc("GET /api/vouchers", a.handleVouchersList)
	mux.HandleFunc("GET /api/vouchers/batches", a.handleBatchesList)
	mux.HandleFunc("GET /api/vouchers/batches/export", a.requireRole(2, a.handleBatchesExport))
	mux.HandleFunc("DELETE /api/vouchers/{id}", a.requireRole(2, a.handleUserDelete))
	mux.HandleFunc("POST /api/vouchers/batch/{batchId}/delete", a.requireRole(2, a.handleVouchersBatchDelete))
	mux.HandleFunc("POST /api/vouchers/batch/{batchId}/transfer", a.requireRole(2, a.handleVouchersBatchTransfer))
	// N°22 — impression tracée : seul canal de sortie des codes des tickets
	// revendeur depuis la console (les listes les masquent désormais).
	mux.HandleFunc("POST /api/vouchers/print", a.requireRole(2, a.handleVouchersPrint))

	// Sessions
	mux.HandleFunc("GET /api/sessions", a.handleSessionsList)
	mux.HandleFunc("DELETE /api/sessions/{id}", a.handleSessionKick)

	// Revendeurs
	mux.HandleFunc("GET /api/resellers", a.requireRole(2, a.handleResellersList))
	mux.HandleFunc("POST /api/resellers", a.requireRole(2, a.handleResellerCreate))
	mux.HandleFunc("PUT /api/resellers/{id}", a.requireRole(2, a.handleResellerUpdate))
	mux.HandleFunc("DELETE /api/resellers/{id}", a.requireRole(2, a.handleResellerDelete))
	mux.HandleFunc("POST /api/resellers/{id}/credit", a.requireRole(2, a.handleResellerCredit))
	mux.HandleFunc("POST /api/resellers/{id}/settle", a.requireRole(2, a.handleResellerSettle))

	// Divers
	mux.HandleFunc("GET /api/transactions", a.requireRole(2, a.handleTransactionsList))
	mux.HandleFunc("GET /api/reports", a.requireRole(2, a.handleReports))
	mux.HandleFunc("GET /api/accounting", a.requireRole(2, a.handleAccounting))
	mux.HandleFunc("GET /api/accounting/export", a.requireRole(2, a.handleAccountingExport))
	mux.HandleFunc("GET /api/wave/link", a.requireRole(2, a.handleWaveLink))
	mux.HandleFunc("GET /api/stats/hourly", a.handleStatsHourly)

	mux.HandleFunc("GET /api/activity", a.requireRole(2, a.handleActivityList))
	mux.HandleFunc("GET /api/settings", a.handleSettingsGet)
	mux.HandleFunc("PUT /api/settings", a.requireRole(3, a.handleSettingsPut))

	// Abonnement SaaS — formules FCFA (Essentiel 1 250 F/mois/routeur,
	// Illimité 12 000 F/an routeurs illimités). Catalogue et état lisibles
	// par toute l'équipe. VERROU FACTURATION : l'activation d'un abonnement
	// est réservée à la plateforme (PUT /api/admin/accounts/{id}/subscription,
	// après encaissement) ; le client ne peut que DEMANDER un renouvellement
	// et obtenir le lien de paiement Wave de la plateforme (POST ci-dessous).
	mux.HandleFunc("GET /api/plans", a.handlePlansList)
	mux.HandleFunc("GET /api/subscription", a.handleSubscriptionGet)
	mux.HandleFunc("POST /api/subscription", a.requireRole(3, a.handleSubscriptionPost))
	// Paiement EN LIGNE de la demande d'abonnement (GeniusPay -> Wave) :
	// initiation client + vérification de statut (filet de sécurité).
	mux.HandleFunc("POST /api/subscription/pay", a.requireRole(3, a.handleSubscriptionPay))
	mux.HandleFunc("GET /api/subscription/pay/status", a.handleSubscriptionPayStatus)
	// Abonnement récurrent par carte (Stripe via GeniusPay) — initiation
	// (redirection Stripe Checkout), statut (filet de sécurité au retour) et
	// résiliation. Autorisés aux comptes expirés ET suspendus (liste blanche
	// du middleware) : c'est une voie de réactivation.
	mux.HandleFunc("GET /api/subscription/stripe", a.requireRole(3, a.handleSubscriptionStripeGet))
	mux.HandleFunc("POST /api/subscription/stripe", a.requireRole(3, a.handleSubscriptionStripePost))
	mux.HandleFunc("POST /api/subscription/stripe/cancel", a.requireRole(3, a.handleSubscriptionStripeCancel))
	// M — facturation client : historique des factures + facture imprimable
	// (HTML print-friendly). Accessibles aux comptes expirés ET suspendus.
	mux.HandleFunc("GET /api/billing/history", a.handleBillingHistory)
	mux.HandleFunc("GET /api/billing/invoice/{id}", a.handleBillingInvoice)
	mux.HandleFunc("POST /api/admin/wipe", a.requireRole(3, a.handleWipe))
	mux.HandleFunc("POST /api/admin/reload", a.requireRole(3, a.handleReload))
	// Purge des données par catégories (UI Paramètres plateforme) — voir
	// handlers_purge.go. FUSION : portée GLOBALE (accountId absent/vide —
	// tous les comptes) ou CIBLÉE (accountId renseigné — ce compte seul).
	// L'ancien POST /api/admin/purge/account est fusionné dans cet endpoint.
	// NB : POST /api/admin/reset (régénération du seed démo) a été SUPPRIMÉ —
	// c'était la cause du retour des données de test et de la disparition des
	// routeurs réels. Aucun endpoint ne régénère de données démo.
	mux.HandleFunc("GET /api/admin/purge/stats", a.requireRole(3, a.handlePurgeStats))
	mux.HandleFunc("POST /api/admin/purge", a.requireRole(3, a.handlePurge))
	// Nettoyage CHIRURGICALE des données de démonstration héritées de
	// l'ancien seed (routeurs simulés + cascade, revendeurs res-1…res-5 +
	// leurs transactions) — préserve les données réelles du compte.
	mux.HandleFunc("POST /api/admin/purge-demo", a.requireRole(3, a.handlePurgeDemo))
	// Compteurs par élément de CHAQUE compte (alimente le sélecteur de
	// portée de la purge fusionnée ; l'exécution passe par POST /purge).
	mux.HandleFunc("GET /api/admin/purge/accounts", a.requireRole(3, a.handlePurgeAccountsStats))

	// Administration plateforme (rôle admin uniquement)
	mux.HandleFunc("GET /api/admin/accounts", a.requireRole(3, a.handleAdminAccounts))
	mux.HandleFunc("POST /api/admin/accounts", a.requireRole(3, a.handleAdminAccountCreate))
	mux.HandleFunc("POST /api/admin/accounts/{id}/status", a.requireRole(3, a.handleAdminAccountStatus))
	// P2 — fiche client, attribution/renouvellement d'abonnement, suppression.
	mux.HandleFunc("GET /api/admin/accounts/{id}", a.requireRole(3, a.handleAdminAccountDetail))
	mux.HandleFunc("PUT /api/admin/accounts/{id}/subscription", a.requireRole(3, a.handleAdminAccountSubscription))
	mux.HandleFunc("DELETE /api/admin/accounts/{id}", a.requireRole(3, a.handleAdminAccountDelete))
	// Bascule support : ouvrir la console d'un compte client à la demande
	// (token scoping le compte, rôle plateforme conservé, action tracée).
	mux.HandleFunc("POST /api/admin/accounts/{id}/impersonate", a.requireRole(3, a.handleAdminImpersonate))
	// Console plateforme (super-admin MikCloud, multi-comptes).
	mux.HandleFunc("GET /api/admin/overview", a.requireRole(3, a.handleAdminOverview))
	mux.HandleFunc("GET /api/admin/activity", a.requireRole(3, a.handleAdminActivity))
	// I (paramètres plateforme) — config globale du SaaS (nom, inscriptions).
	mux.HandleFunc("GET /api/admin/platform/settings", a.requireRole(3, a.handlePlatformSettingsGet))
	mux.HandleFunc("PUT /api/admin/platform/settings", a.requireRole(3, a.handlePlatformSettingsPut))
	mux.HandleFunc("GET /api/admin/team", a.requireRole(3, a.handlePlatformTeamList))
	mux.HandleFunc("POST /api/admin/team", a.requireRole(3, a.handlePlatformTeamCreate))
	mux.HandleFunc("DELETE /api/admin/team/{id}", a.requireRole(3, a.handlePlatformTeamDelete))

	// Facturation (verrou du cycle) — file des demandes de renouvellement +
	// webhook d'encaissement Wave (public, authentifié par secret partagé).
	mux.HandleFunc("GET /api/admin/billing-requests", a.requireRole(3, a.handleAdminBillingRequests))
	mux.HandleFunc("POST /api/admin/billing-requests/{id}/resolve", a.requireRole(3, a.handleAdminBillingRequestResolve))
	mux.HandleFunc("POST /api/webhooks/wave", a.handleWaveWebhook)
	// webhook d'encaissement GeniusPay (public, authentifié par signature HMAC).
	mux.HandleFunc("POST /api/webhooks/geniuspay", a.handleGeniusPayWebhook)

	// P0 (audit Mikhmon) — voir docs/CONTRACT-V2.md (F2 à F5, découpage :
	// handlers_templates.go, handlers_userlogs.go, handlers_users_ops.go ;
	// moteur d'enforcement F1 et filtres sessions live dans helpers.go)
	// Modèles de vouchers (F2)
	mux.HandleFunc("GET /api/templates", a.handleTemplatesList)
	mux.HandleFunc("POST /api/templates", a.requireRole(2, a.handleTemplateCreate))
	mux.HandleFunc("PUT /api/templates/{id}", a.requireRole(2, a.handleTemplateUpdate))
	mux.HandleFunc("DELETE /api/templates/{id}", a.requireRole(2, a.handleTemplateDelete))
	// Journal utilisateurs (F3)
	mux.HandleFunc("GET /api/user-logs", a.requireRole(2, a.handleUserLogsList))
	mux.HandleFunc("GET /api/user-logs/export", a.requireRole(2, a.handleUserLogsExport))
	// Actions utilisateurs (F4/F5)
	mux.HandleFunc("POST /api/users/{id}/reset-stats", a.handleUserResetStats)
	mux.HandleFunc("POST /api/users/{id}/extend", a.handleUserExtend)
	// N — resynchronisation utilisateur « absent du routeur » (rapprochement doux).
	mux.HandleFunc("POST /api/users/{id}/resync", a.requireRole(2, a.handleUserResync))
	mux.HandleFunc("GET /api/users/export", a.requireRole(2, a.handleUsersExport))
	mux.HandleFunc("POST /api/users/cleanup", a.requireRole(2, a.handleUsersCleanup))
	mux.HandleFunc("POST /api/users/bulk", a.handleUsersBulk)

	// P1 (audit Mikhmon) — voir docs/CONTRACT-V2.md (F6 à F10, découpage :
	// handlers_routers.go, handlers_ipbindings.go, handlers_commands.go,
	// handlers_router_tools.go, handlers_scheduler.go)
	// Trafic temps réel (F6)
	mux.HandleFunc("GET /api/routers/{id}/traffic", a.handleRouterTraffic)
	// IP bindings (F7)
	mux.HandleFunc("GET /api/routers/{id}/ipbindings", a.requireRole(2, a.handleIPBindingsList))
	mux.HandleFunc("POST /api/routers/{id}/ipbindings", a.requireRole(2, a.handleIPBindingCreate))
	mux.HandleFunc("PUT /api/ipbindings/{id}", a.requireRole(2, a.handleIPBindingUpdate))
	mux.HandleFunc("DELETE /api/ipbindings/{id}", a.requireRole(2, a.handleIPBindingDelete))
	// Ping + statut de commande (F8)
	mux.HandleFunc("POST /api/routers/{id}/ping", a.requireRole(2, a.handleRouterPing))
	mux.HandleFunc("GET /api/commands/{id}", a.handleCommandStatus)
	// Outils routeur (F9)
	mux.HandleFunc("GET /api/routers/{id}/dhcp", a.requireRole(2, a.handleRouterDhcp))
	mux.HandleFunc("GET /api/routers/{id}/hosts", a.requireRole(2, a.handleRouterHosts))
	mux.HandleFunc("GET /api/routers/{id}/cookies", a.requireRole(2, a.handleRouterCookies))
	mux.HandleFunc("GET /api/routers/{id}/log", a.requireRole(2, a.handleRouterLog))
	// Parité Mikhmon : ressources routeur (pools / files / serveurs) pour les formulaires
	mux.HandleFunc("GET /api/routers/{id}/resources", a.requireRole(2, a.handleRouterResources))
	// Scheduler + alimentation (F10)
	mux.HandleFunc("GET /api/routers/{id}/scheduler", a.requireRole(2, a.handleSchedulerGet))
	mux.HandleFunc("POST /api/routers/{id}/scheduler", a.requireRole(2, a.handleSchedulerCreate))
	mux.HandleFunc("POST /api/routers/{id}/scheduler-toggle", a.requireRole(2, a.handleSchedulerToggle))
	mux.HandleFunc("POST /api/routers/{id}/scheduler-remove", a.requireRole(2, a.handleSchedulerRemove))
	mux.HandleFunc("POST /api/routers/{id}/reboot", a.requireRole(2, a.handleRouterReboot))
	mux.HandleFunc("POST /api/routers/{id}/shutdown", a.requireRole(2, a.handleRouterShutdown))

	// B2 « Speed App UX » — Core Web Vitals (voir handlers_vitals.go) :
	// collecte publique (beacon text/plain sans preflight, vitrine anonyme
	// incluse) + synthèse plateforme (isPlatformAdmin).
	mux.HandleFunc("POST /api/vitals", a.handleVitalsPost)
	mux.HandleFunc("GET /api/vitals/summary", a.handleVitalsSummary)

	// Fallback API -> 404 JSON
	mux.HandleFunc("/api/", a.handleAPINotFound)

	return a.authMiddleware(mux)
}

// ---------------------------------------------------------------------------
// Middlewares & helpers
// ---------------------------------------------------------------------------

func (a *API) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"service": "mikcloud-hotspot-api",
		"version": "1.0.0",
		"time":    model.NowISO(),
	})
}

func (a *API) handleAPINotFound(w http.ResponseWriter, r *http.Request) {
	writeErr(w, http.StatusNotFound, "Route introuvable")
}
