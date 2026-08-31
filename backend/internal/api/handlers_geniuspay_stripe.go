// Package api — abonnement RÉCURRENT par carte bancaire (Stripe via
// GeniusPay, API « Abonnements Stripe »).
//
// Complète le paiement Wave PONCTUEL (handlers_geniuspay.go) d'un
// prélèvement automatique : le client souscrit une fois, et chaque échéance
// payée active ou empile la période MikCloud — exactement comme un
// règlement accepté par la plateforme (source unique applySubscriptionLocked).
//
// COMPORTEMENT RÉEL DE L'API (constaté en production, différent de la doc) :
// POST /subscriptions crée l'abonnement directement « active », SANS
// redirection Stripe Checkout, le premier prélèvement tombant à
// next_billing_date. Le statut « active » ne prouve donc AUCUN encaissement.
// D'où le flux retenu :
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
// Sécurité : la clé secrète ne quitte jamais le serveur (même client
// geniusPayCall que les paiements) ; un événement d'abonnement NON signé ne
// produit un effet QUE re-vérifié à la source (factures GeniusPay réelles) ;
// le montant débité est confronté au montant souscrit ; l'application d'une
// facture est idempotente (paid_at ≤ last_invoice_at → no-op). Garde P3 :
// souscrire reste possible pour un compte expiré OU suspendu — c'est une voie
// de réactivation (middleware autorisant /api/subscription/stripe).
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"mikcloud/hotspot-api/internal/model"
)

// ---------------------------------------------------------------------------
// Types API « Abonnements Stripe » GeniusPay
// ---------------------------------------------------------------------------

// geniusPaySubscription — abonnement renvoyé par POST /subscriptions, le
// détail et le statut. Forme RÉELLE : identifiant dans « id » (UUID) ;
// forme DOCUMENTÉE (compat future) : « uuid ». Les deux sont acceptés.
type geniusPaySubscription struct {
	ID              string  `json:"id"`
	UUID            string  `json:"uuid"`
	Status          string  `json:"status"` // pending|trialing|active|past_due|paused|cancelled|expired
	PlanName        string  `json:"plan_name"`
	Amount          float64 `json:"amount"`
	Currency        string  `json:"currency"`
	BillingCycle    string  `json:"billing_cycle"`
	NextBillingDate string  `json:"next_billing_date"`
}

// key — identifiant utilisable (uuid documenté, sinon id réel).
func (s *geniusPaySubscription) key() string {
	if k := strings.TrimSpace(s.UUID); k != "" {
		return k
	}
	return strings.TrimSpace(s.ID)
}

// geniusPaySubCreateResp — réponse POST /subscriptions. Forme RÉELLE : champs
// directs dans data (id, status, next_billing_date…) ; forme DOCUMENTÉE
// (compat future) : {subscription:{…}, redirect_url}. Décodage tolérant.
type geniusPaySubCreateResp struct {
	Subscription *geniusPaySubscription `json:"subscription"` // forme documentée
	RedirectURL  string                 `json:"redirect_url"` // présent si redirection Checkout (futur)
}

// UnmarshalJSON — accepte les deux formes (wrapper documenté, puis direct).
func (c *geniusPaySubCreateResp) UnmarshalJSON(b []byte) error {
	var wrapper struct {
		Subscription *geniusPaySubscription `json:"subscription"`
		RedirectURL  string                 `json:"redirect_url"`
	}
	if err := json.Unmarshal(b, &wrapper); err != nil {
		return err
	}
	c.Subscription, c.RedirectURL = wrapper.Subscription, wrapper.RedirectURL
	if c.Subscription == nil {
		var direct geniusPaySubscription
		if err := json.Unmarshal(b, &direct); err != nil {
			return err
		}
		c.Subscription = &direct
	}
	return nil
}

// geniusPayInvoice — facture d'abonnement (GET /subscriptions/{id}/invoices).
type geniusPayInvoice struct {
	UUID      string  `json:"uuid"`
	Amount    float64 `json:"amount"`
	Status    string  `json:"status"` // paid|pending|failed|voided
	PaidAt    string  `json:"paid_at"`
	CreatedAt string  `json:"created_at"`
}

