// Package api — handlers de la vague P1 (audit Mikhmon) :
//
//   - F6  : moniteur de trafic temps réel (GET /api/routers/{id}/traffic) ;
//   - F7  : IP bindings (CRUD cloud + commandes agent ipbinding_add/set/remove) ;
//   - F8  : ping (simulé immédiat / agent via commande ping) + route générique
//     GET /api/commands/{id} (poll des commandes) ;
//   - F9  : outils routeur — DHCP leases, hôtes hotspot, cookies, journal
//     (données simulées déterministes par routeur / cache agent 120 s) ;
//   - F10 : scheduler (CRUD cloud + commandes read/scheduler_*) et
//     reboot/shutdown (cloud direct en simulé, commande agent sinon).
//
// Décision d'organisation : les handlers P1 vivent dans CE fichier dédié
// (handlers_p1.go) plutôt que dans handlers_ext.go (réservé à la vague P0) —
// les deux fichiers partent des mêmes helpers (accountScope, findRouterScoped,
// queueCommandLocked, splitAgentList…). Les routes sont enregistrées dans
// Handler() (handlers.go, bloc « P1 (audit Mikhmon) »).
//
// Matrice des modes (CONTRACT-V2 §0) : simulated ✅ partout ; agent ✅ via
// commandes servies au check-in (≤ 45 s) ; real → 400 realModeUnsupported.
package api

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math/rand"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"mikcloud/hotspot-api/internal/model"
	"mikcloud/hotspot-api/internal/store"
)

// realModeUnsupported — message unique du mode API directe (matrice §0).
const realModeUnsupported = "Non supporté en mode API directe — utilisez le mode agent"

// toolCacheWindow — durée de validité du cache d'un outil agent (F9/F10) :
// une commande read_* done depuis moins de 120 s est resservie telle quelle.
const toolCacheWindow = 120 * time.Second

