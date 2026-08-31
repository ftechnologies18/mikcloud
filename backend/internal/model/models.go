// Package model — types partagés MikCloud, alignés sur le contrat API (voir worklog.md).
package model

import (
	"crypto/rand"
	"encoding/hex"
	"math/big"
	"regexp"
	"strings"
	"time"
)

// CodeCharset — alphabet sans caractères ambigus (pas de 0/1/I/L/O) pour les vouchers.
const CodeCharset = "23456789ABCDEFGHJKMNPQRSTUVWXYZ"

// NewID génère un identifiant court lisible (ex. "u-9f3c1a2b4d5e").
func NewID(prefix string) string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return prefix + hex.EncodeToString([]byte(time.Now().UTC().Format("150405.000000000")))
	}
	return prefix + hex.EncodeToString(b)
}

// RandomCode génère un code alphanumérique de n caractères sans caractères ambigus.
func RandomCode(n int) string {
	var sb strings.Builder
	max := big.NewInt(int64(len(CodeCharset)))
	for i := 0; i < n; i++ {
		idx, err := rand.Int(rand.Reader, max)
		if err != nil {
			idx = big.NewInt(0)
		}
		sb.WriteByte(CodeCharset[idx.Int64()])
	}
	return sb.String()
}

// Presets de charset pour les codes de vouchers — inspirés du User Manager
// MikroTik (« abcd », « ABCD », « aBcD », « 5ab2C34d », « 5AB2C34D », « 5aB2c34D »).
// Tous les alphabets excluent les caractères ambigus (0/1/I/L/O) : les codes
// restent lisibles sur un ticket imprimé ou lus à voix haute par un revendeur.
const (
	CharsetDefault = ""    // chiffres + majuscules sûres (CodeCharset, recommandé)
	CharsetLower   = "abc" // minuscules            — preset « abcd »
	CharsetUpper   = "ABC" // majuscules            — preset « ABCD »
	CharsetLetters = "aBc" // lettres min + maj     — preset « aBcD »
	CharsetDigLow  = "5ab" // chiffres + minuscules — preset « 5ab2c34d »
	CharsetDigUp   = "5AB" // chiffres + majuscules — preset « 5AB2C34D »
	CharsetDigMix  = "5aB" // chiffres + lettres    — preset « 5aB2c34D »
)

const (
	lowerSafe = "abcdefghijkmnpqrstuvwxyz" // sans l, o
	upperSafe = "ABCDEFGHJKMNPQRSTUVWXYZ"  // sans I, L, O
	digitSafe = "23456789"                 // sans 0, 1
)

// CharsetAlphabets associe chaque preset à son alphabet (sans ambiguïtés).
var CharsetAlphabets = map[string]string{
	CharsetDefault: digitSafe + upperSafe,
	CharsetLower:   lowerSafe,
	CharsetUpper:   upperSafe,
	CharsetLetters: lowerSafe + upperSafe,
	CharsetDigLow:  digitSafe + lowerSafe,
	CharsetDigUp:   digitSafe + upperSafe,
	CharsetDigMix:  digitSafe + lowerSafe + upperSafe,
}

// RandomCodeFrom génère un code de n caractères dans l'alphabet du preset
// demandé (charset vide ou inconnu → alphabet MikCloud par défaut).
func RandomCodeFrom(n int, charset string) string {
	alphabet, ok := CharsetAlphabets[charset]
	if !ok || alphabet == "" {
		alphabet = CodeCharset
	}
	var sb strings.Builder
	max := big.NewInt(int64(len(alphabet)))
	for i := 0; i < n; i++ {
		idx, err := rand.Int(rand.Reader, max)
		if err != nil {
			idx = big.NewInt(0)
		}
		sb.WriteByte(alphabet[idx.Int64()])
	}
	return sb.String()
}

// RandomMAC génère une adresse MAC aléatoire "AA:BB:CC:DD:EE:FF".
func RandomMAC() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	parts := make([]string, 6)
	for i, x := range b {
		parts[i] = strings.ToUpper(hex.EncodeToString([]byte{x}))
	}
	return strings.Join(parts, ":")
}

// NowISO retourne l'heure courante au format RFC3339 (UTC).
func NowISO() string { return time.Now().UTC().Format(time.RFC3339) }

