"use client";

// Onglet N°103/N°104 — Qualité de ligne & QoS Manager.
//
// DEUX cartes, UNE vérité : la capacité de la ligne (mesure passive N°103 —
// déclarée par le gérant ou observée sur le WAN) alimente la recommandation
// du plafond agrégat (N°104 — file mikcloud-qos convergée au check-in).
// La recommandation n'est JAMAIS inventée : sans base honnête, rien n'est
// proposé. Toute action est explicitement confirmée (geste de gérant) et la
// convergence routeur (≤ 45 s) est annoncée, jamais attendue bloquée.

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ArrowDown,
  ArrowUp,
  Gauge,
  LineChart,
  Loader2,
  Pencil,
  Save,
  ShieldCheck,
  X,
} from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";
import { EmptyState } from "@/components/hotspot/empty-state";
import { StatusBadge } from "@/components/hotspot/status-badge";
import { api } from "@/lib/hotspot/api";
import { useI18n } from "@/lib/hotspot/i18n";
import { formatBitsPerSec, formatBytes, timeAgo } from "@/lib/hotspot/format";
import type { RouterDevice, RouterLineQuality, RouterQoS } from "@/lib/hotspot/types";
import { ToolError, UnsupportedState } from "./shared";

// bps → Mbps entier pour les champs de saisie (la saisie est en Mbps, le
// contrat API en bits/s — l'aller-retour reste en nombres exacts).
const bpsToMbps = (bps: number): string => (bps > 0 ? String(Math.round(bps / 1_000_000)) : "");
const mbpsToBps = (mbps: string): number => {
  const n = Number.parseInt(mbps.replace(",", "."), 10);
  return Number.isFinite(n) && n > 0 ? n * 1_000_000 : 0;
};

