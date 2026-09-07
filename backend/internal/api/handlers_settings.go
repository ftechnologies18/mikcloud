// handlers_settings.go — réglages du compte (tenant).

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"mikcloud/hotspot-api/internal/model"
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
		if logRetentionDays == nil {
			logRetentionDays = req.Tenant.LogRetentionDays
		}
	}

	// Validations P0 (contrat F2/F5).
	if dnsName != nil && len(*dnsName) > 100 {
		writeErr(w, http.StatusBadRequest, "Le nom DNS doit faire au plus 100 caractères")
		return
	}
	if logoURL != nil && *logoURL != "" {
		if !strings.HasPrefix(*logoURL, "data:image/") {
			writeErr(w, http.StatusBadRequest, "Logo invalide : image intégrée (data:image/…) requise")
			return
		}
		if len(*logoURL) > 300*1024 {
			writeErr(w, http.StatusBadRequest, "Logo trop volumineux (300 Ko max)")
			return
		}
	}
	// N°45 — bannière du portail : data:image/… ≤ 500 Ko OU URL https://
	// (Cloudflare R2 et tout hébergeur d'images — le portail et la page
	// WiFi exigent https pour éviter le mixed content). Vide = retirée.
	if bannerURL != nil && *bannerURL != "" {
		v := strings.TrimSpace(*bannerURL)
		if !strings.HasPrefix(v, "data:image/") && !strings.HasPrefix(v, "https://") {
			writeErr(w, http.StatusBadRequest, "Bannière invalide : image intégrée (data:image/…) ou URL https:// requise")
			return
		}
		if strings.HasPrefix(v, "data:image/") && len(v) > 500*1024 {
			writeErr(w, http.StatusBadRequest, "Bannière trop volumineuse (500 Ko max)")
			return
		}
	}
	if expiryMode != nil && *expiryMode != "keep" && *expiryMode != "remove" {
		writeErr(w, http.StatusBadRequest, "Politique d'expiration invalide (keep ou remove)")
		return
	}
	// N°55 — validations hospitalité : style borné, bienvenue courte, promos
	// ≤ 6 (titres/descriptions/prix bornés, image https ou R2 via /api/media),
	// socials ≤ 4 (URL https). Le JSON final est resérialisé côté serveur —
	// le client ne peut rien injecter d'autre que ces champs validés.
	encodePromos := func(list []portalPromoReq) (string, error) {
		if len(list) > 6 {
			return "", fmt.Errorf("au plus 6 promos")
		}
		type promo struct {
			ID         string `json:"id,omitempty"`
			Title      string `json:"title"`
			Desc       string `json:"desc,omitempty"`
			ImageURL   string `json:"imageUrl,omitempty"`
			PriceLabel string `json:"priceLabel,omitempty"`
			Link       string `json:"link,omitempty"`
		}
		out := make([]promo, 0, len(list))
		for _, it := range list {
			title := strings.TrimSpace(it.Title)
			if title == "" || len(title) > 60 {
				return "", fmt.Errorf("titre de promo requis (1-60 caractères)")
			}
			desc := strings.TrimSpace(it.Desc)
			if len(desc) > 160 {
				return "", fmt.Errorf("description de promo trop longue (160 caractères max)")
			}
			img := strings.TrimSpace(it.ImageURL)
			if img != "" && !strings.HasPrefix(img, "https://") {
				return "", fmt.Errorf("image de promo invalide : URL https:// requise")
			}
			if len(img) > 300 {
				return "", fmt.Errorf("URL d'image trop longue")
			}
			price := strings.TrimSpace(it.PriceLabel)
			if len(price) > 30 {
				return "", fmt.Errorf("prix trop long (30 caractères max)")
			}
			// N°56 — ID : conservé tel quel s'il est déjà bien formé
			// (le round-trip console GET→PUT ne doit JAMAIS
			// régénérer les ids, sinon les compteurs analytics
			// repartiraient de zéro à chaque enregistrement) ; sinon
			// un id aléatoire est posé (premier enregistrement d'une
			// ligne, y compris les lignes envoyées sans id par un
			// appelant API). Link : https only, comme l'image (le
			// portail captive est mixte-content-free).
			id := strings.TrimSpace(it.ID)
			if id != "" && !promoIDValid(id) {
				return "", fmt.Errorf("id de promo invalide")
			}
			if id == "" {
				id = model.NewID("p")
			}
			link := strings.TrimSpace(it.Link)
			if link != "" && (!strings.HasPrefix(link, "https://") || len(link) > 300) {
				return "", fmt.Errorf("lien de promo invalide (https://, 300 caractères max)")
			}
			out = append(out, promo{ID: id, Title: title, Desc: desc, ImageURL: img, PriceLabel: price, Link: link})
		}
		if len(out) == 0 {
			return "", nil // liste vidée = promos retirées
		}
		b, err := json.Marshal(out)
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
	encodeSocials := func(list []portalSocialReq) (string, error) {
		if len(list) > 4 {
			return "", fmt.Errorf("au plus 4 liens sociaux")
		}
		type social struct {
			Label string `json:"label"`
			URL   string `json:"url"`
		}
		out := make([]social, 0, len(list))
		for _, it := range list {
			label := strings.TrimSpace(it.Label)
			url := strings.TrimSpace(it.URL)
			if label == "" || len(label) > 30 {
				return "", fmt.Errorf("libellé de lien requis (1-30 caractères)")
			}
			if !strings.HasPrefix(url, "https://") || len(url) > 200 {
				return "", fmt.Errorf("URL de lien invalide (https://, 200 caractères max)")
			}
			out = append(out, social{Label: label, URL: url})
		}
		if len(out) == 0 {
			return "", nil
		}
		b, err := json.Marshal(out)
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
	if portalStyle != nil {
		v := strings.TrimSpace(*portalStyle)
		if v != "" && v != "commercial" && v != "hospitality" {
			writeErr(w, http.StatusBadRequest, "Style de portail invalide (commercial ou hospitality)")
			return
		}
	}
	if portalWelcome != nil && len(strings.TrimSpace(*portalWelcome)) > 200 {
		writeErr(w, http.StatusBadRequest, "Message de bienvenue trop long (200 caractères max)")
		return
	}
	var promosJSON, socialsJSON string
	if portalPromos != nil {
		v, err := encodePromos(*portalPromos)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "Promos invalides : "+err.Error())
			return
		}
		promosJSON = v
	}
	if portalSocials != nil {
		v, err := encodeSocials(*portalSocials)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "Liens sociaux invalides : "+err.Error())
			return
		}
		socialsJSON = v
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
