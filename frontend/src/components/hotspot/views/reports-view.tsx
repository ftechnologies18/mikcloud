"use client";

// Vue Rapports v2 — Comptabilité multi-sites (ventes par jour/semaine/mois et
// par routeur) + onglet Activité (performance commerciale et trafic réseau) +
// onglet Marge (F13 : prix de vente vs coût, 30 jours glissants).
//
// v2 — KPI enrichis (logique métier mikCloud) :
//  - Δ% vs période précédente sur chaque KPI (tendance visible d'un coup d'œil) ;
//  - marge en KPI de l'onglet Comptabilité (déjà calculée côté serveur, jamais affichée) ;
//  - ventes DIRECTES vs RÉSEAU REVENDEURS (canal de distribution — « qui vend mes tickets ? ») ;
//  - taux de marge par SITE (multi-sites : quel routeur est le plus rentable ?) ;
//  - sessions RÉELLES de la fenêtre (comptage + trafic cumulé) ;
//  - TOP 5 revendeurs par CA (leaderboard du réseau de distribution) ;
//  - HEURES DE POINTE (CA + connexions par heure, /api/stats/hourly, fuseau du compte) ;
//  - évolution QUOTIDIENNE de la marge (vert/rouge) + marge par site + part de marge.

import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  Bar,
  BarChart,
  CartesianGrid,
  Cell,
  ComposedChart,
  Legend,
  Line,
  LineChart,
  ReferenceLine,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import {
  CalendarRange,
  Clock3,
  Coins,
  Download,
  Percent,
  Router as RouterIcon,
  ShoppingCart,
  Store,
  TrendingUp,
  Users,
  Wallet,
  Wifi,
} from "lucide-react";
import { toast } from "sonner";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";

import { api, apiDownload } from "@/lib/hotspot/api";
import { localeOf, useI18n } from "@/lib/hotspot/i18n";
import { useChartPalette, type ChartPalette } from "@/lib/hotspot/chart-theme";
import type { Lang } from "@/lib/hotspot/i18n";
import type {
  AccountingData,
  AccountingPeriod,
  HourlyStats,
  ReportsData,
  RouterDevice,
} from "@/lib/hotspot/types";
import { formatBytes, formatCurrency } from "@/lib/hotspot/format";
import { EmptyState } from "@/components/hotspot/empty-state";
import { LoadingCards } from "@/components/hotspot/loading";
import { PageHeader } from "@/components/hotspot/page-header";
import { StatCard } from "@/components/hotspot/stat-card";
import { ChartTooltip } from "@/components/hotspot/parts/sd-chart-tooltip";
import { useCurrency } from "@/components/hotspot/parts/sd-currency";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";

const PERIODS = [
  { value: "7", labelKey: "reports.days7" },
  { value: "14", labelKey: "reports.days14" },
  { value: "30", labelKey: "reports.days30" },
];

const ACCOUNTING_PERIODS: {
  value: AccountingPeriod;
  labelKey: string;
  windowKey: string;
  barsKey: string;
  unitKey: string;
}[] = [
  { value: "day", labelKey: "reports.period.day", windowKey: "reports.window.day", barsKey: "reports.bars.day", unitKey: "reports.unit.day" },
  { value: "week", labelKey: "reports.period.week", windowKey: "reports.window.week", barsKey: "reports.bars.week", unitKey: "reports.unit.week" },
  { value: "month", labelKey: "reports.period.month", windowKey: "reports.window.month", barsKey: "reports.bars.month", unitKey: "reports.unit.month" },
];

// Palette thématée (nuit/jour) injectée dans chaque onglet à graphiques.
const voucherStatusRows = (p: ChartPalette) => [
  { key: "active", labelKey: "common.statusActive", color: p.series[0] },
  { key: "used", labelKey: "common.statusUsed", color: p.series[2] },
  { key: "expired", labelKey: "common.statusExpired", color: p.axis },
  { key: "disabled", labelKey: "common.statusDisabled", color: p.series[3] },
] as const;

/** Couleur des barres de la courbe de marge : vert si positive, rouge sinon. */
const MARGIN_POS = "#10b981";
const MARGIN_NEG = "#ef4444";

/** Pourcentage localisé (12,4 % en FR, 12.4% en EN). */
function fmtPct(value: number, lang: Lang): string {
  return `${value.toFixed(1).replace(".", lang === "fr" ? "," : ".")}${lang === "fr" ? " " : ""}%`;
}

/** Δ% vs période précédente → badge de tendance StatCard (rien si pas de base). */
function deltaTrend(
  current: number,
  previous: number | undefined,
  lang: Lang,
): { value: string; up: boolean } | undefined {
  if (previous === undefined || previous <= 0) return undefined;
  const pct = ((current - previous) / previous) * 100;
  if (Math.abs(pct) < 0.05) return undefined;
  return { value: fmtPct(Math.abs(pct), lang), up: pct > 0 };
}

/** Badge de taux de marge : vert positif, rouge négatif, neutre à zéro. */
function RateBadge({ rate }: { rate: number }) {
  return (
    <Badge
      variant="outline"
      className={
        rate > 0
          ? "border-emerald-500/25 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400"
          : rate < 0
            ? "border-destructive/25 bg-destructive/10 text-destructive"
            : "border-border bg-muted text-muted-foreground"
      }
    >
      {rate.toFixed(1).replace(".", ",")} %
    </Badge>
  );
}

/** Couleur de la marge : verte si positive, rouge si négative, neutre sinon. */
function cnMargin(margin: number): string {
  if (margin > 0) return "font-semibold text-emerald-600 dark:text-emerald-400";
  if (margin < 0) return "font-semibold text-destructive";
  return "font-semibold text-muted-foreground";
}

