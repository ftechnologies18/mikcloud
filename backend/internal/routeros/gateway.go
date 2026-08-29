// Package routeros — abstraction Gateway : le même contrat pour un routeur
// simulé (via le store) et un routeur MikroTik réel (protocole binaire 8728).
package routeros

import (
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"time"

	"mikcloud/hotspot-api/internal/model"
	"mikcloud/hotspot-api/internal/store"
)

// TestResult — résultat d'un test de connexion (latence + version).
type TestResult struct {
	LatencyMs int64
	Version   string
}

// SystemStats — télémétrie d'un routeur.
type SystemStats struct {
	CPULoad       int
	MemUsedPct    int
	FreeMemoryMb  int64
	TotalMemoryMb int64
	UptimeSec     int64
	Version       string
}

// Gateway — abstraction d'un routeur hotspot MikroTik.
type Gateway interface {
	TestConnection() (TestResult, error)
	ListUsers() ([]model.HotspotUser, error)
	AddUser(u *model.HotspotUser) error
	SetUser(u *model.HotspotUser, oldUsername string, passwordChanged bool) error
	RemoveUser(id string) error
	EnableUser(id string) (*model.HotspotUser, error)
	DisableUser(id string) (*model.HotspotUser, error)
	ListSessions() ([]model.Session, error)
	KickSession(id string) error
	SystemStats() (SystemStats, error)
	Close() error
}

// TestRouter teste une connexion RouterOS (TCP + login + version), sans gateway persistant.
func TestRouter(host string, port int, username, password string) (TestResult, error) {
	start := time.Now()
	c, err := Dial(host, port, username, password, 5*time.Second)
	if err != nil {
		return TestResult{}, err
	}
	defer c.Close()
	rows, _, err := c.Run("/system/resource/print")
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return TestResult{LatencyMs: latency}, fmt.Errorf("commande /system/resource/print échouée : %w", err)
	}
	version := ""
	if len(rows) > 0 {
		version = rows[0]["version"]
	}
	return TestResult{LatencyMs: latency, Version: version}, nil
}

// ---------------------------------------------------------------------------
// Helpers communs (sous verrou store)
// ---------------------------------------------------------------------------

func dbRouter(db *model.DB, id string) *model.Router {
	for i := range db.Routers {
		if db.Routers[i].ID == id {
			return &db.Routers[i]
		}
	}
	return nil
}

func dbUser(db *model.DB, id string) *model.HotspotUser {
	for i := range db.HotspotUsers {
		if db.HotspotUsers[i].ID == id {
			return &db.HotspotUsers[i]
		}
	}
	return nil
}

