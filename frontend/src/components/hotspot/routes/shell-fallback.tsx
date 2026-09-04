"use client";

import { Loader2 } from "lucide-react";

/** Écran d'attente partagé — rendu pendant la fenêtre pré-montage des
 * routes gardées (cohérence SSR : le HTML prérendu = fallback), pendant le
 * chargement différé d'un shell, et pendant une redirection de garde.
 * Fond nuit identitaire, spinner accessible. */
export function ShellFallback() {
  return (
    <div
      className="flex min-h-screen items-center justify-center bg-background"
      role="status"
      aria-live="polite"
    >
      <Loader2 className="size-7 animate-spin text-muted-foreground" aria-hidden="true" />
    </div>
  );
}
