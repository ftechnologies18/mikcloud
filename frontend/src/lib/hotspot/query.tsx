"use client";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useState } from "react";

/**
 * N°130 — P0 audit réactivité : fraîcheur GRADUÉE par nature de donnée.
 *
 * L'ancien défaut unique (staleTime 10 s) traitait pareil un compteur de
 * sessions (qui bouge à chaque read_state) et une liste de profils (qui ne
 * change qu'à l'écriture) : chaque remontée de vue re-demandait TOUT au
 * backend pour des données de référence inchangées. Les valeurs :
 *
 * - live        : données vivantes (sessions, dashboard) — le polling
 *                 explicite (refetchInterval) reste maître du rythme, la
 *                 fraîcheur ne sert qu'aux remontées de vue ;
 * - operational : état du parc (routeurs, appareils, activité) — bouge aux
 *                 check-ins agents (~45 s) et actions gérant ;
 * - reference   : données de référence (profils, modèles de voucher,
 *                 revendeurs) — changent uniquement à l'écriture ;
 * - frozen      : catalogue de formules, réglages portail publics — jamais
 *                 de re-fetch au montage (invalidation explicite seulement).
 */
export const STALE_TIME = {
  live: 10_000,
  operational: 30_000,
  reference: 5 * 60_000,
  frozen: Infinity,
} as const;

export function QueryProvider({ children }: { children: React.ReactNode }) {
  const [client] = useState(
    () =>
      new QueryClient({
        defaultOptions: {
          queries: {
            // Défaut = données vivantes (le cas le plus fréquent) ; les
            // requêtes de référence surchargent localement via STALE_TIME.
            staleTime: STALE_TIME.live,
            retry: 1,
            refetchOnWindowFocus: false,
          },
        },
      }),
  );
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}
