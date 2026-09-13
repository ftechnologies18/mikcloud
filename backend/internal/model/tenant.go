// Entités multi-tenant : tenant, plans SaaS, abonnement, paramètres, configuration plateforme.
// Extrait du monolithe models.go (N°88) — même package, contenu inchangé.
package model

// Tenant — infos du tenant.
type Tenant struct {
	Name     string `json:"name"`
	Currency string `json:"currency"`
	Timezone string `json:"timezone"`
	// Wave CI — lien marchand pay.wave.com (ex. https://pay.wave.com/m/M_xxx/c/ci/)
	// composé avec /amount/<montant>/ pour les demandes de paiement.
	WaveLink string `json:"waveLink,omitempty"`
	// P0 (audit Mikhmon) — F2 : personnalisation voucher.
	DNSName string `json:"dnsName,omitempty"` // ex. wifi.mondomaine.ci
	LogoURL string `json:"logoUrl,omitempty"` // data URL image ≤ 300 Ko
	// Bannière du portail captif (N°45) : image affichée en tête de la page
	// de login. Deux formes acceptées — data URL `data:image/…` ≤ 500 Ko
	// (upload console) ou URL https:// (Cloudflare R2, session suivante).
	// Vide = aucune bannière (portail sans image tête).
	BannerURL string `json:"bannerUrl,omitempty"`
	// Bouton « S'inscrire » du portail captif (N°46) : quand activé (valeur
	// effective par défaut : nil OU true), la page de login affiche le
	// bouton « S'inscrire » pointant vers le lien d'inscription publique
	// actif lié au routeur (quota MAC N°33). Quand désactivé (false),
	// AUCUN bouton d'inscription n'est rendu — le reliquat Mikhmon
	// « Scanner un QR Code » (lien externe sans fonction métier) est
	// retiré de la page. Pointeur : nil = défaut ON sans écrire le champ
	// dans le JSON renvoyé (compatibilité zéro-migration, même pattern
	// que Settings.AutoImportRouterUsers) ; la colonne Neon
	// `settings.join_button` (NOT NULL DEFAULT TRUE) reporte la valeur
	// explicite au premier Save.
	JoinButton *bool `json:"joinButton,omitempty"`
	// N°55 — mode hospitalité du portail captif. MikCloud sert deux usages :
	// la vente de tickets (commercial, défaut — grille tarifaire + Wave) ET
	// l'offre gratuite de fidélisation (hôtel, maquis, café-glacier, salon…)
	// où le portail devient une vitrine des produits/services de
	// l'établissement. "" OU "commercial" = portail historique ;
	// "hospitality" = grille tarifaire/Wave masquée, remplacée par le
	// message de bienvenue, les promos produits (images R2 via N°53) et
	// les liens réseaux sociaux.
	PortalStyle string `json:"portalStyle,omitempty"` // "" | "commercial" | "hospitality"
	// Message de bienvenue affiché en tête du mode hospitalité (≤ 200 car.).
	PortalWelcome string `json:"portalWelcome,omitempty"`
	// Promos produits — JSON [{title,desc,imageUrl,priceLabel}] ≤ 6 items
	// (structurés, persistés en string : pattern N°55, pas de table dédiée).
	PortalPromos string `json:"portalPromos,omitempty"`
	// Liens réseaux sociaux — JSON [{label,url}] ≤ 4 (WhatsApp, Facebook…).
	PortalSocials string `json:"portalSocials,omitempty"`
	// N°56 — clé publique du portail (16 hex, générée une fois par compte) :
	// identifiant NON secret embarqué dans la config du portail captif
	// (bloc mikcloud-config, visible de chaque invité par design) qui
	// permet au track analytics (POST /api/portal/track) de résoudre le
	// compte SANS authentification (pré-auth du hotspot). Elle n'ouvre
	// AUCUN droit de lecture : uniquement le dépôt d'événements
	// impressions/clics, dédupliqués et bornés côté serveur.
	PortalKey string `json:"portalKey,omitempty"`
	// P0 (audit Mikhmon) — F5 : politique de nettoyage des expirés.
	ExpiryPolicyMode      string `json:"expiryPolicyMode"`      // "keep" (défaut) | "remove"
	ExpiryPolicyAfterDays int    `json:"expiryPolicyAfterDays"` // défaut 30
	// N°65 — rétention du journal utilisateurs PAR COMPTE (30/60/90 jours,
	// défaut 90). Pointeur : nil = défaut 90 sans écrire le champ dans le JSON
	// renvoyé (compatibilité zéro-migration pour les comptes existants, même
	// pattern que JoinButton) ; la colonne Neon settings.log_retention_days
	// (NOT NULL DEFAULT 90) reporte la valeur explicite au premier Save.
	LogRetentionDays *int `json:"logRetentionDays,omitempty"`
}

