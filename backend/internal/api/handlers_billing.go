// Package api — file des demandes de renouvellement + encaissement Wave
// (console plateforme, facturation SaaS MikCloud).
//
// Le verrou du cycle de facturation (POST /api/subscription) transforme la
// souscription client en DEMANDE tracée dans le journal. Ce fichier ajoute :
//   - la FILE actionnable : chaque demande porte un statut (pending | done |
//     cancelled) et une référence de paiement (MC-XXXXXXXX) ;
//   - la résolution par la plateforme (encaisser & activer / rejeter) ;
//   - le webhook Wave (POST /api/webhooks/wave) qui, authentifié par secret,
//     apparie un paiement à une référence et automatise encaissement +
//     activation.
package api

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"mikcloud/hotspot-api/internal/model"
)

// ---------------------------------------------------------------------------
// Référence de paiement
// ---------------------------------------------------------------------------

// newPayRef — référence de paiement publique (MC-XXXXXXXX, 4 octets aléatoires)
// renvoyée au client et attendue dans le webhook Wave. Unicité garantie par le
// hasard puis vérifiée à l'insertion.
func newPayRef(db *model.DB) string {
	for {
		b := make([]byte, 4)
		if _, err := rand.Read(b); err != nil {
			return "MC-" + fmt.Sprintf("%08X", time.Now().UnixNano()&0xFFFFFFFF)
		}
		ref := "MC-" + strings.ToUpper(hex.EncodeToString(b))
		dup := false
		for i := range db.BillingRequests {
			if db.BillingRequests[i].Ref == ref {
				dup = true
				break
			}
		}
		if !dup {
			return ref
		}
	}
}

// ---------------------------------------------------------------------------
// GET /api/admin/billing-requests — file des demandes (console plateforme)
// ---------------------------------------------------------------------------

// billingRequestItem — demande enrichie du nom/statut du compte client.
type billingRequestItem struct {
	model.BillingRequest
	AccountName   string `json:"accountName"`
	AccountStatus string `json:"accountStatus"`
}

// handleAdminBillingRequests — liste des demandes de souscription /
// renouvellement. Tri : EN ATTENTE d'abord (les plus anciennes en tête — file
// de travail), puis l'historique résolu (les plus récents en tête, plafonné).
func (a *API) handleAdminBillingRequests(w http.ResponseWriter, r *http.Request) {
	if !isPlatformAdmin(r) {
		writeErrCode(w, http.StatusForbidden, "forbidden", "Réservé aux administrateurs de la plateforme", nil)
		return
	}
	a.store.Lock()
	db := a.store.Data()

	accName := map[string]string{}
	accStatus := map[string]string{}
	for i := range db.Accounts {
		accName[db.Accounts[i].ID] = db.Accounts[i].Name
		accStatus[db.Accounts[i].ID] = db.Accounts[i].Status
	}

	items := make([]billingRequestItem, 0, len(db.BillingRequests))
	for _, br := range db.BillingRequests {
		items = append(items, billingRequestItem{
			BillingRequest: br,
			AccountName:    accName[br.AccountID],
			AccountStatus:  accStatus[br.AccountID],
		})
	}
	a.store.Unlock()

	pending := []billingRequestItem{}
	resolved := []billingRequestItem{}
	for _, it := range items {
		if it.Status == "pending" {
			pending = append(pending, it)
		} else {
			resolved = append(resolved, it)
		}
	}
	sort.Slice(pending, func(i, j int) bool { return pending[i].CreatedAt < pending[j].CreatedAt })
	sort.Slice(resolved, func(i, j int) bool { return resolved[i].ResolvedAt > resolved[j].ResolvedAt })
	if len(resolved) > 150 {
		resolved = resolved[:150]
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"requests": append(pending, resolved...),
		"pending":  len(pending),
	})
}

// ---------------------------------------------------------------------------
// POST /api/admin/billing-requests/{id}/resolve — encaisser & activer / rejeter
// ---------------------------------------------------------------------------

