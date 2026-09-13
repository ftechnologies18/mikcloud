"use client";

// Onglet « Lots » de la vue Vouchers — traçabilité complète des générations :
// pipeline « tour de contrôle » (totaux filtrés), filtres fiche de vie
// (desktop : rangée compacte ; mobile : sheet bottom + badge de filtres actifs),
// cartes mobiles + table desktop (cycle de vie BatchLifeBar, détention chips,
// vélocité/dormance, valeur faciale), actions par lot (3 boutons icône + menu ⋯)
// et pagination.
// L'état et les requêtes vivent dans le shell (views/vouchers-view.tsx) ;
// les helpers de rendu (chips, vélocité, selects partagés, boutons d'action,
// menu ⋯) sont des closures internes de ce composant, comme ils l'étaient du
// shell d'origine — mêmes signatures, mêmes rendus.

import {
  ArrowLeftRight,
  ChevronLeft,
  ChevronRight,
  Download,
  Eye,
  Filter,
  Info,
  Layers,
  Loader2,
  MoreHorizontal,
  Printer,
  RotateCcw,
  Search,
  Ticket,
  TicketPlus,
  Trash2,
} from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { EmptyState } from "@/components/hotspot/empty-state";
import { LoadingRows } from "@/components/hotspot/loading";
import { useCurrency } from "@/components/hotspot/parts/sd-currency";
import { BatchLifeBadge, BatchLifeBar } from "@/components/hotspot/parts/batch-life-bar";
import { BatchPipeline } from "@/components/hotspot/parts/batch-pipeline";
import { useI18n } from "@/lib/hotspot/i18n";
import { formatCurrency, formatDate } from "@/lib/hotspot/format";
import type { BatchSummary, BatchWithStats, Reseller, RouterDevice } from "@/lib/hotspot/types";
import { cn } from "@/lib/utils";
import { BATCH_PAGE_SIZE, BATCH_STATUS_OPTIONS, shortBatch } from "./shared";

interface BatchesTabProps {
  // Pipeline « tour de contrôle » (totaux sur l'ensemble FILTRÉ)
  batchSummary: BatchSummary | undefined;
  firstLoading: boolean;
  // Filtres fiche de vie (l'état vit dans le shell — callbacks de changement,
  // chaque changement ramène la pagination à la page 1 côté shell)
  batchSearchInput: string;
  onBatchSearchInput: (value: string) => void;
  batchRouterFilter: string;
  onBatchRouterFilter: (value: string) => void;
  batchChannelFilter: string;
  onBatchChannelFilter: (value: string) => void;
  batchStatusFilter: string;
  onBatchStatusFilter: (value: string) => void;
  batchHolderFilter: string;
  onBatchHolderFilter: (value: string) => void;
  routers: RouterDevice[] | undefined;
  resellers: Reseller[] | undefined;
  // Sheet « Filtres » mobile (< md)
  filtersOpen: boolean;
  onFiltersOpenChange: (open: boolean) => void;
  // Filtres actifs (calculés sur la recherche DEBOUNCÉE côté shell) + export + reset
  hasBatchFilters: boolean;
  batchActiveFilterCount: number;
  exportingBatches: boolean;
  onExportBatches: () => void;
  onResetFilters: () => void;
  // Table des lots
  batches: BatchWithStats[];
  isFetching: boolean;
  batchTotal: number;
  batchPage: number;
  onSetBatchPage: (updater: (page: number) => number) => void;
  printingBatchId: string | null;
  multiPrintBatch: BatchWithStats | null;
  onGenerate: () => void;
  onOpenDetail: (batch: BatchWithStats) => void;
  onViewVouchers: (batch: BatchWithStats) => void;
  onPrintMulti: (batch: BatchWithStats) => void;
  onPrintSimple: (batch: BatchWithStats) => void;
  onTransfer: (batch: BatchWithStats) => void;
  onDeleteBatch: (batch: BatchWithStats) => void;
}

