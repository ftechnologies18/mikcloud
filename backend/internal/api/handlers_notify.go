// Package api — notifications : réglages des canaux (Telegram, WhatsApp Cloud
// API, Email — SMTP direct ou API Resend), test d'envoi et historique.
//
//	GET  /api/notifications        → réglages du compte (secrets masqués)
//	PUT  /api/notifications        → mise à jour (secret vide = conservé)
//	POST /api/notifications/test   → message de test sur UN canal
//	GET  /api/notifications/log    → 50 derniers envois du compte
//
// Le moniteur (internal/notify) utilise les mêmes réglages pour les alertes
// automatiques : routeur hors ligne, retour en ligne, stock bas, rapport
// journalier.
package api

import (
	"net/http"
	"sort"
	"strings"

	"mikcloud/hotspot-api/internal/model"
	"mikcloud/hotspot-api/internal/notify"
	"mikcloud/hotspot-api/internal/store"
)

// registerNotifRoutes — endpoints console (auth JWT via middleware /api/).
func (a *API) registerNotifRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/notifications", a.requireRole(2, a.handleNotifGet))
	mux.HandleFunc("PUT /api/notifications", a.requireRole(2, a.handleNotifPut))
	mux.HandleFunc("POST /api/notifications/test", a.requireRole(2, a.handleNotifTest))
	mux.HandleFunc("GET /api/notifications/log", a.requireRole(2, a.handleNotifLog))
	// N°150 — pairage du canal Telegram plateforme (lien magique t.me/<bot>?start=<code>).
	mux.HandleFunc("POST /api/notifications/telegram/pair-code", a.requireRole(2, a.handleTelegramPairCode))
	mux.HandleFunc("GET /api/notifications/telegram/pair-status", a.requireRole(2, a.handleTelegramPairStatus))
}

// notifView — réglages exposés à la console : les secrets ne partent JAMAIS,
// seuls des booléens « …Set » indiquent qu'ils sont déjà configurés.
type notifView struct {
	AccountID string `json:"accountId"`
	Enabled   bool   `json:"enabled"`

	TelegramEnabled     bool   `json:"telegramEnabled"`
	TelegramBotTokenSet bool   `json:"telegramBotTokenSet"`
	TelegramChatID      string `json:"telegramChatId"`

	WhatsAppEnabled  bool   `json:"whatsappEnabled"`
	WhatsAppTokenSet bool   `json:"whatsappTokenSet"`
	WhatsAppPhoneID  string `json:"whatsappPhoneId"`
	WhatsAppTo       string `json:"whatsappTo"`

	EmailEnabled  bool   `json:"emailEnabled"`
	EmailProvider string `json:"emailProvider"` // smtp | resend (normalisé, jamais vide)
	SMTPHost      string `json:"smtpHost"`
	SMTPPort      int    `json:"smtpPort"`
	SMTPUser      string `json:"smtpUser"`
	SMTPPassSet   bool   `json:"smtpPassSet"`
	// Resend (N°67) — la clé API ne part JAMAIS, seul le booléen l'annonce.
	ResendAPIKeySet bool   `json:"resendApiKeySet"`
	ResendFrom      string `json:"resendFrom"`
	EmailTo         string `json:"emailTo"`

	OfflineAfterSec   int  `json:"offlineAfterSec"`
	LowStockThreshold int  `json:"lowStockThreshold"`
	DailyReport       bool `json:"dailyReport"`
	ReportHour        int  `json:"reportHour"`
	// N°150 — disponibilité des canaux plateforme : la console adapte ses
	// cartes (Telegram « zéro setup » quand le bot FTCI est actif, e-mail
	// simplifié quand le relais du compte principal peut porter l'envoi).
	TelegramPlatformAvailable bool   `json:"telegramPlatformAvailable"`
	TelegramBotUsername       string `json:"telegramBotUsername,omitempty"`
	EmailPlatformRelay        bool   `json:"emailPlatformRelay"`
}