// Validation (contrats F7/F8/F10).
var (
	macPattern        = regexp.MustCompile(`^([0-9A-Fa-f]{2}:){5}[0-9A-Fa-f]{2}$`)
	hostnamePattern   = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?(\.[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?)*$`)
	schedulerNamePat  = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
	schedulerInterval = regexp.MustCompile(`^[0-9]+[smhdw]$`)
)

// ---------------------------------------------------------------------------
// F6 — Trafic temps réel
// ---------------------------------------------------------------------------

// handleRouterTraffic — GET /api/routers/{id}/traffic → RouterTraffic.
// store.Tick est appelé avant lecture : la simulation vit au rythme des polls
// du front (5 s) même si personne d'autre ne consulte les sessions/dashboard.
// Routeur agent sans mesure : enveloppe vide {routerId, interfaces: [], history: []}.
func (a *API) handleRouterTraffic(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	id := r.PathValue("id")
	now := time.Now().UTC()

	a.store.Lock()
	db := a.store.Data()
	rr := findRouterScoped(db, id, acc)
	if rr == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Routeur introuvable")
		return
	}
	if rr.Mode == "real" {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, realModeUnsupported)
		return
	}
	store.Tick(db, now)
	out := model.RouterTraffic{
		RouterID: id, AccountID: acc,
		Interfaces: []model.IfaceTraffic{}, History: []model.TrafficPoint{},
	}
	for i := range db.Traffic {
		if db.Traffic[i].RouterID != id {
			continue
		}
		// Copie défensive (le rendu JSON se fait hors verrou).
		out = db.Traffic[i]
		out.Interfaces = append([]model.IfaceTraffic(nil), db.Traffic[i].Interfaces...)
		out.History = append([]model.TrafficPoint(nil), db.Traffic[i].History...)
		if out.Interfaces == nil {
			out.Interfaces = []model.IfaceTraffic{}
		}
		if out.History == nil {
			out.History = []model.TrafficPoint{}
		}
		break
	}
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, out)
}

// ---------------------------------------------------------------------------
// F7 — IP bindings
// ---------------------------------------------------------------------------

// handleIPBindingsList — GET /api/routers/{id}/ipbindings → IPBinding[].
func (a *API) handleIPBindingsList(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	id := r.PathValue("id")

	a.store.Lock()
	db := a.store.Data()
	rr := findRouterScoped(db, id, acc)
	if rr == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Routeur introuvable")
		return
	}
	if rr.Mode == "real" {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, realModeUnsupported)
		return
	}
	bindings := []model.IPBinding{}
	for _, b := range db.IPBindings {
		if b.AccountID == acc && b.RouterID == id {
			bindings = append(bindings, b)
		}
	}
	a.store.Unlock()
	writeJSON(w, http.StatusOK, bindings)
}

// handleIPBindingCreate — POST /api/routers/{id}/ipbindings
// {mac, address?, comment?, type?} : simulated → CRUD cloud (201) ; agent →
// cloud + commande ipbinding_add (201 + queued:true) ; real → 400.
func (a *API) handleIPBindingCreate(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	id := r.PathValue("id")
	var req struct {
		MAC     string `json:"mac"`
		Address string `json:"address"`
		Comment string `json:"comment"`
		Type    string `json:"type"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	mac := strings.TrimSpace(req.MAC)
	if !macPattern.MatchString(mac) {
		writeErr(w, http.StatusBadRequest, "Adresse MAC invalide (format AA:BB:CC:DD:EE:FF)")
		return
	}
	typ := strings.TrimSpace(req.Type)
	if typ == "" {
		typ = "bypassed"
	}
	if typ != "bypassed" && typ != "blocked" {
		writeErr(w, http.StatusBadRequest, "Type invalide (bypassed ou blocked)")
		return
	}
	address := strings.TrimSpace(req.Address)
	if address != "" && net.ParseIP(address) == nil {
		writeErr(w, http.StatusBadRequest, "Adresse IP invalide")
		return
	}
	comment := strings.TrimSpace(req.Comment)
	if len(comment) > 200 {
		comment = comment[:200]
	}

	a.store.Lock()
	db := a.store.Data()
	rr := findRouterScoped(db, id, acc)
	if rr == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Routeur introuvable")
		return
	}
	if rr.Mode == "real" {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, realModeUnsupported)
		return
	}
	for _, b := range db.IPBindings {
		if b.AccountID == acc && b.RouterID == id && strings.EqualFold(b.MAC, mac) {
			a.store.Unlock()
			writeErr(w, http.StatusBadRequest, "Cette adresse MAC est déjà liée sur ce routeur")
			return
		}
	}
	binding := model.IPBinding{
		ID: model.NewID("ipb-"), AccountID: acc, RouterID: id,
		MAC: strings.ToUpper(mac), Address: address, Comment: comment,
		Type: typ, Disabled: false, CreatedAt: model.NowISO(),
	}
	db.IPBindings = append(db.IPBindings, binding)
	var cmdID string
	if rr.Mode == "agent" {
		cmd := queueCommandLocked(db, acc, id, model.CmdIpbindingAdd, map[string]any{
			"mac": binding.MAC, "address": address, "comment": comment, "type": typ,
		})
		cmdID = cmd.ID
	}
	a.logActivity(db, acc, "router", "IP binding "+binding.MAC+" ("+typ+") ajouté sur «"+rr.Name+"»")
	a.store.Save()
	a.store.Unlock()

	data, _ := json.Marshal(binding)
	var out map[string]any
	_ = json.Unmarshal(data, &out)
	if cmdID != "" {
		out["queued"] = true
		out["commandId"] = cmdID
		out["message"] = "Commande envoyée — création au prochain check-in du routeur (≤ 45 s)"
	}
	writeJSON(w, http.StatusCreated, out)
}

// findIPBindingScoped — binding scopé au compte (404 s'il appartient à un autre).
func findIPBindingScoped(db *model.DB, id, acc string) *model.IPBinding {
	for i := range db.IPBindings {
		if db.IPBindings[i].ID == id && db.IPBindings[i].AccountID == acc {
			return &db.IPBindings[i]
		}
	}
	return nil
}