// geniusPayWebhookData — charge utile d'un événement GeniusPay. Les champs
// Reference/Amount/Status/Metadata concernent les PAIEMENTS (transaction) ;
// Subscription/Invoice concernent les ABONNEMENTS carte (Stripe).
type geniusPayWebhookData struct {
	Reference    string               `json:"reference"`
	Amount       float64              `json:"amount"`
	Status       string               `json:"status"`
	Provider     string               `json:"provider"`
	Metadata     map[string]any       `json:"metadata"`
	Subscription *geniusPayWebhookSub `json:"subscription,omitempty"`
	Invoice      *geniusPayWebhookInv `json:"invoice,omitempty"`
}

// geniusPayWebhookSub — abonnement embarqué dans un événement subscription.*.
type geniusPayWebhookSub struct {
	UUID            string  `json:"uuid"`
	Status          string  `json:"status"`
	PlanName        string  `json:"plan_name"`
	Amount          float64 `json:"amount"`
	Currency        string  `json:"currency"`
	NextBillingDate string  `json:"next_billing_date"`
}

// geniusPayWebhookInv — facture embarquée dans un événement subscription.*.
type geniusPayWebhookInv struct {
	UUID   string  `json:"uuid"`
	Amount float64 `json:"amount"`
	Status string  `json:"status"`
	PaidAt string  `json:"paid_at"`
}

// ---------------------------------------------------------------------------
// Appels API abonnements (même base et mêmes en-têtes que l'API paiements)
// ---------------------------------------------------------------------------

// geniusPaySubStatus — statut réel de l'abonnement chez GeniusPay
// (forme réelle : {is_active, status, next_billing_date, …}).
func geniusPaySubStatus(id string) (*geniusPaySubscription, error) {
	env, err := geniusPayCall(http.MethodGet, "/subscriptions/"+url.PathEscape(id)+"/status", nil)
	if err != nil {
		return nil, err
	}
	var s geniusPaySubscription
	if len(env.Data) == 0 || json.Unmarshal(env.Data, &s) != nil || strings.TrimSpace(s.Status) == "" {
		return nil, errors.New("réponse GeniusPay inattendue")
	}
	return &s, nil
}

// geniusPaySubInvoices — factures de l'abonnement (tri par date de paiement
// croissante fait par l'appelant).
func geniusPaySubInvoices(id string) ([]geniusPayInvoice, error) {
	env, err := geniusPayCall(http.MethodGet, "/subscriptions/"+url.PathEscape(id)+"/invoices?per_page=50", nil)
	if err != nil {
		return nil, err
	}
	var invs []geniusPayInvoice
	if len(env.Data) == 0 || json.Unmarshal(env.Data, &invs) != nil {
		return nil, errors.New("réponse GeniusPay inattendue")
	}
	return invs, nil
}

// geniusPaySubCancel — résiliation synchronisée chez GeniusPay (arrêt des
// prélèvements). Renvoie le statut final de l'abonnement distant.
func geniusPaySubCancel(id string, immediately bool, reason string) (string, error) {
	body := map[string]any{"immediately": immediately}
	if rr := strings.TrimSpace(reason); rr != "" {
		body["reason"] = rr
	}
	env, err := geniusPayCall(http.MethodPost, "/subscriptions/"+url.PathEscape(id)+"/cancel", body)
	if err != nil {
		return "", err
	}
	var out geniusPaySubCreateResp
	if len(env.Data) > 0 && json.Unmarshal(env.Data, &out) == nil && out.Subscription != nil {
		if s := strings.TrimSpace(out.Subscription.Status); s != "" {
			return s, nil
		}
	}
	return "cancelled", nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// stripeSubInProgress — le prélèvement automatique est-il en cours (fiche à
// protéger d'une double souscription, candidate à la résiliation) ?
func stripeSubInProgress(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "pending", "trialing", "active", "past_due", "paused", "":
		return true
	}
	return false
}

// stripeSubNeedsSync — le statut justifie-t-il une resynchronisation auprès
// de GeniusPay (factures réelles) ?
func stripeSubNeedsSync(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "pending", "trialing", "active", "past_due", "paused", "":
		return true
	}
	return false
}

// stripeCycleMonths — période MikCloud couverte par un paiement du cycle
// donné. Seuls monthly et yearly sont proposés par l'initiation ; les autres
// valeurs GeniusPay sont mappées prudemment (1 mois par paiement).
func stripeCycleMonths(cycle string) int {
	switch strings.ToLower(strings.TrimSpace(cycle)) {
	case "yearly":
		return 12
	case "quarterly":
		return 3
	default: // monthly, weekly, daily… : période minimale défensive
		return 1
	}
}

