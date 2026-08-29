"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AnimatePresence, motion } from "framer-motion";
import {
  Activity,
  Clock,
  Cpu,
  Loader2,
  MoreHorizontal,
  Pencil,
  Plus,
  Radio,
  Router as RouterIcon,
  Trash2,
  Users,
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
import { StatusBadge } from "@/components/hotspot/status-badge";
import { api } from "@/lib/hotspot/api";
import { formatDuration } from "@/lib/hotspot/format";
import type { RouterDevice, RouterMode, RouterStats, RouterTestResult } from "@/lib/hotspot/types";

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
  mode: "simulated",
};

function parsePort(raw: string): number {
  const port = parseInt(raw, 10);
  return Number.isFinite(port) && port > 0 && port <= 65535 ? port : 8728;
}

function nf(value: number): string {
  return new Intl.NumberFormat("fr-FR").format(value);
}

export default function RoutersView() {
  const queryClient = useQueryClient();
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editing, setEditing] = useState<RouterDevice | null>(null);
  const [form, setForm] = useState<RouterForm>(DEFAULT_FORM);
  const [deleting, setDeleting] = useState<RouterDevice | null>(null);

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
      const body: Record<string, unknown> = {
        name: payload.form.name.trim(),
        host: payload.form.host.trim(),
        port: parsePort(payload.form.port),
        username: payload.form.username.trim(),
        mode: payload.form.mode,
      };
      if (payload.form.password.length > 0 || !payload.id) body.password = payload.form.password;
      if (payload.id) {
        return api<RouterDevice>(`/api/routers/${payload.id}`, { method: "PUT", body });
      }
      return api<RouterDevice>("/api/routers", { method: "POST", body });
    },
    onSuccess: (router, variables) => {
      toast.success(
        variables.id ? `Routeur ${router.name} mis à jour` : `Routeur ${router.name} connecté`,
      );
      closeDialog(false);
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
        toast.success(`Connecté — latence ${res.latencyMs} ms, RouterOS ${res.version}`);
      } else {
        toast.error(res.message || "Connexion impossible");
      }
    },
    onError: (err: Error) => {
      toast.error(err.message);
    },
  });

  const statsMutation = useMutation({
    mutationFn: (router: RouterDevice) => api<RouterStats>(`/api/routers/${router.id}/stats`),
    onSuccess: (stats, router) => {
      toast.success(`Statistiques — ${router.name}`, {
        description: `CPU ${stats.cpuLoad} % · Mémoire ${stats.memUsedPct} % · Uptime ${formatDuration(stats.uptimeSec)}`,
      });
    },
    onError: (err: Error) => {
      toast.error(err.message);
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (router: RouterDevice) => api<{ ok: boolean }>(`/api/routers/${router.id}`, { method: "DELETE" }),
    onSuccess: (_res, router) => {
      toast.success(`Routeur ${router.name} supprimé`);
      invalidateRouters();
    },
    onError: (err: Error) => {
      toast.error(err.message);
    },
  });

  const formValid = form.name.trim().length > 0 && form.host.trim().length > 0;
  const busyId = testMutation.isPending
    ? testMutation.variables?.id ?? null
    : statsMutation.isPending
      ? statsMutation.variables?.id ?? null
      : null;

  return (
    <div className="space-y-6">
      <PageHeader
        title="Routeurs MikroTik"
        description="Connectez et surveillez vos routeurs en quelques secondes"
        actions={
          <Button onClick={openCreate}>
            <Plus className="size-4" />
            Ajouter un routeur
          </Button>
        }
      />

      {isLoading ? (
        <LoadingCards cards={3} />
      ) : !routers || routers.length === 0 ? (
        <Card>
          <EmptyState
            icon={RouterIcon}
            title="Aucun routeur connecté"
            description="Ajoutez votre premier routeur MikroTik pour gérer vos utilisateurs et vos vouchers hotspot."
            action={
              <Button onClick={openCreate}>
                <Plus className="size-4" />
                Ajouter votre premier routeur
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
                          {router.host}:{router.port}
                        </p>
                      </div>
                      <div className="flex shrink-0 items-center gap-1">
                        <StatusBadge status={router.status} dot />
                        <DropdownMenu>
                          <DropdownMenuTrigger asChild>
                            <Button
                              variant="ghost"
                              size="icon"
                              className="size-9 text-muted-foreground hover:text-foreground"
                              aria-label={`Actions pour ${router.name}`}
                            >
                              <MoreHorizontal className="size-4" />
                            </Button>
                          </DropdownMenuTrigger>
                          <DropdownMenuContent align="end" className="w-52">
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
                              Tester la connexion
                            </DropdownMenuItem>
                            <DropdownMenuItem
                              className="min-h-10"
                              disabled={busyId === router.id}
                              onClick={() => statsMutation.mutate(router)}
                            >
                              <Activity className="size-4" />
                              Statistiques
                            </DropdownMenuItem>
                            <DropdownMenuItem className="min-h-10" onClick={() => openEdit(router)}>
                              <Pencil className="size-4" />
                              Modifier
                            </DropdownMenuItem>
                            <DropdownMenuSeparator />
                            <DropdownMenuItem
                              className="min-h-10 text-destructive focus:text-destructive"
                              onClick={() => setDeleting(router)}
                            >
                              <Trash2 className="size-4" />
                              Supprimer
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
                        <span className="text-muted-foreground">RouterOS</span>
                        <span className="ml-auto font-medium tabular-nums">{router.version || "—"}</span>
                      </div>
                      <div className="flex min-h-6 items-center gap-2 text-sm">
                        <Clock className="size-4 shrink-0 text-muted-foreground" />
                        <span className="text-muted-foreground">Uptime</span>
                        <span className="ml-auto font-medium">{formatDuration(router.uptimeSec)}</span>
                      </div>
                      <div className="flex min-h-6 items-center gap-2 text-sm">
                        <Radio className="size-4 shrink-0 text-muted-foreground" />
                        <span className="text-muted-foreground">Sessions</span>
                        <span className="ml-auto font-medium tabular-nums">{nf(router.activeSessions)}</span>
                      </div>
                      <div className="flex min-h-6 items-center gap-2 text-sm">
                        <Users className="size-4 shrink-0 text-muted-foreground" />
                        <span className="text-muted-foreground">Utilisateurs</span>
                        <span className="ml-auto font-medium tabular-nums">{nf(router.hotspotUsers)}</span>
                      </div>
                    </div>

                    <div className="mt-auto">
                      <div className="mb-1.5 flex items-center justify-between text-xs">
                        <span className="text-muted-foreground">Charge CPU</span>
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
                        aria-label={`Charge CPU de ${router.name}`}
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
            <DialogTitle>{editing ? "Modifier le routeur" : "Ajouter un routeur"}</DialogTitle>
            <DialogDescription>
              {editing
                ? "Ajustez la configuration du routeur. Laissez le mot de passe vide pour le conserver."
                : "Renseignez les accès API de votre MikroTik ou testez en mode simulé."}
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
              <Label htmlFor="router-name">Nom</Label>
              <Input
                id="router-name"
                placeholder="RB-Main-Centre"
                value={form.name}
                onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
                disabled={saveMutation.isPending}
              />
            </div>

            <div className="grid grid-cols-1 gap-4 sm:grid-cols-[1fr_130px]">
              <div className="space-y-2">
                <Label htmlFor="router-host">Adresse IP / hôte</Label>
                <Input
                  id="router-host"
                  placeholder="10.10.10.1"
                  value={form.host}
                  onChange={(e) => setForm((f) => ({ ...f, host: e.target.value }))}
                  disabled={saveMutation.isPending}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="router-port">Port API</Label>
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
                <Label htmlFor="router-username">Utilisateur API</Label>
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
                <Label htmlFor="router-password">Mot de passe API</Label>
                <Input
                  id="router-password"
                  type="password"
                  placeholder={editing ? "Inchangé" : "••••••••"}
                  autoComplete="new-password"
                  value={form.password}
                  onChange={(e) => setForm((f) => ({ ...f, password: e.target.value }))}
                  disabled={saveMutation.isPending}
                />
              </div>
            </div>

            <div className="space-y-2">
              <Label htmlFor="router-mode">Mode</Label>
              <Select
                value={form.mode}
                onValueChange={(v) => setForm((f) => ({ ...f, mode: v as RouterMode }))}
                disabled={saveMutation.isPending}
              >
                <SelectTrigger id="router-mode" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="simulated">Simulé (démo)</SelectItem>
                  <SelectItem value="real">Réel (RouterOS)</SelectItem>
                </SelectContent>
              </Select>
              {form.mode === "real" && (
                <p className="text-xs text-muted-foreground">
                  Activez le service API sur votre MikroTik : IP → Services → api (port 8728)
                </p>
              )}
            </div>

            {saveMutation.isError && (
              <p className="text-sm text-destructive" role="alert">
                {saveMutation.error.message}
              </p>
            )}

            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => closeDialog(false)} disabled={saveMutation.isPending}>
                Annuler
              </Button>
              <Button type="submit" disabled={!formValid || saveMutation.isPending}>
                {saveMutation.isPending && <Loader2 className="size-4 animate-spin" />}
                {editing ? "Enregistrer" : "Ajouter le routeur"}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* Confirmation de suppression */}
      <AlertDialog open={deleting !== null} onOpenChange={(open) => !open && setDeleting(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Supprimer le routeur ?</AlertDialogTitle>
            <AlertDialogDescription>
              {deleting
                ? `Le routeur « ${deleting.name} » sera retiré de MikCloud. Ses utilisateurs hotspot seront conservés.`
                : ""}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleteMutation.isPending}>Annuler</AlertDialogCancel>
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
              Supprimer
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
