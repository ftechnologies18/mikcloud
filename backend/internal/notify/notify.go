// Package notify — sendeurs multi-canaux (Telegram, WhatsApp Cloud API, Email
// SMTP direct ou API Resend) + livraison des notifications MikCloud. Fonctions
// libres sans état : utilisables depuis le moniteur (goroutine) comme depuis
// les handlers API (test de canal).
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

// resendEndpoint — API d'envoi Resend. Variable de package : les tests la
// remplacent par un serveur httptest local (aucun réseau réel en CI).
var resendEndpoint = "https://api.resend.com/emails"

// resendDefaultFrom — expéditeur par défaut quand ResendFrom est vide : le
// domaine d'essai Resend ne délivre qu'à l'adresse du propriétaire du compte
// Resend ; en production, renseignez ResendFrom avec un domaine vérifié.
const resendDefaultFrom = "MikCloud <onboarding@resend.dev>"

// Kinds de notification (colonne kind de NotificationLog).
const (
	KindRouterOffline = "router_offline"
	KindRouterBack    = "router_back"
	KindLowStock      = "low_stock"
	KindDailyReport   = "daily_report"
	KindTest          = "test"
	// KindPasswordReset — N°68 : e-mail transactionnel « Mot de passe
	// oublié ? » (lien de réinitialisation). Pas un canal d'alerte : envoyé à
	// la demande, hors moniteur automatique.
	KindPasswordReset = "password_reset"
)

// EmailProviderOf — fournisseur du canal e-mail normalisé : "resend" ou
// "smtp" (défaut historique, y compris valeur vide ou inconnue).
func EmailProviderOf(cfg *model.NotificationSettings) string {
	if strings.EqualFold(strings.TrimSpace(cfg.EmailProvider), "resend") {
		return "resend"
	}
	return "smtp"
}

// Configured — le canal demandé est activé ET suffisamment renseigné.
func Configured(cfg *model.NotificationSettings, channel string) bool {
	switch channel {
	case "telegram":
		return cfg.TelegramEnabled && cfg.TelegramBotToken != "" && cfg.TelegramChatID != ""
	case "whatsapp":
		return cfg.WhatsAppEnabled && cfg.WhatsAppToken != "" && cfg.WhatsAppPhoneID != "" && cfg.WhatsAppTo != ""
	case "email":
		if !cfg.EmailEnabled || cfg.EmailTo == "" {
			return false
		}
		if EmailProviderOf(cfg) == "resend" {
			return cfg.ResendAPIKey != ""
		}
		return cfg.SMTPHost != ""
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
		var err error
		// Notifications automatiques : corps texte seul (le HTML
		// brandé est réservé aux e-mails transactionnels — N°79).
		if EmailProviderOf(cfg) == "resend" {
			err = sendEmailResend(cfg.ResendAPIKey, cfg.ResendFrom, cfg.EmailTo, title, body, "")
		} else {
			err = sendEmail(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUser, cfg.SMTPPass, cfg.EmailTo, title, body, "")
		}
		logs = append(logs, logEntry(cfg, "email", kind, title, body, err))
	}
	if len(logs) == 0 {
		logs = append(logs, logEntry(cfg, "system", kind, title, body, errors.New("aucun canal configuré")))
	}
	return logs
}

// LogEntry — trace normalisée d'un envoi, exposée aux handlers pour les
// envois TRANSACTIONNELS effectués hors Deliver (N°68 : réinitialisation de
// mot de passe — même format d'historique que les notifications).
func LogEntry(cfg *model.NotificationSettings, channel, kind, title, body string, err error) model.NotificationLog {
	return logEntry(cfg, channel, kind, title, body, err)
}

