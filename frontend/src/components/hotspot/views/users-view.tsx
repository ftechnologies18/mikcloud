"use client";

// Vue Utilisateurs réguliers (kind=regular) — comptes hotspot personnalisés :
// filtres (recherche, profil, statut), table paginée, création / édition / activation / suppression.
// Sélection multiple avec actions groupées (activations / prolongation / suppression…)
// et choix du mode d'affichage : liste (table) ou cartes.

import { useEffect, useRef, useState } from "react";
import { usePathname, useRouter } from "next/navigation";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  CalendarPlus,
  ChevronLeft,
  ChevronRight,
  ClipboardCopy,
  Download,
  Eraser,
  LayoutGrid,
  List,
  Loader2,
  MoreHorizontal,
  Pencil,
  Power,
  PowerOff,
  RefreshCcw,
  RotateCcw,
  Search,
  ShieldQuestion,
  Trash2,
  UserPlus,
  Users,
  X,
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Textarea } from "@/components/ui/textarea";
import { EmptyState } from "@/components/hotspot/empty-state";
import { LoadingRows } from "@/components/hotspot/loading";
import { PageHeader } from "@/components/hotspot/page-header";
import { StatusBadge } from "@/components/hotspot/status-badge";
import { useCurrency } from "@/components/hotspot/parts/sd-currency";
import { copyToClipboard } from "@/components/hotspot/parts/uc-clipboard";
import { PasswordCell } from "@/components/hotspot/parts/uc-password-cell";
import { api, apiDownload } from "@/lib/hotspot/api";
import { useI18n } from "@/lib/hotspot/i18n";
import { detailFromPath, viewToPath } from "@/lib/hotspot/view-path";
import { formatBytes, formatCurrency, formatDate } from "@/lib/hotspot/format";
import type { HotspotUser, PagedUsers, Profile, RouterDevice } from "@/lib/hotspot/types";
import { cn } from "@/lib/utils";

const PAGE_SIZE = 10;

const STATUS_OPTIONS = [
  { value: "all", labelKey: "common.allStatuses" },
  { value: "active", labelKey: "common.statusActive" },
  { value: "online", labelKey: "common.statusOnline" },
  { value: "disabled", labelKey: "common.statusDisabled" },
  { value: "used", labelKey: "common.statusUsed" },
  { value: "expired", labelKey: "common.statusExpired" },
];

interface UserForm {
  username: string;
  password: string;
  profileId: string;
  routerId: string;
  comment: string;
}

const EMPTY_FORM: UserForm = { username: "", password: "", profileId: "", routerId: "", comment: "" };

