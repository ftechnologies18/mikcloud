package model

// Tests N°103 — Qualité de ligne (mesure passive du débit FAI) :
//   - histogrammes : seau d'un débit, ajout d'échantillon, fusion (base de la
//     survie aux redéploiements), percentile depuis le sommet, tolérance du
//     parse (formes corrompues → distribution vierge, jamais de panique) ;
//   - accumulation : une ligne par (routeur, jour, interface), max par
//     direction, jour calendaire UTC (== Abidjan — arbitrage FamilyGuard) ;
//   - rétention : 90 jours glissants, les jours se comparent en chaînes.

import (
	"strings"
	"testing"
	"time"
)

func TestLineQualityBucketIndex(t *testing.T) {
	cas := []struct {
		bps  int64
		want int
	}{
		{-1, 0},             // donnée absurde → seau 0
		{0, 0},              // idle
		{99_999, 0},         // juste sous la première borne
		{100_000, 0},        // borne INCLUSE
		{100_001, 1},        // juste au-dessus
		{5_000_000, 5},      // 5 Mbps pile
		{95_000_000, 11},    // 95 Mbps → seau 75-100M
		{1_000_000_000, 15}, // 1 Gbps pile
		{4_000_000_000, 15}, // débordement → dernier seau
	}
	for _, c := range cas {
		if got := LineQualityBucketIndex(c.bps); got != c.want {
			t.Errorf("LineQualityBucketIndex(%d) = %d, attendu %d", c.bps, got, c.want)
		}
	}
}

func TestLineQualityHistAddAndPercentile(t *testing.T) {
	// 20 échantillons à ~2 Mbps (seau 4) + 1 à ~95 Mbps (seau 11) :
	// p95 = 2,5 Mbps (borne haute du seau 4) — le pic isolé ne tire PAS la
	// réponse d'un seau entier, c'est précisément son rôle.
	hist := ""
	for i := 0; i < 20; i++ {
		hist = LineQualityHistAdd(hist, 2_000_000)
	}
	hist = LineQualityHistAdd(hist, 95_000_000)
	if got := LineQualityHistPercentile(hist, 95); got != 2_500_000 {
		t.Fatalf("p95 = %d, attendu 2500000 (borne du seau 2-2,5M)", got)
	}
	// p50 : médiane dans la masse → même seau.
	if got := LineQualityHistPercentile(hist, 50); got != 2_500_000 {
		t.Fatalf("p50 = %d, attendu 2500000", got)
	}
	// p100 : le sommet compte → borne du seau du pic.
	if got := LineQualityHistPercentile(hist, 100); got != 100_000_000 {
		t.Fatalf("p100 = %d, attendu 100000000", got)
	}
	// Distribution vide → 0, jamais de panique.
	if got := LineQualityHistPercentile("", 95); got != 0 {
		t.Fatalf("p95 d'un histogramme vide = %d, attendu 0", got)
	}
}

func TestLineQualityHistMerge(t *testing.T) {
	a := LineQualityHistAdd("", 3_000_000)
	a = LineQualityHistAdd(a, 3_000_000)
	b := LineQualityHistAdd("", 90_000_000)
	m := LineQualityHistMerge(a, b)
	// Fusion commutative : 2 × seau 4 + 1 × seau 11.
	if m != LineQualityHistMerge(b, a) {
		t.Fatalf("fusion non commutative : %q vs %q", m, LineQualityHistMerge(b, a))
	}
	if got := LineQualityHistPercentile(m, 66); got != 5_000_000 {
		t.Fatalf("p66 de la fusion = %d, attendu 5000000 (2/3 des échantillons sous le seau 5)", got)
	}
	// Fusion avec une forme corrompue : les champs LISIBLES comptent, les
	// champs non numériques valent 0 (défense en profondeur — la base ne
	// renvoie que des formes canoniques, une corruption partielle ne doit pas
	// tout effacer). 2×3M + 1×<100k + 3×250-500k : la médiane tombe dans le
	// seau 250-500k.
	if got := LineQualityHistPercentile(LineQualityHistMerge(a, "1,x,3"), 50); got != 500_000 {
		t.Fatalf("fusion avec forme corrompue : p50 = %d, attendu 500000", got)
	}
}

