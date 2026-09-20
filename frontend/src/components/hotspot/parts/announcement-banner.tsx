"use client";

// N°152 — bandeau d'annonces de la plateforme : la plus récente annonce
// ACTIVE non masquée s'affiche sous le header de la console client, couleur
// selon le niveau (info = émeraude, warning = ambre, critical = rouge).
// Masquage PAR UTILISATEUR ET PAR ANNONCE (localStorage : le bandeau est
// informatif, la trace durable vit dans la cloche). L'annonce reste
// accessible via la cloche et la destination « tout voir ».
// N°165 — plus aucun message tronqué sans issue : la zone de message ouvre
// une fenêtre de LECTURE COMPLÈTE (titre + corps intégral, défilement,
// retours à la ligne préservés, date de fin de visibilité). Le titre et
// l'extrait restent élégamment tronqués en ligne, mais le chevron « Lire »
// et le clic sur le bandeau donnent toujours accès au texte entier — la
// cloche et cette fenêtre lisent la même vérité (tri par date effective :
// une annonce programmée qui vient d'être publiée passe devant).
// Contrat : GET /api/announcements (rang 2+, annonces actives du compte).

import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { ChevronDown, Info, Megaphone, TriangleAlert, X } from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { fetchClientAnnouncements } from "@/lib/hotspot/api";
import { useI18n } from "@/lib/hotspot/i18n";
import { useHotspotStore } from "@/lib/hotspot/store";
import type { Announcement } from "@/lib/hotspot/types";

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
  const { t, tf, lang } = useI18n();
  const user = useHotspotStore((s) => s.user);
  // Masquages locaux — re-render immédiat, pas d'attente de refetch.
  const [dismissed, setDismissed] = useState<Record<string, boolean>>({});
  // N°165 — fenêtre de lecture complète du message (bandeau non tronqué).
  const [reading, setReading] = useState<Announcement | null>(null);

  // Annonces actives du compte — rafraîchies toutes les 5 min (un bandeau
  // d'annonce ne demande pas la fraîcheur de 60 s de la cloche).
  const { data } = useQuery({
    queryKey: ["/api/announcements"],
    queryFn: fetchClientAnnouncements,
    refetchInterval: 300_000,
    staleTime: 120_000,
    retry: false,
  });

  // La plus récente non masquée (les annonces viennent triées récent-d'abord
  // par date effective ; un incident plus récent chasse une info plus ancienne).
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

  // N°165 — dates en clair dans la fenêtre de lecture (heure locale du
  // gérant ; l'Abidjan GMT est le public naturel mais la console suit la
  // locale du navigateur).
  const dateFmt = new Intl.DateTimeFormat(lang === "fr" ? "fr-FR" : "en-US", {
    day: "numeric",
    month: "long",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
  const readDialog = reading;

  function dismiss() {
    const key = dismissKey(user?.id ?? "", current!.id);
    if (typeof window !== "undefined") {
      window.localStorage.setItem(key, "1");
    }
    setDismissed((prev) => ({ ...prev, [key]: true }));
  }

  return (
    <div className={`${styles.wrapper} px-4 py-2 sm:px-6`} role="status">
      <div className="flex min-h-9 items-center justify-between gap-2 sm:gap-3">
        {/* Zone message = lecture complète (N°165) : cible tactile large,
            chevron d'affordance — le texte tronqué en ligne n'est jamais
            une impasse, tout le contenu vit dans la fenêtre. */}
        <button
          type="button"
          onClick={() => setReading(current)}
          aria-label={t("ann.banner.read")}
          title={t("ann.banner.read")}
          className={`flex min-w-0 flex-1 items-center gap-2 rounded-md px-1.5 py-1 text-left text-sm ${styles.text} hover:bg-foreground/5 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring`}
        >
          <Icon className={`size-4 shrink-0 ${styles.icon}`} aria-hidden />
          <span className="min-w-0 flex-1">
            <span className="block truncate font-medium">{current.title}</span>
            {current.body ? (
              <span className="hidden min-w-0 truncate text-[13px] opacity-80 md:block">
                {current.body}
              </span>
            ) : null}
          </span>
          <span className="hidden shrink-0 rounded-full border border-border/60 px-1.5 py-0 text-[10px] font-semibold uppercase tracking-wide opacity-70 lg:inline">
            {t("ann.banner.platform")}
          </span>
          <ChevronDown className={`size-4 shrink-0 ${styles.icon}`} aria-hidden />
        </button>
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

      {/* — N°165 : lecture complète (message long sans troncature) — */}
      <Dialog open={!!readDialog} onOpenChange={(open) => !open && setReading(null)}>
        <DialogContent className="max-h-[85dvh] overflow-y-auto sm:max-w-lg">
          <DialogHeader>
            <DialogTitle className="flex items-start gap-2.5 pr-2 leading-snug">
              {readDialog ? (
                (() => {
                  const ReadIcon = LEVEL_ICON[readDialog.level] ?? Megaphone;
                  const readStyles = LEVEL_STYLES[readDialog.level] ?? LEVEL_STYLES.info;
                  return (
                    <ReadIcon className={`mt-0.5 size-5 shrink-0 ${readStyles.icon}`} aria-hidden />
                  );
                })()
              ) : null}
              <span>{readDialog?.title}</span>
            </DialogTitle>
            <DialogDescription className="flex flex-wrap items-center gap-2">
              <span className="inline-block rounded-full border border-border/60 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-muted-foreground">
                {t("ann.banner.platform")}
              </span>
              <span>{readDialog ? t(`ann.level.${readDialog.level}`) : ""}</span>
            </DialogDescription>
          </DialogHeader>

          {/* Corps intégral : retours à la ligne préservés, défilement
              autonome pour les longs messages — plus jamais coupé. */}
          {readDialog?.body ? (
            <p className="max-h-[55dvh] overflow-y-auto whitespace-pre-line break-words text-sm leading-relaxed text-foreground/90">
              {readDialog.body}
            </p>
          ) : null}

          <p className="text-xs text-muted-foreground">
            {readDialog?.publishAt
              ? tf("ann.banner.published", { date: dateFmt.format(new Date(readDialog.publishAt)) })
              : tf("ann.banner.published", { date: dateFmt.format(new Date(readDialog?.createdAt ?? Date.now())) })}
          </p>

          <DialogFooter>
            {readDialog?.expiresAt ? (
              <p className="mr-auto text-xs text-muted-foreground">
                {tf("ann.banner.until", { date: dateFmt.format(new Date(readDialog.expiresAt)) })}
              </p>
            ) : null}
            <Button variant="outline" onClick={() => setReading(null)} className="min-h-10">
              {t("ann.banner.close")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
