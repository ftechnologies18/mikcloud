package api

// Tests N°200 — journaux MENSUELS GELÉS (décision D3) :
//   - buildMonthlyJournal : les cinq KPI de l'aperçu (doctrine N°198) + la
//     répartition par canal, figés sur la fenêtre du mois, volume borné par
//     le dernier jour couvert (la ligne du jour d'une clôture manuelle est
//     partielle et doit pourtant compter) ;
//   - ensureMonthlyJournals : gel AUTOMATIQUE du mois précédent au bascule,
//     idempotent, multi-comptes, ne touche JAMAIS un mois déjà gelé
//     manuellement, au FUSEAU DU COMPTE ;
//   - endpoints : liste (rattrapage paresseux + tri descendant + état du
//     mois courant), clôture manuelle (201 → partial, 409 au doublon),
//     RBAC revendeur 403, export CSV Excel FR ;
//   - balayage horaire : RunRetentionSweep gèle aussi (comptes dormants).

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"mikcloud/hotspot-api/internal/auth"
	"mikcloud/hotspot-api/internal/model"
)

// prevMonthOf — frontières du mois PRÉCÉDENT un mois calendaire avant `now`,
// au fuseau donné (même dérivation que le serveur : depuis le 1ᵉʳ du mois
// courant, jamais AddDate(0,-1,0) sur un 29/30/31 qui déborderait).
func prevMonthOf(now time.Time, loc *time.Location) (prevStart, curStart time.Time) {
	nowLocal := now.In(loc)
	curStart = time.Date(nowLocal.Year(), nowLocal.Month(), 1, 0, 0, 0, 0, loc)
	return curStart.AddDate(0, -1, 0), curStart
}

