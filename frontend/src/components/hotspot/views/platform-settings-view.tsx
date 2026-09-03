"use client";

// Console plateforme — PARAMÈTRES PLATEFORME (admin plateforme uniquement).
// Fusion anti-redondance (K2) : 3 onglets, chaque préoccupation a UN seul foyer.
//   • Général     — identité (nom du SaaS) + inscriptions (bascule + clé
//                   d'invitation ; priorité env REGISTER_KEY > config DB) ;
//   • Sécurité    — mot de passe + 2FA via les cartes PARTAGÉES
//                   (parts/security-cards — une seule implémentation pour les
//                   deux vues ; l'ancien formulaire dupliqué, plus pauvre, a
//                   été retiré ; la langue vit dans le menu utilisateur) ;
//   • Maintenance — rechargement base, nettoyage démo, et purge des données
//                   FUSIONNÉE : l'ancienne « Purge globale des données » et la
//                   « Purge ciblée par compte » étaient deux cartes jumelles
//                   (même grille de catégories, même confirmation, deux moteurs
//                   backend) — elles sont regroupées en UNE carte avec un
//                   sélecteur de PORTÉE (tous les comptes / un compte précis),
//                   une grille de 10 catégories unique et un endpoint unique
//                   (POST /api/admin/purge, accountId optionnel).

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
  Trash2,
  TriangleAlert,
} from "lucide-react";
import { toast } from "sonner";

import {
  api,
  fetchPlatformSettings,
  fetchPurgeAccounts,
  purgeData,
  updatePlatformSettings,
} from "@/lib/hotspot/api";
import { useI18n } from "@/lib/hotspot/i18n";
import type {
  AccountPurgeStats,
  PlatformSettingsResponse,
  PurgeAccountRow,
  PurgeResponse,
} from "@/lib/hotspot/types";
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