func viewOf(cfg model.NotificationSettings, telegramPlatform bool, botUsername string, emailRelay bool) notifView {
	return notifView{
		AccountID: cfg.AccountID,
		Enabled:   cfg.Enabled,

		TelegramEnabled:     cfg.TelegramEnabled,
		TelegramBotTokenSet: cfg.TelegramBotToken != "",
		TelegramChatID:      cfg.TelegramChatID,

		WhatsAppEnabled:  cfg.WhatsAppEnabled,
		WhatsAppTokenSet: cfg.WhatsAppToken != "",
		WhatsAppPhoneID:  cfg.WhatsAppPhoneID,
		WhatsAppTo:       cfg.WhatsAppTo,

		EmailEnabled:    cfg.EmailEnabled,
		EmailProvider:   notify.EmailProviderOf(&cfg),
		SMTPHost:        cfg.SMTPHost,
		SMTPPort:        cfg.SMTPPort,
		SMTPUser:        cfg.SMTPUser,
		SMTPPassSet:     cfg.SMTPPass != "",
		ResendAPIKeySet: cfg.ResendAPIKey != "",
		ResendFrom:      cfg.ResendFrom,
		EmailTo:         cfg.EmailTo,

		OfflineAfterSec:           cfg.OfflineAfterSec,
		LowStockThreshold:         cfg.LowStockThreshold,
		DailyReport:               cfg.DailyReport,
		ReportHour:                cfg.ReportHour,
		TelegramPlatformAvailable: telegramPlatform,
		TelegramBotUsername:       botUsername,
		EmailPlatformRelay:        emailRelay,
	}
}

// notifPutPayload — corps du PUT. Les champs secrets sont OPTIONNELS : absents
// ou vides → la valeur déjà stockée est conservée (le masquage côté console
// empêche de toute façon d'envoyer un secret inconnu).
type notifPutPayload struct {
	Enabled *bool `json:"enabled"`

	TelegramEnabled  *bool   `json:"telegramEnabled"`
	TelegramBotToken *string `json:"telegramBotToken"`
	TelegramChatID   *string `json:"telegramChatId"`

	WhatsAppEnabled *bool   `json:"whatsappEnabled"`
	WhatsAppToken   *string `json:"whatsappToken"`
	WhatsAppPhoneID *string `json:"whatsappPhoneId"`
	WhatsAppTo      *string `json:"whatsappTo"`

	EmailEnabled  *bool   `json:"emailEnabled"`
	EmailProvider *string `json:"emailProvider"` // "smtp" | "resend" (toute autre valeur → smtp)
	SMTPHost      *string `json:"smtpHost"`
	SMTPPort      *int    `json:"smtpPort"`
	SMTPUser      *string `json:"smtpUser"`
	SMTPPass      *string `json:"smtpPass"`
	ResendAPIKey  *string `json:"resendApiKey"`
	ResendFrom    *string `json:"resendFrom"`
	EmailTo       *string `json:"emailTo"`

	OfflineAfterSec   *int  `json:"offlineAfterSec"`
	LowStockThreshold *int  `json:"lowStockThreshold"`
	DailyReport       *bool `json:"dailyReport"`
	ReportHour        *int  `json:"reportHour"`
}

// handleNotifGet — GET /api/notifications
func (a *API) handleNotifGet(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	a.store.Lock()
	db := a.store.Data()
	cfg := store.GetOrCreateNotifSettings(db, acc)
	emailRelay := a.platformEmailRelayLocked(db) // N°150
	a.store.Save()                               // persister la création éventuelle (défauts)
	a.store.Unlock()
	a.tgMu.Lock()
	botUsername := a.telegramBotUsername
	a.tgMu.Unlock()
	writeJSON(w, http.StatusOK, viewOf(cfg, notify.TelegramPlatformToken != "", botUsername, emailRelay))
}

