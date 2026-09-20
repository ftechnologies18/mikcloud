// handlers_bell.go — N°151 : la vraie boîte de notifications de la console.
//
// GET  /api/bell        → {items, seenAt, unread} — journal du compte filtré
//
//	par le RBAC (N°149 : un gérant ne voit pas billing/
//	team), borné, trié décroissant ; seenAt LU PAR
//	UTILISATEUR (AdminUser.ActivitySeenAt) ; unread
//	compte TOUT le journal visible au-delà de la limite
//	(le badge ne ment pas sur une 21e entrée).
//
// POST /api/bell/seen   → acquitte : seenAt = maintenant (ou l'instant
//
//	fourni — migration de l'ancien localStorage, borné
//	à maintenant), MONOTONE (un acquit ne recule
//	jamais), persisté en base : le badge devient
//	cohérent multi-appareils et par membre de l'équipe.
package api

import (
	"net/http"
	"sort"
	"time"

	"mikcloud/hotspot-api/internal/model"
)

// bellResponse — contrat de GET /api/bell (la cloche de la topbar).
type bellResponse struct {
	Items []model.Activity `json:"items"`
	// SeenAt — instant du dernier acquit de CET utilisateur (vide = première
	// visite, tout considéré lu — comportement historique).
	SeenAt string `json:"seenAt"`
	// Unread — entrées visibles plus récentes que SeenAt (0 si SeenAt vide).
	Unread int `json:"unread"`
}

// handleBellList — GET /api/bell?limit=20 (rang 2+, miroir de /api/activity).
func (a *API) handleBellList(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	limit := queryInt(r, "limit", 20, 1, 50)
	rank := 0
	if c := claimsFrom(r); c != nil {
		rank = roleRank(c.Role)
	}
	a.store.Lock()
	db := a.store.Data()
	acts := []model.Activity{}
	unread := 0
	seenAt := ""
	// Le read-state est CELUI DU PORTEUR (claims.Sub) — pas du compte : deux
	// membres de la même équipe ont chacun leur boîte. Le super-admin en
	// session support garde le sien (il consulte, il n'éponge pas celui du
	// gérant).
	for i := range db.Users {
		if db.Users[i].ID == claimsFrom(r).Sub {
			seenAt = db.Users[i].ActivitySeenAt
			break
		}
	}
	for _, act := range db.Activity {
		if act.AccountID != acc || activityTypeMinRank(act.Type) > rank {
			continue
		}
		acts = append(acts, act)
		if seenAt != "" && act.At > seenAt {
			unread++ // compté sur TOUT le journal visible, pas seulement la page
		}
	}
	// N°152 — annonces de la plateforme : les annonces ACTIVES pour le compte
	// (audience × expiration) entrent dans la boîte comme items synthétiques
	// type « announcement » — pas de ligne d'activité par compte (une annonce
	// est globale), mais elle compte dans le badge via le même read-state :
	// créée après le dernier acquit = non lue, jusqu'à ouverture de la cloche.
	// Le compte principal (plateforme) n'est pas un client : rien.
	if acc != model.AccountMainID {
		usage := model.AccountUsageHotspot
		for _, a2 := range db.Accounts {
			if a2.ID == acc {
				usage = normalizeAccountUsage(a2.Usage)
				break
			}
		}
		now := model.NowISO()
		for _, ann := range db.Announcements {
			if !ann.Active(usage, now) {
				continue
			}
			msg := ann.Title
			if ann.Body != "" {
				msg = ann.Title + " — " + ann.Body
			}
			// N°165 — date EFFECTIVE : PublishAt si programmée (sa cloche
			// « sonne » à sa publication, pas à sa rédaction), CreatedAt sinon.
			eff := ann.EffectiveAt()
			acts = append(acts, model.Activity{
				ID: ann.ID, Type: "announcement", Message: msg, At: eff,
				Level: ann.Level, Title: ann.Title, Body: ann.Body,
			})
			if seenAt != "" && eff > seenAt {
				unread++
			}
		}
	}
	a.store.Unlock()
	sort.Slice(acts, func(i, j int) bool { return acts[i].At > acts[j].At })
	if len(acts) > limit {
		acts = acts[:limit]
	}
	writeJSON(w, http.StatusOK, bellResponse{Items: acts, SeenAt: seenAt, Unread: unread})
}

// handleBellSeen — POST /api/bell/seen {"at"?: RFC3339} (rang 2+).
// Corps absent/vide : acquit à maintenant. Champ « at » : la migration de
// l'ancien localStorage du navigateur (l'utilisateur garde son avancement),
// borné à maintenant +1 min (un horodatage falsifié du futur n'enterre pas
// les notifications à venir), monotone (un acquit ancien qui arrive en retard
// ne rouvre pas les non-lus).
func (a *API) handleBellSeen(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r)
	if claims == nil {
		writeErr(w, http.StatusUnauthorized, "Token invalide ou expiré")
		return
	}
	var req struct {
		At string `json:"at"`
	}
	if r.Body != nil {
		_ = decodeBody(r, &req) // corps vide = acquit à maintenant
	}
	at := time.Now().UTC()
	if req.At != "" {
		parsed, err := time.Parse(time.RFC3339, req.At)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "Horodatage d'acquit invalide (RFC 3339 attendu)")
			return
		}
		at = parsed
		if at.After(time.Now().UTC().Add(time.Minute)) {
			at = time.Now().UTC() // borne anti-futur falsifié
		}
	}
	stamp := at.Format(time.RFC3339)
	a.store.Lock()
	db := a.store.Data()
	updated := false
	for i := range db.Users {
		if db.Users[i].ID == claims.Sub {
			if stamp > db.Users[i].ActivitySeenAt { // monotone : jamais en arrière
				db.Users[i].ActivitySeenAt = stamp
				updated = true
			}
			stamp = db.Users[i].ActivitySeenAt
			break
		}
	}
	if updated {
		a.store.Save()
	}
	a.store.Unlock()
	writeJSON(w, http.StatusOK, map[string]string{"seenAt": stamp})
}
