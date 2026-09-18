// routes.go — table de routage HTTP complète de l'API (contrat : docs/CONTRACT-V2.md).

package api

import (
	"net/http"
	"os"
	"sync"
	"time"

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
	pinLock *pinLimiter    // sécurité S2 — verrouillage PIN revendeur par compte
	signup  *signupLimiter // sécurité S3 — quota d'inscription par IP
	join    *signupLimiter // N°27 — quota anti-abus du formulaire public d'inscription
	// N°50 — quota anti-fermage du claim WiFi jetable (par IP, bornes
	// NAT-friendly : 20/10 min + 100/24 h). Complète les plafonds métier
	// journaliers (téléphone/MAC/site) : un attaquant qui tourne sur des
	// numéros falsifiés est coupé AVANT toute création de voucher.
	wifiClaim *signupLimiter
	// N°56 — quota du track analytics du portail (public) : bornes larges
	// car NAT-friendly (une page = ≤ 12 événements ; un établissement
	// entier partage une IP) — 300/10 min + 3000/24 h par IP.
	portalTrack *signupLimiter
	// N°68 — quota des demandes de réinitialisation de mot de passe (public) :
	// 5/10 min + 20/24 h par IP — les e-mails de lien sont des envois RÉELS
	// (Resend/SMTP), le flood est coupé avant d'atteindre le fournisseur.
	reset *signupLimiter
	// N°127 — quota de l'assistant conversationnel public de la
	// vitrine (session/message/handoff, par IP) : 30/10 min + 400/24 h —
	// un humain en conversation active reste très loin du seuil, un
	// script de spam est coupé.
	chat   *signupLimiter
	vitals *telemetry.Collector // B2 — Core Web Vitals (nil = collecte désactivée)
	// N°72 — bande passante sortante du jour (octets de corps de réponse
	// par catégorie : agents / portail / medias / console / autre), exposée
	// dans GET /api/admin/sync-status. Le quota Render (5 Go/mois gratuit)
	// avait été épuisé en ~9 jours SANS qu'aucune métrique n'existe.
	egress *egressStats
	// N°74 — télémétrie cadencée : horodatage du dernier read_state APPLIQUÉ
	// par routeur (handleAgentResult). La boucle de télémétrie RE-filetait un
	// read_state à CHAQUE check-in (toutes les 45 s, 24 h/24) — ~75 % du
	// trafic agents partait dans des snapshots complets O(n) dont personne
	// ne regardait le résultat. La cadence est désormais bornée par
	// readStateMinInterval ; les re-sync manuelles et post-écriture restent
	// immédiates. Accédé UNIQUEMENT sous le verrou du store.
	readStateDone map[string]time.Time
	// N°76 — read_state PAGINÉ : accumulateur des chunks reçus par routeur
	// (union des usernames + complétude par offset). Un cycle complet =
	// len(starts) chunks couvrant [0, total) ; la réconciliation (badges,
	// import inconnus, diff sessions) ne s'applique QU'AU RAPPORT COMPLET —
	// un chunk perdu abandonne le cycle sans AUCUNE déduction (honnêteté
	// v2/v4 : rien n'est pire qu'un badge mensonger). Accédé UNIQUEMENT sous
	// le verrou du store (applyReadState est toujours appelée sous verrou).
	readAcc map[string]*readStateAccum
	// N°76 — cadence adaptée à la taille du parc : nombre de chunks du dernier
	// cycle complet par routeur. L'intervalle minimum devient
	// readStateMinInterval × max(1, chunks) : un parc de 3 500 users (7 chunks)
	// se réconcilie toutes les ~14 min au lieu de 2 min — le coût egress des
	// scripts servus (7 × ~2 Ko par cycle) reste dans le régime N°75.
	// Accédé UNIQUEMENT sous le verrou du store.
	readStateChunks map[string]int
	// N°101 — cadence de l'inventaire HomeNet : horodatage du dernier
	// rapport read_dhcp APPLIQUÉ par routeur (handleAgentResult) — borne
	// le cycle à devicesMinInterval (2 min), miroir de readStateDone.
	// Accédé UNIQUEMENT sous le verrou du store.
	devicesDone map[string]time.Time
	// N°75 — veille adaptative : signaux d'attention VOLATILS (jamais
	// persistés — un redémarrage repart en mode rapide partout, le plus
	// sûr). Clés « acc:<id> » (requête console authentifiée du compte) et
	// « rt:<id> » (portail/claim sur le routeur). Verrou dédié, jamais
	// imbriqué avec le verrou du store (le marqueur console est posé
	// depuis le middleware d'auth, hors section critique).
	attnMu sync.Mutex
	attn   map[string]time.Time
	// N°150 — pairage Telegram plateforme (bot FTCI, lien magique) :
	// codes éphémères code → compte (15 min, usage unique) servis par
	// POST /api/notifications/telegram/pair-code, consommés par le webhook
	// public POST /api/webhooks/telegram (validé par l'en-tête secret
	// X-Telegram-Bot-Api-Secret-Token). tgMu protège les trois champs ;
	// JAMAIS de section critique tgMu imbriquée avec le verrou du store
	// (les écritures de réglages prennent le store APRÈS avoir consommé le
	// code sous tgMu seul).
	tgMu                sync.Mutex
	tgPairings          map[string]telegramPairing
	tgAccountCode       map[string]string
	telegramBotUsername string
}

