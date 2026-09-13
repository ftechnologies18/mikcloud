package api

// N°80 — SafeWiFi : protection DNS du WiFi public (filtrage par redirection).
//
// Un seul point d'entrée console : PUT /api/routers/{id}/safewifi
// {level: off|threats|family}. Le niveau est persisté sur le routeur ; la
// signature ne bouge PAS : ensureSafeWifiLocked voit la différence au
// check-in suivant et file la commande safewifi — servie ≤ 45 s (console
// ouverte = attention N°75) ou ≤ 180 s (veille). Le retour « ok » VÉRIFIÉ du
// routeur (compte de règles marquées rapporté) pose la signature : un échec
// est retenté automatiquement, sans intervention.
//
// Technique (MVP validé) : deux règles NAT dst-nat réécrivent le port 53
// vers le résolveur filtrant du niveau — 0 Mo de RAM sur le routeur ( MIPS
// 128 Mo compris), 0 FCFA d'infrastructure (résolveurs publics anycast),
// zéro modification de /ip dns : le DNS propre du routeur (check-in agent)
// n'est jamais exposé à la disponibilité du résolveur filtrant.

import (
	"encoding/json"
	"net/http"

	"mikcloud/hotspot-api/internal/model"
)

// handleRouterSetSafeWifi — PUT /api/routers/{id}/safewifi
// Change le niveau de protection DNS du WiFi public du site. Corps attendu :
// {"level":"off"|"threats"|"family"} — 400 sinon. Trace l'acteur dans le
// journal d'activité.
func (a *API) handleRouterSetSafeWifi(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	id := r.PathValue("id")

	var body struct {
		Level string `json:"level"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<14)).Decode(&body); err != nil ||
		!model.ValidSafeWifiLevel(body.Level) {
		writeErrCode(w, http.StatusBadRequest, "invalid_level",
			"Niveau invalide — attendu : off, threats ou family", nil)
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
			"La protection WiFi public ne s'applique qu'aux routeurs en mode agent", nil)
		return
	}
	// P3 — compte expiré : écritures métier refusées.
	if a.subscriptionGuardStateLocked(cur.AccountID).Status == "expired" {
		a.store.Unlock()
		writeErrCode(w, http.StatusPaymentRequired, "subscription_expired",
			"Abonnement expiré — modification impossible", nil)
		return
	}

	prev := cur.SafeWifiLevelEffective()
	if body.Level != prev {
		cur.SafeWifiLevel = body.Level // sig inchangée → ensure re-file au check-in
		a.logActivityBy(r, db, acc, "router", "Protection WiFi public de «"+cur.Name+"» : "+
			safeWifiLevelLabel(prev)+" → "+safeWifiLevelLabel(body.Level))
	}
	a.store.Save()
	name := cur.Name
	a.store.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":    true,
		"level": body.Level,
		"message": "Protection «" + name + "» : " + safeWifiLevelLabel(body.Level) +
			" — appliquée au prochain check-in du routeur (≤ 45 s)",
	})
}
