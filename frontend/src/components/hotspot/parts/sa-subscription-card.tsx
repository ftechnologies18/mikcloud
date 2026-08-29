"use client";

// Carte Abonnement SaaS — deux formules FCFA agressives pour un marché
// concurrentiel :
//   • Essentiel — 1 250 F/mois/routeur (sans engagement, facturé sur les
//     routeurs enregistrés : le gérant paie au fil de sa croissance) ;
//   • Illimité — 12 000 F/an, routeurs illimités (prix fondateur : 1 000 F/mois
//     équivalent, 2 mois offerts vs Essentiel, −92 % à 10 routeurs).
// Le paiement passe par le lien marchand Wave CI composé par montant.
// Bilingue FR/EN (F11) : les libellés du catalogue côté serveur (nom, tagline,
// badge, période) sont traduits par id de formule, avec repli sur la valeur
// serveur pour toute formule inconnue.

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, Crown, Infinity as InfinityIcon, Router as RouterIcon, Sparkles } from "lucide-react";
import { toast } from "sonner";

import { api } from "@/lib/hotspot/api";
import type { SaasPlan, SubscribeResponse, SubscriptionView } from "@/lib/hotspot/types";
import { formatCurrency, formatDate } from "@/lib/hotspot/format";
import { useI18n } from "@/lib/hotspot/i18n";
import { SETTINGS_QUERY_KEY } from "@/components/hotspot/parts/sd-currency";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
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
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";

export const SUBSCRIPTION_QUERY_KEY = ["/api/subscription"] as const;

export function useSubscription() {
  return useQuery({
    queryKey: SUBSCRIPTION_QUERY_KEY,
    queryFn: () => api<SubscriptionView>("/api/subscription"),
    staleTime: 30_000,
  });
}

// Comparatif annuel : formule mensuelle (1 250 F × routeurs × 12) vs Illimité.
const COMPARISON_ROUTERS = [1, 2, 5, 10];

// Caractéristiques des formules — clés i18n (traduites FR/EN).
const ESSENTIEL_FEATURE_KEYS = ["sub.feat.ess1", "sub.feat.ess2", "sub.feat.ess3", "sub.feat.ess4"] as const;
const ILLIMITE_FEATURE_KEYS = ["sub.feat.ill1", "sub.feat.ill2", "sub.feat.ill3", "sub.feat.ill4"] as const;

// Montant d'une période pour une formule (miroir de planAmount côté Go).
function planAmount(plan: SaasPlan, routerCount: number): number {
  return plan.perRouter ? plan.priceFcfa * Math.max(routerCount, 1) : plan.priceFcfa;
}

