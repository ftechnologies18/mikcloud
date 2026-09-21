// Package store — persistance + moteur de simulation (tick).
//
// Deux modes de persistance, choisis automatiquement au démarrage :
//   - DATABASE_URL défini  → PostgreSQL (production : Render + Neon),
//     schéma relationnel + synchro différentielle (voir pg.go) ;
//   - sinon                → fichier JSON atomique (développement local).
package store

import (
	crand "crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"mikcloud/hotspot-api/internal/auth"
	"mikcloud/hotspot-api/internal/model"
	"mikcloud/hotspot-api/internal/secretbox"
)

// Store — base de données en mémoire protégée par un mutex global, persistée
// soit en PostgreSQL (synchro différentielle), soit en JSON atomique
// (écriture tmp + rename).
type Store struct {
	mu   sync.Mutex
	db   *model.DB
	path string // mode JSON uniquement
	pg   *PG    // mode PostgreSQL uniquement

	// N°130 — P0 audit performance : sauvegarde ASYNCHRONE (mode PG
	// uniquement). Avant, CHAQUE lecture pollée (dashboard 15 s, sessions
	// 10 s, listes utilisateurs, check-ins agent…) exécutait un Save()
	// complet SOUS LE VERROU GLOBAL : re-hash JSON des 31 tables +
	// transaction Neon (jusqu'à syncTimeout = 20 s sur un incident) — le
	// 0,1 vCPU Render sérialisait tout, et chaque clic attendait la
	// base. Désormais Save() pose un drapeau « sale » (micro-verrou
	// dédié) et réveille le syncreur de fond : celui-ci photographie
	// l'état sous le verrou (CloneDeep, quelques ms) puis synchronise
	// HORS verrou — une requête ne peut plus être bloquée par une
	// sauvegarde. Le mode JSON (dev/E2E) reste synchrone et inchangé.
	//
	// N°133 — P1 audit performance : le marquage devient PAR TABLE.
	// Save() = diff COMPLET (toutes les tables — défaut sûr, inchangé
	// pour tout handler métier) ; SaveTables() = diff CIBLÉ sur les
	// seules tables marquées sales par un chemin de lecture (Tick +
	// enforcement retournent leur empreinte, cf. tables.go). En état de
	// croisière d'un parc agent, la synchro re-hashe ~6 lignes au lieu
	// de ~8 500 : le CPU Render rendu au service par chaque flush.
	saveMu      sync.Mutex
	dirtyAll    bool
	dirtyTables map[string]bool
	closed      bool
	saveCh      chan struct{} // réveil du syncreur (capacité 1, signal non bloquant)
	closing     chan struct{} // arrêt propre (fermé par Close)
	syncDone    chan struct{} // syncreur terminé (flush final effectué)

	// N°164 — boot résilient. pgActive distingue le mode PostgreSQL
	// DÉCIDÉ (Save ne passe plus par JSON) du pool réellement ouvert
	// (pg != nil seulement après un Load réussi — boot normal ou
	// récupération). Les drapeaux dégradés sont atomiques : lus par la
	// carte Santé sans verrou, posés par la boucle de récupération.
	pgActive        atomic.Bool
	pgDegraded      atomic.Bool
	pgDegradedSince atomic.Int64           // unix secondes
	pgRecoveredAt   atomic.Int64           // unix secondes (0 = jamais)
	pgRecoverTries  atomic.Int64           // tentatives de récupération
	pgRecoverErr    atomic.Pointer[string] // dernière erreur de récupération
	syncRunning     bool                   // sous saveMu : le syncreur est-il démarré ?
}

// New charge l'état persisté (PostgreSQL si DATABASE_URL est défini, sinon
// data/db.json) ; toute base vide ou illisible (PostgreSQL comme JSON)
// démarre sur l'état de mise en service (BuildEmptyState — zéro démo).
// Le seed de démonstration a été SUPPRIMÉ du code : aucune donnée de test
// ne peut plus être générée, en production comme en développement.
func New(dir string) (*Store, error) {
	s := &Store{}

	// Bascule en mode PostgreSQL uniquement pour une URL postgres:// (ou
	// postgresql://) ; toute autre valeur (ex. sqlite locale) est ignorée
	// au profit du mode JSON.
	databaseURL := os.Getenv("DATABASE_URL")
	if strings.HasPrefix(databaseURL, "postgres://") || strings.HasPrefix(databaseURL, "postgresql://") {
		pg, err := OpenPG(databaseURL)
		if err != nil {
			// N°164 — boot résilient : base injoignable ≠ service
			// mort. Démarrage dégradé (mémoire seule) + récupération
			// en arrière-plan (fusion sans perte au retour).
			s.bootDegraded(databaseURL, err)
			return s, nil
		}
		// N°130 — canaux du syncreur de fond, posés AVANT tout Save() :
		// les marquages « sale » du boot (migrations, override admin)
		// restent en attente dans le canal jusqu'à son démarrage.
		s.saveCh = make(chan struct{}, 1)
		s.closing = make(chan struct{})
		s.syncDone = make(chan struct{})
		// Phase C « Speed App UX » — keep-alive Neon intelligent (cf. pg.go) :
		// supprime le cold start (~0,5-1 s) payé par la première mutation
		// pendant les périodes calmes, SANS ping superflu quand la base reçoit
		// déjà du trafic réel (agents en ligne). Défaut « business »
		// (05:00–24:00 UTC) ; NEON_KEEPALIVE=off pour désactiver.
		kaMode := os.Getenv("NEON_KEEPALIVE")
		if kaMode == "" {
			kaMode = "business"
		}
		if kaMode != "off" {
			pg.StartKeepAlive(kaMode)
		}
		s.pg = pg
		db, found, err := pg.Load()
		if err != nil {
			pg.Close()
			// N°164 — base joignable mais illisible : même traitement
			// (la boucle de récupération rejoue OpenPG + Load).
			s.bootDegraded(databaseURL, err)
			return s, nil
		}
		if found {
			log.Printf("store: état chargé depuis PostgreSQL (%d utilisateurs hotspot, %d routeurs, %d comptes)",
				len(db.HotspotUsers), len(db.Routers), len(db.Accounts))
			s.db = db
		} else {
			log.Println("store: base PostgreSQL vide — état de mise en service (aucune donnée démo)")
			s.db = BuildEmptyState()
		}
		// Migration mono-tenant → multi-tenant (avant l'override admin),
		// détachement plateforme, puis persistance immédiate si l'état a changé.
		mtChanged := migrateMultiTenant(s.db)
		if migrateDetachPlatform(s.db) {
			mtChanged = true
		}
		if migrateRemoveOperator(s.db) {
			mtChanged = true
		}
		if migrateUsageScopedPlans(s.db) {
			mtChanged = true
		}
		if migrateActiveTrialCap(s.db) {
			mtChanged = true
		}
		if mtChanged {
			s.Lock()
			s.Save()
			s.Unlock()
		}
		// L'override admin (variables d'environnement) s'applique à chaque
		// démarrage et doit être persisté aussitôt. La synchro est
		// FORCÉE à chaque boot : elle purge aussi les lignes orphelines
		// (settings d'un compte supprimé…) via la synchro différentielle.
		if err := bootstrapAdmin(s.db); err != nil {
			pg.Close()
			return nil, err
		}
		applyAdminOverride(s.db)
		s.Lock()
		s.Save()
		s.Unlock()
		// N°130 — le syncreur de fond prend le relais : les marquages posés
		// ci-dessus (migrations + override admin) sont flushés peu après le
		// démarrage, puis à chaque mutation au rythme plafonné saveMinInterval.
		s.pgActive.Store(true)
		s.saveMu.Lock()
		s.syncRunning = true
		s.saveMu.Unlock()
		go s.syncLoop()
		log.Println("store: persistance PostgreSQL active (DATABASE_URL, sauvegarde asynchrone)")
		return s, nil
	}

	// Mode développement : fichier JSON local.
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	s.path = filepath.Join(dir, "db.json")
	if data, err := os.ReadFile(s.path); err == nil && len(data) > 0 {
		// Sécurité P0 #6 — un db.json créé avant le correctif peut être en 0644.
		_ = os.Chmod(s.path, 0o600)
		var db model.DB
		if err := json.Unmarshal(data, &db); err != nil {
			log.Printf("store: db.json illisible (%v) — état de mise en service (aucune donnée démo)", err)
			s.db = BuildEmptyState()
		} else {
			unsealSecrets(&db)
			s.db = &db
			s.ensureSlices()
			// Un ancien db.json mono-tenant (champs legacy tenant/settings, sans
			// accounts) est migré ici vers le modèle multi-comptes puis re-sauvegardé.
			mtChanged := migrateMultiTenant(s.db)
			if migrateDetachPlatform(s.db) {
				mtChanged = true
			}
			if migrateRemoveOperator(s.db) {
				mtChanged = true
			}
			if migrateUsageScopedPlans(s.db) {
				mtChanged = true
			}
			if migrateActiveTrialCap(s.db) {
				mtChanged = true
			}
			if mtChanged {
				s.Lock()
				s.Save()
				s.Unlock()
			}
			// Override admin (variables d'environnement) — même garantie
			// qu'en mode PostgreSQL : appliqué à chaque démarrage.
			if err := bootstrapAdmin(s.db); err != nil {
				return nil, err
			}
			if applyAdminOverride(s.db) {
				s.Lock()
				s.Save()
				s.Unlock()
			}
			return s, nil
		}
	} else {
		log.Println("store: db.json absent — état de mise en service (aucune donnée démo)")
		s.db = BuildEmptyState()
	}
	// État initial : sécurité P0 (bootstrapAdmin — plus AUCUN identifiant
	// par défaut connu du repo), puis l'override admin crée/remplace le
	// compte depuis ADMIN_PASSWORD si elle est définie.
	if err := bootstrapAdmin(s.db); err != nil {
		return nil, err
	}
	applyAdminOverride(s.db)
	s.Lock()
	s.Save()
	s.Unlock()
	log.Printf("store: persistance JSON active (%s)", s.path)
	return s, nil
}

