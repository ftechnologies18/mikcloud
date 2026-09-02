// handlers_dashboard.go — tableau de bord et analytique (ventes, revenus, top profils).

package api

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"mikcloud/hotspot-api/internal/model"
	"mikcloud/hotspot-api/internal/store"
)

type timelinePoint struct {
	T     string `json:"t"`
	Value int    `json:"value"`
}

type dayValue struct {
	Day   string `json:"day"`
	Value int    `json:"value"`
}

type topProfilePoint struct {
	Name  string `json:"name"`
	Users int    `json:"users"`
	Total int    `json:"total"`
}

func (a *API) handleDashboard(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	now := time.Now().UTC()
	a.store.Lock()
	db := a.store.Data()
	// P0 (audit Mikhmon) : simulation vivante + expiration à jour +
	// enforcement routeur (F1), comme handleSessionsList/handleUsersList.
	store.Tick(db, now)
	a.enforceExpired(db)
	a.store.Save()

	// Vue d'ensemble multi-sites : 1 compte = N hotspots. Tous les agrégats
	// ci-dessous sont calculés DANS le compte demandeur (isolation stricte).
	type siteOverview struct {
		RouterID       string `json:"routerId"`
		RouterName     string `json:"routerName"`
		Status         string `json:"status"`
		ActiveSessions int    `json:"activeSessions"`
		HotspotUsers   int    `json:"hotspotUsers"`
		OnlineUsers    int    `json:"onlineUsers"`
		ActiveVouchers int    `json:"activeVouchers"`
		SalesToday     int    `json:"salesToday"`
		SoldToday      int    `json:"soldToday"`
		Revenue30d     int    `json:"revenue30d"`
	}

	accSessions := []model.Session{}
	onlineNow := map[string]bool{} // clé routerID|username — users EN LIGNE
	onlineByRouter := map[string]int{}
	sessionsByRouter := map[string]int{}
	for i := range db.Sessions {
		s := db.Sessions[i]
		if s.AccountID != acc {
			continue
		}
		accSessions = append(accSessions, s)
		// « Utilisateurs actifs » = users avec une session LIVE, identifiés
		// par (routeur, username) : le même username peut exister sur deux
		// routeurs sans être le même client — ni compter double sur un seul.
		if key := s.RouterID + "|" + s.Username; !onlineNow[key] {
			onlineNow[key] = true
			onlineByRouter[s.RouterID]++
		}
		sessionsByRouter[s.RouterID]++
	}

	// « Ticket vendu = ticket utilisé » : un voucher est compté vendu le jour
	// de sa PREMIÈRE connexion (UsedAt horodaté par markVoucherUsed au login
	// détecté, fuseau du compte). Les Sales enregistrées à la génération
	// restent la source du chiffre d'affaires ; ce compteur mesure l'ACTIVATION
	// réelle des tickets — c'est lui que le dashboard affiche en « vendus ».
	loc := accountTimezone(db, acc)
	nowLocal := now.In(loc)
	todayStartLocal := time.Date(nowLocal.Year(), nowLocal.Month(), nowLocal.Day(), 0, 0, 0, 0, loc)
	soldToday := 0
	soldTodayByRouter := map[string]int{}
	totalUsers, activeVouchers := 0, 0
	usersByRouter := map[string]int{}
	vouchersByRouter := map[string]int{}
	counts := map[string]int{}
	totals := map[string]int{}
	for i := range db.HotspotUsers {
		u := &db.HotspotUsers[i]
		if u.AccountID != acc {
			continue
		}
		totalUsers++
		counts[u.ProfileName]++
		totals[u.ProfileName] += u.Price
		if u.Kind == "voucher" && u.UsedAt != "" {
			if used, err := time.Parse(time.RFC3339, u.UsedAt); err == nil && !used.In(loc).Before(todayStartLocal) {
				soldToday++
				soldTodayByRouter[u.RouterID]++
			}
		}
		if model.EffectiveStatus(u, now) != "active" {
			continue
		}
		usersByRouter[u.RouterID]++
		if u.Kind == "voucher" {
			vouchersByRouter[u.RouterID]++
			activeVouchers++
		}
	}

	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	cutoff30 := now.AddDate(0, 0, -30)
	salesToday, revenue30d := 0, 0
	salesTodayByRouter := map[string]int{}
	revenue30dByRouter := map[string]int{}
	// Revenus RÉELS (voir helpers en tête de fichier) : ventes directes
	// consommées + encaissements revendeurs nets des retours — générer du
	// stock n'est pas vendre.
	for _, e := range collectSaleEvents(db, acc, cutoff30) {
		revenue30d += e.Amount
		revenue30dByRouter[e.RouterID] += e.Amount
	}
	// Volume : tickets ÉCULÉS du jour (remis au client ou consommés).
	for _, v := range collectSoldVouchers(db, acc, todayStart) {
		salesToday++
		salesTodayByRouter[v.RouterID]++
	}

	accRouters := []model.Router{}
	routersOnline := 0
	for i := range db.Routers {
		rr := db.Routers[i]
		if rr.AccountID != acc {
			continue
		}
		accRouters = append(accRouters, rr)
		if rr.Status == "online" {
			routersOnline++
		}
	}

	sites := []siteOverview{}
	for _, rr := range accRouters {
		sites = append(sites, siteOverview{
			RouterID:       rr.ID,
			RouterName:     rr.Name,
			Status:         rr.Status,
			ActiveSessions: sessionsByRouter[rr.ID],
			HotspotUsers:   usersByRouter[rr.ID],
			OnlineUsers:    onlineByRouter[rr.ID],
			ActiveVouchers: vouchersByRouter[rr.ID],
			SalesToday:     salesTodayByRouter[rr.ID],
			SoldToday:      soldTodayByRouter[rr.ID],
			Revenue30d:     revenue30dByRouter[rr.ID],
		})
	}
	kpis := map[string]any{
		"activeSessions": len(accSessions),
		"totalUsers":     totalUsers,
		"activeVouchers": activeVouchers,
		"soldToday":      soldToday,
		"salesToday":     salesToday,
		"revenue30d":     revenue30d,
		"routersOnline":  routersOnline,
		"routersTotal":   len(accRouters),
		"onlineNow":      len(onlineNow),
	}

	revenueByDay := buildRevenueByDay(db, acc, now, 14)

	top := []topProfilePoint{}
	for name, c := range counts {
		top = append(top, topProfilePoint{Name: name, Users: c, Total: totals[name]})
	}
	sort.Slice(top, func(i, j int) bool {
		if top[i].Users != top[j].Users {
			return top[i].Users > top[j].Users
		}
		return top[i].Total > top[j].Total
	})
	if len(top) > 5 {
		top = top[:5]
	}

	recent := []model.Activity{}
	for _, act := range db.Activity {
		if act.AccountID != acc {
			continue
		}
		if len(recent) >= 12 {
			break
		}
		recent = append(recent, act)
	}
	timeline := buildHourlyLogins(db, acc, now, loc)
	// N°19 V2 — créances revendeurs (dépôt-vente) : trésorerie dormant
	// chez les revendeurs, avec ancienneté et verrou plafond.
	receivables := buildReceivables(db, acc, now)
	a.store.Unlock()

	// N°10 — courbe 24 h RÉELLE : connexions/heure agrégées depuis les
	// UserLogs (simulation + agent), dans le fuseau du compte. Un compte
	// sans activité affiche zéro — honnête par construction.
	writeJSON(w, http.StatusOK, map[string]any{
		"kpis":             kpis,
		"sites":            sites,
		"sessionsTimeline": timeline,
		"revenueByDay":     revenueByDay,
		"topProfiles":      top,
		"recentActivity":   recent,
		"receivables":      receivables,
	})
}

