// Package telemetry — B2 « Speed App UX » : collecte des Core Web Vitals
// (LCP, INP, CLS, FCP, TTFB) remontés par le frontend vers POST /api/vitals.
//
// Isolée volontairement du store principal : la télémétrie est à haut volume,
// append-only et non critique — elle ne doit jamais alourdir l'état métier
// (model.DB) ni la synchro différentielle de pg.go. Deux niveaux :
//   - mémoire : ring buffer borné — source des agrégats de
//     GET /api/vitals/summary, fonctionne même sans Neon ;
//   - Neon (si DATABASE_URL) : table web_vitals, insertions par LOT
//     asynchrones (file bornée, flush 10 min ou 200 échantillons) —
//     l'historique survit aux redéploiements Render ; un incident Neon ne
//     peut jamais bloquer une requête API (dégradation silencieuse, pertes
//     loguées).
//
// Le démarrage ne touche JAMAIS Neon synchrone (le cold start Render ne doit
// pas s'allonger d'un réveil de base) : le schéma et la recharge de
// l'historique récent sont faits paresseusement par la goroutine de flush.
package telemetry

import (
	"context"
	"database/sql"
	"log"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // pilote « pgx » pour database/sql (même driver que pg.go)
)

// Métriques Core Web Vitals acceptées (+ diagnostics associés FCP/TTFB).
var validMetrics = map[string]bool{"LCP": true, "CLS": true, "INP": true, "FCP": true, "TTFB": true}

// ValidMetric — true si la métrique est une Core Web Vital connue (validation
// du handler POST /api/vitals).
func ValidMetric(name string) bool { return validMetrics[name] }

const (
	ringCapacity   = 20_000           // échantillons conservés en mémoire (~jours à cette échelle)
	queueCapacity  = 2_000            // échantillons en attente d'insertion Neon (au-delà : drop + log)
	flushEvery     = 10 * time.Minute // cadence d'insertion (alignée sur le keep-alive Render)
	flushThreshold = 200              // flush anticipé dès ce seuil
	insertChunk    = 200              // max lignes par INSERT multi-lignes (8 col × 200 = 1 600 paramètres)
	insertTimeout  = 15 * time.Second
	bootstrapDelay = 45 * time.Second // 1er passage : schéma + recharge historique (en tâche de fond)
	lookback       = 48 * time.Hour   // historique rechargé au boot
	minGroupN      = 5                // p75 par groupe calculé seulement si assez d'échantillons
)

// Sample — un rapport de métrique (un rapport web-vitals = un échantillon).
type Sample struct {
	At     time.Time
	Metric string  // LCP | CLS | INP | FCP | TTFB
	Value  float64 // ms (CLS sans unité)
	Rating string  // good | needs-improvement | poor | ""
	Path   string  // chemin de la page mesurée (≤ 128)
	Device string  // mobile | desktop (dérivé serveur du User-Agent, jamais stocké brut)
	Nav    string  // navigate | reload | back-forward | …
	Sid    string  // id de session de mesure (sessionStorage, sans cookie ni PII)
}

// Collector — ring mémoire + file d'insertion Neon (best-effort).
type Collector struct {
	mu      sync.Mutex
	ring    []Sample
	head    int // position du plus ancien quand le ring est plein
	count   int
	queue   []Sample
	dropped int // échantillons perdus (file saturée) — logué au flush

	db          *sql.DB
	schemaReady bool
	kick        chan struct{}
	stop        chan struct{}
	done        chan struct{}
}

// NewCollector — construit le collecteur. databaseURL vide ou non-postgres
// (mode JSON de développement) → collecte mémoire seule.
func NewCollector(databaseURL string) *Collector {
	c := &Collector{kick: make(chan struct{}, 1), stop: make(chan struct{}), done: make(chan struct{})}
	if strings.HasPrefix(databaseURL, "postgres://") || strings.HasPrefix(databaseURL, "postgresql://") {
		db, err := sql.Open("pgx", databaseURL)
		if err != nil {
			log.Printf("vitals: pool PostgreSQL indisponible (%v) — collecte mémoire seule", err)
		} else {
			// Pool minuscule dédié à la télémétrie : 1-2 connexions suffisent
			// (un INSERT par lot toutes les 10 min) — aucune contention avec pg.go.
			db.SetMaxOpenConns(2)
			db.SetMaxIdleConns(1)
			db.SetConnMaxIdleTime(5 * time.Minute)
			c.db = db
		}
	}
	go c.run()
	return c
}

