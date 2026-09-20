// announcement_sweep.go — N°165 : balayage PÉRIODIQUE des annonces programmées.
//
// La VISIBILITÉ d'une annonce programmée ne demande AUCUNE action de fond :
// Active() borne chaque lecture (bandeau, cloche, liste) et l'annonce
// apparaît d'elle-même quand « now » franchit PublishAt. Le balayage ne
// s'occupe que de la seule chose qui ne peut PAS se calculer à la lecture :
// l'E-MAIL DIFFÉRÉ d'une annonce programmée avec e-mail demandé
// (EmailPending) — il part au moment de la publication, jamais avant (sinon
// l'annonce serait connue par e-mail avant d'apparaître en console).
//
// Patron des balayages existants (retention.go N°64, chat_sweep.go N°129) :
// goroutine lancée par main.go, rattrapage immédiat au démarrage (un
// redéploiement juste après l'instant de publication rattrape l'envoi),
// passage par minute (granularité de la programmation), panique récupérée
// (la boucle repart au passage suivant).
//
// Idempotence : EmailedAt posé et EmailPending épongé SOUS le verrou AVANT
// la moindre mise en file d'envoi — un crash entre les deux laisse au pire
// un envoi perdu (jamais un doublon), un second passage ne refile rien.
package api

import (
	"log"
	"runtime/debug"
	"time"

	"mikcloud/hotspot-api/internal/model"
)

// announcementSweepInterval — période du balayage : la programmation d'une
// annonce est au mieux précise à la minute, et le passage ne coûte qu'un
// verrou bref sur un panier borné (100 annonces max, announcementKeep).
const announcementSweepInterval = time.Minute

// RunAnnouncementSweepForever — boucle du balayage (lancée en goroutine par
// main.go) : rattrapage immédiat au démarrage, puis passage par minute.
func (a *API) RunAnnouncementSweepForever() {
	sweep := func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("annonces : panique récupérée (reprise à la minute suivante) : %v\n%s", r, debug.Stack())
			}
		}()
		a.RunAnnouncementSweep()
	}
	sweep()
	for range time.Tick(announcementSweepInterval) {
		sweep()
	}
}

// RunAnnouncementSweep — un passage : pour chaque annonce DUE (PublishAt
// atteint, ou vide par prudence défensive) à e-mail en attente, résout les
// destinataires sous le verrou (l'audience du moment de la PUBLICATION —
// un compte créé entre la programmation et la publication en fait partie),
// pose la trace de diffusion, éponge le drapeau, Save, PUIS file les envois
// best-effort après déverrouillage (discipline N°146 : aucun envoi sous
// verrou, aucun état lu hors verrou).
func (a *API) RunAnnouncementSweep() {
	type due struct {
		ann     model.Announcement
		targets []announcementMailTarget
	}
	var pendings []due
	a.store.Lock()
	db := a.store.Data()
	now := model.NowISO()
	for i := range db.Announcements {
		ann := &db.Announcements[i]
		if !ann.EmailPending {
			continue
		}
		if ann.PublishAt != "" && ann.PublishAt > now {
			continue // pas encore publiée : l'e-mail attend sa parution
		}
		fresh := *ann
		targets := resolveAnnouncementTargetsLocked(db, fresh)
		fresh.EmailedAt = model.NowISO()
		fresh.EmailedCount = len(targets)
		fresh.EmailPending = false
		*ann = fresh
		pendings = append(pendings, due{ann: fresh, targets: targets})
	}
	if len(pendings) > 0 {
		a.store.Save() // un Save PostgreSQL seulement si l'état a changé
	}
	a.store.Unlock()
	// Traces et envois APRÈS déverrouillage (aucune E/S sous le verrou).
	for _, d := range pendings {
		log.Printf("annonces : publication de « %s » — e-mail différé parti vers %d destinataire(s)", d.ann.Title, len(d.targets))
		a.sendAnnouncementEmails(d.ann, d.targets)
	}
}
