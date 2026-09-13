package api

// Tests N°80 — SafeWiFi : convergence au check-in (ensure), signature
// vérifiée au retour (vérité routeur), silence des routeurs qui n'ont
// jamais ouvert la carte, et priorité dans le batch du check-in.

import (
	"strings"
	"testing"
	"time"

	"mikcloud/hotspot-api/internal/agent"
	"mikcloud/hotspot-api/internal/model"
)

// safeWifiRouter — routeur agent antérieur au N°80 (aucune colonne posée).
func safeWifiRouter() *model.Router {
	return &model.Router{ID: "r-sw", AccountID: "acc-sw", Name: "SafeWifiy", Mode: "agent", Version: "7.20 (stable)"}
}

// TestSafeWifiNeverUsedStaysSilent — le contrat d'économie : un routeur dont
// le gérant n'a JAMAIS ouvert la carte (niveau off implicite, sig vide) ne
// reçoit AUCUNE commande — l'économie de veille N°75 reste entière, y
// compris pour tout le parc existant après le déploiement du N°80.
func TestSafeWifiNeverUsedStaysSilent(t *testing.T) {
	a := newWatchAPI()
	db := &model.DB{}
	router := safeWifiRouter()

	a.ensureSafeWifiLocked(db, router)
	if len(db.Commands) != 0 {
		t.Fatalf("routeur n'ayant jamais ouvert la carte : %d commande(s) filée(s), attendu 0", len(db.Commands))
	}

	// Simulé : pas concerné non plus.
	router.Mode = "simulated"
	router.SafeWifiLevel = model.SafeWifiFamily
	a.ensureSafeWifiLocked(db, router)
	if len(db.Commands) != 0 {
		t.Fatal("routeur simulé : aucune commande safewifi attendue")
	}
	router.Mode = "agent"
	router.SafeWifiLevel = ""
}

