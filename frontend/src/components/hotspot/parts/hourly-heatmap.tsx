"use client";

// N°10 — Heatmap d'affluence par tranche horaire (jour × heure).
// Données RÉELLES agrégées côté backend depuis les UserLogs (connexions) —
// plus aucune courbe synthétique. Rendu CSS Grid pur (pas de lib de charts) :
// 24 colonnes d'heures, une ligne par jour, intensité = nombre de connexions.
// Bascule « Connexions / Ventes » : barres horaires agrégées sur la fenêtre.

import { useMemo, useState } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { api } from "@/lib/hotspot/api";
import { localeOf, useI18n } from "@/lib/hotspot/i18n";
import { formatCurrency } from "@/lib/hotspot/format";
import type { HourlyStats } from "@/lib/hotspot/types";
import { useQuery } from "@tanstack/react-query";

type Metric = "logins" | "sales";

// Échelle d'intensité émeraude (0 → max) — même teinte que les graphiques.
function cellStyle(value: number, max: number): string {
  if (value <= 0 || max <= 0) return "bg-muted/60";
  const r = value / max; // 0..1
  if (r > 0.85) return "bg-emerald-500";
  if (r > 0.65) return "bg-emerald-600/90";
  if (r > 0.45) return "bg-emerald-500/80";
  if (r > 0.3) return "bg-emerald-500/60";
  if (r > 0.18) return "bg-emerald-500/45";
  if (r > 0.08) return "bg-emerald-500/30";
  return "bg-emerald-500/20";
}

