"use client";

// Dialogs du Mode Vente — présentation pure extraite de sell-shell :
// rapport de fin de journée (P3-d : ventilation par canal, dépôt-vente,
// export comptable, partage), confirmation de vente (UX R2), reçu
// « Vente confirmée » (anti-fuite : seule porte de sortie du code) et
// confirmation de sortie de stock (N°20/N°21 : retour gérant ou transfert
// entre revendeurs). L'état et les mutations vivent dans sell-shell.tsx.

import {
  ArrowLeftRight,
  BadgeCheck,
  CloudUpload,
  Download,
  FileBarChart,
  Loader2,
  Share2,
  Undo2,
} from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { isSamePasswordMode } from "@/components/hotspot/parts/template-render";
import { useI18n } from "@/lib/hotspot/i18n";
import { formatCurrency } from "@/lib/hotspot/format";
import type { SellDayReport } from "@/lib/hotspot/types";
import { VIA_KEYS, VIA_ORDER, viaIcon, type SellPeer, type SellVoucher } from "./helpers";

// ---------------------------------------------------------------------------
// Rapport de fin de journée
// ---------------------------------------------------------------------------

interface DayReportDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  report: SellDayReport | undefined;
  reportLoading: boolean;
  currency: string;
  exporting: boolean;
  onExport: () => void;
  onShare: () => void;
}

