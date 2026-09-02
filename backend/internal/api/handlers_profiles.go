// handlers_profiles.go — gestion des profils de vouchers (CRUD).

package api

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"mikcloud/hotspot-api/internal/agent"
	"mikcloud/hotspot-api/internal/model"
)

func (a *API) handleProfilesList(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	a.store.Lock()
	ps := []model.Profile{}
	for _, p := range a.store.Data().Profiles {
		if p.AccountID == acc {
			ps = append(ps, p)
		}
	}
	a.store.Unlock()
	sort.Slice(ps, func(i, j int) bool { return ps[i].CreatedAt > ps[j].CreatedAt })
	writeJSON(w, http.StatusOK, ps)
}

func (a *API) handleProfileCreate(w http.ResponseWriter, r *http.Request) {
	// P3 — compte expiré : écritures métier refusées (lecture seule).
	if !a.guardAccountWrite(w, r) {
		return
	}
	acc := accountScope(r)
	var req struct {
		Name              string `json:"name"`
		RateLimit         string `json:"rateLimit"`
		SessionTimeoutMin int    `json:"sessionTimeoutMin"`
		SharedUsers       int    `json:"sharedUsers"`
		ValidityDays      int    `json:"validityDays"`
		Price             int    `json:"price"`
		DataQuotaMb       int    `json:"dataQuotaMb"`
		// P0/P2 (audit Mikhmon) — expiration cloud (F1) + marge (F13).
		ExpMode        string `json:"expMode"`
		GracePeriodMin int    `json:"gracePeriodMin"`
		LockUser       bool   `json:"lockUser"`
		SellingPrice   int    `json:"sellingPrice"`
		// v2 — verrou « 1er appareil » (liaison MAC par on-login).
		LockFirstDevice bool `json:"lockFirstDevice"`
		// Parité Mikhmon : address-pool / parent-queue RouterOS + validité fine.
		AddressPool string `json:"addressPool"`
		ParentQueue string `json:"parentQueue"`
		ValidityMin int    `json:"validityMin"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeErr(w, http.StatusBadRequest, "Nom du profil requis")
		return
	}
	// Bornes contrat F1/F13.
	expMode := strings.TrimSpace(req.ExpMode)
	if expMode == "" {
		expMode = "notify"
	}
	if expMode != "none" && expMode != "notify" && expMode != "remove" {
		writeErr(w, http.StatusBadRequest, "Mode d'expiration invalide (none, notify ou remove)")
		return
	}
	if req.GracePeriodMin < 0 || req.GracePeriodMin > 43200 {
		writeErr(w, http.StatusBadRequest, "La période de grâce doit être comprise entre 0 et 43200 minutes")
		return
	}
	if req.SellingPrice < 0 {
		writeErr(w, http.StatusBadRequest, "Le prix de vente doit être positif")
		return
	}
	// Parité Mikhmon : validité fine en minutes (borne 5 ans) ; 0 = hériter
	// de validityDays (compatibilité contrat V2 / données existantes).
	if req.ValidityMin < 0 || req.ValidityMin > 2628000 {
		writeErr(w, http.StatusBadRequest, "La validité doit être comprise entre 0 et 2628000 minutes")
		return
	}
	// Sécurité P0 — plafond économique (cf. prixMaxProfil) : refuse plutôt
	// que de clamp silencieusement, pour que l'erreur soit visible en console.
	if req.Price > prixMaxProfil || req.SellingPrice > prixMaxProfil {
		writeErr(w, http.StatusBadRequest, fmt.Sprintf("Le prix ne peut pas dépasser %d FCFA", prixMaxProfil))
		return
	}
	a.store.Lock()
	for _, p := range a.store.Data().Profiles {
		if p.AccountID == acc && strings.EqualFold(p.Name, name) {
			a.store.Unlock()
			writeErr(w, http.StatusBadRequest, "Ce profil existe déjà")
			return
		}
	}
	a.store.Unlock()
	rateLimit := strings.TrimSpace(req.RateLimit)
	if rateLimit == "" {
		rateLimit = "1M/1M"
	}
	profile := model.Profile{
		ID: model.NewID("p-"), AccountID: acc, Name: name, RateLimit: rateLimit,
		SessionTimeoutMin: defaultPositive(req.SessionTimeoutMin, 60),
		SharedUsers:       defaultPositive(req.SharedUsers, 1),
		ValidityDays:      defaultPositive(req.ValidityDays, 1),
		Price:             defaultMinZero(req.Price),
		DataQuotaMb:       defaultMinZero(req.DataQuotaMb),
		CreatedAt:         model.NowISO(),
		ExpMode:           expMode, GracePeriodMin: req.GracePeriodMin,
		LockUser: req.LockUser, SellingPrice: defaultMinZero(req.SellingPrice),
		LockFirstDevice: req.LockFirstDevice,
		AddressPool:     strings.TrimSpace(req.AddressPool),
		ParentQueue:     strings.TrimSpace(req.ParentQueue),
		ValidityMin:     defaultMinZero(req.ValidityMin),
	}
	// Parité Mikhmon : la validité fine est la source de vérité ; le champ
	// historique validityDays reste cohérent (arrondi supérieur, contrat V2).
	if profile.ValidityMin > 0 {
		profile.ValidityDays = (profile.ValidityMin + 1439) / 1440
	}
	a.store.Lock()
	db := a.store.Data()
	db.Profiles = append(db.Profiles, profile)
	// Synchroniser le profil vers TOUS les routeurs agents dès la création :
	// si un profil du même nom existe déjà sur un routeur (Winbox/import), le
	// set aligne session-timeout/rate-limit/shared-users/verrou sur le cloud —
	// sans quoi les vouchers liés gardaient les ANCIENS paramètres du routeur.
	a.queueProfileSetLocked(db, acc, profile)
	a.logActivityBy(r, db, acc, "user", "Profil "+profile.Name+" créé")
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusCreated, profile)
}

func (a *API) handleProfileUpdate(w http.ResponseWriter, r *http.Request) {
	// P3 — compte expiré : écritures métier refusées (lecture seule).
	if !a.guardAccountWrite(w, r) {
		return
	}
	acc := accountScope(r)
	id := r.PathValue("id")
	var req struct {
		Name              *string `json:"name"`
		RateLimit         *string `json:"rateLimit"`
		SessionTimeoutMin *int    `json:"sessionTimeoutMin"`
		SharedUsers       *int    `json:"sharedUsers"`
		ValidityDays      *int    `json:"validityDays"`
		Price             *int    `json:"price"`
		DataQuotaMb       *int    `json:"dataQuotaMb"`
		// P0/P2 (audit Mikhmon) — expiration cloud (F1) + marge (F13).
		ExpMode        *string `json:"expMode"`
		GracePeriodMin *int    `json:"gracePeriodMin"`
		LockUser       *bool   `json:"lockUser"`
		SellingPrice   *int    `json:"sellingPrice"`
		// v2 — verrou « 1er appareil » (liaison MAC par on-login).
		LockFirstDevice *bool `json:"lockFirstDevice"`
		// Parité Mikhmon : address-pool / parent-queue RouterOS + validité fine.
		AddressPool *string `json:"addressPool"`
		ParentQueue *string `json:"parentQueue"`
		ValidityMin *int    `json:"validityMin"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	// Bornes contrat F1/F13 (validées avant verrou).
	if req.ExpMode != nil {
		m := strings.TrimSpace(*req.ExpMode)
		if m != "" && m != "none" && m != "notify" && m != "remove" {
			writeErr(w, http.StatusBadRequest, "Mode d'expiration invalide (none, notify ou remove)")
			return
		}
	}
	if req.GracePeriodMin != nil && (*req.GracePeriodMin < 0 || *req.GracePeriodMin > 43200) {
		writeErr(w, http.StatusBadRequest, "La période de grâce doit être comprise entre 0 et 43200 minutes")
		return
	}
	if req.SellingPrice != nil && *req.SellingPrice < 0 {
		writeErr(w, http.StatusBadRequest, "Le prix de vente doit être positif")
		return
	}
	// Sécurité P0 — plafond économique (cf. handleProfileCreate).
	if (req.Price != nil && *req.Price > prixMaxProfil) || (req.SellingPrice != nil && *req.SellingPrice > prixMaxProfil) {
		writeErr(w, http.StatusBadRequest, fmt.Sprintf("Le prix ne peut pas dépasser %d FCFA", prixMaxProfil))
		return
	}
	// Parité Mikhmon : validité fine en minutes (borne 5 ans).
	if req.ValidityMin != nil && (*req.ValidityMin < 0 || *req.ValidityMin > 2628000) {
		writeErr(w, http.StatusBadRequest, "La validité doit être comprise entre 0 et 2628000 minutes")
		return
	}
	a.store.Lock()
	db := a.store.Data()
	p := findProfileScoped(db, id, acc)
	if p == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Profil introuvable")
		return
	}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			a.store.Unlock()
			writeErr(w, http.StatusBadRequest, "Nom du profil requis")
			return
		}
		for _, other := range db.Profiles {
			if other.ID != id && other.AccountID == acc && strings.EqualFold(other.Name, name) {
				a.store.Unlock()
				writeErr(w, http.StatusBadRequest, "Ce profil existe déjà")
				return
			}
		}
		p.Name = name
	}
	if req.RateLimit != nil && strings.TrimSpace(*req.RateLimit) != "" {
		p.RateLimit = strings.TrimSpace(*req.RateLimit)
	}
	if req.SessionTimeoutMin != nil && *req.SessionTimeoutMin > 0 {
		p.SessionTimeoutMin = *req.SessionTimeoutMin
	}
	if req.SharedUsers != nil && *req.SharedUsers > 0 {
		p.SharedUsers = *req.SharedUsers
	}
	if req.ValidityDays != nil && *req.ValidityDays > 0 {
		p.ValidityDays = *req.ValidityDays
	}
	if req.ValidityMin != nil && *req.ValidityMin >= 0 {
		p.ValidityMin = *req.ValidityMin
		if p.ValidityMin > 0 {
			p.ValidityDays = (p.ValidityMin + 1439) / 1440
		}
	}
	if req.AddressPool != nil {
		p.AddressPool = strings.TrimSpace(*req.AddressPool)
	}
	if req.ParentQueue != nil {
		p.ParentQueue = strings.TrimSpace(*req.ParentQueue)
	}
	if req.Price != nil && *req.Price >= 0 {
		p.Price = *req.Price
	}
	if req.DataQuotaMb != nil && *req.DataQuotaMb >= 0 {
		p.DataQuotaMb = *req.DataQuotaMb
	}
	// P0/P2 (audit Mikhmon).
	if req.ExpMode != nil && strings.TrimSpace(*req.ExpMode) != "" {
		p.ExpMode = strings.TrimSpace(*req.ExpMode)
	}
	if req.GracePeriodMin != nil {
		p.GracePeriodMin = *req.GracePeriodMin
	}
	if req.LockUser != nil {
		p.LockUser = *req.LockUser
	}
	// v2 — verrou « 1er appareil » : un changement d'état est propagé aux
	// routeurs agents (on-login de liaison MAC appliqué ou retiré).
	lockChanged := false
	if req.LockFirstDevice != nil && *req.LockFirstDevice != p.LockFirstDevice {
		p.LockFirstDevice = *req.LockFirstDevice
		lockChanged = true
	}
	updated := *p
	// Toute modification (session-timeout, rate-limit, shared-users, verrou…)
	// est propagée aux routeurs agents : le cloud est la source de vérité.
	a.queueProfileSetLocked(db, acc, updated)
	if lockChanged {
		state := "désactivé"
		if updated.LockFirstDevice {
			state = "activé"
		}
		a.logActivityBy(r, db, acc, "user", "Verrou « 1er appareil » "+state+" pour le profil "+updated.Name)
	} else {
		a.logActivityBy(r, db, acc, "user", "Profil "+updated.Name+" modifié (synchronisé vers le routeur)")
	}
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, updated)
}

