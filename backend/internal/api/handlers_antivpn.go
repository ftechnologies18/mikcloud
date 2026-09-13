package api

// N°88 — AntiVPN : bloque-VPN du WiFi public.
//
// Un seul point d'entrée console : PUT /api/routers/{id}/antivpn
// {level: off|on}. Le niveau est persisté sur le routeur ; la signature
// ne bouge PAS : ensureAntiVpnLocked voit la différence au check-in
// suivant et file la commande antivpn — servie ≤ 45 s (console ouverte =
// attention N°75) ou ≤ 180 s (veille). Le retour « ok » VÉRIFIÉ du
// routeur (compte de règles marquées == 4 × hotspots rapportés) pose la
// signature : un échec est retenté automatiquement, sans intervention.
//
// Technique (MVP validé) : QUATRE règles FILTER par serveur hotspot,
// ciblées sur l'INTERFACE du hotspot (lue sur le routeur, pattern N°81) :
// GRE (tunnels), ESP (IPsec) et les ports UDP/TCP des tunnels standards
// (IKE, L2TP, PPTP, OpenVPN, WireGuard, WARP, Tor) — IPv6 coupé de même
// (best-effort, pattern N°85). Le DNS (53 — SafeWiFi N°80 reste maître),
// le NTP (123) et l'UDP 443 (appels WhatsApp, critiques en Côte d'Ivoire)
// ne sont JAMAIS touchés : 0 Mo de RAM sur le routeur, 0 FCFA
// d'infrastructure, et une limite résiduelle écrite noir sur blanc dans
// la footnote (un tunnel camouflé en HTTPS pur peut encore passer — pas
// de DPI sur un routeur 128 Mo).

import (
	"encoding/json"
	"net/http"

	"mikcloud/hotspot-api/internal/model"
)

// handleRouterSetAntiVpn — PUT /api/routers/{id}/antivpn
// Active ou désactive le bloque-VPN du WiFi public du site. Corps
// attendu : {"level":"off"|"on"} — 400 sinon. Trace l'acteur dans le
// journal d'activité.
func (a *API) handleRouterSetAntiVpn(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	id := r.PathValue("id")

	var body struct {
		Level string `json:"level"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<14)).Decode(&body); err != nil ||
		!model.ValidAntiVpnLevel(body.Level) {
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
			"Le bloque-VPN ne s'applique qu'aux routeurs en mode agent", nil)
		return
	}
	// P3 — compte expiré : écritures métier refusées.
	if a.subscriptionGuardStateLocked(cur.AccountID).Status == "expired" {
		a.store.Unlock()
		writeErrCode(w, http.StatusPaymentRequired, "subscription_expired",
			"Abonnement expiré — modification impossible", nil)
		return
	}

	prev := cur.AntiVpnLevelEffective()
	if body.Level != prev {
		cur.AntiVpnLevel = body.Level // sig inchangée → ensure re-file au check-in
		a.logActivityBy(r, db, acc, "router", "Bloque-VPN de «"+cur.Name+"» : "+
			antiVpnLevelLabel(prev)+" → "+antiVpnLevelLabel(body.Level))
	}
	a.store.Save()
	name := cur.Name
	a.store.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":    true,
		"level": body.Level,
		"message": "Bloque-VPN «" + name + "» : " + antiVpnLevelLabel(body.Level) +
			" — appliqué au prochain check-in du routeur (≤ 45 s)",
	})
}
