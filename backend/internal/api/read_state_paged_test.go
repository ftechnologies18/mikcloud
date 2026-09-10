package api

// Tests N°76 — read_state PAGINÉ (v5). Motivation vue en production : le compte
// ProMax WIFI (3 478 vouchers actifs) affichait 3 334 faux badges « absent du
// routeur » — posés avant N°75 quand le cap était 150, figés après N°75 (le
// rapport tronqué en permanence ne posait NI ne levait AUCUN badge : la
// réconciliation ne tournait plus jamais), compteur de parc gelé à 150.
//
// v5 découpe le parc en fenêtres de 500 : chunk 0 découvre le total et enfile
// la suite, les chunks s'accumulent, la réconciliation ne s'applique QU'AU
// CYCLE COMPLET (badges, import inconnus, diff sessions) — un chunk perdu
// abandonne le cycle sans AUCUNE déduction (honnêteté v2/v4 : rien n'est pire
// qu'un badge mensonger).

import (
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"mikcloud/hotspot-api/internal/agent"
	"mikcloud/hotspot-api/internal/model"
)

// pagedSeed — état de base type ProMax : un routeur agent (pas 45 s, compteur
// de parc GELÉ à 150, 25 sessions actives au compteur), des users actifs
// badgés à tort (l'artefact pré-N°75), un user réellement absent du routeur
// (carol — supprimé dans Winbox), un voucher « used » porteur du badge
// (artefact : le badge n'a de sens que pour un user actif/disabled), et une
// session live précédente.
func pagedSeed() (*model.DB, *model.Router, *API) {
	old := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339)
	db := &model.DB{}
	db.Routers = []model.Router{{ID: "r-pm", AccountID: "acc", Name: "PM", Mode: "agent", Status: "online",
		HotspotUsers: 150, ActiveSessions: 25, SchedulerSec: 45}}
	db.HotspotUsers = []model.HotspotUser{
		{ID: "u-alice", AccountID: "acc", RouterID: "r-pm", Username: "alice", Status: "active", CreatedAt: old, MissingOnRouter: true},
		{ID: "u-bob", AccountID: "acc", RouterID: "r-pm", Username: "bob", Status: "active", CreatedAt: old, MissingOnRouter: true},
		{ID: "u-carol", AccountID: "acc", RouterID: "r-pm", Username: "carol", Status: "active", CreatedAt: old, MissingOnRouter: true},
		{ID: "u-dave", AccountID: "acc", RouterID: "r-pm", Username: "dave", Status: "used", CreatedAt: old, MissingOnRouter: true},
	}
	// 1 199 users de remplissage badgés à tort (la masse ProMax : au-delà de
	// l'ancienne borne 150, jamais couverts par le rapport v4).
	for i := 0; i < 1199; i++ {
		db.HotspotUsers = append(db.HotspotUsers, model.HotspotUser{
			ID: fmt.Sprintf("u-f%04d", i), AccountID: "acc", RouterID: "r-pm",
			Username: fmt.Sprintf("f%04d", i), Status: "active", CreatedAt: old, MissingOnRouter: true,
		})
	}
	// Parc routeur : alice + bob + 1 199 fillers = 1 201 users (carol absente,
	// dave used → pas sur le routeur). Fenêtres de 500 : [0,500), [500,1000),
	// [1000,1201).
	db.Sessions = []model.Session{{ID: "s-prev", AccountID: "acc", RouterID: "r-pm", Username: "alice", StartedAt: old}}
	a := &API{}
	return db, &db.Routers[0], a
}

// chunkVals — rapport v5 d'une fenêtre : users = les fillers f0000..f0NNN de
// la fenêtre (+ alice en 0, bob en 500), sessions au chunk final seulement.
func chunkVals(start, count, total int, users []string, sessions string) url.Values {
	v := url.Values{}
	v.Set("total", fmt.Sprintf("%d", total))
	v.Set("start", fmt.Sprintf("%d", start))
	v.Set("count", fmt.Sprintf("%d", count))
	v.Set("out", fmt.Sprintf("%d", len(users)))
	var sb strings.Builder
	for _, u := range users {
		sb.WriteString(u + "|default|false;")
	}
	v.Set("users", sb.String())
	v.Set("trunc", "true")
	if sessions != "" {
		v.Set("stotal", "1")
		v.Set("sessions", sessions)
	} else {
		v.Set("stotal", "1")
		v.Set("sessions", "")
	}
	return v
}

