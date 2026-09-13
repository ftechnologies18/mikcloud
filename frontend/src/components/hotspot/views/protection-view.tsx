"use client";

// N°96 — Refonte UX/UI de la vue « Protection » (/app/protection, section
// Supervision de la sidebar principale).
//
// Historique (N°83) : les modules sécurité (SafeWiFi N°80, Shield N°81,
// FamilyGuard N°82 — puis AntiVPN N°88) vivaient au fond du 4e onglet
// « Système » de la fiche routeur ; la protection est un ARGUMENT DE VENTE,
// elle vit à 1 clic de l'atterrissage. Fiche adressable /app/protection/<id>
// (pattern routers), mono-routeur sans étape de sélection, verdict calculé
// depuis les champs de GET /api/routers (zéro endpoint neuf).
//
// N°96 — la promesse « l'état en un coup d'œil » devient littérale :
//   • HÉROS : identité du site (nom + badges) à gauche, colonne score à
//     droite — anneau n/4 coloré par verdict (ProtectionScoreRing) + badge
//     « Bien protégé / À renforcer / Non protégé » ;
//   • ENSEIGNEMENT : un encart selon le verdict — tout actif (encens
//     sobre), modules MANQUANTS NOMMÉS (« À activer : Bloque-VPN, … » :
//     le gérant sait exactement quoi actionner, sans scanner les 4 cartes),
//     aucune protection (par où commencer : le filtrage de sites) ;
//   • GRILLE 2 colonnes dès md : SafeWiFi porte 3 options et FamilyGuard
//     un éditeur complet — 4 colonnes les compressaient, la lecture des
//     bénéfices reprenait à la ligne à chaque mot ;
//   • chargement miroir (héros + 4 cartes squelettes), comportements
//     inchangés (poll 15 s, segment orphelin re-normalisé, gardes agent).