// ---------------------------------------------------------------------------
// Types métier (réponses JSON strictement conformes au contrat TS)
// ---------------------------------------------------------------------------

// AccountMainID — identifiant littéral du compte principal (plateforme). Le
// compte principal porte TOUJOURS cet ID (migrations déterministes) : il hérite
// des données de l'ère mono-tenant et ne peut pas être désactivé.
const AccountMainID = "acc-main"

// Account — compte client SaaS (isolation multi-tenant). Chaque entité métier
// porte un AccountID : un compte ne voit et ne modifie que ses données.
type Account struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Status    string `json:"status"` // active | disabled
	CreatedAt string `json:"createdAt"`
	// Contact propriétaire (signup) — support WhatsApp/email + segmentation
	// géographique (devise/timezone automatiques, rapports commerciaux).
	Email   string `json:"email,omitempty"`
	Phone   string `json:"phone,omitempty"`   // WhatsApp de préférence, format E.164 sans +
	Country string `json:"country,omitempty"` // code ISO 3166-1 alpha-2 (CI, SN, NG…) ou "other"
	City    string `json:"city,omitempty"`
}

// Router — équipement MikroTik géré (simulé, réel ou agent). Password non exposé dans l'API.
type Router struct {
	ID             string `json:"id"`
	AccountID      string `json:"accountId"`
	Name           string `json:"name"`
	Host           string `json:"host"`
	Port           int    `json:"port"`
	Username       string `json:"username"`
	Password       string `json:"password,omitempty"`
	Mode           string `json:"mode"`   // simulated | real | agent
	Status         string `json:"status"` // online | offline
	Version        string `json:"version"`
	UptimeSec      int64  `json:"uptimeSec"`
	CPULoad        int    `json:"cpuLoad"`
	HotspotUsers   int    `json:"hotspotUsers"`
	ActiveSessions int    `json:"activeSessions"`
	CreatedAt      string `json:"createdAt"`
	// HotspotLoginUrl — page de login du portail captive MikroTik (ex.
	// http://10.5.50.1/login). Utilisée par les QR codes des vouchers imprimés :
	// le QR encode {url}?username=CODE&password=PASS → connexion en 1 scan.
	// Vide → le QR contient simplement « CODE / PASS ».
	HotspotLoginUrl string `json:"hotspotLoginUrl,omitempty"`
	// Mode agent (HTTP-poll sortant) : le token n'est JAMAIS stocké en clair.
	AgentTokenHash string `json:"agentTokenHash,omitempty"`
	TokenPreview   string `json:"tokenPreview,omitempty"`
	LastSeen       string `json:"lastSeen,omitempty"`
	// P1 (audit Mikhmon) — F8 : status étendu (board, disque). Colonnes
	// créées dès la vague P0 (migrations idempotentes pg.go).
	BoardName  string `json:"boardName,omitempty"`
	FreeHddMb  int    `json:"freeHddMb,omitempty"`
	TotalHddMb int    `json:"totalHddMb,omitempty"`
}

// Profile — profil hotspot (débit, durée, prix, validité).
type Profile struct {
	ID                string `json:"id"`
	AccountID         string `json:"accountId"`
	Name              string `json:"name"`
	RateLimit         string `json:"rateLimit"`
	SessionTimeoutMin int    `json:"sessionTimeoutMin"`
	SharedUsers       int    `json:"sharedUsers"`
	ValidityDays      int    `json:"validityDays"`
	Price             int    `json:"price"`
	DataQuotaMb       int    `json:"dataQuotaMb"`
	CreatedAt         string `json:"createdAt"`
	// P0 (audit Mikhmon) — expiration cloud (F1).
	ExpMode        string `json:"expMode"`        // "notify" (défaut) | "remove"
	GracePeriodMin int    `json:"gracePeriodMin"` // 0 = immédiat (borne 43200)
	LockUser       bool   `json:"lockUser"`       // verrouiller : 1 session à la fois
	// P2 (audit Mikhmon) — marge (F13) : prix de vente affiché (0 = même prix que Price).
	SellingPrice int `json:"sellingPrice"`
}

