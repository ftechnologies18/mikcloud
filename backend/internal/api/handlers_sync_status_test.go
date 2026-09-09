// Tests N°71 — GET /api/admin/sync-status : double garde (rang 3 + rôle
// plateforme), contrat JSON complet en mode local (json) et photographie des
// agents (fraîcheur des check-ins, file de commandes, zombies).
package api

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"mikcloud/hotspot-api/internal/auth"
	"mikcloud/hotspot-api/internal/model"
	"mikcloud/hotspot-api/internal/store"
)

// seedSyncStatusRouter — 1 routeur AGENT en ligne (check-in frais) + 3
// commandes : 1 « queued », 1 « sent » récente, 1 « sent » ZOMBIE (servie
// depuis plus de staleSentLimit = 10 min). Sous verrou, comme le veut le
// store.
func seedSyncStatusRouter(t *testing.T, st *store.Store, accID string) {
	t.Helper()
	st.Lock()
	defer st.Unlock()
	db := st.Data()
	now := time.Now().UTC()
	stale := now.Add(-20 * time.Minute).Format(time.RFC3339)
	db.Routers = append(db.Routers, model.Router{
		ID:        "rt-sync",
		AccountID: accID,
		Name:      "Agent diagnostic",
		Host:      "10.10.10.10",
		Port:      8728,
		Mode:      "agent",
		Status:    "online",
		LastSeen:  model.NowISO(),
	})
	db.Commands = append(db.Commands,
		model.Command{ID: "cmd-queued", RouterID: "rt-sync", AccountID: accID, Kind: "read_state", Status: "queued", CreatedAt: model.NowISO()},
		model.Command{ID: "cmd-sent", RouterID: "rt-sync", AccountID: accID, Kind: "ping", Status: "sent", CreatedAt: model.NowISO(), SentAt: model.NowISO()},
		model.Command{ID: "cmd-zombie", RouterID: "rt-sync", AccountID: accID, Kind: "read_state", Status: "sent", CreatedAt: stale, SentAt: stale},
	)
	st.Save()
}

// TestSyncStatusRoleMatrix — l'endpoint est réservé à l'admin plateforme :
// 401 sans jeton, 403 pour un owner de compte client, 403 rôle insuffisant
// pour un manager, 200 pour le super-admin plateforme.
func TestSyncStatusRoleMatrix(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	ownerToken, accID, _ := registerAccount(t, ts, "proprio-sync", "")

	seedUser(t, st, "usr-mgr-sync", accID, "usr-mgr-sync", model.RoleManager)
	seedUser(t, st, "usr-plat-sync", "", "usr-plat-sync", model.RolePlatformAdmin)
	manager := auth.Sign(testJWTSecret, auth.NewClaims("usr-mgr-sync", "Manager", model.RoleManager, accID, 0))
	platform := auth.Sign(testJWTSecret, auth.NewClaims("usr-plat-sync", "Plateforme", model.RolePlatformAdmin, "", 0))

	// Sans jeton : le middleware coupe avant le handler (401).
	if status, _ := doJSON(t, ts, "GET", "/api/admin/sync-status", "", nil); status != http.StatusUnauthorized {
		t.Fatalf("sans jeton : 401 attendu, obtenu %d", status)
	}
	// Owner d'un compte CLIENT (rang 3 mais pas plateforme) : 403 réservé.
	status, out := doJSON(t, ts, "GET", "/api/admin/sync-status", ownerToken, nil)
	if status != http.StatusForbidden {
		t.Fatalf("owner client : 403 attendu, obtenu %d", status)
	}
	if !strings.Contains(fmt.Sprint(out["error"]), "Réservé aux administrateurs") {
		t.Fatalf("message inattendu : %v", out["error"])
	}
	// Manager (rang 2) : refusé par requireRole avant la garde plateforme.
	status, out = doJSON(t, ts, "GET", "/api/admin/sync-status", manager, nil)
	if status != http.StatusForbidden {
		t.Fatalf("manager : 403 attendu, obtenu %d", status)
	}
	if !strings.Contains(fmt.Sprint(out["error"]), "rôle insuffisant") {
		t.Fatalf("message inattendu : %v", out["error"])
	}
	// Super-admin plateforme : accès.
	if status, _ := doJSON(t, ts, "GET", "/api/admin/sync-status", platform, nil); status != http.StatusOK {
		t.Fatalf("plateforme : 200 attendu, obtenu %d", status)
	}
}

