// N°103 — Qualité de ligne (mesure passive du débit FAI), logique pure.
//
// OBJECTIF : dimensionner la QoS d'un site sur sa capacité RÉELLE, pas sur
// l'étiquette commerciale du forfait. Un routeur ne peut pas exposer « le
// débit du FAI » (paramètre commercial du réseau opérateur) ; il peut en
// revanche mesurer la capacité EFFECTIVE de la ligne : l'enveloppe des débits
// observés sur l'interface WAN aux heures chargées converge vers le plafond
// réel (GPON partagé, offre « 110M » qui en rend 96 : c'est le 96 qu'il faut
// bridé, pas le 110).
//
// MOYEN : chaque read_state (~2 min, N°74) rapporte les compteurs cumulés
// rx-byte/tx-byte de chaque interface running ; applyAgentTraffic en déduit
// déjà les débits instantanés (fenêtre de mesure = l'intervalle entre deux
// rapports). Il suffit d'ACCUMULER ces débits par interface et par jour UTC
// (== heure d'Abidjan, la Côte d'Ivoire n'applique pas l'heure d'été — même
// arbitrage que FamilyGuard N°82) pour obtenir, après quelques jours, une
// enveloppe honnête : max et p95 par jour et par direction.
//
// HISTOGRAMME FUSIONNABLE : les échantillons individuels ne sont PAS gardés
// (720/jour/interface ≈ lourd et inutile) — chaque jour porte un histogramme
// de débits à 16 seaux log-échelle, forme « c0,c1,…,c15 ». Un histogramme se
// MERGE incrémentalement (comptes additionnés), donc l'agrégat survit aux
// redémarrages du cloud (Render redéploie souvent) sans jamais perdre un
// échantillon déjà compté ; le p95 se relit de la distribution en cumulant
// depuis le sommet. Un pic isolé (glitch de compteur, fenêtre atypique) ne
// déforme ni le p95 ni le max journalier — le max est rapporté à part, le p95
// est par construction insensible aux ≤ 5 % d'outliers.
//
// RÉTENTION : 90 jours (pruneLineQuality, moteur commun applyExpiry) —
// suffisamment long pour couvrir un cycle de fréquentation complet (vacances
// scolaires, Ramadan, fêtes), suffisamment court pour borner la table Neon à
// ~90 j × interfaces × routeurs (quelques centaines de lignes par site).
package model

import "time"

// lineQualityBucketEdges — bornes SUPÉRIEURES des seaux de l'histogramme de
// débits (bits/s). Échelle quasi-logarithmique : assez fine dans la zone
// utile d'un hotspot (1-100 Mbps) pour qu'un p95 de seau reste une estimation
// serrée, assez large en dessous (idle, ~0) pour ne pas compter le vide.
// Dernier seau = débordement implicite « ≥ 1 Gbps » (le p95 y retourne la
// borne haute — un hotspot ne dépasse pas 1 Gbps de fenêtre moyenne 2 min).
var lineQualityBucketEdges = []int64{
	100_000,       // seau 0 : < 0,1 Mbps — idle / fichiers de service
	250_000,       // seau 1 : 0,1-0,25 Mbps
	500_000,       // seau 2 : 0,25-0,5 Mbps
	1_000_000,     // seau 3 : 0,5-1 Mbps
	2_500_000,     // seau 4 : 1-2,5 Mbps
	5_000_000,     // seau 5 : 2,5-5 Mbps
	10_000_000,    // seau 6 : 5-10 Mbps
	20_000_000,    // seau 7 : 10-20 Mbps
	30_000_000,    // seau 8 : 20-30 Mbps
	50_000_000,    // seau 9 : 30-50 Mbps
	75_000_000,    // seau 10 : 50-75 Mbps
	100_000_000,   // seau 11 : 75-100 Mbps
	150_000_000,   // seau 12 : 100-150 Mbps
	250_000_000,   // seau 13 : 150-250 Mbps
	500_000_000,   // seau 14 : 250-500 Mbps
	1_000_000_000, // seau 15 : 500 Mbps-1 Gbps (débordement : ≥ 1 Gbps)
}

