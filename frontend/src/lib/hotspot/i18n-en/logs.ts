// Fragment EN du domaine « logs » — clés préfixées "logs.".
// Extrait du monolithe i18n-en.ts (N°87) ; l'ordre des clés suit le fichier d'origine.
// Ne pas importer directement : passer par l'agrégateur (fusion + logique de résolution).

export const enLogs: Record<string, string> = {

  // — logs —
  "logs.title": "User log",
  "logs.description": "Logins, logouts, expirations and kicks — refreshed every 10 seconds",
  "logs.retentionNote": "Retention: {days} days maximum, automatic hourly sweep (even with no console visits). Volume cap: the last 5,000 entries are kept — a high-traffic site may see its log trimmed before {days} days.",
  "logs.searchPlaceholder": "Search for a user or IP…",
  "logs.searchLabel": "Search the log",
  "logs.filterByAction": "Filter by action",
  "logs.allActions": "All actions",
  "logs.logins": "Logins",
  "logs.logouts": "Logouts",
  "logs.expirations": "Expirations",
  "logs.kicks": "Kicks",
  "logs.login": "Login",
  "logs.logout": "Logout",
  "logs.expire": "Expired",
  "logs.kick": "Kick",
  "logs.empty": "No events",
  "logs.emptyFiltered": "No entries match these filters.",
  "logs.emptyDesc": "Logins and logouts will appear here as soon as they are captured.",
  "logs.action": "Action",
};
