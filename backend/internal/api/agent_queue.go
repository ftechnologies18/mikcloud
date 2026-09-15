// File de commandes agent : enqueue, reprise des lectures obsolètes, watcher, read-state dû.
// Extrait du monolithe agent_handlers.go (N°88) — même package, contenu inchangé.
package api

import (
	"log"
	"mikcloud/hotspot-api/internal/agent"
	"mikcloud/hotspot-api/internal/model"
	"time"
)

// staleSentReadKinds — kinds dont la re-exécution est sans effet de bord
// (idempotents) : seuls ceux-là sont repris s'ils restent « sent » sans
// rapport (les écritures non idempotentes ne sont jamais re-exécutées).
// Audit N°31 : walled_garden rejoint la liste — le bloc est conçu
// idempotent (remove+add des seules règles marquées mikcloud-wg). Sans
// cela, UN rapport perdu (blip réseau entre l'import et le fetch de
// rapport, reboot en cours de check-in…) laissait la commande « sent » à
// jamais : ensureWalledGardenLocked la croyait « en vol » et ne la
// re-filait jamais — walled-garden jamais appliqué, en silence.
var staleSentReadKinds = map[string]bool{
	model.CmdReadState:     true,
	model.CmdReadDhcp:      true,
	model.CmdReadHosts:     true,
	model.CmdReadCookies:   true,
	model.CmdReadLog:       true,
	model.CmdReadScheduler: true,
	model.CmdReadResources: true,
	model.CmdImportHotspot: true,
	model.CmdPing:          true,
	model.CmdWalledGarden:  true, // N°31 : idempotent (marqueur mikcloud-wg)
	model.CmdHotspotFiles:  true, // N°35 : idempotent (surcharge atomique des fichiers du portail)
	model.CmdSafeWifi:      true, // N°80 : idempotent (marqueur mikcloud-safewifi — remove-then-add)
	model.CmdShield:        true, // N°81 : idempotent (marqueur mikcloud-shield — remove-then-add)
	model.CmdFamilyGuard:   true, // N°82 : idempotent (marqueur mikcloud-familyguard — remove-then-add)
	model.CmdAntiVpn:       true, // N°88 : idempotent (marqueur mikcloud-antivpn — remove-then-add)
	model.CmdDevicePause:   true, // N°101 : idempotent (marqueur mikcloud-pause — remove-then-add)
	model.CmdQueueEnsure:   true, // N°104 : idempotent (file mikcloud-qos — create-or-set + relecture de vérification)
	model.CmdQueueRemove:   true, // N°104 : idempotent (retrait de la file agrégat — détache les profils d'abord, prouve la disparition)
	model.CmdQuotaEnsure:   true, // N°106 : idempotent (scheduler mikcloud-quota — remove-then-add, pattern watcher N°77)
	model.CmdRouterOSCheck: true, // N°115 : lecture (check-for-updates) — re-exécution sans effet de bord
}

// staleSentLimit — au-delà de cette ancienneté sans rapport, une commande
// idempotente « sent » est considérée perdue et repart en file.

// staleSentLimit — au-delà de cette ancienneté sans rapport, une commande
// idempotente « sent » est considérée perdue et repart en file.
const staleSentLimit = 10 * time.Minute

// requeueStaleReadsLocked — remet en file les commandes IDEMPOTENTES
// « sent » zombies — lectures ET walled_garden (sous verrou ; Save à charge
// de l'appelant, comme le reste du flux).

// requeueStaleReadsLocked — remet en file les commandes IDEMPOTENTES
// « sent » zombies — lectures ET walled_garden (sous verrou ; Save à charge
// de l'appelant, comme le reste du flux).
func requeueStaleReadsLocked(db *model.DB, routerID string) {
	lim := time.Now().Add(-staleSentLimit).Format(time.RFC3339)
	changed := false
	for i := range db.Commands {
		c := &db.Commands[i]
		if c.RouterID == routerID && c.Status == "sent" && staleSentReadKinds[c.Kind] && c.SentAt != "" && c.SentAt < lim {
			c.Status = "queued"
			c.SentAt = ""
			changed = true
		}
	}
	if changed {
		log.Printf("agent/cmd: commandes idempotentes « sent » sans rapport reprises en file (routeur %s)", routerID)
	}
}

// touchAgent — marque le routeur en ligne (sous verrou).

