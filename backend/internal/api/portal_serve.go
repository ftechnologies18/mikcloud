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
// N°182 — chaîne de personnalisation ROUTEUR → SITE → COMPTE : le branding
// servi n'est plus lu directement sur le tenant du compte, mais RÉSOLU par
// resolvePortalBranging : la surcharge individuelle du routeur (si posée)
// prime, sinon celle de son site (si assigné et surchargé), sinon le compte.
// Champ par champ, vide = hérite — un routeur sans site ni surcharge sert
// EXACTEMENT le portail du compte (rétrocompatibilité stricte).
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
	// Les fichiers binaires (png, ico, woff2, jpg) retournent false → servis
	// tels quels — aucun marqueur à substituer, gain de cycles et zéro risque
	// de corruption binaire.
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

// ---------------------------------------------------------------------------
// N°182 — chaîne de personnalisation ROUTEUR → SITE → COMPTE.
// ---------------------------------------------------------------------------

// portalBranding — le branding EFFECTIF du portail d'un routeur, après
// résolution de la chaîne N°182. Les listes restent en strings JSON (formats
// PERSISTÉS du tenant : [{id,title,desc,imageUrl,priceLabel,link}] pour les
// promos, ["msg"] pour le ticker, etc.) : les surcharges model.PortalOverride
// sont converties dans ces formats par applyPortalOverride — les helpers de
// décodage ci-dessous restent homéomorphes aux réglages du compte.
type portalBranding struct {
	DisplayName  string // nom affiché sur le portail (tenant.name sinon)
	LogoURL      string
	BannerURL    string
	WaveLink     string
	Style        string // "" | commercial | hospitality
	Welcome      string
	PromosJSON   string
	SocialsJSON  string
	ServicesJSON string
	TickerJSON   string
	WhatsappJSON string
	SlidesJSON   string
}

// resolvePortalBranding — part du branding du COMPTE (settings du compte acc),
// applique la surcharge du SITE du routeur (si assigné au compte, sinon
// ignoré — l'isolation multi-tenant se revérifie à CHAQUE résolution), puis
// la surcharge INDIVIDUELLE du routeur. router nil = branding du compte seul
// (fetch live d'un site WiFi au routeur introuvable — N°35-c). À appeler sous
// verrou (lit les settings et les sites).
func resolvePortalBranding(db *model.DB, acc string, router *model.Router) portalBranding {
	settings := ensureSettings(db, acc)
	t := settings.Tenant
	b := portalBranding{
		DisplayName:  t.Name,
		LogoURL:      t.LogoURL,
		BannerURL:    t.BannerURL,
		WaveLink:     t.WaveLink,
		Style:        t.PortalStyle,
		Welcome:      t.PortalWelcome,
		PromosJSON:   t.PortalPromos,
		SocialsJSON:  t.PortalSocials,
		ServicesJSON: t.PortalServices,
		TickerJSON:   t.PortalTicker,
		WhatsappJSON: t.PortalWhatsapp,
		SlidesJSON:   t.PortalSlides,
	}
	if router == nil {
		return b
	}
	if site := model.FindSiteScoped(db, router.SiteID, router.AccountID); site != nil {
		applyPortalOverride(&b, model.ParsePortalOverride(site.PortalOverride))
	}
	applyPortalOverride(&b, model.ParsePortalOverride(router.PortalOverride))
	return b
}

