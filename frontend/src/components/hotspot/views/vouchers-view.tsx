"use client";

// Vue Vouchers (kind=voucher) — générateur en série, impression de tickets,
// suivi des statuts et traçabilité des lots (onglet Lots).
// Quick print (F12) : bouton « Réimpression rapide » qui réimprime le dernier
// lot imprimé (localStorage "mikcloud-last-batch", écrit par uc-print-dialog).

import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  CheckCircle2,
  ChevronLeft,
  ChevronRight,
  Clock,
  Copy,
  Eye,
  Gauge,
  Layers,
  Loader2,
  MoreHorizontal,
  Printer,
  QrCode,
  RefreshCcw,
  Search,
  ShieldQuestion,
  SlidersHorizontal,
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
import { VoucherA4PrintDialog } from "@/components/hotspot/parts/voucher-a4-print-dialog";
import { api } from "@/lib/hotspot/api";
import { useRouterResources } from "@/lib/hotspot/use-router-resources";
import { useI18n, localeOf } from "@/lib/hotspot/i18n";
import { formatBytes, formatCurrency, formatDate, formatDuration, fmtRouterDuration } from "@/lib/hotspot/format";
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
// 3 à 8 caractères comme Mikhmon (parité), jusqu'à 10 (alphabets sans
// caractères ambigus côté serveur).
const CODE_LENGTHS = [3, 4, 5, 6, 7, 8, 9, 10];

// Jeux de caractères — presets du User Manager MikroTik (« abcd », « ABCD »,
// « aBcD », « 5ab2c34d », « 5AB2C34D », « 5aB2c34D »). Le serveur exclut
// toujours 0/1/I/L/O : les codes restent lisibles sur un ticket imprimé.
const CHARSET_OPTIONS = [
  { value: "mikcloud", labelKey: "vouchers.gen.charset.mikcloud" },
  { value: "abc", labelKey: "vouchers.gen.charset.abc" },
  { value: "ABC", labelKey: "vouchers.gen.charset.ABC" },
  { value: "aBc", labelKey: "vouchers.gen.charset.aBc" },
  { value: "5ab", labelKey: "vouchers.gen.charset.5ab" },
  { value: "5AB", labelKey: "vouchers.gen.charset.5AB" },
  { value: "5aB", labelKey: "vouchers.gen.charset.5aB" },
  { value: "num", labelKey: "vouchers.gen.charset.num" },
];

// Quotas courants (1 Go = 1024 Mo) — l'argument « 5 Go = 500 F ».
// Le libellé (Mo/GB selon la langue) est construit au rendu via fmtQuota().
const QUOTA_OPTIONS = [
  { value: "0", mb: 0 },
  { value: "512", mb: 512 },
  { value: "1024", mb: 1024 },
  { value: "2048", mb: 2048 },
  { value: "5120", mb: 5120 },
  { value: "10240", mb: 10240 },
  { value: "20480", mb: 20480 },
  { value: "51200", mb: 51200 },
];

// Parité Mikhmon : libellés du mode d'expiration, réutilisés par le récap.
const EXP_MODE_LABEL_KEY: Record<string, string> = {
  none: "profiles.dialog.expNone",
  notify: "profiles.dialog.expNotify",
  remove: "profiles.dialog.expRemove",
};

// Parité Mikhmon : Time Limit (limit-uptime) par lot — quotas de temps
// courants en minutes, libellés au format RouterOS via fmtRouterDuration().
const TIME_LIMIT_OPTIONS = [
  { value: "0", min: 0 },
  { value: "30", min: 30 },
  { value: "60", min: 60 },
  { value: "120", min: 120 },
  { value: "180", min: 180 },
  { value: "300", min: 300 },
  { value: "720", min: 720 },
  { value: "1440", min: 1440 },
  { value: "2880", min: 2880 },
  { value: "4320", min: 4320 },
  { value: "10080", min: 10080 },
  { value: "43200", min: 43200 },
];

// Repères de prix FCFA pour vendre au quota (le prix réel reste celui du profil) :
// paires [quota en Mo, prix en F CFA] — libellés localisés au rendu.
const QUOTA_PRICE_HINTS: [number, number][] = [
  [1024, 100],
  [2048, 200],
  [5120, 500],
  [10240, 1000],
  [30720, 3000],
];

