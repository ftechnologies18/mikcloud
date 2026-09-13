"use client";

// N°97 — Docteur du pool d'adresses IP du hotspot (onglet Système des
// Outils routeur). Corrige l'épuisement « no more free addresses from
// pool » aux heures de pointe :
//   - jauge d'occupation live (poolHosts/poolCap — hôtes payants + zombies)
//     quand la capacité est connue (auto-diagnostic du check-in) ;
//   - « Recycler les IP zombies » : pose login-timeout/idle-timeout/
//     keepalive-timeout + address-per-mac=1 (aucun subnet touché) ;
//   - « Étendre le pool » (confirmation explicite) : ajoute le range
//     dédié 10.77.0.0/21 (~2 037 IP) au pool du profil + IP secondaire +
//     entrée network masquerade + règle NAT marquée.
// N°99 — switch « Auto-réparation » (mode agent) : le gérant autorise le
// cloud à recycler les zombies LUI-MÊME à chaque transition d'alerte
// (≥ 80 %/≥ 95 %) — un geste initial, plus jamais de clic. L'extension
// reste manuelle (geste topologique à confirmation explicite).
// Mode agent : POST + poll GET /api/commands/{id} (pattern ping F8).
// Simulé : réponse immédiate. Mode API directe : carte muette (§0).

import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Activity, Gauge, Loader2, Sparkles, Wand2 } from "lucide-react";
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
import { Switch } from "@/components/ui/switch";
import { api } from "@/lib/hotspot/api";
import { tf as translateF, t as translate, useI18n } from "@/lib/hotspot/i18n";
import type { Lang } from "@/lib/hotspot/i18n";
import type { CommandStatus, RouterDevice } from "@/lib/hotspot/types";
import { sleep } from "./shared";

/** Réponse du POST pool-doctor : directe (simulé) ou mise en file (agent). */
interface PoolDoctorResult {
  ok?: boolean;
  queued?: boolean;
  commandId?: string;
  poolCap?: number;
  poolHosts?: number;
  usagePct?: number;
  message?: string;
}

/** Statut d'occupation coloré selon les seuils d'alerte (80/95). */
function usageBadgeClass(pct: number): string {
  if (pct >= 95)
    return "border-destructive/25 bg-destructive/10 text-destructive";
  if (pct >= 80)
    return "border-amber-500/25 bg-amber-500/10 text-amber-600 dark:text-amber-400";
  return "border-emerald-500/25 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400";
}

/** POST pool-doctor puis, en mode agent, poll de la commande (max 120 s). */
async function runPoolDoctor(
  routerId: string,
  extend: boolean,
  lang: Lang,
): Promise<PoolDoctorResult | null> {
  const res = await api<PoolDoctorResult>(`/api/routers/${routerId}/pool-doctor`, {
    method: "POST",
    body: extend ? { extend: true } : {},
  });
  if (!res.queued) return res; // simulé : réponse directe
  const commandId = res.commandId ?? "";
  if (!commandId) return res;
  const deadline = Date.now() + 120_000;
  while (Date.now() <= deadline) {
    await sleep(3_000);
    const cmd = await api<CommandStatus>(`/api/commands/${commandId}`);
    if (cmd.status === "done") return null; // ok : les champs suivent via /api/routers
    if (cmd.status === "error") {
      const raw = cmd.result as { error?: string; message?: string } | string | null;
      const message =
        typeof raw === "string" && raw.length > 0
          ? raw
          : raw && typeof raw === "object" && typeof raw.error === "string"
            ? raw.error
            : typeof raw === "object" && raw && typeof raw.message === "string"
              ? raw.message
              : translate(lang, "tools.pool.failedDefault");
      throw new Error(message);
    }
  }
  return null; // timeout silencieux : la commande finira côté routeur
}

