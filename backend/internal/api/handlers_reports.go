// handlers_reports.go — transactions, journal d'activité et rapports de marge.

package api

import (
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"mikcloud/hotspot-api/internal/model"
)

func (a *API) handleTransactionsList(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	limit := queryInt(r, "limit", 20, 1, 200)
	a.store.Lock()
	txs := []model.Transaction{}
	for _, tx := range a.store.Data().Transactions {
		if tx.AccountID == acc {
			txs = append(txs, tx)
		}
	}
	a.store.Unlock()
	sort.Slice(txs, func(i, j int) bool { return txs[i].At > txs[j].At })
	if len(txs) > limit {
		txs = txs[:limit]
	}
	writeJSON(w, http.StatusOK, txs)
}

func (a *API) handleActivityList(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	limit := queryInt(r, "limit", 20, 1, 200)
	a.store.Lock()
	db := a.store.Data()
	acts := []model.Activity{}
	for _, act := range db.Activity {
		if act.AccountID == acc {
			acts = append(acts, act)
		}
	}
	a.store.Unlock()
	sort.Slice(acts, func(i, j int) bool { return acts[i].At > acts[j].At })
	if len(acts) > limit {
		acts = acts[:limit]
	}
	writeJSON(w, http.StatusOK, acts)
}

func (a *API) handleReports(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	days := 7
	if raw := r.URL.Query().Get("days"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || (v != 7 && v != 14 && v != 30) {
			writeErr(w, http.StatusBadRequest, "days doit valoir 7, 14 ou 30")
			return
		}
		days = v
	}
	now := time.Now().UTC()
	a.store.Lock()
	db := a.store.Data()

	revenueByDay := buildRevenueByDay(db, acc, now, days)

	since := now.AddDate(0, 0, -days)
	prevSince := now.AddDate(0, 0, -2*days) // v2 — comparaison Δ% (fenêtre précédente)

	// Revenus RÉELS (trésorerie) + écoulements (volume) — voir helpers en tête
	// de fichier : générer du stock n'est pas vendre, les retours se déduisent.
	events := collectSaleEvents(db, acc, prevSince)
	sold := collectSoldVouchers(db, acc, prevSince)

	profCount := map[string]int{}
	profRevenue := map[string]int{}
	prevRevenue, prevSales := 0, 0
	chanDirectRevenue, chanResellerRevenue := 0, 0
	chanDirectSales, chanResellerSales := 0, 0
	type resellerAgg struct{ sales, revenue int }
	topResellersAgg := map[string]*resellerAgg{}
	totals := struct {
		revenue int
		sales   int
	}{}
	// Trésorerie réelle : ventes directes consommées + encaissements
	// revendeurs nets (achats/versements − retours recrédités).
	for _, e := range events {
		if e.At.Before(since) {
			// Fenêtre précédente de même longueur — comparaison Δ%.
			prevRevenue += e.Amount
			continue
		}
		totals.revenue += e.Amount
		if e.Reseller == "" {
			chanDirectRevenue += e.Amount
		} else {
			chanResellerRevenue += e.Amount
			tr := topResellersAgg[e.Reseller]
			if tr == nil {
				tr = &resellerAgg{}
				topResellersAgg[e.Reseller] = tr
			}
			tr.revenue += e.Amount
		}
	}
	// Volume réel : tickets ÉCULÉS (remis au client ou consommés) —
	// indépendant du canal financier (prépayé payé à l'achat, dépôt-vente
	// payé au versement) : ce que les clients finaux ont reçu.
	for _, v := range sold {
		if v.At.Before(since) {
			prevSales++
			continue
		}
		totals.sales++
		profCount[v.Profile]++
		profRevenue[v.Profile] += v.Public
		if v.ResellerName == "" {
			chanDirectSales++
		} else {
			chanResellerSales++
			if tr := topResellersAgg[v.ResellerName]; tr != nil {
				tr.sales++
			} else {
				topResellersAgg[v.ResellerName] = &resellerAgg{sales: 1}
			}
		}
	}
	// Top 5 revendeurs par ENCAISSEMENT NET de la fenêtre (achats prépayés +
	// versements dépôt-vente − retours recrédités) — mikCloud : le réseau de
	// distribution est un moteur clé — savoir qui rapporte vraiment.
	type resellerPerf struct {
		Name    string `json:"name"`
		Sales   int    `json:"sales"`
		Revenue int    `json:"revenue"`
	}
	topResellers := []resellerPerf{}
	for name, tr := range topResellersAgg {
		if strings.TrimSpace(name) == "" {
			name = "(inconnu)"
		}
		topResellers = append(topResellers, resellerPerf{Name: name, Sales: tr.sales, Revenue: tr.revenue})
	}
	sort.Slice(topResellers, func(i, j int) bool {
		if topResellers[i].Revenue != topResellers[j].Revenue {
			return topResellers[i].Revenue > topResellers[j].Revenue
		}
		return topResellers[i].Name < topResellers[j].Name
	})
	if len(topResellers) > 5 {
		topResellers = topResellers[:5]
	}
	type profileSale struct {
		Name    string `json:"name"`
		Count   int    `json:"count"`
		Revenue int    `json:"revenue"`
	}
	salesByProfile := []profileSale{}
	for name, c := range profCount {
		salesByProfile = append(salesByProfile, profileSale{Name: name, Count: c, Revenue: profRevenue[name]})
	}
	sort.Slice(salesByProfile, func(i, j int) bool { return salesByProfile[i].Revenue > salesByProfile[j].Revenue })

	// N°21 (zéro donnée inventée) — l'ancienne courbe « trafic réseau »
	// était SYNTHÉTIQUE : des octets tirés au sort (rand seedé par jour),
	// mis à l'échelle par le nombre de sessions. Elle est SUPPRIMÉE. Les
	// sessions fermées ne sont pas conservées en base — aucun historique
	// d'octets n'existe — toute courbe de trafic passé serait donc
	// inventée. À la place : les CONNEXIONS réellement journalisées
	// (UserLogs action=login — la même source que la heatmap horaire
	// N°10), regroupées par jour local du compte.
	loc := accountTimezone(db, acc)
	type dayLogins struct {
		Day   string `json:"day"`
		Count int    `json:"count"`
	}
	todayLocal := now.In(loc)
	dayIndex := make(map[string]int, days)
	loginsByDay := make([]dayLogins, days)
	for i := 0; i < days; i++ {
		d := todayLocal.AddDate(0, 0, -(days - 1 - i))
		dayIndex[d.Format("2006-01-02")] = i
		loginsByDay[i] = dayLogins{Day: d.Format("02/01")}
	}
	for i := range db.UserLogs {
		l := &db.UserLogs[i]
		if l.AccountID != acc || l.Action != "login" {
			continue
		}
		at, err := time.Parse(time.RFC3339, l.At)
		if err != nil {
			continue
		}
		if idx, ok := dayIndex[at.In(loc).Format("2006-01-02")]; ok {
			loginsByDay[idx].Count++
		}
	}

	// Sessions RÉELLES de la fenêtre (comptage + trafic cumulé) — KPI
	// réseau véridique : seules les sessions effectivement ouvertes sur
	// la fenêtre sont comptées, jamais de valeur estimée.
	var winSessions int
	var winBytesIn, winBytesOut int64
	for _, s := range db.Sessions {
		if s.AccountID != acc {
			continue
		}
		if st, err := time.Parse(time.RFC3339, s.StartedAt); err == nil && st.Before(since) {
			continue
		}
		winSessions++
		winBytesIn += s.BytesIn
		winBytesOut += s.BytesOut
	}

	voucherStatus := map[string]int{"active": 0, "used": 0, "expired": 0, "disabled": 0}
	online := onlineSessions(db, now)
	for i := range db.HotspotUsers {
		u := &db.HotspotUsers[i]
		if u.AccountID != acc {
			continue
		}
		if u.Kind == "voucher" {
			st := model.ResolvedStatus(u, online[onlineKey(u)], now)
			if st == "online" {
				st = "used" // agrégat : en ligne = consommé (session en cours)
			}
			voucherStatus[st]++
		}
	}
	avgTicket := 0
	if totals.sales > 0 {
		avgTicket = totals.revenue / totals.sales
	}
	prevAvgTicket := 0
	if prevSales > 0 {
		prevAvgTicket = prevRevenue / prevSales
	}
	// P0 (audit Mikhmon) — bloc marge (F13) : prix de vente vs coût sur
	// 30 jours glissants, par profil + totaux (cohérent avec les ventes).
	margin := buildMarginReport(db, acc, now)
	a.store.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"revenueByDay":   revenueByDay,
		"salesByProfile": salesByProfile,
		"loginsByDay":    loginsByDay,
		"voucherStatus":  voucherStatus,
		"margin":         margin,
		// v2 — comparaison Δ% avec la fenêtre précédente de même longueur.
		"prev": map[string]any{
			"revenue":   prevRevenue,
			"sales":     prevSales,
			"avgTicket": prevAvgTicket,
		},
		// v2 — CA par canal de distribution (direct vs revendeurs).
		"channel": map[string]any{
			"directRevenue":   chanDirectRevenue,
			"resellerRevenue": chanResellerRevenue,
			"directSales":     chanDirectSales,
			"resellerSales":   chanResellerSales,
		},
		// v2 — top 5 revendeurs par CA de la fenêtre (leaderboard).
		"topResellers": topResellers,
		// v2 — sessions réelles de la fenêtre : comptage + trafic cumulé.
		"sessions": map[string]any{
			"count":    winSessions,
			"bytesIn":  winBytesIn,
			"bytesOut": winBytesOut,
		},
		"totals": map[string]any{
			"revenue":   totals.revenue,
			"sales":     totals.sales,
			"avgTicket": avgTicket,
		},
	})
}