// DefaultLogRetentionDays — rétention par défaut du journal utilisateurs
// (F3), en jours. N°65 : chaque compte peut resserrer à 30/60 via
// tenant.logRetentionDays ; 90 reste le comportement historique (N°64).

// DefaultLogRetentionDays — rétention par défaut du journal utilisateurs
// (F3), en jours. N°65 : chaque compte peut resserrer à 30/60 via
// tenant.logRetentionDays ; 90 reste le comportement historique (N°64).
const DefaultLogRetentionDays = 90

// LogRetentionDaysEffective — valeur EFFECTIVE de la rétention du journal
// pour un compte (nil ou valeur hors 30/60/90 = 90 jours : le comportement
// N°64 est préservé pour les comptes existants — une valeur invalide glissée
// en base ne peut jamais ouvrir une rétention illimitée).

// LogRetentionDaysEffective — valeur EFFECTIVE de la rétention du journal
// pour un compte (nil ou valeur hors 30/60/90 = 90 jours : le comportement
// N°64 est préservé pour les comptes existants — une valeur invalide glissée
// en base ne peut jamais ouvrir une rétention illimitée).
func (t Tenant) LogRetentionDaysEffective() int {
	if t.LogRetentionDays == nil {
		return DefaultLogRetentionDays
	}
	switch *t.LogRetentionDays {
	case 30, 60, 90:
		return *t.LogRetentionDays
	default:
		return DefaultLogRetentionDays
	}
}

// Plan — plan d'abonnement SaaS (libellé hérité de l'ère pré-facturation ;
// maintenu pour compatibilité d'affichage, l'état réel vit dans Subscription).

// Plan — plan d'abonnement SaaS (libellé hérité de l'ère pré-facturation ;
// maintenu pour compatibilité d'affichage, l'état réel vit dans Subscription).
type Plan struct {
	Name       string `json:"name"`
	MaxRouters string `json:"maxRouters"`
	MaxUsers   string `json:"maxUsers"`
}

// Subscription — état d'abonnement SaaS d'un compte. PlanID vide = ère bêta
// (aucune formule souscrite). PeriodEnd vide = non expirant.

// Subscription — état d'abonnement SaaS d'un compte. PlanID vide = ère bêta
// (aucune formule souscrite). PeriodEnd vide = non expirant.
type Subscription struct {
	PlanID      string `json:"planId"`      // "" (bêta) | essentiel | illimite
	Status      string `json:"status"`      // active | expired
	PeriodStart string `json:"periodStart"` // RFC3339
	PeriodEnd   string `json:"periodEnd"`   // RFC3339 — "" = non expirant
	// LastAmountFcfa — montant de la période en cours : Essentiel =
	// 1 250 F × routeurs enregistrés au moment de la souscription, Illimité = forfait.
	LastAmountFcfa int `json:"lastAmountFcfa"`
	// P2/P3 (console plateforme) — RouterSlots : nombre de routeurs couverts
	// par une période Essentiel (quota réel vérifié côté serveur à la
	// création de routeur ; 0 = non plafonné : bêta, illimité, plateforme).
	// LastPaidAt : date RFC3339 du dernier paiement marqué par la plateforme
	// (vide = période en attente de paiement — indicatif, sans blocage).
	RouterSlots int    `json:"routerSlots,omitempty"`
	LastPaidAt  string `json:"lastPaidAt,omitempty"`
}

// SaasPlan — formule d'abonnement MikCloud (catalogue public de la console).

// SaasPlan — formule d'abonnement MikCloud (catalogue public de la console).
type SaasPlan struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	PriceFcfa int    `json:"priceFcfa"`
	Period    string `json:"period"`    // mois | an
	PerRouter bool   `json:"perRouter"` // true : prix × routeurs enregistrés
	Unlimited bool   `json:"unlimited"` // routeurs illimités
	Tagline   string `json:"tagline"`
	Badge     string `json:"badge,omitempty"`
}

// SaasPlans — catalogue des formules MikCloud (marché FCFA concurrentiel).
//   - Essentiel : 1 250 F/mois/routeur — acquisition, sans engagement, le gérant
//     paie au fil de sa croissance (remboursé par 4-5 tickets 24 h vendus).
//   - Illimité : 12 000 F/an, routeurs illimités — arme de conquête :
//     1 000 F/mois équivalent, 2 mois offerts vs Essentiel (−20 % à 1 routeur,
//     −92 % à 10 routeurs), verrouille 12 mois et fait consolider tous les sites.