// Close — arrête la boucle et flushe le dernier lot (appelé sur SIGTERM).
func (c *Collector) Close() {
	close(c.stop)
	<-c.done
	if c.db != nil {
		_ = c.db.Close()
	}
}

// Add — enregistre des échantillons (validés en amont par le handler).
// Jamais bloquant : la file Neon est bornée (drop + compteur au-delà).
func (c *Collector) Add(samples []Sample) {
	c.mu.Lock()
	for _, s := range samples {
		if c.count < ringCapacity {
			c.ring = append(c.ring, s)
			c.count++
		} else {
			c.ring[c.head] = s
			c.head = (c.head + 1) % ringCapacity
		}
		if c.db != nil {
			if len(c.queue) < queueCapacity {
				c.queue = append(c.queue, s)
			} else {
				c.dropped++
			}
		}
	}
	q := len(c.queue)
	c.mu.Unlock()
	if q >= flushThreshold {
		select {
		case c.kick <- struct{}{}:
		default:
		}
	}
}

// run — goroutine de fond : bootstrap paresseux puis flush périodique/au seuil.
func (c *Collector) run() {
	defer close(c.done)
	if c.db == nil {
		<-c.stop // mémoire seule : rien à flusher, attendre l'arrêt
		return
	}
	select {
	case <-c.stop:
		return
	case <-time.After(bootstrapDelay):
	}
	c.bootstrap()
	ticker := time.NewTicker(flushEvery)
	defer ticker.Stop()
	for {
		select {
		case <-c.stop:
			c.flush()
			return
		case <-c.kick:
			c.flush()
		case <-ticker.C:
			c.flush()
		}
	}
}

// bootstrap — DDL (CREATE TABLE IF NOT EXISTS, même mécanisme que pg.go au
// boot) + recharge de l'historique récent : les agrégats survivent aux
// redéploiements Render au lieu de repartir de zéro à chaque push.
func (c *Collector) bootstrap() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err := c.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS web_vitals (
        id          BIGSERIAL PRIMARY KEY,
        sampled_at  TIMESTAMPTZ NOT NULL,
        metric      TEXT NOT NULL,
        value       DOUBLE PRECISION NOT NULL,
        rating      TEXT NOT NULL DEFAULT '',
        path        TEXT NOT NULL DEFAULT '',
        device      TEXT NOT NULL DEFAULT '',
        nav         TEXT NOT NULL DEFAULT '',
        sid         TEXT NOT NULL DEFAULT ''
)`)
	if err == nil {
		_, err = c.db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_web_vitals_at ON web_vitals (sampled_at DESC)`)
	}
	if err != nil {
		log.Printf("vitals: schéma Neon non prêt (%v) — nouvelle tentative au prochain flush", err)
		return
	}
	c.schemaReady = true

	// Recharge de l'historique récent dans le ring (les plus récents d'abord).
	rows, err := c.db.QueryContext(ctx, `
SELECT sampled_at, metric, value, rating, path, device, nav, sid
FROM web_vitals WHERE sampled_at > $1 ORDER BY sampled_at DESC LIMIT $2`,
		time.Now().UTC().Add(-lookback), ringCapacity)
	if err != nil {
		log.Printf("vitals: recharge historique impossible (%v)", err)
		return
	}
	defer rows.Close()
	n := 0
	c.mu.Lock()
	for rows.Next() {
		var s Sample
		if err := rows.Scan(&s.At, &s.Metric, &s.Value, &s.Rating, &s.Path, &s.Device, &s.Nav, &s.Sid); err == nil {
			if c.count < ringCapacity {
				c.ring = append(c.ring, s)
				c.count++
			}
			n++
		}
	}
	c.mu.Unlock()
	log.Printf("vitals: historique rechargé depuis Neon (%d échantillons sur %dh)", n, int(lookback.Hours()))
}

