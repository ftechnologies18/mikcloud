package api

// Tests N°198 — aperçu de PÉRIODE CALENDAIRE (GET /api/stats/overview) et
// corrections de cohérence des rapports :
//   - fenêtres Jour/Semaine/Mois/Année au FUSEAU DU COMPTE (fini le sélecteur
//     qui ne réglait que la taille des buckets d'un graphe glissant) ;
//   - comparaison Δ% contre la période précédente AU MÊME MOMENT (même durée
//     écoulée) — la zone morte entre les deux fenêtres ne compte nulle part ;
//   - panier moyen = PRIX RÉELLEMENT PAYÉ (fini le mélange trésorerie/volume) ;
//   - CA horaire sur la doctrine « CONSOMMÉ » (plus JAMAIS db.Sales = génération
//     de stock) ;
//   - filtre site étendu à l'aperçu, à l'activité (connexions, sessions, parc,
//     marge) et au CA horaire.
//
// Déterminisme : les événements sont semés à des positions RELATIVES aux
// frontières de fenêtre recalculées côté test (mi-fenêtre, zone morte) — les
// assertions tiennent à n'importe quelle heure d'exécution.

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"mikcloud/hotspot-api/internal/auth"
	"mikcloud/hotspot-api/internal/model"
)

// storeIface — surface du store utilisée par les helpers de semis (satisfait
// par *store.Store, sans l'importer ici).
type storeIface interface {
	Lock()
	Unlock()
	Data() *model.DB
	Save()
}

// setAccountTimezone — impose le fuseau du compte (Tenant.Timezone).
func setAccountTimezone(t *testing.T, st storeIface, accID, tz string) {
	t.Helper()
	st.Lock()
	s := st.Data().SettingsByAccount[accID]
	s.Tenant.Timezone = tz
	st.Data().SettingsByAccount[accID] = s
	st.Save()
	st.Unlock()
}

// dayWindows — frontières de la période JOUR au fuseau donné, recalculées
// comme le serveur : [minuit local, maintenant] + fenêtre précédente au même
// moment + le milieu de la zone morte [prevEnd, curStart).
func dayWindows(tz string) (curStart, prevStart, prevEnd, mid, prevMid, deadMid, now time.Time) {
	loc, _ := time.LoadLocation(tz)
	now = time.Now().UTC()
	nowLocal := now.In(loc)
	curStart = time.Date(nowLocal.Year(), nowLocal.Month(), nowLocal.Day(), 0, 0, 0, 0, loc)
	prevStart = curStart.AddDate(0, 0, -1)
	prevEnd = prevStart.Add(now.Sub(curStart))
	mid = curStart.Add(now.Sub(curStart) / 2)
	prevMid = prevStart.Add(now.Sub(curStart) / 2)
	deadMid = prevEnd.Add(curStart.Sub(prevEnd) / 2)
	return
}

// seedVoucher — voucher ÉCOULÉ (première connexion) à l'instant donné.
func seedVoucher(t *testing.T, st storeIface, id, accID, routerID string, price, sellingPrice int, usedAt time.Time, resellerID, resellerName string) {
	t.Helper()
	st.Lock()
	st.Data().HotspotUsers = append(st.Data().HotspotUsers, model.HotspotUser{
		ID: id, AccountID: accID, Kind: "voucher", Username: id,
		ProfileName: "1 Heure", RouterID: routerID, RouterName: "Site " + routerID,
		Status: "used", ResellerID: resellerID, ResellerName: resellerName,
		CreatedAt: usedAt.Add(-time.Hour).Format(time.RFC3339),
		ExpiresAt: usedAt.Add(7 * 24 * time.Hour).Format(time.RFC3339),
		Price:     price, SellingPrice: sellingPrice,
		UsedAt: usedAt.Format(time.RFC3339),
	})
	st.Save()
	st.Unlock()
}

