// Fragment EN du domaine « platformChat » — clés préfixées "platformChat.".
// N°127 — inbox of the showcase chatbot: the bot answers visitors, requests
// handed over to a human land in the "Conversations" view of the platform
// console.
// Ne pas importer directement : passer par l'agrégateur (fusion + logique de résolution).

export const enPlatformChat: Record<string, string> = {
  "platformChat.title": "Conversations",
  "platformChat.description":
    "The showcase assistant answers visitors — requests handed over to a human land here.",
  "platformChat.loadError": "Unable to load conversations",
  "platformChat.loadErrorDesc": "Check your connection and try again.",
  "platformChat.kpi.total": "Conversations",
  "platformChat.kpi.totalSub": "All statuses combined",
  "platformChat.kpi.human": "With a human",
  "platformChat.kpi.humanSub": "Handed over to support",
  "platformChat.kpi.unread": "Unread",
  "platformChat.kpi.unreadSub": "Visitor messages to read",
  "platformChat.kpi.bot": "Active bots",
  "platformChat.kpi.botSub": "Automatic answers",
  "platformChat.filter.all": "All",
  "platformChat.filter.human": "Human",
  "platformChat.filter.bot": "Bot",
  "platformChat.filter.closed": "Closed",
  "platformChat.empty": "No conversation",
  "platformChat.emptyDesc":
    "Site visitors ask their questions to the assistant — conversations appear here.",
  "platformChat.selectPrompt": "Select a conversation",
  "platformChat.selectPromptDesc":
    "Open a conversation from the list to read the thread and reply to the visitor.",
  "platformChat.status.bot": "Bot",
  "platformChat.status.human": "Human",
  "platformChat.status.closed": "Closed",
  "platformChat.badge.unread": "{n} unread",
  "platformChat.thread.started": "Opened {ago}",
  "platformChat.thread.activity": "Last message {ago}",
  "platformChat.thread.you": "You",
  "platformChat.thread.visitor": "Visitor",
  "platformChat.thread.bot": "Assistant",
  "platformChat.replyPlaceholder": "Write your reply to the visitor…",
  "platformChat.reply": "Reply",
  "platformChat.replySent": "Reply sent to the visitor.",
  "platformChat.replyEmpty": "The reply is empty.",
  "platformChat.close": "Close",
  "platformChat.closeConfirmTitle": "Close this conversation?",
  "platformChat.closeConfirmDesc":
    "The visitor sees an end message and can start a new conversation. The thread is kept for 30 days then purged.",
  "platformChat.closeConfirmAction": "Close the conversation",
  "platformChat.closed": "Conversation closed.",
  "platformChat.note.bot":
    "The automatic assistant is answering — you can take over at any time by replying.",
  "platformChat.note.closed": "Conversation closed — the thread is read-only.",
  "platformChat.note.autoClose":
    "Auto-close: after 15 minutes without a new message, the assistant closes the conversation — an advisor's reply reopens it at any time.",
  "platformChat.note.retention":
    "Retention: closed conversations are deleted after 30 days, idle bot conversations after 7 days — the inbox stays lean under heavy traffic.",
};
