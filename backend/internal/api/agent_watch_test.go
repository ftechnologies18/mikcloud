package api

// Tests N°77 — veilleur d'invités + priorité des actionnables dans le batch.
//
// Constat production : le claim gratuit du portail est passé de ~45 s à 1-2 min
// après le déploiement N°75/N°76. Deux causes empilées :
//   - la veille N°75 : un routeur endormi (180 s) ne sert le claim du portail
//     qu'à son prochain check-in — le marqueur d'attention ne peut pas
//     RÉVEILLER un routeur qui n'appelle pas ; moyenne ~90 s, pire cas 180 s ;
//   - les chunks read_state N°76 : un parc de 3 500 users enfile jusqu'à 10
//     chunks d'un coup, TOUJOURS plus anciens qu'un claim fraîchement posé —
//     le FIFO pur leur donnait les 10 slots et le claim attendait un
//     check-in DE PLUS puis s'exécutait derrière ~30 s de lectures.
//
// Correctifs couverts ici :
//   - watcher_ensure : le scheduler mikcloud-watch (check-in 20 s pendant
//     qu'un hôte non autorisé est présent — un invité est SUR le portail) est
//     déployé automatiquement au premier check-in, le drapeau WatcherOK n'est
//     posé qu'au retour « ok » (pattern walled-garden), jamais re-filé après ;
//   - priorité du batch : les commandes actionnables précèdent les chunks
//     read_state dans le script servi (le claim s'exécute EN PREMIER), les
//     chunks ferment la marche avec walled_garden/hotspot_files.

import (
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"mikcloud/hotspot-api/internal/agent"
	"mikcloud/hotspot-api/internal/model"
)

// newWatchAPI — API minimale avec la carte d'attention initialisée.
func newWatchAPI() *API {
	return &API{attn: map[string]time.Time{}}
}

// watchRouter — routeur agent antérieur au N°77 (WatcherOK absent = faux).
func watchRouter() *model.Router {
	return &model.Router{ID: "r-watch", AccountID: "acc-watch", Name: "Watchy", Mode: "agent", Version: "7.20 (stable)", SchedulerSec: agentSleepSec}
}

// TestWatcherEnsureQueuedWhenMissing — un routeur sans veilleur confirmé
// reçoit watcher_ensure en file au check-in ; déjà en file/en vol → pas de
// doublon ; drapeau posé → plus jamais.
func TestWatcherEnsureQueuedWhenMissing(t *testing.T) {
	a := newWatchAPI()
	db := &model.DB{}
	router := watchRouter()

	// Antérieur au N°77 : WatcherOK faux → mise en file.
	a.ensureWatcherLocked(db, router)
	if findQueuedByKind(db, model.CmdWatcherEnsure) == nil {
		t.Fatal("routeur sans veilleur confirmé : watcher_ensure doit être en file")
	}

	// Déjà en file → pas de doublon.
	a.ensureWatcherLocked(db, router)
	n := 0
	for _, c := range db.Commands {
		if c.Kind == model.CmdWatcherEnsure {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("watcher_ensure dupliqué : %d commandes, attendu 1", n)
	}

	// En vol (« sent ») → pas de doublon non plus.
	for i := range db.Commands {
		db.Commands[i].Status = "sent"
	}
	a.ensureWatcherLocked(db, router)
	for _, c := range db.Commands {
		if c.Kind == model.CmdWatcherEnsure && c.Status == "queued" {
			t.Fatal("déploiement en vol : aucun nouveau watcher_ensure ne doit être filé")
		}
	}

	// Drapeau posé → silence définitif.
	router.WatcherOK = true
	db.Commands = nil
	a.ensureWatcherLocked(db, router)
	if findQueuedByKind(db, model.CmdWatcherEnsure) != nil {
		t.Fatal("WatcherOK posé : aucun watcher_ensure ne doit être re-filé")
	}

	// Un routeur simulé n'est pas concerné.
	router.WatcherOK = false
	router.Mode = "simulated"
	a.ensureWatcherLocked(db, router)
	if findQueuedByKind(db, model.CmdWatcherEnsure) != nil {
		t.Fatal("routeur simulé : aucun watcher_ensure ne doit être filé")
	}
}

// TestWatcherEnsureScriptShape — le script servi déploie le scheduler
// mikcloud-watch : fichier PROPRE au veilleur (jamais le dst-path du
// scheduler principal), garde d'hôtes non autorisés avec exclusions
// (bypassed/blocked), check-in complet, rapport standard.
func TestWatcherEnsureScriptShape(t *testing.T) {
	b := agent.Builder{BaseURL: "https://cloud.exemple", Token: "wt0ken-abcdefghijklmnop"}
	script, err := b.ScriptFor(model.Command{ID: "c-w1", Kind: model.CmdWatcherEnsure, Payload: map[string]any{}})
	if err != nil {
		t.Fatalf("ScriptFor(watcher_ensure) : %v", err)
	}
	for _, want := range []string{
		`name="` + agent.WatcherName + `"`,
		`interval=` + fmt.Sprintf("%d", agent.WatcherIntervalSec) + `s`,
		`/ip hotspot host print count-only where !authorized && !bypassed && !blocked`,
		// Forme ÉCHAPPÉE : le on-event est un attribut quoté — le corps du
		// veilleur est ré-échappé par rosEscape (même double échappement que
		// buildSchedulerAdd, éprouvé en production) : dans le .rsc servi, le
		// dst-path interne apparaît comme dst-path=\"mikcloud-watch.rsc\".
		`dst-path=\"` + agent.WatcherFilename + `\"`,
		`/agent/cmd?token=wt0ken-abcdefghijklmnop`,
		`/agent/result?token=`,
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("script watcher_ensure doit contenir %q :\n%s", want, preview(script, 600))
		}
	}
	// Le veilleur ne doit JAMAIS écrire le fichier du scheduler principal :
	// deux check-ins concurrents ne peuvent pas s'écraser mutuellement le
	// fichier (l'import d'un fichier à moitié réécrit tue les commandes du
	// même check-in — perte silencieuse d'un claim).
	if strings.Contains(strings.Split(script, `/system scheduler add`)[1], agent.ScriptFilename) {
		t.Fatalf("le on-event du veilleur ne doit pas utiliser %s (fichier du scheduler principal)", agent.ScriptFilename)
	}
	// Remove-then-add idempotent : un re-déploiement converge au lieu d'échouer
	// sur « entry already exists ».
	if !strings.Contains(script, `/system scheduler remove [find name="`+agent.WatcherName+`"]`) {
		t.Fatal("watcher_ensure doit retirer le veilleur existant avant de le recréer (idempotence)")
	}
}