// queueCommandLocked — dépose une commande en file (sous verrou ; Save à charge
// de l'appelant). Déduplique les read_state déjà en attente. La commande porte
// l'identifiant du compte du routeur (isolation multi-tenant).
func queueCommandLocked(db *model.DB, acc, routerID, kind string, payload map[string]any) *model.Command {
	if kind == model.CmdReadState || kind == model.CmdImportHotspot {
		for i := range db.Commands {
			if db.Commands[i].RouterID == routerID && db.Commands[i].Kind == kind && db.Commands[i].Status == "queued" {
				return &db.Commands[i]
			}
		}
	}
	cmd := model.Command{
		ID:        model.NewID("c-"),
		RouterID:  routerID,
		AccountID: acc,
		Kind:      kind,
		Payload:   payload,
		Status:    "queued",
		CreatedAt: model.NowISO(),
	}
	db.Commands = append(db.Commands, cmd)
	return &db.Commands[len(db.Commands)-1]
}

// queueReadChunkLocked — N°76 — enfile un CHUNK (fenêtre [start, start+count))
// d'un cycle read_state. Ne passe PAS par la dédup base de queueCommandLocked :
// les chunks d'un cycle coexistent en file ; la dédup par OFFSET évite le
// doublon (reprise zombie N°73 : la commande sent redevient queued, jamais
// dupliquée).

// queueReadChunkLocked — N°76 — enfile un CHUNK (fenêtre [start, start+count))
// d'un cycle read_state. Ne passe PAS par la dédup base de queueCommandLocked :
// les chunks d'un cycle coexistent en file ; la dédup par OFFSET évite le
// doublon (reprise zombie N°73 : la commande sent redevient queued, jamais
// dupliquée).
func queueReadChunkLocked(db *model.DB, acc, routerID string, start, count int) *model.Command {
	for i := range db.Commands {
		c := &db.Commands[i]
		if c.RouterID == routerID && c.Kind == model.CmdReadState && c.Status == "queued" &&
			int(plPayloadInt(c.Payload, "start")) == start {
			return c
		}
	}
	cmd := model.Command{
		ID:        model.NewID("c-"),
		RouterID:  routerID,
		AccountID: acc,
		Kind:      model.CmdReadState,
		Payload:   map[string]any{"start": start, "count": count},
		Status:    "queued",
		CreatedAt: model.NowISO(),
	}
	db.Commands = append(db.Commands, cmd)
	return &db.Commands[len(db.Commands)-1]
}

// queueReadCycleRestLocked — N°76 — au résultat du chunk 0, enfile d'un coup
// TOUTES les fenêtres restantes du cycle (≤ MaxReadChunks fenêtres au total) :
// servies au check-in suivant par paquets de 10 (limite FIFO/check-in), le
// cycle progresse sans intervention même en veille.

// queueReadCycleRestLocked — N°76 — au résultat du chunk 0, enfile d'un coup
// TOUTES les fenêtres restantes du cycle (≤ MaxReadChunks fenêtres au total) :
// servies au check-in suivant par paquets de 10 (limite FIFO/check-in), le
// cycle progresse sans intervention même en veille.
func queueReadCycleRestLocked(db *model.DB, router *model.Router, total, count int) {
	winCap := agent.MaxReadChunks * count
	for s := count; s < total && s < winCap; s += count {
		queueReadChunkLocked(db, router.AccountID, router.ID, s, count)
	}
}

// queueReadStateFreshLocked — N°76 — enfile un read_state de BASE (chunk 0) si
// et seulement si AUCUN cycle n'est en cours pour ce routeur (aucun read_state
// queued/sent — chunks compris). Un cycle paginé en cours EST déjà une
// synchronisation : le casser par un chunk 0 concurrent désordonnerait
// l'accumulateur (chunks orphelins). Retourne la commande enfilée, ou le
// cycle en cours (l'appelant y lit l'ID — la fraîcheur viendra de lui).

// queueReadStateFreshLocked — N°76 — enfile un read_state de BASE (chunk 0) si
// et seulement si AUCUN cycle n'est en cours pour ce routeur (aucun read_state
// queued/sent — chunks compris). Un cycle paginé en cours EST déjà une
// synchronisation : le casser par un chunk 0 concurrent désordonnerait
// l'accumulateur (chunks orphelins). Retourne la commande enfilée, ou le
// cycle en cours (l'appelant y lit l'ID — la fraîcheur viendra de lui).
func queueReadStateFreshLocked(db *model.DB, router *model.Router) *model.Command {
	for i := range db.Commands {
		c := &db.Commands[i]
		if c.RouterID == router.ID && c.Kind == model.CmdReadState && (c.Status == "queued" || c.Status == "sent") {
			return c // cycle en cours : ne pas le casser
		}
	}
	return queueCommandLocked(db, router.AccountID, router.ID, model.CmdReadState, map[string]any{})
}

// profileRef — construit la référence compacte d'un profil pour les payloads.

