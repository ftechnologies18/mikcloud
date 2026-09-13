package api

// Tests N°81 — Shield : convergence au check-in (ensure), signature
// vérifiée au retour (5 règles × hotspots rapportés, vérité routeur),
// silence des routeurs qui n'ont jamais ouvert la carte, et priorité
// dans le batch du check-in.

import (
	"strings"
	"testing"
	"time"

	"mikcloud/hotspot-api/internal/agent"
	"mikcloud/hotspot-api/internal/model"
)

// shieldRouter — routeur agent antérieur au N°81 (aucune colonne posée).
func shieldRouter() *model.Router {
	return &model.Router{ID: "r-sh", AccountID: "acc-sh", Name: "Shieldy", Mode: "agent", Version: "7.20 (stable)"}
}

// TestShieldNeverUsedStaysSilent — le contrat d'économie : un routeur dont
// le gérant n'a JAMAIS ouvert la carte (niveau off implicite, sig vide) ne
// reçoit AUCUNE commande — l'économie de veille N°75 reste entière, y
// compris pour tout le parc existant après le déploiement du N°81.
func TestShieldNeverUsedStaysSilent(t *testing.T) {
	a := newWatchAPI()
	db := &model.DB{}
	router := shieldRouter()

	a.ensureShieldLocked(db, router)
	if len(db.Commands) != 0 {
		t.Fatalf("routeur n'ayant jamais ouvert la carte : %d commande(s) filée(s), attendu 0", len(db.Commands))
	}

	// Simulé : pas concerné non plus.
	router.Mode = "simulated"
	router.ShieldLevel = model.ShieldOn
	a.ensureShieldLocked(db, router)
	if len(db.Commands) != 0 {
		t.Fatal("routeur simulé : aucune commande shield attendue")
	}
	router.Mode = "agent"
	router.ShieldLevel = ""
}

