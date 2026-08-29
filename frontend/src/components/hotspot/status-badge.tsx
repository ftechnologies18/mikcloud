"use client";

import { Badge } from "@/components/ui/badge";
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
  | "direct"
  | "reseller"
  | "credit"
  | "sale";

const LABELS: Record<BadgeStatus, string> = {
  active: "Actif",
  disabled: "Désactivé",
  used: "Utilisé",
  expired: "Expiré",
  online: "En ligne",
  offline: "Hors ligne",
  simulated: "Simulé",
  real: "Réel",
  direct: "Direct",
  reseller: "Revendeur",
  credit: "Crédit",
  sale: "Vente",
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
  return (
    <Badge variant="outline" className={cn("gap-1.5 font-medium", STYLES[status], className)}>
      {dot && <span className="size-1.5 rounded-full bg-current" aria-hidden />}
      {label ?? LABELS[status]}
    </Badge>
  );
}
