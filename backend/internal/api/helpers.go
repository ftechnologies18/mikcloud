// helpers.go — outils partagés : réponses JSON, décodage, finders scopés, passerelles routeur.

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"mikcloud/hotspot-api/internal/model"
	"mikcloud/hotspot-api/internal/routeros"
)

// prixMaxProfil — plafond économique des prix de profil (création/édition) :
// un prix géant combiné au débit des lots permettait un débordement
// arithmétique (cost négatif → portefeuille revendeur recrédité). Les prix
// réels de tickets vont de quelques dizaines à quelques milliers de FCFA ;
// 1 million est un plafond généreux. Voir la garde en profondeur dans
// handleVouchersGenerate.
const prixMaxProfil = 1_000_000

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

func ptrInt(i int) *int { return &i }

// logActivity ajoute une entrée en tête du journal DU COMPTE (sous verrou).
// Acteur vide = moteur interne (simulation, agent, notifications).
func (a *API) logActivity(db *model.DB, acc, typ, message string) {
	db.Activity = append([]model.Activity{{
		ID: model.NewID("act-"), AccountID: acc, Type: typ, Message: message, At: model.NowISO(),
	}}, db.Activity...)
	if len(db.Activity) > 500 {
		db.Activity = db.Activity[:500]
	}
}

// logActivityBy — journal d'AUDIT (N°7) : trace l'acteur authentifié à
// l'origine de l'action (claims du token). Repli « système » si sans token.
func (a *API) logActivityBy(r *http.Request, db *model.DB, acc, typ, message string) {
	actorID, actorName := "", ""
	if c := claimsFrom(r); c != nil {
		actorID, actorName = c.Sub, c.Name
	}
	db.Activity = append([]model.Activity{{
		ID: model.NewID("act-"), AccountID: acc, Type: typ, Message: message, At: model.NowISO(),
		ActorID: actorID, ActorName: actorName,
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
		Plan: model.Plan{Name: "Essai", MaxRouters: "1", MaxUsers: "Illimité"},
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
