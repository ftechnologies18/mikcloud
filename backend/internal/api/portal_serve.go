// Package api — servage du portail captif aux routeurs (N°35).
//
// GET /portal/{token}/{path} sert le fichier {path} du template hotpage,
// personnalisé pour le compte propriétaire du routeur identifié par {token}.
// Le routeur agent fetch ce endpoint via /tool fetch au moment du déploiement
// (commande hotspot_files). Le token agent (32 car, haché côté cloud) auth la
// requête — pas de JWT, pas de cookie, pas de session : c'est un flux
// machine-à-machine pré-auth (avant le login du client du WiFi).
//
// Personnalisation : dans la phase initiale (N°35-a), le fichier est servi
// TEL QUEL depuis le template (hotpage.RawFile). La personnalisation par
// compte (branding, slug WiFi, lien join avec MAC, offres, textes) sera
// introduite en N°35-b/c via un moteur de templating léger (substitution de
// marqueurs {{MIKCLOUD_TENANT_NAME}}, {{MIKCLOUD_WIFI_SLUG}}, …). L'objectif
// est que login.html, au chargement côté client, fetch la config live via
// /api/wifi/site/{slug}/portal (portail hybride Cloud/Local avec fallback).
//
// Sécurité :
//   - token agent haché (routerByToken) → seul un routeur légitime peut fetch ;
//   - path sanitize (sanitizePortalPath côté agent + HasFile côté hotpage) :
//     pas de remontée de dossier, pas de fichier arbitraire ;
//   - TLS strict hérité (RouterOS ≥ 7.19 requis pour /agent/cmd, donc pour
//     /portal) — un routeur < 7.19 ne reçoit aucune commande, donc ne fetch rien ;
//   - Cache-Control: no-store (sécurité S1-A4) — le contenu est personnalisé
//     par compte, ne doit pas être mis en cache par un intermédiaire.
package api

import (
	"net/http"
	"path"
	"strings"

	"mikcloud/hotspot-api/internal/hotpage"
)

// handlePortalFile — sert un fichier du portail captif personnalisé pour le
// routeur (token). La route est /portal/{token}/{path...} — le path est
// capturé en multi-segment pour permettre css/bootstrap.min.css, js/typed.umd.js, etc.
func (a *API) handlePortalFile(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	if token == "" {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("# mikcloud: token manquant\n"))
		return
	}
	// Le path est le reste de l'URL après /portal/{token}/. On le reconstruit
	// à partir de r.URL.Path pour garder les slashes et l'extension.
	rawPath := strings.TrimPrefix(r.URL.Path, "/portal/"+token+"/")
	rawPath = strings.TrimPrefix(rawPath, "/")
	// sanitizePortalPath (côté agent) + HasFile (côté hotpage) : double
	// validation. Un path invalide ou absent → 404, jamais une 500.
	if strings.Contains(rawPath, "..") || strings.HasPrefix(rawPath, "/") || rawPath == "" {
		http.NotFound(w, r)
		return
	}
	// Vérifier le routeur sous verrou (court — on relache avant de servir).
	a.store.Lock()
	router := routerByToken(a.store.Data(), token)
	a.store.Unlock()
	if router == nil {
		// Token inconnu : 404 sans révéler la structure (anti-énumération).
		http.NotFound(w, r)
		return
	}
	if !hotpage.HasFile(rawPath) {
		http.NotFound(w, r)
		return
	}
	body, ok := hotpage.File(rawPath)
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", contentTypeFor(rawPath))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(body)
}

// contentTypeFor — Content-Type minimal pour les fichiers du portail. Les
// fichiers binaires ne sont pas servis par RawFile (cf. commentaire ci-dessus).
func contentTypeFor(p string) string {
	switch path.Ext(p) {
	case ".html":
		return "text/html; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".js":
		return "application/javascript; charset=utf-8"
	case ".txt":
		return "text/plain; charset=utf-8"
	case ".png":
		return "image/png"
	case ".ico":
		return "image/x-icon"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".woff2":
		return "font/woff2"
	case ".ttf":
		return "font/ttf"
	case ".svg":
		return "image/svg+xml"
	default:
		return "application/octet-stream"
	}
}
