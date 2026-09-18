package api

// Tests N°148 — la synchro de routine quitte le journal d'activité :
// un cycle read_state COMPLET (réconciliation synced) ne produit PLUS
// aucune ligne (« Routeur «X» synchronisé ») — en production cette ligne
// tombait à chaque cycle (toutes les ~20-45 s par routeur sous attention)
// et noyait la cloche : 464 entrées/24 h sur un compte, 100 % du volume.
// Le journal ne garde que les événements : ce test en prouve le silence.

import (
	"net/url"
	"strings"
	"testing"

	"mikcloud/hotspot-api/internal/agent"
	"mikcloud/hotspot-api/internal/model"
)

// TestReadStateRoutineSilentInJournal — E2E : trois cycles read_state
// complets (mono-chunk v5) traversent le VRAI chemin HTTP du rapport agent
// → le journal d'activité du compte reste VIDE. La preuve porte sur le
// chemin complet (handleAgentResult → applyReadState), pas sur la seule
// fonction utilitaire.
func TestReadStateRoutineSilentInJournal(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	_, accID, _ := registerAccount(t, ts, "gerant-silence", "")

	const tok = "qu1et-t0ken-abcdefghijkl"
	st.Lock()
	st.Data().Routers = append(st.Data().Routers, model.Router{
		ID: "r-silent", AccountID: accID, Name: "Silence Radio", Mode: "agent", Status: "online",
		Version: "7.20 (stable)", AgentTokenHash: agent.HashToken(tok), SchedulerSec: agentFastSec,
		LastSeen: model.NowISO(),
	})
	st.Save()
	st.Unlock()

	// Trois cycles complets : check-in (le cadenceur enfile le read_state —
	// premier cycle : aucun read_state appliqué depuis le boot) puis rapport
	// v5 mono-chunk (total=2 ≤ count=500 → chunk final, réconciliation
	// complète = synced). Avant N°148, CHAQUE cycle ajoutait une ligne.
	// Les cycles suivants sont enfilés à la main : le cadenceur N°74 ne
	// re-file qu'après readStateMinInterval (2 min) — le test ne dort pas.
	for cycle := 0; cycle < 3; cycle++ {
		if cycle > 0 {
			st.Lock()
			st.Data().Commands = append(st.Data().Commands, model.Command{
				ID: "c-rs-" + string(rune('0'+cycle)), RouterID: "r-silent", AccountID: accID,
				Kind: model.CmdReadState, Status: "queued", CreatedAt: model.NowISO(),
			})
			st.Save()
			st.Unlock()
		}
		script := agentCheckIn(t, ts, tok)
		cmdID := ""
		for _, line := range strings.Split(script, "\n") {
			if strings.Contains(line, "mikcloud cmd ") && strings.Contains(line, model.CmdReadState) {
				parts := strings.Fields(strings.TrimSpace(line))
				if len(parts) >= 4 {
					cmdID = parts[3]
				}
			}
		}
		if cmdID == "" {
			t.Fatalf("cycle %d : aucune commande read_state servie au check-in", cycle)
		}
		report := url.Values{
			"status": {"ok"},
			"total":  {"2"}, "start": {"0"}, "count": {"500"}, "out": {"2"},
			"users":  {"alice|jour|false;bob|semaine|false;"},
			"stotal": {"1"}, "sessions": {"alice|10.0.0.9|00:01:00|100|50;"},
			"ifaces":  {},
			"version": {"7.20 (stable)"},
		}
		agentReport(t, ts, tok, cmdID, report)
	}

	// Le journal du compte ne contient AUCUNE entrée « synchronisé » — et
	// plus largement AUCUNE entrée du tout : la connexion du compte
	// (registerAccount) journalise sur le compte, mais les seedées ci-dessus
	// n'écrivent rien ; ne compter que les lignes nées des cycles.
	st.Lock()
	syncLines := 0
	for _, act := range st.Data().Activity {
		if act.AccountID == accID {
			if strings.Contains(act.Message, "synchronis") {
				syncLines++
			}
		}
	}
	routerSeen := false
	for _, r := range st.Data().Routers {
		if r.ID == "r-silent" {
			routerSeen = r.HotspotUsers == 2 && r.ActiveSessions == 1
		}
	}
	st.Unlock()

	if syncLines != 0 {
		t.Fatalf("3 cycles read_state complets ont produit %d ligne(s) « synchronisé » — la synchro de routine doit être SILENCIEUSE (N°148)", syncLines)
	}
	if !routerSeen {
		t.Fatal("la télémétrie doit continuer à s'appliquer (parc = 2, sessions actives = 1) — le silence du journal ne doit pas geler les compteurs")
	}
}
