// Package api — paiement Wave EN LIGNE via GeniusPay (encaissement SaaS).
//
// Complète le verrou du cycle de facturation (POST /api/subscription →
// demande EN ATTENTE, activation réservée à la plateforme) par un
// encaissement automatique :
//   - POST /api/subscription/pay : le client initie le paiement de sa demande
//     en attente par Wave (API marchand GeniusPay, mode direct
//     payment_method=wave) et est redirigé vers la page de paiement ;
//   - POST /api/webhooks/geniuspay : webhook public signé (HMAC-SHA256 sur
//     « <timestamp>.<corps> » + anti-replay 5 min) qui apparie la transaction
//     à la demande (metadata.ref, repli référence marchande) puis encaisse et
//     active la période exactement comme la plateforme (source unique
//     applySubscriptionLocked) ;
//   - GET /api/subscription/pay/status : filet de sécurité — consulte le
//     statut réel de la transaction chez GeniusPay et finalise l'activation
//     si le webhook n'est pas encore arrivé (client de retour de Wave).
//   - POST /api/subscription/stripe (+ /cancel, GET statut) et webhook
//     subscription.* : abonnement RÉCURRENT par carte bancaire (Stripe via
//     GeniusPay) — implémenté dans handlers_geniuspay_stripe.go.
//
// Sécurité : la clé secrète (GENIUSPAY_API_SECRET) ne quitte jamais le
// serveur ; le webhook est vérifié par signature à temps constant ; le
// montant reçu est confronté au montant attendu de la demande ; l'appariement
// est idempotent (webhook rejoué sans effet). Garde P3 : initier un paiement
// reste possible pour un compte expiré OU suspendu — c'est la seule sortie
// de ces états (le middleware autorise /api/subscription/pay*).
package api

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"mikcloud/hotspot-api/internal/model"
)

// Base de l'API marchand GeniusPay (production — clés pk_live_/sk_live_).
const geniusPayAPIBase = "https://geniuspay.ci/api/v1/merchant"

// geniusPayHTTP — client partagé (timeout serré : appel synchrone côté client).
var geniusPayHTTP = &http.Client{Timeout: 20 * time.Second}

// ---------------------------------------------------------------------------
// Types API marchand GeniusPay
// ---------------------------------------------------------------------------

// geniusPayCustomer — client du paiement (nom + téléphone international ;
// e-mail requis par le module carte/Stripe).
type geniusPayCustomer struct {
	Name  string `json:"name,omitempty"`
	Phone string `json:"phone,omitempty"`
	Email string `json:"email,omitempty"`
}

// geniusPayCreateReq — corps POST /payments (mode direct : payment_method=wave).
type geniusPayCreateReq struct {
	Amount        int                `json:"amount"` // XOF (min 200)
	PaymentMethod string             `json:"payment_method,omitempty"`
	Description   string             `json:"description,omitempty"`
	Customer      *geniusPayCustomer `json:"customer,omitempty"`
	SuccessURL    string             `json:"success_url,omitempty"`
	ErrorURL      string             `json:"error_url,omitempty"`
	// Metadata — retournées telles quelles dans les réponses et webhooks :
	// c'est le lien transaction <-> demande de facturation (ref MC-…).
	Metadata map[string]string `json:"metadata,omitempty"`
}

// geniusPayPayment — transaction renvoyée par POST /payments et GET /payments/{ref}.
type geniusPayPayment struct {
	Reference     string         `json:"reference"`
	Amount        float64        `json:"amount"`
	Status        string         `json:"status"` // pending|processing|completed|failed|expired|cancelled|refunded
	PaymentMethod string         `json:"payment_method"`
	Provider      string         `json:"provider"`
	PaymentURL    string         `json:"payment_url"`
	CheckoutURL   string         `json:"checkout_url"`
	Environment   string         `json:"environment"`
	Metadata      map[string]any `json:"metadata"`
}

