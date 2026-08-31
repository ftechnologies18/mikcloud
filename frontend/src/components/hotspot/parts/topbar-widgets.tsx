"use client";

// Widgets du header (topbar) :
// - SearchPalette : palette de commandes ⌘K/Ctrl+K — navigue vers n'importe
//   quelle vue autorisée + actions rapides (rafraîchir, thème, langue, logout)
// - ActivityBell  : cloche de notification — activité récente (/api/activity),
//   badge de non-lus persisté en localStorage, ouverture = tout lu
// - LiveClock     : date + heure live (seconde par seconde), style console

import { useEffect, useMemo, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useTheme } from "next-themes";
import {
  Bell,
  Inbox,
  Languages,
  LogOut,
  Moon,
  Radio,
  RefreshCw,
  Router as RouterIcon,
  Search,
  Settings,
  Store,
  Ticket,
  Users,
} from "lucide-react";
import { toast } from "sonner";
import type { LucideIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { api } from "@/lib/hotspot/api";
import { localeOf, useI18n } from "@/lib/hotspot/i18n";
import { navItemsFor } from "@/lib/hotspot/nav";
import { canView } from "@/lib/hotspot/roles";
import { useHotspotStore } from "@/lib/hotspot/store";
import type { Activity } from "@/lib/hotspot/types";

/* ─────────────────────────── LiveClock ─────────────────────────── */

export function LiveClock() {
  const { lang } = useI18n();
  const [now, setNow] = useState(() => Date.now());

  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), 1_000);
    return () => clearInterval(id);
  }, []);

  // Formatters recréés à chaque tick — coût négligeable (2×/s), et le React
  // Compiler mémoïse le composant automatiquement entre rendus identiques.
  const rawDate = new Intl.DateTimeFormat(localeOf(lang), {
    weekday: "long",
    day: "numeric",
    month: "long",
  }).format(now);
  const dateLabel = rawDate.charAt(0).toUpperCase() + rawDate.slice(1);
  const timeLabel = new Intl.DateTimeFormat(localeOf(lang), {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  }).format(now);

  return (
    <div className="hidden flex-col items-end gap-0.5 md:flex" aria-label={dateLabel}>
      <span className="max-w-44 truncate text-[11px] font-medium capitalize text-muted-foreground">
        {dateLabel}
      </span>
      <span className="flex items-center gap-1.5 text-sm font-semibold tabular-nums tracking-tight">
        <span className="live-dot block size-1.5 rounded-full bg-primary" aria-hidden />
        {timeLabel}
      </span>
    </div>
  );
}

/* ────────────────────────── SearchPalette ────────────────────────── */

