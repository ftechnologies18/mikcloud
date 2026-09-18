// tables_test.go — N°133 — P1 audit performance : tests du marquage ciblé.
//
// Deux garde-fous :
//   - TestTableSet : le type TableSet (Mark/Has/Empty/Names) est la brique
//     du contrat SaveTables — nil-sûr, filtrant, déterministe ;
//   - TestSyncKnownTablesConcordance : les TROIS listes de tables du dépôt
//     (constantes de tables.go, specs du plan de synchro, volumétrie santé)
//     désignent exactement les MÊMES tables. Une table ajoutée à la synchro
//     sans constante (ou l'inverse) casse le marquage ciblé silencieusement :
//     le test l'impose avant la CI.
//   - TestTickMarksTouchedTables : le moteur marque ce qu'il mutate — un
//     parc agent ne marque QUE routers (l'invariant perf de l'audit), un
//     simulateur marque traffic/line_quality/sessions/… au fil de ses events.
package store

import (
	"strings"
	"testing"
	"time"

	"mikcloud/hotspot-api/internal/model"
)

func TestTableSet(t *testing.T) {
	// Nil-sûreté : un récepteur nil absorbe tout (Sweep, tests, chemins
	// d'écriture qui ne marquent pas).
	var nilSet *TableSet
	nilSet.Mark(TableRouters)
	if nilSet.Has(TableRouters) {
		t.Fatal("un TableSet nil ne doit rien contenir")
	}
	if !nilSet.Empty() {
		t.Fatal("un TableSet nil doit être vide")
	}
	if len(nilSet.Names()) != 0 {
		t.Fatal("un TableSet nil ne doit lister aucune table")
	}

	// Marquage, idempotence, filtrage des inconnus.
	ts := NewTableSet(TableRouters, TableTraffic, "table_inexistante", "")
	if ts.Empty() {
		t.Fatal("l'ensemble doit contenir routers et traffic")
	}
	if !ts.Has(TableRouters) || !ts.Has(TableTraffic) {
		t.Fatal("routers et traffic doivent être marqués")
	}
	if ts.Has(TableHotspotUsers) {
		t.Fatal("hotspot_users n'a pas été marqué")
	}
	ts.Mark(TableRouters) // idempotent
	if got := ts.Names(); len(got) != 2 || got[0] != TableRouters || got[1] != TableTraffic {
		t.Fatalf("noms triés attendus [routers traffic], obtenus %v", got)
	}
}

// TestSyncKnownTablesConcordance — constantes ↔ plan de synchro ↔ santé :
// toute table différentielle de la synchro possède une constante (donc est
// marquable), et la volumétrie couvre exactement les mêmes tables + settings.
func TestSyncKnownTablesConcordance(t *testing.T) {
	db := BuildEmptyState()

	// 1. Le plan de synchro (specs) : 34 tables différentielles, uniques
	// (N°152 : + announcements).
	steps := syncSteps(db)
	if len(steps) != 34 {
		t.Fatalf("34 tables différentielles attendues dans le plan de synchro, obtenues %d", len(steps))
	}
	inPlan := map[string]bool{}
	for _, st := range steps {
		if st.name == "" {
			t.Fatal("step de synchro sans nom de table")
		}
		if inPlan[st.name] {
			t.Fatalf("table en double dans le plan de synchro : %s", st.name)
		}
		inPlan[st.name] = true
	}

	// 2. Le registre (SaveTables) couvre le plan + settings : chaque table
	// marquable correspond à une table réellement synchronisée.
	if len(syncKnownTables) != len(inPlan)+1 {
		t.Fatalf("le registre doit compter plan+settings = %d tables, en compte %d", len(inPlan)+1, len(syncKnownTables))
	}
	for name := range syncKnownTables {
		if name != TableSettings && !inPlan[name] {
			t.Fatalf("table %q enregistrée dans syncKnownTables mais absente du plan de synchro", name)
		}
	}
	for name := range inPlan {
		if !syncKnownTables[name] {
			t.Fatalf("table %q synchronisée mais sans constante marquable (tables.go)", name)
		}
	}

	// 3. La volumétrie santé suit les mêmes tables (+ settings).
	rows := liveTableRows(db)
	if len(rows) != len(syncKnownTables) {
		t.Fatalf("liveTableRows doit aligner les %d tables du registre, en liste %d", len(syncKnownTables), len(rows))
	}
	for _, th := range rows {
		if !syncKnownTables[th.Table] {
			t.Fatalf("liveTableRows liste %q hors registre de la synchro", th.Table)
		}
	}
}

