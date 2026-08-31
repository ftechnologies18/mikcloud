// Package store — état initial de mise en service (base vide).
package store

import (
	"time"

	"mikcloud/hotspot-api/internal/auth"
	"mikcloud/hotspot-api/internal/model"
)

// BuildEmptyState construit l'état initial de MISE EN SERVICE : compte
// principal + administrateur + réglages FCFA + 3 gabarits de tickets prêts à
// l'emploi — et AUCUNE donnée de démonstration (zéro routeur, zéro profil,
// zéro utilisateur hotspot, zéro lot, zéro revendeur, zéro vente).
//
// Utilisé au démarrage d'une base PostgreSQL vide (production Render/Neon) :
// le système démarre vide, prêt pour un vrai routeur. Les données de démo ne
// reviennent JAMAIS d'elles-mêmes — le seed démo (BuildSeed) est désormais
// réservé au mode développement JSON local.
//
// Le mot de passe initial admin/admin123 est remplacé au boot par les
// identifiants de production via ADMIN_PASSWORD (applyAdminOverride).
func BuildEmptyState() *model.DB {
	now := time.Now().UTC()
	nowISO := now.Format(time.RFC3339)
	return &model.DB{
		Accounts: []model.Account{{
			ID:        model.AccountMainID,
			Name:      "MikCloud",
			Status:    "active",
			CreatedAt: nowISO,
		}},
		SettingsByAccount: map[string]model.Settings{
			model.AccountMainID: {
				Tenant: model.Tenant{
					Name: "MikCloud", Currency: "FCFA", Timezone: "Africa/Abidjan",
					ExpiryPolicyMode: "keep", ExpiryPolicyAfterDays: 30,
				},
				Plan: model.Plan{Name: "Bêta", MaxRouters: "Illimité", MaxUsers: "Illimité"},
			},
		},
		LastTick: now,
		Users: []model.AdminUser{{
			ID:           "admin-1",
			AccountID:    model.AccountMainID,
			Name:         "Administrateur",
			Username:     "admin",
			Role:         "admin",
			PasswordHash: auth.HashPassword("admin123", ""),
			CreatedAt:    nowISO,
		}},
		// Gabarits de tickets prêts à l'emploi — le reste est VIDE (slices
		// non-nil pour que l'API serve [] et que la synchro différentielle PG
		// n'insère rien).
		Routers:        []model.Router{},
		Profiles:       []model.Profile{},
		HotspotUsers:   []model.HotspotUser{},
		Batches:        []model.Batch{},
		Resellers:      []model.Reseller{},
		Transactions:   []model.Transaction{},
		Sessions:       []model.Session{},
		Activity:       []model.Activity{},
		Sales:          []model.Sale{},
		Templates:      SeedTemplatesFor(model.AccountMainID),
		UserLogs:       []model.UserLog{},
		IPBindings:     []model.IPBinding{},
		SchedulerTasks: []model.SchedulerTask{},
		Traffic:        []model.RouterTraffic{},
	}
}
