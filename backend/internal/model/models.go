// Package model — types partagés MikCloud, alignés sur le contrat API (voir worklog.md).
package model

import (
	"crypto/rand"
	"encoding/hex"
	"math/big"
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
	// Mode agent (HTTP-poll sortant) : le token n'est JAMAIS stocké en clair.
	AgentTokenHash string `json:"agentTokenHash,omitempty"`
	TokenPreview   string `json:"tokenPreview,omitempty"`
	LastSeen       string `json:"lastSeen,omitempty"`
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

// Activity — journal d'activité.
type Activity struct {
	ID        string `json:"id"`
	AccountID string `json:"accountId"`
	Type      string `json:"type"` // router | user | voucher | reseller | session | system
	Message   string `json:"message"`
	At        string `json:"at"`
}

// Sale — vente de vouchers (par lot), attribuée au routeur (site) émetteur.
type Sale struct {
	ID           string `json:"id"`
	AccountID    string `json:"accountId"`
	Amount       int    `json:"amount"`
	ProfileName  string `json:"profileName"`
	Count        int    `json:"count"`
	Channel      string `json:"channel"` // direct | reseller
	ResellerName string `json:"resellerName"`
	RouterID     string `json:"routerId"`
	RouterName   string `json:"routerName"`
	BatchID      string `json:"batchId"`
	At           string `json:"at"`
}

// Batch — lot de vouchers générés en une fois (traçabilité complète).
type Batch struct {
	ID           string `json:"id"`
	AccountID    string `json:"accountId"`
	ProfileID    string `json:"profileId"`
	ProfileName  string `json:"profileName"`
	RouterID     string `json:"routerId"`
	RouterName   string `json:"routerName"`
	Count        int    `json:"count"`
	UnitPrice    int    `json:"unitPrice"`
	TotalCost    int    `json:"totalCost"`
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
}

// Plan — plan d'abonnement SaaS.
type Plan struct {
	Name       string `json:"name"`
	MaxRouters string `json:"maxRouters"`
	MaxUsers   string `json:"maxUsers"`
}

// Settings — paramètres du tenant (tenant + plan).
type Settings struct {
	Tenant Tenant `json:"tenant"`
	Plan   Plan   `json:"plan"`
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

// Kinds de commandes agent (routeur -> cloud en HTTP-poll).
const (
	CmdReadState    = "read_state"    // télémétrie + users + sessions actives
	CmdUserAdd      = "user_add"      // créer un utilisateur hotspot
	CmdVoucherBatch = "voucher_batch" // créer un lot de vouchers
	CmdUserRemove   = "user_remove"   // supprimer un/des utilisateurs
	CmdUserSet      = "user_set"      // modifier (nom/profil/password/disabled)
	CmdKick         = "kick"          // fermer une session active
)

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
	Tenant            Tenant              `json:"tenant"`   // legacy mono-tenant
	Settings          Settings            `json:"settings"` // legacy mono-tenant
	LastTick          time.Time           `json:"lastTick"`
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
