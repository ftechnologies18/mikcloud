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
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
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
	// UX R1 — référence du lot d'origine (tracé à la génération) : permet à la
	// PWA de regrouper le stock par profil puis par lot. Champ additif —
	// rétrocompatible avec les PWA déjà installées.
	BatchID string `json:"batchId"`
}

// requireReseller — le token doit porter le rôle « reseller » (PIN) ET le
// revendeur doit toujours exister (V4, audit revendeurs) : un token TTL 24 h
// qui survit au DELETE du revendeur ne doit plus rien pouvoir faire — sinon
// le revendeur fantôme vendrait sans garde de plafond (findResellerScoped →
// nil) et créerait des créances orphelines. La PWA Mode Vente traite ce 403
// comme une fin de session (retour à l'écran PIN).
func (a *API) requireReseller(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c := claimsFrom(r)
		if c == nil || c.Role != "reseller" {
			writeErr(w, http.StatusForbidden, "Réservé aux revendeurs (Mode Vente)")
			return
		}
		a.store.Lock()
		exists := false
		for i := range a.store.Data().Resellers {
			if a.store.Data().Resellers[i].ID == c.Sub && a.store.Data().Resellers[i].AccountID == c.Acc {
				exists = true
				break
			}
		}
		a.store.Unlock()
		if !exists {
			writeErr(w, http.StatusForbidden, "Session expirée : revendeur supprimé")
			return
		}
		next(w, r)
	}
}

// handleResellerLogin — POST /api/reseller/login {username, pin}.
// Retourne un token scopé (role=reseller) + le profil de tournée.
// Sécurité S2 — verrouillage par compte (cf. pinlock.go) : après 5 échecs
// consécutifs de PIN sur le MÊME revendeur, toute tentative est refusée
// 429 + Retry-After pendant 15 minutes (même bon PIN, même IP neuve) ; les
// échecs sont journalisés (cf. auth_audit.go), la réponse reste générique.
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

	// Sécurité S2 — verrou par compte AVANT toute vérification : un revendeur
	// ciblé par trop d'échecs est refusé sans même consommer un hachage
	// bcrypt (le verrou survit au changement d'IP : clé = ID interne).
	if res != nil {
		if locked, remaining := a.pinLock.check(res.ID); locked {
			a.logAuthFailure(r, "reseller_pin", req.Username, "locked")
			w.Header().Set("Retry-After", strconv.Itoa(int(remaining.Seconds())+1))
			writeErr(w, http.StatusTooManyRequests, "Trop de tentatives — réessayez dans quelques minutes")
			return
		}
	}
	if res == nil {
		a.logAuthFailure(r, "reseller_pin", req.Username, "unknown_reseller")
		writeErr(w, http.StatusBadRequest, "Identifiant ou PIN invalide")
		return
	}
	if pinHash == "" || !auth.CheckPassword(req.Pin, "", pinHash) {
		a.pinLock.fail(res.ID)
		a.logAuthFailure(r, "reseller_pin", req.Username, "bad_pin")
		writeErr(w, http.StatusBadRequest, "Identifiant ou PIN invalide")
		return
	}
	// PIN correct : l'historique d'échecs du revendeur est effacé.
	a.pinLock.reset(res.ID)
	if res.Status != "active" {
		a.logAuthFailure(r, "reseller_pin", req.Username, "reseller_disabled")
		writeErr(w, http.StatusForbidden, "Compte revendeur désactivé")
		return
	}
	if accStatus == "disabled" {
		a.logAuthFailure(r, "reseller_pin", req.Username, "account_disabled")
		writeErr(w, http.StatusForbidden, "Compte désactivé — contactez le support")
		return
	}

	// ver=0 : le revendeur n'est pas un AdminUser — le garde de révocation
	// S1-A3 du middleware ne s'applique pas au rôle « reseller ».
	token := auth.Sign(a.secret, auth.NewClaims(res.ID, res.Name, "reseller", res.AccountID, 0))
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

