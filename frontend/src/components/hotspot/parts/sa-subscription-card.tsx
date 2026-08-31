"use client";

// Carte Abonnement SaaS — deux formules FCFA agressives pour un marché
// concurrentiel :
//   • Essentiel — 1 250 F/mois/routeur (sans engagement, facturé sur les
//     routeurs enregistrés : le gérant paie au fil de sa croissance) ;
//   • Illimité — 12 000 F/an, routeurs illimités (1 000 F/mois équivalent,
//     2 mois offerts vs Essentiel, −92 % à 10 routeurs).
// VERROU FACTURATION : la souscription n'active RIEN côté client — choisir
// une formule enregistre une demande, renvoie le montant et le lien de
// paiement Wave de la PLATEFORME (WAVE_PAY_LINK) ; l'activation de la
// période est effectuée par l'équipe MikCloud après encaissement.
// Bilingue FR/EN (F11) : les libellés du catalogue côté serveur (nom, tagline,
// badge, période) sont traduits par id de formule, avec repli sur la valeur
// serveur pour toute formule inconnue.

import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, CreditCard, Crown, Infinity as InfinityIcon, Router as RouterIcon, ShieldAlert, Sparkles } from "lucide-react";
import { toast } from "sonner";

import { api } from "@/lib/hotspot/api";
import type {
  PayInitiateResponse,
  SaasPlan,
  StripeCreateResponse,
  StripeSubView,
  SubscribeResponse,
  SubscriptionView,
} from "@/lib/hotspot/types";
import { formatCurrency, formatDate } from "@/lib/hotspot/format";
import { useI18n } from "@/lib/hotspot/i18n";
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

// Prélèvement automatique par carte (Stripe via GeniusPay) : interrogation
// rapprochée tant que la saisie de carte est attendue (retour de Stripe
// Checkout — l'activation est finalisée côté serveur à la resynchronisation).
export const STRIPE_QUERY_KEY = ["/api/subscription/stripe"] as const;

