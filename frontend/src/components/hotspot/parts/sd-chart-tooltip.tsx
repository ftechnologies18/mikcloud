"use client";

// Tooltip sombre partagé pour les graphiques recharts (vues Rapports).

import type { TooltipProps } from "recharts";

interface Entry {
  dataKey?: string | number;
  name?: string | number;
  value?: number | string;
  color?: string;
}

export function ChartTooltip({
  active,
  payload,
  label,
  formatter,
}: TooltipProps<number, string> & { formatter: (value: number) => string }) {
  if (!active || !payload || payload.length === 0) return null;
  const entries = payload as unknown as Entry[];
  return (
    <div className="rounded-lg border bg-popover px-3 py-2 text-xs shadow-md">
      <p className="mb-1.5 font-medium text-foreground">{label}</p>
      <div className="space-y-1">
        {entries.map((entry, i) => (
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
