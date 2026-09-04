// handlers_vitals.go — B2 « Speed App UX » : collecte des Core Web Vitals
// (contrat : docs/CONTRACT-V2.md, section B2).
//
//   - POST /api/vitals : PUBLIC (la vitrine anonyme est justement la page à
//     mesurer) — whitelisté dans middleware.go. Reçu via navigator.sendBeacon
//     avec un Blob « text/plain » : requête SIMPLE, zéro preflight CORS, la
//     réponse (204) n'est jamais lue. Le limiteur général (120/min/IP + 900/
//     min global, main.go) protège la route ; le corps est borné à 8 Kio.
//   - GET /api/vitals/summary : plateforme uniquement (isPlatformAdmin) —
//     p50/p75/p95, répartition des ratings, top chemins, mobile/desktop.
//
// Aucune donnée personnelle : chemin de page, métrique, rating, type de
// navigation, indice mobile/desktop (dérivé du User-Agent) et un identifiant
// de session de mesure éphémère (sessionStorage). IP et User-Agent bruts ne
// sont JAMAIS stockés (l'IP ne sert qu'au limiteur, en mémoire).

package api

import (
	"math"
	"net/http"
	"strings"
	"time"

	"mikcloud/hotspot-api/internal/telemetry"
)

// vitalsMaxBody — les beacons font quelques centaines d'octets ; 8 Kio
// laissent une marge large tout en bornant drastiquement l'abus.
const vitalsMaxBody = 8 << 10

// vitalsMaxMetrics — un chargement de page produit 5 métriques (LCP, INP,
// CLS, FCP, TTFB) ; la mèche de ré-émission standard reste sous 8.
const vitalsMaxMetrics = 8

// WithVitals — branche le collecteur de télémétrie (main.go). Chaînable :
// api.New(st, secret).WithVitals(collector).Handler().
func (a *API) WithVitals(c *telemetry.Collector) *API {
	a.vitals = c
	return a
}

// vitalsMetricReq / vitalsReq — payload du beacon (text/plain contenant du JSON).
type vitalsMetricReq struct {
	Name   string  `json:"name"`
	Value  float64 `json:"value"`
	Rating string  `json:"rating"`
}

type vitalsReq struct {
	Path    string            `json:"path"`
	Sid     string            `json:"sid"`
	Nav     string            `json:"nav"`
	Metrics []vitalsMetricReq `json:"metrics"`
}

// validVitalsRatings / validVitalsNav — miroirs des listes du package telemetry.
var (
	validVitalsRatings = map[string]bool{"good": true, "needs-improvement": true, "poor": true, "": true}
	validVitalsNav     = map[string]bool{
		"navigate": true, "reload": true, "back-forward": true,
		"back-forward-cache": true, "prerender": true, "restore": true, "": true,
	}
)

// handleVitalsPost — réception des échantillons. Réponse 204 systématique en
// cas de succès (le beacon ne lit jamais la réponse) ; erreurs 400 pour un
// corps malformé (utile uniquement au débogage du reporter).
func (a *API) handleVitalsPost(w http.ResponseWriter, r *http.Request) {
	if a.vitals == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, vitalsMaxBody)
	var req vitalsReq
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	if len(req.Metrics) == 0 || len(req.Metrics) > vitalsMaxMetrics {
		writeErr(w, http.StatusBadRequest, "Métriques absentes ou trop nombreuses (max 8)")
		return
	}
	path := strings.TrimSpace(req.Path)
	if len(path) > 128 {
		writeErr(w, http.StatusBadRequest, "Chemin trop long (max 128)")
		return
	}
	if path != "" && !strings.HasPrefix(path, "/") {
		writeErr(w, http.StatusBadRequest, "Chemin invalide")
		return
	}
	sid := strings.TrimSpace(req.Sid)
	if len(sid) > 64 {
		sid = sid[:64]
	}
	nav := req.Nav
	if !validVitalsNav[nav] {
		nav = ""
	}
	device := deviceHint(r.UserAgent())
	now := time.Now().UTC()

	samples := make([]telemetry.Sample, 0, len(req.Metrics))
	for _, m := range req.Metrics {
		name := strings.TrimSpace(m.Name)
		if !telemetry.ValidMetric(name) {
			writeErr(w, http.StatusBadRequest, "Métrique inconnue : "+name)
			return
		}
		if math.IsNaN(m.Value) || math.IsInf(m.Value, 0) || m.Value < 0 || m.Value > 600_000 {
			writeErr(w, http.StatusBadRequest, "Valeur hors bornes pour "+name)
			return
		}
		rating := m.Rating
		if !validVitalsRatings[rating] {
			rating = ""
		}
		samples = append(samples, telemetry.Sample{
			At:     now,
			Metric: name,
			Value:  m.Value,
			Rating: rating,
			Path:   path,
			Device: device,
			Nav:    nav,
			Sid:    sid,
		})
	}
	a.vitals.Add(samples)
	w.WriteHeader(http.StatusNoContent)
}

// handleVitalsSummary — synthèse des Core Web Vitals (plateforme uniquement).
// ?window=heures (1-168, défaut 24). Source : ring mémoire (rechargé depuis
// Neon au boot, voir telemetry) — couvre au moins les 48 dernières heures.
func (a *API) handleVitalsSummary(w http.ResponseWriter, r *http.Request) {
	if !isPlatformAdmin(r) {
		writeErr(w, http.StatusForbidden, "Accès réservé à la plateforme MikCloud")
		return
	}
	if a.vitals == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"window": 24, "samples": 0,
			"metrics": map[string]*telemetry.MetricAgg{},
			"paths":   []any{}, "devices": map[string]*telemetry.GroupAgg{},
		})
		return
	}
	hours := queryInt(r, "window", 24, 1, 168)
	writeJSON(w, http.StatusOK, a.vitals.Summary(hours))
}

// deviceHint — indice mobile/desktop dérivé du User-Agent (jamais stocké
// brut) : la segmentation mobile compte double ici — les usagers 3G/4G
// d'Abidjan sur Android forment le cœur de la mesure de latence réelle.
func deviceHint(ua string) string {
	if ua == "" {
		return ""
	}
	if strings.Contains(ua, "Mobi") || strings.Contains(ua, "Android") {
		return "mobile"
	}
	return "desktop"
}
