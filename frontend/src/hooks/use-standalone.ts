"use client";

// N°8 — Détection « app installée » (PWA standalone) : lancée depuis l'écran
// d'accueil, la PWA doit s'ouvrir directement sur l'écran de connexion — la
// vitrine publique (landing) reste réservée au navigateur.
//
// useSyncExternalStore est indispensable ici : le snapshot serveur est
// toujours false (le HTML prérendu reste la landing, cohérence SSR), puis
// React bascule vers le snapshot client dès le montage SANS erreur
// d'hydratation et sans attendre un second rendu visible.
import * as React from "react";

/** iOS < 16.4 ignore la media query display-mode → fallback propriétaire
 * navigator.standalone (true uniquement depuis l'écran d'accueil). */
function readStandalone(): boolean {
  if (typeof window === "undefined") return false;
  if (window.matchMedia?.("(display-mode: standalone)").matches) return true;
  return (window.navigator as Navigator & { standalone?: boolean }).standalone === true;
}

export function useIsStandalone(): boolean {
  const subscribe = React.useCallback((onChange: () => void) => {
    const mql = window.matchMedia("(display-mode: standalone)");
    mql.addEventListener("change", onChange);
    return () => mql.removeEventListener("change", onChange);
  }, []);

  return React.useSyncExternalStore(
    subscribe,
    readStandalone,
    // Snapshot serveur : jamais standalone → hydratation toujours cohérente.
    () => false,
  );
}
