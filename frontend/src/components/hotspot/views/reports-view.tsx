"use client";

// Vue Rapports — Comptabilité multi-sites (ventes par jour/semaine/mois et par
// routeur) + onglet Activité (performance commerciale et trafic réseau) +
// onglet Marge (F13 : prix de vente vs coût, 30 jours glissants).

import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  Bar,
  BarChart,
  CartesianGrid,
  Legend,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import {
  CalendarRange,
  Coins,
  Download,
  Percent,
  Router as RouterIcon,
  ShoppingCart,
  TrendingUp,
  Wallet,
} from "lucide-react";
import { toast } from "sonner";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";

import { api, apiDownload } from "@/lib/hotspot/api";
import { localeOf, useI18n } from "@/lib/hotspot/i18n";
import type { Lang } from "@/lib/hotspot/i18n";
import type { AccountingData, AccountingPeriod, ReportsData, RouterDevice } from "@/lib/hotspot/types";
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

const GRID_STROKE = "#27272a";
const AXIS_TICK = { fontSize: 11, fill: "#71717a" };

const VOUCHER_STATUS_ROWS = [
  { key: "active", labelKey: "common.statusActive", color: "#10b981" },
  { key: "used", labelKey: "common.statusUsed", color: "#f59e0b" },
  { key: "expired", labelKey: "common.statusExpired", color: "#71717a" },
  { key: "disabled", labelKey: "common.statusDisabled", color: "#f43f5e" },
] as const;

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

// ---------------------------------------------------------------------------
// Onglet Comptabilité — ventes par jour/semaine/mois, filtrables par routeur.
// ---------------------------------------------------------------------------

function AccountingTab({ visible }: { visible: boolean }) {
  const { t, tf, lang } = useI18n();
  const currency = useCurrency();
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
          <LoadingCards cards={3} />
          <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
            <Skeleton className="h-80 rounded-xl" />
            <Skeleton className="h-80 rounded-xl" />
          </div>
        </div>
      ) : !data ? null : (
        <>
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
            <StatCard
              title={t("reports.revenue")}
              value={formatCurrency(data.totals.revenue, currency, lang)}
              sub={`${filterLabel}${t(periodMeta.windowKey)}`}
              icon={Wallet}
            />
            <StatCard
              title={t("reports.sales")}
              value={String(data.totals.sales)}
              sub={`${t("reports.vouchersSold")} · ${t(periodMeta.barsKey)}`}
              icon={ShoppingCart}
            />
            <StatCard
              title={t("reports.avgTicket")}
              value={formatCurrency(data.totals.avgTicket, currency, lang)}
              sub={t("reports.perVoucher")}
              icon={TrendingUp}
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
                </CardDescription>
              </CardHeader>
              <CardContent className="px-4 sm:px-6">
                <ResponsiveContainer width="100%" height={280}>
                  <BarChart data={data.series} margin={{ top: 8, right: 8, left: 0, bottom: 0 }}>
                    <CartesianGrid stroke={GRID_STROKE} strokeOpacity={0.6} vertical={false} />
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
                      cursor={{ fill: "rgba(255,255,255,0.04)" }}
                      content={
                        <AccountingTooltip
                          currency={currency}
                          lang={lang}
                          revenueLabel={t("reports.tooltipRevenue")}
                          salesLabel={t("reports.tooltipSales")}
                        />
                      }
                    />
                    <Bar dataKey="revenue" name={t("reports.revenue")} fill="#10b981" radius={[4, 4, 0, 0]} maxBarSize={40} />
                  </BarChart>
                </ResponsiveContainer>
              </CardContent>
            </Card>

            {/* Répartition par routeur */}
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
                  <div className="space-y-5">
                    {byRouter.map((router) => (
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
                          </span>
                        </div>
                        <div className="mt-1.5 h-2 overflow-hidden rounded-full bg-muted">
                          <div
                            className="h-full rounded-full bg-primary transition-all"
                            style={{ width: `${Math.max(2, (router.share / maxShare) * 100)}%` }}
                          />
                        </div>
                      </div>
                    ))}
                  </div>
                )}
              </CardContent>
            </Card>
          </div>
        </>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Onglet Activité — performance commerciale et trafic réseau (7/14/30 jours).
// ---------------------------------------------------------------------------

