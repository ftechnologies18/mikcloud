// Package api — tous les handlers HTTP du service hotspot-api (contrat du worklog).
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"mikcloud/hotspot-api/internal/agent"
	"mikcloud/hotspot-api/internal/auth"
	"mikcloud/hotspot-api/internal/model"
	"mikcloud/hotspot-api/internal/routeros"
	"mikcloud/hotspot-api/internal/store"
)

// API — registre des routes + dépendances.
type API struct {
	store  *store.Store
	secret string
	gwMu   sync.Mutex
	gws    map[string]routeros.Gateway
}

// New construit l'API.
func New(s *store.Store, jwtSecret string) *API {
	return &API{store: s, secret: jwtSecret, gws: map[string]routeros.Gateway{}}
}

// Handler — mux complet, protégé par le middleware d'authentification.
func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /{$}", a.handleHealth)

	// Agent MikCloud (routeur -> cloud, HTTP-poll sortant) + provisionning console
	a.registerAgentRoutes(mux)

	// Notifications (réglages canaux, test, historique)
	a.registerNotifRoutes(mux)

	// Auth
	mux.HandleFunc("POST /api/auth/login", a.handleLogin)
	mux.HandleFunc("POST /api/auth/register", a.handleRegister)
	mux.HandleFunc("GET /api/auth/me", a.handleMe)
	mux.HandleFunc("POST /api/auth/password", a.handlePasswordChange)

	// Dashboard
	mux.HandleFunc("GET /api/dashboard", a.handleDashboard)

	// Routeurs
	mux.HandleFunc("GET /api/routers", a.handleRoutersList)
	mux.HandleFunc("POST /api/routers", a.handleRouterCreate)
	mux.HandleFunc("PUT /api/routers/{id}", a.handleRouterUpdate)
	mux.HandleFunc("DELETE /api/routers/{id}", a.handleRouterDelete)
	mux.HandleFunc("POST /api/routers/{id}/test", a.handleRouterTest)
	mux.HandleFunc("GET /api/routers/{id}/stats", a.handleRouterStats)

	// Profils
	mux.HandleFunc("GET /api/profiles", a.handleProfilesList)
	mux.HandleFunc("POST /api/profiles", a.handleProfileCreate)
	mux.HandleFunc("PUT /api/profiles/{id}", a.handleProfileUpdate)
	mux.HandleFunc("DELETE /api/profiles/{id}", a.handleProfileDelete)

	// Utilisateurs hotspot
	mux.HandleFunc("GET /api/users", a.handleUsersList)
	mux.HandleFunc("POST /api/users", a.handleUserCreate)
	mux.HandleFunc("PUT /api/users/{id}", a.handleUserUpdate)
	mux.HandleFunc("DELETE /api/users/{id}", a.handleUserDelete)
	mux.HandleFunc("POST /api/users/{id}/enable", a.handleUserEnable)
	mux.HandleFunc("POST /api/users/{id}/disable", a.handleUserDisable)

	// Vouchers
	mux.HandleFunc("POST /api/vouchers/generate", a.handleVouchersGenerate)
	mux.HandleFunc("GET /api/vouchers", a.handleVouchersList)
	mux.HandleFunc("GET /api/vouchers/batches", a.handleBatchesList)
	mux.HandleFunc("DELETE /api/vouchers/{id}", a.handleUserDelete)
	mux.HandleFunc("POST /api/vouchers/batch/{batchId}/delete", a.handleVouchersBatchDelete)

	// Sessions
	mux.HandleFunc("GET /api/sessions", a.handleSessionsList)
	mux.HandleFunc("DELETE /api/sessions/{id}", a.handleSessionKick)

	// Revendeurs
	mux.HandleFunc("GET /api/resellers", a.handleResellersList)
	mux.HandleFunc("POST /api/resellers", a.handleResellerCreate)
	mux.HandleFunc("PUT /api/resellers/{id}", a.handleResellerUpdate)
	mux.HandleFunc("DELETE /api/resellers/{id}", a.handleResellerDelete)
	mux.HandleFunc("POST /api/resellers/{id}/credit", a.handleResellerCredit)

	// Divers
	mux.HandleFunc("GET /api/transactions", a.handleTransactionsList)
	mux.HandleFunc("GET /api/reports", a.handleReports)
	mux.HandleFunc("GET /api/accounting", a.handleAccounting)
	mux.HandleFunc("GET /api/accounting/export", a.handleAccountingExport)
	mux.HandleFunc("GET /api/wave/link", a.handleWaveLink)
	mux.HandleFunc("GET /api/stats/hourly", a.handleStatsHourly)

	mux.HandleFunc("GET /api/activity", a.handleActivityList)
	mux.HandleFunc("GET /api/settings", a.handleSettingsGet)
	mux.HandleFunc("PUT /api/settings", a.handleSettingsPut)
	mux.HandleFunc("POST /api/admin/reset", a.handleReset)
	mux.HandleFunc("POST /api/admin/reload", a.handleReload)

	// Administration plateforme (rôle admin uniquement)
	mux.HandleFunc("GET /api/admin/accounts", a.handleAdminAccounts)
	mux.HandleFunc("POST /api/admin/accounts/{id}/status", a.handleAdminAccountStatus)

	// P0 (audit Mikhmon) — voir docs/CONTRACT-V2.md
	// Modèles de vouchers (F2)
	mux.HandleFunc("GET /api/templates", a.handleTemplatesList)
	mux.HandleFunc("POST /api/templates", a.handleTemplateCreate)
	mux.HandleFunc("PUT /api/templates/{id}", a.handleTemplateUpdate)
	mux.HandleFunc("DELETE /api/templates/{id}", a.handleTemplateDelete)
	// Journal utilisateurs (F3)
	mux.HandleFunc("GET /api/user-logs", a.handleUserLogsList)
	mux.HandleFunc("GET /api/user-logs/export", a.handleUserLogsExport)
	// Actions utilisateurs (F4/F5)
	mux.HandleFunc("POST /api/users/{id}/reset-stats", a.handleUserResetStats)
	mux.HandleFunc("POST /api/users/{id}/extend", a.handleUserExtend)
	mux.HandleFunc("GET /api/users/export", a.handleUsersExport)
	mux.HandleFunc("POST /api/users/cleanup", a.handleUsersCleanup)

	// P1 (audit Mikhmon) — voir docs/CONTRACT-V2.md (handlers_p1.go)
	// Trafic temps réel (F6)
	mux.HandleFunc("GET /api/routers/{id}/traffic", a.handleRouterTraffic)
	// IP bindings (F7)
	mux.HandleFunc("GET /api/routers/{id}/ipbindings", a.handleIPBindingsList)
	mux.HandleFunc("POST /api/routers/{id}/ipbindings", a.handleIPBindingCreate)
	mux.HandleFunc("PUT /api/ipbindings/{id}", a.handleIPBindingUpdate)
	mux.HandleFunc("DELETE /api/ipbindings/{id}", a.handleIPBindingDelete)
	// Ping + statut de commande (F8)
	mux.HandleFunc("POST /api/routers/{id}/ping", a.handleRouterPing)
	mux.HandleFunc("GET /api/commands/{id}", a.handleCommandStatus)
	// Outils routeur (F9)
	mux.HandleFunc("GET /api/routers/{id}/dhcp", a.handleRouterDhcp)
	mux.HandleFunc("GET /api/routers/{id}/hosts", a.handleRouterHosts)
	mux.HandleFunc("GET /api/routers/{id}/cookies", a.handleRouterCookies)
	mux.HandleFunc("GET /api/routers/{id}/log", a.handleRouterLog)
	// Scheduler + alimentation (F10)
	mux.HandleFunc("GET /api/routers/{id}/scheduler", a.handleSchedulerGet)
	mux.HandleFunc("POST /api/routers/{id}/scheduler", a.handleSchedulerCreate)
	mux.HandleFunc("POST /api/routers/{id}/scheduler-toggle", a.handleSchedulerToggle)
	mux.HandleFunc("POST /api/routers/{id}/scheduler-remove", a.handleSchedulerRemove)
	mux.HandleFunc("POST /api/routers/{id}/reboot", a.handleRouterReboot)
	mux.HandleFunc("POST /api/routers/{id}/shutdown", a.handleRouterShutdown)

	// Fallback API -> 404 JSON
	mux.HandleFunc("/api/", a.handleAPINotFound)

	return a.authMiddleware(mux)
}

// ---------------------------------------------------------------------------
// Middlewares & helpers
// ---------------------------------------------------------------------------

type claimsCtxKey struct{}

func claimsFrom(r *http.Request) *auth.Claims {
	if c, ok := r.Context().Value(claimsCtxKey{}).(*auth.Claims); ok {
		return c
	}
	return nil
}

// accountScope — identifiant du compte SaaS du porteur du token. Les tokens
// émis avant la migration multi-tenant ne portent pas de compte : ils ne
// peuvent provenir que de l'unique tenant historique → compte principal.
func accountScope(r *http.Request) string {
	if c := claimsFrom(r); c != nil && c.Acc != "" {
		return c.Acc
	}
	return model.AccountMainID
}

// isPlatformAdmin — true si le porteur du token a le rôle « admin »
// (super-administrateur de la plateforme MikCloud).
func isPlatformAdmin(r *http.Request) bool {
	if c := claimsFrom(r); c != nil {
		return c.Role == "admin"
	}
	return false
}

func (a *API) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/api/auth/login" || path == "/api/auth/register" || !strings.HasPrefix(path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			writeErr(w, http.StatusUnauthorized, "Token d'authentification manquant")
			return
		}
		token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
		claims, err := auth.Verify(a.secret, token)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "Token invalide ou expiré")
			return
		}
		// Compte désactivé ou supprimé : le token reste signé mais n'autorise plus
		// aucun accès (effet immédiat de POST /api/admin/accounts/{id}/status).
		if claims.Acc != "" {
			a.store.Lock()
			active := false
			for i := range a.store.Data().Accounts {
				if acc := &a.store.Data().Accounts[i]; acc.ID == claims.Acc && acc.Status == "active" {
					active = true
					break
				}
			}
			a.store.Unlock()
			if !active {
				writeErr(w, http.StatusUnauthorized, "Compte désactivé — contactez le support")
				return
			}
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), claimsCtxKey{}, claims)))
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func decodeBody(r *http.Request, v any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}

func queryInt(r *http.Request, key string, def, min, max int) int {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		return def
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < min {
		return def
	}
	if v > max {
		return max
	}
	return v
}

func ptrString(s string) *string { return &s }
func ptrInt(i int) *int          { return &i }

// logActivity ajoute une entrée en tête du journal DU COMPTE (sous verrou).
func (a *API) logActivity(db *model.DB, acc, typ, message string) {
	db.Activity = append([]model.Activity{{
		ID: model.NewID("act-"), AccountID: acc, Type: typ, Message: message, At: model.NowISO(),
	}}, db.Activity...)
	if len(db.Activity) > 500 {
		db.Activity = db.Activity[:500]
	}
}

func (a *API) gatewayFor(r model.Router) routeros.Gateway {
	a.gwMu.Lock()
	defer a.gwMu.Unlock()
	if g, ok := a.gws[r.ID]; ok {
		return g
	}
	var g routeros.Gateway
	if r.Mode == "real" {
		g = routeros.NewRealGateway(a.store, r.ID)
	} else {
		g = routeros.NewSimulator(a.store, r.ID)
	}
	a.gws[r.ID] = g
	return g
}

func (a *API) invalidateGateway(routerID string) {
	a.gwMu.Lock()
	defer a.gwMu.Unlock()
	if g, ok := a.gws[routerID]; ok {
		_ = g.Close()
		delete(a.gws, routerID)
	}
}

func (a *API) clearGateways() {
	a.gwMu.Lock()
	defer a.gwMu.Unlock()
	for id, g := range a.gws {
		_ = g.Close()
		delete(a.gws, id)
	}
}

// findX helpers — TOUJOURS scopés au compte demandeur : une ressource d'un
// autre compte est invisible (404, sans fuiter son existence).
func findRouterScoped(db *model.DB, id, acc string) *model.Router {
	for i := range db.Routers {
		if db.Routers[i].ID == id && db.Routers[i].AccountID == acc {
			return &db.Routers[i]
		}
	}
	return nil
}

// sanitizeVoucherComment nettoie un commentaire libre de voucher avant envoi
// au routeur : une seule ligne, 64 caractères max (lisible dans Winbox).
func sanitizeVoucherComment(s string) string {
	s = strings.Map(func(c rune) rune {
		if c == '\n' || c == '\r' || c == '\t' || c < 0x20 {
			return ' '
		}
		return c
	}, strings.TrimSpace(s))
	r := []rune(s)
	if len(r) > 64 {
		r = r[:64]
	}
	return strings.TrimSpace(string(r))
}

// quotaNote — suffixe de journal (ex. « — quota 5 Go ») pour un quota en Mo.
func quotaNote(mb int) string {
	if mb <= 0 {
		return ""
	}
	if mb >= 1024 && mb%1024 == 0 {
		return fmt.Sprintf(" — quota %d Go", mb/1024)
	}
	return fmt.Sprintf(" — quota %d Mo", mb)
}

func findProfileScoped(db *model.DB, id, acc string) *model.Profile {
	for i := range db.Profiles {
		if db.Profiles[i].ID == id && db.Profiles[i].AccountID == acc {
			return &db.Profiles[i]
		}
	}
	return nil
}

func findUserScoped(db *model.DB, id, acc string) *model.HotspotUser {
	for i := range db.HotspotUsers {
		if db.HotspotUsers[i].ID == id && db.HotspotUsers[i].AccountID == acc {
			return &db.HotspotUsers[i]
		}
	}
	return nil
}

