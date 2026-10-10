"use client";

// N°294 — vue « Abonnés PPPoE » (console WISP, chantier ⑥ lot UI de N°289) :
// registre des secrets d'un pppoe-server EXISTANT, piloté par l'AGENT MikCloud
// (D1 — commandes ppp_*, zéro cred stockée pour le socle, zéro port public,
// CGNAT-proof) + RENFORT temps réel opt-in via le tunnel WireGuard (phase B —
// l'agent reste le socle : un tunnel mort dégrade le temps réel, jamais le
// contrôle).
//
//   - Toute la vue est scopée à UN routeur (les endpoints PPP sont par
//     routeur) : sélecteur de parc + EmptyState quand aucun routeur agent ;
//   - Onglets : Abonnés (registre cloud, machine à états pending/active/
//     error), Sessions actives (cache agent ≤ 120 s + lecture DIRECTE via
//     tunnel), Découverte (lecture seule des secrets créés hors MikCloud) ;
//   - Renouvellement F4 (gabarit users-view), édition deltas-only (le set
//     partiel est le contrat du ppp_secret_set), suspension/reprise,
//     kick agent (202) ou temps réel (kick-live), suppression confirmée ;
//   - Provisionnement assisté (D2) : script .rsc IDEMPOTENT généré côté
//     backend — MikCloud ne touche JAMAIS au routeur, le WISP colle le
//     script lui-même.
//
// Honnêteté produit (discipline des vues gabarits) : 202 = « lecture en
// file » avec re-poll (≤ 45 s, check-in agent), jamais de donnée inventée ;
// suppression/retrait visibles SEULEMENT à la confirmation du routeur.

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  CalendarPlus,
  Cable,
  Copy,
  Download,
  KeyRound,
  Link2,
  Loader2,
  LogOut,
  MoreHorizontal,
  Pause,
  Pencil,
  Play,
  Plus,
  Radar,
  Radio,
  RotateCw,
  Search,
  Terminal,
  Trash2,
  TriangleAlert,
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
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
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
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Separator } from "@/components/ui/separator";
import { Switch } from "@/components/ui/switch";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  api,
  createPppSecret,
  deletePppApiCreds,
  deletePppSecret,
  getPppApiCreds,
  getPppProvisioningScript,
  kickPppSecret,
  kickPppSecretLive,
  listPppActive,
  listPppSecrets,
  pppDiscover,
  pppLiveActive,
  putPppApiCreds,
  renewPppSecret,
  updatePppSecret,
  type PppLiveActiveResponse,
  type PppProvisioningResponse,
  type PppSecretUpdateBody,
} from "@/lib/hotspot/api";
import { STALE_TIME } from "@/lib/hotspot/query";
import { useI18n } from "@/lib/hotspot/i18n";
import { formatDate, timeAgo } from "@/lib/hotspot/format";
import { copyToClipboard } from "@/components/hotspot/parts/uc-clipboard";
import { EmptyState } from "@/components/hotspot/empty-state";
import { LoadingRows } from "@/components/hotspot/loading";
import { PageHeader } from "@/components/hotspot/page-header";
import { StatCard } from "@/components/hotspot/stat-card";
import { useHotspotStore } from "@/lib/hotspot/store";
import type { PppActiveRow, PppSecret, RouterDevice } from "@/lib/hotspot/types";
import { cn } from "@/lib/utils";

const REFRESH_OPTIONS = [
  { value: "10000", label: "10 s" },
  { value: "30000", label: "30 s" },
  { value: "60000", label: "60 s" },
];

const RENEW_PRESETS = [7, 30, 90, 180, 365];
const REMIND_CHOICES = [0, 1, 3, 7, 15, 30];

/** Fin de journée UTC d'une saisie de date (yyyy-mm-dd) → RFC3339 ; vide =
 * illimité (le contrat du champ expiresAt). */
function endOfDayUtc(dateInput: string): string {
  if (!dateInput) return "";
  const d = new Date(`${dateInput}T23:59:59.000Z`);
  if (Number.isNaN(d.getTime())) return "";
  return d.toISOString();
}

/** Saisie yyyy-mm-dd pré-remplie depuis un RFC3339 (dialog édition). */
function dateInputOf(iso: string | undefined): string {
  if (!iso) return "";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  return d.toISOString().slice(0, 10);
}

/** Jours restants avant l'échéance (arrondi au jour supérieur) ; négatif =
 * expiré. NaN si échéance absente ou illisible. */
function daysLeft(iso: string | undefined): number {
  if (!iso) return Number.NaN;
  const d = new Date(iso).getTime();
  if (Number.isNaN(d)) return Number.NaN;
  return Math.ceil((d - Date.now()) / 86_400_000);
}

type PppTab = "subscribers" | "sessions" | "discover";

