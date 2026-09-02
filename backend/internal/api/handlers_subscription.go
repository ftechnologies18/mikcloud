// handlers_subscription.go — abonnement SaaS, formules et lien marchand Wave.

package api

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"mikcloud/hotspot-api/internal/model"
)

// handleWaveLink — deep link Wave Côte d'Ivoire par montant.
// Wave CI n'a pas d'API publique : le marchand configure son lien
// pay.wave.com dans Settings (Tenant.WaveLink) et MikCloud compose
// l'URL de paiement avec le montant demandé (ex. pour un lot de vouchers).
func (a *API) handleWaveLink(w http.ResponseWriter, r *http.Request) {
	amount, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("amount")))
	if err != nil || amount <= 0 {
		writeErr(w, http.StatusBadRequest, "Montant invalide")
		return
	}
	a.store.Lock()
	acc := accountScope(r)
	waveLink := ""
	if s, ok := a.store.Data().SettingsByAccount[acc]; ok {
		waveLink = s.Tenant.WaveLink
	}
	a.store.Unlock()
	if strings.TrimSpace(waveLink) == "" {
		writeErr(w, http.StatusConflict, "Lien marchand Wave non configuré (Settings → Lien Wave)")
		return
	}
	ref := strings.TrimSpace(r.URL.Query().Get("ref"))
	link := strings.TrimRight(waveLink, "/") + "/amount/" + strconv.Itoa(amount) + "/"
	resp := map[string]any{"amount": amount, "link": link, "currency": "FCFA"}
	if ref != "" {
		resp["ref"] = ref
	}
	writeJSON(w, http.StatusOK, resp)
}

// subscriptionView — état complet de l'abonnement renvoyé à la console.
// planPricingView — tarification d'une formule pour le compte (assiette
// routeurs du moment) : montants par moyen de paiement — répercussion des
// frais GeniusPay validée (handlers_pricing.go).
type planPricingView struct {
	PlanID string `json:"planId"`
	planPricing
}

type subscriptionView struct {
	Subscription      model.Subscription `json:"subscription"`
	Status            string             `json:"status"` // none | active | expired (effectif)
	RouterCount       int                `json:"routerCount"`
	CurrentAmountFcfa int                `json:"currentAmountFcfa"` // montant de la période en cours
	Plans             []model.SaasPlan   `json:"plans"`
	WaveConfigured    bool               `json:"waveConfigured"`
	// Pricing — montants par moyen pour chaque formule (remise Wave / prix
	// de liste carte), à l'assiette routeurs du compte.
	Pricing []planPricingView `json:"pricing,omitempty"`
}

// subscriptionStatus — statut effectif : une période échue passe en « expired »
// (calculé à la lecture, sans muter le stockage — le renouvellement réactive).
func subscriptionStatus(sub model.Subscription, now time.Time) string {
	if sub.PlanID == "" {
		return "none"
	}
	if sub.PeriodEnd != "" {
		if end, err := time.Parse(time.RFC3339, sub.PeriodEnd); err == nil {
			if now.After(end.Add(suspendGracePeriod)) {
				return "suspended"
			}
			if now.After(end) {
				return "expired"
			}
		}
	}
	return "active"
}

// suspendGracePeriod — durée de grâce en lecture seule avant suspension totale.
// Au-delà de PeriodEnd + cette durée, le compte passe en "suspended" : plus
// aucune route métier n'est accessible (même en GET), sauf /api/subscription
// (pour payer) et /api/auth/me (pour identifier l'utilisateur).
const suspendGracePeriod = 30 * 24 * time.Hour // 30 jours

// planAmount — montant de la période pour une formule : Essentiel =
// prix × routeurs enregistrés (minimum 1) ; Illimité = forfait 12 000 F.
func planAmount(p model.SaasPlan, routerCount int) int {
	if p.PerRouter {
		rc := routerCount
		if rc < 1 {
			rc = 1
		}
		return p.PriceFcfa * rc
	}
	return p.PriceFcfa
}

// accountRouterCount — nombre de routeurs enregistrés du compte (tous modes :
// simulé, réel, agent). C'est l'assiette de facturation de la formule Essentiel.
func accountRouterCount(db *model.DB, acc string) int {
	n := 0
	for _, rt := range db.Routers {
		if rt.AccountID == acc {
			n++
		}
	}
	return n
}

