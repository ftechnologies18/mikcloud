// handlers_resellers.go — gestion des revendeurs (CRUD, PIN, crédit).

package api

import (
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"mikcloud/hotspot-api/internal/auth"
	"mikcloud/hotspot-api/internal/model"
)

// sanitizeReseller — copie SANS le hash du PIN (jamais exposé par l'API).
// pinSet informe l'UI qu'un PIN Mode Vente est actif (N°8).
func sanitizeReseller(res model.Reseller) map[string]any {
	pinSet := res.PinHash != ""
	return map[string]any{
		"id": res.ID, "accountId": res.AccountID, "name": res.Name,
		"username": res.Username, "phone": res.Phone, "credit": res.Credit,
		"vouchersSold": res.VouchersSold, "revenue": res.Revenue,
		"status": res.Status, "createdAt": res.CreatedAt, "pinSet": pinSet,
		// N°19 — mode de paiement (prépayé par défaut) + plafond de créance.
		"paymentMode": map[bool]string{true: res.PaymentMode, false: "prepaid"}[res.PaymentMode != ""],
		"debtCeiling": res.DebtCeiling,
	}
}

func (a *API) handleResellersList(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	a.store.Lock()
	db := a.store.Data()
	rs := []model.Reseller{}
	for _, res := range db.Resellers {
		if res.AccountID == acc {
			rs = append(rs, res)
		}
	}
	// N°8 — stats LIVE « stock vs vendus » (traçabilité anti-vol) :
	// recalculées à chaque lecture depuis les vouchers attribués, jamais
	// depuis les compteurs de démo. stock = attribués non remis toujours
	// actifs (vendables) ; vendus = remis au client (SoldAt) ; attribués =
	// tout voucher portant le revendeur — l'écart stock+vendus vs attribués
	// révèle les tickets sortis sans déclaration.
	now := time.Now().UTC()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	type resellerStats struct {
		stock, sold, assigned, soldToday, revenueToday, revenueTotal int
	}
	stats := make(map[string]*resellerStats, len(rs))
	for i := range db.HotspotUsers {
		u := &db.HotspotUsers[i]
		if u.Kind != "voucher" || u.ResellerID == "" || u.AccountID != acc {
			continue
		}
		st := stats[u.ResellerID]
		if st == nil {
			st = &resellerStats{}
			stats[u.ResellerID] = st
		}
		st.assigned++
		price := u.SellingPrice
		if price == 0 {
			price = u.Price
		}
		if u.SoldAt != "" {
			st.sold++
			st.revenueTotal += price
			if at, err := time.Parse(time.RFC3339, u.SoldAt); err == nil && !at.Before(todayStart) {
				st.soldToday++
				st.revenueToday += price
			}
			continue
		}
		if model.EffectiveStatus(u, now) == "active" {
			st.stock++
		}
	}
	// N°19 — créance dépôt-vente : Σ(debt) − Σ(settlement) par revendeur,
	// comptée sur les Transactions (la créance naît à la remise, pas à la
	// prise de stock — voir handlers_deposit.go).
	type debtStats struct {
		debt, settlements int
		lastSettlement    string
	}
	dstats := make(map[string]*debtStats, len(rs))
	for _, tx := range db.Transactions {
		if tx.AccountID != acc {
			continue
		}
		ds := dstats[tx.ResellerID]
		if ds == nil {
			ds = &debtStats{}
			dstats[tx.ResellerID] = ds
		}
		switch tx.Type {
		case "debt":
			ds.debt += tx.Amount
		case "settlement":
			ds.debt -= tx.Amount
			ds.settlements++
			if tx.At > ds.lastSettlement {
				ds.lastSettlement = tx.At
			}
		}
	}
	a.store.Unlock()
	sort.Slice(rs, func(i, j int) bool { return rs[i].CreatedAt > rs[j].CreatedAt })
	out := make([]map[string]any, len(rs))
	for i := range rs {
		m := sanitizeReseller(rs[i])
		st := stats[rs[i].ID]
		if st == nil {
			st = &resellerStats{}
		}
		m["stockCount"] = st.stock
		m["soldCount"] = st.sold
		m["assignedCount"] = st.assigned
		m["soldToday"] = st.soldToday
		m["revenueToday"] = st.revenueToday
		m["revenueTotal"] = st.revenueTotal
		// N°19 — dette (0 pour un prépayé sans créance), versements, dernière date.
		ds := dstats[rs[i].ID]
		if ds == nil {
			ds = &debtStats{}
		}
		m["debt"] = ds.debt
		m["settlementsCount"] = ds.settlements
		m["lastSettlementAt"] = ds.lastSettlement
		out[i] = m
	}
	writeJSON(w, http.StatusOK, out)
}

// resellerPinPattern — PIN Mode Vente : 4 à 6 chiffres (N°8).
var resellerPinPattern = regexp.MustCompile(`^[0-9]{4,6}$`)

