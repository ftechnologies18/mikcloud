// Sites physiques multi-routeurs et surcharges de branding du portail captif
// (N°182). Hiérarchie de personnalisation : ROUTEUR → SITE → COMPTE — chaque
// champ de surcharge vide hérite du niveau supérieur, le portail d'un routeur
// sans site ni surcharge est EXACTEMENT celui du compte (rétrocompatibilité
// stricte : zéro changement observable pour les comptes existants).
//
// Le choix opérateur (option 3 de l'analyse n181) : le « site » devient une
// entité de premier ordre — un regroupement de routeurs (hôtel multi-bâtiments,
// chaîne de cybercafés, campus) qui porte sa propre identité de portail. Le
// portail UNIFIÉ reste le statu quo (routeurs hors site, sans surcharge) ;
// le portail PAR SITE s'obtient en assignant des routeurs à un site surchargé ;
// le portail INDIVIDUEL en posant une surcharge directement sur le routeur.
package model

import (
	"encoding/json"
	"strings"
)

// Site — un site physique du compte, regroupant 0..N routeurs. Le nom est
// unique PAR COMPTE (même discipline que les routeurs). Description et
// localisation sont purement informatives (cartes de la console) ; seule la
// SURCHARGE influence le portail captif servi aux routeurs assignés.
type Site struct {
	ID          string `json:"id"`
	AccountID   string `json:"accountId"`
	Name        string `json:"name"`                  // 1-60, unique par compte
	Description string `json:"description,omitempty"` // ≤ 200, informative
	Location    string `json:"location,omitempty"`    // ≤ 120, informative (ville/quartier)
	// PortalOverride — surcharge de branding du portail pour les routeurs
	// assignés, en JSON canonique (cf. PortalOverride). Vide = le site
	// regroupe ses routeurs SANS toucher au portail (ils servent celui du
	// compte). Les routeurs peuvent individually surcharger le site (champ
	// Router.PortalOverride).
	PortalOverride string `json:"portalOverride,omitempty"`
	CreatedAt      string `json:"createdAt"`
	UpdatedAt      string `json:"updatedAt"`
}

// PortalOverride — les surcharges de branding acceptées aux niveaux SITE et
// ROUTEUR. SÉMANTIQUE STRICTE « VIDE = HÉRITE » : un champ vide (string "",
// liste vide, Whatsapp nil ou number vide) ne surcharge RIEN — le niveau
// supérieur s'applique. Conséquence assumée (documentée console) : une
// surcharge ne peut pas MASQUER un élément que le compte affiche ; pour ce
// cas, vider le champ côté compte et le surcharger là où il doit apparaître.
type PortalOverride struct {
	// DisplayName — le nom affiché sur le portail (tenant.name sinon).
	DisplayName string `json:"displayName,omitempty"` // ≤ 80
	// LogoURL — logo du site/routeur (data:image/ ≤ 300 Ko, comme le compte).
	LogoURL string `json:"logoUrl,omitempty"`
	// BannerURL — bannière du portail (data:image/ ≤ 500 Ko ou https://).
	BannerURL string `json:"bannerUrl,omitempty"`
	// WaveLink — lien marchand Wave du site/routeur (https://pay.wave.com/…).
	WaveLink string `json:"waveLink,omitempty"`
	// Style — "" | commercial | hospitality (même contrat que le compte).
	Style string `json:"portalStyle,omitempty"`
	// Welcome — message de bienvenue du mode hospitalité (≤ 200 car.).
	Welcome string `json:"portalWelcome,omitempty"`
	// Listes structurées — MÊMES bornes que les réglages du compte
	// (promos ≤ 6, socials ≤ 4, services ≤ 6, slides ≤ 3, ticker ≤ 5) ;
	// la validation côté API est partagée avec les réglages settings.
	Promos   []PortalPromoLine   `json:"portalPromos,omitempty"`
	Socials  []PortalSocialLine  `json:"portalSocials,omitempty"`
	Services []PortalServiceLine `json:"portalServices,omitempty"`
	Slides   []string            `json:"portalSlides,omitempty"`
	Ticker   []string            `json:"portalTicker,omitempty"`
	// Whatsapp — le numéro support du site/routeur (nil/number vide = hérite).
	Whatsapp *PortalWhatsappLine `json:"portalWhatsapp,omitempty"`
}

