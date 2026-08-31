// Package api — facturation CLIENT (M) : historique des factures et facture
// téléchargeable.
//
// L'historique expose les demandes RÉSOLUES (status=done) du compte, chacune
// portant un numéro de facture séquentiel annuel (MC-2026-0001…) calculé à
// la volée : les factures d'une année, triées par date de résolution, sont
// numérotées 1..N — stable car une demande « done » ne redevient jamais
// « cancelled ». La facture elle-même est une page HTML print-friendly
// (window.print() → « Enregistrer au format PDF ») : zéro dépendance,
// rendu fidèle, téléchargeable même sur mobile — l'approche des factures
// Stripe.

package api

import (
	"fmt"
	"html"
	"net/http"
	"sort"
	"strings"
	"time"

	"mikcloud/hotspot-api/internal/model"
)

// ---------------------------------------------------------------------------
// Historique de facturation (client)
// ---------------------------------------------------------------------------

// invoiceRow — ligne d'historique côté client (GET /api/billing/history).
type invoiceRow struct {
	ID          string `json:"id"`
	InvoiceNo   string `json:"invoiceNo"` // MC-2026-0001 — séquentiel annuel
	PlanName    string `json:"planName"`
	AmountFcfa  int    `json:"amountFcfa"`
	PeriodLabel string `json:"periodLabel"`
	RouterCount int    `json:"routerCount"`
	Ref         string `json:"ref"`      // référence de paiement (MC-XXXXXXXX)
	PaidVia     string `json:"paidVia"`  // wave | manual
	IssuedAt    string `json:"issuedAt"` // date de résolution (RFC3339)
}

// invoiceNumber — numéro séquentiel annuel d'une facture résolue : les
// demandes « done » de la même année que la facture, triées par ResolvedAt,
// sont numérotées 1..N (format MC-2026-0001).
func invoiceNumber(all []model.BillingRequest, target model.BillingRequest) string {
	year := issuedYear(target)
	sameYear := []model.BillingRequest{}
	for _, br := range all {
		if br.Status == "done" && issuedYear(br) == year {
			sameYear = append(sameYear, br)
		}
	}
	sort.SliceStable(sameYear, func(i, j int) bool {
		ti, _ := time.Parse(time.RFC3339, sameYear[i].ResolvedAt)
		tj, _ := time.Parse(time.RFC3339, sameYear[j].ResolvedAt)
		return ti.Before(tj)
	})
	for i, br := range sameYear {
		if br.ID == target.ID {
			return fmt.Sprintf("MC-%d-%04d", year, i+1)
		}
	}
	return fmt.Sprintf("MC-%d-0001", year)
}

// issuedYear — année de résolution (repli : année de création).
func issuedYear(br model.BillingRequest) int {
	if t, err := time.Parse(time.RFC3339, br.ResolvedAt); err == nil {
		return t.UTC().Year()
	}
	if t, err := time.Parse(time.RFC3339, br.CreatedAt); err == nil {
		return t.UTC().Year()
	}
	return time.Now().UTC().Year()
}

// handleBillingHistory — GET /api/billing/history : factures du compte du
// token (demandes résolues, plus récentes d'abord). Accessible aux comptes
// expirés ET suspendus (consultation de ses propres justificatifs).
func (a *API) handleBillingHistory(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	a.store.Lock()
	db := a.store.Data()
	out := []invoiceRow{}
	for _, br := range db.BillingRequests {
		if br.AccountID != acc || br.Status != "done" {
			continue
		}
		out = append(out, invoiceRow{
			ID:          br.ID,
			InvoiceNo:   invoiceNumber(db.BillingRequests, br),
			PlanName:    br.PlanName,
			AmountFcfa:  br.AmountFcfa,
			PeriodLabel: br.PeriodLabel,
			RouterCount: br.RouterCount,
			Ref:         br.Ref,
			PaidVia:     br.PaidVia,
			IssuedAt:    br.ResolvedAt,
		})
	}
	a.store.Unlock()
	// Plus récentes d'abord.
	sort.SliceStable(out, func(i, j int) bool {
		ti, _ := time.Parse(time.RFC3339, out[i].IssuedAt)
		tj, _ := time.Parse(time.RFC3339, out[j].IssuedAt)
		return ti.After(tj)
	})
	writeJSON(w, http.StatusOK, out)
}

