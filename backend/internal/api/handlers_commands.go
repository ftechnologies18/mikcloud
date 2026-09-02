// Package api — ping routeur + statut des commandes (F8, audit Mikhmon) :
// ping immédiat en simulé, commande agent (≤ 45 s) sinon ; la route générique
// GET /api/commands/{id} sert le résultat normalisé de toute commande
// (CONTRACT-V2 §0).
package api

import (
	"math/rand"
	"mikcloud/hotspot-api/internal/model"
	"net"
	"net/http"
	"regexp"
	"strings"
)

// hostnamePattern — validation d'une cible ping (hostname, contrat F8).
var hostnamePattern = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?(\.[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?)*$`)

// ---------------------------------------------------------------------------
// F8 — Ping + statut de commande
// ---------------------------------------------------------------------------

// validPingTarget — IP littérale ou nom d'hôte ≤ 253 caractères.
func validPingTarget(t string) bool {
	if t == "" || len(t) > 253 {
		return false
	}
	if net.ParseIP(t) != nil {
		return true
	}
	return hostnamePattern.MatchString(t)
}

// handleRouterPing — POST /api/routers/{id}/ping {target} :
//   - simulated : réponse immédiate (8-60 ms ; ~10 % → 3/4 reçus, 25 % de perte) ;
//   - agent     : commande ping en file → {queued:true, commandId} ;
//   - real      : 400.
func (a *API) handleRouterPing(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	id := r.PathValue("id")
	var req struct {
		Target string `json:"target"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	target := strings.TrimSpace(req.Target)
	if !validPingTarget(target) {
		writeErr(w, http.StatusBadRequest, "Cible invalide (adresse IP ou nom d'hôte de 253 caractères max)")
		return
	}

	a.store.Lock()
	db := a.store.Data()
	rr := findRouterScoped(db, id, acc)
	if rr == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Routeur introuvable")
		return
	}
	if rr.Mode == "real" {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, realModeUnsupported)
		return
	}
	if rr.Mode == "agent" {
		cmd := queueCommandLocked(db, acc, id, model.CmdPing, map[string]any{"target": target})
		a.store.Save()
		cmdID := cmd.ID
		a.store.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{
			"queued": true, "commandId": cmdID,
			"message": "Ping en attente du prochain check-in du routeur (≤ 45 s)",
		})
		return
	}
	a.store.Unlock()

	// simulated — statistiques immédiates.
	received, lossPct := 4, 0
	if rand.Float64() < 0.10 {
		received, lossPct = 3, 25
	}
	minMs := 8 + rand.Intn(18) // 8-25 ms
	maxMs := minMs + 12 + rand.Intn(18)
	if maxMs > 60 {
		maxMs = 60
	}
	avgMs := (minMs + maxMs) / 2
	writeJSON(w, http.StatusOK, map[string]any{
		"queued": false, "ok": true, "target": target,
		"sent": 4, "received": received, "lossPct": lossPct,
		"minMs": minMs, "avgMs": avgMs, "maxMs": maxMs,
	})
}

// handleCommandStatus — GET /api/commands/{id} → {id, kind, status, result}
// (scopé compte ; le front poll toutes les 2 s pour un ping agent).
func (a *API) handleCommandStatus(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	id := r.PathValue("id")

	a.store.Lock()
	var out map[string]any
	for i := range a.store.Data().Commands {
		c := &a.store.Data().Commands[i]
		if c.ID != id || c.AccountID != acc {
			continue
		}
		out = map[string]any{
			"id": c.ID, "kind": c.Kind, "status": c.Status,
			"createdAt": c.CreatedAt, "sentAt": c.SentAt, "doneAt": c.DoneAt,
		}
		if c.Result != nil {
			// Copie défensive (rendu JSON hors verrou).
			res := make(map[string]any, len(c.Result))
			for k, v := range c.Result {
				res[k] = v
			}
			out["result"] = res
		} else {
			out["result"] = nil
		}
		break
	}
	a.store.Unlock()
	if out == nil {
		writeErr(w, http.StatusNotFound, "Commande introuvable")
		return
	}
	writeJSON(w, http.StatusOK, out)
}