export function QoSTab({ router }: { router: RouterDevice }) {
  const { t, tf, lang } = useI18n();
  const queryClient = useQueryClient();

  // ─── Qualité de ligne (N°103) ───
  const lineQuery = useQuery({
    queryKey: ["/api/routers", router.id, "line-quality"],
    queryFn: () => api<RouterLineQuality>(`/api/routers/${router.id}/line-quality`),
    enabled: router.mode !== "real",
    refetchInterval: 30_000,
  });

  // ─── QoS Manager (N°104) ───
  const qosQuery = useQuery({
    queryKey: ["/api/routers", router.id, "qos"],
    queryFn: () => api<RouterQoS>(`/api/routers/${router.id}/qos`),
    enabled: router.mode !== "real",
    refetchInterval: 30_000,
  });

  const invalidateQoS = () => {
    void queryClient.invalidateQueries({ queryKey: ["/api/routers", router.id, "qos"] });
    void queryClient.invalidateQueries({ queryKey: ["/api/routers", router.id, "line-quality"] });
    void queryClient.invalidateQueries({ queryKey: ["/api/routers"] });
  };

  const failure = (err: Error) =>
    toast.error(err.message || t("tools.qos.failDefault"));

  // Déclaration du forfait FAI (PUT /api/routers/{id} — champs N°103).
  const [editingPlan, setEditingPlan] = useState(false);
  const [planDown, setPlanDown] = useState("");
  const [planUp, setPlanUp] = useState("");
  const planMutation = useMutation({
    mutationFn: () =>
      api<RouterDevice>(`/api/routers/${router.id}`, {
        method: "PUT",
        body: { lineDownBps: mbpsToBps(planDown), lineUpBps: mbpsToBps(planUp) },
      }),
    onSuccess: () => {
      toast.success(t("tools.qos.declaredToast"));
      setEditingPlan(false);
      invalidateQoS();
    },
    onError: failure,
  });

  // Application de la QoS (recommandation ou valeurs saisies).
  const [target, setTarget] = useState("");
  const [maxDown, setMaxDown] = useState("");
  const [maxUp, setMaxUp] = useState("");
  const qosMutation = useMutation({
    mutationFn: (body: Record<string, unknown>) =>
      api<{ ok: boolean }>(`/api/routers/${router.id}/qos`, { method: "PUT", body }),
    onSuccess: () => {
      toast.success(t("tools.qos.applyToast"));
      invalidateQoS();
    },
    onError: failure,
  });
  const disableMutation = useMutation({
    mutationFn: () => api<{ ok: boolean }>(`/api/routers/${router.id}/qos`, { method: "DELETE" }),
    onSuccess: () => {
      toast.success(t("tools.qos.disableToast"));
      invalidateQoS();
    },
    onError: failure,
  });

  const line = lineQuery.data;
  const qos = qosQuery.data;
  const rec = qos?.recommendation;

  if (router.mode === "real") {
    return <UnsupportedState />;
  }
  if (lineQuery.isLoading || qosQuery.isLoading) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-40 rounded-lg" />
        <Skeleton className="h-64 rounded-lg" />
      </div>
    );
  }
  if (lineQuery.isError) {
    return <ToolError error={lineQuery.error} onRetry={() => void lineQuery.refetch()} />;
  }
  if (qosQuery.isError) {
    return <ToolError error={qosQuery.error} onRetry={() => void qosQuery.refetch()} />;
  }

  const startEditPlan = () => {
    setPlanDown(bpsToMbps(line?.configured.downBps ?? 0));
    setPlanUp(bpsToMbps(line?.configured.upBps ?? 0));
    setEditingPlan(true);
  };

  const applyRecommended = () => {
    if (!rec || rec.source === "none") return;
    qosMutation.mutate({
      enabled: true,
      target: qos?.status.target || target || "192.168.10.0/24",
      maxDownBps: rec.maxDownBps,
      maxUpBps: rec.maxUpBps,
    });
  };

  const applyCustom = () => {
    qosMutation.mutate({
      enabled: true,
      target: target.trim(),
      maxDownBps: mbpsToBps(maxDown),
      maxUpBps: mbpsToBps(maxUp),
    });
  };

  return (
    <div className="space-y-6">
      {/* ─── Carte 1 : Qualité de ligne (N°103) ─── */}
      <section className="rounded-lg border p-4">
        <div className="flex flex-wrap items-start justify-between gap-2">
          <div>
            <h3 className="flex items-center gap-1.5 text-sm font-semibold">
              <LineChart className="size-4 text-primary" aria-hidden />
              {t("tools.qos.lineTitle")}
            </h3>
            <p className="mt-0.5 max-w-xl text-xs text-muted-foreground">{t("tools.qos.lineDesc")}</p>
          </div>
        </div>

        <div className="mt-3 grid grid-cols-2 gap-3 lg:grid-cols-4">
          {/* WAN détecté */}
          <div className="rounded-lg border p-3">
            <p className="text-xs text-muted-foreground">{t("tools.qos.wan")}</p>
            <p className="mt-1 truncate font-mono text-sm font-semibold">{line?.wanIface || "—"}</p>
            {!line?.wanIface && (
              <p className="mt-1 text-[11px] leading-tight text-muted-foreground">{t("tools.qos.wanNone")}</p>
            )}
          </div>
          {/* Débit live */}
          <div className="rounded-lg border p-3">
            <p className="flex items-center gap-1 text-xs text-muted-foreground">
              <ArrowDown className="size-3" aria-hidden /> {t("tools.qos.live")}
            </p>
            <p className="mt-1 text-sm font-semibold tabular-nums text-emerald-600 dark:text-emerald-400">
              {formatBitsPerSec(line?.live.rxBps ?? 0)}
            </p>
            <p className="flex items-center gap-1 text-xs tabular-nums text-amber-600 dark:text-amber-400">
              <ArrowUp className="size-3" aria-hidden /> {formatBitsPerSec(line?.live.txBps ?? 0)}
            </p>
          </div>
          {/* Capacité observée */}
          <div className="rounded-lg border p-3">
            <p className="text-xs text-muted-foreground">{t("tools.qos.measured")}</p>
            {line?.measured.days && line.measured.days > 0 ? (
              <>
                <p className="mt-1 text-sm font-semibold tabular-nums text-emerald-600 dark:text-emerald-400">
                  {formatBitsPerSec(line.measured.downBps)}
                </p>
                <p className="text-xs tabular-nums text-amber-600 dark:text-amber-400">
                  {formatBitsPerSec(line.measured.upBps)}
                </p>
                <p className="mt-1 text-[11px] text-muted-foreground">
                  {tf("tools.qos.measuredDays", { days: line.measured.days })} ·{" "}
                  {line.measured.confident ? t("tools.qos.confident") : t("tools.qos.notConfident")}
                </p>
              </>
            ) : (
              <p className="mt-1 text-[11px] leading-tight text-muted-foreground">{t("tools.qos.measuredNone")}</p>
            )}
          </div>
          {/* Forfait déclaré */}
          <div className="rounded-lg border p-3">
            <p className="text-xs text-muted-foreground">{t("tools.qos.declared")}</p>
            {editingPlan ? (
              <div className="mt-1 space-y-1.5">
                <div className="flex items-center gap-1">
                  <Input
                    inputMode="numeric"
                    value={planDown}
                    onChange={(e) => setPlanDown(e.target.value)}
                    className="h-7 w-full text-xs tabular-nums"
                    aria-label={t("tools.qos.maxDown")}
                    placeholder="110"
                  />
                  <span className="text-xs text-muted-foreground">/</span>
                  <Input
                    inputMode="numeric"
                    value={planUp}
                    onChange={(e) => setPlanUp(e.target.value)}
                    className="h-7 w-full text-xs tabular-nums"
                    aria-label={t("tools.qos.maxUp")}
                    placeholder="20"
                  />
                  <span className="shrink-0 text-[10px] text-muted-foreground">Mbps</span>
                </div>
                <p className="text-[11px] text-muted-foreground">{t("tools.qos.declaredHint")}</p>
                <div className="flex gap-1.5">
                  <Button
                    size="sm"
                    className="h-7 px-2 text-xs"
                    disabled={planMutation.isPending}
                    onClick={() => planMutation.mutate()}
                  >
                    {planMutation.isPending ? (
                      <Loader2 className="size-3 animate-spin" />
                    ) : (
                      <Save className="size-3" />
                    )}
                    {t("tools.qos.declaredSave")}
                  </Button>
                  <Button size="sm" variant="ghost" className="h-7 px-2 text-xs" onClick={() => setEditingPlan(false)}>
                    <X className="size-3" />
                    {t("tools.qos.declaredCancel")}
                  </Button>
                </div>
              </div>
            ) : line?.configured.downBps ? (
              <button
                type="button"
                onClick={startEditPlan}
                className="group mt-1 flex items-center gap-1 text-left"
                title={t("tools.qos.declaredEdit")}
              >
                <p className="text-sm font-semibold tabular-nums">
                  {Math.round(line.configured.downBps / 1_000_000)}/{Math.round(line.configured.upBps / 1_000_000)} Mbps
                </p>
                <Pencil className="size-3 text-muted-foreground opacity-0 transition-opacity group-hover:opacity-100" aria-hidden />
              </button>
            ) : (
              <Button size="sm" variant="outline" className="mt-1 h-7 px-2 text-xs" onClick={startEditPlan}>
                <Pencil className="size-3" />
                {t("tools.qos.declaredEdit")}
              </Button>
            )}
          </div>
        </div>

        {/* Historique quotidien (14 j) */}
        {line?.days && line.days.length > 0 && (
          <div className="mt-3 max-h-40 overflow-y-auto rounded-lg border">
            <Table>
              <TableHeader>
                <TableRow className="hover:bg-transparent">
                  <TableHead className="pl-4 text-muted-foreground">{t("tools.qos.measured")}</TableHead>
                  <TableHead className="text-right text-muted-foreground">Max ↓</TableHead>
                  <TableHead className="text-right text-muted-foreground">Max ↑</TableHead>
                  <TableHead className="hidden text-right text-muted-foreground sm:table-cell">
                    {tf("tools.qos.p95", { value: "↓" })}
                  </TableHead>
                  <TableHead className="hidden pr-4 text-right text-muted-foreground sm:table-cell">
                    {t("tools.qos.samples")}
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {line.days.map((d) => (
                  <TableRow key={d.day} className="hover:bg-transparent">
                    <TableCell className="pl-4 font-mono text-[13px]">{d.day}</TableCell>
                    <TableCell className="text-right tabular-nums text-emerald-600 dark:text-emerald-400">
                      {formatBitsPerSec(d.rxMaxBps)}
                    </TableCell>
                    <TableCell className="text-right tabular-nums text-amber-600 dark:text-amber-400">
                      {formatBitsPerSec(d.txMaxBps)}
                    </TableCell>
                    <TableCell className="hidden text-right tabular-nums text-muted-foreground sm:table-cell">
                      {formatBitsPerSec(d.rxP95Bps)}
                    </TableCell>
                    <TableCell className="hidden pr-4 text-right tabular-nums text-muted-foreground sm:table-cell">
                      {d.samples}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}
      </section>

      {/* ─── Carte 2 : QoS Manager (N°104) ─── */}
      <section className="rounded-lg border p-4">
        <div className="flex flex-wrap items-start justify-between gap-2">
          <div>
            <h3 className="flex items-center gap-1.5 text-sm font-semibold">
              <Gauge className="size-4 text-primary" aria-hidden />
              {t("tools.qos.qosTitle")}
            </h3>
            <p className="mt-0.5 max-w-xl text-xs text-muted-foreground">{t("tools.qos.qosDesc")}</p>
          </div>
          {qos && (
            <div className="flex flex-wrap items-center gap-2">
              {qos.status.enabled ? (
                <StatusBadge status="online" dot />
              ) : (
                <StatusBadge status="offline" dot />
              )}
              <span className="text-xs text-muted-foreground">
                {qos.status.enabled ? t("tools.qos.enabled") : t("tools.qos.disabled")}
                {qos.status.enabled && (qos.status.applied ? ` · ${t("tools.qos.applied")}` : ` · ${t("tools.qos.pending")}`)}
                {qos.status.removalPending ? ` · ${t("tools.qos.removalPending")}` : ""}
              </span>
            </div>
          )}
        </div>

        {/* Recommandation */}
        <div className="mt-3 rounded-lg border bg-accent/30 p-3">
          <p className="text-xs font-semibold text-foreground">{t("tools.qos.recommendation")}</p>
          {rec && rec.source !== "none" ? (
            <div className="mt-1.5 flex flex-wrap items-center gap-x-4 gap-y-1.5">
              <span className="flex items-center gap-1.5 text-sm font-semibold tabular-nums">
                <ArrowDown className="size-3.5 text-emerald-600 dark:text-emerald-400" aria-hidden />
                {formatBitsPerSec(rec.maxDownBps)}
                <ArrowUp className="ml-1 size-3.5 text-amber-600 dark:text-amber-400" aria-hidden />
                {formatBitsPerSec(rec.maxUpBps)}
              </span>
              <span className="text-xs text-muted-foreground">
                {rec.source === "declared" ? t("tools.qos.recSourceDeclared") : t("tools.qos.recSourceMeasured")}
                {" · "}
                {tf("tools.qos.burstInfo", {
                  burst: formatBitsPerSec(rec.burstDownBps),
                  thr: formatBitsPerSec(rec.thrDownBps),
                })}
              </span>
              {!qos?.status.enabled && (
                <Button
                  size="sm"
                  className="h-8"
                  disabled={qosMutation.isPending}
                  onClick={applyRecommended}
                >
                  {qosMutation.isPending ? <Loader2 className="size-3.5 animate-spin" /> : <ShieldCheck className="size-3.5" />}
                  {t("tools.qos.applyRecommended")}
                </Button>
              )}
            </div>
          ) : (
            <p className="mt-1 text-xs text-muted-foreground">{t("tools.qos.recNone")}</p>
          )}
        </div>

        {/* Configuration (désactivée : résumé ; active : valeurs appliquées) */}
        <div className="mt-3 grid gap-3 sm:grid-cols-3">
          <div className="space-y-1.5">
            <Label htmlFor="qos-target" className="text-xs">
              {t("tools.qos.target")}
            </Label>
            <Input
              id="qos-target"
              value={qos?.status.enabled ? qos.status.target : target}
              onChange={(e) => setTarget(e.target.value)}
              disabled={qos?.status.enabled || qosMutation.isPending}
              placeholder={t("tools.qos.targetPlaceholder")}
              className="h-9 font-mono text-xs"
            />
            <p className="text-[11px] text-muted-foreground">{t("tools.qos.targetHint")}</p>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="qos-maxdown" className="text-xs">
              {t("tools.qos.maxDown")} (Mbps)
            </Label>
            <Input
              id="qos-maxdown"
              inputMode="numeric"
              value={qos?.status.enabled ? String(Math.round(qos.status.maxDownBps / 1_000_000)) : maxDown}
              onChange={(e) => setMaxDown(e.target.value)}
              disabled={qos?.status.enabled || qosMutation.isPending}
              placeholder={rec && rec.source !== "none" ? String(Math.round(rec.maxDownBps / 1_000_000)) : "95"}
              className="h-9 tabular-nums"
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="qos-maxup" className="text-xs">
              {t("tools.qos.maxUp")} (Mbps)
            </Label>
            <Input
              id="qos-maxup"
              inputMode="numeric"
              value={qos?.status.enabled ? String(Math.round(qos.status.maxUpBps / 1_000_000)) : maxUp}
              onChange={(e) => setMaxUp(e.target.value)}
              disabled={qos?.status.enabled || qosMutation.isPending}
              placeholder={rec && rec.source !== "none" ? String(Math.round(rec.maxUpBps / 1_000_000)) : "19"}
              className="h-9 tabular-nums"
            />
          </div>
        </div>
        <p className="mt-1.5 text-[11px] text-muted-foreground">{t("tools.qos.limitsHint")}</p>

        <div className="mt-3 flex flex-wrap items-center gap-2">
          {!qos?.status.enabled ? (
            <Button
              size="sm"
              disabled={qosMutation.isPending || !target.trim() || mbpsToBps(maxDown) === 0 || mbpsToBps(maxUp) === 0}
              onClick={applyCustom}
            >
              {qosMutation.isPending ? <Loader2 className="size-4 animate-spin" /> : <Gauge className="size-4" />}
              {t("tools.qos.apply")}
            </Button>
          ) : (
            <>
              {qos.status.appliedAt && (
                <span className="text-xs text-muted-foreground">
                  {tf("tools.qos.appliedAt", { ago: timeAgo(qos.status.appliedAt, lang) })}
                </span>
              )}
              <AlertDialog>
                <AlertDialogTrigger asChild>
                  <Button size="sm" variant="outline" disabled={disableMutation.isPending}>
                    {disableMutation.isPending ? <Loader2 className="size-4 animate-spin" /> : <X className="size-4" />}
                    {t("tools.qos.disable")}
                  </Button>
                </AlertDialogTrigger>
                <AlertDialogContent>
                  <AlertDialogHeader>
                    <AlertDialogTitle>{t("tools.qos.disableTitle")}</AlertDialogTitle>
                    <AlertDialogDescription>{t("tools.qos.disableDesc")}</AlertDialogDescription>
                  </AlertDialogHeader>
                  <AlertDialogFooter>
                    <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
                    <AlertDialogAction onClick={() => disableMutation.mutate()}>
                      {t("tools.qos.disableConfirm")}
                    </AlertDialogAction>
                  </AlertDialogFooter>
                </AlertDialogContent>
              </AlertDialog>
            </>
          )}
        </div>

        {/* Files du routeur (vérité RouterOS) */}
        <div className="mt-4">
          <p className="text-xs font-semibold text-foreground">{t("tools.qos.queues")}</p>
          {qos?.queues.queued ? (
            <p className="mt-1.5 text-xs text-muted-foreground">{t("tools.qos.queuesQueued")}</p>
          ) : qos?.queues.data.length ? (
            <div className="mt-1.5 max-h-48 overflow-y-auto rounded-lg border">
              <Table>
                <TableHeader>
                  <TableRow className="hover:bg-transparent">
                    <TableHead className="pl-4 text-muted-foreground">{t("tools.qos.queueName")}</TableHead>
                    <TableHead className="hidden text-muted-foreground sm:table-cell">
                      {t("tools.qos.queueTarget")}
                    </TableHead>
                    <TableHead className="text-right text-muted-foreground">{t("tools.qos.queueLimit")}</TableHead>
                    <TableHead className="text-right text-muted-foreground">{t("tools.qos.queueRate")}</TableHead>
                    <TableHead className="hidden pr-4 text-right text-muted-foreground sm:table-cell">
                      {t("tools.qos.queueBytes")}
                    </TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {qos.queues.data.map((q) => (
                    <TableRow key={q.name} className={q.name === qos.queueName ? "bg-accent/40" : undefined}>
                      <TableCell className="pl-4 font-mono text-[13px] font-medium">
                        {q.name}
                        <span className="ml-1.5 align-middle text-[10px] text-muted-foreground">
                          {q.name === qos.queueName ? t("tools.qos.aggregate") : q.dynamic ? t("tools.qos.dynamic") : ""}
                        </span>
                      </TableCell>
                      <TableCell className="hidden font-mono text-xs text-muted-foreground sm:table-cell">{q.target}</TableCell>
                      <TableCell className="text-right tabular-nums">
                        {formatBitsPerSec(q.maxUpBps)} / {formatBitsPerSec(q.maxDownBps)}
                      </TableCell>
                      <TableCell className="text-right tabular-nums">
                        <span className="text-amber-600 dark:text-amber-400">{formatBitsPerSec(q.rateUpBps)}</span>
                        {" · "}
                        <span className="text-emerald-600 dark:text-emerald-400">{formatBitsPerSec(q.rateDownBps)}</span>
                      </TableCell>
                      <TableCell className="hidden pr-4 text-right tabular-nums text-muted-foreground sm:table-cell">
                        {formatBytes(q.bytesUp + q.bytesDown, lang)}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          ) : (
            <div className="mt-1.5">
              <EmptyState
                icon={Gauge}
                title={t("tools.qos.disabled")}
                description={t("tools.qos.qosDesc")}
              />
            </div>
          )}
        </div>
      </section>
    </div>
  );
}
