package api

// Tests N°151 — la vraie boîte de notifications : read-state SERVEUR par
// utilisateur. L'ancien badge vivait dans un localStorage PAR NAVIGATEUR
// (clé globale, incohérente multi-appareils et entre membres d'une équipe) ;
// il vit désormais dans AdminUser.ActivitySeenAt : GET /api/bell renvoie
// items (filtrés RBAC) + seenAt + unread, POST /api/bell/seen acquitte
// (monotone, borné au futur, migration de l'ancien localStorage acceptée).

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"mikcloud/hotspot-api/internal/model"
)

// bellOf — GET /api/bell décodé.
func bellOf(t *testing.T, ts *httptest.Server, token string) bellResponse {
	t.Helper()
	req, _ := http.NewRequest("GET", ts.URL+"/api/bell?limit=20", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept-Encoding", "identity")
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("GET /api/bell : %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("GET /api/bell : statut %d", resp.StatusCode)
	}
	var out bellResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("décodage /api/bell : %v", err)
	}
	return out
}

// postBellSeen — POST /api/bell/seen avec corps optionnel.
func postBellSeen(t *testing.T, ts *httptest.Server, token string, body any) (int, map[string]any) {
	t.Helper()
	return doJSON(t, ts, "POST", "/api/bell/seen", token, body)
}

// TestBellSeenPerUserIndependent — deux membres d'une même équipe ont des
// boîtes INDÉPENDANTES : l'acquit du propriétaire n'éponge pas le badge du
// gérant (et réciproquement).
func TestBellSeenPerUserIndependent(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	ownerToken, accID, _ := registerAccount(t, ts, "bell-owner", "")

	// Journal AUX HORODATAGES CONTRÔLÉS (la granularité de NowISO est la
	// seconde : un test de read-state ne peut pas semer « maintenant » et
	// attendre un compte exact de non-lus) : deux entrées lues d'il y a
	// 3 min, une entrée non lue d'il y a 1 min.
	st.Lock()
	db := st.Data()
	model.AppendActivity(db, model.Activity{AccountID: accID, Type: "router",
		At:      time.Now().UTC().Add(-3 * time.Minute).Format(time.RFC3339),
		Message: "Routeur «Plateau» hors ligne — sans check-in depuis 4m0s"})
	model.AppendActivity(db, model.Activity{AccountID: accID, Type: "system",
		At:      time.Now().UTC().Add(-3 * time.Minute).Format(time.RFC3339),
		Message: "Session support ouverte — la plateforme consulte la console de ce compte"})
	st.Save()
	st.Unlock()

	// Ajout d'un gérant (l'entrée team qui en naît ne lui sera pas visible).
	status, _ := doJSON(t, ts, "POST", "/api/team", ownerToken, map[string]string{
		"name": "Vendeuse", "username": "bell.manager", "password": "mot-de-passe-3", "role": "manager",
	})
	if status != 201 {
		t.Fatalf("création du gérant : statut %d", status)
	}
	loginStatus, out := doJSON(t, ts, "POST", "/api/auth/login", "", map[string]string{
		"username": "bell.manager", "password": "mot-de-passe-3",
	})
	if loginStatus != 200 {
		t.Fatalf("login gérant : statut %d", loginStatus)
	}
	managerToken, _ := out["token"].(string)

	// Read-state initial posé directement dans le store (déterministe :
	// NowISO et seenAt partagent la granularité de la SECONDE — poser une
	// entrée « après » un acquit HTTP du même instant serait une course).
	// Les deux membres ont lu il y a 2 minutes ; une entrée arrive il y a
	// 1 minute : non lue pour les deux.
	past := time.Now().UTC().Add(-2 * time.Minute).Format(time.RFC3339)
	st.Lock()
	for i := range st.Data().Users {
		if st.Data().Users[i].AccountID == accID {
			st.Data().Users[i].ActivitySeenAt = past
		}
	}
	model.AppendActivity(st.Data(), model.Activity{AccountID: accID, Type: "user",
		At:      time.Now().UTC().Add(-time.Minute).Format(time.RFC3339),
		Message: "Voucher «MT-bell» vendu par la plateforme"})
	st.Save()
	st.Unlock()

	// Chacun voit des non-lus (l'entrée d'il y a 1 min est postérieure à
	// leur read-state commun). Le propriétaire en voit un de PLUS que le
	// gérant : l'entrée team « Membre ajouté » lui est réservée (RBAC
	// N°149 — la boîte du gérant est filtrée, c'est voulu).
	ownerUnread := bellOf(t, ts, ownerToken).Unread
	if ownerUnread == 0 {
		t.Fatal("owner : des non-lus sont attendus (entrée posée après son read-state)")
	}
	managerUnread := bellOf(t, ts, managerToken).Unread
	if managerUnread == 0 || managerUnread > ownerUnread {
		t.Fatalf("manager : non-lus attendus (≤ owner, filtre RBAC), %d obtenus (owner %d)", managerUnread, ownerUnread)
	}

	// Le gérant acquitte : SON badge tombe à zéro, celui du propriétaire ne
	// bouge PAS — l'acquit de l'un n'éponge pas la boîte de l'autre.
	if s, _ := postBellSeen(t, ts, managerToken, nil); s != 200 {
		t.Fatalf("acquit manager : statut %d", s)
	}
	if b := bellOf(t, ts, managerToken); b.Unread != 0 {
		t.Fatalf("manager après acquit : 0 non lu attendu, %d obtenu", b.Unread)
	}
	if b := bellOf(t, ts, ownerToken); b.Unread != ownerUnread {
		t.Fatalf("owner : son badge ne doit pas bouger (%d non lus), %d obtenus", ownerUnread, b.Unread)
	}
}

