"use client";

// N°101 — vue « Appareils » (console HomeNet), Phase 3 : le VRAI registre
// du foyer. Fin de la dépendance aux sessions hotspot (N°100) : un foyer
// n'a pas de portail captif, ses appareils rejoignent le WiFi et reçoivent
// un bail DHCP de la box — c'est CE bail que l'agent rapporte (read_dhcp,
// cadence 2 min) et que GET /api/devices sert.
//
// Le vocabulaire reste domestique : un « appareil » nommé par la famille
// (« TV du salon », « Tel de mama » — nom affecté, repli host-name puis
// MAC), un statut « En ligne / Hors ligne » (bail bound ou non) et LA
// pause dîner : couper l'internet d'un appareil précis pour 30 min, 1 h,
// 2 h ou jusqu'à réactivation. L'état affiché est l'état DÉSIRÉ connu du
// cloud (pattern Protection) ; la coupure est appliquée par la box à son
// prochain check-in — l'encart sous le tableau le dit sans jargon.
//
// Lecture : poll 10 s (ETag/304 serveur). Écritures : renommage (PUT) et
// pause/reprise (POST) — mutation + invalidation TanStack, toast du
// message serveur, jamais d'optimisme (l'état réel revient du poll).

import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Laptop,
  MonitorSmartphone,
  Pause,
  Pencil,
  Play,
  Search,
  Smartphone,
  Tv,
  Wifi,
  WifiOff,
} from "lucide-react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { api, pauseDevice, renameDevice } from "@/lib/hotspot/api";
import { useI18n } from "@/lib/hotspot/i18n";
import { formatDuration } from "@/lib/hotspot/format";
import { EmptyState } from "@/components/hotspot/empty-state";
import { LoadingRows } from "@/components/hotspot/loading";
import { PageHeader } from "@/components/hotspot/page-header";
import { StatCard } from "@/components/hotspot/stat-card";
import type { HomeDevice, RouterDevice } from "@/lib/hotspot/types";

const REFRESH_OPTIONS = [
  { value: "5000", label: "5 s" },
  { value: "10000", label: "10 s" },
  { value: "30000", label: "30 s" },
];

/** Options de la pause dîner — presets courts + illimité (N°101). */
const PAUSE_PRESETS = [
  { minutes: 30, key: "devices.pause.30" },
  { minutes: 60, key: "devices.pause.60" },
  { minutes: 120, key: "devices.pause.120" },
  { minutes: 0, key: "devices.pause.forever" },
];

/** Icône d'identité heuristique : le host-name trahit souvent l'appareil. */
function DeviceIcon({ device }: { device: HomeDevice }) {
  const hint = `${device.name} ${device.hostname}`.toLowerCase();
  let Icon = MonitorSmartphone;
  if (/\btv\b|smart-?tv|television/.test(hint)) Icon = Tv;
  else if (/phone|tel\b|galaxy|android|iphone|redmi|tecno|infinix|huawei/.test(hint)) Icon = Smartphone;
  else if (/laptop|pc\b|macbook|notebook|ordinateur/.test(hint)) Icon = Laptop;
  return <Icon className="size-4 shrink-0 text-muted-foreground" aria-hidden />;
}

/** Nom affiché : nom affecté → host-name DHCP → MAC (repli de l'UI). */
function deviceLabel(d: HomeDevice): string {
  return d.name || d.hostname || d.mac;
}

/** Pause effective côté client — miroir du calcul serveur (état désiré). */
function pauseRemainingMs(d: HomeDevice, now: number): number | null {
  if (!d.paused) return null;
  if (!d.pausedUntil) return null; // illimité
  const until = Date.parse(d.pausedUntil);
  if (Number.isNaN(until)) return null;
  return Math.max(0, until - now);
}

/** Chip de statut : En ligne (bail bound) / En pause / Hors ligne. */
function DeviceStatusChip({ device, remainingMs }: { device: HomeDevice; remainingMs: number | null }) {
  const { t } = useI18n();
  if (device.paused) {
    return (
      <Badge className="gap-1.5 border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-300" variant="outline">
        <Pause className="size-3" aria-hidden />
        {t("devices.status.paused")}
        {remainingMs !== null && remainingMs > 0 && (
          <span className="tabular-nums opacity-80">· {formatDuration(Math.floor(remainingMs / 1000))}</span>
        )}
      </Badge>
    );
  }
  if (device.status === "bound") {
    return (
      <Badge className="gap-1.5 border-primary/25 bg-primary/10 text-primary" variant="outline">
        <span className="live-dot size-1.5 rounded-full bg-primary" aria-hidden />
        {t("devices.status.online")}
      </Badge>
    );
  }
  return (
    <Badge variant="outline" className="gap-1.5 text-muted-foreground">
      {t("devices.status.offline")}
    </Badge>
  );
}

