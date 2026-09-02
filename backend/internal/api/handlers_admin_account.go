// Package api — fiche client, gestion d'abonnement et suppression de compte
// (console plateforme — super-admin MikCloud).
//
// P2 : attribuer / renouveler un plan, marquer payé, consulter la fiche
// détaillée d'un compte client (usage, routeurs, équipe, journal récent).
// P3 : enforcement serveur — un compte dont l'abonnement est expiré repasse
// en lecture seule (402 sur les écritures métier) et le plan Essentiel est
// plafonné aux routeurs couverts par la période payée (routerSlots).
package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"mikcloud/hotspot-api/internal/auth"
	"mikcloud/hotspot-api/internal/model"
)

// ---------------------------------------------------------------------------
// P3 — enforcement serveur des abonnements (guard d'écriture)
// ---------------------------------------------------------------------------

// writeErrCode — erreur JSON enrichie d'un code machine (le front peut
// adapter sa réaction ; le message reste humain et francophone).
func writeErrCode(w http.ResponseWriter, status int, code, msg string, extra map[string]any) {
	body := map[string]any{"error": msg, "code": code}
	for k, v := range extra {
		body[k] = v
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// subscriptionGuardView — état d'abonnement vu par le guard (hors verrou).
type subscriptionGuardView struct {
	Status      string // none | active | expired (effectif)
	PlanID      string
	RouterSlots int
	PeriodEnd   string
}

// subscriptionGuardStateLocked — lit l'état d'abonnement du compte, à appeler
// LE VERROU PRIS (compteurs exacts pour le plafond de routeurs).
func (a *API) subscriptionGuardStateLocked(acc string) subscriptionGuardView {
	settings := ensureSettings(a.store.Data(), acc)
	return subscriptionGuardView{
		Status:      subscriptionStatus(settings.Subscription, time.Now().UTC()),
		PlanID:      settings.Subscription.PlanID,
		RouterSlots: settings.Subscription.RouterSlots,
		PeriodEnd:   settings.Subscription.PeriodEnd,
	}
}

// subscriptionGuardState — lit l'état d'abonnement du compte (sous verrou).
func (a *API) subscriptionGuardState(acc string) subscriptionGuardView {
	a.store.Lock()
	defer a.store.Unlock()
	return a.subscriptionGuardStateLocked(acc)
}

// guardAccountWrite — bloque les écritures métier d'un compte expiré (P3).
// Exemption : les administrateurs plateforme (session support ou console) —
// le guard protège le modèle économique contre les CLIENTS, pas contre
// l'opérateur du SaaS. Renvoie true si l'écriture est autorisée, sinon
// répond 402 et renvoie false.
//
// En pratique : un compte expiré repasse en LECTURE SEULE — consultations,
// exports, suppressions et actions de maintenance (activer/désactiver un
// utilisateur, déconnecter une session, réglages) restent possibles ; la
// création et la modification de ressources métier (routeurs, utilisateurs
// hotspot, vouchers, profils, revendeurs, équipe, import agent) est refusée
// avec le code machine « subscription_expired ».
func (a *API) guardAccountWrite(w http.ResponseWriter, r *http.Request) bool {
	if isPlatformAdmin(r) {
		return true
	}
	view := a.subscriptionGuardState(accountScope(r))
	if view.Status != "expired" {
		return true
	}
	writeErrCode(w, http.StatusPaymentRequired, "subscription_expired",
		"Abonnement expiré — renouvelez auprès de MikCloud pour reprendre les modifications",
		map[string]any{"periodEnd": view.PeriodEnd})
	return false
}

// guardAccountRouterLimit — plafond de routeurs du plan Essentiel (P3) :
// la période couvre RouterSlots routeurs ; au-delà, la création est refusée
// (402, code « plan_router_limit »). À appeler APRÈS guardAccountWrite,
// sous verrou (routerCount = routeurs déjà enregistrés du compte).
func guardAccountRouterLimit(w http.ResponseWriter, view subscriptionGuardView, routerCount int) bool {
	// Essai : 1 routeur max. Essentiel : quota de routeurs couverts par la période.
	if (view.PlanID != "essentiel" && view.PlanID != "essai") || view.RouterSlots <= 0 || routerCount < view.RouterSlots {
		return true
	}
	writeErrCode(w, http.StatusPaymentRequired, "plan_router_limit",
		fmt.Sprintf("Votre formule couvre %d routeur(s) — passez au plan Essentiel (plus de routeurs) ou Illimité pour en ajouter", view.RouterSlots),
		map[string]any{"limit": view.RouterSlots, "current": routerCount, "plan": view.PlanID})
	return false
}

// ---------------------------------------------------------------------------
// P2 — fiche détaillée d'un compte client : GET /api/admin/accounts/{id}
// ---------------------------------------------------------------------------

// handleAdminAccountDetail — tout ce que la plateforme sait sur un compte :
// informations, propriétaire + équipe, abonnement (état réel), usage 30 j,
// routeurs, journal d'activité récent. Réservé aux super-admins plateforme.
func (a *API) handleAdminAccountDetail(w http.ResponseWriter, r *http.Request) {
	if !isPlatformAdmin(r) {
		writeErrCode(w, http.StatusForbidden, "forbidden", "Réservé aux administrateurs de la plateforme", nil)
		return
	}
	id := r.PathValue("id")
	now := time.Now().UTC()
	cutoff30 := now.AddDate(0, 0, -30)

	a.store.Lock()
	db := a.store.Data()

	var acc *model.Account
	for i := range db.Accounts {
		if db.Accounts[i].ID == id {
			acc = &db.Accounts[i]
			break
		}
	}
	if acc == nil {
		a.store.Unlock()
		writeErrCode(w, http.StatusNotFound, "not_found", "Compte introuvable", nil)
		return
	}

	// Équipe du compte (owner en premier, puis les autres).
	type memberRow struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		Username  string `json:"username"`
		Role      string `json:"role"`
		CreatedAt string `json:"createdAt"`
		Owner     bool   `json:"owner"`
	}
	team := make([]memberRow, 0, 4)
	for i := range db.Users {
		u := &db.Users[i]
		if u.AccountID != id {
			continue
		}
		team = append(team, memberRow{
			ID: u.ID, Name: u.Name, Username: u.Username, Role: u.Role,
			CreatedAt: u.CreatedAt, Owner: u.Role == model.RoleOwner || u.Role == "admin",
		})
	}
	// owner = premier owner du compte (sinon premier membre).
	var owner *memberRow
	for i := range team {
		if team[i].Owner {
			owner = &team[i]
			break
		}
	}
	if owner == nil && len(team) > 0 {
		owner = &team[0]
	}

	// Abonnement (état réel + libellé).
	settings, hasSettings := db.SettingsByAccount[id]
	sub := settings.Subscription
	status := "none"
	if hasSettings {
		status = subscriptionStatus(sub, now)
	}
	planName := "Essai"
	switch sub.PlanID {
	case "":
		planName = "Essai"
	default:
		if p, ok := model.PlanByID(sub.PlanID); ok {
			planName = p.Name
		} else {
			planName = sub.PlanID
		}
	}
	routerCount := accountRouterCount(db, id)
	amount := 0
	if p, ok := model.PlanByID(sub.PlanID); ok {
		amount = planAmount(p, routerCount)
	}

	// Usage.
	var routersOnline, users, sessions, sales30d, revenue30d, vouchersAvailable int
	type routerRow struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Mode     string `json:"mode"`
		Status   string `json:"status"`
		Users    int    `json:"users"`
		LastSeen string `json:"lastSeen"`
	}
	routers := make([]routerRow, 0, 4)
	routerIDs := map[string]int{}
	for i := range db.Routers {
		rt := &db.Routers[i]
		if rt.AccountID != id {
			continue
		}
		routerIDs[rt.ID] = len(routers)
		if rt.Status == "online" {
			routersOnline++
		}
		routers = append(routers, routerRow{
			ID: rt.ID, Name: rt.Name, Mode: rt.Mode, Status: rt.Status, LastSeen: rt.LastSeen,
		})
	}
	for i := range db.HotspotUsers {
		u := &db.HotspotUsers[i]
		if u.AccountID != id {
			continue
		}
		users++
		if idx, ok := routerIDs[u.RouterID]; ok {
			routers[idx].Users++
		}
		if u.Kind == "voucher" && u.Status == "active" {
			vouchersAvailable++
		}
	}
	for i := range db.Sessions {
		if db.Sessions[i].AccountID == id {
			sessions++
		}
	}
	for i := range db.Sales {
		s := &db.Sales[i]
		if s.AccountID != id {
			continue
		}
		if at, err := time.Parse(time.RFC3339, s.At); err == nil && at.After(cutoff30) {
			sales30d++
			revenue30d += s.Amount
		}
	}

	// Journal récent du compte (15 dernières entrées).
	type activityRow struct {
		ID        string `json:"id"`
		Type      string `json:"type"`
		Message   string `json:"message"`
		At        string `json:"at"`
		ActorName string `json:"actorName,omitempty"`
	}
	activity := make([]activityRow, 0, 15)
	for i := range db.Activity {
		act := &db.Activity[i]
		if act.AccountID != id {
			continue
		}
		activity = append(activity, activityRow{
			ID: act.ID, Type: act.Type, Message: act.Message, At: act.At, ActorName: act.ActorName,
		})
		if len(activity) >= 15 {
			break
		}
	}
	a.store.Unlock()

	type subscriptionOut struct {
		PlanID         string `json:"planId"`
		PlanName       string `json:"planName"`
		Status         string `json:"status"` // none | active | expired (effectif)
		PeriodStart    string `json:"periodStart"`
		PeriodEnd      string `json:"periodEnd"`
		LastAmountFcfa int    `json:"lastAmountFcfa"`
		RouterSlots    int    `json:"routerSlots"`
		LastPaidAt     string `json:"lastPaidAt"`
		RouterCount    int    `json:"routerCount"`
		AmountFcfa     int    `json:"amountFcfa"`
	}
	var ownerOut any
	if owner != nil {
		ownerOut = *owner
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":        acc.ID,
		"name":      acc.Name,
		"status":    acc.Status,
		"createdAt": acc.CreatedAt,
		"owner":     ownerOut,
		"team":      team,
		"subscription": subscriptionOut{
			PlanID: sub.PlanID, PlanName: planName, Status: status,
			PeriodStart: sub.PeriodStart, PeriodEnd: sub.PeriodEnd,
			LastAmountFcfa: sub.LastAmountFcfa, RouterSlots: sub.RouterSlots,
			LastPaidAt: sub.LastPaidAt, RouterCount: routerCount, AmountFcfa: amount,
		},
		"stats": map[string]int{
			"users": users, "routers": len(routers), "routersOnline": routersOnline,
			"sessions": sessions, "sales30d": sales30d, "revenue30d": revenue30d,
			"vouchersAvailable": vouchersAvailable,
		},
		"routers":  routers,
		"activity": activity,
	})
}

