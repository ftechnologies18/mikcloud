// Tests de la purge FUSIONNÉE (Paramètres plateforme) :
//   - GET  /api/admin/purge/stats   : compteurs globaux (tous comptes) ;
//   - GET  /api/admin/purge/accounts : compteurs par élément pour chaque compte ;
//   - POST /api/admin/purge         : exécution portée GLOBALE (accountId vide)
//     ou CIBLÉE (accountId renseigné — l'ancien /purge/account est fusionné).
//
// Garanties vérifiées : isolation stricte en portée ciblée (les données des
// AUTRES comptes restent intactes), préservation des routeurs réels et des
// comptes, cascades conformes (sessions fermées avec les utilisateurs/tickets,
// vouchers des lots, transactions des revendeurs, entités des routeurs
// simulés), ligne de traçabilité écrite sur le compte ciblé, erreurs 400/404
// défensives, sélection explicite exigée (scopes vide → 400).
//
// AUCUNE connexion réseau réelle (pas de PostgreSQL) : store JSON éphémère,
// token plateforme forgé avec le secret de test.
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"mikcloud/hotspot-api/internal/auth"
	"mikcloud/hotspot-api/internal/model"
	"mikcloud/hotspot-api/internal/store"
)

// countBy — comptage génrique sur le champ compte d'une collection du store.
func countBy[T any](items []T, accID string, acc func(T) string) int {
	n := 0
	for _, it := range items {
		if acc(it) == accID {
			n++
		}
	}
	return n
}

// purgeSeed — deux comptes clients avec des données croisées :
//   - accA : routeur simulé + routeur réel (agent), 2 vouchers (un sur le
//     simulé → cascade, un sur le réel), 1 utilisateur régulier, profil, lot,
//     revendeur + transaction, vente, session, journaux, gabarit ;
//   - accB (témoin d'isolation) : voucher, session, lot, revendeur +
//     transaction, vente.
func purgeSeed(t *testing.T, st *store.Store) (accA, accB string) {
	t.Helper()
	st.Lock()
	db := st.Data()
	accA, accB = "acc-pa", "acc-pb"
	db.Accounts = append(db.Accounts,
		model.Account{ID: accA, Name: "Client A", Status: "active", CreatedAt: model.NowISO()},
		model.Account{ID: accB, Name: "Client B", Status: "active", CreatedAt: model.NowISO()},
	)
	rtSim, rtReal := "rt-pa-sim", "rt-pa-real"
	db.Routers = append(db.Routers,
		model.Router{ID: rtSim, AccountID: accA, Name: "Sim A", Mode: "simulated"},
		model.Router{ID: rtReal, AccountID: accA, Name: "Réel A", Mode: "agent"},
	)
	vSim, vReal, vB := "hu-pa-vsim", "hu-pa-vreal", "hu-pb-v"
	db.HotspotUsers = append(db.HotspotUsers,
		model.HotspotUser{ID: vSim, AccountID: accA, Kind: "voucher", Username: "VS-1", RouterID: rtSim, Status: "active"},
		model.HotspotUser{ID: vReal, AccountID: accA, Kind: "voucher", Username: "VR-1", RouterID: rtReal, Status: "active"},
		model.HotspotUser{ID: "hu-pa-u", AccountID: accA, Kind: "regular", Username: "client-a", RouterID: rtReal, Status: "active"},
		model.HotspotUser{ID: vB, AccountID: accB, Kind: "voucher", Username: "VB-1", Status: "active"},
	)
	db.Sessions = append(db.Sessions,
		model.Session{ID: "ses-pa-1", AccountID: accA, UserID: vReal, RouterID: rtReal},
		model.Session{ID: "ses-pb-1", AccountID: accB, UserID: vB},
	)
	db.Profiles = append(db.Profiles, model.Profile{ID: "prof-pa", AccountID: accA, Name: "Forfait A"})
	db.Batches = append(db.Batches,
		model.Batch{ID: "bat-pa", AccountID: accA, RouterID: rtReal},
		model.Batch{ID: "bat-pb", AccountID: accB, RouterID: rtReal},
	)
	db.Resellers = append(db.Resellers,
		model.Reseller{ID: "res-pa", AccountID: accA, Name: "Revendeur A"},
		model.Reseller{ID: "res-pb", AccountID: accB, Name: "Revendeur B"},
	)
	db.Transactions = append(db.Transactions,
		model.Transaction{ID: "tx-pa", AccountID: accA, ResellerID: "res-pa"},
		model.Transaction{ID: "tx-pb", AccountID: accB, ResellerID: "res-pb"},
	)
	db.Sales = append(db.Sales,
		model.Sale{ID: "sale-pa", AccountID: accA, Amount: 500},
		model.Sale{ID: "sale-pb", AccountID: accB, Amount: 300},
	)
	db.UserLogs = append(db.UserLogs, model.UserLog{ID: "ul-pa", AccountID: accA})
	db.Activity = append(db.Activity, model.Activity{ID: "act-pa", AccountID: accA})
	db.Templates = append(db.Templates, model.VoucherTemplate{ID: "tpl-pa", AccountID: accA})
	st.Save()
	st.Unlock()
	return accA, accB
}