// Réponse de GET /api/admin/purge/stats — compteurs GLOBAUX (tous comptes)
// par catégorie de purge. Les routeurs réels (realRouters) sont informatifs :
// JAMAIS purgés. Les champs métier sont alignés sur AccountPurgeStats (grille
// unifiée de la purge fusionnée).
interface PurgeStats {
  simulatedRouters: number;
  vouchers: number;
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

// purgeToast — bilan du toast construit côté client (i18n) à partir des
// compteurs renvoyés par l'API.
function purgeToast(res: PurgeResponse, tf: (key: string, vars: Record<string, string | number>) => string): string {
  const p = res.purged ?? {};
  const routers = p.routers ?? 0;
  const vouchers = p.vouchers ?? 0;
  const hotspotUsers = p.hotspotUsers ?? 0;
  const profiles = p.profiles ?? 0;
  const batches = p.batches ?? 0;
  const resellers = p.resellers ?? 0;
  const transactions = p.transactions ?? 0;
  const sales = p.sales ?? 0;
  const sessions = p.sessions ?? 0;
  const logs = p.logs ?? 0;
  const templates = p.templates ?? 0;
  const parts: string[] = [];
  if (routers > 0) parts.push(tf("settings.purge.cat.routers", { n: routers }));
  if (vouchers > 0) parts.push(tf("settings.purge.cat.vouchers", { n: vouchers }));
  if (hotspotUsers > 0) parts.push(tf("settings.purge.cat.hotspotUsers", { n: hotspotUsers }));
  if (profiles > 0) parts.push(tf("settings.purge.cat.profiles", { n: profiles }));
  if (batches > 0) parts.push(tf("settings.purge.cat.batches", { n: batches }));
  if (resellers > 0) parts.push(tf("settings.purge.cat.resellers", { n: resellers }));
  if (transactions > 0) parts.push(tf("settings.purge.cat.transactions", { n: transactions }));
  if (sales > 0) parts.push(tf("settings.purge.cat.sales", { n: sales }));
  if (sessions > 0) parts.push(tf("settings.purge.cat.sessions", { n: sessions }));
  if (logs > 0) parts.push(tf("settings.purge.cat.logs", { n: logs }));
  if (templates > 0) parts.push(tf("settings.purge.cat.templates", { n: templates }));
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
            <DataPurgeCard />
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
    mutationFn: () => api<PurgeResponse>("/api/admin/purge-demo", { method: "POST" }),
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

/* ═══ Purge des données FUSIONNÉE (une seule carte, un seul endpoint) ═══
// L'ancienne « Purge globale des données » et la « Purge ciblée par compte »
// partageaient déjà la même grille de catégories, les mêmes garanties et le
// même style de confirmation : elles sont regroupées ici derrière un
// sélecteur de PORTÉE. Côté backend, UN SEUL moteur et UN SEUL endpoint
// (POST /api/admin/purge, accountId optionnel — l'ancien
// /api/admin/purge/account a été fusionné). Confirmation par saisie :
// « PURGER » en portée globale, nom du compte en portée ciblée. */

const PURGE_STATS_KEY = ["purge-stats"] as const;
const PURGE_ACCOUNTS_KEY = ["/api/admin/purge/accounts"] as const;

// Valeur sentinelle du sélecteur de portée : « tous les comptes »
// (Radix Select n'accepte pas value="" d'où une sentinelle explicite).
const SCOPE_ALL_ACCOUNTS = "global";

// Grille UNIFIÉE des 10 catégories purgables (identique dans les deux
// portées) : scope API ↔ compteur ↔ libellé i18n. Les transactions n'ont pas
// de scope autonome : elles partent avec les revendeurs (cascade) — affichées
// en indication sous l'élément revendeurs.
const PURGE_CATEGORIES: {
  scope: string;
  stat: keyof AccountPurgeStats;
  label: string;
  hint?: string;
}[] = [
  { scope: "simulated_routers", stat: "simulatedRouters", label: "settings.purge.simulatedRouters", hint: "settings.purge.simulatedRoutersHint" },
  { scope: "vouchers", stat: "vouchers", label: "settings.purge.vouchers", hint: "settings.purge.vouchersHint" },
  { scope: "hotspot_users", stat: "hotspotUsers", label: "settings.purge.hotspotUsers", hint: "settings.purge.hotspotUsersHint" },
  { scope: "profiles", stat: "profiles", label: "settings.purge.profiles" },
  { scope: "batches", stat: "batches", label: "settings.purge.batches" },
  { scope: "resellers", stat: "resellers", label: "settings.purge.resellers" },
  { scope: "sales", stat: "sales", label: "settings.purge.sales" },
  { scope: "sessions", stat: "sessions", label: "settings.purge.sessions" },
  { scope: "logs", stat: "logs", label: "settings.purge.logs" },
  { scope: "templates", stat: "templates", label: "settings.purge.templates" },
];

function DataPurgeCard() {
  const { t, tf } = useI18n();
  const queryClient = useQueryClient();
  const [scopeValue, setScopeValue] = useState<string>(SCOPE_ALL_ACCOUNTS);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [confirmText, setConfirmText] = useState("");

  // Compteurs des deux portées (les deux requêtes sont légères et servent
  // ensemble le sélecteur) : stats GLOBALES + lignes PAR COMPTE.
  const globalStats = useQuery({
    queryKey: PURGE_STATS_KEY,
    queryFn: () => api<PurgeStats>("/api/admin/purge/stats"),
  });
  const accountsQuery = useQuery({
    queryKey: PURGE_ACCOUNTS_KEY,
    queryFn: fetchPurgeAccounts,
  });

  const isGlobal = scopeValue === SCOPE_ALL_ACCOUNTS;
  const accounts: PurgeAccountRow[] = accountsQuery.data ?? [];
  const selectedAccount = accounts.find((row) => row.id === scopeValue);

  // Compteurs de la portée courante (globaux ou du compte sélectionné) —
  // mêmes noms de champs (grille unifiée).
  const counters: AccountPurgeStats | undefined = isGlobal
    ? globalStats.data
    : selectedAccount?.stats;
  const countFor = (stat: keyof AccountPurgeStats): number => counters?.[stat] ?? 0;

  const refresh = () => {
    void globalStats.refetch();
    void accountsQuery.refetch();
  };
  const refreshing = globalStats.isRefetching || accountsQuery.isRefetching;

  const changeScope = (value: string) => {
    setScopeValue(value);
    setSelected(new Set());
    setConfirmText("");
  };

  const purgeMutation = useMutation({
    mutationFn: (scopes: string[]) => purgeData(isGlobal ? "" : scopeValue, scopes),
    onSuccess: (res) => {
      toast.success(purgeToast(res, tf));
      setConfirmOpen(false);
      setConfirmText("");
      setSelected(new Set());
      void queryClient.invalidateQueries({ queryKey: PURGE_STATS_KEY });
      void queryClient.invalidateQueries({ queryKey: PURGE_ACCOUNTS_KEY });
    },
    onError: (error: Error) => toast.error(error.message),
  });

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
    setSelected((prev) =>
      prev.size === PURGE_CATEGORIES.length
        ? new Set<string>()
        : new Set(PURGE_CATEGORIES.map((c) => c.scope)),
    );
  };

  const selectedCategories = PURGE_CATEGORIES.filter((c) => selected.has(c.scope));
  const totalPurgeable = PURGE_CATEGORIES.reduce((sum, c) => sum + countFor(c.stat), 0);
  const clean = counters !== undefined && totalPurgeable === 0;
  const loading = globalStats.isLoading || accountsQuery.isLoading;

  // Confirmation : « PURGER » en portée globale, nom exact du compte en
  // portée ciblée (réflexe de sécurité distinct selon la portée).
  const canConfirm = isGlobal
    ? confirmText.trim() === "PURGER" && selectedCategories.length > 0
    : confirmText.trim() === selectedAccount?.name && selectedCategories.length > 0;

  return (
    <Card className="gap-4 border-destructive/30 py-4 sm:py-6 lg:col-span-2">
      <CardHeader className="px-4 sm:px-6">
        <CardTitle className="flex items-center gap-2 text-base text-destructive">
          <TriangleAlert className="size-4" />
          {t("platformSettings.dataPurge.title")}
        </CardTitle>
        <CardDescription>{t("platformSettings.dataPurge.desc")}</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-4 px-4 sm:px-6">
        {/* Sélecteur de PORTÉE — globale (tous les comptes) ou compte précis */}
        <div className="grid gap-2">
          <Label htmlFor="purge-scope">{t("platformSettings.dataPurge.scope")}</Label>
          <div className="flex gap-2">
            <Select value={scopeValue} onValueChange={changeScope} disabled={loading}>
              <SelectTrigger id="purge-scope" className="flex-1">
                <SelectValue placeholder={t("platformSettings.targetedAccountPlaceholder")} />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={SCOPE_ALL_ACCOUNTS} className="font-medium">
                  {t("platformSettings.dataPurge.scopeAll")}
                </SelectItem>
                {accounts.map((row) => (
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
              onClick={refresh}
              disabled={refreshing}
              aria-label={t("common.refresh")}
              title={t("common.refresh")}
            >
              <RotateCw className={`size-4 ${refreshing ? "animate-spin" : ""}`} />
            </Button>
          </div>
          <p className="text-xs text-muted-foreground">
            {isGlobal
              ? t("platformSettings.dataPurge.scopeGlobalHint")
              : t("platformSettings.targetedAccountHint")}
          </p>
        </div>

        {loading && <Skeleton className="h-24 w-full" />}

        {/* État vide — vert quand la portée courante est déjà propre */}
        {!loading && counters && clean && (
          <div className="flex items-center gap-3 rounded-lg border border-emerald-500/30 bg-emerald-500/10 p-4 text-sm text-emerald-700 dark:text-emerald-400">
            <ShieldCheck className="size-5 shrink-0" />
            <div>
              <p className="font-medium">
                {isGlobal ? t("settings.purge.empty") : t("settings.purge.emptyAccount")}
              </p>
              <p className="mt-0.5 text-xs opacity-80">{t("settings.purge.protected")}</p>
            </div>
          </div>
        )}

        {/* Grille de catégories (compteurs live de la portée courante) */}
        {!loading && counters && !clean && (
          <>
            <div className="flex flex-wrap items-center justify-between gap-2">
              <Label>{t("platformSettings.targetedElements")}</Label>
              <div className="flex gap-1">
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  className="h-7 px-2 text-xs"
                  onClick={toggleAll}
                  disabled={selected.size === PURGE_CATEGORIES.length}
                >
                  {selected.size === PURGE_CATEGORIES.length
                    ? t("settings.purge.clearAll")
                    : t("settings.purge.selectAll")}
                </Button>
              </div>
            </div>
            <div
              className="grid gap-2 sm:grid-cols-2 xl:grid-cols-3"
              role="group"
              aria-label={t("platformSettings.targetedElements")}
            >
              {PURGE_CATEGORIES.map((c) => {
                const count = countFor(c.stat);
                return (
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
                      aria-label={t(c.label)}
                    />
                    <span className="flex min-w-0 flex-1 items-start justify-between gap-2">
                      <span className="min-w-0">
                        <span className="block text-sm font-medium leading-tight">{t(c.label)}</span>
                        {c.scope === "resellers" && countFor("transactions") > 0 && (
                          <span className="mt-0.5 block text-xs leading-snug text-muted-foreground">
                            {tf("settings.purge.resellersHint", { n: countFor("transactions") })}
                          </span>
                        )}
                        {c.hint && c.scope !== "resellers" && (
                          <span className="mt-0.5 block text-xs leading-snug text-muted-foreground">
                            {t(c.hint)}
                          </span>
                        )}
                      </span>
                      <Badge
                        variant="secondary"
                        className={`shrink-0 tabular-nums ${count > 0 ? "" : "border-border bg-muted text-muted-foreground"}`}
                      >
                        {count}
                      </Badge>
                    </span>
                  </Label>
                );
              })}
            </div>
            <p className="text-xs text-muted-foreground">
              {selectedCategories.length === 0
                ? t("platformSettings.targetedNone")
                : t("settings.purge.protected")}
            </p>
          </>
        )}
      </CardContent>

      {!loading && counters && !clean && (
        <CardFooter className="px-4 sm:px-6">
          <Button
            variant="destructive"
            className="h-10"
            disabled={selectedCategories.length === 0 || purgeMutation.isPending}
            onClick={() => {
              setConfirmText("");
              setConfirmOpen(true);
            }}
          >
            <Trash2 className="size-4" />
            {t("settings.purge.purgeButton")}
            {selectedCategories.length > 0 && ` (${selectedCategories.length})`}
          </Button>
        </CardFooter>
      )}

      {/* Confirmation — récapitulatif + saisie obligatoire (« PURGER » en
          global, nom exact du compte en ciblé) */}
      <AlertDialog
        open={confirmOpen}
        onOpenChange={(open) => {
          setConfirmOpen(open);
          if (!open) setConfirmText("");
        }}
      >
        <AlertDialogContent className="max-h-[85vh] overflow-y-auto">
          <AlertDialogHeader>
            <AlertDialogTitle className="text-destructive">
              {isGlobal
                ? t("settings.purge.confirmTitle")
                : tf("platformSettings.targetedConfirmTitle", { name: selectedAccount?.name ?? "" })}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {isGlobal
                ? t("settings.purge.confirmDesc")
                : t("platformSettings.targetedConfirmDesc")}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <ul className="max-h-48 space-y-1.5 overflow-y-auto rounded-lg border bg-muted/40 p-3 text-sm">
            {selectedCategories.map((c) => (
              <li key={c.scope} className="flex items-center justify-between gap-3">
                <span>{t(c.label)}</span>
                <span className="shrink-0 font-semibold tabular-nums">{countFor(c.stat)}</span>
              </li>
            ))}
          </ul>
          <div className="space-y-2">
            <p className="text-sm font-medium">
              {isGlobal
                ? t("settings.purge.confirmHint")
                : t("platformSettings.dataPurge.confirmHintAccount")}
            </p>
            <Input
              value={confirmText}
              onChange={(e) => setConfirmText(e.target.value)}
              placeholder={
                isGlobal
                  ? t("settings.purge.confirmPlaceholder")
                  : t("platformSettings.targetedConfirmPlaceholder")
              }
              aria-label={
                isGlobal
                  ? t("settings.purge.confirmPlaceholder")
                  : t("platformSettings.targetedConfirmPlaceholder")
              }
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
