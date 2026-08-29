// N°7 — Rôles équipe : miroir CLIENT de la hiérarchie serveur (requireRole).
// L'UI masque ce que le serveur refuserait de toute façon (403) — le masquage
// est purement du confort, l'autorisation réelle vit côté Go.

export type TeamRole = "operator" | "manager" | "owner" | "platform_admin" | "admin";

/** Rang : operator(1) < manager(2) < owner(3) ≈ platform admin(3). Inconnu → 0. */
export function roleRank(role: string): number {
  switch (role) {
    case "operator":
      return 1;
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

/** L'utilisateur de ce rôle peut-il ouvrir cette vue ? */
export function canView(role: string | undefined, view: string): boolean {
  if (!role) return false;
  if (view === "accounts") return role === "admin" || role === "platform_admin";
  const min = VIEW_MIN_RANK[view];
  if (min === undefined) return true;
  return roleRank(role) >= min;
}
