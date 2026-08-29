"use client";

// Vue Paramètres — organisation, abonnement, expiration des vouchers, branding
// voucher (DNS + logo, F2), langue de l'interface (F11), connexion routeur réel,
// sécurité, zone sensible.

import { useRef, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Building2,
  CalendarClock,
  Database,
  Eye,
  EyeOff,
  Globe,
  ImagePlus,
  Image as ImageIcon,
  KeyRound,
  Languages,
  Router as RouterIcon,
  Ticket,
  TriangleAlert,
  X,
} from "lucide-react";
import { toast } from "sonner";

import { api } from "@/lib/hotspot/api";
import { useI18n } from "@/lib/hotspot/i18n";
import type { AppSettings, ExpiryPolicyMode } from "@/lib/hotspot/types";
import { useHotspotStore } from "@/lib/hotspot/store";
import { PageHeader } from "@/components/hotspot/page-header";
import { SETTINGS_QUERY_KEY, useSettings } from "@/components/hotspot/parts/sd-currency";
import { SubscriptionCard } from "@/components/hotspot/parts/sa-subscription-card";
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
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
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

const MIKROTIK_STEPS = [
  { titleKey: "settings.guide.step1", textKey: "settings.guide.step1Text" },
  { titleKey: "settings.guide.step2", textKey: "settings.guide.step2Text" },
  { titleKey: "settings.guide.step3", textKey: "settings.guide.step3Text" },
];

interface SettingsForm {
  name: string;
  currency: string;
  timezone: string;
  waveLink?: string;
}

// Réponse de POST /api/admin/reload — résumé de l'état réimporté.
interface ReloadStats {
  ok: boolean;
  accounts: number;
  users: number;
  hotspotUsers: number;
  routers: number;
  sessions: number;
}

