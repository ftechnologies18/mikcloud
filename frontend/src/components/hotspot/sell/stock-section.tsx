"use client";

// Section « Stock » du comptoir Mode Vente — le bloc <main> : squelettes,
// état vide, barre du stock (comptage + bascule de vue + imprimer tout),
// recherche locale R3, bannière de vente auto, file hors-ligne R6, les
// DEUX vues (groupée profil → lot, ou liste plate « récents »), la
// sélection par lot du mode retour et la pagination « Afficher plus ».
// Présentation pure : l'état, la recherche et la sélection vivent dans
// sell-shell.tsx ; ce composant rend ce qu'on lui passe.

import {
  CheckCheck,
  ChevronDown,
  CloudUpload,
  Layers,
  Loader2,
  Printer,
  Search,
  Store,
  Wifi,
  X,
} from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { useI18n } from "@/lib/hotspot/i18n";
import { formatCurrency } from "@/lib/hotspot/format";
import type { SellPrintScope } from "@/components/hotspot/parts/sell-print-dialog";
import {
  batchExpiringSoon,
  fmtDay,
  NO_BATCH,
  type SellProfileGroup,
  type SellVoucher,
} from "./helpers";
import { VoucherCard } from "./voucher-card";

interface StockSectionProps {
  // Chargement / données
  isLoading: boolean;
  stock: SellVoucher[] | undefined;
  stockTotal: number;
  hasNextPage: boolean;
  isFetchingNextPage: boolean;
  onLoadMore: () => void;
  // Vue (R1) + filtre (R3)
  view: "profile" | "recent";
  onChangeView: (view: "profile" | "recent") => void;
  query: string;
  onQueryChange: (value: string) => void;
  searching: boolean;
  filteredStock: SellVoucher[];
  filteredGroups: SellProfileGroup[];
  // Groupes repliés (R1)
  collapsedGroups: Set<string>;
  onToggleGroup: (key: string) => void;
  // Mode retour (N°20) + sélection
  returnMode: boolean;
  selected: Set<string>;
  onToggleSelected: (id: string) => void;
  onToggleBatchSelection: (batch: { vouchers: SellVoucher[] }) => void;
  // File hors-ligne (UX R6) — ids en attente de replay
  queuedIds: Set<string>;
  // Vente
  sellPending: boolean;
  sellPendingId: string | null;
  onSell: (voucher: SellVoucher) => void;
  // Impression
  onOpenPrint: (scope: SellPrintScope) => void;
  currency: string;
}

