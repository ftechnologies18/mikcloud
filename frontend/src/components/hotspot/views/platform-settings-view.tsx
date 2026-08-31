"use client";

// Console plateforme — PARAMÈTRES PLATEFORME (admin plateforme uniquement).
// Config globale du SaaS vivant sur les settings du compte principal :
//   • Identité — nom affiché du SaaS ;
//   • Inscriptions — bascule ouvertes/fermées + clé d'invitation optionnelle
//     (priorité : env REGISTER_KEY > config DB — filet de sécurité) ;
//   • Sécurité — changement de mot de passe (POST /api/auth/password existant) ;
//   • Langue — FR/EN (préférence locale du navigateur) ;
//   • Danger zone — purge globale des données (moteur existant, tous comptes).

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  AlertTriangle,
  KeyRound,
  Languages,
  Radio,
  ScrollText,
  ShieldAlert,
  ShieldCheck,
  Trash2,
} from "lucide-react";
import { toast } from "sonner";

import { api, fetchPlatformSettings, updatePlatformSettings } from "@/lib/hotspot/api";
import { useI18n } from "@/lib/hotspot/i18n";
import type { PlatformSettingsResponse } from "@/lib/hotspot/types";
import { PageHeader } from "@/components/hotspot/page-header";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";

const PLATFORM_SETTINGS_KEY = ["/api/admin/platform/settings"] as const;

export default function PlatformSettingsView() {
  const { t } = useI18n();
  const queryClient = useQueryClient();

  const { data, isLoading, error } = useQuery({
    queryKey: PLATFORM_SETTINGS_KEY,
    queryFn: fetchPlatformSettings,
  });

  if (isLoading) {
    return (
      <div className="space-y-4">
        <PageHeader title={t("platformSettings.title")} description={t("platformSettings.description")} />
        <div className="grid gap-4">
          <Skeleton className="h-48 w-full" />
          <Skeleton className="h-64 w-full" />
        </div>
      </div>
    );
  }

  if (error || !data) {
    return (
      <div className="space-y-4">
        <PageHeader title={t("platformSettings.title")} description={t("platformSettings.description")} />
        <Card className="border-destructive/30">
          <CardContent className="pt-6 text-sm text-muted-foreground">
            {error instanceof Error ? error.message : t("platformSettings.loadError")}
          </CardContent>
        </Card>
      </div>
    );
  }

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: PLATFORM_SETTINGS_KEY });
  };

  return (
    <div className="space-y-4">
      <PageHeader title={t("platformSettings.title")} description={t("platformSettings.description")} />
      <div className="grid gap-4 lg:grid-cols-2">
        <IdentityCard data={data} onDone={invalidate} />
        <RegisterCard data={data} onDone={invalidate} />
        <SecurityCard />
        <LanguageCard />
        <DangerCard />
      </div>
    </div>
  );
}

/* ─── Identité plateforme ─── */
function IdentityCard({ data, onDone }: { data: PlatformSettingsResponse; onDone: () => void }) {
  const { t } = useI18n();
  const [name, setName] = useState(data.platform.name);

  const mutation = useMutation({
    mutationFn: () => updatePlatformSettings({ name: name.trim() }),
    onSuccess: () => {
      toast.success(t("platformSettings.identitySaved"));
      onDone();
    },
    onError: (err: Error) => toast.error(err.message),
  });

  return (
    <Card className="gap-4 py-4 sm:py-6">
      <CardHeader className="px-4 sm:px-6">
        <CardTitle className="flex items-center gap-2 text-base">
          <span className="flex size-8 items-center justify-center rounded-lg bg-primary/15 text-primary">
            <Radio className="size-4" />
          </span>
          {t("platformSettings.identity")}
        </CardTitle>
        <CardDescription>{t("platformSettings.identityDesc")}</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-3 px-4 sm:px-6">
        <div className="space-y-2">
          <Label htmlFor="platform-name">{t("platformSettings.platformName")}</Label>
          <Input
            id="platform-name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="MikCloud"
            maxLength={60}
            disabled={mutation.isPending}
          />
          <p className="text-xs text-muted-foreground">{t("platformSettings.platformNameHint")}</p>
        </div>
      </CardContent>
      <CardFooter className="px-4 sm:px-6">
        <Button size="sm" onClick={() => mutation.mutate()} disabled={!name.trim() || name.trim() === data.platform.name || mutation.isPending}>
          {mutation.isPending ? t("common.saving") : t("common.save")}
        </Button>
      </CardFooter>
    </Card>
  );
}

