"use client";

// Détection de montage SSR-sûre — même mécanique que use-standalone.ts :
// useSyncExternalStore avec snapshot serveur toujours `false`. Le HTML
// prérendu (serveur) et le premier rendu client (hydratation) voient
// `false`, puis React bascule vers le snapshot client `true` dès le montage
// — sans erreur d'hydratation. Utilisée par les routes gardées pour ne
// prendre leurs décisions (session, redirections, sync URL) qu'après
// montage, quand le store persisté (zustand/localStorage) est rehydraté.
//
// (Contrairement à un `useEffect(() => setMounted(true), [])`, cette forme
// n'enchaîne pas de rendus via setState dans un effet — règle
// react-hooks/set-state-in-effect.)
import * as React from "react";

const emptySubscribe = () => () => {};

export function useMounted(): boolean {
  return React.useSyncExternalStore(
    emptySubscribe,
    () => true, // snapshot client : composant monté
    () => false, // snapshot serveur : jamais monté
  );
}