// HotspotUser — utilisateur hotspot régulier ou voucher.
type HotspotUser struct {
	ID            string `json:"id"`
	AccountID     string `json:"accountId"`
	Kind          string `json:"kind"` // regular | voucher
	Username      string `json:"username"`
	Password      string `json:"password"`
	ProfileID     string `json:"profileId"`
	ProfileName   string `json:"profileName"`
	RouterID      string `json:"routerId"`
	RouterName    string `json:"routerName"`
	Status        string `json:"status"` // active | used | expired | disabled
	BatchID       string `json:"batchId"`
	ResellerID    string `json:"resellerId"`
	ResellerName  string `json:"resellerName"`
	Comment       string `json:"comment"`
	BytesIn       int64  `json:"bytesIn"`
	BytesOut      int64  `json:"bytesOut"`
	UptimeUsedSec int64  `json:"uptimeUsedSec"`
	CreatedAt     string `json:"createdAt"`
	ExpiresAt     string `json:"expiresAt"`
	UsedAt        string `json:"usedAt"`
	Price         int    `json:"price"`
	// P0/P2 (audit Mikhmon) — marge (F13) : prix de vente copié du profil à la
	// génération ({{price}} du voucher = sellingPrice || price).
	SellingPrice int `json:"sellingPrice"`
	// P0 (audit Mikhmon) — F1 : false tant que l'expiration n'a pas été
	// appliquée au routeur (user_remove / user_set disabled — voir enforceExpired).
	Enforced bool `json:"enforced"`
	// DataQuotaMb — quota de données par voucher appliqué sur le routeur
	// (/ip hotspot user add limit-bytes-total=…, exprimé en Mo ; 0 = illimité
	// dans la limite de la validité). Ex. « 5 Go = 500 F » → DataQuotaMb 5120.
	DataQuotaMb int64 `json:"dataQuotaMb"`
	// N°8 — Mode Vente : remise effective du voucher au client par le
	// revendeur (traçabilité anti-vol). Vide = encore en stock.
	SoldAt  string `json:"soldAt,omitempty"`  // RFC3339
	SoldVia string `json:"soldVia,omitempty"` // "sell_mode" (app revendeur)
}

// Session — session hotspot active.
type Session struct {
	ID          string `json:"id"`
	AccountID   string `json:"accountId"`
	UserID      string `json:"userId"`
	Username    string `json:"username"`
	ProfileName string `json:"profileName"`
	RouterID    string `json:"routerId"`
	RouterName  string `json:"routerName"`
	IP          string `json:"ip"`
	MAC         string `json:"mac"`
	StartedAt   string `json:"startedAt"`
	UptimeSec   int64  `json:"uptimeSec"`
	BytesIn     int64  `json:"bytesIn"`
	BytesOut    int64  `json:"bytesOut"`
}

// Reseller — revendeur avec portefeuille.
type Reseller struct {
	ID           string `json:"id"`
	AccountID    string `json:"accountId"`
	Name         string `json:"name"`
	Username     string `json:"username"`
	Phone        string `json:"phone"`
	Credit       int    `json:"credit"`
	VouchersSold int    `json:"vouchersSold"`
	Revenue      int    `json:"revenue"`
	Status       string `json:"status"` // active | disabled
	CreatedAt    string `json:"createdAt"`
	// N°8 — Mode Vente : PIN (4-6 chiffres) pour l'app PWA du revendeur.
	// Bcrypt ; vide = connexion Mode Vente interdite. Persisté dans db.json
	// (le store sérialise via ces mêmes tags) mais TOUJOURS vidé par
	// sanitizeReseller avant toute réponse API.
	PinHash string `json:"pinHash,omitempty"`
}

// Transaction — mouvement de portefeuille revendeur (credit | sale).
type Transaction struct {
	ID           string `json:"id"`
	AccountID    string `json:"accountId"`
	Type         string `json:"type"`
	ResellerID   string `json:"resellerId"`
	ResellerName string `json:"resellerName"`
	Amount       int    `json:"amount"`
	Note         string `json:"note"`
	At           string `json:"at"`
}

// Rôles d'équipe (N°7) — hiérarchie de privilèges croissante. Le rôle
// « admin » historique (= super-admin plateforme) devient RolePlatformAdmin ;
// « admin » reste accepté en lecture pour les tokens/JWT existants.
const (
	RoleManager       = "manager"        // gérant : tout le compte SAUF équipe et réglages/billing
	RoleOwner         = "owner"          // propriétaire du compte : tout, y compris équipe
	RolePlatformAdmin = "platform_admin" // super-admin MikCloud (multi-comptes)
)

