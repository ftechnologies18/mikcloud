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

// poolAutoNotifs — extrait les confirmations d'auto-réparation (kind pool_auto).
func poolAutoNotifs(outbox []outboxItem) []outboxItem {
	var out []outboxItem
	for _, item := range outbox {
		if item.kind == KindPoolAuto {
			out = append(out, item)
		}
	}
	return out
}

// TestPoolAutoRepairMarksPendingOnTransition — N°99 : à la transition de
// pression (high/full) un routeur agent dont le gérant a activé le switch
// est MARQUÉ (PoolAutoPending — le check-in filera le recyclage) et le
// gérant reçoit UNE confirmation par transition. Anti-boucle (leçon
// N°97-ter) : le flag consommé n'est JAMAIS re-posé tant que la pression
// n'est pas redescendue puis remontée.
func TestPoolAutoRepairMarksPendingOnTransition(t *testing.T) {
	st, svc, router := seedPoolRouter(t, 254, 210) // 82 % → high
	now := time.Now().UTC()
	st.Lock()
	router.PoolAuto = true // switch activé par le gérant
	st.Unlock()

	// Transition ok → high : pending posé + UNE confirmation.
	st.Lock()
	router.PoolAutoPending = false
	st.Unlock()
	items := poolAutoNotifs(svc.collect(now))
	if len(items) != 1 {
		t.Fatalf("premier passage : %d confirmation(s) auto, attendu 1", len(items))
	}
	if !containsStr(items[0].title, "auto-réparation") || !containsStr(items[0].body, "82 %") {
		t.Errorf("confirmation inattendue : %q / %q", items[0].title, items[0].body)
	}
	st.Lock()
	pending := router.PoolAutoPending
	st.Unlock()
	if !pending {
		t.Fatal("PoolAutoPending doit être posé à la transition high")
	}

	// Même pression au tick suivant : RIEN (anti-spam par transition).
	if items = poolAutoNotifs(svc.collect(now.Add(30 * time.Second))); len(items) != 0 {
		t.Fatalf("pression inchangée : %d confirmation(s), attendu 0", len(items))
	}

	// Le check-in a consommé le pending (recyclage filé) MAIS la pression
	// reste haute : le marquage ne re-part PAS (sinon ~80 commandes/heure —
	// exactement l'inondation N°97-ter).
	st.Lock()
	router.PoolAutoPending = false
	st.Unlock()
	if items = poolAutoNotifs(svc.collect(now.Add(time.Minute))); len(items) != 0 {
		t.Fatalf("pression haute après consommation : %d confirmation(s), attendu 0", len(items))
	}
	st.Lock()
	pending = router.PoolAutoPending
	st.Unlock()
	if pending {
		t.Fatal("le pending consommé ne doit PAS être re-posé tant que la pression ne redescend pas")
	}

	// Aggravation high → full : NOUVELLE transition → confirmation (le
	// pending reste éteint si le recyclage est déjà en vol — pas de file
	// doublée : une seule commande queued à la fois côté check-in).
	st.Lock()
	router.PoolHosts = 245 // 96 % → full
	st.Unlock()
	if items = poolAutoNotifs(svc.collect(now.Add(90 * time.Second))); len(items) != 1 {
		t.Fatalf("passage à full : %d confirmation(s), attendu 1", len(items))
	}
	if !containsStr(items[0].body, "no more free addresses") {
		t.Errorf("la confirmation full doit citer l'erreur terrain : %q", items[0].body)
	}

	// Retour au calme PUIS re-montée : la mémoire est purgée au calme, la
	// prochaine montée re-marque (nouveau cycle de zombies à recycler).
	st.Lock()
	router.PoolHosts = 100
	st.Unlock()
	svc.collect(now.Add(2 * time.Minute))
	st.Lock()
	router.PoolHosts = 210
	st.Unlock()
	if items = poolAutoNotifs(svc.collect(now.Add(3 * time.Minute))); len(items) != 1 {
		t.Fatalf("re-montée après calme : %d confirmation(s), attendu 1", len(items))
	}
	st.Lock()
	pending = router.PoolAutoPending
	st.Unlock()
	if !pending {
		t.Fatal("le pending doit être re-posé à la nouvelle montée")
	}
}

// TestPoolAutoRepairRequiresOptInAndAgent — N°99 : sans switch (comportement
// N°97 inchangé — le cloud n'agit jamais de son propre chef) ni en mode
// simulé, aucune marque n'est posée.
func TestPoolAutoRepairRequiresOptInAndAgent(t *testing.T) {
	// Switch OFF : pression haute, rien ne part.
	st, svc, router := seedPoolRouter(t, 254, 210)
	svc.collect(time.Now().UTC())
	st.Lock()
	pending := router.PoolAutoPending
	st.Unlock()
	if pending {
		t.Fatal("sans switch (PoolAuto=false), aucun pending ne doit être posé (doctrine N°97)")
	}

	// Mode simulé : le switch n'a personne pour exécuter la commande.
	st.Lock()
	router.PoolAuto = true
	router.Mode = "simulated"
	st.Unlock()
	if items := poolAutoNotifs(svc.collect(time.Now().UTC())); len(items) != 0 {
		t.Fatalf("mode simulé : %d confirmation(s), attendu 0", len(items))
	}
	st.Lock()
	pending = router.PoolAutoPending
	st.Unlock()
	if pending {
		t.Fatal("en mode simulé, aucun pending ne doit être posé")
	}
}

// TestPoolAutoRepairWorksWithoutChannels — N°99 : l'auto-réparation est un
// réglage DÉDIÉ du routeur : sans canal de notification, l'action a quand
// même lieu (pending posé) — mais sans confirmation envoyée.
func TestPoolAutoRepairWorksWithoutChannels(t *testing.T) {
	st, svc, router := seedPoolRouter(t, 254, 210)
	st.Lock()
	router.PoolAuto = true
	cfg := store.GetOrCreateNotifSettings(st.Data(), "acc-pool")
	cfg.TelegramEnabled = false
	cfg.TelegramBotToken = ""
	store.SetNotifSettings(st.Data(), cfg)
	st.Unlock()

	if items := poolAutoNotifs(svc.collect(time.Now().UTC())); len(items) != 0 {
		t.Fatalf("sans canal : %d confirmation(s), attendu 0", len(items))
	}
	st.Lock()
	pending := router.PoolAutoPending
	st.Unlock()
	if !pending {
		t.Fatal("sans canal, le pending doit quand même être posé (réglage dédié du routeur)")
	}
}

func containsStr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
