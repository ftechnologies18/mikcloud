// Package hotpage — templating par compte au moment du SERVE (N°35-b).
//
// Personalize substitue les marqueurs {{MIKCLOUD_*}} d'un fichier du template
// par les valeurs personnalisées pour le compte propriétaire du routeur qui
// fetch le portail. Les fichiers binaires (png, ico, woff2, jpg) ne sont JAMAIS
// touchés — ils sont servis tels quels par File().
//
// Marqueurs supportés (substitution simple, sensible à la casse) :
//
//	{{MIKCLOUD_CONFIG_JSON}}  — bloc <script type="application/json"> embarquant
//	                            la config complète (tenant, slug WiFi, lien join,
//	                            offres, API base, wave link) au format JSON. C'est
//	                            le SEUL marqueur injecté dans login.html/status.html
//	                            à ce stade (N°35-b) ; la page lit ce bloc via
//	                            document.getElementById('mikcloud-config').textContent
//	                            (côté client, N°35-c).
//	{{MIKCLOUD_TENANT_NAME}}  — nom du tenant (échappé HTML)
//	{{MIKCLOUD_API_BASE}}     — URL de base de l'API cloud (https://mikcloud.onrender.com)
//	{{MIKCLOUD_WIFI_SLUG}}    — slug du site WiFi jetable lié au routeur (ou "")
//	{{MIKCLOUD_JOIN_URL}}     — URL d'inscription publique liée au routeur (ou "")
//	                            avec ?mac= laissé à ajouter côté page (la MAC est
//	                            disponible côté routeur via $(mac-esc))
//	                            (N°46 : l'AFFICHAGE du bouton « S'inscrire » est
//	                            piloté par le champ joinEnabled du bloc config JSON,
//	                            lui-même posé par le réglage console tenant.joinButton)
//	{{MIKCLOUD_WAVE_LINK}}    — lien marchand Wave du tenant (ou "")
//	{{MIKCLOUD_LOGO_URL}}     — logo du tenant (data URL, ou "")
//	{{MIKCLOUD_LOGO_BLOCK}}   — bloc logo COMPLET du portail (N°135) : <img> du
//	                                logo DU CLIENT quand LogoURL est défini, sinon
//	                                l'initiale du tenant (repli neutre — le portail
//	                                d'un client ne porte JAMAIS le logo d'un autre
//	                                client, ce que faisait l'asset img/logo.png du
//	                                template de référence — le logo du site pilote).
//	{{MIKCLOUD_BANNER_URL}}   — bannière du portail du tenant (data URL ≤ 500 Ko
//	                            ou URL https, ex. Cloudflare R2 ; ou "")
//	{{MIKCLOUD_SERVICES_ATTR}} — attribut du wrap « Nos Services » (N°137) :
//	                        style="display:none" quand le tenant n'a AUCUN
//	                        service configuré (repli neutre — le template
//	                        historique y affichait les services du site pilote),
//	                        sinon chaîne vide
//	{{MIKCLOUD_SERVICES_BLOCK}} — les <li> des services DU TENANT (≤ 6,
//	                        icônes Font Awesome curées, échappement strict),
//	                        vide si aucun service
//
// Sécurité : les valeurs sont ÉCHAPPÉES pour leur contexte d'insertion. Pour le
// bloc JSON, on utilise encoding/json (échappement strict : guillemets,
// backslash, contrôle, et les « </ » deviennent « <\\/ » pour éviter la fermeture
// prématurée du <script>). Pour les marqueurs HTML simples, on échappe <, >, &,
// ", '. Aucune injection possible via le nom du tenant ou autre.
package hotpage

import (
	"encoding/json"
	"html"
	"strings"
	"unicode"
)