// TestSyncStatusContract — en mode JSON (tests sans DATABASE_URL) :
// mode="json", blocs sync/neon null, 30 tables avec les lignes mémoire,
// et la photographie agents cohérente avec le seed (1 agent en ligne,
// file 1/2 dont 1 zombie).
func TestSyncStatusContract(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	_, accID, _ := registerAccount(t, ts, "proprio-contrat", "")
	seedUser(t, st, "usr-plat-contrat", "", "usr-plat-contrat", model.RolePlatformAdmin)
	platform := auth.Sign(testJWTSecret, auth.NewClaims("usr-plat-contrat", "Plateforme", model.RolePlatformAdmin, "", 0))
	seedSyncStatusRouter(t, st, accID)

	status, out := doJSON(t, ts, "GET", "/api/admin/sync-status", platform, nil)
	if status != http.StatusOK {
		t.Fatalf("200 attendu, obtenu %d (%v)", status, out)
	}
	if out["mode"] != "json" {
		t.Fatalf("mode attendu « json », obtenu %v", out["mode"])
	}
	if out["sync"] != nil {
		t.Fatalf("mode JSON : sync doit être null, obtenu %v", out["sync"])
	}
	if out["neon"] != nil {
		t.Fatalf("mode JSON : neon doit être null, obtenu %v", out["neon"])
	}

	// Tables : 30 entrées, lignes mémoire exactes pour le seed.
	tables, ok := out["tables"].([]any)
	if !ok || len(tables) != 30 {
		t.Fatalf("30 tables attendues, obtenu %v", len(tables))
	}
	findTable := func(name string) map[string]any {
		for _, e := range tables {
			if m, ok := e.(map[string]any); ok && m["table"] == name {
				return m
			}
		}
		return nil
	}
	for _, tc := range []struct {
		table string
		rows  float64
	}{
		{"routers", 1},
		{"commands", 3},
		{"accounts", 1},
		{"geniuspay_subs", 0},
	} {
		entry := findTable(tc.table)
		if entry == nil {
			t.Fatalf("table absente de la réponse : %s", tc.table)
		}
		if entry["rows"] != tc.rows {
			t.Fatalf("table %s : %v lignes attendues, obtenu %v", tc.table, tc.rows, entry["rows"])
		}
	}

	// Agents : 1 routeur agent en ligne (check-in frais), file 1 en attente,
	// 2 « sent » dont 1 zombie (> 10 min), dernier check-in renseigné.
	agents, ok := out["agents"].(map[string]any)
	if !ok {
		t.Fatalf("bloc agents absent : %v", out["agents"])
	}
	for _, tc := range []struct {
		key  string
		want float64
	}{
		{"routers", 1},
		{"routersAgent", 1},
		{"routersOnline", 1},
		{"routersConflict", 0},
		{"commandsQueued", 1},
		{"commandsSent", 2},
		{"commandsStale", 1},
	} {
		if agents[tc.key] != tc.want {
			t.Fatalf("agents.%s : %v attendu, obtenu %v", tc.key, tc.want, agents[tc.key])
		}
	}
	if s, _ := agents["lastCheckIn"].(string); s == "" {
		t.Fatalf("lastCheckIn attendu (routeur fraîchement vu), obtenu %v", agents["lastCheckIn"])
	}

	// Bande passante (N°72) : 5 catégories canoniques dans l'ordre, la
	// requête en cours est déjà comptée en « console » (startRequest AVANT
	// le handler) — le rapport se mesure lui-même.
	bw, ok := out["bandwidth"].(map[string]any)
	if !ok {
		t.Fatalf("bloc bandwidth absent : %v", out["bandwidth"])
	}
	bwCats, ok := bw["categories"].([]any)
	if !ok || len(bwCats) != 5 {
		t.Fatalf("5 catégories attendues, obtenu %v", bwCats)
	}
	if first, _ := bwCats[0].(map[string]any); first["name"] != "agents" {
		t.Fatalf("ordre canonique attendu (agents en tête), obtenu %v", bwCats[0])
	}
	if bw["totalRequests"].(float64) < 1 {
		t.Fatalf("la requête en cours doit être comptée : %v", bw["totalRequests"])
	}
	for _, e := range bwCats {
		if m, ok := e.(map[string]any); ok && m["name"] == "console" {
			if m["requests"].(float64) < 1 {
				t.Fatalf("catégorie console non comptée : %v", m)
			}
		}
	}
}

// TestSyncStatusAgentOffline — un agent vu il y a plus d'OnlineWindow
// (3 min) n'est PAS compté en ligne : la fraîcheur du check-in est la
// vérité, pas le champ Status posé au dernier passage.
func TestSyncStatusAgentOffline(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	_, accID, _ := registerAccount(t, ts, "proprio-offline", "")
	seedUser(t, st, "usr-plat-off", "", "usr-plat-off", model.RolePlatformAdmin)
	platform := auth.Sign(testJWTSecret, auth.NewClaims("usr-plat-off", "Plateforme", model.RolePlatformAdmin, "", 0))

	st.Lock()
	db := st.Data()
	db.Routers = append(db.Routers, model.Router{
		ID: "rt-ghost", AccountID: accID, Name: "Agent fantôme", Mode: "agent", Status: "online",
		LastSeen: time.Now().UTC().Add(-30 * time.Minute).Format(time.RFC3339),
	})
	st.Save()
	st.Unlock()

	status, out := doJSON(t, ts, "GET", "/api/admin/sync-status", platform, nil)
	if status != http.StatusOK {
		t.Fatalf("200 attendu, obtenu %d", status)
	}
	agents, _ := out["agents"].(map[string]any)
	if agents == nil {
		t.Fatal("bloc agents absent")
	}
	if agents["routersAgent"] != float64(1) || agents["routersOnline"] != float64(0) {
		t.Fatalf("agent fantôme : 1 agent / 0 en ligne attendus, obtenu %v / %v",
			agents["routersAgent"], agents["routersOnline"])
	}
}
