// Tests N°22 — protection des codes des tickets revendeur côté console gérant :
//   - masquage des listes (/api/vouchers, /api/users) et de l'export CSV :
//     tout voucher attribué (ResellerID != "") sort avec username « •••••• »
//     et mot de passe vide ; le stock direct du gérant reste en clair ; le
//     filtre `search` continue de tourner sur les VRAIS codes (vérifier un
//     ticket papier qui revient au comptoir reste possible) ;
//   - canal d'impression tracé (POST /api/vouchers/print) : codes complets
//     rendus pour l'impression en cours + trace d'activité pour les tickets
//     revendeur (aucune trace pour le stock direct, comportement historique) ;
//   - garde de mise à jour : le code d'un ticket attribué n'est pas modifiable
//     depuis la console (403 structuré), le commentaire reste éditable et la
//     réponse PUT est masquée (pas de fuite par l'action unitaire).
//
// AUCUNE connexion réseau : store JSON éphémère + httptest.Server (surface
// publique réelle), helpers partagés du package (newTestServerWithStore,
// registerAccount, doJSON).
package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mikcloud/hotspot-api/internal/model"
	"mikcloud/hotspot-api/internal/store"
)

// seedCodeVouchers — un voucher stock direct (gérant) + un voucher attribué à
// « res-mask » (Fatou), codes distincts, mêmes autres attributs.
func seedCodeVouchers(t *testing.T, st *store.Store, accID string) {
	t.Helper()
	st.Lock()
	st.Data().Resellers = append(st.Data().Resellers, model.Reseller{
		ID: "res-mask", AccountID: accID, Name: "Fatou", Username: "fatou",
		PaymentMode: "prepaid", DebtCeiling: 0, Status: "active",
		CreatedAt: model.NowISO(),
	})
	expires := time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339)
	st.Data().HotspotUsers = append(st.Data().HotspotUsers,
		model.HotspotUser{
			ID: "hu-direct", AccountID: accID, Kind: "voucher",
			Username: "DIRECT1", Password: "pass-direct",
			ProfileName: "1 Heure", RouterName: "site-test",
			Status: "active", CreatedAt: model.NowISO(),
			ExpiresAt: expires, Price: 100,
		},
		model.HotspotUser{
			ID: "hu-res", AccountID: accID, Kind: "voucher",
			Username: "SECRET1", Password: "pass-secret",
			ProfileName: "1 Heure", RouterName: "site-test",
			Status: "active", ResellerID: "res-mask", ResellerName: "Fatou",
			CreatedAt: model.NowISO(),
			ExpiresAt: expires, Price: 100,
		})
	st.Save()
	st.Unlock()
}

