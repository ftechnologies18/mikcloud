// Tests du cycle de vie revendeur (audit purge/revendeurs — V1→V5) :
//   - V1 : garde-fous 409 au DELETE — crédit restant, créance dépôt-vente,
//     stock attribué encore vendable (réponse structurée credit/debt/stock) ;
//   - V2 : cascade de suppression — transactions purgées, vouchers attribués
//     détachés (ResellerID vidé, ResellerName conservé en trace), ventes
//     (Sale) et lots (Batch) volontairement conservés ;
//   - V3 : purge scope « resellers » couvrant les transactions ORPHELINES
//     (reliques de DELETE antérieurs à la cascade) sans toucher aux autres
//     comptes ;
//   - V4 : révocation du Mode Vente — un token reseller qui survit au DELETE
//     est refusé 403 sur tout /api/sell/*.
//
// AUCUNE connexion réseau : store JSON éphémère + httptest.Server (surface
// publique réelle), tokens forgés avec le secret de test. Les helpers
// seedSellReseller/seedStockVoucher/seedTransaction/newPurgeTestServer sont
// partagés depuis les autres fichiers de test du package.
package api

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"mikcloud/hotspot-api/internal/auth"
	"mikcloud/hotspot-api/internal/model"
)

// TestResellerDeleteGuards — V1 : la suppression est refusée tant que le
// revendeur porte une valeur active, avec un motif détaillé et structuré.
func TestResellerDeleteGuards(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	token, accID, _ := registerAccount(t, ts, "gerant-guards", "")

	// 1) Crédit restant : recharge réaliste via l'API, puis DELETE → 409.
	seedSellReseller(t, st, "res-guard-credit", accID, "Riche", "prepaid", 0)
	status, out := doJSON(t, ts, "POST", "/api/resellers/res-guard-credit/credit", token, map[string]any{"amount": 5000})
	if status != http.StatusOK {
		t.Fatalf("recharge crédit : statut %d (%v)", status, out)
	}
	status, out = doJSON(t, ts, "DELETE", "/api/resellers/res-guard-credit", token, nil)
	if status != http.StatusConflict {
		t.Fatalf("garde crédit : statut %d attendu 409 (%v)", status, out)
	}
	if msg, _ := out["error"].(string); !strings.Contains(msg, "crédit restant") {
		t.Fatalf("garde crédit : message inattendu %v", out["error"])
	}
	if got, _ := out["credit"].(float64); int(got) != 5000 {
		t.Fatalf("garde crédit : payload credit attendu 5000, obtenu %v", out["credit"])
	}

	// 2) Créance dépôt-vente non soldée → 409.
	seedSellReseller(t, st, "res-guard-debt", accID, "Dettes", "deposit", 10000)
	seedTransaction(t, st, accID, "res-guard-debt", "debt", 2000, time.Now())
	status, out = doJSON(t, ts, "DELETE", "/api/resellers/res-guard-debt", token, nil)
	if status != http.StatusConflict {
		t.Fatalf("garde créance : statut %d attendu 409 (%v)", status, out)
	}
	if msg, _ := out["error"].(string); !strings.Contains(msg, "créance") {
		t.Fatalf("garde créance : message inattendu %v", out["error"])
	}

	// 3) Stock attribué encore vendable → 409.
	seedSellReseller(t, st, "res-guard-stock", accID, "Stocké", "prepaid", 0)
	seedStockVoucher(t, st, "hu-guard-stock", accID, "res-guard-stock", "vk-stock", 100, 150, "", "", false)
	status, out = doJSON(t, ts, "DELETE", "/api/resellers/res-guard-stock", token, nil)
	if status != http.StatusConflict {
		t.Fatalf("garde stock : statut %d attendu 409 (%v)", status, out)
	}
	if msg, _ := out["error"].(string); !strings.Contains(msg, "en stock") {
		t.Fatalf("garde stock : message inattendu %v", out["error"])
	}

	// Aucun des trois revendeurs ne doit avoir disparu.
	st.Lock()
	if n := len(st.Data().Resellers); n != 3 {
		t.Fatalf("revendeurs restants attendus 3, obtenu %d", n)
	}
	st.Unlock()
}

