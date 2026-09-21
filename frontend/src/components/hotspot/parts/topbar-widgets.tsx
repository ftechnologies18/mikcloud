"use client";

// Widgets du header (topbar) :
// - SearchPalette : palette de commandes ⌘K/Ctrl+K — navigue vers n'importe
//   quelle vue autorisée + actions rapides (rafraîchir, thème, langue, logout)
// - ActivityBell  : cloche de notification — boîte SERVEUR (/api/bell,
//   N°151 : read-state par utilisateur, badge multi-appareils, filtre RBAC).
//   N°153 : refonte UX — les non-lus restent marqués (fond teinté + pastille
//   « nouveau ») jusqu'au bouton « Tout marquer comme lu » (patron Gmail/
//   GitHub : ouvrir = consulter, pas acquitter), animations framer-motion
//   (sonnerie à l'arrivée, badge ressort, cascade des items, acquit en fondu)
// - LiveClock     : date + heure live (seconde par seconde), style console

import { useEffect, useMemo, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useTheme } from "next-themes";
import { AnimatePresence, motion, useAnimationControls } from "framer-motion";
import {
  Bell,
  BellRing,
  Building2,
  CheckCheck,
  Languages,
  Loader2,
  LogOut,
  Megaphone,
  MonitorSmartphone,
  Moon,
  Radio,
  ReceiptText,
  RefreshCw,
  Router as RouterIcon,
  Search,
  Settings,
  Store,
  Ticket,
  Users,
  Wifi,
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
import { Skeleton } from "@/components/ui/skeleton";
import { fetchBell, markBellSeen } from "@/lib/hotspot/api";
import { localeOf, useI18n } from "@/lib/hotspot/i18n";
import { navItemsFor } from "@/lib/hotspot/nav";
import { canView } from "@/lib/hotspot/roles";
import { useHotspotStore } from "@/lib/hotspot/store";
import { cn } from "@/lib/utils";

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
  // La palette propose les vues de la console ACTIVE (plateforme, client
  // métier ou MAISON — N°100 : même usage, mêmes vues que la sidebar).
  const mode: "platform" | "client" = isAdmin && shellMode === "platform" ? "platform" : "client";
  const items = useMemo(() => {
    const list = navItemsFor(user?.role, isAdmin, mode, user?.usage);
    // Session support (console client ouverte) : « Comptes » reste un outil
    // plateforme — il n'encombre pas la palette du client consulté.
    return mode === "client" ? list.filter((item) => item.id !== "accounts") : list;
  }, [user?.role, user?.usage, isAdmin, mode]);

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
    // N°57-f — la palette reflète la sidebar (modules métier uniquement) :
    // la zone Paramètres s'ouvre depuis le menu utilisateur.
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

// N°151 — l'ancienne clé localStorage du read-state (par navigateur,
// incohérente multi-appareils et entre membres d'une équipe) : elle ne sert
// plus qu'à la MIGRATION one-shot vers le read-state serveur, puis disparaît.
const LEGACY_SEEN_KEY = "mikcloud:activity-seen";

const TYPE_ICON: Record<string, LucideIcon> = {
  router: RouterIcon,
  user: Users,
  voucher: Ticket,
  reseller: Store,
  session: Radio,
  system: Settings,
  team: Users,
  billing: ReceiptText,
  registration: Users,
  wifi: Wifi,
  device: MonitorSmartphone,
  compte: Building2,
  announcement: Megaphone,
};

// N°153 — pastille de l'icône par catégorie : palette Aurora Emerald
// (émeraude, sarcelle, ambre, orange, rose — jamais d'indigo/bleu). Les
// annonces ne s'y fient pas : elles suivent leur NIVEAU (info → primaire,
// warning → ambre, critical → destructif) via announcementChip().
const TYPE_TONE: Record<string, string> = {
  router: "bg-emerald-500/10 text-emerald-600 dark:text-emerald-400",
  user: "bg-teal-500/10 text-teal-600 dark:text-teal-400",
  voucher: "bg-amber-500/10 text-amber-600 dark:text-amber-400",
  reseller: "bg-orange-500/10 text-orange-600 dark:text-orange-400",
  session: "bg-teal-500/10 text-teal-600 dark:text-teal-400",
  system: "bg-foreground/10 text-foreground",
  team: "bg-rose-500/10 text-rose-600 dark:text-rose-400",
  billing: "bg-emerald-500/10 text-emerald-600 dark:text-emerald-400",
  registration: "bg-teal-500/10 text-teal-600 dark:text-teal-400",
  wifi: "bg-emerald-500/10 text-emerald-600 dark:text-emerald-400",
  device: "bg-orange-500/10 text-orange-600 dark:text-orange-400",
  compte: "bg-foreground/10 text-foreground",
  announcement: "bg-primary/10 text-primary",
};

// N°153 — barre de niveau sur le bord gauche des annonces (ancrage visuel
// du megaphone plateforme). N°179 — 5 niveaux, miroir du bandeau client :
// gris = info, émeraude = nouveauté, sarcelle = maintenance, ambre = action
// recommandée, rouge = incident.
const ANNOUNCEMENT_BAR: Record<string, string> = {
  info: "bg-foreground/40",
  success: "bg-emerald-500",
  maintenance: "bg-teal-500",
  warning: "bg-amber-500",
  critical: "bg-destructive",
};

function announcementChip(level?: string): string {
  // N°179 — la pastille suit le niveau (5 couleurs du bandeau) ; repli
  // neutre pour un niveau inconnu (transition backend/frontend).
  switch (level) {
    case "critical":
      return "bg-destructive/10 text-destructive";
    case "warning":
      return "bg-amber-500/10 text-amber-600 dark:text-amber-400";
    case "maintenance":
      return "bg-teal-500/10 text-teal-600 dark:text-teal-400";
    case "success":
      return "bg-emerald-500/10 text-emerald-600 dark:text-emerald-400";
    default:
      return "bg-foreground/10 text-foreground/80";
  }
}

/** Temps relatif compact — « il y a 5 min », « hier »… selon la langue.
 * N°153 : l'instant de référence est injecté (horloge vivante du panneau,
 * tick 30 s uniquement quand il est ouvert) au lieu d'un Date.now() figé au
 * rendu — les « il y a X min » avancent pendant la lecture. */
function relTime(iso: string, lang: string, nowMs?: number): string {
  const then = new Date(iso).getTime();
  if (Number.isNaN(then)) return "";
  const now = nowMs ?? Date.now();
  const diffMin = Math.round((now - then) / 60_000);
  const rtf = new Intl.RelativeTimeFormat(localeOf(lang as "fr" | "en"), { numeric: "auto" });
  if (diffMin < 1) return rtf.format(0, "minute");
  if (diffMin < 60) return rtf.format(-diffMin, "minute");
  const hours = Math.floor(diffMin / 60);
  if (hours < 24) return rtf.format(-hours, "hour");
  return rtf.format(-Math.floor(hours / 24), "day");
}

export function ActivityBell() {
  const { t, tf, lang } = useI18n();
  const setView = useHotspotStore((s) => s.setView);
  const user = useHotspotStore((s) => s.user);
  const queryClient = useQueryClient();
  const [open, setOpen] = useState(false);
  const [marking, setMarking] = useState(false);
  // Migration one-shot par session : l'ancien read-state localStorage est
  // porté au serveur la première fois que la boîte répond « première visite »
  // (seenAt vide) — l'utilisateur garde son avancement, puis la clé disparaît.
  const migratedRef = useRef(false);
  // N°153 — garde anti double-clic sur l'acquit + sonnerie de la cloche.
  const markingRef = useRef(false);
  const prevUnreadRef = useRef<number | null>(null);
  const bellControls = useAnimationControls();
  // Horloge relative du panneau : tick 30 s UNIQUEMENT panneau ouvert.
  const [nowTick, setNowTick] = useState(() => Date.now());

  // Le journal d'activité est réservé aux gérants+ (requireRole 2 côté API,
  // miroir canView côté client) — la cloche suit la même règle. N°100 — la
  // vue pivot de la cloche est « notifications » (sa destination « tout
  // voir ») : l'activité (/api/bell, ouvert aux deux usages) intéresse
  // un foyer autant qu'un établissement, et cette section de zone est
  // partagée — contrairement au Journal (user-logs, produit hotspot).
  const allowed = canView(user?.role, "notifications", user?.usage);

  // N°151 — la boîte de notifications vit côté SERVEUR : GET /api/bell
  // renvoie items (filtrés RBAC N°149 : un gérant ne voit ni billing ni
  // team) + seenAt + unread. Le badge « non lus » est calculé serveur —
  // cohérent multi-appareils et par membre de l'équipe, contrairement à
  // l'ancien localStorage par navigateur.
  const { data, isPending } = useQuery({
    queryKey: ["/api/bell"],
    queryFn: () => fetchBell(20),
    enabled: allowed,
    refetchInterval: 60_000,
    staleTime: 30_000,
    retry: false,
  });

  // Migration de l'ancien localStorage (best-effort, une tentative par
  // session) : le serveur ne connaît pas encore cet utilisateur (seenAt
  // vide) mais son navigateur si → on lui porte l'instant, il le borne à
  // maintenant et ne reculera jamais. Après quoi la clé est retirée.
  useEffect(() => {
    if (!allowed || migratedRef.current || !data || data.seenAt) return;
    const legacy =
      typeof window === "undefined" ? "" : (window.localStorage.getItem(LEGACY_SEEN_KEY) ?? "");
    migratedRef.current = true;
    if (!legacy) return;
    void markBellSeen(legacy)
      .then(() => {
        window.localStorage.removeItem(LEGACY_SEEN_KEY);
        void queryClient.invalidateQueries({ queryKey: ["/api/bell"] });
      })
      .catch(() => {
        /* best-effort : retenté à la prochaine session */
      });
  }, [allowed, data, queryClient]);

  // N°153 — la cloche SONNE quand une notification arrive pendant que le
  // panneau est fermé : petit balancement amorti (transform uniquement —
  // GPU-friendly). Pas de sonnerie au premier chargement ni pendant la
  // lecture : uniquement une HAUSSE du compteur non-lus.
  useEffect(() => {
    if (!data) return;
    const prev = prevUnreadRef.current;
    prevUnreadRef.current = data.unread;
    if (prev === null || data.unread <= prev || open) return;
    void bellControls.start(
      { rotate: [0, 14, -12, 8, -5, 3, 0] },
      { duration: 0.85, ease: "easeInOut" },
    );
  }, [data, open, bellControls]);

  // N°153 — horloge relative : les « il y a X min » avancent pendant la
  // lecture, mais le minuteur ne tourne QUE panneau ouvert (zéro coût
  // fenêtre fermée — la boîte est déjà rafraîchie par le poll 60 s).
  useEffect(() => {
    if (!open) return;
    const id = setInterval(() => setNowTick(Date.now()), 30_000);
    return () => clearInterval(id);
  }, [open]);

  const items = useMemo(() => (data?.items ?? []).slice(0, 8), [data]);
  const unread = data?.unread ?? 0;

  function handleOpenChange(next: boolean) {
    setOpen(next);
    // N°153 — ouvrir rafraîchit la boîte en tâche de fond, mais N'ACQUITTE
    // PLUS : les non-lus restent marqués (fond teinté + pastille « nouveau »)
    // jusqu'au bouton « Tout marquer comme lu » — l'ouverture est une
    // consultation, pas un acquit (patron Gmail/GitHub). L'acquit serveur
    // reste monotone et multi-appareils (N°151 inchangé).
    if (next) {
      void queryClient.invalidateQueries({ queryKey: ["/api/bell"] });
    }
  }

  // N°153 — acquit EXPLICITE : le bouton « Tout marquer comme lu » avance le
  // read-state serveur à maintenant, puis rafraîchit — les pastilles
  // « nouveau » fondent (exit AnimatePresence) et le badge disparaît en
  // ressort. Best-effort bloquant : un double-clic ne double pas l'envoi.
  async function markAllRead() {
    if (markingRef.current || unread === 0) return;
    markingRef.current = true;
    setMarking(true);
    try {
      await markBellSeen();
      await queryClient.invalidateQueries({ queryKey: ["/api/bell"] });
    } catch {
      toast.error(t("topbar.bellMarkReadError"));
    } finally {
      setMarking(false);
      markingRef.current = false;
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
          aria-label={
            unread > 0 ? tf("topbar.bellAriaUnread", { count: unread }) : t("topbar.bell")
          }
        >
          <motion.span animate={bellControls} className="flex origin-top">
            <Bell className="size-4.5" aria-hidden />
          </motion.span>
          <AnimatePresence initial={false}>
            {unread > 0 && (
              <motion.span
                key={unread}
                initial={{ scale: 0.3, opacity: 0 }}
                animate={{ scale: 1, opacity: 1 }}
                exit={{ scale: 0.3, opacity: 0 }}
                transition={{ type: "spring", stiffness: 550, damping: 26 }}
                className="absolute right-1 top-1 flex h-4 min-w-4 items-center justify-center rounded-full bg-primary px-1 text-[9px] font-bold leading-none text-primary-foreground shadow-sm"
              >
                {unread > 9 ? "9+" : unread}
              </motion.span>
            )}
          </AnimatePresence>
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-[min(22.5rem,calc(100vw-1.5rem))] p-0">
        {/* En-tête : titre + compteur non-lus */}
        <div className="flex items-center justify-between border-b border-border/60 px-4 py-3">
          <p className="text-sm font-semibold">{t("topbar.bellTitle")}</p>
          <AnimatePresence initial={false}>
            {unread > 0 && (
              <motion.span
                initial={{ scale: 0.5, opacity: 0 }}
                animate={{ scale: 1, opacity: 1 }}
                exit={{ scale: 0.5, opacity: 0 }}
                transition={{ type: "spring", stiffness: 500, damping: 28 }}
                className="rounded-full bg-primary/15 px-2 py-0.5 text-[10px] font-semibold text-primary"
              >
                {unread > 9 ? "9+" : unread}
              </motion.span>
            )}
          </AnimatePresence>
        </div>

        {isPending ? (
          // N°153 — squelette de premier chargement : la boîte s'annonce
          // chargée plutôt que de cligner (3 lignes shimmer au rythme des
          // futures rangées).
          <div className="space-y-2.5 p-3" role="status" aria-label={t("topbar.bellLoading")}>
            {[0, 1, 2, 3].map((i) => (
              <div key={i} className="flex items-start gap-3 px-1">
                <Skeleton className="size-7 shrink-0 rounded-lg" />
                <div className="min-w-0 flex-1 space-y-1.5 pt-1">
                  <Skeleton className="h-3 w-full" />
                  <Skeleton className="h-3 w-2/5" />
                </div>
              </div>
            ))}
          </div>
        ) : items.length === 0 ? (
          // N°153 — état vide enrichi : la cloche « à jour » célèbre le calme
          // au lieu d'un simple panneau muet.
          <div className="flex flex-col items-center gap-2.5 px-4 py-10 text-center">
            <span className="flex size-11 items-center justify-center rounded-full bg-primary/10 text-primary">
              <BellRing className="size-5" aria-hidden />
            </span>
            <p className="text-sm font-medium">{t("topbar.bellCaughtUp")}</p>
            <p className="max-w-56 text-xs leading-relaxed text-muted-foreground">
              {t("topbar.bellCaughtUpHint")}
            </p>
          </div>
        ) : (
          <ul className="max-h-80 divide-y divide-border/50 overflow-y-auto">
            {items.map((a, i) => {
              const Icon = TYPE_ICON[a.type] ?? Settings;
              const isAnnouncement = a.type === "announcement";
              // Non-lu = plus récent que le dernier acquit de CET utilisateur
              // (miroir exact du calcul serveur : seenAt vide = tout lu).
              const isUnread = Boolean(data?.seenAt && a.at > data.seenAt);
              return (
                <motion.li
                  key={a.id}
                  initial={{ opacity: 0, y: 10 }}
                  animate={{ opacity: 1, y: 0 }}
                  transition={{ delay: Math.min(i * 0.045, 0.25), duration: 0.22, ease: "easeOut" }}
                  className={cn(
                    "relative flex list-none gap-3 px-4 py-3 transition-colors",
                    isUnread && "bg-primary/5 dark:bg-primary/10",
                  )}
                >
                  {isAnnouncement && (
                    <span
                      aria-hidden
                      className={cn(
                        "absolute bottom-0 left-0 top-0 w-[3px]",
                        ANNOUNCEMENT_BAR[a.level ?? "info"],
                      )}
                    />
                  )}
                  <span
                    className={cn(
                      "flex size-7 shrink-0 items-center justify-center rounded-lg",
                      isAnnouncement ? announcementChip(a.level) : (TYPE_TONE[a.type] ?? TYPE_TONE.system),
                    )}
                  >
                    <Icon className="size-3.5" aria-hidden />
                  </span>
                  <div className="min-w-0 flex-1">
                    {isAnnouncement && a.title ? (
                      // N°153 — l'annonce gagne une vraie hiérarchie : titre
                      // en gras + corps en retrait (au lieu d'un « titre —
                      // corps » aplati en une ligne).
                      <>
                        <p className="line-clamp-1 text-[13px] font-semibold leading-snug">
                          {a.title}
                        </p>
                        {a.body && (
                          <p className="mt-0.5 line-clamp-2 text-xs leading-snug text-muted-foreground">
                            {a.body}
                          </p>
                        )}
                      </>
                    ) : (
                      <p
                        className={cn(
                          "line-clamp-2 text-[13px] leading-snug",
                          isUnread && "font-medium",
                        )}
                      >
                        {a.message}
                      </p>
                    )}
                    <div className="mt-1 flex items-center gap-2">
                      <time className="text-[11px] text-muted-foreground">
                        {relTime(a.at, lang, nowTick)}
                      </time>
                      <AnimatePresence>
                        {isUnread && (
                          <motion.span
                            initial={{ opacity: 0, scale: 0.5 }}
                            animate={{ opacity: 1, scale: 1 }}
                            exit={{ opacity: 0, scale: 0.4 }}
                            transition={{ duration: 0.18, ease: "easeOut" }}
                            className="flex items-center gap-1 rounded-full bg-primary/15 px-1.5 py-px text-[9px] font-semibold uppercase tracking-wide text-primary"
                          >
                            <span className="size-1 rounded-full bg-primary" aria-hidden />
                            {t("topbar.bellNew")}
                          </motion.span>
                        )}
                      </AnimatePresence>
                    </div>
                  </div>
                </motion.li>
              );
            })}
          </ul>
        )}

        {/* Pied : « Tout voir » + acquit explicite N°153 (visible seulement
            quand il reste des non-lus — il n'y a rien à acquitter sinon). */}
        <div className="flex items-center gap-1.5 border-t border-border/60 p-2">
          <Button
            variant="ghost"
            size="sm"
            className="flex-1 text-muted-foreground hover:text-foreground"
            onClick={viewAll}
          >
            {t("topbar.bellViewAll")}
          </Button>
          {unread > 0 && (
            <Button
              variant="ghost"
              size="sm"
              className="gap-1.5 text-primary hover:text-primary"
              onClick={markAllRead}
              disabled={marking}
            >
              {marking ? (
                <Loader2 className="size-3.5 animate-spin" aria-hidden />
              ) : (
                <CheckCheck className="size-3.5" aria-hidden />
              )}
              {t("topbar.bellMarkRead")}
            </Button>
          )}
        </div>
      </PopoverContent>
    </Popover>
  );
}
