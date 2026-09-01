package api

// N°18 — Transfert de stock : redistribution a posteriori de lots déjà générés.
//
// Le modèle historique attribue les vouchers à un revendeur UNIQUEMENT à la
// génération (canal « reseller », crédit débité, Transaction « sale »). Ce
// fichier ajoute le maillon manquant du circuit de distribution : le gérant
// ou le propriétaire (requireRole 2) redistribue le stock VENDABLE d'un lot
// déjà généré — vers un revendeur (distribution) ou vers le stock direct
// (retour de stock).
//
// Règles d'or :
//  1. Transférer = changer la propriété (ResellerID/ResellerName), JAMAIS
//     dupliquer : zéro write RouterOS (le routeur ignore les revendeurs) —
//     le transfert fonctionne même routeur hors ligne.
//  2. Seuls les vouchers VENDABLES partent : statut effectif « active »
//     (EffectiveStatus : expiration et limit-uptime résolues) et jamais
//     remis (SoldAt vide). Un ticket remis à un client ne bouge plus
//     (anti-fraude) ; les used/expired/disabled restent dans leur
//     attribution d'origine (audit).
//  3. Transfert partiel : quantity ≤ stock transférable ; à quantité égale,
//     les tickets les plus récemment générés partent en premier (les vieux
//     restent au comptoir pour s'écouler vite — même rotation que le stock
//     du Mode Vente).
//  4. L'argent suit le transfert : chaque voucher ENTRANT chez un revendeur
//     débite son portefeuille de son prix facial (u.Price — ce que la
//     génération lui aurait débité) ; chaque voucher SORTANT d'un revendeur
//     lui est recrédité (retour de stock ; le stock direct ne paie rien).
//     AUCUNE ligne Sale : les Sales restent liés à la génération (émission
//     de stock) — dashboard, rapports et compta ne double-comptent pas.
//  5. Traçabilité : activity avec l'acteur (gérant/propriétaire) + une
//     Transaction par mouvement de portefeuille.
//
// Le lot (Batch) reste IMMUABLE : Channel/ResellerID décrivent la GÉNÉRATION
// (provenance). La possession live est recalculée à la lecture depuis les
// vouchers (holdings — voir handleBatchesList), comme les stats de lots.

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"mikcloud/hotspot-api/internal/model"
)

// transferTargetDirect — valeur spéciale de resellerId : rappatrier le stock
// vers la réserve du gérant (retour de stock, recrédite les revendeurs).
const transferTargetDirect = "direct"

// transferRequest — corps de POST /api/vouchers/batch/{batchId}/transfer.
type transferRequest struct {
	// ResellerID cible : identifiant de revendeur, ou « direct » pour un
	// retour de stock.
	ResellerID string `json:"resellerId"`
	// Quantity — nombre de vouchers à transférer (0/omis = tout le stock
	// transférable pour cette destination).
	Quantity int `json:"quantity"`
	// ExcludeExpiringDays — garde-fou stock mort : exclure les vouchers
	// expirant dans les N prochains jours (0 = aucun filtre).
	ExcludeExpiringDays int `json:"excludeExpiringDays"`
}

// transferRefund — restitution au portefeuille d'un revendeur qui cède du
// stock (retour de stock / ré-affectation).
type transferRefund struct {
	ResellerID   string `json:"resellerId"`
	ResellerName string `json:"resellerName"`
	Amount       int    `json:"amount"`
	CreditAfter  int    `json:"creditAfter"`
}