// newPurgeTestServer — serveur de test avec un admin plateforme forgé
// (utilisateur réel du store, même secret JWT que le mux de test).
func newPurgeTestServer(t *testing.T) (*store.Store, *httptest.Server, string) {
	t.Helper()
	st, ts := newTestServerWithStore(t)
	seedUser(t, st, "usr-plat", "", "usr-plat", model.RolePlatformAdmin)
	token := auth.Sign(testJWTSecret, auth.NewClaims("usr-plat", "Plateforme", model.RolePlatformAdmin, "", 0))
	return st, ts, token
}

// purgeStatsFor — GET /api/admin/purge/accounts et retourne la ligne du
// compte demandé (réponse = tableau JSON).
func purgeStatsFor(t *testing.T, ts *httptest.Server, token, accID string) map[string]any {
	t.Helper()
	req, err := http.NewRequest("GET", ts.URL+"/api/admin/purge/accounts", nil)
	if err != nil {
		t.Fatalf("requête stats impossible : %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("GET /api/admin/purge/accounts : %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/admin/purge/accounts : statut %d", resp.StatusCode)
	}
	var rows []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
		t.Fatalf("décodage stats impossible : %v", err)
	}
	for _, row := range rows {
		if row["id"] == accID {
			return row
		}
	}
	t.Fatalf("compte %s absent de la liste de purge", accID)
	return nil
}

// purgeCount — lit un compteur entier dans les stats d'une ligne.
func purgeCount(t *testing.T, row map[string]any, field string) int {
	t.Helper()
	stats, ok := row["stats"].(map[string]any)
	if !ok {
		t.Fatalf("stats absentes de la ligne de purge : %v", row)
	}
	v, _ := stats[field].(float64)
	return int(v)
}

// purgeCountIn — lit un compteur entier dans la réponse d'une purge.
func purgeCountIn(t *testing.T, out map[string]any, field string) int {
	t.Helper()
	purged, ok := out["purged"].(map[string]any)
	if !ok {
		t.Fatalf("réponse de purge sans compteurs : %v", out)
	}
	v, _ := purged[field].(float64)
	return int(v)
}

