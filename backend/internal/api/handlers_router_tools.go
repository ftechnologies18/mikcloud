// Package api — outils routeur (F9, audit Mikhmon) : DHCP leases, hôtes
// hotspot, cookies, journal, ressources. Données simulées déterministes par
// routeur, ou cache d'un rapport agent (read_*) frais de toolCacheWindow
// (120 s) — CONTRACT-V2 §0.
package api

import (
	"fmt"
	"hash/fnv"
	"math/rand"
	"mikcloud/hotspot-api/internal/model"
	"mikcloud/hotspot-api/internal/store"
	"net"
	"net/http"
	"strings"
	"time"
)

// toolCacheWindow — durée de validité du cache d'un outil agent (F9/F10) :
// une commande read_* done depuis moins de 120 s est resservie telle quelle.
const toolCacheWindow = 120 * time.Second

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

// resourcesRow — Parité Mikhmon : une ressource routeur utile aux formulaires
// (pool d'adresses, file parent, serveur hotspot). Les listes alimentent les
// sélecteurs address-pool / parent-queue du profil et « serveur » du lot.
type resourcesRow struct {
	Kind string `json:"kind"` // pool | queue | server
	Name string `json:"name"`
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

// parseResourcesRows — rapport agent « kind|name;… » : on ne garde que les
// kinds connus (tolérance aux versions d'agent plus récentes).
func parseResourcesRows(rows [][]string) []resourcesRow {
	out := make([]resourcesRow, 0, len(rows))
	for _, e := range rows {
		kind, name := field(e, 0), field(e, 1)
		if name == "" {
			continue
		}
		switch kind {
		case "pool", "queue", "server":
			out = append(out, resourcesRow{Kind: kind, Name: name})
		}
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

// genResourcesRows — ressources simulées déterministes (structure stable :
// 3 pools, 2 files parent, 1 serveur hotspot) pour la démo sans matériel.
func genResourcesRows(rnd *rand.Rand, rr *model.Router) []resourcesRow {
	_ = rnd
	_ = rr
	return []resourcesRow{
		{Kind: "pool", Name: "pool-hotspot"},
		{Kind: "pool", Name: "pool-vip"},
		{Kind: "pool", Name: "pool-invites"},
		{Kind: "queue", Name: "q-parent-all"},
		{Kind: "queue", Name: "q-vip"},
		{Kind: "server", Name: "hotspot1"},
	}
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

// handleRouterResources — Parité Mikhmon : pools / files / serveurs hotspot
// (l'équivalent des listes déroulantes « Address Pool » et « Parent Queue »
// du formulaire Mikhmon).
func (a *API) handleRouterResources(w http.ResponseWriter, r *http.Request) {
	serveRouterTool(w, r, a.store, model.CmdReadResources, parseResourcesRows, genResourcesRows)
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