// windowUsers — fillers de la fenêtre [start, end) du parc reconstitué :
// 1 201 positions (alice en 0, bob en 500) ; les fillers f0000..f1198
// occupent les positions restantes (décalage −1 avant bob, −2 après).
func windowUsers(start, end int) []string {
	out := []string{}
	for pos := start; pos < end; pos++ {
		switch pos {
		case 0:
			out = append(out, "alice")
		case 500:
			out = append(out, "bob")
		default:
			idx := pos - 1
			if pos > 500 {
				idx = pos - 2
			}
			if idx >= 0 && idx < 1199 {
				out = append(out, fmt.Sprintf("f%04d", idx))
			}
		}
	}
	return out
}

// TestPagedCycleLiftsFalseBadges — LE scénario ProMax : un cycle complet de 3
// chunks lève les 1 200 faux badges (alice, bob, fillers), garde carol absente
// (véritablement supprimée du routeur), nettoie le badge du voucher « used »,
// pose le compteur de parc EXACT et journalise UNE seule synchronisation.
func TestPagedCycleLiftsFalseBadges(t *testing.T) {
	db, router, a := pagedSeed()
	const total = 1201

	// Chunk 0 : accumulation, enfilement de la suite, ZÉRO déduction.
	final, synced := a.applyReadState(db, router, chunkVals(0, 500, total, windowUsers(0, 500), ""))
	if final || synced {
		t.Fatalf("chunk intermédiaire : final=%v synced=%v attendus (false, false)", final, synced)
	}
	if router.HotspotUsers != total {
		t.Fatalf("chunk 0 : compteur parc = total exact (%d) attendu, obtenu %d", total, router.HotspotUsers)
	}
	if db.HotspotUsers[0].MissingOnRouter != true {
		t.Fatal("chunk intermédiaire : AUCUN badge ne doit bouger (alice encore badgée — pas de déduction prématurée)")
	}
	// La suite du cycle est enfileée d'un coup : chunks 500 et 1000.
	queued := map[int]bool{}
	for _, c := range db.Commands {
		if c.Kind == model.CmdReadState && c.Status == "queued" {
			queued[int(plPayloadInt(c.Payload, "start"))] = true
		}
	}
	if !queued[500] || !queued[1000] {
		t.Fatalf("chunk 0 : les fenêtres restantes (500, 1000) doivent être enfilées, obtenu %v", queued)
	}

	// Chunk 500 : accumulation silencieuse.
	final, synced = a.applyReadState(db, router, chunkVals(500, 500, total, windowUsers(500, 1000), ""))
	if final || synced {
		t.Fatalf("chunk 500 : final=%v synced=%v attendus (false, false)", final, synced)
	}

	// Chunk final 1000 (porte les sessions).
	final, synced = a.applyReadState(db, router, chunkVals(1000, 500, total, windowUsers(1000, total), "alice|10.0.0.9|00:01:00|100|50;"))
	if !final || !synced {
		t.Fatalf("chunk final : final=%v synced=%v attendus (true, true)", final, synced)
	}

	byName := map[string]model.HotspotUser{}
	for _, u := range db.HotspotUsers {
		byName[u.Username] = u
	}
	if byName["alice"].MissingOnRouter {
		t.Fatal("cycle complet : alice EST sur le routeur (chunk 0) — son faux badge doit être LEVÉ")
	}
	if byName["bob"].MissingOnRouter {
		t.Fatal("cycle complet : bob EST sur le routeur (chunk 500) — son faux badge doit être LEVÉ")
	}
	if !byName["carol"].MissingOnRouter {
		t.Fatal("cycle complet : carol est VRAIMENT absente du routeur (aucune fenêtre) — son badge doit rester POSÉ")
	}
	if byName["dave"].MissingOnRouter {
		t.Fatal("cycle complet : dave est « used » — le badge n'a de sens que pour un user actif, il doit être levé (nettoyage N°76)")
	}
	for i := 0; i < 1199; i++ {
		if byName[fmt.Sprintf("f%04d", i)].MissingOnRouter {
			t.Fatalf("cycle complet : le filler f%04d est sur le routeur — faux badge de masse non levé", i)
		}
	}
	if router.HotspotUsers != total {
		t.Fatalf("cycle complet : compteur parc exact (%d) attendu, obtenu %d", total, router.HotspotUsers)
	}
	if router.ActiveSessions != 1 {
		t.Fatalf("cycle complet : ActiveSessions = 1 (stotal), obtenu %d", router.ActiveSessions)
	}
	// La session d'alice est conservée (diff apparié sur l'username), pas de logout fantôme.
	if len(db.Sessions) != 1 || db.Sessions[0].Username != "alice" {
		t.Fatalf("cycle complet : la session live d'alice doit être remplacée par celle du rapport, obtenu %v", db.Sessions)
	}
	logins, logouts := 0, 0
	for _, l := range db.UserLogs {
		switch l.Action {
		case "login":
			logins++
		case "logout":
			logouts++
		}
	}
	if logouts != 0 || logins != 0 {
		t.Fatalf("cycle complet : session ré-appariée → aucun login/logout attendu, obtenu %d/%d", logins, logouts)
	}
	// L'accumulateur du cycle est purgé.
	if len(a.readAcc) != 0 {
		t.Fatalf("cycle complet : l'accumulateur doit être purgé, obtenu %d entrée(s)", len(a.readAcc))
	}
}

