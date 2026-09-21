package api

// Tests N°165 — annonces programmées (publishAt) : invisibilité avant la
// date (route + cloche), état « scheduled » en console, validations de la
// date, publication automatique à la lecture (Active), tri des listes
// clientes par date EFFECTIVE, et e-mail différé parti au balayage à la
// publication — une seule fois (idempotence), jamais avant.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mikcloud/hotspot-api/internal/model"
)

// adminAnnouncementRows — GET /api/admin/announcements décodé brut (état
// calculé « state » compris).
func adminAnnouncementRows(t *testing.T, ts *httptest.Server, token string) []map[string]any {
	t.Helper()
	req, _ := http.NewRequest("GET", ts.URL+"/api/admin/announcements", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept-Encoding", "identity")
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("liste annonces : %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("liste annonces : statut %d", resp.StatusCode)
	}
	var rows []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
		t.Fatalf("décodage liste annonces : %v", err)
	}
	return rows
}

// TestAnnouncementScheduling — création programmée (publishAt futur) :
// invisible des clients (route + cloche), état « scheduled » en console ;
// dates invalides ou trop lointaines rejetées ; date passée = immédiate ;
// PublishAt atteint (posé au store) → visible, cloche à la date EFFECTIVE ;
// tri des listes clientes par date effective.
func TestAnnouncementScheduling(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	adminToken := adminTokenOf(t, ts)
	ownerToken, _, _ := registerAccount(t, ts, "ann-sched", "")

	// Validations : publishAt hors RFC 3339 ; publishAt > 365 j.
	if s, _ := createAnnouncement(t, ts, adminToken, map[string]any{
		"title": "Maintenance programmée", "level": "warning", "audience": "all",
		"publishAt": "demain-matin",
	}); s != 400 {
		t.Fatalf("publishAt invalide : 400 attendu, %d obtenu", s)
	}
	if s, _ := createAnnouncement(t, ts, adminToken, map[string]any{
		"title": "Maintenance programmée", "level": "warning", "audience": "all",
		"publishAt": time.Now().UTC().AddDate(2, 0, 0).Format(time.RFC3339),
	}); s != 400 {
		t.Fatalf("publishAt > 365 j : 400 attendu, %d obtenu", s)
	}
	// publishAt PASSÉ = diffusion immédiate (comportement historique).
	s, out := createAnnouncement(t, ts, adminToken, map[string]any{
		"title": "Annonce immédiate", "level": "info", "audience": "all",
		"publishAt": time.Now().UTC().Add(-time.Hour).Format(time.RFC3339),
	})
	if s != 201 {
		t.Fatalf("publishAt passé : 201 attendu, %d obtenu (%v)", s, out)
	}
	if pa, _ := out["publishAt"].(string); pa != "" {
		t.Fatalf("publishAt passé : vide attendu (immédiate), obtenu %q", pa)
	}

	// Annonce programmée dans 1 h : créée, invisible côté client et cloche.
	inOneHour := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	s, out = createAnnouncement(t, ts, adminToken, map[string]any{
		"title": "Migration samedi 04h", "body": "Coupure brève de 1 à 3 minutes.",
		"level": "warning", "audience": "all", "publishAt": inOneHour, "email": true,
	})
	if s != 201 {
		t.Fatalf("création programmée : 201 attendu, %d obtenu (%v)", s, out)
	}
	if pa, _ := out["publishAt"].(string); pa != inOneHour {
		t.Fatalf("création programmée : publishAt=%q attendu, obtenu %q", inOneHour, pa)
	}
	if ep, _ := out["emailPending"].(bool); !ep {
		t.Fatal("création programmée + email : emailPending attendu")
	}
	// La cliente ne voit QUE l'immédiate — la programmée est invisible,
	// bandeau comme cloche.
	visible := clientAnnouncementsOf(t, ts, ownerToken)
	if len(visible) != 1 || visible[0].Title != "Annonce immédiate" {
		t.Fatalf("annonce programmée : seule « Annonce immédiate » attendue côté client, %d obtenues", len(visible))
	}
	for _, it := range bellItemsOf(t, ts, ownerToken) {
		if it.Type == "announcement" && it.Title == "Migration samedi 04h" {
			t.Fatalf("cloche : l'annonce programmée ne doit pas y figurer, obtenu %+v", it)
		}
	}

	// Console : état « scheduled » (pas active) tant que la date est future.
	rows := adminAnnouncementRows(t, ts, adminToken)
	if len(rows) != 2 {
		t.Fatalf("liste : 2 annonces attendues, %d obtenues", len(rows))
	}
	byTitle := map[string]map[string]any{}
	for _, r := range rows {
		byTitle[r["title"].(string)] = r
	}
	if r := byTitle["Migration samedi 04h"]; r["state"] != "scheduled" || r["active"] != false {
		t.Fatalf("liste : state=scheduled attendu pour la programmée, obtenu %v", r)
	}
	if r := byTitle["Annonce immédiate"]; r["state"] != "active" || r["active"] != true {
		t.Fatalf("liste : state=active attendu pour l'immédiate, obtenu %v", r)
	}

	// L'instant de publication passe (PublishAt déplacé dans le passé,
	// directement au store — aucune action de fond pour la VISIBILITÉ) :
	// l'annonce apparaît côté client, la cloche sonne à la date EFFECTIVE.
	st.Lock()
	for i := range st.Data().Announcements {
		if st.Data().Announcements[i].Title == "Migration samedi 04h" {
			st.Data().Announcements[i].PublishAt = time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
		}
	}
	st.Save()
	st.Unlock()
	got := clientAnnouncementsOf(t, ts, ownerToken)
	var published model.Announcement
	for _, ann := range got {
		if ann.Title == "Migration samedi 04h" {
			published = ann
		}
	}
	if published.ID == "" {
		t.Fatalf("après publication : « Migration samedi 04h » attendue, obtenu %v", got)
	}
	found := false
	for _, it := range bellItemsOf(t, ts, ownerToken) {
		if it.Type == "announcement" && it.Title == "Migration samedi 04h" {
			found = true
			if it.At != published.PublishAt {
				t.Fatalf("cloche : date effective %s attendue, obtenue %s", published.PublishAt, it.At)
			}
		}
	}
	if !found {
		t.Fatal("cloche : item announcement attendu après publication")
	}

	// Tri par date EFFECTIVE : l'immédiate (créée après la rédaction de la
	// programmée, publiée AVANT elle) reste en tête ; la programmée publiée
	// à now-1 min la suit. Une info RÉDIGÉE avant mais publiée après
	// passerait devant — le bandeau suit la publication, pas la rédaction.
	if len(got) != 2 || got[0].Title != "Annonce immédiate" || got[1].Title != "Migration samedi 04h" {
		var titles []string
		for _, ann := range got {
			titles = append(titles, ann.Title)
		}
		t.Fatalf("tri par date effective : [immédiate, migration] attendu, obtenu %v", titles)
	}
}