// geniusPayEnvelope — enveloppe commune {success, data, error}.
type geniusPayEnvelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// geniusPayCall — appel authentifié à l'API marchand (headers X-API-Key /
// X-API-Secret). Renvoie l'enveloppe décodée ou une erreur lisible (réseau,
// statut HTTP, code marchand). La clé secrète reste côté serveur.
func geniusPayCall(method, path string, reqBody any, extra ...[]string) (*geniusPayEnvelope, error) {
	key := strings.TrimSpace(os.Getenv("GENIUSPAY_API_KEY"))
	secret := strings.TrimSpace(os.Getenv("GENIUSPAY_API_SECRET"))
	if key == "" || secret == "" {
		return nil, errors.New("paiement en ligne non configuré (clés GeniusPay absentes)")
	}
	var rd io.Reader
	if reqBody != nil {
		b, err := json.Marshal(reqBody)
		if err != nil {
			return nil, err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, geniusPayAPIBase+path, rd)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-API-Key", key)
	req.Header.Set("X-API-Secret", secret)
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for _, kv := range extra {
		if len(kv) == 2 {
			req.Header.Set(kv[0], kv[1])
		}
	}
	res, err := geniusPayHTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GeniusPay injoignable : %w", err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("réponse GeniusPay illisible : %w", err)
	}
	var env geniusPayEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("réponse GeniusPay invalide (HTTP %d)", res.StatusCode)
	}
	if res.StatusCode >= 400 || !env.Success {
		msg := "erreur GeniusPay"
		if env.Error != nil && strings.TrimSpace(env.Error.Message) != "" {
			msg = env.Error.Message
		}
		return &env, fmt.Errorf("%s (HTTP %d)", msg, res.StatusCode)
	}
	return &env, nil
}

// geniusPayGetPayment — détail d'une transaction par sa référence marchande.
func geniusPayGetPayment(reference string) (*geniusPayPayment, error) {
	env, err := geniusPayCall(http.MethodGet, "/payments/"+url.PathEscape(reference), nil)
	if err != nil {
		return nil, err
	}
	var p geniusPayPayment
	if len(env.Data) == 0 || json.Unmarshal(env.Data, &p) != nil {
		return nil, errors.New("réponse GeniusPay inattendue")
	}
	return &p, nil
}

// normalizeGeniusPayPhone — le téléphone du compte est stocké en E.164 sans
// « + » (ex. 2250701020304) ; GeniusPay attend un préfixe international
// (« +2250701020304 »).
func normalizeGeniusPayPhone(phone string) string {
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, strings.TrimSpace(phone))
	if digits == "" {
		return ""
	}
	return "+" + digits
}

// ---------------------------------------------------------------------------
// POST /api/subscription/pay — initiation du paiement Wave (client)
// ---------------------------------------------------------------------------

