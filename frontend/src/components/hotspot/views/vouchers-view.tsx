"use client";

// Vue Vouchers (kind=voucher) — générateur en série, impression de tickets,
// suivi des statuts et traçabilité des lots (onglet Lots).
// Quick print (F12) : bouton « Réimpression rapide » qui réimprime le dernier
// lot imprimé (localStorage "mikcloud-last-batch", écrit par uc-print-dialog).

import { useEffect, useMemo, useRef, useState } from "react";
import { usePathname, useRouter } from "next/navigation";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ArrowLeftRight,
  CheckCircle2,
  ChevronLeft,
  ChevronRight,
  Clock,
  Copy,
  Download,
  Eye,
  Info,
  Layers,
  Loader2,
  MoreHorizontal,
  Printer,
  RefreshCcw,
  Search,
  ShieldQuestion,
  Ticket,
  TicketPlus,
  Trash2,
  Wallet,
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
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { EmptyState } from "@/components/hotspot/empty-state";
import { LoadingCards, LoadingRows } from "@/components/hotspot/loading";
import { PageHeader } from "@/components/hotspot/page-header";
import { StatCard } from "@/components/hotspot/stat-card";
import { StatusBadge } from "@/components/hotspot/status-badge";
import { useCurrency, useSettings } from "@/components/hotspot/parts/sd-currency";
import { copyToClipboard } from "@/components/hotspot/parts/uc-clipboard";
import { PasswordCell } from "@/components/hotspot/parts/uc-password-cell";
import { LAST_BATCH_STORAGE_KEY, UcPrintDialog } from "@/components/hotspot/parts/uc-print-dialog";
import { VoucherTransferDialog } from "@/components/hotspot/parts/voucher-transfer-dialog";
import { VoucherWizardDialog } from "@/components/hotspot/parts/voucher-wizard-dialog";
import { BatchLifeBadge, BatchLifeBar } from "@/components/hotspot/parts/batch-life-bar";
import { BatchDetailSheet } from "@/components/hotspot/parts/batch-detail-sheet";
import { BatchPrintDialog } from "@/components/hotspot/parts/batch-print-dialog";
import { api, apiDownload } from "@/lib/hotspot/api";
import { useI18n } from "@/lib/hotspot/i18n";
import { detailFromPath, viewToPath } from "@/lib/hotspot/view-path";
import { formatBytes, formatCurrency, formatDate } from "@/lib/hotspot/format";
import type {
  BatchWithStats,
  GenerateVouchersRequest,
  GenerateVouchersResponse,
  HotspotUser,
  PagedBatches,
  PagedUsers,
  Profile,
  Reseller,
  RouterDevice,
  VoucherTemplate,
} from "@/lib/hotspot/types";
import { cn } from "@/lib/utils";

const PAGE_SIZE = 12;
const BATCH_PAGE_SIZE = 10;

const STATUS_OPTIONS = [
  { value: "all", labelKey: "common.allStatuses" },
  { value: "active", labelKey: "common.statusActive" },
  { value: "online", labelKey: "common.statusOnline" },
  { value: "used", labelKey: "common.statusUsed" },
  { value: "expired", labelKey: "common.statusExpired" },
  { value: "disabled", labelKey: "common.statusDisabled" },
];

// Refonte « fiche de vie » — cycle de vie filtrable (dérivé backend batchLifecycle).
const BATCH_STATUS_OPTIONS = [
  { value: "all", labelKey: "common.allStatuses" },
  { value: "stock", labelKey: "vouchers.batches.life.stock" },
  { value: "consumed", labelKey: "vouchers.batches.life.consumed" },
  { value: "expired", labelKey: "vouchers.batches.life.expired" },
  { value: "purged", labelKey: "vouchers.batches.life.purged" },
];

function shortBatch(batchId: string): string {
  return batchId.split("-").pop() || batchId;
}