export function StockSection({
  isLoading,
  stock,
  stockTotal,
  hasNextPage,
  isFetchingNextPage,
  onLoadMore,
  view,
  onChangeView,
  query,
  onQueryChange,
  searching,
  filteredStock,
  filteredGroups,
  collapsedGroups,
  onToggleGroup,
  returnMode,
  selected,
  onToggleSelected,
  onToggleBatchSelection,
  queuedIds,
  sellPending,
  sellPendingId,
  onSell,
  onOpenPrint,
  currency,
}: StockSectionProps) {
  const { t, tf, lang } = useI18n();

  return (
    <main className="flex-1 space-y-3 p-4" aria-label={t("sell.stock")}>
      {isLoading ? (
        Array.from({ length: 4 }).map((_, i) => <Skeleton key={i} className="h-28 w-full" />)
      ) : !stock || stock.length === 0 ? (
        <Card>
          <CardContent className="flex flex-col items-center gap-3 p-8 text-center">
            <span className="flex size-12 items-center justify-center rounded-xl bg-primary/10 text-primary">
              <Store className="size-6" />
            </span>
            <p className="font-medium">{t("sell.empty")}</p>
            <p className="text-sm text-muted-foreground">{t("sell.emptyDesc")}</p>
          </CardContent>
        </Card>
      ) : (
        <>
          {/* UX R1 — barre du stock : comptage + bascule de vue. */}
          <div className="flex items-center justify-between gap-3">
            <p className="text-sm text-muted-foreground">
              {tf("sell.stockCountLabel", { count: searching ? filteredStock.length : stock.length })}
            </p>
            <div className="flex items-center gap-2">
              {!returnMode && (
                <Button
                  variant="outline"
                  size="sm"
                  className="min-h-9 shrink-0"
                  onClick={() => onOpenPrint({ kind: "all" })}
                  disabled={stock.length === 0}
                >
                  <Printer className="size-4" />
                  <span className="hidden min-[420px]:inline">{t("sell.printAll")}</span>
                </Button>
              )}
              <div className="flex rounded-lg border bg-muted/30 p-0.5" role="group" aria-label={t("sell.viewLabel")}>
                <button
                  type="button"
                  onClick={() => onChangeView("profile")}
                  aria-pressed={view === "profile"}
                  className={`min-h-9 rounded-md px-3 text-xs font-medium transition-colors ${
                    view === "profile" ? "bg-background text-foreground shadow-sm" : "text-muted-foreground hover:text-foreground"
                  }`}
                >
                  {t("sell.viewProfile")}
                </button>
                <button
                  type="button"
                  onClick={() => onChangeView("recent")}
                  aria-pressed={view === "recent"}
                  className={`min-h-9 rounded-md px-3 text-xs font-medium transition-colors ${
                    view === "recent" ? "bg-background text-foreground shadow-sm" : "text-muted-foreground hover:text-foreground"
                  }`}
                >
                  {t("sell.viewRecent")}
                </button>
              </div>
            </div>
          </div>

          {/* UX R3 — recherche locale : code, profil ou référence de lot.
              Les groupes correspondants se déplient automatiquement. */}
          <div className="relative">
            <Search aria-hidden className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              type="text"
              inputMode="search"
              value={query}
              onChange={(e) => onQueryChange(e.target.value)}
              placeholder={t("sell.searchPlaceholder")}
              aria-label={t("sell.searchPlaceholder")}
              autoCapitalize="none"
              autoCorrect="off"
              spellCheck={false}
              className="h-10 pl-9 pr-9"
            />
            {searching && (
              <button
                type="button"
                onClick={() => onQueryChange("")}
                aria-label={t("sell.searchClear")}
                className="absolute right-1.5 top-1/2 -translate-y-1/2 rounded-md p-1.5 text-muted-foreground transition-colors hover:text-foreground"
              >
                <X className="size-4" aria-hidden />
              </button>
            )}
          </div>

          {/* UX R5 — la saisie papier (R3) est retirée : elle compliquait
              l'UX (3ᵉ mode de vente, codes à taper — risque d'homoglyphes)
              et est devenue redondante. Le ticket papier remis au client se
              vend tout seul à sa 1ʳᵉ connexion hotspot (auto_connect,
              idempotent) ; la vente tactile reste le geste « je
              comptabilise maintenant ». La bannière garde le flux visible —
              le vendeur comprend pourquoi son stock baisse « tout seul ». */}
          {!returnMode && (
            <div
              role="note"
              aria-label={t("sell.autoSaleTitle")}
              className="flex items-start gap-2.5 rounded-xl border bg-muted/20 p-3"
            >
              <Wifi aria-hidden className="mt-0.5 size-4 shrink-0 text-primary" />
              <div className="min-w-0">
                <p className="text-sm font-semibold">{t("sell.autoSaleTitle")}</p>
                <p className="mt-0.5 text-xs text-muted-foreground">{t("sell.autoSaleDesc")}</p>
              </div>
            </div>
          )}

          {/* UX R6 — file de ventes hors-ligne : tout ce qui est parti sans
              réseau attend ici, et repartira tout seul au retour du signal
              (replay 409-safe — jamais de double décompte). */}
          {queuedIds.size > 0 && (
            <div
              role="status"
              aria-live="polite"
              className="flex items-start gap-2.5 rounded-xl border border-amber-500/40 bg-amber-500/10 p-3"
            >
              <CloudUpload aria-hidden className="mt-0.5 size-4 shrink-0 text-amber-700 dark:text-amber-300" />
              <div className="min-w-0">
                <p className="text-sm font-semibold text-amber-700 dark:text-amber-300">
                  {t("sell.queueBannerTitle")}
                </p>
                <p className="mt-0.5 text-xs text-muted-foreground">
                  {tf("sell.queueBannerDesc", { count: queuedIds.size })}
                </p>
              </div>
            </div>
          )}

          {searching && filteredStock.length === 0 ? (
            <Card>
              <CardContent className="p-6 text-center text-sm text-muted-foreground">
                {t("sell.searchNoResult")}
              </CardContent>
            </Card>
          ) : view === "recent" ? (
            filteredStock.map((v) => (
              <VoucherCard
                key={v.id}
                voucher={v}
                currency={currency}
                returnMode={returnMode}
                isSelected={selected.has(v.id)}
                isQueued={queuedIds.has(v.id)}
                sellPending={sellPending}
                sellPendingId={sellPendingId}
                onToggle={onToggleSelected}
                onSell={onSell}
              />
            ))
          ) : (
            filteredGroups.map((g) => {
              const groupKey = `profil:${g.profileName}`;
              // UX R3 — en recherche, tous les groupes correspondants sont
              // dépliés (le filtre remplace l'état de repliage mémorisé).
              const isCollapsed = !searching && collapsedGroups.has(groupKey);
              return (
                <section key={g.profileName} aria-label={g.profileName} className="space-y-3">
                  <button
                    type="button"
                    onClick={() => onToggleGroup(groupKey)}
                    aria-expanded={!isCollapsed}
                    className="flex min-h-11 w-full items-center gap-2 rounded-xl border bg-muted/30 px-3 py-2 text-left transition-colors hover:bg-muted/60"
                  >
                    <ChevronDown
                      aria-hidden
                      className={`size-4 shrink-0 text-muted-foreground transition-transform ${isCollapsed ? "-rotate-90" : ""}`}
                    />
                    <span className="min-w-0 flex-1">
                      <span className="block truncate text-sm font-semibold">{g.profileName}</span>
                      <span className="block text-[11px] text-muted-foreground">
                        {tf("sell.groupMeta", { lots: g.batches.length })} · {formatCurrency(g.value, currency, lang)}
                      </span>
                    </span>
                    <Badge variant="secondary" className="shrink-0 tabular-nums">
                      {g.count}
                    </Badge>
                  </button>

                  {!isCollapsed &&
                    (g.hasLots ? (
                      g.batches.map((b) => {
                        const batchAllIn = returnMode && b.vouchers.every((v) => selected.has(v.id));
                        return (
                          <div key={b.key} className="space-y-3">
                            <div className="flex items-center gap-1.5 px-1">
                              <Layers aria-hidden className="size-3 shrink-0 text-muted-foreground" />
                              <p className="min-w-0 flex-1 truncate text-[11px] font-medium uppercase tracking-wide text-muted-foreground">
                                {b.key === NO_BATCH ? t("sell.lotNone") : tf("sell.lotLabel", { id: b.labelId })} ·{" "}
                                {fmtDay(b.createdAt, lang)} · {tf("sell.lotCount", { count: b.vouchers.length })}
                                {batchExpiringSoon(b.vouchers) && (
                                  <span className="text-amber-600 dark:text-amber-400">
                                    {" "}· {t("sell.lotExpiring")}
                                  </span>
                                )}
                              </p>
                              {!returnMode && (
                                <button
                                  type="button"
                                  onClick={() =>
                                    onOpenPrint({
                                      kind: "batch",
                                      batchId: b.key === NO_BATCH ? null : b.key,
                                      label: b.key === NO_BATCH ? t("sell.lotNone") : b.labelId,
                                    })
                                  }
                                  aria-label={t("sell.printLotAria")}
                                  className="flex size-8 shrink-0 items-center justify-center rounded-md border bg-background text-muted-foreground transition-colors hover:bg-muted/60 hover:text-foreground"
                                >
                                  <Printer className="size-3.5" aria-hidden />
                                </button>
                              )}
                              {returnMode && (
                                <button
                                  type="button"
                                  onClick={() => onToggleBatchSelection(b)}
                                  aria-label={t("sell.lotSelectAllAria")}
                                  aria-pressed={batchAllIn}
                                  className={`flex min-h-8 shrink-0 items-center gap-1 rounded-md border px-2 text-[11px] font-medium transition-colors ${
                                    batchAllIn
                                      ? "border-primary bg-primary text-primary-foreground"
                                      : "bg-background text-muted-foreground hover:bg-muted/60 hover:text-foreground"
                                  }`}
                                >
                                  <CheckCheck className="size-3.5" aria-hidden />
                                  {t("sell.lotSelectAll")}
                                </button>
                              )}
                            </div>
                            {b.vouchers.map((v) => (
                              <VoucherCard
                                key={v.id}
                                voucher={v}
                                currency={currency}
                                returnMode={returnMode}
                                isSelected={selected.has(v.id)}
                                isQueued={queuedIds.has(v.id)}
                                sellPending={sellPending}
                                sellPendingId={sellPendingId}
                                onToggle={onToggleSelected}
                                onSell={onSell}
                              />
                            ))}
                          </div>
                        );
                      })
                    ) : (
                      g.batches.flatMap((b) => b.vouchers).map((v) => (
                        <VoucherCard
                          key={v.id}
                          voucher={v}
                          currency={currency}
                          returnMode={returnMode}
                          isSelected={selected.has(v.id)}
                          isQueued={queuedIds.has(v.id)}
                          sellPending={sellPending}
                          sellPendingId={sellPendingId}
                          onToggle={onToggleSelected}
                          onSell={onSell}
                        />
                      ))
                    ))}
                </section>
              );
            })
          )}

          {/* P3-e — pagination du stock : les pages suivantes se chargent
              sur demande (un gros stock ne casse plus le premier rendu). */}
          {hasNextPage && (
            <div className="flex flex-col items-center gap-1 py-1">
              <Button
                variant="outline"
                onClick={onLoadMore}
                disabled={isFetchingNextPage}
                className="min-h-11 w-full max-w-xs"
              >
                {isFetchingNextPage ? (
                  <Loader2 className="size-4 animate-spin" aria-hidden />
                ) : (
                  <ChevronDown className="size-4" aria-hidden />
                )}
                {tf("sell.loadMore", {
                  shown: stock?.length ?? 0,
                  total: stockTotal,
                })}
              </Button>
              {!searching && (
                <p className="text-[11px] text-muted-foreground">
                  {t("sell.loadMoreHint")}
                </p>
              )}
            </div>
          )}
        </>
      )}
    </main>
  );
}
