"use client";

import { useMemo } from "react";
import { useIsFetching, useQuery, useQueryClient } from "@tanstack/react-query";
import { AnimatePresence, motion } from "framer-motion";
import {
  BarChart3,
  Building2,
  Gauge,
  LayoutDashboard,
  LogOut,
  Menu,
  Radio,
  RefreshCw,
  Router as RouterIcon,
  Settings,
  Store,
  Ticket,
  Users,
  Wifi,
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
import { useHotspotStore } from "@/lib/hotspot/store";
import type { HotspotSession, ViewId } from "@/lib/hotspot/types";

import AccountsView from "./views/accounts-view";
import DashboardView from "./views/dashboard-view";
import ProfilesView from "./views/profiles-view";
import ReportsView from "./views/reports-view";
import ResellersView from "./views/resellers-view";
import RoutersView from "./views/routers-view";
import SessionsView from "./views/sessions-view";
import SettingsView from "./views/settings-view";
import UsersView from "./views/users-view";
import VouchersView from "./views/vouchers-view";

const VIEW_TITLES: Record<ViewId, string> = {
  dashboard: "Tableau de bord",
  sessions: "Sessions actives",
  users: "Utilisateurs",
  vouchers: "Vouchers",
  profiles: "Profils",
  resellers: "Revendeurs",
  routers: "Routeurs",
  reports: "Rapports",
  accounts: "Comptes",
  settings: "Paramètres",
};

interface NavItem {
  id: ViewId;
  label: string;
  icon: LucideIcon;
}

const NAV_SECTIONS: { label: string; items: NavItem[] }[] = [
  {
    label: "Général",
    items: [
      { id: "dashboard", label: "Tableau de bord", icon: LayoutDashboard },
      { id: "sessions", label: "Sessions actives", icon: Radio },
    ],
  },
  {
    label: "Hotspot",
    items: [
      { id: "users", label: "Utilisateurs", icon: Users },
      { id: "vouchers", label: "Vouchers", icon: Ticket },
      { id: "profiles", label: "Profils", icon: Gauge },
    ],
  },
  { label: "Distribution", items: [{ id: "resellers", label: "Revendeurs", icon: Store }] },
  { label: "Infrastructure", items: [{ id: "routers", label: "Routeurs", icon: RouterIcon }] },
  { label: "Analyse", items: [{ id: "reports", label: "Rapports", icon: BarChart3 }] },
  {
    label: "Système",
    items: [
      // « Comptes » n'est visible que de l'admin plateforme (rôle admin) — filtré dans NavList.
      { id: "accounts", label: "Comptes", icon: Building2 },
      { id: "settings", label: "Paramètres", icon: Settings },
    ],
  },
];

const VIEWS: Record<ViewId, React.ComponentType> = {
  dashboard: DashboardView,
  sessions: SessionsView,
  users: UsersView,
  vouchers: VouchersView,
  profiles: ProfilesView,
  resellers: ResellersView,
  routers: RoutersView,
  reports: ReportsView,
  accounts: AccountsView,
  settings: SettingsView,
};

function initials(name: string): string {
  const parts = name.trim().split(/\s+/).filter(Boolean);
  if (parts.length === 0) return "SC";
  if (parts.length === 1) return parts[0].slice(0, 2).toUpperCase();
  return (parts[0][0] + parts[1][0]).toUpperCase();
}

function roleLabel(role: string): string {
  if (!role) return "Utilisateur";
  if (role.toLowerCase() === "admin" || role.toLowerCase() === "administrator") return "Administrateur";
  return role.charAt(0).toUpperCase() + role.slice(1);
}

function BrandHeader() {
  return (
    <div className="flex items-center gap-3 px-5 py-5">
      <div className="flex size-9 shrink-0 items-center justify-center rounded-xl bg-gradient-to-br from-primary to-chart-2 shadow-md shadow-primary/20">
        <Wifi className="size-5 text-white" />
      </div>
      <div className="flex min-w-0 items-center gap-2">
        <span className="truncate text-base font-semibold tracking-tight">MikCloud</span>
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

function UserCard() {
  const user = useHotspotStore((s) => s.user);
  const logout = useHotspotStore((s) => s.logout);
  const queryClient = useQueryClient();
  const name = user?.name ?? "Utilisateur";

  function handleLogout() {
    logout();
    void queryClient.clear();
  }

  return (
    <div className="flex items-center gap-3 rounded-lg border border-border/70 bg-card/60 px-3 py-3">
      <Avatar className="size-9 shrink-0">
        <AvatarFallback className="bg-primary/15 text-xs font-semibold text-primary">
          {initials(name)}
        </AvatarFallback>
      </Avatar>
      <div className="min-w-0 flex-1">
        <p className="truncate text-sm font-medium">{name}</p>
        <p className="truncate text-xs text-muted-foreground">{roleLabel(user?.role ?? "")}</p>
      </div>
      <Button
        variant="ghost"
        size="icon"
        className="size-9 shrink-0 text-muted-foreground hover:text-foreground"
        onClick={handleLogout}
        aria-label="Se déconnecter"
      >
        <LogOut className="size-4" />
      </Button>
    </div>
  );
}

function NavList() {
  const view = useHotspotStore((s) => s.view);
  const setView = useHotspotStore((s) => s.setView);
  const user = useHotspotStore((s) => s.user);
  const isAdmin = user?.role === "admin";

  const { data: sessions } = useQuery({
    queryKey: ["/api/sessions"],
    queryFn: () => api<HotspotSession[]>("/api/sessions"),
    refetchInterval: 10_000,
  });
  const sessionsCount = sessions?.length ?? 0;

  return (
    <nav className="flex-1 space-y-5 overflow-y-auto px-3 pb-4" aria-label="Navigation principale">
      {NAV_SECTIONS.map((section) => {
        const items = section.items.filter((item) => item.id !== "accounts" || isAdmin);
        if (items.length === 0) return null;
        return (
          <div key={section.label}>
            <p className="px-2.5 pb-1.5 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground/70">
              {section.label}
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
                        "relative flex min-h-11 w-full items-center gap-3 rounded-md px-2.5 py-2 text-sm font-medium transition-colors",
                        active
                          ? "bg-primary/10 text-primary"
                          : "text-muted-foreground hover:bg-accent/60 hover:text-foreground",
                      )}
                    >
                      {active && (
                        <span
                          className="absolute left-0 top-1/2 h-5 w-1 -translate-y-1/2 rounded-full bg-primary"
                          aria-hidden
                        />
                      )}
                      <span className="relative flex shrink-0 items-center">
                        <item.icon className="size-4.5" />
                        {item.id === "sessions" && sessionsCount > 0 && (
                          <span className="live-dot absolute -right-1 -top-1 block size-2 rounded-full bg-primary" aria-hidden />
                        )}
                      </span>
                      <span className="flex-1 truncate text-left">{item.label}</span>
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
  const view = useHotspotStore((s) => s.view);
  const setSidebarOpen = useHotspotStore((s) => s.setSidebarOpen);
  const user = useHotspotStore((s) => s.user);
  const logout = useHotspotStore((s) => s.logout);
  const queryClient = useQueryClient();
  const fetchingCount = useIsFetching();

  const dateLabel = useMemo(() => {
    const raw = new Intl.DateTimeFormat("fr-FR", {
      weekday: "long",
      day: "numeric",
      month: "long",
    }).format(new Date());
    return raw.charAt(0).toUpperCase() + raw.slice(1);
  }, []);

  function handleRefresh() {
    void queryClient.invalidateQueries();
    toast.success("Actualisé");
  }

  function handleLogout() {
    logout();
    void queryClient.clear();
  }

  const name = user?.name ?? "Utilisateur";

  return (
    <header className="sticky top-0 z-20 border-b bg-background/80 backdrop-blur">
      <div className="flex h-14 items-center gap-2 px-4 sm:gap-3 sm:px-6">
        <Button
          variant="ghost"
          size="icon"
          className="size-10 lg:hidden"
          onClick={() => setSidebarOpen(true)}
          aria-label="Ouvrir le menu"
        >
          <Menu className="size-5" />
        </Button>
        <h2 className="truncate text-base font-semibold tracking-tight">{VIEW_TITLES[view]}</h2>
        <div className="ml-auto flex items-center gap-1 sm:gap-3">
          <span className="hidden text-xs text-muted-foreground md:inline">{dateLabel}</span>
          <Button
            variant="ghost"
            size="icon"
            className="size-10 text-muted-foreground hover:text-foreground"
            onClick={handleRefresh}
            aria-label="Actualiser"
          >
            <RefreshCw className={cn("size-4.5", fetchingCount > 0 && "animate-spin")} />
          </Button>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="ghost" className="size-10 rounded-full p-0" aria-label="Menu utilisateur">
                <Avatar className="size-9">
                  <AvatarFallback className="bg-primary/15 text-xs font-semibold text-primary">
                    {initials(name)}
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
              <DropdownMenuItem onClick={handleLogout} className="min-h-10 text-destructive focus:text-destructive">
                <LogOut className="size-4" />
                Se déconnecter
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
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
  const isAdmin = user?.role === "admin";

  // Garde-fou : la vue Comptes est réservée à l'admin plateforme.
  const ActiveView = view === "accounts" && !isAdmin ? DashboardView : (VIEWS[view] ?? DashboardView);

  return (
    <div className="flex min-h-screen">
      {/* Sidebar desktop */}
      <aside className="fixed inset-y-0 left-0 z-30 hidden w-64 flex-col border-r bg-sidebar lg:flex">
        <BrandHeader />
        <NavList />
        <div className="px-3 pb-4">
          <UserCard />
        </div>
      </aside>

      {/* Sidebar mobile (Sheet) */}
      <Sheet open={sidebarOpen} onOpenChange={setSidebarOpen}>
        <SheetContent side="left" className="flex w-72 flex-col gap-0 bg-sidebar p-0">
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