// Activity — journal d'activité/audit (N°7 : trace QUI a agi, pas seulement quoi).
type Activity struct {
	ID        string `json:"id"`
	AccountID string `json:"accountId"`
	Type      string `json:"type"` // router | user | voucher | reseller | session | system | team
	Message   string `json:"message"`
	At        string `json:"at"`
	// N°7 — acteur authentifié à l'origine de l'action (vide = moteur interne :
	// simulation, agent routeur, notifications automatiques).
	ActorID   string `json:"actorId,omitempty"`
	ActorName string `json:"actorName,omitempty"`
}

// Sale — vente de vouchers (par lot), attribuée au routeur (site) émetteur.
type Sale struct {
	ID           string `json:"id"`
	AccountID    string `json:"accountId"`
	Amount       int    `json:"amount"` // conserve sa sémantique : price × count
	ProfileName  string `json:"profileName"`
	Count        int    `json:"count"`
	Channel      string `json:"channel"` // direct | reseller
	ResellerName string `json:"resellerName"`
	RouterID     string `json:"routerId"`
	RouterName   string `json:"routerName"`
	BatchID      string `json:"batchId"`
	At           string `json:"at"`
	// P2 (audit Mikhmon) — marge (F13) : Cost = price×count,
	// SellingTotal = (sellingPrice || price)×count.
	Cost         int `json:"cost"`
	SellingTotal int `json:"selling"`
}

// Batch — lot de vouchers générés en une fois (traçabilité complète).
type Batch struct {
	ID          string `json:"id"`
	AccountID   string `json:"accountId"`
	ProfileID   string `json:"profileId"`
	ProfileName string `json:"profileName"`
	RouterID    string `json:"routerId"`
	RouterName  string `json:"routerName"`
	Count       int    `json:"count"`
	UnitPrice   int    `json:"unitPrice"`
	TotalCost   int    `json:"totalCost"`
	// DataQuotaMb — quota de données (Mo) porté par chaque voucher du lot
	// (0 = illimité). Tracé pour l'affichage et la comptabilité.
	DataQuotaMb  int64  `json:"dataQuotaMb"`
	Channel      string `json:"channel"` // direct | reseller
	ResellerID   string `json:"resellerId"`
	ResellerName string `json:"resellerName"`
	CreatedAt    string `json:"createdAt"`
}

// Tenant — infos du tenant.
type Tenant struct {
	Name     string `json:"name"`
	Currency string `json:"currency"`
	Timezone string `json:"timezone"`
	// Wave CI — lien marchand pay.wave.com (ex. https://pay.wave.com/m/M_xxx/c/ci/)
	// composé avec /amount/<montant>/ pour les demandes de paiement.
	WaveLink string `json:"waveLink,omitempty"`
	// P0 (audit Mikhmon) — F2 : personnalisation voucher.
	DNSName string `json:"dnsName,omitempty"` // ex. wifi.mondomaine.ci
	LogoURL string `json:"logoUrl,omitempty"` // data URL image ≤ 300 Ko
	// P0 (audit Mikhmon) — F5 : politique de nettoyage des expirés.
	ExpiryPolicyMode      string `json:"expiryPolicyMode"`      // "keep" (défaut) | "remove"
	ExpiryPolicyAfterDays int    `json:"expiryPolicyAfterDays"` // défaut 30
}

// Plan — plan d'abonnement SaaS (libellé hérité de l'ère pré-facturation ;
// maintenu pour compatibilité d'affichage, l'état réel vit dans Subscription).
type Plan struct {
	Name       string `json:"name"`
	MaxRouters string `json:"maxRouters"`
	MaxUsers   string `json:"maxUsers"`
}

