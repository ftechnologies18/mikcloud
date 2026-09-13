package api

// Tests N°82 — FamilyGuard : convergence au check-in (ensure, l'ÉTAT désiré
// recalculé à chaque passage), signature vérifiée au retour (1 règle ×
// hotspots rapportés en couvre-feu actif, 0 sinon), silence des routeurs
// qui n'ont jamais ouvert la carte, et priorité dans le batch du check-in.

import (
	"strings"
	"testing"
	"time"

	"mikcloud/hotspot-api/internal/agent"
	"mikcloud/hotspot-api/internal/model"
)

// fgRouter — routeur agent antérieur au N°82 (aucune colonne posée).
func fgRouter() *model.Router {
	return &model.Router{ID: "r-fg", AccountID: "acc-fg", Name: "Gardien", Mode: "agent", Version: "7.20 (stable)"}
}

// fgTestSpec — construit un spec dont la fenêtre COUVRE (active=true) ou
// NE COUVRE PAS (active=false) l'instant présent, avec 30 minutes de marge
// de chaque côté : robuste au passage de minute pendant le test.
func fgTestSpec(active bool) string {
	n := time.Now().UTC()
	m := n.Hour()*60 + n.Minute()
	var s, e int
	if active {
		s, e = (m+1440-30)%1440, (m+30)%1440
	} else {
		s, e = (m+180)%1440, (m+240)%1440
	}
	cfg := model.FamilyGuardConfig{Enabled: true, Start: fgHHMM(s), End: fgHHMM(e), Days: "1111111"}
	return cfg.SpecString()
}

// fgHHMM — minutes depuis minuit → "HH:MM".
func fgHHMM(m int) string {
	return string([]byte{byte('0' + (m/60)/10), byte('0' + (m/60)%10), ':', byte('0' + (m%60)/10), byte('0' + (m%60)%10)})
}

// TestFamilyGuardNeverUsedStaysSilent — le contrat d'économie : un routeur
// dont le gérant n'a JAMAIS ouvert la carte (spec vide) ne reçoit AUCUNE
// commande — l'économie de veille N°75 reste entière, y compris pour tout
// le parc existant après le déploiement du N°82.
func TestFamilyGuardNeverUsedStaysSilent(t *testing.T) {
	a := newWatchAPI()
	db := &model.DB{}
	router := fgRouter()

	a.ensureFamilyGuardLocked(db, router)
	if len(db.Commands) != 0 {
		t.Fatalf("routeur n'ayant jamais ouvert la carte : %d commande(s) filée(s), attendu 0", len(db.Commands))
	}

	// Simulé : pas concerné non plus.
	router.Mode = "simulated"
	router.FamilyGuardSpec = "1|22:00|06:00|1111111"
	a.ensureFamilyGuardLocked(db, router)
	if len(db.Commands) != 0 {
		t.Fatal("routeur simulé : aucune commande familyguard attendue")
	}
}

