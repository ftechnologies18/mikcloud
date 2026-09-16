// plans_usage_test.go — N°122 : tarifs segmentés Hotspot/HomeNet.
//
// Couvre la SOURCE UNIQUE du pricing (model.ResolvePlan), le moteur
// d'application des périodes (applySubscriptionLocked : montants par mode,
// normalisation des identifiants historiques, forfaits annuels pro-ratisés),
// l'essai segmenté à l'inscription (30 j HomeNet / 3 mois Hotspot) et la
// garde de mode sur la demande de souscription (un compte HomeNet ne peut
// pas souscrire au tarif Hotspot, et réciproquement).
package api

import (
	"testing"
	"time"

	"mikcloud/hotspot-api/internal/model"
)

// TestResolvePlanSegmented — résolution du catalogue : identifiants segmentés
// exacts, identifiants historiques mappés au mode du compte, inconnus refusés.
func TestResolvePlanSegmented(t *testing.T) {
	cas := []struct {
		nom      string
		id       string
		usage    string
		wantOK   bool
		wantID   string
		wantPrix int
	}{
		{"segmenté exact hotspot mensuel", "hotspot-mensuel", "hotspot", true, "hotspot-mensuel", 2500},
		{"segmenté exact hotspot annuel", "hotspot-annuel", "hotspot", true, "hotspot-annuel", 25000},
		{"segmenté exact homenet mensuel", "homenet-mensuel", "homenet", true, "homenet-mensuel", 1250},
		{"segmenté exact homenet annuel", "homenet-annuel", "homenet", true, "homenet-annuel", 12000},
		{"legacy essentiel → mensuel hotspot", "essentiel", "hotspot", true, "hotspot-mensuel", 2500},
		{"legacy essentiel → mensuel homenet", "essentiel", "homenet", true, "homenet-mensuel", 1250},
		{"legacy essentiel usage vide → hotspot", "essentiel", "", true, "hotspot-mensuel", 2500},
		{"legacy illimite → annuel hotspot", "illimite", "hotspot", true, "hotspot-annuel", 25000},
		{"legacy illimite → annuel homenet", "illimite", "homenet", true, "homenet-annuel", 12000},
		{"essai n'est PAS une formule payante", "essai", "hotspot", false, "", 0},
		{"inconnu refusé", "premium-gold", "hotspot", false, "", 0},
	}
	for _, c := range cas {
		p, ok := model.ResolvePlan(c.id, c.usage)
		if ok != c.wantOK {
			t.Fatalf("%s : ResolvePlan(%q, %q) ok = %v, attendu %v", c.nom, c.id, c.usage, ok, c.wantOK)
		}
		if !c.wantOK {
			continue
		}
		if p.ID != c.wantID || p.PriceFcfa != c.wantPrix {
			t.Fatalf("%s : formule résolue = %s/%d, attendu %s/%d", c.nom, p.ID, p.PriceFcfa, c.wantID, c.wantPrix)
		}
	}

	// PlansForUsage — deux formules par mode, dans l'ordre mensuel puis annuel.
	if got := model.PlansForUsage("homenet"); len(got) != 2 || got[0].ID != "homenet-mensuel" || got[1].ID != "homenet-annuel" {
		t.Fatalf("PlansForUsage(homenet) = %+v", got)
	}
	if got := model.PlansForUsage(""); len(got) != 2 || got[0].PriceFcfa != 2500 || got[1].PriceFcfa != 25000 {
		t.Fatalf("PlansForUsage(\"\") doit replier sur hotspot (2500/25000), obtenu %+v", got)
	}

	// Classification — identifiants historiques compris (défense en profondeur).
	for _, id := range []string{"hotspot-annuel", "homenet-annuel", "illimite"} {
		if !model.IsAnnualPlanID(id) {
			t.Fatalf("IsAnnualPlanID(%q) = faux", id)
		}
	}
	for _, id := range []string{"hotspot-mensuel", "homenet-mensuel", "essentiel"} {
		if !model.IsPerRouterPlanID(id) {
			t.Fatalf("IsPerRouterPlanID(%q) = faux", id)
		}
	}
	if model.IsAnnualPlanID("hotspot-mensuel") || model.IsPerRouterPlanID("homenet-annuel") {
		t.Fatal("classification annuelle/par-routeur incohérente")
	}
}