// stripeCycleLabel — libellé français du cycle (journal).
func stripeCycleLabel(cycle string) string {
	if strings.EqualFold(strings.TrimSpace(cycle), "yearly") {
		return "an"
	}
	return "mois"
}

// stripeSyncMeta — met à jour le statut et/ou la prochaine échéance de la
// fiche locale (mise à jour informative : la période MikCloud active n'est
// PAS modifiée ici). Sous verrou, no-op si rien ne change.
func (a *API) stripeSyncMeta(id, status, nextBilling string) {
	status = strings.ToLower(strings.TrimSpace(status))
	switch status {
	case "pending", "trialing", "active", "past_due", "paused", "cancelled", "expired":
	default:
		status = "" // statut inconnu : ignoré
	}
	nextBilling = strings.TrimSpace(nextBilling)
	if status == "" && nextBilling == "" {
		return
	}
	a.store.Lock()
	defer a.store.Unlock()
	db := a.store.Data()
	for i := range db.GeniusPaySubs {
		if db.GeniusPaySubs[i].UUID != id {
			continue
		}
		g := &db.GeniusPaySubs[i]
		changed := false
		if status != "" && status != strings.ToLower(g.Status) {
			g.Status = status
			changed = true
		}
		if nextBilling != "" && nextBilling != g.NextBilling {
			g.NextBilling = nextBilling
			changed = true
		}
		if changed {
			g.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
			a.store.Save()
		}
		return
	}
}

// stripeSubLookup — fiche locale de l'abonnement carte par identifiant
// GeniusPay (copie, pour consultation hors verrou).
func (a *API) stripeSubLookup(id string) (model.GeniusPaySub, bool) {
	a.store.Lock()
	defer a.store.Unlock()
	db := a.store.Data()
	for i := range db.GeniusPaySubs {
		if db.GeniusPaySubs[i].UUID == id {
			return db.GeniusPaySubs[i], true
		}
	}
	return model.GeniusPaySub{}, false
}

// applyStripeRenewalByUUID — active/empile la période MikCloud correspondant
// à une facture PAYÉE de l'abonnement carte identifié par son identifiant
// GeniusPay. Idempotent (paid_at ≤ last_invoice_at → no-op) ; le montant reçu
// est confronté au montant souscrit (incohérence → refus tracé). Renvoie true
// si la période a été appliquée. Sous verrou, source unique
// applySubscriptionLocked.
func (a *API) applyStripeRenewalByUUID(id, paidAt string, amount float64) bool {
	a.store.Lock()
	defer a.store.Unlock()
	db := a.store.Data()
	for i := range db.GeniusPaySubs {
		if db.GeniusPaySubs[i].UUID != id {
			continue
		}
		g := &db.GeniusPaySubs[i]
		if paidAt != "" && g.LastInvoiceAt != "" && paidAt <= g.LastInvoiceAt {
			return false // déjà appliqué (webhook rejoué / resync redondante)
		}
		if amount > 0 && int(amount) != g.AmountFcfa {
			a.logActivity(db, g.AccountID, "billing",
				fmt.Sprintf("Prélèvement carte (%s) refusé — montant reçu %.0f FCFA ≠ montant souscrit %d FCFA",
					g.PlanName, amount, g.AmountFcfa))
			a.store.Save()
			return false
		}
		months := stripeCycleMonths(g.Cycle)
		slots := g.Slots
		if g.PlanID == "essentiel" {
			// La couverture suit le parc réel (comme finalizeBillingSuccess) :
			// des routeurs ajoutés restent couverts après renouvellement.
			if rc := accountRouterCount(db, g.AccountID); rc > slots {
				slots = rc
			}
			if slots < 1 {
				slots = 1
			}
		}
		sub, label, _, _ := applySubscriptionLocked(db, g.AccountID, g.PlanID, months, slots, true)
		now := time.Now().UTC()
		g.Status = "active"
		g.LastRenewalAt = now.Format(time.RFC3339)
		if paidAt != "" {
			g.LastInvoiceAt = paidAt
		}
		g.UpdatedAt = now.Format(time.RFC3339)
		a.logActivity(db, g.AccountID, "billing",
			fmt.Sprintf("Prélèvement carte confirmé (Stripe via GeniusPay) — abonnement %s activé/empilé %d mois (%d FCFA / %s), période jusqu'au %s",
				label, months, sub.LastAmountFcfa, stripeCycleLabel(g.Cycle), sub.PeriodEnd))
		a.store.Save()
		return true
	}
	return false
}

