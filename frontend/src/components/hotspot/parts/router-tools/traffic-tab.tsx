"use client";

// Onglet F6 — trafic temps réel : graphique aérien par interface (poll 5 s,
// historique ~5 min), sélection d'interface, mise à jour manuelle.
// Transfert pur depuis router-tools.tsx.

import { useMemo, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Activity, ArrowDown, ArrowUp, Clock, Loader2, Network, RefreshCw } from "lucide-react";
import { toast } from "sonner";
import {
  Area,
  AreaChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis
} from "recharts";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow
} from "@/components/ui/table";
import { EmptyState } from "@/components/hotspot/empty-state";
import { ChartTooltip } from "@/components/hotspot/parts/sd-chart-tooltip";
import { cn } from "@/lib/utils";
import { api } from "@/lib/hotspot/api";
import { localeOf, useI18n } from "@/lib/hotspot/i18n";
import { useChartPalette } from "@/lib/hotspot/chart-theme";
import { formatBitsPerSec, formatBytes, timeAgo } from "@/lib/hotspot/format";
import type { RouterDevice, RouterTraffic } from "@/lib/hotspot/types";
import { MAX_SAMPLES, ToolError, UnsupportedState, shortBits, shortClock } from "./shared";

// ─── F6 — Trafic temps réel ───

interface TrafficSample {
  label: string;
  rxBps: number;
  txBps: number;
}