// rawGet — GET brut (réponses non-JSON : CSV) → (statut, corps).
func rawGet(t *testing.T, ts *httptest.Server, url, token string) (int, string) {
	t.Helper()
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		t.Fatalf("requête impossible : %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s : %v", url, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

// TestResellerVoucherCodesMaskedInLists — les listes console masquent les
// codes revendeur, pas ceux du stock direct ; la recherche par code réel
// fonctionne toujours (ligne masquée en sortie).
func TestResellerVoucherCodesMaskedInLists(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	token, accID, _ := registerAccount(t, ts, "gerant-mask", "")
	seedCodeVouchers(t, st, accID)

	for _, path := range []string{"/api/vouchers", "/api/users"} {
		status, out := doJSON(t, ts, "GET", path, token, nil)
		if status != http.StatusOK {
			t.Fatalf("%s : statut %d", path, status)
		}
		rows, _ := out["data"].([]any)
		if len(rows) != 2 {
			t.Fatalf("%s : 2 lignes attendues, obtenu %d", path, len(rows))
		}
		seen := map[string]map[string]any{}
		for _, raw := range rows {
			row, _ := raw.(map[string]any)
			id, _ := row["id"].(string)
			seen[id] = row
		}
		direct, res := seen["hu-direct"], seen["hu-res"]
		if direct == nil || res == nil {
			t.Fatalf("%s : lignes attendues hu-direct + hu-res", path)
		}
		if code, _ := direct["username"].(string); code != "DIRECT1" {
			t.Fatalf("%s : stock direct doit rester en clair, obtenu %q", path, code)
		}
		if code, _ := res["username"].(string); code != "••••••" {
			t.Fatalf("%s : code revendeur non masqué, obtenu %q", path, code)
		}
		if pass, _ := res["password"].(string); pass != "" {
			t.Fatalf("%s : mot de passe revendeur non vidé, obtenu %q", path, pass)
		}
		// La propriété reste lisible (identification côté gérant).
		if name, _ := res["resellerName"].(string); name != "Fatou" {
			t.Fatalf("%s : resellerName attendu « Fatou », obtenu %q", path, name)
		}
	}

	// Export CSV : mêmes règles (code masqué, mot de passe vide).
	status, body := rawGet(t, ts, ts.URL+"/api/users/export", token)
	if status != http.StatusOK {
		t.Fatalf("export CSV : statut %d", status)
	}
	if !strings.Contains(body, "••••••;;") && !strings.Contains(body, "••••••;") {
		t.Fatalf("export CSV : code revendeur masqué attendu dans %q", body)
	}
	if strings.Contains(body, "SECRET1") || strings.Contains(body, "pass-secret") {
		t.Fatalf("export CSV : le code revendeur fuit (%q)", body)
	}
	if !strings.Contains(body, "DIRECT1") || !strings.Contains(body, "pass-direct") {
		t.Fatalf("export CSV : le stock direct doit rester en clair (%q)", body)
	}

	// Recherche par le VRAI code : la ligne ressort (masquée) — vérifier un
	// ticket papier qui revient au comptoir reste possible.
	status, out := doJSON(t, ts, "GET", "/api/vouchers?search=SECRET1", token, nil)
	if status != http.StatusOK {
		t.Fatalf("recherche code réel : statut %d", status)
	}
	if total, _ := out["total"].(float64); int(total) != 1 {
		t.Fatalf("recherche code réel : 1 résultat attendu, obtenu %v", out["total"])
	}
}

// TestResellerVoucherPrintChannelTraced — POST /api/vouchers/print rend les
// codes complets et trace la remise pour les tickets revendeur uniquement.
func TestResellerVoucherPrintChannelTraced(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	token, accID, _ := registerAccount(t, ts, "gerant-print", "")
	seedCodeVouchers(t, st, accID)

	countTraces := func() int {
		st.Lock()
		defer st.Unlock()
		n := 0
		for _, act := range st.Data().Activity {
			if act.AccountID == accID && strings.Contains(act.Message, "Codes remis pour impression") {
				n++
			}
		}
		return n
	}

	// 1) Impression mixte (direct + revendeur) : codes complets, trace unique.
	status, out := doJSON(t, ts, "POST", "/api/vouchers/print", token,
		map[string]any{"ids": []string{"hu-direct", "hu-res"}})
	if status != http.StatusOK {
		t.Fatalf("impression mixte : statut %d (%v)", status, out)
	}
	vs, _ := out["vouchers"].([]any)
	if len(vs) != 2 {
		t.Fatalf("impression mixte : 2 tickets attendus, obtenu %d", len(vs))
	}
	for _, raw := range vs {
		row, _ := raw.(map[string]any)
		id, _ := row["id"].(string)
		code, _ := row["username"].(string)
		if id == "hu-res" && code != "SECRET1" {
			t.Fatalf("canal d'impression : code complet attendu pour hu-res, obtenu %q", code)
		}
		if id == "hu-direct" && code != "DIRECT1" {
			t.Fatalf("canal d'impression : code direct inattendu %q", code)
		}
	}
	if traced, _ := out["tracedCount"].(float64); int(traced) != 1 {
		t.Fatalf("tracedCount attendu 1, obtenu %v", out["tracedCount"])
	}
	if n := countTraces(); n != 1 {
		t.Fatalf("exactement 1 trace d'activité attendue, obtenu %d", n)
	}
	// Détail de la trace : horodatée, par revendeur.
	st.Lock()
	detail := ""
	for _, act := range st.Data().Activity {
		if act.AccountID == accID && strings.Contains(act.Message, "Codes remis pour impression") {
			detail = act.Message
		}
	}
	st.Unlock()
	if !strings.Contains(detail, "Fatou : 1") {
		t.Fatalf("trace d'activité sans décompte par revendeur : %q", detail)
	}

	// 2) Impression stock direct seul : AUCUNE nouvelle trace (comportement
	// historique conservé).
	status, out = doJSON(t, ts, "POST", "/api/vouchers/print", token,
		map[string]any{"ids": []string{"hu-direct"}})
	if status != http.StatusOK {
		t.Fatalf("impression directe : statut %d (%v)", status, out)
	}
	if traced, _ := out["tracedCount"].(float64); int(traced) != 0 {
		t.Fatalf("impression directe : tracedCount attendu 0, obtenu %v", out["tracedCount"])
	}
	if n := countTraces(); n != 1 {
		t.Fatalf("l'impression du stock direct ne doit pas être tracée (%d traces)", n)
	}

	// 3) Identifiants inconnus / hors compte : ignorés.
	status, out = doJSON(t, ts, "POST", "/api/vouchers/print", token,
		map[string]any{"ids": []string{"hu-ghost"}})
	if status != http.StatusNotFound {
		t.Fatalf("identifiants inconnus : statut %d attendu 404", status)
	}
}

// TestResellerVoucherCodeUpdateLocked — le code d'un ticket attribué n'est pas
// modifiable depuis la console (403 structuré) ; le commentaire reste
// éditable et la réponse PUT est masquée ; le code stocké ne bouge pas.
func TestResellerVoucherCodeUpdateLocked(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	token, accID, _ := registerAccount(t, ts, "gerant-lock", "")
	seedCodeVouchers(t, st, accID)

	// 1) Réécriture du username → 403 reseller_voucher_locked.
	status, out := doJSON(t, ts, "PUT", "/api/users/hu-res", token,
		map[string]any{"username": "HACKED"})
	if status != http.StatusForbidden {
		t.Fatalf("réécriture username : statut %d attendu 403 (%v)", status, out)
	}
	if code, _ := out["code"].(string); code != "reseller_voucher_locked" {
		t.Fatalf("code d'erreur structuré attendu, obtenu %v", out["code"])
	}

	// 2) Réécriture du password → 403.
	status, out = doJSON(t, ts, "PUT", "/api/users/hu-res", token,
		map[string]any{"password": "HACKED"})
	if status != http.StatusForbidden {
		t.Fatalf("réécriture password : statut %d attendu 403 (%v)", status, out)
	}

	// 3) Écho strict du username inchangé + commentaire : OK, réponse masquée.
	status, out = doJSON(t, ts, "PUT", "/api/users/hu-res", token,
		map[string]any{"username": "SECRET1", "comment": "ticket vérifié"})
	if status != http.StatusOK {
		t.Fatalf("édition commentaire : statut %d (%v)", status, out)
	}
	if code, _ := out["username"].(string); code != "••••••" {
		t.Fatalf("réponse PUT non masquée : username %q", code)
	}
	if pass, _ := out["password"].(string); pass != "" {
		t.Fatalf("réponse PUT non masquée : password %q", pass)
	}

	// 4) Le code stocké n'a pas bougé et le commentaire est persisté.
	st.Lock()
	u := model.HotspotUser{}
	for _, item := range st.Data().HotspotUsers {
		if item.ID == "hu-res" {
			u = item
		}
	}
	st.Unlock()
	if u.Username != "SECRET1" || u.Password != "pass-secret" {
		t.Fatalf("code stocké modifié : %q / %q", u.Username, u.Password)
	}
	if u.Comment != "ticket vérifié" {
		t.Fatalf("commentaire non persisté : %q", u.Comment)
	}

	// 5) Le stock direct reste pleinement éditable (régression).
	status, out = doJSON(t, ts, "PUT", "/api/users/hu-direct", token,
		map[string]any{"username": "DIRECT2"})
	if status != http.StatusOK {
		t.Fatalf("édition stock direct : statut %d (%v)", status, out)
	}
	if code, _ := out["username"].(string); code != "DIRECT2" {
		t.Fatalf("stock direct : username attendu DIRECT2, obtenu %q", code)
	}
}
