// N°7 — Rôles équipe : miroir CLIENT de la hiérarchie serveur (requireRole).
// L'UI masque ce que le serveur refuserait de toute façon (403) — le masquage
// est purement du confort, l'autorisation réelle vit côté Go.

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
  users: 1,
  vouchers: 1,
  stats: 1, // heatmap affluence (lecture)
  templates: 2, // gestion des modèles (l'impression reste accessible à tous)
  profiles: 2,
  resellers: 2,
  routers: 2, // outils routeur (config/diagnostics) — gérant+
  reports: 2,
  logs: 2,
  notifications: 2,
  settings: 3,
  team: 3,
  accounts: 3, // + vérification isPlatformAdmin côté vue
};

/** Vues de la CONSOLE PLATEFORME — réservées au super-admin MikCloud
 * (platform_admin, ou « admin » historique). Le reste de la navigation
 * appartient à la console client. */
const PLATFORM_VIEWS: ReadonlySet<string> = new Set(["platform", "platformLogs", "platformTeam", "billingRequests"]);

/** Cette vue appartient-elle à la console plateforme ? */
export function isPlatformView(view: string): boolean {
  return PLATFORM_VIEWS.has(view) || view === "accounts";
}

/** L'utilisateur de ce rôle peut-il ouvrir cette vue ? */
export function canView(role: string | undefined, view: string): boolean {
  if (!role) return false;
  if (isPlatformView(view)) return role === "admin" || role === "platform_admin";
  const min = VIEW_MIN_RANK[view];
  if (min === undefined) return true;
  return roleRank(role) >= min;
}
