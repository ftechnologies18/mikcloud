/* MikCloud — service worker minimal et sûr (N°8, PWA Mode Vente).
 *
 * Politique volontairement CONSERVATRICE :
 * - AUCUNE mise en cache des appels /api (données de caisse toujours fraîches ;
 *   les écritures ne passent JAMAIS par le cache) ;
 * - navigation : network-first avec repli offline.html (tournée = réseau
 *   intermittent, l'app doit s'ouvrir et expliquer l'état hors ligne) ;
 * - assets statiques (/_next/static, images) : cache-first (immuables).
 */
const CACHE = "mikcloud-v1";
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
  if (url.pathname.startsWith("/_next/static") || /\.(png|svg|ico|woff2?)$/.test(url.pathname)) {
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