// migrateMultiTenant — fait passer un état mono-tenant (ancien db.json ou base
// PostgreSQL d'avant la migration) au modèle multi-comptes :
//   - crée le compte principal {AccountMainID} s'il existe des utilisateurs
//     clients à y rattacher (l'admin plateforme n'a PAS de compte propre) ;
//   - backfill AccountID == "" → AccountMainID sur les entités métier ;
//   - initialise SettingsByAccount (réglages legacy migrés, sinon défauts FCFA) ;
//   - vide les champs legacy db.Tenant / db.Settings.
//
// Idempotent : retourne true uniquement si l'état a été modifié (à persister
// par l'appelant). Le seed démo est déjà multi-tenant : cette fonction n'y
// change rien.
func migrateMultiTenant(db *model.DB) bool {
	changed := false

	if len(db.Accounts) == 0 {
		// Ne recrée le compte principal QUE si des utilisateurs clients
		// (ou des réglages legacy) ont besoin d'un foyer — l'admin
		// plateforme étant un opérateur sans compte client.
		hasClientUser := false
		for i := range db.Users {
			if !isPlatformRole(db.Users[i].Role) {
				hasClientUser = true
				break
			}
		}
		if hasClientUser || db.Tenant.Name != "" {
			name := db.Tenant.Name
			if name == "" {
				// Mode PostgreSQL : loadSettings remplit directement SettingsByAccount.
				if s, ok := db.SettingsByAccount[model.AccountMainID]; ok && s.Tenant.Name != "" {
					name = s.Tenant.Name
				}
			}
			if name == "" {
				name = "MikCloud"
			}
			db.Accounts = append(db.Accounts, model.Account{
				ID:        model.AccountMainID,
				Name:      name,
				Status:    "active",
				CreatedAt: model.NowISO(),
				// N°98 — le compte principal hérite des données
				// de l'ère mono-tenant : du HOTSPOT par définition.
				Usage: model.AccountUsageHotspot,
			})
			changed = true
		}
	}

	// N°98 — usage des comptes : les comptes créés avant la colonne
	// (bases JSON de dev, états injectés par des tests) n'ont pas
	// d'usage → « hotspot » (produit historique — le comportement
	// d'avant la colonne, strictement). La synchro différentielle
	// persiste la valeur au premier Save qui suit.
	for i := range db.Accounts {
		if db.Accounts[i].Usage == "" {
			db.Accounts[i].Usage = model.AccountUsageHotspot
			changed = true
		}
	}

	backfill := func(n int, set func(i int)) {
		for i := 0; i < n; i++ {
			set(i)
		}
	}
	backfill(len(db.Users), func(i int) {
		// L'admin plateforme n'est PAS rattaché au compte principal :
		// c'est un opérateur du SaaS, sans compte client propre.
		if db.Users[i].AccountID == "" && !isPlatformRole(db.Users[i].Role) {
			db.Users[i].AccountID = model.AccountMainID
			changed = true
		}
	})
	backfill(len(db.Routers), func(i int) {
		if db.Routers[i].AccountID == "" {
			db.Routers[i].AccountID = model.AccountMainID
			changed = true
		}
	})
	backfill(len(db.Profiles), func(i int) {
		if db.Profiles[i].AccountID == "" {
			db.Profiles[i].AccountID = model.AccountMainID
			changed = true
		}
	})
	backfill(len(db.HotspotUsers), func(i int) {
		if db.HotspotUsers[i].AccountID == "" {
			db.HotspotUsers[i].AccountID = model.AccountMainID
			changed = true
		}
	})
	backfill(len(db.Batches), func(i int) {
		if db.Batches[i].AccountID == "" {
			db.Batches[i].AccountID = model.AccountMainID
			changed = true
		}
	})
	backfill(len(db.Resellers), func(i int) {
		if db.Resellers[i].AccountID == "" {
			db.Resellers[i].AccountID = model.AccountMainID
			changed = true
		}
	})
	backfill(len(db.Transactions), func(i int) {
		if db.Transactions[i].AccountID == "" {
			db.Transactions[i].AccountID = model.AccountMainID
			changed = true
		}
	})
	backfill(len(db.Sessions), func(i int) {
		if db.Sessions[i].AccountID == "" {
			db.Sessions[i].AccountID = model.AccountMainID
			changed = true
		}
	})
	// NB : les lignes de JOURNAL (activity) ne sont PAS backfillées — les
	// événements plateforme (création/suppression de compte, équipe,
	// sessions support) sont transverses (AccountID vide).
	backfill(len(db.Sales), func(i int) {
		if db.Sales[i].AccountID == "" {
			db.Sales[i].AccountID = model.AccountMainID
			changed = true
		}
	})
	backfill(len(db.Commands), func(i int) {
		if db.Commands[i].AccountID == "" {
			db.Commands[i].AccountID = model.AccountMainID
			changed = true
		}
	})
	backfill(len(db.Templates), func(i int) {
		if db.Templates[i].AccountID == "" {
			db.Templates[i].AccountID = model.AccountMainID
			changed = true
		}
	})
	backfill(len(db.UserLogs), func(i int) {
		if db.UserLogs[i].AccountID == "" {
			db.UserLogs[i].AccountID = model.AccountMainID
			changed = true
		}
	})
	backfill(len(db.IPBindings), func(i int) {
		if db.IPBindings[i].AccountID == "" {
			db.IPBindings[i].AccountID = model.AccountMainID
			changed = true
		}
	})
	backfill(len(db.SchedulerTasks), func(i int) {
		if db.SchedulerTasks[i].AccountID == "" {
			db.SchedulerTasks[i].AccountID = model.AccountMainID
			changed = true
		}
	})
	backfill(len(db.Traffic), func(i int) {
		if db.Traffic[i].AccountID == "" {
			db.Traffic[i].AccountID = model.AccountMainID
			changed = true
		}
	})

	// P0 (audit Mikhmon) — ventes antérieures à la marge (F13) : les champs
	// Cost/SellingTotal n'existaient pas ; Amount (= price×count) servait de
	// base → cost = amount, selling = amount (sellingPrice n'existait pas).
	for i := range db.Sales {
		s := &db.Sales[i]
		if s.Cost == 0 && s.SellingTotal == 0 && s.Amount > 0 {
			s.Cost = s.Amount
			s.SellingTotal = s.Amount
			changed = true
		}
	}

	// P0 (audit Mikhmon) — comptes créés avant la vague P0 : chaque compte sans
	// AUCUN modèle de voucher reçoit les 3 gabarits par défaut (contrat F2).
	// Idempotent : la suppression du dernier modèle étant interdite par l'API,
	// ce rattrapage ne se joue qu'une fois par compte pré-P0.
	if len(db.Templates) > 0 || len(db.Accounts) > 0 {
		accHasTemplates := map[string]bool{}
		for i := range db.Templates {
			accHasTemplates[db.Templates[i].AccountID] = true
		}
		for i := range db.Accounts {
			accID := db.Accounts[i].ID
			if accHasTemplates[accID] {
				continue
			}
			db.Templates = append(db.Templates, SeedTemplatesFor(accID)...)
			changed = true
		}
	}

	if db.SettingsByAccount == nil {
		db.SettingsByAccount = map[string]model.Settings{}
	}
	if _, ok := db.SettingsByAccount[model.AccountMainID]; !ok {
		hasMainAccount := false
		for i := range db.Accounts {
			if db.Accounts[i].ID == model.AccountMainID {
				hasMainAccount = true
				break
			}
		}
		if hasMainAccount {
			if db.Settings != (model.Settings{}) {
				db.SettingsByAccount[model.AccountMainID] = db.Settings // réglages legacy migrés
			} else {
				db.SettingsByAccount[model.AccountMainID] = model.Settings{
					Tenant: model.Tenant{Name: "MikCloud", Currency: "XOF", Timezone: "Africa/Abidjan"},
					Plan:   model.Plan{Name: "PRO", MaxRouters: "Illimité", MaxUsers: "Illimité"},
				}
			}
			changed = true
		}
	}

	// P0 (audit Mikhmon) — défauts des politiques d'expiration (F5) sur les
	// réglages existants (données créées avant la vague P0 : mode vide).
	for accID, s := range db.SettingsByAccount {
		if normalizeSettingsDefaults(&s) {
			db.SettingsByAccount[accID] = s
			changed = true
		}
	}

	// Les champs legacy sont vidés : le modèle multi-comptes devient la seule
	// source de vérité (ils seront omis du prochain JSON non nuls → à zéro).
	if db.Tenant != (model.Tenant{}) || db.Settings != (model.Settings{}) {
		db.Tenant = model.Tenant{}
		db.Settings = model.Settings{}
		changed = true
	}
	return changed
}

// isPlatformRole — true pour les rôles d'opérateur plateforme (sans compte
// client propre) : « platform_admin » et « admin » historique.
func isPlatformRole(role string) bool {
	return role == model.RolePlatformAdmin || role == "admin"
}

// migrateDetachPlatform — l'admin plateforme est le PROPRIÉTAIRE du SaaS, pas
// un client : il n'a plus de compte ni d'abonnement propres.
//   - détache les administrateurs plateforme de tout compte client ;
//   - supprime le compte principal {AccountMainID} (et ses réglages, gabarits
//     et notifications) dès qu'il ne porte plus AUCUN utilisateur client.
//
// Idempotent, appliqué à chaque démarrage et reload (auto-réparant).
func migrateDetachPlatform(db *model.DB) bool {
	changed := false
	for i := range db.Users {
		if isPlatformRole(db.Users[i].Role) && db.Users[i].AccountID != "" {
			db.Users[i].AccountID = ""
			changed = true
		}
	}
	for i := range db.Accounts {
		if db.Accounts[i].ID != model.AccountMainID {
			continue
		}
		hasClientUser := false
		for j := range db.Users {
			if db.Users[j].AccountID == model.AccountMainID && !isPlatformRole(db.Users[j].Role) {
				hasClientUser = true
				break
			}
		}
		if hasClientUser {
			break
		}
		// Suppression du compte principal : compte + réglages + gabarits
		// + notifications. Le journal (activity) est conservé — audit.
		db.Accounts = append(db.Accounts[:i], db.Accounts[i+1:]...)
		delete(db.SettingsByAccount, model.AccountMainID)
		delete(db.NotifSettings, model.AccountMainID)
		kept := db.Templates[:0]
		for _, t := range db.Templates {
			if t.AccountID != model.AccountMainID {
				kept = append(kept, t)
			}
		}
		db.Templates = kept
		changed = true
		break
	}
	return changed
}

