package api

// Tests N°162 — autoréparation des utilisateurs absents du routeur
// (incident Zikisso 20/09/2026 : lot de vouchers généré pour « Wifi
// Zikisso », compte à DEUX routeurs — les ajouts avaient INTÉGRALEMENT
// échoué sur le routeur mais le script de lot avalait les échecs
// (on-error sans compteur) et rapportait « ok / created=N » : tickets
// « Actif / absent du routeur », connexion impossible, et AUCUNE
// retrouvabilité puisque les écritures ne sont jamais rejouées (N°73)).
//
// Le correctif a deux volets testés ici :
//   - la réconciliation COMPLÈTE renvoie les absents ACTIFS en commande
//     de réparation idempotente (garde d'in-flight, tombstones, backoff
//     N°159 dédié, limites fidèles au ticket vendu) ;
//   - l'issue de la vague (ok/error) pilote son backoff via
//     handleAgentResult (clé synthétique user_repair — les lots de
//     GÉNÉRATION classiques n'y touchent pas).

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"mikcloud/hotspot-api/internal/agent"
	"mikcloud/hotspot-api/internal/model"
	"mikcloud/hotspot-api/internal/store"
)

// repairSeed — l'état de l'incident : compte à deux routeurs, un profil,
// un voucher actif QUOTA+TEMPS résolus (le ticket vendu), plus les
// contre-exemples (autre routeur, used, disabled, profil supprimé,
// trop récent pour la grâce, purgé).
func repairSeed() (*model.DB, *model.Router) {
	old := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339)
	db := &model.DB{}
	db.Profiles = []model.Profile{{ID: "p-1", AccountID: "acc", Name: "1h-5go"}}
	db.Routers = []model.Router{
		{ID: "r-zk", AccountID: "acc", Name: "Wifi Zikisso", Mode: "agent", Status: "online"},
		{ID: "r-autre", AccountID: "acc", Name: "Autre routeur", Mode: "agent", Status: "online"},
	}
	db.HotspotUsers = []model.HotspotUser{
		// La victime : actif, profil résolvable, quota 5 Go + 60 min.
		{ID: "v-1", AccountID: "acc", RouterID: "r-zk", Username: "zk101", Password: "zk101",
			ProfileID: "p-1", ProfileName: "1h-5go", Status: "active", CreatedAt: old,
			DataQuotaMb: 5120, TimeLimitMin: 60},
		// User de l'AUTRE routeur du compte : jamais réparé par celui-ci.
		{ID: "v-2", AccountID: "acc", RouterID: "r-autre", Username: "other9", ProfileID: "p-1",
			Status: "active", CreatedAt: old},
		// Consommé : absence attendue, rien à réparer.
		{ID: "v-3", AccountID: "acc", RouterID: "r-zk", Username: "zk-used", ProfileID: "p-1",
			Status: "used", CreatedAt: old, MissingOnRouter: true},
		// Désactivé : hors périmètre (le add de réparation ne porte pas
		// disabled — le garder hors de la vague évite tout faux retour).
		{ID: "v-4", AccountID: "acc", RouterID: "r-zk", Username: "zk-dis", ProfileID: "p-1",
			Status: "disabled", CreatedAt: old, MissingOnRouter: true},
		// Profil supprimé : la réparation attend la réaffectation du gérant.
		{ID: "v-5", AccountID: "acc", RouterID: "r-zk", Username: "zk-noprof", ProfileID: "p-x",
			Status: "active", CreatedAt: old, MissingOnRouter: true},
		// Trop récent : couvert par la grâce (l'ajout peut être en file).
		{ID: "v-6", AccountID: "acc", RouterID: "r-zk", Username: "zk-fresh", ProfileID: "p-1",
			Status: "active", CreatedAt: time.Now().UTC().Format(time.RFC3339)},
	}
	return db, &db.Routers[0]
}

// repairReport — rapport read_state COMPLET (v4, non paginé) dont la liste
// d'utilisateurs NE CONTIENT PAS les vouchers de Zikisso : ils sont absents
// du routeur.
func repairReport() url.Values {
	vals := url.Values{}
	vals.Set("users", "quelquun|default|false;")
	vals.Set("sessions", "")
	return vals
}

