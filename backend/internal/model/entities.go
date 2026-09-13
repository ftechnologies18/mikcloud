// Entités transverses : modèles de vouchers, logs, trafic, IP bindings, scheduler, commandes, tombstones de purge.
// Extrait du monolithe models.go (N°88) — même package, contenu inchangé.
package model

import (
	"regexp"
	"strings"
	"time"
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

// IfaceTraffic — compteur cumulé et débit instantané d'une interface (F6).
type IfaceTraffic struct {
	Name    string `json:"name"`
	RxBytes int64  `json:"rxBytes"` // compteurs cumulés
	TxBytes int64  `json:"txBytes"`
	RxBps   int64  `json:"rxBps"` // débit calculé
	TxBps   int64  `json:"txBps"`
}

// TrafficPoint — point d'historique de trafic, somme toutes interfaces (F6).

// TrafficPoint — point d'historique de trafic, somme toutes interfaces (F6).
type TrafficPoint struct {
	T     string `json:"t"` // RFC3339
	RxBps int64  `json:"rxBps"`
	TxBps int64  `json:"txBps"`
}

// RouterTraffic — trafic temps réel d'un routeur (F6). Une entrée par
// routeur : ID = RouterID (pattern de persistance : clé primaire "id").

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

// scriptTagPattern — blocs <script>…</script> (insensible à la casse,
// multi-lignes) retirés des corps de templates à la sauvegarde.
var scriptTagPattern = regexp.MustCompile(`(?is)<script\b[^>]*>.*?(</script\s*>|$)`)

// SanitizeTemplateHTML retire les blocs <script>…</script> (y compris un
// bloc non fermé, jusqu'à la fin) du corps d'un modèle de voucher : le rendu
// se fait côté client à l'impression, aucune exécution de script n'est attendue.

// SanitizeTemplateHTML retire les blocs <script>…</script> (y compris un
// bloc non fermé, jusqu'à la fin) du corps d'un modèle de voucher : le rendu
// se fait côté client à l'impression, aucune exécution de script n'est attendue.
func SanitizeTemplateHTML(s string) string {
	return strings.TrimSpace(scriptTagPattern.ReplaceAllString(s, ""))
}

// Command — ordre déposé par le cloud, récupéré puis exécuté par l'agent.

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

// PurgeTombstone — marqueur anti-résurgence (audit purge) : posé par la purge
// admin pour CHAQUE username supprimé du cloud. La synchronisation agent
// (applyReadState) refuse de ré-importer un username tombstoné : le routeur
// réel garde ses /ip hotspot user après une purge, sans marqueur ils
// réapparaîtraient dans le cloud à la première synchro (résurgence constatée
// en production). Le tombstone EXPIRE (TTL — cf. PurgeTombstoneTTL) ou se
// LÈVE quand l'opérateur recrée volontairement le même username dans MikCloud :
// la découverte Winbox fonctionne à nouveau sans rien perdre.

// PurgeTombstone — marqueur anti-résurgence (audit purge) : posé par la purge
// admin pour CHAQUE username supprimé du cloud. La synchronisation agent
// (applyReadState) refuse de ré-importer un username tombstoné : le routeur
// réel garde ses /ip hotspot user après une purge, sans marqueur ils
// réapparaîtraient dans le cloud à la première synchro (résurgence constatée
// en production). Le tombstone EXPIRE (TTL — cf. PurgeTombstoneTTL) ou se
// LÈVE quand l'opérateur recrée volontairement le même username dans MikCloud :
// la découverte Winbox fonctionne à nouveau sans rien perdre.
type PurgeTombstone struct {
	ID        string `json:"id"`
	AccountID string `json:"accountId"`
	// Username en MINUSCULES (comparaison insensible à la casse avec les
	// rapports agent — RouterOS est sensible à la casse mais l'agent
	// remonte les noms tels quels ; le cloud normalise en lower).
	Username  string `json:"username"`
	PurgedAt  string `json:"purgedAt"`
	ExpiresAt string `json:"expiresAt"`
}

// PurgeTombstoneTTL — durée de vie d'un tombstone : au-delà, l'import
// automatique redevient possible (fenêtre de 30 jours, largement au-delà de
// tout cycle de synchro agent — un routeur hors-ligne pendant la purge ne
// peut pas ressusciter les données à son retour).

// PurgeTombstoneTTL — durée de vie d'un tombstone : au-delà, l'import
// automatique redevient possible (fenêtre de 30 jours, largement au-delà de
// tout cycle de synchro agent — un routeur hors-ligne pendant la purge ne
// peut pas ressusciter les données à son retour).
const PurgeTombstoneTTL = 30 * 24 * time.Hour

// ---------------------------------------------------------------------------
// N°28 — WiFi Jetable : mode d'accès offert aux établissements (restaurants,
// cafés, salons de coiffure, maquis, buvettes). Le client scanne un QR code,
// laisse son numéro (marketing opt-in), reçoit un code à quota gratuit
// (limit-uptime / limit-bytes-total côté routeur) directement sur la page
// publique, puis bascule en 1 clic vers une offre payante au-delà du quota.
// ---------------------------------------------------------------------------

// WifiSite — un établissement proposant le WiFi jetable. Le slug identifie
// la page publique /wifi/{slug} encodée dans le QR code affiché sur les
// tables (UNICITÉ GLOBALE : l'URL publique ne porte pas le compte).
// FreeTimeMin / FreeDataMb = quotas gratuits surchargés à l'émission
// (0 = hériter du profil) ; DailyPerPhone / DailyCap = garde-fous anti-abus
// (budget gratuit du gérant) ; Active = bascule 1 clic du gérant.
// N°49 : WifiSSID (+ WifiPassword si le réseau est WPA) alimente le QR de
// CONNEXION de l'affiche — format universel « WIFI:T:...;S:...;P:...;; »,
// scanné par l'appareil photo (iOS 11+ / Android 10+) : le téléphone
// propose de rejoindre le réseau, puis le portail captif inline (N°48)
// prend le relais (numéro → code → en ligne). Vide = affiche limitée au
// QR page web (/wifi/{slug}).
