// Fragment EN du domaine « portal » — clés préfixées "portal.".
// Extrait du monolithe i18n-en.ts (N°87) ; l'ordre des clés suit le fichier d'origine.
// Ne pas importer directement : passer par l'agrégateur (fusion + logique de résolution).

export const enPortal: Record<string, string> = {

  // — N°35-d — captive portal (Portal view) —
  // N°186 — dead-key purge: title/subtitle (pre-N°57-d standalone view)
  // and the old textarea-dialog labels (customizeTitle/customizeHint/
  // resetOverride/identitySection, bannerUrl/style/styleInherit/welcome/
  // whatsapp*/ticker/services/slides) — replaced by the unified editor.
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
  // N°184 — the Experience tab is gone (merged into the Portail tab): the
  // account's logo and branding live in the "Account portal" section at
  // the top of the Portail tab.
  "portal.brandingNote":
    "The logo shown on the portal is your establishment's own: set it in the “Account portal” section at the top of the Portail tab — the header banner, the “Our Services” list and the animated banner under the logo are configured in the same form. Any branding change is redeployed to your routers automatically (≤ 45 s).",
  "portal.journal": "Deployment log",
  "portal.journalEmpty": "No deployment recorded",

  // — N°182 — physical sites + per-site/per-router portal branding —
  // N°186 — the note mentions the "You are customizing" selector (unified
  // account / site / router editor at the top of the tab).
  "portal.chainNote":
    "Each router serves the first portal found up its chain: its own customization → its site's → the account's. Pick the level to edit with “You are customizing”: every non-customized group inherits from the level above, and every change redeploys automatically (≤ 45 s).",
  "portal.sites": "Sites",
  "portal.sitesHint":
    "Group your routers per establishment (building, shop, campus) and give each site its own portal identity.",
  "portal.newSite": "New site",
  "portal.noSites":
    "No site yet — all your routers serve the account's portal.",
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
  "portal.siteCreatedDesc": "The editor opens on its portal: customize it group by group.",
  "portal.siteUpdated": "Site updated",
  "portal.siteName": "Site name",
  "portal.siteLocation": "Location (city, area)",
  "portal.siteDescription": "Description",
  "portal.siteCustomize": "Site portal",
  "portal.siteDialogBrandingHint":
    "The site's portal identity (logo, banner, services, messages…) is customized in the “You are customizing” editor at the top of the tab — the “Site portal” button on its card switches there.",
  "portal.routerSection": "Routers",
  "portal.routerSite": "Site",
  "portal.routerNoSite": "No site (account portal)",
  "portal.regimeAccount": "Portal: account",
  "portal.regimeSite": "Portal: site",
  "portal.regimeCustom": "Portal: custom",
  "portal.customize": "Customize",
  "portal.overrideSaved": "Router portal updated — redeploy ≤ 45 s",
  "portal.siteSaved": "Site saved — portals redeployed automatically (≤ 45 s)",
  "portal.assignSaved": "Router attached to the site",
  "portal.displayName": "Name shown on the portal",
  "portal.logoUrl": "Logo (data:image/… ≤ 300 KB)",
  "portal.waveLink": "Wave merchant link",
  "portal.styleCommercial": "Commercial",
  "portal.styleHospitality": "Hospitality",
  "portal.save": "Save",

  // — N°186 — UNIFIED editor with context selector (Account / Site /
  // Router) — the same rich bricks at all three levels, inheritance made
  // visible (resolved value + per-group source). —
  "portal.editorSection": "Portal editor — Account · Site · Router chain",
  "portal.ctxLabel": "You are customizing:",
  "portal.ctxPlaceholder": "Account, site or router…",
  "portal.ctxGroupAccount": "Account",
  "portal.ctxAccountItem": "Account — {name}",
  "portal.editorHintAccount": "The base of the chain: sites then routers inherit these settings.",
  "portal.editorHintSite": "Non-customized groups inherit from the account — {n} router(s) serve this portal.",
  "portal.editorHintRouter": "Non-customized groups inherit from {source}, then the account.",
  "portal.editorHintNone": "Pick the level to customize: each group first shows the inherited value.",
  "portal.editorEmptyTitle": "Choose the portal to customize",
  "portal.editorEmptyHint":
    "Pick a site or router in the selector above, or click “Customize” on a card below. Each group shows the value inherited from the level above before customizing.",
  "portal.editorSiteTitle": "Portal of site \"{name}\"",
  "portal.editorRouterTitle": "Portal of router \"{name}\"",
  "portal.editorOverrideDesc":
    "Customize group by group: an “Inherited” group follows the level above, a “Customized” group applies here. One button saves everything (Ctrl+Enter works too).",
  "portal.editorOverrideCardDesc":
    "Each group left “Inherited” follows the level above; “Customize” pre-fills it with the inherited value, “Reset” gives it back. Saving redeploys the portal at the next check-in (≤ 45 s).",
  "portal.editorSiteSaved": "Site portal updated — redeploy ≤ 45 s",
  "portal.navIdentity": "Identity",
  "portal.identityCard": "Portal identity",
  "portal.identityCardDesc": "Display name, logo and Wave merchant link specific to this level.",
  "portal.identityNamePlaceholder": "Name shown on the portal",
  "portal.identityLogoHint":
    "Image ≤ 300 KB, embedded (data:image/…) — replaces the account logo at this level.",
  "portal.groupInherited": "Inherited",
  "portal.groupPersonalized": "Customized",
  "portal.groupPersonalize": "Customize",
  "portal.groupReset": "Reset (inherit)",
  "portal.inheritedFromAccount": "Inherited from the account",
  "portal.inheritedFromSite": "Inherited from site \"{name}\"",
  "portal.summaryNothing": "Nothing set above — the portal keeps its defaults.",
  "portal.summarySlides": "{n} slide(s)",
  "portal.summaryPromos": "{n} promo(s)",
  "portal.summarySocials": "{n} network(s)",
  "portal.summaryWave": "Wave linked",
  "portal.removeOverride": "Remove all customization",
  "portal.removeOverrideTitle": "Remove all customization?",
  "portal.removeOverrideDesc":
    "All customized groups at this level will inherit from the level above again — automatic redeploy at the next check-in (≤ 45 s).",
  "portal.overrideRemoved": "Customization removed — the portal inherits again (≤ 45 s)",
  "portal.ctxGuardTitle": "Switch portal?",
  "portal.ctxGuardDesc":
    "{n} unsaved change(s) on the portal being edited will be lost.",
};
