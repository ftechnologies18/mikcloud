package api

// Tests N°88 — AntiVPN : convergence au check-in (ensure), signature
// vérifiée au retour (4 règles × hotspots rapportés, vérité routeur),
// silence des routeurs qui n'ont jamais ouvert la carte, et priorité
// dans le batch du check-in (deferred bucket, vague 88 en fermeture).

import (
	"strings"
	"testing"
	"time"

	"mikcloud/hotspot-api/internal/agent"
	"mikcloud/hotspot-api/internal/model"
)

// antiVpnRouter — routeur agent antérieur au N°88 (aucune colonne posée).
func antiVpnRouter() *model.Router {
	return &model.Router{ID: "r-av", AccountID: "acc-av", Name: "Tunnelier", Mode: "agent", Version: "7.20 (stable)"}
}

// TestAntiVpnNeverUsedStaysSilent — le contrat d'économie : un routeur
// dont le gérant n'a JAMAIS ouvert la carte (niveau off implicite, sig
// vide) ne reçoit AUCUNE commande — l'économie de veille N°75 reste
// entière, y compris pour tout le parc existant après le déploiement
// du N°88.
func TestAntiVpnNeverUsedStaysSilent(t *testing.T) {
	a := newWatchAPI()
	db := &model.DB{}
	router := antiVpnRouter()

	a.ensureAntiVpnLocked(db, router)
	if len(db.Commands) != 0 {
		t.Fatalf("routeur n'ayant jamais ouvert la carte : %d commande(s) filée(s), attendu 0", len(db.Commands))
	}

	// Simulé : pas concerné non plus.
	router.Mode = "simulated"
	router.AntiVpnLevel = model.AntiVpnOn
	a.ensureAntiVpnLocked(db, router)
	if len(db.Commands) != 0 {
		t.Fatal("routeur simulé : aucune commande antivpn attendue")
	}
	router.Mode = "agent"
	router.AntiVpnLevel = ""
}

