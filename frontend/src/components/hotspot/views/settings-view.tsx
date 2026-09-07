"use client";

// N°57-d — Vue « Général » (section racine de la zone Paramètres,
// /app/settings/general). L'ancienne vue Paramètres portait trois onglets
// internes (Général / Hotspot / Sécurité) : les sections sont désormais
// ÉCLATÉES dans la sidebar de zone (une préoccupation = une section,
// cf. settings-sections.ts). Cette vue garde UNE préoccupation —
// l'identité de l'espace — SANS onglet interne :
//   • Organisation — nom, devise, fuseau horaire, lien Wave ;
//   • Langue — préférence d'interface (appliquée immédiatement) ;
//   • Abonnement — état réel (GET /api/subscription) + accès direct à la
//     vue dédiée (contenu de lecture, pas de duplicate du flux de paiement).
// Les cartes métier hotspot (expiration, tickets, portail) vivent dans la
// section Hotspot (parts/hotspot-cards.tsx), la sécurité dans la section
// Sécurité (views/security-view.tsx).

import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Building2, CreditCard, Languages } from "lucide-react";
import { toast } from "sonner";

import { api } from "@/lib/hotspot/api";
import { useI18n } from "@/lib/hotspot/i18n";
import { localeOf } from "@/lib/hotspot/i18n";
import type { AppSettings } from "@/lib/hotspot/types";
import { PageHeader } from "@/components/hotspot/page-header";
import { useSubscription } from "@/components/hotspot/parts/sa-subscription-card";
import { SETTINGS_QUERY_KEY, useSettings } from "@/components/hotspot/parts/sd-currency";
import { useHotspotStore } from "@/lib/hotspot/store";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";

const CURRENCIES = ["FCFA", "EUR", "USD", "MAD", "XOF", "GBP", "CDF", "GNF"];
const TIMEZONES = ["UTC", "Africa/Abidjan", "Africa/Dakar", "Africa/Casablanca", "Europe/Paris", "Europe/Brussels"];

/** Libellé de formule — clés i18n existantes, identifiant brut en repli. */
function planName(planId: string | undefined): string {
  if (planId === "essentiel") return "Essentiel";
  if (planId === "illimite") return "Illimité";
  return planId || "—";
}

interface SettingsForm {
  name: string;
  currency: string;
  timezone: string;
  waveLink?: string;
}

