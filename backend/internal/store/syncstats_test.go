// Tests N°71 — instrumentation santé de la synchro (syncstats.go).
// Mode JSON uniquement, comme le reste du package (aucune connexion
// PostgreSQL en test) : les compteurs sont de la logique pure, testables
// sans base ; le chemin PostgreSQL (Sync → tx) reste vérifié par le build,
// go vet et la production (l'endpoint est read-only, sans risque).
package store

import (
	"errors"
	"testing"
	"time"

	"mikcloud/hotspot-api/internal/model"
)

// TestSyncStatsRecordAndSnapshot — success/failure alternés : compteurs,
// chaîne d'échecs consécutifs (remise à zéro au succès), volumétrie du
// dernier delta RÉUSSI, mémorisation de la dernière erreur.
func TestSyncStatsRecordAndSnapshot(t *testing.T) {
	var s syncStats

	s.recordSuccess(syncDelta{changed: 4, removed: 2}, 3*time.Millisecond)
	s.recordFailure(errors.New("néon injoignable"), time.Millisecond)
	s.recordFailure(errors.New("commit refusé"), time.Millisecond)

	snap := s.snapshot()
	if snap.Attempts != 3 || snap.Successes != 1 || snap.Failures != 2 {
		t.Fatalf("compteurs inattendus : %+v", snap)
	}
	if snap.ConsecutiveFails != 2 {
		t.Fatalf("chaîne d'échecs attendue 2, obtenue %d", snap.ConsecutiveFails)
	}
	if snap.LastError != "commit refusé" {
		t.Fatalf("dernière erreur attendue « commit refusé », obtenue %q", snap.LastError)
	}
	if snap.LastErrorAt == "" || snap.LastSuccessAt == "" {
		t.Fatalf("horodatages attendus : %+v", snap)
	}
	if snap.LastChangedRows != 4 || snap.LastRemovedRows != 2 {
		t.Fatalf("volumétrie du dernier delta attendue 4/2, obtenue %d/%d", snap.LastChangedRows, snap.LastRemovedRows)
	}

	// Un succès remet la chaîne d'échecs à zéro et remplace la volumétrie.
	s.recordSuccess(syncDelta{changed: 7, removed: 0}, 2*time.Millisecond)
	snap = s.snapshot()
	if snap.Attempts != 4 || snap.Successes != 2 || snap.ConsecutiveFails != 0 {
		t.Fatalf("après retour au nominal : %+v", snap)
	}
	if snap.LastChangedRows != 7 || snap.LastRemovedRows != 0 {
		t.Fatalf("nouveau delta attendu 7/0, obtenu %d/%d", snap.LastChangedRows, snap.LastRemovedRows)
	}

	// recordFailure(nil) est un no-op défensif.
	s.recordFailure(nil, time.Millisecond)
	if s.snapshot().Attempts != 4 {
		t.Fatal("recordFailure(nil) ne doit rien compter")
	}
}

// TestSyncStatsErrorBorne — une erreur géante (requête SQL entière) est
// tronquée pour ne pas gonfler l'état ni la réponse JSON.
func TestSyncStatsErrorBorne(t *testing.T) {
	var s syncStats
	giant := make([]byte, maxSyncErrorLen+50)
	for i := range giant {
		giant[i] = 'x'
	}
	s.recordFailure(errors.New(string(giant)), time.Millisecond)
	snap := s.snapshot()
	if len(snap.LastError) > maxSyncErrorLen+3 { // tronqué + « … »
		t.Fatalf("erreur non bornée : %d caractères", len(snap.LastError))
	}
	if snap.LastError == string(giant) {
		t.Fatal("l'erreur géante doit être tronquée")
	}
}

// TestSyncHealthJSONMode — en mode JSON (développement), la photographie
// annonce le mode, n'expose NI compteurs NI bloc Neon, et liste les tables
// avec les lignes mémoire uniquement.
func TestSyncHealthJSONMode(t *testing.T) {
	st := newJSONStore(t)

	h := st.SyncHealth()
	if h.Mode != "json" {
		t.Fatalf("mode attendu json, obtenu %q", h.Mode)
	}
	if h.Sync != nil || h.Neon != nil {
		t.Fatalf("en mode JSON, sync et neon doivent être absents : %+v %+v", h.Sync, h.Neon)
	}
	if len(h.Tables) != 30 {
		t.Fatalf("30 tables attendues (29 différentielles + settings), obtenues %d", len(h.Tables))
	}

	// Une mutation mémoire apparaît dans les lignes, sans réplique (mirrored
	// omis) ; geniuspay_subs est désormais dans la liste (écart N°71).
	st.Lock()
	st.Data().Routers = append(st.Data().Routers, model.Router{ID: "rt-1", Mode: "agent"})
	st.Data().GeniusPaySubs = append(st.Data().GeniusPaySubs, model.GeniusPaySub{UUID: "gpx-1"})
	st.Save()
	st.Unlock()

	h = st.SyncHealth()
	byTable := map[string]int{}
	for _, th := range h.Tables {
		byTable[th.Table] = th.Rows
		if th.Mirrored != 0 {
			t.Fatalf("mode JSON : mirrored doit rester à zéro, %s = %d", th.Table, th.Mirrored)
		}
	}
	if byTable["routers"] != 1 || byTable["geniuspay_subs"] != 1 {
		t.Fatalf("routers=1 et geniuspay_subs=1 attendus, obtenus %v", byTable)
	}
}

// TestLiveTableRowsConcordance — la liste suit les tables de la synchro
// différentielle : 30 entrées, noms uniques, aucune table en double (une
// table ajoutée à Sync sans être ajoutée ici doit faire échouer le compte).
func TestLiveTableRowsConcordance(t *testing.T) {
	rows := liveTableRows(BuildEmptyState())
	if len(rows) != 30 {
		t.Fatalf("30 entrées attendues (29 différentielles + settings), obtenues %d", len(rows))
	}
	seen := map[string]bool{}
	for _, th := range rows {
		if th.Table == "" {
			t.Fatal("nom de table vide")
		}
		if seen[th.Table] {
			t.Fatalf("table en double : %s", th.Table)
		}
		seen[th.Table] = true
		if th.Rows < 0 {
			t.Fatalf("compteur négatif pour %s : %d", th.Table, th.Rows)
		}
	}
	// Les tables attendues (miroir de pg.go — Sync + rebuildHashes + settings).
	for _, want := range []string{"accounts", "admin_users", "routers", "commands", "geniuspay_subs", "settings"} {
		if !seen[want] {
			t.Fatalf("table manquante dans la liste : %s", want)
		}
	}
}