func TestLineQualityHistParseTolerance(t *testing.T) {
	for _, bad := range []string{",,,;", "abc", "-5,-5"} {
		if got := LineQualityHistPercentile(bad, 95); got != 0 {
			t.Errorf("forme corrompue %q : p95 = %d, attendu 0", bad, got)
		}
	}
	// Forme TRONQUÉE mais lisible : les premiers seaux survivent, le reste
	// vaut 0 — la distribution reste exploitable (1×<100k + 2×100-250k +
	// 3×250-500k : p95 au seau 250-500k).
	if got := LineQualityHistPercentile("1,2,3", 95); got != 500_000 {
		t.Errorf("forme tronquée %q : p95 = %d, attendu 500000", "1,2,3", got)
	}
	// Forme canonique produite par HistAdd : exactement un champ par seau.
	h := LineQualityHistAdd("", 95_000_000) // seau 11 = 1
	if len(strings.Split(h, ",")) != len(lineQualityBucketEdges) {
		t.Fatalf("forme canonique : %d champs attendus, obtenu %q", len(lineQualityBucketEdges), h)
	}
}

func TestAccumulateLineQuality(t *testing.T) {
	db := &DB{LineQuality: []LineQualityDay{}}
	router := &Router{ID: "r-lq", AccountID: "acc-lq"}
	t0 := time.Date(2026, 9, 14, 18, 0, 0, 0, time.UTC)

	// Fenêtre 1 : deux interfaces, débits plausibles.
	model := []IfaceTraffic{
		{Name: "ether1", RxBps: 40_000_000, TxBps: 8_000_000},
		{Name: "bridge-lan", RxBps: 1_000_000, TxBps: 500_000},
	}
	AccumulateLineQuality(db, router, model, t0)
	if len(db.LineQuality) != 2 {
		t.Fatalf("2 lignes attendues (une par interface), %d obtenues", len(db.LineQuality))
	}
	var wan LineQualityDay
	for i := range db.LineQuality {
		if db.LineQuality[i].Iface == "ether1" {
			wan = db.LineQuality[i]
		}
	}
	if wan.Samples != 1 || wan.RxMaxBps != 40_000_000 || wan.TxMaxBps != 8_000_000 {
		t.Fatalf("premier échantillon mal enregistré : %+v", wan)
	}
	if wan.Day != "2026-09-14" || wan.AccountID != "acc-lq" || wan.ID == "" {
		t.Fatalf("scoping du jour/compte : %+v", wan)
	}

	// Fenêtre 2 (même jour) : max montants, échantillons cumulés.
	AccumulateLineQuality(db, router, []IfaceTraffic{
		{Name: "ether1", RxBps: 96_000_000, TxBps: 4_000_000},
	}, t0.Add(2*time.Minute))
	if len(db.LineQuality) != 2 {
		t.Fatalf("le jour suivant ne crée pas de ligne, %d obtenues", len(db.LineQuality))
	}
	for i := range db.LineQuality {
		lq := &db.LineQuality[i]
		if lq.Iface == "ether1" {
			if lq.Samples != 2 || lq.RxMaxBps != 96_000_000 || lq.TxMaxBps != 8_000_000 {
				t.Fatalf("second échantillon : %+v", lq)
			}
		}
	}

	// Jour suivant : NOUVELLE ligne, l'historique du jour 1 est conservé.
	AccumulateLineQuality(db, router, []IfaceTraffic{
		{Name: "ether1", RxBps: 60_000_000, TxBps: 6_000_000},
	}, t0.Add(26*time.Hour))
	var days int
	for _, lq := range db.LineQuality {
		if lq.Iface == "ether1" {
			days++
		}
	}
	if days != 2 {
		t.Fatalf("2 jours distincts attendus pour ether1, %d obtenus", days)
	}

	// Routeur nil / interfaces vides : aucun effet, aucune panique.
	AccumulateLineQuality(db, nil, model, t0)
	AccumulateLineQuality(db, router, nil, t0)
}

func TestPruneLineQuality(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	db := &DB{LineQuality: []LineQualityDay{
		{ID: "lq-1", Day: "2026-09-14"}, // aujourd'hui — conservé
		{ID: "lq-2", Day: "2026-06-20"}, // 86 j — conservé (≤ 90 j)
		{ID: "lq-3", Day: "2026-06-15"}, // 91 j — purgé
		{ID: "lq-4", Day: "2026-01-01"}, // très vieux — purgé
	}}
	if n := PruneLineQuality(db, now); n != 2 {
		t.Fatalf("2 lignes attendues purgées, %d obtenues", n)
	}
	if len(db.LineQuality) != 2 || db.LineQuality[0].ID != "lq-1" || db.LineQuality[1].ID != "lq-2" {
		t.Fatalf("rétention : lignes survivantes %v", db.LineQuality)
	}
	if n := PruneLineQuality(db, now); n != 0 {
		t.Fatalf("purge idempotente : %d lignes re-purgées", n)
	}
}
