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
//
// N°193 — la mise à jour ENFIN VISIBLE à la réouverture (la lacune qui
// faisait vivre les N°191/192 « invisibles » sur les PWA installées) :
// skipWaiting + clients.claim (déjà dans le SW, N°59) rendent le NOUVEAU
// service worker actif en arrière-plan… mais la page EN COURS continue
// d'exécuter les ANCIENS bundles. Sur Android, rouvrir la PWA ne
// déclenche AUCUNE navigation (launch_handler « focus-existing », N°60) :
// sans reload, l'ancienne interface peut vivre des JOURS malgré des
// déploiements quotidiens. Trois pièces complémentaires :
//   1. controllerchange → la page se recharge UNE fois par vie de
//      document (garde module) — SAUF si le Mode Vente est ouvert : le
//      geste du comptoir n'est jamais interrompu (les ventes en file
//      IndexedDB seraient de toute façon rejouées, mais pas au milieu
//      d'une remise au client) ; le reload est alors REPORTÉ jusqu'à la
//      sortie du comptoir (watcher léger sur le pathname).
//   2. Première installation ignorée : si la page n'était contrôlée par
//      AUCUN SW avant le register, le controllerchange qui suit
//      clients.claim() est l'installation initiale — rien à recharger
//      (la page qui vient de charger EST la version courante).
//   3. Le throttle de update() est RÉARMÉ à chaque passage en
//      arrière-plan : chaque session d'usage (réouverture de la PWA)
//      rechecke le sw.js, les micro-focus au sein d'une session restent
//      throttlés. Avant : une PWA rouverte moins de 30 min après sa
//      dernière consultation ne recheckait RIEN.
// Ceinture et bretelles : updateViaCache "none" — le sw.js ne passe
// JAMAIS par le cache HTTP pour son byte-check (la route le sert déjà en
// max-age=0, must-revalidate ; le navigateur n'a plus d'excuse).
import { useEffect } from "react";
import { ensureStoragePersisted } from "@/lib/hotspot/offline-queue";

const UPDATE_MIN_INTERVAL_MS = 30 * 60 * 1000;
// N°193 — sortie du report Mode Vente : vérification du pathname.
const DEFERRED_WATCH_MS = 10 * 1000;

// Garde anti-boucle : controllerchange ne recharge la page qu'UNE fois
// par vie de document (après reload, c'est un NOUVEAU document — un
// déploiement ultérieur rechargera de nouveau, c'est le comportement
// voulu ; c'est la boucle AU SEIN d'une même vie qui est bloquée).
let reloadedForUpdate = false;
// Mise à jour prête mais reportée (Mode Vente ouvert à l'instant du
// controllerchange) : le reload attend la sortie du comptoir.
let deferredUpdateReload = false;

export function PWARegister() {
  useEffect(() => {
    if (process.env.NODE_ENV !== "production") return;
    if (typeof navigator === "undefined" || !("serviceWorker" in navigator)) return;

    // N°59 — persistance du stockage dès le chargement (silencieux, voir
    // offline-queue.ts pour la stratégie complète).
    ensureStoragePersisted();

    // N°193 — capturé AVANT le register : « cette page était-elle déjà
    // contrôlée par un SW ? » Non = la toute première installation ; son
    // controllerchange (clients.claim) ne doit RIEN recharger.
    const hadController = !!navigator.serviceWorker.controller;

    let registration: ServiceWorkerRegistration | null = null;
    const register = () => {
      navigator.serviceWorker
        .register("/sw.js", { updateViaCache: "none" })
        .then((reg) => {
          registration = reg;
        })
        .catch(() => {
          /* PWA optionnelle : échec silencieux */
        });
    };
    if (document.readyState === "complete") register();
    else window.addEventListener("load", register, { once: true });

    // N°193 — le nouveau SW vient de prendre le contrôle (skipWaiting) :
    // la page exécute encore les ANCIENS bundles → reload pour charger le
    // déploiement courant. Reporté si le comptoir est ouvert (voir en-tête).
    const reloadForUpdate = () => {
      if (reloadedForUpdate) return;
      if (window.location.pathname === "/sell") {
        deferredUpdateReload = true;
        return;
      }
      reloadedForUpdate = true;
      window.location.reload();
    };
    navigator.serviceWorker.addEventListener("controllerchange", reloadForUpdate);

    // N°193 — issue du report : l'opérateur a quitté le Mode Vente
    // (navigation interne — le pathname change SANS recharger la page) ;
    // la mise à jour en attente s'applique enfin. Vérif légère, bornée :
    // le watcher ne vit que tant qu'un report est en attente.
    const deferredWatcher = window.setInterval(() => {
      if (!deferredUpdateReload || reloadedForUpdate) return;
      if (window.location.pathname !== "/sell") reloadForUpdate();
    }, DEFERRED_WATCH_MS);

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
      // N°193 — réarmement au passage en arrière-plan : la PROCHAINE
      // réouverture rechecke le sw.js, même 5 min après la dernière
      // vérification (une PWA consultée en continu pendant 29 min ne
      // pouvait JAMAIS voir un déploiement de moins de 30 min).
      if (document.visibilityState === "hidden") {
        lastCheck = 0;
        return;
      }
      checkForUpdate();
    };
    document.addEventListener("visibilitychange", onVisibility);
    window.addEventListener("online", checkForUpdate);
    // N°193 — kiosque de comptoir : la page peut vivre des heures en
    // PREMIER plan (aucun visibilitychange). Le check périodique passe
    // par le même throttle 30 min — un GET conditionnel tant que le
    // sw.js n'a pas changé.
    const periodicCheck = window.setInterval(checkForUpdate, UPDATE_MIN_INTERVAL_MS);

    return () => {
      window.removeEventListener("load", register);
      document.removeEventListener("visibilitychange", onVisibility);
      window.removeEventListener("online", checkForUpdate);
      navigator.serviceWorker.removeEventListener("controllerchange", reloadForUpdate);
      window.clearInterval(deferredWatcher);
      window.clearInterval(periodicCheck);
    };
  }, []);
  return null;
}
