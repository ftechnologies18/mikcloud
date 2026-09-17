"use client";

/**
 * N°130 — P0 audit réactivité : préchargement au SURVOL de la navigation.
 *
 * Chaque vue est un chunk distinct (next/dynamic — 100 à 390 Ko brut) : le
 * PREMIER clic sur une vue payait deux allers-retours avant le premier rendu
 * utile — téléchargement + parsing du chunk, PUIS requêtes de données. Le
 * survol (ou le focus clavier) d'un item de navigation déclenche les DEUX
 * téléchargements en avance de phase :
 *
 * - le CHUNK : le chargeur dynamique est mis en cache par le bundler —
 *   next/dynamic réutilise le module déjà résolu, le clic devient instantané ;
 * - la REQUÊTE PRINCIPALE : queryClient.prefetchQuery avec EXACTEMENT la
 *   même clé que la vue (les clés instables — listes filtrées, rapports
 *   datés — ne sont volontairement PAS préchargées : un prefetch mal calé
 *   doublerait le fetch au clic). Combiné au 304 (api.ts), un survol
 *   typique coûte ~200 o d'en-têtes par requête.
 *
 * Idempotent par construction : import() et prefetchQuery ne font rien si
 * la ressource est déjà là. Le tactile n'a pas de survol — le comportement
 * au clic est strictement inchangé.
 */

import type { QueryClient } from "@tanstack/react-query";
import { api } from "./api";
import type { ViewId } from "./types";

/**
 * Chargeurs de chunks — MIROIR des dynamic() de app-shell.tsx : appeler le
 * chargeur télécharge le module ; next/dynamic le réutilise au rendu.
 * (users → users-hub-view et hotspot → hotspot-view : hubs à onglets, même
 * miroir que la navigation.)
 */
const CHUNK_LOADERS: Partial<Record<ViewId, () => Promise<unknown>>> = {
  accounts: () => import("@/components/hotspot/views/accounts-view"),
  billingRequests: () => import("@/components/hotspot/views/billing-requests-view"),
  dashboard: () => import("@/components/hotspot/views/dashboard-view"),
  devices: () => import("@/components/hotspot/views/devices-view"),
  home: () => import("@/components/hotspot/views/home-view"),
  hotspot: () => import("@/components/hotspot/views/hotspot-view"),
  logs: () => import("@/components/hotspot/views/logs-view"),
  notifications: () => import("@/components/hotspot/views/notifications-view"),
  platform: () => import("@/components/hotspot/views/platform-overview-view"),
  platformFleet: () => import("@/components/hotspot/views/platform-fleet-view"),
  platformChat: () => import("@/components/hotspot/views/platform-chat-view"),
  platformLogs: () => import("@/components/hotspot/views/platform-logs-view"),
  platformSettings: () => import("@/components/hotspot/views/platform-settings-view"),
  platformTeam: () => import("@/components/hotspot/views/platform-team-view"),
  profiles: () => import("@/components/hotspot/views/profiles-view"),
  protection: () => import("@/components/hotspot/views/protection-view"),
  reports: () => import("@/components/hotspot/views/reports-view"),
  resellers: () => import("@/components/hotspot/views/resellers-view"),
  routers: () => import("@/components/hotspot/views/routers-view"),
  security: () => import("@/components/hotspot/views/security-view"),
  sessions: () => import("@/components/hotspot/views/sessions-view"),
  settings: () => import("@/components/hotspot/views/settings-view"),
  subscription: () => import("@/components/hotspot/views/subscription-view"),
  team: () => import("@/components/hotspot/views/team-view"),
  users: () => import("@/components/hotspot/views/users-hub-view"),
  wifi: () => import("@/components/hotspot/views/wifi-view"),
  vouchers: () => import("@/components/hotspot/views/vouchers-view"),
};

/** Une requête à précharger : clé React Query EXACTE + chemin api(). */
interface ViewQuery {
  key: readonly string[];
  path: string;
}

/**
 * Requêtes principales par vue — UNIQUEMENT les clés STABLES (sans
 * paramètres), strictement identiques à celles des vues (cf. useQuery dans
 * chaque vue) : prefetch mal calé = double fetch au clic, on ne prend aucun
 * risque. Les listes filtrées (utilisateurs, vouchers), rapports datés et
 * autres clés paramétrées restent à la charge du clic.
 */
const VIEW_QUERIES: Partial<Record<ViewId, readonly ViewQuery[]>> = {
  dashboard: [
    { key: ["/api/dashboard"], path: "/api/dashboard" },
    { key: ["/api/settings"], path: "/api/settings" },
  ],
  sessions: [{ key: ["/api/sessions"], path: "/api/sessions" }],
  users: [
    { key: ["/api/profiles"], path: "/api/profiles" },
    { key: ["/api/routers"], path: "/api/routers" },
  ],
  vouchers: [
    { key: ["/api/profiles"], path: "/api/profiles" },
    { key: ["/api/routers"], path: "/api/routers" },
    { key: ["/api/resellers"], path: "/api/resellers" },
    { key: ["/api/templates"], path: "/api/templates" },
    { key: ["/api/vouchers", "stats"], path: "/api/vouchers/stats" },
  ],
  profiles: [{ key: ["/api/profiles"], path: "/api/profiles" }],
  resellers: [
    { key: ["/api/resellers"], path: "/api/resellers" },
    { key: ["/api/transactions"], path: "/api/transactions" },
  ],
  routers: [{ key: ["/api/routers"], path: "/api/routers" }],
  devices: [
    { key: ["/api/routers"], path: "/api/routers" },
    { key: ["/api/devices"], path: "/api/devices" },
  ],
  home: [
    { key: ["/api/routers"], path: "/api/routers" },
    { key: ["/api/devices"], path: "/api/devices" },
  ],
  protection: [{ key: ["/api/routers"], path: "/api/routers" }],
  reports: [{ key: ["/api/routers"], path: "/api/routers" }],
  wifi: [
    { key: ["/api/wifi/sites"], path: "/api/wifi/sites" },
    { key: ["/api/routers"], path: "/api/routers" },
    { key: ["/api/profiles"], path: "/api/profiles" },
  ],
};

/**
 * prefetchView — précharge le chunk et les requêtes principales d'une vue.
 * Appelé au survol/focus d'un item de navigation (desktop) ; sans effet au
 * clic tactile (pas de survol) ni si tout est déjà en cache.
 */
export function prefetchView(view: ViewId, queryClient?: QueryClient): void {
  const load = CHUNK_LOADERS[view];
  if (load) {
    void load().catch(() => {
      /* prefetch best-effort : une erreur ici n'a aucune conséquence,
         le clic refera le trajet normal */
    });
  }
  if (!queryClient) return;
  for (const q of VIEW_QUERIES[view] ?? []) {
    void queryClient.prefetchQuery({
      queryKey: [...q.key],
      queryFn: () => api(q.path),
    });
  }
}
