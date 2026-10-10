"use client";

// N°290 — vue « Cybercafé » (module activable, overlay du mode hotspot) :
// le registre des POSTES (machines identifiées par leur MAC, mêmes
// disciplines que les appareils HomeNet N°101), l'attribution d'un
// CODE-TEMPS par poste (voucher limit-uptime créé sur le gabarit de
// génération + caisse enregistrée — Transaction/Sale canal « direct ») et
// la PAUSE par poste (commande device_pause réutilisée — l'ensemble désiré
// fusionne postes et appareils foyers dans desiredPauseMacsLocked).
//
// Le module est OPT-IN (settings.tenant.cyberEnabled, défaut OFF — décision
// D6 de N°289) : la vue propose l'activation en un clic tant qu'il est
// éteint, puis se comporte comme les autres vues produit (poll ETag/304,
// mutations sans optimisme, toast du message serveur).

import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  BadgeCheck,
  Check,
  Copy,
  Armchair,
  Download,
  MoreHorizontal,
  Pause,
  Pencil,
  Play,
  Plus,
  Search,
  Trash2,
  Wallet,
} from "lucide-react";
import { toast } from "sonner";

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
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  assignCyberPoste,
  api,
  createCyberPoste,
  deleteCyberPoste,
  fetchCyberDiscover,
  fetchCyberPostes,
  importCyberPoste,
  pauseCyberPoste,
  releaseCyberPoste,
  renameCyberPoste,
  setCyberEnabled,
} from "@/lib/hotspot/api";
import { useI18n } from "@/lib/hotspot/i18n";
import { formatCurrency, formatDuration } from "@/lib/hotspot/format";
import { SETTINGS_QUERY_KEY, useCurrency, useSettings } from "@/components/hotspot/parts/sd-currency";
import { copyToClipboard } from "@/components/hotspot/parts/uc-clipboard";
import { EmptyState } from "@/components/hotspot/empty-state";
import { LoadingRows } from "@/components/hotspot/loading";
import { PageHeader } from "@/components/hotspot/page-header";
import { StatCard } from "@/components/hotspot/stat-card";
import type { CyberDiscoveryRow, CyberPoste, Profile, RouterDevice } from "@/lib/hotspot/types";

const REFRESH_OPTIONS = [
  { value: "10000", label: "10 s" },
  { value: "30000", label: "30 s" },
  { value: "60000", label: "60 s" },
];

/** Presets de pause poste — mêmes durées que la pause dîner HomeNet. */
const PAUSE_PRESETS = [
  { minutes: 30, key: "cyber.pause.30" },
  { minutes: 60, key: "cyber.pause.60" },
  { minutes: 120, key: "cyber.pause.120" },
  { minutes: 0, key: "cyber.pause.forever" },
];

/** Réponse GET /api/accounting?period=day — le DERNIER point de la série
 * porte la journée en cours au FUSEAU DU COMPTE (buckets N°198). */
interface AccountingSeriesPoint {
  label: string;
  revenue: number;
  sales: number;
}

interface AccountingDayResponse {
  period: string;
  series: AccountingSeriesPoint[];
}

/** Nom affiché : nom affecté → MAC (même repli que les appareils). */
function posteLabel(p: CyberPoste): string {
  return p.name || p.mac;
}

/** Pause effective côté client — miroir du calcul serveur (état désiré). */
function pauseRemainingMs(p: CyberPoste, now: number): number | null {
  if (!p.paused) return null;
  if (!p.pausedUntil) return null; // illimité
  const until = Date.parse(p.pausedUntil);
  if (Number.isNaN(until)) return null;
  return Math.max(0, until - now);
}

