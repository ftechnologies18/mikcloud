"use client";

// N°83 — Vue « Protection » (/app/protection, section Supervision de la
// sidebar principale) : l'état de sécurité du WiFi en un coup d'œil.
//
// Avant N°83, les 3 modules sécurité (SafeWiFi N°80, Shield N°81,
// FamilyGuard N°82) vivaient au fond du 4e onglet « Système » de la fiche
// routeur, elle-même au fond de la zone Paramètres — 5 à 6 interactions
// pour toucher les fonctions différenciantes du produit. La protection
// est un ARGUMENT DE VENTE : elle vit désormais à 1 clic de l'atterrissage.
//
//   • Verdict par routeur — Bien protégé / À renforcer / Non protégé —
//     calculé depuis les champs de GET /api/routers (zéro endpoint neuf) ;
//   • Les 3 cartes actionnables directement (composants partagés
//     parts/protection-cards.tsx) ;
//   • Mono-routeur : l'étape de sélection est sautée (le cas de la quasi-
//     totalité des comptes) ;
//   • Fiche adressable /app/protection/<id> (pattern routers) : le CTA du
//     résumé (onglet Système) ouvre CE routeur, Retour navigateur rejoué.

import { useEffect } from "react";
import { usePathname, useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import {
  Activity,
  Router as RouterIcon,
  ShieldCheck,
  TriangleAlert,
  Wrench,
} from "lucide-react";

import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { EmptyState } from "@/components/hotspot/empty-state";
import { LoadingCards } from "@/components/hotspot/loading";
import { PageHeader } from "@/components/hotspot/page-header";
import {
  AntiVpnCard,
  FamilyGuardCard,
  ProtectionVerdictBadge,
  SafeWifiCard,
  ShieldCard,
} from "@/components/hotspot/parts/protection-cards";
import { StatusBadge } from "@/components/hotspot/status-badge";
import { api } from "@/lib/hotspot/api";
import { useI18n } from "@/lib/hotspot/i18n";
import { protectionScore, protectionVerdict } from "@/lib/hotspot/protection";
import type { RouterDevice } from "@/lib/hotspot/types";
import { detailFromPath, viewToPath } from "@/lib/hotspot/view-path";

export default function ProtectionView() {
  const { t, tf } = useI18n();
  const nav = useRouter();
  const pathname = usePathname();

  const { data: routers, isLoading, isError, refetch } = useQuery({
    queryKey: ["/api/routers"],
    queryFn: () => api<RouterDevice[]>("/api/routers"),
    refetchInterval: 15_000,
  });

  // ── Fiche adressable /app/protection/<id> (pattern routers) : le segment
  // vit dans l'URL — le routeur est résolu depuis le poll partagé (fiche
  // « vivante », même cache queryKey que la liste des routeurs).
  const detailId = detailFromPath(pathname, "protection");

  // Segment orphelin (routeur supprimé, signet périmé) : re-normalisation
  // vers la racine de la vue — replace, zéro entrée d'historique parasite.
  useEffect(() => {
    if (!routers || !detailId) return;
    if (!routers.some((r) => r.id === detailId)) {
      nav.replace(viewToPath("protection"), { scroll: false });
    }
  }, [routers, detailId, nav]);

  if (isLoading) {
    return (
      <div className="space-y-6">
        <PageHeader title={t("protection.title")} description={t("protection.description")} />
        <Skeleton className="h-24 w-full rounded-xl" />
        <LoadingCards cards={4} />
      </div>
    );
  }

  if (isError || !routers) {
    return (
      <Card>
        <EmptyState
          icon={Activity}
          title={t("protection.loadError")}
          description={t("protection.loadErrorDesc")}
          action={
            <Button variant="outline" onClick={() => void refetch()}>
              {t("common.retry")}
            </Button>
          }
        />
      </Card>
    );
  }

  if (routers.length === 0) {
    return (
      <Card>
        <EmptyState
          icon={ShieldCheck}
          title={t("protection.empty")}
          description={t("protection.emptyDesc")}
          action={
            <Button onClick={() => nav.push(viewToPath("routers"), { scroll: false })}>
              <RouterIcon className="size-4" aria-hidden />
              {t("protection.openRouters")}
            </Button>
          }
        />
      </Card>
    );
  }

  // Sélection : segment URL > 1er routeur. Mono-routeur (le cas des
  // comptes réels) : aucune étape de sélection, verdict immédiat.
  const selected = routers.find((r) => r.id === detailId) ?? routers[0];
  const verdict = protectionVerdict(selected);
  const score = protectionScore(selected);

  function selectRouter(id: string) {
    // L'URL porte la sélection (pattern fiche routeur) : Retour navigateur
    // rejoué, partage de lien direct possible.
    nav.push(viewToPath("protection", id), { scroll: false });
  }

  return (
    <div className="space-y-4 sm:space-y-6">
      <PageHeader title={t("protection.title")} description={t("protection.description")} />

      {/* Multi-sites : sélecteur (mono-routeur = étape sautée). */}
      {routers.length > 1 && (
        <div className="max-w-xs space-y-1.5">
          <label htmlFor="protection-router-select" className="text-xs font-medium text-muted-foreground">
            {t("protection.selectRouter")}
          </label>
          <Select value={selected.id} onValueChange={selectRouter}>
            <SelectTrigger id="protection-router-select" className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {routers.map((r) => (
                <SelectItem key={r.id} value={r.id}>
                  {r.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      )}

      {/* En-tête du site : identité + statut + verdict en 5 secondes. */}
      <Card className="gap-0 py-0">
        <CardContent className="p-4 sm:p-5">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div className="flex min-w-0 items-center gap-3">
              <span className="flex size-10 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
                <RouterIcon className="size-5" aria-hidden />
              </span>
              <div className="min-w-0">
                <p className="truncate font-semibold" title={selected.name}>
                  {selected.name}
                </p>
                <span className="mt-1 flex flex-wrap items-center gap-2">
                  <StatusBadge status={selected.mode} />
                  <StatusBadge status={selected.status} dot />
                </span>
              </div>
            </div>
            <div className="flex flex-wrap items-center gap-3">
              <span className="text-xs font-medium text-muted-foreground tabular-nums">
                {tf("protection.ofModules", { n: score })}
              </span>
              <ProtectionVerdictBadge verdict={verdict} />
            </div>
          </div>

          {selected.mode !== "agent" && (
            <p className="mt-3 flex items-start gap-2 rounded-lg border border-amber-500/40 bg-amber-500/10 px-3 py-2 text-xs text-amber-700 dark:text-amber-300">
              <TriangleAlert className="mt-0.5 size-3.5 shrink-0" aria-hidden />
              {t("protection.agentHint")}
            </p>
          )}

          <Button
            type="button"
            variant="outline"
            size="sm"
            className="mt-3"
            onClick={() => nav.push(viewToPath("routers", selected.id), { scroll: false })}
          >
            <Wrench className="size-3.5" aria-hidden />
            {t("protection.openRouter")}
          </Button>
        </CardContent>
      </Card>

      {/* Les 4 protections — actionnables sans quitter la vue. */}
      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-4">
        <SafeWifiCard router={selected} />
        <ShieldCard router={selected} />
        <FamilyGuardCard router={selected} />
        <AntiVpnCard router={selected} />
      </div>
    </div>
  );
}
