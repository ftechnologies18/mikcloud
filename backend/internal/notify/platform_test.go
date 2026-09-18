// N°150 — tests du canal plateforme : bot Telegram FTCI (token de package)
// et relais e-mail du compte principal. AUCUN réseau réel : les endpoints
// sont remplacés par des serveurs httptest locaux.
package notify

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"mikcloud/hotspot-api/internal/model"
)

// stubTelegramAPI — serveur qui enregistre chaque sendMessage (token du path,
// chat_id et texte du corps).
func stubTelegramAPI(t *testing.T) (*httptest.Server, *[]sendCall) {
	t.Helper()
	var calls []sendCall
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		_ = json.NewDecoder(r.Body).Decode(&payload)
		mu.Lock()
		calls = append(calls, sendCall{
			url:    r.URL.Path,
			chatID: payload["chat_id"].(string),
			text:   payload["text"].(string),
		})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)
	old := telegramEndpoint
	telegramEndpoint = srv.URL
	t.Cleanup(func() { telegramEndpoint = old })
	return srv, &calls
}

type sendCall struct {
	url    string
	chatID string
	text   string
}

// withPlatformToken — pose le token du bot FTCI pour la durée du test.
func withPlatformToken(t *testing.T, token string) {
	t.Helper()
	old := TelegramPlatformToken
	TelegramPlatformToken = token
	t.Cleanup(func() { TelegramPlatformToken = old })
}

// TestDeliverTelegramPlatformToken — un compte SANS bot propre (champ vide)
// mais avec chat ID envoie via le BOT PLATEFORME ; la trace reste au compte.
func TestDeliverTelegramPlatformToken(t *testing.T) {
	_, calls := stubTelegramAPI(t)
	withPlatformToken(t, "777:FTCI")

	cfg := model.NotificationSettings{
		AccountID:       "acc-client",
		TelegramEnabled: true,
		TelegramChatID:  "424242",
		// TelegramBotToken vide : c'est le bot plateforme qui porte l'envoi.
	}
	logs := Deliver(&cfg, KindTest, "Titre", "Corps", "telegram")
	if len(logs) != 1 || logs[0].Status != "sent" {
		t.Fatalf("envoi plateforme attendu (1 log sent), obtenu %+v", logs)
	}
	if logs[0].AccountID != "acc-client" {
		t.Errorf("la trace doit rester au compte émetteur, obtenu %q", logs[0].AccountID)
	}
	if len(*calls) != 1 {
		t.Fatalf("1 sendMessage attendu, obtenu %d", len(*calls))
	}
	c := (*calls)[0]
	if !strings.Contains(c.url, "/bot777:FTCI/sendMessage") {
		t.Errorf("l'envoi doit partir du token plateforme, path=%q", c.url)
	}
	if c.chatID != "424242" || !strings.Contains(c.text, "Titre") {
		t.Errorf("chat/text incorrects : %+v", c)
	}
}

// TestDeliverTelegramOwnTokenWins — le bot PROPRE du compte reste prioritaire
// sur le bot plateforme (BYO d'abord).
func TestDeliverTelegramOwnTokenWins(t *testing.T) {
	_, calls := stubTelegramAPI(t)
	withPlatformToken(t, "777:FTCI")

	cfg := model.NotificationSettings{
		AccountID:        "acc-client",
		TelegramEnabled:  true,
		TelegramChatID:   "424242",
		TelegramBotToken: "111:BYO",
	}
	Deliver(&cfg, KindTest, "T", "B", "telegram")
	if len(*calls) != 1 || !strings.Contains((*calls)[0].url, "/bot111:BYO/sendMessage") {
		t.Fatalf("le bot du compte doit gagner, obtenu %+v", *calls)
	}
}

