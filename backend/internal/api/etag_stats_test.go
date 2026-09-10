package api

// Tests N°74 — ETag/304 sur les GET publics quasi statiques du portail
// (writeJSONCacheable) + compteurs de stock serveur (GET /api/vouchers/stats,
// fin du poll pageSize:500 qui comptait faux au-delà de 200 tickets).

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"mikcloud/hotspot-api/internal/model"
)

// TestWriteJSONCacheableConditional — contrat de writeJSONCacheable :
// 200 + ETag + Cache-Control: no-cache au premier appel ; If-None-Match
// concordant → 304 SANS corps ; ETag divergent → 200 complet.
func TestWriteJSONCacheableConditional(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSONCacheable(w, r, http.StatusOK, map[string]any{"tenantName": "Maquis Test"})
	})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("premier appel : %d attendu, %d obtenu", http.StatusOK, rec.Code)
	}
	etag := rec.Header().Get("ETag")
	if etag == "" {
		t.Fatal("ETag absent du premier 200")
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Fatalf("Cache-Control %q attendu (stockage + revalidation), %q obtenu", "no-cache", cc)
	}
	if len(rec.Body.Bytes()) == 0 {
		t.Fatal("le premier 200 doit porter le corps complet")
	}

	// Revalidation concordante → 304 vide.
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/x", nil)
	req2.Header.Set("If-None-Match", etag)
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusNotModified {
		t.Fatalf("If-None-Match concordant : %d attendu, %d obtenu", http.StatusNotModified, rec2.Code)
	}
	if rec2.Body.Len() != 0 {
		t.Fatalf("304 doit être sans corps, %d octets obtenus", rec2.Body.Len())
	}
	if rec2.Header().Get("ETag") == "" {
		t.Fatal("le 304 doit répéter l'ETag (RFC 9110)")
	}

	// ETag divergent → 200 complet.
	rec3 := httptest.NewRecorder()
	req3 := httptest.NewRequest(http.MethodGet, "/x", nil)
	req3.Header.Set("If-None-Match", `"vieux"`)
	handler.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusOK || rec3.Body.Len() == 0 {
		t.Fatalf("If-None-Match divergent : 200 complet attendu, %d / %d octets obtenus", rec3.Code, rec3.Body.Len())
	}
}

