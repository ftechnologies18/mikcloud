package notify

// Tests N°97 — alerte occupation du pool d'adresses IP : transitions
// ok → high (≥ 80 %) → full (≥ 95 %) avec anti-spam mémorisé, retour au
// calme (état purgé), silence sur capacité inconnue et comptes sans canal.

import (
	"testing"
	"time"

	"mikcloud/hotspot-api/internal/model"
	"mikcloud/hotspot-api/internal/store"
)

// seedPoolRouter — compte + routeur diagnostiqué + notifications actives
// (canal Telegram configuré) dans un store éphémère.
func seedPoolRouter(t *testing.T, cap, hosts int) (*store.Store, *Service, *model.Router) {
	t.Helper()
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatalf("store.New impossible : %v", err)
	}
	svc := NewService(st)
	st.Lock()
	db := st.Data()
	db.Accounts = append(db.Accounts, model.Account{ID: "acc-pool", Name: "Pool Corp", Status: "active"})
	db.Routers = append(db.Routers, model.Router{
		ID: "r-pool", AccountID: "acc-pool", Name: "Saturé", Mode: "agent", Status: "online",
		PoolCap: cap, PoolHosts: hosts,
	})
	cfg := store.GetOrCreateNotifSettings(db, "acc-pool")
	cfg.Enabled = true
	cfg.TelegramEnabled = true
	cfg.TelegramBotToken = "bot-token"
	cfg.TelegramChatID = "42"
	store.SetNotifSettings(db, cfg)
	st.Unlock()
	router := &db.Routers[0]
	return st, svc, router
}

// poolAlerts — extrait les alertes pool de l'outbox d'un passage.
func poolAlerts(outbox []outboxItem) []outboxItem {
	var out []outboxItem
	for _, item := range outbox {
		if item.kind == KindPoolAlert {
			out = append(out, item)
		}
	}
	return out
}

func TestPoolAlertHighThenFullThenCalm(t *testing.T) {
	st, svc, router := seedPoolRouter(t, 254, 210) // 210/254 = 82 % → high
	now := time.Now().UTC()

	// Transition ok → high : UNE alerte, message « presque plein ».
	alerts := poolAlerts(svc.collect(now))
	if len(alerts) != 1 {
		t.Fatalf("premier passage : %d alerte(s) pool, attendu 1", len(alerts))
	}
	if !containsStr(alerts[0].title, "presque plein") {
		t.Errorf("titre inattendu : %q", alerts[0].title)
	}

	// Anti-spam : même état au passage suivant → rien.
	if alerts = poolAlerts(svc.collect(now.Add(30 * time.Second))); len(alerts) != 0 {
		t.Fatalf("état inchangé : %d alerte(s), attendu 0", len(alerts))
	}

	// Montée en saturation (245/254 = 96 %) → full : UNE nouvelle alerte.
	st.Lock()
	router.PoolHosts = 245
	st.Unlock()
	alerts = poolAlerts(svc.collect(now.Add(time.Minute)))
	if len(alerts) != 1 {
		t.Fatalf("passage à full : %d alerte(s), attendu 1", len(alerts))
	}
	if !containsStr(alerts[0].title, "plein") {
		t.Errorf("titre de saturation inattendu : %q", alerts[0].title)
	}
	if !containsStr(alerts[0].body, "no more free addresses") {
		t.Errorf("le message doit citer l'erreur terrain : %q", alerts[0].body)
	}

	// Retour au calme (100/254) : état purgé, aucune alerte.
	st.Lock()
	router.PoolHosts = 100
	st.Unlock()
	if alerts = poolAlerts(svc.collect(now.Add(2 * time.Minute))); len(alerts) != 0 {
		t.Fatalf("retour au calme : %d alerte(s), attendu 0", len(alerts))
	}
	st.Lock()
	cfg := store.GetOrCreateNotifSettings(st.Data(), "acc-pool")
	if _, still := cfg.PoolAlertState[router.ID]; still {
		t.Fatal("l'état pool doit être purgé au retour au calme")
	}
	st.Unlock()

	// Re-saturation directe depuis l'état purgé : alerte à nouveau.
	st.Lock()
	router.PoolHosts = 254
	st.Unlock()
	if alerts = poolAlerts(svc.collect(now.Add(3 * time.Minute))); len(alerts) != 1 {
		t.Fatalf("re-saturation : %d alerte(s), attendu 1", len(alerts))
	}
}

func TestPoolAlertSilentWithoutCapacityOrChannel(t *testing.T) {
	// Capacité inconnue (PoolCap=0 : jamais diagnostiqué) → jamais d'alerte.
	st, svc, router := seedPoolRouter(t, 0, 500)
	if alerts := poolAlerts(svc.collect(time.Now().UTC())); len(alerts) != 0 {
		t.Fatalf("capacité inconnue : %d alerte(s), attendu 0", len(alerts))
	}

	// Capacité connue mais AUCUN canal configuré : transition détectée
	// (état mémorisé) mais rien à envoyer.
	st.Lock()
	router.PoolCap = 254
	router.PoolHosts = 245 // 96 % → full
	cfg := store.GetOrCreateNotifSettings(st.Data(), "acc-pool")
	cfg.TelegramEnabled = false
	cfg.TelegramBotToken = ""
	store.SetNotifSettings(st.Data(), cfg)
	st.Unlock()
	if alerts := poolAlerts(svc.collect(time.Now().UTC())); len(alerts) != 0 {
		t.Fatalf("sans canal : %d alerte(s), attendu 0", len(alerts))
	}
	st.Lock()
	cfg = store.GetOrCreateNotifSettings(st.Data(), "acc-pool")
	if cfg.PoolAlertState[router.ID] != "full" {
		t.Fatalf("l'état doit rester mémorisé sans canal : %v", cfg.PoolAlertState)
	}
	st.Unlock()
}

func containsStr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
