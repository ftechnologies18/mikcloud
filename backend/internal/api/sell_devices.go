// sell_devices.go — N°66 : limite d'appareils simultanés par compte
// revendeur (Mode Vente).
//
// Un revendeur dont MaxDevices > 0 obtient au login PIN une SESSION inscrite
// au registre (db.SellSessions, table sell_sessions) dont l'identifiant — le
// jti — est embarqué dans le JWT. Chaque requête /api/sell/* recontrôle la
// présence de cette session : le token d'un appareil évincé est refusé en
// 401 (fin de session PWA, retour à l'écran PIN).
//
// Politique d'éviction : FIFO par login — l'appareil connecté depuis le plus
// longtemps (plus petit IssuedAt) est déconnecté quand la limite est dépassée
// (nouveau login) ou quand le gérant abaisse la limite (trim immédiat). Les
// sessions plus vieilles que le TTL du token + 1 h de grâce sont purgées au
// login suivant : le registre reste borné sans balayage global.
//
// Compatibilité : MaxDevices = 0 (défaut) conserve le comportement
// historique — token stateless SANS registre, aucune écriture, aucun
// contrôle. Les comptes existants ne changent de comportement qu'au moment
// où le gérant active une limite (les tokens alors en vol sont révoqués :
// ils ne portent pas de jti, le login suivant les régularise).

package api

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	"mikcloud/hotspot-api/internal/auth"
	"mikcloud/hotspot-api/internal/model"
)

// maxDevicesLimit — borne haute de la limite (garde-fou : au-delà de 20
// appareils simultanés, la limite n'a plus de valeur anti-partage).
const maxDevicesLimit = 20

// sellSessionGrace — marge au-delà du TTL du JWT (24 h) : une session dont
// le token vient d'expirer reste affichée « active » côté console, mais sera
// purgée au login suivant du même revendeur.
const sellSessionGrace = time.Hour

// sellSessionsCutoff — borne ISO (incluse = morte) en dessous de laquelle
// une session n'est plus comptée vivante.
func sellSessionsCutoff(now time.Time) string {
	return now.UTC().Add(-auth.TokenTTL - sellSessionGrace).Format(time.RFC3339)
}

// clampRunes — tronque une étiquette sans couper un caractère UTF-8.
func clampRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// activeSellSessionsLocked — sessions vivantes du revendeur, triées par
// IssuedAt croissant (la plus ancienne est la candidate à l'éviction).
// Verrou store requis.
func activeSellSessionsLocked(db *model.DB, resellerID, cutoffISO string) []model.SellSession {
	out := make([]model.SellSession, 0, 4)
	for _, s := range db.SellSessions {
		if s.ResellerID == resellerID && s.IssuedAt > cutoffISO {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].IssuedAt < out[j].IssuedAt })
	return out
}

// sellSessionLiveLocked — la session jti du revendeur vit-elle encore au
// registre ? Verrou store requis.
func sellSessionLiveLocked(db *model.DB, resellerID, jti, cutoffISO string) bool {
	for _, s := range db.SellSessions {
		if s.ID == jti && s.ResellerID == resellerID {
			return s.IssuedAt > cutoffISO
		}
	}
	return false
}

// trimSellSessionsLocked — purge les sessions MORTES du revendeur (fenêtre
// TTL dépassée), puis borne le registre à sa limite : les surnuméraires sont
// évincés du plus ancien au plus récent. Retourne les sessions évincées
// encore vivantes (audit) — nil quand le revendeur est illimité (0). Verrou
// store requis ; l'appelant persiste via Save().
func trimSellSessionsLocked(db *model.DB, res *model.Reseller, now time.Time) (evicted []model.SellSession) {
	cutoff := sellSessionsCutoff(now)
	kept := db.SellSessions[:0]
	for _, s := range db.SellSessions {
		if s.ResellerID == res.ID && s.IssuedAt <= cutoff {
			continue // session morte : purge silencieuse
		}
		kept = append(kept, s)
	}
	db.SellSessions = kept
	if res.MaxDevices <= 0 {
		return nil
	}
	live := activeSellSessionsLocked(db, res.ID, cutoff)
	for len(live) > res.MaxDevices {
		oldest := live[0]
		dropSellSessionLocked(db, oldest.ID)
		evicted = append(evicted, oldest)
		live = live[1:]
	}
	return evicted
}

// dropSellSessionLocked — retire UNE session du registre par identifiant.
// Verrou store requis.
func dropSellSessionLocked(db *model.DB, id string) {
	kept := db.SellSessions[:0]
	for _, s := range db.SellSessions {
		if s.ID != id {
			kept = append(kept, s)
		}
	}
	db.SellSessions = kept
}

// dropSellSessionsOfReseller — cascade de suppression d'un revendeur : son
// registre entier disparaît (aucun orphelin). Retourne le volume purgé.
// Verrou store requis.
func dropSellSessionsOfReseller(db *model.DB, resellerID string) int {
	n := 0
	kept := db.SellSessions[:0]
	for _, s := range db.SellSessions {
		if s.ResellerID == resellerID {
			n++
			continue
		}
		kept = append(kept, s)
	}
	db.SellSessions = kept
	return n
}

// registerSellLogin — inscrit la session d'un login PIN réussi (gère le
// verrou store lui-même, persiste). Retourne le jti à embarquer dans le JWT
// et le nombre d'appareils évincés (jti vide = revendeur illimité ou disparu
// entre-temps : token stateless historique). L'éviction est tracée dans le
// journal d'activité du compte.
func (a *API) registerSellLogin(resellerID, acc string, r *http.Request) (string, int) {
	now := time.Now().UTC()
	a.store.Lock()
	db := a.store.Data()
	res := findResellerScoped(db, resellerID, acc)
	if res == nil || res.MaxDevices <= 0 {
		a.store.Unlock()
		return "", 0
	}
	jti := model.NewID("dev-")
	db.SellSessions = append(db.SellSessions, model.SellSession{
		ID: jti, AccountID: acc, ResellerID: res.ID,
		IssuedAt: model.NowISO(), LastSeen: model.NowISO(),
		UserAgent: clampRunes(r.UserAgent(), 200), IP: clientIP(r),
	})
	evicted := trimSellSessionsLocked(db, res, now)
	if len(evicted) > 0 {
		a.logActivityBy(r, db, acc, "reseller", fmt.Sprintf(
			"Revendeur %s — %d appareil(s) déconnecté(s) : limite d'appareils simultanés (%d) atteinte",
			res.Name, len(evicted), res.MaxDevices))
	}
	a.store.Save()
	a.store.Unlock()
	return jti, len(evicted)
}

// devicesNote — mention d'audit quand une limite d'appareils est définie.
func devicesNote(n int) string {
	if n <= 0 {
		return ""
	}
	return fmt.Sprintf(" (limite d'appareils : %d)", n)
}