// PortalConfig — personnalisation d'un portail pour un compte donné. Construit
// par l'appelant (api.handlePortalFile) à partir du store, puis passé à
// Personalize qui substitue les marqueurs.
type PortalConfig struct {
	// TenantName — nom du tenant (ex. « Cyber Espace SC »). Échappé HTML.
	TenantName string `json:"tenantName"`
	// APIBase — URL de base de l'API cloud pour les fetchs hybrides
	// (ex. https://mikcloud.onrender.com). Le portail l'utilise pour
	// récupérer la config live au chargement (fallback = config inlinée).
	APIBase string `json:"apiBase"`
	// WifiSlug — slug du site WiFi jetable lié au routeur (ex. « cyber-espace-sc »).
	// Vide si le routeur n'a pas de site WiFi jetable actif.
	WifiSlug string `json:"wifiSlug,omitempty"`
	// WifiURL — URL publique de la page WiFi (ex. https://mikcloud.ftci.fr/wifi/cyber-espace-sc).
	// Vide si pas de site WiFi.
	WifiURL string `json:"wifiUrl,omitempty"`
	// Active — N°51 : état du site WiFi lié. true = carte claim « WiFi offert »
	// affichée sur le portail ; false = site en pause → la page RETIRE la carte
	// (et le claim refuserait de toute façon : 403 site_inactive). Sérialisé SANS
	// omitempty — même raison que JoinEnabled : un false omis (undefined ≠ false
	// côté page) réafficherait la carte malgré la pause. Les portails déployés
	// avant N°51 n'embarquent pas ce champ : côté page, undefined ≠ false = carte
	// injectée (comportement historique), le fetch live corrige au chargement.
	Active bool `json:"active"`
	// FreeTimeMin — minutes offertes sur le site WiFi jetable (0 = hériter
	// du profil). Alimente l'affichage de la dotation gratuite côté portail.
	FreeTimeMin int64 `json:"freeTimeMin,omitempty"`
	// FreeDataMb — Mo offerts sur le site WiFi jetable (0 = illimité/hérité).
	FreeDataMb int64 `json:"freeDataMb,omitempty"`
	// JoinURL — URL d'inscription publique liée au routeur
	// (ex. https://mikcloud.ftci.fr/join/abcdef1234...). La MAC doit être
	// ajoutée côté page (?mac=$(mac-esc)). Vide si pas de lien actif.
	JoinURL string `json:"joinUrl,omitempty"`
	// JoinEnabled — bouton « S'inscrire » visible sur la page (N°46).
	// Piloté par le réglage console tenant.joinButton (défaut effectif ON,
	// cf. model.Tenant.JoinButtonEnabled). false = AUCUN bouton
	// d'inscription : la page retire le reliquat Mikhmon « Scanner un QR
	// Code » du DOM. Sérialisé SANS omitempty : true/false doit toujours
	// être EXPLICITE dans le bloc config JSON — avec omitempty, un false
	// serait omis et la page (undefined ≠ false) réactiverait le bouton
	// malgré le réglage du gérant.
	JoinEnabled bool `json:"joinEnabled"`
	// WaveLink — lien marchand Wave du tenant
	// (ex. https://pay.wave.com/m/M_xxx/c/ci/). Vide si non configuré.
	WaveLink string `json:"waveLink,omitempty"`
	// LogoURL — logo du tenant (data URL, ex. data:image/png;base64,...). Vide = défaut.
	LogoURL string `json:"logoUrl,omitempty"`
	// BannerURL — bannière du portail (N°45) : image affichée en tête de la
	// page de login. data URL image ≤ 500 Ko OU URL https:// (Cloudflare R2).
	// Vide = pas de bannière — la page n'insère rien.
	BannerURL string `json:"bannerUrl,omitempty"`
	// N°55 — mode hospitalité : quand Style vaut « hospitality », la page
	// masque la vitrine commerciale (grille tarifaire + Wave) et affiche le
	// message de bienvenue, les promos produits (images R2 via N°53) et les
	// liens réseaux sociaux. Vide OU « commercial » = portail historique.
	Style   string `json:"portalStyle,omitempty"`
	Welcome string `json:"portalWelcome,omitempty"`
	// Promos — vitrine produits de l'établissement (≤ 6, validées côté API).
	Promos []PortalPromo `json:"portalPromos,omitempty"`
	// N°136 — slides du carrousel COMMERCIAL : URLs https (R2 via /api/media)
	// qui remplacent les 3 images génériques pub1/2/3 du slider. ≤ 3 entrées,
	// validées côté API. Vide/absent = images par défaut du template (les
	// portails déployés avant N°136 ignorent le champ sans erreur).
	Slides []string `json:"portalSlides,omitempty"`
	// Socials — liens réseaux sociaux (≤ 4, https, validés côté API).
	Socials []PortalSocial `json:"portalSocials,omitempty"`
	// N°137 — services de l'établissement affichés dans la section
	// « Nos Services » du portail (mode commercial, ≤ 6, validés côté
	// API — icônes Font Awesome curées). Vide = section masquée
	// (le template historique y portait les services du site pilote).
	Services []PortalService `json:"portalServices,omitempty"`
	// N°56 — clé publique du portail (analytics) : résout le compte pour
	// POST /api/portal/track sans authentification (pré-auth du hotspot).
	// NON secret par design (visible de chaque invité dans le bloc config) :
	// elle ne permet que le dépôt d'événements bornés côté serveur.
	PortalKey string `json:"portalKey,omitempty"`
	// Offers — offres payantes du compte (profils à prix > 0), max 8. Sérialisées
	// dans le bloc JSON pour que la page les affiche. Chaque offre porte le nom,
	// le prix, la durée et le waveUrl pré-construit.
	Offers []PortalOffer `json:"offers,omitempty"`
	// N°65 — rétention du journal du compte, en jours (30/60/90, défaut 90) :
	// alimente la note de confidentialité du portail (« données de connexion
	// conservées N jours »). Portée par le fallback inliné ET l'endpoint live
	// — le réglage s'applique aux portails déployés sans re-déploiement.
	LogRetentionDays int `json:"logRetentionDays"`
	// N°69 — consentement marketing : l'interrupteur « Me tenir informé »
	// est proposé sur le claim du portail (login.html) quand le site WiFi
	// l'a activé (WifiSite.MarketingOptIn, réglage console du wizard).
	// Sérialisé SANS omitempty — même raison que JoinEnabled/Active :
	// true/false doit toujours être EXPLICITE (avec omitempty, un false
	// omis serait lu « undefined ≠ false » côté page et l'interrupteur
	// réapparaîtrait malgré le réglage du gérant). Les portails déployés
	// avant N°69 n'embarquent pas le champ : undefined = false = pas
	// d'interrupteur (comportement historique du portail, optIn:false en
	// dur) — le fetch live corrige au chargement.
	MarketingOptIn bool `json:"marketingOptIn"`
}