// ---------------------------------------------------------------------------
// Revenus RÉELS — source de vérité unique des modules financiers (rapports,
// comptabilité, marge, dashboard).
//
// Logique métier mikCloud : générer du stock N'EST PAS vendre. Un revenu ne
// naît qu'à un événement d'argent réel :
//
//   - vente DIRECTE : un voucher du stock direct (sans ResellerID) n'est
//     compté que lorsqu'un client le CONSOMME (UsedAt — première connexion)
//     au prix réellement payé (SellingPrice sinon Price) ;
//   - canal REVENDEUR : l'argent suit les Transactions — achats de stock
//     prépayés (type « sale » : génération pour revendeur OU transfert N°18)
//     + versements dépôt-vente (type « settlement » : espèces OU compensation
//     crédit N°19) − retours de stock recrédités (type « credit » noté
//     « Retour de stock : » — N°18/N°20). Un rechargement de portefeuille
//     (type « credit ») est une AVANCE, pas un revenu.
//
// Conséquence : un retour de stock n'appelle AUCUNE écriture négative dans
// les rapports — le ticket rendu retourne au stock et ne sera compté que le
// jour où il sera réellement écoulé (achats 1000 − retours 600 = 400 nets).
// ---------------------------------------------------------------------------

// txNoteReturn — préfixe normalisé des Transactions de retour de stock
// (handlers_sell.go N°20 et handlers_transfer.go N°18). Le rechargement
// manuel (handleResellerCredit) utilise le même type « credit » mais une
// autre note : c'est une avance, jamais un revenu ni un retour.
const txNoteReturn = "Retour de stock :"

