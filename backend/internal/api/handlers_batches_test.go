// Tests v2 « tour de contrôle du stock » de l'onglet Lots.
//
// La liste des lots est interrogée par la surface publique réelle
// (httptest.Server sur api.Handler(), store JSON éphémère) après semis
// direct de lots et de vouchers pour contrôler exactement SoldAt/UsedAt :
// la fenêtre 7 j (UTC) et la dormance dépendent de l'horloge, les dates de
// test sont donc posées relativement à time.Now().
package api

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"mikcloud/hotspot-api/internal/model"
	"mikcloud/hotspot-api/internal/store"
)

// seedBatchV2 — lot semé avec sa date de création exacte (dormance).
func seedBatchV2(t *testing.T, st *store.Store, id, accID string, createdAt time.Time) {
	t.Helper()
	st.Lock()
	st.Data().Batches = append(st.Data().Batches, model.Batch{
		ID: id, AccountID: accID, ProfileName: "1h 100F", RouterName: "site-v2",
		Count: 6, UnitPrice: 100, TotalCost: 600, Channel: "direct",
		CreatedAt: createdAt.Format(time.RFC3339),
	})
	st.Save()
	st.Unlock()
}

// seedInsightVoucher — voucher de lot semé avec son état de sortie exact
// (SoldAt/UsedAt vides ou horodatés) et son détenteur.
func seedInsightVoucher(t *testing.T, st *store.Store, id, accID, batchID, resellerID, code string, price, sellingPrice int, soldAt, usedAt time.Time) {
	t.Helper()
	sold, used := "", ""
	if !soldAt.IsZero() {
		sold = soldAt.Format(time.RFC3339)
	}
	if !usedAt.IsZero() {
		used = usedAt.Format(time.RFC3339)
	}
	st.Lock()
	st.Data().HotspotUsers = append(st.Data().HotspotUsers, model.HotspotUser{
		ID: id, AccountID: accID, Kind: "voucher",
		Username: code, Password: "pass-" + code,
		ProfileName: "1h 100F", RouterName: "site-v2", BatchID: batchID,
		Status: "active", ResellerID: resellerID, ResellerName: "Xavier",
		CreatedAt: model.NowISO(),
		ExpiresAt: time.Now().UTC().Add(30 * 24 * time.Hour).Format(time.RFC3339),
		Price:     price, SellingPrice: sellingPrice,
		SoldAt: sold, UsedAt: used,
	})
	st.Save()
	st.Unlock()
}

// emptyTime — zéro horodatage : le seeder n'écrit pas le champ.
func emptyTime() time.Time { return time.Time{} }

