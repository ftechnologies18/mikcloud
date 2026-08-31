"use client";

// Console plateforme — FILE DES DEMANDES DE RENOUVELLEMENT (facturation).
// Chaque demande d'abonnement d'un client (verrou du cycle de facturation)
// arrive ici avec sa référence de paiement : en attente d'abord (file de
// travail, les plus anciennes en tête), historique résolu ensuite.
// Actions : « Encaisser & activer » (applique la période exactement comme la
// fiche client, marquage payé optionnel) et « Rejeter ». Le webhook Wave
// (secret partagé) peut traiter une demande automatiquement — la ligne porte
// alors « via Wave ».

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Ban,
  CheckCircle2,
  Loader2,
  ReceiptText,
  XCircle,
} from "lucide-react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { Textarea } from "@/components/ui/textarea";
import { EmptyState } from "@/components/hotspot/empty-state";
import { PageHeader } from "@/components/hotspot/page-header";
import { fetchBillingRequests, resolveBillingRequest } from "@/lib/hotspot/api";
import { useI18n } from "@/lib/hotspot/i18n";
import { formatCurrency, timeAgo } from "@/lib/hotspot/format";
import type { BillingRequestRow } from "@/lib/hotspot/types";

export default function BillingRequestsView() {
  const { t, tf, lang } = useI18n();
  const queryClient = useQueryClient();
  const [activateTarget, setActivateTarget] = useState<BillingRequestRow | null>(null);
  const [rejectTarget, setRejectTarget] = useState<BillingRequestRow | null>(null);
  const [markPaid, setMarkPaid] = useState(true);
  const [note, setNote] = useState("");

  const { data, isLoading, isError } = useQuery({
    queryKey: ["/api/admin/billing-requests"],
    queryFn: fetchBillingRequests,
    refetchInterval: 30_000,
  });

  const { pending, resolved } = useMemo(() => {
    const rows = data?.requests ?? [];
    return {
      pending: rows.filter((r) => r.status === "pending"),
      resolved: rows.filter((r) => r.status !== "pending"),
    };
  }, [data]);

  function closeDialogs() {
    setActivateTarget(null);
    setRejectTarget(null);
    setMarkPaid(true);
    setNote("");
  }

  const activate = useMutation({
    mutationFn: (row: BillingRequestRow) =>
      resolveBillingRequest(row.id, { action: "activate", markPaid, note: note.trim() || undefined }),
    onSuccess: (_res, row) => {
      toast.success(tf("billingRequests.activatedToast", { ref: row.ref }));
      void queryClient.invalidateQueries();
      closeDialogs();
    },
    onError: (err: Error) => toast.error(t("billingRequests.error"), { description: err.message }),
  });

  const reject = useMutation({
    mutationFn: (row: BillingRequestRow) =>
      resolveBillingRequest(row.id, { action: "cancel", note: note.trim() || undefined }),
    onSuccess: (_res, row) => {
      toast.success(tf("billingRequests.rejectedToast", { ref: row.ref }));
      void queryClient.invalidateQueries();
      closeDialogs();
    },
    onError: (err: Error) => toast.error(t("billingRequests.error"), { description: err.message }),
  });

  const busy = activate.isPending || reject.isPending;

  return (
    <div className="space-y-4 sm:space-y-6">
      <PageHeader
        title={t("billingRequests.title")}
        description={t("billingRequests.description")}
      />

      {/* File — demandes en attente */}
      <Card className="gap-0 py-0">
        <CardHeader className="border-b py-3.5">
          <CardTitle className="flex items-center gap-2 text-base">
            <ReceiptText className="size-4 text-primary" />
            {tf("billingRequests.pendingTitle", { n: pending.length })}
            {pending.length > 0 && (
              <span className="live-dot size-2 rounded-full bg-primary" aria-hidden />
            )}
          </CardTitle>
        </CardHeader>
        <CardContent className="p-0">
          {isLoading ? (
            <div className="space-y-3 p-4 sm:p-6">
              {Array.from({ length: 3 }).map((_, i) => (
                <Skeleton key={i} className="h-16 rounded-lg" />
              ))}
            </div>
          ) : isError ? (
            <div className="p-6">
              <EmptyState icon={ReceiptText} title={t("billingRequests.loadError")} />
            </div>
          ) : pending.length === 0 ? (
            <div className="p-6">
              <EmptyState
                icon={CheckCircle2}
                title={t("billingRequests.empty")}
                description={t("billingRequests.emptyDesc")}
              />
            </div>
          ) : (
            <ul className="divide-y" role="list" aria-label={t("billingRequests.title")}>
              {pending.map((row) => (
                <li
                  key={row.id}
                  className="flex flex-col gap-3 px-4 py-3.5 transition-colors hover:bg-accent/40 sm:flex-row sm:items-center sm:gap-4 sm:px-6"
                >
                  <div className="flex min-w-0 flex-1 items-start gap-3">
                    <span className="mt-0.5 flex size-9 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
                      <ReceiptText className="size-4.5" />
                    </span>
                    <div className="min-w-0">
                      <p className="truncate text-sm font-semibold">
                        {row.accountName || row.accountId}
                        {row.accountStatus === "disabled" && (
                          <Badge variant="outline" className="ml-2 px-1.5 py-0 text-[10px]">
                            {t("billingRequests.accountDisabled")}
                          </Badge>
                        )}
                      </p>
                      <p className="mt-0.5 flex flex-wrap items-center gap-x-2 text-xs text-muted-foreground">
                        <span className="font-mono font-medium text-foreground/80">{row.ref}</span>
                        <span>· {timeAgo(row.createdAt, lang)}</span>
                        <span>
                          ·{" "}
                          {tf("billingRequests.routers", {
                            n: row.routerCount,
                          })}
                        </span>
                      </p>
                    </div>
                  </div>
                  <div className="flex items-center gap-2 pl-12 sm:pl-0">
                    <Badge variant="secondary" className="shrink-0">
                      {row.planName}
                    </Badge>
                    <span className="text-sm font-semibold tabular-nums">
                      {formatCurrency(row.amountFcfa, undefined, lang)}
                      <span className="ml-1 text-xs font-normal text-muted-foreground">
                        / {row.periodLabel}
                      </span>
                    </span>
                  </div>
                  <div className="flex shrink-0 gap-2 sm:pl-2">
                    <Button
                      size="sm"
                      className="min-h-9 flex-1 sm:flex-none"
                      onClick={() => {
                        setMarkPaid(true);
                        setNote("");
                        setActivateTarget(row);
                      }}
                    >
                      <CheckCircle2 className="size-4" />
                      {t("billingRequests.activate")}
                    </Button>
                    <Button
                      size="sm"
                      variant="outline"
                      className="min-h-9 flex-1 text-destructive hover:bg-destructive/10 hover:text-destructive sm:flex-none"
                      onClick={() => {
                        setNote("");
                        setRejectTarget(row);
                      }}
                    >
                      <Ban className="size-4" />
                      {t("billingRequests.reject")}
                    </Button>
                  </div>
                </li>
              ))}
            </ul>
          )}
        </CardContent>
      </Card>

      {/* Historique — demandes traitées */}
      <Card className="gap-0 py-0">
        <CardHeader className="border-b py-3.5">
          <CardTitle className="text-base">{t("billingRequests.historyTitle")}</CardTitle>
        </CardHeader>
        <CardContent className="p-0">
          {resolved.length === 0 ? (
            <div className="p-6">
              <EmptyState icon={ReceiptText} title={t("billingRequests.historyEmpty")} />
            </div>
          ) : (
            <ul className="max-h-96 divide-y overflow-y-auto" role="list">
              {resolved.map((row) => (
                <li
                  key={row.id}
                  className="flex flex-col gap-2 px-4 py-3 sm:flex-row sm:items-center sm:gap-4 sm:px-6"
                >
                  <div className="flex min-w-0 flex-1 items-center gap-3">
                    {row.status === "done" ? (
                      <CheckCircle2 className="size-4.5 shrink-0 text-emerald-600" aria-hidden />
                    ) : (
                      <XCircle className="size-4.5 shrink-0 text-muted-foreground" aria-hidden />
                    )}
                    <div className="min-w-0">
                      <p className="truncate text-sm">
                        {row.accountName || row.accountId}
                        <span className="text-muted-foreground"> · {row.planName}</span>
                      </p>
                      <p className="mt-0.5 flex flex-wrap items-center gap-x-2 text-xs text-muted-foreground">
                        <span className="font-mono">{row.ref}</span>
                        <span>· {timeAgo(row.resolvedAt || row.createdAt, lang)}</span>
                        {row.resolvedBy && (
                          <span>· {tf("billingRequests.by", { who: row.resolvedBy })}</span>
                        )}
                        {row.status === "done" && row.paidVia && (
                          <span>· {t(`billingRequests.via.${row.paidVia}`, row.paidVia)}</span>
                        )}
                      </p>
                    </div>
                  </div>
                  <div className="flex items-center gap-2 pl-7.5 sm:pl-0">
                    <span className="text-sm tabular-nums text-muted-foreground">
                      {formatCurrency(row.amountFcfa, undefined, lang)}
                    </span>
                    <Badge
                      variant={row.status === "done" ? "default" : "outline"}
                      className={
                        row.status === "done"
                          ? "bg-emerald-600 text-white hover:bg-emerald-600"
                          : "text-muted-foreground"
                      }
                    >
                      {t(`billingRequests.status.${row.status}`)}
                    </Badge>
                  </div>
                </li>
              ))}
            </ul>
          )}
        </CardContent>
      </Card>

      {/* Confirmation « Encaisser & activer » */}
      <Dialog open={activateTarget !== null} onOpenChange={(open) => !open && closeDialogs()}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>
              {tf("billingRequests.activateTitle", { ref: activateTarget?.ref ?? "" })}
            </DialogTitle>
            <DialogDescription>
              {activateTarget
                ? tf("billingRequests.activateDesc", {
                    plan: activateTarget.planName,
                    amount: formatCurrency(activateTarget.amountFcfa, undefined, lang),
                    period: activateTarget.periodLabel,
                    routers: activateTarget.routerCount,
                    name: activateTarget.accountName || activateTarget.accountId,
                  })
                : ""}
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4 py-1">
            <div className="flex items-start gap-2.5">
              <Checkbox
                id="billing-mark-paid"
                checked={markPaid}
                onCheckedChange={(v) => setMarkPaid(v === true)}
              />
              <div className="grid gap-0.5">
                <Label htmlFor="billing-mark-paid" className="text-sm leading-snug">
                  {t("billingRequests.markPaid")}
                </Label>
                <p className="text-xs text-muted-foreground">{t("billingRequests.markPaidHint")}</p>
              </div>
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="billing-note">{t("billingRequests.noteLabel")}</Label>
              <Textarea
                id="billing-note"
                rows={2}
                value={note}
                onChange={(e) => setNote(e.target.value)}
                placeholder={t("billingRequests.notePlaceholder")}
              />
            </div>
          </div>
          <DialogFooter className="gap-2 sm:gap-0">
            <Button variant="outline" onClick={closeDialogs} disabled={busy}>
              {t("common.cancel")}
            </Button>
            <Button
              onClick={() => activateTarget && activate.mutate(activateTarget)}
              disabled={busy}
            >
              {busy ? <Loader2 className="size-4 animate-spin" /> : <CheckCircle2 className="size-4" />}
              {t("billingRequests.confirmActivate")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Confirmation « Rejeter » */}
      <Dialog open={rejectTarget !== null} onOpenChange={(open) => !open && closeDialogs()}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>
              {tf("billingRequests.rejectTitle", { ref: rejectTarget?.ref ?? "" })}
            </DialogTitle>
            <DialogDescription>{t("billingRequests.rejectDesc")}</DialogDescription>
          </DialogHeader>
          <div className="grid gap-1.5 py-1">
            <Label htmlFor="billing-note-reject">{t("billingRequests.noteLabel")}</Label>
            <Textarea
              id="billing-note-reject"
              rows={2}
              value={note}
              onChange={(e) => setNote(e.target.value)}
              placeholder={t("billingRequests.notePlaceholder")}
            />
          </div>
          <DialogFooter className="gap-2 sm:gap-0">
            <Button variant="outline" onClick={closeDialogs} disabled={busy}>
              {t("common.cancel")}
            </Button>
            <Button
              variant="destructive"
              onClick={() => rejectTarget && reject.mutate(rejectTarget)}
              disabled={busy}
            >
              {busy ? <Loader2 className="size-4 animate-spin" /> : <Ban className="size-4" />}
              {t("billingRequests.confirmReject")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
