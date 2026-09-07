// Package api — servage du portail captif aux routeurs (N°35).
//
// GET /portal/{token}/{path} sert le fichier {path} du template hotpage,
// personnalisé pour le compte propriétaire du routeur identifié par {token}.
// Le routeur agent fetch ce endpoint via /tool fetch au moment du déploiement
// (commande hotspot_files). Le token agent (32 car, haché côté cloud) auth la
// requête — pas de JWT, pas de cookie, pas de session : c'est un flux
// machine-à-machine pré-auth (avant le login du client du WiFi).
//
// Personnalisation (N°35-b) : les fichiers TEXTE (HTML, JS, txt) sont passés
// par hotpage.Personalize qui substitue les marqueurs {{MIKCLOUD_*}} par les
// valeurs du compte (tenant, slug WiFi, lien join, offres, API base, wave link).
// Les fichiers BINAIRES (png, ico, woff2, jpg) sont servis tels quels — aucun
// marqueur à substituer, gain de cycles. La config complète est embarquée dans
// login.html/status.html via un bloc <script type="application/json"
// id="mikcloud-config">{{MIKCLOUD_CONFIG_JSON}}</script> que la page lit côté
// client (N°35-c pour la consommation hybride fetch/fallback).
//
// Sécurité :
//   - token agent haché (routerByToken) → seul un routeur légitime peut fetch ;
//   - path sanitize (sanitizePortalPath côté agent + HasFile côté hotpage) :
//     pas de remontée de dossier, pas de fichier arbitraire ;
//   - TLS strict hérité (RouterOS ≥ 7.19 requis pour /agent/cmd, donc pour
//     /portal) — un routeur < 7.19 ne reçoit aucune commande, donc ne fetch rien ;
//   - Cache-Control: no-store (sécurité S1-A4) — le contenu est personnalisé
//     par compte, ne doit pas être mis en cache par un intermédiaire.
//   - Échappement strict des valeurs (html.EscapeString + encoding/json avec
//     SetEscapeHTML par défaut) → aucune injection XSS possible via le nom du
//     tenant ou autre.
package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"

	"mikcloud/hotspot-api/internal/hotpage"
	"mikcloud/hotspot-api/internal/model"
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

	// Résoudre le routeur ET son compte, puis construire le PortalConfig
	// en UN seul passage sous verrou (évite le re-lock pour lire les settings).
	a.store.Lock()
	db := a.store.Data()
	router := routerByToken(db, token)
	if router == nil {
		a.store.Unlock()
		// Token inconnu : 404 sans révéler la structure (anti-énumération).
		http.NotFound(w, r)
		return
	}
	cfg := buildPortalConfig(db, router, r)
	a.store.Unlock()

	if !hotpage.HasFile(rawPath) {
		http.NotFound(w, r)
		return
	}
	body, ok := hotpage.File(rawPath)
	if !ok {
		http.NotFound(w, r)
		return
	}
	// N°35-b — personnalisation : les fichiers texte sont passés par Personalize.
	// Les fichiers binaires (png, ico, woff2, jpg) sont servis tels quels — aucun
	// marqueur à substituer, gain de cycles et zéro risque de corruption binaire.
	if isTextAsset(rawPath) {
		body = []byte(hotpage.Personalize(string(body), cfg))
	}
	w.Header().Set("Content-Type", contentTypeFor(rawPath))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(body)
}

// isTextAsset — true si le path est un fichier texte (HTML, CSS, JS, txt) qui
// doit être passé par Personalize. Les fichiers binaires (png, ico, woff2, ttf,
// jpg) retournent false → servis tels quels.
func isTextAsset(p string) bool {
	switch path.Ext(p) {
	case ".html", ".css", ".js", ".txt":
		return true
	}
	return false
}

