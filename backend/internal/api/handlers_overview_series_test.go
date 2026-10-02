package api

// Tests N°203 (P4, UX finale de la refonte reports) — VOLUME DANS LES
// BUCKETS de la série intrapériode de GET /api/stats/overview :
//   - période Jour : chaque bucket porte l'heure locale correspondante de
//     l'histogramme « h0,…,h23 » des lignes du jour (multi-routeurs sommés) ;
//   - période Semaine / Mois : un bucket par jour calendaire, lignes
//     journalières versées au jour qui leur appartient ;
//   - période Année : un bucket par mois, lignes sommées par préfixe ;
//   - cohérence par construction : Σ buckets = KPI dataBytes (l'histogramme
//     et les totaux sont versés ENSEMBLE par AccumulateVolumeDay) ;
//   - fenêtres précédentes et autres comptes : JAMAIS dans la série.
//
// Déterminisme : les heures et jours semés sont RELATIFS à l'instant
// d'exécution (heure en cours, lundi en cours, 1ᵉʳ du mois, 1ᵉʳ janvier) —
// les assertions tiennent à n'importe quelle date, y compris un lundi,
// un 1ᵉʳ du mois ou un 1ᵉʳ janvier. Mois et Année utilisent des COMPTES
// DISTINCTS : les lignes semées pour l'un ne doivent pas polluer la fenêtre
// de l'autre (le 1ᵉʳ du mois et le mois précédent sont DANS l'année).

import (
	"net/http"
	"testing"
	"time"

	"mikcloud/hotspot-api/internal/model"
)

// seriesVolumeSum — Σ dataBytes des buckets de la série intrapériode.
func seriesVolumeSum(t *testing.T, out map[string]any) int64 {
	t.Helper()
	series, _ := out["series"].([]any)
	var sum int64
	for _, raw := range series {
		p, _ := raw.(map[string]any)
		if p == nil {
			continue
		}
		if n, ok := p["dataBytes"].(float64); ok {
			sum += int64(n)
		}
	}
	return sum
}

// seriesVolumeAt — dataBytes du bucket i (-1 si le bucket n'existe pas).
func seriesVolumeAt(t *testing.T, out map[string]any, i int) int64 {
	t.Helper()
	series, _ := out["series"].([]any)
	if i < 0 || i >= len(series) {
		return -1
	}
	p, _ := series[i].(map[string]any)
	if p == nil {
		return -1
	}
	n, _ := p["dataBytes"].(float64)
	return int64(n)
}

// kpiDataBytesOf — KPI dataBytes de la réponse overview.
func kpiDataBytesOf(t *testing.T, out map[string]any) int64 {
	t.Helper()
	kpis := overviewKPIsOf(t, out)
	v, ok := kpis["dataBytes"].(float64)
	if !ok {
		t.Fatalf("KPI dataBytes absent ou non numérique : %v", kpis["dataBytes"])
	}
	return int64(v)
}

// TestOverviewSeriesVolumeDayHourly — Jour : buckets HORAIRES depuis
// l'histogramme des lignes du jour, multi-routeurs sommés ; le filtre site
// borne la série ; un autre compte n'apparaît jamais.
func TestOverviewSeriesVolumeDayHourly(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	token, accID, _ := registerAccount(t, ts, "ov-ser-day-owner", "")

	now := time.Now().UTC()
	h := now.Hour()
	todayKey := now.Format("2006-01-02")

	// rt-a : 5 Mo à l'heure en cours (3 Mo in + 2 Mo out).
	histA := model.VolumeHoursAdd("", h, 5_000_000)
	seedVolumeDay(t, st, "vd-ser-a", accID, "rt-a", todayKey, 3_000_000, 2_000_000, histA)

	// rt-b : 2 Mo à l'heure en cours + 1 Mo à l'heure précédente si elle existe.
	inB := int64(2_000_000)
	histB := model.VolumeHoursAdd("", h, 2_000_000)
	if h >= 1 {
		histB = model.VolumeHoursAdd(histB, h-1, 1_000_000)
		inB += 1_000_000
	}
	seedVolumeDay(t, st, "vd-ser-b", accID, "rt-b", todayKey, inB, 0, histB)

	// Hors sujet : autre compte, même jour.
	seedVolumeDay(t, st, "vd-ser-etranger", "acc-etranger", "rt-a", todayKey, 42_000_000, 0, "")

	status, out := doJSON(t, ts, "GET", "/api/stats/overview?period=day", token, nil)
	if status != http.StatusOK {
		t.Fatalf("overview day : statut %d, corps %v", status, out)
	}
	series, _ := out["series"].([]any)
	if len(series) != h+1 {
		t.Fatalf("série de %d points, attendu %d (00 h → heure en cours)", len(series), h+1)
	}
	// Bucket de l'heure en cours : 5 Mo (rt-a) + 2 Mo (rt-b).
	if got := seriesVolumeAt(t, out, h); got != 7_000_000 {
		t.Fatalf("bucket %02dh : dataBytes = %d, attendu 7 000 000", h, got)
	}
	// Bucket de l'heure précédente : 1 Mo (rt-b) — s'il est affiché.
	if h >= 1 {
		if got := seriesVolumeAt(t, out, h-1); got != 1_000_000 {
			t.Fatalf("bucket %02dh : dataBytes = %d, attendu 1 000 000", h-1, got)
		}
	}
	// Cohérence par construction : Σ buckets = KPI.
	want := int64(5_000_000 + inB)
	if got := seriesVolumeSum(t, out); got != want {
		t.Fatalf("Σ série = %d, attendu %d", got, want)
	}
	if got := kpiDataBytesOf(t, out); got != want {
		t.Fatalf("KPI dataBytes = %d, attendu %d", got, want)
	}

	// Filtre site : seul rt-a apparaît (bucket + KPI).
	status, out = doJSON(t, ts, "GET", "/api/stats/overview?period=day&routerId=rt-a", token, nil)
	if status != http.StatusOK {
		t.Fatalf("overview day site : statut %d, corps %v", status, out)
	}
	if got := seriesVolumeAt(t, out, h); got != 5_000_000 {
		t.Fatalf("bucket %02dh (site rt-a) : dataBytes = %d, attendu 5 000 000", h, got)
	}
	if got := kpiDataBytesOf(t, out); got != 5_000_000 {
		t.Fatalf("KPI dataBytes (site rt-a) = %d, attendu 5 000 000", got)
	}
}