export default function SettingsView() {
  const { t, tf } = useI18n();
  const queryClient = useQueryClient();
  const { data, isLoading } = useSettings();
  const [resetOpen, setResetOpen] = useState(false);
  const user = useHotspotStore((s) => s.user);
  // La réinitialisation des données devient admin-only côté serveur.
  const isAdmin = user?.role === "admin";

  const resetMutation = useMutation({
    mutationFn: () => api<{ ok: boolean }>("/api/admin/reset", { method: "POST" }),
    onSuccess: () => {
      toast.success(t("settings.resetToast"));
      setResetOpen(false);
      queryClient.invalidateQueries();
    },
    onError: (error: Error) => toast.error(error.message),
  });

  const reloadMutation = useMutation({
    mutationFn: () => api<ReloadStats>("/api/admin/reload", { method: "POST" }),
    onSuccess: (stats) => {
      toast.success(
        tf("settings.reloadedToast", {
          accounts: stats.accounts,
          users: stats.hotspotUsers,
          routers: stats.routers,
        }),
      );
      queryClient.invalidateQueries();
    },
    onError: (error: Error) => toast.error(error.message),
  });

  if (isLoading || !data) {
    return (
      <div className="space-y-4 sm:space-y-6">
        <PageHeader title={t("settings.title")} description={t("settings.description")} />
        <div className="grid grid-cols-1 gap-4 lg:grid-cols-3 lg:gap-6">
          <Skeleton className="h-96 rounded-xl lg:col-span-2" />
          <Skeleton className="h-96 rounded-xl" />
          <Skeleton className="h-64 rounded-xl lg:col-span-2" />
          <Skeleton className="h-64 rounded-xl" />
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-4 sm:space-y-6">
      <PageHeader title={t("settings.title")} description={t("settings.description")} />

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-3 lg:gap-6">
        {/* Organisation */}
        <OrganizationCard settings={data} />

        {/* Abonnement SaaS — formules Essentiel (1 250 F/mois/routeur) et
            Illimité (12 000 F/an, routeurs illimités), paiement via Wave. */}
        <SubscriptionCard />

        {/* Expiration des vouchers (F1/F5) — politique de nettoyage cloud */}
        <ExpiryCard settings={data} />

        {/* Vouchers — DNS + logo (F2) */}
        <VoucherCard settings={data} />

        {/* Langue de l'interface (F11) */}
        <LanguageCard />

        {/* Guide connexion routeur réel */}
        <Card className="gap-4 border-primary/20 bg-primary/5 py-4 sm:py-6 lg:col-span-2">
          <CardHeader className="px-4 sm:px-6">
            <CardTitle className="flex items-center gap-2 text-base">
              <span className="flex size-8 items-center justify-center rounded-lg bg-primary/15 text-primary">
                <RouterIcon className="size-4" />
              </span>
              {t("settings.guide.title")}
            </CardTitle>
            <CardDescription>{t("settings.guide.desc")}</CardDescription>
          </CardHeader>
          <CardContent className="px-4 sm:px-6">
            <ol className="grid gap-4 sm:grid-cols-3">
              {MIKROTIK_STEPS.map((step, index) => (
                <li key={step.titleKey} className="rounded-lg border bg-card p-3">
                  <p className="flex items-center gap-2 text-sm font-medium">
                    <span className="flex size-6 shrink-0 items-center justify-center rounded-full bg-primary/15 text-xs font-semibold text-primary">
                      {index + 1}
                    </span>
                    {t(step.titleKey)}
                  </p>
                  <p className="mt-1.5 text-xs leading-relaxed text-muted-foreground">{t(step.textKey)}</p>
                </li>
              ))}
            </ol>
            <p className="mt-4 text-xs text-muted-foreground">{t("settings.guide.simulatedNote")}</p>
          </CardContent>
        </Card>

        {/* Sécurité — changement de mot de passe (POST /api/auth/password, tout utilisateur connecté) */}
        <SecurityCard />

        {/* Base de données — admin plateforme uniquement (POST /api/admin/reload admin-only) */}
        {isAdmin && (
          <Card className="gap-4 py-4 sm:py-6">
            <CardHeader className="px-4 sm:px-6">
              <CardTitle className="flex items-center gap-2 text-base">
                <span className="flex size-8 items-center justify-center rounded-lg bg-primary/15 text-primary">
                  <Database className="size-4" />
                </span>
                {t("settings.database")}
              </CardTitle>
              <CardDescription>{t("settings.databaseDesc")}</CardDescription>
            </CardHeader>
            <CardFooter className="px-4 sm:px-6">
              <Button
                variant="outline"
                className="h-10"
                onClick={() => reloadMutation.mutate()}
                disabled={reloadMutation.isPending}
              >
                {reloadMutation.isPending ? t("settings.reloading") : t("settings.reload")}
              </Button>
            </CardFooter>
          </Card>
        )}

        {/* Zone sensible — admin plateforme uniquement (endpoint /api/admin/reset admin-only) */}
        {isAdmin && (
          <Card className="gap-4 border-destructive/30 py-4 sm:py-6">
            <CardHeader className="px-4 sm:px-6">
              <CardTitle className="flex items-center gap-2 text-base text-destructive">
                <TriangleAlert className="size-4" />
                {t("settings.dangerZone")}
              </CardTitle>
              <CardDescription>{t("settings.dangerZoneDesc")}</CardDescription>
            </CardHeader>
            <CardFooter className="px-4 sm:px-6">
              <Button variant="destructive" className="h-10" onClick={() => setResetOpen(true)}>
                {t("settings.reset")}
              </Button>
            </CardFooter>
          </Card>
        )}
      </div>

      {/* Double confirmation de réinitialisation */}
      <AlertDialog open={resetOpen} onOpenChange={setResetOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t("settings.resetTitle")}</AlertDialogTitle>
            <AlertDialogDescription>{t("settings.resetDesc")}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-white hover:bg-destructive/90"
              onClick={(event) => {
                event.preventDefault();
                resetMutation.mutate();
              }}
            >
              {resetMutation.isPending ? t("settings.resetting") : t("settings.reset")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

// Carte Langue (F11) — bascule FR/EN, appliquée immédiatement (store zustand).
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

// Champ mot de passe avec bascule de visibilité.
function PasswordInput({
  id,
  value,
  onChange,
  placeholder,
  autoComplete,
}: {
  id: string;
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  autoComplete?: string;
}) {
  const { t } = useI18n();
  const [visible, setVisible] = useState(false);
  return (
    <div className="relative">
      <Input
        id={id}
        type={visible ? "text" : "password"}
        value={value}
        onChange={(event) => onChange(event.target.value)}
        placeholder={placeholder}
        autoComplete={autoComplete}
        className="h-10 pr-10"
      />
      <button
        type="button"
        onClick={() => setVisible((v) => !v)}
        aria-label={visible ? t("settings.passwordHide") : t("settings.passwordShow")}
        className="absolute right-1.5 top-1/2 flex size-7 -translate-y-1/2 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
      >
        {visible ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
      </button>
    </div>
  );
}

// Carte Sécurité — changement du mot de passe de connexion de l'utilisateur courant.
// Le backend exige le mot de passe actuel (une session ouverte ne suffit pas).
function SecurityCard() {
  const { t } = useI18n();
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");

  const changeMutation = useMutation({
    mutationFn: () =>
      api<{ ok: boolean }>("/api/auth/password", {
        method: "POST",
        body: { currentPassword, newPassword },
      }),
    onSuccess: () => {
      toast.success(t("settings.passwordChangedToast"), {
        description: t("settings.passwordChangedToastDesc"),
      });
      setCurrentPassword("");
      setNewPassword("");
      setConfirmPassword("");
    },
    onError: (error: Error) => toast.error(error.message),
  });

  const canSubmit =
    currentPassword !== "" && newPassword !== "" && confirmPassword !== "" && !changeMutation.isPending;

  function submitPassword() {
    if (newPassword.length < 8) {
      toast.error(t("settings.passwordTooShort"));
      return;
    }
    if (newPassword === currentPassword) {
      toast.error(t("settings.passwordSame"));
      return;
    }
    if (newPassword !== confirmPassword) {
      toast.error(t("settings.passwordMismatch"));
      return;
    }
    changeMutation.mutate();
  }

  return (
    <Card className="gap-4 py-4 sm:py-6">
      <CardHeader className="px-4 sm:px-6">
        <CardTitle className="flex items-center gap-2 text-base">
          <span className="flex size-8 items-center justify-center rounded-lg bg-primary/15 text-primary">
            <KeyRound className="size-4" />
          </span>
          {t("settings.security")}
        </CardTitle>
        <CardDescription>{t("settings.securityDesc")}</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-4 px-4 sm:px-6">
        <div className="grid gap-2">
          <Label htmlFor="pwd-current">{t("settings.currentPassword")}</Label>
          <PasswordInput
            id="pwd-current"
            value={currentPassword}
            onChange={setCurrentPassword}
            autoComplete="current-password"
          />
        </div>
        <div className="grid gap-2">
          <Label htmlFor="pwd-new">{t("settings.newPassword")}</Label>
          <PasswordInput
            id="pwd-new"
            value={newPassword}
            onChange={setNewPassword}
            autoComplete="new-password"
          />
          <p className="text-xs text-muted-foreground">{t("settings.passwordHint")}</p>
        </div>
        <div className="grid gap-2">
          <Label htmlFor="pwd-confirm">{t("settings.confirmPassword")}</Label>
          <PasswordInput
            id="pwd-confirm"
            value={confirmPassword}
            onChange={setConfirmPassword}
            autoComplete="new-password"
          />
        </div>
      </CardContent>
      <CardFooter className="px-4 sm:px-6">
        <Button className="h-10" onClick={submitPassword} disabled={!canSubmit}>
          {changeMutation.isPending ? t("settings.passwordChanging") : t("settings.passwordSubmit")}
        </Button>
      </CardFooter>
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
    <Card className="gap-4 py-4 sm:py-6 lg:col-span-2">
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

// Carte Expiration des vouchers (F1/F5) — politique de nettoyage des expirés.
// Le moteur d'expiration du cloud (Tick) applique la politique automatiquement.
function ExpiryCard({ settings }: { settings: AppSettings }) {
  const { t } = useI18n();
  const queryClient = useQueryClient();
  const [mode, setMode] = useState<ExpiryPolicyMode>(
    settings.tenant.expiryPolicyMode === "remove" ? "remove" : "keep",
  );
  const [days, setDays] = useState(String(settings.tenant.expiryPolicyAfterDays ?? 30));

  const daysNum = parseInt(days, 10);
  const daysValid = Number.isInteger(daysNum) && daysNum >= 1 && daysNum <= 365;

  const saveMutation = useMutation({
    mutationFn: () => {
      const afterDays = mode === "remove" && daysValid ? daysNum : undefined;
      return api<AppSettings>("/api/settings", {
        method: "PUT",
        // Corps défensif : champs plats (forme du handler actuel) + forme imbriquée
        // « tenant » du contrat — le décodeur Go ignore les champs inconnus.
        body: {
          expiryPolicyMode: mode,
          expiryPolicyAfterDays: afterDays,
          tenant: { expiryPolicyMode: mode, expiryPolicyAfterDays: afterDays },
        },
      });
    },
    onSuccess: () => {
      toast.success(t("settings.expirySavedToast"));
      void queryClient.invalidateQueries({ queryKey: SETTINGS_QUERY_KEY });
    },
    onError: (error: Error) => toast.error(error.message),
  });

  return (
    <Card className="gap-4 py-4 sm:py-6">
      <CardHeader className="px-4 sm:px-6">
        <CardTitle className="flex items-center gap-2 text-base">
          <span className="flex size-8 items-center justify-center rounded-lg bg-primary/15 text-primary">
            <CalendarClock className="size-4" />
          </span>
          {t("settings.expiryCard")}
        </CardTitle>
        <CardDescription>{t("settings.expiryCardDesc")}</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-4 px-4 sm:px-6">
        <RadioGroup
          value={mode}
          onValueChange={(value) => setMode(value as ExpiryPolicyMode)}
          className="grid gap-3"
        >
          <label className="flex min-h-14 cursor-pointer items-start gap-3 rounded-lg border p-3 transition-colors has-[[data-state=checked]]:border-primary/50">
            <RadioGroupItem value="keep" className="mt-1" />
            <span className="text-sm">
              {t("settings.expiryKeep")}
              <span className="block text-xs font-normal text-muted-foreground">
                {t("settings.expiryKeepDesc")}
              </span>
            </span>
          </label>
          <label className="flex min-h-14 cursor-pointer items-start gap-3 rounded-lg border p-3 transition-colors has-[[data-state=checked]]:border-primary/50">
            <RadioGroupItem value="remove" className="mt-1" />
            <span className="text-sm">
              {t("settings.expiryRemove")}
              <span className="block text-xs font-normal text-muted-foreground">
                {t("settings.expiryRemoveDesc")}
              </span>
            </span>
          </label>
        </RadioGroup>

        {mode === "remove" && (
          <div className="grid gap-2">
            <Label htmlFor="expiry-days">{t("settings.expiryDays")}</Label>
            <Input
              id="expiry-days"
              type="number"
              min={1}
              max={365}
              value={days}
              onChange={(event) => setDays(event.target.value)}
              className="h-10"
              aria-invalid={!daysValid}
            />
            <p className={daysValid ? "text-xs text-muted-foreground" : "text-xs text-destructive"}>
              {daysValid ? t("settings.expiryDaysHint") : t("settings.expiryDaysInvalid")}
            </p>
          </div>
        )}
      </CardContent>
      <CardFooter className="justify-end px-4 sm:px-6">
        <Button
          className="h-10"
          onClick={() => saveMutation.mutate()}
          disabled={saveMutation.isPending || (mode === "remove" && !daysValid)}
        >
          {saveMutation.isPending ? t("common.saving") : t("common.save")}
        </Button>
      </CardFooter>
    </Card>
  );
}

// Carte Vouchers (F2) — nom DNS du hotspot + logo affichés sur les tickets
// (variables {{dnsName}} et {{logo}} des modèles).
function VoucherCard({ settings }: { settings: AppSettings }) {
  const { t } = useI18n();
  const queryClient = useQueryClient();
  const [dnsName, setDnsName] = useState(settings.tenant.dnsName ?? "");
  const [logoUrl, setLogoUrl] = useState(settings.tenant.logoUrl ?? "");
  const fileInputRef = useRef<HTMLInputElement>(null);

  // Logo : image ≤ 300 Ko encodée en data URL (contrat F2).
  function handleLogoFile(event: React.ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0];
    // Permet de re-sélectionner le même fichier après une erreur.
    event.target.value = "";
    if (!file) return;
    if (!file.type.startsWith("image/")) {
      toast.error(t("settings.logoNotImage"));
      return;
    }
    if (file.size > 300 * 1024) {
      toast.error(t("settings.logoTooBig"));
      return;
    }
    const reader = new FileReader();
    reader.onload = () => {
      if (typeof reader.result === "string") setLogoUrl(reader.result);
    };
    reader.onerror = () => toast.error(t("settings.logoReadError"));
    reader.readAsDataURL(file);
  }

  const saveMutation = useMutation({
    mutationFn: () =>
      api<AppSettings>("/api/settings", {
        method: "PUT",
        // Corps défensif : champs plats + forme imbriquée « tenant » (cf. ExpiryCard).
        body: {
          dnsName: dnsName.trim(),
          logoUrl,
          tenant: { dnsName: dnsName.trim(), logoUrl },
        },
      }),
    onSuccess: () => {
      toast.success(t("settings.voucherSavedToast"));
      void queryClient.invalidateQueries({ queryKey: SETTINGS_QUERY_KEY });
    },
    onError: (error: Error) => toast.error(error.message),
  });

  return (
    <Card className="gap-4 py-4 sm:py-6 lg:col-span-2">
      <CardHeader className="px-4 sm:px-6">
        <CardTitle className="flex items-center gap-2 text-base">
          <span className="flex size-8 items-center justify-center rounded-lg bg-primary/15 text-primary">
            <Ticket className="size-4" />
          </span>
          {t("settings.voucherCard")}
        </CardTitle>
        <CardDescription>{t("settings.voucherCardDesc")}</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-4 px-4 sm:px-6 sm:grid-cols-2">
        <div className="grid gap-2">
          <Label htmlFor="voucher-dns">{t("settings.dnsName")}</Label>
          <div className="relative">
            <Globe className="absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" aria-hidden />
            <Input
              id="voucher-dns"
              className="h-10 pl-9"
              placeholder="wifi.mondomaine.ci"
              value={dnsName}
              onChange={(event) => setDnsName(event.target.value)}
              maxLength={100}
            />
          </div>
          <p className="text-xs text-muted-foreground">
            {t("settings.dnsNameHintPre")}
            <code className="font-mono text-[11px]">{"{{dnsName}}"}</code>
            {t("settings.dnsNameHintPost")}
          </p>
        </div>

        <div className="grid gap-2">
          <Label>{t("settings.logo")}</Label>
          <div className="flex items-center gap-3">
            <Avatar className="size-14 rounded-xl border bg-white">
              {logoUrl ? (
                <AvatarImage src={logoUrl} alt={t("settings.logoAlt")} className="object-contain" />
              ) : null}
              <AvatarFallback className="rounded-xl bg-muted text-muted-foreground">
                <ImageIcon className="size-5" aria-hidden />
              </AvatarFallback>
            </Avatar>
            <div className="flex flex-wrap gap-2">
              <Button
                type="button"
                variant="outline"
                className="h-10"
                onClick={() => fileInputRef.current?.click()}
              >
                <ImagePlus className="size-4" />
                {logoUrl ? t("settings.change") : t("settings.upload")}
              </Button>
              {logoUrl && (
                <Button
                  type="button"
                  variant="ghost"
                  className="h-10 text-destructive hover:text-destructive"
                  onClick={() => setLogoUrl("")}
                >
                  <X className="size-4" />
                  {t("settings.remove")}
                </Button>
              )}
            </div>
            <input
              ref={fileInputRef}
              type="file"
              accept="image/*"
              className="sr-only"
              onChange={handleLogoFile}
              aria-label={t("settings.logoInputAria")}
            />
          </div>
          <p className="text-xs text-muted-foreground">
            {t("settings.logoHintPre")}
            <code className="font-mono text-[11px]">{"{{logo}}"}</code>
            {t("settings.logoHintPost")}
          </p>
        </div>
      </CardContent>
      <CardFooter className="justify-end px-4 sm:px-6">
        <Button
          className="h-10"
          onClick={() => saveMutation.mutate()}
          disabled={saveMutation.isPending || dnsName.trim().length > 100}
        >
          {saveMutation.isPending ? t("common.saving") : t("common.save")}
        </Button>
      </CardFooter>
    </Card>
  );
}
