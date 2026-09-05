"use client";

// Vue Inscriptions (N°27) — inscriptions publiques par QR code (campus,
// écoles, administration, entreprise) :
// - onglet Demandes : file des demandes soumises via les liens publics
//   (filtres à compteurs, validation avec choix profil / routeur /
//   identifiants, refus motivé, purge manuelle) ;
// - onglet Liens & QR : création de liens d'invitation, QR par carte
//   (copier, PNG 512 px, affiche A4 imprimable), révocation et compteur
//   d'usages.
// Mode de connexion des inscrits publics : « Nom d'utilisateur & Mot de
// passe » (deux codes distincts au choix) — DISTINCT des vouchers, qui
// restent verrouillés « nom d'utilisateur = mot de passe » côté serveur (N°25).

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import QRCode from "qrcode";
import { QRCodeSVG } from "qrcode.react";
import {
  Copy,
  Eye,
  EyeOff,
  ImageDown,
  Inbox,
  KeyRound,
  Loader2,
  Printer,
  QrCode,
  Trash2,
  UserCheck,
  UserPlus,
  UserX,
  Users,
  Ban,
  RotateCcw,
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
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { EmptyState } from "@/components/hotspot/empty-state";
import { LoadingRows } from "@/components/hotspot/loading";
import { PageHeader } from "@/components/hotspot/page-header";
import { StatusBadge } from "@/components/hotspot/status-badge";
import { useCurrency, useSettings } from "@/components/hotspot/parts/sd-currency";
import { copyToClipboard } from "@/components/hotspot/parts/uc-clipboard";
import { ApiError, api } from "@/lib/hotspot/api";
import { useI18n } from "@/lib/hotspot/i18n";
import { formatCurrency, formatDate } from "@/lib/hotspot/format";
import type {
  JoinLinkCreatePayload,
  JoinLinkState,
  JoinLinkUpdatePayload,
  JoinLinkView,
  RegistrationApprovePayload,
  RegistrationApproveResponse,
  RegistrationRequest,
  RegistrationsResponse,
  Profile,
  RouterDevice,
} from "@/lib/hotspot/types";
import { cn } from "@/lib/utils";

/** Mêmes règles que le backend : 3–32 caractères [a-zA-Z0-9._-]. */
const USERNAME_RE = /^[A-Za-z0-9._-]{3,32}$/;

/** Étapes numérotées de l'affiche (clés i18n figées). */
const POSTER_STEP_KEYS = ["join.poster.step1", "join.poster.step2", "join.poster.step3"] as const;

type RequestFilter = "pending" | "approved" | "rejected" | "all";

interface ApproveForm {
  profileId: string;
  routerId: string;
  username: string;
  password: string;
}

interface LinkForm {
  name: string;
  profileId: string;
  routerId: string;
  autoValidate: boolean;
  maxUses: string;
  expiresAt: string; // yyyy-mm-dd (input type=date) — "" = sans expiration
}

const EMPTY_LINK_FORM: LinkForm = { name: "", profileId: "", routerId: "", autoValidate: false, maxUses: "0", expiresAt: "" };

/** Nom de fichier sûr pour le PNG du QR (« inscription-<nom>.png »). */
function safeFileName(name: string): string {
  const cleaned = name
    .normalize("NFKD")
    .replace(/[^\w.-]+/g, "-")
    .replace(/-+/g, "-")
    .replace(/^-|-$/g, "")
    .slice(0, 60);
  return cleaned || "lien";
}

/** Convertit une date locale (yyyy-mm-dd) en RFC3339 fin de journée. */
function endOfDayRFC3339(dateStr: string): string {
  const d = new Date(`${dateStr}T23:59:59`);
  return Number.isNaN(d.getTime()) ? "" : d.toISOString();
}

/** Badge d'état d'une demande (palette existante : ambre / vert / rouge). */
function RequestStatusBadge({ status }: { status: RegistrationRequest["status"] }) {
  const { t } = useI18n();
  if (status === "approved") return <StatusBadge status="active" label={t("join.statusApproved")} />;
  if (status === "rejected") {
    return (
      <Badge variant="outline" className="border-destructive/25 bg-destructive/10 text-destructive">
        {t("join.statusRejected")}
      </Badge>
    );
  }
  return (
    <Badge variant="outline" className="border-amber-500/30 bg-amber-500/10 text-amber-600 dark:text-amber-400">
      {t("join.statusPending")}
    </Badge>
  );
}

/** Badge d'état d'un lien : Actif (vert) / Révoqué (gris) / Expiré (ambre) / Épuisé (ardoise). */
function LinkStateBadge({ state }: { state: JoinLinkState }) {
  const { t } = useI18n();
  const styles: Record<JoinLinkState, string> = {
    active: "border-primary/25 bg-primary/15 text-primary",
    revoked: "bg-muted text-muted-foreground border-border",
    expired: "border-amber-500/30 bg-amber-500/10 text-amber-600 dark:text-amber-400",
    exhausted: "border-slate-400/40 bg-slate-500/10 text-slate-600 dark:text-slate-300",
  };
  const labelKeys: Record<JoinLinkState, string> = {
    active: "join.links.stateActive",
    revoked: "join.links.stateRevoked",
    expired: "join.links.stateExpired",
    exhausted: "join.links.stateExhausted",
  };
  return (
    <Badge variant="outline" className={cn("font-medium", styles[state])}>
      {t(labelKeys[state])}
    </Badge>
  );
}

export default function RegistrationsView() {
  const { t, tf, lang } = useI18n();
  const currency = useCurrency();
  const queryClient = useQueryClient();
  const { data: settings } = useSettings();
  const tenantName = settings?.tenant.name || "MikCloud";

  // Onglet actif : Demandes (défaut) ou Liens & QR.
  const [tab, setTab] = useState<"requests" | "links">("requests");

  // Origine courante — l'URL encodée dans les QR. La vue ne se rend que côté
  // client (garde useMounted d'app-route) : window est toujours défini ici.
  const [origin] = useState(() => (typeof window !== "undefined" ? window.location.origin : ""));

  /* ─── Ressources partagées (profils, routeurs, liens) ─── */

  const { data: profiles } = useQuery({
    queryKey: ["/api/profiles"],
    queryFn: () => api<Profile[]>("/api/profiles"),
  });
  const { data: routers } = useQuery({
    queryKey: ["/api/routers"],
    queryFn: () => api<RouterDevice[]>("/api/routers"),
  });
  const { data: linksData, isLoading: linksLoading } = useQuery({
    queryKey: ["/api/join-links"],
    queryFn: () => api<{ items: JoinLinkView[] }>("/api/join-links"),
  });
  const links = linksData?.items ?? [];

  /* ─── Onglet Demandes ─── */

  const [statusFilter, setStatusFilter] = useState<RequestFilter>("pending");
  const { data: regsData, isLoading: requestsLoading } = useQuery({
    queryKey: ["/api/registrations"],
    queryFn: () => api<RegistrationsResponse>("/api/registrations"),
    placeholderData: (previous) => previous,
  });
  const allRequests = regsData?.items ?? [];
  const counts = regsData?.counts ?? { pending: 0, approved: 0, rejected: 0 };
  const requests = useMemo(
    () => (statusFilter === "all" ? allRequests : allRequests.filter((r) => r.status === statusFilter)),
    [allRequests, statusFilter],
  );

  // Dialogs des demandes
  const [approving, setApproving] = useState<RegistrationRequest | null>(null);
  const [approveForm, setApproveForm] = useState<ApproveForm>({ profileId: "", routerId: "", username: "", password: "" });
  const [showPassword, setShowPassword] = useState(true);
  const [rejecting, setRejecting] = useState<RegistrationRequest | null>(null);
  const [rejectReason, setRejectReason] = useState("");
  const [deletingRequest, setDeletingRequest] = useState<RegistrationRequest | null>(null);

  /** Ouvre le dialog d'approbation — pré-rempli par le lien (profil et routeur
   * pré-attribués) si la demande vient d'un lien configuré. */
  function openApprove(request: RegistrationRequest) {
    approveMutation.reset(); // aucune erreur résiduelle d'un dialog précédent
    const link = links.find((l) => l.id === request.linkId);
    setApproveForm({
      profileId: link?.profileId ?? "",
      routerId: link?.routerId ?? "",
      username: request.desiredUsername,
      password: request.password,
    });
    setShowPassword(true);
    setApproving(request);
  }

  const approveMutation = useMutation({
    mutationFn: (vars: { request: RegistrationRequest; form: ApproveForm }) => {
      const payload: RegistrationApprovePayload = {
        profileId: vars.form.profileId,
        routerId: vars.form.routerId,
        username: vars.form.username.trim(),
        password: vars.form.password || undefined,
      };
      return api<RegistrationApproveResponse>(`/api/registrations/${vars.request.id}/approve`, {
        method: "POST",
        body: payload,
      });
    },
    onSuccess: (res, vars) => {
      toast.success(tf("join.approvedToast", { name: vars.request.fullName }), {
        description: res.queued ? t("join.queuedToast") : undefined,
      });
      setApproving(null);
      void queryClient.invalidateQueries({ queryKey: ["/api/registrations"] });
      void queryClient.invalidateQueries({ queryKey: ["/api/users"] });
      void queryClient.invalidateQueries({ queryKey: ["/api/dashboard"] });
    },
    onError: () => {
      // 409 username_taken : géré inline dans le dialog (suggestion éventuelle).
    },
  });

  const rejectMutation = useMutation({
    mutationFn: (vars: { request: RegistrationRequest; reason: string }) =>
      api<RegistrationRequest>(`/api/registrations/${vars.request.id}/reject`, {
        method: "POST",
        body: { reason: vars.reason },
      }),
    onSuccess: (_res, vars) => {
      toast.success(tf("join.rejectedToast", { name: vars.request.fullName }));
      setRejecting(null);
      setRejectReason("");
      void queryClient.invalidateQueries({ queryKey: ["/api/registrations"] });
    },
    onError: (error: Error) => toast.error(error.message),
  });

  const deleteRequestMutation = useMutation({
    mutationFn: (request: RegistrationRequest) =>
      api<{ ok: boolean }>(`/api/registrations/${request.id}`, { method: "DELETE" }),
    onSuccess: () => {
      toast.success(t("join.deletedToast"));
      setDeletingRequest(null);
      void queryClient.invalidateQueries({ queryKey: ["/api/registrations"] });
    },
    onError: (error: Error) => toast.error(error.message),
  });

  // Aperçu de validité recalculé au fil du profil choisi (validité minutes =
  // validityMin > 0 ? validityMin : validityDays*1440 — logique users-view).
  const approveProfile = profiles?.find((p) => p.id === approveForm.profileId);
  const approveValidityMin = approveProfile
    ? approveProfile.validityMin > 0
      ? approveProfile.validityMin
      : approveProfile.validityDays * 1440
    : 0;
  const approveExpiresAt =
    approveValidityMin > 0 ? new Date(Date.now() + approveValidityMin * 60_000).toISOString() : "";

  // Erreur 409 username_taken — suggestion exposée par l'API si disponible.
  const approveError = approveMutation.error instanceof ApiError ? approveMutation.error : null;
  const usernameSuggestion =
    approveError?.code === "username_taken" && approveError.suggestion ? approveError.suggestion : "";

  const approveUsernameOk = USERNAME_RE.test(approveForm.username.trim());
  const approvePasswordOk = approveForm.password === "" || (approveForm.password.length >= 6 && approveForm.password.length <= 64);
  const approveValid = approveForm.profileId !== "" && approveForm.routerId !== "" && approveUsernameOk && approvePasswordOk;

  const rejectReasonOk = rejectReason.trim().length > 0 && rejectReason.trim().length <= 300;

  /* ─── Onglet Liens & QR ─── */

  const [createOpen, setCreateOpen] = useState(false);
  const [linkForm, setLinkForm] = useState<LinkForm>(EMPTY_LINK_FORM);
  const [deletingLink, setDeletingLink] = useState<JoinLinkView | null>(null);
  const [posterLink, setPosterLink] = useState<JoinLinkView | null>(null);

  function openCreateLink() {
    setLinkForm(EMPTY_LINK_FORM);
    setCreateOpen(true);
  }

  const maxUsesNum = Number.parseInt(linkForm.maxUses, 10);
  const linkNameOk = linkForm.name.trim().length > 0 && linkForm.name.trim().length <= 60;
  const linkMaxUsesOk = Number.isInteger(maxUsesNum) && maxUsesNum >= 0 && maxUsesNum <= 100000;
  // Le kiosque exige profil + routeur pré-attribués (contrainte serveur) —
  // le Switch est désactivé tant que les deux ne sont pas choisis.
  const kioskPossible = linkForm.profileId !== "" && linkForm.routerId !== "";
  const linkFormValid = linkNameOk && linkMaxUsesOk;

  const createLinkMutation = useMutation({
    mutationFn: (form: LinkForm) => {
      const payload: JoinLinkCreatePayload = {
        name: form.name.trim(),
        autoValidate: form.autoValidate && form.profileId !== "" && form.routerId !== "",
        maxUses: Number.parseInt(form.maxUses, 10) || 0,
        ...(form.profileId ? { profileId: form.profileId } : {}),
        ...(form.routerId ? { routerId: form.routerId } : {}),
        ...(form.expiresAt ? { expiresAt: endOfDayRFC3339(form.expiresAt) } : {}),
      };
      return api<JoinLinkView>("/api/join-links", { method: "POST", body: payload });
    },
    onSuccess: (link) => {
      toast.success(tf("join.links.createdToast", { name: link.name }));
      setCreateOpen(false);
      setLinkForm(EMPTY_LINK_FORM);
      void queryClient.invalidateQueries({ queryKey: ["/api/join-links"] });
    },
    onError: (error: Error) => toast.error(error.message),
  });

  const toggleLinkMutation = useMutation({
    mutationFn: (link: JoinLinkView) => {
      const payload: JoinLinkUpdatePayload = { revoked: !link.revoked };
      return api<JoinLinkView>(`/api/join-links/${link.id}`, { method: "PUT", body: payload });
    },
    onSuccess: (updated) => {
      toast.success(
        updated.revoked
          ? tf("join.links.revokedToast", { name: updated.name })
          : tf("join.links.reactivatedToast", { name: updated.name }),
      );
      void queryClient.invalidateQueries({ queryKey: ["/api/join-links"] });
    },
    onError: (error: Error) => toast.error(error.message),
  });

  const deleteLinkMutation = useMutation({
    mutationFn: (link: JoinLinkView) => api<{ ok: boolean }>(`/api/join-links/${link.id}`, { method: "DELETE" }),
    onSuccess: (_res, link) => {
      toast.success(tf("join.links.deletedToast", { name: link.name }));
      setDeletingLink(null);
      void queryClient.invalidateQueries({ queryKey: ["/api/join-links"] });
    },
    onError: (error: Error) => toast.error(error.message),
  });

  /** URL publique d'un lien (origine courante + /join/{token}). */
  function joinUrl(link: JoinLinkView): string {
    return `${origin}/join/${link.token}`;
  }

  async function copyLink(link: JoinLinkView) {
    const ok = await copyToClipboard(joinUrl(link));
    if (ok) toast.success(t("join.links.copiedToast"));
    else toast.error(t("common.copyImpossible"));
  }

  async function downloadPng(link: JoinLinkView) {
    try {
      const dataUrl = await QRCode.toDataURL(joinUrl(link), { width: 512, margin: 2 });
      const a = document.createElement("a");
      a.href = dataUrl;
      a.download = `inscription-${safeFileName(link.name)}.png`;
      document.body.appendChild(a);
      a.click();
      a.remove();
      toast.success(t("join.links.pngToast"));
    } catch {
      toast.error(t("join.links.pngFailed"));
    }
  }

  const filterChips: { value: RequestFilter; label: string; count: number; warn?: boolean }[] = [
    { value: "pending", label: t("join.filterPending"), count: counts.pending, warn: counts.pending > 0 },
    { value: "approved", label: t("join.filterApproved"), count: counts.approved },
    { value: "rejected", label: t("join.filterRejected"), count: counts.rejected },
    { value: "all", label: t("join.filterAll"), count: allRequests.length },
  ];

  /* ─── Rendu d'une demande : actions (partagées table / cartes mobile) ─── */

  function renderRequestActions(request: RegistrationRequest, compact = false) {
    if (request.status !== "pending") {
      return (
        <Button
          variant="ghost"
          size="icon"
          className="size-10 shrink-0 text-muted-foreground hover:text-destructive sm:size-9"
          onClick={() => setDeletingRequest(request)}
          aria-label={t("common.delete")}
          title={t("common.delete")}
        >
          <Trash2 className="size-4" />
        </Button>
      );
    }
    return (
      <div className={cn("flex shrink-0 items-center gap-2", compact && "w-full justify-between sm:w-auto sm:justify-end")}>
        <Button
          size="sm"
          className="h-10 flex-1 sm:h-9 sm:flex-none"
          disabled={approveMutation.isPending}
          onClick={() => openApprove(request)}
        >
          <UserCheck className="size-4" />
          {t("join.approve")}
        </Button>
        <Button
          variant="outline"
          size="sm"
          className="h-10 flex-1 border-destructive/40 text-destructive hover:bg-destructive/10 hover:text-destructive sm:h-9 sm:flex-none"
          disabled={rejectMutation.isPending}
          onClick={() => {
            rejectMutation.reset(); // aucune erreur résiduelle d'un dialog précédent
            setRejectReason("");
            setRejecting(request);
          }}
        >
          <UserX className="size-4" />
          {t("join.reject")}
        </Button>
        <Button
          variant="ghost"
          size="icon"
          className="size-10 text-muted-foreground hover:text-destructive sm:size-9"
          onClick={() => setDeletingRequest(request)}
          aria-label={t("common.delete")}
          title={t("common.delete")}
        >
          <Trash2 className="size-4" />
        </Button>
      </div>
    );
  }

  return (
    <div className="space-y-4 sm:space-y-6">
      <PageHeader
        title={t("join.title")}
        description={t("join.description")}
        actions={
          tab === "links" && (
            <Button className="h-10" onClick={openCreateLink}>
              <UserPlus className="size-4" />
              {t("join.links.new")}
            </Button>
          )
        }
      />

      {/* Onglets Demandes / Liens & QR (pattern vouchers-view) */}
      <Tabs value={tab} onValueChange={(value) => setTab(value as "requests" | "links")}>
        <TabsList>
          <TabsTrigger value="requests" className="gap-1.5">
            <UserCheck className="size-3.5" aria-hidden />
            {t("join.tabRequests")}
          </TabsTrigger>
          <TabsTrigger value="links" className="gap-1.5">
            <QrCode className="size-3.5" aria-hidden />
            {t("join.tabLinks")}
          </TabsTrigger>
        </TabsList>
      </Tabs>

      {/* ─────────────────────────── Onglet Demandes ─────────────────────────── */}

      {tab === "requests" && (
        <div className="space-y-4">
          {/* Chips-filtres avec compteurs (pattern pipeline batch-pipeline) */}
          <div className="flex flex-wrap items-center gap-2" role="group" aria-label={t("join.filtersLabel")}>
            {filterChips.map((chip) => {
              const active = statusFilter === chip.value;
              return (
                <button
                  key={chip.value}
                  type="button"
                  onClick={() => setStatusFilter(chip.value)}
                  aria-pressed={active}
                  className={cn(
                    "inline-flex min-h-10 items-center gap-2 rounded-full border px-3.5 text-sm font-medium transition-colors",
                    "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
                    active
                      ? "border-primary/40 bg-primary/10 text-primary ring-1 ring-primary"
                      : "border-border bg-background text-muted-foreground hover:bg-muted/60 hover:text-foreground",
                  )}
                >
                  {chip.label}
                  <span
                    className={cn(
                      "inline-flex min-w-6 items-center justify-center rounded-full px-1.5 text-xs font-semibold tabular-nums",
                      chip.warn
                        ? "bg-amber-500/15 text-amber-600 dark:text-amber-400"
                        : active
                          ? "bg-primary/15 text-primary"
                          : "bg-muted text-muted-foreground",
                    )}
                  >
                    {chip.count}
                  </span>
                </button>
              );
            })}
          </div>

          {/* Liste des demandes : table md+ / cartes mobile */}
          <Card className="gap-0 py-0">
            {requestsLoading ? (
              <LoadingRows rows={6} />
            ) : requests.length === 0 ? (
              <EmptyState
                icon={statusFilter === "pending" ? Inbox : statusFilter === "rejected" ? UserX : Users}
                title={
                  statusFilter === "all"
                    ? t("join.emptyRequests")
                    : statusFilter === "pending"
                      ? t("join.emptyPending")
                      : t("join.emptyFiltered")
                }
                description={
                  statusFilter === "pending"
                    ? t("join.emptyPendingDesc")
                    : statusFilter === "all"
                      ? t("join.emptyRequestsDesc")
                      : undefined
                }
              />
            ) : (
              <>
                {/* Table compacte (md+) */}
                <div className="hidden overflow-x-auto md:block">
                  <Table>
                    <TableHeader>
                      <TableRow className="hover:bg-transparent">
                        <TableHead className="pl-4 text-muted-foreground sm:pl-6">{t("join.table.fullName")}</TableHead>
                        <TableHead className="text-muted-foreground">{t("join.table.phone")}</TableHead>
                        <TableHead className="text-muted-foreground">{t("join.table.username")}</TableHead>
                        <TableHead className="text-muted-foreground">{t("join.table.link")}</TableHead>
                        <TableHead className="text-muted-foreground">{t("common.date")}</TableHead>
                        <TableHead className="text-muted-foreground">{t("common.status")}</TableHead>
                        <TableHead className="pr-4 text-right text-muted-foreground sm:pr-6">{t("common.actions")}</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {requests.map((request) => (
                        <TableRow key={request.id}>
                          <TableCell className="max-w-52 pl-4 sm:pl-6">
                            <div className="flex flex-col gap-0.5 py-1">
                              <span className="truncate font-medium">{request.fullName}</span>
                              {request.message && (
                                <span className="truncate text-xs italic text-muted-foreground" title={request.message}>
                                  {request.message}
                                </span>
                              )}
                            </div>
                          </TableCell>
                          <TableCell className="tabular-nums">{request.phone}</TableCell>
                          <TableCell className="font-mono text-sm">{request.desiredUsername}</TableCell>
                          <TableCell>
                            <span className="block max-w-44 truncate text-muted-foreground" title={request.linkName}>
                              {request.linkName}
                            </span>
                          </TableCell>
                          <TableCell className="tabular-nums">{formatDate(request.createdAt, lang)}</TableCell>
                          <TableCell>
                            <div className="flex flex-col gap-1">
                              <RequestStatusBadge status={request.status} />
                              {request.status === "rejected" && request.rejectionReason && (
                                <span
                                  className="max-w-48 truncate text-xs text-muted-foreground"
                                  title={request.rejectionReason}
                                >
                                  {tf("join.rejectionReason", { reason: request.rejectionReason })}
                                </span>
                              )}
                            </div>
                          </TableCell>
                          <TableCell className="pr-4 sm:pr-6">
                            <div className="flex justify-end">{renderRequestActions(request)}</div>
                          </TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </div>

                {/* Cartes mobile (même hiérarchie que les cartes lots) */}
                <div className="grid gap-3 p-4 md:hidden">
                  {requests.map((request) => (
                    <div key={request.id} className="flex flex-col rounded-lg border p-4">
                      <div className="flex items-start justify-between gap-2">
                        <span className="min-w-0 truncate font-medium">{request.fullName}</span>
                        <RequestStatusBadge status={request.status} />
                      </div>
                      {request.message && (
                        <p className="mt-1 text-xs italic text-muted-foreground">{request.message}</p>
                      )}
                      <dl className="mt-3 grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-xs">
                        <dt className="text-muted-foreground">{t("join.table.phone")}</dt>
                        <dd className="tabular-nums text-foreground">{request.phone}</dd>
                        <dt className="text-muted-foreground">{t("join.table.username")}</dt>
                        <dd className="truncate text-right font-mono text-foreground">{request.desiredUsername}</dd>
                        <dt className="text-muted-foreground">{t("join.table.link")}</dt>
                        <dd className="truncate text-right text-foreground">{request.linkName}</dd>
                        <dt className="text-muted-foreground">{t("common.date")}</dt>
                        <dd className="tabular-nums text-foreground">{formatDate(request.createdAt, lang)}</dd>
                      </dl>
                      {request.status === "rejected" && request.rejectionReason && (
                        <p className="mt-2 text-xs text-muted-foreground">
                          {tf("join.rejectionReason", { reason: request.rejectionReason })}
                        </p>
                      )}
                      <div className="mt-3 flex flex-wrap items-center gap-2 border-t pt-3">
                        {renderRequestActions(request, true)}
                      </div>
                    </div>
                  ))}
                </div>
              </>
            )}
          </Card>
        </div>
      )}

      {/* ─────────────────────────── Onglet Liens & QR ─────────────────────────── */}

      {tab === "links" &&
        (linksLoading ? (
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-3">
            {Array.from({ length: 3 }).map((_, i) => (
              <div key={i} className="h-80 animate-pulse rounded-xl bg-muted/60" />
            ))}
          </div>
        ) : links.length === 0 ? (
          <Card className="gap-0 py-0">
            <EmptyState
              icon={QrCode}
              title={t("join.links.empty")}
              description={t("join.links.emptyDesc")}
              action={
                <Button onClick={openCreateLink}>
                  <UserPlus className="size-4" />
                  {t("join.links.new")}
                </Button>
              }
            />
          </Card>
        ) : (
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-3">
            {links.map((link) => (
              <Card key={link.id} className="gap-0 py-0">
                <CardContent className="flex flex-col gap-3 p-4">
                  <div className="flex items-start justify-between gap-2">
                    <span className="min-w-0 truncate font-semibold" title={link.name}>
                      {link.name}
                    </span>
                    <LinkStateBadge state={link.state} />
                  </div>

                  <div className="flex items-center gap-4">
                    {/* QR du lien — encodé depuis l'origine courante */}
                    <div
                      role="img"
                      aria-label={tf("join.links.qrAlt", { name: link.name })}
                      className="shrink-0 rounded-lg border bg-white p-1.5"
                    >
                      {origin ? (
                        <QRCodeSVG value={joinUrl(link)} size={120} level="M" className="size-[120px]" />
                      ) : (
                        <div className="size-[120px]" />
                      )}
                    </div>
                    <dl className="grid min-w-0 flex-1 grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-xs">
                      <dt className="text-muted-foreground">{t("common.created")}</dt>
                      <dd className="tabular-nums text-foreground">{formatDate(link.createdAt, lang)}</dd>
                      <dt className="text-muted-foreground">{t("join.links.maxUses")}</dt>
                      <dd className="tabular-nums text-foreground">
                        {link.maxUses > 0
                          ? tf("join.links.usagesOf", { uses: link.uses, max: link.maxUses })
                          : tf("join.links.usagesUnlimited", { uses: link.uses })}
                      </dd>
                      <dt className="text-muted-foreground">{t("join.links.expires")}</dt>
                      <dd className="tabular-nums text-foreground">
                        {link.expiresAt ? tf("join.links.expiresOn", { date: formatDate(link.expiresAt, lang) }) : t("join.links.noExpiry")}
                      </dd>
                    </dl>
                  </div>

                  {(link.profileName || link.routerName || link.autoValidate) && (
                    <div className="flex flex-wrap items-center gap-1.5">
                      {link.autoValidate && (
                        <Badge variant="outline" className="border-chart-2/30 bg-chart-2/10 text-chart-2">
                          {t("join.links.kiosk")}
                        </Badge>
                      )}
                      {link.profileName && <Badge variant="outline">{link.profileName}</Badge>}
                      {link.routerName && <Badge variant="outline">{link.routerName}</Badge>}
                    </div>
                  )}

                  <div className="flex flex-wrap items-center gap-1.5 border-t pt-3">
                    <Button
                      variant="outline"
                      size="sm"
                      className="h-10 flex-1 sm:h-9 sm:flex-none"
                      onClick={() => void copyLink(link)}
                    >
                      <Copy className="size-4" />
                      {t("join.links.copy")}
                    </Button>
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <Button
                          variant="ghost"
                          size="icon"
                          className="size-10 text-muted-foreground hover:text-foreground sm:size-9"
                          onClick={() => void downloadPng(link)}
                          aria-label={t("join.links.png")}
                          title={t("join.links.png")}
                        >
                          <ImageDown className="size-4" />
                        </Button>
                      </TooltipTrigger>
                      <TooltipContent>{t("join.links.png")}</TooltipContent>
                    </Tooltip>
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <Button
                          variant="ghost"
                          size="icon"
                          className="size-10 text-muted-foreground hover:text-foreground sm:size-9"
                          onClick={() => setPosterLink(link)}
                          aria-label={t("join.links.poster")}
                          title={t("join.links.poster")}
                        >
                          <Printer className="size-4" />
                        </Button>
                      </TooltipTrigger>
                      <TooltipContent>{t("join.links.poster")}</TooltipContent>
                    </Tooltip>
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <Button
                          variant="ghost"
                          size="icon"
                          className="size-10 text-muted-foreground hover:text-foreground sm:size-9"
                          disabled={toggleLinkMutation.isPending && toggleLinkMutation.variables?.id === link.id}
                          onClick={() => toggleLinkMutation.mutate(link)}
                          aria-label={link.revoked ? t("join.links.reactivate") : t("join.links.revoke")}
                          title={link.revoked ? t("join.links.reactivate") : t("join.links.revoke")}
                        >
                          {link.revoked ? <RotateCcw className="size-4" /> : <Ban className="size-4" />}
                        </Button>
                      </TooltipTrigger>
                      <TooltipContent>{link.revoked ? t("join.links.reactivate") : t("join.links.revoke")}</TooltipContent>
                    </Tooltip>
                    <Button
                      variant="ghost"
                      size="icon"
                      className="ml-auto size-10 text-muted-foreground hover:text-destructive sm:size-9"
                      onClick={() => setDeletingLink(link)}
                      aria-label={t("common.delete")}
                      title={t("common.delete")}
                    >
                      <Trash2 className="size-4" />
                    </Button>
                  </div>
                </CardContent>
              </Card>
            ))}
          </div>
        ))}

      {/* ─────────────────────────── Dialogs : demandes ─────────────────────────── */}

      {/* Dialog d'approbation — le cœur du flux N°27 */}
      <Dialog open={approving !== null} onOpenChange={(open) => !open && !approveMutation.isPending && setApproving(null)}>
        <DialogContent className="gap-4 sm:max-w-lg">
          <DialogHeader>
            <DialogTitle className="truncate">
              {tf("join.approveTitle", { name: approving?.fullName ?? "" })}
            </DialogTitle>
            <DialogDescription>{t("join.approveDesc")}</DialogDescription>
          </DialogHeader>

          <form
            className="grid gap-4"
            onSubmit={(event) => {
              event.preventDefault();
              if (approveValid && !approveMutation.isPending && approving) {
                approveMutation.mutate({ request: approving, form: approveForm });
              }
            }}
          >
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="grid gap-2">
                <Label htmlFor="approve-profile">{t("common.profile")}</Label>
                <Select
                  value={approveForm.profileId}
                  onValueChange={(value) => setApproveForm((f) => ({ ...f, profileId: value }))}
                  disabled={approveMutation.isPending}
                >
                  <SelectTrigger id="approve-profile" className="h-10 w-full">
                    <SelectValue placeholder={t("common.selectProfile")} />
                  </SelectTrigger>
                  <SelectContent>
                    {profiles?.map((profile) => (
                      <SelectItem key={profile.id} value={profile.id}>
                        {profile.name} — {formatCurrency(profile.price, currency, lang)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="grid gap-2">
                <Label htmlFor="approve-router">{t("common.router")}</Label>
                <Select
                  value={approveForm.routerId}
                  onValueChange={(value) => setApproveForm((f) => ({ ...f, routerId: value }))}
                  disabled={approveMutation.isPending}
                >
                  <SelectTrigger id="approve-router" className="h-10 w-full">
                    <SelectValue placeholder={t("common.selectRouter")} />
                  </SelectTrigger>
                  <SelectContent>
                    {routers?.map((router) => (
                      <SelectItem key={router.id} value={router.id}>
                        {router.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </div>

            <div className="grid gap-4 sm:grid-cols-2">
              <div className="grid gap-2">
                <Label htmlFor="approve-username">{t("join.approveUsername")}</Label>
                <Input
                  id="approve-username"
                  className="h-10 font-mono"
                  autoComplete="off"
                  value={approveForm.username}
                  onChange={(event) => setApproveForm((f) => ({ ...f, username: event.target.value.replace(/\s/g, "") }))}
                  disabled={approveMutation.isPending}
                  aria-invalid={approveForm.username !== "" && !approveUsernameOk}
                />
                <p className="text-xs text-muted-foreground">{t("join.approveUsernameHint")}</p>
              </div>
              <div className="grid gap-2">
                <Label htmlFor="approve-password">{t("common.password")}</Label>
                {/* Mot de passe lisible par défaut : le gérant doit pouvoir le
                    dicter au client — l'œil permet de le masquer à l'écran. */}
                <div className="relative">
                  <Input
                    id="approve-password"
                    className="h-10 pr-11 font-mono"
                    type={showPassword ? "text" : "password"}
                    autoComplete="new-password"
                    value={approveForm.password}
                    onChange={(event) => setApproveForm((f) => ({ ...f, password: event.target.value.replace(/\s/g, "") }))}
                    disabled={approveMutation.isPending}
                    aria-invalid={!approvePasswordOk}
                  />
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    className="absolute top-1/2 right-1 size-9 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                    onClick={() => setShowPassword((v) => !v)}
                    aria-label={showPassword ? t("join.hidePassword") : t("join.showPassword")}
                    title={showPassword ? t("join.hidePassword") : t("join.showPassword")}
                  >
                    {showPassword ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
                  </Button>
                </div>
                <p className="text-xs text-muted-foreground">{t("join.approvePasswordHint")}</p>
              </div>
            </div>

            {approveProfile && approveExpiresAt && (
              <p className="rounded-lg border bg-muted/40 px-3 py-2 text-sm">
                {tf("join.validityPreview", { date: formatDate(approveExpiresAt, lang) })}
              </p>
            )}

            {/* Mention du mode de connexion — distinct des vouchers (N°25) */}
            <div className="rounded-lg border border-primary/25 bg-primary/5 px-3 py-2.5">
              <p className="flex items-center gap-2 text-sm font-semibold text-primary">
                <KeyRound className="size-4 shrink-0" aria-hidden />
                {t("join.modeLabel")}
              </p>
              <p className="mt-1 text-xs text-muted-foreground">{t("join.modeHint")}</p>
            </div>

            {approveError && (
              <div
                role="alert"
                className="rounded-lg border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-destructive"
              >
                <p>{approveError.message}</p>
                {usernameSuggestion && (
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    className="mt-2 h-9 border-destructive/40 text-destructive hover:bg-destructive/10 hover:text-destructive"
                    onClick={() => {
                      setApproveForm((f) => ({ ...f, username: usernameSuggestion }));
                      approveMutation.reset();
                    }}
                  >
                    {tf("join.useSuggestion", { suggestion: usernameSuggestion })}
                  </Button>
                )}
              </div>
            )}

            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => setApproving(null)}
                disabled={approveMutation.isPending}
              >
                {t("common.cancel")}
              </Button>
              <Button type="submit" disabled={!approveValid || approveMutation.isPending}>
                {approveMutation.isPending ? (
                  <Loader2 className="size-4 animate-spin" />
                ) : (
                  <UserCheck className="size-4" />
                )}
                {t("join.approveSubmit")}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* Dialog de refus — motif requis (1–300), destructif */}
      <Dialog open={rejecting !== null} onOpenChange={(open) => !open && !rejectMutation.isPending && setRejecting(null)}>
        <DialogContent className="gap-4 sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{tf("join.rejectTitle", { name: rejecting?.fullName ?? "" })}</DialogTitle>
            <DialogDescription>{t("join.rejectDesc")}</DialogDescription>
          </DialogHeader>
          <form
            className="grid gap-4"
            onSubmit={(event) => {
              event.preventDefault();
              if (rejectReasonOk && !rejectMutation.isPending && rejecting) {
                rejectMutation.mutate({ request: rejecting, reason: rejectReason.trim() });
              }
            }}
          >
            <div className="grid gap-2">
              <Label htmlFor="reject-reason">{t("join.rejectReason")}</Label>
              <Textarea
                id="reject-reason"
                rows={4}
                maxLength={300}
                placeholder={t("join.rejectReasonPlaceholder")}
                value={rejectReason}
                onChange={(event) => setRejectReason(event.target.value)}
                disabled={rejectMutation.isPending}
              />
              <p className="text-right text-xs tabular-nums text-muted-foreground">{rejectReason.length}/300</p>
            </div>
            {rejectMutation.isError && (
              <p className="text-sm text-destructive" role="alert">
                {rejectMutation.error.message}
              </p>
            )}
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setRejecting(null)} disabled={rejectMutation.isPending}>
                {t("common.cancel")}
              </Button>
              <Button
                type="submit"
                className="bg-destructive text-white hover:bg-destructive/90"
                disabled={!rejectReasonOk || rejectMutation.isPending}
              >
                {rejectMutation.isPending ? <Loader2 className="size-4 animate-spin" /> : <UserX className="size-4" />}
                {t("join.rejectSubmit")}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* Confirmation suppression d'une demande */}
      <AlertDialog open={deletingRequest !== null} onOpenChange={(open) => !open && setDeletingRequest(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{tf("join.deleteRegTitle", { name: deletingRequest?.fullName ?? "" })}</AlertDialogTitle>
            <AlertDialogDescription>{t("join.deleteRegDesc")}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleteRequestMutation.isPending}>{t("common.cancel")}</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-white hover:bg-destructive/90"
              disabled={deleteRequestMutation.isPending}
              onClick={(event) => {
                event.preventDefault();
                if (deletingRequest) deleteRequestMutation.mutate(deletingRequest);
              }}
            >
              {deleteRequestMutation.isPending && <Loader2 className="size-4 animate-spin" />}
              {t("common.delete")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* ─────────────────────────── Dialogs : liens ─────────────────────────── */}

      {/* Dialog de création d'un lien */}
      <Dialog open={createOpen} onOpenChange={(open) => !open && !createLinkMutation.isPending && setCreateOpen(open)}>
        <DialogContent className="gap-4 sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>{t("join.links.dialogTitle")}</DialogTitle>
            <DialogDescription>{t("join.links.dialogDesc")}</DialogDescription>
          </DialogHeader>
          <form
            className="grid gap-4"
            onSubmit={(event) => {
              event.preventDefault();
              if (linkFormValid && !createLinkMutation.isPending) {
                createLinkMutation.mutate(linkForm);
              }
            }}
          >
            <div className="grid gap-2">
              <Label htmlFor="link-name">{t("join.links.name")}</Label>
              <Input
                id="link-name"
                className="h-10"
                maxLength={60}
                placeholder={t("join.links.namePlaceholder")}
                value={linkForm.name}
                onChange={(event) => setLinkForm((f) => ({ ...f, name: event.target.value }))}
                disabled={createLinkMutation.isPending}
              />
            </div>

            <div className="grid gap-4 sm:grid-cols-2">
              <div className="grid gap-2">
                <Label htmlFor="link-profile">{t("join.links.profileOptional")}</Label>
                <Select
                  value={linkForm.profileId}
                  onValueChange={(value) => setLinkForm((f) => ({ ...f, profileId: value }))}
                  disabled={createLinkMutation.isPending}
                >
                  <SelectTrigger id="link-profile" className="h-10 w-full">
                    <SelectValue placeholder={t("join.links.profilePlaceholder")} />
                  </SelectTrigger>
                  <SelectContent>
                    {profiles?.map((profile) => (
                      <SelectItem key={profile.id} value={profile.id}>
                        {profile.name} — {formatCurrency(profile.price, currency, lang)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="grid gap-2">
                <Label htmlFor="link-router">{t("join.links.routerOptional")}</Label>
                <Select
                  value={linkForm.routerId}
                  onValueChange={(value) => setLinkForm((f) => ({ ...f, routerId: value }))}
                  disabled={createLinkMutation.isPending}
                >
                  <SelectTrigger id="link-router" className="h-10 w-full">
                    <SelectValue placeholder={t("common.selectRouter")} />
                  </SelectTrigger>
                  <SelectContent>
                    {routers?.map((router) => (
                      <SelectItem key={router.id} value={router.id}>
                        {router.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </div>

            {/* Kiosque — Switch verrouillé tant que profil + routeur manquent */}
            <div className="rounded-lg border p-3">
              <div className="flex items-center justify-between gap-3">
                <Label htmlFor="link-kiosk" className="cursor-pointer font-normal">
                  {t("join.links.autoValidate")}
                </Label>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <span className="inline-flex">
                      <Switch
                        id="link-kiosk"
                        checked={linkForm.autoValidate}
                        disabled={!kioskPossible || createLinkMutation.isPending}
                        onCheckedChange={(checked) => setLinkForm((f) => ({ ...f, autoValidate: checked }))}
                      />
                    </span>
                  </TooltipTrigger>
                  <TooltipContent className="max-w-64 whitespace-normal text-center">
                    {t("join.links.autoValidateTooltip")}
                  </TooltipContent>
                </Tooltip>
              </div>
              <p className="mt-1.5 text-xs text-muted-foreground">
                {kioskPossible ? t("join.links.autoValidateTooltip") : t("join.links.autoValidateLocked")}
              </p>
            </div>

            <div className="grid gap-4 sm:grid-cols-2">
              <div className="grid gap-2">
                <Label htmlFor="link-max-uses">{t("join.links.maxUses")}</Label>
                <Input
                  id="link-max-uses"
                  className="h-10 tabular-nums"
                  type="number"
                  min={0}
                  max={100000}
                  step={1}
                  value={linkForm.maxUses}
                  onChange={(event) => setLinkForm((f) => ({ ...f, maxUses: event.target.value }))}
                  disabled={createLinkMutation.isPending}
                  aria-invalid={!linkMaxUsesOk}
                />
                <p className="text-xs text-muted-foreground">{t("join.links.maxUsesHint")}</p>
              </div>
              <div className="grid gap-2">
                <Label htmlFor="link-expires">{t("join.links.expires")}</Label>
                <Input
                  id="link-expires"
                  className="h-10"
                  type="date"
                  min={new Date().toISOString().slice(0, 10)}
                  value={linkForm.expiresAt}
                  onChange={(event) => setLinkForm((f) => ({ ...f, expiresAt: event.target.value }))}
                  disabled={createLinkMutation.isPending}
                />
                <p className="text-xs text-muted-foreground">{t("join.links.expiresHint")}</p>
              </div>
            </div>

            {createLinkMutation.isError && (
              <p className="text-sm text-destructive" role="alert">
                {createLinkMutation.error.message}
              </p>
            )}

            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => setCreateOpen(false)}
                disabled={createLinkMutation.isPending}
              >
                {t("common.cancel")}
              </Button>
              <Button type="submit" disabled={!linkFormValid || createLinkMutation.isPending}>
                {createLinkMutation.isPending ? (
                  <Loader2 className="size-4 animate-spin" />
                ) : (
                  <QrCode className="size-4" />
                )}
                {t("join.links.createSubmit")}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* Confirmation suppression d'un lien */}
      <AlertDialog open={deletingLink !== null} onOpenChange={(open) => !open && setDeletingLink(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{tf("join.links.deleteTitle", { name: deletingLink?.name ?? "" })}</AlertDialogTitle>
            <AlertDialogDescription>{t("join.links.deleteDesc")}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleteLinkMutation.isPending}>{t("common.cancel")}</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-white hover:bg-destructive/90"
              disabled={deleteLinkMutation.isPending}
              onClick={(event) => {
                event.preventDefault();
                if (deletingLink) deleteLinkMutation.mutate(deletingLink);
              }}
            >
              {deleteLinkMutation.isPending && <Loader2 className="size-4 animate-spin" />}
              {t("common.delete")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* Affiche A4 imprimable (window.print + .print-area, pattern uc-print-dialog) */}
      <Dialog open={posterLink !== null} onOpenChange={(open) => !open && setPosterLink(null)}>
        <DialogContent className="gap-4 sm:max-w-2xl">
          <div className="no-print flex flex-wrap items-center justify-between gap-3">
            <div className="min-w-0">
              <DialogTitle className="truncate">{t("join.links.poster")}</DialogTitle>
              <DialogDescription className="truncate">{posterLink?.name}</DialogDescription>
            </div>
            <div className="flex shrink-0 items-center gap-2">
              <Button variant="outline" onClick={() => setPosterLink(null)}>
                {t("common.close")}
              </Button>
              <Button onClick={() => window.print()}>
                <Printer className="size-4" />
                {t("print.action")}
              </Button>
            </div>
          </div>

          <div className="max-h-[65vh] overflow-y-auto print:max-h-none print:overflow-visible">
            {posterLink && (
              <div className="print-area mx-auto flex w-full max-w-md flex-col items-center gap-6 rounded-lg bg-white p-8 text-black sm:p-10">
                {/* En-tête : organisation + titre */}
                <div className="flex flex-col items-center gap-2 text-center">
                  <p className="text-2xl font-bold tracking-tight">{tenantName}</p>
                  <div className="h-px w-16 bg-black/20" aria-hidden />
                  <p className="text-4xl font-extrabold tracking-tight">{t("join.poster.title")}</p>
                </div>

                {/* Grand QR centré (≥ 320 px) */}
                <div
                  role="img"
                  aria-label={tf("join.links.qrAlt", { name: posterLink.name })}
                  className="p-2"
                >
                  {origin ? (
                    <QRCodeSVG value={joinUrl(posterLink)} size={320} level="M" className="size-80" />
                  ) : (
                    <div className="size-80" />
                  )}
                </div>

                <p className="max-w-full text-center text-sm break-all text-black/70">
                  {t("join.poster.orUrl")}{" "}
                  <span className="font-mono font-semibold text-black">{joinUrl(posterLink)}</span>
                </p>

                {/* 3 étapes numérotées */}
                <ol className="w-full max-w-sm space-y-3">
                  {POSTER_STEP_KEYS.map((key, index) => (
                    <li key={key} className="flex items-center gap-3">
                      <span className="flex size-7 shrink-0 items-center justify-center rounded-full bg-black text-sm font-bold text-white">
                        {index + 1}
                      </span>
                      <span className="text-sm leading-snug">{t(key)}</span>
                    </li>
                  ))}
                </ol>

                {/* Mode de connexion — distinct des tickets */}
                <p className="flex w-full max-w-sm items-center justify-center gap-2 rounded-lg border border-black/15 bg-black/5 px-3 py-2.5 text-center text-sm font-semibold">
                  <KeyRound className="size-4 shrink-0" aria-hidden />
                  {t("join.poster.modeNote")}
                </p>
              </div>
            )}
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}
