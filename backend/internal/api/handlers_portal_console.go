// Package api — endpoints console pour le portail captif (N°35-d).
//
// Deux endpoints pour la vue « Portail » de la console MikCloud :
//
//	POST /api/routers/{id}/redeploy-portal — force le re-déploiement du portail
//	  captif sur le routeur. Vide router.HotspotFilesSig → ensureHotspotFilesLocked
//	  (appelé au prochain check-in agent, ≤ 45 s) verra la sig vide ≠ sig par
//	  défaut → re-file automatiquement CmdHotspotFiles. Aucune commande filée
//	  manuellement ici : c'est le mécanisme N°35-a qui s'en charge. L'endpoint
//	  trace l'acteur (logActivityBy) pour le journal d'audit.
//
//	POST /api/routers/{id}/repair-walled-garden — force le re-file du
//	  walled-garden d'inscription publique sur le routeur. Vide
//	  router.WalledGardenSig ET router.WalledGardenAppliedAt →
//	  ensureWalledGardenLocked (appelé au prochain check-in agent, ≤ 45 s)
//	  re-file automatiquement CmdWalledGarden. Utile après une suppression
//	  manuelle des règles sur le routeur (entre deux cycles d'auto-réparation
//	  N°49). Trace l'acteur (logActivityBy) pour le journal d'audit.
//
//	GET /api/routers/{id}/portal-preview — retourne le HTML personnalisé de
//	  login.html pour le routeur, à injecter dans une iframe srcDoc côté
//	  console. Réutilise buildPortalConfig (résout tenant + 1er WifiSite actif
//	  lié + 1er JoinLink actif + offres) + hotpage.Personalize. Le HTML est
//	  servit avec Content-Type text/html + Cache-Control no-store. Le fetch
//	  live (login.html côté routeur) n'est pas actif en aperçu (origine
//	  backend ≠ origine routeur) — l'aperçu montre le fallback inliné, ce
//	  qui est suffisant pour valider le branding + la structure de la page.
//
// Pré-requis : le routeur doit être en mode "agent" (les modes "simulated" et
// "real" ne déploient pas de portail — pas de check-in agent). 400 sinon.
package api

import (
	"net/http"

	"mikcloud/hotspot-api/internal/hotpage"
	"mikcloud/hotspot-api/internal/model"
)

// handleRouterRedeployPortal — POST /api/routers/{id}/redeploy-portal
// Force le re-déploiement du portail captif sur le routeur agent. Vide la
// signature HotspotFilesSig : au prochain check-in (≤ 45 s), ensureHotspotFilesLocked
// re-file CmdHotspotFiles automatiquement. Trace l'acteur dans le journal.
func (a *API) handleRouterRedeployPortal(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	id := r.PathValue("id")
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
			"Le déploiement du portail ne s'applique qu'aux routeurs en mode agent", nil)
		return
	}
	// P3 — compte expiré : écritures métier refusées.
	if a.subscriptionGuardStateLocked(cur.AccountID).Status == "expired" {
		a.store.Unlock()
		writeErrCode(w, http.StatusPaymentRequired, "subscription_expired",
			"Abonnement expiré — re-déploiement impossible", nil)
		return
	}
	// Vide la sig → ensureHotspotFilesLocked re-file au prochain check-in.
	cur.HotspotFilesSig = ""
	a.logActivityBy(r, db, acc, "router", "Re-déploiement du portail demandé pour «"+cur.Name+"»")
	a.store.Save()
	name := cur.Name
	a.store.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"message": "Re-déploiement programmé pour «" + name + " » — le routeur rechargera le portail à son prochain check-in (≤ 45 s)",
	})
}

// handleRouterRepairWalledGarden — POST /api/routers/{id}/repair-walled-garden
// N°49 — force le re-file du walled-garden d'inscription publique sur le
// routeur agent : sig + horodatage vidés → ensureWalledGardenLocked re-file
// CmdWalledGarden au prochain check-in (≤ 45 s). Le bloc étant idempotent
// (seules les règles marquées mikcloud-wg sont remplacées), la réparation est
// sans risque pour les règles personnelles du gérant.
func (a *API) handleRouterRepairWalledGarden(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	id := r.PathValue("id")
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
			"La réparation du walled-garden ne s'applique qu'aux routeurs en mode agent", nil)
		return
	}
	// P3 — compte expiré : écritures métier refusées.
	if a.subscriptionGuardStateLocked(cur.AccountID).Status == "expired" {
		a.store.Unlock()
		writeErrCode(w, http.StatusPaymentRequired, "subscription_expired",
			"Abonnement expiré — réparation impossible", nil)
		return
	}
	// Sig + horodatage vidés → ensureWalledGardenLocked re-file au prochain check-in.
	cur.WalledGardenSig = ""
	cur.WalledGardenAppliedAt = ""
	a.logActivityBy(r, db, acc, "router", "Réparation du walled-garden demandée pour «"+cur.Name+"»")
	a.store.Save()
	name := cur.Name
	a.store.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"message": "Réparation programmée pour «" + name + " » — le walled-garden sera réappliqué à son prochain check-in (≤ 45 s)",
	})
}

// handleRouterPortalPreview — GET /api/routers/{id}/portal-preview
// Retourne le HTML personnalisé de login.html pour ce routeur (aperçu console).
// Le HTML est servi avec text/html + no-store. Le fetch live n'est pas actif
// (origine backend ≠ origine routeur) — l'aperçu montre le fallback inliné.
func (a *API) handleRouterPortalPreview(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	id := r.PathValue("id")
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
			"L'aperçu du portail ne s'applique qu'aux routeurs en mode agent", nil)
		return
	}
	cfg := buildPortalConfig(db, cur, r)
	a.store.Unlock()

	body, ok := hotpage.File("login.html")
	if !ok {
		writeErr(w, http.StatusInternalServerError, "Template login.html introuvable")
		return
	}
	personalized := hotpage.Personalize(string(body), cfg)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// CSP permissive pour l'iframe srcDoc : le HTML vient du cloud (trust), les
	// scripts internes doivent pouvoir s'exécuter. Le sandbox de l'iframe
	// (allow-scripts) isole l'aperçu du reste de la console.
	w.Header().Set("Content-Security-Policy", "default-src 'self' 'unsafe-inline' 'unsafe-eval' data: blob:; frame-ancestors 'self'")
	_, _ = w.Write([]byte(personalized))
}

// findRouterScoped — résout un routeur par ID dans le compte courant (sécurité
// multi-tenant). Retourne nil si introuvable ou appartient à un autre compte.
// Rappel : cette fonction existe déjà dans helpers.go (cf. findProfileScoped).
// On l'utilise ici pour la cohérence avec handleRouterRefresh.
var _ = model.Router{} // garde l'import model vivant (utilisé indirectement via findRouterScoped)
