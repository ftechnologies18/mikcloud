// handlers_overview.go — N°198 : aperçu de PÉRIODE CALENDAIRE (jour /
// semaine / mois / année EN COURS) calculé au FUSEAU DU COMPTE, avec
// comparaison Δ% contre la période équivalente précédente AU MÊME MOMENT
// (même durée écoulée — un mardi 14 h ne se compare pas à un lundi entier).
//
// Réponse au constat C1 de l'audit « app/reports » : l'ancien sélecteur
// Jour/Semaine/Mois de la comptabilité réglait la TAILLE DES BUCKETS d'un
// graphe glissant (30 j / 12 semaines / 12 mois) — jamais la période
// courante. « Aujourd'hui » n'existait nulle part dans les rapports. Ces
// fenêtres calendaires deviennent les KPI de tête ; les vues glissantes
// restent en graphes secondaires (décision D4 de l'opérateur).
//
// Doctrine (inchangée — voir handlers_dashboard.go) :
//   - Revenus     = TRÉSORERIE RÉELLE : ventes directes consommées au prix
//     réellement payé + encaissements revendeurs nets (achats/versements −
//     retours). Vue site : direct du site + valeur gros des tickets réseau
//     du site (miroir buildAccounting).
//   - Ventes      = tickets ÉCOULÉS (remis au client ou consommés), tous
//     canaux.
//   - Panier moyen = prix MOYENEMENT PAYÉ par le client final (prix public
//     de chaque ticket écoulé) — et NON plus le mélange trésorerie/volume
//     de l'ancien revenue/sales (constat C6 : le gros encaissé des tickets
//     revendeurs gonflait artificiellement le panier affiché).
//   - Connexions  = logins réellement journalisés (UserLogs action=login).
//   - Volume de données (N°199) = agrégats JOURNALIERS persistés (une ligne
//     par compte, routeur et jour au fuseau du compte, alimentée en live par
//     les deltas read_state — les sessions fermées comptent enfin). La
//     fenêtre précédente coupe au MÊME MOMENT grâce à l'histogramme horaire
//     de la ligne frontière (heures 0..heure en cours) ; historique borné au
//     déploiement de l'accumulateur (décision D2 : pas de rétrofabrication)
//     — dataBytesPrev = 0 tant qu'aucune base n'existe, le badge Δ% reste
//     masqué plutôt que de comparer au vide.

package api

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"mikcloud/hotspot-api/internal/model"
)

// frenchWeekday — libellés courts des jours de la semaine (lundi → dimanche),
// même convention que frenchMonth (handlers_reports.go).
var frenchWeekday = [...]string{"lun.", "mar.", "mer.", "jeu.", "ven.", "sam.", "dim."}

// overviewKPIs — les cinq indicateurs de la période + leur base de
// comparaison (période précédente au même moment). N°199 : le volume de
// données a désormais sa base (agrégats journaliers) — la comparaison
// précédente est honnête ; elle vaut 0 (badge masqué) tant que
// l'accumulateur n'a pas d'historique (premier cycle post-déploiement).
type overviewKPIs struct {
	Sales         int   `json:"sales"`
	SalesPrev     int   `json:"salesPrev"`
	Revenue       int   `json:"revenue"`
	RevenuePrev   int   `json:"revenuePrev"`
	AvgTicket     int   `json:"avgTicket"`
	AvgTicketPrev int   `json:"avgTicketPrev"`
	Logins        int   `json:"logins"`
	LoginsPrev    int   `json:"loginsPrev"`
	DataBytes     int64 `json:"dataBytes"`
	DataBytesPrev int64 `json:"dataBytesPrev"`
}

// overviewPoint — un bucket de la série intrapériode (échelle adaptée à la
// période : heures pour le jour, jours pour la semaine et le mois, mois pour
// l'année — le dernier bucket est PARTIEL, période en cours).
type overviewPoint struct {
	Label   string `json:"label"`
	Revenue int    `json:"revenue"`
	Sales   int    `json:"sales"`
	Logins  int    `json:"logins"`
}

