// Package api — console plateforme MikCloud (super-admin, multi-comptes).
//
// L'admin plateforme est le PROPRIÉTAIRE du SaaS, pas un client : ces
// endpoints lui donnent un cockpit d'opérateur — vue d'ensemble (KPIs
// globaux), création de comptes clients, journal d'activité transverse et
// gestion de l'équipe plateforme. Tous les endpoints exigent le rôle
// platform_admin (ou « admin » historique) via isPlatformAdmin.
//
// NB : la gestion d'abonnement fine (attribution/renouvellement de plan) et
// le drill-down complet d'un compte client restent des phases ultérieures —
// le statut d'abonnement est déjà exposé dans la liste des comptes.
package api

import (
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"mikcloud/hotspot-api/internal/auth"
	"mikcloud/hotspot-api/internal/model"
	"mikcloud/hotspot-api/internal/store"
)

// ---------------------------------------------------------------------------
// Vue d'ensemble plateforme — GET /api/admin/overview
// ---------------------------------------------------------------------------

// monthKey — clé « 2006-01 » d'un timestamp RFC3339 ; renvoie "" si invalide.
func monthKey(rfc3339 string) string {
	t, err := time.Parse(time.RFC3339, rfc3339)
	if err != nil {
		return ""
	}
	return t.UTC().Format("2006-01")
}