// LineQualityBucketIndex — seau d'un débit (bits/s) : premier seau dont la
// borne supérieure couvre la valeur. Débits négatifs (donnée absurde) → 0.
func LineQualityBucketIndex(bps int64) int {
	if bps < 0 {
		return 0
	}
	for i, edge := range lineQualityBucketEdges {
		if bps <= edge {
			return i
		}
	}
	return len(lineQualityBucketEdges) - 1
}

// LineQualityHistAdd — ajoute UN échantillon à un histogramme sérialisé
// (« c0,c1,… ») et retourne la nouvelle forme. Une forme vide/corrompue est
// traitée comme une distribution vierge (défense : la colonne ne reçoit que
// des formes produites ici, mais le parse reste tolérant).
func LineQualityHistAdd(hist string, bps int64) string {
	counts := parseLineQualityHist(hist)
	counts[LineQualityBucketIndex(bps)]++
	return formatLineQualityHist(counts)
}

// LineQualityHistMerge — fusionne deux histogrammes sérialisés (somme des
// comptes, seau par seau). Base de la fusion incrémentale : redémarrages,
// rechargements, convergence multi-chunks.
func LineQualityHistMerge(a, b string) string {
	ca := parseLineQualityHist(a)
	cb := parseLineQualityHist(b)
	for i := range ca {
		ca[i] += cb[i]
	}
	return formatLineQualityHist(ca)
}

// LineQualityHistPercentile — valeur de seau du percentile demandé (0-100),
// calculée depuis le SOMMET : plus petite borne supérieure de seau couvrant
// les (100-pct) % d'échantillons les plus élevés. Résolution = la largeur du
// seau (l'agrégat ne garde pas les échantillons — c'est le prix du merge).
// Distribution vide → 0.
func LineQualityHistPercentile(hist string, pct float64) int64 {
	counts := parseLineQualityHist(hist)
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	var total int
	for _, c := range counts {
		total += c
	}
	if total == 0 {
		return 0
	}
	// Part des échantillons autorisée AU-DESSUS du percentile (arrondie vers
	// le haut : le p95 est « la valeur que 95 % n'excèdent pas » — un échantillon
	// isolé au sommet ne doit pas faire sauter la réponse d'un seau entier).
	excess := int(float64(total) * (100 - pct) / 100)
	if excess < 0 {
		excess = 0
	}
	if excess > total {
		excess = total
	}
	seen := 0
	for i := len(counts) - 1; i >= 0; i-- {
		seen += counts[i]
		if seen > excess {
			return lineQualityBucketEdges[i]
		}
	}
	return lineQualityBucketEdges[0]
}

// parseLineQualityHist — « c0,c1,… » → comptes TOUJOURS de la longueur des
// seaux (forme vide, champs non numériques ou tronqués : 0, la distribution
// est reconstruite à la prochaine fusion — jamais de panique sur une donnée
// de base).
func parseLineQualityHist(s string) []int {
	counts := make([]int, len(lineQualityBucketEdges))
	if s == "" {
		return counts
	}
	part := 0
	idx := 0
	for i := 0; i <= len(s) && idx < len(counts); i++ {
		if i == len(s) || s[i] == ',' {
			if i > part {
				n := 0
				signed := false
				for _, c := range s[part:i] {
					if c >= '0' && c <= '9' {
						n = n*10 + int(c-'0')
					} else {
						signed = true // champ non numérique : compte 0
					}
				}
				if !signed && n > 0 {
					counts[idx] = n
				}
			}
			part = i + 1
			idx++
		}
	}
	return counts
}

// formatLineQualityHist — comptes → « c0,c1,… » (longueur canonique des
// seaux ; un seau ne déborde jamais car les comptes d'un jour de hotspot sont
// bornés par 1440 min / 2 min = 720 échantillons max).
func formatLineQualityHist(counts []int) string {
	if len(counts) != len(lineQualityBucketEdges) {
		fixed := make([]int, len(lineQualityBucketEdges))
		copy(fixed, counts)
		counts = fixed
	}
	var sb []byte
	for i, c := range counts {
		if i > 0 {
			sb = append(sb, ',')
		}
		// Comptes négatifs (donnée absurde) : sérialisés 0 — la distribution
		// reste une distribution.
		if c < 0 {
			c = 0
		}
		sb = appendInt(sb, c)
	}
	return string(sb)
}

