// handlers_join_test.go — N°27 — inscriptions publiques par QR code.
//
// Couverture :
//   - cycle complet : création de lien (console) → info publique → soumission
//     « pending » → approbation (utilisateur régulier créé en mode
//     « Nom d'utilisateur & Mot de passe » — codes DISTINCTS au choix,
//     validité = profil, mot de passe de la demande VIDÉ) → refus d'une
//     seconde demande (motif conservé + mot de passe vidé) ;
//   - garde-fous du lien : inconnu (404), expiré / révoqué / saturé (409) ;
//   - validations de soumission : honeypot (succès factice, rien créé),
//     identifiant pris (409 + suggestion), téléphone en doublon (409),
//     mot de passe trop court, nom trop court, téléphone invalide (400) ;
//   - lien kiosque autoValidate : création IMMÉDIATE + file agent user_add ;
//     autoValidate sans pré-attribution → 400 ;
//   - scoping : liens et demandes d'un autre compte invisibles (404) ;
//   - sweep : les demandes refusées de plus de 30 jours sont purgées.
//
// Aucune connexion réseau réelle : store JSON éphémère + routeurs simulé/agent.
package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mikcloud/hotspot-api/internal/model"
	"mikcloud/hotspot-api/internal/store"
)

// joinRegister — variante de registerAccount avec téléphone unique : le
// dédoublonnage SaaS (S5) refuse deux comptes avec le même numéro WhatsApp.
func joinRegister(t *testing.T, ts *httptest.Server, username, phone string) (string, string) {
	t.Helper()
	status, out := doJSON(t, ts, "POST", "/api/auth/register", "", map[string]string{
		"name":     "Gérant " + username,
		"username": username,
		"password": "mot-de-passe-8+",
		"key":      "",
		"email":    username + "@example.ci",
		"phone":    phone,
		"country":  "CI",
		"city":     "Abidjan",
	})
	if status != http.StatusCreated {
		t.Fatalf("inscription %s : statut %d, corps %v", username, status, out)
	}
	token, _ := out["token"].(string)
	user, _ := out["user"].(map[string]any)
	accID, _ := user["accountId"].(string)
	return token, accID
}

// joinSeedEnv — compte réel + profil + routeurs simulé et agent injectés
// sous verrou. Retourne (token gérant, accountID).
func joinSeedEnv(t *testing.T, st *store.Store, ts *httptest.Server, username, phone string) (string, string) {
	t.Helper()
	token, accID := joinRegister(t, ts, username, phone)
	st.Lock()
	db := st.Data()
	db.Profiles = append(db.Profiles, model.Profile{
		ID: "p-join", AccountID: accID, Name: "Etudiant",
		RateLimit: "1M/1M", SessionTimeoutMin: 60, ValidityDays: 30,
	})
	db.Routers = append(db.Routers,
		model.Router{ID: "r-sim", AccountID: accID, Name: "Campus-SIM", Mode: "simulated"},
		model.Router{ID: "r-agt", AccountID: accID, Name: "Campus-AGT", Mode: "agent"},
	)
	st.Save()
	st.Unlock()
	return token, accID
}

// joinItems — extrait "items" (tableau de maps) d'une réponse.
func joinItems(t *testing.T, out map[string]any) []map[string]any {
	t.Helper()
	raw, ok := out["items"].([]any)
	if !ok {
		t.Fatalf("réponse sans items : %v", out)
	}
	items := make([]map[string]any, 0, len(raw))
	for _, it := range raw {
		if m, ok := it.(map[string]any); ok {
			items = append(items, m)
		}
	}
	return items
}

// joinCounts — extrait les compteurs par statut.
func joinCounts(t *testing.T, out map[string]any) map[string]int {
	t.Helper()
	raw, ok := out["counts"].(map[string]any)
	if !ok {
		t.Fatalf("réponse sans counts : %v", out)
	}
	counts := map[string]int{}
	for k, v := range raw {
		if f, ok := v.(float64); ok {
			counts[k] = int(f)
		}
	}
	return counts
}

