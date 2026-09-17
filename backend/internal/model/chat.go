// chat.go — N°127 — assistant conversationnel public de la vitrine.
//
// La section « Questions fréquentes » de la vitrine a été remplacée par un
// chatbot : les visiteurs posent leurs questions dans un widget (bas de page
// du landing), l'assistant automatique répond depuis une base de
// connaissances FAQ (chatbot.go), et la conversation peut être TRANSMISE à
// un humain qui répond depuis la console plateforme (vue « Conversations »).
//
// Modèle :
//   - ChatConversation : une session visiteur. Le secret d'accès (token)
//     n'est JAMAIS stocké en clair — uniquement son SHA-256 (même garantie
//     que PasswordReset N°68) : connaître l'identifiant public ne suffit
//     pas pour lire ou écrire dans la conversation.
//   - ChatMessage : un message du fil, append-only (jamais réordonné ni
//     inséré au milieu — le front suit le fil par OFFSET, pas par horloge).
//
// Statuts : "bot" (l'assistant répond), "human" (transmise au support —
// l'assistant se tait, les messages du visiteur partent dans la file de
// l'inbox plateforme), "closed" (clôturée par le support OU
// automatiquement — N°129 : aucun message depuis 15 minutes).
//
// Clôture automatique (N°129, chatAutoCloseLocked) : une conversation
// vivante (bot OU human) sans nouveau message depuis 15 minutes est
// fermée par l'assistant — atomiquement sous le verrou, au balayage
// périodique (chat_sweep.go, chaque minute) comme à la lecture de
// l'inbox console. Un message du visiteur rouvre une conversation
// clôturée (« human » si un conseiller était intervenu, « bot » sinon).
//
// Rétention (chatPruneLocked, appelée à la création de session ET au
// balayage périodique, sous le verrou du store) : les conversations
// clôturées de plus de 30 jours et les conversations "bot" sans
// interaction depuis 7 jours sont purgées avec leurs messages —
// l'inbox du support ne gonfle pas indéfiniment ; les conversations
// "human" ne sont JAMAIS purgées automatiquement (elles passent
// "closed" par la clôture d'inactivité, puis purge à 30 jours).
package model

// ChatConversation — une conversation visiteur ↔ assistant ↔ support.
type ChatConversation struct {
	ID        string `json:"id"`                  // "chat-…" (préfixe NewID)
	TokenHash string `json:"tokenHash"`           // SHA-256 hex du secret visiteur
	Lang      string `json:"lang"`                // "fr" | "en" — langue des réponses du bot
	Status    string `json:"status"`              // bot | human | closed
	CreatedIP string `json:"createdIp,omitempty"` // anti-abus (limiteur IP)
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"` // dernier message — tri de l'inbox
	Unread    int    `json:"unread"`    // messages visiteur non lus par le support
}

// Statuts d'une conversation chat.
const (
	ChatStatusBot    = "bot"
	ChatStatusHuman  = "human"
	ChatStatusClosed = "closed"
)

// ChatMessage — un message du fil de conversation (append-only).
type ChatMessage struct {
	ID             string `json:"id"` // "cmsg-…"
	ConversationID string `json:"conversationId"`
	Sender         string `json:"sender"` // visitor | bot | agent
	Body           string `json:"body"`
	At             string `json:"at"`
}

// Expéditeurs d'un message chat.
const (
	ChatSenderVisitor = "visitor"
	ChatSenderBot     = "bot"
	ChatSenderAgent   = "agent"
)
