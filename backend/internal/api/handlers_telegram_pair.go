// Package api — N°150 : pairage du canal Telegram PLATEFORME (bot FTCI).
//
// Parcours « lien magique », zéro @BotFather pour le gérant :
//
//	POST /api/notifications/telegram/pair-code  (auth JWT, rang ≥ 2)
//	      → {code, url, botUsername, expiresAt} — code éphémère 15 min,
//	        usage unique, lié au compte demandeur ;
//	le frontend ouvre t.me/<bot>?start=<code> → le gérant appuie sur
//	« Démarrer » dans Telegram ;
//	POST /api/webhooks/telegram  (PUBLIC — serveurs Telegram uniquement,
//	      authentifié par l'en-tête X-Telegram-Bot-Api-Secret-Token posé au
//	      setWebhook, comparaison à temps constant) → l'update /start <code>
//	      écrit le chat ID dans les réglages du compte (TelegramEnabled = true)
//	      et le bot répond la confirmation ;
//	GET  /api/notifications/telegram/pair-status?code=…  (auth JWT)
//	      → {status: pending|linked|expired, chatId?} — la console détecte
//	        la liaison et rafraîchit les réglages.
//
// Sécurité : le code secret (8 car. crypto/rand, alphabet sans caractères
// ambigus ≈ 2⁴⁰ combinaisons) EST le consentement — il n'est montré qu'à la
// session console du compte, expire en 15 min et ne sert qu'une fois ; le
// webhook répond TOUJOURS 200 après validation du secret (Telegram réessaie
// sinon pendant des heures) ; tgMu n'est JAMAIS tenu pendant une prise du
// verrou du store (ni l'inverse).
package api

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"io"
	"log"
	"math/big"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"mikcloud/hotspot-api/internal/notify"
	"mikcloud/hotspot-api/internal/store"
)

// telegramPairTTL — durée de vie d'un code de pairage (fenêtre confortable
// pour ouvrir Telegram et appuyer sur Démarrer, tout en restant éphémère).
// Variable de package : les tests la raccourcissent pour tester l'expiry.
var telegramPairTTL = 15 * time.Minute

// telegramPairing — code en attente de /start (sous tgMu).
type telegramPairing struct {
	accountID string
	expiresAt time.Time
}

// Stubs testables — même discipline que sendResetEmail (N°68) : les
// tests du package api remplacent ces fonctions pour capturer les
// envois sans réseau réel.
var (
	telegramGetMeFn = notify.TelegramGetMe
	telegramSendFn  = notify.SendTelegramRaw
	telegramHookFn  = notify.TelegramSetWebhook
)

// telegramCodeAlphabet — sans 0/O/1/I/l : le code est recopié à l'œil depuis
// un écran vers un clavier téléphone, les sosies coûtent des essais.
const telegramCodeAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

// newTelegramCode — 8 caractères crypto/rand (≈ 2⁴⁰, hors de portée d'un
// essai exhaustif dans la fenêtre de 15 min même à grande échelle).
func newTelegramCode() string {
	b := make([]byte, 8)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(telegramCodeAlphabet))))
		if err != nil {
			panic("crypto/rand indisponible : " + err.Error()) // unrecoverable, comme model.NewID
		}
		b[i] = telegramCodeAlphabet[n.Int64()]
	}
	return string(b)
}

