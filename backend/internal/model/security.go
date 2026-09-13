// Modules sécurité routeur : niveaux SafeWiFi / Shield et configuration FamilyGuard (spéc + validité).
// Extrait du monolithe models.go (N°88) — même package, contenu inchangé.
package model

import (
	"strings"
	"time"
)

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
	CmdReadResources   = "read_resources"   // Parité Mikhmon : noms /ip pool + /queue simple + /ip hotspot
	CmdSchedulerAdd    = "scheduler_add"    // F10 : /system scheduler add
	CmdSchedulerSet    = "scheduler_set"    // F10 : /system scheduler set (disabled)
	CmdSchedulerRemove = "scheduler_remove" // F10 : /system scheduler remove
	CmdReboot          = "reboot"           // F10 : /system reboot
	CmdShutdown        = "shutdown"         // F10 : /system shutdown
	CmdImportHotspot   = "import_hotspot"   // import initial : lecture paginée des profils + utilisateurs existants sur le routeur
	CmdProfileSet      = "profile_set"      // v2 : applique/retire le verrou « 1er appareil » (on-login de liaison MAC) sur un profil
	CmdWalledGarden    = "walled_garden"    // N°29 : walled-garden d'inscription publique (runbook N°27-D automatisé)
	CmdHotspotFiles    = "hotspot_files"    // N°35 : déploiement automatique du portail captif (login.html, status.html, assets) — pattern walled_garden
	CmdWatcherEnsure   = "watcher_ensure"   // N°77 : veilleur d'invités — scheduler mikcloud-watch (check-in 20 s quand un hôte non autorisé est présent)
	CmdSafeWifi        = "safewifi"         // N°80 : protection DNS du WiFi public — redirection du port 53 vers un résolveur filtrant (règles marquées mikcloud-safewifi, idempotent) ; N°85 : durcie — NAT en tête de table, DoT/DoH bloqués, IPv6 coupé (best-effort)
	CmdShield          = "shield"           // N°81 : bouclier réseau du WiFi public — administration du routeur et vecteurs malveillants bloqués pour les clients (règles filter marquées mikcloud-shield, idempotent)
	CmdFamilyGuard     = "familyguard"      // N°82 : couvre-feu internet du WiFi public — fenêtre horaire pendant laquelle l'internet des clients est coupé (règles filter marquées mikcloud-familyguard, idempotent)
	CmdAntiVpn         = "antivpn"          // N°88 : bloque-VPN du WiFi public — VPN et tunnels standards (GRE, ESP, IKE, L2TP, PPTP, OpenVPN, WireGuard, WARP, Tor) coupés pour les clients (règles filter marquées mikcloud-antivpn, idempotent)
	CmdPoolDoctor      = "pool_doctor"      // N°97 : docteur du pool d'adresses IP du hotspot — diagnostic (pools/serveurs/profils/hôtes) + recyclage des IP zombies (login/idle/keepalive-timeout, address-per-mac=1) + extension de capacité optionnelle (range 10.77.0.0/21) ; idempotent, objets marqués mikcloud-pool
)

// N°80 — niveaux SafeWiFi (filtrage DNS du WiFi public par redirection).
// Le niveau est choisi par le gérant dans la console et converge via la
// commande safewifi au check-in suivant (pattern walled-garden).

// N°80 — niveaux SafeWiFi (filtrage DNS du WiFi public par redirection).
// Le niveau est choisi par le gérant dans la console et converge via la
// commande safewifi au check-in suivant (pattern walled-garden).
const (
	SafeWifiOff     = "off"     // aucune protection
	SafeWifiThreats = "threats" // malwares, phishing, arnaques (Quad9)
	SafeWifiFamily  = "family"  // + contenus adultes, publicités (AdGuard Family)
)

// ValidSafeWifiLevel — vrai si le niveau fait partie du contrat N°80.

