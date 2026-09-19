package api

// Tests N°159 — backoff des watchers en échec répété (le moteur de volume,
// volet re-files). Production 19/09 : shield ×808/12 h (ProMax), safewifi
// ×107 (ProMax) et ×112 (CYBER), queue_ensure « done » non vérifiés ×967
// (Benie) — chaque échec re-filait au check-in suivant sans AUCUNE chance
// de converger (l'état routeur ne change pas en 20 s) et nourrissait le
// ping-pong de fraîcheur post-écriture.

import (
	"testing"
	"time"

	"mikcloud/hotspot-api/internal/model"
)

// TestWatcherBackoffDelay — les paliers : 1 min, 5 min, 15 min puis plafond
// 30 min — et zéro tant qu'aucun échec.
func TestWatcherBackoffDelay(t *testing.T) {
	cas := []struct {
		fails int
		want  time.Duration
	}{
		{0, 0},
		{1, time.Minute},
		{2, 5 * time.Minute},
		{3, 15 * time.Minute},
		{4, 30 * time.Minute},
		{9, 30 * time.Minute},
	}
	for _, c := range cas {
		if got := watcherBackoffDelay(c.fails); got != c.want {
			t.Errorf("watcherBackoffDelay(%d) = %v, attendu %v", c.fails, got, c.want)
		}
	}
}

// TestWatcherBackoffBlocksThenAllows — un échec bloque le re-file pendant le
// palier, l'expiration le permet ; un « ok » réinitialise immédiatement.
func TestWatcherBackoffBlocksThenAllows(t *testing.T) {
	a := &API{watcherFailN: map[string]int{}, watcherFailAt: map[string]time.Time{}}
	now := time.Now().UTC()

	a.recordWatcherError("r-1", model.CmdShield, now)
	if !a.watcherBackoffBlocks("r-1", model.CmdShield, now.Add(30*time.Second)) {
		t.Fatal("30 s après 1 échec : le re-file doit être bloqué (palier 1 min)")
	}
	if a.watcherBackoffBlocks("r-1", model.CmdShield, now.Add(61*time.Second)) {
		t.Fatal("61 s après 1 échec : le re-file doit être permis (palier expiré)")
	}

	// Paliers successifs : 2 échecs → 5 min.
	a.recordWatcherError("r-1", model.CmdShield, now.Add(2*time.Minute))
	if !a.watcherBackoffBlocks("r-1", model.CmdShield, now.Add(2*time.Minute+3*time.Minute)) {
		t.Fatal("2 échecs : bloqué à +3 min (palier 5 min)")
	}
	if a.watcherBackoffBlocks("r-1", model.CmdShield, now.Add(2*time.Minute+6*time.Minute)) {
		t.Fatal("2 échecs : permis à +6 min (palier 5 min expiré)")
	}

	// « ok » : réactivité immédiate (contrat de convergence inchangé).
	a.resetWatcherBackoff("r-1", model.CmdShield)
	if a.watcherBackoffBlocks("r-1", model.CmdShield, now) {
		t.Fatal("après un ok : plus aucun blocage")
	}
	if a.watcherFailN[watcherKey("r-1", model.CmdShield)] != 0 {
		t.Fatal("après un ok : le compteur doit être réinitialisé")
	}

	// Isolation par kind : un shield en échec ne ralentit pas un queue_ensure.
	a.recordWatcherError("r-1", model.CmdShield, now)
	if a.watcherBackoffBlocks("r-1", model.CmdQueueEnsure, now) {
		t.Fatal("le backoff est PAR KIND : queue_ensure ne doit pas être bloqué par un échec shield")
	}
	// Isolation par routeur.
	if a.watcherBackoffBlocks("r-2", model.CmdShield, now) {
		t.Fatal("le backoff est PAR ROUTEUR : r-2 ne doit pas être bloqué par un échec de r-1")
	}
}

// TestWatcherBackoffNilStateSafe — une API sans état de backoff (premier
// boot, tests) ne bloque jamais : comportement historique préservé.
func TestWatcherBackoffNilStateSafe(t *testing.T) {
	a := &API{}
	if a.watcherBackoffBlocks("r-x", model.CmdShield, time.Now().UTC()) {
		t.Fatal("état nil : jamais de blocage")
	}
}

