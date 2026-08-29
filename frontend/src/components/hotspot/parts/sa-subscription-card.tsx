"use client";

// Carte Abonnement SaaS — deux formules FCFA agressives pour un marché
// concurrentiel :
//   • Essentiel — 1 250 F/mois/routeur (sans engagement, facturé sur les
//     routeurs enregistrés : le gérant paie au fil de sa croissance) ;
//   • Illimité — 12 000 F/an, routeurs illimités (prix fondateur : 1 000 F/mois
//     équivalent, 2 mois offerts vs Essentiel, −92 % à 10 routeurs).
// Le paiement passe par le lien marchand Wave CI composé par montant.

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, Crown, Infinity as InfinityIcon, ExternalLink, Router as RouterIcon, Sparkles } from "lucide-react";
import { toast } from "sonner";

import { api } from "@/lib/hotspot/api";
import type { SaasPlan, SubscribeResponse, SubscriptionView } from "@/lib/hotspot/types";
import { formatCurrency, formatDate } from "@/lib/hotspot/format";
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

const ESSENTIEL_FEATURES = [
  "Toutes les fonctionnalités MikCloud",
  "Notifications Telegram / WhatsApp / Email",
  "Quotas data (Go) sur les vouchers",
  "Impression A4 + QR par lot, revendeurs",
];

const ILLIMITE_FEATURES = [
  "Tous vos routeurs, actuels et futurs",
  "Toutes les fonctionnalités MikCloud",
  "Revendeurs & comptabilité illimités",
  "Prix fondateur — bloqué à vie",
];

// Montant d'une période pour une formule (miroir de planAmount côté Go).
function planAmount(plan: SaasPlan, routerCount: number): number {
  return plan.perRouter ? plan.priceFcfa * Math.max(routerCount, 1) : plan.priceFcfa;
}

