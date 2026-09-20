// N°152 — annonces de la plateforme MikCloud : messages diffusés par le
// super-admin à l'ensemble des comptes clients (maintenance prévue, nouvelle
// fonctionnalité, incident en cours…).
package model

// Niveaux d'annonce — pilotent la couleur du bandeau et de l'entrée de
// cloche côté client.
const (
	AnnouncementInfo     = "info"     // information neutre (nouveautés, conseils)
	AnnouncementWarning  = "warning"  // action recommandée (maintenance, migration)
	AnnouncementCritical = "critical" // incident/urgence (service dégradé)
)

// Audiences de diffusion — ciblage par usage du compte client.
const (
	AnnouncementAudienceAll     = "all"     // tous les comptes clients
	AnnouncementAudienceHotspot = "hotspot" // comptes établissements (cybers, hôtels…)
	AnnouncementAudienceHomeNet = "homenet" // comptes foyers
)

// Announcement — une annonce diffusée aux clients MikCloud.
//
// Vit dans db.Announcements (collection PLATEFORME, pas par compte : une
// annonce est globale ; sa VISIBILITÉ par compte se calcule à la lecture —
// audience × programmation × expiration). Le read-state de la cloche (AdminUser.ActivitySeenAt,
// N°151) rend l'annonce « non lue » tant que l'utilisateur ne l'a pas vue :
// une annonce créée APRÈS le dernier acquit compte dans le badge.
type Announcement struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Body      string `json:"body,omitempty"`
	Level     string `json:"level"`    // info | warning | critical
	Audience  string `json:"audience"` // all | hotspot | homenet
	CreatedAt string `json:"createdAt"`
	// PublishAt — date de DIFFUSION programmée (RFC 3339, N°165). Vide =
	// diffusion immédiate (comportement historique). Avant cette date :
	// invisible des clients (bandeau, cloche, liste), visible en console
	// plateforme avec l'état « programmée ». La visibilité se calcule à la
	// LECTURE (Active) : aucune action de fond n'est nécessaire pour
	// « publier » — l'annonce devient visible d'elle-même le moment venu.
	PublishAt string `json:"publishAt,omitempty"`
	// Auteur (claims du super-admin émetteur — trace d'audit).
	CreatedBy     string `json:"createdBy,omitempty"`
	CreatedByName string `json:"createdByName,omitempty"`
	// ExpiresAt — fin de vie de l'annonce (RFC 3339). Vide = sans expiration
	// (retrait manuel uniquement). Passée cette date : plus de bandeau, plus
	// de cloche, conservée dans la console plateforme pour l'historique.
	ExpiresAt string `json:"expiresAt,omitempty"`
	// EmailedAt — trace de la diffusion e-mail (une seule fois : à la création
	// si immédiate, sinon AU MOMENT DE LA PUBLICATION — N°165) ;
	// EmailedCount = destinataires mis en file.
	EmailedAt    string `json:"emailedAt,omitempty"`
	EmailedCount int    `json:"emailedCount,omitempty"`
	// EmailPending — N°165 : e-mail DEMANDÉ pour une annonce programmée, pas
	// encore parti. Posé à la création (publishAt futur + email), épongé par
	// le balayage d'annonce à l'instant de la publication (RunAnnouncementSweep,
	// api/announcement_sweep.go) qui pose EmailedAt/EmailedCount et déclare
	// EmailPending. Idempotent : le drapeau cleared SOUS verrou avant l'envoi,
	// un redémarrage ne double jamais la diffusion.
	EmailPending bool `json:"emailPending,omitempty"`
}

// announcementKeep — profondeur de l'historique des annonces (les annonces
// expirées ne servent plus que de mémoire ; la plateforme n'en garde qu'un
// panier borné).
const announcementKeep = 100

// États d'une annonce pour la console plateforme (N°165) — pilotent le badge
// de statut de la liste. « scheduled » = PublishAt dans le futur.
const (
	AnnouncementStateActive    = "active"    // visible des clients à l'instant présent
	AnnouncementStateScheduled = "scheduled" // sera diffusée automatiquement à PublishAt
	AnnouncementStateExpired   = "expired"   // hors délai (ExpiresAt passé)
)

// AnnouncementActive — visible par un compte d'usage donné à l'instant t ?
// (audience OUverte : « all » matche tout usage ; programmation et expiration
// strictes : une annonce programmée n'est PAS active avant sa date, une
// expirée ne l'est plus après — comparaison lexicographique de RFC 3339 UTC,
// cohérente avec l'ExpiresAt historique).
func (a Announcement) Active(usage, now string) bool {
	if a.Audience != AnnouncementAudienceAll && a.Audience != usage {
		return false
	}
	if a.PublishAt != "" && a.PublishAt > now {
		return false // N°165 — programmée : pas encore diffusée
	}
	if a.ExpiresAt == "" {
		return true
	}
	return a.ExpiresAt > now
}

// EffectiveAt — l'instant où l'annonce EST APPARUE (ou apparaîtra) aux
// clients : PublishAt si programmée, CreatedAt sinon (N°165). Sert au tri
// « la plus récente d'abord » des listes clients et au read-state de la
// cloche (une annonce programmée est « non lue » à compter de sa
// PUBLICATION, pas de sa rédaction).
func (a Announcement) EffectiveAt() string {
	if a.PublishAt != "" {
		return a.PublishAt
	}
	return a.CreatedAt
}

// State — état de l'annonce pour la console plateforme, SANS la dimension
// audience (une annonce est globale : son état ne dépend pas du compte qui
// la regarde) : scheduled si PublishAt est dans le futur, expired si
// ExpiresAt est passé, active sinon (N°165).
func (a Announcement) State(now string) string {
	if a.PublishAt != "" && a.PublishAt > now {
		return AnnouncementStateScheduled
	}
	if a.ExpiresAt != "" && a.ExpiresAt <= now {
		return AnnouncementStateExpired
	}
	return AnnouncementStateActive
}

// AppendAnnouncement — insertion en tête + cap (sous verrou).
func AppendAnnouncement(db *DB, e Announcement) {
	if e.ID == "" {
		e.ID = NewID("ann-")
	}
	if e.CreatedAt == "" {
		e.CreatedAt = NowISO()
	}
	db.Announcements = append([]Announcement{e}, db.Announcements...)
	if len(db.Announcements) > announcementKeep {
		db.Announcements = db.Announcements[:announcementKeep]
	}
}