// readStateMinInterval — N°74 — intervalle minimum entre deux read_state
// AUTOMATIQUES pour un même routeur (les commandes d'écriture re-enfilent
// toujours un read_state immédiat : fraîcheur post-action préservée).
// 2 minutes = un read_state toutes les ~2,7 check-ins au lieu de chacun :
// -62 % du volume agents (mesuré : 1 920 read_states/jour/routeur → ~720).
// La fraîcheur des vues Sessions/dashboard passe de 45 s à ≤ 2 min — le
// comptage des sessions reste exact (la vue ne fait que s'actualiser un
// peu plus tard), les expirations restent servies à chaque check-in.
const readStateMinInterval = 2 * time.Minute

// readStateAccumStale — N°76 — un cycle paginé inachevé depuis plus de 15 min
// est abandonné silencieusement (l'accumulateur est purgé au prochain chunk 0
// du routeur) : un routeur qui a perdu son cycle relancera de toute façon un
// cycle complet au cadenceur, l'accumulateur ne doit jamais fuiter en mémoire.
const readStateAccumStale = 15 * time.Minute

// New construit l'API.
func New(s *store.Store, jwtSecret string) *API {
	// N°114 — le quota S3 d'inscription lit ses bornes de l'environnement
	// (SIGNUP_BURST_MAX / SIGNUP_DAILY_MAX, repli franc sur les constantes
	// S3 en production) : le runner E2E partage UNE IP et le retry d'un
	// groupe serial rejoue l'inscription déjà passée — cf. signup_abuse.go.
	// Les autres limiteurs S3 (join, reset) gardent les constantes : aucune
	// suite E2E ne les traverse.
	return &API{store: s, secret: jwtSecret, gws: map[string]routeros.Gateway{}, pinLock: newPinLimiter(), signup: signupLimiterFromEnv(os.Getenv), join: newSignupLimiter(), wifiClaim: newSignupLimiterLimits(20, 100), portalTrack: newSignupLimiterLimits(300, 3000), reset: newSignupLimiter(), chat: newSignupLimiterLimits(30, 400), egress: newEgressStats(), readStateDone: map[string]time.Time{}, readAcc: map[string]*readStateAccum{}, readStateChunks: map[string]int{}, devicesDone: map[string]time.Time{}, attn: map[string]time.Time{}, tgPairings: map[string]telegramPairing{}, tgAccountCode: map[string]string{}}
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
	// N°68 — « Mot de passe oublié ? » : demande de lien e-mail (public,
	// quota IP) puis consommation du lien (usage unique, 60 min).
	mux.HandleFunc("POST /api/auth/forgot-password", a.handleForgotPassword)
	mux.HandleFunc("POST /api/auth/reset-password", a.handleResetPassword)
	// N°127 — assistant conversationnel public (remplace la FAQ de la
	// vitrine) : le bot répond aux questions fréquentes, la conversation
	// peut être transmise à un humain (console plateforme). Le secret
	// visiteur fait l'authentification — voir handlers_chat.go.
	mux.HandleFunc("POST /api/chat/session", a.handleChatSession)
	mux.HandleFunc("POST /api/chat/message", a.handleChatMessage)
	mux.HandleFunc("GET /api/chat/messages", a.handleChatMessages)
	mux.HandleFunc("POST /api/chat/handoff", a.handleChatHandoff)

	// N°7 — équipe & rôles (owner uniquement ; le super-admin plateforme est
	// traité owner sur le compte consulté).
	mux.HandleFunc("GET /api/team", a.requireRole(3, a.handleTeamList))
	mux.HandleFunc("POST /api/team", a.requireRole(3, a.handleTeamCreate))
	mux.HandleFunc("PUT /api/team/{id}", a.requireRole(3, a.handleTeamUpdate))
	mux.HandleFunc("DELETE /api/team/{id}", a.requireRole(3, a.handleTeamDelete))

	// N°98 — Phase 1 Hotspot/HomeNet : les familles PRODUIT HOTSPOT (Mode
	// Vente, profils, utilisateurs hotspot, vouchers, inscriptions publiques,
	// revendeurs, ventes/comptabilité, modèles, journaux utilisateurs, WiFi
	// jetable, analytics portail) sont enveloppées de requireUsage(hotspot) :
	// un compte « homenet » reçoit 404 — la fonctionnalité n'existe pas pour
	// lui (cf. usage_guard.go). Les familles PARTAGÉES (dashboard, routeurs et
	// outils, sessions, protection, réglages, activité, équipe, abonnements,
	// notifications, médias) restent ouvertes aux deux usages — c'est le pont
	// de données de la Phase 2 (la Protection en tête). Les routes PUBLIQUES
	// (/api/join/{token}, /api/reseller/login, portail WiFi) ne sont pas
	// gardées : sans JWT, pas de compte à vérifier.
	// N°8 — Mode Vente (PWA revendeur, token scopé role=reseller).
	mux.HandleFunc("POST /api/reseller/login", a.handleResellerLogin)
	mux.HandleFunc("GET /api/sell/me", a.requireUsage(model.AccountUsageHotspot, a.requireReseller(a.handleSellMe)))
	mux.HandleFunc("GET /api/sell/stock", a.requireUsage(model.AccountUsageHotspot, a.requireReseller(a.handleSellStock)))
	mux.HandleFunc("POST /api/sell/{id}/sold", a.requireUsage(model.AccountUsageHotspot, a.requireReseller(a.handleSellSold)))
	// N°20 — retour de stock initié par le revendeur (rendre des tickets au gérant).
	mux.HandleFunc("POST /api/sell/return", a.requireUsage(model.AccountUsageHotspot, a.requireReseller(a.handleSellReturn)))
	// N°21 — transfert de stock entre revendeurs (Mode Vente, fusion UX avec
	// le retour : même sélection de tickets, destination pair au lieu du gérant).
	mux.HandleFunc("GET /api/sell/peers", a.requireUsage(model.AccountUsageHotspot, a.requireReseller(a.handleSellPeers)))
	mux.HandleFunc("POST /api/sell/transfer", a.requireUsage(model.AccountUsageHotspot, a.requireReseller(a.handleSellTransfer)))
	mux.HandleFunc("GET /api/sell/day-report", a.requireUsage(model.AccountUsageHotspot, a.requireReseller(a.handleSellDayReport)))
	// P3-d — export comptable « journal de caisse » (CSV Excel, date passée admise).
	mux.HandleFunc("GET /api/sell/day-report.csv", a.requireUsage(model.AccountUsageHotspot, a.requireReseller(a.handleSellDayReportCSV)))

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
	mux.HandleFunc("GET /api/profiles", a.requireUsage(model.AccountUsageHotspot, a.handleProfilesList))
	mux.HandleFunc("POST /api/profiles", a.requireUsage(model.AccountUsageHotspot, a.requireRole(2, a.handleProfileCreate)))
	mux.HandleFunc("PUT /api/profiles/{id}", a.requireUsage(model.AccountUsageHotspot, a.requireRole(2, a.handleProfileUpdate)))
	mux.HandleFunc("DELETE /api/profiles/{id}", a.requireUsage(model.AccountUsageHotspot, a.requireRole(2, a.handleProfileDelete)))

	// Utilisateurs hotspot
	mux.HandleFunc("GET /api/users", a.requireUsage(model.AccountUsageHotspot, a.handleUsersList))
	mux.HandleFunc("POST /api/users", a.requireUsage(model.AccountUsageHotspot, a.handleUserCreate))
	mux.HandleFunc("PUT /api/users/{id}", a.requireUsage(model.AccountUsageHotspot, a.handleUserUpdate))
	mux.HandleFunc("DELETE /api/users/{id}", a.requireUsage(model.AccountUsageHotspot, a.requireRole(2, a.handleUserDelete)))
	mux.HandleFunc("POST /api/users/{id}/enable", a.requireUsage(model.AccountUsageHotspot, a.handleUserEnable))
	mux.HandleFunc("POST /api/users/{id}/disable", a.requireUsage(model.AccountUsageHotspot, a.handleUserDisable))

	// Vouchers
	mux.HandleFunc("POST /api/vouchers/generate", a.requireUsage(model.AccountUsageHotspot, a.handleVouchersGenerate))
	mux.HandleFunc("GET /api/vouchers", a.requireUsage(model.AccountUsageHotspot, a.handleVouchersList))
	mux.HandleFunc("GET /api/vouchers/stats", a.requireUsage(model.AccountUsageHotspot, a.handleVouchersStats)) // N°74 — compteurs serveur (fin du poll pageSize:500)
	mux.HandleFunc("GET /api/vouchers/batches", a.requireUsage(model.AccountUsageHotspot, a.handleBatchesList))
	mux.HandleFunc("GET /api/vouchers/batches/export", a.requireUsage(model.AccountUsageHotspot, a.requireRole(2, a.handleBatchesExport)))
	mux.HandleFunc("DELETE /api/vouchers/{id}", a.requireUsage(model.AccountUsageHotspot, a.requireRole(2, a.handleUserDelete)))
	mux.HandleFunc("POST /api/vouchers/batch/{batchId}/delete", a.requireUsage(model.AccountUsageHotspot, a.requireRole(2, a.handleVouchersBatchDelete)))
	mux.HandleFunc("POST /api/vouchers/batch/{batchId}/transfer", a.requireUsage(model.AccountUsageHotspot, a.requireRole(2, a.handleVouchersBatchTransfer)))
	// N°22 — impression tracée : seul canal de sortie des codes des tickets
	// revendeur depuis la console (les listes les masquent désormais).
	mux.HandleFunc("POST /api/vouchers/print", a.requireUsage(model.AccountUsageHotspot, a.requireRole(2, a.handleVouchersPrint)))
	// N°23 (W6) — reprise gérant : reprendre au revendeur du stock invendu.
	mux.HandleFunc("POST /api/vouchers/reprise", a.requireUsage(model.AccountUsageHotspot, a.requireRole(2, a.handleVouchersReprise)))

	// N°27 — inscriptions publiques (QR) : /api/join/{token} est PUBLIC
	// (whitelist middleware) — le token du lien fait l'accès. La gestion des
	// liens et la file de validation restent derrière le JWT (manager et plus).
	mux.HandleFunc("GET /api/join/{token}", a.handleJoinInfo)
	mux.HandleFunc("POST /api/join/{token}", a.handleJoinSubmit)
	mux.HandleFunc("GET /api/join-links", a.requireUsage(model.AccountUsageHotspot, a.requireRole(2, a.handleJoinLinksList)))
	mux.HandleFunc("POST /api/join-links", a.requireUsage(model.AccountUsageHotspot, a.requireRole(2, a.handleJoinLinkCreate)))
	mux.HandleFunc("PUT /api/join-links/{id}", a.requireUsage(model.AccountUsageHotspot, a.requireRole(2, a.handleJoinLinkUpdate)))
	mux.HandleFunc("DELETE /api/join-links/{id}", a.requireUsage(model.AccountUsageHotspot, a.requireRole(2, a.handleJoinLinkDelete)))
	mux.HandleFunc("GET /api/registrations", a.requireUsage(model.AccountUsageHotspot, a.requireRole(2, a.handleRegistrationsList)))
	mux.HandleFunc("POST /api/registrations/{id}/approve", a.requireUsage(model.AccountUsageHotspot, a.requireRole(2, a.handleRegistrationApprove)))
	mux.HandleFunc("POST /api/registrations/{id}/reject", a.requireUsage(model.AccountUsageHotspot, a.requireRole(2, a.handleRegistrationReject)))
	mux.HandleFunc("DELETE /api/registrations/{id}", a.requireUsage(model.AccountUsageHotspot, a.requireRole(2, a.handleRegistrationDelete)))

	// Sessions
	mux.HandleFunc("GET /api/sessions", a.handleSessionsList)
	// N°101 — Phase 3 HomeNet : le registre des appareils du foyer (bails
	// DHCP rapportés par l'agent + noms affectés + pause dîner). Famille
	// RÉSERVÉE aux comptes homenet (404 pour un compte hotspot — miroir des
	// vues produit hotspot refusées aux foyers) ; renommage et pause sont
	// des gestes de gérant de la maison (rang 2, miroir des protections).
	mux.HandleFunc("GET /api/devices", a.requireUsage(model.AccountUsageHomeNet, a.handleDevicesList))
	mux.HandleFunc("PUT /api/devices/{id}", a.requireUsage(model.AccountUsageHomeNet, a.requireRole(2, a.handleDeviceRename)))
	mux.HandleFunc("POST /api/devices/{id}/pause", a.requireUsage(model.AccountUsageHomeNet, a.requireRole(2, a.handleDevicePause)))
	mux.HandleFunc("DELETE /api/sessions/{id}", a.handleSessionKick)

	// Revendeurs
	mux.HandleFunc("GET /api/resellers", a.requireUsage(model.AccountUsageHotspot, a.requireRole(2, a.handleResellersList)))
	mux.HandleFunc("POST /api/resellers", a.requireUsage(model.AccountUsageHotspot, a.requireRole(2, a.handleResellerCreate)))
	mux.HandleFunc("PUT /api/resellers/{id}", a.requireUsage(model.AccountUsageHotspot, a.requireRole(2, a.handleResellerUpdate)))
	mux.HandleFunc("DELETE /api/resellers/{id}", a.requireUsage(model.AccountUsageHotspot, a.requireRole(2, a.handleResellerDelete)))
	mux.HandleFunc("POST /api/resellers/{id}/credit", a.requireUsage(model.AccountUsageHotspot, a.requireRole(2, a.handleResellerCredit)))
	mux.HandleFunc("POST /api/resellers/{id}/settle", a.requireUsage(model.AccountUsageHotspot, a.requireRole(2, a.handleResellerSettle)))

	// Divers
	mux.HandleFunc("GET /api/transactions", a.requireUsage(model.AccountUsageHotspot, a.requireRole(2, a.handleTransactionsList)))
	mux.HandleFunc("GET /api/reports", a.requireUsage(model.AccountUsageHotspot, a.requireRole(2, a.handleReports)))
	mux.HandleFunc("GET /api/accounting", a.requireUsage(model.AccountUsageHotspot, a.requireRole(2, a.handleAccounting)))
	mux.HandleFunc("GET /api/accounting/export", a.requireUsage(model.AccountUsageHotspot, a.requireRole(2, a.handleAccountingExport)))
	mux.HandleFunc("GET /api/wave/link", a.requireUsage(model.AccountUsageHotspot, a.requireRole(2, a.handleWaveLink)))
	mux.HandleFunc("GET /api/stats/hourly", a.handleStatsHourly)

	mux.HandleFunc("GET /api/activity", a.requireRole(2, a.handleActivityList))
	mux.HandleFunc("GET /api/settings", a.handleSettingsGet)
	mux.HandleFunc("PUT /api/settings", a.requireRole(3, a.handleSettingsPut))

	// N°53 — Média Cloudflare R2 : dépôt d'images du gérant (bannière du
	// portail N°45, promos hospitalité à venir) + lecture PUBLIQUE des
	// objets (le portail captif charge ses images pré-authentification —
	// même hôte que apiBase, donc déjà couvert par le walled-garden).
	// {key...} : les clés hiérarchiques (media/{compte}/{année}/{hex}.jpg)
	// contiennent des slash. Lecture bornée par le limiter global "api".
	mux.HandleFunc("POST /api/media", a.requireRole(3, a.handleMediaUpload))
	mux.HandleFunc("GET /api/media/{key...}", a.handleMediaGet)

	// Abonnement SaaS — formules FCFA SEGMENTÉES par mode (N°122) :
	// Hotspot Mensuel 2 500 F/mois/routeur, Hotspot Annuel 25 000 F/an
	// routeurs illimités ; HomeNet Mensuel 1 250 F/mois/routeur, HomeNet
	// Annuel 12 000 F/an routeurs illimités. Le catalogue /api/subscription
	// est filtré par l'usage du compte. Catalogue complet et état lisibles
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
	// N°98 — usage du compte (Hotspot ⇄ HomeNet) : le SEUL point de bascule
	// en Phase 1 (l'identité produit d'un compte ne se change pas côté client).
	mux.HandleFunc("PUT /api/admin/accounts/{id}/usage", a.requireRole(3, a.handleAdminAccountUsage))
	mux.HandleFunc("DELETE /api/admin/accounts/{id}", a.requireRole(3, a.handleAdminAccountDelete))
	// Bascule support : ouvrir la console d'un compte client à la demande
	// (token scoping le compte, rôle plateforme conservé, action tracée).
	mux.HandleFunc("POST /api/admin/accounts/{id}/impersonate", a.requireRole(3, a.handleAdminImpersonate))
	// Console plateforme (super-admin MikCloud, multi-comptes).
	mux.HandleFunc("GET /api/admin/overview", a.requireRole(3, a.handleAdminOverview))
	mux.HandleFunc("GET /api/admin/activity", a.requireRole(3, a.handleAdminActivity))
	// N°117 — parc routeurs global : mise à jour RouterOS de FLOTTE. La vue
	// « Parc routeurs » de la console plateforme liste chaque routeur de
	// chaque compte (version installée, version disponible détectée, état)
	// ; « Vérifier tout » enfile un routeros_check par routeur agent
	// (lecture seule), « Mettre à jour » n'installe QUE le retard connu
	// (état available du dernier check — jamais à l'aveugle : un update
	// redémarre le routeur et coupe le hotspot du client). Voir
	// handlers_admin_fleet.go.
	mux.HandleFunc("GET /api/admin/fleet/routers", a.requireRole(3, a.handleAdminFleetRouters))
	mux.HandleFunc("POST /api/admin/fleet/routeros-check", a.requireRole(3, a.handleAdminFleetRouterOSCheck))
	mux.HandleFunc("POST /api/admin/fleet/routeros-update", a.requireRole(3, a.handleAdminFleetRouterOSUpdate))
	// N°127 — inbox du chatbot : conversations de la vitrine (bot,
	// transmises à un humain, clôturées) — le support répond depuis la
	// console plateforme. Voir handlers_chat.go.
	mux.HandleFunc("GET /api/admin/chat/conversations", a.requireRole(3, a.handleAdminChatConversations))
	mux.HandleFunc("GET /api/admin/chat/conversations/{id}", a.requireRole(3, a.handleAdminChatConversation))
	mux.HandleFunc("POST /api/admin/chat/conversations/{id}/reply", a.requireRole(3, a.handleAdminChatReply))
	mux.HandleFunc("POST /api/admin/chat/conversations/{id}/close", a.requireRole(3, a.handleAdminChatClose))
	// N°71 — santé de la persistance (synchro différentielle FNV-1a → Neon) et
	// des agents : diagnostic READ-ONLY pour l'opérateur (dernier sync,
	// volumétrie du delta, erreurs, dérive mémoire/miroir, file de commandes).
	// Voir handlers_sync_status.go.
	mux.HandleFunc("GET /api/admin/sync-status", a.requireRole(3, a.handleSyncStatus))
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
	// N°150 — webhook Telegram (public : appelé par les serveurs de Telegram,
	// authentifié par l'en-tête X-Telegram-Bot-Api-Secret-Token posé au
	// setWebhook — même discipline que Wave/GeniusPay : un secret d'env).
	mux.HandleFunc("POST /api/webhooks/telegram", a.handleTelegramWebhook)

	// P0 (audit Mikhmon) — voir docs/CONTRACT-V2.md (F2 à F5, découpage :
	// handlers_templates.go, handlers_userlogs.go, handlers_users_ops.go ;
	// moteur d'enforcement F1 et filtres sessions live dans helpers.go)
	// Modèles de vouchers (F2)
	mux.HandleFunc("GET /api/templates", a.requireUsage(model.AccountUsageHotspot, a.handleTemplatesList))
	mux.HandleFunc("POST /api/templates", a.requireUsage(model.AccountUsageHotspot, a.requireRole(2, a.handleTemplateCreate)))
	mux.HandleFunc("PUT /api/templates/{id}", a.requireUsage(model.AccountUsageHotspot, a.requireRole(2, a.handleTemplateUpdate)))
	mux.HandleFunc("DELETE /api/templates/{id}", a.requireUsage(model.AccountUsageHotspot, a.requireRole(2, a.handleTemplateDelete)))
	// Journal utilisateurs (F3)
	mux.HandleFunc("GET /api/user-logs", a.requireUsage(model.AccountUsageHotspot, a.requireRole(2, a.handleUserLogsList)))
	mux.HandleFunc("GET /api/user-logs/export", a.requireUsage(model.AccountUsageHotspot, a.requireRole(2, a.handleUserLogsExport)))
	// Actions utilisateurs (F4/F5)
	mux.HandleFunc("POST /api/users/{id}/reset-stats", a.requireUsage(model.AccountUsageHotspot, a.handleUserResetStats))
	mux.HandleFunc("POST /api/users/{id}/extend", a.requireUsage(model.AccountUsageHotspot, a.handleUserExtend))
	// N — resynchronisation utilisateur « absent du routeur » (rapprochement doux).
	mux.HandleFunc("POST /api/users/{id}/resync", a.requireUsage(model.AccountUsageHotspot, a.requireRole(2, a.handleUserResync)))
	mux.HandleFunc("GET /api/users/export", a.requireUsage(model.AccountUsageHotspot, a.requireRole(2, a.handleUsersExport)))
	mux.HandleFunc("POST /api/users/cleanup", a.requireUsage(model.AccountUsageHotspot, a.requireRole(2, a.handleUsersCleanup)))
	mux.HandleFunc("POST /api/users/bulk", a.requireUsage(model.AccountUsageHotspot, a.handleUsersBulk))

	// P1 (audit Mikhmon) — voir docs/CONTRACT-V2.md (F6 à F10, découpage :
	// handlers_routers.go, handlers_ipbindings.go, handlers_commands.go,
	// handlers_router_tools.go, handlers_scheduler.go)
	// Trafic temps réel (F6)
	mux.HandleFunc("GET /api/routers/{id}/traffic", a.handleRouterTraffic)
	// N°103 — Qualité de ligne : mesure passive du débit FAI (enveloppe
	// 14 j de l'interface WAN détectée + capacité déclarée par le gérant).
	// Ouvert à toute l'équipe connectée (lecture), comme /traffic.
	mux.HandleFunc("GET /api/routers/{id}/line-quality", a.handleRouterLineQuality)
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
	// N°115 — mise à jour RouterOS depuis la console (vérification + installation).
	mux.HandleFunc("POST /api/routers/{id}/routeros-check", a.requireRole(2, a.handleRouterOSCheck))
	mux.HandleFunc("POST /api/routers/{id}/routeros-update", a.requireRole(2, a.handleRouterOSUpdate))
	// N°125 — firmware RouterBOARD en attente (appliquage + redémarrage).
	mux.HandleFunc("POST /api/routers/{id}/routerboard-firmware", a.requireRole(2, a.handleRouterboardFirmware))
	// N°97 — docteur pool IP (épuisement heures de pointe)
	mux.HandleFunc("POST /api/routers/{id}/pool-doctor", a.requireRole(2, a.handleRouterPoolDoctor))
	// N°99 — auto-réparation du pool (opt-in par routeur)
	mux.HandleFunc("PUT /api/routers/{id}/pool-auto", a.requireRole(2, a.handleRouterPoolAuto))
	// N°104 — QoS Manager : état + recommandation (lecture, comme /traffic),
	// pose de l'état désiré et désactivation (gestes de gérant, rang 2).
	mux.HandleFunc("GET /api/routers/{id}/qos", a.handleRouterQoSGet)
	mux.HandleFunc("PUT /api/routers/{id}/qos", a.requireRole(2, a.handleRouterQoSPut))
	mux.HandleFunc("DELETE /api/routers/{id}/qos", a.requireRole(2, a.handleRouterQoSDelete))
	// N°106 — ménage à distance : retrait d'une file statique LEGACY
	// (posée à la main avant le QoS Manager) depuis la table des files.
	mux.HandleFunc("DELETE /api/routers/{id}/queues/{name}", a.requireRole(2, a.handleRouterQueueDelete))

	// B2 « Speed App UX » — Core Web Vitals (voir handlers_vitals.go) :
	// collecte publique (beacon text/plain sans preflight, vitrine anonyme
	// incluse) + synthèse plateforme (isPlatformAdmin).
	mux.HandleFunc("POST /api/vitals", a.handleVitalsPost)
	mux.HandleFunc("GET /api/vitals/summary", a.handleVitalsSummary)

	// N°27 — WiFi jetable : page publique (SANS auth, whitelist middleware
	// + rate-limit dédié) résolue par slug GLOBALEMENT unique + console
	// gérant (CRUD sites, registre marketing).
	mux.HandleFunc("GET /api/wifi/site/{slug}", a.handleWifiSiteInfo)
	mux.HandleFunc("POST /api/wifi/site/{slug}/claim", a.handleWifiClaim)
	// N°69 — bascule du consentement marketing d'un numéro (retrait
	// « Ne plus recevoir » de la carte code /wifi, opt-in post-claim).
	// Public, mêmes gardes que le claim (rate-limit + honeypot).
	mux.HandleFunc("POST /api/wifi/site/{slug}/consent", a.handleWifiConsent)
	mux.HandleFunc("GET /api/wifi/site/{slug}/status", a.handleWifiStatus)
	// N°56 — analytics du portail hospitalité : dépôt PUBLIC des
	// événements impressions/clics (la page du portail est pré-auth ;
	// résolution de compte par la clé publique portalKey, dédup + quotas
	// serveur cf. handlers_promo_events.go) et lecture CONSOLE des
	// agrégats (« votre menu vu 480 fois cette semaine »).
	mux.HandleFunc("POST /api/portal/track", a.handlePromoTrack)
	mux.HandleFunc("GET /api/promos/stats", a.requireUsage(model.AccountUsageHotspot, a.requireRole(2, a.handlePromoStats)))

	// N°35-c — config LIVE du portail captif (fetch hybride cloud/local).
	// Public (pas de JWT, pas de token agent) — appelé par login.html au
	// chargement. CORS ouverte à toute origine (cf. corsMiddleware).
	mux.HandleFunc("GET /api/wifi/site/{slug}/portal", a.handleWifiPortal)
	mux.HandleFunc("GET /api/wifi/sites", a.requireUsage(model.AccountUsageHotspot, a.requireRole(2, a.handleWifiSitesList)))
	mux.HandleFunc("POST /api/wifi/sites", a.requireUsage(model.AccountUsageHotspot, a.requireRole(2, a.handleWifiSiteCreate)))
	mux.HandleFunc("PUT /api/wifi/sites/{id}", a.requireUsage(model.AccountUsageHotspot, a.requireRole(2, a.handleWifiSiteUpdate)))
	mux.HandleFunc("DELETE /api/wifi/sites/{id}", a.requireUsage(model.AccountUsageHotspot, a.requireRole(2, a.handleWifiSiteDelete)))
	mux.HandleFunc("GET /api/wifi/guests", a.requireUsage(model.AccountUsageHotspot, a.requireRole(2, a.handleWifiGuests)))

	// Fallback API -> 404 JSON
	mux.HandleFunc("/api/", a.handleAPINotFound)

	// N°72 — chaîne de sortie : compteur egress (httpstats.go) AU-DESSUS
	// du compresseur gzip (gzip.go) lui-même au-dessus de l'auth : les
	// octets comptés sont ceux réellement écrits sur le réseau (après
	// compression), et les 401 du middleware d'authentification sont
	// compressées comme le reste. Les clients sans Accept-Encoding
	// (agents RouterOS /tool fetch) ne sont jamais compressés.
	return a.observeEgress(gzipMiddleware(a.authMiddleware(mux)))
}

// ---------------------------------------------------------------------------
// Middlewares & helpers
// ---------------------------------------------------------------------------

func (a *API) handleHealth(w http.ResponseWriter, r *http.Request) {
	// N°64 — preuve d'audit du balayage périodique de rétention : lecture
	// brève sous verrou (une valeur). Vide = jamais balayé (impossible en
	// pratique : rattrapage au démarrage du service).
	a.store.Lock()
	lastSweep := a.store.Data().LastSweep
	a.store.Unlock()
	sweepISO := ""
	if !lastSweep.IsZero() {
		sweepISO = lastSweep.UTC().Format(time.RFC3339)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"service": "mikcloud-hotspot-api",
		"version": "1.0.0",
		"time":    model.NowISO(),
		// N°64 — dernier passage du balayage (purge journaux 90 j, 1 h).
		"lastSweepAt": sweepISO,
	})
}

func (a *API) handleAPINotFound(w http.ResponseWriter, r *http.Request) {
	writeErr(w, http.StatusNotFound, "Route introuvable")
}
