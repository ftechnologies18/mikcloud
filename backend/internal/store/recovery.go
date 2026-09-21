// recovery.go — N°164 : boot résilient + récupération de la persistance.
//
// PROBLÈME (incident du 20/09) : un démarrage du backend pendant une
// indisponibilité PostgreSQL (quota Neon suspendu, panne hébergeur, fenêtre
// de bascule) était un log.Fatalf — store.New → OpenPG/Load en erreur → le
// process refusait de servir. Or l'état complet vit EN MÉMOIRE : le bon
// comportement est de démarrer DÉGRADÉ (mémoire seule) et de rétablir la
// persistance dès le retour de la base.
//
// GARDE ANTI-ÉCRASEMENT (exigence de revue) : le danger d'un mode dégradé
// naïf serait l'écrasement de la base au retour — la synchro pousserait
// l'état (quasi vide) par-dessus les 34 tables de production. Garde-fous
// structurels :
//
//  1. le syncreur n'est JAMAIS démarré avant qu'un Load complet ait réussi :
//     sans empreintes semées par un vrai Load, Sync ne peut pas émettre de
//     suppression (les « removed » naissent de la différence
//     empreintes↔mémoire) ;
//  2. au retour de la base, l'état de la fenêtre dégradée est FUSIONNÉ avec
//     l'état chargé (union par clé primaire, la mémoire gagne les
//     collisions, tombstones de purge respectées pour les usernames) : la
//     base retrouve son historique ET conserve les écritures de la fenêtre.
//
// Scénario dual (process démarré AVANT la panne — cas du 20/09) : aucun
// changement — le syncreur existant réessaie indéfiniment (backoff 5 s,
// empreintes conservées, « for !s.flush() ») et rattrape tout au retour,
// sans passer par ici.
package store

import (
	"log"
	"os"
	"strings"
	"time"

	"mikcloud/hotspot-api/internal/model"
)

// recoverRetryInterval — cadence de la boucle de récupération en mode
// dégradé. 15 s : assez court pour une reprise rapide au retour de la base
// (réveil Neon ~30 s ; Supabase répond instantanément), assez long pour ne
// pas noyer les journaux pendant une vraie panne.
const recoverRetryInterval = 15 * time.Second

// bootDegraded — démarre le store en MODE DÉGRADÉ : PostgreSQL est
// injoignable au boot (quota suspendu, panne, migration). Le service démarre
// sur l'état de mise en service (mémoire seule) : console et API répondent,
// l'admin d'environnement peut se connecter pour OBSERVER la dégradation
// (carte Santé + bannière), mais :
//   - les données existantes ne sont pas chargées (comptes/agents enregistrés
//     avant l'incident sont invisibles jusqu'au retour de la base) ;
//   - la persistance est SUSPENDUE : les marquages s'accumulent, RIEN n'est
//     poussé tant qu'aucun Load n'a réussi (garde anti-écrasement) ;
//   - recoveryLoop rétablit tout (fusion sans perte) dès le retour.
//
// Appelé par New quand OpenPG ou Load échoue — ne retourne jamais d'erreur :
// démarrer dégradé vaut mieux que de ne pas démarrer.
func (s *Store) bootDegraded(databaseURL string, cause error) {
	log.Printf("store: AVERTISSEMENT MODE DEGRADÉ — PostgreSQL injoignable au démarrage : %v", cause)
	log.Printf("store: MODE DEGRADÉ — persistance SUSPENDUE (aucune écriture base possible), recuperation en arriere-plan toutes les %v", recoverRetryInterval)
	log.Printf("store: MODE DEGRADÉ — l'etat memoire demarre VIDE ; les donnees existantes reviendront par FUSION au retour de la base (aucun ecrasement possible)")

	// Canaux du cycle de vie, posés AVANT tout Save() (pattern N°130).
	s.saveCh = make(chan struct{}, 1)
	s.closing = make(chan struct{})
	s.syncDone = make(chan struct{})

	// État de mise en service + garanties d'un boot propre : migrations
	// idempotentes puis admin d'environnement — l'opérateur doit pouvoir se
	// connecter pour voir l'état dégradé (bannière + carte Santé).
	s.db = BuildEmptyState()
	migrateMultiTenant(s.db)
	migrateDetachPlatform(s.db)
	migrateRemoveOperator(s.db)
	migrateUsageScopedPlans(s.db)
	migrateActiveTrialCap(s.db)
	if err := bootstrapAdmin(s.db); err != nil {
		// Sans admin la console est inaccessible, mais le service reste
		// utile (portail/agents) et la récupération re-jouera le bootstrap.
		log.Printf("store: MODE DEGRADÉ — bootstrap admin impossible : %v", err)
	}
	applyAdminOverride(s.db)

	s.pgActive.Store(true) // le mode PG est décidé : Save() ne passera plus par JSON
	s.pgDegraded.Store(true)
	s.pgDegradedSince.Store(time.Now().Unix())

	go s.recoveryLoop(databaseURL)
}