// TestApplySubscriptionUsagePricing — le moteur de périodes applique les
// tarifs du MODE du compte : montants, quotas, libellés et NORMALISATION des
// identifiants historiques vers les identifiants segmentés.
func TestApplySubscriptionUsagePricing(t *testing.T) {
	db := &model.DB{
		Accounts: []model.Account{
			{ID: "acc-hot", Usage: model.AccountUsageHotspot},
			{ID: "acc-home", Usage: model.AccountUsageHomeNet},
		},
		Routers: []model.Router{
			{ID: "r1", AccountID: "acc-hot"},
			{ID: "r2", AccountID: "acc-hot"},
		},
		SettingsByAccount: map[string]model.Settings{},
	}

	type attendu struct {
		nom       string
		acc       string
		planID    string
		months    int
		slots     int
		wantPlan  string
		wantMont  int
		wantSlots int
		wantLabel string
	}
	cas := []attendu{
		{"legacy essentiel hotspot 3 routeurs", "acc-hot", "essentiel", 1, 3, "hotspot-mensuel", 7500, 3, "MikCloud Hotspot Mensuel"},
		{"hotspot mensuel explicite", "acc-hot", "hotspot-mensuel", 1, 2, "hotspot-mensuel", 5000, 2, "MikCloud Hotspot Mensuel"},
		{"hotspot annuel 12 mois", "acc-hot", "hotspot-annuel", 12, 0, "hotspot-annuel", 25000, 0, "MikCloud Hotspot Annuel"},
		{"hotspot annuel 24 mois (empilement tarifaire)", "acc-hot", "hotspot-annuel", 24, 0, "hotspot-annuel", 50000, 0, "MikCloud Hotspot Annuel"},
		{"legacy essentiel homenet (park 0 → plancher 1)", "acc-home", "essentiel", 1, 0, "homenet-mensuel", 1250, 1, "MikCloud HomeNet Mensuel"},
		{"homenet mensuel explicite", "acc-home", "homenet-mensuel", 1, 1, "homenet-mensuel", 1250, 1, "MikCloud HomeNet Mensuel"},
		{"legacy illimite homenet", "acc-home", "illimite", 12, 0, "homenet-annuel", 12000, 0, "MikCloud HomeNet Annuel"},
		{"homenet annuel explicite", "acc-home", "homenet-annuel", 12, 0, "homenet-annuel", 12000, 0, "MikCloud HomeNet Annuel"},
		{"essai reste hors catalogue", "acc-hot", "essai", 1, 0, "essai", 0, 1, "Essai"},
	}
	for _, c := range cas {
		sub, label, planName, amount := applySubscriptionLocked(db, c.acc, c.planID, c.months, c.slots, true)
		if sub.PlanID != c.wantPlan {
			t.Fatalf("%s : PlanID stocké = %q, attendu %q (normalisation segmentée)", c.nom, sub.PlanID, c.wantPlan)
		}
		if amount != c.wantMont {
			t.Fatalf("%s : montant = %d, attendu %d", c.nom, amount, c.wantMont)
		}
		if sub.RouterSlots != c.wantSlots {
			t.Fatalf("%s : RouterSlots = %d, attendu %d", c.nom, sub.RouterSlots, c.wantSlots)
		}
		if label != c.wantLabel {
			t.Fatalf("%s : libellé = %q, attendu %q", c.nom, label, c.wantLabel)
		}
		if sub.LastAmountFcfa != amount {
			t.Fatalf("%s : LastAmountFcfa (%d) ≠ montant appliqué (%d)", c.nom, sub.LastAmountFcfa, amount)
		}
		if c.planID != "essai" && planName == "" {
			t.Fatalf("%s : nom court de formule vide", c.nom)
		}
	}

	// Empilement : un renouvellement du MÊME plan encore actif part de la fin
	// de la période en cours (identifiant normalisé — une demande historique
	// « essentiel » empile bien sur un abonnement « hotspot-mensuel »).
	sub1, _, _, _ := applySubscriptionLocked(db, "acc-hot", "hotspot-mensuel", 1, 1, true)
	sub2, _, _, _ := applySubscriptionLocked(db, "acc-hot", "essentiel", 1, 1, true)
	end1, err1 := time.Parse(time.RFC3339, sub1.PeriodEnd)
	end2, err2 := time.Parse(time.RFC3339, sub2.PeriodEnd)
	if err1 != nil || err2 != nil {
		t.Fatalf("périodes illisibles : %v / %v", err1, err2)
	}
	if !end2.After(end1) {
		t.Fatalf("le renouvellement legacy doit S'EMILER sur la période active : fin %v ≤ fin %v", end2, end1)
	}
}

