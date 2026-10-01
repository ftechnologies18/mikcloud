// Package api — tests N°197 : carte « détails de connexion » de la vue
// Sessions (GET /api/users/{id}/connection-detail) + MAC des sessions en
// mode agent (6e champ du rapport read_state) + lookup OUI IEEE.
//
// Familles :
//   - macVendor : préfixes MA-L/MA-M/MA-S, séparateurs, inconnu → "" ;
//   - applyReadState : la ligne session à 6 champs pose la MAC normalisée
//     sur la session ET sur le log login du diff ;
//   - carte complète : résolution par ID, agrégat (user/statut résolu,
//     sessions live, dernière connexion, compteur 30 j, journal, MAC,
//     marque), mot de passe jamais servi ;
//   - résolution par username (Winbox-only : user hors registre cloud) ;
//   - homonymes multi-routeurs départagés par le hint routerId ;
//   - isolation multi-compte (404, aucun indice) ;
//   - RBAC : revendeur refusé (rang 2 exigé), gérant accepté.
package api

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"mikcloud/hotspot-api/internal/auth"
	"mikcloud/hotspot-api/internal/model"
	"mikcloud/hotspot-api/internal/store"
)

// TestMACVendor — lookup OUI : les trois registres, séparateurs tolérés,
// préfixe inconnu → "" (jamais une déduction risquée).
func TestMACVendor(t *testing.T) {
	cas := []struct{ mac, want string }{
		{"9C:3A:AF:11:22:33", "Samsung"}, // MA-L
		{"9c3aaf112233", "Samsung"},      // minuscules, sans séparateur
		{"3C-22-FB-AA-BB-CC", "Apple"},   // séparateurs tirets
		{"0C:73:EB:D0:00:00", "D-Link"},  // MA-M (28 bits)
		{"70:B3:D5:07:3A:11", "LiteOn"},  // MA-S (36 bits)
		{"02:00:00:00:00:00", ""},        // administré localement : inconnu
		{"", ""},                         // pas de MAC
		{"zz:zz:zz:zz:zz:zz", ""},        // pas un hexa : aucun préfixe
		{"AA:BB", ""},                    // tronquée : trop courte
	}
	for _, c := range cas {
		if got := macVendor(c.mac); got != c.want {
			t.Errorf("macVendor(%q) = %q, attendu %q", c.mac, got, c.want)
		}
	}
}

// TestApplyReadStateSessionMAC — N°197 : la ligne session à 6 champs
// (« user|ip|uptime|in|out|mac ») pose la MAC normalisée sur la session
// reconstruite ET sur le log login détecté par diff ; une ligne à 5 champs
// (routeur encore sur l'ancien script) laisse la MAC vide sans erreur.
func TestApplyReadStateSessionMAC(t *testing.T) {
	build := func(sessions string) *model.DB {
		old := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339)
		db := &model.DB{}
		db.Routers = []model.Router{{ID: "r-mac", AccountID: "acc", Name: "R", Mode: "agent", Status: "online"}}
		db.HotspotUsers = []model.HotspotUser{{
			ID: "v-mac", AccountID: "acc", RouterID: "r-mac", Username: "macuser",
			Kind: "voucher", Status: "active", CreatedAt: old,
		}}
		vals := url.Values{}
		vals.Set("users", "macuser|default|false;")
		vals.Set("sessions", sessions)
		(&API{}).applyReadState(db, &db.Routers[0], vals)
		return db
	}

	// Nouveau script (6 champs) : MAC normalisée partout où elle vit.
	db := build("macuser|10.5.0.7|2m|1024|2048|9c:3a:af:11:22:33;")
	if len(db.Sessions) != 1 {
		t.Fatalf("1 session attendue, obtenu %d", len(db.Sessions))
	}
	if db.Sessions[0].MAC != "9C:3A:AF:11:22:33" {
		t.Errorf("MAC session = %q, attendu la forme normalisée 9C:3A:AF:11:22:33", db.Sessions[0].MAC)
	}
	found := false
	for _, l := range db.UserLogs {
		if l.Action == "login" && l.Username == "macuser" {
			found = true
			if l.MAC != "9C:3A:AF:11:22:33" {
				t.Errorf("MAC du log login = %q, attendu 9C:3A:AF:11:22:33", l.MAC)
			}
		}
	}
	if !found {
		t.Fatal("le login de la nouvelle session doit être journalisé")
	}

	// Ancien script (5 champs) : MAC vide, aucune erreur — rétrocompatible.
	dbOld := build("macuser|10.5.0.8|1m|512|256;")
	if len(dbOld.Sessions) != 1 {
		t.Fatalf("ancien format : 1 session attendue, obtenu %d", len(dbOld.Sessions))
	}
	if dbOld.Sessions[0].MAC != "" {
		t.Errorf("ancien format : MAC attendue vide, obtenu %q", dbOld.Sessions[0].MAC)
	}
}

