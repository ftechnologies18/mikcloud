// handlers_vouchers.go — vouchers : génération, listes, lots, suppression en masse.

package api

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"mikcloud/hotspot-api/internal/agent"
	"mikcloud/hotspot-api/internal/model"
)

func (a *API) handleVouchersList(w http.ResponseWriter, r *http.Request) {
	a.usersList(w, r, "voucher")
}

// N°22 — Impression tracée : SEUL canal de sortie des codes des tickets
// revendeur depuis la console gérant.
//
// Contexte : depuis le masquage des listes (maskResellerCodes), la console
// n'expose plus les codes des tickets attribués (ResellerID != "") — le gérant
// ne peut plus les dicter au comptoir ni les recopier. Mais il reste
// L'IMPRIMEUR DE SERVICE du revendeur : celui-ci n'a pas toujours d'imprimante
// et demande au gérant d'imprimer ses tickets (thermique 58/80 mm, grille A4).
// Cet endpoint répond à ce besoin légitime SANS rouvrir la fuite :
//   - il délivre les codes COMPLETS uniquement pour l'impression en cours ;
//   - toute remise de codes revendeur est TRACÉE dans le journal d'activité
//     (anti-litige : horodatée, attribuée à l'acteur connecté, décompte par
//     revendeur) ;
//   - la propriété ne change PAS : le ticket reste chez le revendeur et la
//     vente auto (auto_connect) continue de créditer SON compte à la première
//     connexion du client ;
//   - le stock direct du gérant s'imprime comme avant, sans tracé.

// vouchersPrintRequest — corps de POST /api/vouchers/print.
type vouchersPrintRequest struct {
	IDs []string `json:"ids"`
}

// handleVouchersPrint — POST /api/vouchers/print {ids:[...]} (requireRole 2).
// Répond {vouchers:[...], tracedCount:N} — vouchers complets (codes réels),
// scopés au compte ; tracedCount = tickets revendeur tracés dans l'activité.
func (a *API) handleVouchersPrint(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	var req vouchersPrintRequest
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	if len(req.IDs) == 0 {
		writeErr(w, http.StatusBadRequest, "Aucun ticket demandé")
		return
	}
	// Plafond = cap de génération : un lot thermique / une grille A4 ne
	// dépasse jamais quelques centaines de tickets.
	if len(req.IDs) > 500 {
		writeErr(w, http.StatusBadRequest, "Maximum 500 tickets par impression")
		return
	}
	wanted := make(map[string]bool, len(req.IDs))
	for _, id := range req.IDs {
		if id = strings.TrimSpace(id); id != "" {
			wanted[id] = true
		}
	}

	a.store.Lock()
	db := a.store.Data()
	out := make([]model.HotspotUser, 0, len(wanted))
	byReseller := map[string]int{}
	resNames := map[string]string{}
	for i := range db.HotspotUsers {
		u := &db.HotspotUsers[i]
		if u.AccountID != acc || !wanted[u.ID] {
			continue
		}
		out = append(out, *u)
		if u.ResellerID != "" {
			byReseller[u.ResellerID]++
			resNames[u.ResellerID] = u.ResellerName
		}
	}
	// Traçabilité — uniquement les tickets revendeur (le stock direct du
	// gérant s'imprime sans journal : comportement historique conservé).
	resCount := 0
	details := make([]string, 0, len(byReseller))
	for id, n := range byReseller {
		resCount += n
		details = append(details, fmt.Sprintf("%s : %d", resNames[id], n))
	}
	if resCount > 0 {
		sort.Strings(details)
		a.logActivityBy(r, db, acc, "voucher",
			fmt.Sprintf("Codes remis pour impression : %d ticket(s) revendeur(s) — %s", resCount, strings.Join(details, ", ")))
		a.store.Save()
	}
	a.store.Unlock()

	if len(out) == 0 {
		writeErr(w, http.StatusNotFound, "Aucun ticket trouvé pour ces identifiants")
		return
	}
	// Tri création décroissante : ordre stable, identique aux listes.
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	writeJSON(w, http.StatusOK, map[string]any{
		"vouchers":    out,
		"tracedCount": resCount,
	})
}

// vouchersRepriseRequest — corps de POST /api/vouchers/reprise.
type vouchersRepriseRequest struct {
	IDs []string `json:"ids"`
}