// TestBuildMonthlyJournalKPIs — horloge FIXE (octobre 2026, compte à
// Africa/Abidjan) : les KPI figés valent exactement les événements semés —
// doctrine N°198 (trésorerie réelle, écoulés, panier payé, logins, agrégats
// journaliers) + répartition par canal + fenêtre couverte (auto = mois
// plein, manuel = partiel borné au clic).
func TestBuildMonthlyJournalKPIs(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	_, accID, _ := registerAccount(t, ts, "mj-kpi-owner", "")
	setAccountTimezone(t, st, accID, "Africa/Abidjan")
	loc, _ := time.LoadLocation("Africa/Abidjan")
	oct := func(day, hour int) time.Time {
		return time.Date(2026, 10, day, hour, 30, 0, 0, loc)
	}

	// Écoulements : 2 directs (700 et 500 payés) + 1 réseau (public 300).
	seedVoucher(t, st, "mj-v-a", accID, "rt-a", 500, 700, oct(5, 10), "", "")
	seedVoucher(t, st, "mj-v-b", accID, "rt-a", 500, 0, oct(12, 11), "", "")
	seedVoucher(t, st, "mj-v-r", accID, "rt-a", 300, 0, oct(20, 9), "res-1", "Awa Koné")
	// Hors fenêtre manuelle (après le 15) : comptés AUTO, pas MANUEL.
	seedVoucher(t, st, "mj-v-tard", accID, "rt-a", 500, 0, oct(28, 20), "", "")
	// Trésorerie réseau : encaissement net 900 (le 8).
	seedTreasury(t, st, "mj-tx", accID, "Awa Koné", 900, oct(8, 16))
	// Connexions : 3 dans le mois, 1 avant, 1 après.
	seedLogin(t, st, "mj-l1", accID, "rt-a", oct(3, 8))
	seedLogin(t, st, "mj-l2", accID, "rt-a", oct(10, 19))
	seedLogin(t, st, "mj-l3", accID, "rt-a", oct(30, 7))
	seedLogin(t, st, "mj-l-avant", accID, "rt-a", time.Date(2026, 9, 30, 23, 0, 0, 0, loc))
	seedLogin(t, st, "mj-l-apres", accID, "rt-a", time.Date(2026, 11, 1, 1, 0, 0, 0, loc))
	// Volume : jours 05, 15 et 31 dans le mois ; 04 du mois SUIVANT et 30 du
	// mois PRÉCÉDENT hors fenêtre (les clés jour sont au fuseau du compte).
	seedVolumeDay(t, st, "vd-mj-1", accID, "rt-a", "2026-10-05", 1_000_000, 3_000_000, "")
	seedVolumeDay(t, st, "vd-mj-2", accID, "rt-b", "2026-10-15", 2_000_000, 6_000_000, "")
	seedVolumeDay(t, st, "vd-mj-3", accID, "rt-a", "2026-10-31", 500_000, 1_500_000, "")
	seedVolumeDay(t, st, "vd-mj-suivant", accID, "rt-a", "2026-11-04", 9_000_000, 9_000_000, "")
	seedVolumeDay(t, st, "vd-mj-precedent", accID, "rt-a", "2026-09-30", 9_000_000, 9_000_000, "")

	db := st.Data()
	st.Lock()
	auto := buildMonthlyJournal(db, accID, time.Date(2026, 10, 1, 0, 0, 0, 0, loc),
		time.Date(2026, 11, 1, 0, 0, 0, 0, loc), "2026-10-31", "auto",
		time.Date(2026, 11, 1, 2, 0, 0, 0, time.UTC))
	manual := buildMonthlyJournal(db, accID, time.Date(2026, 10, 1, 0, 0, 0, 0, loc),
		time.Date(2026, 10, 15, 12, 0, 0, 0, loc), "2026-10-15", "manual",
		time.Date(2026, 10, 15, 12, 0, 0, 0, loc))
	st.Unlock()

	if auto.ID != "mj-"+accID+":2026-10" || auto.Month != "2026-10" {
		t.Fatalf("journal auto : id=%s month=%s", auto.ID, auto.Month)
	}
	if auto.Sales != 4 || auto.DirectSales != 3 || auto.ResellerSales != 1 {
		t.Fatalf("ventes auto = %d (direct %d, réseau %d), attendu 4 (3+1)", auto.Sales, auto.DirectSales, auto.ResellerSales)
	}
	if auto.Revenue != 2600 || auto.DirectRevenue != 1700 || auto.ResellerRevenue != 900 {
		t.Fatalf("revenus auto = %d (direct %d, réseau %d), attendu 2600 (1700+900)", auto.Revenue, auto.DirectRevenue, auto.ResellerRevenue)
	}
	if auto.AvgTicket != 500 { // (700+500+300+500)/4
		t.Fatalf("panier auto = %d, attendu 500", auto.AvgTicket)
	}
	if auto.Logins != 3 {
		t.Fatalf("connexions auto = %d, attendu 3", auto.Logins)
	}
	if auto.DataIn != 3_500_000 || auto.DataOut != 10_500_000 {
		t.Fatalf("volume auto = %d/%d, attendu 3500000/10500000", auto.DataIn, auto.DataOut)
	}
	if auto.Days != 31 || auto.Partial {
		t.Fatalf("couverture auto = %d j (partial=%v), attendu 31 j pleine", auto.Days, auto.Partial)
	}
	if auto.Source != "auto" || auto.Timezone != "Africa/Abidjan" || auto.Currency == "" {
		t.Fatalf("contexte auto : source=%s tz=%s devise=%q", auto.Source, auto.Timezone, auto.Currency)
	}

	// Manuel : gelé au 15 à midi — le 28, la ligne du 31 et le login du 30
	// sont exclus ; la ligne PARTIELLE du 15 compte (instantané, pas mesure
	// de minuit). Trésorerie = 700 + 500 + 900 (l'encaissement réseau du 8
	// est dans la fenêtre).
	if manual.Sales != 2 || manual.Revenue != 2100 || manual.AvgTicket != 600 { // (700+500)/2
		t.Fatalf("manuel : ventes=%d revenus=%d panier=%d, attendu 2/2100/600", manual.Sales, manual.Revenue, manual.AvgTicket)
	}
	if manual.Logins != 2 {
		t.Fatalf("connexions manuel = %d, attendu 2", manual.Logins)
	}
	if manual.DataIn != 3_000_000 || manual.DataOut != 9_000_000 {
		t.Fatalf("volume manuel = %d/%d, attendu 3000000/9000000 (lignes 05+15)", manual.DataIn, manual.DataOut)
	}
	if manual.Days != 15 || !manual.Partial || manual.Source != "manual" {
		t.Fatalf("couverture manuel = %d j (partial=%v source=%s), attendu 15 j partiel manuel", manual.Days, manual.Partial, manual.Source)
	}
}