// applyPortalOverride — écrase champ par champ les valeurs NON VIDES de la
// surcharge. SÉMANTIQUE « VIDE = HÉRITE » (N°182) : un champ vide ne touche
// rien ; une liste vide non plus ; Whatsapp nil ou number vide non plus. La
// surcharge ne peut donc pas MASQUER un élément que le compte affiche — pour
// ce cas, vider le champ côté compte et le surcharger là où il doit
// apparaître (documenté console). Les listes structurées (types model miroirs
// des json tags persistés) sont re-sérialisées en JSON : les helpers de
// décodage aval ne font aucune différence entre un réglage du compte et une
// surcharge résolue.
func applyPortalOverride(b *portalBranding, ov model.PortalOverride) {
	if ov.DisplayName != "" {
		b.DisplayName = ov.DisplayName
	}
	if ov.LogoURL != "" {
		b.LogoURL = ov.LogoURL
	}
	if ov.BannerURL != "" {
		b.BannerURL = ov.BannerURL
	}
	if ov.WaveLink != "" {
		b.WaveLink = ov.WaveLink
	}
	if ov.Style != "" {
		b.Style = ov.Style
	}
	if ov.Welcome != "" {
		b.Welcome = ov.Welcome
	}
	if len(ov.Promos) > 0 {
		b.PromosJSON = mustJSON(ov.Promos)
	}
	if len(ov.Socials) > 0 {
		b.SocialsJSON = mustJSON(ov.Socials)
	}
	if len(ov.Services) > 0 {
		b.ServicesJSON = mustJSON(ov.Services)
	}
	if len(ov.Slides) > 0 {
		b.SlidesJSON = mustJSON(ov.Slides)
	}
	if len(ov.Ticker) > 0 {
		b.TickerJSON = mustJSON(ov.Ticker)
	}
	if ov.Whatsapp != nil && ov.Whatsapp.Number != "" {
		b.WhatsappJSON = mustJSON(ov.Whatsapp)
	}
}

// mustJSON — marshaling des types simples de surcharge (échec impossible en
// pratique ; repli chaîne vide = hérite, jamais de portail cassé).
func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

// portalHospitalityLists — décode le branding hospitalité (N°55) depuis les
// strings JSON PERSISTÉS (réglage du compte OU surcharge N°182 résolue —
// formats identiques). Les listes invalides ou vides donnent des listes
// vides — la page reste cohérente (mode commercial ou hospitalité sans
// vitrine), jamais cassée.
func portalHospitalityLists(promosJSON, socialsJSON string) (promos []hotpage.PortalPromo, socials []hotpage.PortalSocial) {
	if promosJSON != "" {
		var raw []struct {
			ID         string `json:"id"`
			Title      string `json:"title"`
			Desc       string `json:"desc"`
			ImageURL   string `json:"imageUrl"`
			PriceLabel string `json:"priceLabel"`
			Link       string `json:"link"`
		}
		if json.Unmarshal([]byte(promosJSON), &raw) == nil {
			for _, it := range raw {
				// N°56 — chaque promo part avec un ID : celui posé à
				// l'enregistrement console (stable), ou à défaut un id
				// déterministe dérivé du contenu (lignes héritées d'avant
				// N°56, jamais ré-enregistrées). Sans id, la carte resterait
				// hors analytics — le gérant perdrait ses compteurs jusqu'au
				// prochain enregistrement.
				id := it.ID
				if id == "" {
					id = promoFallbackID(it.Title, it.Desc, it.ImageURL, it.PriceLabel)
				}
				promos = append(promos, hotpage.PortalPromo{ID: id, Title: it.Title, Desc: it.Desc, ImageURL: it.ImageURL, PriceLabel: it.PriceLabel, Link: it.Link})
			}
		}
	}
	if socialsJSON != "" {
		var raw []struct {
			Label string `json:"label"`
			URL   string `json:"url"`
		}
		if json.Unmarshal([]byte(socialsJSON), &raw) == nil {
			for _, it := range raw {
				socials = append(socials, hotpage.PortalSocial{Label: it.Label, URL: it.URL})
			}
		}
	}
	return promos, socials
}