// TestConfiguredWithPlatformMatrix — gardes étendues : bot FTCI pour
// Telegram, relais principal pour l'e-mail.
func TestConfiguredWithPlatformMatrix(t *testing.T) {
	withPlatformToken(t, "777:FTCI")
	platform := &model.NotificationSettings{EmailProvider: "resend", ResendAPIKey: "re_main"}

	tg := model.NotificationSettings{TelegramEnabled: true, TelegramChatID: "1"}
	if !ConfiguredWithPlatform(&tg, nil, "telegram") {
		t.Error("telegram sans bot propre + plateforme → configuré")
	}
	TelegramPlatformToken = ""
	if ConfiguredWithPlatform(&tg, nil, "telegram") {
		t.Error("telegram sans bot propre ni plateforme → NON configuré")
	}
	TelegramPlatformToken = "777:FTCI"

	mail := model.NotificationSettings{EmailEnabled: true, EmailTo: "gerant@example.ci"}
	if !ConfiguredWithPlatform(&mail, platform, "email") {
		t.Error("email sans identifiants propres + relais principal → configuré")
	}
	if ConfiguredWithPlatform(&mail, nil, "email") {
		t.Error("email sans identifiants ni relais → NON configuré")
	}
	if ConfiguredWithPlatform(&model.NotificationSettings{EmailEnabled: true}, platform, "email") {
		t.Error("email sans destinataire → NON configuré")
	}
	if !HasAnyChannelWithPlatform(&mail, platform) {
		t.Error("HasAnyChannelWithPlatform doit voir le relais")
	}
	if HasAnyChannelWithPlatform(&mail, nil) {
		t.Error("sans relais ni autres canaux → false")
	}
}

// TestDeliverEmailPlatformRelay — le relais du compte principal porte l'envoi
// (clé Resend du principal), le destinataire ET la trace restent au compte.
func TestDeliverEmailPlatformRelay(t *testing.T) {
	var mu sync.Mutex
	var bodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		_ = json.NewDecoder(r.Body).Decode(&payload)
		mu.Lock()
		bodies = append(bodies, payload)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"email-1"}`))
	}))
	t.Cleanup(srv.Close)
	old := resendEndpoint
	resendEndpoint = srv.URL
	t.Cleanup(func() { resendEndpoint = old })

	platform := &model.NotificationSettings{
		AccountID:     "acc-main",
		EmailProvider: "resend",
		ResendAPIKey:  "re_main",
		ResendFrom:    "MikCloud <alertes@ftci.fr>",
	}
	cfg := model.NotificationSettings{
		AccountID:    "acc-client",
		EmailEnabled: true,
		EmailTo:      "gerant@example.ci",
		// aucun identifiant propre : SMTP absent, Resend absent
	}
	logs := DeliverWithPlatform(&cfg, platform, KindTest, "Alerte", "Corps", "email")
	if len(logs) != 1 || logs[0].Status != "sent" {
		t.Fatalf("envoi relais attendu, obtenu %+v", logs)
	}
	if logs[0].AccountID != "acc-client" || logs[0].Channel != "email" {
		t.Errorf("trace au compte émetteur attendue, obtenu %+v", logs[0])
	}
	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 1 {
		t.Fatalf("1 appel Resend attendu, obtenu %d", len(bodies))
	}
	toList, _ := bodies[0]["to"].([]any)
	if len(toList) != 1 || toList[0] != "gerant@example.ci" {
		t.Errorf("destinataire du compte attendu, obtenu %v", bodies[0]["to"])
	}
}

// TestTelegramGetMeSetWebhook — les helpers du bootstrap parlent le protocol
// Bot API (ok/result.username, secret_token au setWebhook).
func TestTelegramGetMeSetWebhook(t *testing.T) {
	var gotSecret, gotURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/getMe"):
			_, _ = w.Write([]byte(`{"ok":true,"result":{"username":"MikCloudAlertesBot"}}`))
		case strings.HasSuffix(r.URL.Path, "/setWebhook"):
			var payload map[string]any
			_ = json.NewDecoder(r.Body).Decode(&payload)
			gotURL, _ = payload["url"].(string)
			gotSecret, _ = payload["secret_token"].(string)
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	old := telegramEndpoint
	telegramEndpoint = srv.URL
	t.Cleanup(func() { telegramEndpoint = old })

	me, err := TelegramGetMe("777:FTCI")
	if err != nil || me != "MikCloudAlertesBot" {
		t.Fatalf("getMe : %q, %v", me, err)
	}
	if err := TelegramSetWebhook("777:FTCI", "https://exemple.ci/api/webhooks/telegram", "s3cret"); err != nil {
		t.Fatalf("setWebhook : %v", err)
	}
	if gotURL != "https://exemple.ci/api/webhooks/telegram" || gotSecret != "s3cret" {
		t.Errorf("setWebhook params incorrects : %q %q", gotURL, gotSecret)
	}
}
