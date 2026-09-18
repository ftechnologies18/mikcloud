// N°150 — tests E2E du pairage Telegram plateforme (lien magique) et du
// relais e-mail : pair-code → webhook Telegram (secret header, /start code)
// → réglages du compte ; relais principal pour les tests d'envoi e-mail.
// AUCUN réseau réel : telegramGetMeFn/telegramSendFn stubbés, Resend stubbé
// par variable d'endpoint (notify).
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"mikcloud/hotspot-api/internal/model"
	"mikcloud/hotspot-api/internal/notify"
	"mikcloud/hotspot-api/internal/store"
)

// tgCapture — enregistre les messages envoyés par le bot (chat, texte).
type tgCapture struct {
	mu    sync.Mutex
	calls []struct{ chatID, text string }
}

func (c *tgCapture) add(chatID, text string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls = append(c.calls, struct{ chatID, text string }{chatID, text})
}

func (c *tgCapture) snapshot() []struct{ chatID, text string } {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]struct{ chatID, text string }, len(c.calls))
	copy(out, c.calls)
	return out
}

// newPairServer — serveur + store + stubs Telegram (getMe/send) + token du
// bot plateforme posé, restaurés à la fin du test.
func newPairServer(t *testing.T, cap *tgCapture) (*httptest.Server, *store.Store) {
	t.Helper()
	t.Setenv("DATABASE_URL", "")
	t.Setenv("ADMIN_PASSWORD", "admin-test-1234")
	t.Setenv("ADMIN_USERNAME", "")
	t.Setenv("ALLOWED_ORIGIN", "")
	t.Setenv("TELEGRAM_WEBHOOK_SECRET", "tg-secret-test")
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatalf("store.New impossible : %v", err)
	}
	ts := startTestServer(st)
	t.Cleanup(ts.Close)

	oldTok := notify.TelegramPlatformToken
	notify.TelegramPlatformToken = "777:FTCI"
	t.Cleanup(func() { notify.TelegramPlatformToken = oldTok })

	oldGetMe, oldSend := telegramGetMeFn, telegramSendFn
	telegramGetMeFn = func(string) (string, error) { return "MikCloudAlertesBot", nil }
	if cap != nil {
		telegramSendFn = func(_, chatID, text string) error {
			cap.add(chatID, text)
			return nil
		}
	} else {
		telegramSendFn = func(_, _, _ string) error { return nil }
	}
	t.Cleanup(func() { telegramGetMeFn, telegramSendFn = oldGetMe, oldSend })
	return ts, st
}

