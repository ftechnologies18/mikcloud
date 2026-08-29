// Package store — persistance + moteur de simulation (tick).
//
// Deux modes de persistance, choisis automatiquement au démarrage :
//   - DATABASE_URL défini  → PostgreSQL (production : Render + Neon),
//     schéma relationnel + synchro différentielle (voir pg.go) ;
//   - sinon                → fichier JSON atomique (développement local).
package store

import (
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
	"time"

	"mikcloud/hotspot-api/internal/auth"
	"mikcloud/hotspot-api/internal/model"
)

// Store — base de données en mémoire protégée par un mutex global, persistée
// soit en PostgreSQL (synchro différentielle), soit en JSON atomique
// (écriture tmp + rename).
type Store struct {
	mu   sync.Mutex
	db   *model.DB
	path string // mode JSON uniquement
	pg   *PG    // mode PostgreSQL uniquement
}

// New charge l'état persisté (PostgreSQL si DATABASE_URL est défini, sinon
// data/db.json) ou crée le seed démo si la source est vide/absente.
func New(dir string) (*Store, error) {
	s := &Store{}

	// Bascule en mode PostgreSQL uniquement pour une URL postgres:// (ou
	// postgresql://) ; toute autre valeur (ex. sqlite locale) est ignorée
	// au profit du mode JSON.
	databaseURL := os.Getenv("DATABASE_URL")
	if strings.HasPrefix(databaseURL, "postgres://") || strings.HasPrefix(databaseURL, "postgresql://") {
		pg, err := OpenPG(databaseURL)
		if err != nil {
			return nil, err
		}
		s.pg = pg
		db, found, err := pg.Load()
		if err != nil {
			pg.Close()
			return nil, err
		}
		if found {
			log.Printf("store: état chargé depuis PostgreSQL (%d utilisateurs hotspot, %d routeurs, %d comptes)",
				len(db.HotspotUsers), len(db.Routers), len(db.Accounts))
			s.db = db
		} else {
			log.Println("store: base PostgreSQL vide — initialisation des données démo")
			s.db = BuildSeed() // seed déjà multi-tenant
		}
		// Migration mono-tenant → multi-tenant (avant l'override admin), puis
		// persistance immédiate si l'état a changé.
		if migrateMultiTenant(s.db) {
			s.Lock()
			s.Save()
			s.Unlock()
		}
		// L'override admin (variables d'environnement) s'applique à chaque
		// démarrage et doit être persisté aussitôt.
		changed := applyAdminOverride(s.db)
		if changed || !found {
			s.Lock()
			s.Save()
			s.Unlock()
		}
		log.Println("store: persistance PostgreSQL active (DATABASE_URL)")
		return s, nil
	}

	// Mode développement : fichier JSON local.
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	s.path = filepath.Join(dir, "db.json")
	if data, err := os.ReadFile(s.path); err == nil && len(data) > 0 {
		var db model.DB
		if err := json.Unmarshal(data, &db); err != nil {
			log.Printf("store: db.json illisible (%v) — re-seed des données démo", err)
			s.db = BuildSeed()
		} else {
			s.db = &db
			s.ensureSlices()
			// Un ancien db.json mono-tenant (champs legacy tenant/settings, sans
			// accounts) est migré ici vers le modèle multi-comptes puis re-sauvegardé.
			if migrateMultiTenant(s.db) {
				s.Lock()
				s.Save()
				s.Unlock()
			}
			// Override admin (variables d'environnement) — même garantie
			// qu'en mode PostgreSQL : appliqué à chaque démarrage.
			if applyAdminOverride(s.db) {
				s.Lock()
				s.Save()
				s.Unlock()
			}
			return s, nil
		}
	} else {
		s.db = BuildSeed()
	}
	// Seed frais : l'override admin remplace aussi le compte démo (admin123)
	// par les identifiants de production si ADMIN_PASSWORD est définie.
	applyAdminOverride(s.db)
	s.Lock()
	s.Save()
	s.Unlock()
	log.Printf("store: persistance JSON active (%s)", s.path)
	return s, nil
}