// txNoteCompensation — écriture de portefeuille d'un versement dépôt-vente
// compensé par le crédit prépayé (handlers_deposit.go N°19 v2) : double
// écriture du même « settlement », à ne compter qu'une seule fois.
const txNoteCompensation = "Crédit prépayé converti en versement"

// directSalePrice — prix réellement payé par le client final d'un voucher
// direct (prix de vente copié du profil à la génération, sinon prix gros).
func directSalePrice(u *model.HotspotUser) int {
	if u.SellingPrice > 0 {
		return u.SellingPrice
	}
	return u.Price
}

// voucherSoldDate — date d'ÉCOULEMENT d'un voucher : la remise au client
// (Mode Vente, SoldAt) sinon la première connexion (UsedAt) — les dates
// RFC3339 UTC sont comparables lexicalement. Vide = jamais éculé (stock).
func voucherSoldDate(u *model.HotspotUser) (time.Time, bool) {
	raw := u.SoldAt
	if u.UsedAt != "" && (raw == "" || u.UsedAt < raw) {
		raw = u.UsedAt
	}
	if raw == "" {
		return time.Time{}, false
	}
	at, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, false
	}
	return at, true
}

// resellerNetIncome — contribution TRÉSORERIE d'une transaction au revenu
// revendeurs : +achats de stock prépayés et versements dépôt-vente,
// −retours de stock recrédités ; 0 pour tout le reste (avances/rechargements,
// créances naissantes, écritures de portefeuille de compensation…).
func resellerNetIncome(t model.Transaction) int {
	switch t.Type {
	case "sale":
		if strings.HasPrefix(t.Note, txNoteCompensation) {
			return 0 // doublon du « settlement » déjà compté
		}
		return t.Amount
	case "settlement":
		return t.Amount
	case "credit":
		if strings.HasPrefix(t.Note, txNoteReturn) {
			return -t.Amount // retour de stock : l'argent repart au revendeur
		}
		return 0
	}
	return 0
}