// postTelegramUpdate — simule le serveur Telegram : POST webhook avec le
// secret d'en-tête (ou un autre si erroné).
func postTelegramUpdate(t *testing.T, ts *httptest.Server, secret, text string, chatID int64) (int, map[string]any) {
	t.Helper()
	body := map[string]any{
		"update_id": 1,
		"message":   map[string]any{"chat": map[string]any{"id": chatID}, "text": text},
	}
	b, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/webhooks/telegram", strings.NewReader(string(b)))
	if err != nil {
		t.Fatalf("requête impossible : %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Telegram-Bot-Api-Secret-Token", secret)
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("webhook : %v", err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

// TestTelegramPairFlow — parcours complet : pair-code → webhook (mauvais
// secret 401, bon secret + /start CODE → liaison + confirmation) → statut
// linked → code à usage unique → réglages visibles à la console.
func TestTelegramPairFlow(t *testing.T) {
	cap := &tgCapture{}
	ts, _ := newPairServer(t, cap)
	token, accID, _ := registerAccount(t, ts, "gerant-tgpair", "")

	// 1. pair-code : lien magique avec le @username du bot.
	status, out := doJSON(t, ts, "POST", "/api/notifications/telegram/pair-code", token, map[string]string{})
	if status != http.StatusOK {
		t.Fatalf("pair-code : statut %d, corps %v", status, out)
	}
	code, _ := out["code"].(string)
	url, _ := out["url"].(string)
	if code == "" || len(code) != 8 {
		t.Fatalf("code éphémère attendu (8 car.), obtenu %q", code)
	}
	if url != "https://t.me/MikCloudAlertesBot?start="+code {
		t.Errorf("lien magique incorrect : %q", url)
	}

	// 2. Statut avant démarrage : pending.
	status, out = doJSON(t, ts, "GET", "/api/notifications/telegram/pair-status?code="+code, token, nil)
	if status != http.StatusOK || out["status"] != "pending" {
		t.Fatalf("statut pending attendu, obtenu %d %v", status, out)
	}

	// 3. Webhook avec MAUVAIS secret → 401, rien n'est écrit.
	if status, _ := postTelegramUpdate(t, ts, "mauvais-secret", "/start "+code, 987654321); status != http.StatusUnauthorized {
		t.Fatalf("mauvais secret → 401 attendu, obtenu %d", status)
	}
	status, _ = doJSON(t, ts, "GET", "/api/notifications", token, nil)
	if status != http.StatusOK {
		t.Fatalf("GET notifications : %d", status)
	}

	// 4. Webhook authentifié : /start <code> depuis le chat 424242.
	if status, out := postTelegramUpdate(t, ts, "tg-secret-test", "/start "+code, 424242); status != http.StatusOK {
		t.Fatalf("webhook valide → 200 attendu, obtenu %d %v", status, out)
	}

	// 5. Les réglages du compte portent le chat ID + canal activé.
	status, out = doJSON(t, ts, "GET", "/api/notifications", token, nil)
	if status != http.StatusOK {
		t.Fatalf("GET notifications : %d", status)
	}
	if out["telegramChatId"] != "424242" {
		t.Errorf("chat ID non enregistré : %v", out["telegramChatId"])
	}
	if out["telegramEnabled"] != true {
		t.Errorf("canal Telegram doit être activé par le pairage : %v", out["telegramEnabled"])
	}
	if out["telegramPlatformAvailable"] != true {
		t.Errorf("telegramPlatformAvailable attendu (token posé) : %v", out["telegramPlatformAvailable"])
	}
	if out["telegramBotUsername"] != "MikCloudAlertesBot" {
		t.Errorf("telegramBotUsername attendu : %v", out["telegramBotUsername"])
	}

	// 6. Statut : linked avec le chat ID.
	status, out = doJSON(t, ts, "GET", "/api/notifications/telegram/pair-status?code="+code, token, nil)
	if status != http.StatusOK || out["status"] != "linked" || out["chatId"] != "424242" {
		t.Fatalf("statut linked attendu, obtenu %d %v", status, out)
	}

	// 7. Confirmation envoyée au bon chat par le bot.
	calls := cap.snapshot()
	if len(calls) != 1 || calls[0].chatID != "424242" || !strings.Contains(calls[0].text, "Connecté") {
		t.Fatalf("confirmation de pairage attendue au chat 424242, obtenu %+v", calls)
	}

	// 8. Code à usage unique : rejouer le /start ne lie rien d'autre.
	if status, _ := postTelegramUpdate(t, ts, "tg-secret-test", "/start "+code, 111222333); status != http.StatusOK {
		t.Fatalf("rejeu du code → 200 quand même (Telegram ne doit pas ressiner), obtenu %d", status)
	}
	status, out = doJSON(t, ts, "GET", "/api/notifications", token, nil)
	if out["telegramChatId"] != "424242" {
		t.Errorf("le rejeu ne doit PAS écraser le chat ID : %v", out["telegramChatId"])
	}
	// Le rejeu a reçu la réponse d'aide (best-effort), pas la confirmation.
	if calls := cap.snapshot(); len(calls) != 2 || strings.Contains(calls[1].text, "Connecté") {
		t.Errorf("rejeu → réponse d'aide attendue, obtenu %+v", calls)
	}

	_ = accID
}

// TestTelegramPairExpiry — code expiré : le webhook répond 200 (politesse
// Telegram) mais n'écrit RIEN.
func TestTelegramPairExpiry(t *testing.T) {
	cap := &tgCapture{}
	ts, _ := newPairServer(t, cap)
	token, _, _ := registerAccount(t, ts, "gerant-tgexp", "")

	oldTTL := telegramPairTTL
	telegramPairTTL = 5 * time.Millisecond
	t.Cleanup(func() { telegramPairTTL = oldTTL })

	status, out := doJSON(t, ts, "POST", "/api/notifications/telegram/pair-code", token, map[string]string{})
	if status != http.StatusOK {
		t.Fatalf("pair-code : %d %v", status, out)
	}
	code, _ := out["code"].(string)
	time.Sleep(20 * time.Millisecond)

	if status, _ := postTelegramUpdate(t, ts, "tg-secret-test", "/start "+code, 555); status != http.StatusOK {
		t.Fatalf("code expiré → 200 quand même attendu, obtenu %d", status)
	}
	status, out = doJSON(t, ts, "GET", "/api/notifications", token, nil)
	if out["telegramChatId"] != "" {
		t.Errorf("aucun chat ID ne doit être posé sur un code expiré : %v", out["telegramChatId"])
	}
	// Réponse d'aide envoyée (pas de confirmation).
	if calls := cap.snapshot(); len(calls) != 1 || strings.Contains(calls[0].text, "Connecté") {
		t.Errorf("réponse d'aide attendue pour code expiré, obtenu %+v", calls)
	}
}

// TestTelegramWebhookDisabled — sans TELEGRAM_WEBHOOK_SECRET, le webhook est
// fermé (503, discipline Wave).
func TestTelegramWebhookDisabled(t *testing.T) {
	ts, _ := newPairServer(t, nil)
	t.Setenv("TELEGRAM_WEBHOOK_SECRET", "")
	if status, _ := postTelegramUpdate(t, ts, "nimporte", "/start ABCD2345", 42); status != http.StatusServiceUnavailable {
		t.Fatalf("sans secret d'env → 503 attendu, obtenu %d", status)
	}
}

// TestTelegramPairCodeNeedsPlatform — sans token de bot plateforme, le
// pair-code refuse (503) : la console BYO reste le seul chemin.
func TestTelegramPairCodeNeedsPlatform(t *testing.T) {
	ts, _ := newPairServer(t, nil)
	notify.TelegramPlatformToken = ""
	token, _, _ := registerAccount(t, ts, "gerant-tgoff", "")
	status, out := doJSON(t, ts, "POST", "/api/notifications/telegram/pair-code", token, map[string]string{})
	if status != http.StatusServiceUnavailable {
		t.Fatalf("sans bot plateforme → 503 attendu, obtenu %d %v", status, out)
	}
}

// TestNotifTestEmailViaRelay — test d'envoi e-mail par un compte SANS
// identifiants propres : la garde s'ouvre grâce au relais du compte principal
// (emailPlatformRelay dans la vue), l'envoi reçoit les réglages du principal
// en argument, la trace reste au compte. L'envoi réel via relais est couvert
// par notify.TestDeliverEmailPlatformRelay.
func TestNotifTestEmailViaRelay(t *testing.T) {
	ts, st := newPairServer(t, nil)
	notify.TelegramPlatformToken = "" // sans bot plateforme : la vue reste BYO

	// Compte principal porteur du relais Resend.
	st.Lock()
	db := st.Data()
	store.SetNotifSettings(db, model.NotificationSettings{
		AccountID:     model.AccountMainID,
		EmailProvider: "resend",
		ResendAPIKey:  "re_main",
		ResendFrom:    "MikCloud <alertes@ftci.fr>",
	})
	st.Save()
	st.Unlock()

	// Capture de l'envoi (deliverNotif stubbé) — vérifie l'argument relais.
	var mu sync.Mutex
	var gotPlatformKey string
	oldDeliver := deliverNotif
	deliverNotif = func(cfg *model.NotificationSettings, platformEmail *model.NotificationSettings, _, _, _, _ string) []model.NotificationLog {
		mu.Lock()
		if platformEmail != nil {
			gotPlatformKey = platformEmail.ResendAPIKey
		}
		mu.Unlock()
		return []model.NotificationLog{notify.LogEntry(cfg, "email", notify.KindTest, "MikCloud — Test de notification", "", nil)}
	}
	t.Cleanup(func() { deliverNotif = oldDeliver })

	token, accID, _ := registerAccount(t, ts, "gerant-relais", "")

	// Le compte active l'e-mail avec juste un destinataire (aucun SMTP/Resend).
	status, out := doJSON(t, ts, "PUT", "/api/notifications", token, map[string]any{
		"enabled":      true,
		"emailEnabled": true,
		"emailTo":      "gerant-relais@example.ci",
	})
	if status != http.StatusOK {
		t.Fatalf("PUT notifications : %d %v", status, out)
	}
	if out["emailPlatformRelay"] != true {
		t.Errorf("emailPlatformRelay attendu dans la vue : %v", out["emailPlatformRelay"])
	}
	if out["emailProvider"] != "smtp" {
		t.Errorf("fournisseur du compte inchangé (smtp par défaut) : %v", out["emailProvider"])
	}

	// Sans relais connu du handler, la garde aurait renvoyé 400 — ici 200.
	status, out = doJSON(t, ts, "POST", "/api/notifications/test", token, map[string]string{"channel": "email"})
	if status != http.StatusOK {
		t.Fatalf("test e-mail via relais → 200 attendu, obtenu %d %v", status, out)
	}
	mu.Lock()
	if gotPlatformKey != "re_main" {
		t.Errorf("l'envoi doit recevoir les identifiants du compte principal, obtenu %q", gotPlatformKey)
	}
	mu.Unlock()

	// Historique : trace « sent » AU COMPTE du gérant (pas au compte principal).
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/notifications/log", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("log : %v", err)
	}
	defer resp.Body.Close()
	var entries []map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&entries)
	if len(entries) != 1 || entries[0]["status"] != "sent" || entries[0]["channel"] != "email" {
		t.Fatalf("une trace sent/email au compte attendue, obtenu %v", entries)
	}
	if entries[0]["accountId"] != accID {
		t.Errorf("la trace doit rester au compte émetteur %s, obtenu %v", accID, entries[0]["accountId"])
	}
}