// portalSlidesList — décode les slides du carrousel COMMERCIAL (N°136) depuis
// la string JSON persistée : JSON ["url",…] ≤ 3 URLs https. Un JSON invalide,
// vide ou une liste vide donne nil — la page garde ses 3 images génériques
// (jamais cassée). Le plafond est re-vérifié au décodage (défense en
// profondeur : une ligne héritée d'un appel API direct ne peut pas gonfler le
// carrousel).
func portalSlidesList(s string) []string {
	if s == "" {
		return nil
	}
	var raw []string
	if json.Unmarshal([]byte(s), &raw) != nil {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, u := range raw {
		if u = strings.TrimSpace(u); u != "" && strings.HasPrefix(u, "https://") {
			out = append(out, u)
		}
		if len(out) >= 3 {
			break
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// portalServicesList — décode les services de l'établissement (N°137, section
// « Nos Services » du portail commercial) depuis la string JSON persistée :
// un JSON invalide ou un champ vide donne une liste vide — la section est
// masquée côté page, jamais cassée (même robustesse que portalHospitalityLists).
func portalServicesList(s string) []hotpage.PortalService {
	if s == "" {
		return nil
	}
	var raw []struct {
		Icon  string `json:"icon"`
		Label string `json:"label"`
	}
	if json.Unmarshal([]byte(s), &raw) != nil {
		return nil
	}
	var out []hotpage.PortalService
	for _, it := range raw {
		if strings.TrimSpace(it.Label) == "" {
			continue // ligne incomplète (héritée d'un import manuel) → ignorée
		}
		out = append(out, hotpage.PortalService{Icon: it.Icon, Label: it.Label})
	}
	return out
}

// portalTickerList — décode les messages du bandeau animé sous le logo
// (N°138, effet Typed.js du login.html) depuis la string JSON persistée :
// un JSON invalide ou un champ vide donne nil — le template garde ses 3
// messages historiques (jamais cassé). Le plafond (5) et la longueur (80)
// sont re-vérifiés au décodage (défense en profondeur).
func portalTickerList(s string) []string {
	if s == "" {
		return nil
	}
	var raw []string
	if json.Unmarshal([]byte(s), &raw) != nil {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, m := range raw {
		if m = strings.TrimSpace(m); m != "" && len(m) <= 80 {
			out = append(out, m)
		}
		if len(out) >= 5 {
			break
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// portalWhatsappInfo — décode le numéro WhatsApp SUPPORT (N°139, lien du
// footer login/logout/error) depuis la string JSON persistée : un JSON
// invalide, un champ vide ou un numéro mal formé donnent nil — le portail
// garde le numéro du support MikCloud (repli historique, jamais cassé). Le
// format est revalidé au décodage via hotpage.WhatsappNumber (défense en
// profondeur).
func portalWhatsappInfo(s string) *hotpage.PortalWhatsapp {
	if s == "" {
		return nil
	}
	var raw struct {
		Number string `json:"number"`
		Label  string `json:"label"`
	}
	if json.Unmarshal([]byte(s), &raw) != nil {
		return nil
	}
	d := hotpage.WhatsappNumber(raw.Number)
	if d == "" {
		return nil
	}
	return &hotpage.PortalWhatsapp{Number: d, Label: strings.TrimSpace(raw.Label)}
}

// buildPortalConfig — construit le PortalConfig pour le compte propriétaire
// du routeur, à partir du store. À appeler SOUS VERROU (lit db.SettingsByAccount,
// db.Sites, db.WifiSites, db.JoinLinks, db.Profiles).
//
// N°182 — le branding passe par resolvePortalBranding (chaîne ROUTEUR →
// SITE → COMPTE, champ par champ). Les résolutions LIÉES AU ROUTEUR sont
// inchangées :
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
//     Wave EFFECTIF (N°182 : la surcharge site/routeur prime sur le compte).
func buildPortalConfig(db *model.DB, router *model.Router, r *http.Request) hotpage.PortalConfig {
	acc := router.AccountID
	settings := ensureSettings(db, acc) // défauts si absent
	b := resolvePortalBranding(db, acc, router)
	cfg := hotpage.PortalConfig{
		TenantName: b.DisplayName,
		APIBase:    agentBaseURL(r),
		WaveLink:   b.WaveLink,
		LogoURL:    b.LogoURL,
		BannerURL:  b.BannerURL,
		// N°46 — l'affichage du bouton « S'inscrire » est piloté par le
		// réglage console (défaut effectif ON pour les comptes existants).
		JoinEnabled: settings.Tenant.JoinButtonEnabled(),
		// N°65 — rétention du journal du compte (note de confidentialité).
		LogRetentionDays: settings.Tenant.LogRetentionDaysEffective(),
	}
	cfg.Style, cfg.Welcome = b.Style, b.Welcome
	cfg.Promos, cfg.Socials = portalHospitalityLists(b.PromosJSON, b.SocialsJSON) // N°55
	cfg.Slides = portalSlidesList(b.SlidesJSON)                                   // N°136 — carrousel commercial
	cfg.PortalKey = settings.Tenant.PortalKey                                     // N°56 — analytics pré-auth
	cfg.Services = portalServicesList(b.ServicesJSON)                             // N°137 — section « Nos Services »
	cfg.Ticker = portalTickerList(b.TickerJSON)                                   // N°138 — bandeau animé sous le logo
	cfg.Whatsapp = portalWhatsappInfo(b.WhatsappJSON)                             // N°139 — support WhatsApp du footer
	// WifiSlug — 1er site WiFi actif lié à ce routeur.
	for i := range db.WifiSites {
		s := &db.WifiSites[i]
		if s.AccountID == acc && s.RouterID == router.ID && s.Active {
			cfg.WifiSlug = s.Slug
			cfg.Active = true // N°51 — site actif lié trouvé → carte claim affichée
			// N°69 — l'interrupteur de consentement marketing du
			// claim portail suit le réglage du site (fallback inliné
			// + endpoint live, même pilotage que la carte claim).
			cfg.MarketingOptIn = s.MarketingOptIn
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
		// (cf. handlers_subscription.go wavePayLink). N°182 : waveLink EFFECTIF
		// (surcharge site/routeur comprise). Vide si aucun lien marchand.
		if b.WaveLink != "" {
			offer.WaveURL = strings.TrimRight(b.WaveLink, "/") + "/amount/" + strconv.Itoa(p.Price) + "/"
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
// routeur). N°182 : le branding passe par la même chaîne ROUTEUR → SITE →
// COMPTE (routeur nil = branding du compte).
//
// Note : le PortalConfig renvoyé ne contient PAS de secret (pas de token
// agent, pas de mots de passe). C'est la même structure que celle inlinée
// dans login.html au déploiement — cohérente avec le pattern fallback/live.
func buildPortalConfigForSite(db *model.DB, site *model.WifiSite, router *model.Router, r *http.Request) hotpage.PortalConfig {
	acc := site.AccountID
	settings := ensureSettings(db, acc)
	b := resolvePortalBranding(db, acc, router)
	cfg := hotpage.PortalConfig{
		TenantName: b.DisplayName,
		APIBase:    agentBaseURL(r),
		WaveLink:   b.WaveLink,
		LogoURL:    b.LogoURL,
		BannerURL:  b.BannerURL,
		// N°46 — même pilotage que le fallback inliné : le fetch live
		// prime sur le fallback, le réglage s'applique donc sans
		// re-déploiement sur les portails des routeurs déjà déployés.
		JoinEnabled: settings.Tenant.JoinButtonEnabled(),
		// N°65 — rétention du journal du compte (note de confidentialité).
		LogRetentionDays: settings.Tenant.LogRetentionDaysEffective(),
		// N°69 — interrupteur de consentement marketing du claim portail
		// (état réel du site, même pilotage que Active).
		MarketingOptIn: site.MarketingOptIn,
		WifiSlug:       site.Slug,
		Active:         site.Active, // N°51 — état réel (peut être en pause)
	}
	cfg.Style, cfg.Welcome = b.Style, b.Welcome
	cfg.Promos, cfg.Socials = portalHospitalityLists(b.PromosJSON, b.SocialsJSON) // N°55
	cfg.Slides = portalSlidesList(b.SlidesJSON)                                   // N°136 — carrousel commercial
	cfg.PortalKey = settings.Tenant.PortalKey                                     // N°56 — analytics pré-auth
	cfg.Services = portalServicesList(b.ServicesJSON)                             // N°137 — section « Nos Services »
	cfg.Ticker = portalTickerList(b.TickerJSON)                                   // N°138 — bandeau animé sous le logo
	cfg.Whatsapp = portalWhatsappInfo(b.WhatsappJSON)                             // N°139 — support WhatsApp du footer
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
		// N°182 — waveLink EFFECTIF (surcharge site/routeur comprise).
		if b.WaveLink != "" {
			offer.WaveURL = strings.TrimRight(b.WaveLink, "/") + "/amount/" + strconv.Itoa(p.Price) + "/"
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
	// En l'absence de APP_PUBLIC_URL, on ne peut pas deviner l'origine du
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
