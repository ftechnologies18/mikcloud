// Package api — abonnement RÉCURRENT par carte bancaire (Stripe via
// GeniusPay) : routes console du compte.
//
// Flux retenu :
//   - POST /api/subscription/stripe : création de l'abonnement récurrent
//     (Essentiel → mensuel au prorata des routeurs, Illimité → annuel),
//     création/réutilisation de la DEMANDE de facturation de la première
//     période puis initiation d'un paiement EN LIGNE pour celle-ci (routage
//     à la discrétion de GeniusPay : carte si disponible, sinon Wave) ;
//   - la confirmation du paiement (webhook payment.success SIGNÉ, chemin
//     existant) active la première période ;
//   - les ÉCHÉANCES suivantes sont débitées par GeniusPay :
//     subscription.payment_succeeded (webhook, signé ou re-vérifié à la
//     source) active/empile la période — filet de sécurité : resync des
//     factures réelles au GET statut ;
//   - POST /api/subscription/stripe/cancel : résiliation synchronisée chez
//     GeniusPay — l'accès MikCloud reste actif jusqu'au terme payé ;
//   - GET /api/subscription/stripe : état + resynchronisation (retour de
//     paiement, webhook retardé…).
//
// Garde P3 : souscrire reste possible pour un compte expiré OU suspendu —
// c'est une voie de réactivation (middleware autorisant
// /api/subscription/stripe).
package api