const STATUS_OPTIONS = [
  { value: "all", labelKey: "common.allStatuses" },
  { value: "active", labelKey: "common.statusActive" },
  { value: "used", labelKey: "common.statusUsed" },
  { value: "expired", labelKey: "common.statusExpired" },
  { value: "disabled", labelKey: "common.statusDisabled" },
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

  // Libellé localisé d'un quota : « 512 Mo » / « 5 GB » selon la langue.
  const fmtQuota = (mb: number) =>
    mb < 1024 ? tf("vouchers.gen.quota.mb", { n: mb }) : tf("vouchers.gen.quota.gb", { n: mb / 1024 });
  // Prix F CFA avec séparateur de milliers localisé (1 000 F / 1,000 F).
  const fmtFcfa = (amount: number) => `${new Intl.NumberFormat(localeOf(lang)).format(amount)} F`;

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

  // Générateur
  const [genOpen, setGenOpen] = useState(false);
  const [genCount, setGenCount] = useState("10");
  const [genProfileId, setGenProfileId] = useState("");
  const [genRouterId, setGenRouterId] = useState("");
  const [genPrefix, setGenPrefix] = useState("");
  const [genCodeLength, setGenCodeLength] = useState("6");
  const [genResellerId, setGenResellerId] = useState("none");
  // Génération avancée (style User Manager MikroTik) : mode utilisateur, jeu de
  // caractères, commentaire libre et quota data (« 5 Go = 500 F »).
  const [genUserMode, setGenUserMode] = useState<"userpass" | "same">("userpass");
  const [genCharset, setGenCharset] = useState("mikcloud");
  const [genComment, setGenComment] = useState("");
  const [genQuotaMb, setGenQuotaMb] = useState("inherit");
  // Parité Mikhmon : Time Limit (limit-uptime) par lot — "inherit" = profil.
  const [genTimeLimit, setGenTimeLimit] = useState("inherit");
  // Parité Mikhmon : serveur hotspot visé par le lot (« all » = omis au routeur).
  const [genServer, setGenServer] = useState("all");

  // Impression (liste simple — ancien dialog)
  const [printOpen, setPrintOpen] = useState(false);
  const [printVouchers, setPrintVouchers] = useState<HotspotUser[]>([]);
  const [printTitle, setPrintTitle] = useState("");
  /** Lot en cours d'impression (F12) — mémorisé à l'impression par le dialog. */
  const [printBatchId, setPrintBatchId] = useState<string | undefined>(undefined);

  // Impression A4 + QR (flux revendeur « imprimer → vendre »)
  const [a4Open, setA4Open] = useState(false);
  const [a4Vouchers, setA4Vouchers] = useState<HotspotUser[]>([]);
  const [a4Title, setA4Title] = useState("");
  const [a4HotspotUrl, setA4HotspotUrl] = useState<string | undefined>(undefined);

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
  // Parité Mikhmon : ressources du routeur sélectionné (serveurs hotspot) —
  // la liste « Server » du générateur Mikhmon.
  const genResources = useRouterResources(routers, genRouterId || undefined);

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

  // Aperçu coût + quota du générateur
  const countNum = parseInt(genCount, 10);
  const countValid = Number.isInteger(countNum) && countNum >= 1 && countNum <= 500;
  const selectedGenProfile = profiles?.find((p) => p.id === genProfileId);
  const selectedReseller = resellers?.find((r) => r.id === genResellerId);
  const unitPrice = selectedGenProfile?.price ?? 0;
  const totalCost = (countValid ? countNum : 0) * unitPrice;
  // Quota effectif affiché : hérité du profil ou choisi dans l'onglet Limites.
  const effectiveQuotaMb =
    genQuotaMb === "inherit" ? (selectedGenProfile?.dataQuotaMb ?? 0) : Number(genQuotaMb);
  const quotaLabel =
    effectiveQuotaMb > 0
      ? formatBytes(effectiveQuotaMb * 1048576, lang)
      : genQuotaMb === "inherit" && !selectedGenProfile
        ? t("vouchers.quotaWillInherit")
        : t("vouchers.quotaUnlimitedShort");
  const creditAfter = selectedReseller ? selectedReseller.credit - totalCost : null;
  const insufficient = creditAfter !== null && creditAfter < 0;

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

  const { data: batchData, isLoading: batchesLoading, isFetching: batchesFetching } = useQuery({
    queryKey: ["/api/vouchers", "batches", { search: batchSearch, routerId: batchRouterParam, page: batchPage }],
    queryFn: () =>
      api<PagedBatches>("/api/vouchers/batches", {
        params: { search: batchSearch, routerId: batchRouterParam, page: batchPage, pageSize: BATCH_PAGE_SIZE },
      }),
    refetchInterval: 30_000,
    placeholderData: (previous) => previous,
  });

  const batches = batchData?.data ?? [];
  const batchTotal = batchData?.total ?? 0;
  const batchMaxPage = Math.max(1, Math.ceil(batchTotal / BATCH_PAGE_SIZE));
  const batchSafePage = Math.min(batchPage, batchMaxPage);
  const batchRangeStart = batchTotal === 0 ? 0 : (batchSafePage - 1) * BATCH_PAGE_SIZE + 1;
  const batchRangeEnd = Math.min(batchSafePage * BATCH_PAGE_SIZE, batchTotal);

  const [deletingBatch, setDeletingBatch] = useState<BatchWithStats | null>(null);
  const [printingBatchId, setPrintingBatchId] = useState<string | null>(null);
  const [a4BatchId, setA4BatchId] = useState<string | null>(null);

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
  }

  // « Imprimer le lot » (liste simple) : charge les vouchers restants du lot puis ouvre l'ancien dialog.
  async function printBatch(batch: BatchWithStats) {
    setPrintingBatchId(batch.id);
    try {
      const res = await api<PagedUsers>("/api/vouchers", {
        params: { search: batch.id, page: 1, pageSize: 500 },
      });
      if (res.data.length === 0) {
        toast.info(tf("vouchers.batches.purgedToast", { batch: shortBatch(batch.id) }));
        return;
      }
      setPrintVouchers(res.data);
      setPrintTitle(
        tf("vouchers.batches.printTitle", { batch: shortBatch(batch.id), n: res.data.length }),
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
  // (localStorage "mikcloud-last-batch"). Aucun lot récent → toast informatif.
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
      // Même mécanique que printBatch : recherche par identifiant de lot.
      const res = await api<PagedUsers>("/api/vouchers", {
        params: { search: lastBatch, page: 1, pageSize: 500 },
      });
      if (res.data.length === 0) {
        toast.info(tf("vouchers.batches.purgedToast", { batch: shortBatch(lastBatch) }));
        return;
      }
      setPrintVouchers(res.data);
      setPrintTitle(
        tf("vouchers.batches.printTitle", { batch: shortBatch(lastBatch), n: res.data.length }),
      );
      setPrintBatchId(lastBatch);
      setPrintOpen(true);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t("vouchers.printFailed"));
    } finally {
      setPrintingBatchId(null);
    }
  }

  // « Imprimer A4 + QR » : charge les vouchers ACTIFS (invendus, prêts à la vente) du lot
  // puis ouvre le dialog A4 (20 tickets/page, QR vers la page de login du hotspot).
  async function printBatchA4(batch: BatchWithStats) {
    setA4BatchId(batch.id);
    try {
      const res = await api<PagedUsers>("/api/vouchers", {
        params: { search: batch.id, page: 1, pageSize: 500 },
      });
      const printable = res.data.filter((v) => v.status === "active");
      if (printable.length === 0) {
        toast.info(`Lot #${shortBatch(batch.id)} : plus aucun voucher disponible à imprimer dans ce lot.`);
        return;
      }
      setA4Vouchers(printable);
      setA4Title(`Lot #${shortBatch(batch.id)} — ${printable.length} tickets`);
      setA4HotspotUrl(routers?.find((r) => r.id === batch.routerId)?.hotspotLoginUrl);
      setA4Open(true);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Impression impossible");
    } finally {
      setA4BatchId(null);
    }
  }

  const genValid = countValid && genProfileId !== "" && genRouterId !== "" && !insufficient;

  function submitGenerate() {
    if (!genValid || generateMutation.isPending) return;
    generateMutation.mutate({
      count: countNum,
      profileId: genProfileId,
      routerId: genRouterId,
      server: genServer !== "all" ? genServer : undefined,
      prefix: genPrefix.trim() || undefined,
      codeLength: Number(genCodeLength),
      resellerId: genResellerId === "none" ? undefined : genResellerId,
      userMode: genUserMode === "same" ? "same" : undefined,
      charset: genCharset && genCharset !== "mikcloud" ? genCharset : undefined,
      comment: genComment.trim() || undefined,
      dataQuotaMb: genQuotaMb === "inherit" ? undefined : Number(genQuotaMb),
      timeLimitMin: genTimeLimit === "inherit" ? undefined : Number(genTimeLimit),
    });
  }

  const hasFilters = search !== "" || statusFilter !== "all" || profileFilter !== "all";

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
          {/* Filtres des lots */}
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
              <div className="flex flex-1 flex-wrap gap-3 sm:justify-end">
                <Select
                  value={batchRouterFilter}
                  onValueChange={(value) => {
                    setBatchRouterFilter(value);
                    setBatchPage(1);
                  }}
                >
                  <SelectTrigger className="h-10 w-full sm:w-52" aria-label={t("vouchers.batches.siteFilter")}>
                    <SelectValue placeholder={t("vouchers.batches.siteFilterLabel")} />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="all">{t("common.allSites")}</SelectItem>
                    {routers?.map((router) => (
                      <SelectItem key={router.id} value={router.id}>
                        {router.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
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
                  batchSearch !== "" || batchRouterFilter !== "all"
                    ? t("vouchers.batches.emptyFiltered")
                    : t("vouchers.emptyDesc")
                }
                action={
                  batchSearch === "" && batchRouterFilter === "all" && (
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
                        <TableHead className="pl-4 text-muted-foreground sm:pl-6">{t("common.batch")}</TableHead>
                        <TableHead className="text-muted-foreground">{t("common.profile")}</TableHead>
                        <TableHead className="hidden text-muted-foreground md:table-cell">{t("vouchers.batches.site")}</TableHead>
                        <TableHead className="hidden text-muted-foreground lg:table-cell">{t("vouchers.batches.channel")}</TableHead>
                        <TableHead className="text-right text-muted-foreground">{t("vouchers.batches.vouchers")}</TableHead>
                        <TableHead className="text-right text-muted-foreground">{t("vouchers.batches.active")}</TableHead>
                        <TableHead className="hidden text-right text-muted-foreground xl:table-cell">
                          {t("vouchers.batches.usedExpired")}
                        </TableHead>
                        <TableHead className="hidden text-right text-muted-foreground md:table-cell">{t("vouchers.batches.cost")}</TableHead>
                        <TableHead className="pr-4 text-right text-muted-foreground sm:pr-6">{t("common.actions")}</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {batches.map((batch) => {
                        const purged =
                          batch.active === 0 && batch.used === 0 && batch.expired === 0 && batch.disabled === 0;
                        return (
                          <TableRow key={batch.id}>
                            <TableCell className="pl-4 sm:pl-6">
                              <p className="font-mono text-sm font-medium">#{shortBatch(batch.id)}</p>
                              <p className="text-xs text-muted-foreground">{formatDate(batch.createdAt, lang)}</p>
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
                            <TableCell className="hidden lg:table-cell">
                              {batch.channel === "reseller" ? (
                                <span className="text-muted-foreground">
                                  {t("common.reseller")} · <span className="text-foreground">{batch.resellerName || "—"}</span>
                                </span>
                              ) : (
                                <span className="text-muted-foreground">{t("common.direct")}</span>
                              )}
                            </TableCell>
                            <TableCell className="text-right font-medium tabular-nums">{batch.count}</TableCell>
                            <TableCell className="text-right tabular-nums">
                              {purged ? (
                                <span className="text-xs text-muted-foreground">{t("vouchers.batches.purged")}</span>
                              ) : (
                                <span className={cn("font-medium", batch.active > 0 ? "text-primary" : "text-muted-foreground")}>
                                  {batch.active}
                                </span>
                              )}
                            </TableCell>
                            <TableCell className="hidden text-right text-xs text-muted-foreground tabular-nums xl:table-cell">
                              {batch.used} · {batch.expired}
                            </TableCell>
                            <TableCell className="hidden text-right tabular-nums text-muted-foreground md:table-cell">
                              {formatCurrency(batch.totalCost, currency, lang)}
                            </TableCell>
                            <TableCell className="pr-4 text-right sm:pr-6">
                              <div className="flex items-center justify-end gap-1.5">
                                <Button
                                  variant="outline"
                                  size="sm"
                                  className="h-10 gap-1.5 px-2.5"
                                  disabled={a4BatchId === batch.id}
                                  onClick={() => void printBatchA4(batch)}
                                  aria-label={tf("vouchers.batches.printA4Aria", { batch: batch.id })}
                                  title={t("vouchers.batches.printA4Title")}
                                >
                                  {a4BatchId === batch.id ? (
                                    <Loader2 className="size-4 animate-spin" />
                                  ) : (
                                    <QrCode className="size-4" />
                                  )}
                                  <span className="hidden sm:inline">A4 + QR</span>
                                </Button>
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
                                    <DropdownMenuItem
                                      className="min-h-10"
                                      disabled={a4BatchId === batch.id}
                                      onClick={() => void printBatchA4(batch)}
                                    >
                                      {a4BatchId === batch.id ? (
                                        <Loader2 className="size-4 animate-spin" />
                                      ) : (
                                        <QrCode className="size-4" />
                                      )}
                                      {t("vouchers.batches.printA4")}
                                    </DropdownMenuItem>
                                    <DropdownMenuItem className="min-h-10" onClick={() => viewBatchVouchers(batch)}>
                                      <Eye className="size-4" />
                                      {t("vouchers.batches.view")}
                                    </DropdownMenuItem>
                                    <DropdownMenuItem
                                      className="min-h-10"
                                      disabled={printingBatchId === batch.id}
                                      onClick={() => void printBatch(batch)}
                                    >
                                      {printingBatchId === batch.id ? (
                                        <Loader2 className="size-4 animate-spin" />
                                      ) : (
                                        <Printer className="size-4" />
                                      )}
                                      {t("vouchers.batches.printSimple")}
                                    </DropdownMenuItem>
                                    <DropdownMenuSeparator />
                                    <DropdownMenuItem
                                      variant="destructive"
                                      className="min-h-10"
                                      onClick={() => setDeletingBatch(batch)}
                                    >
                                      <Trash2 className="size-4" />
                                      {t("common.deleteBatch")}
                                    </DropdownMenuItem>
                                  </DropdownMenuContent>
                                </DropdownMenu>
                              </div>
                            </TableCell>
                          </TableRow>
                        );
                      })}
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

      {/* Dialogue générateur */}
      <Dialog open={genOpen} onOpenChange={setGenOpen}>
        <DialogContent className="sm:max-w-xl">
          <DialogHeader>
            <DialogTitle>{t("vouchers.generateTitle")}</DialogTitle>
            <DialogDescription>{t("vouchers.generateDesc")}</DialogDescription>
          </DialogHeader>

          <form
            className="grid gap-4"
            onSubmit={(event) => {
              event.preventDefault();
              submitGenerate();
            }}
          >
            {/* Deux onglets comme le User Manager MikroTik : Général / Limites */}
            <Tabs defaultValue="general">
              <TabsList className="grid w-full grid-cols-2">
                <TabsTrigger value="general" className="gap-1.5">
                  <SlidersHorizontal className="size-3.5" aria-hidden />
                  {t("vouchers.gen.tabGeneral")}
                </TabsTrigger>
                <TabsTrigger value="limits" className="gap-1.5">
                  <Gauge className="size-3.5" aria-hidden />
                  {t("vouchers.gen.tabLimits")}
                </TabsTrigger>
              </TabsList>

              <TabsContent value="general" className="mt-4 grid gap-4">
                <div className="grid gap-4 sm:grid-cols-2">
                  <div className="grid gap-2">
                    <Label htmlFor="gen-count">{t("vouchers.count")}</Label>
                    <Input
                      id="gen-count"
                      type="number"
                      min={1}
                      max={500}
                      value={genCount}
                      onChange={(event) => setGenCount(event.target.value)}
                      disabled={generateMutation.isPending}
                      aria-invalid={!countValid}
                    />
                    <p className="text-xs text-muted-foreground">{t("vouchers.countHint")}</p>
                  </div>
                  <div className="grid gap-2">
                    <Label htmlFor="gen-profile">{t("vouchers.gen.profileLabel")}</Label>
                    <Select
                      value={genProfileId}
                      onValueChange={setGenProfileId}
                      disabled={generateMutation.isPending}
                    >
                      <SelectTrigger id="gen-profile" className="h-10 w-full">
                        <SelectValue placeholder={t("common.selectProfile")} />
                      </SelectTrigger>
                      <SelectContent>
                        {profiles?.map((profile) => (
                          <SelectItem key={profile.id} value={profile.id}>
                            {profile.name} — {formatCurrency(profile.price, currency)} ·{" "}
                            {formatDuration(profile.sessionTimeoutMin * 60)}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                </div>

                {/* Parité Mikhmon (GetValidPrice) : rappel du profil sélectionné —
                    validité RouterOS, prix de vente, verrou, mode d'expiration. */}
                {selectedGenProfile && (
                  <div className="grid gap-1 rounded-lg border bg-muted/40 p-3 text-xs sm:grid-cols-2">
                    <span className="text-muted-foreground">
                      {t("vouchers.gen.recapValidity")} :{" "}
                      <span className="font-medium text-foreground">
                        {fmtRouterDuration(
                          selectedGenProfile.validityMin > 0
                            ? selectedGenProfile.validityMin
                            : selectedGenProfile.validityDays * 1440,
                        )}
                      </span>
                    </span>
                    <span className="text-muted-foreground">
                      {t("vouchers.gen.recapPrice")} :{" "}
                      <span className="font-medium text-foreground">
                        {formatCurrency(
                          selectedGenProfile.sellingPrice > 0
                            ? selectedGenProfile.sellingPrice
                            : selectedGenProfile.price,
                          currency,
                        )}
                      </span>
                    </span>
                    <span className="text-muted-foreground">
                      {t("vouchers.gen.recapLock")} :{" "}
                      <span className="font-medium text-foreground">
                        {selectedGenProfile.lockFirstDevice ? t("common.yes") : t("common.no")}
                      </span>
                    </span>
                    <span className="text-muted-foreground">
                      {t("vouchers.gen.recapExpiry")} :{" "}
                      <span className="font-medium text-foreground">
                        {EXP_MODE_LABEL_KEY[selectedGenProfile.expMode]
                          ? t(EXP_MODE_LABEL_KEY[selectedGenProfile.expMode])
                          : selectedGenProfile.expMode}
                      </span>
                    </span>
                  </div>
                )}

                <div className="grid gap-4 sm:grid-cols-2">
                  <div className="grid gap-2">
                    <Label htmlFor="gen-router">{t("vouchers.gen.routerLabel")}</Label>
                    <Select
                      value={genRouterId}
                      onValueChange={(value) => {
                        setGenRouterId(value);
                        setGenServer("all");
                      }}
                      disabled={generateMutation.isPending}
                    >
                      <SelectTrigger id="gen-router" className="h-10 w-full">
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
                  <div className="grid gap-2">
                    <Label htmlFor="gen-code-length">{t("vouchers.codeLength")}</Label>
                    <Select
                      value={genCodeLength}
                      onValueChange={setGenCodeLength}
                      disabled={generateMutation.isPending}
                    >
                      <SelectTrigger id="gen-code-length" className="h-10 w-full">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {CODE_LENGTHS.map((length) => (
                          <SelectItem key={length} value={String(length)}>
                            {tf("vouchers.codeLengthUnit", { n: length })}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                </div>

            {/* Parité Mikhmon : serveur hotspot du routeur (hidden si inconnu). */}
            {(genResources.data?.servers.length ?? 0) > 0 && (
              <div className="grid gap-2">
                <Label htmlFor="gen-server">{t("vouchers.gen.server")}</Label>
                <Select
                  value={genServer}
                  onValueChange={setGenServer}
                  disabled={generateMutation.isPending}
                >
                  <SelectTrigger id="gen-server" className="h-10 w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="all">{t("vouchers.gen.serverAll")}</SelectItem>
                    {genResources.data?.servers.map((server) => (
                      <SelectItem key={server} value={server}>
                        {server}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <p className="text-xs text-muted-foreground">{t("vouchers.gen.serverHint")}</p>
              </div>
            )}
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="grid gap-2">
                <Label htmlFor="gen-prefix">{t("vouchers.prefix")}</Label>
                <Input
                  id="gen-prefix"
                  value={genPrefix}
                  onChange={(event) => setGenPrefix(event.target.value)}
                  disabled={generateMutation.isPending}
                />
                <p className="text-xs text-muted-foreground">{t("vouchers.prefixHint")}</p>
              </div>
              <div className="grid gap-2">
                <Label htmlFor="gen-reseller">{t("common.reseller")}</Label>
                <Select
                  value={genResellerId}
                  onValueChange={setGenResellerId}
                  disabled={generateMutation.isPending}
                >
                  <SelectTrigger id="gen-reseller" className="h-10 w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="none">{t("vouchers.noReseller")}</SelectItem>
                    {resellers?.map((reseller) => (
                      <SelectItem key={reseller.id} value={reseller.id}>
                        {tf("vouchers.creditLine", {
                          name: reseller.name,
                          credit: formatCurrency(reseller.credit, currency, lang),
                        })}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </div>

                <div className="grid gap-4 sm:grid-cols-2">
                  <div className="grid gap-2">
                    <Label htmlFor="gen-user-mode">{t("vouchers.gen.userMode")}</Label>
                    <Select
                      value={genUserMode}
                      onValueChange={(value) => setGenUserMode(value as "userpass" | "same")}
                      disabled={generateMutation.isPending}
                    >
                      <SelectTrigger id="gen-user-mode" className="h-10 w-full">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="userpass">{t("vouchers.gen.userModeUserpass")}</SelectItem>
                        <SelectItem value="same">{t("vouchers.gen.userModeSame")}</SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                  <div className="grid gap-2">
                    <Label htmlFor="gen-charset">{t("vouchers.gen.charset")}</Label>
                    <Select
                      value={genCharset}
                      onValueChange={setGenCharset}
                      disabled={generateMutation.isPending}
                    >
                      <SelectTrigger id="gen-charset" className="h-10 w-full">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {CHARSET_OPTIONS.map((option) => (
                          <SelectItem key={option.value} value={option.value}>
                            {t(option.labelKey)}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                </div>


                <div className="grid gap-2">
                  <Label htmlFor="gen-comment">{t("vouchers.gen.comment")}</Label>
                  <Input
                    id="gen-comment"
                    placeholder={t("vouchers.gen.commentPlaceholder")}
                    value={genComment}
                    onChange={(event) => setGenComment(event.target.value)}
                    disabled={generateMutation.isPending}
                    maxLength={64}
                  />
                  <p className="text-xs text-muted-foreground">{t("vouchers.gen.commentHint")}</p>
                </div>
              </TabsContent>

              <TabsContent value="limits" className="mt-4 grid gap-4">
                {/* Parité Mikhmon : Time Limit (limit-uptime) par lot */}
                <div className="grid gap-2">
                  <Label htmlFor="gen-timelimit">{t("vouchers.gen.timeLimit")}</Label>
                  <Select
                    value={genTimeLimit}
                    onValueChange={setGenTimeLimit}
                    disabled={generateMutation.isPending}
                  >
                    <SelectTrigger id="gen-timelimit" className="h-10 w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="inherit">
                        {t("vouchers.gen.timeLimitInherit")}
                        {selectedGenProfile
                          ? ` (${
                              selectedGenProfile.sessionTimeoutMin > 0
                                ? fmtRouterDuration(selectedGenProfile.sessionTimeoutMin)
                                : t("vouchers.gen.timeLimitUnlimited")
                            })`
                          : ""}
                      </SelectItem>
                      {TIME_LIMIT_OPTIONS.map((option) => (
                        <SelectItem key={option.value} value={option.value}>
                          {option.min === 0
                            ? t("vouchers.gen.timeLimitUnlimited")
                            : fmtRouterDuration(option.min)}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  <p className="text-xs text-muted-foreground">{t("vouchers.gen.timeLimitHint")}</p>
                </div>

                <div className="grid gap-2">
                  <Label htmlFor="gen-quota">{t("vouchers.gen.quotaLabel")}</Label>
                  <Select
                    value={genQuotaMb}
                    onValueChange={setGenQuotaMb}
                    disabled={generateMutation.isPending}
                  >
                    <SelectTrigger id="gen-quota" className="h-10 w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="inherit">
                        {t("vouchers.gen.quotaInherit")}
                        {selectedGenProfile
                          ? ` (${
                              selectedGenProfile.dataQuotaMb > 0
                                ? formatBytes(selectedGenProfile.dataQuotaMb * 1048576, lang)
                                : t("vouchers.quotaUnlimitedShort")
                            })`
                          : ""}
                      </SelectItem>
                      {QUOTA_OPTIONS.map((quota) => (
                        <SelectItem key={quota.value} value={quota.value}>
                          {quota.mb === 0 ? t("vouchers.gen.quotaUnlimited") : fmtQuota(quota.mb)}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  <p className="text-xs text-muted-foreground">{t("vouchers.gen.quotaHint")}</p>
                </div>

                {/* Repères FCFA — l'argument « 5 Go = 500 F », lisible d'un coup d'œil */}
                <div className="rounded-lg border bg-background p-3">
                  <p className="text-sm font-medium">{t("vouchers.gen.priceHintsTitle")}</p>
                  <div className="mt-2 flex flex-wrap gap-1.5">
                    {QUOTA_PRICE_HINTS.map(([quotaMb, price]) => (
                      <Badge key={quotaMb} variant="secondary" className="font-mono text-xs">
                        {fmtQuota(quotaMb)} = {fmtFcfa(price)}
                      </Badge>
                    ))}
                  </div>
                  <p className="mt-2 text-xs text-muted-foreground">{t("vouchers.gen.priceHintsHint")}</p>
                </div>
              </TabsContent>
            </Tabs>

            {/* Aperçu du coût et du quota en temps réel */}
            <div className="rounded-lg bg-muted/50 p-3 text-sm">
              <p>
                {t("vouchers.totalCost")}{" "}
                <span className="font-semibold">{formatCurrency(totalCost, currency, lang)}</span>{" "}
                <span className="text-muted-foreground">
                  {tf("vouchers.costDetail", {
                    n: countValid ? countNum : 0,
                    price: formatCurrency(unitPrice, currency, lang),
                  })}
                </span>
              </p>
              <p className="mt-1 text-muted-foreground">
                {t("vouchers.gen.quotaPerVoucher")}{" "}
                <span className="font-medium text-foreground">{quotaLabel}</span>
                {selectedGenProfile && tf("vouchers.gen.profileSuffix", { name: selectedGenProfile.name })}
              </p>
              {selectedReseller && (
                <p
                  className={cn(
                    "mt-1",
                    insufficient ? "font-medium text-destructive" : "text-muted-foreground",
                  )}
                >
                  {tf("vouchers.creditAfter", {
                    amount: formatCurrency(creditAfter ?? 0, currency, lang),
                  })}
                  {insufficient &&
                    tf("vouchers.insufficient", { name: selectedReseller.name })}
                </p>
              )}
            </div>

            {generateMutation.isError && (
              <p className="text-sm text-destructive" role="alert">
                {generateMutation.error.message}
              </p>
            )}

            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => setGenOpen(false)}
                disabled={generateMutation.isPending}
              >
                {t("common.cancel")}
              </Button>
              <Button type="submit" disabled={!genValid || generateMutation.isPending}>
                {generateMutation.isPending && <Loader2 className="size-4 animate-spin" />}
                {t("vouchers.generateSubmit")}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

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

      {/* Dialogue d'impression A4 + QR (flux revendeur — lots) */}
      <VoucherA4PrintDialog
        open={a4Open}
        onOpenChange={setA4Open}
        vouchers={a4Vouchers}
        title={a4Title}
        tenantName={tenantName}
        hotspotLoginUrl={a4HotspotUrl}
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