// seedDetailVoucher — le ticket témoin de la carte : vendu par un revendeur,
// quota data+temps résolus, cumuls non nuls, session live d'un Samsung.
func seedDetailVoucher(t *testing.T, st *store.Store, accID, routerID string) {
	t.Helper()
	now := time.Now().UTC()
	old := func(d time.Duration) string { return now.Add(-d).Format(time.RFC3339) }
	st.Lock()
	defer st.Unlock()
	db := st.Data()
	db.HotspotUsers = append(db.HotspotUsers, model.HotspotUser{
		ID: "v-carte", AccountID: accID, RouterID: routerID, RouterName: "PA-Yopougon",
		Kind: "voucher", Username: "tk4321", Password: "SECRET-A-CACHER", ProfileName: "5go-1h",
		Status: "used", CreatedAt: old(72 * time.Hour), UsedAt: old(70 * time.Hour),
		ExpiresAt:   now.Add(30 * 24 * time.Hour).Format(time.RFC3339),
		DataQuotaMb: 5120, TimeLimitMin: 120,
		BytesIn: 4096, BytesOut: 1048576, UptimeUsedSec: 1800,
		ResellerID: "rs-carte", ResellerName: "Awa", Price: 500,
		SoldAt: old(71 * time.Hour), SoldVia: "sell_mode",
	})
	db.Sessions = append(db.Sessions, model.Session{
		ID: "s-carte", AccountID: accID, UserID: "v-carte", Username: "tk4321",
		RouterID: routerID, RouterName: "PA-Yopougon", IP: "10.5.0.7",
		MAC: "F8:D0:BD:11:22:33", StartedAt: old(10 * time.Minute), UptimeSec: 600,
		BytesIn: 1024, BytesOut: 204800,
	})
	db.UserLogs = append(db.UserLogs,
		model.UserLog{ID: "ul-1", AccountID: accID, UserID: "v-carte", Username: "tk4321", Action: "login", RouterID: routerID, IP: "10.5.0.7", MAC: "F8:D0:BD:11:22:33", At: old(45 * 24 * time.Hour)},
		model.UserLog{ID: "ul-2", AccountID: accID, UserID: "v-carte", Username: "tk4321", Action: "login", RouterID: routerID, IP: "10.5.0.7", MAC: "F8:D0:BD:11:22:33", At: old(70 * time.Hour)},
		model.UserLog{ID: "ul-3", AccountID: accID, UserID: "v-carte", Username: "tk4321", Action: "logout", RouterID: routerID, IP: "10.5.0.7", MAC: "F8:D0:BD:11:22:33", At: old(48 * time.Hour)},
		model.UserLog{ID: "ul-4", AccountID: accID, UserID: "v-carte", Username: "tk4321", Action: "login", RouterID: routerID, IP: "10.5.0.7", MAC: "F8:D0:BD:11:22:33", At: old(1 * time.Hour)},
	)
	st.Save()
}

