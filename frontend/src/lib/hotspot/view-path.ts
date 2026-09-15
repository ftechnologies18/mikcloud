// Couche URL de la navigation (Phase A « Speed App UX ») — mapping bijectif
// entre les vues de la console (ViewId) et les segments d'URL /app/<vue>.
//
// La vue reste pilotée par le store (source de vérité unique) : ce module
// fournit uniquement la traduction ViewId ↔ chemin, utilisée par la
// synchronisation bidirectionnelle d'app-route.tsx. Le bouton Retour du
// navigateur et les liens directs (/app/users…) deviennent fonctionnels sans
// changer ni le store ni les vues.
//
// N°57 — zone Paramètres : les vues de configuration (modèles, portail,
// notifications, équipe) vivent sous /app/settings/<section>
// (segment IMBRIQUÉ). Les chemins historiques (/app/templates, /app/team…)
// restent deep-linkables : la vue est résolue via LEGACY_SLUG_VIEWS puis
// l'URL est re-normalisée (replace, zéro entrée d'historique parasite) par
// app-route — même mécanique que la fusion N°30 « registrations → hub ».
//
// N°57-d — réorganisation des sections : Général / Hotspot / Sécurité /
// Notifications / Équipe. Le slug de « settings » devient
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
//
// N°112 — « Routeurs » RETOURNE en navigation principale : son slug
// redevient le simple /app/routers (pré-N°57). L'ancien chemin canonique
// N°57 (/app/settings/routers) reste deep-linkable via LEGACY_SLUG_VIEWS
// puis re-normalisé — signets et historiques navigateur conservés. La
// fiche routeur reste adressable : /app/routers/<id> (DETAIL_VIEWS).

import type { ViewId } from "./types";

/** Préfixe de toutes les routes console. */
export const APP_BASE_PATH = "/app";

/** Segment d'URL de chaque vue — kebab-case dérivé de l'identifiant.
 * N°57 : les vues de la zone Paramètres portent un segment IMBRIQUÉ
 * « settings/<section> ». N°57-d : « settings » = Général (settings/general),
 * « hotspot » = hub à 3 segments (portail/modeles = onglets deep-linkables).
 * N°112 : « routers » retrouve son segment SIMPLE d'avant la zone — la vue
 * vit en navigation principale (/app/routers). */
const VIEW_SLUGS: Record<ViewId, string> = {
  dashboard: "dashboard",
  sessions: "sessions",
  // N°83 — vue Protection : l'état de sécurité du WiFi (verdict + 3 cartes
  // SafeWiFi/Shield/FamilyGuard) vit dans la navigation principale — un
  // argument de vente ne s'enterre pas dans la zone Paramètres.
  protection: "protection",
  // N°100 — console HomeNet : le tableau de bord maison (/app/home) et les
  // appareils connectés (/app/devices). Segments simples, zone Paramètres
  // partagée (settings/…) — la traduction ViewId ↔ chemin reste bijective.
  home: "home",
  devices: "devices",
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
  // N°112 — retour au segment simple (vue de navigation principale,
  // section Infrastructure) — cf. en-tête du fichier.
  routers: "routers",
  portal: "settings/hotspot/portail",
  reports: "reports",
  logs: "logs",
  platform: "platform",
  // N°117 — parc routeurs global (flotte tous comptes, mise à jour RouterOS).
  platformFleet: "platform-fleet",
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

/** Slugs historiques (pré-N°57, pré-N°57-d et pré-N°112) des vues qui ont
 * changé de chemin : /app/<slug> résout toujours vers sa vue (signets,
 * historique navigateur, liens sortants) — app-route replace ensuite vers
 * le chemin canonique /app/settings/<…> ou /app/<…>. Paires
 * « settings/<x> » : anciens chemins canoniques N°57-b/c (portal,
 * templates) + racine de zone « settings » (N°57-d → Général) + ancien
 * chemin N°57 des routeurs (N°112 → /app/routers). Une section de zone
 * inconnue (/app/settings/xyz) retombe sur la racine de zone par la
 * résolution simple. */
const LEGACY_SLUG_VIEWS: Record<string, ViewId> = {
  templates: "templates",
  portal: "portal",
  notifications: "notifications",
  team: "team",
  settings: "settings",
  "settings/portal": "portal",
  "settings/templates": "templates",
  // N°57-e — ancien chemin racine de la vue Abonnement (pré-zone).
  subscription: "subscription",
  // N°112 — ancien chemin canonique N°57 de la vue Routeurs (pré-retour
  // en navigation principale) : /app/settings/routers → /app/routers.
  "settings/routers": "routers",
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
 * éventuel (/app/routers/<id>, /app/users/<name>…) est ignoré ici : il
 * appartient à la vue (detailFromPath), la synchro de vue d'app-route reste
 * inchangée. */
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
  // canonique N°57-d, legacy N°57-b/c ou ancien chemin routeurs N°112).
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
 * détail, /app/routers/<id> — plus de modale d'inspection).
 * N°83 : « protection » adresse le routeur sélectionné (le CTA du résumé
 * de l'onglet Système ouvre CE routeur, /app/protection/<id>). */
const DETAIL_VIEWS: ReadonlySet<ViewId> = new Set<ViewId>(["users", "vouchers", "sessions", "routers", "protection"]);

/** Identifiant de détail d'un chemin /app/<slug>/<détail> — null si absent
 * ou si la vue n'expose pas de détail. Ne lit QUE le segment suivant le
 * préfixe canonique de la vue (3e/4e segment en zone) : les vues branchent
 * leur état local dessus (ouverture de fiche, filtre lot…) sans toucher au
 * mécanisme de navigation (fix 192ad9f préservé). Un segment suivant
 * éventuel est ignoré (le détail ne s'imbrique pas). N°112 — note : les
 * liens profonds de l'ère N°57 AVEC détail (/app/settings/routers/<id>)
 * résolvent la vue mais perdent le segment de fiche (préfixe canonique
 * changé) : ils atterrissent sur la liste — même traitement que les autres
 * chemins historiques, jamais une page morte. */
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
