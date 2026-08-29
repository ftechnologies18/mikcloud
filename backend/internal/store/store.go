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
			return s, nil
		}
	} else {
		s.db = BuildSeed()
	}
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

	// 2. Crée ou met à jour le compte administrateur déclaré par l'environnement.
	// L'admin plateforme est toujours rattaché au compte principal.
	salt := auth.NewSalt()
	for i := range db.Users {
		if db.Users[i].Username == username {
			db.Users[i].Name = name
			db.Users[i].Role = "admin"
			db.Users[i].AccountID = model.AccountMainID
			db.Users[i].Salt = salt
			db.Users[i].PasswordHash = auth.HashPassword(password, salt)
			log.Printf("store: compte admin « %s » mis à jour depuis l'environnement", username)
			if removedDemo {
				log.Println("store: compte démo admin/admin123 supprimé")
			}
			return true
		}
	}
	db.Users = append(db.Users, model.AdminUser{
		ID:           model.NewID("adm-"),
		AccountID:    model.AccountMainID,
		Name:         name,
		Username:     username,
		Role:         "admin",
		PasswordHash: auth.HashPassword(password, salt),
		Salt:         salt,
		CreatedAt:    model.NowISO(),
	})
	log.Printf("store: compte admin « %s » créé depuis l'environnement", username)
	if removedDemo {
		log.Println("store: compte démo admin/admin123 supprimé")
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

// Tick — progression du temps simulé :
//   - uptime/octet des sessions actives (1-3 Mo/s descendant, 0,2-0,8 Mo/s montant)
//   - télémétrie des routeurs (uptime croissant, CPU random walk 5-45 %)
//   - ~30 % de chance de créer une session depuis un user actif (voucher -> used)
//   - ~12 % de chance de terminer une session aléatoire
func Tick(db *model.DB, now time.Time) {
	if !db.LastTick.IsZero() && now.Sub(db.LastTick) <= 2*time.Second {
		return
	}
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

	// Index utilisateurs par nom
	userIdx := make(map[string]int, len(db.HotspotUsers))
	for i := range db.HotspotUsers {
		userIdx[db.HotspotUsers[i].Username] = i
	}

	// Progression (et purge) des sessions
	kept := db.Sessions[:0]
	for i := range db.Sessions {
		s := db.Sessions[i]
		idx, ok := userIdx[s.Username]
		if !ok {
			continue // utilisateur supprimé -> session abandonnée
		}
		u := &db.HotspotUsers[idx]
		if u.Status == "disabled" {
			continue // utilisateur désactivé -> session coupée
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
			db.Sessions = append(db.Sessions, model.Session{
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
			})
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

	// Fin de session aléatoire (~12 %)
	if len(db.Sessions) > 0 && rand.Float64() < 0.12 {
		i := rand.Intn(len(db.Sessions))
		db.Sessions = append(db.Sessions[:i], db.Sessions[i+1:]...)
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