export default function PppView() {
  const { t, tf, lang } = useI18n();
  const queryClient = useQueryClient();
  const setView = useHotspotStore((s) => s.setView);

  const [refreshMs, setRefreshMs] = useState(30000);
  const [tab, setTab] = useState<PppTab>("subscribers");
  const [query, setQuery] = useState("");
  const [busyId, setBusyId] = useState<string | null>(null);

  // — Parc + routeur sélectionné (les endpoints PPP sont PAR ROUTEUR) —
  const routersQuery = useQuery({
    queryKey: ["/api/routers"],
    queryFn: () => api<RouterDevice[]>("/api/routers"),
    staleTime: STALE_TIME.operational,
  });
  const routers = routersQuery.data;
  // D1 — le PPPoE est piloté par l'agent : les routeurs simulés n'ont PAS de
  // pppoe-server (réponses vides honnêtes) et les routeurs réels refusent les
  // commandes — le sélecteur ne propose que le mode agent.
  const eligibleRouters = useMemo(() => (routers ?? []).filter((r) => r.mode === "agent"), [routers]);
  const [routerChoice, setRouterChoice] = useState("");
  // Dérivation (pas d'effet) : le choix humain reste tant que le routeur est
  // éligible ; sinon le premier routeur agent (sélection par défaut vivante —
  // suit le parc sans jamais écraser une sélection valide).
  const routerId = eligibleRouters.some((r) => r.id === routerChoice)
    ? routerChoice
    : (eligibleRouters[0]?.id ?? "");
  const router = eligibleRouters.find((r) => r.id === routerId) ?? null;

  // — Données (poll ETag/304, mutations sans optimisme) —
  const secretsQuery = useQuery({
    queryKey: ["/api/routers", routerId, "ppp/secrets"],
    queryFn: () => listPppSecrets(routerId),
    refetchInterval: refreshMs,
    enabled: routerId !== "",
  });
  const credsQuery = useQuery({
    queryKey: ["/api/routers", routerId, "ppp/api-creds"],
    queryFn: () => getPppApiCreds(routerId),
    refetchInterval: 60_000,
    enabled: routerId !== "",
  });
  const activeQuery = useQuery({
    queryKey: ["/api/routers", routerId, "ppp/active"],
    queryFn: () => listPppActive(routerId),
    // 202 (queued) → re-poll accéléré (check-in ≤ 45 s) ; cache frais →
    // entretien 45 s (< TTL serveur 120 s : zéro commande superflue).
    refetchInterval: (q) => (q.state.data?.queued ? 5_000 : 45_000),
    enabled: routerId !== "",
  });
  const discoverQuery = useQuery({
    queryKey: ["/api/routers", routerId, "ppp/discover"],
    queryFn: () => pppDiscover(routerId),
    refetchInterval: (q) => (q.state.data?.queued ? 5_000 : 120_000),
    enabled: routerId !== "" && tab === "discover",
  });

  const secrets = useMemo(() => secretsQuery.data ?? [], [secretsQuery.data]);
  const activeEnv = activeQuery.data;
  const creds = credsQuery.data;

  // Le renfort temps réel n'existe que creds posées ET tunnel présent —
  // sinon les gestes « temps réel » sont masqués (409 côté serveur de toute
  // façon, mais l'UI ne propose pas un geste voué à l'échec).
  const liveAvailable = creds?.configured === true && creds?.hasTunnel === true;

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return secrets;
    return secrets.filter(
      (s) =>
        s.name.toLowerCase().includes(q) ||
        s.profile.toLowerCase().includes(q) ||
        (s.comment ?? "").toLowerCase().includes(q) ||
        (s.staticAddress ?? "").includes(q),
    );
  }, [secrets, query]);

  const invalidatePpp = () => {
    if (routerId === "") return;
    void queryClient.invalidateQueries({ queryKey: ["/api/routers", routerId, "ppp/secrets"] });
    void queryClient.invalidateQueries({ queryKey: ["/api/routers", routerId, "ppp/active"] });
    void queryClient.invalidateQueries({ queryKey: ["/api/routers", routerId, "ppp/discover"] });
    void queryClient.invalidateQueries({ queryKey: ["/api/routers", routerId, "ppp/api-creds"] });
  };

  // La sélection et le résultat temps réel sont vidés au changement de
  // routeur — ajustement PENDANT le rendu (pattern React « You Might Not
  // Need an Effect ») : pas d'effet, pas de rendu en cascade.
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [liveResult, setLiveResult] = useState<PppLiveActiveResponse | null>(null);
  const [liveError, setLiveError] = useState<string | null>(null);
  const [prevRouter, setPrevRouter] = useState(routerId);
  if (prevRouter !== routerId) {
    setPrevRouter(routerId);
    setSelected(new Set());
    setLiveResult(null);
    setLiveError(null);
  }

  const toggleOne = (id: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };
  const allSelected = filtered.length > 0 && filtered.every((s) => selected.has(s.id));
  const someSelected = filtered.some((s) => selected.has(s.id));
  const toggleAll = () => {
    setSelected(allSelected ? new Set() : new Set(filtered.map((s) => s.id)));
  };

  const toastError = (err: unknown) => toast.error(err instanceof Error ? err.message : t("ppp.errorToast"));

  // — Création —
  const [addOpen, setAddOpen] = useState(false);
  const [addName, setAddName] = useState("");
  const [addPassword, setAddPassword] = useState("");
  const [addProfile, setAddProfile] = useState("");
  const [addComment, setAddComment] = useState("");
  const [addExpires, setAddExpires] = useState("");
  const [addStatic, setAddStatic] = useState("");
  const createMutation = useMutation({
    mutationFn: () =>
      createPppSecret(routerId, {
        name: addName.trim().toLowerCase(),
        password: addPassword,
        profile: addProfile.trim(),
        comment: addComment.trim(),
        ...(addExpires ? { expiresAt: endOfDayUtc(addExpires) } : {}),
        ...(addStatic.trim() ? { staticAddress: addStatic.trim() } : {}),
      }),
    onSuccess: (res) => {
      toast.success(res.message || t("ppp.addToast"));
      setAddOpen(false);
      setAddName("");
      setAddPassword("");
      setAddProfile("");
      setAddComment("");
      setAddExpires("");
      setAddStatic("");
      invalidatePpp();
    },
    onError: toastError,
  });

  // — Renouvellement F4 (individuel) —
  const [renewTarget, setRenewTarget] = useState<PppSecret | null>(null);
  const [renewDays, setRenewDays] = useState("30");
  const renewNum = parseInt(renewDays, 10);
  const renewValid = Number.isInteger(renewNum) && renewNum >= 1 && renewNum <= 3650;
  const renewMutation = useMutation({
    mutationFn: (vars: { id: string; days: number }) => renewPppSecret(vars.id, vars.days),
    onSuccess: (res) => {
      toast.success(res.message || t("ppp.renewToast"));
      setRenewTarget(null);
      invalidatePpp();
    },
    onError: toastError,
  });

  // — Renouvellement groupé (sélection multiple) —
  const [bulkRenewOpen, setBulkRenewOpen] = useState(false);
  const bulkRenewMutation = useMutation({
    mutationFn: async (ids: string[]) => {
      let ok = 0;
      let failed = 0;
      for (const id of ids) {
        try {
          await renewPppSecret(id, renewNum);
          ok += 1;
        } catch {
          failed += 1;
        }
      }
      return { ok, failed };
    },
    onSuccess: ({ ok, failed }) => {
      if (failed > 0) toast.warning(tf("ppp.bulk.renewPartial", { n: ok, m: failed }));
      else toast.success(tf("ppp.bulk.renewDone", { n: ok }));
      setBulkRenewOpen(false);
      setSelected(new Set());
      invalidatePpp();
    },
  });

  // — Modification (PATCH deltas-only) + « Réessayer » du badge erreur —
  const [editOpen, setEditOpen] = useState(false);
  const [editTarget, setEditTarget] = useState<PppSecret | null>(null);
  const [editPassword, setEditPassword] = useState("");
  const [editProfile, setEditProfile] = useState("");
  const [editComment, setEditComment] = useState("");
  const [editExpires, setEditExpires] = useState("");
  const [editStatic, setEditStatic] = useState("");
  const [editExpMode, setEditExpMode] = useState("disable");
  const [editAutoRenew, setEditAutoRenew] = useState(false);
  const [editRenewDays, setEditRenewDays] = useState("30");
  const [editRemind, setEditRemind] = useState("0");
  // Dernier PATCH posé par abonné — le badge « erreur » propose de relancer
  // EXACTEMENT le même delta (le set partiel est idempotent côté routeur).
  const [lastPatch, setLastPatch] = useState<{ id: string; body: PppSecretUpdateBody } | null>(null);

  const openEdit = (s: PppSecret) => {
    setEditTarget(s);
    setEditPassword("");
    setEditProfile(s.profile);
    setEditComment(s.comment ?? "");
    setEditExpires(dateInputOf(s.expiresAt));
    setEditStatic(s.staticAddress ?? "");
    setEditExpMode(s.expMode === "none" ? "none" : "disable");
    setEditAutoRenew(s.autoRenew === true);
    setEditRenewDays(String(s.renewDays ?? 30));
    setEditRemind(String(s.remindDays ?? 0));
    setEditOpen(true);
  };

  const updateMutation = useMutation({
    mutationFn: (vars: { id: string; body: PppSecretUpdateBody }) => updatePppSecret(vars.id, vars.body),
    onSuccess: (res, vars) => {
      toast.success(res.message || t("ppp.editToast"));
      setEditOpen(false);
      if (lastPatch?.id === vars.id) setLastPatch(null);
      invalidatePpp();
    },
    onError: toastError,
  });

  function submitEdit() {
    if (!editTarget || updateMutation.isPending) return;
    const body: PppSecretUpdateBody = {};
    if (editPassword !== "") body.password = editPassword;
    if (editProfile.trim() !== editTarget.profile) body.profile = editProfile.trim();
    if (editComment !== (editTarget.comment ?? "")) body.comment = editComment;
    // Comparaison PAR JOUR (yyyy-mm-dd) : le backend stocke le RFC3339 avec
    // sa propre précision — comparer les instants produirait un delta
    // parasite (PATCH superflu, retour inutile en « pending »).
    if (editExpires !== dateInputOf(editTarget.expiresAt)) body.expiresAt = endOfDayUtc(editExpires);
    if (editStatic.trim() !== (editTarget.staticAddress ?? "")) body.staticAddress = editStatic.trim();
    if (editExpMode !== (editTarget.expMode === "none" ? "none" : "disable")) body.expMode = editExpMode;
    if (editAutoRenew !== (editTarget.autoRenew === true)) {
      body.autoRenew = editAutoRenew;
      body.renewDays = Math.max(1, Math.min(3650, parseInt(editRenewDays, 10) || 30));
    }
    if (parseInt(editRemind, 10) !== (editTarget.remindDays ?? 0)) {
      body.remindDays = Math.max(0, Math.min(30, parseInt(editRemind, 10) || 0));
    }
    if (Object.keys(body).length === 0) {
      // Rien modifié — fermer sans requête (le set partiel n'enverrait rien).
      setEditOpen(false);
      return;
    }
    setLastPatch({ id: editTarget.id, body });
    updateMutation.mutate({ id: editTarget.id, body });
  }

  // — Suspension / reprise (confirmation pour suspendre) —
  const [suspendTarget, setSuspendTarget] = useState<PppSecret | null>(null);
  const toggleMutation = useMutation({
    mutationFn: (vars: { id: string; disabled: boolean }) => updatePppSecret(vars.id, { disabled: vars.disabled }),
    onSuccess: (res, vars) => {
      toast.success(res.message || t(vars.disabled ? "ppp.suspendToast" : "ppp.resumeToast"));
      setSuspendTarget(null);
      invalidatePpp();
    },
    onError: toastError,
  });

  // — Déconnection (kick agent 202 / kick-live tunnel) —
  const kickMutation = useMutation({
    mutationFn: (id: string) => kickPppSecret(id),
    onSuccess: (res) => {
      toast.success(res.message || t("ppp.kickToast"));
      invalidatePpp();
    },
    onError: toastError,
    onSettled: () => setBusyId(null),
  });
  const kickLiveMutation = useMutation({
    mutationFn: (id: string) => kickPppSecretLive(id),
    onSuccess: (res) => {
      toast.success(res.message || t("ppp.kickLiveToast"));
      invalidatePpp();
    },
    onError: toastError,
    onSettled: () => setBusyId(null),
  });

  // — Suppression (destructive, retrait du registre à la confirmation agent) —
  const [deleteTarget, setDeleteTarget] = useState<PppSecret | null>(null);
  const deleteMutation = useMutation({
    mutationFn: (id: string) => deletePppSecret(id),
    onSuccess: (res) => {
      toast.success(res.message || t("ppp.deleteToast"));
      setDeleteTarget(null);
      invalidatePpp();
    },
    onError: toastError,
    onSettled: () => setBusyId(null),
  });

  // — Renfort temps réel : credentials API RouterOS (sealed côté backend) —
  const [credsOpen, setCredsOpen] = useState(false);
  const [credsUser, setCredsUser] = useState("");
  const [credsPass, setCredsPass] = useState("");
  const credsMutation = useMutation({
    mutationFn: () => putPppApiCreds(routerId, { username: credsUser.trim(), password: credsPass }),
    onSuccess: (res) => {
      toast.success(res.message || t("ppp.renfort.saveToast"));
      setCredsOpen(false);
      setCredsUser("");
      setCredsPass("");
      invalidatePpp();
    },
    onError: toastError,
  });
  const [credsRemoveOpen, setCredsRemoveOpen] = useState(false);
  const credsRemoveMutation = useMutation({
    mutationFn: () => deletePppApiCreds(routerId),
    onSuccess: (res) => {
      toast.success(res.message || t("ppp.renfort.removeToast"));
      setCredsRemoveOpen(false);
      invalidatePpp();
    },
    onError: toastError,
  });

  // — Lecture temps réel des sessions actives via le tunnel —
  const liveMutation = useMutation({
    mutationFn: () => pppLiveActive(routerId),
    onSuccess: (res) => {
      setLiveResult(res);
      setLiveError(null);
    },
    // 409/502 : le backend renvoie un message DISTINCT et explicite (tunnel
    // absent / creds absentes / API fermée) — affiché tel quel.
    onError: (err) => setLiveError(err instanceof Error ? err.message : t("ppp.errorToast")),
  });

  // — Provisionnement assisté (script .rsc idempotent) —
  const [provOpen, setProvOpen] = useState(false);
  const [provResult, setProvResult] = useState<PppProvisioningResponse | null>(null);
  const [provIface, setProvIface] = useState("");
  const [provService, setProvService] = useState("mikcloud");
  const [provProfile, setProvProfile] = useState("mikcloud-ppp");
  const [provPoolStart, setProvPoolStart] = useState("");
  const [provPoolEnd, setProvPoolEnd] = useState("");
  const [provLocal, setProvLocal] = useState("");
  const [provDns, setProvDns] = useState("");
  const provValid = provIface.trim() !== "" && provPoolStart.trim() !== "" && provPoolEnd.trim() !== "";
  const provMutation = useMutation({
    mutationFn: () =>
      getPppProvisioningScript(routerId, {
        interface: provIface.trim(),
        service: provService.trim(),
        profile: provProfile.trim(),
        poolStart: provPoolStart.trim(),
        poolEnd: provPoolEnd.trim(),
        ...(provLocal.trim() ? { localAddress: provLocal.trim() } : {}),
        ...(provDns.trim() ? { dns: provDns.trim() } : {}),
      }),
    onSuccess: (res) => setProvResult(res),
    onError: toastError,
  });
  const openProv = () => {
    setProvResult(null);
    setProvOpen(true);
  };
  const downloadScript = () => {
    if (!provResult) return;
    const blob = new Blob([provResult.script], { type: "text/plain;charset=utf-8" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `mikcloud-ppp-${router?.name || routerId}.rsc`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
  };

  // — KPIs (tous scopés au routeur sélectionné) —
  const suspendedCount = secrets.filter((s) => s.disabled).length;
  const autoSuspendedCount = secrets.filter((s) => s.disabled && s.autoSuspended === true).length;
  const expiringCount = secrets.filter((s) => {
    const d = daysLeft(s.expiresAt);
    return !Number.isNaN(d) && d >= 0 && d <= 7;
  }).length;
  const onlineValue = activeEnv === undefined ? "…" : activeEnv.queued ? "—" : String(activeEnv.data.length);

  // — États d'attente globaux —
  if (routersQuery.isLoading) {
    return (
      <div className="space-y-4 sm:space-y-6">
        <PageHeader title={t("ppp.title")} description={t("ppp.description")} />
        <Card className="gap-0 py-0">
          <LoadingRows rows={6} />
        </Card>
      </div>
    );
  }

  // Aucun routeur agent : EmptyState (gabarit) invitant au parc routeurs.
  if (eligibleRouters.length === 0) {
    return (
      <div className="space-y-4 sm:space-y-6">
        <PageHeader title={t("ppp.title")} description={t("ppp.description")} />
        <Card className="p-4 sm:p-6">
          <EmptyState
            icon={Cable}
            title={t("ppp.noRouter.title")}
            description={t("ppp.noRouter.desc")}
            action={
              <Button className="min-h-11" onClick={() => setView("routers")}>
                <Plus className="size-4" aria-hidden />
                {t("ppp.noRouter.cta")}
              </Button>
            }
          />
          <p className="mx-auto max-w-md pb-2 text-center text-xs text-muted-foreground">{t("ppp.routerHint")}</p>
        </Card>
      </div>
    );
  }

  /** Menu d'actions d'une ligne — partagé par la table (desktop) et la pile
   * de cartes (mobile). Les gestes temps réel ne sont proposés QUE quand le
   * renfort est disponible (creds + tunnel) — jamais un geste voué au 409. */
  const rowMenu = (s: PppSecret) => {
    const busy =
      busyId === s.id ||
      toggleMutation.isPending ||
      deleteMutation.isPending ||
      kickMutation.isPending ||
      kickLiveMutation.isPending ||
      updateMutation.isPending;
    return (
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            size="icon"
            variant="ghost"
            className="size-8 shrink-0"
            aria-label={t("ppp.more")}
            title={t("ppp.more")}
            disabled={busy}
          >
            <MoreHorizontal className="size-4" aria-hidden />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-56">
          <DropdownMenuItem
            onClick={() => {
              setRenewDays("30");
              setRenewTarget(s);
            }}
          >
            <CalendarPlus className="size-3.5" aria-hidden />
            {t("ppp.renew")}
          </DropdownMenuItem>
          <DropdownMenuItem onClick={() => openEdit(s)}>
            <Pencil className="size-3.5" aria-hidden />
            {t("ppp.edit")}
          </DropdownMenuItem>
          {s.disabled ? (
            <DropdownMenuItem onClick={() => toggleMutation.mutate({ id: s.id, disabled: false })}>
              <Play className="size-3.5" aria-hidden />
              {t("ppp.resume")}
            </DropdownMenuItem>
          ) : (
            <DropdownMenuItem onClick={() => setSuspendTarget(s)}>
              <Pause className="size-3.5" aria-hidden />
              {t("ppp.suspend")}
            </DropdownMenuItem>
          )}
          <DropdownMenuItem
            disabled={busyId === s.id}
            onClick={() => {
              setBusyId(s.id);
              kickMutation.mutate(s.id);
            }}
          >
            <LogOut className="size-3.5" aria-hidden />
            {t("ppp.kick")}
          </DropdownMenuItem>
          {liveAvailable && (
            <DropdownMenuItem
              disabled={busyId === s.id}
              onClick={() => {
                setBusyId(s.id);
                kickLiveMutation.mutate(s.id);
              }}
            >
              <Zap className="size-3.5" aria-hidden />
              {t("ppp.kickLive")}
            </DropdownMenuItem>
          )}
          <DropdownMenuSeparator />
          <DropdownMenuItem className="text-destructive" onClick={() => setDeleteTarget(s)}>
            <Trash2 className="size-3.5" aria-hidden />
            {t("ppp.delete")}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    );
  };

  /** Badge de statut (machine à états : erreur > attente > suspension > actif). */
  const statusBadge = (s: PppSecret) => {
    if (s.state === "error") {
      return (
        <Badge
          className="gap-1.5 border-destructive/30 bg-destructive/10 text-destructive"
          variant="outline"
          title={s.errorMsg ?? undefined}
        >
          <TriangleAlert className="size-3" aria-hidden />
          {t("ppp.state.error")}
        </Badge>
      );
    }
    if (s.state === "pending") {
      return (
        <Badge className="gap-1.5 border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-300" variant="outline">
          <Loader2 className="size-3 animate-spin" aria-hidden />
          {t("ppp.state.pending")}
        </Badge>
      );
    }
    if (s.disabled) {
      return s.autoSuspended === true ? (
        <Badge className="gap-1.5 border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-300" variant="outline">
          <Pause className="size-3" aria-hidden />
          {t("ppp.state.disabledAuto")}
        </Badge>
      ) : (
        <Badge className="gap-1.5 text-muted-foreground" variant="outline">
          <Pause className="size-3" aria-hidden />
          {t("ppp.state.disabled")}
        </Badge>
      );
    }
    return (
      <Badge className="gap-1.5 border-primary/25 bg-primary/10 text-primary" variant="outline">
        <span className="live-dot size-1.5 rounded-full bg-primary" aria-hidden />
        {t("ppp.state.active")}
      </Badge>
    );
  };

  /** Cellule échéance : date + badge rouge « expiré » / ambre « J-X ». */
  const expiryCell = (s: PppSecret) => {
    if (!s.expiresAt) {
      return <span className="text-muted-foreground">{t("ppp.expiresNever")}</span>;
    }
    const d = daysLeft(s.expiresAt);
    return (
      <span className="flex flex-col gap-0.5">
        <span className="tabular-nums">{formatDate(s.expiresAt, lang)}</span>
        {!Number.isNaN(d) && d < 0 && (
          <Badge className="w-fit border-destructive/30 bg-destructive/10 text-destructive" variant="outline">
            {t("ppp.expired")}
          </Badge>
        )}
        {!Number.isNaN(d) && d >= 0 && d <= 7 && (
          <Badge
            className="w-fit border-amber-500/30 bg-amber-500/10 text-amber-700 tabular-nums dark:text-amber-300"
            variant="outline"
          >
            {tf("ppp.expiresIn", { n: d })}
          </Badge>
        )}
      </span>
    );
  };

  /** Rangée de cartes mobile (md:hidden) — même contenu que la table. */
  const mobileCard = (s: PppSecret) => (
    <div
      key={s.id}
      className={cn(
        "flex flex-col rounded-lg border p-4 transition-colors",
        selected.has(s.id) && "border-primary/40 bg-primary/5",
      )}
    >
      <div className="flex items-start justify-between gap-2">
        <div className="flex min-w-0 items-start gap-2.5">
          <Checkbox
            className="mt-0.5"
            checked={selected.has(s.id)}
            onCheckedChange={() => toggleOne(s.id)}
            aria-label={t("ppp.selectAll")}
          />
          <div className="min-w-0">
            <span className="block truncate font-mono text-sm font-medium" title={s.name}>
              {s.name}
            </span>
            {s.comment && (
              <span className="block truncate text-xs text-muted-foreground" title={s.comment}>
                {s.comment}
              </span>
            )}
          </div>
        </div>
        {rowMenu(s)}
      </div>
      <div className="mt-3 flex flex-wrap items-center gap-1.5">
        {statusBadge(s)}
        {(() => {
          const retry = lastPatch && lastPatch.id === s.id ? lastPatch : null;
          if (!retry) return null;
          return (
            <Button
              size="sm"
              variant="outline"
              className="h-7 gap-1 px-2 text-xs"
              disabled={updateMutation.isPending}
              onClick={() => updateMutation.mutate({ id: s.id, body: retry.body })}
            >
              <RotateCw className="size-3" aria-hidden />
              {t("ppp.errorRetry")}
            </Button>
          );
        })()}
        {!s.expiresAt ? (
          <Badge variant="outline" className="text-muted-foreground">
            {t("ppp.expiresNever")}
          </Badge>
        ) : (
          (() => {
            const d = daysLeft(s.expiresAt);
            return (
              <Badge
                variant="outline"
                className={cn(
                  "tabular-nums",
                  !Number.isNaN(d) && d < 0
                    ? "border-destructive/30 bg-destructive/10 text-destructive"
                    : !Number.isNaN(d) && d <= 7
                      ? "border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-300"
                      : "text-muted-foreground",
                )}
              >
                {formatDate(s.expiresAt, lang)}
                {!Number.isNaN(d) && d >= 0 && d <= 7 && ` · ${tf("ppp.expiresIn", { n: d })}`}
              </Badge>
            );
          })()
        )}
      </div>
      <dl className="mt-3 grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-xs">
        <dt className="text-muted-foreground">{t("ppp.profile")}</dt>
        <dd className="truncate text-right text-foreground sm:text-left">{s.profile}</dd>
        <dt className="text-muted-foreground">{t("ppp.staticIp")}</dt>
        <dd className="truncate text-right font-mono text-foreground sm:text-left">
          {s.staticAddress || t("ppp.pool")}
        </dd>
        <dt className="text-muted-foreground">{t("ppp.parity")}</dt>
        <dd className="text-right text-foreground sm:text-left">
          {s.lastSeenOnRouter ? tf("ppp.lastSeen", { time: timeAgo(s.lastSeenOnRouter, lang) }) : t("ppp.neverSeen")}
        </dd>
      </dl>
    </div>
  );

  const tableForActive = (rows: PppActiveRow[]) => (
    <Table>
      <TableHeader>
        <TableRow className="hover:bg-transparent">
          <TableHead className="pl-4 text-muted-foreground sm:pl-6">{t("ppp.sessions.col.name")}</TableHead>
          <TableHead className="hidden text-muted-foreground md:table-cell">{t("ppp.sessions.col.service")}</TableHead>
          <TableHead className="font-mono text-muted-foreground">{t("ppp.sessions.col.address")}</TableHead>
          <TableHead className="hidden font-mono text-muted-foreground lg:table-cell">{t("ppp.sessions.col.caller")}</TableHead>
          <TableHead className="hidden text-muted-foreground sm:table-cell">{t("ppp.sessions.col.uptime")}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {rows.map((row, i) => (
          <TableRow key={`${row.name}-${i}`} className="transition-colors hover:bg-muted/50">
            <TableCell className="max-w-40 pl-4 font-mono text-sm sm:pl-6">
              <span className="block truncate" title={row.name}>
                {row.name}
              </span>
            </TableCell>
            <TableCell className="hidden text-muted-foreground md:table-cell">{row.service || "—"}</TableCell>
            <TableCell className="font-mono text-muted-foreground">{row.address || "—"}</TableCell>
            <TableCell className="hidden max-w-40 truncate font-mono text-muted-foreground lg:table-cell">
              {row.callerId || "—"}
            </TableCell>
            <TableCell className="hidden tabular-nums text-muted-foreground sm:table-cell">{row.uptime || "—"}</TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );

  return (
    <div className="space-y-4 sm:space-y-6">
      <PageHeader
        title={t("ppp.title")}
        description={t("ppp.description")}
        actions={
          <>
            <Select
              value={routerId}
              onValueChange={(value) => setRouterChoice(value)}
              disabled={eligibleRouters.length < 2}
            >
              <SelectTrigger size="sm" className="h-10 w-44 sm:w-56" aria-label={t("ppp.routerLabel")}>
                <SelectValue placeholder={t("ppp.routerLabel")} />
              </SelectTrigger>
              <SelectContent>
                {eligibleRouters.map((r) => (
                  <SelectItem key={r.id} value={r.id}>
                    <span className="flex items-center gap-2">
                      <span
                        className={cn(
                          "size-2 shrink-0 rounded-full",
                          r.status === "online" ? "animate-pulse bg-chart-1" : "bg-destructive",
                        )}
                        aria-hidden
                      />
                      <span className="max-w-40 truncate">{r.name}</span>
                    </span>
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Button size="sm" className="h-10 gap-1.5" onClick={() => setAddOpen(true)}>
              <Plus className="size-4" aria-hidden />
              {t("ppp.add")}
            </Button>
            <Button size="sm" variant="outline" className="h-10 gap-1.5" onClick={openProv}>
              <Terminal className="size-4" aria-hidden />
              <span className="hidden sm:inline">{t("ppp.provision")}</span>
            </Button>
            <div className="relative">
              <Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" aria-hidden />
              <Input
                value={query}
                onChange={(event) => setQuery(event.target.value)}
                placeholder={t("ppp.search")}
                className="h-10 w-40 pl-9 sm:w-56"
                aria-label={t("ppp.search")}
              />
            </div>
            <div className="flex items-center gap-2">
              <span className="hidden text-xs text-muted-foreground sm:inline">{t("sessions.refresh")}</span>
              <Select value={String(refreshMs)} onValueChange={(value) => setRefreshMs(Number(value))}>
                <SelectTrigger size="sm" className="h-10 w-24" aria-label={t("sessions.refreshLabel")}>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {REFRESH_OPTIONS.map((option) => (
                    <SelectItem key={option.value} value={option.value}>
                      {option.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </>
        }
      />

      {/* — KPIs (scopés au routeur sélectionné) — */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <StatCard
          title={t("ppp.kpi.subscribers")}
          value={String(secrets.length)}
          sub={t("ppp.kpi.subscribersSub")}
          icon={Users}
          live
        />
        <StatCard
          title={t("ppp.kpi.online")}
          value={onlineValue}
          sub={activeEnv?.queued ? t("ppp.kpi.onlineUnknown") : t("ppp.kpi.onlineSub")}
          icon={Radio}
          live={!activeEnv?.queued}
        />
        <StatCard
          title={t("ppp.kpi.suspended")}
          value={String(suspendedCount)}
          sub={tf("ppp.kpi.suspendedSub", { n: autoSuspendedCount })}
          icon={Pause}
        />
        <StatCard
          title={t("ppp.kpi.expiring")}
          value={String(expiringCount)}
          sub={t("ppp.kpi.expiringSub")}
          icon={CalendarPlus}
        />
      </div>

      {/* — Renfort temps réel (phase B) : panneau compact, canal agent socle — */}
      <Card className="p-4 sm:p-5">
        <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
          <div className="min-w-0">
            <p className="flex items-center gap-2 text-sm font-medium">
              <Zap className="size-4 text-muted-foreground" aria-hidden />
              {t("ppp.renfort.title")}
            </p>
            <p className="mt-1 text-xs text-muted-foreground">{t("ppp.renfort.desc")}</p>
            <div className="mt-2 flex flex-wrap gap-2">
              <Badge
                className={cn(
                  "gap-1.5",
                  creds?.configured
                    ? "border-primary/25 bg-primary/10 text-primary"
                    : "border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-300",
                )}
                variant="outline"
                title={creds?.username || undefined}
              >
                <span
                  className={cn(
                    "size-1.5 rounded-full",
                    creds?.configured ? "bg-primary" : "bg-amber-500",
                  )}
                  aria-hidden
                />
                <KeyRound className="size-3" aria-hidden />
                {creds?.configured ? t("ppp.renfort.credsOk") : t("ppp.renfort.credsMissing")}
              </Badge>
              <Badge
                className={cn(
                  "gap-1.5",
                  creds?.hasTunnel
                    ? "border-primary/25 bg-primary/10 text-primary"
                    : "border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-300",
                )}
                variant="outline"
                title={creds?.wgIpv4 ? `${creds.wgIpv4} · ${creds.tunnelState}` : undefined}
              >
                <span
                  className={cn("size-1.5 rounded-full", creds?.hasTunnel ? "bg-primary" : "bg-amber-500")}
                  aria-hidden
                />
                <Link2 className="size-3" aria-hidden />
                {creds?.hasTunnel ? t("ppp.renfort.tunnelOk") : t("ppp.renfort.tunnelMissing")}
              </Badge>
            </div>
          </div>
          <div className="flex shrink-0 items-center gap-2">
            <Button size="sm" variant="outline" className="min-h-10" onClick={() => setCredsOpen(true)}>
              <KeyRound className="size-3.5" aria-hidden />
              {t("ppp.renfort.configure")}
            </Button>
            {creds?.configured && (
              <Button
                size="sm"
                variant="ghost"
                className="min-h-10 text-destructive"
                onClick={() => setCredsRemoveOpen(true)}
              >
                {t("ppp.renfort.remove")}
              </Button>
            )}
          </div>
        </div>
        <p className="mt-3 rounded-md border bg-muted/40 p-2.5 text-xs text-muted-foreground">{t("ppp.renfort.note")}</p>
      </Card>

      {/* — Barre d'actions groupées (sélection multiple, onglet Abonnés) — */}
      {tab === "subscribers" && selected.size > 0 && (
        <div
          className="flex flex-wrap items-center gap-2 rounded-lg border bg-muted/30 p-3"
          role="toolbar"
          aria-label={t("ppp.bulk.renew")}
        >
          <span className="text-sm font-medium">{tf("ppp.bulk.selected", { n: selected.size })}</span>
          <Button
            size="sm"
            variant="outline"
            className="min-h-9 gap-1.5"
            disabled={bulkRenewMutation.isPending}
            onClick={() => setBulkRenewOpen(true)}
          >
            <CalendarPlus className="size-3.5" aria-hidden />
            {t("ppp.bulk.renew")}
          </Button>
          <Button size="sm" variant="ghost" className="min-h-9" onClick={() => setSelected(new Set())}>
            {t("common.cancel")}
          </Button>
        </div>
      )}

      <Tabs value={tab} onValueChange={(value) => setTab(value as PppTab)}>
        <TabsList>
          <TabsTrigger value="subscribers" className="gap-1.5">
            <Users className="size-3.5" aria-hidden />
            {t("ppp.tab.subscribers")}
          </TabsTrigger>
          <TabsTrigger value="sessions" className="gap-1.5">
            <Radio className="size-3.5" aria-hidden />
            {t("ppp.tab.sessions")}
          </TabsTrigger>
          <TabsTrigger value="discover" className="gap-1.5">
            <Radar className="size-3.5" aria-hidden />
            {t("ppp.tab.discover")}
          </TabsTrigger>
        </TabsList>

        {/* — Onglet Abonnés : registre cloud du routeur — */}
        <TabsContent value="subscribers" className="mt-3">
          <Card className="gap-0 py-0">
            {secretsQuery.isLoading ? (
              <LoadingRows rows={6} />
            ) : filtered.length === 0 ? (
              secrets.length === 0 ? (
                <EmptyState icon={Users} title={t("ppp.empty.title")} description={t("ppp.empty.desc")} />
              ) : (
                <EmptyState icon={Search} title={t("ppp.noMatch")} description={t("ppp.noMatchDesc")} />
              )
            ) : (
              <>
                {/* Table (desktop) */}
                <div className="hidden overflow-x-auto md:block">
                  <Table>
                    <TableHeader>
                      <TableRow className="hover:bg-transparent">
                        <TableHead className="w-12 pl-4 sm:pl-6">
                          <Checkbox
                            checked={allSelected ? true : someSelected ? "indeterminate" : false}
                            onCheckedChange={toggleAll}
                            aria-label={t("ppp.selectAll")}
                          />
                        </TableHead>
                        <TableHead className="text-muted-foreground">{t("ppp.name")}</TableHead>
                        <TableHead className="hidden text-muted-foreground lg:table-cell">{t("ppp.profile")}</TableHead>
                        <TableHead className="hidden font-mono text-muted-foreground lg:table-cell">
                          {t("ppp.staticIp")}
                        </TableHead>
                        <TableHead className="hidden text-muted-foreground sm:table-cell">{t("ppp.expires")}</TableHead>
                        <TableHead className="text-muted-foreground">{t("ppp.status")}</TableHead>
                        <TableHead className="hidden text-muted-foreground xl:table-cell">{t("ppp.parity")}</TableHead>
                        <TableHead className="pr-4 text-right text-muted-foreground sm:pr-6">{t("ppp.actions")}</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {filtered.map((s) => {
                        const busy = busyId === s.id || deleteMutation.isPending || toggleMutation.isPending;
                        return (
                          <TableRow key={s.id} data-state={selected.has(s.id) ? "selected" : undefined} className="transition-colors hover:bg-muted/50">
                            <TableCell className="pl-4 sm:pl-6">
                              <Checkbox
                                checked={selected.has(s.id)}
                                onCheckedChange={() => toggleOne(s.id)}
                                aria-label={s.name}
                              />
                            </TableCell>
                            <TableCell className="max-w-56">
                              <span className="flex items-center gap-2">
                                <Cable className="size-4 shrink-0 text-muted-foreground" aria-hidden />
                                <span className="min-w-0">
                                  <span className="block truncate font-mono text-sm font-medium" title={s.name}>
                                    {s.name}
                                  </span>
                                  {s.comment && (
                                    <span className="block truncate text-xs text-muted-foreground" title={s.comment}>
                                      {s.comment}
                                    </span>
                                  )}
                                </span>
                              </span>
                            </TableCell>
                            <TableCell className="hidden max-w-32 truncate text-muted-foreground lg:table-cell" title={s.profile}>
                              {s.profile}
                            </TableCell>
                            <TableCell className="hidden font-mono text-muted-foreground lg:table-cell">
                              {s.staticAddress || t("ppp.pool")}
                            </TableCell>
                            <TableCell className="hidden sm:table-cell">{expiryCell(s)}</TableCell>
                            <TableCell>
                              <span className="flex items-center gap-1.5">
                                {statusBadge(s)}
                                {(() => {
                                  const retry = lastPatch && lastPatch.id === s.id ? lastPatch : null;
                                  if (!retry) return null;
                                  return (
                                    <Button
                                      size="icon"
                                      variant="ghost"
                                      className="size-7"
                                      aria-label={t("ppp.errorRetry")}
                                      title={t("ppp.errorRetry")}
                                      disabled={updateMutation.isPending}
                                      onClick={() => updateMutation.mutate({ id: s.id, body: retry.body })}
                                    >
                                      <RotateCw className="size-3.5" aria-hidden />
                                    </Button>
                                  );
                                })()}
                              </span>
                            </TableCell>
                            <TableCell className="hidden text-xs text-muted-foreground xl:table-cell">
                              {s.lastSeenOnRouter
                                ? tf("ppp.lastSeen", { time: timeAgo(s.lastSeenOnRouter, lang) })
                                : t("ppp.neverSeen")}
                            </TableCell>
                            <TableCell className="pr-4 sm:pr-6">
                              <div className="flex items-center justify-end gap-1">
                                <Button
                                  size="sm"
                                  variant="outline"
                                  className="h-8 gap-1.5"
                                  disabled={busy || renewMutation.isPending}
                                  onClick={() => {
                                    setRenewDays("30");
                                    setRenewTarget(s);
                                  }}
                                >
                                  <CalendarPlus className="size-3.5" aria-hidden />
                                  {t("ppp.renew")}
                                </Button>
                                {rowMenu(s)}
                              </div>
                            </TableCell>
                          </TableRow>
                        );
                      })}
                    </TableBody>
                  </Table>
                </div>
                {/* Pile de cartes (mobile) — mêmes données, touch ≥ 44 px. */}
                <div className="grid gap-3 p-4 md:hidden">{filtered.map((s) => mobileCard(s))}</div>
                <p className="border-t px-4 py-2.5 text-xs text-muted-foreground sm:px-6">{t("ppp.addDesc")}</p>
              </>
            )}
          </Card>
        </TabsContent>

        {/* — Onglet Sessions actives : cache agent + temps réel via tunnel — */}
        <TabsContent value="sessions" className="mt-3">
          <Card className="gap-0 py-0">
            <div className="flex flex-wrap items-center justify-between gap-2 px-4 py-3 sm:px-6">
              <p className="text-sm text-muted-foreground">
                {activeEnv?.queued
                  ? t("ppp.sessions.queued")
                  : activeEnv?.updatedAt
                    ? tf("ppp.sessions.updated", { time: timeAgo(activeEnv.updatedAt, lang) })
                    : ""}
              </p>
              <Button
                size="sm"
                variant="outline"
                className="min-h-9 gap-1.5"
                disabled={liveMutation.isPending}
                onClick={() => liveMutation.mutate()}
              >
                {liveMutation.isPending ? (
                  <Loader2 className="size-3.5 animate-spin" aria-hidden />
                ) : (
                  <Zap className="size-3.5" aria-hidden />
                )}
                {liveMutation.isPending ? t("ppp.sessions.livePending") : t("ppp.sessions.live")}
              </Button>
            </div>
            <Separator />
            <div aria-live="polite">
              {activeQuery.isLoading ? (
                <LoadingRows rows={4} />
              ) : activeEnv?.queued ? (
                <div className="flex items-center justify-center gap-2 px-6 py-10 text-sm text-muted-foreground" role="status">
                  <Loader2 className="size-4 animate-spin" aria-hidden />
                  {t("ppp.sessions.queued")}
                </div>
              ) : (activeEnv?.data.length ?? 0) === 0 ? (
                <EmptyState icon={Radio} title={t("ppp.sessions.empty")} description={t("ppp.sessions.emptyDesc")} />
              ) : (
                tableForActive(activeEnv?.data ?? [])
              )}
            </div>
            {/* Résultat temps réel (ou diagnostic 409/502 explicite) */}
            {(liveError || liveResult) && (
              <>
                <Separator />
                <div className="space-y-3 p-4 sm:p-6">
                  {liveError && (
                    <Alert variant="destructive">
                      <TriangleAlert className="size-4" aria-hidden />
                      <AlertTitle>{t("ppp.sessions.liveErrorTitle")}</AlertTitle>
                      <AlertDescription>{liveError}</AlertDescription>
                    </Alert>
                  )}
                  {liveResult && (
                    <>
                      <p className="text-sm text-muted-foreground">
                        {tf("ppp.sessions.liveCount", { n: liveResult.count, ms: liveResult.latencyMs })}
                      </p>
                      {liveResult.data.length === 0 ? (
                        <EmptyState icon={Radio} title={t("ppp.sessions.empty")} description={t("ppp.sessions.emptyDesc")} />
                      ) : (
                        tableForActive(liveResult.data)
                      )}
                    </>
                  )}
                </div>
              </>
            )}
          </Card>
        </TabsContent>

        {/* — Onglet Découverte : lecture seule des secrets côté routeur — */}
        <TabsContent value="discover" className="mt-3">
          <Card className="gap-0 py-0">
            <div className="space-y-3 p-4 sm:p-6">
              <Alert className="border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-300">
                <TriangleAlert className="size-4" aria-hidden />
                <AlertTitle>{t("ppp.discover.title")}</AlertTitle>
                <AlertDescription>{t("ppp.discover.note")}</AlertDescription>
              </Alert>
              {discoverQuery.data?.updatedAt && (
                <p className="text-xs text-muted-foreground">
                  {tf("ppp.discover.updated", { time: timeAgo(discoverQuery.data.updatedAt, lang) })}
                </p>
              )}
            </div>
            <Separator />
            <div aria-live="polite">
              {discoverQuery.isLoading ? (
                <LoadingRows rows={4} />
              ) : discoverQuery.data?.queued ? (
                <div className="flex items-center justify-center gap-2 px-6 py-10 text-sm text-muted-foreground" role="status">
                  <Loader2 className="size-4 animate-spin" aria-hidden />
                  {t("ppp.discover.queued")}
                </div>
              ) : (discoverQuery.data?.data.length ?? 0) === 0 ? (
                <EmptyState icon={Radar} title={t("ppp.discover.empty")} description={t("ppp.discover.emptyDesc")} />
              ) : (
                <Table>
                  <TableHeader>
                    <TableRow className="hover:bg-transparent">
                      <TableHead className="pl-4 text-muted-foreground sm:pl-6">{t("ppp.discover.col.name")}</TableHead>
                      <TableHead className="hidden text-muted-foreground sm:table-cell">{t("ppp.discover.col.profile")}</TableHead>
                      <TableHead className="text-muted-foreground">{t("ppp.discover.col.disabled")}</TableHead>
                      <TableHead className="hidden text-muted-foreground md:table-cell">{t("ppp.discover.col.service")}</TableHead>
                      <TableHead className="hidden text-muted-foreground lg:table-cell">{t("ppp.discover.col.comment")}</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {(discoverQuery.data?.data ?? []).map((row, i) => (
                      <TableRow key={`${row.name}-${i}`} className="transition-colors hover:bg-muted/50">
                        <TableCell className="max-w-40 pl-4 font-mono text-sm sm:pl-6">
                          <span className="block truncate" title={row.name}>
                            {row.name}
                          </span>
                        </TableCell>
                        <TableCell className="hidden max-w-32 truncate text-muted-foreground sm:table-cell" title={row.profile}>
                          {row.profile || "—"}
                        </TableCell>
                        <TableCell>
                          {row.disabled ? (
                            <Badge className="gap-1.5 border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-300" variant="outline">
                              <Pause className="size-3" aria-hidden />
                              {t("common.yes")}
                            </Badge>
                          ) : (
                            <Badge className="gap-1.5 text-muted-foreground" variant="outline">
                              {t("common.no")}
                            </Badge>
                          )}
                        </TableCell>
                        <TableCell className="hidden text-muted-foreground md:table-cell">{row.service || "—"}</TableCell>
                        <TableCell className="hidden max-w-56 truncate text-muted-foreground lg:table-cell" title={row.comment}>
                          {row.comment || "—"}
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              )}
            </div>
          </Card>
        </TabsContent>
      </Tabs>

      {/* — Dialog « Nouvel abonné » — */}
      <Dialog open={addOpen} onOpenChange={setAddOpen}>
        <DialogContent className="max-h-[85dvh] overflow-y-auto sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{t("ppp.addTitle")}</DialogTitle>
            <DialogDescription>{t("ppp.addDesc")}</DialogDescription>
          </DialogHeader>
          <form
            className="space-y-3 py-1"
            onSubmit={(event) => {
              event.preventDefault();
              if (addName.trim() === "" || addProfile.trim() === "") return;
              createMutation.mutate();
            }}
          >
            <div className="space-y-1.5">
              <Label htmlFor="ppp-add-name">{t("ppp.addName")}</Label>
              <Input
                id="ppp-add-name"
                value={addName}
                onChange={(event) => setAddName(event.target.value)}
                placeholder={t("ppp.addNamePlaceholder")}
                maxLength={64}
                autoComplete="off"
                autoFocus
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="ppp-add-password">{t("ppp.addPassword")}</Label>
              <Input
                id="ppp-add-password"
                value={addPassword}
                onChange={(event) => setAddPassword(event.target.value)}
                placeholder={t("ppp.addPasswordPlaceholder")}
                maxLength={64}
                autoComplete="new-password"
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="ppp-add-profile">{t("ppp.addProfile")}</Label>
              <Input
                id="ppp-add-profile"
                value={addProfile}
                onChange={(event) => setAddProfile(event.target.value)}
                placeholder={t("ppp.addProfilePlaceholder")}
                maxLength={64}
              />
              <p className="text-xs text-muted-foreground">{t("ppp.addProfileHint")}</p>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="ppp-add-comment">{t("ppp.addComment")}</Label>
              <Input
                id="ppp-add-comment"
                value={addComment}
                onChange={(event) => setAddComment(event.target.value)}
                maxLength={120}
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="ppp-add-expires">{t("ppp.addExpires")}</Label>
              <Input
                id="ppp-add-expires"
                type="date"
                value={addExpires}
                onChange={(event) => setAddExpires(event.target.value)}
              />
              <p className="text-xs text-muted-foreground">{t("ppp.addExpiresHint")}</p>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="ppp-add-static">{t("ppp.addStatic")}</Label>
              <Input
                id="ppp-add-static"
                value={addStatic}
                onChange={(event) => setAddStatic(event.target.value)}
                placeholder="10.10.0.25"
                inputMode="numeric"
              />
              <p className="text-xs text-muted-foreground">{t("ppp.addStaticHint")}</p>
            </div>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setAddOpen(false)}>
                {t("common.cancel")}
              </Button>
              <Button
                type="submit"
                disabled={createMutation.isPending || addName.trim() === "" || addProfile.trim() === ""}
              >
                {createMutation.isPending ? (
                  <>
                    <Loader2 className="size-4 animate-spin" aria-hidden />
                    {t("ppp.addPending")}
                  </>
                ) : (
                  t("ppp.addCta")
                )}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* — Dialog « Renouveler » (F4) — */}
      <Dialog open={renewTarget !== null} onOpenChange={(open) => !open && setRenewTarget(null)}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{tf("ppp.renewTitle", { name: renewTarget?.name ?? "" })}</DialogTitle>
            <DialogDescription>
              {renewTarget?.expiresAt
                ? tf("ppp.renewCurrent", { date: formatDate(renewTarget.expiresAt, lang) })
                : t("ppp.renewNoDate")}{" "}
              {t("ppp.renewAuto")}
            </DialogDescription>
          </DialogHeader>
          <form
            className="grid gap-4"
            onSubmit={(event) => {
              event.preventDefault();
              if (!renewValid || renewMutation.isPending || !renewTarget) return;
              renewMutation.mutate({ id: renewTarget.id, days: renewNum });
            }}
          >
            <div className="grid gap-2">
              <Label htmlFor="ppp-renew-days">{t("ppp.renewDays")}</Label>
              <div className="flex flex-wrap gap-2">
                {RENEW_PRESETS.map((preset) => (
                  <Button
                    key={preset}
                    type="button"
                    variant={renewDays === String(preset) ? "secondary" : "outline"}
                    className="min-h-10 flex-1"
                    disabled={renewMutation.isPending}
                    onClick={() => setRenewDays(String(preset))}
                  >
                    {preset} j
                  </Button>
                ))}
              </div>
              <Input
                id="ppp-renew-days"
                type="number"
                min={1}
                max={3650}
                value={renewDays}
                onChange={(event) => setRenewDays(event.target.value)}
                disabled={renewMutation.isPending}
                aria-invalid={!renewValid}
              />
              <p className="text-xs text-muted-foreground">{t("ppp.renewDaysHint")}</p>
              <p className="rounded-md border bg-muted/40 p-2.5 text-xs text-muted-foreground">{t("ppp.renewAutoNote")}</p>
            </div>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setRenewTarget(null)} disabled={renewMutation.isPending}>
                {t("common.cancel")}
              </Button>
              <Button type="submit" disabled={!renewValid || renewMutation.isPending}>
                {renewMutation.isPending && <Loader2 className="size-4 animate-spin" aria-hidden />}
                {t("ppp.renewSubmit")}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* — Dialog « Modifier » (PATCH deltas-only) — */}
      <Dialog open={editOpen} onOpenChange={setEditOpen}>
        <DialogContent className="max-h-[85dvh] overflow-y-auto sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{tf("ppp.editTitle", { name: editTarget?.name ?? "" })}</DialogTitle>
            <DialogDescription>{t("ppp.editDesc")}</DialogDescription>
          </DialogHeader>
          <form
            className="space-y-3 py-1"
            onSubmit={(event) => {
              event.preventDefault();
              submitEdit();
            }}
          >
            <div className="space-y-1.5">
              <Label htmlFor="ppp-edit-password">{t("ppp.editPassword")}</Label>
              <Input
                id="ppp-edit-password"
                type="password"
                value={editPassword}
                onChange={(event) => setEditPassword(event.target.value)}
                placeholder="••••••••"
                maxLength={64}
                autoComplete="new-password"
              />
              <p className="text-xs text-muted-foreground">{t("ppp.editPasswordHint")}</p>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="ppp-edit-profile">{t("ppp.editProfile")}</Label>
              <Input
                id="ppp-edit-profile"
                value={editProfile}
                onChange={(event) => setEditProfile(event.target.value)}
                maxLength={64}
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="ppp-edit-comment">{t("ppp.editComment")}</Label>
              <Input
                id="ppp-edit-comment"
                value={editComment}
                onChange={(event) => setEditComment(event.target.value)}
                maxLength={120}
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="ppp-edit-expires">{t("ppp.editExpires")}</Label>
              <Input
                id="ppp-edit-expires"
                type="date"
                value={editExpires}
                onChange={(event) => setEditExpires(event.target.value)}
              />
              <p className="text-xs text-muted-foreground">{t("ppp.editExpiresHint")}</p>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="ppp-edit-static">{t("ppp.editStatic")}</Label>
              <Input
                id="ppp-edit-static"
                value={editStatic}
                onChange={(event) => setEditStatic(event.target.value)}
                placeholder="10.10.0.25"
                inputMode="numeric"
              />
              <p className="text-xs text-muted-foreground">{t("ppp.editStaticHint")}</p>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="ppp-edit-expmode">{t("ppp.editExpMode")}</Label>
              <Select value={editExpMode} onValueChange={setEditExpMode}>
                <SelectTrigger id="ppp-edit-expmode" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="disable">{t("ppp.editExpMode.disable")}</SelectItem>
                  <SelectItem value="none">{t("ppp.editExpMode.none")}</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="flex items-center justify-between gap-3 rounded-lg border p-3">
              <div className="min-w-0">
                <Label htmlFor="ppp-edit-autorenew" className="text-sm">
                  {t("ppp.editAutoRenew")}
                </Label>
                <p className="mt-0.5 text-xs text-muted-foreground">{t("ppp.renewAutoNote")}</p>
              </div>
              <Switch
                id="ppp-edit-autorenew"
                checked={editAutoRenew}
                onCheckedChange={setEditAutoRenew}
                aria-label={t("ppp.editAutoRenew")}
              />
            </div>
            {editAutoRenew && (
              <div className="space-y-1.5">
                <Label htmlFor="ppp-edit-renewdays">{t("ppp.editRenewDays")}</Label>
                <Input
                  id="ppp-edit-renewdays"
                  type="number"
                  min={1}
                  max={3650}
                  value={editRenewDays}
                  onChange={(event) => setEditRenewDays(event.target.value)}
                />
              </div>
            )}
            <div className="space-y-1.5">
              <Label htmlFor="ppp-edit-remind">{t("ppp.editRemind")}</Label>
              <Select value={editRemind} onValueChange={setEditRemind}>
                <SelectTrigger id="ppp-edit-remind" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {REMIND_CHOICES.map((d) => (
                    <SelectItem key={d} value={String(d)}>
                      {d === 0 ? t("ppp.editRemind.off") : tf("ppp.editRemind.days", { n: d })}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setEditOpen(false)} disabled={updateMutation.isPending}>
                {t("common.cancel")}
              </Button>
              <Button type="submit" disabled={updateMutation.isPending || editProfile.trim() === ""}>
                {updateMutation.isPending && <Loader2 className="size-4 animate-spin" aria-hidden />}
                {t("ppp.editSubmit")}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* — Dialog « Renouveler la sélection » — */}
      <Dialog open={bulkRenewOpen} onOpenChange={setBulkRenewOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{t("ppp.bulk.renew")}</DialogTitle>
            <DialogDescription>{tf("ppp.bulk.selected", { n: selected.size })}</DialogDescription>
          </DialogHeader>
          <form
            className="grid gap-4"
            onSubmit={(event) => {
              event.preventDefault();
              if (!renewValid || bulkRenewMutation.isPending || selected.size === 0) return;
              bulkRenewMutation.mutate(Array.from(selected));
            }}
          >
            <div className="grid gap-2">
              <Label htmlFor="ppp-bulk-days">{t("ppp.renewDays")}</Label>
              <div className="flex flex-wrap gap-2">
                {RENEW_PRESETS.map((preset) => (
                  <Button
                    key={preset}
                    type="button"
                    variant={renewDays === String(preset) ? "secondary" : "outline"}
                    className="min-h-10 flex-1"
                    disabled={bulkRenewMutation.isPending}
                    onClick={() => setRenewDays(String(preset))}
                  >
                    {preset} j
                  </Button>
                ))}
              </div>
              <Input
                id="ppp-bulk-days"
                type="number"
                min={1}
                max={3650}
                value={renewDays}
                onChange={(event) => setRenewDays(event.target.value)}
                disabled={bulkRenewMutation.isPending}
                aria-invalid={!renewValid}
              />
              <p className="text-xs text-muted-foreground">{t("ppp.renewAutoNote")}</p>
            </div>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setBulkRenewOpen(false)} disabled={bulkRenewMutation.isPending}>
                {t("common.cancel")}
              </Button>
              <Button type="submit" disabled={!renewValid || bulkRenewMutation.isPending || selected.size === 0}>
                {bulkRenewMutation.isPending && <Loader2 className="size-4 animate-spin" aria-hidden />}
                {t("ppp.bulk.renew")}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* — Confirmation de suspension — */}
      <AlertDialog
        open={suspendTarget !== null}
        onOpenChange={(open) => {
          if (!open) setSuspendTarget(null);
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t("ppp.suspendConfirmTitle")}</AlertDialogTitle>
            <AlertDialogDescription>
              {suspendTarget ? `${suspendTarget.name} — ` : ""}
              {t("ppp.suspendConfirmDesc")}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
            <AlertDialogAction
              disabled={toggleMutation.isPending}
              onClick={(event) => {
                event.preventDefault();
                if (suspendTarget) toggleMutation.mutate({ id: suspendTarget.id, disabled: true });
              }}
            >
              {toggleMutation.isPending && <Loader2 className="size-4 animate-spin" aria-hidden />}
              {t("ppp.suspend")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* — Confirmation de suppression (destructive) — */}
      <AlertDialog
        open={deleteTarget !== null}
        onOpenChange={(open) => {
          if (!open) setDeleteTarget(null);
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t("ppp.deleteConfirmTitle")}</AlertDialogTitle>
            <AlertDialogDescription>
              {deleteTarget ? `${deleteTarget.name} — ` : ""}
              {t("ppp.deleteConfirmDesc")}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleteMutation.isPending}>{t("common.cancel")}</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-white hover:bg-destructive/90"
              disabled={deleteMutation.isPending}
              onClick={(event) => {
                event.preventDefault();
                if (!deleteTarget) return;
                setBusyId(deleteTarget.id);
                deleteMutation.mutate(deleteTarget.id);
              }}
            >
              {deleteMutation.isPending ? (
                <Loader2 className="size-4 animate-spin" aria-hidden />
              ) : (
                <Trash2 className="size-4" aria-hidden />
              )}
              {t("ppp.delete")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* — Dialog credentials du renfort temps réel — */}
      <Dialog open={credsOpen} onOpenChange={setCredsOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{t("ppp.renfort.dialogTitle")}</DialogTitle>
            <DialogDescription>{t("ppp.renfort.dialogDesc")}</DialogDescription>
          </DialogHeader>
          <form
            className="space-y-3 py-1"
            onSubmit={(event) => {
              event.preventDefault();
              if (credsUser.trim() === "" || credsPass === "") return;
              credsMutation.mutate();
            }}
          >
            <div className="space-y-1.5">
              <Label htmlFor="ppp-creds-user">{t("ppp.renfort.username")}</Label>
              <Input
                id="ppp-creds-user"
                value={credsUser}
                onChange={(event) => setCredsUser(event.target.value)}
                placeholder={t("ppp.renfort.usernamePlaceholder")}
                maxLength={64}
                autoComplete="off"
                autoFocus
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="ppp-creds-pass">{t("ppp.renfort.password")}</Label>
              <Input
                id="ppp-creds-pass"
                type="password"
                value={credsPass}
                onChange={(event) => setCredsPass(event.target.value)}
                placeholder={t("ppp.renfort.passwordPlaceholder")}
                maxLength={64}
                autoComplete="new-password"
              />
            </div>
            <p className="rounded-md border bg-muted/40 p-2.5 text-xs text-muted-foreground">{t("ppp.renfort.note")}</p>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setCredsOpen(false)} disabled={credsMutation.isPending}>
                {t("common.cancel")}
              </Button>
              <Button type="submit" disabled={credsMutation.isPending || credsUser.trim() === "" || credsPass === ""}>
                {credsMutation.isPending && <Loader2 className="size-4 animate-spin" aria-hidden />}
                {t("ppp.renfort.save")}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* — Confirmation retrait des credentials — */}
      <AlertDialog open={credsRemoveOpen} onOpenChange={setCredsRemoveOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t("ppp.renfort.removeConfirmTitle")}</AlertDialogTitle>
            <AlertDialogDescription>{t("ppp.renfort.removeConfirmDesc")}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={credsRemoveMutation.isPending}>{t("common.cancel")}</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-white hover:bg-destructive/90"
              disabled={credsRemoveMutation.isPending}
              onClick={(event) => {
                event.preventDefault();
                credsRemoveMutation.mutate();
              }}
            >
              {credsRemoveMutation.isPending && <Loader2 className="size-4 animate-spin" aria-hidden />}
              {t("ppp.renfort.remove")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* — Dialog provisionnement assisté (script .rsc) — */}
      <Dialog open={provOpen} onOpenChange={setProvOpen}>
        <DialogContent className="max-h-[85dvh] overflow-y-auto sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>{t("ppp.provisionTitle")}</DialogTitle>
            <DialogDescription>{t("ppp.provisionDesc")}</DialogDescription>
          </DialogHeader>
          <form
            className="space-y-3 py-1"
            onSubmit={(event) => {
              event.preventDefault();
              if (!provValid || provMutation.isPending) return;
              provMutation.mutate();
            }}
          >
            <div className="grid gap-3 sm:grid-cols-2">
              <div className="space-y-1.5">
                <Label htmlFor="ppp-prov-iface">{t("ppp.provisionInterface")}</Label>
                <Input
                  id="ppp-prov-iface"
                  value={provIface}
                  onChange={(event) => setProvIface(event.target.value)}
                  placeholder={t("ppp.provisionInterfacePlaceholder")}
                  maxLength={32}
                  autoFocus
                />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="ppp-prov-service">{t("ppp.provisionService")}</Label>
                <Input
                  id="ppp-prov-service"
                  value={provService}
                  onChange={(event) => setProvService(event.target.value)}
                  maxLength={32}
                />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="ppp-prov-profile">{t("ppp.provisionProfile")}</Label>
                <Input
                  id="ppp-prov-profile"
                  value={provProfile}
                  onChange={(event) => setProvProfile(event.target.value)}
                  maxLength={64}
                />
              </div>
              <div className="grid grid-cols-2 gap-2">
                <div className="space-y-1.5">
                  <Label htmlFor="ppp-prov-start">{t("ppp.provisionPoolStart")}</Label>
                  <Input
                    id="ppp-prov-start"
                    value={provPoolStart}
                    onChange={(event) => setProvPoolStart(event.target.value)}
                    placeholder="10.10.0.10"
                    inputMode="numeric"
                  />
                </div>
                <div className="space-y-1.5">
                  <Label htmlFor="ppp-prov-end">{t("ppp.provisionPoolEnd")}</Label>
                  <Input
                    id="ppp-prov-end"
                    value={provPoolEnd}
                    onChange={(event) => setProvPoolEnd(event.target.value)}
                    placeholder="10.10.0.254"
                    inputMode="numeric"
                  />
                </div>
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="ppp-prov-local">{t("ppp.provisionLocal")}</Label>
                <Input
                  id="ppp-prov-local"
                  value={provLocal}
                  onChange={(event) => setProvLocal(event.target.value)}
                  placeholder="10.10.0.1"
                  inputMode="numeric"
                />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="ppp-prov-dns">{t("ppp.provisionDns")}</Label>
                <Input
                  id="ppp-prov-dns"
                  value={provDns}
                  onChange={(event) => setProvDns(event.target.value)}
                  placeholder={t("ppp.provisionDnsPlaceholder")}
                />
              </div>
            </div>
            {provResult ? (
              <div className="space-y-3">
                {provResult.warnings.length > 0 && (
                  <Alert className="border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-300">
                    <TriangleAlert className="size-4" aria-hidden />
                    <AlertTitle>{t("ppp.provisionWarnings")}</AlertTitle>
                    <AlertDescription>
                      <ul className="list-disc space-y-1 pl-4">
                        {provResult.warnings.map((w, i) => (
                          <li key={i}>{w}</li>
                        ))}
                      </ul>
                    </AlertDescription>
                  </Alert>
                )}
                <pre className="max-h-96 overflow-y-auto rounded-lg border bg-muted/40 p-3 font-mono text-[11px] leading-relaxed">
                  {provResult.script}
                </pre>
                <div className="flex flex-wrap gap-2">
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    className="min-h-9 gap-1.5"
                    onClick={() => {
                      void copyToClipboard(provResult.script).then((ok) => {
                        if (ok) toast.success(t("ppp.provisionCopied"));
                        else toast.error(t("common.copyImpossible"));
                      });
                    }}
                  >
                    <Copy className="size-3.5" aria-hidden />
                    {t("ppp.provisionCopy")}
                  </Button>
                  <Button type="button" size="sm" variant="outline" className="min-h-9 gap-1.5" onClick={downloadScript}>
                    <Download className="size-3.5" aria-hidden />
                    {t("ppp.provisionDownload")}
                  </Button>
                </div>
                <p className="text-xs text-muted-foreground">{t("ppp.provisionScriptNote")}</p>
              </div>
            ) : (
              <DialogFooter>
                <Button type="button" variant="outline" onClick={() => setProvOpen(false)} disabled={provMutation.isPending}>
                  {t("common.cancel")}
                </Button>
                <Button type="submit" disabled={!provValid || provMutation.isPending}>
                  {provMutation.isPending ? (
                    <>
                      <Loader2 className="size-4 animate-spin" aria-hidden />
                      {t("ppp.provisionPending")}
                    </>
                  ) : (
                    t("ppp.provisionCta")
                  )}
                </Button>
              </DialogFooter>
            )}
          </form>
        </DialogContent>
      </Dialog>
    </div>
  );
}