// EmailCredentialsOK — N°68 : le fournisseur e-mail du compte est
// renseigné (clé Resend ou hôte SMTP), SANS exiger EmailEnabled ni EmailTo
// (interrupteurs des alertes automatiques) : les e-mails transactionnels
// précisent leur propre destinataire.
func EmailCredentialsOK(cfg *model.NotificationSettings) bool {
	if EmailProviderOf(cfg) == "resend" {
		return strings.TrimSpace(cfg.ResendAPIKey) != ""
	}
	return strings.TrimSpace(cfg.SMTPHost) != ""
}

// SendEmailTo — N°68 : envoi e-mail transactionnel — même mécanique que le
// canal e-mail des notifications (Resend ou SMTP selon le provider du
// compte), mais le DESTINATAIRE est fourni par l'appelant (ex. mot de passe
// oublié → l'e-mail enregistré du compte). N°79 : textBody est la version
// texte (repli des clients sans HTML, pièce text/plain du multipart) et
// htmlBody la version brandée (pièce text/html — ignorée si vide). Aucune
// écriture de journal : c'est l'appelant qui trace (il connaît le kind et
// le contexte).
func SendEmailTo(cfg *model.NotificationSettings, to, title, textBody, htmlBody string) error {
	if EmailProviderOf(cfg) == "resend" {
		return sendEmailResend(cfg.ResendAPIKey, cfg.ResendFrom, to, title, textBody, htmlBody)
	}
	return sendEmail(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUser, cfg.SMTPPass, to, title, textBody, htmlBody)
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
// Email — Resend (API HTTP) puis SMTP direct (TLS implicite sur 465, STARTTLS
// sinon). Le fournisseur est choisi par NotificationSettings.EmailProvider.
// ---------------------------------------------------------------------------

// sendEmailResend — POST /emails de l'API Resend (https://resend.com/docs).
// Un statut 2xx (202 Accepted en pratique) suffit : l'identifiant renvoyé
// n'est pas conservé (l'historique notif_log trace le résultat côté MikCloud).
// N°79 : htmlBody non vide → champ "html" du payload (Resend délivre alors
// text ET html — chaque client affiche sa meilleure pièce).
func sendEmailResend(apiKey, from, to, title, textBody, htmlBody string) error {
	from = strings.TrimSpace(from)
	if from == "" {
		from = resendDefaultFrom
	}
	payload := map[string]any{
		"from":    from,
		"to":      []string{to},
		"subject": title,
		"text":    textBody,
	}
	if strings.TrimSpace(htmlBody) != "" {
		payload["html"] = htmlBody
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("resend : payload : %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, resendEndpoint, bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("resend : requête invalide : %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: httpTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("resend injoignable : %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	// Corps d'erreur Resend : {"name":"…","message":"…"} (clé invalide,
	// expéditeur non vérifié, limite de débit…) — message repris tel quel.
	var out struct {
		Name    string `json:"name"`
		Message string `json:"message"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if msg := strings.TrimSpace(out.Message); msg != "" {
		return fmt.Errorf("resend : %s", msg)
	}
	if out.Name != "" {
		return fmt.Errorf("resend : %s", out.Name)
	}
	return fmt.Errorf("resend : HTTP %d", resp.StatusCode)
}

func sendEmail(host string, port int, user, pass, to, title, textBody, htmlBody string) error {
	if port <= 0 || port > 65535 {
		port = 587
	}
	addr := host + ":" + strconv.Itoa(port)
	from := user
	if from == "" {
		from = to // cas rare : relais local sans authentification
	}

	msg := buildMessage(from, to, title, textBody, htmlBody)
	auth := smtp.PlainAuth("", user, pass, host)
	hostname, _, err := net.SplitHostPort(addr)
	if err != nil {
		hostname = host
	}

	// N°74 — deadlines PARTOUT : tls.Dial (465) et smtp.SendMail (587)
	// n'avaient AUCUN timeout — un serveur SMTP muet tenait la connexion
	// ~2 min par tentative (timeout TCP OS), durée pendant laquelle
	// l'appelant attendait. Borne : 10 s d'établissement, 25 s de session.
	const dialTimeout = 10 * time.Second
	const sessionDeadline = 25 * time.Second

	if port == 465 {
		// TLS implicite (ex. certains hébergeurs mail ivoiriens sur 465).
		conn, err := tls.DialWithDialer(&net.Dialer{Timeout: dialTimeout}, "tcp", addr, &tls.Config{ServerName: hostname})
		if err != nil {
			return fmt.Errorf("email : connexion TLS : %w", err)
		}
		_ = conn.SetDeadline(time.Now().Add(sessionDeadline))
		client, err := smtp.NewClient(conn, hostname)
		if err != nil {
			_ = conn.Close()
			return fmt.Errorf("email : %w", err)
		}
		defer client.Close()
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("email : authentification : %w", err)
		}
		return smtpSend(client, from, to, msg)
	}

	// STARTTLS (587 et autres) : négociation StartTLS quand le serveur
	// l'annonce (même sémantique que smtp.SendMail, avec deadlines).
	conn, err := net.DialTimeout("tcp", addr, dialTimeout)
	if err != nil {
		return fmt.Errorf("email : connexion : %w", err)
	}
	_ = conn.SetDeadline(time.Now().Add(sessionDeadline))
	client, err := smtp.NewClient(conn, hostname)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("email : %w", err)
	}
	defer client.Close()
	if ok, _ := client.Extension("StartTLS"); ok {
		if err := client.StartTLS(&tls.Config{ServerName: hostname}); err != nil {
			return fmt.Errorf("email : STARTTLS : %w", err)
		}
	}
	if user != "" {
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("email : authentification : %w", err)
		}
	}
	return smtpSend(client, from, to, msg)
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

// buildMessage — message MIME : texte simple, ou multipart/alternative
// texte + HTML quand htmlBody est fourni (N°79 — chaque client affiche sa
// meilleure pièce, les clients sans HTML tombent sur le texte) ; le sujet est
// encodé en B-UTF-8 pour survivre aux accents (« Routeur hors ligne », noms
// ivoiriens…).
func buildMessage(from, to, title, textBody, htmlBody string) []byte {
	subject := "=?UTF-8?B?" + base64.StdEncoding.EncodeToString([]byte(title)) + "?="
	var sb strings.Builder
	sb.WriteString("From: MikCloud <" + from + ">\r\n")
	sb.WriteString("To: <" + to + ">\r\n")
	sb.WriteString("Subject: " + subject + "\r\n")
	sb.WriteString("MIME-Version: 1.0\r\n")
	if strings.TrimSpace(htmlBody) == "" {
		sb.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
		sb.WriteString("Content-Transfer-Encoding: 8bit\r\n")
		sb.WriteString("\r\n")
		sb.WriteString(title + "\n\n" + textBody + "\n")
		return []byte(sb.String())
	}
	// Frontière MIME : littérale volontairement exotique — elle ne peut
	// apparaître ni dans le texte ni dans le HTML brandé (contrôlé).
	const boundary = "=_mikcloud-alt-7C4F2A"
	sb.WriteString("Content-Type: multipart/alternative; boundary=\"" + boundary + "\"\r\n")
	sb.WriteString("\r\n")
	sb.WriteString("--" + boundary + "\r\n")
	sb.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	sb.WriteString("Content-Transfer-Encoding: 8bit\r\n")
	sb.WriteString("\r\n")
	sb.WriteString(title + "\n\n" + textBody + "\n")
	sb.WriteString("\r\n")
	sb.WriteString("--" + boundary + "\r\n")
	sb.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	sb.WriteString("Content-Transfer-Encoding: 8bit\r\n")
	sb.WriteString("\r\n")
	sb.WriteString(htmlBody)
	sb.WriteString("\r\n")
	sb.WriteString("--" + boundary + "--\r\n")
	return []byte(sb.String())
}