// handleVouchersReprise — N°23 (W6) — reprise initiée par le GÉRANT : il
// reprend au revendeur des tickets encore invendus (revendeur absent,
// litige, désengagement). Miroir exact du retour de stock du revendeur
// (N°20, handleSellReturn) mais à l'initiative du gérant (requireRole 2),
// scopé au compte :
//
//   - règle d'or 1 (N°18) : changer la propriété, jamais dupliquer —
//     ResellerID/ResellerName/CreditSale vidés (retour au stock direct) ;
//   - règle d'or 4 : l'argent suit le retour, mais seulement pour du stock
//     VIVANT — actif : prépayé → portefeuille recrédité du prix GROS
//     (u.Price, ce qui avait été débité) + UNE Transaction « credit »
//     agrégée par revendeur ; dépôt-vente → aucun recrédit (rien n'avait
//     été débité à la prise) ; expiré/désactivé → aucun recrédit (le
//     ticket a péri chez le revendeur) : la reprise sert alors à fermer la
//     boucle (le gérant peut nettoyer, le revendeur voit le stock partir) ;
//   - règle d'or 5 : Activity avec le gérant comme acteur (audit, décompte
//     par revendeur et recrédits).
//
// Refus (409, tout-le-lot — cohérent avec les refus idempotents N°20) :
// tickets déjà REMIS au client (SoldAt != « » — la créance est née, le
// ticket en est la preuve : la reprise n'existe pas pour du stock vendu) et
// tickets inconnus/hors compte/stock direct du gérant.
func (a *API) handleVouchersReprise(w http.ResponseWriter, r *http.Request) {
	// P3 — compte expiré : écritures métier refusées (lecture seule).
	if !a.guardAccountWrite(w, r) {
		return
	}
	acc := accountScope(r)
	var req vouchersRepriseRequest
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	if len(req.IDs) == 0 {
		writeErr(w, http.StatusBadRequest, "Aucun ticket sélectionné pour la reprise")
		return
	}
	if len(req.IDs) > 500 {
		writeErr(w, http.StatusBadRequest, "Trop de tickets en une fois (500 maximum)")
		return
	}
	wanted := make(map[string]bool, len(req.IDs))
	for _, id := range req.IDs {
		if id = strings.TrimSpace(id); id != "" {
			wanted[id] = true
		}
	}
	if len(wanted) == 0 {
		writeErr(w, http.StatusBadRequest, "Aucun ticket sélectionné pour la reprise")
		return
	}

	now := time.Now().UTC()
	a.store.Lock()
	defer a.store.Unlock()
	db := a.store.Data()

	// Résolution scopée : chaque id doit être un ticket ATTRIBUÉ du compte
	// (ResellerID != "", AccountID == acc, Kind == "voucher").
	found := map[string]*model.HotspotUser{}
	for i := range db.HotspotUsers {
		u := &db.HotspotUsers[i]
		if !wanted[u.ID] || u.ResellerID == "" || u.AccountID != acc || u.Kind != "voucher" {
			continue
		}
		found[u.ID] = u
	}
	rejected := []string{}
	for id := range wanted {
		u := found[id]
		switch {
		case u == nil:
			rejected = append(rejected, id) // inconnu, hors compte ou stock direct
		case u.SoldAt != "":
			rejected = append(rejected, u.Username) // déjà remis au client
		}
	}
	if len(rejected) > 0 {
		writeErr(w, http.StatusConflict,
			fmt.Sprintf("Ticket(s) non reprisable(s) : %s (déjà remis au client, stock direct ou introuvable)",
				strings.Join(rejected, ", ")))
		return
	}

	// Décompte par revendeur + recrédit du stock VIVANT (prix GROS).
	byReseller := map[string]int{}  // resellerID → tickets repris
	resNames := map[string]string{} // resellerID → nom (audit)
	liveValue := map[string]int{}   // resellerID → valeur recréditable (stock actif)
	for _, u := range found {
		byReseller[u.ResellerID]++
		resNames[u.ResellerID] = u.ResellerName
		if model.EffectiveStatus(u, now) == "active" {
			liveValue[u.ResellerID] += u.Price
		}
	}
	credited := 0
	for resID, total := range liveValue {
		res := findResellerScoped(db, resID, acc)
		if res == nil || total == 0 || res.PaymentMode == "deposit" {
			continue // dépôt-vente ou revendeur disparu : aucun recrédit
		}
		res.Credit += total
		credited += total
		db.Transactions = append([]model.Transaction{{
			ID: model.NewID("tx-"), AccountID: acc, Type: "credit",
			ResellerID: resID, ResellerName: res.Name, Amount: total,
			Note: fmt.Sprintf("Reprise par le gérant : %d voucher(s) repris à %s (Mode Vente)",
				byReseller[resID], res.Name),
			At: model.NowISO(),
		}}, db.Transactions...)
	}

	// Application — retour au stock direct (règle d'or 1).
	returned := make([]string, 0, len(found))
	for _, u := range found {
		returned = append(returned, u.Username)
		u.ResellerID, u.ResellerName = "", ""
		u.CreditSale = false
	}
	sort.Strings(returned) // message d'audit stable

	details := make([]string, 0, len(byReseller))
	for resID, n := range byReseller {
		label := fmt.Sprintf("%s : %d ticket(s)", resNames[resID], n)
		if c := liveValue[resID]; c > 0 {
			label += fmt.Sprintf(", recrédité : %d", c)
		} else {
			label += ", aucun recrédit"
		}
		details = append(details, label)
	}
	sort.Strings(details)
	a.logActivityBy(r, db, acc, "voucher",
		fmt.Sprintf("Reprise de stock : %d voucher(s) [%s] repris au stock direct par le gérant — %s",
			len(returned), strings.Join(returned, ", "), strings.Join(details, " ; ")))
	a.store.Save()

	writeJSON(w, http.StatusOK, map[string]any{
		"returned": len(returned),
		"credited": credited,
		"codes":    returned,
		"details":  details,
	})
}

