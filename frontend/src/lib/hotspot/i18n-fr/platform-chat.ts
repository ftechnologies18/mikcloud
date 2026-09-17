// Fragment FR du domaine « platformChat » — clés préfixées "platformChat.".
// N°127 — inbox de l'assistant conversationnel de la vitrine : le bot
// répond aux visiteurs, les demandes transmises à un humain arrivent dans
// la vue « Conversations » de la console plateforme.
// Ne pas importer directement : passer par l'agrégateur (fusion + logique de résolution).

export const frPlatformChat: Record<string, string> = {
  "platformChat.title": "Conversations",
  "platformChat.description":
    "L'assistant de la vitrine répond aux visiteurs — les demandes transmises à un humain arrivent ici.",
  "platformChat.loadError": "Impossible de charger les conversations",
  "platformChat.loadErrorDesc": "Vérifiez votre connexion puis réessayez.",
  "platformChat.kpi.total": "Conversations",
  "platformChat.kpi.totalSub": "Tous statuts confondus",
  "platformChat.kpi.human": "Avec un humain",
  "platformChat.kpi.humanSub": "Transmises au support",
  "platformChat.kpi.unread": "Non lus",
  "platformChat.kpi.unreadSub": "Messages visiteur à lire",
  "platformChat.kpi.bot": "Bot actives",
  "platformChat.kpi.botSub": "Réponses automatiques",
  "platformChat.filter.all": "Toutes",
  "platformChat.filter.human": "Humain",
  "platformChat.filter.bot": "Bot",
  "platformChat.filter.closed": "Fermées",
  "platformChat.empty": "Aucune conversation",
  "platformChat.emptyDesc":
    "Les visiteurs du site posent leurs questions à l'assistant — les échanges apparaissent ici.",
  "platformChat.selectPrompt": "Sélectionnez une conversation",
  "platformChat.selectPromptDesc":
    "Ouvrez une conversation de la liste pour lire le fil et répondre au visiteur.",
  "platformChat.status.bot": "Bot",
  "platformChat.status.human": "Humain",
  "platformChat.status.closed": "Fermée",
  "platformChat.badge.unread": "{n} non lu{p}",
  "platformChat.thread.started": "Ouverte {ago}",
  "platformChat.thread.activity": "Dernier message {ago}",
  "platformChat.thread.you": "Vous",
  "platformChat.thread.visitor": "Visiteur",
  "platformChat.thread.bot": "Assistant",
  "platformChat.replyPlaceholder": "Écrivez votre réponse au visiteur…",
  "platformChat.reply": "Répondre",
  "platformChat.replySent": "Réponse envoyée au visiteur.",
  "platformChat.replyEmpty": "La réponse est vide.",
  "platformChat.close": "Clôturer",
  "platformChat.closeConfirmTitle": "Clôturer cette conversation ?",
  "platformChat.closeConfirmDesc":
    "Le visiteur voit un message de fin et peut ouvrir une nouvelle conversation. Le fil est conservé 30 jours puis purgé.",
  "platformChat.closeConfirmAction": "Clôturer la conversation",
  "platformChat.closed": "Conversation clôturée.",
  "platformChat.note.bot":
    "L'assistant automatique répond — vous pouvez reprendre la main à tout moment en répondant.",
  "platformChat.note.closed": "Conversation clôturée — le fil est en lecture seule.",
  "platformChat.note.autoClose":
    "Clôture automatique : sans nouveau message pendant 15 minutes, l'assistant ferme la conversation — la réponse d'un conseiller la rouvre à tout moment.",
  "platformChat.note.retention":
    "Rétention : les conversations fermées sont supprimées après 30 jours, les conversations bot inactives après 7 jours — l'inbox ne gonfle pas sous affluence.",
};