// TestInstallScriptDeploysWatcher — l'installation d'un NOUVEL agent pose le
// veilleur directement (l'invité du premier soir n'attend pas la convergence
// du premier check-in).
func TestInstallScriptDeploysWatcher(t *testing.T) {
	script := agent.InstallScript("https://cloud.exemple", "inst4ll-token-abc", "Restaurant")
	if !strings.Contains(script, `name="`+agent.WatcherName+`"`) {
		t.Fatalf("l'install doit créer le scheduler %s", agent.WatcherName)
	}
	if !strings.Contains(script, `!authorized && !bypassed && !blocked`) {
		t.Fatal("l'install doit poser la garde d'hôtes non autorisés du veilleur")
	}
}

// TestAgentCmdBatchPriority — le scénario ProMax : 7 chunks read_state PLUS
// ANCIENS qu'un claim voucher_batch fraîchement posé. Le claim doit être
// servi EN PREMIER dans le script du check-in (et non à un check-in suivant,
// derrière ~30 s de lectures).
func TestAgentCmdBatchPriority(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	_, accID, _ := registerAccount(t, ts, "gerant-batch-prio", "")

	const tok = "batch-prio-t0ken-abcdefgh"
	st.Lock()
	st.Data().Routers = append(st.Data().Routers, model.Router{
		ID: "r-batch", AccountID: accID, Name: "Batch Prio", Mode: "agent", Status: "online",
		Version: "7.20 (stable)", AgentTokenHash: agent.HashToken(tok), SchedulerSec: agentSleepSec,
		LastSeen: model.NowISO(),
	})
	// Horodatages RÉELLEMENT passés : le check-in auto-enfile des commandes
	// (watcher_ensure…) à l'instant présent — des graines dans le futur
	// les feraient précéder le claim au tri par CreatedAt (arrivé en vrai :
	// cycle 10:00, claim 10:00:59, check-in 10:03 → le claim est bien le plus
	// ancien des actionnables).
	base := time.Now().UTC().Add(-time.Hour)
	for i := 0; i < 7; i++ {
		st.Data().Commands = append(st.Data().Commands, model.Command{
			ID: fmt.Sprintf("c-chunk%d", i), RouterID: "r-batch", Kind: model.CmdReadState,
			Status: "queued", CreatedAt: base.Add(time.Duration(i) * time.Second).Format(time.RFC3339),
		})
	}
	st.Data().Commands = append(st.Data().Commands, model.Command{
		ID: "c-claim", RouterID: "r-batch", Kind: model.CmdVoucherBatch,
		Status: "queued", CreatedAt: base.Add(59 * time.Second).Format(time.RFC3339),
	})
	st.Unlock()

	body := agentCheckIn(t, ts, tok)

	// Ordre réel des commandes servies (commentaires d'audit du script).
	var order []string
	for _, line := range strings.Split(body, "\n") {
		if strings.Contains(line, "mikcloud cmd ") {
			parts := strings.Fields(strings.TrimSpace(line))
			// parts : # mikcloud cmd <id> <kind> — on collecte le KIND.
			if len(parts) >= 5 {
				order = append(order, parts[4])
			}
		}
	}
	if len(order) == 0 {
		t.Fatalf("aucune commande servie : %q", preview(body, 300))
	}
	if order[0] != model.CmdVoucherBatch {
		t.Fatalf("le claim doit être servi EN PREMIER, ordre = %v", order)
	}
	firstRead := -1
	for i, kind := range order {
		if kind == model.CmdReadState {
			firstRead = i
			break
		}
	}
	if firstRead == -1 {
		t.Fatalf("les chunks read_state doivent être servis derrière le claim, ordre = %v", order)
	}
	if firstRead < 1 {
		t.Fatalf("le claim (index 0) doit précéder les chunks (index %d)", firstRead)
	}
	if len(order) > 10 {
		t.Fatalf("bornes du batch respectées : %d commandes servies, max 10", len(order))
	}
}

