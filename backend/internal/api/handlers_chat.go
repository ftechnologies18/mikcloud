// handlers_chat.go — N°127 — assistant conversationnel public de la
// vitrine + inbox support de la console plateforme.
//
// Côté VITRINE (public, sans JWT — le secret visiteur fait l'auth) :
//
//	POST /api/chat/session          {lang}                      → 201
//	POST /api/chat/message          {token, body, offset}       → 200
//	GET  /api/chat/messages?token=&offset=                      → 200
//	POST /api/chat/handoff          {token, offset}             → 200
//
// Côté CONSOLE PLATEFORME (requireRole 3) :
//
//	GET  /api/admin/chat/conversations             → {conversations, summary}
//	GET  /api/admin/chat/conversations/{id}        → {conversation, messages} (+ lu)
//	POST /api/admin/chat/conversations/{id}/reply  {body}         → {ok, message}
//	POST /api/admin/chat/conversations/{id}/close                 → {ok}
//
// CONTRAT DE FIL (append-only, offset) : les messages ne sont jamais
// réordonnés ni insérés au milieu — le client suit le fil par OFFSET (le
// nombre de messages qu'il connaît déjà), jamais par horloge : deux
// messages écrits dans la même seconde ne peuvent pas être ratés.
//
// SÉCURITÉ : le secret visiteur (token 40 hex) n'est stocké que sous
// forme de SHA-256 (même garantie que PasswordReset N°68) — l'identifiant
// public de conversation ne suffit ni à lire ni à écrire. Quota IP partagé
// par les trois POST publics (a.chat : 30/10 min + 400/24 h — un humain
// en conversation active reste très loin du seuil). Corps bornés
// (1 000 caractères visiteur, 2 000 agent). Rétention chatPruneLocked.
package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"mikcloud/hotspot-api/internal/model"
)

// ---------------------------------------------------------------------------
// Helpers (sous le verrou du store)
// ---------------------------------------------------------------------------

// newChatToken — génère le secret visiteur et son SHA-256 hex.
func newChatToken() (token, hash string) {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		// Repli déterministe improbable (crypto/rand en panne) : le flux
		// de temps rend le token difficilement prévisible, jamais vide.
		now := time.Now().UTC().UnixNano()
		for i := range b {
			b[i] = byte(now >> (uint(i%8) * 8))
		}
	}
	token = hex.EncodeToString(b)
	sum := sha256.Sum256([]byte(token))
	return token, hex.EncodeToString(sum[:])
}

// chatFindConvLocked — conversation par secret visiteur (pointeur DANS la
// slice, à utiliser sous le verrou). nil si inconnu.
func chatFindConvLocked(db *model.DB, token string) *model.ChatConversation {
	if token == "" {
		return nil
	}
	sum := sha256.Sum256([]byte(token))
	want := hex.EncodeToString(sum[:])
	for i := range db.ChatConversations {
		if db.ChatConversations[i].TokenHash == want {
			return &db.ChatConversations[i]
		}
	}
	return nil
}

// chatConvByIDLocked — conversation par identifiant public (console).
func chatConvByIDLocked(db *model.DB, id string) *model.ChatConversation {
	for i := range db.ChatConversations {
		if db.ChatConversations[i].ID == id {
			return &db.ChatConversations[i]
		}
	}
	return nil
}

// chatMessagesLocked — copie des messages d'une conversation, dans
// l'ordre d'ajout (append-only). offset < 0 → 0 ; offset > total → vide.
func chatMessagesLocked(db *model.DB, convID string, offset int) []model.ChatMessage {
	if offset < 0 {
		offset = 0
	}
	out := []model.ChatMessage{}
	seen := 0
	for i := range db.ChatMessages {
		m := &db.ChatMessages[i]
		if m.ConversationID != convID {
			continue
		}
		if seen < offset { // saute les `offset` premiers du fil
			seen++
			continue
		}
		out = append(out, *m)
	}
	return out
}