// seedLogin — entrée login du journal à l'instant donné.
func seedLogin(t *testing.T, st storeIface, id, accID, routerID string, at time.Time) {
	t.Helper()
	st.Lock()
	st.Data().UserLogs = append(st.Data().UserLogs, model.UserLog{
		ID: id, AccountID: accID, Action: "login", RouterID: routerID, At: at.Format(time.RFC3339),
	})
	st.Save()
	st.Unlock()
}

// seedSession — session VIVANTE ouverte à l'instant donné.
func seedSession(t *testing.T, st storeIface, id, accID, routerID string, startedAt time.Time, bytesIn, bytesOut int64) {
	t.Helper()
	st.Lock()
	st.Data().Sessions = append(st.Data().Sessions, model.Session{
		ID: id, AccountID: accID, RouterID: routerID, StartedAt: startedAt.Format(time.RFC3339),
		BytesIn: bytesIn, BytesOut: bytesOut,
	})
	st.Save()
	st.Unlock()
}

// seedTreasury — transaction d'encaissement revendeur (trésorerie réelle).
func seedTreasury(t *testing.T, st storeIface, id, accID, resellerName string, amount int, at time.Time) {
	t.Helper()
	st.Lock()
	st.Data().Transactions = append(st.Data().Transactions, model.Transaction{
		ID: id, AccountID: accID, Type: "sale", ResellerName: resellerName,
		Amount: amount, At: at.Format(time.RFC3339),
	})
	st.Save()
	st.Unlock()
}

// overviewKPIsOf — décode le bloc kpis de GET /api/stats/overview.
func overviewKPIsOf(t *testing.T, out map[string]any) map[string]any {
	t.Helper()
	kpis, ok := out["kpis"].(map[string]any)
	if !ok {
		t.Fatalf("bloc kpis absent de la réponse : %v", out)
	}
	return kpis
}

func kpiInt(t *testing.T, kpis map[string]any, key string) int {
	t.Helper()
	v, ok := kpis[key].(float64)
	if !ok {
		t.Fatalf("KPI %s absent ou non numérique : %v", key, kpis[key])
	}
	return int(v)
}

// TestStatsOverviewDayWindows — fenêtres calendaires au FUSEAU DU COMPTE :
// avec America/New_York, window.start = minuit NEW-YORKAIS (04:00/05:00 UTC
// selon l'heure d'été), PAS minuit UTC. Les événements de la zone morte
// [prevEnd, curStart) ne comptent pour AUCUNE fenêtre.
func TestStatsOverviewDayWindows(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	token, accID, _ := registerAccount(t, ts, "ov-tz-owner", "")
	setAccountTimezone(t, st, accID, "America/New_York")

	curStart, prevStart, prevEnd, mid, prevMid, deadMid, _ := dayWindows("America/New_York")
	seedVoucher(t, st, "ov-tz-cur", accID, "rt-a", 300, 500, mid, "", "")
	seedVoucher(t, st, "ov-tz-prev", accID, "rt-a", 300, 400, prevMid, "", "")
	seedVoucher(t, st, "ov-tz-dead", accID, "rt-a", 300, 700, deadMid, "", "")

	status, out := doJSON(t, ts, "GET", "/api/stats/overview?period=day", token, nil)
	if status != http.StatusOK {
		t.Fatalf("overview day : statut %d, corps %v", status, out)
	}
	window, _ := out["window"].(map[string]any)
	if window["start"] != curStart.Format(time.RFC3339) {
		t.Fatalf("window.start = %v, attendu minuit local NY %s", window["start"], curStart.Format(time.RFC3339))
	}
	if window["prevStart"] != prevStart.Format(time.RFC3339) {
		t.Fatalf("window.prevStart = %v, attendu %s", window["prevStart"], prevStart.Format(time.RFC3339))
	}
	if window["prevEnd"] != prevEnd.Format(time.RFC3339) {
		t.Fatalf("window.prevEnd = %v, attendu %s", window["prevEnd"], prevEnd.Format(time.RFC3339))
	}
	if tz, _ := out["timezone"].(string); tz != "America/New_York" {
		t.Fatalf("timezone = %q, attendu America/New_York", tz)
	}

	kpis := overviewKPIsOf(t, out)
	if n := kpiInt(t, kpis, "sales"); n != 1 {
		t.Fatalf("sales = %d, attendu 1 (seul le voucher de la fenêtre courante)", n)
	}
	if n := kpiInt(t, kpis, "salesPrev"); n != 1 {
		t.Fatalf("salesPrev = %d, attendu 1 (fenêtre précédente au même moment)", n)
	}
	if n := kpiInt(t, kpis, "revenue"); n != 500 {
		t.Fatalf("revenue = %d, attendu 500", n)
	}
	if n := kpiInt(t, kpis, "revenuePrev"); n != 400 {
		t.Fatalf("revenuePrev = %d, attendu 400", n)
	}
}