export function SearchPalette() {
  const { t, lang, setLang } = useI18n();
  const { resolvedTheme, setTheme } = useTheme();
  const [open, setOpen] = useState(false);
  const setView = useHotspotStore((s) => s.setView);
  const logout = useHotspotStore((s) => s.logout);
  const user = useHotspotStore((s) => s.user);
  const queryClient = useQueryClient();
  const isAdmin = user?.role === "admin" || user?.role === "platform_admin";
  const shellMode = useHotspotStore((s) => s.shellMode);
  // La palette propose les vues de la console ACTIVE (plateforme ou client).
  const items = useMemo(
    () => navItemsFor(user?.role, isAdmin, isAdmin && shellMode === "platform" ? "platform" : "client"),
    [user?.role, isAdmin, shellMode],
  );

  // ⌘K / Ctrl+K — ouvre (ou referme) la palette depuis n'importe où.
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if ((e.key === "k" || e.key === "K") && (e.metaKey || e.ctrlKey)) {
        e.preventDefault();
        setOpen((v) => !v);
      }
    }
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, []);

  function go(id: Parameters<typeof setView>[0]) {
    setView(id);
    setOpen(false);
  }

  function handleRefresh() {
    void queryClient.invalidateQueries();
    toast.success(t("shell.refreshed"));
    setOpen(false);
  }

  function handleTheme() {
    setTheme(resolvedTheme === "dark" ? "light" : "dark");
    setOpen(false);
  }

  function handleLanguage() {
    setLang(lang === "fr" ? "en" : "fr");
    setOpen(false);
  }

  function handleLogout() {
    setOpen(false);
    logout();
    void queryClient.clear();
  }

  return (
    <>
      {/* Pilule de recherche (sm+) — centrée dans le header */}
      <button
        type="button"
        onClick={() => setOpen(true)}
        aria-label={t("topbar.search")}
        className="hidden h-9 w-52 items-center gap-2 rounded-full border border-border/70 bg-background/60 px-4 text-sm text-muted-foreground outline-none transition-all hover:border-primary/40 hover:bg-accent/60 hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring sm:flex lg:w-80"
      >
        <Search className="size-4 shrink-0" aria-hidden />
        <span className="flex-1 truncate text-left">{t("topbar.search")}</span>
        <kbd className="shrink-0 rounded border border-border/70 bg-muted px-1.5 py-0.5 font-mono text-[10px] font-semibold text-muted-foreground">
          Ctrl K
        </kbd>
      </button>

      {/* Icône seule sur mobile */}
      <Button
        variant="ghost"
        size="icon"
        className="size-10 text-muted-foreground hover:text-foreground sm:hidden"
        onClick={() => setOpen(true)}
        aria-label={t("topbar.search")}
      >
        <Search className="size-4.5" />
      </Button>

      <CommandDialog
        open={open}
        onOpenChange={setOpen}
        title={t("topbar.search")}
        description={t("topbar.searchPlaceholder")}
      >
        <CommandInput placeholder={t("topbar.searchPlaceholder")} />
        <CommandList>
          <CommandEmpty>{t("topbar.searchEmpty")}</CommandEmpty>
          <CommandGroup heading={t("topbar.searchNavigation")}>
            {items.map((item) => {
              const Icon = item.icon;
              return (
                <CommandItem
                  key={item.id}
                  value={`${t(item.labelKey)} ${item.id}`}
                  onSelect={() => go(item.id)}
                >
                  <Icon />
                  <span className="truncate">{t(item.labelKey)}</span>
                </CommandItem>
              );
            })}
          </CommandGroup>
          <CommandGroup heading={t("topbar.searchActions")}>
            <CommandItem onSelect={handleRefresh}>
              <RefreshCw />
              {t("topbar.actionRefresh")}
            </CommandItem>
            <CommandItem onSelect={handleTheme}>
              <Moon />
              {t("topbar.actionTheme")}
            </CommandItem>
            <CommandItem onSelect={handleLanguage}>
              <Languages />
              {t("topbar.actionLanguage")}
            </CommandItem>
            <CommandItem className="text-destructive" onSelect={handleLogout}>
              <LogOut />
              {t("topbar.actionLogout")}
            </CommandItem>
          </CommandGroup>
        </CommandList>
      </CommandDialog>
    </>
  );
}

/* ────────────────────────── ActivityBell ────────────────────────── */

const SEEN_KEY = "mikcloud:activity-seen";

const TYPE_ICON: Record<Activity["type"], LucideIcon> = {
  router: RouterIcon,
  user: Users,
  voucher: Ticket,
  reseller: Store,
  session: Radio,
  system: Settings,
};

/** Temps relatif compact — « il y a 5 min », « hier »… selon la langue. */
function relTime(iso: string, lang: string): string {
  const then = new Date(iso).getTime();
  if (Number.isNaN(then)) return "";
  const diffMin = Math.round((Date.now() - then) / 60_000);
  const rtf = new Intl.RelativeTimeFormat(localeOf(lang as "fr" | "en"), { numeric: "auto" });
  if (diffMin < 1) return rtf.format(0, "minute");
  if (diffMin < 60) return rtf.format(-diffMin, "minute");
  const hours = Math.floor(diffMin / 60);
  if (hours < 24) return rtf.format(-hours, "hour");
  return rtf.format(-Math.floor(hours / 24), "day");
}