// ValidSafeWifiLevel — vrai si le niveau fait partie du contrat N°80.
func ValidSafeWifiLevel(l string) bool {
	return l == SafeWifiOff || l == SafeWifiThreats || l == SafeWifiFamily
}

// SafeWifiLevelEffective — niveau de filtrage courant de ce routeur,
// normalisé ("" ou valeur inconnue = état antérieur au N°80 → off).

// SafeWifiLevelEffective — niveau de filtrage courant de ce routeur,
// normalisé ("" ou valeur inconnue = état antérieur au N°80 → off).
func (r *Router) SafeWifiLevelEffective() string {
	if !ValidSafeWifiLevel(r.SafeWifiLevel) {
		return SafeWifiOff
	}
	return r.SafeWifiLevel
}

// N°81 — niveaux Shield (bouclier réseau du WiFi public).

// N°81 — niveaux Shield (bouclier réseau du WiFi public).
const (
	ShieldOff = "off" // aucun blindage
	ShieldOn  = "on"  // administration du routeur et vecteurs malveillants bloqués pour les clients WiFi
)

// ValidShieldLevel — vrai si le niveau fait partie du contrat N°81.

// ValidShieldLevel — vrai si le niveau fait partie du contrat N°81.
func ValidShieldLevel(l string) bool {
	return l == ShieldOff || l == ShieldOn
}

// ShieldLevelEffective — niveau de blindage courant de ce routeur,
// normalisé ("" ou valeur inconnue = état antérieur au N°81 → off).

// ShieldLevelEffective — niveau de blindage courant de ce routeur,
// normalisé ("" ou valeur inconnue = état antérieur au N°81 → off).
func (r *Router) ShieldLevelEffective() string {
	if !ValidShieldLevel(r.ShieldLevel) {
		return ShieldOff
	}
	return r.ShieldLevel
}

// N°88 — niveaux AntiVPN (bloque-VPN du WiFi public). Le niveau est
// choisi par le gérant dans la console et converge via la commande
// antivpn au check-in suivant (pattern walled-garden).

// N°88 — niveaux AntiVPN (bloque-VPN du WiFi public). Le niveau est
// choisi par le gérant dans la console et converge via la commande
// antivpn au check-in suivant (pattern walled-garden).
const (
	AntiVpnOff = "off" // aucun blocage
	AntiVpnOn  = "on"  // VPN et tunnels standards coupés pour les clients WiFi
)

// ValidAntiVpnLevel — vrai si le niveau fait partie du contrat N°88.

// ValidAntiVpnLevel — vrai si le niveau fait partie du contrat N°88.
func ValidAntiVpnLevel(l string) bool {
	return l == AntiVpnOff || l == AntiVpnOn
}

// AntiVpnLevelEffective — niveau de blocage courant de ce routeur,
// normalisé ("" ou valeur inconnue = état antérieur au N°88 → off).

// AntiVpnLevelEffective — niveau de blocage courant de ce routeur,
// normalisé ("" ou valeur inconnue = état antérieur au N°88 → off).
func (r *Router) AntiVpnLevelEffective() string {
	if !ValidAntiVpnLevel(r.AntiVpnLevel) {
		return AntiVpnOff
	}
	return r.AntiVpnLevel
}

// ---------------------------------------------------------------------------
// N°82 — FamilyGuard : couvre-feu internet du WiFi public (logique pure)
// ---------------------------------------------------------------------------
//
// Le gérant programme une fenêtre horaire (ex. 22:00 → 06:00 tous les
// soirs) pendant laquelle l'internet du WiFi public est coupé. Complète la
// gamme sécurité : SafeWiFi (N°80) filtre QUOI (menaces, contenus),
// Shield (N°81) protège CONTRE QUI (administration, propagation), FamilyGuard
// décide QUAND l'internet est accessible (nuit, heures de fermeture, salle
// familiale).
//
// ARBITRAGE (documenté) : l'ÉTAT désiré — couvre-feu en cours ou non — est
// calculé PAR LE CLOUD à chaque check-in, en UTC (== heure d'Abidjan GMT,
// la Côte d'Ivoire n'applique pas l'heure d'été). L'horloge routeur N'est
// PAS consultée : un routeur sans NTP (fréquent sur le terrain) verrait le
// couvre-feu partir à la mauvaise heure via le paramètre natif time= de
// RouterOS. Contrepartie assumée : la bascule s'applique au check-in
// suivant (≤ 45 s console ouverte — attention N°75, ≤ 180 s en veille),
// et un routeur hors-ligne qui revient converge immédiatement vers
// l'état « maintenant » (aucune commande périmée en attente).

