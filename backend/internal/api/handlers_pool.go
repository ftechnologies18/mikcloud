// N°97 — Docteur du pool d'adresses IP du hotspot (console).
//
//	POST /api/routers/{id}/pool-doctor   { "extend": true|false }
//
// Corrige l'épuisement « cannot assign ip address - no more free addresses
// from pool » aux heures de pointe (constat gérant 13/09/2026) :
//   - simulated : applique un diagnostic synthétique honnête (capacité 254
//     par défaut, hôtes = sessions actives + zombies simulés) ;
//   - agent     : enfile la commande pool_doctor (recyclage TOUJOURS inclus :
//     login-timeout 5m / idle-timeout 10m / keepalive-timeout 2m /
//     address-per-mac=1 — libère les IP des zombies) ; extend=true ajoute
//     en plus le range dédié 10.77.0.10-10.77.7.254 (~2 037 IP) au pool du profil ;
//   - real      : 400 (matrice §0 : les outils du mode API directe ne sont
//     pas supportés).
package api

import (
	"net/http"

	"mikcloud/hotspot-api/internal/model"
)

// handleRouterPoolDoctor — POST /api/routers/{id}/pool-doctor.
func (a *API) handleRouterPoolDoctor(w http.ResponseWriter, r *http.Request) {
	// Écriture routeur (timeouts, pool) : compte expiré refusé.
	if !a.guardAccountWrite(w, r) {
		return
	}
	acc := accountScope(r)
	id := r.PathValue("id")
	extend := parseBodyBool(r, "extend")

	a.store.Lock()
	db := a.store.Data()
	cur := findRouterScoped(db, id, acc)
	if cur == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Routeur introuvable")
		return
	}
	if cur.Mode == "real" {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, realModeUnsupported)
		return
	}
	name := cur.Name

	if cur.Mode == "agent" {
		// Recyclage toujours inclus : geste sans risque (aucun subnet
		// touché), l'extension est l'opt-in explicite du gérant.
		cmd := queueCommandLocked(db, cur.AccountID, id, model.CmdPoolDoctor,
			map[string]any{"recycle": true, "extend": extend})
		a.logActivityBy(r, db, acc, "router", "Docteur pool IP lancé sur «"+name+"» (recyclage"+
			map[bool]string{true: " + extension", false: ""}[extend]+")")
		a.store.Save()
		cmdID := cmd.ID
		a.store.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{
			"queued":    true,
			"commandId": cmdID,
			"message":   "Docteur envoyé — application au prochain check-in du routeur (≤ 45 s)",
		})
		return
	}

	// simulated — diagnostic synthétique : capacité /24 (254) et hôtes =
	// sessions actives + 40 % de zombies (moteur de démo cohérent avec la
	// télémétrie simulée). Les champs posés sont les MÊMES que le mode agent
	// (PoolCap/PoolHosts/PoolRanges/PoolDoctorAt) : la console affiche et le
	// moniteur alerte identiquement.
	cur.PoolCap = 254
	cur.PoolHosts = cur.ActiveSessions + cur.ActiveSessions*2/5
	cur.PoolRanges = "10.5.50.2-10.5.50.254"
	cur.PoolDoctorAt = model.NowISO()
	poolCap, poolHosts := cur.PoolCap, cur.PoolHosts
	usagePct := poolUsagePct(cur)
	a.logActivityBy(r, db, acc, "router", "Docteur pool IP appliqué sur «"+name+"» (simulation)")
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":        true,
		"poolCap":   poolCap,
		"poolHosts": poolHosts,
		"usagePct":  usagePct,
		"message":   "Diagnostic pool appliqué (routeur simulé)",
	})
}

// parseBodyBool — lit un booléen optionnel du corps JSON (absent → false).
// Corps vide ou illisible → false (l'extension est un opt-in : son absence
// ne doit jamais déclencher un geste de configuration).
func parseBodyBool(r *http.Request, key string) bool {
	var body map[string]any
	if r.Body != nil {
		if err := decodeBody(r, &body); err != nil || body == nil {
			return false
		}
		v, ok := body[key].(bool)
		return ok && v
	}
	return false
}