// migrateRemoveOperator — le rôle « operator » a été retiré du produit : les
// comptes concernés sont promus « manager » (accès strictement supérieur —
// aucune perte de droits, l'operator n'avait que la lecture/vente).
// Retourne true si au moins un utilisateur a changé (→ re-sauvegarde).
func migrateRemoveOperator(db *model.DB) bool {
	changed := false
	for i := range db.Users {
		if db.Users[i].Role == "operator" {
			db.Users[i].Role = model.RoleManager
			changed = true
		}
	}
	return changed
}

// migrateUsageScopedPlans — N°122 (tarifs segmentés Hotspot/HomeNet) :
// réécrit les identifiants de formule HISTORIQUES vers leur équivalent du
// mode du compte, partout où ils sont stockés :
//   - SettingsByAccount[].Subscription.PlanID : « essentiel » → mensuel du
//     mode, « illimite » → annuel du mode (les PÉRIODES et LastAmountFcfa
//     sont conservés tels quels — le renouvelement appliquera le nouveau
//     tarif, conforme à la décision prix) ; le libellé compat Settings.Plan
//     est rafraîchi ;
//   - BillingRequests[].PlanID (demandes en attente et historique) : même
//     résolution, pour que l'activation applique la formule du bon mode.
//     Le MONTANT demandé reste celui calculé à la demande (l'activation
//     plateforme reste une décision de l'opérateur après encaissement).
//
// Les prélèvements carte GeniusPaySubs gardent leur identifiant d'origine :
// ResolvePlan les résout à l'usage à chaque facture — leur MONTANT souscrit
// chez GeniusPay reste le tarif de création (résilier/re-créer pour aligner).
// Idempotent : les identifiants segmentés ne re-matchent jamais les anciens.
func migrateUsageScopedPlans(db *model.DB) bool {
	usageOf := func(accID string) string {
		for i := range db.Accounts {
			if db.Accounts[i].ID == accID {
				if db.Accounts[i].Usage == model.AccountUsageHomeNet {
					return model.AccountUsageHomeNet
				}
				return model.AccountUsageHotspot
			}
		}
		return model.AccountUsageHotspot
	}
	resolve := func(accID, legacy string) string {
		if p, ok := model.ResolvePlan(legacy, usageOf(accID)); ok {
			return p.ID
		}
		return legacy
	}
	changed := false
	for accID, s := range db.SettingsByAccount {
		if s.Subscription.PlanID == "essentiel" || s.Subscription.PlanID == "illimite" {
			s.Subscription.PlanID = resolve(accID, s.Subscription.PlanID)
			if p, ok := model.PlanByID(s.Subscription.PlanID); ok {
				s.Plan = model.Plan{
					Name:       "MikCloud " + p.Name,
					MaxRouters: "Illimité",
					MaxUsers:   "Illimité",
				}
				if p.PerRouter {
					s.Plan.MaxRouters = "Par routeur"
				}
			}
			db.SettingsByAccount[accID] = s
			changed = true
		}
	}
	for i := range db.BillingRequests {
		br := &db.BillingRequests[i]
		if br.PlanID == "essentiel" || br.PlanID == "illimite" {
			br.PlanID = resolve(br.AccountID, br.PlanID)
			changed = true
		}
	}
	return changed
}

// migrateActiveTrialCap — N°123 (essai Hotspot réduit à 60 jours) : met à
// jour les clients ACTIFS en essai vers la durée segmentée courante —
// 60 jours en Hotspot, 30 jours en HomeNet — calculée depuis le DÉBUT de
// la période posée en base (PeriodStart) :
//   - un essai de 90 jours encore en cours passe à 60 jours à compter de
//     son début : le reliquat au-delà de la nouvelle durée est retiré ;
//   - un essai entamé depuis plus que la durée cible voit sa fin ramenée
//     dans le passé : le compte passe « expired » (lecture seule), puis
//     « suspended » après la durée de grâce (30 j) — la réduction s'applique
//     aussi aux essais déjà largement consommés, c'est la décision produit ;
//   - un essai plus court que la durée cible (prolongation manuelle
//     antérieure plus courte, essai Maison de 30 j conforme) reste INTACT :
//     la migration ne fait que raccourcir, jamais allonger.
//
// Périmètre : comptes de statut « active » dont l'abonnement courant est
// « essai ». Les abonnements PAYÉS (segmentés ou historiques réécrits par
// migrateUsageScopedPlans) et les essais des comptes désactivés ne sont
// pas touchés. Une PeriodEnd vide (non expirante) est ignorée — une
// migration ne ferme jamais un accès illimité posé à la main.
// Idempotent : une fin déjà ≤ à PeriodStart + durée cible ne re-matche pas.
func migrateActiveTrialCap(db *model.DB) bool {
	statusOf := func(accID string) (string, string) {
		for i := range db.Accounts {
			if db.Accounts[i].ID == accID {
				if db.Accounts[i].Usage == model.AccountUsageHomeNet {
					return db.Accounts[i].Status, model.AccountUsageHomeNet
				}
				return db.Accounts[i].Status, model.AccountUsageHotspot
			}
		}
		return "", model.AccountUsageHotspot
	}
	changed := false
	for accID, s := range db.SettingsByAccount {
		if s.Subscription.PlanID != "essai" {
			continue
		}
		accStatus, usage := statusOf(accID)
		if accStatus != "active" {
			continue
		}
		if s.Subscription.PeriodEnd == "" || s.Subscription.PeriodStart == "" {
			continue
		}
		start, errS := time.Parse(time.RFC3339, s.Subscription.PeriodStart)
		end, errE := time.Parse(time.RFC3339, s.Subscription.PeriodEnd)
		if errS != nil || errE != nil {
			continue
		}
		trialCap := start.AddDate(0, 0, 60)
		if usage == model.AccountUsageHomeNet {
			trialCap = start.AddDate(0, 0, 30)
		}
		if end.After(trialCap) {
			s.Subscription.PeriodEnd = trialCap.Format(time.RFC3339)
			db.SettingsByAccount[accID] = s
			changed = true
		}
	}
	return changed
}

// bootstrapAdmin — sécurité P0 : il n'existe PLUS d'identifiants par défaut
// connus du code (l'ancien admin/admin123 documenté publiquement est
// supprimé — BuildEmptyState ne crée plus aucun utilisateur). Sur un état
// sans administrateur plateforme :
//   - ADMIN_PASSWORD définie → rien à faire ici, applyAdminOverride (appelé
//     juste après) crée le compte depuis l'environnement ;
//   - production (PostgreSQL) SANS ADMIN_PASSWORD → REFUS de démarrer : un
//     SaaS commercial ne doit jamais exposer une console sans opérateur
//     identifié, encore moins avec un mot de passe public ;
//   - développement local → administrateur « admin » créé avec un mot de
//     passe ALÉATOIRE (crypto/rand), affiché une seule fois dans les logs.
func bootstrapAdmin(db *model.DB) error {
	for i := range db.Users {
		if db.Users[i].Role == "admin" {
			return nil // un opérateur plateforme existe déjà
		}
	}
	if os.Getenv("ADMIN_PASSWORD") != "" {
		return nil // applyAdminOverride (appelé juste après) crée le compte
	}
	if u := os.Getenv("DATABASE_URL"); strings.HasPrefix(u, "postgres://") || strings.HasPrefix(u, "postgresql://") {
		return errors.New("sécurité : ADMIN_PASSWORD est obligatoire en production (base PostgreSQL) — définissez-la puis redéployez")
	}
	// Développement local : mot de passe aléatoire (jamais un mot de passe connu).
	buf := make([]byte, 15)
	if _, err := crand.Read(buf); err != nil {
		return fmt.Errorf("génération du mot de passe administrateur impossible : %w", err)
	}
	password := base64.RawURLEncoding.EncodeToString(buf)
	salt := auth.NewSalt()
	db.Users = append(db.Users, model.AdminUser{
		ID:           model.NewID("adm-"),
		AccountID:    "", // opérateur plateforme sans compte client
		Name:         "Administrateur",
		Username:     "admin",
		Role:         "admin",
		Salt:         salt,
		PasswordHash: auth.HashPassword(password, salt),
		CreatedAt:    model.NowISO(),
	})
	log.Printf("sécurité : aucun identifiant par défaut — administrateur local « admin » créé, mot de passe : %s", password)
	log.Println("sécurité : changez ce mot de passe (console) et définissez ADMIN_PASSWORD en production")
	return nil
}