// PortalPromo — une ligne de vitrine « hospitalité » (N°55) : un produit ou
// service mis en avant par l'établissement (image R2 optionnelle).
// N°56 : ID (stable, sert de clé aux compteurs analytics — les promos
// héritent d'un id aléatoire à l'enregistrement console, ou d'un id
// déterministe dérivé du contenu pour les lignes héritées d'avant N°56) et
// Link (URL https optionnelle : quand présente, la carte devient cliquable
// et son ouverture est comptée comme un « click »).
type PortalPromo struct {
	ID         string `json:"id,omitempty"`
	Title      string `json:"title"`
	Desc       string `json:"desc,omitempty"`
	ImageURL   string `json:"imageUrl,omitempty"`
	PriceLabel string `json:"priceLabel,omitempty"`
	Link       string `json:"link,omitempty"`
}

// PortalSocial — un lien réseau social affiché en pied du mode hospitalité.
type PortalSocial struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

// PortalService — une ligne de la section « Nos Services » du portail
// captif (N°137) : ce que l'établissement fait en plus du WiFi (impression,
// restauration, recharge, monétique…). Icon est une classe Font Awesome 6
// free (« fa-print »), posée depuis la whitelist curée côté console ; vide au
// rendu = « fa-check » (coche neutre).
type PortalService struct {
	Icon  string `json:"icon,omitempty"`
	Label string `json:"label"`
}

// PortalOffer — une offre payante du compte, vue par la page du portail.
type PortalOffer struct {
	Name        string `json:"name"`
	PriceFcfa   int    `json:"priceFcfa"`
	ValidityMin int    `json:"validityMin"`       // minutes
	DataQuotaMb int    `json:"dataQuotaMb"`       // 0 = illimité
	WaveURL     string `json:"waveUrl,omitempty"` // deep-link Wave pré-construit
}

// Personalize — substitue les marqueurs {{MIKCLOUD_*}} d'un contenu par les
// valeurs du PortalConfig. Retourne le contenu modifié. Les marqueurs absents
// sont laissés tels quels (pas d'erreur) — un template sans marqueur retourne
// le contenu original.
//
// Appliqué UNIQUEMENT aux fichiers texte (HTML, JS, txt) — l'appelant (handlePortalFile)
// décide de l'appliquer ou non selon l'extension. Les fichiers binaires sont
// servis tels quels par File().
func Personalize(content string, cfg PortalConfig) string {
	if content == "" {
		return content
	}
	// Marqueurs HTML simples (échappement HTML strict).
	repl := strings.NewReplacer(
		"{{MIKCLOUD_TENANT_NAME}}", html.EscapeString(cfg.TenantName),
		"{{MIKCLOUD_API_BASE}}", html.EscapeString(cfg.APIBase),
		"{{MIKCLOUD_WIFI_SLUG}}", html.EscapeString(cfg.WifiSlug),
		"{{MIKCLOUD_WIFI_URL}}", html.EscapeString(cfg.WifiURL),
		"{{MIKCLOUD_JOIN_URL}}", html.EscapeString(cfg.JoinURL),
		"{{MIKCLOUD_WAVE_LINK}}", html.EscapeString(cfg.WaveLink),
		"{{MIKCLOUD_LOGO_URL}}", html.EscapeString(cfg.LogoURL),
		"{{MIKCLOUD_LOGO_BLOCK}}", logoBlock(cfg),
		"{{MIKCLOUD_BANNER_URL}}", html.EscapeString(cfg.BannerURL),
		"{{MIKCLOUD_SERVICES_ATTR}}", servicesAttr(cfg),
		"{{MIKCLOUD_SERVICES_BLOCK}}", servicesBlock(cfg),
		"{{MIKCLOUD_CONFIG_JSON}}", configJSON(cfg),
	)
	return repl.Replace(content)
}

