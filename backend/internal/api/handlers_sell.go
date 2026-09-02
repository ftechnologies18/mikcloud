package api

// N°8 — Mode Vente (PWA revendeur en tournée).
//
// Authentification ALLÉGée : le revendeur se connecte avec son identifiant
// + un PIN 4-6 chiffres (pas le mot de passe console). Le JWT émis porte le
// rôle « reseller » — rang 0 dans la hiérarchie N°7 : il est REFUSÉ par
// toutes les routes requireRole, seul le trio /api/sell/* lui répond.
//
// Modèle économique inchangé : le crédit est débité à la GÉNÉRATION du lot
// (console). En tournée, « vendre » = remettre le voucher au client →
// SoldAt/SoldVia tracés (anti-vol : le gérant voit stock vs remis) —
// aucune double écriture financière.

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"mikcloud/hotspot-api/internal/auth"
	"mikcloud/hotspot-api/internal/model"
)

// sellVoucherItem — ligne de stock côté app revendeur (code visible : le
// vendeur doit pouvoir le dicter/imprimer/partager).
type sellVoucherItem struct {
	ID           string `json:"id"`
	Username     string `json:"username"` // = code du voucher
	Password     string `json:"password"`
	ProfileName  string `json:"profileName"`
	Price        int    `json:"price"`
	SellingPrice int    `json:"sellingPrice"`
	DataQuotaMb  int64  `json:"dataQuotaMb"`
	ExpiresAt    string `json:"expiresAt"`
	RouterName   string `json:"routerName"`
	CreatedAt    string `json:"createdAt"`
}

// requireReseller — le token doit porter le rôle « reseller » (PIN).
func (a *API) requireReseller(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if c := claimsFrom(r); c != nil && c.Role == "reseller" {
			next(w, r)
			return
		}
		writeErr(w, http.StatusForbidden, "Réservé aux revendeurs (Mode Vente)")
	}
}