/* ─── Inscriptions ─── */
function RegisterCard({ data, onDone }: { data: PlatformSettingsResponse; onDone: () => void }) {
  const { t } = useI18n();
  const [open, setOpen] = useState(data.platform.registerOpen);
  const [key, setKey] = useState("");
  const [keyMode, setKeyMode] = useState<"keep" | "set" | "remove">("keep");

  // Env REGISTER_KEY définie : elle verrouille les inscriptions et prend le
  // dessus — la console ne peut pas les rouvrir (affichage informatif).
  const envLocked = data.registerSource === "env";

  const mutation = useMutation({
    mutationFn: () => {
      const payload: Record<string, unknown> = { registerOpen: open };
      if (keyMode === "set" && key.trim()) payload.registerKey = key.trim();
      if (keyMode === "remove") payload.registerKey = "";
      return updatePlatformSettings(payload);
    },
    onSuccess: () => {
      toast.success(t("platformSettings.registerSaved"));
      setKey("");
      setKeyMode("keep");
      onDone();
    },
    onError: (err: Error) => toast.error(err.message),
  });

  const dirty = open !== data.platform.registerOpen || keyMode !== "keep" || (keyMode === "set" && key.trim() !== "");

  return (
    <Card className="gap-4 py-4 sm:py-6">
      <CardHeader className="px-4 sm:px-6">
        <CardTitle className="flex items-center gap-2 text-base">
          <span className="flex size-8 items-center justify-center rounded-lg bg-primary/15 text-primary">
            <ShieldCheck className="size-4" />
          </span>
          {t("platformSettings.register")}
        </CardTitle>
        <CardDescription>{t("platformSettings.registerDesc")}</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-4 px-4 sm:px-6">
        {envLocked && (
          <div className="flex items-start gap-3 rounded-lg border border-amber-500/40 bg-amber-500/10 px-3 py-2.5">
            <AlertTriangle className="mt-0.5 size-4 shrink-0 text-amber-600 dark:text-amber-400" />
            <p className="text-xs leading-relaxed text-amber-800/90 dark:text-amber-200/80">
              {t("platformSettings.registerEnvLocked")}
            </p>
          </div>
        )}
        <div className="flex items-center justify-between rounded-lg border p-3">
          <div className="min-w-0 pr-3">
            <p className="text-sm font-medium">{t("platformSettings.registerOpen")}</p>
            <p className="mt-0.5 text-xs text-muted-foreground">
              {t("platformSettings.registerOpenHint")}
            </p>
          </div>
          <Switch
            checked={open}
            onCheckedChange={setOpen}
            disabled={envLocked || mutation.isPending}
            aria-label={t("platformSettings.registerOpen")}
          />
        </div>

        {!open && !envLocked && (
          <div className="space-y-2">
            <Label>{t("platformSettings.inviteKey")}</Label>
            <Select value={keyMode} onValueChange={(v) => setKeyMode(v as "keep" | "set" | "remove")} disabled={mutation.isPending}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="keep">
                  {data.platform.registerKeySet
                    ? t("platformSettings.keyKeepExisting")
                    : t("platformSettings.keyKeepNone")}
                </SelectItem>
                <SelectItem value="set">{t("platformSettings.keySet")}</SelectItem>
                <SelectItem value="remove">{t("platformSettings.keyRemove")}</SelectItem>
              </SelectContent>
            </Select>
            {keyMode === "set" && (
              <Input
                value={key}
                onChange={(e) => setKey(e.target.value)}
                placeholder="MIK-XXXXX-…"
                className="font-mono"
                maxLength={64}
                disabled={mutation.isPending}
              />
            )}
            <p className="text-xs text-muted-foreground">{t("platformSettings.inviteKeyHint")}</p>
          </div>
        )}
      </CardContent>
      <CardFooter className="px-4 sm:px-6">
        <Button size="sm" onClick={() => mutation.mutate()} disabled={envLocked || !dirty || mutation.isPending}>
          {mutation.isPending ? t("common.saving") : t("common.save")}
        </Button>
      </CardFooter>
    </Card>
  );
}

/* ─── Sécurité (changement de mot de passe) ─── */
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
        <CardDescription>{t("platformSettings.securityDesc")}</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-3 px-4 sm:px-6">
        <div className="space-y-2">
          <Label htmlFor="plat-current-password">{t("settings.currentPassword")}</Label>
          <Input
            id="plat-current-password"
            type="password"
            autoComplete="current-password"
            value={currentPassword}
            onChange={(e) => setCurrentPassword(e.target.value)}
            disabled={changeMutation.isPending}
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="plat-new-password">{t("settings.newPassword")}</Label>
          <Input
            id="plat-new-password"
            type="password"
            autoComplete="new-password"
            value={newPassword}
            onChange={(e) => setNewPassword(e.target.value)}
            disabled={changeMutation.isPending}
          />
          <p className="text-xs text-muted-foreground">{t("signup.passwordHint", "Minimum 8 caractères")}</p>
        </div>
        <div className="space-y-2">
          <Label htmlFor="plat-confirm-password">{t("settings.confirmPassword")}</Label>
          <Input
            id="plat-confirm-password"
            type="password"
            autoComplete="new-password"
            value={confirmPassword}
            onChange={(e) => setConfirmPassword(e.target.value)}
            disabled={changeMutation.isPending}
          />
        </div>
      </CardContent>
      <CardFooter className="px-4 sm:px-6">
        <Button size="sm" onClick={submitPassword} disabled={!canSubmit}>
          {changeMutation.isPending ? t("common.saving") : t("settings.changePassword")}
        </Button>
      </CardFooter>
    </Card>
  );
}