// applyAdminOverride — si ADMIN_PASSWORD est défini, remplace le compte démo
// (admin/admin123) par des identifiants de production : le compte démo encore
// intact est supprimé, le compte ADMIN_USERNAME est créé ou mis à jour.
// Variables : ADMIN_USERNAME (défaut « admin »), ADMIN_PASSWORD, ADMIN_NAME.
// Retourne true si l'état a été modifié (à persister par l'appelant).
//
// MOT DE PASSE CHANGÉ PAR L'UTILISATEUR : si l'admin a modifié son mot de
// passe depuis la console (POST /api/auth/password, PasswordSetByUser=true)
// et que la variable ADMIN_PASSWORD n'a pas changé depuis la dernière
// application (EnvPasswordHash), le mot de passe utilisateur est PRÉSERVÉ —
// l'override ne s'applique que si l'opérateur modifie la variable (chemin de
// récupération documenté : Render → Environment → ADMIN_PASSWORD).
func applyAdminOverride(db *model.DB) bool {
	password := os.Getenv("ADMIN_PASSWORD")
	if password == "" {
		return false
	}
	username := os.Getenv("ADMIN_USERNAME")
	if username == "" {
		username = "admin"
	}
	name := os.Getenv("ADMIN_NAME")
	if name == "" {
		name = "Administrateur MikCloud"
	}

	// 1. Retire tout compte démo encore intact (username « admin » + mot de passe « admin123 »).
	kept := db.Users[:0]
	removedDemo := false
	for _, u := range db.Users {
		if u.Username == "admin" && auth.CheckPassword("admin123", u.Salt, u.PasswordHash) {
			removedDemo = true
			continue
		}
		kept = append(kept, u)
	}
	db.Users = kept

	envHash := auth.HashPassword(password, "")

	// 2. Crée ou met à jour le compte administrateur déclaré par l'environnement.
	// L'admin plateforme est un OPÉRATEUR du SaaS : sans compte client propre
	// (les consoles clients s'ouvrent par session support).
	salt := auth.NewSalt()
	for i := range db.Users {
		if db.Users[i].Username == username {
			u := &db.Users[i]
			u.Name = name
			u.Role = "admin"
			u.AccountID = ""
			// Mot de passe conservé si l'utilisateur l'a changé lui-même
			// ET que la variable d'environnement n'a pas changé d'intention.
			userPreserved := u.PasswordSetByUser && u.EnvPasswordHash != "" &&
				auth.CheckPassword(password, "", u.EnvPasswordHash)
			if !userPreserved {
				u.Salt = salt
				u.PasswordHash = auth.HashPassword(password, salt)
				u.PasswordSetByUser = false
			}
			u.EnvPasswordHash = envHash
			if userPreserved {
				log.Printf("store: mot de passe de « %s » conservé (modifié par l'utilisateur, ADMIN_PASSWORD inchangée)", username)
			} else {
				log.Printf("store: compte admin « %s » mis à jour depuis l'environnement", username)
			}
			if removedDemo {
				log.Println("store: compte démo admin/admin123 supprimé")
			}
			return true
		}
	}
	db.Users = append(db.Users, model.AdminUser{
		ID:              model.NewID("adm-"),
		AccountID:       "", // opérateur plateforme sans compte client
		Name:            name,
		Username:        username,
		Role:            "admin",
		PasswordHash:    auth.HashPassword(password, salt),
		Salt:            salt,
		CreatedAt:       model.NowISO(),
		EnvPasswordHash: envHash,
	})
	log.Printf("store: compte admin « %s » créé depuis l'environnement", username)
	if removedDemo {
		log.Println("store: compte démo admin/admin123 supprimé")
	}
	return true
}

// normalizeSettingsDefaults — défauts P0 (audit Mikhmon) des politiques
// d'expiration d'un compte : un mode vide (données antérieures) devient
// "keep" ; les jours restent à 30 par défaut dans ce cas uniquement (un mode
// explicitement configuré — même avec 0 jour — est respecté tel quel).
func normalizeSettingsDefaults(s *model.Settings) bool {
	if s.Tenant.ExpiryPolicyMode != "" {
		return false
	}
	s.Tenant.ExpiryPolicyMode = "keep"
	if s.Tenant.ExpiryPolicyAfterDays == 0 {
		s.Tenant.ExpiryPolicyAfterDays = 30
	}
	return true
}

func (s *Store) ensureSlices() {
	if s.db.Accounts == nil {
		s.db.Accounts = []model.Account{}
	}
	if s.db.SettingsByAccount == nil {
		s.db.SettingsByAccount = map[string]model.Settings{}
	}
	if s.db.Users == nil {
		s.db.Users = []model.AdminUser{}
	}
	if s.db.Routers == nil {
		s.db.Routers = []model.Router{}
	}
	if s.db.Profiles == nil {
		s.db.Profiles = []model.Profile{}
	}
	if s.db.HotspotUsers == nil {
		s.db.HotspotUsers = []model.HotspotUser{}
	}
	if s.db.Resellers == nil {
		s.db.Resellers = []model.Reseller{}
	}
	if s.db.Transactions == nil {
		s.db.Transactions = []model.Transaction{}
	}
	if s.db.Sessions == nil {
		s.db.Sessions = []model.Session{}
	}
	if s.db.Activity == nil {
		s.db.Activity = []model.Activity{}
	}
	if s.db.Sales == nil {
		s.db.Sales = []model.Sale{}
	}
	if s.db.Templates == nil {
		s.db.Templates = []model.VoucherTemplate{}
	}
	if s.db.UserLogs == nil {
		s.db.UserLogs = []model.UserLog{}
	}
	if s.db.IPBindings == nil {
		s.db.IPBindings = []model.IPBinding{}
	}
	if s.db.SchedulerTasks == nil {
		s.db.SchedulerTasks = []model.SchedulerTask{}
	}
	if s.db.Traffic == nil {
		s.db.Traffic = []model.RouterTraffic{}
	}
	if s.db.NotifSettings == nil {
		s.db.NotifSettings = map[string]model.NotificationSettings{}
	}
	if s.db.NotifLog == nil {
		s.db.NotifLog = []model.NotificationLog{}
	}
	// N°127 — assistant conversationnel public de la vitrine.
	if s.db.ChatConversations == nil {
		s.db.ChatConversations = []model.ChatConversation{}
	}
	if s.db.ChatMessages == nil {
		s.db.ChatMessages = []model.ChatMessage{}
	}
}

// GetOrCreateNotifSettings — réglages de notification d'un compte, créés avec
// les défauts si absents (à appeler sous verrou ; persistés par le Save de
// l'appelant). Retourne une COPIE : modifier puis réécrire avec SetNotifSettings.
func GetOrCreateNotifSettings(db *model.DB, acc string) model.NotificationSettings {
	if db.NotifSettings != nil {
		if s, ok := db.NotifSettings[acc]; ok {
			s.Normalize()
			return s
		}
	}
	s := model.NotificationSettings{
		AccountID:         acc,
		OfflineAfterSec:   135,
		LowStockThreshold: 25,
		ReportHour:        20,
	}
	if db.NotifSettings == nil {
		db.NotifSettings = map[string]model.NotificationSettings{}
	}
	db.NotifSettings[acc] = s
	return s
}

// SetNotifSettings réécrit les réglages de notification d'un compte (sous verrou).
func SetNotifSettings(db *model.DB, s model.NotificationSettings) {
	s.Normalize()
	if db.NotifSettings == nil {
		db.NotifSettings = map[string]model.NotificationSettings{}
	}
	db.NotifSettings[s.AccountID] = s
}

// Lock / Unlock — verrou global du store (à tenir lors de chaque accès à Data()).
func (s *Store) Lock()   { s.mu.Lock() }
func (s *Store) Unlock() { s.mu.Unlock() }

// Data retourne la base courante (à n'utiliser que sous verrou).
func (s *Store) Data() *model.DB { return s.db }

// ---------------------------------------------------------------------------
// Sécurité P0 #6 — chiffrement au repos des identifiants routeur (mode JSON)
// ---------------------------------------------------------------------------

// sealedSnapshot renvoie une COPIE superficielle de l'état dont les secrets
// sont chiffrés, pour la sérialisation JSON. La mémoire vivante reste en clair
// (aucun handler à modifier) ; la copie est jetable et ne sert qu'au marshal.
//
// N°75 — en plus des mots de passe routeur, les secrets de notification
// (tokens bots, clé Resend, mot de passe SMTP) sont scellés. Le secret 2FA
// (AdminUser.TOTPSecret) porte json:"-" : il n'est PAS sérialisable en mode
// JSON (limitation assumée du mode dev — la production PostgreSQL le
// chiffre et le persiste, empreinte dédiée incluse).
func sealedSnapshot(db *model.DB) *model.DB {
	clone := *db
	clone.Routers = make([]model.Router, len(db.Routers))
	for i, r := range db.Routers {
		r.Password = secretbox.Encrypt(r.Password)
		clone.Routers[i] = r
	}
	if db.NotifSettings != nil {
		clone.NotifSettings = make(map[string]model.NotificationSettings, len(db.NotifSettings))
		for k, v := range db.NotifSettings {
			v.TelegramBotToken = secretbox.Encrypt(v.TelegramBotToken)
			v.WhatsAppToken = secretbox.Encrypt(v.WhatsAppToken)
			v.ResendAPIKey = secretbox.Encrypt(v.ResendAPIKey)
			v.SMTPPass = secretbox.Encrypt(v.SMTPPass)
			clone.NotifSettings[k] = v
		}
	}
	return &clone
}

// unsealSecrets déchiffre en place les secrets d'un état issu du JSON
// (valeurs antérieures au correctif : passthrough transparent).
func unsealSecrets(db *model.DB) {
	for i := range db.Routers {
		db.Routers[i].Password = secretbox.Decrypt(db.Routers[i].Password)
	}
	for k, v := range db.NotifSettings {
		v.TelegramBotToken = secretbox.Decrypt(v.TelegramBotToken)
		v.WhatsAppToken = secretbox.Decrypt(v.WhatsAppToken)
		v.ResendAPIKey = secretbox.Decrypt(v.ResendAPIKey)
		v.SMTPPass = secretbox.Decrypt(v.SMTPPass)
		db.NotifSettings[k] = v
	}
}

// Save persiste la base.
//
// Mode PostgreSQL (N°130) : ASYNCHRONE — simple marquage « sale » + réveil
// du syncreur de fond. La requête rend la main immédiatement ; la synchro
// différentielle (re-hash des tables + transaction Neon) s'exécute HORS du
// verrou global, sur une photographie CloneDeep. Le micro-verrou saveMu
// (indépendant du verrou global) rend le marquage sûr même pour un appelant
// qui ne tiendrait pas le verrou.
//
// N°133 — Save() marque TOUTES les tables sales (diff complet). Les
// chemins de LECTURE pollés qui connaissent précisément ce qu'ils ont
// touché utilisent SaveTables (tables.go) ; tout autre appelant garde ce
// comportement conservateur — une mutation non ciblée reste toujours
// persistée au cycle suivant.
//
// Mode JSON (développement, E2E) : écriture atomique synchrone inchangée.
func (s *Store) Save() {
	// N°164 — le mode PostgreSQL est décidé dès le boot (pgActive), même
	// dégradé (pool pas encore ouvert) : les marquages s'accumulent et le
	// premier flush de la récupération les emportera (diff complet).
	if s.pgActive.Load() {
		s.saveMu.Lock()
		s.dirtyAll = true
		s.saveMu.Unlock()
		select {
		case s.saveCh <- struct{}{}:
		default: // un réveil est déjà en attente — le drapeau suffit
		}
		return
	}
	s.saveJSON()
}

