// handlers_settings.go — réglages du compte (tenant).

package api

import (
	"net/http"
	"strings"
)

func (a *API) handleSettingsGet(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	a.store.Lock()
	settings := ensureSettings(a.store.Data(), acc)
	a.store.Unlock()
	writeJSON(w, http.StatusOK, settings)
}

// tenantPut — forme imbriquée « tenant { … } » de PUT /api/settings. Le
// frontend P0 envoie les nouveaux champs À LA FOIS plats et imbriqués : les
// deux formes sont acceptées (le plat prime, le nested sert de repli).
type tenantPut struct {
	Name                  *string `json:"name"`
	Currency              *string `json:"currency"`
	Timezone              *string `json:"timezone"`
	WaveLink              *string `json:"waveLink"`
	DNSName               *string `json:"dnsName"`
	LogoURL               *string `json:"logoUrl"`
	ExpiryPolicyMode      *string `json:"expiryPolicyMode"`
	ExpiryPolicyAfterDays *int    `json:"expiryPolicyAfterDays"`
}

func (a *API) handleSettingsPut(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	var req struct {
		Name     *string `json:"name"`
		Currency *string `json:"currency"`
		Timezone *string `json:"timezone"`
		WaveLink *string `json:"waveLink"`
		// P0 (audit Mikhmon) — champs plats…
		DNSName               *string `json:"dnsName"`
		LogoURL               *string `json:"logoUrl"`
		ExpiryPolicyMode      *string `json:"expiryPolicyMode"`
		ExpiryPolicyAfterDays *int    `json:"expiryPolicyAfterDays"`
		// …et forme imbriquée tenant{…}.
		Tenant *tenantPut `json:"tenant"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	// Résolution plat > imbriqué : le corps défensif du front P0 envoie les
	// nouveaux champs À LA FOIS plats et dans tenant{…} — le plat prime.
	name, currency, timezone, waveLink := req.Name, req.Currency, req.Timezone, req.WaveLink
	dnsName, logoURL, expiryMode, expiryAfterDays := req.DNSName, req.LogoURL, req.ExpiryPolicyMode, req.ExpiryPolicyAfterDays
	if req.Tenant != nil {
		if name == nil {
			name = req.Tenant.Name
		}
		if currency == nil {
			currency = req.Tenant.Currency
		}
		if timezone == nil {
			timezone = req.Tenant.Timezone
		}
		if waveLink == nil {
			waveLink = req.Tenant.WaveLink
		}
		if dnsName == nil {
			dnsName = req.Tenant.DNSName
		}
		if logoURL == nil {
			logoURL = req.Tenant.LogoURL
		}
		if expiryMode == nil {
			expiryMode = req.Tenant.ExpiryPolicyMode
		}
		if expiryAfterDays == nil {
			expiryAfterDays = req.Tenant.ExpiryPolicyAfterDays
		}
	}

	// Validations P0 (contrat F2/F5).
	if dnsName != nil && len(*dnsName) > 100 {
		writeErr(w, http.StatusBadRequest, "Le nom DNS doit faire au plus 100 caractères")
		return
	}
	if logoURL != nil && *logoURL != "" {
		if !strings.HasPrefix(*logoURL, "data:image/") {
			writeErr(w, http.StatusBadRequest, "Logo invalide : image intégrée (data:image/…) requise")
			return
		}
		if len(*logoURL) > 300*1024 {
			writeErr(w, http.StatusBadRequest, "Logo trop volumineux (300 Ko max)")
			return
		}
	}
	if expiryMode != nil && *expiryMode != "keep" && *expiryMode != "remove" {
		writeErr(w, http.StatusBadRequest, "Politique d'expiration invalide (keep ou remove)")
		return
	}
	if expiryAfterDays != nil && (*expiryAfterDays < 0 || *expiryAfterDays > 365) {
		writeErr(w, http.StatusBadRequest, "Le nombre de jours doit être compris entre 0 et 365")
		return
	}

	a.store.Lock()
	db := a.store.Data()
	settings := ensureSettings(db, acc) // créés avec les défauts FCFA si absents
	if name != nil && strings.TrimSpace(*name) != "" {
		settings.Tenant.Name = strings.TrimSpace(*name)
	}
	if currency != nil && strings.TrimSpace(*currency) != "" {
		settings.Tenant.Currency = strings.TrimSpace(*currency)
	}
	if timezone != nil && strings.TrimSpace(*timezone) != "" {
		settings.Tenant.Timezone = strings.TrimSpace(*timezone)
	}
	if waveLink != nil {
		settings.Tenant.WaveLink = strings.TrimSpace(*waveLink) // vide = désactivé
	}
	// P0 (audit Mikhmon).
	if dnsName != nil {
		settings.Tenant.DNSName = strings.TrimSpace(*dnsName)
	}
	if logoURL != nil {
		settings.Tenant.LogoURL = strings.TrimSpace(*logoURL) // vide = logo retiré
	}
	if expiryMode != nil {
		settings.Tenant.ExpiryPolicyMode = *expiryMode
	}
	if expiryAfterDays != nil {
		settings.Tenant.ExpiryPolicyAfterDays = *expiryAfterDays
	}
	db.SettingsByAccount[acc] = settings
	a.logActivityBy(r, db, acc, "system", "Paramètres du tenant mis à jour")
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, settings)
}