func findResellerScoped(db *model.DB, id, acc string) *model.Reseller {
	for i := range db.Resellers {
		if db.Resellers[i].ID == id && db.Resellers[i].AccountID == acc {
			return &db.Resellers[i]
		}
	}
	return nil
}

func sanitizeRouter(r model.Router) model.Router {
	r.Password = ""
	// Le hash du token agent n'est jamais exposé à l'interface (secret serveur).
	r.AgentTokenHash = ""
	return r
}

// usernameTaken — unicité des usernames hotspot PAR COMPTE : deux comptes
// peuvent chacun avoir un voucher « SC-ABCDE ».
func usernameTaken(db *model.DB, acc, username string) bool {
	for i := range db.HotspotUsers {
		if db.HotspotUsers[i].AccountID == acc && db.HotspotUsers[i].Username == username {
			return true
		}
	}
	return false
}

// normalizeHotspotLoginUrl — nettoie l'URL de connexion hotspot utilisée par
// les QR codes des vouchers. Vide → "" (QR texte). Sinon : schéma http(s)
// obligatoire (new URL complète → découpe pour retirer "?query"/"#fragment"
// accidentels : les paramètres username/password sont ajoutés à l'impression).
func normalizeHotspotLoginUrl(raw string) (string, bool) {
	u := strings.TrimSpace(raw)
	if u == "" {
		return "", true
	}
	if !strings.Contains(u, "://") {
		u = "http://" + u
	}
	parsed, err := url.Parse(u)
	if err != nil {
		return "", false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", false
	}
	if parsed.Host == "" {
		return "", false
	}
	return parsed.Scheme + "://" + parsed.Host + parsed.Path, true
}

// ensureSettings — réglages du compte, créés avec les défauts FCFA si absents
// (à appeler sous verrou ; la création est persistée par le Save de l'appelant).
func ensureSettings(db *model.DB, acc string) model.Settings {
	if s, ok := db.SettingsByAccount[acc]; ok {
		return s
	}
	name := "MikCloud"
	for i := range db.Accounts {
		if db.Accounts[i].ID == acc {
			name = db.Accounts[i].Name
			break
		}
	}
	s := model.Settings{
		Tenant: model.Tenant{
			Name: name, Currency: "XOF", Timezone: "Africa/Abidjan",
			ExpiryPolicyMode: "keep", ExpiryPolicyAfterDays: 30,
		},
		Plan: model.Plan{Name: "Bêta", MaxRouters: "Illimité", MaxUsers: "Illimité"},
	}
	if db.SettingsByAccount == nil {
		db.SettingsByAccount = map[string]model.Settings{}
	}
	db.SettingsByAccount[acc] = s
	return s
}

// ---------------------------------------------------------------------------
// Healthcheck & 404
// ---------------------------------------------------------------------------

func (a *API) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"service": "mikcloud-hotspot-api",
		"version": "1.0.0",
		"time":    model.NowISO(),
	})
}

func (a *API) handleAPINotFound(w http.ResponseWriter, r *http.Request) {
	writeErr(w, http.StatusNotFound, "Route introuvable")
}

// ---------------------------------------------------------------------------
// Auth
// ---------------------------------------------------------------------------

func (a *API) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		writeErr(w, http.StatusBadRequest, "Nom d'utilisateur et mot de passe requis")
		return
	}
	a.store.Lock()
	var id, name, username, role, salt, hash, accID string
	var accName, accStatus string
	var user *model.AdminUser
	for i := range a.store.Data().Users {
		u := &a.store.Data().Users[i]
		if strings.EqualFold(u.Username, req.Username) {
			id, name, username, role, salt, hash = u.ID, u.Name, u.Username, u.Role, u.Salt, u.PasswordHash
			accID = u.AccountID
			user = u
			break
		}
	}
	if accID != "" {
		for i := range a.store.Data().Accounts {
			acc := &a.store.Data().Accounts[i]
			if acc.ID == accID {
				accName, accStatus = acc.Name, acc.Status
				break
			}
		}
	}
	a.store.Unlock()
	if id == "" || !auth.CheckPassword(req.Password, salt, hash) {
		writeErr(w, http.StatusBadRequest, "Identifiants invalides")
		return
	}
	// Compte désactivé : le login est refusé même avec des identifiants valides.
	if accID != "" && accStatus == "disabled" {
		writeErr(w, http.StatusForbidden, "Compte désactivé — contactez le support")
		return
	}
	// Migration transparente : ancien hash SHA-256 → bcrypt au premier login.
	if user != nil && auth.IsLegacyHash(hash) {
		a.store.Lock()
		user.PasswordHash = auth.HashPassword(req.Password, "")
		user.Salt = ""
		a.store.Save()
		a.store.Unlock()
	}
	token := auth.Sign(a.secret, auth.NewClaims(id, name, role, accID))
	writeJSON(w, http.StatusOK, map[string]any{
		"token": token,
		"user": map[string]any{
			"id": id, "name": name, "username": username, "role": role,
			"accountId": accID, "accountName": accName,
		},
	})
}