// handleIPBindingUpdate — PUT /api/ipbindings/{id} {disabled?, comment?, address?} :
// simulated → cloud ; agent → cloud + commande ipbinding_set ; real → 400.
func (a *API) handleIPBindingUpdate(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	id := r.PathValue("id")
	var req struct {
		Disabled *bool   `json:"disabled"`
		Comment  *string `json:"comment"`
		Address  *string `json:"address"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	if req.Address != nil && strings.TrimSpace(*req.Address) != "" && net.ParseIP(strings.TrimSpace(*req.Address)) == nil {
		writeErr(w, http.StatusBadRequest, "Adresse IP invalide")
		return
	}
	if req.Comment != nil && len(*req.Comment) > 200 {
		writeErr(w, http.StatusBadRequest, "Le commentaire doit faire au plus 200 caractères")
		return
	}

	a.store.Lock()
	db := a.store.Data()
	b := findIPBindingScoped(db, id, acc)
	if b == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "IP binding introuvable")
		return
	}
	rr := findRouterScoped(db, b.RouterID, acc)
	if rr == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Routeur introuvable")
		return
	}
	if rr.Mode == "real" {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, realModeUnsupported)
		return
	}
	payload := map[string]any{"mac": b.MAC}
	if req.Disabled != nil {
		b.Disabled = *req.Disabled
		payload["disabled"] = *req.Disabled
	}
	if req.Comment != nil {
		b.Comment = strings.TrimSpace(*req.Comment)
	}
	if req.Address != nil {
		b.Address = strings.TrimSpace(*req.Address)
		if b.Address != "" {
			payload["address"] = b.Address
		}
	}
	if rr.Mode == "agent" {
		queueCommandLocked(db, acc, rr.ID, model.CmdIpbindingSet, payload)
	}
	updated := *b
	a.logActivity(db, acc, "router", "IP binding "+updated.MAC+" modifié sur «"+rr.Name+"»")
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, updated)
}

// handleIPBindingDelete — DELETE /api/ipbindings/{id} : simulated → cloud ;
// agent → cloud + commande ipbinding_remove ; real → 400.
func (a *API) handleIPBindingDelete(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	id := r.PathValue("id")

	a.store.Lock()
	db := a.store.Data()
	idx := -1
	for i := range db.IPBindings {
		if db.IPBindings[i].ID == id && db.IPBindings[i].AccountID == acc {
			idx = i
			break
		}
	}
	if idx < 0 {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "IP binding introuvable")
		return
	}
	b := db.IPBindings[idx]
	rr := findRouterScoped(db, b.RouterID, acc)
	if rr == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Routeur introuvable")
		return
	}
	if rr.Mode == "real" {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, realModeUnsupported)
		return
	}
	db.IPBindings = append(db.IPBindings[:idx], db.IPBindings[idx+1:]...)
	if rr.Mode == "agent" {
		queueCommandLocked(db, acc, rr.ID, model.CmdIpbindingRemove, map[string]any{"mac": b.MAC})
	}
	a.logActivity(db, acc, "router", "IP binding "+b.MAC+" supprimé de «"+rr.Name+"»")
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ---------------------------------------------------------------------------
// F8 — Ping + statut de commande
// ---------------------------------------------------------------------------

// validPingTarget — IP littérale ou nom d'hôte ≤ 253 caractères.
func validPingTarget(t string) bool {
	if t == "" || len(t) > 253 {
		return false
	}
	if net.ParseIP(t) != nil {
		return true
	}
	return hostnamePattern.MatchString(t)
}

// handleRouterPing — POST /api/routers/{id}/ping {target} :
//   - simulated : réponse immédiate (8-60 ms ; ~10 % → 3/4 reçus, 25 % de perte) ;
//   - agent     : commande ping en file → {queued:true, commandId} ;
//   - real      : 400.
func (a *API) handleRouterPing(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	id := r.PathValue("id")
	var req struct {
		Target string `json:"target"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	target := strings.TrimSpace(req.Target)
	if !validPingTarget(target) {
		writeErr(w, http.StatusBadRequest, "Cible invalide (adresse IP ou nom d'hôte de 253 caractères max)")
		return
	}

	a.store.Lock()
	db := a.store.Data()
	rr := findRouterScoped(db, id, acc)
	if rr == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Routeur introuvable")
		return
	}
	if rr.Mode == "real" {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, realModeUnsupported)
		return
	}
	if rr.Mode == "agent" {
		cmd := queueCommandLocked(db, acc, id, model.CmdPing, map[string]any{"target": target})
		a.store.Save()
		cmdID := cmd.ID
		a.store.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{
			"queued": true, "commandId": cmdID,
			"message": "Ping en attente du prochain check-in du routeur (≤ 45 s)",
		})
		return
	}
	a.store.Unlock()

	// simulated — statistiques immédiates.
	received, lossPct := 4, 0
	if rand.Float64() < 0.10 {
		received, lossPct = 3, 25
	}
	minMs := 8 + rand.Intn(18) // 8-25 ms
	maxMs := minMs + 12 + rand.Intn(18)
	if maxMs > 60 {
		maxMs = 60
	}
	avgMs := (minMs + maxMs) / 2
	writeJSON(w, http.StatusOK, map[string]any{
		"queued": false, "ok": true, "target": target,
		"sent": 4, "received": received, "lossPct": lossPct,
		"minMs": minMs, "avgMs": avgMs, "maxMs": maxMs,
	})
}