// TestPagedCycleLostChunkDefers — un chunk PERDU (chunk 500 muet) : le chunk
// final arrive mais la complétude est indémontrable → AUCUNE déduction (ni
// levée ni pose de badge), compteurs exacts conservés — honnêteté N°75.
func TestPagedCycleLostChunkDefers(t *testing.T) {
	db, router, a := pagedSeed()
	const total = 1201

	a.applyReadState(db, router, chunkVals(0, 500, total, windowUsers(0, 500), ""))
	// chunk 500 : JAMAIS rapporté (perte réseau).
	final, synced := a.applyReadState(db, router, chunkVals(1000, 500, total, windowUsers(1000, total), "alice|10.0.0.9|00:01:00|100|50;"))
	if !final {
		t.Fatal("chunk 1000 : final attendu (fin de cycle)")
	}
	if synced {
		t.Fatal("cycle incomplet : synced=false attendu (aucune déduction sur complétude indémontrable)")
	}
	byName := map[string]model.HotspotUser{}
	for _, u := range db.HotspotUsers {
		byName[u.Username] = u
	}
	if !byName["alice"].MissingOnRouter {
		t.Fatal("cycle incomplet : le badge d'alice doit rester EN L'ÉTAT (posé) — aucune levée sans complétude")
	}
	if byName["carol"].MissingOnRouter != true {
		t.Fatal("cycle incomplet : badge de carol inchangé")
	}
	if router.HotspotUsers != total {
		t.Fatalf("cycle incomplet : compteur parc exact (%d) attendu quand même, obtenu %d", total, router.HotspotUsers)
	}
}

// TestPagedFinalWithoutAccumulatorInert — chunk final orphelin (reboot cloud,
// re-queue zombie tardif) : inert — télémétrie seule, aucune déduction.
func TestPagedFinalWithoutAccumulatorInert(t *testing.T) {
	db, router, a := pagedSeed()
	final, synced := a.applyReadState(db, router, chunkVals(1000, 500, 1201, windowUsers(1000, 1201), ""))
	if !final || synced {
		t.Fatalf("orphelin : final=true synced=false attendus, obtenu (%v, %v)", final, synced)
	}
	if !db.HotspotUsers[0].MissingOnRouter {
		t.Fatal("orphelin : badge d'alice inchangé attendu")
	}
}

// TestPagedOrphanIntermediateQueuesNothing — un chunk intermédiaire orphelin
// (start > 0, aucun cycle en cours) n'enfile RIEN et ne crée pas d'état.
func TestPagedOrphanIntermediateQueuesNothing(t *testing.T) {
	db, router, a := pagedSeed()
	a.applyReadState(db, router, chunkVals(500, 500, 1201, windowUsers(500, 1000), ""))
	if len(db.Commands) != 0 {
		t.Fatalf("chunk orphelin : aucune commande ne doit être enfilée, obtenu %d", len(db.Commands))
	}
	if len(a.readAcc) != 0 {
		t.Fatal("chunk orphelin : aucun accumulateur ne doit être créé pour un start > 0 sans cycle")
	}
}

