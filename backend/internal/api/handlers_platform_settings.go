// Package api — paramètres de la PLATEFORME (console plateforme, vue
// « Paramètres plateforme »). La config vit sur les settings du compte
// principal (model.AccountMainID → Settings.Platform) et est persistée en
// PostgreSQL : plus besoin de redéployer Render pour ouvrir/fermer les
// inscriptions ou changer le nom affiché du SaaS.
//
// Sécurité : toutes les routes exigent le rôle admin plateforme
// (isPlatformAdmin), vérifié dans chaque handler.

package api

import (
	"net/http"
	"os"
	"strings"

	"mikcloud/hotspot-api/internal/model"
)

// handlePlatformSettingsGet — GET /api/admin/platform/settings :
// renvoie la config plateforme (nom, inscriptions) + la source effective
// du contrôle des inscriptions (env REGISTER_KEY prioritaire sur la DB).
func (a *API) handlePlatformSettingsGet(w http.ResponseWriter, r *http.Request) {
	if !isPlatformAdmin(r) {
		writeErr(w, http.StatusForbidden, "Réservé aux administrateurs de la plateforme")
		return
	}
	a.store.Lock()
	db := a.store.Data()
	settings := ensureSettings(db, model.AccountMainID)
	cfg := settings.Platform
	if cfg == nil {
		cfg = &model.PlatformConfig{Name: "MikCloud", RegisterOpen: true}
	}
	// Une env REGISTER_KEY définie PREND LE DESSUS sur la config DB (filet de
	// sécurité pour verrouiller les inscriptions sans accès à la console).
	envKey := strings.TrimSpace(os.Getenv("REGISTER_KEY"))
	source := "database"
	if envKey != "" {
		source = "env"
	}
	a.store.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"platform": map[string]any{
			"name":         cfg.Name,
			"registerOpen": cfg.RegisterOpen,
			// La clé n'est JAMAIS renvoyée en clair — seulement un indicateur
			// de présence (l'admin la définit, il ne la relit pas).
			"registerKeySet": cfg.RegisterKey != "",
		},
		"registerSource": source,
	})
}

// handlePlatformSettingsPut — PUT /api/admin/platform/settings :
// met à jour le nom affiché et/ou la politique d'inscription.
// Corps : {"name": "MikCloud", "registerOpen": true, "registerKey": "…"}.
// registerKey = "" SUPPRIME la clé (inscriptions totalement fermées si
// registerOpen = false, sinon ouvertes).
func (a *API) handlePlatformSettingsPut(w http.ResponseWriter, r *http.Request) {
	if !isPlatformAdmin(r) {
		writeErr(w, http.StatusForbidden, "Réservé aux administrateurs de la plateforme")
		return
	}
	var req struct {
		Name         string  `json:"name"`
		RegisterOpen *bool   `json:"registerOpen"`
		RegisterKey  *string `json:"registerKey"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}

	// Filet de sécurité : si l'env REGISTER_KEY est définie, elle verrouille
	// les inscriptions — la console ne peut pas les rouvrir (retirer la
	// variable d'environnement pour rendre la main à la DB).
	if envKey := strings.TrimSpace(os.Getenv("REGISTER_KEY")); envKey != "" && req.RegisterOpen != nil && *req.RegisterOpen {
		writeErr(w, http.StatusConflict, "La variable d'environnement REGISTER_KEY est définie — retirez-la pour piloter les inscriptions depuis la console")
		return
	}

	a.store.Lock()
	db := a.store.Data()
	settings := ensureSettings(db, model.AccountMainID)
	cfg := settings.Platform
	if cfg == nil {
		cfg = &model.PlatformConfig{Name: "MikCloud", RegisterOpen: true}
	}
	name := strings.TrimSpace(req.Name)
	if name != "" {
		if len(name) > 60 {
			a.store.Unlock()
			writeErr(w, http.StatusBadRequest, "Le nom de la plateforme doit faire 60 caractères maximum")
			return
		}
		cfg.Name = name
	}
	if req.RegisterOpen != nil {
		cfg.RegisterOpen = *req.RegisterOpen
	}
	if req.RegisterKey != nil {
		key := strings.TrimSpace(*req.RegisterKey)
		if len(key) > 64 {
			a.store.Unlock()
			writeErr(w, http.StatusBadRequest, "La clé d'invitation doit faire 64 caractères maximum")
			return
		}
		cfg.RegisterKey = key
	}
	settings.Platform = cfg
	db.SettingsByAccount[model.AccountMainID] = settings
	a.logActivityBy(r, db, model.AccountMainID, "system", "Paramètres plateforme mis à jour")
	a.store.Save()
	a.store.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"platform": map[string]any{
			"name":           cfg.Name,
			"registerOpen":   cfg.RegisterOpen,
			"registerKeySet": cfg.RegisterKey != "",
		},
	})
}

// registerGate — contrôle d'inscription EFFECTIF (handleRegister) :
// priorité env REGISTER_KEY > config DB du compte principal.
// Retourne (ouvert, clé attendue).
func (a *API) registerGate() (bool, string) {
	// 1. Env var : filet de sécurité historique (verrouillage dur).
	if envKey := strings.TrimSpace(os.Getenv("REGISTER_KEY")); envKey != "" {
		return false, envKey
	}
	// 2. Config DB (pilotable depuis la console plateforme).
	a.store.Lock()
	cfg := ensureSettings(a.store.Data(), model.AccountMainID).Platform
	a.store.Unlock()
	if cfg != nil {
		if cfg.RegisterOpen {
			return true, ""
		}
		return false, cfg.RegisterKey
	}
	// 3. Défaut historique : ouvert.
	return true, ""
}
