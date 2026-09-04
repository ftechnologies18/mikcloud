// Tests du Mode Vente — rapport de journée enrichi + export comptable (P3-d).
//
// Surface publique réelle (httptest.Server sur api.Handler(), store JSON
// éphémère) : le rapport est interrogé avec un token « reseller » forgé
// (exempté du contrôle S1-A3, cf. TestRequireRoleMatrix). Les vouchers et
// transactions sont semés directement dans le store pour contrôler
// précisément SoldAt/SoldVia (frontière de jour UTC) ; les flux réels
// (vente tactile, retour de stock, rechargement) passent par l'API.
package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"mikcloud/hotspot-api/internal/auth"
	"mikcloud/hotspot-api/internal/model"
	"mikcloud/hotspot-api/internal/store"
)

// seedSellReseller — revendeur semé dans le store (mode et plafond contrôlés).
func seedSellReseller(t *testing.T, st *store.Store, id, accID, name, paymentMode string, ceiling int) {
	t.Helper()
	st.Lock()
	st.Data().Resellers = append(st.Data().Resellers, model.Reseller{
		ID: id, AccountID: accID, Name: name, Username: name,
		PaymentMode: paymentMode, DebtCeiling: ceiling, Status: "active",
		CreatedAt: model.NowISO(),
	})
	st.Save()
	st.Unlock()
}

// seedStockVoucher — voucher semé avec son état de vente exact.
func seedStockVoucher(t *testing.T, st *store.Store, id, accID, resellerID, code string, price, sellingPrice int, soldAt, soldVia string, creditSale bool) {
	t.Helper()
	st.Lock()
	st.Data().HotspotUsers = append(st.Data().HotspotUsers, model.HotspotUser{
		ID: id, AccountID: accID, Kind: "voucher",
		Username: code, Password: "pass-" + code,
		ProfileName: "1 Heure", RouterName: "site-test",
		Status: "active", ResellerID: resellerID, ResellerName: "Ulrich Test",
		CreatedAt: model.NowISO(),
		ExpiresAt: time.Now().UTC().Add(7 * 24 * time.Hour).Format(time.RFC3339),
		Price:     price, SellingPrice: sellingPrice, CreditSale: creditSale,
		SoldAt: soldAt, SoldVia: soldVia,
	})
	st.Save()
	st.Unlock()
}

// seedTransaction — écriture financière semée (dette, versement, …).
func seedTransaction(t *testing.T, st *store.Store, accID, resellerID, txType string, amount int, at time.Time) {
	t.Helper()
	st.Lock()
	st.Data().Transactions = append(st.Data().Transactions, model.Transaction{
		ID: model.NewID("tx-"), AccountID: accID, Type: txType,
		ResellerID: resellerID, ResellerName: "Ulrich Test",
		Amount: amount, Note: "écriture de test " + txType, At: at.Format(time.RFC3339),
	})
	st.Save()
	st.Unlock()
}