// TestBellSeenMonotoneAndBounded — un acquit ANCIEN (migration localStorage
// en retard) ne rouvre pas les non-lus ; un horodatage FUTUR est borné à
// maintenant ; un format invalide est refusé.
func TestBellSeenMonotoneAndBounded(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	ownerToken, accID, _ := registerAccount(t, ts, "bell-mono", "")
	seedActivityForRBAC(st, accID)

	// Acquit courant.
	if s, _ := postBellSeen(t, ts, ownerToken, nil); s != 200 {
		t.Fatalf("acquit : statut %d", s)
	}
	current := bellOf(t, ts, ownerToken).SeenAt
	if current == "" {
		t.Fatal("acquit : seenAt attendu")
	}

	// Acquit ANCIEN (localStorage périmé d'un autre navigateur) : monotone,
	// le seenAt ne recule PAS.
	old := "2020-01-01T00:00:00Z"
	if s, out := postBellSeen(t, ts, ownerToken, map[string]string{"at": old}); s != 200 {
		t.Fatalf("acquit ancien : statut %d", s)
	} else if got, _ := out["seenAt"].(string); got != current {
		t.Fatalf("acquit ancien : seenAt ne doit pas reculer (%q → %q)", current, got)
	}

	// Acquit FUTUR : borné à maintenant (jamais plus de ~maintenant).
	future := "2099-01-01T00:00:00Z"
	if s, out := postBellSeen(t, ts, ownerToken, map[string]string{"at": future}); s != 200 {
		t.Fatalf("acquit futur : statut %d", s)
	} else if got, _ := out["seenAt"].(string); got >= future {
		t.Fatalf("acquit futur : doit être borné à maintenant, obtenu %q", got)
	}

	// Format invalide : 400.
	if s, _ := postBellSeen(t, ts, ownerToken, map[string]string{"at": "hier"}); s != 400 {
		t.Fatalf("horodatage invalide : 400 attendu, %d obtenu", s)
	}
}

// TestBellUnreadCountsBeyondLimit — le badge compte TOUT le journal visible,
// pas seulement la page demandée : 25 entrées neuves, limit=20 → 25 non-lus
// (badge « 9+ » honnête).
func TestBellUnreadCountsBeyondLimit(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	ownerToken, accID, _ := registerAccount(t, ts, "bell-limit", "")

	st.Lock()
	for i := 0; i < 25; i++ {
		model.AppendActivity(st.Data(), model.Activity{AccountID: accID, Type: "router",
			Message: "Routeur «Yopougon» hors ligne — sans check-in depuis 4m0s"})
	}
	st.Save()
	st.Unlock()

	// Première visite (seenAt vide) : tout est lu (comportement historique).
	if b := bellOf(t, ts, ownerToken); b.Unread != 0 {
		t.Fatalf("première visite : 0 non lu attendu (tout lu historique), %d obtenu", b.Unread)
	}

	// On acquitte à un instant ancien : les 25 deviennent non-lues, la page
	// n'en montre que 20 mais le badge en compte 25.
	past := "2020-01-01T00:00:00Z"
	st.Lock()
	for i := range st.Data().Users {
		if st.Data().Users[i].AccountID == accID {
			st.Data().Users[i].ActivitySeenAt = past
		}
	}
	st.Save()
	st.Unlock()
	b := bellOf(t, ts, ownerToken)
	if len(b.Items) != 20 {
		t.Fatalf("page : 20 items attendus (limit), %d obtenus", len(b.Items))
	}
	// 25 entrées seed + « Nouveau compte créé » de l'inscription = 26 non-lues.
	if b.Unread != 26 {
		t.Fatalf("badge : 26 non-lus attendus (25 seed + inscription, au-delà de la page), %d obtenus", b.Unread)
	}
}

// TestBellRespectsRBAC — la boîte applique le même filtre que /api/activity
// (N°149) : un gérant ne reçoit ni billing ni team dans sa cloche.
func TestBellRespectsRBAC(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	ownerToken, accID, _ := registerAccount(t, ts, "bell-rbac", "")
	seedActivityForRBAC(st, accID)

	status, _ := doJSON(t, ts, "POST", "/api/team", ownerToken, map[string]string{
		"name": "Adjoint", "username": "bell.rbac", "password": "mot-de-passe-4", "role": "manager",
	})
	if status != 201 {
		t.Fatalf("création du gérant : statut %d", status)
	}
	loginStatus, out := doJSON(t, ts, "POST", "/api/auth/login", "", map[string]string{
		"username": "bell.rbac", "password": "mot-de-passe-4",
	})
	if loginStatus != 200 {
		t.Fatalf("login gérant : statut %d", loginStatus)
	}
	managerToken, _ := out["token"].(string)

	for _, item := range bellOf(t, ts, managerToken).Items {
		if item.Type == "billing" || item.Type == "team" {
			t.Fatalf("la cloche du gérant ne doit contenir aucune entrée %q", item.Type)
		}
	}
	// Le propriétaire, lui, voit les catégories sensibles.
	seen := map[string]bool{}
	for _, item := range bellOf(t, ts, ownerToken).Items {
		seen[item.Type] = true
	}
	if !seen["billing"] || !seen["team"] {
		t.Fatalf("owner : billing et team attendus dans sa cloche, obtenu %v", seen)
	}
}