// TestPurgeTargetedAccountIsolation — portée CIBLÉE (accountId) : purge de
// vouchers + utilisateurs + ventes + journaux + sessions sur A uniquement ;
// B témoin strictement intact.
func TestPurgeTargetedAccountIsolation(t *testing.T) {
	st, ts, token := newPurgeTestServer(t)
	accA, accB := purgeSeed(t, st)

	// 1. Compteurs par élément avant purge (les entités du routeur simulé
	//    de A sont exclues de leur catégorie — cascade).
	row := purgeStatsFor(t, ts, token, accA)
	if got := purgeCount(t, row, "vouchers"); got != 1 {
		t.Fatalf("A : vouchers attendus 1 (hors simulé), obtenu %d", got)
	}
	if got := purgeCount(t, row, "hotspotUsers"); got != 1 {
		t.Fatalf("A : utilisateurs attendus 1, obtenu %d", got)
	}
	if got := purgeCount(t, row, "simulatedRouters"); got != 1 {
		t.Fatalf("A : routeurs simulés attendus 1, obtenu %d", got)
	}
	if got := purgeCount(t, row, "sales"); got != 1 {
		t.Fatalf("A : ventes attendues 1, obtenu %d", got)
	}
	if got := purgeCount(t, row, "logs"); got != 2 { // 1 user log + 1 activité
		t.Fatalf("A : journaux attendus 2, obtenu %d", got)
	}

	// 2. Purge ciblée sur A (endpoint FUSIONNÉ : accountId dans le corps) :
	//    vouchers + utilisateurs + ventes + journaux + sessions (les
	//    routeurs, profil, lot, revendeur, transaction, gabarit de A et
	//    TOUT B restent).
	status, out := doJSON(t, ts, "POST", "/api/admin/purge", token, map[string]any{
		"accountId": accA,
		"scopes":    []string{"vouchers", "hotspot_users", "sales", "sessions", "logs"},
	})
	if status != http.StatusOK {
		t.Fatalf("purge ciblée : statut %d (%v)", status, out)
	}
	if got := purgeCountIn(t, out, "vouchers"); got != 2 { // vSim + vReal
		t.Fatalf("purge : vouchers attendus 2, obtenu %d", got)
	}
	if got := purgeCountIn(t, out, "hotspotUsers"); got != 1 {
		t.Fatalf("purge : utilisateurs attendus 1, obtenu %d", got)
	}
	if got := purgeCountIn(t, out, "sessions"); got != 1 { // session de vReal
		t.Fatalf("purge : sessions attendues 1, obtenu %d", got)
	}
	if got := purgeCountIn(t, out, "sales"); got != 1 {
		t.Fatalf("purge : ventes attendues 1, obtenu %d", got)
	}

	// 3. État du store : A purgé sur les catégories, le reste intact ;
	//    B strictement intact ; routeurs de A préservés (réel ET simulé).
	st.Lock()
	db := st.Data()
	if n := countBy(db.HotspotUsers, accA, func(u model.HotspotUser) string { return u.AccountID }); n != 0 {
		t.Fatalf("A : %d hotspot user(s) restant(s), attendu 0", n)
	}
	if n := countBy(db.Sales, accA, func(s model.Sale) string { return s.AccountID }); n != 0 {
		t.Fatalf("A : %d vente(s) restante(s), attendu 0", n)
	}
	if n := countBy(db.Profiles, accA, func(p model.Profile) string { return p.AccountID }); n != 1 {
		t.Fatalf("A : profil doit être conservé, obtenu %d", n)
	}
	if n := countBy(db.Batches, accA, func(b model.Batch) string { return b.AccountID }); n != 1 {
		t.Fatalf("A : lot doit être conservé (scope non sélectionné), obtenu %d", n)
	}
	if n := countBy(db.Resellers, accA, func(res model.Reseller) string { return res.AccountID }); n != 1 {
		t.Fatalf("A : revendeur doit être conservé, obtenu %d", n)
	}
	if n := countBy(db.Transactions, accA, func(tx model.Transaction) string { return tx.AccountID }); n != 1 {
		t.Fatalf("A : transaction doit être conservée (revendeur non purgé), obtenu %d", n)
	}
	if n := countBy(db.Templates, accA, func(tp model.VoucherTemplate) string { return tp.AccountID }); n != 1 {
		t.Fatalf("A : gabarit doit être conservé, obtenu %d", n)
	}
	if n := len(db.Routers); n != 2 {
		t.Fatalf("routeurs de A : 2 attendus (réel + simulé préservés), obtenu %d", n)
	}
	// B témoin : aucune donnée touchée.
	if n := countBy(db.HotspotUsers, accB, func(u model.HotspotUser) string { return u.AccountID }); n != 1 {
		t.Fatalf("B : voucher témoin doit rester, obtenu %d", n)
	}
	if n := countBy(db.Sessions, accB, func(s model.Session) string { return s.AccountID }); n != 1 {
		t.Fatalf("B : session témoin doit rester, obtenu %d", n)
	}
	if n := countBy(db.Sales, accB, func(s model.Sale) string { return s.AccountID }); n != 1 {
		t.Fatalf("B : vente témoin doit rester, obtenu %d", n)
	}
	if n := countBy(db.Resellers, accB, func(res model.Reseller) string { return res.AccountID }); n != 1 {
		t.Fatalf("B : revendeur témoin doit rester, obtenu %d", n)
	}
	if n := countBy(db.Transactions, accB, func(tx model.Transaction) string { return tx.AccountID }); n != 1 {
		t.Fatalf("B : transaction témoin doit rester, obtenu %d", n)
	}
	if n := countBy(db.Batches, accB, func(b model.Batch) string { return b.AccountID }); n != 1 {
		t.Fatalf("B : lot témoin doit rester, obtenu %d", n)
	}
	// Traçabilité : la ligne d'activité de A a été remplacée par la purge.
	if n := countBy(db.Activity, accA, func(act model.Activity) string { return act.AccountID }); n != 1 {
		t.Fatalf("A : 1 ligne de traçabilité attendue après purge, obtenu %d", n)
	}
	st.Unlock()

	// 4. Compteurs par élément après purge (la ligne de traçabilité compte
	//    pour 1 journal sur A ; B inchangé).
	row = purgeStatsFor(t, ts, token, accA)
	if got := purgeCount(t, row, "vouchers"); got != 0 {
		t.Fatalf("A après purge : vouchers attendus 0, obtenu %d", got)
	}
	if got := purgeCount(t, row, "sales"); got != 0 {
		t.Fatalf("A après purge : ventes attendues 0, obtenu %d", got)
	}
	if got := purgeCount(t, row, "logs"); got != 1 {
		t.Fatalf("A après purge : journaux attendus 1 (traçabilité), obtenu %d", got)
	}
	row = purgeStatsFor(t, ts, token, accB)
	if got := purgeCount(t, row, "vouchers"); got != 1 {
		t.Fatalf("B après purge : vouchers témoins attendus 1, obtenu %d", got)
	}
	if got := purgeCount(t, row, "sales"); got != 1 {
		t.Fatalf("B après purge : ventes témoins attendues 1, obtenu %d", got)
	}
}