// TestEnsureShieldBackoffIntegration — le convergeur du check-in respecte le
// backoff : un bouclier en échec récent ne re-file PAS, l'expiration ou le
// retour « ok » rétablit la convergence (sig non fraîche → re-file).
func TestEnsureShieldBackoffIntegration(t *testing.T) {
	a := &API{watcherFailN: map[string]int{}, watcherFailAt: map[string]time.Time{}}
	db := &model.DB{}
	router := &model.Router{ID: "r-sh", AccountID: "acc", Mode: "agent",
		ShieldLevel: model.ShieldOn, ShieldSig: "vielli", ShieldAppliedAt: "2020-01-01T00:00:00Z"}

	now := time.Now().UTC()
	a.recordWatcherError("r-sh", model.CmdShield, now)

	a.ensureShieldLocked(db, router)
	if len(db.Commands) != 0 {
		t.Fatal("échec tout récent : le re-file doit être bloqué (palier 1 min)")
	}

	a.watcherFailAt[watcherKey("r-sh", model.CmdShield)] = now.Add(-2 * time.Minute)
	a.ensureShieldLocked(db, router)
	if len(db.Commands) != 1 || db.Commands[0].Kind != model.CmdShield {
		t.Fatal("palier expiré : le re-file doit reprendre (convergence préservée)")
	}
}

// TestQosEnsureVerifiedMultiTarget — N°159 — LA preuve du correctif : le
// rapport de relecture tel que le NOUVEAU script l'émet (cible rejointe à
// VIRGULES) vérifie une file multi-cibles — le rapport historique à
// POINTS-VIRGULES scindait la ligne et la signature n'était JAMAIS posée
// (Benie wifi / ProMax WIFI, production 19/09 : re-file perpétuel).
func TestQosEnsureVerifiedMultiTarget(t *testing.T) {
	cmd := model.Command{
		ID:     "c-qe",
		Kind:   model.CmdQueueEnsure,
		Status: "done",
		Payload: map[string]any{
			"target":     "11.11.11.0/24,10.77.0.0/21",
			"sig":        "2278d1525802f999",
			"maxUpBps":   int64(14_000_000),
			"maxDownBps": int64(238_000_000),
		},
	}
	// Rapport NOUVEAU (rosQueueTargetCSV) : cible à virgules, débits
	// formatés RouterOS (« 14M/238M ») — la forme exacte observée en
	// production, seul le séparateur de cible change.
	dataNew := "queue|mikcloud-qos|11.11.11.0/24,10.77.0.0/21|14M/238M|pcq-upload-default/pcq-download-default|false;"
	if !qosEnsureVerified(&cmd, dataNew) {
		t.Fatal("rapport multi-cibles normalisé : la vérification DOIT passer (signature posée, fin du re-file)")
	}

	// Ordre de relecture différent (vérité routeur, N°110 — comparaison en
	// ENSEMBLE) : toujours conforme.
	dataReordered := "queue|mikcloud-qos|10.77.0.0/21,11.11.11.0/24|14M/238M|pcq-upload-default/pcq-download-default|false;"
	if !qosEnsureVerified(&cmd, dataReordered) {
		t.Fatal("cible relue dans un autre ordre : la couverture est identique (N°110), la vérification doit passer")
	}

	// Le rapport HISTORIQUE (points-virgules dans la cible) reste
	// invérifiable — scindé en fragments par splitAgentList : ces commandes
	// en vol au déploiement échoueront une dernière fois leur vérification,
	// la suivante partira du nouveau script.
	dataLegacy := "queue|mikcloud-qos|11.11.11.0/24;10.77.0.0/21|14M/238M|pcq-upload-default/pcq-download-default|false;"
	if qosEnsureVerified(&cmd, dataLegacy) {
		t.Fatal("rapport historique scindé : ne doit PAS être accepté (fragments malformés)")
	}

	// Une limite qui diverge ne vérifie toujours pas (garde N°104 intacte).
	dataDivergent := "queue|mikcloud-qos|11.11.11.0/24,10.77.0.0/21|10M/100M|pcq-upload-default/pcq-download-default|false;"
	if qosEnsureVerified(&cmd, dataDivergent) {
		t.Fatal("limites divergentes : la vérification doit échouer (garde N°104)")
	}
}