// FamilyGuardConfig — fenêtre du couvre-feu, forme structurée.

// ---------------------------------------------------------------------------
// N°82 — FamilyGuard : couvre-feu internet du WiFi public (logique pure)
// ---------------------------------------------------------------------------
//
// Le gérant programme une fenêtre horaire (ex. 22:00 → 06:00 tous les
// soirs) pendant laquelle l'internet du WiFi public est coupé. Complète la
// gamme sécurité : SafeWiFi (N°80) filtre QUOI (menaces, contenus),
// Shield (N°81) protège CONTRE QUI (administration, propagation), FamilyGuard
// décide QUAND l'internet est accessible (nuit, heures de fermeture, salle
// familiale).
//
// ARBITRAGE (documenté) : l'ÉTAT désiré — couvre-feu en cours ou non — est
// calculé PAR LE CLOUD à chaque check-in, en UTC (== heure d'Abidjan GMT,
// la Côte d'Ivoire n'applique pas l'heure d'été). L'horloge routeur N'est
// PAS consultée : un routeur sans NTP (fréquent sur le terrain) verrait le
// couvre-feu partir à la mauvaise heure via le paramètre natif time= de
// RouterOS. Contrepartie assumée : la bascule s'applique au check-in
// suivant (≤ 45 s console ouverte — attention N°75, ≤ 180 s en veille),
// et un routeur hors-ligne qui revient converge immédiatement vers
// l'état « maintenant » (aucune commande périmée en attente).

// FamilyGuardConfig — fenêtre du couvre-feu, forme structurée.
type FamilyGuardConfig struct {
	Enabled bool   // false : configuré mais désactivé (la fenêtre est conservée)
	Start   string // "HH:MM" début (inclus)
	End     string // "HH:MM" fin (EXCLU — la fenêtre s'arrête à 06:00, pas 06:00:59)
	Days    string // "1111111" — lundi→dimanche, '1' = la fenêtre DÉMARRE ce jour
}

// familyGuardDayCount — indices de la chaîne Days (lundi = 0).

// familyGuardDayCount — indices de la chaîne Days (lundi = 0).
const familyGuardDayCount = 7

// familyGuardMinutes — "HH:MM" → minutes depuis minuit ; ok=false si mal formé.

// familyGuardMinutes — "HH:MM" → minutes depuis minuit ; ok=false si mal formé.
func familyGuardMinutes(hhmm string) (int, bool) {
	if len(hhmm) != 5 || hhmm[2] != ':' {
		return 0, false
	}
	h := int(hhmm[0]-'0')*10 + int(hhmm[1]-'0')
	m := int(hhmm[3]-'0')*10 + int(hhmm[4]-'0')
	if h > 23 || m > 59 {
		return 0, false
	}
	return h*60 + m, true
}

// ValidFamilyGuardConfig — validation stricte : heures bien formées,
// début ≠ fin (une fenêtre nulle est ambiguë), exactement 7 jours 0/1 dont
// au moins un actif (sinon rien ne se déclenche jamais).