// TestOverviewSeriesVolumeWeekBuckets — Semaine : un bucket par jour
// calendaire (lundi → aujourd'hui) ; la ligne de la semaine précédente
// alimente la fenêtre de comparaison, JAMAIS la série.
func TestOverviewSeriesVolumeWeekBuckets(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	token, accID, _ := registerAccount(t, ts, "ov-ser-week-owner", "")

	nowLocal := time.Now().UTC()
	wd := int(nowLocal.Weekday())
	if wd == 0 {
		wd = 7
	}
	curStart := time.Date(nowLocal.Year(), nowLocal.Month(), nowLocal.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -(wd - 1))
	mondayKey := curStart.Format("2006-01-02")
	todayKey := nowLocal.Format("2006-01-02")
	prevWeekKey := curStart.AddDate(0, 0, -7).Format("2006-01-02")

	seedVolumeDay(t, st, "vd-ser-wk-mon", accID, "rt-a", mondayKey, 1_000_000, 0, "")
	seedVolumeDay(t, st, "vd-ser-wk-today", accID, "rt-b", todayKey, 2_000_000, 0, "")
	seedVolumeDay(t, st, "vd-ser-wk-prev", accID, "rt-a", prevWeekKey, 9_000_000, 0, "")

	status, out := doJSON(t, ts, "GET", "/api/stats/overview?period=week", token, nil)
	if status != http.StatusOK {
		t.Fatalf("overview week : statut %d, corps %v", status, out)
	}
	series, _ := out["series"].([]any)
	if len(series) != wd {
		t.Fatalf("série de %d points, attendu %d (lundi → aujourd'hui)", len(series), wd)
	}
	// Lundi ET aujourd'hui fusionnent quand la semaine vient de tourner.
	lastWant, firstWant := int64(2_000_000), int64(1_000_000)
	if wd == 1 {
		lastWant, firstWant = 3_000_000, 3_000_000
	}
	if got := seriesVolumeAt(t, out, wd-1); got != lastWant {
		t.Fatalf("bucket aujourd'hui : dataBytes = %d, attendu %d", got, lastWant)
	}
	if got := seriesVolumeAt(t, out, 0); got != firstWant {
		t.Fatalf("bucket lundi : dataBytes = %d, attendu %d", got, firstWant)
	}
	// Σ = 3 Mo exactement — la semaine précédente n'y est pas.
	if got := seriesVolumeSum(t, out); got != 3_000_000 {
		t.Fatalf("Σ série = %d, attendu 3 000 000 (semaine précédente exclue)", got)
	}
	if got := kpiDataBytesOf(t, out); got != 3_000_000 {
		t.Fatalf("KPI dataBytes = %d, attendu 3 000 000", got)
	}
}