// handlePlansList — catalogue public des formules (console → panneau Abonnement).
func (a *API) handlePlansList(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, model.SaasPlans)
}

// handleSubscriptionGet — état de l'abonnement du compte + catalogue + assiette.
func (a *API) handleSubscriptionGet(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	if c := claimsFrom(r); c != nil && c.Acc == "" {
		// Admin plateforme (sans compte client) ou token legacy sans périmètre :
		// pas d'état d'abonnement auto-géré — la plateforme consulte chaque client depuis sa console.
		writeErrCode(w, http.StatusForbidden, "forbidden", "Réservé aux comptes clients — la plateforme gère les abonnements", nil)
		return
	}
	a.store.Lock()
	db := a.store.Data()
	settings := ensureSettings(db, acc)
	view := subscriptionView{
		Subscription: settings.Subscription,
		RouterCount:  accountRouterCount(db, acc),
		Plans:        model.SaasPlans,
	}
	// Répercussion des frais de paiement (stratégie validée) : montants par
	// moyen pour chaque formule, à l'assiette routeurs du compte — le client
	// voit le montant exact AVANT de choisir (remise Wave / prix de liste).
	for _, p := range model.SaasPlans {
		pr := planPricingOfPlan(p, view.RouterCount)
		view.Pricing = append(view.Pricing, planPricingView{PlanID: p.ID, planPricing: pr})
	}
	// waveConfigured = lien de paiement Wave de la PLATEFORME (WAVE_PAY_LINK)
	// configuré — PAS le lien marchand du client (réservé à ses propres vouchers).
	view.WaveConfigured = wavePayLink() != ""
	if p, ok := model.PlanByID(settings.Subscription.PlanID); ok {
		view.CurrentAmountFcfa = planAmount(p, view.RouterCount)
	}
	view.Status = subscriptionStatus(settings.Subscription, time.Now().UTC())
	a.store.Unlock()
	writeJSON(w, http.StatusOK, view)
}

// wavePayLink — lien marchand Wave de la PLATEFORME pour l'encaissement des
// abonnements SaaS (variable d'environnement WAVE_PAY_LINK, configurée sur
// Render). ATTENTION : le Tenant.WaveLink du client est son PROPRE lien
// marchand (vente de vouchers) — il ne doit jamais servir à encaisser
// l'abonnement MikCloud.
func wavePayLink() string {
	return strings.TrimSpace(os.Getenv("WAVE_PAY_LINK"))
}

