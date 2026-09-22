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
  // N°135 — the portal logo is the client's own: where to set it.
  "portal.brandingNote":
    "The logo shown on the portal is your establishment's own: set it in the Experience tab (\"Vouchers\" card) — the header banner, the \"Our Services\" list and the animated banner under the logo are configured in the same area. Any branding change is redeployed to your routers automatically (≤ 45 s).",
  "portal.journal": "Deployment log",
  "portal.journalEmpty": "No deployment recorded",

  // — N°182 — physical sites + per-site/per-router portal branding —
  "portal.chainNote":
    "Each router serves the first portal found up its chain: its own customization → its site's → the account's. An empty field always inherits from the level above; every change redeploys automatically (≤ 45 s).",
  "portal.sites": "Sites",
  "portal.sitesHint":
    "Group your routers per establishment (building, shop, campus) and give each site its own portal identity.",
  "portal.newSite": "New site",
  "portal.noSites":
    "No site yet — all your routers serve the account's unified portal (Experience tab settings).",
  "portal.siteRouters": "{count} router(s)",
  "portal.sitePortalCustom": "Custom portal",
  "portal.sitePortalAccount": "Account portal",
  "portal.siteEdit": "Edit",
  "portal.siteDelete": "Delete",
  "portal.siteDeleteTitle": "Delete site \"{name}\"?",
  "portal.siteDeleteConfirm":
    "Its {count} router(s) will be detached and serve the account portal again — automatic update at next check-in (≤ 45 s).",
  "portal.siteDeleted": "Site deleted",
  "portal.siteCreated": "Site created",
  "portal.siteUpdated": "Site updated",
  "portal.siteName": "Site name",
  "portal.siteLocation": "Location (city, area)",
  "portal.siteDescription": "Description",
  "portal.identitySection": "Portal identity — empty = inherits from the account",
  "portal.routerSection": "Routers",
  "portal.routerSite": "Site",
  "portal.routerNoSite": "No site (account portal)",
  "portal.regimeAccount": "Portal: account",
  "portal.regimeSite": "Portal: site",
  "portal.regimeCustom": "Portal: custom",
  "portal.customize": "Customize",
  "portal.customizeTitle": "Portal of router \"{name}\"",
  "portal.customizeHint":
    "These settings override, for THIS router, its site's then the account's. An empty field inherits — the portal redeploys automatically (≤ 45 s).",
  "portal.resetOverride": "Reset (inherit)",
  "portal.overrideSaved": "Router portal updated — redeploy ≤ 45 s",
  "portal.siteSaved": "Site saved — portals redeployed automatically (≤ 45 s)",
  "portal.assignSaved": "Router attached to the site",
  "portal.displayName": "Name shown on the portal",
  "portal.logoUrl": "Logo (data:image/… ≤ 300 KB)",
  "portal.bannerUrl": "Banner (data:image/… or https:// URL)",
  "portal.waveLink": "Wave merchant link",
  "portal.style": "Portal style",
  "portal.styleInherit": "Inherit",
  "portal.styleCommercial": "Commercial",
  "portal.styleHospitality": "Hospitality",
  "portal.welcome": "Welcome message (hospitality mode)",
  "portal.whatsappNumber": "Support WhatsApp (digits, 8-15)",
  "portal.whatsappLabel": "WhatsApp label",
  "portal.ticker": "Animated banner messages (one per line, max 5)",
  "portal.services": "\"Our Services\" lines (one per line, max 6)",
  "portal.slides": "Carousel slides (https URL per line, max 3)",
  "portal.save": "Save",
};
