"use client";

import { Card, CardContent } from "@/components/ui/card";
import { cn } from "@/lib/utils";
import type { LucideIcon } from "lucide-react";

interface StatCardProps {
  title: string;
  value: string;
  sub?: string;
  icon: LucideIcon;
  live?: boolean;
  trend?: { value: string; up?: boolean };
  /** Classe additionnelle pour la valeur (ex. couleur conditionnelle). */
  valueClassName?: string;
  className?: string;
}

export function StatCard({ title, value, sub, icon: Icon, live, trend, valueClassName, className }: StatCardProps) {
  return (
    <Card className={cn("relative overflow-hidden transition-colors duration-300 hover:border-primary/35", className)}>
      {/* Halo aurora en coin — signature MikCloud */}
      <span
        aria-hidden
        className="pointer-events-none absolute -right-8 -top-8 size-28 rounded-full bg-primary/10 blur-2xl"
      />
      <CardContent className="relative p-4 sm:p-5">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <p className="text-xs font-medium uppercase tracking-wide text-muted-foreground">{title}</p>
            <p className={cn("mt-1.5 truncate text-2xl font-semibold tracking-tight", valueClassName)}>{value}</p>
            {sub && <p className="mt-1 text-xs text-muted-foreground">{sub}</p>}
          </div>
          <div className="flex shrink-0 items-center gap-2">
            {trend && (
              <span
                className={cn(
                  "rounded-full px-1.5 py-0.5 text-[10px] font-semibold",
                  trend.up ? "bg-primary/15 text-primary" : "bg-destructive/15 text-destructive",
                )}
              >
                {trend.up ? "▲" : "▼"} {trend.value}
              </span>
            )}
            <div className="tile-aurora flex size-9 items-center justify-center rounded-lg">
              <Icon className="size-4.5" />
            </div>
          </div>
        </div>
        {live && (
          <span className="live-dot absolute right-0 top-0 m-3 block size-2 rounded-full bg-primary" aria-hidden />
        )}
      </CardContent>
    </Card>
  );
}