// handleStatsOverview — GET /api/stats/overview?period=day|week|month|year
// &routerId=<id|all> : KPI de la période calendaire EN COURS au fuseau du
// compte + fenêtre précédente équivalente (même durée écoulée).
func (a *API) handleStatsOverview(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	q := r.URL.Query()
	period := q.Get("period")
	if period == "" {
		period = "day"
	}
	if period != "day" && period != "week" && period != "month" && period != "year" {
		writeErr(w, http.StatusBadRequest, "period doit valoir day, week, month ou year")
		return
	}
	routerID := strings.TrimSpace(q.Get("routerId")) // "" ou "all" = tous les sites

	now := time.Now().UTC()
	a.store.Lock()
	db := a.store.Data()
	loc := accountTimezone(db, acc)
	nowLocal := now.In(loc)

	// Fenêtre courante — début de la période calendaire en cours, dans le
	// fuseau du compte (« aujourd'hui 00 h 00 », lundi en cours, 1ᵉʳ du mois,
	// 1ᵉʳ janvier).
	var curStart time.Time
	var prevY, prevM, prevD int
	switch period {
	case "day":
		curStart = time.Date(nowLocal.Year(), nowLocal.Month(), nowLocal.Day(), 0, 0, 0, 0, loc)
		prevD = -1
	case "week":
		wd := int(nowLocal.Weekday())
		if wd == 0 {
			wd = 7
		}
		curStart = time.Date(nowLocal.Year(), nowLocal.Month(), nowLocal.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, -(wd - 1))
		prevD = -7
	case "month":
		curStart = time.Date(nowLocal.Year(), nowLocal.Month(), 1, 0, 0, 0, 0, loc)
		prevM = -1
	case "year":
		curStart = time.Date(nowLocal.Year(), 1, 1, 0, 0, 0, 0, loc)
		prevY = -1
	}
	// Fenêtre précédente ÉQUIVALENTE : [début de la période précédente,
	// même instant relatif que maintenant]. Comparer « aujourd'hui à 14 h »
	// à « hier ENTIER » fausserait structurellement le Δ% (jour en cours
	// toujours en retard) — le même écoulé des deux côtés.
	prevStart := curStart.AddDate(prevY, prevM, prevD)
	prevEnd := prevStart.Add(now.Sub(curStart))

	// Série intrapériode — l'échelle suit la période.
	var bucketStarts []time.Time
	var labels []string
	switch period {
	case "day": // heures 00 h → heure en cours (partielle)
		for h := 0; h <= nowLocal.Hour(); h++ {
			bucketStarts = append(bucketStarts, curStart.Add(time.Duration(h)*time.Hour))
			labels = append(labels, fmt.Sprintf("%02dh", h))
		}
	case "week": // lundi → aujourd'hui (dernier jour partiel)
		wd := int(nowLocal.Weekday())
		if wd == 0 {
			wd = 7
		}
		for i := 0; i < wd; i++ {
			bucketStarts = append(bucketStarts, curStart.AddDate(0, 0, i))
			labels = append(labels, frenchWeekday[i])
		}
	case "month": // 1ᵉʳ → aujourd'hui (dernier jour partiel)
		for d := 1; d <= nowLocal.Day(); d++ {
			bucketStarts = append(bucketStarts, time.Date(nowLocal.Year(), nowLocal.Month(), d, 0, 0, 0, 0, loc))
			labels = append(labels, strconv.Itoa(d))
		}
	case "year": // janvier → mois en cours (dernier mois partiel)
		for m := 1; m <= int(nowLocal.Month()); m++ {
			bucketStarts = append(bucketStarts, time.Date(nowLocal.Year(), time.Month(m), 1, 0, 0, 0, 0, loc))
			labels = append(labels, frenchMonth[m-1])
		}
	}
	series := make([]overviewPoint, len(labels))
	for i := range labels {
		series[i] = overviewPoint{Label: labels[i]}
	}
	bucketIndex := func(at time.Time) int {
		return sort.Search(len(bucketStarts), func(i int) bool { return at.Before(bucketStarts[i]) }) - 1
	}

	globalScope := routerID == "" || routerID == "all"
	kpis := overviewKPIs{}

	// TRÉSORERIE RÉELLE bornée au filtre site (vue globale : direct +
	// revendeurs nets ; vue site : direct du site + valeur gros des tickets
	// réseau du site). Les événements de la zone morte [prevEnd, curStart)
	// — le reste de la période précédente après l'instant équivalent — ne
	// comptent pour AUCUNE fenêtre.
	for _, e := range revenueEventsForScope(db, acc, routerID, prevStart) {
		if e.At.Before(curStart) {
			if e.At.Before(prevEnd) {
				kpis.RevenuePrev += e.Amount
			}
			continue
		}
		kpis.Revenue += e.Amount
		if idx := bucketIndex(e.At); idx >= 0 && idx < len(series) {
			series[idx].Revenue += e.Amount
		}
	}

	// ÉCOULEMENTS — volume de tickets + prix réellement payé (panier moyen).
	var publicPaid, prevPublic int
	for _, v := range collectSoldVouchers(db, acc, prevStart) {
		if !globalScope && v.RouterID != routerID {
			continue
		}
		if v.At.Before(curStart) {
			if v.At.Before(prevEnd) {
				kpis.SalesPrev++
				prevPublic += v.Public
			}
			continue
		}
		kpis.Sales++
		publicPaid += v.Public
		if idx := bucketIndex(v.At); idx >= 0 && idx < len(series) {
			series[idx].Sales++
		}
	}
	if kpis.Sales > 0 {
		kpis.AvgTicket = publicPaid / kpis.Sales
	}
	if kpis.SalesPrev > 0 {
		kpis.AvgTicketPrev = prevPublic / kpis.SalesPrev
	}

	// CONNEXIONS — logins journalisés (seule source honnête ; le plafond
	// de rétention du journal borne naturellement l'historique lointain).
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
		if at.Before(curStart) {
			// Fenêtre précédente : [prevStart, prevEnd) — la zone
			// morte [prevEnd, curStart) ne compte nulle part.
			if !at.Before(prevStart) && at.Before(prevEnd) {
				kpis.LoginsPrev++
			}
			continue
		}
		kpis.Logins++
		if idx := bucketIndex(at); idx >= 0 && idx < len(series) {
			series[idx].Logins++
		}
	}

	// VOLUME DE DONNÉES (N°199) — agrégats JOURNALIERS persistés (une ligne
	// par compte, routeur, jour au fuseau du compte). Fenêtre courante :
	// lignes du jour de début à aujourd'hui (la ligne du jour est partielle
	// PAR NATURE — l'accumulation est live). Fenêtre précédente « au même
	// moment » : lignes pleines + histogramme horaire 0..heure en cours de la
	// ligne frontière (les deux fenêtres partagent la même plage d'heures ;
	// seule la dernière heure est complète côté précédent — biais < 1 h).
	// Les jours de la zone morte ne comptent nulle part.
	curDayKey := curStart.Format("2006-01-02")
	todayKey := nowLocal.Format("2006-01-02")
	prevStartKey := prevStart.Format("2006-01-02")
	prevBoundaryKey := prevEnd.Format("2006-01-02")
	for i := range db.VolumeDays {
		v := &db.VolumeDays[i]
		if v.AccountID != acc {
			continue
		}
		if !globalScope && v.RouterID != routerID {
			continue
		}
		switch {
		case v.Day >= curDayKey && v.Day <= todayKey:
			kpis.DataBytes += v.BytesIn + v.BytesOut
		case v.Day >= prevStartKey && v.Day <= prevBoundaryKey:
			if v.Day == prevBoundaryKey {
				kpis.DataBytesPrev += model.VolumeHoursSumThrough(v.Hours, nowLocal.Hour())
			} else {
				kpis.DataBytesPrev += v.BytesIn + v.BytesOut
			}
		}
	}
	a.store.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"period":   period,
		"routerId": routerID,
		"timezone": loc.String(),
		"window": map[string]string{
			"start":     curStart.Format(time.RFC3339),
			"end":       now.Format(time.RFC3339),
			"prevStart": prevStart.Format(time.RFC3339),
			"prevEnd":   prevEnd.Format(time.RFC3339),
		},
		"kpis":        kpis,
		"series":      series,
		"generatedAt": now.Format(time.RFC3339),
	})
}

