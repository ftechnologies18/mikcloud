// Fragment FR du domaine « print » — clés préfixées "print.".
// Extrait du monolithe i18n.ts (N°87) ; l'ordre des clés suit le fichier d'origine.
// Ne pas importer directement : passer par l'agrégateur (fusion + logique de résolution).

export const frPrint: Record<string, string> = {

  // — print (uc-print-dialog) —
  "print.action": "Imprimer",
  "print.ticketsReady": "{n} ticket{p} prêt{p} à imprimer — découpez le long des cadres.",

  // N°22 — impression pour le compte des revendeurs (action tracée côté serveur).
  "print.resellerNotice": "{n} ticket{p} attribué{p} à un revendeur ({names}) — impression pour son compte, action tracée dans le journal d’activité. La vente reste créditée au revendeur.",
  "print.template": "Modèle",
  "print.templateAria": "Modèle de voucher pour l'impression",
  "print.standard": "Ticket standard MikCloud",
  "print.defaultSuffix": " (défaut)",
  "print.generating": "Génération de l'aperçu…",
  "print.previewFailed": "Aperçu indisponible — le modèle n'a pas pu être rendu.",
  "print.wifiHotspot": "WiFi Hotspot",
  "print.passwordLabel": "Mot de passe :",
  "print.validity": "validité {n} j",
  "print.ticketsReadySimple": "{n} ticket{p} prêt{p} à imprimer.",
  "print.fmtA4": "Grille A4",
  "print.fmt58": "Thermique 58 mm",
  "print.fmt80": "Thermique 80 mm",
  "print.formatAria": "Format d'impression",
  "print.keepTicket": "Gardez ce ticket pour vous connecter",
};
