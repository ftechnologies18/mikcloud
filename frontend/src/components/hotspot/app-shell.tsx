"use client";

import { useEffect, useState } from "react";
import Image from "next/image";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { AnimatePresence, motion } from "framer-motion";
import {
  ArrowLeftRight,
  Building2,
  ChevronsUpDown,
  Languages,
  Loader2,
  LogOut,
  Menu,
  RefreshCw,
  Settings,
  ShieldCheck,
  UserRound,
} from "lucide-react";
import { toast } from "sonner";

import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { PaywallOverlay } from "@/components/hotspot/parts/paywall-overlay";
import { cn } from "@/lib/utils";
import { api, fetchAccounts, fetchBillingRequests, impersonateAccount } from "@/lib/hotspot/api";
import { useI18n } from "@/lib/hotspot/i18n";
import { NAV_PLATFORM_SECTIONS, NAV_SECTIONS } from "@/lib/hotspot/nav";
import { roleLabel, userInitials } from "@/lib/hotspot/format";
import { canView, isPlatformView } from "@/lib/hotspot/roles";
import { useHotspotStore } from "@/lib/hotspot/store";
import type { AccountSummary, HotspotSession, ViewId } from "@/lib/hotspot/types";
import { ThemeToggle } from "./theme-toggle";
import { UserProfileDialog } from "./parts/user-profile-dialog";
import { ActivityBell, LiveClock, SearchPalette } from "./parts/topbar-widgets";

import { ACCOUNTS_QUERY_KEY } from "./views/accounts-view";
import AccountsView from "./views/accounts-view";
import BillingRequestsView from "./views/billing-requests-view";
import DashboardView from "./views/dashboard-view";
import LogsView from "./views/logs-view";
import NotificationsView from "./views/notifications-view";
import PlatformLogsView from "./views/platform-logs-view";
import PlatformOverviewView from "./views/platform-overview-view";
import PlatformTeamView from "./views/platform-team-view";
import ProfilesView from "./views/profiles-view";
import ReportsView from "./views/reports-view";
import ResellersView from "./views/resellers-view";
import RoutersView from "./views/routers-view";
import SessionsView from "./views/sessions-view";
import SettingsView from "./views/settings-view";
import TeamView from "./views/team-view";
import TemplatesView from "./views/templates-view";
import UsersView from "./views/users-view";
import VouchersView from "./views/vouchers-view";

/** Libellé dynamique de la vue active — utilisé pour l'aria du conteneur
 *  principal (le titre visible vit dans chaque vue via PageHeader). */
function viewTitle(view: ViewId, t: (key: string) => string): string {
  const keys: Record<ViewId, string> = {
    dashboard: "nav.dashboard",
    sessions: "nav.sessions",
    users: "nav.users",
    vouchers: "nav.vouchers",
    templates: "templates.title",
    profiles: "nav.profiles",
    resellers: "nav.resellers",
    routers: "nav.routers",
    reports: "nav.reports",
    logs: "logs.title",
    platform: "nav.platform",
    platformLogs: "platformLogs.title",
    platformTeam: "platformTeam.title",
    billingRequests: "billingRequests.title",
    accounts: "nav.accounts",
    notifications: "nav.notifications",
    settings: "nav.settings",
    team: "nav.team",
  };
  return t(keys[view]);
}

const VIEWS: Record<ViewId, React.ComponentType> = {
  dashboard: DashboardView,
  sessions: SessionsView,
  users: UsersView,
  vouchers: VouchersView,
  templates: TemplatesView,
  profiles: ProfilesView,
  resellers: ResellersView,
  routers: RoutersView,
  reports: ReportsView,
  logs: LogsView,
  platform: PlatformOverviewView,
  platformLogs: PlatformLogsView,
  platformTeam: PlatformTeamView,
  billingRequests: BillingRequestsView,
  accounts: AccountsView,
  notifications: NotificationsView,
  settings: SettingsView,
  team: TeamView,
};

