"use client";

// Vue ABONNEMENT (M) — navigation dédiée pour le client (pas un onglet des
// Paramètres) : le paiement étant manuel (Wave/mobile money), le renouvellement
// est une tâche ACTIVE que le client doit exécuter avant chaque échéance.
//   • Carte statut — formule, échéance, compte à rebours, CTA renouveler ;
//   • Formules — SubscriptionCard existante (Essentiel vs Illimité, paiement) ;
//   • Historique de facturation — factures payées (MC-2026-0001), chacune
//     consultable/téléchargeable (facture HTML print-friendly → PDF navigateur).

import { useQuery } from "@tanstack/react-query";
import { CreditCard, Download, FileText, ReceiptText } from "lucide-react";

import { fetchBillingHistory, invoiceURL } from "@/lib/hotspot/api";
import { useI18n } from "@/lib/hotspot/i18n";
import type { InvoiceRow, SubscriptionView } from "@/lib/hotspot/types";
import { formatCurrency, formatDate } from "@/lib/hotspot/format";
import { useSubscription } from "@/components/hotspot/parts/sa-subscription-card";
import { SubscriptionCard } from "@/components/hotspot/parts/sa-subscription-card";
import { EmptyState } from "@/components/hotspot/empty-state";
import { PageHeader } from "@/components/hotspot/page-header";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";

const DAY_MS = 86_400_000;

export default function SubscriptionView() {
  const { t, tf, lang } = useI18n();
  const { data: view, isLoading } = useSubscription();
  const { data: invoices, isLoading: invoicesLoading } = useQuery({
    queryKey: ["/api/billing/history"],
    queryFn: fetchBillingHistory,
  });

  if (isLoading) {
    return (
      <div className="space-y-4">
        <PageHeader title={t("sub.title")} description={t("subView.description")} />
        <Skeleton className="h-32 w-full rounded-xl" />
        <Skeleton className="h-96 w-full rounded-xl" />
        <Skeleton className="h-64 w-full rounded-xl" />
      </div>
    );
  }

  return (
    <div className="space-y-4 sm:space-y-6">
      <PageHeader title={t("sub.title")} description={t("subView.description")} />

      <StatusCard view={view} />
      <SubscriptionCard />
      <HistoryCard invoices={invoices ?? []} loading={invoicesLoading} t={t} lang={lang} />
    </div>
  );
}

/* ─── Carte statut : formule + échéance + compte à rebours ─── */
function StatusCard({ view }: { view?: SubscriptionView }) {
  const { t, tf, lang } = useI18n();
  if (!view) return null;

  const sub = view.subscription;
  const planName =
    view.plans.find((p) => p.id === sub.planId)?.name ?? (sub.planId || t("sub.status.beta"));

  // Compte à rebours : jours restants avant l'échéance.
  let daysLeft: number | null = null;
  if (sub.periodEnd) {
    const end = new Date(sub.periodEnd).getTime();
    daysLeft = Math.ceil((end - Date.now()) / DAY_MS);
  }

  // Couleur du statut : vert actif / ambre J-7 / rouge expiré-suspendu.
  const expired = view.status === "expired" || view.status === "suspended";
  const soon = daysLeft !== null && daysLeft >= 0 && daysLeft <= 7;
  const dotClass = expired
    ? "bg-destructive"
    : soon
      ? "bg-amber-500"
      : "bg-emerald-500";
  const statusLabel = expired
    ? t("subView.statusExpired")
    : view.status === "none"
      ? t("sub.status.beta")
      : t("sub.status.active");

  return (
    <Card
      className={`gap-4 py-4 sm:py-6 ${
        expired ? "border-destructive/40" : soon ? "border-amber-500/40" : "border-primary/30"
      }`}
    >
      <CardHeader className="px-4 sm:px-6">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <CardTitle className="flex items-center gap-2 text-base">
            <span className="flex size-8 items-center justify-center rounded-lg bg-primary/15 text-primary">
              <CreditCard className="size-4" />
            </span>
            {planName}
            <Badge
              variant="outline"
              className={`gap-1.5 px-2 py-0 text-[11px] font-semibold ${
                expired
                  ? "border-destructive/30 bg-destructive/10 text-destructive"
                  : soon
                    ? "border-amber-500/30 bg-amber-500/10 text-amber-600 dark:text-amber-400"
                    : "border-emerald-500/30 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400"
              }`}
            >
              <span className={`live-dot size-1.5 rounded-full ${dotClass}`} aria-hidden />
              {statusLabel}
            </Badge>
          </CardTitle>
          {sub.periodEnd && !expired && (
            <span className="text-sm text-muted-foreground">
              {tf("subView.expiresOn", {
                date: formatDate(sub.periodEnd, lang),
                days: daysLeft ?? 0,
              })}
            </span>
          )}
        </div>
      </CardHeader>
      {expired && (
        <CardContent className="px-4 sm:px-6 pt-0">
          <p className="text-sm text-destructive">{t("subView.expiredHint")}</p>
        </CardContent>
      )}
    </Card>
  );
}

/* ─── Historique de facturation ─── */
function HistoryCard({
  invoices,
  loading,
  t,
  lang,
}: {
  invoices: InvoiceRow[];
  loading: boolean;
  t: ReturnType<typeof useI18n>["t"];
  lang: "fr" | "en";
}) {
  return (
    <Card className="gap-4 py-4 sm:py-6">
      <CardHeader className="px-4 sm:px-6">
        <CardTitle className="flex items-center gap-2 text-base">
          <span className="flex size-8 items-center justify-center rounded-lg bg-primary/15 text-primary">
            <ReceiptText className="size-4" />
          </span>
          {t("subView.history")}
        </CardTitle>
        <CardDescription>{t("subView.historyDesc")}</CardDescription>
      </CardHeader>
      <CardContent className="px-4 sm:px-6">
        {loading ? (
          <div className="space-y-2">
            <Skeleton className="h-14 w-full" />
            <Skeleton className="h-14 w-full" />
          </div>
        ) : invoices.length === 0 ? (
          <EmptyState
            icon={FileText}
            title={t("subView.historyEmpty")}
            description={t("subView.historyEmptyDesc")}
          />
        ) : (
          <ul className="divide-y divide-border/60">
            {invoices.map((inv) => (
              <li key={inv.id} className="flex flex-wrap items-center gap-3 py-3 first:pt-0 last:pb-0">
                <div className="min-w-0 flex-1">
                  <p className="flex flex-wrap items-center gap-x-2 gap-y-0.5 text-sm font-medium">
                    <span className="font-mono text-[13px]">{inv.invoiceNo}</span>
                    <span className="text-muted-foreground">·</span>
                    <span>{inv.planName}</span>
                    <span className="text-muted-foreground">·</span>
                    <span className="font-semibold text-primary">
                      {formatCurrency(inv.amountFcfa, "FCFA", lang)}
                    </span>
                  </p>
                  <p className="mt-0.5 text-xs text-muted-foreground">
                    {formatDate(inv.issuedAt, lang)} · {inv.periodLabel} ·{" "}
                    {inv.paidVia === "wave" ? t("subView.paidWave") : t("subView.paidManual")}
                  </p>
                </div>
                <Button
                  variant="outline"
                  size="sm"
                  className="shrink-0"
                  onClick={() => window.open(invoiceURL(inv.id), "_blank", "noopener")}
                >
                  <Download className="size-4" />
                  {t("subView.invoice")}
                </Button>
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  );
}