// sellStockPage — réponse paginée du stock (P3-e). Sans paramètre `limit`,
// /api/sell/stock conserve sa réponse historique (tableau complet) — les PWA
// déjà installées ne voient aucun changement ; avec `limit`, la réponse devient
// une page explicite (items/total/hasMore) et l'app peut charger par pages.
type sellStockPage struct {
	Items   []sellVoucherItem `json:"items"`
	Total   int               `json:"total"`
	HasMore bool              `json:"hasMore"`
}

// handleSellStock — GET /api/sell/stock : vouchers actifs non remis, du jour
// le plus récent au plus ancien (le prochain à vendre en tête).
//
// P3-e — pagination additive : `limit` (1..200) + `offset` (≥ 0). Sans
// `limit` → tableau complet (contrat historique inchangé) ; avec `limit` →
// page `{items, total, hasMore}` sur le même tri (le tri est stable : une
// page se retrouve identique entre deux appels tant que le stock ne bouge pas).
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
			RouterName: u.RouterName, CreatedAt: u.CreatedAt, BatchID: u.BatchID,
		})
	}
	// tri : plus récemment généré en premier (rotation FIFO du stock).
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && items[j].CreatedAt > items[j-1].CreatedAt; j-- {
			items[j], items[j-1] = items[j-1], items[j]
		}
	}
	if r.URL.Query().Get("limit") != "" {
		limit := queryInt(r, "limit", 60, 1, 200)
		offset := queryInt(r, "offset", 0, 0, len(items))
		end := offset + limit
		if end > len(items) {
			end = len(items)
		}
		page := items
		if offset > 0 || end < len(items) {
			page = items[offset:end]
		}
		writeJSON(w, http.StatusOK, sellStockPage{
			Items:   page,
			Total:   len(items),
			HasMore: end < len(items),
		})
		return
	}
	writeJSON(w, http.StatusOK, items)
}

// handleSellSold — POST /api/sell/{id}/sold : remise au client (idempotent
// refusé si déjà remis). Audit avec le revendeur comme acteur.
//
// UX R3 — corps OPTIONNEL {"via":"paper"} : les PWA déjà installées POSTent
// sans corps (vente tactile, SoldVia=sell_mode) ; la vente d'un ticket papier
// imprimé envoie via=paper → SoldVia=sell_mode_paper. Champ additif —
// rétrocompatible.
func (a *API) handleSellSold(w http.ResponseWriter, r *http.Request) {
	c := claimsFrom(r)
	id := r.PathValue("id")
	var body struct {
		Via string `json:"via"`
	}
	if raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)); err == nil && len(bytes.TrimSpace(raw)) > 0 {
		_ = json.Unmarshal(raw, &body)
	}
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
	// P3-c — durcissement : on ne vend qu'un ticket VIVANT. Le stock
	// n'expose que des actifs ; entre l'affichage et la confirmation, un
	// voucher peut expirer (ou être consommé) — refus explicite, jamais de
	// décompte fantôme sur un ticket mort.
	if model.EffectiveStatus(u, time.Now().UTC()) != "active" {
		writeErr(w, http.StatusConflict, "Voucher expiré ou consommé — vente impossible")
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
	// UX R3 — trace d'origine : vente tactile (défaut) vs ticket papier imprimé.
	u.SoldVia = map[bool]string{true: "sell_mode_paper", false: "sell_mode"}[body.Via == "paper"]
	// N°19 — dépôt-vente : la créance naît à la REMISE (prix gros u.Price).
	if u.CreditSale {
		db.Transactions = append([]model.Transaction{{
			ID: model.NewID("tx-"), AccountID: c.Acc, Type: "debt", ResellerID: u.ResellerID, ResellerName: c.Name,
			Amount: u.Price, Note: fmt.Sprintf("Créance : voucher %s remis au client (Mode Vente)", u.Username),
			At: now,
		}}, db.Transactions...)
	}
	a.logActivityBy(r, db, c.Acc, "voucher",
		"Voucher "+u.Username+" remis au client par "+c.Name+" ("+map[bool]string{true: "Mode Vente — ticket papier", false: "Mode Vente"}[body.Via == "paper"]+")")
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
	// P3-d — canal de la vente (audit R4) : sell_mode (tactile),
	// auto_connect (1ʳᵉ connexion client), sell_mode_paper (papier historique).
	// Vide sur les ventes antérieures au traçage — affichées comme tactiles
	// (seul canal qui existait alors).
	SoldVia string `json:"soldVia,omitempty"`
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

	// P3-d — enrichissement comptable (champs additifs, omitempty : les PWA
	// déjà installées ignorent ces clés sans rien casser).
	ByVia map[string]int `json:"byVia,omitempty"` // canal → nombre de ventes du jour
	// Retours de stock du jour (N°20, initiative revendeur OU gérant) :
	// tickets rendus au gérant + recrédit prépayé correspondant (0 en dépôt-vente).
	ReturnedCount    int `json:"returnedCount,omitempty"`
	ReturnedCredited int `json:"returnedCredited,omitempty"`
	// Versements dépôt-vente déjà encaissés par le gérant aujourd'hui —
	// la clôture annonce le RESTE à verser (toDeposit − settledToday).
	SettledToday int `json:"settledToday,omitempty"`
}

