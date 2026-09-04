// Tests du blocage anti-résurrection (audit purge/résurgence) :
//   - la purge pose un TOMBSTONE par username purgé (réponse purged.tombstones) ;
//   - applyReadState refuse de ré-importer un username tombstoné (ni user,
//     ni session, ni badge « inconnu ») — c'est le correctif de la résurgence ;
//   - le tombstone EXPIRE (TTL 30 j) : l'import redevient possible ;
//   - la création VOLONTAIRE du même username lève le tombstone ;
//   - import auto DÉSACTIVÉ : les inconnus ne sont pas créés mais comptés
//     (Router.UnknownOnRouter) pour adoption manuelle ;
//   - la file de commandes qui recréerait les entités purgées est annulée ;
//   - purge TOTALE (alsoRouter) : double confirmation exigée + commandes
//     user_remove enfilées pour les routeurs AGENT.
//
// AUCUNE connexion réseau réelle : store JSON éphémère + API vide
// (applyReadState n'utilise que l'état passé en argument).
package api

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"mikcloud/hotspot-api/internal/model"
)

// readStateUsers — applique un rapport read_state users "a|p|d;…" au routeur.
func readStateUsers(t *testing.T, db *model.DB, router *model.Router, users, sessions string) {
	t.Helper()
	vals := url.Values{}
	if users != "" {
		vals.Set("users", users)
	}
	if sessions != "" {
		vals.Set("sessions", sessions)
	}
	(&API{}).applyReadState(db, router, vals)
}

func routerByID(db *model.DB, id string) *model.Router {
	for i := range db.Routers {
		if db.Routers[i].ID == id {
			return &db.Routers[i]
		}
	}
	return nil
}

// TestPurgeWritesTombstones — purge ciblée (vouchers + comptes client) :
// chaque username purgé du compte reçoit un tombstone (TTL 30 jours) et la
// réponse expose le compteur purged.tombstones.
func TestPurgeWritesTombstones(t *testing.T) {
	st, ts, token := newPurgeTestServer(t)
	accA, _ := purgeSeed(t, st)

	status, out := doJSON(t, ts, "POST", "/api/admin/purge", token, map[string]any{
		"accountId": accA,
		"scopes":    []string{"vouchers", "hotspot_users"},
	})
	if status != http.StatusOK {
		t.Fatalf("purge ciblée : statut %d", status)
	}
	// VS-1 (voucher simulé), VR-1 (voucher agent), client-a (régulier) :
	// 3 usernames purgés → 3 tombstones.
	if got := purgeCountIn(t, out, "tombstones"); got != 3 {
		t.Fatalf("tombstones attendus 3, obtenu %d", got)
	}

	st.Lock()
	db := st.Data()
	if got := len(db.PurgeTombstones); got != 3 {
		t.Fatalf("tombstones en base : 3 attendus, obtenu %d", got)
	}
	now := time.Now().UTC()
	for _, tb := range db.PurgeTombstones {
		if tb.AccountID != accA {
			t.Fatalf("tombstone hors compte ciblé : %v", tb)
		}
		if tb.Username != "vs-1" && tb.Username != "vr-1" && tb.Username != "client-a" {
			t.Fatalf("username tombstoné inattendu : %q", tb.Username)
		}
		exp, err := time.Parse(time.RFC3339, tb.ExpiresAt)
		if err != nil {
			t.Fatalf("expires_at illisible : %v", err)
		}
		if d := exp.Sub(now); d <= 0 || d > 31*24*time.Hour {
			t.Fatalf("TTL du tombstone hors fenêtre 30 j : %v", d)
		}
	}
	st.Unlock()
}

// TestPurgeBlocksReadStateResurrection — LE test du bug signalé : après purge,
// le read_state agent qui renvoie les mêmes utilisateurs ne recrée RIEN
// (avant le correctif, tout revenait : résurgence + vouchers en « regular »
// fantômes). Les tombstonés ne comptent pas non plus comme « inconnus ».
func TestPurgeBlocksReadStateResurrection(t *testing.T) {
	st, ts, token := newPurgeTestServer(t)
	accA, _ := purgeSeed(t, st)

	status, _ := doJSON(t, ts, "POST", "/api/admin/purge", token, map[string]any{
		"accountId": accA,
		"scopes":    []string{"vouchers", "hotspot_users"},
	})
	if status != http.StatusOK {
		t.Fatalf("purge ciblée : statut %d", status)
	}

	st.Lock()
	db := st.Data()
	rt := routerByID(db, "rt-pa-real")
	if rt == nil {
		t.Fatal("routeur agent introuvable")
	}
	// Le routeur réel garde ses comptes (la purge ne touche jamais les
	// routeurs) : il les renvoie au read_state suivant — CHIFFRE IDENTIQUE.
	readStateUsers(t, db, rt,
		"VS-1|default|false;VR-1|default|false;client-a|default|false",
		"VR-1|10.0.0.5|1h2m3s|1000|2000;client-a|10.0.0.6|5m|10|20")

	if n := countBy(db.HotspotUsers, accA, func(u model.HotspotUser) string { return u.AccountID }); n != 0 {
		t.Fatalf("RÉSURGENCE : %d utilisateur(s) ré-importé(s) après purge (attendu 0)", n)
	}
	if n := countBy(db.Sessions, accA, func(s model.Session) string { return s.AccountID }); n != 0 {
		t.Fatalf("RÉSURGENCE : %d session(s) recréée(s) après purge (attendu 0)", n)
	}
	if rt.UnknownOnRouter != 0 {
		t.Fatalf("UnknownOnRouter attendu 0 (tombstonés = purge assumée), obtenu %d", rt.UnknownOnRouter)
	}
	if rt.HotspotUsers != 3 {
		t.Fatalf("compteur télémétrie routeur : 3 attendu (état BRUT du routeur), obtenu %d", rt.HotspotUsers)
	}
	st.Unlock()
}