export function SubscriptionCard() {
  const { t, tf, lang } = useI18n();
  const { data: view, isLoading, isError, error, refetch } = useSubscription();
  const queryClient = useQueryClient();
  const [confirmPlan, setConfirmPlan] = useState<SaasPlan | null>(null);

  // Libellés du catalogue : traduits par id de formule, repli valeur serveur.
  const nameOf = (plan?: SaasPlan): string => (plan ? t(`sub.plan.${plan.id}.name`, plan.name) : "");
  const periodOf = (plan: SaasPlan): string => t(`sub.period.${plan.period}`, plan.period);

  const subscribeMutation = useMutation({
    mutationFn: (planId: string) =>
      api<SubscribeResponse>("/api/subscription", { method: "POST", body: { planId } }),
    onSuccess: (res) => {
      toast.success(
        tf("sub.toast.activated", {
          amount: formatCurrency(res.amountFcfa, "FCFA", lang),
          period: res.periodLabel === "1 an" ? t("sub.periodLabel.an") : t("sub.periodLabel.mois"),
          n: res.routerCount,
        }),
        { description: res.waveLink ? t("sub.toast.waveOpened") : undefined },
      );
      queryClient.invalidateQueries({ queryKey: SUBSCRIPTION_QUERY_KEY });
      queryClient.invalidateQueries({ queryKey: SETTINGS_QUERY_KEY });
      setConfirmPlan(null);
      if (res.waveLink) window.open(res.waveLink, "_blank", "noopener,noreferrer");
    },
    onError: (err: Error) => toast.error(err.message),
  });

  if (isError) {
    // État d'erreur explicite (API injoignable / backend pas encore à jour) —
    // jamais de squelette infini silencieux.
    return (
      <Card className="gap-4 py-4 sm:py-6 lg:col-span-2">
        <CardHeader className="px-4 sm:px-6">
          <CardTitle className="text-base">{t("sub.title")}</CardTitle>
          <CardDescription role="alert">
            {tf("sub.errorDesc", { msg: error?.message ?? t("sub.errorNetwork") })}
          </CardDescription>
        </CardHeader>
        <CardContent className="px-4 sm:px-6">
          <Button variant="outline" size="sm" onClick={() => refetch()}>
            {t("common.retry")}
          </Button>
        </CardContent>
      </Card>
    );
  }

  if (isLoading || !view) {
    return (
      <Card className="gap-4 py-4 sm:py-6 lg:col-span-2">
        <CardHeader className="px-4 sm:px-6">
          <Skeleton className="h-5 w-40" />
          <Skeleton className="h-4 w-64" />
        </CardHeader>
        <CardContent className="grid gap-4 px-4 sm:grid-cols-2 sm:px-6">
          <Skeleton className="h-56 rounded-xl" />
          <Skeleton className="h-56 rounded-xl" />
        </CardContent>
      </Card>
    );
  }

  const status = view.status;
  const currentPlan = view.plans.find((p) => p.id === view.subscription.planId);
  const statusBadge =
    status === "active" ? (
      <Badge className="gap-1 border-emerald-500/30 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400" variant="outline">
        <Check className="size-3" />
        {t("sub.status.active")}
      </Badge>
    ) : status === "expired" ? (
      <Badge className="gap-1 border-destructive/30 bg-destructive/10 text-destructive" variant="outline">
        {t("sub.status.expired")}
      </Badge>
    ) : (
      <Badge className="gap-1 border-primary/25 bg-primary/15 text-primary" variant="outline">
        <Sparkles className="size-3" />
        {t("sub.status.beta")}
      </Badge>
    );

  return (
    <Card className="gap-4 py-4 sm:py-6 lg:col-span-2">
      <CardHeader className="px-4 sm:px-6">
        <CardTitle className="flex items-center justify-between gap-2 text-base">
          {t("sub.title")}
          {statusBadge}
        </CardTitle>
        <CardDescription>
          {status === "active" && currentPlan ? (
            <>
              {t("sub.planWord")} <strong>{nameOf(currentPlan)}</strong> —{" "}
              {formatCurrency(view.currentAmountFcfa, "FCFA", lang)} / {periodOf(currentPlan)} ·{" "}
              {currentPlan.unlimited ? t("sub.unlimitedRouters") : tf("sub.routersRegistered", { n: view.routerCount })}
              {view.subscription.periodEnd ? (
                <> · {tf("sub.renewalOn", { date: formatDate(view.subscription.periodEnd, lang) })}</>
              ) : null}
            </>
          ) : status === "expired" ? (
            <>{t("sub.descExpired")}</>
          ) : (
            <>{t("sub.descBeta")}</>
          )}
        </CardDescription>
      </CardHeader>

      <CardContent className="space-y-4 px-4 sm:px-6">
        <div className="grid gap-4 sm:grid-cols-2">
          {/* Formule Essentiel */}
          <PlanCard
            plan={view.plans.find((p) => p.id === "essentiel")}
            routerCount={view.routerCount}
            isCurrent={view.subscription.planId === "essentiel" && status === "active"}
            featureKeys={ESSENTIEL_FEATURE_KEYS}
            featured={false}
            onSelect={setConfirmPlan}
          />
          {/* Formule Illimité */}
          <PlanCard
            plan={view.plans.find((p) => p.id === "illimite")}
            routerCount={view.routerCount}
            isCurrent={view.subscription.planId === "illimite" && status === "active"}
            featureKeys={ILLIMITE_FEATURE_KEYS}
            featured
            onSelect={setConfirmPlan}
          />
        </div>

        {/* Comparatif — l'argument qui fait basculer vers l'annuel */}
        <div className="overflow-hidden rounded-lg border">
          <table className="w-full text-sm">
            <caption className="sr-only">{t("sub.cmp.caption")}</caption>
            <thead>
              <tr className="border-b bg-muted/50 text-left text-xs uppercase tracking-wide text-muted-foreground">
                <th scope="col" className="px-3 py-2 font-medium">{t("sub.cmp.routers")}</th>
                <th scope="col" className="px-3 py-2 font-medium">{t("sub.cmp.essYear")}</th>
                <th scope="col" className="px-3 py-2 font-medium">{t("sub.cmp.illYear")}</th>
                <th scope="col" className="px-3 py-2 font-medium">{t("sub.cmp.save")}</th>
              </tr>
            </thead>
            <tbody>
              {COMPARISON_ROUTERS.map((routers) => {
                const monthlyAnnual = 1250 * routers * 12;
                const save = Math.round((1 - 12000 / monthlyAnnual) * 100);
                return (
                  <tr key={routers} className="border-b last:border-b-0">
                    <td className="px-3 py-2 font-medium">{routers}</td>
                    <td className="px-3 py-2 text-muted-foreground">{formatCurrency(monthlyAnnual, "FCFA", lang)}</td>
                    <td className="px-3 py-2">{formatCurrency(12000, "FCFA", lang)}</td>
                    <td className="px-3 py-2 font-semibold text-emerald-600 dark:text-emerald-400">−{save} %</td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
        <p className="text-xs text-muted-foreground">
          {t("sub.footnoteCalc")}
          {!view.waveConfigured ? ` ${t("sub.footnoteWave")}` : ""}
        </p>
      </CardContent>

      {/* Confirmation avant souscription — le montant est engagé immédiatement */}
      <AlertDialog open={confirmPlan !== null} onOpenChange={(open) => !open && setConfirmPlan(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {confirmPlan?.id === view.subscription.planId && status === "active"
                ? t("sub.dialog.renewTitle")
                : t("sub.dialog.subscribeTitle")}{" "}
              {nameOf(confirmPlan ?? undefined)} ?
            </AlertDialogTitle>
            <AlertDialogDescription>
              {t("sub.dialog.amount")}{" "}
              <strong>{confirmPlan ? formatCurrency(planAmount(confirmPlan, view.routerCount), "FCFA", lang) : "—"}</strong> /{" "}
              {confirmPlan ? periodOf(confirmPlan) : "—"}
              {confirmPlan?.perRouter
                ? ` ${tf("sub.dialog.scopeRouters", { n: view.routerCount })}`
                : confirmPlan
                  ? ` ${t("sub.dialog.scopeUnlimited")}`
                  : ""}
              . {t("sub.dialog.note")}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
            <AlertDialogAction
              onClick={(event) => {
                event.preventDefault();
                if (confirmPlan) subscribeMutation.mutate(confirmPlan.id);
              }}
            >
              {subscribeMutation.isPending ? t("sub.dialog.pending") : t("sub.dialog.confirm")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </Card>
  );
}

// PlanCard — une formule du catalogue, avec état courant et CTA.
function PlanCard({
  plan,
  routerCount,
  isCurrent,
  featureKeys,
  featured,
  onSelect,
}: {
  plan?: SaasPlan;
  routerCount: number;
  isCurrent: boolean;
  featureKeys: readonly string[];
  featured: boolean;
  onSelect: (plan: SaasPlan) => void;
}) {
  const { t, tf, lang } = useI18n();
  if (!plan) return null;
  const amount = planAmount(plan, routerCount);
  const name = t(`sub.plan.${plan.id}.name`, plan.name);
  const tagline = t(`sub.plan.${plan.id}.tagline`, plan.tagline);
  const badge = plan.badge ? t(`sub.plan.${plan.id}.badge`, plan.badge) : "";
  const period = t(`sub.period.${plan.period}`, plan.period);
  return (
    <div
      className={`relative flex flex-col rounded-xl border p-4 ${
        featured ? "border-primary/40 bg-primary/5 shadow-sm" : "bg-card"
      }`}
    >
      {badge ? (
        <Badge
          className={`absolute -top-2.5 left-4 gap-1 ${
            featured ? "bg-primary text-primary-foreground" : "border-muted-foreground/25 bg-muted text-foreground"
          }`}
          variant={featured ? "default" : "outline"}
        >
          {featured ? <Sparkles className="size-3" /> : null}
          {badge}
        </Badge>
      ) : null}

      <p className="flex items-center gap-2 font-semibold">
        {featured ? <InfinityIcon className="size-4 text-primary" /> : <RouterIcon className="size-4 text-primary" />}
        {name}
      </p>
      <p className="mt-2 text-2xl font-bold tracking-tight">
        {formatCurrency(plan.priceFcfa, "FCFA", lang)}
        <span className="ml-1 text-sm font-normal text-muted-foreground">
          / {period}
          {plan.perRouter ? t("sub.card.perRouterSuffix") : ""}
        </span>
      </p>
      <p className="mt-0.5 text-xs text-muted-foreground">{tagline}</p>

      {plan.perRouter ? (
        <p className="mt-2 rounded-md bg-muted/60 px-2.5 py-1.5 text-xs">
          {tf("sub.card.fleet", {
            amount: formatCurrency(amount, "FCFA", lang),
            period,
            n: routerCount,
          })}
        </p>
      ) : (
        <p className="mt-2 rounded-md bg-muted/60 px-2.5 py-1.5 text-xs">{t("sub.card.flat")}</p>
      )}

      <ul className="mt-3 flex-1 space-y-2">
        {featureKeys.map((key) => (
          <li key={key} className="flex items-start gap-2 text-xs">
            <Check className="mt-0.5 size-3.5 shrink-0 text-primary" />
            {t(key)}
          </li>
        ))}
      </ul>

      <Button
        className="mt-4 w-full"
        size="sm"
        variant={featured ? "default" : "outline"}
        onClick={() => onSelect(plan)}
      >
        {featured ? <Crown className="size-4" /> : null}
        {isCurrent ? t("sub.card.renew") : t("sub.card.subscribe")}
      </Button>
    </div>
  );
}
