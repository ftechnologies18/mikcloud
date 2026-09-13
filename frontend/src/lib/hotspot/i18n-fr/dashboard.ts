// Fragment FR du domaine « dashboard » — clés préfixées "dashboard.".
// Extrait du monolithe i18n.ts (N°87) ; l'ordre des clés suit le fichier d'origine.
// Ne pas importer directement : passer par l'agrégateur (fusion + logique de résolution).

export const frDashboard: Record<string, string> = {

  // — dashboard —
  "dashboard.title": "Tableau de bord",
  "dashboard.description": "Vue d'ensemble de votre réseau hotspot en temps réel",
  "dashboard.loadError": "Impossible de charger le tableau de bord",
  "dashboard.loadErrorDesc": "Le serveur est peut-être momentanément indisponible. Réessayez dans un instant.",
  "dashboard.kpi.activeSessions": "Sessions actives",
  "dashboard.kpi.activeUsers": "Utilisateurs actifs",
  "dashboard.kpi.activeUsersSub": "en ligne · {n} clients au total",
  "dashboard.kpi.activeVouchers": "Vouchers actifs",
  "dashboard.kpi.soldToday": "Tickets vendus",
  "dashboard.kpi.soldTodaySub": "utilisés aujourd'hui (1ʳᵉ connexion)",
  "dashboard.kpi.revenue30": "Revenu 30 jours",
  "dashboard.kpi.routers": "Routeurs",
  "dashboard.kpi.routersSub": "en ligne",
  "dashboard.sites": "Vue d'ensemble multi-sites",
  "dashboard.sitesCount": "{n} hotspot{p} rattaché{p} à votre compte",
  "dashboard.site.sessions": "Sessions",
  "dashboard.site.salesToday": "Tickets vendus",
  "dashboard.site.users": "Utilisateurs actifs",
  "dashboard.site.vouchers": "Vouchers actifs",
  "dashboard.site.revenue30": "Revenu 30 jours",
  "dashboard.live": "en direct",
  "dashboard.sessionsChart": "Connexions / heure — dernières 24 h",
  "dashboard.sessionUnit": "{n} connexion{p}",
  "dashboard.revenueChart": "Revenus — 14 derniers jours",
  "dashboard.topProfiles": "Profils les plus vendus",
  "dashboard.topProfilesEmpty": "Aucune vente enregistrée",
  "dashboard.topProfilesEmptyDesc": "Les profils les plus vendus apparaîtront ici.",
  "dashboard.usersCount": "{n} vendu{p}",
  "dashboard.activity": "Activité récente",
  "dashboard.activityEmpty": "Aucune activité récente",
  "dashboard.receivables": "Créances revendeurs",
  "dashboard.receivablesDesc": "Dépôt-vente — argent en attente de versement",
  "dashboard.receivablesTotal": "Total dû",
  "dashboard.receivablesCount": "{n} revendeur(s) avec créance",
  "dashboard.receivablesAging": "{n} j",
  "dashboard.receivablesOverCeiling": "Vente bloquée",
};
