"use client";

// Vue Rapports — performance commerciale et trafic réseau (périodes 7/14/30 jours).

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
import { ShoppingCart, TrendingUp, Wallet } from "lucide-react";

import { api } from "@/lib/hotspot/api";
import type { ReportsData } from "@/lib/hotspot/types";
import { formatBytes, formatCurrency } from "@/lib/hotspot/format";
import { LoadingCards } from "@/components/hotspot/loading";
import { PageHeader } from "@/components/hotspot/page-header";
import { StatCard } from "@/components/hotspot/stat-card";
import { ChartTooltip } from "@/components/hotspot/parts/sd-chart-tooltip";
import { useCurrency } from "@/components/hotspot/parts/sd-currency";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";

const PERIODS = [
  { value: "7", label: "7 jours" },
  { value: "14", label: "14 jours" },
  { value: "30", label: "30 jours" },
];

const GRID_STROKE = "#27272a";
const AXIS_TICK = { fontSize: 11, fill: "#71717a" };

const VOUCHER_STATUS_ROWS = [
  { key: "active", label: "Actifs", color: "#10b981" },
  { key: "used", label: "Utilisés", color: "#f59e0b" },
  { key: "expired", label: "Expirés", color: "#71717a" },
  { key: "disabled", label: "Désactivés", color: "#f43f5e" },
] as const;

const compactFormatter = new Intl.NumberFormat("fr-FR", { notation: "compact", maximumFractionDigits: 1 });

export default function ReportsView() {
  const currency = useCurrency();
  const [days, setDays] = useState(14);

  const { data, isLoading } = useQuery({
    queryKey: ["/api/reports", days],
    queryFn: () => api<ReportsData>("/api/reports", { params: { days } }),
    placeholderData: (previous) => previous,
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
      <PageHeader
        title="Rapports"
        description="Performance commerciale et trafic réseau"
        actions={
          <Tabs value={String(days)} onValueChange={(value) => setDays(Number(value))}>
            <TabsList>
              {PERIODS.map((period) => (
                <TabsTrigger key={period.value} value={period.value}>
                  {period.label}
                </TabsTrigger>
              ))}
            </TabsList>
          </Tabs>
        }
      />

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
              title="Revenus"
              value={formatCurrency(data.totals.revenue, currency)}
              sub={`sur ${days} derniers jours`}
              icon={Wallet}
            />
            <StatCard
              title="Ventes"
              value={String(data.totals.sales)}
              sub="vouchers vendus"
              icon={ShoppingCart}
            />
            <StatCard
              title="Panier moyen"
              value={formatCurrency(data.totals.avgTicket, currency)}
              sub="par voucher vendu"
              icon={TrendingUp}
            />
          </div>

          <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
            <Card className="gap-4 py-4 sm:py-6">
              <CardHeader className="px-4 sm:px-6">
                <CardTitle className="text-base">Revenus</CardTitle>
                <CardDescription>Chiffre d'affaires quotidien</CardDescription>
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
                      tickFormatter={(value: number) => compactFormatter.format(value)}
                    />
                    <Tooltip
                      cursor={{ fill: "rgba(255,255,255,0.04)" }}
                      content={<ChartTooltip formatter={(value) => formatCurrency(value, currency)} />}
                    />
                    <Bar dataKey="value" name="Revenus" fill="#10b981" radius={[4, 4, 0, 0]} maxBarSize={40} />
                  </BarChart>
                </ResponsiveContainer>
              </CardContent>
            </Card>

            <Card className="gap-4 py-4 sm:py-6">
              <CardHeader className="px-4 sm:px-6">
                <CardTitle className="text-base">Trafic réseau</CardTitle>
                <CardDescription>Données échangées par jour</CardDescription>
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
                      name="Entrant"
                      type="monotone"
                      stroke="#10b981"
                      strokeWidth={2}
                      dot={false}
                      activeDot={{ r: 4 }}
                    />
                    <Line
                      dataKey="bytesOut"
                      name="Sortant"
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
                <CardTitle className="text-base">Ventes par profil</CardTitle>
                <CardDescription>Vouchers vendus et revenus générés</CardDescription>
              </CardHeader>
              <CardContent className="px-4 sm:px-6">
                {salesByProfile.length === 0 ? (
                  <p className="py-6 text-center text-sm text-muted-foreground">Aucune vente sur la période.</p>
                ) : (
                  <div className="space-y-4">
                    {salesByProfile.map((sale) => (
                      <div key={sale.name}>
                        <div className="flex items-baseline justify-between gap-3 text-sm">
                          <span className="truncate">{sale.name}</span>
                          <span className="shrink-0 text-xs text-muted-foreground">
                            <span className="font-medium text-foreground">{sale.count} vendus</span>
                            {" · "}
                            {formatCurrency(sale.revenue, currency)}
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
                <CardTitle className="text-base">Statut des vouchers</CardTitle>
                <CardDescription>Répartition du parc de vouchers</CardDescription>
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
                            {row.label}
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