// recoveryLoop — retente OpenPG+Load jusqu'à réussir ou arrêt du service.
// Chaque échec est compté et journalisé (visible dans la carte Santé) ;
// l'intervalle est interruptible par Close.
func (s *Store) recoveryLoop(databaseURL string) {
	for {
		select {
		case <-s.closing:
			return
		default:
		}
		pg, err := OpenPG(databaseURL)
		if err == nil {
			var (
				db    *model.DB
				found bool
			)
			db, found, err = pg.Load()
			if err == nil {
				if s.installRecovered(pg, db, found) {
					return // installé (ou service en arrêt) — boucle terminée
				}
				// installRecovered a fermé pg et demande une nouvelle tentative.
				err = nil
			} else {
				pg.Close()
			}
		}
		if err != nil {
			s.pgRecoverTries.Add(1)
			msg := err.Error()
			s.pgRecoverErr.Store(&msg)
			log.Printf("store: recuperation PostgreSQL differee (tentative %d) : %v", s.pgRecoverTries.Load(), err)
		}
		select {
		case <-s.closing:
			return
		case <-time.After(recoverRetryInterval):
		}
	}
}

// installRecovered — installe la persistance récupérée : fusion sous le
// verrou global, garanties de boot propre, keep-alive, puis DERNIER
// geste — démarrage du syncreur. Retourne false pour retenter (pg fermé).
func (s *Store) installRecovered(pg *PG, db *model.DB, found bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Garde fermeture (1) : le service s'arrête pendant la lecture — on
	// abandonne proprement, le boot suivant refera le travail.
	s.saveMu.Lock()
	if s.closed {
		s.saveMu.Unlock()
		pg.Close()
		return true
	}
	s.saveMu.Unlock()

	if found {
		// GARDE ANTI-ÉCRASEMENT (2) — fusion : les lignes de la base absentes
		// de la mémoire sont restaurées (l'historique d'avant-pagne revient) ;
		// les lignes créées pendant la fenêtre dégradée sont conservées (elles
		// ne vivent qu'en mémoire) ; la mémoire gagne les collisions.
		if n := mergeRecovered(s.db, db); n > 0 {
			log.Printf("store: recuperation — %d ligne(s) restauree(s) depuis PostgreSQL (fusion, la memoire garde la main sur les collisions)", n)
		}
	}

	// Garanties identiques à un boot propre sur l'état fusionné.
	migrateMultiTenant(s.db)
	migrateDetachPlatform(s.db)
	migrateRemoveOperator(s.db)
	migrateUsageScopedPlans(s.db)
	migrateActiveTrialCap(s.db)
	if err := bootstrapAdmin(s.db); err != nil {
		// Erreur de CONFIGURATION (ADMIN_PASSWORD absent et aucun admin
		// dans l'état fusionné), pas de base : tolérée et journalisée — la
		// récupération de la persistance ne doit pas rester bloquée pour un
		// problème de credentials (en production ADMIN_PASSWORD est posé,
		// bootstrapAdmin rend nil dès qu'un admin existe).
		log.Printf("store: RECUPERATION — bootstrap admin impossible : %v (service persisté, connexion console indisponible)", err)
	}
	applyAdminOverride(s.db)

	// Keep-alive identique au boot normal (parité de comportement).
	kaMode := os.Getenv("NEON_KEEPALIVE")
	if kaMode == "" {
		kaMode = "business"
	}
	if kaMode != "off" {
		pg.StartKeepAlive(kaMode)
	}

	// Les empreintes de pg décrivent l'état CHARGÉ (Load → rebuildHashes) :
	// la première synchro pousse exactement les lignes créées pendant la
	// fenêtre dégradée (présentes en mémoire, absentes des empreintes) et ne
	// peut rien supprimer (tout le chargé est en mémoire après fusion).
	s.saveMu.Lock()
	// Garde fermeture (2) : re-check sous le verrou d'installation.
	if s.closed {
		s.saveMu.Unlock()
		pg.Close()
		return true
	}
	s.pg = pg
	s.syncRunning = true
	s.dirtyAll = true // diff complet garanti au premier flush
	s.saveMu.Unlock()
	select {
	case s.saveCh <- struct{}{}:
	default:
	}

	s.pgDegraded.Store(false)
	s.pgRecoveredAt.Store(time.Now().Unix())

	go s.syncLoop()

	log.Printf("store: RECUPERATION REUSSIE — persistance PostgreSQL retablie, syncreur relance (diff complet au prochain marquage)")
	return true
}

