// handlers_settings.go — réglages du compte (tenant).

package api

import (
	"net/http"
	"strings"
)

func (a *API) handleSettingsGet(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	a.store.Lock()
	settings := ensureSettings(a.store.Data(), acc)
	a.store.Unlock()
	writeJSON(w, http.StatusOK, settings)
}

// tenantPut — forme imbriquée « tenant { … } » de PUT /api/settings. Le
// frontend P0 envoie les nouveaux champs À LA FOIS plats et imbriqués : les
// deux formes sont acceptées (le plat prime, le nested sert de repli).
type tenantPut struct {
	Name                  *string `json:"name"`
	Currency              *string `json:"currency"`
	Timezone              *string `json:"timezone"`
	WaveLink              *string `json:"waveLink"`
	DNSName               *string `json:"dnsName"`
	LogoURL               *string `json:"logoUrl"`
	BannerURL             *string `json:"bannerUrl"`
	ExpiryPolicyMode      *string `json:"expiryPolicyMode"`
	ExpiryPolicyAfterDays *int    `json:"expiryPolicyAfterDays"`
	// N°46 — bouton « S'inscrire » du portail captif (repli nested du
	// champ plat : le corps défensif du front envoie les deux formes, le
	// plat prime).
	JoinButton *bool `json:"joinButton"`
	// Audit purge/résurgence — repli nested du champ plat (corps défensif
	// du front : les deux formes sont envoyées, le plat prime).
	AutoImportRouterUsers *bool `json:"autoImportRouterUsers"`
	// N°55 — repli nested des champs hospitalité (cf. req plats).
	PortalStyle   *string            `json:"portalStyle"`
	PortalWelcome *string            `json:"portalWelcome"`
	PortalPromos  *[]portalPromoReq  `json:"portalPromos"`
	PortalSocials *[]portalSocialReq `json:"portalSocials"`
	// N°136 — repli nested des slides du carrousel commercial (cf. req plat).
	PortalSlides *[]string `json:"portalSlides"`
	// N°137 — repli nested des services du portail (cf. req plat).
	PortalServices *[]portalServiceReq `json:"portalServices"`
	// N°138 — repli nested des messages du bandeau animé (cf. req plat).
	PortalTicker *[]string `json:"portalTicker"`
	// N°139 — repli nested du numéro WhatsApp support (cf. req plat).
	PortalWhatsapp *portalWhatsappReq `json:"portalWhatsapp"`
	// N°65 — rétention du journal utilisateurs (repli nested du champ plat :
	// 30/60/90 j, défaut 90).
	LogRetentionDays *int `json:"logRetentionDays"`
}

// portalPromoReq — une ligne de vitrine « hospitalité » (N°55).
// N°56 : ID (posé par le serveur au premier enregistrement, conservé ensuite —
// c'est la clé des compteurs analytics) et Link (URL https optionnelle qui
// rend la carte cliquable sur le portail ; son ouverture est comptée « click »).
type portalPromoReq struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Desc       string `json:"desc"`
	ImageURL   string `json:"imageUrl"`
	PriceLabel string `json:"priceLabel"`
	Link       string `json:"link"`
}

// portalSocialReq — un lien réseau social (N°55).
type portalSocialReq struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

// portalServiceReq — une ligne « Nos Services » du portail captif
// (N°137) : un libellé + une icône de la whitelist curée.
type portalServiceReq struct {
	Icon  string `json:"icon"`
	Label string `json:"label"`
}

// portalWhatsappReq — le numéro WhatsApp SUPPORT du portail captif (N°139) :
// number au format international (chiffres seuls, 8-15 — espaces/+/-/()
// tolérés en entrée puis retirés) + label d'affichage optionnel (≤ 30 car.).
// Objet vide (number vide) = numéro retiré → retour au support MikCloud.
type portalWhatsappReq struct {
	Number string `json:"number"`
	Label  string `json:"label"`
}

// portalServiceIcons — N°137 — whitelist des icônes Font Awesome 6 free
// acceptées pour la section « Nos Services » du portail (toutes embarquées
// dans le template css/all.min.css — vérifiées une à une). La validation EST la
// whitelist : aucune classe arbitraire ne peut rejoindre le portail
// (défense en profondeur — le rendu hotpage échappe déjà, mais une classe
// inconnue casserait le glyphe). Miroir frontend : PORTAL_SERVICE_ICONS
// (types.ts).
var portalServiceIcons = map[string]bool{
	"fa-wifi": true, "fa-globe": true, "fa-laptop": true, "fa-tools": true,
	"fa-code": true, "fa-print": true, "fa-credit-card": true, "fa-money-bill-wave": true,
	"fa-phone": true, "fa-headset": true, "fa-gamepad": true, "fa-mug-hot": true,
	"fa-utensils": true, "fa-car": true, "fa-bolt": true, "fa-store": true,
	"fa-camera": true, "fa-scissors": true, "fa-book": true, "fa-spa": true,
}