// createJoinLink — crée un lien via la console et retourne (id, token).
func createJoinLink(t *testing.T, ts *httptest.Server, token string, body map[string]any) (string, string) {
	t.Helper()
	status, out := doJSON(t, ts, "POST", "/api/join-links", token, body)
	if status != http.StatusOK {
		t.Fatalf("création du lien : statut %d, corps %v", status, out)
	}
	id, _ := out["id"].(string)
	tok, _ := out["token"].(string)
	if id == "" || len(tok) != 32 {
		t.Fatalf("lien créé incomplet : id=%q token=%q", id, tok)
	}
	return id, tok
}

// TestJoinFullFlow — cycle complet console ↔ page publique ↔ validation.
func TestJoinFullFlow(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	managerToken, _ := joinSeedEnv(t, st, ts, "proprio-join-a", "0101010101")

	// Création du lien + liste console.
	linkID, joinToken := createJoinLink(t, ts, managerToken, map[string]any{
		"name": "Rentrée 2026", "maxUses": 0,
	})
	status, out := doJSON(t, ts, "GET", "/api/join-links", managerToken, nil)
	if status != http.StatusOK || len(joinItems(t, out)) != 1 {
		t.Fatalf("liste des liens : statut %d, corps %v", status, out)
	}

	// Page publique : info accessible SANS token.
	status, out = doJSON(t, ts, "GET", "/api/join/"+joinToken, "", nil)
	if status != http.StatusOK {
		t.Fatalf("info publique : statut %d", status)
	}
	if out["state"] != "active" || out["name"] != "Rentrée 2026" {
		t.Fatalf("info publique inattendue : %v", out)
	}
	if org, _ := out["organization"].(string); org == "" {
		t.Fatalf("nom d'organisation absent : %v", out)
	}

	// Soumission publique — téléphone avec espaces et « + » (normalisé).
	body := map[string]any{
		"fullName": "Awa Traoré", "phone": "+225 07 08 09 10 11",
		"username": "awa.t", "password": "motdepasse1",
	}
	status, out = doJSON(t, ts, "POST", "/api/join/"+joinToken, "", body)
	if status != http.StatusOK || out["status"] != "pending" {
		t.Fatalf("soumission : statut %d, corps %v", status, out)
	}

	// Compteur d'usages du lien.
	st.Lock()
	if st.Data().JoinLinks[0].Uses != 1 {
		t.Fatalf("usages du lien : 1 attendu, obtenu %d", st.Data().JoinLinks[0].Uses)
	}
	st.Unlock()

	// File du gérant : 1 en attente, téléphone normalisé, mot de passe présent.
	status, out = doJSON(t, ts, "GET", "/api/registrations", managerToken, nil)
	if status != http.StatusOK {
		t.Fatalf("liste des demandes : statut %d", status)
	}
	if counts := joinCounts(t, out); counts["pending"] != 1 {
		t.Fatalf("compteur pending : 1 attendu, obtenu %v", counts)
	}
	items := joinItems(t, out)
	if len(items) != 1 || items[0]["fullName"] != "Awa Traoré" || items[0]["phone"] != "+2250708091011" {
		t.Fatalf("demande inattendue : %v", items)
	}
	reqID, _ := items[0]["id"].(string)
	if pw, _ := items[0]["password"].(string); pw != "motdepasse1" {
		t.Fatalf("mot de passe de la demande absent pour le gérant : %v", items[0])
	}

	// Doublon de téléphone en attente → 409.
	body2 := map[string]any{
		"fullName": "Autre Personne", "phone": "+225 07 08 09 10 11",
		"username": "autre.user", "password": "motdepasse2",
	}
	status, out = doJSON(t, ts, "POST", "/api/join/"+joinToken, "", body2)
	if status != http.StatusConflict || out["code"] != "phone_pending" {
		t.Fatalf("doublon téléphone : statut %d, corps %v", status, out)
	}

	// Approbation — mode « Nom d'utilisateur & Mot de passe » (codes distincts).
	approve := map[string]any{
		"profileId": "p-join", "routerId": "r-sim",
		"username": "awa.t", "password": "motdepasse1",
	}
	status, out = doJSON(t, ts, "POST", "/api/registrations/"+reqID+"/approve", managerToken, approve)
	if status != http.StatusOK {
		t.Fatalf("approbation : statut %d, corps %v", status, out)
	}
	user, _ := out["user"].(map[string]any)
	if user["kind"] != "regular" || user["username"] != "awa.t" || user["password"] != "motdepasse1" {
		t.Fatalf("utilisateur créé inattendu : %v", user)
	}
	if user["profileId"] != "p-join" {
		t.Fatalf("profil non appliqué : %v", user)
	}
	if exp, _ := user["expiresAt"].(string); exp == "" {
		t.Fatalf("validité absente (profil 30 j) : %v", user)
	}
	if exp, _ := user["expiresAt"].(string); exp != "" {
		if t0, err := time.Parse(time.RFC3339, exp); err == nil {
			if d := time.Until(t0); d < 29*24*time.Hour || d > 31*24*time.Hour {
				t.Fatalf("validité hors fenêtre 30 j : %v", exp)
			}
		}
	}
	reqOut, _ := out["request"].(map[string]any)
	if reqOut["status"] != "approved" || reqOut["userId"] != user["id"] {
		t.Fatalf("demande non approuvée : %v", reqOut)
	}
	if pw, _ := reqOut["password"].(string); pw != "" {
		t.Fatalf("mot de passe de la demande non vidé : %v", reqOut)
	}

	// Utilisateur réellement en base (régulier, identité dans le commentaire).
	st.Lock()
	users := st.Data().HotspotUsers
	if len(users) != 1 || users[0].Kind != "regular" {
		t.Fatalf("utilisateur en base inattendu : %v", users)
	}
	if !strings.Contains(users[0].Comment, "Awa Traoré") {
		t.Fatalf("identité absente du commentaire : %q", users[0].Comment)
	}
	st.Unlock()

	// Seconde demande → refus (motif conservé, mot de passe vidé).
	status, _ = doJSON(t, ts, "POST", "/api/join/"+joinToken, "", body2)
	if status != http.StatusOK {
		t.Fatalf("seconde soumission : statut %d", status)
	}
	_, out = doJSON(t, ts, "GET", "/api/registrations?status=pending", managerToken, nil)
	items = joinItems(t, out)
	if len(items) != 1 {
		t.Fatalf("seconde demande absente : %v", items)
	}
	reqID2, _ := items[0]["id"].(string)
	status, out = doJSON(t, ts, "POST", "/api/registrations/"+reqID2+"/reject", managerToken, map[string]any{"reason": "Hors zone de couverture"})
	if status != http.StatusOK || out["status"] != "rejected" {
		t.Fatalf("refus : statut %d, corps %v", status, out)
	}
	if pw, _ := out["password"].(string); pw != "" {
		t.Fatalf("mot de passe non vidé au refus : %v", out)
	}
	if out["rejectionReason"] != "Hors zone de couverture" {
		t.Fatalf("motif du refus perdu : %v", out)
	}

	// Compteurs finaux + re-traitement impossible.
	_, out = doJSON(t, ts, "GET", "/api/registrations", managerToken, nil)
	counts := joinCounts(t, out)
	if counts["pending"] != 0 || counts["approved"] != 1 || counts["rejected"] != 1 {
		t.Fatalf("compteurs finaux inattendus : %v", counts)
	}
	status, _ = doJSON(t, ts, "POST", "/api/registrations/"+reqID2+"/approve", managerToken, approve)
	if status != http.StatusConflict {
		t.Fatalf("re-approbation d'une demande refusée : statut %d", status)
	}

	// Révocation immédiate du lien + suppression.
	status, out = doJSON(t, ts, "PUT", "/api/join-links/"+linkID, managerToken, map[string]any{"revoked": true})
	if status != http.StatusOK || out["state"] != "revoked" {
		t.Fatalf("révocation : statut %d, corps %v", status, out)
	}
	status, out = doJSON(t, ts, "POST", "/api/join/"+joinToken, "", map[string]any{
		"fullName": "Nouveau Bac", "phone": "0102030405", "username": "nouveau.b", "password": "motdepasse3",
	})
	if status != http.StatusConflict || out["code"] != "join_link_closed" {
		t.Fatalf("soumission sur lien révoqué : statut %d, corps %v", status, out)
	}
	status, _ = doJSON(t, ts, "DELETE", "/api/join-links/"+linkID, managerToken, nil)
	if status != http.StatusOK {
		t.Fatalf("suppression du lien : statut %d", status)
	}
}

