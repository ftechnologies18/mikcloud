"use client";

// Onglet « Vouchers » de la vue Vouchers — présentation pure : statistiques
// du stock (KPI serveur N°74), barre de filtres (recherche debouncée, statut,
// détenteur, profil), table des tickets (codes, mots de passe révélables,
// quota, prix, statut, lot cliquable) et pagination.
// L'état et les requêtes vivent dans le shell (views/vouchers-view.tsx) :
// ce composant ne fait que rendre ce qu'on lui passe.

import {
  CheckCircle2,
  ChevronLeft,
  ChevronRight,
  Clock,
  Copy,
  Loader2,
  Lock,
  MoreHorizontal,
  Printer,
  RefreshCcw,
  Search,
  ShieldQuestion,
  Ticket,
  TicketPlus,
  Trash2,
  Undo2,
  Wallet,
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
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { EmptyState } from "@/components/hotspot/empty-state";
import { LoadingCards, LoadingRows } from "@/components/hotspot/loading";
import { StatCard } from "@/components/hotspot/stat-card";
import { StatusBadge } from "@/components/hotspot/status-badge";
import { useCurrency } from "@/components/hotspot/parts/sd-currency";
import { PasswordCell } from "@/components/hotspot/parts/uc-password-cell";
import { useI18n } from "@/lib/hotspot/i18n";
import { formatBytes, formatCurrency, formatDate } from "@/lib/hotspot/format";
import type { HotspotUser, Profile } from "@/lib/hotspot/types";
import { PAGE_SIZE, STATUS_OPTIONS, shortBatch } from "./shared";

interface VouchersTabProps {
  // Statistiques du stock (compteurs serveur N°74)
  statsLoading: boolean;
  activeCount: number;
  usedCount: number;
  expiredCount: number;
  allocatedCount: number;
  stockValue: number;
  // Filtres (l'état vit dans le shell — callbacks de changement)
  searchInput: string;
  onSearchInput: (value: string) => void;
  statusFilter: string;
  onStatusFilter: (value: string) => void;
  holderFilter: string;
  onHolderFilter: (value: string) => void;
  profileFilter: string;
  onProfileFilter: (value: string) => void;
  profiles: Profile[] | undefined;
  // Table des vouchers
  isLoading: boolean;
  isFetching: boolean;
  vouchers: HotspotUser[];
  hasFilters: boolean;
  revealed: Set<string>;
  onToggleReveal: (id: string) => void;
  printingVoucherId: string | null;
  reprisePendingId: string | null;
  resyncPendingId: string | null;
  onGenerate: () => void;
  onCopyCode: (voucher: HotspotUser) => void;
  onPrintOne: (voucher: HotspotUser) => void;
  onReprise: (voucher: HotspotUser) => void;
  onResync: (voucher: HotspotUser, action: "recreate" | "forget") => void;
  onDelete: (voucher: HotspotUser) => void;
  onFilterByBatch: (batchId: string) => void;
  // Pagination
  totalCount: number;
  page: number;
  onSetPage: (updater: (page: number) => number) => void;
}

export function VouchersTab({
  statsLoading,
  activeCount,
  usedCount,
  expiredCount,
  allocatedCount,
  stockValue,
  searchInput,
  onSearchInput,
  statusFilter,
  onStatusFilter,
  holderFilter,
  onHolderFilter,
  profileFilter,
  onProfileFilter,
  profiles,
  isLoading,
  isFetching,
  vouchers,
  hasFilters,
  revealed,
  onToggleReveal,
  printingVoucherId,
  reprisePendingId,
  resyncPendingId,
  onGenerate,
  onCopyCode,
  onPrintOne,
  onReprise,
  onResync,
  onDelete,
  onFilterByBatch,
  totalCount,
  page,
  onSetPage,
}: VouchersTabProps) {
  const { t, tf, lang } = useI18n();
  const currency = useCurrency();

  // Pagination — bornes recalculées localement (PAGE_SIZE partagé, mêmes formules
  // que le shell d'origine : safePage = min(page, maxPage)).
  const maxPage = Math.max(1, Math.ceil(totalCount / PAGE_SIZE));
  const safePage = Math.min(page, maxPage);
  const rangeStart = totalCount === 0 ? 0 : (safePage - 1) * PAGE_SIZE + 1;
  const rangeEnd = Math.min(safePage * PAGE_SIZE, totalCount);

  return (
    <>
      {/* Statistiques du stock */}
      {statsLoading ? (
        <LoadingCards cards={5} />
      ) : (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-5">
          <StatCard title={t("vouchers.kpi.active")} value={String(activeCount)} sub={t("vouchers.kpi.activeSub")} icon={Ticket} />
          <StatCard title={t("vouchers.kpi.used")} value={String(usedCount)} sub={t("vouchers.kpi.usedSub")} icon={CheckCircle2} />
          <StatCard title={t("vouchers.kpi.expired")} value={String(expiredCount)} sub={t("vouchers.kpi.expiredSub")} icon={Clock} />
          {/* N°23 (W3/W4) — stock confié aux revendeurs, visible d'un coup d'œil. */}
          <StatCard
            title={t("vouchers.kpiAllocated")}
            value={String(allocatedCount)}
            sub={t("vouchers.kpiAllocatedSub")}
            icon={Undo2}
          />
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
              onChange={(event) => onSearchInput(event.target.value)}
              aria-label={t("vouchers.searchLabel")}
            />
          </div>
          <div className="flex flex-1 flex-wrap gap-3 sm:justify-end">
            <Select value={statusFilter} onValueChange={onStatusFilter}>
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
            {/* N°23 (W3/W4) — détenteur du stock : direct vs revendeurs. */}
            <Select value={holderFilter} onValueChange={onHolderFilter}>
              <SelectTrigger className="h-10 w-full sm:w-44" aria-label={t("vouchers.holderLabel")}>
                <SelectValue placeholder={t("vouchers.holderLabel")} />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">{t("vouchers.holderAll")}</SelectItem>
                <SelectItem value="direct">{t("vouchers.holderDirect")}</SelectItem>
                <SelectItem value="reseller">{t("vouchers.holderReseller")}</SelectItem>
              </SelectContent>
            </Select>
            <Select value={profileFilter} onValueChange={onProfileFilter}>
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
                <Button onClick={onGenerate}>
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
                          {voucher.resellerId ? (
                            // N°22 — ticket revendeur : code masqué côté serveur,
                            // copie impossible (anti-vente en direct) ; l'impression
                            // tracée reste disponible dans le menu d'actions.
                            <span
                              className="inline-flex size-9 items-center justify-center text-muted-foreground"
                              title={tf("vouchers.resellerLocked", { name: voucher.resellerName })}
                            >
                              <Lock className="size-4" aria-hidden />
                              <span className="sr-only">
                                {tf("vouchers.resellerLocked", { name: voucher.resellerName })}
                              </span>
                            </span>
                          ) : (
                            <Button
                              variant="ghost"
                              size="icon"
                              className="size-9 text-muted-foreground hover:text-foreground"
                              onClick={() => onCopyCode(voucher)}
                              aria-label={tf("vouchers.copyCodeAria", { code: voucher.username })}
                              title={t("vouchers.copyCode")}
                            >
                              <Copy className="size-4" />
                            </Button>
                          )}
                        </span>
                      </TableCell>
                      <TableCell>
                        {voucher.resellerId ? (
                          <span className="font-mono text-sm tracking-widest text-muted-foreground">••••••</span>
                        ) : (
                          <PasswordCell
                            password={voucher.password}
                            visible={revealed.has(voucher.id)}
                            onToggle={() => onToggleReveal(voucher.id)}
                            label={
                              revealed.has(voucher.id)
                                ? tf("vouchers.hidePassword", { name: voucher.username })
                                : tf("vouchers.showPassword", { name: voucher.username })
                            }
                          />
                        )}
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
                            onClick={() => onFilterByBatch(voucher.batchId)}
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
                            <DropdownMenuItem
                              className="min-h-10"
                              disabled={printingVoucherId === voucher.id}
                              onClick={() => onPrintOne(voucher)}
                            >
                              {printingVoucherId === voucher.id ? (
                                <Loader2 className="size-4 animate-spin" />
                              ) : (
                                <Printer className="size-4" />
                              )}
                              {t("vouchers.printOne")}
                            </DropdownMenuItem>
                            {/* N°23 (W6) — reprise gérant : uniquement les tickets
                                revendeur invendus (soldAt vide = pas encore remis
                                au client ; auto_connect pose SoldAt à la 1ʳᵉ connexion). */}
                            {voucher.resellerId && !voucher.soldAt && (
                              <>
                                <DropdownMenuSeparator />
                                <DropdownMenuItem
                                  className="min-h-10"
                                  disabled={reprisePendingId === voucher.id}
                                  onClick={() => onReprise(voucher)}
                                >
                                  {reprisePendingId === voucher.id ? (
                                    <Loader2 className="size-4 animate-spin" />
                                  ) : (
                                    <Undo2 className="size-4" />
                                  )}
                                  {t("vouchers.reprise.action")}
                                </DropdownMenuItem>
                              </>
                            )}
                            {/* N — resynchronisation (uniquement si absent du routeur). */}
                            {voucher.missingOnRouter && (
                              <>
                                <DropdownMenuSeparator />
                                <DropdownMenuItem
                                  className="min-h-10"
                                  disabled={resyncPendingId === voucher.id}
                                  onClick={() => onResync(voucher, "recreate")}
                                >
                                  {resyncPendingId === voucher.id ? (
                                    <Loader2 className="size-4 animate-spin" />
                                  ) : (
                                    <RefreshCcw className="size-4" />
                                  )}
                                  {t("users.resyncRecreate")}
                                </DropdownMenuItem>
                                <DropdownMenuItem
                                  className="min-h-10"
                                  disabled={resyncPendingId === voucher.id}
                                  onClick={() => {
                                    if (window.confirm(t("users.resyncForgetConfirm"))) {
                                      onResync(voucher, "forget");
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
                              // N°23 (W1) — le stock revendeur n'est pas destructible
                              // depuis la console (le serveur refuse en 403 ; on
                              // neutralise l'action pour éviter l'aller-retour).
                              disabled={!!voucher.resellerId}
                              title={voucher.resellerId ? t("vouchers.deleteBlocked") : undefined}
                              onClick={() => onDelete(voucher)}
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
                  onClick={() => onSetPage((p) => Math.max(1, p - 1))}
                  disabled={safePage <= 1}
                >
                  <ChevronLeft className="size-4" />
                  {t("common.previous")}
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  className="h-10"
                  onClick={() => onSetPage((p) => Math.min(maxPage, p + 1))}
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
  );
}