// ValidFamilyGuardConfig — validation stricte : heures bien formées,
// début ≠ fin (une fenêtre nulle est ambiguë), exactement 7 jours 0/1 dont
// au moins un actif (sinon rien ne se déclenche jamais).
func ValidFamilyGuardConfig(c FamilyGuardConfig) bool {
	s, okS := familyGuardMinutes(c.Start)
	e, okE := familyGuardMinutes(c.End)
	if !okS || !okE || s == e {
		return false
	}
	if len(c.Days) != familyGuardDayCount {
		return false
	}
	anyDay := false
	for i := 0; i < familyGuardDayCount; i++ {
		if c.Days[i] != '0' && c.Days[i] != '1' {
			return false
		}
		if c.Days[i] == '1' {
			anyDay = true
		}
	}
	return anyDay
}

// SpecString — forme canonique persistée dans Router.FamilyGuardSpec :
// "1|22:00|06:00|1111111".

// SpecString — forme canonique persistée dans Router.FamilyGuardSpec :
// "1|22:00|06:00|1111111".
func (c FamilyGuardConfig) SpecString() string {
	enabled := "0"
	if c.Enabled {
		enabled = "1"
	}
	return enabled + "|" + c.Start + "|" + c.End + "|" + c.Days
}

// ParseFamilyGuardSpec — "" → ok=false (jamais utilisé). Toute forme
// invalide → ok=false (défense : la colonne ne reçoit que des specs
// validés par le handler, mais le parse reste strict).

// ParseFamilyGuardSpec — "" → ok=false (jamais utilisé). Toute forme
// invalide → ok=false (défense : la colonne ne reçoit que des specs
// validés par le handler, mais le parse reste strict).
func ParseFamilyGuardSpec(s string) (FamilyGuardConfig, bool) {
	parts := strings.Split(s, "|")
	if len(parts) != 4 {
		return FamilyGuardConfig{}, false
	}
	c := FamilyGuardConfig{Enabled: parts[0] == "1", Start: parts[1], End: parts[2], Days: parts[3]}
	return c, ValidFamilyGuardConfig(c)
}

// ActiveAt — vrai si `now` tombe dans la fenêtre du couvre-feu.
// Sémantique des jours : un jour positionné est le jour de DÉBUT de la
// fenêtre — « vendredi » + 22:00→06:00 couvre vendredi 22:00 → samedi 06:00
// (la portion après minuit appartient à la fenêtre PARTIE la veille, même
// si le samedi n'est pas coché).

// ActiveAt — vrai si `now` tombe dans la fenêtre du couvre-feu.
// Sémantique des jours : un jour positionné est le jour de DÉBUT de la
// fenêtre — « vendredi » + 22:00→06:00 couvre vendredi 22:00 → samedi 06:00
// (la portion après minuit appartient à la fenêtre PARTIE la veille, même
// si le samedi n'est pas coché).
func (c FamilyGuardConfig) ActiveAt(now time.Time) bool {
	if !c.Enabled {
		return false
	}
	s, okS := familyGuardMinutes(c.Start)
	e, okE := familyGuardMinutes(c.End)
	if !okS || !okE || s == e {
		return false
	}
	day := (int(now.Weekday()) + 6) % familyGuardDayCount // lundi=0 … dimanche=6
	m := now.Hour()*60 + now.Minute()
	if s < e {
		// fenêtre intra-jour : 08:00 → 12:00
		return m >= s && m < e && c.Days[day] == '1'
	}
	// fenêtre franchissant minuit : 22:00 → 06:00 — active si on est
	// dans la portion du soir (jour de début) OU celle du matin (jour
	// de début = veille).
	prev := (day + familyGuardDayCount - 1) % familyGuardDayCount
	return (m >= s && c.Days[day] == '1') || (m < e && c.Days[prev] == '1')
}

// ---------------------------------------------------------------------------
// P0/P1 (audit Mikhmon) — nouveaux types (contrat V2 : F2/F3/F6/F7/F10)
// ---------------------------------------------------------------------------

// VoucherTemplate — modèle d'impression de vouchers (F2). Le rendu des
// variables {{…}} se fait côté CLIENT à l'impression ; le corps est stocké
// tel quel (scripts retirés à la sauvegarde — voir SanitizeTemplateHTML).