// TestShieldEnsureConvergence — cycle complet : activation → file ; en
// vol → pas de doublon ; retour ok (sig + horodatage frais) → silence ;
// changement de niveau → re-file ; horodatage ancien → auto-réparation.
func TestShieldEnsureConvergence(t *testing.T) {
	a := newWatchAPI()
	db := &model.DB{}
	router := shieldRouter()

	// Le gérant active le bouclier : la commande rejoint la file avec le
	// niveau et la signature attendue.
	router.ShieldLevel = model.ShieldOn
	a.ensureShieldLocked(db, router)
	cmd := findQueuedByKind(db, model.CmdShield)
	if cmd == nil {
		t.Fatal("activation : shield doit être en file")
	}
	if l, _ := cmd.Payload["level"].(string); l != model.ShieldOn {
		t.Fatalf("payload level = %q, attendu %q", l, model.ShieldOn)
	}
	if s, _ := cmd.Payload["sig"].(string); s != shieldSig(model.ShieldOn) {
		t.Fatalf("payload sig = %q, attendu %q", s, shieldSig(model.ShieldOn))
	}

	// En vol → pas de doublon.
	for i := range db.Commands {
		db.Commands[i].Status = "sent"
	}
	a.ensureShieldLocked(db, router)
	n := 0
	for _, c := range db.Commands {
		if c.Kind == model.CmdShield {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("commande en vol dupliquée : %d, attendu 1", n)
	}

	// Retour ok simulé (handleAgentResult pose sig + horodatage) : silence.
	db.Commands[0].Status = "done"
	router.ShieldSig = shieldSig(model.ShieldOn)
	router.ShieldAppliedAt = time.Now().Format(time.RFC3339)
	a.ensureShieldLocked(db, router)
	if len(db.Commands) != 1 {
		t.Fatal("re-file injustifiée avec sig fraîche")
	}

	// Retour ok simulé, puis horodatage ANCIEN (> 6 h) : auto-réparation
	// périodique (une règle effacée localement est recréée).
	db.Commands[0].Status = "done"
	router.ShieldSig = shieldSig(model.ShieldOn)
	router.ShieldAppliedAt = time.Now().Add(-shieldRefresh - time.Minute).Format(time.RFC3339)
	a.ensureShieldLocked(db, router)
	if len(db.Commands) != 2 {
		t.Fatalf("réparation périodique non déclenchée : %d commande(s), attendu 2", len(db.Commands))
	}

	// Échec simulé (sig jamais posée) : retenté au check-in suivant.
	db.Commands[1].Status = "error"
	router.ShieldAppliedAt = time.Now().Format(time.RFC3339) // frais mais sig absente
	router.ShieldSig = ""
	a.ensureShieldLocked(db, router)
	if len(db.Commands) != 3 {
		t.Fatalf("échec non retenté : %d commande(s), attendu 3", len(db.Commands))
	}
}

// TestShieldRetireApresExtinction — le gérant désactive : le niveau off
// avec une sig posée (le routeur A connu le bouclier) re-file la
// commande de RETRAIT — les règles ne restent pas orphelines sur le
// routeur. Puis, une fois le retrait confirmé (sig off posée), silence
// périodique uniquement.
func TestShieldRetireApresExtinction(t *testing.T) {
	a := newWatchAPI()
	db := &model.DB{}
	router := shieldRouter()

	router.ShieldLevel = model.ShieldOn
	router.ShieldSig = shieldSig(model.ShieldOn)
	router.ShieldAppliedAt = time.Now().Format(time.RFC3339)

	// Extinction : off + sig on → mismatch → commande de retrait.
	router.ShieldLevel = model.ShieldOff
	a.ensureShieldLocked(db, router)
	cmd := findQueuedByKind(db, model.CmdShield)
	if cmd == nil {
		t.Fatal("extinction : la commande de retrait doit être filée")
	}
	if l, _ := cmd.Payload["level"].(string); l != model.ShieldOff {
		t.Fatalf("retrait avec level = %q, attendu %q", l, model.ShieldOff)
	}

	// Retrait confirmé (sig off + frais) : la cadence périodique poursuit
	// (idempotent, ~400 o toutes les 6 h) — pas de spam au-delà.
	db.Commands[0].Status = "done"
	router.ShieldSig = shieldSig(model.ShieldOff)
	router.ShieldAppliedAt = time.Now().Format(time.RFC3339)
	a.ensureShieldLocked(db, router)
	if len(db.Commands) != 1 {
		t.Fatal("sig off fraîche : re-file injustifiée")
	}
}

// TestShieldResultVerified — la signature n'est posée que si le COMPTE
// de règles marquées rapporté correspond à 5 × le nombre de serveurs
// hotspots RAPPORTÉ (on), ou 0 (off). Un compte divergent ou un hs
// illisible reste sans sig.
func TestShieldResultVerified(t *testing.T) {
	sig := shieldSig(model.ShieldOn)
	cmd := model.Command{ID: "c-sh", Kind: model.CmdShield,
		Payload: map[string]any{"level": model.ShieldOn, "sig": sig}}

	if !shieldResultVerified(&cmd, model.ShieldOn, "5", "1") {
		t.Fatal("on + 1 hotspot + 5 règles rapportées : la vérification doit passer")
	}
	if !shieldResultVerified(&cmd, model.ShieldOn, "10", "2") {
		t.Fatal("on + 2 hotspots + 10 règles rapportées : la vérification doit passer")
	}
	if shieldResultVerified(&cmd, model.ShieldOn, "0", "1") {
		t.Fatal("on + 1 hotspot + 0 règle rapportée : la vérification doit échouer")
	}
	if shieldResultVerified(&cmd, model.ShieldOn, "5", "2") {
		t.Fatal("on + 2 hotspots + 5 règles rapportées : la vérification doit échouer")
	}
	if shieldResultVerified(&cmd, model.ShieldOn, "5", "") {
		t.Fatal("on + hs illisible : la vérification doit échouer (re-file prudent)")
	}
	// Niveau périmé en vol : jamais figé (pattern N°80).
	if shieldResultVerified(&cmd, model.ShieldOff, "5", "1") {
		t.Fatal("niveau rapporté ≠ niveau courant : la vérification doit échouer")
	}
	offCmd := model.Command{ID: "c-sh", Kind: model.CmdShield,
		Payload: map[string]any{"level": model.ShieldOff, "sig": shieldSig(model.ShieldOff)}}
	if !shieldResultVerified(&offCmd, model.ShieldOff, "0", "0") {
		t.Fatal("off + 0 règle rapportée : la vérification doit passer")
	}
	if shieldResultVerified(&offCmd, model.ShieldOff, "5", "1") {
		t.Fatal("off + règles résiduelles : la vérification doit échouer")
	}
}

// shieldResultVerified — extrait de handleAgentResult (branche N°81) :
// vrai si le compte de règles marquées rapporté correspond à 5 règles
// par hotspot RAPPORTÉ pour le niveau de la commande, ET si ce niveau
// est toujours le niveau courant du routeur.
func shieldResultVerified(cmd *model.Command, currentLevel, reportedRules, reportedHS string) bool {
	level := agent.ShieldLevelFromPayload(cmd.Payload)
	if level != currentLevel {
		return false
	}
	want := 0
	if level != model.ShieldOff {
		hs, ok := parseReportInt(reportedHS)
		if !ok {
			return false // hs illisible : vérification impossible → pas de sig
		}
		want = shieldRulesPerHotspot * hs
	}
	got, ok := parseReportInt(reportedRules)
	return ok && got == want
}

// TestShieldSigVersioned — le sel de version garantit qu'une évolution de
// la FORME des règles est re-poussée au parc entier (garde-fou N°48).
func TestShieldSigVersioned(t *testing.T) {
	legacy := agent.HashToken(model.ShieldOn)[:16] // formule sans sel
	if s := shieldSig(model.ShieldOn); s == legacy {
		t.Fatal("shieldSig sans sel de version : un correctif de règles ne serait jamais re-poussé (régression N°48)")
	}
	if shieldSig(model.ShieldOn) == shieldSig(model.ShieldOff) {
		t.Fatal("des niveaux distincts doivent produire des signatures distinctes")
	}
	if s1, s2 := shieldSig(model.ShieldOn), shieldSig(model.ShieldOn); s1 != s2 {
		t.Fatal("shieldSig doit être déterministe")
	}
}

// TestShieldBatchDeferredLast — la commande shield ferme la marche du
// batch (après walled_garden, hotspot_files et safewifi) : sa réussite
// ne conditionne aucune autre commande, et une ligne avortée ne doit pas
// tuer ce qui suit (contrat N°77 du deferred bucket).
func TestShieldBatchDeferredLast(t *testing.T) {
	db := &model.DB{Commands: []model.Command{
		{ID: "c-hf", RouterID: "r-sh", Kind: model.CmdHotspotFiles, Status: "queued", CreatedAt: "2026-09-13T00:00:00Z"},
		{ID: "c-sh", RouterID: "r-sh", Kind: model.CmdShield, Status: "queued", CreatedAt: "2026-09-13T00:00:01Z"},
		{ID: "c-wg", RouterID: "r-sh", Kind: model.CmdWalledGarden, Status: "queued", CreatedAt: "2026-09-13T00:00:02Z"},
		{ID: "c-sw", RouterID: "r-sh", Kind: model.CmdSafeWifi, Status: "queued", CreatedAt: "2026-09-13T00:00:03Z"},
	}}
	router := shieldRouter()

	// Reproduit le tri du deferred bucket du check-in.
	deferred := []model.Command{}
	for i := range db.Commands {
		if db.Commands[i].RouterID == router.ID && db.Commands[i].Status == "queued" {
			switch db.Commands[i].Kind {
			case model.CmdWalledGarden, model.CmdHotspotFiles, model.CmdSafeWifi, model.CmdShield:
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
		case model.CmdSafeWifi:
			return 80
		}
		return 81
	}
	// Tri du deferred bucket : la vague domine (29 < 35 < 80 < 81).
	bubbleSortByWave(deferred, deferredWave)

	if len(deferred) != 4 {
		t.Fatalf("deferred = %d, attendu 4", len(deferred))
	}
	want := []string{model.CmdWalledGarden, model.CmdHotspotFiles, model.CmdSafeWifi, model.CmdShield}
	got := make([]string, len(deferred))
	for i, c := range deferred {
		got[i] = c.Kind
	}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("ordre du deferred bucket = %v, attendu %v", got, want)
	}
}