// migrateMultiTenant — fait passer un état mono-tenant (ancien db.json ou base
// PostgreSQL d'avant la migration) au modèle multi-comptes :
//   - crée le compte principal {AccountMainID} s'il n'existe aucun compte ;
//   - backfill AccountID == "" → AccountMainID sur toutes les entités ;
//   - initialise SettingsByAccount (réglages legacy migrés, sinon défauts FCFA) ;
//   - vide les champs legacy db.Tenant / db.Settings.
//
// Idempotent : retourne true uniquement si l'état a été modifié (à persister
// par l'appelant). Le seed démo est déjà multi-tenant : cette fonction n'y
// change rien.
func migrateMultiTenant(db *model.DB) bool {
	changed := false

	if len(db.Accounts) == 0 {
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
		})
		changed = true
	}

	backfill := func(n int, set func(i int)) {
		for i := 0; i < n; i++ {
			set(i)
		}
	}
	backfill(len(db.Users), func(i int) {
		if db.Users[i].AccountID == "" {
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
	backfill(len(db.Activity), func(i int) {
		if db.Activity[i].AccountID == "" {
			db.Activity[i].AccountID = model.AccountMainID
			changed = true
		}
	})
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
	// L'admin plateforme est toujours rattaché au compte principal.
	salt := auth.NewSalt()
	for i := range db.Users {
		if db.Users[i].Username == username {
			u := &db.Users[i]
			u.Name = name
			u.Role = "admin"
			u.AccountID = model.AccountMainID
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
		AccountID:       model.AccountMainID,
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

// Save persiste la base (à appeler sous verrou) : synchro différentielle
// PostgreSQL en production, écriture JSON atomique en développement.
func (s *Store) Save() {
	if s.pg != nil {
		if err := s.pg.Sync(s.db); err != nil {
			// L'état reste en mémoire : la prochaine sauvegarde retentera la synchro.
			log.Printf("store: synchro PostgreSQL échouée (%v) — nouvelle tentative au prochain Save", err)
		}
		return
	}
	data, err := json.MarshalIndent(s.db, "", "  ")
	if err != nil {
		log.Printf("store: sérialisation impossible : %v", err)
		return
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		log.Printf("store: écriture impossible : %v", err)
		return
	}
	if err := os.Rename(tmp, s.path); err != nil {
		log.Printf("store: renommage impossible : %v", err)
	}
}

// Close ferme proprement la persistance (appelé au SIGTERM/SIGINT Render).
// Le Save() synchrone garantit déjà l'écriture à chaque modification ; Close
// ne fait que libérer la connexion PostgreSQL.
func (s *Store) Close() error {
	if s.pg != nil {
		return s.pg.Close()
	}
	return nil
}

// Reset régénère entièrement les données démo (POST /api/admin/reset).
func (s *Store) Reset() {
	s.Lock()
	defer s.Unlock()
	s.db = BuildSeed()
	s.Save()
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
	} else {
		data, err := os.ReadFile(s.path)
		if err != nil {
			return ReloadStats{}, fmt.Errorf("lecture %s : %w", s.path, err)
		}
		fresh := &model.DB{}
		if err := json.Unmarshal(data, fresh); err != nil {
			return ReloadStats{}, fmt.Errorf("fichier illisible : %w", err)
		}
		db = fresh
	}

	s.db = db
	s.ensureSlices()

	// Garanties identiques à un démarrage propre, persistées si besoin.
	changed := migrateMultiTenant(s.db)
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

// Rétention et volumétrie du journal utilisateurs (F3).
const (
	userLogRetention = 90 * 24 * time.Hour // purge au-delà de 90 jours
	maxUserLogs      = 5000                // garde-fou volumétrie (mode JSON)
)

// Tick — progression du temps simulé :
//   - moteur d'expiration cloud (applyExpiry, F1/F5) AVANT tout ;
//   - uptime/octet des sessions actives (1-3 Mo/s descendant, 0,2-0,8 Mo/s montant)
//   - télémétrie des routeurs (uptime croissant, CPU random walk 5-45 %)
//   - ~30 % de chance de créer une session depuis un user actif (voucher -> used)
//   - ~12 % de chance de terminer une session aléatoire
//   - P0 : journalisation login/logout (F3) + verrouillage LockUser (F1)
func Tick(db *model.DB, now time.Time) {
	if !db.LastTick.IsZero() && now.Sub(db.LastTick) <= 2*time.Second {
		return
	}

	// P0 (audit Mikhmon) — moteur d'expiration cloud en TÊTE : les lectures
	// suivantes voient des statuts à jour. L'enforcement routeur (commandes
	// agent) est réalisé par les handlers via enforceExpired.
	applyExpiry(db, now)

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

	// Télémétrie routeurs
	for i := range db.Routers {
		db.Routers[i].UptimeSec += dt
		db.Routers[i].CPULoad = clamp(db.Routers[i].CPULoad+rand.Intn(13)-6, 5, 45)
	}

	// P1 (audit Mikhmon) — F6 : marche aléatoire du trafic des routeurs
	// simulés (débits lissés 0,5-50 Mbps, compteurs cumulés, point
	// d'historique toutes les ~5 s). Vérrouillé + rapide par construction
	// (quelques interfaces par routeur, aucune allocation lourde).
	tickTraffic(db, now, dt)

	// Index utilisateurs par nom + profils par id (F1)
	userIdx := make(map[string]int, len(db.HotspotUsers))
	profileIdx := make(map[string]int, len(db.Profiles))
	for i := range db.HotspotUsers {
		userIdx[db.HotspotUsers[i].Username] = i
	}
	for i := range db.Profiles {
		profileIdx[db.Profiles[i].ID] = i
	}

	// Progression (et purge) des sessions — P0 : chaque session coupée
	// (utilisateur supprimé/désactivé) produit un UserLog "logout" (F3).
	kept := db.Sessions[:0]
	for i := range db.Sessions {
		s := db.Sessions[i]
		idx, ok := userIdx[s.Username]
		if !ok {
			logUserEvent(db, s, "logout", now) // utilisateur supprimé -> session abandonnée
			continue
		}
		u := &db.HotspotUsers[idx]
		if u.Status == "disabled" {
			logUserEvent(db, s, "logout", now) // utilisateur désactivé -> session coupée
			continue
		}
		s.UptimeSec += dt
		dIn := dt * int64(1_000_000+rand.Intn(2_000_000)) // 1-3 Mo/s
		dOut := dt * int64(200_000+rand.Intn(600_000))    // 0,2-0,8 Mo/s
		s.BytesIn += dIn
		s.BytesOut += dOut
		u.BytesIn += dIn
		u.BytesOut += dOut
		u.UptimeUsedSec += dt
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
			logUserEvent(db, sess, "login", now) // P0 : F3 — session créée
			if u.Kind == "voucher" {
				u.Status = "used"
				u.UsedAt = nowISO
				if u.ResellerID != "" {
					for j := range db.Resellers {
						if db.Resellers[j].ID == u.ResellerID {
							db.Resellers[j].VouchersSold++
							db.Resellers[j].Revenue += u.Price
						}
					}
				}
			}
		}
	}

	// Fin de session aléatoire (~12 %) — P0 : le username est capturé AVANT
	// la suppression pour journaliser le logout (F3).
	if len(db.Sessions) > 0 && rand.Float64() < 0.12 {
		i := rand.Intn(len(db.Sessions))
		logUserEvent(db, db.Sessions[i], "logout", now)
		db.Sessions = append(db.Sessions[:i], db.Sessions[i+1:]...)
	}

	// P0 (audit Mikhmon) — LockUser (F1) : un utilisateur dont le profil
	// verrouille les sessions n'en garde qu'une — les plus anciennes sont
	// fermées (kick) et journalisées.
	kickLockedUsers(db, userIdx, profileIdx, now)
}

// logUserEvent ajoute une entrée au journal utilisateurs (F3), sous verrou.
func logUserEvent(db *model.DB, s model.Session, action string, now time.Time) {
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
}

// kickLockedUsers — F1 LockUser : pour chaque utilisateur dont le profil a
// lockUser et qui possède plus d'une session active, les plus anciennes sont
// fermées (UserLog "kick"). La session la plus récente est conservée.
func kickLockedUsers(db *model.DB, userIdx, profileIdx map[string]int, now time.Time) {
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
		return
	}
	kept := db.Sessions[:0]
	for _, s := range db.Sessions {
		if kicked[s.ID] {
			logUserEvent(db, s, "kick", now)
			continue
		}
		kept = append(kept, s)
	}
	db.Sessions = kept
}

// applyExpiry — moteur d'expiration cloud (F1 + nettoyage F5), appelé en tête
// de Tick sous verrou :
//
//  1. chaque utilisateur (voucher) « active » dont ExpiresAt + grâce du profil
//     est dépassé passe « expired » (Enforced=false) + UserLog {action:"expire"} ;
//  2. politique de nettoyage du compte (expiryPolicyMode == "remove") :
//     les utilisateurs « expired » dont l'expiration date de plus de
//     expiryPolicyAfterDays jours sont supprimés du cloud (+ Activity résumé) ;
//  3. purge des UserLogs de plus de 90 jours (rétention F3).
//
// Retour : accountID → usernames dont l'expiration vient d'être appliquée
// (information disponible pour l'enforcement routeur — cf. enforceExpired).
func applyExpiry(db *model.DB, now time.Time) map[string][]string {
	applied := map[string][]string{}
	if db.UserLogs == nil {
		db.UserLogs = []model.UserLog{}
	}

	// 1. Passage « expired » (grâce du profil prise en compte). Comme
	// EffectiveStatus, l'expiration cloud ne s'applique qu'aux vouchers : le
	// statut des utilisateurs réguliers reste géré manuellement
	// (active/disabled) — leur date d'expiration est informative.
	grace := make(map[string]int, len(db.Profiles))
	for _, p := range db.Profiles {
		g := p.GracePeriodMin
		if g < 0 {
			g = 0
		}
		grace[p.ID] = g
	}
	for i := range db.HotspotUsers {
		u := &db.HotspotUsers[i]
		if u.Kind != "voucher" || u.Status != "active" || u.ExpiresAt == "" {
			continue
		}
		exp, err := time.Parse(time.RFC3339, u.ExpiresAt)
		if err != nil {
			continue
		}
		if !now.After(exp.Add(time.Duration(grace[u.ProfileID]) * time.Minute)) {
			continue
		}
		u.Status = "expired"
		u.Enforced = false // à appliquer au routeur par enforceExpired
		logUserEvent(db, model.Session{
			AccountID: u.AccountID, UserID: u.ID, Username: u.Username,
			RouterID: u.RouterID, RouterName: u.RouterName,
		}, "expire", now)
		applied[u.AccountID] = append(applied[u.AccountID], u.Username)
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

	// 3. Rétention du journal utilisateurs (90 jours) + garde-fou volumétrie.
	lim := now.Add(-userLogRetention).Format(time.RFC3339)
	keptLogs := db.UserLogs[:0]
	for _, l := range db.UserLogs {
		if l.At < lim {
			continue
		}
		keptLogs = append(keptLogs, l)
	}
	if len(keptLogs) > maxUserLogs {
		keptLogs = keptLogs[len(keptLogs)-maxUserLogs:]
	}
	db.UserLogs = keptLogs

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
func tickTraffic(db *model.DB, now time.Time, dt int64) {
	if db.Traffic == nil {
		db.Traffic = []model.RouterTraffic{}
	}
	for i := range db.Routers {
		rr := &db.Routers[i]
		if rr.Mode != "simulated" {
			continue
		}
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
