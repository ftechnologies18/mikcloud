"use client";

import { useEffect, useState } from "react";
import { useTheme } from "next-themes";
import { AnimatePresence, motion } from "framer-motion";
import { Moon, Sun } from "lucide-react";

import { Button } from "@/components/ui/button";
import { useI18n } from "@/lib/hotspot/i18n";

/**
 * Bascule Nuit ☾ / Jour ☀ — identité duale MikCloud « Aurora Emerald ».
 * Rotation douce Framer Motion entre les deux icônes (monté→éviter le
 * mismatch SSR : rendu neutre jusqu'à ce que le thème soit résolu).
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
        <AnimatePresence mode="wait" initial={false}>
          <motion.span
            key={isDark ? "moon" : "sun"}
            initial={{ rotate: -70, opacity: 0, scale: 0.7 }}
            animate={{ rotate: 0, opacity: 1, scale: 1 }}
            exit={{ rotate: 70, opacity: 0, scale: 0.7 }}
            transition={{ duration: 0.25, ease: "easeOut" }}
            className="flex"
          >
            {isDark ? <Moon className="size-4.5" aria-hidden /> : <Sun className="size-4.5" aria-hidden />}
          </motion.span>
        </AnimatePresence>
      ) : (
        <Sun className="size-4.5" aria-hidden />
      )}
    </Button>
  );
}