// pinNote — mention d'audit quand un PIN Mode Vente est défini.
func pinNote(pin string) string {
	if pin == "" {
		return ""
	}
	return " (PIN Mode Vente défini)"
}

func (a *API) handleResellerCreate(w http.ResponseWriter, r *http.Request) {
	// P3 — compte expiré : écritures métier refusées (lecture seule).
	if !a.guardAccountWrite(w, r) {
		return
	}
	acc := accountScope(r)
	var req struct {
		Name     string `json:"name"`
		Username string `json:"username"`
		Phone    string `json:"phone"`
		Credit   int    `json:"credit"`
		Pin      string `json:"pin"` // N°8 — PIN Mode Vente (4-6 chiffres, optionnel)
		// N°19 — mode de paiement : prepaid (défaut) | deposit (plafond requis).
		PaymentMode string `json:"paymentMode"`
		DebtCeiling int    `json:"debtCeiling"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	name := strings.TrimSpace(req.Name)
	username := strings.TrimSpace(req.Username)
	if name == "" || username == "" {
		writeErr(w, http.StatusBadRequest, "Nom et nom d'utilisateur du revendeur requis")
		return
	}
	pin := strings.TrimSpace(req.Pin)
	if pin != "" && !resellerPinPattern.MatchString(pin) {
		writeErr(w, http.StatusBadRequest, "PIN invalide : 4 à 6 chiffres")
		return
	}
	if req.Credit < 0 {
		writeErr(w, http.StatusBadRequest, "Crédit initial invalide")
		return
	}
	// N°19 — mode de paiement : prépayé (défaut) ou dépôt-vente (plafond requis).
	paymentMode := req.PaymentMode
	if paymentMode == "" {
		paymentMode = "prepaid"
	}
	if paymentMode != "prepaid" && paymentMode != "deposit" {
		writeErr(w, http.StatusBadRequest, "Mode de paiement invalide (prepaid ou deposit)")
		return
	}
	debtCeiling := req.DebtCeiling
	if paymentMode == "deposit" && debtCeiling <= 0 {
		writeErr(w, http.StatusBadRequest, "Plafond de créance requis en dépôt-vente (> 0)")
		return
	}
	if paymentMode == "prepaid" {
		debtCeiling = 0
	}
	a.store.Lock()
	db := a.store.Data()
	for _, res := range db.Resellers {
		if res.AccountID == acc && strings.EqualFold(res.Username, username) {
			a.store.Unlock()
			writeErr(w, http.StatusBadRequest, "Ce nom d'utilisateur revendeur existe déjà")
			return
		}
	}
	reseller := model.Reseller{
		ID: model.NewID("res-"), AccountID: acc, Name: name, Username: username, Phone: strings.TrimSpace(req.Phone),
		Credit: req.Credit, VouchersSold: 0, Revenue: 0, Status: "active", CreatedAt: model.NowISO(),
		PaymentMode: paymentMode, DebtCeiling: debtCeiling,
	}
	if pin != "" {
		reseller.PinHash = auth.HashPassword(pin, "")
	}
	db.Resellers = append(db.Resellers, reseller)
	a.logActivityBy(r, db, acc, "reseller", "Revendeur "+reseller.Name+" créé"+pinNote(pin)+modeNote(paymentMode, debtCeiling))
	a.store.Save()
	sanitized := sanitizeReseller(reseller)
	a.store.Unlock()
	writeJSON(w, http.StatusOK, sanitized)
}

func (a *API) handleResellerUpdate(w http.ResponseWriter, r *http.Request) {
	// P3 — compte expiré : écritures métier refusées (lecture seule).
	if !a.guardAccountWrite(w, r) {
		return
	}
	acc := accountScope(r)
	id := r.PathValue("id")
	var req struct {
		Name   *string `json:"name"`
		Phone  *string `json:"phone"`
		Status *string `json:"status"`
		Pin    *string `json:"pin"` // N°8 — définir/remplacer ; "" = retirer
		// N°19 — bascule de mode de paiement + plafond de créance.
		PaymentMode *string `json:"paymentMode"`
		DebtCeiling *int    `json:"debtCeiling"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	if req.Pin != nil {
		p := strings.TrimSpace(*req.Pin)
		if p != "" && !resellerPinPattern.MatchString(p) {
			writeErr(w, http.StatusBadRequest, "PIN invalide : 4 à 6 chiffres")
			return
		}
	}
	a.store.Lock()
	db := a.store.Data()
	res := findResellerScoped(db, id, acc)
	if res == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Revendeur introuvable")
		return
	}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			a.store.Unlock()
			writeErr(w, http.StatusBadRequest, "Nom du revendeur requis")
			return
		}
		res.Name = name
	}
	if req.Phone != nil {
		res.Phone = strings.TrimSpace(*req.Phone)
	}
	if req.Status != nil {
		if *req.Status != "active" && *req.Status != "disabled" {
			a.store.Unlock()
			writeErr(w, http.StatusBadRequest, "Statut invalide (active ou disabled)")
			return
		}
		res.Status = *req.Status
	}
	// N°19 — mode de paiement + plafond : la bascule en prépayé exige une
	// dette soldée ; le dépôt-vente exige un plafond > 0.
	newMode := res.PaymentMode
	if newMode == "" {
		newMode = "prepaid"
	}
	if req.PaymentMode != nil {
		m := strings.TrimSpace(*req.PaymentMode)
		if m != "prepaid" && m != "deposit" {
			a.store.Unlock()
			writeErr(w, http.StatusBadRequest, "Mode de paiement invalide (prepaid ou deposit)")
			return
		}
		newMode = m
	}
	newCeiling := res.DebtCeiling
	if req.DebtCeiling != nil {
		if *req.DebtCeiling < 0 {
			a.store.Unlock()
			writeErr(w, http.StatusBadRequest, "Le plafond de créance ne peut pas être négatif")
			return
		}
		newCeiling = *req.DebtCeiling
	}
	if newMode == "deposit" && newCeiling <= 0 {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, "Plafond de créance requis en dépôt-vente (> 0)")
		return
	}
	if newMode == "prepaid" {
		newCeiling = 0
		if res.PaymentMode == "deposit" {
			if debt := depositDebt(db, acc, res.ID); debt > 0 {
				a.store.Unlock()
				writeErr(w, http.StatusBadRequest, fmt.Sprintf("Régularisez la dette (%d FCFA) avant de repasser en prépayé", debt))
				return
			}
		}
	}
	modeChanged := newMode != res.PaymentMode || newCeiling != res.DebtCeiling
	res.PaymentMode = newMode
	res.DebtCeiling = newCeiling
	if req.Pin != nil {
		p := strings.TrimSpace(*req.Pin)
		if p == "" {
			res.PinHash = "" // retrait explicite : Mode Vente désactivé
		} else {
			res.PinHash = auth.HashPassword(p, "")
		}
	}
	updated := *res
	if modeChanged {
		a.logActivityBy(r, db, acc, "reseller", "Revendeur "+updated.Name+" modifié"+modeNote(updated.PaymentMode, updated.DebtCeiling))
	} else {
		a.logActivityBy(r, db, acc, "reseller", "Revendeur "+updated.Name+" modifié")
	}
	a.store.Save()
	sanitized := sanitizeReseller(updated)
	a.store.Unlock()
	writeJSON(w, http.StatusOK, sanitized)
}