// PortalPromoLine — une ligne de vitrine hospitalité (miroir hotpage.PortalPromo
// et du format persisté dans tenant.portalPromos — le JSON canonique de la
// surcharge est converti en string JSON homéomorphe à la résolution).
type PortalPromoLine struct {
	ID         string `json:"id,omitempty"`
	Title      string `json:"title"`
	Desc       string `json:"desc,omitempty"`
	ImageURL   string `json:"imageUrl,omitempty"`
	PriceLabel string `json:"priceLabel,omitempty"`
	Link       string `json:"link,omitempty"`
}

// PortalSocialLine — un lien réseau social du mode hospitalité.
type PortalSocialLine struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

// PortalServiceLine — une ligne « Nos Services » du portail commercial.
type PortalServiceLine struct {
	Icon  string `json:"icon,omitempty"`
	Label string `json:"label"`
}

// PortalWhatsappLine — le numéro WhatsApp SUPPORT affiché au footer du portail.
type PortalWhatsappLine struct {
	Number string `json:"number"`
	Label  string `json:"label,omitempty"`
}

// ParsePortalOverride — décode le JSON canonique d'une surcharge. Un JSON
// invalide OU vide donne une surcharge VIDE (jamais d'erreur : la résolution
// du portail ne doit jamais casser — même robustesse que portalHospitality
// et consorts sur les réglages du compte).
func ParsePortalOverride(s string) PortalOverride {
	var ov PortalOverride
	s = strings.TrimSpace(s)
	if s == "" {
		return ov
	}
	_ = json.Unmarshal([]byte(s), &ov)
	return ov
}

// IsEmpty — true quand la surcharge ne porte AUCUN champ non vide : elle
// n'influence ni le portail servi, ni la signature de déploiement (un site
// sans surcharge = un regroupement pur, le portail du compte s'applique).
func (ov PortalOverride) IsEmpty() bool {
	return ov.DisplayName == "" && ov.LogoURL == "" && ov.BannerURL == "" && ov.WaveLink == "" &&
		ov.Style == "" && ov.Welcome == "" &&
		len(ov.Promos) == 0 && len(ov.Socials) == 0 && len(ov.Services) == 0 &&
		len(ov.Slides) == 0 && len(ov.Ticker) == 0 &&
		(ov.Whatsapp == nil || ov.Whatsapp.Number == "")
}

// CanonicalPortalOverride — sérialise la surcharge en JSON canonique (champs
// vides omis). Retourne "" pour une surcharge vide : la colonne reste propre
// et le miroir de signature (portalBrandingFingerprint) voit un marqueur
// stable par surcharge non vide.
func CanonicalPortalOverride(ov PortalOverride) string {
	if ov.IsEmpty() {
		return ""
	}
	b, err := json.Marshal(ov)
	if err != nil {
		return "" // types simples : impossible en pratique, repli vide
	}
	return string(b)
}

// FindSiteScoped — retrouve le site d'un compte par son ID exact (nil si
// absent ou appartenant à un autre compte — l'isolation multi-tenant se
// vérifie à CHAQUE résolution, jamais à l'assignation seule).
func FindSiteScoped(db *DB, id, accountID string) *Site {
	if id == "" {
		return nil
	}
	for i := range db.Sites {
		if db.Sites[i].ID == id && db.Sites[i].AccountID == accountID {
			return &db.Sites[i]
		}
	}
	return nil
}

// MaxSitesPerAccount — plafond de sites par compte (regroupement de routeurs :
// 20 couvre largement les chaînes réelles tout en bornant la table).
const MaxSitesPerAccount = 20
