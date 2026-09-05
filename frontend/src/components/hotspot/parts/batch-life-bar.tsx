"use client";

// Barre de « fiche de vie » d'un lot (refonte onglet Lots) : composition
// visuelle du stock — en stock (primary), vendus (chart-3), expirés (gris),
// désactivés (destructive) — le reste (purgés) restant en piste vide.
// Les couleurs reprennent la sémantique de StatusBadge pour rester lisibles
// d'un écran à l'autre (table, cartes mobiles, fiche 360°).

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
  /** Légende compacte sous la barre (segments > 0 + purgés). Défaut : oui. */
  legend?: boolean;
  className?: string;
}

export function BatchLifeBar({
  count,
  active,
  used,
  expired,
  disabled,
  legend = true,
  className,
}: BatchLifeBarProps) {
  const { t, tf } = useI18n();
  const sum = active + used + expired + disabled;
  const total = Math.max(count, sum);
  const purged = Math.max(0, count - sum);
  if (total === 0) return null;
  const segments = [
    { n: active, cls: "bg-primary", key: "vouchers.batches.legendActive" },
    { n: used, cls: "bg-chart-3", key: "vouchers.batches.legendUsed" },
    { n: expired, cls: "bg-muted-foreground/40", key: "vouchers.batches.legendExpired" },
    { n: disabled, cls: "bg-destructive/70", key: "vouchers.batches.legendDisabled" },
  ];
  const visible = segments.filter((s) => s.n > 0);
  return (
    <div className={cn("space-y-1.5", className)}>
      {/* Purged : count - (active+used+expired+disabled) reste en piste vide. */}
      <div
        className="flex h-2 w-full overflow-hidden rounded-full bg-muted"
        role="img"
        aria-label={visible
          .map((s) => `${s.n} ${t(s.key)}`)
          .concat(purged > 0 ? [`${purged} ${t("vouchers.batches.legendPurged")}`] : [])
          .join(", ")}
      >
        {visible.map((s) => (
          <span
            key={s.key}
            className={cn("h-full", s.cls)}
            style={{ width: `${(s.n / total) * 100}%` }}
          />
        ))}
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

/** Refonte — badge du cycle de vie du lot (sémantique alignée sur StatusBadge). */
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
