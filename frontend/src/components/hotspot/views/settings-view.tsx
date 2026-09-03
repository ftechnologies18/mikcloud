"use client";

// Vue Paramètres — réorganisée en 4 onglets (J), chacun portant UNE
// préoccupation au lieu d'une grille plate de 9 cartes mélangées :
//   • Général    — organisation (nom, devise, timezone, Wave) + langue ;
//   • Abonnement — formules SaaS (Essentiel/Illimité), échéance, Wave ;
//   • Hotspot    — expiration des vouchers, personnalisation tickets
//                  (DNS + logo, F2) et guide de connexion routeur réel ;
//   • Avancé     — sécurité (mot de passe) + maintenance admin plateforme
//                  (rechargement base, purge par catégories).

import { useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Building2,
  CalendarClock,
  Database,
  Eraser,
  Eye,
  EyeOff,
  Globe,
  ImagePlus,
  Image as ImageIcon,
  Copy,
  KeyRound,
  Languages,
  Loader2,
  Router as RouterIcon,
  ShieldCheck,
  Smartphone,
  Ticket,
  Trash2,
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
import { qrWithLogoDataUrl } from "@/components/hotspot/parts/template-render";
import { AlertDialog,
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
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";

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

export default function SettingsView() {
  const { t, tf } = useI18n();
  const queryClient = useQueryClient();
  const { data, isLoading } = useSettings();
  const user = useHotspotStore((s) => s.user);
  // Administration de la base et purge : réservées à l'admin plateforme
  // (les endpoints exigent le rôle côté serveur — défense en profondeur).
  const isAdmin = user?.role === "admin";

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

      {/* J — Paramètres réorganisés en onglets : chaque onglet porte UNE
          préoccupation (identité, facturation, métier hotspot, maintenance)
          au lieu d'une grille plate de 9 cartes mélangées. */}
      <Tabs defaultValue="general" className="gap-4 sm:gap-6">
        <TabsList className="glass-chip h-auto w-full justify-start overflow-x-auto rounded-xl p-1">
          <TabsTrigger value="general" className="gap-1.5 px-3 py-1.5 text-xs sm:px-4 sm:text-sm">
            <Building2 className="size-3.5" />
            {t("settings.tabGeneral")}
          </TabsTrigger>
          <TabsTrigger value="hotspot" className="gap-1.5 px-3 py-1.5 text-xs sm:px-4 sm:text-sm">
            <Ticket className="size-3.5" />
            {t("settings.tabHotspot")}
          </TabsTrigger>
          <TabsTrigger value="advanced" className="gap-1.5 px-3 py-1.5 text-xs sm:px-4 sm:text-sm">
            <ShieldCheck className="size-3.5" />
            {t("settings.tabAdvanced")}
          </TabsTrigger>
        </TabsList>

        {/* ── Onglet GÉNÉRAL — identité métier + préférences ── */}
        <TabsContent value="general">
          <div className="grid grid-cols-1 gap-4 lg:grid-cols-2 lg:gap-6">
            <OrganizationCard settings={data} />
            <LanguageCard />
          </div>
        </TabsContent>

        {/* ── Onglet HOTSPOT — règles métier + personnalisation tickets ── */}
        <TabsContent value="hotspot">
          <div className="grid grid-cols-1 gap-4 lg:grid-cols-2 lg:gap-6">
            {/* Expiration des vouchers (F1/F5) — politique de nettoyage cloud */}
            <ExpiryCard settings={data} />

            {/* Vouchers — DNS + logo (F2) */}
            <VoucherCard settings={data} />

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
          </div>
        </TabsContent>

        {/* ── Onglet AVANCÉ — sécurité + maintenance admin ── */}
        <TabsContent value="advanced">
          <div className="grid grid-cols-1 gap-4 lg:grid-cols-2 lg:gap-6">
            {/* Sécurité — changement de mot de passe (POST /api/auth/password,
                tout utilisateur connecté) */}
            <SecurityCard />

            {/* Sécurité S4 — 2FA TOTP (pairage, activation, désactivation) */}
            <TwoFactorCard />

            {/* Base de données — admin plateforme uniquement (POST /api/admin/reload
                admin-only) */}
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

            {/* Nettoyage des données de démonstration — admin plateforme
                uniquement (endpoint admin-only) : suppression chirurgicale
                des artefacts de l'ancien seed, données réelles préservées. */}
            {isAdmin && <DemoCleanupCard />}

            {/* Purge des données — admin plateforme uniquement ; endpoint purge
                admin-only. Les routeurs réels (agent), comptes, équipe et réglages
                ne sont JAMAIS touchés, et rien n'est régénéré. */}
            {isAdmin && <PurgeCard />}
          </div>
        </TabsContent>
      </Tabs>
    </div>
  );
}

// Carte « Données de démonstration » — nettoyage CHIRURGICAL des artefacts
// hérités de l'ancien seed de démo (BuildSeed, supprimé du code) : routeurs
// simulés + cascade (utilisateurs/tickets/lots/ventes/sessions) et revendeurs
// de démonstration « res-1 »…« res-5 » + leurs transactions. Ne touche NI les
// routeurs réels (agent), NI les profils, NI les revendeurs réels, NI les
// réglages. Confirmation simple : rien de réel n'est supprimé, l'opération
// est idempotente (re-cliquer sur une base propre ne fait rien).
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
    <Card className="gap-4 py-4 sm:py-6 lg:col-span-2">
      <CardHeader className="px-4 sm:px-6">
        <CardTitle className="flex items-center gap-2 text-base">
          <Eraser className="size-4 text-amber-500" />
          {t("settings.purgeDemo.title")}
        </CardTitle>
        <CardDescription>{t("settings.purgeDemo.desc")}</CardDescription>
      </CardHeader>
      <CardContent className="px-4 sm:px-6">
        <ul className="space-y-1.5 text-sm text-muted-foreground">
          <li className="flex items-start gap-2">
            <span className="mt-1.5 size-1.5 shrink-0 rounded-full bg-muted-foreground/60" aria-hidden />
            {t("settings.purgeDemo.itemRouters")}
          </li>
          <li className="flex items-start gap-2">
            <span className="mt-1.5 size-1.5 shrink-0 rounded-full bg-muted-foreground/60" aria-hidden />
            {t("settings.purgeDemo.itemResellers")}
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
          {t("settings.purgeDemo.action")}
        </Button>
      </CardFooter>

      {/* Confirmation simple — pas de saisie « PURGER » : la cible est
          strictement limitée aux artefacts de démonstration. */}
      <AlertDialog open={open} onOpenChange={setOpen}>
        <AlertDialogContent className="max-h-[85vh] overflow-y-auto">
          <AlertDialogHeader>
            <AlertDialogTitle>{t("settings.purgeDemo.confirmTitle")}</AlertDialogTitle>
            <AlertDialogDescription>{t("settings.purgeDemo.confirmDesc")}</AlertDialogDescription>
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
                  {t("settings.purgeDemo.running")}
                </>
              ) : (
                t("settings.purgeDemo.confirm")
              )}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </Card>
  );
}

