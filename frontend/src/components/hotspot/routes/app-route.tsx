"use client";

// Route /app/[[...vue]] — console (gestionnaire, admin client, plateforme).
//
// Couche URL « Speed App UX » (Phase A) :
// 1. Garde d'accès : session requise ; un revendeur est renvoyé vers le
//    Mode Vente (/sell) — mêmes règles que l'ancien rendu conditionnel de
//    page.tsx, sans rien changer au store ni aux vues.
// 2. Synchronisation bidirectionnelle URL ↔ store :
//    - URL → store : boutons Retour/Avancer du navigateur (popstate) et
//      liens directs (/app/users…) mettent à jour la vue courante ;
//    - store → URL : chaque changement de vue (sidebar, palette de
//      recherche, redirections internes) crée une entrée d'historique.
//    Les deux sens sont idempotents (comparaison avant écriture) : pas de
//    boucle, pas d'entrée parasite.
// 3. Préchauffe (B4) : les chunks des vues les plus consultées sont
//    importés quand le navigateur est inactif — premier clic instantané.

import { useEffect, useRef } from "react";
import { usePathname, useRouter } from "next/navigation";
import AppShell from "@/components/hotspot/app-shell";
import { useMounted } from "@/hooks/use-mounted";
import { useHotspotStore } from "@/lib/hotspot/store";
import { viewFromPath, viewToPath } from "@/lib/hotspot/view-path";
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
  const view = useHotspotStore((s) => s.view);
  const setView = useHotspotStore((s) => s.setView);
  const normalized = useRef(false);

  // Garde : sans session → connexion ; revendeur → Mode Vente.
  useEffect(() => {
    if (!mounted) return;
    if (!token) router.replace("/login");
    else if (userRole === "reseller") router.replace("/sell");
  }, [mounted, token, userRole, router]);

  // URL → store (back/forward + liens directs).
  useEffect(() => {
    const target = viewFromPath(pathname);
    if (target && target !== useHotspotStore.getState().view) setView(target);
  }, [pathname, setView]);

  // store → URL (historique réel de la navigation console).
  useEffect(() => {
    if (!mounted) return;
    const path = viewToPath(view);
    if (pathname === path) return;
    // 1er alignement en replace : normaliser /app → /app/<vue> sans créer
    // d'entrée d'historique parasite ; ensuite, push à chaque navigation.
    if (!normalized.current) {
      normalized.current = true;
      router.replace(path, { scroll: false });
    } else {
      router.push(path, { scroll: false });
    }
  }, [mounted, view, pathname, router]);

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
