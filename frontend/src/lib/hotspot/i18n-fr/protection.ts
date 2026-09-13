// Fragment FR du domaine « protection » — clés préfixées "protection.".
// Extrait du monolithe i18n.ts (N°87) ; l'ordre des clés suit le fichier d'origine.
// Ne pas importer directement : passer par l'agrégateur (fusion + logique de résolution).

export const frProtection: Record<string, string> = {

  // — protection (N°83 : vue Protection, bandeau tableau de bord, résumé fiche routeur) —
  "protection.title": "Protection",
  "protection.description": "L'état de sécurité de votre WiFi, en un coup d'œil",
  "protection.verdict.protected": "Bien protégé",
  "protection.verdict.partial": "À renforcer",
  "protection.verdict.unprotected": "Non protégé",
  "protection.loadError": "Impossible de charger la protection",
  "protection.loadErrorDesc": "Vérifiez votre connexion, puis réessayez.",
  "protection.empty": "Aucun routeur à protéger",
  "protection.emptyDesc": "Ajoutez un routeur en mode agent pour activer les protections.",
  "protection.openRouters": "Ajouter un routeur",
  "protection.selectRouter": "Routeur",
  "protection.ofModules": "{n}/4 protections actives",
  "protection.agentHint": "Les protections nécessitent un routeur en mode agent — demandez la réinstallation du script MikCloud sur ce site.",
  "protection.openRouter": "Voir la fiche du routeur",
  "protection.summary.title": "Protection du WiFi",
  "protection.summary.desc": "Sites dangereux, anti-piratage, couvre-feu et bloque-VPN se pilotent désormais depuis la vue Protection.",
  "protection.summary.cta": "Ouvrir la vue Protection",
  "protection.banner.protectedTitle": "Votre WiFi est protégé",
  "protection.banner.partialTitle": "Renforcez la protection de votre WiFi",
  "protection.banner.unprotectedTitle": "Votre WiFi n'est pas protégé",
  "protection.banner.count": "{on}/{total} protections actives · {routers} routeur{s}",
  "protection.banner.cta": "Gérer la protection",

  // — N°96 (refonte UX/UI de la vue Protection) —
  // Anneau de score (libellé accessible), état inactif des cartes modules,
  // option recommandée SafeWiFi, titre des notes « Bon à savoir » (popover),
  // intitulé du bloc planning FamilyGuard et encarts du héros (verdict).
  "protection.scoreAria": "{n} protections actives sur 4",
  "protection.stateOff": "Inactif",
  "protection.recommended": "Recommandé",
  "protection.detailsTitle": "Bon à savoir",
  "protection.scheduleTitle": "Planning",
  "protection.hero.allOn": "Les 4 protections veillent sur votre WiFi public.",
  "protection.hero.missing": "À activer : {modules}",
  "protection.hero.noneOn": "Aucune protection active — commencez par le filtrage des sites dangereux.",
};
