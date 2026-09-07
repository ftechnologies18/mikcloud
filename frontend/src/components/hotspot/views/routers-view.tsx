"use client";

// N°57-d — Vue « Routeurs » de la zone Paramètres (/app/settings/routers),
// réorganisée d'après le retour utilisateur :
//   • la liste est une grille de CARTES CLIQUABLES (focus clavier, aria) ;
//   • cliquer une carte ouvre la FICHE routeur EN PLEINE PAGE — plus de
//     fenêtre modale d'inspection (l'ancien RouterToolsDialog devient un
//     panneau inline, parts/router-tools.tsx RouterToolsPanel) ;
//   • la fiche porte un bouton « Tous les routeurs » (Retour aux cartes) et
//     concentre les actions (test, stats, import, réparation walled-garden,
//     script d'installation, édition, suppression) ;
//   • la fiche est ADRESSABLE : /app/settings/routers/<id> (mécanisme de
//     détail Phase D — router.push à l'ouverture, replace au retour, la
//     discipline « zéro entrée d'historique parasite » du fix 192ad9f est
//     préservée : Retour navigateur et bouton Retour font la même chose).
// Les flux de création restent des dialogues (formulaire d'ajout, wizard
// agent étape 2, réinstallation) : ce sont des ACTIONS, pas des inspections.