// TestStatsOverviewWeekMonthYearWindows — débuts de période : lundi pour la
// semaine, 1ᵉʳ du mois, 1ᵉʳ janvier — au fuseau du compte.
func TestStatsOverviewWeekMonthYearWindows(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	token, accID, _ := registerAccount(t, ts, "ov-cal-owner", "")
	setAccountTimezone(t, st, accID, "Africa/Abidjan")

	loc, _ := time.LoadLocation("Africa/Abidjan")
	now := time.Now().UTC().In(loc)

	wd := int(now.Weekday())
	if wd == 0 {
		wd = 7
	}
	monday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, -(wd - 1))
	firstOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
	janFirst := time.Date(now.Year(), 1, 1, 0, 0, 0, 0, loc)

	for _, c := range []struct {
		period string
		want   time.Time
	}{
		{"week", monday},
		{"month", firstOfMonth},
		{"year", janFirst},
	} {
		status, out := doJSON(t, ts, "GET", "/api/stats/overview?period="+c.period, token, nil)
		if status != http.StatusOK {
			t.Fatalf("overview %s : statut %d, corps %v", c.period, status, out)
		}
		window, _ := out["window"].(map[string]any)
		if window["start"] != c.want.Format(time.RFC3339) {
			t.Fatalf("period %s : window.start = %v, attendu %s", c.period, window["start"], c.want.Format(time.RFC3339))
		}
		// Série intrapériode — l'échelle suit la période.
		series, _ := out["series"].([]any)
		wantLen := 0
		switch c.period {
		case "week":
			wantLen = wd
		case "month":
			wantLen = now.Day()
		case "year":
			wantLen = int(now.Month())
		}
		if len(series) != wantLen {
			t.Fatalf("period %s : série de %d points, attendu %d (jusqu'à la période en cours)", c.period, len(series), wantLen)
		}
	}
}

