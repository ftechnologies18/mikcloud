package agent

// Tests N°162 — le lot de vouchers dit la vérité. Incident Zikisso
// (20/09/2026) : buildVoucherBatch avalait chaque échec d'ajout dans un
// log routeur (on-error sans compteur) et rapportait « ok / created=N »
// même sans AUCUN utilisateur créé — les tickets restaient « Actif »,
// badgés « absent du routeur », invendables et sans retrouvabilité
// (les écritures ne sont jamais rejouées, N°73).
//
//   - chaque échec d'ajout incrémente un compteur routeur ;
//   - le lot passe « error » dès le premier échec, avec les compteurs
//     created/failed calculés CÔTÉ ROUTEUR (pattern $step du N°159) ;
//   - la vague de RÉPARATION (repair:true) est idempotente : un nom déjà
//     présent n'est ni recompté ni retouché (verrou MAC, marqueur mikq:,
//     comment de traçabilité préservés) ;
//   - les limites PAR VOUCHER de la réparation (quota/uptime stockés par
//     ticket) voyagent dans chaque ligne — réparer un « 5 Go » sans
//     limite serait offrir des données.

import (
	"strings"
	"testing"

	"mikcloud/hotspot-api/internal/model"
)

func batchCmd(payload map[string]any) model.Command {
	return model.Command{ID: "c-b1", Kind: model.CmdVoucherBatch, Payload: payload}
}

// TestVoucherBatchCountsFailures — le compteur d'échecs alimente le
// rapport : initialisation, incrémentation par add, bascule error, et le
// fetch d'erreur porte les compteurs dynamiques.
func TestVoucherBatchCountsFailures(t *testing.T) {
	b := Builder{BaseURL: "https://cloud.example", Token: "tok"}
	cmd := batchCmd(map[string]any{
		"profile": map[string]any{"name": "1h"},
		"users": []map[string]any{
			{"name": "v1", "password": "p1"},
			{"name": "v2", "password": "p2"},
		},
		"batch": "B1",
	})
	script := b.buildVoucherBatch(cmd)

	if !strings.Contains(script, ":local fails"+idSafe(cmd.ID)+" 0") {
		t.Fatal("le compteur d'échecs doit être initialisé à zéro")
	}
	if n := strings.Count(script, ":set fails"+idSafe(cmd.ID)+" (fails"+idSafe(cmd.ID)+" + 1)"); n != 2 {
		t.Fatalf("chaque add doit incrémenter le compteur en cas d'échec, %d incrémentations trouvées", n)
	}
	// L'ancien bug : le on-error ne faisait QUE logger (compteur absent).
	if strings.Contains(script, `on-error={ :log warning "mikcloud: add voucher echoue" }`) {
		t.Fatal("le on-error doit AUSSI compter l'échec, pas seulement logger (bug Zikisso)")
	}
	if !strings.Contains(script, ":if ($fails"+idSafe(cmd.ID)+" > 0) do={ :set ok"+idSafe(cmd.ID)+" false }") {
		t.Fatal("un seul échec doit faire passer le lot en error")
	}
	// Rapports : ok statique, error DYNAMIQUE (created = total - failed).
	if !strings.Contains(script, "status=ok&created=2") {
		t.Fatal("le rapport ok doit porter created=2")
	}
	if !strings.Contains(script, `&status=error&message=ajouts_en_echec_sur_le_routeur&created=" . (2 - $fails`+idSafe(cmd.ID)+`) . "&failed=" . $fails`+idSafe(cmd.ID)) {
		t.Fatal("le rapport d'erreur doit porter les compteurs dynamiques created/failed")
	}
}