// handleNotifPut — PUT /api/notifications
func (a *API) handleNotifPut(w http.ResponseWriter, r *http.Request) {
	var req notifPutPayload
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	acc := accountScope(r)

	a.store.Lock()
	db := a.store.Data()
	cfg := store.GetOrCreateNotifSettings(db, acc)
	cfg.AccountID = acc
	applyNotifPut(&cfg, &req)
	store.SetNotifSettings(db, cfg)
	a.logActivityBy(r, db, acc, "system", "Configuration des notifications mise à jour")
	emailRelay := a.platformEmailRelayLocked(db) // N°150
	a.store.Save()
	a.store.Unlock()
	a.tgMu.Lock()
	botUsername := a.telegramBotUsername
	a.tgMu.Unlock()

	writeJSON(w, http.StatusOK, viewOf(cfg, notify.TelegramPlatformToken != "", botUsername, emailRelay))
}

// applyNotifPut — fusionne le payload dans les réglages existants (les secrets
// vides conservent la valeur stockée ; les autres champs vides effacent).
func applyNotifPut(cfg *model.NotificationSettings, req *notifPutPayload) {
	if req.Enabled != nil {
		cfg.Enabled = *req.Enabled
	}

	if req.TelegramEnabled != nil {
		cfg.TelegramEnabled = *req.TelegramEnabled
	}
	if req.TelegramBotToken != nil && strings.TrimSpace(*req.TelegramBotToken) != "" {
		cfg.TelegramBotToken = strings.TrimSpace(*req.TelegramBotToken)
	}
	if req.TelegramChatID != nil {
		cfg.TelegramChatID = strings.TrimSpace(*req.TelegramChatID)
	}

	if req.WhatsAppEnabled != nil {
		cfg.WhatsAppEnabled = *req.WhatsAppEnabled
	}
	if req.WhatsAppToken != nil && strings.TrimSpace(*req.WhatsAppToken) != "" {
		cfg.WhatsAppToken = strings.TrimSpace(*req.WhatsAppToken)
	}
	if req.WhatsAppPhoneID != nil {
		cfg.WhatsAppPhoneID = strings.TrimSpace(*req.WhatsAppPhoneID)
	}
	if req.WhatsAppTo != nil {
		cfg.WhatsAppTo = strings.TrimSpace(strings.ReplaceAll(*req.WhatsAppTo, " ", ""))
	}

	if req.EmailEnabled != nil {
		cfg.EmailEnabled = *req.EmailEnabled
	}
	// Fournisseur du canal e-mail (N°67) : toute valeur autre que "resend"
	// retombe sur SMTP ("" = smtp, défaut historique conservé en base).
	if req.EmailProvider != nil {
		if strings.EqualFold(strings.TrimSpace(*req.EmailProvider), "resend") {
			cfg.EmailProvider = "resend"
		} else {
			cfg.EmailProvider = ""
		}
	}
	if req.SMTPHost != nil {
		cfg.SMTPHost = strings.TrimSpace(*req.SMTPHost)
	}
	if req.SMTPPort != nil {
		cfg.SMTPPort = *req.SMTPPort
	}
	if req.SMTPUser != nil {
		cfg.SMTPUser = strings.TrimSpace(*req.SMTPUser)
	}
	if req.SMTPPass != nil && *req.SMTPPass != "" {
		cfg.SMTPPass = *req.SMTPPass
	}
	// Clé API Resend : secret — vide/absent = valeur stockée conservée
	// (même contrat que le mot de passe SMTP et les tokens Telegram/WhatsApp).
	if req.ResendAPIKey != nil && strings.TrimSpace(*req.ResendAPIKey) != "" {
		cfg.ResendAPIKey = strings.TrimSpace(*req.ResendAPIKey)
	}
	// Expéditeur Resend : champ ORDINAIRE (pas un secret) — vide = efface,
	// le sendeur retombe alors sur l'expéditeur par défaut.
	if req.ResendFrom != nil {
		cfg.ResendFrom = strings.TrimSpace(*req.ResendFrom)
	}
	if req.EmailTo != nil {
		cfg.EmailTo = strings.TrimSpace(*req.EmailTo)
	}

	if req.OfflineAfterSec != nil {
		cfg.OfflineAfterSec = *req.OfflineAfterSec
	}
	if req.LowStockThreshold != nil {
		cfg.LowStockThreshold = *req.LowStockThreshold
	}
	if req.DailyReport != nil {
		cfg.DailyReport = *req.DailyReport
	}
	if req.ReportHour != nil {
		cfg.ReportHour = *req.ReportHour
	}
}