// Subscription — état d'abonnement SaaS d'un compte. PlanID vide = ère bêta
// (aucune formule souscrite). PeriodEnd vide = non expirant.
type Subscription struct {
	PlanID      string `json:"planId"`      // "" (bêta) | essentiel | illimite
	Status      string `json:"status"`      // active | expired
	PeriodStart string `json:"periodStart"` // RFC3339
	PeriodEnd   string `json:"periodEnd"`   // RFC3339 — "" = non expirant
	// LastAmountFcfa — montant de la période en cours : Essentiel =
	// 1 250 F × routeurs enregistrés au moment de la souscription, Illimité = forfait.
	LastAmountFcfa int `json:"lastAmountFcfa"`
	// P2/P3 (console plateforme) — RouterSlots : nombre de routeurs couverts
	// par une période Essentiel (quota réel vérifié côté serveur à la
	// création de routeur ; 0 = non plafonné : bêta, illimité, plateforme).
	// LastPaidAt : date RFC3339 du dernier paiement marqué par la plateforme
	// (vide = période en attente de paiement — indicatif, sans blocage).
	RouterSlots int    `json:"routerSlots,omitempty"`
	LastPaidAt  string `json:"lastPaidAt,omitempty"`
}

// SaasPlan — formule d'abonnement MikCloud (catalogue public de la console).
type SaasPlan struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	PriceFcfa int    `json:"priceFcfa"`
	Period    string `json:"period"`    // mois | an
	PerRouter bool   `json:"perRouter"` // true : prix × routeurs enregistrés
	Unlimited bool   `json:"unlimited"` // routeurs illimités
	Tagline   string `json:"tagline"`
	Badge     string `json:"badge,omitempty"`
}

// SaasPlans — catalogue des formules MikCloud (marché FCFA concurrentiel).
//   - Essentiel : 1 250 F/mois/routeur — acquisition, sans engagement, le gérant
//     paie au fil de sa croissance (remboursé par 4-5 tickets 24 h vendus).
//   - Illimité : 12 000 F/an, routeurs illimités — arme de conquête :
//     1 000 F/mois équivalent, 2 mois offerts vs Essentiel (−20 % à 1 routeur,
//     −92 % à 10 routeurs), verrouille 12 mois et fait consolider tous les sites.
var SaasPlans = []SaasPlan{
	{
		ID: "essentiel", Name: "Essentiel", PriceFcfa: 1250, Period: "mois",
		PerRouter: true, Tagline: "Payez au fil de votre croissance",
		Badge: "Sans engagement",
	},
	{
		ID: "illimite", Name: "Illimité", PriceFcfa: 12000, Period: "an",
		Unlimited: true, Tagline: "Tous vos routeurs, un seul prix",
		Badge: "2 mois offerts · −20 %",
	},
}

// PlanByID — retrouve une formule du catalogue par son identifiant.
func PlanByID(id string) (SaasPlan, bool) {
	for _, p := range SaasPlans {
		if p.ID == id {
			return p, true
		}
	}
	return SaasPlan{}, false
}

// Settings — paramètres du tenant (tenant + plan + abonnement).
type Settings struct {
	Tenant       Tenant       `json:"tenant"`
	Plan         Plan         `json:"plan"`
	Subscription Subscription `json:"subscription"`
}

// AdminUser — compte d'accès à la console (login), rattaché à un compte SaaS.
type AdminUser struct {
	ID           string `json:"id"`
	AccountID    string `json:"accountId"`
	Name         string `json:"name"`
	Username     string `json:"username"`
	Role         string `json:"role"`
	PasswordHash string `json:"passwordHash"`
	Salt         string `json:"salt"`
	CreatedAt    string `json:"createdAt"`
	// PasswordSetByUser — true quand le mot de passe a été modifié par
	// l'utilisateur via POST /api/auth/password : applyAdminOverride ne
	// l'écrase alors PAS (sauf si la variable ADMIN_PASSWORD change).
	PasswordSetByUser bool `json:"passwordSetByUser,omitempty"`
	// EnvPasswordHash — hash du DERNIER mot de passe appliqué par la
	// variable d'environnement ADMIN_PASSWORD. Sert à détecter un
	// changement d'intention de l'opérateur (env modifiée) par rapport à
	// un mot de passe changé par l'utilisateur depuis la console.
	EnvPasswordHash string `json:"envPasswordHash,omitempty"`
}

