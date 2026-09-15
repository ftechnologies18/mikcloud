"use client";

// N°117 — Parc routeurs GLOBAL (console plateforme, super-admin uniquement).
// Miroir multi-comptes de la carte RouterOS N°115 : chaque routeur de chaque
// compte client avec sa version installée, la version disponible détectée et
// son état — puis deux gestes de flotte :
//   • « Vérifier tout le parc » — lecture seule : un routeros_check par
//     routeur agent, chaque routeur interroge LUI-MÊME les serveurs MikroTik
//     (la vérité du canal du routeur) et répond à son prochain check-in ;
//   • « Mettre à jour le parc » — n'installe QUE le retard connu (état
//     available du dernier check, jamais à l'aveugle) : un update RouterOS
//     redémarre le routeur et coupe le hotspot du client, la confirmation
//     affiche le compte exact avant le geste.
// Zéro état dérivé côté front : tout vient de GET /api/admin/fleet/routers
// (état dérivé serveur des commandes N°115 + télémétrie) — le poll
// s'accélère (10 s) tant qu'une vérification ou installation est en vol.

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ArrowDownToLine,
  BadgeCheck,
  CircleAlert,
  Clock,
  Download,
  HelpCircle,
  Loader2,
  RefreshCw,
  Router as RouterIcon,
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
import { Card, CardContent } from "@/components/ui/card";
import { EmptyState } from "@/components/hotspot/empty-state";
import { LoadingCards } from "@/components/hotspot/loading";
import { PageHeader } from "@/components/hotspot/page-header";
import { StatCard } from "@/components/hotspot/stat-card";
import { StatusBadge } from "@/components/hotspot/status-badge";
import { fetchFleetRouters, fleetRouterOSCheck, fleetRouterOSUpdate } from "@/lib/hotspot/api";
import { localeOf, useI18n } from "@/lib/hotspot/i18n";
import { timeAgo } from "@/lib/hotspot/format";
import type { FleetRouter } from "@/lib/hotspot/types";

/** Cible de la confirmation d'installation : tout le parc à jour disponible,
 * ou un routeur précis (bouton de la ligne). */
type UpdateTarget = { mode: "all" } | { mode: "one"; router: FleetRouter };

/** Badge d'état de mise à jour RouterOS (dérivé serveur, cf. N°115/N°117). */
function RosStateBadge({ router }: { router: FleetRouter }) {
  const { t } = useI18n();

  if (router.updating) {
    return (
      <Badge variant="outline" className="gap-1.5 border-amber-500/30 bg-amber-500/10 px-1.5 py-0 text-[10px] font-medium text-amber-600 dark:text-amber-400">
        <Loader2 className="size-3 animate-spin" aria-hidden />
        {t("platform.fleet.ros.updating")}
      </Badge>
    );
  }
  if (router.checking) {
    return (
      <Badge variant="outline" className="gap-1.5 border-amber-500/30 bg-amber-500/10 px-1.5 py-0 text-[10px] font-medium text-amber-600 dark:text-amber-400">
        <Loader2 className="size-3 animate-spin" aria-hidden />
        {t("platform.fleet.ros.checking")}
      </Badge>
    );
  }
  switch (router.rosState) {
    case "latest":
      return (
        <Badge variant="outline" className="gap-1.5 border-emerald-500/30 bg-emerald-500/10 px-1.5 py-0 text-[10px] font-medium text-emerald-600 dark:text-emerald-400">
          <BadgeCheck className="size-3" aria-hidden />
          {t("platform.fleet.ros.latest")}
        </Badge>
      );
    case "available":
      return (
        <Badge variant="outline" className="gap-1.5 border-amber-500/30 bg-amber-500/10 px-1.5 py-0 text-[10px] font-medium text-amber-600 dark:text-amber-400">
          <ArrowDownToLine className="size-3" aria-hidden />
          {t("platform.fleet.ros.available")}
        </Badge>
      );
    case "error":
      return (
        <Badge variant="outline" className="gap-1.5 border-destructive/30 bg-destructive/10 px-1.5 py-0 text-[10px] font-medium text-destructive">
          <CircleAlert className="size-3" aria-hidden />
          {t("platform.fleet.ros.error")}
        </Badge>
      );
    case "unknown":
      return (
        <Badge variant="outline" className="gap-1.5 px-1.5 py-0 text-[10px] font-medium text-muted-foreground">
          <HelpCircle className="size-3" aria-hidden />
          {t("platform.fleet.ros.unknown")}
        </Badge>
      );
    default:
      return (
        <Badge variant="outline" className="px-1.5 py-0 text-[10px] font-medium text-muted-foreground">
          {t("platform.fleet.ros.never")}
        </Badge>
      );
  }
}

