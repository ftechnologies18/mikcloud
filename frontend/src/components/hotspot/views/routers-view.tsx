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
  QrCode,
  Radio,
  Router as RouterIcon,
  ShieldCheck,
  Terminal,
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
import { formatDuration, timeAgo } from "@/lib/hotspot/format";
import type { RouterDevice, RouterMode, RouterRotateTokenResponse, RouterStats, RouterTestResult } from "@/lib/hotspot/types";

interface RouterForm {
  name: string;
  host: string;
  port: string;
  username: string;
  password: string;
  mode: RouterMode;
  hotspotLoginUrl: string;
}

const DEFAULT_FORM: RouterForm = {
  name: "",
  host: "",
  port: "8728",
  username: "",
  password: "",
  mode: "agent",
  hotspotLoginUrl: "",
};

/** Routeur en attente d'installation de l'agent (étape 2 du wizard). */
interface AgentWizard {
  routerId: string;
  name: string;
  script: string;
}

/** Bloc script .rsc copiable — utilisé par le wizard et la réinstallation. */
function ScriptBox({ script }: { script: string }) {
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
      toast.success("Script copié", { description: "Collez-le dans Winbox → Terminal" });
      window.setTimeout(() => setCopied(false), 2500);
    } catch {
      toast.error("Copie impossible — sélectionnez le script manuellement");
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
        {copied ? "Script copié !" : "Copier le script"}
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
      toast.success(`« ${wizard.name} » est en ligne !`, {
        description: "L'agent MikCloud communique avec le routeur (check-in toutes les 45 s).",
      });
    }
  }, [online]);

  return (
    <Dialog open={wizard !== null} onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <ShieldCheck className="size-5 text-emerald-600" />
            Installez l&apos;agent sur « {wizard?.name} »
          </DialogTitle>
          <DialogDescription>
            Une seule fois, 30 secondes — le routeur restera connecté pour toujours, même derrière
            l&apos;Internet d&apos;Orange, en CGNAT ou via Starlink.
          </DialogDescription>
        </DialogHeader>

        <ol className="space-y-2 text-sm">
          {[
            "Ouvrez Winbox et connectez-vous au routeur (adresse MAC ou IP locale).",
            "Cliquez sur « New Terminal » dans le menu de gauche.",
            "Collez le script ci-dessous dans le terminal puis appuyez sur Entrée.",
          ].map((step, i) => (
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
              Routeur en ligne — agent installé avec succès !
            </span>
          ) : (
            <span className="text-amber-700 dark:text-amber-400">
              En attente du premier check-in… (jusqu&apos;à 45 s après le collage du script)
            </span>
          )}
        </div>

        <DialogFooter>
          <Button type="button" onClick={onClose} className={cn(online && "bg-emerald-600 hover:bg-emerald-700")}>
            {online ? "Terminé" : "Fermer — j'installerai plus tard"}
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

/** Valide l'URL de connexion hotspot : vide autorisé, sinon http(s) absolu. */
function isValidHotspotUrl(raw: string): boolean {
  const value = raw.trim();
  if (!value) return true;
  try {
    const url = new URL(value);
    return url.protocol === "http:" || url.protocol === "https:";
  } catch {
    return false;
  }
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
  const [wizard, setWizard] = useState<AgentWizard | null>(null);
  const [reinstall, setReinstall] = useState<RouterDevice | null>(null);
  const [reinstallScript, setReinstallScript] = useState<string | null>(null);

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
      hotspotLoginUrl: router.hotspotLoginUrl ?? "",
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
      // URL de login hotspot (QR des vouchers) — tous modes ; chaîne vide = effacer.
      body.hotspotLoginUrl = payload.form.hotspotLoginUrl.trim();
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
        variables.id ? `Routeur ${router.name} mis à jour` : `Routeur ${router.name} connecté`,
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

  const formValid =
    form.name.trim().length > 0 &&
    (form.mode === "agent" || form.host.trim().length > 0) &&
    isValidHotspotUrl(form.hotspotLoginUrl);
  const hotspotUrlInvalid = !isValidHotspotUrl(form.hotspotLoginUrl);
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
                          {router.mode === "agent"
                            ? router.lastSeen
                              ? `agent · vu ${timeAgo(router.lastSeen)}`
                              : "agent · jamais vu"
                            : `${router.host}:${router.port}`}
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
                            {router.mode === "agent" ? (
                              <DropdownMenuItem
                                className="min-h-10"
                                onClick={() => {
                                  setReinstall(router);
                                  setReinstallScript(null);
                                }}
                              >
                                <Terminal className="size-4" />
                                Script d&apos;installation
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
                              </>
                            )}
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

                    {router.hotspotLoginUrl && (
                      <div
                        className="flex min-h-6 items-center gap-2 text-xs text-muted-foreground"
                        title={`QR des vouchers → ${router.hotspotLoginUrl}`}
                      >
                        <QrCode className="size-3.5 shrink-0" />
                        <span className="shrink-0">Hotspot</span>
                        <span className="truncate font-mono">{router.hotspotLoginUrl}</span>
                      </div>
                    )}

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

            {form.mode !== "agent" && (
              <>
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
              </>
            )}

            <div className="space-y-2">
              <Label htmlFor="router-mode">Mode de connexion</Label>
              <Select
                value={form.mode}
                onValueChange={(v) => setForm((f) => ({ ...f, mode: v as RouterMode }))}
                disabled={saveMutation.isPending}
              >
                <SelectTrigger id="router-mode" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="agent">Agent MikCloud (recommandé)</SelectItem>
                  <SelectItem value="simulated">Simulé (démo)</SelectItem>
                  <SelectItem value="real">Réel — connexion directe (LAN)</SelectItem>
                </SelectContent>
              </Select>
              {form.mode === "agent" && (
                <div className="rounded-lg border border-emerald-500/30 bg-emerald-500/10 p-3 text-xs">
                  <p className="flex items-center gap-1.5 font-medium text-emerald-700 dark:text-emerald-400">
                    <ShieldCheck className="size-3.5" />
                    Aucune configuration réseau requise — juste un nom
                  </p>
                  <p className="mt-1 leading-relaxed text-muted-foreground">
                    Le routeur se connecte lui-même à MikCloud toutes les 45 s. Fonctionne derrière
                    l&apos;Internet Orange CI, en CGNAT et via Starlink : pas d&apos;IP publique, pas de
                    port-forward, pas d&apos;identifiants à stocker. Après création, vous copierez un
                    script et le collerez dans Winbox → Terminal (30 s).
                  </p>
                </div>
              )}
              {form.mode === "real" && (
                <p className="text-xs text-muted-foreground">
                  Connexion directe depuis ce serveur (réseau local / VPN). Activez le service API
                  sur le MikroTik : IP → Services → api (port 8728).
                </p>
              )}
            </div>

            <div className="space-y-2">
              <Label htmlFor="router-hotspot-url">URL de connexion hotspot (QR codes)</Label>
              <Input
                id="router-hotspot-url"
                type="url"
                inputMode="url"
                placeholder="http://10.5.50.1/login ou https://wifi.mondomaine.ci/login"
                value={form.hotspotLoginUrl}
                onChange={(e) => setForm((f) => ({ ...f, hotspotLoginUrl: e.target.value }))}
                disabled={saveMutation.isPending}
                aria-invalid={hotspotUrlInvalid}
              />
              {hotspotUrlInvalid ? (
                <p className="text-xs text-destructive" role="alert">
                  URL invalide — utilisez une adresse commençant par http:// ou https://.
                </p>
              ) : (
                <p className="text-xs text-muted-foreground">
                  Page de login MikroTik du hotspot. Le QR de chaque voucher ouvrira cette page avec
                  le code pré-rempli (connexion en 1 scan). Laissez vide : le QR contiendra le code
                  en texte.
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
                {editing
                  ? "Enregistrer"
                  : form.mode === "agent"
                    ? "Créer et générer le script"
                    : "Ajouter le routeur"}
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

      {/* Wizard agent — étape 2 : script + attente du premier check-in */}
      <AgentInstallDialog wizard={wizard} onClose={() => setWizard(null)} />

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
              Script d&apos;installation — {reinstall?.name}
            </DialogTitle>
            <DialogDescription>
              {reinstallScript
                ? "Copiez ce script et collez-le dans Winbox → Terminal. Il remplace l'ancien agent."
                : "Le token de l'agent n'est jamais stocké en clair : régénérez le script pour l'afficher."}
            </DialogDescription>
          </DialogHeader>

          {reinstallScript ? (
            <ScriptBox script={reinstallScript} />
          ) : (
            <div className="rounded-lg border border-amber-500/40 bg-amber-500/10 p-3 text-sm">
              <p className="font-medium text-amber-700 dark:text-amber-400">
                Attention — régénérer le script crée un nouveau token.
              </p>
              <p className="mt-1 leading-relaxed text-muted-foreground">
                L&apos;agent actuellement installé sur « {reinstall?.name} » cessera de fonctionner
                dès la régénération. À n&apos;utiliser que si vous avez perdu le script d&apos;origine
                ou si vous voulez révoquer un ancien agent.
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
                Terminé
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
                  Annuler
                </Button>
                <Button
                  type="button"
                  disabled={rotateMutation.isPending || reinstall === null}
                  onClick={() => reinstall && rotateMutation.mutate(reinstall.id)}
                >
                  {rotateMutation.isPending && <Loader2 className="size-4 animate-spin" />}
                  Régénérer et afficher le script
                </Button>
              </>
            )}
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
