"use client";

import { useEffect, useState } from "react";
import { useTheme } from "next-themes";
import { Moon, Sun } from "lucide-react";

import { Button } from "@/components/ui/button";
import { useI18n } from "@/lib/hotspot/i18n";

/**
 * Bascule Nuit ☾ / Jour ☀ — identité duale MikCloud « Aurora Emerald ».
 * Rotation douce entre les deux icônes en CSS pur (N°78 : keyframe
 * mik-icon-in — framer-motion hors du bundle initial ; le montage par
 * `key` rejoue l'animation à chaque bascule, la sortie est instantanée) —
 * éviter le mismatch SSR : rendu neutre jusqu'à ce que le thème soit
 * résolu.
 */
export function ThemeToggle() {
  const { resolvedTheme, setTheme } = useTheme();
  const { t } = useI18n();
  const [mounted, setMounted] = useState(false);

  // Pattern officiel next-themes : le thème n'est connu qu'après montage
  // (localStorage) — rendu icône neutre au SSR pour éviter le mismatch.
  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect -- bascule unique post-hydratation, aucun rendu en cascade
    setMounted(true);
  }, []);

  const isDark = resolvedTheme !== "light";

  return (
    <Button
      variant="ghost"
      size="icon"
      className="size-10 text-muted-foreground hover:text-foreground"
      onClick={() => setTheme(isDark ? "light" : "dark")}
      aria-label={t(isDark ? "theme.toLight" : "theme.toDark")}
      title={t(isDark ? "theme.toLight" : "theme.toDark")}
    >
      {mounted ? (
        <span key={isDark ? "moon" : "sun"} className="mik-icon-in flex">
          {isDark ? <Moon className="size-4.5" aria-hidden /> : <Sun className="size-4.5" aria-hidden />}
        </span>
      ) : (
        <Sun className="size-4.5" aria-hidden />
      )}
    </Button>
  );
}