// handlePasswordChange — POST /api/auth/password : l'utilisateur connecté
// modifie SON PROPRE mot de passe. Exige le mot de passe actuel (une session
// laissée ouverte ne suffit pas), 8 caractères minimum, différent de l'actuel.
// Le flag PasswordSetByUser protège le nouveau mot de passe contre l'override
// ADMIN_PASSWORD au prochain démarrage/reload (tant que l'opérateur ne change
// pas la variable). Le token en cours reste valide jusqu'à son expiration.
func (a *API) handlePasswordChange(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r)
	if claims == nil {
		writeErr(w, http.StatusUnauthorized, "Token invalide ou expiré")
		return
	}
	var req struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	if req.CurrentPassword == "" || req.NewPassword == "" {
		writeErr(w, http.StatusBadRequest, "Mot de passe actuel et nouveau mot de passe requis")
		return
	}
	if len(req.NewPassword) < 8 {
		writeErr(w, http.StatusBadRequest, "Le nouveau mot de passe doit faire au moins 8 caractères")
		return
	}
	if req.NewPassword == req.CurrentPassword {
		writeErr(w, http.StatusBadRequest, "Le nouveau mot de passe doit être différent de l'actuel")
		return
	}
	a.store.Lock()
	db := a.store.Data()
	var user *model.AdminUser
	for i := range db.Users {
		if db.Users[i].ID == claims.Sub {
			user = &db.Users[i]
			break
		}
	}
	// Message unique pour utilisateur inconnu et mot de passe incorrect (pas d'oracle).
	if user == nil || !auth.CheckPassword(req.CurrentPassword, user.Salt, user.PasswordHash) {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, "Mot de passe actuel incorrect")
		return
	}
	user.PasswordHash = auth.HashPassword(req.NewPassword, "") // bcrypt : sel intégré
	user.Salt = ""
	user.PasswordSetByUser = true
	a.logActivity(db, user.AccountID, "system", "Mot de passe modifié par "+user.Username)
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleRegister — inscription SaaS : crée un COMPTE isolé (Account), son
// propriétaire (owner) et ses réglages par défaut, puis connecte immédiatement.
// Si REGISTER_KEY est définie (bêta privée), la clé doit être fournie. Le
// nouveau compte démarre vide : il ne voit aucune donnée des autres comptes.
func (a *API) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string `json:"name"`
		Username string `json:"username"`
		Password string `json:"password"`
		Key      string `json:"key"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	// Clé d'invitation (optionnelle) : limite l'inscription sur une bêta privée.
	if inviteKey := os.Getenv("REGISTER_KEY"); inviteKey != "" && req.Key != inviteKey {
		writeErr(w, http.StatusForbidden, "Inscription fermée — clé d'invitation requise")
		return
	}
	username := strings.ToLower(strings.TrimSpace(req.Username))
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
	if len(req.Password) < 8 {
		writeErr(w, http.StatusBadRequest, "Le mot de passe doit faire au moins 8 caractères")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = username
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
	acc := model.Account{
		ID:        model.NewID("acc-"),
		Name:      name,
		Status:    "active",
		CreatedAt: model.NowISO(),
	}
	db.Accounts = append(db.Accounts, acc)
	u := model.AdminUser{
		ID:           model.NewID("usr-"),
		AccountID:    acc.ID,
		Name:         name,
		Username:     username,
		Role:         "owner",
		PasswordHash: auth.HashPassword(req.Password, ""),
		CreatedAt:    model.NowISO(),
	}
	db.Users = append(db.Users, u)
	if db.SettingsByAccount == nil {
		db.SettingsByAccount = map[string]model.Settings{}
	}
	db.SettingsByAccount[acc.ID] = model.Settings{
		Tenant: model.Tenant{
			Name: name, Currency: "XOF", Timezone: "Africa/Abidjan",
			ExpiryPolicyMode: "keep", ExpiryPolicyAfterDays: 30,
		},
		Plan: model.Plan{Name: "Bêta", MaxRouters: "Illimité", MaxUsers: "Illimité"},
	}
	// P0 (audit Mikhmon) — chaque nouveau compte démarre avec les 3 modèles
	// de vouchers par défaut (contrat F2).
	db.Templates = append(db.Templates, store.SeedTemplatesFor(acc.ID)...)
	a.logActivity(db, acc.ID, "compte", "Nouveau compte créé : "+acc.Name)
	a.store.Save()
	a.store.Unlock()

	token := auth.Sign(a.secret, auth.NewClaims(u.ID, u.Name, u.Role, acc.ID))
	writeJSON(w, http.StatusCreated, map[string]any{
		"token": token,
		"user": map[string]any{
			"id": u.ID, "name": u.Name, "username": u.Username, "role": u.Role,
			"accountId": acc.ID, "accountName": acc.Name,
		},
	})
}

func (a *API) handleMe(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r)
	if claims == nil {
		writeErr(w, http.StatusUnauthorized, "Token invalide ou expiré")
		return
	}
	a.store.Lock()
	db := a.store.Data()
	id, name, username, role := claims.Sub, claims.Name, claims.Sub, claims.Role
	accID := accountScope(r)
	for i := range db.Users {
		u := &db.Users[i]
		if u.ID == claims.Sub {
			id, name, username, role, accID = u.ID, u.Name, u.Username, u.Role, u.AccountID
			if accID == "" {
				accID = model.AccountMainID // token émis avant la migration multi-tenant
			}
			break
		}
	}
	accName := ""
	for i := range db.Accounts {
		if db.Accounts[i].ID == accID {
			accName = db.Accounts[i].Name
			break
		}
	}
	a.store.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"user": map[string]any{
			"id": id, "name": name, "username": username, "role": role,
			"accountId": accID, "accountName": accName,
		},
	})
}

// ---------------------------------------------------------------------------
// Dashboard
// ---------------------------------------------------------------------------

type timelinePoint struct {
	T     string `json:"t"`
	Value int    `json:"value"`
}

type dayValue struct {
	Day   string `json:"day"`
	Value int    `json:"value"`
}

type topProfilePoint struct {
	Name  string `json:"name"`
	Users int    `json:"users"`
	Total int    `json:"total"`
}

func (a *API) handleDashboard(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	now := time.Now().UTC()
	a.store.Lock()
	db := a.store.Data()
	// P0 (audit Mikhmon) : simulation vivante + expiration à jour +
	// enforcement routeur (F1), comme handleSessionsList/handleUsersList.
	store.Tick(db, now)
	a.enforceExpired(db)
	a.store.Save()

	// Vue d'ensemble multi-sites : 1 compte = N hotspots. Tous les agrégats
	// ci-dessous sont calculés DANS le compte demandeur (isolation stricte).
	type siteOverview struct {
		RouterID       string `json:"routerId"`
		RouterName     string `json:"routerName"`
		Status         string `json:"status"`
		ActiveSessions int    `json:"activeSessions"`
		HotspotUsers   int    `json:"hotspotUsers"`
		ActiveVouchers int    `json:"activeVouchers"`
		SalesToday     int    `json:"salesToday"`
		Revenue30d     int    `json:"revenue30d"`
	}

	accSessions := []model.Session{}
	onlineNow := map[string]bool{}
	sessionsByRouter := map[string]int{}
	for i := range db.Sessions {
		s := db.Sessions[i]
		if s.AccountID != acc {
			continue
		}
		accSessions = append(accSessions, s)
		onlineNow[s.Username] = true
		sessionsByRouter[s.RouterID]++
	}

	totalUsers, activeVouchers := 0, 0
	usersByRouter := map[string]int{}
	vouchersByRouter := map[string]int{}
	counts := map[string]int{}
	totals := map[string]int{}
	for i := range db.HotspotUsers {
		u := &db.HotspotUsers[i]
		if u.AccountID != acc {
			continue
		}
		totalUsers++
		counts[u.ProfileName]++
		totals[u.ProfileName] += u.Price
		if model.EffectiveStatus(u, now) != "active" {
			continue
		}
		usersByRouter[u.RouterID]++
		if u.Kind == "voucher" {
			vouchersByRouter[u.RouterID]++
			activeVouchers++
		}
	}

	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	cutoff30 := now.AddDate(0, 0, -30)
	salesToday, revenue30d := 0, 0
	salesTodayByRouter := map[string]int{}
	revenue30dByRouter := map[string]int{}
	for i := range db.Sales {
		s := db.Sales[i]
		if s.AccountID != acc {
			continue
		}
		at, err := time.Parse(time.RFC3339, s.At)
		if err != nil {
			continue
		}
		if !at.Before(todayStart) {
			salesToday += s.Count
			salesTodayByRouter[s.RouterID] += s.Count
		}
		if at.After(cutoff30) {
			revenue30d += s.Amount
			revenue30dByRouter[s.RouterID] += s.Amount
		}
	}

	accRouters := []model.Router{}
	routersOnline := 0
	for i := range db.Routers {
		rr := db.Routers[i]
		if rr.AccountID != acc {
			continue
		}
		accRouters = append(accRouters, rr)
		if rr.Status == "online" {
			routersOnline++
		}
	}

	sites := []siteOverview{}
	for _, rr := range accRouters {
		sites = append(sites, siteOverview{
			RouterID:       rr.ID,
			RouterName:     rr.Name,
			Status:         rr.Status,
			ActiveSessions: sessionsByRouter[rr.ID],
			HotspotUsers:   usersByRouter[rr.ID],
			ActiveVouchers: vouchersByRouter[rr.ID],
			SalesToday:     salesTodayByRouter[rr.ID],
			Revenue30d:     revenue30dByRouter[rr.ID],
		})
	}
	kpis := map[string]any{
		"activeSessions": len(accSessions),
		"totalUsers":     totalUsers,
		"activeVouchers": activeVouchers,
		"salesToday":     salesToday,
		"revenue30d":     revenue30d,
		"routersOnline":  routersOnline,
		"routersTotal":   len(accRouters),
		"onlineNow":      len(onlineNow),
	}

	revenueByDay := buildRevenueByDay(db, acc, now, 14)

	top := []topProfilePoint{}
	for name, c := range counts {
		top = append(top, topProfilePoint{Name: name, Users: c, Total: totals[name]})
	}
	sort.Slice(top, func(i, j int) bool {
		if top[i].Users != top[j].Users {
			return top[i].Users > top[j].Users
		}
		return top[i].Total > top[j].Total
	})
	if len(top) > 5 {
		top = top[:5]
	}

	recent := []model.Activity{}
	for _, act := range db.Activity {
		if act.AccountID != acc {
			continue
		}
		if len(recent) >= 12 {
			break
		}
		recent = append(recent, act)
	}
	loc := accountTimezone(db, acc)
	timeline := buildHourlyLogins(db, acc, now, loc)
	a.store.Unlock()

	// N°10 — courbe 24 h RÉELLE : connexions/heure agrégées depuis les
	// UserLogs (simulation + agent), dans le fuseau du compte. Un compte
	// sans activité affiche zéro — honnête par construction.
	writeJSON(w, http.StatusOK, map[string]any{
		"kpis":             kpis,
		"sites":            sites,
		"sessionsTimeline": timeline,
		"revenueByDay":     revenueByDay,
		"topProfiles":      top,
		"recentActivity":   recent,
	})
}

func buildRevenueByDay(db *model.DB, acc string, now time.Time, days int) []dayValue {
	out := make([]dayValue, 0, days)
	for i := days - 1; i >= 0; i-- {
		day := now.AddDate(0, 0, -i)
		start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC)
		end := start.Add(24 * time.Hour)
		sum := 0
		for _, s := range db.Sales {
			if s.AccountID != acc {
				continue
			}
			if at, err := time.Parse(time.RFC3339, s.At); err == nil && !at.Before(start) && at.Before(end) {
				sum += s.Amount
			}
		}
		out = append(out, dayValue{Day: fmt.Sprintf("%02d/%02d", day.Day(), int(day.Month())), Value: sum})
	}
	return out
}

// ---------------------------------------------------------------------------
// Routeurs
// ---------------------------------------------------------------------------

func (a *API) handleRoutersList(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	a.store.Lock()
	db := a.store.Data()
	rs := []model.Router{}
	userCount := map[string]int{}
	for _, u := range db.HotspotUsers {
		if u.AccountID == acc {
			userCount[u.RouterID]++
		}
	}
	sessCount := map[string]int{}
	for _, s := range db.Sessions {
		if s.AccountID == acc {
			sessCount[s.RouterID]++
		}
	}
	for _, rr := range db.Routers {
		if rr.AccountID == acc {
			rs = append(rs, rr)
		}
	}
	a.store.Unlock()
	sort.Slice(rs, func(i, j int) bool { return rs[i].CreatedAt > rs[j].CreatedAt })
	out := make([]model.Router, 0, len(rs))
	for _, rr := range rs {
		rr.HotspotUsers = userCount[rr.ID]
		rr.ActiveSessions = sessCount[rr.ID]
		out = append(out, sanitizeRouter(rr))
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *API) handleRouterCreate(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	var req struct {
		Name            string `json:"name"`
		Host            string `json:"host"`
		Port            int    `json:"port"`
		Username        string `json:"username"`
		Password        string `json:"password"`
		Mode            string `json:"mode"`
		HotspotLoginUrl string `json:"hotspotLoginUrl"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	name := strings.TrimSpace(req.Name)
	host := strings.TrimSpace(req.Host)
	if name == "" {
		writeErr(w, http.StatusBadRequest, "Nom du routeur requis")
		return
	}
	hotspotLoginUrl, ok := normalizeHotspotLoginUrl(req.HotspotLoginUrl)
	if !ok {
		writeErr(w, http.StatusBadRequest, "URL de connexion hotspot invalide (http(s) attendu)")
		return
	}
	mode := req.Mode
	if mode == "" {
		mode = "simulated"
	}
	if mode != "simulated" && mode != "real" && mode != "agent" {
		writeErr(w, http.StatusBadRequest, "Mode invalide (simulated, real ou agent)")
		return
	}
	router := model.Router{
		ID: model.NewID("r-"), AccountID: acc, Name: name, Host: host, Mode: mode,
		Username: strings.TrimSpace(req.Username), Password: req.Password,
		HotspotLoginUrl: hotspotLoginUrl,
		Status:          "online", CreatedAt: model.NowISO(),
	}

	var agentToken string
	switch mode {
	case "agent":
		// Mode agent : aucune adresse IP requise (connexions 100 % sortantes,
		// compatibles CGNAT/Starlink). En ligne dès le premier check-in.
		tok, err := agent.NewToken()
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "Génération du token agent impossible")
			return
		}
		agentToken = tok
		router.AgentTokenHash = agent.HashToken(tok)
		router.TokenPreview = agent.Preview(tok)
		router.Status = "offline"
	case "real":
		port := req.Port
		if port == 0 {
			port = 8728
		}
		if port < 1 || port > 65535 {
			writeErr(w, http.StatusBadRequest, "Port invalide")
			return
		}
		if host == "" {
			writeErr(w, http.StatusBadRequest, "Adresse du routeur requise")
			return
		}
		router.Port = port
		username := strings.TrimSpace(req.Username)
		if username == "" {
			username = "admin"
		}
		router.Username = username
		res, err := routeros.TestRouter(host, port, username, req.Password)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "Connexion impossible : "+err.Error())
			return
		}
		router.Version = res.Version
	default: // simulated
		if host == "" {
			host = "simulé"
		}
		port := req.Port
		if port == 0 {
			port = 8728
		}
		username := strings.TrimSpace(req.Username)
		if username == "" {
			username = "admin"
		}
		router.Host = host
		router.Port = port
		router.Username = username
		versions := []string{"7.14.3", "7.15.2", "7.13.5", "7.12.1"}
		router.Version = versions[rand.Intn(len(versions))]
		router.UptimeSec = int64(3600 + rand.Intn(72*3600))
		router.CPULoad = 5 + rand.Intn(30)
	}
	a.store.Lock()
	for _, rr := range a.store.Data().Routers {
		if rr.AccountID == acc && strings.EqualFold(rr.Name, name) {
			a.store.Unlock()
			writeErr(w, http.StatusBadRequest, "Ce nom de routeur existe déjà")
			return
		}
	}
	a.store.Data().Routers = append(a.store.Data().Routers, router)
	msg := "Routeur " + router.Name + " ajouté"
	if mode == "agent" {
		msg += " (mode agent — en ligne au premier check-in)"
		// P1 (audit Mikhmon) — F6/F8 : le premier read_state est enfilé
		// dès la création : le premier check-in rapporte télémétrie,
		// carte/disque, interfaces et sessions. Chaque résultat en
		// enfile un suivant (handleAgentResult) → télémétrie continue.
		queueCommandLocked(a.store.Data(), acc, router.ID, model.CmdReadState, map[string]any{})
	}
	a.logActivity(a.store.Data(), acc, "router", msg)
	a.store.Save()
	a.store.Unlock()

	data, _ := json.Marshal(sanitizeRouter(router))
	var out map[string]any
	_ = json.Unmarshal(data, &out)
	if agentToken != "" {
		out["agentToken"] = agentToken
		out["installScript"] = agent.InstallScript(agentBaseURL(r), agentToken, name)
		out["message"] = "Copiez le script et collez-le dans Terminal (Winbox) — le routeur passera « En ligne » au premier check-in (≤ 45 s)."
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *API) handleRouterUpdate(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	id := r.PathValue("id")
	var req struct {
		Name            *string `json:"name"`
		Host            *string `json:"host"`
		Port            *int    `json:"port"`
		Username        *string `json:"username"`
		Password        *string `json:"password"`
		Mode            *string `json:"mode"`
		HotspotLoginUrl *string `json:"hotspotLoginUrl"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	a.store.Lock()
	cur := findRouterScoped(a.store.Data(), id, acc)
	if cur == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Routeur introuvable")
		return
	}
	updated := *cur
	a.store.Unlock()

	if req.Name != nil {
		updated.Name = strings.TrimSpace(*req.Name)
	}
	if updated.Name == "" {
		writeErr(w, http.StatusBadRequest, "Nom du routeur requis")
		return
	}
	if req.HotspotLoginUrl != nil {
		hlu, ok := normalizeHotspotLoginUrl(*req.HotspotLoginUrl)
		if !ok {
			writeErr(w, http.StatusBadRequest, "URL de connexion hotspot invalide (http(s) attendu)")
			return
		}
		updated.HotspotLoginUrl = hlu
	}
	if req.Host != nil {
		updated.Host = strings.TrimSpace(*req.Host)
	}
	if req.Port != nil {
		updated.Port = *req.Port
	}
	if req.Username != nil {
		updated.Username = strings.TrimSpace(*req.Username)
	}
	if req.Password != nil && *req.Password != "" {
		updated.Password = *req.Password
	}
	prevMode := updated.Mode
	if req.Mode != nil {
		if *req.Mode != "simulated" && *req.Mode != "real" && *req.Mode != "agent" {
			writeErr(w, http.StatusBadRequest, "Mode invalide (simulated, real ou agent)")
			return
		}
		updated.Mode = *req.Mode
	}
	if updated.Port < 1 || updated.Port > 65535 {
		updated.Port = 8728
	}
	if updated.Mode != "agent" && updated.Host == "" {
		writeErr(w, http.StatusBadRequest, "Adresse du routeur requise")
		return
	}

	var agentToken string
	if updated.Mode == "agent" {
		if prevMode != "agent" || updated.AgentTokenHash == "" {
			tok, err := agent.NewToken()
			if err != nil {
				writeErr(w, http.StatusInternalServerError, "Génération du token agent impossible")
				return
			}
			agentToken = tok
			updated.AgentTokenHash = agent.HashToken(tok)
			updated.TokenPreview = agent.Preview(tok)
			updated.LastSeen = ""
			updated.Status = "offline"
		}
	} else {
		if prevMode == "agent" {
			updated.AgentTokenHash = ""
			updated.TokenPreview = ""
			updated.LastSeen = ""
		}
		// Test de connectivité (best effort) : met à jour le statut.
		if updated.Mode == "real" {
			if res, err := routeros.TestRouter(updated.Host, updated.Port, updated.Username, updated.Password); err != nil {
				updated.Status = "offline"
			} else {
				updated.Status = "online"
				if res.Version != "" {
					updated.Version = res.Version
				}
			}
		} else {
			updated.Status = "online"
		}
	}

	a.store.Lock()
	cur = findRouterScoped(a.store.Data(), id, acc)
	if cur == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Routeur introuvable")
		return
	}
	for _, rr := range a.store.Data().Routers {
		if rr.ID != id && rr.AccountID == acc && strings.EqualFold(rr.Name, updated.Name) {
			a.store.Unlock()
			writeErr(w, http.StatusBadRequest, "Ce nom de routeur existe déjà")
			return
		}
	}
	*cur = updated
	a.logActivity(a.store.Data(), acc, "router", "Routeur "+updated.Name+" modifié")
	a.store.Save()
	a.store.Unlock()
	a.invalidateGateway(id)

	data, _ := json.Marshal(sanitizeRouter(updated))
	var out map[string]any
	_ = json.Unmarshal(data, &out)
	if agentToken != "" {
		out["agentToken"] = agentToken
		out["installScript"] = agent.InstallScript(agentBaseURL(r), agentToken, updated.Name)
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *API) handleRouterDelete(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	id := r.PathValue("id")
	a.store.Lock()
	db := a.store.Data()
	cur := findRouterScoped(db, id, acc)
	if cur == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Routeur introuvable")
		return
	}
	name := cur.Name
	routers := db.Routers[:0]
	for _, rr := range db.Routers {
		if rr.ID != id {
			routers = append(routers, rr)
		}
	}
	db.Routers = routers
	users := db.HotspotUsers[:0]
	for _, u := range db.HotspotUsers {
		if u.RouterID != id {
			users = append(users, u)
		}
	}
	db.HotspotUsers = users
	sessions := db.Sessions[:0]
	for _, s := range db.Sessions {
		if s.RouterID != id {
			sessions = append(sessions, s)
		}
	}
	db.Sessions = sessions
	commands := db.Commands[:0]
	for _, c := range db.Commands {
		if c.RouterID != id {
			commands = append(commands, c)
		}
	}
	db.Commands = commands
	a.logActivity(db, acc, "router", "Routeur "+name+" supprimé")
	a.store.Save()
	a.store.Unlock()
	a.invalidateGateway(id)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *API) handleRouterTest(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	id := r.PathValue("id")
	a.store.Lock()
	cur := findRouterScoped(a.store.Data(), id, acc)
	if cur == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Routeur introuvable")
		return
	}
	routerCopy := *cur
	a.store.Unlock()

	if routerCopy.Mode == "agent" {
		online := false
		last := "jamais"
		if t, err := time.Parse(time.RFC3339, routerCopy.LastSeen); err == nil {
			online = time.Since(t) < OnlineWindow
			last = routerCopy.LastSeen
		}
		msg := "Agent connecté (dernier check-in : " + last + ")"
		if !online {
			msg = "Agent injoignable : aucun check-in récent. Recollez le script d'installation si le scheduler a été supprimé."
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok": online, "message": msg, "latencyMs": 0, "version": routerCopy.Version,
		})
		return
	}

	gw := a.gatewayFor(routerCopy)
	res, err := gw.TestConnection()
	if err != nil {
		a.store.Lock()
		if rr := findRouterScoped(a.store.Data(), id, acc); rr != nil {
			rr.Status = "offline"
		}
		a.store.Save()
		a.store.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{
			"ok": false, "message": "Connexion échouée : " + err.Error(), "latencyMs": 0, "version": "",
		})
		return
	}
	a.store.Lock()
	if rr := findRouterScoped(a.store.Data(), id, acc); rr != nil {
		rr.Status = "online"
		if routerCopy.Mode == "real" && res.Version != "" {
			rr.Version = res.Version
		}
	}
	a.store.Save()
	a.store.Unlock()
	msg := "Connexion réussie"
	if routerCopy.Mode == "simulated" {
		msg = "Connexion réussie (routeur simulé)"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "message": msg, "latencyMs": res.LatencyMs, "version": res.Version,
	})
}

func (a *API) handleRouterStats(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	id := r.PathValue("id")
	a.store.Lock()
	cur := findRouterScoped(a.store.Data(), id, acc)
	if cur == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Routeur introuvable")
		return
	}
	routerCopy := *cur
	a.store.Unlock()

	if routerCopy.Mode == "agent" {
		// Mode agent : télémétrie en cache, mise à jour par read_state.
		a.store.Lock()
		active := 0
		for _, s := range a.store.Data().Sessions {
			if s.RouterID == id && s.AccountID == acc {
				active++
			}
		}
		a.store.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{
			"cpuLoad": routerCopy.CPULoad, "memUsedPct": 0,
			"freeMemoryMb": 0, "totalMemoryMb": 0,
			"uptimeSec": routerCopy.UptimeSec, "version": routerCopy.Version,
			"activeSessions": active,
		})
		return
	}

	gw := a.gatewayFor(routerCopy)
	stats, err := gw.SystemStats()
	if err != nil {
		writeErr(w, http.StatusBadRequest, "Statistiques indisponibles : "+err.Error())
		return
	}
	activeSessions := 0
	if routerCopy.Mode == "real" {
		if live, lerr := gw.ListSessions(); lerr == nil {
			activeSessions = len(live)
		}
	} else {
		a.store.Lock()
		for _, s := range a.store.Data().Sessions {
			if s.RouterID == id && s.AccountID == acc {
				activeSessions++
			}
		}
		a.store.Save()
		a.store.Unlock()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"cpuLoad":        stats.CPULoad,
		"memUsedPct":     stats.MemUsedPct,
		"freeMemoryMb":   stats.FreeMemoryMb,
		"totalMemoryMb":  stats.TotalMemoryMb,
		"uptimeSec":      stats.UptimeSec,
		"version":        stats.Version,
		"activeSessions": activeSessions,
	})
}

// ---------------------------------------------------------------------------
// Profils
// ---------------------------------------------------------------------------

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
	if expMode != "notify" && expMode != "remove" {
		writeErr(w, http.StatusBadRequest, "Mode d'expiration invalide (notify ou remove)")
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
	}
	a.store.Lock()
	a.store.Data().Profiles = append(a.store.Data().Profiles, profile)
	a.logActivity(a.store.Data(), acc, "user", "Profil "+profile.Name+" créé")
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusCreated, profile)
}

func defaultPositive(v, def int) int {
	if v <= 0 {
		return def
	}
	return v
}

func defaultMinZero(v int) int {
	if v < 0 {
		return 0
	}
	return v
}

func (a *API) handleProfileUpdate(w http.ResponseWriter, r *http.Request) {
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
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	// Bornes contrat F1/F13 (validées avant verrou).
	if req.ExpMode != nil {
		m := strings.TrimSpace(*req.ExpMode)
		if m != "" && m != "notify" && m != "remove" {
			writeErr(w, http.StatusBadRequest, "Mode d'expiration invalide (notify ou remove)")
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
	if req.SellingPrice != nil && *req.SellingPrice >= 0 {
		p.SellingPrice = *req.SellingPrice
	}
	updated := *p
	a.logActivity(db, acc, "user", "Profil "+updated.Name+" modifié")
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, updated)
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
	a.logActivity(db, acc, "user", "Profil "+name+" supprimé")
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ---------------------------------------------------------------------------
// Utilisateurs hotspot & vouchers (liste commune)
// ---------------------------------------------------------------------------

func (a *API) handleUsersList(w http.ResponseWriter, r *http.Request) {
	a.usersList(w, r, "")
}

func (a *API) handleVouchersList(w http.ResponseWriter, r *http.Request) {
	a.usersList(w, r, "voucher")
}

func (a *API) usersList(w http.ResponseWriter, r *http.Request, kindOverride string) {
	acc := accountScope(r)
	q := r.URL.Query()
	if kindOverride != "" {
		q.Set("kind", kindOverride)
	}
	page := queryInt(r, "page", 1, 1, 1_000_000)
	pageSize := queryInt(r, "pageSize", 15, 1, 200)

	now := time.Now().UTC()
	a.store.Lock()
	db := a.store.Data()
	// P0 (audit Mikhmon) : fait vivre la simulation (statuts d'expiration à
	// jour via applyExpiry) puis applique l'enforcement routeur (F1).
	store.Tick(db, now)
	a.enforceExpired(db)
	filtered := filterUsers(db, acc, q, now)
	a.store.Save()
	a.store.Unlock()

	total := len(filtered)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data":     filtered[start:end],
		"total":    total,
		"page":     page,
		"pageSize": pageSize,
	})
}

func (a *API) handleUserCreate(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	var req struct {
		Username  string `json:"username"`
		Password  string `json:"password"`
		ProfileID string `json:"profileId"`
		RouterID  string `json:"routerId"`
		Comment   string `json:"comment"`
		Kind      string `json:"kind"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	now := time.Now().UTC()
	nowISO := model.NowISO()

	// Validation + construction (sous verrou)
	a.store.Lock()
	db := a.store.Data()
	profile := findProfileScoped(db, strings.TrimSpace(req.ProfileID), acc)
	if profile == nil {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, "Profil introuvable")
		return
	}
	router := findRouterScoped(db, strings.TrimSpace(req.RouterID), acc)
	if router == nil {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, "Routeur introuvable")
		return
	}
	routerCopy := *router
	kind := req.Kind
	if kind == "" {
		kind = "regular"
	}
	if kind != "regular" && kind != "voucher" {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, "Type d'utilisateur invalide (regular ou voucher)")
		return
	}
	username := strings.TrimSpace(req.Username)
	if username == "" {
		for i := 0; i < 50; i++ {
			candidate := "user-" + model.RandomCode(5)
			if !usernameTaken(db, acc, candidate) {
				username = candidate
				break
			}
		}
	} else if usernameTaken(db, acc, username) {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, "Ce nom d'utilisateur existe déjà")
		return
	}
	password := req.Password
	if password == "" {
		password = model.RandomCode(6)
	}
	u := model.HotspotUser{
		ID: model.NewID("u-"), AccountID: acc, Kind: kind, Username: username, Password: password,
		ProfileID: profile.ID, ProfileName: profile.Name,
		RouterID: routerCopy.ID, RouterName: routerCopy.Name,
		Status: "active", BatchID: "", ResellerID: "", ResellerName: "",
		Comment:   strings.TrimSpace(req.Comment),
		CreatedAt: nowISO,
		ExpiresAt: now.Add(time.Duration(profile.ValidityDays) * 24 * time.Hour).Format(time.RFC3339),
		UsedAt:    "", Price: profile.Price, DataQuotaMb: int64(profile.DataQuotaMb),
	}
	a.store.Unlock()

	if routerCopy.Mode == "agent" {
		// Mode agent : persistance immédiate + commande user_add en file.
		u.Username = agent.SanitizeName(u.Username)
		u.ProfileName = agent.SanitizeName(u.ProfileName)
		a.store.Lock()
		db = a.store.Data()
		db.HotspotUsers = append(db.HotspotUsers, u)
		userPayload := map[string]any{
			"name": u.Username, "password": u.Password,
			"profile": profileRef(*profile), "comment": u.Comment,
		}
		if profile.DataQuotaMb > 0 {
			userPayload["limitBytesTotal"] = profile.DataQuotaMb * 1048576
		}
		cmd := queueCommandLocked(db, routerCopy.AccountID, routerCopy.ID, model.CmdUserAdd, userPayload)
		a.logActivity(db, acc, "user", "Utilisateur "+u.Username+" créé (en attente du routeur, commande "+cmd.ID+")")
		a.store.Save()
		cmdID := cmd.ID
		a.store.Unlock()
		data, _ := json.Marshal(u)
		var out map[string]any
		_ = json.Unmarshal(data, &out)
		out["queued"] = true
		out["commandId"] = cmdID
		writeJSON(w, http.StatusOK, out)
		return
	}

	gw := a.gatewayFor(routerCopy)
	if err := gw.AddUser(&u); err != nil {
		writeErr(w, http.StatusBadRequest, "Création impossible : "+err.Error())
		return
	}

	a.store.Lock()
	a.logActivity(a.store.Data(), acc, "user", "Utilisateur "+u.Username+" créé")
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, u)
}

