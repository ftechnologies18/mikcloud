// agent_sleep.go — N°75 — veille adaptative des agents routeurs.
//
// AVANT : le scheduler MikCloud posé à l'installation tourne à 45 s À JAMAIS.
// Un check-in = ~400 o (réponse gzip), 1 920/jour/routeur → ~23 Mo/mois de
// bande passante par routeur sur le seul battement de cœur. Le mur « plan
// gratuit Render 5 Go » tombait vers ~100 routeurs, quel que soit l'usage
// réel.
//
// APRÈS : le CLOUD pilote le pas. À chaque check-in, il décide :
//
//	45 s  (actif)  — le routeur est « sous attention » : une console du
//	                 compte est ouverte (toute requête console authentifiée
//	                 marque le compte), un invité est sur le portail (page,
//	                 claim, poll de statut — le routeur du site est marqué),
//	                 ou des commandes attendent en file ;
//	240 s (veille) — plus rien ni personne n'attend le routeur. Les
//	                 read_states restent cadencés à 2 min (N°74) : ils sont
//	                 simplement servis aux check-in espacés.
//
// La bascule est une commande scheduler_set ordinaire (même FIFO, même
// garantie de livraison, même fermeture zombie que tout le protocole). Le
// retour « ok » pose Router.SchedulerSec — la vérité vient toujours du
// routeur, comme les signatures walled-garden/portail.
//
// LATENCE DE RÉVEIL : pire cas = un cycle de veille (≤ 4 min) — la PREMIÈRE
// action après une période d'inattention peut attendre le prochain check-in.
// Les marqueurs d'attention font que ce cas est rare : le gérant qui ouvre
// sa console marque le compte AVANT de cliquer, l'invité qui charge le
// portail marque le routeur AVANT de claimer (le claim est servi à 45 s).
//
// INTÉGRITÉ : un routeur en veille checke toutes les 4 min — les seuils
// « hors ligne » doivent s'élargir en conséquence (le moniteur de
// notifications et la fenêtre en ligne du sync-status utilisent
// AgentOfflineAfter / agentOnlineWindow, fonctions de SchedulerSec).
package api

import (
	"time"

	"mikcloud/hotspot-api/internal/agent"
	"mikcloud/hotspot-api/internal/model"
)

// Pas du scheduler MikCloud (secondes).
const (
	// agentFastSec — mode actif : l'installation pose cet intervalle, le
	// cloud le rétablit dès qu'une attention existe.
	agentFastSec = 45
	// agentSleepSec — mode veille : 3 min (la fourchette de l'audit était
	// 3-5 min). 180 s divise le battement de cœur par 4 → ~6 Mo/mois par
	// routeur → capacité ~350-450 routeurs sur le plan gratuit, TOUT EN
	// gardant la promesse de la page de claim (le pire cas d'attente du
	// voucher — un cycle de veille — reste sous la patience de 5 min de
	// la page ; la moyenne est 1,5 min, et le premier check-in qui suit le
	// chargement du portail repasse le routeur en mode rapide).
	agentSleepSec = 180
	// agentAttentionWindow — fraîcheur d'attention qui garde le mode rapide
	// après le DERNIER signal (console fermée, portail quitté). 10 min :
	// couvre la navigation entre vues sans faire vivre le mode rapide
	// éternellement.
	agentAttentionWindow = 10 * time.Minute
)

// markAttention — enregistre un signal d'attention pour une clé
// ("acc:<id>" compte console, "rt:<id>" routeur direct). Verrou dédié,
// JAMAIS pris sous le verrou du store ni l'inverse (le marqueur est posé
// depuis le middleware d'auth, hors de toute section critique).
func (a *API) markAttention(key string) {
	if key == "" {
		return
	}
	a.attnMu.Lock()
	a.attn[key] = time.Now().UTC()
	a.attnMu.Unlock()
}

// attentionFresh — true si la clé a été marquée dans la fenêtre d'attention.
func (a *API) attentionFresh(key string) bool {
	a.attnMu.Lock()
	at, ok := a.attn[key]
	a.attnMu.Unlock()
	return ok && time.Since(at) < agentAttentionWindow
}