// flush — insère la file par lots bornés. Best-effort : en cas d'erreur, le
// lot est remis en tête de file (bornée — les plus anciens sont perdus, c'est
// assumé) et l'erreur est loguée ; aucune requête API n'est impactée.
func (c *Collector) flush() {
	if c.db == nil {
		return
	}
	c.mu.Lock()
	batch := c.queue
	c.queue = nil
	dropped := c.dropped
	c.dropped = 0
	c.mu.Unlock()
	if len(batch) == 0 && dropped == 0 {
		return
	}
	if !c.schemaReady {
		c.bootstrap() // un Neon joignable entre-temps complète le schéma
		if !c.schemaReady {
			c.requeue(batch, dropped)
			return
		}
	}
	for start := 0; start < len(batch); start += insertChunk {
		end := start + insertChunk
		if end > len(batch) {
			end = len(batch)
		}
		if err := c.insertBatch(batch[start:end]); err != nil {
			log.Printf("vitals: insertion Neon échouée (%v) — lot remis en file", err)
			c.requeue(batch[start:], 0)
			return
		}
	}
	if dropped > 0 {
		log.Printf("vitals: %d échantillon(s) perdu(s) — file Neon saturée (incident ou pic)", dropped)
	}
}

// insertBatch — INSERT multi-lignes (8 colonnes × ≤ 200 lignes = 1 600
// paramètres, très sous la limite PostgreSQL des 65 535).
func (c *Collector) insertBatch(rows []Sample) error {
	ctx, cancel := context.WithTimeout(context.Background(), insertTimeout)
	defer cancel()
	var sb strings.Builder
	args := make([]any, 0, len(rows)*8)
	sb.WriteString(`INSERT INTO web_vitals (sampled_at, metric, value, rating, path, device, nav, sid) VALUES `)
	for i, s := range rows {
		if i > 0 {
			sb.WriteByte(',')
		}
		base := i * 8
		sb.WriteString("(")
		for j := 0; j < 8; j++ {
			if j > 0 {
				sb.WriteByte(',')
			}
			sb.WriteString("$" + itoa(base+j+1))
		}
		sb.WriteString(")")
		args = append(args, s.At, s.Metric, s.Value, s.Rating, s.Path, s.Device, s.Nav, s.Sid)
	}
	_, err := c.db.ExecContext(ctx, sb.String(), args...)
	return err
}

// requeue — remet un lot non inséré en tête de file (bornée : FIFO drop).
func (c *Collector) requeue(batch []Sample, dropped int) {
	c.mu.Lock()
	if len(batch) >= queueCapacity {
		c.dropped += len(batch) + dropped - queueCapacity
		c.queue = append(c.queue[:0], batch[len(batch)-queueCapacity:]...)
	} else if room := queueCapacity - len(batch); len(c.queue) > room {
		c.dropped += len(c.queue) - room + dropped
		c.queue = c.queue[:room]
		c.queue = append(batch, c.queue...)
	} else {
		c.queue = append(batch, c.queue...)
		c.dropped += dropped
	}
	c.mu.Unlock()
}

// itoa — conversion int → décimal (évite fmt.Sprintf dans l'assemblage SQL).
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// ---------------------------------------------------------------------------
// Agrégats (GET /api/vitals/summary)
// ---------------------------------------------------------------------------

// MetricAgg — agrégat d'une métrique sur la fenêtre.
type MetricAgg struct {
	N                int     `json:"n"`
	P50              float64 `json:"p50"`
	P75              float64 `json:"p75"`
	P95              float64 `json:"p95"`
	Good             int     `json:"good"`
	NeedsImprovement int     `json:"needsImprovement"`
	Poor             int     `json:"poor"`
}

// GroupAgg — agrégat d'un groupe (chemin ou type d'appareil).
type GroupAgg struct {
	Path string             `json:"path,omitempty"` // renseigné pour les groupes par chemin
	N    int                `json:"n"`
	P75  map[string]float64 `json:"p75"`
}