// handleSubscriptionPay — initie le paiement Wave de la demande EN ATTENTE du
// compte via GeniusPay. Une seule demande en attente existe par compte
// (garanti par POST /api/subscription). Chaque appel crée une NOUVELLE
// transaction marchande (la précédente non réglée expire d'elle-même côté
// GeniusPay) — l'activation reste unique grâce à l'idempotence de la demande.
// Réponse : {ref, gatewayRef, paymentUrl, amountFcfa, status:"pending"}.
func (a *API) handleSubscriptionPay(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	if c := claimsFrom(r); c != nil && c.Acc == "" {
		// Admin plateforme (sans compte client) : pas d'auto-souscription.
		writeErrCode(w, http.StatusForbidden, "forbidden", "Réservé aux comptes clients — la plateforme gère les abonnements", nil)
		return
	}
	if _, _, ok := geniusPayKeys(); !ok {
		writeErrCode(w, http.StatusServiceUnavailable, "geniuspay_disabled",
			"Paiement en ligne non configuré — contactez l'équipe MikCloud", nil)
		return
	}

	a.store.Lock()
	db := a.store.Data()
	idx := -1
	for i := range db.BillingRequests {
		if db.BillingRequests[i].AccountID == acc && db.BillingRequests[i].Status == "pending" {
			if idx < 0 || db.BillingRequests[i].CreatedAt > db.BillingRequests[idx].CreatedAt {
				idx = i
			}
		}
	}
	if idx < 0 {
		a.store.Unlock()
		writeErrCode(w, http.StatusNotFound, "no_pending_request",
			"Aucune demande d'abonnement en attente — choisissez d'abord une formule", nil)
		return
	}
	br := db.BillingRequests[idx]
	phone, ownerName := "", ""
	for i := range db.Accounts {
		if db.Accounts[i].ID == acc {
			phone = normalizeGeniusPayPhone(db.Accounts[i].Phone)
			ownerName = db.Accounts[i].Name
			break
		}
	}
	if phone == "" {
		a.store.Unlock()
		writeErrCode(w, http.StatusUnprocessableEntity, "missing_phone",
			"Renseignez votre numéro de téléphone (profil) pour payer par Wave", nil)
		return
	}
	ref, amount, planID, planName, periodLabel := br.Ref, br.AmountFcfa, br.PlanID, br.PlanName, br.PeriodLabel
	a.store.Unlock()

	// Transaction GeniusPay — mode direct Wave, metadata = appariement webhook.
	payload := geniusPayCreateReq{
		Amount:        amount,
		PaymentMethod: "wave",
		Description:   fmt.Sprintf("Abonnement MikCloud %s (%s) — réf %s", planName, periodLabel, ref),
		Customer:      &geniusPayCustomer{Name: ownerName, Phone: phone},
		Metadata:      map[string]string{"ref": ref, "account_id": acc, "plan_id": planID},
	}
	if appURL := strings.TrimRight(strings.TrimSpace(os.Getenv("APP_PUBLIC_URL")), "/"); appURL != "" {
		payload.SuccessURL = appURL + "/"
		payload.ErrorURL = appURL + "/"
	}
	env, err := geniusPayCall(http.MethodPost, "/payments", payload)
	if err != nil || env == nil {
		writeErrCode(w, http.StatusBadGateway, "geniuspay_error",
			"Initiation du paiement impossible — "+err.Error(), nil)
		return
	}
	var pay geniusPayPayment
	if len(env.Data) == 0 || json.Unmarshal(env.Data, &pay) != nil || strings.TrimSpace(pay.Reference) == "" {
		writeErrCode(w, http.StatusBadGateway, "geniuspay_error",
			"Réponse GeniusPay inattendue (référence de transaction absente)", nil)
		return
	}

	// Enregistrement de la référence marchande sur la demande (ré-ouverte sous
	// verrou : la demande peut avoir été résolue entre-temps).
	a.store.Lock()
	db = a.store.Data()
	for i := range db.BillingRequests {
		if db.BillingRequests[i].ID == br.ID {
			if db.BillingRequests[i].Status == "pending" {
				db.BillingRequests[i].GatewayRef = strings.TrimSpace(pay.Reference)
				a.logActivity(db, acc, "billing",
					fmt.Sprintf("Paiement Wave initié via GeniusPay — demande %s (%d FCFA), transaction %s",
						ref, amount, pay.Reference))
				a.store.Save()
			}
			break
		}
	}
	a.store.Unlock()

	payURL := pay.PaymentURL
	if payURL == "" {
		payURL = pay.CheckoutURL // repli : page de checkout hébergée GeniusPay
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ref":        ref,
		"gatewayRef": pay.Reference,
		"paymentUrl": payURL,
		"amountFcfa": amount,
		"status":     "pending",
	})
}

// geniusPayKeys — clés API marchand configurées ?
func geniusPayKeys() (key, secret string, ok bool) {
	key = strings.TrimSpace(os.Getenv("GENIUSPAY_API_KEY"))
	secret = strings.TrimSpace(os.Getenv("GENIUSPAY_API_SECRET"))
	return key, secret, key != "" && secret != ""
}

// ---------------------------------------------------------------------------
// GET /api/subscription/pay/status — filet de sécurité (client de retour)
// ---------------------------------------------------------------------------

// handleSubscriptionPayStatus — état du paiement de la demande la plus
// récente du compte. Si une transaction GeniusPay est connue, son statut RÉEL
// est consulté : completed → activation immédiate (le webhook peut avoir été
// retardé) ; failed/expired/cancelled → demande annulée ; sinon « pending ».
func (a *API) handleSubscriptionPayStatus(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	if c := claimsFrom(r); c != nil && c.Acc == "" {
		writeErrCode(w, http.StatusForbidden, "forbidden", "Réservé aux comptes clients", nil)
		return
	}

	a.store.Lock()
	db := a.store.Data()
	idx := -1
	for i := range db.BillingRequests {
		if db.BillingRequests[i].AccountID == acc {
			if idx < 0 || db.BillingRequests[i].CreatedAt > db.BillingRequests[idx].CreatedAt {
				idx = i
			}
		}
	}
	if idx < 0 {
		a.store.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{"status": "none"})
		return
	}
	br := db.BillingRequests[idx]
	switch {
	case br.Status == "done":
		a.store.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{"status": "done", "ref": br.Ref})
		return
	case br.Status == "cancelled":
		a.store.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{"status": "cancelled", "ref": br.Ref})
		return
	case br.GatewayRef == "":
		// Demande en attente sans transaction initiée.
		a.store.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{"status": "pending", "ref": br.Ref})
		return
	}
	gatewayRef, ref := br.GatewayRef, br.Ref
	a.store.Unlock()

	// Statut réel auprès de GeniusPay (hors verrou : appel réseau).
	pay, err := geniusPayGetPayment(gatewayRef)
	if err != nil {
		// GeniusPay injoignable : la demande reste en attente, sans échec côté client.
		writeJSON(w, http.StatusOK, map[string]any{"status": "pending", "ref": ref, "gatewayRef": gatewayRef})
		return
	}
	switch strings.ToLower(strings.TrimSpace(pay.Status)) {
	case "completed", "complete", "succeeded", "success", "paid", "captured":
		if a.finalizePayRequest(acc, ref, gatewayRef) {
			writeJSON(w, http.StatusOK, map[string]any{"status": "done", "ref": ref, "gatewayRef": gatewayRef})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "pending", "ref": ref, "gatewayRef": gatewayRef})
	case "failed", "cancelled", "canceled", "expired", "declined", "rejected":
		if a.cancelPayRequest(acc, ref, "Paiement Wave non abouti ("+pay.Status+")") {
			writeJSON(w, http.StatusOK, map[string]any{"status": "cancelled", "ref": ref, "gatewayRef": gatewayRef})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "pending", "ref": ref, "gatewayRef": gatewayRef})
	default:
		writeJSON(w, http.StatusOK, map[string]any{"status": "pending", "ref": ref, "gatewayRef": gatewayRef})
	}
}

