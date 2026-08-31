"use client";

import { useState } from "react";
import { motion } from "framer-motion";
import { Check, Lock, LogOut, ShieldAlert } from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Separator } from "@/components/ui/separator";
import { useSubscription } from "@/components/hotspot/parts/sa-subscription-card";
import { useI18n } from "@/lib/hotspot/i18n";
import { useHotspotStore } from "@/lib/hotspot/store";
import { api } from "@/lib/hotspot/api";
import { formatCurrency } from "@/lib/hotspot/format";

/* ─── PaywallOverlay ───
 * Mur total non refermable : s'affiche par-dessus toute la console quand
 * le compte est "suspended" (PeriodEnd + 30j dépassé). L'utilisateur peut
 * se déconnecter mais ne peut pas naviguer dans la console sans payer.
 * Réutilise POST /api/subscription — VERROU FACTURATION : l'endpoint
 * enregistre la DEMANDE et renvoie le lien de paiement Wave de la
 * plateforme ; l'activation du compte est effectuée par MikCloud après
 * encaissement (jamais côté client).
 */
export function PaywallOverlay() {
  const { t, lang } = useI18n();
  const logout = useHotspotStore((s) => s.logout);
  const { data: view, refetch } = useSubscription();
  const [paying, setPaying] = useState<string | null>(null);

  // L'overlay ne s'affiche que si suspended. Les autres statuts (active, expired,
  // none) laissent la console accessible (expired = lecture seule via guards backend).
  if (!view || view.status !== "suspended") return null;

  async function subscribe(planId: "essentiel" | "illimite") {
    setPaying(planId);
    try {
      const res = await api<{ waveLink?: string }>("/api/subscription", {
        method: "POST",
        body: { planId },
      });
      if (res.waveLink) {
        window.location.href = res.waveLink;
      } else {
        toast.success(
          t("paywall.requested", "Demande enregistrée — MikCloud activera votre accès dès réception du paiement."),
          { description: t("paywall.contact", "Une question ? Contactez l'équipe MikCloud.") },
        );
        await refetch();
      }
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("paywall.failed", "Échec de la souscription"));
    } finally {
      setPaying(null);
    }
  }

  return (
    <div className="fixed inset-0 z-[100] flex items-center justify-center overflow-y-auto bg-background/95 backdrop-blur-md p-4">
      {/* Décor halos cohérents avec la landing */}
      <div aria-hidden className="pointer-events-none absolute inset-0">
        <div className="absolute -top-32 left-1/4 size-96 rounded-full bg-primary/20 blur-[120px]" />
        <div className="absolute bottom-0 right-1/4 size-80 rounded-full bg-destructive/20 blur-[100px]" />
      </div>

      <motion.div
        initial={{ opacity: 0, y: 20, scale: 0.98 }}
        animate={{ opacity: 1, y: 0, scale: 1 }}
        transition={{ duration: 0.5, ease: "easeOut" }}
        className="relative z-10 w-full max-w-3xl"
      >
        <Card className="border-destructive/30 shadow-2xl">
          <CardHeader className="text-center pb-4">
            <div className="mx-auto mb-4 grid size-14 place-items-center rounded-2xl bg-destructive/10 text-destructive">
              <Lock className="size-7" aria-hidden />
            </div>
            <CardTitle className="text-2xl">
              {t("paywall.title", "Compte suspendu")}
            </CardTitle>
            <CardDescription className="text-base">
              {t(
                "paywall.subtitle",
                "Votre accès est suspendu. Choisissez une formule et réglez le paiement — MikCloud réactivera votre compte dès réception. Vos données sont conservées.",
              )}
            </CardDescription>
          </CardHeader>

          <CardContent className="space-y-6">
            <div className="rounded-lg border border-amber-500/40 bg-amber-500/10 px-4 py-3">
              <div className="flex items-start gap-3">
                <ShieldAlert className="mt-0.5 size-5 shrink-0 text-amber-600 dark:text-amber-400" />
                <div className="text-sm">
                  <p className="font-medium text-amber-700 dark:text-amber-300">
                    {t("paywall.warningTitle", "Accès suspendu")}
                  </p>
                  <p className="mt-1 text-amber-800/90 dark:text-amber-200/80">
                    {t(
                      "paywall.warningDesc",
                      "Vos routeurs continuent de fonctionner pour vos clients, mais vous ne pouvez plus accéder à la console. Vos données (routeurs, vouchers, sessions, rapports) sont conservées en sécurité.",
                    )}
                  </p>
                </div>
              </div>
            </div>

            <Separator />

            <div className="grid gap-4 sm:grid-cols-2">
              {view.plans.map((plan) => {
                const amount = plan.perRouter
                  ? plan.priceFcfa * Math.max(1, view.routerCount)
                  : plan.priceFcfa;
                return (
                  <Card
                    key={plan.id}
                    className={
                      plan.unlimited
                        ? "border-primary/50 ring-1 ring-primary/30"
                        : "border-border/60"
                    }
                  >
                    <CardHeader className="pb-3">
                      <CardTitle className="text-lg">{plan.name}</CardTitle>
                      <CardDescription>{plan.tagline}</CardDescription>
                      <div className="mt-2">
                        <span className="text-2xl font-bold">
                          {formatCurrency(amount, "FCFA", lang)}
                        </span>
                        <span className="ml-1 text-sm text-muted-foreground">
                          / {plan.period}
                        </span>
                      </div>
                    </CardHeader>
                    <CardContent>
                      <Button
                        className="w-full"
                        variant={plan.unlimited ? "default" : "outline"}
                        onClick={() => subscribe(plan.id as "essentiel" | "illimite")}
                        disabled={paying !== null}
                      >
                        {paying === plan.id ? (
                          <>
                            <Lock className="size-4 animate-pulse" />
                            {t("paywall.processing", "Traitement…")}
                          </>
                        ) : (
                          t("paywall.subscribe", "Souscrire & payer")
                        )}
                      </Button>
                      <ul className="mt-4 space-y-2">
                        {plan.period === "an" && (
                          <li className="flex items-start gap-2 text-sm">
                            <Check className="mt-0.5 size-4 text-primary" />
                            <span className="text-muted-foreground">
                              {t("paywall.unlimited", "Routeurs illimités")}
                            </span>
                          </li>
                        )}
                        {plan.period === "mois" && (
                          <li className="flex items-start gap-2 text-sm">
                            <Check className="mt-0.5 size-4 text-primary" />
                            <span className="text-muted-foreground">
                              {t("paywall.perRouter", "Payez au fil de votre croissance")}
                            </span>
                          </li>
                        )}
                      </ul>
                    </CardContent>
                  </Card>
                );
              })}
            </div>

            <Separator />

            <div className="flex items-center justify-center">
              <Button
                variant="ghost"
                size="sm"
                onClick={() => {
                  logout();
                }}
              >
                <LogOut className="size-4" />
                {t("paywall.logout", "Se déconnecter")}
              </Button>
            </div>
          </CardContent>
        </Card>
      </motion.div>
    </div>
  );
}
