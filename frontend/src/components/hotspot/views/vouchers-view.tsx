"use client";

// Vue Vouchers (kind=voucher) — générateur en série, impression de tickets,
// suivi des statuts et traçabilité des lots (onglet Lots).

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
  Search,
  SlidersHorizontal,
  Ticket,
  TicketPlus,
  Trash2,
  Wallet,
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
import { UcPrintDialog } from "@/components/hotspot/parts/uc-print-dialog";
import { VoucherA4PrintDialog } from "@/components/hotspot/parts/voucher-a4-print-dialog";
import { api } from "@/lib/hotspot/api";
import { formatBytes, formatCurrency, formatDate, formatDuration } from "@/lib/hotspot/format";
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
} from "@/lib/hotspot/types";
import { cn } from "@/lib/utils";

const PAGE_SIZE = 12;
const BATCH_PAGE_SIZE = 10;
// 4 caractères comme le User Manager MikroTik, jusqu'à 10 (alphabets sans
// caractères ambigus côté serveur).
const CODE_LENGTHS = [4, 5, 6, 7, 8, 9, 10];

// Jeux de caractères — presets du User Manager MikroTik (« abcd », « ABCD »,
// « aBcD », « 5ab2c34d », « 5AB2C34D », « 5aB2c34D »). Le serveur exclut
// toujours 0/1/I/L/O : les codes restent lisibles sur un ticket imprimé.
const CHARSET_OPTIONS = [
  { value: "mikcloud", label: "MikCloud (recommandé)" },
  { value: "abc", label: "abcd — minuscules" },
  { value: "ABC", label: "ABCD — majuscules" },
  { value: "aBc", label: "aBcD — lettres (min + maj)" },
  { value: "5ab", label: "5ab2c34d — chiffres + minuscules" },
  { value: "5AB", label: "5AB2C34D — chiffres + majuscules" },
  { value: "5aB", label: "5aB2c34D — chiffres + lettres" },
];

// Quotas courants (1 Go = 1024 Mo) — l'argument « 5 Go = 500 F ».
const QUOTA_OPTIONS = [
  { value: "0", label: "Illimité (pas de plafond data)" },
  { value: "512", label: "512 Mo" },
  { value: "1024", label: "1 Go" },
  { value: "2048", label: "2 Go" },
  { value: "5120", label: "5 Go" },
  { value: "10240", label: "10 Go" },
  { value: "20480", label: "20 Go" },
  { value: "51200", label: "50 Go" },
];

// Repères de prix FCFA pour vendre au quota (le prix réel reste celui du profil).
const QUOTA_PRICE_HINTS: [string, string][] = [
  ["1 Go", "100 F"],
  ["2 Go", "200 F"],
  ["5 Go", "500 F"],
  ["10 Go", "1 000 F"],
  ["30 Go", "3 000 F"],
];

const STATUS_OPTIONS = [
  { value: "all", label: "Tous les statuts" },
  { value: "active", label: "Actifs" },
  { value: "used", label: "Utilisés" },
  { value: "expired", label: "Expirés" },
  { value: "disabled", label: "Désactivés" },
];

function shortBatch(batchId: string): string {
  return batchId.split("-").pop() || batchId;
}