// TestEnsureMonthlyJournalsAutoRollover — horloge FABRIQUÉE (1ᵉʳ novembre
// 2026) : le gel automatique couvre le mois précédent de CHAQUE compte, une
// seule fois (clé naturelle), et ne réécrit JAMAIS un mois gelé
// manuellement. L'activité du mois courant n'est pas journalisée.
func TestEnsureMonthlyJournalsAutoRollover(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	_, accA, _ := registerAccount(t, ts, "mj-ensure-a", "")
	// Garde anti-abus : téléphones DISTINCTS pour deux comptes (N°197).
	status, out := doJSON(t, ts, "POST", "/api/auth/register", "", map[string]string{
		"name": "Gérant mj-ensure-b", "username": "mj-ensure-b", "password": "mot-de-passe-8+",
		"email": "mj-ensure-b@example.ci", "phone": "0707070708", "country": "CI", "city": "Abidjan",
	})
	if status != http.StatusCreated {
		t.Fatalf("inscription mj-ensure-b : statut %d, corps %v", status, out)
	}
	user, _ := out["user"].(map[string]any)
	accB, _ := user["accountId"].(string)
	loc := time.UTC
	now := time.Date(2026, 11, 1, 12, 0, 0, 0, loc)
	prevStart, curStart := prevMonthOf(now, loc)

	seedVoucher(t, st, "mj-e-oct", accA, "rt-a", 500, 700, prevStart.AddDate(0, 0, 10), "", "")
	seedVoucher(t, st, "mj-e-nov", accA, "rt-a", 500, 700, curStart.Add(time.Hour), "", "")
	seedLogin(t, st, "mj-e-l", accA, "rt-a", prevStart.AddDate(0, 0, 12))

	db := st.Data()
	a := New(st, testJWTSecret)
	st.Lock()
	if n := a.ensureMonthlyJournals(db, now); n != 2 {
		t.Fatalf("gel auto : %d journaux créés, attendu 2 (un par compte)", n)
	}
	if n := a.ensureMonthlyJournals(db, now); n != 0 {
		t.Fatalf("gel auto non idempotent : %d journaux recréés", n)
	}
	j := journalOf(t, db, accA, "2026-10")
	if j.Sales != 1 || j.Revenue != 700 || j.Logins != 1 || j.Source != "auto" || j.Days != 31 || j.Partial {
		t.Fatalf("journal auto A = ventes %d revenus %d logins %d source %s %d j partial=%v, attendu 1/700/1/auto/31/non", j.Sales, j.Revenue, j.Logins, j.Source, j.Days, j.Partial)
	}
	if _, ok := journalOfOK(db, accB, "2026-10"); !ok {
		t.Fatal("le compte B (dormant) doit aussi avoir son journal d'octobre")
	}
	if _, ok := journalOfOK(db, accA, "2026-11"); ok {
		t.Fatal("le mois COURANT (novembre) ne doit pas être journalisé par le gel auto")
	}

	// Un mois déjà gelé MANUELLEMENT n'est jamais réécrit : l'opération
	// suivante (bascule de décembre) saute octobre.
	manual := model.MonthlyJournal{ID: model.MonthlyJournalID(accA, "2026-10"),
		AccountID: accA, Month: "2026-10", Source: "manual", Partial: true, Days: 14}
	db.MonthlyJournals = []model.MonthlyJournal{manual}
	if n := a.ensureMonthlyJournals(db, time.Date(2026, 12, 1, 9, 0, 0, 0, loc)); n != 2 {
		t.Fatalf("bascule de décembre : %d créations, attendu 2 (novembre de chaque compte — octobre manuel préservé)", n)
	}
	got := journalOf(t, db, accA, "2026-10")
	if got.Source != "manual" || got.Days != 14 {
		t.Fatalf("octobre réécrit ! source=%s days=%d — gelé = gelé", got.Source, got.Days)
	}
	if _, ok := journalOfOK(db, accA, "2026-11"); !ok {
		t.Fatal("novembre doit être gelé à la bascule de décembre")
	}
	st.Unlock()
}

