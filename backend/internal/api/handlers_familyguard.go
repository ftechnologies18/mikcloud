package api

// N°82 — FamilyGuard : couvre-feu internet du WiFi public.
//
// Un seul point d'entrée console : PUT /api/routers/{id}/familyguard
// {enabled, start, end, days}. Le spec canonique est persisté sur le
// routeur ; la signature ne bouge PAS : ensureFamilyGuardLocked voit la
// différence au check-in suivant et file la commande familyguard — servie
// ≤ 45 s (console ouverte = attention N°75) ou ≤ 180 s (veille). Le retour
// « ok » VÉRIFIÉ du routeur (règles marquées == 1 × hotspots rapportés en
// couvre-feu, 0 sinon) pose la signature : un échec est retenté
// automatiquement, sans intervention.
//
// Phase 3 de la roadmap sécurité : le module est livré COMPLET et
// testable, mais son lancement COMMERCIAL (vendre l'add-on) est volontairement
// différé au premier revenu — la carte console l'affiche comme module en
// phase de test, gratuit pour les pilotes.
//
// Technique (MVP validé) : l'ÉTAT désiré (couvre-feu en cours ou non) est
// calculé PAR LE CLOUD à chaque check-in, en UTC (== heure d'Abidjan GMT,
// sans heure d'été) — l'horloge routeur n'est jamais consultée. Contrepartie
// assumée : la bascule s'applique au check-in suivant. 0 Mo de RAM routeur
// (règle filter sans état), 0 FCFA d'infrastructure, compatible MIPS 128 Mo.

import (
	"encoding/json"
	"net/http"

	"mikcloud/hotspot-api/internal/model"
)

// handleRouterSetFamilyGuard — PUT /api/routers/{id}/familyguard
// Programme (ou désactive) le couvre-feu internet du site. Corps attendu :
// {"enabled":bool,"start":"HH:MM","end":"HH:MM","days":"1111111"} — 400 si
// la fenêtre est invalide (heures mal formées, début == fin, jours != 7
// caractères 0/1, aucun jour actif). Trace l'acteur dans le journal
// d'activité.
func (a *API) handleRouterSetFamilyGuard(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	id := r.PathValue("id")

	var body struct {
		Enabled bool   `json:"enabled"`
		Start   string `json:"start"`
		End     string `json:"end"`
		Days    string `json:"days"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<14)).Decode(&body); err != nil {
		writeErrCode(w, http.StatusBadRequest, "invalid_body",
			"Corps invalide — attendu : {enabled, start, end, days}", nil)
		return
	}
	cfg := model.FamilyGuardConfig{Enabled: body.Enabled, Start: body.Start, End: body.End, Days: body.Days}
	if !model.ValidFamilyGuardConfig(cfg) {
		writeErrCode(w, http.StatusBadRequest, "invalid_window",
			"Fenêtre invalide — heures « HH:MM » (début ≠ fin), jours « 1111111 » (lundi→dimanche, au moins un jour)", nil)
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
			"Le couvre-feu internet ne s'applique qu'aux routeurs en mode agent", nil)
		return
	}
	// P3 — compte expiré : écritures métier refusées.
	if a.subscriptionGuardStateLocked(cur.AccountID).Status == "expired" {
		a.store.Unlock()
		writeErrCode(w, http.StatusPaymentRequired, "subscription_expired",
			"Abonnement expiré — modification impossible", nil)
		return
	}

	spec := cfg.SpecString()
	prev, hadPrev := model.ParseFamilyGuardSpec(cur.FamilyGuardSpec)
	if !hadPrev || prev != cfg {
		cur.FamilyGuardSpec = spec // sig inchangée → ensure re-file au check-in
		state := "désactivé"
		if cfg.Enabled {
			state = "activé (" + familyGuardSpecSummary(cfg) + ")"
		}
		a.logActivityBy(r, db, acc, "router", "Couvre-feu internet de «"+cur.Name+"» : "+state)
	}
	a.store.Save()
	name := cur.Name
	a.store.Unlock()

	state := "Couvre-feu de «" + name + "» : "
	if cfg.Enabled {
		state += "activé — " + familyGuardSpecSummary(cfg)
	} else {
		state += "désactivé (fenêtre conservée : " + familyGuardSpecSummary(cfg) + ")"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"enabled": cfg.Enabled,
		"message": state + " — appliqué au prochain check-in du routeur (≤ 45 s)",
	})
}
