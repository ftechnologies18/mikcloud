"use client";

// Outils routeur (vague P1 — CONTRACT-V2) : F6 trafic temps réel, F7 IP bindings,
// F9 DHCP/hôtes/cookies/journal, F8+F10 système (infos, ping, scheduler,
// reboot/shutdown). Le dialogue est monté conditionnellement par routers-view
// (pattern Next 16 : dialogs montés avec leur état initial, aucun setState en effect).

import { useMemo, useRef, useState } from "react";
import type { ReactNode } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Activity,
  ArrowDown,
  ArrowUp,
  CircuitBoard,
  Clock,
  Cookie,
  Cpu,
  FileText,
  HardDrive,
  Loader2,
  MemoryStick,
  Network,
  Plus,
  Power,
  RefreshCw,
  RotateCcw,
  Router as RouterIcon,
  Trash2,
  TriangleAlert,
  Users,
  Wrench,
  Zap,
} from "lucide-react";
import { toast } from "sonner";
import {
  Area,
  AreaChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { EmptyState } from "@/components/hotspot/empty-state";
import { ChartTooltip } from "@/components/hotspot/parts/sd-chart-tooltip";
import { StatusBadge } from "@/components/hotspot/status-badge";
import { cn } from "@/lib/utils";
import { api, ApiError } from "@/lib/hotspot/api";
import { localeOf, t as translate, useI18n } from "@/lib/hotspot/i18n";
import type { Lang } from "@/lib/hotspot/i18n";
import { useChartPalette } from "@/lib/hotspot/chart-theme";
import { formatBitsPerSec, formatBytes, formatDuration, formatMb, timeAgo } from "@/lib/hotspot/format";
import type {
  CommandStatus,
  DhcpLeaseRow,
  HotspotCookieRow,
  HotspotHostRow,
  IPBinding,
  IPBindingType,
  PingResult,
  RouterDevice,
  RouterStats,
  RouterTraffic,
  RouterLogRow,
  SchedulerRow,
  ToolEnvelope,
} from "@/lib/hotspot/types";

// ─── Constantes ───

// Palette thématée (nuit/jour) — injectée par TrafficTab via useChartPalette().

const MAC_RE = /^([0-9A-Fa-f]{2}:){5}[0-9A-Fa-f]{2}$/;
const INTERVAL_RE = /^\d+[smhdw]$/; // format RouterOS : 45s, 5m, 1h, 2d, 1w
const MAX_SAMPLES = 60; // ~5 minutes d'historique par interface (poll 5 s)

const sleep = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms));

/** Heure locale courte « 14:05:32 ». */
function shortClock(iso: string, lang: Lang): string {
  const d = new Date(iso);
  if (isNaN(d.getTime())) return iso;
  return d.toLocaleTimeString(localeOf(lang), { hour: "2-digit", minute: "2-digit", second: "2-digit" });
}

/** Version ultra compacte pour l'axe Y du graphique (« 20 M », « 500 K »). */
function shortBits(v: number): string {
  if (v >= 1_000_000_000) return `${Math.round(v / 1_000_000_000)} G`;
  if (v >= 1_000_000) return `${Math.round(v / 1_000_000)} M`;
  if (v >= 1_000) return `${Math.round(v / 1_000)} K`;
  return String(Math.round(v));
}

/** Latence « 12,3 ms » (entier au-delà de 100 ms). */
function fmtMs(v: number): string {
  return `${v >= 100 ? Math.round(v) : v.toFixed(1).replace(".", ",")} ms`;
}

// ─── Enveloppe des outils (F9/F10) ───

/**
 * Récupère une enveloppe {queued, data, updatedAt} — tolère aussi un simple
 * tableau (défense contre les variantes de forme du backend).
 */
async function fetchToolEnvelope<T>(path: string): Promise<ToolEnvelope<T>> {
  const raw = await api<ToolEnvelope<T> | T[]>(path);
  if (Array.isArray(raw)) return { queued: false, data: raw, updatedAt: "" };
  if (raw && typeof raw === "object") {
    return {
      queued: raw.queued ?? false,
      data: raw.data ?? [],
      updatedAt: raw.updatedAt ?? "",
    };
  }
  return { queued: false, data: [], updatedAt: "" };
}

// ─── États partagés ───

function UnsupportedState() {
  const { t } = useI18n();
  return (
    <EmptyState
      icon={TriangleAlert}
      title={t("tools.unsupportedTitle")}
      description={t("tools.unsupportedDesc")}
    />
  );
}

function ToolError({ error, onRetry }: { error: Error | null; onRetry: () => void }) {
  const { t } = useI18n();
  if (error instanceof ApiError && error.status === 400) return <UnsupportedState />;
  return (
    <EmptyState
      icon={TriangleAlert}
      title={t("tools.loadError")}
      description={error?.message ?? t("tools.loadErrorDefault")}
      action={
        <Button variant="outline" onClick={onRetry}>
          <RefreshCw className="size-4" />
          {t("common.retry")}
        </Button>
      }
    />
  );
}

function ToolSkeleton({ rows = 4 }: { rows?: number }) {
  return (
    <div className="space-y-2.5 rounded-lg border p-4">
      {Array.from({ length: rows }).map((_, i) => (
        <div key={i} className="flex items-center gap-3">
          <Skeleton className="h-4 flex-1" />
          <Skeleton className="hidden h-4 w-24 sm:block" />
          <Skeleton className="h-4 w-14" />
        </div>
      ))}
    </div>
  );
}

/** Bandeau « file d'attente agent » commun (F9/F10). */
function QueuedBanner() {
  const { t } = useI18n();
  return (
    <p
      className="flex items-center gap-2 rounded-lg border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-sm text-amber-700 dark:text-amber-400"
      role="status"
      aria-live="polite"
    >
      <Loader2 className="size-4 shrink-0 animate-spin" />
      {t("tools.queued")}
    </p>
  );
}

// ─── F6 — Trafic temps réel ───

interface TrafficSample {
  label: string;
  rxBps: number;
  txBps: number;
}