// TestAnnouncementSweepDeferredEmail — l'e-mail d'une annonce programmée ne
// part PAS à la création : le balayage l'envoie au moment de la publication,
// une seule fois (EmailedAt/EmailPending épongés sous le verrou AVANT l'envoi).
func TestAnnouncementSweepDeferredEmail(t *testing.T) {
	ts, st := newTransactionalServer(t)
	adminToken := adminTokenOf(t, ts)

	// Stub AVANT l'inscription du destinataire (N°178) : son e-mail de
	// bienvenue part alors sous le dispatch capturé — wg.Wait() le draine —
	// au lieu de partir sous le dispatch PRODUCTION et rester en vol PENDANT
	// l'installation du stub (la course mémoire mesurée par le run 457 de la
	// CI : lecture du pointeur par la goroutine welcome non synchronisée
	// avec l'écriture du remplacement).
	var calls []sentEmail
	wg := stubAccountEmailCapture(t, &calls)
	seedResend(t, st, model.AccountMainID)  // expéditeur : réglages du compte principal
	registerAccount(t, ts, "ann-sweep", "") // destinataire (e-mail connu)
	wg.Wait()                               // le welcome est capté…
	resetSentEmails(&calls)                 // …et écarté : seuls les e-mail d'ANNONCE comptent

	// Programmée dans 1 h + e-mail : RIEN ne part maintenant.
	inOneHour := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	s, out := createAnnouncement(t, ts, adminToken, map[string]any{
		"title": "Maintenance programmée", "body": "Coupure brève du cloud.",
		"level": "warning", "audience": "all", "publishAt": inOneHour, "email": true,
	})
	if s != 201 {
		t.Fatalf("création programmée : 201 attendu, %d obtenu (%v)", s, out)
	}
	if len(calls) != 0 {
		t.Fatalf("aucun e-mail ne doit partir avant la publication, %d obtenus", len(calls))
	}

	// Pas encore due : le balayage ne fait rien.
	engine := New(st, testJWTSecret)
	engine.RunAnnouncementSweep()
	if len(calls) != 0 {
		t.Fatalf("balayage avant publication : aucun e-mail attendu, %d obtenus", len(calls))
	}

	// L'instant passe (PublishAt passé, EmailPending toujours posé).
	st.Lock()
	for i := range st.Data().Announcements {
		st.Data().Announcements[i].PublishAt = time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	}
	st.Save()
	st.Unlock()

	// Balayage : l'e-mail différé part UNE fois vers le destinataire.
	engine.RunAnnouncementSweep()
	wg.Wait()
	if len(calls) != 1 {
		t.Fatalf("1 e-mail attendu après publication, %d obtenus", len(calls))
	}
	if !strings.Contains(calls[0].title, "Maintenance programmée") || !strings.Contains(calls[0].to, "@") {
		t.Fatalf("e-mail mal adressé : to=%q title=%q", calls[0].to, calls[0].title)
	}

	// Trace posée + drapeau épongé : un second passage ne double JAMAIS l'envoi.
	st.Lock()
	var traced, pending bool
	for _, ann := range st.Data().Announcements {
		if ann.Title == "Maintenance programmée" {
			traced = ann.EmailedAt != "" && ann.EmailedCount == 1
			pending = ann.EmailPending
		}
	}
	st.Unlock()
	if !traced || pending {
		t.Fatalf("trace de diffusion attendue (emailedAt+count, plus de pending), obtenu traced=%v pending=%v", traced, pending)
	}
	engine.RunAnnouncementSweep()
	wg.Wait()
	if len(calls) != 1 {
		t.Fatalf("idempotence : toujours 1 e-mail après second balayage, %d obtenus", len(calls))
	}
}