// findRepairCommands — les commandes de réparation (voucher_batch marqué
// repair) du routeur, quel que soit leur statut.
func findRepairCommands(db *model.DB, routerID string) []*model.Command {
	out := []*model.Command{}
	for i := range db.Commands {
		c := &db.Commands[i]
		if c.RouterID == routerID && c.Kind == model.CmdVoucherBatch {
			if v, ok := c.Payload["repair"].(bool); ok && v {
				out = append(out, c)
			}
		}
	}
	return out
}

// TestApplyReadStateQueuesRepairForMissingActive — la réconciliation
// complète badgе la victime (absente du rapport, post-grâce) et enfile UNE
// commande de réparation FIDÈLE au ticket vendu : profil, mot de passe,
// quota et temps résolus à la génération. Les contre-exemples ne
// rejoignent JAMAIS la vague.
func TestApplyReadStateQueuesRepairForMissingActive(t *testing.T) {
	db, router := repairSeed()
	(&API{}).applyReadState(db, router, repairReport())

	byName := map[string]model.HotspotUser{}
	for _, u := range db.HotspotUsers {
		byName[u.Username] = u
	}
	if !byName["zk101"].MissingOnRouter {
		t.Fatal("zk101 absent du rapport complet, post-grâce : le badge doit être posé")
	}
	if byName["zk-fresh"].MissingOnRouter {
		t.Fatal("zk-fresh est dans la fenêtre de grâce : jamais badgé (l'ajout peut être en file)")
	}

	repairs := findRepairCommands(db, "r-zk")
	if len(repairs) != 1 {
		t.Fatalf("une commande de réparation attendue, obtenu %d", len(repairs))
	}
	cmd := repairs[0]
	if cmd.Status != "queued" || cmd.AccountID != "acc" {
		t.Fatalf("commande de réparation inattendue : %+v", cmd)
	}
	if c, _ := cmd.Payload["comment"].(string); c != "mikcloud:repair" {
		t.Fatalf("commentaire de traçabilité = %q, attendu mikcloud:repair", cmd.Payload["comment"])
	}
	prof, _ := cmd.Payload["profile"].(map[string]any)
	if prof == nil || prof["name"] != "1h-5go" {
		t.Fatalf("profileRef attendue dans la vague (autoguérison du profil), obtenu %v", cmd.Payload["profile"])
	}
	users, _ := cmd.Payload["users"].([]map[string]any)
	if len(users) != 1 {
		t.Fatalf("un seul utilisateur réparable (les contre-exemples exclus), obtenu %d", len(users))
	}
	u := users[0]
	if u["name"] != "zk101" || u["password"] != "zk101" {
		t.Fatalf("identités du voucher : %+v", u)
	}
	if q, _ := u["limitBytesTotal"].(int64); q != 5120*1048576 {
		t.Fatalf("quota fidèle au ticket vendu : limitBytesTotal = %v, attendu %d (5 Go)", u["limitBytesTotal"], int64(5120)*1048576)
	}
	if m, _ := u["limitUptimeMin"].(int64); m != 60 {
		t.Fatalf("temps fidèle au ticket vendu : limitUptimeMin = %v, attendu 60", u["limitUptimeMin"])
	}
	// Le journal du compte porte la trace lisible par le gérant.
	found := false
	for _, l := range db.Activity {
		if l.AccountID == "acc" && strings.Contains(l.Message, "Réparation") && strings.Contains(l.Message, "Wifi Zikisso") {
			found = true
		}
	}
	if !found {
		t.Fatal("une ligne « Réparation … renvoyé(s) sur «Wifi Zikisso» » doit vivre au journal du compte")
	}
}

// TestApplyReadStateRepairSkipsTombstoned — un username purgé par le gérant
// n'est JAMAIS ressuscité, même badgé absent : la garde
// anti-résurrection s'applique à la vague de réparation.
func TestApplyReadStateRepairSkipsTombstoned(t *testing.T) {
	db, router := repairSeed()
	db.PurgeTombstones = []model.PurgeTombstone{{
		ID: "pt-1", AccountID: "acc", Username: "zk101",
		PurgedAt:  time.Now().UTC().Format(time.RFC3339),
		ExpiresAt: time.Now().UTC().Add(30 * 24 * time.Hour).Format(time.RFC3339),
	}}
	(&API{}).applyReadState(db, router, repairReport())

	if len(findRepairCommands(db, "r-zk")) != 0 {
		t.Fatal("un username tombstoné (purgé) ne doit jamais être ressuscité par la réparation")
	}
}