// routerAttended — le routeur est-il « sous attention » ? Clés testées :
// le routeur lui-même (portail) puis son compte (console).
func (a *API) routerAttended(router *model.Router) bool {
	return a.attentionFresh("rt:"+router.ID) || a.attentionFresh("acc:"+router.AccountID)
}

// AgentOfflineAfter — seuil « hors ligne » effectif d'un routeur agent.
// Délègue à model.Router.EffectiveOfflineAfter (formule partagée avec le
// moniteur de notifications, cf. models.go).
func AgentOfflineAfter(cfgOfflineSec int, schedulerSec int) time.Duration {
	r := model.Router{SchedulerSec: schedulerSec}
	return r.EffectiveOfflineAfter(cfgOfflineSec)
}

// AgentSchedulerSec — pas de scheduler connu d'un routeur (0 = antérieur au
// N°75 : l'installation posait 45 s).
func AgentSchedulerSec(r *model.Router) int {
	return r.SchedulerSecEffective()
}

// agentOnlineWindow — fenêtre « en ligne » du sync-status pour un routeur
// (N°71 : fraîcheur du check-in plutôt que le champ Status). Élargie au
// régime de veille : 3 × le pas + marge réseau.
func agentOnlineWindow(r *model.Router) time.Duration {
	r2 := model.Router{SchedulerSec: r.SchedulerSecEffective()}
	return r2.EffectiveOfflineAfter(180)
}

// desiredAgentIntervalSec — le pas que le cloud VEUT pour ce routeur
// (à consulter sous le verrou du store).
func (a *API) desiredAgentIntervalSec(db *model.DB, router *model.Router) int {
	if a.routerAttended(router) {
		return agentFastSec
	}
	// Des commandes ACTIONNABLES attendent : le routeur doit passer vite pour
	// les servir — et repasser en mode rapide pour les suivantes (la file se
	// vide en un check-in, la veille revient au cycle d'après si plus rien ne
	// bouge).
	//
	// PIÈGE (découvert en traçant le flux) : les commandes de BALAYAGE
	// (read_state cadencé par N°74 — toutes les 2 min, donc TOUJOURS en file
	// au moment de décider ; walled_garden/hotspot_files re-sequencés par
	// signature) ne doivent PAS compter — sinon le routeur ping-pongerait
	// 45 s ↔ 240 s à CHAQUE check-in, pire que le régime fixe. Elles sont
	// idempotentes et parfaitement servies aux check-in espacés.
	for i := range db.Commands {
		c := &db.Commands[i]
		if c.RouterID != router.ID || c.Status != "queued" {
			continue
		}
		switch c.Kind {
		case model.CmdReadState, model.CmdWalledGarden, model.CmdHotspotFiles:
			continue // balayage : servi en veille sans dommage
		}
		return agentFastSec
	}
	return agentSleepSec
}

// ensureSchedulerIntervalLocked — aligne le pas du scheduler routeur sur la
// décision du cloud : enfile une commande scheduler_set si l'état connu
// (Router.SchedulerSec, vérité du dernier « ok » routeur) diverge. À appeler
// sous le verrou du store depuis handleAgentCmd — la commande rejoint la FIFO
// de CE check-in (aucun cycle supplémentaire).
func (a *API) ensureSchedulerIntervalLocked(db *model.DB, router *model.Router) {
	if router.Mode != "agent" {
		return
	}
	desired := a.desiredAgentIntervalSec(db, router)
	if desired == AgentSchedulerSec(router) {
		return
	}
	// Une bascule déjà en file/en vol suffit — le state machine converge au
	// check-in suivant (au pire un cycle à l'ancien pas).
	for i := range db.Commands {
		c := &db.Commands[i]
		if c.RouterID == router.ID && c.Kind == model.CmdSchedulerSet &&
			(c.Status == "queued" || c.Status == "sent") {
			return
		}
	}
	queueCommandLocked(db, router.AccountID, router.ID, model.CmdSchedulerSet, map[string]any{
		"name":        agent.SchedulerName,
		"intervalSec": desired,
	})
}
