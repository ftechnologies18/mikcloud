// handlers_promo_events_test.go — N°56 : analytics du portail hospitalité
// (track public dédupliqué, stats console, ids de promos stables).

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mikcloud/hotspot-api/internal/model"
)

// seedPromoVitrine — pose une vitrine hospitalité (2 promos, dont une avec
// lien cliquable) via PUT /api/settings et retourne (portalKey, idPromo1,
// idPromo2). Les ids sont générés par le serveur (premier enregistrement).
func seedPromoVitrine(t *testing.T, ts *httptest.Server, token string) (string, string, string) {
	t.Helper()
	status, out := doJSON(t, ts, "PUT", "/api/settings", token, map[string]any{
		"portalStyle": "hospitality",
		"portalPromos": []map[string]string{
			{"title": "Menu du jour", "desc": "Grillades", "priceLabel": "3 000 F", "link": "https://exemple.ci/menu"},
			{"title": "Jus frais", "desc": "Bissap, gingembre", "priceLabel": "500 F"},
		},
	})
	if status != http.StatusOK {
		t.Fatalf("PUT vitrine : statut %d, corps %v", status, out)
	}
	tenant, _ := out["tenant"].(map[string]any)
	if tenant == nil {
		t.Fatal("PUT settings doit renvoyer tenant{}")
	}
	key, _ := tenant["portalKey"].(string)
	if len(key) != 16 {
		t.Fatalf("portalKey attendue (16 hex), reçue %q", key)
	}
	rawJSON, _ := tenant["portalPromos"].(string)
	var promos []map[string]any
	if err := json.Unmarshal([]byte(rawJSON), &promos); err != nil {
		t.Fatalf("portalPromos doit être un JSON de promos avec ids : %v", err)
	}
	if len(promos) != 2 || promos[0]["id"] == "" || promos[1]["id"] == "" {
		t.Fatalf("chaque promo enregistrée doit recevoir un id, reçu %v", promos)
	}
	return key, promos[0]["id"].(string), promos[1]["id"].(string)
}

// track — POST /api/portal/track public et silencieux (204 attendu).
func track(t *testing.T, ts *httptest.Server, key, promoID, kind, clientKey string) int {
	t.Helper()
	status, _ := doJSON(t, ts, "POST", "/api/portal/track", "", map[string]string{
		"key": key, "promoId": promoID, "kind": kind, "clientKey": clientKey,
	})
	if status != http.StatusNoContent {
		t.Fatalf("track %s/%s : statut %d (le track est TOUJOURS 204)", kind, promoID, status)
	}
	return status
}

