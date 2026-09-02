package api

// N°19 — Dépôt-vente (revendeur « il vend puis verse ») + plafond de créance.
//
// Deux modes de paiement cohabitent PAR revendeur (Reseller.PaymentMode) :
//   - « prepaid » (historique) : le crédit est débité À LA PRISE de stock —
//     vente reconnue à la génération (Sale + Transaction « sale ») ;
//   - « deposit » : la prise de stock est GRATUITE et bornée par le plafond
//     de créance (Reseller.DebtCeiling) ; la créance naît à la REMISE au
//     client (Transaction « debt », posée par le Mode Vente) et se règle
//     par versement (POST /api/resellers/{id}/settle → Transaction
//     « settlement » + ligne Sale — reconnaissance à l'encaissement).
//
// Règle d'or comptable : une vente = UNE écriture. En dépôt-vente, AUCUNE
// écriture financière à la génération ni au transfert (mouvement de stock
// pur) — dashboard, rapports et compta (consommateurs de db.Sales) ne
// double-comptent pas. Le marqueur HotspotUser.CreditSale (posé à chaque
// attribution) survit aux changements de mode : seul le stock pris à crédit
// crée une créance, même si le revendeur rebascule ensuite en prépayé.
//
// La créance est basée sur le prix GROS (u.Price — ce que le prépayé aurait
// débité) : la marge de rue (SellingPrice) reste le revenu du revendeur.

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	"mikcloud/hotspot-api/internal/model"
)

// depositDebt — créance courante du revendeur : Σ(Transaction « debt ») −
// Σ(Transaction « settlement »). À appeler SOUS store.Lock.
func depositDebt(db *model.DB, acc, resellerID string) int {
	debt := 0
	for _, tx := range db.Transactions {
		if tx.AccountID != acc || tx.ResellerID != resellerID {
			continue
		}
		switch tx.Type {
		case "debt":
			debt += tx.Amount
		case "settlement":
			debt -= tx.Amount
		}
	}
	return debt
}

// depositStockValue — valeur GROSSE du stock encore en portefeuille pris à
// crédit (attribué, non remis, toujours actif). À appeler SOUS store.Lock.
func depositStockValue(db *model.DB, now time.Time, acc, resellerID string) int {
	value := 0
	for i := range db.HotspotUsers {
		u := &db.HotspotUsers[i]
		if u.Kind != "voucher" || u.ResellerID != resellerID || u.AccountID != acc {
			continue
		}
		if u.SoldAt != "" || !u.CreditSale || model.EffectiveStatus(u, now) != "active" {
			continue
		}
		value += u.Price
	}
	return value
}

// depositExposure — exposition totale du gérant : créance déjà née + valeur
// du stock à crédit non encore vendu. Le plafond borne CETTE somme.
func depositExposure(db *model.DB, now time.Time, acc string, res *model.Reseller) int {
	return depositDebt(db, acc, res.ID) + depositStockValue(db, now, acc, res.ID)
}

// modeNote — mention d'audit du mode de paiement dans l'Activity.
func modeNote(mode string, ceiling int) string {
	if mode == "deposit" {
		return fmt.Sprintf(" (dépôt-vente, plafond %d FCFA)", ceiling)
	}
	return ""
}

// depositAgingDays — ancienneté de la créance courante : jours depuis le
// dernier versement (sinon depuis la première créance). Approximation
// volontairement simple : un signal de recouvrement, pas de la compta.
func depositAgingDays(db *model.DB, acc, resellerID string, now time.Time) int {
	lastSettlement, firstDebt := "", ""
	for _, tx := range db.Transactions {
		if tx.AccountID != acc || tx.ResellerID != resellerID {
			continue
		}
		switch tx.Type {
		case "debt":
			if firstDebt == "" || tx.At < firstDebt {
				firstDebt = tx.At
			}
		case "settlement":
			if tx.At > lastSettlement {
				lastSettlement = tx.At
			}
		}
	}
	ref := lastSettlement
	if ref == "" {
		ref = firstDebt
	}
	if ref == "" {
		return 0
	}
	at, err := time.Parse(time.RFC3339, ref)
	if err != nil {
		return 0
	}
	return int(now.Sub(at).Hours() / 24)
}

// buildReceivables — créances revendeurs (dépôt-vente) pour le dashboard :
// argent dormant chez les revendeurs + ancienneté (warn ≥ 7 j, danger ≥ 30 j)
// + verrou (dette > plafond → Mode Vente bloqué). Sous store.Lock.
func buildReceivables(db *model.DB, acc string, now time.Time) map[string]any {
	items := []map[string]any{}
	total, count := 0, 0
	for i := range db.Resellers {
		res := &db.Resellers[i]
		if res.AccountID != acc || res.PaymentMode != "deposit" {
			continue
		}
		debt := depositDebt(db, acc, res.ID)
		if debt <= 0 {
			continue
		}
		aging := depositAgingDays(db, acc, res.ID, now)
		level := "ok"
		if aging >= 30 {
			level = "danger"
		} else if aging >= 7 {
			level = "warn"
		}
		total += debt
		count++
		items = append(items, map[string]any{
			"resellerId": res.ID, "name": res.Name, "debt": debt,
			"ceiling": res.DebtCeiling, "agingDays": aging, "level": level,
			"overCeiling": res.DebtCeiling > 0 && debt > res.DebtCeiling,
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i]["debt"].(int) > items[j]["debt"].(int) })
	return map[string]any{"totalDebt": total, "count": count, "items": items}
}

