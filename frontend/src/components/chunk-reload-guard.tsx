"use client";

// N°141 — recharge UNIQUE sur échec de chunk dynamique.
//
// Scénario : un déploiement a lieu pendant qu'une session est ouverte. Le
// nouveau service worker s'active (skipWaiting) et purge les caches des
// versions précédentes ; la page TOURNANTE garde en mémoire le HTML de
// l'ancien build. Au premier clic sur une vue paresseuse, l'import dynamique
// demande /_next/static/<ancien-hash>.js → 404 chez Vercel → l'import
// rejette → la vue meurt (et sans frontière d'erreur : page blanche).
//
// Remède standard : recharger la page UNE fois — la recharge re-sert le HTML
// frais du nouveau build. Deux écouteurs :
// - « error » en CAPTURE sur window : les échecs de chargement <script>/<link>
//   ne bouillonnent pas, ils remontent seulement en capture ; on ne retient
//   que les src/href /_next/ (chunk du build) ;
// - « unhandledrejection » : les messages d'échec d'import dynamique des
//   trois moteurs (Chrome/Firefox/Safari).
//
// Garde-fou anti-boucle : une empreinte sessionStorage — si le chunk échoue
// TOUJOURS après la recharge (réseau mort, build cassé), on ne recharge plus :
// la frontière d'erreur racine (src/app/error.tsx) prend le relais avec un
// écran honnête et deux sorties.

import { useEffect } from "react";

const RELOAD_KEY = "mikcloud-chunk-reloaded";
const CHUNK_ERROR = /dynamically imported module|importing a script module failed|loading chunk|chunkloaderror|failed to fetch module/i;

export function ChunkReloadGuard() {
  useEffect(() => {
    const reloadOnce = () => {
      try {
        if (sessionStorage.getItem(RELOAD_KEY)) return;
        sessionStorage.setItem(RELOAD_KEY, "1");
        window.location.reload();
      } catch {
        /* sessionStorage indisponible (navigation privée stricte) : ne pas
           boucler, ne jamais casser — la frontière d'erreur reste le filet. */
      }
    };

    const onScriptError = (event: ErrorEvent) => {
      const target = event.target as (EventTarget & { src?: unknown; href?: unknown }) | null;
      if (!target) return;
      const src =
        typeof target.src === "string" ? target.src : typeof target.href === "string" ? target.href : "";
      if (src.includes("/_next/")) reloadOnce();
    };

    const onRejection = (event: PromiseRejectionEvent) => {
      const reason: unknown = event && event.reason;
      const message =
        reason instanceof Error ? `${reason.name}: ${reason.message}` : String(reason ?? "");
      if (CHUNK_ERROR.test(message)) reloadOnce();
    };

    window.addEventListener("error", onScriptError, true);
    window.addEventListener("unhandledrejection", onRejection);
    return () => {
      window.removeEventListener("error", onScriptError, true);
      window.removeEventListener("unhandledrejection", onRejection);
    };
  }, []);
  return null;
}