// portalHospitality — décode le branding hospitalité du tenant (N°55).
// Les listes sont persistées en JSON (model.Tenant.PortalPromos/Socials) :
// un JSON invalide ou un champ vide donne des listes vides — la page reste
// cohérente (mode commercial ou hospitalité sans vitrine), jamais cassée.
func portalHospitality(t model.Tenant) (style, welcome string, promos []hotpage.PortalPromo, socials []hotpage.PortalSocial) {
	style, welcome = t.PortalStyle, t.PortalWelcome
	if t.PortalPromos != "" {
		var raw []struct {
			ID         string `json:"id"`
			Title      string `json:"title"`
			Desc       string `json:"desc"`
			ImageURL   string `json:"imageUrl"`
			PriceLabel string `json:"priceLabel"`
			Link       string `json:"link"`
		}
		if json.Unmarshal([]byte(t.PortalPromos), &raw) == nil {
			for _, it := range raw {
				// N°56 — chaque promo part avec un ID : celui posé à
				// l'enregistrement console (stable), ou à défaut un id
				// déterministe dérivé du contenu (lignes héritées
				// d'avant N°56, jamais ré-enregistrées). Sans id, la
				// carte resterait hors analytics — le gérant perdrait
				// ses compteurs jusqu'au prochain enregistrement.
				id := it.ID
				if id == "" {
					id = promoFallbackID(it.Title, it.Desc, it.ImageURL, it.PriceLabel)
				}
				promos = append(promos, hotpage.PortalPromo{ID: id, Title: it.Title, Desc: it.Desc, ImageURL: it.ImageURL, PriceLabel: it.PriceLabel, Link: it.Link})
			}
		}
	}
	if t.PortalSocials != "" {
		var raw []struct {
			Label string `json:"label"`
			URL   string `json:"url"`
		}
		if json.Unmarshal([]byte(t.PortalSocials), &raw) == nil {
			for _, it := range raw {
				socials = append(socials, hotpage.PortalSocial{Label: it.Label, URL: it.URL})
			}
		}
	}
	return style, welcome, promos, socials
}

// buildPortalConfig — construit le PortalConfig pour le compte propriétaire
// du routeur, à partir du store. À appeler SOUS VERROU (lit db.SettingsByAccount,
// db.WifiSites, db.JoinLinks, db.Profiles).
//
// Raisonnement sur la résolution des entités liées au routeur :
//   - WifiSlug : on cherche le site WiFi jetable du compte qui est LIÉ à ce
//     routeur (WifiSite.RouterID == router.ID) ET actif. Si plusieurs, on prend
//     le 1er (ordre d'itération du store). Si aucun, on laisse WifiSlug vide
//     et Active reste false (N°51) — la page ne pose pas la carte claim (ou
//     la retire si un fallback inliné périmé l'avait posée).
//   - JoinURL : on cherche le lien d'inscription publique du compte qui est
//     LIÉ à ce routeur (JoinLink.RouterID == router.ID) ET actif (pas révoqué,
//     pas expiré, pas épuisé). Si plusieurs, on prend le 1er. Si aucun, on
//     laisse vide — la page cachera le bloc inscription.
//   - Offers : on prend les profils du compte à prix > 0 (max 8, ordre
//     d'itération), on construit le waveUrl pré-construit via le lien marchand
//     Wave du tenant (si configuré).
func buildPortalConfig(db *model.DB, router *model.Router, r *http.Request) hotpage.PortalConfig {
	acc := router.AccountID
	settings := ensureSettings(db, acc) // défauts si absent
	cfg := hotpage.PortalConfig{
		TenantName: settings.Tenant.Name,
		APIBase:    agentBaseURL(r),
		WaveLink:   settings.Tenant.WaveLink,
		LogoURL:    settings.Tenant.LogoURL,
		BannerURL:  settings.Tenant.BannerURL,
		// N°46 — l'affichage du bouton « S'inscrire » est piloté par le
		// réglage console (défaut effectif ON pour les comptes existants).
		JoinEnabled: settings.Tenant.JoinButtonEnabled(),
	}
	cfg.Style, cfg.Welcome, cfg.Promos, cfg.Socials = portalHospitality(settings.Tenant) // N°55
	cfg.PortalKey = settings.Tenant.PortalKey                                            // N°56 — analytics pré-auth
	// WifiSlug — 1er site WiFi actif lié à ce routeur.
	for i := range db.WifiSites {
		s := &db.WifiSites[i]
		if s.AccountID == acc && s.RouterID == router.ID && s.Active {
			cfg.WifiSlug = s.Slug
			cfg.Active = true // N°51 — site actif lié trouvé → carte claim affichée
			// WifiURL — construit à partir de l'origine publique (frontend Vercel).
			// Pour l'instant, on dérive du Host de la requête si c'est une origine
			// connue (mikcloud.ftci.fr), sinon on laisse vide (la page utilisera
			// l'APIBase pour construire le lien relatif /wifi/{slug}).
			if origin := publicFrontendURL(r); origin != "" {
				cfg.WifiURL = origin + "/wifi/" + s.Slug
			}
			// Quota gratuit effectif (0 site = hériter du profil) —
			// exposé au fallback inliné comme à l'endpoint live.
			if profile := findProfileScoped(db, s.ProfileID, acc); profile != nil {
				cfg.FreeTimeMin, cfg.FreeDataMb = wifiQuotaResp(s, profile)
			}
			break
		}
	}
	// JoinURL — 1er lien d'inscription publique actif lié à ce routeur.
	for i := range db.JoinLinks {
		l := &db.JoinLinks[i]
		if l.AccountID == acc && l.RouterID == router.ID && !l.Revoked && joinLinkActive(l) {
			if origin := publicFrontendURL(r); origin != "" {
				cfg.JoinURL = origin + "/join/" + l.Token
			}
			break
		}
	}
	// Offers — profils à prix > 0 (max 8).
	for i := range db.Profiles {
		p := &db.Profiles[i]
		if p.AccountID != acc || p.Price <= 0 {
			continue
		}
		if len(cfg.Offers) >= 8 {
			break
		}
		offer := hotpage.PortalOffer{
			Name:        p.Name,
			PriceFcfa:   p.Price,
			ValidityMin: p.ValidityMinutes(),
			DataQuotaMb: p.DataQuotaMb,
		}
		// WaveURL — deep-link Wave pré-construit : {waveLink}/amount/{priceFcfa}/
		// (cf. handlers_subscription.go wavePayLink). Vide si le tenant n'a pas
		// configuré son lien marchand Wave.
		if settings.Tenant.WaveLink != "" {
			offer.WaveURL = strings.TrimRight(settings.Tenant.WaveLink, "/") + "/amount/" + strconv.Itoa(p.Price) + "/"
		}
		cfg.Offers = append(cfg.Offers, offer)
	}
	return cfg
}