// TestPurgeTargetedAccountCascade — portée ciblée, scope « all » : purge
// totale du compte (cascade du routeur simulé : voucher/lot/vente attachés ;
// lots restants + leurs vouchers ; revendeurs + transactions ; profil,
// gabarit, journaux) — B et le routeur réel de A restent intacts.
func TestPurgeTargetedAccountCascade(t *testing.T) {
	st, ts, token := newPurgeTestServer(t)
	accA, accB := purgeSeed(t, st)

	status, out := doJSON(t, ts, "POST", "/api/admin/purge", token, map[string]any{
		"accountId": accA,
		"scopes":    []string{"all"},
	})
	if status != http.StatusOK {
		t.Fatalf("purge ciblée « all » : statut %d (%v)", status, out)
	}
	if got := purgeCountIn(t, out, "routers"); got != 1 { // uniquement le simulé
		t.Fatalf("purge all : routeurs simulés attendus 1, obtenu %d", got)
	}
	if got := purgeCountIn(t, out, "batches"); got != 1 { // bat-pa uniquement (bat-pb est de B)
		t.Fatalf("purge all : lots de A attendus 1, obtenu %d", got)
	}

	st.Lock()
	db := st.Data()
	// Cascade simulée : le routeur simulé part avec son voucher.
	if n := len(db.Routers); n != 1 {
		t.Fatalf("A : seul le routeur réel doit rester, obtenu %d routeur(s)", n)
	}
	if db.Routers[0].ID != "rt-pa-real" {
		t.Fatalf("A : le routeur conservé doit être le réel, obtenu %s", db.Routers[0].ID)
	}
	if n := countBy(db.HotspotUsers, accA, func(u model.HotspotUser) string { return u.AccountID }); n != 0 {
		t.Fatalf("A : tous les vouchers/utilisateurs doivent partir (all), obtenu %d", n)
	}
	if n := countBy(db.Batches, accA, func(b model.Batch) string { return b.AccountID }); n != 0 {
		t.Fatalf("A : le lot de A doit partir (all), obtenu %d", n)
	}
	if n := countBy(db.Sessions, accA, func(s model.Session) string { return s.AccountID }); n != 0 {
		t.Fatalf("A : la session de A doit partir (all), obtenu %d", n)
	}
	// B témoin : strictement intact.
	if n := countBy(db.HotspotUsers, accB, func(u model.HotspotUser) string { return u.AccountID }); n != 1 {
		t.Fatalf("B : voucher témoin doit rester, obtenu %d", n)
	}
	if n := countBy(db.Batches, accB, func(b model.Batch) string { return b.AccountID }); n != 1 {
		t.Fatalf("B : lot témoin doit rester, obtenu %d", n)
	}
	if n := countBy(db.Transactions, accB, func(tx model.Transaction) string { return tx.AccountID }); n != 1 {
		t.Fatalf("B : transaction témoin doit rester, obtenu %d", n)
	}
	// Comptes jamais purgés (le compte principal n'existe en mode JSON que
	// si des utilisateurs sont inscrits : A et B doivent rester).
	hasA, hasB := false, false
	for _, acc := range db.Accounts {
		switch acc.ID {
		case accA:
			hasA = true
		case accB:
			hasB = true
		}
	}
	if !hasA || !hasB {
		t.Fatalf("comptes jamais purgés : A présent=%v, B présent=%v", hasA, hasB)
	}
	st.Unlock()
}

