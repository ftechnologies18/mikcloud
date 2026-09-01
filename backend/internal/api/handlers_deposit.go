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

// handleResellerSettle — POST /api/resellers/{id}/settle {amount, note?}.
// Encaissement d'un versement du revendeur en dépôt-vente : la créance
// baisse (Transaction « settlement ») et le cash entre au dashboard/rapports
// via une ligne Sale (reconnaissance à l'encaissement — PAS un doublon :
// aucune Sale n'a été posée à la prise de stock en dépôt-vente).
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
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	if req.Amount <= 0 {
		writeErr(w, http.StatusBadRequest, "Le montant du versement doit être positif")
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
	nowISO := model.NowISO()
	note := req.Note
	if note == "" {
		note = fmt.Sprintf("Versement dépôt-vente de %s", res.Name)
	}
	tx := model.Transaction{
		ID: model.NewID("tx-"), AccountID: acc, Type: "settlement", ResellerID: res.ID, ResellerName: res.Name,
		Amount: req.Amount, Note: note, At: nowISO,
	}
	db.Transactions = append([]model.Transaction{tx}, db.Transactions...)
	// Reconnaissance à l'encaissement : le revenu dépôt-vente entre au
	// dashboard/rapports/compta quand l'argent rentre (une seule écriture).
	db.Sales = append(db.Sales, model.Sale{
		ID: model.NewID("sale-"), AccountID: acc, Amount: req.Amount, ProfileName: "Versement dépôt-vente", Count: 1,
		Channel: "reseller", ResellerName: res.Name,
		At: nowISO, Cost: req.Amount, SellingTotal: req.Amount,
	})
	debtAfter := debt - req.Amount
	a.logActivityBy(r, db, acc, "reseller", fmt.Sprintf("Versement de %d FCFA encaissé de %s — dette restante %d FCFA", req.Amount, res.Name, debtAfter))
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "debtAfter": debtAfter, "transaction": tx})
}