// TestAntiVpnEnsureConvergence — cycle complet : activation → file ; en
// vol → pas de doublon ; retour ok (sig + horodatage frais) → silence ;
// horodatage ancien → auto-réparation ; échec → retenté.
func TestAntiVpnEnsureConvergence(t *testing.T) {
	a := newWatchAPI()
	db := &model.DB{}
	router := antiVpnRouter()

	// Le gérant active le bloque-VPN : la commande rejoint la file avec le
	// niveau et la signature attendue.
	router.AntiVpnLevel = model.AntiVpnOn
	a.ensureAntiVpnLocked(db, router)
	cmd := findQueuedByKind(db, model.CmdAntiVpn)
	if cmd == nil {
		t.Fatal("activation : antivpn doit être en file")
	}
	if l, _ := cmd.Payload["level"].(string); l != model.AntiVpnOn {
		t.Fatalf("payload level = %q, attendu %q", l, model.AntiVpnOn)
	}
	if s, _ := cmd.Payload["sig"].(string); s != antiVpnSig(model.AntiVpnOn) {
		t.Fatalf("payload sig = %q, attendu %q", s, antiVpnSig(model.AntiVpnOn))
	}

	// En vol → pas de doublon.
	for i := range db.Commands {
		db.Commands[i].Status = "sent"
	}
	a.ensureAntiVpnLocked(db, router)
	n := 0
	for _, c := range db.Commands {
		if c.Kind == model.CmdAntiVpn {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("commande en vol dupliquée : %d, attendu 1", n)
	}

	// Retour ok simulé (handleAgentResult pose sig + horodatage) : silence.
	db.Commands[0].Status = "done"
	router.AntiVpnSig = antiVpnSig(model.AntiVpnOn)
	router.AntiVpnAppliedAt = time.Now().Format(time.RFC3339)
	a.ensureAntiVpnLocked(db, router)
	if len(db.Commands) != 1 {
		t.Fatal("re-file injustifiée avec sig fraîche")
	}

	// Retour ok simulé, puis horodatage ANCIEN (> 6 h) : auto-réparation
	// périodique (une règle effacée localement est recréée).
	db.Commands[0].Status = "done"
	router.AntiVpnSig = antiVpnSig(model.AntiVpnOn)
	router.AntiVpnAppliedAt = time.Now().Add(-antiVpnRefresh - time.Minute).Format(time.RFC3339)
	a.ensureAntiVpnLocked(db, router)
	if len(db.Commands) != 2 {
		t.Fatalf("réparation périodique non déclenchée : %d commande(s), attendu 2", len(db.Commands))
	}

	// Échec simulé (sig jamais posée) : retenté au check-in suivant.
	db.Commands[1].Status = "error"
	router.AntiVpnAppliedAt = time.Now().Format(time.RFC3339) // frais mais sig absente
	router.AntiVpnSig = ""
	a.ensureAntiVpnLocked(db, router)
	if len(db.Commands) != 3 {
		t.Fatalf("échec non retenté : %d commande(s), attendu 3", len(db.Commands))
	}
}

// TestAntiVpnRetireApresExtinction — le gérant désactive : le niveau off
// avec une sig posée (le routeur A connu le bloque-VPN) re-file la
// commande de RETRAIT — les règles ne restent pas orphelines sur le
// routeur. Puis, une fois le retrait confirmé (sig off posée), silence
// périodique uniquement.
func TestAntiVpnRetireApresExtinction(t *testing.T) {
	a := newWatchAPI()
	db := &model.DB{}
	router := antiVpnRouter()

	router.AntiVpnLevel = model.AntiVpnOn
	router.AntiVpnSig = antiVpnSig(model.AntiVpnOn)
	router.AntiVpnAppliedAt = time.Now().Format(time.RFC3339)

	// Extinction : off + sig on → mismatch → commande de retrait.
	router.AntiVpnLevel = model.AntiVpnOff
	a.ensureAntiVpnLocked(db, router)
	cmd := findQueuedByKind(db, model.CmdAntiVpn)
	if cmd == nil {
		t.Fatal("extinction : la commande de retrait doit être filée")
	}
	if l, _ := cmd.Payload["level"].(string); l != model.AntiVpnOff {
		t.Fatalf("retrait avec level = %q, attendu %q", l, model.AntiVpnOff)
	}

	// Retrait confirmé (sig off + frais) : la cadence périodique poursuit
	// (idempotent, ~400 o toutes les 6 h) — pas de spam au-delà.
	db.Commands[0].Status = "done"
	router.AntiVpnSig = antiVpnSig(model.AntiVpnOff)
	router.AntiVpnAppliedAt = time.Now().Format(time.RFC3339)
	a.ensureAntiVpnLocked(db, router)
	if len(db.Commands) != 1 {
		t.Fatal("sig off fraîche : re-file injustifiée")
	}
}

// TestAntiVpnResultVerified — la signature n'est posée que si le COMPTE
// de règles marquées rapporté correspond à 4 × le nombre de serveurs
// hotspots RAPPORTÉ (on), ou 0 (off). Un compte divergent ou un hs
// illisible reste sans sig.
func TestAntiVpnResultVerified(t *testing.T) {
	sig := antiVpnSig(model.AntiVpnOn)
	cmd := model.Command{ID: "c-av", Kind: model.CmdAntiVpn,
		Payload: map[string]any{"level": model.AntiVpnOn, "sig": sig}}

	if !antiVpnResultVerified(&cmd, model.AntiVpnOn, "4", "1") {
		t.Fatal("on + 1 hotspot + 4 règles rapportées : la vérification doit passer")
	}
	if !antiVpnResultVerified(&cmd, model.AntiVpnOn, "8", "2") {
		t.Fatal("on + 2 hotspots + 8 règles rapportées : la vérification doit passer")
	}
	if antiVpnResultVerified(&cmd, model.AntiVpnOn, "0", "1") {
		t.Fatal("on + 1 hotspot + 0 règle rapportée : la vérification doit échouer")
	}
	if antiVpnResultVerified(&cmd, model.AntiVpnOn, "4", "2") {
		t.Fatal("on + 2 hotspots + 4 règles rapportées : la vérification doit échouer")
	}
	if antiVpnResultVerified(&cmd, model.AntiVpnOn, "4", "") {
		t.Fatal("on + hs illisible : la vérification doit échouer (re-file prudent)")
	}
	// Niveau périmé en vol : jamais figé (pattern N°80/81).
	if antiVpnResultVerified(&cmd, model.AntiVpnOff, "4", "1") {
		t.Fatal("niveau rapporté ≠ niveau courant : la vérification doit échouer")
	}
	offCmd := model.Command{ID: "c-av", Kind: model.CmdAntiVpn,
		Payload: map[string]any{"level": model.AntiVpnOff, "sig": antiVpnSig(model.AntiVpnOff)}}
	if !antiVpnResultVerified(&offCmd, model.AntiVpnOff, "0", "0") {
		t.Fatal("off + 0 règle rapportée : la vérification doit passer")
	}
	if antiVpnResultVerified(&offCmd, model.AntiVpnOff, "4", "1") {
		t.Fatal("off + règles résiduelles : la vérification doit échouer")
	}
}

// antiVpnResultVerified — extrait de handleAgentResult (branche N°88) :
// vrai si le compte de règles marquées rapporté correspond à 4 règles
// par hotspot RAPPORTÉ pour le niveau de la commande, ET si ce niveau
// est toujours le niveau courant du routeur.
func antiVpnResultVerified(cmd *model.Command, currentLevel, reportedRules, reportedHS string) bool {
	level := agent.AntiVpnLevelFromPayload(cmd.Payload)
	if level != currentLevel {
		return false
	}
	want := 0
	if level != model.AntiVpnOff {
		hs, ok := parseReportInt(reportedHS)
		if !ok {
			return false // hs illisible : vérification impossible → pas de sig
		}
		want = agent.AntiVpnRulesPerHotspot * hs
	}
	got, ok := parseReportInt(reportedRules)
	return ok && got == want
}

// TestAntiVpnSigVersioned — le sel de version garantit qu'une évolution
// de la FORME des règles est re-poussée au parc entier (garde-fou N°48).
func TestAntiVpnSigVersioned(t *testing.T) {
	legacy := agent.HashToken(model.AntiVpnOn)[:16] // formule sans sel
	if s := antiVpnSig(model.AntiVpnOn); s == legacy {
		t.Fatal("antiVpnSig sans sel de version : un correctif de règles ne serait jamais re-poussé (régression N°48)")
	}
	if antiVpnSig(model.AntiVpnOn) == antiVpnSig(model.AntiVpnOff) {
		t.Fatal("des niveaux distincts doivent produire des signatures distinctes")
	}
	if s1, s2 := antiVpnSig(model.AntiVpnOn), antiVpnSig(model.AntiVpnOn); s1 != s2 {
		t.Fatal("antiVpnSig doit être déterministe")
	}
}

// TestAntiVpnBatchDeferredLast — la commande antivpn ferme la marche du
// batch (après walled_garden, hotspot_files, safewifi, shield et
// familyguard) : sa réussite ne conditionne aucune autre commande, et
// une ligne avortée ne doit pas tuer ce qui suit (contrat N°77 du
// deferred bucket).
func TestAntiVpnBatchDeferredLast(t *testing.T) {
	db := &model.DB{Commands: []model.Command{
		{ID: "c-fg", RouterID: "r-av", Kind: model.CmdFamilyGuard, Status: "queued", CreatedAt: "2026-09-13T00:00:00Z"},
		{ID: "c-av", RouterID: "r-av", Kind: model.CmdAntiVpn, Status: "queued", CreatedAt: "2026-09-13T00:00:01Z"},
		{ID: "c-sh", RouterID: "r-av", Kind: model.CmdShield, Status: "queued", CreatedAt: "2026-09-13T00:00:02Z"},
		{ID: "c-sw", RouterID: "r-av", Kind: model.CmdSafeWifi, Status: "queued", CreatedAt: "2026-09-13T00:00:03Z"},
	}}
	router := antiVpnRouter()

	// Reproduit le tri du deferred bucket du check-in.
	deferred := []model.Command{}
	for i := range db.Commands {
		if db.Commands[i].RouterID == router.ID && db.Commands[i].Status == "queued" {
			switch db.Commands[i].Kind {
			case model.CmdWalledGarden, model.CmdHotspotFiles, model.CmdSafeWifi, model.CmdShield, model.CmdFamilyGuard, model.CmdAntiVpn:
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
		case model.CmdFamilyGuard:
			return 82
		}
		return 88
	}
	// Tri du deferred bucket : la vague domine (29 < 35 < 80 < 81 < 82 < 88).
	bubbleSortByWave(deferred, deferredWave)

	if len(deferred) != 4 {
		t.Fatalf("deferred = %d, attendu 4", len(deferred))
	}
	want := []string{model.CmdSafeWifi, model.CmdShield, model.CmdFamilyGuard, model.CmdAntiVpn}
	got := make([]string, len(deferred))
	for i, c := range deferred {
		got[i] = c.Kind
	}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("ordre du deferred bucket = %v, attendu %v", got, want)
	}
}
