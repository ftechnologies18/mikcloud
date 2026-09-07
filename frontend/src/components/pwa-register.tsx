"use client";

// N°8 — enregistrement du service worker PWA (production uniquement :
// en dev le SW cacherait des assets en cours d'édition).
//
// N°59 — robustesse :
// - storage.persist() demandé dès le register (le helper est idempotent ;
//   il est aussi relancé par queueSale au moment d'enfiler une vente) :
//   la file IndexedDB des ventes hors-ligne contient de l'argent réel,
//   elle ne doit pas être évictionnée sous pression disque.
// - registration.update() au retour de visibilité et au retour du
//   réseau : Chrome ne vérifie le SW qu'au maximum 1×/24 h de son propre
//   chef ; au comptoir, un déploiement doit être visible au premier
//   rallumage de l'écran, pas le lendemain. Throttle 30 min : un update()
//   n'est qu'un GET conditionnel tant que le sw.js n'a pas changé — mais
//   inutile de le spawner à chaque micro-focus.
import { useEffect } from "react";
import { ensureStoragePersisted } from "@/lib/hotspot/offline-queue";

const UPDATE_MIN_INTERVAL_MS = 30 * 60 * 1000;

export function PWARegister() {
  useEffect(() => {
    if (process.env.NODE_ENV !== "production") return;
    if (typeof navigator === "undefined" || !("serviceWorker" in navigator)) return;

    // N°59 — persistance du stockage dès le chargement (silencieux, voir
    // offline-queue.ts pour la stratégie complète).
    ensureStoragePersisted();

    let registration: ServiceWorkerRegistration | null = null;
    const register = () => {
      navigator.serviceWorker
        .register("/sw.js")
        .then((reg) => {
          registration = reg;
        })
        .catch(() => {
          /* PWA optionnelle : échec silencieux */
        });
    };
    if (document.readyState === "complete") register();
    else window.addEventListener("load", register, { once: true });

    // N°59 — fraîcheur du SW au-delà du check natif (1×/24 h max) :
    let lastCheck = 0;
    const checkForUpdate = () => {
      const now = Date.now();
      if (now - lastCheck < UPDATE_MIN_INTERVAL_MS) return;
      lastCheck = now;
      registration?.update().catch(() => {
        /* hors ligne / SW retiré : silencieux */
      });
    };
    const onVisibility = () => {
      if (document.visibilityState === "visible") checkForUpdate();
    };
    document.addEventListener("visibilitychange", onVisibility);
    window.addEventListener("online", checkForUpdate);

    return () => {
      window.removeEventListener("load", register);
      document.removeEventListener("visibilitychange", onVisibility);
      window.removeEventListener("online", checkForUpdate);
    };
  }, []);
  return null;
}
