// Tests N°23 — reprise gérant (W6) et verrou de destruction du stock
// revendeur (W1) :
//   - POST /api/vouchers/reprise (requireRole 2) : miroir du retour de stock
//     revendeur (N°20) à l'initiative du gérant — propriété rendue au stock
//     direct, recrédit prépayé du stock VIVANT au prix GROS (dépôt-vente :
//     aucun), tickets vendus refusés (409 tout-le-lot), trace d'activité ;
//   - W1 : un ticket attribué n'est ni supprimable en unitaire (403), ni en
//     bulk (409), ni via la suppression de son lot (409) ; le nettoyage des
//     expirés l'épargne aussi — la reprise ferme la boucle (suppression OK
//     après reprise) ;
//   - W3/W4 : filtre `holder` des listes (direct vs revendeur).
//
// AUCUNE connexion réseau : store JSON éphémère + httptest.Server, helpers
// partagés du package (newTestServerWithStore, registerAccount, doJSON).
package api

import (
        "net/http"
        "strings"
        "testing"
        "time"

        "mikcloud/hotspot-api/internal/model"
)

// seedReprise — un revendeur prépayé « res-rep » (Ada) + un revendeur
// dépôt-vente « res-dep » (Bakary) ; stock : 1 voucher direct, 2 alloués
// vivants à Ada (100 chacun), 1 alloué vivant à Bakary (150), 1 alloué VENDU
// à Ada (SoldAt — créance née), 1 alloué EXPIRÉ à Ada (péri chez le
// revendeur — reprise sans recrédit).
func seedReprise(t *testing.T, st interface {
        Lock()
        Unlock()
        Data() *model.DB
        Save()
}, accID string) {
        t.Helper()
        st.Lock()
        db := st.Data()
        db.Resellers = append(db.Resellers,
                model.Reseller{
                        ID: "res-rep", AccountID: accID, Name: "Ada", Username: "ada",
                        PaymentMode: "prepaid", DebtCeiling: 0, Status: "active", Credit: 500,
                        CreatedAt: model.NowISO(),
                },
                model.Reseller{
                        ID: "res-dep", AccountID: accID, Name: "Bakary", Username: "bakary",
                        PaymentMode: "deposit", DebtCeiling: 0, Status: "active",
                        CreatedAt: model.NowISO(),
                })
        exp := time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339)
        past := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)
        db.HotspotUsers = append(db.HotspotUsers,
                model.HotspotUser{
                        ID: "hu-direct", AccountID: accID, Kind: "voucher",
                        Username: "DIRECT1", Password: "pass-direct",
                        ProfileName: "1 Heure", RouterName: "site-test",
                        Status: "active", CreatedAt: model.NowISO(),
                        ExpiresAt: exp, Price: 100,
                },
                model.HotspotUser{
                        // Membre direct du lot batch-w1 : garantit que la suppression de lot
                        // reste possible (200) après reprise + suppression unitaire de l'alloué.
                        ID: "hu-batch2", AccountID: accID, Kind: "voucher",
                        Username: "BATCH2", Password: "pass-b2",
                        ProfileName: "1 Heure", RouterName: "site-test",
                        Status: "active", CreatedAt: model.NowISO(), BatchID: "batch-w1",
                        ExpiresAt: exp, Price: 100,
                },
                model.HotspotUser{
                        ID: "hu-alive1", AccountID: accID, Kind: "voucher",
                        Username: "ALIVE1", Password: "pass-a1",
                        ProfileName: "1 Heure", RouterName: "site-test",
                        Status: "active", ResellerID: "res-rep", ResellerName: "Ada",
                        CreatedAt: model.NowISO(), ExpiresAt: exp, Price: 100,
                },
                model.HotspotUser{
                        ID: "hu-alive2", AccountID: accID, Kind: "voucher",
                        Username: "ALIVE2", Password: "pass-a2",
                        ProfileName: "1 Heure", RouterName: "site-test",
                        Status: "active", ResellerID: "res-rep", ResellerName: "Ada",
                        CreatedAt: model.NowISO(), ExpiresAt: exp, Price: 100,
                },
                model.HotspotUser{
                        ID: "hu-dep", AccountID: accID, Kind: "voucher",
                        Username: "DEPOSIT1", Password: "pass-d1",
                        ProfileName: "1 Heure", RouterName: "site-test",
                        Status: "active", ResellerID: "res-dep", ResellerName: "Bakary",
                        CreatedAt: model.NowISO(), ExpiresAt: exp, Price: 150,
                },
                model.HotspotUser{
                        ID: "hu-sold", AccountID: accID, Kind: "voucher",
                        Username: "SOLD1", Password: "pass-s1",
                        ProfileName: "1 Heure", RouterName: "site-test",
                        Status: "active", ResellerID: "res-rep", ResellerName: "Ada",
                        CreatedAt: model.NowISO(), ExpiresAt: exp, Price: 100,
                        SoldAt: model.NowISO(),
                },
                model.HotspotUser{
                        ID: "hu-expired", AccountID: accID, Kind: "voucher",
                        Username: "EXPIRED1", Password: "pass-e1",
                        ProfileName: "1 Heure", RouterName: "site-test",
                        Status: "active", ResellerID: "res-rep", ResellerName: "Ada",
                        CreatedAt: model.NowISO(), ExpiresAt: past, Price: 100,
                })
        st.Save()
        st.Unlock()
}

