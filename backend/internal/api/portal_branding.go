// Package api — validations et encodage du branding du portail captif,
// PARTAGÉS entre les réglages du compte (PUT /api/settings, N°45 → N°139) et
// les surcharges SITE/ROUTEUR de la chaîne de personnalisation N°182
// (POST/PUT /api/sites, PUT /api/routers/{id}/portal). Mêmes bornes, mêmes
// messages d'erreur aux trois niveaux : le contrat du gérant ne dépend pas du
// niveau où il personnalise. Tout JSON final est RE-SÉRIALISÉ côté serveur —
// le client ne peut rien injecter d'autre que ces champs validés.
package api

import (
	"encoding/json"
	"fmt"
	"strings"

	"mikcloud/hotspot-api/internal/hotpage"
	"mikcloud/hotspot-api/internal/model"
)

// portalOverrideReq — corps de requête d'une surcharge de branding (site ou
// routeur). SÉMANTIQUE : la surcharge ENTIÈRE est remplacée à chaque écriture
// (la console renvoie l'état complet du formulaire, comme la carte Expérience
// du compte — pas de nil = inchangé). Champs vides = le niveau supérieur
// s'applique (« vide = hérite », N°182).
type portalOverrideReq struct {
	DisplayName *string             `json:"displayName"`
	LogoURL     *string             `json:"logoUrl"`
	BannerURL   *string             `json:"bannerUrl"`
	WaveLink    *string             `json:"waveLink"`
	Style       *string             `json:"portalStyle"`
	Welcome     *string             `json:"portalWelcome"`
	Promos      *[]portalPromoReq   `json:"portalPromos"`
	Socials     *[]portalSocialReq  `json:"portalSocials"`
	Services    *[]portalServiceReq `json:"portalServices"`
	Slides      *[]string           `json:"portalSlides"`
	Ticker      *[]string           `json:"portalTicker"`
	Whatsapp    *portalWhatsappReq  `json:"portalWhatsapp"`
}

// portalValidateLogo — data:image/ ≤ 300 Ko (même contrat que le logo du
// compte, N°135). Vide = pas de surcharge du logo.
func portalValidateLogo(v string) error {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	if !strings.HasPrefix(v, "data:image/") {
		return fmt.Errorf("Logo invalide : image intégrée (data:image/…) requise")
	}
	if len(v) > 300*1024 {
		return fmt.Errorf("Logo trop volumineux (300 Ko max)")
	}
	return nil
}

// portalValidateBanner — data:image/ ≤ 500 Ko OU URL https:// (N°45). Vide =
// pas de surcharge de la bannière.
func portalValidateBanner(v string) error {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	if !strings.HasPrefix(v, "data:image/") && !strings.HasPrefix(v, "https://") {
		return fmt.Errorf("Bannière invalide : image intégrée (data:image/…) ou URL https:// requise")
	}
	if strings.HasPrefix(v, "data:image/") && len(v) > 500*1024 {
		return fmt.Errorf("Bannière trop volumineuse (500 Ko max)")
	}
	return nil
}

// portalValidateStyle — "" (hérite) | commercial | hospitality (N°55).
func portalValidateStyle(v string) error {
	v = strings.TrimSpace(v)
	if v != "" && v != "commercial" && v != "hospitality" {
		return fmt.Errorf("Style de portail invalide (commercial ou hospitality)")
	}
	return nil
}

// portalValidateWelcome — ≤ 200 caractères (N°55).
func portalValidateWelcome(v string) error {
	if len(strings.TrimSpace(v)) > 200 {
		return fmt.Errorf("Message de bienvenue trop long (200 caractères max)")
	}
	return nil
}

