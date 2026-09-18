package notify

// Tests N°74 — la restructuration du moniteur : collect() rend TOUJOURS le
// verrou (defer), la délivrance réseau a lieu HORS verrou, et une panique du
// tick ne tue pas la boucle (reprise au passage suivant).

import (
	"strings"
	"testing"
	"time"

	"mikcloud/hotspot-api/internal/model"
	"mikcloud/hotspot-api/internal/store"
)

// TestMonitorTickReleasesStoreLock — deux passages consécutifs de tick() :
// si la phase collect() ne rendait pas le verrou (déverrouillage manuel de
// l'ancienne structure, perdu en cas d'early-return ou de panique), le
// second passerait en deadlock et le test mourrait au timeout.
func TestMonitorTickReleasesStoreLock(t *testing.T) {
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatalf("store.New impossible : %v", err)
	}
	svc := NewService(st)
	svc.tick()
	svc.tick() // pas de deadlock = le defer Unlock du premier a bien joué
	// Le verrou reste utilisable de l'extérieur.
	done := make(chan struct{})
	go func() {
		st.Lock()
		st.Unlock()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("le verrou du store est mort après deux ticks du moniteur")
	}
}

// TestMonitorLogsOfflineBackTransitions — N°148 : les transitions d'état
// d'un routeur agent sont journalisées dans l'activité du compte (la
// cloche) — une ligne par TRANSITION, jamais par tick. Le hors-ligne et le
// retour écrivent chacun UNE entrée ; les rappels périodiques (30 min) et
// les ticks suivants n'écrivent RIEN ; un compte désactivé n'écrit pas.
func TestMonitorLogsOfflineBackTransitions(t *testing.T) {
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatalf("store.New impossible : %v", err)
	}
	svc := NewService(st)

	st.Lock()
	st.Data().Accounts = []model.Account{{ID: "acc-mon", Name: "Cyber Moniteur", Status: "active"}}
	st.Data().Routers = []model.Router{{
		ID: "r-mon", AccountID: "acc-mon", Name: "Moniteur", Mode: "agent", Status: "online",
		LastSeen: time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339),
	}}
	st.Save()
	st.Unlock()

	svc.collect(time.Now().UTC()) // transition online → offline

	st.Lock()
	offLines, otherLines := 0, 0
	for _, a := range st.Data().Activity {
		if a.AccountID != "acc-mon" {
			continue
		}
		if a.Type == "router" && strings.Contains(a.Message, "hors ligne") {
			offLines++
		} else {
			otherLines++
		}
	}
	st.Unlock()
	if offLines != 1 {
		t.Fatalf("transition hors ligne : 1 entrée attendue, %d obtenue(s)", offLines)
	}
	if otherLines != 0 {
		t.Fatalf("la transition hors ligne doit être la SEULE écriture, %d autre(s) ligne(s) obtenue(s)", otherLines)
	}

	// Ticks suivants SANS changement d'état : rappel périodique côté canaux
	// (30 min) mais AUCUNE nouvelle ligne d'activité.
	svc.collect(time.Now().UTC())
	st.Lock()
	n := 0
	for _, a := range st.Data().Activity {
		if a.AccountID == "acc-mon" {
			n++
		}
	}
	st.Unlock()
	if n != 1 {
		t.Fatalf("un tick sans transition ne doit pas écrire (anti-bruit N°148) : %d entrée(s)", n)
	}

	// Retour en ligne (touchAgent a repassé le statut + LastSeen récent).
	st.Lock()
	st.Data().Routers[0].Status = "online"
	st.Data().Routers[0].LastSeen = model.NowISO()
	st.Save()
	st.Unlock()
	svc.collect(time.Now().UTC())

	st.Lock()
	back, off := 0, 0
	for _, a := range st.Data().Activity {
		if a.AccountID != "acc-mon" {
			continue
		}
		if strings.Contains(a.Message, "de retour en ligne") {
			back++
		}
		if strings.Contains(a.Message, "hors ligne") {
			off++
		}
	}
	st.Unlock()
	if back != 1 || off != 1 {
		t.Fatalf("après retour : 1 ligne « hors ligne » + 1 ligne « retour » attendues, obtenu %d/%d", off, back)
	}

	// État stable en ligne : plus RIEN.
	svc.collect(time.Now().UTC())
	st.Lock()
	n = 0
	for _, a := range st.Data().Activity {
		if a.AccountID == "acc-mon" {
			n++
		}
	}
	st.Unlock()
	if n != 2 {
		t.Fatalf("état stable : aucune nouvelle écriture attendue, %d entrée(s) au total", n)
	}
}