// TestRepriseManagerPrepaidCredit — reprise prépayée : propriété rendue au
// stock direct, portefeuille recrédité du prix GROS du stock vivant, UNE
// transaction « credit », trace d'activité avec décompte.
func TestRepriseManagerPrepaidCredit(t *testing.T) {
        st, ts := newTestServerWithStore(t)
        token, accID, _ := registerAccount(t, ts, "gerant-reprise", "")
        seedReprise(t, st, accID)

        status, out := doJSON(t, ts, "POST", "/api/vouchers/reprise", token,
                map[string]any{"ids": []string{"hu-alive1", "hu-alive2"}})
        if status != http.StatusOK {
                t.Fatalf("reprise : statut %d (%v)", status, out)
        }
        if n, _ := out["returned"].(float64); int(n) != 2 {
                t.Fatalf("returned attendu 2, obtenu %v", out["returned"])
        }
        if c, _ := out["credited"].(float64); int(c) != 200 {
                t.Fatalf("credited attendu 200 (prix gros, stock vivant), obtenu %v", out["credited"])
        }

        // Vérifications store : propriété vidée, crédit, transaction, audit.
        st.Lock()
        db := st.Data()
        for _, id := range []string{"hu-alive1", "hu-alive2"} {
                found := false
                for i := range db.HotspotUsers {
                        u := &db.HotspotUsers[i]
                        if u.ID == id {
                                found = true
                                if u.ResellerID != "" || u.ResellerName != "" || u.CreditSale {
                                        t.Fatalf("%s : propriété revendeur non vidée (%q/%q)", id, u.ResellerID, u.ResellerName)
                                }
                        }
                }
                if !found {
                        t.Fatalf("%s : voucher disparu du store", id)
                }
        }
        res := findResellerScoped(db, "res-rep", accID)
        if res == nil || res.Credit != 700 { // 500 initial + 200 recrédités
                t.Fatalf("crédit revendeur attendu 700, obtenu %+v", res)
        }
        if len(db.Transactions) != 1 {
                t.Fatalf("1 transaction attendue, obtenu %d", len(db.Transactions))
        }
        tx := db.Transactions[0]
        if tx.Type != "credit" || tx.Amount != 200 || tx.ResellerID != "res-rep" {
                t.Fatalf("transaction inattendue : %+v", tx)
        }
        if !strings.Contains(tx.Note, "Reprise par le gérant") {
                t.Fatalf("note de transaction sans mention reprise : %q", tx.Note)
        }
        traced := 0
        for _, act := range db.Activity {
                if act.AccountID == accID && strings.Contains(act.Message, "Reprise de stock") {
                        traced++
                        if !strings.Contains(act.Message, "ALIVE1") || !strings.Contains(act.Message, "Ada") {
                                t.Fatalf("trace d'activité incomplète : %q", act.Message)
                        }
                }
        }
        st.Unlock()
        if traced != 1 {
                t.Fatalf("exactement 1 trace « Reprise de stock » attendue, obtenu %d", traced)
        }
}

// TestRepriseManagerDepositNoCredit — dépôt-vente : reprise OK, AUCUN
// recrédit (rien n'avait été débité à la prise).
func TestRepriseManagerDepositNoCredit(t *testing.T) {
        st, ts := newTestServerWithStore(t)
        token, accID, _ := registerAccount(t, ts, "gerant-reprise-dep", "")
        seedReprise(t, st, accID)

        status, out := doJSON(t, ts, "POST", "/api/vouchers/reprise", token,
                map[string]any{"ids": []string{"hu-dep"}})
        if status != http.StatusOK {
                t.Fatalf("reprise dépôt-vente : statut %d (%v)", status, out)
        }
        if c, _ := out["credited"].(float64); c != 0 {
                t.Fatalf("dépôt-vente : aucun recrédit attendu, obtenu %v", out["credited"])
        }
        st.Lock()
        defer st.Unlock()
        if res := findResellerScoped(st.Data(), "res-dep", accID); res == nil || res.Credit != 0 {
                t.Fatalf("dépôt-vente : crédit doit rester 0, obtenu %+v", res)
        }
}

