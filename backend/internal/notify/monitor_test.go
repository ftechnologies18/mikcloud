package notify

// Tests N°74 — la restructuration du moniteur : collect() rend TOUJOURS le
// verrou (defer), la délivrance réseau a lieu HORS verrou, et une panique du
// tick ne tue pas la boucle (reprise au passage suivant).

import (
	"testing"
	"time"

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
