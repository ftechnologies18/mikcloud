"use client";

// Console plateforme — PARAMÈTRES PLATEFORME (admin plateforme uniquement).
// Fusion anti-redondance (K2) : 3 onglets, chaque préoccupation a UN seul foyer.
//   • Général     — identité (nom du SaaS) + inscriptions (bascule + clé
//                   d'invitation ; priorité env REGISTER_KEY > config DB) ;
//   • Sécurité    — mot de passe + 2FA via les cartes PARTAGÉES
//                   (parts/security-cards — une seule implémentation pour les
//                   deux vues ; l'ancien formulaire dupliqué, plus pauvre, a
//                   été retiré ; la langue vit dans le menu utilisateur) ;
//   • Maintenance — rechargement base, nettoyage démo, purge GLOBALE par
//                   catégories avec compteurs (remplace l'ancienne « zone
//                   sensible » : purge tout / purger le journal en étaient un
//                   sous-ensemble moins sûr — confirmation par saisie
//                   « PURGER » désormais partout) et purge CIBLÉE par compte.

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  AlertTriangle,
  Database,
  Eraser,
  Loader2,
  Radio,
  RotateCw,
  ShieldCheck,
  Target,
  Trash2,
  TriangleAlert,
} from "lucide-react";
import { toast } from "sonner";

import {
  api,
  fetchPlatformSettings,
  fetchPurgeAccounts,
  purgeTargetedAccount,
  updatePlatformSettings,
} from "@/lib/hotspot/api";
import { useI18n } from "@/lib/hotspot/i18n";
import type { AccountPurgeStats, PlatformSettingsResponse } from "@/lib/hotspot/types";
import { PageHeader } from "@/components/hotspot/page-header";
import { SecurityCard, TwoFactorCard } from "@/components/hotspot/parts/security-cards";
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
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";

const PLATFORM_SETTINGS_KEY = ["/api/admin/platform/settings"] as const;

// Réponse de POST /api/admin/reload — résumé de l'état réimporté.
interface ReloadStats {
  ok: boolean;
  accounts: number;
  users: number;
  hotspotUsers: number;
  routers: number;
  sessions: number;
}

// Réponse de GET /api/admin/purge/stats — compteurs par catégorie de purge.
// Les routeurs réels (realRouters) sont informatifs : JAMAIS purgés.
interface PurgeStats {
  simulatedRouters: number;
  hotspotUsers: number;
  profiles: number;
  batches: number;
  resellers: number;
  transactions: number;
  sales: number;
  sessions: number;
  logs: number;
  templates: number;
  realRouters: number;
}

interface PurgeCounts {
  routers: number;
  hotspotUsers: number;
  profiles: number;
  batches: number;
  resellers: number;
  transactions: number;
  sales: number;
  sessions: number;
  logs: number;
  templates: number;
}

interface PurgeResult {
  ok: boolean;
  summary: string;
  purged: PurgeCounts;
}