// TestStatsOverviewKPIs — doctrine complète : trésorerie réelle (direct
// consommé + encaissements revendeurs), panier moyen = PRIX PAYÉ (pas le
// mélange trésorerie/volume), connexions depuis le journal, volume de données
// des sessions vivantes.
func TestStatsOverviewKPIs(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	token, accID, _ := registerAccount(t, ts, "ov-kpi-owner", "")

	_, _, _, mid, prevMid, deadMid, _ := dayWindows("UTC")

	// Fenêtre courante : direct 500 payé + réseau écoulé 800 public/300 gros
	// + encaissement revendeur 2 000.
	seedVoucher(t, st, "ov-kpi-direct", accID, "rt-a", 300, 500, mid, "", "")
	seedVoucher(t, st, "ov-kpi-reseau", accID, "rt-a", 300, 800, mid, "res-awa", "Awa Koné")
	seedTreasury(t, st, "ov-kpi-tx", accID, "Awa Koné", 2000, mid)
	// Fenêtre précédente : un direct à 400.
	seedVoucher(t, st, "ov-kpi-prev", accID, "rt-a", 300, 400, prevMid, "", "")
	// Zone morte : ne compte NULLE PART.
	seedVoucher(t, st, "ov-kpi-dead", accID, "rt-a", 300, 700, deadMid, "", "")
	seedTreasury(t, st, "ov-kpi-deadtx", accID, "Awa Koné", 9999, deadMid)

	seedLogin(t, st, "ov-kpi-l1", accID, "rt-a", mid)
	seedLogin(t, st, "ov-kpi-l2", accID, "rt-a", mid.Add(time.Minute))
	seedLogin(t, st, "ov-kpi-l3", accID, "rt-a", prevMid)
	seedLogin(t, st, "ov-kpi-l4", accID, "rt-a", prevMid.Add(time.Minute))
	seedLogin(t, st, "ov-kpi-l5", accID, "rt-a", prevMid.Add(2*time.Minute))
	seedLogin(t, st, "ov-kpi-l6", accID, "rt-a", deadMid)

	// N°199 — le volume se lit dans les AGRÉGATS JOURNALIERS (les sessions
	// fermées quittent le store) : jour courant 3 Mo, veille à histogramme
	// connu (la fenêtre précédente coupe à l'heure en cours).
	nowUTC := time.Now().UTC()
	seedVolumeDay(t, st, "ov-kpi-vd-cur", accID, "rt-a", nowUTC.Format("2006-01-02"), 1_000_000, 2_000_000, "")
	seedVolumeDay(t, st, "ov-kpi-vd-prev", accID, "rt-a", nowUTC.AddDate(0, 0, -1).Format("2006-01-02"), 60_000_000, 0, volHours())

	status, out := doJSON(t, ts, "GET", "/api/stats/overview?period=day", token, nil)
	if status != http.StatusOK {
		t.Fatalf("overview : statut %d, corps %v", status, out)
	}
	kpis := overviewKPIsOf(t, out)
	if n := kpiInt(t, kpis, "sales"); n != 2 {
		t.Fatalf("sales = %d, attendu 2 (direct + réseau, la zone morte exclue)", n)
	}
	if n := kpiInt(t, kpis, "revenue"); n != 2500 {
		t.Fatalf("revenue = %d, attendu 2500 (500 direct + 2000 encaissement réseau ; le gros du ticket réseau n'est PAS re-compté en vue globale)", n)
	}
	// Le cœur du constat C6 : panier moyen = (500+800)/2 = 650 — et NON
	// revenue/sales = 1250 (mélange trésorerie/volume de l'ancienne formule).
	if n := kpiInt(t, kpis, "avgTicket"); n != 650 {
		t.Fatalf("avgTicket = %d, attendu 650 (prix réellement payé)", n)
	}
	if n := kpiInt(t, kpis, "salesPrev"); n != 1 {
		t.Fatalf("salesPrev = %d, attendu 1", n)
	}
	if n := kpiInt(t, kpis, "revenuePrev"); n != 400 {
		t.Fatalf("revenuePrev = %d, attendu 400", n)
	}
	if n := kpiInt(t, kpis, "avgTicketPrev"); n != 400 {
		t.Fatalf("avgTicketPrev = %d, attendu 400", n)
	}
	if n := kpiInt(t, kpis, "logins"); n != 2 {
		t.Fatalf("logins = %d, attendu 2", n)
	}
	if n := kpiInt(t, kpis, "loginsPrev"); n != 3 {
		t.Fatalf("loginsPrev = %d, attendu 3", n)
	}
	if n := kpiInt(t, kpis, "dataBytes"); n != 3_000_000 {
		t.Fatalf("dataBytes = %d, attendu 3000000 (ligne du jour — agrégats N°199)", n)
	}
	if n := kpiInt(t, kpis, "dataBytesPrev"); n != int(volHoursSum(nowUTC.Hour())) {
		t.Fatalf("dataBytesPrev = %d, attendu %d (histogramme d'hier coupé à l'heure en cours)", n, volHoursSum(nowUTC.Hour()))
	}
}