export default function SettingsView() {
  const { t } = useI18n();
  const { data, isLoading } = useSettings();

  if (isLoading || !data) {
    return (
      <div className="space-y-4 sm:space-y-6">
        <PageHeader title={t("settings.tabGeneral")} description={t("settings.generalDesc")} />
        <div className="grid grid-cols-1 gap-4 lg:grid-cols-2 lg:gap-6">
          <Skeleton className="h-96 rounded-xl" />
          <div className="grid gap-4">
            <Skeleton className="h-64 rounded-xl" />
            <Skeleton className="h-40 rounded-xl" />
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-4 sm:space-y-6">
      <PageHeader title={t("settings.tabGeneral")} description={t("settings.generalDesc")} />

      {/* Une préoccupation = une carte, aucune navigation interne : le
          propriétaire voit tout le Général d'un coup d'œil. */}
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2 lg:gap-6">
        <OrganizationCard settings={data} />
        <div className="grid grid-cols-1 content-start gap-4 sm:gap-6">
          <LanguageCard />
          <SubscriptionCard />
        </div>
      </div>
    </div>
  );
}

// Carte Langue (F11) — bascule FR/EN, appliquée immédiate (store zustand).
// La carte est bilingue par nature : titre affiché dans les deux langues.
function LanguageCard() {
  const { t, lang, setLang } = useI18n();
  return (
    <Card className="gap-4 py-4 sm:py-6">
      <CardHeader className="px-4 sm:px-6">
        <CardTitle className="flex items-center gap-2 text-base">
          <span className="flex size-8 items-center justify-center rounded-lg bg-primary/15 text-primary">
            <Languages className="size-4" />
          </span>
          {t("settings.language.title")}
        </CardTitle>
        <CardDescription>{t("settings.language.desc")}</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-4 px-4 sm:px-6">
        <RadioGroup
          value={lang}
          onValueChange={(value) => setLang(value === "en" ? "en" : "fr")}
          className="grid gap-3"
        >
          <label className="flex min-h-14 cursor-pointer items-center gap-3 rounded-lg border p-3 transition-colors has-[[data-state=checked]]:border-primary/50">
            <RadioGroupItem value="fr" className="mt-1" />
            <span className="text-sm">
              {t("settings.language.french")}
              <span className="block text-xs font-normal text-muted-foreground">FR</span>
            </span>
          </label>
          <label className="flex min-h-14 cursor-pointer items-center gap-3 rounded-lg border p-3 transition-colors has-[[data-state=checked]]:border-primary/50">
            <RadioGroupItem value="en" className="mt-1" />
            <span className="text-sm">
              {t("settings.language.english")}
              <span className="block text-xs font-normal text-muted-foreground">EN</span>
            </span>
          </label>
        </RadioGroup>
      </CardContent>
    </Card>
  );
}

// Formulaire Organisation — état local initialisé depuis les paramètres serveur.
function OrganizationCard({ settings }: { settings: AppSettings }) {
  const { t } = useI18n();
  const queryClient = useQueryClient();
  const [form, setForm] = useState<SettingsForm>(() => ({
    name: settings.tenant.name,
    currency: settings.tenant.currency,
    timezone: settings.tenant.timezone,
    waveLink: settings.tenant.waveLink ?? "",
  }));

  const saveMutation = useMutation({
    mutationFn: (payload: SettingsForm) =>
      api<AppSettings>("/api/settings", {
        method: "PUT",
        body: {
          name: payload.name,
          currency: payload.currency,
          timezone: payload.timezone,
          waveLink: payload.waveLink?.trim() ?? "",
        },
      }),
    onSuccess: () => {
      toast.success(t("settings.savedToast"));
      queryClient.invalidateQueries({ queryKey: SETTINGS_QUERY_KEY });
      // La devise se propage sur toutes les vues (dashboard, revendeurs, rapports…).
      queryClient.invalidateQueries();
    },
    onError: (error: Error) => toast.error(error.message),
  });

  const submitSettings = () => {
    if (!form.name.trim()) {
      toast.error(t("settings.orgNameRequired"));
      return;
    }
    saveMutation.mutate({ ...form, name: form.name.trim() });
  };

  return (
    <Card className="gap-4 py-4 sm:py-6">
      <CardHeader className="px-4 sm:px-6">
        <CardTitle className="flex items-center gap-2 text-base">
          <Building2 className="size-4 text-primary" />
          {t("settings.organization")}
        </CardTitle>
        <CardDescription>{t("settings.organizationDesc")}</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-4 px-4 sm:grid-cols-2 sm:px-6">
        <div className="grid gap-2 sm:col-span-2">
          <Label htmlFor="org-name">{t("settings.orgName")}</Label>
          <Input
            id="org-name"
            value={form.name}
            onChange={(event) => setForm((f) => ({ ...f, name: event.target.value }))}
            placeholder={t("settings.orgNamePlaceholder")}
          />
        </div>
        <div className="grid gap-2 sm:col-span-2">
          <Label htmlFor="org-wave">{t("settings.waveLink")}</Label>
          <Input
            id="org-wave"
            value={form.waveLink ?? ""}
            onChange={(event) => setForm((f) => ({ ...f, waveLink: event.target.value }))}
            placeholder={t("settings.waveLinkPlaceholder")}
          />
          <p className="text-xs text-muted-foreground">{t("settings.waveLinkDesc")}</p>
        </div>
        <div className="grid gap-2">
          <Label htmlFor="org-currency">{t("settings.currency")}</Label>
          <Select value={form.currency} onValueChange={(value) => setForm((f) => ({ ...f, currency: value }))}>
            <SelectTrigger id="org-currency" className="h-10 w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {CURRENCIES.map((currency) => (
                <SelectItem key={currency} value={currency}>
                  {currency}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="grid gap-2">
          <Label htmlFor="org-timezone">{t("settings.timezone")}</Label>
          <Select value={form.timezone} onValueChange={(value) => setForm((f) => ({ ...f, timezone: value }))}>
            <SelectTrigger id="org-timezone" className="h-10 w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {TIMEZONES.map((timezone) => (
                <SelectItem key={timezone} value={timezone}>
                  {timezone}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </CardContent>
      <CardFooter className="justify-end px-4 sm:px-6">
        <Button className="h-10" onClick={submitSettings} disabled={saveMutation.isPending || !form.name.trim()}>
          {saveMutation.isPending ? t("common.saving") : t("common.save")}
        </Button>
      </CardFooter>
    </Card>
  );
}

// Carte Abonnement (N°57-d) — contenu de LECTURE : état réel de l'espace
// (GET /api/subscription) sans dupliquer le flux de paiement (renouvellement,
// factures) qui vit dans la vue Abonnement dédiée (M).
function SubscriptionCard() {
  const { t, tf, lang } = useI18n();
  const setView = useHotspotStore((s) => s.setView);
  const { data, isLoading } = useSubscription();

  const status = data?.status ?? "none";
  const periodEnd = data?.subscription.periodEnd ?? "";
  const routerCount = data?.routerCount ?? 0;

  const df = new Intl.DateTimeFormat(localeOf(lang), { day: "2-digit", month: "short", year: "numeric" });
  const statusBadge =
    status === "active"
      ? "border-emerald-500/40 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400"
      : status === "expired" || status === "suspended"
        ? "border-destructive/40 bg-destructive/10 text-destructive"
        : "border-border bg-muted text-muted-foreground";

  return (
    <Card className="gap-4 py-4 sm:py-6">
      <CardHeader className="px-4 sm:px-6">
        <CardTitle className="flex items-center justify-between gap-2 text-base">
          <span className="flex min-w-0 items-center gap-2">
            <span className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-primary/15 text-primary">
              <CreditCard className="size-4" />
            </span>
            <span className="truncate">{t("settings.subscriptionCard")}</span>
          </span>
          <Badge variant="outline" className={`shrink-0 text-[11px] font-semibold ${statusBadge}`}>
            {status === "active"
              ? t("sub.status.active")
              : status === "expired"
                ? t("sub.status.expired")
                : status === "suspended"
                  ? t("sub.status.suspended")
                  : t("sub.status.none")}
          </Badge>
        </CardTitle>
        <CardDescription>{t("settings.subscriptionCardDesc")}</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-3 px-4 sm:grid-cols-2 sm:px-6">
        {isLoading ? (
          <Skeleton className="h-16 rounded-lg sm:col-span-2" />
        ) : (
          <>
            <div className="flex min-h-14 items-center justify-between gap-3 rounded-lg border p-3">
              <span className="text-sm text-muted-foreground">{t("sub.planWord")}</span>
              <span className="text-sm font-semibold">{planName(data?.subscription.planId)}</span>
            </div>
            <div className="flex min-h-14 items-center justify-between gap-3 rounded-lg border p-3">
              <span className="text-sm text-muted-foreground">{t("routers.title")}</span>
              <span className="text-sm font-semibold tabular-nums">{routerCount}</span>
            </div>
            {periodEnd && (
              <div className="flex min-h-14 items-center gap-3 rounded-lg border p-3 sm:col-span-2">
                <span className="text-sm font-medium">
                  {tf("sub.renewalOn", { date: df.format(new Date(periodEnd)) })}
                </span>
              </div>
            )}
          </>
        )}
      </CardContent>
      <CardFooter className="px-4 sm:px-6">
        <Button
          type="button"
          variant="outline"
          className="h-10 w-full sm:w-auto"
          onClick={() => setView("subscription")}
        >
          {t("settings.subscriptionManage")}
        </Button>
      </CardFooter>
    </Card>
  );
}