// TestPromoTrackDedupeAndStats — le fil complet : vitrine posée → impressions
// et clics déposés publiquement → dédup journalière par appareil → stats
// console cohérentes (le pitch « vu N fois cette semaine »).
func TestPromoTrackDedupeAndStats(t *testing.T) {
	ts, st := newWifiTestServer(t)
	token, accID, _ := registerAccount(t, ts, "gerant-promo-stats", "")
	key, idMenu, idJus := seedPromoVitrine(t, ts, token)

	// Premier appareil : impression menu + clic + impression jus.
	track(t, ts, key, idMenu, "impression", "AA:BB:CC:DD:EE:01")
	track(t, ts, key, idMenu, "click", "AA:BB:CC:DD:EE:01")
	track(t, ts, key, idJus, "impression", "AA:BB:CC:DD:EE:01")

	// Re-POST identiques (refresh-spam) : dédup → toujours 3 événements.
	track(t, ts, key, idMenu, "impression", "AA:BB:CC:DD:EE:01")
	track(t, ts, key, idMenu, "click", "AA:BB:CC:DD:EE:01")

	// Deuxième appareil : impression menu → 4 événements.
	track(t, ts, key, idMenu, "impression", "AA:BB:CC:DD:EE:02")

	st.Lock()
	if n := len(st.Data().PromoEvents); n != 4 {
		st.Unlock()
		t.Fatalf("journal attendu = 4 événements (dédup par appareil/jour), trouvé %d", n)
	}
	st.Unlock()

	// Dépôts INVALIDES : tous silencieusement ignorés (204), zéro écriture.
	track(t, ts, "0123456789abcdef", idMenu, "impression", "AA:BB:CC:DD:EE:03") // clé d'UN autre compte (aucun match)
	track(t, ts, "pas-une-cle!!", idMenu, "impression", "AA:BB:CC:DD:EE:03")    // clé malformée
	track(t, ts, key, "pdeadbeef123", "impression", "AA:BB:CC:DD:EE:03")        // promo inexistante
	track(t, ts, key, idMenu, "ping", "AA:BB:CC:DD:EE:03")                      // kind inconnu
	track(t, ts, key, "<script>", "impression", "AA:BB:CC:DD:EE:03")            // promoId hors format
	// Sans clé du tout (portail commercial sans analytics) : ignoré.
	track(t, ts, "", idMenu, "impression", "AA:BB:CC:DD:EE:03")

	st.Lock()
	if n := len(st.Data().PromoEvents); n != 4 {
		st.Unlock()
		t.Fatalf("dépôts invalides doivent être ignorés (journal=4), trouvé %d", n)
	}
	// L'empreinte client : MAC normalisée (majuscules, séparateurs « : »).
	for i := range st.Data().PromoEvents {
		if ev := &st.Data().PromoEvents[i]; ev.ClientKey != "AA:BB:CC:DD:EE:01" && ev.ClientKey != "AA:BB:CC:DD:EE:02" {
			st.Unlock()
			t.Fatalf("clientKey attendue = MAC normalisée, reçue %q", ev.ClientKey)
		}
		if ev := &st.Data().PromoEvents[i]; ev.AccountID != accID || ev.Day != time.Now().UTC().Format("2006-01-02") {
			st.Unlock()
			t.Fatalf("événement mal scopé : %+v", ev)
		}
	}
	st.Unlock()

	// Stats console : manager et plus (rôle owner du gérant OK).
	status, out := doJSON(t, ts, "GET", "/api/promos/stats", token, nil)
	if status != http.StatusOK {
		t.Fatalf("GET stats : statut %d, corps %v", status, out)
	}
	totals, _ := out["totals"].(map[string]any)
	if totals == nil {
		t.Fatal("stats.totals manquant")
	}
	impr, _ := totals["impression"].(map[string]any)
	click, _ := totals["click"].(map[string]any)
	if impr["week"].(float64) != 3 || impr["total"].(float64) != 3 {
		t.Fatalf("impressions attendues 3/3 (jour/total — 2 appareils menu + 1 jus), reçues %v", impr)
	}
	if click["week"].(float64) != 1 {
		t.Fatalf("clics attendus 1, reçus %v", click)
	}
	promos, _ := out["promos"].([]any)
	if len(promos) != 2 {
		t.Fatalf("2 lignes de vitrine attendues, reçues %d", len(promos))
	}
	first, _ := promos[0].(map[string]any)
	if first["id"] != idMenu || first["title"] != "Menu du jour" {
		t.Fatalf("1re ligne attendue = menu (%s), reçue %v", idMenu, first)
	}
	fi, _ := first["impressions"].(map[string]any)
	if fi["week"].(float64) != 2 {
		t.Fatalf("menu : 2 impressions (2 appareils), reçues %v", fi)
	}

	// Stats SANS token : refusées (lecture console uniquement).
	if status, _ := doJSON(t, ts, "GET", "/api/promos/stats", "", nil); status != http.StatusUnauthorized {
		t.Fatalf("stats sans token : statut %d (401 attendu)", status)
	}
}