// purgeToast — bilan du toast construit côté client (i18n) à partir des
// compteurs renvoyés par l'API.
function purgeToast(res: PurgeResult, tf: (key: string, vars: Record<string, string | number>) => string): string {
  const p = res.purged;
  const parts: string[] = [];
  if (p.routers > 0) parts.push(tf("settings.purge.cat.routers", { n: p.routers }));
  if (p.hotspotUsers > 0) parts.push(tf("settings.purge.cat.hotspotUsers", { n: p.hotspotUsers }));
  if (p.profiles > 0) parts.push(tf("settings.purge.cat.profiles", { n: p.profiles }));
  if (p.batches > 0) parts.push(tf("settings.purge.cat.batches", { n: p.batches }));
  if (p.resellers > 0) parts.push(tf("settings.purge.cat.resellers", { n: p.resellers }));
  if (p.transactions > 0) parts.push(tf("settings.purge.cat.transactions", { n: p.transactions }));
  if (p.sales > 0) parts.push(tf("settings.purge.cat.sales", { n: p.sales }));
  if (p.sessions > 0) parts.push(tf("settings.purge.cat.sessions", { n: p.sessions }));
  if (p.logs > 0) parts.push(tf("settings.purge.cat.logs", { n: p.logs }));
  if (p.templates > 0) parts.push(tf("settings.purge.cat.templates", { n: p.templates }));
  if (parts.length === 0) return tf("settings.purge.toastEmpty", {});
  return tf("settings.purge.toast", { summary: parts.join(" · ") });
}

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

      {/* Fusion K2 — onglets alignés sur la vue Paramètres client : chaque
          onglet porte UNE préoccupation, plus aucune carte dupliquée. */}
      <Tabs defaultValue="general" className="gap-4 sm:gap-6">
        <TabsList className="glass-chip h-auto w-full justify-start overflow-x-auto rounded-xl p-1">
          <TabsTrigger value="general" className="gap-1.5 px-3 py-1.5 text-xs sm:px-4 sm:text-sm">
            <Radio className="size-3.5" />
            {t("platformSettings.tabGeneral")}
          </TabsTrigger>
          <TabsTrigger value="security" className="gap-1.5 px-3 py-1.5 text-xs sm:px-4 sm:text-sm">
            <ShieldCheck className="size-3.5" />
            {t("platformSettings.tabSecurity")}
          </TabsTrigger>
          <TabsTrigger value="maintenance" className="gap-1.5 px-3 py-1.5 text-xs sm:px-4 sm:text-sm">
            <Database className="size-3.5" />
            {t("platformSettings.tabMaintenance")}
          </TabsTrigger>
        </TabsList>

        {/* ── Onglet GÉNÉRAL — identité SaaS + politique d'inscription ── */}
        <TabsContent value="general">
          <div className="grid gap-4 lg:grid-cols-2">
            <IdentityCard data={data} onDone={invalidate} />
            <RegisterCard data={data} onDone={invalidate} />
          </div>
        </TabsContent>

        {/* ── Onglet SÉCURITÉ — préférences personnelles de l'admin (cartes
            partagées : seule implémentation, cf. parts/security-cards) ── */}
        <TabsContent value="security">
          <div className="grid gap-4 lg:grid-cols-2">
            <SecurityCard descKey="platformSettings.securityDesc" />
            <TwoFactorCard />
          </div>
        </TabsContent>

        {/* ── Onglet MAINTENANCE — outils GLOBAUX (délégés de la vue client
            où ils étaient redondants avec cette console) ── */}
        <TabsContent value="maintenance">
          <div className="grid gap-4 lg:grid-cols-2">
            <DatabaseCard />
            <DemoCleanupCard />
            <GlobalPurgeCard />
            <TargetedPurgeCard />
          </div>
        </TabsContent>
      </Tabs>
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

  const dirty = open !== data.platform.registerOpen || keyMode !== "keep";

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

/* ═══ Onglet MAINTENANCE — outils globaux, un seul foyer (console plateforme) ═══ */

/* ─── Rechargement base (POST /api/admin/reload) ─── */
function DatabaseCard() {
  const { t, tf } = useI18n();
  const queryClient = useQueryClient();

  const reloadMutation = useMutation({
    mutationFn: () => api<ReloadStats>("/api/admin/reload", { method: "POST" }),
    onSuccess: (stats) => {
      toast.success(
        tf("platformSettings.reloadedToast", {
          accounts: stats.accounts,
          users: stats.hotspotUsers,
          routers: stats.routers,
        }),
      );
      queryClient.invalidateQueries();
    },
    onError: (error: Error) => toast.error(error.message),
  });

  return (
    <Card className="gap-4 py-4 sm:py-6">
      <CardHeader className="px-4 sm:px-6">
        <CardTitle className="flex items-center gap-2 text-base">
          <span className="flex size-8 items-center justify-center rounded-lg bg-primary/15 text-primary">
            <Database className="size-4" />
          </span>
          {t("platformSettings.database")}
        </CardTitle>
        <CardDescription>{t("platformSettings.databaseDesc")}</CardDescription>
      </CardHeader>
      <CardFooter className="px-4 sm:px-6">
        <Button
          variant="outline"
          className="h-10"
          onClick={() => reloadMutation.mutate()}
          disabled={reloadMutation.isPending}
        >
          {reloadMutation.isPending ? t("platformSettings.reloading") : t("platformSettings.reload")}
        </Button>
      </CardFooter>
    </Card>
  );
}