// TestBatchesInsights — vélocité (sorties 7 j SANS double comptage SoldAt +
// UsedAt d'un même ticket), dormance (dernière sortie, sinon création du lot
// si rien n'est jamais sorti, lot vivant uniquement), marge en attente
// (face − gros sur le vendable), pipeline revendeurs du summary et filtre
// détenteur « resellers ».
func TestBatchesInsights(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	token, accID, _ := registerAccount(t, ts, "gerant-lots-v2", "")
	seedSellReseller(t, st, "res-x", accID, "Xavier", "prepaid", 0)

	now := time.Now().UTC()
	seedBatchV2(t, st, "bat-v2a", accID, now.Add(-60*24*time.Hour))
	seedBatchV2(t, st, "bat-v2b", accID, now.Add(-9*24*time.Hour))

	// Lot A — 6 tickets 100 gros / 150 public :
	//   a1 vendu il y a 2 j (SoldAt seul — vente directe gérant)
	//   a2 vendu il y a 3 j ET consommé il y a 1 j → ne compte qu'UNE fois
	//   a3 consommé il y a 10 j (hors fenêtre 7 j)
	//   a4/a5 stock vendable chez le revendeur Xavier (marge 2×50)
	//   a6 vendu il y a 40 j, consommé il y a 39 j (hors fenêtre)
	seedInsightVoucher(t, st, "v-a1", accID, "bat-v2a", "", "code-a1", 100, 150, now.Add(-2*24*time.Hour), emptyTime())
	seedInsightVoucher(t, st, "v-a2", accID, "bat-v2a", "", "code-a2", 100, 150, now.Add(-3*24*time.Hour), now.Add(-1*24*time.Hour))
	seedInsightVoucher(t, st, "v-a3", accID, "bat-v2a", "", "code-a3", 100, 150, emptyTime(), now.Add(-10*24*time.Hour))
	seedInsightVoucher(t, st, "v-a4", accID, "bat-v2a", "res-x", "code-a4", 100, 150, emptyTime(), emptyTime())
	seedInsightVoucher(t, st, "v-a5", accID, "bat-v2a", "res-x", "code-a5", 100, 150, emptyTime(), emptyTime())
	seedInsightVoucher(t, st, "v-a6", accID, "bat-v2a", "", "code-a6", 100, 150, now.Add(-40*24*time.Hour), now.Add(-39*24*time.Hour))
	// Lot B — vivant, jamais rien sorti : dormance lue depuis la création (9 j).
	seedInsightVoucher(t, st, "v-b1", accID, "bat-v2b", "", "code-b1", 100, 150, emptyTime(), emptyTime())

	status, out := doJSON(t, ts, http.MethodGet, "/api/vouchers/batches", token, nil)
	if status != http.StatusOK {
		t.Fatalf("liste des lots : statut %d, corps %v", status, out)
	}
	data, _ := out["data"].([]any)
	if len(data) != 2 {
		t.Fatalf("2 lots attendus, reçu %d", len(data))
	}
	byID := map[string]map[string]any{}
	for _, raw := range data {
		row, _ := raw.(map[string]any)
		byID[row["id"].(string)] = row
	}

	// Lot A — vélocité sans double comptage, dormance = dernier mouvement de
	// sortie (UsedAt de a2, il y a 1 j — le plus récent).
	a := byID["bat-v2a"]
	if got := int(a["sold7d"].(float64)); got != 2 {
		t.Errorf("bat-v2a sold7d = %d, want 2 (a1 + a2 une seule fois)", got)
	}
	if got := int(a["transferable"].(float64)); got != 2 {
		t.Errorf("bat-v2a transferable = %d, want 2", got)
	}
	if got := int(a["stockFace"].(float64)); got != 300 {
		t.Errorf("bat-v2a stockFace = %d, want 300 (2×150)", got)
	}
	if got := int(a["transferableValue"].(float64)); got != 200 {
		t.Errorf("bat-v2a transferableValue = %d, want 200 (2×100 gros)", got)
	}
	if got := int(a["marginPending"].(float64)); got != 100 {
		t.Errorf("bat-v2a marginPending = %d, want 100", got)
	}
	if got := int(a["dormantDays"].(float64)); got != 1 {
		t.Errorf("bat-v2a dormantDays = %d, want 1 (UsedAt a2, dernier mouvement)", got)
	}
	if a["lastEgressAt"] == "" {
		t.Error("bat-v2a lastEgressAt doit être renseigné (lot vivant)")
	}

	// Lot B — dormance depuis la création (9 j), aucune sortie.
	b := byID["bat-v2b"]
	if got := int(b["sold7d"].(float64)); got != 0 {
		t.Errorf("bat-v2b sold7d = %d, want 0", got)
	}
	if got := int(b["dormantDays"].(float64)); got != 9 {
		t.Errorf("bat-v2b dormantDays = %d, want 9 (création)", got)
	}
	if got := int(b["marginPending"].(float64)); got != 50 {
		t.Errorf("bat-v2b marginPending = %d, want 50", got)
	}

	// Summary — pipeline métier sur l'ensemble FILTRÉ (ici : tout).
	sum, _ := out["summary"].(map[string]any)
	if got := int(sum["sold7d"].(float64)); got != 2 {
		t.Errorf("summary sold7d = %d, want 2", got)
	}
	if got := int(sum["resellerStock"].(float64)); got != 2 {
		t.Errorf("summary resellerStock = %d, want 2", got)
	}
	if got := int(sum["resellerStockValue"].(float64)); got != 200 {
		t.Errorf("summary resellerStockValue = %d, want 200", got)
	}
	if got := int(sum["marginPending"].(float64)); got != 150 {
		t.Errorf("summary marginPending = %d, want 150", got)
	}
	if got := int(sum["stockFace"].(float64)); got != 450 {
		t.Errorf("summary stockFace = %d, want 450 (300+150)", got)
	}

	// Filtre détenteur « resellers » — seuls les lots avec du vendable chez
	// un revendeur passent (bat-v2a), le summary suit le filtre.
	status, out = doJSON(t, ts, http.MethodGet, "/api/vouchers/batches?holder=resellers", token, nil)
	if status != http.StatusOK {
		t.Fatalf("liste filtrée resellers : statut %d", status)
	}
	data, _ = out["data"].([]any)
	if len(data) != 1 {
		t.Fatalf("filtre holder=resellers : 1 lot attendu, reçu %d", len(data))
	}
	if row, _ := data[0].(map[string]any); row["id"] != "bat-v2a" {
		t.Errorf("filtre holder=resellers : bat-v2a attendu, reçu %v", row["id"])
	}
	sum, _ = out["summary"].(map[string]any)
	if got := int(sum["resellerStock"].(float64)); got != 2 {
		t.Errorf("summary filtré resellerStock = %d, want 2", got)
	}

	// Filtre détenteur « direct » — le lot B seul (vendable jamais remis).
	status, out = doJSON(t, ts, http.MethodGet, "/api/vouchers/batches?holder=direct", token, nil)
	if status != http.StatusOK {
		t.Fatalf("liste filtrée direct : statut %d", status)
	}
	data, _ = out["data"].([]any)
	if len(data) != 1 {
		t.Fatalf("filtre holder=direct : 1 lot attendu, reçu %d", len(data))
	}

	// Export CSV — 3 colonnes additives (sorties 7 j, dormance, marge).
	status, _, body := doRaw(t, ts, ts.URL+"/api/vouchers/batches/export", token)
	if status != http.StatusOK {
		t.Fatalf("export CSV : statut %d", status)
	}
	if !strings.Contains(body, "Ecoules 7j ;Dormance j ;Marge en attente") {
		t.Error("entête CSV : colonnes v2 manquantes")
	}
	if !strings.Contains(body, "600 ;2 ;1 ;100 ;") {
		t.Error("ligne CSV bat-v2a : sorties/dormance/marge inattendues")
	}
}
