package api

// Tests N°199 — accumulateur journalier de volume de données :
//   - overview : le KPI Volume se lit dans les AGRÉGATS JOURNALIERS (une
//     ligne par compte, routeur, jour au fuseau du compte) — plus dans les
//     sessions vivantes (constat C2 : les sessions fermées quittent le
//     store) ;
//   - fenêtre précédente « au même moment » : lignes pleines + histogramme
//     horaire 0..heure en cours de la ligne frontière ; la zone morte ne
//     compte nulle part ;
//   - filtre site : seules les lignes du routeur sélectionné ;
//   - read_state (mode agent) : chaque delta verse dans l'agrégat du jour —
//     initial puis incrémental, sans double comptage.
//
// Déterminisme : les jours et heures attendus sont recalculés côté test à
// partir de time.Now() (même arithmétique que le serveur) — les assertions
// tiennent à n'importe quelle heure d'exécution.

import (
	"net/url"
	"testing"
	"time"

	"mikcloud/hotspot-api/internal/model"
)

// volHours — histogramme horaire déterministe : 1 Mo à h0, 2 Mo à h1,
// 3 Mo à h2, 0 ensuite. La somme 0..h est recalculable côté test.
func volHours() string {
	hist := model.VolumeHoursAdd("", 0, 1_000_000)
	hist = model.VolumeHoursAdd(hist, 1, 2_000_000)
	hist = model.VolumeHoursAdd(hist, 2, 3_000_000)
	return hist
}

// volHoursSum — somme attendue des seaux 0..h de volHours.
func volHoursSum(upto int) int64 {
	counts := []int64{1_000_000, 2_000_000, 3_000_000}
	var sum int64
	for h := 0; h <= upto && h < len(counts); h++ {
		sum += counts[h]
	}
	return sum
}

// seedVolumeDay — agrégat journalier de volume servi (N°199).
func seedVolumeDay(t *testing.T, st storeIface, id, accID, routerID, day string, bytesIn, bytesOut int64, hours string) {
	t.Helper()
	st.Lock()
	st.Data().VolumeDays = append(st.Data().VolumeDays, model.VolumeDay{
		ID: id, AccountID: accID, RouterID: routerID, Day: day,
		BytesIn: bytesIn, BytesOut: bytesOut, Hours: hours,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
	})
	st.Save()
	st.Unlock()
}

// TestStatsOverviewVolumeFromDailyRows — jour : la fenêtre courante additionne
// les lignes d'aujourd'hui (tous sites), la précédente coupe la ligne
// d'hier à l'heure en cours (histogramme). Un jour plus ancien, un autre
// compte et une session VIVANTE sans ligne ne comptent pas.
func TestStatsOverviewVolumeFromDailyRows(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	token, accID, _ := registerAccount(t, ts, "ov-vd-owner", "")

	now := time.Now().UTC()
	todayKey := now.Format("2006-01-02")
	yesterdayKey := now.AddDate(0, 0, -1).Format("2006-01-02")
	dayBeforeKey := now.AddDate(0, 0, -2).Format("2006-01-02")

	seedVolumeDay(t, st, "vd-cur-a", accID, "rt-a", todayKey, 1_000_000, 2_000_000, "")
	seedVolumeDay(t, st, "vd-cur-b", accID, "rt-b", todayKey, 7_000_000, 0, "")
	seedVolumeDay(t, st, "vd-cur-autre", "acc-etranger", "rt-a", todayKey, 42_000_000, 0, "")
	seedVolumeDay(t, st, "vd-prev-a", accID, "rt-a", yesterdayKey, 60_000_000, 0, volHours())
	seedVolumeDay(t, st, "vd-avant", accID, "rt-a", dayBeforeKey, 5_000_000, 0, "")
	// Session vivante SANS ligne : la nouvelle doctrine ne la compte plus
	// (l'accumulateur est la seule source — garde anti-retour).
	seedSession(t, st, "ov-vd-live", accID, "rt-a", now.Add(-time.Minute), 999_999, 0)

	status, out := doJSON(t, ts, "GET", "/api/stats/overview?period=day", token, nil)
	if status != 200 {
		t.Fatalf("overview : statut %d, corps %v", status, out)
	}
	kpis := overviewKPIsOf(t, out)
	if n := kpiInt(t, kpis, "dataBytes"); n != 10_000_000 {
		t.Fatalf("dataBytes = %d, attendu 10000000 (3 Mo site A + 7 Mo site B — l'autre compte et la session vive sans ligne exclus)", n)
	}
	wantPrev := int(volHoursSum(now.Hour()))
	if n := kpiInt(t, kpis, "dataBytesPrev"); n != int(wantPrev) {
		t.Fatalf("dataBytesPrev = %d, attendu %d (histogramme d'hier coupé à l'heure en cours %dh — le jour d'avant et les 60 Mo du total d'hier exclus)", n, wantPrev, now.Hour())
	}

	// Filtre site : uniquement les lignes du routeur A.
	status, out = doJSON(t, ts, "GET", "/api/stats/overview?period=day&routerId=rt-a", token, nil)
	if status != 200 {
		t.Fatalf("overview site : statut %d", status)
	}
	kpis = overviewKPIsOf(t, out)
	if n := kpiInt(t, kpis, "dataBytes"); n != 3_000_000 {
		t.Fatalf("dataBytes site A = %d, attendu 3000000", n)
	}
	if n := kpiInt(t, kpis, "dataBytesPrev"); n != int(wantPrev) {
		t.Fatalf("dataBytesPrev site A = %d, attendu %d", n, wantPrev)
	}
}