// TestPagedSingleChunkCycleReconciles — petit parc (total ≤ 500) : le rapport
// de base est DÉJÀ le cycle complet — comportement identique à v4 (réconciliation
// directe, badge posé pour l'absent, compteur exact).
func TestPagedSingleChunkCycleReconciles(t *testing.T) {
	old := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339)
	db := &model.DB{}
	db.Routers = []model.Router{{ID: "r-1", AccountID: "acc", Name: "R1", Mode: "agent", Status: "online", HotspotUsers: 0}}
	db.HotspotUsers = []model.HotspotUser{
		{ID: "u-a", AccountID: "acc", RouterID: "r-1", Username: "a", Status: "active", CreatedAt: old},
		{ID: "u-b", AccountID: "acc", RouterID: "r-1", Username: "b", Status: "active", CreatedAt: old, MissingOnRouter: true},
	}
	a := &API{}
	v := url.Values{}
	v.Set("total", "2")
	v.Set("start", "0")
	v.Set("count", "500")
	v.Set("out", "1")
	v.Set("users", "a|default|false;")
	v.Set("stotal", "0")
	v.Set("sessions", "")
	final, synced := a.applyReadState(db, &db.Routers[0], v)
	if !final || !synced {
		t.Fatalf("mono-chunk : final=true synced=true attendus, obtenu (%v, %v)", final, synced)
	}
	byName := map[string]model.HotspotUser{}
	for _, u := range db.HotspotUsers {
		byName[u.Username] = u
	}
	if byName["b"].MissingOnRouter != true {
		t.Fatal("mono-chunk : b absent du rapport complet → badge posé (comportement historique)")
	}
	if db.Routers[0].HotspotUsers != 2 {
		t.Fatalf("mono-chunk : compteur parc = 2 attendu, obtenu %d", db.Routers[0].HotspotUsers)
	}
}

// TestPagedSessionListTruncatedKeepsState — liste de sessions bornée à 250
// (stotal > entrées rapportées) : le diff sessions est suspendu (pas de
// logouts en cascade), le compteur exact est posé via stotal.
func TestPagedSessionListTruncatedKeepsState(t *testing.T) {
	old := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339)
	db := &model.DB{}
	db.Routers = []model.Router{{ID: "r-1", AccountID: "acc", Name: "R1", Mode: "agent", Status: "online", ActiveSessions: 3}}
	db.HotspotUsers = []model.HotspotUser{
		{ID: "u-a", AccountID: "acc", RouterID: "r-1", Username: "a", Status: "active", CreatedAt: old},
	}
	db.Sessions = []model.Session{
		{ID: "s-1", AccountID: "acc", RouterID: "r-1", Username: "a", StartedAt: old},
		{ID: "s-2", AccountID: "acc", RouterID: "r-1", Username: "b", StartedAt: old},
	}
	a := &API{}
	v := url.Values{}
	v.Set("total", "1")
	v.Set("start", "0")
	v.Set("count", "500")
	v.Set("users", "a|default|false;")
	v.Set("stotal", "300") // 300 sessions actives, liste rapportée vide (bornée)
	v.Set("sessions", "")
	final, synced := a.applyReadState(db, &db.Routers[0], v)
	if !final || !synced {
		t.Fatalf("sessions tronquées : final=true synced=true attendus, obtenu (%v, %v)", final, synced)
	}
	if len(db.Sessions) != 2 {
		t.Fatalf("sessions tronquées : l'état précédent (2 sessions) doit être conservé, obtenu %d", len(db.Sessions))
	}
	if db.Routers[0].ActiveSessions != 300 {
		t.Fatalf("sessions tronquées : compteur exact via stotal (300) attendu, obtenu %d", db.Routers[0].ActiveSessions)
	}
}

// TestQueueReadStateFreshSkipsInFlightCycle — la fraîcheur post-écriture ne
// casse PAS un cycle en cours : avec un chunk en file, queueReadStateFreshLocked
// retourne le cycle lui-même (aucun chunk 0 concurrent).
func TestQueueReadStateFreshSkipsInFlightCycle(t *testing.T) {
	db := &model.DB{}
	router := &model.Router{ID: "r-1", AccountID: "acc"}
	db.Commands = []model.Command{
		{ID: "c-chunk", RouterID: "r-1", AccountID: "acc", Kind: model.CmdReadState, Status: "queued",
			Payload: map[string]any{"start": 500, "count": 500}},
	}
	cmd := queueReadStateFreshLocked(db, router)
	if cmd.ID != "c-chunk" {
		t.Fatalf("cycle en cours : la garde doit retourner le chunk en file, obtenu %v", cmd.ID)
	}
	// Rien en file/en vol → base enfilée.
	db.Commands = nil
	cmd = queueReadStateFreshLocked(db, router)
	if cmd.Kind != model.CmdReadState || int(plPayloadInt(cmd.Payload, "start")) != 0 {
		t.Fatalf("aucun cycle : un read_state de base doit être enfilé, obtenu %+v", cmd)
	}
}

