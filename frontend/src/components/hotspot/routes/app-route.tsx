"use client";

// Route /app/[[...vue]] — console (gestionnaire, admin client, plateforme).
//
// Couche URL « Speed App UX » (Phase A) :
// 1. Garde d'accès : session requise ; un revendeur est renvoyé vers le
//    Mode Vente (/sell) — mêmes règles que l'ancien rendu conditionnel de
//    page.tsx, sans rien changer au store ni aux vues.
// 2. Synchronisation bidirectionnelle URL ↔ store, CONSCIENTE DE L'ORIGINE
//    du changement de vue :
//    - URL → store : boutons Retour/Avancer du navigateur (popstate) et
//      liens directs (/app/users…) mettent à jour la vue — sans créer
//      d'entrée d'historique (l'entrée existe déjà, c'est le navigateur
//      qui vient de la rejouer) ;
//    - store → URL : chaque changement de vue initié par l'interface
//      (sidebar, palette de recherche, bannières, impersonation, bascule
//      de console) pousse une entrée d'historique — via un abonnement
//      zustand SYNCHRONE (listener `subscribe`, état réel `state`/`prev`).
//      L'ancien effet React sur [view, pathname] tournait pendant un
//      popstate avec la vue PÉRIMÉE du commit et repoussait l'URL qu'on
//      venait de quitter : le Retour ne rejouait plus la navigation et le
//      ping-pong d'URLs consommait l'historique jusqu'à quitter l'app.
//    - Sans session (logout) : aucun push — la garde redirige vers /login.
// 3. Normalisation : /app (nu), slug inconnu, ou segment de détail orphelin
//    (2e segment sur une vue qui n'expose pas de détail) → replace vers la
//    vue cible — aucune entrée d'historique parasite.
// 4. Préchauffe (B4) : les chunks des vues les plus consultées sont
//    importés quand le navigateur est inactif — premier clic instantané.

import { useEffect, useRef } from "react";
import { usePathname, useRouter } from "next/navigation";
import AppShell from "@/components/hotspot/app-shell";
import { useMounted } from "@/hooks/use-mounted";
import { useHotspotStore } from "@/lib/hotspot/store";
import { APP_BASE_PATH, detailFromPath, viewFromPath, viewToPath } from "@/lib/hotspot/view-path";
import { ShellFallback } from "./shell-fallback";

export default function AppRoute() {
  const router = useRouter();
  const pathname = usePathname();

  // Fenêtre pré-montage : le HTML prérendu = ShellFallback (cohérence SSR),
  // les décisions (garde, sync URL) attendent le montage — le store persisté
  // (zustand/localStorage) est alors rehydraté.
  const mounted = useMounted();

  const token = useHotspotStore((s) => s.token);
  const userRole = useHotspotStore((s) => s.user?.role);

  // Dernier chemin connu, lu par l'abonnement store→URL : celui-ci
  // s'exécute hors rendu React (callback zustand), il ne peut pas lire
  // `pathname` sans closure périmée — on maintient un miroir à jour.
  const pathnameRef = useRef(pathname);
  useEffect(() => {
    pathnameRef.current = pathname;
  }, [pathname]);

  // Marque le setView en cours comme initié par l'URL (popstate, lien
  // direct) : l'abonnement store→URL ne repousse alors PAS d'entrée —
  // l'entrée existe déjà, c'est le navigateur qui vient de la rejouer.
  const urlDriven = useRef(false);

  // Garde : sans session → connexion ; revendeur → Mode Vente.
  useEffect(() => {
    if (!mounted) return;
    if (!token) router.replace("/login");
    else if (userRole === "reseller") router.replace("/sell");
  }, [mounted, token, userRole, router]);

  // store → URL : abonnement zustand (callback synchrone, aucune closure
  // React périmée). C'est la SEULE source fiable de l'origine du changement
  // de vue : navigations interface → push ; changements venus de l'URL →
  // rien ; changement sans session (logout) → rien.
  useEffect(() => {
    const unsubscribe = useHotspotStore.subscribe((state, prev) => {
      if (state.view === prev.view) return; // autre champ (sidebarOpen…) : rien à faire
      if (urlDriven.current) {
        urlDriven.current = false; // venu de l'URL : l'entrée existe déjà
        return;
      }
      if (!state.token) return; // logout : la garde redirige vers /login
      const path = viewToPath(state.view);
      if (pathnameRef.current !== path) {
        router.push(path, { scroll: false }); // navigation réelle → entrée d'historique
      }
    });
    return unsubscribe;
  }, [router]);

  // URL → store (back/forward, liens directs, normalisation).
  useEffect(() => {
    if (!pathname.startsWith(APP_BASE_PATH)) return; // transitions hors console : rien à faire
    const target = viewFromPath(pathname);
    if (target) {
      if (target !== useHotspotStore.getState().view) {
        urlDriven.current = true;
        useHotspotStore.getState().setView(target);
        // Consommé par l'abonnement s'il est enregistré ; nettoyé ici sinon
        // (fenêtre de montage) — le flag ne fuite jamais vers le clic suivant.
        urlDriven.current = false;
      }
      // Phase D — segment de détail orphelin (2e segment sur une vue qui
      // n'expose pas de détail, ex. /app/dashboard/xyz) : re-normaliser —
      // seules users/vouchers/sessions adressent un détail, consommé par la
      // vue elle-même (la vue ne re-normalise PAS son propre segment).
      if (detailFromPath(pathname, target) === null && pathname !== viewToPath(target)) {
        router.replace(viewToPath(target), { scroll: false });
      }
    } else {
      // /app nu ou slug inconnu : normaliser sur la vue courante — replace,
      // aucune entrée d'historique parasite.
      router.replace(viewToPath(useHotspotStore.getState().view), { scroll: false });
    }
  }, [pathname, router]);

  // B4 — préchauffe des vues chaudes à l'idle (sessions, utilisateurs,
  // vouchers) : les chunks sont déjà en cache au premier clic.
  useEffect(() => {
    if (!mounted || !token || userRole === "reseller") return;
    const warm = () => {
      void import("@/components/hotspot/views/sessions-view");
      void import("@/components/hotspot/views/users-view");
      void import("@/components/hotspot/views/vouchers-view");
    };
    const w = window as Window & {
      requestIdleCallback?: (cb: () => void, opts?: { timeout: number }) => number;
      cancelIdleCallback?: (id: number) => void;
    };
    if (typeof w.requestIdleCallback === "function") {
      const id = w.requestIdleCallback(warm, { timeout: 3000 });
      return () => w.cancelIdleCallback?.(id);
    }
    const id = window.setTimeout(warm, 1500);
    return () => window.clearTimeout(id);
  }, [mounted, token, userRole]);

  if (!mounted || !token || userRole === "reseller") return <ShellFallback />;
  return <AppShell />;
}