import { useEffect } from "react";
import { usePathname, useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import {
  Activity,
  Router as RouterIcon,
  ShieldAlert,
  ShieldCheck,
  ShieldHalf,
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
import { PageHeader } from "@/components/hotspot/page-header";
import {
  AntiVpnCard,
  FamilyGuardCard,
  ProtectionScoreRing,
  ProtectionVerdictBadge,
  SafeWifiCard,
  ShieldCard,
} from "@/components/hotspot/parts/protection-cards";
import { StatusBadge } from "@/components/hotspot/status-badge";
import { api } from "@/lib/hotspot/api";
import { useI18n } from "@/lib/hotspot/i18n";
import {
  antiVpnOn,
  parseFamilyGuardSpec,
  protectionScore,
  protectionVerdict,
  safeWifiLevelOf,
  shieldOn,
  type ProtectionVerdict,
} from "@/lib/hotspot/protection";
import type { RouterDevice } from "@/lib/hotspot/types";
import { detailFromPath, viewToPath } from "@/lib/hotspot/view-path";

/** Encart pédagogique du héros (N°96) : le verdict ne dit pas seulement
 * « À renforcer », il dit QUOI renforcer — les modules inactifs sont
 * nommés, le gérant n'a plus qu'à repérer les cartes correspondantes
 * (leurs chips « Inactif » répondent à l'encart). */
function VerdictCallout({ verdict, missing }: { verdict: ProtectionVerdict; missing: string[] }) {
  const { t, tf } = useI18n();

  if (verdict === "protected") {
    return (
      <p className="flex items-start gap-2 rounded-lg border border-primary/25 bg-primary/5 px-3 py-2 text-xs leading-relaxed text-foreground">
        <ShieldCheck className="mt-0.5 size-3.5 shrink-0 text-primary" aria-hidden />
        {t("protection.hero.allOn")}
      </p>
    );
  }

  if (verdict === "unprotected") {
    return (
      <p className="flex items-start gap-2 rounded-lg border border-amber-500/40 bg-amber-500/10 px-3 py-2 text-xs leading-relaxed text-amber-700 dark:text-amber-300">
        <ShieldAlert className="mt-0.5 size-3.5 shrink-0" aria-hidden />
        {t("protection.hero.noneOn")}
      </p>
    );
  }

  return (
    <p className="flex items-start gap-2 rounded-lg border border-amber-500/25 bg-amber-500/5 px-3 py-2 text-xs leading-relaxed text-amber-700 dark:text-amber-300">
      <ShieldHalf className="mt-0.5 size-3.5 shrink-0" aria-hidden />
      {tf("protection.hero.missing", { modules: missing.join(", ") })}
    </p>
  );
}

export default function ProtectionView() {
  const { t } = useI18n();
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
      <div className="space-y-4 sm:space-y-6">
        <PageHeader title={t("protection.title")} description={t("protection.description")} />
        <Skeleton className="h-44 w-full rounded-xl sm:h-40" />
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
          {Array.from({ length: 4 }).map((_, i) => (
            <Skeleton key={i} className="h-56 rounded-xl" />
          ))}
        </div>
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

  // Modules inactifs, dans l'ordre des cartes — l'encart du héros les
  // nomme (source : mêmes helpers purs que le score, zéro dérive).
  const missing = [
    safeWifiLevelOf(selected) === "off" ? t("tools.safewifi.title") : null,
    !shieldOn(selected) ? t("tools.shield.title") : null,
    !parseFamilyGuardSpec(selected.familyGuardSpec).enabled ? t("tools.familyguard.title") : null,
    !antiVpnOn(selected) ? t("tools.antivpn.title") : null,
  ].filter((name): name is string => name !== null);

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

      {/* Héros du site : identité + enseignement à gauche, verdict chiffré
          à droite — l'état de sécurité se lit en une seconde. */}
      <Card className="gap-0 py-0">
        <CardContent className="p-4 sm:p-5">
          <div className="grid items-center gap-4 sm:grid-cols-[minmax(0,1fr)_auto] sm:gap-6">
            <div className="min-w-0 space-y-3">
              <div className="flex items-center gap-3">
                <span className="flex size-11 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
                  <RouterIcon className="size-5" aria-hidden />
                </span>
                <div className="min-w-0">
                  <p className="truncate text-base font-semibold leading-tight" title={selected.name}>
                    {selected.name}
                  </p>
                  <span className="mt-1.5 flex flex-wrap items-center gap-2">
                    <StatusBadge status={selected.mode} />
                    <StatusBadge status={selected.status} dot />
                  </span>
                </div>
              </div>

              <VerdictCallout verdict={verdict} missing={missing} />

              {selected.mode !== "agent" && (
                <p className="flex items-start gap-2 rounded-lg border border-amber-500/40 bg-amber-500/10 px-3 py-2 text-xs leading-relaxed text-amber-700 dark:text-amber-300">
                  <TriangleAlert className="mt-0.5 size-3.5 shrink-0" aria-hidden />
                  {t("protection.agentHint")}
                </p>
              )}

              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => nav.push(viewToPath("routers", selected.id), { scroll: false })}
              >
                <Wrench className="size-3.5" aria-hidden />
                {t("protection.openRouter")}
              </Button>
            </div>

            {/* Colonne score : anneau n/4 + badge verdict (centrée sur
                mobile où la grille s'empile, alignée à droite ensuite). */}
            <div className="flex flex-col items-center gap-2 justify-self-center sm:justify-self-end">
              <ProtectionScoreRing score={score} verdict={verdict} />
              <ProtectionVerdictBadge verdict={verdict} />
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Les 4 protections — actionnables sans quitter la vue. Deux
          colonnes dès md : les éditeurs (SafeWiFi 3 options, FamilyGuard
          planning complet) respirent, les bénéfices se lisent d'un trait. */}
      <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
        <SafeWifiCard router={selected} />
        <ShieldCard router={selected} />
        <FamilyGuardCard router={selected} />
        <AntiVpnCard router={selected} />
      </div>
    </div>
  );
}