export default function UsersView() {
  const { t, tf, lang } = useI18n();
  const currency = useCurrency();
  const queryClient = useQueryClient();

  // Filtres (recherche avec debounce ~400 ms)
  const [searchInput, setSearchInput] = useState("");
  const [search, setSearch] = useState("");
  const [profileFilter, setProfileFilter] = useState("all");
  const [statusFilter, setStatusFilter] = useState("all");
  const [page, setPage] = useState(1);

  useEffect(() => {
    const timer = setTimeout(() => {
      setSearch(searchInput.trim());
      setPage(1);
    }, 400);
    return () => clearTimeout(timer);
  }, [searchInput]);

  // Révélation des mots de passe (par ligne)
  const [revealed, setRevealed] = useState<Set<string>>(new Set());

  // Dialogues
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editing, setEditing] = useState<HotspotUser | null>(null);
  const [form, setForm] = useState<UserForm>(EMPTY_FORM);
  const [deleting, setDeleting] = useState<HotspotUser | null>(null);

  // Actions globales P0 (F4/F5) : export CSV, nettoyage des expirés, prolongation
  const [exporting, setExporting] = useState(false);
  const [cleanupOpen, setCleanupOpen] = useState(false);
  const [extending, setExtending] = useState<HotspotUser | null>(null);
  const [extendDays, setExtendDays] = useState("7");

  // Mode d'affichage (liste ou cartes) — mémorisé dans le navigateur.
  const [viewMode, setViewMode] = useState<"list" | "cards">("list");

  // Sélection multiple (ids) — barre d'actions groupées.
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [bulkDeleteOpen, setBulkDeleteOpen] = useState(false);
  const [bulkExtendOpen, setBulkExtendOpen] = useState(false);

  const { data: profiles } = useQuery({
    queryKey: ["/api/profiles"],
    queryFn: () => api<Profile[]>("/api/profiles"),
  });

  const { data: routers } = useQuery({
    queryKey: ["/api/routers"],
    queryFn: () => api<RouterDevice[]>("/api/routers"),
  });

  const statusParam = statusFilter === "all" ? undefined : statusFilter;
  const profileParam = profileFilter === "all" ? undefined : profileFilter;

  // La borne réelle vient de pagedData : safePage évite une page hors bornes après suppression/filtrage.
  const { data: pagedData, isLoading, isFetching } = useQuery({
    queryKey: ["/api/users", { kind: "regular", search, status: statusParam, profileId: profileParam, page }],
    queryFn: () =>
      api<PagedUsers>("/api/users", {
        params: { kind: "regular", search, status: statusParam, profileId: profileParam, page, pageSize: PAGE_SIZE },
      }),
    placeholderData: (previous) => previous,
  });

  const users = pagedData?.data ?? [];
  const totalCount = pagedData?.total ?? 0;
  const maxPage = Math.max(1, Math.ceil(totalCount / PAGE_SIZE));
  const safePage = Math.min(page, maxPage);
  const rangeStart = totalCount === 0 ? 0 : (safePage - 1) * PAGE_SIZE + 1;
  const rangeEnd = Math.min(safePage * PAGE_SIZE, totalCount);

  // Préférence d'affichage restaurée au montage (localStorage).
  useEffect(() => {
    try {
      const saved = window.localStorage.getItem("mikcloud.usersView");
      if (saved === "cards" || saved === "list") setViewMode(saved);
    } catch {
      // stockage indisponible — liste par défaut
    }
  }, []);

  // La sélection est vidée au changement de page ou de filtres.
  useEffect(() => {
    setSelected(new Set());
  }, [safePage, search, profileFilter, statusFilter]);

  function invalidateUsers() {
    void queryClient.invalidateQueries({ queryKey: ["/api/users"] });
    void queryClient.invalidateQueries({ queryKey: ["/api/dashboard"] });
  }

  // Phase D — deep-link /app/users/<id> : ouvre directement le dialog
  // d'édition de l'utilisateur (s'il est dans la page chargée — un lien
  // pointant une autre page/filtre retombe proprement sur la liste). Le
  // segment est consommé LOCALEMENT : ni le store ni app-route ne changent
  // (fix 192ad9f préservé). suppressDetail neutralise la ré-ouverture
  // pendant une fermeture (le replace de l'URL est asynchrone).
  const router = useRouter();
  const pathname = usePathname();
  const detailUserId = detailFromPath(pathname, "users");
  const prevDetail = useRef<string | null>(null);
  const suppressDetail = useRef(false);

  useEffect(() => {
    if (detailUserId) {
      if (suppressDetail.current || editing?.id === detailUserId) {
        prevDetail.current = detailUserId;
        return;
      }
      const user = users.find((u) => u.id === detailUserId);
      if (user) {
        setEditing(user);
        setForm({
          username: user.username,
          password: "",
          profileId: user.profileId,
          routerId: user.routerId,
          comment: user.comment || "",
        });
        setDialogOpen(true);
      } else if (!isLoading) {
        // id absent des données chargées → retour à la liste (pas de dialog fantôme)
        router.replace(viewToPath("users"), { scroll: false });
      }
    } else if (prevDetail.current && editing?.id === prevDetail.current) {
      // sortie du détail (Retour du navigateur) → refermer le dialog
      setDialogOpen(false);
      setEditing(null);
    }
    prevDetail.current = detailUserId;
  }, [detailUserId, users, isLoading, editing, router]);

  function toggleReveal(id: string) {
    setRevealed((previous) => {
      const next = new Set(previous);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  function changeView(mode: "list" | "cards") {
    setViewMode(mode);
    try {
      window.localStorage.setItem("mikcloud.usersView", mode);
    } catch {
      // stockage indisponible — préférence non persistée
    }
  }

  function clearSelection() {
    setSelected(new Set());
  }

  function toggleAllPage() {
    setSelected((previous) => {
      const next = new Set(previous);
      if (users.every((u) => next.has(u.id))) {
        users.forEach((u) => next.delete(u.id));
      } else {
        users.forEach((u) => next.add(u.id));
      }
      return next;
    });
  }

  function toggleOne(id: string) {
    setSelected((previous) => {
      const next = new Set(previous);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  function openCreate() {
    setEditing(null);
    setForm(EMPTY_FORM);
    setDialogOpen(true);
  }

  function openEdit(user: HotspotUser) {
    setEditing(user);
    setForm({
      username: user.username,
      password: "",
      profileId: user.profileId,
      routerId: user.routerId,
      comment: user.comment || "",
    });
    setDialogOpen(true);
    // Phase D — le dialog d'édition devient adressable : /app/users/<id>
    // (push → le Retour du navigateur referme le dialog). La vue reste
    // pilotée par le store : ce segment est consommé localement.
    router.push(viewToPath("users", user.id), { scroll: false });
  }

  function closeDialog(open: boolean) {
    setDialogOpen(open);
    if (!open) {
      if (editing) {
        // Fermeture d'un dialog d'édition (potentiellement ouvert par
        // deep-link) : retirer le segment — replace, aucune entrée
        // d'historique parasite (même discipline que le fix 192ad9f).
        suppressDetail.current = true;
        router.replace(viewToPath("users"), { scroll: false });
      }
      setEditing(null);
    }
  }

  async function copyCredentials(user: HotspotUser) {
    const ok = await copyToClipboard(`${user.username} / ${user.password}`);
    if (ok) toast.success(t("users.credentialsCopied"));
    else toast.error(t("common.copyImpossible"));
  }

  const saveMutation = useMutation({
    mutationFn: async (payload: { id: string | null; form: UserForm }) => {
      if (payload.id) {
        // PUT : seuls username / password / profileId / comment sont modifiables côté API.
        const body: Record<string, unknown> = {
          profileId: payload.form.profileId,
          comment: payload.form.comment.trim(),
        };
        if (payload.form.username.trim()) body.username = payload.form.username.trim();
        if (payload.form.password) body.password = payload.form.password;
        return api<HotspotUser>(`/api/users/${payload.id}`, { method: "PUT", body });
      }
      return api<HotspotUser>("/api/users", {
        method: "POST",
        body: {
          kind: "regular",
          username: payload.form.username.trim() || undefined,
          password: payload.form.password || undefined,
          profileId: payload.form.profileId,
          routerId: payload.form.routerId,
          comment: payload.form.comment.trim() || undefined,
        },
      });
    },
    onSuccess: (user, variables) => {
      toast.success(
        variables.id ? tf("users.updatedToast", { name: user.username }) : tf("users.createdToast", { name: user.username }),
      );
      closeDialog(false);
      invalidateUsers();
    },
    onError: (error: Error) => toast.error(error.message),
  });

  const toggleMutation = useMutation({
    // Le statut listé est RÉSOLU (une expiration peut masquer « disabled ») :
    // le toggle s'appuie sur le miroir du statut stocké (user.disabled).
    mutationFn: (user: HotspotUser) =>
      api<HotspotUser>(
        `/api/users/${user.id}/${user.disabled === true || user.status === "disabled" ? "enable" : "disable"}`,
        {
          method: "POST",
        },
      ),
    // Phase D (UI optimiste) — bascule immédiate dans TOUTES les pages en
    // cache (setQueriesData couvre les clés paginées/filtrées) ; rollback si
    // l'API refuse ; la réponse serveur est écrite avant l'invalidation.
    onMutate: async (user) => {
      await queryClient.cancelQueries({ queryKey: ["/api/users"] });
      const snapshots = queryClient.getQueriesData<PagedUsers>({ queryKey: ["/api/users"] });
      const disabling = user.disabled === true || user.status === "disabled";
      queryClient.setQueriesData<PagedUsers>({ queryKey: ["/api/users"] }, (old) =>
        old
          ? {
              ...old,
              data: old.data.map((u) =>
                u.id === user.id
                  ? {
                      ...u,
                      disabled: disabling,
                      status: disabling ? "disabled" : u.status === "disabled" ? "active" : u.status,
                    }
                  : u,
              ),
            }
          : old,
      );
      return snapshots;
    },
    onSuccess: (user) => {
      toast.success(
        user.status === "disabled"
          ? tf("users.deactivatedToast", { name: user.username })
          : tf("users.activatedToast", { name: user.username }),
      );
      queryClient.setQueriesData<PagedUsers>({ queryKey: ["/api/users"] }, (old) =>
        old ? { ...old, data: old.data.map((u) => (u.id === user.id ? user : u)) } : old,
      );
      invalidateUsers();
    },
    onError: (error: Error, _user, ctx) => {
      if (ctx) for (const [key, value] of ctx) queryClient.setQueryData(key, value);
      toast.error(error.message);
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (user: HotspotUser) => api<{ ok: boolean }>(`/api/users/${user.id}`, { method: "DELETE" }),
    onSuccess: (_res, user) => {
      toast.success(tf("users.deletedToast", { name: user.username }));
      setDeleting(null);
      invalidateUsers();
    },
    onError: (error: Error) => toast.error(error.message),
  });

  // Nettoyage cloud des utilisateurs expirés (F5) — POST /api/users/cleanup.
  const cleanupMutation = useMutation({
    mutationFn: () =>
      api<{ ok: boolean; removed: number }>("/api/users/cleanup", {
        method: "POST",
        body: { mode: "expired" },
      }),
    onSuccess: (res) => {
      toast.success(
        res.removed > 0
          ? tf("users.cleanupDone", { n: res.removed, p: res.removed > 1 ? "s" : "" })
          : t("users.cleanupNone"),
      );
      setCleanupOpen(false);
      invalidateUsers();
    },
    onError: (error: Error) => toast.error(error.message),
  });

  // Prolongation d'expiration (F4) — POST /api/users/{id}/extend.
  const extendMutation = useMutation({
    mutationFn: (payload: { user: HotspotUser; days: number }) =>
      api<HotspotUser>(`/api/users/${payload.user.id}/extend`, {
        method: "POST",
        body: { days: payload.days },
      }),
    // Phase D (UI optimiste) — même règle que le backend : nouvelle
    // expiration = max(maintenant, expiration actuelle) + days, et un
    // utilisateur « expiré » repasse « actif ». La réponse serveur corrige
    // toute divergence à l'invalidation ; rollback si l'API refuse (ex. :
    // ticket jamais connecté → 400).
    onMutate: async ({ user, days }) => {
      await queryClient.cancelQueries({ queryKey: ["/api/users"] });
      const snapshots = queryClient.getQueriesData<PagedUsers>({ queryKey: ["/api/users"] });
      const currentMs = user.expiresAt ? new Date(user.expiresAt).getTime() : NaN;
      const baseMs = !Number.isNaN(currentMs) && currentMs > Date.now() ? currentMs : Date.now();
      const expiresAt = new Date(baseMs + days * 86_400_000).toISOString();
      queryClient.setQueriesData<PagedUsers>({ queryKey: ["/api/users"] }, (old) =>
        old
          ? {
              ...old,
              data: old.data.map((u) =>
                u.id === user.id
                  ? { ...u, expiresAt, status: u.status === "expired" ? "active" : u.status }
                  : u,
              ),
            }
          : old,
      );
      return snapshots;
    },
    onSuccess: (user, variables) => {
      toast.success(tf("users.extendedToast", { name: user.username, n: variables.days }), {
        description: user.expiresAt ? tf("users.newExpiry", { date: formatDate(user.expiresAt, lang) }) : undefined,
      });
      queryClient.setQueriesData<PagedUsers>({ queryKey: ["/api/users"] }, (old) =>
        old ? { ...old, data: old.data.map((u) => (u.id === user.id ? user : u)) } : old,
      );
      setExtending(null);
      invalidateUsers();
    },
    onError: (error: Error, _payload, ctx) => {
      if (ctx) for (const [key, value] of ctx) queryClient.setQueryData(key, value);
      toast.error(error.message);
    },
  });

  // Remise à zéro des compteurs (F4) — POST /api/users/{id}/reset-stats.
  const resetStatsMutation = useMutation({
    mutationFn: (user: HotspotUser) =>
      api<{ ok: boolean }>(`/api/users/${user.id}/reset-stats`, { method: "POST" }),
    onSuccess: (_res, user) => {
      toast.success(tf("users.resetStatsToast", { name: user.username }));
      invalidateUsers();
    },
    onError: (error: Error) => toast.error(error.message),
  });

  // Actions groupées — POST /api/users/bulk {ids, action, days}.
  const bulkMutation = useMutation({
    mutationFn: (vars: {
      action: "enable" | "disable" | "delete" | "extend" | "reset-stats";
      ids: string[];
      days?: number;
    }) =>
      api<{ ok: boolean; processed: number; failed: number }>("/api/users/bulk", {
        method: "POST",
        body: { ids: vars.ids, action: vars.action, days: vars.days },
      }),
    onSuccess: (res) => {
      if (res.failed > 0) {
        toast.warning(tf("users.bulkPartial", { n: res.processed, m: res.failed, s: res.processed > 1 ? "s" : "" }));
      } else {
        toast.success(tf("users.bulkDone", { n: res.processed, s: res.processed > 1 ? "s" : "" }));
      }
      setBulkDeleteOpen(false);
      setBulkExtendOpen(false);
      setSelected(new Set());
      invalidateUsers();
    },
    onError: (error: Error) => toast.error(error.message),
  });

  // N (rapprochement doux) — resynchronisation d'un utilisateur « absent du
  // routeur » : recréer (user_add en file) ou oublier (retrait du cloud).
  const resyncMutation = useMutation({
    mutationFn: (vars: { user: HotspotUser; action: "recreate" | "forget" }) =>
      api<{ ok: boolean }>(`/api/users/${vars.user.id}/resync`, {
        method: "POST",
        body: { action: vars.action },
      }),
    onSuccess: (_res, vars) => {
      toast.success(
        vars.action === "recreate"
          ? tf("users.resyncRecreateToast", { name: vars.user.username })
          : tf("users.resyncForgetToast", { name: vars.user.username }),
      );
      invalidateUsers();
    },
    onError: (error: Error) => toast.error(error.message),
  });

  // Export CSV (F4) — GET /api/users/export avec les filtres courants.
  async function handleExportCsv() {
    try {
      setExporting(true);
      const date = new Date().toISOString().slice(0, 10);
      await apiDownload("/api/users/export", `${lang === "fr" ? "utilisateurs" : "users"}-${date}.csv`, {
        kind: "regular",
        search,
        status: statusParam,
        profileId: profileParam,
      });
      toast.success(t("common.exportDownloaded"));
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t("common.exportFailed"));
    } finally {
      setExporting(false);
    }
  }

  const selectedProfile = profiles?.find((p) => p.id === form.profileId);
  const formValid = form.profileId !== "" && (editing !== null || form.routerId !== "");

  function submitUser() {
    if (!formValid || saveMutation.isPending) return;
    saveMutation.mutate({ id: editing?.id ?? null, form });
  }

  const extendNum = parseInt(extendDays, 10);
  const extendValid = Number.isInteger(extendNum) && extendNum >= 1 && extendNum <= 3650;

  function submitExtend() {
    if (!extendValid || extendMutation.isPending || !extending) return;
    extendMutation.mutate({ user: extending, days: extendNum });
  }

  function submitBulkExtend() {
    if (!extendValid || bulkMutation.isPending || selected.size === 0) return;
    bulkMutation.mutate({ action: "extend", ids: selectedIds, days: extendNum });
  }

  const hasFilters = search !== "" || profileFilter !== "all" || statusFilter !== "all";

  // Sélection : état de la page courante pour la case « tout sélectionner ».
  const allPageSelected = users.length > 0 && users.every((u) => selected.has(u.id));
  const somePageSelected = users.some((u) => selected.has(u.id));
  const selectedIds = Array.from(selected);

  // Menu d'actions d'un utilisateur — partagé entre la vue liste et la vue cartes.
  function renderUserMenu(user: HotspotUser) {
    return (
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            variant="ghost"
            size="icon"
            className="size-10 text-muted-foreground hover:text-foreground"
            aria-label={tf("common.actionsFor", { name: user.username })}
          >
            <MoreHorizontal className="size-4" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-56">
          <DropdownMenuItem className="min-h-10" onClick={() => openEdit(user)}>
            <Pencil className="size-4" />
            {t("common.edit")}
          </DropdownMenuItem>
          <DropdownMenuItem
            className="min-h-10"
            disabled={toggleMutation.isPending && toggleMutation.variables?.id === user.id}
            onClick={() => toggleMutation.mutate(user)}
          >
            {toggleMutation.isPending && toggleMutation.variables?.id === user.id ? (
              <Loader2 className="size-4 animate-spin" />
            ) : (
              <Power className="size-4" />
            )}
            {user.disabled === true || user.status === "disabled"
              ? t("common.activate")
              : t("common.deactivate")}
          </DropdownMenuItem>
          <DropdownMenuItem className="min-h-10" onClick={() => void copyCredentials(user)}>
            <ClipboardCopy className="size-4" />
            {t("users.copyCredentials")}
          </DropdownMenuItem>
          {/* Prolongation masquée sur un voucher jamais connecté : sa validité
              s'ancrera au 1er login (elle se règle via le profil). */}
          {!(user.kind === "voucher" && !user.expiresAt) && (
            <DropdownMenuItem
              className="min-h-10"
              onClick={() => {
                setExtendDays("7");
                setExtending(user);
              }}
            >
              <CalendarPlus className="size-4" />
              {t("users.extend")}
            </DropdownMenuItem>
          )}
          <DropdownMenuItem
            className="min-h-10"
            disabled={resetStatsMutation.isPending && resetStatsMutation.variables?.id === user.id}
            onClick={() => resetStatsMutation.mutate(user)}
          >
            {resetStatsMutation.isPending && resetStatsMutation.variables?.id === user.id ? (
              <Loader2 className="size-4 animate-spin" />
            ) : (
              <RotateCcw className="size-4" />
            )}
            {t("users.resetStats")}
          </DropdownMenuItem>
          {/* N — resynchronisation (uniquement si absent du routeur). */}
          {user.missingOnRouter && (
            <>
              <DropdownMenuSeparator />
              <DropdownMenuItem
                className="min-h-10"
                disabled={resyncMutation.isPending && resyncMutation.variables?.user.id === user.id}
                onClick={() => resyncMutation.mutate({ user, action: "recreate" })}
              >
                {resyncMutation.isPending && resyncMutation.variables?.user.id === user.id ? (
                  <Loader2 className="size-4 animate-spin" />
                ) : (
                  <RefreshCcw className="size-4" />
                )}
                {t("users.resyncRecreate")}
              </DropdownMenuItem>
              <DropdownMenuItem
                className="min-h-10"
                disabled={resyncMutation.isPending && resyncMutation.variables?.user.id === user.id}
                onClick={() => {
                  if (window.confirm(t("users.resyncForgetConfirm"))) {
                    resyncMutation.mutate({ user, action: "forget" });
                  }
                }}
              >
                <Trash2 className="size-4" />
                {t("users.resyncForget")}
              </DropdownMenuItem>
            </>
          )}
          <DropdownMenuSeparator />
          <DropdownMenuItem variant="destructive" className="min-h-10" onClick={() => setDeleting(user)}>
            <Trash2 className="size-4" />
            {t("common.delete")}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    );
  }

  return (
    <div className="space-y-4 sm:space-y-6">
      <PageHeader
        title={t("users.title")}
        description={t("users.description")}
        actions={
          <>
            <Button
              variant="outline"
              className="h-10"
              onClick={() => void handleExportCsv()}
              disabled={exporting}
            >
              {exporting ? <Loader2 className="size-4 animate-spin" /> : <Download className="size-4" />}
              {t("common.exportCsv")}
            </Button>
            <Button variant="outline" className="h-10" onClick={() => setCleanupOpen(true)}>
              <Eraser className="size-4" />
              {t("users.cleanup")}
            </Button>
            <Button className="h-10" onClick={openCreate}>
              <UserPlus className="size-4" />
              {t("users.new")}
            </Button>
          </>
        }
      />

      {/* Barre de filtres */}
      <Card className="gap-0 py-0">
        <CardContent className="flex flex-col gap-3 p-4 sm:flex-row sm:items-center">
          <div className="relative w-full sm:max-w-xs sm:flex-1">
            <Search className="absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" aria-hidden />
            <Input
              className="h-10 pl-9"
              placeholder={t("users.searchPlaceholder")}
              value={searchInput}
              onChange={(event) => setSearchInput(event.target.value)}
              aria-label={t("users.searchLabel")}
            />
          </div>
          <div className="flex flex-1 flex-wrap gap-3 sm:justify-end">
            <Select
              value={profileFilter}
              onValueChange={(value) => {
                setProfileFilter(value);
                setPage(1);
              }}
            >
              <SelectTrigger className="h-10 w-full sm:w-48" aria-label={t("common.filterByProfile")}>
                <SelectValue placeholder={t("common.profile")} />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">{t("common.allProfiles")}</SelectItem>
                {profiles?.map((profile) => (
                  <SelectItem key={profile.id} value={profile.id}>
                    {profile.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Select
              value={statusFilter}
              onValueChange={(value) => {
                setStatusFilter(value);
                setPage(1);
              }}
            >
              <SelectTrigger className="h-10 w-full sm:w-44" aria-label={t("common.filterByStatus")}>
                <SelectValue placeholder={t("common.status")} />
              </SelectTrigger>
              <SelectContent>
                {STATUS_OPTIONS.map((option) => (
                  <SelectItem key={option.value} value={option.value}>
                    {t(option.labelKey)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <div
              className="flex items-center gap-1 rounded-lg border p-1"
              role="group"
              aria-label={t("users.viewMode")}
            >
              <Button
                variant={viewMode === "list" ? "secondary" : "ghost"}
                size="icon"
                className="size-8"
                onClick={() => changeView("list")}
                aria-pressed={viewMode === "list"}
                aria-label={t("users.viewListLabel")}
                title={t("users.viewListLabel")}
              >
                <List className="size-4" />
              </Button>
              <Button
                variant={viewMode === "cards" ? "secondary" : "ghost"}
                size="icon"
                className="size-8"
                onClick={() => changeView("cards")}
                aria-pressed={viewMode === "cards"}
                aria-label={t("users.viewCardsLabel")}
                title={t("users.viewCardsLabel")}
              >
                <LayoutGrid className="size-4" />
              </Button>
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Barre d'actions groupées (sélection multiple) */}
      {selected.size > 0 && (
        <div
          className="flex flex-wrap items-center gap-2 rounded-lg border border-primary/30 bg-primary/5 px-4 py-3"
          role="toolbar"
          aria-label={t("users.bulkActions")}
        >
          <Badge variant="secondary" className="mr-1 gap-1 px-2">
            <Users className="size-3.5" aria-hidden />
            {tf("users.selected", { n: selected.size, s: selected.size > 1 ? "s" : "" })}
          </Badge>
          <Button
            variant="outline"
            size="sm"
            className="h-9"
            disabled={bulkMutation.isPending}
            onClick={() => bulkMutation.mutate({ action: "enable", ids: selectedIds })}
          >
            <Power className="size-4" />
            {t("users.bulkActivate")}
          </Button>
          <Button
            variant="outline"
            size="sm"
            className="h-9"
            disabled={bulkMutation.isPending}
            onClick={() => bulkMutation.mutate({ action: "disable", ids: selectedIds })}
          >
            <PowerOff className="size-4" />
            {t("users.bulkDeactivate")}
          </Button>
          <Button
            variant="outline"
            size="sm"
            className="h-9"
            disabled={bulkMutation.isPending}
            onClick={() => {
              setExtendDays("7");
              setBulkExtendOpen(true);
            }}
          >
            <CalendarPlus className="size-4" />
            {t("users.bulkExtend")}
          </Button>
          <Button
            variant="outline"
            size="sm"
            className="h-9"
            disabled={bulkMutation.isPending}
            onClick={() => bulkMutation.mutate({ action: "reset-stats", ids: selectedIds })}
          >
            <RotateCcw className="size-4" />
            {t("users.bulkResetStats")}
          </Button>
          <Button variant="destructive" size="sm" className="h-9" onClick={() => setBulkDeleteOpen(true)}>
            <Trash2 className="size-4" />
            {t("users.bulkDelete")}
          </Button>
          <Button variant="ghost" size="sm" className="ml-auto h-9" onClick={clearSelection}>
            <X className="size-4" />
            {t("users.clearSelection")}
          </Button>
        </div>
      )}

      {/* Table des utilisateurs */}
      <Card className="gap-0 py-0">
        {isLoading ? (
          <LoadingRows rows={8} />
        ) : users.length === 0 ? (
          <EmptyState
            icon={Users}
            title={t("users.empty")}
            description={hasFilters ? t("users.emptyFiltered") : t("users.emptyDesc")}
            action={
              !hasFilters && (
                <Button onClick={openCreate}>
                  <UserPlus className="size-4" />
                  {t("users.new")}
                </Button>
              )
            }
          />
        ) : (
          <>
            {viewMode === "cards" ? (
              <div className="grid gap-3 p-4 sm:grid-cols-2 sm:p-6 xl:grid-cols-3">
                {users.map((user) => (
                  <div
                    key={user.id}
                    className={cn(
                      "flex flex-col rounded-lg border p-4 transition-colors",
                      selected.has(user.id) && "border-primary/40 bg-primary/5",
                    )}
                  >
                    <div className="flex items-start justify-between gap-2">
                      <div className="flex min-w-0 items-start gap-2.5">
                        <Checkbox
                          className="mt-0.5"
                          checked={selected.has(user.id)}
                          onCheckedChange={() => toggleOne(user.id)}
                          aria-label={tf("users.selectUser", { name: user.username })}
                        />
                        <div className="flex min-w-0 flex-col gap-0.5">
                          <span className="truncate font-mono text-sm font-medium">{user.username}</span>
                          <PasswordCell
                            password={user.password}
                            visible={revealed.has(user.id)}
                            onToggle={() => toggleReveal(user.id)}
                            label={
                              revealed.has(user.id)
                                ? tf("vouchers.hidePassword", { name: user.username })
                                : tf("vouchers.showPassword", { name: user.username })
                            }
                          />
                        </div>
                      </div>
                      {renderUserMenu(user)}
                    </div>
                    <div className="mt-3 flex flex-wrap items-center gap-1.5">
                      <Badge variant="outline" className="max-w-36 truncate">
                        {user.profileName}
                      </Badge>
                      <StatusBadge status={user.status} dot />
                      {user.missingOnRouter && (
                        <Badge
                          variant="outline"
                          className="gap-1 border-amber-500/30 bg-amber-500/10 px-1.5 py-0 text-[10px] font-medium text-amber-600 dark:text-amber-400"
                          title={t("users.missingOnRouterHint")}
                        >
                          <ShieldQuestion className="size-3" aria-hidden />
                          {t("users.missingOnRouter")}
                        </Badge>
                      )}
                    </div>
                    <dl className="mt-3 grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-xs">
                      <dt className="text-muted-foreground">{t("common.router")}</dt>
                      <dd className="truncate text-right text-foreground sm:text-left">{user.routerName || "—"}</dd>
                      <dt className="text-muted-foreground">{t("users.data")}</dt>
                      <dd className="tabular-nums text-foreground">{formatBytes(user.bytesIn + user.bytesOut, lang)}</dd>
                      <dt className="text-muted-foreground">{t("users.expires")}</dt>
                      <dd className="tabular-nums text-foreground">
                        {user.kind === "voucher" && !user.expiresAt
                          ? t("users.firstLoginValidity")
                          : formatDate(user.expiresAt, lang)}
                      </dd>
                      <dt className="text-muted-foreground">{t("common.created")}</dt>
                      <dd className="tabular-nums text-foreground">{formatDate(user.createdAt, lang)}</dd>
                    </dl>
                  </div>
                ))}
              </div>
            ) : (
            <div className="overflow-x-auto">
              <Table>
                <TableHeader>
                  <TableRow className="hover:bg-transparent">
                    <TableHead className="w-12 pl-4 sm:pl-6">
                      <Checkbox
                        checked={allPageSelected ? true : somePageSelected ? "indeterminate" : false}
                        onCheckedChange={toggleAllPage}
                        aria-label={t("users.selectAllPage")}
                      />
                    </TableHead>
                    <TableHead className="text-muted-foreground">{t("common.user")}</TableHead>
                    <TableHead className="text-muted-foreground">{t("common.profile")}</TableHead>
                    <TableHead className="hidden text-muted-foreground md:table-cell">{t("common.router")}</TableHead>
                    <TableHead className="text-muted-foreground">{t("common.status")}</TableHead>
                    <TableHead className="text-muted-foreground">{t("users.data")}</TableHead>
                    <TableHead className="hidden text-muted-foreground sm:table-cell">{t("users.expires")}</TableHead>
                    <TableHead className="hidden text-muted-foreground lg:table-cell">{t("common.created")}</TableHead>
                    <TableHead className="pr-4 text-right text-muted-foreground sm:pr-6">{t("common.actions")}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {users.map((user) => (
                    <TableRow key={user.id} data-state={selected.has(user.id) ? "selected" : undefined}>
                      <TableCell className="pl-4 sm:pl-6">
                        <Checkbox
                          checked={selected.has(user.id)}
                          onCheckedChange={() => toggleOne(user.id)}
                          aria-label={tf("users.selectUser", { name: user.username })}
                        />
                      </TableCell>
                      <TableCell>
                        <div className="flex flex-col gap-0.5 py-1">
                          <span className="font-mono text-sm font-medium">{user.username}</span>
                          <PasswordCell
                            password={user.password}
                            visible={revealed.has(user.id)}
                            onToggle={() => toggleReveal(user.id)}
                            label={
                              revealed.has(user.id)
                                ? tf("vouchers.hidePassword", { name: user.username })
                                : tf("vouchers.showPassword", { name: user.username })
                            }
                          />
                        </div>
                      </TableCell>
                      <TableCell>
                        <Badge variant="outline" className="max-w-36 truncate">
                          {user.profileName}
                        </Badge>
                      </TableCell>
                      <TableCell className="hidden max-w-40 truncate text-muted-foreground md:table-cell">
                        {user.routerName}
                      </TableCell>
                      <TableCell>
                        <div className="flex flex-wrap items-center gap-1.5">
                          <StatusBadge status={user.status} dot />
                          {/* N — rapprochement doux : badge « absent du routeur ». */}
                          {user.missingOnRouter && (
                            <Badge
                              variant="outline"
                              className="gap-1 border-amber-500/30 bg-amber-500/10 px-1.5 py-0 text-[10px] font-medium text-amber-600 dark:text-amber-400"
                              title={t("users.missingOnRouterHint")}
                            >
                              <ShieldQuestion className="size-3" aria-hidden />
                              {t("users.missingOnRouter")}
                            </Badge>
                          )}
                        </div>
                      </TableCell>
                      <TableCell className="tabular-nums text-muted-foreground">
                        {formatBytes(user.bytesIn + user.bytesOut, lang)}
                      </TableCell>
                      <TableCell className="hidden tabular-nums text-muted-foreground sm:table-cell">
                        {user.kind === "voucher" && !user.expiresAt
                          ? t("users.firstLoginValidity")
                          : formatDate(user.expiresAt, lang)}
                      </TableCell>
                      <TableCell className="hidden tabular-nums text-muted-foreground lg:table-cell">
                        {formatDate(user.createdAt, lang)}
                      </TableCell>
                      <TableCell className="pr-4 text-right sm:pr-6">{renderUserMenu(user)}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
            )}

            {/* Pagination */}
            <div className="flex flex-wrap items-center justify-between gap-3 border-t px-4 py-3 sm:px-6">
              <p className="text-xs text-muted-foreground">
                {isFetching
                  ? t("common.refreshing")
                  : tf("common.range", { start: rangeStart, end: rangeEnd, total: totalCount })}
              </p>
              <div className="flex items-center gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  className="h-10"
                  onClick={() => setPage((p) => Math.max(1, p - 1))}
                  disabled={safePage <= 1}
                >
                  <ChevronLeft className="size-4" />
                  {t("common.previous")}
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  className="h-10"
                  onClick={() => setPage((p) => Math.min(maxPage, p + 1))}
                  disabled={safePage >= maxPage}
                >
                  {t("common.next")}
                  <ChevronRight className="size-4" />
                </Button>
              </div>
            </div>
          </>
        )}
      </Card>

      {/* Dialogue création / édition */}
      <Dialog open={dialogOpen} onOpenChange={closeDialog}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>
              {editing ? tf("users.editTitle", { name: editing.username }) : t("users.new")}
            </DialogTitle>
            <DialogDescription>
              {editing ? t("users.editDesc") : t("users.createDesc")}
            </DialogDescription>
          </DialogHeader>

          <form
            className="grid gap-4"
            onSubmit={(event) => {
              event.preventDefault();
              submitUser();
            }}
          >
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="grid gap-2">
                <Label htmlFor="user-username">{t("common.user")}</Label>
                <Input
                  id="user-username"
                  placeholder={t("users.autoGenerated")}
                  value={form.username}
                  onChange={(event) => setForm((f) => ({ ...f, username: event.target.value }))}
                  disabled={saveMutation.isPending}
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="user-password">{t("common.password")}</Label>
                <Input
                  id="user-password"
                  type="text"
                  autoComplete="new-password"
                  placeholder={editing ? t("users.unchangedIfEmpty") : t("users.autoGenerated")}
                  value={form.password}
                  onChange={(event) => setForm((f) => ({ ...f, password: event.target.value }))}
                  disabled={saveMutation.isPending}
                />
              </div>
            </div>

            <div className="grid gap-4 sm:grid-cols-2">
              <div className="grid gap-2">
                <Label htmlFor="user-profile">{t("common.profile")}</Label>
                <Select
                  value={form.profileId}
                  onValueChange={(value) => setForm((f) => ({ ...f, profileId: value }))}
                  disabled={saveMutation.isPending}
                >
                  <SelectTrigger id="user-profile" className="h-10 w-full">
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
                <Label htmlFor="user-router">{t("common.router")}</Label>
                <Select
                  value={form.routerId}
                  onValueChange={(value) => setForm((f) => ({ ...f, routerId: value }))}
                  disabled={saveMutation.isPending || editing !== null}
                >
                  <SelectTrigger id="user-router" className="h-10 w-full">
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
                {editing && <p className="text-xs text-muted-foreground">{t("users.routerLocked")}</p>}
              </div>
            </div>

            <div className="grid gap-2">
              <Label htmlFor="user-comment">{t("users.commentLabel")}</Label>
              <Textarea
                id="user-comment"
                placeholder={t("users.commentPlaceholder")}
                value={form.comment}
                onChange={(event) => setForm((f) => ({ ...f, comment: event.target.value }))}
                disabled={saveMutation.isPending}
              />
            </div>

            {selectedProfile && (
              <p className="rounded-lg border bg-muted/40 px-3 py-2 text-sm">
                {t("users.plan")} : <span className="font-semibold text-primary">{selectedProfile.name}</span> —{" "}
                {formatCurrency(selectedProfile.price, currency, lang)} ·{" "}
                {tf("users.validity", { n: selectedProfile.validityDays })}
              </p>
            )}

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
                {editing ? t("common.save") : t("users.createSubmit")}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* Confirmation suppression */}
      <AlertDialog open={deleting !== null} onOpenChange={(open) => !open && setDeleting(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{tf("users.deleteTitle", { name: deleting?.username ?? "" })}</AlertDialogTitle>
            <AlertDialogDescription>{t("users.deleteDesc")}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleteMutation.isPending}>{t("common.cancel")}</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-white hover:bg-destructive/90"
              disabled={deleteMutation.isPending}
              onClick={(event) => {
                event.preventDefault();
                if (deleting) deleteMutation.mutate(deleting);
              }}
            >
              {deleteMutation.isPending && <Loader2 className="size-4 animate-spin" />}
              {t("common.delete")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* Prolongation d'expiration (F4) */}
      <Dialog open={extending !== null} onOpenChange={(open) => !open && setExtending(null)}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{tf("users.extendTitle", { name: extending?.username ?? "" })}</DialogTitle>
            <DialogDescription>
              {extending?.expiresAt
                ? tf("users.extendCurrent", { date: formatDate(extending.expiresAt, lang) })
                : t("users.extendNoDate")}{" "}
              {t("users.extendAuto")}
            </DialogDescription>
          </DialogHeader>
          <form
            className="grid gap-4"
            onSubmit={(event) => {
              event.preventDefault();
              submitExtend();
            }}
          >
            <div className="grid gap-2">
              <Label htmlFor="extend-days">{t("users.extendDays")}</Label>
              <div className="flex gap-2">
                {[1, 7, 30].map((preset) => (
                  <Button
                    key={preset}
                    type="button"
                    variant={extendDays === String(preset) ? "secondary" : "outline"}
                    className="h-10 flex-1"
                    disabled={extendMutation.isPending}
                    onClick={() => setExtendDays(String(preset))}
                  >
                    {tf("users.daysUnit", { n: preset })}
                  </Button>
                ))}
              </div>
              <Input
                id="extend-days"
                type="number"
                min={1}
                max={3650}
                value={extendDays}
                onChange={(event) => setExtendDays(event.target.value)}
                disabled={extendMutation.isPending}
                aria-invalid={!extendValid}
              />
              <p className="text-xs text-muted-foreground">{t("users.extendDaysHint")}</p>
            </div>

            {extendMutation.isError && (
              <p className="text-sm text-destructive" role="alert">
                {extendMutation.error.message}
              </p>
            )}

            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => setExtending(null)}
                disabled={extendMutation.isPending}
              >
                {t("common.cancel")}
              </Button>
              <Button type="submit" disabled={!extendValid || extendMutation.isPending}>
                {extendMutation.isPending && <Loader2 className="size-4 animate-spin" />}
                {t("users.extendSubmit")}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* Nettoyage des expirés (F5) */}
      <AlertDialog open={cleanupOpen} onOpenChange={setCleanupOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t("users.cleanupTitle")}</AlertDialogTitle>
            <AlertDialogDescription>{t("users.cleanupDesc")}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={cleanupMutation.isPending}>{t("common.cancel")}</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-white hover:bg-destructive/90"
              disabled={cleanupMutation.isPending}
              onClick={(event) => {
                event.preventDefault();
                cleanupMutation.mutate();
              }}
            >
              {cleanupMutation.isPending && <Loader2 className="size-4 animate-spin" />}
              {t("users.cleanupSubmit")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* Prolongation groupée (sélection multiple) */}
      <Dialog open={bulkExtendOpen} onOpenChange={setBulkExtendOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>
              {tf("users.bulkExtendTitle", { n: selected.size, s: selected.size > 1 ? "s" : "" })}
            </DialogTitle>
            <DialogDescription>{t("users.bulkExtendDesc")}</DialogDescription>
          </DialogHeader>
          <form
            className="grid gap-4"
            onSubmit={(event) => {
              event.preventDefault();
              submitBulkExtend();
            }}
          >
            <div className="grid gap-2">
              <Label htmlFor="bulk-extend-days">{t("users.extendDays")}</Label>
              <div className="flex gap-2">
                {[1, 7, 30].map((preset) => (
                  <Button
                    key={preset}
                    type="button"
                    variant={extendDays === String(preset) ? "secondary" : "outline"}
                    className="h-10 flex-1"
                    disabled={bulkMutation.isPending}
                    onClick={() => setExtendDays(String(preset))}
                  >
                    {tf("users.daysUnit", { n: preset })}
                  </Button>
                ))}
              </div>
              <Input
                id="bulk-extend-days"
                type="number"
                min={1}
                max={3650}
                value={extendDays}
                onChange={(event) => setExtendDays(event.target.value)}
                disabled={bulkMutation.isPending}
                aria-invalid={!extendValid}
              />
              <p className="text-xs text-muted-foreground">{t("users.extendDaysHint")}</p>
            </div>

            {bulkMutation.isError && (
              <p className="text-sm text-destructive" role="alert">
                {bulkMutation.error.message}
              </p>
            )}

            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => setBulkExtendOpen(false)}
                disabled={bulkMutation.isPending}
              >
                {t("common.cancel")}
              </Button>
              <Button type="submit" disabled={!extendValid || bulkMutation.isPending}>
                {bulkMutation.isPending && <Loader2 className="size-4 animate-spin" />}
                {t("users.extendSubmit")}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* Suppression groupée (sélection multiple) */}
      <AlertDialog open={bulkDeleteOpen} onOpenChange={setBulkDeleteOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {tf("users.bulkDeleteTitle", { n: selected.size, s: selected.size > 1 ? "s" : "" })}
            </AlertDialogTitle>
            <AlertDialogDescription>{t("users.bulkDeleteDesc")}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={bulkMutation.isPending}>{t("common.cancel")}</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-white hover:bg-destructive/90"
              disabled={bulkMutation.isPending}
              onClick={(event) => {
                event.preventDefault();
                bulkMutation.mutate({ action: "delete", ids: selectedIds });
              }}
            >
              {bulkMutation.isPending && <Loader2 className="size-4 animate-spin" />}
              {t("common.delete")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