// handleCommandStatus — GET /api/commands/{id} → {id, kind, status, result}
// (scopé compte ; le front poll toutes les 2 s pour un ping agent).
func (a *API) handleCommandStatus(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	id := r.PathValue("id")

	a.store.Lock()
	var out map[string]any
	for i := range a.store.Data().Commands {
		c := &a.store.Data().Commands[i]
		if c.ID != id || c.AccountID != acc {
			continue
		}
		out = map[string]any{
			"id": c.ID, "kind": c.Kind, "status": c.Status,
			"createdAt": c.CreatedAt, "sentAt": c.SentAt, "doneAt": c.DoneAt,
		}
		if c.Result != nil {
			// Copie défensive (rendu JSON hors verrou).
			res := make(map[string]any, len(c.Result))
			for k, v := range c.Result {
				res[k] = v
			}
			out["result"] = res
		} else {
			out["result"] = nil
		}
		break
	}
	a.store.Unlock()
	if out == nil {
		writeErr(w, http.StatusNotFound, "Commande introuvable")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// ---------------------------------------------------------------------------
// F9 — Outils routeur : DHCP / hôtes / cookies / journal
// ---------------------------------------------------------------------------

// Lignes servies (formes JSON strictes du contrat F9 — types du front).
type dhcpRow struct {
	IP      string `json:"ip"`
	MAC     string `json:"mac"`
	Host    string `json:"host"`
	Expires string `json:"expires"`
	Status  string `json:"status"`
}

type hostRow struct {
	MAC        string `json:"mac"`
	IP         string `json:"ip"`
	Server     string `json:"server"`
	Uptime     int64  `json:"uptime"` // secondes
	Authorized bool   `json:"authorized"`
}

type cookieRow struct {
	User    string `json:"user"`
	MAC     string `json:"mac"`
	Expires string `json:"expires"`
}

type logRow struct {
	Time    string `json:"time"`
	Topics  string `json:"topics"`
	Message string `json:"message"`
}

type schedulerRow struct {
	Name     string `json:"name"`
	Interval string `json:"interval"`
	OnEvent  string `json:"onEvent"`
	Disabled bool   `json:"disabled"`
}

// freshToolCommand — dernière commande du kind « done » depuis < 120 s
// (cache F9/F10), nil sinon. Sous verrou.
func freshToolCommand(db *model.DB, routerID, kind string, now time.Time) *model.Command {
	var best *model.Command
	for i := range db.Commands {
		c := &db.Commands[i]
		if c.RouterID != routerID || c.Kind != kind || c.Status != "done" || c.DoneAt == "" {
			continue
		}
		if t, err := time.Parse(time.RFC3339, c.DoneAt); err != nil || now.Sub(t) >= toolCacheWindow {
			continue
		}
		if best == nil || c.DoneAt > best.DoneAt {
			best = c
		}
	}
	return best
}

// pendingToolCommand — true si une commande du kind est déjà en file
// (queued|sent) : pas d'accumulation pendant que le front re-poll toutes
// les 3 s. Sous verrou.
func pendingToolCommand(db *model.DB, routerID, kind string) bool {
	for i := range db.Commands {
		c := &db.Commands[i]
		if c.RouterID == routerID && c.Kind == kind && (c.Status == "queued" || c.Status == "sent") {
			return true
		}
	}
	return false
}

// serveRouterTool — mécanique commune des outils F9 :
//
//	simulated → données générées à la volée, DÉTERMINISTES par routeur (rand
//	             seedé par hash du routerID) → {queued:false, data, updatedAt:now} ;
//	agent     → commande du kind done depuis < 120 s → Result["data"] re-splitté
//	             (splitAgentList) → {queued:false, data, updatedAt:DoneAt} ;
//	             sinon commande en file (dédupliquée) → {queued:true, data:[], updatedAt:""} ;
//	real      → 400 realModeUnsupported.
//
// Fonction libre générique : les méthodes Go ne portent pas de paramètres de type.
func serveRouterTool[T any](w http.ResponseWriter, r *http.Request, st *store.Store, kind string,
	parseRows func([][]string) []T, genRows func(*rand.Rand, *model.Router) []T) {
	acc := accountScope(r)
	id := r.PathValue("id")

	st.Lock()
	db := st.Data()
	rr := findRouterScoped(db, id, acc)
	if rr == nil {
		st.Unlock()
		writeErr(w, http.StatusNotFound, "Routeur introuvable")
		return
	}
	if rr.Mode == "real" {
		st.Unlock()
		writeErr(w, http.StatusBadRequest, realModeUnsupported)
		return
	}
	if rr.Mode == "simulated" {
		rnd := seededRouterRand(rr.ID)
		rows := genRows(rnd, rr)
		st.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{"queued": false, "data": rows, "updatedAt": model.NowISO()})
		return
	}
	// mode agent — cache puis file.
	now := time.Now().UTC()
	if fresh := freshToolCommand(db, id, kind, now); fresh != nil {
		raw, _ := fresh.Result["data"].(string)
		rows := parseRows(splitAgentList(raw))
		doneAt := fresh.DoneAt
		st.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{"queued": false, "data": rows, "updatedAt": doneAt})
		return
	}
	if !pendingToolCommand(db, id, kind) {
		queueCommandLocked(db, acc, id, kind, map[string]any{})
		st.Save()
	}
	st.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"queued": true, "data": []T{}, "updatedAt": ""})
}