// TestSafeWifiEnsureConvergence — cycle complet : choix d'un niveau → file ;
// en vol → pas de doublon ; retour ok (sig + horodatage frais) → silence ;
// changement de niveau → re-file ; horodatage ancien → auto-réparation.
func TestSafeWifiEnsureConvergence(t *testing.T) {
	a := newWatchAPI()
	db := &model.DB{}
	router := safeWifiRouter()

	// Le gérant active « menaces » : la commande rejoint la file avec le
	// niveau et la signature attendue.
	router.SafeWifiLevel = model.SafeWifiThreats
	a.ensureSafeWifiLocked(db, router)
	cmd := findQueuedByKind(db, model.CmdSafeWifi)
	if cmd == nil {
		t.Fatal("changement de niveau : safewifi doit être en file")
	}
	if l, _ := cmd.Payload["level"].(string); l != model.SafeWifiThreats {
		t.Fatalf("payload level = %q, attendu %q", l, model.SafeWifiThreats)
	}
	if s, _ := cmd.Payload["sig"].(string); s != safeWifiSig(model.SafeWifiThreats) {
		t.Fatalf("payload sig = %q, attendu %q", s, safeWifiSig(model.SafeWifiThreats))
	}

	// En vol → pas de doublon.
	for i := range db.Commands {
		db.Commands[i].Status = "sent"
	}
	a.ensureSafeWifiLocked(db, router)
	n := 0
	for _, c := range db.Commands {
		if c.Kind == model.CmdSafeWifi {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("commande en vol dupliquée : %d, attendu 1", n)
	}

	// Retour ok simulé (handleAgentResult pose sig + horodatage) : silence.
	db.Commands[0].Status = "done"
	router.SafeWifiSig = safeWifiSig(model.SafeWifiThreats)
	router.SafeWifiAppliedAt = time.Now().Format(time.RFC3339)
	a.ensureSafeWifiLocked(db, router)
	if len(db.Commands) != 1 {
		t.Fatal("re-file injustifiée avec sig fraîche")
	}

	// Le gérant passe à « famille » : re-file automatique avec le NOUVEAU
	// niveau (la sig diffère).
	router.SafeWifiLevel = model.SafeWifiFamily
	a.ensureSafeWifiLocked(db, router)
	if len(db.Commands) != 2 {
		t.Fatalf("changement de niveau non détecté : %d commande(s), attendu 2", len(db.Commands))
	}
	if l, _ := db.Commands[1].Payload["level"].(string); l != model.SafeWifiFamily {
		t.Fatalf("re-file avec level = %q, attendu %q", l, model.SafeWifiFamily)
	}

	// Retour ok simulé, puis horodatage ANCIEN (> 6 h) : auto-réparation
	// périodique (une règle effacée localement est recréée).
	db.Commands[1].Status = "done"
	router.SafeWifiSig = safeWifiSig(model.SafeWifiFamily)
	router.SafeWifiAppliedAt = time.Now().Add(-safeWifiRefresh - time.Minute).Format(time.RFC3339)
	a.ensureSafeWifiLocked(db, router)
	if len(db.Commands) != 3 {
		t.Fatalf("réparation périodique non déclenchée : %d commande(s), attendu 3", len(db.Commands))
	}

	// Échec simulé (sig jamais posée) : retenté au check-in suivant.
	db.Commands[2].Status = "error"
	router.SafeWifiAppliedAt = time.Now().Format(time.RFC3339) // frais mais sig absente
	router.SafeWifiSig = ""
	a.ensureSafeWifiLocked(db, router)
	if len(db.Commands) != 4 {
		t.Fatalf("échec non retenté : %d commande(s), attendu 4", len(db.Commands))
	}
}

// TestSafeWifiRetireApresExtinction — le gérant désactive : le niveau off
// avec une sig posée (le routeur A connu la protection) re-file la
// commande de RETRAIT — les règles ne restent pas orphelines sur le
// routeur. Puis, une fois le retrait confirmé (sig off posée), silence
// périodique uniquement.
func TestSafeWifiRetireApresExtinction(t *testing.T) {
	a := newWatchAPI()
	db := &model.DB{}
	router := safeWifiRouter()

	router.SafeWifiLevel = model.SafeWifiFamily
	router.SafeWifiSig = safeWifiSig(model.SafeWifiFamily)
	router.SafeWifiAppliedAt = time.Now().Format(time.RFC3339)

	// Extinction : off + sig family → mismatch → commande de retrait.
	router.SafeWifiLevel = model.SafeWifiOff
	a.ensureSafeWifiLocked(db, router)
	cmd := findQueuedByKind(db, model.CmdSafeWifi)
	if cmd == nil {
		t.Fatal("extinction : la commande de retrait doit être filée")
	}
	if l, _ := cmd.Payload["level"].(string); l != model.SafeWifiOff {
		t.Fatalf("retrait avec level = %q, attendu %q", l, model.SafeWifiOff)
	}

	// Retrait confirmé (sig off + frais) : la cadence périodique poursuit
	// (idempotent, ~400 o toutes les 6 h) — pas de spam au-delà.
	db.Commands[0].Status = "done"
	router.SafeWifiSig = safeWifiSig(model.SafeWifiOff)
	router.SafeWifiAppliedAt = time.Now().Format(time.RFC3339)
	a.ensureSafeWifiLocked(db, router)
	if len(db.Commands) != 1 {
		t.Fatal("sig off fraîche : re-file injustifiée")
	}
}

// TestSafeWifiResultVerified — la signature n'est posée que si le COMPTE de
// règles marquées rapporté par le routeur correspond au niveau attendu :
// 2 en filtrage actif, 0 sinon. Un compte divergent reste sans sig.
func TestSafeWifiResultVerified(t *testing.T) {
	sig := safeWifiSig(model.SafeWifiFamily)
	cmd := model.Command{ID: "c-sw", Kind: model.CmdSafeWifi,
		Payload: map[string]any{"level": model.SafeWifiFamily, "sig": sig}}

	if !safeWifiResultVerified(&cmd, model.SafeWifiFamily, "2") {
		t.Fatal("family + 2 règles rapportées : la vérification doit passer")
	}
	if safeWifiResultVerified(&cmd, model.SafeWifiFamily, "0") {
		t.Fatal("family + 0 règle rapportée : la vérification doit échouer")
	}
	if safeWifiResultVerified(&cmd, model.SafeWifiFamily, "1") {
		t.Fatal("compte divergent (1) : la vérification doit échouer")
	}
	offCmd := model.Command{ID: "c-sw", Kind: model.CmdSafeWifi,
		Payload: map[string]any{"level": model.SafeWifiOff, "sig": safeWifiSig(model.SafeWifiOff)}}
	if !safeWifiResultVerified(&offCmd, model.SafeWifiOff, "0") {
		t.Fatal("off + 0 règle rapportée : la vérification doit passer")
	}
	if safeWifiResultVerified(&offCmd, model.SafeWifiOff, "2") {
		t.Fatal("off + règles résiduelles : la vérification doit échouer")
	}
}

// safeWifiResultVerified — extrait de handleAgentResult (branche N°80) :
// vrai si le compte de règles marquées rapporté correspond au niveau de la
// commande ET si ce niveau est toujours le niveau courant du routeur.
func safeWifiResultVerified(cmd *model.Command, currentLevel, reported string) bool {
	level := agent.SafeWifiLevelFromPayload(cmd.Payload)
	if level != currentLevel {
		return false
	}
	want := 0
	if level != model.SafeWifiOff {
		want = 2
	}
	got, ok := parseReportInt(reported)
	return ok && got == want
}

// TestSafeWifiSigVersioned — le sel de version garantit qu'une évolution de
// la FORME des règles est re-poussée au parc entier (garde-fou N°48).
func TestSafeWifiSigVersioned(t *testing.T) {
	legacy := agent.HashToken(model.SafeWifiFamily)[:16] // formule sans sel
	if s := safeWifiSig(model.SafeWifiFamily); s == legacy {
		t.Fatal("safeWifiSig sans sel de version : un correctif de règles ne serait jamais re-poussé (régression N°48)")
	}
	if safeWifiSig(model.SafeWifiThreats) == safeWifiSig(model.SafeWifiFamily) {
		t.Fatal("des niveaux distincts doivent produire des signatures distinctes")
	}
	if s1, s2 := safeWifiSig(model.SafeWifiFamily), safeWifiSig(model.SafeWifiFamily); s1 != s2 {
		t.Fatal("safeWifiSig doit être déterministe")
	}
}

// TestSafeWifiBatchDeferredLast — la commande safewifi ferme la marche du
// batch (après walled_garden et hotspot_files) : sa réussite ne conditionne
// aucune autre commande, et une ligne avortée ne doit pas tuer ce qui suit
// (contrat N°77 du deferred bucket).
func TestSafeWifiBatchDeferredLast(t *testing.T) {
	db := &model.DB{Commands: []model.Command{
		{ID: "c-hf", RouterID: "r-sw", Kind: model.CmdHotspotFiles, Status: "queued", CreatedAt: "2026-09-12T00:00:00Z"},
		{ID: "c-sw", RouterID: "r-sw", Kind: model.CmdSafeWifi, Status: "queued", CreatedAt: "2026-09-12T00:00:01Z"},
		{ID: "c-wg", RouterID: "r-sw", Kind: model.CmdWalledGarden, Status: "queued", CreatedAt: "2026-09-12T00:00:02Z"},
	}}
	router := safeWifiRouter()

	// Reproduit le tri du deferred bucket du check-in.
	deferred := []model.Command{}
	for i := range db.Commands {
		if db.Commands[i].RouterID == router.ID && db.Commands[i].Status == "queued" {
			switch db.Commands[i].Kind {
			case model.CmdWalledGarden, model.CmdHotspotFiles, model.CmdSafeWifi:
				deferred = append(deferred, db.Commands[i])
			}
		}
	}
	deferredWave := func(k string) int {
		switch k {
		case model.CmdWalledGarden:
			return 29
		case model.CmdHotspotFiles:
			return 35
		}
		return 80
	}
	// Tri du deferred bucket : la vague domine (29 < 35 < 80).
	bubbleSortByWave(deferred, deferredWave)

	if len(deferred) != 3 {
		t.Fatalf("deferred = %d, attendu 3", len(deferred))
	}
	if deferred[0].Kind != model.CmdWalledGarden || deferred[1].Kind != model.CmdHotspotFiles || deferred[2].Kind != model.CmdSafeWifi {
		kinds := make([]string, 3)
		for i, c := range deferred {
			kinds[i] = c.Kind
		}
		t.Fatalf("ordre du deferred bucket = %v, attendu [walled_garden hotspot_files safewifi]", strings.Join(kinds, " "))
	}
}

// bubbleSortByWave — tri stable minimal par vague pour le test (le code
// production utilise sort.SliceStable).
func bubbleSortByWave(cmds []model.Command, wave func(string) int) {
	for i := 0; i < len(cmds); i++ {
		for j := i + 1; j < len(cmds); j++ {
			if wave(cmds[j].Kind) < wave(cmds[i].Kind) {
				cmds[i], cmds[j] = cmds[j], cmds[i]
			}
		}
	}
}