// chatCountMessagesLocked — nombre de messages d'une conversation.
func chatCountMessagesLocked(db *model.DB, convID string) int {
	n := 0
	for i := range db.ChatMessages {
		if db.ChatMessages[i].ConversationID == convID {
			n++
		}
	}
	return n
}

// chatPruneLocked — rétention (TOUJOURS sous le verrou, appelée à la
// création de session — moment rare où le coût O(n) est indolore) :
//   - conversations "closed" sans activité depuis 30 jours → purgées ;
//   - conversations "bot" sans activité depuis 7 jours → purgées (le
//     visiteur est parti, une conversation bot n'a aucune valeur
//     d'archive) ;
//   - conversations "human" → JAMAIS purgées automatiquement (le support
//     décide, la clôture engage la purge à 30 j) ;
//   - garde-fou mémoire : au-delà de 2 000 conversations, les plus
//     anciennes "bot" sont purgées pour rester sous le plafond.
//
// Les messages des conversations purgées disparaissent dans la même
// passe. Retourne true si l'état a changé (l'appelant Save).
func chatPruneLocked(db *model.DB, now time.Time) bool {
	drop := map[string]bool{}
	for i := range db.ChatConversations {
		conv := &db.ChatConversations[i]
		age := chatAge(conv.UpdatedAt, now)
		if conv.Status == model.ChatStatusClosed && age > 30*24*time.Hour {
			drop[conv.ID] = true
		} else if conv.Status == model.ChatStatusBot && age > 7*24*time.Hour {
			drop[conv.ID] = true
		}
	}
	// Garde-fou mémoire : plafond 2 000 conversations vivantes.
	const chatMaxConversations = 2000
	if len(db.ChatConversations)-len(drop) > chatMaxConversations {
		type aged struct {
			id  string
			at  string
			bot bool
		}
		var bots []aged
		for i := range db.ChatConversations {
			conv := &db.ChatConversations[i]
			if conv.Status != model.ChatStatusBot || drop[conv.ID] {
				continue
			}
			bots = append(bots, aged{conv.ID, conv.UpdatedAt, true})
		}
		sort.Slice(bots, func(i, j int) bool { return bots[i].at < bots[j].at })
		overflow := len(db.ChatConversations) - len(drop) - chatMaxConversations
		for i := 0; i < overflow && i < len(bots); i++ {
			drop[bots[i].id] = true
		}
	}
	if len(drop) == 0 {
		return false
	}
	convs := db.ChatConversations[:0]
	for i := range db.ChatConversations {
		if !drop[db.ChatConversations[i].ID] {
			convs = append(convs, db.ChatConversations[i])
		}
	}
	msgs := db.ChatMessages[:0]
	for i := range db.ChatMessages {
		if !drop[db.ChatMessages[i].ConversationID] {
			msgs = append(msgs, db.ChatMessages[i])
		}
	}
	db.ChatConversations = convs
	db.ChatMessages = msgs
	return true
}

// chatAge — âge d'un horodatage RFC3339 (zéro si illisible — jamais purgé
// sur donnée corrompue, le conservatisme d'abord).
func chatAge(at string, now time.Time) time.Duration {
	t, err := time.Parse(time.RFC3339, at)
	if err != nil {
		return 0
	}
	return now.Sub(t)
}

// chatAllow — quota IP partagé par les POST publics du chat. Le corps du
// 429 suit le contrat S2/S3 (Retry-After en secondes).
func (a *API) chatAllow(w http.ResponseWriter, r *http.Request) bool {
	if ok, retry := a.chat.allow(clientIP(r)); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
		writeErr(w, http.StatusTooManyRequests, "Trop de messages — patientez un instant")
		return false
	}
	return true
}

// chatOffsetFrom — offset de fil borné (query ou corps).
func chatOffsetFrom(raw string) int {
	v, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || v < 0 {
		return 0
	}
	return v
}

// chatBodyVisitor — corps d'un message visiteur : trim + bornes.
// Vide → "" (le handler répond 400).
func chatBodyVisitor(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Join(strings.Fields(s), " ") // lignes compactées, pas de \n
	if utf8.RuneCountInString(s) > 1000 {
		s = string([]rune(s)[:1000])
	}
	return s
}