// joinLinkActive — true si le lien n'est ni révoqué, ni expiré, ni épuisé.
// Reflète la logique de joinLinkState (handlers_join.go) sans la dépendance
// au temps humain (on utilise model.NowISO).
func joinLinkActive(l *model.JoinLink) bool {
	if l.Revoked {
		return false
	}
	if l.ExpiresAt != "" && l.ExpiresAt < model.NowISO() {
		return false
	}
	if l.MaxUses > 0 && l.Uses >= l.MaxUses {
		return false
	}
	return true
}

// buildPortalConfigForSite — N°35-c : variante de buildPortalConfig par SLUG
// de site WiFi (au lieu de par token agent). Utilisée par handleWifiPortal
// (endpoint public /api/wifi/site/{slug}/portal) pour exposer la config LIVE
// au portail routeur au chargement. Le site WiFi est résolu par son slug
// (global, unique) ; le routeur est retrouvé via site.RouterID.
//
// Différence avec buildPortalConfig : ici on a déjà le site (résolu par slug),
// pas besoin de le chercher. Le wifiSlug et le wifiURL sont TOUJOURS peuplés
// (le site existe, sinon 404 déjà renvoyé). N°51 : Active reflète l'état
// RÉEL du site — handleWifiPortal n'appelle ce builder avec un site en
// pause que si le routeur est introuvable (sinon il bascule sur
// buildPortalConfig, qui résout le 1er site actif du routeur).
// Le joinURL est résolu via le routeur lié au site (1er JoinLink actif du
// routeur).
//
// Note : le PortalConfig renvoyé ne contient PAS de secret (pas de token
// agent, pas de mots de passe). C'est la même structure que celle inlinée
// dans login.html au déploiement — cohérente avec le pattern fallback/live.
func buildPortalConfigForSite(db *model.DB, site *model.WifiSite, router *model.Router, r *http.Request) hotpage.PortalConfig {
	acc := site.AccountID
	settings := ensureSettings(db, acc)
	cfg := hotpage.PortalConfig{
		TenantName: settings.Tenant.Name,
		APIBase:    agentBaseURL(r),
		WaveLink:   settings.Tenant.WaveLink,
		LogoURL:    settings.Tenant.LogoURL,
		BannerURL:  settings.Tenant.BannerURL,
		// N°46 — même pilotage que le fallback inliné : le fetch live
		// prime sur le fallback, le réglage s'applique donc sans
		// re-déploiement sur les portails des routeurs déjà déployés.
		JoinEnabled: settings.Tenant.JoinButtonEnabled(),
		WifiSlug:    site.Slug,
		Active:      site.Active, // N°51 — état réel (peut être en pause)
	}
	cfg.Style, cfg.Welcome, cfg.Promos, cfg.Socials = portalHospitality(settings.Tenant) // N°55
	cfg.PortalKey = settings.Tenant.PortalKey                                            // N°56 — analytics pré-auth
	if origin := publicFrontendURL(r); origin != "" {
		cfg.WifiURL = origin + "/wifi/" + site.Slug
	}
	// Quota gratuit effectif (0 site = hériter du profil) — servit par
	// l'endpoint live GET /api/wifi/site/{slug}/portal.
	if profile := findProfileScoped(db, site.ProfileID, acc); profile != nil {
		cfg.FreeTimeMin, cfg.FreeDataMb = wifiQuotaResp(site, profile)
	}
	// JoinURL — 1er lien d'inscription publique actif lié au routeur.
	if router != nil {
		for i := range db.JoinLinks {
			l := &db.JoinLinks[i]
			if l.AccountID == acc && l.RouterID == router.ID && !l.Revoked && joinLinkActive(l) {
				if origin := publicFrontendURL(r); origin != "" {
					cfg.JoinURL = origin + "/join/" + l.Token
				}
				break
			}
		}
	}
	// Offers — profils à prix > 0 (max 8).
	for i := range db.Profiles {
		p := &db.Profiles[i]
		if p.AccountID != acc || p.Price <= 0 {
			continue
		}
		if len(cfg.Offers) >= 8 {
			break
		}
		offer := hotpage.PortalOffer{
			Name:        p.Name,
			PriceFcfa:   p.Price,
			ValidityMin: p.ValidityMinutes(),
			DataQuotaMb: p.DataQuotaMb,
		}
		if settings.Tenant.WaveLink != "" {
			offer.WaveURL = strings.TrimRight(settings.Tenant.WaveLink, "/") + "/amount/" + strconv.Itoa(p.Price) + "/"
		}
		cfg.Offers = append(cfg.Offers, offer)
	}
	return cfg
}

