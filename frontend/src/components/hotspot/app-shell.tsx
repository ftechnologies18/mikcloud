"use client";

import { useMemo, useState } from "react";
import Image from "next/image";
import { useIsFetching, useQuery, useQueryClient } from "@tanstack/react-query";
import { AnimatePresence, motion } from "framer-motion";
import {
  BarChart3,
  Bell,
  Building2,
  ChevronsUpDown,
  Gauge,
  Languages,
  LayoutDashboard,
  LogOut,
  Menu,
  Printer,
  Radio,
  RefreshCw,
  Router as RouterIcon,
  ScrollText,
  Settings,
  Store,
  Ticket,
  UserRound,
  Users,
  UsersRound,
} from "lucide-react";
import { toast } from "sonner";
import type { LucideIcon } from "lucide-react";

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
import { cn } from "@/lib/utils";
import { api } from "@/lib/hotspot/api";
import { localeOf, useI18n } from "@/lib/hotspot/i18n";
import { roleLabel, userInitials } from "@/lib/hotspot/format";
import { canView } from "@/lib/hotspot/roles";
import { useHotspotStore } from "@/lib/hotspot/store";
import type { HotspotSession, ViewId } from "@/lib/hotspot/types";
import { ThemeToggle } from "./theme-toggle";
import { UserProfileDialog } from "./parts/user-profile-dialog";

import AccountsView from "./views/accounts-view";
import DashboardView from "./views/dashboard-view";
import LogsView from "./views/logs-view";
import NotificationsView from "./views/notifications-view";
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

/** Titre dynamique de la vue active (topbar) — dépend de la langue. */
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
    accounts: "nav.accounts",
    notifications: "nav.notifications",
    settings: "nav.settings",
    team: "nav.team",
  };
  return t(keys[view]);
}

interface NavItem {
  id: ViewId;
  labelKey: string;
  icon: LucideIcon;
}

const NAV_SECTIONS: { labelKey: string; items: NavItem[] }[] = [
  {
    labelKey: "nav.section.general",
    items: [
      { id: "dashboard", labelKey: "nav.dashboard", icon: LayoutDashboard },
      { id: "sessions", labelKey: "nav.sessions", icon: Radio },
    ],
  },
  {
    labelKey: "nav.section.hotspot",
    items: [
      { id: "users", labelKey: "nav.users", icon: Users },
      { id: "vouchers", labelKey: "nav.vouchers", icon: Ticket },
      { id: "templates", labelKey: "nav.templates", icon: Printer },
      { id: "profiles", labelKey: "nav.profiles", icon: Gauge },
    ],
  },
  { labelKey: "nav.section.distribution", items: [{ id: "resellers", labelKey: "nav.resellers", icon: Store }] },
  { labelKey: "nav.section.infrastructure", items: [{ id: "routers", labelKey: "nav.routers", icon: RouterIcon }] },
  {
    labelKey: "nav.section.analysis",
    items: [
      { id: "reports", labelKey: "nav.reports", icon: BarChart3 },
      { id: "logs", labelKey: "nav.logs", icon: ScrollText },
    ],
  },
  {
    labelKey: "nav.section.system",
    items: [
      // « Comptes » n'est visible que de l'admin plateforme (rôle admin) — filtré dans NavList.
      { id: "accounts", labelKey: "nav.accounts", icon: Building2 },
      { id: "team", labelKey: "nav.team", icon: UsersRound },
      { id: "notifications", labelKey: "nav.notifications", icon: Bell },
      { id: "settings", labelKey: "nav.settings", icon: Settings },
    ],
  },
];

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
        className="size-9 shrink-0 rounded-xl shadow-md shadow-primary/20"
      />
      <div className="flex min-w-0 items-center gap-2">
        <span className="text-aurora truncate text-base font-semibold tracking-tight">MikCloud</span>
        <Badge
          variant="outline"
          className="border-primary/25 bg-primary/10 px-1.5 py-0 text-[10px] font-semibold tracking-wide text-primary"
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
            className="flex w-full items-center gap-3 rounded-lg border border-border/70 bg-card/60 px-3 py-3 text-left outline-none transition-colors hover:bg-accent/60 focus-visible:ring-2 focus-visible:ring-ring data-[state=open]:bg-accent/60 data-[state=open]:text-accent-foreground"
          >
            <Avatar className="size-9 shrink-0">
              <AvatarFallback className="bg-primary/15 text-xs font-semibold text-primary">
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