// TestJoinLinkGuards — garde-fous et validations de la page publique.
func TestJoinLinkGuards(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	managerToken, accID := joinSeedEnv(t, st, ts, "proprio-join-b", "0202020202")

	// Liens dans chaque état non actif, injectés directement.
	st.Lock()
	db := st.Data()
	past := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	db.JoinLinks = append(db.JoinLinks,
		model.JoinLink{ID: "jl-exp", AccountID: accID, Name: "Expiré", Token: "EXPIRED-TOKEN-00000000000000000000", ExpiresAt: past},
		model.JoinLink{ID: "jl-rev", AccountID: accID, Name: "Révoqué", Token: "REVOKED-TOKEN-00000000000000000000", Revoked: true},
		model.JoinLink{ID: "jl-max", AccountID: accID, Name: "Saturé", Token: "MAXED-TOKEN-0000000000000000000000", MaxUses: 1, Uses: 1},
	)
	db.HotspotUsers = append(db.HotspotUsers, model.HotspotUser{
		ID: "u-taken", AccountID: accID, Kind: "regular", Username: "taken.user", Status: "active",
	})
	st.Save()
	st.Unlock()

	// Lien inconnu → 404.
	status, out := doJSON(t, ts, "GET", "/api/join/UNKNOWN-TOKEN", "", nil)
	if status != http.StatusNotFound || out["code"] != "join_link_unknown" {
		t.Fatalf("lien inconnu : statut %d, corps %v", status, out)
	}

	// États non actifs : info 200 avec état, soumission 409.
	for _, cas := range []struct{ token, state string }{
		{"EXPIRED-TOKEN-00000000000000000000", "expired"},
		{"REVOKED-TOKEN-00000000000000000000", "revoked"},
		{"MAXED-TOKEN-0000000000000000000000", "exhausted"},
	} {
		_, out = doJSON(t, ts, "GET", "/api/join/"+cas.token, "", nil)
		if out["state"] != cas.state {
			t.Fatalf("état %q attendu, obtenu %v", cas.state, out["state"])
		}
		status, out = doJSON(t, ts, "POST", "/api/join/"+cas.token, "", map[string]any{
			"fullName": "Test Garde", "phone": "0708091011", "username": "test.g", "password": "motdepasse",
		})
		if status != http.StatusConflict || out["code"] != "join_link_closed" {
			t.Fatalf("soumission sur lien %s : statut %d, corps %v", cas.state, status, out)
		}
	}

	// Identifiant déjà pris → 409 + suggestion libre.
	_, joinToken := createJoinLink(t, ts, managerToken, map[string]any{"name": "Sain"})
	status, out = doJSON(t, ts, "POST", "/api/join/"+joinToken, "", map[string]any{
		"fullName": "Test Pris", "phone": "0601020304", "username": "taken.user", "password": "motdepasse",
	})
	if status != http.StatusConflict || out["code"] != "username_taken" {
		t.Fatalf("identifiant pris : statut %d, corps %v", status, out)
	}
	if sugg, _ := out["suggestion"].(string); !strings.HasPrefix(sugg, "taken.user2") {
		t.Fatalf("suggestion attendue taken.user2…, obtenue %q", sugg)
	}

}