/** En-tête de marque — logo + nom MikCloud. */
function BrandHeader() {
  const { t } = useI18n();
  return (
    <div className="flex items-center gap-3 px-5 py-5">
      <Image
        src="/logo.png"
        alt={t("shell.logoAlt")}
        width={36}
        height={36}
        className="sidebar-logo size-9 shrink-0 rounded-xl shadow-md shadow-primary/20"
      />
      <div className="flex min-w-0 items-center gap-2">
        <span className="text-aurora truncate text-base font-semibold tracking-tight">MikCloud</span>
        <Badge
          variant="outline"
          className="badge-pro border-primary/25 bg-primary/10 px-1.5 py-0 text-[10px] font-semibold tracking-wide text-primary"
        >
          PRO
        </Badge>
      </div>
    </div>
  );
}

/** Item « English / Français » — bascule immédiate de la langue (F11). */
function LanguageMenuItem() {
  const { lang, setLang } = useI18n();
  return (
    <DropdownMenuItem className="min-h-10" onClick={() => setLang(lang === "fr" ? "en" : "fr")}>
      <Languages className="size-4" />
      {lang === "fr" ? "English" : "Français"}
    </DropdownMenuItem>
  );
}

function UserCard() {
  const { t, lang } = useI18n();
  const user = useHotspotStore((s) => s.user);
  const logout = useHotspotStore((s) => s.logout);
  const setView = useHotspotStore((s) => s.setView);
  const queryClient = useQueryClient();
  const [profileOpen, setProfileOpen] = useState(false);
  const name = user?.name ?? t("profile.defaultUser");

  function handleLogout() {
    logout();
    void queryClient.clear();
  }

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <button
            type="button"
            aria-label={t("shell.profileMenu")}
            className="sidebar-usercard flex w-full items-center gap-3 rounded-lg border border-border/70 bg-card/60 px-3 py-3 text-left outline-none transition-colors hover:bg-accent/60 focus-visible:ring-2 focus-visible:ring-ring data-[state=open]:bg-accent/60 data-[state=open]:text-accent-foreground"
          >
            <Avatar className="size-9 shrink-0">
              <AvatarFallback className="sidebar-avatar bg-primary/15 text-xs font-semibold text-primary">
                {userInitials(name)}
              </AvatarFallback>
            </Avatar>
            <span className="min-w-0 flex-1">
              <span className="block truncate text-sm font-medium">{name}</span>
              <span className="block truncate text-xs text-muted-foreground">
                {roleLabel(user?.role ?? "", lang)}
              </span>
            </span>
            <ChevronsUpDown className="size-4 shrink-0 text-muted-foreground" aria-hidden />
          </button>
        </DropdownMenuTrigger>
        <DropdownMenuContent side="top" align="start" className="w-60">
          <DropdownMenuLabel>
            <p className="truncate text-sm font-medium">{name}</p>
            <p className="truncate text-xs font-normal text-muted-foreground">@{user?.username ?? "—"}</p>
          </DropdownMenuLabel>
          <DropdownMenuSeparator />
          <DropdownMenuItem onClick={() => setProfileOpen(true)} className="min-h-10">
            <UserRound className="size-4" />
            {t("shell.profile")}
          </DropdownMenuItem>
          <DropdownMenuItem onClick={() => setView("settings")} className="min-h-10">
            <Settings className="size-4" />
            {t("shell.settings")}
          </DropdownMenuItem>
          <LanguageMenuItem />
          <DropdownMenuSeparator />
          <DropdownMenuItem
            onClick={handleLogout}
            className="min-h-10 text-destructive focus:text-destructive"
          >
            <LogOut className="size-4" />
            {t("shell.logout")}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      <UserProfileDialog open={profileOpen} onOpenChange={setProfileOpen} />
    </>
  );
}

/** Bascule Console plateforme ↔ Console client — visible uniquement de l'admin
 * plateforme (propriétaire du SaaS). La console plateforme = cockpit
 * d'opérateur ; « Console client » ouvre une SESSION SUPPORT dans la console
 * de n'importe quel compte client de la plateforme (assistance, configuration,
 * diagnostic) — le retour se fait en un clic depuis la session ouverte. */