// TestSignupTrialSegmented — l'essai public est segmenté : 30 jours en mode
// HomeNet, 3 mois (~90 jours) en mode Hotspot. Vérifié bout-en-bout via
// l'inscription publique puis l'état d'abonnement renvoyé à la console.
func TestSignupTrialSegmented(t *testing.T) {
	ts := newTestServer(t)

	inscrire := func(suffix, usage string) (string, map[string]any) {
		t.Helper()
		code, out := doJSON(t, ts, "POST", "/api/auth/register", "", map[string]any{
			"username": "trial-" + suffix,
			"password": "Essai-123456",
			"name":     "Essai " + suffix,
			"email":    suffix + "@trial-mikcloud.test",
			"phone":    "22507070" + map[string]string{"home01": "11", "hot002": "22"}[suffix] + "9",
			"country":  "ci",
			"city":     "Abidjan",
			"usage":    usage,
		})
		if code != 201 {
			t.Fatalf("inscription %s : code %d, corps %v", usage, code, out)
		}
		tok, _ := out["token"].(string)
		return tok, out
	}
	dureeJours := func(tok string) float64 {
		t.Helper()
		code, out := doJSON(t, ts, "GET", "/api/subscription", tok, nil)
		if code != 200 {
			t.Fatalf("GET /api/subscription : code %d, corps %v", code, out)
		}
		sub, _ := out["subscription"].(map[string]any)
		start, _ := sub["periodStart"].(string)
		end, _ := sub["periodEnd"].(string)
		t0, e0 := time.Parse(time.RFC3339, start)
		t1, e1 := time.Parse(time.RFC3339, end)
		if e0 != nil || e1 != nil {
			t.Fatalf("périodes d'essai illisibles : %v / %v", e0, e1)
		}
		return t1.Sub(t0).Hours() / 24
	}

	tokHome, outHome := inscrire("home01", "homenet")
	if days := dureeJours(tokHome); days < 29.9 || days > 30.1 {
		t.Fatalf("essai HomeNet = %.1f jours, attendu 30", days)
	}
	if u, _ := outHome["user"].(map[string]any); u == nil || u["usage"] != "homenet" {
		t.Fatalf("la session doit transporter usage=homenet, obtenu %v", outHome["user"])
	}

	tokHot, outHot := inscrire("hot002", "hotspot")
	if days := dureeJours(tokHot); days < 89 || days > 92 {
		t.Fatalf("essai Hotspot = %.1f jours, attendu ~90 (3 mois)", days)
	}
	if u, _ := outHot["user"].(map[string]any); u == nil || u["usage"] != "hotspot" {
		t.Fatalf("la session doit transporter usage=hotspot, obtenu %v", outHot["user"])
	}

	// Catalogue filtré par mode : le compte HomeNet ne voit QUE ses formules.
	_, view := doJSON(t, ts, "GET", "/api/subscription", tokHome, nil)
	if view["usage"] != "homenet" {
		t.Fatalf("usage renvoyé = %v, attendu homenet", view["usage"])
	}
	plans, _ := view["plans"].([]any)
	if len(plans) != 2 {
		t.Fatalf("le compte homenet doit voir exactement 2 formules, obtenu %d", len(plans))
	}
	for _, p := range plans {
		if pm, _ := p.(map[string]any); pm != nil && pm["id"] == "hotspot-mensuel" {
			t.Fatal("le compte homenet ne doit PAS voir la formule hotspot-mensuel")
		}
	}
}

// TestSubscriptionPostWrongMode — garde de mode sur la demande de souscription :
// la formule d'un autre mode est refusée (400, code wrong_mode) ; celle du mode
// du compte aboutit avec le montant catalogue correspondant.
func TestSubscriptionPostWrongMode(t *testing.T) {
	ts := newTestServer(t)

	code, out := doJSON(t, ts, "POST", "/api/auth/register", "", map[string]any{
		"username": "mode-home",
		"password": "Essai-123456",
		"name":     "Maison Mode",
		"email":    "mode-home@mikcloud.test",
		"phone":    "22507070808",
		"country":  "ci",
		"city":     "Abidjan",
		"usage":    "homenet",
	})
	if code != 201 {
		t.Fatalf("inscription : code %d, corps %v", code, out)
	}
	tok, _ := out["token"].(string)

	// Formule d'un AUTRE mode → refus explicite.
	code, out = doJSON(t, ts, "POST", "/api/subscription", tok, map[string]any{"planId": "hotspot-mensuel"})
	if code != 400 {
		t.Fatalf("demande cross-mode doit être refusée : code %d, corps %v", code, out)
	}
	if ec, _ := out["code"].(string); ec != "wrong_mode" {
		t.Fatalf("code machine attendu wrong_mode, obtenu %q", ec)
	}

	// Formule du mode → demande enregistrée au tarif HomeNet (base catalogue).
	code, out = doJSON(t, ts, "POST", "/api/subscription", tok, map[string]any{"planId": "homenet-mensuel"})
	if code != 200 {
		t.Fatalf("demande homenet-mensuel : code %d, corps %v", code, out)
	}
	if base, _ := out["baseAmountFcfa"].(float64); int(base) != 1250 {
		t.Fatalf("base catalogue homenet-mensuel = %v, attendu 1250", out["baseAmountFcfa"])
	}
	if pending, _ := out["pending"].(bool); !pending {
		t.Fatal("la demande doit rester en attente d'encaissement plateforme")
	}
}
