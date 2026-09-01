"use client";

// N°18 — Dialog « Transférer le stock » : redistribution a posteriori du stock
// vendable d'un lot déjà généré, vers un revendeur (débit du portefeuille) ou
// vers le stock direct (retour de stock, recrédite le revendeur). Le lot reste
// immuable : seuls partent les tickets actifs jamais remis (anti-fraude), les
// plus récents d'abord. Garde-fou optionnel : exclure les tickets expirant
// sous 7 jours. À la clôture : impression A4 + QR des tickets transférés.
//
// La possession live (holdings) vient du backend : le stock qu'une
// destination détient déjà n'est pas re-transférable — le sélecteur de
// quantité plafonne donc à `transferable − déjà chez elle`.

import { useMemo, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { ArrowLeftRight, Loader2, Printer } from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { api } from "@/lib/hotspot/api";
import { formatCurrency } from "@/lib/hotspot/format";
import { useI18n } from "@/lib/hotspot/i18n";
import type { BatchWithStats, HotspotUser, Reseller } from "@/lib/hotspot/types";
import { cn } from "@/lib/utils";

const DIRECT = "direct";

function shortBatch(batchId: string): string {
  return batchId.split("-").pop() || batchId;
}

interface TransferResponse {
  transferred: number;
  debited: number;
  credited: number;
  creditAfter: number;
  refunds: { resellerId: string; resellerName: string; amount: number; creditAfter: number }[];
  vouchers: HotspotUser[];
}

interface VoucherTransferDialogProps {
  batch: BatchWithStats | null;
  resellers: Reseller[];
  currency: string;
  /** Fermeture du dialog (clic hors zone, Échap, succès ou annulation). */
  onOpenChange: (open: boolean) => void;
  /** Succès : le parent décide d'ouvrir (ou non) l'impression A4 des tickets transférés. */
  onTransferred: (batchId: string, vouchers: HotspotUser[], print: boolean) => void;
}

export function VoucherTransferDialog({
  batch,
  resellers,
  currency,
  onOpenChange,
  onTransferred,
}: VoucherTransferDialogProps) {
  const { t, tf, lang } = useI18n();
  const queryClient = useQueryClient();
  const open = batch !== null;

  const activeResellers = useMemo(
    () => resellers.filter((r) => r.status === "active"),
    [resellers],
  );

  // Le parent remonte le composant par `key={batch?.id}` à chaque lot : ces
  // initialisateurs paresseux rejouent à chaque ouverture (jamais en cours de
  // saisie). Destination par défaut intelligente : lot généré pour un
  // revendeur → retour de stock proposé ; lot direct → premier revendeur actif.
  const [target, setTarget] = useState<string>(
    () => (batch?.channel === "reseller" ? DIRECT : activeResellers[0]?.id) ?? DIRECT,
  );
  const [quantity, setQuantity] = useState("");
  const [excludeExpiring, setExcludeExpiring] = useState(false);
  const [print, setPrint] = useState(true);

  // Ce que la destination détient déjà n'est pas re-transférable.
  const heldByTarget = useMemo(() => {
    if (!batch?.holdings) return 0;
    const key = target === DIRECT ? "" : target;
    return batch.holdings.find((h) => h.resellerId === key)?.count ?? 0;
  }, [batch, target]);
  const maxForTarget = Math.max(0, (batch?.transferable ?? 0) - heldByTarget);

  const isRecall = target === DIRECT;
  const targetReseller = activeResellers.find((r) => r.id === target) ?? null;

  // Quantité : vide = tout le stock transférable pour cette destination.
  const parsedQty = quantity.trim() === "" ? maxForTarget : Math.floor(Number(quantity));
  const qty = Math.min(Number.isFinite(parsedQty) ? Math.max(0, parsedQty) : 0, maxForTarget);

  // Aperçu financier — approximation d'affichage (prix unitaire du lot) ; le
  // débit/recrédit EXACT, par voucher, est renvoyé par l'API (toast de succès).
  const previewAmount = qty * (batch?.unitPrice ?? 0);
  const insufficient = !isRecall && targetReseller !== null && previewAmount > targetReseller.credit;
  const canSubmit = maxForTarget > 0 && qty > 0 && !insufficient;

  // Sources d'un retour de stock (revendeurs détenant du stock vendable du lot).
  const refundSources = (batch?.holdings ?? []).filter((h) => h.resellerId !== "");
  const refundSourcesLabel =
    refundSources.map((h) => h.name || h.resellerId).join(", ") ||
    t("vouchers.batches.transferTargetDirect");

  const mutation = useMutation({
    mutationFn: (batchId: string) =>
      api<TransferResponse>(`/api/vouchers/batch/${batchId}/transfer`, {
        method: "POST",
        body: {
          resellerId: target,
          quantity: quantity.trim() === "" ? 0 : qty,
          excludeExpiringDays: excludeExpiring ? 7 : 0,
        },
      }),
    onSuccess: (res) => {
      if (isRecall) {
        const refund = res.refunds[0];
        toast.success(
          tf("vouchers.batches.transferSuccessRecall", {
            n: res.transferred,
            amount: formatCurrency(res.credited, currency, lang),
            name: refund?.resellerName ?? refundSourcesLabel,
          }),
        );
      } else {
        toast.success(
          tf("vouchers.batches.transferSuccess", {
            n: res.transferred,
            name: targetReseller?.name ?? "",
            amount: formatCurrency(res.debited, currency, lang),
            after: formatCurrency(res.creditAfter, currency, lang),
          }),
        );
      }
      void queryClient.invalidateQueries({ queryKey: ["/api/vouchers"] });
      void queryClient.invalidateQueries({ queryKey: ["/api/dashboard"] });
      void queryClient.invalidateQueries({ queryKey: ["/api/resellers"] });
      void queryClient.invalidateQueries({ queryKey: ["/api/transactions"] });
      if (batch) onTransferred(batch.id, res.vouchers, print);
      onOpenChange(false);
    },
    onError: (error: Error) => toast.error(error.message),
  });

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <ArrowLeftRight className="size-4 text-primary" aria-hidden />
            {batch ? tf("vouchers.batches.transferTitle", { batch: shortBatch(batch.id) }) : ""}
          </DialogTitle>
          <DialogDescription>{t("vouchers.batches.transferDesc")}</DialogDescription>
        </DialogHeader>

        {batch && (
          <div className="space-y-4">
            {/* Destination : stock direct (retour) ou revendeur (distribution) */}
            <div className="space-y-1.5">
              <Label htmlFor="transfer-target">{t("vouchers.batches.transferTarget")}</Label>
              <Select value={target} onValueChange={setTarget}>
                <SelectTrigger id="transfer-target" className="h-10 w-full">
                  <SelectValue aria-label={t("vouchers.batches.transferTarget")} />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={DIRECT}>
                    <span className="font-medium">{t("vouchers.batches.transferTargetDirect")}</span>
                  </SelectItem>
                  {activeResellers.map((r) => (
                    <SelectItem key={r.id} value={r.id}>
                      <span className="font-medium">{r.name}</span>
                      <span className="text-muted-foreground">
                        {" "}
                        ·{" "}
                        {tf("vouchers.batches.transferResellerMeta", {
                          credit: formatCurrency(r.credit, currency, lang),
                          stock: r.stockCount ?? 0,
                        })}
                      </span>
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            {/* Disponibilité + garde-fou expiration */}
            <div className="rounded-lg border bg-muted/40 p-3">
              <p className={cn("text-sm font-medium", maxForTarget > 0 ? "text-foreground" : "text-muted-foreground")}>
                {tf("vouchers.batches.transferAvailable", { n: maxForTarget })}
              </p>
              {batch.expiring7d > 0 && (
                <p className="mt-1 text-xs text-amber-600 dark:text-amber-500">
                  {tf("vouchers.batches.transferExpiringWarn", { n: batch.expiring7d })}
                </p>
              )}
            </div>

            {/* Quantité (vide = tout) */}
            <div className="space-y-1.5">
              <Label htmlFor="transfer-qty">{t("vouchers.batches.transferQuantity")}</Label>
              <div className="flex gap-2">
                <Input
                  id="transfer-qty"
                  inputMode="numeric"
                  min={1}
                  max={maxForTarget}
                  value={quantity}
                  placeholder={String(maxForTarget)}
                  disabled={maxForTarget === 0}
                  onChange={(event) => setQuantity(event.target.value.replace(/[^\d]/g, ""))}
                  className="h-10"
                />
                <Button
                  type="button"
                  variant="outline"
                  className="h-10 shrink-0"
                  disabled={maxForTarget === 0}
                  onClick={() => setQuantity(String(maxForTarget))}
                >
                  {t("vouchers.batches.transferAll")}
                </Button>
              </div>
            </div>

            {/* Garde-fou : exclure le stock mort imminent */}
            {batch.expiring7d > 0 && (
              <div className="flex min-h-11 items-center justify-between gap-3 rounded-lg border p-3">
                <Label htmlFor="transfer-expiring" className="text-sm font-normal">
                  {t("vouchers.batches.transferExpiring")}
                </Label>
                <Switch id="transfer-expiring" checked={excludeExpiring} onCheckedChange={setExcludeExpiring} />
              </div>
            )}

            {/* Impression A4 + QR des tickets transférés */}
            <div className="flex min-h-11 items-center justify-between gap-3 rounded-lg border p-3">
              <Label htmlFor="transfer-print" className="flex items-center gap-2 text-sm font-normal">
                <Printer className="size-4 text-muted-foreground" aria-hidden />
                {t("vouchers.batches.transferPrint")}
              </Label>
              <Switch id="transfer-print" checked={print} onCheckedChange={setPrint} />
            </div>

            {/* Aperçu financier (le débit exact est confirmé par l'API) */}
            {qty > 0 && (
              <p className={cn("text-sm", insufficient ? "text-destructive" : "text-muted-foreground")}>
                {isRecall
                  ? tf("vouchers.batches.transferPreviewRefund", {
                      name: refundSourcesLabel,
                      amount: formatCurrency(previewAmount, currency, lang),
                    })
                  : tf("vouchers.batches.transferPreviewDebit", {
                      amount: formatCurrency(previewAmount, currency, lang),
                      after: formatCurrency((targetReseller?.credit ?? 0) - previewAmount, currency, lang),
                    })}
              </p>
            )}
            {insufficient && (
              <p className="text-sm text-destructive">{t("vouchers.batches.transferPreviewInsufficient")}</p>
            )}
          </div>
        )}

        <DialogFooter className="gap-2">
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={mutation.isPending}>
            {t("common.cancel")}
          </Button>
          <Button
            onClick={() => batch && mutation.mutate(batch.id)}
            disabled={!canSubmit || mutation.isPending}
          >
            {mutation.isPending ? (
              <>
                <Loader2 className="size-4 animate-spin" aria-hidden />
                {t("vouchers.batches.transferSubmitting")}
              </>
            ) : (
              tf(isRecall ? "vouchers.batches.transferSubmitRecall" : "vouchers.batches.transferSubmit", { n: qty })
            )}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
