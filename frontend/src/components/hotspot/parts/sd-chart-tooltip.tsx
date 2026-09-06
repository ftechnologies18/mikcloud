"use client";

// Tooltip sombre partagé pour les graphiques recharts (vues Rapports).
// recharts 3 : le contenu personnalisé reçoit TooltipContentProps<TValue,
// TName> (active/payload/label y sont intégrés — l'ancien TooltipProps les
// omet des props de composant).

import type { TooltipContentProps } from "recharts";

// Partial : recharts fournit active/payload/label/coordinate AU RENDU via
// content={<ChartTooltip formatter={...} />} — les sites consommateurs ne
// déclarent que formatter (sinon TS2739 props requises manquantes).
type ChartTooltipProps = Partial<TooltipContentProps<number, string>> & {
  formatter: (value: number) => string;
};

export function ChartTooltip({
  active,
  payload,
  label,
  formatter,
}: ChartTooltipProps) {
  if (!active || !payload || payload.length === 0) return null;
  return (
    <div className="rounded-lg border bg-popover px-3 py-2 text-xs shadow-md">
      <p className="mb-1.5 font-medium text-foreground">{label}</p>
      <div className="space-y-1">
        {payload.map((entry, i) => (
          <p key={i} className="flex items-center gap-1.5 text-muted-foreground">
            <span className="size-2 shrink-0 rounded-full" style={{ background: entry.color }} aria-hidden />
            <span>{entry.name}</span>
            <span className="ml-auto pl-3 font-medium text-foreground">
              {typeof entry.value === "number" ? formatter(entry.value) : entry.value}
            </span>
          </p>
        ))}
      </div>
    </div>
  );
}