export function ActivityBell() {
  const { t, lang } = useI18n();
  const setView = useHotspotStore((s) => s.setView);
  const user = useHotspotStore((s) => s.user);
  const [open, setOpen] = useState(false);
  // Dernier instant où la cloche a été ouverte — les activités plus récentes
  // sont « non lues ». Absence de valeur (première visite) = tout considéré lu.
  const [seenAt, setSeenAt] = useState<string>(() =>
    typeof window === "undefined" ? "" : (window.localStorage.getItem(SEEN_KEY) ?? ""),
  );

  // Le journal d'activité est réservé aux gérants+ (requireRole 2 côté API,
  // miroir canView("logs") côté client) — la cloche suit la même règle.
  const allowed = canView(user?.role, "logs");

  const { data } = useQuery({
    queryKey: ["/api/activity"],
    queryFn: () => api<Activity[]>("/api/activity", { params: { limit: 20 } }),
    enabled: allowed,
    refetchInterval: 60_000,
    staleTime: 30_000,
    retry: false,
  });

  const items = useMemo(() => (data ?? []).slice(0, 6), [data]);
  const unread = useMemo(() => {
    if (!seenAt) return 0;
    return (data ?? []).filter((a) => a.at > seenAt).length;
  }, [data, seenAt]);

  function handleOpenChange(next: boolean) {
    setOpen(next);
    // Ouvrir la cloche = acquitter : les activités en cours passent à « lu ».
    if (next && data?.length) {
      const latest = data[0].at; // l'API trie par date décroissante
      window.localStorage.setItem(SEEN_KEY, latest);
      setSeenAt(latest);
    }
  }

  function viewAll() {
    setOpen(false);
    setView("notifications");
  }

  if (!allowed) return null;

  return (
    <Popover open={open} onOpenChange={handleOpenChange}>
      <PopoverTrigger asChild>
        <Button
          variant="ghost"
          size="icon"
          className="relative size-10 text-muted-foreground hover:text-foreground"
          aria-label={t("topbar.bell")}
        >
          <Bell className="size-4.5" />
          {unread > 0 && (
            <span className="absolute right-1 top-1 flex h-4 min-w-4 items-center justify-center rounded-full bg-primary px-1 text-[9px] font-bold leading-none text-primary-foreground shadow-sm">
              {unread > 9 ? "9+" : unread}
            </span>
          )}
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-80 p-0">
        <div className="flex items-center justify-between border-b border-border/60 px-4 py-3">
          <p className="text-sm font-semibold">{t("topbar.bellTitle")}</p>
          {unread > 0 && (
            <span className="rounded-full bg-primary/15 px-2 py-0.5 text-[10px] font-semibold text-primary">
              {unread > 9 ? "9+" : unread}
            </span>
          )}
        </div>

        {items.length === 0 ? (
          <div className="flex flex-col items-center gap-2 px-4 py-8 text-center">
            <Inbox className="size-7 text-muted-foreground/50" aria-hidden />
            <p className="text-sm text-muted-foreground">{t("topbar.bellEmpty")}</p>
          </div>
        ) : (
          <ul className="max-h-72 divide-y divide-border/50 overflow-y-auto">
            {items.map((a) => {
              const Icon = TYPE_ICON[a.type] ?? Settings;
              return (
                <li key={a.id} className="flex gap-3 px-4 py-3">
                  <span className="flex size-7 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
                    <Icon className="size-3.5" aria-hidden />
                  </span>
                  <div className="min-w-0 flex-1">
                    <p className="line-clamp-2 text-[13px] leading-snug">{a.message}</p>
                    <p className="mt-0.5 text-[11px] text-muted-foreground">{relTime(a.at, lang)}</p>
                  </div>
                </li>
              );
            })}
          </ul>
        )}

        <div className="border-t border-border/60 p-2">
          <Button variant="ghost" size="sm" className="w-full text-muted-foreground hover:text-foreground" onClick={viewAll}>
            {t("topbar.bellViewAll")}
          </Button>
        </div>
      </PopoverContent>
    </Popover>
  );
}