export default function VouchersView() {
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

  // Impression (liste simple — ancien dialog)
  const [printOpen, setPrintOpen] = useState(false);
  const [printVouchers, setPrintVouchers] = useState<HotspotUser[]>([]);
  const [printTitle, setPrintTitle] = useState("");

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

  const { data: resellers } = useQuery({
    queryKey: ["/api/resellers"],
    queryFn: () => api<Reseller[]>("/api/resellers"),
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
    if (ok) toast.success("Code copié");
    else toast.error("Copie impossible dans le presse-papiers");
  }

  function printOne(voucher: HotspotUser) {
    setPrintVouchers([voucher]);
    setPrintTitle(`Imprimer le voucher — ${voucher.username}`);
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
      ? formatBytes(effectiveQuotaMb * 1048576)
      : genQuotaMb === "inherit" && !selectedGenProfile
        ? "Héritera du profil"
        : "Illimité";
  const creditAfter = selectedReseller ? selectedReseller.credit - totalCost : null;
  const insufficient = creditAfter !== null && creditAfter < 0;

  const generateMutation = useMutation({
    mutationFn: (payload: GenerateVouchersRequest) =>
      api<GenerateVouchersResponse>("/api/vouchers/generate", { method: "POST", body: payload }),
    onSuccess: (res) => {
      toast.success(`${res.vouchers.length} vouchers générés (lot #${shortBatch(res.batchId)})`);
      setGenOpen(false);
      // Flux revendeur : ouverture directe du dialog A4 + QR (20 tickets/page).
      const routerUrl = routers?.find(
        (r) => r.id === (res.vouchers[0]?.routerId ?? genRouterId),
      )?.hotspotLoginUrl;
      setA4Vouchers(res.vouchers);
      setA4Title(`Lot #${shortBatch(res.batchId)} — ${res.vouchers.length} tickets`);
      setA4HotspotUrl(routerUrl);
      setA4Open(true);
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
      toast.success(`Voucher ${voucher.username} supprimé`);
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
      toast.success(`Lot #${shortBatch(batch.id)} supprimé (${res.deleted} vouchers)`);
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
        toast.info(`Lot #${shortBatch(batch.id)} : plus aucun voucher en base (purgé).`);
        return;
      }
      setPrintVouchers(res.data);
      setPrintTitle(`Imprimer le lot #${shortBatch(batch.id)} — ${res.data.length} tickets`);
      setPrintOpen(true);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Impression impossible");
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
      prefix: genPrefix.trim() || undefined,
      codeLength: Number(genCodeLength),
      resellerId: genResellerId === "none" ? undefined : genResellerId,
      userMode: genUserMode === "same" ? "same" : undefined,
      charset: genCharset && genCharset !== "mikcloud" ? genCharset : undefined,
      comment: genComment.trim() || undefined,
      dataQuotaMb: genQuotaMb === "inherit" ? undefined : Number(genQuotaMb),
    });
  }

  const hasFilters = search !== "" || statusFilter !== "all" || profileFilter !== "all";

  return (
    <div className="space-y-4 sm:space-y-6">
      <PageHeader
        title="Vouchers"
        description="Générez, imprimez et suivez vos tickets WiFi en série"
        actions={
          <Button className="h-10" onClick={() => setGenOpen(true)}>
            <TicketPlus className="size-4" />
            Générer des vouchers
          </Button>
        }
      />

      {/* Onglets Vouchers / Lots (traçabilité) */}
      <Tabs value={tab} onValueChange={(value) => setTab(value as "vouchers" | "batches")}>
        <TabsList>
          <TabsTrigger value="vouchers" className="gap-1.5">
            <Ticket className="size-3.5" aria-hidden />
            Vouchers
          </TabsTrigger>
          <TabsTrigger value="batches" className="gap-1.5">
            <Layers className="size-3.5" aria-hidden />
            Lots
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
          <StatCard title="Vouchers actifs" value={String(activeCount)} sub="prêts à la vente" icon={Ticket} />
          <StatCard title="Utilisés" value={String(usedCount)} sub="tickets consommés" icon={CheckCircle2} />
          <StatCard title="Expirés" value={String(expiredCount)} sub="validité dépassée" icon={Clock} />
          <StatCard
            title="Valeur en stock"
            value={formatCurrency(stockValue, currency)}
            sub="somme des vouchers actifs"
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
              placeholder="Rechercher un code…"
              value={searchInput}
              onChange={(event) => setSearchInput(event.target.value)}
              aria-label="Rechercher un code de voucher"
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
              <SelectTrigger className="h-10 w-full sm:w-44" aria-label="Filtrer par statut">
                <SelectValue placeholder="Statut" />
              </SelectTrigger>
              <SelectContent>
                {STATUS_OPTIONS.map((option) => (
                  <SelectItem key={option.value} value={option.value}>
                    {option.label}
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
              <SelectTrigger className="h-10 w-full sm:w-48" aria-label="Filtrer par profil">
                <SelectValue placeholder="Profil" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">Tous les profils</SelectItem>
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
            title="Aucun voucher"
            description={
              hasFilters
                ? "Aucun ticket ne correspond à ces filtres."
                : "Générez votre premier lot de tickets WiFi."
            }
            action={
              !hasFilters && (
                <Button onClick={() => setGenOpen(true)}>
                  <TicketPlus className="size-4" />
                  Générer des vouchers
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
                    <TableHead className="pl-4 text-muted-foreground sm:pl-6">Code</TableHead>
                    <TableHead className="text-muted-foreground">Mot de passe</TableHead>
                    <TableHead className="text-muted-foreground">Profil</TableHead>
                    <TableHead className="hidden text-muted-foreground md:table-cell">Quota</TableHead>
                    <TableHead className="text-right text-muted-foreground">Prix</TableHead>
                    <TableHead className="text-muted-foreground">Statut</TableHead>
                    <TableHead className="hidden text-muted-foreground md:table-cell">Revendeur</TableHead>
                    <TableHead className="hidden text-muted-foreground md:table-cell">Lot</TableHead>
                    <TableHead className="hidden text-muted-foreground lg:table-cell">Créé</TableHead>
                    <TableHead className="pr-4 text-right text-muted-foreground sm:pr-6">Actions</TableHead>
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
                            aria-label={`Copier le code ${voucher.username}`}
                            title="Copier le code"
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
                              ? `Masquer le mot de passe de ${voucher.username}`
                              : `Afficher le mot de passe de ${voucher.username}`
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
                            {formatBytes(voucher.dataQuotaMb * 1048576)}
                          </span>
                        ) : (
                          <span className="text-sm text-muted-foreground">—</span>
                        )}
                      </TableCell>
                      <TableCell className="text-right font-medium tabular-nums">
                        {formatCurrency(voucher.price, currency)}
                      </TableCell>
                      <TableCell>
                        <StatusBadge status={voucher.status} dot />
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
                            title={`Filtrer le lot ${voucher.batchId}`}
                          >
                            #{shortBatch(voucher.batchId)}
                          </button>
                        ) : (
                          "—"
                        )}
                      </TableCell>
                      <TableCell className="hidden tabular-nums text-muted-foreground lg:table-cell">
                        {formatDate(voucher.createdAt)}
                      </TableCell>
                      <TableCell className="pr-4 text-right sm:pr-6">
                        <DropdownMenu>
                          <DropdownMenuTrigger asChild>
                            <Button
                              variant="ghost"
                              size="icon"
                              className="size-10 text-muted-foreground hover:text-foreground"
                              aria-label={`Actions pour ${voucher.username}`}
                            >
                              <MoreHorizontal className="size-4" />
                            </Button>
                          </DropdownMenuTrigger>
                          <DropdownMenuContent align="end" className="w-56">
                            <DropdownMenuItem className="min-h-10" onClick={() => printOne(voucher)}>
                              <Printer className="size-4" />
                              Imprimer ce voucher
                            </DropdownMenuItem>
                            <DropdownMenuSeparator />
                            <DropdownMenuItem
                              variant="destructive"
                              className="min-h-10"
                              onClick={() => setDeleting(voucher)}
                            >
                              <Trash2 className="size-4" />
                              Supprimer
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
                {isFetching ? "Actualisation…" : `${rangeStart}–${rangeEnd} sur ${totalCount}`}
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
                  Précédent
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  className="h-10"
                  onClick={() => setPage((p) => Math.min(maxPage, p + 1))}
                  disabled={safePage >= maxPage}
                >
                  Suivant
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
                  placeholder="Rechercher un lot, un profil, un revendeur…"
                  value={batchSearchInput}
                  onChange={(event) => setBatchSearchInput(event.target.value)}
                  aria-label="Rechercher un lot"
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
                  <SelectTrigger className="h-10 w-full sm:w-52" aria-label="Filtrer les lots par site">
                    <SelectValue placeholder="Site" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="all">Tous les sites</SelectItem>
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
                title="Aucun lot"
                description={
                  batchSearch !== "" || batchRouterFilter !== "all"
                    ? "Aucun lot ne correspond à ces filtres."
                    : "Générez votre premier lot de tickets WiFi."
                }
                action={
                  batchSearch === "" && batchRouterFilter === "all" && (
                    <Button onClick={() => setGenOpen(true)}>
                      <TicketPlus className="size-4" />
                      Générer des vouchers
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
                        <TableHead className="pl-4 text-muted-foreground sm:pl-6">Lot</TableHead>
                        <TableHead className="text-muted-foreground">Profil</TableHead>
                        <TableHead className="hidden text-muted-foreground md:table-cell">Site</TableHead>
                        <TableHead className="hidden text-muted-foreground lg:table-cell">Canal</TableHead>
                        <TableHead className="text-right text-muted-foreground">Vouchers</TableHead>
                        <TableHead className="text-right text-muted-foreground">Actifs</TableHead>
                        <TableHead className="hidden text-right text-muted-foreground xl:table-cell">
                          Utilisés · Expirés
                        </TableHead>
                        <TableHead className="hidden text-right text-muted-foreground md:table-cell">Coût</TableHead>
                        <TableHead className="pr-4 text-right text-muted-foreground sm:pr-6">Actions</TableHead>
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
                              <p className="text-xs text-muted-foreground">{formatDate(batch.createdAt)}</p>
                            </TableCell>
                            <TableCell>
                              <Badge variant="outline" className="max-w-36 truncate">
                                {batch.profileName}
                              </Badge>
                              {batch.dataQuotaMb > 0 && (
                                <p className="mt-0.5 text-xs text-muted-foreground">
                                  {formatBytes(batch.dataQuotaMb * 1048576)} / voucher
                                </p>
                              )}
                            </TableCell>
                            <TableCell className="hidden max-w-40 truncate text-muted-foreground md:table-cell">
                              {batch.routerName}
                            </TableCell>
                            <TableCell className="hidden lg:table-cell">
                              {batch.channel === "reseller" ? (
                                <span className="text-muted-foreground">
                                  Revendeur · <span className="text-foreground">{batch.resellerName || "—"}</span>
                                </span>
                              ) : (
                                <span className="text-muted-foreground">Direct</span>
                              )}
                            </TableCell>
                            <TableCell className="text-right font-medium tabular-nums">{batch.count}</TableCell>
                            <TableCell className="text-right tabular-nums">
                              {purged ? (
                                <span className="text-xs text-muted-foreground">Purgé</span>
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
                              {formatCurrency(batch.totalCost, currency)}
                            </TableCell>
                            <TableCell className="pr-4 text-right sm:pr-6">
                              <div className="flex items-center justify-end gap-1.5">
                                <Button
                                  variant="outline"
                                  size="sm"
                                  className="h-10 gap-1.5 px-2.5"
                                  disabled={a4BatchId === batch.id}
                                  onClick={() => void printBatchA4(batch)}
                                  aria-label={`Imprimer A4 + QR les vouchers actifs du lot ${batch.id}`}
                                  title="Imprimer A4 + QR — vouchers actifs à vendre (20 par page)"
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
                                      aria-label={`Actions pour le lot ${batch.id}`}
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
                                      Imprimer A4 + QR
                                    </DropdownMenuItem>
                                    <DropdownMenuItem className="min-h-10" onClick={() => viewBatchVouchers(batch)}>
                                      <Eye className="size-4" />
                                      Voir les vouchers
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
                                      Imprimer (liste simple)
                                    </DropdownMenuItem>
                                    <DropdownMenuSeparator />
                                    <DropdownMenuItem
                                      variant="destructive"
                                      className="min-h-10"
                                      onClick={() => setDeletingBatch(batch)}
                                    >
                                      <Trash2 className="size-4" />
                                      Supprimer le lot
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
                      ? "Actualisation…"
                      : `${batchRangeStart}–${batchRangeEnd} sur ${batchTotal} lots tracés`}
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
                      Précédent
                    </Button>
                    <Button
                      variant="outline"
                      size="sm"
                      className="h-10"
                      onClick={() => setBatchPage((p) => Math.min(batchMaxPage, p + 1))}
                      disabled={batchSafePage >= batchMaxPage}
                    >
                      Suivant
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
            <DialogTitle>Générer des vouchers</DialogTitle>
            <DialogDescription>
              Tickets WiFi imprimables en série — avec quota data en Go et prix FCFA.
            </DialogDescription>
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
                  Général
                </TabsTrigger>
                <TabsTrigger value="limits" className="gap-1.5">
                  <Gauge className="size-3.5" aria-hidden />
                  Limites
                </TabsTrigger>
              </TabsList>

              <TabsContent value="general" className="mt-4 grid gap-4">
                <div className="grid gap-4 sm:grid-cols-2">
                  <div className="grid gap-2">
                    <Label htmlFor="gen-count">Nombre de vouchers</Label>
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
                    <p className="text-xs text-muted-foreground">Entre 1 et 500 tickets.</p>
                  </div>
                  <div className="grid gap-2">
                    <Label htmlFor="gen-profile">Profil (prix de vente)</Label>
                    <Select
                      value={genProfileId}
                      onValueChange={setGenProfileId}
                      disabled={generateMutation.isPending}
                    >
                      <SelectTrigger id="gen-profile" className="h-10 w-full">
                        <SelectValue placeholder="Sélectionner un profil" />
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

                <div className="grid gap-4 sm:grid-cols-2">
                  <div className="grid gap-2">
                    <Label htmlFor="gen-router">Routeur (serveur hotspot)</Label>
                    <Select
                      value={genRouterId}
                      onValueChange={setGenRouterId}
                      disabled={generateMutation.isPending}
                    >
                      <SelectTrigger id="gen-router" className="h-10 w-full">
                        <SelectValue placeholder="Sélectionner un routeur" />
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
                    <Label htmlFor="gen-code-length">Longueur du code</Label>
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
                            {length} caractères
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                </div>

                <div className="grid gap-4 sm:grid-cols-2">
                  <div className="grid gap-2">
                    <Label htmlFor="gen-user-mode">Mode utilisateur</Label>
                    <Select
                      value={genUserMode}
                      onValueChange={(value) => setGenUserMode(value as "userpass" | "same")}
                      disabled={generateMutation.isPending}
                    >
                      <SelectTrigger id="gen-user-mode" className="h-10 w-full">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="userpass">Nom d'utilisateur & mot de passe</SelectItem>
                        <SelectItem value="same">Mot de passe = nom d'utilisateur</SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                  <div className="grid gap-2">
                    <Label htmlFor="gen-charset">Jeu de caractères</Label>
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
                            {option.label}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                </div>

                <div className="grid gap-4 sm:grid-cols-2">
                  <div className="grid gap-2">
                    <Label htmlFor="gen-prefix">Préfixe (optionnel)</Label>
                    <Input
                      id="gen-prefix"
                      placeholder="SC-"
                      value={genPrefix}
                      onChange={(event) => setGenPrefix(event.target.value)}
                      disabled={generateMutation.isPending}
                    />
                  </div>
                  <div className="grid gap-2">
                    <Label htmlFor="gen-reseller">Revendeur</Label>
                    <Select
                      value={genResellerId}
                      onValueChange={setGenResellerId}
                      disabled={generateMutation.isPending}
                    >
                      <SelectTrigger id="gen-reseller" className="h-10 w-full">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="none">Aucun (vente directe)</SelectItem>
                        {resellers?.map((reseller) => (
                          <SelectItem key={reseller.id} value={reseller.id}>
                            {reseller.name} — crédit {formatCurrency(reseller.credit, currency)}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                </div>

                <div className="grid gap-2">
                  <Label htmlFor="gen-comment">Commentaire (optionnel)</Label>
                  <Input
                    id="gen-comment"
                    placeholder="Ex. kiosque gare, lot revendeur…"
                    value={genComment}
                    onChange={(event) => setGenComment(event.target.value)}
                    disabled={generateMutation.isPending}
                    maxLength={64}
                  />
                  <p className="text-xs text-muted-foreground">
                    Inscrit sur le routeur avec le n° de lot (64 caractères max).
                  </p>
                </div>
              </TabsContent>

              <TabsContent value="limits" className="mt-4 grid gap-4">
                <div className="grid gap-2">
                  <Label htmlFor="gen-quota">Quota de données par voucher</Label>
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
                        Hériter du profil
                        {selectedGenProfile
                          ? ` (${
                              selectedGenProfile.dataQuotaMb > 0
                                ? formatBytes(selectedGenProfile.dataQuotaMb * 1048576)
                                : "illimité"
                            })`
                          : ""}
                      </SelectItem>
                      {QUOTA_OPTIONS.map((quota) => (
                        <SelectItem key={quota.value} value={quota.value}>
                          {quota.label}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  <p className="text-xs text-muted-foreground">
                    Le routeur coupe la connexion quand le quota est épuisé (limit-bytes-total).
                  </p>
                </div>

                {/* Repères FCFA — l'argument « 5 Go = 500 F », lisible d'un coup d'œil */}
                <div className="rounded-lg border bg-background p-3">
                  <p className="text-sm font-medium">Repères FCFA du marché</p>
                  <div className="mt-2 flex flex-wrap gap-1.5">
                    {QUOTA_PRICE_HINTS.map(([quota, price]) => (
                      <Badge key={quota} variant="secondary" className="font-mono text-xs">
                        {quota} = {price}
                      </Badge>
                    ))}
                  </div>
                  <p className="mt-2 text-xs text-muted-foreground">
                    Le prix de vente reste celui du profil — alignez-le sur le quota choisi.
                  </p>
                </div>
              </TabsContent>
            </Tabs>

            {/* Aperçu du coût et du quota en temps réel */}
            <div className="rounded-lg bg-muted/50 p-3 text-sm">
              <p>
                Coût total :{" "}
                <span className="font-semibold">{formatCurrency(totalCost, currency)}</span>{" "}
                <span className="text-muted-foreground">
                  ({countValid ? countNum : 0} × {formatCurrency(unitPrice, currency)})
                </span>
              </p>
              <p className="mt-1 text-muted-foreground">
                Quota par voucher :{" "}
                <span className="font-medium text-foreground">{quotaLabel}</span>
                {selectedGenProfile && ` · Profil : ${selectedGenProfile.name}`}
              </p>
              {selectedReseller && (
                <p
                  className={cn(
                    "mt-1",
                    insufficient ? "font-medium text-destructive" : "text-muted-foreground",
                  )}
                >
                  Crédit après achat : {formatCurrency(creditAfter ?? 0, currency)}
                  {insufficient &&
                    ` — crédit insuffisant, rechargez ${selectedReseller.name} avant de générer.`}
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
                Annuler
              </Button>
              <Button type="submit" disabled={!genValid || generateMutation.isPending}>
                {generateMutation.isPending && <Loader2 className="size-4 animate-spin" />}
                Générer
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* Dialogue d'impression (liste simple — voucher unitaire, lots) */}
      <UcPrintDialog
        open={printOpen}
        onOpenChange={setPrintOpen}
        vouchers={printVouchers}
        title={printTitle}
        tenantName={tenantName}
        profiles={profiles ?? []}
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
            <AlertDialogTitle>Supprimer le voucher {deleting?.username} ?</AlertDialogTitle>
            <AlertDialogDescription>
              Cette action est irréversible. Le ticket ne pourra plus être utilisé.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleteMutation.isPending}>Annuler</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-white hover:bg-destructive/90"
              disabled={deleteMutation.isPending}
              onClick={(event) => {
                event.preventDefault();
                if (deleting) deleteMutation.mutate(deleting);
              }}
            >
              {deleteMutation.isPending && <Loader2 className="size-4 animate-spin" />}
              Supprimer
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* Confirmation suppression d'un lot entier */}
      <AlertDialog open={deletingBatch !== null} onOpenChange={(open) => !open && setDeletingBatch(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Supprimer le lot #{deletingBatch && shortBatch(deletingBatch.id)} ?</AlertDialogTitle>
            <AlertDialogDescription>
              {deletingBatch &&
                `Les ${deletingBatch.count} vouchers de ce lot (${deletingBatch.profileName} — ${deletingBatch.routerName}) seront retirés du routeur et de la base. Cette action est irréversible.`}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={batchDeleteMutation.isPending}>Annuler</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-white hover:bg-destructive/90"
              disabled={batchDeleteMutation.isPending}
              onClick={(event) => {
                event.preventDefault();
                if (deletingBatch) batchDeleteMutation.mutate(deletingBatch);
              }}
            >
              {batchDeleteMutation.isPending && <Loader2 className="size-4 animate-spin" />}
              Supprimer le lot
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