export default function PlatformFleetView() {
  const { t, tf, lang } = useI18n();
  const queryClient = useQueryClient();
  const [confirmTarget, setConfirmTarget] = useState<UpdateTarget | null>(null);

  const { data, isLoading, isError, refetch } = useQuery({
    queryKey: ["/api/admin/fleet/routers"],
    queryFn: fetchFleetRouters,
    // Poll accéléré pendant les vols (check/installation), calme sinon — la
    // forme fonctionnelle lit l'état du query SANS fermer sur `data`
    // (déclaré par cette même expression).
    refetchInterval: (query) => {
      const s = query.state.data?.summary;
      return s && (s.checking > 0 || s.updating > 0) ? 10_000 : 30_000;
    },
    retry: (failureCount, err) => !(err instanceof Error && err.message.includes("403")) && failureCount < 1,
  });

  const nf = (value: number): string => new Intl.NumberFormat(localeOf(lang)).format(value);

  /** Routeurs avec une mise à jour DÉTECTÉE — la cible du geste de flotte. */
  const updatable = useMemo(
    () => (data ? data.routers.filter((r) => r.rosState === "available" && !r.updating) : []),
    [data],
  );
  /** Comptes clients distincts touchés par le geste de flotte. */
  const updatableAccounts = useMemo(
    () => new Set(updatable.map((r) => r.accountId)).size,
    [updatable],
  );

  const invalidate = () => {
    for (const key of ["/api/admin/fleet/routers", "/api/admin/overview"]) {
      void queryClient.invalidateQueries({ queryKey: [key] });
    }
  };

  const checkMutation = useMutation({
    mutationFn: (routerIds?: string[]) => fleetRouterOSCheck(routerIds),
    onSuccess: (res) => {
      toast.success(res.message);
      invalidate();
    },
    onError: (err: Error) => toast.error(err.message),
  });

  const updateMutation = useMutation({
    mutationFn: (target: UpdateTarget) =>
      target.mode === "one"
        ? fleetRouterOSUpdate([target.router.id], target.router.rosLatest)
        : fleetRouterOSUpdate(),
    onSuccess: (res) => {
      setConfirmTarget(null);
      const total = (res.queued ?? 0) + (res.applied ?? 0);
      if (total > 0) {
        toast.success(res.message);
      } else {
        toast.info(res.message);
      }
      invalidate();
    },
    onError: (err: Error) => toast.error(err.message),
  });

  if (isLoading) {
    return (
      <div className="space-y-6">
        <PageHeader title={t("platform.fleet.title")} description={t("platform.fleet.description")} />
        <LoadingCards cards={4} />
      </div>
    );
  }

  if (isError || !data) {
    return (
      <Card>
        <EmptyState
          icon={RouterIcon}
          title={t("platform.fleet.loadError")}
          description={t("platform.fleet.loadErrorDesc")}
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

  const { summary } = data;
  const confirmCount = confirmTarget?.mode === "one" ? 1 : updatable.length;
  const busy = checkMutation.isPending || updateMutation.isPending;

  return (
    <div className="space-y-6">
      <PageHeader title={t("platform.fleet.title")} description={t("platform.fleet.description")} />

      {/* Synthèse du parc */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4" aria-live="polite">
        <StatCard
          title={t("platform.fleet.kpi.total")}
          value={nf(summary.total)}
          sub={t("platform.fleet.kpi.totalSub")}
          icon={RouterIcon}
        />
        <StatCard
          title={t("platform.fleet.kpi.online")}
          value={`${nf(summary.online)}/${nf(summary.total)}`}
          sub={`${nf(summary.agent)} ${t("platform.fleet.kpi.agents")}`}
          icon={RouterIcon}
          live
        />
        <StatCard
          title={t("platform.fleet.kpi.available")}
          value={nf(summary.available)}
          sub={t("platform.fleet.kpi.availableSub")}
          icon={ArrowDownToLine}
          valueClassName={summary.available > 0 ? "text-amber-600 dark:text-amber-400" : undefined}
        />
        <StatCard
          title={t("platform.fleet.kpi.updating")}
          value={nf(summary.updating)}
          sub={
            summary.checking > 0
              ? `${nf(summary.checking)} ${t("platform.fleet.ros.checking")}`
              : t("platform.fleet.ros.updatingNote")
          }
          icon={Download}
        />
      </div>

      {/* Gestes de flotte */}
      <Card className="gap-0 py-0">
        <CardContent className="flex flex-col gap-3 p-4 sm:p-5 lg:flex-row lg:items-center lg:justify-between">
          <div className="min-w-0 space-y-1">
            <div className="flex flex-wrap items-center gap-2">
              <Button
                variant="outline"
                size="sm"
                onClick={() => checkMutation.mutate(undefined)}
                disabled={busy || summary.agent === 0}
              >
                {checkMutation.isPending ? (
                  <Loader2 className="size-4 animate-spin" />
                ) : (
                  <RefreshCw className="size-4" />
                )}
                {t("platform.fleet.checkAll")}
              </Button>
              {updatable.length > 0 ? (
                <Button size="sm" onClick={() => setConfirmTarget({ mode: "all" })} disabled={busy}>
                  <Download className="size-4" />
                  {tf("platform.fleet.updateCount", { n: nf(updatable.length) })}
                </Button>
              ) : (
                <Button size="sm" disabled title={t("platform.fleet.updateNoneNote")}>
                  <Download className="size-4" />
                  {t("platform.fleet.updateNone")}
                </Button>
              )}
            </div>
            <p className="text-xs text-muted-foreground">
              {updatable.length > 0 ? t("platform.fleet.checkAllNote") : t("platform.fleet.updateNoneNote")}
            </p>
          </div>
          <p className="shrink-0 text-xs text-muted-foreground" role="status">
            {nf(summary.agent)} {t("platform.fleet.mode.agent")} · {nf(summary.simulated)}{" "}
            {t("platform.fleet.mode.simulated")} · {nf(summary.latest)} {t("platform.fleet.ros.latest")}
          </p>
        </CardContent>
      </Card>

      {/* Le parc, tous comptes confondus */}
      <Card className="gap-0 py-0">
        {data.routers.length === 0 ? (
          <div className="px-4 py-6 sm:px-6">
            <EmptyState
              icon={RouterIcon}
              title={t("platform.fleet.empty")}
              description={t("platform.fleet.emptyDesc")}
            />
          </div>
        ) : (
          <ul className="max-h-[32rem] divide-y overflow-y-auto">
            {data.routers.map((r) => (
              <li
                key={r.id}
                className="flex flex-col gap-2.5 px-4 py-3 transition-colors hover:bg-muted/40 sm:px-6 lg:flex-row lg:items-center lg:gap-3"
              >
                {/* Compte + routeur */}
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-medium">{r.name}</p>
                  <p className="truncate text-xs text-muted-foreground">{r.accountName}</p>
                </div>

                {/* Mode + ligne */}
                <div className="flex flex-wrap items-center gap-1.5 lg:w-44 lg:shrink-0">
                  <StatusBadge status={r.mode === "agent" ? "agent" : r.mode === "real" ? "real" : "simulated"} dot />
                  <StatusBadge status={r.status} dot />
                </div>

                {/* Versions : installée → disponible */}
                <div className="flex min-w-0 items-center gap-1.5 text-xs lg:w-44 lg:shrink-0 lg:justify-center">
                  <span className="truncate font-mono tabular-nums" title={r.version}>
                    {r.version || "—"}
                  </span>
                  {r.rosState === "available" && r.rosLatest && (
                    <>
                      <span aria-hidden>→</span>
                      <span className="truncate font-mono font-semibold tabular-nums text-amber-600 dark:text-amber-400">
                        {r.rosLatest}
                      </span>
                    </>
                  )}
                </div>

                {/* État de mise à jour + dernière vérification */}
                <div className="flex min-w-0 flex-col gap-0.5 lg:w-48 lg:shrink-0">
                  <RosStateBadge router={r} />
                  <p className="truncate text-[10px] text-muted-foreground" title={r.updateError || r.rosStatus}>
                    {r.updateError && !r.updating
                      ? r.updateError
                      : r.checkedAt
                        ? tf("platform.fleet.lastCheck", { at: timeAgo(r.checkedAt, lang) })
                        : r.rosStatus || t("platform.fleet.checkedNever")}
                  </p>
                </div>

                {/* Actions par routeur */}
                <div className="flex shrink-0 items-center gap-1.5 lg:w-40 lg:justify-end">
                  {r.mode === "agent" && (
                    <Button
                      variant="ghost"
                      size="sm"
                      className="h-8 px-2"
                      onClick={() => checkMutation.mutate([r.id])}
                      disabled={busy || r.checking || r.updating}
                      title={r.status === "offline" ? t("platform.fleet.offline") : t("platform.fleet.checkOne")}
                    >
                      {r.checking ? (
                        <Loader2 className="size-4 animate-spin" />
                      ) : (
                        <RefreshCw className="size-4" />
                      )}
                      <span className="sr-only">{t("platform.fleet.checkOne")}</span>
                    </Button>
                  )}
                  {r.mode !== "real" && r.rosState === "available" && !r.updating && (
                    <Button
                      variant="outline"
                      size="sm"
                      className="h-8 border-amber-500/30 text-amber-600 hover:bg-amber-500/10 dark:text-amber-400"
                      onClick={() => setConfirmTarget({ mode: "one", router: r })}
                      disabled={busy}
                    >
                      <Download className="size-4" />
                      {t("platform.fleet.updateOne")}
                    </Button>
                  )}
                  {r.mode === "real" && (
                    <span className="text-[10px] text-muted-foreground">{t("platform.fleet.realNote")}</span>
                  )}
                </div>
              </li>
            ))}
          </ul>
        )}
      </Card>

      {/* Confirmation — le geste coupant (redémarrage + portail coupé) ne
          part JAMAIS sans un avertissement au compte exact. */}
      <AlertDialog open={confirmTarget !== null} onOpenChange={(open) => !open && setConfirmTarget(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{tf("platform.fleet.updateTitle", { n: nf(confirmCount) })}</AlertDialogTitle>
            <AlertDialogDescription asChild>
              <span>
                {tf("platform.fleet.updateDesc1", {
                  n: nf(confirmCount),
                  a: nf(confirmTarget?.mode === "one" ? 1 : updatableAccounts),
                })}{" "}
                <strong>{t("platform.fleet.updateDescStrong")}</strong> {t("platform.fleet.updateDesc2")}
              </span>
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={updateMutation.isPending}>{t("common.cancel")}</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-white hover:bg-destructive/90"
              disabled={updateMutation.isPending}
              onClick={(e) => {
                e.preventDefault();
                if (confirmTarget) updateMutation.mutate(confirmTarget);
              }}
            >
              {updateMutation.isPending && <Loader2 className="size-4 animate-spin" />}
              {t("platform.fleet.updateConfirm")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