// seededRouterRand — générateur déterministe par routeur (hash FNV du routerID).
func seededRouterRand(routerID string) *rand.Rand {
	h := fnv.New32a()
	_, _ = h.Write([]byte(routerID))
	return rand.New(rand.NewSource(int64(h.Sum32())))
}

// field — i-ème champ d'une entrée splitAgentList, "" si absent.
func field(e []string, i int) string {
	if len(e) > i {
		return e[i]
	}
	return ""
}

// ---- Parseurs des rapports agent (format « champ|champ|…;… », voir agent.go) ----

func parseDhcpRows(rows [][]string) []dhcpRow {
	out := make([]dhcpRow, 0, len(rows))
	for _, e := range rows {
		out = append(out, dhcpRow{
			MAC: field(e, 0), IP: field(e, 1), Host: field(e, 2),
			Expires: field(e, 3), Status: field(e, 4),
		})
	}
	return out
}

func parseHostRows(rows [][]string) []hostRow {
	out := make([]hostRow, 0, len(rows))
	for _, e := range rows {
		out = append(out, hostRow{
			MAC: field(e, 0), IP: field(e, 1), Server: field(e, 2),
			Uptime: parseRosUptime(field(e, 3)), Authorized: field(e, 4) == "true",
		})
	}
	return out
}

func parseCookieRows(rows [][]string) []cookieRow {
	out := make([]cookieRow, 0, len(rows))
	for _, e := range rows {
		out = append(out, cookieRow{User: field(e, 0), MAC: field(e, 1), Expires: field(e, 2)})
	}
	return out
}

func parseLogRows(rows [][]string) []logRow {
	out := make([]logRow, 0, len(rows))
	for _, e := range rows {
		out = append(out, logRow{Time: field(e, 0), Topics: field(e, 1), Message: field(e, 2)})
	}
	return out
}

func parseSchedulerRows(rows [][]string) []schedulerRow {
	out := make([]schedulerRow, 0, len(rows))
	for _, e := range rows {
		out = append(out, schedulerRow{
			Name: field(e, 0), Interval: field(e, 1),
			Disabled: field(e, 2) == "true", OnEvent: field(e, 3),
		})
	}
	return out
}

// ---- Générateurs simulés (déterministes par routerID) ----

// Pools plausibles (contexte hotspot FR/CI).
var (
	simClientNames = []string{
		"aminata", "yao_k", "mariam", "jean-marc", "fatou", "koffi",
		"awa", "ibrahim", "sekou", "nadia", "SC-4KD2T", "SC-9MXP2",
	}
	simHostNames = []string{
		"android-9f2a", "Galaxy-A14", "iPhone-de-Mariam", "PC-BUREAU", "TV-Salon",
		"RedmiNote12", "TECNO-SPARK", "HP-Laptop-Yao", "iPad-cuisine", "Infinix-Hot",
	}
	simLeaseExpiry = []string{"58s", "4m12s", "12m48s", "58m2s", "1h24m", "2h58m", "9m40s", "23m5s", "45s", "1h2m"}
	simCookieExpy  = []string{"1d23h", "9h48m", "3d2h", "12h5m", "22h41m"}
	simLogTopics   = []string{
		"hotspot,info,account", "hotspot,info,debug", "hotspot,debug",
		"hotspot,info", "hotspot,account,info",
	}
	simLogMessages = []string{ // %s = username
		"user %s logged in",
		"user %s logged in",
		"user %s logged out",
		"user %s logged out",
		"%s: login timed out",
		"%s: session timeout reached",
		"user %s kicked by admin",
		"user %s logged in via cookie",
		"%s: trying to log in again",
	}
)

// randMACSeeded — MAC déterministe depuis le rand seedé du routeur.
func randMACSeeded(rnd *rand.Rand) string {
	b := []byte{0x48, 0x8F, 0x5A, byte(rnd.Intn(256)), byte(rnd.Intn(256)), byte(rnd.Intn(256))}
	parts := make([]string, 6)
	for i, x := range b {
		parts[i] = fmt.Sprintf("%02X", x)
	}
	return strings.Join(parts, ":")
}

