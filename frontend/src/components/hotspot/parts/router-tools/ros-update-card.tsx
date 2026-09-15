"use client";

// N°115 — Mise à jour RouterOS depuis MikCloud : vérification (le routeur
// interroge LUI-MÊME les serveurs MikroTik — la vérité du canal du routeur,
// stable par défaut) puis installation (téléchargement + installation +
// REDÉMARRAGE). Parc concerné : agents RouterOS v7.
//
// Zéro état dérivé pour suivre l'installation : la fiche routeur est « vivante »
// (poll 15 s partagé avec la liste — RouterToolsPanel), le panneau bascule de
// « installation en cours » à « mis à jour » dès que router.version CHANGE —
// la version finale revient d'elle-même au read_state post-redémarrage.

import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  ArrowDownToLine,
  BadgeCheck,
  CircleAlert,
  Download,
  Loader2,
  PartyPopper,
  RefreshCw
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
import { api } from "@/lib/hotspot/api";
import { t as translate, useI18n } from "@/lib/hotspot/i18n";
import type { Lang } from "@/lib/hotspot/i18n";
import type { CommandStatus, RouterDevice, RouterOSCheckResult } from "@/lib/hotspot/types";
import { sleep } from "./shared";

// ─── N°115 — Mise à jour RouterOS ───

/** Réponse du POST /routeros-check : directe (simulated) ou en file (agent). */
interface CheckResponse {
  queued: boolean;
  ok?: boolean;
  state?: RouterOSCheckResult["state"];
  status?: string;
  latestVersion?: string;
  installedVersion?: string;
  channel?: string;
  commandId?: string;
  message?: string;
}

type CheckOutcome =
  | { kind: "result"; result: RouterOSCheckResult }
  | { kind: "error"; message: string }
  | { kind: "timeout" };

/** Normalise une réponse directe ou un résultat de commande en RouterOSCheckResult. */
function toCheckResult(raw: unknown): RouterOSCheckResult | null {
  if (!raw || typeof raw !== "object") return null;
  const r = raw as Record<string, unknown>;
  const str = (v: unknown): string => (typeof v === "string" ? v : "");
  const state = str(r.state);
  if (state !== "latest" && state !== "available" && state !== "error" && state !== "unknown") {
    return null;
  }
  return {
    state,
    status: str(r.status),
    latestVersion: str(r.latestVersion) || undefined,
    installedVersion: str(r.installedVersion) || undefined,
    channel: str(r.channel) || undefined
  };
}

/** Message d'erreur lisible depuis le résultat d'une commande échouée. */
function checkErrorFrom(raw: unknown, fallback: string): string {
  if (raw && typeof raw === "object") {
    const r = raw as Record<string, unknown>;
    for (const key of ["message", "error"]) {
      const v = r[key];
      if (typeof v === "string" && v.length > 0) return v;
    }
  }
  if (typeof raw === "string" && raw.length > 0) return raw;
  return fallback;
}

/**
 * POST /routeros-check puis, en mode agent, poll GET /api/commands/{id} toutes
 * les 2 s (max 90 s — le check-in vient toutes les 45 s et le script routeur
 * patiente lui-même jusqu'à ~30 s que les serveurs MikroTik répondent).
 */
async function runRouterOSCheck(routerId: string, lang: Lang): Promise<CheckOutcome> {
  try {
    const res = await api<CheckResponse>(`/api/routers/${routerId}/routeros-check`, { method: "POST" });
    if (!res.queued) {
      const result = toCheckResult(res);
      return result
        ? { kind: "result", result }
        : { kind: "error", message: translate(lang, "tools.ros.checkUnreadable") };
    }
    const commandId = res.commandId ?? "";
    if (!commandId) return { kind: "error", message: translate(lang, "tools.ros.checkUnreadable") };
    const deadline = Date.now() + 90_000;
    while (Date.now() <= deadline) {
      await sleep(2_000);
      const cmd = await api<CommandStatus>(`/api/commands/${commandId}`);
      if (cmd.status === "done") {
        const result = toCheckResult(cmd.result);
        return result
          ? { kind: "result", result }
          : { kind: "error", message: translate(lang, "tools.ros.checkUnreadable") };
      }
      if (cmd.status === "error") {
        return { kind: "error", message: checkErrorFrom(cmd.result, translate(lang, "tools.ros.checkFailed")) };
      }
    }
    return { kind: "timeout" };
  } catch (err) {
    return {
      kind: "error",
      message: err instanceof Error ? err.message : translate(lang, "tools.ros.checkFailed")
    };
  }
}