// TestTickMarksTouchedTables — l'invariant perf de l'audit N°133 : en
// croisière d'un parc AGENT (aucun routeur simulé), un Tick de lecture ne
// marque QUE routers — ni les 3 000+ utilisateurs, ni les 5 000 journaux,
// ni les sessions. Un simulateur, lui, marque bien ses tables.
func TestTickMarksTouchedTables(t *testing.T) {
	now := time.Now().UTC()

	// Parc 100 % agent : 2 routeurs, un client réel vivant (session + compte).
	// LastTick daté (sinon la garde de 2 s avale le tick — même comportement
	// qu'une vraie console qui poll après une longue absence).
	agentDB := BuildEmptyState()
	agentDB.LastTick = now.Add(-1 * time.Hour)
	agentDB.Routers = []model.Router{
		{ID: "rt-a1", AccountID: "acc-main", Name: "Agent 1", Mode: "agent"},
		{ID: "rt-a2", AccountID: "acc-main", Name: "Agent 2", Mode: "agent"},
	}
	agentDB.HotspotUsers = []model.HotspotUser{
		{ID: "u-1", AccountID: "acc-main", Username: "client", Status: "active", RouterID: "rt-a1"},
	}
	agentDB.Sessions = []model.Session{
		{ID: "s-real", AccountID: "acc-main", Username: "client", RouterID: "rt-a1"},
	}
	touched := NewTableSet()
	Tick(agentDB, now, touched)
	if got := touched.Names(); len(got) != 1 || got[0] != TableRouters {
		t.Fatalf("parc agent : seuls routers doit être marqué (télémétrie), marqué %v", got)
	}

	// Garde 2 s : un second tick immédiat ne marque RIEN (rien n'a bougé).
	touched2 := NewTableSet()
	Tick(agentDB, now.Add(500*time.Millisecond), touched2)
	if !touched2.Empty() {
		t.Fatalf("tick sous la garde de 2 s ne doit rien marquer, marqué %v", touched2.Names())
	}

	// Parc simulé : un simulateur avec trafic marque au minimum routers et
	// traffic (les lignes progressent à chaque tick).
	simDB := BuildEmptyState()
	simDB.LastTick = now.Add(-1 * time.Hour)
	simDB.Routers = []model.Router{{ID: "rt-sim", AccountID: "acc-main", Name: "Démo", Mode: "simulated"}}
	touched3 := NewTableSet()
	Tick(simDB, now, touched3)
	if !touched3.Has(TableRouters) || !touched3.Has(TableTraffic) {
		t.Fatalf("simulateur : routers et traffic attendus, marqué %v", touched3.Names())
	}
}

// TestSaveTablesJSONMode — en mode JSON (développement/E2E), le ciblage est
// ignoré : SaveTables retombe sur l'écriture complète synchrone (le fichier
// est la seule vérité — pas de diff d'empreintes possible).
func TestSaveTablesJSONMode(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatalf("New : %v", err)
	}
	defer s.Close()
	if s.pg != nil {
		t.Skip("mode PostgreSQL actif — le test couvre le mode JSON")
	}
	s.Lock()
	s.Data().Routers = append(s.Data().Routers, model.Router{ID: "rt-x", AccountID: "acc-main", Mode: "agent"})
	s.SaveTables(TableRouters)
	s.Unlock()

	reloaded, err := New(dir)
	if err != nil {
		t.Fatalf("relecture : %v", err)
	}
	defer reloaded.Close()
	found := false
	for _, rr := range reloaded.Data().Routers {
		if rr.ID == "rt-x" {
			found = true
		}
	}
	if !found {
		t.Fatal("SaveTables en mode JSON doit persister (écriture complète)")
	}
}

// TestSyncSettingsAlwaysWrittenByTargetedPlan — garde statique : le littéral
// syncSettings doit rester appelé par syncPlan (le ciblage ne doit JAMAIS
// sauter settings — last_tick/last_sweep y vivent). Vérification source,
// même pattern que TestSyncSettingsSQLConsistency.
func TestSyncSettingsAlwaysWrittenByTargetedPlan(t *testing.T) {
	src, err := readSourceFile("pg_sync.go")
	if err != nil {
		t.Fatalf("lecture du source impossible : %v", err)
	}
	// Le moteur commun syncPlan doit appeler syncSettings APRÈS la boucle
	// des steps ciblés (aucun early-return entre les deux).
	planIdx := strings.Index(src, "func (p *PG) syncPlan(")
	if planIdx < 0 {
		t.Fatal("syncPlan introuvable dans pg_sync.go")
	}
	body := src[planIdx:]
	loopIdx := strings.Index(body, "for _, st := range syncSteps(db)")
	settingsIdx := strings.Index(body, "p.syncSettings(ctx, tx, db)")
	if loopIdx < 0 || settingsIdx < 0 || settingsIdx < loopIdx {
		t.Fatal("syncPlan doit appeler syncSettings après la boucle des steps ciblés")
	}
}
