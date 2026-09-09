// Tests N°72 — compression gzip : décisions (Accept-Encoding, Content-Type,
// statuts sans corps), transparence pour les clients qui ne demandent rien
// (agents RouterOS), équivalence du corps décompressé et gain réel sur une
// réponse volumineuse.
package api

import (
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mikcloud/hotspot-api/internal/auth"
	"mikcloud/hotspot-api/internal/model"
)

// doRaw — requête au serveur de test avec en-têtes pilotés À LA MAIN.
// Poser « Accept-Encoding » soi-même (plutôt que laisser le transport le
// faire) désactive la décompression transparente de net/http : le corps
// arrive brut, exactement comme sur le réseau.
func doGzipReq(t *testing.T, ts *httptest.Server, method, path, token, acceptEncoding, body string) (*http.Response, string) {
	t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, ts.URL+path, rd)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if acceptEncoding != "" {
		req.Header.Set("Accept-Encoding", acceptEncoding)
	}
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	return resp, string(raw)
}

// TestGzipSkipsTinyResponses — les réponses sous le seuil de
// rentabilité (gzipMinBody) sortent en clair même si le client demandait
// gzip : le gain du deflate sur quelques centaines d'octets ne paie pas
// l'en-tête gzip + le CRC, ni le CPU des deux côtés. Le corps est
// IDENTIQUE au chemin clair.
func TestGzipSkipsTinyResponses(t *testing.T) {
	_, ts := newTestServerWithStore(t)

	respGz, gzBody := doGzipReq(t, ts, http.MethodGet, "/", "", "gzip", "")
	if ce := respGz.Header.Get("Content-Encoding"); ce != "" {
		t.Fatalf("petite réponse : aucun Content-Encoding attendu, obtenu %q", ce)
	}
	if !strings.Contains(gzBody, `"service"`) {
		t.Fatalf("le corps doit être servi en clair : %s", gzBody)
	}
	_, plain := doGzipReq(t, ts, http.MethodGet, "/", "", "identity", "")
	if gzBody != plain {
		t.Fatalf("corps identiques attendus\nclair : %s\ngzip : %s", plain, gzBody)
	}
}

// TestGzipWriterBuffersThenCompresses — le tampon retient les octets
// tant que le corps est sous le seuil ; dès qu'il le dépasse, la
// décision tombe (en-têtes + statut retenu) et le corps complet part
// compressé — les octets écrits AVANT la décision sont dans le flux
// compressé, rien n'est perdu ni doublé.
func TestGzipWriterBuffersThenCompresses(t *testing.T) {
	rec := httptest.NewRecorder()
	gw := &gzipWriter{ResponseWriter: rec}
	gw.Header().Set("Content-Type", "application/json; charset=utf-8")
	gw.WriteHeader(http.StatusOK)
	// 900 octets : sous le seuil — rien n'est encore parti.
	chunk := strings.Repeat("a", 900)
	if _, err := gw.Write([]byte(chunk)); err != nil {
		t.Fatal(err)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("sous le seuil, rien ne doit partir : %d octets déjà écrits", rec.Body.Len())
	}
	// 900 de plus : le seuil est franchi — décision, en-têtes, compression.
	if _, err := gw.Write([]byte(chunk)); err != nil {
		t.Fatal(err)
	}
	gw.close()
	if ce := rec.Header().Get("Content-Encoding"); ce != "gzip" {
		t.Fatalf("Content-Encoding gzip attendu au-delà du seuil, obtenu %q", ce)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("statut retenu 200 attendu, obtenu %d", rec.Code)
	}
	zr, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatalf("corps gzip illisible : %v", err)
	}
	decoded, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded) != 1800 {
		t.Fatalf("1800 octets attendus après décompression, obtenu %d", len(decoded))
	}
	if string(decoded) != chunk+chunk {
		t.Fatal("le corps compressé doit contenir EXACTEMENT les octets écrits (aucune perte au tampon)")
	}
}

