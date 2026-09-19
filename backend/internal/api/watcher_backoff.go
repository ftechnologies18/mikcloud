// Backoff des watchers — N°159 — le moteur de volume, volet re-files.
//
// CONSTAT production 19/09 (suite incident N°157) : un watcher dont la
// commande ÉCHOUE sur le routeur re-file à CHAQUE check-in — shield ×808 et
// safewifi ×107 en 12 h sur ProMax WIFI (toutes « echec_des_regles... »
// depuis le boot du routeur), shield+safewifi ×110/h sur CYBER S.C : ~950
// commandes/jour qui n'apportent AUCUNE convergence (l'état routeur ne change
// pas entre deux tentatives espacées de 20 s) et nourrissent le ping-pong de
// fraîcheur post-écriture.
//
// PRINCIPE : la cadence de re-file suit l'ISSUE de la dernière commande du
// watcher sur ce routeur —
//   - « ok » : réinitialisation immédiate (le watcher redevient réactif :
//     auto-réparation 6 h et changements de config servis au check-in
//     suivant, contrat N°49/80/81/82/88/104 inchangé) ;
//   - « error » : palier progressif — 1 min, 5 min, 15 min puis 30 min de
//     plancher entre deux re-files. Un échec PERSISTANT n'est plus un
//     martèlement : ~48 tentatives/jour au palier maximal au lieu de ~2 000,
//     chaque rapport restant journalisé (l'opérateur voit l'échec, la
//     signature n'étant posée que sur un retour vérifié).
//
// ÉTAT VOLATIL (map en mémoire, jamais synchronisé) : au redémarrage de l'API
// tout watcher re-tente immédiatement — les pannes passagères (reboot
// routeur, blip réseau, suspension Render) gardent le comportement historique
// de re-file au check-in suivant ; seuls les échecs RÉPÉTÉS ralentissent.
// Accédé UNIQUEMENT sous le verrou du store (ensure* du check-in comme
// handleAgentResult y vivent tous — miroir readStateDone N°74).
package api

import (
	"time"

	"mikcloud/hotspot-api/internal/model"
)

// watcherBackoffSteps — paliers du délai entre deux re-files après des
// échecs consécutifs (indexé par fails-1 ; au-delà, watcherBackoffCap).
var watcherBackoffSteps = []time.Duration{
	1 * time.Minute,
	5 * time.Minute,
	15 * time.Minute,
}

// watcherBackoffCap — plafond du palier : un watcher en échec permanent
// reste CONVERGENT (une tentative toutes les 30 min suffit à détecter le
// retour du routeur en état de converger) sans marteler.
const watcherBackoffCap = 30 * time.Minute

// watcherBackoffDelay — délai courant après fails échecs consécutifs.
func watcherBackoffDelay(fails int) time.Duration {
	if fails <= 0 {
		return 0
	}
	if fails > len(watcherBackoffSteps) {
		return watcherBackoffCap
	}
	return watcherBackoffSteps[fails-1]
}

// watcherBackoffKinds — les kinds du bucket différé dont la convergence
// re-file tant que la signature vérifiée n'est pas posée. Les lectures
// d'outils (ping, read_*) et les commandes métier ponctuelles (user_add…)
// n'en font PAS partie : leur échec est visible dans l'historique et le
// geste du gérant les relance — seul le martèlement AUTOMATIQUE est borné.
func watcherBackoffKind(kind string) bool {
	switch kind {
	case model.CmdShield, model.CmdSafeWifi, model.CmdFamilyGuard, model.CmdAntiVpn,
		model.CmdQueueEnsure, model.CmdQueueRemove:
		return true
	}
	return false
}

// watcherKey — clé du backoff : le routeur ET le kind (les watchers sont
// indépendants — un shield en échec ne ralentit pas un queue_ensure sain).
func watcherKey(routerID, kind string) string {
	return routerID + "|" + kind
}

// watcherBackoffBlocks — vrai si le re-file de CE watcher pour CE routeur
// doit attendre (dernière commande en échec plus récente que le palier
// courant). Nil-map safe : sans état (tests, premier boot), jamais bloqué.
func (a *API) watcherBackoffBlocks(routerID, kind string, now time.Time) bool {
	last, ok := a.watcherFailAt[watcherKey(routerID, kind)]
	if !ok {
		return false
	}
	return now.Sub(last) < watcherBackoffDelay(a.watcherFailN[watcherKey(routerID, kind)])
}

// recordWatcherError — un rapport « error » fait monter d'un palier.
func (a *API) recordWatcherError(routerID, kind string, now time.Time) {
	k := watcherKey(routerID, kind)
	if a.watcherFailN == nil {
		a.watcherFailN = map[string]int{}
	}
	if a.watcherFailAt == nil {
		a.watcherFailAt = map[string]time.Time{}
	}
	a.watcherFailN[k]++
	a.watcherFailAt[k] = now
}

// resetWatcherBackoff — un rapport « ok » réinitialise (le watcher redevient
// réactif au check-in suivant — contrat de convergence inchangé).
func (a *API) resetWatcherBackoff(routerID, kind string) {
	delete(a.watcherFailN, watcherKey(routerID, kind))
	delete(a.watcherFailAt, watcherKey(routerID, kind))
}
