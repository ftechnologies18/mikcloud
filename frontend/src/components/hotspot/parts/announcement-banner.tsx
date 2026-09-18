"use client";

// N°152 — bandeau d'annonces de la plateforme : la plus récente annonce
// ACTIVE non masquée s'affiche sous le header de la console client, couleur
// selon le niveau (info = émeraude, warning = ambre, critical = rouge).
// Masquage PAR UTILISATEUR ET PAR ANNONCE (localStorage : le bandeau est
// informatif, la trace durable vit dans la cloche). L'annonce reste
// accessible via la cloche et la destination « tout voir ».
// Contrat : GET /api/announcements (rang 2+, annonces actives du compte).

import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Info, Megaphone, TriangleAlert, X } from "lucide-react";

import { Button } from "@/components/ui/button";
import { fetchClientAnnouncements } from "@/lib/hotspot/api";
import { useI18n } from "@/lib/hotspot/i18n";
import { useHotspotStore } from "@/lib/hotspot/store";

/** Clé de masquage par (utilisateur, annonce) — namespace mikcloud:. */
function dismissKey(userID: string, announcementID: string): string {
  return `mikcloud:ann-dismissed:${userID}:${announcementID}`;
}

/** Habillage du bandeau selon le niveau — miroir des couleurs console. */
const LEVEL_STYLES = {
  info: {
    wrapper: "border-b border-emerald-600/20 bg-emerald-500/10",
    text: "text-emerald-700 dark:text-emerald-400",
    icon: "text-emerald-600 dark:text-emerald-400",
  },
  warning: {
    wrapper: "border-b border-amber-600/25 bg-amber-500/10",
    text: "text-amber-800 dark:text-amber-300",
    icon: "text-amber-600 dark:text-amber-400",
  },
  critical: {
    wrapper: "border-b border-destructive/25 bg-destructive/10",
    text: "text-destructive",
    icon: "text-destructive",
  },
} as const;

const LEVEL_ICON = {
  info: Info,
  warning: TriangleAlert,
  critical: TriangleAlert,
} as const;

export function AnnouncementBanner() {
  const { t } = useI18n();
  const user = useHotspotStore((s) => s.user);
  // Masquages locaux — re-render immédiat, pas d'attente de refetch.
  const [dismissed, setDismissed] = useState<Record<string, boolean>>({});

  // Annonces actives du compte — rafraîchies toutes les 5 min (un bandeau
  // d'annonce ne demande pas la fraîcheur de 60 s de la cloche).
  const { data } = useQuery({
    queryKey: ["/api/announcements"],
    queryFn: fetchClientAnnouncements,
    refetchInterval: 300_000,
    staleTime: 120_000,
    retry: false,
  });

  // La plus récente non masquée (les annonces viennent triées récent-d'abord ;
  // un incident plus récent chasse une info plus ancienne).
  const current = useMemo(() => {
    const userID = user?.id ?? "";
    if (!data?.length) return null;
    for (const ann of data) {
      const key = dismissKey(userID, ann.id);
      const locallyDismissed =
        dismissed[key] ||
        (typeof window !== "undefined" && window.localStorage.getItem(key) === "1");
      if (!locallyDismissed) return ann;
    }
    return null;
  }, [data, dismissed, user?.id]);

  if (!current) return null;

  const styles = LEVEL_STYLES[current.level] ?? LEVEL_STYLES.info;
  const Icon = LEVEL_ICON[current.level] ?? Megaphone;

  function dismiss() {
    const key = dismissKey(user?.id ?? "", current!.id);
    if (typeof window !== "undefined") {
      window.localStorage.setItem(key, "1");
    }
    setDismissed((prev) => ({ ...prev, [key]: true }));
  }

  return (
    <div className={`${styles.wrapper} px-4 py-2 sm:px-6`} role="status">
      <div className="flex min-h-9 items-center justify-between gap-3">
        <p className={`flex min-w-0 items-center gap-2 text-sm ${styles.text}`}>
          <Icon className={`size-4 shrink-0 ${styles.icon}`} aria-hidden />
          <span className="truncate font-medium">{current.title}</span>
          {current.body ? (
            <span className="hidden min-w-0 truncate opacity-80 md:inline">
              <span className="opacity-60">·</span> {current.body}
            </span>
          ) : null}
          <span className="hidden shrink-0 rounded-full border border-border/60 px-1.5 py-0 text-[10px] font-semibold uppercase tracking-wide opacity-70 lg:inline">
            {t("ann.banner.platform")}
          </span>
        </p>
        <Button
          type="button"
          size="sm"
          variant="ghost"
          className={`h-8 shrink-0 ${styles.text} hover:bg-foreground/5`}
          aria-label={t("ann.banner.dismiss")}
          onClick={dismiss}
        >
          <X className="size-4" />
          <span className="hidden sm:inline">{t("ann.banner.dismiss")}</span>
        </Button>
      </div>
    </div>
  );
}