// syncStripeInvoices — consulte les factures RÉELLES de l'abonnement chez
// GeniusPay et applique chaque facture payée postérieure à la dernière
// appliquée (idempotent, montant contrôlé). Renvoie le nombre de périodes
// activées. Filet de sécurité du GET statut et seule voie d'application
// d'un événement payment_succeeded NON signé.
func (a *API) syncStripeInvoices(id string) int {
	invs, err := geniusPaySubInvoices(id)
	if err != nil {
		return 0 // GeniusPay injoignable : réessai au prochain passage
	}
	sort.Slice(invs, func(i, j int) bool { return invs[i].PaidAt < invs[j].PaidAt })
	n := 0
	for _, inv := range invs {
		if !strings.EqualFold(strings.TrimSpace(inv.Status), "paid") || strings.TrimSpace(inv.PaidAt) == "" {
			continue
		}
		if a.applyStripeRenewalByUUID(id, inv.PaidAt, inv.Amount) {
			n++
		}
	}
	return n
}

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
		if br.PlanID == planID && br.AmountFcfa == amount {
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
			AmountFcfa: amount, PeriodLabel: periodLabel, RouterCount: routerCount,
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

// ---------------------------------------------------------------------------
// Webhook subscription.* (aiguillé par handleGeniusPayWebhook)
// ---------------------------------------------------------------------------

// handleStripeWebhookEvent — application d'un événement d'abonnement carte.
//   - payment_succeeded : SEUL événement engageant. Signé → montant de la
//     facture confronté au montant souscrit puis activation idempotente.
//     NON signé → aucune confiance au payload : re-vérification à la source
//     (factures réelles GeniusPay) avant application.
//   - autres (payment_failed, past_due, cancelled, expired, paused, created) :
//     mise à jour informative de la fiche locale (accès MikCloud inchangé —
//     la période active court jusqu'à son terme).
func (a *API) handleStripeWebhookEvent(w http.ResponseWriter, data *geniusPayWebhookData, signed bool, kind string) {
	id := ""
	if data != nil && data.Subscription != nil {
		id = strings.TrimSpace(data.Subscription.UUID)
	}
	if id == "" {
		writeErrCode(w, http.StatusBadRequest, "missing_uuid", "UUID d'abonnement absent", nil)
		return
	}

	if kind == "payment_succeeded" {
		if signed {
			gps, known := a.stripeSubLookup(id)
			if !known {
				writeErrCode(w, http.StatusNotFound, "unknown_subscription",
					"Aucun abonnement carte pour "+id, nil)
				return
			}
			amount := 0.0
			paidAt := ""
			if data.Invoice != nil {
				amount = data.Invoice.Amount
				paidAt = strings.TrimSpace(data.Invoice.PaidAt)
			}
			if amount > 0 && int(amount) != gps.AmountFcfa {
				writeErrCode(w, http.StatusConflict, "amount_mismatch",
					fmt.Sprintf("Montant incohérent : reçu %.0f FCFA, attendu %d FCFA (abonnement %s)",
						amount, gps.AmountFcfa, id), nil)
				return
			}
			if a.applyStripeRenewalByUUID(id, paidAt, amount) {
				a.stripeSyncMeta(id, data.Subscription.Status, data.Subscription.NextBillingDate)
				writeJSON(w, http.StatusOK, map[string]any{"ok": true, "uuid": id, "status": "done"})
				return
			}
			// Rien à appliquer : déjà traité (idempotence) ou montant refusé.
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "uuid": id, "status": "done", "idempotent": true})
			return
		}
		// Non signé : re-vérification à la source avant tout effet.
		n := a.syncStripeInvoices(id)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "uuid": id, "applied": n})
		return
	}

	if data != nil && data.Subscription != nil {
		a.stripeSyncMeta(id, data.Subscription.Status, data.Subscription.NextBillingDate)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "uuid": id, "event": kind})
}