function ModeSwitch() {
  const { t, tf } = useI18n();
  const queryClient = useQueryClient();
  const user = useHotspotStore((s) => s.user);
  const impersonating = useHotspotStore((s) => !!s.ownToken);
  const impersonate = useHotspotStore((s) => s.impersonate);
  const exitImpersonation = useHotspotStore((s) => s.exitImpersonation);
  const [pickerOpen, setPickerOpen] = useState(false);
  const isPlatformAdmin = user?.role === "admin" || user?.role === "platform_admin";

  const { data: accounts, isLoading: accountsLoading } = useQuery({
    queryKey: ACCOUNTS_QUERY_KEY,
    queryFn: fetchAccounts,
    enabled: isPlatformAdmin && !impersonating && pickerOpen,
    staleTime: 30_000,
  });

  if (!isPlatformAdmin) return null;

  function openConsole(account: AccountSummary) {
    impersonateAccount(account.id)
      .then((res) => {
        impersonate(res.token, res.user);
        queryClient.clear();
        toast.success(tf("shell.impersonateToast", { name: account.name }));
      })
      .catch((err: unknown) => {
        toast.error(err instanceof Error ? err.message : t("shell.impersonateError"));
      });
  }

  function backToPlatform() {
    exitImpersonation();
    queryClient.clear();
    toast.success(t("shell.exitImpersonationToast"));
  }

  // Session support en cours → retour à la console plateforme.
  if (impersonating) {
    return (
      <div className="px-3 pb-3">
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="h-10 w-full justify-start gap-2.5 border-sidebar-border bg-card/50 text-left font-medium"
          onClick={backToPlatform}
        >
          <ShieldCheck className="size-4" />
          {t("shell.exitImpersonation")}
        </Button>
      </div>
    );
  }

  // Console plateforme → sélecteur de console client (tous les comptes).
  return (
    <div className="px-3 pb-3">
      <DropdownMenu open={pickerOpen} onOpenChange={setPickerOpen}>
        <DropdownMenuTrigger asChild>
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="h-10 w-full justify-start gap-2.5 border-primary/30 bg-primary/10 text-left font-medium text-primary hover:bg-primary/15 hover:text-primary"
          >
            <ArrowLeftRight className="size-4" />
            {t("shell.goClient")}
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent side="right" align="start" className="w-64">
          <DropdownMenuLabel>{t("shell.pickAccountTitle")}</DropdownMenuLabel>
          <DropdownMenuSeparator />
          {accountsLoading ? (
            <DropdownMenuItem disabled>
              <Loader2 className="size-4 animate-spin" />
              …
            </DropdownMenuItem>
          ) : !accounts || accounts.length === 0 ? (
            <DropdownMenuItem disabled className="text-muted-foreground">
              {t("shell.pickAccountEmpty")}
            </DropdownMenuItem>
          ) : (
            accounts.map((account) => (
              <DropdownMenuItem
                key={account.id}
                className="min-h-10"
                disabled={account.status === "disabled"}
                onClick={() => {
                  setPickerOpen(false);
                  openConsole(account);
                }}
              >
                <Building2 className="size-4 shrink-0" />
                <span className="min-w-0 flex-1 truncate">{account.name}</span>
                {account.status === "disabled" && (
                  <Badge variant="outline" className="shrink-0 border-border bg-muted px-1.5 py-0 text-[10px] text-muted-foreground">
                    {t("badge.disabled")}
                  </Badge>
                )}
              </DropdownMenuItem>
            ))
          )}
          <DropdownMenuSeparator />
          <p className="px-2 pb-1.5 text-xs leading-snug text-muted-foreground">{t("shell.pickAccountHint")}</p>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  );
}

/** Bannière de session support — affichée au-dessus du contenu tant que
 * l'admin plateforme consulte la console d'un client (contexte explicite,
 * retour en un clic). */