// handleVouchersBatchTransfer — POST /api/vouchers/batch/{batchId}/transfer
// {resellerId: "<id>"|"direct", quantity?, excludeExpiringDays?} (requireRole 2).
// Répond {transferred, debited, credited, creditAfter, refunds[], vouchers[]}
// — vouchers = les tickets transférés (pour l'impression A4 immédiate).
func (a *API) handleVouchersBatchTransfer(w http.ResponseWriter, r *http.Request) {
	// P3 — compte expiré : écritures métier refusées (lecture seule).
	if !a.guardAccountWrite(w, r) {
		return
	}
	acc := accountScope(r)
	var req transferRequest
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	req.ResellerID = strings.TrimSpace(req.ResellerID)
	if req.ResellerID == "" {
		writeErr(w, http.StatusBadRequest, "Destination requise : identifiant de revendeur ou « direct »")
		return
	}
	if req.Quantity < 0 {
		req.Quantity = 0
	}
	if req.ExcludeExpiringDays < 0 {
		req.ExcludeExpiringDays = 0
	}

	now := time.Now().UTC()
	a.store.Lock()
	defer a.store.Unlock()
	db := a.store.Data()

	// Lot scopé au compte.
	batchID := r.PathValue("batchId")
	var batch *model.Batch
	for i := range db.Batches {
		if db.Batches[i].ID == batchID && db.Batches[i].AccountID == acc {
			batch = &db.Batches[i]
			break
		}
	}
	if batch == nil {
		writeErr(w, http.StatusNotFound, "Lot introuvable")
		return
	}

	// Cible : revendeur actif du compte, ou stock direct.
	var target *model.Reseller
	if req.ResellerID != transferTargetDirect {
		target = findResellerScoped(db, req.ResellerID, acc)
		if target == nil {
			writeErr(w, http.StatusNotFound, "Revendeur introuvable")
			return
		}
		if target.Status != "active" {
			writeErr(w, http.StatusBadRequest, "Revendeur désactivé — réactivez-le avant de lui transférer du stock")
			return
		}
	}

	// Collecte des transférables (règle d'or 2) : vouchers actifs jamais
	// remis, qui ne sont pas DÉJÀ chez la destination. Garde-fou optionnel
	// sur l'expiration imminente.
	type candidate struct{ idx int }
	cands := []candidate{}
	for i := range db.HotspotUsers {
		u := &db.HotspotUsers[i]
		if u.AccountID != acc || u.Kind != "voucher" || u.BatchID != batch.ID {
			continue
		}
		if u.SoldAt != "" || model.EffectiveStatus(u, now) != "active" {
			continue // remis à un client, consommé, expiré ou désactivé : ça ne bouge pas
		}
		if target != nil {
			if u.ResellerID == target.ID {
				continue // déjà chez le destinataire
			}
		} else if u.ResellerID == "" {
			continue // déjà en stock direct
		}
		if req.ExcludeExpiringDays > 0 {
			if exp, err := time.Parse(time.RFC3339, u.ExpiresAt); err == nil && exp.Before(now.AddDate(0, 0, req.ExcludeExpiringDays)) {
				continue // expire trop vite : le gérant l'a exclu
			}
		}
		cands = append(cands, candidate{idx: i})
	}
	if len(cands) == 0 {
		writeErr(w, http.StatusConflict,
			"Aucun voucher transférable dans ce lot pour cette destination (vendus, consommés, expirés ou déjà chez le destinataire)")
		return
	}
	// Règle d'or 3 — rotation : les plus récemment générés partent en premier.
	sort.Slice(cands, func(i, j int) bool {
		return db.HotspotUsers[cands[i].idx].CreatedAt > db.HotspotUsers[cands[j].idx].CreatedAt
	})
	n := len(cands)
	if req.Quantity > 0 && req.Quantity < n {
		n = req.Quantity
	}

	// Règle d'or 4 — l'argent suit le transfert. Débit : le revendeur cible
	// paie le prix facial de chaque ticket entrant. Crédit : chaque revendeur
	// qui cède du stock est recrédité (le stock direct ne paie ni ne reçoit).
	debitTotal := 0
	creditByReseller := map[string]int{}
	countByReseller := map[string]int{}
	for _, c := range cands[:n] {
		u := &db.HotspotUsers[c.idx]
		if target != nil {
			debitTotal += u.Price
		}
		if u.ResellerID != "" {
			creditByReseller[u.ResellerID] += u.Price
			countByReseller[u.ResellerID]++
		}
	}
	if target != nil && debitTotal > target.Credit {
		writeErr(w, http.StatusBadRequest,
			fmt.Sprintf("Crédit insuffisant (disponible: %d, requis: %d)", target.Credit, debitTotal))
		return
	}

	// Application (règle d'or 1 — changement de propriété, rien d'autre).
	nowISO := model.NowISO()
	transferred := make([]model.HotspotUser, 0, n)
	for _, c := range cands[:n] {
		u := &db.HotspotUsers[c.idx]
		if target != nil {
			u.ResellerID, u.ResellerName = target.ID, target.Name
		} else {
			u.ResellerID, u.ResellerName = "", ""
		}
		transferred = append(transferred, *u)
	}

	// Mouvements de portefeuille + Transactions (règles d'or 4 et 5).
	creditAfter := 0
	if target != nil && debitTotal > 0 {
		target.Credit -= debitTotal
		creditAfter = target.Credit
		db.Transactions = append([]model.Transaction{{
			ID: model.NewID("tx-"), AccountID: acc, Type: "sale",
			ResellerID: target.ID, ResellerName: target.Name, Amount: debitTotal,
			Note: fmt.Sprintf("Transfert de stock : %d voucher(s) du lot %s", len(transferred), batch.ID),
			At:   nowISO,
		}}, db.Transactions...)
	}
	refunds := []transferRefund{}
	credited := 0
	for srcID, amount := range creditByReseller {
		src := findResellerScoped(db, srcID, acc)
		if src == nil {
			continue // revendeur supprimé entre-temps : rien à recréditer
		}
		src.Credit += amount
		credited += amount
		refunds = append(refunds, transferRefund{
			ResellerID: src.ID, ResellerName: src.Name, Amount: amount, CreditAfter: src.Credit,
		})
		db.Transactions = append([]model.Transaction{{
			ID: model.NewID("tx-"), AccountID: acc, Type: "credit",
			ResellerID: src.ID, ResellerName: src.Name, Amount: amount,
			Note: fmt.Sprintf("Retour de stock : %d voucher(s) du lot %s", countByReseller[srcID], batch.ID),
			At:   nowISO,
		}}, db.Transactions...)
	}

	// Règle d'or 5 — activity horodatée avec l'acteur (gérant/propriétaire).
	if target != nil {
		a.logActivityBy(r, db, acc, "voucher",
			fmt.Sprintf("Transfert du lot %s : %d voucher(s) → %s (débité : %d)", batch.ID, len(transferred), target.Name, debitTotal))
	} else {
		names := make([]string, 0, len(refunds))
		for _, rf := range refunds {
			names = append(names, rf.ResellerName)
		}
		a.logActivityBy(r, db, acc, "voucher",
			fmt.Sprintf("Retour de stock du lot %s : %d voucher(s) ← %s (recrédité : %d)",
				batch.ID, len(transferred), strings.Join(names, ", "), credited))
	}
	a.store.Save()

	writeJSON(w, http.StatusOK, map[string]any{
		"transferred": len(transferred),
		"debited":     debitTotal,
		"credited":    credited,
		"creditAfter": creditAfter,
		"refunds":     refunds,
		"vouchers":    transferred,
	})
}