/** Panneau du résultat de vérification : à jour / disponible / erreur / inconnu. */
function CheckResultPanel({
  result,
  onUpdate
}: {
  result: RouterOSCheckResult;
  onUpdate: (target: string) => void;
}) {
  const { t, tf } = useI18n();

  if (result.state === "latest") {
    return (
      <div
        className="mt-3 rounded-lg border border-emerald-500/25 bg-emerald-500/10 p-3 text-sm"
        aria-live="polite"
      >
        <p className="flex items-center gap-2 font-medium text-emerald-700 dark:text-emerald-400">
          <BadgeCheck className="size-4 shrink-0" aria-hidden />
          {t("tools.ros.stateLatest")}
        </p>
        <p className="mt-1 text-xs text-muted-foreground">{t("tools.ros.uptodateNote")}</p>
      </div>
    );
  }

  if (result.state === "available") {
    const target = result.latestVersion ?? "";
    return (
      <div className="mt-3 rounded-lg border border-amber-500/25 bg-amber-500/10 p-3 text-sm" aria-live="polite">
        <p className="flex flex-wrap items-center gap-2 font-medium text-amber-700 dark:text-amber-400">
          <ArrowDownToLine className="size-4 shrink-0" aria-hidden />
          {t("tools.ros.stateAvailable")}
        </p>
        <p className="mt-1.5 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground">
          <span className="font-mono">{result.installedVersion ?? "—"}</span>
          <span aria-hidden>→</span>
          <span className="font-mono font-semibold text-foreground">{target || "?"}</span>
          {result.channel && (
            <Badge variant="outline" className="ml-1 h-5 px-1.5 text-[10px] font-normal">
              {tf("tools.ros.channel", { channel: result.channel })}
            </Badge>
          )}
        </p>
        {result.status && <p className="mt-1 text-xs italic text-muted-foreground">{result.status}</p>}
        {target && (
          <Button size="sm" className="mt-2.5" onClick={() => onUpdate(target)}>
            <Download className="size-4" />
            {tf("tools.ros.updateBtn", { version: target })}
          </Button>
        )}
      </div>
    );
  }

  if (result.state === "error") {
    return (
      <div
        className="mt-3 rounded-lg border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive"
        role="alert"
      >
        <p className="flex items-center gap-2 font-medium">
          <CircleAlert className="size-4 shrink-0" aria-hidden />
          {t("tools.ros.stateError")}
        </p>
        {result.status && <p className="mt-1 text-xs">{result.status}</p>}
      </div>
    );
  }

  // unknown — le status brut, affiché honnêtement.
  return (
    <div className="mt-3 rounded-lg border p-3 text-sm" aria-live="polite">
      <p className="flex items-center gap-2 font-medium">
        <CircleAlert className="size-4 shrink-0 text-muted-foreground" aria-hidden />
        {t("tools.ros.stateUnknown")}
      </p>
      {result.status && <p className="mt-1 text-xs italic text-muted-foreground">{result.status}</p>}
    </div>
  );
}

/** Panneau d'installation : en cours (spinner) ou terminée (from → cible). */
function InstallPanel({ from, to, current }: { from: string; to: string; current: string }) {
  const { t, tf } = useI18n();
  // Terminé : la cible est atteinte, OU la version a changé depuis une base
  // connue (mise à jour manuelle Winbox en parallèle, build voisin). Une base
  // INCONNUE (from vide — aucun read_state encore arrivé) ne conclut JAMAIS
  // sur une version ≠ cible : l'arrivée d'une version périmée pendant le
  // téléchargement ne doit pas faire passer le panneau pour terminé.
  const done = current !== "" && (current === to || (from !== "" && current !== from));
  if (!done) {
    // Toujours l'ancienne version : le routeur télécharge / installe / va redémarrer.
    return (
      <div
        className="mt-3 rounded-lg border border-amber-500/30 bg-amber-500/10 p-3 text-sm"
        role="status"
        aria-live="polite"
      >
        <p className="flex items-center gap-2 font-medium text-amber-700 dark:text-amber-400">
          <Loader2 className="size-4 shrink-0 animate-spin" aria-hidden />
          {t("tools.ros.installingTitle")}
        </p>
        <p className="mt-1 text-xs text-muted-foreground">{t("tools.ros.installingNote")}</p>
      </div>
    );
  }
  // La version a changé : la mise à jour est passée (la vérité du read_state,
  // même si la version finale diffère de la cible).
  return (
    <div
      className="mt-3 rounded-lg border border-emerald-500/25 bg-emerald-500/10 p-3 text-sm"
      aria-live="polite"
    >
      <p className="flex items-center gap-2 font-medium text-emerald-700 dark:text-emerald-400">
        <PartyPopper className="size-4 shrink-0" aria-hidden />
        {t("tools.ros.installedTitle")}
      </p>
      <p className="mt-1 text-xs text-muted-foreground">
        <span className="font-mono">{from || "—"}</span> <span aria-hidden>→</span>{" "}
        <span className="font-mono font-semibold text-foreground">{current}</span>
        {" · "}
        {tf("tools.ros.installedNote", { version: current })}
      </p>
    </div>
  );
}