function ImpersonationBanner() {
  const { t, tf } = useI18n();
  const queryClient = useQueryClient();
  const user = useHotspotStore((s) => s.user);
  const impersonating = useHotspotStore((s) => !!s.ownToken);
  const exitImpersonation = useHotspotStore((s) => s.exitImpersonation);
  if (!impersonating) return null;
  return (
    <div className="border-b border-primary/20 bg-primary/10 px-4 py-2 sm:px-6" role="status">
      <div className="flex min-h-9 items-center justify-between gap-3">
        <p className="flex min-w-0 items-center gap-2 text-sm font-medium text-primary">
          <ShieldCheck className="size-4 shrink-0" aria-hidden />
          <span className="truncate">{tf("shell.impersonatingAs", { name: user?.accountName ?? "—" })}</span>
          <Badge
            variant="outline"
            className="hidden shrink-0 border-primary/30 px-1.5 py-0 text-[10px] font-semibold uppercase tracking-wide text-primary sm:inline"
          >
            {t("shell.impersonatingBadge")}
          </Badge>
        </p>
        <Button
          type="button"
          size="sm"
          variant="outline"
          className="h-8 shrink-0 border-primary/30 text-primary hover:bg-primary/10 hover:text-primary"
          onClick={() => {
            exitImpersonation();
            queryClient.clear();
            toast.success(t("shell.exitImpersonationToast"));
          }}
        >
          <ShieldCheck className="size-3.5" />
          <span className="hidden sm:inline">{t("shell.exitImpersonation")}</span>
        </Button>
      </div>
    </div>
  );
}

function NavList() {
  const { t } = useI18n();
  const view = useHotspotStore((s) => s.view);
  const setView = useHotspotStore((s) => s.setView);
  const user = useHotspotStore((s) => s.user);
  const shellMode = useHotspotStore((s) => s.shellMode);
  const isAdmin = user?.role === "admin" || user?.role === "platform_admin";
  // Console plateforme : navigation dédiée (cockpit opérateur) ; sinon la
  // navigation client habituelle. « accounts » n'est rendu qu'en mode client
  // pour l'admin plateforme (en mode plateforme, il est déjà dans sa section).
  const sections =
    isAdmin && shellMode === "platform"
      ? NAV_PLATFORM_SECTIONS
      : NAV_SECTIONS.map((section) => ({
          ...section,
          items: section.items.filter((item) => !(isAdmin && shellMode === "client" && item.id === "accounts")),
        }));

  const { data: sessions } = useQuery({
    queryKey: ["/api/sessions"],
    queryFn: () => api<HotspotSession[]>("/api/sessions"),
    refetchInterval: 10_000,
    enabled: !(isAdmin && shellMode === "platform"),
  });
  const sessionsCount = sessions?.length ?? 0;
  // File de facturation : compteur de demandes EN ATTENTE (console plateforme).
  const isPlatformMode = isAdmin && shellMode === "platform";
  const { data: billing } = useQuery({
    queryKey: ["/api/admin/billing-requests"],
    queryFn: fetchBillingRequests,
    refetchInterval: 30_000,
    enabled: isPlatformMode,
  });
  const billingPending = billing?.pending ?? 0;
  return (
    <nav className="flex-1 space-y-5 overflow-y-auto px-3 pb-4" aria-label={t("nav.main")}>
      <ModeSwitch />
      {sections.map((section) => {
        // N°7 — chaque vue n'apparaît que si le rôle peut l'ouvrir
        // (miroir client des requireRole serveur ; comptes = admin plateforme).
        const items = section.items.filter(
          (item) => (item.id !== "accounts" || isAdmin) && canView(user?.role, item.id),
        );
        if (items.length === 0) return null;
        return (
          <div key={section.labelKey}>
            <p className="nav-section-label px-2.5 pb-1.5 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground/70">
              {t(section.labelKey)}
            </p>
            <ul className="space-y-0.5">
              {items.map((item) => {
                const active = view === item.id;
                return (
                  <li key={item.id}>
                    <button
                      type="button"
                      onClick={() => setView(item.id)}
                      aria-current={active ? "page" : undefined}
                      className={cn(
                        "sidebar-nav-item relative flex min-h-11 w-full items-center gap-3 rounded-lg px-2.5 py-2 text-sm font-medium transition-all duration-200",
                        active
                          ? "nav-active"
                          : "text-muted-foreground hover:bg-accent/60 hover:text-foreground",
                      )}
                    >
                      <span className="relative flex shrink-0 items-center">
                        <item.icon className="size-4.5" />
                        {item.id === "sessions" && sessionsCount > 0 && (
                          <span className="live-dot absolute -right-1 -top-1 block size-2 rounded-full bg-primary" aria-hidden />
                        )}
                        {item.id === "billingRequests" && billingPending > 0 && (
                          <span className="live-dot absolute -right-1 -top-1 block size-2 rounded-full bg-primary" aria-hidden />
                        )}
                      </span>
                      <span className="flex-1 truncate text-left">{t(item.labelKey)}</span>
                      {item.id === "billingRequests" && billingPending > 0 && (
                        <Badge
                          variant="outline"
                          className="sidebar-count border-primary/30 bg-primary/10 px-1.5 py-0 text-[10px] font-semibold tabular-nums text-primary"
                        >
                          {billingPending}
                        </Badge>
                      )}
                      {item.id === "sessions" && sessionsCount > 0 && (
                        <Badge
                          variant="outline"
                          className="sidebar-count border-border bg-muted px-1.5 py-0 text-[10px] font-semibold tabular-nums text-foreground"
                        >
                          {sessionsCount}
                        </Badge>
                      )}
                    </button>
                  </li>
                );
              })}
            </ul>
          </div>
        );
      })}
    </nav>
  );
}