export function PoolDoctorCard({ router }: { router: RouterDevice }) {
  const { t, lang } = useI18n();
  const queryClient = useQueryClient();
  const [confirmExtend, setConfirmExtend] = useState(false);
  const realMode = router.mode === "real";

  const doctorMutation = useMutation({
    mutationFn: (extend: boolean) => runPoolDoctor(router.id, extend, lang),
    onSuccess: (res, extend) => {
      if (res && res.ok) {
        // Simulé : la réponse porte les chiffres.
        toast.success(t("tools.pool.doneToast"));
      } else {
        toast.success(
          extend ? t("tools.pool.queuedExtendToast") : t("tools.pool.queuedToast"),
        );
      }
      setConfirmExtend(false);
      for (const key of ["/api/routers", "/api/routers/" + router.id, "/api/dashboard"]) {
        void queryClient.invalidateQueries({ queryKey: [key] });
      }
    },
    onError: (err: Error) => toast.error(err.message),
  });

  // N°99 — bascule de l'auto-réparation (mode agent uniquement : la
  // commande qui en découle attend un agent pour l'exécuter).
  const autoMutation = useMutation({
    mutationFn: (auto: boolean) =>
      api<{ ok: boolean; auto: boolean }>(`/api/routers/${router.id}/pool-auto`, {
        method: "PUT",
        body: { auto },
      }),
    onSuccess: (res) => {
      toast.success(res.auto ? t("tools.pool.autoOnToast") : t("tools.pool.autoOffToast"));
      for (const key of ["/api/routers", "/api/routers/" + router.id]) {
        void queryClient.invalidateQueries({ queryKey: [key] });
      }
    },
    onError: (err: Error) => toast.error(err.message),
  });

  // Occupation : capacités connues (auto-diagnostic posé par le check-in).
  const cap = router.poolCap ?? 0;
  const hosts = router.poolHosts ?? 0;
  const pct = cap > 0 && hosts > 0 ? Math.min(100, Math.round((hosts * 100) / cap)) : 0;
  const zombies = Math.max(0, hosts - router.activeSessions);
  const known = cap > 0;

  return (
    <Card className="gap-0 py-0">
      <CardContent className="p-4 sm:p-5">
        <h3 className="text-sm font-semibold">{t("tools.pool.title")}</h3>
        <p className="mt-0.5 text-xs text-muted-foreground">{t("tools.pool.desc")}</p>

        {known ? (
          <div className="mt-3 space-y-2" aria-live="polite">
            <div className="flex flex-wrap items-center gap-x-4 gap-y-1.5 text-sm">
              <span className="text-muted-foreground">
                {t("tools.pool.usage")}{" "}
                <span className="font-semibold tabular-nums text-foreground">
                  {hosts}/{cap}
                </span>
              </span>
              <Badge variant="outline" className={usageBadgeClass(pct)}>
                {pct} %
              </Badge>
              {zombies > 0 && (
                <span className="text-muted-foreground">
                  {t("tools.pool.zombies")}{" "}
                  <span className="font-semibold tabular-nums text-foreground">{zombies}</span>
                </span>
              )}
            </div>
            {/* Jauge — max 100 %, couleur par seuil d'alerte. */}
            <div
              className="h-2 w-full overflow-hidden rounded-full bg-muted"
              role="img"
              aria-label={translateF(lang, "tools.pool.gaugeAria", { pct })}
            >
              <div
                className={
                  pct >= 95
                    ? "h-full rounded-full bg-destructive transition-all duration-500"
                    : pct >= 80
                      ? "h-full rounded-full bg-amber-500 transition-all duration-500"
                      : "h-full rounded-full bg-emerald-500 transition-all duration-500"
                }
                style={{ width: `${Math.min(100, pct)}%` }}
              />
            </div>
            {router.poolRanges && (
              <p className="font-mono text-xs text-muted-foreground" title={router.poolRanges}>
                {router.poolRanges}
              </p>
            )}
          </div>
        ) : (
          <p className="mt-3 text-xs text-muted-foreground">{t("tools.pool.unknown")}</p>
        )}

        {realMode ? (
          <p className="mt-3 text-xs text-muted-foreground">{t("tools.realNote")}</p>
        ) : (
          <div className="mt-4 flex flex-wrap gap-3">
            <Button
              onClick={() => doctorMutation.mutate(false)}
              disabled={doctorMutation.isPending}
            >
              {doctorMutation.isPending && !confirmExtend ? (
                <Loader2 className="size-4 animate-spin" />
              ) : (
                <Sparkles className="size-4" />
              )}
              {t("tools.pool.recycle")}
            </Button>
            <Button
              variant="outline"
              onClick={() => setConfirmExtend(true)}
              disabled={doctorMutation.isPending}
            >
              <Gauge className="size-4" />
              {t("tools.pool.extend")}
            </Button>
          </div>
        )}

        {/* N°99 — auto-réparation (mode agent : la commande a besoin d'un
            agent pour l'exécuter — un switch sans exécutant serait une
            promesse morte). */}
        {router.mode === "agent" && (
          <div className="mt-4 flex flex-col gap-2 rounded-lg border border-border/70 bg-muted/40 p-3 sm:flex-row sm:items-start sm:gap-4">
            <div className="flex min-w-0 flex-1 items-start gap-3">
              <Wand2 className="mt-0.5 size-4 shrink-0 text-primary" aria-hidden />
              <div className="min-w-0">
                <label
                  htmlFor="pool-auto-switch"
                  className="text-sm font-medium leading-tight"
                >
                  {t("tools.pool.auto")}
                </label>
                <p className="mt-0.5 text-xs text-muted-foreground">
                  {t("tools.pool.autoDesc")}
                </p>
                {router.poolAutoPending && (
                  <p
                    className="mt-1.5 flex items-center gap-1.5 text-xs text-amber-600 dark:text-amber-400"
                    role="status"
                  >
                    <Loader2 className="size-3.5 animate-spin" aria-hidden />
                    {t("tools.pool.autoPending")}
                  </p>
                )}
              </div>
            </div>
            <div className="flex shrink-0 items-center ps-7 sm:ps-0">
              <Switch
                id="pool-auto-switch"
                checked={router.poolAuto === true}
                onCheckedChange={(checked) => autoMutation.mutate(checked)}
                disabled={autoMutation.isPending}
                aria-label={t("tools.pool.auto")}
              />
            </div>
          </div>
        )}

        {doctorMutation.isPending && router.mode === "agent" && (
          <p className="mt-3 flex items-center gap-2 text-sm text-muted-foreground" role="status">
            <Activity className="size-4 animate-pulse text-amber-600" />
            {t("tools.pool.pending")}
          </p>
        )}
      </CardContent>

      <AlertDialog open={confirmExtend} onOpenChange={(open) => !open && setConfirmExtend(false)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t("tools.pool.extendTitle")}</AlertDialogTitle>
            <AlertDialogDescription asChild>
              <span>
                {t("tools.pool.extendDesc1")}
                <br />
                <span className="text-xs text-muted-foreground">{t("tools.pool.extendDesc2")}</span>
                {router.mode === "agent" && (
                  <>
                    <br />
                    <span className="text-xs text-muted-foreground">{t("tools.pool.agentNote")}</span>
                  </>
                )}
              </span>
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={doctorMutation.isPending}>{t("common.cancel")}</AlertDialogCancel>
            <AlertDialogAction
              disabled={doctorMutation.isPending}
              onClick={(e) => {
                e.preventDefault();
                doctorMutation.mutate(true);
              }}
            >
              {doctorMutation.isPending && <Loader2 className="size-4 animate-spin" />}
              {t("tools.pool.extendConfirm")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </Card>
  );
}