// mergeSlice — union par clé primaire : les lignes src dont la clé n'existe
// pas encore dans dst sont ajoutées (dst — la mémoire — gagne les
// collisions). Retourne le nombre de lignes ajoutées.
func mergeSlice[T any](dst *[]T, src []T, idOf func(*T) string) int {
	if len(src) == 0 {
		return 0
	}
	seen := make(map[string]struct{}, len(*dst))
	for i := range *dst {
		seen[idOf(&(*dst)[i])] = struct{}{}
	}
	added := 0
	for i := range src {
		id := idOf(&src[i])
		if _, ok := seen[id]; ok {
			continue
		}
		*dst = append(*dst, src[i])
		added++
	}
	return added
}

// mergeHotspotUsers — fusion des utilisateurs hotspot avec le filtre
// anti-résurgence : un username purgé (tombstone vivant, des deux côtés)
// ne doit pas ressusciter par la fusion. Les tombstones expirés bloquent
// aussi (conservateur, sans danger : la purge les nettoie en temps voulu).
func mergeHotspotUsers(dst, src *model.DB) int {
	blocked := make(map[string]struct{})
	for i := range dst.PurgeTombstones {
		blocked[strings.ToLower(dst.PurgeTombstones[i].Username)] = struct{}{}
	}
	for i := range src.PurgeTombstones {
		blocked[strings.ToLower(src.PurgeTombstones[i].Username)] = struct{}{}
	}
	if len(blocked) == 0 {
		return mergeSlice(&dst.HotspotUsers, src.HotspotUsers, hotspotUserSpec.idOf)
	}
	seen := make(map[string]struct{}, len(dst.HotspotUsers))
	for i := range dst.HotspotUsers {
		seen[hotspotUserSpec.idOf(&dst.HotspotUsers[i])] = struct{}{}
	}
	added := 0
	for i := range src.HotspotUsers {
		id := hotspotUserSpec.idOf(&src.HotspotUsers[i])
		if _, ok := seen[id]; ok {
			continue
		}
		if _, ok := blocked[strings.ToLower(src.HotspotUsers[i].Username)]; ok {
			continue // purgé avant l'incident : il reste purgé
		}
		dst.HotspotUsers = append(dst.HotspotUsers, src.HotspotUsers[i])
		added++
	}
	return added
}