// SaveTables — N°133 — variante CIBLÉE de Save pour les chemins de LECTURE
// pollés (dashboard, listes, check-in agent) : seules les tables listées
// seront re-hashées puis différentiellement synchronisées au prochain flush
// ; les autres conservent leurs empreintes (aucun re-hash, aucune écriture).
// Les noms viennent des constantes Table* (tables.go) ; les marqueurs sont
// produits par le moteur lui-même (Tick/applyExpiry/enforcement).
//
// SÉCURITÉ DU CONTRAT : ne doit être utilisé QUE par des chemins dont
// TOUTES les mutations passent par le moteur marqué (Tick, enforceExpired,
// touchAgent, file de commandes). Le moindre doute → Save() complet.
// Un marquage incomplet retarderait la persistance d'une table jusqu'à la
// prochaine sauvegarde complète (jamais de perte : l'état mémoire reste la
// vérité, le boot suivant resynchronise tout).
//
// Mode JSON : pas de diff possible — retombe sur l'écriture complète
// synchrone (strictement identique à Save).
func (s *Store) SaveTables(names ...string) {
	// N°164 — même garde que Save : mode PG décidé (dégradé inclus).
	if !s.pgActive.Load() {
		s.saveJSON()
		return
	}
	marked := false
	s.saveMu.Lock()
	for _, n := range names {
		if n == "" || !syncKnownTables[n] {
			continue
		}
		if s.dirtyTables == nil {
			s.dirtyTables = make(map[string]bool, 8)
		}
		if !s.dirtyTables[n] {
			s.dirtyTables[n] = true
			marked = true
		}
	}
	s.saveMu.Unlock()
	if !marked {
		return // rien de nouveau : un flush est déjà en attente/en cours
	}
	select {
	case s.saveCh <- struct{}{}:
	default:
	}
}

// saveJSON — écriture atomique du fichier db.json (mode développement).
func (s *Store) saveJSON() {
	data, err := json.MarshalIndent(sealedSnapshot(s.db), "", "  ")
	if err != nil {
		log.Printf("store: sérialisation impossible : %v", err)
		return
	}
	// Sécurité P0 #6 — fichier de données restreint au propriétaire du process.
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		log.Printf("store: écriture impossible : %v", err)
		return
	}
	if err := os.Rename(tmp, s.path); err != nil {
		log.Printf("store: renommage impossible : %v", err)
	}
}

// Cadences du syncreur de fond (N°130).
const (
	// saveDebounce — fenêtre de coalescence : la rafale de marquages qui
	// suit un poll (télémétrie Tick → plusieurs handlers) se fondent en UNE
	// photographie.
	saveDebounce = 500 * time.Millisecond
	// saveMinInterval — cadence PLAFOND des synchronisations Neon : la
	// télémétrie Tick rend l'état sale à chaque lecture pollée ; sans
	// plafond, le syncreur enchaînerait les transactions au rythme des 37
	// sources de polling du front. 3 s borne la charge Neon (≤ 20
	// transactions/min) au prix d'une fenêtre de perte au crash ≤ ~3 s
	// (un arrêt propre SIGTERM flushe avant de rendre la main).
	saveMinInterval = 3 * time.Second
	// saveRetryBackoff — attente entre deux tentatives après un échec de
	// synchro (chaque tentative reste bornée par syncTimeout).
	saveRetryBackoff = 5 * time.Second
)

// syncLoop — goroutine de sauvegarde de fond (mode PostgreSQL uniquement,
// démarrée par New). Attend un marquage, coalesce la rafale (debounce),
// respecte la cadence plafond, puis flushe ; un échec est retenté avec
// backoff (les empreintes n'étant rafraîchies qu'après succès, le retry
// repart des VRAIES différences — cf. Sync).
func (s *Store) syncLoop() {
	defer close(s.syncDone)
	var lastSync time.Time
	for {
		select {
		case <-s.closing:
			s.flush()
			return
		case <-s.saveCh:
		}
		// Coalescence : absorbe les marquages arrivant en rafale.
		time.Sleep(saveDebounce)
		// Cadence plafond : une transaction Neon au plus toutes les
		// saveMinInterval (le drapeau « sale » garde la trace des
		// marquages arrivés entre-temps — le flush les emportera).
		if wait := saveMinInterval - time.Since(lastSync); wait > 0 {
			select {
			case <-s.closing:
				s.flush()
				return
			case <-time.After(wait):
			}
		}
		for !s.flush() {
			select {
			case <-s.closing:
				return // échec persistant pendant l'arrêt : rien de plus à tenter
			case <-time.After(saveRetryBackoff):
			}
		}
		lastSync = time.Now()
	}
}

// flush — UNE synchronisation : photographie sous le verrou global
// (CloneDeep — quelques millisecondes, aucune E/S) puis synchro
// PostgreSQL HORS verrou, bornée par syncTimeout. Les requêtes
// concurrentes continuent d'être servies pendant l'écriture.
//
// N°133 — deux régimes : diff COMPLET (marquage Save) ou diff CIBLÉ
// sur les tables marquées par SaveTables (re-hash des seules tables
// concernées — les empreintes des autres sont reportées telles quelles
// dans le cache, cf. PG.SyncTables). Retourne false sur échec :
// l'appelant retente avec backoff en BASCULANT sur le diff complet
// (conservateur — le retry repart de toutes les vraies différences).
func (s *Store) flush() bool {
	s.saveMu.Lock()
	full := s.dirtyAll
	var tables map[string]bool
	if !full {
		if len(s.dirtyTables) == 0 {
			s.saveMu.Unlock()
			return true // rien de sale (marquages absorbés par le flush précédent)
		}
		tables = s.dirtyTables
		s.dirtyTables = nil
	} else {
		// Diff complet : les marquages ciblés éventuellement en attente sont
		// couverts (et vidés) — sinon le flush suivant re-syncerait ces
		// tables pour rien.
		s.dirtyTables = nil
	}
	s.dirtyAll = false
	s.saveMu.Unlock()

	s.mu.Lock()
	snap := s.db.CloneDeep()
	s.mu.Unlock()

	var err error
	if full {
		err = s.pg.Sync(snap)
	} else {
		err = s.pg.SyncTables(snap, tables)
	}
	if err != nil {
		log.Printf("store: synchro PostgreSQL différée échouée (%v) — nouvelle tentative (diff complet)", err)
		s.saveMu.Lock()
		s.dirtyAll = true
		s.dirtyTables = nil
		s.saveMu.Unlock()
		return false
	}
	return true
}

// Close ferme proprement la persistance (appelé au SIGTERM/SIGINT Render).
// N°130 — le syncreur de fond reçoit l'ordre d'arrêt et exécute un FLUSH
// FINAL de l'état sale éventuel (borné par syncTimeout) avant la fermeture
// du pool : la fenêtre de perte asynchrone ne survit pas à un arrêt propre.
// N°164 — en mode dégradé (syncreur jamais démarré), l'attente de syncDone
// est sautée (elle ne serait jamais satisfaite) et la boucle de récupération
// est prévenue par closing. Idempotent (double Close sans panique).
func (s *Store) Close() error {
	if !s.pgActive.Load() {
		return nil // mode JSON : rien à fermer
	}
	s.saveMu.Lock()
	if s.closed {
		s.saveMu.Unlock()
		return nil
	}
	s.closed = true
	s.saveMu.Unlock()
	close(s.closing)
	s.saveMu.Lock()
	running := s.syncRunning
	s.saveMu.Unlock()
	if running {
		<-s.syncDone
	}
	s.saveMu.Lock()
	p := s.pg
	s.saveMu.Unlock()
	if p != nil {
		return p.Close()
	}
	return nil
}

// ReloadStats — résumé de l'état réimporté (réponse de POST /api/admin/reload).
type ReloadStats struct {
	OK           bool `json:"ok"`
	Accounts     int  `json:"accounts"`
	Users        int  `json:"users"`
	HotspotUsers int  `json:"hotspotUsers"`
	Routers      int  `json:"routers"`
	Sessions     int  `json:"sessions"`
}

// Reload réimporte l'intégralité de l'état depuis la source persistée
// (PostgreSQL en production, db.json en local) en écrasant l'état mémoire.
// Cas d'usage : maintenance plateforme après une modification SQL directe de
// la base — évite un redémarrage complet du service.
//
// Les garanties d'un démarrage propre sont réappliquées (migration
// multi-tenant idempotente + override admin depuis l'environnement) et le
// cache d'empreintes PG est recalé sur l'état rechargé (PG.Load) : le prochain
// Save() n'écrira que les vraies différences.
//
// L'opération se déroule sous verrou global : les requêtes concurrentes sont
// simplement mises en attente le temps de la lecture (~1-2 s sur Neon).
func (s *Store) Reload() (ReloadStats, error) {
	s.Lock()
	defer s.Unlock()

	var db *model.DB
	if s.pg != nil {
		loaded, found, err := s.pg.Load()
		if err != nil {
			return ReloadStats{}, fmt.Errorf("lecture PostgreSQL : %w", err)
		}
		if !found {
			return ReloadStats{}, errors.New("base vide — rechargement refusé, état mémoire conservé")
		}
		db = loaded
	} else if s.pgActive.Load() {
		// N°164 — mode dégradé : pas de pool, pas de rechargement.
		return ReloadStats{}, errors.New("persistance degradée (PostgreSQL injoignable) — rechargement impossible tant que la récupération n'a pas abouti")
	} else {
		data, err := os.ReadFile(s.path)
		if err != nil {
			return ReloadStats{}, fmt.Errorf("lecture %s : %w", s.path, err)
		}
		fresh := &model.DB{}
		if err := json.Unmarshal(data, fresh); err != nil {
			return ReloadStats{}, fmt.Errorf("fichier illisible : %w", err)
		}
		unsealSecrets(fresh)
		db = fresh
	}

	s.db = db
	s.ensureSlices()

	// Garanties identiques à un démarrage propre, persistées si besoin.
	changed := migrateMultiTenant(s.db)
	if migrateDetachPlatform(s.db) {
		changed = true
	}
	if migrateRemoveOperator(s.db) {
		changed = true
	}
	if migrateUsageScopedPlans(s.db) {
		changed = true
	}
	if migrateActiveTrialCap(s.db) {
		changed = true
	}
	if applyAdminOverride(s.db) {
		changed = true
	}
	if changed {
		s.Save()
	}

	return ReloadStats{
		OK:           true,
		Accounts:     len(s.db.Accounts),
		Users:        len(s.db.Users),
		HotspotUsers: len(s.db.HotspotUsers),
		Routers:      len(s.db.Routers),
		Sessions:     len(s.db.Sessions),
	}, nil
}