// logoBlock — N°135 — le bloc HTML du logo du portail : l'<img> du logo DU
// CLIENT quand LogoURL est défini (data URL ≤ 300 Ko, posée en console),
// sinon l'initiale du tenant sur le dégradé teal — un repli NEUTRE : le
// portail d'un client ne doit JAMAIS porter le logo d'un autre client (le
// « défaut » historique, img/logo.png, était le logo du site pilote de
// l'audit). Échappement strict (html.EscapeString) : ni le nom du tenant
// ni l'URL du logo ne peuvent sortir de leur contexte attribut/contenu.
func logoBlock(cfg PortalConfig) string {
	initial := html.EscapeString(tenantInitial(cfg.TenantName))
	if cfg.LogoURL == "" {
		return `<span class="logo-fallback">` + initial + `</span>`
	}
	return `<img src="` + html.EscapeString(cfg.LogoURL) + `" alt="Logo ` + html.EscapeString(cfg.TenantName) +
		`" class="rounded" loading="lazy" onerror="this.style.display='none';this.nextElementSibling.style.display='flex';">` +
		`<span class="logo-fallback" style="display:none;">` + initial + `</span>`
}

// servicesBlock — N°137 — les <li> de la section « Nos Services » : les
// services DU TENANT (posés en console, ≤ 6), échappés strict. Vide quand le
// compte n'en a pas configuré — l'ATTR masque alors la section entière (repli
// neutre : le template historique y affichait les 4 services du site pilote,
// même chasse que le logo N°135).
func servicesBlock(cfg PortalConfig) string {
	var b strings.Builder
	for _, s := range cfg.Services {
		icon := strings.TrimSpace(s.Icon)
		if icon == "" {
			icon = "fa-check"
		}
		b.WriteString(`<li class="service-list-item"><div class="service-icon-box"><i class="fas `)
		b.WriteString(html.EscapeString(icon))
		b.WriteString(`"></i></div><span>`)
		b.WriteString(html.EscapeString(s.Label))
		b.WriteString(`</span></li>`)
	}
	return b.String()
}

// servicesAttr — N°137 — l'attribut du wrap « Nos Services » : masqué
// quand le tenant n'a AUCUN service (une section vide n'a rien à faire sur
// un portail). Le fetch live (applyConfig côté page) retire ce style inline
// quand la config live apporte des services — pilotage sans re-déploiement
// (pattern N°48).
func servicesAttr(cfg PortalConfig) string {
	if len(cfg.Services) == 0 {
		return ` style="display:none"`
	}
	return ""
}

// tenantInitial — l'initiale d'affichage du tenant : première LETTRE
// (unicode) du nom, majuscule. Repli « W » (WiFi) pour un nom vide ou sans
// lettre — neutre sur un portail hotspot.
func tenantInitial(name string) string {
	for _, r := range strings.TrimSpace(name) {
		if unicode.IsLetter(r) {
			return strings.ToUpper(string(r))
		}
	}
	return "W"
}

// configJSON — sérialise le PortalConfig en JSON, ÉCHAPPÉ pour insertion dans
// un bloc <script type="application/json">. encoding/json échappe déjà <, >, &
// par défaut (cf. json.Encoder.SetEscapeHTML(true) qui est le défaut). On
// retire les newlines pour compacter et éviter les pbs de formatage.
func configJSON(cfg PortalConfig) string {
	b, err := json.Marshal(cfg)
	if err != nil {
		// PortalConfig ne contient que des types simples (string, int, slice) —
		// l'erreur de marshaling ne peut pas se produire en pratique. Si elle se
		// produit quand même, on renvoie un objet vide plutôt que de casser la page.
		return "{}"
	}
	return string(b)
}