// portalEncodePromos — vitrine hospitalité : ≤ 6 lignes, titres 1-60,
// descriptions ≤ 160, images https ≤ 300 car., prix ≤ 30, lien https ≤ 300,
// ID conservé s'il est bien formé sinon généré (N°56 — les compteurs
// analytics ne repartent jamais de zéro au round-trip console GET→PUT).
func portalEncodePromos(list []portalPromoReq) (string, error) {
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

// portalEncodeSocials — liens réseaux sociaux du mode hospitalité : ≤ 4,
// libellé 1-30, URL https ≤ 200.
func portalEncodeSocials(list []portalSocialReq) (string, error) {
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

// portalEncodeServices — section « Nos Services » (N°137) : ≤ 6 lignes,
// libellé 1-60, icône dans la whitelist curée (défaut fa-wifi).
func portalEncodeServices(list []portalServiceReq) (string, error) {
	if len(list) > 6 {
		return "", fmt.Errorf("au plus 6 services")
	}
	type service struct {
		Icon  string `json:"icon,omitempty"`
		Label string `json:"label"`
	}
	out := make([]service, 0, len(list))
	for _, it := range list {
		label := strings.TrimSpace(it.Label)
		if label == "" || len(label) > 60 {
			return "", fmt.Errorf("nom de service requis (1-60 caractères)")
		}
		icon := strings.TrimSpace(it.Icon)
		if icon == "" {
			icon = "fa-wifi"
		}
		if !portalServiceIcons[icon] {
			return "", fmt.Errorf("icône non supportée : %s", icon)
		}
		out = append(out, service{Icon: icon, Label: label})
	}
	if len(out) == 0 {
		return "", nil // liste vidée = section « Nos Services » masquée
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// portalEncodeSlides — carrousel commercial (N°136) : ≤ 3 URLs https ≤ 300
// car. Liste vide = retour aux 3 images génériques du template.
func portalEncodeSlides(list []string) (string, error) {
	if len(list) > 3 {
		return "", fmt.Errorf("Slides invalides : au plus 3 slides")
	}
	out := make([]string, 0, len(list))
	for _, it := range list {
		u := strings.TrimSpace(it)
		if u == "" {
			continue // entrée vide ignorée (la console n'en envoie pas)
		}
		if !strings.HasPrefix(u, "https://") {
			return "", fmt.Errorf("Slides invalides : URL https:// requise")
		}
		if len(u) > 300 {
			return "", fmt.Errorf("Slides invalides : URL trop longue")
		}
		out = append(out, u)
	}
	if len(out) > 0 {
		b, err := json.Marshal(out)
		if err != nil {
			return "", fmt.Errorf("Slides invalides : %v", err)
		}
		return string(b), nil
	}
	return "", nil // liste vidée = slides retirées → images par défaut
}

// portalEncodeTicker — bandeau animé (N°138) : ≤ 5 messages de 1-80 car.
// (texte brut trimé). Liste vide = retour aux 3 messages par défaut.
func portalEncodeTicker(list []string) (string, error) {
	if len(list) > 5 {
		return "", fmt.Errorf("Messages du bandeau invalides : au plus 5 messages")
	}
	out := make([]string, 0, len(list))
	for _, it := range list {
		m := strings.TrimSpace(it)
		if m == "" {
			continue
		}
		if len(m) > 80 {
			return "", fmt.Errorf("Messages du bandeau invalides : 80 caractères max par message")
		}
		out = append(out, m)
	}
	if len(out) > 0 {
		b, err := json.Marshal(out)
		if err != nil {
			return "", fmt.Errorf("Messages du bandeau invalides : %v", err)
		}
		return string(b), nil
	}
	return "", nil
}

// portalEncodeWhatsapp — numéro WhatsApp SUPPORT (N°139) : 8-15 chiffres en
// format international (espaces/+/-/() tolérés puis retirés), label ≤ 30.
// Number vide = numéro retiré → repli support MikCloud.
func portalEncodeWhatsapp(req portalWhatsappReq) (string, error) {
	num := hotpage.WhatsappNumber(req.Number)
	if req.Number != "" && num == "" {
		return "", fmt.Errorf("Numéro WhatsApp invalide : 8 à 15 chiffres en format international requis (ex. 2250708091012)")
	}
	label := strings.TrimSpace(req.Label)
	if len(label) > 30 {
		return "", fmt.Errorf("Libellé WhatsApp trop long (30 caractères max)")
	}
	if num == "" {
		return "", nil
	}
	type wa struct {
		Number string `json:"number"`
		Label  string `json:"label,omitempty"`
	}
	b, err := json.Marshal(wa{Number: num, Label: label})
	if err != nil {
		return "", fmt.Errorf("Numéro WhatsApp invalide : %v", err)
	}
	return string(b), nil
}

// assemblePortalOverride — valide CHAQUE champ de la requête d'une surcharge
// (mêmes bornes que les réglages du compte) et construit le JSON CANONIQUE
// (model.CanonicalPortalOverride : champs vides omis, "" si surcharge vide).
// L'appelant stocke la valeur telle quelle : elle rejoint le miroir de
// signature hotspot_files sans re-sérialisation supplémentaire.
func assemblePortalOverride(req *portalOverrideReq) (string, error) {
	if req == nil {
		return "", nil
	}
	var ov model.PortalOverride
	if req.DisplayName != nil {
		v := strings.TrimSpace(*req.DisplayName)
		if len(v) > 80 {
			return "", fmt.Errorf("nom affiché trop long (80 caractères max)")
		}
		ov.DisplayName = v
	}
	if req.LogoURL != nil {
		v := strings.TrimSpace(*req.LogoURL)
		if err := portalValidateLogo(v); err != nil {
			return "", err
		}
		ov.LogoURL = v
	}
	if req.BannerURL != nil {
		v := strings.TrimSpace(*req.BannerURL)
		if err := portalValidateBanner(v); err != nil {
			return "", err
		}
		ov.BannerURL = v
	}
	if req.WaveLink != nil {
		v := strings.TrimSpace(*req.WaveLink)
		if len(v) > 300 {
			return "", fmt.Errorf("lien marchand Wave trop long (300 caractères max)")
		}
		ov.WaveLink = v
	}
	if req.Style != nil {
		v := strings.TrimSpace(*req.Style)
		if err := portalValidateStyle(v); err != nil {
			return "", err
		}
		ov.Style = v
	}
	if req.Welcome != nil {
		v := strings.TrimSpace(*req.Welcome)
		if err := portalValidateWelcome(v); err != nil {
			return "", err
		}
		ov.Welcome = v
	}
	if req.Promos != nil {
		s, err := portalEncodePromos(*req.Promos)
		if err != nil {
			return "", err
		}
		if s != "" {
			// La string JSON persistée a les MÊMES json tags que
			// model.PortalPromoLine : le décodage est homéomorphe.
			_ = json.Unmarshal([]byte(s), &ov.Promos)
		}
	}
	if req.Socials != nil {
		s, err := portalEncodeSocials(*req.Socials)
		if err != nil {
			return "", err
		}
		if s != "" {
			_ = json.Unmarshal([]byte(s), &ov.Socials)
		}
	}
	if req.Services != nil {
		s, err := portalEncodeServices(*req.Services)
		if err != nil {
			return "", err
		}
		if s != "" {
			_ = json.Unmarshal([]byte(s), &ov.Services)
		}
	}
	if req.Slides != nil {
		s, err := portalEncodeSlides(*req.Slides)
		if err != nil {
			return "", err
		}
		if s != "" {
			_ = json.Unmarshal([]byte(s), &ov.Slides)
		}
	}
	if req.Ticker != nil {
		s, err := portalEncodeTicker(*req.Ticker)
		if err != nil {
			return "", err
		}
		if s != "" {
			_ = json.Unmarshal([]byte(s), &ov.Ticker)
		}
	}
	if req.Whatsapp != nil {
		s, err := portalEncodeWhatsapp(*req.Whatsapp)
		if err != nil {
			return "", err
		}
		if s != "" {
			_ = json.Unmarshal([]byte(s), &ov.Whatsapp)
		}
	}
	return model.CanonicalPortalOverride(ov), nil
}