// saleEvent — un événement de revenu réel, normalisé pour toutes les
// agrégations (courbes, KPI, canal, top revendeurs, export CSV).
type saleEvent struct {
	At       time.Time
	Amount   int // argent perçu par le propriétaire (trésorerie)
	Selling  int // valorisation prix public (analyse F13)
	Cost     int // valorisation prix gros (coût de référence réseau)
	RouterID string
	Profile  string
	Reseller string // vide = vente directe
}

// collectSaleEvents — TOUS les événements de revenu réel du compte depuis
// `since` : ventes directes (tickets consommés) + encaissements revendeurs
// nets (transactions), triés chronologiquement. L'appelant tient le verrou.
func collectSaleEvents(db *model.DB, acc string, since time.Time) []saleEvent {
	events := []saleEvent{}
	for i := range db.HotspotUsers {
		u := &db.HotspotUsers[i]
		if u.AccountID != acc || u.Kind != "voucher" || u.ResellerID != "" {
			continue
		}
		at, ok := voucherSoldDate(u)
		if !ok || at.Before(since) {
			continue
		}
		events = append(events, saleEvent{
			At: at, Amount: directSalePrice(u), Selling: directSalePrice(u), Cost: u.Price,
			RouterID: u.RouterID, Profile: u.ProfileName,
		})
	}
	for _, t := range db.Transactions {
		if t.AccountID != acc {
			continue
		}
		gain := resellerNetIncome(t)
		if gain == 0 {
			continue
		}
		at, err := time.Parse(time.RFC3339, t.At)
		if err != nil || at.Before(since) {
			continue
		}
		// Revenu revendeur : le propriétaire encaisse le PRIX GROS — marge
		// publique nulle par construction (elle est cédée au revendeur).
		events = append(events, saleEvent{At: at, Amount: gain, Selling: gain, Cost: gain, Reseller: t.ResellerName})
	}
	sort.Slice(events, func(i, j int) bool { return events[i].At.Before(events[j].At) })
	return events
}

// soldVoucher — un voucher ÉCULÉ (remis au client ou consommé), quel que
// soit son canal : base des analyses de volume et de marge (profils, sites).
type soldVoucher struct {
	At           time.Time
	Public       int // valorisation prix public (SellingPrice sinon Price)
	Cost         int // valeur gros (Price)
	RouterID     string
	Profile      string
	ResellerID   string
	ResellerName string // vide = éculé en direct par le propriétaire
}

// collectSoldVouchers — tous les vouchers éculés du compte depuis `since`
// (date d'écoulement dans la fenêtre). L'appelant tient le verrou.
func collectSoldVouchers(db *model.DB, acc string, since time.Time) []soldVoucher {
	out := []soldVoucher{}
	for i := range db.HotspotUsers {
		u := &db.HotspotUsers[i]
		if u.AccountID != acc || u.Kind != "voucher" {
			continue
		}
		at, ok := voucherSoldDate(u)
		if !ok || at.Before(since) {
			continue
		}
		out = append(out, soldVoucher{
			At: at, Public: directSalePrice(u), Cost: u.Price,
			RouterID: u.RouterID, Profile: u.ProfileName,
			ResellerID: u.ResellerID, ResellerName: u.ResellerName,
		})
	}
	return out
}

// buildRevenueByDay — courbe du chiffre d'affaires RÉEL par jour (ventes
// directes consommées + encaissements revendeurs nets des retours).
func buildRevenueByDay(db *model.DB, acc string, now time.Time, days int) []dayValue {
	events := collectSaleEvents(db, acc, now.AddDate(0, 0, -days))
	out := make([]dayValue, 0, days)
	for i := days - 1; i >= 0; i-- {
		day := now.AddDate(0, 0, -i)
		start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC)
		end := start.Add(24 * time.Hour)
		sum := 0
		for _, e := range events {
			if !e.At.Before(start) && e.At.Before(end) {
				sum += e.Amount
			}
		}
		out = append(out, dayValue{Day: fmt.Sprintf("%02d/%02d", day.Day(), int(day.Month())), Value: sum})
	}
	return out
}