export function useStripeSub() {
  return useQuery({
    queryKey: STRIPE_QUERY_KEY,
    queryFn: () => api<StripeSubView>("/api/subscription/stripe"),
    staleTime: 15_000,
    refetchInterval: (q) =>
      (q.state.data as StripeSubView | undefined)?.status === "pending" ? 5_000 : false,
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
  const { data: stripe } = useStripeSub();
  const queryClient = useQueryClient();
  const [confirmPlan, setConfirmPlan] = useState<SaasPlan | null>(null);
  const [cancelStripeOpen, setCancelStripeOpen] = useState(false);

  // Retour de Stripe Checkout : la facture peut n'être réglée que quelques
  // secondes après la redirection — on déclenche la resynchronisation, le
  // polling du hook useStripeSub finalise l'affichage (statut pending).
  useEffect(() => {
    if (typeof window === "undefined") return;
    const sp = new URLSearchParams(window.location.search);
    if (sp.get("stripe") === "return") {
      toast.info(t("sub.stripe.returning", "Retour de Stripe — vérification du paiement en cours…"), { duration: 6000 });
      queryClient.invalidateQueries({ queryKey: STRIPE_QUERY_KEY });
    }
  }, []);

  // Libellés du catalogue : traduits par id de formule, repli valeur serveur.
  const nameOf = (plan?: SaasPlan): string => (plan ? t(`sub.plan.${plan.id}.name`, plan.name) : "");
  const periodOf = (plan: SaasPlan): string => t(`sub.period.${plan.period}`, plan.period);

  const subscribeMutation = useMutation({
    mutationFn: (planId: string) =>
      api<SubscribeResponse>("/api/subscription", { method: "POST", body: { planId } }),
    onSuccess: (res) => {
      toast.success(
        tf("sub.toast.requested", {
          amount: formatCurrency(res.amountFcfa, "FCFA", lang),
          period: res.periodLabel === "1 an" ? t("sub.periodLabel.an") : t("sub.periodLabel.mois"),
          n: res.routerCount,
        }),
        {
          description: t("sub.pay.initiating", "Ouverture du paiement Wave…"),
        },
      );
      queryClient.invalidateQueries({ queryKey: SUBSCRIPTION_QUERY_KEY });
      setConfirmPlan(null);
      void openWavePayment(res);
    },
    onError: (err: Error) => toast.error(err.message),
  });

  // openWavePayment — enchaîne le paiement en ligne : initiation GeniusPay
  // (mode Wave) puis redirection du client vers la page de paiement ; la
  // confirmation arrive par webhook et active l'abonnement. Repli : lien Wave
  // manuel de la plateforme, sinon la demande reste enregistrée pour règlement
  // auprès de MikCloud.
  async function openWavePayment(res: SubscribeResponse) {
    try {
      const pay = await api<PayInitiateResponse>("/api/subscription/pay", { method: "POST" });
      if (pay.paymentUrl) {
        toast.info(t("sub.pay.redirect", "Redirection vers Wave pour régler votre abonnement…"));
        window.location.href = pay.paymentUrl;
        return;
      }
    } catch {
      // Paiement en ligne indisponible (non configuré, téléphone manquant,
      // erreur GeniusPay) : la demande reste valable, repli sur lien manuel.
    }
    if (res.waveLink) {
      toast.info(t("sub.toast.waveOpened"), { duration: 8000 });
      window.open(res.waveLink, "_blank", "noopener,noreferrer");
    } else {
      toast.info(t("sub.toast.noWaveLink"), { duration: 8000 });
    }
  }

  // Abonnement RÉCURRENT par carte (Stripe via GeniusPay) : création puis
  // redirection vers la page de paiement sécurisée Stripe ; chaque facture
  // payée active/empile automatiquement la période (webhook signé).
  const stripeMutation = useMutation({
    mutationFn: (planId: string) =>
      api<StripeCreateResponse>("/api/subscription/stripe", { method: "POST", body: { planId } }),
    onSuccess: (res) => {
      setConfirmPlan(null);
      queryClient.invalidateQueries({ queryKey: STRIPE_QUERY_KEY });
      // Prélèvement automatique créé. Si GeniusPay renvoie une page de paiement
      // (Checkout Stripe) ou un lien pour la première période, on y redirige
      // immédiatement ; sinon la demande reste à régler (bouton Wave).
      const target = res.redirectUrl || res.paymentUrl;
      if (target) {
        toast.info(t("sub.stripe.redirect", "Redirection vers la page de paiement sécurisée (Stripe)…"));
        window.location.href = target;
        return;
      }
      toast.info(t("sub.stripe.autoOnly", "Prélèvement automatique activé — réglez la première période pour l'activer."), { duration: 8000 });
    },
    onError: (err: Error) => toast.error(err.message),
  });

  const cancelStripeMutation = useMutation({
    mutationFn: () => api<{ status: string }>("/api/subscription/stripe/cancel", { method: "POST", body: {} }),
    onSuccess: () => {
      toast.success(t("sub.stripe.cancelled", "Prélèvement automatique résilié — votre accès reste actif jusqu'à la fin de la période payée."));
      setCancelStripeOpen(false);
      queryClient.invalidateQueries({ queryKey: STRIPE_QUERY_KEY });
    },
    onError: (err: Error) => toast.error(err.message),
  });

  // Libellé d'état du prélèvement automatique (bandeau de la carte).
  function stripeStatusText(s: StripeSubView): string {
    const cycle = s.cycle === "yearly" ? t("sub.periodLabel.an") : t("sub.periodLabel.mois");
    switch (s.status) {
      case "active":
        return tf("sub.stripe.active", {
          amount: formatCurrency(s.amountFcfa ?? 0, "FCFA", lang),
          cycle,
          date: s.nextBilling ? formatDate(s.nextBilling, lang) : "—",
        });
      case "trialing":
        return t("sub.stripe.trialing");
      case "past_due":
        return t("sub.stripe.pastDue");
      case "paused":
        return t("sub.stripe.paused");
      default: // pending
        return t("sub.stripe.pending");
    }
  }

  const stripeRunning =
    !!stripe && !"none|cancelled|expired".split("|").includes(stripe.status);

  // Répercussion des frais de paiement (stratégie validée) : montants par
  // moyen pour la formule confirmée — source de vérité serveur (view.pricing),
  // repli = prix catalogue si le backend ne l'expose pas encore.
  const confirmedPricing = confirmPlan
    ? view?.pricing?.find((p) => p.planId === confirmPlan.id)
    : undefined;
  const confirmedBase =
    confirmedPricing?.baseFcfa ?? (confirmPlan ? planAmount(confirmPlan, view?.routerCount ?? 0) : 0);
  const confirmedWave = confirmedPricing?.waveFcfa ?? confirmedBase;
  const confirmedList = confirmedPricing?.listFcfa ?? confirmedBase;

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
    ) : status === "suspended" ? (
      <Badge className="gap-1 border-destructive/40 bg-destructive/15 text-destructive" variant="outline">
        <ShieldAlert className="size-3" />
        {t("sub.status.suspended")}
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
          ) : status === "suspended" ? (
            <>{t("sub.descSuspended")}</>
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

        {/* Prélèvement automatique par carte (Stripe via GeniusPay) */}
        {stripeRunning && stripe ? (
          <div
            className={`flex flex-col gap-2 rounded-lg border px-4 py-3 sm:flex-row sm:items-center sm:justify-between ${
              stripe.status === "past_due" ? "border-amber-500/40 bg-amber-500/10" : "border-primary/25 bg-primary/5"
            }`}
          >
            <div className="min-w-0">
              <p className="flex items-center gap-2 text-sm font-medium">
                <CreditCard className="size-4 shrink-0 text-primary" />
                {t("sub.stripe.title")}
              </p>
              <p className="mt-0.5 text-xs text-muted-foreground">{stripeStatusText(stripe)}</p>
            </div>
            <Button
              variant="outline"
              size="sm"
              className="shrink-0"
              onClick={() => setCancelStripeOpen(true)}
              disabled={cancelStripeMutation.isPending}
            >
              {t("sub.stripe.cancel")}
            </Button>
          </div>
        ) : null}

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
          {t("sub.footnoteCalc")} {t("sub.footnotePay")}
        </p>
      </CardContent>

      {/* Confirmation avant demande — rien n'est débité ici : demande + lien de paiement */}
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
              <strong>{confirmPlan ? formatCurrency(confirmedBase, "FCFA", lang) : "—"}</strong> /{" "}
              {confirmPlan ? periodOf(confirmPlan) : "—"}
              {confirmPlan?.perRouter
                ? ` ${tf("sub.dialog.scopeRouters", { n: view.routerCount })}`
                : confirmPlan
                  ? ` ${t("sub.dialog.scopeUnlimited")}`
                  : ""}
              . {t("sub.dialog.feesNote")}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
            {/* Wave (ponctuel, défaut) — montant avec remise mobile money −3 % */}
            <AlertDialogAction
              onClick={(event) => {
                event.preventDefault();
                if (confirmPlan) subscribeMutation.mutate(confirmPlan.id);
              }}
              disabled={subscribeMutation.isPending || stripeMutation.isPending}
            >
              {subscribeMutation.isPending ? (
                t("sub.dialog.pending")
              ) : (
                <>
                  {tf("sub.dialog.payWaveAmt", { amount: formatCurrency(confirmedWave, "FCFA", lang) })}
                  <span className="ml-1.5 rounded bg-white/25 px-1.5 py-0.5 text-[10px] font-bold text-white">
                    {t("sub.dialog.waveBadge")}
                  </span>
                </>
              )}
            </AlertDialogAction>
            {/* Carte bancaire — prélèvement automatique (Stripe via GeniusPay) — prix de liste */}
            <Button
              variant="outline"
              onClick={(event) => {
                event.preventDefault();
                if (confirmPlan) stripeMutation.mutate(confirmPlan.id);
              }}
              disabled={subscribeMutation.isPending || stripeMutation.isPending}
            >
              <CreditCard className="size-4" />
              {stripeMutation.isPending ? t("sub.dialog.pending") : tf("sub.dialog.payCardAmt", { amount: formatCurrency(confirmedList, "FCFA", lang) })}
            </Button>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* Résiliation du prélèvement automatique — fin de période, accès conservé */}
      <AlertDialog open={cancelStripeOpen} onOpenChange={setCancelStripeOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t("sub.stripe.cancelTitle")}</AlertDialogTitle>
            <AlertDialogDescription>{t("sub.stripe.cancelDesc")}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
            <AlertDialogAction
              onClick={(event) => {
                event.preventDefault();
                cancelStripeMutation.mutate();
              }}
              disabled={cancelStripeMutation.isPending}
            >
              {cancelStripeMutation.isPending ? t("sub.stripe.cancelling") : t("sub.stripe.cancelConfirm")}
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