// TestPurgeGlobalUnified — portée GLOBALE (accountId ABSENT) : le scope
// « vouchers » supprime les tickets de TOUS les comptes (routeur réel ET
// compte B), sans toucher aux lots, utilisateurs réguliers, ventes ni
// sessions hors tickets. C'est la sémantique fusionnée : la grille de
// catégories est identique dans les deux portées.
func TestPurgeGlobalUnified(t *testing.T) {
	st, ts, token := newPurgeTestServer(t)
	accA, accB := purgeSeed(t, st)

	// Compteurs globaux : 3 vouchers (vSim est en cascade → exclu), 1 compte
	// client, 2 ventes, 2 sessions.
	status, stats := doJSON(t, ts, "GET", "/api/admin/purge/stats", token, nil)
	if status != http.StatusOK {
		t.Fatalf("GET purge/stats : statut %d (%v)", status, stats)
	}
	if got := int(stats["vouchers"].(float64)); got != 2 {
		t.Fatalf("stats globales : vouchers attendus 2, obtenu %d", got)
	}
	if got := int(stats["hotspotUsers"].(float64)); got != 1 {
		t.Fatalf("stats globales : utilisateurs attendus 1, obtenu %d", got)
	}

	// Purge GLOBALE des vouchers uniquement (pas d'accountId dans le corps).
	// NB : le ticket vSim attaché au routeur SIMULÉ part aussi — la cascade
	// simulée n'est déclenchée que par le scope « simulated_routers », tandis
	// que le scope « vouchers » s'applique à TOUS les tickets de la portée.
	status, out := doJSON(t, ts, "POST", "/api/admin/purge", token, map[string]any{
		"scopes": []string{"vouchers"},
	})
	if status != http.StatusOK {
		t.Fatalf("purge globale vouchers : statut %d (%v)", status, out)
	}
	if got := purgeCountIn(t, out, "vouchers"); got != 3 { // vSim + vReal (A) + vB (B)
		t.Fatalf("purge globale : vouchers attendus 3, obtenu %d", got)
	}
	if got := purgeCountIn(t, out, "sessions"); got != 2 { // sessions de vReal et vB
		t.Fatalf("purge globale : sessions attendues 2, obtenu %d", got)
	}

	st.Lock()
	db := st.Data()
	// Aucun voucher restant (A et B)…
	if n := len(db.HotspotUsers); n != 1 {
		t.Fatalf("vouchers purgés partout : 1 utilisateur régulier restant attendu, obtenu %d", n)
	}
	if db.HotspotUsers[0].ID != "hu-pa-u" {
		t.Fatalf("l'utilisateur régulier doit rester, obtenu %s", db.HotspotUsers[0].ID)
	}
	// …mais les LOTS, routeurs, ventes, revendeurs restent.
	if n := len(db.Batches); n != 2 {
		t.Fatalf("lots conservés (scope vouchers seul) : attendu 2, obtenu %d", n)
	}
	if n := len(db.Routers); n != 2 {
		t.Fatalf("routeurs conservés : attendu 2, obtenu %d", n)
	}
	if n := len(db.Sales); n != 2 {
		t.Fatalf("ventes conservées : attendu 2, obtenu %d", n)
	}
	// Session témoin de B (voucher VB purgé → sa session part) et la session
	// de A n'avait pas de ticket lié restant : 0 session restante.
	if n := len(db.Sessions); n != 0 {
		t.Fatalf("sessions des tickets purgés : 0 restante attendue, obtenu %d", n)
	}
	_ = accA
	_ = accB
	st.Unlock()
}

