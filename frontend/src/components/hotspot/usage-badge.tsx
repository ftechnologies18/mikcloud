"use client";

// Badge d'usage de compte SaaS (N°98 — Hotspot vs HomeNet).
// Partagé entre la liste des comptes (accounts-view) et la fiche détail
// (account-detail-dialog) — même patron que status-badge.tsx.

import { Badge } from "@/components/ui/badge";
import { useI18n } from "@/lib/hotspot/i18n";
import type { AccountUsage } from "@/lib/hotspot/types";

/** Hotspot = le produit historique (émeraude, comme l'actif) ; HomeNet = les
 * réseaux domestiques (ambre, la chaleur de la maison). Absent = hotspot
 * (ère pré-colonne — le serveur normalise de toute façon). */
export function UsageBadge({ usage }: { usage?: AccountUsage }) {
  const { t } = useI18n();
  if (usage === "homenet") {
    return (
      <Badge
        variant="outline"
        className="border-amber-500/30 bg-amber-500/10 px-1.5 py-0 text-[10px] font-medium text-amber-600 dark:text-amber-400"
      >
        {t("accounts.usageHomeNet")}
      </Badge>
    );
  }
  return (
    <Badge
      variant="outline"
      className="border-emerald-500/30 bg-emerald-500/10 px-1.5 py-0 text-[10px] font-medium text-emerald-600 dark:text-emerald-400"
    >
      {t("accounts.usageHotspot")}
    </Badge>
  );
}