// ---------------------------------------------------------------------------
// Moteur de simulation : Tick fait "vivre" la démo.
// À appeler sous verrou, au maximum toutes les 2 secondes.
// ---------------------------------------------------------------------------

// Rétention et volumétrie du journal utilisateurs (F3). N°65 : la rétention
// est PAR COMPTE (tenant.logRetentionDays, 30/60/90 j) ; userLogRetention est
// la valeur appliquée aux comptes SANS réglage explicite (défaut 90).
const (
	userLogRetention = time.Duration(model.DefaultLogRetentionDays) * 24 * time.Hour
	maxUserLogs      = 5000 // garde-fou volumétrie (mode JSON)
)

// Tick — progression du temps simulé :
//   - moteur d'expiration cloud (applyExpiry, F1/F5) AVANT tout ;
//   - uptime/octet des sessions actives (1-3 Mo/s descendant, 0,2-0,8 Mo/s montant)
//   - télémétrie des routeurs (uptime croissant, CPU random walk 5-45 %)
//   - ~30 % de chance de créer une session depuis un user actif (voucher -> used)
//   - ~12 % de chance de terminer une session aléatoire
//   - P0 : journalisation login/logout (F3) + verrouillage LockUser (F1)
//
// N°133 — P1 audit performance : `touched` collecte les tables RÉELLEMENT
// modifiées par le passage (nil = marquage désactivé — Sweep, tests) pour
// une sauvegarde ciblée (Store.SaveTables). En croisière d'un parc AGENT,
// seuls routers (télémétrie) et settings (last_tick, écrit par syncSettings
// à chaque flush) bougent : les 3 000+ utilisateurs hotspot et 5 000 lignes
// de journal ne sont plus re-hashés à chaque lecture pollée.
func Tick(db *model.DB, now time.Time, touched *TableSet) {
	if !db.LastTick.IsZero() && now.Sub(db.LastTick) <= 2*time.Second {
		return // trop tôt : RIEN n'a bougé, rien à marquer
	}

	// P0 (audit Mikhmon) — moteur d'expiration cloud en TÊTE : les lectures
	// suivantes voient des statuts à jour. L'enforcement routeur (commandes
	// agent) est réalisé par les handlers via enforceExpired.
	applyExpiry(db, now, touched)

	var dt int64
	if db.LastTick.IsZero() {
		dt = 0
	} else {
		dt = int64(now.Sub(db.LastTick).Seconds())
		if dt > 300 {
			dt = 300 // borne les sauts après une longue inactivité
		}
		if dt < 0 {
			dt = 0
		}
	}
	db.LastTick = now

	// Télémétrie routeurs — N°133 : UptimeSec/CPULoad progressent pour TOUS
	// les routeurs (l'affichage console en dépend), la table est donc marquée
	// dès qu'un routeur existe — quelques lignes, re-hash immédiat.
	if len(db.Routers) > 0 {
		touched.Mark(TableRouters)
	}
	for i := range db.Routers {
		db.Routers[i].UptimeSec += dt
		db.Routers[i].CPULoad = clamp(db.Routers[i].CPULoad+rand.Intn(13)-6, 5, 45)
		// Routeurs simulés : « vus » à chaque tick (sessions vivantes → état
		// « en ligne » des vouchers actif en démo). Les routeurs agents ne sont
		// PAS touchés ici : leur LastSeen réel (touchAgent) porte la garde
		// anti-sessions figées de ResolvedStatus/onlineSessions.
		if db.Routers[i].Mode == "simulated" {
			db.Routers[i].LastSeen = now.UTC().Format(time.RFC3339)
			// N°103 — le routeur simulé a son WAN par convention
			// (ether1, cf. newSimTraffic) : la carte Qualité de ligne
			// est immédiatement lisible en démo, sans attendre une
			// détection qui n'existe pas sur un routeur fictif.
			if db.Routers[i].WanIface == "" {
				db.Routers[i].WanIface = "ether1"
			}
		}
	}

	// P1 (audit Mikhmon) — F6 : marche aléatoire du trafic des routeurs
	// simulés (débits lissés 0,5-50 Mbps, compteurs cumulés, point
	// d'historique toutes les ~5 s). Vérrouillé + rapide par construction
	// (quelques interfaces par routeur, aucune allocation lourde).
	tickTraffic(db, now, dt, touched)

	// Index utilisateurs par nom + profils par id (F1)
	userIdx := make(map[string]int, len(db.HotspotUsers))
	profileIdx := make(map[string]int, len(db.Profiles))
	for i := range db.HotspotUsers {
		userIdx[db.HotspotUsers[i].Username] = i
	}
	for i := range db.Profiles {
		profileIdx[db.Profiles[i].ID] = i
	}

	// Modes de routeurs — P0 (audit clignotement) : les sessions des
	// routeurs RÉELS (agent) vivent au rythme du routeur (diff read_state
	// F3) et ne doivent subir AUCUNE dynamique simulée. Sinon le moteur de
	// démo coupe aléatoirement des sessions réelles (journal
	// « login/logout en continu » alors que la connexion routeur est
	// stable) et fausse les compteurs utilisateurs avec des octets fictifs.
	routerModes := make(map[string]string, len(db.Routers))
	for i := range db.Routers {
		routerModes[db.Routers[i].ID] = db.Routers[i].Mode
	}

	// Progression (et purge) des sessions — P0 : chaque session coupée
	// (utilisateur supprimé/désactivé) produit un UserLog "logout" (F3).
	// N°133 — flags de marquage ciblé : seules les VRAIES mutations marquent
	// (un parc agent sans session simulée ne marque ni sessions ni users).
	sessionsChanged := false // lignes créées/supprimées/progressées
	usersChanged := false    // compteurs octets/uptime des utilisateurs simulés
	kept := db.Sessions[:0]
	for i := range db.Sessions {
		s := db.Sessions[i]
		idx, ok := userIdx[s.Username]
		if !ok {
			logUserEvent(db, s, "logout", now, touched) // utilisateur supprimé -> session abandonnée
			sessionsChanged = true
			continue
		}
		u := &db.HotspotUsers[idx]
		if u.Status == "disabled" {
			logUserEvent(db, s, "logout", now, touched) // utilisateur désactivé -> session coupée
			sessionsChanged = true
			continue
		}
		if routerModes[s.RouterID] != "simulated" {
			// Réel/agent : uptime/bytes viennent du read_state ; le
			// quota temps est cumulé à la DÉCONNEXION détectée
			// (accumulateUptime, F3) — jamais ici (double comptage).
			kept = append(kept, s)
			continue
		}
		s.UptimeSec += dt
		// Sémantique RouterOS (doc officielle) : bytes-in = UPLOADED (petit),
		// bytes-out = DOWNLOADED (gros) — la démo doit se comporter comme un
		// vrai routeur (download ≫ upload), sinon l'UI paraît inversée.
		dIn := dt * int64(200_000+rand.Intn(600_000))      // 0,2-0,8 Mo/s upload
		dOut := dt * int64(1_000_000+rand.Intn(2_000_000)) // 1-3 Mo/s download
		s.BytesIn += dIn
		s.BytesOut += dOut
		u.BytesIn += dIn
		u.BytesOut += dOut
		u.UptimeUsedSec += dt
		sessionsChanged = true
		usersChanged = true
		kept = append(kept, s)
	}
	db.Sessions = kept

	// Nouvelle session (~30 %) depuis un utilisateur actif sans session
	if len(db.Sessions) < 40 && rand.Float64() < 0.30 {
		active := map[string]bool{}
		for _, s := range db.Sessions {
			active[s.Username] = true
		}
		candidates := []int{}
		for i := range db.HotspotUsers {
			u := &db.HotspotUsers[i]
			if active[u.Username] || u.Status != "active" {
				continue
			}
			// N°119 — stock confié INTOUCHABLE (même principe que
			// sweepDeadBatches N°26/W1) : un ticket remis à un
			// revendeur attend sa VENTE, pas une connexion démo.
			// Le moteur ne doit JAMAIS le consommer (« used », ce
			// l'exclurait du stock vente) ni fabriquer une vente
			// fantôme (VouchersSold/Revenue). Incident E2E N°118 :
			// trois tickets d'un stock de 72 « connectés » par des
			// ticks GLOBAUX déclenchés par les lectures console
			// d'AUTRES comptes → stock amputé à 69, « Afficher
			// plus (60 sur 69) » — la simulation est mondiale,
			// le stock du revendeur ne doit pas l'être.
			if u.ResellerID != "" {
				continue
			}
			if u.Kind == "voucher" && model.EffectiveStatus(u, now) != "active" {
				continue
			}
			r := findRouterByID(db, u.RouterID)
			if r == nil || r.Mode != "simulated" {
				continue
			}
			candidates = append(candidates, i)
		}
		if len(candidates) > 0 {
			i := candidates[rand.Intn(len(candidates))]
			u := &db.HotspotUsers[i]
			r := findRouterByID(db, u.RouterID)
			nowISO := model.NowISO()
			sess := model.Session{
				ID:          model.NewID("s-"),
				AccountID:   u.AccountID, // la session vit dans le compte de l'utilisateur source
				UserID:      u.ID,
				Username:    u.Username,
				ProfileName: u.ProfileName,
				RouterID:    r.ID,
				RouterName:  r.Name,
				IP:          randIP(r),
				MAC:         model.RandomMAC(),
				StartedAt:   nowISO,
				UptimeSec:   0,
				BytesIn:     0,
				BytesOut:    0,
			}
			db.Sessions = append(db.Sessions, sess)
			logUserEvent(db, sess, "login", now, touched) // P0 : F3 — session créée
			sessionsChanged = true
			if u.Kind == "voucher" {
				u.Status = "used"
				u.UsedAt = nowISO
				model.AnchorVoucherValidity(db, u, now) // validité ancrée au 1er login
				usersChanged = true
				if u.ResellerID != "" {
					for j := range db.Resellers {
						if db.Resellers[j].ID == u.ResellerID {
							db.Resellers[j].VouchersSold++
							db.Resellers[j].Revenue += u.Price
							touched.Mark(TableResellers) // vente simulée créditée
						}
					}
				}
			}
		}
	}

	// Fin de session aléatoire (~12 %) — P0 : le username est capturé AVANT
	// la suppression pour journaliser le logout (F3). UNIQUEMENT en
	// simulation : une session de routeur réel/agent ne peut être coupée
	// que par le routeur lui-même (diff read_state F3), jamais par le
	// moteur de démo — sinon clignotement login/logout de sessions réelles.
	if len(db.Sessions) > 0 && rand.Float64() < 0.12 {
		candidates := []int{}
		for i := range db.Sessions {
			if routerModes[db.Sessions[i].RouterID] == "simulated" {
				candidates = append(candidates, i)
			}
		}
		if len(candidates) > 0 {
			i := candidates[rand.Intn(len(candidates))]
			logUserEvent(db, db.Sessions[i], "logout", now, touched)
			db.Sessions = append(db.Sessions[:i], db.Sessions[i+1:]...)
			sessionsChanged = true
		}
	}

	// P0 (audit Mikhmon) — LockUser (F1) : un utilisateur dont le profil
	// verrouille les sessions n'en garde qu'une — les plus anciennes sont
	// fermées (kick) et journalisées.
	if kickLockedUsers(db, userIdx, profileIdx, now, touched) > 0 {
		sessionsChanged = true
	}
	if sessionsChanged {
		touched.Mark(TableSessions)
	}
	if usersChanged {
		touched.Mark(TableHotspotUsers)
	}
}