// buildMarginReport — F13 : analyse de la VALEUR écoulée (tickets réellement
// remis au client ou consommés) : prix public vs prix gros, 30 jours glissants,
// par profil + totaux + par site + courbe quotidienne. La marge des ventes
// directes est captée par le propriétaire ; celle des tickets réseau est
// cédée au revendeur (il encaisse public, le propriétaire a déjà encaissé le
// gros à l'achat) — l'analyse reste la même pour les deux canaux.
func buildMarginReport(db *model.DB, acc string, now time.Time) map[string]any {
	since := now.AddDate(0, 0, -30)
	prevSince := now.AddDate(0, 0, -60)
	type profileMargin struct {
		Name    string `json:"name"`
		Sold    int    `json:"sold"`
		Revenue int    `json:"revenue"`
		Cost    int    `json:"cost"`
		Margin  int    `json:"margin"`
	}
	type routerMargin struct {
		RouterName string `json:"routerName"`
		Revenue    int    `json:"revenue"`
		Cost       int    `json:"cost"`
		Margin     int    `json:"margin"`
	}
	byProfile := map[string]*profileMargin{}
	byRouter := map[string]*routerMargin{}
	routerNames := map[string]string{}
	for _, rr := range db.Routers {
		if rr.AccountID == acc {
			routerNames[rr.ID] = rr.Name
		}
	}
	// Buckets quotidiens (le plus ancien d'abord) pour la courbe de marge.
	type dayMargin struct {
		Day    string `json:"day"`
		Margin int    `json:"margin"`
	}
	dayStarts := make([]time.Time, 30)
	byDay := make([]dayMargin, 30)
	for i := 0; i < 30; i++ {
		d := now.AddDate(0, 0, -(29 - i))
		dayStarts[i] = time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC)
		byDay[i] = dayMargin{Day: fmt.Sprintf("%02d/%02d", d.Day(), int(d.Month()))}
	}
	totalRevenue, totalCost := 0, 0
	prevRevenue, prevCost := 0, 0
	for _, v := range collectSoldVouchers(db, acc, prevSince) {
		if v.At.Before(since) {
			// 30 jours précédents — comparaison Δ%.
			prevRevenue += v.Public
			prevCost += v.Cost
			continue
		}
		pm, ok := byProfile[v.Profile]
		if !ok {
			pm = &profileMargin{Name: v.Profile}
			byProfile[v.Profile] = pm
		}
		pm.Sold++
		pm.Revenue += v.Public
		pm.Cost += v.Cost
		pm.Margin += v.Public - v.Cost
		totalRevenue += v.Public
		totalCost += v.Cost
		// Profitabilité par site (multi-sites : quel routeur rapporte ?).
		rid := v.RouterID
		if rid == "" {
			rid = "unknown"
		}
		if routerNames[rid] == "" {
			routerNames[rid] = "(site supprimé)"
		}
		rm, ok := byRouter[rid]
		if !ok {
			rm = &routerMargin{RouterName: routerNames[rid]}
			byRouter[rid] = rm
		}
		rm.Revenue += v.Public
		rm.Cost += v.Cost
		rm.Margin += v.Public - v.Cost
		// Courbe quotidienne de la marge.
		if idx := sort.Search(len(dayStarts), func(i int) bool { return v.At.Before(dayStarts[i]) }) - 1; idx >= 0 {
			byDay[idx].Margin += v.Public - v.Cost
		}
	}
	rows := make([]profileMargin, 0, len(byProfile))
	for _, pm := range byProfile {
		rows = append(rows, *pm)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Revenue != rows[j].Revenue {
			return rows[i].Revenue > rows[j].Revenue
		}
		return rows[i].Name < rows[j].Name
	})
	routerRows := make([]routerMargin, 0, len(byRouter))
	for _, rm := range byRouter {
		routerRows = append(routerRows, *rm)
	}
	sort.Slice(routerRows, func(i, j int) bool {
		if routerRows[i].Margin != routerRows[j].Margin {
			return routerRows[i].Margin > routerRows[j].Margin
		}
		return routerRows[i].RouterName < routerRows[j].RouterName
	})
	totalMargin := totalRevenue - totalCost
	marginPct := 0.0
	if totalRevenue > 0 {
		marginPct = math.Round(float64(totalMargin)/float64(totalRevenue)*1000) / 10
	}
	return map[string]any{
		"revenue":   totalRevenue,
		"cost":      totalCost,
		"margin":    totalMargin,
		"marginPct": marginPct,
		"byProfile": rows,
		// v2 — comparaison Δ% vs les 30 jours précédents.
		"prev": map[string]any{
			"revenue": prevRevenue,
			"cost":    prevCost,
			"margin":  prevRevenue - prevCost,
		},
		// v2 — marge quotidienne (barres vertes/rouges selon le signe).
		"byDay": byDay,
		// v2 — marge par site (profitabilité multi-sites).
		"byRouter": routerRows,
	}
}

// ---------------------------------------------------------------------------
// Comptabilité — ventes par jour / semaine / mois, par routeur (multi-sites)
// ---------------------------------------------------------------------------

// frenchMonth — noms de mois français pour les libellés du graphe mensuel.
var frenchMonth = [...]string{"janv.", "févr.", "mars", "avr.", "mai", "juin", "juil.", "août", "sept.", "oct.", "nov.", "déc."}
