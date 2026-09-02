// handlers_accounting.go — comptabilité client (vue période, export CSV).

package api

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"mikcloud/hotspot-api/internal/model"
)

func (a *API) handleAccounting(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	q := r.URL.Query()
	period := q.Get("period")
	if period == "" {
		period = "day"
	}
	if period != "day" && period != "week" && period != "month" {
		writeErr(w, http.StatusBadRequest, "period doit valoir day, week ou month")
		return
	}
	routerID := strings.TrimSpace(q.Get("routerId")) // "" ou "all" = tous les sites

	a.store.Lock()
	result := buildAccounting(a.store.Data(), acc, period, routerID, time.Now().UTC())
	a.store.Unlock()
	writeJSON(w, http.StatusOK, result)
}

// handleAccountingExport — export CSV (séparateur « ; », BOM UTF-8 pour Excel)
// de la comptabilité : une ligne par période + totaux + répartition par site.
func (a *API) handleAccountingExport(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	q := r.URL.Query()
	period := q.Get("period")
	if period == "" {
		period = "day"
	}
	if period != "day" && period != "week" && period != "month" {
		writeErr(w, http.StatusBadRequest, "period doit valoir day, week ou month")
		return
	}
	routerID := strings.TrimSpace(q.Get("routerId"))

	a.store.Lock()
	result := buildAccounting(a.store.Data(), acc, period, routerID, time.Now().UTC())
	tenantName := ensureSettings(a.store.Data(), acc).Tenant.Name
	a.store.Unlock()

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"mikcloud-comptabilite-%s.csv\"", period))
	// BOM UTF-8 : Excel reconnaît l'encodage.
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})
	_, _ = w.Write([]byte(fmt.Sprintf("MikCloud ; Comptabilite ;%s ;periode=%s\r\n", csvField(tenantName), period)))
	_, _ = w.Write([]byte("Periode ;Ventes ;Chiffre d'affaires (FCFA) ;Coût (FCFA) ;Marge (FCFA)\r\n"))
	type seriesRow struct {
		Label   string `json:"label"`
		Revenue int    `json:"revenue"`
		Sales   int    `json:"sales"`
		Cost    int    `json:"cost"`
		Selling int    `json:"selling"`
	}
	if raw, err := json.Marshal(result["series"]); err == nil {
		var rows []seriesRow
		if json.Unmarshal(raw, &rows) == nil {
			for _, row := range rows {
				_, _ = w.Write([]byte(fmt.Sprintf("%s ;%d ;%d ;%d ;%d\r\n",
					csvField(row.Label), row.Sales, row.Revenue, row.Cost, row.Selling-row.Cost)))
			}
		}
	}
	if totals, ok := result["totals"].(map[string]any); ok {
		cost, _ := totals["cost"].(int)
		selling, _ := totals["selling"].(int)
		_, _ = w.Write([]byte(fmt.Sprintf("TOTAL ;%v ;%v ;%d ;%d\r\n", totals["sales"], totals["revenue"], cost, selling-cost)))
	}
	_, _ = w.Write([]byte("\r\nSite ;Ventes ;Chiffre d'affaires (FCFA) ;Coût (FCFA) ;Marge (FCFA) ;Part (% )\r\n"))
	type routerRow struct {
		RouterName string  `json:"routerName"`
		Revenue    int     `json:"revenue"`
		Sales      int     `json:"sales"`
		Cost       int     `json:"cost"`
		Selling    int     `json:"selling"`
		Share      float64 `json:"share"`
	}
	if raw, err := json.Marshal(result["byRouter"]); err == nil {
		var rows []routerRow
		if json.Unmarshal(raw, &rows) == nil {
			for _, row := range rows {
				_, _ = w.Write([]byte(fmt.Sprintf("%s ;%d ;%d ;%d ;%d ;%v\r\n",
					csvField(row.RouterName), row.Sales, row.Revenue, row.Cost, row.Selling-row.Cost, row.Share)))
			}
		}
	}
}

// csvField — neutralise les séparateurs/retours à la ligne dans une cellule CSV.
func csvField(s string) string {
	if strings.ContainsAny(s, ";\"\n\r") {
		return "\"" + strings.NewReplacer("\"", "\"\"", "\n", " ", "\r", "").Replace(s) + "\""
	}
	return s
}