// TestPromoStatsBuckets — les fenêtres jour/semaine/total : un événement du
// jour, un de la semaine passée, un hors rétention d'affichage (40 j) et un
// orphelin (promo retirée de la vitrine) ne comptent pas tous pareil.
func TestPromoStatsBuckets(t *testing.T) {
	ts, st := newWifiTestServer(t)
	token, accID, _ := registerAccount(t, ts, "gerant-promo-buckets", "")
	key, idMenu, _ := seedPromoVitrine(t, ts, token)

	today := time.Now().UTC()
	mk := func(daysAgo int, promo string) {
		st.Lock()
		db := st.Data()
		db.PromoEvents = append(db.PromoEvents, model.PromoEvent{
			ID:        fmt.Sprintf("e-seed-%d-%s", daysAgo, promo),
			AccountID: accID,
			PromoID:   promo,
			Kind:      "impression",
			ClientKey: "AABBCCDDEE09",
			Day:       today.AddDate(0, 0, -daysAgo).Format("2006-01-02"),
			CreatedAt: model.NowISO(),
		})
		st.Save()
		st.Unlock()
	}
	mk(0, idMenu)        // aujourd'hui → jour + semaine + total
	mk(3, idMenu)        // dans la semaine glissante → semaine + total
	mk(40, idMenu)       // hors 7 jours, dans la rétention → total seul
	mk(0, "pffffffff01") // orphelin (pas dans la vitrine) → jamais compté
	_ = key

	status, out := doJSON(t, ts, "GET", "/api/promos/stats", token, nil)
	if status != http.StatusOK {
		t.Fatalf("GET stats : statut %d", status)
	}
	promos, _ := out["promos"].([]any)
	if len(promos) != 2 {
		t.Fatalf("2 lignes attendues, reçues %d", len(promos))
	}
	first, _ := promos[0].(map[string]any)
	fi, _ := first["impressions"].(map[string]any)
	if fi["day"].(float64) != 1 || fi["week"].(float64) != 2 || fi["total"].(float64) != 3 {
		t.Fatalf("buckets menu attendus 1/2/3, reçus %v", fi)
	}
	totals, _ := out["totals"].(map[string]any)
	impr, _ := totals["impression"].(map[string]any)
	if impr["total"].(float64) != 3 || impr["week"].(float64) != 2 {
		t.Fatalf("totaux attendus week=2 total=3 (orphelin exclu), reçus %v", impr)
	}
}

// TestPromoIDsStableOnRoundTrip — les ids de promos sont STABLES au
// round-trip console (GET→PUT) : c'est ce qui garantit que les compteurs
// analytics survivent aux éditions de la vitrine. Le lien http:// et l'id
// malformé sont refusés (400).
func TestPromoIDsStableOnRoundTrip(t *testing.T) {
	ts, _ := newWifiTestServer(t)
	token, _, _ := registerAccount(t, ts, "gerant-promo-ids", "")

	// 1er enregistrement : ids posés par le serveur.
	status, out := doJSON(t, ts, "PUT", "/api/settings", token, map[string]any{
		"portalPromos": []map[string]string{{"title": "Plat du jour", "priceLabel": "2 500 F"}},
	})
	if status != http.StatusOK {
		t.Fatalf("PUT #1 : statut %d", status)
	}
	tenant, _ := out["tenant"].(map[string]any)
	var promos []map[string]any
	_ = json.Unmarshal([]byte(tenant["portalPromos"].(string)), &promos)
	id1, _ := promos[0]["id"].(string)
	if !strings.HasPrefix(id1, "p") {
		t.Fatalf("id aléatoire attendu (préfixe p), reçu %q", id1)
	}

	// 2e enregistrement : la console renvoie l'id connu → conservé.
	status, out = doJSON(t, ts, "PUT", "/api/settings", token, map[string]any{
		"portalPromos": []map[string]string{{"id": id1, "title": "Plat du jour — édition", "priceLabel": "2 800 F"}},
	})
	if status != http.StatusOK {
		t.Fatalf("PUT #2 : statut %d, corps %v", status, out)
	}
	tenant, _ = out["tenant"].(map[string]any)
	_ = json.Unmarshal([]byte(tenant["portalPromos"].(string)), &promos)
	if promos[0]["id"] != id1 {
		t.Fatalf("id DOIT survivre à l'édition (analytics), avant %s après %v", id1, promos[0]["id"])
	}

	// Ids hérités (pré-N°56) : le contenu sans id reçoit un id déterministe
	// côté lecture (portalHospitality/promoIDsOf) — le portail peut tracker
	// sans attendre un ré-enregistrement.
	_, out = doJSON(t, ts, "GET", "/api/settings", token, nil)
	tenant, _ = out["tenant"].(map[string]any)
	if key, _ := tenant["portalKey"].(string); len(key) != 16 {
		t.Fatalf("portalKey stable attendue, reçue %q", key)
	}

	// Lien http:// refusé (portail mixte-content-free) et id falsifié refusé.
	status, out = doJSON(t, ts, "PUT", "/api/settings", token, map[string]any{
		"portalPromos": []map[string]string{{"title": "X", "link": "http://exemple.ci"}},
	})
	if status != http.StatusBadRequest {
		t.Fatalf("lien http:// : statut %d (400 attendu), corps %v", status, out)
	}
	status, _ = doJSON(t, ts, "PUT", "/api/settings", token, map[string]any{
		"portalPromos": []map[string]string{{"id": "TROP!!", "title": "X"}},
	})
	if status != http.StatusBadRequest {
		t.Fatalf("id falsifié : statut %d (400 attendu)", status)
	}
}
