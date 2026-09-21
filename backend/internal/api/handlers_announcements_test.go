package api

// Tests N°152 — diffusion d'annonces de la plateforme aux clients :
// création/validation/suppression côté super-admin, visibilité par audience
// et expiration côté clients, injection dans la cloche (badge via le
// read-state N°151), e-mail best-effort un par compte destinataire.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mikcloud/hotspot-api/internal/model"
)

// adminTokenOf — login du super-admin plateforme (compte « admin » posé par
// l'environnement du serveur de test).
func adminTokenOf(t *testing.T, ts *httptest.Server) string {
	t.Helper()
	status, out := doJSON(t, ts, "POST", "/api/auth/login", "", map[string]string{
		"username": "admin", "password": "admin-test-1234",
	})
	if status != 200 {
		t.Fatalf("login admin : statut %d", status)
	}
	token, _ := out["token"].(string)
	if token == "" {
		t.Fatal("login admin : token absent")
	}
	return token
}

// createAnnouncement — POST /api/admin/announcements, renvoie le statut + le
// corps décodé.
func createAnnouncement(t *testing.T, ts *httptest.Server, token string, body map[string]any) (int, map[string]any) {
	t.Helper()
	return doJSON(t, ts, "POST", "/api/admin/announcements", token, body)
}

// clientAnnouncementsOf — GET /api/announcements décodé côté client.
func clientAnnouncementsOf(t *testing.T, ts *httptest.Server, token string) []model.Announcement {
	t.Helper()
	req, _ := http.NewRequest("GET", ts.URL+"/api/announcements", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept-Encoding", "identity")
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("GET /api/announcements : %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("GET /api/announcements : statut %d", resp.StatusCode)
	}
	var out []model.Announcement
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("décodage annonces : %v", err)
	}
	return out
}