// TestRepriseManagerRejectsSoldAndUnknown — tout-le-lot : un ticket vendu
// (SoldAt) ou inconnu/hors scope bloque le lot, SANS aucune mutation (le
// stock vivant ne bouge pas, aucun crédit, aucune transaction).
func TestRepriseManagerRejectsSoldAndUnknown(t *testing.T) {
        st, ts := newTestServerWithStore(t)
        token, accID, _ := registerAccount(t, ts, "gerant-reprise-rej", "")
        seedReprise(t, st, accID)

        for _, ids := range [][]string{
                {"hu-alive1", "hu-sold"},  // vendu — créance née
                {"hu-alive1", "hu-ghost"}, // inconnu
                {"hu-direct"},             // stock direct du gérant — hors reprise
        } {
                status, out := doJSON(t, ts, "POST", "/api/vouchers/reprise", token,
                        map[string]any{"ids": ids})
                if status != http.StatusConflict {
                        t.Fatalf("reprise %v : 409 attendu, obtenu %d (%v)", ids, status, out)
                }
        }
        st.Lock()
        defer st.Unlock()
        db := st.Data()
        if res := findResellerScoped(db, "res-rep", accID); res == nil || res.Credit != 500 {
                t.Fatalf("refus doit être sans effet sur le crédit (500), obtenu %+v", res)
        }
        for _, id := range []string{"hu-alive1", "hu-alive2", "hu-sold"} {
                for i := range db.HotspotUsers {
                        if db.HotspotUsers[i].ID == id && db.HotspotUsers[i].ResellerID == "" {
                                t.Fatalf("%s : propriété modifiée malgré le refus", id)
                        }
                }
        }
        if len(db.Transactions) != 0 {
                t.Fatalf("aucune transaction attendue après refus, obtenu %d", len(db.Transactions))
        }
}

// TestRepriseManagerExpiredNoCreditClosedLoop — un ticket alloué expiré est
// repris SANS recrédit (il a péri chez le revendeur) : la boucle se referme,
// le gérant peut alors le supprimer (le verrou W1 ne crée pas de zombie).
func TestRepriseManagerExpiredNoCreditClosedLoop(t *testing.T) {
        st, ts := newTestServerWithStore(t)
        token, accID, _ := registerAccount(t, ts, "gerant-reprise-exp", "")
        seedReprise(t, st, accID)

        status, out := doJSON(t, ts, "POST", "/api/vouchers/reprise", token,
                map[string]any{"ids": []string{"hu-expired"}})
        if status != http.StatusOK {
                t.Fatalf("reprise expiré : statut %d (%v)", status, out)
        }
        if c, _ := out["credited"].(float64); c != 0 {
                t.Fatalf("expiré : aucun recrédit attendu, obtenu %v", out["credited"])
        }
        // La boucle se referme : suppression unitaire désormais possible (200).
        status, out = doJSON(t, ts, "DELETE", "/api/vouchers/hu-expired", token, nil)
        if status != http.StatusOK {
                t.Fatalf("suppression post-reprise : statut %d (%v)", status, out)
        }
}