func (a *API) handleUserUpdate(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	id := r.PathValue("id")
	var req struct {
		Username  *string `json:"username"`
		Password  *string `json:"password"`
		ProfileID *string `json:"profileId"`
		Comment   *string `json:"comment"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	now := time.Now().UTC()

	a.store.Lock()
	db := a.store.Data()
	cur := findUserScoped(db, id, acc)
	if cur == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Utilisateur introuvable")
		return
	}
	u := *cur
	oldUsername := cur.Username
	router := findRouterScoped(db, u.RouterID, acc)
	var routerCopy *model.Router
	if router != nil {
		c := *router
		routerCopy = &c
	}
	a.store.Unlock()

	if req.Username != nil {
		username := strings.TrimSpace(*req.Username)
		if username == "" {
			writeErr(w, http.StatusBadRequest, "Nom d'utilisateur requis")
			return
		}
		a.store.Lock()
		taken := false
		for i := range a.store.Data().HotspotUsers {
			other := &a.store.Data().HotspotUsers[i]
			if other.AccountID == acc && other.ID != id && other.Username == username {
				taken = true
				break
			}
		}
		a.store.Unlock()
		if taken {
			writeErr(w, http.StatusBadRequest, "Ce nom d'utilisateur existe déjà")
			return
		}
		u.Username = username
	}
	passwordChanged := false
	if req.Password != nil && *req.Password != "" {
		u.Password = *req.Password
		passwordChanged = true
	}
	if req.ProfileID != nil && strings.TrimSpace(*req.ProfileID) != "" {
		a.store.Lock()
		p := findProfileScoped(a.store.Data(), strings.TrimSpace(*req.ProfileID), acc)
		a.store.Unlock()
		if p == nil {
			writeErr(w, http.StatusBadRequest, "Profil introuvable")
			return
		}
		u.ProfileID = p.ID
		u.ProfileName = p.Name
		u.Price = p.Price
		u.ExpiresAt = now.Add(time.Duration(p.ValidityDays) * 24 * time.Hour).Format(time.RFC3339)
	}
	if req.Comment != nil {
		u.Comment = strings.TrimSpace(*req.Comment)
	}

	if routerCopy != nil && routerCopy.Mode == "agent" {
		// Mode agent : persistance + commande user_set en file.
		a.store.Lock()
		payload := map[string]any{
			"oldName": agent.SanitizeName(oldUsername),
			"name":    agent.SanitizeName(u.Username),
		}
		if passwordChanged {
			payload["password"] = u.Password
		}
		if p := findProfileScoped(a.store.Data(), u.ProfileID, acc); p != nil {
			payload["profile"] = profileRef(*p)
		}
		if existing := findUserScoped(a.store.Data(), id, acc); existing != nil {
			*existing = u
		}
		cmd := queueCommandLocked(a.store.Data(), routerCopy.AccountID, routerCopy.ID, model.CmdUserSet, payload)
		a.logActivity(a.store.Data(), acc, "user", "Utilisateur "+u.Username+" modifié (en attente du routeur, commande "+cmd.ID+")")
		a.store.Save()
		a.store.Unlock()
		writeJSON(w, http.StatusOK, u)
		return
	}

	if routerCopy != nil {
		gw := a.gatewayFor(*routerCopy)
		if err := gw.SetUser(&u, oldUsername, passwordChanged); err != nil {
			writeErr(w, http.StatusBadRequest, "Mise à jour impossible : "+err.Error())
			return
		}
	} else {
		a.store.Lock()
		if existing := findUserScoped(a.store.Data(), id, acc); existing != nil {
			*existing = u
		}
		a.store.Unlock()
	}

	a.store.Lock()
	a.logActivity(a.store.Data(), acc, "user", "Utilisateur "+u.Username+" modifié")
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, u)
}

func (a *API) handleUserEnable(w http.ResponseWriter, r *http.Request) {
	a.userSetStatus(w, r, "enable")
}

func (a *API) handleUserDisable(w http.ResponseWriter, r *http.Request) {
	a.userSetStatus(w, r, "disable")
}

func (a *API) userSetStatus(w http.ResponseWriter, r *http.Request, action string) {
	acc := accountScope(r)
	id := r.PathValue("id")
	a.store.Lock()
	db := a.store.Data()
	cur := findUserScoped(db, id, acc)
	if cur == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Utilisateur introuvable")
		return
	}
	username := cur.Username
	router := findRouterScoped(db, cur.RouterID, acc)
	var routerCopy *model.Router
	if router != nil {
		c := *router
		routerCopy = &c
	}
	a.store.Unlock()

	var updated *model.HotspotUser
	if routerCopy != nil && routerCopy.Mode == "agent" {
		// Mode agent : persistance + commande user_set (disabled) en file.
		a.store.Lock()
		if u := findUserScoped(a.store.Data(), id, acc); u != nil {
			if action == "enable" {
				u.Status = "active"
			} else {
				u.Status = "disabled"
			}
			c := *u
			updated = &c
		}
		if updated != nil {
			queueCommandLocked(a.store.Data(), routerCopy.AccountID, routerCopy.ID, model.CmdUserSet, map[string]any{
				"oldName":  agent.SanitizeName(username),
				"name":     agent.SanitizeName(username),
				"disabled": action == "disable",
			})
			verb := "activé"
			if action == "disable" {
				verb = "désactivé"
			}
			a.logActivity(a.store.Data(), acc, "user", "Utilisateur "+username+" "+verb+" (en attente du routeur)")
			a.store.Save()
		}
		a.store.Unlock()
		if updated == nil {
			writeErr(w, http.StatusNotFound, "Utilisateur introuvable")
			return
		}
		writeJSON(w, http.StatusOK, updated)
		return
	}
	if routerCopy != nil {
		gw := a.gatewayFor(*routerCopy)
		var err error
		if action == "enable" {
			updated, err = gw.EnableUser(id)
		} else {
			updated, err = gw.DisableUser(id)
		}
		if err != nil {
			writeErr(w, http.StatusBadRequest, "Opération impossible : "+err.Error())
			return
		}
	} else {
		a.store.Lock()
		if u := findUserScoped(a.store.Data(), id, acc); u != nil {
			if action == "enable" {
				u.Status = "active"
			} else {
				u.Status = "disabled"
			}
			c := *u
			updated = &c
		}
		a.store.Unlock()
		if updated == nil {
			writeErr(w, http.StatusNotFound, "Utilisateur introuvable")
			return
		}
	}

	a.store.Lock()
	verb := "activé"
	if action == "disable" {
		verb = "désactivé"
	}
	a.logActivity(a.store.Data(), acc, "user", "Utilisateur "+username+" "+verb)
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, updated)
}

func (a *API) handleUserDelete(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	id := r.PathValue("id")
	a.store.Lock()
	db := a.store.Data()
	cur := findUserScoped(db, id, acc)
	if cur == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Utilisateur introuvable")
		return
	}
	username := cur.Username
	router := findRouterScoped(db, cur.RouterID, acc)
	var routerCopy *model.Router
	if router != nil {
		c := *router
		routerCopy = &c
	}
	a.store.Unlock()

	if routerCopy != nil && routerCopy.Mode == "agent" {
		// Mode agent : suppression immédiate côté cloud + commande user_remove en file.
		a.store.Lock()
		a.removeUserByID(id)
		queueCommandLocked(a.store.Data(), routerCopy.AccountID, routerCopy.ID, model.CmdUserRemove,
			map[string]any{"names": []string{agent.SanitizeName(username)}})
		a.logActivity(a.store.Data(), acc, "user", "Utilisateur "+username+" supprimé (en attente du routeur)")
		a.store.Save()
		a.store.Unlock()
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}

	if routerCopy != nil {
		gw := a.gatewayFor(*routerCopy)
		if err := gw.RemoveUser(id); err != nil {
			writeErr(w, http.StatusBadRequest, "Suppression impossible : "+err.Error())
			return
		}
	} else {
		a.store.Lock()
		a.removeUserByID(id)
		a.store.Unlock()
	}

	a.store.Lock()
	db = a.store.Data()
	sessions := db.Sessions[:0]
	for _, s := range db.Sessions {
		if s.UserID != id {
			sessions = append(sessions, s)
		}
	}
	db.Sessions = sessions
	a.logActivity(db, acc, "user", "Utilisateur "+username+" supprimé")
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *API) removeUserByID(id string) {
	db := a.store.Data()
	for i := range db.HotspotUsers {
		if db.HotspotUsers[i].ID == id {
			db.HotspotUsers = append(db.HotspotUsers[:i], db.HotspotUsers[i+1:]...)
			return
		}
	}
}

// ---------------------------------------------------------------------------
// Vouchers
// ---------------------------------------------------------------------------

func (a *API) handleVouchersGenerate(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	var req struct {
		Count       int    `json:"count"`
		ProfileID   string `json:"profileId"`
		RouterID    string `json:"routerId"`
		Prefix      string `json:"prefix"`
		CodeLength  int    `json:"codeLength"`
		ResellerID  string `json:"resellerId"`
		UserMode    string `json:"userMode"`    // "" | "userpass" | "same" (mot de passe = nom)
		Charset     string `json:"charset"`     // preset model.Charset* ("" = MikCloud sûr)
		Comment     string `json:"comment"`     // commentaire libre inscrit sur le routeur
		DataQuotaMb *int   `json:"dataQuotaMb"` // nil = hériter du profil · 0 = illimité · >0 = Mo
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	if req.Count < 1 || req.Count > 500 {
		writeErr(w, http.StatusBadRequest, "Le nombre de vouchers doit être compris entre 1 et 500")
		return
	}
	codeLength := req.CodeLength
	if codeLength == 0 {
		codeLength = 5
	}
	// 4 caractères comme le User Manager MikroTik, jusqu'à 10 pour les codes
	// personnalisés ; l'alphabet choisi exclut toujours les caractères ambigus.
	if codeLength < 4 || codeLength > 10 {
		writeErr(w, http.StatusBadRequest, "La longueur du code doit être comprise entre 4 et 10")
		return
	}
	prefix := req.Prefix
	if prefix == "" {
		prefix = "SC-"
	}
	if req.UserMode != "" && req.UserMode != "userpass" && req.UserMode != "same" {
		writeErr(w, http.StatusBadRequest, "Mode utilisateur invalide (userpass ou same)")
		return
	}
	samePassword := req.UserMode == "same"
	voucherComment := sanitizeVoucherComment(req.Comment)
	now := time.Now().UTC()

	// Validation + génération des vouchers (sous verrou)
	a.store.Lock()
	db := a.store.Data()
	profile := findProfileScoped(db, strings.TrimSpace(req.ProfileID), acc)
	if profile == nil {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, "Profil introuvable")
		return
	}
	router := findRouterScoped(db, strings.TrimSpace(req.RouterID), acc)
	if router == nil {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, "Routeur introuvable")
		return
	}
	routerCopy := *router
	var resellerCopy *model.Reseller
	if strings.TrimSpace(req.ResellerID) != "" {
		res := findResellerScoped(db, strings.TrimSpace(req.ResellerID), acc)
		if res == nil {
			a.store.Unlock()
			writeErr(w, http.StatusBadRequest, "Revendeur introuvable")
			return
		}
		c := *res
		resellerCopy = &c
	}
	// Quota de données (l'argument « 5 Go = 500 F ») : par défaut le voucher
	// hérite du quota du profil ; le gérant surcharge par lot (0 = illimité
	// explicite, > 0 = Mo par voucher, plafonné à 1 Po).
	quotaMb := profile.DataQuotaMb
	if req.DataQuotaMb != nil {
		quotaMb = *req.DataQuotaMb
	}
	if quotaMb < 0 {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, "Le quota de données ne peut pas être négatif")
		return
	}
	if quotaMb > 1_073_741_824 {
		quotaMb = 1_073_741_824
	}
	cost := req.Count * profile.Price
	if resellerCopy != nil && resellerCopy.Credit < cost {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, fmt.Sprintf("Crédit insuffisant (disponible: %d, requis: %d)", resellerCopy.Credit, cost))
		return
	}
	// P0 (F13) : prix de vente (0 = même prix que le coût) et totaux marge.
	selling := profile.Price
	if profile.SellingPrice > 0 {
		selling = profile.SellingPrice
	}
	sellingTotal := selling * req.Count

	batchID := fmt.Sprintf("B%s-%04d", now.Format("20060102"), now.Nanosecond()%10000)
	expiresAt := now.Add(time.Duration(profile.ValidityDays) * 24 * time.Hour).Format(time.RFC3339)
	vouchers := make([]model.HotspotUser, 0, req.Count)
	for i := 0; i < req.Count; i++ {
		code := model.RandomCodeFrom(codeLength, req.Charset)
		for j := 0; j < 50 && usernameTaken(db, acc, prefix+code); j++ {
			code = model.RandomCodeFrom(codeLength, req.Charset)
		}
		// Mode « same » : le mot de passe = le nom d'utilisateur COMPLET (avec préfixe).
		password := prefix + code
		if !samePassword {
			// Mode « Username & Password » (défaut) : le mot de passe diffère du code.
			password = model.RandomCodeFrom(codeLength, req.Charset)
			for password == code {
				password = model.RandomCodeFrom(codeLength, req.Charset)
			}
		}
		u := model.HotspotUser{
			ID: model.NewID("v-"), AccountID: acc, Kind: "voucher", Username: prefix + code, Password: password,
			ProfileID: profile.ID, ProfileName: profile.Name,
			RouterID: routerCopy.ID, RouterName: routerCopy.Name,
			Status: "active", BatchID: batchID,
			CreatedAt: model.NowISO(), ExpiresAt: expiresAt, UsedAt: "",
			Price: profile.Price, SellingPrice: profile.SellingPrice, DataQuotaMb: int64(quotaMb),
		}
		if resellerCopy != nil {
			u.ResellerID = resellerCopy.ID
			u.ResellerName = resellerCopy.Name
		}
		vouchers = append(vouchers, u)
	}
	a.store.Unlock()

	// Application : mode agent → file de commandes ; sinon gateway (simulée/réelle).
	if routerCopy.Mode == "agent" {
		names := make([]map[string]any, 0, len(vouchers))
		for i := range vouchers {
			vouchers[i].Username = agent.SanitizeName(vouchers[i].Username)
			vouchers[i].ProfileName = agent.SanitizeName(vouchers[i].ProfileName)
			names = append(names, map[string]any{"name": vouchers[i].Username, "password": vouchers[i].Password})
		}
		a.store.Lock()
		db = a.store.Data()
		db.HotspotUsers = append(db.HotspotUsers, vouchers...)
		batchPayload := map[string]any{
			"profile": profileRef(*profile), "users": names, "batch": batchID,
		}
		if quotaMb > 0 {
			// limit-bytes-total s'exprime en octets sur le routeur (Mo × 1 048 576).
			batchPayload["limitBytesTotal"] = quotaMb * 1048576
		}
		if voucherComment != "" {
			batchPayload["comment"] = voucherComment
		}
		cmd := queueCommandLocked(db, routerCopy.AccountID, routerCopy.ID, model.CmdVoucherBatch, batchPayload)
		a.logActivity(db, acc, "voucher", fmt.Sprintf("Lot %s : %d vouchers en file pour «%s»%s (commande %s)", batchID, req.Count, routerCopy.Name, quotaNote(quotaMb), cmd.ID))
		// Bookkeeping : vente, transaction, portefeuille revendeur
		channel := "direct"
		resName := ""
		if resellerCopy != nil {
			channel = "reseller"
			resName = resellerCopy.Name
			if res := findResellerScoped(db, resellerCopy.ID, acc); res != nil {
				res.Credit -= cost
			}
			db.Transactions = append([]model.Transaction{{
				ID: model.NewID("tx-"), AccountID: acc, Type: "sale", ResellerID: resellerCopy.ID, ResellerName: resName,
				Amount: cost, Note: fmt.Sprintf("Achat de %d vouchers (%s)", req.Count, profile.Name),
				At: model.NowISO(),
			}}, db.Transactions...)
		}
		db.Sales = append(db.Sales, model.Sale{
			ID: model.NewID("sale-"), AccountID: acc, Amount: cost, ProfileName: profile.Name, Count: req.Count,
			Channel: channel, ResellerName: resName,
			RouterID: routerCopy.ID, RouterName: routerCopy.Name, BatchID: batchID,
			At: model.NowISO(), Cost: cost, SellingTotal: sellingTotal,
		})
		batchResellerID := ""
		if resellerCopy != nil {
			batchResellerID = resellerCopy.ID
		}
		db.Batches = append([]model.Batch{{
			ID: batchID, AccountID: acc, ProfileID: profile.ID, ProfileName: profile.Name,
			RouterID: routerCopy.ID, RouterName: routerCopy.Name,
			Count: req.Count, UnitPrice: profile.Price, TotalCost: cost, DataQuotaMb: int64(quotaMb),
			Channel: channel, ResellerID: batchResellerID, ResellerName: resName,
			CreatedAt: model.NowISO(),
		}}, db.Batches...)
		a.store.Save()
		cmdID := cmd.ID
		a.store.Unlock()
		data, _ := json.Marshal(map[string]any{
			"batchId": batchID, "vouchers": vouchers, "totalCost": cost,
			"queued": true, "commandId": cmdID,
			"message": "Vouchers en file : créés sur le routeur au prochain check-in (≤ 45 s)",
		})
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write(data)
		return
	}

	// Application via la gateway (simulée : écrit dans le store ; réelle : pousse au routeur)
	gw := a.gatewayFor(routerCopy)
	for i := range vouchers {
		if err := gw.AddUser(&vouchers[i]); err != nil {
			writeErr(w, http.StatusBadRequest, "Création impossible : "+err.Error())
			return
		}
	}

	// Bookkeeping : vente, transaction, portefeuille revendeur
	a.store.Lock()
	db = a.store.Data()
	channel := "direct"
	resName := ""
	if resellerCopy != nil {
		channel = "reseller"
		resName = resellerCopy.Name
		if res := findResellerScoped(db, resellerCopy.ID, acc); res != nil {
			res.Credit -= cost
		}
		db.Transactions = append([]model.Transaction{{
			ID: model.NewID("tx-"), AccountID: acc, Type: "sale", ResellerID: resellerCopy.ID, ResellerName: resName,
			Amount: cost, Note: fmt.Sprintf("Achat de %d vouchers (%s)", req.Count, profile.Name),
			At: model.NowISO(),
		}}, db.Transactions...)
	}
	db.Sales = append(db.Sales, model.Sale{
		ID: model.NewID("sale-"), AccountID: acc, Amount: cost, ProfileName: profile.Name, Count: req.Count,
		Channel: channel, ResellerName: resName,
		RouterID: routerCopy.ID, RouterName: routerCopy.Name, BatchID: batchID,
		At: model.NowISO(), Cost: cost, SellingTotal: sellingTotal,
	})
	batchResellerID := ""
	if resellerCopy != nil {
		batchResellerID = resellerCopy.ID
	}
	db.Batches = append([]model.Batch{{
		ID: batchID, AccountID: acc, ProfileID: profile.ID, ProfileName: profile.Name,
		RouterID: routerCopy.ID, RouterName: routerCopy.Name,
		Count: req.Count, UnitPrice: profile.Price, TotalCost: cost, DataQuotaMb: int64(quotaMb),
		Channel: channel, ResellerID: batchResellerID, ResellerName: resName,
		CreatedAt: model.NowISO(),
	}}, db.Batches...)
	msg := fmt.Sprintf("Génération de %d vouchers (%s)", req.Count, profile.Name)
	if resName != "" {
		msg += " pour " + resName
	}
	a.logActivity(db, acc, "voucher", msg)
	a.store.Save()
	a.store.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"batchId":   batchID,
		"vouchers":  vouchers,
		"totalCost": cost,
	})
}

func (a *API) handleVouchersBatchDelete(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	batchID := r.PathValue("batchId")
	a.store.Lock()
	db := a.store.Data()
	targets := []model.HotspotUser{}
	for _, u := range db.HotspotUsers {
		if u.AccountID == acc && u.Kind == "voucher" && u.BatchID == batchID {
			targets = append(targets, u)
		}
	}
	routerCopies := map[string]model.Router{}
	for _, u := range targets {
		if rr := findRouterScoped(db, u.RouterID, acc); rr != nil {
			if _, ok := routerCopies[rr.ID]; !ok {
				routerCopies[rr.ID] = *rr
			}
		}
	}
	a.store.Unlock()
	if len(targets) == 0 {
		writeErr(w, http.StatusNotFound, "Lot introuvable")
		return
	}

	deleted := 0
	// Routeurs agent : une commande user_remove par routeur
	for rid, rc := range routerCopies {
		if rc.Mode != "agent" {
			continue
		}
		names := []string{}
		for _, u := range targets {
			if u.RouterID == rid {
				names = append(names, agent.SanitizeName(u.Username))
			}
		}
		if len(names) == 0 {
			continue
		}
		a.store.Lock()
		queueCommandLocked(a.store.Data(), rc.AccountID, rid, model.CmdUserRemove, map[string]any{"names": names})
		a.store.Save()
		a.store.Unlock()
	}
	// Routeurs non-agent : gateway directe
	for _, rc := range routerCopies {
		if rc.Mode == "agent" {
			continue
		}
		gw := a.gatewayFor(rc)
		for _, u := range targets {
			if u.RouterID != rc.ID {
				continue
			}
			if err := gw.RemoveUser(u.ID); err != nil {
				writeErr(w, http.StatusBadRequest, "Suppression impossible : "+err.Error())
				return
			}
			deleted++
		}
	}
	// vouchers orphelins (routeur supprimé) : suppression directe
	a.store.Lock()
	db = a.store.Data()
	remaining := db.HotspotUsers[:0]
	for _, u := range db.HotspotUsers {
		if u.AccountID == acc && u.Kind == "voucher" && u.BatchID == batchID {
			deleted++
			continue
		}
		remaining = append(remaining, u)
	}
	db.HotspotUsers = remaining
	// retire aussi l'enregistrement du lot (traçabilité) — du compte seul
	batches := db.Batches[:0]
	for _, b := range db.Batches {
		if b.AccountID == acc && b.ID == batchID {
			continue
		}
		batches = append(batches, b)
	}
	db.Batches = batches
	a.logActivity(db, acc, "voucher", fmt.Sprintf("Lot %s supprimé (%d vouchers)", batchID, deleted))
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "deleted": deleted})
}

// ---------------------------------------------------------------------------
// Sessions
// ---------------------------------------------------------------------------

func (a *API) handleSessionsList(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	now := time.Now().UTC()
	a.store.Lock()
	db := a.store.Data()
	store.Tick(db, now) // fait vivre la simulation (tous comptes)
	sessions := []model.Session{}
	for _, s := range db.Sessions {
		if s.AccountID == acc {
			sessions = append(sessions, s)
		}
	}
	realRouters := []model.Router{}
	for _, rr := range db.Routers {
		if rr.Mode == "real" && rr.AccountID == acc {
			realRouters = append(realRouters, rr)
		}
	}
	a.store.Save()
	a.store.Unlock()

	// Fusion des sessions live des routeurs réels
	for _, rr := range realRouters {
		gw := a.gatewayFor(rr)
		if live, err := gw.ListSessions(); err == nil {
			sessions = append(sessions, live...)
		}
	}
	sort.Slice(sessions, func(i, j int) bool { return sessions[i].StartedAt > sessions[j].StartedAt })
	writeJSON(w, http.StatusOK, sessions)
}

func (a *API) handleSessionKick(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	id := r.PathValue("id")
	a.store.Lock()
	db := a.store.Data()
	idx := -1
	for i := range db.Sessions {
		if db.Sessions[i].ID == id && db.Sessions[i].AccountID == acc {
			idx = i
			break
		}
	}
	username := ""
	var routerCopy *model.Router
	if idx >= 0 {
		username = db.Sessions[idx].Username
		if rr := findRouterScoped(db, db.Sessions[idx].RouterID, acc); rr != nil {
			c := *rr
			routerCopy = &c
		}
	}
	realRouters := []model.Router{}
	for _, rr := range db.Routers {
		if rr.Mode == "real" && rr.AccountID == acc {
			realRouters = append(realRouters, rr)
		}
	}
	a.store.Unlock()

	if routerCopy != nil && routerCopy.Mode == "agent" {
		// Mode agent : retrait local de la session + commande kick en file.
		if idx >= 0 {
			a.store.Lock()
			db = a.store.Data()
			sessions := db.Sessions[:0]
			for _, s := range db.Sessions {
				if s.ID != id {
					sessions = append(sessions, s)
				}
			}
			db.Sessions = sessions
			queueCommandLocked(db, routerCopy.AccountID, routerCopy.ID, model.CmdKick, map[string]any{"user": agent.SanitizeName(username)})
			a.logActivity(db, acc, "session", "Session de "+username+" fermée (kick, en attente du routeur)")
			a.store.Save()
			a.store.Unlock()
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	if routerCopy != nil {
		gw := a.gatewayFor(*routerCopy)
		if err := gw.KickSession(id); err != nil {
			writeErr(w, http.StatusBadRequest, "Impossible de fermer la session : "+err.Error())
			return
		}
	} else if idx >= 0 {
		// routeur disparu : suppression directe
		a.store.Lock()
		db = a.store.Data()
		sessions := db.Sessions[:0]
		for _, s := range db.Sessions {
			if s.ID != id {
				sessions = append(sessions, s)
			}
		}
		db.Sessions = sessions
		a.store.Unlock()
	} else {
		// peut-être une session live d'un routeur réel
		kicked := false
		for _, rr := range realRouters {
			gw := a.gatewayFor(rr)
			if err := gw.KickSession(id); err == nil {
				kicked = true
				break
			}
		}
		if !kicked {
			writeErr(w, http.StatusNotFound, "Session introuvable")
			return
		}
	}

	a.store.Lock()
	msg := "Session fermée (kick)"
	if username != "" {
		msg = "Session de " + username + " fermée (kick)"
	}
	a.logActivity(a.store.Data(), acc, "session", msg)
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ---------------------------------------------------------------------------
// Revendeurs
// ---------------------------------------------------------------------------

func (a *API) handleResellersList(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	a.store.Lock()
	rs := []model.Reseller{}
	for _, res := range a.store.Data().Resellers {
		if res.AccountID == acc {
			rs = append(rs, res)
		}
	}
	a.store.Unlock()
	sort.Slice(rs, func(i, j int) bool { return rs[i].CreatedAt > rs[j].CreatedAt })
	writeJSON(w, http.StatusOK, rs)
}

func (a *API) handleResellerCreate(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	var req struct {
		Name     string `json:"name"`
		Username string `json:"username"`
		Phone    string `json:"phone"`
		Credit   int    `json:"credit"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	name := strings.TrimSpace(req.Name)
	username := strings.TrimSpace(req.Username)
	if name == "" || username == "" {
		writeErr(w, http.StatusBadRequest, "Nom et nom d'utilisateur du revendeur requis")
		return
	}
	if req.Credit < 0 {
		writeErr(w, http.StatusBadRequest, "Crédit initial invalide")
		return
	}
	a.store.Lock()
	db := a.store.Data()
	for _, res := range db.Resellers {
		if res.AccountID == acc && strings.EqualFold(res.Username, username) {
			a.store.Unlock()
			writeErr(w, http.StatusBadRequest, "Ce nom d'utilisateur revendeur existe déjà")
			return
		}
	}
	reseller := model.Reseller{
		ID: model.NewID("res-"), AccountID: acc, Name: name, Username: username, Phone: strings.TrimSpace(req.Phone),
		Credit: req.Credit, VouchersSold: 0, Revenue: 0, Status: "active", CreatedAt: model.NowISO(),
	}
	db.Resellers = append(db.Resellers, reseller)
	a.logActivity(db, acc, "reseller", "Revendeur "+reseller.Name+" créé")
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, reseller)
}

