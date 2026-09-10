package api

// Tests N°75 — ETag/304 sur les endpoints CONSOLE les plus polleurs :
// GET /api/dashboard et GET /api/sessions. Le navigateur de la console
// revalide automatiquement (Cache-Control: no-cache + If-None-Match) : entre
// deux changements de données, la réponse est un 304 sans corps au lieu du
// téléchargement intégral — c'était le plus gros poste restant du trafic
// console (1,5 Mo / 777 req mesuré en pointe avant N°75).

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"mikcloud/hotspot-api/internal/model"
)

// consoleETagFlow — scénario commun : 1er appel → 200 + ETag + no-cache ;
// revalidation concordante → 304 vide (l'ETag est répété) ; ETag périmé
// (les données ont changé) → 200 complet avec NOUVEAU ETag.
func consoleETagFlow(t *testing.T, ts *httptest.Server, path, token string, mutate func()) {
	t.Helper()

	resp, _ := doGzipReq(t, ts, http.MethodGet, path, token, "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("premier appel %s : statut %d attendu, %d obtenu", path, http.StatusOK, resp.StatusCode)
	}
	etag := resp.Header.Get("ETag")
	if etag == "" {
		t.Fatalf("premier appel %s : ETag absent", path)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "no-cache" {
		t.Fatalf("%s : Cache-Control %q attendu (stockage + revalidation obligatoire), %q obtenu", path, "no-cache", cc)
	}

	// Données inchangées → revalidation → 304 sans corps.
	resp2, body2 := doGzipReqIfNoneMatch(t, ts, http.MethodGet, path, token, etag)
	if resp2.StatusCode != http.StatusNotModified {
		t.Fatalf("revalidation %s (données inchangées) : 304 attendu, %d obtenu", path, resp2.StatusCode)
	}
	if body2 != "" {
		t.Fatalf("304 %s doit être sans corps, %d octets obtenus", path, len(body2))
	}
	if resp2.Header.Get("ETag") != etag {
		t.Fatalf("le 304 %s doit répéter l'ETag courant (%q obtenu, %q attendu)", path, resp2.Header.Get("ETag"), etag)
	}

	// Les données changent → ETag différent → 200 complet.
	if mutate != nil {
		mutate()
		resp3, body3 := doGzipReqIfNoneMatch(t, ts, http.MethodGet, path, token, etag)
		if resp3.StatusCode != http.StatusOK {
			t.Fatalf("après mutation %s : 200 attendu, %d obtenu", path, resp3.StatusCode)
		}
		if body3 == "" {
			t.Fatalf("après mutation %s : le corps complet doit être renvoyé", path)
		}
		if resp3.Header.Get("ETag") == etag {
			t.Fatalf("après mutation %s : l'ETag doit changer (données différentes)", path)
		}
	}
}

// TestDashboardETagConditional — /api/dashboard : 200+ETag, 304 sur
// revalidation, 200 + ETag neuf après un changement de données (une activité
// de compte suffit à faire bouger le corps : recentActivity).
func TestDashboardETagConditional(t *testing.T) {
	ts, st := newWifiTestServer(t)
	token, accID, _ := registerAccount(t, ts, "gerant-etag-dash", "")
	mutate := func() {
		st.Lock()
		db := st.Data()
		db.Activity = append([]model.Activity{{
			ID: model.NewID("act-"), AccountID: accID, Type: "test", Message: "activité N°75", At: model.NowISO(),
		}}, db.Activity...)
		st.Unlock()
	}
	consoleETagFlow(t, ts, "/api/dashboard", token, mutate)
}

// TestSessionsETagConditional — /api/sessions : même contrat. La mutation
// ajoute une session du compte (le read_state suivant ferait de même).
func TestSessionsETagConditional(t *testing.T) {
	ts, st := newWifiTestServer(t)
	token, accID, _ := registerAccount(t, ts, "gerant-etag-sess", "")
	st.Lock()
	st.Data().Sessions = append(st.Data().Sessions, model.Session{ID: "s-etag-1", AccountID: accID, Username: "alice", RouterID: "r-x", StartedAt: "2026-01-01T00:00:00Z"})
	st.Unlock()
	consoleETagFlow(t, ts, "/api/sessions", token, func() {
		st.Lock()
		st.Data().Sessions = append(st.Data().Sessions, model.Session{ID: "s-etag-2", AccountID: accID, Username: "bob", RouterID: "r-x", StartedAt: "2026-01-01T01:00:00Z"})
		st.Unlock()
	})
}

// doGzipReqIfNoneMatch — GET avec If-None-Match (le navigateur revalide
// automatiquement en HTTP cache no-cache ; ici on simule son en-tête).
func doGzipReqIfNoneMatch(t *testing.T, ts *httptest.Server, method, path, token, etag string) (*http.Response, string) {
	t.Helper()
	req, err := http.NewRequest(method, ts.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("If-None-Match", etag)
	req.Header.Set("Accept-Encoding", "identity")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp, string(b)
}