// TestTombstoneExpiryRestoresImport — après expiration du TTL, la découverte
// redevient possible : les mêmes comptes sont ré-importés (comportement
// historique de découverte des comptes créés dans Winbox).
func TestTombstoneExpiryRestoresImport(t *testing.T) {
	st, ts, token := newPurgeTestServer(t)
	accA, _ := purgeSeed(t, st)

	status, _ := doJSON(t, ts, "POST", "/api/admin/purge", token, map[string]any{
		"accountId": accA,
		"scopes":    []string{"vouchers", "hotspot_users"},
	})
	if status != http.StatusOK {
		t.Fatalf("purge ciblée : statut %d", status)
	}

	st.Lock()
	db := st.Data()
	past := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	for i := range db.PurgeTombstones {
		db.PurgeTombstones[i].ExpiresAt = past // TTL dépassé
	}
	rt := routerByID(db, "rt-pa-real")
	readStateUsers(t, db, rt, "VR-1|default|false;client-a|default|false", "")
	if n := countBy(db.HotspotUsers, accA, func(u model.HotspotUser) string { return u.AccountID }); n != 2 {
		t.Fatalf("après expiration : 2 utilisateurs importés attendus, obtenu %d", n)
	}
	if rt.UnknownOnRouter != 0 {
		t.Fatalf("UnknownOnRouter attendu 0 (import auto ON), obtenu %d", rt.UnknownOnRouter)
	}
	st.Unlock()
}

// TestAutoImportOffCountsUnknown — réglage autoImportRouterUsers=false :
// les comptes hors MikCloud ne sont PLUS importés automatiquement, ils sont
// comptés dans Router.UnknownOnRouter (adoption manuelle via l'outil d'import).
func TestAutoImportOffCountsUnknown(t *testing.T) {
	st, _, _ := newPurgeTestServer(t)
	accA, _ := purgeSeed(t, st)

	off := false
	st.Lock()
	db := st.Data()
	db.SettingsByAccount[accA] = model.Settings{AutoImportRouterUsers: &off}
	rt := routerByID(db, "rt-pa-real")
	readStateUsers(t, db, rt, "winbox-user|default|false", "winbox-user|10.0.0.9|30s|0|0")
	if n := countBy(db.HotspotUsers, accA, func(u model.HotspotUser) string { return u.AccountID }); n != 3 {
		t.Fatalf("import auto OFF : 3 users pré-existants attendus (aucun import), obtenu %d", n)
	}
	if rt.UnknownOnRouter != 1 {
		t.Fatalf("UnknownOnRouter attendu 1, obtenu %d", rt.UnknownOnRouter)
	}
	st.Unlock()
}

// TestCreateUserLiftsTombstone — la création VOLONTAIRE du même username lève
// le tombstone : l'identifiant est réclamé par l'opérateur, la purge ne doit
// plus le bloquer.
func TestCreateUserLiftsTombstone(t *testing.T) {
	st, ts, token := newPurgeTestServer(t)
	accA, _ := purgeSeed(t, st)

	status, _ := doJSON(t, ts, "POST", "/api/admin/purge", token, map[string]any{
		"accountId": accA,
		"scopes":    []string{"hotspot_users"},
	})
	if status != http.StatusOK {
		t.Fatalf("purge ciblée : statut %d", status)
	}

	st.Lock()
	db := st.Data()
	// Purge du scope hotspot_users seul : un SEUL tombstone (client-a — les
	// vouchers relèvent du scope « vouchers »).
	if n := len(db.PurgeTombstones); n != 1 {
		t.Fatalf("1 tombstone attendu avant levée, obtenu %d", n)
	}
	liftPurgeTombstone(db, accA, "client-a")
	if n := len(db.PurgeTombstones); n != 0 {
		t.Fatalf("tombstone non levé après création volontaire du même nom (reste %d)", n)
	}
	st.Unlock()
}