// TestUserConnectionDetailFull — la carte complète depuis une session :
// résolution par ID, agrégat exact, mot de passe jamais servi, statut
// résolu « online » (session live), marque Samsung depuis l'OUI.
func TestUserConnectionDetailFull(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	token, accID, _ := registerAccount(t, ts, "gerant-carte-full", "")
	routerID := seedRouterInAccount(t, st, accID, "PA-Yopougon")
	seedDetailVoucher(t, st, accID, routerID)

	status, out := doJSON(t, ts, "GET", "/api/users/v-carte/connection-detail?routerId="+routerID, token, nil)
	if status != http.StatusOK {
		t.Fatalf("GET connection-detail : statut %d, corps %v", status, out)
	}
	user, _ := out["user"].(map[string]any)
	if user == nil {
		t.Fatal("l'utilisateur du registre doit être servi")
	}
	if pw, _ := user["password"].(string); pw != "" {
		t.Errorf("le mot de passe ne doit JAMAIS être servi, obtenu %q", pw)
	}
	if u, _ := user["username"].(string); u != "tk4321" {
		t.Errorf("username = %q, attendu tk4321", u)
	}
	if s, _ := user["status"].(string); s != "online" {
		t.Errorf("statut résolu = %q, attendu online (session live)", s)
	}
	if q, _ := user["dataQuotaMb"].(float64); q != 5120 {
		t.Errorf("dataQuotaMb = %v, attendu 5120", q)
	}
	if r, _ := user["resellerName"].(string); r != "Awa" {
		t.Errorf("resellerName = %q, attendu Awa", r)
	}
	if bi, _ := user["bytesIn"].(float64); bi != 4096 {
		t.Errorf("bytesIn = %v, attendu 4096", bi)
	}
	if bo, _ := user["bytesOut"].(float64); bo != 1048576 {
		t.Errorf("bytesOut = %v, attendu 1048576", bo)
	}
	live, _ := out["liveSessions"].([]any)
	if len(live) != 1 {
		t.Fatalf("1 session live attendue, obtenu %d", len(live))
	}
	if mac, _ := out["mac"].(string); mac != "F8:D0:BD:11:22:33" {
		t.Errorf("mac = %q, attendu F8:D0:BD:11:22:33", mac)
	}
	if brand, _ := out["deviceBrand"].(string); brand != "Samsung" {
		t.Errorf("deviceBrand = %q, attendu Samsung", brand)
	}
	if n, _ := out["loginCount30d"].(float64); n != 2 {
		t.Errorf("loginCount30d = %v, attendu 2 (logins à 70 h et 1 h ; celui à 45 j hors fenêtre)", n)
	}
	logs, _ := out["recentLogs"].([]any)
	if len(logs) != 4 {
		t.Fatalf("4 logs attendus, obtenu %d", len(logs))
	}
	first, _ := logs[0].(map[string]any)
	if id, _ := first["id"].(string); id != "ul-4" {
		t.Errorf("recentLogs[0].id = %q, attendu ul-4 (récents d'abord)", id)
	}
	// Dernière connexion = le StartedAt de la session live (10 min), plus
	// récent que le dernier login journalisé (1 h).
	lastLogin, _ := out["lastLoginAt"].(string)
	if lastLogin == "" {
		t.Fatal("lastLoginAt attendu")
	}
	startedAt := time.Now().UTC().Add(-10 * time.Minute)
	got, err := time.Parse(time.RFC3339, lastLogin)
	if err != nil {
		t.Fatalf("lastLoginAt non RFC3339 : %v", err)
	}
	if d := got.Sub(startedAt); d < -time.Minute || d > time.Minute {
		t.Errorf("lastLoginAt = %v, attendu ~%v", lastLogin, startedAt.Format(time.RFC3339))
	}
}

