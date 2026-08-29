"use client";

import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AnimatePresence, motion } from "framer-motion";
import {
  Activity,
  Check,
  Clock,
  Copy,
  Cpu,
  Loader2,
  MoreHorizontal,
  Pencil,
  Plus,
  Radio,
  Router as RouterIcon,
  ShieldCheck,
  Terminal,
  Trash2,
  Users,
  Wrench,
  Zap,
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
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Progress } from "@/components/ui/progress";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { cn } from "@/lib/utils";
import { EmptyState } from "@/components/hotspot/empty-state";
import { LoadingCards } from "@/components/hotspot/loading";
import { PageHeader } from "@/components/hotspot/page-header";
import { RouterToolsDialog } from "@/components/hotspot/parts/router-tools";
import { StatusBadge } from "@/components/hotspot/status-badge";
import { api } from "@/lib/hotspot/api";
import { localeOf, useI18n } from "@/lib/hotspot/i18n";
import { formatDuration, timeAgo } from "@/lib/hotspot/format";
import type { RouterDevice, RouterMode, RouterRotateTokenResponse, RouterStats, RouterTestResult } from "@/lib/hotspot/types";

interface RouterForm {
  name: string;
  host: string;
  port: string;
  username: string;
  password: string;
  mode: RouterMode;
}

const DEFAULT_FORM: RouterForm = {
  name: "",
  host: "",
  port: "8728",
  username: "",
  password: "",
  mode: "agent",
};

/** Routeur en attente d'installation de l'agent (étape 2 du wizard). */
interface AgentWizard {
  routerId: string;
  name: string;
  script: string;
}

/** Bloc script .rsc copiable — utilisé par le wizard et la réinstallation. */
function ScriptBox({ script }: { script: string }) {
  const { t } = useI18n();
  const [copied, setCopied] = useState(false);

  async function copy() {
    try {
      if (navigator.clipboard && window.isSecureContext) {
        await navigator.clipboard.writeText(script);
      } else {
        const ta = document.createElement("textarea");
        ta.value = script;
        ta.style.position = "fixed";
        ta.style.opacity = "0";
        document.body.appendChild(ta);
        ta.select();
        document.execCommand("copy");
        document.body.removeChild(ta);
      }
      setCopied(true);
      toast.success(t("routers.scriptCopiedToast"), {
        description: t("routers.scriptCopiedToastDesc"),
      });
      window.setTimeout(() => setCopied(false), 2500);
    } catch {
      toast.error(t("routers.copyImpossible"));
    }
  }

  return (
    <div className="space-y-3">
      <div className="max-h-80 overflow-y-auto rounded-lg border bg-zinc-950 p-4 text-left scroll-smooth dark:border-zinc-800">
        <pre className="whitespace-pre-wrap break-all font-mono text-[11px] leading-relaxed text-emerald-300 sm:text-xs">
          {script}
        </pre>
      </div>
      <Button type="button" onClick={copy} className={cn("w-full", copied && "bg-emerald-600 hover:bg-emerald-600")}>
        {copied ? <Check className="size-4" /> : <Copy className="size-4" />}
        {copied ? t("routers.scriptCopiedBtn") : t("routers.copyScript")}
      </Button>
    </div>
  );
}