// TestPurgeGlobalAll — portée GLOBALE, scope « all » : purge totale des
// données métier (routeur simulé en cascade, lots, ventes, revendeurs +
// transactions, journaux) ; routeur réel, profils, gabarits et comptes
// préservés.
func TestPurgeGlobalAll(t *testing.T) {
	st, ts, token := newPurgeTestServer(t)
	accA, accB := purgeSeed(t, st)

	status, out := doJSON(t, ts, "POST", "/api/admin/purge", token, map[string]any{
		"scopes": []string{"all"},
	})
	if status != http.StatusOK {
		t.Fatalf("purge globale « all » : statut %d (%v)", status, out)
	}
	if got := purgeCountIn(t, out, "routers"); got != 1 {
		t.Fatalf("purge all globale : routeur simulé attendu 1, obtenu %d", got)
	}
	if got := purgeCountIn(t, out, "batches"); got != 2 {
		t.Fatalf("purge all globale : lots attendus 2, obtenu %d", got)
	}
	if got := purgeCountIn(t, out, "transactions"); got != 2 {
		t.Fatalf("purge all globale : transactions attendues 2 (cascade revendeurs), obtenu %d", got)
	}

	st.Lock()
	db := st.Data()
	// Routeur réel préservé, données métier vides sur les deux comptes.
	if n := len(db.Routers); n != 1 || db.Routers[0].Mode != "agent" {
		t.Fatalf("purge all globale : seul le routeur réel doit rester, obtenu %d", n)
	}
	if n := countBy(db.HotspotUsers, accA, func(u model.HotspotUser) string { return u.AccountID }) +
		countBy(db.HotspotUsers, accB, func(u model.HotspotUser) string { return u.AccountID }); n != 0 {
		t.Fatalf("purge all globale : tous les utilisateurs/tickets doivent partir, obtenu %d", n)
	}
	if n := countBy(db.Sales, accA, func(s model.Sale) string { return s.AccountID }) +
		countBy(db.Sales, accB, func(s model.Sale) string { return s.AccountID }); n != 0 {
		t.Fatalf("purge all globale : toutes les ventes doivent partir, obtenu %d", n)
	}
	// Comptes préservés (jamais purgés) ; « all » = TOUTES les catégories,
	// profils et gabarits compris — seuls les comptes et l'équipe restent.
	hasA, hasB := false, false
	for _, acc := range db.Accounts {
		switch acc.ID {
		case accA:
			hasA = true
		case accB:
			hasB = true
		}
	}
	if !hasA || !hasB {
		t.Fatalf("comptes jamais purgés : A présent=%v, B présent=%v", hasA, hasB)
	}
	if n := countBy(db.Profiles, accA, func(p model.Profile) string { return p.AccountID }); n != 0 {
		t.Fatalf("purge all : le profil fait partie des catégories (« all »), obtenu %d", n)
	}
	st.Unlock()
}

// TestPurgeGuards — validations défensives : scope inconnu, aucune
// catégorie exploitable, scopes absents, compte inconnu en portée ciblée.
func TestPurgeGuards(t *testing.T) {
	st, ts, token := newPurgeTestServer(t)
	accA, _ := purgeSeed(t, st)

	// Scope inconnu (portée globale).
	status, _ := doJSON(t, ts, "POST", "/api/admin/purge", token, map[string]any{
		"scopes": []string{"categorie_fantome"},
	})
	if status != http.StatusBadRequest {
		t.Fatalf("catégorie inconnue : statut %d attendu 400", status)
	}
	// Scopes blancs uniquement → sélection inexploitable.
	status, _ = doJSON(t, ts, "POST", "/api/admin/purge", token, map[string]any{
		"scopes": []string{"  "},
	})
	if status != http.StatusBadRequest {
		t.Fatalf("aucune catégorie exploitable : statut %d attendu 400", status)
	}
	// Scopes ABSENTS → 400 (sélection explicite exigée, les deux portées).
	status, _ = doJSON(t, ts, "POST", "/api/admin/purge", token, map[string]any{})
	if status != http.StatusBadRequest {
		t.Fatalf("scopes absents : statut %d attendu 400", status)
	}
	// Portée ciblée : compte inconnu → 404.
	status, _ = doJSON(t, ts, "POST", "/api/admin/purge", token, map[string]any{
		"accountId": "acc-inconnu", "scopes": []string{"vouchers"},
	})
	if status != http.StatusNotFound {
		t.Fatalf("compte inconnu : statut %d attendu 404", status)
	}
	// Portée ciblée : catégorie inconnue → 400.
	status, _ = doJSON(t, ts, "POST", "/api/admin/purge", token, map[string]any{
		"accountId": accA, "scopes": []string{"categorie_fantome"},
	})
	if status != http.StatusBadRequest {
		t.Fatalf("catégorie inconnue (ciblée) : statut %d attendu 400", status)
	}
}
