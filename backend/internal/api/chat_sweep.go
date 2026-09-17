// chat_sweep.go — N°129 — balayage PÉRIODIQUE de l'assistant
// conversationnel : clôture d'inactivité + rétention.
//
// AVANT (N°127) : la rétention du chat ne purgeait qu'à la CRÉATION de
// session (chatPruneLocked opportuniste) et rien ne clôturait jamais une
// conversation que le visiteur avait quittée — sous affluence, l'inbox
// support gonflait de fils morts (conversations « bot » muettes depuis
// des heures, transmissions « human » jamais clôturées, donc jamais
// purgées).
//
// DEPUIS : une goroutine (main.go) passe toutes les minutes :
//
//	chatAutoCloseLocked — clôture atomique (sous le verrou) des
//	                      conversations vivantes sans nouveau message
//	                      depuis 15 minutes (bot OU human) : message de
//	                      fin dans la langue du visiteur, widget qui
//	                      propose une nouvelle conversation, réponse du
//	                      support qui rouvre le fil ;
//	chatPruneLocked     — rétention : fermées purgées à 30 jours, bot
//	                      inactives à 7 jours, garde-fou 2 000 —
//	                      désormais AUSSI périodique, plus seulement à
//	                      la création de session.
//
// Le même balayage est rejoué à la lecture de l'inbox console
// (handleAdminChatConversations) : la vue « Conversations » ouverte, le
// support voit les clôtures immédiatement. Rattrapage au démarrage puis
// passage par minute ; chaque passage est protégé contre les paniques
// (même filet que la rétention N°74 — une panique ne tue pas la boucle).
package api

import (
	"log"
	"runtime/debug"
	"time"
)

// chatSweepInterval — période du balayage chat : la clôture d'inactivité
// (15 min) est effective à la minute près. Le coût d'un passage est
// borné par le plafond de conversations (2 000) — insignifiant côté
// charge, un Save PostgreSQL n'a lieu que si l'état change.
const chatSweepInterval = time.Minute

// RunChatSweepForever — boucle du balayage chat (lancée en goroutine par
// main.go) : rattrapage immédiat au démarrage, puis passage par minute.
func (a *API) RunChatSweepForever() {
	sweep := func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("chat : panique récupérée (reprise à la minute suivante) : %v\n%s", r, debug.Stack())
			}
		}()
		a.RunChatSweep()
	}
	sweep()
	for range time.Tick(chatSweepInterval) {
		sweep()
	}
}

// RunChatSweep — un passage : verrou → clôture d'inactivité → rétention
// → Save. Aucune requête HTTP ne dépend de ce passage : les prochaines
// lectures verront simplement l'état déjà à jour.
func (a *API) RunChatSweep() {
	now := time.Now().UTC()
	a.store.Lock()
	defer a.store.Unlock() // filet N°74 — libération garantie, même sur panique
	db := a.store.Data()
	closed := chatAutoCloseLocked(db, now)
	purged := chatPruneLocked(db, now)
	if closed > 0 || purged {
		a.store.Save()
	}
	if closed > 0 {
		log.Printf("chat : %d conversation(s) clôturée(s) automatiquement (inactivité > 15 min)", closed)
	}
	if purged {
		log.Printf("chat : rétention appliquée (fermées > 30 j, bot inactives > 7 j)")
	}
}
