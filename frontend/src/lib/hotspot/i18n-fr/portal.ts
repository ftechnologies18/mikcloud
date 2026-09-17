// Fragment FR du domaine « portal » — clés préfixées "portal.".
// Extrait du monolithe i18n.ts (N°87) ; l'ordre des clés suit le fichier d'origine.
// Ne pas importer directement : passer par l'agrégateur (fusion + logique de résolution).

export const frPortal: Record<string, string> = {

  // — N°35-d — portail captif (vue Portail) —
  "portal.title": "Portail captif",
  "portal.subtitle":
    "Déploiement automatique du portail sur vos routeurs agents — zéro intervention humaine.",
  "portal.empty": "Aucun routeur agent en ligne",
  "portal.emptyHint":
    "Les routeurs en mode agent déploient le portail automatiquement. Ajoutez un routeur en mode agent dans la section Infrastructure pour activer le portail captif.",
  "portal.sigDeployed": "Portail à jour",
  "portal.sigPending": "Re-déploiement en attente",
  "portal.sigNever": "Jamais déployé",
  "portal.preview": "Aperçu",
  "portal.previewTitle": "Aperçu du portail — {name}",
  "portal.previewHint":
    "Aperçu statique du portail (le fetch live n'est pas actif en aperçu — le branding se rafraîchit automatiquement côté routeur).",
  "portal.redeploy": "Re-déployer",
  "portal.redeployConfirmTitle": "Re-déploiement du portail",
  "portal.redeployConfirm":
    "Forcer le re-déploiement du portail sur «{name}» ? Le routeur rechargera tous les fichiers (login.html, status.html, assets) à son prochain check-in (≤ 45 s).",
  "portal.redeployQueued": "Re-déploiement programmé",
  "portal.cancel": "Annuler",
  // N°135 — le logo du portail est celui du client : où le poser.
  "portal.brandingNote":
    "Le logo affiché sur le portail est celui de votre établissement : posez-le dans l'onglet Expérience (carte « Vouchers ») — la bannière d'en-tête, la liste « Nos Services » et le bandeau animé sous le logo se règlent dans la même zone. Tout changement de branding est re-déployé automatiquement sur vos routeurs (≤ 45 s).",
  "portal.journal": "Journal des déploiements",
  "portal.journalEmpty": "Aucun déploiement enregistré",
};