func (a *API) handleResellerUpdate(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	id := r.PathValue("id")
	var req struct {
		Name   *string `json:"name"`
		Phone  *string `json:"phone"`
		Status *string `json:"status"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	a.store.Lock()
	db := a.store.Data()
	res := findResellerScoped(db, id, acc)
	if res == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Revendeur introuvable")
		return
	}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			a.store.Unlock()
			writeErr(w, http.StatusBadRequest, "Nom du revendeur requis")
			return
		}
		res.Name = name
	}
	if req.Phone != nil {
		res.Phone = strings.TrimSpace(*req.Phone)
	}
	if req.Status != nil {
		if *req.Status != "active" && *req.Status != "disabled" {
			a.store.Unlock()
			writeErr(w, http.StatusBadRequest, "Statut invalide (active ou disabled)")
			return
		}
		res.Status = *req.Status
	}
	updated := *res
	a.logActivity(db, acc, "reseller", "Revendeur "+updated.Name+" modifié")
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, updated)
}

func (a *API) handleResellerDelete(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	id := r.PathValue("id")
	a.store.Lock()
	db := a.store.Data()
	idx := -1
	for i := range db.Resellers {
		if db.Resellers[i].ID == id && db.Resellers[i].AccountID == acc {
			idx = i
			break
		}
	}
	if idx < 0 {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Revendeur introuvable")
		return
	}
	name := db.Resellers[idx].Name
	db.Resellers = append(db.Resellers[:idx], db.Resellers[idx+1:]...)
	a.logActivity(db, acc, "reseller", "Revendeur "+name+" supprimé")
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *API) handleResellerCredit(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	id := r.PathValue("id")
	var req struct {
		Amount int    `json:"amount"`
		Note   string `json:"note"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	if req.Amount == 0 {
		writeErr(w, http.StatusBadRequest, "Le montant doit être différent de zéro")
		return
	}
	a.store.Lock()
	db := a.store.Data()
	res := findResellerScoped(db, id, acc)
	if res == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Revendeur introuvable")
		return
	}
	if req.Amount < 0 && res.Credit < -req.Amount {
		credit := res.Credit
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, fmt.Sprintf("Crédit insuffisant (disponible: %d, requis: %d)", credit, -req.Amount))
		return
	}
	res.Credit += req.Amount
	note := strings.TrimSpace(req.Note)
	if note == "" {
		if req.Amount > 0 {
			note = "Rechargement crédit"
		} else {
			note = "Débit manuel"
		}
	}
	tx := model.Transaction{
		ID: model.NewID("tx-"), AccountID: acc, Type: "credit", ResellerID: res.ID, ResellerName: res.Name,
		Amount: req.Amount, Note: note, At: model.NowISO(),
	}
	db.Transactions = append([]model.Transaction{tx}, db.Transactions...)
	if req.Amount > 0 {
		a.logActivity(db, acc, "reseller", fmt.Sprintf("Crédit de %d FCFA ajouté à %s", req.Amount, res.Name))
	} else {
		a.logActivity(db, acc, "reseller", fmt.Sprintf("Débit de %d FCFA sur %s", -req.Amount, res.Name))
	}
	updated := *res
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"reseller": updated, "transaction": tx})
}

