// Package notify — sendeurs multi-canaux (Telegram, WhatsApp Cloud API, Email
// SMTP) + livraison des notifications MikCloud. Fonctions libres sans état :
// utilisables depuis le moniteur (goroutine) comme depuis les handlers API
// (test de canal).
package notify

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"mikcloud/hotspot-api/internal/model"
)

// httpTimeout — borne d'attente par envoi (le moniteur ne doit jamais bloquer
// le service plusieurs minutes si un canal est injoignable).
const httpTimeout = 12 * time.Second

// Kinds de notification (colonne kind de NotificationLog).
const (
	KindRouterOffline = "router_offline"
	KindRouterBack    = "router_back"
	KindLowStock      = "low_stock"
	KindDailyReport   = "daily_report"
	KindTest          = "test"
)

// Configured — le canal demandé est activé ET suffisamment renseigné.
func Configured(cfg *model.NotificationSettings, channel string) bool {
	switch channel {
	case "telegram":
		return cfg.TelegramEnabled && cfg.TelegramBotToken != "" && cfg.TelegramChatID != ""
	case "whatsapp":
		return cfg.WhatsAppEnabled && cfg.WhatsAppToken != "" && cfg.WhatsAppPhoneID != "" && cfg.WhatsAppTo != ""
	case "email":
		return cfg.EmailEnabled && cfg.SMTPHost != "" && cfg.EmailTo != ""
	}
	return false
}

// HasAnyChannel — au moins un canal est activé et configuré.
func HasAnyChannel(cfg *model.NotificationSettings) bool {
	return Configured(cfg, "telegram") || Configured(cfg, "whatsapp") || Configured(cfg, "email")
}

// Deliver envoie une notification sur les canaux configurés du compte
// (onlyChannel non vide → uniquement ce canal). Retourne une entrée de log
// par canal tenté ; aucun canal tenté (tout désactivé) → une entrée « system »
// explicite pour l'historique. Ne JAMAIS appeler sous verrou du store : les
// envois réseau peuvent durer jusqu'à httpTimeout par canal.
func Deliver(cfg *model.NotificationSettings, kind, title, body, onlyChannel string) []model.NotificationLog {
	var logs []model.NotificationLog
	try := func(channel string) bool { return onlyChannel == "" || onlyChannel == channel }

	if try("telegram") && Configured(cfg, "telegram") {
		err := sendTelegram(cfg.TelegramBotToken, cfg.TelegramChatID, title, body)
		logs = append(logs, logEntry(cfg, "telegram", kind, title, body, err))
	}
	if try("whatsapp") && Configured(cfg, "whatsapp") {
		err := sendWhatsApp(cfg.WhatsAppToken, cfg.WhatsAppPhoneID, cfg.WhatsAppTo, title, body)
		logs = append(logs, logEntry(cfg, "whatsapp", kind, title, body, err))
	}
	if try("email") && Configured(cfg, "email") {
		err := sendEmail(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUser, cfg.SMTPPass, cfg.EmailTo, title, body)
		logs = append(logs, logEntry(cfg, "email", kind, title, body, err))
	}
	if len(logs) == 0 {
		logs = append(logs, logEntry(cfg, "system", kind, title, body, errors.New("aucun canal configuré")))
	}
	return logs
}

// logEntry — trace d'un envoi (status sent/error + message d'erreur).
func logEntry(cfg *model.NotificationSettings, channel, kind, title, body string, err error) model.NotificationLog {
	e := model.NotificationLog{
		AccountID: cfg.AccountID,
		Channel:   channel,
		Kind:      kind,
		Title:     title,
		Body:      body,
		At:        model.NowISO(),
	}
	if err != nil {
		e.Status = "error"
		e.Error = err.Error()
	} else {
		e.Status = "sent"
	}
	return e
}

// ---------------------------------------------------------------------------
// Telegram — Bot API sendMessage
// ---------------------------------------------------------------------------