function ActivityTab({ visible }: { visible: boolean }) {
  const { t, tf, lang } = useI18n();
  const currency = useCurrency();
  const [days, setDays] = useState(14);

  const { data, isLoading } = useQuery({
    queryKey: ["/api/reports", days],
    queryFn: () => api<ReportsData>("/api/reports", { params: { days } }),
    placeholderData: (previous) => previous,
    enabled: visible,
  });

  const salesByProfile = useMemo(
    () => [...(data?.salesByProfile ?? [])].sort((a, b) => b.revenue - a.revenue),
    [data],
  );
  const maxProfileRevenue = Math.max(...salesByProfile.map((s) => s.revenue), 1);
  const maxStatusCount = data
    ? Math.max(...VOUCHER_STATUS_ROWS.map((row) => data.voucherStatus[row.key]), 1)
    : 1;

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
          <LoadingCards cards={3} />
          <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
            <Skeleton className="h-80 rounded-xl" />
            <Skeleton className="h-80 rounded-xl" />
            <Skeleton className="h-64 rounded-xl" />
            <Skeleton className="h-64 rounded-xl" />
          </div>
        </div>
      ) : !data ? null : (
        <>
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
            <StatCard
              title={t("reports.revenue")}
              value={formatCurrency(data.totals.revenue, currency, lang)}
              sub={tf("reports.lastDays", { n: days })}
              icon={Wallet}
            />
            <StatCard
              title={t("reports.sales")}
              value={String(data.totals.sales)}
              sub={t("reports.vouchersSold")}
              icon={ShoppingCart}
            />
            <StatCard
              title={t("reports.avgTicket")}
              value={formatCurrency(data.totals.avgTicket, currency, lang)}
              sub={t("reports.perVoucher")}
              icon={TrendingUp}
            />
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
                    <CartesianGrid stroke={GRID_STROKE} strokeOpacity={0.6} vertical={false} />
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
                      cursor={{ fill: "rgba(255,255,255,0.04)" }}
                      content={<ChartTooltip formatter={(value) => formatCurrency(value, currency, lang)} />}
                    />
                    <Bar dataKey="value" name={t("reports.revenue")} fill="#10b981" radius={[4, 4, 0, 0]} maxBarSize={40} />
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
                    <CartesianGrid stroke={GRID_STROKE} strokeOpacity={0.6} vertical={false} />
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
                      tickFormatter={(value: number) => formatBytes(value)}
                    />
                    <Tooltip
                      cursor={{ stroke: "#3f3f46", strokeDasharray: "4 4" }}
                      content={<ChartTooltip formatter={(value) => formatBytes(value)} />}
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
                      stroke="#10b981"
                      strokeWidth={2}
                      dot={false}
                      activeDot={{ r: 4 }}
                    />
                    <Line
                      dataKey="bytesOut"
                      name={t("reports.outbound")}
                      type="monotone"
                      stroke="#14b8a6"
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
                  {VOUCHER_STATUS_ROWS.map((row) => {
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
                              width: `${Math.max(2, (count / maxStatusCount) * 100)}%`,
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
// ---------------------------------------------------------------------------

function MarginTab({ visible }: { visible: boolean }) {
  const { t, lang } = useI18n();
  const currency = useCurrency();

  const { data, isLoading } = useQuery({
    queryKey: ["/api/reports", 30],
    queryFn: () => api<ReportsData>("/api/reports", { params: { days: 30 } }),
    placeholderData: (previous) => previous,
    enabled: visible,
  });

  const margin = data?.margin;
  const pctFmt = (value: number): string =>
    `${value.toFixed(1).replace(".", lang === "fr" ? "," : ".")}${lang === "fr" ? " " : ""}%`;

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

  return (
    <div className="space-y-4 sm:space-y-6">
      {/* KPI : CA, coût, marge, taux de marge */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <StatCard
          title={t("reports.margin.revenue")}
          value={formatCurrency(margin.revenue, currency, lang)}
          sub={t("reports.margin.window")}
          icon={Wallet}
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
        />
        <StatCard
          title={t("reports.margin.rate")}
          value={pctFmt(margin.marginPct)}
          sub={t("reports.margin.window")}
          icon={Percent}
        />
      </div>

      {/* Table par profil : ventes, CA, coût, marge + badge taux */}
      <Card className="gap-4 py-4 sm:py-6">
        <CardHeader className="px-4 sm:px-6">
          <CardTitle className="text-base">{t("reports.margin.byProfileTitle")}</CardTitle>
          <CardDescription>{t("reports.margin.byProfileDesc")}</CardDescription>
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
                    <TableHead className="pr-4 text-right text-muted-foreground sm:pr-6">
                      {t("reports.margin.margin")}
                    </TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {byProfile.map((row) => {
                    const rate = row.revenue > 0 ? (row.margin / row.revenue) * 100 : 0;
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
                        <TableCell className="pr-4 text-right sm:pr-6">
                          <span className="inline-flex items-center gap-2">
                            <span
                              className={cnMargin(row.margin)}
                            >
                              {formatCurrency(row.margin, currency, lang)}
                            </span>
                            {row.revenue > 0 && (
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
                                {pctFmt(rate)}
                              </Badge>
                            )}
                          </span>
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

/** Couleur de la marge : verte si positive, rouge si négative, neutre sinon. */
function cnMargin(margin: number): string {
  if (margin > 0) return "font-semibold text-emerald-600 dark:text-emerald-400";
  if (margin < 0) return "font-semibold text-destructive";
  return "font-semibold text-muted-foreground";
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