export function TrafficTab({ router }: { router: RouterDevice }) {
  const { t, tf, lang } = useI18n();
  const charts = useChartPalette();
  const RX_COLOR = charts.series[0]; // émeraude — réception (descendant)
  const TX_COLOR = charts.series[2]; // ambre — émission (montant)
  const GRID_STROKE = charts.grid;
  const AXIS_TICK = { fill: charts.axis, fontSize: 11 };
  const queryClient = useQueryClient();
  const [iface, setIface] = useState("all");

  // Échantillonnage local des débits PAR interface : le serveur ne fournit
  // l'historique que pour la somme de toutes les interfaces (contrat F6).
  // L'accumulation se fait dans le queryFn (jamais pendant le rendu),
  // dédupliquée par updatedAt.
  const [samples, setSamples] = useState<Record<string, TrafficSample[]>>({});
  const stampRef = useRef("");

  const { data, isLoading, isError, error, refetch } = useQuery({
    queryKey: ["/api/routers", router.id, "traffic"],
    queryFn: async () => {
      const traffic = await api<RouterTraffic>(`/api/routers/${router.id}/traffic`);
      if (traffic.updatedAt && traffic.updatedAt !== stampRef.current) {
        stampRef.current = traffic.updatedAt;
        const label = new Date().toLocaleTimeString(localeOf(lang), {
          hour: "2-digit",
          minute: "2-digit",
          second: "2-digit",
        });
        setSamples((prev) => {
          const next: Record<string, TrafficSample[]> = {};
          for (const it of traffic.interfaces) {
            next[it.name] = [...(prev[it.name] ?? []), { label, rxBps: it.rxBps, txBps: it.txBps }].slice(-MAX_SAMPLES);
          }
          return next;
        });
      }
      return traffic;
    },
    enabled: router.mode !== "real",
    refetchInterval: 5_000,
  });

  const syncMutation = useMutation({
    mutationFn: () => api<{ ok?: boolean; queued?: boolean }>(`/api/routers/${router.id}/refresh`, { method: "POST" }),
    onSuccess: () => {
      toast.success(t("tools.traffic.syncToast"), {
        description: t("tools.traffic.syncToastDesc"),
      });
      void queryClient.invalidateQueries({ queryKey: ["/api/routers", router.id, "traffic"] });
    },
    onError: (err: Error) => toast.error(err.message),
  });

  const interfaces = data?.interfaces ?? [];
  const totalRx = interfaces.reduce((acc, i) => acc + i.rxBps, 0);
  const totalTx = interfaces.reduce((acc, i) => acc + i.txBps, 0);
  const current = iface === "all" ? null : interfaces.find((i) => i.name === iface);
  const rx = current ? current.rxBps : totalRx;
  const tx = current ? current.txBps : totalTx;

  const chartData = useMemo(() => {
    if (iface === "all") {
      return (data?.history ?? []).map((p) => ({ label: shortClock(p.t, lang), rxBps: p.rxBps, txBps: p.txBps }));
    }
    return samples[iface] ?? [];
  }, [data, iface, samples, lang]);

  const hasNoData = !isLoading && !isError && interfaces.length === 0 && (data?.history?.length ?? 0) === 0;

  if (router.mode === "real") {
    return <UnsupportedState />;
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h3 className="text-sm font-semibold">{t("tools.traffic.title")}</h3>
          <p className="mt-0.5 text-xs text-muted-foreground">{t("tools.traffic.desc")}</p>
        </div>
        <Select value={iface} onValueChange={setIface}>
          <SelectTrigger className="h-9 w-full sm:w-56" aria-label={t("tools.traffic.ifaceLabel")}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">{t("tools.traffic.all")}</SelectItem>
            {interfaces.map((it) => (
              <SelectItem key={it.name} value={it.name}>
                {it.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      {isLoading ? (
        <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
          {Array.from({ length: 4 }).map((_, i) => (
            <Skeleton key={i} className="h-[74px] rounded-lg" />
          ))}
        </div>
      ) : isError ? (
        <ToolError error={error} onRetry={() => void refetch()} />
      ) : hasNoData ? (
        <EmptyState
          icon={Activity}
          title={t("tools.traffic.empty")}
          description={
            router.mode === "agent" ? t("tools.traffic.emptyAgent") : t("tools.traffic.emptySim")
          }
          action={
            router.mode === "agent" ? (
              <Button variant="outline" disabled={syncMutation.isPending} onClick={() => syncMutation.mutate()}>
                {syncMutation.isPending ? <Loader2 className="size-4 animate-spin" /> : <RefreshCw className="size-4" />}
                {t("tools.traffic.sync")}
              </Button>
            ) : undefined
          }
        />
      ) : (
        <>
          {/* KPIs */}
          <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
            <div className="rounded-lg border p-3">
              <p className="flex items-center gap-1.5 text-xs text-muted-foreground">
                <ArrowDown className="size-3.5" aria-hidden /> {t("tools.traffic.rx")}
              </p>
              <p className="mt-1 text-lg font-semibold tabular-nums text-emerald-600 dark:text-emerald-400">
                {formatBitsPerSec(rx)}
              </p>
            </div>
            <div className="rounded-lg border p-3">
              <p className="flex items-center gap-1.5 text-xs text-muted-foreground">
                <ArrowUp className="size-3.5" aria-hidden /> {t("tools.traffic.tx")}
              </p>
              <p className="mt-1 text-lg font-semibold tabular-nums text-amber-600 dark:text-amber-400">
                {formatBitsPerSec(tx)}
              </p>
            </div>
            <div className="rounded-lg border p-3">
              <p className="flex items-center gap-1.5 text-xs text-muted-foreground">
                <Network className="size-3.5" aria-hidden /> {t("tools.traffic.interfaces")}
              </p>
              <p className="mt-1 text-lg font-semibold tabular-nums">{interfaces.length}</p>
            </div>
            <div className="rounded-lg border p-3">
              <p className="flex items-center gap-1.5 text-xs text-muted-foreground">
                <Clock className="size-3.5" aria-hidden /> {t("tools.traffic.lastMeasure")}
              </p>
              <p className="mt-1 text-lg font-semibold tabular-nums">
                {data?.updatedAt ? timeAgo(data.updatedAt, lang) : "—"}
              </p>
            </div>
          </div>

          {/* Graphique rx/tx */}
          <div className="rounded-lg border p-3 sm:p-4">
            <div className="h-56 w-full sm:h-64" role="img" aria-label={t("tools.traffic.chartAria")}>
              <ResponsiveContainer width="100%" height="100%">
                <AreaChart data={chartData} margin={{ top: 8, right: 8, left: 0, bottom: 0 }}>
                  <CartesianGrid stroke={GRID_STROKE} strokeOpacity={0.6} vertical={false} />
                  <XAxis
                    dataKey="label"
                    tick={AXIS_TICK}
                    axisLine={false}
                    tickLine={false}
                    interval="preserveStartEnd"
                    minTickGap={48}
                  />
                  <YAxis
                    tick={AXIS_TICK}
                    axisLine={false}
                    tickLine={false}
                    width={44}
                    tickFormatter={(v: number) => shortBits(v)}
                  />
                  <Tooltip cursor={{ stroke: GRID_STROKE }} content={<ChartTooltip formatter={(v) => formatBitsPerSec(v)} />} />
                  <Area
                    type="monotone"
                    dataKey="rxBps"
                    name={t("tools.traffic.rxName")}
                    stroke={RX_COLOR}
                    strokeWidth={2}
                    fill={RX_COLOR}
                    fillOpacity={0.12}
                    connectNulls
                  />
                  <Area
                    type="monotone"
                    dataKey="txBps"
                    name={t("tools.traffic.txName")}
                    stroke={TX_COLOR}
                    strokeWidth={2}
                    fill={TX_COLOR}
                    fillOpacity={0.12}
                    connectNulls
                  />
                </AreaChart>
              </ResponsiveContainer>
            </div>
            {iface !== "all" && chartData.length < 2 && (
              <p className="mt-2 text-center text-xs text-muted-foreground">
                {tf("tools.traffic.collecting", { name: iface })}
              </p>
            )}
          </div>

          {/* Détail courant par interface */}
          {interfaces.length > 0 && (
            <div className="max-h-44 overflow-y-auto rounded-lg border">
              <Table>
                <TableHeader>
                  <TableRow className="hover:bg-transparent">
                    <TableHead className="pl-4 text-muted-foreground">{t("tools.traffic.iface")}</TableHead>
                    <TableHead className="text-right text-muted-foreground">Rx</TableHead>
                    <TableHead className="text-right text-muted-foreground">Tx</TableHead>
                    <TableHead className="hidden text-right text-muted-foreground sm:table-cell">{t("tools.traffic.rxTotal")}</TableHead>
                    <TableHead className="hidden pr-4 text-right text-muted-foreground sm:table-cell">{t("tools.traffic.txTotal")}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {interfaces.map((it) => (
                    <TableRow key={it.name} className={cn(iface === it.name && "bg-accent/40")}>
                      <TableCell className="pl-4 font-mono text-[13px] font-medium">{it.name}</TableCell>
                      <TableCell className="text-right tabular-nums">{formatBitsPerSec(it.rxBps)}</TableCell>
                      <TableCell className="text-right tabular-nums">{formatBitsPerSec(it.txBps)}</TableCell>
                      <TableCell className="hidden text-right tabular-nums text-muted-foreground sm:table-cell">
                        {formatBytes(it.rxBytes, lang)}
                      </TableCell>
                      <TableCell className="hidden pr-4 text-right tabular-nums text-muted-foreground sm:table-cell">
                        {formatBytes(it.txBytes, lang)}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          )}
        </>
      )}
    </div>
  );
}