import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient, type UseMutationResult } from "@tanstack/react-query";
import { usePathname, useRouter } from "next/navigation";
import { AnimatePresence, motion } from "framer-motion";
import {
  Activity,
  ArrowLeft,
  Check,
  Clock,
  Copy,
  Cpu,
  Download,
  Loader2,
  MoreHorizontal,
  Pencil,
  Plus,
  QrCode,
  Radio,
  Router as RouterIcon,
  ShieldCheck,
  Terminal,
  TriangleAlert,
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
import { RouterToolsPanel } from "@/components/hotspot/parts/router-tools";
import { StatusBadge } from "@/components/hotspot/status-badge";
import { api, repairRouterWalledGarden } from "@/lib/hotspot/api";
import { localeOf, useI18n } from "@/lib/hotspot/i18n";
import { formatDuration, timeAgo } from "@/lib/hotspot/format";
import { viewToPath, detailFromPath } from "@/lib/hotspot/view-path";
import type { RouterDevice, RouterMode, RouterRotateTokenResponse, RouterStats, RouterTestResult } from "@/lib/hotspot/types";
import { Badge } from "@/components/ui/badge";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";

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

        <div
          className="flex items-start gap-2.5 rounded-lg border border-amber-500/40 bg-amber-500/10 p-3 text-xs leading-relaxed"
          role="note"
        >
          <TriangleAlert className="mt-0.5 size-4 shrink-0 text-amber-600" aria-hidden="true" />
          <span className="text-amber-700 dark:text-amber-400">{t("routers.wizard.deviceMode")}</span>
        </div>

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

export default function RoutersView() {
  const { t, tf, lang } = useI18n();
  const queryClient = useQueryClient();
  const router = useRouter();
  const pathname = usePathname();
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editing, setEditing] = useState<RouterDevice | null>(null);
  const [form, setForm] = useState<RouterForm>(DEFAULT_FORM);
  const [deleting, setDeleting] = useState<RouterDevice | null>(null);
  const [wizard, setWizard] = useState<AgentWizard | null>(null);
  const [reinstall, setReinstall] = useState<RouterDevice | null>(null);
  const [reinstallScript, setReinstallScript] = useState<string | null>(null);

  const nf = (value: number): string => new Intl.NumberFormat(localeOf(lang)).format(value);

  const { data: routers, isLoading } = useQuery({
    queryKey: ["/api/routers"],
    queryFn: () => api<RouterDevice[]>("/api/routers"),
    refetchInterval: 15_000,
  });

  // ── Fiche adressable (Phase D, N°57-d) : /app/settings/routers/<id>.
  // Le segment vit dans l'URL — Retour navigateur et bouton « Tous les
  // routeurs » rejouent la même sortie ; le routeur est résolu depuis le
  // poll (fiche « vivante », y compris après un changement hors page).
  const detailId = detailFromPath(pathname, "routers");
  const selected = routers?.find((r) => r.id === detailId) ?? null;

  function openDetail(device: RouterDevice) {
    router.push(viewToPath("routers", device.id), { scroll: false });
  }

  function backToList() {
    // replace : la fiche n'a pas vécu en tant qu'entrée d'historique
    // autonome (même discipline que la fermeture du dialog users, fix 192ad9f).
    router.replace(viewToPath("routers"), { scroll: false });
  }

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
      setDeleting(null);
      // Supprimée depuis la fiche : revenir aux cartes (le routeur n'existe plus).
      if (router.id === detailId) backToList();
    },
    onError: (err: Error) => {
      toast.error(err.message);
    },
  });

  // Import des données EXISTANTES du routeur (profils + utilisateurs hotspot)
  // vers le cloud — utile après la première connexion d'un routeur qui
  // gérait déjà son hotspot via Mikhmon ou à la main. Le résultat est appliqué
  // côté serveur au check-in suivant (≤ 45 s), paginé par lots de 300.
  const importMutation = useMutation({
    mutationFn: (router: RouterDevice) =>
      api<{ queued?: boolean; message?: string }>(`/api/routers/${router.id}/import`, { method: "POST" }),
    onSuccess: (res, router) => {
      toast.success(tf("routers.importToast", { name: router.name }), {
        description: res.message ?? t("routers.importToastDesc"),
      });
      invalidateRouters();
      void queryClient.invalidateQueries({ queryKey: ["/api/users"] });
      void queryClient.invalidateQueries({ queryKey: ["/api/profiles"] });
    },
    onError: (err: Error) => {
      toast.error(err.message);
    },
  });

  // N°49 — réparation forcée du walled-garden d'inscription publique :
  // les règles page+DNS (marqueur mikcloud-wg) sont réappliquées au prochain
  // check-in (≤ 45 s). Utile quand le bouton « S'inscrire » ou le QR ne
  // répondent pas depuis le WiFi — ex. règles supprimées localement.
  const repairWgMutation = useMutation({
    mutationFn: (router: RouterDevice) => repairRouterWalledGarden(router.id),
    onSuccess: (res, router) => {
      toast.success(tf("routers.repairWgToast", { name: router.name }), {
        description: res.message,
      });
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

  // ── FICHE ROUTEUR (page directe, N°57-d) ──────────────────────────────
  // Toutes les actions de la carte vivent ici : la liste reste pure
  // (clic = ouvrir), l'inspection se fait en flux de page.
  if (detailId) {
    return (
      <div className="space-y-4 sm:space-y-6">
        {/* Retour aux cartes — remplace la fiche (aucune entrée parasite),
            même comportement que le bouton Retour du navigateur. */}
        <Button
          type="button"
          variant="ghost"
          className="h-10 -ml-2 gap-2 text-muted-foreground hover:text-foreground"
          onClick={backToList}
        >
          <ArrowLeft className="size-4" />
          {t("routers.backToList")}
        </Button>

        {isLoading || !selected ? (
          <LoadingCards cards={2} />
        ) : (
          <>
            {/* En-tête de fiche : identité + statuts + actions concentrées. */}
            <Card className="py-0">
              <CardContent className="flex flex-col gap-4 p-4 sm:p-6">
                <div className="flex flex-wrap items-start justify-between gap-3">
                  <div className="min-w-0">
                    <div className="flex flex-wrap items-center gap-2">
                      <p className="truncate font-semibold leading-tight text-lg">{selected.name}</p>
                      <StatusBadge status={selected.mode} />
                      <StatusBadge status={selected.status} dot />
                    </div>
                    <p className="mt-1 truncate font-mono text-xs text-muted-foreground">
                      {selected.mode === "agent"
                        ? selected.lastSeen
                          ? tf("routers.agentSeen", { ago: timeAgo(selected.lastSeen, lang) })
                          : t("routers.agentNever")
                        : `${selected.host}:${selected.port}`}
                    </p>
                  </div>

                  <div className="flex shrink-0 items-center gap-2">
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      className="h-10 gap-1.5"
                      disabled={busyId === selected.id}
                      onClick={() => testMutation.mutate(selected)}
                    >
                      {busyId === selected.id ? (
                        <Loader2 className="size-4 animate-spin" />
                      ) : (
                        <Zap className="size-4" />
                      )}
                      {t("routers.test")}
                    </Button>
                    <DropdownMenu>
                      <DropdownMenuTrigger asChild>
                        <Button
                          variant="ghost"
                          size="icon"
                          className="size-10 text-muted-foreground hover:text-foreground"
                          aria-label={tf("common.actionsFor", { name: selected.name })}
                        >
                          <MoreHorizontal className="size-4" />
                        </Button>
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end" className="w-52">
                        {selected.mode === "agent" ? (
                          <>
                            <DropdownMenuItem
                              className="min-h-10"
                              onClick={() => {
                                setReinstall(selected);
                                setReinstallScript(null);
                              }}
                            >
                              <Terminal className="size-4" />
                              {t("routers.installScript")}
                            </DropdownMenuItem>
                            <DropdownMenuItem
                              className="min-h-10"
                              disabled={importMutation.isPending || selected.status !== "online"}
                              title={selected.status !== "online" ? t("routers.importOffline") : undefined}
                              onClick={() => importMutation.mutate(selected)}
                            >
                              {importMutation.isPending ? (
                                <Loader2 className="size-4 animate-spin" />
                              ) : (
                                <Download className="size-4" />
                              )}
                              {t("routers.import")}
                            </DropdownMenuItem>
                            {/* N°49 — réparation du walled-garden : disponible
                                même hors ligne (la commande est filée et
                                exécutée au retour de la connexion). */}
                            <DropdownMenuItem
                              className="min-h-10"
                              disabled={repairWgMutation.isPending}
                              title={t("routers.repairWgHint")}
                              onClick={() => repairWgMutation.mutate(selected)}
                            >
                              {repairWgMutation.isPending ? (
                                <Loader2 className="size-4 animate-spin" />
                              ) : (
                                <ShieldCheck className="size-4" />
                              )}
                              {t("routers.repairWg")}
                            </DropdownMenuItem>
                          </>
                        ) : (
                          <DropdownMenuItem
                            className="min-h-10"
                            disabled={busyId === selected.id}
                            onClick={() => statsMutation.mutate(selected)}
                          >
                            <Activity className="size-4" />
                            {t("routers.stats")}
                          </DropdownMenuItem>
                        )}
                        <DropdownMenuItem className="min-h-10" onClick={() => openEdit(selected)}>
                          <Pencil className="size-4" />
                          {t("common.edit")}
                        </DropdownMenuItem>
                        <DropdownMenuSeparator />
                        <DropdownMenuItem
                          className="min-h-10 text-destructive focus:text-destructive"
                          onClick={() => setDeleting(selected)}
                        >
                          <Trash2 className="size-4" />
                          {t("common.delete")}
                        </DropdownMenuItem>
                      </DropdownMenuContent>
                    </DropdownMenu>
                  </div>
                </div>

                {/* Mesures de santé — même gabarit que l'ancienne carte,
                    enrichi : la fiche est l'écran de référence. */}
                <div className="grid grid-cols-1 gap-x-4 gap-y-2 sm:grid-cols-2">
                  <div className="flex min-h-6 items-center gap-2 text-sm">
                    <Cpu className="size-4 shrink-0 text-muted-foreground" />
                    <span className="text-muted-foreground">{t("routers.routeros")}</span>
                    <span className="ml-auto font-medium tabular-nums">{selected.version || "—"}</span>
                  </div>
                  <div className="flex min-h-6 items-center gap-2 text-sm">
                    <Clock className="size-4 shrink-0 text-muted-foreground" />
                    <span className="text-muted-foreground">{t("routers.uptime")}</span>
                    <span className="ml-auto font-medium">{formatDuration(selected.uptimeSec)}</span>
                  </div>
                  <div className="flex min-h-6 items-center gap-2 text-sm">
                    <Radio className="size-4 shrink-0 text-muted-foreground" />
                    <span className="text-muted-foreground">{t("routers.sessions")}</span>
                    <span className="ml-auto font-medium tabular-nums">{nf(selected.activeSessions)}</span>
                  </div>
                  <div className="flex min-h-6 items-center gap-2 text-sm">
                    <Users className="size-4 shrink-0 text-muted-foreground" />
                    <span className="text-muted-foreground">{t("routers.users")}</span>
                    <span className="ml-auto flex min-w-0 items-center gap-1.5">
                      {/* Purge/résurgence — comptes sur le routeur mais
                          inconnus du cloud (non importés automatiquement
                          quand autoImportRouterUsers = false). */}
                      {selected.unknownOnRouter ? (
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <Badge
                              variant="outline"
                              className="cursor-help border-amber-500/40 bg-amber-500/10 text-amber-700 dark:text-amber-400"
                            >
                              {tf("routers.unknownBadge", { n: selected.unknownOnRouter })}
                            </Badge>
                          </TooltipTrigger>
                          <TooltipContent className="max-w-64">
                            <p>{t("routers.unknownTooltip")}</p>
                          </TooltipContent>
                        </Tooltip>
                      ) : null}
                      <span className="font-medium tabular-nums">{nf(selected.hotspotUsers)}</span>
                    </span>
                  </div>
                </div>

                {selected.hotspotLoginUrl && (
                  <div
                    className="flex min-h-6 items-center gap-2 text-xs text-muted-foreground"
                    title={`QR des vouchers → ${selected.hotspotLoginUrl}`}
                  >
                    <QrCode className="size-3.5 shrink-0" />
                    <span className="shrink-0">Hotspot</span>
                    <span className="truncate font-mono">{selected.hotspotLoginUrl}</span>
                  </div>
                )}

                <div>
                  <div className="mb-1.5 flex items-center justify-between text-xs">
                    <span className="text-muted-foreground">{t("routers.cpuLoad")}</span>
                    <span
                      className={cn(
                        "font-semibold tabular-nums",
                        selected.cpuLoad > 85 ? "text-destructive" : "text-primary",
                      )}
                    >
                      {selected.cpuLoad} %
                    </span>
                  </div>
                  <Progress
                    value={selected.cpuLoad}
                    aria-label={tf("routers.cpuLoadAria", { name: selected.name })}
                    className={cn("h-2", selected.cpuLoad > 85 && "[&_[data-slot=progress-indicator]]:bg-destructive")}
                  />
                </div>
              </CardContent>
            </Card>

            {/* Outils routeur (P1) — trafic temps réel, IP bindings,
                DHCP/hôtes/cookies/journal, système : EN FLUX DE PAGE (fin de
                la modale d'inspection, N°57-d). */}
            <Card className="py-4 sm:py-6">
              <RouterToolsPanel router={selected} />
            </Card>
          </>
        )}

        {/* Dialogues d'action (création/édition, wizard, réinstallation,
            confirmation) — mêmes composants que la liste. */}
        <RouterDialogs
          dialogOpen={dialogOpen}
          onDialogOpen={closeDialog}
          editing={editing}
          form={form}
          setForm={setForm}
          formValid={formValid}
          hotspotUrlInvalid={hotspotUrlInvalid}
          saveMutation={saveMutation}
        />
        <DeleteRouterDialog deleting={deleting} setDeleting={setDeleting} deleteMutation={deleteMutation} />
        <AgentInstallDialog wizard={wizard} onClose={() => setWizard(null)} />
        <ReinstallDialog
          reinstall={reinstall}
          reinstallScript={reinstallScript}
          rotateMutation={rotateMutation}
          onClose={() => {
            setReinstall(null);
            setReinstallScript(null);
          }}
        />
      </div>
    );
  }

  // ── LISTE : GRILLE DE CARTES CLIQUABLES ──────────────────────────────
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
                {/* N°57-d — la CARTE est le contrôle : clic (ou Entrée/Espace
                    au clavier) ouvre la fiche routeur en page directe. Les
                    actions vivent dans la fiche, la carte reste informative. */}
                <Card
                  role="button"
                  tabIndex={0}
                  aria-label={tf("routers.openCardAria", { name: router.name })}
                  onClick={() => openDetail(router)}
                  onKeyDown={(event) => {
                    if (event.key === "Enter" || event.key === " ") {
                      event.preventDefault();
                      openDetail(router);
                    }
                  }}
                  className="group h-full cursor-pointer py-0 outline-none transition-all duration-200 hover:-translate-y-0.5 hover:border-primary/40 hover:shadow-lg focus-visible:ring-2 focus-visible:ring-ring"
                >
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
                      <div className="flex shrink-0 items-center gap-1.5">
                        <StatusBadge status={router.status} dot />
                        <StatusBadge status={router.mode} />
                      </div>
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
      <RouterDialogs
        dialogOpen={dialogOpen}
        onDialogOpen={closeDialog}
        editing={editing}
        form={form}
        setForm={setForm}
        formValid={formValid}
        hotspotUrlInvalid={hotspotUrlInvalid}
        saveMutation={saveMutation}
      />

      {/* Confirmation de suppression */}
      <DeleteRouterDialog deleting={deleting} setDeleting={setDeleting} deleteMutation={deleteMutation} />

      {/* Wizard agent — étape 2 : script + attente du premier check-in */}
      <AgentInstallDialog wizard={wizard} onClose={() => setWizard(null)} />

      {/* Réinstallation / rotation du token pour un routeur agent existant */}
      <ReinstallDialog
        reinstall={reinstall}
        reinstallScript={reinstallScript}
        rotateMutation={rotateMutation}
        onClose={() => {
          setReinstall(null);
          setReinstallScript(null);
        }}
      />
    </div>
  );
}

// ─── Dialogues d'action (formes extraites du rendu principal, inchangées) ───

/** Formulaire ajouter / modifier — dialogue d'ACTION (pas d'inspection). */
function RouterDialogs({
  dialogOpen,
  onDialogOpen,
  editing,
  form,
  setForm,
  formValid,
  hotspotUrlInvalid,
  saveMutation,
}: {
  dialogOpen: boolean;
  onDialogOpen: (open: boolean) => void;
  editing: RouterDevice | null;
  form: RouterForm;
  setForm: React.Dispatch<React.SetStateAction<RouterForm>>;
  formValid: boolean;
  hotspotUrlInvalid: boolean;
  saveMutation: UseMutationResult<RouterDevice & { installScript?: string }, Error, { id: string | null; form: RouterForm }>;
}) {
  const { t } = useI18n();
  return (
    <Dialog open={dialogOpen} onOpenChange={onDialogOpen}>
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
            <Button type="button" variant="outline" onClick={() => onDialogOpen(false)} disabled={saveMutation.isPending}>
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
  );
}

/** Confirmation de suppression — partagée liste/fiche. */
function DeleteRouterDialog({
  deleting,
  setDeleting,
  deleteMutation,
}: {
  deleting: RouterDevice | null;
  setDeleting: (router: RouterDevice | null) => void;
  deleteMutation: UseMutationResult<{ ok: boolean }, Error, RouterDevice>;
}) {
  const { t, tf } = useI18n();
  return (
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
              if (deleting) deleteMutation.mutate(deleting);
            }}
          >
            {deleteMutation.isPending && <Loader2 className="size-4 animate-spin" />}
            {t("common.delete")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

/** Réinstallation / rotation du token — dialogue d'action (régénérer le
 * script d'installation d'un routeur agent). */
function ReinstallDialog({
  reinstall,
  reinstallScript,
  rotateMutation,
  onClose,
}: {
  reinstall: RouterDevice | null;
  reinstallScript: string | null;
  rotateMutation: UseMutationResult<RouterRotateTokenResponse, Error, string>;
  onClose: () => void;
}) {
  const { t, tf } = useI18n();
  return (
    <Dialog
      open={reinstall !== null}
      onOpenChange={(open) => {
        if (!open) onClose();
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
            <Button type="button" onClick={onClose}>
              {t("routers.wizard.done")}
            </Button>
          ) : (
            <>
              <Button type="button" variant="outline" disabled={rotateMutation.isPending} onClick={onClose}>
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
  );
}