export function SubscriptionCard() {
  const { data: view, isLoading, isError, error, refetch } = useSubscription();
  const queryClient = useQueryClient();
  const [confirmPlan, setConfirmPlan] = useState<SaasPlan | null>(null);

  const subscribeMutation = useMutation({
    mutationFn: (planId: string) =>
      api<SubscribeResponse>("/api/subscription", { method: "POST", body: { planId } }),
    onSuccess: (res) => {
      toast.success(
        `Formule activée — ${formatCurrency(res.amountFcfa)} / ${res.periodLabel} · ${res.routerCount} routeur(s)`,
        { description: res.waveLink ? "Le lien de paiement Wave s'est ouvert dans un nouvel onglet." : undefined },
      );
      queryClient.invalidateQueries({ queryKey: SUBSCRIPTION_QUERY_KEY });
      queryClient.invalidateQueries({ queryKey: SETTINGS_QUERY_KEY });
      setConfirmPlan(null);
      if (res.waveLink) window.open(res.waveLink, "_blank", "noopener,noreferrer");
    },
    onError: (error: Error) => toast.error(error.message),
  });

  if (isError) {
    // État d'erreur explicite (API injoignable / backend pas encore à jour) —
    // jamais de squelette infini silencieux.
    return (
      <Card className="gap-4 py-4 sm:py-6 lg:col-span-2">
        <CardHeader className="px-4 sm:px-6">
          <CardTitle className="text-base">Abonnement</CardTitle>
          <CardDescription role="alert">
            Impossible de charger l&apos;abonnement ({error?.message ?? "erreur réseau"}). Le service est peut-être en
            cours de mise à jour — réessayez dans un instant.
          </CardDescription>
        </CardHeader>
        <CardContent className="px-4 sm:px-6">
          <Button variant="outline" size="sm" onClick={() => refetch()}>
            Réessayer
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
        Actif
      </Badge>
    ) : status === "expired" ? (
      <Badge className="gap-1 border-destructive/30 bg-destructive/10 text-destructive" variant="outline">
        Expiré
      </Badge>
    ) : (
      <Badge className="gap-1 border-primary/25 bg-primary/15 text-primary" variant="outline">
        <Sparkles className="size-3" />
        Bêta
      </Badge>
    );

  return (
    <Card className="gap-4 py-4 sm:py-6 lg:col-span-2">
      <CardHeader className="px-4 sm:px-6">
        <CardTitle className="flex items-center justify-between gap-2 text-base">
          Abonnement
          {statusBadge}
        </CardTitle>
        <CardDescription>
          {status === "active" && currentPlan ? (
            <>
              Formule <strong>{currentPlan.name}</strong> — {formatCurrency(view.currentAmountFcfa)} /{" "}
              {currentPlan.period} ·{" "}
              {currentPlan.unlimited ? "routeurs illimités" : `${view.routerCount} routeur(s) enregistré(s)`}
              {view.subscription.periodEnd ? <> · renouvellement le {formatDate(view.subscription.periodEnd)}</> : null}
            </>
          ) : status === "expired" ? (
            <>Votre période est échue — choisissez une formule pour réactiver votre espace.</>
          ) : (
            <>Période bêta — choisissez la formule qui accompagne votre croissance.</>
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
            features={ESSENTIEL_FEATURES}
            featured={false}
            onSelect={setConfirmPlan}
          />
          {/* Formule Illimité */}
          <PlanCard
            plan={view.plans.find((p) => p.id === "illimite")}
            routerCount={view.routerCount}
            isCurrent={view.subscription.planId === "illimite" && status === "active"}
            features={ILLIMITE_FEATURES}
            featured
            onSelect={setConfirmPlan}
          />
        </div>

        {/* Comparatif — l'argument qui fait basculer vers l'annuel */}
        <div className="overflow-hidden rounded-lg border">
          <table className="w-full text-sm">
            <caption className="sr-only">
              Comparaison du coût annuel entre la formule mensuelle par routeur et la formule annuelle illimitée
            </caption>
            <thead>
              <tr className="border-b bg-muted/50 text-left text-xs uppercase tracking-wide text-muted-foreground">
                <th scope="col" className="px-3 py-2 font-medium">Routeurs</th>
                <th scope="col" className="px-3 py-2 font-medium">Essentiel / an</th>
                <th scope="col" className="px-3 py-2 font-medium">Illimité / an</th>
                <th scope="col" className="px-3 py-2 font-medium">Vous économisez</th>
              </tr>
            </thead>
            <tbody>
              {COMPARISON_ROUTERS.map((routers) => {
                const monthlyAnnual = 1250 * routers * 12;
                const save = Math.round((1 - 12000 / monthlyAnnual) * 100);
                return (
                  <tr key={routers} className="border-b last:border-b-0">
                    <td className="px-3 py-2 font-medium">{routers}</td>
                    <td className="px-3 py-2 text-muted-foreground">{formatCurrency(monthlyAnnual)}</td>
                    <td className="px-3 py-2">{formatCurrency(12000)}</td>
                    <td className="px-3 py-2 font-semibold text-emerald-600 dark:text-emerald-400">−{save} %</td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
        <p className="text-xs text-muted-foreground">
          Essentiel = 1 250 F × routeurs enregistrés × 12 mois. Le renouvellement recalcule automatiquement le montant
          selon vos routeurs du moment.
          {!view.waveConfigured &&
            " Configurez votre lien marchand Wave (carte Organisation) pour régler en un clic."}
        </p>
      </CardContent>

      {/* Confirmation avant souscription — le montant est engagé immédiatement */}
      <AlertDialog open={confirmPlan !== null} onOpenChange={(open) => !open && setConfirmPlan(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {confirmPlan?.id === view.subscription.planId && status === "active"
                ? "Renouveler la formule"
                : "Souscrire à la formule"}{" "}
              {confirmPlan?.name} ?
            </AlertDialogTitle>
            <AlertDialogDescription>
              Montant : <strong>{confirmPlan ? formatCurrency(planAmount(confirmPlan, view.routerCount)) : "—"}</strong>{" "}
              / {confirmPlan?.period}
              {confirmPlan?.perRouter ? ` (${view.routerCount} routeur(s) enregistré(s))` : " — routeurs illimités"}. La
              période démarre immédiatement et le paiement se règle via votre lien marchand Wave.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Annuler</AlertDialogCancel>
            <AlertDialogAction
              onClick={(event) => {
                event.preventDefault();
                if (confirmPlan) subscribeMutation.mutate(confirmPlan.id);
              }}
            >
              {subscribeMutation.isPending ? "Activation…" : "Confirmer"}
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
  features,
  featured,
  onSelect,
}: {
  plan?: SaasPlan;
  routerCount: number;
  isCurrent: boolean;
  features: string[];
  featured: boolean;
  onSelect: (plan: SaasPlan) => void;
}) {
  if (!plan) return null;
  const amount = planAmount(plan, routerCount);
  return (
    <div
      className={`relative flex flex-col rounded-xl border p-4 ${
        featured ? "border-primary/40 bg-primary/5 shadow-sm" : "bg-card"
      }`}
    >
      {plan.badge ? (
        <Badge
          className={`absolute -top-2.5 left-4 gap-1 ${
            featured ? "bg-primary text-primary-foreground" : "border-muted-foreground/25 bg-muted text-foreground"
          }`}
          variant={featured ? "default" : "outline"}
        >
          {featured ? <Sparkles className="size-3" /> : null}
          {plan.badge}
        </Badge>
      ) : null}

      <p className="flex items-center gap-2 font-semibold">
        {featured ? <InfinityIcon className="size-4 text-primary" /> : <RouterIcon className="size-4 text-primary" />}
        {plan.name}
      </p>
      <p className="mt-2 text-2xl font-bold tracking-tight">
        {formatCurrency(plan.priceFcfa)}
        <span className="ml-1 text-sm font-normal text-muted-foreground">
          / {plan.period}
          {plan.perRouter ? " / routeur" : ""}
        </span>
      </p>
      <p className="mt-0.5 text-xs text-muted-foreground">{plan.tagline}</p>

      {plan.perRouter ? (
        <p className="mt-2 rounded-md bg-muted/60 px-2.5 py-1.5 text-xs">
          Votre parc actuel :{" "}
          <strong>
            {formatCurrency(amount)} / {plan.period}
          </strong>{" "}
          ({routerCount} routeur(s))
        </p>
      ) : (
        <p className="mt-2 rounded-md bg-muted/60 px-2.5 py-1.5 text-xs">
          Soit <strong>1 000 F / mois</strong> — quel que soit votre nombre de routeurs.
        </p>
      )}

      <ul className="mt-3 flex-1 space-y-2">
        {features.map((feature) => (
          <li key={feature} className="flex items-start gap-2 text-xs">
            <Check className="mt-0.5 size-3.5 shrink-0 text-primary" />
            {feature}
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
        {isCurrent ? "Renouveler" : "Souscrire"}
      </Button>
    </div>
  );
}