/* ─── Nettoyage des données de démonstration (POST /api/admin/purge-demo) ───
// Suppression CHIRURGICALE des artefacts hérités de l'ancien seed de démo
// (BuildSeed, supprimé du code) : routeurs simulés + cascade
// (utilisateurs/tickets/lots/ventes/sessions) et revendeurs de démonstration
// « res-1 »…« res-5 » + leurs transactions. Ne touche NI les routeurs réels
// (agent), NI les profils, NI les revendeurs réels, NI les réglages.
// Confirmation simple : rien de réel n'est supprimé, l'opération est
// idempotente (re-cliquer sur une base propre ne fait rien). */
function DemoCleanupCard() {
  const { t } = useI18n();
  const queryClient = useQueryClient();
  const [open, setOpen] = useState(false);

  const cleanMutation = useMutation({
    mutationFn: () => api<PurgeResult>("/api/admin/purge-demo", { method: "POST" }),
    onSuccess: (res) => {
      toast.success(res.summary);
      setOpen(false);
      queryClient.invalidateQueries(); // toutes les vues impactées
    },
    onError: (error: Error) => toast.error(error.message),
  });

  return (
    <Card className="gap-4 py-4 sm:py-6">
      <CardHeader className="px-4 sm:px-6">
        <CardTitle className="flex items-center gap-2 text-base">
          <Eraser className="size-4 text-amber-500" />
          {t("platformSettings.purgeDemo.title")}
        </CardTitle>
        <CardDescription>{t("platformSettings.purgeDemo.desc")}</CardDescription>
      </CardHeader>
      <CardContent className="px-4 sm:px-6">
        <ul className="space-y-1.5 text-sm text-muted-foreground">
          <li className="flex items-start gap-2">
            <span className="mt-1.5 size-1.5 shrink-0 rounded-full bg-muted-foreground/60" aria-hidden />
            {t("platformSettings.purgeDemo.itemRouters")}
          </li>
          <li className="flex items-start gap-2">
            <span className="mt-1.5 size-1.5 shrink-0 rounded-full bg-muted-foreground/60" aria-hidden />
            {t("platformSettings.purgeDemo.itemResellers")}
          </li>
        </ul>
      </CardContent>
      <CardFooter className="px-4 sm:px-6">
        <Button
          variant="outline"
          className="h-10"
          onClick={() => setOpen(true)}
          disabled={cleanMutation.isPending}
        >
          <Eraser className="size-4" />
          {t("platformSettings.purgeDemo.action")}
        </Button>
      </CardFooter>

      {/* Confirmation simple — pas de saisie « PURGER » : la cible est
          strictement limitée aux artefacts de démonstration. */}
      <AlertDialog open={open} onOpenChange={setOpen}>
        <AlertDialogContent className="max-h-[85vh] overflow-y-auto">
          <AlertDialogHeader>
            <AlertDialogTitle>{t("platformSettings.purgeDemo.confirmTitle")}</AlertDialogTitle>
            <AlertDialogDescription>{t("platformSettings.purgeDemo.confirmDesc")}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={cleanMutation.isPending}>{t("common.cancel")}</AlertDialogCancel>
            <AlertDialogAction
              disabled={cleanMutation.isPending}
              onClick={(event) => {
                event.preventDefault(); // laisse la mutation fermer le dialog
                cleanMutation.mutate();
              }}
            >
              {cleanMutation.isPending ? (
                <>
                  <Loader2 className="mr-2 size-4 animate-spin" />
                  {t("platformSettings.purgeDemo.running")}
                </>
              ) : (
                t("platformSettings.purgeDemo.confirm")
              )}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </Card>
  );
}

/* ─── Purge GLOBALE par catégories (GET /api/admin/purge/stats +
    POST /api/admin/purge) — remplace l'ancienne « zone sensible » de la
    console (purge tout + purger le journal) : ces deux actions en étaient un
    sous-ensemble, avec une simple confirmation navigateur. Ici : catégories
    cochables avec compteurs live, confirmation par saisie obligatoire de
    « PURGER », bilan détaillé en toast, état vide vert quand tout est propre.
    Vider le journal = cocher la catégorie « Journaux ». */
