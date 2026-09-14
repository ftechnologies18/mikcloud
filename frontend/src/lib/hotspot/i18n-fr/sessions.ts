// Fragment FR du domaine « sessions » — clés préfixées "sessions.".
// Extrait du monolithe i18n.ts (N°87) ; l'ordre des clés suit le fichier d'origine.
// Ne pas importer directement : passer par l'agrégateur (fusion + logique de résolution).

export const frSessions: Record<string, string> = {

  // — sessions —
  "sessions.title": "Sessions actives",
  "sessions.description": "Clients connectés en direct sur vos points d'accès",
  "sessions.live": "Direct",
  "sessions.searchPlaceholder": "Filtrer par utilisateur…",
  "sessions.refresh": "Actualisation",
  "sessions.refreshLabel": "Période d'actualisation",
  "sessions.kpi.sessions": "Sessions",
  "sessions.kpi.sessionsSub": "clients connectés",
  "sessions.kpi.download": "Trafic descendant",
  "sessions.kpi.downloadSub": "cumul téléchargé",
  "sessions.kpi.upload": "Trafic montant",
  "sessions.kpi.uploadSub": "cumul envoyé",
  "sessions.empty": "Aucune session active",
  "sessions.emptyDesc": "Les clients connectés apparaîtront ici en temps réel.",
  "sessions.connectedSince": "Connecté depuis",
  "sessions.kick": "Déconnecter",
  "sessions.kickAria": "Déconnecter {name}",
  "sessions.kickTitle": "Déconnecter {name} ?",
  "sessions.kickDesc": "La session sera coupée sur le routeur.",
  "sessions.kickPending": "Déconnexion…",
  "sessions.kicked": "Session de {name} terminée",
  "sessions.theUser": "l'utilisateur",
  "sessions.throttled": "Bridé",
  "sessions.throttledTitle": "Quota de données épuisé : le débit est réduit (mode bridage) jusqu'à l'expiration du temps de la session.",
};