// profileRef — construit la référence compacte d'un profil pour les payloads.
func profileRef(p model.Profile) map[string]any {
	return map[string]any{
		"name":              agent.SanitizeName(p.Name),
		"rateLimit":         p.RateLimit,
		"sessionTimeoutMin": p.SessionTimeoutMin,
		"sharedUsers":       p.SharedUsers,
		"lockFirstDevice":   p.LockFirstDevice,
		// Parité Mikhmon : pools/queues RouterOS portés par le profil
		// (chaque user_add / voucher_batch aligne le profil sur le cloud).
		"addressPool": p.AddressPool,

		"parentQueue": p.ParentQueue,

		// N°106 — mode bridage : porté par le profil dans CHAQUE commande

		// user_add/voucher_batch/profile_set (l'agent y lit ses scripts

		// on-login/on-logout génériques ; le débit de bridage, lui, voyage

		// au niveau racine du payload — cf. throttleRate).

		"quotaMode": p.QuotaModeEffective(),
	}
}

// purgeOldCommands — supprime les commandes terminées de plus de 7 jours (sous verrou).
//
// N°73 — ferme d'abord les zombies « sent » orphelins de plus de 7 jours :
// un rapport perdu (blip réseau entre l'exécution routeur et le POST
// /agent/result, fenêtre de suspension plateforme, reboot du routeur en
// plein check-in…) laissait la commande « sent » À VIE — les écritures ne
// sont jamais re-exécutées (cf. requeueStaleReadsLocked : seules les
// idempotentes repartent en file, double-exécution interdite) et RIEN ne
// fermait ces lignes : le compteur « zombies » de la carte Maintenance
// affichait éternellement une commande à l'issue réelle inconnue (vécu au
// réveil post-suspension du 10/09 : un user_remove dont SEUL le rapport
// avait été perdu). Après 7 jours sans retour, la commande est close
// « error » avec un message explicite et DoneAt = maintenant : visible
// 7 jours dans l'historique (l'opérateur constate la fermeture), puis
// balayée par le nettoyage ci-dessous comme tout done/error ancien —
// double phase. Le statut « error » (et non « done ») est le seul
// honnête : l'issue réelle côté routeur est inconnue.
//
// Les « queued » ne sont PAS touchées : un routeur muet qui revient les
// exécute et les rapporte normalement — seul le « sent » sans rapport est
// une fuite. Les sent récents gardent leur fenêtre de reprise
// idempotente (10 min) puis d'observation.

// purgeOldCommands — supprime les commandes terminées de plus de 7 jours (sous verrou).
//
// N°73 — ferme d'abord les zombies « sent » orphelins de plus de 7 jours :
// un rapport perdu (blip réseau entre l'exécution routeur et le POST
// /agent/result, fenêtre de suspension plateforme, reboot du routeur en
// plein check-in…) laissait la commande « sent » À VIE — les écritures ne
// sont jamais re-exécutées (cf. requeueStaleReadsLocked : seules les
// idempotentes repartent en file, double-exécution interdite) et RIEN ne
// fermait ces lignes : le compteur « zombies » de la carte Maintenance
// affichait éternellement une commande à l'issue réelle inconnue (vécu au
// réveil post-suspension du 10/09 : un user_remove dont SEUL le rapport
// avait été perdu). Après 7 jours sans retour, la commande est close
// « error » avec un message explicite et DoneAt = maintenant : visible
// 7 jours dans l'historique (l'opérateur constate la fermeture), puis
// balayée par le nettoyage ci-dessous comme tout done/error ancien —
// double phase. Le statut « error » (et non « done ») est le seul
// honnête : l'issue réelle côté routeur est inconnue.
//
// Les « queued » ne sont PAS touchées : un routeur muet qui revient les
// exécute et les rapporte normalement — seul le « sent » sans rapport est
// une fuite. Les sent récents gardent leur fenêtre de reprise
// idempotente (10 min) puis d'observation.
func purgeOldCommands(db *model.DB) {
	lim := time.Now().UTC().Add(-7 * 24 * time.Hour).Format(time.RFC3339)
	for i := range db.Commands {
		c := &db.Commands[i]
		if c.Status == "sent" && c.SentAt != "" && c.SentAt < lim {
			c.Status = "error"
			c.Result = map[string]any{"message": "rapport perdu (zombie « sent » fermé après 7 j sans retour)"}
			c.DoneAt = model.NowISO()
		}
	}
	kept := db.Commands[:0]
	for _, c := range db.Commands {
		if (c.Status == "done" || c.Status == "error") && c.DoneAt != "" && c.DoneAt < lim {
			continue
		}
		kept = append(kept, c)
	}
	db.Commands = kept
}

// parseRosUptime — parse une durée RouterOS ("3w1d02:15:30", "02:15:30", "2h30m", "45s").