export function BatchesTab({
  batchSummary,
  firstLoading,
  batchSearchInput,
  onBatchSearchInput,
  batchRouterFilter,
  onBatchRouterFilter,
  batchChannelFilter,
  onBatchChannelFilter,
  batchStatusFilter,
  onBatchStatusFilter,
  batchHolderFilter,
  onBatchHolderFilter,
  routers,
  resellers,
  filtersOpen,
  onFiltersOpenChange,
  hasBatchFilters,
  batchActiveFilterCount,
  exportingBatches,
  onExportBatches,
  onResetFilters,
  batches,
  isFetching,
  batchTotal,
  batchPage,
  onSetBatchPage,
  printingBatchId,
  multiPrintBatch,
  onGenerate,
  onOpenDetail,
  onViewVouchers,
  onPrintMulti,
  onPrintSimple,
  onTransfer,
  onDeleteBatch,
}: BatchesTabProps) {
  const { t, tf, lang } = useI18n();
  const currency = useCurrency();

  // Pagination — mêmes formules que le shell d'origine.
  const batchMaxPage = Math.max(1, Math.ceil(batchTotal / BATCH_PAGE_SIZE));
  const batchSafePage = Math.min(batchPage, batchMaxPage);
  const batchRangeStart = batchTotal === 0 ? 0 : (batchSafePage - 1) * BATCH_PAGE_SIZE + 1;
  const batchRangeEnd = Math.min(batchSafePage * BATCH_PAGE_SIZE, batchTotal);

  // Refonte v2 — un filtre n'est « actif » que s'il diverge du DÉFAUT
  // (statut « stock » = Vivants) ; ces indicateurs sont calculés par le shell
  // sur la recherche DEBOUNCÉE (comportement d'origine) et passés en props.

  // v2 — « Détention » : chips live du stock vendable (Direct {n} / {name} {n}).
  function holdingsChips(batch: BatchWithStats) {
    const holdings = batch.holdings ?? [];
    if (holdings.length === 0) {
      return <span className="text-sm text-muted-foreground">—</span>;
    }
    return (
      <div className="flex flex-wrap items-center gap-1.5">
        {holdings.map((h) => (
          <Badge key={h.resellerId || "direct"} variant="secondary" className="gap-1 text-xs">
            {h.resellerId === "" ? t("common.direct") : h.name || h.resellerId}
            <span className="font-semibold tabular-nums">{h.count}</span>
          </Badge>
        ))}
      </div>
    );
  }

  // v2 — « Vélocité » : sorties 7 j (+ projection), sinon dormance du stock.
  // null = rien à afficher (plus de stock, aucune sortie).
  function velocityInfo(batch: BatchWithStats): { main: string; sub?: string; amber: boolean } | null {
    if (batch.sold7d > 0) {
      return {
        main: tf("vouchers.batches.velocity.egress", { n: batch.sold7d }),
        sub:
          batch.transferable > 0
            ? tf("vouchers.batches.velocity.deplete", { d: Math.ceil((batch.transferable * 7) / batch.sold7d) })
            : undefined,
        amber: false,
      };
    }
    if (batch.transferable > 0) {
      return {
        main: tf("vouchers.batches.velocity.dormant", { d: batch.dormantDays }),
        amber: batch.dormantDays >= 7,
      };
    }
    return null;
  }

  // v2 — les 4 selects de filtres des lots, partagés entre la rangée desktop
  // (md+) et le sheet « Filtres » mobile (mêmes états, deux rendus).
  const batchSiteSelect = (triggerClassName: string) => (
    <Select value={batchRouterFilter} onValueChange={onBatchRouterFilter}>
      <SelectTrigger className={triggerClassName} aria-label={t("vouchers.batches.siteFilter")}>
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
  );

  const batchChannelSelect = (triggerClassName: string) => (
    <Select value={batchChannelFilter} onValueChange={onBatchChannelFilter}>
      <SelectTrigger className={triggerClassName} aria-label={t("vouchers.batches.filterChannel")}>
        <SelectValue placeholder={t("vouchers.batches.channelAll")} />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value="all">{t("vouchers.batches.channelAll")}</SelectItem>
        <SelectItem value="direct">{t("common.direct")}</SelectItem>
        <SelectItem value="reseller">{t("common.reseller")}</SelectItem>
      </SelectContent>
    </Select>
  );

  const batchStatusSelect = (triggerClassName: string) => (
    <Select value={batchStatusFilter} onValueChange={onBatchStatusFilter}>
      <SelectTrigger className={triggerClassName} aria-label={t("vouchers.batches.filterStatus")}>
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
  );

  const batchHolderSelect = (triggerClassName: string) => (
    <Select value={batchHolderFilter} onValueChange={onBatchHolderFilter}>
      <SelectTrigger className={triggerClassName} aria-label={t("vouchers.batches.filterHolder")}>
        <SelectValue placeholder={t("vouchers.batches.holderAll")} />
      </SelectTrigger>
      <SelectContent className="max-h-72">
        <SelectItem value="all">{t("vouchers.batches.holderAll")}</SelectItem>
        <SelectItem value="direct">{t("vouchers.batches.holderDirect")}</SelectItem>
        {/* v2 — « Chez les revendeurs » : n'importe quel revendeur (backend « resellers »). */}
        <SelectItem value="resellers">{t("vouchers.batches.holder.resellers")}</SelectItem>
        {resellers?.map((reseller) => (
          <SelectItem key={reseller.id} value={reseller.id}>
            {reseller.name}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );

  // v2 — actions de ligne en boutons icône UNIQUEMENT (ghost + tooltip) :
  // zéro texte dans les boutons, zéro débordement — même rendu en table
  // (md+) et en cartes mobiles.
  function transferIconButton(batch: BatchWithStats, className = "size-9") {
    if (batch.transferable === 0) return null;
    const label = t("vouchers.batches.transfer");
    return (
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            variant="ghost"
            size="icon"
            className={`${className} text-muted-foreground hover:text-primary`}
            onClick={() => onTransfer(batch)}
            aria-label={tf("vouchers.batches.transferAria", { batch: batch.id })}
          >
            <ArrowLeftRight className="size-4" />
          </Button>
        </TooltipTrigger>
        <TooltipContent>{label}</TooltipContent>
      </Tooltip>
    );
  }

  function printIconButton(batch: BatchWithStats, className = "size-9") {
    const label = t("vouchers.batches.detail.printMulti");
    const printing = multiPrintBatch?.id === batch.id;
    return (
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            variant="ghost"
            size="icon"
            className={`${className} text-muted-foreground hover:text-foreground`}
            disabled={printing}
            onClick={() => onPrintMulti(batch)}
            aria-label={tf("vouchers.batches.printMultiTitle", { batch: shortBatch(batch.id) })}
          >
            {printing ? <Loader2 className="size-4 animate-spin" /> : <Printer className="size-4" />}
          </Button>
        </TooltipTrigger>
        <TooltipContent>{label}</TooltipContent>
      </Tooltip>
    );
  }

  // v2 — menu ⋯ du lot : la fiche 360° d'abord, puis toutes les actions.
  // Le déclencheur est un bouton icône ghost (tooltip + aria-label).
  function batchActionsMenu(batch: BatchWithStats, className = "size-9") {
    return (
      <DropdownMenu>
        <Tooltip>
          <TooltipTrigger asChild>
            <DropdownMenuTrigger asChild>
              <Button
                variant="ghost"
                size="icon"
                className={`${className} text-muted-foreground hover:text-foreground`}
                aria-label={tf("vouchers.batches.actionsFor", { batch: batch.id })}
              >
                <MoreHorizontal className="size-4" />
              </Button>
            </DropdownMenuTrigger>
          </TooltipTrigger>
          <TooltipContent>{tf("vouchers.batches.actionsFor", { batch: `#${shortBatch(batch.id)}` })}</TooltipContent>
        </Tooltip>
        <DropdownMenuContent align="end" className="w-60">
          <DropdownMenuItem className="min-h-10" onClick={() => onOpenDetail(batch)}>
            <Info className="size-4" />
            {tf("vouchers.batches.detailOpen", { batch: `#${shortBatch(batch.id)}` })}
          </DropdownMenuItem>
          <DropdownMenuItem className="min-h-10" onClick={() => onViewVouchers(batch)}>
            <Eye className="size-4" />
            {t("vouchers.batches.view")}
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem
            className="min-h-10"
            disabled={multiPrintBatch?.id === batch.id}
            onClick={() => onPrintMulti(batch)}
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
            onClick={() => onPrintSimple(batch)}
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
            onClick={() => onTransfer(batch)}
          >
            <ArrowLeftRight className="size-4" />
            {t("vouchers.batches.transfer")}
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem variant="destructive" className="min-h-10" onClick={() => onDeleteBatch(batch)}>
            <Trash2 className="size-4" />
            {t("common.deleteBatch")}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    );
  }

  return (
    <>
      {/* v2 — « tour de contrôle » : Stock vivant → Chez revendeurs → Éculés 7 j.
          Les étapes cliquables filtrent la liste ; chips d'alerte si pertinentes. */}
      {firstLoading ? (
        <Card className="gap-0 py-0">
          <CardContent className="p-4">
            <div className="h-12 animate-pulse rounded-md bg-muted" aria-hidden />
          </CardContent>
        </Card>
      ) : (
        batchSummary && (
          <BatchPipeline
            summary={batchSummary}
            statusFilter={batchStatusFilter}
            holderFilter={batchHolderFilter}
            onStatusFilter={onBatchStatusFilter}
            onHolderFilter={onBatchHolderFilter}
            money={(amount) => formatCurrency(amount, currency, lang)}
          />
        )
      )}

      {/* Filtres des lots — desktop (md+) : une seule rangée compacte */}
      <Card className="hidden gap-0 py-0 md:block">
        <CardContent className="flex items-center gap-2 p-3 lg:gap-3">
          <div className="relative min-w-0 max-w-56 flex-1">
            <Search className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" aria-hidden />
            <Input
              className="h-10 pl-9"
              placeholder={t("vouchers.batches.searchPlaceholder")}
              value={batchSearchInput}
              onChange={(event) => onBatchSearchInput(event.target.value)}
              aria-label={t("vouchers.batches.searchLabel")}
            />
          </div>
          {batchSiteSelect("h-10 w-36 shrink-0 xl:w-40")}
          {batchChannelSelect("h-10 w-36 shrink-0")}
          {batchStatusSelect("h-10 w-36 shrink-0")}
          {batchHolderSelect("h-10 w-36 shrink-0 xl:w-40")}
          <Button
            variant="outline"
            size="icon"
            className="size-10 shrink-0"
            onClick={onExportBatches}
            disabled={exportingBatches}
            title={t("vouchers.batches.exportCsv")}
            aria-label={t("vouchers.batches.exportCsv")}
          >
            {exportingBatches ? <Loader2 className="size-4 animate-spin" /> : <Download className="size-4" />}
          </Button>
          {hasBatchFilters && (
            <Button
              variant="ghost"
              size="icon"
              className="size-10 shrink-0"
              onClick={onResetFilters}
              title={t("vouchers.batches.resetFilters")}
              aria-label={t("vouchers.batches.resetFilters")}
            >
              <RotateCcw className="size-4" />
            </Button>
          )}
        </CardContent>
      </Card>

      {/* Filtres mobile (< md) — UN seul bouton « Filtres » (+ badge du nombre
          de filtres actifs) qui ouvre un sheet bottom avec tout le panneau. */}
      <div className="md:hidden">
        <Button
          variant="outline"
          className="h-10 w-full justify-between"
          onClick={() => onFiltersOpenChange(true)}
          aria-label={t("vouchers.batches.filters")}
        >
          <span className="inline-flex items-center gap-2">
            <Filter className="size-4" aria-hidden />
            {t("vouchers.batches.filters")}
          </span>
          {batchActiveFilterCount > 0 && (
            <Badge variant="secondary" className="tabular-nums">
              {tf("vouchers.batches.filters.active", {
                n: batchActiveFilterCount,
                p: batchActiveFilterCount > 1 ? "s" : "",
              })}
            </Badge>
          )}
        </Button>
        <Sheet open={filtersOpen} onOpenChange={onFiltersOpenChange}>
          <SheetContent side="bottom" className="max-h-[85dvh] overflow-y-auto">
            <SheetHeader>
              <SheetTitle>{t("vouchers.batches.filters")}</SheetTitle>
              <SheetDescription>{t("vouchers.batches.filtersDesc")}</SheetDescription>
            </SheetHeader>
            <div className="flex flex-col gap-3 px-4 pb-6">
              <div className="relative">
                <Search className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" aria-hidden />
                <Input
                  className="h-10 pl-9"
                  placeholder={t("vouchers.batches.searchPlaceholder")}
                  value={batchSearchInput}
                  onChange={(event) => onBatchSearchInput(event.target.value)}
                  aria-label={t("vouchers.batches.searchLabel")}
                />
              </div>
              {batchSiteSelect("h-10 w-full")}
              {batchChannelSelect("h-10 w-full")}
              {batchStatusSelect("h-10 w-full")}
              {batchHolderSelect("h-10 w-full")}
              <div className="flex gap-2 pt-1">
                <Button
                  variant="outline"
                  className="h-10 flex-1"
                  onClick={onExportBatches}
                  disabled={exportingBatches}
                >
                  {exportingBatches ? <Loader2 className="size-4 animate-spin" /> : <Download className="size-4" />}
                  {t("vouchers.batches.exportCsv")}
                </Button>
                {hasBatchFilters && (
                  <Button variant="ghost" className="h-10" onClick={onResetFilters}>
                    <RotateCcw className="size-4" />
                    {t("vouchers.batches.resetFilters")}
                  </Button>
                )}
              </div>
            </div>
          </SheetContent>
        </Sheet>
      </div>

      {/* Table des lots */}
      <Card className="gap-0 py-0">
        {firstLoading ? (
          <LoadingRows rows={8} />
        ) : batches.length === 0 ? (
          <EmptyState
            icon={Layers}
            title={t("vouchers.batches.empty")}
            description={hasBatchFilters ? t("vouchers.batches.emptyFiltered") : t("vouchers.emptyDesc")}
            action={
              !hasBatchFilters && (
                <Button onClick={onGenerate}>
                  <TicketPlus className="size-4" />
                  {t("vouchers.generate")}
                </Button>
              )
            }
          />
        ) : (
          <>
            {/* Cartes mobiles (< md) — la donnée d'abord : un lot = une carte */}
            <div className="md:hidden">
              {batches.map((batch) => {
                const velocity = velocityInfo(batch);
                return (
                  <div
                    key={batch.id}
                    role="button"
                    tabIndex={0}
                    aria-label={tf("vouchers.batches.detailOpen", { batch: batch.id })}
                    className="cursor-pointer space-y-2.5 border-b p-4 transition-colors last:border-b-0 hover:bg-muted/40 focus-visible:bg-muted/40 focus-visible:outline-none"
                    onClick={() => onOpenDetail(batch)}
                    onKeyDown={(event) => {
                      if (event.key === "Enter" || event.key === " ") {
                        event.preventDefault();
                        onOpenDetail(batch);
                      }
                    }}
                  >
                    {/* Ligne 1 — identité : #id + cycle de vie + date */}
                    <div className="flex items-center justify-between gap-2">
                      <span className="inline-flex items-center gap-1.5">
                        <span className="font-mono text-sm font-semibold tabular-nums">#{shortBatch(batch.id)}</span>
                        <BatchLifeBadge status={batch.status} />
                      </span>
                      <span className="text-xs text-muted-foreground">{formatDate(batch.createdAt, lang)}</span>
                    </div>
                    {/* Ligne 2 — profil */}
                    <p className="truncate text-xs text-muted-foreground">{batch.profileName}</p>
                    {/* Cycle de vie — la barre signature (chiffres dans les segments) */}
                    <BatchLifeBar
                      count={batch.count}
                      active={batch.active}
                      used={batch.used}
                      expired={batch.expired}
                      disabled={batch.disabled}
                    />
                    {/* Ligne 3 — détention (chips live) + valeur faciale */}
                    <div className="flex flex-wrap items-center justify-between gap-2">
                      {holdingsChips(batch)}
                      <span className="text-sm font-semibold tabular-nums">
                        {formatCurrency(batch.stockFace, currency, lang)}
                      </span>
                    </div>
                    {/* Ligne 4 — vélocité / dormance */}
                    {velocity && (
                      <p
                        className={cn(
                          "text-xs",
                          velocity.amber ? "text-amber-600 dark:text-amber-400" : "text-muted-foreground",
                        )}
                      >
                        {velocity.main}
                        {velocity.sub && <span> · {velocity.sub}</span>}
                      </p>
                    )}
                    {/* Actions — 3 boutons icônes en bas à droite */}
                    <div className="flex items-center justify-end gap-1" onClick={(event) => event.stopPropagation()}>
                      {transferIconButton(batch, "size-10")}
                      {printIconButton(batch, "size-10")}
                      {batchActionsMenu(batch, "size-10")}
                    </div>
                  </div>
                );
              })}
            </div>

            {/* Table (md+) — 6 colonnes resserrées, zéro débordement à 1440 px */}
            <div className="hidden overflow-x-auto md:block">
              <Table>
                <TableHeader>
                  <TableRow className="hover:bg-transparent">
                    <TableHead className="pl-4 text-muted-foreground sm:pl-6">{t("common.batch")}</TableHead>
                    <TableHead className="text-muted-foreground">{t("vouchers.batches.life")}</TableHead>
                    <TableHead className="text-muted-foreground">{t("vouchers.batches.table.holding")}</TableHead>
                    <TableHead className="text-muted-foreground">{t("vouchers.batches.table.velocity")}</TableHead>
                    <TableHead className="text-right text-muted-foreground">{t("vouchers.batches.table.value")}</TableHead>
                    <TableHead className="pr-4 text-right text-muted-foreground sm:pr-6">{t("common.actions")}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {batches.map((batch) => {
                    const velocity = velocityInfo(batch);
                    return (
                      <TableRow
                        key={batch.id}
                        className="cursor-pointer"
                        onClick={() => onOpenDetail(batch)}
                      >
                        {/* Lot — #id + badge, profil, date · site (profil + site fusionnés ici) */}
                        <TableCell className="pl-4 sm:pl-6">
                          <div className="flex max-w-52 flex-col items-start gap-0.5">
                            <span className="inline-flex items-center gap-1.5">
                              <span className="font-mono text-sm font-semibold tabular-nums">#{shortBatch(batch.id)}</span>
                              <BatchLifeBadge status={batch.status} />
                            </span>
                            <span className="max-w-52 truncate text-xs text-muted-foreground">{batch.profileName}</span>
                            <span className="max-w-52 truncate text-xs text-muted-foreground">
                              {formatDate(batch.createdAt, lang)} · {batch.routerName}
                            </span>
                          </div>
                        </TableCell>
                        {/* Cycle de vie — la barre signature avec chiffres dans les segments */}
                        <TableCell>
                          <BatchLifeBar
                            count={batch.count}
                            active={batch.active}
                            used={batch.used}
                            expired={batch.expired}
                            disabled={batch.disabled}
                            className="w-40"
                          />
                        </TableCell>
                        {/* Détention — chips live du stock vendable */}
                        <TableCell>{holdingsChips(batch)}</TableCell>
                        {/* Vélocité — sorties 7 j + projection, ou dormance */}
                        <TableCell>
                          {velocity ? (
                            <div>
                              <p
                                className={cn(
                                  "text-sm font-medium tabular-nums",
                                  velocity.amber && "text-amber-600 dark:text-amber-400",
                                )}
                              >
                                {velocity.main}
                              </p>
                              {velocity.sub && (
                                <p className="text-xs text-muted-foreground">{velocity.sub}</p>
                              )}
                            </div>
                          ) : (
                            <span className="text-sm text-muted-foreground">—</span>
                          )}
                        </TableCell>
                        {/* Valeur — faciale puis gros (le « Coût » a disparu) */}
                        <TableCell className="text-right">
                          <p className="text-sm font-medium tabular-nums">
                            {formatCurrency(batch.stockFace, currency, lang)}
                          </p>
                          <p className="text-xs text-muted-foreground">
                            {tf("vouchers.batches.pipeline.resellersSub", {
                              value: formatCurrency(batch.transferableValue, currency, lang),
                            })}
                          </p>
                        </TableCell>
                        {/* Actions — 3 boutons icône UNIQUEMENT ; la ligne reste cliquable */}
                        <TableCell className="pr-4 text-right sm:pr-6" onClick={(event) => event.stopPropagation()}>
                          <div className="flex items-center justify-end gap-0.5">
                            {transferIconButton(batch)}
                            {printIconButton(batch)}
                            {batchActionsMenu(batch)}
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
                {isFetching
                  ? t("common.refreshing")
                  : tf("vouchers.batches.range", { start: batchRangeStart, end: batchRangeEnd, total: batchTotal })}
              </p>
              <div className="flex items-center gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  className="h-10"
                  onClick={() => onSetBatchPage((p) => Math.max(1, p - 1))}
                  disabled={batchSafePage <= 1}
                >
                  <ChevronLeft className="size-4" />
                  {t("common.previous")}
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  className="h-10"
                  onClick={() => onSetBatchPage((p) => Math.min(batchMaxPage, p + 1))}
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
  );
}