// handleResellerLogin — POST /api/reseller/login {username, pin}.
// Retourne un token scopé (role=reseller) + le profil de tournée.
func (a *API) handleResellerLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Pin      string `json:"pin"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	req.Username = strings.TrimSpace(strings.ToLower(req.Username))
	req.Pin = strings.TrimSpace(req.Pin)
	if req.Username == "" || req.Pin == "" {
		writeErr(w, http.StatusBadRequest, "Identifiant et PIN requis")
		return
	}
	a.store.Lock()
	var res *model.Reseller
	var accStatus string
	for i := range a.store.Data().Resellers {
		c := &a.store.Data().Resellers[i]
		if strings.EqualFold(c.Username, req.Username) {
			res = c
			break
		}
	}
	if res != nil {
		for i := range a.store.Data().Accounts {
			if a.store.Data().Accounts[i].ID == res.AccountID {
				accStatus = a.store.Data().Accounts[i].Status
				break
			}
		}
	}
	pinHash := ""
	if res != nil {
		pinHash = res.PinHash
	}
	a.store.Unlock()

	if res == nil || pinHash == "" || !auth.CheckPassword(req.Pin, "", pinHash) {
		writeErr(w, http.StatusBadRequest, "Identifiant ou PIN invalide")
		return
	}
	if res.Status != "active" {
		writeErr(w, http.StatusForbidden, "Compte revendeur désactivé")
		return
	}
	if accStatus == "disabled" {
		writeErr(w, http.StatusForbidden, "Compte désactivé — contactez le support")
		return
	}

	token := auth.Sign(a.secret, auth.NewClaims(res.ID, res.Name, "reseller", res.AccountID))
	writeJSON(w, http.StatusOK, map[string]any{
		"token": token,
		"reseller": map[string]any{
			"id": res.ID, "name": res.Name, "username": res.Username,
		},
	})
}

// sellMe — profil de tournée : portefeuille + stats du jour.
type sellMe struct {
	Name         string `json:"name"`
	Username     string `json:"username"`
	Credit       int    `json:"credit"`
	StockCount   int    `json:"stockCount"`
	SoldToday    int    `json:"soldToday"`
	RevenueToday int    `json:"revenueToday"`
	Currency     string `json:"currency"`
	// N°19 — dépôt-vente : le solde affiché devient « à verser ».
	PaymentMode string `json:"paymentMode"`
	Debt        int    `json:"debt"`
	DebtCeiling int    `json:"debtCeiling"`
}

// handleSellMe — GET /api/sell/me.
func (a *API) handleSellMe(w http.ResponseWriter, r *http.Request) {
	c := claimsFrom(r)
	now := time.Now().UTC()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	a.store.Lock()
	defer a.store.Unlock()
	db := a.store.Data()
	out := sellMe{}
	if res := findResellerScoped(db, c.Sub, c.Acc); res != nil {
		out.Name, out.Username, out.Credit = res.Name, res.Username, res.Credit
		// N°19 — dépôt-vente : la créance courante accompagne le profil.
		out.PaymentMode = res.PaymentMode
		if out.PaymentMode == "" {
			out.PaymentMode = "prepaid"
		}
		out.DebtCeiling = res.DebtCeiling
		out.Debt = depositDebt(db, c.Acc, res.ID)
	}
	for i := range db.HotspotUsers {
		u := &db.HotspotUsers[i]
		if u.ResellerID != c.Sub || u.AccountID != c.Acc || u.Kind != "voucher" {
			continue
		}
		if u.SoldAt != "" {
			if at, err := time.Parse(time.RFC3339, u.SoldAt); err == nil && !at.Before(todayStart) {
				out.SoldToday++
				price := u.SellingPrice
				if price == 0 {
					price = u.Price
				}
				out.RevenueToday += price
			}
			continue
		}
		if model.EffectiveStatus(u, now) == "active" {
			out.StockCount++
		}
	}
	if s, ok := db.SettingsByAccount[c.Acc]; ok {
		out.Currency = s.Tenant.Currency
	}
	writeJSON(w, http.StatusOK, out)
}

// handleSellStock — GET /api/sell/stock : vouchers actifs non remis, du jour
// le plus récent au plus ancien (le prochain à vendre en tête).
func (a *API) handleSellStock(w http.ResponseWriter, r *http.Request) {
	c := claimsFrom(r)
	now := time.Now().UTC()
	a.store.Lock()
	defer a.store.Unlock()
	db := a.store.Data()
	items := []sellVoucherItem{}
	for i := range db.HotspotUsers {
		u := &db.HotspotUsers[i]
		if u.ResellerID != c.Sub || u.AccountID != c.Acc || u.Kind != "voucher" {
			continue
		}
		if u.SoldAt != "" || model.EffectiveStatus(u, now) != "active" {
			continue
		}
		items = append(items, sellVoucherItem{
			ID: u.ID, Username: u.Username, Password: u.Password,
			ProfileName: u.ProfileName, Price: u.Price, SellingPrice: u.SellingPrice,
			DataQuotaMb: u.DataQuotaMb, ExpiresAt: u.ExpiresAt,
			RouterName: u.RouterName, CreatedAt: u.CreatedAt,
		})
	}
	// tri : plus récemment généré en premier (rotation FIFO du stock).
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && items[j].CreatedAt > items[j-1].CreatedAt; j-- {
			items[j], items[j-1] = items[j-1], items[j]
		}
	}
	writeJSON(w, http.StatusOK, items)
}

// handleSellSold — POST /api/sell/{id}/sold : remise au client (idempotent
// refusé si déjà remis). Audit avec le revendeur comme acteur.
func (a *API) handleSellSold(w http.ResponseWriter, r *http.Request) {
	c := claimsFrom(r)
	id := r.PathValue("id")
	a.store.Lock()
	defer a.store.Unlock()
	db := a.store.Data()
	var u *model.HotspotUser
	for i := range db.HotspotUsers {
		if db.HotspotUsers[i].ID == id {
			u = &db.HotspotUsers[i]
			break
		}
	}
	if u == nil || u.ResellerID != c.Sub || u.AccountID != c.Acc {
		writeErr(w, http.StatusNotFound, "Voucher introuvable dans votre stock")
		return
	}
	if u.SoldAt != "" {
		writeErr(w, http.StatusConflict, "Voucher déjà remis à un client")
		return
	}
	// N°19 — anti-vol ACTIF : un revendeur en dépôt-vente dont la créance
	// dépasse le plafond ne peut plus travailler sur l'argent non versé —
	// le gérant encaisse un versement pour débloquer la vente.
	if res := findResellerScoped(db, c.Sub, c.Acc); res != nil && res.PaymentMode == "deposit" {
		if debt := depositDebt(db, c.Acc, res.ID); debt > res.DebtCeiling {
			// (le defer store.Unlock ci-dessus libère le verrou — pas d'Unlock explicite)
			writeErr(w, http.StatusForbidden, fmt.Sprintf("Plafond de créance dépassé (dette: %d, plafond: %d) — versement requis avant de continuer à vendre", debt, res.DebtCeiling))
			return
		}
	}
	now := model.NowISO()
	u.SoldAt = now
	u.SoldVia = "sell_mode"
	// N°19 — dépôt-vente : la créance naît à la REMISE (prix gros u.Price).
	if u.CreditSale {
		db.Transactions = append([]model.Transaction{{
			ID: model.NewID("tx-"), AccountID: c.Acc, Type: "debt", ResellerID: u.ResellerID, ResellerName: c.Name,
			Amount: u.Price, Note: fmt.Sprintf("Créance : voucher %s remis au client (Mode Vente)", u.Username),
			At: now,
		}}, db.Transactions...)
	}
	a.logActivityBy(r, db, c.Acc, "voucher",
		"Voucher "+u.Username+" remis au client par "+c.Name+" (Mode Vente)")
	a.store.Save()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "soldAt": now})
}

// sellReturnRequest — corps de POST /api/sell/return : identifiants des
// vouchers (stock du revendeur) à rendre au gérant.
type sellReturnRequest struct {
	IDs []string `json:"ids"`
}

// handleSellReturn — N°20 — retour de stock initié par le REVENDEUR depuis le
// Mode Vente : il rend au gérant des tickets encore en sa possession (jamais
// remis à un client, statut effectif « actif »). Miroir exact du transfert
// « direct » du gérant (N°18, handlers_transfer.go) mais à l'initiative du
// revendeur, scopé à SON stock (claims Sub/Acc — isolation stricte) :
//
//   - règle d'or 1 (N°18) : changer la propriété, jamais dupliquer —
//     ResellerID/ResellerName/CreditSale vidés (retour au stock direct) ;
//   - règle d'or 4 : l'argent suit le retour — prépayé : le portefeuille est
//     recrédité du prix GROS (u.Price) de chaque ticket + UNE Transaction
//     « credit » agrégée ; dépôt-vente : rien n'avait été débité à la prise,
//     aucun recrédit (le retour réduit seulement le stock exposé — la créance
//     ne naît qu'à la remise client) ;
//   - règle d'or 5 : Activity avec le revendeur comme acteur (audit).
//
// Refus : tickets remis/consommés/expirés/désactivés (409) — on ne rend que
// du stock vivant ; idempotent par construction (revérifié sous verrou).
func (a *API) handleSellReturn(w http.ResponseWriter, r *http.Request) {
	// P3 — compte expiré : écritures métier refusées (lecture seule).
	if !a.guardAccountWrite(w, r) {
		return
	}
	c := claimsFrom(r)
	var req sellReturnRequest
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	if len(req.IDs) == 0 {
		writeErr(w, http.StatusBadRequest, "Aucun ticket sélectionné pour le retour")
		return
	}
	if len(req.IDs) > 500 {
		writeErr(w, http.StatusBadRequest, "Trop de tickets en une fois (500 maximum)")
		return
	}

	now := time.Now().UTC()
	a.store.Lock()
	defer a.store.Unlock()
	db := a.store.Data()

	// Résolution scopée : chaque id doit appartenir AU stock du revendeur
	// (ResellerID == claims.Sub, compte == claims.Acc) et être rendable.
	wanted := make(map[string]bool, len(req.IDs))
	for _, id := range req.IDs {
		id = strings.TrimSpace(id)
		if id != "" {
			wanted[id] = true
		}
	}
	if len(wanted) == 0 {
		writeErr(w, http.StatusBadRequest, "Aucun ticket sélectionné pour le retour")
		return
	}
	type returnable struct {
		idx int
		u   *model.HotspotUser
	}
	found := map[string]*returnable{}
	for i := range db.HotspotUsers {
		u := &db.HotspotUsers[i]
		if !wanted[u.ID] || u.ResellerID != c.Sub || u.AccountID != c.Acc || u.Kind != "voucher" {
			continue
		}
		found[u.ID] = &returnable{idx: i, u: u}
	}
	rejected := []string{}
	for id := range wanted {
		u := found[id]
		switch {
		case u == nil:
			rejected = append(rejected, id) // hors stock / compte voisin / inconnu
		case u.u.SoldAt != "" || model.EffectiveStatus(u.u, now) != "active":
			rejected = append(rejected, u.u.Username) // remis, consommé, expiré ou désactivé
		}
	}
	// Tout refus bloque le lot (cohérent avec le refus idempotent de « sold ») :
	// le revendeur décoche les tickets concernés et relance.
	if len(rejected) > 0 {
		writeErr(w, http.StatusConflict,
			fmt.Sprintf("Ticket(s) non rendable(s) : %s (déjà remis au client, expiré ou introuvable dans votre stock)", strings.Join(rejected, ", ")))
		return
	}

	// Recrédit prépayé : prix GROS (u.Price — ce qui avait été débité), aucune
	// ligne Sale (l'émission reste liée à la génération — zéro double comptage).
	var res *model.Reseller
	if r0 := findResellerScoped(db, c.Sub, c.Acc); r0 != nil {
		res = r0
	}
	total := 0
	for _, rt := range found {
		total += rt.u.Price
	}
	prepaid := res == nil || res.PaymentMode != "deposit"

	// Application — retour au stock direct (règle d'or 1).
	returned := make([]string, 0, len(found))
	for _, rt := range found {
		u := rt.u
		returned = append(returned, u.Username)
		u.ResellerID, u.ResellerName = "", ""
		u.CreditSale = false
	}
	sort.Strings(returned) // message d'audit stable

	creditAfter := 0
	if prepaid && total > 0 && res != nil {
		res.Credit += total
		creditAfter = res.Credit
		db.Transactions = append([]model.Transaction{{
			ID: model.NewID("tx-"), AccountID: c.Acc, Type: "credit",
			ResellerID: c.Sub, ResellerName: c.Name, Amount: total,
			Note: fmt.Sprintf("Retour de stock : %d voucher(s) rendus par %s (Mode Vente)", len(returned), c.Name),
			At:   model.NowISO(),
		}}, db.Transactions...)
	}
	a.logActivityBy(r, db, c.Acc, "voucher",
		fmt.Sprintf("Retour de stock : %d voucher(s) [%s] rendus au gérant par %s (Mode Vente%s)",
			len(returned), strings.Join(returned, ", "), c.Name,
			map[bool]string{true: fmt.Sprintf(" — recrédité : %d", total), false: " — dépôt-vente : aucun recrédit"}[prepaid]))
	a.store.Save()

	writeJSON(w, http.StatusOK, map[string]any{
		"returned":    len(returned),
		"credited":    map[bool]int{true: total, false: 0}[prepaid],
		"creditAfter": creditAfter,
		"codes":       returned,
	})
}

// sellDayReportItem — une vente du rapport de fin de journée (journal
// chronologique : le revendeur relit sa journée dans l'ordre).
type sellDayReportItem struct {
	ID          string `json:"id"`
	Code        string `json:"code"`
	ProfileName string `json:"profileName"`
	Price       int    `json:"price"`
	SoldAt      string `json:"soldAt"`
	RouterName  string `json:"routerName"`
}

// sellDayReport — rapport de fin de journée du revendeur (N°8) : tout ce qui
// a été remis au client AUJOURD'HUI (détail horodaté), la recette du jour et
// le stock restant (avec sa valeur faciale) pour la clôture en tournée.
type sellDayReport struct {
	Date       string              `json:"date"` // YYYY-MM-DD (UTC — journée métier)
	Currency   string              `json:"currency"`
	Sold       []sellDayReportItem `json:"sold"`
	SoldCount  int                 `json:"soldCount"`
	Revenue    int                 `json:"revenue"`
	StockCount int                 `json:"stockCount"`
	StockValue int                 `json:"stockValue"`
	// N°19 V2 — dépôt-vente : ce que la tournée doit ramener au gérant.
	ToDeposit   int    `json:"toDeposit"`   // cash du jour à verser (= recette) ; 0 en prépayé
	DebtTotal   int    `json:"debtTotal"`   // créance totale courante ; 0 en prépayé
	PaymentMode string `json:"paymentMode"` // prepaid | deposit
}

// handleSellDayReport — GET /api/sell/day-report : clôture de journée du
// revendeur. Même frontière de jour que /api/sell/me (UTC, marché cible).
func (a *API) handleSellDayReport(w http.ResponseWriter, r *http.Request) {
	c := claimsFrom(r)
	now := time.Now().UTC()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	a.store.Lock()
	defer a.store.Unlock()
	db := a.store.Data()
	report := sellDayReport{
		Date: now.Format("2006-01-02"),
		Sold: []sellDayReportItem{},
	}
	for i := range db.HotspotUsers {
		u := &db.HotspotUsers[i]
		if u.ResellerID != c.Sub || u.AccountID != c.Acc || u.Kind != "voucher" {
			continue
		}
		price := u.SellingPrice
		if price == 0 {
			price = u.Price
		}
		if u.SoldAt != "" {
			at, err := time.Parse(time.RFC3339, u.SoldAt)
			if err != nil || at.Before(todayStart) {
				continue
			}
			report.Sold = append(report.Sold, sellDayReportItem{
				ID: u.ID, Code: u.Username, ProfileName: u.ProfileName,
				Price: price, SoldAt: u.SoldAt, RouterName: u.RouterName,
			})
			report.SoldCount++
			report.Revenue += price
			continue
		}
		if model.EffectiveStatus(u, now) == "active" {
			report.StockCount++
			report.StockValue += price
		}
	}
	// Journal chronologique : de la première vente du matin à la dernière.
	sort.Slice(report.Sold, func(i, j int) bool { return report.Sold[i].SoldAt < report.Sold[j].SoldAt })
	// N°19 V2 — dépôt-vente : le rapport annonce ce qu'il faut ramener.
	if res := findResellerScoped(db, c.Sub, c.Acc); res != nil {
		report.PaymentMode = res.PaymentMode
		if report.PaymentMode == "" {
			report.PaymentMode = "prepaid"
		}
		if report.PaymentMode == "deposit" {
			report.ToDeposit = report.Revenue
			report.DebtTotal = depositDebt(db, c.Acc, res.ID)
		}
	}
	if s, ok := db.SettingsByAccount[c.Acc]; ok {
		report.Currency = s.Tenant.Currency
	}
	writeJSON(w, http.StatusOK, report)
}