// TestOverviewSeriesVolumeMonthYearBuckets — Mois (1ᵉʳ → aujourd'hui) et
// Année (janvier → mois en cours), sur des COMPTES DISTINCTS : les lignes
// du mois précédent appartiennent à l'année courante et ne doivent pas
// fausser la fenêtre année du même compte.
func TestOverviewSeriesVolumeMonthYearBuckets(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	token, accID, _ := registerAccount(t, ts, "ov-ser-mo-owner", "")

	nowLocal := time.Now().UTC()

	// —— MOIS ——
	firstOfMonth := time.Date(nowLocal.Year(), nowLocal.Month(), 1, 0, 0, 0, 0, time.UTC)
	firstKey := firstOfMonth.Format("2006-01-02")
	todayKey := nowLocal.Format("2006-01-02")
	lastMonthKey := firstOfMonth.AddDate(0, -1, 0).Format("2006-01-02")

	seedVolumeDay(t, st, "vd-ser-mo-first", accID, "rt-a", firstKey, 1_000_000, 0, "")
	seedVolumeDay(t, st, "vd-ser-mo-today", accID, "rt-b", todayKey, 2_000_000, 0, "")
	seedVolumeDay(t, st, "vd-ser-mo-prev", accID, "rt-a", lastMonthKey, 9_000_000, 0, "")

	status, out := doJSON(t, ts, "GET", "/api/stats/overview?period=month", token, nil)
	if status != http.StatusOK {
		t.Fatalf("overview month : statut %d, corps %v", status, out)
	}
	series, _ := out["series"].([]any)
	if len(series) != nowLocal.Day() {
		t.Fatalf("série de %d points, attendu %d (1ᵉʳ → aujourd'hui)", len(series), nowLocal.Day())
	}
	lastWant, firstWant := int64(2_000_000), int64(1_000_000)
	if nowLocal.Day() == 1 {
		lastWant, firstWant = 3_000_000, 3_000_000
	}
	if got := seriesVolumeAt(t, out, nowLocal.Day()-1); got != lastWant {
		t.Fatalf("bucket aujourd'hui : dataBytes = %d, attendu %d", got, lastWant)
	}
	if got := seriesVolumeAt(t, out, 0); got != firstWant {
		t.Fatalf("bucket 1ᵉʳ du mois : dataBytes = %d, attendu %d", got, firstWant)
	}
	if got := seriesVolumeSum(t, out); got != 3_000_000 {
		t.Fatalf("Σ série (mois) = %d, attendu 3 000 000 (mois précédent exclu)", got)
	}

	// —— ANNÉE (compte séparé) ——
	// Garde anti-abus : téléphones DISTINCTS pour deux comptes (N°197).
	status, out = doJSON(t, ts, "POST", "/api/auth/register", "", map[string]string{
		"name": "Gérant ov-ser-yr-owner", "username": "ov-ser-yr-owner", "password": "mot-de-passe-8+",
		"email": "ov-ser-yr-owner@example.ci", "phone": "0707070708", "country": "CI", "city": "Abidjan",
	})
	if status != http.StatusCreated {
		t.Fatalf("inscription ov-ser-yr-owner : statut %d, corps %v", status, out)
	}
	tokenY, _ := out["token"].(string)
	user, _ := out["user"].(map[string]any)
	accY, _ := user["accountId"].(string)
	janKey := time.Date(nowLocal.Year(), 1, 1, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
	lastYearKey := time.Date(nowLocal.Year()-1, 12, 25, 0, 0, 0, 0, time.UTC).Format("2006-01-02")

	seedVolumeDay(t, st, "vd-ser-yr-jan", accY, "rt-a", janKey, 1_000_000, 0, "")
	seedVolumeDay(t, st, "vd-ser-yr-today", accY, "rt-b", todayKey, 2_000_000, 0, "")
	seedVolumeDay(t, st, "vd-ser-yr-prev", accY, "rt-a", lastYearKey, 9_000_000, 0, "")

	status, out = doJSON(t, ts, "GET", "/api/stats/overview?period=year", tokenY, nil)
	if status != http.StatusOK {
		t.Fatalf("overview year : statut %d, corps %v", status, out)
	}
	series, _ = out["series"].([]any)
	if len(series) != int(nowLocal.Month()) {
		t.Fatalf("série de %d points, attendu %d (janvier → mois en cours)", len(series), int(nowLocal.Month()))
	}
	janWant, curWant := int64(1_000_000), int64(2_000_000)
	if nowLocal.Month() == time.January {
		janWant, curWant = 3_000_000, 3_000_000
	}
	if got := seriesVolumeAt(t, out, 0); got != janWant {
		t.Fatalf("bucket janvier : dataBytes = %d, attendu %d", got, janWant)
	}
	if got := seriesVolumeAt(t, out, int(nowLocal.Month())-1); got != curWant {
		t.Fatalf("bucket mois courant : dataBytes = %d, attendu %d", got, curWant)
	}
	if got := seriesVolumeSum(t, out); got != 3_000_000 {
		t.Fatalf("Σ série (année) = %d, attendu 3 000 000 (l'an dernier exclu)", got)
	}
}
