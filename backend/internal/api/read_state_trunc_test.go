package api

// Tests N°75 — rapport read_state TRONQUÉ : le script agent borne le rapport
// à 500 users / 250 sessions actives (limite anti-payload du POST RouterOS).
// Au-delà, la liste rapportée est incomplète : le cloud ne doit RIEN
// déduire des absents — ni badge « absent du routeur » (faux positifs massifs
// dès 151 utilisateurs avec l'ancienne borne v2), ni diff sessions (logouts
// en cascade puis ré-apparitions au rapport suivant).

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"mikcloud/hotspot-api/internal/agent"
	"mikcloud/hotspot-api/internal/model"
)

// truncSeed — état de base : un routeur agent, deux users cloud actifs
// (anciens — hors grâce de 2 min), l'un déjà badgé « absent », l'autre pas,
// et une session live précédente.
func truncSeed() (*model.DB, *model.Router) {
	old := time.Now().UTC().Add(-1 * time.Hour).Format(time.RFC3339)
	db := &model.DB{}
	db.Routers = []model.Router{{ID: "r-tr", AccountID: "acc", Name: "TR", Mode: "agent", Status: "online", HotspotUsers: 320, ActiveSessions: 180}}
	db.HotspotUsers = []model.HotspotUser{
		{ID: "u-1", AccountID: "acc", RouterID: "r-tr", Username: "alice", Status: "active", CreatedAt: old, MissingOnRouter: false},
		{ID: "u-2", AccountID: "acc", RouterID: "r-tr", Username: "bob", Status: "active", CreatedAt: old, MissingOnRouter: true},
	}
	db.Sessions = []model.Session{{ID: "s-prev", AccountID: "acc", RouterID: "r-tr", Username: "alice", StartedAt: old}}
	return db, &db.Routers[0]
}

// TestApplyReadStateTruncatedDefersDeductions — rapport tronqué : AUCUNE
// déduction sur les absents. Les badges restent en l'état (ni posés pour
// alice absente de la liste, ni levés/déposés pour bob), la session
// précédente est conservée (pas de logout en cascade), les compteurs
// HotspotUsers/ActiveSessions gardent leur dernière valeur honnête.
func TestApplyReadStateTruncatedDefersDeductions(t *testing.T) {
	db, router := truncSeed()
	vals := url.Values{}
	vals.Set("users", "carol|default|false;") // alice et bob absents de la LISTE (pas du routeur)
	vals.Set("sessions", "")
	vals.Set("trunc", "true")

	(&API{}).applyReadState(db, router, vals)

	byName := map[string]model.HotspotUser{}
	for _, u := range db.HotspotUsers {
		byName[u.Username] = u
	}
	if byName["alice"].MissingOnRouter {
		t.Fatal("rapport tronqué : alice ne doit PAS être badgée absente (absente de la liste ≠ absente du routeur)")
	}
	if !byName["bob"].MissingOnRouter {
		t.Fatal("rapport tronqué : le badge préexistant de bob doit rester posé (pas de levée/déduction)")
	}
	if len(db.Sessions) != 1 || db.Sessions[0].ID != "s-prev" {
		t.Fatalf("rapport tronqué : la session précédente doit être conservée tel quel, obtenu %v", db.Sessions)
	}
	if router.HotspotUsers != 320 {
		t.Fatalf("rapport tronqué : compteur HotspotUsers doit garder sa valeur (320), obtenu %d", router.HotspotUsers)
	}
	if router.ActiveSessions != 180 {
		t.Fatalf("rapport tronqué : compteur ActiveSessions doit garder sa valeur (180), obtenu %d", router.ActiveSessions)
	}
	for _, l := range db.UserLogs {
		if l.Action == "logout" {
			t.Fatalf("rapport tronqué : aucun logout ne doit être déduit, obtenu %+v", l)
		}
	}
}

// TestApplyReadStateCompleteStillDeduces — rapport complet (pas de trunc) :
// le comportement historique est intact — alice absente du rapport devient
// MissingOnRouter, la session précédente est terminée (logout journalisé),
// les compteurs suivent le rapport.
func TestApplyReadStateCompleteStillDeduces(t *testing.T) {
	db, router := truncSeed()
	vals := url.Values{}
	vals.Set("users", "carol|default|false;")
	vals.Set("sessions", "")

	(&API{}).applyReadState(db, router, vals)

	byName := map[string]model.HotspotUser{}
	for _, u := range db.HotspotUsers {
		byName[u.Username] = u
	}
	if !byName["alice"].MissingOnRouter {
		t.Fatal("rapport complet : alice absente du rapport DOIT être badgée absente (comportement historique)")
	}
	if len(db.Sessions) != 0 {
		t.Fatalf("rapport complet : la session précédente doit être remplacée, obtenu %v", db.Sessions)
	}
	if router.HotspotUsers != 1 {
		t.Fatalf("rapport complet : compteur HotspotUsers = 1 attendu, obtenu %d", router.HotspotUsers)
	}
	logouts := 0
	for _, l := range db.UserLogs {
		if l.Action == "logout" {
			logouts++
		}
	}
	if logouts != 1 {
		t.Fatalf("rapport complet : 1 logout attendu (fin de session alice), obtenu %d", logouts)
	}
}

// TestBuildReadStateReportsTruncFlag — le script généré (v5, N°76) porte la
// pagination : fenêtre [start, start+count) sur l'index du parc, total exact,
// sessions RÉSERVÉES au chunk final, stotal partout. Le cloud et le script
// restent synchrones sur le protocole v5.
func TestBuildReadStateReportsTruncFlag(t *testing.T) {
	b := agent.Builder{BaseURL: "https://cloud.example", Token: "tok"}
	script, err := b.ScriptFor(model.Command{ID: "c-rs", Kind: model.CmdReadState})
	if err != nil {
		t.Fatalf("script read_state : %v", err)
	}
	for _, want := range []string{
		"$n >= 0 && $n < 500",                               // fenêtre de base
		":local mikTotal [:len $mikIds]",                    // total exact du parc
		":if (500 < $mikTotal) do={ :set rtrunc \"true\" }", // trunc = il reste des chunks
		":if (500 >= $mikTotal) do={",                       // sessions au chunk final seulement
		"&total=\". $mikTotal .\"&start=0&count=500",        // fenêtre rapportée
		"&stotal=\". $rstotal",                              // total sessions partout
		"&trunc=\". $rtrunc",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("script read_state v5 : fragment %q absent du script généré", want)
		}
	}
	// Chunk non-base : la fenêtre est inlinée depuis le payload.
	chunk, err := b.ScriptFor(model.Command{ID: "c-rs2", Kind: model.CmdReadState,
		Payload: map[string]any{"start": 500, "count": 500}})
	if err != nil {
		t.Fatalf("script chunk : %v", err)
	}
	for _, want := range []string{
		"$n >= 500 && $n < 1000",
		"&start=500&count=500",
	} {
		if !strings.Contains(chunk, want) {
			t.Fatalf("script chunk v5 : fragment %q absent", want)
		}
	}
	if strings.Contains(script, "$rn < 150") || strings.Contains(script, "$rsn < 100") {
		t.Fatal("script read_state v5 : les anciennes bornes 150/100 ne doivent plus apparaître")
	}
}