// BootstrapTelegramPlatform — N°150 : appelé par main.go dans une goroutine.
// getMe cache le @username du bot (lien magique), setWebhook branche l'URL
// publique (RENDER_EXTERNAL_URL fournie par Render, sinon PUBLIC_BASE_URL)
// avec le secret X-Telegram-Bot-Api-Secret-Token. Best-effort : chaque échec
// est journalisé et retentable (pair-code refait getMe à la demande ; le
// webhook est re-posé au prochain redéploiement).
func (a *API) BootstrapTelegramPlatform() {
	token := notify.TelegramPlatformToken
	if token == "" {
		return // canal plateforme désactivé : comportement BYO historique
	}
	if me, err := telegramGetMeFn(token); err == nil {
		a.tgMu.Lock()
		a.telegramBotUsername = me
		a.tgMu.Unlock()
		log.Printf("[telegram] bot plateforme @%s prêt (lien magique actif)", me)
	} else {
		log.Printf("[telegram] getMe échoué (retenté à la première demande de pairage) : %v", err)
	}
	secret := strings.TrimSpace(os.Getenv("TELEGRAM_WEBHOOK_SECRET"))
	if secret == "" {
		log.Println("[telegram] AVERTISSEMENT : TELEGRAM_WEBHOOK_SECRET absente — le webhook public reste refusé (503) jusqu'à redéploiement avec la variable")
		return
	}
	base := strings.TrimRight(strings.TrimSpace(os.Getenv("RENDER_EXTERNAL_URL")), "/")
	if base == "" {
		base = strings.TrimRight(strings.TrimSpace(os.Getenv("PUBLIC_BASE_URL")), "/")
	}
	if base == "" {
		log.Println("[telegram] AVERTISSEMENT : ni RENDER_EXTERNAL_URL ni PUBLIC_BASE_URL — setWebhook sauté (posez l'URL publique du service)")
		return
	}
	if err := telegramHookFn(token, base+"/api/webhooks/telegram", secret); err != nil {
		log.Printf("[telegram] setWebhook échoué : %v", err)
	} else {
		log.Printf("[telegram] webhook branché sur %s/api/webhooks/telegram", base)
	}
}

// handleTelegramPairCode — POST /api/notifications/telegram/pair-code.
func (a *API) handleTelegramPairCode(w http.ResponseWriter, r *http.Request) {
	if notify.TelegramPlatformToken == "" {
		writeErr(w, http.StatusServiceUnavailable, "Canal Telegram plateforme non configuré sur ce service (TELEGRAM_PLATFORM_BOT_TOKEN absent) — utilisez votre propre bot ou contactez le support")
		return
	}
	acc := accountScope(r)

	// @username du bot : cache, sinon getMe à la demande (bootstrap raté au
	// boot ou démarrage à froid) — borné par httpTimeout côté notify.
	a.tgMu.Lock()
	username := a.telegramBotUsername
	a.tgMu.Unlock()
	if username == "" {
		me, err := telegramGetMeFn(notify.TelegramPlatformToken)
		if err != nil {
			writeErr(w, http.StatusServiceUnavailable, "Bot Telegram injoignable pour l'instant — réessayez dans un instant")
			return
		}
		username = me
		a.tgMu.Lock()
		a.telegramBotUsername = me
		a.tgMu.Unlock()
	}

	// Un code actif par compte : le nouveau remplace l'ancien.
	code := newTelegramCode()
	expires := time.Now().Add(telegramPairTTL)
	a.tgMu.Lock()
	if old, ok := a.tgAccountCode[acc]; ok {
		delete(a.tgPairings, old)
	}
	a.tgPairings[code] = telegramPairing{accountID: acc, expiresAt: expires}
	a.tgAccountCode[acc] = code
	a.tgMu.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"code":        code,
		"url":         "https://t.me/" + username + "?start=" + code,
		"botUsername": username,
		"expiresAt":   expires.UTC().Format(time.RFC3339),
	})
}

