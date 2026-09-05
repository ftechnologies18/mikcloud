"use client";

// Fiche 360° du lot (refonte v2 onglet Lots) — drawer latéral alimenté par la
// ligne cliquée (aucun appel réseau : les stats live, holdings, vélocité et
// cycle de vie sont déjà dans BatchWithStats). Vocabulaire métier unifié :
// stock vendable / éculés (vendus + consommés = CA) / expirés / purgés —
// et intelligence métier : valeur & marge, écoulement (sorties 7 j, rythme,
// projection, dormance). Actions inchangées (impression multi-formats,
// transfert N°18, vouchers, suppression).

import {
  ArrowLeftRight,
  Eye,
  Hourglass,
  Printer,
  Ticket,
  Trash2,
  TriangleAlert,
} from "lucide-react";

import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
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
import { formatBytes, formatCurrency, formatDate, formatDateTime } from "@/lib/hotspot/format";
import type { BatchWithStats } from "@/lib/hotspot/types";
import { cn } from "@/lib/utils";

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
function InfoRow({ label, value, accent }: { label: string; value: string; accent?: boolean }) {
  return (
    <div className="flex items-baseline justify-between gap-3">
      <dt className="shrink-0 text-xs text-muted-foreground">{label}</dt>
      <dd className={cn("truncate text-right text-sm font-medium", accent && "font-semibold text-primary")}>
        {value}
      </dd>
    </div>
  );
}

/** Tuile de la composition du lot (Stock vendable / Éculés / Expirés / Purgés). */
function LifeTile({ label, value, accent }: { label: string; value: number; accent?: boolean }) {
  return (
    <div className="rounded-lg bg-muted/50 px-2.5 py-2">
      <p className="truncate text-[11px] text-muted-foreground">{label}</p>
      <p className={cn("text-sm font-semibold tabular-nums", accent && "text-primary")}>{value}</p>
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
  const money = (amount: number) => formatCurrency(amount, currency, lang);

  // Parité Mikhmon : limit-uptime résolu à la génération (minutes).
  function formatTimeLimit(min: number): string {
    if (!min || min <= 0) return t("vouchers.batches.timeLimitNone");
    if (min % 1440 === 0) return tf("vouchers.batches.timeLimitDays", { n: min / 1440 });
    if (min % 60 === 0) return tf("vouchers.batches.timeLimitHours", { n: min / 60 });
    return tf("vouchers.batches.timeLimitMin", { n: min });
  }

  const holdings = batch?.holdings ?? [];
  const holdingsTotal = holdings.reduce((acc, h) => acc + h.count, 0);

  // Écoulement — rythme 7 j à 1 décimale (séparateur localisé) + projection.
  const sold7d = batch?.sold7d ?? 0;
  const rate = sold7d > 0 ? (sold7d / 7).toFixed(1).replace(".", lang === "fr" ? "," : ".") : null;
  const projection =
    batch && batch.transferable > 0 && sold7d > 0
      ? Math.ceil((batch.transferable * 7) / sold7d)
      : null;
  const dormant = batch != null && batch.transferable > 0 && batch.dormantDays >= 7;

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
              {/* Cycle de vie — héro-barre (chiffres dans les segments) + 4 tuiles métier */}
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
                <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
                  <LifeTile label={t("vouchers.batches.detail.sellable")} value={batch.transferable} accent />
                  {/* Éculés = vendus + désactivés FUSIONNÉS (du CA, jamais une perte) */}
                  <LifeTile label={t("vouchers.batches.detail.consumed")} value={batch.used + batch.disabled} />
                  <LifeTile label={t("vouchers.batches.detail.expired")} value={batch.expired} />
                  <LifeTile
                    label={t("vouchers.batches.detail.purged")}
                    value={Math.max(0, batch.count - batch.active - batch.used - batch.expired - batch.disabled)}
                  />
                </div>
                <p className="text-xs text-muted-foreground">{t("vouchers.batches.detail.sellableSub")}</p>
              </section>

              <Separator />

              {/* Valeur & marge — ce que le stock vivant rapportera à l'écoulement */}
              <section aria-label={t("vouchers.batches.detail.valueMargin")} className="space-y-2.5">
                <h3 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                  {t("vouchers.batches.detail.valueMargin")}
                </h3>
                <dl className="space-y-2">
                  <InfoRow label={t("vouchers.batches.detail.valueFace")} value={money(batch.stockFace)} />
                  <InfoRow label={t("vouchers.batches.detail.valueGros")} value={money(batch.transferableValue)} />
                  {batch.marginPending > 0 && (
                    <InfoRow
                      label={t("vouchers.batches.detail.marginPending")}
                      value={money(batch.marginPending)}
                      accent
                    />
                  )}
                </dl>
              </section>

              <Separator />

              {/* Possession live du stock vendable (lot immuable) */}
              <section aria-label={t("vouchers.batches.detail.holdings")} className="space-y-2">
                <h3 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                  {t("vouchers.batches.detail.holdings")}
                </h3>
                {holdings.length === 0 ? (
                  <p className="text-sm text-muted-foreground">{t("vouchers.batches.detail.holdingsNone")}</p>
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
                {batch.expiring7d > 0 && (
                  <p className="flex items-center gap-1.5 rounded-lg bg-amber-500/10 px-3 py-2 text-xs text-amber-600 dark:text-amber-400">
                    <TriangleAlert className="size-3.5 shrink-0" aria-hidden />
                    {tf("vouchers.batches.transferExpiringWarn", { n: batch.expiring7d })}
                  </p>
                )}
              </section>

              <Separator />

              {/* Écoulement — vélocité, projection et dormance du stock vivant */}
              <section aria-label={t("vouchers.batches.detail.ecoulement")} className="space-y-2.5">
                <h3 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                  {t("vouchers.batches.detail.ecoulement")}
                </h3>
                <dl className="space-y-2 rounded-lg border bg-muted/30 px-3 py-2.5">
                  <InfoRow label={t("vouchers.batches.detail.egress7d")} value={String(sold7d)} />
                  {rate && (
                    <InfoRow
                      label={t("vouchers.batches.detail.egressRate")}
                      value={tf("vouchers.batches.velocity.rate", { rate })}
                    />
                  )}
                  {batch.lastEgressAt && (
                    <InfoRow
                      label={t("vouchers.batches.detail.lastEgress")}
                      value={formatDate(batch.lastEgressAt, lang)}
                    />
                  )}
                </dl>
                {projection !== null && (
                  <p className="text-xs text-muted-foreground">
                    {tf("vouchers.batches.velocity.deplete", { d: projection })}
                  </p>
                )}
                {dormant && (
                  <Badge
                    variant="outline"
                    className="border-amber-500/40 text-amber-600 dark:text-amber-400"
                  >
                    <Hourglass className="size-3" aria-hidden />
                    {tf("vouchers.batches.detail.dormancy", { d: batch.dormantDays })}
                  </Badge>
                )}
              </section>

              <Separator />

              {/* Caractéristiques — générées une fois pour toutes (lot immuable) */}
              <section aria-label={t("vouchers.batches.detail.info")} className="space-y-2.5">
                <h3 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                  {t("vouchers.batches.detail.info")}
                </h3>
                <dl className="space-y-2">
                  <InfoRow
                    label={t("vouchers.batches.detail.generatedVia")}
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
                    label={t("vouchers.batches.detail.unitWholesale")}
                    value={formatCurrency(batch.unitPrice, currency, lang)}
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