// handleResellerSettle — POST /api/resellers/{id}/settle {amount, note?, method?}.
// Encaissement d'un versement du revendeur en dépôt-vente : la créance
// baisse (Transaction « settlement ») et le cash entre au dashboard/rapports
// via une ligne Sale (reconnaissance à l'encaissement — PAS un doublon :
// aucune Sale n'a été posée à la prise de stock en dépôt-vente).
//
// N°19 v2 — method=« credit » (défaut « cash ») : compensation. Le versement
// est prélevé sur le crédit prépayé du revendeur (avance dormante héritée de
// l'ère prépayée) au lieu d'être payé au guichet. Comptabilité : Transaction
// « settlement » (créance −) + Transaction « sale » (portefeuille crédit −,
// traçabilité du mouvement) + UNE ligne Sale (le revenu des vouchers pris à
// crédit vendus est reconnu ici — sinon il ne le serait JAMAIS ; le cash de
// cette avance a déjà été encaissé à la recharge, la ligne Sale à la recharge
// n'existant pas, il n'y a AUCUN double comptage).
func (a *API) handleResellerSettle(w http.ResponseWriter, r *http.Request) {
	// P3 — compte expiré : écritures métier refusées (lecture seule).
	if !a.guardAccountWrite(w, r) {
		return
	}
	acc := accountScope(r)
	id := r.PathValue("id")
	var req struct {
		Amount int    `json:"amount"`
		Note   string `json:"note"`
		Method string `json:"method"` // "" | "cash" (défaut) | "credit" — compensation avec le crédit prépayé
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	if req.Amount <= 0 {
		writeErr(w, http.StatusBadRequest, "Le montant du versement doit être positif")
		return
	}
	if req.Method == "" {
		req.Method = "cash"
	}
	if req.Method != "cash" && req.Method != "credit" {
		writeErr(w, http.StatusBadRequest, "Méthode invalide (cash ou credit)")
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
	if res.PaymentMode != "deposit" {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, "Versement réservé aux revendeurs en dépôt-vente")
		return
	}
	debt := depositDebt(db, acc, res.ID)
	if debt <= 0 {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, "Aucune dette à régler pour ce revendeur")
		return
	}
	if req.Amount > debt {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, fmt.Sprintf("Montant supérieur à la dette (dette: %d, reçu: %d)", debt, req.Amount))
		return
	}
	// N°19 v2 — compensation : l'avance prépayée du revendeur paie sa dette
	// (accord au guichet formalisé). Dernière validation avant écriture.
	if req.Method == "credit" {
		if res.Credit < req.Amount {
			a.store.Unlock()
			writeErr(w, http.StatusBadRequest, fmt.Sprintf("Crédit prépayé insuffisant (crédit: %d, requis: %d)", res.Credit, req.Amount))
			return
		}
		res.Credit -= req.Amount
	}
	nowISO := model.NowISO()
	note := req.Note
	if note == "" {
		if req.Method == "credit" {
			note = fmt.Sprintf("Compensation dette avec crédit prépayé de %s", res.Name)
		} else {
			note = fmt.Sprintf("Versement dépôt-vente de %s", res.Name)
		}
	}
	tx := model.Transaction{
		ID: model.NewID("tx-"), AccountID: acc, Type: "settlement", ResellerID: res.ID, ResellerName: res.Name,
		Amount: req.Amount, Note: note, At: nowISO,
	}
	db.Transactions = append([]model.Transaction{tx}, db.Transactions...)
	if req.Method == "credit" {
		// Traçabilité du portefeuille : le crédit prépayé baisse d'autant
		// (mouvement tracé comme toute mutation du solde).
		walletTx := model.Transaction{
			ID: model.NewID("tx-"), AccountID: acc, Type: "sale", ResellerID: res.ID, ResellerName: res.Name,
			Amount: req.Amount, Note: "Crédit prépayé converti en versement — compensation dette", At: nowISO,
		}
		db.Transactions = append([]model.Transaction{walletTx}, db.Transactions...)
	}
	// Reconnaissance à l'encaissement : le revenu dépôt-vente entre au
	// dashboard/rapports/compta quand l'argent rentre (une seule écriture).
	// En compensation, « l'encaissement » = conversion de l'avance prépayée
	// en paiement de la dette (label distinct pour l'audit).
	saleLabel := "Versement dépôt-vente"
	if req.Method == "credit" {
		saleLabel = "Compensation dette-crédit"
	}
	db.Sales = append(db.Sales, model.Sale{
		ID: model.NewID("sale-"), AccountID: acc, Amount: req.Amount, ProfileName: saleLabel, Count: 1,
		Channel: "reseller", ResellerName: res.Name,
		At: nowISO, Cost: req.Amount, SellingTotal: req.Amount,
	})
	debtAfter := debt - req.Amount
	activityMsg := fmt.Sprintf("Versement de %d FCFA encaissé de %s — dette restante %d FCFA", req.Amount, res.Name, debtAfter)
	if req.Method == "credit" {
		activityMsg = fmt.Sprintf("Compensation de %d FCFA avec le crédit prépayé de %s — dette restante %d FCFA, crédit restant %d FCFA", req.Amount, res.Name, debtAfter, res.Credit)
	}
	a.logActivityBy(r, db, acc, "reseller", activityMsg)
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "debtAfter": debtAfter, "creditAfter": res.Credit, "transaction": tx})
}