export function HourlyHeatmap({
  days = 7,
  currency = "FCFA",
}: {
  days?: 7 | 14 | 30;
  currency?: string;
}) {
  const { t, tf, lang } = useI18n();
  const [metric, setMetric] = useState<Metric>("logins");

  const { data, isLoading } = useQuery({
    queryKey: ["/api/stats/hourly", days],
    queryFn: () => api<HourlyStats>("/api/stats/hourly", { params: { days } }),
    refetchInterval: 60_000,
  });

  const dayLabels = useMemo(() => {
    const fmt = new Intl.DateTimeFormat(localeOf(lang), { weekday: "short", day: "2-digit", month: "2-digit" });
    const today = new Date().toISOString().slice(0, 10);
    const yesterday = new Date(Date.now() - 86_400_000).toISOString().slice(0, 10);
    return (d: string) => {
      const date = new Date(d + "T12:00:00");
      const base = fmt.format(date);
      if (d === today) return t("hourly.today") + " · " + base;
      if (d === yesterday) return t("hourly.yesterday") + " · " + base;
      return base;
    };
  }, [lang, t]);

  if (isLoading || !data) {
    return (
      <Card>
        <CardHeader className="px-4 sm:px-6">
          <CardTitle className="text-base">{t("hourly.title")}</CardTitle>
        </CardHeader>
        <CardContent className="px-4 pb-6 sm:px-6">
          <Skeleton className="h-56 w-full" />
        </CardContent>
      </Card>
    );
  }

  const max = Math.max(1, data.maxCell);
  const hours = data.loginsByHour;
  const sales = data.salesByHour;
  const maxBar = Math.max(1, ...(metric === "logins" ? hours : sales));

  return (
    <Card>
      <CardHeader className="flex-row items-center justify-between gap-3 px-4 sm:px-6">
        <div className="min-w-0">
          <CardTitle className="text-base">{t("hourly.title")}</CardTitle>
          <p className="mt-1 text-xs text-muted-foreground">
            {metric === "logins"
              ? tf("hourly.subtitleLogins", { n: data.totalLogins, d: String(data.days) })
              : tf("hourly.subtitleSales", { n: data.totalSales, d: String(data.days) })}
            {" · "}
            {tf("hourly.peak", { h: String(data.peakHour).padStart(2, "0") })}
          </p>
        </div>
        <div className="flex shrink-0 rounded-lg border bg-muted/40 p-0.5" role="tablist" aria-label={t("hourly.metricLabel")}>
          {(["logins", "sales"] as Metric[]).map((m) => (
            <button
              key={m}
              type="button"
              role="tab"
              aria-selected={metric === m}
              onClick={() => setMetric(m)}
              className={`rounded-md px-3 py-1.5 text-xs font-medium transition-colors ${
                metric === m ? "bg-primary text-primary-foreground" : "text-muted-foreground hover:text-foreground"
              }`}
            >
              {t(m === "logins" ? "hourly.metricLogins" : "hourly.metricSales")}
            </button>
          ))}
        </div>
      </CardHeader>
      <CardContent className="space-y-4 px-4 pb-6 sm:px-6">
        {/* Heatmap jour × heure */}
        <div className="space-y-1">
          {data.rows.map((row) => (
            <div key={row.date} className="flex items-center gap-2">
              <span className="w-28 shrink-0 truncate text-[11px] text-muted-foreground" title={dayLabels(row.date)}>
                {dayLabels(row.date)}
              </span>
              <div className="grid flex-1 grid-cols-24 gap-[3px]" role="img" aria-label={dayLabels(row.date)}>
                {row.hours.map((v, h) => (
                  <div
                    key={h}
                    className={`h-5 rounded-[3px] ${cellStyle(v, max)} transition-transform hover:scale-110 hover:ring-1 hover:ring-emerald-300`}
                    title={tf("hourly.cellTip", {
                      d: dayLabels(row.date),
                      h: String(h).padStart(2, "0"),
                      n: String(v),
                      p: v > 1 ? "s" : "",
                    })}
                  />
                ))}
              </div>
            </div>
          ))}
          {/* Axe des heures (0h, 6h, 12h, 18h, 23h) */}
          <div className="flex items-center gap-2 pt-0.5">
            <span className="w-28 shrink-0" />
            <div className="grid flex-1 grid-cols-24 gap-[3px] text-[10px] text-muted-foreground">
              {[0, 6, 12, 18, 23].map((h) => (
                <span key={h} className="truncate" style={{ gridColumn: `${h + 1} / span 1` }}>
                  {h}h
                </span>
              ))}
            </div>
          </div>
        </div>

        {/* Agrégat horaire (barres CSS) de la métrique choisie */}
        <div>
          <p className="mb-2 text-xs font-medium text-muted-foreground">{t("hourly.byHour")}</p>
          <div className="flex h-20 items-end gap-[3px]" role="img" aria-label={t("hourly.byHour")}>
            {(metric === "logins" ? hours : sales).map((v, h) => {
              const pct = Math.round((v / maxBar) * 100);
              const isPeak = metric === "logins" && h === data.peakHour && v > 0;
              return (
                <div
                  key={h}
                  className="group relative flex-1"
                  title={`${String(h).padStart(2, "0")}:00 — ${
                    metric === "logins" ? tf("hourly.loginsUnit", { n: String(v), p: v > 1 ? "s" : "" }) : formatCurrency(v, currency, lang)
                  }`}
                >
                  <div
                    className={`w-full rounded-t-[3px] transition-all group-hover:opacity-80 ${
                      metric === "logins" ? "bg-emerald-500" : "bg-primary"
                    } ${isPeak ? "ring-1 ring-emerald-300" : ""}`}
                    style={{ height: `${Math.max(v > 0 ? 4 : 1, pct)}%`, minHeight: v > 0 ? 4 : 1 }}
                  />
                </div>
              );
            })}
          </div>
          <div className="mt-1 flex gap-[3px] text-[10px] text-muted-foreground">
            {[0, 6, 12, 18, 23].map((h) => (
              <span key={h} className="flex-1 truncate">
                {h}h
              </span>
            ))}
          </div>
        </div>

        {/* Légende intensité */}
        <div className="flex flex-wrap items-center justify-between gap-2 text-xs text-muted-foreground">
          <span>{t("hourly.legend")}</span>
          <span className="flex items-center gap-1">
            <span className="inline-block size-3 rounded-[3px] bg-muted/60" /> 0
            <span className="ml-1.5 inline-block size-3 rounded-[3px] bg-emerald-500/20" />
            <span className="mx-1 inline-block size-3 rounded-[3px] bg-emerald-500/45" />
            <span className="mx-1 inline-block size-3 rounded-[3px] bg-emerald-500/80" />
            <span className="ml-1 inline-block size-3 rounded-[3px] bg-emerald-500" /> {max}
          </span>
        </div>
      </CardContent>
    </Card>
  );
}