// handleAdminBillingRequestResolve — résout une demande en attente :
//   - action « activate » : encaisse (markPaid, défaut vrai) et active la
//     période exactement comme la fiche client (mêmes règles partagées via
//     applySubscriptionLocked) puis passe la demande à « done » ;
//   - action « cancel » : rejette la demande (statut « cancelled »), sans
//     toucher à l'abonnement.
func (a *API) handleAdminBillingRequestResolve(w http.ResponseWriter, r *http.Request) {
	if !isPlatformAdmin(r) {
		writeErrCode(w, http.StatusForbidden, "forbidden", "Réservé aux administrateurs de la plateforme", nil)
		return
	}
	var req struct {
		Action   string `json:"action"` // activate | cancel
		MarkPaid bool   `json:"markPaid"`
		Months   int    `json:"months"` // optionnel (1-36), défaut selon la formule
		Slots    int    `json:"slots"`  // optionnel (essentiel), défaut assiette demande/actuelle
		Note     string `json:"note"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErrCode(w, http.StatusBadRequest, "bad_request", "Corps de requête invalide", nil)
		return
	}
	action := strings.ToLower(strings.TrimSpace(req.Action))
	if action != "activate" && action != "cancel" {
		writeErrCode(w, http.StatusBadRequest, "bad_action", "Action inconnue (activate | cancel)", nil)
		return
	}

	a.store.Lock()
	db := a.store.Data()
	idx := -1
	for i := range db.BillingRequests {
		if db.BillingRequests[i].ID == r.PathValue("id") {
			idx = i
			break
		}
	}
	if idx < 0 {
		a.store.Unlock()
		writeErrCode(w, http.StatusNotFound, "not_found", "Demande introuvable", nil)
		return
	}
	br := db.BillingRequests[idx]
	if br.Status != "pending" {
		a.store.Unlock()
		writeErrCode(w, http.StatusConflict, "already_resolved", "Demande déjà traitée", nil)
		return
	}
	// Le compte doit exister pour une activation.
	accFound := false
	for i := range db.Accounts {
		if db.Accounts[i].ID == br.AccountID {
			accFound = true
			break
		}
	}
	if !accFound {
		a.store.Unlock()
		writeErrCode(w, http.StatusNotFound, "not_found", "Compte client introuvable (supprimé ?)", nil)
		return
	}

	resolvedBy := "plateforme"
	if c := claimsFrom(r); c != nil && strings.TrimSpace(c.Name) != "" {
		resolvedBy = c.Name
	}
	now := time.Now().UTC()

	if action == "cancel" {
		db.BillingRequests[idx].Status = "cancelled"
		db.BillingRequests[idx].ResolvedAt = now.Format(time.RFC3339)
		db.BillingRequests[idx].ResolvedBy = resolvedBy
		db.BillingRequests[idx].Note = strings.TrimSpace(req.Note)
		a.logActivityBy(r, db, br.AccountID, "billing",
			fmt.Sprintf("Demande de renouvellement %s rejetée par la plateforme (%s, %d FCFA)",
				br.Ref, br.PlanName, br.AmountFcfa))
		a.store.Save()
		a.store.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{"request": db.BillingRequests[idx]})
		return
	}

	// action == « activate » : mêmes règles que la fiche client (source unique).
	months := req.Months
	if months <= 0 {
		months = 1
		if br.PlanID == "illimite" {
			months = 12
		}
	}
	slots := req.Slots
	if slots <= 0 {
		slots = br.RouterCount
		if rc := accountRouterCount(db, br.AccountID); rc > slots {
			slots = rc
		}
		if slots < 1 {
			slots = 1
		}
	}
	sub, label, _, amount := applySubscriptionLocked(db, br.AccountID, br.PlanID, months, slots, req.MarkPaid)

	db.BillingRequests[idx].Status = "done"
	db.BillingRequests[idx].ResolvedAt = now.Format(time.RFC3339)
	db.BillingRequests[idx].ResolvedBy = resolvedBy
	db.BillingRequests[idx].PaidVia = "manual"
	db.BillingRequests[idx].Note = strings.TrimSpace(req.Note)

	paid := ""
	if req.MarkPaid {
		paid = " encaissé"
	}
	a.logActivityBy(r, db, br.AccountID, "billing",
		fmt.Sprintf("Demande de renouvellement %s traitée par la plateforme — abonnement %s activé%s (%d FCFA / %s)",
			br.Ref, label, paid, amount, periodLabelOf(br.PlanID)))
	a.store.Save()
	a.store.Unlock()

	status := subscriptionStatus(sub, time.Now().UTC())
	writeJSON(w, http.StatusOK, map[string]any{
		"request": db.BillingRequests[idx],
		"subscription": map[string]any{
			"planId": sub.PlanID, "planName": label, "status": status,
			"periodStart": sub.PeriodStart, "periodEnd": sub.PeriodEnd,
			"lastAmountFcfa": sub.LastAmountFcfa, "routerSlots": sub.RouterSlots,
			"lastPaidAt": sub.LastPaidAt, "amountFcfa": amount,
		},
	})
}

// ---------------------------------------------------------------------------
// POST /api/webhooks/wave — encaissement automatique (public + secret)
// ---------------------------------------------------------------------------

// handleWaveWebhook — réception des notifications de paiement Wave. La route
// est publique (pas de JWT : appelée par les serveurs Wave) mais protégée par
// WAVE_WEBHOOK_SECRET :
//   - soit « Authorization: Bearer <secret> » (simple, configuré côté Wave) ;
//   - soit « X-Wave-Signature: hex(hmac-sha256(corps, secret)) ».
//
// Corps tolérant (plat ou imbriqué Wave) : {ref|reference, amount, status|type}
// — status succès : success | succeeded | paid | completed | captured ;
// échec : failed | cancelled | expired | declined. Le statut succès apparie la
// demande EN ATTENTE par référence, encaisse et active la période (mêmes
// règles que la plateforme) ; l'échec rejette la demande.
func (a *API) handleWaveWebhook(w http.ResponseWriter, r *http.Request) {
	secret := strings.TrimSpace(os.Getenv("WAVE_WEBHOOK_SECRET"))
	if secret == "" {
		writeErrCode(w, http.StatusServiceUnavailable, "webhook_disabled",
			"Webhook Wave désactivé (WAVE_WEBHOOK_SECRET non configuré)", nil)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeErrCode(w, http.StatusBadRequest, "bad_request", "Corps illisible", nil)
		return
	}
	// Authentification : Bearer OU signature HMAC (comparaison à temps constant).
	authed := false
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		authed = subtle.ConstantTimeCompare([]byte(strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))), []byte(secret)) == 1
	}
	if !authed {
		if sig := strings.TrimSpace(r.Header.Get("X-Wave-Signature")); sig != "" {
			mac := hmac.New(sha256.New, []byte(secret))
			mac.Write(body)
			authed = hmac.Equal([]byte(strings.ToLower(sig)), []byte(hex.EncodeToString(mac.Sum(nil))))
		}
	}
	if !authed {
		writeErrCode(w, http.StatusUnauthorized, "unauthorized", "Signature webhook invalide", nil)
		return
	}

	var payload struct {
		Ref       string  `json:"ref"`
		Reference string  `json:"reference"`
		Amount    float64 `json:"amount"`
		Status    string  `json:"status"`
		Type      string  `json:"type"`
		Data      *struct {
			Ref       string  `json:"ref"`
			Reference string  `json:"reference"`
			Amount    float64 `json:"amount"`
			Status    string  `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		writeErrCode(w, http.StatusBadRequest, "bad_request", "JSON invalide", nil)
		return
	}
	ref := strings.TrimSpace(payload.Ref)
	amount := payload.Amount
	status := strings.TrimSpace(payload.Status)
	if ref == "" {
		ref = strings.TrimSpace(payload.Reference)
	}
	if payload.Data != nil {
		if ref == "" {
			ref = strings.TrimSpace(payload.Data.Ref)
			if ref == "" {
				ref = strings.TrimSpace(payload.Data.Reference)
			}
		}
		if amount == 0 {
			amount = payload.Data.Amount
		}
		if status == "" {
			status = strings.TrimSpace(payload.Data.Status)
		}
	}
	if status == "" {
		status = strings.TrimSpace(payload.Type)
	}
	if ref == "" {
		writeErrCode(w, http.StatusBadRequest, "missing_ref", "Référence de paiement absente", nil)
		return
	}

	success := false
	failure := false
	switch strings.ToLower(status) {
	case "success", "succeeded", "paid", "completed", "complete", "done", "captured", "transfered", "transferred":
		success = true
	case "failed", "error", "cancelled", "canceled", "expired", "declined", "rejected":
		failure = true
	default:
		if strings.Contains(strings.ToLower(status), "complet") || strings.Contains(strings.ToLower(status), "succe") {
			success = true
		}
	}
	if !success && !failure {
		writeErrCode(w, http.StatusBadRequest, "unknown_status", "Statut de paiement inconnu : "+status, nil)
		return
	}

	a.store.Lock()
	db := a.store.Data()
	idx := -1
	for i := range db.BillingRequests {
		if strings.EqualFold(db.BillingRequests[i].Ref, ref) {
			idx = i
			break
		}
	}
	if idx < 0 {
		a.store.Unlock()
		writeErrCode(w, http.StatusNotFound, "unknown_ref", "Aucune demande pour la référence "+ref, nil)
		return
	}
	br := db.BillingRequests[idx]
	if br.Status != "pending" {
		a.store.Unlock()
		// Idempotence : un webhook rejoué ne doit pas réactiver ni erreur 5xx.
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "ref": ref, "status": br.Status, "idempotent": true})
		return
	}
	if amount > 0 && int(amount) != br.AmountFcfa {
		a.store.Unlock()
		writeErrCode(w, http.StatusConflict, "amount_mismatch",
			fmt.Sprintf("Montant incohérent : reçu %.0f FCFA, attendu %d FCFA (référence %s)", amount, br.AmountFcfa, ref), nil)
		return
	}

	now := time.Now().UTC()
	if failure {
		db.BillingRequests[idx].Status = "cancelled"
		db.BillingRequests[idx].ResolvedAt = now.Format(time.RFC3339)
		db.BillingRequests[idx].ResolvedBy = "webhook Wave"
		db.BillingRequests[idx].Note = "Paiement Wave échoué (" + status + ")"
		a.logActivity(db, br.AccountID, "billing",
			fmt.Sprintf("Paiement Wave échoué — demande de renouvellement %s annulée (%s, %d FCFA)", ref, br.PlanName, br.AmountFcfa))
		a.store.Save()
		a.store.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "ref": ref, "status": "cancelled"})
		return
	}

	// Succès : activation aux règles de la plateforme (source unique) puis
	// clôture de la demande (PaidVia wave — encaissement automatique).
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
	db.BillingRequests[idx].Status = "done"
	db.BillingRequests[idx].ResolvedAt = now.Format(time.RFC3339)
	db.BillingRequests[idx].ResolvedBy = "webhook Wave"
	db.BillingRequests[idx].PaidVia = "wave"
	a.logActivity(db, br.AccountID, "billing",
		fmt.Sprintf("Paiement Wave confirmé — demande %s encaissée, abonnement %s activé (%d FCFA / %s)",
			ref, label, applied, periodLabelOf(br.PlanID)))
	a.store.Save()
	a.store.Unlock()

	statusNow := subscriptionStatus(sub, time.Now().UTC())
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "ref": ref, "status": "done",
		"subscription": map[string]any{
			"planId": sub.PlanID, "planName": label, "status": statusNow,
			"periodStart": sub.PeriodStart, "periodEnd": sub.PeriodEnd,
		},
	})
}

// periodLabelOf — libellé de période d'une formule du catalogue.
func periodLabelOf(planID string) string {
	if planID == "illimite" {
		return "1 an"
	}
	return "1 mois"
}