function NavList() {
  const { t } = useI18n();
  const view = useHotspotStore((s) => s.view);
  const setView = useHotspotStore((s) => s.setView);
  const user = useHotspotStore((s) => s.user);
  const isAdmin = user?.role === "admin" || user?.role === "platform_admin";

  const { data: sessions } = useQuery({
    queryKey: ["/api/sessions"],
    queryFn: () => api<HotspotSession[]>("/api/sessions"),
    refetchInterval: 10_000,
  });
  const sessionsCount = sessions?.length ?? 0;

  return (
    <nav className="flex-1 space-y-5 overflow-y-auto px-3 pb-4" aria-label={t("nav.main")}>
      {NAV_SECTIONS.map((section) => {
        // N°7 — chaque vue n'apparaît que si le rôle peut l'ouvrir
        // (miroir client des requireRole serveur ; comptes = admin plateforme).
        const items = section.items.filter(
          (item) => (item.id !== "accounts" || isAdmin) && canView(user?.role, item.id),
        );
        if (items.length === 0) return null;
        return (
          <div key={section.labelKey}>
            <p className="px-2.5 pb-1.5 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground/70">
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
                        "relative flex min-h-11 w-full items-center gap-3 rounded-lg px-2.5 py-2 text-sm font-medium transition-all duration-200",
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
                      </span>
                      <span className="flex-1 truncate text-left">{t(item.labelKey)}</span>
                      {item.id === "sessions" && sessionsCount > 0 && (
                        <Badge
                          variant="outline"
                          className="border-border bg-muted px-1.5 py-0 text-[10px] font-semibold tabular-nums text-foreground"
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
  const { t, lang } = useI18n();
  const view = useHotspotStore((s) => s.view);
  const setView = useHotspotStore((s) => s.setView);
  const setSidebarOpen = useHotspotStore((s) => s.setSidebarOpen);
  const user = useHotspotStore((s) => s.user);
  const logout = useHotspotStore((s) => s.logout);
  const queryClient = useQueryClient();
  const fetchingCount = useIsFetching();
  const [profileOpen, setProfileOpen] = useState(false);

  const dateLabel = useMemo(() => {
    const raw = new Intl.DateTimeFormat(localeOf(lang), {
      weekday: "long",
      day: "numeric",
      month: "long",
    }).format(new Date());
    return raw.charAt(0).toUpperCase() + raw.slice(1);
  }, [lang]);

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
    <header className="sticky top-0 z-20 border-b border-border/60 bg-background/70 backdrop-blur-xl">
      <div className="flex h-14 items-center gap-2 px-4 sm:gap-3 sm:px-6">
        <Button
          variant="ghost"
          size="icon"
          className="size-10 lg:hidden"
          onClick={() => setSidebarOpen(true)}
          aria-label={t("nav.openMenu")}
        >
          <Menu className="size-5" />
        </Button>
        <h2 className="truncate text-base font-semibold tracking-tight">{viewTitle(view, t)}</h2>
        <div className="ml-auto flex items-center gap-1 sm:gap-2">
          <span className="hidden text-xs text-muted-foreground md:inline">{dateLabel}</span>
          <ThemeToggle />
          <Button
            variant="ghost"
            size="icon"
            className="size-10 text-muted-foreground hover:text-foreground"
            onClick={handleRefresh}
            aria-label={t("common.refresh")}
          >
            <RefreshCw className={cn("size-4.5", fetchingCount > 0 && "animate-spin")} />
          </Button>
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
          <UserProfileDialog open={profileOpen} onOpenChange={setProfileOpen} />
        </div>
      </div>
    </header>
  );
}

export default function AppShell() {
  const view = useHotspotStore((s) => s.view);
  const sidebarOpen = useHotspotStore((s) => s.sidebarOpen);
  const setSidebarOpen = useHotspotStore((s) => s.setSidebarOpen);
  const user = useHotspotStore((s) => s.user);
  const isAdmin = user?.role === "admin" || user?.role === "platform_admin";

  // Garde-fou N°7 : une vue interdite au rôle (p.ex. un lien direct restant
  // après un changement de rôle) retombe sur le dashboard — le serveur
  // refuserait les appels de toute façon (403).
  const ActiveView = canView(user?.role, view) ? (VIEWS[view] ?? DashboardView) : DashboardView;

  return (
    <div className="flex min-h-screen">
      {/* Sidebar desktop — colonne de marque Aurora */}
      <aside className="sidebar-aurora fixed inset-y-0 left-0 z-30 hidden w-64 flex-col lg:flex">
        <BrandHeader />
        <NavList />
        <div className="px-3 pb-4">
          <UserCard />
        </div>
      </aside>

      {/* Sidebar mobile (Sheet) */}
      <Sheet open={sidebarOpen} onOpenChange={setSidebarOpen}>
        <SheetContent side="left" className="sidebar-aurora flex w-72 flex-col gap-0 p-0">
          <SheetHeader className="border-b border-sidebar-border pb-0">
            <SheetTitle className="sr-only">MikCloud</SheetTitle>
            <BrandHeader />
          </SheetHeader>
          <NavList />
          <div className="px-3 pb-4">
            <UserCard />
          </div>
        </SheetContent>
      </Sheet>

      {/* Contenu principal */}
      <div className="flex min-w-0 flex-1 flex-col lg:pl-64">
        <Topbar />
        <main className="flex-1">
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
