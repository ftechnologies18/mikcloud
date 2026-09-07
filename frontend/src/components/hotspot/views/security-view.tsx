"use client";

// N°57-d — Vue « Sécurité » (section de la zone Paramètres,
// /app/settings/security). L'onglet Sécurité de l'ancienne vue Paramètres
// devient une section à part entière — SANS onglet interne :
//   • Mot de passe — changement immédiat (POST /api/auth/password) ;
//   • Double authentification — 2FA TOTP (cartes PARTAGÉES parts/
//     security-cards, même implémentation que la console plateforme) ;
//   • Activité récente — les dernières actions réelles du journal
//     (GET /api/activity), lecture seule, avec accès direct au journal
//     complet (vue Logs) : contenu réel, pas de données factices.
// Propriétaire uniquement (rang 3) — la section est masquée au gérant dans
// la sidebar de zone (miroir canView, la carte mot de passe reste ouverte à
// tous via le menu profil).

import { useQuery } from "@tanstack/react-query";
import { History, ScrollText } from "lucide-react";

import { api } from "@/lib/hotspot/api";
import { timeAgo } from "@/lib/hotspot/format";
import { useI18n } from "@/lib/hotspot/i18n";
import { useHotspotStore } from "@/lib/hotspot/store";
import type { Activity } from "@/lib/hotspot/types";
import { PageHeader } from "@/components/hotspot/page-header";
import { SecurityCard, TwoFactorCard } from "@/components/hotspot/parts/security-cards";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card";
import { ScrollArea } from "@/components/ui/scroll-area";

export default function SecurityView() {
  const { t } = useI18n();

  return (
    <div className="space-y-4 sm:space-y-6">
      <PageHeader title={t("settings.tabAdvanced")} description={t("settings.securitySectionDesc")} />

      {/* Une préoccupation = une carte, aucune navigation interne. */}
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2 lg:gap-6">
        {/* Sécurité — changement de mot de passe (POST /api/auth/password,
            tout utilisateur connecté). */}
        <SecurityCard />

        {/* Sécurité S4 — 2FA TOTP (pairage, activation, désactivation). */}
        <TwoFactorCard />
      </div>

      <ActivityCard />
    </div>
  );
}

/** Journal récent — vraies données du compte (GET /api/activity), filtrées
 * sur les types qui touchent la sécurité de l'espace : comptes et système.
 * Lecture seule ; le journal complet vit dans la vue Logs (rang 2). */
function ActivityCard() {
  const { t, lang } = useI18n();
  const setView = useHotspotStore((s) => s.setView);

  const { data, isLoading } = useQuery({
    queryKey: ["/api/activity", "security-recent"],
    queryFn: () => api<Activity[]>("/api/activity", { params: { limit: 40 } }),
    staleTime: 30_000,
    retry: false,
  });

  // Sécurité de l'espace : comptes (créations, rôles, mots de passe) et
  // réglages système — pas le trafic métier (ventes, vouchers, sessions).
  const entries = (data ?? [])
    .filter((a) => a.type === "user" || a.type === "system")
    .slice(0, 8);

  return (
    <Card className="gap-4 py-4 sm:py-6">
      <CardHeader className="px-4 sm:px-6">
        <CardTitle className="flex items-center gap-2 text-base">
          <span className="flex size-8 items-center justify-center rounded-lg bg-primary/15 text-primary">
            <History className="size-4" />
          </span>
          {t("settings.securityActivity")}
        </CardTitle>
        <CardDescription>{t("settings.securityActivityDesc")}</CardDescription>
      </CardHeader>
      <CardContent className="px-4 sm:px-6">
        {isLoading ? (
          <p className="text-sm text-muted-foreground" role="status">
            {t("common.loading")}
          </p>
        ) : entries.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t("topbar.bellEmpty")}</p>
        ) : (
          <ScrollArea className="max-h-72 rounded-lg border">
            <ul className="divide-y">
              {entries.map((entry) => (
                <li key={entry.id} className="flex items-start justify-between gap-3 p-3 text-sm">
                  <span className="min-w-0 flex-1">{entry.message}</span>
                  <span className="shrink-0 text-xs text-muted-foreground">
                    {timeAgo(entry.at, lang)}
                  </span>
                </li>
              ))}
            </ul>
          </ScrollArea>
        )}
      </CardContent>
      <CardFooter className="px-4 sm:px-6">
        <Button
          type="button"
          variant="outline"
          className="h-10"
          onClick={() => setView("logs")}
        >
          <ScrollText className="size-4" />
          {t("settings.securitySeeLogs")}
        </Button>
      </CardFooter>
    </Card>
  );
}
