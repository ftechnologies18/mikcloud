// Fragment FR du domaine « logs » — clés préfixées "logs.".
// Extrait du monolithe i18n.ts (N°87) ; l'ordre des clés suit le fichier d'origine.
// Ne pas importer directement : passer par l'agrégateur (fusion + logique de résolution).

export const frLogs: Record<string, string> = {

  // — logs (F3) —
  "logs.title": "Journal utilisateurs",
  "logs.description": "Connexions, déconnexions, expirations et kicks — mis à jour toutes les 10 secondes",
  "logs.retentionNote": "Rétention : {days} jours maximum, purge automatique toutes les heures (même sans visite de la console). Garde-fou volumétrie : les 5 000 dernières entrées sont conservées — un site à fort trafic peut voir son journal élagué avant {days} jours.",
  "logs.searchPlaceholder": "Rechercher un utilisateur ou une IP…",
  "logs.searchLabel": "Rechercher dans le journal",
  "logs.filterByAction": "Filtrer par action",
  "logs.allActions": "Toutes les actions",
  "logs.logins": "Connexions",
  "logs.logouts": "Déconnexions",
  "logs.expirations": "Expirations",
  "logs.kicks": "Kicks",
  "logs.login": "Connexion",
  "logs.logout": "Déconnexion",
  "logs.expire": "Expiration",
  "logs.kick": "Kick",
  "logs.empty": "Aucun événement",
  "logs.emptyFiltered": "Aucune entrée ne correspond à ces filtres.",
  "logs.emptyDesc": "Les connexions et déconnexions apparaîtront ici dès qu'elles seront capturées.",
  "logs.action": "Action",
};