// ---------------------------------------------------------------------------
// Vitrine — endpoints publics (secret visiteur)
// ---------------------------------------------------------------------------

// handleChatSession — POST /api/chat/session {lang} : ouvre une
// conversation (welcome du bot) et remet le secret au visiteur.
func (a *API) handleChatSession(w http.ResponseWriter, r *http.Request) {
	if !a.chatAllow(w, r) {
		return
	}
	var req struct {
		Lang string `json:"lang"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	lang := strings.ToLower(strings.TrimSpace(req.Lang))
	if lang != "en" {
		lang = "fr" // la vitrine est FR-first
	}

	a.store.Lock()
	db := a.store.Data()
	now := model.NowISO()
	// Rétention opportuniste (rare : une création de session).
	if chatPruneLocked(db, time.Now().UTC()) {
		defer a.store.Save()
	}
	token, hash := newChatToken()
	conv := model.ChatConversation{
		ID:        model.NewID("chat-"),
		TokenHash: hash,
		Lang:      lang,
		Status:    model.ChatStatusBot,
		CreatedIP: clientIP(r),
		CreatedAt: now,
		UpdatedAt: now,
	}
	db.ChatConversations = append(db.ChatConversations, conv)
	db.ChatMessages = append(db.ChatMessages,
		newChatMessage(conv.ID, model.ChatSenderBot, chatWelcome(lang), now))
	a.store.Save()
	msgs := chatMessagesLocked(db, conv.ID, 0)
	total := len(msgs)
	a.store.Unlock()

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":       conv.ID,
		"token":    token,
		"status":   conv.Status,
		"total":    total,
		"messages": msgs,
	})
}

// handleChatMessage — POST /api/chat/message {token, body, offset} :
// enregistre le message visiteur puis, si le bot est aux commandes,
// répond (ou transmet au support si l'intent « human » est reconnu).
// La réponse renvoie le fil depuis offset — le message visiteur confirmé
// y figure (contrat append-only, une seule source de vérité : le serveur).
func (a *API) handleChatMessage(w http.ResponseWriter, r *http.Request) {
	if !a.chatAllow(w, r) {
		return
	}
	var req struct {
		Token  string `json:"token"`
		Body   string `json:"body"`
		Offset string `json:"offset"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	body := chatBodyVisitor(req.Body)
	if body == "" {
		writeErr(w, http.StatusBadRequest, "Message vide")
		return
	}
	offset := chatOffsetFrom(req.Offset)

	a.store.Lock()
	db := a.store.Data()
	conv := chatFindConvLocked(db, req.Token)
	if conv == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Conversation introuvable")
		return
	}
	now := model.NowISO()
	db.ChatMessages = append(db.ChatMessages,
		newChatMessage(conv.ID, model.ChatSenderVisitor, body, now))
	conv.UpdatedAt = now
	if conv.Status == model.ChatStatusHuman {
		conv.Unread++ // le support n'a pas encore lu ce message
	}
	newStatus := conv.Status
	if conv.Status == model.ChatStatusBot {
		answer, intent, _ := chatBotAnswer(body, conv.Lang)
		if intent == chatIntentHuman {
			// Transmission demandée : le bot passe la main.
			conv.Status = model.ChatStatusHuman
			conv.Unread++
			db.ChatMessages = append(db.ChatMessages,
				newChatMessage(conv.ID, model.ChatSenderBot, chatHandoffMessage(conv.Lang), now))
			newStatus = conv.Status
		} else {
			db.ChatMessages = append(db.ChatMessages,
				newChatMessage(conv.ID, model.ChatSenderBot, answer, now))
		}
	}
	a.store.Save()
	msgs := chatMessagesLocked(db, conv.ID, offset)
	total := chatCountMessagesLocked(db, conv.ID)
	a.store.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"status":   newStatus,
		"total":    total,
		"messages": msgs,
	})
}