function GlobalPurgeCard() {
  const { t, tf } = useI18n();
  const queryClient = useQueryClient();
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [purgeOpen, setPurgeOpen] = useState(false);
  const [confirmText, setConfirmText] = useState("");

  const statsQuery = useQuery({
    queryKey: ["purge-stats"],
    queryFn: () => api<PurgeStats>("/api/admin/purge/stats"),
  });
  const stats = statsQuery.data;

  const purgeMutation = useMutation({
    mutationFn: (scopes: string[]) =>
      api<PurgeResult>("/api/admin/purge", { method: "POST", body: { scopes } }),
    onSuccess: (res) => {
      toast.success(purgeToast(res, tf));
      setPurgeOpen(false);
      setConfirmText("");
      setSelected(new Set());
      queryClient.invalidateQueries(); // compteurs + toutes les vues impactées
    },
    onError: (error: Error) => toast.error(error.message),
  });

  const categories: { scope: string; count: number; label: string; hint?: string }[] = stats
    ? [
        {
          scope: "simulated_routers",
          count: stats.simulatedRouters,
          label: t("settings.purge.simulatedRouters"),
          hint: t("settings.purge.simulatedRoutersHint"),
        },
        { scope: "hotspot_users", count: stats.hotspotUsers, label: t("settings.purge.hotspotUsers") },
        { scope: "profiles", count: stats.profiles, label: t("settings.purge.profiles") },
        { scope: "batches", count: stats.batches, label: t("settings.purge.batches") },
        {
          scope: "resellers",
          count: stats.resellers,
          label: t("settings.purge.resellers"),
          hint: stats.transactions > 0 ? tf("settings.purge.resellersHint", { n: stats.transactions }) : undefined,
        },
        { scope: "sales", count: stats.sales, label: t("settings.purge.sales") },
        { scope: "sessions", count: stats.sessions, label: t("settings.purge.sessions") },
        { scope: "logs", count: stats.logs, label: t("settings.purge.logs") },
        { scope: "templates", count: stats.templates, label: t("settings.purge.templates") },
      ]
    : [];

  const totalPurgeable = categories.reduce((sum, c) => sum + c.count, 0);
  const clean = stats !== undefined && totalPurgeable === 0;

  const toggle = (scope: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(scope)) {
        next.delete(scope);
      } else {
        next.add(scope);
      }
      return next;
    });
  };

  const toggleAll = () => {
    setSelected((prev) => (prev.size === categories.length ? new Set<string>() : new Set(categories.map((c) => c.scope))));
  };

  const selectedCategories = categories.filter((c) => selected.has(c.scope));
  const canConfirm = confirmText.trim() === "PURGER" && selectedCategories.length > 0;

  return (
    <Card className="gap-4 border-destructive/30 py-4 sm:py-6 lg:col-span-2">
      <CardHeader className="px-4 sm:px-6">
        <CardTitle className="flex items-center gap-2 text-base text-destructive">
          <TriangleAlert className="size-4" />
          {t("platformSettings.purge.title")}
        </CardTitle>
        <CardDescription>{t("platformSettings.purge.desc")}</CardDescription>
      </CardHeader>
      <CardContent className="px-4 sm:px-6">
        {statsQuery.isLoading || !stats ? (
          <div className="grid gap-2 sm:grid-cols-2 xl:grid-cols-3">
            {Array.from({ length: 9 }).map((_, i) => (
              <Skeleton key={i} className="h-12 rounded-lg" />
            ))}
          </div>
        ) : clean ? (
          <div className="flex items-center gap-3 rounded-lg border border-emerald-500/30 bg-emerald-500/10 p-4 text-sm text-emerald-700 dark:text-emerald-400">
            <ShieldCheck className="size-5 shrink-0" />
            <div>
              <p className="font-medium">{t("settings.purge.empty")}</p>
              <p className="mt-0.5 text-xs opacity-80">{t("settings.purge.protected")}</p>
            </div>
          </div>
        ) : (
          <div className="space-y-3">
            <div className="grid gap-2 sm:grid-cols-2 xl:grid-cols-3">
              {categories.map((c) => (
                <Label
                  key={c.scope}
                  htmlFor={`purge-${c.scope}`}
                  className="flex cursor-pointer items-start gap-2.5 rounded-lg border p-3 transition-colors hover:bg-muted/50 has-[[data-state=checked]]:border-destructive/50 has-[[data-state=checked]]:bg-destructive/5"
                >
                  <Checkbox
                    id={`purge-${c.scope}`}
                    checked={selected.has(c.scope)}
                    onCheckedChange={() => toggle(c.scope)}
                    className="mt-0.5"
                  />
                  <span className="flex min-w-0 flex-1 items-start justify-between gap-2">
                    <span className="min-w-0">
                      <span className="block text-sm font-medium leading-tight">{c.label}</span>
                      {c.hint && <span className="mt-0.5 block text-xs leading-snug text-muted-foreground">{c.hint}</span>}
                    </span>
                    <Badge
                      variant="secondary"
                      className="shrink-0 tabular-nums"
                    >
                      {c.count}
                    </Badge>
                  </span>
                </Label>
              ))}
            </div>
            <div className="flex flex-wrap items-center justify-between gap-2">
              <Button
                type="button"
                variant="outline"
                size="sm"
                className="h-8"
                onClick={toggleAll}
                disabled={selected.size === categories.length}
              >
                {selected.size === categories.length ? t("settings.purge.clearAll") : t("settings.purge.selectAll")}
              </Button>
              <p className="text-xs text-muted-foreground">{t("settings.purge.protected")}</p>
            </div>
          </div>
        )}
      </CardContent>
      {!statsQuery.isLoading && stats && !clean && (
        <CardFooter className="px-4 sm:px-6">
          <Button
            variant="destructive"
            className="h-10"
            disabled={selectedCategories.length === 0}
            onClick={() => {
              setConfirmText("");
              setPurgeOpen(true);
            }}
          >
            <Trash2 className="size-4" />
            {t("settings.purge.purgeButton")}
            {selectedCategories.length > 0 && ` (${selectedCategories.length})`}
          </Button>
        </CardFooter>
      )}

      {/* Confirmation — récapitulatif + saisie obligatoire de « PURGER » */}
      <AlertDialog
        open={purgeOpen}
        onOpenChange={(open) => {
          setPurgeOpen(open);
          if (!open) setConfirmText("");
        }}
      >
        <AlertDialogContent className="max-h-[85vh] overflow-y-auto">
          <AlertDialogHeader>
            <AlertDialogTitle className="text-destructive">{t("settings.purge.confirmTitle")}</AlertDialogTitle>
            <AlertDialogDescription>{t("settings.purge.confirmDesc")}</AlertDialogDescription>
          </AlertDialogHeader>
          <ul className="max-h-48 space-y-1.5 overflow-y-auto rounded-lg border bg-muted/40 p-3 text-sm">
            {selectedCategories.map((c) => (
              <li
                key={c.scope}
                className="flex items-center justify-between gap-3"
              >
                <span>{c.label}</span>
                <span className="shrink-0 font-semibold tabular-nums">{c.count}</span>
              </li>
            ))}
          </ul>
          <div className="space-y-2">
            <p className="text-sm font-medium">{t("settings.purge.confirmHint")}</p>
            <Input
              value={confirmText}
              onChange={(e) => setConfirmText(e.target.value)}
              placeholder={t("settings.purge.confirmPlaceholder")}
              aria-label={t("settings.purge.confirmPlaceholder")}
              autoComplete="off"
              disabled={purgeMutation.isPending}
            />
          </div>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={purgeMutation.isPending}>{t("common.cancel")}</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-white hover:bg-destructive/90"
              disabled={!canConfirm || purgeMutation.isPending}
              onClick={(event) => {
                event.preventDefault(); // laisse la mutation fermer le dialog
                purgeMutation.mutate(selectedCategories.map((c) => c.scope));
              }}
            >
              {purgeMutation.isPending ? (
                <>
                  <Loader2 className="size-4 animate-spin" />
                  {t("settings.purge.purging")}
                </>
              ) : (
                t("settings.purge.purgeButton")
              )}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </Card>
  );
}

