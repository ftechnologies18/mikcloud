// Tests du package notify — constructeurs purs et décision de canal
// UNIQUEMENT. AUCUN envoi réseau réel (Telegram/WhatsApp/SMTP ne sont jamais
// appelés : Deliver est testé uniquement avec AUCUN canal configuré, ce qui
// court-circuite tous les sendeurs).
//
// Couverture :
//   - Configured / HasAnyChannel : conditions exactes d'activation par canal
//     (champs requis, canal inconnu) ;
//   - Deliver sans canal : entrée « system » explicite avec l'erreur
//     « aucun canal configuré » (traçabilité de l'historique) ;
//   - logEntry : statut sent/error + message d'erreur ;
//   - buildMessage : MIME texte, sujet encodé B-UTF-8 (accents), CRLF ;
//   - helpers de mise en forme du moniteur : currencyLabel, formatDuration,
//     strconvI, stockMessage, buildDailyReport (données fixes, heure fixe).
package notify

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"mikcloud/hotspot-api/internal/model"
)

func TestConfigured(t *testing.T) {
	tg := &model.NotificationSettings{TelegramEnabled: true, TelegramBotToken: "tok", TelegramChatID: "42"}
	if !Configured(tg, "telegram") {
		t.Fatal("telegram complet doit être configuré")
	}
	sansChat := &model.NotificationSettings{TelegramEnabled: true, TelegramBotToken: "tok"}
	if Configured(sansChat, "telegram") {
		t.Fatal("telegram sans chat_id ne doit pas être configuré")
	}
	wa := &model.NotificationSettings{WhatsAppEnabled: true, WhatsAppToken: "t", WhatsAppPhoneID: "p", WhatsAppTo: "22507000000"}
	if !Configured(wa, "whatsapp") {
		t.Fatal("whatsapp complet doit être configuré")
	}
	if Configured(&model.NotificationSettings{WhatsAppEnabled: true, WhatsAppToken: "t", WhatsAppPhoneID: "p"}, "whatsapp") {
		t.Fatal("whatsapp sans destinataire ne doit pas être configuré")
	}
	mail := &model.NotificationSettings{EmailEnabled: true, SMTPHost: "smtp.example.ci", EmailTo: "a@b.ci"}
	if !Configured(mail, "email") {
		t.Fatal("email complet doit être configuré")
	}
	if Configured(&model.NotificationSettings{EmailEnabled: true, EmailTo: "a@b.ci"}, "email") {
		t.Fatal("email sans hôte SMTP ne doit pas être configuré")
	}
	if Configured(tg, "sms") {
		t.Fatal("un canal inconnu ne doit jamais être configuré")
	}
	if !HasAnyChannel(tg) {
		t.Fatal("au moins un canal configuré → HasAnyChannel vrai")
	}
	if HasAnyChannel(&model.NotificationSettings{Enabled: true}) {
		t.Fatal("interrupteur général seul : aucun canal exploitable")
	}
}

func TestDeliverWithoutAnyChannel(t *testing.T) {
	// Aucun canal configuré : AUCUN envoi réseau n'a lieu — l'appel retourne
	// une entrée « system » explicite pour l'historique.
	cfg := &model.NotificationSettings{AccountID: "acc-n", Enabled: true}
	logs := Deliver(cfg, KindRouterOffline, "Titre", "Corps", "")
	if len(logs) != 1 {
		t.Fatalf("une seule entrée d'historique attendue, obtenu %d", len(logs))
	}
	e := logs[0]
	if e.Channel != "system" || e.Status != "error" || e.Error != "aucun canal configuré" {
		t.Fatalf("entrée system attendue, obtenue %+v", e)
	}
	if e.Kind != KindRouterOffline || e.Title != "Titre" || e.Body != "Corps" || e.AccountID != "acc-n" {
		t.Fatalf("métadonnées non reportées : %+v", e)
	}
}

func TestLogEntryStatus(t *testing.T) {
	cfg := &model.NotificationSettings{AccountID: "acc-1"}
	ok := logEntry(cfg, "telegram", KindTest, "T", "B", nil)
	if ok.Status != "sent" || ok.Error != "" {
		t.Fatalf("envoi sans erreur → sent, obtenu %+v", ok)
	}
	ko := logEntry(cfg, "email", KindTest, "T", "B", errTest{})
	if ko.Status != "error" || ko.Error != "erreur simulée" {
		t.Fatalf("envoi en échec → error + message, obtenu %+v", ko)
	}
}

type errTest struct{}

func (errTest) Error() string { return "erreur simulée" }

