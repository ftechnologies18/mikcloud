"use client";

// Fiche 360° du lot (refonte onglet Lots) — drawer latéral alimenté par la
// ligne cliquée (aucun appel réseau : les stats live, holdings et cycle de
// vie sont déjà dans BatchWithStats). Une seule fiche pour comprendre la vie
// du lot : caractéristiques, composition du stock, stock vendable, possession,
// et toutes les actions (impression multi-formats, transfert, vouchers, purge).

import {
  ArrowLeftRight,
  Eye,
  Printer,
  Ticket,
  Trash2,
  TriangleAlert,
} from "lucide-react";

import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { BatchLifeBadge, BatchLifeBar } from "@/components/hotspot/parts/batch-life-bar";
import { useI18n } from "@/lib/hotspot/i18n";
import { formatBytes, formatCurrency, formatDateTime } from "@/lib/hotspot/format";
import type { BatchWithStats } from "@/lib/hotspot/types";

interface BatchDetailSheetProps {
  batch: BatchWithStats | null;
  currency: string;
  onOpenChange: (open: boolean) => void;
  onTransfer: (batch: BatchWithStats) => void;
  onPrintMulti: (batch: BatchWithStats) => void;
  onPrintSimple: (batch: BatchWithStats) => void;
  onViewVouchers: (batch: BatchWithStats) => void;
  onDelete: (batch: BatchWithStats) => void;
}

/** Ligne « caractéristiques » de la fiche (dt/dd stylés). */
function InfoRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-baseline justify-between gap-3">
      <dt className="shrink-0 text-xs text-muted-foreground">{label}</dt>
      <dd className="truncate text-right text-sm font-medium">{value}</dd>
    </div>
  );
}

/** Compteur de la composition (Restants / En stock / Vendus / …). */
function LifeStat({ label, value, accent }: { label: string; value: number; accent?: boolean }) {
  return (
    <div className="rounded-lg bg-muted/50 px-3 py-2">
      <p className="text-[11px] text-muted-foreground">{label}</p>
      <p className={`text-sm font-semibold tabular-nums ${accent && value > 0 ? "text-primary" : ""}`}>
        {value}
      </p>
    </div>
  );
}

