// Package store — état initial de mise en service (base vide).
package store

import (
	"time"

	"mikcloud/hotspot-api/internal/auth"
	"mikcloud/hotspot-api/internal/model"
)

// BuildEmptyState construit l'état initial de MISE EN SERVICE : administrateur
// plateforme + AUCUNE donnée — zéro compte client, zéro routeur, zéro profil,
// zéro utilisateur hotspot, zéro lot, zéro revendeur, zéro vente.
//
// L'admin plateforme est un OPÉRATEUR du SaaS : il n'a ni compte client ni
// abonnement propre (les comptes se créent depuis la console plateforme ou
// l'inscription sur invitation ; leurs consoles s'ouvrent par session support).
//
// Utilisé au démarrage de toute base vide ou illisible (production
// Render/Neon comme développement JSON local) : le système démarre vide,
// prêt pour les premiers clients. Les données de démo ne reviennent JAMAIS
// d'elles-mêmes — le seed démo (BuildSeed) a été SUPPRIMÉ du code ; les
// artefacts de démo hérités de l'ancien seed se retirent de la production
// via POST /api/admin/purge-demo (api/handlers_purge.go).
//
// Le mot de passe initial admin/admin123 est remplacé au boot par les
// identifiants de production via ADMIN_PASSWORD (applyAdminOverride).
func BuildEmptyState() *model.DB {
	now := time.Now().UTC()
	nowISO := now.Format(time.RFC3339)
	return &model.DB{
		Accounts:          []model.Account{},
		SettingsByAccount: map[string]model.Settings{},
		LastTick:          now,
		Users: []model.AdminUser{{
			ID:           "admin-1",
			AccountID:    "", // opérateur plateforme sans compte client
			Name:         "Administrateur",
			Username:     "admin",
			Role:         "admin",
			PasswordHash: auth.HashPassword("admin123", ""),
			CreatedAt:    nowISO,
		}},
		// Tout le reste est VIDE (slices non-nil pour que l'API serve [] et que
		// la synchro différentielle PG n'insère rien).
		Routers:        []model.Router{},
		Profiles:       []model.Profile{},
		HotspotUsers:   []model.HotspotUser{},
		Batches:        []model.Batch{},
		Resellers:      []model.Reseller{},
		Transactions:   []model.Transaction{},
		Sessions:       []model.Session{},
		Activity:       []model.Activity{},
		Sales:          []model.Sale{},
		Templates:      []model.VoucherTemplate{},
		UserLogs:       []model.UserLog{},
		IPBindings:     []model.IPBinding{},
		SchedulerTasks: []model.SchedulerTask{},
		Traffic:        []model.RouterTraffic{},
	}
}