// TestApplyReadStateRepairInFlightNoDup — une vague déjà en file (queued)
// ou en vol (sent) ne double JAMAIS ; un lot de GÉNÉRATION classique en
// file ne bloque pas la vague (clés distinctes).
func TestApplyReadStateRepairInFlightNoDup(t *testing.T) {
	db, router := repairSeed()
	db.Commands = []model.Command{{
		ID: "c-inflight", RouterID: "r-zk", AccountID: "acc", Kind: model.CmdVoucherBatch,
		Payload: map[string]any{"repair": true}, Status: "queued", CreatedAt: model.NowISO(),
	}}
	(&API{}).applyReadState(db, router, repairReport())
	if n := len(findRepairCommands(db, "r-zk")); n != 1 {
		t.Fatalf("vague en file : aucun doublon attendu, obtenu %d", n)
	}

	// En vol (sent) : idem.
	db.Commands[0].Status = "sent"
	(&API{}).applyReadState(db, router, repairReport())
	if n := len(findRepairCommands(db, "r-zk")); n != 1 {
		t.Fatalf("vague en vol : aucun doublon attendu, obtenu %d", n)
	}

	// Un lot de GÉNÉRATION (sans marqueur repair) n'occupe PAS la place de
	// la vague : la réparation doit pouvoir partir (l'incident Zikisso est
	// précisément un lot de génération en échec silencieux).
	db.Commands[0].Status = "done"
	db.Commands[0].Payload = map[string]any{"batch": "B20260920-0001"}
	(&API{}).applyReadState(db, router, repairReport())
	if n := len(findRepairCommands(db, "r-zk")); n != 1 {
		t.Fatalf("lot de génération terminé : la vague doit partir, obtenu %d réparation(s)", n)
	}
}

// TestApplyReadStateRepairBackoffGates — après un échec de vague, le
// backoff N°159 (clé synthétique user_repair) cadence les retries : bloqué
// pendant le palier, libéré à son expiration.
func TestApplyReadStateRepairBackoffGates(t *testing.T) {
	db, router := repairSeed()
	a := &API{}
	now := time.Now().UTC()
	a.recordWatcherError(router.ID, repairBackoffKind, now)

	a.applyReadState(db, router, repairReport())
	if n := len(findRepairCommands(db, "r-zk")); n != 0 {
		t.Fatalf("backoff actif (1 min après 1 échec) : aucune vague attendue, obtenu %d", n)
	}

	// Palier expiré (échec il y a 2 min, palier 1 min) : la vague repart.
	if a.watcherFailAt == nil {
		t.Fatal("état backoff attendu après recordWatcherError")
	}
	k := router.ID + "|" + repairBackoffKind
	a.watcherFailAt[k] = now.Add(-2 * time.Minute)
	a.applyReadState(db, router, repairReport())
	if n := len(findRepairCommands(db, "r-zk")); n != 1 {
		t.Fatalf("palier expiré : la vague doit repartir, obtenu %d", n)
	}
}