export default function VouchersView() {
  const { t, tf, lang } = useI18n();
  const currency = useCurrency();
  const { data: settings } = useSettings();
  const tenantName = settings?.tenant.name || "MikCloud";
  const queryClient = useQueryClient();

  // Onglet actif : liste des vouchers ou traçabilité des lots
  const [tab, setTab] = useState<"vouchers" | "batches">("vouchers");

  // Filtres de la table (recherche avec debounce ~400 ms)
  const [searchInput, setSearchInput] = useState("");
  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState("all");
  const [profileFilter, setProfileFilter] = useState("all");
  const [page, setPage] = useState(1);

  // Filtres des lots
  const [batchSearchInput, setBatchSearchInput] = useState("");
  const [batchSearch, setBatchSearch] = useState("");
  const [batchRouterFilter, setBatchRouterFilter] = useState("all");
  const [batchPage, setBatchPage] = useState(1);
  // Refonte « fiche de vie » — filtres canal (provenance), cycle de vie et
  // détenteur LIVE du stock vendable (direct | id du revendeur).
  const [batchChannelFilter, setBatchChannelFilter] = useState("all");
  const [batchStatusFilter, setBatchStatusFilter] = useState("all");
  const [batchHolderFilter, setBatchHolderFilter] = useState("all");

  useEffect(() => {
    const timer = setTimeout(() => {
      setBatchSearch(batchSearchInput.trim());
      setBatchPage(1);
    }, 400);
    return () => clearTimeout(timer);
  }, [batchSearchInput]);

  useEffect(() => {
    const timer = setTimeout(() => {
      setSearch(searchInput.trim());
      setPage(1);
    }, 400);
    return () => clearTimeout(timer);
  }, [searchInput]);

  // Révélation des mots de passe (par ligne)
  const [revealed, setRevealed] = useState<Set<string>>(new Set());

  // Générateur — le formulaire vit dans VoucherWizardDialog (3 étapes).
  const [genOpen, setGenOpen] = useState(false);

  // Impression (liste simple — ancien dialog)
  const [printOpen, setPrintOpen] = useState(false);
  const [printVouchers, setPrintVouchers] = useState<HotspotUser[]>([]);
  const [printTitle, setPrintTitle] = useState("");
  /** Lot en cours d'impression (F12) — mémorisé à l'impression par le dialog. */
  const [printBatchId, setPrintBatchId] = useState<string | undefined>(undefined);

  // Suppression
  const [deleting, setDeleting] = useState<HotspotUser | null>(null);

  const { data: profiles } = useQuery({
    queryKey: ["/api/profiles"],
    queryFn: () => api<Profile[]>("/api/profiles"),
  });

  const { data: routers } = useQuery({
    queryKey: ["/api/routers"],
    queryFn: () => api<RouterDevice[]>("/api/routers"),
  });

  const { data: resellers } = useQuery({
    queryKey: ["/api/resellers"],
    queryFn: () => api<Reseller[]>("/api/resellers"),
  });

  // Modèles de vouchers (F2) pour le dialog d'impression — la query échoue en 404
  // tant que le backend P0 n'est pas déployé : fallback silencieux sur le ticket standard.
  const { data: templates } = useQuery({
    queryKey: ["/api/templates"],
    queryFn: () => api<VoucherTemplate[]>("/api/templates"),
    staleTime: 5 * 60_000,
  });

  // Statistiques globales (page large sans filtre) — statuts + valeur du stock actif.
  const { data: statsData, isLoading: statsLoading } = useQuery({
    queryKey: ["/api/vouchers", "stats"],
    queryFn: () => api<PagedUsers>("/api/vouchers", { params: { pageSize: 500 } }),
    refetchInterval: 20_000,
  });

  const stats = useMemo(() => statsData?.data ?? [], [statsData]);
  const activeCount = stats.filter((v) => v.status === "active").length;
  const usedCount = stats.filter((v) => v.status === "used").length;
  const expiredCount = stats.filter((v) => v.status === "expired").length;
  const stockValue = stats
    .filter((v) => v.status === "active")
    .reduce((acc, v) => acc + v.price, 0);

  // Liste paginée filtrée
  const statusParam = statusFilter === "all" ? undefined : statusFilter;
  const profileParam = profileFilter === "all" ? undefined : profileFilter;

  const { data: pagedData, isLoading, isFetching } = useQuery({
    queryKey: ["/api/vouchers", "list", { search, status: statusParam, profileId: profileParam, page }],
    queryFn: () =>
      api<PagedUsers>("/api/vouchers", {
        params: { search, status: statusParam, profileId: profileParam, page, pageSize: PAGE_SIZE },
      }),
    refetchInterval: 20_000,
    placeholderData: (previous) => previous,
  });

  const vouchers = pagedData?.data ?? [];
  const totalCount = pagedData?.total ?? 0;
  const maxPage = Math.max(1, Math.ceil(totalCount / PAGE_SIZE));
  const safePage = Math.min(page, maxPage);
  const rangeStart = totalCount === 0 ? 0 : (safePage - 1) * PAGE_SIZE + 1;
  const rangeEnd = Math.min(safePage * PAGE_SIZE, totalCount);

  function invalidateVouchers() {
    void queryClient.invalidateQueries({ queryKey: ["/api/vouchers"] });
    void queryClient.invalidateQueries({ queryKey: ["/api/dashboard"] });
  }

  // Phase D — deep-link /app/vouchers/<batchId> : « détail lot » = onglet
  // vouchers filtré sur le lot (équivalent viewBatchVouchers, sans push —
  // on est déjà sur l'URL). Segment consommé localement : ni le store ni
  // app-route ne changent (fix 192ad9f préservé). À la sortie du détail
  // (Retour navigateur), le filtre lot est levé s'il n'a pas divergé.
  const router = useRouter();
  const pathname = usePathname();
  const detailBatchId = detailFromPath(pathname, "vouchers");
  const prevDetail = useRef<string | null>(null);
  // Miroir de la recherche lu par l'effet ci-dessous : synchronisé dans un
  // effet (jamais pendant le rendu — règle react-hooks/refs), déclaré
  // AVANT l'effet de détail pour tourner en premier.
  const searchRef = useRef(search);
  useEffect(() => {
    searchRef.current = search;
  });

  useEffect(() => {
    const leaving = prevDetail.current;
    if (detailBatchId) {
      setTab("vouchers");
      setSearchInput(detailBatchId);
      setSearch(detailBatchId);
      setStatusFilter("all");
      setProfileFilter("all");
      setPage(1);
    } else if (leaving && searchRef.current === leaving) {
      setSearchInput("");
      setSearch("");
      setStatusFilter("all");
      setProfileFilter("all");
      setPage(1);
    }
    prevDetail.current = detailBatchId;
  }, [detailBatchId]);

  // N (rapprochement doux) — resynchronisation d'un voucher « absent du
  // routeur » : recréer (user_add en file) ou oublier (retrait du cloud).
  const resyncMutation = useMutation({
    mutationFn: (vars: { voucher: HotspotUser; action: "recreate" | "forget" }) =>
      api<{ ok: boolean }>(`/api/users/${vars.voucher.id}/resync`, {
        method: "POST",
        body: { action: vars.action },
      }),
    onSuccess: (_res, vars) => {
      toast.success(
        vars.action === "recreate"
          ? tf("users.resyncRecreateToast", { name: vars.voucher.username })
          : tf("users.resyncForgetToast", { name: vars.voucher.username }),
      );
      invalidateVouchers();
    },
    onError: (error: Error) => toast.error(error.message),
  });

  function toggleReveal(id: string) {
    setRevealed((previous) => {
      const next = new Set(previous);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  async function copyCode(voucher: HotspotUser) {
    const ok = await copyToClipboard(voucher.username);
    if (ok) toast.success(t("vouchers.codeCopied"));
    else toast.error(t("common.copyImpossible"));
  }

  function printOne(voucher: HotspotUser) {
    setPrintVouchers([voucher]);
    setPrintTitle(tf("vouchers.printOneTitle", { code: voucher.username }));
    setPrintBatchId(voucher.batchId || undefined);
    setPrintOpen(true);
  }

  const generateMutation = useMutation({
    mutationFn: (payload: GenerateVouchersRequest) =>
      api<GenerateVouchersResponse>("/api/vouchers/generate", { method: "POST", body: payload }),
    onSuccess: (res) => {
      toast.success(
        tf("vouchers.generatedToast", { n: res.vouchers.length, batch: shortBatch(res.batchId) }),
      );
      setGenOpen(false);
      setPrintVouchers(res.vouchers);
      setPrintTitle(tf("vouchers.printBatchTitle", { batch: shortBatch(res.batchId) }));
      setPrintBatchId(res.batchId);
      setPrintOpen(true);
      void queryClient.invalidateQueries({ queryKey: ["/api/vouchers"] });
      void queryClient.invalidateQueries({ queryKey: ["/api/dashboard"] });
      void queryClient.invalidateQueries({ queryKey: ["/api/resellers"] });
      void queryClient.invalidateQueries({ queryKey: ["/api/transactions"] });
      void queryClient.invalidateQueries({ queryKey: ["/api/reports"] });
    },
    onError: (error: Error) => toast.error(error.message),
  });

  const deleteMutation = useMutation({
    mutationFn: (voucher: HotspotUser) => api<{ ok: boolean }>(`/api/vouchers/${voucher.id}`, { method: "DELETE" }),
    onSuccess: (_res, voucher) => {
      toast.success(tf("vouchers.deletedToast", { code: voucher.username }));
      setDeleting(null);
      invalidateVouchers();
    },
    onError: (error: Error) => toast.error(error.message),
  });

  // --- Traçabilité des lots ---
  const batchRouterParam = batchRouterFilter === "all" ? undefined : batchRouterFilter;
  const batchStatusParam = batchStatusFilter === "all" ? undefined : batchStatusFilter;
  const batchHolderParam = batchHolderFilter === "all" ? undefined : batchHolderFilter;
  const batchChannelParam = batchChannelFilter === "all" ? undefined : batchChannelFilter;

  const { data: batchData, isLoading: batchesLoading, isFetching: batchesFetching } = useQuery({
    queryKey: [
      "/api/vouchers",
      "batches",
      {
        search: batchSearch,
        routerId: batchRouterParam,
        status: batchStatusParam,
        holder: batchHolderParam,
        channel: batchChannelParam,
        page: batchPage,
      },
    ],
    queryFn: () =>
      api<PagedBatches>("/api/vouchers/batches", {
        params: {
          search: batchSearch,
          routerId: batchRouterParam,
          status: batchStatusParam,
          holder: batchHolderParam,
          channel: batchChannelParam,
          page: batchPage,
          pageSize: BATCH_PAGE_SIZE,
        },
      }),
    refetchInterval: 30_000,
    placeholderData: (previous) => previous,
  });

  const batches = batchData?.data ?? [];
  // Refonte — bande KPI « fiche de vie » (totaux sur l'ensemble FILTRÉ, back).
  const batchKpi = batchData?.summary;
  const batchTotal = batchData?.total ?? 0;
  const batchMaxPage = Math.max(1, Math.ceil(batchTotal / BATCH_PAGE_SIZE));
  const batchSafePage = Math.min(batchPage, batchMaxPage);
  const batchRangeStart = batchTotal === 0 ? 0 : (batchSafePage - 1) * BATCH_PAGE_SIZE + 1;
  const batchRangeEnd = Math.min(batchSafePage * BATCH_PAGE_SIZE, batchTotal);

  const [deletingBatch, setDeletingBatch] = useState<BatchWithStats | null>(null);
  const [printingBatchId, setPrintingBatchId] = useState<string | null>(null);

  // N°18 — Transfert de stock du lot (distribution revendeur / retour de stock).
  const [transferBatch, setTransferBatch] = useState<BatchWithStats | null>(null);

  // Refonte « fiche de vie » — fiche 360° du lot (drawer) + impression
  // multi-formats (F3) + export CSV.
  const [detailBatch, setDetailBatch] = useState<BatchWithStats | null>(null);
  const [multiPrintBatch, setMultiPrintBatch] = useState<BatchWithStats | null>(null);
  const [multiPrintVouchers, setMultiPrintVouchers] = useState<HotspotUser[] | null>(null);
  const [exportingBatches, setExportingBatches] = useState(false);

  const batchDeleteMutation = useMutation({
    mutationFn: (batch: BatchWithStats) =>
      api<{ ok: boolean; deleted: number }>(`/api/vouchers/batch/${batch.id}/delete`, { method: "POST" }),
    onSuccess: (res, batch) => {
      toast.success(
        tf("vouchers.batches.deletedToast", { batch: shortBatch(batch.id), n: res.deleted }),
      );
      setDeletingBatch(null);
      invalidateVouchers();
    },
    onError: (error: Error) => toast.error(error.message),
  });

  // « Voir les vouchers » : bascule sur l'onglet vouchers filtré par le lot.
  function viewBatchVouchers(batch: BatchWithStats) {
    setSearchInput(batch.id);
    setSearch(batch.id);
    setStatusFilter("all");
    setProfileFilter("all");
    setPage(1);
    setTab("vouchers");
    // Phase D — le « détail lot » devient adressable : /app/vouchers/<batchId>
    // (push → le Retour du navigateur revient à la liste). L'effet de
    // synchronisation ci-dessous ré-applique le filtre à l'arrivée du push.
    router.push(viewToPath("vouchers", batch.id), { scroll: false });
  }

  // Charge les vouchers ACTIFS d'un lot pour l'impression — parité avec le
  // badge « actifs » du lot (agrégat backend : en ligne = consommé) : les
  // tickets utilisés, expirés, désactivés ou en session ne sortent PAS.
  // Pagination obligatoire : /api/vouchers plafonne pageSize à 200.
  async function fetchActiveBatchVouchers(batchId: string): Promise<PagedUsers> {
    const pageSize = 200;
    const first = await api<PagedUsers>("/api/vouchers", {
      params: { search: batchId, page: 1, pageSize, status: "active" },
    });
    const pages = Math.ceil(first.total / pageSize);
    for (let p = 2; p <= pages; p++) {
      const next = await api<PagedUsers>("/api/vouchers", {
        params: { search: batchId, page: p, pageSize, status: "active" },
      });
      first.data.push(...next.data);
    }
    return first;
  }

  // « Imprimer le lot » (liste simple) : seuls les tickets ACTIFS du lot
  // sont imprimés (utilisés/expirés/désactivés exclus).
  async function printBatch(batch: BatchWithStats) {
    setPrintingBatchId(batch.id);
    try {
      const res = await fetchActiveBatchVouchers(batch.id);
      if (res.total === 0) {
        // Lot purgé (plus aucun voucher en base) ou tous consommés — on distingue.
        const all = await api<PagedUsers>("/api/vouchers", {
          params: { search: batch.id, page: 1, pageSize: 1 },
        });
        if (all.total === 0) {
          toast.info(tf("vouchers.batches.purgedToast", { batch: shortBatch(batch.id) }));
        } else {
          toast.info(tf("vouchers.batches.noActiveToast", { batch: shortBatch(batch.id) }));
        }
        return;
      }
      setPrintVouchers(res.data);
      setPrintTitle(
        tf("vouchers.batches.printTitleActive", { batch: shortBatch(batch.id), n: res.total }),
      );
      setPrintBatchId(batch.id);
      setPrintOpen(true);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t("vouchers.printFailed"));
    } finally {
      setPrintingBatchId(null);
    }
  }

  // Quick print (F12) : réimprime directement le dernier lot imprimé
  // (localStorage "mikcloud-last-batch") — tickets ACTIFS uniquement.
  // Aucun lot récent → toast informatif.
  async function quickPrint() {
    let lastBatch: string | null = null;
    try {
      lastBatch = window.localStorage.getItem(LAST_BATCH_STORAGE_KEY);
    } catch {
      /* localStorage indisponible */
    }
    if (!lastBatch) {
      toast.info(t("vouchers.quickPrintNone"));
      return;
    }
    setPrintingBatchId(lastBatch);
    try {
      // Même mécanique que printBatch : recherche par identifiant de lot,
      // filtre actifs côté serveur + pagination (plafond 200/page).
      const res = await fetchActiveBatchVouchers(lastBatch);
      if (res.total === 0) {
        const all = await api<PagedUsers>("/api/vouchers", {
          params: { search: lastBatch, page: 1, pageSize: 1 },
        });
        if (all.total === 0) {
          toast.info(tf("vouchers.batches.purgedToast", { batch: shortBatch(lastBatch) }));
        } else {
          toast.info(tf("vouchers.batches.noActiveToast", { batch: shortBatch(lastBatch) }));
        }
        return;
      }
      setPrintVouchers(res.data);
      setPrintTitle(
        tf("vouchers.batches.printTitleActive", { batch: shortBatch(lastBatch), n: res.total }),
      );
      setPrintBatchId(lastBatch);
      setPrintOpen(true);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t("vouchers.printFailed"));
    } finally {
      setPrintingBatchId(null);
    }
  }

  // N°18 — « Chez : » — possession live du stock vendable, affichée seulement
  // quand elle diverge de la provenance du lot (génération).
  function holdingsDiverge(batch: BatchWithStats): boolean {
    const holdings = batch.holdings ?? [];
    if (holdings.length === 0) return false;
    if (holdings.length > 1) return true;
    return batch.channel === "reseller" ? holdings[0].resellerId !== batch.resellerId : holdings[0].resellerId !== "";
  }

  function holdingsLabel(batch: BatchWithStats): string {
    return (batch.holdings ?? [])
      .map((h) => `${h.resellerId === "" ? t("vouchers.batches.holdingsDirect") : h.name || h.resellerId} (${h.count})`)
      .join(" · ");
  }

  // Refonte (F3) — impression multi-formats du lot (grille A4 / thermique
  // 58/80 mm) : mêmes règles que printBatch — seuls les tickets ACTIFS sortent
  // (utilisés, expirés, désactivés ou en session exclus).
  async function printMulti(batch: BatchWithStats) {
    setMultiPrintBatch(batch);
    setMultiPrintVouchers(null);
    try {
      const res = await fetchActiveBatchVouchers(batch.id);
      if (res.total === 0) {
        // Lot purgé (plus aucun voucher en base) ou tous consommés — on distingue.
        const all = await api<PagedUsers>("/api/vouchers", {
          params: { search: batch.id, page: 1, pageSize: 1 },
        });
        if (all.total === 0) {
          toast.info(tf("vouchers.batches.purgedToast", { batch: shortBatch(batch.id) }));
        } else {
          toast.info(tf("vouchers.batches.noActiveToast", { batch: shortBatch(batch.id) }));
        }
        setMultiPrintBatch(null);
        return;
      }
      setMultiPrintVouchers(res.data);
    } catch (error) {
      setMultiPrintBatch(null);
      toast.error(error instanceof Error ? error.message : t("vouchers.printFailed"));
    }
  }

  // Refonte (F3) — export CSV des lots : mêmes filtres que la liste, sans
  // pagination (endpoint dédié ; séparateur « ; » + BOM Excel côté back).
  async function exportBatches() {
    setExportingBatches(true);
    try {
      await apiDownload("/api/vouchers/batches/export", "mikcloud-lots.csv", {
        search: batchSearch || undefined,
        routerId: batchRouterParam,
        status: batchStatusParam,
        holder: batchHolderParam,
        channel: batchChannelParam,
      });
      toast.success(t("common.exportDownloaded"));
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t("common.exportFailed"));
    } finally {
      setExportingBatches(false);
    }
  }

  function resetBatchFilters() {
    setBatchSearchInput("");
    setBatchSearch("");
    setBatchRouterFilter("all");
    setBatchChannelFilter("all");
    setBatchStatusFilter("all");
    setBatchHolderFilter("all");
    setBatchPage(1);
  }

  // Fiche 360° — actions déléguées aux mécanismes existants (la fiche se ferme).
  function detailToTransfer(batch: BatchWithStats) {
    setDetailBatch(null);
    setTransferBatch(batch);
  }
  function detailToPrintMulti(batch: BatchWithStats) {
    setDetailBatch(null);
    void printMulti(batch);
  }
  function detailToPrintSimple(batch: BatchWithStats) {
    setDetailBatch(null);
    void printBatch(batch);
  }
  function detailToView(batch: BatchWithStats) {
    setDetailBatch(null);
    viewBatchVouchers(batch);
  }
  function detailToDelete(batch: BatchWithStats) {
    setDetailBatch(null);
    setDeletingBatch(batch);
  }

  // Refonte — actions 1ᵉʳ niveau (découvrabilité) : impression multi-formats et
  // transfert, déclinées compacte (cartes mobiles) / étendue (table).
  function printMultiButton(batch: BatchWithStats, iconOnly = false) {
    return (
      <Button
        variant="outline"
        size={iconOnly ? "icon" : "sm"}
        className={iconOnly ? "size-10" : "h-10 gap-1.5 px-2.5"}
        disabled={multiPrintBatch?.id === batch.id}
        onClick={() => void printMulti(batch)}
        aria-label={tf("vouchers.batches.printMultiTitle", { batch: shortBatch(batch.id) })}
        title={t("vouchers.batches.detail.printMulti")}
      >
        {multiPrintBatch?.id === batch.id ? (
          <Loader2 className="size-4 animate-spin" />
        ) : (
          <Printer className="size-4" />
        )}
        {!iconOnly && <span className="hidden md:inline">{t("vouchers.batches.detail.printMulti")}</span>}
      </Button>
    );
  }

  function transferButton(batch: BatchWithStats, iconOnly = false) {
    if (batch.transferable === 0) return null;
    return (
      <Button
        variant="outline"
        size={iconOnly ? "icon" : "sm"}
        className={iconOnly ? "size-10" : "h-10 gap-1.5 px-2.5"}
        onClick={() => setTransferBatch(batch)}
        aria-label={tf("vouchers.batches.transferAria", { batch: batch.id })}
        title={t("vouchers.batches.transfer")}
      >
        <ArrowLeftRight className="size-4" />
        {!iconOnly && <span className="hidden md:inline">{t("vouchers.batches.transfer")}</span>}
      </Button>
    );
  }

  // Refonte — menu ⋯ du lot : la fiche 360° d'abord, puis toutes les actions.
  function batchActionsMenu(batch: BatchWithStats) {
    return (
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            variant="ghost"
            size="icon"
            className="size-10 text-muted-foreground hover:text-foreground"
            aria-label={tf("vouchers.batches.actionsFor", { batch: batch.id })}
          >
            <MoreHorizontal className="size-4" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-60">
          <DropdownMenuItem className="min-h-10" onClick={() => setDetailBatch(batch)}>
            <Info className="size-4" />
            {tf("vouchers.batches.detailOpen", { batch: `#${shortBatch(batch.id)}` })}
          </DropdownMenuItem>
          <DropdownMenuItem className="min-h-10" onClick={() => viewBatchVouchers(batch)}>
            <Eye className="size-4" />
            {t("vouchers.batches.view")}
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem
            className="min-h-10"
            disabled={multiPrintBatch?.id === batch.id}
            onClick={() => void printMulti(batch)}
          >
            {multiPrintBatch?.id === batch.id ? (
              <Loader2 className="size-4 animate-spin" />
            ) : (
              <Printer className="size-4" />
            )}
            {t("vouchers.batches.detail.printMulti")}
          </DropdownMenuItem>
          <DropdownMenuItem
            className="min-h-10"
            disabled={printingBatchId === batch.id}
            onClick={() => void printBatch(batch)}
          >
            {printingBatchId === batch.id ? (
              <Loader2 className="size-4 animate-spin" />
            ) : (
              <Ticket className="size-4" />
            )}
            {t("vouchers.batches.printSimple")}
          </DropdownMenuItem>
          <DropdownMenuItem
            className="min-h-10"
            disabled={batch.transferable === 0}
            onClick={() => setTransferBatch(batch)}
          >
            <ArrowLeftRight className="size-4" />
            {t("vouchers.batches.transfer")}
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem variant="destructive" className="min-h-10" onClick={() => setDeletingBatch(batch)}>
            <Trash2 className="size-4" />
            {t("common.deleteBatch")}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    );
  }

  const hasFilters = search !== "" || statusFilter !== "all" || profileFilter !== "all";
  // Refonte — filtres « fiche de vie » des lots (recherche + site + canal + cycle + détenteur).
  const hasBatchFilters =
    batchSearch !== "" ||
    batchRouterFilter !== "all" ||
    batchChannelFilter !== "all" ||
    batchStatusFilter !== "all" ||
    batchHolderFilter !== "all";

  return (
    <div className="space-y-4 sm:space-y-6">
      <PageHeader
        title={t("vouchers.title")}
        description={t("vouchers.description")}
        actions={
          <>
            <Button variant="outline" className="h-10" onClick={() => void quickPrint()}>
              <Zap className="size-4" />
              {t("vouchers.quickPrint")}
            </Button>
            <Button className="h-10" onClick={() => setGenOpen(true)}>
              <TicketPlus className="size-4" />
              {t("vouchers.generate")}
            </Button>
          </>
        }
      />

      {/* Onglets Vouchers / Lots (traçabilité) */}
      <Tabs value={tab} onValueChange={(value) => setTab(value as "vouchers" | "batches")}>
        <TabsList>
          <TabsTrigger value="vouchers" className="gap-1.5">
            <Ticket className="size-3.5" aria-hidden />
            {t("vouchers.tabVouchers")}
          </TabsTrigger>
          <TabsTrigger value="batches" className="gap-1.5">
            <Layers className="size-3.5" aria-hidden />
            {t("vouchers.tabBatches")}
          </TabsTrigger>
        </TabsList>
      </Tabs>

      {tab === "vouchers" && (
        <>
      {/* Statistiques du stock */}
      {statsLoading ? (
        <LoadingCards cards={4} />
      ) : (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
          <StatCard title={t("vouchers.kpi.active")} value={String(activeCount)} sub={t("vouchers.kpi.activeSub")} icon={Ticket} />
          <StatCard title={t("vouchers.kpi.used")} value={String(usedCount)} sub={t("vouchers.kpi.usedSub")} icon={CheckCircle2} />
          <StatCard title={t("vouchers.kpi.expired")} value={String(expiredCount)} sub={t("vouchers.kpi.expiredSub")} icon={Clock} />
          <StatCard
            title={t("vouchers.kpi.stockValue")}
            value={formatCurrency(stockValue, currency, lang)}
            sub={t("vouchers.kpi.stockValueSub")}
            icon={Wallet}
          />
        </div>
      )}

      {/* Barre de filtres */}
      <Card className="gap-0 py-0">
        <CardContent className="flex flex-col gap-3 p-4 sm:flex-row sm:items-center">
          <div className="relative w-full sm:max-w-xs sm:flex-1">
            <Search className="absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" aria-hidden />
            <Input
              className="h-10 pl-9"
              placeholder={t("vouchers.searchPlaceholder")}
              value={searchInput}
              onChange={(event) => setSearchInput(event.target.value)}
              aria-label={t("vouchers.searchLabel")}
            />
          </div>
          <div className="flex flex-1 flex-wrap gap-3 sm:justify-end">
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
          </div>
        </CardContent>
      </Card>

      {/* Table des vouchers */}
      <Card className="gap-0 py-0">
        {isLoading ? (
          <LoadingRows rows={8} />
        ) : vouchers.length === 0 ? (
          <EmptyState
            icon={Ticket}
            title={t("vouchers.empty")}
            description={hasFilters ? t("vouchers.emptyFiltered") : t("vouchers.emptyDesc")}
            action={
              !hasFilters && (
                <Button onClick={() => setGenOpen(true)}>
                  <TicketPlus className="size-4" />
                  {t("vouchers.generate")}
                </Button>
              )
            }
          />
        ) : (
          <>
            <div className="overflow-x-auto">
              <Table>
                <TableHeader>
                  <TableRow className="hover:bg-transparent">
                    <TableHead className="pl-4 text-muted-foreground sm:pl-6">{t("vouchers.code")}</TableHead>
                    <TableHead className="text-muted-foreground">{t("common.password")}</TableHead>
                    <TableHead className="text-muted-foreground">{t("common.profile")}</TableHead>
                    <TableHead className="hidden text-muted-foreground md:table-cell">{t("vouchers.quota")}</TableHead>
                    <TableHead className="text-right text-muted-foreground">{t("common.price")}</TableHead>
                    <TableHead className="text-muted-foreground">{t("common.status")}</TableHead>
                    <TableHead className="hidden text-muted-foreground md:table-cell">{t("common.reseller")}</TableHead>
                    <TableHead className="hidden text-muted-foreground md:table-cell">{t("common.batch")}</TableHead>
                    <TableHead className="hidden text-muted-foreground lg:table-cell">{t("common.created")}</TableHead>
                    <TableHead className="pr-4 text-right text-muted-foreground sm:pr-6">{t("common.actions")}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {vouchers.map((voucher) => (
                    <TableRow key={voucher.id}>
                      <TableCell className="pl-4 sm:pl-6">
                        <span className="inline-flex items-center gap-0.5">
                          <span className="font-mono text-sm font-medium">{voucher.username}</span>
                          <Button
                            variant="ghost"
                            size="icon"
                            className="size-9 text-muted-foreground hover:text-foreground"
                            onClick={() => void copyCode(voucher)}
                            aria-label={tf("vouchers.copyCodeAria", { code: voucher.username })}
                            title={t("vouchers.copyCode")}
                          >
                            <Copy className="size-4" />
                          </Button>
                        </span>
                      </TableCell>
                      <TableCell>
                        <PasswordCell
                          password={voucher.password}
                          visible={revealed.has(voucher.id)}
                          onToggle={() => toggleReveal(voucher.id)}
                          label={
                            revealed.has(voucher.id)
                              ? tf("vouchers.hidePassword", { name: voucher.username })
                              : tf("vouchers.showPassword", { name: voucher.username })
                          }
                        />
                      </TableCell>
                      <TableCell>
                        <Badge variant="outline" className="max-w-36 truncate">
                          {voucher.profileName}
                        </Badge>
                      </TableCell>
                      <TableCell className="hidden md:table-cell">
                        {voucher.dataQuotaMb > 0 ? (
                          <span className="text-sm tabular-nums">
                            {formatBytes(voucher.dataQuotaMb * 1048576, lang)}
                          </span>
                        ) : (
                          <span className="text-sm text-muted-foreground">—</span>
                        )}
                      </TableCell>
                      <TableCell className="text-right font-medium tabular-nums">
                        {formatCurrency(voucher.price, currency, lang)}
                      </TableCell>
                      <TableCell>
                        <div className="flex flex-wrap items-center gap-1.5">
                          <StatusBadge status={voucher.status} dot />
                          {/* N — rapprochement doux : badge « absent du routeur ». */}
                          {voucher.missingOnRouter && (
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
                      <TableCell className="hidden max-w-40 truncate text-muted-foreground md:table-cell">
                        {voucher.resellerName || "—"}
                      </TableCell>
                      <TableCell className="hidden md:table-cell">
                        {voucher.batchId ? (
                          <button
                            type="button"
                            className="font-mono text-xs text-muted-foreground underline-offset-2 hover:text-foreground hover:underline"
                            onClick={() => {
                              setSearchInput(voucher.batchId);
                              setSearch(voucher.batchId);
                              setStatusFilter("all");
                              setProfileFilter("all");
                              setPage(1);
                            }}
                            title={tf("vouchers.batches.filterBatch", { batch: voucher.batchId })}
                          >
                            #{shortBatch(voucher.batchId)}
                          </button>
                        ) : (
                          "—"
                        )}
                      </TableCell>
                      <TableCell className="hidden tabular-nums text-muted-foreground lg:table-cell">
                        {formatDate(voucher.createdAt, lang)}
                      </TableCell>
                      <TableCell className="pr-4 text-right sm:pr-6">
                        <DropdownMenu>
                          <DropdownMenuTrigger asChild>
                            <Button
                              variant="ghost"
                              size="icon"
                              className="size-10 text-muted-foreground hover:text-foreground"
                              aria-label={tf("common.actionsFor", { name: voucher.username })}
                            >
                              <MoreHorizontal className="size-4" />
                            </Button>
                          </DropdownMenuTrigger>
                          <DropdownMenuContent align="end" className="w-56">
                            <DropdownMenuItem className="min-h-10" onClick={() => printOne(voucher)}>
                              <Printer className="size-4" />
                              {t("vouchers.printOne")}
                            </DropdownMenuItem>
                            {/* N — resynchronisation (uniquement si absent du routeur). */}
                            {voucher.missingOnRouter && (
                              <>
                                <DropdownMenuSeparator />
                                <DropdownMenuItem
                                  className="min-h-10"
                                  disabled={resyncMutation.isPending && resyncMutation.variables?.voucher.id === voucher.id}
                                  onClick={() => resyncMutation.mutate({ voucher, action: "recreate" })}
                                >
                                  {resyncMutation.isPending && resyncMutation.variables?.voucher.id === voucher.id ? (
                                    <Loader2 className="size-4 animate-spin" />
                                  ) : (
                                    <RefreshCcw className="size-4" />
                                  )}
                                  {t("users.resyncRecreate")}
                                </DropdownMenuItem>
                                <DropdownMenuItem
                                  className="min-h-10"
                                  disabled={resyncMutation.isPending && resyncMutation.variables?.voucher.id === voucher.id}
                                  onClick={() => {
                                    if (window.confirm(t("users.resyncForgetConfirm"))) {
                                      resyncMutation.mutate({ voucher, action: "forget" });
                                    }
                                  }}
                                >
                                  <Trash2 className="size-4" />
                                  {t("users.resyncForget")}
                                </DropdownMenuItem>
                              </>
                            )}
                            <DropdownMenuSeparator />
                            <DropdownMenuItem
                              variant="destructive"
                              className="min-h-10"
                              onClick={() => setDeleting(voucher)}
                            >
                              <Trash2 className="size-4" />
                              {t("common.delete")}
                            </DropdownMenuItem>
                          </DropdownMenuContent>
                        </DropdownMenu>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>

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
        </>
      )}

      {/* ----------------------------------------------------------------- */}
      {/* Onglet Lots — traçabilité complète des générations de vouchers     */}
      {/* ----------------------------------------------------------------- */}
      {tab === "batches" && (
        <>
          {/* Refonte — bande KPI « fiche de vie » : la vie du stock en 4 chiffres
              (totaux sur l'ensemble FILTRÉ — filtrez par détenteur, lisez son stock). */}
          {batchesLoading && !batchData ? (
            <LoadingCards cards={4} />
          ) : (
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
              <StatCard
                title={t("vouchers.batches.kpi.batches")}
                value={String(batchKpi?.batches ?? batchTotal)}
                sub={t("vouchers.batches.kpi.batchesSub")}
                icon={Layers}
              />
              <StatCard
                title={t("vouchers.batches.kpi.stock")}
                value={String(batchKpi?.stockTickets ?? 0)}
                sub={t("vouchers.batches.kpi.stockSub")}
                icon={Ticket}
                valueClassName="text-primary"
              />
              <StatCard
                title={t("vouchers.batches.kpi.value")}
                value={formatCurrency(batchKpi?.stockValue ?? 0, currency, lang)}
                sub={t("vouchers.batches.kpi.valueSub")}
                icon={Wallet}
              />
              <StatCard
                title={t("vouchers.batches.kpi.expiring")}
                value={String(batchKpi?.expiring7d ?? 0)}
                sub={t("vouchers.batches.kpi.expiringSub")}
                icon={Clock}
                valueClassName={cn(batchKpi?.expiring7d ? "text-amber-600 dark:text-amber-400" : undefined)}
              />
            </div>
          )}

          {/* Filtres des lots — recherche, site, canal, cycle de vie, détenteur + export */}
          <Card className="gap-0 py-0">
            <CardContent className="flex flex-col gap-3 p-4 sm:flex-row sm:items-center">
              <div className="relative w-full sm:max-w-xs sm:flex-1">
                <Search className="absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" aria-hidden />
                <Input
                  className="h-10 pl-9"
                  placeholder={t("vouchers.batches.searchPlaceholder")}
                  value={batchSearchInput}
                  onChange={(event) => setBatchSearchInput(event.target.value)}
                  aria-label={t("vouchers.batches.searchLabel")}
                />
              </div>
              <div className="flex flex-1 flex-wrap items-center gap-3 sm:justify-end">
                <Select
                  value={batchRouterFilter}
                  onValueChange={(value) => {
                    setBatchRouterFilter(value);
                    setBatchPage(1);
                  }}
                >
                  <SelectTrigger className="h-10 w-full sm:w-44" aria-label={t("vouchers.batches.siteFilter")}>
                    <SelectValue placeholder={t("vouchers.batches.siteFilterLabel")} />
                  </SelectTrigger>
                  <SelectContent className="max-h-72">
                    <SelectItem value="all">{t("common.allSites")}</SelectItem>
                    {routers?.map((router) => (
                      <SelectItem key={router.id} value={router.id}>
                        {router.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <Select
                  value={batchChannelFilter}
                  onValueChange={(value) => {
                    setBatchChannelFilter(value);
                    setBatchPage(1);
                  }}
                >
                  <SelectTrigger className="h-10 w-full sm:w-40" aria-label={t("vouchers.batches.filterChannel")}>
                    <SelectValue placeholder={t("vouchers.batches.channelAll")} />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="all">{t("vouchers.batches.channelAll")}</SelectItem>
                    <SelectItem value="direct">{t("common.direct")}</SelectItem>
                    <SelectItem value="reseller">{t("common.reseller")}</SelectItem>
                  </SelectContent>
                </Select>
                <Select
                  value={batchStatusFilter}
                  onValueChange={(value) => {
                    setBatchStatusFilter(value);
                    setBatchPage(1);
                  }}
                >
                  <SelectTrigger className="h-10 w-full sm:w-40" aria-label={t("vouchers.batches.filterStatus")}>
                    <SelectValue placeholder={t("vouchers.batches.lifeStatus")} />
                  </SelectTrigger>
                  <SelectContent>
                    {BATCH_STATUS_OPTIONS.map((option) => (
                      <SelectItem key={option.value} value={option.value}>
                        {t(option.labelKey)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <Select
                  value={batchHolderFilter}
                  onValueChange={(value) => {
                    setBatchHolderFilter(value);
                    setBatchPage(1);
                  }}
                >
                  <SelectTrigger className="h-10 w-full sm:w-48" aria-label={t("vouchers.batches.filterHolder")}>
                    <SelectValue placeholder={t("vouchers.batches.holderAll")} />
                  </SelectTrigger>
                  <SelectContent className="max-h-72">
                    <SelectItem value="all">{t("vouchers.batches.holderAll")}</SelectItem>
                    <SelectItem value="direct">{t("vouchers.batches.holderDirect")}</SelectItem>
                    {resellers?.map((reseller) => (
                      <SelectItem key={reseller.id} value={reseller.id}>
                        {reseller.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <Button
                  variant="outline"
                  className="h-10 shrink-0"
                  onClick={() => void exportBatches()}
                  disabled={exportingBatches}
                  title={t("vouchers.batches.exportCsv")}
                  aria-label={t("vouchers.batches.exportCsv")}
                >
                  {exportingBatches ? <Loader2 className="size-4 animate-spin" /> : <Download className="size-4" />}
                  <span className="hidden xl:inline">{t("vouchers.batches.exportCsv")}</span>
                </Button>
                {hasBatchFilters && (
                  <Button variant="ghost" className="h-10 shrink-0" onClick={resetBatchFilters}>
                    <RefreshCcw className="size-4" />
                    <span className="hidden lg:inline">{t("vouchers.batches.resetFilters")}</span>
                    <span className="sr-only lg:hidden">{t("vouchers.batches.resetFilters")}</span>
                  </Button>
                )}
              </div>
            </CardContent>
          </Card>

          {/* Table des lots */}
          <Card className="gap-0 py-0">
            {batchesLoading && !batchData ? (
              <LoadingRows rows={8} />
            ) : batches.length === 0 ? (
              <EmptyState
                icon={Layers}
                title={t("vouchers.batches.empty")}
                description={
                  hasBatchFilters ? t("vouchers.batches.emptyFiltered") : t("vouchers.emptyDesc")
                }
                action={
                  !hasBatchFilters && (
                    <Button onClick={() => setGenOpen(true)}>
                      <TicketPlus className="size-4" />
                      {t("vouchers.generate")}
                    </Button>
                  )
                }
              />
            ) : (
              <>
                {/* Cartes mobiles (< md) — chaque lot est une fiche tappable */}
                <div className="md:hidden">
                  {batches.map((batch) => (
                    <div
                      key={batch.id}
                      role="button"
                      tabIndex={0}
                      aria-label={tf("vouchers.batches.detailOpen", { batch: batch.id })}
                      className="cursor-pointer space-y-3 border-b p-4 transition-colors last:border-b-0 hover:bg-muted/40 focus-visible:bg-muted/40 focus-visible:outline-none"
                      onClick={() => setDetailBatch(batch)}
                      onKeyDown={(event) => {
                        if (event.key === "Enter" || event.key === " ") {
                          event.preventDefault();
                          setDetailBatch(batch);
                        }
                      }}
                    >
                      <div className="flex flex-wrap items-center justify-between gap-2">
                        <span className="inline-flex items-center gap-1.5">
                          <span className="font-mono text-sm font-medium">#{shortBatch(batch.id)}</span>
                          <BatchLifeBadge status={batch.status} />
                        </span>
                        <span className="text-xs text-muted-foreground">{formatDate(batch.createdAt, lang)}</span>
                      </div>
                      <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
                        <Badge variant="outline" className="max-w-36 truncate">
                          {batch.profileName}
                        </Badge>
                        <span className="truncate text-xs text-muted-foreground">{batch.routerName}</span>
                        <span className="text-xs text-muted-foreground">
                          · {batch.channel === "reseller" ? batch.resellerName || t("common.reseller") : t("common.direct")}
                        </span>
                      </div>
                      <BatchLifeBar
                        count={batch.count}
                        active={batch.active}
                        used={batch.used}
                        expired={batch.expired}
                        disabled={batch.disabled}
                      />
                      <div className="flex items-center justify-between gap-2" onClick={(event) => event.stopPropagation()}>
                        <span className="text-sm font-medium tabular-nums text-muted-foreground">
                          {formatCurrency(batch.totalCost, currency, lang)}
                        </span>
                        <div className="flex items-center gap-1.5">
                          {transferButton(batch, true)}
                          {printMultiButton(batch, true)}
                          {batchActionsMenu(batch)}
                        </div>
                      </div>
                    </div>
                  ))}
                </div>

                {/* Table (md+) */}
                <div className="hidden overflow-x-auto md:block">
                  <Table>
                    <TableHeader>
                      <TableRow className="hover:bg-transparent">
                        <TableHead className="pl-4 text-muted-foreground sm:pl-6">{t("common.batch")}</TableHead>
                        <TableHead className="text-muted-foreground">{t("common.profile")}</TableHead>
                        <TableHead className="hidden text-muted-foreground md:table-cell">{t("vouchers.batches.site")}</TableHead>
                        <TableHead className="hidden text-muted-foreground xl:table-cell">{t("vouchers.batches.channel")}</TableHead>
                        <TableHead className="hidden min-w-44 text-muted-foreground sm:table-cell">{t("vouchers.batches.life")}</TableHead>
                        <TableHead className="hidden text-right text-muted-foreground md:table-cell">{t("vouchers.batches.cost")}</TableHead>
                        <TableHead className="pr-4 text-right text-muted-foreground sm:pr-6">{t("common.actions")}</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {batches.map((batch) => (
                        <TableRow
                          key={batch.id}
                          className="cursor-pointer"
                          onClick={() => setDetailBatch(batch)}
                        >
                          <TableCell className="pl-4 sm:pl-6">
                            <button
                              type="button"
                              className="flex flex-col items-start gap-0.5 text-left"
                              onClick={(event) => {
                                event.stopPropagation();
                                setDetailBatch(batch);
                              }}
                              aria-label={tf("vouchers.batches.detailOpen", { batch: batch.id })}
                            >
                              <span className="inline-flex items-center gap-1.5">
                                <span className="font-mono text-sm font-medium underline-offset-2 hover:underline">
                                  #{shortBatch(batch.id)}
                                </span>
                                <BatchLifeBadge status={batch.status} />
                              </span>
                              <span className="text-xs text-muted-foreground">{formatDate(batch.createdAt, lang)}</span>
                            </button>
                          </TableCell>
                          <TableCell>
                            <Badge variant="outline" className="max-w-36 truncate">
                              {batch.profileName}
                            </Badge>
                            {batch.dataQuotaMb > 0 && (
                              <p className="mt-0.5 text-xs text-muted-foreground">
                                {formatBytes(batch.dataQuotaMb * 1048576, lang)} / voucher
                              </p>
                            )}
                          </TableCell>
                          <TableCell className="hidden max-w-40 truncate text-muted-foreground md:table-cell">
                            {batch.routerName}
                          </TableCell>
                          <TableCell className="hidden xl:table-cell">
                            {batch.channel === "reseller" ? (
                              <span className="text-muted-foreground">
                                {t("common.reseller")} · <span className="text-foreground">{batch.resellerName || "—"}</span>
                              </span>
                            ) : (
                              <span className="text-muted-foreground">{t("common.direct")}</span>
                            )}
                            {holdingsDiverge(batch) && (
                              <p className="mt-0.5 text-xs text-muted-foreground">
                                {t("vouchers.batches.heldBy")} {holdingsLabel(batch)}
                              </p>
                            )}
                          </TableCell>
                          {/* Refonte — la vie du lot en un coup d'œil (composition + légende) */}
                          <TableCell>
                            <BatchLifeBar
                              count={batch.count}
                              active={batch.active}
                              used={batch.used}
                              expired={batch.expired}
                              disabled={batch.disabled}
                              className="w-44 max-w-56"
                            />
                          </TableCell>
                          <TableCell className="hidden text-right tabular-nums text-muted-foreground md:table-cell">
                            {formatCurrency(batch.totalCost, currency, lang)}
                          </TableCell>
                          <TableCell className="pr-4 text-right sm:pr-6" onClick={(event) => event.stopPropagation()}>
                            <div className="flex items-center justify-end gap-1.5">
                              {transferButton(batch)}
                              {printMultiButton(batch)}
                              {batchActionsMenu(batch)}
                            </div>
                          </TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </div>

                {/* Pagination des lots */}
                <div className="flex flex-wrap items-center justify-between gap-3 border-t px-4 py-3 sm:px-6">
                  <p className="text-xs text-muted-foreground">
                    {batchesFetching
                      ? t("common.refreshing")
                      : tf("vouchers.batches.range", { start: batchRangeStart, end: batchRangeEnd, total: batchTotal })}
                  </p>
                  <div className="flex items-center gap-2">
                    <Button
                      variant="outline"
                      size="sm"
                      className="h-10"
                      onClick={() => setBatchPage((p) => Math.max(1, p - 1))}
                      disabled={batchSafePage <= 1}
                    >
                      <ChevronLeft className="size-4" />
                      {t("common.previous")}
                    </Button>
                    <Button
                      variant="outline"
                      size="sm"
                      className="h-10"
                      onClick={() => setBatchPage((p) => Math.min(batchMaxPage, p + 1))}
                      disabled={batchSafePage >= batchMaxPage}
                    >
                      {t("common.next")}
                      <ChevronRight className="size-4" />
                    </Button>
                  </div>
                </div>
              </>
            )}
          </Card>
        </>
      )}

      {/* Wizard de génération — 3 étapes (Forfait → Codes → Récap) */}
      <VoucherWizardDialog
        open={genOpen}
        onOpenChange={setGenOpen}
        profiles={profiles ?? []}
        routers={routers ?? []}
        resellers={resellers ?? []}
        currency={currency}
        tenantName={tenantName}
        isPending={generateMutation.isPending}
        isError={generateMutation.isError}
        error={generateMutation.error}
        onSubmit={(payload) => generateMutation.mutate(payload)}
      />

      {/* Dialogue d'impression (mode modèle F2 si des templates existent) */}
      <UcPrintDialog
        open={printOpen}
        onOpenChange={setPrintOpen}
        vouchers={printVouchers}
        title={printTitle}
        tenantName={tenantName}
        profiles={profiles ?? []}
        templates={templates ?? []}
        batchId={printBatchId}
      />

      {/* N°18 — Transfert de stock du lot (distribution / retour de stock) */}
      <VoucherTransferDialog
        key={transferBatch?.id ?? "transfer-closed"}
        batch={transferBatch}
        resellers={resellers ?? []}
        currency={currency}
        onOpenChange={(open) => !open && setTransferBatch(null)}
      />

      {/* Refonte — fiche 360° du lot (drawer) : cycle de vie, possession, actions */}
      <BatchDetailSheet
        batch={detailBatch}
        currency={currency}
        onOpenChange={(open) => !open && setDetailBatch(null)}
        onTransfer={detailToTransfer}
        onPrintMulti={detailToPrintMulti}
        onPrintSimple={detailToPrintSimple}
        onViewVouchers={detailToView}
        onDelete={detailToDelete}
      />

      {/* Refonte (F3) — impression multi-formats du lot : grille A4 / thermique 58/80 */}
      <BatchPrintDialog
        open={multiPrintBatch !== null}
        onOpenChange={(open) => {
          if (!open) {
            setMultiPrintBatch(null);
            setMultiPrintVouchers(null);
          }
        }}
        vouchers={multiPrintVouchers}
        title={tf("vouchers.batches.printMultiTitle", { batch: multiPrintBatch ? shortBatch(multiPrintBatch.id) : "" })}
        tenantName={tenantName}
        currency={currency}
      />

      {/* Confirmation suppression voucher */}
      <AlertDialog open={deleting !== null} onOpenChange={(open) => !open && setDeleting(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{tf("vouchers.deleteTitle", { code: deleting?.username ?? "" })}</AlertDialogTitle>
            <AlertDialogDescription>{t("vouchers.deleteDesc")}</AlertDialogDescription>
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

      {/* Confirmation suppression d'un lot entier */}
      <AlertDialog open={deletingBatch !== null} onOpenChange={(open) => !open && setDeletingBatch(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {tf("vouchers.batches.deleteTitle", {
                batch: deletingBatch ? shortBatch(deletingBatch.id) : "",
              })}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {deletingBatch &&
                tf("vouchers.batches.deleteDesc", {
                  n: deletingBatch.count,
                  profile: deletingBatch.profileName,
                  router: deletingBatch.routerName,
                })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={batchDeleteMutation.isPending}>{t("common.cancel")}</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-white hover:bg-destructive/90"
              disabled={batchDeleteMutation.isPending}
              onClick={(event) => {
                event.preventDefault();
                if (deletingBatch) batchDeleteMutation.mutate(deletingBatch);
              }}
            >
              {batchDeleteMutation.isPending && <Loader2 className="size-4 animate-spin" />}
              {t("common.deleteBatch")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
