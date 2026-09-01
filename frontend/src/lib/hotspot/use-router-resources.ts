"use client";

// Parité Mikhmon — ressources routeur pour les formulaires.
//
// Mikhmon alimente « Address Pool », « Parent Queue » (formulaire User
// Profile) et « Server » (formulaire Generate) directement depuis le routeur
// (/ip pool print, /queue simple print, /ip hotspot print). MikCloud fait de
// même via la commande agent read_resources + GET /api/routers/{id}/resources :
// - sans routerId : tous les routeurs NON réels (mode agent/simulé) fusionnés
//   (les profils MikCloud sont multi-routeurs → listes dédupliquées) ;
// - avec routerId : ce routeur uniquement (sélecteur « Charger depuis un
//   routeur » du dialog profil, sélecteur Server du générateur de vouchers).
// Les routeurs en mode réel sont exclus (pas d'API entrante, matrice §0 du
// contrat) ; un routeur agent répond après son check-in (≤ 45 s) — tant que la
// commande read_resources est en file (`queued:true`), les listes sont vides et
// le re-poll est accéléré (5 s) ; consommateur : bannière « en attente ».

import { useQuery } from "@tanstack/react-query";

import { api } from "@/lib/hotspot/api";
import type { RouterDevice, RouterResource } from "@/lib/hotspot/types";

export interface RouterResources {
  pools: string[];
  queues: string[];
  servers: string[];
  /** true tant qu'un routeur agent n'a pas encore rapporté (commande en file). */
  queued: boolean;
  /** Horodatage de la donnée la plus récente ("" si aucune). */
  updatedAt: string;
}

const EMPTY: RouterResources = {
  pools: [],
  queues: [],
  servers: [],
  queued: false,
  updatedAt: "",
};

// Enveloppe F9 {queued, data, updatedAt} — l'endpoint renvoie TOUJOURS cette
// forme (simulated : queued:false immédiat ; agent : queued jusqu'au check-in).
// Tolérance de forme : un tableau nu est accepté aussi (défense, même logique
// que fetchToolEnvelope dans router-tools).
type Envelope = { queued?: boolean; data?: RouterResource[]; updatedAt?: string };

function unwrapResources(raw: Envelope | RouterResource[] | null): RouterResource[] {
  if (Array.isArray(raw)) return raw;
  if (raw && typeof raw === "object") return raw.data ?? [];
  return [];
}

export function useRouterResources(routers: RouterDevice[] | undefined, routerId?: string) {
  const targets = (routers ?? []).filter((r) =>
    routerId ? r.id === routerId : r.mode !== "real",
  );
  const ids = targets.map((r) => r.id).join(",");
  return useQuery({
    queryKey: ["/api/router-resources", routerId ?? "all", ids],
    queryFn: async (): Promise<RouterResources> => {
      if (targets.length === 0) return EMPTY;
      const results = await Promise.all(
        targets.map((r) =>
          api<Envelope | RouterResource[]>(`/api/routers/${r.id}/resources`).catch(() => null),
        ),
      );
      const pools = new Set<string>();
      const queues = new Set<string>();
      const servers = new Set<string>();
      let queued = false;
      let updatedAt = "";
      for (const raw of results) {
        if (raw === null) continue;
        if (!Array.isArray(raw)) {
          if (raw.queued) queued = true;
          if (raw.updatedAt && raw.updatedAt > updatedAt) updatedAt = raw.updatedAt;
        }
        for (const res of unwrapResources(raw)) {
          if (res.kind === "pool") pools.add(res.name);
          else if (res.kind === "queue") queues.add(res.name);
          else if (res.kind === "server") servers.add(res.name);
        }
      }
      const sorted = (s: Set<string>) => [...s].sort((a, b) => a.localeCompare(b));
      return {
        pools: sorted(pools),
        queues: sorted(queues),
        servers: sorted(servers),
        queued,
        updatedAt,
      };
    },
    enabled: targets.length > 0,
    // Re-poll accéléré tant qu'un check-in agent est attendu, sinon entretien.
    refetchInterval: (query) => (query.state.data?.queued ? 5_000 : 15_000),
    staleTime: 30_000,
  });
}