// Summary — synthèse sur la fenêtre (heures, 1-168). Déduplication « dernier
// rapport » par (session, page, métrique) : web-vitals re-rapporte LCP/CLS/INP
// quand la valeur évolue ; la recommandation Google pour les p75 est de ne
// compter que le dernier rapport de chaque chargement de page.
func (c *Collector) Summary(hours int) map[string]any {
	if hours < 1 {
		hours = 24
	}
	if hours > 168 {
		hours = 168
	}
	cutoff := time.Now().UTC().Add(-time.Duration(hours) * time.Hour)

	type dedupeKey struct{ sid, path, metric string }
	latest := map[dedupeKey]Sample{}

	c.mu.Lock()
	for i := 0; i < c.count; i++ {
		s := c.ring[(c.head+i)%ringCapacity]
		if s.At.Before(cutoff) {
			continue
		}
		k := dedupeKey{s.Sid, s.Path, s.Metric}
		if prev, ok := latest[k]; !ok || s.At.After(prev.At) {
			latest[k] = s
		}
	}
	c.mu.Unlock()

	// Une seule passe : valeurs par métrique + groupes (chemin, appareil).
	values := map[string][]float64{}
	aggs := map[string]*MetricAgg{}
	pathVals := map[string]map[string][]float64{}
	deviceVals := map[string]map[string][]float64{}
	pathN := map[string]int{}
	deviceN := map[string]int{}
	for _, s := range latest {
		values[s.Metric] = append(values[s.Metric], s.Value)
		agg := aggs[s.Metric]
		if agg == nil {
			agg = &MetricAgg{}
			aggs[s.Metric] = agg
		}
		agg.N++
		switch s.Rating {
		case "good":
			agg.Good++
		case "needs-improvement":
			agg.NeedsImprovement++
		case "poor":
			agg.Poor++
		}
		if s.Path != "" {
			if pathVals[s.Path] == nil {
				pathVals[s.Path] = map[string][]float64{}
			}
			pathVals[s.Path][s.Metric] = append(pathVals[s.Path][s.Metric], s.Value)
			pathN[s.Path]++
		}
		if s.Device != "" {
			if deviceVals[s.Device] == nil {
				deviceVals[s.Device] = map[string][]float64{}
			}
			deviceVals[s.Device][s.Metric] = append(deviceVals[s.Device][s.Metric], s.Value)
			deviceN[s.Device]++
		}
	}
	metrics := map[string]*MetricAgg{}
	for m, vals := range values {
		sort.Float64s(vals)
		agg := aggs[m]
		agg.P50 = quantile(vals, 0.50)
		agg.P75 = quantile(vals, 0.75)
		agg.P95 = quantile(vals, 0.95)
		metrics[m] = agg
	}
	// Chemins : top 8 par volume, p75 par métrique (si assez d'échantillons).
	pathAggs := make([]*GroupAgg, 0, len(pathN))
	for path, n := range pathN {
		if n < minGroupN {
			continue
		}
		pathAggs = append(pathAggs, &GroupAgg{Path: path, N: n, P75: groupP75(pathVals[path])})
	}
	sort.Slice(pathAggs, func(i, j int) bool { return pathAggs[i].N > pathAggs[j].N })
	if len(pathAggs) > 8 {
		pathAggs = pathAggs[:8]
	}
	// Appareils : mobile / desktop (p75 seulement si assez d'échantillons).
	devices := map[string]*GroupAgg{}
	for device, n := range deviceN {
		if n < minGroupN {
			continue
		}
		devices[device] = &GroupAgg{N: n, P75: groupP75(deviceVals[device])}
	}
	return map[string]any{
		"window":  hours,
		"samples": len(latest),
		"metrics": metrics,
		"paths":   pathAggs,
		"devices": devices,
	}
}

// groupP75 — p75 par métrique pour un groupe (métriques trop rares : omises).
func groupP75(byMetric map[string][]float64) map[string]float64 {
	out := map[string]float64{}
	for m, vals := range byMetric {
		if len(vals) < minGroupN {
			continue
		}
		sort.Float64s(vals)
		out[m] = quantile(vals, 0.75)
	}
	return out
}

// quantile — méthode « nearest-rank » des Core Web Vitals : sur une liste
// TRIÉE, index ceil(q·n)-1.
func quantile(sorted []float64, q float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(math.Ceil(q*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}
