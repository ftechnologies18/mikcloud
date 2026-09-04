// Couche URL de la navigation (Phase A « Speed App UX ») — mapping bijectif
// entre les vues de la console (ViewId) et les segments d'URL /app/<vue>.
//
// La vue reste pilotée par le store (source de vérité unique) : ce module
// fournit uniquement la traduction ViewId ↔ chemin, utilisée par la
// synchronisation bidirectionnelle d'app-route.tsx. Le bouton Retour du
// navigateur et les liens directs (/app/users…) deviennent fonctionnels sans
// changer ni le store ni les vues.

import type { ViewId } from "./types";

/** Préfixe de toutes les routes console. */
export const APP_BASE_PATH = "/app";

/** Segment d'URL de chaque vue — kebab-case dérivé de l'identifiant. */
const VIEW_SLUGS: Record<ViewId, string> = {
  dashboard: "dashboard",
  sessions: "sessions",
  subscription: "subscription",
  users: "users",
  vouchers: "vouchers",
  templates: "templates",
  profiles: "profiles",
  resellers: "resellers",
  routers: "routers",
  reports: "reports",
  logs: "logs",
  platform: "platform",
  platformLogs: "platform-logs",
  platformTeam: "platform-team",
  platformSettings: "platform-settings",
  billingRequests: "billing-requests",
  accounts: "accounts",
  notifications: "notifications",
  settings: "settings",
  team: "team",
};

const SLUG_VIEWS: Record<string, ViewId> = Object.fromEntries(
  Object.entries(VIEW_SLUGS).map(([view, slug]) => [slug, view as ViewId]),
);

/** Chemin complet d'une vue : /app/<slug>, ou /app/<slug>/<détail> quand la
 * vue expose un élément adressable (Phase D : utilisateur, lot, session).
 * Le détail est encodé (un username peut contenir des caractères spéciaux). */
export function viewToPath(view: ViewId, detail?: string): string {
  const base = `${APP_BASE_PATH}/${VIEW_SLUGS[view]}`;
  return detail ? `${base}/${encodeURIComponent(detail)}` : base;
}

/** Vue cible d'un chemin — null si le chemin ne désigne pas une vue
 * (/app sans segment, slug inconnu, hors /app) : la vue courante du store
 * s'applique alors, et l'URL est re-normalisée par la synchro inverse.
 * Le 2e segment éventuel (détail) est ignoré ici : il appartient à la vue
 * (detailFromPath), la synchro de vue d'app-route reste inchangée. */
export function viewFromPath(pathname: string | null): ViewId | null {
  if (!pathname) return null;
  if (pathname === APP_BASE_PATH) return null;
  const prefix = `${APP_BASE_PATH}/`;
  if (!pathname.startsWith(prefix)) return null;
  const slug = pathname.slice(prefix.length).split("/")[0];
  return SLUG_VIEWS[slug] ?? null;
}

/** Vues exposant un détail adressable (Phase D) — les autres n'ont pas de
 * 2e segment valide : app-route re-normalise ces chemins orphelins. */
const DETAIL_VIEWS: ReadonlySet<ViewId> = new Set<ViewId>(["users", "vouchers", "sessions"]);

/** Identifiant de détail d'un chemin /app/<slug>/<détail> — null si absent
 * ou si la vue n'expose pas de détail. Ne lit QUE le 2e segment : les vues
 * branchent leur état local dessus (ouverture de dialog, filtre lot…)
 * sans toucher au mécanisme de navigation (fix 192ad9f préservé).
 * Un 3e segment éventuel est ignoré (le détail ne s'imbrique pas). */
export function detailFromPath(pathname: string | null, view: ViewId): string | null {
  if (!pathname || !DETAIL_VIEWS.has(view)) return null;
  const prefix = `${APP_BASE_PATH}/${VIEW_SLUGS[view]}/`;
  if (!pathname.startsWith(prefix)) return null;
  const seg = pathname.slice(prefix.length).split("/")[0];
  if (!seg) return null;
  try {
    return decodeURIComponent(seg);
  } catch {
    return seg; // segment mal encodé — rendu brut (l'objet ciblé tranchera)
  }
}
