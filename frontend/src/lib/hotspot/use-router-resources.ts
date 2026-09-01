"use client";

// Parité Mikhmon — ressources routeur pour les formulaires.
//
// Mikhmon alimente « Address Pool » et « Parent Queue » (formulaire User
// Profile) ainsi que « Server » (formulaire Generate) directement depuis le
// routeur (/ip pool print, /queue simple print, /ip hotspot print). MikCloud
// fait de même via la commande agent read_resources + GET
// /api/routers/{id}/resources — ce hook agrège les réponses :
// - sans routerId : tous les routeurs NON réels (mode agent/simulé) fusionnés
//   (les profils MikCloud sont multi-routeurs → listes dédupliquées) ;
// - avec routerId : ce routeur uniquement (sélecteur « Serveur hotspot » du
//   générateur de vouchers).
// Les routeurs en mode réel sont exclus (pas d'API entrante) ; un routeur
// agent répond après son check-in (queued=true → re-poll automatique).

import { useQuery } from "@tanstack/react-query";

import { api } from "@/lib/hotspot/api";
import type { RouterDevice, RouterResource } from "@/lib/hotspot/types";

export interface RouterResources {
  pools: string[];
  queues: string[];
  servers: string[];
}

const EMPTY: RouterResources = { pools: [], queues: [], servers: [] };

export function useRouterResources(routers: RouterDevice[] | undefined, routerId?: string) {
  const targets = (routers ?? []).filter((r) =>
    routerId ? r.id === routerId : r.mode !== "real",
  );
  const ids = targets.map((r) => r.id).join(",");
  return useQuery({
    queryKey: ["/api/router-resources", routerId ?? "all", ids],
    queryFn: async () => {
      const results = await Promise.all(
        targets.map((r) =>
          api<RouterResource[]>(`/api/routers/${r.id}/resources`).catch(() => [] as RouterResource[]),
        ),
      );
      const pools = new Set<string>();
      const queues = new Set<string>();
      const servers = new Set<string>();
      for (const res of results.flat()) {
        if (res.kind === "pool") pools.add(res.name);
        else if (res.kind === "queue") queues.add(res.name);
        else if (res.kind === "server") servers.add(res.name);
      }
      const sorted = (s: Set<string>) => [...s].sort((a, b) => a.localeCompare(b));
      return { pools: sorted(pools), queues: sorted(queues), servers: sorted(servers) } as RouterResources;
    },
    enabled: targets.length > 0,
    refetchInterval: 15_000,
    staleTime: 30_000,
  });
}