// ---------------------------------------------------------------------------
// Transactions / Activity / Reports / Settings / Reset
// ---------------------------------------------------------------------------

func (a *API) handleTransactionsList(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	limit := queryInt(r, "limit", 20, 1, 200)
	a.store.Lock()
	txs := []model.Transaction{}
	for _, tx := range a.store.Data().Transactions {
		if tx.AccountID == acc {
			txs = append(txs, tx)
		}
	}
	a.store.Unlock()
	sort.Slice(txs, func(i, j int) bool { return txs[i].At > txs[j].At })
	if len(txs) > limit {
		txs = txs[:limit]
	}
	writeJSON(w, http.StatusOK, txs)
}

func (a *API) handleActivityList(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	limit := queryInt(r, "limit", 20, 1, 200)
	a.store.Lock()
	db := a.store.Data()
	acts := []model.Activity{}
	for _, act := range db.Activity {
		if act.AccountID == acc {
			acts = append(acts, act)
		}
	}
	a.store.Unlock()
	sort.Slice(acts, func(i, j int) bool { return acts[i].At > acts[j].At })
	if len(acts) > limit {
		acts = acts[:limit]
	}
	writeJSON(w, http.StatusOK, acts)
}

func (a *API) handleReports(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	days := 7
	if raw := r.URL.Query().Get("days"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || (v != 7 && v != 14 && v != 30) {
			writeErr(w, http.StatusBadRequest, "days doit valoir 7, 14 ou 30")
			return
		}
		days = v
	}
	now := time.Now().UTC()
	a.store.Lock()
	db := a.store.Data()

	revenueByDay := buildRevenueByDay(db, acc, now, days)

	since := now.AddDate(0, 0, -days)
	profCount := map[string]int{}
	profRevenue := map[string]int{}
	totals := struct {
		revenue int
		sales   int
	}{}
	for _, s := range db.Sales {
		if s.AccountID != acc {
			continue
		}
		at, err := time.Parse(time.RFC3339, s.At)
		if err != nil || at.Before(since) {
			continue
		}
		profCount[s.ProfileName] += s.Count
		profRevenue[s.ProfileName] += s.Amount
		totals.revenue += s.Amount
		totals.sales += s.Count
	}
	type profileSale struct {
		Name    string `json:"name"`
		Count   int    `json:"count"`
		Revenue int    `json:"revenue"`
	}
	salesByProfile := []profileSale{}
	for name, c := range profCount {
		salesByProfile = append(salesByProfile, profileSale{Name: name, Count: c, Revenue: profRevenue[name]})
	}
	sort.Slice(salesByProfile, func(i, j int) bool { return salesByProfile[i].Revenue > salesByProfile[j].Revenue })

	type trafficPoint struct {
		Day      string `json:"day"`
		BytesIn  int64  `json:"bytesIn"`
		BytesOut int64  `json:"bytesOut"`
	}
	trafficByDay := []trafficPoint{}
	todayKey := now.Format("2006-01-02")
	var todayIn, todayOut int64
	accSessions := 0
	for _, s := range db.Sessions {
		if s.AccountID != acc {
			continue
		}
		accSessions++
		todayIn += s.BytesIn
		todayOut += s.BytesOut
	}
	// La courbe synthétique de trafic est mise à l'échelle du compte
	// (~24 sessions = trafic nominal ; compte vide → trafic nul).
	trafficScale := float64(accSessions) / 24.0
	for i := days - 1; i >= 0; i-- {
		day := now.AddDate(0, 0, -i)
		rnd := rand.New(rand.NewSource(day.Unix()))
		bIn := int64(120_000_000_000) + rnd.Int63n(300_000_000_000)
		bOut := int64(15_000_000_000) + rnd.Int63n(55_000_000_000)
		bIn = int64(math.Round(float64(bIn) * trafficScale))
		bOut = int64(math.Round(float64(bOut) * trafficScale))
		if day.Format("2006-01-02") == todayKey {
			bIn += todayIn
			bOut += todayOut
		}
		trafficByDay = append(trafficByDay, trafficPoint{
			Day: fmt.Sprintf("%02d/%02d", day.Day(), int(day.Month())), BytesIn: bIn, BytesOut: bOut,
		})
	}

	voucherStatus := map[string]int{"active": 0, "used": 0, "expired": 0, "disabled": 0}
	for i := range db.HotspotUsers {
		u := &db.HotspotUsers[i]
		if u.AccountID != acc {
			continue
		}
		if u.Kind == "voucher" {
			voucherStatus[model.EffectiveStatus(u, now)]++
		}
	}
	avgTicket := 0
	if totals.sales > 0 {
		avgTicket = totals.revenue / totals.sales
	}
	// P0 (audit Mikhmon) — bloc marge (F13) : prix de vente vs coût sur
	// 30 jours glissants, par profil + totaux (cohérent avec les ventes).
	margin := buildMarginReport(db, acc, now)
	a.store.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"revenueByDay":   revenueByDay,
		"salesByProfile": salesByProfile,
		"trafficByDay":   trafficByDay,
		"voucherStatus":  voucherStatus,
		"margin":         margin,
		"totals": map[string]any{
			"revenue":   totals.revenue,
			"sales":     totals.sales,
			"avgTicket": avgTicket,
		},
	})
}