// TestJoinSubmitValidation — validations de forme du formulaire public
// (serveur dédié : le quota anti-abus par IP — 5 soumissions / 10 min —
// est un garde-fou RÉEL, les tests le respectent).
func TestJoinSubmitValidation(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	managerToken, _ := joinSeedEnv(t, st, ts, "proprio-join-g", "0707070707")
	_, joinToken := createJoinLink(t, ts, managerToken, map[string]any{"name": "Formes"})

	for nom, body := range map[string]map[string]any{
		"mot de passe court":  {"fullName": "Test Court", "phone": "0601020305", "username": "test.c", "password": "12345"},
		"nom trop court":      {"fullName": "T", "phone": "0601020306", "username": "test.n", "password": "motdepasse"},
		"téléphone invalide":  {"fullName": "Test Tel", "phone": "12ab", "username": "test.t", "password": "motdepasse"},
		"identifiant espaces": {"fullName": "Test Esp", "phone": "0601020307", "username": "a b", "password": "motdepasse"},
	} {
		status, _ := doJSON(t, ts, "POST", "/api/join/"+joinToken, "", body)
		if status != http.StatusBadRequest {
			t.Fatalf("%s : 400 attendu, obtenu %d", nom, status)
		}
	}
}

// TestJoinHoneypot — le champ caché reçoit un succès factice, rien n'est créé.
func TestJoinHoneypot(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	managerToken, _ := joinSeedEnv(t, st, ts, "proprio-join-c", "0303030303")
	_, joinToken := createJoinLink(t, ts, managerToken, map[string]any{"name": "Honey"})

	status, out := doJSON(t, ts, "POST", "/api/join/"+joinToken, "", map[string]any{
		"fullName": "Bot Bot", "phone": "0601020308", "username": "bot.bot", "password": "motdepasse",
		"website": "http://spam.example",
	})
	if status != http.StatusOK || out["status"] != "pending" {
		t.Fatalf("honeypot : succès factice attendu, statut %d, corps %v", status, out)
	}
	st.Lock()
	db := st.Data()
	if len(db.RegistrationRequests) != 0 || len(db.JoinLinks) != 1 || db.JoinLinks[0].Uses != 0 {
		t.Fatalf("le honeypot ne doit rien créer : req=%d uses=%d", len(db.RegistrationRequests), db.JoinLinks[0].Uses)
	}
	st.Unlock()
}