func sendTelegram(botToken, chatID, title, body string) error {
	payload := map[string]any{
		"chat_id": chatID,
		"text":    title + "\n\n" + body,
	}
	b, _ := json.Marshal(payload)
	url := "https://api.telegram.org/bot" + botToken + "/sendMessage"
	client := &http.Client{Timeout: httpTimeout}
	resp, err := client.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("telegram injoignable : %w", err)
	}
	defer resp.Body.Close()
	var out struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return fmt.Errorf("telegram : réponse illisible (HTTP %d)", resp.StatusCode)
	}
	if !out.OK {
		return fmt.Errorf("telegram : %s", strings.TrimSpace(out.Description))
	}
	return nil
}

// ---------------------------------------------------------------------------
// WhatsApp — Cloud API (Meta Graph) /messages
// ---------------------------------------------------------------------------

func sendWhatsApp(accessToken, phoneNumberID, to, title, body string) error {
	payload := map[string]any{
		"messaging_product": "whatsapp",
		"recipient_type":    "individual",
		"to":                to,
		"type":              "text",
		"text":              map[string]any{"preview_url": false, "body": title + "\n\n" + body},
	}
	b, _ := json.Marshal(payload)
	url := "https://graph.facebook.com/v20.0/" + phoneNumberID + "/messages"
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("whatsapp : requête invalide : %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: httpTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("whatsapp injoignable : %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	var out struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if msg := strings.TrimSpace(out.Error.Message); msg != "" {
		return fmt.Errorf("whatsapp : %s", msg)
	}
	return fmt.Errorf("whatsapp : HTTP %d", resp.StatusCode)
}

// ---------------------------------------------------------------------------
// Email — SMTP direct (TLS implicite sur 465, STARTTLS sinon)
// ---------------------------------------------------------------------------

func sendEmail(host string, port int, user, pass, to, title, body string) error {
	if port <= 0 || port > 65535 {
		port = 587
	}
	addr := host + ":" + strconv.Itoa(port)
	from := user
	if from == "" {
		from = to // cas rare : relais local sans authentification
	}

	msg := buildMessage(from, to, title, body)
	auth := smtp.PlainAuth("", user, pass, host)
	hostname, _, err := net.SplitHostPort(addr)
	if err != nil {
		hostname = host
	}

	if port == 465 {
		// TLS implicite (ex. certains hébergeurs mail ivoiriens sur 465).
		conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: hostname})
		if err != nil {
			return fmt.Errorf("email : connexion TLS : %w", err)
		}
		client, err := smtp.NewClient(conn, hostname)
		if err != nil {
			return fmt.Errorf("email : %w", err)
		}
		defer client.Close()
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("email : authentification : %w", err)
		}
		return smtpSend(client, from, to, msg)
	}

	// STARTTLS (587 et autres) : smtp.SendMail négocie StartTLS quand annoncé.
	return smtp.SendMail(addr, auth, from, []string{to}, msg)
}

// smtpSend — MAIL FROM/RCPT/DATA sur un client déjà connecté et authentifié.
func smtpSend(client *smtp.Client, from, to string, msg []byte) error {
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("email : MAIL FROM : %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("email : RCPT TO : %w", err)
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("email : DATA : %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("email : écriture : %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("email : clôture : %w", err)
	}
	return client.Quit()
}

// buildMessage — message MIME texte simple ; le sujet est encodé en B-UTF-8
// pour survivre aux accents (« Routeur hors ligne », noms ivoiriens…).
func buildMessage(from, to, title, body string) []byte {
	subject := "=?UTF-8?B?" + base64.StdEncoding.EncodeToString([]byte(title)) + "?="
	var sb strings.Builder
	sb.WriteString("From: MikCloud <" + from + ">\r\n")
	sb.WriteString("To: <" + to + ">\r\n")
	sb.WriteString("Subject: " + subject + "\r\n")
	sb.WriteString("MIME-Version: 1.0\r\n")
	sb.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	sb.WriteString("Content-Transfer-Encoding: 8bit\r\n")
	sb.WriteString("\r\n")
	sb.WriteString(title + "\n\n" + body + "\n")
	return []byte(sb.String())
}