// handleAdminOverview — KPIs globaux du SaaS, toutes comptes confondus :
// comptes (total/actifs/nouveaux 30 j), routeurs (total/online), utilisateurs
// hotspot, sessions actives, ventes et revenus 30 j, santé des abonnements,
// croissance mensuelle (6 derniers mois) et top comptes par revenu 30 j.
func (a *API) handleAdminOverview(w http.ResponseWriter, r *http.Request) {
	if !isPlatformAdmin(r) {
		writeErr(w, http.StatusForbidden, "Réservé aux administrateurs de la plateforme")
		return
	}
	now := time.Now().UTC()
	cutoff30 := now.AddDate(0, 0, -30)

	a.store.Lock()
	db := a.store.Data()

	type AccountBrief struct {
		ID           string `json:"id"`
		Name         string `json:"name"`
		Status       string `json:"status"`
		Revenue30d   int    `json:"revenue30d"`
		Sales30d     int    `json:"sales30d"`
		Users        int    `json:"users"`
		Routers      int    `json:"routers"`
		Subscription string `json:"subscription"` // active | expired | beta
	}
	accounts := map[string]*AccountBrief{}
	for i := range db.Accounts {
		accounts[db.Accounts[i].ID] = &AccountBrief{
			ID:     db.Accounts[i].ID,
			Name:   db.Accounts[i].Name,
			Status: db.Accounts[i].Status,
		}
	}

	var routersTotal, routersOnline, hotspotUsers, sessionsActive int
	var sales30d, revenue30d int

	for i := range db.Routers {
		rr := &db.Routers[i]
		if b, ok := accounts[rr.AccountID]; ok {
			b.Routers++
		}
		routersTotal++
		if rr.Status == "online" {
			routersOnline++
		}
	}
	for i := range db.HotspotUsers {
		hotspotUsers++
		if b, ok := accounts[db.HotspotUsers[i].AccountID]; ok {
			b.Users++
		}
	}
	for range db.Sessions {
		sessionsActive++
	}
	for i := range db.Sales {
		s := &db.Sales[i]
		at, err := time.Parse(time.RFC3339, s.At)
		if err != nil || !at.After(cutoff30) {
			continue
		}
		sales30d++
		revenue30d += s.Amount
		if b, ok := accounts[s.AccountID]; ok {
			b.Sales30d++
			b.Revenue30d += s.Amount
		}
	}

	// Santé des abonnements + croissance mensuelle des comptes (6 mois).
	subActive, subExpired, subEssai := 0, 0, 0
	// Ancrage sur le 1er du mois : AddDate depuis le 31 d'un mois court
	// normaliserait (31 juin → 1er juillet) et dupliquerait des clés.
	monthAnchor := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	months := make([]string, 0, 6)
	accountsPerMonth := map[string]int{}
	for i := 5; i >= 0; i-- {
		months = append(months, monthAnchor.AddDate(0, -i, 0).Format("2006-01"))
	}
	newAccounts30d := 0
	for i := range db.Accounts {
		acc := &db.Accounts[i]
		accB := accounts[acc.ID]
		settings, ok := db.SettingsByAccount[acc.ID]
		if ok {
			switch subscriptionStatus(settings.Subscription, now) {
			case "active":
				subActive++
				accB.Subscription = "active"
			case "expired":
				subExpired++
				accB.Subscription = "expired"
			default:
				// « none » : aucun plan souscrit — essai / compte gratuit.
				subEssai++
				accB.Subscription = "essai"
			}
		} else {
			subEssai++
			accB.Subscription = "essai"
		}
		if mk := monthKey(acc.CreatedAt); mk != "" {
			if _, ok := accountsPerMonth[mk]; ok || contains(months, mk) {
				accountsPerMonth[mk]++
			}
			if at, err := time.Parse(time.RFC3339, acc.CreatedAt); err == nil && at.After(cutoff30) {
				newAccounts30d++
			}
		}
	}

	type GrowthPoint struct {
		Month    string `json:"month"` // « 2006-01 »
		Accounts int    `json:"accounts"`
	}
	growth := make([]GrowthPoint, 0, len(months))
	for _, m := range months {
		growth = append(growth, GrowthPoint{Month: m, Accounts: accountsPerMonth[m]})
	}

	// Top comptes par revenu 30 j (max 5), comptes actifs d'abord.
	top := make([]*AccountBrief, 0, len(accounts))
	for _, b := range accounts {
		top = append(top, b)
	}
	sort.Slice(top, func(i, j int) bool {
		if top[i].Revenue30d != top[j].Revenue30d {
			return top[i].Revenue30d > top[j].Revenue30d
		}
		return top[i].Name < top[j].Name
	})
	if len(top) > 5 {
		top = top[:5]
	}

	// Compteurs de statut (calculés sous verrou) + statut des inscriptions :
	// fermées si REGISTER_KEY est définie.
	totalAccounts := len(db.Accounts)
	activeAccounts, disabledAccounts := 0, 0
	for i := range db.Accounts {
		switch db.Accounts[i].Status {
		case "disabled":
			disabledAccounts++
		default:
			activeAccounts++
		}
	}
	// I (paramètres plateforme) — inscriptions : priorité env REGISTER_KEY
	// (filet de sécurité) > config DB du compte principal.
	registerOpen := true
	if strings.TrimSpace(os.Getenv("REGISTER_KEY")) != "" {
		registerOpen = false
	} else if cfg := ensureSettings(db, model.AccountMainID).Platform; cfg != nil {
		registerOpen = cfg.RegisterOpen
	}

	a.store.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"accounts": map[string]any{
			"total":    totalAccounts,
			"active":   activeAccounts,
			"disabled": disabledAccounts,
			"new30d":   newAccounts30d,
		},
		"routers":       map[string]any{"total": routersTotal, "online": routersOnline},
		"hotspotUsers":  hotspotUsers,
		"sessions":      sessionsActive,
		"sales30d":      sales30d,
		"revenue30d":    revenue30d,
		"subscriptions": map[string]any{"active": subActive, "expired": subExpired, "essai": subEssai},
		"growth":        growth,
		"topAccounts":   top,
		"registerOpen":  registerOpen,
	})
}

// contains — membership simple dans un slice de strings (petit volume : 6 mois).
func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Création d'un compte client — POST /api/admin/accounts
// ---------------------------------------------------------------------------

