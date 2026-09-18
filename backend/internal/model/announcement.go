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
// audience × expiration). Le read-state de la cloche (AdminUser.ActivitySeenAt,
// N°151) rend l'annonce « non lue » tant que l'utilisateur ne l'a pas vue :
// une annonce créée APRÈS le dernier acquit compte dans le badge.
type Announcement struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Body      string `json:"body,omitempty"`
	Level     string `json:"level"`    // info | warning | critical
	Audience  string `json:"audience"` // all | hotspot | homenet
	CreatedAt string `json:"createdAt"`
	// Auteur (claims du super-admin émetteur — trace d'audit).
	CreatedBy     string `json:"createdBy,omitempty"`
	CreatedByName string `json:"createdByName,omitempty"`
	// ExpiresAt — fin de vie de l'annonce (RFC 3339). Vide = sans expiration
	// (retrait manuel uniquement). Passée cette date : plus de bandeau, plus
	// de cloche, conservée dans la console plateforme pour l'historique.
	ExpiresAt string `json:"expiresAt,omitempty"`
	// EmailedAt — trace de la diffusion e-mail (une seule fois, à la
	// création si demandée) ; EmailedCount = destinataires mis en file.
	EmailedAt    string `json:"emailedAt,omitempty"`
	EmailedCount int    `json:"emailedCount,omitempty"`
}

// announcementKeep — profondeur de l'historique des annonces (les annonces
// expirées ne servent plus que de mémoire ; la plateforme n'en garde qu'un
// panier borné).
const announcementKeep = 100

// AnnouncementActive — visible par un compte d'usage donné à l'instant t ?
// (audience OUverte : « all » matche tout usage ; expiration stricte).
func (a Announcement) Active(usage, now string) bool {
	if a.Audience != AnnouncementAudienceAll && a.Audience != usage {
		return false
	}
	if a.ExpiresAt == "" {
		return true
	}
	return a.ExpiresAt > now
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