// finalizePayRequest — encaisse et active la demande identifiée par sa
// référence (résolution par le paiement en ligne). Idempotent : renvoie false
// si la demande n'est plus en attente.
func (a *API) finalizePayRequest(acc, ref, gatewayRef string) bool {
	a.store.Lock()
	db := a.store.Data()
	idx := -1
	for i := range db.BillingRequests {
		if db.BillingRequests[i].AccountID == acc && strings.EqualFold(db.BillingRequests[i].Ref, ref) {
			idx = i
			break
		}
	}
	if idx < 0 || db.BillingRequests[idx].Status != "pending" {
		a.store.Unlock()
		return false
	}
	db.BillingRequests[idx].GatewayRef = gatewayRef
	br := db.BillingRequests[idx]
	_, label, applied := a.finalizeBillingSuccess(db, idx, "geniuspay", "paiement en ligne", "")
	a.logActivity(db, br.AccountID, "billing",
		fmt.Sprintf("Paiement Wave confirmé via GeniusPay — demande %s encaissée, abonnement %s activé (%d FCFA / %s)",
			br.Ref, label, applied, periodLabelOf(br.PlanID)))
	a.store.Save()
	a.store.Unlock()
	return true
}

// cancelPayRequest — annule la demande du compte après échec de paiement.
func (a *API) cancelPayRequest(acc, ref, note string) bool {
	a.store.Lock()
	db := a.store.Data()
	idx := -1
	for i := range db.BillingRequests {
		if db.BillingRequests[i].AccountID == acc && strings.EqualFold(db.BillingRequests[i].Ref, ref) {
			idx = i
			break
		}
	}
	if idx < 0 || db.BillingRequests[idx].Status != "pending" {
		a.store.Unlock()
		return false
	}
	now := time.Now().UTC()
	db.BillingRequests[idx].Status = "cancelled"
	db.BillingRequests[idx].ResolvedAt = now.Format(time.RFC3339)
	db.BillingRequests[idx].ResolvedBy = "paiement en ligne"
	db.BillingRequests[idx].Note = note
	a.logActivity(db, acc, "billing",
		fmt.Sprintf("Paiement Wave non abouti — demande de renouvellement %s annulée", ref))
	a.store.Save()
	a.store.Unlock()
	return true
}

// finalizeBillingSuccess — encaisse et active une demande EN ATTENTE aux
// règles de la plateforme (source unique applySubscriptionLocked), identique
// au webhook Wave. À appeler SOUS VERROU avec idx valide (status pending) ;
// passe la demande à « done » et renvoie (souscription, libellé, montant).
func (a *API) finalizeBillingSuccess(db *model.DB, idx int, paidVia, resolvedBy, note string) (model.Subscription, string, int) {
	br := db.BillingRequests[idx]
	months := 1
	if br.PlanID == "illimite" {
		months = 12
	}
	slots := br.RouterCount
	if rc := accountRouterCount(db, br.AccountID); rc > slots {
		slots = rc
	}
	if slots < 1 {
		slots = 1
	}
	sub, label, _, applied := applySubscriptionLocked(db, br.AccountID, br.PlanID, months, slots, true)
	now := time.Now().UTC()
	db.BillingRequests[idx].Status = "done"
	db.BillingRequests[idx].ResolvedAt = now.Format(time.RFC3339)
	db.BillingRequests[idx].ResolvedBy = resolvedBy
	db.BillingRequests[idx].PaidVia = paidVia
	if note != "" {
		db.BillingRequests[idx].Note = note
	}
	return sub, label, applied
}