export default function DevicesView() {
  const { t, tf, lang } = useI18n();
  const queryClient = useQueryClient();
  const [refreshMs, setRefreshMs] = useState(10000);
  const [now, setNow] = useState(() => Date.now());
  const [query, setQuery] = useState("");

  // La box (pour distinguer « pas encore de routeur agent » de « vide »).
  const { data: routers } = useQuery({
    queryKey: ["/api/routers"],
    queryFn: () => api<RouterDevice[]>("/api/routers"),
    refetchInterval: 30_000,
  });
  const hasAgentRouter = useMemo(() => (routers ?? []).some((r) => r.mode === "agent"), [routers]);

  // Le registre : bails DHCP rapportés par la box (cadence 2 min côté cloud,
  // ETag/304 au poll client — le corps ne voyage que quand il change).
  const { data, isLoading } = useQuery({
    queryKey: ["/api/devices"],
    queryFn: () => api<HomeDevice[]>("/api/devices"),
    refetchInterval: refreshMs,
  });

  // Horloge locale (1 s) : fait « vivre » le compte à rebours des pauses
  // bornées entre deux polls — même mécanique que la vue Sessions.
  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(timer);
  }, []);

  const devices = useMemo(() => data ?? [], [data]);

  // Filtre local (nom affecté, host-name, IP ou MAC) — pas de pagination :
  // un foyer compte ses appareils sur les doigts.
  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return devices;
    return devices.filter(
      (d) =>
        deviceLabel(d).toLowerCase().includes(q) ||
        d.ip.toLowerCase().includes(q) ||
        d.mac.toLowerCase().includes(q),
    );
  }, [devices, query]);

  const onlineCount = devices.filter((d) => d.status === "bound").length;
  const pausedCount = devices.filter((d) => d.paused).length;

  // — Mutations : pause / reprise / renommage (jamais d'optimisme : le
  // message serveur va au toast, l'état réel revient du poll). —
  const [busyId, setBusyId] = useState<string | null>(null);

  const pauseMutation = useMutation({
    mutationFn: ({ id, paused, minutes }: { id: string; paused: boolean; minutes?: number }) =>
      pauseDevice(id, paused, minutes),
    onSuccess: (_res, vars) => {
      const device = devices.find((d) => d.id === vars.id);
      const label = device ? deviceLabel(device) : "";
      if (vars.paused) {
        const preset = PAUSE_PRESETS.find((p) => p.minutes === (vars.minutes ?? 0));
        if (preset && vars.minutes) {
          toast.success(tf("devices.pauseToast", { device: label, duration: t(preset.key) }));
        } else {
          toast.success(tf("devices.pauseForeverToast", { device: label }));
        }
      } else {
        toast.success(tf("devices.resumeToast", { device: label }));
      }
      void queryClient.invalidateQueries({ queryKey: ["/api/devices"] });
    },
    onError: (err) => toast.error(err instanceof Error ? err.message : t("common.error")),
  });

  const renameMutation = useMutation({
    mutationFn: ({ id, name }: { id: string; name: string }) => renameDevice(id, name),
    onSuccess: () => {
      toast.success(t("devices.renameToast"));
      void queryClient.invalidateQueries({ queryKey: ["/api/devices"] });
    },
    onError: (err) => toast.error(err instanceof Error ? err.message : t("common.error")),
  });

  const doPause = (device: HomeDevice, paused: boolean, minutes?: number) => {
    setBusyId(device.id);
    pauseMutation.mutate({ id: device.id, paused, minutes }, { onSettled: () => setBusyId(null) });
  };

  // — Dialog de renommage —
  const [renameTarget, setRenameTarget] = useState<HomeDevice | null>(null);
  const [renameValue, setRenameValue] = useState("");
  const openRename = (device: HomeDevice) => {
    setRenameTarget(device);
    setRenameValue(device.name);
  };
  const submitRename = () => {
    if (!renameTarget) return;
    setBusyId(renameTarget.id);
    renameMutation.mutate(
      { id: renameTarget.id, name: renameValue.trim() },
      {
        onSettled: () => {
          setBusyId(null);
          setRenameTarget(null);
        },
      },
    );
  };

  return (
    <div className="space-y-4 sm:space-y-6">
      <PageHeader
        title={t("devices.title")}
        description={t("devices.description")}
        actions={
          <>
            <Badge variant="outline" className="gap-2 border-primary/25 bg-primary/10 py-1 text-primary">
              <span className="live-dot size-2 rounded-full bg-primary" aria-hidden />
              {t("sessions.live")}
            </Badge>
            <div className="relative">
              <Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" aria-hidden />
              <Input
                value={query}
                onChange={(event) => setQuery(event.target.value)}
                placeholder={t("devices.searchPlaceholder")}
                className="h-10 w-40 pl-9 sm:w-56"
                aria-label={t("devices.searchPlaceholder")}
              />
            </div>
            <div className="flex items-center gap-2">
              <span className="hidden text-xs text-muted-foreground sm:inline">{t("sessions.refresh")}</span>
              <Select value={String(refreshMs)} onValueChange={(value) => setRefreshMs(Number(value))}>
                <SelectTrigger size="sm" className="h-10 w-24" aria-label={t("sessions.refreshLabel")}>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {REFRESH_OPTIONS.map((option) => (
                    <SelectItem key={option.value} value={option.value}>
                      {option.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </>
        }
      />

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
        <StatCard
          title={t("devices.kpi.total")}
          value={String(devices.length)}
          sub={t("devices.kpi.totalSub")}
          icon={MonitorSmartphone}
        />
        <StatCard
          title={t("devices.kpi.online")}
          value={String(onlineCount)}
          sub={t("devices.kpi.onlineSub")}
          icon={Wifi}
          live
        />
        <StatCard
          title={t("devices.kpi.paused")}
          value={String(pausedCount)}
          sub={t("devices.kpi.pausedSub")}
          icon={Pause}
        />
      </div>

      <Card className="gap-0 py-0">
        {isLoading ? (
          <LoadingRows rows={6} />
        ) : filtered.length === 0 ? (
          devices.length === 0 && !hasAgentRouter ? (
            <EmptyState
              icon={MonitorSmartphone}
              title={t("devices.empty.routerTitle")}
              description={t("devices.empty.routerDesc")}
            />
          ) : devices.length === 0 ? (
            <EmptyState
              icon={WifiOff}
              title={t("devices.empty.title")}
              description={t("devices.empty.desc")}
            />
          ) : (
            <EmptyState icon={Search} title={t("devices.noMatch")} description={t("devices.noMatchDesc")} />
          )
        ) : (
          <>
            <Table>
              <TableHeader>
                <TableRow className="hover:bg-transparent">
                  <TableHead className="pl-4 text-muted-foreground sm:pl-6">{t("devices.device")}</TableHead>
                  <TableHead className="text-muted-foreground">{t("common.ip")}</TableHead>
                  <TableHead className="hidden text-muted-foreground md:table-cell">{t("common.mac")}</TableHead>
                  <TableHead className="hidden text-muted-foreground xl:table-cell">{t("common.router")}</TableHead>
                  <TableHead className="text-muted-foreground">{t("devices.status")}</TableHead>
                  <TableHead className="hidden text-muted-foreground lg:table-cell">{t("devices.lease")}</TableHead>
                  <TableHead className="pr-4 text-right text-muted-foreground sm:pr-6">{t("devices.actions")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {filtered.map((device) => {
                  const remaining = pauseRemainingMs(device, now);
                  const busy = busyId === device.id;
                  return (
                    <TableRow key={device.id} className="transition-colors hover:bg-muted/50">
                      <TableCell className="max-w-56 pl-4 sm:pl-6">
                        <span className="flex items-center gap-2">
                          <DeviceIcon device={device} />
                          <span className="min-w-0">
                            <span className="block truncate text-sm font-medium" title={deviceLabel(device)}>
                              {deviceLabel(device)}
                            </span>
                            {(device.name || !device.hostname) && (
                              <span className="block truncate font-mono text-xs text-muted-foreground" title={device.mac}>
                                {device.mac}
                              </span>
                            )}
                            {device.name && device.hostname && (
                              <span className="block truncate text-xs text-muted-foreground/70" title={device.hostname}>
                                {device.hostname}
                              </span>
                            )}
                          </span>
                        </span>
                      </TableCell>
                      <TableCell className="font-mono text-muted-foreground">{device.ip || "—"}</TableCell>
                      <TableCell className="hidden font-mono text-muted-foreground md:table-cell">{device.mac}</TableCell>
                      <TableCell className="hidden max-w-40 truncate text-muted-foreground xl:table-cell">
                        {device.routerName}
                      </TableCell>
                      <TableCell>
                        <DeviceStatusChip device={device} remainingMs={remaining} />
                      </TableCell>
                      <TableCell className="hidden tabular-nums text-muted-foreground lg:table-cell">
                        {device.status === "bound" && device.expires ? device.expires : "—"}
                      </TableCell>
                      <TableCell className="pr-4 sm:pr-6">
                        <div className="flex items-center justify-end gap-1">
                          <Button
                            size="icon"
                            variant="ghost"
                            className="size-8"
                            aria-label={t("devices.rename")}
                            title={t("devices.rename")}
                            disabled={busy || renameMutation.isPending}
                            onClick={() => openRename(device)}
                          >
                            <Pencil className="size-3.5" aria-hidden />
                          </Button>
                          {device.paused ? (
                            <Button
                              size="sm"
                              variant="outline"
                              className="h-8 gap-1.5"
                              disabled={busy}
                              onClick={() => doPause(device, false)}
                            >
                              <Play className="size-3.5" aria-hidden />
                              {t("devices.resume")}
                            </Button>
                          ) : (
                            <DropdownMenu>
                              <DropdownMenuTrigger asChild>
                                <Button size="sm" variant="outline" className="h-8 gap-1.5" disabled={busy}>
                                  <Pause className="size-3.5" aria-hidden />
                                  {t("devices.pause")}
                                </Button>
                              </DropdownMenuTrigger>
                              <DropdownMenuContent align="end" className="w-52">
                                {PAUSE_PRESETS.map((preset) => (
                                  <DropdownMenuItem
                                    key={preset.minutes}
                                    onClick={() => doPause(device, true, preset.minutes)}
                                  >
                                    {preset.minutes > 0 ? (
                                      <span className="tabular-nums">{t(preset.key)}</span>
                                    ) : (
                                      <span>{t(preset.key)}</span>
                                    )}
                                  </DropdownMenuItem>
                                ))}
                                <DropdownMenuSeparator />
                                <DropdownMenuItem disabled className="text-xs text-muted-foreground">
                                  {t("devices.pauseFootnote")}
                                </DropdownMenuItem>
                              </DropdownMenuContent>
                            </DropdownMenu>
                          )}
                        </div>
                      </TableCell>
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
            {/* L'honnêteté de la convergence : l'état affiché est l'état désiré
                du cloud, la box l'applique à son prochain check-in. */}
            <p className="border-t px-4 py-2.5 text-xs text-muted-foreground sm:px-6">
              {t("devices.pauseHint")}
            </p>
          </>
        )}
      </Card>

      {/* — Dialog « Nommer l'appareil » — */}
      <Dialog open={renameTarget !== null} onOpenChange={(open) => !open && setRenameTarget(null)}>
        <DialogContent className="sm:max-w-sm">
          <DialogHeader>
            <DialogTitle>{t("devices.renameTitle")}</DialogTitle>
            <DialogDescription>{t("devices.renameDesc")}</DialogDescription>
          </DialogHeader>
          <div className="space-y-2 py-1">
            <Input
              value={renameValue}
              onChange={(event) => setRenameValue(event.target.value)}
              placeholder={t("devices.renamePlaceholder")}
              maxLength={48}
              autoFocus
              onKeyDown={(event) => {
                if (event.key === "Enter") submitRename();
              }}
            />
            <p className="text-xs text-muted-foreground">
              {renameTarget && (
                <>
                  {t("devices.renameCurrent")} :{" "}
                  <span className="font-mono">{renameTarget.name || renameTarget.hostname || renameTarget.mac}</span>
                </>
              )}
            </p>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setRenameTarget(null)}>
              {t("common.cancel")}
            </Button>
            <Button onClick={submitRename} disabled={renameMutation.isPending}>
              {t("devices.renameSave")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