// SaasPlans — catalogue des formules MikCloud (marché FCFA concurrentiel).
//   - Essentiel : 1 250 F/mois/routeur — acquisition, sans engagement, le gérant
//     paie au fil de sa croissance (remboursé par 4-5 tickets 24 h vendus).
//   - Illimité : 12 000 F/an, routeurs illimités — arme de conquête :
//     1 000 F/mois équivalent, 2 mois offerts vs Essentiel (−20 % à 1 routeur,
//     −92 % à 10 routeurs), verrouille 12 mois et fait consolider tous les sites.
var SaasPlans = []SaasPlan{
	{
		ID: "essentiel", Name: "Essentiel", PriceFcfa: 1250, Period: "mois",
		PerRouter: true, Tagline: "Payez au fil de votre croissance",
		Badge: "Sans engagement",
	},
	{
		ID: "illimite", Name: "Illimité", PriceFcfa: 12000, Period: "an",
		Unlimited: true, Tagline: "Tous vos routeurs, un seul prix",
		Badge: "2 mois offerts · −20 %",
	},
}

// PlanByID — retrouve une formule du catalogue par son identifiant.

// PlanByID — retrouve une formule du catalogue par son identifiant.
func PlanByID(id string) (SaasPlan, bool) {
	for _, p := range SaasPlans {
		if p.ID == id {
			return p, true
		}
	}
	return SaasPlan{}, false
}

// Settings — paramètres du tenant (tenant + plan + abonnement).

// Settings — paramètres du tenant (tenant + plan + abonnement).
type Settings struct {
	Tenant       Tenant       `json:"tenant"`
	Plan         Plan         `json:"plan"`
	Subscription Subscription `json:"subscription"`
	// I (paramètres plateforme) — N'EST UTILISÉ QUE SUR LE COMPTE PRINCIPAL
	// (AccountMainID) : configuration globale du SaaS vue par l'admin
	// plateforme. Ignoré pour les comptes clients.
	Platform *PlatformConfig `json:"platform,omitempty"`
	// AutoImportRouterUsers — réglage par compte (audit purge/résurgence) :
	// quand activé (valeur effective par défaut : nil OU true), les
	// utilisateurs hotspot présents sur un routeur AGENT mais inconnus du
	// cloud sont importés automatiquement à chaque read_state (découverte
	// des comptes créés dans Winbox). Quand désactivé (false), le read_state
	// ne crée RIEN : les comptes hors MikCloud sont comptés dans
	// Router.UnknownOnRouter pour adoption manuelle (outil d'import).
	// Pointeur : nil = défaut ON sans écrire le champ dans le JSON persisté
	// (compatibilité zéro-migration pour les comptes existants).
	AutoImportRouterUsers *bool `json:"autoImportRouterUsers,omitempty"`
}

// ImportAutoEnabled — valeur EFFECTIVE du réglage d'import automatique pour un
// compte (nil = ON : comportement historique préservé, zéro surprise).

// ImportAutoEnabled — valeur EFFECTIVE du réglage d'import automatique pour un
// compte (nil = ON : comportement historique préservé, zéro surprise).
func (s Settings) ImportAutoEnabled() bool {
	return s.AutoImportRouterUsers == nil || *s.AutoImportRouterUsers
}

// JoinButtonEnabled — valeur EFFECTIVE du réglage du bouton « S'inscrire »
// du portail captif (N°46) pour un compte (nil = ON : comportement historique
// préservé, zéro-migration pour les comptes existants).

// JoinButtonEnabled — valeur EFFECTIVE du réglage du bouton « S'inscrire »
// du portail captif (N°46) pour un compte (nil = ON : comportement historique
// préservé, zéro-migration pour les comptes existants).
func (t Tenant) JoinButtonEnabled() bool {
	return t.JoinButton == nil || *t.JoinButton
}

// PlatformConfig — configuration globale de la plateforme MikCloud (vivante
// sur les settings du compte principal). Gérée depuis la console plateforme
// (vue Paramètres plateforme), persistée en PostgreSQL — plus besoin de
// redéployer Render pour ouvrir/fermer les inscriptions.

// PlatformConfig — configuration globale de la plateforme MikCloud (vivante
// sur les settings du compte principal). Gérée depuis la console plateforme
// (vue Paramètres plateforme), persistée en PostgreSQL — plus besoin de
// redéployer Render pour ouvrir/fermer les inscriptions.
type PlatformConfig struct {
	// Nom affiché du SaaS (login, footer) — défaut "MikCloud".
	Name string `json:"name"`
	// RegisterOpen : auto-inscription publique autorisée sans clé.
	// Priorité de handleRegister : env REGISTER_KEY (si définie) > cette clé.
	RegisterOpen bool `json:"registerOpen"`
	// RegisterKey : clé d'invitation requise quand RegisterOpen = false
	// ("" = pas de clé → inscriptions totalement fermées).
	RegisterKey string `json:"registerKey,omitempty"`
}

// AdminUser — compte d'accès à la console (login), rattaché à un compte SaaS.