// ---------------------------------------------------------------------------
// POST /api/webhooks/geniuspay — confirmation de paiement (public + signé)
// ---------------------------------------------------------------------------

// handleGeniusPayWebhook — réception des notifications GeniusPay. Route
// PUBLIQUE authentifiée par signature HMAC-SHA256 :
//
//	X-Webhook-Signature = hex(HMAC-SHA256("<X-Webhook-Timestamp>.<corps>", GENIUSPAY_WEBHOOK_SECRET))
//
// avec anti-replay (horodatage < 5 min). payment.success → encaisse et active
// la demande appariée (metadata.ref, repli référence marchande) ;
// payment.failed → rejette la demande ; subscription.* → abonnement carte
// (Stripe via GeniusPay, handlers_geniuspay_stripe.go) ; autres → accusé OK.
func (a *API) handleGeniusPayWebhook(w http.ResponseWriter, r *http.Request) {
	whsec := strings.TrimSpace(os.Getenv("GENIUSPAY_WEBHOOK_SECRET"))
	if whsec == "" {
		writeErrCode(w, http.StatusServiceUnavailable, "webhook_disabled",
			"Webhook GeniusPay désactivé (GENIUSPAY_WEBHOOK_SECRET non configuré)", nil)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeErrCode(w, http.StatusBadRequest, "bad_request", "Corps illisible", nil)
		return
	}
	// Anti-replay : l'horodatage d'envoi doit dater de moins de 5 minutes
	// (protection recommandée par GeniusPay).
	tsStr := strings.TrimSpace(r.Header.Get("X-Webhook-Timestamp"))
	ts, tsErr := strconv.ParseInt(tsStr, 10, 64)
	now := time.Now()
	if tsErr != nil || now.Sub(time.Unix(ts, 0)) > 5*time.Minute || time.Until(time.Unix(ts, 0)) > 5*time.Minute {
		writeErrCode(w, http.StatusBadRequest, "stale_timestamp",
			"Horodatage webhook invalide ou trop ancien", nil)
		return
	}
	// Signature à temps constant sur « <timestamp>.<corps brut> ». Les
	// PAIEMENTS (montant engagé) exigent une signature valide — absente ou
	// invalide → rejet. Les événements d'ABONNEMENT carte (module Stripe de
	// GeniusPay, secrétariat de signature potentiellement distinct) sont
	// tolérés sans signature valide MAIS ne produisent alors un effet qu'
	// après re-vérification à la source (factures réelles chez GeniusPay).
	var peek struct {
		Event string `json:"event"`
	}
	_ = json.Unmarshal(body, &peek)
	isSubEvent := strings.HasPrefix(strings.TrimSpace(peek.Event), "subscription.")

	mac := hmac.New(sha256.New, []byte(whsec))
	mac.Write([]byte(tsStr + "."))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	sig := strings.ToLower(strings.TrimSpace(r.Header.Get("X-Webhook-Signature")))
	valid := sig != "" && subtle.ConstantTimeCompare([]byte(sig), []byte(expected)) == 1
	if sig != "" && !valid && !isSubEvent {
		writeErrCode(w, http.StatusUnauthorized, "unauthorized", "Signature webhook invalide", nil)
		return
	}

	var payload struct {
		ID          string                `json:"id"`
		Event       string                `json:"event"`
		Environment string                `json:"environment"`
		Data        *geniusPayWebhookData `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		writeErrCode(w, http.StatusBadRequest, "bad_request", "JSON invalide", nil)
		return
	}
	event := strings.TrimSpace(payload.Event)
	switch event {
	case "payment.success", "payment.failed":
		// Paiements : signature OBLIGATOIRE (montant engagé) — déjà exigée
		// ci-dessus (absente ou invalide → rejet avant parse complet).
		if !valid {
			writeErrCode(w, http.StatusUnauthorized, "unauthorized", "Signature webhook manquante", nil)
			return
		}
		// traités ci-dessous
	case "subscription.payment_succeeded":
		a.handleStripeWebhookEvent(w, payload.Data, valid, "payment_succeeded")
		return
	case "subscription.payment_failed", "subscription.past_due", "subscription.cancelled",
		"subscription.expired", "subscription.paused", "subscription.created", "subscription.trial_ending":
		if !valid {
			// État d'abonnement sans signature valide : aucun effet métier —
			// accusé de réception pour arrêter les relivraisons.
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "unsigned_ignored": event})
			return
		}
		a.handleStripeWebhookEvent(w, payload.Data, true, strings.TrimPrefix(event, "subscription."))
		return
	default:
		// payment.initiated / refunded / expired / webhook.test… : aucun
		// effet métier — accusé de réception pour arrêter les relivraisons.
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "ignored": event})
		return
	}

	// Appariement : metadata.ref (MC-…) en priorité — posé à l'initiation —,
	// repli sur la référence marchande enregistrée sur la demande.
	refMeta := ""
	gatewayRef := ""
	amount := 0.0
	if payload.Data != nil {
		if v, ok := payload.Data.Metadata["ref"].(string); ok {
			refMeta = strings.TrimSpace(v)
		}
		gatewayRef = strings.TrimSpace(payload.Data.Reference)
		amount = payload.Data.Amount
	}
	if refMeta == "" && gatewayRef == "" {
		writeErrCode(w, http.StatusBadRequest, "missing_ref", "Référence de paiement absente", nil)
		return
	}

	a.store.Lock()
	db := a.store.Data()
	idx := -1
	if refMeta != "" {
		for i := range db.BillingRequests {
			if strings.EqualFold(db.BillingRequests[i].Ref, refMeta) {
				idx = i
				break
			}
		}
	}
	if idx < 0 && gatewayRef != "" {
		for i := range db.BillingRequests {
			if db.BillingRequests[i].GatewayRef != "" && strings.EqualFold(db.BillingRequests[i].GatewayRef, gatewayRef) {
				idx = i
				break
			}
		}
	}
	if idx < 0 {
		a.store.Unlock()
		writeErrCode(w, http.StatusNotFound, "unknown_ref",
			"Aucune demande pour la référence "+refMeta+gatewayRef, nil)
		return
	}
	br := db.BillingRequests[idx]
	if br.Status != "pending" {
		a.store.Unlock()
		// Idempotence : un webhook rejoué ne réactive pas et ne fait pas d'erreur.
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "ref": br.Ref, "status": br.Status, "idempotent": true})
		return
	}
	// Confrontation du montant reçu au montant attendu de la demande.
	if amount > 0 && int(amount) != br.AmountFcfa {
		a.store.Unlock()
		writeErrCode(w, http.StatusConflict, "amount_mismatch",
			fmt.Sprintf("Montant incohérent : reçu %.0f FCFA, attendu %d FCFA (référence %s)",
				amount, br.AmountFcfa, br.Ref), nil)
		return
	}

	if event == "payment.failed" {
		now := time.Now().UTC()
		db.BillingRequests[idx].Status = "cancelled"
		db.BillingRequests[idx].ResolvedAt = now.Format(time.RFC3339)
		db.BillingRequests[idx].ResolvedBy = "webhook GeniusPay"
		db.BillingRequests[idx].Note = "Paiement Wave échoué (payment.failed)"
		a.logActivity(db, br.AccountID, "billing",
			fmt.Sprintf("Paiement Wave échoué — demande de renouvellement %s annulée (%s, %d FCFA)",
				br.Ref, br.PlanName, br.AmountFcfa))
		a.store.Save()
		a.store.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "ref": br.Ref, "status": "cancelled"})
		return
	}

	// Succès : activation aux règles de la plateforme puis clôture de la
	// demande (PaidVia geniuspay — encaissement automatique).
	db.BillingRequests[idx].GatewayRef = gatewayRef
	sub, label, applied := a.finalizeBillingSuccess(db, idx, "geniuspay", "webhook GeniusPay", "")
	a.logActivity(db, br.AccountID, "billing",
		fmt.Sprintf("Paiement Wave confirmé via GeniusPay — demande %s encaissée, abonnement %s activé (%d FCFA / %s)",
			br.Ref, label, applied, periodLabelOf(br.PlanID)))
	a.store.Save()
	a.store.Unlock()

	statusNow := subscriptionStatus(sub, time.Now().UTC())
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "ref": br.Ref, "status": "done",
		"subscription": map[string]any{
			"planId": sub.PlanID, "planName": label, "status": statusNow,
			"periodStart": sub.PeriodStart, "periodEnd": sub.PeriodEnd,
		},
	})
}
