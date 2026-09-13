// Fragment EN du domaine « portal » — clés préfixées "portal.".
// Extrait du monolithe i18n-en.ts (N°87) ; l'ordre des clés suit le fichier d'origine.
// Ne pas importer directement : passer par l'agrégateur (fusion + logique de résolution).

export const enPortal: Record<string, string> = {

  // — N°35-d — captive portal (Portal view) —
  "portal.title": "Captive portal",
  "portal.subtitle":
    "Automatic portal deployment on your agent routers — zero manual intervention.",
  "portal.empty": "No agent router online",
  "portal.emptyHint":
    "Agent-mode routers deploy the portal automatically. Add an agent-mode router in the Infrastructure section to enable the captive portal.",
  "portal.sigDeployed": "Portal up to date",
  "portal.sigPending": "Redeploy pending",
  "portal.sigNever": "Never deployed",
  "portal.preview": "Preview",
  "portal.previewTitle": "Portal preview — {name}",
  "portal.previewHint":
    "Static portal preview (live fetch is not active in preview — branding refreshes automatically on the router).",
  "portal.redeploy": "Redeploy",
  "portal.redeployConfirmTitle": "Portal redeployment",
  "portal.redeployConfirm":
    "Force portal redeployment on \"{name}\"? The router will reload all files (login.html, status.html, assets) at its next check-in (≤ 45 s).",
  "portal.redeployQueued": "Redeploy scheduled",
  "portal.cancel": "Cancel",
  "portal.journal": "Deployment log",
  "portal.journalEmpty": "No deployment recorded",
};
