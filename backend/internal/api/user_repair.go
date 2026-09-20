// Autoréparation des utilisateurs absents — N°162.
//
// INCIDENT Zikisso (20/09/2026) : un lot de vouchers généré pour le routeur
// « Wifi Zikisso » (compte à DEUX routeurs) a échoué INTÉGRALEMENT à la
// création sur le routeur — sans que le cloud ne le sache : le script de lot
// avalait chaque échec d'ajout dans un log routeur (on-error sans compteur)
// et rapportait « ok / created=N ». Les tickets restaient « Actif », le
// read_state ne les trouvait pas → badge « absent du routeur », connexion
// impossible chez le client — et SANS retrouvabilité : les écritures ne sont
// jamais rejouées (N°73), la commande « réussie » ne repart jamais.
//
// PRINCIPE : le cloud est le registre durable (contrat read_state — « RIEN
// n'est supprimé automatiquement »). Un utilisateur ACTIF badgé absent est
// une créature du registre qui DOIT vivre sur son routeur : la réconciliation
// COMPLÈTE renvoie les absents en commande de réparation idempotente
// (re-parsée par le même builder que la génération, compteur d'échecs
// véridique N°162, garde d'existence par nom — l'existant n'est jamais
// retouché : verrou MAC, marqueur mikq:, comment de traçabilité).
//
// DISCIPLINE DE VOLUME (N°159) : une seule vague en file par routeur,
// bornée à repairBatchCap utilisateurs, évaluée uniquement aux
// réconciliations complètes (jamais sur un chunk intermédiaire), cadencée
// par le backoff des watchers (1 → 5 → 15 → 30 min après échec, reset sur
// succès) sous une clé SYNTHÉTIQUE — les lots de génération classiques ne
// touchent pas cette cadence. Tombstones respectées : un username purgé
// n'est JAMAIS ressuscité.
package api

import (
	"fmt"
	"strings"
	"time"

	"mikcloud/hotspot-api/internal/model"
)

const (
	// repairBackoffKind — clé synthétique du backoff N°159 pour la vague de
	// réparation (pas un kind de commande réelle : la clé ne vit que dans
	// les maps watcherFailN/watcherFailAt).
	repairBackoffKind = "user_repair"

	// repairBatchCap — utilisateurs maximum par commande de réparation :
	// le script .rsc reste loin de la limite RouterOS (~64 Ko) et les grands
	// parcs se drainent une vague par cycle read_state.
	repairBatchCap = 100
)

// repairCommandInFlight — une vague de réparation est-elle déjà en file
// (queued) ou en vol (sent) pour CE routeur ? Une seule à la fois : le
// read_state suivant réévaluera les absents restants une fois la vague
// appliquée. À appeler sous le verrou du store.
func repairCommandInFlight(db *model.DB, routerID string) bool {
	for i := range db.Commands {
		c := &db.Commands[i]
		if c.RouterID != routerID || c.Kind != model.CmdVoucherBatch {
			continue
		}
		if c.Status != "queued" && c.Status != "sent" {
			continue
		}
		if v, ok := c.Payload["repair"].(bool); ok && v {
			return true
		}
	}
	return false
}

// queueMissingRepair — N°162 — enfile UNE vague de réparation pour les
// utilisateurs ACTIFS badgés absents (post-grâce, hors tombstones, profil
// résolvable), groupés par profil (chaque groupe embarque profileRef : le
// profil cloud est réaligné par profileEnsureLine — l'autoguérison du profil
// voyage avec la vague, une cause racine fréquente des lots en échec étant
// un profil référençant une ressource absente du routeur). Les limites
// résolues À LA GÉNÉRATION voyagent par voucher (quota/uptime stockés sur
// le ticket — réparer un « 5 Go » sans limite serait offrir des données).
// À appeler sous le verrou du store, depuis une réconciliation COMPLÈTE.
func (a *API) queueMissingRepair(db *model.DB, router *model.Router, now time.Time, tomb map[string]bool) {
	if router.Mode != "agent" {
		return
	}
	if a.watcherBackoffBlocks(router.ID, repairBackoffKind, now) {
		return
	}
	if repairCommandInFlight(db, router.ID) {
		return
	}
	type repairGroup struct {
		profile *model.Profile
		users   []model.HotspotUser
	}
	groups := map[string]*repairGroup{}
	order := []string{} // ordre d'émission stable (premier profil vu d'abord)
	for i := range db.HotspotUsers {
		u := &db.HotspotUsers[i]
		if u.RouterID != router.ID || !u.MissingOnRouter || u.Status != "active" {
			continue
		}
		if u.ProfileID == "" && u.ProfileName == "" {
			continue
		}
		if tomb[strings.ToLower(u.Username)] {
			continue // purgé : jamais ressuscité (garde anti-résurrection)
		}
		prof := findProfileScoped(db, u.ProfileID, router.AccountID)
		if prof == nil {
			continue // profil supprimé : la réparation attend la réaffectation du gérant
		}
		g, ok := groups[prof.ID]
		if !ok {
			g = &repairGroup{profile: prof}
			groups[prof.ID] = g
			order = append(order, prof.ID)
		}
		if len(g.users) >= repairBatchCap {
			continue // la vague suivante (prochain cycle) prendra le relais
		}
		g.users = append(g.users, *u)
	}
	for _, pid := range order {
		g := groups[pid]
		if len(g.users) == 0 {
			continue
		}
		users := make([]map[string]any, 0, len(g.users))
		for _, u := range g.users {
			entry := map[string]any{"name": u.Username, "password": u.Password}
			// Limites résolues à la génération, stockées par ticket :
			// la réparation est FIDÈLE au ticket vendu.
			if u.DataQuotaMb > 0 {
				entry["limitBytesTotal"] = u.DataQuotaMb * 1048576
			}
			if u.TimeLimitMin > 0 {
				entry["limitUptimeMin"] = u.TimeLimitMin
			}
			users = append(users, entry)
		}
		payload := map[string]any{
			"profile": profileRef(*g.profile),
			"users":   users,
			// Traçabilité routeur : « mikcloud:repair » (le marqueur mikq:
			// du mode bridage est posé par voucher dans le script).
			"comment": "mikcloud:repair",
			"repair":  true,
		}
		// N°106 — mode bridage : le débit voyage au niveau racine (le
		// marqueur mikq: par voucher en est dérivé côté script).
		if g.profile.QuotaModeEffective() == model.QuotaModeThrottle && g.profile.ThrottleRate != "" {
			payload["throttleRate"] = g.profile.ThrottleRate
		}
		cmd := queueCommandLocked(db, router.AccountID, router.ID, model.CmdVoucherBatch, payload)
		a.logActivity(db, router.AccountID, "voucher",
			fmt.Sprintf("Réparation : %d utilisateur(s) absent(s) du routeur renvoyé(s) sur «%s» (commande %s)",
				len(users), router.Name, cmd.ID))
	}
}