// handleChatMessages — GET /api/chat/messages?token=&offset= : polling du
// visiteur (réponses du support). Lecture seule.
func (a *API) handleChatMessages(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	offset := chatOffsetFrom(r.URL.Query().Get("offset"))

	a.store.Lock()
	db := a.store.Data()
	conv := chatFindConvLocked(db, token)
	if conv == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Conversation introuvable")
		return
	}
	msgs := chatMessagesLocked(db, conv.ID, offset)
	total := chatCountMessagesLocked(db, conv.ID)
	status := conv.Status
	a.store.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"status":   status,
		"total":    total,
		"messages": msgs,
	})
}

// handleChatHandoff — POST /api/chat/handoff {token, offset} : le
// visiteur demande explicitement un humain (bouton « Parler à un humain »).
func (a *API) handleChatHandoff(w http.ResponseWriter, r *http.Request) {
	if !a.chatAllow(w, r) {
		return
	}
	var req struct {
		Token  string `json:"token"`
		Offset string `json:"offset"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	offset := chatOffsetFrom(req.Offset)

	a.store.Lock()
	db := a.store.Data()
	conv := chatFindConvLocked(db, req.Token)
	if conv == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Conversation introuvable")
		return
	}
	if conv.Status == model.ChatStatusBot {
		now := model.NowISO()
		conv.Status = model.ChatStatusHuman
		conv.Unread++
		conv.UpdatedAt = now
		db.ChatMessages = append(db.ChatMessages,
			newChatMessage(conv.ID, model.ChatSenderBot, chatHandoffMessage(conv.Lang), now))
		a.store.Save()
	}
	msgs := chatMessagesLocked(db, conv.ID, offset)
	total := chatCountMessagesLocked(db, conv.ID)
	status := conv.Status
	a.store.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"status":   status,
		"total":    total,
		"messages": msgs,
	})
}

// ---------------------------------------------------------------------------
// Console plateforme — inbox support (requireRole 3)
// ---------------------------------------------------------------------------

// chatConvOut — ligne d'inbox (le hash du secret visiteur ne quitte
// JAMAIS le serveur).
type chatConvOut struct {
	ID          string `json:"id"`
	Lang        string `json:"lang"`
	Status      string `json:"status"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
	Unread      int    `json:"unread"`
	LastSender  string `json:"lastSender"`
	LastMessage string `json:"lastMessage"`
}

// chatStatusRank — tri de l'inbox : transmissions humaines d'abord, puis
// conversations bot vivantes, puis clôturées ; à l'intérieur de chaque
// groupe, la plus récente activité remonte.
func chatStatusRank(s string) int {
	switch s {
	case model.ChatStatusHuman:
		return 0
	case model.ChatStatusBot:
		return 1
	default:
		return 2
	}
}

// handleAdminChatConversations — GET /api/admin/chat/conversations :
// l'inbox du support (toutes conversations vivantes + résumé).
func (a *API) handleAdminChatConversations(w http.ResponseWriter, r *http.Request) {
	a.store.Lock()
	db := a.store.Data()
	// Dernier message par conversation (un passage, ordre d'ajout).
	last := map[string]model.ChatMessage{}
	for i := range db.ChatMessages {
		m := &db.ChatMessages[i]
		last[m.ConversationID] = *m
	}
	rows := []chatConvOut{}
	summary := map[string]int{"total": 0, "human": 0, "bot": 0, "closed": 0, "unread": 0}
	for i := range db.ChatConversations {
		conv := &db.ChatConversations[i]
		out := chatConvOut{
			ID:        conv.ID,
			Lang:      conv.Lang,
			Status:    conv.Status,
			CreatedAt: conv.CreatedAt,
			UpdatedAt: conv.UpdatedAt,
			Unread:    conv.Unread,
		}
		if m, ok := last[conv.ID]; ok {
			out.LastSender = m.Sender
			out.LastMessage = chatExcerpt(m.Body)
		}
		rows = append(rows, out)
		summary["total"]++
		summary[conv.Status]++
		summary["unread"] += conv.Unread
	}
	a.store.Unlock()

	sort.SliceStable(rows, func(i, j int) bool {
		ri, rj := chatStatusRank(rows[i].Status), chatStatusRank(rows[j].Status)
		if ri != rj {
			return ri < rj
		}
		if rows[i].Unread != rows[j].Unread {
			return rows[i].Unread > rows[j].Unread
		}
		return rows[i].UpdatedAt > rows[j].UpdatedAt
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"conversations": rows,
		"summary":       summary,
	})
}

// chatExcerpt — aperçu borné d'un message (liste compacte).
func chatExcerpt(s string) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) > 110 {
		return string(r[:110]) + "…"
	}
	return string(r)
}