// dayJournal — le rapport enrichi + le détail des écritures de trésorerie du
// jour (transactions), partagé entre la réponse JSON et l'export CSV. Les
// champs non exportés sont ignorés par encoding/json : /day-report ne
// renvoie que le rapport.
type dayJournal struct {
	report      sellDayReport
	reseller    *model.Reseller
	returns     []model.Transaction // crédits « Retour de stock » du jour
	settlements []model.Transaction // versements dépôt-vente du jour
}

// sellViaLabel — libellé humain d'un canal de vente (rapport, CSV).
func sellViaLabel(via string) string {
	switch via {
	case "auto_connect":
		return "Auto connexion"
	case "sell_mode_paper":
		return "Papier"
	default:
		// sell_mode + ventes antérieures au traçage (seul canal existant alors).
		return "Tactile"
	}
}

// computeDayJournal — cœur de calcul partagé JSON / CSV (l'appelant tient le
// verrou du store). Frontière de jour identique à /api/sell/me : UTC
// (marché cible). dayEnd borne la fenêtre — aujourd'hui pour le rapport,
// n'importe quel jour passé pour l'export comptable.
func (a *API) computeDayJournal(db *model.DB, c *auth.Claims, dayStart, dayEnd, now time.Time) dayJournal {
	j := dayJournal{
		report: sellDayReport{
			Date: dayStart.Format("2006-01-02"),
			Sold: []sellDayReportItem{},
		},
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
			if err != nil || at.Before(dayStart) || !at.Before(dayEnd) {
				continue
			}
			via := u.SoldVia
			if via == "" {
				via = "sell_mode"
			}
			j.report.Sold = append(j.report.Sold, sellDayReportItem{
				ID: u.ID, Code: u.Username, ProfileName: u.ProfileName,
				Price: price, SoldAt: u.SoldAt, RouterName: u.RouterName, SoldVia: via,
			})
			j.report.SoldCount++
			j.report.Revenue += price
			if j.report.ByVia == nil {
				j.report.ByVia = map[string]int{}
			}
			j.report.ByVia[via]++
			continue
		}
		// Le stock affiché reste le stock ACTUEL (les tickets restants
		// aujourd'hui) — pertinent pour la clôture du jour, neutre pour un
		// export d'une date passée (les lignes stock sont alors omises).
		if now.Before(dayEnd) && model.EffectiveStatus(u, now) == "active" {
			j.report.StockCount++
			j.report.StockValue += price
		}
	}
	// Journal chronologique : de la première vente du matin à la dernière.
	sort.Slice(j.report.Sold, func(a, b int) bool { return j.report.Sold[a].SoldAt < j.report.Sold[b].SoldAt })

	// Écritures de trésorerie du jour — retours de stock (crédits, initiative
	// revendeur N°20 ou gérant) et versements dépôt-vente (encaissements).
	// Les autres crédits (rechargements) ne sont PAS des flux de tournée.
	for _, tx := range db.Transactions {
		if tx.ResellerID != c.Sub || tx.AccountID != c.Acc {
			continue
		}
		at, err := time.Parse(time.RFC3339, tx.At)
		if err != nil || at.Before(dayStart) || !at.Before(dayEnd) {
			continue
		}
		switch {
		case tx.Type == "credit" && strings.HasPrefix(tx.Note, "Retour de stock"):
			j.returns = append(j.returns, tx)
		case tx.Type == "settlement":
			j.settlements = append(j.settlements, tx)
		}
	}
	for _, tx := range j.returns {
		j.report.ReturnedCount++
		j.report.ReturnedCredited += tx.Amount
	}
	for _, tx := range j.settlements {
		j.report.SettledToday += tx.Amount
	}

	// N°19 V2 — dépôt-vente : le rapport annonce ce qu'il faut ramener.
	if res := findResellerScoped(db, c.Sub, c.Acc); res != nil {
		j.reseller = res
		j.report.PaymentMode = res.PaymentMode
		if j.report.PaymentMode == "" {
			j.report.PaymentMode = "prepaid"
		}
		if j.report.PaymentMode == "deposit" {
			j.report.ToDeposit = j.report.Revenue
			j.report.DebtTotal = depositDebt(db, c.Acc, res.ID)
		}
	}
	if s, ok := db.SettingsByAccount[c.Acc]; ok {
		j.report.Currency = s.Tenant.Currency
	}
	return j
}