// TestStatsOverviewVolumeWeekWindows — semaine : fenêtre courante = lignes
// lundi→aujourd'hui ; fenêtre précédente = lignes pleines de la semaine
// passée + histogramme de la ligne frontière (jour de prevEnd) ; les jours
// de la zone morte ne comptent NULLE PART.
func TestStatsOverviewVolumeWeekWindows(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	token, accID, _ := registerAccount(t, ts, "ov-vw-owner", "")

	loc := time.UTC
	now := time.Now().UTC().In(loc)
	wd := int(now.Weekday())
	if wd == 0 {
		wd = 7
	}
	monday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, -(wd - 1))
	prevStart := monday.AddDate(0, 0, -7)
	prevEnd := prevStart.Add(now.Sub(monday)) // même durée écoulée
	boundaryDay := time.Date(prevEnd.Year(), prevEnd.Month(), prevEnd.Day(), 0, 0, 0, 0, loc)
	todayKey := now.Format("2006-01-02")

	// Fenêtre courante : un montant distinct par jour lundi → hier, plus le
	// jour courant (partiel par nature).
	var wantCur int64 = 5_000_000 // ligne du jour courant
	for d := 0; d < wd-1; d++ {
		day := monday.AddDate(0, 0, d)
		amount := int64(1_000_000 * (d + 1))
		seedVolumeDay(t, st, "vd-cur-"+string(rune('a'+d)), accID, "rt-a", day.Format("2006-01-02"), amount, 0, "")
		wantCur += amount
	}
	seedVolumeDay(t, st, "vd-cur-today", accID, "rt-a", todayKey, 5_000_000, 0, "")

	// Fenêtre précédente : jours pleins de la semaine passée avant la
	// frontière, puis la ligne frontière (coupe à l'heure en cours).
	var wantPrev int64
	for d := 0; d < 7; d++ {
		day := prevStart.AddDate(0, 0, d)
		if day.Before(boundaryDay) {
			seedVolumeDay(t, st, "vd-prev-"+string(rune('a'+d)), accID, "rt-a", day.Format("2006-01-02"), 2_000_000, 0, "")
			wantPrev += 2_000_000
		}
	}
	seedVolumeDay(t, st, "vd-prev-boundary", accID, "rt-a", boundaryDay.Format("2006-01-02"), 50_000_000, 0, volHours())
	wantPrev += volHoursSum(now.Hour())

	// Zone morte : premier jour strictement après la frontière et avant
	// lundi (s'il existe) — ne compte NULLE PART.
	if dead := boundaryDay.AddDate(0, 0, 1); dead.Before(monday) {
		seedVolumeDay(t, st, "vd-dead", accID, "rt-a", dead.Format("2006-01-02"), 4_000_000, 0, "")
	}

	status, out := doJSON(t, ts, "GET", "/api/stats/overview?period=week", token, nil)
	if status != 200 {
		t.Fatalf("overview week : statut %d, corps %v", status, out)
	}
	kpis := overviewKPIsOf(t, out)
	if n := kpiInt(t, kpis, "dataBytes"); n != int(wantCur) {
		t.Fatalf("dataBytes semaine = %d, attendu %d (lundi→aujourd'hui)", n, wantCur)
	}
	if n := kpiInt(t, kpis, "dataBytesPrev"); n != int(wantPrev) {
		t.Fatalf("dataBytesPrev semaine = %d, attendu %d (semaine passée au même moment, zone morte exclue)", n, wantPrev)
	}
}