/** Étape 2 du wizard : instructions + script + statut live du premier check-in. */
function AgentInstallDialog({
  wizard,
  onClose,
}: {
  wizard: AgentWizard | null;
  onClose: () => void;
}) {
  const { t, tf } = useI18n();
  const { data: routers } = useQuery({
    queryKey: ["/api/routers"],
    queryFn: () => api<RouterDevice[]>("/api/routers"),
    enabled: wizard !== null,
    refetchInterval: wizard ? 8_000 : false,
  });
  const router = routers?.find((r) => r.id === wizard?.routerId);
  const online = router?.status === "online";

  useEffect(() => {
    if (online && wizard) {
      toast.success(tf("routers.wizard.onlineToast", { name: wizard.name }), {
        description: t("routers.wizard.onlineToastDesc"),
      });
    }
  }, [online]);

  return (
    <Dialog open={wizard !== null} onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <ShieldCheck className="size-5 text-emerald-600" />
            {tf("routers.wizard.title", { name: wizard?.name ?? "" })}
          </DialogTitle>
          <DialogDescription>{t("routers.wizard.desc")}</DialogDescription>
        </DialogHeader>

        <ol className="space-y-2 text-sm">
          {[t("routers.wizard.step1"), t("routers.wizard.step2"), t("routers.wizard.step3")].map((step, i) => (
            <li key={i} className="flex gap-3">
              <span className="flex size-6 shrink-0 items-center justify-center rounded-full bg-primary font-semibold text-primary-foreground text-xs">
                {i + 1}
              </span>
              <span className="leading-relaxed">{step}</span>
            </li>
          ))}
        </ol>

        {wizard && <ScriptBox script={wizard.script} />}

        <div
          className={cn(
            "flex items-center gap-2.5 rounded-lg border p-3 text-sm",
            online
              ? "border-emerald-500/40 bg-emerald-500/10 text-emerald-700 dark:text-emerald-400"
              : "border-amber-500/40 bg-amber-500/10",
          )}
          role="status"
          aria-live="polite"
        >
          {online ? (
            <Check className="size-4 shrink-0 text-emerald-600" />
          ) : (
            <Loader2 className="size-4 shrink-0 animate-spin text-amber-600" />
          )}
          {online ? (
            <span className="font-medium text-emerald-700 dark:text-emerald-400">
              {t("routers.wizard.online")}
            </span>
          ) : (
            <span className="text-amber-700 dark:text-amber-400">{t("routers.wizard.waiting")}</span>
          )}
        </div>

        <DialogFooter>
          <Button type="button" onClick={onClose} className={cn(online && "bg-emerald-600 hover:bg-emerald-700")}>
            {online ? t("routers.wizard.done") : t("routers.wizard.later")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function parsePort(raw: string): number {
  const port = parseInt(raw, 10);
  return Number.isFinite(port) && port > 0 && port <= 65535 ? port : 8728;
}

export default function RoutersView() {
  const { t, tf, lang } = useI18n();
  const queryClient = useQueryClient();
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editing, setEditing] = useState<RouterDevice | null>(null);
  const [form, setForm] = useState<RouterForm>(DEFAULT_FORM);
  const [deleting, setDeleting] = useState<RouterDevice | null>(null);
  const [wizard, setWizard] = useState<AgentWizard | null>(null);
  const [reinstall, setReinstall] = useState<RouterDevice | null>(null);
  const [reinstallScript, setReinstallScript] = useState<string | null>(null);
  const [toolsRouter, setToolsRouter] = useState<RouterDevice | null>(null);

  const nf = (value: number): string => new Intl.NumberFormat(localeOf(lang)).format(value);

  const { data: routers, isLoading } = useQuery({
    queryKey: ["/api/routers"],
    queryFn: () => api<RouterDevice[]>("/api/routers"),
    refetchInterval: 15_000,
  });

  function invalidateRouters() {
    void queryClient.invalidateQueries({ queryKey: ["/api/routers"] });
    void queryClient.invalidateQueries({ queryKey: ["/api/dashboard"] });
  }

  function openCreate() {
    setEditing(null);
    setForm(DEFAULT_FORM);
    setDialogOpen(true);
  }

  function openEdit(router: RouterDevice) {
    setEditing(router);
    setForm({
      name: router.name,
      host: router.host,
      port: String(router.port),
      username: router.username,
      password: "",
      mode: router.mode,
    });
    setDialogOpen(true);
  }

  function closeDialog(open: boolean) {
    setDialogOpen(open);
    if (!open) setEditing(null);
  }

  const saveMutation = useMutation({
    mutationFn: async (payload: { id: string | null; form: RouterForm }) => {
      const isAgent = payload.form.mode === "agent";
      const body: Record<string, unknown> = {
        name: payload.form.name.trim(),
        mode: payload.form.mode,
      };
      if (!isAgent) {
        body.host = payload.form.host.trim();
        body.port = parsePort(payload.form.port);
        body.username = payload.form.username.trim();
        if (payload.form.password.length > 0 || !payload.id) body.password = payload.form.password;
      }
      if (payload.id) {
        return api<RouterDevice & { installScript?: string }>(`/api/routers/${payload.id}`, {
          method: "PUT",
          body,
        });
      }
      return api<RouterDevice & { installScript?: string }>("/api/routers", { method: "POST", body });
    },
    onSuccess: (router, variables) => {
      invalidateRouters();
      if (router.installScript && !variables.id) {
        // Mode agent : étape 2 — afficher le script d'installation.
        closeDialog(false);
        setWizard({ routerId: router.id, name: router.name, script: router.installScript });
        return;
      }
      toast.success(
        variables.id ? tf("routers.updatedToast", { name: router.name }) : tf("routers.connectedToast", { name: router.name }),
      );
      closeDialog(false);
    },
    onError: (err: Error) => {
      toast.error(err.message);
    },
  });

  const rotateMutation = useMutation({
    mutationFn: (routerId: string) =>
      api<RouterRotateTokenResponse>(`/api/routers/${routerId}/rotate-token`, { method: "POST" }),
    onSuccess: (res) => {
      setReinstallScript(res.installScript);
      invalidateRouters();
    },
    onError: (err: Error) => {
      toast.error(err.message);
    },
  });

  const testMutation = useMutation({
    mutationFn: (router: RouterDevice) =>
      api<RouterTestResult>(`/api/routers/${router.id}/test`, { method: "POST" }),
    onSuccess: (res) => {
      if (res.ok) {
        toast.success(tf("routers.testOk", { latency: res.latencyMs, version: res.version }));
      } else {
        toast.error(res.message || t("routers.testFailed"));
      }
    },
    onError: (err: Error) => {
      toast.error(err.message);
    },
  });

  const statsMutation = useMutation({
    mutationFn: (router: RouterDevice) => api<RouterStats>(`/api/routers/${router.id}/stats`),
    onSuccess: (stats, router) => {
      toast.success(tf("routers.statsToast", { name: router.name }), {
        description: tf("routers.statsToastDesc", {
          cpu: stats.cpuLoad,
          mem: stats.memUsedPct,
          uptime: formatDuration(stats.uptimeSec),
        }),
      });
    },
    onError: (err: Error) => {
      toast.error(err.message);
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (router: RouterDevice) => api<{ ok: boolean }>(`/api/routers/${router.id}`, { method: "DELETE" }),
    onSuccess: (_res, router) => {
      toast.success(tf("routers.deletedToast", { name: router.name }));
      invalidateRouters();
    },
    onError: (err: Error) => {
      toast.error(err.message);
    },
  });

  const formValid =
    form.name.trim().length > 0 && (form.mode === "agent" || form.host.trim().length > 0);
  const busyId = testMutation.isPending
    ? testMutation.variables?.id ?? null
    : statsMutation.isPending
      ? statsMutation.variables?.id ?? null
      : null;

  return (
    <div className="space-y-6">
      <PageHeader
        title={t("routers.title")}
        description={t("routers.description")}
        actions={
          <Button onClick={openCreate}>
            <Plus className="size-4" />
            {t("routers.add")}
          </Button>
        }
      />

      {isLoading ? (
        <LoadingCards cards={3} />
      ) : !routers || routers.length === 0 ? (
        <Card>
          <EmptyState
            icon={RouterIcon}
            title={t("routers.empty")}
            description={t("routers.emptyDesc")}
            action={
              <Button onClick={openCreate}>
                <Plus className="size-4" />
                {t("routers.addFirst")}
              </Button>
            }
          />
        </Card>
      ) : (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
          <AnimatePresence initial={false}>
            {routers.map((router) => (
              <motion.div
                key={router.id}
                layout
                initial={{ opacity: 0, y: 14 }}
                animate={{ opacity: 1, y: 0 }}
                exit={{ opacity: 0, scale: 0.96 }}
                transition={{ duration: 0.25, ease: "easeOut" }}
              >
                <Card className="h-full py-0">
                  <CardContent className="flex h-full flex-col gap-4 p-4 sm:p-6">
                    <div className="flex items-start justify-between gap-2">
                      <div className="min-w-0">
                        <p className="truncate font-semibold leading-tight">{router.name}</p>
                        <p className="mt-1 truncate font-mono text-xs text-muted-foreground">
                          {router.mode === "agent"
                            ? router.lastSeen
                              ? tf("routers.agentSeen", { ago: timeAgo(router.lastSeen, lang) })
                              : t("routers.agentNever")
                            : `${router.host}:${router.port}`}
                        </p>
                      </div>
                      <div className="flex shrink-0 items-center gap-1">
                        <StatusBadge status={router.status} dot />
                        <Button
                          variant="ghost"
                          size="icon"
                          className="size-9 text-muted-foreground hover:text-foreground"
                          onClick={() => setToolsRouter(router)}
                          aria-label={tf("routers.toolsAria", { name: router.name })}
                          title={t("routers.tools")}
                        >
                          <Wrench className="size-4" />
                        </Button>
                        <DropdownMenu>
                          <DropdownMenuTrigger asChild>
                            <Button
                              variant="ghost"
                              size="icon"
                              className="size-9 text-muted-foreground hover:text-foreground"
                              aria-label={tf("common.actionsFor", { name: router.name })}
                            >
                              <MoreHorizontal className="size-4" />
                            </Button>
                          </DropdownMenuTrigger>
                          <DropdownMenuContent align="end" className="w-52">
                            {router.mode === "agent" ? (
                              <DropdownMenuItem
                                className="min-h-10"
                                onClick={() => {
                                  setReinstall(router);
                                  setReinstallScript(null);
                                }}
                              >
                                <Terminal className="size-4" />
                                {t("routers.installScript")}
                              </DropdownMenuItem>
                            ) : (
                              <>
                                <DropdownMenuItem
                                  className="min-h-10"
                                  disabled={busyId === router.id}
                                  onClick={() => testMutation.mutate(router)}
                                >
                                  {busyId === router.id ? (
                                    <Loader2 className="size-4 animate-spin" />
                                  ) : (
                                    <Zap className="size-4" />
                                  )}
                                  {t("routers.test")}
                                </DropdownMenuItem>
                                <DropdownMenuItem
                                  className="min-h-10"
                                  disabled={busyId === router.id}
                                  onClick={() => statsMutation.mutate(router)}
                                >
                                  <Activity className="size-4" />
                                  {t("routers.stats")}
                                </DropdownMenuItem>
                              </>
                            )}
                            <DropdownMenuItem className="min-h-10" onClick={() => openEdit(router)}>
                              <Pencil className="size-4" />
                              {t("common.edit")}
                            </DropdownMenuItem>
                            <DropdownMenuSeparator />
                            <DropdownMenuItem
                              className="min-h-10 text-destructive focus:text-destructive"
                              onClick={() => setDeleting(router)}
                            >
                              <Trash2 className="size-4" />
                              {t("common.delete")}
                            </DropdownMenuItem>
                          </DropdownMenuContent>
                        </DropdownMenu>
                      </div>
                    </div>

                    <div>
                      <StatusBadge status={router.mode} />
                    </div>

                    <div className="grid grid-cols-1 gap-x-4 gap-y-2 sm:grid-cols-2">
                      <div className="flex min-h-6 items-center gap-2 text-sm">
                        <Cpu className="size-4 shrink-0 text-muted-foreground" />
                        <span className="text-muted-foreground">{t("routers.routeros")}</span>
                        <span className="ml-auto font-medium tabular-nums">{router.version || "—"}</span>
                      </div>
                      <div className="flex min-h-6 items-center gap-2 text-sm">
                        <Clock className="size-4 shrink-0 text-muted-foreground" />
                        <span className="text-muted-foreground">{t("routers.uptime")}</span>
                        <span className="ml-auto font-medium">{formatDuration(router.uptimeSec)}</span>
                      </div>
                      <div className="flex min-h-6 items-center gap-2 text-sm">
                        <Radio className="size-4 shrink-0 text-muted-foreground" />
                        <span className="text-muted-foreground">{t("routers.sessions")}</span>
                        <span className="ml-auto font-medium tabular-nums">{nf(router.activeSessions)}</span>
                      </div>
                      <div className="flex min-h-6 items-center gap-2 text-sm">
                        <Users className="size-4 shrink-0 text-muted-foreground" />
                        <span className="text-muted-foreground">{t("routers.users")}</span>
                        <span className="ml-auto font-medium tabular-nums">{nf(router.hotspotUsers)}</span>
                      </div>
                    </div>

                    <div className="mt-auto">
                      <div className="mb-1.5 flex items-center justify-between text-xs">
                        <span className="text-muted-foreground">{t("routers.cpuLoad")}</span>
                        <span
                          className={cn(
                            "font-semibold tabular-nums",
                            router.cpuLoad > 85 ? "text-destructive" : "text-primary",
                          )}
                        >
                          {router.cpuLoad} %
                        </span>
                      </div>
                      <Progress
                        value={router.cpuLoad}
                        aria-label={tf("routers.cpuLoadAria", { name: router.name })}
                        className={cn("h-2", router.cpuLoad > 85 && "[&_[data-slot=progress-indicator]]:bg-destructive")}
                      />
                    </div>
                  </CardContent>
                </Card>
              </motion.div>
            ))}
          </AnimatePresence>
        </div>
      )}

      {/* Dialogue ajouter / modifier */}
      <Dialog open={dialogOpen} onOpenChange={closeDialog}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>{editing ? t("routers.editTitle") : t("routers.addTitle")}</DialogTitle>
            <DialogDescription>
              {editing ? t("routers.editDesc") : t("routers.addDesc")}
            </DialogDescription>
          </DialogHeader>

          <form
            className="space-y-4"
            onSubmit={(e) => {
              e.preventDefault();
              if (!formValid || saveMutation.isPending) return;
              saveMutation.mutate({ id: editing?.id ?? null, form });
            }}
          >
            <div className="space-y-2">
              <Label htmlFor="router-name">{t("routers.name")}</Label>
              <Input
                id="router-name"
                placeholder={t("routers.namePlaceholder")}
                value={form.name}
                onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
                disabled={saveMutation.isPending}
              />
            </div>

            {form.mode !== "agent" && (
              <>
                <div className="grid grid-cols-1 gap-4 sm:grid-cols-[1fr_130px]">
                  <div className="space-y-2">
                    <Label htmlFor="router-host">{t("routers.host")}</Label>
                    <Input
                      id="router-host"
                      placeholder="10.10.10.1"
                      value={form.host}
                      onChange={(e) => setForm((f) => ({ ...f, host: e.target.value }))}
                      disabled={saveMutation.isPending}
                    />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="router-port">{t("routers.port")}</Label>
                    <Input
                      id="router-port"
                      type="number"
                      inputMode="numeric"
                      min={1}
                      max={65535}
                      placeholder="8728"
                      value={form.port}
                      onChange={(e) => setForm((f) => ({ ...f, port: e.target.value }))}
                      disabled={saveMutation.isPending}
                    />
                  </div>
                </div>

                <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                  <div className="space-y-2">
                    <Label htmlFor="router-username">{t("routers.apiUser")}</Label>
                    <Input
                      id="router-username"
                      placeholder="admin"
                      autoComplete="off"
                      value={form.username}
                      onChange={(e) => setForm((f) => ({ ...f, username: e.target.value }))}
                      disabled={saveMutation.isPending}
                    />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="router-password">{t("routers.apiPassword")}</Label>
                    <Input
                      id="router-password"
                      type="password"
                      placeholder={editing ? t("routers.passwordUnchanged") : "••••••••"}
                      autoComplete="new-password"
                      value={form.password}
                      onChange={(e) => setForm((f) => ({ ...f, password: e.target.value }))}
                      disabled={saveMutation.isPending}
                    />
                  </div>
                </div>
              </>
            )}

            <div className="space-y-2">
              <Label htmlFor="router-mode">{t("routers.mode")}</Label>
              <Select
                value={form.mode}
                onValueChange={(v) => setForm((f) => ({ ...f, mode: v as RouterMode }))}
                disabled={saveMutation.isPending}
              >
                <SelectTrigger id="router-mode" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="agent">{t("routers.modeAgent")}</SelectItem>
                  <SelectItem value="simulated">{t("routers.modeSimulated")}</SelectItem>
                  <SelectItem value="real">{t("routers.modeReal")}</SelectItem>
                </SelectContent>
              </Select>
              {form.mode === "agent" && (
                <div className="rounded-lg border border-emerald-500/30 bg-emerald-500/10 p-3 text-xs">
                  <p className="flex items-center gap-1.5 font-medium text-emerald-700 dark:text-emerald-400">
                    <ShieldCheck className="size-3.5" />
                    {t("routers.agentNoConfig")}
                  </p>
                  <p className="mt-1 leading-relaxed text-muted-foreground">{t("routers.agentDesc")}</p>
                </div>
              )}
              {form.mode === "real" && (
                <p className="text-xs text-muted-foreground">{t("routers.realDesc")}</p>
              )}
            </div>

            {saveMutation.isError && (
              <p className="text-sm text-destructive" role="alert">
                {saveMutation.error.message}
              </p>
            )}

            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => closeDialog(false)} disabled={saveMutation.isPending}>
                {t("common.cancel")}
              </Button>
              <Button type="submit" disabled={!formValid || saveMutation.isPending}>
                {saveMutation.isPending && <Loader2 className="size-4 animate-spin" />}
                {editing
                  ? t("common.save")
                  : form.mode === "agent"
                    ? t("routers.createAgent")
                    : t("routers.addSubmit")}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* Confirmation de suppression */}
      <AlertDialog open={deleting !== null} onOpenChange={(open) => !open && setDeleting(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t("routers.deleteTitle")}</AlertDialogTitle>
            <AlertDialogDescription>
              {deleting ? tf("routers.deleteDesc", { name: deleting.name }) : ""}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleteMutation.isPending}>{t("common.cancel")}</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-white hover:bg-destructive/90"
              disabled={deleteMutation.isPending}
              onClick={(e) => {
                e.preventDefault();
                if (deleting) {
                  deleteMutation.mutate(deleting, { onSuccess: () => setDeleting(null) });
                }
              }}
            >
              {deleteMutation.isPending && <Loader2 className="size-4 animate-spin" />}
              {t("common.delete")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* Wizard agent — étape 2 : script + attente du premier check-in */}
      <AgentInstallDialog wizard={wizard} onClose={() => setWizard(null)} />

      {/* Outils routeur (P1) : trafic temps réel, IP bindings, DHCP/hôtes/cookies/journal, système */}
      {toolsRouter && <RouterToolsDialog router={toolsRouter} onClose={() => setToolsRouter(null)} />}

      {/* Réinstallation / rotation du token pour un routeur agent existant */}
      <Dialog
        open={reinstall !== null}
        onOpenChange={(open) => {
          if (!open) {
            setReinstall(null);
            setReinstallScript(null);
          }
        }}
      >
        <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <Terminal className="size-5" />
              {tf("routers.reinstall.title", { name: reinstall?.name ?? "" })}
            </DialogTitle>
            <DialogDescription>
              {reinstallScript ? t("routers.reinstall.withScript") : t("routers.reinstall.noScript")}
            </DialogDescription>
          </DialogHeader>

          {reinstallScript ? (
            <ScriptBox script={reinstallScript} />
          ) : (
            <div className="rounded-lg border border-amber-500/40 bg-amber-500/10 p-3 text-sm">
              <p className="font-medium text-amber-700 dark:text-amber-400">
                {t("routers.reinstall.warning")}
              </p>
              <p className="mt-1 leading-relaxed text-muted-foreground">
                {tf("routers.reinstall.warningDesc", { name: reinstall?.name ?? "" })}
              </p>
            </div>
          )}

          <DialogFooter>
            {reinstallScript ? (
              <Button
                type="button"
                onClick={() => {
                  setReinstall(null);
                  setReinstallScript(null);
                }}
              >
                {t("routers.wizard.done")}
              </Button>
            ) : (
              <>
                <Button
                  type="button"
                  variant="outline"
                  disabled={rotateMutation.isPending}
                  onClick={() => {
                    setReinstall(null);
                    setReinstallScript(null);
                  }}
                >
                  {t("common.cancel")}
                </Button>
                <Button
                  type="button"
                  disabled={rotateMutation.isPending || reinstall === null}
                  onClick={() => reinstall && rotateMutation.mutate(reinstall.id)}
                >
                  {rotateMutation.isPending && <Loader2 className="size-4 animate-spin" />}
                  {t("routers.reinstall.regenerate")}
                </Button>
              </>
            )}
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