// NotificationSettings — canaux et règles d'alerte d'un compte SaaS. Les
// secrets (tokens, mot de passe SMTP) sont stockés mais JAMAIS renvoyés par
// l'API (l'API expose uniquement des booléens « …Set ») ; un PUT avec un
// champ secret vide conserve la valeur existante.
type NotificationSettings struct {
	AccountID string `json:"accountId"`
	Enabled   bool   `json:"enabled"` // interrupteur général des alertes automatiques
	// Telegram — bot API (https://core.telegram.org/bots)
	TelegramEnabled  bool   `json:"telegramEnabled"`
	TelegramBotToken string `json:"telegramBotToken,omitempty"`
	TelegramChatID   string `json:"telegramChatId,omitempty"`
	// WhatsApp Cloud API (Meta Graph)
	WhatsAppEnabled bool   `json:"whatsappEnabled"`
	WhatsAppToken   string `json:"whatsappToken,omitempty"`
	WhatsAppPhoneID string `json:"whatsappPhoneId,omitempty"`
	WhatsAppTo      string `json:"whatsappTo,omitempty"`
	// Email — SMTP direct (STARTTLS 587 / TLS implicite 465)
	EmailEnabled bool   `json:"emailEnabled"`
	SMTPHost     string `json:"smtpHost,omitempty"`
	SMTPPort     int    `json:"smtpPort,omitempty"`
	SMTPUser     string `json:"smtpUser,omitempty"`
	SMTPPass     string `json:"smtpPass,omitempty"`
	EmailTo      string `json:"emailTo,omitempty"`
	// Règles d'alerte
	OfflineAfterSec   int  `json:"offlineAfterSec"`   // sans check-in depuis X s → hors ligne (défaut 135 = 3 × 45 s)
	LowStockThreshold int  `json:"lowStockThreshold"` // vouchers actifs restants < X → alerte stock (défaut 25)
	DailyReport       bool `json:"dailyReport"`       // rapport quotidien
	ReportHour        int  `json:"reportHour"`        // heure d'envoi (UTC = Abidjan GMT+0), défaut 20
	// État interne anti-spam : dernier jour de rapport envoyé (YYYY-MM-DD)
	LastReportDate string `json:"lastReportDate,omitempty"`
	// État anti-spam stock : routerID → "low" | "empty" (dernier état notifié)
	StockAlertState map[string]string `json:"stockAlertState,omitempty"`
}

// Normalize applique les défauts et bornes (appelé avant chaque lecture/écriture).
func (s *NotificationSettings) Normalize() {
	switch {
	case s.OfflineAfterSec == 0:
		s.OfflineAfterSec = 135
	case s.OfflineAfterSec < 60:
		s.OfflineAfterSec = 60
	}
	if s.LowStockThreshold == 0 {
		s.LowStockThreshold = 25
	}
	if s.ReportHour < 0 || s.ReportHour > 23 {
		s.ReportHour = 20
	}
}

// NotificationLog — trace d'un envoi de notification (historique console).
type NotificationLog struct {
	ID        string `json:"id"`
	AccountID string `json:"accountId"`
	Channel   string `json:"channel"` // telegram | whatsapp | email | system
	Kind      string `json:"kind"`    // router_offline | router_back | low_stock | daily_report | test | settings
	Title     string `json:"title"`
	Body      string `json:"body,omitempty"`
	Status    string `json:"status"` // sent | error
	Error     string `json:"error,omitempty"`
	At        string `json:"at"`
}

// Kinds de commandes agent (routeur -> cloud en HTTP-poll).
const (
	CmdReadState    = "read_state"    // télémétrie + users + sessions actives
	CmdUserAdd      = "user_add"      // créer un utilisateur hotspot
	CmdVoucherBatch = "voucher_batch" // créer un lot de vouchers
	CmdUserRemove   = "user_remove"   // supprimer un/des utilisateurs
	CmdUserSet      = "user_set"      // modifier (nom/profil/password/disabled)
	CmdKick         = "kick"          // fermer une session active
	CmdUserReset    = "user_reset"    // remettre à zéro les compteurs d'un utilisateur (F4)
)