// buildMarginReport — marge par profil (F13) sur 30 jours glissants :
// revenue = total vente (SellingTotal), cost = coût (Cost), margin = écart.
// À appeler sous verrou.
func buildMarginReport(db *model.DB, acc string, now time.Time) map[string]any {
	since := now.AddDate(0, 0, -30)
	type profileMargin struct {
		Name    string `json:"name"`
		Sold    int    `json:"sold"`
		Revenue int    `json:"revenue"`
		Cost    int    `json:"cost"`
		Margin  int    `json:"margin"`
	}
	byProfile := map[string]*profileMargin{}
	totalRevenue, totalCost := 0, 0
	for i := range db.Sales {
		s := &db.Sales[i]
		if s.AccountID != acc {
			continue
		}
		at, err := time.Parse(time.RFC3339, s.At)
		if err != nil || at.Before(since) {
			continue
		}
		// Ventes antérieures à la marge : SellingTotal = 0 → base = Amount
		// (équivalent prix coût, cf. migration migrateMultiTenant).
		selling := s.SellingTotal
		if selling == 0 {
			selling = s.Amount
		}
		cost := s.Cost
		if cost == 0 {
			cost = s.Amount
		}
		pm, ok := byProfile[s.ProfileName]
		if !ok {
			pm = &profileMargin{Name: s.ProfileName}
			byProfile[s.ProfileName] = pm
		}
		pm.Sold += s.Count
		pm.Revenue += selling
		pm.Cost += cost
		pm.Margin += selling - cost
		totalRevenue += selling
		totalCost += cost
	}
	rows := make([]profileMargin, 0, len(byProfile))
	for _, pm := range byProfile {
		rows = append(rows, *pm)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Revenue != rows[j].Revenue {
			return rows[i].Revenue > rows[j].Revenue
		}
		return rows[i].Name < rows[j].Name
	})
	totalMargin := totalRevenue - totalCost
	marginPct := 0.0
	if totalRevenue > 0 {
		marginPct = math.Round(float64(totalMargin)/float64(totalRevenue)*1000) / 10
	}
	return map[string]any{
		"revenue":   totalRevenue,
		"cost":      totalCost,
		"margin":    totalMargin,
		"marginPct": marginPct,
		"byProfile": rows,
	}
}

// ---------------------------------------------------------------------------
// Comptabilité — ventes par jour / semaine / mois, par routeur (multi-sites)
// ---------------------------------------------------------------------------

// frenchMonth — noms de mois français pour les libellés du graphe mensuel.
var frenchMonth = [...]string{"janv.", "févr.", "mars", "avr.", "mai", "juin", "juil.", "août", "sept.", "oct.", "nov.", "déc."}

// handleWaveLink — deep link Wave Côte d'Ivoire par montant.
// Wave CI n'a pas d'API publique : le marchand configure son lien
// pay.wave.com dans Settings (Tenant.WaveLink) et MikCloud compose
// l'URL de paiement avec le montant demandé (ex. pour un lot de vouchers).
func (a *API) handleWaveLink(w http.ResponseWriter, r *http.Request) {
	amount, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("amount")))
	if err != nil || amount <= 0 {
		writeErr(w, http.StatusBadRequest, "Montant invalide")
		return
	}
	a.store.Lock()
	acc := accountScope(r)
	waveLink := ""
	if s, ok := a.store.Data().SettingsByAccount[acc]; ok {
		waveLink = s.Tenant.WaveLink
	}
	a.store.Unlock()
	if strings.TrimSpace(waveLink) == "" {
		writeErr(w, http.StatusConflict, "Lien marchand Wave non configuré (Settings → Lien Wave)")
		return
	}
	ref := strings.TrimSpace(r.URL.Query().Get("ref"))
	link := strings.TrimRight(waveLink, "/") + "/amount/" + strconv.Itoa(amount) + "/"
	resp := map[string]any{"amount": amount, "link": link, "currency": "FCFA"}
	if ref != "" {
		resp["ref"] = ref
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *API) handleAccounting(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	q := r.URL.Query()
	period := q.Get("period")
	if period == "" {
		period = "day"
	}
	if period != "day" && period != "week" && period != "month" {
		writeErr(w, http.StatusBadRequest, "period doit valoir day, week ou month")
		return
	}
	routerID := strings.TrimSpace(q.Get("routerId")) // "" ou "all" = tous les sites

	a.store.Lock()
	result := buildAccounting(a.store.Data(), acc, period, routerID, time.Now().UTC())
	a.store.Unlock()
	writeJSON(w, http.StatusOK, result)
}

// handleAccountingExport — export CSV (séparateur « ; », BOM UTF-8 pour Excel)
// de la comptabilité : une ligne par période + totaux + répartition par site.
func (a *API) handleAccountingExport(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	q := r.URL.Query()
	period := q.Get("period")
	if period == "" {
		period = "day"
	}
	if period != "day" && period != "week" && period != "month" {
		writeErr(w, http.StatusBadRequest, "period doit valoir day, week ou month")
		return
	}
	routerID := strings.TrimSpace(q.Get("routerId"))

	a.store.Lock()
	result := buildAccounting(a.store.Data(), acc, period, routerID, time.Now().UTC())
	tenantName := ensureSettings(a.store.Data(), acc).Tenant.Name
	a.store.Unlock()

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"mikcloud-comptabilite-%s.csv\"", period))
	// BOM UTF-8 : Excel reconnaît l'encodage.
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})
	_, _ = w.Write([]byte(fmt.Sprintf("MikCloud ; Comptabilite ;%s ;periode=%s\r\n", csvField(tenantName), period)))
	_, _ = w.Write([]byte("Periode ;Ventes ;Chiffre d'affaires (FCFA) ;Coût (FCFA) ;Marge (FCFA)\r\n"))
	type seriesRow struct {
		Label   string `json:"label"`
		Revenue int    `json:"revenue"`
		Sales   int    `json:"sales"`
		Cost    int    `json:"cost"`
		Selling int    `json:"selling"`
	}
	if raw, err := json.Marshal(result["series"]); err == nil {
		var rows []seriesRow
		if json.Unmarshal(raw, &rows) == nil {
			for _, row := range rows {
				_, _ = w.Write([]byte(fmt.Sprintf("%s ;%d ;%d ;%d ;%d\r\n",
					csvField(row.Label), row.Sales, row.Revenue, row.Cost, row.Selling-row.Cost)))
			}
		}
	}
	if totals, ok := result["totals"].(map[string]any); ok {
		cost, _ := totals["cost"].(int)
		selling, _ := totals["selling"].(int)
		_, _ = w.Write([]byte(fmt.Sprintf("TOTAL ;%v ;%v ;%d ;%d\r\n", totals["sales"], totals["revenue"], cost, selling-cost)))
	}
	_, _ = w.Write([]byte("\r\nSite ;Ventes ;Chiffre d'affaires (FCFA) ;Coût (FCFA) ;Marge (FCFA) ;Part (% )\r\n"))
	type routerRow struct {
		RouterName string  `json:"routerName"`
		Revenue    int     `json:"revenue"`
		Sales      int     `json:"sales"`
		Cost       int     `json:"cost"`
		Selling    int     `json:"selling"`
		Share      float64 `json:"share"`
	}
	if raw, err := json.Marshal(result["byRouter"]); err == nil {
		var rows []routerRow
		if json.Unmarshal(raw, &rows) == nil {
			for _, row := range rows {
				_, _ = w.Write([]byte(fmt.Sprintf("%s ;%d ;%d ;%d ;%d ;%v\r\n",
					csvField(row.RouterName), row.Sales, row.Revenue, row.Cost, row.Selling-row.Cost, row.Share)))
			}
		}
	}
}