/* ─── Langue (préférence locale) ─── */
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

/* ─── Danger zone (purge journal + purge globale) ─── */
function DangerCard() {
  const { t } = useI18n();

  const purgeMutation = useMutation({
    mutationFn: () =>
      api<Record<string, number>>("/api/admin/purge", {
        method: "POST",
        body: { scope: "all" },
      }),
    onSuccess: (counts) => {
      const total = Object.values(counts).reduce((a, b) => a + b, 0);
      toast.success(t("platformSettings.purgeDone", { count: total }));
    },
    onError: (error: Error) => toast.error(error.message),
  });

  // Purge ciblée du journal (scope « logs » uniquement — activité, journaux
  // utilisateurs, notifications, commandes agent). K (retour d'expérience
  // production) : les actions d'administration s'accumulent sur le compte
  // principal (audit) et polluent le journal plateforme — cet outil permet
  // de le nettoyer sans toucher aux données métier.
  const journalMutation = useMutation({
    mutationFn: () =>
      api<Record<string, number>>("/api/admin/purge", {
        method: "POST",
        body: { scopes: ["logs"] },
      }),
    onSuccess: (counts) => {
      toast.success(t("platformSettings.journalDone", { count: counts.logs ?? 0 }));
    },
    onError: (error: Error) => toast.error(error.message),
  });

  return (
    <Card className="gap-4 border-destructive/30 py-4 sm:py-6 lg:col-span-2">
      <CardHeader className="px-4 sm:px-6">
        <CardTitle className="flex items-center gap-2 text-base text-destructive">
          <span className="flex size-8 items-center justify-center rounded-lg bg-destructive/10 text-destructive">
            <ShieldAlert className="size-4" />
          </span>
          {t("platformSettings.dangerZone")}
        </CardTitle>
        <CardDescription>{t("platformSettings.dangerDesc")}</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-3 px-4 sm:px-6">
        <div className="flex flex-col gap-3 rounded-lg border border-border/60 bg-muted/30 p-3 sm:flex-row sm:items-center sm:justify-between">
          <div className="min-w-0">
            <p className="flex items-center gap-2 text-sm font-medium">
              <ScrollText className="size-4 text-muted-foreground" />
              {t("platformSettings.journalTitle")}
            </p>
            <p className="mt-0.5 text-xs text-muted-foreground">
              {t("platformSettings.journalHint")}
            </p>
          </div>
          <Button
            variant="outline"
            size="sm"
            className="shrink-0"
            onClick={() => {
              if (window.confirm(t("platformSettings.journalConfirm"))) {
                journalMutation.mutate();
              }
            }}
            disabled={journalMutation.isPending}
          >
            {journalMutation.isPending ? t("common.saving") : t("platformSettings.journalButton")}
          </Button>
        </div>

        <div className="flex flex-col gap-3 rounded-lg border border-destructive/30 bg-destructive/5 p-3 sm:flex-row sm:items-center sm:justify-between">
          <div className="min-w-0">
            <p className="flex items-center gap-2 text-sm font-medium">
              <Trash2 className="size-4 text-destructive" />
              {t("platformSettings.purgeAll")}
            </p>
            <p className="mt-0.5 text-xs text-muted-foreground">
              {t("platformSettings.purgeAllHint")}
            </p>
          </div>
          <Button
            variant="destructive"
            size="sm"
            className="shrink-0"
            onClick={() => {
              if (window.confirm(t("platformSettings.purgeConfirm"))) {
                purgeMutation.mutate();
              }
            }}
            disabled={purgeMutation.isPending}
          >
            {purgeMutation.isPending ? t("common.saving") : t("platformSettings.purgeButton")}
          </Button>
        </div>
        <p className="text-xs text-muted-foreground">
          <Badge variant="outline" className="mr-1.5 border-border bg-muted px-1.5 py-0 text-[10px] text-muted-foreground">
            {t("platformSettings.protectedBadge")}
          </Badge>
          {t("platformSettings.protectedHint")}
        </p>
      </CardContent>
    </Card>
  );
}