// P1 (audit Mikhmon) — kinds de commandes agent des vagues F6-F10.
// Le résultat de chaque commande est rapporté via POST /agent/result et stocké
// dans Command.Result (les outils F9/F10 mettent en cache leurs lignes dans la
// clé "data" — relue tant que la commande est done depuis < 120 s).
const (
	CmdPing            = "ping"             // F8 : test de latence (/ping count=4 as-value)
	CmdIpbindingAdd    = "ipbinding_add"    // F7 : /ip hotspot ip-binding add
	CmdIpbindingSet    = "ipbinding_set"    // F7 : /ip hotspot ip-binding set
	CmdIpbindingRemove = "ipbinding_remove" // F7 : /ip hotspot ip-binding remove
	CmdReadDhcp        = "read_dhcp"        // F9 : /ip dhcp-server lease print
	CmdReadHosts       = "read_hosts"       // F9 : /ip hotspot host print
	CmdReadCookies     = "read_cookies"     // F9 : /ip hotspot cookie print
	CmdReadLog         = "read_log"         // F9 : /log print where topics~"hotspot"
	CmdReadScheduler   = "read_scheduler"   // F10 : /system scheduler print
	CmdSchedulerAdd    = "scheduler_add"    // F10 : /system scheduler add
	CmdSchedulerSet    = "scheduler_set"    // F10 : /system scheduler set (disabled)
	CmdSchedulerRemove = "scheduler_remove" // F10 : /system scheduler remove
	CmdReboot          = "reboot"           // F10 : /system reboot
	CmdShutdown        = "shutdown"         // F10 : /system shutdown
	CmdImportHotspot   = "import_hotspot"   // import initial : lecture paginée des profils + utilisateurs existants sur le routeur
)

// ---------------------------------------------------------------------------
// P0/P1 (audit Mikhmon) — nouveaux types (contrat V2 : F2/F3/F6/F7/F10)
// ---------------------------------------------------------------------------

// VoucherTemplate — modèle d'impression de vouchers (F2). Le rendu des
// variables {{…}} se fait côté CLIENT à l'impression ; le corps est stocké
// tel quel (scripts retirés à la sauvegarde — voir SanitizeTemplateHTML).
type VoucherTemplate struct {
	ID        string `json:"id"`
	AccountID string `json:"accountId"`
	Name      string `json:"name"`     // 1-60 caractères
	Format    string `json:"format"`   // "a4" | "58mm" | "80mm"
	BodyHTML  string `json:"bodyHtml"` // ≤ 20 000 caractères, styles inline
	IsDefault bool   `json:"isDefault"`
	CreatedAt string `json:"createdAt"`
}

// UserLog — journal utilisateurs (F3) : login / logout / expire / kick.
type UserLog struct {
	ID         string `json:"id"`
	AccountID  string `json:"accountId"`
	UserID     string `json:"userId"`
	Username   string `json:"username"`
	Action     string `json:"action"` // "login" | "logout" | "expire" | "kick"
	RouterID   string `json:"routerId"`
	RouterName string `json:"routerName"`
	IP         string `json:"ip"`
	MAC        string `json:"mac"`
	At         string `json:"at"`
}

// IfaceTraffic — compteur cumulé et débit instantané d'une interface (F6).
type IfaceTraffic struct {
	Name    string `json:"name"`
	RxBytes int64  `json:"rxBytes"` // compteurs cumulés
	TxBytes int64  `json:"txBytes"`
	RxBps   int64  `json:"rxBps"` // débit calculé
	TxBps   int64  `json:"txBps"`
}

// TrafficPoint — point d'historique de trafic, somme toutes interfaces (F6).
type TrafficPoint struct {
	T     string `json:"t"` // RFC3339
	RxBps int64  `json:"rxBps"`
	TxBps int64  `json:"txBps"`
}

// RouterTraffic — trafic temps réel d'un routeur (F6). Une entrée par
// routeur : ID = RouterID (pattern de persistance : clé primaire "id").
type RouterTraffic struct {
	ID         string         `json:"id"` // = RouterID
	RouterID   string         `json:"routerId"`
	AccountID  string         `json:"accountId"`
	UpdatedAt  string         `json:"updatedAt"`
	Interfaces []IfaceTraffic `json:"interfaces"` // détail courant par interface
	History    []TrafficPoint `json:"history"`    // 60 derniers points (somme interfaces)
}

// IPBinding — règle hotspot IP binding (F7) : bypass ou blocage par MAC.
type IPBinding struct {
	ID        string `json:"id"`
	AccountID string `json:"accountId"`
	RouterID  string `json:"routerId"`
	MAC       string `json:"mac"`     // "AA:BB:CC:DD:EE:FF"
	Address   string `json:"address"` // IP optionnelle
	Comment   string `json:"comment"`
	Type      string `json:"type"` // "bypassed" | "blocked"
	Disabled  bool   `json:"disabled"`
	CreatedAt string `json:"createdAt"`
}