// handleAdminAccountCreate — crée un compte client complet depuis la console
// plateforme : compte + utilisateur owner (identifiants remis au client) +
// réglages par défaut + 3 gabarits de vouchers. L'auto-inscription publique
// reste possible séparément (POST /api/auth/register, clé d'invitation).
func (a *API) handleAdminAccountCreate(w http.ResponseWriter, r *http.Request) {
	if !isPlatformAdmin(r) {
		writeErr(w, http.StatusForbidden, "Réservé aux administrateurs de la plateforme")
		return
	}
	var req struct {
		Name     string `json:"name"`
		Username string `json:"username"`
		Password string `json:"password"`
		Email    string `json:"email"`
		Phone    string `json:"phone"`
		Country  string `json:"country"`
		City     string `json:"city"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	name := strings.TrimSpace(req.Name)
	username := strings.ToLower(strings.TrimSpace(req.Username))
	if name == "" {
		writeErr(w, http.StatusBadRequest, "Nom du compte requis")
		return
	}
	if len(name) > 80 {
		writeErr(w, http.StatusBadRequest, "Nom du compte trop long (80 caractères max)")
		return
	}
	if len(username) < 3 || len(username) > 32 {
		writeErr(w, http.StatusBadRequest, "Le nom d'utilisateur doit faire entre 3 et 32 caractères")
		return
	}
	for _, c := range username {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
			writeErr(w, http.StatusBadRequest, "Le nom d'utilisateur n'accepte que a-z, 0-9, tirets et tirets bas")
			return
		}
	}
	// Sécurité S2 — politique centralisée (10 caractères, denylist, ≠ username).
	if msg := passwordPolicyViolation(req.Password, username); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}

	a.store.Lock()
	db := a.store.Data()
	// Unicité GLOBALE des usernames console (toutes consoles confondues).
	for i := range db.Users {
		if strings.EqualFold(db.Users[i].Username, username) {
			a.store.Unlock()
			writeErr(w, http.StatusConflict, "Ce nom d'utilisateur est déjà pris")
			return
		}
	}
	// Sécurité S5 — même dédoublonnage email/WhatsApp que l'auto-inscription
	// (accountContactTaken, cf. handlers_auth.go) : la console plateforme ne
	// doit pas devenir un contournement du verrou anti-fermage. Pour recréer
	// le compte d'un client existant : supprimer l'ancien compte (zone
	// sensible) ou corriger son contact d'abord.
	emailNorm := strings.TrimSpace(req.Email)
	phoneNorm := digitsOnly.ReplaceAllString(req.Phone, "")
	if emailTaken, phoneTaken := accountContactTaken(db.Accounts, emailNorm, phoneNorm, ""); emailTaken || phoneTaken {
		a.store.Unlock()
		if emailTaken {
			writeErr(w, http.StatusConflict, "Un compte existe déjà avec cet email")
		} else {
			writeErr(w, http.StatusConflict, "Un compte existe déjà avec ce numéro WhatsApp")
		}
		return
	}
	acc := model.Account{
		ID:        model.NewID("acc-"),
		Name:      name,
		Status:    "active",
		CreatedAt: model.NowISO(),
		Email:     emailNorm,
		Phone:     phoneNorm,
		Country:   strings.ToLower(strings.TrimSpace(req.Country)),
		City:      strings.TrimSpace(req.City),
	}
	db.Accounts = append(db.Accounts, acc)
	u := model.AdminUser{
		ID:           model.NewID("usr-"),
		AccountID:    acc.ID,
		Name:         name,
		Username:     username,
		Role:         model.RoleOwner,
		PasswordHash: auth.HashPassword(req.Password, ""),
		CreatedAt:    model.NowISO(),
	}
	db.Users = append(db.Users, u)
	if db.SettingsByAccount == nil {
		db.SettingsByAccount = map[string]model.Settings{}
	}
	now := time.Now().UTC()
	db.SettingsByAccount[acc.ID] = model.Settings{
		Tenant: model.Tenant{
			Name: name, Currency: "XOF", Timezone: "Africa/Abidjan",
			ExpiryPolicyMode: "keep", ExpiryPolicyAfterDays: 30,
		},
		Plan: model.Plan{Name: "Essai", MaxRouters: "1", MaxUsers: "Illimité"},
		Subscription: model.Subscription{
			PlanID:      "essai",
			Status:      "active",
			PeriodStart: now.Format(time.RFC3339),
			PeriodEnd:   now.AddDate(0, 3, 0).Format(time.RFC3339),
			RouterSlots: 1,
		},
	}
	// Même contrat que l'auto-inscription : les 3 gabarits de vouchers par défaut.
	db.Templates = append(db.Templates, store.SeedTemplatesFor(acc.ID)...)
	// v2 — et le profil « Staff » par défaut.
	db.Profiles = append(db.Profiles, store.SeedProfilesFor(acc.ID)...)
	a.logActivityBy(r, db, "", "system", "Compte client créé par la plateforme : "+name+" («"+username+"»)")
	a.store.Save()
	a.store.Unlock()

	writeJSON(w, http.StatusCreated, map[string]any{
		"account": map[string]any{"id": acc.ID, "name": acc.Name, "status": acc.Status, "createdAt": acc.CreatedAt},
		"owner":   map[string]any{"username": u.Username, "role": u.Role},
	})
}

// ---------------------------------------------------------------------------
// Journal plateforme — GET /api/admin/activity
// ---------------------------------------------------------------------------

// handleAdminActivity — journal d'activité TRANSVERSE (tous comptes), pour le
// support et l'audit. Filtres : ?accountId=acc-xxx (un compte), ?limit=N
// (défaut 100, max 300). Chaque entrée porte le nom du compte concerné.
func (a *API) handleAdminActivity(w http.ResponseWriter, r *http.Request) {
	if !isPlatformAdmin(r) {
		writeErr(w, http.StatusForbidden, "Réservé aux administrateurs de la plateforme")
		return
	}
	accFilter := strings.TrimSpace(r.URL.Query().Get("accountId"))
	limit := 100
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 {
		limit = v
	}
	if limit > 300 {
		limit = 300
	}

	a.store.Lock()
	db := a.store.Data()
	names := map[string]string{}
	for i := range db.Accounts {
		names[db.Accounts[i].ID] = db.Accounts[i].Name
	}
	type Row struct {
		ID          string `json:"id"`
		AccountID   string `json:"accountId"`
		AccountName string `json:"accountName"`
		Type        string `json:"type"`
		Message     string `json:"message"`
		At          string `json:"at"`
		ActorID     string `json:"actorId,omitempty"`
		ActorName   string `json:"actorName,omitempty"`
	}
	rows := make([]Row, 0, limit)
	// Activité déjà triée par insertion antéchronologique côté store (les
	// entrées sont prépendues) — on filtre puis on borne.
	for i := range db.Activity {
		act := &db.Activity[i]
		if accFilter != "" && act.AccountID != accFilter {
			continue
		}
		rows = append(rows, Row{
			ID: act.ID, AccountID: act.AccountID, AccountName: names[act.AccountID],
			Type: act.Type, Message: act.Message, At: act.At,
			ActorID: act.ActorID, ActorName: act.ActorName,
		})
		if len(rows) >= limit {
			break
		}
	}
	a.store.Unlock()

	writeJSON(w, http.StatusOK, rows)
}

// ---------------------------------------------------------------------------
// Équipe plateforme — GET/POST /api/admin/team, DELETE /api/admin/team/{id}
// ---------------------------------------------------------------------------

// isPlatformUser — true pour les rôles super-admin plateforme (nouveau et
// historique).
func isPlatformUser(role string) bool {
	return role == model.RolePlatformAdmin || role == "admin"
}

// handlePlatformTeamList — membres de l'équipe plateforme (super-admins).
func (a *API) handlePlatformTeamList(w http.ResponseWriter, r *http.Request) {
	if !isPlatformAdmin(r) {
		writeErr(w, http.StatusForbidden, "Réservé aux administrateurs de la plateforme")
		return
	}
	type Row struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		Username  string `json:"username"`
		Role      string `json:"role"`
		CreatedAt string `json:"createdAt"`
		Self      bool   `json:"self"`
	}
	callerID := claimsFrom(r).Sub
	a.store.Lock()
	rows := make([]Row, 0, 4)
	for i := range a.store.Data().Users {
		u := &a.store.Data().Users[i]
		if !isPlatformUser(u.Role) {
			continue
		}
		rows = append(rows, Row{ID: u.ID, Name: u.Name, Username: u.Username, Role: u.Role, CreatedAt: u.CreatedAt, Self: u.ID == callerID})
	}
	a.store.Unlock()
	writeJSON(w, http.StatusOK, rows)
}

// handlePlatformTeamCreate — crée un autre super-admin plateforme (redondance
// de l'équipe opérationnelle : plus de single point of failure). Un
// platform_admin ne peut JAMAIS être créé via les endpoints « équipe » d'un
// compte client (voir handlers_team.go) — uniquement ici, par la plateforme.
func (a *API) handlePlatformTeamCreate(w http.ResponseWriter, r *http.Request) {
	if !isPlatformAdmin(r) {
		writeErr(w, http.StatusForbidden, "Réservé aux administrateurs de la plateforme")
		return
	}
	var req struct {
		Name     string `json:"name"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	name := strings.TrimSpace(req.Name)
	username := strings.ToLower(strings.TrimSpace(req.Username))
	if name == "" {
		name = username
	}
	if len(username) < 3 || len(username) > 32 {
		writeErr(w, http.StatusBadRequest, "Le nom d'utilisateur doit faire entre 3 et 32 caractères")
		return
	}
	for _, c := range username {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
			writeErr(w, http.StatusBadRequest, "Le nom d'utilisateur n'accepte que a-z, 0-9, tirets et tirets bas")
			return
		}
	}
	// Sécurité S2 — politique centralisée (10 caractères, denylist, ≠ username).
	if msg := passwordPolicyViolation(req.Password, username); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}

	a.store.Lock()
	db := a.store.Data()
	for i := range db.Users {
		if strings.EqualFold(db.Users[i].Username, username) {
			a.store.Unlock()
			writeErr(w, http.StatusConflict, "Ce nom d'utilisateur est déjà pris")
			return
		}
	}
	u := model.AdminUser{
		ID:           model.NewID("usr-"),
		AccountID:    "", // l'admin plateforme est un opérateur SANS compte client
		Name:         name,
		Username:     username,
		Role:         model.RolePlatformAdmin,
		PasswordHash: auth.HashPassword(req.Password, ""),
		CreatedAt:    model.NowISO(),
	}
	db.Users = append(db.Users, u)
	a.logActivityBy(r, db, "", "team", "Administrateur plateforme ajouté : "+name+" («"+username+"»)")
	a.store.Save()
	a.store.Unlock()

	writeJSON(w, http.StatusCreated, map[string]any{
		"id": u.ID, "name": u.Name, "username": u.Username, "role": u.Role, "createdAt": u.CreatedAt,
	})
}