export function BatchDetailSheet({
  batch,
  currency,
  onOpenChange,
  onTransfer,
  onPrintMulti,
  onPrintSimple,
  onViewVouchers,
  onDelete,
}: BatchDetailSheetProps) {
  const { t, tf, lang } = useI18n();

  // Parité Mikhmon : limit-uptime résolu à la génération (minutes).
  function formatTimeLimit(min: number): string {
    if (!min || min <= 0) return t("vouchers.batches.timeLimitNone");
    if (min % 1440 === 0) return tf("vouchers.batches.timeLimitDays", { n: min / 1440 });
    if (min % 60 === 0) return tf("vouchers.batches.timeLimitHours", { n: min / 60 });
    return tf("vouchers.batches.timeLimitMin", { n: min });
  }

  const holdings = batch?.holdings ?? [];
  const holdingsTotal = holdings.reduce((acc, h) => acc + h.count, 0);

  return (
    <Sheet open={batch !== null} onOpenChange={onOpenChange}>
      <SheetContent side="right" className="flex w-full flex-col gap-0 overflow-y-auto sm:max-w-md">
        {batch && (
          <>
            <SheetHeader className="pb-0">
              <div className="flex flex-wrap items-center gap-2">
                <SheetTitle className="font-mono">#{batch.id}</SheetTitle>
                <BatchLifeBadge status={batch.status} />
              </div>
              <SheetDescription>
                {batch.profileName} · {formatDateTime(batch.createdAt, lang)}
              </SheetDescription>
            </SheetHeader>

            <div className="flex-1 space-y-5 px-4 pb-6">
              {/* Cycle de vie — la composition du lot en un coup d'œil */}
              <section aria-label={t("vouchers.batches.detail.life")} className="space-y-3">
                <h3 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                  {t("vouchers.batches.detail.life")}
                </h3>
                <BatchLifeBar
                  count={batch.count}
                  active={batch.active}
                  used={batch.used}
                  expired={batch.expired}
                  disabled={batch.disabled}
                />
                <div className="grid grid-cols-3 gap-2">
                  <LifeStat label={t("vouchers.batches.statRemaining")} value={batch.remaining} />
                  <LifeStat label={t("vouchers.batches.life.stock")} value={batch.active} accent />
                  <LifeStat label={t("vouchers.batches.legendUsed")} value={batch.used} />
                  <LifeStat label={t("vouchers.batches.legendExpired")} value={batch.expired} />
                  <LifeStat label={t("vouchers.batches.legendDisabled")} value={batch.disabled} />
                  <LifeStat label={t("vouchers.batches.legendPurged")} value={Math.max(0, batch.count - batch.remaining - batch.used - batch.expired - batch.disabled)} />
                </div>
              </section>

              <Separator />

              {/* Stock vendable — matière première du transfert N°18 */}
              <section aria-label={t("vouchers.batches.detail.sellable")} className="space-y-3">
                <h3 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                  {t("vouchers.batches.detail.sellable")}
                </h3>
                <div className="flex flex-wrap items-center justify-between gap-2 rounded-lg border bg-primary/5 px-3 py-2.5">
                  <p className="text-sm">
                    <span className="text-xl font-semibold tabular-nums text-primary">{batch.transferable}</span>{" "}
                    <span className="text-muted-foreground">{t("vouchers.batches.detail.sellableSub")}</span>
                  </p>
                  <p className="text-sm font-medium tabular-nums">
                    {formatCurrency(batch.transferableValue, currency, lang)}
                  </p>
                </div>
                {batch.expiring7d > 0 && (
                  <p className="flex items-center gap-1.5 rounded-lg bg-amber-500/10 px-3 py-2 text-xs text-amber-600 dark:text-amber-400">
                    <TriangleAlert className="size-3.5 shrink-0" aria-hidden />
                    {tf("vouchers.batches.transferExpiringWarn", { n: batch.expiring7d })}
                  </p>
                )}
                {/* Possession live du stock vendable (lot immuable) */}
                <div className="space-y-2">
                  <p className="text-xs text-muted-foreground">
                    {t("vouchers.batches.detail.holdings")}
                  </p>
                  {holdings.length === 0 ? (
                    <p className="text-sm text-muted-foreground">
                      {t("vouchers.batches.detail.holdingsNone")}
                    </p>
                  ) : (
                    <ul className="space-y-2">
                      {holdings.map((h) => (
                        <li key={h.resellerId || "direct"} className="space-y-1">
                          <div className="flex items-baseline justify-between gap-2 text-sm">
                            <span className="truncate font-medium">
                              {h.resellerId === ""
                                ? t("vouchers.batches.holdingsDirect")
                                : h.name || h.resellerId}
                            </span>
                            <span className="shrink-0 tabular-nums text-muted-foreground">
                              {h.count} · {formatCurrency(h.value, currency, lang)}
                            </span>
                          </div>
                          <div className="h-1.5 w-full overflow-hidden rounded-full bg-muted">
                            <div
                              className="h-full rounded-full bg-primary/70"
                              style={{ width: `${holdingsTotal > 0 ? (h.count / holdingsTotal) * 100 : 0}%` }}
                            />
                          </div>
                        </li>
                      ))}
                    </ul>
                  )}
                </div>
              </section>

              <Separator />

              {/* Caractéristiques — générées une fois pour toutes (lot immuable) */}
              <section aria-label={t("vouchers.batches.detail.info")} className="space-y-2.5">
                <h3 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                  {t("vouchers.batches.detail.info")}
                </h3>
                <dl className="space-y-2">
                  <InfoRow
                    label={t("vouchers.batches.detail.provenance")}
                    value={
                      batch.channel === "reseller"
                        ? `${t("common.reseller")} · ${batch.resellerName || "—"}`
                        : t("common.direct")
                    }
                  />
                  <InfoRow label={t("vouchers.batches.site")} value={batch.routerName} />
                  <InfoRow
                    label={t("vouchers.batches.detail.quota")}
                    value={
                      batch.dataQuotaMb > 0
                        ? formatBytes(batch.dataQuotaMb * 1048576, lang)
                        : t("vouchers.quotaUnlimitedShort")
                    }
                  />
                  <InfoRow label={t("vouchers.batches.detail.timeLimit")} value={formatTimeLimit(batch.timeLimitMin)} />
                  <InfoRow
                    label={t("vouchers.batches.detail.unitPrice")}
                    value={formatCurrency(batch.unitPrice, currency, lang)}
                  />
                  <InfoRow
                    label={t("vouchers.batches.detail.totalCost")}
                    value={formatCurrency(batch.totalCost, currency, lang)}
                  />
                </dl>
              </section>
            </div>

            {/* Actions — les mêmes que la ligne, avec les accents bien placés */}
            <SheetFooter className="border-t">
              <div className="flex w-full flex-wrap items-center gap-2">
                <Button
                  className="h-10 flex-1"
                  disabled={batch.transferable === 0}
                  onClick={() => onTransfer(batch)}
                >
                  <ArrowLeftRight className="size-4" />
                  {t("vouchers.batches.transfer")}
                </Button>
                <Button variant="outline" className="h-10 flex-1" onClick={() => onPrintMulti(batch)}>
                  <Printer className="size-4" />
                  {t("vouchers.batches.detail.printMulti")}
                </Button>
                <Button variant="outline" size="icon" className="size-10" onClick={() => onPrintSimple(batch)}>
                  <Ticket className="size-4" />
                  <span className="sr-only">{t("vouchers.batches.printSimple")}</span>
                </Button>
                <Button variant="outline" size="icon" className="size-10" onClick={() => onViewVouchers(batch)}>
                  <Eye className="size-4" />
                  <span className="sr-only">{t("vouchers.batches.view")}</span>
                </Button>
                <Button
                  variant="ghost"
                  size="icon"
                  className="size-10 text-destructive hover:bg-destructive/10 hover:text-destructive"
                  onClick={() => onDelete(batch)}
                >
                  <Trash2 className="size-4" />
                  <span className="sr-only">{t("common.deleteBatch")}</span>
                </Button>
              </div>
            </SheetFooter>
          </>
        )}
      </SheetContent>
    </Sheet>
  );
}