// handleTelegramPairStatus — GET /api/notifications/telegram/pair-status?code=…
func (a *API) handleTelegramPairStatus(w http.ResponseWriter, r *http.Request) {
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	if code == "" {
		writeErr(w, http.StatusBadRequest, "Paramètre « code » requis")
		return
	}
	acc := accountScope(r)

	// Réglages D'ABORD : chat ID posé = lié. Le code est consommé au
	// webhook (usage unique, retiré du map) et une liaison antérieure
	// (nouveau code demandé alors que le canal est déjà connecté) reste
	// « linked » — la console n'a pas à distinguer ces cas.
	a.store.Lock()
	cfg := store.GetOrCreateNotifSettings(a.store.Data(), acc)
	a.store.Unlock()
	if cfg.TelegramChatID != "" {
		writeJSON(w, http.StatusOK, map[string]any{"status": "linked", "chatId": cfg.TelegramChatID})
		return
	}

	// Aucun chat ID : le code est-il encore en attente (non expiré) ?
	a.tgMu.Lock()
	p, ok := a.tgPairings[code]
	valid := ok && p.accountID == acc && time.Now().Before(p.expiresAt)
	a.tgMu.Unlock()
	if !valid {
		writeJSON(w, http.StatusOK, map[string]any{"status": "expired"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "pending"})
}

// telegramUpdate — extrait d'un update Bot API : seul /start <code> nous
// intéresse (le reste reçoit une réponse d'aide polie, best-effort).
type telegramUpdate struct {
	Message *struct {
		Chat struct {
			ID int64 `json:"id"`
		} `json:"chat"`
		Text string `json:"text"`
	} `json:"message"`
}

// handleTelegramWebhook — POST /api/webhooks/telegram (PUBLIC, serveurs
// Telegram). Authentification par secret d'en-tête (posé au setWebhook) ;
// sans variable d'env le endpoint est fermé (même discipline que Wave).
func (a *API) handleTelegramWebhook(w http.ResponseWriter, r *http.Request) {
	secret := strings.TrimSpace(os.Getenv("TELEGRAM_WEBHOOK_SECRET"))
	if secret == "" {
		writeErrCode(w, http.StatusServiceUnavailable, "webhook_disabled",
			"Webhook Telegram désactivé (TELEGRAM_WEBHOOK_SECRET non configuré)", nil)
		return
	}
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Telegram-Bot-Api-Secret-Token")), []byte(secret)) != 1 {
		writeErrCode(w, http.StatusUnauthorized, "unauthorized", "Secret webhook Telegram invalide", nil)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeErrCode(w, http.StatusBadRequest, "bad_request", "Corps illisible", nil)
		return
	}
	var upd telegramUpdate
	if err := json.Unmarshal(body, &upd); err != nil || upd.Message == nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true}) // update ignoré : TOUJOURS 200 (Telegram ressinerait sinon)
		return
	}
	chatID := strconv.FormatInt(upd.Message.Chat.ID, 10)
	if chatID == "0" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}

	// /start <code> : consommer le pairage SOUS tgMu SEUL (jamais de verrou
	// store imbriqué), puis écrire les réglages sous le verrou store seul.
	fields := strings.Fields(upd.Message.Text)
	var code string
	if len(fields) >= 2 && fields[0] == "/start" {
		code = strings.ToUpper(strings.TrimSpace(fields[1]))
	}
	var acc string
	if code != "" {
		a.tgMu.Lock()
		if p, ok := a.tgPairings[code]; ok {
			if time.Now().Before(p.expiresAt) {
				acc = p.accountID // consommé : usage unique, quel que soit le résultat
			}
			delete(a.tgPairings, code)
			if cur, ok := a.tgAccountCode[acc]; !ok || cur == code {
				delete(a.tgAccountCode, acc)
			}
		}
		a.tgMu.Unlock()
	}

	if acc == "" {
		// Aucun code valide (démarrage nu, code expiré ou déjà consommé) :
		// réponse d'aide best-effort, puis 200 — rien n'est écrit.
		go func() {
			_ = a.telegramReply(chatID,
				"Bonjour 👋 Je suis le bot des alertes MikCloud.\n"+
					"Pour recevoir vos alertes ici, ouvrez la console MikCloud → Notifications → « Connecter Telegram » puis appuyez sur Démarrer avec le code fourni.")
		}()
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}

	a.store.Lock()
	db := a.store.Data()
	cfg := store.GetOrCreateNotifSettings(db, acc)
	cfg.TelegramChatID = chatID
	cfg.TelegramEnabled = true
	store.SetNotifSettings(db, cfg)
	a.logActivityBy(r, db, acc, "system", "Canal Telegram connecté (lien magique)")
	a.store.Save()
	a.store.Unlock()

	go func() {
		_ = a.telegramReply(chatID,
			"✅ Connecté ! Vous recevrez ici les alertes MikCloud de votre compte :\n"+
				"routeur hors ligne, retour en ligne, stock de vouchers bas, rapport quotidien.\n"+
				"(Gérez les alertes depuis la console : Notifications)")
	}()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// telegramReply — sendMessage via le bot plateforme, best-effort (l'échec
// d'une réponse n'affecte jamais le pairage déjà persisté).
func (a *API) telegramReply(chatID, text string) error {
	// sendTelegram attend (title, body) collés par un saut de ligne double :
	// passer le texte entier en titre avec un corps vide garde UN bloc.
	return telegramSendFn(notify.TelegramPlatformToken, chatID, text)
}