func dbUserIndexByUsername(db *model.DB, username string) int {
	for i := range db.HotspotUsers {
		if db.HotspotUsers[i].Username == username {
			return i
		}
	}
	return -1
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// ---------------------------------------------------------------------------
// Simulateur — routeur simulé : les opérations touchent réellement le store.
// ---------------------------------------------------------------------------

// Simulator — gateway simulée branchée sur le store (routeurs mode "simulated").
type Simulator struct {
	store    *store.Store
	routerID string
}

// NewSimulator construit une gateway simulée pour le routeur donné.
func NewSimulator(s *store.Store, routerID string) *Simulator {
	return &Simulator{store: s, routerID: routerID}
}

// TestConnection — latence simulée 2-45 ms, version du routeur.
func (g *Simulator) TestConnection() (TestResult, error) {
	latency := 2 + rand.Intn(44) // 2 à 45 ms
	g.store.Lock()
	defer g.store.Unlock()
	r := dbRouter(g.store.Data(), g.routerID)
	if r == nil {
		return TestResult{}, fmt.Errorf("routeur introuvable")
	}
	r.Status = "online"
	return TestResult{LatencyMs: int64(latency), Version: r.Version + " (stable)"}, nil
}

// ListUsers — utilisateurs hotspot du routeur (simulés).
func (g *Simulator) ListUsers() ([]model.HotspotUser, error) {
	g.store.Lock()
	defer g.store.Unlock()
	out := []model.HotspotUser{}
	for _, u := range g.store.Data().HotspotUsers {
		if u.RouterID == g.routerID {
			out = append(out, u)
		}
	}
	return out, nil
}

// AddUser ajoute réellement l'utilisateur au store.
func (g *Simulator) AddUser(u *model.HotspotUser) error {
	g.store.Lock()
	defer g.store.Unlock()
	db := g.store.Data()
	if dbRouter(db, g.routerID) == nil {
		return fmt.Errorf("routeur introuvable")
	}
	db.HotspotUsers = append(db.HotspotUsers, *u)
	return nil
}

// SetUser remplace l'enregistrement local.
func (g *Simulator) SetUser(u *model.HotspotUser, oldUsername string, passwordChanged bool) error {
	g.store.Lock()
	defer g.store.Unlock()
	db := g.store.Data()
	existing := dbUser(db, u.ID)
	if existing == nil {
		return fmt.Errorf("utilisateur introuvable")
	}
	*existing = *u
	return nil
}

// RemoveUser supprime l'utilisateur du store.
func (g *Simulator) RemoveUser(id string) error {
	g.store.Lock()
	defer g.store.Unlock()
	db := g.store.Data()
	for i := range db.HotspotUsers {
		if db.HotspotUsers[i].ID == id {
			db.HotspotUsers = append(db.HotspotUsers[:i], db.HotspotUsers[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("utilisateur introuvable")
}

// EnableUser passe le statut local à "active".
func (g *Simulator) EnableUser(id string) (*model.HotspotUser, error) {
	return g.setStatus(id, "active")
}

// DisableUser passe le statut local à "disabled".
func (g *Simulator) DisableUser(id string) (*model.HotspotUser, error) {
	return g.setStatus(id, "disabled")
}

func (g *Simulator) setStatus(id, status string) (*model.HotspotUser, error) {
	g.store.Lock()
	defer g.store.Unlock()
	u := dbUser(g.store.Data(), id)
	if u == nil {
		return nil, fmt.Errorf("utilisateur introuvable")
	}
	u.Status = status
	out := *u
	return &out, nil
}

// ListSessions — sessions actives du routeur (les plus récentes d'abord).
func (g *Simulator) ListSessions() ([]model.Session, error) {
	g.store.Lock()
	defer g.store.Unlock()
	out := []model.Session{}
	for _, s := range g.store.Data().Sessions {
		if s.RouterID == g.routerID {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt > out[j].StartedAt })
	return out, nil
}

// KickSession supprime la session du store.
func (g *Simulator) KickSession(id string) error {
	g.store.Lock()
	defer g.store.Unlock()
	db := g.store.Data()
	for i := range db.Sessions {
		if db.Sessions[i].ID == id {
			db.Sessions = append(db.Sessions[:i], db.Sessions[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("session introuvable")
}

// SystemStats — télémétrie plausible : CPU random walk 5-45 %, RAM stable par routeur.
func (g *Simulator) SystemStats() (SystemStats, error) {
	g.store.Lock()
	defer g.store.Unlock()
	r := dbRouter(g.store.Data(), g.routerID)
	if r == nil {
		return SystemStats{}, fmt.Errorf("routeur introuvable")
	}
	cpu := clampInt(r.CPULoad+rand.Intn(11)-5, 5, 45)
	r.CPULoad = cpu
	total := int64(memTotalMb(g.routerID))
	memPct := 25 + rand.Intn(41) // 25-65 %
	free := total * int64(100-memPct) / 100
	return SystemStats{
		CPULoad:       cpu,
		MemUsedPct:    memPct,
		FreeMemoryMb:  free,
		TotalMemoryMb: total,
		UptimeSec:     r.UptimeSec,
		Version:       r.Version,
	}, nil
}

// Close — rien à fermer.
func (g *Simulator) Close() error { return nil }

func memTotalMb(routerID string) int {
	sum := 0
	for i := 0; i < len(routerID); i++ {
		sum += int(routerID[i])
	}
	switch sum % 4 {
	case 0:
		return 64
	case 1:
		return 128
	case 2:
		return 256
	default:
		return 512
	}
}

// ---------------------------------------------------------------------------
// Client réel — MikroTik via le protocole binaire (port 8728).
// ---------------------------------------------------------------------------

// RealGateway — gateway vers un routeur MikroTik réel : pousse les opérations
// sur l'équipement et maintient un miroir local dans le store.
type RealGateway struct {
	mu       sync.Mutex
	store    *store.Store
	routerID string
	client   *Client
	live     map[string]model.Session // sessions vues au dernier ListSessions (id RouterOS -> session)
}

// NewRealGateway construit la gateway réelle (connexion paresseuse au premier appel).
func NewRealGateway(s *store.Store, routerID string) *RealGateway {
	return &RealGateway{store: s, routerID: routerID, live: map[string]model.Session{}}
}

func (g *RealGateway) creds() (host string, port int, username, password string, err error) {
	g.store.Lock()
	defer g.store.Unlock()
	r := dbRouter(g.store.Data(), g.routerID)
	if r == nil {
		return "", 0, "", "", fmt.Errorf("routeur introuvable")
	}
	return r.Host, r.Port, r.Username, r.Password, nil
}

// withClient exécute fn sur la connexion (reconnecte si nécessaire).
func (g *RealGateway) withClient(fn func(c *Client) error) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.client == nil {
		host, port, user, pass, err := g.creds()
		if err != nil {
			return err
		}
		c, err := Dial(host, port, user, pass, 5*time.Second)
		if err != nil {
			return err
		}
		g.client = c
	}
	if err := fn(g.client); err != nil {
		_ = g.client.Close()
		g.client = nil
		return err
	}
	return nil
}

// TestConnection mesure TCP + login + lecture de la version (connexion jetable).
func (g *RealGateway) TestConnection() (TestResult, error) {
	host, port, user, pass, err := g.creds()
	if err != nil {
		return TestResult{}, err
	}
	return TestRouter(host, port, user, pass)
}

// ListUsers lit /ip/hotspot/user/print et fait la jointure avec le store.
func (g *RealGateway) ListUsers() ([]model.HotspotUser, error) {
	var rows []map[string]string
	err := g.withClient(func(c *Client) error {
		var e error
		rows, _, e = c.Run("/ip/hotspot/user/print")
		return e
	})
	if err != nil {
		return nil, err
	}
	g.store.Lock()
	defer g.store.Unlock()
	db := g.store.Data()
	routerName := ""
	if r := dbRouter(db, g.routerID); r != nil {
		routerName = r.Name
	}
	byName := map[string]int{}
	for i := range db.HotspotUsers {
		byName[db.HotspotUsers[i].Username] = i
	}
	out := []model.HotspotUser{}
	for _, row := range rows {
		name := row["name"]
		if name == "" {
			continue
		}
		if idx, ok := byName[name]; ok {
			u := db.HotspotUsers[idx]
			if row["disabled"] == "true" {
				u.Status = "disabled"
			}
			u.BytesIn = parseInt64(row["bytes-in"])
			u.BytesOut = parseInt64(row["bytes-out"])
			out = append(out, u)
			continue
		}
		status := "active"
		if row["disabled"] == "true" {
			status = "disabled"
		}
		out = append(out, model.HotspotUser{
			ID:          "ros-" + strings.TrimPrefix(row[".id"], "*"),
			Kind:        "regular",
			Username:    name,
			Password:    row["password"],
			ProfileName: row["profile"],
			RouterID:    g.routerID,
			RouterName:  routerName,
			Status:      status,
			Comment:     row["comment"],
			BytesIn:     parseInt64(row["bytes-in"]),
			BytesOut:    parseInt64(row["bytes-out"]),
			CreatedAt:   model.NowISO(),
		})
	}
	return out, nil
}

// AddUser pousse /ip/hotspot/user/add puis enregistre le miroir local.
func (g *RealGateway) AddUser(u *model.HotspotUser) error {
	words := []string{"/ip/hotspot/user/add", "=name=" + u.Username, "=password=" + u.Password, "=profile=" + u.ProfileName}
	if u.Comment != "" {
		words = append(words, "=comment="+u.Comment)
	}
	if err := g.withClient(func(c *Client) error { return c.Exec(words...) }); err != nil {
		return err
	}
	g.store.Lock()
	defer g.store.Unlock()
	g.store.Data().HotspotUsers = append(g.store.Data().HotspotUsers, *u)
	return nil
}

// SetUser retrouve l'utilisateur par ancien nom puis pousse /ip/hotspot/user/set.
func (g *RealGateway) SetUser(u *model.HotspotUser, oldUsername string, passwordChanged bool) error {
	err := g.withClient(func(c *Client) error {
		rows, _, err := c.Run("/ip/hotspot/user/print", "?name="+oldUsername)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return fmt.Errorf("utilisateur %s introuvable sur le routeur", oldUsername)
		}
		words := []string{"/ip/hotspot/user/set", "=.id=" + rows[0][".id"], "=name=" + u.Username, "=profile=" + u.ProfileName}
		if u.Comment != "" {
			words = append(words, "=comment="+u.Comment)
		}
		if passwordChanged {
			words = append(words, "=password="+u.Password)
		}
		return c.Exec(words...)
	})
	if err != nil {
		return err
	}
	g.store.Lock()
	defer g.store.Unlock()
	if existing := dbUser(g.store.Data(), u.ID); existing != nil {
		*existing = *u
	}
	return nil
}

func (g *RealGateway) rosUserOp(id, action string) error {
	g.store.Lock()
	u := dbUser(g.store.Data(), id)
	var username string
	if u != nil {
		username = u.Username
	}
	g.store.Unlock()
	if username == "" {
		return fmt.Errorf("utilisateur introuvable")
	}
	return g.withClient(func(c *Client) error {
		rows, _, err := c.Run("/ip/hotspot/user/print", "?name="+username)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return fmt.Errorf("utilisateur %s introuvable sur le routeur", username)
		}
		return c.Exec("/ip/hotspot/user/"+action, "=.id="+rows[0][".id"])
	})
}

// RemoveUser pousse /ip/hotspot/user/remove puis supprime le miroir local.
func (g *RealGateway) RemoveUser(id string) error {
	if err := g.rosUserOp(id, "remove"); err != nil {
		return err
	}
	g.store.Lock()
	defer g.store.Unlock()
	db := g.store.Data()
	for i := range db.HotspotUsers {
		if db.HotspotUsers[i].ID == id {
			db.HotspotUsers = append(db.HotspotUsers[:i], db.HotspotUsers[i+1:]...)
			return nil
		}
	}
	return nil
}

// EnableUser pousse /ip/hotspot/user/enable et met à jour le miroir.
func (g *RealGateway) EnableUser(id string) (*model.HotspotUser, error) {
	return g.setRemoteStatus(id, "enable", "active")
}

// DisableUser pousse /ip/hotspot/user/disable et met à jour le miroir.
func (g *RealGateway) DisableUser(id string) (*model.HotspotUser, error) {
	return g.setRemoteStatus(id, "disable", "disabled")
}

func (g *RealGateway) setRemoteStatus(id, action, status string) (*model.HotspotUser, error) {
	if err := g.rosUserOp(id, action); err != nil {
		return nil, err
	}
	g.store.Lock()
	defer g.store.Unlock()
	u := dbUser(g.store.Data(), id)
	if u == nil {
		return nil, fmt.Errorf("utilisateur introuvable")
	}
	u.Status = status
	out := *u
	return &out, nil
}

// ListSessions lit /ip/hotspot/active/print et le mappe en sessions.
func (g *RealGateway) ListSessions() ([]model.Session, error) {
	var rows []map[string]string
	err := g.withClient(func(c *Client) error {
		var e error
		rows, _, e = c.Run("/ip/hotspot/active/print")
		return e
	})
	if err != nil {
		return nil, err
	}
	g.store.Lock()
	db := g.store.Data()
	routerName := ""
	if r := dbRouter(db, g.routerID); r != nil {
		routerName = r.Name
	}
	byName := map[string]model.HotspotUser{}
	for i := range db.HotspotUsers {
		if db.HotspotUsers[i].RouterID == g.routerID {
			byName[db.HotspotUsers[i].Username] = db.HotspotUsers[i]
		}
	}
	g.store.Unlock()

	now := time.Now().UTC()
	sessions := []model.Session{}
	live := map[string]model.Session{}
	for _, row := range rows {
		uptime := parseUptime(row["uptime"])
		var userID, profileName string
		if u, ok := byName[row["user"]]; ok {
			userID, profileName = u.ID, u.ProfileName
		}
		s := model.Session{
			ID:          row[".id"],
			UserID:      userID,
			Username:    row["user"],
			ProfileName: profileName,
			RouterID:    g.routerID,
			RouterName:  routerName,
			IP:          row["address"],
			MAC:         row["mac-address"],
			StartedAt:   now.Add(-time.Duration(uptime) * time.Second).Format(time.RFC3339),
			UptimeSec:   uptime,
			BytesIn:     parseInt64(row["bytes-in"]),
			BytesOut:    parseInt64(row["bytes-out"]),
		}
		sessions = append(sessions, s)
		live[s.ID] = s
	}
	sort.Slice(sessions, func(i, j int) bool { return sessions[i].StartedAt > sessions[j].StartedAt })
	g.mu.Lock()
	g.live = live
	g.mu.Unlock()
	return sessions, nil
}

// KickSession supprime la session active côté routeur (/ip/hotspot/active/remove).
func (g *RealGateway) KickSession(id string) error {
	g.mu.Lock()
	_, known := g.live[id]
	g.mu.Unlock()
	if !known {
		return fmt.Errorf("session inconnue sur ce routeur")
	}
	return g.withClient(func(c *Client) error {
		return c.Exec("/ip/hotspot/active/remove", "=.id="+id)
	})
}

// SystemStats lit /system/resource/print.
func (g *RealGateway) SystemStats() (SystemStats, error) {
	var rows []map[string]string
	err := g.withClient(func(c *Client) error {
		var e error
		rows, _, e = c.Run("/system/resource/print")
		return e
	})
	if err != nil {
		return SystemStats{}, err
	}
	if len(rows) == 0 {
		return SystemStats{}, fmt.Errorf("réponse vide du routeur")
	}
	row := rows[0]
	total := parseInt64(row["total-memory"])
	free := parseInt64(row["free-memory"])
	memPct := 0
	if total > 0 {
		memPct = int((total - free) * 100 / total)
	}
	return SystemStats{
		CPULoad:       int(parseInt64(row["cpu-load"])),
		MemUsedPct:    memPct,
		FreeMemoryMb:  free / 1048576,
		TotalMemoryMb: total / 1048576,
		UptimeSec:     parseUptime(row["uptime"]),
		Version:       row["version"],
	}, nil
}

// Close coupe la connexion en cache.
func (g *RealGateway) Close() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.client != nil {
		err := g.client.Close()
		g.client = nil
		return err
	}
	return nil
}
