"use client";

// Confirmations destructives/irréversibles de la vue Vouchers — extraits du
// shell : reprise gérant (N°23 W6), suppression d'un voucher, suppression
// d'un lot entier. Présentation pure : l'état (cible en cours) et les
// mutations vivent dans le shell (views/vouchers-view.tsx).

import { Loader2 } from "lucide-react";

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
import { useI18n } from "@/lib/hotspot/i18n";
import type { BatchWithStats, HotspotUser } from "@/lib/hotspot/types";
import { shortBatch } from "./shared";

interface RepriseConfirmDialogProps {
  voucher: HotspotUser | null;
  isPending: boolean;
  onOpenChange: (open: boolean) => void;
  onConfirm: (voucher: HotspotUser) => void;
}

/** N°23 (W6) — confirmation reprise gérant (retour au stock direct). */
export function RepriseConfirmDialog({ voucher, isPending, onOpenChange, onConfirm }: RepriseConfirmDialogProps) {
  const { t, tf } = useI18n();
  return (
    <AlertDialog open={voucher !== null} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t("vouchers.reprise.title")}</AlertDialogTitle>
          <AlertDialogDescription>
            {voucher && tf("vouchers.reprise.desc", { name: voucher.resellerName || t("common.reseller") })}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={isPending}>{t("common.cancel")}</AlertDialogCancel>
          <AlertDialogAction
            disabled={isPending}
            onClick={(event) => {
              event.preventDefault();
              if (voucher) onConfirm(voucher);
            }}
          >
            {isPending && <Loader2 className="size-4 animate-spin" />}
            {t("vouchers.reprise.action")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

interface DeleteVoucherConfirmDialogProps {
  voucher: HotspotUser | null;
  isPending: boolean;
  onOpenChange: (open: boolean) => void;
  onConfirm: (voucher: HotspotUser) => void;
}

/** Confirmation suppression voucher. */
export function DeleteVoucherConfirmDialog({
  voucher,
  isPending,
  onOpenChange,
  onConfirm,
}: DeleteVoucherConfirmDialogProps) {
  const { t, tf } = useI18n();
  return (
    <AlertDialog open={voucher !== null} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{tf("vouchers.deleteTitle", { code: voucher?.username ?? "" })}</AlertDialogTitle>
          <AlertDialogDescription>{t("vouchers.deleteDesc")}</AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={isPending}>{t("common.cancel")}</AlertDialogCancel>
          <AlertDialogAction
            className="bg-destructive text-white hover:bg-destructive/90"
            disabled={isPending}
            onClick={(event) => {
              event.preventDefault();
              if (voucher) onConfirm(voucher);
            }}
          >
            {isPending && <Loader2 className="size-4 animate-spin" />}
            {t("common.delete")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

interface DeleteBatchConfirmDialogProps {
  batch: BatchWithStats | null;
  isPending: boolean;
  onOpenChange: (open: boolean) => void;
  onConfirm: (batch: BatchWithStats) => void;
}

/** Confirmation suppression d'un lot entier. */
export function DeleteBatchConfirmDialog({
  batch,
  isPending,
  onOpenChange,
  onConfirm,
}: DeleteBatchConfirmDialogProps) {
  const { t, tf } = useI18n();
  return (
    <AlertDialog open={batch !== null} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>
            {tf("vouchers.batches.deleteTitle", {
              batch: batch ? shortBatch(batch.id) : "",
            })}
          </AlertDialogTitle>
          <AlertDialogDescription>
            {batch &&
              tf("vouchers.batches.deleteDesc", {
                n: batch.count,
                profile: batch.profileName,
                router: batch.routerName,
              })}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={isPending}>{t("common.cancel")}</AlertDialogCancel>
          <AlertDialogAction
            className="bg-destructive text-white hover:bg-destructive/90"
            disabled={isPending}
            onClick={(event) => {
              event.preventDefault();
              if (batch) onConfirm(batch);
            }}
          >
            {isPending && <Loader2 className="size-4 animate-spin" />}
            {t("common.deleteBatch")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