// Tooltip comptabilité : revenus + ventes du point survolé.
function AccountingTooltip({
  active,
  payload,
  label,
  currency,
  lang,
  revenueLabel,
  salesLabel,
}: {
  active?: boolean;
  payload?: { payload?: { revenue: number; sales: number } }[];
  label?: string;
  currency: string;
  lang: Lang;
  revenueLabel: string;
  salesLabel: string;
}) {
  if (!active || !payload || payload.length === 0 || !payload[0]?.payload) return null;
  const point = payload[0].payload;
  return (
    <div className="rounded-lg border bg-popover px-3 py-2 text-xs shadow-md">
      <p className="mb-1.5 font-medium text-foreground">{label}</p>
      <p className="text-muted-foreground">
        {revenueLabel}{" "}
        <span className="font-medium text-foreground">{formatCurrency(point.revenue, currency, lang)}</span>
      </p>
      <p className="text-muted-foreground">
        {salesLabel} <span className="font-medium text-foreground">{point.sales}</span>
      </p>
    </div>
  );
}

// Tooltip heures de pointe : CA + connexions de la tranche survolée.
function PeakHoursTooltip({
  active,
  payload,
  label,
  currency,
  lang,
  revenueLabel,
  loginsLabel,
}: {
  active?: boolean;
  payload?: { dataKey?: string | number; value?: number }[];
  label?: string;
  currency: string;
  lang: Lang;
  revenueLabel: string;
  loginsLabel: string;
}) {
  if (!active || !payload || payload.length === 0) return null;
  const revenue = payload.find((p) => p.dataKey === "revenue")?.value ?? 0;
  const logins = payload.find((p) => p.dataKey === "logins")?.value ?? 0;
  return (
    <div className="rounded-lg border bg-popover px-3 py-2 text-xs shadow-md">
      <p className="mb-1.5 font-medium text-foreground">{label}</p>
      <p className="text-muted-foreground">
        {revenueLabel}{" "}
        <span className="font-medium text-foreground">{formatCurrency(revenue, currency, lang)}</span>
      </p>
      <p className="text-muted-foreground">
        {loginsLabel} <span className="font-medium text-foreground">{logins}</span>
      </p>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Onglet Comptabilité — ventes par jour/semaine/mois, filtrables par routeur.
// v2 : marge en KPI, Δ% vs période précédente, canal direct/revendeurs,
// taux de marge par site, pic de CA de la fenêtre.
// ---------------------------------------------------------------------------

function AccountingTab({ visible }: { visible: boolean }) {
  const { t, tf, lang } = useI18n();
  const currency = useCurrency();
  const charts = useChartPalette();
  const AXIS_TICK = { fontSize: 11, fill: charts.axis };
  const [period, setPeriod] = useState<AccountingPeriod>("day");
  const [routerFilter, setRouterFilter] = useState("all");

  const { data: routers } = useQuery({
    queryKey: ["/api/routers"],
    queryFn: () => api<RouterDevice[]>("/api/routers"),
  });

  const { data, isLoading } = useQuery({
    queryKey: ["/api/accounting", period, routerFilter],
    queryFn: () =>
      api<AccountingData>("/api/accounting", { params: { period, routerId: routerFilter } }),
    placeholderData: (previous) => previous,
    enabled: visible,
  });

  const periodMeta = ACCOUNTING_PERIODS.find((p) => p.value === period) ?? ACCOUNTING_PERIODS[0];
  const selectedRouter = routers?.find((r) => r.id === routerFilter);
  const filterLabel = selectedRouter ? `${selectedRouter.name} · ` : "";
  const byRouter = data?.byRouter ?? [];
  const maxShare = Math.max(...byRouter.map((r) => r.share), 1);
  const margin = data?.totals.margin;
  const channel = data?.channel;
  const channelTotal = channel ? channel.directRevenue + channel.resellerRevenue : 0;
  // Pic de CA de la fenêtre (meilleur bucket de la série affichée).
  const bestBucket = useMemo(
    () => (data?.series ?? []).reduce<{ label: string; revenue: number }>(
      (best, pt) => (pt.revenue > best.revenue ? { label: pt.label, revenue: pt.revenue } : best),
      { label: "", revenue: 0 },
    ),
    [data],
  );

  return (
    <div className="space-y-4 sm:space-y-6">
      {/* Période + filtre routeur */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <Tabs value={period} onValueChange={(value) => setPeriod(value as AccountingPeriod)}>
          <TabsList>
            {ACCOUNTING_PERIODS.map((p) => (
              <TabsTrigger key={p.value} value={p.value}>
                {t(p.labelKey)}
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>
        <div className="flex items-center gap-2">
          <RouterIcon className="size-4 text-muted-foreground" aria-hidden />
          <Select value={routerFilter} onValueChange={setRouterFilter}>
            <SelectTrigger className="h-10 w-full sm:w-56" aria-label={t("reports.filterRouter")}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">{t("common.allSites")}</SelectItem>
              {routers?.map((router) => (
                <SelectItem key={router.id} value={router.id}>
                  {router.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Button
            variant="outline"
            className="h-10"
            onClick={() =>
              apiDownload(`/api/accounting/export`, `mikcloud-comptabilite-${period}.csv`, {
                period,
                routerId: routerFilter,
              })
                .then(() => toast.success(t("common.exportDownloaded")))
                .catch((err: Error) => toast.error(err.message))
            }
          >
            <Download className="size-4" />
            <span className="hidden sm:inline">{t("common.exportCsv")}</span>
          </Button>
        </div>
      </div>

      {isLoading && !data ? (
        <div className="space-y-4 sm:space-y-6">
          <LoadingCards cards={4} />
          <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
            <Skeleton className="h-80 rounded-xl" />
            <Skeleton className="h-80 rounded-xl" />
          </div>
        </div>
      ) : !data ? null : (
        <>
          {/* KPI : revenus / ventes / marge / panier moyen — Δ% vs fenêtre précédente */}
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
            <StatCard
              title={t("reports.revenue")}
              value={formatCurrency(data.totals.revenue, currency, lang)}
              sub={`${filterLabel}${t(periodMeta.windowKey)}`}
              icon={Wallet}
              trend={deltaTrend(data.totals.revenue, data.prev?.revenue, lang)}
            />
            <StatCard
              title={t("reports.sales")}
              value={String(data.totals.sales)}
              sub={`${t("reports.vouchersSold")} · ${t(periodMeta.barsKey)}`}
              icon={ShoppingCart}
              trend={deltaTrend(data.totals.sales, data.prev?.sales, lang)}
            />
            {margin !== undefined && (
              <StatCard
                title={t("reports.margin.margin")}
                value={formatCurrency(margin, currency, lang)}
                sub={`${t("reports.margin.rate")} : ${data.totals.selling ? fmtPct((margin / data.totals.selling) * 100, lang) : fmtPct(0, lang)}`}
                icon={Coins}
                valueClassName={cnMargin(margin)}
                trend={deltaTrend(margin, data.prev?.margin, lang)}
              />
            )}
            <StatCard
              title={t("reports.avgTicket")}
              value={formatCurrency(data.totals.avgTicket, currency, lang)}
              sub={t("reports.perVoucher")}
              icon={TrendingUp}
              trend={deltaTrend(data.totals.avgTicket, data.prev?.avgTicket, lang)}
            />
          </div>

          <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
            {/* Chiffre d'affaires par bucket */}
            <Card className="gap-4 py-4 sm:py-6">
              <CardHeader className="px-4 sm:px-6">
                <CardTitle className="text-base">
                  {tf("reports.revenueBy", { unit: t(periodMeta.unitKey) })}
                </CardTitle>
                <CardDescription>
                  {selectedRouter ? `${selectedRouter.name} — ` : `${t("reports.allSites")} — `}
                  {t(periodMeta.windowKey)}
                  {bestBucket.revenue > 0 && (
                    <>
                      {" · "}
                      {tf("reports.bestPeriod", {
                        label: bestBucket.label,
                        amount: formatCurrency(bestBucket.revenue, currency, lang),
                      })}
                    </>
                  )}
                </CardDescription>
              </CardHeader>
              <CardContent className="px-4 sm:px-6">
                <ResponsiveContainer width="100%" height={280}>
                  <BarChart data={data.series} margin={{ top: 8, right: 8, left: 0, bottom: 0 }}>
                    <CartesianGrid stroke={charts.grid} strokeOpacity={0.6} vertical={false} />
                    <XAxis
                      dataKey="label"
                      tick={AXIS_TICK}
                      axisLine={false}
                      tickLine={false}
                      minTickGap={12}
                    />
                    <YAxis
                      tick={AXIS_TICK}
                      axisLine={false}
                      tickLine={false}
                      width={48}
                      tickFormatter={(value: number) =>
                        new Intl.NumberFormat(localeOf(lang), { notation: "compact", maximumFractionDigits: 1 }).format(value)
                      }
                    />
                    <Tooltip
                      cursor={{ fill: charts.cursorFill, fillOpacity: 0.06 }}
                      content={
                        <AccountingTooltip
                          currency={currency}
                          lang={lang}
                          revenueLabel={t("reports.tooltipRevenue")}
                          salesLabel={t("reports.tooltipSales")}
                        />
                      }
                    />
                    <Bar dataKey="revenue" name={t("reports.revenue")} fill={charts.series[0]} radius={[4, 4, 0, 0]} maxBarSize={40} />
                  </BarChart>
                </ResponsiveContainer>
              </CardContent>
            </Card>

            {/* Répartition par routeur — avec taux de marge par site */}
            <Card className="gap-4 py-4 sm:py-6">
              <CardHeader className="px-4 sm:px-6">
                <CardTitle className="text-base">{t("reports.salesBySite")}</CardTitle>
                <CardDescription>{t("reports.salesBySiteDesc")}</CardDescription>
              </CardHeader>
              <CardContent className="px-4 sm:px-6">
                {byRouter.length === 0 ? (
                  <EmptyState
                    icon={CalendarRange}
                    title={t("reports.noSales")}
                    description={t("reports.noSalesDesc")}
                  />
                ) : (
                  <div className="max-h-72 space-y-5 overflow-y-auto pr-1">
                    {byRouter.map((router) => {
                      const siteMargin =
                        router.selling !== undefined && router.cost !== undefined
                          ? router.selling - router.cost
                          : undefined;
                      const siteRate =
                        siteMargin !== undefined && router.selling ? (siteMargin / router.selling) * 100 : null;
                      return (
                        <div key={router.routerId}>
                          <div className="flex items-baseline justify-between gap-3 text-sm">
                            <span className="flex min-w-0 items-center gap-2">
                              <RouterIcon className="size-3.5 shrink-0 text-muted-foreground" aria-hidden />
                              <span className="truncate font-medium">{router.routerName}</span>
                            </span>
                            <span className="shrink-0 text-xs text-muted-foreground">
                              <span className="font-medium text-foreground">
                                {formatCurrency(router.revenue, currency, lang)}
                              </span>{" "}
                              · {tf("reports.soldCount", { n: router.sales })} ·{" "}
                              {new Intl.NumberFormat(localeOf(lang), { maximumFractionDigits: 1 }).format(router.share)} %
                              {siteRate !== null && (
                                <>
                                  {" · "}
                                  <span className={cnMargin(siteMargin ?? 0)}>{fmtPct(siteRate, lang)}</span>
                                </>
                              )}
                            </span>
                          </div>
                          <div className="mt-1.5 h-2 overflow-hidden rounded-full bg-muted">
                            <div
                              className="h-full rounded-full bg-primary transition-all"
                              style={{ width: `${Math.max(2, (router.share / maxShare) * 100)}%` }}
                            />
                          </div>
                        </div>
                      );
                    })}
                  </div>
                )}
              </CardContent>
            </Card>
          </div>

          {/* Canal de distribution — ventes directes vs réseau revendeurs */}
          {channel && (
            <Card className="gap-4 py-4 sm:py-6">
              <CardHeader className="px-4 sm:px-6">
                <CardTitle className="text-base">{t("reports.channel.title")}</CardTitle>
                <CardDescription>
                  {t(routerFilter === "all" ? "reports.channel.desc" : "reports.channel.descSite")}
                </CardDescription>
              </CardHeader>
              <CardContent className="px-4 sm:px-6">
                {channelTotal === 0 ? (
                  <p className="py-4 text-center text-sm text-muted-foreground">{t("reports.channel.empty")}</p>
                ) : (
                  <div className="grid grid-cols-1 gap-6 sm:grid-cols-2">
                    {(
                      [
                        {
                          key: "direct",
                          icon: Store,
                          nameKey: "reports.channel.direct",
                          revenue: channel.directRevenue,
                          sales: channel.directSales,
                          color: charts.series[0],
                        },
                        {
                          key: "reseller",
                          icon: Users,
                          nameKey: "reports.channel.resellers",
                          revenue: channel.resellerRevenue,
                          sales: channel.resellerSales,
                          color: charts.series[1],
                        },
                      ] as const
                    ).map((row) => {
                      const share = (row.revenue / channelTotal) * 100;
                      return (
                        <div key={row.key}>
                          <div className="flex items-baseline justify-between gap-3 text-sm">
                            <span className="flex min-w-0 items-center gap-2">
                              <row.icon className="size-3.5 shrink-0 text-muted-foreground" aria-hidden />
                              <span className="truncate font-medium">{t(row.nameKey)}</span>
                            </span>
                            <span className="shrink-0 text-xs text-muted-foreground">
                              <span className="font-medium text-foreground">
                                {formatCurrency(row.revenue, currency, lang)}
                              </span>{" "}
                              · {tf("reports.soldCount", { n: row.sales })} ·{" "}
                              {new Intl.NumberFormat(localeOf(lang), { maximumFractionDigits: 1 }).format(share)} %
                            </span>
                          </div>
                          <div className="mt-1.5 h-2 overflow-hidden rounded-full bg-muted">
                            <div
                              className="h-full rounded-full transition-all"
                              style={{ width: `${Math.max(2, share)}%`, background: row.color }}
                            />
                          </div>
                        </div>
                      );
                    })}
                  </div>
                )}
              </CardContent>
            </Card>
          )}
        </>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Onglet Activité — performance commerciale et trafic réseau (7/14/30 jours).
// v2 : sessions réelles en KPI, Δ% vs période précédente, top revendeurs,
// heures de pointe (affluence + CA par heure, fuseau du compte).
// ---------------------------------------------------------------------------

function ActivityTab({ visible }: { visible: boolean }) {
  const { t, tf, lang } = useI18n();
  const currency = useCurrency();
  const charts = useChartPalette();
  const AXIS_TICK = { fontSize: 11, fill: charts.axis };
  const [days, setDays] = useState(14);

  const { data, isLoading } = useQuery({
    queryKey: ["/api/reports", days],
    queryFn: () => api<ReportsData>("/api/reports", { params: { days } }),
    placeholderData: (previous) => previous,
    enabled: visible,
  });

  // N°10 — affluence réelle par tranche horaire (même fenêtre que l'onglet).
  const { data: hourly } = useQuery({
    queryKey: ["/api/stats/hourly", days],
    queryFn: () => api<HourlyStats>("/api/stats/hourly", { params: { days } }),
    placeholderData: (previous) => previous,
    enabled: visible,
  });

  const salesByProfile = useMemo(
    () => [...(data?.salesByProfile ?? [])].sort((a, b) => b.revenue - a.revenue),
    [data],
  );
  const maxProfileRevenue = Math.max(...salesByProfile.map((s) => s.revenue), 1);
  const maxStatus = useMemo(
    () =>
      data
        ? Math.max(...voucherStatusRows(charts).map((row) => data.voucherStatus[row.key]), 1)
        : 1,
    [data, charts],
  );
  const topResellers = data?.topResellers ?? [];
  const maxResellerRevenue = Math.max(...topResellers.map((r) => r.revenue), 1);
  const sessions = data?.sessions;
  const sessionTraffic = sessions ? sessions.bytesIn + sessions.bytesOut : 0;
  const hourlyData = useMemo(
    () =>
      hourly
        ? hourly.salesByHour.map((revenue, i) => ({
            hour: `${String(i).padStart(2, "0")}h`,
            revenue,
            logins: hourly.loginsByHour[i] ?? 0,
          }))
        : [],
    [hourly],
  );
  const hasHourlyActivity = hourly ? hourly.totalLogins > 0 || hourly.totalSales > 0 : false;

  return (
    <div className="space-y-4 sm:space-y-6">
      <div className="flex justify-end">
        <Tabs value={String(days)} onValueChange={(value) => setDays(Number(value))}>
          <TabsList>
            {PERIODS.map((period) => (
              <TabsTrigger key={period.value} value={period.value}>
                {t(period.labelKey)}
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>
      </div>

      {isLoading && !data ? (
        <div className="space-y-4 sm:space-y-6">
          <LoadingCards cards={4} />
          <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
            <Skeleton className="h-80 rounded-xl" />
            <Skeleton className="h-80 rounded-xl" />
            <Skeleton className="h-64 rounded-xl" />
            <Skeleton className="h-64 rounded-xl" />
          </div>
        </div>
      ) : !data ? null : (
        <>
          {/* KPI : revenus / ventes / panier moyen / sessions réelles — Δ% vs fenêtre précédente */}
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
            <StatCard
              title={t("reports.revenue")}
              value={formatCurrency(data.totals.revenue, currency, lang)}
              sub={tf("reports.lastDays", { n: days })}
              icon={Wallet}
              trend={deltaTrend(data.totals.revenue, data.prev?.revenue, lang)}
            />
            <StatCard
              title={t("reports.sales")}
              value={String(data.totals.sales)}
              sub={t("reports.vouchersSold")}
              icon={ShoppingCart}
              trend={deltaTrend(data.totals.sales, data.prev?.sales, lang)}
            />
            <StatCard
              title={t("reports.avgTicket")}
              value={formatCurrency(data.totals.avgTicket, currency, lang)}
              sub={t("reports.perVoucher")}
              icon={TrendingUp}
              trend={deltaTrend(data.totals.avgTicket, data.prev?.avgTicket, lang)}
            />
            {sessions && (
              <StatCard
                title={t("reports.sessions")}
                value={new Intl.NumberFormat(localeOf(lang)).format(sessions.count)}
                sub={tf("reports.sessions.sub", { n: days, bytes: formatBytes(sessionTraffic, lang) })}
                icon={Wifi}
                live
              />
            )}
          </div>

          <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
            <Card className="gap-4 py-4 sm:py-6">
              <CardHeader className="px-4 sm:px-6">
                <CardTitle className="text-base">{t("reports.revenueTitle")}</CardTitle>
                <CardDescription>{t("reports.revenueDaily")}</CardDescription>
              </CardHeader>
              <CardContent className="px-4 sm:px-6">
                <ResponsiveContainer width="100%" height={260}>
                  <BarChart data={data.revenueByDay} margin={{ top: 8, right: 8, left: 0, bottom: 0 }}>
                    <CartesianGrid stroke={charts.grid} strokeOpacity={0.6} vertical={false} />
                    <XAxis
                      dataKey="day"
                      tick={AXIS_TICK}
                      axisLine={false}
                      tickLine={false}
                      minTickGap={12}
                    />
                    <YAxis
                      tick={AXIS_TICK}
                      axisLine={false}
                      tickLine={false}
                      width={48}
                      tickFormatter={(value: number) =>
                        new Intl.NumberFormat(localeOf(lang), { notation: "compact", maximumFractionDigits: 1 }).format(value)
                      }
                    />
                    <Tooltip
                      cursor={{ fill: charts.cursorFill, fillOpacity: 0.06 }}
                      content={<ChartTooltip formatter={(value) => formatCurrency(value, currency, lang)} />}
                    />
                    <Bar dataKey="value" name={t("reports.revenue")} fill={charts.series[0]} radius={[4, 4, 0, 0]} maxBarSize={40} />
                  </BarChart>
                </ResponsiveContainer>
              </CardContent>
            </Card>

            <Card className="gap-4 py-4 sm:py-6">
              <CardHeader className="px-4 sm:px-6">
                <CardTitle className="text-base">{t("reports.networkTraffic")}</CardTitle>
                <CardDescription>{t("reports.networkTrafficDesc")}</CardDescription>
              </CardHeader>
              <CardContent className="px-4 sm:px-6">
                <ResponsiveContainer width="100%" height={260}>
                  <LineChart data={data.trafficByDay} margin={{ top: 8, right: 8, left: 0, bottom: 0 }}>
                    <CartesianGrid stroke={charts.grid} strokeOpacity={0.6} vertical={false} />
                    <XAxis
                      dataKey="day"
                      tick={AXIS_TICK}
                      axisLine={false}
                      tickLine={false}
                      minTickGap={12}
                    />
                    <YAxis
                      tick={AXIS_TICK}
                      axisLine={false}
                      tickLine={false}
                      width={56}
                      tickFormatter={(value: number) => formatBytes(value, lang)}
                    />
                    <Tooltip
                      cursor={{ stroke: "#3f3f46", strokeDasharray: "4 4" }}
                      content={<ChartTooltip formatter={(value) => formatBytes(value, lang)} />}
                    />
                    <Legend
                      iconType="circle"
                      iconSize={8}
                      formatter={(value) => <span className="text-xs text-muted-foreground">{value}</span>}
                    />
                    <Line
                      dataKey="bytesIn"
                      name={t("reports.inbound")}
                      type="monotone"
                      stroke={charts.series[0]}
                      strokeWidth={2}
                      dot={false}
                      activeDot={{ r: 4 }}
                    />
                    <Line
                      dataKey="bytesOut"
                      name={t("reports.outbound")}
                      type="monotone"
                      stroke={charts.series[1]}
                      strokeWidth={2}
                      dot={false}
                      activeDot={{ r: 4 }}
                    />
                  </LineChart>
                </ResponsiveContainer>
              </CardContent>
            </Card>
          </div>

          <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
            {/* Top 5 revendeurs par CA — moteur de distribution mikCloud */}
            <Card className="gap-4 py-4 sm:py-6">
              <CardHeader className="px-4 sm:px-6">
                <CardTitle className="text-base">{t("reports.topResellers.title")}</CardTitle>
                <CardDescription>{t("reports.topResellers.desc")}</CardDescription>
              </CardHeader>
              <CardContent className="px-4 sm:px-6">
                {topResellers.length === 0 ? (
                  <EmptyState
                    icon={Users}
                    title={t("reports.topResellers.empty")}
                    description={t("reports.salesByProfileDesc")}
                  />
                ) : (
                  <div className="space-y-4">
                    {topResellers.map((reseller, index) => (
                      <div key={reseller.name} className="flex items-center gap-3">
                        <Badge
                          variant="outline"
                          className={
                            index === 0
                              ? "shrink-0 border-primary/30 bg-primary/10 text-primary"
                              : "shrink-0 border-border bg-muted text-muted-foreground"
                          }
                        >
                          {tf("reports.rank", { n: index + 1 })}
                        </Badge>
                        <div className="min-w-0 flex-1">
                          <div className="flex items-baseline justify-between gap-3 text-sm">
                            <span className="truncate font-medium">{reseller.name}</span>
                            <span className="shrink-0 text-xs text-muted-foreground">
                              <span className="font-medium text-foreground">
                                {formatCurrency(reseller.revenue, currency, lang)}
                              </span>{" "}
                              · {tf("reports.soldCount", { n: reseller.sales })}
                            </span>
                          </div>
                          <div className="mt-1.5 h-2 overflow-hidden rounded-full bg-muted">
                            <div
                              className="h-full rounded-full bg-primary"
                              style={{ width: `${Math.max(2, (reseller.revenue / maxResellerRevenue) * 100)}%` }}
                            />
                          </div>
                        </div>
                      </div>
                    ))}
                  </div>
                )}
              </CardContent>
            </Card>

            {/* Heures de pointe — CA + connexions par heure (fuseau du compte) */}
            <Card className="gap-4 py-4 sm:py-6">
              <CardHeader className="px-4 sm:px-6">
                <CardTitle className="flex items-center gap-2 text-base">
                  <Clock3 className="size-4 text-muted-foreground" aria-hidden />
                  {t("reports.peakHours.title")}
                </CardTitle>
                <CardDescription>
                  {t("reports.peakHours.desc")}
                  {hourly && hasHourlyActivity && (
                    <>
                      {" · "}
                      {tf("reports.peakHours.peak", {
                        h: String(hourly.peakHour).padStart(2, "0"),
                      })}
                    </>
                  )}
                </CardDescription>
              </CardHeader>
              <CardContent className="px-4 sm:px-6">
                {!hourly ? (
                  <Skeleton className="h-60 rounded-lg" />
                ) : !hasHourlyActivity ? (
                  <p className="py-12 text-center text-sm text-muted-foreground">
                    {t("reports.peakHours.empty")}
                  </p>
                ) : (
                  <ResponsiveContainer width="100%" height={260}>
                    <ComposedChart data={hourlyData} margin={{ top: 8, right: 4, left: 0, bottom: 0 }}>
                      <CartesianGrid stroke={charts.grid} strokeOpacity={0.6} vertical={false} />
                      <XAxis
                        dataKey="hour"
                        tick={AXIS_TICK}
                        axisLine={false}
                        tickLine={false}
                        interval={2}
                      />
                      <YAxis
                        yAxisId="revenue"
                        tick={AXIS_TICK}
                        axisLine={false}
                        tickLine={false}
                        width={48}
                        tickFormatter={(value: number) =>
                          new Intl.NumberFormat(localeOf(lang), { notation: "compact", maximumFractionDigits: 1 }).format(value)
                        }
                      />
                      <YAxis
                        yAxisId="logins"
                        orientation="right"
                        tick={AXIS_TICK}
                        axisLine={false}
                        tickLine={false}
                        width={36}
                        tickFormatter={(value: number) =>
                          new Intl.NumberFormat(localeOf(lang), { notation: "compact", maximumFractionDigits: 1 }).format(value)
                        }
                      />
                      <Tooltip
                        cursor={{ fill: charts.cursorFill, fillOpacity: 0.06 }}
                        content={
                          <PeakHoursTooltip
                            currency={currency}
                            lang={lang}
                            revenueLabel={t("reports.peakHours.revenue")}
                            loginsLabel={t("reports.peakHours.logins")}
                          />
                        }
                      />
                      <Legend
                        iconType="circle"
                        iconSize={8}
                        formatter={(value) => <span className="text-xs text-muted-foreground">{value}</span>}
                      />
                      <Bar
                        yAxisId="revenue"
                        dataKey="revenue"
                        name={t("reports.peakHours.revenue")}
                        fill={charts.series[0]}
                        radius={[3, 3, 0, 0]}
                        maxBarSize={14}
                      />
                      <Line
                        yAxisId="logins"
                        dataKey="logins"
                        name={t("reports.peakHours.logins")}
                        type="monotone"
                        stroke={charts.series[1]}
                        strokeWidth={2}
                        dot={false}
                        activeDot={{ r: 4 }}
                      />
                    </ComposedChart>
                  </ResponsiveContainer>
                )}
              </CardContent>
            </Card>
          </div>

          <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
            <Card className="gap-4 py-4 sm:py-6">
              <CardHeader className="px-4 sm:px-6">
                <CardTitle className="text-base">{t("reports.salesByProfile")}</CardTitle>
                <CardDescription>{t("reports.salesByProfileDesc")}</CardDescription>
              </CardHeader>
              <CardContent className="px-4 sm:px-6">
                {salesByProfile.length === 0 ? (
                  <p className="py-6 text-center text-sm text-muted-foreground">{t("reports.noSalesPeriod")}</p>
                ) : (
                  <div className="space-y-4">
                    {salesByProfile.map((sale) => (
                      <div key={sale.name}>
                        <div className="flex items-baseline justify-between gap-3 text-sm">
                          <span className="truncate">{sale.name}</span>
                          <span className="shrink-0 text-xs text-muted-foreground">
                            <span className="font-medium text-foreground">
                              {tf("reports.soldCount", { n: sale.count })}
                            </span>
                            {" · "}
                            {formatCurrency(sale.revenue, currency, lang)}
                          </span>
                        </div>
                        <div className="mt-1.5 h-2 overflow-hidden rounded-full bg-muted">
                          <div
                            className="h-full rounded-full bg-primary"
                            style={{ width: `${Math.max(2, (sale.revenue / maxProfileRevenue) * 100)}%` }}
                          />
                        </div>
                      </div>
                    ))}
                  </div>
                )}
              </CardContent>
            </Card>

            <Card className="gap-4 py-4 sm:py-6">
              <CardHeader className="px-4 sm:px-6">
                <CardTitle className="text-base">{t("reports.voucherStatus")}</CardTitle>
                <CardDescription>{t("reports.voucherStatusDesc")}</CardDescription>
              </CardHeader>
              <CardContent className="px-4 sm:px-6">
                <div className="space-y-4">
                  {voucherStatusRows(charts).map((row) => {
                    const count = data.voucherStatus[row.key];
                    return (
                      <div key={row.key}>
                        <div className="flex items-center justify-between gap-3 text-sm">
                          <span className="flex items-center gap-2">
                            <span className="size-2 rounded-full" style={{ background: row.color }} aria-hidden />
                            {t(row.labelKey)}
                          </span>
                          <span className="font-medium tabular-nums">{count}</span>
                        </div>
                        <div className="mt-1.5 h-2 overflow-hidden rounded-full bg-muted">
                          <div
                            className="h-full rounded-full"
                            style={{
                              width: `${Math.max(2, (count / maxStatus) * 100)}%`,
                              background: row.color,
                            }}
                          />
                        </div>
                      </div>
                    );
                  })}
                </div>
              </CardContent>
            </Card>
          </div>
        </>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Onglet Marge (F13) — prix de vente vs coût sur les 30 derniers jours
// glissants. Bloc « margin » de GET /api/reports (backend P0 Task 17).
// v2 : Δ% vs 30 j précédents, évolution quotidienne, marge par site,
// part de marge par profil.
// ---------------------------------------------------------------------------

function MarginTab({ visible }: { visible: boolean }) {
  const { t, tf, lang } = useI18n();
  const currency = useCurrency();
  const charts = useChartPalette();
  const AXIS_TICK = { fontSize: 11, fill: charts.axis };

  const { data, isLoading } = useQuery({
    queryKey: ["/api/reports", 30],
    queryFn: () => api<ReportsData>("/api/reports", { params: { days: 30 } }),
    placeholderData: (previous) => previous,
    enabled: visible,
  });

  const margin = data?.margin;
  const pctFmt = (value: number): string => fmtPct(value, lang);

  if (isLoading && !data) {
    return (
      <div className="space-y-4 sm:space-y-6">
        <LoadingCards cards={4} />
        <Skeleton className="h-72 rounded-xl" />
      </div>
    );
  }

  // Backend non à jour (bloc « margin » absent) → EmptyState discret.
  if (!margin) {
    return (
      <Card className="gap-0 py-0">
        <EmptyState
          icon={Percent}
          title={t("reports.margin.unavailable")}
          description={t("reports.margin.unavailableDesc")}
        />
      </Card>
    );
  }

  const byProfile = [...(margin.byProfile ?? [])].sort((a, b) => b.margin - a.margin);
  const bySite = margin.byRouter ?? [];
  const byDay = margin.byDay ?? [];
  const prev = margin.prev;
  const maxSiteMargin = Math.max(...bySite.map((r) => Math.abs(r.margin)), 1);
  const bestProfile = byProfile[0];

  return (
    <div className="space-y-4 sm:space-y-6">
      {/* KPI : CA, coût, marge, taux de marge — Δ% vs 30 jours précédents */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <StatCard
          title={t("reports.margin.revenue")}
          value={formatCurrency(margin.revenue, currency, lang)}
          sub={t("reports.margin.window")}
          icon={Wallet}
          trend={deltaTrend(margin.revenue, prev?.revenue, lang)}
        />
        <StatCard
          title={t("reports.margin.cost")}
          value={formatCurrency(margin.cost, currency, lang)}
          sub={t("reports.margin.window")}
          icon={ShoppingCart}
        />
        <StatCard
          title={t("reports.margin.margin")}
          value={formatCurrency(margin.margin, currency, lang)}
          sub={t("reports.margin.window")}
          icon={Coins}
          valueClassName={cnMargin(margin.margin)}
          trend={deltaTrend(margin.margin, prev?.margin, lang)}
        />
        <StatCard
          title={t("reports.margin.rate")}
          value={pctFmt(margin.marginPct)}
          sub={t("reports.margin.window")}
          icon={Percent}
        />
      </div>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        {/* Évolution quotidienne de la marge (vert positif / rouge négatif) */}
        <Card className="gap-4 py-4 sm:py-6">
          <CardHeader className="px-4 sm:px-6">
            <CardTitle className="text-base">{t("reports.margin.trendTitle")}</CardTitle>
            <CardDescription>{t("reports.margin.trendDesc")}</CardDescription>
          </CardHeader>
          <CardContent className="px-4 sm:px-6">
            {byDay.length === 0 ? (
              <p className="py-12 text-center text-sm text-muted-foreground">{t("reports.margin.noProfiles")}</p>
            ) : (
              <ResponsiveContainer width="100%" height={240}>
                <BarChart data={byDay} margin={{ top: 8, right: 8, left: 0, bottom: 0 }}>
                  <CartesianGrid stroke={charts.grid} strokeOpacity={0.6} vertical={false} />
                  <XAxis
                    dataKey="day"
                    tick={AXIS_TICK}
                    axisLine={false}
                    tickLine={false}
                    minTickGap={12}
                  />
                  <YAxis
                    tick={AXIS_TICK}
                    axisLine={false}
                    tickLine={false}
                    width={48}
                    tickFormatter={(value: number) =>
                      new Intl.NumberFormat(localeOf(lang), { notation: "compact", maximumFractionDigits: 1 }).format(value)
                    }
                  />
                  <Tooltip
                    cursor={{ fill: charts.cursorFill, fillOpacity: 0.06 }}
                    content={<ChartTooltip formatter={(value) => formatCurrency(value, currency, lang)} />}
                  />
                  <ReferenceLine y={0} stroke={charts.axis} />
                  <Bar dataKey="margin" name={t("reports.margin.margin")} radius={[3, 3, 0, 0]} maxBarSize={14}>
                    {byDay.map((pt, i) => (
                      <Cell key={i} fill={pt.margin >= 0 ? MARGIN_POS : MARGIN_NEG} />
                    ))}
                  </Bar>
                </BarChart>
              </ResponsiveContainer>
            )}
          </CardContent>
        </Card>

        {/* Marge par site — quel routeur rapporte le plus ? */}
        <Card className="gap-4 py-4 sm:py-6">
          <CardHeader className="px-4 sm:px-6">
            <CardTitle className="text-base">{t("reports.margin.bySiteTitle")}</CardTitle>
            <CardDescription>{t("reports.margin.bySiteDesc")}</CardDescription>
          </CardHeader>
          <CardContent className="px-4 sm:px-6">
            {bySite.length === 0 ? (
              <p className="py-12 text-center text-sm text-muted-foreground">{t("reports.margin.noProfiles")}</p>
            ) : (
              <div className="max-h-64 space-y-4 overflow-y-auto pr-1">
                {bySite.map((site) => {
                  const rate = site.revenue > 0 ? (site.margin / site.revenue) * 100 : 0;
                  return (
                    <div key={site.routerName}>
                      <div className="flex items-baseline justify-between gap-3 text-sm">
                        <span className="truncate font-medium">{site.routerName}</span>
                        <span className="inline-flex shrink-0 items-center gap-2 text-xs text-muted-foreground">
                          <span className={cnMargin(site.margin)}>
                            {formatCurrency(site.margin, currency, lang)}
                          </span>
                          {site.revenue > 0 && <RateBadge rate={rate} />}
                        </span>
                      </div>
                      <div className="mt-1.5 h-2 overflow-hidden rounded-full bg-muted">
                        <div
                          className="h-full rounded-full"
                          style={{
                            width: `${Math.max(2, (Math.abs(site.margin) / maxSiteMargin) * 100)}%`,
                            background: site.margin >= 0 ? MARGIN_POS : MARGIN_NEG,
                          }}
                        />
                      </div>
                    </div>
                  );
                })}
              </div>
            )}
          </CardContent>
        </Card>
      </div>

      {/* Table par profil : ventes, CA, coût, marge + badge taux + part */}
      <Card className="gap-4 py-4 sm:py-6">
        <CardHeader className="px-4 sm:px-6">
          <CardTitle className="text-base">{t("reports.margin.byProfileTitle")}</CardTitle>
          <CardDescription>
            {t("reports.margin.byProfileDesc")}
            {bestProfile && bestProfile.margin > 0 && (
              <>
                {" · "}
                {tf("reports.margin.bestProfile", { name: bestProfile.name })}
              </>
            )}
          </CardDescription>
        </CardHeader>
        <CardContent className="px-0 sm:px-0">
          {byProfile.length === 0 ? (
            <p className="px-4 py-6 text-center text-sm text-muted-foreground sm:px-6">
              {t("reports.margin.noProfiles")}
            </p>
          ) : (
            <div className="overflow-x-auto">
              <Table>
                <TableHeader>
                  <TableRow className="hover:bg-transparent">
                    <TableHead className="pl-4 text-muted-foreground sm:pl-6">
                      {t("reports.margin.profile")}
                    </TableHead>
                    <TableHead className="text-right text-muted-foreground">
                      {t("reports.margin.sold")}
                    </TableHead>
                    <TableHead className="text-right text-muted-foreground">
                      {t("reports.margin.revenue")}
                    </TableHead>
                    <TableHead className="hidden text-right text-muted-foreground sm:table-cell">
                      {t("reports.margin.cost")}
                    </TableHead>
                    <TableHead className="text-right text-muted-foreground">
                      {t("reports.margin.margin")}
                    </TableHead>
                    <TableHead className="pr-4 text-right text-muted-foreground sm:pr-6">
                      {t("reports.margin.share")}
                    </TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {byProfile.map((row) => {
                    const rate = row.revenue > 0 ? (row.margin / row.revenue) * 100 : 0;
                    const share =
                      margin.margin > 0 ? Math.max(0, (row.margin / margin.margin) * 100) : 0;
                    return (
                      <TableRow key={row.name}>
                        <TableCell className="max-w-44 truncate pl-4 font-medium sm:pl-6">{row.name}</TableCell>
                        <TableCell className="text-right tabular-nums">{row.sold}</TableCell>
                        <TableCell className="text-right tabular-nums">
                          {formatCurrency(row.revenue, currency, lang)}
                        </TableCell>
                        <TableCell className="hidden text-right tabular-nums text-muted-foreground sm:table-cell">
                          {formatCurrency(row.cost, currency, lang)}
                        </TableCell>
                        <TableCell className="text-right">
                          <span className="inline-flex items-center gap-2">
                            <span className={cnMargin(row.margin)}>
                              {formatCurrency(row.margin, currency, lang)}
                            </span>
                            {row.revenue > 0 && <RateBadge rate={rate} />}
                          </span>
                        </TableCell>
                        <TableCell className="pr-4 text-right tabular-nums text-muted-foreground sm:pr-6">
                          {margin.margin > 0 ? pctFmt(share) : "—"}
                        </TableCell>
                      </TableRow>
                    );
                  })}
                </TableBody>
              </Table>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Vue Rapports — onglets Comptabilité / Activité / Marge.
// ---------------------------------------------------------------------------

export default function ReportsView() {
  const { t } = useI18n();
  const [tab, setTab] = useState<"accounting" | "activity" | "margin">("accounting");

  return (
    <div className="space-y-4 sm:space-y-6">
      <PageHeader
        title={t("reports.title")}
        description={t("reports.description")}
        actions={
          <Tabs value={tab} onValueChange={(value) => setTab(value as "accounting" | "activity" | "margin")}>
            <TabsList>
              <TabsTrigger value="accounting">{t("reports.tabAccounting")}</TabsTrigger>
              <TabsTrigger value="activity">{t("reports.tabActivity")}</TabsTrigger>
              <TabsTrigger value="margin">{t("reports.tabMargin")}</TabsTrigger>
            </TabsList>
          </Tabs>
        }
      />

      {tab === "accounting" ? (
        <AccountingTab visible={tab === "accounting"} />
      ) : tab === "activity" ? (
        <ActivityTab visible={tab === "activity"} />
      ) : (
        <MarginTab visible={tab === "margin"} />
      )}
    </div>
  );
}