// TestStatsOverviewSiteFilter — vue site : direct du site + valeur GROS des
// tickets réseau du site ; les transactions (sans site) et l'autre site sont
// exclus ; connexions et sessions suivent le même filtre.
func TestStatsOverviewSiteFilter(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	token, accID, _ := registerAccount(t, ts, "ov-site-owner", "")

	_, _, _, mid, _, _, _ := dayWindows("UTC")
	seedVoucher(t, st, "ov-site-direct-a", accID, "rt-a", 300, 500, mid, "", "")
	seedVoucher(t, st, "ov-site-reseau-a", accID, "rt-a", 300, 800, mid, "res-awa", "Awa Koné")
	seedVoucher(t, st, "ov-site-direct-b", accID, "rt-b", 300, 900, mid, "", "")
	seedTreasury(t, st, "ov-site-tx", accID, "Awa Koné", 2000, mid) // sans site → vue site exclue
	seedLogin(t, st, "ov-site-la", accID, "rt-a", mid)
	seedLogin(t, st, "ov-site-lb1", accID, "rt-b", mid)
	seedLogin(t, st, "ov-site-lb2", accID, "rt-b", mid.Add(time.Minute))
	// N°199 — volume : lignes journalières bornées au filtre site.
	seedVolumeDay(t, st, "ov-site-vda", accID, "rt-a", time.Now().UTC().Format("2006-01-02"), 1_000_000, 0, "")
	seedVolumeDay(t, st, "ov-site-vdb", accID, "rt-b", time.Now().UTC().Format("2006-01-02"), 5_000_000, 0, "")

	status, out := doJSON(t, ts, "GET", "/api/stats/overview?period=day&routerId=rt-a", token, nil)
	if status != http.StatusOK {
		t.Fatalf("overview site : statut %d, corps %v", status, out)
	}
	kpis := overviewKPIsOf(t, out)
	if n := kpiInt(t, kpis, "sales"); n != 2 {
		t.Fatalf("sales = %d, attendu 2 (les 2 tickets du site A)", n)
	}
	if n := kpiInt(t, kpis, "revenue"); n != 800 {
		t.Fatalf("revenue = %d, attendu 800 (500 direct + 300 gros du réseau du site)", n)
	}
	if n := kpiInt(t, kpis, "logins"); n != 1 {
		t.Fatalf("logins = %d, attendu 1 (site A uniquement)", n)
	}
	if n := kpiInt(t, kpis, "dataBytes"); n != 1_000_000 {
		t.Fatalf("dataBytes = %d, attendu 1000000 (session du site A)", n)
	}
}

// TestStatsOverviewValidationRBAC — période inconnue → 400 ; le rôle
// revendeur (Mode Vente) n'a pas accès aux rapports → 403.
func TestStatsOverviewValidationRBAC(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	token, accID, _ := registerAccount(t, ts, "ov-rbac-owner", "")

	if status, _ := doJSON(t, ts, "GET", "/api/stats/overview?period=quarter", token, nil); status != http.StatusBadRequest {
		t.Fatalf("period=quarter : statut %d, attendu 400", status)
	}

	seedSellReseller(t, st, "usr-ov-r", accID, "Revendeur OV", "prepaid", 0)
	resellerToken := auth.Sign(testJWTSecret, auth.NewClaims("usr-ov-r", "R", "reseller", accID, 0))
	if status, _ := doJSON(t, ts, "GET", "/api/stats/overview?period=day", resellerToken, nil); status != http.StatusForbidden {
		t.Fatalf("revendeur : statut %d, attendu 403", status)
	}
}

