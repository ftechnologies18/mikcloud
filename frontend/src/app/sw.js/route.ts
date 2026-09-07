// N°59 — service worker VERSIONNÉ PAR DÉPLOIEMENT (route handler).
//
// Problème (audit PWA, P0) : l'ancien public/sw.js statique ne changeait
// JAMAIS entre deux déploiements → le navigateur ne détectait aucun
// byte-diff → l'activate ne re-déclenchait jamais → le Cache Storage
// grossissait sans borne (chaque build produit de nouveaux bundles
// /_next/static/<hash> mis en cache, les anciens n'étant jamais purgés).
//
// Solution zéro dépendance : cette route handler (convention App Router,
// le fichier route.ts sous app/sw.js/) sert /sw.js en générant le script
// avec le numéro de version du DÉPLOIEMENT. `force-static` pré-rend la
// réponse au build : sur Vercel, VERCEL_GIT_COMMIT_SHA est injecté au
// build → chaque déploiement produit un sw.js différent → byte-diff →
// réinstallation → activate() ci-dessous purge les caches des versions
// précédentes. La croissance du Cache Storage est bornée par déploiement.
//
// Les stratégies restent EXACTEMENT celles du N°8 (conservatrices) :
// - AUCUNE mise en cache des appels /api (données de caisse fraîches,
//   les écritures ne passent jamais par le cache) ;
// - navigation : network-first avec repli offline.html ;
// - statiques immuables (/_next/static, images) : cache-first.
// La SEULE différence est le nom du cache, versionné.

export const dynamic = "force-static";

// SHA du commit (Vercel, injecté au build) ; BUILD_ID en local ; "dev"
// en dernier recours. 7 caractères suffisent (lisibilité DevTools).
const RAW_VERSION: string =
  process.env.VERCEL_GIT_COMMIT_SHA ?? process.env.BUILD_ID ?? "dev";
const VERSION = RAW_VERSION.slice(0, 7);

const SW = `/* MikCloud — service worker minimal et sûr (N°8, N°59).
 *
 * Généré par src/app/sw.js/route.ts — NE PAS ÉDITER le résultat servi,
 * éditer la route. Version du déploiement : ${VERSION}
 *
 * Politique volontairement CONSERVATRICE :
 * - AUCUNE mise en cache des appels /api (données de caisse toujours
 *   fraîches ; les écritures ne passent JAMAIS par le cache) ;
 * - navigation : network-first avec repli offline.html (tournée = réseau
 *   intermittent, l'app doit s'ouvrir et expliquer l'état hors ligne) ;
 * - assets statiques (/_next/static, images) : cache-first (immuables).
 * - N°59 : cache versionné par déploiement — l'activate purge les caches
 *   des déploiements précédents (Cache Storage borné).
 */
const CACHE = "mikcloud-${VERSION}";
const OFFLINE_URL = "/offline.html";
const PRECACHE = [OFFLINE_URL, "/logo.png", "/icon.png"];

self.addEventListener("install", (event) => {
  event.waitUntil(
    caches
      .open(CACHE)
      .then((cache) => cache.addAll(PRECACHE))
      .then(() => self.skipWaiting()),
  );
});

self.addEventListener("activate", (event) => {
  event.waitUntil(
    caches
      .keys()
      .then((keys) => Promise.all(keys.filter((k) => k !== CACHE).map((k) => caches.delete(k))))
      .then(() => self.clients.claim()),
  );
});

self.addEventListener("fetch", (event) => {
  const url = new URL(event.request.url);
  if (event.request.method !== "GET" || url.pathname.startsWith("/api")) return; // jamais l'API

  // Navigation document : network-first → repli hors ligne.
  if (event.request.mode === "navigate") {
    event.respondWith(
      fetch(event.request).catch(() =>
        caches.match(OFFLINE_URL).then((r) => r || Response.error()),
      ),
    );
    return;
  }

  // Statiques immuables du build + images : cache-first.
  if (url.pathname.startsWith("/_next/static") || /\\.(png|svg|ico|woff2?)$/.test(url.pathname)) {
    event.respondWith(
      caches.match(event.request).then(
        (hit) =>
          hit ||
          fetch(event.request).then((res) => {
            if (res.ok) {
              const clone = res.clone();
              caches.open(CACHE).then((c) => c.put(event.request, clone));
            }
            return res;
          }),
      ),
    );
  }
});
`;

export function GET() {
  return new Response(SW, {
    headers: {
      "Content-Type": "application/javascript; charset=utf-8",
      // Jamais de cache long sur le script SW lui-même : le navigateur
      // doit voir le byte-diff dès son prochain check (Chrome vérifie au
      // moins 1×/24 h ; le N°59 ajoute registration.update() au focus).
      "Cache-Control": "public, max-age=0, must-revalidate",
    },
  });
}