// TestJoinAutoValidate — lien kiosque : création immédiate + file agent.
func TestJoinAutoValidate(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	managerToken, _ := joinSeedEnv(t, st, ts, "proprio-join-d", "0404040404")

	// autoValidate sans pré-attribution → 400.
	status, _ := doJSON(t, ts, "POST", "/api/join-links", managerToken, map[string]any{
		"name": "Kiosque incomplet", "autoValidate": true,
	})
	if status != http.StatusBadRequest {
		t.Fatalf("autoValidate sans profil/routeur : 400 attendu, obtenu %d", status)
	}

	// autoValidate complet (routeur AGENT → file user_add).
	status, _ = createJoinLinkStatus(t, ts, managerToken, map[string]any{
		"name": "Kiosque Cantine", "autoValidate": true, "profileId": "p-join", "routerId": "r-agt",
	})
	if status != http.StatusOK {
		t.Fatalf("création du lien kiosque : statut %d", status)
	}
	_, out := doJSON(t, ts, "GET", "/api/join-links", managerToken, nil)
	link := joinItems(t, out)[0]
	joinToken, _ := link["token"].(string)

	status, out = doJSON(t, ts, "POST", "/api/join/"+joinToken, "", map[string]any{
		"fullName": "Kiosque Etudiant", "phone": "0601020309", "username": "kiosque.e", "password": "motdepasse9",
	})
	if status != http.StatusOK || out["status"] != "approved" {
		t.Fatalf("kiosque : approbation immédiate attendue, statut %d, corps %v", status, out)
	}
	if out["username"] != "kiosque.e" || out["password"] != "motdepasse9" {
		t.Fatalf("identifiants non restitués : %v", out)
	}
	st.Lock()
	db := st.Data()
	if len(db.HotspotUsers) != 1 || db.HotspotUsers[0].Username != "kiosque.e" || db.HotspotUsers[0].Kind != "regular" {
		t.Fatalf("utilisateur kiosque non créé : %v", db.HotspotUsers)
	}
	if !strings.Contains(db.HotspotUsers[0].Comment, "Kiosque Etudiant") {
		t.Fatalf("identité absente du commentaire kiosque : %q", db.HotspotUsers[0].Comment)
	}
	if len(db.Commands) != 1 || db.Commands[0].Kind != model.CmdUserAdd {
		t.Fatalf("commande user_add attendue pour le routeur agent : %v", db.Commands)
	}
	if db.JoinLinks[0].Uses != 1 {
		t.Fatalf("usages du lien kiosque : 1 attendu, obtenu %d", db.JoinLinks[0].Uses)
	}
	if len(db.RegistrationRequests) != 1 || db.RegistrationRequests[0].Status != "approved" || db.RegistrationRequests[0].Password != "" {
		t.Fatalf("trace de la demande kiosque inattendue : %v", db.RegistrationRequests)
	}
	st.Unlock()

	// Identifiant désormais pris → 409 (même lien).
	status, _ = doJSON(t, ts, "POST", "/api/join/"+joinToken, "", map[string]any{
		"fullName": "Deuxieme Etudiant", "phone": "0601020310", "username": "kiosque.e", "password": "motdepasse",
	})
	if status != http.StatusConflict {
		t.Fatalf("doublon kiosque : 409 attendu, obtenu %d", status)
	}
}