// ---------------------------------------------------------------------------
// P2 — attribuer / renouveler un abonnement : PUT /api/admin/accounts/{id}/subscription
// ---------------------------------------------------------------------------

// handleAdminAccountSubscription — la plateforme attribue, renouvelle ou
// retire un plan pour le compte d'un client :
//   - planId : « essentiel » | « illimite » | « beta » (gratuit non expirant) ;
//   - months : durée en mois (défaut 1 pour Essentiel, 12 pour Illimité ;
//     1..36) — le renouvellement du MÊME plan actif s'empile après la
//     période en cours, un changement de plan démarre immédiatement ;
//   - routerSlots (Essentiel) : nombre de routeurs couverts — défaut = le
//     quota actuel, sinon le nombre de routeurs enregistrés (min. 1) ;
//   - markPaid : date le paiement (lastPaidAt = maintenant, sinon effacé) ;
//   - note : commentaire libre tracé dans le journal.
func (a *API) handleAdminAccountSubscription(w http.ResponseWriter, r *http.Request) {
	if !isPlatformAdmin(r) {
		writeErrCode(w, http.StatusForbidden, "forbidden", "Réservé aux administrateurs de la plateforme", nil)
		return
	}
	id := r.PathValue("id")
	var req struct {
		PlanID      string `json:"planId"`
		Months      int    `json:"months"`
		RouterSlots int    `json:"routerSlots"`
		MarkPaid    bool   `json:"markPaid"`
		Note        string `json:"note"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErrCode(w, http.StatusBadRequest, "bad_request", "Corps de requête invalide", nil)
		return
	}
	planID := strings.ToLower(strings.TrimSpace(req.PlanID))
	if planID != "essentiel" && planID != "illimite" && planID != "essai" {
		writeErrCode(w, http.StatusBadRequest, "bad_plan", "Formule inconnue (essentiel | illimite | essai)", nil)
		return
	}
	// La formule et le montant sont calculés par applySubscriptionLocked.
	months := req.Months
	if planID == "illimite" && months <= 0 {
		months = 12
	}
	if planID == "essentiel" && months <= 0 {
		months = 1
	}
	if planID == "essai" && months <= 0 {
		months = 3 // essai par défaut : 3 mois (90 jours)
	}
	if planID == "essentiel" || planID == "illimite" || planID == "essai" {
		if months < 1 || months > 36 {
			writeErrCode(w, http.StatusBadRequest, "bad_months", "Durée invalide (1 à 36 mois)", nil)
			return
		}
	}

	a.store.Lock()
	db := a.store.Data()
	var acc *model.Account
	for i := range db.Accounts {
		if db.Accounts[i].ID == id {
			acc = &db.Accounts[i]
			break
		}
	}
	if acc == nil {
		a.store.Unlock()
		writeErrCode(w, http.StatusNotFound, "not_found", "Compte introuvable", nil)
		return
	}
	routerCount := accountRouterCount(db, id)
	sub, label, planName, amount := applySubscriptionLocked(db, id, planID, months, req.RouterSlots, req.MarkPaid)

	// Journal (transverse + visible du compte).
	msg := ""
	switch planID {
	case "essai":
		msg = fmt.Sprintf("Essai prolongé par la plateforme — %d mois, 1 routeur couvert", months)
	default:
		paid := ""
		if req.MarkPaid {
			paid = ", payé"
		}
		msg = fmt.Sprintf("Abonnement %s attribué par la plateforme — %d mois, %d routeur(s) couvert(s), %d FCFA%s",
			planName, months, sub.RouterSlots, amount, paid)
	}
	if note := strings.TrimSpace(req.Note); note != "" {
		msg += " — " + note
	}
	a.logActivityBy(r, db, id, "billing", msg)
	a.store.Save()
	a.store.Unlock()

	status := subscriptionStatus(sub, time.Now().UTC())
	writeJSON(w, http.StatusOK, map[string]any{
		"subscription": map[string]any{
			"planId": sub.PlanID, "planName": label, "status": status,
			"periodStart": sub.PeriodStart, "periodEnd": sub.PeriodEnd,
			"lastAmountFcfa": sub.LastAmountFcfa, "routerSlots": sub.RouterSlots,
			"lastPaidAt": sub.LastPaidAt, "routerCount": routerCount, "amountFcfa": amount,
		},
	})
}

// applySubscriptionLocked — active / renouvelle la période d'abonnement d'un
// compte client (verrou POSÉ par l'appelant). Règles partagées par la fiche
// client (PUT /api/admin/accounts/{id}/subscription), la file des demandes de
// renouvellement et le webhook Wave : source unique du calcul de période
// (empilement sur la période active), des quotas Essentiel et du montant.
// Retourne l'abonnement, le libellé d'affichage (compat contrat historique),
// le nom court de la formule et le montant appliqué.
func applySubscriptionLocked(db *model.DB, accID, planID string, months, reqSlots int, markPaid bool) (model.Subscription, string, string, int) {
	settings := ensureSettings(db, accID)
	sub := settings.Subscription
	now := time.Now().UTC()

	var plan model.SaasPlan
	var amount int
	var slots int
	switch planID {
	case "essai":
		// Essai : gratuit, 1 routeur, période limitée (prolongeable par la plateforme).
		slots = 1
	case "illimite":
		// 1 000 F/mois équivalent (12 000 F/an) × durée.
		slots = 0
		amount = 1000 * months
	case "essentiel":
		plan, _ = model.PlanByID(planID)
		slots = reqSlots
		if slots <= 0 {
			slots = sub.RouterSlots // renouvellement : le quota actuel est conservé
		}
		if slots <= 0 {
			slots = accountRouterCount(db, accID)
		}
		if slots < 1 {
			slots = 1
		}
		amount = plan.PriceFcfa * slots * months
	}

	if planID == "essentiel" || planID == "illimite" || planID == "essai" {
		// Renouvellement du même plan encore actif : la nouvelle période
		// s'empile à la fin de la période en cours. Sinon : immédiat.
		start := now
		if sub.PlanID == planID && subscriptionStatus(sub, now) == "active" && sub.PeriodEnd != "" {
			if end, err := time.Parse(time.RFC3339, sub.PeriodEnd); err == nil && now.Before(end) {
				start = end
			}
		}
		sub.PlanID = planID
		sub.Status = "active"
		sub.PeriodStart = start.Format(time.RFC3339)
		sub.PeriodEnd = start.AddDate(0, months, 0).Format(time.RFC3339)
		sub.RouterSlots = slots
		sub.LastAmountFcfa = amount
	} else {
		// Cas résiduel (jamais atteint avec la validation ci-dessus, mais
		// conservé pour la sécurité du type) : période immédiate non expirante.
		sub.PlanID = planID
		sub.Status = "active"
		sub.PeriodStart = now.Format(time.RFC3339)
		sub.PeriodEnd = "" // non expirant
		sub.RouterSlots = 0
		sub.LastAmountFcfa = 0
	}
	if markPaid {
		sub.LastPaidAt = now.Format(time.RFC3339)
	} else {
		sub.LastPaidAt = ""
	}
	settings.Subscription = sub

	// Compatibilité d'affichage avec l'ancien contrat (libellé Plan).
	label := "Essai"
	maxRouters := "1"
	switch planID {
	case "essentiel":
		label = "MikCloud Essentiel"
		maxRouters = "Par routeur"
	case "illimite":
		label = "MikCloud Illimité"
		maxRouters = "Illimité"
	}
	settings.Plan = model.Plan{Name: label, MaxRouters: maxRouters, MaxUsers: "Illimité"}
	db.SettingsByAccount[accID] = settings
	return sub, label, plan.Name, amount
}

// ---------------------------------------------------------------------------
// P2 — suppression d'un compte client : DELETE /api/admin/accounts/{id}
// ---------------------------------------------------------------------------

// handleAdminAccountDelete — supprime un compte client et TOUTES ses données
// (routeurs + cascade, utilisateurs, lots, ventes, revendeurs, transactions,
// sessions, trafic, commandes, gabarits, journaux, réglages, notifications).
// Garde-fou : un compte portant encore un administrateur plateforme n'est pas
// supprimable (les platform_admin n'ont normalement plus de compte —
// migration de détachement au démarrage).
func (a *API) handleAdminAccountDelete(w http.ResponseWriter, r *http.Request) {
	if !isPlatformAdmin(r) {
		writeErrCode(w, http.StatusForbidden, "forbidden", "Réservé aux administrateurs de la plateforme", nil)
		return
	}
	id := r.PathValue("id")
	a.store.Lock()
	db := a.store.Data()
	idx := -1
	for i := range db.Accounts {
		if db.Accounts[i].ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		a.store.Unlock()
		writeErrCode(w, http.StatusNotFound, "not_found", "Compte introuvable", nil)
		return
	}
	for i := range db.Users {
		if db.Users[i].AccountID == id && (db.Users[i].Role == model.RolePlatformAdmin || db.Users[i].Role == "admin") {
			a.store.Unlock()
			writeErrCode(w, http.StatusBadRequest, "platform_account",
				"Ce compte porte encore un administrateur plateforme — il ne peut pas être supprimé", nil)
			return
		}
	}
	name := db.Accounts[idx].Name

	// Routeurs du compte + cascade par routeur (users/sessions/trafic/
	// commandes/bindings/tâches planifiées), mêmes mécaniques que la purge.
	routerIDs := map[string]bool{}
	for i := range db.Routers {
		if db.Routers[i].AccountID == id {
			routerIDs[db.Routers[i].ID] = true
		}
	}
	dropRouter := func(rid string) bool { return routerIDs[rid] }
	db.HotspotUsers = dropByRouterID(db.HotspotUsers, dropRouter, func(u model.HotspotUser) string { return u.RouterID }, nil)
	db.Sessions = dropByRouterID(db.Sessions, dropRouter, func(s model.Session) string { return s.RouterID }, nil)
	db.Traffic = dropByRouterID(db.Traffic, dropRouter, func(t model.RouterTraffic) string { return t.RouterID }, nil)
	db.Commands = dropByRouterID(db.Commands, dropRouter, func(c model.Command) string { return c.RouterID }, nil)
	db.IPBindings = dropByRouterID(db.IPBindings, dropRouter, func(b model.IPBinding) string { return b.RouterID }, nil)
	db.SchedulerTasks = dropByRouterID(db.SchedulerTasks, dropRouter, func(t model.SchedulerTask) string { return t.RouterID }, nil)

	// Cascade par compte.
	db.Accounts = append(db.Accounts[:idx], db.Accounts[idx+1:]...)
	db.Users = dropByString(db.Users, id, func(u model.AdminUser) string { return u.AccountID })
	db.Routers = dropByString(db.Routers, id, func(rt model.Router) string { return rt.AccountID })
	db.Profiles = dropByString(db.Profiles, id, func(p model.Profile) string { return p.AccountID })
	db.HotspotUsers = dropByString(db.HotspotUsers, id, func(u model.HotspotUser) string { return u.AccountID })
	db.Batches = dropByString(db.Batches, id, func(b model.Batch) string { return b.AccountID })
	db.Resellers = dropByString(db.Resellers, id, func(rs model.Reseller) string { return rs.AccountID })
	db.Transactions = dropByString(db.Transactions, id, func(t model.Transaction) string { return t.AccountID })
	db.Sales = dropByString(db.Sales, id, func(s model.Sale) string { return s.AccountID })
	db.Activity = dropByString(db.Activity, id, func(ac model.Activity) string { return ac.AccountID })
	db.Templates = dropByString(db.Templates, id, func(t model.VoucherTemplate) string { return t.AccountID })
	db.UserLogs = dropByString(db.UserLogs, id, func(l model.UserLog) string { return l.AccountID })
	db.NotifLog = dropByString(db.NotifLog, id, func(l model.NotificationLog) string { return l.AccountID })
	db.BillingRequests = dropByString(db.BillingRequests, id, func(b model.BillingRequest) string { return b.AccountID })
	// Prélèvements automatiques carte (Stripe via GeniusPay) — les fiches
	// locales sont purgées ; les abonnements encore vivants chez GeniusPay
	// sont résiliés à distance après libération du verrou (best-effort).
	gpToCancel := []string{}
	for i := range db.GeniusPaySubs {
		g := db.GeniusPaySubs[i]
		if g.AccountID == id && stripeSubInProgress(g.Status) && !strings.EqualFold(g.Status, "cancelled") {
			gpToCancel = append(gpToCancel, g.UUID)
		}
	}
	db.GeniusPaySubs = dropByString(db.GeniusPaySubs, id, func(g model.GeniusPaySub) string { return g.AccountID })
	delete(db.SettingsByAccount, id)
	delete(db.NotifSettings, id)

	a.logActivityBy(r, db, "", "system", "Compte client supprimé par la plateforme : "+name+" (données effacées)")
	a.store.Save()
	a.store.Unlock()

	// Résiliation distante des prélèvements automatiques encore actifs
	// (hors verrou, best-effort — un échec est journalisé côté serveur).
	for _, gid := range gpToCancel {
		go func(gid string) {
			if _, err := geniusPaySubCancel(gid, true, "Compte MikCloud supprimé"); err != nil {
				log.Printf("[geniuspay] résiliation de l'abonnement %s après suppression de compte : %v", gid, err)
			}
		}(gid)
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ---------------------------------------------------------------------------
// Bascule support — POST /api/admin/accounts/{id}/impersonate
// ---------------------------------------------------------------------------

// handleAdminImpersonate — ouvre une SESSION SUPPORT dans la console du
// compte client demandé (assistance, configuration, diagnostic). Répond avec
// un token JWT dont le périmètre (claim « acc ») pointe sur le compte client :
// TOUTE l'isolation multi-tenant et les garde-fous serveur restent actifs,
// seul le périmètre de données change. L'action est tracée dans le journal
// du compte (« Session support ouverte »).
func (a *API) handleAdminImpersonate(w http.ResponseWriter, r *http.Request) {
	if !isPlatformAdmin(r) {
		writeErrCode(w, http.StatusForbidden, "forbidden", "Réservé aux administrateurs de la plateforme", nil)
		return
	}
	claims := claimsFrom(r)
	if claims == nil {
		writeErrCode(w, http.StatusUnauthorized, "unauthorized", "Token invalide ou expiré", nil)
		return
	}
	id := r.PathValue("id")
	a.store.Lock()
	db := a.store.Data()
	var acc *model.Account
	for i := range db.Accounts {
		if db.Accounts[i].ID == id {
			acc = &db.Accounts[i]
			break
		}
	}
	if acc == nil {
		a.store.Unlock()
		writeErrCode(w, http.StatusNotFound, "not_found", "Compte introuvable", nil)
		return
	}
	if acc.Status != "active" {
		a.store.Unlock()
		writeErrCode(w, http.StatusConflict, "account_disabled", "Compte désactivé — réactivez-le avant d'ouvrir sa console", nil)
		return
	}
	// Nom réel de l'admin (le claim ne porte que l'ID).
	adminName, adminUsername := claims.Name, ""
	for i := range db.Users {
		if db.Users[i].ID == claims.Sub {
			adminName, adminUsername = db.Users[i].Name, db.Users[i].Username
			break
		}
	}
	a.logActivityBy(r, db, id, "system", "Session support ouverte — la plateforme consulte la console de ce compte")
	a.store.Save()
	a.store.Unlock()

	token := auth.Sign(a.secret, auth.NewClaims(claims.Sub, adminName, model.RolePlatformAdmin, id))
	writeJSON(w, http.StatusOK, map[string]any{
		"token": token,
		"user": map[string]any{
			"id": claims.Sub, "name": adminName, "username": adminUsername,
			"role": model.RolePlatformAdmin, "accountId": id, "accountName": acc.Name,
		},
	})
}

// dropByString — retirer d'une slice toutes les entrées dont le champ vaut v
// (générique, même mécanique que dropByRouterID mais sur n'importe quel champ).
func dropByString[T any](items []T, v string, field func(T) string) []T {
	out := items[:0]
	dropped := false
	for i := range items {
		if field(items[i]) == v {
			dropped = true
			continue
		}
		out = append(out, items[i])
	}
	if !dropped {
		return items
	}
	return out
}

// handleAdminAccounts — liste tous les comptes SaaS (triés par création
// décroissante) avec le login de leur propriétaire et des statistiques d'usage.
// Réservé au rôle « admin » (super-administrateur MikCloud).
func (a *API) handleAdminAccounts(w http.ResponseWriter, r *http.Request) {
	if !isPlatformAdmin(r) {
		writeErr(w, http.StatusForbidden, "Réservé aux administrateurs de la plateforme")
		return
	}
	now := time.Now().UTC()
	cutoff30 := now.AddDate(0, 0, -30)
	a.store.Lock()
	db := a.store.Data()

	type accountStats struct {
		Users      int `json:"users"`
		Routers    int `json:"routers"`
		Sessions   int `json:"sessions"`
		Sales30d   int `json:"sales30d"`
		Revenue30d int `json:"revenue30d"`
	}
	type accountRow struct {
		ID           string       `json:"id"`
		Name         string       `json:"name"`
		Status       string       `json:"status"`
		CreatedAt    string       `json:"createdAt"`
		Owner        string       `json:"owner"`
		Subscription string       `json:"subscription"` // active | expired | beta
		Stats        accountStats `json:"stats"`
	}

	stats := map[string]*accountStats{}
	for i := range db.Accounts {
		stats[db.Accounts[i].ID] = &accountStats{}
	}
	// Propriétaire = premier utilisateur (owner ou admin) du compte.
	owners := map[string]string{}
	for i := range db.Users {
		u := &db.Users[i]
		if _, ok := stats[u.AccountID]; !ok {
			continue
		}
		if u.Role == "owner" || u.Role == "admin" {
			if _, seen := owners[u.AccountID]; !seen {
				owners[u.AccountID] = u.Username
			}
		}
	}
	for i := range db.HotspotUsers {
		if st, ok := stats[db.HotspotUsers[i].AccountID]; ok {
			st.Users++
		}
	}
	for i := range db.Routers {
		if st, ok := stats[db.Routers[i].AccountID]; ok {
			st.Routers++
		}
	}
	for i := range db.Sessions {
		if st, ok := stats[db.Sessions[i].AccountID]; ok {
			st.Sessions++
		}
	}
	// Stats admin : mêmes règles RÉELLES que les rapports — ventes directes
	// consommées + encaissements revendeurs nets des retours (générer du
	// stock n'est pas vendre ; voir helpers en tête de fichier).
	for i := range db.HotspotUsers {
		u := &db.HotspotUsers[i]
		if u.Kind != "voucher" || u.ResellerID != "" {
			continue
		}
		st, ok := stats[u.AccountID]
		if !ok {
			continue
		}
		if at, sold := voucherSoldDate(u); sold && !at.Before(cutoff30) {
			st.Sales30d++
			st.Revenue30d += directSalePrice(u)
		}
	}
	for _, t := range db.Transactions {
		gain := resellerNetIncome(t)
		if gain == 0 {
			continue
		}
		st, ok := stats[t.AccountID]
		if !ok {
			continue
		}
		if at, err := time.Parse(time.RFC3339, t.At); err == nil && at.After(cutoff30) {
			st.Revenue30d += gain
		}
	}
	rows := make([]accountRow, 0, len(db.Accounts))
	for i := range db.Accounts {
		acc := db.Accounts[i]
		sub := "essai"
		if settings, ok := db.SettingsByAccount[acc.ID]; ok {
			switch subscriptionStatus(settings.Subscription, now) {
			case "active":
				sub = "active"
			case "expired":
				sub = "expired"
			default:
				sub = "essai" // « none » : aucun plan souscrit (essai)
			}
		}
		rows = append(rows, accountRow{
			ID: acc.ID, Name: acc.Name, Status: acc.Status, CreatedAt: acc.CreatedAt,
			Owner: owners[acc.ID], Subscription: sub, Stats: *stats[acc.ID],
		})
	}
	a.store.Unlock()

	sort.Slice(rows, func(i, j int) bool { return rows[i].CreatedAt > rows[j].CreatedAt })
	writeJSON(w, http.StatusOK, rows)
}

// handleAdminAccountStatus — active ou désactive un compte SaaS. Effet immédiat :
// les tokens du compte sont bloqués par le middleware (401) et le login est
// refusé (403). Le compte principal ne peut pas être désactivé.
func (a *API) handleAdminAccountStatus(w http.ResponseWriter, r *http.Request) {
	if !isPlatformAdmin(r) {
		writeErr(w, http.StatusForbidden, "Réservé aux administrateurs de la plateforme")
		return
	}
	id := r.PathValue("id")
	var req struct {
		Status string `json:"status"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	if req.Status != "active" && req.Status != "disabled" {
		writeErr(w, http.StatusBadRequest, "Statut invalide (active ou disabled)")
		return
	}
	a.store.Lock()
	db := a.store.Data()
	var acc *model.Account
	for i := range db.Accounts {
		if db.Accounts[i].ID == id {
			acc = &db.Accounts[i]
			break
		}
	}
	if acc == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Compte introuvable")
		return
	}
	acc.Status = req.Status
	verb := "activé"
	if req.Status == "disabled" {
		verb = "désactivé"
	}
	a.logActivityBy(r, db, id, "system", "Compte «"+acc.Name+"» "+verb+" par la plateforme")
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
