// Type DB (racine de l'état en mémoire) et logique de résolution de statut des vouchers.
// Extrait du monolithe models.go (N°88) — même package, contenu inchangé.
package model

import "time"

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
	// Facturation (verrou du cycle) — file des demandes de souscription /
	// renouvellement, actionnable depuis la console plateforme.
	BillingRequests []BillingRequest `json:"billingRequests"`
	// Tombstones de purge (audit purge/résurgence) — voir PurgeTombstone.
	PurgeTombstones []PurgeTombstone `json:"purgeTombstones"`
	// N°27 — inscriptions publiques par QR : liens d'invitation + demandes.
	JoinLinks            []JoinLink            `json:"joinLinks"`
	RegistrationRequests []RegistrationRequest `json:"registrationRequests"`
	// N°28 — WiFi jetable : sites publics + registre marketing visiteurs.
	WifiSites  []WifiSite  `json:"wifiSites"`
	WifiGuests []WifiGuest `json:"wifiGuests"`
	// N°56 — analytics du portail hospitalité (impressions/clics par promo).
	// Journal borné : déduplication par (compte, promo, type, appareil, jour)
	// + rétention 90 jours + plafond mémoire (voir prunePromoEvents).
	PromoEvents []PromoEvent `json:"promoEvents"`
	// Abonnement récurrent par carte (Stripe via GeniusPay) — prélèvements
	// automatiques, synchronisés avec l'API abonnements GeniusPay.
	GeniusPaySubs []GeniusPaySub `json:"geniuspaySubs"`
	// N°66 — registre des sessions PIN Mode Vente (limite d'appareils
	// simultanés par revendeur) — voir SellSession.
	SellSessions []SellSession `json:"sellSessions"`
	// N°101 — appareils des foyers HomeNet (bails DHCP rapportés par
	// read_dhcp + noms affectés + pause dîner). Remplie UNIQUEMENT pour
	// les routeurs agent de comptes homenet (le cadenceur du check-in
	// garde la famille — les clics DHCP de la console hotspot restent
	// du cache outil F9, jamais des lignes d'appareil).
	Devices []Device `json:"devices"`
	// N°68 — demandes de réinitialisation de mot de passe (« Mot de passe
	// oublié ? ») : token hashé, expiration 60 min, usage unique — voir
	// PasswordReset. Borné par prunePasswordResetsLocked.
	PasswordResets []PasswordReset `json:"passwordResets"`
	Tenant         Tenant          `json:"tenant"`   // legacy mono-tenant
	Settings       Settings        `json:"settings"` // legacy mono-tenant
	LastTick       time.Time       `json:"lastTick"`
	// LastSweep — N°64 — horodatage du dernier BALAYAGE PÉRIODIQUE de
	// rétention (goroutine main.go, 1 h) : purge des journaux utilisateurs
	// à 90 j + expirations/nettoyages, indépendamment des visites console
	// (le Tick paresseux des handlers ne datait QUE ces lectures-là).
	// Preuve d'audit exposée par GET / (lastSweepAt). Même mécanique de
	// persistance que LastTick (colonne settings.last_sweep).
	LastSweep time.Time `json:"lastSweep"`
}

// voucherExpired — expiration « calculée » d'un voucher : validité (ExpiresAt)
// dépassée OU quota temps cumulé (limit-uptime, reflété au cloud via
// uptimeUsedSec) épuisé. Indépendant du statut stocké. expiresAt vide =
// voucher jamais connecté (validité ancrée au 1er login) : pas d'échéance
// par date.

// voucherExpired — expiration « calculée » d'un voucher : validité (ExpiresAt)
// dépassée OU quota temps cumulé (limit-uptime, reflété au cloud via
// uptimeUsedSec) épuisé. Indépendant du statut stocké. expiresAt vide =
// voucher jamais connecté (validité ancrée au 1er login) : pas d'échéance
// par date.
func voucherExpired(u *HotspotUser, now time.Time) bool {
	if u.Kind != "voucher" {
		return false
	}
	if u.ExpiresAt != "" {
		if exp, err := time.Parse(time.RFC3339, u.ExpiresAt); err == nil && now.After(exp) {
			return true
		}
	}
	if u.TimeLimitMin > 0 && u.UptimeUsedSec >= u.TimeLimitMin*60 {
		return true
	}
	return false
}

