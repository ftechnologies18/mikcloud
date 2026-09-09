// gzip.go — compression HTTP des réponses textuelles (N°72).
//
// Contexte : le plan gratuit Render (workspace Hobby) alloue 5 Go/mois de
// bande passante sortante. Le quota a été épuisé en ~9 jours avec deux
// clients en essai → suspension automatique de TOUS les services du
// workspace (email Render « Workspace suspended — free bandwidth »). Or le
// backend servait TOUTES ses réponses en clair : JSON console, portail
// captif, exports CSV, factures HTML. La compression gzip divise par 4 à 8
// le volume de ces réponses textuelles — c'est le levier le plus simple et
// le plus transversal du plan « tenir le plancher free » (décompte N°72 :
// consoles, médias proxifiés, check-ins agents 24/7, status polling des
// invités, bots Internet).
//
// Règles de la compression :
//   - UNIQUEMENT si le client annonce « Accept-Encoding: gzip » (les
//     navigateurs le font ; l'agent RouterOS /tool fetch ne le fait pas →
//     le protocole texte des check-ins 45 s reste strictement inchangé,
//     compatible routeurs comme aujourd'hui) ;
//   - UNIQUEMENT les types compressibles (texte, JSON, JS, CSS, SVG) —
//     jamais les images ni les polices (déjà compressées : gzip n'y
//     gagnerait rien et brûlerait du CPU) ;
//   - les statuts SANS corps (204 No Content du track analytics portail,
//     304) ne passent jamais par le compresseur ;
//   - « Vary: Accept-Encoding » posé (Add — le CORS peut déjà avoir posé
//     « Vary: Origin ») pour que les caches intermédiaires distinguent les
//     variantes, et « Content-Length » supprimé (la longueur change) ;
//   - décision au PREMIER Write : le Content-Type est posé par writeJSON /
//     serveFile AVANT l'écriture (convention du code) ; à défaut il est
//     reniflé sur les premiers octets (http.DetectContentType).
//
// Positionnement : inséré dans API.Handler() SOUS authMiddleware (les 401
// du middleware sont compressées aussi) et AU-DESSUS du compteur egress
// (N°72, httpstats.go) — le compteur voit donc les octets réellement
// écrits sur le réseau, c'est-à-dire compressés.
package api

import (
	"compress/gzip"
	"net/http"
	"strings"
	"sync"
)

// gzipPool — réutilise les compresseurs (un gzip.Writer embarque un tampon
// et des tables Huffman) : une requête compressée ne coûte qu'un Reset.
var gzipPool = sync.Pool{
	New: func() any { return gzip.NewWriter(nil) },
}

// clientAcceptsGzip — true si l'en-tête Accept-Encoding cite gzip (avec ou
// sans paramètre de qualité, ex. « gzip;q=0.8 » ou « br, gzip »).
func clientAcceptsGzip(r *http.Request) bool {
	for _, part := range strings.Split(r.Header.Get("Accept-Encoding"), ",") {
		enc := strings.TrimSpace(part)
		if i := strings.IndexByte(enc, ';'); i >= 0 {
			enc = strings.TrimSpace(enc[:i])
		}
		if enc == "gzip" {
			return true
		}
	}
	return false
}

// compressibleContentType — types où gzip gagne réellement (texte). Tout le
// reste (images, polices, binaire) sort en clair.
func compressibleContentType(ct string) bool {
	for _, prefix := range []string{
		"text/",
		"application/json",
		"application/javascript",
		"application/x-javascript",
		"application/xml",
		"image/svg+xml",
	} {
		if strings.HasPrefix(ct, prefix) {
			return true
		}
	}
	return false
}

// gzipMiddleware — enveloppe le mux : compresse les réponses compressibles
// des clients qui le demandent, délègue tout le reste tel quel.
func gzipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !clientAcceptsGzip(r) {
			next.ServeHTTP(w, r)
			return
		}
		gw := &gzipWriter{ResponseWriter: w}
		defer gw.close()
		next.ServeHTTP(gw, r)
	})
}

// gzipWriter — décide de la compression au PREMIER Write ; le statut
// reçu via WriteHeader est RETENU jusqu'à cette décision. En effet la
// convention du code (writeJSON, serveFile) appelle WriteHeader AVANT
// d'écrire le corps : si le statut engageait immédiatement la réponse,
// les en-têtes de compression (Content-Encoding, Vary, Content-Length)
// posés ensuite seraient silencieusement IGNORÉS par net/http — le corps
// partirait compressé sans en-tête, illisible pour le client. Retenir le
// statut garantit que la réponse part avec ses en-têtes définitifs.
type gzipWriter struct {
	http.ResponseWriter
	gz      *gzip.Writer // nil tant que la compression n'est pas activée
	decided bool         // la décision (compresser ou non) est prise
	status  int          // statut retenu (0 = non reçu / déjà émis)
}

// WriteHeader — les statuts sans corps (204 No Content du track
// analytics, 304) et les 1xx partent immédiatement en clair : rien à
// compresser. Tout autre statut est RETENU (émis au Write ou au close,
// avec les en-têtes définitifs).
func (g *gzipWriter) WriteHeader(code int) {
	if code == http.StatusNoContent || code == http.StatusNotModified || code < http.StatusOK {
		g.decided = true
		g.status = 0
		g.ResponseWriter.WriteHeader(code)
		return
	}
	g.status = code
}

// emitStatus — émet le statut retenu, une seule fois.
func (g *gzipWriter) emitStatus() {
	if g.status != 0 {
		code := g.status
		g.status = 0
		g.ResponseWriter.WriteHeader(code)
	}
}

// Write — premier appel : décide selon le Content-Type (posé par le
// handler ; sinon reniflé sur les octets ; rien à écrire → en clair),
// pose les en-têtes de variante, émet le statut retenu, puis route les
// octets vers le compresseur ou la sortie directe.
func (g *gzipWriter) Write(p []byte) (int, error) {
	if !g.decided {
		g.decided = true
		ct := g.Header().Get("Content-Type")
		if ct == "" {
			if len(p) == 0 {
				// Rien à écrire : rien à compresser.
				return g.ResponseWriter.Write(p)
			}
			ct = http.DetectContentType(p)
		}
		if compressibleContentType(ct) {
			g.Header().Add("Vary", "Accept-Encoding")
			g.Header().Del("Content-Length") // la longueur change avec gzip
			g.Header().Set("Content-Encoding", "gzip")
			g.gz, _ = gzipPool.Get().(*gzip.Writer)
			g.gz.Reset(g.ResponseWriter)
		}
		g.emitStatus()
	}
	if g.gz == nil {
		return g.ResponseWriter.Write(p)
	}
	return g.gz.Write(p)
}

// Flush — vide le compresseur puis délègue : aucun flux temps réel
// n'existe aujourd'hui (polling partout), la méthode n'est là que pour
// ne pas briser un futur handler qui demanderait http.Flusher.
func (g *gzipWriter) Flush() {
	if g.gz != nil {
		_ = g.gz.Flush()
	}
	if f, ok := g.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// close — émet le statut retenu (handler muet : WriteHeader sans
// corps écrit), ferme le compresseur (CRC final) et le rend au pool.
// Sans compression activée : simple émission du statut retenu.
func (g *gzipWriter) close() {
	g.decided = true
	g.emitStatus()
	if g.gz != nil {
		_ = g.gz.Close()
		gzipPool.Put(g.gz)
		g.gz = nil
	}
}