/* ─── Zone sensible — purge CIBLÉE par compte (éléments précis) ─── */

// Éléments purgables sur un compte : scope API ↔ compteur ↔ libellé i18n.
// Les transactions n'ont pas de scope autonome : elles partent avec les
// revendeurs (cascade) — affichées en indication sous l'élément revendeurs.
const TARGET_ELEMENTS = [
  { scope: "vouchers", stat: "vouchers", label: "platformSettings.elVouchers" },
  { scope: "hotspot_users", stat: "hotspotUsers", label: "platformSettings.elHotspotUsers" },
  { scope: "simulated_routers", stat: "simulatedRouters", label: "platformSettings.elSimulatedRouters" },
  { scope: "profiles", stat: "profiles", label: "platformSettings.elProfiles" },
  { scope: "batches", stat: "batches", label: "platformSettings.elBatches" },
  { scope: "resellers", stat: "resellers", label: "platformSettings.elResellers" },
  { scope: "sales", stat: "sales", label: "platformSettings.elSales" },
  { scope: "sessions", stat: "sessions", label: "platformSettings.elSessions" },
  { scope: "logs", stat: "logs", label: "platformSettings.elLogs" },
  { scope: "templates", stat: "templates", label: "platformSettings.elTemplates" },
] as const;

const PURGE_ACCOUNTS_KEY = ["/api/admin/purge/accounts"] as const;

