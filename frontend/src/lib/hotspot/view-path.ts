// Couche URL de la navigation (Phase A « Speed App UX ») — mapping bijectif
// entre les vues de la console (ViewId) et les segments d'URL /app/<vue>.
//
// La vue reste pilotée par le store (source de vérité unique) : ce module
// fournit uniquement la traduction ViewId ↔ chemin, utilisée par la
// synchronisation bidirectionnelle d'app-route.tsx. Le bouton Retour du
// navigateur et les liens directs (/app/users…) deviennent fonctionnels sans
// changer ni le store ni les vues.
//
// N°57 — zone Paramètres : les vues de configuration (modèles, routeurs,
// portail, notifications, équipe) vivent sous /app/settings/<section>
// (segment IMBRIQUÉ). Les chemins historiques (/app/templates, /app/team…)
// restent deep-linkables : la vue est résolue via LEGACY_SLUG_VIEWS puis
// l'URL est re-normalisée (replace, zéro entrée d'historique parasite) par
// app-route — même mécanique que la fusion N°30 « registrations → hub ».

import type { ViewId } from "./types";

/** Préfixe de toutes les routes console. */
export const APP_BASE_PATH = "/app";

/** Segment d'URL de chaque vue — kebab-case dérivé de l'identifiant.
 * N°57 : les vues de la zone Paramètres portent un segment IMBRIQUÉ
 * « settings/<section> » ; « settings » est la racine de la zone (la vue
 * Paramètres historique, onglets internes Général/Hotspot/Avancé). */
const VIEW_SLUGS: Record<ViewId, string> = {
  dashboard: "dashboard",
  sessions: "sessions",
  subscription: "subscription",
  users: "users",
  registrations: "registrations",
  vouchers: "vouchers",
  templates: "settings/templates",
  profiles: "profiles",
  resellers: "resellers",
  wifi: "wifi",
  routers: "settings/routers",
  portal: "settings/portal",
  reports: "reports",
  logs: "logs",
  platform: "platform",
  platformLogs: "platform-logs",
  platformTeam: "platform-team",
  platformSettings: "platform-settings",
  billingRequests: "billing-requests",
  accounts: "accounts",
  notifications: "settings/notifications",
  settings: "settings",
  team: "settings/team",
};

const SLUG_VIEWS: Record<string, ViewId> = Object.fromEntries(
  Object.entries(VIEW_SLUGS).map(([view, slug]) => [slug, view as ViewId]),
);

/** Slugs historiques (pré-N°57) des vues déplacées dans la zone Paramètres :
 * /app/<slug> résout toujours vers sa vue (signets, historique navigateur,
 * liens sortants) — app-route replace ensuite vers le chemin canonique
 * /app/settings/<section>. Une section de zone inconnue (/app/settings/xyz)
 * retombe quant à elle sur la racine « settings » par la résolution simple. */
const LEGACY_SLUG_VIEWS: Record<string, ViewId> = {
  templates: "templates",
  routers: "routers",
  portal: "portal",
  notifications: "notifications",
  team: "team",
};

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
 * N°57 : la paire de segments est essayée d'abord (settings/<section>) —
 * les vues à détail (users/<id>, vouchers/<lot>) ne matchent jamais une
 * paire connue ; puis le slug simple, y compris legacy (re-normalisé
 * ensuite). Le 2e segment éventuel (détail) est ignoré ici : il appartient à
 * la vue (detailFromPath), la synchro de vue d'app-route reste inchangée. */
export function viewFromPath(pathname: string | null): ViewId | null {
  if (!pathname) return null;
  if (pathname === APP_BASE_PATH) return null;
  const prefix = `${APP_BASE_PATH}/`;
  if (!pathname.startsWith(prefix)) return null;
  const segs = pathname.slice(prefix.length).split("/");
  // Segment imbriqué — section de la zone Paramètres (settings/<section>).
  if (segs.length >= 2) {
    const nested = SLUG_VIEWS[`${segs[0]}/${segs[1]}`];
    if (nested) return nested;
  }
  const slug = segs[0];
  return SLUG_VIEWS[slug] ?? LEGACY_SLUG_VIEWS[slug] ?? null;
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