// handleSubscriptionPost — DEMANDE de souscription / renouvellement (verrou
// cycle de facturation). Aucune activation ici : la souscription immédiate
// permettait à un compte expiré — voire suspendu — de se réabonner sans
// payer. Désormais : le client choisit une formule, reçoit le montant et le
// lien de paiement Wave de la plateforme ; la demande est tracée dans le
// journal (type billing) et l'ACTIVATION reste réservée à la plateforme
// (PUT /api/admin/accounts/{id}/subscription), après encaissement confirmé.
// Corps : {"planId":"essentiel"|"illimite"}.
func (a *API) handleSubscriptionPost(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	if c := claimsFrom(r); c != nil && c.Acc == "" {
		// Admin plateforme (sans compte client) ou token legacy sans périmètre :
		// pas d'auto-souscription — la plateforme gère les abonnements via
		// /api/admin/accounts/{id}/subscription (après encaissement).
		writeErrCode(w, http.StatusForbidden, "forbidden", "Réservé aux comptes clients — la plateforme gère les abonnements", nil)
		return
	}
	var req struct {
		PlanID string `json:"planId"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	plan, ok := model.PlanByID(strings.TrimSpace(req.PlanID))
	if !ok {
		writeErr(w, http.StatusBadRequest, "Formule inconnue (essentiel | illimite)")
		return
	}

	a.store.Lock()
	db := a.store.Data()
	settings := ensureSettings(db, acc)
	routerCount := accountRouterCount(db, acc)
	// Répercussion des frais de paiement (stratégie validée) : le montant
	// DEMANDÉ est celui du moyen par défaut (Wave, remise mobile money) ;
	// la base (net cible) et le prix de liste carte accompagnent la demande —
	// l'initiation carte bascule le montant (POST /api/subscription/stripe)
	// et l'activation manuelle plateforme reste au net (exemption admin).
	payPr := payPricingOf(planAmount(plan, routerCount))
	base := payPr.BaseFcfa
	amount := payPr.WaveFcfa
	periodLabel := "1 mois"
	if plan.Period == "an" {
		periodLabel = "1 an"
	}
	// Anti-spam : une demande identique de moins de 10 minutes n'encombre pas
	// le journal (la réponse — montant + référence — reste identique).
	prefix := fmt.Sprintf("Demande d'abonnement %s", plan.Name)
	recent := false
	for _, act := range db.Activity {
		if act.AccountID == acc && act.Type == "billing" && strings.HasPrefix(act.Message, prefix) {
			if at, err := time.Parse(time.RFC3339, act.At); err == nil && time.Since(at) < 10*time.Minute {
				recent = true
			}
			break
		}
	}
	// File de facturation (console plateforme) : une SEULE demande en attente
	// par compte. Une demande identique en attente est réutilisée (même
	// référence) ; une demande d'une autre formule est remplacée (l'ancienne
	// passe « cancelled »). Le journal conserve son anti-spam propre.
	ref := ""
	changed := false
	for i := range db.BillingRequests {
		if db.BillingRequests[i].AccountID == acc && db.BillingRequests[i].Status == "pending" {
			// Appariement sur la BASE (indépendant du moyen) : une demande
			// basculée carte reste réutilisable, renormalisée sur le moyen
			// par défaut (Wave).
			br := &db.BillingRequests[i]
			baseReq := br.BaseAmountFcfa
			if baseReq <= 0 {
				baseReq = br.AmountFcfa // demande antérieure aux frais : base = net historique
			}
			if br.PlanID == plan.ID && baseReq == base {
				if br.AmountFcfa != amount || br.PayMethod != "wave" {
					br.AmountFcfa = amount
					br.PayMethod = "wave"
					changed = true
				}
				ref = br.Ref // réutilisation, même référence
			} else {
				br.Status = "cancelled"
				br.ResolvedAt = model.NowISO()
				br.ResolvedBy = "client"
				br.Note = "Remplacée par une nouvelle demande du client"
				changed = true
			}
			break
		}
	}
	if ref == "" {
		ref = newPayRef(db)
		db.BillingRequests = append([]model.BillingRequest{{
			ID: model.NewID("breq-"), AccountID: acc, PlanID: plan.ID, PlanName: plan.Name,
			AmountFcfa: amount, BaseAmountFcfa: base, PayMethod: "wave",
			PeriodLabel: periodLabel, RouterCount: routerCount,
			Ref: ref, Status: "pending", CreatedAt: model.NowISO(),
		}}, db.BillingRequests...)
		changed = true
		// Historique borné : au-delà de 500 demandes, les plus anciennes déjà
		// traitées sont élaguées (les EN ATTENTE ne sont jamais retirées).
		if len(db.BillingRequests) > 500 {
			kept := make([]model.BillingRequest, 0, len(db.BillingRequests))
			for i, br := range db.BillingRequests {
				if i < 500 || br.Status == "pending" {
					kept = append(kept, br)
				}
			}
			db.BillingRequests = kept
		}
	}
	if !recent {
		a.logActivityBy(r, db, acc, "billing",
			fmt.Sprintf("%s — %d FCFA / %s · %d routeur(s) enregistré(s) — en attente d'encaissement (réf. %s)",
				prefix, amount, periodLabel, routerCount, ref))
		changed = true
	}
	if changed {
		a.store.Save()
	}
	a.store.Unlock()

	waveLink := ""
	if base := wavePayLink(); base != "" {
		waveLink = strings.TrimRight(base, "/") + "/amount/" + strconv.Itoa(amount) + "/"
	}
	// subscription RENVOYÉE INCHANGÉE : l'activation est une décision de la
	// plateforme (après encaissement), jamais du client. La référence sert au
	// suivi de paiement (file plateforme + webhook Wave).
	writeJSON(w, http.StatusOK, map[string]any{
		"subscription":   settings.Subscription,
		"amountFcfa":     amount,
		"baseAmountFcfa": base,
		"listAmountFcfa": payPr.ListFcfa,
		"routerCount":    routerCount,
		"periodLabel":    periodLabel,
		"ref":            ref,
		"waveLink":       waveLink,
		"pending":        true,
	})
}