// ---------------------------------------------------------------------------
// Facture HTML print-friendly
// ---------------------------------------------------------------------------

// handleBillingInvoice — GET /api/billing/invoice/{id} : facture complète au
// format HTML imprimable (window.print() → PDF navigateur). Le client peut
// la consulter et la télécharger ; la plateforme (support) y a aussi accès.
func (a *API) handleBillingInvoice(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	id := r.PathValue("id")
	a.store.Lock()
	db := a.store.Data()
	var br *model.BillingRequest
	for i := range db.BillingRequests {
		if db.BillingRequests[i].ID == id && db.BillingRequests[i].AccountID == acc {
			br = &db.BillingRequests[i]
			break
		}
	}
	if br == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Facture introuvable")
		return
	}
	// Données du compte (client facturé) : nom + contact du propriétaire.
	var accnt *model.Account
	for i := range db.Accounts {
		if db.Accounts[i].ID == acc {
			accnt = &db.Accounts[i]
			break
		}
	}
	settings := ensureSettings(db, acc)
	ownerName, ownerEmail, ownerPhone := "", "", ""
	if accnt != nil {
		ownerName = accnt.Name
		ownerEmail, ownerPhone = accnt.Email, accnt.Phone
	}
	_ = settings
	a.store.Unlock()

	invoiceNo := invoiceNumber(db.BillingRequests, *br)
	issued := br.ResolvedAt
	if t, err := time.Parse(time.RFC3339, br.ResolvedAt); err == nil {
		issued = t.Format("02/01/2006")
	}
	paidVia := "Wave"
	if br.PaidVia == "manual" {
		paidVia = "Versement plateforme"
	}

	// Facture HTML : styles inline (impression A4), typographie sobre.
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="fr">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Facture %s — MikCloud</title>
<style>
  :root { color-scheme: light; }
  * { box-sizing: border-box; margin: 0; padding: 0; }
  body { font-family: -apple-system, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif; color: #1a2b26; background: #f4f7f5; padding: 24px; }
  .sheet { max-width: 720px; margin: 0 auto; background: #fff; border-radius: 12px; padding: 40px; box-shadow: 0 2px 12px rgba(0,0,0,.06); }
  header { display: flex; justify-content: space-between; align-items: flex-start; gap: 24px; border-bottom: 3px solid #16a34a; padding-bottom: 20px; margin-bottom: 24px; }
  .brand { font-size: 26px; font-weight: 800; color: #14532d; }
  .brand small { display: block; font-size: 12px; font-weight: 500; color: #4b6358; margin-top: 4px; line-height: 1.5; }
  .meta { text-align: right; font-size: 13px; color: #4b6358; line-height: 1.6; }
  .meta b { color: #1a2b26; font-size: 15px; }
  h1 { font-size: 18px; margin: 24px 0 16px; color: #14532d; }
  .parties { display: flex; justify-content: space-between; gap: 24px; margin-bottom: 24px; }
  .party { font-size: 13px; line-height: 1.7; color: #33453e; }
  .party h2 { font-size: 11px; text-transform: uppercase; letter-spacing: .08em; color: #7a9187; margin-bottom: 6px; }
  table { width: 100%%; border-collapse: collapse; margin: 16px 0 24px; font-size: 14px; }
  th { text-align: left; padding: 10px 12px; background: #eef5f1; color: #14532d; font-size: 12px; text-transform: uppercase; letter-spacing: .05em; }
  td { padding: 12px; border-bottom: 1px solid #e6ede9; }
  .total td { border-top: 2px solid #14532d; border-bottom: none; font-size: 17px; font-weight: 700; padding-top: 14px; }
  .total .amount { color: #14532d; }
  .amount { text-align: right; font-variant-numeric: tabular-nums; }
  .foot { margin-top: 28px; padding-top: 16px; border-top: 1px solid #e6ede9; font-size: 12px; color: #7a9187; line-height: 1.7; }
  .badge { display: inline-block; padding: 4px 10px; border-radius: 999px; background: #dcfce7; color: #14532d; font-size: 12px; font-weight: 600; }
  @media print {
    body { background: #fff; padding: 0; }
    .sheet { box-shadow: none; border-radius: 0; max-width: none; padding: 24px; }
    .noprint { display: none; }
  }
</style>
</head>
<body>
<div class="sheet">
  <header>
    <div class="brand">MikCloud<small>Freelance Technologies (FTCI)<br>freelancetechnologies.ci@gmail.com<br>Abidjan, Côte d'Ivoire</small></div>
    <div class="meta"><b>Facture %s</b><br>Émise le %s<br><span class="badge">Payée — %s</span></div>
  </header>

  <h1>Facturé à</h1>
  <div class="parties">
    <div class="party">
      <h2>Client</h2>
      <b>%s</b><br>
      %s<br>
      %s
    </div>
    <div class="party">
      <h2>Règlement</h2>
      Référence paiement : <b>%s</b><br>
      Mode : %s<br>
      Résolue par : %s
    </div>
  </div>

  <table>
    <thead><tr><th>Désignation</th><th>Période</th><th class="amount">Montant</th></tr></thead>
    <tbody>
      <tr>
        <td>Abonnement MikCloud %s%s</td>
        <td>%s</td>
        <td class="amount">%s FCFA</td>
      </tr>
      <tr class="total"><td>Total payé</td><td></td><td class="amount">%s FCFA</td></tr>
    </tbody>
  </table>

  <div class="foot">
    Facture générée par MikCloud — plateforme de gestion de hotspot MikroTik.<br>
    Ce document tient lieu de justificatif de paiement. Conservez-le pour vos archives comptables.
  </div>

  <p class="noprint" style="margin-top:24px; text-align:center;">
    <button onclick="window.print()" style="padding:10px 22px; border:none; border-radius:8px; background:#16a34a; color:#fff; font-size:14px; font-weight:600; cursor:pointer;">Télécharger / Imprimer (PDF)</button>
  </p>
</div>
</body>
</html>`,
		html.EscapeString(invoiceNo),
		html.EscapeString(invoiceNo),
		html.EscapeString(issued),
		html.EscapeString(paidVia),
		html.EscapeString(ownerName),
		html.EscapeString(ownerEmail),
		html.EscapeString(formatPhoneDisplay(ownerPhone)),
		html.EscapeString(br.Ref),
		html.EscapeString(paidVia),
		html.EscapeString(resolverDisplay(br.ResolvedBy)),
		html.EscapeString(br.PlanName),
		routersSuffix(br.RouterCount),
		html.EscapeString(br.PeriodLabel),
		formatFcfa(br.AmountFcfa),
		formatFcfa(br.AmountFcfa),
	)
}

// formatFcfa — 12000 → « 12 000 ».
func formatFcfa(n int) string {
	s := fmt.Sprintf("%d", n)
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(' ')
		}
		b.WriteRune(c)
	}
	return b.String()
}

// routersSuffix — mention « · N routeur(s) couvert(s) » pour l'Essentiel.
func routersSuffix(n int) string {
	if n <= 0 {
		return ""
	}
	if n == 1 {
		return " · 1 routeur couvert"
	}
	return fmt.Sprintf(" · %d routeurs couverts", n)
}

// resolverDisplay — libellé du résolveur (« webhook Wave » → Wave).
func resolverDisplay(by string) string {
	if strings.Contains(strings.ToLower(by), "wave") || strings.Contains(strings.ToLower(by), "webhook") {
		return "Wave (automatique)"
	}
	if by == "" {
		return "Plateforme MikCloud"
	}
	return by
}

// formatPhoneDisplay — +225 07… si l'indicatif est présent (2250700…).
func formatPhoneDisplay(p string) string {
	if len(p) > 8 && strings.HasPrefix(p, "225") {
		return "+" + p[:3] + " " + p[3:]
	}
	if p != "" {
		return "+" + p
	}
	return "—"
}