// TestStatsHourlyConsumedDoctrine — le CA horaire ne compte PLUS la génération
// de stock (db.Sales) : uniquement la trésorerie consommée. Le filtre site
// s'applique aussi.
func TestStatsHourlyConsumedDoctrine(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	token, accID, _ := registerAccount(t, ts, "hourly-owner", "")

	_, _, _, mid, _, _, _ := dayWindows("UTC")
	seedVoucher(t, st, "hourly-direct-a", accID, "rt-a", 300, 500, mid, "", "")
	seedVoucher(t, st, "hourly-direct-b", accID, "rt-b", 300, 700, mid, "", "")

	// Génération de stock : 9 999 F de tickets CRÉÉS — ne doit PAS compter
	// comme chiffre d'affaires.
	st.Lock()
	st.Data().Sales = append(st.Data().Sales, model.Sale{
		ID: "hourly-gen", AccountID: accID, Amount: 9999, At: mid.Format(time.RFC3339),
	})
	st.Save()
	st.Unlock()

	status, out := doJSON(t, ts, "GET", "/api/stats/hourly?days=7", token, nil)
	if status != http.StatusOK {
		t.Fatalf("hourly : statut %d, corps %v", status, out)
	}
	if n := int(out["totalSales"].(float64)); n != 1200 {
		t.Fatalf("totalSales = %d, attendu 1200 (500 + 700 consommés — la génération de 9999 est exclue)", n)
	}

	status, out = doJSON(t, ts, "GET", "/api/stats/hourly?days=7&routerId=rt-a", token, nil)
	if status != http.StatusOK {
		t.Fatalf("hourly site : statut %d, corps %v", status, out)
	}
	if n := int(out["totalSales"].(float64)); n != 500 {
		t.Fatalf("totalSales site = %d, attendu 500 (site A uniquement)", n)
	}
}

// TestAccountingAvgTicketPaidPrice — comptabilité : panier moyen = prix payé
// (Σ public / ventes), pas revenue/sales.
func TestAccountingAvgTicketPaidPrice(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	token, accID, _ := registerAccount(t, ts, "acc-avg-owner", "")

	now := time.Now().UTC()
	seedVoucher(t, st, "acc-avg-direct", accID, "rt-a", 300, 500, now.Add(-2*time.Hour), "", "")
	seedVoucher(t, st, "acc-avg-reseau", accID, "rt-a", 500, 800, now.Add(-3*time.Hour), "res-awa", "Awa Koné")
	seedTreasury(t, st, "acc-avg-tx", accID, "Awa Koné", 2000, now.Add(-1*time.Hour))

	status, out := doJSON(t, ts, "GET", "/api/accounting?period=day", token, nil)
	if status != http.StatusOK {
		t.Fatalf("accounting : statut %d, corps %v", status, out)
	}
	totals, _ := out["totals"].(map[string]any)
	if n := int(totals["sales"].(float64)); n != 2 {
		t.Fatalf("sales = %d, attendu 2", n)
	}
	if n := int(totals["revenue"].(float64)); n != 2500 {
		t.Fatalf("revenue = %d, attendu 2500", n)
	}
	if n := int(totals["avgTicket"].(float64)); n != 650 {
		t.Fatalf("avgTicket = %d, attendu 650 (prix payé (500+800)/2, pas revenue/sales)", n)
	}
}

