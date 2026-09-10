package api

// Tests N°74 — télémétrie cadencée : la boucle read_state (un snapshot
// complet O(n) servi à CHAQUE check-in 45 s, 24 h/24, ~75 % du trafic agents)
// est désormais bornée par readStateMinInterval via ensureReadStateDue.

import (
	"testing"
	"time"

	"mikcloud/hotspot-api/internal/model"
)

func newThrottleAPI(t *testing.T) *API {
	t.Helper()
	a := &API{readStateDone: map[string]time.Time{}}
	return a
}

// TestEnsureReadStateDueQueuesWhenNeverRun — premier check-in après le boot :
// aucun read_state appliqué → il est enfilé immédiatement (la télémétrie
// démarre vite, la cadence ne s'applique qu'aux REPRISES).
func TestEnsureReadStateDueQueuesWhenNeverRun(t *testing.T) {
	a := newThrottleAPI(t)
	db := &model.DB{}
	router := &model.Router{ID: "r1", AccountID: "acc1"}

	a.ensureReadStateDue(db, router)

	if len(db.Commands) != 1 || db.Commands[0].Kind != model.CmdReadState {
		t.Fatalf("read_state attendu en file après le boot, %d commande(s) obtenue(s)", len(db.Commands))
	}
}

// TestEnsureReadStateDueSkipsWhenFresh — un read_state appliqué il y a moins
// de readStateMinInterval : le check-in N'ENFILE PAS de nouveau (l'ancienne
// boucle re-enfilait à chaque résultat — 1 920 snapshots/jour/routeur).
func TestEnsureReadStateDueSkipsWhenFresh(t *testing.T) {
	a := newThrottleAPI(t)
	db := &model.DB{}
	router := &model.Router{ID: "r1", AccountID: "acc1"}
	a.readStateDone[router.ID] = time.Now().UTC().Add(-30 * time.Second) // frais

	a.ensureReadStateDue(db, router)

	if len(db.Commands) != 0 {
		t.Fatalf("aucune commande attendue (read_state frais), %d obtenue(s)", len(db.Commands))
	}
}

// TestEnsureReadStateDueQueuesWhenStale — le dernier read_state appliqué date
// de plus de l'intervalle : le check-in en re-file un.
func TestEnsureReadStateDueQueuesWhenStale(t *testing.T) {
	a := newThrottleAPI(t)
	db := &model.DB{}
	router := &model.Router{ID: "r1", AccountID: "acc1"}
	a.readStateDone[router.ID] = time.Now().UTC().Add(-readStateMinInterval - time.Second) // périmé

	a.ensureReadStateDue(db, router)

	if len(db.Commands) != 1 || db.Commands[0].Kind != model.CmdReadState {
		t.Fatalf("read_state attendu (intervalle écoulé), %d commande(s) obtenue(s)", len(db.Commands))
	}
}

// TestEnsureReadStateDueSkipsWhenAlreadyQueuedOrSent — un read_state déjà en
// file (ou en vol, statut « sent ») bloque tout re-enfilement, même périmé :
// pas de doublon dans la FIFO, pas de double snapshot en vol.
func TestEnsureReadStateDueSkipsWhenAlreadyQueuedOrSent(t *testing.T) {
	for _, status := range []string{"queued", "sent"} {
		a := newThrottleAPI(t)
		db := &model.DB{Commands: []model.Command{
			{ID: "c-deja", RouterID: "r1", Kind: model.CmdReadState, Status: status, CreatedAt: "2026-01-01T00:00:00Z"},
		}}
		router := &model.Router{ID: "r1", AccountID: "acc1"}
		a.readStateDone[router.ID] = time.Now().UTC().Add(-readStateMinInterval - time.Minute) // périmé

		a.ensureReadStateDue(db, router)

		if len(db.Commands) != 1 {
			t.Fatalf("statut %q : aucun re-enfilement attendu, %d commande(s) au total", status, len(db.Commands))
		}
	}
}

// TestEnsureReadStateDueIgnoresOtherRouters — la cadence est PAR routeur :
// le read_state en file d'un AUTRE routeur ne bloque pas celui-ci.
func TestEnsureReadStateDueIgnoresOtherRouters(t *testing.T) {
	a := newThrottleAPI(t)
	db := &model.DB{Commands: []model.Command{
		{ID: "c-autre", RouterID: "r2", Kind: model.CmdReadState, Status: "queued", CreatedAt: "2026-01-01T00:00:00Z"},
	}}
	router := &model.Router{ID: "r1", AccountID: "acc1"}

	a.ensureReadStateDue(db, router)

	found := false
	for _, c := range db.Commands {
		if c.RouterID == "r1" && c.Kind == model.CmdReadState {
			found = true
		}
	}
	if !found {
		t.Fatal("le read_state de r1 doit être enfilé malgré celui de r2")
	}
}
