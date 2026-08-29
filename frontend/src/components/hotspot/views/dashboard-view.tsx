"use client";

import { useQuery } from "@tanstack/react-query";
import {
  Activity,
  Clock,
  Cog,
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
import { LoadingCards, LoadingRows } from "@/components/hotspot/loading";
import { PageHeader } from "@/components/hotspot/page-header";
import { StatCard } from "@/components/hotspot/stat-card";
import { StatusBadge } from "@/components/hotspot/status-badge";
import { api } from "@/lib/hotspot/api";
import { formatCurrency, timeAgo } from "@/lib/hotspot/format";
import type { Activity as ActivityItem, AppSettings, DashboardData, SiteOverview } from "@/lib/hotspot/types";

const GRID_STROKE = "#27272a";
const AXIS_TICK = { fill: "#71717a", fontSize: 12 };

function nf(value: number): string {
  return new Intl.NumberFormat("fr-FR").format(value);
}

function compactFr(value: number): string {
  return new Intl.NumberFormat("fr-FR", { notation: "compact", maximumFractionDigits: 1 }).format(value);
}

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
      <p className="mt-0.5 font-semibold text-white">
        {formatter ? formatter(value) : nf(value)}
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
function SiteCard({ site, currency }: { site: SiteOverview; currency: string }) {
  const online = site.status === "online";
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
            <dt className="text-xs text-muted-foreground">Sessions</dt>
            <dd className="mt-0.5 flex items-center gap-1.5 font-semibold tabular-nums">
              {nf(site.activeSessions)}
              {online && <span className="live-dot" aria-label="en direct" />}
            </dd>
          </div>
          <div>
            <dt className="text-xs text-muted-foreground">Ventes du jour</dt>
            <dd className="mt-0.5 font-semibold tabular-nums">{nf(site.salesToday)}</dd>
          </div>
          <div>
            <dt className="text-xs text-muted-foreground">Utilisateurs actifs</dt>
            <dd className="mt-0.5 font-semibold tabular-nums">{nf(site.hotspotUsers)}</dd>
          </div>
          <div>
            <dt className="text-xs text-muted-foreground">Vouchers actifs</dt>
            <dd className="mt-0.5 font-semibold tabular-nums">{nf(site.activeVouchers)}</dd>
          </div>
        </dl>

        <div className="mt-4 flex items-center justify-between gap-2 border-t pt-3">
          <span className="text-xs text-muted-foreground">Revenu 30 jours</span>
          <span className="text-sm font-semibold text-primary tabular-nums">
            {formatCurrency(site.revenue30d, currency)}
          </span>
        </div>
      </CardContent>
    </Card>
  );
}