// TestPurgeAlsoRouterRequiresConfirm — purge TOTALE (P2) : sans la
// confirmation « SUPPRIMER », refus 400 (double garde client + serveur).
func TestPurgeAlsoRouterRequiresConfirm(t *testing.T) {
	st, ts, token := newPurgeTestServer(t)
	accA, _ := purgeSeed(t, st)

	status, _ := doJSON(t, ts, "POST", "/api/admin/purge", token, map[string]any{
		"accountId":  accA,
		"scopes":     []string{"hotspot_users"},
		"alsoRouter": true,
		// confirm absent
	})
	if status != http.StatusBadRequest {
		t.Fatalf("purge totale sans confirm : 400 attendu, obtenu %d", status)
	}
	status, _ = doJSON(t, ts, "POST", "/api/admin/purge", token, map[string]any{
		"accountId":  accA,
		"scopes":     []string{"hotspot_users"},
		"alsoRouter": true,
		"confirm":    "supprimer", // casse différente : refusée
	})
	if status != http.StatusBadRequest {
		t.Fatalf("confirm mal orthographié : 400 attendu, obtenu %d", status)
	}
}

// TestPurgeAlsoRouterQueuesRemovals — purge TOTALE confirmée : une commande
// user_remove (payload {"names": [...]}) est enfilée pour le routeur AGENT
// qui détenait les comptes purgés ; routerRemovals expose le total.
func TestPurgeAlsoRouterQueuesRemovals(t *testing.T) {
	st, ts, token := newPurgeTestServer(t)
	accA, _ := purgeSeed(t, st)

	status, out := doJSON(t, ts, "POST", "/api/admin/purge", token, map[string]any{
		"accountId":  accA,
		"scopes":     []string{"hotspot_users"},
		"alsoRouter": true,
		"confirm":    "SUPPRIMER",
	})
	if status != http.StatusOK {
		t.Fatalf("purge totale confirmée : statut %d", status)
	}
	if got := purgeCountIn(t, out, "routerRemovals"); got != 1 {
		t.Fatalf("routerRemovals attendu 1 (client-a sur le routeur agent), obtenu %d", got)
	}

	st.Lock()
	db := st.Data()
	found := false
	for _, c := range db.Commands {
		if c.Kind == model.CmdUserRemove && c.Status == "queued" && c.RouterID == "rt-pa-real" {
			// Store mémoire : le payload vit en []string natif (pas de round-trip
			// JSON → []any) — tester les deux formes pour rester robuste.
			var names []string
			switch v := c.Payload["names"].(type) {
			case []string:
				names = v
			case []any:
				for _, it := range v {
					if s, ok := it.(string); ok {
						names = append(names, s)
					}
				}
			}
			if len(names) == 1 && names[0] == "client-a" {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("commande user_remove attendue en file pour rt-pa-real (client-a)")
	}
	st.Unlock()
}

// TestCancelQueuedForPurged — la purge annule les commandes EN FILE qui
// recréeraient les entités purgées (user_add du nom purgé, voucher_batch du
// compte) mais laisse intactes celles déjà ENVOYÉES (le tombstone bloquera
// leur effet de ré-import au read_state suivant).
func TestCancelQueuedForPurged(t *testing.T) {
	st, ts, token := newPurgeTestServer(t)
	accA, _ := purgeSeed(t, st)

	st.Lock()
	db := st.Data()
	db.Commands = append(db.Commands,
		model.Command{ID: "cmd-add-1", AccountID: accA, RouterID: "rt-pa-real", Kind: model.CmdUserAdd, Status: "queued", Payload: map[string]any{"name": "VR-1"}},
		model.Command{ID: "cmd-batch-1", AccountID: accA, RouterID: "rt-pa-real", Kind: model.CmdVoucherBatch, Status: "queued"},
		model.Command{ID: "cmd-add-sent", AccountID: accA, RouterID: "rt-pa-real", Kind: model.CmdUserAdd, Status: "sent", Payload: map[string]any{"name": "VR-1"}},
	)
	st.Unlock()

	status, out := doJSON(t, ts, "POST", "/api/admin/purge", token, map[string]any{
		"accountId": accA,
		"scopes":    []string{"vouchers", "hotspot_users"},
	})
	if status != http.StatusOK {
		t.Fatalf("purge ciblée : statut %d", status)
	}
	if got := purgeCountIn(t, out, "tombstones"); got != 3 {
		t.Fatalf("tombstones attendus 3, obtenu %d", got)
	}

	st.Lock()
	db = st.Data()
	queued, sent := 0, 0
	for _, c := range db.Commands {
		if c.Status == "queued" {
			queued++
		}
		if c.Status == "sent" {
			sent++
		}
	}
	if queued != 0 {
		t.Fatalf("commandes en file après purge : 0 attendu, obtenu %d", queued)
	}
	if sent != 1 {
		t.Fatalf("commande sent doit être conservée : 1 attendue, obtenue %d", sent)
	}
	st.Unlock()
}