// TestJournalsTimezoneBoundaries — compte à America/New_York : le mois
// précédent démarre à MINUIT NEW-YORKAIS, pas UTC. Un voucher écoulé le
// 30 septembre à 23 h 30 NY (= 1ᵉʳ octobre en UTC) est HORS octobre.
func TestJournalsTimezoneBoundaries(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	_, accID, _ := registerAccount(t, ts, "mj-tz-owner", "")
	setAccountTimezone(t, st, accID, "America/New_York")
	ny, _ := time.LoadLocation("America/New_York")
	now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	prevStart, _ := prevMonthOf(now, ny)

	seedVoucher(t, st, "mj-tz-in", accID, "rt-a", 500, 700,
		time.Date(2026, 10, 1, 0, 30, 0, 0, ny), "", "")
	seedVoucher(t, st, "mj-tz-out", accID, "rt-a", 500, 700,
		time.Date(2026, 9, 30, 23, 30, 0, 0, ny), "", "")
	seedVolumeDay(t, st, "vd-tz-in", accID, "rt-a", "2026-10-01", 1_000_000, 0, "")
	seedVolumeDay(t, st, "vd-tz-out", accID, "rt-a", "2026-09-30", 9_000_000, 9_000_000, "")

	if got := prevStart.In(ny).Format("2006-01-02 15:04"); got != "2026-10-01 00:00" {
		t.Fatalf("début du mois précédent = %s (UTC %s), attendu minuit new-yorkais", got, prevStart.UTC().Format("2006-01-02 15:04"))
	}
	db := st.Data()
	a := New(st, testJWTSecret)
	st.Lock()
	if n := a.ensureMonthlyJournals(db, now); n != 1 {
		t.Fatalf("gel auto : %d journaux, attendu 1", n)
	}
	j := journalOf(t, db, accID, "2026-10")
	st.Unlock()
	if j.Sales != 1 || j.Revenue != 700 || j.DataIn != 1_000_000 {
		t.Fatalf("journal NY : ventes=%d revenus=%d dataIn=%d — le voucher et la ligne du 30 septembre (23 h 30 NY) doivent être EXCLUS", j.Sales, j.Revenue, j.DataIn)
	}
}

// TestJournalsListLazyEnsureAndOrder — GET /api/reports/journals : le
// rattrapage paresseux gèle le mois précédent à la première consultation
// (déterministe : le mois précédent est entièrement passé), l'ordre est
// DESCENDANT, l'état du mois courant est servi pour le bouton de clôture.
func TestJournalsListLazyEnsureAndOrder(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	token, accID, _ := registerAccount(t, ts, "mj-list-owner", "")
	loc := time.UTC
	now := time.Now().UTC()
	prevStart, _ := prevMonthOf(now, loc)

	seedVoucher(t, st, "mj-l-oct", accID, "rt-a", 500, 700, prevStart.AddDate(0, 0, 14), "", "")
	// Un mois encore plus ancien reste un TROU visible (jamais reconstruit
	// de sources purgées) — le rattrapage ne couvre que le mois précédent.
	seedVoucher(t, st, "mj-l-vieux", accID, "rt-a", 500, 700, prevStart.AddDate(0, -1, 3), "", "")

	status, out := doJSON(t, ts, "GET", "/api/reports/journals", token, nil)
	if status != http.StatusOK {
		t.Fatalf("liste : statut %d, corps %v", status, out)
	}
	journals, _ := out["journals"].([]any)
	if len(journals) != 1 {
		t.Fatalf("%d journaux servis, attendu 1 (le mois précédent — les trous plus anciens restent des trous)", len(journals))
	}
	first, _ := journals[0].(map[string]any)
	if first["month"] != prevStart.Format("2006-01") || first["source"] != "auto" {
		t.Fatalf("journal servi : month=%v source=%v", first["month"], first["source"])
	}
	if sales, _ := first["sales"].(float64); int(sales) != 1 {
		t.Fatalf("ventes archivées = %v, attendu 1 (le voucher d'il y a deux mois n'est PAS reconstruit)", first["sales"])
	}
	if out["currentClosed"] != false {
		t.Fatalf("currentClosed = %v, attendu false", out["currentClosed"])
	}
	if m, _ := out["currentMonth"].(string); m == "" {
		t.Fatal("currentMonth doit être servi")
	}
}