// revenueEventsForScope — événements de TRÉSORERIE bornés au filtre site,
// source unique de la doctrine « consommé » pour les agrégats de revenus
// (comptabilité, aperçu de période, affluence horaire) :
//   - vue globale (routerID vide ou "all") : ventes directes consommées au
//     prix payé + encaissements revendeurs nets (achats/versements −
//     retours) — les transactions ne sont liées à aucun site ;
//   - vue site : ventes directes consommées DU site + valeur gros des
//     tickets réseau du site (proxy honnête — l'encaissement réel des
//     achats de stock n'est pas attribuable à un site). Les événements gros
//     portent le nom du revendeur : les répartitions par canal restent
//     justes (revendeur ≠ direct).
//
// L'appelant tient le verrou du store.
func revenueEventsForScope(db *model.DB, acc, routerID string, since time.Time) []saleEvent {
	if routerID == "" || routerID == "all" {
		return collectSaleEvents(db, acc, since)
	}
	out := []saleEvent{}
	for _, e := range collectSaleEvents(db, acc, since) {
		if e.Reseller == "" && e.RouterID == routerID {
			out = append(out, e)
		}
	}
	for _, v := range collectSoldVouchers(db, acc, since) {
		if v.ResellerName == "" || v.RouterID != routerID {
			continue
		}
		out = append(out, saleEvent{
			At: v.At, Amount: v.Cost, Selling: v.Public, Cost: v.Cost,
			RouterID: v.RouterID, Profile: v.Profile, Reseller: v.ResellerName,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })
	return out
}