// chatConvPublicOut — conversation sans le hash du secret.
type chatConvPublicOut struct {
	ID        string `json:"id"`
	Lang      string `json:"lang"`
	Status    string `json:"status"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// handleAdminChatConversation — GET /api/admin/chat/conversations/{id} :
// le fil complet ; ouvrir la conversation marque les messages non lus
// comme lus (Unread → 0).
func (a *API) handleAdminChatConversation(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	a.store.Lock()
	db := a.store.Data()
	conv := chatConvByIDLocked(db, id)
	if conv == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Conversation introuvable")
		return
	}
	if conv.Unread > 0 {
		conv.Unread = 0
		a.store.Save()
	}
	msgs := chatMessagesLocked(db, id, 0)
	total := len(msgs)
	out := chatConvPublicOut{
		ID:        conv.ID,
		Lang:      conv.Lang,
		Status:    conv.Status,
		CreatedAt: conv.CreatedAt,
		UpdatedAt: conv.UpdatedAt,
	}
	a.store.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"conversation": out,
		"total":        total,
		"messages":     msgs,
	})
}

// handleAdminChatReply — POST /api/admin/chat/conversations/{id}/reply
// {body} : réponse du support. La conversation passe (ou reste) « human »
// — après une intervention humaine, le bot ne reprend jamais la main ;
// répondre à une conversation clôturée la rouvre.
func (a *API) handleAdminChatReply(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Body string `json:"body"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	body := strings.Join(strings.Fields(strings.TrimSpace(req.Body)), " ")
	if body == "" {
		writeErr(w, http.StatusBadRequest, "Réponse vide")
		return
	}
	if utf8.RuneCountInString(body) > 2000 {
		body = string([]rune(body)[:2000])
	}

	a.store.Lock()
	db := a.store.Data()
	conv := chatConvByIDLocked(db, id)
	if conv == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Conversation introuvable")
		return
	}
	now := model.NowISO()
	msg := newChatMessage(conv.ID, model.ChatSenderAgent, body, now)
	db.ChatMessages = append(db.ChatMessages, msg)
	conv.Status = model.ChatStatusHuman
	conv.Unread = 0
	conv.UpdatedAt = now
	a.logActivityBy(r, db, model.AccountMainID, "system",
		"Chat support : réponse envoyée au visiteur ("+conv.Lang+")")
	a.store.Save()
	a.store.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": msg})
}

// handleAdminChatClose — POST /api/admin/chat/conversations/{id}/close :
// clôture (message de fin du bot côté visiteur, statut « closed »,
// purge automatique 30 jours plus tard).
func (a *API) handleAdminChatClose(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	a.store.Lock()
	db := a.store.Data()
	conv := chatConvByIDLocked(db, id)
	if conv == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Conversation introuvable")
		return
	}
	if conv.Status != model.ChatStatusClosed {
		now := model.NowISO()
		conv.Status = model.ChatStatusClosed
		conv.UpdatedAt = now
		db.ChatMessages = append(db.ChatMessages,
			newChatMessage(conv.ID, model.ChatSenderBot, chatClosedMessage(conv.Lang), now))
		a.logActivityBy(r, db, model.AccountMainID, "system",
			"Chat support : conversation clôturée")
		a.store.Save()
	}
	a.store.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