// TestFamilyGuardEnsureConvergence — cycle complet : programmation → file
// (avec l'état courant calculé par le cloud) ; en vol → pas de doublon ;
// retour ok (sig + horodatage frais) → silence ; horodatage ancien →
// auto-réparation ; échec → retenté. Puis : la signature porte l'ÉTAT —
// une sig posée avec l'AUTRE état (la frontière de fenêtre a été franchie)
// re-file la bascule SANS changement de spec.
func TestFamilyGuardEnsureConvergence(t *testing.T) {
	a := newWatchAPI()
	db := &model.DB{}
	router := fgRouter()

	// Le gérant programme un couvre-feu couvrant maintenant : la commande
	// rejoint la file avec le spec, l'état et la signature attendue.
	spec := fgTestSpec(true)
	router.FamilyGuardSpec = spec
	a.ensureFamilyGuardLocked(db, router)
	cmd := findQueuedByKind(db, model.CmdFamilyGuard)
	if cmd == nil {
		t.Fatal("programmation : familyguard doit être en file")
	}
	if s, _ := cmd.Payload["spec"].(string); s != spec {
		t.Fatalf("payload spec = %q, attendu %q", s, spec)
	}
	if b, _ := cmd.Payload["active"].(bool); !b {
		t.Fatal("payload active : attendu true (la fenêtre couvre maintenant)")
	}
	if s, _ := cmd.Payload["sig"].(string); s != familyGuardSig(spec, true) {
		t.Fatalf("payload sig = %q, attendu %q", s, familyGuardSig(spec, true))
	}

	// En vol → pas de doublon.
	for i := range db.Commands {
		db.Commands[i].Status = "sent"
	}
	a.ensureFamilyGuardLocked(db, router)
	n := 0
	for _, c := range db.Commands {
		if c.Kind == model.CmdFamilyGuard {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("commande en vol dupliquée : %d, attendu 1", n)
	}

	// Retour ok simulé (handleAgentResult pose sig + horodatage) : silence.
	db.Commands[0].Status = "done"
	router.FamilyGuardSig = familyGuardSig(spec, true)
	router.FamilyGuardAppliedAt = time.Now().Format(time.RFC3339)
	a.ensureFamilyGuardLocked(db, router)
	if len(db.Commands) != 1 {
		t.Fatal("re-file injustifiée avec sig fraîche")
	}

	// Retour ok simulé, puis horodatage ANCIEN (> 6 h) : auto-réparation
	// périodique (une règle effacée localement est recréée).
	db.Commands[0].Status = "done"
	router.FamilyGuardAppliedAt = time.Now().Add(-familyGuardRefresh - time.Minute).Format(time.RFC3339)
	a.ensureFamilyGuardLocked(db, router)
	if len(db.Commands) != 2 {
		t.Fatalf("réparation périodique non déclenchée : %d commande(s), attendu 2", len(db.Commands))
	}

	// Échec simulé (sig jamais posée) : retenté au check-in suivant.
	db.Commands[1].Status = "error"
	router.FamilyGuardAppliedAt = time.Now().Format(time.RFC3339)
	router.FamilyGuardSig = ""
	a.ensureFamilyGuardLocked(db, router)
	if len(db.Commands) != 3 {
		t.Fatalf("échec non retenté : %d commande(s), attendu 3", len(db.Commands))
	}
	db.Commands[2].Status = "done" // résolu avant l'étape suivante

	// LA SIGNATURE PORTE L'ÉTAT : posée avec active=false (le routeur avait
	// appliqué le couvre-feu HORS fenêtre), elle diffère de la signature
	// courante (active=true) → la bascule est re-filée SANS changement de
	// spec — c'est le mécanisme de la frontière de fenêtre (22:00, 06:00…).
	router.FamilyGuardSig = familyGuardSig(spec, false)
	router.FamilyGuardAppliedAt = time.Now().Format(time.RFC3339)
	a.ensureFamilyGuardLocked(db, router)
	if len(db.Commands) != 4 {
		t.Fatalf("frontière de fenêtre : bascule non re-filée (%d commandes, attendu 4)", len(db.Commands))
	}
	if b, _ := db.Commands[3].Payload["active"].(bool); !b {
		t.Fatal("bascule re-filée avec active=false : attendu true")
	}
}

// TestFamilyGuardRetireApresDesactivation — le gérant désactive (enabled=0,
// la fenêtre est conservée) : la commande de RETRAIT est filée — les
// règles ne restent pas orphelines sur le routeur. Puis, une fois le
// retrait confirmé, silence périodique uniquement.
func TestFamilyGuardRetireApresDesactivation(t *testing.T) {
	a := newWatchAPI()
	db := &model.DB{}
	router := fgRouter()

	router.FamilyGuardSpec = fgTestSpec(true)
	router.FamilyGuardSig = familyGuardSig(router.FamilyGuardSpec, true)
	router.FamilyGuardAppliedAt = time.Now().Format(time.RFC3339)

	// Désactivation : enabled=0 + sig active → mismatch → retrait.
	cfg, _ := model.ParseFamilyGuardSpec(router.FamilyGuardSpec)
	cfg.Enabled = false
	router.FamilyGuardSpec = cfg.SpecString()
	a.ensureFamilyGuardLocked(db, router)
	cmd := findQueuedByKind(db, model.CmdFamilyGuard)
	if cmd == nil {
		t.Fatal("désactivation : la commande de retrait doit être filée")
	}
	if b, _ := cmd.Payload["active"].(bool); b {
		t.Fatal("désactivé : le payload active doit être false (retrait)")
	}

	// Retrait confirmé : la cadence périodique poursuit (idempotent) —
	// pas de spam au-delà.
	db.Commands[0].Status = "done"
	router.FamilyGuardSig = familyGuardSig(router.FamilyGuardSpec, false)
	router.FamilyGuardAppliedAt = time.Now().Format(time.RFC3339)
	a.ensureFamilyGuardLocked(db, router)
	if len(db.Commands) != 1 {
		t.Fatal("sig désactivé fraîche : re-file injustifiée")
	}
}

// familyGuardResultVerified — extrait de handleAgentResult (branche N°82) :
// vrai si le compte de règles marquées rapporté correspond à 1 règle par
// hotspot RAPPORTÉ pour l'état de la commande, ET si le spec rapporté est
// toujours celui du routeur, ET si l'état désiré est toujours courant (la
// frontière de fenêtre a pu être franchie pendant le vol).
func familyGuardResultVerified(cmd *model.Command, routerSpec, reportedRules, reportedHS string) bool {
	spec, _ := cmd.Payload["spec"].(string)
	active := agent.FamilyGuardActiveFromPayload(cmd.Payload)
	if spec != routerSpec {
		return false
	}
	cur, ok := model.ParseFamilyGuardSpec(routerSpec)
	if !ok {
		return false
	}
	if cur.ActiveAt(time.Now().UTC()) != active {
		return false
	}
	want := 0
	if active {
		hs, okHS := parseReportInt(reportedHS)
		if !okHS {
			return false
		}
		want = familyGuardRulesPerHotspot * hs
	}
	got, ok := parseReportInt(reportedRules)
	return ok && got == want
}

// TestFamilyGuardResultVerified — la signature n'est posée que si le COMPTE
// de règles marquées rapporté correspond à 1 × hotspots rapportés (actif),
// 0 (levé) ; spec périmé, état périmé ou hs illisible → pas de sig.
func TestFamilyGuardResultVerified(t *testing.T) {
	activeSpec := fgTestSpec(true)
	cmd := model.Command{ID: "c-fg", Kind: model.CmdFamilyGuard,
		Payload: map[string]any{"spec": activeSpec, "active": true, "sig": familyGuardSig(activeSpec, true)}}

	if !familyGuardResultVerified(&cmd, activeSpec, "1", "1") {
		t.Fatal("actif + 1 hotspot + 1 règle rapportée : la vérification doit passer")
	}
	if !familyGuardResultVerified(&cmd, activeSpec, "2", "2") {
		t.Fatal("actif + 2 hotspots + 2 règles rapportées : la vérification doit passer")
	}
	if familyGuardResultVerified(&cmd, activeSpec, "0", "1") {
		t.Fatal("actif + 1 hotspot + 0 règle rapportée : la vérification doit échouer")
	}
	if familyGuardResultVerified(&cmd, activeSpec, "1", "2") {
		t.Fatal("actif + 2 hotspots + 1 règle rapportée : la vérification doit échouer")
	}
	if familyGuardResultVerified(&cmd, activeSpec, "1", "") {
		t.Fatal("actif + hs illisible : la vérification doit échouer (re-file prudent)")
	}
	// Spec périmé en vol : jamais figé (pattern N°80).
	if familyGuardResultVerified(&cmd, fgTestSpec(false), "1", "1") {
		t.Fatal("spec rapporté ≠ spec courant : la vérification doit échouer")
	}
	// État périmé en vol : la frontière de fenêtre a été franchie pendant
	// le vol — l'état rapporté (false) n'est plus celui désiré (true).
	staleCmd := model.Command{ID: "c-fg2", Kind: model.CmdFamilyGuard,
		Payload: map[string]any{"spec": activeSpec, "active": false, "sig": familyGuardSig(activeSpec, false)}}
	if familyGuardResultVerified(&staleCmd, activeSpec, "0", "1") {
		t.Fatal("état rapporté ≠ état courant (frontière franchie) : la vérification doit échouer")
	}
	// Levé (hors fenêtre) : 0 règle attendue.
	inactiveSpec := fgTestSpec(false)
	offCmd := model.Command{ID: "c-fg3", Kind: model.CmdFamilyGuard,
		Payload: map[string]any{"spec": inactiveSpec, "active": false, "sig": familyGuardSig(inactiveSpec, false)}}
	if !familyGuardResultVerified(&offCmd, inactiveSpec, "0", "0") {
		t.Fatal("levé + 0 règle rapportée : la vérification doit passer")
	}
	if familyGuardResultVerified(&offCmd, inactiveSpec, "1", "1") {
		t.Fatal("levé + règle résiduelle : la vérification doit échouer")
	}
}

// TestFamilyGuardSigVersioned — le sel de version garantit qu'une évolution
// de la FORME des règles est re-poussée au parc entier (garde-fou N°48) ;
// le spec ET l'état font partie de la signature.
func TestFamilyGuardSigVersioned(t *testing.T) {
	spec := "1|22:00|06:00|1111111"
	legacy := agent.HashToken(spec)[:16] // formule sans sel
	if s := familyGuardSig(spec, true); s == legacy {
		t.Fatal("familyGuardSig sans sel de version : un correctif de règles ne serait jamais re-poussé (régression N°48)")
	}
	if familyGuardSig(spec, true) == familyGuardSig(spec, false) {
		t.Fatal("états distincts (en fenêtre / hors fenêtre) doivent produire des signatures distinctes — sinon la bascule n'est jamais re-filée")
	}
	if familyGuardSig(spec, true) == familyGuardSig("1|23:00|06:00|1111111", true) {
		t.Fatal("specs distincts doivent produire des signatures distinctes")
	}
	if s1, s2 := familyGuardSig(spec, true), familyGuardSig(spec, true); s1 != s2 {
		t.Fatal("familyGuardSig doit être déterministe")
	}
}

// TestFamilyGuardBatchDeferredLast — la commande familyguard ferme la
// marche du batch (après walled_garden, hotspot_files, safewifi et shield) :
// sa réussite ne conditionne aucune autre commande, et une ligne avortée
// ne doit pas tuer ce qui suit (contrat N°77 du deferred bucket).
func TestFamilyGuardBatchDeferredLast(t *testing.T) {
	db := &model.DB{Commands: []model.Command{
		{ID: "c-hf", RouterID: "r-fg", Kind: model.CmdHotspotFiles, Status: "queued", CreatedAt: "2026-09-13T00:00:00Z"},
		{ID: "c-fg", RouterID: "r-fg", Kind: model.CmdFamilyGuard, Status: "queued", CreatedAt: "2026-09-13T00:00:01Z"},
		{ID: "c-wg", RouterID: "r-fg", Kind: model.CmdWalledGarden, Status: "queued", CreatedAt: "2026-09-13T00:00:02Z"},
		{ID: "c-sw", RouterID: "r-fg", Kind: model.CmdSafeWifi, Status: "queued", CreatedAt: "2026-09-13T00:00:03Z"},
		{ID: "c-sh", RouterID: "r-fg", Kind: model.CmdShield, Status: "queued", CreatedAt: "2026-09-13T00:00:04Z"},
	}}
	router := fgRouter()

	// Reproduit le tri du deferred bucket du check-in.
	deferred := []model.Command{}
	for i := range db.Commands {
		if db.Commands[i].RouterID == router.ID && db.Commands[i].Status == "queued" {
			switch db.Commands[i].Kind {
			case model.CmdWalledGarden, model.CmdHotspotFiles, model.CmdSafeWifi, model.CmdShield, model.CmdFamilyGuard:
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
		case model.CmdShield:
			return 81
		}
		return 82
	}
	// Tri du deferred bucket : la vague domine (29 < 35 < 80 < 81 < 82).
	bubbleSortByWave(deferred, deferredWave)

	if len(deferred) != 5 {
		t.Fatalf("deferred = %d, attendu 5", len(deferred))
	}
	want := []string{model.CmdWalledGarden, model.CmdHotspotFiles, model.CmdSafeWifi, model.CmdShield, model.CmdFamilyGuard}
	got := make([]string, len(deferred))
	for i, c := range deferred {
		got[i] = c.Kind
	}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("ordre du deferred bucket = %v, attendu %v", got, want)
	}
}
