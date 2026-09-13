"use client";

// Onglet F8+F10 — Système : carte d'informations (ressources), ping
// (cible + résultat détaillé), carte d'alimentation (reboot/shutdown) et
// l'assembleur SystemTab (résumé Protection N°83 + 4 cartes).
// Transfert pur depuis router-tools.tsx (deux segments).

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  CircuitBoard,
  Clock,
  Cpu,
  HardDrive,
  Loader2,
  MemoryStick,
  Power,
  RotateCcw,
  Wrench,
  Zap
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
  AlertDialogTitle
} from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { ProtectionSummaryCard } from "@/components/hotspot/parts/protection-cards";
import { PoolDoctorCard } from "@/components/hotspot/parts/router-tools/pool-card";
import { api } from "@/lib/hotspot/api";
import { t as translate, useI18n } from "@/lib/hotspot/i18n";
import type { Lang } from "@/lib/hotspot/i18n";
import { formatDuration, formatMb } from "@/lib/hotspot/format";
import type { CommandStatus, PingResult, RouterDevice, RouterStats } from "@/lib/hotspot/types";
import { fmtMs, sleep } from "./shared";
import { SchedulerCard } from "./scheduler-card";

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

export function SystemTab({ router }: { router: RouterDevice }) {
  return (
    <div className="space-y-4">
      <SystemInfoCard router={router} />
      {/* N°83 — les 3 cartes sécurité ont déménagé dans la vue Protection
          (/app/protection) : l'onglet Système garde un résumé compact avec
          lien — aucun contrôle dupliqué. */}
      <ProtectionSummaryCard router={router} />
      {/* N°97 — docteur pool IP : jauge d'occupation + recyclage zombies +
          extension (l'épuisement « no more free addresses from pool » aux
          heures de pointe). */}
      <PoolDoctorCard router={router} />
      <PingCard router={router} />
      <SchedulerCard router={router} />
      <PowerCard router={router} />
    </div>
  );
}