// TimeLimitParityGraceSec — tolérance de parité (secondes) entre le cumul
// cloud d'uptime et la coupure ROUTEUR limit-uptime. La session disparaît du
// read_state APRÈS la coupure : le dernier échantillon rapporté peut manquer
// la limite de jusqu'à un intervalle de lecture (scheduler agent = 45 s).
// Sans tolérance, un voucher coupé par le routeur resterait « utilisé » à
// jamais — aucun logout ultérieur ne viendrait combler le déficit de quelques
// secondes. Dans cette fenêtre, la déconnexion observée EST l'épuisement du
// quota : le cumul est aligné sur la limite (accumulateUptime +
// RepairTimeLimitParity).

// TimeLimitParityGraceSec — tolérance de parité (secondes) entre le cumul
// cloud d'uptime et la coupure ROUTEUR limit-uptime. La session disparaît du
// read_state APRÈS la coupure : le dernier échantillon rapporté peut manquer
// la limite de jusqu'à un intervalle de lecture (scheduler agent = 45 s).
// Sans tolérance, un voucher coupé par le routeur resterait « utilisé » à
// jamais — aucun logout ultérieur ne viendrait combler le déficit de quelques
// secondes. Dans cette fenêtre, la déconnexion observée EST l'épuisement du
// quota : le cumul est aligné sur la limite (accumulateUptime +
// RepairTimeLimitParity).
const TimeLimitParityGraceSec = 60

// RepairTimeLimitParity — réparation idempotente des vouchers dont la session
// a été coupée par le routeur (limit-uptime atteint) mais dont le cumul cloud
// est resté juste SOUS la limite (déficit d'échantillonnage ≤ intervalle de
// lecture). Sans réparation, ces tickets restent affichés « utilisés » alors
// qu'ils sont inutilisables (le routeur refuse la reconnexion) — ils ne
// passeraient « expirés » qu'à l'échéance de leur validité. Règle : sans
// session live, un voucher dont le cumul est dans la fenêtre de grâce sous la
// limite est aligné sur la limite — voucherExpired le repasse alors « expiré »
// dès le prochain affichage. Renvoie le nombre de vouchers realignés.
// À appeler sous verrou.

// RepairTimeLimitParity — réparation idempotente des vouchers dont la session
// a été coupée par le routeur (limit-uptime atteint) mais dont le cumul cloud
// est resté juste SOUS la limite (déficit d'échantillonnage ≤ intervalle de
// lecture). Sans réparation, ces tickets restent affichés « utilisés » alors
// qu'ils sont inutilisables (le routeur refuse la reconnexion) — ils ne
// passeraient « expirés » qu'à l'échéance de leur validité. Règle : sans
// session live, un voucher dont le cumul est dans la fenêtre de grâce sous la
// limite est aligné sur la limite — voucherExpired le repasse alors « expiré »
// dès le prochain affichage. Renvoie le nombre de vouchers realignés.
// À appeler sous verrou.
func RepairTimeLimitParity(db *DB) int {
	live := make(map[string]bool, len(db.Sessions))
	for _, s := range db.Sessions {
		if s.UserID != "" {
			live[s.UserID] = true // session en cours : le routeur n'a pas encore coupé
		}
	}
	n := 0
	for i := range db.HotspotUsers {
		u := &db.HotspotUsers[i]
		if u.Kind != "voucher" || u.TimeLimitMin <= 0 || live[u.ID] {
			continue
		}
		if u.Status != "active" && u.Status != "used" {
			continue // disabled/expired : déjà hors service côté stockage
		}
		limit := u.TimeLimitMin * 60
		if u.UptimeUsedSec > 0 && u.UptimeUsedSec < limit && limit-u.UptimeUsedSec <= TimeLimitParityGraceSec {
			u.UptimeUsedSec = limit
			n++
		}
	}
	return n
}