// TestJournalCloseManualEndpoint — « Clôturer le mois maintenant » : gel
// partiel du mois courant, réponse complète, puis 409 au second appel (gelé
// = gelé). La liste reflète l'état clos et l'ordre descendant.
func TestJournalCloseManualEndpoint(t *testing.T) {
	_, ts := newTestServerWithStore(t)
	token, accID, _ := registerAccount(t, ts, "mj-close-owner", "")

	if status, _ := doJSON(t, ts, "GET", "/api/reports/journals", token, nil); status != http.StatusOK {
		t.Fatalf("pré-liste : statut %d", status)
	}
	status, out := doJSON(t, ts, "POST", "/api/reports/journals/close", token, nil)
	if status != http.StatusOK {
		t.Fatalf("clôture : statut %d, corps %v", status, out)
	}
	j, _ := out["journal"].(map[string]any)
	if j == nil {
		t.Fatalf("clôture : journal absent de la réponse : %v", out)
	}
	if j["source"] != "manual" || j["partial"] != true {
		t.Fatalf("clôture : source=%v partial=%v, attendu manual/true", j["source"], j["partial"])
	}
	if m, _ := j["month"].(string); m == "" || m != out["currentMonth"] {
		t.Fatalf("clôture : month=%v currentMonth=%v", j["month"], out["currentMonth"])
	}
	if days, _ := j["days"].(float64); int(days) < 1 || int(days) > 31 {
		t.Fatalf("clôture : days=%v hors bornes calendaires", j["days"])
	}
	if id, _ := j["id"].(string); id != "mj-"+accID+":"+out["currentMonth"].(string) {
		t.Fatalf("clôture : id=%v, attendu la clé naturelle mj-<compte>:<mois>", id)
	}

	// Second appel : 409 — un journal gelé ne se réécrit jamais.
	if status, out := doJSON(t, ts, "POST", "/api/reports/journals/close", token, nil); status != http.StatusConflict {
		t.Fatalf("double clôture : statut %d (corps %v), attendu 409", status, out)
	}

	// La liste : mois courant clos en tête, mois précédent auto derrière.
	status, out = doJSON(t, ts, "GET", "/api/reports/journals", token, nil)
	if status != http.StatusOK || out["currentClosed"] != true {
		t.Fatalf("liste après clôture : statut %d currentClosed=%v", status, out["currentClosed"])
	}
	journals, _ := out["journals"].([]any)
	if len(journals) != 2 {
		t.Fatalf("%d journaux, attendu 2 (courant manuel + précédent auto)", len(journals))
	}
	head, _ := journals[0].(map[string]any)
	tail, _ := journals[1].(map[string]any)
	if head["month"] != out["currentMonth"] || head["source"] != "manual" {
		t.Fatalf("ordre descendant : tête month=%v source=%v", head["month"], head["source"])
	}
	if tail["source"] != "auto" {
		t.Fatalf("queue : source=%v, attendu auto (rattrapage de la pré-liste)", tail["source"])
	}
}