// queueProfileSetLocked — propage l'état COMPLET d'un profil vers TOUS les
// routeurs en mode agent du compte (une commande profile_set par routeur :
// session-timeout, rate-limit, shared-users et verrou « 1er appareil » via
// on-login). Les modes real/simulated n'exposent pas les profils hotspot via
// la gateway : la synchro y est sans effet (mode agent requis).
func (a *API) queueProfileSetLocked(db *model.DB, acc string, p model.Profile) {
	name := agent.SanitizeName(p.Name)
	for i := range db.Routers {
		r := &db.Routers[i]
		if r.AccountID == acc && r.Mode == "agent" {
			queueCommandLocked(db, acc, r.ID, model.CmdProfileSet, map[string]any{
				"name":              name,
				"rateLimit":         p.RateLimit,
				"sessionTimeoutMin": p.SessionTimeoutMin,
				"sharedUsers":       p.SharedUsers,
				"lockFirstDevice":   p.LockFirstDevice,
				"addressPool":       p.AddressPool,
				"parentQueue":       p.ParentQueue,
			})
		}
	}
}

func (a *API) handleProfileDelete(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	id := r.PathValue("id")
	a.store.Lock()
	db := a.store.Data()
	idx := -1
	for i := range db.Profiles {
		if db.Profiles[i].ID == id && db.Profiles[i].AccountID == acc {
			idx = i
			break
		}
	}
	if idx < 0 {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Profil introuvable")
		return
	}
	name := db.Profiles[idx].Name
	db.Profiles = append(db.Profiles[:idx], db.Profiles[idx+1:]...)
	a.logActivityBy(r, db, acc, "user", "Profil "+name+" supprimé")
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