export default function CyberView() {
  const { t, tf, lang } = useI18n();
  const queryClient = useQueryClient();
  const currency = useCurrency();
  const { data: settings, isLoading: settingsLoading } = useSettings();
  const cyberEnabled = settings?.tenant?.cyberEnabled === true;

  const [refreshMs, setRefreshMs] = useState(30000);
  const [now, setNow] = useState(() => Date.now());
  const [query, setQuery] = useState("");
  const [busyId, setBusyId] = useState<string | null>(null);

  // Horloge locale (1 s) : fait vivre les comptes à rebours de pause.
  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(timer);
  }, []);

  // — Activation / désactivation du module (rang 3 serveur) —
  const toggleModule = useMutation({
    mutationFn: (enabled: boolean) => setCyberEnabled(enabled),
    onSuccess: (_res, enabled) => {
      toast.success(t(enabled ? "cyber.activate.toast" : "cyber.deactivate.toast"));
      void queryClient.invalidateQueries({ queryKey: SETTINGS_QUERY_KEY });
      void queryClient.invalidateQueries({ queryKey: ["/api/cyber/postes"] });
    },
    onError: (err) => toast.error(err instanceof Error ? err.message : t("common.error")),
  });

  // — Données (la caisse jour suit les buckets N°198, fuseau du compte) —
  const { data: postesData, isLoading: postesLoading } = useQuery({
    queryKey: ["/api/cyber/postes"],
    queryFn: () => fetchCyberPostes(),
    refetchInterval: refreshMs,
    enabled: cyberEnabled,
  });
  const { data: routers } = useQuery({
    queryKey: ["/api/routers"],
    queryFn: () => api<RouterDevice[]>("/api/routers"),
    refetchInterval: 30_000,
    enabled: cyberEnabled,
  });
  const { data: accounting } = useQuery({
    queryKey: ["/api/accounting", "day"],
    queryFn: () => api<AccountingDayResponse>("/api/accounting?period=day"),
    refetchInterval: 60_000,
    enabled: cyberEnabled,
  });

  const postes = useMemo(() => postesData ?? [], [postesData]);
  const agentRouterCount = useMemo(() => (routers ?? []).filter((r) => r.mode === "agent").length, [routers]);

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return postes;
    return postes.filter(
      (p) =>
        posteLabel(p).toLowerCase().includes(q) ||
        p.mac.toLowerCase().includes(q) ||
        p.ip.toLowerCase().includes(q) ||
        p.routerName.toLowerCase().includes(q),
    );
  }, [postes, query]);

  const busyCount = postes.filter((p) => p.linkedUser).length;
  const pausedCount = postes.filter((p) => p.paused).length;
  const today = accounting?.series?.length ? accounting.series[accounting.series.length - 1] : undefined;

  // — Mutations (jamais d'optimisme : l'état réel revient du poll) —
  const pauseMutation = useMutation({
    mutationFn: ({ id, paused, minutes }: { id: string; paused: boolean; minutes: number }) =>
      pauseCyberPoste(id, paused, minutes),
    onSuccess: (_res, vars) => {
      const poste = postes.find((p) => p.id === vars.id);
      const label = poste ? posteLabel(poste) : "";
      if (vars.paused) {
        const preset = PAUSE_PRESETS.find((p) => p.minutes === vars.minutes);
        if (preset && vars.minutes > 0) {
          toast.success(tf("devices.pauseToast", { device: label, duration: t(preset.key) }));
        } else {
          toast.success(tf("devices.pauseForeverToast", { device: label }));
        }
      } else {
        toast.success(tf("devices.resumeToast", { device: label }));
      }
      void queryClient.invalidateQueries({ queryKey: ["/api/cyber/postes"] });
    },
    onError: (err) => toast.error(err instanceof Error ? err.message : t("common.error")),
  });

  const releaseMutation = useMutation({
    mutationFn: (id: string) => releaseCyberPoste(id),
    onSuccess: () => {
      toast.success(t("cyber.releaseToast"));
      void queryClient.invalidateQueries({ queryKey: ["/api/cyber/postes"] });
    },
    onError: (err) => toast.error(err instanceof Error ? err.message : t("common.error")),
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => deleteCyberPoste(id),
    onSuccess: () => {
      toast.success(t("cyber.deleteToast"));
      void queryClient.invalidateQueries({ queryKey: ["/api/cyber/postes"] });
    },
    onError: (err) => toast.error(err instanceof Error ? err.message : t("common.error")),
  });

  const renameMutation = useMutation({
    mutationFn: ({ id, name }: { id: string; name: string }) => renameCyberPoste(id, name),
    onSuccess: () => {
      toast.success(t("cyber.renameToast"));
      void queryClient.invalidateQueries({ queryKey: ["/api/cyber/postes"] });
    },
    onError: (err) => toast.error(err instanceof Error ? err.message : t("common.error")),
  });

  // — Dialog « Ajouter un poste » —
  const [addOpen, setAddOpen] = useState(false);
  const [addMac, setAddMac] = useState("");
  const [addName, setAddName] = useState("");
  const [addRouterId, setAddRouterId] = useState("");
  const addMutation = useMutation({
    mutationFn: () =>
      createCyberPoste({ mac: addMac.trim(), name: addName.trim(), routerId: addRouterId }),
    onSuccess: () => {
      toast.success(t("cyber.addToast"));
      void queryClient.invalidateQueries({ queryKey: ["/api/cyber/postes"] });
      setAddOpen(false);
      setAddMac("");
      setAddName("");
    },
    onError: (err) => toast.error(err instanceof Error ? err.message : t("common.error")),
  });

  // — Dialog « Importer (DHCP) » —
  const [discoverOpen, setDiscoverOpen] = useState(false);
  const { data: discovered, isLoading: discoverLoading } = useQuery({
    queryKey: ["/api/cyber/discover"],
    queryFn: () => fetchCyberDiscover(),
    enabled: discoverOpen && cyberEnabled,
    refetchInterval: 15_000,
  });
  const importMutation = useMutation({
    mutationFn: (deviceId: string) => importCyberPoste(deviceId),
    onSuccess: () => {
      toast.success(t("cyber.importToast"));
      void queryClient.invalidateQueries({ queryKey: ["/api/cyber/postes"] });
      void queryClient.invalidateQueries({ queryKey: ["/api/cyber/discover"] });
    },
    onError: (err) => toast.error(err instanceof Error ? err.message : t("common.error")),
  });

  // — Dialog « Attribuer un code-temps » —
  const [assignTarget, setAssignTarget] = useState<CyberPoste | null>(null);
  const [assignProfileId, setAssignProfileId] = useState("");
  const [assignMinutes, setAssignMinutes] = useState("");
  const [assigned, setAssigned] = useState<{ code: string; price: number } | null>(null);
  const { data: profiles } = useQuery({
    queryKey: ["/api/profiles"],
    queryFn: () => api<Profile[]>("/api/profiles"),
    enabled: cyberEnabled && assignTarget !== null,
  });
  const assignMutation = useMutation({
    mutationFn: () =>
      assignCyberPoste(
        assignTarget!.id,
        assignProfileId,
        assignMinutes.trim() === "" ? 0 : Math.max(0, Number(assignMinutes)),
      ),
    onSuccess: (res) => {
      toast.success(t("cyber.assignToast"));
      void queryClient.invalidateQueries({ queryKey: ["/api/cyber/postes"] });
      void queryClient.invalidateQueries({ queryKey: ["/api/accounting"] });
      setAssigned({ code: res.voucher.username, price: res.totalCost });
    },
    onError: (err) => toast.error(err instanceof Error ? err.message : t("common.error")),
  });
  const openAssign = (p: CyberPoste) => {
    setAssignTarget(p);
    setAssignProfileId("");
    setAssignMinutes("");
    setAssigned(null);
  };
  const profilePrice = (p: Profile) => (p.sellingPrice > 0 ? p.sellingPrice : p.price);

  // — Dialog « Renommer » —
  const [renameTarget, setRenameTarget] = useState<CyberPoste | null>(null);
  const [renameValue, setRenameValue] = useState("");
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

  // — Confirmations (libérer / supprimer) —
  const [releaseTarget, setReleaseTarget] = useState<CyberPoste | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<CyberPoste | null>(null);

  // — États d'attente des réglages / module éteint —
  if (settingsLoading) {
    return (
      <div className="space-y-4 sm:space-y-6">
        <PageHeader title={t("cyber.title")} description={t("cyber.description")} />
        <Card className="gap-0 py-0">
          <LoadingRows rows={6} />
        </Card>
      </div>
    );
  }
  if (!cyberEnabled) {
    return (
      <div className="space-y-4 sm:space-y-6">
        <PageHeader title={t("cyber.title")} description={t("cyber.description")} />
        <Card className="mx-auto max-w-2xl p-6 text-center">
          <Armchair className="mx-auto size-10 text-muted-foreground" aria-hidden />
          <h2 className="mt-3 text-lg font-semibold">{t("cyber.activate.title")}</h2>
          <p className="mx-auto mt-2 max-w-md text-sm text-muted-foreground">{t("cyber.activate.desc")}</p>
          <Button
            className="mt-5"
            disabled={toggleModule.isPending}
            onClick={() => toggleModule.mutate(true)}
          >
            <BadgeCheck className="size-4" aria-hidden />
            {t("cyber.activate.cta")}
          </Button>
        </Card>
      </div>
    );
  }

  return (
    <div className="space-y-4 sm:space-y-6">
      <PageHeader
        title={t("cyber.title")}
        description={t("cyber.description")}
        actions={
          <>
            <Button size="sm" className="h-10 gap-1.5" onClick={() => setAddOpen(true)}>
              <Plus className="size-4" aria-hidden />
              {t("cyber.add")}
            </Button>
            <Button size="sm" variant="outline" className="h-10 gap-1.5" onClick={() => setDiscoverOpen(true)}>
              <Download className="size-4" aria-hidden />
              {t("cyber.import")}
            </Button>
            <div className="relative">
              <Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" aria-hidden />
              <Input
                value={query}
                onChange={(event) => setQuery(event.target.value)}
                placeholder={t("cyber.search")}
                className="h-10 w-40 pl-9 sm:w-56"
                aria-label={t("cyber.search")}
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

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <StatCard title={t("cyber.kpi.total")} value={String(postes.length)} sub={t("cyber.kpi.totalSub")} icon={Armchair} />
        <StatCard title={t("cyber.kpi.busy")} value={String(busyCount)} sub={t("cyber.kpi.busySub")} icon={Play} live />
        <StatCard title={t("cyber.kpi.paused")} value={String(pausedCount)} sub={t("cyber.kpi.pausedSub")} icon={Pause} />
        <StatCard
          title={t("cyber.kpi.caisse")}
          value={today ? formatCurrency(today.revenue, currency, lang) : "—"}
          sub={t("cyber.kpi.caisseSub")}
          icon={Wallet}
        />
      </div>

      <Card className="gap-0 py-0">
        {postesLoading ? (
          <LoadingRows rows={6} />
        ) : filtered.length === 0 ? (
          postes.length === 0 ? (
            agentRouterCount === 0 ? (
              <EmptyState icon={Armchair} title={t("cyber.empty.routerTitle")} description={t("cyber.empty.routerDesc")} />
            ) : (
              <EmptyState icon={Armchair} title={t("cyber.empty.title")} description={t("cyber.empty.desc")} />
            )
          ) : (
            <EmptyState icon={Search} title={t("cyber.noMatch")} description={t("cyber.noMatchDesc")} />
          )
        ) : (
          <>
            <Table>
              <TableHeader>
                <TableRow className="hover:bg-transparent">
                  <TableHead className="pl-4 text-muted-foreground sm:pl-6">{t("cyber.poste")}</TableHead>
                  <TableHead className="text-muted-foreground">{t("cyber.status")}</TableHead>
                  <TableHead className="hidden text-muted-foreground md:table-cell">{t("cyber.code")}</TableHead>
                  <TableHead className="hidden text-muted-foreground xl:table-cell">{t("cyber.router")}</TableHead>
                  <TableHead className="hidden font-mono text-muted-foreground lg:table-cell">MAC</TableHead>
                  <TableHead className="pr-4 text-right text-muted-foreground sm:pr-6">{t("cyber.actions")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {filtered.map((poste) => {
                  const remaining = pauseRemainingMs(poste, now);
                  const busy = busyId === poste.id || pauseMutation.isPending || deleteMutation.isPending;
                  const linked = poste.linkedUser;
                  return (
                    <TableRow key={poste.id} className="transition-colors hover:bg-muted/50">
                      <TableCell className="max-w-56 pl-4 sm:pl-6">
                        <span className="flex items-center gap-2">
                          <Armchair className="size-4 shrink-0 text-muted-foreground" aria-hidden />
                          <span className="min-w-0">
                            <span className="block truncate text-sm font-medium" title={posteLabel(poste)}>
                              {posteLabel(poste)}
                            </span>
                            {poste.name && (
                              <span className="block truncate font-mono text-xs text-muted-foreground" title={poste.mac}>
                                {poste.mac}
                              </span>
                            )}
                          </span>
                        </span>
                      </TableCell>
                      <TableCell>
                        {poste.paused ? (
                          <Badge className="gap-1.5 border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-300" variant="outline">
                            <Pause className="size-3" aria-hidden />
                            {t("cyber.status.paused")}
                            {remaining !== null && remaining > 0 && (
                              <span className="tabular-nums opacity-80">· {formatDuration(Math.floor(remaining / 1000))}</span>
                            )}
                          </Badge>
                        ) : linked ? (
                          linked.status === "online" ? (
                            <Badge className="gap-1.5 border-primary/25 bg-primary/10 text-primary" variant="outline">
                              <span className="live-dot size-1.5 rounded-full bg-primary" aria-hidden />
                              {t("cyber.status.busy")}
                            </Badge>
                          ) : (
                            <Badge
                              variant="outline"
                              className={
                                linked.status === "expired"
                                  ? "gap-1.5 border-destructive/30 bg-destructive/10 text-destructive"
                                  : "gap-1.5 text-muted-foreground"
                              }
                            >
                              {t(linked.status === "expired" ? "cyber.status.expired" : `cyber.status.${linked.status}`)}
                            </Badge>
                          )
                        ) : (
                          <Badge variant="outline" className="gap-1.5 text-muted-foreground">
                            {t("cyber.status.free")}
                          </Badge>
                        )}
                      </TableCell>
                      <TableCell className="hidden md:table-cell">
                        {linked ? (
                          <span className="flex flex-col">
                            <span className="font-mono text-sm">{linked.username}</span>
                            {linked.timeLimitMin > 0 && (
                              <span className="text-xs tabular-nums text-muted-foreground">
                                {tf("cyber.code.remaining", {
                                  used: String(Math.floor(linked.uptimeUsedSec / 60)),
                                  limit: String(linked.timeLimitMin),
                                })}
                              </span>
                            )}
                          </span>
                        ) : (
                          <span className="text-muted-foreground">—</span>
                        )}
                      </TableCell>
                      <TableCell className="hidden max-w-40 truncate text-muted-foreground xl:table-cell">
                        {poste.routerName}
                      </TableCell>
                      <TableCell className="hidden font-mono text-muted-foreground lg:table-cell">{poste.mac}</TableCell>
                      <TableCell className="pr-4 sm:pr-6">
                        <div className="flex items-center justify-end gap-1">
                          {!poste.linkedUser && !poste.paused && (
                            <Button
                              size="sm"
                              className="h-8 gap-1.5"
                              disabled={busy}
                              onClick={() => openAssign(poste)}
                            >
                              <Wallet className="size-3.5" aria-hidden />
                              {t("cyber.assign")}
                            </Button>
                          )}
                          {poste.linkedUser && (
                            <Button
                              size="sm"
                              variant="outline"
                              className="h-8 gap-1.5"
                              disabled={busy}
                              onClick={() => setReleaseTarget(poste)}
                            >
                              {t("cyber.release")}
                            </Button>
                          )}
                          {poste.paused ? (
                            <Button
                              size="sm"
                              variant="outline"
                              className="h-8 gap-1.5"
                              disabled={busy}
                              onClick={() => pauseMutation.mutate({ id: poste.id, paused: false, minutes: 0 })}
                            >
                              <Play className="size-3.5" aria-hidden />
                              {t("cyber.resume")}
                            </Button>
                          ) : (
                            <DropdownMenu>
                              <DropdownMenuTrigger asChild>
                                <Button size="sm" variant="outline" className="h-8 gap-1.5" disabled={busy}>
                                  <Pause className="size-3.5" aria-hidden />
                                  {t("cyber.pause")}
                                </Button>
                              </DropdownMenuTrigger>
                              <DropdownMenuContent align="end" className="w-52">
                                {PAUSE_PRESETS.map((preset) => (
                                  <DropdownMenuItem
                                    key={preset.minutes}
                                    onClick={() => pauseMutation.mutate({ id: poste.id, paused: true, minutes: preset.minutes })}
                                  >
                                    <span className="tabular-nums">{t(preset.key)}</span>
                                  </DropdownMenuItem>
                                ))}
                                <DropdownMenuSeparator />
                                <DropdownMenuItem disabled className="text-xs text-muted-foreground">
                                  {t("cyber.pauseHint")}
                                </DropdownMenuItem>
                              </DropdownMenuContent>
                            </DropdownMenu>
                          )}
                          <DropdownMenu>
                            <DropdownMenuTrigger asChild>
                              <Button
                                size="icon"
                                variant="ghost"
                                className="size-8"
                                aria-label={t("cyber.more")}
                                title={t("cyber.more")}
                                disabled={busy}
                              >
                                <MoreHorizontal className="size-4" aria-hidden />
                              </Button>
                            </DropdownMenuTrigger>
                            <DropdownMenuContent align="end">
                              <DropdownMenuItem
                                onClick={() => {
                                  setRenameTarget(poste);
                                  setRenameValue(poste.name);
                                }}
                              >
                                <Pencil className="size-3.5" aria-hidden />
                                {t("cyber.rename")}
                              </DropdownMenuItem>
                              <DropdownMenuSeparator />
                              <DropdownMenuItem className="text-destructive" onClick={() => setDeleteTarget(poste)}>
                                <Trash2 className="size-3.5" aria-hidden />
                                {t("cyber.delete")}
                              </DropdownMenuItem>
                            </DropdownMenuContent>
                          </DropdownMenu>
                        </div>
                      </TableCell>
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
            {/* Honnêteté de la convergence + rappel de la caisse. */}
            <p className="border-t px-4 py-2.5 text-xs text-muted-foreground sm:px-6">{t("cyber.pauseHint")}</p>
          </>
        )}
      </Card>

      <p className="px-1 text-xs text-muted-foreground">{t("cyber.settingsHint")}</p>
      <Button
        size="sm"
        variant="ghost"
        className="h-8 text-muted-foreground"
        disabled={toggleModule.isPending}
        onClick={() => toggleModule.mutate(false)}
      >
        {t("cyber.deactivate.cta")}
      </Button>

      {/* — Dialog « Ajouter un poste » — */}
      <Dialog open={addOpen} onOpenChange={setAddOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{t("cyber.addTitle")}</DialogTitle>
            <DialogDescription>{t("cyber.addDesc")}</DialogDescription>
          </DialogHeader>
          <div className="space-y-3 py-1">
            <div className="space-y-1.5">
              <Label htmlFor="cyber-add-mac">{t("cyber.addMac")}</Label>
              <Input
                id="cyber-add-mac"
                value={addMac}
                onChange={(event) => setAddMac(event.target.value)}
                placeholder={t("cyber.addMacPlaceholder")}
                className="font-mono uppercase"
                autoFocus
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="cyber-add-name">{t("cyber.addName")}</Label>
              <Input
                id="cyber-add-name"
                value={addName}
                onChange={(event) => setAddName(event.target.value)}
                placeholder={t("cyber.addNamePlaceholder")}
                maxLength={48}
              />
            </div>
            <div className="space-y-1.5">
              <Label>{t("cyber.addRouter")}</Label>
              <Select value={addRouterId} onValueChange={setAddRouterId}>
                <SelectTrigger className="w-full">
                  <SelectValue placeholder={t("cyber.addRouter")} />
                </SelectTrigger>
                <SelectContent>
                  {(routers ?? []).map((r) => (
                    <SelectItem key={r.id} value={r.id}>
                      {r.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setAddOpen(false)}>
              {t("common.cancel")}
            </Button>
            <Button
              onClick={() => addMutation.mutate()}
              disabled={addMutation.isPending || addMac.trim() === "" || addRouterId === ""}
            >
              {t("cyber.addSave")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* — Dialog « Importer (DHCP) » — */}
      <Dialog open={discoverOpen} onOpenChange={setDiscoverOpen}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>{t("cyber.discoverTitle")}</DialogTitle>
            <DialogDescription>{t("cyber.discoverDesc")}</DialogDescription>
          </DialogHeader>
          <div className="max-h-80 space-y-1.5 overflow-y-auto py-1">
            {discoverLoading ? (
              <LoadingRows rows={4} />
            ) : (discovered ?? []).length === 0 ? (
              <p className="py-6 text-center text-sm text-muted-foreground">{t("cyber.discoverEmpty")}</p>
            ) : (
              (discovered ?? []).map((d: CyberDiscoveryRow) => (
                <div key={d.id} className="flex items-center justify-between gap-3 rounded-lg border px-3 py-2">
                  <div className="min-w-0">
                    <p className="truncate text-sm font-medium">{d.hostname || d.mac}</p>
                    <p className="truncate font-mono text-xs text-muted-foreground">
                      {d.mac} · {d.ip || "—"} · {d.routerName}
                    </p>
                  </div>
                  {d.imported ? (
                    <Badge variant="outline" className="gap-1 text-muted-foreground">
                      <Check className="size-3" aria-hidden />
                      {t("cyber.imported")}
                    </Badge>
                  ) : (
                    <Button
                      size="sm"
                      variant="outline"
                      className="h-8"
                      disabled={importMutation.isPending}
                      onClick={() => importMutation.mutate(d.id)}
                    >
                      {t("cyber.importCta")}
                    </Button>
                  )}
                </div>
              ))
            )}
          </div>
        </DialogContent>
      </Dialog>

      {/* — Dialog « Attribuer un code-temps » (formulaire puis code attribué) — */}
      <Dialog
        open={assignTarget !== null}
        onOpenChange={(open) => {
          if (!open) setAssignTarget(null);
        }}
      >
        <DialogContent className="sm:max-w-md">
          {assigned === null ? (
            <>
              <DialogHeader>
                <DialogTitle>{t("cyber.assignTitle")}</DialogTitle>
                <DialogDescription>{t("cyber.assignDesc")}</DialogDescription>
              </DialogHeader>
              <div className="space-y-3 py-1">
                <p className="text-sm">
                  <span className="text-muted-foreground">{t("cyber.poste")} :</span>{" "}
                  <span className="font-medium">{assignTarget ? posteLabel(assignTarget) : ""}</span>
                </p>
                <div className="space-y-1.5">
                  <Label>{t("cyber.assignProfile")}</Label>
                  <Select value={assignProfileId} onValueChange={setAssignProfileId}>
                    <SelectTrigger className="w-full">
                      <SelectValue placeholder={t("cyber.assignProfile")} />
                    </SelectTrigger>
                    <SelectContent>
                      {(profiles ?? []).map((p) => (
                        <SelectItem key={p.id} value={p.id}>
                          {p.name} · {formatCurrency(profilePrice(p), currency, lang)}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
                <div className="space-y-1.5">
                  <Label htmlFor="cyber-assign-minutes">{t("cyber.assignTime")}</Label>
                  <Input
                    id="cyber-assign-minutes"
                    type="number"
                    min={0}
                    value={assignMinutes}
                    onChange={(event) => setAssignMinutes(event.target.value)}
                    placeholder={t("cyber.assignTimePlaceholder")}
                  />
                </div>
              </div>
              <DialogFooter>
                <Button variant="outline" onClick={() => setAssignTarget(null)}>
                  {t("common.cancel")}
                </Button>
                <Button
                  onClick={() => assignMutation.mutate()}
                  disabled={assignMutation.isPending || assignProfileId === ""}
                >
                  {t("cyber.assignCta")}
                </Button>
              </DialogFooter>
            </>
          ) : (
            <>
              <DialogHeader>
                <DialogTitle>{t("cyber.assignedTitle")}</DialogTitle>
                <DialogDescription>{t("cyber.assignedDesc")}</DialogDescription>
              </DialogHeader>
              <div className="space-y-3 py-2 text-center">
                <p className="text-xs uppercase tracking-wide text-muted-foreground">{t("cyber.assignedCode")}</p>
                <p className="font-mono text-3xl font-bold tracking-widest">{assigned.code}</p>
                <p className="text-sm text-muted-foreground">
                  {t("cyber.assignedPrice")} :{" "}
                  <span className="font-semibold text-foreground">{formatCurrency(assigned.price, currency, lang)}</span>
                </p>
                <Button
                  variant="outline"
                  className="gap-1.5"
                  onClick={() => {
                    void copyToClipboard(assigned.code);
                    toast.success(t("cyber.assignedCopied"));
                  }}
                >
                  <Copy className="size-3.5" aria-hidden />
                  {t("cyber.assignedCopy")}
                </Button>
                <p className="text-xs text-muted-foreground">{t("cyber.assignedHint")}</p>
              </div>
              <DialogFooter>
                <Button onClick={() => setAssignTarget(null)}>{t("common.close")}</Button>
              </DialogFooter>
            </>
          )}
        </DialogContent>
      </Dialog>

      {/* — Dialog « Renommer » — */}
      <Dialog open={renameTarget !== null} onOpenChange={(open) => !open && setRenameTarget(null)}>
        <DialogContent className="sm:max-w-sm">
          <DialogHeader>
            <DialogTitle>{t("cyber.renameTitle")}</DialogTitle>
          </DialogHeader>
          <div className="space-y-2 py-1">
            <Input
              value={renameValue}
              onChange={(event) => setRenameValue(event.target.value)}
              placeholder={t("cyber.renamePlaceholder")}
              maxLength={48}
              autoFocus
              onKeyDown={(event) => {
                if (event.key === "Enter") submitRename();
              }}
            />
            <p className="text-xs text-muted-foreground">
              {renameTarget && (
                <>
                  MAC : <span className="font-mono">{renameTarget.mac}</span>
                </>
              )}
            </p>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setRenameTarget(null)}>
              {t("common.cancel")}
            </Button>
            <Button onClick={submitRename} disabled={renameMutation.isPending}>
              {t("cyber.renameSave")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* — Confirmation « Libérer » — */}
      <AlertDialog open={releaseTarget !== null} onOpenChange={(open) => !open && setReleaseTarget(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t("cyber.releaseConfirmTitle")}</AlertDialogTitle>
            <AlertDialogDescription>
              {releaseTarget ? `${posteLabel(releaseTarget)} — ${releaseTarget.activeUsername}` : ""}
              <br />
              {t("cyber.releaseConfirmDesc")}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
            <AlertDialogAction
              onClick={() => {
                if (!releaseTarget) return;
                setBusyId(releaseTarget.id);
                releaseMutation.mutate(releaseTarget.id, { onSettled: () => setBusyId(null) });
                setReleaseTarget(null);
              }}
            >
              {t("cyber.release")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* — Confirmation « Supprimer » — */}
      <AlertDialog open={deleteTarget !== null} onOpenChange={(open) => !open && setDeleteTarget(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t("cyber.deleteConfirmTitle")}</AlertDialogTitle>
            <AlertDialogDescription>
              {deleteTarget ? posteLabel(deleteTarget) : ""}
              <br />
              {t("cyber.deleteConfirmDesc")}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-white hover:bg-destructive/90"
              onClick={() => {
                if (!deleteTarget) return;
                setBusyId(deleteTarget.id);
                deleteMutation.mutate(deleteTarget.id, { onSettled: () => setBusyId(null) });
                setDeleteTarget(null);
              }}
            >
              {t("cyber.delete")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