// doRaw — requête brute (réponse non-JSON : CSV…) → statut, en-têtes, corps.
func doRaw(t *testing.T, ts interface{ Client() *http.Client }, url, token string) (int, http.Header, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("requête impossible : %v", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("GET %s : %v", url, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("lecture du corps : %v", err)
	}
	return resp.StatusCode, resp.Header, string(body)
}

func resellerToken(id, accID string) string {
	return auth.Sign(testJWTSecret, auth.NewClaims(id, "Ulrich Test", "reseller", accID, 0))
}

// TestSellDayReportEnriched — P3-d : ventilation par canal, versements du
// jour, créance dépôt-vente, exclusion du hors-jour (hier / autre revendeur).
func TestSellDayReportEnriched(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	_, accID, _ := registerAccount(t, ts, "gerant-rapport", "")

	now := time.Now().UTC()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	yesterday := todayStart.Add(-26 * time.Hour)

	seedSellReseller(t, st, "res-p3d", accID, "Ulrich Test", "deposit", 50_000)
	// v1 : vendue aujourd'hui en tactile — VRAIE vente API (écrit la dette).
	seedStockVoucher(t, st, "v1", accID, "res-p3d", "CODETACT", 200, 200, "", "", true)
	// v2 : vendue aujourd'hui en auto-connexion (moteur agent) + sa dette.
	seedStockVoucher(t, st, "v2", accID, "res-p3d", "CODEAUTO", 100, 0, model.NowISO(), "auto_connect", true)
	seedTransaction(t, st, accID, "res-p3d", "debt", 100, now)
	// v3 : vendue HIER — hors journée.
	seedStockVoucher(t, st, "v3", accID, "res-p3d", "CODEHIER", 100, 100, yesterday.Format(time.RFC3339), "sell_mode", false)
	// v4 : stock courant.
	seedStockVoucher(t, st, "v4", accID, "res-p3d", "CODESTOCK", 150, 150, "", "", false)
	// v5 : vendue aujourd'hui mais par un AUTRE revendeur — isolation.
	seedSellReseller(t, st, "res-autre", accID, "Autre Reseller", "deposit", 10_000)
	seedStockVoucher(t, st, "v5", accID, "res-autre", "CODEAUTRE", 999, 999, model.NowISO(), "sell_mode", false)
	// Trésorerie : versement 100 aujourd'hui, 50 hier.
	seedTransaction(t, st, accID, "res-p3d", "settlement", 100, now)
	seedTransaction(t, st, accID, "res-p3d", "settlement", 50, yesterday)

	token := resellerToken("res-p3d", accID)

	// Vente tactile v1 via l'API réelle.
	status, out := doJSON(t, ts, "POST", "/api/sell/v1/sold", token, nil)
	if status != http.StatusOK {
		t.Fatalf("vente tactile : statut %d (%v)", status, out)
	}

	status, out = doJSON(t, ts, "GET", "/api/sell/day-report", token, nil)
	if status != http.StatusOK {
		t.Fatalf("rapport du jour : statut %d (%v)", status, out)
	}
	if got := int(out["soldCount"].(float64)); got != 2 {
		t.Fatalf("soldCount = %d, attendu 2 (hier et autre revendeur exclus) (%v)", got, out)
	}
	if got := int(out["revenue"].(float64)); got != 300 {
		t.Fatalf("revenue = %d, attendu 300 (200 tactile + 100 auto)", got)
	}
	byVia, _ := out["byVia"].(map[string]any)
	if byVia["sell_mode"] != float64(1) || byVia["auto_connect"] != float64(1) {
		t.Fatalf("byVia inattendu : %v", byVia)
	}
	sold, _ := out["sold"].([]any)
	viaSeen := map[string]bool{}
	for _, row := range sold {
		s, _ := row.(map[string]any)
		viaSeen[s["soldVia"].(string)] = true
	}
	if !viaSeen["sell_mode"] || !viaSeen["auto_connect"] {
		t.Fatalf("soldVia des lignes incomplet : %v", sold)
	}
	if got := int(out["stockCount"].(float64)); got != 1 {
		t.Fatalf("stockCount = %d, attendu 1 (v4)", got)
	}
	// Dépôt-vente : à verser = recette du jour ; créance = dettes − versements.
	if got := int(out["toDeposit"].(float64)); got != 300 {
		t.Fatalf("toDeposit = %d, attendu 300", got)
	}
	if got := int(out["settledToday"].(float64)); got != 100 {
		t.Fatalf("settledToday = %d, attendu 100 (le versement d'hier est hors jour)", got)
	}
	if got := int(out["debtTotal"].(float64)); got != 150 {
		t.Fatalf("debtTotal = %d, attendu 150 (dettes 300 − versements 100 et 50, tous deux déduits de la créance)", got)
	}
	// returnedCount est omitempty : absent (donc 0) tant qu'aucun retour cash.
	if got, _ := out["returnedCount"].(float64); got != 0 {
		t.Fatalf("returnedCount = %v, attendu 0", out["returnedCount"])
	}
}

// TestSellDayReportCSV — P3-d : l'export « journal de caisse » liste ventes,
// retours (flux cash) et versements ; les rechargements n'y entrent pas ;
// date passée admise, date invalide refusée.
func TestSellDayReportCSV(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	ownerToken, accID, _ := registerAccount(t, ts, "gerant-csv", "")

	now := time.Now().UTC()
	seedSellReseller(t, st, "res-csv", accID, "Ulrich Test", "prepaid", 0)
	// v1 vendue aujourd'hui (tactile, semée), v3 vendue (auto), v2 en stock.
	seedStockVoucher(t, st, "v1", accID, "res-csv", "CSVVENTE", 200, 200, model.NowISO(), "sell_mode", false)
	seedStockVoucher(t, st, "v3", accID, "res-csv", "CSVAUTO", 100, 0, model.NowISO(), "auto_connect", false)
	seedStockVoucher(t, st, "v2", accID, "res-csv", "CSVSTOCK", 300, 300, "", "", false)

	token := resellerToken("res-csv", accID)

	// Retour de stock réel (prépayé → recrédit + transaction « Retour de stock »).
	status, out := doJSON(t, ts, "POST", "/api/sell/return", token, map[string]any{"ids": []string{"v2"}})
	if status != http.StatusOK {
		t.Fatalf("retour de stock : statut %d (%v)", status, out)
	}
	// Rechargement crédit par le gérant — flux hors tournée : jamais dans le journal.
	if status, out = doJSON(t, ts, "POST", "/api/resellers/res-csv/credit", ownerToken, map[string]any{"amount": 1000}); status != http.StatusOK {
		t.Fatalf("rechargement : statut %d (%v)", status, out)
	}

	status, header, body := doRaw(t, ts, ts.URL+"/api/sell/day-report.csv", token)
	if status != http.StatusOK {
		t.Fatalf("CSV : statut %d", status)
	}
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Fatalf("Content-Type inattendu : %q", ct)
	}
	if !strings.Contains(header.Get("Content-Disposition"), "journal-caisse-") {
		t.Fatalf("nom de fichier absent : %q", header.Get("Content-Disposition"))
	}
	if !strings.HasPrefix(body, "\xEF\xBB\xBF") {
		t.Fatal("le CSV doit commencer par un BOM UTF-8 (Excel)")
	}
	for _, want := range []string{
		"Journal de caisse",
		"CSVVENTE",       // vente tactile listée
		"CSVAUTO",        // vente auto listée
		"Tactile",        // canal tracé
		"Auto connexion", // canal tracé
		"Total ventes ;2 ; ;300 ;",
		"RETOURS",
		"Total retours ; ;300",  // recrédit du retour
		"Total versements ; ;0", // prépayé : aucun versement
		"Stock restant ;0 ;0",   // v2 rendue, v1/v3 vendues
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("CSV incomplet — %q absent du corps :\n%s", want, body)
		}
	}
	if strings.Contains(body, "Rechargement") || strings.Contains(body, "écriture de test") {
		t.Fatalf("le journal liste des écritures hors tournée :\n%s", body)
	}

	// Date passée : journal vide (aucune vente ce jour-là), lignes stock omises.
	yesterday := now.AddDate(0, 0, -1).Format("2006-01-02")
	status, _, body = doRaw(t, ts, ts.URL+"/api/sell/day-report.csv?date="+yesterday, token)
	if status != http.StatusOK {
		t.Fatalf("CSV date passée : statut %d", status)
	}
	if !strings.Contains(body, "Total ventes ;0 ; ;0 ;") || strings.Contains(body, "Stock restant") {
		t.Fatalf("journal d'une date passée inattendu :\n%s", body)
	}
	if strings.Contains(body, "CSVVENTE") {
		t.Fatalf("les ventes d'aujourd'hui fuient dans le journal d'hier :\n%s", body)
	}

	// Date invalide : 400 explicite.
	if status, _ := doJSON(t, ts, "GET", "/api/sell/day-report.csv?date=hier", token, nil); status != http.StatusBadRequest {
		t.Fatalf("date invalide doit répondre 400, obtenu %d", status)
	}
}