// TestGzipBigJSONActuallyShrinks — la preuve du gain : le contrat
// sync-status (30 tables + agents + bande passante) doit être nettement
// plus petit compressé qu'en clair — c'est la promesse du N°72 (÷4 à ÷8).
func TestGzipBigJSONActuallyShrinks(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	seedUser(t, st, "usr-plat-gz", "", "usr-plat-gz", model.RolePlatformAdmin)
	platform := auth.Sign(testJWTSecret, auth.NewClaims("usr-plat-gz", "Plateforme", model.RolePlatformAdmin, "", 0))

	respPlain, plain := doGzipReq(t, ts, http.MethodGet, "/api/admin/sync-status", platform, "identity", "")
	if respPlain.StatusCode != http.StatusOK {
		t.Fatalf("200 attendu, obtenu %d", respPlain.StatusCode)
	}
	respGz, gzBody := doGzipReq(t, ts, http.MethodGet, "/api/admin/sync-status", platform, "gzip", "")
	if respGz.Header.Get("Content-Encoding") != "gzip" {
		t.Fatalf("Content-Encoding gzip attendu, obtenu %q", respGz.Header.Get("Content-Encoding"))
	}
	if len(gzBody) >= len(plain) {
		t.Fatalf("la réponse compressée doit être plus petite : gzip=%d octets, clair=%d octets", len(gzBody), len(plain))
	}
	// Le corps compressé reste du JSON valide une fois décompressé.
	zr, err := gzip.NewReader(strings.NewReader(gzBody))
	if err != nil {
		t.Fatalf("corps gzip illisible : %v", err)
	}
	decoded, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("décompression impossible : %v", err)
	}
	var check map[string]any
	if err := json.Unmarshal(decoded, &check); err != nil {
		t.Fatalf("JSON illisible après décompression : %v", err)
	}
	if check["mode"] != "json" {
		t.Fatalf("mode attendu « json », obtenu %v", check["mode"])
	}
}

// TestGzipNoBodyStatus — le track analytics du portail répond 204
// silencieux : jamais de Content-Encoding sur un statut sans corps, même
// si le client demandait gzip.
func TestGzipNoBodyStatus(t *testing.T) {
	_, ts := newTestServerWithStore(t)
	resp, body := doGzipReq(t, ts, http.MethodPost, "/api/portal/track", "", "gzip", "pas du json")
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("204 attendu (track silencieux), obtenu %d (body %q)", resp.StatusCode, body)
	}
	if ce := resp.Header.Get("Content-Encoding"); ce != "" {
		t.Fatalf("204 : aucune compression attendue, Content-Encoding=%q", ce)
	}
}

// TestGzipWriterSkipsBinary — un corps image (Content-Type binaire) sort
// en clair même si le client demandait gzip : ne rien gagner sur de
// l'already-compressed ne vaut pas le coût CPU.
func TestGzipWriterSkipsBinary(t *testing.T) {
	rec := httptest.NewRecorder()
	gw := &gzipWriter{ResponseWriter: rec}
	gw.Header().Set("Content-Type", "image/png")
	gw.WriteHeader(http.StatusOK)
	png := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0, 0, 0, 0}
	if _, err := gw.Write(png); err != nil {
		t.Fatal(err)
	}
	gw.close()
	if ce := rec.Header().Get("Content-Encoding"); ce != "" {
		t.Fatalf("image : aucune compression attendue, Content-Encoding=%q", ce)
	}
	if !strings.Contains(string(rec.Body.Bytes()), string(png)) {
		t.Fatal("le corps binaire doit passer intact en clair")
	}
}

// TestClientAcceptsGzip — parsing de l'en-tête (avec ou sans qualité).
func TestClientAcceptsGzip(t *testing.T) {
	for _, tc := range []struct {
		header string
		want   bool
	}{
		{"gzip", true},
		{"gzip;q=0.8", true},
		{"deflate, gzip", true},
		{"br, gzip ;q=1", true},
		{"deflate", false},
		{"br", false},
		{"", false},
		{"identity", false},
	} {
		req, _ := http.NewRequest(http.MethodGet, "/", nil)
		if tc.header != "" {
			req.Header.Set("Accept-Encoding", tc.header)
		}
		if got := clientAcceptsGzip(req); got != tc.want {
			t.Fatalf("Accept-Encoding %q : %v attendu, obtenu %v", tc.header, tc.want, got)
		}
	}
}

// TestCompressibleContentType — filtre des types compressibles.
func TestCompressibleContentType(t *testing.T) {
	for _, tc := range []struct {
		ct   string
		want bool
	}{
		{"application/json; charset=utf-8", true},
		{"text/html; charset=utf-8", true},
		{"text/css; charset=utf-8", true},
		{"application/javascript; charset=utf-8", true},
		{"image/svg+xml", true},
		{"image/png", false},
		{"image/jpeg", false},
		{"font/woff2", false},
		{"application/octet-stream", false},
		{"", false},
	} {
		if got := compressibleContentType(tc.ct); got != tc.want {
			t.Fatalf("Content-Type %q : %v attendu, obtenu %v", tc.ct, tc.want, got)
		}
	}
}
