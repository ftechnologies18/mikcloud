"use client";

// Vue Vouchers (kind=voucher) — générateur en série, impression de tickets, suivi des statuts.

import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  CheckCircle2,
  ChevronLeft,
  ChevronRight,
  Clock,
  Copy,
  Loader2,
  MoreHorizontal,
  Printer,
  Search,
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
import { EmptyState } from "@/components/hotspot/empty-state";
import { LoadingCards, LoadingRows } from "@/components/hotspot/loading";
import { PageHeader } from "@/components/hotspot/page-header";
import { StatCard } from "@/components/hotspot/stat-card";
import { StatusBadge } from "@/components/hotspot/status-badge";
import { useCurrency, useSettings } from "@/components/hotspot/parts/sd-currency";
import { copyToClipboard } from "@/components/hotspot/parts/uc-clipboard";
import { PasswordCell } from "@/components/hotspot/parts/uc-password-cell";
import { UcPrintDialog } from "@/components/hotspot/parts/uc-print-dialog";
import { api } from "@/lib/hotspot/api";
import { formatCurrency, formatDate, formatDuration } from "@/lib/hotspot/format";
import type {
  GenerateVouchersRequest,
  GenerateVouchersResponse,
  HotspotUser,
  PagedUsers,
  Profile,
  Reseller,
  RouterDevice,
} from "@/lib/hotspot/types";
import { cn } from "@/lib/utils";

const PAGE_SIZE = 12;
const CODE_LENGTHS = [5, 6, 8, 10];

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
  const tenantName = settings?.tenant.name || "SpotCloud";
  const queryClient = useQueryClient();

  // Filtres de la table (recherche avec debounce ~400 ms)
  const [searchInput, setSearchInput] = useState("");
  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState("all");
  const [profileFilter, setProfileFilter] = useState("all");
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

  // Générateur
  const [genOpen, setGenOpen] = useState(false);
  const [genCount, setGenCount] = useState("10");
  const [genProfileId, setGenProfileId] = useState("");
  const [genRouterId, setGenRouterId] = useState("");
  const [genPrefix, setGenPrefix] = useState("");
  const [genCodeLength, setGenCodeLength] = useState("6");
  const [genResellerId, setGenResellerId] = useState("none");

  // Impression
  const [printOpen, setPrintOpen] = useState(false);
  const [printVouchers, setPrintVouchers] = useState<HotspotUser[]>([]);
  const [printTitle, setPrintTitle] = useState("");

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

  // Aperçu coût du générateur
  const countNum = parseInt(genCount, 10);
  const countValid = Number.isInteger(countNum) && countNum >= 1 && countNum <= 500;
  const selectedGenProfile = profiles?.find((p) => p.id === genProfileId);
  const selectedReseller = resellers?.find((r) => r.id === genResellerId);
  const unitPrice = selectedGenProfile?.price ?? 0;
  const totalCost = (countValid ? countNum : 0) * unitPrice;
  const creditAfter = selectedReseller ? selectedReseller.credit - totalCost : null;
  const insufficient = creditAfter !== null && creditAfter < 0;

  const generateMutation = useMutation({
    mutationFn: (payload: GenerateVouchersRequest) =>
      api<GenerateVouchersResponse>("/api/vouchers/generate", { method: "POST", body: payload }),
    onSuccess: (res) => {
      toast.success(`${res.vouchers.length} vouchers générés (lot #${shortBatch(res.batchId)})`);
      setGenOpen(false);
      setPrintVouchers(res.vouchers);
      setPrintTitle(`Imprimer les vouchers — Lot #${shortBatch(res.batchId)}`);
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
      toast.success(`Voucher ${voucher.username} supprimé`);
      setDeleting(null);
      invalidateVouchers();
    },
    onError: (error: Error) => toast.error(error.message),
  });

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
                    <TableHead className="text-right text-muted-foreground">Prix</TableHead>
                    <TableHead className="text-muted-foreground">Statut</TableHead>
                    <TableHead className="hidden text-muted-foreground md:table-cell">Revendeur</TableHead>
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
                      <TableCell className="text-right font-medium tabular-nums">
                        {formatCurrency(voucher.price, currency)}
                      </TableCell>
                      <TableCell>
                        <StatusBadge status={voucher.status} dot />
                      </TableCell>
                      <TableCell className="hidden max-w-40 truncate text-muted-foreground md:table-cell">
                        {voucher.resellerName || "—"}
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

      {/* Dialogue générateur */}
      <Dialog open={genOpen} onOpenChange={setGenOpen}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>Générer des vouchers</DialogTitle>
            <DialogDescription>
              Créez une série de tickets WiFi imprimables, prêts à être vendus.
            </DialogDescription>
          </DialogHeader>

          <form
            className="grid gap-4"
            onSubmit={(event) => {
              event.preventDefault();
              submitGenerate();
            }}
          >
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
                <Label htmlFor="gen-profile">Profil</Label>
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
                <Label htmlFor="gen-router">Routeur</Label>
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

            {/* Aperçu du coût en temps réel */}
            <div className="rounded-lg bg-muted/50 p-3 text-sm">
              <p>
                Coût total :{" "}
                <span className="font-semibold">{formatCurrency(totalCost, currency)}</span>{" "}
                <span className="text-muted-foreground">
                  ({countValid ? countNum : 0} × {formatCurrency(unitPrice, currency)})
                </span>
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

      {/* Dialogue d'impression */}
      <UcPrintDialog
        open={printOpen}
        onOpenChange={setPrintOpen}
        vouchers={printVouchers}
        title={printTitle}
        tenantName={tenantName}
        profiles={profiles ?? []}
      />

      {/* Confirmation suppression */}
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
    </div>
  );
}