// SchedulerTask — tâche planifiée du routeur (F10), source cloud.
type SchedulerTask struct {
	ID        string `json:"id"`
	AccountID string `json:"accountId"`
	RouterID  string `json:"routerId"`
	Name      string `json:"name"`
	Interval  string `json:"interval"` // affichage RouterOS ex. "45s", "1d"
	OnEvent   string `json:"onEvent"`
	Disabled  bool   `json:"disabled"`
	CreatedAt string `json:"createdAt"`
}

// scriptTagPattern — blocs <script>…</script> (insensible à la casse,
// multi-lignes) retirés des corps de templates à la sauvegarde.
var scriptTagPattern = regexp.MustCompile(`(?is)<script\b[^>]*>.*?(</script\s*>|$)`)

// SanitizeTemplateHTML retire les blocs <script>…</script> (y compris un
// bloc non fermé, jusqu'à la fin) du corps d'un modèle de voucher : le rendu
// se fait côté client à l'impression, aucune exécution de script n'est attendue.
func SanitizeTemplateHTML(s string) string {
	return strings.TrimSpace(scriptTagPattern.ReplaceAllString(s, ""))
}

// Command — ordre déposé par le cloud, récupéré puis exécuté par l'agent.
type Command struct {
	ID        string         `json:"id"`
	RouterID  string         `json:"routerId"`
	AccountID string         `json:"accountId"`
	Kind      string         `json:"kind"`
	Payload   map[string]any `json:"payload,omitempty"`
	Status    string         `json:"status"` // queued | sent | done | error
	Result    map[string]any `json:"result,omitempty"`
	CreatedAt string         `json:"createdAt"`
	SentAt    string         `json:"sentAt,omitempty"`
	DoneAt    string         `json:"doneAt,omitempty"`
}

// DB — base de données persistée en JSON.
//   - Accounts/SettingsByAccount : modèle multi-tenant (source de vérité) ;
//   - Tenant/Settings : champs LEGACY mono-tenant, uniquement lus pour migrer
//     un ancien db.json — vidés après migration puis ignorés.
type DB struct {
	Accounts          []Account           `json:"accounts"`
	SettingsByAccount map[string]Settings `json:"settingsByAccount"`
	Users             []AdminUser         `json:"users"`
	Routers           []Router            `json:"routers"`
	Profiles          []Profile           `json:"profiles"`
	HotspotUsers      []HotspotUser       `json:"hotspotUsers"`
	Batches           []Batch             `json:"batches"`
	Resellers         []Reseller          `json:"resellers"`
	Transactions      []Transaction       `json:"transactions"`
	Sessions          []Session           `json:"sessions"`
	Activity          []Activity          `json:"activity"`
	Sales             []Sale              `json:"sales"`
	Commands          []Command           `json:"commands"`
	// P0/P1 (audit Mikhmon) — nouvelles collections.
	Templates      []VoucherTemplate `json:"templates"`      // F2
	UserLogs       []UserLog         `json:"userLogs"`       // F3
	IPBindings     []IPBinding       `json:"ipBindings"`     // F7
	SchedulerTasks []SchedulerTask   `json:"schedulerTasks"` // F10
	Traffic        []RouterTraffic   `json:"traffic"`        // F6
	// Tier 1 — notifications multi-canaux.
	NotifSettings map[string]NotificationSettings `json:"notifSettings"` // accountId → réglages
	NotifLog      []NotificationLog               `json:"notifLog"`
	Tenant        Tenant                          `json:"tenant"`   // legacy mono-tenant
	Settings      Settings                        `json:"settings"` // legacy mono-tenant
	LastTick      time.Time                       `json:"lastTick"`
}

// EffectiveStatus retourne le statut réel d'un utilisateur : un voucher encore
// "active" mais dont la date d'expiration est dépassée est vu comme "expired".
func EffectiveStatus(u *HotspotUser, now time.Time) string {
	if u.Kind == "voucher" && u.Status == "active" && u.ExpiresAt != "" {
		if exp, err := time.Parse(time.RFC3339, u.ExpiresAt); err == nil && now.After(exp) {
			return "expired"
		}
	}
	return u.Status
}
