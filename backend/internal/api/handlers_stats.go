package api

// N°10 (feuille de route) — Statistiques horaires RÉELLES.
//
// Remplace la courbe synthétique « crédible » de l'ancien dashboard
// (buildSessionsTimeline, supprimée) : l'affluence est désormais agrégée
// depuis les UserLogs (action=login) réellement collectés — par le moteur
// de simulation OU par l'agent routeur (diff de sessions). Le CA horaire
// suit la doctrine « CONSOMMÉ » (N°198, constat C3 de l'audit reports) :
// ventes directes consommées au prix payé + encaissements revendeurs
// nets — l'ancienne lecture de db.Sales comptait la GÉNÉRATION de stock
// (créer des tickets n'est pas les vendre) et contredisait tous les
// autres chiffres du même onglet. Toutes les tranches horaires sont
// calculées dans le FUSEAU DU COMPTE (Tenant.Timezone, défaut UTC) :
// « pic 19h-22h » affiché = pic 19h-22h vécu sur place.

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"mikcloud/hotspot-api/internal/model"
)

// accountTimezone — fuseau du compte (SettingsByAccount → Tenant.Timezone).
// Repli UTC si absent ou inconnu (time/tzdata embarqué → jamais d'échec réel).
func accountTimezone(db *model.DB, acc string) *time.Location {
	if s, ok := db.SettingsByAccount[acc]; ok && s.Tenant.Timezone != "" {
		if loc, err := time.LoadLocation(s.Tenant.Timezone); err == nil {
			return loc
		}
	}
	return time.UTC
}

// hourBucket — début d'heure locale (tronque min/séc) d'un instant.
func hourBucket(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, t.Location())
}

// buildHourlyLogins — 24 points horaires RÉELS (connexions/heure), heure
// courante en dernier (dernier point = heure en cours, partielle). Fenêtre
// glissante de 24 h alignée sur l'heure locale du compte.
func buildHourlyLogins(db *model.DB, acc string, now time.Time, loc *time.Location) []timelinePoint {
	nowLocal := now.In(loc)
	cur := hourBucket(nowLocal)
	since := cur.Add(-23 * time.Hour)
	counts := make([]int, 24)
	for i := range db.UserLogs {
		l := &db.UserLogs[i]
		if l.AccountID != acc || l.Action != "login" {
			continue
		}
		at, err := time.Parse(time.RFC3339, l.At)
		if err != nil || at.Before(since) {
			continue
		}
		idx := int(hourBucket(at.In(loc)).Sub(cur).Hours()) // -23..0
		if idx < -23 || idx > 0 {
			continue
		}
		counts[idx+23]++
	}
	pts := make([]timelinePoint, 24)
	for i := 0; i < 24; i++ {
		h := (cur.Hour() - (23 - i) + 48) % 24
		pts[i] = timelinePoint{T: fmt.Sprintf("%02d:00", h), Value: counts[i]}
	}
	return pts
}

// hourlyRow — une ligne de la heatmap : un jour local, 24 compteurs.
type hourlyRow struct {
	Date  string `json:"date"`  // "2006-01-02" (local)
	Hours []int  `json:"hours"` // 24 compteurs de connexions
}

// hourlyStats — réponse GET /api/stats/hourly.
type hourlyStats struct {
	Timezone     string      `json:"timezone"`
	Days         int         `json:"days"`
	Rows         []hourlyRow `json:"rows"`         // oldest → today
	LoginsByHour []int       `json:"loginsByHour"` // total/heure sur la fenêtre (24)
	SalesByHour  []int       `json:"salesByHour"`  // CA/heure cumulé sur la fenêtre (24)
	MaxCell      int         `json:"maxCell"`
	TotalLogins  int         `json:"totalLogins"`
	TotalSales   int         `json:"totalSales"`
	PeakHour     int         `json:"peakHour"` // heure locale de pic de connexions (0-23)
	GeneratedAt  string      `json:"generatedAt"`
}

// handleStatsHourly — GET /api/stats/hourly?days=7|14|30&routerId=<id|all> :
// affluence réelle par tranche horaire (heatmap jour × heure) + CA et
// connexions par heure, agrégés dans le fuseau du compte. N°198 : le
// filtre site s'applique aux connexions ET au CA (vue site : direct du
// site + valeur gros des tickets réseau du site).
func (a *API) handleStatsHourly(w http.ResponseWriter, r *http.Request) {
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
	routerID := strings.TrimSpace(r.URL.Query().Get("routerId")) // "" ou "all" = tous les sites
	now := time.Now().UTC()
	a.store.Lock()
	db := a.store.Data()
	loc := accountTimezone(db, acc)
	nowLocal := now.In(loc)
	dayStart := time.Date(nowLocal.Year(), nowLocal.Month(), nowLocal.Day(), 0, 0, 0, 0, loc)
	windowStart := dayStart.AddDate(0, 0, -(days - 1))

	rows := make([]hourlyRow, days)
	for i := 0; i < days; i++ {
		d := windowStart.AddDate(0, 0, i)
		rows[i] = hourlyRow{Date: d.Format("2006-01-02"), Hours: make([]int, 24)}
	}
	rowIdx := make(map[string]int, days)
	for i := range rows {
		rowIdx[rows[i].Date] = i
	}
	loginsByHour := make([]int, 24)
	salesByHour := make([]int, 24)
	maxCell, totalLogins, totalSales := 0, 0, 0

	globalScope := routerID == "" || routerID == "all"
	for i := range db.UserLogs {
		l := &db.UserLogs[i]
		if l.AccountID != acc || l.Action != "login" {
			continue
		}
		if !globalScope && l.RouterID != routerID {
			continue
		}
		at, err := time.Parse(time.RFC3339, l.At)
		if err != nil {
			continue
		}
		al := at.In(loc)
		if al.Before(windowStart) {
			continue
		}
		h := al.Hour()
		loginsByHour[h]++
		totalLogins++
		if ri, ok := rowIdx[al.Format("2006-01-02")]; ok {
			rows[ri].Hours[h]++
			if rows[ri].Hours[h] > maxCell {
				maxCell = rows[ri].Hours[h]
			}
		}
	}
	// N°198 (constat C3) — CA horaire sur la doctrine « CONSOMMÉ » :
	// trésorerie réelle (direct consommé + revendeurs nets), plus JAMAIS
	// db.Sales qui journalise la GÉNÉRATION de stock.
	for _, e := range revenueEventsForScope(db, acc, routerID, windowStart) {
		al := e.At.In(loc)
		salesByHour[al.Hour()] += e.Amount
		totalSales += e.Amount
	}
	a.store.Unlock()

	peak := 0
	for h := 1; h < 24; h++ {
		if loginsByHour[h] > loginsByHour[peak] {
			peak = h
		}
	}

	writeJSON(w, http.StatusOK, hourlyStats{
		Timezone:     loc.String(),
		Days:         days,
		Rows:         rows,
		LoginsByHour: loginsByHour,
		SalesByHour:  salesByHour,
		MaxCell:      maxCell,
		TotalLogins:  totalLogins,
		TotalSales:   totalSales,
		PeakHour:     peak,
		GeneratedAt:  now.Format(time.RFC3339),
	})
}