// TestReportsSiteFilter — le filtre site borne TOUT l'onglet Activité :
// écoulements, connexions, sessions, parc de vouchers et bloc marge.
func TestReportsSiteFilter(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	token, accID, _ := registerAccount(t, ts, "rep-site-owner", "")

	_, _, _, mid, _, _, _ := dayWindows("UTC")
	seedVoucher(t, st, "rep-site-direct-a", accID, "rt-a", 300, 500, mid, "", "")
	seedVoucher(t, st, "rep-site-reseau-a", accID, "rt-a", 300, 800, mid, "res-awa", "Awa Koné")
	seedVoucher(t, st, "rep-site-direct-b", accID, "rt-b", 300, 900, mid, "", "")
	seedTreasury(t, st, "rep-site-tx", accID, "Awa Koné", 2000, mid)
	seedLogin(t, st, "rep-site-la", accID, "rt-a", mid)
	seedLogin(t, st, "rep-site-lb", accID, "rt-b", mid)
	seedSession(t, st, "rep-site-sa", accID, "rt-a", mid, 1_000_000, 0)
	seedSession(t, st, "rep-site-sb", accID, "rt-b", mid, 5_000_000, 0)

	status, out := doJSON(t, ts, "GET", "/api/reports?days=7&routerId=rt-a", token, nil)
	if status != http.StatusOK {
		t.Fatalf("reports site : statut %d, corps %v", status, out)
	}
	totals, _ := out["totals"].(map[string]any)
	if n := int(totals["sales"].(float64)); n != 2 {
		t.Fatalf("sales = %d, attendu 2 (site A)", n)
	}
	if n := int(totals["revenue"].(float64)); n != 800 {
		t.Fatalf("revenue = %d, attendu 800 (500 direct + 300 gros réseau du site)", n)
	}
	if n := int(totals["avgTicket"].(float64)); n != 650 {
		t.Fatalf("avgTicket = %d, attendu 650 (prix payé)", n)
	}
	// Connexions du site A uniquement.
	loginsByDay, _ := out["loginsByDay"].([]any)
	sumLogins := 0
	for _, raw := range loginsByDay {
		p, _ := raw.(map[string]any)
		sumLogins += int(p["count"].(float64))
	}
	if sumLogins != 1 {
		t.Fatalf("Σ logins = %d, attendu 1 (site A)", sumLogins)
	}
	// Sessions du site A uniquement.
	sessions, _ := out["sessions"].(map[string]any)
	if n := int(sessions["count"].(float64)); n != 1 {
		t.Fatalf("sessions.count = %d, attendu 1 (site A)", n)
	}
	// Parc de vouchers borné au site A.
	voucherStatus, _ := out["voucherStatus"].(map[string]any)
	if n := int(voucherStatus["used"].(float64)); n != 2 {
		t.Fatalf("voucherStatus.used = %d, attendu 2 (site A)", n)
	}
	// Bloc marge borné au site A : CA public 1300, coût 600, marge 700.
	margin, _ := out["margin"].(map[string]any)
	if n := int(margin["revenue"].(float64)); n != 1300 {
		t.Fatalf("margin.revenue = %d, attendu 1300 (500 + 800 public du site A)", n)
	}
	if n := int(margin["cost"].(float64)); n != 600 {
		t.Fatalf("margin.cost = %d, attendu 600 (300 + 300 gros du site A)", n)
	}
	if n := int(margin["margin"].(float64)); n != 700 {
		t.Fatalf("margin.margin = %d, attendu 700", n)
	}
}

// TestOverviewSeriesHourlyBuckets — la série du jour est découpée en heures
// (00 h → heure en cours) et les événements tombent dans LEUR bucket.
func TestOverviewSeriesHourlyBuckets(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	token, accID, _ := registerAccount(t, ts, "ov-series-owner", "")

	curStart, _, _, mid, _, _, _ := dayWindows("UTC")
	_ = curStart
	seedVoucher(t, st, "ov-series-v", accID, "rt-a", 300, 500, mid, "", "")
	seedLogin(t, st, "ov-series-l", accID, "rt-a", mid)

	status, out := doJSON(t, ts, "GET", "/api/stats/overview?period=day", token, nil)
	if status != http.StatusOK {
		t.Fatalf("overview : statut %d, corps %v", status, out)
	}
	series, _ := out["series"].([]any)
	nowLocal := time.Now().UTC()
	if len(series) != nowLocal.Hour()+1 {
		t.Fatalf("série de %d points, attendu %d (00 h → heure en cours)", len(series), nowLocal.Hour()+1)
	}
	wantIdx := mid.Hour()
	for i, raw := range series {
		p, _ := raw.(map[string]any)
		label, _ := p["label"].(string)
		if label != fmt.Sprintf("%02dh", i) {
			t.Fatalf("point %d : label %q, attendu %q", i, label, fmt.Sprintf("%02dh", i))
		}
		if i == wantIdx {
			if n := int(p["revenue"].(float64)); n != 500 {
				t.Fatalf("bucket %02dh : revenue = %d, attendu 500", i, n)
			}
			if n := int(p["sales"].(float64)); n != 1 {
				t.Fatalf("bucket %02dh : sales = %d, attendu 1", i, n)
			}
			if n := int(p["logins"].(float64)); n != 1 {
				t.Fatalf("bucket %02dh : logins = %d, attendu 1", i, n)
			}
		}
	}
	_ = curStart
}