import (
	"encoding/json"
	"fmt"
	"mikcloud/hotspot-api/internal/model"
	"net/http"
	"os"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// POST /api/subscription/stripe — initiation du prélèvement automatique
// ---------------------------------------------------------------------------

// handleSubscriptionStripePost — crée l'abonnement récurrent carte du compte
// (Essentiel → mensuel, 1 250 F × routeurs ; Illimité → annuel, 12 000 F),
// prépare la DEMANDE de la première période et initie son paiement en ligne.
// L'e-mail est requis par le module carte de GeniusPay ; le téléphone du
// profil complète le dossier client. Réponse : {uuid, status, nextBilling,
// redirectUrl, paymentUrl, ref, planId, planName, amountFcfa, cycle}.
func (a *API) handleSubscriptionStripePost(w http.ResponseWriter, r *http.Request) {
	if c := claimsFrom(r); c != nil && c.Acc == "" {
		// Admin plateforme (sans compte client) : pas d'auto-souscription.
		writeErrCode(w, http.StatusForbidden, "forbidden", "Réservé aux comptes clients — la plateforme gère les abonnements", nil)
		return
	}
	if _, _, ok := geniusPayKeys(); !ok {
		writeErrCode(w, http.StatusServiceUnavailable, "geniuspay_disabled",
			"Paiement par carte non configuré — contactez l'équipe MikCloud", nil)
		return
	}
	var req struct {
		PlanID string `json:"planId"`
		Email  string `json:"email"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErrCode(w, http.StatusBadRequest, "bad_request", "Corps de requête invalide", nil)
		return
	}
	planID := strings.ToLower(strings.TrimSpace(req.PlanID))
	plan, ok := model.PlanByID(planID)
	if !ok {
		writeErrCode(w, http.StatusBadRequest, "bad_plan", "Formule inconnue (essentiel | illimite)", nil)
		return
	}

	acc, phone, name, email := "", "", "", ""
	routerCount := 0
	a.store.Lock()
	db := a.store.Data()
	for i := range db.Accounts {
		if db.Accounts[i].ID == accountScope(r) {
			acc = db.Accounts[i].ID
			phone = normalizeGeniusPayPhone(db.Accounts[i].Phone)
			name = db.Accounts[i].Name
			email = strings.TrimSpace(db.Accounts[i].Email)
			routerCount = accountRouterCount(db, acc)
			break
		}
	}
	a.store.Unlock()
	if acc == "" {
		writeErrCode(w, http.StatusNotFound, "not_found", "Compte introuvable", nil)
		return
	}
	if e := strings.TrimSpace(req.Email); e != "" {
		email = e
	}
	if phone == "" {
		writeErrCode(w, http.StatusUnprocessableEntity, "missing_phone",
			"Renseignez votre numéro de téléphone (profil) pour le prélèvement automatique", nil)
		return
	}
	if email == "" {
		writeErrCode(w, http.StatusUnprocessableEntity, "missing_email",
			"L'e-mail est requis pour le prélèvement automatique par carte", nil)
		return
	}
	// Un seul prélèvement automatique actif à la fois (re-souscription
	// tolérée au-delà d'une heure : la précédente non réglée est considérée
	// abandonnée — si elle était finalement payée, la période serait
	// quand même honorée par le webhook, idempotent).
	a.store.Lock()
	db = a.store.Data()
	for i := range db.GeniusPaySubs {
		g := db.GeniusPaySubs[i]
		if g.AccountID != acc || !stripeSubInProgress(g.Status) {
			continue
		}
		if created, err := time.Parse(time.RFC3339, g.CreatedAt); err == nil && time.Since(created) < time.Hour {
			a.store.Unlock()
			writeErrCode(w, http.StatusConflict, "already_active",
				"Un prélèvement automatique par carte est déjà en cours — annulez-le d'abord pour changer de formule", nil)
			return
		}
	}
	a.store.Unlock()

	cycle, slots := "monthly", 0
	amount := plan.PriceFcfa
	if plan.PerRouter {
		slots = routerCount
		if slots < 1 {
			slots = 1
		}
		amount = plan.PriceFcfa * slots
	} else {
		cycle = "yearly"
	}
	// Répercussion des frais (stratégie validée) : la CARTE est encaissée au
	// PRIX DE LISTE (gross-up des frais carte 6 % + 100 XOF, arrondi 25 XOF
	// supérieur) — le net cible plateforme est préservé à chaque échéance du
	// prélèvement automatique.
	base := amount
	amount = cardListPriceFcfa(base)
	periodLabel := "1 mois"
	if plan.Period == "an" {
		periodLabel = "1 an"
	}
	planName := fmt.Sprintf("MikCloud %s (%s)", plan.Name, periodLabel)
	if plan.PerRouter {
		planName = fmt.Sprintf("MikCloud %s (%d routeur(s))", plan.Name, slots)
	}

	// Abonnement GeniusPay — mode stripe_checkout, metadata = traçabilité.
	payload := map[string]any{
		"customer_phone": phone,
		"customer_name":  name,
		"customer_email": email,
		"plan_name":      planName,
		"amount":         amount,
		"currency":       "XOF",
		"billing_cycle":  cycle,
		"payment_method": "stripe_checkout",
		"metadata":       map[string]string{"account_id": acc, "plan_id": planID, "source": "mikcloud"},
	}
	var appURL string
	if appURL = strings.TrimRight(strings.TrimSpace(os.Getenv("APP_PUBLIC_URL")), "/"); appURL != "" {
		payload["success_url"] = appURL + "/?stripe=return"
		payload["cancel_url"] = appURL + "/?stripe=cancel"
	}
	idem := fmt.Sprintf("mikcloud-%s-%d", acc, time.Now().UnixNano())
	env, err := geniusPayCall(http.MethodPost, "/subscriptions", payload, []string{"Idempotency-Key", idem})
	if err != nil || env == nil {
		writeErrCode(w, http.StatusBadGateway, "geniuspay_error",
			"Initiation du prélèvement automatique impossible — "+err.Error(), nil)
		return
	}
	var cr geniusPaySubCreateResp
	if len(env.Data) == 0 || json.Unmarshal(env.Data, &cr) != nil || cr.Subscription == nil || cr.Subscription.key() == "" {
		writeErrCode(w, http.StatusBadGateway, "geniuspay_error",
			"Réponse GeniusPay inattendue (abonnement absent)", nil)
		return
	}
	status := strings.TrimSpace(cr.Subscription.Status)
	if status == "" {
		status = "pending"
	}

	// Demande de facturation de la PREMIÈRE période — mêmes règles que
	// POST /api/subscription : une SEULE demande en attente par compte
	// (identique réutilisée, autre formule remplacée). L'abonnement récurrent
	// ne prouve aucun encaissement : la première période doit être réglée
	// explicitement ; les suivantes sont débitées par GeniusPay.
	ref := ""
	now := time.Now().UTC().Format(time.RFC3339)
	a.store.Lock()
	db = a.store.Data()
	for i := range db.BillingRequests {
		br := db.BillingRequests[i]
		if br.AccountID != acc || br.Status != "pending" {
			continue
		}
		matchBase := br.BaseAmountFcfa
		if matchBase <= 0 {
			matchBase = br.AmountFcfa // demande antérieure aux frais : base = net historique
		}
		if br.PlanID == planID && matchBase == base {
			// Réutilisation — bascule sur le moyen carte (prix de liste) : une
			// initiation Wave précédente devient obsolète.
			if br.AmountFcfa != amount || br.PayMethod != "card" {
				db.BillingRequests[i].AmountFcfa = amount
				db.BillingRequests[i].PayMethod = "card"
				db.BillingRequests[i].GatewayRef = ""
			}
			ref = br.Ref // réutilisation (même référence)
		} else {
			db.BillingRequests[i].Status = "cancelled"
			db.BillingRequests[i].ResolvedAt = model.NowISO()
			db.BillingRequests[i].ResolvedBy = "client"
			db.BillingRequests[i].Note = "Remplacée par une nouvelle demande du client"
		}
		break
	}
	if ref == "" {
		ref = newPayRef(db)
		db.BillingRequests = append([]model.BillingRequest{{
			ID: model.NewID("breq-"), AccountID: acc, PlanID: planID, PlanName: plan.Name,
			AmountFcfa: amount, BaseAmountFcfa: base, PayMethod: "card",
			PeriodLabel: periodLabel, RouterCount: routerCount,
			Ref: ref, Status: "pending", CreatedAt: model.NowISO(),
		}}, db.BillingRequests...)
	}
	gps := model.GeniusPaySub{
		UUID:          cr.Subscription.key(),
		AccountID:     acc,
		PlanID:        planID,
		PlanName:      planName,
		Cycle:         cycle,
		AmountFcfa:    amount,
		Slots:         slots,
		Status:        status,
		CustomerName:  name,
		CustomerEmail: email,
		Phone:         phone,
		NextBilling:   strings.TrimSpace(cr.Subscription.NextBillingDate),
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	db.GeniusPaySubs = append(db.GeniusPaySubs, gps)
	a.logActivity(db, acc, "billing",
		fmt.Sprintf("Prélèvement automatique par carte initié (Stripe via GeniusPay) — %s, %d FCFA / %s, abonnement %s, première période en attente (réf. %s)",
			planName, amount, stripeCycleLabel(cycle), gps.UUID, ref))
	a.store.Save()
	a.store.Unlock()

	// Paiement EN LIGNE de la première période (routage à la discrétion de
	// GeniusPay : carte si disponible, sinon Wave) — sa confirmation (webhook
	// payment.success signé) active la période, comme pour un règlement Wave.
	payURL, gatewayRef := "", ""
	payPayload := geniusPayCreateReq{
		Amount:        amount,
		PaymentMethod: "card",
		Description:   fmt.Sprintf("Abonnement MikCloud %s (%s) — réf %s", plan.Name, periodLabel, ref),
		Customer:      &geniusPayCustomer{Name: name, Phone: phone, Email: email},
		Metadata:      map[string]string{"ref": ref, "account_id": acc, "plan_id": planID, "source": "stripe-first"},
	}
	if appURL != "" {
		payPayload.SuccessURL = appURL + "/?stripe=return"
		payPayload.ErrorURL = appURL + "/?stripe=cancel"
	}
	if env, err := geniusPayCall(http.MethodPost, "/payments", payPayload); err == nil && env != nil {
		var pay geniusPayPayment
		if json.Unmarshal(env.Data, &pay) == nil {
			payURL = pay.PaymentURL
			if payURL == "" {
				payURL = pay.CheckoutURL
			}
			gatewayRef = strings.TrimSpace(pay.Reference)
		}
	}
	if gatewayRef != "" {
		a.store.Lock()
		db = a.store.Data()
		for i := range db.BillingRequests {
			if db.BillingRequests[i].AccountID == acc && strings.EqualFold(db.BillingRequests[i].Ref, ref) && db.BillingRequests[i].Status == "pending" {
				db.BillingRequests[i].GatewayRef = gatewayRef
				a.store.Save()
				break
			}
		}
		a.store.Unlock()
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"uuid":        gps.UUID,
		"status":      gps.Status,
		"nextBilling": gps.NextBilling,
		"redirectUrl": cr.RedirectURL, // présent uniquement si GeniusPay renvoie un Checkout (futur)
		"paymentUrl":  payURL,         // paiement en ligne de la première période
		"ref":         ref,
		"planId":      planID,
		"planName":    planName,
		"amountFcfa":  amount,
		"cycle":       cycle,
	})
}

// ---------------------------------------------------------------------------
// GET /api/subscription/stripe — état + resynchronisation (retour paiement)
// ---------------------------------------------------------------------------

// handleSubscriptionStripeGet — état du prélèvement automatique du compte.
// Si la fiche est en cours, les factures réelles sont consultées chez
// GeniusPay : une facture payée non encore appliquée active/empile la période
// (webhook retardé…), puis le statut distant rafraîchit la fiche. Réponse :
// {status, uuid, planId, planName, cycle, amountFcfa, nextBilling,
// lastRenewalAt, subscription{...}}.
func (a *API) handleSubscriptionStripeGet(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	a.store.Lock()
	db := a.store.Data()
	idx := -1
	for i := range db.GeniusPaySubs {
		if db.GeniusPaySubs[i].AccountID == acc {
			if idx < 0 || db.GeniusPaySubs[i].CreatedAt > db.GeniusPaySubs[idx].CreatedAt {
				idx = i
			}
		}
	}
	if idx < 0 {
		a.store.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{"status": "none"})
		return
	}
	gps := db.GeniusPaySubs[idx]
	id := gps.UUID
	a.store.Unlock()

	// Resynchronisation (hors verrou : appels réseau, budget ~2 requêtes).
	if stripeSubNeedsSync(gps.Status) {
		a.syncStripeInvoices(id)
		if st, err := geniusPaySubStatus(id); err == nil {
			a.stripeSyncMeta(id, st.Status, st.NextBillingDate)
		}
	}

	a.store.Lock()
	db = a.store.Data()
	out := gps
	for i := range db.GeniusPaySubs {
		if db.GeniusPaySubs[i].UUID == id {
			out = db.GeniusPaySubs[i]
			break
		}
	}
	var sub model.Subscription
	statusNow := "none"
	if st, ok := db.SettingsByAccount[acc]; ok {
		sub = st.Subscription
		statusNow = subscriptionStatus(sub, time.Now().UTC())
	}
	a.store.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"status": out.Status, "uuid": out.UUID, "planId": out.PlanID,
		"planName": out.PlanName, "cycle": out.Cycle, "amountFcfa": out.AmountFcfa,
		"nextBilling": out.NextBilling, "lastRenewalAt": out.LastRenewalAt,
		"subscription": map[string]any{
			"planId": sub.PlanID, "status": statusNow,
			"periodStart": sub.PeriodStart, "periodEnd": sub.PeriodEnd,
		},
	})
}

// ---------------------------------------------------------------------------
// POST /api/subscription/stripe/cancel — résiliation du prélèvement
// ---------------------------------------------------------------------------

// handleSubscriptionStripeCancel — résilie le prélèvement automatique chez
// GeniusPay (fin de période par défaut : le client garde l'accès jusqu'au
// terme payé ; immédiat sur demande explicite). La période MikCloud en cours
// n'est PAS raccourcie — elle expire naturellement.
func (a *API) handleSubscriptionStripeCancel(w http.ResponseWriter, r *http.Request) {
	if c := claimsFrom(r); c != nil && c.Acc == "" {
		writeErrCode(w, http.StatusForbidden, "forbidden", "Réservé aux comptes clients", nil)
		return
	}
	var req struct {
		Immediately bool   `json:"immediately"`
		Reason      string `json:"reason"`
	}
	_ = decodeBody(r, &req) // corps optionnel

	acc := accountScope(r)
	a.store.Lock()
	db := a.store.Data()
	idx := -1
	for i := range db.GeniusPaySubs {
		if db.GeniusPaySubs[i].AccountID == acc && stripeSubInProgress(db.GeniusPaySubs[i].Status) {
			if idx < 0 || db.GeniusPaySubs[i].CreatedAt > db.GeniusPaySubs[idx].CreatedAt {
				idx = i
			}
		}
	}
	if idx < 0 {
		a.store.Unlock()
		writeErrCode(w, http.StatusNotFound, "no_active_subscription",
			"Aucun prélèvement automatique actif à résilier", nil)
		return
	}
	id, planName := db.GeniusPaySubs[idx].UUID, db.GeniusPaySubs[idx].PlanName
	a.store.Unlock()

	finalStatus, err := geniusPaySubCancel(id, req.Immediately, req.Reason)
	if err != nil {
		writeErrCode(w, http.StatusBadGateway, "geniuspay_error",
			"Résiliation impossible — "+err.Error(), nil)
		return
	}

	now := time.Now().UTC()
	a.store.Lock()
	db = a.store.Data()
	for i := range db.GeniusPaySubs {
		if db.GeniusPaySubs[i].UUID == id {
			g := &db.GeniusPaySubs[i]
			g.Status = finalStatus
			g.CancelledAt = now.Format(time.RFC3339)
			g.UpdatedAt = g.CancelledAt
			break
		}
	}
	when := "fin de période"
	if req.Immediately {
		when = "immédiate"
	}
	a.logActivity(db, acc, "billing",
		fmt.Sprintf("Prélèvement automatique carte résilié (%s, %s) — l'accès reste actif jusqu'à la fin de la période payée",
			planName, when))
	a.store.Save()
	a.store.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{"status": finalStatus, "uuid": id})
}