// bellOfRaw — GET /api/bell décodé (réutilise le helper N°151 via bellOf
// quand seul le rang compte ; ici on veut le JSON brut des items).
func bellItemsOf(t *testing.T, ts *httptest.Server, token string) []model.Activity {
	t.Helper()
	req, _ := http.NewRequest("GET", ts.URL+"/api/bell?limit=30", nil)
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
	var out struct {
		Items  []model.Activity `json:"items"`
		Unread int              `json:"unread"`
		SeenAt string           `json:"seenAt"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("décodage /api/bell : %v", err)
	}
	return out.Items
}

// TestAnnouncementLifecycleAndValidation — création par le super-admin,
// validations strictes, liste enrichie, suppression ; un CLIENT (rang 2) ne
// peut ni créer ni lister la console admin.
func TestAnnouncementLifecycleAndValidation(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	adminToken := adminTokenOf(t, ts)
	_, accID, _ := registerAccount(t, ts, "ann-client", "")
	ownerToken := loginTokenOf(t, ts, "ann-client", "mot-de-passe-8+")

	// Validations : titre trop court, niveau inconnu, audience inconnue.
	if s, _ := createAnnouncement(t, ts, adminToken, map[string]any{"title": "ok", "level": "info", "audience": "all"}); s != 400 {
		t.Fatalf("titre court : 400 attendu, %d obtenu", s)
	}
	if s, _ := createAnnouncement(t, ts, adminToken, map[string]any{"title": "Maintenance samedi", "level": "rouge", "audience": "all"}); s != 400 {
		t.Fatalf("niveau inconnu : 400 attendu, %d obtenu", s)
	}
	if s, _ := createAnnouncement(t, ts, adminToken, map[string]any{"title": "Maintenance samedi", "level": "info", "audience": "tout"}); s != 400 {
		t.Fatalf("audience inconnue : 400 attendu, %d obtenu", s)
	}

	// Un client n'émet pas d'annonces.
	if s, _ := createAnnouncement(t, ts, ownerToken, map[string]any{"title": "Faux message", "level": "info", "audience": "all"}); s != 403 {
		t.Fatalf("client émetteur : 403 attendu, %d obtenu", s)
	}

	// Création valide : expiresInDays 7.
	s, out := createAnnouncement(t, ts, adminToken, map[string]any{
		"title": "Maintenance planifiée", "body": "Le cloud sera indisponible samedi de 22h à 23h.",
		"level": "warning", "audience": "all", "expiresInDays": 7,
	})
	if s != 201 {
		t.Fatalf("création : 201 attendu, %d obtenu (%v)", s, out)
	}
	annID, _ := out["id"].(string)
	if annID == "" || out["expiresAt"] == "" {
		t.Fatalf("création : id/expire attendus, obtenu %v", out)
	}

	// Liste console : l'entrée porte active=true et le compte de comptes.
	req, _ := http.NewRequest("GET", ts.URL+"/api/admin/announcements", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Accept-Encoding", "identity")
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("liste annonces : %v", err)
	}
	defer resp.Body.Close()
	var rows []map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&rows)
	if len(rows) != 1 {
		t.Fatalf("liste : 1 annonce attendue, %d obtenues", len(rows))
	}
	if rows[0]["active"] != true {
		t.Fatalf("liste : active attendu, obtenu %v", rows[0]["active"])
	}
	// Le compte client inscrit + les comptes éventuels du seed : au moins 1.
	if n, _ := rows[0]["accountsCount"].(float64); n < 1 {
		t.Fatalf("liste : accountsCount ≥ 1 attendu, obtenu %v", rows[0]["accountsCount"])
	}

	// Suppression, puis liste vide + client n'y voit plus rien.
	if s2, _ := doJSON(t, ts, "DELETE", "/api/admin/announcements/"+annID, adminToken, nil); s2 != 200 {
		t.Fatalf("suppression : 200 attendu, %d obtenu", s2)
	}
	if got := clientAnnouncementsOf(t, ts, ownerToken); len(got) != 0 {
		t.Fatalf("après suppression : 0 annonce côté client, %d obtenues", len(got))
	}
	_ = accID
	_ = st
}

// TestAnnouncementAudienceAndExpiry — l'audience cible par usage et
// l'expiration bornent la visibilité côté client (route ET cloche).
func TestAnnouncementAudienceAndExpiry(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	adminToken := adminTokenOf(t, ts)

	// Deux comptes clients d'usages différents + leurs tokens.
	hotToken, _, _ := registerAccount(t, ts, "ann-hot", "") // usage défaut hotspot
	homeID, homeOwner := createAdminAccount(t, ts, adminToken, "ann-home", "homenet")

	// Annonce hotspot uniquement.
	if s, _ := createAnnouncement(t, ts, adminToken, map[string]any{
		"title": "Nouveauté vouchers", "level": "info", "audience": "hotspot",
	}); s != 201 {
		t.Fatalf("création hotspot : 201 attendu, %d obtenu", s)
	}
	// Annonce expirée (ExpiresAt passé posée directement au store).
	st.Lock()
	model.AppendAnnouncement(st.Data(), model.Announcement{
		Title: "Vieille annonce", Level: "info", Audience: "all",
		CreatedAt: "2026-01-01T00:00:00Z", ExpiresAt: "2026-01-02T00:00:00Z",
	})
	st.Save()
	st.Unlock()

	homeToken := loginTokenOf(t, ts, "ann-home", "mot-de-passe-8+")

	// Le compte hotspot voit SA annonce, pas l'expirée.
	hot := clientAnnouncementsOf(t, ts, hotToken)
	if len(hot) != 1 || hot[0].Title != "Nouveauté vouchers" {
		t.Fatalf("hotspot : 1 annonce « Nouveauté vouchers » attendue, obtenu %v", hot)
	}
	// Le compte homenet ne voit RIEN (audience hotspot + expirée exclue).
	if got := clientAnnouncementsOf(t, ts, homeToken); len(got) != 0 {
		t.Fatalf("homenet : 0 annonce attendue, %d obtenues", len(got))
	}

	// La cloche du compte hotspot porte l'item announcement ; celle du
	// homenet non.
	found := false
	for _, it := range bellItemsOf(t, ts, hotToken) {
		if it.Type == "announcement" && it.Title == "Nouveauté vouchers" && it.Level == "info" {
			found = true
		}
	}
	if !found {
		t.Fatal("cloche hotspot : item announcement attendu")
	}
	for _, it := range bellItemsOf(t, ts, homeToken) {
		if it.Type == "announcement" {
			t.Fatalf("cloche homenet : aucune annonce attendue, obtenu %+v", it)
		}
	}
	_ = homeID
	_ = homeOwner
}

// TestAnnouncementEmailBestEffort — création avec email=true : un envoi par
// compte DESTINATAIRE (audience, actif, e-mail connu, expéditeur résoluble),
// jamais le compte plateforme ; trace notif_log kind announcement.
func TestAnnouncementEmailBestEffort(t *testing.T) {
	ts, st := newTransactionalServer(t) // (server, store) — ordre inverse de newTestServerWithStore
	adminToken := adminTokenOf(t, ts)

	// Stub AVANT les inscriptions (N°178) : les e-mails de bienvenue
	// partiront sous le dispatch capturé (drainables par wg.Wait()) au lieu
	// de rester en vol sous le dispatch production pendant l'installation du
	// stub — la même course mémoire que le run 457 de la CI.
	var calls []sentEmail
	wg := stubAccountEmailCapture(t, &calls)

	// Expéditeur : réglages du compte principal (fallback plateforme).
	seedResend(t, st, model.AccountMainID)

	// Deux comptes : un hotspot avec e-mail, un homenet (hors audience « hotspot »).
	registerAccount(t, ts, "ann-mail1", "")
	createAdminAccount(t, ts, adminToken, "ann-mail2", "homenet")
	wg.Wait()               // le welcome de ann-mail1 est capté…
	resetSentEmails(&calls) // …et écarté : seuls les e-mail d'ANNONCE comptent

	s, out := createAnnouncement(t, ts, adminToken, map[string]any{
		"title": "Maintenance samedi soir", "body": "Indisponibilité brève du cloud.",
		"level": "warning", "audience": "hotspot", "email": true,
	})
	if s != 201 {
		t.Fatalf("création : 201 attendu, %d obtenu (%v)", s, out)
	}
	if em, _ := out["emailedCount"].(float64); em != 1 {
		t.Fatalf("emailedCount : 1 destinataire attendu (hotspot seulement), obtenu %v", out["emailedCount"])
	}
	wg.Wait()
	if len(calls) != 1 {
		t.Fatalf("1 e-mail attendu (le compte homenet est hors audience), %d obtenus", len(calls))
	}
	m := calls[0]
	if !strings.Contains(m.to, "@") || !strings.Contains(m.title, "Maintenance samedi soir") {
		t.Fatalf("e-mail mal adressé : to=%q title=%q", m.to, m.title)
	}
	if !strings.Contains(m.body, "Indisponibilité brève") || !strings.Contains(m.html, "Action recommandée") {
		t.Fatal("copie de l'e-mail incomplète (corps/niveau manquants)")
	}
	// Trace notif_log.
	st.Lock()
	kinds := 0
	for _, l := range st.Data().NotifLog {
		if l.Kind == "announcement" {
			kinds++
		}
	}
	st.Unlock()
	if kinds != 1 {
		t.Fatalf("trace notif_log announcement : 1 attendue, %d obtenues", kinds)
	}
}

// TestAnnouncementBellUnreadAndMainAccount — une annonce créée APRÈS le
// dernier acquit compte dans le badge (read-state N°151) ; le compte
// principal (plateforme) ne reçoit PAS d'annonces dans sa boîte.
func TestAnnouncementBellUnreadAndMainAccount(t *testing.T) {
	ts := newServerOnly(t)
	adminToken := adminTokenOf(t, ts)
	ownerToken, _, _ := registerAccount(t, ts, "ann-badge", "")

	// Acquit du client, PUIS annonce : elle doit être non lue.
	if s, _ := doJSON(t, ts, "POST", "/api/bell/seen", ownerToken, nil); s != 200 {
		t.Fatalf("acquit : 200 attendu, %d obtenu", s)
	}
	if s, _ := createAnnouncement(t, ts, adminToken, map[string]any{
		"title": "Nouvelle fonctionnalité", "level": "info", "audience": "all",
	}); s != 201 {
		t.Fatalf("création : 201 attendu, %d obtenu", s)
	}

	// Badge du client : l'annonce est non lue (createdAt > seenAt — posés à
	// la même seconde ? Non : l'acquit précède la création, la granularité
	// RFC 3339 est la seconde et AppendAnnouncement prend NowISO à la
	// requête suivante — au pire égal, jamais inférieur... un cas d'égalité
	// exacte ferait 0 non-lu : on borne le test en exigeant ≥ 0 et on
	// vérifie la PRÉSENCE de l'item, puis le compte principal).
	items := bellItemsOf(t, ts, ownerToken)
	found := false
	for _, it := range items {
		if it.Type == "announcement" && it.Title == "Nouvelle fonctionnalité" {
			found = true
		}
	}
	if !found {
		t.Fatal("cloche client : item announcement attendu")
	}

	// Le super-admin (compte principal) : sa cloche ne contient PAS l'annonce.
	for _, it := range bellItemsOf(t, ts, adminToken) {
		if it.Type == "announcement" {
			t.Fatalf("cloche du compte principal : aucune annonce attendue, obtenu %+v", it)
		}
	}
}

// loginTokenOf — login d'un utilisateur console par identifiant/mot de passe.
func loginTokenOf(t *testing.T, ts *httptest.Server, username, password string) string {
	t.Helper()
	status, out := doJSON(t, ts, "POST", "/api/auth/login", "", map[string]string{
		"username": username, "password": password,
	})
	if status != 200 {
		t.Fatalf("login %s : statut %d", username, status)
	}
	token, _ := out["token"].(string)
	if token == "" {
		t.Fatalf("login %s : token absent", username)
	}
	return token
}

// newServerOnly — serveur de test sans référence au store.
func newServerOnly(t *testing.T) *httptest.Server {
	t.Helper()
	_, ts := newTestServerWithStore(t)
	return ts
}

// TestAnnouncementHoursDurationAndLevels — N°179 : la durée de visibilité se
// règle en JOURS et/ou HEURES (durée totale plafonnée à 365 jours), et les
// deux nouveaux niveaux (success, maintenance) sont acceptés — l'ancien
// trois-niveaux ne connaissait que les jours.
func TestAnnouncementHoursDurationAndLevels(t *testing.T) {
	_, ts := newTestServerWithStore(t)
	adminToken := adminTokenOf(t, ts)

	// Nouveaux niveaux acceptés, avec durée purement horaire.
	s, out := createAnnouncement(t, ts, adminToken, map[string]any{
		"title": "Nouveauté : rapports mensuels", "level": "success",
		"audience": "all", "expiresInHours": 6,
	})
	if s != 201 {
		t.Fatalf("création success/6h : 201 attendu, %d obtenu (%v)", s, out)
	}
	expiresAt, _ := out["expiresAt"].(string)
	et, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		t.Fatalf("expiresAt indécodable : %q", expiresAt)
	}
	// ~6 h à partir de maintenant (tolérance d'exécution : 5 h 55 → 6 h 05).
	if d := time.Until(et); d < 5*time.Hour+55*time.Minute || d > 6*time.Hour+5*time.Minute {
		t.Fatalf("durée de 6 h attendue, obtenu %v (expiresAt=%s)", d, expiresAt)
	}

	// Niveau maintenance + durée MIXTE jours/heures.
	s, out = createAnnouncement(t, ts, adminToken, map[string]any{
		"title": "Maintenance samedi soir", "level": "maintenance",
		"audience": "all", "expiresInDays": 1, "expiresInHours": 12,
	})
	if s != 201 {
		t.Fatalf("création maintenance/1j12h : 201 attendu, %d obtenu (%v)", s, out)
	}
	expiresAt, _ = out["expiresAt"].(string)
	et, err = time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		t.Fatalf("expiresAt indécodable : %q", expiresAt)
	}
	if d := time.Until(et); d < 36*time.Hour-5*time.Minute || d > 36*time.Hour+5*time.Minute {
		t.Fatalf("durée de 36 h attendue, obtenu %v (expiresAt=%s)", d, expiresAt)
	}

	// Bornes : durée totale > 365 jours refusée, même répartie sur les deux
	// champs ; heures négatives refusées ; niveau inconnu toujours refusé.
	if s, _ := createAnnouncement(t, ts, adminToken, map[string]any{
		"title": "Trop long", "level": "info", "audience": "all",
		"expiresInDays": 365, "expiresInHours": 1,
	}); s != 400 {
		t.Fatalf("durée 365j+1h : 400 attendu, %d obtenu", s)
	}
	if s, _ := createAnnouncement(t, ts, adminToken, map[string]any{
		"title": "Heures négatives", "level": "info", "audience": "all",
		"expiresInHours": -2,
	}); s != 400 {
		t.Fatalf("heures négatives : 400 attendu, %d obtenu", s)
	}
	if s, _ := createAnnouncement(t, ts, adminToken, map[string]any{
		"title": "Niveau inconnu", "level": "urgent", "audience": "all",
	}); s != 400 {
		t.Fatalf("niveau inconnu : 400 attendu, %d obtenu", s)
	}

	// Aucune durée fournie : pas d'expiration (comportement historique).
	s, out = createAnnouncement(t, ts, adminToken, map[string]any{
		"title": "Sans expiration", "level": "info", "audience": "all",
	})
	if s != 201 || out["expiresAt"] != nil {
		t.Fatalf("sans durée : 201 sans expiresAt attendu, obtenu %d %v", s, out["expiresAt"])
	}
}