// simIPPrefix — préfixe réseau du routeur (« 10.10.10.1 » → « 10.10.10. »).
func simIPPrefix(host string) string {
	prefix := "10.10.10."
	if i := strings.LastIndexByte(host, '.'); i > 0 && net.ParseIP(host) != nil {
		prefix = host[:i+1]
	}
	return prefix
}

// genDhcpRows — 5-15 baux DHCP (contrat F9), statut majoritairement « bound ».
func genDhcpRows(rnd *rand.Rand, rr *model.Router) []dhcpRow {
	n := 5 + rnd.Intn(11)
	prefix := simIPPrefix(rr.Host)
	rows := make([]dhcpRow, 0, n)
	for i := 0; i < n; i++ {
		status := "bound"
		if rnd.Intn(8) == 0 {
			status = "waiting"
		}
		rows = append(rows, dhcpRow{
			IP:      fmt.Sprintf("%s%d", prefix, 10+i*7+rnd.Intn(5)),
			MAC:     randMACSeeded(rnd),
			Host:    simHostNames[rnd.Intn(len(simHostNames))],
			Expires: simLeaseExpiry[rnd.Intn(len(simLeaseExpiry))],
			Status:  status,
		})
	}
	return rows
}

// genHostRows — 8-20 hôtes hotspot, ~1/4 autorisés (bypassed).
func genHostRows(rnd *rand.Rand, rr *model.Router) []hostRow {
	n := 8 + rnd.Intn(13)
	prefix := simIPPrefix(rr.Host)
	rows := make([]hostRow, 0, n)
	for i := 0; i < n; i++ {
		rows = append(rows, hostRow{
			MAC:        randMACSeeded(rnd),
			IP:         fmt.Sprintf("%s%d", prefix, 20+i*6+rnd.Intn(4)),
			Server:     "hotspot1",
			Uptime:     int64(rnd.Intn(86400)),
			Authorized: rnd.Intn(4) == 0,
		})
	}
	return rows
}

// genCookieRows — 0-4 cookies hotspot.
func genCookieRows(rnd *rand.Rand, rr *model.Router) []cookieRow {
	n := rnd.Intn(5)
	rows := make([]cookieRow, 0, n)
	for i := 0; i < n; i++ {
		rows = append(rows, cookieRow{
			User:    simClientNames[rnd.Intn(len(simClientNames))],
			MAC:     randMACSeeded(rnd),
			Expires: simCookieExpy[rnd.Intn(len(simCookieExpy))],
		})
	}
	return rows
}

// genLogRows — 20 lignes de journal hotspot, plus récentes d'abord.
func genLogRows(rnd *rand.Rand, rr *model.Router) []logRow {
	const n = 20
	rows := make([]logRow, 0, n)
	t := time.Now()
	for i := 0; i < n; i++ {
		t = t.Add(-time.Duration(3+rnd.Intn(25)) * time.Second)
		user := simClientNames[rnd.Intn(len(simClientNames))]
		rows = append(rows, logRow{
			Time:    t.Format("15:04:05"),
			Topics:  simLogTopics[rnd.Intn(len(simLogTopics))],
			Message: fmt.Sprintf(simLogMessages[rnd.Intn(len(simLogMessages))], user),
		})
	}
	return rows
}

// ---- Routes outils ----

func (a *API) handleRouterDhcp(w http.ResponseWriter, r *http.Request) {
	serveRouterTool(w, r, a.store, model.CmdReadDhcp, parseDhcpRows, genDhcpRows)
}

func (a *API) handleRouterHosts(w http.ResponseWriter, r *http.Request) {
	serveRouterTool(w, r, a.store, model.CmdReadHosts, parseHostRows, genHostRows)
}

func (a *API) handleRouterCookies(w http.ResponseWriter, r *http.Request) {
	serveRouterTool(w, r, a.store, model.CmdReadCookies, parseCookieRows, genCookieRows)
}

func (a *API) handleRouterLog(w http.ResponseWriter, r *http.Request) {
	serveRouterTool(w, r, a.store, model.CmdReadLog, parseLogRows, genLogRows)
}

// ---------------------------------------------------------------------------
// F10 — Scheduler + reboot/shutdown
// ---------------------------------------------------------------------------