// handlePlatformTeamDelete — retire un super-admin plateforme. Garde-fous :
// impossible de se retirer soi-même, impossible de retirer le DERNIER
// platform_admin (sinon plus personne ne pilote le SaaS).
func (a *API) handlePlatformTeamDelete(w http.ResponseWriter, r *http.Request) {
	if !isPlatformAdmin(r) {
		writeErr(w, http.StatusForbidden, "Réservé aux administrateurs de la plateforme")
		return
	}
	id := r.PathValue("id")
	callerID := claimsFrom(r).Sub

	a.store.Lock()
	db := a.store.Data()
	idx := -1
	for i := range db.Users {
		if db.Users[i].ID == id && isPlatformUser(db.Users[i].Role) {
			idx = i
			break
		}
	}
	if idx < 0 {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Administrateur plateforme introuvable")
		return
	}
	if db.Users[idx].ID == callerID {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, "Impossible de retirer votre propre compte")
		return
	}
	name := db.Users[idx].Name
	remaining := 0
	for i := range db.Users {
		if i != idx && isPlatformUser(db.Users[i].Role) {
			remaining++
		}
	}
	if remaining == 0 {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, "Impossible de retirer le dernier administrateur plateforme")
		return
	}
	db.Users = append(db.Users[:idx], db.Users[idx+1:]...)
	a.logActivityBy(r, db, "", "team", "Administrateur plateforme retiré : "+name)
	a.store.Save()
	a.store.Unlock()

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleWipe — POST /api/admin/wipe : compatibilité — purge TOTALE déléguée
// au moteur de purge (scope « all », portée globale, voir handlers_purge.go).
// Les données métier sont supprimées sans toucher aux comptes, à l'équipe,
// aux réglages ni aux routeurs réels (agent/real) ; rien n'est régénéré.
func (a *API) handleWipe(w http.ResponseWriter, r *http.Request) {
	a.purgeScopes(w, r, "", nil) // accID vide → portée globale ; nil → scope « all »
}

// handleReload — POST /api/admin/reload : réimporte l'état complet depuis la
// base persistée (Neon en production) sans redémarrer le service. Réservé à
// l'admin plateforme. Cas d'usage : après une modification SQL directe de la
// base, ou pour appliquer immédiatement un changement de ADMIN_PASSWORD.
func (a *API) handleReload(w http.ResponseWriter, r *http.Request) {
	if !isPlatformAdmin(r) {
		writeErr(w, http.StatusForbidden, "Réservé aux administrateurs de la plateforme")
		return
	}
	stats, err := a.store.Reload()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Rechargement impossible : "+err.Error())
		return
	}
	a.store.Lock()
	a.logActivityBy(r, a.store.Data(), accountScope(r), "system", "Données rechargées depuis la base persistée")
	a.store.Save()
	a.store.Unlock()
	// Les connexions routeurs en cache peuvent référencer des routeurs
	// modifiés ou supprimés directement en base : on les réinitialise.
	a.clearGateways()
	writeJSON(w, http.StatusOK, stats)
}

// ---------------------------------------------------------------------------
// Administration plateforme (rôle admin) — gestion des comptes SaaS
// ---------------------------------------------------------------------------