export function DayReportDialog({
  open,
  onOpenChange,
  report,
  reportLoading,
  currency,
  exporting,
  onExport,
  onShare,
}: DayReportDialogProps) {
  const { t, tf, lang } = useI18n();
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <FileBarChart className="size-4 text-primary" aria-hidden />
            {t("sell.dayReport")}
          </DialogTitle>
          <DialogDescription>
            {report
              ? new Date(`${report.date}T12:00:00Z`).toLocaleDateString(lang === "en" ? "en-GB" : "fr-FR", {
                  weekday: "long",
                  day: "2-digit",
                  month: "long",
                  year: "numeric",
                })
              : t("sell.dayReportDesc")}
          </DialogDescription>
        </DialogHeader>

        {reportLoading || !report ? (
          <div className="space-y-2">
            <div className="grid grid-cols-3 gap-2">
              {Array.from({ length: 3 }).map((_, i) => (
                <Skeleton key={i} className="h-16 rounded-lg" />
              ))}
            </div>
            <Skeleton className="h-40 rounded-lg" />
          </div>
        ) : (
          <>
            <div className="grid grid-cols-3 gap-2">
              <div className="rounded-lg border bg-muted/30 p-3 text-center">
                <p className="text-lg font-bold tabular-nums">{report.soldCount}</p>
                <p className="text-[11px] text-muted-foreground">{t("sell.dayReportSold")}</p>
              </div>
              <div className="rounded-lg border bg-muted/30 p-3 text-center">
                <p className="text-lg font-bold text-primary tabular-nums">
                  {formatCurrency(report.revenue, currency, lang)}
                </p>
                <p className="text-[11px] text-muted-foreground">{t("sell.dayReportRevenue")}</p>
              </div>
              <div className="rounded-lg border bg-muted/30 p-3 text-center">
                <p className="text-lg font-bold tabular-nums">{report.stockCount}</p>
                <p className="text-[11px] text-muted-foreground">{t("sell.dayReportStock")}</p>
              </div>
            </div>

            {/* P3-d — ventilation par canal : le comptoir comprend d'où viennent
                les ventes (tactile / auto à la connexion / papier historique). */}
            {report.byVia && (report.byVia.sell_mode ?? 0) + (report.byVia.auto_connect ?? 0) + (report.byVia.sell_mode_paper ?? 0) > 0 && (
              <div className="flex flex-wrap items-center gap-1.5" role="list" aria-label={t("sell.dayReportByVia")}>
                {VIA_ORDER.map((via) => {
                  const n = report.byVia?.[via] ?? 0;
                  if (!n) return null;
                  const Icon = viaIcon(via);
                  return (
                    <Badge key={via} variant="secondary" className="gap-1 text-[11px]" role="listitem">
                      <Icon aria-hidden className="size-3" />
                      {t(VIA_KEYS[via])} · {n}
                    </Badge>
                  );
                })}
              </div>
            )}

            {report.paymentMode === "deposit" && (
              <div className="rounded-lg border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-sm">
                <p className="font-medium text-amber-700 dark:text-amber-300">
                  {tf("sell.dayReportToDepositText", { amount: formatCurrency(report.toDeposit ?? 0, currency, lang) })}
                </p>
                <p className="text-xs text-amber-600/80 dark:text-amber-400/80">
                  {tf("sell.dayReportDebtText", { amount: formatCurrency(report.debtTotal ?? 0, currency, lang) })}
                </p>
                {/* P3-d — versements déjà encaissés par le gérant aujourd'hui :
                    le reste à verser est donc toDeposit − settledToday. */}
                {(report.settledToday ?? 0) > 0 && (
                  <p className="text-xs font-medium text-amber-700 dark:text-amber-300">
                    {tf("sell.dayReportSettled", { amount: formatCurrency(report.settledToday ?? 0, currency, lang) })}
                  </p>
                )}
              </div>
            )}

            {/* P3-d — retours du jour avec flux cash (recrédit prépayé).
                En dépôt-vente un retour ne déplace pas d'argent : il n'est
                pas listé — le journal reste un vrai journal de caisse. */}
            {(report.returnedCredited ?? 0) > 0 && (
              <p className="text-xs text-muted-foreground">
                {tf("sell.dayReportReturned", {
                  count: report.returnedCount ?? 0,
                  amount: formatCurrency(report.returnedCredited ?? 0, currency, lang),
                })}
              </p>
            )}

            <div className="max-h-64 overflow-y-auto rounded-lg border" aria-label={t("sell.dayReportDetail")}>
              {report.sold.length === 0 ? (
                <p className="px-3 py-8 text-center text-sm text-muted-foreground">{t("sell.dayReportEmpty")}</p>
              ) : (
                report.sold.map((s) => {
                  const Icon = viaIcon(s.soldVia);
                  return (
                    <div
                      key={s.id}
                      className="flex items-center justify-between gap-3 border-b px-3 py-2 last:border-b-0"
                    >
                      <div className="min-w-0">
                        <p className="flex items-center gap-1.5 truncate font-mono text-sm font-semibold">
                          <span title={t(VIA_KEYS[s.soldVia ?? "sell_mode"])} className="flex shrink-0">
                            <Icon
                              aria-hidden
                              className="size-3 text-muted-foreground"
                            />
                          </span>
                          {s.code}
                        </p>
                        <p className="truncate text-xs text-muted-foreground">
                          {s.profileName} ·{" "}
                          {new Date(s.soldAt).toLocaleTimeString(lang === "en" ? "en-GB" : "fr-FR", {
                            hour: "2-digit",
                            minute: "2-digit",
                          })}
                        </p>
                      </div>
                      <p className="shrink-0 text-sm font-semibold text-primary tabular-nums">
                        {formatCurrency(s.price, currency, lang)}
                      </p>
                    </div>
                  );
                })
              )}
            </div>
          </>
        )}

        <DialogFooter>
          {/* P3-d — export comptable : journal de caisse Excel (CSV « ; », BOM). */}
          <Button
            variant="outline"
            onClick={onExport}
            disabled={reportLoading || !report || exporting}
          >
            {exporting ? <Loader2 className="size-4 animate-spin" /> : <Download className="size-4" />}
            {t("sell.dayReportExport")}
          </Button>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t("common.close")}
          </Button>
          <Button onClick={onShare} disabled={reportLoading || !report}>
            <Share2 className="size-4" />
            {t("sell.dayReportShare")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

// ---------------------------------------------------------------------------
// Confirmation de vente (UX R2)
// ---------------------------------------------------------------------------

interface SaleConfirmDialogProps {
  pendingSale: SellVoucher | null;
  currency: string;
  sellPending: boolean;
  onOpenChange: (open: boolean) => void;
  onConfirm: () => void;
  onCancel: () => void;
}

export function SaleConfirmDialog({
  pendingSale,
  currency,
  sellPending,
  onOpenChange,
  onConfirm,
  onCancel,
}: SaleConfirmDialogProps) {
  const { t, lang } = useI18n();
  return (
    <Dialog open={pendingSale !== null} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <BadgeCheck className="size-4 text-primary" aria-hidden />
            {t("sell.sellConfirmTitle")}
          </DialogTitle>
          <DialogDescription>{t("sell.sellConfirmDesc")}</DialogDescription>
        </DialogHeader>

        {pendingSale && (
          <>
            <div className="rounded-lg border bg-muted/30 p-3">
              <div className="flex items-center justify-between gap-3">
                <p className="font-semibold">{pendingSale.profileName}</p>
                <p className="shrink-0 font-bold text-primary tabular-nums">
                  {formatCurrency(pendingSale.sellingPrice || pendingSale.price, currency, lang)}
                </p>
              </div>
              {/* Anti-fuite : même le récapitulatif pré-confirmation reste
                  muet sur le code — il n'apparaîtra que dans le reçu. */}
              <p className="mt-1 font-mono text-sm tracking-widest text-muted-foreground" aria-label={t("sell.codeAfterConfirm")}>••••••</p>
            </div>

            <DialogFooter>
              <Button variant="outline" onClick={onCancel} disabled={sellPending}>
                {t("common.cancel")}
              </Button>
              <Button onClick={onConfirm} disabled={sellPending}>
                {sellPending ? <Loader2 className="size-4 animate-spin" /> : <BadgeCheck className="size-4" />}
                {t("sell.sellConfirmAction")}
              </Button>
            </DialogFooter>
          </>
        )}
      </DialogContent>
    </Dialog>
  );
}

// ---------------------------------------------------------------------------
// Reçu « Vente confirmée » (anti-fuite)
// ---------------------------------------------------------------------------

interface ReceiptDialogProps {
  receipt: { voucher: SellVoucher; offline: boolean } | null;
  currency: string;
  onOpenChange: (open: boolean) => void;
  onShare: (voucher: SellVoucher) => void;
}

export function ReceiptDialog({ receipt, currency, onOpenChange, onShare }: ReceiptDialogProps) {
  const { t, lang } = useI18n();
  return (
    <Dialog open={receipt !== null} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <BadgeCheck className="size-4 text-primary" aria-hidden />
            {t("sell.receiptTitle")}
          </DialogTitle>
          <DialogDescription>{t("sell.receiptDesc")}</DialogDescription>
        </DialogHeader>

        {receipt && (
          <>
            <div className="rounded-lg border bg-muted/30 p-3">
              <div className="flex items-center justify-between gap-3">
                <p className="font-semibold">{receipt.voucher.profileName}</p>
                <p className="shrink-0 font-bold text-primary tabular-nums">
                  {formatCurrency(receipt.voucher.sellingPrice || receipt.voucher.price, currency, lang)}
                </p>
              </div>
              <div className={`mt-3 grid gap-2 font-mono text-sm ${isSamePasswordMode(receipt.voucher) ? "grid-cols-1" : "grid-cols-2"}`}>
                <div>
                  <p className="text-[10px] tracking-wide text-muted-foreground uppercase">{t("sell.code")}</p>
                  <p className="mt-0.5 text-base font-semibold select-all">{receipt.voucher.username}</p>
                </div>
                {!isSamePasswordMode(receipt.voucher) && (
                  <div>
                    <p className="text-[10px] tracking-wide text-muted-foreground uppercase">{t("sell.password")}</p>
                    <p className="mt-0.5 text-base font-semibold select-all">{receipt.voucher.password}</p>
                  </div>
                )}
              </div>
            </div>

            {receipt.offline && (
              <p
                role="status"
                className="flex items-start gap-2 rounded-lg border border-amber-500/40 bg-amber-500/10 px-3 py-2 text-xs text-amber-700 dark:text-amber-300"
              >
                <CloudUpload aria-hidden className="mt-0.5 size-3.5 shrink-0" />
                {t("sell.receiptOffline")}
              </p>
            )}

            <DialogFooter>
              <Button variant="outline" onClick={() => onOpenChange(false)}>
                {t("common.close")}
              </Button>
              <Button onClick={() => onShare(receipt.voucher)}>
                <Share2 className="size-4" />
                {t("sell.share")}
              </Button>
            </DialogFooter>
          </>
        )}
      </DialogContent>
    </Dialog>
  );
}

// ---------------------------------------------------------------------------
// Sortie de stock : retour gérant OU transfert entre revendeurs (N°20/N°21)
// ---------------------------------------------------------------------------

interface OutboundConfirmDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  currency: string;
  dest: string;
  onDestChange: (dest: string) => void;
  destName: string;
  peers: SellPeer[] | undefined;
  selectedCount: number;
  selectedWholesale: number;
  isDeposit: boolean;
  returnPending: boolean;
  transferPending: boolean;
  /** Fermetures du shell : ids sélectionnés + destination capturés au parent. */
  onReturn: () => void;
  onTransfer: () => void;
}

