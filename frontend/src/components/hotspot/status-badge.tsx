"use client";

import { Badge } from "@/components/ui/badge";
import { useI18n } from "@/lib/hotspot/i18n";
import { cn } from "@/lib/utils";

export type BadgeStatus =
  | "active"
  | "disabled"
  | "used"
  | "expired"
  | "online"
  | "offline"
  | "simulated"
  | "real"
  | "agent"
  | "direct"
  | "reseller"
  | "credit"
  | "sale";

/** Clés i18n par statut (F11) — le libellé est résolu à chaque rendu. */
const LABEL_KEYS: Record<BadgeStatus, string> = {
  active: "badge.active",
  disabled: "badge.disabled",
  used: "badge.used",
  expired: "badge.expired",
  online: "badge.online",
  offline: "badge.offline",
  simulated: "badge.simulated",
  real: "badge.real",
  agent: "badge.agent",
  direct: "badge.direct",
  reseller: "badge.reseller",
  credit: "badge.credit",
  sale: "badge.sale",
};

const STYLES: Record<BadgeStatus, string> = {
  active: "bg-primary/15 text-primary border-primary/25",
  online: "bg-primary/15 text-primary border-primary/25",
  used: "bg-chart-3/15 text-chart-3 border-chart-3/25",
  reseller: "bg-chart-3/15 text-chart-3 border-chart-3/25",
  sale: "bg-chart-3/15 text-chart-3 border-chart-3/25",
  expired: "bg-muted text-muted-foreground border-border",
  disabled: "bg-muted text-muted-foreground border-border",
  offline: "bg-destructive/15 text-destructive border-destructive/25",
  simulated: "bg-chart-2/15 text-chart-2 border-chart-2/25",
  real: "bg-chart-5/15 text-chart-5 border-chart-5/25",
  agent: "bg-chart-4/15 text-chart-4 border-chart-4/25",
  direct: "bg-chart-5/15 text-chart-5 border-chart-5/25",
  credit: "bg-primary/15 text-primary border-primary/25",
};

export function StatusBadge({
  status,
  label,
  className,
  dot,
}: {
  status: BadgeStatus;
  label?: string;
  className?: string;
  dot?: boolean;
}) {
  const { t } = useI18n();
  return (
    <Badge variant="outline" className={cn("gap-1.5 font-medium", STYLES[status], className)}>
      {dot && <span className="size-1.5 rounded-full bg-current" aria-hidden />}
      {label ?? t(LABEL_KEYS[status])}
    </Badge>
  );
}
