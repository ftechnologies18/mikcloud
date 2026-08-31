"use client";

// Console plateforme — VUE D'ENSEMBLE (admin plateforme uniquement).
// Cockpit du propriétaire du SaaS : KPIs globaux toutes comptes confondus,
// santé des abonnements, croissance mensuelle et top comptes par revenu.

import { useQuery } from "@tanstack/react-query";
import {
  Activity,
  Building2,
  Clock,
  CreditCard,
  Router as RouterIcon,
  ShoppingCart,
  Ticket,
  TrendingUp,
  Users,
  Wallet,
} from "lucide-react";
import { Area, AreaChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { EmptyState } from "@/components/hotspot/empty-state";
import { LoadingCards } from "@/components/hotspot/loading";
import { PageHeader } from "@/components/hotspot/page-header";
import { StatCard } from "@/components/hotspot/stat-card";
import { StatusBadge } from "@/components/hotspot/status-badge";
import { useCurrency } from "@/components/hotspot/parts/sd-currency";
import { fetchPlatformOverview } from "@/lib/hotspot/api";
import { localeOf, useI18n } from "@/lib/hotspot/i18n";
import { formatCurrency } from "@/lib/hotspot/format";
import { useChartPalette } from "@/lib/hotspot/chart-theme";
import { useHotspotStore } from "@/lib/hotspot/store";
import type { PlatformOverview } from "@/lib/hotspot/types";

function ChartTooltip({
  active,
  payload,
  label,
  formatter,
}: {
  active?: boolean;
  payload?: { value?: number | string }[];
  label?: string;
  formatter?: (value: number) => string;
}) {
  if (!active || !payload || payload.length === 0) return null;
  const raw = payload[0].value;
  const value = typeof raw === "number" ? raw : Number(raw ?? 0);
  return (
    <div className="rounded-lg border border-border bg-popover px-3 py-2 text-xs shadow-lg">
      <p className="text-muted-foreground">{label}</p>
      <p className="mt-0.5 font-semibold text-foreground">{formatter ? formatter(value) : String(value)}</p>
    </div>
  );
}

function SubBadge({ state }: { state: string }) {
  const { t } = useI18n();
  if (state === "active") {
    return (
      <Badge variant="outline" className="border-emerald-500/30 bg-emerald-500/10 px-1.5 py-0 text-[10px] font-medium text-emerald-600 dark:text-emerald-400">
        {t("platform.sub.active")}
      </Badge>
    );
  }
  if (state === "expired") {
    return (
      <Badge variant="outline" className="border-destructive/30 bg-destructive/10 px-1.5 py-0 text-[10px] font-medium text-destructive">
        {t("platform.sub.expired")}
      </Badge>
    );
  }
  return (
    <Badge variant="outline" className="border-border bg-muted px-1.5 py-0 text-[10px] font-medium text-muted-foreground">
      {t("platform.sub.beta")}
    </Badge>
  );
}

export default function PlatformOverviewView() {
  const { t, tf, lang } = useI18n();
  const charts = useChartPalette();
  const currency = useCurrency();
  const setView = useHotspotStore((s) => s.setView);
  const AXIS_TICK = { fill: charts.axis, fontSize: 12 };

  const { data, isLoading, isError, refetch } = useQuery({
    queryKey: ["/api/admin/overview"],
    queryFn: fetchPlatformOverview,
    refetchInterval: 30_000,
    retry: (failureCount, err) => !(err instanceof Error && err.message.includes("403")) && failureCount < 1,
  });

  const nf = (value: number): string => new Intl.NumberFormat(localeOf(lang)).format(value);
  const monthLabel = (m: string): string => {
    const d = new Date(`${m}-01T00:00:00Z`);
    if (Number.isNaN(d.getTime())) return m;
    return new Intl.DateTimeFormat(localeOf(lang), { month: "short", year: "2-digit", timeZone: "UTC" }).format(d);
  };

  if (isLoading) {
    return (
      <div className="space-y-6">
        <PageHeader title={t("platform.title")} description={t("platform.description")} />
        <LoadingCards cards={6} />
      </div>
    );
  }

  if (isError || !data) {
    return (
      <Card>
        <EmptyState
          icon={Building2}
          title={t("platform.loadError")}
          description={t("platform.loadErrorDesc")}
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

  const maxGrowth = Math.max(1, ...data.growth.map((g) => g.accounts));

  return (
    <div className="space-y-6">
      <PageHeader title={t("platform.title")} description={t("platform.description")} />

      {/* KPIs globaux */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-3">
        <StatCard
          title={t("platform.kpi.accounts")}
          value={`${nf(data.accounts.active)}/${nf(data.accounts.total)}`}
          icon={Building2}
          sub={tf("platform.kpi.accountsSub", { n: nf(data.accounts.new30d) })}
        />
        <StatCard
          title={t("platform.kpi.routers")}
          value={`${nf(data.routers.online)}/${nf(data.routers.total)}`}
          icon={RouterIcon}
          sub={t("platform.kpi.routersSub")}
        />
        <StatCard title={t("platform.kpi.users")} value={nf(data.hotspotUsers)} icon={Users} />
        <StatCard title={t("platform.kpi.sessions")} value={nf(data.sessions)} icon={Activity} live />
        <StatCard title={t("platform.kpi.sales30")} value={nf(data.sales30d)} icon={ShoppingCart} />
        <StatCard title={t("platform.kpi.revenue30")} value={formatCurrency(data.revenue30d, currency, lang)} icon={Wallet} />
      </div>

      {/* Santé abonnements + inscriptions */}
      <Card>
        <CardHeader className="px-4 sm:px-6">
          <CardTitle className="flex items-center gap-2 text-base">
            <CreditCard className="size-4 text-muted-foreground" />
            {t("platform.subscriptions")}
          </CardTitle>
        </CardHeader>
        <CardContent className="grid grid-cols-1 gap-4 px-4 pb-4 sm:grid-cols-3 sm:px-6 sm:pb-6">
          <div className="flex items-center justify-between rounded-lg border p-3">
            <span className="text-sm text-muted-foreground">{t("platform.sub.active")}</span>
            <span className="text-lg font-semibold tabular-nums text-emerald-600 dark:text-emerald-400">
              {nf(data.subscriptions.active)}
            </span>
          </div>
          <div className="flex items-center justify-between rounded-lg border p-3">
            <span className="text-sm text-muted-foreground">{t("platform.sub.expired")}</span>
            <span className="text-lg font-semibold tabular-nums text-destructive">{nf(data.subscriptions.expired)}</span>
          </div>
          <div className="flex items-center justify-between rounded-lg border p-3">
            <span className="text-sm text-muted-foreground">{t("platform.sub.beta")}</span>
            <span className="text-lg font-semibold tabular-nums">{nf(data.subscriptions.beta)}</span>
          </div>
          <p className="text-xs text-muted-foreground sm:col-span-3">
            {data.registerOpen ? t("platform.register.open") : t("platform.register.closed")}
          </p>
        </CardContent>
      </Card>

      {/* Croissance + top comptes */}
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader className="px-4 sm:px-6">
            <CardTitle className="flex items-center gap-2 text-base">
              <TrendingUp className="size-4 text-muted-foreground" />
              {t("platform.growthChart")}
            </CardTitle>
          </CardHeader>
          <CardContent className="px-2 pb-2 sm:px-4 sm:pb-4">
            <div className="h-64 w-full sm:h-72">
              <ResponsiveContainer width="100%" height="100%">
                <AreaChart data={data.growth.map((g) => ({ ...g, label: monthLabel(g.month) }))} margin={{ top: 8, right: 8, left: 0, bottom: 0 }}>
                  <CartesianGrid stroke={charts.grid} strokeOpacity={0.6} vertical={false} />
                  <XAxis dataKey="label" tick={AXIS_TICK} axisLine={false} tickLine={false} minTickGap={12} />
                  <YAxis tick={AXIS_TICK} axisLine={false} tickLine={false} width={32} allowDecimals={false} />
                  <Tooltip
                    cursor={{ stroke: charts.grid }}
                    content={<ChartTooltip formatter={(v) => tf("platform.accountUnit", { n: nf(v), p: v > 1 ? "s" : "" })} />}
                  />
                  <Area type="monotone" dataKey="accounts" stroke={charts.series[0]} strokeWidth={2} fill={charts.areaFill} fillOpacity={1} />
                </AreaChart>
              </ResponsiveContainer>
            </div>
          </CardContent>
        </Card>

        <Card className="gap-0 py-0">
          <CardHeader className="px-4 pb-0 sm:px-6">
            <CardTitle className="text-base">{t("platform.topAccounts")}</CardTitle>
          </CardHeader>
          <CardContent className="px-0 pb-2">
            {data.topAccounts.length === 0 ? (
              <div className="px-4 py-6 sm:px-6">
                <EmptyState icon={Ticket} title={t("platform.topAccountsEmpty")} />
              </div>
            ) : (
              <ul className="max-h-80 divide-y overflow-y-auto">
                {data.topAccounts.map((acc) => (
                  <li key={acc.id} className="flex items-center gap-3 px-4 py-3 sm:px-6">
                    <span className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
                      <Building2 className="size-4" />
                    </span>
                    <div className="min-w-0 flex-1">
                      <p className="truncate text-sm font-medium">{acc.name}</p>
                      <p className="text-xs text-muted-foreground">
                        {nf(acc.users)} {t("platform.usersShort")} · {nf(acc.routers)} {t("platform.routersShort")}
                      </p>
                    </div>
                    <SubBadge state={acc.subscription} />
                    <StatusBadge status={acc.status === "disabled" ? "disabled" : "online"} />
                    <span className="w-24 shrink-0 text-right text-sm font-semibold tabular-nums text-primary">
                      {formatCurrency(acc.revenue30d, currency, lang)}
                    </span>
                  </li>
                ))}
              </ul>
            )}
          </CardContent>
        </Card>
      </div>

      {/* Raccourci : gérer les comptes */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-dashed p-4">
        <p className="text-sm text-muted-foreground">{t("platform.manageCta")}</p>
        <Button size="sm" onClick={() => setView("accounts")}>
          <Building2 className="size-4" />
          {t("platform.manageCtaButton")}
        </Button>
      </div>
    </div>
  );
}