func (a *API) handleSettingsPut(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	var req struct {
		Name     *string `json:"name"`
		Currency *string `json:"currency"`
		Timezone *string `json:"timezone"`
		WaveLink *string `json:"waveLink"`
		// P0 (audit Mikhmon) — champs plats…
		DNSName               *string `json:"dnsName"`
		LogoURL               *string `json:"logoUrl"`
		BannerURL             *string `json:"bannerUrl"`
		ExpiryPolicyMode      *string `json:"expiryPolicyMode"`
		ExpiryPolicyAfterDays *int    `json:"expiryPolicyAfterDays"`
		// N°46 — bouton « S'inscrire » du portail captif (nil = inchangé ;
		// défaut effectif ON — cf. Tenant.JoinButtonEnabled).
		JoinButton *bool `json:"joinButton"`
		// Audit purge/résurgence — import automatique des utilisateurs
		// créés hors MikCloud (nil = inchangé ; défaut effectif ON).
		AutoImportRouterUsers *bool `json:"autoImportRouterUsers"`
		// N°55 — mode hospitalité du portail captif (cf. model.Tenant) :
		// style, message de bienvenue, promos produits et liens sociaux.
		PortalStyle   *string            `json:"portalStyle"`
		PortalWelcome *string            `json:"portalWelcome"`
		PortalPromos  *[]portalPromoReq  `json:"portalPromos"`
		PortalSocials *[]portalSocialReq `json:"portalSocials"`
		// N°136 — slides du carrousel COMMERCIAL du portail captif : URLs
		// https (R2 via /api/media) qui remplacent les 3 pub génériques.
		PortalSlides *[]string `json:"portalSlides"`
		// N°137 — services de l'établissement affichés dans la section
		// « Nos Services » du portail captif (nil = inchangé ; liste vide =
		// section masquée).
		PortalServices *[]portalServiceReq `json:"portalServices"`
		// N°138 — messages du bandeau animé sous le logo du portail
		// captif (nil = inchangé ; liste vide = messages par défaut).
		PortalTicker *[]string `json:"portalTicker"`
		// N°139 — numéro WhatsApp SUPPORT du portail captif (nil =
		// inchangé ; number vide = retour au support MikCloud).
		PortalWhatsapp *portalWhatsappReq `json:"portalWhatsapp"`
		// N°65 — rétention du journal utilisateurs (30/60/90 j ; nil = inchangé,
		// défaut effectif 90).
		LogRetentionDays *int `json:"logRetentionDays"`
		// …et forme imbriquée tenant{…}.
		Tenant *tenantPut `json:"tenant"`
	}

	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	// Résolution plat > imbriqué : le corps défensif du front P0 envoie les
	// nouveaux champs À LA FOIS plats et dans tenant{…} — le plat prime.
	name, currency, timezone, waveLink := req.Name, req.Currency, req.Timezone, req.WaveLink
	dnsName, logoURL, expiryMode, expiryAfterDays := req.DNSName, req.LogoURL, req.ExpiryPolicyMode, req.ExpiryPolicyAfterDays
	bannerURL := req.BannerURL
	// N°46 — même résolution plat > imbriqué pour le bouton d'inscription.
	joinButton := req.JoinButton
	// Audit purge — même résolution plat > imbriqué pour le réglage d'import.
	autoImport := req.AutoImportRouterUsers
	// N°55 — résolution plat > imbriqué des champs hospitalité.
	portalStyle, portalWelcome := req.PortalStyle, req.PortalWelcome
	portalPromos, portalSocials := req.PortalPromos, req.PortalSocials
	// N°136 — même résolution pour les slides du carrousel commercial.
	portalSlides := req.PortalSlides
	// N°137 — même résolution pour les services du portail.
	portalServices := req.PortalServices
	// N°138 — même résolution pour les messages du bandeau animé.
	portalTicker := req.PortalTicker
	// N°139 — même résolution pour le numéro WhatsApp support.
	portalWhatsapp := req.PortalWhatsapp
	// N°65 — même résolution plat > imbriqué pour la rétention du journal.
	logRetentionDays := req.LogRetentionDays
	if req.Tenant != nil {
		if name == nil {
			name = req.Tenant.Name
		}
		if currency == nil {
			currency = req.Tenant.Currency
		}
		if timezone == nil {
			timezone = req.Tenant.Timezone
		}
		if waveLink == nil {
			waveLink = req.Tenant.WaveLink
		}
		if dnsName == nil {
			dnsName = req.Tenant.DNSName
		}
		if logoURL == nil {
			logoURL = req.Tenant.LogoURL
		}
		if bannerURL == nil {
			bannerURL = req.Tenant.BannerURL
		}
		if expiryMode == nil {
			expiryMode = req.Tenant.ExpiryPolicyMode
		}
		if expiryAfterDays == nil {
			expiryAfterDays = req.Tenant.ExpiryPolicyAfterDays
		}
		if joinButton == nil {
			joinButton = req.Tenant.JoinButton
		}
		if autoImport == nil {
			autoImport = req.Tenant.AutoImportRouterUsers
		}
		if portalStyle == nil {
			portalStyle = req.Tenant.PortalStyle
		}
		if portalWelcome == nil {
			portalWelcome = req.Tenant.PortalWelcome
		}
		if portalPromos == nil {
			portalPromos = req.Tenant.PortalPromos
		}
		if portalSocials == nil {
			portalSocials = req.Tenant.PortalSocials
		}
		if portalSlides == nil {
			portalSlides = req.Tenant.PortalSlides
		}
		if portalServices == nil {
			portalServices = req.Tenant.PortalServices
		}
		if portalTicker == nil {
			portalTicker = req.Tenant.PortalTicker
		}
		if portalWhatsapp == nil {
			portalWhatsapp = req.Tenant.PortalWhatsapp
		}
		if logRetentionDays == nil {
			logRetentionDays = req.Tenant.LogRetentionDays
		}
	}

	// Validations P0 (contrat F2/F5).
	if dnsName != nil && len(*dnsName) > 100 {
		writeErr(w, http.StatusBadRequest, "Le nom DNS doit faire au plus 100 caractères")
		return
	}
	if logoURL != nil {
		if err := portalValidateLogo(*logoURL); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	// N°45 — bannière du portail (validation partagée N°182 avec les
	// surcharges site/routeur : mêmes bornes aux trois niveaux).
	if bannerURL != nil {
		if err := portalValidateBanner(*bannerURL); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if expiryMode != nil && *expiryMode != "keep" && *expiryMode != "remove" {
		writeErr(w, http.StatusBadRequest, "Politique d'expiration invalide (keep ou remove)")
		return
	}
	// N°55/N°137 — les encodeurs promos/socials/services sont PARTAGÉS avec
	// les surcharges site/routeur de la chaîne N°182 (portal_branding.go) :
	// mêmes bornes, mêmes messages, aux trois niveaux de personnalisation.
	if portalStyle != nil {
		if err := portalValidateStyle(*portalStyle); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if portalWelcome != nil {
		if err := portalValidateWelcome(*portalWelcome); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	var promosJSON, socialsJSON, servicesJSON string
	if portalPromos != nil {
		v, err := portalEncodePromos(*portalPromos)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "Promos invalides : "+err.Error())
			return
		}
		promosJSON = v
	}
	if portalSocials != nil {
		v, err := portalEncodeSocials(*portalSocials)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "Liens sociaux invalides : "+err.Error())
			return
		}
		socialsJSON = v
	}
	// N°136 — slides du carrousel commercial (encodeur partagé N°182).
	var slidesJSON string
	if portalSlides != nil {
		v, err := portalEncodeSlides(*portalSlides)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		slidesJSON = v
	}
	if portalServices != nil {
		v, err := portalEncodeServices(*portalServices)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "Services du portail invalides : "+err.Error())
			return
		}
		servicesJSON = v
	}
	// N°138 — messages du bandeau animé (encodeur partagé N°182).
	var tickerJSON string
	if portalTicker != nil {
		v, err := portalEncodeTicker(*portalTicker)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		tickerJSON = v
	}
	// N°139 — numéro WhatsApp SUPPORT (encodeur partagé N°182).
	var whatsappJSON string
	if portalWhatsapp != nil {
		v, err := portalEncodeWhatsapp(*portalWhatsapp)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		whatsappJSON = v
	}
	if expiryAfterDays != nil && (*expiryAfterDays < 0 || *expiryAfterDays > 365) {
		writeErr(w, http.StatusBadRequest, "Le nombre de jours doit être compris entre 0 et 365")
		return
	}
	// N°65 — rétention du journal : exactement 30, 60 ou 90 jours (le contrat
	// est borné côté serveur ; toute autre valeur est refusée).
	if logRetentionDays != nil {
		v := *logRetentionDays
		if v != 30 && v != 60 && v != 90 {
			writeErr(w, http.StatusBadRequest, "Rétention du journal invalide (30, 60 ou 90 jours)")
			return
		}
	}

	a.store.Lock()
	db := a.store.Data()
	settings := ensureSettings(db, acc) // créés avec les défauts FCFA si absents
	if name != nil && strings.TrimSpace(*name) != "" {
		settings.Tenant.Name = strings.TrimSpace(*name)
	}
	if currency != nil && strings.TrimSpace(*currency) != "" {
		settings.Tenant.Currency = strings.TrimSpace(*currency)
	}
	if timezone != nil && strings.TrimSpace(*timezone) != "" {
		settings.Tenant.Timezone = strings.TrimSpace(*timezone)
	}
	if waveLink != nil {
		settings.Tenant.WaveLink = strings.TrimSpace(*waveLink) // vide = désactivé
	}
	// P0 (audit Mikhmon).
	if dnsName != nil {
		settings.Tenant.DNSName = strings.TrimSpace(*dnsName)
	}
	if logoURL != nil {
		settings.Tenant.LogoURL = strings.TrimSpace(*logoURL) // vide = logo retiré
	}
	if bannerURL != nil {
		settings.Tenant.BannerURL = strings.TrimSpace(*bannerURL) // vide = bannière retirée
	}
	// N°55 — application des champs hospitalité (les listes arrivent déjà
	// sérialisées/validées ci-dessus ; nil = inchangé, liste vide = retirée).
	if portalStyle != nil {
		settings.Tenant.PortalStyle = strings.TrimSpace(*portalStyle) // vide = commercial (défaut)
	}
	if portalWelcome != nil {
		settings.Tenant.PortalWelcome = strings.TrimSpace(*portalWelcome)
	}
	if portalPromos != nil {
		settings.Tenant.PortalPromos = promosJSON
	}
	if portalSocials != nil {
		settings.Tenant.PortalSocials = socialsJSON
	}
	// N°136 — application des slides du carrousel commercial (nil = inchangé,
	// liste vide = retour aux images génériques pub1/2/3 du template).
	if portalSlides != nil {
		settings.Tenant.PortalSlides = slidesJSON
	}
	// N°137 — les services arrivent déjà sérialisés/validés ci-dessus
	// (nil = inchangé ; liste vide = section masquée sur le portail).
	if portalServices != nil {
		settings.Tenant.PortalServices = servicesJSON
	}
	// N°138 — les messages du bandeau animé arrivent déjà
	// sérialisés/validés ci-dessus (nil = inchangé ; liste vide =
	// retour aux 3 messages par défaut du template).
	if portalTicker != nil {
		settings.Tenant.PortalTicker = tickerJSON
	}
	// N°139 — le numéro WhatsApp support arrive déjà sérialisé/validé
	// ci-dessus (nil = inchangé ; number vide = retour au support
	// MikCloud — repli historique du template).
	if portalWhatsapp != nil {
		settings.Tenant.PortalWhatsapp = whatsappJSON
	}
	if expiryMode != nil {
		settings.Tenant.ExpiryPolicyMode = *expiryMode
	}
	if expiryAfterDays != nil {
		settings.Tenant.ExpiryPolicyAfterDays = *expiryAfterDays
	}
	// N°65 — la rétention est posée explicitement (nil = inchangé ; la valeur
	// effective par défaut reste 90 via Tenant.LogRetentionDaysEffective).
	if logRetentionDays != nil {
		settings.Tenant.LogRetentionDays = logRetentionDays
	}
	// N°46 — le réglage du bouton « S'inscrire » est posé explicitement
	// (le nil reste « inchangé » ; défaut effectif ON via
	// Tenant.JoinButtonEnabled pour les comptes qui ne l'ont jamais touché).
	if joinButton != nil {
		settings.Tenant.JoinButton = joinButton
	}
	// Audit purge/résurgence — le réglage d'import automatique est posé
	// explicitement (le nil reste « inchangé », la valeur effective par
	// défaut est ON via Settings.ImportAutoEnabled).
	if autoImport != nil {
		settings.AutoImportRouterUsers = autoImport
	}
	db.SettingsByAccount[acc] = settings
	a.logActivityBy(r, db, acc, "system", "Paramètres du tenant mis à jour")
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, settings)
}
