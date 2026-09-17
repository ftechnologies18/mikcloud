// Fragment FR du domaine « nav » — clés préfixées "nav.".
// Extrait du monolithe i18n.ts (N°87) ; l'ordre des clés suit le fichier d'origine.
// Ne pas importer directement : passer par l'agrégateur (fusion + logique de résolution).

export const frNav: Record<string, string> = {

  // — nav (app-shell) —
  "nav.dashboard": "Tableau de bord",
  "nav.sessions": "Sessions actives",
  "nav.protection": "Protection",
  "nav.subscription": "Abonnement",
  "nav.users": "Utilisateurs",
  "nav.vouchers": "Vouchers",
  "nav.templates": "Modèles",
  "nav.profiles": "Profils",
  "nav.resellers": "Revendeurs",
  "nav.routers": "Routeurs",
  "nav.portal": "Portail",
  "nav.reports": "Rapports",
  "nav.logs": "Journal",
  "nav.accounts": "Comptes",
  "nav.notifications": "Notifications",
  "nav.settings": "Paramètres",
  "nav.section.general": "Général",
  "nav.section.hotspot": "Hotspot",
  "nav.section.supervision": "Supervision",
  "nav.section.people": "Personnes",
  "nav.section.distribution": "Distribution",
  "nav.section.exploitation": "Exploitation",
  "nav.section.commercial": "Facturation & Ventes",
  "nav.section.infrastructure": "Infrastructure",
  "nav.section.analysis": "Analyse",
  "nav.section.system": "Système",
  "nav.main": "Navigation principale",
  "nav.openMenu": "Ouvrir le menu",

  // — console plateforme (admin plateforme uniquement) —
  "nav.section.platform": "Plateforme",
  "nav.platform": "Vue d'ensemble",
  // N°117 — parc routeurs global (flotte tous comptes).
  "nav.platformFleet": "Parc routeurs",
  "nav.platformChat": "Conversations",
  "nav.billingRequests": "Demandes",
  "nav.platformLogs": "Journal plateforme",
  "nav.platformTeam": "Équipe plateforme",
  "nav.platformSettings": "Paramètres plateforme",

  // — N°7 : équipe & rôles —
  "nav.team": "Équipe",

  // — N°27 — inscriptions publiques par QR code (vue Inscriptions) —
  "nav.registrations": "Inscriptions",

  // — N°100 : console HomeNet (foyer) —
  "nav.section.home": "Votre maison",
  "nav.home": "Tableau de bord",
  "nav.devices": "Appareils",
};