// TestWifiPortalETagThroughChain — la validation croisée traverse TOUTE la
// chaîne (gzip + egress + sécurité) : le portail reçoit 200+ETag, puis 304
// au rechargement — le corps ne part qu'une fois. Le 304 doit passer le
// middleware gzip EN CLAIR (statut sans corps, jamais compressé).
func TestWifiPortalETagThroughChain(t *testing.T) {
	ts, st := newWifiTestServer(t)
	_, accID, _ := registerAccount(t, ts, "gerant-etag-portail", "")
	routerID, profileID := seedWifiEnv(t, st, accID)
	seedWifiSite(t, st, accID, "etag-portail", routerID, profileID, true, 1, 100)

	resp, _ := doGzipReq(t, ts, http.MethodGet, "/api/wifi/site/etag-portail/portal", "", "gzip", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("premier appel : statut %d", resp.StatusCode)
	}
	etag := resp.Header.Get("ETag")
	if etag == "" {
		t.Fatal("ETag absent de la config live (writeJSONCacheable non appliqué ?)")
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "no-cache" {
		t.Fatalf("Cache-Control %q attendu, %q obtenu", "no-cache", cc)
	}

	// Revalidation : If-None-Match concordant → 304 sans corps, à travers
	// TOUTE la chaîne de middlewares.
	req2, err := http.NewRequest(http.MethodGet, ts.URL+"/api/wifi/site/etag-portail/portal", nil)
	if err != nil {
		t.Fatal(err)
	}
	req2.Header.Set("Accept-Encoding", "gzip")
	req2.Header.Set("If-None-Match", etag)
	resp2, err := ts.Client().Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusNotModified {
		t.Fatalf("revalidation : 304 attendu, statut %d obtenu", resp2.StatusCode)
	}
	if resp2.Header.Get("ETag") != etag {
		t.Fatalf("le 304 doit répéter l'ETag %q, %q obtenu", etag, resp2.Header.Get("ETag"))
	}
}

// TestVouchersStatsServerSide — N°74 : les compteurs de la vue Vouchers sont
// calculés sur l'ENSEMBLE du stock côté serveur (l'ancien comptage client
// plafonnait à 200 tickets et téléchargeait tout). Vérifie les 6 compteurs +
// la valeur du stock (somme des prix des actifs SEULEMENT).
func TestVouchersStatsServerSide(t *testing.T) {
	ts, st := newWifiTestServer(t)
	token, accID, _ := registerAccount(t, ts, "gerant-stats-vouchers", "")
	routerID, _ := seedWifiEnv(t, st, accID)

	future := time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339)
	past := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)
	st.Lock()
	st.Data().HotspotUsers = append(st.Data().HotspotUsers, []model.HotspotUser{
		{ID: "v-actif-1", AccountID: accID, Kind: "voucher", Username: "ACTIF1", RouterID: routerID, Status: "active", Price: 200, ExpiresAt: future},
		{ID: "v-actif-2", AccountID: accID, Kind: "voucher", Username: "ACTIF2", RouterID: routerID, Status: "active", Price: 500, ExpiresAt: future, ResellerID: "res-1"},
		{ID: "v-actif-3", AccountID: accID, Kind: "voucher", Username: "ACTIF3", RouterID: routerID, Status: "active", Price: 300, ExpiresAt: future},
		{ID: "v-utilise", AccountID: accID, Kind: "voucher", Username: "UTILISE", RouterID: routerID, Status: "used", Price: 200, ExpiresAt: future},
		{ID: "v-expire", AccountID: accID, Kind: "voucher", Username: "EXPIRE", RouterID: routerID, Status: "expired", Price: 200, ExpiresAt: past},
		// Kind regular : hors périmètre vouchers (le endpoint force kind=voucher).
		{ID: "v-regulier", AccountID: accID, Kind: "regular", Username: "REGULIER", RouterID: routerID, Status: "active", Price: 999, ExpiresAt: future},
		// Autre compte : invisible.
		{ID: "v-autre", AccountID: "acc-autre", Kind: "voucher", Username: "AUTRE", RouterID: routerID, Status: "active", Price: 999, ExpiresAt: future},
	}...)
	st.Unlock()

	status, out := doJSON(t, ts, "GET", "/api/vouchers/stats", token, nil)
	if status != http.StatusOK {
		t.Fatalf("stats : statut %d, corps %v", status, out)
	}
	if n := int(out["active"].(float64)); n != 3 {
		t.Fatalf("active = %d, voulu 3", n)
	}
	if n := int(out["used"].(float64)); n != 1 {
		t.Fatalf("used = %d, voulu 1", n)
	}
	if n := int(out["expired"].(float64)); n != 1 {
		t.Fatalf("expired = %d, voulu 1", n)
	}
	if n := int(out["allocated"].(float64)); n != 1 {
		t.Fatalf("allocated = %d, voulu 1 (un seul voucher confié au revendeur)", n)
	}
	if v := int(out["stockValue"].(float64)); v != 1000 {
		t.Fatalf("stockValue = %d, voulu 1000 (somme des actifs uniquement : 200+500+300)", v)
	}
	if n := int(out["total"].(float64)); n != 5 {
		t.Fatalf("total = %d, voulu 5 (vouchers du compte, kind voucher)", n)
	}
}

// TestVouchersStatsRequiresAuth — endpoint scopé console : sans JWT → 401.
func TestVouchersStatsRequiresAuth(t *testing.T) {
	ts, _ := newWifiTestServer(t)
	if status, _ := doJSON(t, ts, "GET", "/api/vouchers/stats", "", nil); status != http.StatusUnauthorized {
		t.Fatalf("sans jeton : 401 attendu, %d obtenu", status)
	}
}
