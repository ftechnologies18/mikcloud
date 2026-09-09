// httpstats.go — compteur de bande passante sortante par catégorie (N°72).
//
// Avant N°72, RIEN ne mesurait ce qui sortait du service : l'opérateur ne
// découvrait la consommation qu'à la suspension du workspace (email Render
// « 5 GB of free bandwidth used » — le diagnostic exact de la panne du
// 2026-09-09). Ce compteur alimente le bloc « bandwidth » de
// GET /api/admin/sync-status (console plateforme, onglet Maintenance,
// carte « Santé de la persistance ») : l'opérateur voit chaque jour ce qui
// consomme le quota, sans attendre un incident.
//
// Ce qui est compté : les OCTETS DE CORPS de réponse effectivement écrits
// sur le réseau (après compression gzip le cas échéant — le compteur est
// monté au-dessus du compresseur) + une requête par requête servie, même
// sans corps (un 204 reste une requête). Les en-têtes HTTP ne sont pas
// mesurables côté application : le total est une BORNE BASSE du quota réel
// (~200-500 octets d'en-têtes par réponse en plus). Le reset est calé sur
// le jour UTC — la fenêtre de facturation Render est calée sur le mois
// calendaire UTC.
//
// Catégories (les cinq canaux du décompte N°72) :
//   - agents    : /agent/* — check-ins 45 s des routeurs (24 h/24) +
//     résultats de commandes + fichiers du portail hybride ;
//   - portail   : endpoints publics consommés par les INVITÉS WiFi
//     (page WiFi jetable, claim, status, track analytics,
//     config live du portail) + fichiers /portal/{token} ;
//   - medias    : /api/media/* — proxy d'images R2 (logo/pubs
//     personnalisés : ce trafic transite par Render) ;
//   - console   : le reste de /api/* — API authentifiée (JWT gérant,
//     équipe, revendeurs, webhooks de paiement entrants) ;
//   - autre     : tout le reste — santé GET /, 404 des bots/scanners
//     d'Internet qui frappent le domaine public.
//
// Verrou DÉDIÉ (jamais pris pendant un handler : les incrémentations se
// font au moment des Write, hors de tout verrou du store) : aucune
// contention mesurable, aucun interblocement possible.
package api

import (
	"net/http"
	"strings"
	"sync"
	"time"
)

// egressCategoryNames — catégories observées, ORDRE CANONIQUE du rapport
// (contrat stable pour la console et les tests : la liste est toujours
// complète, même si une catégorie n'a rien servi).
var egressCategoryNames = [...]string{"agents", "portail", "medias", "console", "autre"}

// egressCategory — classe un chemin de requête.
//
// NB : « /api/wifi/site/ » (préfixe AVEC slash final) désigne les
// endpoints publics des invités ; « /api/wifi/sites » et « /api/wifi/
// guests » (console) ne matchent PAS ce préfixe — même découpage que
// l'allowlist du middleware d'authentification.
func egressCategory(path string) string {
	switch {
	case strings.HasPrefix(path, "/agent/"):
		return "agents"
	case strings.HasPrefix(path, "/portal/"):
		return "portail"
	case path == "/api/portal/track":
		return "portail"
	case strings.HasPrefix(path, "/api/wifi/site/"):
		return "portail"
	case strings.HasPrefix(path, "/api/media/"):
		return "medias"
	case strings.HasPrefix(path, "/api/"):
		return "console"
	default:
		return "autre"
	}
}

// egressCat — compteurs d'UNE catégorie pour le jour courant.
type egressCat struct {
	requests int64
	bytes    int64
}

// egressStats — bande passante sortante du jour UTC courant.
type egressStats struct {
	mu   sync.Mutex
	day  string // « 2006-01-02 » UTC courant ("" = rien servi)
	cats map[string]*egressCat
}

func newEgressStats() *egressStats {
	return &egressStats{cats: map[string]*egressCat{}}
}

// startRequest — une requête est servie (comptée même sans octet : un
// 204/304 « zéro octet » reste une requête consommant des en-têtes).
func (e *egressStats) startRequest(cat string, now time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.rollDayLocked(now)
	c := e.cats[cat]
	if c == nil {
		c = &egressCat{}
		e.cats[cat] = c
	}
	c.requests++
}

// addBytes — octets de corps réellement écrits sur le réseau.
func (e *egressStats) addBytes(cat string, n int64, now time.Time) {
	if n <= 0 {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.rollDayLocked(now)
	c := e.cats[cat]
	if c == nil {
		c = &egressCat{}
		e.cats[cat] = c
	}
	c.bytes += n
}

// rollDayLocked — remise à zéro au changement de jour UTC.
func (e *egressStats) rollDayLocked(now time.Time) {
	day := now.UTC().Format("2006-01-02")
	if day != e.day {
		e.day = day
		e.cats = map[string]*egressCat{}
	}
}

// EgressCatSnapshot — une catégorie du rapport (contrat JSON camelCase).
type EgressCatSnapshot struct {
	Name     string `json:"name"`
	Requests int64  `json:"requests"`
	Bytes    int64  `json:"bytes"`
}

// EgressSnapshot — photographie du jour (bloc « bandwidth » de
// GET /api/admin/sync-status).
type EgressSnapshot struct {
	Day           string              `json:"day"` // jour UTC courant ("" = rien servi depuis le boot)
	TotalRequests int64               `json:"totalRequests"`
	TotalBytes    int64               `json:"totalBytes"`
	Categories    []EgressCatSnapshot `json:"categories"` // ordre canonique, toujours 5
}

// snapshot — copie cohérente (micro-verrou), catégories dans l'ordre
// canonique ; les absentes valent zéro (contrat stable).
func (e *egressStats) snapshot(now time.Time) EgressSnapshot {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.rollDayLocked(now)
	snap := EgressSnapshot{Day: e.day, Categories: make([]EgressCatSnapshot, 0, len(egressCategoryNames))}
	for _, name := range egressCategoryNames {
		s := EgressCatSnapshot{Name: name}
		if c := e.cats[name]; c != nil {
			s.Requests = c.requests
			s.Bytes = c.bytes
			snap.TotalRequests += c.requests
			snap.TotalBytes += c.bytes
		}
		snap.Categories = append(snap.Categories, s)
	}
	return snap
}

// observeEgress — middleware : catégorise la requête puis pèse chaque
// Write du corps de réponse. Monté au-dessus de la compression gzip : les
// octets comptés sont ceux réellement écrits sur le réseau.
func (a *API) observeEgress(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cat := egressCategory(r.URL.Path)
		a.egress.startRequest(cat, time.Now())
		ew := &egressWriter{ResponseWriter: w, cat: cat, stats: a.egress}
		next.ServeHTTP(ew, r)
	})
}

// egressWriter — ResponseWriter compteur : chaque Write informe le
// compteur de la catégorie de la requête en cours.
type egressWriter struct {
	http.ResponseWriter
	cat   string
	stats *egressStats
}

func (e *egressWriter) Write(p []byte) (int, error) {
	n, err := e.ResponseWriter.Write(p)
	if n > 0 {
		e.stats.addBytes(e.cat, int64(n), time.Now())
	}
	return n, err
}