// handleSchedulerGet — GET /api/routers/{id}/scheduler → {queued, data, updatedAt}
// (réponse UNIFIÉE) : simulated → SchedulerTask[] du cloud ; agent → mécanique
// F9 avec read_scheduler (data = [{name, interval, onEvent, disabled}]).
func (a *API) handleSchedulerGet(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	id := r.PathValue("id")

	a.store.Lock()
	db := a.store.Data()
	rr := findRouterScoped(db, id, acc)
	if rr == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Routeur introuvable")
		return
	}
	if rr.Mode == "real" {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, realModeUnsupported)
		return
	}
	if rr.Mode == "simulated" {
		tasks := []model.SchedulerTask{}
		for _, t := range db.SchedulerTasks {
			if t.AccountID == acc && t.RouterID == id {
				tasks = append(tasks, t)
			}
		}
		a.store.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{"queued": false, "data": tasks, "updatedAt": model.NowISO()})
		return
	}
	// mode agent — cache puis file.
	now := time.Now().UTC()
	if fresh := freshToolCommand(db, id, model.CmdReadScheduler, now); fresh != nil {
		raw, _ := fresh.Result["data"].(string)
		rows := parseSchedulerRows(splitAgentList(raw))
		doneAt := fresh.DoneAt
		a.store.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{"queued": false, "data": rows, "updatedAt": doneAt})
		return
	}
	if !pendingToolCommand(db, id, model.CmdReadScheduler) {
		queueCommandLocked(db, acc, id, model.CmdReadScheduler, map[string]any{})
		a.store.Save()
	}
	a.store.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"queued": true, "data": []schedulerRow{}, "updatedAt": ""})
}

