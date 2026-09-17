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
// Les stratégies restent conservatrices :
// - AUCUNE mise en cache des appels /api (données de caisse fraîches,
//   les écritures ne passent jamais par le cache) ;
// - navigation : network-first AVEC DÉLIMITE (N°141) et repli shell
//   offline (N°61 pour /sell, N°141 pour toutes les autres) ;
// - statiques immuables (/_next/static, images) : cache-first.
//
// N°61 — Mode Vente offline AU LANCEMENT : la navigation /sell (icône
// PWA, raccourci « Mode Vente » du N°60, lien direct) est network-first
// MAIS bornée à 4 s, avec repli sur le SHELL /sell en cache — pas sur
// offline.html. En tournée, réseau captif ou hotspot non authentifié :
// le comptoir S'OUVRE (snapshots localStorage + file IndexedDB prennent
// le relais côté client).
//
// N°141 — audit PWA « pages qui restent blanches » : TOUTES les navigations
// sont désormais bornées et disposent d'un repli shell. Avant ce changement,
// seules les non-/sell étaient network-first SANS délai : derrière un portail
// captif (le cas d'usage CENTRAL de MikCloud — le téléphone du revendeur vit
// sur le hotspot qu'il vend), un fetch de navigation peut PENDRE 75 s+ sans
// rejeter : pendant toute cette attente la navigation n'est pas commise et
// l'app reste sur un écran VIDE. Chaque navigation réussie (2xx non
// redirigée) est copiée en cache : /, /login, /app/<vue>… obtiennent leur
// shell offline, servi quand le réseau tombe OU dépasse le délai. Le repli
// final reste offline.html (statique, autonome, jamais masqué par le
// mécanisme pwa-standalone — il n'embarque pas le layout). Le précachage
// devient TOLÉRANT (put individuels) : un /sell momentanément indisponible
// au moment de l'install ne doit pas empêcher offline.html d'exister —
// l'ancien cache.addAll atomique faisait échouer TOUT l'install.

export const dynamic = "force-static";

// SHA du commit (Vercel, injecté au build) ; BUILD_ID en local ; "dev"
// en dernier recours. 7 caractères suffisent (lisibilité DevTools).
const RAW_VERSION: string =
  process.env.VERCEL_GIT_COMMIT_SHA ?? process.env.BUILD_ID ?? "dev";
const VERSION = RAW_VERSION.slice(0, 7);

const SW = `/* MikCloud — service worker minimal et sûr (N°8, N°59, N°141).
 *
 * Généré par src/app/sw.js/route.ts — NE PAS ÉDITER le résultat servi,
 * éditer la route. Version du déploiement : ${VERSION}
 *
 * Politique volontairement CONSERVATRICE :
 * - AUCUNE mise en cache des appels /api (données de caisse toujours
 *   fraîches ; les écritures ne passent JAMAIS par le cache) ;
 * - navigation : network-first BORNÉE (N°141 — réseau captif : le fetch
 *   peut pendre 75 s+) → repli shell en cache → offline.html ;
 * - assets statiques (/_next/static, images) : cache-first (immuables).
 * - N°59 : cache versionné par déploiement — l'activate purge les caches
 *   des déploiements précédents (Cache Storage borné).
 * - N°61 : navigation /sell bornée à 4 s (le comptoir s'ouvre immédiatement
 *   en tournée, même pendu sur un portail captif).
 * - N°141 : TOUTES les navigations bornées (10 s) avec repli shell —
 *   chaque page visitée en ligne devient ouvrable hors ligne ; le
 *   précachage est tolérant (un échec n'emporte pas les autres).
 */
const CACHE = "mikcloud-${VERSION}";
const OFFLINE_URL = "/offline.html";
const SELL_URL = "/sell";
const NAV_TIMEOUT_MS = 10000;
const SELL_TIMEOUT_MS = 4000;
const PRECACHE = [OFFLINE_URL, "/logo.png", "/icon.png", SELL_URL, "/login"];

self.addEventListener("install", (event) => {
  event.waitUntil(
    caches
      .open(CACHE)
      .then((cache) =>
        // N°141 — précachage TOLÉRANT : chaque entrée est posée
        // indépendamment. L'ancien cache.addAll était atomique : un seul
        // 404/timeout sur /sell faisait échouer TOUT l'install → le
        // service worker ne s'activait jamais → zéro repli offline.
        Promise.all(
          PRECACHE.map((url) =>
            cache.add(url).catch(function () {
              /* entrée indisponible à cet instant : les autres vivent */
            }),
          ),
        ),
      )
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

  // Navigation document : network-first BORNÉE → repli shell → offline.
  // /sell garde son délai court dédié (N°61) ; toutes les autres
  // navigations ont 10 s (N°141) — assez généreux pour un réseau 2G
  // légitime, assez court pour ne jamais montrer un écran vide 75 s.
  if (event.request.mode === "navigate") {
    event.respondWith(
      url.pathname === SELL_URL
        ? shellNavigation(event.request, SELL_TIMEOUT_MS)
        : shellNavigation(event.request, NAV_TIMEOUT_MS),
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

// N°61 + N°141 — navigation network-first BORNÉE (course contre le délai),
// repli sur le shell de l'URL exacte en cache, puis offline.html. Trois cas :
// - réseau OK (≤ délai) : réponse réseau servie ET copie fraîche mise en
//   cache pour le prochain lancement hors ligne (uniquement réponse
//   directe 2xx non-redirigée : une redirection n'est jamais
//   sémantiquement « le shell de cette page ») ;
// - réseau tombé (fetch rejette) : repli IMMÉDIAT sur le shell caché ;
// - réseau captif (fetch pend) : repli au délai — pas d'écran vide 75 s.
// Réseau ok mais réponse d'erreur (404, 500) : servie telle quelle —
// honnête, et elle n'entre jamais au cache. Le shell hydraté prend le
// relais côté client : snapshots localStorage (profil + stock), ventes
// mises en file IndexedDB, replay 60 s.
async function shellNavigation(request, timeoutMs) {
  const networkFetch = fetch(request)
    .then((res) => {
      if (res && res.ok && !res.redirected) {
        try {
          const clone = res.clone();
          caches.open(CACHE).then((c) => c.put(request, clone));
        } catch (e) {
          /* mise à jour du repli : confort, jamais bloquant */
        }
      }
      return res;
    })
    .catch(() => null);
  const timeout = new Promise((resolve) => setTimeout(() => resolve(null), timeoutMs));
  const winner = await Promise.race([networkFetch, timeout]);
  if (winner) return winner;
  const shell = await caches.match(request);
  if (shell) return shell;
  const offline = await caches.match(OFFLINE_URL);
  return offline || Response.error();
}
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
