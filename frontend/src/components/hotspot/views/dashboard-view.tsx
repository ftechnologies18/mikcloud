"use client";

import { useQuery } from "@tanstack/react-query";
import {
  Activity,
  Clock,
  Cog,
  HandCoins,
  Radio,
  Router as RouterIcon,
  ShoppingCart,
  Store,
  Ticket,
  Users,
  Wallet,
} from "lucide-react";
import type { LucideIcon } from "lucide-react";
import {
  Area,
  AreaChart,
  Bar,
  BarChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { EmptyState } from "@/components/hotspot/empty-state";
import { HourlyHeatmap } from "@/components/hotspot/parts/hourly-heatmap";
import { LoadingCards, LoadingRows } from "@/components/hotspot/loading";
import { PageHeader } from "@/components/hotspot/page-header";
import { StatCard } from "@/components/hotspot/stat-card";
import { StatusBadge } from "@/components/hotspot/status-badge";
import { SubscriptionBanner } from "@/components/hotspot/parts/sa-subscription-banner";
import { api } from "@/lib/hotspot/api";
import { localeOf, useI18n } from "@/lib/hotspot/i18n";
import { formatCurrency, timeAgo } from "@/lib/hotspot/format";
import { useChartPalette } from "@/lib/hotspot/chart-theme";
import { cn } from "@/lib/utils";
import type { Lang } from "@/lib/hotspot/i18n";
import type { Activity as ActivityItem, AppSettings, DashboardData, SiteOverview } from "@/lib/hotspot/types";

interface TooltipItem {
  value?: number | string;
}

function ChartTooltip({
  active,
  payload,
  label,
  formatter,
}: {
  active?: boolean;
  payload?: TooltipItem[];
  label?: string;
  formatter?: (value: number) => string;
}) {
  if (!active || !payload || payload.length === 0) return null;
  const raw = payload[0].value;
  const value = typeof raw === "number" ? raw : Number(raw ?? 0);
  return (
    <div className="rounded-lg border border-border bg-popover px-3 py-2 text-xs shadow-lg">
      <p className="text-muted-foreground">{label}</p>
      <p className="mt-0.5 font-semibold text-foreground">
        {formatter ? formatter(value) : String(value)}
      </p>
    </div>
  );
}

const ACTIVITY_ICONS: Record<ActivityItem["type"], LucideIcon> = {
  router: RouterIcon,
  user: Users,
  voucher: Ticket,
  reseller: Store,
  session: Radio,
  system: Cog,
};

// SiteCard — carte de vue d'ensemble d'un site (routeur) : le multi-sites
// est au cœur du modèle SaaS (1 compte = N hotspots).
function SiteCard({ site, currency, lang }: { site: SiteOverview; currency: string; lang: Lang }) {
  const { t } = useI18n();
  const online = site.status === "online";
  const nf = (value: number): string => new Intl.NumberFormat(localeOf(lang)).format(value);
  return (
    <Card className="gap-0 py-0">
      <CardContent className="p-4 sm:p-5">
        <div className="flex items-center justify-between gap-2">
          <div className="flex min-w-0 items-center gap-2">
            <span className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
              <RouterIcon className="size-4" />
            </span>
            <span className="truncate font-medium" title={site.routerName}>
              {site.routerName}
            </span>
          </div>
          <StatusBadge status={online ? "online" : "offline"} dot />
        </div>

        <dl className="mt-4 grid grid-cols-2 gap-x-4 gap-y-3">
          <div>
            <dt className="text-xs text-muted-foreground">{t("dashboard.site.sessions")}</dt>
            <dd className="mt-0.5 flex items-center gap-1.5 font-semibold tabular-nums">
              {nf(site.activeSessions)}
              {online && <span className="live-dot" aria-label={t("dashboard.live")} />}
            </dd>
          </div>
          <div>
            <dt className="text-xs text-muted-foreground">{t("dashboard.site.salesToday")}</dt>
            <dd className="mt-0.5 font-semibold tabular-nums">{nf(site.soldToday)}</dd>
          </div>
          <div>
            <dt className="text-xs text-muted-foreground">{t("dashboard.site.users")}</dt>
            <dd className="mt-0.5 font-semibold tabular-nums">{nf(site.onlineUsers)}</dd>
          </div>
          <div>
            <dt className="text-xs text-muted-foreground">{t("dashboard.site.vouchers")}</dt>
            <dd className="mt-0.5 font-semibold tabular-nums">{nf(site.activeVouchers)}</dd>
          </div>
        </dl>

        <div className="mt-4 flex items-center justify-between gap-2 border-t pt-3">
          <span className="text-xs text-muted-foreground">{t("dashboard.site.revenue30")}</span>
          <span className="text-sm font-semibold text-primary tabular-nums">
            {formatCurrency(site.revenue30d, currency, lang)}
          </span>
        </div>
      </CardContent>
    </Card>
  );
}

export default function DashboardView() {
  const { t, tf, lang } = useI18n();
  const charts = useChartPalette();
  const AXIS_TICK = { fill: charts.axis, fontSize: 12 };
  const { data, isLoading, isError, refetch } = useQuery({
    queryKey: ["/api/dashboard"],
    queryFn: () => api<DashboardData>("/api/dashboard"),
    refetchInterval: 15_000,
  });

  const { data: settings } = useQuery({
    queryKey: ["/api/settings"],
    queryFn: () => api<AppSettings>("/api/settings"),
  });
  const currency = settings?.tenant.currency ?? "FCFA";

  const nf = (value: number): string => new Intl.NumberFormat(localeOf(lang)).format(value);
  const compact = (value: number): string =>
    new Intl.NumberFormat(localeOf(lang), { notation: "compact", maximumFractionDigits: 1 }).format(value);

  if (isLoading) {
    return (
      <div className="space-y-6">
        <PageHeader title={t("dashboard.title")} description={t("dashboard.description")} />
        <LoadingCards cards={6} />
        <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
          <Card>
            <CardContent className="p-4 sm:p-6">
              <Skeleton className="h-64 w-full sm:h-72" />
            </CardContent>
          </Card>
          <Card>
            <CardContent className="p-4 sm:p-6">
              <Skeleton className="h-64 w-full sm:h-72" />
            </CardContent>
          </Card>
        </div>
        <Card>
          <LoadingRows rows={5} />
        </Card>
      </div>
    );
  }

  if (isError || !data) {
    return (
      <Card>
        <EmptyState
          icon={Activity}
          title={t("dashboard.loadError")}
          description={t("dashboard.loadErrorDesc")}
          action={
            <Button variant="outline" onClick={() => void refetch()}>
              <Clock className="size-4" />
              {t("common.retry")}
            </Button>
          }
        />
      </Card>
    );
  }

  const { kpis } = data;
  const maxProfileTotal = Math.max(1, ...data.topProfiles.map((p) => p.total));

  return (
    <div className="space-y-6">
      <PageHeader title={t("dashboard.title")} description={t("dashboard.description")} />

      {/* Bannière abonnement (v1 bienveillante) : expiré → rappel persistant,
          échéance ≤ 7 j → rappel doux. Accès jamais bloqué. */}
      <SubscriptionBanner />

      {/* KPIs */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-3">
        <StatCard title={t("dashboard.kpi.activeSessions")} value={nf(kpis.activeSessions)} icon={Radio} live />
        <StatCard
          title={t("dashboard.kpi.activeUsers")}
          value={nf(kpis.onlineNow)}
          icon={Users}
          live
          sub={tf("dashboard.kpi.activeUsersSub", { n: nf(kpis.totalUsers) })}
        />
        <StatCard title={t("dashboard.kpi.activeVouchers")} value={nf(kpis.activeVouchers)} icon={Ticket} />
        <StatCard
          title={t("dashboard.kpi.soldToday")}
          value={nf(kpis.soldToday)}
          icon={ShoppingCart}
          sub={t("dashboard.kpi.soldTodaySub")}
        />
        <StatCard
          title={t("dashboard.kpi.revenue30")}
          value={formatCurrency(kpis.revenue30d, currency, lang)}
          icon={Wallet}
        />
        <StatCard
          title={t("dashboard.kpi.routers")}
          value={`${kpis.routersOnline}/${kpis.routersTotal}`}
          icon={RouterIcon}
          sub={t("dashboard.kpi.routersSub")}
        />
      </div>

      {/* Vue d'ensemble multi-sites — 1 compte = N hotspots */}
      {data.sites.length > 0 && (
        <section aria-label={t("dashboard.sites")} className="space-y-3">
          <div className="flex flex-wrap items-baseline justify-between gap-2">
            <h2 className="text-sm font-semibold tracking-wide text-muted-foreground uppercase">
              {t("dashboard.sites")}
            </h2>
            <p className="text-xs text-muted-foreground">
              {tf("dashboard.sitesCount", {
                n: data.sites.length,
                p: data.sites.length > 1 ? "s" : "",
              })}
            </p>
          </div>
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-3">
            {data.sites.map((site) => (
              <SiteCard key={site.routerId} site={site} currency={currency} lang={lang} />
            ))}
          </div>
        </section>
      )}

      {/* N°19 V2 — créances revendeurs (dépôt-vente) : visible uniquement
          quand une créance existe — la trésorerie dormant chez les revendeurs. */}
      {data.receivables && data.receivables.count > 0 && (
        <Card>
          <CardHeader className="px-4 sm:px-6">
            <CardTitle className="flex flex-wrap items-center justify-between gap-2 text-base">
              <span className="flex items-center gap-2">
                <HandCoins className="size-4 text-amber-600 dark:text-amber-400" aria-hidden />
                {t("dashboard.receivables")}
              </span>
              <span className="text-sm font-semibold text-amber-600 tabular-nums dark:text-amber-400">
                {t("dashboard.receivablesTotal")} : {formatCurrency(data.receivables.totalDebt, currency, lang)}
              </span>
            </CardTitle>
          </CardHeader>
          <CardContent className="px-4 pb-4 sm:px-6 sm:pb-6">
            <p className="text-xs text-muted-foreground">
              {tf("dashboard.receivablesCount", { n: data.receivables.count })} · {t("dashboard.receivablesDesc")}
            </p>
            <ul className="mt-3 max-h-56 space-y-1.5 overflow-y-auto pr-1">
              {data.receivables.items.map((item) => (
                <li key={item.resellerId} className="flex items-center justify-between gap-3 rounded-lg border px-3 py-2">
                  <div className="min-w-0">
                    <p className="truncate text-sm font-medium">{item.name}</p>
                    <p className="text-xs text-muted-foreground">
                      <span
                        className={cn(
                          "font-medium",
                          item.level === "danger" ? "text-destructive" : item.level === "warn" ? "text-amber-600 dark:text-amber-400" : "",
                        )}
                      >
                        {tf("dashboard.receivablesAging", { n: item.agingDays })}
                      </span>
                      {item.overCeiling && (
                        <span className="ml-2 font-medium text-destructive">· {t("dashboard.receivablesOverCeiling")}</span>
                      )}
                    </p>
                  </div>
                  <p
                    className={cn(
                      "shrink-0 text-sm font-semibold tabular-nums",
                      item.overCeiling ? "text-destructive" : "text-amber-600 dark:text-amber-400",
                    )}
                  >
                    {formatCurrency(item.debt, currency, lang)}
                  </p>
                </li>
              ))}
            </ul>
          </CardContent>
        </Card>
      )}

      {/* Graphiques */}
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader className="px-4 sm:px-6">
            <CardTitle className="text-base">{t("dashboard.sessionsChart")}</CardTitle>
          </CardHeader>
          <CardContent className="px-2 pb-2 sm:px-4 sm:pb-4">
            <div className="h-64 w-full sm:h-72">
              <ResponsiveContainer width="100%" height="100%">
                <AreaChart data={data.sessionsTimeline} margin={{ top: 8, right: 8, left: 0, bottom: 0 }}>
                  <CartesianGrid stroke={charts.grid} strokeOpacity={0.6} vertical={false} />
                  <XAxis
                    dataKey="t"
                    tick={AXIS_TICK}
                    axisLine={false}
                    tickLine={false}
                    interval="preserveStartEnd"
                    minTickGap={24}
                  />
                  <YAxis tick={AXIS_TICK} axisLine={false} tickLine={false} width={32} allowDecimals={false} />
                  <Tooltip
                    cursor={{ stroke: charts.grid }}
                    content={
                      <ChartTooltip formatter={(v) => tf("dashboard.sessionUnit", { n: nf(v), p: v > 1 ? "s" : "" })} />
                    }
                  />
                  <Area
                    type="monotone"
                    dataKey="value"
                    stroke={charts.series[0]}
                    strokeWidth={2}
                    fill={charts.areaFill}
                    fillOpacity={1}
                  />
                </AreaChart>
              </ResponsiveContainer>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="px-4 sm:px-6">
            <CardTitle className="text-base">{t("dashboard.revenueChart")}</CardTitle>
          </CardHeader>
          <CardContent className="px-2 pb-2 sm:px-4 sm:pb-4">
            <div className="h-64 w-full sm:h-72">
              <ResponsiveContainer width="100%" height="100%">
                <BarChart data={data.revenueByDay} margin={{ top: 8, right: 8, left: 0, bottom: 0 }}>
                  <CartesianGrid stroke={charts.grid} strokeOpacity={0.6} vertical={false} />
                  <XAxis dataKey="day" tick={AXIS_TICK} axisLine={false} tickLine={false} minTickGap={16} />
                  <YAxis
                    tick={AXIS_TICK}
                    axisLine={false}
                    tickLine={false}
                    width={44}
                    tickFormatter={(v: number) => compact(v)}
                  />
                  <Tooltip
                    cursor={{ fill: charts.cursorFill, fillOpacity: 0.08 }}
                    content={<ChartTooltip formatter={(v) => formatCurrency(v, currency, lang)} />}
                  />
                  <Bar dataKey="value" fill={charts.series[0]} radius={[4, 4, 0, 0]} maxBarSize={36} />
                </BarChart>
              </ResponsiveContainer>
            </div>
          </CardContent>
        </Card>
      </div>

      {/* N°10 — affluence réelle par tranche horaire (heatmap 7 jours) */}
      <HourlyHeatmap days={7} currency={currency} />

      {/* Profils + activité */}
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader className="px-4 sm:px-6">
            <CardTitle className="text-base">{t("dashboard.topProfiles")}</CardTitle>
          </CardHeader>
          <CardContent className="px-4 pb-4 sm:px-6 sm:pb-6">
            {data.topProfiles.length === 0 ? (
              <EmptyState
                icon={Ticket}
                title={t("dashboard.topProfilesEmpty")}
                description={t("dashboard.topProfilesEmptyDesc")}
              />
            ) : (
              <div className="space-y-4">
                {data.topProfiles.map((p) => (
                  <div key={p.name}>
                    <div className="flex items-baseline justify-between gap-3">
                      <span className="truncate text-sm font-medium">{p.name}</span>
                      <span className="shrink-0 text-xs text-muted-foreground">
                        {tf("dashboard.usersCount", { n: nf(p.users), p: p.users > 1 ? "s" : "" })} ·{" "}
                        {formatCurrency(p.total, currency, lang)}
                      </span>
                    </div>
                    <div className="mt-1.5 h-2 w-full overflow-hidden rounded-full bg-primary/20">
                      <div
                        className="h-full rounded-full bg-primary transition-all"
                        style={{ width: `${Math.max(3, Math.round((p.total / maxProfileTotal) * 100))}%` }}
                      />
                    </div>
                  </div>
                ))}
              </div>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="px-4 sm:px-6">
            <CardTitle className="text-base">{t("dashboard.activity")}</CardTitle>
          </CardHeader>
          <CardContent className="px-4 pb-4 sm:px-6 sm:pb-6">
            {data.recentActivity.length === 0 ? (
              <EmptyState icon={Activity} title={t("dashboard.activityEmpty")} />
            ) : (
              <ul className="max-h-96 space-y-1 overflow-y-auto pr-1">
                {data.recentActivity.map((a) => {
                  const Icon = ACTIVITY_ICONS[a.type] ?? Cog;
                  return (
                    <li key={a.id} className="flex items-start gap-3 rounded-md px-2 py-2 transition-colors hover:bg-accent/40">
                      <span className="mt-0.5 flex size-7 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
                        <Icon className="size-3.5" />
                      </span>
                      <div className="min-w-0 flex-1">
                        <p className="text-sm leading-snug">{a.message}</p>
                        <p className="mt-0.5 text-xs text-muted-foreground">{timeAgo(a.at, lang)}</p>
                      </div>
                    </li>
                  );
                })}
              </ul>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