// TestApplyReadStateAccumulatesVolumeDay — mode agent : la PREMIÈRE
// observation d'une session verse ses octets initiaux à l'agrégat du jour ;
// les polls suivants ne versent que les DELTAS (jamais de double comptage).
func TestApplyReadStateAccumulatesVolumeDay(t *testing.T) {
	old := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339)
	db := &model.DB{}
	db.Routers = []model.Router{{ID: "r-vol", AccountID: "acc", Name: "R", Mode: "agent", Status: "online"}}
	db.HotspotUsers = []model.HotspotUser{{
		ID: "v-vol", AccountID: "acc", RouterID: "r-vol", Username: "voluser",
		Kind: "voucher", Status: "active", CreatedAt: old,
	}}
	vals := url.Values{}
	vals.Set("users", "voluser|default|false;")

	// 1er poll : session nouvelle, octets initiaux 1024/2048.
	vals.Set("sessions", "voluser|10.5.0.9|1m|1024|2048|9c:3a:af:11:22:33;")
	(&API{}).applyReadState(db, &db.Routers[0], vals)
	if len(db.VolumeDays) != 1 {
		t.Fatalf("1 ligne de volume attendue après le 1er read_state, obtenu %d", len(db.VolumeDays))
	}
	row := db.VolumeDays[0]
	if row.AccountID != "acc" || row.RouterID != "r-vol" {
		t.Fatalf("clé de ligne = (%s, %s), attendu (acc, r-vol)", row.AccountID, row.RouterID)
	}
	if row.BytesIn != 1024 || row.BytesOut != 2048 {
		t.Fatalf("totaux = %d/%d, attendu 1024/2048 (octets initiaux)", row.BytesIn, row.BytesOut)
	}
	now := time.Now().UTC()
	if row.Day != now.Format("2006-01-02") {
		t.Fatalf("Day = %q, attendu le jour courant (compte sans réglage → UTC)", row.Day)
	}
	if row.ID != model.VolumeDayID("acc", "r-vol", row.Day) {
		t.Fatalf("ID = %q, attendu la clé naturelle", row.ID)
	}
	if got := model.VolumeHoursSumThrough(row.Hours, now.Hour()); got != 1024+2048 {
		t.Fatalf("histogramme 0..%dh = %d, attendu 3072", now.Hour(), got)
	}
	if u := db.HotspotUsers[0]; u.BytesIn != 1024 || u.BytesOut != 2048 {
		t.Fatalf("compteurs user = %d/%d, attendu 1024/2048 (miroir inchangé)", u.BytesIn, u.BytesOut)
	}

	// 2e poll : session appariée, DELTAS uniquement (2048/2048 en plus).
	vals.Set("sessions", "voluser|10.5.0.9|2m|3072|4096|9c:3a:af:11:22:33;")
	(&API{}).applyReadState(db, &db.Routers[0], vals)
	if len(db.VolumeDays) != 1 {
		t.Fatalf("toujours 1 ligne (même jour), obtenu %d", len(db.VolumeDays))
	}
	row = db.VolumeDays[0]
	if row.BytesIn != 3072 || row.BytesOut != 4096 {
		t.Fatalf("totaux = %d/%d, attendu 3072/4096 (deltas cumulés, pas de double comptage)", row.BytesIn, row.BytesOut)
	}
	if got := model.VolumeHoursSumThrough(row.Hours, 23); got != 3072+4096 {
		t.Fatalf("histogramme complet = %d, attendu 7168", got)
	}
	if u := db.HotspotUsers[0]; u.BytesIn != 3072 || u.BytesOut != 4096 {
		t.Fatalf("compteurs user = %d/%d, attendu 3072/4096", u.BytesIn, u.BytesOut)
	}

	// Compteur décroissant (reset-counters) : delta 0, la ligne ne recule pas.
	vals.Set("sessions", "voluser|10.5.0.9|1m|100|100|9c:3a:af:11:22:33;")
	(&API{}).applyReadState(db, &db.Routers[0], vals)
	row = db.VolumeDays[0]
	if row.BytesIn != 3072 || row.BytesOut != 4096 {
		t.Fatalf("reset-counters : la ligne doit rester monotone, obtenu %d/%d", row.BytesIn, row.BytesOut)
	}
}