// TestWatcherEndToEnd — flux complet au travers du serveur HTTP : le premier
// check-in d'un routeur antérieur au N°77 sert watcher_ensure, le rapport
// « ok » pose WatcherOK, et le check-in suivant ne re-file PLUS le
// déploiement (convergence, silence).
func TestWatcherEndToEnd(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	_, accID, _ := registerAccount(t, ts, "gerant-watch-e2e", "")

	const tok = "w4tch-t0ken-abcdefghijklmnop"
	st.Lock()
	st.Data().Routers = append(st.Data().Routers, model.Router{
		ID: "r-w-e2e", AccountID: accID, Name: "E2E Watch", Mode: "agent", Status: "online",
		Version: "7.20 (stable)", AgentTokenHash: agent.HashToken(tok), SchedulerSec: agentSleepSec,
		LastSeen: model.NowISO(),
	})
	st.Unlock()

	// 1) Premier check-in : le déploiement du veilleur est DANS le script.
	body := agentCheckIn(t, ts, tok)
	if !strings.Contains(body, `name="`+agent.WatcherName+`"`) {
		t.Fatalf("premier check-in : le script doit déployer le veilleur : %q", preview(body, 300))
	}
	cmdID := watchCmdID(t, body)

	// 2) Rapport ok → WatcherOK posé (vérité routeur uniquement).
	agentReport(t, ts, tok, cmdID, url.Values{"status": {"ok"}})
	st.Lock()
	got := false
	for i := range st.Data().Routers {
		if st.Data().Routers[i].ID == "r-w-e2e" {
			got = st.Data().Routers[i].WatcherOK
		}
	}
	st.Unlock()
	if !got {
		t.Fatal("après rapport ok : Router.WatcherOK doit être vrai")
	}

	// 3) Check-in suivant : silence — plus aucun watcher_ensure servi.
	body2 := agentCheckIn(t, ts, tok)
	if strings.Contains(body2, model.CmdWatcherEnsure) {
		t.Fatalf("veilleur déjà confirmé : aucun nouveau watcher_ensure ne doit être servi : %q", preview(body2, 300))
	}
}

// watchCmdID — relit l'ID de la commande watcher_ensure depuis le commentaire
// d'audit du script (« # mikcloud cmd {id} watcher_ensure »).
func watchCmdID(t *testing.T, script string) string {
	t.Helper()
	for _, line := range strings.Split(script, "\n") {
		if strings.Contains(line, "mikcloud cmd ") && strings.Contains(line, model.CmdWatcherEnsure) {
			parts := strings.Fields(strings.TrimSpace(line))
			if len(parts) >= 4 {
				return parts[3]
			}
		}
	}
	t.Fatal("aucune commande watcher_ensure dans le script du check-in")
	return ""
}