// logUserEvent ajoute une entrée au journal utilisateurs (F3), sous verrou.
// N°133 — marque la table user_logs (chaque appel AJOUTE une ligne).
func logUserEvent(db *model.DB, s model.Session, action string, now time.Time, touched *TableSet) {
	if db.UserLogs == nil {
		db.UserLogs = []model.UserLog{}
	}
	db.UserLogs = append(db.UserLogs, model.UserLog{
		ID:         model.NewID("ul-"),
		AccountID:  s.AccountID,
		UserID:     s.UserID,
		Username:   s.Username,
		Action:     action, // login | logout | expire | kick
		RouterID:   s.RouterID,
		RouterName: s.RouterName,
		IP:         s.IP,
		MAC:        s.MAC,
		At:         now.UTC().Format(time.RFC3339),
	})
	touched.Mark(TableUserLogs)
}

// kickLockedUsers — F1 LockUser : pour chaque utilisateur dont le profil a
// lockUser et qui possède plus d'une session active, les plus anciennes sont
// fermées (UserLog "kick"). La session la plus récente est conservée.
// N°133 — retourne le nombre de sessions fermées (marquage ciblé de Tick).
func kickLockedUsers(db *model.DB, userIdx, profileIdx map[string]int, now time.Time, touched *TableSet) int {
	sessByUser := map[string][]model.Session{}
	for _, s := range db.Sessions {
		sessByUser[s.Username] = append(sessByUser[s.Username], s)
	}
	kicked := map[string]bool{}
	for username, list := range sessByUser {
		if len(list) <= 1 {
			continue
		}
		idx, ok := userIdx[username]
		if !ok {
			continue
		}
		u := &db.HotspotUsers[idx]
		pIdx, ok := profileIdx[u.ProfileID]
		if !ok || !db.Profiles[pIdx].LockUser {
			continue
		}
		// La plus récente (StartedAt max) est conservée.
		sort.Slice(list, func(i, j int) bool { return list[i].StartedAt < list[j].StartedAt })
		for _, v := range list[:len(list)-1] {
			kicked[v.ID] = true
		}
	}
	if len(kicked) == 0 {
		return 0
	}
	kept := db.Sessions[:0]
	for _, s := range db.Sessions {
		if kicked[s.ID] {
			logUserEvent(db, s, "kick", now, touched)
			continue
		}
		kept = append(kept, s)
	}
	db.Sessions = kept
	return len(kicked)
}

// Sweep — N°64 — point d'entrée du BALAYAGE PÉRIODIQUE (goroutine main.go,
// 1 h + rattrapage au démarrage) : même moteur d'expiration/rétention que
// Tick (applyExpiry) SANS la progression de la simulation (sessions, uptime,
// télémétrie aléatoire). La purge des journaux (rétention 30/60/90 j PAR
// COMPTE, N°65 + plafond 5 000) ne dépend ainsi PLUS des seules lectures
// console : un compte dormant, jamais
// consulté, est couvert aussi. Date le passage (db.LastSweep, preuve d'audit
// via GET /) et renvoie le nombre d'entrées du journal purgées.
// À appeler sous verrou ; le Save est à charge de l'appelant.
// N°133 — `touched` collecte les tables modifiées (nil = balayage complet
// non ciblé : le goroutine de rétention appelle Save() full, plus simple
// et sans risque pour un passage horaire).
func Sweep(db *model.DB, now time.Time, touched *TableSet) int {
	before := len(db.UserLogs)
	applyExpiry(db, now, touched)
	db.LastSweep = now
	return before - len(db.UserLogs)
}

// applyExpiry — moteur d'expiration cloud (F1 + nettoyage F5), appelé en tête
// de Tick (lectures console) ET par le balayage périodique (Sweep, N°64),
// sous verrou :
//
//  1. chaque utilisateur (voucher) « active » dont ExpiresAt + grâce du profil
//     est dépassé passe « expired » (Enforced=false) + UserLog {action:"expire"} ;
//  2. politique de nettoyage du compte (expiryPolicyMode == "remove") :
//     les utilisateurs « expired » dont l'expiration date de plus de
//     expiryPolicyAfterDays jours sont supprimés du cloud (+ Activity résumé) ;
//  3. purge des UserLogs selon la rétention DU COMPTE (30/60/90 j, N°65 ;
//     défaut 90) + garde-fou volumétrie (5 000 entrées) ;
//  4. N°103 — rétention des agrégats de qualité de ligne (90 j glissants,
//     global : télémétrie de ligne, pas des données client).
//
// Retour : accountID → usernames dont l'expiration vient d'être appliquée
// (information disponible pour l'enforcement routeur — cf. enforceExpired).
// N°133 — `touched` collecte les tables réellement modifiées (nil = sans
// marquage) : expiration (hotspot_users + user_logs), nettoyage (hotspot_users
// + activity), rétentions (user_logs, line_quality).
func applyExpiry(db *model.DB, now time.Time, touched *TableSet) map[string][]string {
	applied := map[string][]string{}
	if db.UserLogs == nil {
		db.UserLogs = []model.UserLog{}
	}
	// 4. N°103 — rétention de la télémétrie de ligne (en tête, avant tout
	// return : tous les chemins d'applyExpiry passent par ici).
	if model.PruneLineQuality(db, now) > 0 {
		touched.Mark(TableLineQuality)
	}

	// 1. Passage « expired » (grâce du profil prise en compte). Comme
	// EffectiveStatus, l'expiration cloud ne s'applique qu'aux vouchers : le
	// statut des utilisateurs réguliers reste géré manuellement
	// (active/disabled) — leur date d'expiration est informative.
	grace := make(map[string]int, len(db.Profiles))
	noExpiry := make(map[string]bool, len(db.Profiles))
	for _, p := range db.Profiles {
		g := p.GracePeriodMin
		if g < 0 {
			g = 0
		}
		grace[p.ID] = g
		// Parité Mikhmon — ExpMode « none » : aucune expiration cloud, le
		// voucher reste actif jusqu'à épuisement temps/data sur le routeur.
		noExpiry[p.ID] = p.ExpMode == "none"
	}
	for i := range db.HotspotUsers {
		u := &db.HotspotUsers[i]
		if u.Kind != "voucher" || noExpiry[u.ProfileID] {
			continue
		}
		// BUG (audit Winbox) — les vouchers « used » (vendus puis utilisés au
		// 1er login : le cycle de vie NORMAL) n'étaient JAMAIS expirés : le
		// statut stocké restait « used » après le passage de la validité, donc
		// enforceExpired (qui ne traite que les « expired ») ne poussait AUCUNE
		// commande au routeur → tickets expirés toujours présents dans
		// /ip hotspot user (Winbox). Seuls les vouchers déjà hors service
		// (disabled/expired) restent exclus de la transition.
		if u.Status != "active" && u.Status != "used" {
			continue
		}
		// Expiration par date : validité ancrée au 1er login + grâce du profil.
		over := false
		if u.ExpiresAt != "" {
			if exp, err := time.Parse(time.RFC3339, u.ExpiresAt); err == nil {
				over = now.After(exp.Add(time.Duration(grace[u.ProfileID]) * time.Minute))
			}
		}
		// …ou par épuisement du quota temps cumulé (parité limit-uptime, même
		// règle que voucherExpired/EffectiveStatus) : le routeur a déjà coupé
		// la session, le cloud reflète l'expiration et pousse l'enforcement.
		if !over && u.TimeLimitMin > 0 && u.UptimeUsedSec >= u.TimeLimitMin*60 {
			over = true
		}
		if !over {
			continue
		}
		u.Status = "expired"
		u.Enforced = false // à appliquer au routeur par enforceExpired
		logUserEvent(db, model.Session{
			AccountID: u.AccountID, UserID: u.ID, Username: u.Username,
			RouterID: u.RouterID, RouterName: u.RouterName,
		}, "expire", now, touched)
		applied[u.AccountID] = append(applied[u.AccountID], u.Username)
	}
	if len(applied) > 0 {
		touched.Mark(TableHotspotUsers) // statuts « expired » posés
	}

	// 2. Nettoyage cloud (F5) : politique « remove » par compte.
	removed := map[string]int{}
	for accID, s := range db.SettingsByAccount {
		if s.Tenant.ExpiryPolicyMode != "remove" {
			continue
		}
		days := s.Tenant.ExpiryPolicyAfterDays
		if days < 0 {
			days = 0
		}
		deadline := now.AddDate(0, 0, -days)
		kept := db.HotspotUsers[:0]
		for _, u := range db.HotspotUsers {
			if u.AccountID == accID && u.Status == "expired" && u.ExpiresAt != "" {
				if exp, err := time.Parse(time.RFC3339, u.ExpiresAt); err == nil && exp.Before(deadline) {
					removed[accID]++
					continue // supprimé du cloud
				}
			}
			kept = append(kept, u)
		}
		db.HotspotUsers = kept
	}
	for accID, n := range removed {
		days := 0
		if s, ok := db.SettingsByAccount[accID]; ok {
			days = s.Tenant.ExpiryPolicyAfterDays
		}
		db.Activity = append([]model.Activity{{
			ID:        model.NewID("act-"),
			AccountID: accID,
			Type:      "user",
			Message:   fmt.Sprintf("Nettoyage : %d utilisateurs expirés supprimés (politique %d j)", n, days),
			At:        model.NowISO(),
		}}, db.Activity...)
		if len(db.Activity) > 500 {
			db.Activity = db.Activity[:500]
		}
	}
	if len(removed) > 0 {
		touched.Mark(TableHotspotUsers, TableActivity) // suppressions + résumés
	}

	// 3. Rétention du journal utilisateurs — N°65 : 30/60/90 jours PAR COMPTE
	// (tenant.logRetentionDays ; défaut 90 = userLogRetention) + garde-fou
	// volumétrie global (5 000 dernières entrées).
	cutoffs := make(map[string]string, len(db.SettingsByAccount))
	for accID, s := range db.SettingsByAccount {
		cutoffs[accID] = now.AddDate(0, 0, -s.Tenant.LogRetentionDaysEffective()).Format(time.RFC3339)
	}
	defLim := now.Add(-userLogRetention).Format(time.RFC3339)
	logsBefore := len(db.UserLogs)
	keptLogs := db.UserLogs[:0]
	for _, l := range db.UserLogs {
		lim := defLim
		if c, ok := cutoffs[l.AccountID]; ok {
			lim = c
		}
		if l.At < lim {
			continue
		}
		keptLogs = append(keptLogs, l)
	}
	if len(keptLogs) > maxUserLogs {
		keptLogs = keptLogs[len(keptLogs)-maxUserLogs:]
	}
	db.UserLogs = keptLogs
	if len(db.UserLogs) != logsBefore {
		touched.Mark(TableUserLogs) // rétention/plafond : lignes purgées
	}

	return applied
}