function TrafficTab({ router }: { router: RouterDevice }) {
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

// ─── F7 — IP bindings ───

const BINDING_TYPE_BADGES: Record<IPBindingType, { labelKey: string; className: string }> = {
  bypassed: { labelKey: "tools.bindings.bypassed", className: "border-emerald-500/25 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400" },
  blocked: { labelKey: "tools.bindings.blocked", className: "border-destructive/25 bg-destructive/10 text-destructive" },
};

function AddBindingDialog({ router, onClose }: { router: RouterDevice; onClose: () => void }) {
  const { t, tf } = useI18n();
  const queryClient = useQueryClient();
  const [mac, setMac] = useState("");
  const [address, setAddress] = useState("");
  const [comment, setComment] = useState("");
  const [type, setType] = useState<IPBindingType>("bypassed");

  const macOk = MAC_RE.test(mac.trim());
  const valid = macOk;

  const createMutation = useMutation({
    mutationFn: () =>
      api<IPBinding & { queued?: boolean }>(`/api/routers/${router.id}/ipbindings`, {
        method: "POST",
        body: {
          mac: mac.trim(),
          address: address.trim() || undefined,
          comment: comment.trim(),
          type,
        },
      }),
    onSuccess: (res) => {
      toast.success(
        res.queued ? t("tools.bindings.queuedCreate") : t("tools.bindings.addedToast"),
        {
          description: `${mac.trim()} · ${t(BINDING_TYPE_BADGES[type].labelKey).toLowerCase()}`,
        },
      );
      onClose();
      void queryClient.invalidateQueries({ queryKey: ["/api/routers", router.id, "ipbindings"] });
    },
    onError: (err: Error) => toast.error(err.message),
  });

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{t("tools.bindings.addTitle")}</DialogTitle>
          <DialogDescription>
            {tf("tools.bindings.addDesc", { name: router.name })}
          </DialogDescription>
        </DialogHeader>

        <form
          className="space-y-4"
          onSubmit={(e) => {
            e.preventDefault();
            if (!valid || createMutation.isPending) return;
            createMutation.mutate();
          }}
        >
          <div className="space-y-2">
            <Label htmlFor="binding-mac">{t("tools.bindings.mac")}</Label>
            <Input
              id="binding-mac"
              className="font-mono"
              placeholder="AA:BB:CC:DD:EE:FF"
              value={mac}
              onChange={(e) => setMac(e.target.value)}
              aria-invalid={mac.length > 0 && !macOk}
              disabled={createMutation.isPending}
              autoFocus
            />
            {mac.length > 0 && !macOk && (
              <p className="text-xs text-destructive" role="alert">
                {t("tools.bindings.macFormat")}
              </p>
            )}
          </div>

          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="binding-address">{t("tools.bindings.address")}</Label>
              <Input
                id="binding-address"
                className="font-mono"
                placeholder="192.168.88.10"
                value={address}
                onChange={(e) => setAddress(e.target.value)}
                disabled={createMutation.isPending}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="binding-type">{t("tools.bindings.type")}</Label>
              <Select value={type} onValueChange={(v) => setType(v as IPBindingType)} disabled={createMutation.isPending}>
                <SelectTrigger id="binding-type" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="bypassed">{t("tools.bindings.typeBypassed")}</SelectItem>
                  <SelectItem value="blocked">{t("tools.bindings.typeBlocked")}</SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>

          <div className="space-y-2">
            <Label htmlFor="binding-comment">{t("tools.bindings.comment")}</Label>
            <Input
              id="binding-comment"
              placeholder={t("tools.bindings.commentPlaceholder")}
              value={comment}
              onChange={(e) => setComment(e.target.value)}
              disabled={createMutation.isPending}
            />
          </div>

          {createMutation.isError && (
            <p className="text-sm text-destructive" role="alert">
              {createMutation.error.message}
            </p>
          )}

          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose} disabled={createMutation.isPending}>
              {t("common.cancel")}
            </Button>
            <Button type="submit" disabled={!valid || createMutation.isPending}>
              {createMutation.isPending && <Loader2 className="size-4 animate-spin" />}
              {t("common.add")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function IpBindingsTab({ router }: { router: RouterDevice }) {
  const { t, tf } = useI18n();
  const queryClient = useQueryClient();
  const [adding, setAdding] = useState(false);
  const [deleting, setDeleting] = useState<IPBinding | null>(null);

  const { data, isLoading, isError, error, refetch } = useQuery({
    queryKey: ["/api/routers", router.id, "ipbindings"],
    queryFn: () => api<IPBinding[]>(`/api/routers/${router.id}/ipbindings`),
    enabled: router.mode !== "real",
  });

  const bindings = data ?? [];

  const invalidate = () => queryClient.invalidateQueries({ queryKey: ["/api/routers", router.id, "ipbindings"] });

  const toggleMutation = useMutation({
    mutationFn: (binding: IPBinding) =>
      api<IPBinding>(`/api/ipbindings/${binding.id}`, { method: "PUT", body: { disabled: !binding.disabled } }),
    onSuccess: (updated) => {
      toast.success(updated.disabled ? t("tools.bindings.disabledToast") : t("tools.bindings.enabledToast"));
      void invalidate();
    },
    onError: (err: Error) => toast.error(err.message),
  });

  const deleteMutation = useMutation({
    mutationFn: (binding: IPBinding) => api<{ ok: boolean }>(`/api/ipbindings/${binding.id}`, { method: "DELETE" }),
    onSuccess: () => {
      toast.success(t("tools.bindings.deletedToast"));
      setDeleting(null);
      void invalidate();
    },
    onError: (err: Error) => toast.error(err.message),
  });

  if (router.mode === "real") {
    return <UnsupportedState />;
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h3 className="text-sm font-semibold">{t("tools.bindings.title")}</h3>
          <p className="mt-0.5 text-xs text-muted-foreground">{t("tools.bindings.desc")}</p>
        </div>
        <Button size="sm" className="h-9" onClick={() => setAdding(true)}>
          <Plus className="size-4" />
          {t("common.add")}
        </Button>
      </div>

      {isLoading ? (
        <ToolSkeleton rows={4} />
      ) : isError ? (
        <ToolError error={error} onRetry={() => void refetch()} />
      ) : bindings.length === 0 ? (
        <EmptyState
          icon={Network}
          title={t("tools.bindings.empty")}
          description={t("tools.bindings.emptyDesc")}
          action={
            <Button variant="outline" onClick={() => setAdding(true)}>
              <Plus className="size-4" />
              {t("tools.bindings.addBinding")}
            </Button>
          }
        />
      ) : (
        <div className="max-h-80 overflow-y-auto rounded-lg border">
          <Table>
            <TableHeader>
              <TableRow className="hover:bg-transparent">
                <TableHead className="pl-4 text-muted-foreground">{t("common.mac")}</TableHead>
                <TableHead className="text-muted-foreground">{t("tools.bindings.addressCol")}</TableHead>
                <TableHead className="text-muted-foreground">{t("common.type")}</TableHead>
                <TableHead className="hidden text-muted-foreground md:table-cell">{t("tools.bindings.commentCol")}</TableHead>
                <TableHead className="text-muted-foreground">{t("common.status")}</TableHead>
                <TableHead className="pr-4 text-right text-muted-foreground">{t("common.actions")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {bindings.map((binding) => (
                <TableRow key={binding.id}>
                  <TableCell className="pl-4 font-mono text-[13px] font-medium">{binding.mac}</TableCell>
                  <TableCell className="font-mono text-[13px] text-muted-foreground">{binding.address || "—"}</TableCell>
                  <TableCell>
                    <Badge variant="outline" className={BINDING_TYPE_BADGES[binding.type]?.className ?? ""}>
                      {BINDING_TYPE_BADGES[binding.type] ? t(BINDING_TYPE_BADGES[binding.type].labelKey) : binding.type}
                    </Badge>
                  </TableCell>
                  <TableCell className="hidden max-w-40 truncate text-muted-foreground md:table-cell" title={binding.comment}>
                    {binding.comment || "—"}
                  </TableCell>
                  <TableCell>
                    {binding.disabled ? (
                      <Badge variant="outline" className="border-border bg-muted text-muted-foreground">{t("tools.bindings.inactive")}</Badge>
                    ) : (
                      <Badge variant="outline" className="border-primary/25 bg-primary/10 text-primary">{t("tools.bindings.active")}</Badge>
                    )}
                  </TableCell>
                  <TableCell className="pr-4">
                    <div className="flex items-center justify-end gap-2">
                      <Switch
                        checked={!binding.disabled}
                        disabled={toggleMutation.isPending && toggleMutation.variables?.id === binding.id}
                        onCheckedChange={() => toggleMutation.mutate(binding)}
                        aria-label={tf("tools.bindings.toggleAria", {
                          action: binding.disabled ? t("common.activate") : t("common.deactivate"),
                          mac: binding.mac,
                        })}
                      />
                      <Button
                        variant="ghost"
                        size="icon"
                        className="size-8 text-muted-foreground hover:text-destructive"
                        onClick={() => setDeleting(binding)}
                        aria-label={tf("tools.bindings.deleteAria", { mac: binding.mac })}
                      >
                        <Trash2 className="size-4" />
                      </Button>
                    </div>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}

      {adding && <AddBindingDialog router={router} onClose={() => setAdding(false)} />}

      <AlertDialog open={deleting !== null} onOpenChange={(open) => !open && setDeleting(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t("tools.bindings.deleteTitle")}</AlertDialogTitle>
            <AlertDialogDescription>
              {deleting
                ? `${tf("tools.bindings.deleteDesc", { mac: deleting.mac, router: router.name })}${
                    deleting.type === "bypassed" ? t("tools.bindings.deleteExtra") : ""
                  }.`
                : ""}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleteMutation.isPending}>{t("common.cancel")}</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-white hover:bg-destructive/90"
              disabled={deleteMutation.isPending}
              onClick={(e) => {
                e.preventDefault();
                if (deleting) deleteMutation.mutate(deleting);
              }}
            >
              {deleteMutation.isPending && <Loader2 className="size-4 animate-spin" />}
              {t("common.delete")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

// ─── F9 — DHCP · Hôtes · Cookies · Journal ───

function DhcpTable({ rows }: { rows: DhcpLeaseRow[] }) {
  const { t } = useI18n();
  return (
    <Table>
      <TableHeader>
        <TableRow className="hover:bg-transparent">
          <TableHead className="pl-4 text-muted-foreground">{t("common.ip")}</TableHead>
          <TableHead className="text-muted-foreground">{t("common.mac")}</TableHead>
          <TableHead className="text-muted-foreground">{t("tools.dhcp.host")}</TableHead>
          <TableHead className="hidden text-muted-foreground sm:table-cell">{t("tools.dhcp.expires")}</TableHead>
          <TableHead className="pr-4 text-muted-foreground">{t("common.status")}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {rows.map((row, i) => (
          <TableRow key={`${row.mac}-${row.ip}-${i}`}>
            <TableCell className="pl-4 font-mono text-[13px] font-medium">{row.ip || "—"}</TableCell>
            <TableCell className="font-mono text-[13px] text-muted-foreground">{row.mac || "—"}</TableCell>
            <TableCell className="max-w-32 truncate" title={row.host}>
              {row.host || "—"}
            </TableCell>
            <TableCell className="hidden whitespace-nowrap text-muted-foreground sm:table-cell">{row.expires || "—"}</TableCell>
            <TableCell className="pr-4">
              <Badge
                variant="outline"
                className={
                  row.status?.toLowerCase() === "bound"
                    ? "border-emerald-500/25 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400"
                    : "border-border bg-muted text-muted-foreground"
                }
              >
                {row.status || "—"}
              </Badge>
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}

function HostsTable({ rows }: { rows: HotspotHostRow[] }) {
  const { t } = useI18n();
  return (
    <Table>
      <TableHeader>
        <TableRow className="hover:bg-transparent">
          <TableHead className="pl-4 text-muted-foreground">{t("common.mac")}</TableHead>
          <TableHead className="text-muted-foreground">{t("common.ip")}</TableHead>
          <TableHead className="hidden text-muted-foreground md:table-cell">{t("tools.hosts.server")}</TableHead>
          <TableHead className="text-muted-foreground">{t("tools.hosts.uptime")}</TableHead>
          <TableHead className="pr-4 text-muted-foreground">{t("tools.hosts.authorized")}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {rows.map((row, i) => (
          <TableRow key={`${row.mac}-${row.ip}-${i}`}>
            <TableCell className="pl-4 font-mono text-[13px] font-medium">{row.mac || "—"}</TableCell>
            <TableCell className="font-mono text-[13px] text-muted-foreground">{row.ip || "—"}</TableCell>
            <TableCell className="hidden max-w-32 truncate text-muted-foreground md:table-cell" title={row.server}>
              {row.server || "—"}
            </TableCell>
            <TableCell className="whitespace-nowrap tabular-nums">{formatDuration(row.uptime)}</TableCell>
            <TableCell className="pr-4">
              {row.authorized ? (
                <Badge variant="outline" className="border-emerald-500/25 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400">
                  {t("tools.hosts.authorized")}
                </Badge>
              ) : (
                <Badge variant="outline" className="border-border bg-muted text-muted-foreground">
                  {t("tools.hosts.unauthorized")}
                </Badge>
              )}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}

function CookiesTable({ rows }: { rows: HotspotCookieRow[] }) {
  const { t } = useI18n();
  return (
    <Table>
      <TableHeader>
        <TableRow className="hover:bg-transparent">
          <TableHead className="pl-4 text-muted-foreground">{t("common.user")}</TableHead>
          <TableHead className="text-muted-foreground">{t("common.mac")}</TableHead>
          <TableHead className="pr-4 text-muted-foreground">{t("tools.cookies.expires")}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {rows.map((row, i) => (
          <TableRow key={`${row.user}-${row.mac}-${i}`}>
            <TableCell className="pl-4 font-mono text-[13px] font-medium">{row.user || "—"}</TableCell>
            <TableCell className="font-mono text-[13px] text-muted-foreground">{row.mac || "—"}</TableCell>
            <TableCell className="pr-4 whitespace-nowrap text-muted-foreground">{row.expires || "—"}</TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}

function LogTable({ rows }: { rows: RouterLogRow[] }) {
  const { t } = useI18n();
  return (
    <Table>
      <TableHeader>
        <TableRow className="hover:bg-transparent">
          <TableHead className="pl-4 whitespace-nowrap text-muted-foreground">{t("tools.log.time")}</TableHead>
          <TableHead className="text-muted-foreground">{t("tools.log.topics")}</TableHead>
          <TableHead className="pr-4 text-muted-foreground">{t("tools.log.message")}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {rows.map((row, i) => (
          <TableRow key={`${row.time}-${i}`}>
            <TableCell className="pl-4 whitespace-nowrap font-mono text-[13px] tabular-nums text-muted-foreground">
              {row.time || "—"}
            </TableCell>
            <TableCell>
              <div className="flex max-w-32 flex-wrap gap-1">
                {(row.topics ?? "")
                  .split(/[,; ]+/)
                  .filter(Boolean)
                  .map((topic) => (
                    <Badge key={topic} variant="outline" className="border-border bg-muted px-1.5 text-[10px] text-muted-foreground">
                      {topic}
                    </Badge>
                  ))}
              </div>
            </TableCell>
            <TableCell className="pr-4 font-mono text-xs break-all">{row.message || "—"}</TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}

function ToolSection<T>({
  router,
  kind,
  title,
  description,
  emptyTitle,
  emptyDescription,
  children,
}: {
  router: RouterDevice;
  kind: "dhcp" | "hosts" | "cookies" | "log";
  title: string;
  description: string;
  emptyTitle: string;
  emptyDescription: string;
  children: (rows: T[]) => ReactNode;
}) {
  const { t, tf, lang } = useI18n();
  const { data, isLoading, isError, error, refetch, isFetching } = useQuery({
    queryKey: ["/api/routers", router.id, kind],
    queryFn: () => fetchToolEnvelope<T>(`/api/routers/${router.id}/${kind}`),
    enabled: router.mode !== "real",
    // Poll 3 s tant que la commande agent est en file, puis arrêt.
    refetchInterval: (query) => (query.state.data?.queued ? 3_000 : false),
  });

  const rows = data?.data ?? [];

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div>
          <h3 className="text-sm font-semibold">{title}</h3>
          <p className="mt-0.5 text-xs text-muted-foreground">{description}</p>
        </div>
        <div className="flex items-center gap-2">
          {data?.updatedAt && (
            <span className="hidden text-xs text-muted-foreground sm:inline">
              {tf("tools.updatedAgo", { ago: timeAgo(data.updatedAt, lang) })}
            </span>
          )}
          <Button
            variant="outline"
            size="sm"
            className="h-8"
            onClick={() => void refetch()}
            disabled={isFetching}
            aria-label={tf("tools.refreshAria", { title })}
          >
            <RefreshCw className={cn("size-3.5", isFetching && "animate-spin")} />
            {t("tools.refresh")}
          </Button>
        </div>
      </div>

      {data?.queued && <QueuedBanner />}

      {isLoading ? (
        <ToolSkeleton rows={5} />
      ) : isError ? (
        <ToolError error={error} onRetry={() => void refetch()} />
      ) : rows.length === 0 && !data?.queued ? (
        <EmptyState icon={TriangleAlert} title={emptyTitle} description={emptyDescription} />
      ) : rows.length === 0 ? (
        <ToolSkeleton rows={5} />
      ) : (
        <div className="max-h-80 overflow-y-auto rounded-lg border">{children(rows)}</div>
      )}
    </div>
  );
}

function ToolsTab({ router }: { router: RouterDevice }) {
  const { t } = useI18n();
  if (router.mode === "real") {
    return <UnsupportedState />;
  }
  return (
    <Tabs defaultValue="dhcp" className="gap-0">
      <div className="overflow-x-auto pb-3">
        <TabsList className="w-full min-w-max sm:w-fit">
          <TabsTrigger value="dhcp">
            <RouterIcon className="size-4" />
            {t("tools.dhcp")}
          </TabsTrigger>
          <TabsTrigger value="hosts">
            <Users className="size-4" />
            {t("tools.hosts")}
          </TabsTrigger>
          <TabsTrigger value="cookies">
            <Cookie className="size-4" />
            {t("tools.cookies")}
          </TabsTrigger>
          <TabsTrigger value="log">
            <FileText className="size-4" />
            {t("tools.log")}
          </TabsTrigger>
        </TabsList>
      </div>

      <TabsContent value="dhcp" className="pt-1">
        <ToolSection<DhcpLeaseRow>
          router={router}
          kind="dhcp"
          title={t("tools.dhcp.title")}
          description={t("tools.dhcp.desc")}
          emptyTitle={t("tools.dhcp.empty")}
          emptyDescription={t("tools.dhcp.emptyDesc")}
        >
          {(rows) => <DhcpTable rows={rows} />}
        </ToolSection>
      </TabsContent>

      <TabsContent value="hosts" className="pt-1">
        <ToolSection<HotspotHostRow>
          router={router}
          kind="hosts"
          title={t("tools.hosts.title")}
          description={t("tools.hosts.desc")}
          emptyTitle={t("tools.hosts.empty")}
          emptyDescription={t("tools.hosts.emptyDesc")}
        >
          {(rows) => <HostsTable rows={rows} />}
        </ToolSection>
      </TabsContent>

      <TabsContent value="cookies" className="pt-1">
        <ToolSection<HotspotCookieRow>
          router={router}
          kind="cookies"
          title={t("tools.cookies.title")}
          description={t("tools.cookies.desc")}
          emptyTitle={t("tools.cookies.empty")}
          emptyDescription={t("tools.cookies.emptyDesc")}
        >
          {(rows) => <CookiesTable rows={rows} />}
        </ToolSection>
      </TabsContent>

      <TabsContent value="log" className="pt-1">
        <ToolSection<RouterLogRow>
          router={router}
          kind="log"
          title={t("tools.log.title")}
          description={t("tools.log.desc")}
          emptyTitle={t("tools.log.empty")}
          emptyDescription={t("tools.log.emptyDesc")}
        >
          {(rows) => <LogTable rows={rows} />}
        </ToolSection>
      </TabsContent>
    </Tabs>
  );
}

// ─── F8+F10 — Système ───

function SystemInfoCard({ router }: { router: RouterDevice }) {
  const { t } = useI18n();
  // RAM : l'endpoint stats existant (simulé = gateway, agent = télémétrie en cache).
  const { data: stats } = useQuery({
    queryKey: ["/api/routers", router.id, "stats"],
    queryFn: () => api<RouterStats>(`/api/routers/${router.id}/stats`),
    retry: 1,
  });

  const memLabel =
    stats && stats.totalMemoryMb > 0
      ? `${formatMb(stats.freeMemoryMb)} / ${formatMb(stats.totalMemoryMb)} · ${stats.memUsedPct} %`
      : "—";
  const diskLabel = router.totalHddMb ? `${formatMb(router.freeHddMb)} / ${formatMb(router.totalHddMb)}` : "—";

  const rows: { icon: typeof Cpu; label: string; value: string; title?: string }[] = [
    { icon: CircuitBoard, label: t("tools.system.board"), value: router.boardName || "—" },
    { icon: Cpu, label: t("tools.system.cpu"), value: router.cpuLoad ? `${router.cpuLoad} %` : "—" },
    { icon: MemoryStick, label: t("tools.system.memory"), value: memLabel },
    { icon: HardDrive, label: t("tools.system.disk"), value: diskLabel },
    { icon: Clock, label: t("routers.uptime"), value: formatDuration(router.uptimeSec) },
    { icon: Wrench, label: t("routers.routeros"), value: router.version || "—" },
  ];

  return (
    <Card className="gap-0 py-0">
      <CardContent className="p-4 sm:p-5">
        <h3 className="text-sm font-semibold">{t("tools.system.infoTitle")}</h3>
        <p className="mt-0.5 text-xs text-muted-foreground">{t("tools.system.infoDesc")}</p>
        <dl className="mt-3 grid grid-cols-1 gap-x-6 gap-y-2.5 sm:grid-cols-2">
          {rows.map((row) => (
            <div key={row.label} className="flex min-h-6 items-center gap-2 text-sm">
              <row.icon className="size-4 shrink-0 text-muted-foreground" aria-hidden />
              <dt className="text-muted-foreground">{row.label}</dt>
              <dd className="ml-auto truncate font-medium tabular-nums" title={row.value}>
                {row.value}
              </dd>
            </div>
          ))}
        </dl>
      </CardContent>
    </Card>
  );
}

interface PingStats {
  sent: number;
  received: number;
  lossPct: number;
  minMs: number;
  avgMs: number;
  maxMs: number;
}

type PingOutcome =
  | { kind: "stats"; stats: PingStats }
  | { kind: "error"; message: string }
  | { kind: "timeout" };

/** Extrait les stats de ping d'une réponse directe (F8) ou du résultat d'une commande. */
function toPingStats(raw: unknown): PingStats | null {
  let obj: unknown = raw;
  if (typeof obj === "string") {
    try {
      obj = JSON.parse(obj);
    } catch {
      return null;
    }
  }
  if (!obj || typeof obj !== "object") return null;
  const r = obj as Record<string, unknown>;
  const num = (v: unknown): number => (typeof v === "number" && Number.isFinite(v) ? v : 0);
  const stats: PingStats = {
    sent: num(r.sent),
    received: num(r.received),
    lossPct: num(r.lossPct ?? r.loss),
    minMs: num(r.minMs ?? r.min),
    avgMs: num(r.avgMs ?? r.avg),
    maxMs: num(r.maxMs ?? r.max),
  };
  if (stats.sent === 0 && stats.received === 0 && stats.avgMs === 0) return null;
  return stats;
}

/** Message d'erreur lisible depuis le résultat d'une commande échouée. */
function errorMessageFrom(raw: unknown, fallback: string): string {
  if (raw && typeof raw === "object" && "error" in raw) {
    const err = (raw as { error: unknown }).error;
    if (typeof err === "string" && err.length > 0) return err;
  }
  if (typeof raw === "string" && raw.length > 0) return raw;
  return fallback;
}

/** POST ping puis, en mode agent, poll GET /api/commands/{id} toutes les 2 s (max 60 s). */
async function runPing(routerId: string, target: string, lang: Lang): Promise<PingOutcome> {
  try {
    const res = await api<PingResult>(`/api/routers/${routerId}/ping`, { method: "POST", body: { target } });
    if (!res.queued) {
      const stats = toPingStats(res);
      return stats ? { kind: "stats", stats } : { kind: "error", message: translate(lang, "tools.system.pingUnreadable") };
    }
    const commandId = res.commandId ?? "";
    if (!commandId) return { kind: "error", message: translate(lang, "tools.system.pingNotFound") };
    const deadline = Date.now() + 60_000;
    while (Date.now() <= deadline) {
      await sleep(2_000);
      const cmd = await api<CommandStatus>(`/api/commands/${commandId}`);
      if (cmd.status === "done") {
        const stats = toPingStats(cmd.result);
        return stats
          ? { kind: "stats", stats }
          : { kind: "error", message: translate(lang, "tools.system.pingResultUnreadable") };
      }
      if (cmd.status === "error") {
        return { kind: "error", message: errorMessageFrom(cmd.result, translate(lang, "tools.system.pingFailed")) };
      }
    }
    return { kind: "timeout" };
  } catch (err) {
    return {
      kind: "error",
      message: err instanceof Error ? err.message : translate(lang, "tools.system.pingImpossible"),
    };
  }
}

function PingResultPanel({ target, outcome }: { target: string; outcome: PingOutcome }) {
  const { t, tf } = useI18n();
  if (outcome.kind === "timeout") {
    return (
      <div className="mt-3 rounded-lg border border-amber-500/30 bg-amber-500/10 p-3 text-sm text-amber-700 dark:text-amber-400" role="alert">
        {t("tools.system.pingTimeout")}
      </div>
    );
  }
  if (outcome.kind === "error") {
    return (
      <div className="mt-3 rounded-lg border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive" role="alert">
        {outcome.message}
      </div>
    );
  }

  const { stats } = outcome;
  const lossClass =
    stats.lossPct >= 100
      ? "border-destructive/25 bg-destructive/10 text-destructive"
      : stats.lossPct > 0
        ? "border-amber-500/25 bg-amber-500/10 text-amber-600 dark:text-amber-400"
        : "border-emerald-500/25 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400";

  return (
    <div className="mt-3 rounded-lg border p-3" aria-live="polite">
      <p className="text-xs text-muted-foreground">
        {t("tools.system.pingResult")} — <span className="font-mono">{target}</span>
      </p>
      <div className="mt-2 flex flex-wrap items-center gap-x-4 gap-y-2 text-sm">
        <span className="text-muted-foreground">
          {t("tools.system.pingSent")} <span className="font-semibold text-foreground tabular-nums">{stats.sent}</span>
        </span>
        <span className="text-muted-foreground">
          {t("tools.system.pingReceived")} <span className="font-semibold text-foreground tabular-nums">{stats.received}</span>
        </span>
        <Badge variant="outline" className={lossClass}>
          {tf("tools.system.pingLoss", { n: stats.lossPct })}
        </Badge>
        {stats.received > 0 && (
          <span className="text-muted-foreground">
            min <span className="font-semibold text-foreground tabular-nums">{fmtMs(stats.minMs)}</span>
            {" · "}moy. <span className="font-semibold text-foreground tabular-nums">{fmtMs(stats.avgMs)}</span>
            {" · "}max <span className="font-semibold text-foreground tabular-nums">{fmtMs(stats.maxMs)}</span>
          </span>
        )}
      </div>
    </div>
  );
}

function PingCard({ router }: { router: RouterDevice }) {
  const { t, lang } = useI18n();
  const [target, setTarget] = useState("");
  const realMode = router.mode === "real";

  const pingMutation = useMutation({
    mutationFn: (t: string) => runPing(router.id, t, lang),
  });

  const targetOk = target.trim().length > 0 && target.trim().length <= 253;

  return (
    <Card className="gap-0 py-0">
      <CardContent className="p-4 sm:p-5">
        <h3 className="text-sm font-semibold">{t("tools.system.pingTitle")}</h3>
        <p className="mt-0.5 text-xs text-muted-foreground">{t("tools.system.pingDesc")}</p>

        <form
          className="mt-3 flex flex-col gap-2 sm:flex-row"
          onSubmit={(e) => {
            e.preventDefault();
            if (!targetOk || pingMutation.isPending || realMode) return;
            pingMutation.mutate(target.trim());
          }}
        >
          <Input
            className="font-mono"
            placeholder={t("tools.system.pingPlaceholder")}
            value={target}
            onChange={(e) => setTarget(e.target.value)}
            aria-label={t("tools.system.pingTarget")}
            disabled={realMode || pingMutation.isPending}
          />
          <Button type="submit" disabled={!targetOk || pingMutation.isPending || realMode} className="shrink-0">
            {pingMutation.isPending ? <Loader2 className="size-4 animate-spin" /> : <Zap className="size-4" />}
            {t("tools.system.pingSubmit")}
          </Button>
        </form>

        {realMode && <p className="mt-2 text-xs text-muted-foreground">{t("tools.realNote")}</p>}
        {pingMutation.isPending && router.mode === "agent" && (
          <p className="mt-3 flex items-center gap-2 text-sm text-muted-foreground" role="status">
            <Loader2 className="size-4 animate-spin text-amber-600" />
            {t("tools.queued")}
          </p>
        )}
        {pingMutation.data && <PingResultPanel target={pingMutation.variables ?? target} outcome={pingMutation.data} />}
      </CardContent>
    </Card>
  );
}

// ─── F10 — Scheduler ───

function SchedulerAddDialog({ router, onClose }: { router: RouterDevice; onClose: () => void }) {
  const { t, tf } = useI18n();
  const queryClient = useQueryClient();
  const [name, setName] = useState("");
  const [interval, setIntervalValue] = useState("");
  const [onEvent, setOnEvent] = useState("");

  const trimmedName = name.trim();
  const nameOk = trimmedName.length > 0 && trimmedName.length <= 48 && !/\s/.test(trimmedName);
  const intervalOk = INTERVAL_RE.test(interval.trim());
  const valid = nameOk && intervalOk && onEvent.trim().length > 0;

  const createMutation = useMutation({
    mutationFn: () =>
      api<SchedulerRow & { ok?: boolean; queued?: boolean }>(`/api/routers/${router.id}/scheduler`, {
        method: "POST",
        body: { name: trimmedName, interval: interval.trim(), onEvent: onEvent.trim() },
      }),
    onSuccess: (res) => {
      toast.success(
        res.queued ? t("tools.bindings.queuedCreate") : tf("tools.scheduler.createdToast", { name: trimmedName }),
      );
      onClose();
      void queryClient.invalidateQueries({ queryKey: ["/api/routers", router.id, "scheduler"] });
    },
    onError: (err: Error) => toast.error(err.message),
  });

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{t("tools.scheduler.addTitle")}</DialogTitle>
          <DialogDescription>{tf("tools.scheduler.addDesc", { name: router.name })}</DialogDescription>
        </DialogHeader>

        <form
          className="space-y-4"
          onSubmit={(e) => {
            e.preventDefault();
            if (!valid || createMutation.isPending) return;
            createMutation.mutate();
          }}
        >
          <div className="space-y-2">
            <Label htmlFor="task-name">{t("common.name")}</Label>
            <Input
              id="task-name"
              className="font-mono"
              placeholder="daily-backup"
              value={name}
              onChange={(e) => setName(e.target.value)}
              disabled={createMutation.isPending}
              aria-invalid={trimmedName.length > 0 && !nameOk}
              autoFocus
            />
            {trimmedName.length > 0 && !nameOk && (
              <p className="text-xs text-destructive" role="alert">
                {t("tools.scheduler.nameInvalid")}
              </p>
            )}
          </div>

          <div className="space-y-2">
            <Label htmlFor="task-interval">{t("tools.scheduler.interval")}</Label>
            <Input
              id="task-interval"
              className="font-mono"
              placeholder="45s"
              value={interval}
              onChange={(e) => setIntervalValue(e.target.value)}
              disabled={createMutation.isPending}
              aria-invalid={interval.length > 0 && !intervalOk}
            />
            {interval.length > 0 && !intervalOk ? (
              <p className="text-xs text-destructive" role="alert">
                {t("tools.scheduler.intervalInvalid")}
              </p>
            ) : (
              <p className="text-xs text-muted-foreground">{t("tools.scheduler.intervalHint")}</p>
            )}
          </div>

          <div className="space-y-2">
            <Label htmlFor="task-onevent">{t("tools.scheduler.onEvent")}</Label>
            <Input
              id="task-onevent"
              className="font-mono"
              placeholder="/system script run backup"
              value={onEvent}
              onChange={(e) => setOnEvent(e.target.value)}
              disabled={createMutation.isPending}
            />
          </div>

          {createMutation.isError && (
            <p className="text-sm text-destructive" role="alert">
              {createMutation.error.message}
            </p>
          )}

          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose} disabled={createMutation.isPending}>
              {t("common.cancel")}
            </Button>
            <Button type="submit" disabled={!valid || createMutation.isPending}>
              {createMutation.isPending && <Loader2 className="size-4 animate-spin" />}
              {t("tools.scheduler.createSubmit")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function SchedulerCard({ router }: { router: RouterDevice }) {
  const { t, tf, lang } = useI18n();
  const queryClient = useQueryClient();
  const [adding, setAdding] = useState(false);
  const [deleting, setDeleting] = useState<SchedulerRow | null>(null);

  const { data, isLoading, isError, error, refetch, isFetching } = useQuery({
    queryKey: ["/api/routers", router.id, "scheduler"],
    queryFn: () => fetchToolEnvelope<SchedulerRow>(`/api/routers/${router.id}/scheduler`),
    enabled: router.mode !== "real",
    refetchInterval: (query) => (query.state.data?.queued ? 3_000 : false),
  });

  const tasks = data?.data ?? [];

  const invalidate = () => queryClient.invalidateQueries({ queryKey: ["/api/routers", router.id, "scheduler"] });

  const toggleMutation = useMutation({
    mutationFn: (task: SchedulerRow) =>
      api<{ ok?: boolean; queued?: boolean }>(`/api/routers/${router.id}/scheduler-toggle`, {
        method: "POST",
        body: { name: task.name, disabled: !task.disabled },
      }),
    onSuccess: (res, task) => {
      toast.success(
        res.queued
          ? t("tools.scheduler.queuedToggle")
          : task.disabled
            ? tf("tools.scheduler.activatedToast", { name: task.name })
            : tf("tools.scheduler.deactivatedToast", { name: task.name }),
      );
      void invalidate();
    },
    onError: (err: Error) => toast.error(err.message),
  });

  const removeMutation = useMutation({
    mutationFn: (task: SchedulerRow) =>
      api<{ ok?: boolean; queued?: boolean }>(`/api/routers/${router.id}/scheduler-remove`, {
        method: "POST",
        body: { name: task.name },
      }),
    onSuccess: (res, task) => {
      toast.success(
        res.queued ? t("tools.scheduler.queuedRemove") : tf("tools.scheduler.deletedToast", { name: task.name }),
      );
      setDeleting(null);
      void invalidate();
    },
    onError: (err: Error) => toast.error(err.message),
  });

  return (
    <Card className="gap-0 py-0">
      <CardContent className="p-4 sm:p-5">
        <div className="flex flex-wrap items-start justify-between gap-2">
          <div>
            <h3 className="text-sm font-semibold">{t("tools.scheduler.title")}</h3>
            <p className="mt-0.5 text-xs text-muted-foreground">{t("tools.scheduler.desc")}</p>
          </div>
          <div className="flex items-center gap-2">
            <Button
              variant="outline"
              size="icon"
              className="size-8"
              onClick={() => void refetch()}
              disabled={isFetching}
              aria-label={t("tools.scheduler.refreshAria")}
            >
              <RefreshCw className={cn("size-3.5", isFetching && "animate-spin")} />
            </Button>
            <Button size="sm" className="h-8" onClick={() => setAdding(true)}>
              <Plus className="size-4" />
              {t("tools.scheduler.new")}
            </Button>
          </div>
        </div>

        <div className="mt-3">
          {data?.updatedAt && (
            <p className="mb-2 hidden text-xs text-muted-foreground sm:block">
              {tf("tools.updatedAgo", { ago: timeAgo(data.updatedAt, lang) })}
            </p>
          )}

          {data?.queued && <QueuedBanner />}

          {isLoading ? (
            <ToolSkeleton rows={3} />
          ) : isError ? (
            <ToolError error={error} onRetry={() => void refetch()} />
          ) : tasks.length === 0 && !data?.queued ? (
            <EmptyState
              icon={Clock}
              title={t("tools.scheduler.empty")}
              description={t("tools.scheduler.emptyDesc")}
              action={
                <Button variant="outline" onClick={() => setAdding(true)}>
                  <Plus className="size-4" />
                  {t("tools.scheduler.new")}
                </Button>
              }
            />
          ) : tasks.length === 0 ? (
            <ToolSkeleton rows={3} />
          ) : (
            <div className="max-h-64 overflow-y-auto rounded-lg border">
              <Table>
                <TableHeader>
                  <TableRow className="hover:bg-transparent">
                    <TableHead className="pl-4 text-muted-foreground">{t("common.name")}</TableHead>
                    <TableHead className="text-muted-foreground">{t("tools.scheduler.interval")}</TableHead>
                    <TableHead className="hidden text-muted-foreground md:table-cell">{t("tools.scheduler.onEventCol")}</TableHead>
                    <TableHead className="text-muted-foreground">{t("common.status")}</TableHead>
                    <TableHead className="pr-4 text-right text-muted-foreground">{t("common.actions")}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {tasks.map((task) => (
                    <TableRow key={task.id ?? task.name}>
                      <TableCell className="pl-4 font-mono text-[13px] font-medium">{task.name}</TableCell>
                      <TableCell className="whitespace-nowrap tabular-nums">{task.interval || "—"}</TableCell>
                      <TableCell className="hidden max-w-48 md:table-cell">
                        <span className="block truncate font-mono text-xs text-muted-foreground" title={task.onEvent}>
                          {task.onEvent || "—"}
                        </span>
                      </TableCell>
                      <TableCell>
                        <Switch
                          checked={!task.disabled}
                          disabled={toggleMutation.isPending && toggleMutation.variables?.name === task.name}
                          onCheckedChange={() => toggleMutation.mutate(task)}
                          aria-label={tf("tools.scheduler.toggleAria", {
                            action: task.disabled ? t("common.activate") : t("common.deactivate"),
                            name: task.name,
                          })}
                        />
                      </TableCell>
                      <TableCell className="pr-4">
                        <Button
                          variant="ghost"
                          size="icon"
                          className="size-8 text-muted-foreground hover:text-destructive"
                          onClick={() => setDeleting(task)}
                          aria-label={tf("tools.scheduler.deleteAria", { name: task.name })}
                        >
                          <Trash2 className="size-4" />
                        </Button>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          )}
        </div>

        {adding && <SchedulerAddDialog router={router} onClose={() => setAdding(false)} />}

        <AlertDialog open={deleting !== null} onOpenChange={(open) => !open && setDeleting(null)}>
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>{t("tools.scheduler.deleteTitle")}</AlertDialogTitle>
              <AlertDialogDescription>
                {deleting
                  ? tf("tools.scheduler.deleteDesc", { name: deleting.name, router: router.name })
                  : ""}
              </AlertDialogDescription>
            </AlertDialogHeader>
            <AlertDialogFooter>
              <AlertDialogCancel disabled={removeMutation.isPending}>{t("common.cancel")}</AlertDialogCancel>
              <AlertDialogAction
                className="bg-destructive text-white hover:bg-destructive/90"
                disabled={removeMutation.isPending}
                onClick={(e) => {
                  e.preventDefault();
                  if (deleting) removeMutation.mutate(deleting);
                }}
              >
                {removeMutation.isPending && <Loader2 className="size-4 animate-spin" />}
                {t("common.delete")}
              </AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>
      </CardContent>
    </Card>
  );
}

// ─── F10 — Alimentation (reboot / shutdown) ───

function PowerCard({ router }: { router: RouterDevice }) {
  const { t, tf } = useI18n();
  const queryClient = useQueryClient();
  const [confirmKind, setConfirmKind] = useState<"reboot" | "shutdown" | null>(null);

  const powerMutation = useMutation({
    mutationFn: (kind: "reboot" | "shutdown") =>
      api<{ ok?: boolean; queued?: boolean }>(`/api/routers/${router.id}/${kind}`, { method: "POST" }),
    onSuccess: (res, kind) => {
      if (res.queued) {
        toast.success(
          kind === "reboot" ? t("tools.power.rebootQueued") : t("tools.power.shutdownQueued"),
        );
      } else {
        toast.success(kind === "reboot" ? t("tools.power.rebootedToast") : t("tools.power.shutdownToast"));
      }
      setConfirmKind(null);
      for (const key of ["/api/routers", "/api/sessions", "/api/dashboard"]) {
        void queryClient.invalidateQueries({ queryKey: [key] });
      }
    },
    onError: (err: Error) => toast.error(err.message),
  });

  return (
    <Card className="gap-0 py-0">
      <CardContent className="p-4 sm:p-5">
        <h3 className="text-sm font-semibold">{t("tools.power.title")}</h3>
        <p className="mt-0.5 text-xs text-muted-foreground">
          {router.mode === "real" ? t("tools.realNote") : t("tools.power.desc")}
        </p>

        <div className="mt-3 flex flex-wrap gap-3">
          <Button
            variant="outline"
            className="border-destructive/40 text-destructive hover:bg-destructive/10 hover:text-destructive"
            onClick={() => setConfirmKind("reboot")}
            disabled={powerMutation.isPending}
          >
            <RotateCcw className="size-4" />
            {t("tools.power.reboot")}
          </Button>
          <Button variant="destructive" onClick={() => setConfirmKind("shutdown")} disabled={powerMutation.isPending}>
            <Power className="size-4" />
            {t("tools.power.shutdown")}
          </Button>
        </div>
      </CardContent>

      <AlertDialog open={confirmKind !== null} onOpenChange={(open) => !open && setConfirmKind(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {confirmKind === "shutdown" ? t("tools.power.shutdownTitle") : t("tools.power.rebootTitle")}
            </AlertDialogTitle>
            <AlertDialogDescription asChild>
              <span>
                {confirmKind === "shutdown" ? (
                  <>
                    {tf("tools.power.shutdownDesc1", { name: router.name })}{" "}
                    <strong>{t("tools.power.shutdownDescStrong")}</strong>{" "}
                    {t("tools.power.shutdownDesc2")}
                  </>
                ) : (
                  <>{tf("tools.power.rebootDesc", { name: router.name })}</>
                )}
                {router.mode === "agent" && t("tools.power.agentNote")}
              </span>
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={powerMutation.isPending}>{t("common.cancel")}</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-white hover:bg-destructive/90"
              disabled={powerMutation.isPending}
              onClick={(e) => {
                e.preventDefault();
                if (confirmKind) powerMutation.mutate(confirmKind);
              }}
            >
              {powerMutation.isPending && <Loader2 className="size-4 animate-spin" />}
              {confirmKind === "shutdown" ? t("tools.power.shutdownConfirm") : t("tools.power.reboot")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </Card>
  );
}

function SystemTab({ router }: { router: RouterDevice }) {
  return (
    <div className="space-y-4">
      <SystemInfoCard router={router} />
      <PingCard router={router} />
      <SchedulerCard router={router} />
      <PowerCard router={router} />
    </div>
  );
}

// ─── Dialogue principal ───

export function RouterToolsDialog({ router: snapshot, onClose }: { router: RouterDevice; onClose: () => void }) {
  const { t, tf } = useI18n();
  // Routeur « vivant » : la vue Routeurs poll déjà ["/api/routers"] toutes les 15 s
  // (même queryKey → cache partagé). Si le routeur passe hors ligne ou disparaît
  // pendant l'affichage, les onglets dégradent proprement (états d'erreur/toasts).
  const { data: routers } = useQuery({
    queryKey: ["/api/routers"],
    queryFn: () => api<RouterDevice[]>("/api/routers"),
    refetchInterval: 15_000,
  });
  const router = routers?.find((r) => r.id === snapshot.id) ?? snapshot;

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="flex max-h-[90vh] flex-col gap-0 overflow-hidden p-0 sm:max-w-4xl">
        <DialogHeader className="shrink-0 border-b px-4 pb-4 pt-5 sm:px-6">
          <DialogTitle className="flex items-center gap-2 pr-8">
            <Wrench className="size-5 shrink-0 text-primary" />
            {tf("tools.dialogTitle", { name: router.name })}
          </DialogTitle>
          <DialogDescription>
            <span className="flex flex-wrap items-center gap-2">
              <StatusBadge status={router.mode} />
              <StatusBadge status={router.status} dot />
              <span>{t("tools.dialogDesc")}</span>
            </span>
          </DialogDescription>
        </DialogHeader>

        <Tabs defaultValue="traffic" className="flex min-h-0 flex-1 flex-col gap-0">
          <div className="shrink-0 overflow-x-auto px-4 py-3 sm:px-6">
            <TabsList className="w-full min-w-max sm:w-fit">
              <TabsTrigger value="traffic">
                <Activity className="size-4" />
                {t("tools.tabTraffic")}
              </TabsTrigger>
              <TabsTrigger value="bindings">
                <Network className="size-4" />
                {t("tools.tabBindings")}
              </TabsTrigger>
              <TabsTrigger value="tools">
                <Wrench className="size-4" />
                {t("tools.tabTools")}
              </TabsTrigger>
              <TabsTrigger value="system">
                <Cpu className="size-4" />
                {t("tools.tabSystem")}
              </TabsTrigger>
            </TabsList>
          </div>

          <div className="min-h-0 flex-1 overflow-y-auto px-4 pb-5 sm:px-6">
            <TabsContent value="traffic" className="mt-0">
              <TrafficTab router={router} />
            </TabsContent>
            <TabsContent value="bindings" className="mt-0">
              <IpBindingsTab router={router} />
            </TabsContent>
            <TabsContent value="tools" className="mt-0">
              <ToolsTab router={router} />
            </TabsContent>
            <TabsContent value="system" className="mt-0">
              <SystemTab router={router} />
            </TabsContent>
          </div>
        </Tabs>
      </DialogContent>
    </Dialog>
  );
}