// buildAccounting — cœur de calcul partagé entre la réponse JSON et l'export CSV
// (l'appelant tient le verrou du store).
func buildAccounting(db *model.DB, acc, period, routerID string, now time.Time) map[string]any {
	// Découpage en buckets + fenêtre d'analyse.
	var buckets []time.Time
	var labels []string
	var windowStart time.Time

	switch period {
	case "day": // 30 derniers jours
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		windowStart = today.AddDate(0, 0, -29)
		for i := 29; i >= 0; i-- {
			d := today.AddDate(0, 0, -i)
			buckets = append(buckets, d)
			labels = append(labels, fmt.Sprintf("%02d/%02d", d.Day(), int(d.Month())))
		}
	case "week": // 12 dernières semaines (lundi → dimanche)
		wd := int(now.Weekday())
		if wd == 0 {
			wd = 7
		}
		thisMonday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -(wd - 1))
		for i := 11; i >= 0; i-- {
			monday := thisMonday.AddDate(0, 0, -7*i)
			buckets = append(buckets, monday)
			labels = append(labels, fmt.Sprintf("%02d/%02d", monday.Day(), int(monday.Month())))
		}
		windowStart = thisMonday.AddDate(0, 0, -77)
	case "month": // 12 derniers mois
		firstOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		windowStart = firstOfMonth.AddDate(0, -11, 0)
		for i := 11; i >= 0; i-- {
			m := firstOfMonth.AddDate(0, -i, 0)
			buckets = append(buckets, m)
			labels = append(labels, fmt.Sprintf("%s %02d", frenchMonth[int(m.Month())-1], m.Year()%100))
		}
	}

	// Fenêtre précédente de même longueur — comparaison Δ% (tendance) :
	// 30 j ← 30 j, 12 semaines ← 12 semaines, 12 mois ← 12 mois.
	var prevStart time.Time
	switch period {
	case "day":
		prevStart = windowStart.AddDate(0, 0, -30)
	case "week":
		prevStart = windowStart.AddDate(0, 0, -84)
	case "month":
		prevStart = windowStart.AddDate(0, -12, 0)
	}

	type seriesPoint struct {
		Label   string `json:"label"`
		Revenue int    `json:"revenue"`
		Sales   int    `json:"sales"`
		Cost    int    `json:"cost"`    // F13 : coût agrégé
		Selling int    `json:"selling"` // F13 : total vente agrégé
	}
	revSeries := make([]seriesPoint, len(buckets))
	for i := range labels {
		revSeries[i] = seriesPoint{Label: labels[i]}
	}

	routerNames := map[string]string{}
	for _, rr := range db.Routers {
		if rr.AccountID == acc {
			routerNames[rr.ID] = rr.Name
		}
	}

	totalsRevenue, totalsSales, totalsCost, totalsSelling := 0, 0, 0, 0
	prevRevenue, prevSales, prevCost, prevSelling := 0, 0, 0, 0
	chanDirectRevenue, chanResellerRevenue, chanDirectSales, chanResellerSales := 0, 0, 0, 0
	byRouterRevenue := map[string]int{}
	byRouterSales := map[string]int{}
	byRouterCost := map[string]int{}
	byRouterSelling := map[string]int{}

	globalScope := routerID == "" || routerID == "all"
	events := collectSaleEvents(db, acc, prevStart)
	sold := collectSoldVouchers(db, acc, prevStart)

	bucketIndex := func(at time.Time) int {
		return sort.Search(len(buckets), func(i int) bool { return at.Before(buckets[i]) }) - 1
	}

	// TRÉSORERIE RÉELLE — ventes directes (tickets consommés, prix réellement
	// payé, routables par site) +, en vue globale uniquement, encaissements
	// revendeurs nets (achats/versements − retours : les transactions ne sont
	// liées à aucun site).
	for _, e := range events {
		inScope := globalScope || e.RouterID == routerID
		if !inScope {
			continue
		}
		if e.Reseller != "" && !globalScope {
			continue // couvert par la valeur gros écoulée du site, ci-dessous
		}
		if e.At.Before(windowStart) {
			prevRevenue += e.Amount
			continue
		}
		totalsRevenue += e.Amount
		if e.Reseller == "" {
			chanDirectRevenue += e.Amount
		} else {
			chanResellerRevenue += e.Amount
		}
		if idx := bucketIndex(e.At); idx >= 0 && idx < len(revSeries) {
			revSeries[idx].Revenue += e.Amount
		}
	}
	if !globalScope {
		// SITE FILTRÉ : la part revendeurs du site = valeur GROS de ses
		// tickets réseau écoulés (proxy honnête — l'encaissement réel des
		// achats de stock n'est pas attribuable à un site).
		for _, v := range sold {
			if v.ResellerName == "" || v.RouterID != routerID {
				continue
			}
			if v.At.Before(windowStart) {
				prevRevenue += v.Cost
				continue
			}
			totalsRevenue += v.Cost
			chanResellerRevenue += v.Cost
			if idx := bucketIndex(v.At); idx >= 0 && idx < len(revSeries) {
				revSeries[idx].Revenue += v.Cost
			}
		}
	}

	// ÉCOULEMENTS — volume de tickets réellement passés au client (remis ou
	// consommés) : série « Ventes », valorisation F13 (public/gros) et
	// répartition par site.
	for _, v := range sold {
		rid := v.RouterID
		if rid == "" {
			rid = "unknown"
		}
		if routerNames[rid] == "" {
			routerNames[rid] = "(site supprimé)"
		}

		if v.At.Before(windowStart) {
			// Fenêtre précédente — uniquement les totaux Δ% (même filtre).
			if globalScope || v.RouterID == routerID {
				prevSales++
				prevCost += v.Cost
				prevSelling += v.Public
			}
			continue
		}

		// Répartition par site — fenêtre affichée, TOUS les sites (comparaison) :
		// CA du site = direct réel (prix payé) + réseau à la valeur gros.
		siteRevenue := v.Public
		if v.ResellerName != "" {
			siteRevenue = v.Cost
		}
		byRouterRevenue[rid] += siteRevenue
		byRouterSales[rid]++
		byRouterCost[rid] += v.Cost
		byRouterSelling[rid] += v.Public

		if globalScope || v.RouterID == routerID {
			totalsSales++
			totalsCost += v.Cost
			totalsSelling += v.Public
			if idx := bucketIndex(v.At); idx >= 0 && idx < len(revSeries) {
				revSeries[idx].Sales++
				revSeries[idx].Cost += v.Cost
				revSeries[idx].Selling += v.Public
			}
			if v.ResellerName == "" {
				chanDirectSales++
			} else {
				chanResellerSales++
			}
		}
	}

	type routerAgg struct {
		RouterID   string  `json:"routerId"`
		RouterName string  `json:"routerName"`
		Revenue    int     `json:"revenue"`
		Sales      int     `json:"sales"`
		Cost       int     `json:"cost"`    // F13
		Selling    int     `json:"selling"` // F13
		Share      float64 `json:"share"`
	}
	byRouter := []routerAgg{}
	allSum := 0
	for _, rev := range byRouterRevenue {
		allSum += rev
	}
	for rid, rev := range byRouterRevenue {
		share := 0.0
		if allSum > 0 {
			share = math.Round(float64(rev)/float64(allSum)*1000) / 10
		}
		byRouter = append(byRouter, routerAgg{
			RouterID: rid, RouterName: routerNames[rid],
			Revenue: rev, Sales: byRouterSales[rid],
			Cost: byRouterCost[rid], Selling: byRouterSelling[rid], Share: share,
		})
	}
	sort.Slice(byRouter, func(i, j int) bool { return byRouter[i].Revenue > byRouter[j].Revenue })

	avgTicket := 0
	if totalsSales > 0 {
		avgTicket = totalsRevenue / totalsSales
	}
	prevAvgTicket := 0
	if prevSales > 0 {
		prevAvgTicket = prevRevenue / prevSales
	}

	return map[string]any{
		"period":   period,
		"routerId": routerID,
		"series":   revSeries,
		"byRouter": byRouter,
		// v2 — comparaison Δ% avec la fenêtre précédente de même longueur
		// (mêmes totaux : revenue/sales/avgTicket/marge).
		"prev": map[string]any{
			"revenue":   prevRevenue,
			"sales":     prevSales,
			"avgTicket": prevAvgTicket,
			"margin":    prevSelling - prevCost,
		},
		// v2 — CA par canal de distribution (direct vs revendeurs),
		// dans la même fenêtre et le même filtre routeur.
		"channel": map[string]any{
			"directRevenue":   chanDirectRevenue,
			"resellerRevenue": chanResellerRevenue,
			"directSales":     chanDirectSales,
			"resellerSales":   chanResellerSales,
		},
		"totals": map[string]any{
			"revenue":   totalsRevenue,
			"sales":     totalsSales,
			"avgTicket": avgTicket,
			// F13 : coût, total vente et marge (selling − cost).
			"cost":    totalsCost,
			"selling": totalsSelling,
			"margin":  totalsSelling - totalsCost,
		},
	}
}