func (a *API) handleResellerDelete(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	id := r.PathValue("id")
	a.store.Lock()
	db := a.store.Data()
	idx := -1
	for i := range db.Resellers {
		if db.Resellers[i].ID == id && db.Resellers[i].AccountID == acc {
			idx = i
			break
		}
	}
	if idx < 0 {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Revendeur introuvable")
		return
	}
	name := db.Resellers[idx].Name
	db.Resellers = append(db.Resellers[:idx], db.Resellers[idx+1:]...)
	a.logActivityBy(r, db, acc, "reseller", "Revendeur "+name+" supprimé")
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *API) handleResellerCredit(w http.ResponseWriter, r *http.Request) {
	// P3 — compte expiré : écritures métier refusées (lecture seule).
	if !a.guardAccountWrite(w, r) {
		return
	}
	acc := accountScope(r)
	id := r.PathValue("id")
	var req struct {
		Amount int    `json:"amount"`
		Note   string `json:"note"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	if req.Amount == 0 {
		writeErr(w, http.StatusBadRequest, "Le montant doit être différent de zéro")
		return
	}
	a.store.Lock()
	db := a.store.Data()
	res := findResellerScoped(db, id, acc)
	if res == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Revendeur introuvable")
		return
	}
	if req.Amount < 0 && res.Credit < -req.Amount {
		credit := res.Credit
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, fmt.Sprintf("Crédit insuffisant (disponible: %d, requis: %d)", credit, -req.Amount))
		return
	}
	res.Credit += req.Amount
	note := strings.TrimSpace(req.Note)
	if note == "" {
		if req.Amount > 0 {
			note = "Rechargement crédit"
		} else {
			note = "Débit manuel"
		}
	}
	tx := model.Transaction{
		ID: model.NewID("tx-"), AccountID: acc, Type: "credit", ResellerID: res.ID, ResellerName: res.Name,
		Amount: req.Amount, Note: note, At: model.NowISO(),
	}
	db.Transactions = append([]model.Transaction{tx}, db.Transactions...)
	if req.Amount > 0 {
		a.logActivityBy(r, db, acc, "reseller", fmt.Sprintf("Crédit de %d FCFA ajouté à %s", req.Amount, res.Name))
	} else {
		a.logActivityBy(r, db, acc, "reseller", fmt.Sprintf("Débit de %d FCFA sur %s", -req.Amount, res.Name))
	}
	updated := *res
	a.store.Save()
	a.store.Unlock()
	// N°19 — réponse sanitisée (la structure brute incluait PinHash).
	writeJSON(w, http.StatusOK, map[string]any{"reseller": sanitizeReseller(updated), "transaction": tx})
}
