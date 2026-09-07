"use client";

// N°57-d — Vue « Général » (section racine de la zone Paramètres,
// /app/settings/general). L'ancienne vue Paramètres portait trois onglets
// internes (Général / Hotspot / Sécurité) : les sections sont désormais
// ÉCLATÉES dans la sidebar de zone (une préoccupation = une section,
// cf. settings-sections.ts). Cette vue garde UNE préoccupation —
// l'identité de l'espace — SANS onglet interne :
//   • Organisation — nom, devise, fuseau horaire, lien Wave ;
//   • Langue — préférence d'interface (appliquée immédiatement).
// N°57-e — la carte pont « Abonnement » (accès direct à la vue dédiée,
// créée en N°57-d quand l'Abonnement vivait HORS zone) est retirée : la
// facturation est désormais une section SŒUR de la sidebar de zone
// (/app/settings/subscription) — le pont devient redondant.
// Les cartes métier hotspot (expiration, tickets, portail) vivent dans la
// section Hotspot (parts/hotspot-cards.tsx), la sécurité dans la section
// Sécurité (views/security-view.tsx).

import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Building2, Languages, ShieldCheck } from "lucide-react";
import { toast } from "sonner";

import { api } from "@/lib/hotspot/api";
import { useI18n } from "@/lib/hotspot/i18n";
import type { AppSettings } from "@/lib/hotspot/types";
import { PageHeader } from "@/components/hotspot/page-header";
import { SETTINGS_QUERY_KEY, useSettings } from "@/components/hotspot/parts/sd-currency";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";

const CURRENCIES = ["FCFA", "EUR", "USD", "MAD", "XOF", "GBP", "CDF", "GNF"];
const TIMEZONES = ["UTC", "Africa/Abidjan", "Africa/Dakar", "Africa/Casablanca", "Europe/Paris", "Europe/Brussels"];

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
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-4 sm:space-y-6">
      <PageHeader title={t("settings.tabGeneral")} description={t("settings.generalDesc")} />

      {/* Une préoccupation = une carte, aucune navigation interne : le
          propriétaire voit tout le Général d'un coup d'œil. N°57-e : la
          facturation vit dans la section Abonnement (sidebar de zone). */}
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2 lg:gap-6">
        <OrganizationCard settings={data} />
        <div className="grid grid-cols-1 content-start gap-4 sm:gap-6">
          <LanguageCard />
          <RetentionCard settings={data} />
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

// Carte Rétention du journal (N°65) — durée de conservation des journaux de
// connexion du compte (30/60/90 j, défaut 90). La purge automatique horaire
// (N°64) applique la valeur du compte ; la note de confidentialité du portail
// captif et la bannière de la vue Journal la reflètent.
function RetentionCard({ settings }: { settings: AppSettings }) {
  const { t, tf } = useI18n();
  const queryClient = useQueryClient();
  // Absent du JSON tant que le réglage n'a pas été touché = 90 (N°64).
  const effective = settings.tenant.logRetentionDays ?? 90;
  const [days, setDays] = useState(String(effective));

  const saveMutation = useMutation({
    mutationFn: (value: number) =>
      api<AppSettings>("/api/settings", {
        method: "PUT",
        // Corps défensif : champ plat + forme imbriquée tenant{…} (même
        // convention que les autres réglages du compte).
        body: { logRetentionDays: value, tenant: { logRetentionDays: value } },
      }),
    onSuccess: () => {
      toast.success(t("settings.retentionSavedToast"));
      void queryClient.invalidateQueries({ queryKey: SETTINGS_QUERY_KEY });
    },
    onError: (error: Error) => toast.error(error.message),
  });

  return (
    <Card className="gap-4 py-4 sm:py-6">
      <CardHeader className="px-4 sm:px-6">
        <CardTitle className="flex items-center gap-2 text-base">
          <span className="flex size-8 items-center justify-center rounded-lg bg-primary/15 text-primary">
            <ShieldCheck className="size-4" />
          </span>
          {t("settings.retentionCard")}
        </CardTitle>
        <CardDescription>{t("settings.retentionCardDesc")}</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-4 px-4 sm:px-6">
        <div className="grid gap-2">
          <Label htmlFor="log-retention-days">{t("settings.retentionLabel")}</Label>
          <Select value={days} onValueChange={setDays}>
            <SelectTrigger id="log-retention-days" className="h-10 w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {[30, 60, 90].map((value) => (
                <SelectItem key={value} value={String(value)}>
                  {value === 90
                    ? tf("settings.retentionDefaultOption", { days: value })
                    : tf("settings.retentionOption", { days: value })}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <p className="text-xs leading-relaxed text-muted-foreground">{t("settings.retentionHint")}</p>
        </div>
      </CardContent>
      <CardFooter className="justify-end px-4 sm:px-6">
        <Button
          className="h-10"
          onClick={() => saveMutation.mutate(Number(days))}
          disabled={saveMutation.isPending}
        >
          {saveMutation.isPending ? t("common.saving") : t("common.save")}
        </Button>
      </CardFooter>
    </Card>
  );
}