export function OutboundConfirmDialog({
  open,
  onOpenChange,
  currency,
  dest,
  onDestChange,
  destName,
  peers,
  selectedCount,
  selectedWholesale,
  isDeposit,
  returnPending,
  transferPending,
  onReturn,
  onTransfer,
}: OutboundConfirmDialogProps) {
  const { t, tf, lang } = useI18n();
  const pending = returnPending || transferPending;
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            {dest === "manager" ? (
              <Undo2 className="size-4 text-primary" aria-hidden />
            ) : (
              <ArrowLeftRight className="size-4 text-primary" aria-hidden />
            )}
            {dest === "manager"
              ? t("sell.returnConfirmTitle")
              : tf("sell.transferConfirmTitle", { name: destName })}
          </DialogTitle>
          <DialogDescription>
            {dest === "manager" ? (
              isDeposit ? (
                tf("sell.returnConfirmDescDeposit", { count: selectedCount })
              ) : (
                tf("sell.returnConfirmDescPrepaid", {
                  count: selectedCount,
                  amount: formatCurrency(selectedWholesale, currency, lang),
                })
              )
            ) : isDeposit ? (
              tf("sell.transferConfirmDescDeposit", { count: selectedCount, name: destName })
            ) : (
              tf("sell.transferConfirmDescPrepaid", {
                count: selectedCount,
                name: destName,
                amount: formatCurrency(selectedWholesale, currency, lang),
              })
            )}
          </DialogDescription>
        </DialogHeader>
        {/* Destination : gérant (retour de stock) ou pair (transfert). */}
        <div className="space-y-1.5">
          <Label htmlFor="outbound-dest">{t("sell.outboundDest")}</Label>
          <Select value={dest} onValueChange={onDestChange}>
            <SelectTrigger id="outbound-dest" className="h-10 w-full">
              <SelectValue aria-label={t("sell.outboundDest")} />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="manager">{t("sell.outboundDestManager")}</SelectItem>
              {(peers ?? []).map((p) => (
                <SelectItem key={p.id} value={p.id}>
                  {tf("sell.outboundDestPeer", { name: p.name })}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          {dest !== "manager" && (
            <p className="text-xs text-muted-foreground">{t("sell.transferHint")}</p>
          )}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={pending}>
            {t("common.cancel")}
          </Button>
          <Button
            onClick={() => (dest === "manager" ? onReturn() : onTransfer())}
            disabled={pending || selectedCount === 0}
          >
            {pending ? (
              <Loader2 className="size-4 animate-spin" />
            ) : dest === "manager" ? (
              <Undo2 className="size-4" />
            ) : (
              <ArrowLeftRight className="size-4" />
            )}
            {dest === "manager" ? t("sell.returnAction") : t("sell.transferAction")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