// deliverNotif — envoi réel multi-canaux (variable pour les tests, même
// discipline que sendAccountEmail N°146 : les tests api capturent sans
// réseau ; le relais plateforme est couvert par les tests notify).
var deliverNotif = notify.DeliverWithPlatform

// handleNotifTest — POST /api/notifications/test {channel: telegram|whatsapp|email}
func (a *API) handleNotifTest(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Channel string `json:"channel"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	channel := strings.ToLower(strings.TrimSpace(req.Channel))
	if channel != "telegram" && channel != "whatsapp" && channel != "email" {
		writeErr(w, http.StatusBadRequest, "Canal inconnu (telegram, whatsapp ou email)")
		return
	}
	acc := accountScope(r)

	a.store.Lock()
	cfg := store.GetOrCreateNotifSettings(a.store.Data(), acc)
	a.store.Unlock()

	var platformEmail *model.NotificationSettings // N°150 : relais e-mail du compte principal
	if channel == "email" {
		a.store.Lock()
		platformEmail = a.platformEmailRelayPtrLocked(a.store.Data())
		a.store.Unlock()
	}
	if !notify.ConfiguredWithPlatform(&cfg, platformEmail, channel) {
		writeErr(w, http.StatusBadRequest, "Ce canal n'est pas encore configuré : activez-le et renseignez tous les champs requis")
		return
	}

	title := "MikCloud — Test de notification"
	body := "Canal " + channel + " opérationnel ✔\n" +
		"Vous recevrez ici : routeur hors ligne, stock de vouchers bas et rapport quotidien."
	logs := deliverNotif(&cfg, platformEmail, notify.KindTest, title, body, channel)

	// Historique sous verrou (les envois réseau sont déjà terminés).
	var firstErr string
	a.store.Lock()
	db := a.store.Data()
	for _, l := range logs {
		l.ID = model.NewID("n-")
		l.At = model.NowISO()
		db.NotifLog = append(db.NotifLog, l)
		if l.Status == "error" && firstErr == "" {
			firstErr = l.Error
		}
	}
	a.store.Save()
	a.store.Unlock()

	if firstErr != "" {
		writeErr(w, http.StatusBadRequest, firstErr)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "channel": channel})
}

// handleNotifLog — GET /api/notifications/log : 50 derniers envois du compte.
func (a *API) handleNotifLog(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	a.store.Lock()
	db := a.store.Data()
	logs := make([]model.NotificationLog, 0, 50)
	for _, l := range db.NotifLog {
		if l.AccountID == acc {
			logs = append(logs, l)
		}
	}
	a.store.Unlock()

	sort.SliceStable(logs, func(i, j int) bool { return logs[i].At > logs[j].At })
	if len(logs) > 50 {
		logs = logs[:50]
	}
	writeJSON(w, http.StatusOK, logs)
}

// platformEmailRelayLocked — N°150 : le compte principal a-t-il des
// identifiants e-mail exploitables (relais) ? À appeler SOUS VERROU du
// store. Sert uniquement à AFFICHER la disponibilité (vue console).
func (a *API) platformEmailRelayLocked(db *model.DB) bool {
	return a.platformEmailRelayPtrLocked(db) != nil
}

// platformEmailRelayPtrLocked — N°150 : réglages e-mail du compte principal
// (copie du map, déréférencée après déverrouillage sans danger), nil si pas
// d'identifiants. Même résolution que le moniteur (notify.platformEmailLocked
// vit dans le package notify pour la boucle de surveillance ; l'API a son
// propre accès au store).
func (a *API) platformEmailRelayPtrLocked(db *model.DB) *model.NotificationSettings {
	if cfg, ok := db.NotifSettings[model.AccountMainID]; ok && notify.EmailCredentialsOK(&cfg) {
		return &cfg
	}
	return nil
}