// handleSchedulerCreate — POST /api/routers/{id}/scheduler {name, interval, onEvent} :
// simulated → SchedulerTask (201) ; agent → commande scheduler_add → {queued:true}.
func (a *API) handleSchedulerCreate(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	id := r.PathValue("id")
	var req struct {
		Name     string `json:"name"`
		Interval string `json:"interval"`
		OnEvent  string `json:"onEvent"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" || len(name) > 48 || !schedulerNamePat.MatchString(name) {
		writeErr(w, http.StatusBadRequest, "Nom de tâche invalide (48 caractères max, sans espaces : A-Za-z0-9._-)")
		return
	}
	interval := strings.TrimSpace(req.Interval)
	if !schedulerInterval.MatchString(interval) {
		writeErr(w, http.StatusBadRequest, "Intervalle invalide (format RouterOS : 30s, 5m, 1h, 2d, 1w)")
		return
	}
	onEvent := strings.TrimSpace(req.OnEvent)
	if onEvent == "" || len(onEvent) > 4096 {
		writeErr(w, http.StatusBadRequest, "On-event requis (4096 caractères max)")
		return
	}

	a.store.Lock()
	db := a.store.Data()
	rr := findRouterScoped(db, id, acc)
	if rr == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Routeur introuvable")
		return
	}
	if rr.Mode == "real" {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, realModeUnsupported)
		return
	}
	if rr.Mode == "agent" {
		cmd := queueCommandLocked(db, acc, id, model.CmdSchedulerAdd, map[string]any{
			"name": name, "interval": interval, "onEvent": onEvent,
		})
		a.store.Save()
		cmdID := cmd.ID
		a.store.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{
			"queued": true, "commandId": cmdID,
			"message": "Commande envoyée — création au prochain check-in du routeur (≤ 45 s)",
		})
		return
	}
	// simulated — source cloud (un nom unique par routeur, comme RouterOS).
	for i := range db.SchedulerTasks {
		if db.SchedulerTasks[i].AccountID == acc && db.SchedulerTasks[i].RouterID == id && db.SchedulerTasks[i].Name == name {
			a.store.Unlock()
			writeErr(w, http.StatusBadRequest, "Une tâche portant ce nom existe déjà sur ce routeur")
			return
		}
	}
	task := model.SchedulerTask{
		ID: model.NewID("sch-"), AccountID: acc, RouterID: id,
		Name: name, Interval: interval, OnEvent: onEvent,
		Disabled: false, CreatedAt: model.NowISO(),
	}
	db.SchedulerTasks = append(db.SchedulerTasks, task)
	a.logActivity(db, acc, "router", "Tâche planifiée «"+name+"» ("+interval+") créée sur «"+rr.Name+"»")
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusCreated, task)
}

// handleSchedulerToggle — POST /api/routers/{id}/scheduler-toggle {name, disabled} :
// simulated → update cloud by name ; agent → commande scheduler_set.
func (a *API) handleSchedulerToggle(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	id := r.PathValue("id")
	var req struct {
		Name     string `json:"name"`
		Disabled bool   `json:"disabled"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" || len(name) > 48 {
		writeErr(w, http.StatusBadRequest, "Nom de tâche requis (48 caractères max)")
		return
	}

	a.store.Lock()
	db := a.store.Data()
	rr := findRouterScoped(db, id, acc)
	if rr == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Routeur introuvable")
		return
	}
	if rr.Mode == "real" {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, realModeUnsupported)
		return
	}
	if rr.Mode == "agent" {
		queueCommandLocked(db, acc, id, model.CmdSchedulerSet, map[string]any{
			"name": name, "disabled": req.Disabled,
		})
		a.store.Save()
		a.store.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{
			"queued":  true,
			"message": "Commande envoyée — appliquée au prochain check-in du routeur (≤ 45 s)",
		})
		return
	}
	var task *model.SchedulerTask
	for i := range db.SchedulerTasks {
		if db.SchedulerTasks[i].AccountID == acc && db.SchedulerTasks[i].RouterID == id && db.SchedulerTasks[i].Name == name {
			task = &db.SchedulerTasks[i]
			break
		}
	}
	if task == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Tâche introuvable")
		return
	}
	task.Disabled = req.Disabled
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleSchedulerRemove — POST /api/routers/{id}/scheduler-remove {name} :
// simulated → delete cloud by name ; agent → commande scheduler_remove.
func (a *API) handleSchedulerRemove(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	id := r.PathValue("id")
	var req struct {
		Name string `json:"name"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" || len(name) > 48 {
		writeErr(w, http.StatusBadRequest, "Nom de tâche requis (48 caractères max)")
		return
	}

	a.store.Lock()
	db := a.store.Data()
	rr := findRouterScoped(db, id, acc)
	if rr == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Routeur introuvable")
		return
	}
	if rr.Mode == "real" {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, realModeUnsupported)
		return
	}
	if rr.Mode == "agent" {
		queueCommandLocked(db, acc, id, model.CmdSchedulerRemove, map[string]any{"name": name})
		a.store.Save()
		a.store.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{
			"queued":  true,
			"message": "Commande envoyée — suppression au prochain check-in du routeur (≤ 45 s)",
		})
		return
	}
	idx := -1
	for i := range db.SchedulerTasks {
		if db.SchedulerTasks[i].AccountID == acc && db.SchedulerTasks[i].RouterID == id && db.SchedulerTasks[i].Name == name {
			idx = i
			break
		}
	}
	if idx < 0 {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Tâche introuvable")
		return
	}
	db.SchedulerTasks = append(db.SchedulerTasks[:idx], db.SchedulerTasks[idx+1:]...)
	a.logActivity(db, acc, "router", "Tâche planifiée «"+name+"» supprimée de «"+rr.Name+"»")
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleRouterReboot — POST /api/routers/{id}/reboot :
//   - simulated → uptimeSec=0 + sessions du routeur purgées (UserLog logout
//     pour chacune, cohérent F3) + Activity → {ok:true} ;
//   - agent     → commande reboot → {queued:true, message} ;
//   - real      → 400.
func (a *API) handleRouterReboot(w http.ResponseWriter, r *http.Request) {
	a.handleRouterPower(w, r, "reboot")
}

// handleRouterShutdown — POST /api/routers/{id}/shutdown : idem reboot.
func (a *API) handleRouterShutdown(w http.ResponseWriter, r *http.Request) {
	a.handleRouterPower(w, r, "shutdown")
}

func (a *API) handleRouterPower(w http.ResponseWriter, r *http.Request, action string) {
	acc := accountScope(r)
	id := r.PathValue("id")

	a.store.Lock()
	db := a.store.Data()
	rr := findRouterScoped(db, id, acc)
	if rr == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Routeur introuvable")
		return
	}
	if rr.Mode == "real" {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, realModeUnsupported)
		return
	}
	if rr.Mode == "agent" {
		kind := model.CmdReboot
		if action == "shutdown" {
			kind = model.CmdShutdown
		}
		cmd := queueCommandLocked(db, acc, id, kind, nil)
		a.store.Save()
		cmdID := cmd.ID
		a.store.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{
			"queued": true, "commandId": cmdID,
			"message": "Commande envoyée — " + action + " au prochain check-in du routeur (≤ 45 s)",
		})
		return
	}

	// simulated — redémarrage immédiat : uptime remis à zéro, sessions
	// coupées (journalisées logout), trafic conservé (compteurs matériels).
	rr.UptimeSec = 0
	rr.ActiveSessions = 0
	now := time.Now().UTC()
	kept := db.Sessions[:0]
	for _, s := range db.Sessions {
		if s.RouterID == id {
			logRouterUserEvent(db, rr, s, "logout", now)
			continue
		}
		kept = append(kept, s)
	}
	db.Sessions = kept
	verb := "redémarré"
	if action == "shutdown" {
		verb = "éteint"
	}
	a.logActivity(db, acc, "router", "Routeur "+rr.Name+" "+verb+" (commande console)")
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
