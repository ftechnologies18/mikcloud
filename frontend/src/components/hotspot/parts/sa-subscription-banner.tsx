"use client";

// Bannière d'abonnement du tableau de bord — v1 bienveillante :
//   • abonnement expiré → bandeau ambre PERSISTANT (non refermable) : le statut
//     est affiché en toute transparence, l'accès reste maintenu, CTA vers la
//     carte Abonnement (Paramètres) ;
//   • échéance dans ≤ 7 jours → rappel doux refermable pour la session.
// Aucun blocage fonctionnel : MikCloud reste 100 % utilisable — la gérante ne
// perd jamais l'accès à ses données (le durcissement de facturation est une
// décision produit ultérieure, à activer seulement une fois le marché lancé).

import { useState } from "react";
import { CalendarClock, ShieldAlert } from "lucide-react";

import { Button } from "@/components/ui/button";
import { formatDate } from "@/lib/hotspot/format";
import { useI18n } from "@/lib/hotspot/i18n";
import { useHotspotStore } from "@/lib/hotspot/store";
import { useSubscription } from "@/components/hotspot/parts/sa-subscription-card";

// Jours restants à partir desquels le rappel doux « échéance proche » s'affiche.
const EXPIRING_SOON_DAYS = 7;
const DAY_MS = 86_400_000;

export function SubscriptionBanner() {
  const { t, tf, lang } = useI18n();
  const setView = useHotspotStore((s) => s.setView);
  const { data: view } = useSubscription();
  // Rappel « échéance proche » refermable pour la session (jamais persisté :
  // à la prochaine visite l'échéance redevient visible).
  const [expiringDismissed, setExpiringDismissed] = useState(false);

  if (!view) return null;

  // ── Abonnement expiré : bandeau persistant, ton bienveillant ──
  if (view.status === "expired") {
    return (
      <div
        role="alert"
        className="flex flex-col gap-3 rounded-xl border border-amber-500/40 bg-amber-500/10 px-4 py-3 sm:flex-row sm:items-center sm:justify-between"
      >
        <div className="flex min-w-0 items-start gap-3">
          <span className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg bg-amber-500/15 text-amber-600 dark:text-amber-400">
            <ShieldAlert className="size-4" aria-hidden />
          </span>
          <div className="min-w-0">
            <p className="text-sm font-semibold text-amber-700 dark:text-amber-300">
              {t("sub.banner.expiredTitle")}
            </p>
            <p className="mt-0.5 text-xs leading-relaxed text-amber-800/90 dark:text-amber-200/80">
              {tf("sub.banner.expiredDesc", { date: formatDate(view.subscription.periodEnd, lang) })}
            </p>
          </div>
        </div>
        <Button
          size="sm"
          className="shrink-0 self-start sm:self-center"
          onClick={() => setView("settings")}
        >
          {t("sub.banner.renew")}
        </Button>
      </div>
    );
  }

  // ── Échéance proche (≤ 7 jours) : rappel doux, refermable pour la session ──
  if (view.status === "active" && view.subscription.periodEnd && !expiringDismissed) {
    const days = Math.ceil((new Date(view.subscription.periodEnd).getTime() - Date.now()) / DAY_MS);
    if (days >= 0 && days <= EXPIRING_SOON_DAYS) {
      return (
        <div
          role="status"
          className="flex flex-col gap-3 rounded-xl border border-amber-500/25 bg-amber-500/5 px-4 py-3 sm:flex-row sm:items-center sm:justify-between"
        >
          <div className="flex min-w-0 items-start gap-3">
            <span className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg bg-amber-500/15 text-amber-600 dark:text-amber-400">
              <CalendarClock className="size-4" aria-hidden />
            </span>
            <div className="min-w-0">
              <p className="text-sm font-semibold text-amber-700 dark:text-amber-300">
                {t("sub.banner.expiringTitle")}
              </p>
              <p className="mt-0.5 text-xs leading-relaxed text-amber-800/90 dark:text-amber-200/80">
                {tf("sub.banner.expiringDesc", {
                  date: formatDate(view.subscription.periodEnd, lang),
                  days,
                })}
              </p>
            </div>
          </div>
          <div className="flex shrink-0 flex-wrap gap-2 self-start sm:self-center">
            <Button variant="ghost" size="sm" onClick={() => setExpiringDismissed(true)}>
              {t("sub.banner.later")}
            </Button>
            <Button size="sm" onClick={() => setView("settings")}>
              {t("sub.banner.renew")}
            </Button>
          </div>
        </div>
      );
    }
  }

  // Essai (aucune formule) ou échéance lointaine : pas de bannière.
  return null;
}