// TestResellerDeleteCascade — V2 : un revendeur « soldé » (dette nette nulle,
// crédit 0, stock vendable nul) est supprimé avec TOUT son historique de
// transactions ; ses vouchers attribués sont détachés (nom conservé) ; les
// ventes et lots tracés à son nom sont conservés (comptabilité du gérant).
func TestResellerDeleteCascade(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	token, accID, _ := registerAccount(t, ts, "gerant-cascade", "")

	seedSellReseller(t, st, "res-casc", accID, "Cascading", "deposit", 10000)
	seedTransaction(t, st, accID, "res-casc", "credit", 1000, time.Now())
	seedTransaction(t, st, accID, "res-casc", "debt", 2000, time.Now())
	seedTransaction(t, st, accID, "res-casc", "settlement", 2000, time.Now())
	seedStockVoucher(t, st, "hu-casc-sold", accID, "res-casc", "vk-sold", 100, 150,
		time.Now().UTC().Format(time.RFC3339), "sell_mode", true)
	// Voucher expiré non vendu : non compté comme stock vendable, mais
	// détaché lui aussi (aucun orphelin ne doit survivre au DELETE).
	st.Lock()
	st.Data().HotspotUsers = append(st.Data().HotspotUsers, model.HotspotUser{
		ID: "hu-casc-exp", AccountID: accID, Kind: "voucher",
		Username: "vk-exp", Password: "pass-exp",
		ProfileName: "1 Heure", RouterName: "site-test",
		Status: "active", ResellerID: "res-casc", ResellerName: "Cascading",
		CreatedAt: model.NowISO(),
		ExpiresAt: time.Now().UTC().Add(-time.Hour).Format(time.RFC3339),
		Price:     100,
	})
	st.Save()
	st.Unlock()
	// Vente + lot tracés au nom du revendeur : conservés volontairement.
	st.Lock()
	db := st.Data()
	db.Sales = append(db.Sales, model.Sale{
		ID: "sl-casc", AccountID: accID, Amount: 1500, Count: 10,
		Channel: "reseller", ResellerName: "Cascading", At: model.NowISO(),
	})
	db.Batches = append(db.Batches, model.Batch{
		ID: "bt-casc", AccountID: accID, Channel: "reseller",
		ResellerID: "res-casc", ResellerName: "Cascading", Count: 10,
		CreatedAt: model.NowISO(),
	})
	st.Save()
	st.Unlock()

	status, out := doJSON(t, ts, "DELETE", "/api/resellers/res-casc", token, nil)
	if status != http.StatusOK {
		t.Fatalf("delete cascade : statut %d (%v)", status, out)
	}
	if got, _ := out["transactionsPurged"].(float64); int(got) != 3 {
		t.Fatalf("transactions purgées attendues 3, obtenu %v", out["transactionsPurged"])
	}
	if got, _ := out["vouchersDetached"].(float64); int(got) != 2 {
		t.Fatalf("vouchers détachés attendus 2, obtenu %v", out["vouchersDetached"])
	}

	st.Lock()
	db = st.Data()
	if n := len(db.Resellers); n != 0 {
		t.Fatalf("revendeur restant : %d", n)
	}
	if n := len(db.Transactions); n != 0 {
		t.Fatalf("transactions restantes attendues 0, obtenu %d", n)
	}
	for i := range db.HotspotUsers {
		u := &db.HotspotUsers[i]
		if u.ResellerID != "" {
			t.Fatalf("voucher %s non détaché : ResellerID=%q", u.ID, u.ResellerID)
		}
		if u.ResellerName == "" {
			t.Fatalf("trace du nom perdue pour %s", u.ID)
		}
	}
	if n := len(db.Sales); n != 1 {
		t.Fatalf("vente doit être conservée, restantes : %d", n)
	}
	if n := len(db.Batches); n != 1 {
		t.Fatalf("lot doit être conservé, restants : %d", n)
	}
	st.Unlock()
}