// createJoinLinkStatus — variante de createJoinLink qui retourne le statut
// sans assertion (cas d'erreur attendus).
func createJoinLinkStatus(t *testing.T, ts *httptest.Server, token string, body map[string]any) (int, map[string]any) {
	t.Helper()
	return doJSON(t, ts, "POST", "/api/join-links", token, body)
}

// TestJoinScoping — isolation entre comptes (liens, demandes, actions).
func TestJoinScoping(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	tokenA, accA := joinSeedEnv(t, st, ts, "proprio-join-e", "0505050505")
	tokenB, _ := joinSeedEnv(t, st, ts, "proprio-join-f", "0606060606")
	_ = tokenB

	// Compte A : un lien + une demande.
	_, joinToken := createJoinLink(t, ts, tokenA, map[string]any{"name": "Lien A"})
	doJSON(t, ts, "POST", "/api/join/"+joinToken, "", map[string]any{
		"fullName": "Chez A", "phone": "0601020311", "username": "chez.a", "password": "motdepasse",
	})

	// Compte B ne voit rien de A.
	_, out := doJSON(t, ts, "GET", "/api/join-links", tokenB, nil)
	if items := joinItems(t, out); len(items) != 0 {
		t.Fatalf("liens d'A visibles par B : %v", items)
	}
	_, out = doJSON(t, ts, "GET", "/api/registrations", tokenB, nil)
	if counts := joinCounts(t, out); counts["pending"] != 0 {
		t.Fatalf("demandes d'A visibles par B : %v", counts)
	}

	// Les actions de B sur les objets de A sont refusées.
	linkID, _ := func() (string, string) {
		_, out := doJSON(t, ts, "GET", "/api/join-links", tokenA, nil)
		items := joinItems(t, out)
		id, _ := items[0]["id"].(string)
		return id, joinToken
	}()
	status, _ := doJSON(t, ts, "PUT", "/api/join-links/"+linkID, tokenB, map[string]any{"revoked": true})
	if status != http.StatusNotFound {
		t.Fatalf("révocation inter-comptes : 404 attendu, obtenu %d", status)
	}
	_, out = doJSON(t, ts, "GET", "/api/registrations?status=pending", tokenA, nil)
	reqID, _ := joinItems(t, out)[0]["id"].(string)
	status, _ = doJSON(t, ts, "POST", "/api/registrations/"+reqID+"/reject", tokenB, map[string]any{"reason": "spoil"})
	if status != http.StatusNotFound {
		t.Fatalf("refus inter-comptes : 404 attendu, obtenu %d", status)
	}
	_ = accA
}

// TestSweepStaleRegistrations — les refusés > 30 j sont purgés, rien d'autre.
func TestSweepStaleRegistrations(t *testing.T) {
	_, ts := newTestServerWithStore(t)
	_ = ts
	now := time.Now().UTC()
	db := &model.DB{RegistrationRequests: []model.RegistrationRequest{
		{ID: "r-old", Status: "rejected", CreatedAt: now.Add(-31 * 24 * time.Hour).Format(time.RFC3339)},
		{ID: "r-recent", Status: "rejected", CreatedAt: now.Add(-10 * 24 * time.Hour).Format(time.RFC3339)},
		{ID: "r-approved", Status: "approved", CreatedAt: now.Add(-40 * 24 * time.Hour).Format(time.RFC3339)},
		{ID: "r-pending", Status: "pending", CreatedAt: now.Add(-40 * 24 * time.Hour).Format(time.RFC3339)},
	}}
	removed := sweepStaleRegistrations(db)
	if removed != 1 {
		t.Fatalf("1 refusé à purger, obtenu %d", removed)
	}
	if len(db.RegistrationRequests) != 3 {
		t.Fatalf("3 demandes conservées attendues, obtenu %d", len(db.RegistrationRequests))
	}
	for _, q := range db.RegistrationRequests {
		if q.ID == "r-old" {
			t.Fatalf("le refusé de 31 jours devait être purgé")
		}
	}
}
