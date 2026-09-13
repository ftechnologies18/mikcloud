// N°7 — Rôles équipe : miroir CLIENT de la hiérarchie serveur (requireRole).
// L'UI masque ce que le serveur refuserait de toute façon (403/404) — le
// masquage est purement du confort, l'autorisation réelle vit côté Go.

import type { AccountUsage } from "./types";

export type TeamRole = "manager" | "owner" | "platform_admin" | "admin";

/** Rang : manager(2) < owner(3) ≈ platform admin(3). Inconnu → 0.
 * Le rôle « operator » a été retiré du produit (les comptes existants sont
 * promus manager par la migration store.migrateRemoveOperator côté serveur). */
export function roleRank(role: string): number {
  switch (role) {
    case "manager":
      return 2;
    case "owner":
    case "platform_admin":
    case "admin": // rôle historique (pré-renommage), toujours accepté
      return 3;
    default:
      return 0;
  }
}

/** Vues accessibles par rang minimal — reflet des routes requireRole côté API. */
const VIEW_MIN_RANK: Record<string, number> = {
  dashboard: 1,
  sessions: 1,
  protection: 2, // N°83 — vue Protection : statut + contrôles sécurité (miroir de l'ancien onglet Système de la fiche routeur, gérant+ ; PUT safewifi/shield/familyguard derrière le JWT + accountScope)
  users: 1,
  registrations: 2, // N°27 — file des demandes + liens (API requireRole(2) : manager+)
  vouchers: 1,
  stats: 1, // heatmap affluence (lecture)
  templates: 2, // gestion des modèles (l'impression reste accessible à tous)
  profiles: 2,
  resellers: 2,
  routers: 2, // outils routeur (config/diagnostics) — gérant+
  portal: 2, // N°35-d — portail captif (aperçu + re-déploiement) — gérant+
  reports: 2,
  logs: 2,
  notifications: 2,
  settings: 3,
  // N°100 — Abonnement : lecture ouverte à tout rôle authentifié (GET
  // /api/subscription sans restriction serveur, section partagée par les
  // DEUX consoles — une entrée explicite évite que le repli default-open
  // ci-dessous ne ferme la vue aux comptes homenet, dont la facturation
  // vit AUSSI dans la zone Paramètres).
  subscription: 1,
  hotspot: 2, // N°57-d — hub Hotspot : l'expérience (PUT /api/settings, rang 3) y est masquée pour le gérant, l'onglet Portail/Modèles (rang 2) reste accessible
  security: 3, // N°57-d — mot de passe + 2FA (ex-onglet Avancé de la vue Paramètres, rang 3)
  team: 3,
  accounts: 3, // + vérification isPlatformAdmin côté vue
  // N°100 — console HomeNet : lecture pure, tout rôle authentifié.
  home: 1,
  devices: 1,
};

/** Vues de la CONSOLE PLATEFORME — réservées au super-admin MikCloud
 * (platform_admin, ou « admin » historique). Le reste de la navigation
 * appartient à la console client. */
const PLATFORM_VIEWS: ReadonlySet<string> = new Set(["platform", "platformLogs", "platformTeam", "billingRequests", "platformSettings"]);

/** Cette vue appartient-elle à la console plateforme ? */
export function isPlatformView(view: string): boolean {
  return PLATFORM_VIEWS.has(view) || view === "accounts";
}

/** N°100 — vues de la CONSOLE HOMENET (foyer). Liste FERMÉE : tout ce
 * qui n'y figure pas est invisible pour un compte homenet — la polarité
 * de la console hotspot (default-open, un ViewId non enregistré y est
 * visible par tous) est volontairement INVERSÉE ici. Un foyer ne doit
 * jamais tomber sur « Vouchers » ou « Revendeurs » : ces vues n'existent
 * simplement pas pour lui, miroir client des 404 serveur (requireUsage,
 * N°98 — « l'UI masque ce que le serveur refuserait de toute façon »).
 * Contenu : la nav maison (home, devices, protection) + les sections de
 * la zone Paramètres partagées avec la console hotspot (général, sécurité,
 * routeurs, notifications, abonnement, équipe) — les endpoints
 * correspondants sont OUVERTS aux deux usages côté Go. La section Hotspot
 * de la zone (hub/portail/modèles) en est exclue : produit hotspot. */
const HOMENET_VIEWS: ReadonlySet<string> = new Set([
  "home",
  "devices",
  "protection",
  "settings",
  "security",
  "routers",
  "notifications",
  "subscription",
  "team",
]);

/** N°100 — cette vue existe-t-elle pour la console homenet ? */
export function isHomeNetView(view: string): boolean {
  return HOMENET_VIEWS.has(view);
}

/** Usage effectif d'un utilisateur de session — absent/vide (admin
 * plateforme, sessions antérieures à N°98) = hotspot, le comportement
 * historique. */
export function usageOf(usage: AccountUsage | "" | undefined): AccountUsage {
  return usage === "homenet" ? "homenet" : "hotspot";
}

/** L'utilisateur de ce rôle peut-il ouvrir cette vue ?
 * N°100 — `usage` (dérivé du compte de session) choisit la CONSOLE :
 * hotspot = comportement historique default-open (une vue non enregistrée
 * est visible — la barrière réelle vit côté serveur) ; homenet = liste
 * fermée HOMENET_VIEWS (une vue inconnue n'y est JAMAIS visible). Les
 * vues maison (home, devices) sont réciproquement invisibles des comptes
 * hotspot — chaque console ne montre QUE son monde. */
export function canView(
  role: string | undefined,
  view: string,
  usage: AccountUsage | "" | undefined = "hotspot",
): boolean {
  if (!role) return false;
  if (isPlatformView(view)) return role === "admin" || role === "platform_admin";
  if (usage === "homenet") {
    if (!HOMENET_VIEWS.has(view)) return false; // liste fermée — default-CLOSED
  } else if (view === "home" || view === "devices") {
    return false; // vues maison : réservées à la console homenet
  }
  const min = VIEW_MIN_RANK[view];
  if (min === undefined) return usage !== "homenet"; // default-open hotspot uniquement
  return roleRank(role) >= min;
}