// TestRepairResultHookDrivesBackoff — par le VRAI handler /agent/result :
// un rapport « error » d'une vague fait monter le palier (la réconciliation
// suivante ne re-file pas), un rapport « ok » réinitialise (re-file
// immédiat). Les lots de GÉNÉRATION classiques ne touchent pas cette
// cadence.
func TestRepairResultHookDrivesBackoff(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("ADMIN_PASSWORD", "admin-test-1234")
	t.Setenv("ADMIN_USERNAME", "")
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatalf("store.New impossible : %v", err)
	}
	a := New(st, testJWTSecret)
	ts := httptest.NewServer(a.Handler())
	t.Cleanup(ts.Close)

	old := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339)
	st.Lock()
	db := st.Data()
	db.Profiles = []model.Profile{{ID: "p-1", AccountID: "acc", Name: "1h-5go"}}
	db.Routers = []model.Router{{
		ID: "r-zk", AccountID: "acc", Name: "Wifi Zikisso", Mode: "agent",
		Status: "online", AgentTokenHash: "hash-zk",
	}}
	db.HotspotUsers = []model.HotspotUser{{
		ID: "v-1", AccountID: "acc", RouterID: "r-zk", Username: "zk101", Password: "zk101",
		ProfileID: "p-1", ProfileName: "1h-5go", Status: "active", CreatedAt: old,
		DataQuotaMb: 5120, TimeLimitMin: 60,
	}}
	st.Unlock()

	// routerByToken hache le jeton : poser le hash correspondant à « tok-zk ».
	st.Lock()
	db = st.Data()
	db.Routers[0].AgentTokenHash = agent.HashToken("tok-zk")
	st.Unlock()

	// La réconciliation enfile la première vague (utilisateur absent).
	st.Lock()
	db = st.Data()
	router := &db.Routers[0]
	a.applyReadState(db, router, repairReport())
	st.Unlock()
	repairs := findRepairCommands(st.Data(), "r-zk")
	if len(repairs) != 1 {
		t.Fatalf("première vague attendue, obtenu %d", len(repairs))
	}

	// Rapport « error » (la cause racine n'est pas corrigée) : palier 1.
	form := url.Values{"token": {"tok-zk"}, "cmd": {repairs[0].ID}, "status": {"error"}}
	if resp, err := ts.Client().PostForm(ts.URL+"/agent/result", form); err != nil || resp.StatusCode != 200 {
		t.Fatalf("rapport error : err=%v statut=%v", err, resp)
	} else {
		resp.Body.Close()
	}
	k := "r-zk|" + repairBackoffKind
	if a.watcherFailN[k] != 1 {
		t.Fatalf("après un échec de vague : palier 1 attendu, obtenu %d", a.watcherFailN[k])
	}

	// Réconciliation suivante : bloquée par le backoff.
	st.Lock()
	db = st.Data()
	router = &db.Routers[0]
	a.applyReadState(db, router, repairReport())
	st.Unlock()
	if n := len(findRepairCommands(st.Data(), "r-zk")); n != 1 {
		t.Fatalf("backoff actif : pas de nouvelle vague, obtenu %d", n)
	}

	// Le backoff expire (palier 1 min) : nouvelle vague en file…
	if a.watcherFailAt == nil {
		t.Fatal("état backoff attendu")
	}
	a.watcherFailAt[k] = time.Now().UTC().Add(-2 * time.Minute)
	st.Lock()
	db = st.Data()
	router = &db.Routers[0]
	a.applyReadState(db, router, repairReport())
	st.Unlock()
	repairs = findRepairCommands(st.Data(), "r-zk")
	if len(repairs) != 2 {
		t.Fatalf("palier expiré : seconde vague attendue, obtenu %d", len(repairs))
	}

	// … rapport « ok » : réinitialisation immédiate — et un lot de
	// GÉNÉRATION rapporté « error » ne touche PAS la cadence de réparation.
	form = url.Values{"token": {"tok-zk"}, "cmd": {repairs[1].ID}, "status": {"ok"}}
	if resp, err := ts.Client().PostForm(ts.URL+"/agent/result", form); err != nil || resp.StatusCode != 200 {
		t.Fatalf("rapport ok : err=%v statut=%v", err, resp)
	} else {
		resp.Body.Close()
	}
	if a.watcherFailN[k] != 0 {
		t.Fatalf("après un ok de vague : backoff réinitialisé, obtenu palier %d", a.watcherFailN[k])
	}

	st.Lock()
	db = st.Data()
	gen := model.Command{ID: "c-gen", RouterID: "r-zk", AccountID: "acc", Kind: model.CmdVoucherBatch,
		Payload: map[string]any{"batch": "B-gen"}, Status: "sent", CreatedAt: model.NowISO()}
	db.Commands = append(db.Commands, gen)
	st.Unlock()
	form = url.Values{"token": {"tok-zk"}, "cmd": {"c-gen"}, "status": {"error"}}
	if resp, err := ts.Client().PostForm(ts.URL+"/agent/result", form); err != nil || resp.StatusCode != 200 {
		t.Fatalf("rapport error génération : err=%v statut=%v", err, resp)
	} else {
		resp.Body.Close()
	}
	if a.watcherFailN[k] != 0 {
		t.Fatalf("un lot de GÉNÉRATION en échec ne doit pas monter le palier de réparation, obtenu %d", a.watcherFailN[k])
	}
}