// ensureWatcherLocked — N°77 — veilleur d'invités : garantit que le scheduler
// mikcloud-watch est déployé sur ce routeur (check-in 20 s pendant qu'un hôte
// non autorisé est présent — un invité est SUR le portail, son claim est servi
// en ≤ 20 s au lieu d'attendre le réveil du scheduler principal, jusqu'à 180 s
// de veille N°75). Pattern walled-garden/scheduler_set : le drapeau
// Router.WatcherOK n'est posé qu'au retour « ok » de la commande — jamais à la
// mise en file. Faux/absent = routeur antérieur au N°77, échec, ou veilleur
// effacé à la main → re-file au check-in suivant (auto-réparation). À appeler
// sous le verrou du store depuis handleAgentCmd.
func (a *API) ensureWatcherLocked(db *model.DB, router *model.Router) {
	if router.Mode != "agent" || router.WatcherOK {
		return
	}
	// Un déploiement déjà en file ou en vol suffit — le rapport tranchera
	// (ok → WatcherOK posé, error → re-file au check-in suivant).
	for i := range db.Commands {
		c := &db.Commands[i]
		if c.RouterID == router.ID && c.Kind == model.CmdWatcherEnsure &&
			(c.Status == "queued" || c.Status == "sent") {
			return
		}
	}
	queueCommandLocked(db, router.AccountID, router.ID, model.CmdWatcherEnsure, map[string]any{})
}

// ensureQuotaThrottleLocked — N°106 : converge le scheduler mikcloud-quota
// (tick 20 s — pose/retire les files mikthrottle-<user>) vers les routeurs
// agents du compte — UNIQUEMENT si le compte possède au moins un profil en
// mode throttle : un site qui ne bridle jamais ne consomme rien (économie
// de veille N°75 entière — le tick serait de toute façon inerte sans
// marqueur mikq:, mais on n'installe même pas le scheduler). Pattern
// watcher N°77 : re-file tant que le retour « ok » n'est pas arrivé
// (QuotaSchedOK posé au rapport, cf. applyAgentResult).
//
// N°113 — la GÉNÉRATION du tick compte autant que sa présence : le script
// est figé dans le on-event du scheduler routeur, un routeur convergé sur
// une génération ANCIENNE (QuotaSchedVer < agent.QuotaTickVersion) doit
// être re-déployé — c'est LE canal qui a porté le correctif du terrain
// N°106 (le tick v1 posait la file de bridage SOUS la dynamique <user> :
// premier-match gagnant, aucun bridage, 200 Mo consommés sans bridage).
// La version voyage dans le payload et revient dans le rapport (pattern
// sel safeWifiRulesVersion N°80).
func (a *API) ensureQuotaThrottleLocked(db *model.DB, router *model.Router) {
	if router.Mode != "agent" || (router.QuotaSchedOK && router.QuotaSchedVer >= agent.QuotaTickVersion) {
		return
	}
	anyThrottle := false
	for i := range db.Profiles {
		if db.Profiles[i].AccountID == router.AccountID &&
			db.Profiles[i].QuotaModeEffective() == model.QuotaModeThrottle {
			anyThrottle = true
			break
		}
	}
	if !anyThrottle {
		return
	}
	// Un déploiement déjà en file ou en vol suffit — le rapport tranchera
	// (ok → QuotaSchedOK posé, error → re-file au check-in suivant).
	for i := range db.Commands {
		c := &db.Commands[i]
		if c.RouterID == router.ID && c.Kind == model.CmdQuotaEnsure &&
			(c.Status == "queued" || c.Status == "sent") {
			return
		}
	}
	queueCommandLocked(db, router.AccountID, router.ID, model.CmdQuotaEnsure, map[string]any{
		// N°113 — génération du tick embarquée : le rapport la reposera
		// (vérité de CE script-ci, jamais d'un ordre en vol antérieur).
		"tickVer": agent.QuotaTickVersion,
	})
}

// safeWifiRulesVersion — sel de version des règles SafeWiFi : toute
// évolution de la FORME des règles (changement de résolveur, nouveau
// marquage, champs supplémentaires) change ce sel → chaque routeur en
// ligne reçoit la mise à niveau automatiquement à son premier check-in
// (ensureSafeWifiLocked voit un mismatch → re-file). Pattern walled-garden
// N°48. sw-v2 (N°85) : règles NAT en tête de table (place-before=0, une
// règle dstnat antérieure ne peut plus passer devant), blocage DoT
// (tcp/853) et DoH (tcp/443 vers liste mikcloud-safewifi-doh) par
// serveur hotspot, coupure DNS/DoT/DoH IPv6 (best-effort).