function TargetedPurgeCard() {
  const { t, tf } = useI18n();
  const queryClient = useQueryClient();
  const [accountId, setAccountId] = useState("");
  const [selected, setSelected] = useState<Record<string, boolean>>({});
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [confirmText, setConfirmText] = useState("");

  const { data: accounts, isLoading, refetch, isRefetching } = useQuery({
    queryKey: PURGE_ACCOUNTS_KEY,
    queryFn: fetchPurgeAccounts,
  });

  const selectedAccount = accounts?.find((row) => row.id === accountId);
  const stats: AccountPurgeStats | undefined = selectedAccount?.stats;
  const selectedScopes = TARGET_ELEMENTS.filter((el) => selected[el.scope]).map((el) => el.scope);

  const purgeMutation = useMutation({
    mutationFn: () => purgeTargetedAccount(accountId, selectedScopes),
    onSuccess: (res) => {
      toast.success(res.summary);
      setConfirmOpen(false);
      setConfirmText("");
      setSelected({});
      void queryClient.invalidateQueries({ queryKey: PURGE_ACCOUNTS_KEY });
    },
    onError: (error: Error) => toast.error(error.message),
  });

  const pickAccount = (id: string) => {
    setAccountId(id);
    setSelected({});
    setConfirmText("");
  };

  return (
    <Card className="gap-4 border-destructive/30 py-4 sm:py-6 lg:col-span-2">
      <CardHeader className="px-4 sm:px-6">
        <CardTitle className="flex items-center gap-2 text-base text-destructive">
          <span className="flex size-8 items-center justify-center rounded-lg bg-destructive/10 text-destructive">
            <Target className="size-4" />
          </span>
          {t("platformSettings.targetedTitle")}
        </CardTitle>
        <CardDescription>{t("platformSettings.targetedDesc")}</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-4 px-4 sm:px-6">
        {/* Sélecteur de compte + rafraîchissement des compteurs */}
        <div className="grid gap-2">
          <Label htmlFor="purge-account">{t("platformSettings.targetedAccount")}</Label>
          <div className="flex gap-2">
            <Select value={accountId} onValueChange={pickAccount} disabled={isLoading}>
              <SelectTrigger id="purge-account" className="flex-1">
                <SelectValue placeholder={t("platformSettings.targetedAccountPlaceholder")} />
              </SelectTrigger>
              <SelectContent>
                {(accounts ?? []).map((row) => (
                  <SelectItem key={row.id} value={row.id}>
                    {row.name}
                    {row.owner ? ` — ${row.owner}` : ""}
                    {row.status === "disabled" ? " ⏸" : ""}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Button
              variant="outline"
              size="icon"
              className="size-9 shrink-0"
              onClick={() => void refetch()}
              disabled={isRefetching}
              aria-label={t("common.refresh")}
              title={t("common.refresh")}
            >
              <RotateCw className={`size-4 ${isRefetching ? "animate-spin" : ""}`} />
            </Button>
          </div>
          <p className="text-xs text-muted-foreground">{t("platformSettings.targetedAccountHint")}</p>
        </div>

        {isLoading && <Skeleton className="h-24 w-full" />}

        {accounts && accounts.length === 0 && (
          <p className="text-sm text-muted-foreground">{t("platformSettings.targetedNoAccounts")}</p>
        )}

        {/* Éléments purgables du compte sélectionné (compteurs live) */}
        {selectedAccount && stats && (
          <>
            <div className="flex items-center justify-between gap-2">
              <Label>{t("platformSettings.targetedElements")}</Label>
              <div className="flex gap-1">
                <Button
                  variant="ghost"
                  size="sm"
                  className="h-7 px-2 text-xs"
                  onClick={() =>
                    setSelected(Object.fromEntries(TARGET_ELEMENTS.map((el) => [el.scope, true])))
                  }
                >
                  {t("platformSettings.targetedSelectAll")}
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  className="h-7 px-2 text-xs"
                  onClick={() => setSelected({})}
                >
                  {t("platformSettings.targetedClearAll")}
                </Button>
              </div>
            </div>
            <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3" role="group" aria-label={t("platformSettings.targetedElements")}>
              {TARGET_ELEMENTS.map((el) => {
                const count = stats[el.stat as keyof AccountPurgeStats] ?? 0;
                const checked = !!selected[el.scope];
                return (
                  <label
                    key={el.scope}
                    className={`flex min-h-11 cursor-pointer items-center gap-2.5 rounded-lg border px-3 py-2 transition-colors has-[[data-state=checked]]:border-destructive/50 has-[[data-state=checked]]:bg-destructive/5 ${count === 0 ? "opacity-60" : ""}`}
                  >
                    <Checkbox
                      checked={checked}
                      onCheckedChange={(v) => setSelected((s) => ({ ...s, [el.scope]: !!v }))}
                      aria-label={t(el.label)}
                    />
                    <span className="min-w-0 flex-1 truncate text-sm">{t(el.label)}</span>
                    <Badge
                      variant="outline"
                      className={`shrink-0 px-1.5 py-0 text-[10px] tabular-nums ${count > 0 ? "border-destructive/40 text-destructive" : "border-border bg-muted text-muted-foreground"}`}
                    >
                      {count}
                    </Badge>
                  </label>
                );
              })}
            </div>
            {selected.resellers && stats.transactions > 0 && (
              <p className="text-xs text-muted-foreground">
                <AlertTriangle className="mr-1.5 inline size-3.5 align-[-2px] text-amber-600 dark:text-amber-400" />
                {tf("platformSettings.targetedResellersHint", { count: stats.transactions })}
              </p>
            )}
            {selectedScopes.length === 0 ? (
              <p className="text-xs text-muted-foreground">{t("platformSettings.targetedNone")}</p>
            ) : (
              <Button
                variant="destructive"
                size="sm"
                className="justify-self-start"
                onClick={() => {
                  setConfirmText("");
                  setConfirmOpen(true);
                }}
                disabled={purgeMutation.isPending}
              >
                <Trash2 className="mr-1.5 size-4" />
                {purgeMutation.isPending
                  ? t("common.saving")
                  : t("platformSettings.targetedButton")}
              </Button>
            )}
          </>
        )}

        <p className="text-xs text-muted-foreground">
          <Badge variant="outline" className="mr-1.5 border-border bg-muted px-1.5 py-0 text-[10px] text-muted-foreground">
            {t("platformSettings.protectedBadge")}
          </Badge>
          {t("platformSettings.protectedHint")}
        </p>
      </CardContent>

      {/* Confirmation par saisie du nom du compte (zone sensible) */}
      <AlertDialog open={confirmOpen} onOpenChange={setConfirmOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {tf("platformSettings.targetedConfirmTitle", { name: selectedAccount?.name ?? "" })}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {t("platformSettings.targetedConfirmDesc")}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <ul className="grid gap-1 rounded-lg border border-destructive/30 bg-destructive/5 p-3 text-xs">
            {TARGET_ELEMENTS.filter((el) => selected[el.scope]).map((el) => (
              <li key={el.scope} className="flex items-center justify-between gap-3">
                <span>{t(el.label)}</span>
                <Badge variant="outline" className="border-destructive/40 px-1.5 py-0 text-[10px] tabular-nums text-destructive">
                  {stats?.[el.stat as keyof AccountPurgeStats] ?? 0}
                </Badge>
              </li>
            ))}
          </ul>
          <Input
            value={confirmText}
            onChange={(e) => setConfirmText(e.target.value)}
            placeholder={t("platformSettings.targetedConfirmPlaceholder")}
            disabled={purgeMutation.isPending}
            autoComplete="off"
          />
          <AlertDialogFooter>
            <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-white hover:bg-destructive/90"
              disabled={confirmText.trim() !== selectedAccount?.name || purgeMutation.isPending}
              onClick={(event) => {
                event.preventDefault();
                purgeMutation.mutate();
              }}
            >
              {purgeMutation.isPending ? t("common.saving") : t("platformSettings.targetedButton")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </Card>
  );
}