// handleSellDayReport — GET /api/sell/day-report : clôture de journée du
// revendeur. Même frontière de jour que /api/sell/me (UTC, marché cible).
func (a *API) handleSellDayReport(w http.ResponseWriter, r *http.Request) {
	c := claimsFrom(r)
	now := time.Now().UTC()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	a.store.Lock()
	defer a.store.Unlock()
	j := a.computeDayJournal(a.store.Data(), c, todayStart, todayStart.AddDate(0, 0, 1), now)
	writeJSON(w, http.StatusOK, j.report)
}

// handleSellDayReportCSV — GET /api/sell/day-report.csv?date=YYYY-MM-DD :
// export comptable « journal de caisse » du revendeur. Le jour par défaut est
// aujourd'hui ; toute date passée est admise (compta), la date du jour porte
// en plus les lignes de stock/créance (état courant). Format aligné sur
// l'export console (séparateur « ; », BOM UTF-8, CRLF — Excel FR).
func (a *API) handleSellDayReportCSV(w http.ResponseWriter, r *http.Request) {
	c := claimsFrom(r)
	now := time.Now().UTC()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	dayStart := todayStart
	if raw := strings.TrimSpace(r.URL.Query().Get("date")); raw != "" {
		d, err := time.ParseInLocation("2006-01-02", raw, time.UTC)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "Date invalide — format attendu AAAA-MM-JJ")
			return
		}
		dayStart = d
	}
	dayEnd := dayStart.AddDate(0, 0, 1)

	a.store.Lock()
	j := a.computeDayJournal(a.store.Data(), c, dayStart, dayEnd, now)
	currency := j.report.Currency
	resellerName := c.Name
	if j.reseller != nil && j.reseller.Name != "" {
		resellerName = j.reseller.Name
	}
	mode := "Prepaye"
	if j.report.PaymentMode == "deposit" {
		mode = "Depot-vente"
	}
	isToday := dayStart.Equal(todayStart)
	a.store.Unlock()

	hm := func(iso string) string {
		t, err := time.Parse(time.RFC3339, iso)
		if err != nil {
			return ""
		}
		return t.Format("15:04")
	}

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf("attachment; filename=\"journal-caisse-%s-%s.csv\"", j.report.Date, c.Sub))
	// BOM UTF-8 : Excel reconnaît l'encodage.
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})
	_, _ = w.Write([]byte("Journal de caisse ; MikCloud\r\n"))
	_, _ = w.Write([]byte(fmt.Sprintf("Date ;%s\r\n", j.report.Date)))
	_, _ = w.Write([]byte(fmt.Sprintf("Revendeur ;%s (%s)\r\n", csvField(resellerName), csvField(c.Sub))))
	_, _ = w.Write([]byte(fmt.Sprintf("Mode ;%s\r\n", mode)))
	_, _ = w.Write([]byte(fmt.Sprintf("Devise ;%s\r\n", csvField(currency))))

	// VENTES — une ligne par ticket remis, canal tracé (audit R4).
	_, _ = w.Write([]byte("\r\nVENTES\r\n"))
	_, _ = w.Write([]byte("Heure ;Code ;Profil ;Prix ;Canal\r\n"))
	for _, s := range j.report.Sold {
		_, _ = w.Write([]byte(fmt.Sprintf("%s ;%s ;%s ;%d ;%s\r\n",
			hm(s.SoldAt), csvField(s.Code), csvField(s.ProfileName), s.Price, sellViaLabel(s.SoldVia))))
	}
	_, _ = w.Write([]byte(fmt.Sprintf("Total ventes ;%d ; ;%d ;\r\n", j.report.SoldCount, j.report.Revenue)))

	// RETOURS — tickets rendus au gérant dans la journée.
	_, _ = w.Write([]byte("\r\nRETOURS\r\n"))
	_, _ = w.Write([]byte("Heure ;Detail ;Montant recredite\r\n"))
	for _, tx := range j.returns {
		_, _ = w.Write([]byte(fmt.Sprintf("%s ;%s ;%d\r\n", hm(tx.At), csvField(tx.Note), tx.Amount)))
	}
	_, _ = w.Write([]byte(fmt.Sprintf("Total retours ; ;%d\r\n", j.report.ReturnedCredited)))

	// VERSEMENTS — encaissements dépôt-vente du jour.
	_, _ = w.Write([]byte("\r\nVERSEMENTS\r\n"))
	_, _ = w.Write([]byte("Heure ;Montant ;Note\r\n"))
	for _, tx := range j.settlements {
		_, _ = w.Write([]byte(fmt.Sprintf("%s ;%d ;%s\r\n", hm(tx.At), tx.Amount, csvField(tx.Note))))
	}
	_, _ = w.Write([]byte(fmt.Sprintf("Total versements ; ;%d\r\n", j.report.SettledToday)))

	// TOTAUX DU JOUR — la synthèse que l'agent comptable attache au journal.
	_, _ = w.Write([]byte("\r\nTOTAUX DU JOUR\r\n"))
	_, _ = w.Write([]byte(fmt.Sprintf("Ventes ;%d ;%d\r\n", j.report.SoldCount, j.report.Revenue)))
	_, _ = w.Write([]byte(fmt.Sprintf("Retours ;%d ;%d\r\n", j.report.ReturnedCount, j.report.ReturnedCredited)))
	_, _ = w.Write([]byte(fmt.Sprintf("Versements ;%d ;%d\r\n", len(j.settlements), j.report.SettledToday)))
	if j.report.PaymentMode == "deposit" {
		_, _ = w.Write([]byte(fmt.Sprintf("A verser (recette du jour) ; ;%d\r\n", j.report.ToDeposit)))
		if isToday {
			_, _ = w.Write([]byte(fmt.Sprintf("Dette en cours ; ;%d\r\n", j.report.DebtTotal)))
		}
	}
	if isToday {
		_, _ = w.Write([]byte(fmt.Sprintf("Stock restant ;%d ;%d\r\n", j.report.StockCount, j.report.StockValue)))
	}
}
