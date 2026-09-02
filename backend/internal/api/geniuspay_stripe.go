// Package api — intégration GeniusPay « API Abonnements Stripe » (carte
// bancaire, prélèvement récurrent). Complète le paiement Wave PONCTUEL
// (handlers_geniuspay.go).
//
// COMPORTEMENT RÉEL DE L'API (constaté en production, différent de la doc) :
// POST /subscriptions crée l'abonnement directement « active », SANS
// redirection Stripe Checkout, le premier prélèvement tombant à
// next_billing_date. Le statut « active » ne prouve donc AUCUN encaissement.
//
// Sécurité : la clé secrète ne quitte jamais le serveur (même client
// geniusPayCall que les paiements) ; un événement d'abonnement NON signé ne
// produit un effet QUE re-vérifié à la source (factures GeniusPay réelles) ;
// le montant débité est confronté au montant souscrit ; l'application d'une
// facture est idempotente (paid_at ≤ last_invoice_at → no-op).
//
// Les routes console (POST/GET/cancel /api/subscription/stripe) sont dans
// handlers_subscription_stripe.go ; l'aiguillage du webhook public est dans
// handlers_geniuspay.go.
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"mikcloud/hotspot-api/internal/model"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
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