// publicFrontendURL — l'origine publique du frontend Vercel (pour construire
// les URL /wifi/{slug} et /join/{token}). En production, le frontend est sur
// mikcloud.ftci.fr, le backend sur mikcloud.onrender.com : ce sont DEUX hôtes
// distincts. On dérive l'origine du frontend à partir de APP_PUBLIC_URL (env)
// si défini, sinon de l'origine de la requête courante si elle semble être le
// frontend (rare — les requêtes /portal/ viennent des routeurs, pas des
// navigateurs), sinon on laisse vide (la page utilisera l'APIBase pour
// construire les liens en relatif).
func publicFrontendURL(r *http.Request) string {
	if v := strings.TrimSpace(getEnv("APP_PUBLIC_URL")); v != "" {
		return strings.TrimRight(v, "/")
	}
	// En l'absence d'APP_PUBLIC_URL, on ne peut pas deviner l'origine du
	// frontend (Render ≠ Vercel). On retourne "" : la page devra utiliser
	// l'APIBase (backend Render) pour construire les liens relatifs /wifi/{slug}
	// et /join/{token} — le backend Render redirige ou proxie vers le frontend.
	// C'est un compromis acceptable pour la phase initiale (N°35-b) ; le
	// mécanisme sera affiné quand le portail hybride (N°35-c) aura besoin de
	// liens absolus pour le QR code imprimé.
	return ""
}

// getEnv — wrapper os.Getenv pour faciliter le mock en tests.
func getEnv(key string) string {
	return osGetEnv(key)
}

// osGetEnv — indirection pour permettre le mock en tests (sans dépendre de
// os.Getenv directement, ce qui rendrait buildPortalConfig non testable).
var osGetEnv = osGetEnvReal

// osGetEnvReal — implémentation réelle de os.Getenv.
func osGetEnvReal(key string) string {
	return os.Getenv(key)
}

// contentTypeFor — Content-Type minimal pour les fichiers du portail.
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