export default function DashboardView() {
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

  if (isLoading) {
    return (
      <div className="space-y-6">
        <PageHeader title="Tableau de bord" description="Vue d'ensemble de votre réseau hotspot en temps réel" />
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
          title="Impossible de charger le tableau de bord"
          description="Le serveur est peut-être momentanément indisponible. Réessayez dans un instant."
          action={
            <Button variant="outline" onClick={() => void refetch()}>
              <Clock className="size-4" />
              Réessayer
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
      <PageHeader title="Tableau de bord" description="Vue d'ensemble de votre réseau hotspot en temps réel" />

      {/* KPIs */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-3">
        <StatCard title="Sessions actives" value={nf(kpis.activeSessions)} icon={Radio} live />
        <StatCard title="Utilisateurs" value={nf(kpis.totalUsers)} icon={Users} sub="clients hotspot" />
        <StatCard title="Vouchers actifs" value={nf(kpis.activeVouchers)} icon={Ticket} />
        <StatCard title="Ventes du jour" value={nf(kpis.salesToday)} icon={ShoppingCart} sub="tickets vendus" />
        <StatCard title="Revenu 30 jours" value={formatCurrency(kpis.revenue30d, currency)} icon={Wallet} />
        <StatCard
          title="Routeurs"
          value={`${kpis.routersOnline}/${kpis.routersTotal}`}
          icon={RouterIcon}
          sub="en ligne"
        />
      </div>

      {/* Vue d'ensemble multi-sites — 1 compte = N hotspots */}
      {data.sites.length > 0 && (
        <section aria-label="Vue d'ensemble multi-sites" className="space-y-3">
          <div className="flex flex-wrap items-baseline justify-between gap-2">
            <h2 className="text-sm font-semibold tracking-wide text-muted-foreground uppercase">
              Vue d'ensemble multi-sites
            </h2>
            <p className="text-xs text-muted-foreground">
              {data.sites.length} hotspot{data.sites.length > 1 ? "s" : ""} rattaché
              {data.sites.length > 1 ? "s" : ""} à votre compte
            </p>
          </div>
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-3">
            {data.sites.map((site) => (
              <SiteCard key={site.routerId} site={site} currency={currency} />
            ))}
          </div>
        </section>
      )}

      {/* Graphiques */}
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader className="px-4 sm:px-6">
            <CardTitle className="text-base">Sessions — dernières 24h</CardTitle>
          </CardHeader>
          <CardContent className="px-2 pb-2 sm:px-4 sm:pb-4">
            <div className="h-64 w-full sm:h-72">
              <ResponsiveContainer width="100%" height="100%">
                <AreaChart data={data.sessionsTimeline} margin={{ top: 8, right: 8, left: 0, bottom: 0 }}>
                  <CartesianGrid stroke={GRID_STROKE} strokeOpacity={0.6} vertical={false} />
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
                    cursor={{ stroke: GRID_STROKE }}
                    content={<ChartTooltip formatter={(v) => `${nf(v)} session${v > 1 ? "s" : ""}`} />}
                  />
                  <Area
                    type="monotone"
                    dataKey="value"
                    stroke="#10b981"
                    strokeWidth={2}
                    fill="#10b98122"
                    fillOpacity={1}
                  />
                </AreaChart>
              </ResponsiveContainer>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="px-4 sm:px-6">
            <CardTitle className="text-base">Revenus — 14 derniers jours</CardTitle>
          </CardHeader>
          <CardContent className="px-2 pb-2 sm:px-4 sm:pb-4">
            <div className="h-64 w-full sm:h-72">
              <ResponsiveContainer width="100%" height="100%">
                <BarChart data={data.revenueByDay} margin={{ top: 8, right: 8, left: 0, bottom: 0 }}>
                  <CartesianGrid stroke={GRID_STROKE} strokeOpacity={0.6} vertical={false} />
                  <XAxis dataKey="day" tick={AXIS_TICK} axisLine={false} tickLine={false} minTickGap={16} />
                  <YAxis
                    tick={AXIS_TICK}
                    axisLine={false}
                    tickLine={false}
                    width={44}
                    tickFormatter={(v: number) => compactFr(v)}
                  />
                  <Tooltip
                    cursor={{ fill: "#10b981", fillOpacity: 0.08 }}
                    content={<ChartTooltip formatter={(v) => formatCurrency(v, currency)} />}
                  />
                  <Bar dataKey="value" fill="#10b981" radius={[4, 4, 0, 0]} maxBarSize={36} />
                </BarChart>
              </ResponsiveContainer>
            </div>
          </CardContent>
        </Card>
      </div>

      {/* Profils + activité */}
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader className="px-4 sm:px-6">
            <CardTitle className="text-base">Profils les plus vendus</CardTitle>
          </CardHeader>
          <CardContent className="px-4 pb-4 sm:px-6 sm:pb-6">
            {data.topProfiles.length === 0 ? (
              <EmptyState
                icon={Ticket}
                title="Aucune vente enregistrée"
                description="Les profils les plus vendus apparaîtront ici."
              />
            ) : (
              <div className="space-y-4">
                {data.topProfiles.map((p) => (
                  <div key={p.name}>
                    <div className="flex items-baseline justify-between gap-3">
                      <span className="truncate text-sm font-medium">{p.name}</span>
                      <span className="shrink-0 text-xs text-muted-foreground">
                        {nf(p.users)} utilisateur{p.users > 1 ? "s" : ""} · {formatCurrency(p.total, currency)}
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
            <CardTitle className="text-base">Activité récente</CardTitle>
          </CardHeader>
          <CardContent className="px-4 pb-4 sm:px-6 sm:pb-6">
            {data.recentActivity.length === 0 ? (
              <EmptyState icon={Activity} title="Aucune activité récente" />
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
                        <p className="mt-0.5 text-xs text-muted-foreground">{timeAgo(a.at)}</p>
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
