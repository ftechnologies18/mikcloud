package api

// Tests N°73 — purgeOldCommands : fermeture des zombies « sent » orphelins
// (rapport perdu depuis plus de 7 jours) en « error » avec message, SANS
// toucher les sent récents ni les queued ; double phase — fermé aujourd'hui,
// balayé 7 jours plus tard comme tout done/error ancien.

import (
	"testing"
	"time"

	"mikcloud/hotspot-api/internal/model"
)

// TestPurgeOldCommandsClosesAncientSentZombies — le cœur du N°73 : un
// « sent » muet depuis plus de 7 jours (vécu au réveil post-suspension du
// 10/09 : user_remove dont seul le POST /agent/result avait été perdu) est
// fermé « error » + DoneAt + message — pas supprimé, l'historique doit
// montrer la fermeture. Les sent récents (fenêtre de reprise idempotente
// 10 min puis d'observation) et les queued anciennes (un routeur muet qui
// revient les exécute et les rapporte normalement) restent intactes.
func TestPurgeOldCommandsClosesAncientSentZombies(t *testing.T) {
	db := &model.DB{}
	ancient := time.Now().UTC().Add(-8 * 24 * time.Hour).Format(time.RFC3339)
	recent := time.Now().UTC().Add(-20 * time.Minute).Format(time.RFC3339)
	db.Commands = []model.Command{
		{ID: "c-zombie", RouterID: "r1", Kind: "user_remove", Status: "sent", CreatedAt: ancient, SentAt: ancient},
		{ID: "c-volant", RouterID: "r1", Kind: "read_state", Status: "sent", CreatedAt: recent, SentAt: recent},
		{ID: "c-queue", RouterID: "r1", Kind: "user_add", Status: "queued", CreatedAt: ancient},
	}

	purgeOldCommands(db)

	byID := map[string]model.Command{}
	for _, c := range db.Commands {
		byID[c.ID] = c
	}

	z, ok := byID["c-zombie"]
	if !ok {
		t.Fatal("zombie ancien : fermé attendu (error + DoneAt), SUPPRIMÉ obtenu")
	}
	if z.Status != "error" {
		t.Fatalf("zombie ancien : status %q attendu, %q obtenu", "error", z.Status)
	}
	if z.DoneAt == "" {
		t.Fatal("zombie ancien : DoneAt attendu (double phase : visible 7 j), vide obtenu")
	}
	if msg, _ := z.Result["message"].(string); msg == "" {
		t.Fatal("zombie ancien : message de fermeture absent du résultat")
	}

	if v, ok := byID["c-volant"]; !ok || v.Status != "sent" {
		t.Fatalf("sent récent (20 min) ne doit PAS être touché : %+v", v)
	}
	if q, ok := byID["c-queue"]; !ok || q.Status != "queued" {
		t.Fatalf("queued ancienne ne doit PAS être touchée (le routeur revenu l'exécutera) : %+v", q)
	}
}

// TestPurgeOldCommandsSweepsClosedZombies — la seconde phase : un zombie
// fermé au cycle précédent (error + DoneAt de plus de 7 jours) est balayé
// par le nettoyage existant, comme tout done/error ancien.
func TestPurgeOldCommandsSweepsClosedZombies(t *testing.T) {
	db := &model.DB{}
	ancient := time.Now().UTC().Add(-8 * 24 * time.Hour).Format(time.RFC3339)
	fresh := time.Now().UTC().Add(-20 * time.Minute).Format(time.RFC3339)
	db.Commands = []model.Command{
		{ID: "c-ferme-vieux", RouterID: "r1", Kind: "user_remove", Status: "error", CreatedAt: ancient, SentAt: ancient, DoneAt: ancient,
			Result: map[string]any{"message": "rapport perdu (zombie « sent » fermé après 7 j sans retour)"}},
		{ID: "c-done-vieux", RouterID: "r1", Kind: "read_state", Status: "done", CreatedAt: ancient, SentAt: ancient, DoneAt: ancient},
		{ID: "c-ferme-neuf", RouterID: "r1", Kind: "user_set", Status: "error", CreatedAt: ancient, SentAt: fresh, DoneAt: fresh},
	}

	purgeOldCommands(db)

	for _, id := range []string{"c-ferme-vieux", "c-done-vieux"} {
		found := false
		for _, c := range db.Commands {
			if c.ID == id {
				found = true
			}
		}
		if found {
			t.Fatalf("%s (terminé de plus de 7 j) aurait dû être balayé", id)
		}
	}
	foundFresh := false
	for _, c := range db.Commands {
		if c.ID == "c-ferme-neuf" {
			foundFresh = true
		}
	}
	if !foundFresh {
		t.Fatal("error récent (DoneAt 20 min) ne doit PAS encore être balayé")
	}
}