// TestUserConnectionDetailByUsernameWinboxOnly — utilisateur créé
// directement dans Winbox (hors registre cloud, userId vide sur la
// session) : résolution par username, carte SANS section ticket mais AVEC
// la connexion, le journal et la MAC du dernier log (un Apple).
func TestUserConnectionDetailByUsernameWinboxOnly(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	token, accID, _ := registerAccount(t, ts, "gerant-carte-winbox", "")
	routerID := seedRouterInAccount(t, st, accID, "PA-Cocody")
	now := time.Now().UTC()
	st.Lock()
	st.Data().Sessions = append(st.Data().Sessions, model.Session{
		ID: "s-wbx", AccountID: accID, Username: "winboxguy",
		RouterID: routerID, RouterName: "PA-Cocody", IP: "10.6.0.9",
		StartedAt: now.Add(-5 * time.Minute).Format(time.RFC3339), UptimeSec: 300,
	})
	st.Data().UserLogs = append(st.Data().UserLogs, model.UserLog{
		ID: "ul-wbx", AccountID: accID, Username: "winboxguy", Action: "login",
		RouterID: routerID, IP: "10.6.0.9", MAC: "3C:22:FB:AA:BB:CC",
		At: now.Add(-24 * time.Hour).Format(time.RFC3339),
	})
	st.Save()
	st.Unlock()

	status, out := doJSON(t, ts, "GET", "/api/users/winboxguy/connection-detail?routerId="+routerID, token, nil)
	if status != http.StatusOK {
		t.Fatalf("GET par username : statut %d, corps %v", status, out)
	}
	if user, _ := out["user"].(map[string]any); user != nil {
		t.Errorf("user attendu null (hors registre cloud), obtenu %v", user)
	}
	live, _ := out["liveSessions"].([]any)
	if len(live) != 1 {
		t.Fatalf("1 session live attendue, obtenu %d", len(live))
	}
	if mac, _ := out["mac"].(string); mac != "3C:22:FB:AA:BB:CC" {
		t.Errorf("mac = %q, attendu celle du dernier log", mac)
	}
	if brand, _ := out["deviceBrand"].(string); brand != "Apple" {
		t.Errorf("deviceBrand = %q, attendu Apple", brand)
	}
	if n, _ := out["loginCount30d"].(float64); n != 1 {
		t.Errorf("loginCount30d = %v, attendu 1", n)
	}
	if lastLogin, _ := out["lastLoginAt"].(string); lastLogin == "" {
		t.Error("lastLoginAt attendu (StartedAt de la session live)")
	}
}

// TestUserConnectionDetailHomonymes — un même username sur deux routeurs du
// compte : le hint routerId de la ligne cliquée désigne LE bon utilisateur,
// la session du homonyme ne pollue pas la carte.
func TestUserConnectionDetailHomonymes(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	token, accID, _ := registerAccount(t, ts, "gerant-carte-homonymes", "")
	rA := seedRouterInAccount(t, st, accID, "PA-Riviera")
	rB := seedRouterInAccount(t, st, accID, "PA-Plateau")
	st.Lock()
	st.Data().HotspotUsers = append(st.Data().HotspotUsers,
		model.HotspotUser{ID: "v-a", AccountID: accID, RouterID: rA, Username: "codememe", Kind: "voucher", Status: "used", BytesOut: 111, CreatedAt: "2026-01-01T00:00:00Z"},
		model.HotspotUser{ID: "v-b", AccountID: accID, RouterID: rB, Username: "codememe", Kind: "voucher", Status: "used", BytesOut: 222, CreatedAt: "2026-01-02T00:00:00Z"},
	)
	st.Data().Sessions = append(st.Data().Sessions,
		model.Session{ID: "s-a", AccountID: accID, UserID: "v-a", Username: "codememe", RouterID: rA, StartedAt: "2026-09-30T10:00:00Z"},
		model.Session{ID: "s-b", AccountID: accID, UserID: "v-b", Username: "codememe", RouterID: rB, StartedAt: "2026-09-30T11:00:00Z"},
	)
	st.Save()
	st.Unlock()

	status, out := doJSON(t, ts, "GET", "/api/users/codememe/connection-detail?routerId="+rA, token, nil)
	if status != http.StatusOK {
		t.Fatalf("GET homonymes : statut %d, corps %v", status, out)
	}
	user, _ := out["user"].(map[string]any)
	if id, _ := user["id"].(string); id != "v-a" {
		t.Errorf("le hint routeur doit désigner v-a, obtenu %q", id)
	}
	live, _ := out["liveSessions"].([]any)
	if len(live) != 1 {
		t.Fatalf("1 session (celle du routeur du hint), obtenu %d", len(live))
	}
	s, _ := live[0].(map[string]any)
	if sid, _ := s["id"].(string); sid != "s-a" {
		t.Errorf("session attendue s-a, obtenu %q", sid)
	}
}