// TestPurgeResellersCoversOrphans — V3 : le scope « resellers » purge les
// transactions des revendeurs supprimés ET les orphelines (ResellerID sans
// revendeur), tout en préservant strictement les autres comptes.
func TestPurgeResellersCoversOrphans(t *testing.T) {
	st, ts, platToken := newPurgeTestServer(t)
	accA, accB := "acc-orp-a", "acc-orp-b"
	st.Lock()
	db := st.Data()
	db.Accounts = append(db.Accounts,
		model.Account{ID: accA, Name: "Orphelins A", Status: "active", CreatedAt: model.NowISO()},
		model.Account{ID: accB, Name: "Témoin B", Status: "active", CreatedAt: model.NowISO()},
	)
	db.Resellers = append(db.Resellers,
		model.Reseller{ID: "res-live-a", AccountID: accA, Name: "Vivant A", Status: "active", CreatedAt: model.NowISO()},
		model.Reseller{ID: "res-b", AccountID: accB, Name: "Témoin B", Status: "active", CreatedAt: model.NowISO()},
	)
	db.Transactions = append(db.Transactions,
		model.Transaction{ID: "tx-a-live", AccountID: accA, Type: "credit", ResellerID: "res-live-a", ResellerName: "Vivant A", Amount: 500, At: model.NowISO()},
		model.Transaction{ID: "tx-a-ghost", AccountID: accA, Type: "debt", ResellerID: "res-ghost", ResellerName: "Fantôme", Amount: 1000, At: model.NowISO()},
		model.Transaction{ID: "tx-a-noid", AccountID: accA, Type: "settlement", ResellerID: "", ResellerName: "Anonyme", Amount: 50, At: model.NowISO()},
		model.Transaction{ID: "tx-b-live", AccountID: accB, Type: "credit", ResellerID: "res-b", ResellerName: "Témoin B", Amount: 700, At: model.NowISO()},
	)
	st.Save()
	st.Unlock()

	status, out := doJSON(t, ts, "POST", "/api/admin/purge", platToken, map[string]any{
		"accountId": accA,
		"scopes":    []string{"resellers"},
	})
	if status != http.StatusOK {
		t.Fatalf("purge ciblée : statut %d (%v)", status, out)
	}
	if got := purgeCountIn(t, out, "resellers"); got != 1 {
		t.Fatalf("purge : revendeurs attendus 1, obtenu %d", got)
	}
	if got := purgeCountIn(t, out, "transactions"); got != 3 {
		t.Fatalf("purge : transactions attendues 3 (vivante + 2 orphelines), obtenu %d", got)
	}

	st.Lock()
	defer st.Unlock()
	db = st.Data()
	if n := countBy(db.Resellers, accA, func(r model.Reseller) string { return r.AccountID }); n != 0 {
		t.Fatalf("A : %d revendeur(s) restant(s), attendu 0", n)
	}
	if n := countBy(db.Transactions, accA, func(x model.Transaction) string { return x.AccountID }); n != 0 {
		t.Fatalf("A : %d transaction(s) restante(s) (orphelines non purgées ?), attendu 0", n)
	}
	if n := countBy(db.Resellers, accB, func(r model.Reseller) string { return r.AccountID }); n != 1 {
		t.Fatalf("B (témoin) : revendeur restant attendu 1, obtenu %d", n)
	}
	if n := countBy(db.Transactions, accB, func(x model.Transaction) string { return x.AccountID }); n != 1 {
		t.Fatalf("B (témoin) : transaction restante attendue 1, obtenu %d", n)
	}
}

// TestSellModeRevokedAfterResellerDelete — V4 : le DELETE du revendeur révoque
// de fait son Mode Vente : le token survivant (TTL 24 h) est refusé 403 sur
// tout /api/sell/* dès la requête suivante.
func TestSellModeRevokedAfterResellerDelete(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	token, accID, _ := registerAccount(t, ts, "gerant-revoke", "")
	seedSellReseller(t, st, "res-revoke", accID, "Vendeur", "prepaid", 0)
	// Voucher DÉJÀ VENDU : un revendeur avec du stock vendable serait bloqué
	// par le garde-fou V1 (comportement attendu) — on teste ici la révocation,
	// donc le revendeur doit être « soldé » pour que le DELETE passe.
	seedStockVoucher(t, st, "hu-revoke", accID, "res-revoke", "vk-revoke", 100, 150,
		time.Now().UTC().Format(time.RFC3339), "sell_mode", false)
	resToken := auth.Sign(testJWTSecret, auth.NewClaims("res-revoke", "Vendeur", "reseller", accID, 0))

	// Avant suppression : le stock du Mode Vente répond (200).
	status, _ := doJSON(t, ts, "GET", "/api/sell/stock", resToken, nil)
	if status != http.StatusOK {
		t.Fatalf("stock avant suppression : statut %d attendu 200", status)
	}

	// DELETE du revendeur (soldé : cascade immédiate).
	status, out := doJSON(t, ts, "DELETE", "/api/resellers/res-revoke", token, nil)
	if status != http.StatusOK {
		t.Fatalf("delete revendeur : statut %d (%v)", status, out)
	}

	// Après : le token reseller survivant ne peut plus rien faire.
	status, out = doJSON(t, ts, "GET", "/api/sell/stock", resToken, nil)
	if status != http.StatusForbidden {
		t.Fatalf("stock après suppression : statut %d attendu 403 (%v)", status, out)
	}
	if msg, _ := out["error"].(string); !strings.Contains(msg, "revendeur supprimé") {
		t.Fatalf("message de révocation inattendu : %v", out["error"])
	}
}