export function RouterOSUpdateCard({ router }: { router: RouterDevice }) {
  const { t, tf, lang } = useI18n();
  const queryClient = useQueryClient();
  const [checkResult, setCheckResult] = useState<RouterOSCheckResult | null>(null);
  const [confirmTarget, setConfirmTarget] = useState<string | null>(null);
  // Lancement d'installation : version au moment du clic + CIBLE (le panneau
  // vit tant que la version du routeur « vivant » n'a pas atteint la cible).
  const [installState, setInstallState] = useState<{ from: string; to: string } | null>(null);
  const realMode = router.mode === "real";

  const checkMutation = useMutation({
    mutationFn: () => runRouterOSCheck(router.id, lang),
    onSuccess: (outcome) => {
      if (outcome.kind === "result") {
        // Une vérification fraîche remplace le panneau d'installation : le
        // check est l'action la plus récente du gérant, et c'est la vérité
        // du serveur (déjà à jour / encore en retard) qui doit s'afficher.
        setInstallState(null);
        setCheckResult(outcome.result);
      } else if (outcome.kind === "error") {
        setCheckResult(null);
        toast.error(outcome.message);
      } else {
        setCheckResult(null);
        toast.error(t("tools.ros.checkTimeout"));
      }
    }
  });

  const updateMutation = useMutation({
    mutationFn: (target: string) =>
      api<{ ok?: boolean; queued?: boolean; version?: string; already?: boolean; message?: string }>(
        `/api/routers/${router.id}/routeros-update`,
        { method: "POST", body: { latest: target } }
      ),
    onSuccess: (res, target) => {
      setConfirmTarget(null);
      setCheckResult(null);
      // La cible : la version réponse (simulated — application immédiate)
      // ou la version du check (agent — la vérité du canal MikroTik).
      setInstallState({ from: router.version ?? "", to: res.version ?? target });
      if (res.queued) {
        // Agent : la commande part au prochain check-in ; le panneau
        // « installation en cours » prend le relais jusqu'au redémarrage.
        toast.success(res.message ?? t("tools.ros.updateQueuedToast"));
      } else {
        // Simulated : application immédiate (le routeur « redémarre »).
        toast.success(tf("tools.ros.updatedToast", { version: res.version ?? target }));
      }
      for (const key of ["/api/routers", "/api/sessions", "/api/dashboard"]) {
        void queryClient.invalidateQueries({ queryKey: [key] });
      }
    },
    onError: (err: Error) => toast.error(err.message)
  });

  const busy = checkMutation.isPending || updateMutation.isPending;

  return (
    <Card className="gap-0 py-0">
      <CardContent className="p-4 sm:p-5">
        <h3 className="text-sm font-semibold">{t("tools.ros.title")}</h3>
        <p className="mt-0.5 text-xs text-muted-foreground">
          {realMode ? t("tools.realNote") : t("tools.ros.desc")}
        </p>

        <div className="mt-3 flex flex-wrap items-center gap-3">
          <span className="flex items-center gap-2 text-sm">
            <span className="text-muted-foreground">{t("tools.ros.current")}</span>
            <span className="font-mono font-semibold tabular-nums">{router.version || "—"}</span>
          </span>
          <Button
            variant="outline"
            size="sm"
            onClick={() => checkMutation.mutate()}
            disabled={realMode || busy}
          >
            {checkMutation.isPending ? (
              <Loader2 className="size-4 animate-spin" />
            ) : (
              <RefreshCw className="size-4" />
            )}
            {t("tools.ros.checkBtn")}
          </Button>
        </div>

        {realMode && <p className="mt-2 text-xs text-muted-foreground">{t("tools.realNote")}</p>}
        {checkMutation.isPending && router.mode === "agent" && (
          <p className="mt-3 flex items-center gap-2 text-sm text-muted-foreground" role="status">
            <Loader2 className="size-4 animate-spin text-amber-600" />
            {t("tools.ros.checkQueued")}
          </p>
        )}

        {checkResult && !installState && (
          <CheckResultPanel
            result={checkResult}
            onUpdate={(target) => setConfirmTarget(target)}
          />
        )}
        {installState && (
          <InstallPanel
            from={installState.from}
            to={installState.to}
            current={router.version ?? ""}
          />
        )}
      </CardContent>

      <AlertDialog open={confirmTarget !== null} onOpenChange={(open) => !open && setConfirmTarget(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{tf("tools.ros.updateTitle", { version: confirmTarget ?? "" })}</AlertDialogTitle>
            <AlertDialogDescription asChild>
              <span>
                {tf("tools.ros.updateDesc1", { name: router.name, version: confirmTarget ?? "" })}{" "}
                <strong>{t("tools.ros.updateDescStrong")}</strong>{" "}
                {t("tools.ros.updateDesc2")}
                {router.mode === "agent" && t("tools.ros.agentNote")}
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
              {t("tools.ros.updateConfirm")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </Card>
  );
}