// TestW1DeleteGuards — un ticket attribué n'est destructible par AUCUNE
// porte console : unitaire (403), bulk (409), lot entier (409) ; après
// reprise, les mêmes portes s'ouvrent.
func TestW1DeleteGuards(t *testing.T) {
        st, ts := newTestServerWithStore(t)
        token, accID, _ := registerAccount(t, ts, "gerant-w1", "")
        seedReprise(t, st, accID)

        // 1) Suppression unitaire → 403 structuré (code verrou N°22).
        status, out := doJSON(t, ts, "DELETE", "/api/vouchers/hu-alive1", token, nil)
        if status != http.StatusForbidden {
                t.Fatalf("delete unitaire alloué : 403 attendu, obtenu %d", status)
        }
        if code, _ := out["code"].(string); code != "reseller_voucher_locked" {
                t.Fatalf("code d'erreur attendu reseller_voucher_locked, obtenu %v", out["code"])
        }

        // 2) Suppression groupée → 409 tout-le-lot.
        status, out = doJSON(t, ts, "POST", "/api/users/bulk", token,
                map[string]any{"ids": []string{"hu-alive1", "hu-direct"}, "action": "delete"})
        if status != http.StatusConflict {
                t.Fatalf("bulk delete alloué : 409 attendu, obtenu %d (%v)", status, out)
        }

        // 3) Suppression du lot entier (les alloués partagent batchId "batch-w1").
        st.Lock()
        for i := range st.Data().HotspotUsers {
                if st.Data().HotspotUsers[i].ID == "hu-alive1" {
                        st.Data().HotspotUsers[i].BatchID = "batch-w1"
                }
        }
        st.Save()
        st.Unlock()
        status, _ = doJSON(t, ts, "POST", "/api/vouchers/batch/batch-w1/delete", token, nil)
        if status != http.StatusConflict {
                t.Fatalf("delete lot avec alloué : 409 attendu, obtenu %d", status)
        }

        // 4) Nettoyage des expirés : le ticket alloué expiré est épargné, le
        // direct expiré est purgé.
        st.Lock()
        for i := range st.Data().HotspotUsers {
                if st.Data().HotspotUsers[i].ID == "hu-direct" {
                        st.Data().HotspotUsers[i].ExpiresAt = time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
                        st.Data().HotspotUsers[i].Status = "expired" // statut STOCKÉ (critère du nettoyage)
                }
        }
        st.Save()
        st.Unlock()
        status, out = doJSON(t, ts, "POST", "/api/users/cleanup", token, map[string]any{"mode": "expired"})
        if status != http.StatusOK {
                t.Fatalf("cleanup : statut %d (%v)", status, out)
        }
        st.Lock()
        directGone, expiredKept := true, false
        for i := range st.Data().HotspotUsers {
                switch st.Data().HotspotUsers[i].ID {
                case "hu-direct":
                        directGone = false
                case "hu-expired":
                        expiredKept = st.Data().HotspotUsers[i].ResellerID != ""
                }
        }
        st.Unlock()
        if !directGone {
                t.Fatal("cleanup : le direct expiré devait être purgé")
        }
        if !expiredKept {
                t.Fatal("cleanup : le ticket alloué expiré ne doit pas être purgé en silence")
        }

        // 5) La reprise ferme la boucle : après reprise, delete unitaire et lot OK.
        status, out = doJSON(t, ts, "POST", "/api/vouchers/reprise", token,
                map[string]any{"ids": []string{"hu-alive1"}})
        if status != http.StatusOK {
                t.Fatalf("reprise préalable : statut %d (%v)", status, out)
        }
        status, _ = doJSON(t, ts, "DELETE", "/api/vouchers/hu-alive1", token, nil)
        if status != http.StatusOK {
                t.Fatalf("delete unitaire post-reprise : 200 attendu, obtenu %d", status)
        }
        status, _ = doJSON(t, ts, "POST", "/api/vouchers/batch/batch-w1/delete", token, nil)
        if status != http.StatusOK {
                t.Fatalf("delete lot post-reprise : 200 attendu, obtenu %d", status)
        }
}

// TestHolderFilter — W3/W4 : le filtre `holder` des listes sépare le stock
// direct du stock alloué (resellerId reste lisible côté gérant).
func TestHolderFilter(t *testing.T) {
        st, ts := newTestServerWithStore(t)
        token, accID, _ := registerAccount(t, ts, "gerant-holder", "")
        seedReprise(t, st, accID)

        status, out := doJSON(t, ts, "GET", "/api/vouchers?holder=reseller", token, nil)
        if status != http.StatusOK {
                t.Fatalf("holder=reseller : statut %d", status)
        }
        if total, _ := out["total"].(float64); int(total) != 5 { // alive1+alive2+dep+sold+expired
                t.Fatalf("holder=reseller : 5 attendus, obtenu %v", out["total"])
        }

        status, out = doJSON(t, ts, "GET", "/api/vouchers?holder=direct", token, nil)
        if status != http.StatusOK {
                t.Fatalf("holder=direct : statut %d", status)
        }
        rows, _ := out["data"].([]any)
        if int(out["total"].(float64)) != 2 || len(rows) != 2 { // hu-direct + hu-batch2
                t.Fatalf("holder=direct : 2 attendus, obtenu %v", out["total"])
        }
        ids := map[string]bool{}
        for _, raw := range rows {
                row, _ := raw.(map[string]any)
                id, _ := row["id"].(string)
                ids[id] = true
        }
        if !ids["hu-direct"] || !ids["hu-batch2"] || len(ids) != 2 {
                t.Fatalf("holder=direct : hu-direct + hu-batch2 attendus, obtenu %v", ids)
        }
}
