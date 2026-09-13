// Fragment FR du domaine « hourly » — clés préfixées "hourly.".
// Extrait du monolithe i18n.ts (N°87) ; l'ordre des clés suit le fichier d'origine.
// Ne pas importer directement : passer par l'agrégateur (fusion + logique de résolution).

export const frHourly: Record<string, string> = {

  // — N°10 : affluence par tranche horaire —
  "hourly.title": "Affluence par tranche horaire",
  "hourly.subtitleLogins": "{n} connexion{p} sur {d} jours",
  "hourly.subtitleSales": "CA horaire cumulé sur {d} jours",
  "hourly.subtitleLoginsN": "{n} connexions sur {d} jours",
  "hourly.peak": "Pic à {h}h",
  "hourly.metricLabel": "Métrique horaire",
  "hourly.metricLogins": "Connexions",
  "hourly.metricSales": "Ventes",
  "hourly.byHour": "Cumul par heure (tous jours confondus)",
  "hourly.loginsUnit": "{n} connexion{p}",
  "hourly.legend": "Moins → plus de connexions",
  "hourly.today": "Auj.",
  "hourly.yesterday": "Hier",
  "hourly.cellTip": "{d} {h}h — {n} connexion{p}",
  "hourly.subtitleSalesMoney": "CA horaire cumulé sur {d} jours",
};
