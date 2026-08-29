// Package store — persistance + moteur de simulation (tick).
//
// Deux modes de persistance, choisis automatiquement au démarrage :
//   - DATABASE_URL défini  → PostgreSQL (production : Render + Neon),
//     schéma relationnel + synchro différentielle (voir pg.go) ;
//   - sinon                → fichier JSON atomique (développement local).
package store

import (
	"encoding/json"
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
			log.Printf("store: état chargé depuis PostgreSQL (%d utilisateurs hotspot, %d routeurs)",
				len(db.HotspotUsers), len(db.Routers))
			s.db = db
		} else {
			log.Println("store: base PostgreSQL vide — initialisation des données démo")
			s.db = BuildSeed()
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
	salt := auth.NewSalt()
	for i := range db.Users {
		if db.Users[i].Username == username {
			db.Users[i].Name = name
			db.Users[i].Role = "admin"
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

// Reset régénère entièrement les données démo (POST /api/admin/reset).
func (s *Store) Reset() {
	s.Lock()
	defer s.Unlock()
	s.db = BuildSeed()
	s.Save()
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