// csvField — neutralise les séparateurs/retours à la ligne dans une cellule CSV.
func csvField(s string) string {
	if strings.ContainsAny(s, ";\"\n\r") {
		return "\"" + strings.NewReplacer("\"", "\"\"", "\n", " ", "\r", "").Replace(s) + "\""
	}
	return s
}

// buildAccounting — cœur de calcul partagé entre la réponse JSON et l'export CSV
// (l'appelant tient le verrou du store).
func buildAccounting(db *model.DB, acc, period, routerID string, now time.Time) map[string]any {
	// Découpage en buckets + fenêtre d'analyse.
	var buckets []time.Time
	var labels []string
	var windowStart time.Time

	switch period {
	case "day": // 30 derniers jours
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		windowStart = today.AddDate(0, 0, -29)
		for i := 29; i >= 0; i-- {
			d := today.AddDate(0, 0, -i)
			buckets = append(buckets, d)
			labels = append(labels, fmt.Sprintf("%02d/%02d", d.Day(), int(d.Month())))
		}
	case "week": // 12 dernières semaines (lundi → dimanche)
		wd := int(now.Weekday())
		if wd == 0 {
			wd = 7
		}
		thisMonday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -(wd - 1))
		for i := 11; i >= 0; i-- {
			monday := thisMonday.AddDate(0, 0, -7*i)
			buckets = append(buckets, monday)
			labels = append(labels, fmt.Sprintf("%02d/%02d", monday.Day(), int(monday.Month())))
		}
		windowStart = thisMonday.AddDate(0, 0, -77)
	case "month": // 12 derniers mois
		firstOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		windowStart = firstOfMonth.AddDate(0, -11, 0)
		for i := 11; i >= 0; i-- {
			m := firstOfMonth.AddDate(0, -i, 0)
			buckets = append(buckets, m)
			labels = append(labels, fmt.Sprintf("%s %02d", frenchMonth[int(m.Month())-1], m.Year()%100))
		}
	}

	type seriesPoint struct {
		Label   string `json:"label"`
		Revenue int    `json:"revenue"`
		Sales   int    `json:"sales"`
		Cost    int    `json:"cost"`    // F13 : coût agrégé
		Selling int    `json:"selling"` // F13 : total vente agrégé
	}
	revSeries := make([]seriesPoint, len(buckets))
	for i := range labels {
		revSeries[i] = seriesPoint{Label: labels[i]}
	}

	routerNames := map[string]string{}
	for _, rr := range db.Routers {
		if rr.AccountID == acc {
			routerNames[rr.ID] = rr.Name
		}
	}

	totalsRevenue, totalsSales, totalsCost, totalsSelling := 0, 0, 0, 0
	byRouterRevenue := map[string]int{}
	byRouterSales := map[string]int{}
	byRouterCost := map[string]int{}
	byRouterSelling := map[string]int{}

	for _, s := range db.Sales {
		if s.AccountID != acc {
			continue
		}
		at, err := time.Parse(time.RFC3339, s.At)
		if err != nil || at.Before(windowStart) {
			continue
		}
		// F13 : ventes antérieures à la marge → base = Amount.
		cost := s.Cost
		if cost == 0 {
			cost = s.Amount
		}
		selling := s.SellingTotal
		if selling == 0 {
			selling = s.Amount
		}

		// répartition par routeur (toujours tous sites, pour comparaison)
		rid := s.RouterID
		if rid == "" {
			rid = "unknown"
			routerNames["unknown"] = "(inconnu)"
		}
		byRouterRevenue[rid] += s.Amount
		byRouterSales[rid] += s.Count
		byRouterCost[rid] += cost
		byRouterSelling[rid] += selling

		// filtre routeur : la série et les totaux ne comptent que le site choisi
		if routerID != "" && routerID != "all" && s.RouterID != routerID {
			continue
		}
		totalsRevenue += s.Amount
		totalsSales += s.Count
		totalsCost += cost
		totalsSelling += selling

		idx := sort.Search(len(buckets), func(i int) bool { return at.Before(buckets[i]) }) - 1
		if idx >= 0 && idx < len(revSeries) {
			revSeries[idx].Revenue += s.Amount
			revSeries[idx].Sales += s.Count
			revSeries[idx].Cost += cost
			revSeries[idx].Selling += selling
		}
	}

	type routerAgg struct {
		RouterID   string  `json:"routerId"`
		RouterName string  `json:"routerName"`
		Revenue    int     `json:"revenue"`
		Sales      int     `json:"sales"`
		Cost       int     `json:"cost"`    // F13
		Selling    int     `json:"selling"` // F13
		Share      float64 `json:"share"`
	}
	byRouter := []routerAgg{}
	allSum := 0
	for _, rev := range byRouterRevenue {
		allSum += rev
	}
	for rid, rev := range byRouterRevenue {
		share := 0.0
		if allSum > 0 {
			share = math.Round(float64(rev)/float64(allSum)*1000) / 10
		}
		byRouter = append(byRouter, routerAgg{
			RouterID: rid, RouterName: routerNames[rid],
			Revenue: rev, Sales: byRouterSales[rid],
			Cost: byRouterCost[rid], Selling: byRouterSelling[rid], Share: share,
		})
	}
	sort.Slice(byRouter, func(i, j int) bool { return byRouter[i].Revenue > byRouter[j].Revenue })

	avgTicket := 0
	if totalsSales > 0 {
		avgTicket = totalsRevenue / totalsSales
	}

	return map[string]any{
		"period":   period,
		"routerId": routerID,
		"series":   revSeries,
		"byRouter": byRouter,
		"totals": map[string]any{
			"revenue":   totalsRevenue,
			"sales":     totalsSales,
			"avgTicket": avgTicket,
			// F13 : coût, total vente et marge (selling − cost).
			"cost":    totalsCost,
			"selling": totalsSelling,
			"margin":  totalsSelling - totalsCost,
		},
	}
}

// ---------------------------------------------------------------------------
// Lots de vouchers — traçabilité complète
// ---------------------------------------------------------------------------

func (a *API) handleBatchesList(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	q := r.URL.Query()
	search := strings.ToLower(strings.TrimSpace(q.Get("search")))
	routerID := strings.TrimSpace(q.Get("routerId"))
	page := queryInt(r, "page", 1, 1, 1_000_000)
	pageSize := queryInt(r, "pageSize", 15, 1, 100)

	now := time.Now().UTC()
	a.store.Lock()
	db := a.store.Data()
	a.enforceExpired(db) // P0 (audit Mikhmon) : expiration appliquée au passage

	// Statuts live des vouchers, agrégés par lot.
	type liveStats struct {
		Remaining, Active, Used, Expired, Disabled int
	}
	stats := map[string]*liveStats{}
	for i := range db.HotspotUsers {
		u := &db.HotspotUsers[i]
		if u.AccountID != acc || u.Kind != "voucher" || u.BatchID == "" {
			continue
		}
		st, ok := stats[u.BatchID]
		if !ok {
			st = &liveStats{}
			stats[u.BatchID] = st
		}
		switch model.EffectiveStatus(u, now) {
		case "active":
			st.Active++
			st.Remaining++
		case "used":
			st.Used++
		case "expired":
			st.Expired++
		case "disabled":
			st.Disabled++
		}
	}

	type batchRow struct {
		model.Batch
		Remaining int `json:"remaining"`
		Active    int `json:"active"`
		Used      int `json:"used"`
		Expired   int `json:"expired"`
		Disabled  int `json:"disabled"`
	}
	filtered := []batchRow{}
	for _, b := range db.Batches {
		if b.AccountID != acc {
			continue
		}
		if routerID != "" && routerID != "all" && b.RouterID != routerID {
			continue
		}
		if search != "" {
			hay := strings.ToLower(b.ID + " " + b.ProfileName + " " + b.RouterName + " " + b.ResellerName + " " + b.Channel)
			if !strings.Contains(hay, search) {
				continue
			}
		}
		row := batchRow{Batch: b}
		if st, ok := stats[b.ID]; ok {
			row.Remaining = st.Remaining
			row.Active = st.Active
			row.Used = st.Used
			row.Expired = st.Expired
			row.Disabled = st.Disabled
		}
		filtered = append(filtered, row)
	}
	a.store.Save() // P0 : persiste les flags Enforced déposés par enforceExpired
	a.store.Unlock()

	sort.Slice(filtered, func(i, j int) bool { return filtered[i].CreatedAt > filtered[j].CreatedAt })
	total := len(filtered)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data":     filtered[start:end],
		"total":    total,
		"page":     page,
		"pageSize": pageSize,
	})
}

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
	a.logActivity(db, acc, "system", "Paramètres du tenant mis à jour")
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, settings)
}

func (a *API) handleReset(w http.ResponseWriter, r *http.Request) {
	if !isPlatformAdmin(r) {
		writeErr(w, http.StatusForbidden, "Réservé aux administrateurs de la plateforme")
		return
	}
	a.store.Reset()
	a.store.Lock()
	a.logActivity(a.store.Data(), accountScope(r), "system", "Données de démonstration réinitialisées")
	a.store.Save()
	a.store.Unlock()
	a.clearGateways()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
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
	a.logActivity(a.store.Data(), accountScope(r), "system", "Données rechargées depuis la base persistée")
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

// handleAdminAccounts — liste tous les comptes SaaS (triés par création
// décroissante) avec le login de leur propriétaire et des statistiques d'usage.
// Réservé au rôle « admin » (super-administrateur MikCloud).
func (a *API) handleAdminAccounts(w http.ResponseWriter, r *http.Request) {
	if !isPlatformAdmin(r) {
		writeErr(w, http.StatusForbidden, "Réservé aux administrateurs de la plateforme")
		return
	}
	now := time.Now().UTC()
	cutoff30 := now.AddDate(0, 0, -30)
	a.store.Lock()
	db := a.store.Data()

	type accountStats struct {
		Users      int `json:"users"`
		Routers    int `json:"routers"`
		Sessions   int `json:"sessions"`
		Sales30d   int `json:"sales30d"`
		Revenue30d int `json:"revenue30d"`
	}
	type accountRow struct {
		ID        string       `json:"id"`
		Name      string       `json:"name"`
		Status    string       `json:"status"`
		CreatedAt string       `json:"createdAt"`
		Owner     string       `json:"owner"`
		Stats     accountStats `json:"stats"`
	}

	stats := map[string]*accountStats{}
	for i := range db.Accounts {
		stats[db.Accounts[i].ID] = &accountStats{}
	}
	// Propriétaire = premier utilisateur (owner ou admin) du compte.
	owners := map[string]string{}
	for i := range db.Users {
		u := &db.Users[i]
		if _, ok := stats[u.AccountID]; !ok {
			continue
		}
		if u.Role == "owner" || u.Role == "admin" {
			if _, seen := owners[u.AccountID]; !seen {
				owners[u.AccountID] = u.Username
			}
		}
	}
	for i := range db.HotspotUsers {
		if st, ok := stats[db.HotspotUsers[i].AccountID]; ok {
			st.Users++
		}
	}
	for i := range db.Routers {
		if st, ok := stats[db.Routers[i].AccountID]; ok {
			st.Routers++
		}
	}
	for i := range db.Sessions {
		if st, ok := stats[db.Sessions[i].AccountID]; ok {
			st.Sessions++
		}
	}
	for i := range db.Sales {
		s := db.Sales[i]
		st, ok := stats[s.AccountID]
		if !ok {
			continue
		}
		if at, err := time.Parse(time.RFC3339, s.At); err == nil && at.After(cutoff30) {
			st.Sales30d++
			st.Revenue30d += s.Amount
		}
	}
	rows := make([]accountRow, 0, len(db.Accounts))
	for i := range db.Accounts {
		acc := db.Accounts[i]
		rows = append(rows, accountRow{
			ID: acc.ID, Name: acc.Name, Status: acc.Status, CreatedAt: acc.CreatedAt,
			Owner: owners[acc.ID], Stats: *stats[acc.ID],
		})
	}
	a.store.Unlock()

	sort.Slice(rows, func(i, j int) bool { return rows[i].CreatedAt > rows[j].CreatedAt })
	writeJSON(w, http.StatusOK, rows)
}

// handleAdminAccountStatus — active ou désactive un compte SaaS. Effet immédiat :
// les tokens du compte sont bloqués par le middleware (401) et le login est
// refusé (403). Le compte principal ne peut pas être désactivé.
func (a *API) handleAdminAccountStatus(w http.ResponseWriter, r *http.Request) {
	if !isPlatformAdmin(r) {
		writeErr(w, http.StatusForbidden, "Réservé aux administrateurs de la plateforme")
		return
	}
	id := r.PathValue("id")
	var req struct {
		Status string `json:"status"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	if req.Status != "active" && req.Status != "disabled" {
		writeErr(w, http.StatusBadRequest, "Statut invalide (active ou disabled)")
		return
	}
	a.store.Lock()
	db := a.store.Data()
	var acc *model.Account
	for i := range db.Accounts {
		if db.Accounts[i].ID == id {
			acc = &db.Accounts[i]
			break
		}
	}
	if acc == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Compte introuvable")
		return
	}
	if id == model.AccountMainID {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, "Le compte principal ne peut pas être désactivé")
		return
	}
	acc.Status = req.Status
	verb := "activé"
	if req.Status == "disabled" {
		verb = "désactivé"
	}
	a.logActivity(db, accountScope(r), "system", "Compte «"+acc.Name+"» "+verb)
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