// TestUserConnectionDetailIsolation — un compte ne voit RIEN de l'autre :
// ni par ID ni par username (404 sec, aucun indice d'existence).
func TestUserConnectionDetailIsolation(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	tokenA, _, _ := registerAccount(t, ts, "gerant-carte-iso-a", "")
	// Compte B : inscription directe avec un téléphone DISTINCT (garde
	// anti-abus multi-comptes — miroir handlers_sites_test.go).
	statusB, outB := doJSON(t, ts, "POST", "/api/auth/register", "", map[string]string{
		"name": "Gérant B", "username": "gerant-carte-iso-b", "password": "mot-de-passe-8+",
		"email": "gerant-carte-iso-b@example.ci", "phone": "0707070708", "country": "CI", "city": "Abidjan",
	})
	if statusB != http.StatusCreated {
		t.Fatalf("inscription compte B : statut %d, corps %v", statusB, outB)
	}
	tokenB, _ := outB["token"].(string)
	userB, _ := outB["user"].(map[string]any)
	accB, _ := userB["accountId"].(string)
	routerB := seedRouterInAccount(t, st, accB, "PA-Adjamé")
	seedDetailVoucher(t, st, accB, routerB)

	for _, path := range []string{
		"/api/users/v-carte/connection-detail",
		"/api/users/tk4321/connection-detail",
		"/api/users/tk4321/connection-detail?routerId=" + routerB,
	} {
		status, out := doJSON(t, ts, "GET", path, tokenA, nil)
		if status != http.StatusNotFound {
			t.Errorf("GET %s depuis le compte A : statut %d, attendu 404 (corps %v)", path, status, out)
		}
	}
	// Le compte B voit sa carte normalement.
	status, _ := doJSON(t, ts, "GET", "/api/users/v-carte/connection-detail?routerId="+routerB, tokenB, nil)
	if status != http.StatusOK {
		t.Errorf("GET depuis le compte B : statut %d, attendu 200", status)
	}
}

// TestUserConnectionDetailRBAC — la carte est un geste de gérant /
// propriétaire (rang 2) : un token revendeur est refusé, un gérant passe.
func TestUserConnectionDetailRBAC(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	ownerToken, accID, _ := registerAccount(t, ts, "gerant-carte-rbac", "")
	routerID := seedRouterInAccount(t, st, accID, "PA-Marcory")
	seedDetailVoucher(t, st, accID, routerID)

	// Propriétaire (rang 3) : la carte s'ouvre.
	if status, _ := doJSON(t, ts, "GET", "/api/users/v-carte/connection-detail", ownerToken, nil); status != http.StatusOK {
		t.Errorf("propriétaire : statut %d, attendu 200", status)
	}

	seedSellReseller(t, st, "usr-res-carte", accID, "Res Carte", "prepaid", 0)
	resToken := auth.Sign(testJWTSecret, auth.NewClaims("usr-res-carte", "R", "reseller", accID, 0))
	req, _ := http.NewRequest("GET", ts.URL+"/api/users/v-carte/connection-detail", nil)
	req.Header.Set("Authorization", "Bearer "+resToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("requête revendeur : %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("revendeur : statut %d, attendu 403", resp.StatusCode)
	}

	seedUser(t, st, "usr-m-carte", accID, "usr-m-carte", model.RoleManager)
	mgrToken := auth.Sign(testJWTSecret, auth.NewClaims("usr-m-carte", "M", model.RoleManager, accID, 0))
	req, _ = http.NewRequest("GET", ts.URL+"/api/users/v-carte/connection-detail", nil)
	req.Header.Set("Authorization", "Bearer "+mgrToken)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("requête gérant : %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("gérant : statut %d, attendu 200", resp.StatusCode)
	}
}

// TestUserConnectionDetailNotFound — un identifiant inconnu du compte ne
// correspondant à aucune session ni log : 404.
func TestUserConnectionDetailNotFound(t *testing.T) {
	_, ts := newTestServerWithStore(t)
	token, _, _ := registerAccount(t, ts, "gerant-carte-404", "")
	status, _ := doJSON(t, ts, "GET", "/api/users/inconnu-total/connection-detail", token, nil)
	if status != http.StatusNotFound {
		t.Errorf("utilisateur inconnu : statut %d, attendu 404", status)
	}
}
