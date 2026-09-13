// Identité routeur : résolution de conflits d'identité et parsing d'uptime RouterOS.
// Extrait du monolithe agent_handlers.go (N°88) — même package, contenu inchangé.
package api

import (
	"mikcloud/hotspot-api/internal/model"
	"strconv"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Sécurité S6 — détection d'identité routeur dupliquée (anti-fermage d'essai)
// ---------------------------------------------------------------------------
// L'agent déclare son empreinte RouterOS (System Identity + board-name) au
// register (script d'installation : /system identity get name + /system
// resource get board-name). Un client sous paywall (guard P3) peut créer un
// nouveau compte d'essai et y re-provisionner le MÊME routeur physique : le
// nouveau script écrase l'ancien scheduler, l'ancien compte devient muet.
// Le dédoublonnage des coordonnées (S5, handlers_auth.go) bloque les
// coordonnées réutilisées, pas les identités inventées. Ce garde-fou ferme
// la boucle :
//
//   - POST /agent/register : l'empreinte déclarée est comparée aux routeurs
//     ACTIFS (LastSeen < 24 h) des AUTRES comptes. Conflit → 409 code
//     « router_identity_conflict » + flag persistant IdentityConflict.
//   - GET /agent/cmd : un routeur flaggé ne reçoit AUCUNE commande tant que
//     le porteur de l'empreinte reste actif ; le flag se lève automatiquement
//     dès que le porteur disparaît (suppression du routeur fantôme par le
//     support — impersonation) ou dort plus de 24 h — le check-in reprend
//     alors normalement.
//
// Exclusions assumées : identités génériques (« mikrotik », défaut RouterOS,
// ou vide — des milliers d'appareils non renommés la portent) et MÊME compte
// (re-register, rotate-token, doublon logique = gestion interne du client,
// pas de l'abus plateforme). Fenêtre de 24 h : compromis documenté contre le
// faux positif « routeur revendu » — le support débloque en supprimant le
// routeur fantôme de l'ancien compte, ou l'attente naturelle sort son
// LastSeen de la fenêtre. Limite assumée : l'identity est forgeable par qui
// contrôle le routeur — la barrière vise le fermage de masse paresseux et
// rend TOUTE tentative visible dans le journal d'activité (traçabilité).

// identityConflictWindow — fenêtre de récence du porteur de l'empreinte :
// un routeur qui n'a plus check-in depuis plus de 24 h n'est plus considéré
// actif (l'appareil physique a vraisemblablement quitté ce compte).
const identityConflictWindow = 24 * time.Hour

// normalizeRouterIdent — normalisation d'empreinte : trim + minuscules.

// normalizeRouterIdent — normalisation d'empreinte : trim + minuscules.
func normalizeRouterIdent(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// genericRouterIdentity — identité non discriminante (défaut RouterOS ou
// vide) : jamais de conflit sur une empreinte générique.

// genericRouterIdentity — identité non discriminante (défaut RouterOS ou
// vide) : jamais de conflit sur une empreinte générique.
func genericRouterIdentity(ident string) bool {
	return ident == "" || ident == "mikrotik"
}

// identityHolderLocked — renvoie le routeur qui PORTE déjà cette empreinte
// (identity + modèle, normalisées) sur un AUTRE compte, vu ACTIF récemment
// (LastSeen < identityConflictWindow) ; nil sinon. Appelable sous verrou
// store. Garde interne : empreinte non discriminante → nil.

// identityHolderLocked — renvoie le routeur qui PORTE déjà cette empreinte
// (identity + modèle, normalisées) sur un AUTRE compte, vu ACTIF récemment
// (LastSeen < identityConflictWindow) ; nil sinon. Appelable sous verrou
// store. Garde interne : empreinte non discriminante → nil.
func identityHolderLocked(db *model.DB, router *model.Router, ident, mod string) *model.Router {
	if genericRouterIdentity(ident) || mod == "" {
		return nil
	}
	now := time.Now().UTC()
	for i := range db.Routers {
		other := &db.Routers[i]
		if other.ID == router.ID || other.AccountID == router.AccountID {
			continue // même routeur (re-register) ou même compte (gestion interne)
		}
		if normalizeRouterIdent(other.Host) != ident || normalizeRouterIdent(other.BoardName) != mod {
			continue // empreinte différente
		}
		seen, err := time.Parse(time.RFC3339, other.LastSeen)
		if err != nil || now.Sub(seen) >= identityConflictWindow {
			continue // porteur jamais check-in ou endormi hors fenêtre
		}
		return other
	}
	return nil
}

// queueCommandLocked — dépose une commande en file (sous verrou ; Save à charge
// de l'appelant). Déduplique les read_state déjà en attente. La commande porte
// l'identifiant du compte du routeur (isolation multi-tenant).

// parseRosUptime — parse une durée RouterOS ("3w1d02:15:30", "02:15:30", "2h30m", "45s").
func parseRosUptime(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	if i := strings.LastIndex(s, ":"); i >= 0 {
		seg := s
		if j := strings.LastIndexAny(s[:i], "wd"); j >= 0 {
			seg = s[j+1:]
		}
		parts := strings.Split(seg, ":")
		vals := make([]int, 0, 3)
		ok := true
		for _, p := range parts {
			n, err := strconv.Atoi(strings.TrimSpace(p))
			if err != nil {
				ok = false
				break
			}
			vals = append(vals, n)
		}
		if ok && len(vals) >= 2 {
			var hms int64
			if len(vals) == 3 {
				hms = int64(vals[0])*3600 + int64(vals[1])*60 + int64(vals[2])
			} else {
				hms = int64(vals[0])*60 + int64(vals[1])
			}
			prefix := s
			if k := strings.Index(s, seg); k > 0 {
				prefix = s[:k]
			}
			return hms + parseRosUptimeSuffix(prefix)
		}
	}
	return parseRosUptimeSuffix(s)
}

// parseRosUptimeSuffix — suffixes w/d/h/m/s ("3w1d", "2h30m", "45s").

// parseRosUptimeSuffix — suffixes w/d/h/m/s ("3w1d", "2h30m", "45s").
func parseRosUptimeSuffix(s string) int64 {
	var total int64
	num := 0
	has := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= '0' && c <= '9' {
			num = num*10 + int(c-'0')
			has = true
			continue
		}
		if !has {
			continue
		}
		switch c {
		case 'w':
			total += int64(num) * 7 * 86400
		case 'd':
			total += int64(num) * 86400
		case 'h':
			total += int64(num) * 3600
		case 'm':
			total += int64(num) * 60
		case 's':
			total += int64(num)
		}
		num = 0
		has = false
	}
	return total
}

// ---------------------------------------------------------------------------
// POST /agent/register
// ---------------------------------------------------------------------------