// mergeRecovered — fusion l'état chargé depuis PostgreSQL (src) DANS l'état
// mémoire (dst) : union par clé primaire sur toutes les collections du Load
// (toute table ajoutée à pg_load.go doit être ajoutée ici), cartes par
// compte complétées, horloges globales conservées au plus récent. Retourne
// le nombre de lignes restaurées.
func mergeRecovered(dst, src *model.DB) int {
	if dst == nil || src == nil {
		return 0
	}
	n := 0
	n += mergeSlice(&dst.Accounts, src.Accounts, accountSpec.idOf)
	n += mergeSlice(&dst.Users, src.Users, adminSpec.idOf)
	n += mergeSlice(&dst.Routers, src.Routers, routerSpec.idOf)
	n += mergeSlice(&dst.Profiles, src.Profiles, profileSpec.idOf)
	n += mergeHotspotUsers(dst, src)
	n += mergeSlice(&dst.Batches, src.Batches, batchSpec.idOf)
	n += mergeSlice(&dst.Resellers, src.Resellers, resellerSpec.idOf)
	n += mergeSlice(&dst.SellSessions, src.SellSessions, sellSessionSpec.idOf)
	n += mergeSlice(&dst.PasswordResets, src.PasswordResets, passwordResetSpec.idOf)
	n += mergeSlice(&dst.Transactions, src.Transactions, transactionSpec.idOf)
	n += mergeSlice(&dst.Sessions, src.Sessions, sessionSpec.idOf)
	n += mergeSlice(&dst.Devices, src.Devices, deviceSpec.idOf)
	n += mergeSlice(&dst.Activity, src.Activity, activitySpec.idOf)
	n += mergeSlice(&dst.Sales, src.Sales, saleSpec.idOf)
	n += mergeSlice(&dst.Commands, src.Commands, commandSpec.idOf)
	n += mergeSlice(&dst.Templates, src.Templates, templateSpec.idOf)
	n += mergeSlice(&dst.UserLogs, src.UserLogs, userLogSpec.idOf)
	n += mergeSlice(&dst.IPBindings, src.IPBindings, ipBindingSpec.idOf)
	n += mergeSlice(&dst.SchedulerTasks, src.SchedulerTasks, schedulerTaskSpec.idOf)
	n += mergeSlice(&dst.Traffic, src.Traffic, trafficSpec.idOf)
	n += mergeSlice(&dst.LineQuality, src.LineQuality, lineQualitySpec.idOf)
	n += mergeSlice(&dst.NotifLog, src.NotifLog, notifLogSpec.idOf)
	n += mergeSlice(&dst.BillingRequests, src.BillingRequests, billingRequestSpec.idOf)
	n += mergeSlice(&dst.GeniusPaySubs, src.GeniusPaySubs, geniusPaySubSpec.idOf)
	n += mergeSlice(&dst.PurgeTombstones, src.PurgeTombstones, purgeTombstoneSpec.idOf)
	n += mergeSlice(&dst.JoinLinks, src.JoinLinks, joinLinkSpec.idOf)
	n += mergeSlice(&dst.RegistrationRequests, src.RegistrationRequests, registrationRequestSpec.idOf)
	n += mergeSlice(&dst.WifiSites, src.WifiSites, wifiSiteSpec.idOf)
	n += mergeSlice(&dst.WifiGuests, src.WifiGuests, wifiGuestSpec.idOf)
	n += mergeSlice(&dst.PromoEvents, src.PromoEvents, promoEventSpec.idOf)
	n += mergeSlice(&dst.ChatConversations, src.ChatConversations, chatConversationSpec.idOf)
	n += mergeSlice(&dst.ChatMessages, src.ChatMessages, chatMessageSpec.idOf)
	n += mergeSlice(&dst.Announcements, src.Announcements, announcementSpec.idOf)

	// Cartes par compte : complétées, jamais écrasées (la mémoire — réglages
	// modifiés pendant la fenêtre — garde la main).
	if dst.SettingsByAccount == nil {
		dst.SettingsByAccount = map[string]model.Settings{}
	}
	for k, v := range src.SettingsByAccount {
		if _, ok := dst.SettingsByAccount[k]; !ok {
			dst.SettingsByAccount[k] = v
			n++
		}
	}
	if dst.NotifSettings == nil {
		dst.NotifSettings = map[string]model.NotificationSettings{}
	}
	for k, v := range src.NotifSettings {
		if _, ok := dst.NotifSettings[k]; !ok {
			dst.NotifSettings[k] = v
			n++
		}
	}

	// Horloges globales du moteur : la plus récente l'emporte (le Tick a
	// continué de tourner en mémoire pendant la fenêtre dégradée).
	if src.LastTick.After(dst.LastTick) {
		dst.LastTick = src.LastTick
	}
	if src.LastSweep.After(dst.LastSweep) {
		dst.LastSweep = src.LastSweep
	}
	return n
}