func (a *API) handleVouchersGenerate(w http.ResponseWriter, r *http.Request) {
	// P3 — compte expiré : écritures métier refusées (lecture seule).
	if !a.guardAccountWrite(w, r) {
		return
	}
	acc := accountScope(r)
	var req struct {
		Count       int    `json:"count"`
		ProfileID   string `json:"profileId"`
		RouterID    string `json:"routerId"`
		Prefix      string `json:"prefix"`
		CodeLength  int    `json:"codeLength"`
		ResellerID  string `json:"resellerId"`
		UserMode    string `json:"userMode"`    // N°25 — IGNORÉ (verrou) : toujours "same", voir plus bas
		Charset     string `json:"charset"`     // preset model.Charset* ("" = MikCloud sûr)
		Comment     string `json:"comment"`     // commentaire libre inscrit sur le routeur
		DataQuotaMb *int   `json:"dataQuotaMb"` // nil = hériter du profil · 0 = illimité · >0 = Mo
		// Parité Mikhmon : Time Limit par lot (limit-uptime, minutes ; 0 =
		// hériter du sessionTimeoutMin du profil) + serveur hotspot optionnel.
		TimeLimitMin int    `json:"timeLimitMin"`
		Server       string `json:"server"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	if req.Count < 1 || req.Count > 500 {
		writeErr(w, http.StatusBadRequest, "Le nombre de vouchers doit être compris entre 1 et 500")
		return
	}
	codeLength := req.CodeLength
	if codeLength == 0 {
		codeLength = 5
	}
	// 3 à 8 caractères comme Mikhmon (parité), jusqu'à 10 pour les codes
	// personnalisés ; l'alphabet choisi exclut toujours les caractères ambigus.
	if codeLength < 3 || codeLength > 10 {
		writeErr(w, http.StatusBadRequest, "La longueur du code doit être comprise entre 3 et 10")
		return
	}
	// Préfixe optionnel : vide = AUCUN préfixe (parité Mikhmon) — le code du
	// ticket est alors le code généré seul, sans valeur par défaut.
	prefix := strings.TrimSpace(req.Prefix)
	// Parité Mikhmon : préfixe borné à 6 caractères (maxlength du formulaire
	// Mikhmon) — les codes restent courts et imprimables sur les tickets.
	if len(prefix) > 6 {
		writeErr(w, http.StatusBadRequest, "Le préfixe est limité à 6 caractères")
		return
	}
	if req.UserMode != "" && req.UserMode != "userpass" && req.UserMode != "same" {
		writeErr(w, http.StatusBadRequest, "Mode utilisateur invalide (userpass ou same)")
		return
	}
	// N°25 — VERROU métier : tout voucher est créé en mode « nom d'utilisateur
	// = mot de passe » (code unique). La valeur du client est ignorée — même un
	// appel API direct (console, PWA, script) reçoit des tickets à code unique.
	samePassword := true
	voucherComment := sanitizeVoucherComment(req.Comment)
	now := time.Now().UTC()

	// Validation + génération des vouchers (sous verrou)
	a.store.Lock()
	db := a.store.Data()
	profile := findProfileScoped(db, strings.TrimSpace(req.ProfileID), acc)
	if profile == nil {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, "Profil introuvable")
		return
	}
	router := findRouterScoped(db, strings.TrimSpace(req.RouterID), acc)
	if router == nil {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, "Routeur introuvable")
		return
	}
	routerCopy := *router
	var resellerCopy *model.Reseller
	if strings.TrimSpace(req.ResellerID) != "" {
		res := findResellerScoped(db, strings.TrimSpace(req.ResellerID), acc)
		if res == nil {
			a.store.Unlock()
			writeErr(w, http.StatusBadRequest, "Revendeur introuvable")
			return
		}
		c := *res
		resellerCopy = &c
	}
	// Quota de données (l'argument « 5 Go = 500 F ») : par défaut le voucher
	// hérite du quota du profil ; le gérant surcharge par lot (0 = illimité
	// explicite, > 0 = Mo par voucher, plafonné à 1 Po).
	quotaMb := profile.DataQuotaMb
	if req.DataQuotaMb != nil {
		quotaMb = *req.DataQuotaMb
	}
	if quotaMb < 0 {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, "Le quota de données ne peut pas être négatif")
		return
	}
	if quotaMb > 1_073_741_824 {
		quotaMb = 1_073_741_824
	}
	cost := req.Count * profile.Price
	// Sécurité P0 — garde anti-débordement (défense en profondeur) : le plafond
	// prixMaxProfil × count ≤ 500 rend le débordement impossible sur données
	// neuves, mais les profils hérités d'avant plafond restent défendus ici —
	// sans elle, cost négatif faisait passer le contrôle de crédit puis
	// RECREDITAIT le portefeuille du revendeur (Reseller.Credit -= cost).
	if profile.Price > 0 && req.Count > math.MaxInt/profile.Price {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, "Coût du lot trop élevé — corrigez le prix du profil ou réduisez la quantité")
		return
	}
	if resellerCopy != nil && resellerCopy.PaymentMode == "deposit" {
		// N°19 — dépôt-vente : rien à débiter, mais exposition bornée par le
		// plafond de créance (dette née + stock à crédit + ce lot ≤ plafond).
		debtNow := depositDebt(db, acc, resellerCopy.ID)
		stockNow := depositStockValue(db, time.Now().UTC(), acc, resellerCopy.ID)
		if debtNow+stockNow+cost > resellerCopy.DebtCeiling {
			a.store.Unlock()
			writeErr(w, http.StatusBadRequest, fmt.Sprintf("Plafond de créance dépassé (dette: %d, stock: %d, plafond: %d, requis: %d)", debtNow, stockNow, resellerCopy.DebtCeiling, cost))
			return
		}
	} else if resellerCopy != nil && resellerCopy.Credit < cost {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, fmt.Sprintf("Crédit insuffisant (disponible: %d, requis: %d)", resellerCopy.Credit, cost))
		return
	}
	// P0 (F13) : prix de vente (0 = même prix que le coût) et totaux marge.
	selling := profile.Price
	if profile.SellingPrice > 0 {
		selling = profile.SellingPrice
	}
	sellingTotal := selling * req.Count

	batchID := fmt.Sprintf("B%s-%04d", now.Format("20060102"), now.Nanosecond()%10000)
	expiresAt := "" // validité ancrée au 1er login — vide tant que le ticket n'est pas connecté
	// Parité Mikhmon : Time Limit (limit-uptime) par lot — 0 = hériter du
	// sessionTimeoutMin du profil. Résolu une fois, tracé et poussé au routeur.
	if req.TimeLimitMin < 0 || req.TimeLimitMin > 2628000 {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, "Le quota de temps doit être compris entre 0 et 2628000 minutes")
		return
	}
	timeLimitMin := int64(req.TimeLimitMin)
	if timeLimitMin <= 0 {
		timeLimitMin = int64(profile.SessionTimeoutMin)
	}
	server := strings.TrimSpace(req.Server)
	if server != "" && (len(server) > 64 || strings.ContainsAny(server, "\"\n\r")) {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, "Nom de serveur hotspot invalide")
		return
	}
	vouchers := make([]model.HotspotUser, 0, req.Count)
	for i := 0; i < req.Count; i++ {
		code := model.RandomCodeFrom(codeLength, req.Charset)
		for j := 0; j < 50 && usernameTaken(db, acc, prefix+code); j++ {
			code = model.RandomCodeFrom(codeLength, req.Charset)
		}
		// Mode « same » : le mot de passe = le nom d'utilisateur COMPLET (avec préfixe).
		password := prefix + code
		if !samePassword {
			// Mode « Username & Password » (défaut) : le mot de passe diffère du code.
			password = model.RandomCodeFrom(codeLength, req.Charset)
			for password == code {
				password = model.RandomCodeFrom(codeLength, req.Charset)
			}
		}
		u := model.HotspotUser{
			ID: model.NewID("v-"), AccountID: acc, Kind: "voucher", Username: prefix + code, Password: password,
			ProfileID: profile.ID, ProfileName: profile.Name,
			RouterID: routerCopy.ID, RouterName: routerCopy.Name,
			Status: "active", BatchID: batchID,
			CreatedAt: model.NowISO(), ExpiresAt: expiresAt, UsedAt: "",
			Price: profile.Price, SellingPrice: profile.SellingPrice, DataQuotaMb: int64(quotaMb),
			TimeLimitMin: timeLimitMin,
		}
		if resellerCopy != nil {
			u.ResellerID = resellerCopy.ID
			u.ResellerName = resellerCopy.Name
			// N°19 — dépôt-vente : ticket pris à crédit ; sa remise créera la
			// créance (le marqueur survit aux changements de mode).
			u.CreditSale = resellerCopy.PaymentMode == "deposit"
		}
		vouchers = append(vouchers, u)
	}
	a.store.Unlock()

	// Application : mode agent → file de commandes ; sinon gateway (simulée/réelle).
	if routerCopy.Mode == "agent" {
		names := make([]map[string]any, 0, len(vouchers))
		for i := range vouchers {
			vouchers[i].Username = agent.SanitizeName(vouchers[i].Username)
			vouchers[i].ProfileName = agent.SanitizeName(vouchers[i].ProfileName)
			names = append(names, map[string]any{"name": vouchers[i].Username, "password": vouchers[i].Password})
		}
		a.store.Lock()
		db = a.store.Data()
		db.HotspotUsers = append(db.HotspotUsers, vouchers...)
		batchPayload := map[string]any{
			"profile": profileRef(*profile), "users": names, "batch": batchID,
		}
		if quotaMb > 0 {
			// limit-bytes-total s'exprime en octets sur le routeur (Mo × 1 048 576).
			batchPayload["limitBytesTotal"] = quotaMb * 1048576
		}
		if voucherComment != "" {
			batchPayload["comment"] = voucherComment
		}
		if timeLimitMin > 0 {
			// Parité Mikhmon : limit-uptime du lot (minutes → RouterOS).
			batchPayload["limitUptimeMin"] = timeLimitMin
		}
		if server != "" {
			batchPayload["server"] = server
		}
		cmd := queueCommandLocked(db, routerCopy.AccountID, routerCopy.ID, model.CmdVoucherBatch, batchPayload)
		a.logActivityBy(r, db, acc, "voucher", fmt.Sprintf("Lot %s : %d vouchers en file pour «%s»%s (commande %s)", batchID, req.Count, routerCopy.Name, quotaNote(quotaMb), cmd.ID))
		// Bookkeeping : vente, transaction, portefeuille revendeur
		channel := "direct"
		resName := ""
		if resellerCopy != nil {
			channel = "reseller"
			resName = resellerCopy.Name
			// N°19 — en dépôt-vente, la prise de stock est gratuite : la
			// créance naîtra à la remise (Mode Vente), pas à la génération.
			if resellerCopy.PaymentMode != "deposit" {
				if res := findResellerScoped(db, resellerCopy.ID, acc); res != nil {
					res.Credit -= cost
				}
				db.Transactions = append([]model.Transaction{{
					ID: model.NewID("tx-"), AccountID: acc, Type: "sale", ResellerID: resellerCopy.ID, ResellerName: resName,
					Amount: cost, Note: fmt.Sprintf("Achat de %d vouchers (%s)", req.Count, profile.Name),
					At: model.NowISO(),
				}}, db.Transactions...)
			}
		}
		// N°19 — pas de Sale à la génération en dépôt-vente (reconnaissance à
		// l'encaissement au versement) : dashboard/rapports/compta ne
		// double-comptent pas.
		if resellerCopy == nil || resellerCopy.PaymentMode != "deposit" {
			db.Sales = append(db.Sales, model.Sale{
				ID: model.NewID("sale-"), AccountID: acc, Amount: cost, ProfileName: profile.Name, Count: req.Count,
				Channel: channel, ResellerName: resName,
				RouterID: routerCopy.ID, RouterName: routerCopy.Name, BatchID: batchID,
				At: model.NowISO(), Cost: cost, SellingTotal: sellingTotal,
			})
		}
		batchResellerID := ""
		if resellerCopy != nil {
			batchResellerID = resellerCopy.ID
		}
		db.Batches = append([]model.Batch{{
			ID: batchID, AccountID: acc, ProfileID: profile.ID, ProfileName: profile.Name,
			RouterID: routerCopy.ID, RouterName: routerCopy.Name,
			Count: req.Count, UnitPrice: profile.Price, TotalCost: cost, DataQuotaMb: int64(quotaMb), TimeLimitMin: timeLimitMin,
			Channel: channel, ResellerID: batchResellerID, ResellerName: resName,
			CreatedAt: model.NowISO(),
		}}, db.Batches...)
		a.store.Save()
		cmdID := cmd.ID
		a.store.Unlock()
		data, _ := json.Marshal(map[string]any{
			"batchId": batchID, "vouchers": vouchers, "totalCost": cost,
			"queued": true, "commandId": cmdID,
			"message": "Vouchers en file : créés sur le routeur au prochain check-in (≤ 45 s)",
		})
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write(data)
		return
	}

	// Application via la gateway (simulée : écrit dans le store ; réelle : pousse au routeur)
	gw := a.gatewayFor(routerCopy)
	for i := range vouchers {
		if err := gw.AddUser(&vouchers[i]); err != nil {
			writeErr(w, http.StatusBadRequest, "Création impossible : "+err.Error())
			return
		}
	}

	// Bookkeeping : vente, transaction, portefeuille revendeur
	a.store.Lock()
	db = a.store.Data()
	channel := "direct"
	resName := ""
	if resellerCopy != nil {
		channel = "reseller"
		resName = resellerCopy.Name
		// N°19 — en dépôt-vente, la prise de stock est gratuite : la
		// créance naîtra à la remise (Mode Vente), pas à la génération.
		if resellerCopy.PaymentMode != "deposit" {
			if res := findResellerScoped(db, resellerCopy.ID, acc); res != nil {
				res.Credit -= cost
			}
			db.Transactions = append([]model.Transaction{{
				ID: model.NewID("tx-"), AccountID: acc, Type: "sale", ResellerID: resellerCopy.ID, ResellerName: resName,
				Amount: cost, Note: fmt.Sprintf("Achat de %d vouchers (%s)", req.Count, profile.Name),
				At: model.NowISO(),
			}}, db.Transactions...)
		}
	}
	// N°19 — pas de Sale à la génération en dépôt-vente (reconnaissance à
	// l'encaissement au versement) : dashboard/rapports/compta ne
	// double-comptent pas.
	if resellerCopy == nil || resellerCopy.PaymentMode != "deposit" {
		db.Sales = append(db.Sales, model.Sale{
			ID: model.NewID("sale-"), AccountID: acc, Amount: cost, ProfileName: profile.Name, Count: req.Count,
			Channel: channel, ResellerName: resName,
			RouterID: routerCopy.ID, RouterName: routerCopy.Name, BatchID: batchID,
			At: model.NowISO(), Cost: cost, SellingTotal: sellingTotal,
		})
	}
	batchResellerID := ""
	if resellerCopy != nil {
		batchResellerID = resellerCopy.ID
	}
	db.Batches = append([]model.Batch{{
		ID: batchID, AccountID: acc, ProfileID: profile.ID, ProfileName: profile.Name,
		RouterID: routerCopy.ID, RouterName: routerCopy.Name,
		Count: req.Count, UnitPrice: profile.Price, TotalCost: cost, DataQuotaMb: int64(quotaMb), TimeLimitMin: timeLimitMin,
		Channel: channel, ResellerID: batchResellerID, ResellerName: resName,
		CreatedAt: model.NowISO(),
	}}, db.Batches...)
	msg := fmt.Sprintf("Génération de %d vouchers (%s)", req.Count, profile.Name)
	if resName != "" {
		msg += " pour " + resName
	}
	a.logActivityBy(r, db, acc, "voucher", msg)
	a.store.Save()
	a.store.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"batchId":   batchID,
		"vouchers":  vouchers,
		"totalCost": cost,
	})
}

func (a *API) handleVouchersBatchDelete(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	batchID := r.PathValue("batchId")
	a.store.Lock()
	db := a.store.Data()
	targets := []model.HotspotUser{}
	for _, u := range db.HotspotUsers {
		if u.AccountID == acc && u.Kind == "voucher" && u.BatchID == batchID {
			targets = append(targets, u)
		}
	}
	routerCopies := map[string]model.Router{}
	for _, u := range targets {
		if rr := findRouterScoped(db, u.RouterID, acc); rr != nil {
			if _, ok := routerCopies[rr.ID]; !ok {
				routerCopies[rr.ID] = *rr
			}
		}
	}
	a.store.Unlock()
	if len(targets) == 0 {
		writeErr(w, http.StatusNotFound, "Lot introuvable")
		return
	}
	// N°23 (W1) — un lot contenant des tickets revendeur ne se supprime pas
	// en silence : ce stock appartient au revendeur (créance à la remise
	// client). Compte seul dans le message — jamais les codes réels (le
	// message remonte dans les toasts console, masquage N°22 maintenu).
	// La voie propre : reprise (POST /api/vouchers/reprise) ou retour de
	// stock initié par le revendeur (Mode Vente), puis suppression.
	allocated := 0
	for _, u := range targets {
		if u.ResellerID != "" {
			allocated++
		}
	}
	if allocated > 0 {
		writeErr(w, http.StatusConflict,
			fmt.Sprintf("Suppression impossible : %d ticket(s) de ce lot sont attribués à un revendeur — effectuez une reprise (ou faites-les retourner par le revendeur) avant de supprimer.", allocated))
		return
	}

	deleted := 0
	// Routeurs agent : une commande user_remove par routeur
	for rid, rc := range routerCopies {
		if rc.Mode != "agent" {
			continue
		}
		names := []string{}
		for _, u := range targets {
			if u.RouterID == rid {
				names = append(names, agent.SanitizeName(u.Username))
			}
		}
		if len(names) == 0 {
			continue
		}
		a.store.Lock()
		queueCommandLocked(a.store.Data(), rc.AccountID, rid, model.CmdUserRemove, map[string]any{"names": names})
		a.store.Save()
		a.store.Unlock()
	}
	// Routeurs non-agent : gateway directe
	for _, rc := range routerCopies {
		if rc.Mode == "agent" {
			continue
		}
		gw := a.gatewayFor(rc)
		for _, u := range targets {
			if u.RouterID != rc.ID {
				continue
			}
			if err := gw.RemoveUser(u.ID); err != nil {
				writeErr(w, http.StatusBadRequest, "Suppression impossible : "+err.Error())
				return
			}
			deleted++
		}
	}
	// vouchers orphelins (routeur supprimé) : suppression directe
	a.store.Lock()
	db = a.store.Data()
	remaining := db.HotspotUsers[:0]
	for _, u := range db.HotspotUsers {
		if u.AccountID == acc && u.Kind == "voucher" && u.BatchID == batchID {
			deleted++
			continue
		}
		remaining = append(remaining, u)
	}
	db.HotspotUsers = remaining
	// retire aussi l'enregistrement du lot (traçabilité) — du compte seul
	batches := db.Batches[:0]
	for _, b := range db.Batches {
		if b.AccountID == acc && b.ID == batchID {
			continue
		}
		batches = append(batches, b)
	}
	db.Batches = batches
	a.logActivityBy(r, db, acc, "voucher", fmt.Sprintf("Lot %s supprimé (%d vouchers)", batchID, deleted))
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "deleted": deleted})
}

// ---------------------------------------------------------------------------
// Sessions
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Lots — liste enrichie + export CSV (refonte « fiche de vie »)
// ---------------------------------------------------------------------------

// holding — détenteur LIVE d'une part du stock vendable d'un lot (N°18) :
// ResellerID vide = stock direct du gérant. Tri affichage : direct d'abord,
// puis quantité décroissante.
type holding struct {
	ResellerID string `json:"resellerId"`
	Name       string `json:"name"`
	Count      int    `json:"count"`
	Value      int    `json:"value"`
}

// batchRow — un lot enrichi des stats live de ses vouchers + statut de
// « fiche de vie » (dérivé) + possession du stock vendable.
type batchRow struct {
	model.Batch
	Remaining int `json:"remaining"`
	Active    int `json:"active"`
	Used      int `json:"used"`
	Expired   int `json:"expired"`
	Disabled  int `json:"disabled"`
	// Refonte onglet Lots — cycle de vie dérivé : stock | consumed | expired | purged.
	Status string `json:"status"`
	// N°18 — possession live du stock vendable (lot immuable).
	Transferable      int `json:"transferable"`
	TransferableValue int `json:"transferableValue"`
	Expiring7d        int `json:"expiring7d"`
	// v2 « tour de contrôle » — intelligence métier (additif) :
	// Sold7d = sorties de stock sur 7 j glissants (vente SoldAt ou consommation
	// UsedAt, jamais les deux pour un même ticket) — la vélocité du lot.
	Sold7d int `json:"sold7d"`
	// Dernier mouvement de sortie + jours de dormance (lot VIVANT uniquement :
	// base = dernière sortie, sinon création du lot si rien n'est jamais sorti).
	LastEgressAt string `json:"lastEgressAt,omitempty"`
	DormantDays  int    `json:"dormantDays"`
	// Valeur faciale du stock vendable (Σ prix public) et marge en attente
	// (face − gros) : ce que le stock vivant RAPPORTEERA à l'écoulement.
	StockFace     int       `json:"stockFace"`
	MarginPending int       `json:"marginPending"`
	Holdings      []holding `json:"holdings,omitempty"`
}

// batchSummary — bande KPI de l'onglet Lots : totaux sur l'ensemble FILTRÉ
// (avant pagination) — le gérant filtre par détenteur et lit « son » stock.
type batchSummary struct {
	Batches      int `json:"batches"`
	StockTickets int `json:"stockTickets"`
	Transferable int `json:"transferable"`
	StockValue   int `json:"stockValue"`
	Expiring7d   int `json:"expiring7d"`
	// v2 — pipeline métier (mêmes règles que batchRow) :
	ResellerStock      int `json:"resellerStock"`      // vendables détenus par des revendeurs
	ResellerStockValue int `json:"resellerStockValue"` // valeur gros de ce stock
	Sold7d             int `json:"sold7d"`             // sorties de stock 7 j (lots filtrés)
	StockFace          int `json:"stockFace"`          // valeur faciale du vendable (Σ prix public)
	MarginPending      int `json:"marginPending"`      // marge en attente cumulée
}

// batchFilter — filtres communs à la liste et à l'export CSV.
// status : cycle de vie (stock|consumed|expired|purged) · holder : détenteur
// LIVE du stock vendable (direct|resellerId) · channel : provenance (direct|reseller).
type batchFilter struct {
	search   string
	routerID string
	status   string
	holder   string
	channel  string
}

func batchFilterFrom(r *http.Request) batchFilter {
	q := r.URL.Query()
	return batchFilter{
		search:   strings.ToLower(strings.TrimSpace(q.Get("search"))),
		routerID: strings.TrimSpace(q.Get("routerId")),
		status:   strings.TrimSpace(q.Get("status")),
		holder:   strings.TrimSpace(q.Get("holder")),
		channel:  strings.TrimSpace(q.Get("channel")),
	}
}

// batchLifecycle — statut de « fiche de vie » d'un lot, dérivé des stats live :
// stock (du stock consommable reste) → consumed (épuisé : utilisé/désactivé)
// → expired (jamais utilisé, validité envolée) → purged (plus rien en base).
func batchLifecycle(active, used, expired, disabled int) string {
	switch {
	case active > 0:
		return "stock"
	case used > 0 || disabled > 0:
		return "consumed"
	case expired > 0:
		return "expired"
	default:
		return "purged"
	}
}

// batchHeldBy — le détenteur vit dans les holdings (possession LIVE du stock
// vendable) : « direct » = stock du gérant, « resellers » = n'importe quel
// revendeur (v2 pipeline), sinon l'ID précis du revendeur. Un lot sans stock
// vendable (tout vendu, purgé) n'appartient à personne.
func batchHeldBy(hs []holding, holder string) bool {
	for _, h := range hs {
		if holder == "direct" {
			if h.ResellerID == "" {
				return true
			}
			continue
		}
		if holder == "resellers" {
			if h.ResellerID != "" {
				return true
			}
			continue
		}
		if h.ResellerID == holder {
			return true
		}
	}
	return false
}

// computeBatches — agrégation partagée de GET /api/vouchers/batches (liste)
// et /api/vouchers/batches/export (CSV) : stats live des vouchers, holdings
// N°18, statut de vie, filtres (recherche, site, canal, statut, détenteur —
// « resellers » v2), intelligence métier v2 (sorties 7 j, dormance, marge),
// tri du plus récent au plus ancien. Le contrat de la liste reste intact :
// les nouveaux champs (status, summary) et filtres sont ADDITIFS.
func (a *API) computeBatches(acc string, f batchFilter) ([]batchRow, batchSummary) {
	now := time.Now().UTC()
	a.store.Lock()
	db := a.store.Data()
	a.enforceExpired(db)              // P0 (audit Mikhmon) : expiration appliquée au passage
	online := onlineSessions(db, now) // sessions live (routeurs vus < 3 min)

	// Statuts live des vouchers, agrégés par lot.
	type liveStats struct {
		Remaining, Active, Used, Expired, Disabled int
	}
	stats := map[string]*liveStats{}

	// N°18 — possession LIVE du stock vendable, recalculée depuis les vouchers :
	// le lot (Batch) reste IMMUABLE (Channel/ResellerID = génération/provenance),
	// la redistribution se lit dans les vouchers. « Transférable » = même
	// définition que l'endpoint de transfert (actif, jamais remis).
	resNames := make(map[string]string, len(db.Resellers))
	for i := range db.Resellers {
		resNames[db.Resellers[i].ID] = db.Resellers[i].Name
	}
	holdings := map[string]map[string]*holding{}
	transferable := map[string]int{}
	transferableValue := map[string]int{}
	expiring7d := map[string]int{}
	// v2 — intelligence métier : sorties de stock 7 j (vélocité), dernier
	// mouvement de sortie (dormance), valeur faciale du vendable (marge).
	sold7d := map[string]int{}
	lastEgress := map[string]time.Time{}
	stockFace := map[string]int{}
	window7d := now.Add(-7 * 24 * time.Hour)
	for i := range db.HotspotUsers {
		u := &db.HotspotUsers[i]
		if u.AccountID != acc || u.Kind != "voucher" || u.BatchID == "" {
			continue
		}
		st, ok := stats[u.BatchID]
		if !ok {
			st = &liveStats{}
			stats[u.BatchID] = st
		}
		resolved := model.ResolvedStatus(u, online[onlineKey(u)], now)
		if resolved == "online" {
			resolved = "used" // agrégat : en ligne = consommé (session en cours)
		}
		switch resolved {
		case "active":
			st.Active++
			st.Remaining++
		case "used":
			st.Used++
		case "expired":
			st.Expired++
		case "disabled":
			st.Disabled++
		}
		// v2 — sortie de stock : vente enregistrée (SoldAt) ou consommation
		// (UsedAt). Un ticket ne sort qu'une fois : booléen par ticket, jamais
		// de double comptage. Dernier mouvement = max des deux horodatages.
		egress7d := false
		if ua, err := time.Parse(time.RFC3339, u.UsedAt); err == nil && !ua.IsZero() {
			if ua.After(window7d) {
				egress7d = true
			}
			if ua.After(lastEgress[u.BatchID]) {
				lastEgress[u.BatchID] = ua
			}
		}
		if sa, err := time.Parse(time.RFC3339, u.SoldAt); err == nil && !sa.IsZero() {
			if sa.After(window7d) {
				egress7d = true
			}
			if sa.After(lastEgress[u.BatchID]) {
				lastEgress[u.BatchID] = sa
			}
		}
		if egress7d {
			sold7d[u.BatchID]++
		}
		if u.SoldAt == "" && model.EffectiveStatus(u, now) == "active" {
			transferable[u.BatchID]++
			transferableValue[u.BatchID] += u.Price
			face := u.SellingPrice
			if face <= 0 {
				face = u.Price // générations anciennes sans prix public copié
			}
			stockFace[u.BatchID] += face
			if exp, err := time.Parse(time.RFC3339, u.ExpiresAt); err == nil && exp.Before(now.AddDate(0, 0, 7)) {
				expiring7d[u.BatchID]++ // garde-fou dialog : stock mort imminent
			}
			hm, ok := holdings[u.BatchID]
			if !ok {
				hm = map[string]*holding{}
				holdings[u.BatchID] = hm
			}
			h, ok := hm[u.ResellerID]
			if !ok {
				h = &holding{ResellerID: u.ResellerID, Name: resNames[u.ResellerID]}
				hm[u.ResellerID] = h
			}
			h.Count++
			h.Value += u.Price
		}
	}

	filtered := []batchRow{}
	for _, b := range db.Batches {
		if b.AccountID != acc {
			continue
		}
		if f.routerID != "" && f.routerID != "all" && b.RouterID != f.routerID {
			continue
		}
		if f.channel != "" && f.channel != "all" && b.Channel != f.channel {
			continue
		}
		if f.search != "" {
			hay := strings.ToLower(b.ID + " " + b.ProfileName + " " + b.RouterName + " " + b.ResellerName + " " + b.Channel)
			if !strings.Contains(hay, f.search) {
				continue
			}
		}
		row := batchRow{Batch: b}
		if st, ok := stats[b.ID]; ok {
			row.Remaining = st.Remaining
			row.Active = st.Active
			row.Used = st.Used
			row.Expired = st.Expired
			row.Disabled = st.Disabled
		}
		// Refonte — statut de « fiche de vie » dérivé des stats live.
		row.Status = batchLifecycle(row.Active, row.Used, row.Expired, row.Disabled)
		row.Transferable = transferable[b.ID]
		row.TransferableValue = transferableValue[b.ID]
		row.Expiring7d = expiring7d[b.ID]
		// v2 — intelligence métier du lot (dormance : lot vivant uniquement,
		// base = dernière sortie sinon création ; jamais de jour négatif).
		row.Sold7d = sold7d[b.ID]
		row.StockFace = stockFace[b.ID]
		row.MarginPending = row.StockFace - row.TransferableValue
		if row.MarginPending < 0 {
			row.MarginPending = 0 // config dégénérée (public < gros) : pas de marge négative affichée
		}
		if row.Transferable > 0 {
			base := lastEgress[b.ID]
			if base.IsZero() {
				if ct, err := time.Parse(time.RFC3339, b.CreatedAt); err == nil {
					base = ct
				}
			}
			if !base.IsZero() {
				d := int(now.Sub(base).Hours() / 24)
				if d < 0 {
					d = 0
				}
				row.DormantDays = d
				row.LastEgressAt = base.UTC().Format(time.RFC3339)
			}
		}
		if hm, ok := holdings[b.ID]; ok && len(hm) > 0 {
			hs := make([]holding, 0, len(hm))
			for _, h := range hm {
				hs = append(hs, *h)
			}
			// Affichage stable : stock direct d'abord, puis par quantité décroissante.
			sort.Slice(hs, func(i, j int) bool {
				di, dj := hs[i].ResellerID == "", hs[j].ResellerID == ""
				if di != dj {
					return di
				}
				if hs[i].Count != hs[j].Count {
					return hs[i].Count > hs[j].Count
				}
				return hs[i].ResellerID < hs[j].ResellerID
			})
			row.Holdings = hs
		}
		// Filtres « fiche de vie » : cycle de vie + détenteur du stock vendable.
		if f.status != "" && f.status != "all" && row.Status != f.status {
			continue
		}
		if f.holder != "" && f.holder != "all" && !batchHeldBy(row.Holdings, f.holder) {
			continue
		}
		filtered = append(filtered, row)
	}
	a.store.Save() // P0 : persiste les flags Enforced déposés par enforceExpired
	a.store.Unlock()

	sort.Slice(filtered, func(i, j int) bool { return filtered[i].CreatedAt > filtered[j].CreatedAt })
	summary := batchSummary{Batches: len(filtered)}
	for _, row := range filtered {
		summary.StockTickets += row.Active
		summary.Transferable += row.Transferable
		summary.StockValue += row.TransferableValue
		summary.Expiring7d += row.Expiring7d
		// v2 — pipeline métier (toute valeur lue sur l'ensemble FILTRÉ).
		summary.Sold7d += row.Sold7d
		summary.StockFace += row.StockFace
		summary.MarginPending += row.MarginPending
		for _, h := range row.Holdings {
			if h.ResellerID != "" {
				summary.ResellerStock += h.Count
				summary.ResellerStockValue += h.Value
			}
		}
	}
	return filtered, summary
}

func (a *API) handleBatchesList(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	page := queryInt(r, "page", 1, 1, 1_000_000)
	pageSize := queryInt(r, "pageSize", 15, 1, 100)
	rows, summary := a.computeBatches(acc, batchFilterFrom(r))
	total := len(rows)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data":     rows[start:end],
		"total":    total,
		"page":     page,
		"pageSize": pageSize,
		"summary":  summary,
	})
}

// handleBatchesExport — export CSV des lots (mêmes filtres que la liste, sans
// pagination) : une ligne par lot — cycle de vie, stock vendable, possession.
// Séparateur « ; » + BOM UTF-8 (convention mikcloud-comptabilite).
func (a *API) handleBatchesExport(w http.ResponseWriter, r *http.Request) {
	rows, _ := a.computeBatches(accountScope(r), batchFilterFrom(r))
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\"mikcloud-lots.csv\"")
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})
	_, _ = w.Write([]byte("Lot ;Date ;Statut ;Profil ;Quota Mo ;Site ;Canal ;Revendeur ;Tickets ;En stock ;Vendables ;Utilises ;Expires ;Desactives ;Expirent < 7j ;Valeur stock ;Cout total ;Ecoules 7j ;Dormance j ;Marge en attente ;Possession\r\n"))
	statusLabels := map[string]string{"stock": "En stock", "consumed": "Epuise", "expired": "Expire", "purged": "Purge"}
	for _, row := range rows {
		parts := make([]string, 0, len(row.Holdings))
		for _, h := range row.Holdings {
			name := h.Name
			if h.ResellerID == "" {
				name = "Stock direct"
			}
			parts = append(parts, fmt.Sprintf("%s (%d)", name, h.Count))
		}
		_, _ = w.Write([]byte(fmt.Sprintf(
			"%s ;%s ;%s ;%s ;%d ;%s ;%s ;%s ;%d ;%d ;%d ;%d ;%d ;%d ;%d ;%d ;%d ;%d ;%d ;%d ;%s\r\n",
			csvField(row.ID), csvField(row.CreatedAt), csvField(statusLabels[row.Status]),
			csvField(row.ProfileName), row.DataQuotaMb, csvField(row.RouterName),
			row.Channel, csvField(row.ResellerName), row.Count, row.Active,
			row.Transferable, row.Used, row.Expired, row.Disabled, row.Expiring7d,
			row.TransferableValue, row.TotalCost, row.Sold7d, row.DormantDays,
			row.MarginPending, csvField(strings.Join(parts, " · ")))))
	}
}
