"use client";

// Barre de vie d'un lot — ÉLÉMENT SIGNATURE de l'onglet Lots (v2) :
// h-6 avec le CHIFFRE de chaque segment affiché dans le segment. Vocabulaire
// métier unifié : en stock (primary = vendable), éculés (chart-3 = vendus +
// consommés, FUSIONNÉS — du chiffre d'affaires, jamais une perte), expirés
// (destructive = perte sèche), purgés (piste vide). Les couleurs reprennent la
// sémantique des badges pour rester lisibles d'un écran à l'autre.

import { Badge } from "@/components/ui/badge";
import { useI18n } from "@/lib/hotspot/i18n";
import type { BatchLifecycle } from "@/lib/hotspot/types";
import { cn } from "@/lib/utils";

interface BatchLifeBarProps {
  count: number;
  active: number;
  used: number;
  expired: number;
  disabled: number;
  /** Légende compacte sous la barre — défaut : NON (v2 : les chiffres sont
   * dans les segments, l'aria-label porte toujours la composition complète). */
  legend?: boolean;
  className?: string;
}

export function BatchLifeBar({
  count,
  active,
  used,
  expired,
  disabled,
  legend = false,
  className,
}: BatchLifeBarProps) {
  const { t, tf } = useI18n();
  // v2 — fusion métier : used + disabled = « éculés » (vendus/consommés = CA).
  const sold = used + disabled;
  const sum = active + sold + expired;
  const total = Math.max(count, sum);
  const purged = Math.max(0, count - sum);
  if (total === 0) return null;
  const segments = [
    { n: active, cls: "bg-primary", numCls: "text-primary-foreground", key: "vouchers.batches.legendActive" },
    { n: sold, cls: "bg-chart-3", numCls: "text-amber-950", key: "vouchers.batches.legendConsumed" },
    { n: expired, cls: "bg-destructive/70", numCls: "text-white", key: "vouchers.batches.legendExpired" },
  ];
  const visible = segments.filter((s) => s.n > 0);
  return (
    <div className={cn("space-y-1.5", className)}>
      {/* Purged : count - (active + éculés + expirés) reste en piste vide.
          Somme nulle + purgés > 0 → piste vide, sans chiffres. */}
      <div
        className="flex h-6 w-full gap-px overflow-hidden rounded-sm bg-muted"
        role="img"
        aria-label={visible
          .map((s) => `${s.n} ${t(s.key)}`)
          .concat(purged > 0 ? [`${purged} ${t("vouchers.batches.legendPurged")}`] : [])
          .join(", ")}
      >
        {visible.map((s) => {
          const pct = (s.n / total) * 100;
          // Chiffre affiché uniquement si le segment est assez large (≥ 10 %)
          // pour le porter — sinon l'aria-label garde l'information.
          const showCount = pct >= 10;
          return (
            <span
              key={s.key}
              className={cn("flex h-full items-center justify-center rounded-sm", s.cls)}
              style={{ width: `${pct}%` }}
            >
              {showCount && (
                <span className={cn("px-1 text-[10px] font-semibold tabular-nums leading-none", s.numCls)}>
                  {s.n}
                </span>
              )}
            </span>
          );
        })}
      </div>
      {legend && (
        <p className="flex flex-wrap gap-x-3 gap-y-0.5 text-xs text-muted-foreground">
          {visible.map((s) => (
            <span key={s.key} className="inline-flex items-center gap-1">
              <span aria-hidden className={cn("size-1.5 rounded-full", s.cls)} />
              <span className="tabular-nums">{s.n}</span> {t(s.key)}
            </span>
          ))}
          {purged > 0 && (
            <span className="inline-flex items-center gap-1">
              <span aria-hidden className="size-1.5 rounded-full border border-muted-foreground/50 bg-transparent" />
              <span className="tabular-nums">{purged}</span> {t("vouchers.batches.legendPurged")}
            </span>
          )}
          {visible.length === 0 && purged === 0 && (
            <span>{tf("vouchers.batches.ofTickets", { n: count })}</span>
          )}
        </p>
      )}
    </div>
  );
}

/** Badge du cycle de vie du lot (sémantique alignée sur StatusBadge). */
const LIFE_BADGE: Record<BatchLifecycle, { className: string; labelKey: string }> = {
  stock: { className: "bg-primary/15 text-primary border-primary/25", labelKey: "vouchers.batches.life.stock" },
  consumed: { className: "bg-chart-3/15 text-chart-3 border-chart-3/25", labelKey: "vouchers.batches.life.consumed" },
  expired: {
    className: "bg-amber-500/15 text-amber-600 border-amber-500/25 dark:text-amber-400",
    labelKey: "vouchers.batches.life.expired",
  },
  purged: { className: "bg-muted text-muted-foreground border-border", labelKey: "vouchers.batches.life.purged" },
};

export function BatchLifeBadge({ status, className }: { status: BatchLifecycle; className?: string }) {
  const { t } = useI18n();
  const cfg = LIFE_BADGE[status];
  return (
    <Badge variant="outline" className={cn("gap-1 px-1.5 py-0 text-[10px] font-medium", cfg.className, className)}>
      {t(cfg.labelKey)}
    </Badge>
  );
}