// ---------------------------------------------------------------------------
// F6 (P1) — Trafic temps réel des routeurs simulés
// ---------------------------------------------------------------------------

// Bornes de la marche aléatoire des débits simulés (0,5-50 Mbps, contrat F6).
const (
	simMinRxBps = int64(500_000)    // 0,5 Mbps
	simMaxRxBps = int64(50_000_000) // 50 Mbps
	trafficCap  = 60                // historique : 60 derniers points
)

// tickTraffic — F6 : fait vivre le trafic des routeurs SIMULÉS :
//   - init à la volée (routeur créé sans seed) : 3 interfaces ether1/wlan1/
//     hotspot, compteurs cumulés plausibles + historique backfillé (12 points
//     de 5 s, comme le seed) pour un graphique immédiatement lisible ;
//   - tick suivant : marche aléatoire LISSÉE (bps = ¾ ancien + ¼ cible,
//     cible uniforme 0,5-50 Mbps ; tx ≈ 10-28 % de rx), compteurs cumulés
//     avancés de bps×dt/8 ;
//   - TrafficPoint (somme des interfaces) si le dernier point date de ≥ 5 s.
//
// Tick reste rapide : 3 interfaces par routeur, aucune allocation au-delà des
// points d'historique (1 toutes les 5 s).
// N°133 — ne marque `touched` QUE si au moins un routeur simulé existe : un
// parc 100 % agent ne touche ni traffic ni line_quality ici (leurs lignes
// vivent au rythme du read_state, persisté par les Save() complets).
func tickTraffic(db *model.DB, now time.Time, dt int64, touched *TableSet) {
	if db.Traffic == nil {
		db.Traffic = []model.RouterTraffic{}
	}
	simulated := false
	for i := range db.Routers {
		rr := &db.Routers[i]
		if rr.Mode != "simulated" {
			continue
		}
		simulated = true
		var tr *model.RouterTraffic
		for j := range db.Traffic {
			if db.Traffic[j].RouterID == rr.ID {
				tr = &db.Traffic[j]
				break
			}
		}
		if tr == nil {
			db.Traffic = append(db.Traffic, newSimTraffic(rr))
			continue
		}
		var sumRx, sumTx int64
		for j := range tr.Interfaces {
			it := &tr.Interfaces[j]
			target := simMinRxBps + rand.Int63n(simMaxRxBps-simMinRxBps)
			nb := it.RxBps*3/4 + target/4
			if nb < simMinRxBps {
				nb = simMinRxBps
			}
			if nb > simMaxRxBps {
				nb = simMaxRxBps
			}
			it.RxBps = nb
			it.TxBps = nb * int64(10+rand.Intn(19)) / 100 // tx ≈ 10-28 % de rx
			it.RxBytes += it.RxBps / 8 * dt
			it.TxBytes += it.TxBps / 8 * dt
			sumRx += it.RxBps
			sumTx += it.TxBps
		}
		if len(tr.Interfaces) > 0 && historyStale(tr, now, 5*time.Second) {
			tr.History = append(tr.History, model.TrafficPoint{
				T: now.UTC().Format(time.RFC3339), RxBps: sumRx, TxBps: sumTx,
			})
			if len(tr.History) > trafficCap {
				tr.History = tr.History[len(tr.History)-trafficCap:]
			}
		}
		tr.UpdatedAt = now.UTC().Format(time.RFC3339Nano)
		// N°103 — Qualité de ligne : le SIMULÉ échantillonne aussi (les
		// agrégats se remplissent au rythme des ticks — la carte et la
		// recommandation QoS se testent en démo, pattern de la plateforme
		// « tout est essayable sans matériel »). Même garde que l'agent :
		// uniquement les fenêtres réelles (dt > 0).
		if dt > 0 {
			model.AccumulateLineQuality(db, rr, tr.Interfaces, now)
		}
	}
	if simulated {
		// Les lignes de trafic simulé progressent à chaque tick
		// (débits, compteurs, UpdatedAt) ; la qualité de ligne
		// s'échantillonne dès qu'une fenêtre réelle s'écoule (dt > 0).
		touched.Mark(TableTraffic)
		if dt > 0 {
			touched.Mark(TableLineQuality)
		}
	}
}

// historyStale — true si le dernier point d'historique est plus vieux que d.
func historyStale(tr *model.RouterTraffic, now time.Time, d time.Duration) bool {
	if len(tr.History) == 0 {
		return true
	}
	last, err := time.Parse(time.RFC3339, tr.History[len(tr.History)-1].T)
	if err != nil {
		return true
	}
	return now.Sub(last) >= d
}

// newSimTraffic — état de trafic initial d'un routeur simulé (3 interfaces,
// compteurs cumulés fonction de l'uptime, 12 points d'historique sur 1 min).
func newSimTraffic(rr *model.Router) model.RouterTraffic {
	rxBps := func() int64 { return simMinRxBps + rand.Int63n(simMaxRxBps-simMinRxBps) }
	txBps := func() int64 { return rxBps() * int64(10+rand.Intn(19)) / 100 }
	now := time.Now().UTC()
	cumul := func(avg int64) int64 { return rr.UptimeSec * avg / 8 }
	avgRx := int64(6_000_000) // ~6 Mbps de moyenne cumulée
	ifaces := []model.IfaceTraffic{
		{Name: "ether1", RxBytes: cumul(avgRx), TxBytes: cumul(avgRx / 3), RxBps: rxBps(), TxBps: txBps()},
		{Name: "wlan1", RxBytes: cumul(avgRx * 2 / 3), TxBytes: cumul(avgRx / 2), RxBps: rxBps(), TxBps: txBps()},
		{Name: "hotspot", RxBytes: cumul(avgRx / 2), TxBytes: cumul(avgRx * 2 / 3), RxBps: rxBps(), TxBps: txBps()},
	}
	var sumRx, sumTx int64
	history := make([]model.TrafficPoint, 0, 12)
	for i := 11; i >= 0; i-- {
		sumRx, sumTx = 0, 0
		for range ifaces {
			rx, tx := rxBps(), txBps()
			sumRx += rx
			sumTx += tx
		}
		history = append(history, model.TrafficPoint{
			T:     now.Add(-time.Duration(i) * 5 * time.Second).UTC().Format(time.RFC3339),
			RxBps: sumRx, TxBps: sumTx,
		})
	}
	return model.RouterTraffic{
		ID: rr.ID, RouterID: rr.ID, AccountID: rr.AccountID,
		UpdatedAt:  now.Format(time.RFC3339Nano),
		Interfaces: ifaces,
		History:    history,
	}
}

// ---------------------------------------------------------------------------
// Helpers (sous verrou)
// ---------------------------------------------------------------------------

func findRouterByID(db *model.DB, id string) *model.Router {
	for i := range db.Routers {
		if db.Routers[i].ID == id {
			return &db.Routers[i]
		}
	}
	return nil
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func randIP(r *model.Router) string {
	prefix := "10.10.10."
	if i := strings.LastIndexByte(r.Host, '.'); i > 0 {
		prefix = r.Host[:i+1]
	}
	return prefix + strconv.Itoa(2+rand.Intn(200))
}