// Carte « Purge des données » — remplace l'ancienne zone sensible (boutons
// reset/wipe). L'ancien POST /api/admin/reset régénérait le jeu de démo :
// c'était la cause du retour des données de test et de la disparition des
// routeurs réels. Désormais : 9 catégories cochables avec compteurs live
// (GET /api/admin/purge/stats), confirmation avec saisie obligatoire de
// « PURGER », bilan détaillé en toast, état vide vert quand tout est propre.
function PurgeCard() {
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
          {t("settings.purge.title")}
        </CardTitle>
        <CardDescription>{t("settings.purge.desc")}</CardDescription>
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
    if (newPassword.length < 10) {
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

// Carte 2FA TOTP (sécurité S4) — statut, activation en deux temps
// (setup → saisie du code de l'authenticator → activate) puis désactivation
// exigeant le mot de passe courant. Le secret n'est affiché QUE pendant le
// pairage ; le statut vient de GET /api/auth/me (champ totpEnabled).
function TwoFactorCard() {
  const { t } = useI18n();
  const queryClient = useQueryClient();
  const meQuery = useQuery({
    queryKey: ["totp-status"],
    queryFn: () => api<{ user: { totpEnabled?: boolean } }>("/api/auth/me"),
  });
  const totpEnabled = meQuery.data?.user?.totpEnabled ?? false;

  const [pairing, setPairing] = useState(false);
  const [secret, setSecret] = useState("");
  const [otpauth, setOtpauth] = useState("");
  const [code, setCode] = useState("");
  const [disablePassword, setDisablePassword] = useState("");

  const setupMutation = useMutation({
    mutationFn: () =>
      api<{ secret: string; otpauth: string }>("/api/auth/2fa/setup", { method: "POST" }),
    onSuccess: (res) => {
      setSecret(res.secret);
      setOtpauth(res.otpauth);
      setPairing(true);
      setCode("");
    },
    onError: (error: Error) => toast.error(error.message),
  });

  const activateMutation = useMutation({
    mutationFn: () =>
      api<{ ok: boolean }>("/api/auth/2fa/activate", { method: "POST", body: { code } }),
    onSuccess: () => {
      toast.success(t("settings.totp.activatedToast"));
      setPairing(false);
      setSecret("");
      setOtpauth("");
      setCode("");
      queryClient.invalidateQueries({ queryKey: ["totp-status"] });
    },
    onError: (error: Error) => toast.error(error.message),
  });

  const disableMutation = useMutation({
    mutationFn: () =>
      api<{ ok: boolean }>("/api/auth/2fa/disable", { method: "POST", body: { password: disablePassword } }),
    onSuccess: () => {
      toast.success(t("settings.totp.disabledToast"));
      setDisablePassword("");
      queryClient.invalidateQueries({ queryKey: ["totp-status"] });
    },
    onError: (error: Error) => toast.error(error.message),
  });

  return (
    <Card className="gap-4 py-4 sm:py-6">
      <CardHeader className="px-4 sm:px-6">
        <CardTitle className="flex items-center gap-2 text-base">
          <span className="flex size-8 items-center justify-center rounded-lg bg-primary/15 text-primary">
            <Smartphone className="size-4" />
          </span>
          {t("settings.totp.title")}
          <Badge variant={totpEnabled ? "default" : "secondary"} className="ml-auto">
            {totpEnabled ? t("settings.totp.statusOn") : t("settings.totp.statusOff")}
          </Badge>
        </CardTitle>
        <CardDescription>{t("settings.totp.desc")}</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-4 px-4 sm:px-6">
        {pairing && !totpEnabled && (
          <div className="grid gap-3 rounded-lg border border-dashed p-4">
            <p className="text-sm font-medium">{t("settings.totp.pairTitle")}</p>
            <p className="text-xs text-muted-foreground">{t("settings.totp.pairHint")}</p>
            <div className="grid gap-1.5">
              <Label htmlFor="totp-secret">{t("settings.totp.secretLabel")}</Label>
              <div className="flex items-center gap-2">
                <Input id="totp-secret" readOnly value={secret} className="font-mono text-xs" />
                <Button
                  type="button"
                  variant="outline"
                  size="icon"
                  aria-label={t("settings.totp.copySecret")}
                  onClick={() => {
                    void navigator.clipboard.writeText(secret);
                    toast.success(t("settings.totp.copiedToast"));
                  }}
                >
                  <Copy className="size-4" />
                </Button>
              </div>
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="totp-code">{t("settings.totp.codeLabel")}</Label>
              <Input
                id="totp-code"
                inputMode="numeric"
                autoComplete="one-time-code"
                placeholder="000000"
                maxLength={6}
                value={code}
                onChange={(e) => setCode(e.target.value.replace(/[^0-9]/g, ""))}
                className="text-center font-mono tracking-[0.4em]"
              />
              <p className="truncate text-xs text-muted-foreground">{otpauth}</p>
            </div>
          </div>
        )}
        {!pairing && !totpEnabled && (
          <p className="text-sm text-muted-foreground">{t("settings.totp.enableHint")}</p>
        )}
        {totpEnabled && (
          <div className="grid gap-2">
            <Label htmlFor="totp-disable-pwd">{t("settings.totp.passwordLabel")}</Label>
            <PasswordInput
              id="totp-disable-pwd"
              value={disablePassword}
              onChange={setDisablePassword}
              autoComplete="current-password"
            />
          </div>
        )}
      </CardContent>
      <CardFooter className="flex flex-wrap gap-2 px-4 sm:px-6">
        {!totpEnabled && !pairing && (
          <Button className="h-10" onClick={() => setupMutation.mutate()} disabled={setupMutation.isPending}>
            {setupMutation.isPending && <Loader2 className="size-4 animate-spin" />}
            {t("settings.totp.enable")}
          </Button>
        )}
        {pairing && (
          <>
            <Button
              className="h-10"
              onClick={() => activateMutation.mutate()}
              disabled={code.length !== 6 || activateMutation.isPending}
            >
              {activateMutation.isPending && <Loader2 className="size-4 animate-spin" />}
              {t("settings.totp.confirmActivate")}
            </Button>
            <Button
              variant="outline"
              className="h-10"
              onClick={() => {
                setPairing(false);
                setSecret("");
                setCode("");
              }}
            >
              {t("settings.totp.cancel")}
            </Button>
          </>
        )}
        {totpEnabled && (
          <Button
            variant="destructive"
            className="h-10"
            onClick={() => disableMutation.mutate()}
            disabled={disablePassword === "" || disableMutation.isPending}
          >
            {disableMutation.isPending && <Loader2 className="size-4 animate-spin" />}
            {t("settings.totp.disable")}
          </Button>
        )}
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

  // Aperçu live « logo au centre du QR » — même fonction de composition que
  // les tickets réels ; se régénère dès que le logo importé change.
  const [qrPreview, setQrPreview] = useState("");
  useEffect(() => {
    let cancelled = false;
    qrWithLogoDataUrl("MIKCLOUD\nDEMO-2026", logoUrl || undefined)
      .then((url) => {
        if (!cancelled) setQrPreview(url);
      })
      .catch(() => {
        if (!cancelled) setQrPreview("");
      });
    return () => {
      cancelled = true;
    };
  }, [logoUrl]);

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
    <Card className="gap-4 py-4 sm:py-6">
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
          <p className="text-xs font-medium">{t("settings.qrPreviewTitle")}</p>
          <div className="flex items-center gap-3 rounded-lg border bg-white p-3">
            {qrPreview ? (
              <img
                src={qrPreview}
                alt={t("settings.qrPreviewAlt")}
                className="size-20 shrink-0"
              />
            ) : (
              <Skeleton className="size-20 shrink-0" />
            )}
            <p className="text-xs text-muted-foreground">{t("settings.qrPreviewHint")}</p>
          </div>
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
