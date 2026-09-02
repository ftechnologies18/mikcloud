// Package api — garde d'écriture liée à l'abonnement (P3) : un compte dont
// l'abonnement est expiré repasse en LECTURE SEULE (402 sur les écritures
// métier) et le plan Essentiel est plafonné aux routeurs couverts par la
// période payée. Exemption : les administrateurs plateforme — le guard
// protège le modèle économique contre les CLIENTS, pas contre l'opérateur
// du SaaS.
package api

import (
	"fmt"
	"net/http"
	"time"
)

// ---------------------------------------------------------------------------
// P3 — enforcement serveur des abonnements (guard d'écriture)
// ---------------------------------------------------------------------------

// subscriptionGuardView — état d'abonnement vu par le guard (hors verrou).
type subscriptionGuardView struct {
	Status      string // none | active | expired (effectif)
	PlanID      string
	RouterSlots int
	PeriodEnd   string
}

// subscriptionGuardStateLocked — lit l'état d'abonnement du compte, à appeler
// LE VERROU PRIS (compteurs exacts pour le plafond de routeurs).
func (a *API) subscriptionGuardStateLocked(acc string) subscriptionGuardView {
	settings := ensureSettings(a.store.Data(), acc)
	return subscriptionGuardView{
		Status:      subscriptionStatus(settings.Subscription, time.Now().UTC()),
		PlanID:      settings.Subscription.PlanID,
		RouterSlots: settings.Subscription.RouterSlots,
		PeriodEnd:   settings.Subscription.PeriodEnd,
	}
}

// subscriptionGuardState — lit l'état d'abonnement du compte (sous verrou).
func (a *API) subscriptionGuardState(acc string) subscriptionGuardView {
	a.store.Lock()
	defer a.store.Unlock()
	return a.subscriptionGuardStateLocked(acc)
}

// guardAccountWrite — bloque les écritures métier d'un compte expiré (P3).
// Exemption : les administrateurs plateforme (session support ou console) —
// le guard protège le modèle économique contre les CLIENTS, pas contre
// l'opérateur du SaaS. Renvoie true si l'écriture est autorisée, sinon
// répond 402 et renvoie false.
//
// En pratique : un compte expiré repasse en LECTURE SEULE — consultations,
// exports, suppressions et actions de maintenance (activer/désactiver un
// utilisateur, déconnecter une session, réglages) restent possibles ; la
// création et la modification de ressources métier (routeurs, utilisateurs
// hotspot, vouchers, profils, revendeurs, équipe, import agent) est refusée
// avec le code machine « subscription_expired ».
func (a *API) guardAccountWrite(w http.ResponseWriter, r *http.Request) bool {
	if isPlatformAdmin(r) {
		return true
	}
	view := a.subscriptionGuardState(accountScope(r))
	if view.Status != "expired" {
		return true
	}
	writeErrCode(w, http.StatusPaymentRequired, "subscription_expired",
		"Abonnement expiré — renouvelez auprès de MikCloud pour reprendre les modifications",
		map[string]any{"periodEnd": view.PeriodEnd})
	return false
}

// guardAccountRouterLimit — plafond de routeurs du plan Essentiel (P3) :
// la période couvre RouterSlots routeurs ; au-delà, la création est refusée
// (402, code « plan_router_limit »). À appeler APRÈS guardAccountWrite,
// sous verrou (routerCount = routeurs déjà enregistrés du compte).
func guardAccountRouterLimit(w http.ResponseWriter, view subscriptionGuardView, routerCount int) bool {
	// Essai : 1 routeur max. Essentiel : quota de routeurs couverts par la période.
	if (view.PlanID != "essentiel" && view.PlanID != "essai") || view.RouterSlots <= 0 || routerCount < view.RouterSlots {
		return true
	}
	writeErrCode(w, http.StatusPaymentRequired, "plan_router_limit",
		fmt.Sprintf("Votre formule couvre %d routeur(s) — passez au plan Essentiel (plus de routeurs) ou Illimité pour en ajouter", view.RouterSlots),
		map[string]any{"limit": view.RouterSlots, "current": routerCount, "plan": view.PlanID})
	return false
}
