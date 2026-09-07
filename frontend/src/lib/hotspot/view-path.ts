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
//
// N°57-d — réorganisation des sections : Général / Hotspot / Sécurité /
// Routeurs / Notifications / Équipe. Le slug de « settings » devient
// settings/general ; la section Hotspot est un HUB : l'expérience vit sur
// settings/hotspot, le portail et les modèles sont ses ONGLETS adressables
// (settings/hotspot/portail, settings/hotspot/modeles — pattern N°30).
// L'ancienne racine /app/settings (et les anciens chemins canoniques
// /app/settings/portal, /app/settings/templates) restent deep-linkables
// via LEGACY_SLUG_VIEWS puis re-normalisés.
//
// N°57-e — l’Abonnement (vue dédiée M) rejoint la zone : /app/settings/
// subscription. L’ancien chemin racine /app/subscription reste deep-linkable
// (LEGACY_SLUG_VIEWS) puis re-normalisé — signets et historiques navigateur
// conservés.

import type { ViewId } from "./types";

/** Préfixe de toutes les routes console. */
export const APP_BASE_PATH = "/app";

/** Segment d'URL de chaque vue — kebab-case dérivé de l'identifiant.
 * N°57 : les vues de la zone Paramètres portent un segment IMBRIQUÉ
 * « settings/<section> ». N°57-d : « settings » = Général (settings/general),
 * « hotspot » = hub à 3 segments (portail/modeles = onglets deep-linkables). */
const VIEW_SLUGS: Record<ViewId, string> = {
  dashboard: "dashboard",
  sessions: "sessions",
  // N°57-e — section Abonnement de la zone Paramètres (facturation de
  // l’espace : formule, échéance, renouvellement, factures).
  subscription: "settings/subscription",
  users: "users",
  registrations: "registrations",
  vouchers: "vouchers",
  templates: "settings/hotspot/modeles",
  profiles: "profiles",
  resellers: "resellers",
  wifi: "wifi",
  routers: "settings/routers",
  portal: "settings/hotspot/portail",
  reports: "reports",
  logs: "logs",
  platform: "platform",
  platformLogs: "platform-logs",
  platformTeam: "platform-team",
  platformSettings: "platform-settings",
  billingRequests: "billing-requests",
  accounts: "accounts",
  notifications: "settings/notifications",
  settings: "settings/general",
  hotspot: "settings/hotspot",
  security: "settings/security",
  team: "settings/team",
};

const SLUG_VIEWS: Record<string, ViewId> = Object.fromEntries(
  Object.entries(VIEW_SLUGS).map(([view, slug]) => [slug, view as ViewId]),
);

/** Slugs historiques (pré-N°57 et pré-N°57-d) des vues de la zone :
 * /app/<slug> résout toujours vers sa vue (signets, historique navigateur,
 * liens sortants) — app-route replace ensuite vers le chemin canonique
 * /app/settings/<…>. Paires « settings/<x> » : anciens chemins canoniques
 * N°57-b/c (portal, templates) + racine de zone « settings » (N°57-d →
 * Général). Une section de zone inconnue (/app/settings/xyz) retombe sur
 * la racine de zone par la résolution simple. */
const LEGACY_SLUG_VIEWS: Record<string, ViewId> = {
  templates: "templates",
  routers: "routers",
  portal: "portal",
  notifications: "notifications",
  team: "team",
  settings: "settings",
  "settings/portal": "portal",
  "settings/templates": "templates",
  // N°57-e — ancien chemin racine de la vue Abonnement (pré-zone).
  subscription: "subscription",
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
 * N°57-d : résolution du segment le PLUS LONG d'abord — le hub Hotspot
 * adresse ses onglets sur 3 segments (settings/hotspot/portail) ; puis
 * la paire (settings/<section>, canonique OU legacy) ; puis le slug simple
 * (canonique ou legacy, re-normalisé ensuite). Le segment de détail
 * éventuel (/app/settings/routers/<id>) est ignoré ici : il appartient à
 * la vue (detailFromPath), la synchro de vue d'app-route reste inchangée. */
export function viewFromPath(pathname: string | null): ViewId | null {
  if (!pathname) return null;
  if (pathname === APP_BASE_PATH) return null;
  const prefix = `${APP_BASE_PATH}/`;
  if (!pathname.startsWith(prefix)) return null;
  const segs = pathname.slice(prefix.length).split("/");
  // Hub Hotspot (N°57-d) — 3 segments : settings/hotspot/<onglet>.
  if (segs.length >= 3) {
    const triplet = SLUG_VIEWS[`${segs[0]}/${segs[1]}/${segs[2]}`];
    if (triplet) return triplet;
  }
  // Segment imbriqué — section de la zone Paramètres (settings/<section>,
  // canonique N°57-d ou legacy N°57-b/c).
  if (segs.length >= 2) {
    const pair = `${segs[0]}/${segs[1]}`;
    const nested = SLUG_VIEWS[pair] ?? LEGACY_SLUG_VIEWS[pair];
    if (nested) return nested;
  }
  const slug = segs[0];
  return SLUG_VIEWS[slug] ?? LEGACY_SLUG_VIEWS[slug] ?? null;
}

/** Vues exposant un détail adressable (Phase D, N°57-d) — les autres n'ont
 * pas de 2e segment valide : app-route re-normalise ces chemins orphelins.
 * N°57-d : « routers » expose la FICHE routeur (carte cliquable → page de
 * détail, /app/settings/routers/<id> — plus de modale d'inspection). */
const DETAIL_VIEWS: ReadonlySet<ViewId> = new Set<ViewId>(["users", "vouchers", "sessions", "routers"]);

/** Identifiant de détail d'un chemin /app/<slug>/<détail> — null si absent
 * ou si la vue n'expose pas de détail. Ne lit QUE le segment suivant le
 * préfixe canonique de la vue (3e/4e segment en zone) : les vues branchent
 * leur état local dessus (ouverture de fiche, filtre lot…) sans toucher au
 * mécanisme de navigation (fix 192ad9f préservé). Un segment suivant
 * éventuel est ignoré (le détail ne s'imbrique pas). */
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