function Topbar() {
  const { t } = useI18n();
  const setView = useHotspotStore((s) => s.setView);
  const setSidebarOpen = useHotspotStore((s) => s.setSidebarOpen);
  const user = useHotspotStore((s) => s.user);
  const logout = useHotspotStore((s) => s.logout);
  const queryClient = useQueryClient();
  const [profileOpen, setProfileOpen] = useState(false);

  function handleRefresh() {
    void queryClient.invalidateQueries();
    toast.success(t("shell.refreshed"));
  }

  function handleLogout() {
    logout();
    void queryClient.clear();
  }

  const name = user?.name ?? t("profile.defaultUser");

  return (
    <header className="topbar-surface sticky top-0 z-20 border-b border-border/60">
      <div className="flex h-14 items-center gap-2 px-4 sm:gap-3 sm:px-6">
        <Button
          variant="ghost"
          size="icon"
          className="size-10 shrink-0 lg:hidden"
          onClick={() => setSidebarOpen(true)}
          aria-label={t("nav.openMenu")}
        >
          <Menu className="size-5" />
        </Button>
        {/* Respiration gauche — équilibre la zone d'actions pour centrer la
            recherche. Le titre de la vue vit dans la page elle-même
            (PageHeader h1) : l'afficher aussi ici était redondant. */}
        <div className="hidden flex-1 lg:block" aria-hidden />
        {/* Recherche : accolée au burger < lg, parfaitement centrée ≥ lg */}
        <div className="flex min-w-0 justify-start lg:justify-center">
          <SearchPalette />
        </div>
        <div className="ml-auto flex flex-1 items-center justify-end gap-1 sm:gap-1.5">
          <LiveClock />
          <ActivityBell />
          <ThemeToggle />
          <Button
            variant="ghost"
            size="icon"
            className="hidden size-10 text-muted-foreground hover:text-foreground sm:inline-flex"
            onClick={handleRefresh}
            aria-label={t("common.refresh")}
          >
            <RefreshCw className="size-4.5" />
          </Button>
          {/* Profil — mobile/tablette uniquement : la sidebar mobile n'a plus
              de carte utilisateur, ce menu est le seul accès au profil.
              Sur desktop, le profil vit dans la sidebar → simple bouton
              déconnexion à la place (ci-dessous). */}
          <div className="lg:hidden">
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button variant="ghost" className="size-10 rounded-full p-0" aria-label={t("shell.userMenu")}>
                  <Avatar className="size-9">
                    <AvatarFallback className="bg-primary/15 text-xs font-semibold text-primary">
                      {userInitials(name)}
                    </AvatarFallback>
                  </Avatar>
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end" className="w-56">
                <DropdownMenuLabel>
                  <p className="truncate text-sm font-medium">{name}</p>
                  <p className="truncate text-xs font-normal text-muted-foreground">
                    @{user?.username ?? "—"}
                  </p>
                </DropdownMenuLabel>
                <DropdownMenuSeparator />
                <DropdownMenuItem onClick={() => setProfileOpen(true)} className="min-h-10">
                  <UserRound className="size-4" />
                  {t("shell.profile")}
                </DropdownMenuItem>
                <DropdownMenuItem onClick={() => setView("settings")} className="min-h-10">
                  <Settings className="size-4" />
                  {t("shell.settings")}
                </DropdownMenuItem>
                <LanguageMenuItem />
                <DropdownMenuSeparator />
                <DropdownMenuItem onClick={handleLogout} className="min-h-10 text-destructive focus:text-destructive">
                  <LogOut className="size-4" />
                  {t("shell.logout")}
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
          {/* Desktop : déconnexion directe — le menu profil complet reste
              dans la carte utilisateur de la sidebar. */}
          <Button
            variant="ghost"
            size="icon"
            className="hidden size-10 text-muted-foreground hover:bg-destructive/10 hover:text-destructive lg:inline-flex"
            onClick={handleLogout}
            aria-label={t("shell.logout")}
          >
            <LogOut className="size-4.5" />
          </Button>
          <UserProfileDialog open={profileOpen} onOpenChange={setProfileOpen} />
        </div>
      </div>
    </header>
  );
}