// TestSellStockPagination — P3-e : sans `limit`, /api/sell/stock renvoie le
// tableau historique complet ; avec `limit`, une page {items,total,hasMore}
// sur un tri stable (du plus récent au plus ancien).
func TestSellStockPagination(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	_, accID, _ := registerAccount(t, ts, "gerant-page", "")
	seedSellReseller(t, st, "res-page", accID, "Ulrich Test", "prepaid", 0)
	// 5 vouchers du revendeur + 1 hors stock (vendu) + 1 d'un autre revendeur.
	seedStockVoucher(t, st, "s1", accID, "res-page", "PAGEA", 100, 100, "", "", false)
	seedStockVoucher(t, st, "s2", accID, "res-page", "PAGEB", 100, 100, model.NowISO(), "sell_mode", false)
	seedStockVoucher(t, st, "s3", accID, "res-page", "PAGEC", 100, 100, "", "", false)
	seedStockVoucher(t, st, "s4", accID, "res-page", "PAGED", 100, 100, "", "", false)
	seedStockVoucher(t, st, "s5", accID, "res-page", "PAGEE", 100, 100, "", "", false)
	seedStockVoucher(t, st, "sx", accID, "res-autre", "PAGEX", 100, 100, "", "", false)
	// CreatedAt distincts pour un ordre déterministe (s5 = plus récent).
	st.Lock()
	created := map[string]string{
		"s1": "2026-09-01T10:00:00Z", "s2": "2026-09-02T10:00:00Z",
		"s3": "2026-09-03T10:00:00Z", "s4": "2026-09-04T10:00:00Z",
		"s5": "2026-09-05T10:00:00Z",
	}
	for i := range st.Data().HotspotUsers {
		if c, ok := created[st.Data().HotspotUsers[i].ID]; ok {
			st.Data().HotspotUsers[i].CreatedAt = c
		}
	}
	st.Save()
	st.Unlock()

	token := resellerToken("res-page", accID)

	// Sans limit : tableau brut historique — 4 tickets actifs du revendeur.
	status, _, body := doRaw(t, ts, ts.URL+"/api/sell/stock", token)
	if status != http.StatusOK {
		t.Fatalf("stock sans limit : statut %d", status)
	}
	var flat []map[string]any
	if err := json.Unmarshal([]byte(body), &flat); err != nil {
		t.Fatalf("sans limit, la réponse doit rester un tableau JSON : %v", err)
	}
	if len(flat) != 4 {
		t.Fatalf("stock complet = %d items, attendu 4 (vendu et autre revendeur exclus)", len(flat))
	}

	// Avec limit=2 : page {items,total,hasMore}, du plus récent au plus ancien.
	status, _, body = doRaw(t, ts, ts.URL+"/api/sell/stock?limit=2&offset=0", token)
	if status != http.StatusOK {
		t.Fatalf("stock paginé : statut %d", status)
	}
	var page struct {
		Items   []map[string]any `json:"items"`
		Total   int              `json:"total"`
		HasMore bool             `json:"hasMore"`
	}
	if err := json.Unmarshal([]byte(body), &page); err != nil {
		t.Fatalf("réponse paginée invalide : %v", err)
	}
	if page.Total != 4 || !page.HasMore || len(page.Items) != 2 {
		t.Fatalf("page 1 inattendue : total=%d hasMore=%v items=%d", page.Total, page.HasMore, len(page.Items))
	}
	if page.Items[0]["username"] != "PAGEE" || page.Items[1]["username"] != "PAGED" {
		t.Fatalf("tri anti-chronologique attendu, obtenu : %v / %v", page.Items[0]["username"], page.Items[1]["username"])
	}

	// offset=2 : dernière page — hasMore=false.
	status, _, body = doRaw(t, ts, ts.URL+"/api/sell/stock?limit=2&offset=2", token)
	if err := json.Unmarshal([]byte(body), &page); err != nil {
		t.Fatalf("réponse paginée invalide : %v", err)
	}
	if status != http.StatusOK || page.HasMore || len(page.Items) != 2 {
		t.Fatalf("page 2 inattendue : statut=%d hasMore=%v items=%d", status, page.HasMore, len(page.Items))
	}
	// offset au-delà du total : page vide, pas d'erreur.
	status, _, body = doRaw(t, ts, ts.URL+"/api/sell/stock?limit=2&offset=99", token)
	if err := json.Unmarshal([]byte(body), &page); err != nil {
		t.Fatalf("réponse paginée invalide : %v", err)
	}
	if status != http.StatusOK || page.HasMore || len(page.Items) != 0 {
		t.Fatalf("offset hors bornes doit donner une page vide : %v", page)
	}
}