func TestBuildMessage(t *testing.T) {
	msg := string(buildMessage("from@example.ci", "to@example.ci", "Routeur hors ligne", "Le routeur A ne répond plus."))
	for _, attendu := range []string{
		"From: MikCloud <from@example.ci>",
		"To: <to@example.ci>",
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"Subject: =?UTF-8?B?",
	} {
		if !strings.Contains(msg, attendu) {
			t.Fatalf("en-tête absent : %q", attendu)
		}
	}
	// Le sujet B-UTF-8 décode vers le titre exact (accents conservés).
	if !strings.HasPrefix(msg, "From: ") {
		t.Fatal("le message doit commencer par From")
	}
	debut := strings.Index(msg, "Subject: =?UTF-8?B?") + len("Subject: =?UTF-8?B?")
	fin := strings.Index(msg[debut:], "?=") + debut
	subject, err := base64.StdEncoding.DecodeString(msg[debut:fin])
	if err != nil {
		t.Fatalf("sujet non décodable : %v", err)
	}
	if string(subject) != "Routeur hors ligne" {
		t.Fatalf("sujet décodé = %q, attendu %q", subject, "Routeur hors ligne")
	}
	// Corps en fin de message après la ligne vide.
	if !strings.HasSuffix(msg, "Routeur hors ligne\n\nLe routeur A ne répond plus.\n") {
		t.Fatalf("corps du message incorrect : %q", msg)
	}
}

// ---------------------------------------------------------------------------
// Helpers de mise en forme du moniteur (monitor.go) — purs
// ---------------------------------------------------------------------------

func TestCurrencyLabel(t *testing.T) {
	if currencyLabel("XOF") != "FCFA" || currencyLabel("") != "FCFA" || currencyLabel("xof") != "FCFA" {
		t.Fatal("XOF (et vide) doivent s'afficher FCFA")
	}
	if currencyLabel("USD") != "USD" {
		t.Fatal("une autre devise est conservée telle quelle")
	}
}

func TestFormatDuration(t *testing.T) {
	cas := map[time.Duration]string{
		30 * time.Second: "< 1 min",
		5 * time.Minute:  "5 min",
		90 * time.Minute: "1 h 30 min",
		2 * time.Hour:    "2 h",
		26 * time.Hour:   "1 j 2 h",
		72 * time.Hour:   "3 j 0 h",
	}
	for d, want := range cas {
		if got := formatDuration(d); got != want {
			t.Fatalf("formatDuration(%v) = %q, attendu %q", d, got, want)
		}
	}
}

func TestStrconvI(t *testing.T) {
	cas := map[int]string{0: "0", 7: "7", 42: "42", 1234567: "1234567", -9: "-9"}
	for n, want := range cas {
		if got := strconvI(n); got != want {
			t.Fatalf("strconvI(%d) = %q, attendu %q", n, got, want)
		}
	}
}

func TestStockMessage(t *testing.T) {
	r := &model.Router{Name: "Cocody 2"}
	title, body := stockMessage(r, 3, "low", 25)
	if !strings.Contains(title, "Stock de vouchers bas") || !strings.Contains(title, "Cocody 2") {
		t.Fatalf("titre d'alerte stock incorrect : %q", title)
	}
	if !strings.Contains(body, "3 voucher(s)") || !strings.Contains(body, "25") {
		t.Fatalf("corps d'alerte stock incorrect : %q", body)
	}
	title, body = stockMessage(r, 0, "empty", 25)
	if !strings.Contains(title, "Stock épuisé") {
		t.Fatalf("titre stock épuisé incorrect : %q", title)
	}
	if !strings.Contains(body, "Plus aucun voucher") {
		t.Fatalf("corps stock épuisé incorrect : %q", body)
	}
}

func TestBuildDailyReport(t *testing.T) {
	db := &model.DB{
		SettingsByAccount: map[string]model.Settings{
			"acc-1": {Tenant: model.Tenant{Name: "Hotspot Yopougon", Currency: "XOF"}},
		},
	}
	now := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	db.Sales = []model.Sale{{AccountID: "acc-1", Amount: 1500, At: now.Add(-2 * time.Hour).Format(time.RFC3339)}}
	db.HotspotUsers = []model.HotspotUser{
		{AccountID: "acc-1", Kind: "voucher", Status: "active", CreatedAt: now.Add(-1 * time.Hour).Format(time.RFC3339)},
		{AccountID: "acc-1", Kind: "voucher", Status: "used"},
		{AccountID: "acc-autre", Kind: "voucher", Status: "active"}, // autre compte : ignoré
	}
	db.Sessions = []model.Session{{AccountID: "acc-1"}}
	db.Routers = []model.Router{{AccountID: "acc-1", Status: "online"}, {AccountID: "acc-1", Status: "offline"}}

	title, body := buildDailyReport(db, "acc-1", now, model.NotificationSettings{})
	if !strings.Contains(title, "15/06/2025") || !strings.Contains(title, "Rapport MikCloud") {
		t.Fatalf("titre du rapport incorrect : %q", title)
	}
	for _, attendu := range []string{
		"Hotspot Yopougon",
		"1 ticket(s) — 1500 FCFA", // vente du jour, devise XOF → FCFA
		"Nouveaux utilisateurs : 1",
		"Sessions actives : 1",
		"Routeurs en ligne : 1/2",
		"Vouchers disponibles : 1", // le « used » et l'autre compte sont exclus
	} {
		if !strings.Contains(body, attendu) {
			t.Fatalf("ligne de rapport absente : %q (corps : %s)", attendu, body)
		}
	}
}
