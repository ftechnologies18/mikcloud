"use client";

// Carte F10 — planificateur RouterOS : tâches listées (intervalle,
// prochain passage), ajout via dialog (nom + intervalle + script),
// suppression avec confirmation. Transfert pur depuis router-tools.tsx.

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Clock, Loader2, Plus, RefreshCw, Trash2 } from "lucide-react";
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
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow
} from "@/components/ui/table";
import { EmptyState } from "@/components/hotspot/empty-state";
import { cn } from "@/lib/utils";
import { api } from "@/lib/hotspot/api";
import { useI18n } from "@/lib/hotspot/i18n";
import { timeAgo } from "@/lib/hotspot/format";
import type { RouterDevice, SchedulerRow } from "@/lib/hotspot/types";
import { INTERVAL_RE, QueuedBanner, ToolError, ToolSkeleton, fetchToolEnvelope } from "./shared";

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

export function SchedulerCard({ router }: { router: RouterDevice }) {
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