// TestEnsureReadStateDueScalesWithParkSize — cadence adaptée : un parc de 7
// chunks (3 500 users) se réconcilie au maximum toutes les 2 min × 7 — pas de
// re-cycle prématuré qui gonflerait l'egress scripts.
func TestEnsureReadStateDueScalesWithParkSize(t *testing.T) {
	db := &model.DB{}
	router := &model.Router{ID: "r-1", AccountID: "acc", Mode: "agent"}
	a := &API{readStateDone: map[string]time.Time{}, readStateChunks: map[string]int{}}

	// Frais (3 min) ET parc lourd (7 chunks → intervalle 14 min) : rien.
	a.readStateDone[router.ID] = time.Now().UTC().Add(-3 * time.Minute)
	a.readStateChunks[router.ID] = 7
	a.ensureReadStateDue(db, router)
	if len(db.Commands) != 0 {
		t.Fatal("parc lourd frais : aucun read_state ne doit être enfilé avant 2 min × 7 chunks")
	}

	// Périmé pour un parc lourd (15 min > 14 min) : cycle relancé.
	a.readStateDone[router.ID] = time.Now().UTC().Add(-15 * time.Minute)
	a.ensureReadStateDue(db, router)
	if len(db.Commands) != 1 || db.Commands[0].Kind != model.CmdReadState {
		t.Fatal("parc lourd périmé : un nouveau cycle (base) doit être enfilé")
	}

	// Petit parc (1 chunk) : cadence 2 min inchangée.
	db.Commands = nil
	a.readStateChunks[router.ID] = 1
	a.readStateDone[router.ID] = time.Now().UTC().Add(-3 * time.Minute)
	a.ensureReadStateDue(db, router)
	if len(db.Commands) != 1 {
		t.Fatal("petit parc périmé (3 min > 2 min) : un read_state doit être enfilé (cadence N°74 inchangée)")
	}
}

// TestPagedGraceCoversCycleDuration — la grâce anti-faux-badge des users
// récents couvre la durée du CYCLE paginé (2 min + chunks × pas scheduler) :
// un user créé PENDANT le cycle ne doit pas être badgé parce que sa fenêtre
// est déjà passée (ici : créé il y a 10 min, cycle de 3 chunks en veille
// 180 s → grâce 11 min).
func TestPagedGraceCoversCycleDuration(t *testing.T) {
	db, router, a := pagedSeed()
	router.SchedulerSec = 180
	const total = 1201
	// Un user créé il y a 10 min (SA commande user_add peut être en file, ou
	// sa fenêtre d'index est déjà rapportée) — absent de toutes les fenêtres.
	db.HotspotUsers = append(db.HotspotUsers, model.HotspotUser{
		ID: "u-new", AccountID: "acc", RouterID: "r-pm", Username: "new", Status: "active",
		CreatedAt: time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339),
	})

	a.applyReadState(db, router, chunkVals(0, 500, total, windowUsers(0, 500), ""))
	a.applyReadState(db, router, chunkVals(500, 500, total, windowUsers(500, 1000), ""))
	_, synced := a.applyReadState(db, router, chunkVals(1000, 500, total, windowUsers(1000, total), ""))
	if !synced {
		t.Fatal("cycle complet : synced attendu")
	}
	byName := map[string]model.HotspotUser{}
	for _, u := range db.HotspotUsers {
		byName[u.Username] = u
	}
	if byName["new"].MissingOnRouter {
		t.Fatal("user créé il y a 10 min pendant un cycle de 3 chunks en veille (180 s) : DANS la grâce (2 min + 3×180 s = 11 min) — pas de badge")
	}
	// Un user ANCIEN vraiment absent reste badgé : la grâce ne masque pas tout.
	if !byName["carol"].MissingOnRouter {
		t.Fatal("user ancien réellement absent : le badge doit être posé (la grâce ne couvre que les récents)")
	}
}