// TestVoucherBatchRepairGuard — la vague de réparation ne touche JAMAIS un
// utilisateur déjà présent sur le routeur (idempotence stricte).
func TestVoucherBatchRepairGuard(t *testing.T) {
	b := Builder{BaseURL: "https://cloud.example", Token: "tok"}
	cmd := batchCmd(map[string]any{
		"profile": map[string]any{"name": "1h"},
		"users": []map[string]any{
			{"name": "v1", "password": "p1"},
		},
		"repair": true,
	})
	script := b.buildVoucherBatch(cmd)

	guard := `:if ([:len [/ip hotspot user find name="v1"]] = 0) do={`
	if !strings.Contains(script, guard) {
		t.Fatal("la réparation doit être gardée par l'existence du nom (idempotence)")
	}

	// Contre-épreuve : un lot de GÉNÉRATION classique n'a PAS la garde
	// (un nom déjà pris est un échec à RAPPORTER, pas à ignorer).
	gen := batchCmd(map[string]any{
		"profile": map[string]any{"name": "1h"},
		"users": []map[string]any{
			{"name": "v1", "password": "p1"},
		},
		"batch": "B1",
	})
	if strings.Contains(b.buildVoucherBatch(gen), guard) {
		t.Fatal("la génération classique ne doit pas porter la garde d'existence")
	}
}

// TestVoucherBatchPerUserLimits — la réparation recompose les limites
// résolues à la génération, STOCKÉES PAR TICKET : deux vouchers du même
// profil avec des quotas/temps différents portent CHACUN sa limite.
func TestVoucherBatchPerUserLimits(t *testing.T) {
	b := Builder{BaseURL: "https://cloud.example", Token: "tok"}
	cmd := batchCmd(map[string]any{
		"profile": map[string]any{"name": "libre"},
		"users": []map[string]any{
			{"name": "fivego", "password": "p1", "limitBytesTotal": int64(5120 * 1048576), "limitUptimeMin": int64(60)},
			{"name": "free", "password": "p2"},
		},
		"repair": true,
	})
	script := b.buildVoucherBatch(cmd)

	if !strings.Contains(script, `limit-bytes-total=5368709120`) {
		t.Fatal("le voucher 5 Go doit porter SON quota (fidélité au ticket vendu)")
	}
	if !strings.Contains(script, `limit-uptime=1h`) {
		t.Fatal("le voucher 60 min doit porter SON temps")
	}
	// Le voucher libre : sa ligne ne doit PAS porter de limites.
	for _, line := range strings.Split(script, "\n") {
		if strings.Contains(line, `name="free"`) && (strings.Contains(line, "limit-bytes-total") || strings.Contains(line, "limit-uptime=")) {
			t.Fatalf("le voucher sans limite ne doit en porter aucune : %s", line)
		}
	}
	// Héritage du lot : un utilisateur SANS limites individuelles dans un
	// lot qui en porte au niveau batch garde le comportement historique.
	legacy := batchCmd(map[string]any{
		"profile":         map[string]any{"name": "1h"},
		"users":           []map[string]any{{"name": "gen1", "password": "p"}},
		"batch":           "B9",
		"limitBytesTotal": int64(1073741824),
		"limitUptimeMin":  120,
	})
	gen := b.buildVoucherBatch(legacy)
	if !strings.Contains(gen, `limit-bytes-total=1073741824`) || !strings.Contains(gen, `limit-uptime=2h`) {
		t.Fatal("la génération classique hérite des limites du lot (comportement inchangé)")
	}
}

// TestVoucherBatchThrottleMarkerPerUser — mode bridage : le marqueur mikq:
// est posé PAR VOUCHER avec SON quota (la réparation peut mêler des
// tickets de quotas différents dans une même vague).
func TestVoucherBatchThrottleMarkerPerUser(t *testing.T) {
	b := Builder{BaseURL: "https://cloud.example", Token: "tok"}
	cmd := batchCmd(map[string]any{
		"profile": map[string]any{"name": "throttle", "quotaMode": "throttle"},
		"users": []map[string]any{
			{"name": "t1", "password": "p1", "limitBytesTotal": int64(5368709120)},
			{"name": "t2", "password": "p2", "limitBytesTotal": int64(1073741824)},
		},
		"repair":       true,
		"throttleRate": "1M",
	})
	script := b.buildVoucherBatch(cmd)

	if strings.Contains(script, "limit-bytes-total") {
		t.Fatal("mode throttle : limit-bytes-total interdit (le routeur couperait à l'épuisement)")
	}
	if !strings.Contains(script, `mikq:5368709120,1M`) || !strings.Contains(script, `mikq:1073741824,1M`) {
		t.Fatal("chaque voucher bridé doit porter SON marqueur mikq: avec SON quota")
	}
}
