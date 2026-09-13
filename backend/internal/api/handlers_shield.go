package api

// N°81 — Shield : bouclier réseau du WiFi public.
//
// Un seul point d'entrée console : PUT /api/routers/{id}/shield
// {level: off|on}. Le niveau est persisté sur le routeur ; la signature
// ne bouge PAS : ensureShieldLocked voit la différence au check-in
// suivant et file la commande shield — servie ≤ 45 s (console ouverte =
// attention N°75) ou ≤ 180 s (veille). Le retour « ok » VÉRIFIÉ du
// routeur (compte de règles marquées == 5 × hotspots rapportés) pose
// la signature : un échec est retenté automatiquement, sans
// intervention.
//
// Technique (MVP validé) : règles FILTER ciblées sur l'INTERFACE du
// hotspot, lue sur le routeur au moment de l'exécution — l'input rend
// l'administration du routeur (winbox, ssh, telnet, api) inaccessible
// depuis le WiFi public, le forward bloque SMB/NetBIOS et les
// connexions invalides des appareils clients. Le réseau du gérant
// (LAN) et le trafic propre du routeur (chain=output) ne sont jamais
// touchés : 0 Mo de RAM sur le routeur, 0 FCFA d'infrastructure.

import (
	"encoding/json"
	"net/http"

	"mikcloud/hotspot-api/internal/model"
)

// handleRouterSetShield — PUT /api/routers/{id}/shield
// Active ou désactive le bouclier réseau du WiFi public du site. Corps
// attendu : {"level":"off"|"on"} — 400 sinon. Trace l'acteur dans le
// journal d'activité.
func (a *API) handleRouterSetShield(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	id := r.PathValue("id")

	var body struct {
		Level string `json:"level"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<14)).Decode(&body); err != nil ||
		!model.ValidShieldLevel(body.Level) {
		writeErrCode(w, http.StatusBadRequest, "invalid_level",
			"Niveau invalide — attendu : off ou on", nil)
		return
	}

	a.store.Lock()
	db := a.store.Data()
	cur := findRouterScoped(db, id, acc)
	if cur == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Routeur introuvable")
		return
	}
	if cur.Mode != "agent" {
		a.store.Unlock()
		writeErrCode(w, http.StatusBadRequest, "not_agent",
			"Le bouclier réseau ne s'applique qu'aux routeurs en mode agent", nil)
		return
	}
	// P3 — compte expiré : écritures métier refusées.
	if a.subscriptionGuardStateLocked(cur.AccountID).Status == "expired" {
		a.store.Unlock()
		writeErrCode(w, http.StatusPaymentRequired, "subscription_expired",
			"Abonnement expiré — modification impossible", nil)
		return
	}

	prev := cur.ShieldLevelEffective()
	if body.Level != prev {
		cur.ShieldLevel = body.Level // sig inchangée → ensure re-file au check-in
		a.logActivityBy(r, db, acc, "router", "Bouclier réseau de «"+cur.Name+"» : "+
			shieldLevelLabel(prev)+" → "+shieldLevelLabel(body.Level))
	}
	a.store.Save()
	name := cur.Name
	a.store.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":    true,
		"level": body.Level,
		"message": "Bouclier «" + name + "» : " + shieldLevelLabel(body.Level) +
			" — appliqué au prochain check-in du routeur (≤ 45 s)",
	})
}