// TestPagedAutoImportUnknownFromFullCycle — l'import automatique des inconnus
// (créés hors MikCloud) s'applique sur le cycle COMPLET, avec profil/disabled
// de leur fenêtre d'origine.
func TestPagedAutoImportUnknownFromFullCycle(t *testing.T) {
	old := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339)
	db := &model.DB{}
	db.Routers = []model.Router{{ID: "r-1", AccountID: "acc", Name: "R1", Mode: "agent", Status: "online"}}
	db.HotspotUsers = []model.HotspotUser{
		{ID: "u-a", AccountID: "acc", RouterID: "r-1", Username: "a", Status: "active", CreatedAt: old},
	}
	a := &API{}

	v0 := url.Values{}
	v0.Set("total", "700")
	v0.Set("start", "0")
	v0.Set("count", "500")
	v0.Set("users", "a|default|false;")
	v0.Set("stotal", "0")
	a.applyReadState(db, &db.Routers[0], v0)

	v1 := url.Values{}
	v1.Set("total", "700")
	v1.Set("start", "500")
	v1.Set("count", "500")
	v1.Set("users", "inconnu|profilX|true;")
	v1.Set("stotal", "0")
	v1.Set("sessions", "")
	a.applyReadState(db, &db.Routers[0], v1)

	byName := map[string]model.HotspotUser{}
	for _, u := range db.HotspotUsers {
		byName[u.Username] = u
	}
	imp, ok := byName["inconnu"]
	if !ok {
		t.Fatal("cycle complet : l'inconnu de la fenêtre 500 doit être importé (import auto actif par défaut)")
	}
	if imp.ProfileName != "profilX" || imp.Status != "disabled" {
		t.Fatalf("import : profil/disabled de la fenêtre d'origine attendus (profilX/disabled), obtenu %s/%s", imp.ProfileName, imp.Status)
	}
	if byName["a"].MissingOnRouter {
		t.Fatal("cycle complet : « a » est sur le routeur — pas de badge")
	}
}

// TestPagedCycleOverAbsoluteCapDefers — parc au-delà de la borne absolue
// (MaxReadChunks × 500 = 10 000) : le cycle s'arrête à la borne, AUCUNE
// déduction (les users au-delà seraient de faux absents), compteurs exacts.
func TestPagedCycleOverAbsoluteCapDefers(t *testing.T) {
	db, router, a := pagedSeed()
	const total = 1201
	// total simulé au-delà de la borne : le cloud doit refuser d'enfiler
	// au-delà de MaxReadChunks fenêtres ET ne rien déduire au final borné.
	v0 := url.Values{}
	v0.Set("total", "11000")
	v0.Set("start", "0")
	v0.Set("count", "500")
	v0.Set("users", "alice|default|false;")
	v0.Set("stotal", "0")
	a.applyReadState(db, router, v0)
	queued := 0
	for _, c := range db.Commands {
		if c.Kind == model.CmdReadState && c.Status == "queued" {
			queued++
		}
	}
	// 10 500 / 500 - 1 = 20 chunks restants maximum (borne MaxReadChunks).
	if queued > agent.MaxReadChunks-1 {
		t.Fatalf("parc > borne : au plus %d chunks enfilés, obtenu %d", agent.MaxReadChunks-1, queued)
	}
	if router.HotspotUsers != 11000 {
		t.Fatalf("parc > borne : compteur exact (11000) attendu quand même, obtenu %d", router.HotspotUsers)
	}

	// Le chunk final BORNÉ (start = 9500 = (MaxReadChunks-1)×500) est final
	// (winCap atteint) mais le cycle est incomplet PAR CONSTRUCTION.
	vf := url.Values{}
	vf.Set("total", "11000")
	vf.Set("start", "9500")
	vf.Set("count", "500")
	vf.Set("users", "f0999|default|false;")
	vf.Set("stotal", "0")
	vf.Set("sessions", "")
	final, synced := a.applyReadState(db, router, vf)
	if !final {
		t.Fatal("chunk borné : final=true attendu (fin de cycle à la borne)")
	}
	if synced {
		t.Fatal("parc > borne absolue : synced=false attendu — AUCUNE déduction (comportement trunc honnête)")
	}
	if !db.HotspotUsers[0].MissingOnRouter {
		t.Fatal("parc > borne : le badge d'alice doit rester en l'état (aucune déduction)")
	}
}