// AnchorVoucherValidity — ancre la validité d'un voucher à son PREMIER login :
// expiresAt = 1er login + validité du profil COURANT (parité routeur : le
// profil est lu à l'authentification). Appelé au 1er login détecté (agent)
// et à l'ouverture de session simulée. Un ticket jamais connecté n'a pas
// d'expiresAt (reste « actif » en stock indéfiniment) ; profil introuvable
// ou validité nulle : expiresAt reste vide (pas d'échéance par date).
// À appeler sous verrou. Renvoie true si l'ancrage a été posé.

// AnchorVoucherValidity — ancre la validité d'un voucher à son PREMIER login :
// expiresAt = 1er login + validité du profil COURANT (parité routeur : le
// profil est lu à l'authentification). Appelé au 1er login détecté (agent)
// et à l'ouverture de session simulée. Un ticket jamais connecté n'a pas
// d'expiresAt (reste « actif » en stock indéfiniment) ; profil introuvable
// ou validité nulle : expiresAt reste vide (pas d'échéance par date).
// À appeler sous verrou. Renvoie true si l'ancrage a été posé.
func AnchorVoucherValidity(db *DB, u *HotspotUser, now time.Time) bool {
	if u.Kind != "voucher" || u.ExpiresAt != "" {
		return false
	}
	for i := range db.Profiles {
		p := &db.Profiles[i]
		if p.ID == u.ProfileID && p.AccountID == u.AccountID {
			if v := p.ValidityMinutes(); v > 0 {
				u.ExpiresAt = now.Add(time.Duration(v) * time.Minute).Format(time.RFC3339)
				return true
			}
			return false
		}
	}
	return false
}

// EffectiveStatus retourne le statut réel d'un utilisateur :
//   - un voucher (actif ou utilisé en base) dont la validité est dépassée ou
//     dont le quota temps est épuisé est renvoyé "expired" ;
//   - un voucher "active" en base mais déjà connecté au moins une fois
//     (UsedAt renseigné, ex. réactivé après désactivation) est renvoyé "used".
//
// Les autres statuts et les utilisateurs réguliers sont renvoyés tels quels.
// Pour l'AFFICHAGE, utiliser ResolvedStatus (5 états priorisés).

// EffectiveStatus retourne le statut réel d'un utilisateur :
//   - un voucher (actif ou utilisé en base) dont la validité est dépassée ou
//     dont le quota temps est épuisé est renvoyé "expired" ;
//   - un voucher "active" en base mais déjà connecté au moins une fois
//     (UsedAt renseigné, ex. réactivé après désactivation) est renvoyé "used".
//
// Les autres statuts et les utilisateurs réguliers sont renvoyés tels quels.
// Pour l'AFFICHAGE, utiliser ResolvedStatus (5 états priorisés).
func EffectiveStatus(u *HotspotUser, now time.Time) string {
	if u.Kind == "voucher" && u.Status != "disabled" && voucherExpired(u, now) {
		return "expired"
	}
	if u.Status == "active" && u.UsedAt != "" {
		return "used"
	}
	return u.Status
}

// ResolvedStatus — statut AFFICHÉ (5 états priorisés) :
//
//  1. expired  — validité dépassée ou quota temps épuisé (calculé) ;
//  2. disabled — désactivation manuelle ;
//  3. online   — session live au dernier read_state (≤ 45 s de latence) ;
//  4. used     — déjà connecté au moins une fois, hors ligne ;
//  5. active   — jamais connecté (disponible).
//
// `online` provient de la carte des sessions live (voir onlineSessions, api).

// ResolvedStatus — statut AFFICHÉ (5 états priorisés) :
//
//  1. expired  — validité dépassée ou quota temps épuisé (calculé) ;
//  2. disabled — désactivation manuelle ;
//  3. online   — session live au dernier read_state (≤ 45 s de latence) ;
//  4. used     — déjà connecté au moins une fois, hors ligne ;
//  5. active   — jamais connecté (disponible).
//
// `online` provient de la carte des sessions live (voir onlineSessions, api).
func ResolvedStatus(u *HotspotUser, online bool, now time.Time) string {
	if u.Kind == "voucher" && voucherExpired(u, now) {
		return "expired"
	}
	switch u.Status {
	case "expired":
		return "expired"
	case "disabled":
		return "disabled"
	}
	if online {
		return "online"
	}
	if u.UsedAt != "" {
		return "used"
	}
	return u.Status
}