export default function AppShell() {
  const { t } = useI18n();
  const view = useHotspotStore((s) => s.view);
  const sidebarOpen = useHotspotStore((s) => s.sidebarOpen);
  const setSidebarOpen = useHotspotStore((s) => s.setSidebarOpen);
  const setView = useHotspotStore((s) => s.setView);
  const user = useHotspotStore((s) => s.user);
  const shellMode = useHotspotStore((s) => s.shellMode);
  const isPlatformAdmin = user?.role === "admin" || user?.role === "platform_admin";
  const platformMode = isPlatformAdmin && shellMode === "platform";

  // Cohérence mode ↔ vue : en mode plateforme, une vue client résiduelle
  // (rechargement, palette, lien) retombe sur la vue d'ensemble plateforme —
  // et inversement en mode client (les vues plateforme y sont interdites).
  useEffect(() => {
    if (platformMode && !isPlatformView(view)) {
      setView("platform");
    } else if (isPlatformAdmin && !platformMode && isPlatformView(view)) {
      setView("dashboard");
    }
  }, [platformMode, isPlatformAdmin, view, setView]);

  // Garde-fou N°7 : une vue interdite au rôle (p.ex. un lien direct restant
  // après un changement de rôle) retombe sur le dashboard — le serveur
  // refuserait les appels de toute façon (403).
  const ActiveView = canView(user?.role, view)
    ? (VIEWS[view] ?? DashboardView)
    : platformMode
      ? PlatformOverviewView
      : DashboardView;

  return (
    <div className="flex min-h-screen">
      {/* PaywallOverlay (P5) — mur total si compte suspendu (PeriodEnd + 30j).
          S'affiche par-dessus toute la console, non refermable. */}
      <PaywallOverlay />

      {/* Sidebar desktop — colonne de marque Aurora */}
      <aside className="sidebar-aurora fixed inset-y-0 left-0 z-30 hidden w-64 flex-col lg:flex">
        <BrandHeader />
        <NavList />
        <div className="px-3 pb-4">
          <UserCard />
        </div>
      </aside>

      {/* Sidebar mobile (Sheet) — pas de carte utilisateur : le profil
          reste accessible via l'avatar du header sur mobile. */}
      <Sheet open={sidebarOpen} onOpenChange={setSidebarOpen}>
        <SheetContent side="left" className="sidebar-aurora flex w-72 flex-col gap-0 p-0">
          <SheetHeader className="border-b border-sidebar-border pb-0">
            <SheetTitle className="sr-only">MikCloud</SheetTitle>
            <BrandHeader />
          </SheetHeader>
          <NavList />
        </SheetContent>
      </Sheet>

      {/* Contenu principal */}
      <div className="flex min-w-0 flex-1 flex-col lg:pl-64">
        <Topbar />
        <ImpersonationBanner />
        <main className="flex-1" aria-label={viewTitle(view, t)}>
          <div className="mx-auto max-w-7xl space-y-6 p-4 sm:p-6">
            <AnimatePresence mode="wait">
              <motion.div
                key={view}
                initial={{ opacity: 0, y: 8 }}
                animate={{ opacity: 1, y: 0 }}
                exit={{ opacity: 0, y: -8 }}
                transition={{ duration: 0.2, ease: "easeOut" }}
              >
                <ActiveView />
              </motion.div>
            </AnimatePresence>
          </div>
        </main>
      </div>
    </div>
  );
}