// TestJournalsRBACAndCSV — un revendeur (Mode Vente) n'a pas accès aux
// archives (403 sur la liste ET la clôture) ; l'export CSV porte les
// conventions Excel FR (BOM UTF-8, « ; », CRLF) et une ligne par mois gelé.
func TestJournalsRBACAndCSV(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	ownerToken, accID, _ := registerAccount(t, ts, "mj-rbac-owner", "")
	resellerToken := auth.Sign(testJWTSecret, auth.NewClaims("usr-mj-r", "R", "reseller", accID, 0))

	if status, _ := doJSON(t, ts, "GET", "/api/reports/journals", resellerToken, nil); status != http.StatusForbidden {
		t.Fatalf("revendeur liste : statut %d, attendu 403", status)
	}
	if status, _ := doJSON(t, ts, "POST", "/api/reports/journals/close", resellerToken, nil); status != http.StatusForbidden {
		t.Fatalf("revendeur clôture : statut %d, attendu 403", status)
	}

	// Deux mois gelés (seeding direct — l'export est testé, pas le gel).
	st.Lock()
	st.Data().MonthlyJournals = []model.MonthlyJournal{
		{ID: model.MonthlyJournalID(accID, "2026-08"), AccountID: accID, Month: "2026-08",
			Sales: 40, Revenue: 26000, AvgTicket: 650, Logins: 512, Days: 31, Source: "auto"},
		{ID: model.MonthlyJournalID(accID, "2026-09"), AccountID: accID, Month: "2026-09",
			Sales: 560, Revenue: 350000, AvgTicket: 625, Logins: 4100, Days: 30, Source: "auto"},
	}
	st.Save()
	st.Unlock()

	req, err := http.NewRequest("GET", ts.URL+"/api/reports/journals.csv", nil)
	if err != nil {
		t.Fatalf("CSV requête : %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+ownerToken)
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("CSV : %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("CSV : statut %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Fatalf("CSV : Content-Type=%s", ct)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("CSV lecture : %v", err)
	}
	body := string(raw)
	if !strings.HasPrefix(body, "\xEF\xBB\xBF") {
		t.Fatal("CSV : BOM UTF-8 absent (Excel FR)")
	}
	if !strings.Contains(body, "2026-08 ;40 ;26000") || !strings.Contains(body, "2026-09 ;560 ;350000") {
		t.Fatalf("CSV : lignes des mois absentes ou mal formées :\n%s", body)
	}
	if !strings.Contains(body, "\r\n") {
		t.Fatal("CSV : CRLF attendus (Excel FR)")
	}
	if !strings.Contains(body, "Journaux mensuels") {
		t.Fatal("CSV : en-tête du document absent")
	}
}

// TestRetentionSweepClosesJournals — le BALAYAGE HORAIRE (comptes dormants)
// gèle le mois précédent : même moteur que la consultation, sans requête.
func TestRetentionSweepClosesJournals(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	_, accID, _ := registerAccount(t, ts, "mj-sweep-owner", "")
	prevStart, _ := prevMonthOf(time.Now().UTC(), time.UTC)
	seedVoucher(t, st, "mj-s-oct", accID, "rt-a", 500, 700, prevStart.AddDate(0, 0, 20), "", "")

	a := New(st, testJWTSecret)
	a.RunRetentionSweep()

	st.Lock()
	j, ok := journalOfOK(st.Data(), accID, prevStart.Format("2006-01"))
	st.Unlock()
	if !ok {
		t.Fatal("le balayage horaire doit geler le mois précédent d'un compte jamais consulté")
	}
	if j.Sales != 1 || j.Revenue != 700 || j.Source != "auto" {
		t.Fatalf("journal du balayage : ventes=%d revenus=%d source=%s", j.Sales, j.Revenue, j.Source)
	}
}

// journalOfOK — journal gelé d'un compte pour un mois, s'il existe.
func journalOfOK(db *model.DB, accID, month string) (model.MonthlyJournal, bool) {
	id := model.MonthlyJournalID(accID, month)
	for i := range db.MonthlyJournals {
		if db.MonthlyJournals[i].ID == id {
			return db.MonthlyJournals[i], true
		}
	}
	return model.MonthlyJournal{}, false
}

// journalOf — idem, échec du test si absent.
func journalOf(t *testing.T, db *model.DB, accID, month string) model.MonthlyJournal {
	t.Helper()
	j, ok := journalOfOK(db, accID, month)
	if !ok {
		t.Fatalf("journal %s du compte %s absent", month, accID)
	}
	return j
}