// appendInt — strconv sans l'importer : les comptes d'un histogramme sont
// petits et positifs (≤ 720), un append direct évite l'allocation de Sprint.
func appendInt(b []byte, n int) []byte {
	if n == 0 {
		return append(b, '0')
	}
	var tmp [20]byte
	i := len(tmp)
	for n > 0 {
		i--
		tmp[i] = byte('0' + n%10)
		n /= 10
	}
	return append(b, tmp[i:]...)
}

// LineQualityDayKey — clé du jour UTC (== Abidjan) d'un instant :
// « 2006-01-02 ». Les jours se comparent lexicographiquement (tri naturel).
func LineQualityDayKey(now time.Time) string {
	return now.UTC().Format("2006-01-02")
}

// AccumulateLineQuality — N°103 — verse les débits INSTANTANÉS d'un rapport
// (fenêtre de mesure = l'intervalle entre deux rapports, cf. applyAgentTraffic
// côté api / tickTraffic côté store) dans les agrégats quotidiens du routeur.
// Une ligne par (routeur, jour, interface) : Samples++ par interface rapportée,
// max/direction mis à jour, histogrammes alimentés des deux directions.
// À appeler sous le verrou du store ; idempotent de fait (un même rapport
// re-appliqué ne compte double que si le cloud le reçoit deux fois — la
// convergence N°73 ne rejoue jamais un chunk déjà ackné).
func AccumulateLineQuality(db *DB, router *Router, ifaces []IfaceTraffic, now time.Time) {
	if len(ifaces) == 0 || router == nil {
		return
	}
	day := LineQualityDayKey(now)
	for _, it := range ifaces {
		if it.Name == "" {
			continue
		}
		var row *LineQualityDay
		for i := range db.LineQuality {
			if db.LineQuality[i].RouterID == router.ID && db.LineQuality[i].Day == day && db.LineQuality[i].Iface == it.Name {
				row = &db.LineQuality[i]
				break
			}
		}
		if row == nil {
			if db.LineQuality == nil {
				db.LineQuality = []LineQualityDay{}
			}
			db.LineQuality = append(db.LineQuality, LineQualityDay{
				ID:        NewID("lq-"),
				AccountID: router.AccountID,
				RouterID:  router.ID,
				Day:       day,
				Iface:     it.Name,
				UpdatedAt: now.UTC().Format(time.RFC3339),
			})
			row = &db.LineQuality[len(db.LineQuality)-1]
		}
		row.Samples++
		if it.RxBps > row.RxMaxBps {
			row.RxMaxBps = it.RxBps
		}
		if it.TxBps > row.TxMaxBps {
			row.TxMaxBps = it.TxBps
		}
		row.RxHist = LineQualityHistAdd(row.RxHist, it.RxBps)
		row.TxHist = LineQualityHistAdd(row.TxHist, it.TxBps)
		row.UpdatedAt = now.UTC().Format(time.RFC3339)
	}
}

// PruneLineQuality — rétention : retire les agrégats de plus de 90 jours.
// Les jours « YYYY-MM-DD » se trient lexicographiquement — une simple
// comparaison de chaînes suffit. À appeler sous verrou (moteur commun
// applyExpiry) ; renvoie le nombre de lignes purgées.
func PruneLineQuality(db *DB, now time.Time) int {
	deadline := now.UTC().AddDate(0, 0, -90).Format("2006-01-02")
	before := len(db.LineQuality)
	if before == 0 {
		return 0
	}
	kept := db.LineQuality[:0]
	for _, lq := range db.LineQuality {
		if lq.Day >= deadline {
			kept = append(kept, lq)
		}
	}
	db.LineQuality = kept
	return before - len(db.LineQuality)
}
