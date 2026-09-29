"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import dynamic from "next/dynamic";
import Image from "next/image";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Check,
  ChevronDown,
  ChevronsUpDown,
  Languages,
  Loader2,
  LogOut,
  Menu,
  MousePointer2,
  PanelLeft,
  PanelLeftClose,
  PanelLeftOpen,
  RefreshCw,
  Settings,
  ShieldCheck,
  UserRound,
} from "lucide-react";
import type { LucideIcon } from "lucide-react";
import { toast } from "sonner";

import { FtciCredit } from "@/components/ftci-credit";
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
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { PaywallOverlay } from "@/components/hotspot/parts/paywall-overlay";
import { cn } from "@/lib/utils";
import { api, fetchBillingRequests } from "@/lib/hotspot/api";
import { useI18n } from "@/lib/hotspot/i18n";
import { NAV_HOMENET_SECTIONS, NAV_PLATFORM_SECTIONS, NAV_SECTIONS } from "@/lib/hotspot/nav";
import { roleLabel, userInitials } from "@/lib/hotspot/format";
import { canView, isPlatformView, usageOf } from "@/lib/hotspot/roles";
// N°130 — préchargement au survol de la navigation (chunks + requêtes).
import { prefetchView } from "@/lib/hotspot/prefetch";
import { isSettingsView, settingsLandingView } from "@/lib/hotspot/settings-sections";
import { useHotspotStore } from "@/lib/hotspot/store";
import type { AuthUser, HotspotSession, ViewId } from "@/lib/hotspot/types";
import { ThemeToggle } from "./theme-toggle";
import { UserProfileDialog } from "./parts/user-profile-dialog";
import { SettingsSidebar } from "./settings/settings-shell";
import { ActivityBell, LiveClock, SearchPalette } from "./parts/topbar-widgets";
import { AnnouncementBanner } from "./parts/announcement-banner";
// N°164 — bannière de persistance dégradée (mode plateforme uniquement : le
// diagnostic complet vit dans la carte Santé, réservée rang 3).
import { PersistenceDegradedBanner } from "./parts/persistence-degraded-banner";

// Perf — vues en chargement différé : chaque vue = chunk distinct chargé à
// l'ouverture. Les librairies lourdes (recharts, qrcode…) ne sont plus
// téléchargées qu'à la première vue qui les utilise. Le rendu et le contrat
// d'API restent inchangés (mêmes composants, mêmes props).
const ViewFallback = (
  <div
    className="flex min-h-[40vh] items-center justify-center"
    role="status"
    aria-live="polite"
  >
    <Loader2 className="size-6 animate-spin text-muted-foreground" aria-hidden="true" />
  </div>
);

const AccountsView = dynamic(() => import("./views/accounts-view"), { loading: () => ViewFallback });
const BillingRequestsView = dynamic(() => import("./views/billing-requests-view"), { loading: () => ViewFallback });
const DashboardView = dynamic(() => import("./views/dashboard-view"), { loading: () => ViewFallback });
// N°100 — console HomeNet : tableau de bord maison et appareils connectés
// (chunks dédiés — un foyer ne paie jamais le bundle du dashboard métier).
const DevicesView = dynamic(() => import("./views/devices-view"), { loading: () => ViewFallback });
const HomeView = dynamic(() => import("./views/home-view"), { loading: () => ViewFallback });
// N°57-d — hub Hotspot : « portal » et « templates » rendent la MÊME page
// (onglet dérivé du ViewId — pattern N°30) : /app/settings/hotspot/portail
// et /app/settings/hotspot/modeles. N°184 — plus de vue « hotspot » (onglet
// Expérience retiré — fusion dans le hub, l'ancienne URL re-normalisée).
const HotspotView = dynamic(() => import("./views/hotspot-view"), { loading: () => ViewFallback });
const LogsView = dynamic(() => import("./views/logs-view"), { loading: () => ViewFallback });
const NotificationsView = dynamic(() => import("./views/notifications-view"), { loading: () => ViewFallback });
const PlatformLogsView = dynamic(() => import("./views/platform-logs-view"), { loading: () => ViewFallback });
const PlatformOverviewView = dynamic(() => import("./views/platform-overview-view"), { loading: () => ViewFallback });
// N°117 — parc routeurs global : flotte tous comptes + mise à jour RouterOS.
const PlatformFleetView = dynamic(() => import("./views/platform-fleet-view"), { loading: () => ViewFallback });
// N°127 — inbox de l'assistant conversationnel public (vitrine).
const PlatformChatView = dynamic(() => import("./views/platform-chat-view"), { loading: () => ViewFallback });
const PlatformSettingsView = dynamic(() => import("./views/platform-settings-view"), { loading: () => ViewFallback });
const PlatformTeamView = dynamic(() => import("./views/platform-team-view"), { loading: () => ViewFallback });
// N°152 — diffusion d'annonces aux clients MikCloud (console plateforme).
const PlatformAnnouncementsView = dynamic(() => import("./views/platform-announcements-view"), { loading: () => ViewFallback });
const ProfilesView = dynamic(() => import("./views/profiles-view"), { loading: () => ViewFallback });
// N°83 — vue Protection : l'état de sécurité du WiFi (verdict + 3 cartes
// SafeWiFi/Shield/FamilyGuard) vit dans la navigation principale.
const ProtectionView = dynamic(() => import("./views/protection-view"), { loading: () => ViewFallback });
const ReportsView = dynamic(() => import("./views/reports-view"), { loading: () => ViewFallback });
const ResellersView = dynamic(() => import("./views/resellers-view"), { loading: () => ViewFallback });
const RoutersView = dynamic(() => import("./views/routers-view"), { loading: () => ViewFallback });
const SecurityView = dynamic(() => import("./views/security-view"), { loading: () => ViewFallback });
const SessionsView = dynamic(() => import("./views/sessions-view"), { loading: () => ViewFallback });
const SettingsView = dynamic(() => import("./views/settings-view"), { loading: () => ViewFallback });
const SubscriptionView = dynamic(() => import("./views/subscription-view"), { loading: () => ViewFallback });
const TeamView = dynamic(() => import("./views/team-view"), { loading: () => ViewFallback });
// N°30 — hub fusionné : les vues « users » et « registrations » rendent la
// MÊME page Utilisateurs (onglets Comptes / Inscriptions) — l'onglet actif
// dérive du ViewId courant, les deux URLs restent deep-linkables.
const UsersHubView = dynamic(() => import("./views/users-hub-view"), { loading: () => ViewFallback });
const WifiView = dynamic(() => import("./views/wifi-view"), { loading: () => ViewFallback });
const VouchersView = dynamic(() => import("./views/vouchers-view"), { loading: () => ViewFallback });

/** Libellé dynamique de la vue active — utilisé pour l'aria du conteneur
 *  principal (le titre visible vit dans chaque vue via PageHeader). */
function viewTitle(view: ViewId, t: (key: string) => string): string {
  const keys: Record<ViewId, string> = {
    dashboard: "nav.dashboard",
    sessions: "nav.sessions",
    protection: "nav.protection",
    subscription: "sub.title",
    users: "nav.users",
    registrations: "nav.registrations",
    vouchers: "nav.vouchers",
    templates: "nav.templates",
    profiles: "nav.profiles",
    resellers: "nav.resellers",
    wifi: "wifi.title",
    routers: "nav.routers",
    portal: "nav.portal",
    reports: "nav.reports",
    logs: "logs.title",
    platform: "nav.platform",
    platformFleet: "nav.platformFleet",
    platformChat: "platformChat.title",
    platformLogs: "platformLogs.title",
    platformTeam: "platformTeam.title",
    billingRequests: "billingRequests.title",
    accounts: "nav.accounts",
    notifications: "nav.notifications",
    settings: "settings.tabGeneral",
    security: "settings.tabAdvanced",
    team: "nav.team",
    platformSettings: "platformSettings.title",
    platformAnnouncements: "ann.title",
    platformNotifications: "nav.platformNotifications", // N°153 — notifications du compte principal
    home: "nav.home",
    devices: "nav.devices",
  };
  return t(keys[view]);
}

const VIEWS: Record<ViewId, React.ComponentType> = {
  dashboard: DashboardView,
  sessions: SessionsView,
  // N°83 — vue Protection : verdict + 3 cartes sécurité (routeur adressable
  // /app/protection/<id>) — partagée par les DEUX consoles (N°100 : c'est
  // l'argument massue du foyer).
  protection: ProtectionView,
  subscription: SubscriptionView,
  // N°30 — les deux ViewIds pointent le même hub (onglet dérivé du ViewId).
  users: UsersHubView,
  registrations: UsersHubView,
  vouchers: VouchersView,
  // N°57-d — hub Hotspot : les DEUX ViewIds pointent le même hub
  // (Portail / Vouchers & tickets dérivent du ViewId, pattern N°30).
  // N°184 — la vue racine « hotspot » a disparu avec l'onglet Expérience.
  portal: HotspotView,
  templates: HotspotView,
  profiles: ProfilesView,
  resellers: ResellersView,
  wifi: WifiView,
  routers: RoutersView,
  reports: ReportsView,
  logs: LogsView,
  platform: PlatformOverviewView,
  platformFleet: PlatformFleetView,
  platformChat: PlatformChatView,
  platformLogs: PlatformLogsView,
  platformTeam: PlatformTeamView,
  platformSettings: PlatformSettingsView,
  platformAnnouncements: PlatformAnnouncementsView,
  // N°153 — même composant que la section client (le token super-admin
  // cible acc-main : la vue reçoit isPlatformAccount=true et se différencie
  // seule — bandeau compte principal + carte « E-mail plateforme »).
  platformNotifications: NotificationsView,
  billingRequests: BillingRequestsView,
  accounts: AccountsView,
  notifications: NotificationsView,
  settings: SettingsView,
  security: SecurityView,
  team: TeamView,
  // N°100 — console HomeNet.
  home: HomeView,
  devices: DevicesView,
};

/** N°112 — dernière vue MÉTIER visitée (hors zone Paramètres) : destination
 * du bouton « Retour » de la sidebar de zone. Vit au NIVEAU MODULE et non
 * dans un useRef de l'app-shell : Next.js REMONTE la page du catch-all
 * /app/[[...vue]] à chaque changement de segment d'URL (/app/routers →
 * /app/settings/general…), un ref d'instance repartait donc de zéro à
 * CHAQUE navigation et le Retour retombait systématiquement sur
 * l'atterrissage de la console — le défaut N°57-c « rouvre la dernière vue
 * métier visitée » n'a jamais réellement tenu (masqué tant qu'on entrait
 * en zone depuis le tableau de bord : le reset tombait par coïncidence sur
 * la bonne destination). Le module, lui, survit à tous ces remontages ;
 * null tant qu'aucune vue métier n'a été visitée (entrée par lien direct →
 * atterrissage de la console, comportement documenté N°100). Réécrit dès la
 * première vue métier d'une session (login → atterrissage) : aucune fuite
 * entre consoles. */
let zoneReturnView: ViewId | null = null;

/** N°195 — contrôle de la barre latérale desktop, TROIS modes :
 *  • "expanded" — Étendu : sidebar pleine largeur (w-64, contenu décalé) ;
 *  • "reduced"  — Réduit : rail d'icônes (w-16, contenu décalé du rail) ;
 *  • "hover"    — Survol : rail d'icônes, le SURVOL ouvre la sidebar en
 *    SURCOUCHE (le contenu ne se décale JAMAIS — l'ouverture flotte au-dessus,
 *    ombre portée, et se referme à la sortie du pointeur).
 * Bascule rapide : Ctrl+B — Étendu ↔ dernier mode rail utilisé (mémoire :
 * un amateur de Survol le retrouve au prochain repli). Sélecteur explicite
 * au BOUTON LATÉRAL de l'en-tête (N°196 — menu dédié au clic, le menu
 * avatar reste minimal). Préférence persistée localStorage (pattern
 * mikcloud.*, N°193) — lecture sûre : l'app-shell ne rend qu'après montage
 * (ShellFallback pré-montage, N°100). */
type SidebarMode = "expanded" | "reduced" | "hover";
const SIDEBAR_MODE_KEY = "mikcloud-sidebar-mode";

function readSidebarMode(): SidebarMode {
  try {
    const raw = localStorage.getItem(SIDEBAR_MODE_KEY);
    return raw === "reduced" || raw === "hover" ? raw : "expanded";
  } catch {
    return "expanded";
  }
}

/** Dernier mode « rail » choisi (reduced | hover) — destination du repli
 * Ctrl+B / bouton du rail. Niveau MODULE : l'app-shell est REMONTÉ par
 * Next à chaque changement de segment du catch-all (pattern zoneReturnView
 * N°112) — un état d'instance perdrait la mémoire à chaque navigation. */
let lastRailMode: Exclude<SidebarMode, "expanded"> = "reduced";

/** Infobulle du rail — libellé à droite de l'icône (modes Réduit/Survol :
 * le rail seul ne dit pas tout, l'infobulle si). Composant Tooltip shadcn
 * auto-porté (provider interne, N°193 — mêmes infobulles que vouchers). */
function RailTip({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>{children}</TooltipTrigger>
      <TooltipContent side="right" className="font-medium">
        {label}
      </TooltipContent>
    </Tooltip>
  );
}

/** Transition d'apparition de la vue active — fade + translation légère.
 * Extraite du rendu principal (N°57) : identique zone Paramètres ou non
 * (N°57-c — la zone vit dans la sidebar substituée, pas dans le contenu).
 * N°78 — framer-motion retiré du bundle initial : l'entrée est une
 * @keyframe CSS (mik-view-in) ; la sortie est instantanée (démontage
 * direct par React via key), ce qui remplace l'ancien mode="wait". */
function ViewTransition({ viewKey, children }: { viewKey: ViewId; children: React.ReactNode }) {
  return (
    <div key={viewKey} className="mik-view-in">
      {children}
    </div>
  );
}

/** En-tête de marque — logo + nom MikCloud. N°196 : le bouton latéral
 *  ouvre un MENU DE CHOIX DU MODE (Étendu / Réduit / Survol, coche sur
 *  l'actif) sur le modèle des consoles Azure/OneDrive — retour opérateur :
 *  le sélecteur au menu de la carte utilisateur était une surcharge, ce menu
 *  redevient minimal. En mode Étendu le bouton siège en bout de ligne
 *  (menu dessous, aligné à droite) ; en mode rail, sous le logo (menu à
 *  droite du bouton). Ctrl+B reste la bascule rapide Étendu ↔ dernier mode
 *  rail. Sur le Sheet mobile, aucun bouton (la fermeture vit au voile du
 *  tiroir — pas de modes desktop au mobile). */
function BrandHeader({
  rail = false,
  sidebarMode,
  onSidebarMode,
}: {
  rail?: boolean;
  sidebarMode?: SidebarMode;
  onSidebarMode?: (m: SidebarMode) => void;
}) {
  const { t } = useI18n();
  const logo = (
    <Image
      src="/logo.png"
      alt={t("shell.logoAlt")}
      width={36}
      height={36}
      className="sidebar-logo size-9 shrink-0 rounded-xl shadow-md shadow-primary/20"
    />
  );
  // Le menu est PORTÉ au body par Radix (portal) : jamais coupé par
  // l'overflow-hidden de l'aside pendant les transitions de largeur.
  const modeMenu = sidebarMode && onSidebarMode && (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button
          type="button"
          aria-label={t("shell.sidebarModeButtonTitle")}
          title={t("shell.sidebarModeButtonTitle")}
          className="flex size-8 shrink-0 items-center justify-center rounded-lg text-muted-foreground outline-none transition-colors hover:bg-accent/60 hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
        >
          <PanelLeft className="size-4" aria-hidden />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent side={rail ? "right" : "bottom"} align={rail ? "start" : "end"} className="w-56">
        <DropdownMenuLabel>{t("shell.sidebarMode")}</DropdownMenuLabel>
        <SidebarModeItems mode={sidebarMode} onPick={onSidebarMode} />
      </DropdownMenuContent>
    </DropdownMenu>
  );
  if (rail) {
    return (
      <div className="flex flex-col items-center gap-1 px-2 py-4">
        {logo}
        {modeMenu}
      </div>
    );
  }
  return (
    <div className="flex items-center gap-3 px-5 py-5">
      {logo}
      <div className="flex min-w-0 flex-1 items-center gap-2">
        <span className="text-aurora truncate text-base font-semibold tracking-tight">MikCloud</span>
        <Badge
          variant="outline"
          className="badge-pro border-primary/25 bg-primary/10 px-1.5 py-0 text-[10px] font-semibold tracking-wide text-primary"
        >
          PRO
        </Badge>
      </div>
      {modeMenu}
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

/** N°195/N°196 — items du sélecteur de mode de la barre latérale (menu
 *  ouvert par le BOUTON LATÉRAL de l'en-tête) : Étendu / Réduit / Survol,
 *  l'option active cochée. Le hint « Survol » (le survol ouvre la barre
 *  latérale) voyage en attribut title — l'infobulle native du menu déroulant
 *  reste discrète. */
function SidebarModeItems({ mode, onPick }: { mode: SidebarMode; onPick: (m: SidebarMode) => void }) {
  const { t } = useI18n();
  const options: { value: SidebarMode; labelKey: string; hintKey?: string; icon: LucideIcon }[] = [
    { value: "expanded", labelKey: "shell.sidebarModeExpanded", icon: PanelLeftOpen },
    { value: "reduced", labelKey: "shell.sidebarModeReduced", icon: PanelLeftClose },
    { value: "hover", labelKey: "shell.sidebarModeHover", hintKey: "shell.sidebarModeHoverHint", icon: MousePointer2 },
  ];
  return (
    <>
      {options.map((opt) => (
        <DropdownMenuItem
          key={opt.value}
          className="min-h-10"
          title={opt.hintKey ? t(opt.hintKey) : undefined}
          onClick={() => onPick(opt.value)}
        >
          <opt.icon className="size-4" />
          <span className="flex-1">{t(opt.labelKey)}</span>
          {mode === opt.value && <Check className="size-4 shrink-0 text-primary" aria-hidden />}
        </DropdownMenuItem>
      ))}
    </>
  );
}

function UserCard({ rail = false }: { rail?: boolean }) {
  const { t, lang } = useI18n();
  const user = useHotspotStore((s) => s.user);
  const logout = useHotspotStore((s) => s.logout);
  const setView = useHotspotStore((s) => s.setView);
  const shellMode = useHotspotStore((s) => s.shellMode);
  const queryClient = useQueryClient();
  const [profileOpen, setProfileOpen] = useState(false);
  const name = user?.name ?? t("profile.defaultUser");
  // I — en mode plateforme, « Paramètres » ouvre les paramètres plateforme
  // (la vue client serait immédiatement redirigée par le guard platformMode).
  const isPlatformMode = shellMode === "platform";

  function handleLogout() {
    logout();
    void queryClient.clear();
  }

  // N°195 — en mode rail, le déclencheur se réduit à l'avatar centré (même
  // menu, mêmes actions) ; l'identité complète vit dans l'infobulle aria.
  const trigger = rail ? (
    <button
      type="button"
      aria-label={t("shell.profileMenu")}
      className="sidebar-usercard flex size-10 items-center justify-center rounded-lg border border-border/70 bg-card/60 outline-none transition-colors hover:bg-accent/60 focus-visible:ring-2 focus-visible:ring-ring data-[state=open]:bg-accent/60 data-[state=open]:text-accent-foreground"
    >
      <Avatar className="size-8 shrink-0">
        <AvatarFallback className="sidebar-avatar bg-primary/15 text-xs font-semibold text-primary">
          {userInitials(name)}
        </AvatarFallback>
      </Avatar>
    </button>
  ) : (
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
  );

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>{trigger}</DropdownMenuTrigger>
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
          <DropdownMenuItem
            onClick={() => setView(isPlatformMode ? "platformSettings" : settingsLandingView(user?.role, user?.usage))}
            className="min-h-10"
          >
            <Settings className="size-4" />
            {t("shell.settings")}
          </DropdownMenuItem>
          <LanguageMenuItem />
          {/* N°196 — retour opérateur : le menu de la carte utilisateur
              redevient MINIMAL (identité / actions de compte) — le choix du
              mode de barre latérale vit au bouton latéral de l'en-tête. */}
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

/** ModeSwitch — affiche uniquement le bouton « retour à la console plateforme »
 * quand l'admin plateforme est en session support (impersonation d'un compte
 * client). La bascule vers une console client se fait depuis la vue « Comptes
 * SaaS » (accounts-view) qui dispose d'un bouton « Ouvrir la console » par
 * compte — le sélecteur de la sidebar était redondant et a été retiré (H).
 * N°195 — en mode rail : même bouton réduit à l'icône + infobulle. */
function ModeSwitch({ rail = false }: { rail?: boolean }) {
  const { t } = useI18n();
  const queryClient = useQueryClient();
  const user = useHotspotStore((s) => s.user);
  const impersonating = useHotspotStore((s) => !!s.ownToken);
  const exitImpersonation = useHotspotStore((s) => s.exitImpersonation);
  const isPlatformAdmin = user?.role === "admin" || user?.role === "platform_admin";

  if (!isPlatformAdmin || !impersonating) return null;

  function backToPlatform() {
    exitImpersonation();
    queryClient.clear();
    toast.success(t("shell.exitImpersonationToast"));
  }

  if (rail) {
    return (
      <div className="px-2 pb-2">
        <RailTip label={t("shell.exitImpersonation")}>
          <button
            type="button"
            onClick={backToPlatform}
            aria-label={t("shell.exitImpersonation")}
            className="flex min-h-11 w-full items-center justify-center rounded-lg border border-sidebar-border bg-card/50 text-muted-foreground outline-none transition-colors hover:bg-accent/60 hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
          >
            <ShieldCheck className="size-4" aria-hidden />
          </button>
        </RailTip>
      </div>
    );
  }

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

function NavList({ rail = false }: { rail?: boolean }) {
  const { t } = useI18n();
  const view = useHotspotStore((s) => s.view);
  const setView = useHotspotStore((s) => s.setView);
  const user = useHotspotStore((s) => s.user);
  const shellMode = useHotspotStore((s) => s.shellMode);
  // N°130 — prefetch au survol : le queryClient porte les requêtes
  // principales de la vue visée (clés stables uniquement, cf. prefetch.ts).
  const queryClient = useQueryClient();
  // N°30 — « registrations » est fusionné dans la page Utilisateurs (hub) :
  // la sidebar surligne et auto-ouvre « Utilisateurs » pour les deux ViewIds
  // (l'item « Inscriptions » n'existe plus dans la navigation).
  const navView: ViewId = view === "registrations" ? "users" : view;
  const isAdmin = user?.role === "admin" || user?.role === "platform_admin";
  // O — sections repliables : état persisté par section (labelKey). Une
  // section fermée manuellement reste fermée entre les visites ; celle qui
  // contient la vue active s'ouvre toujours automatiquement.
  const [collapsed, setCollapsed] = useState<Set<string>>(() => {
    try {
      const raw = localStorage.getItem("mikcloud-nav-collapsed");
      return new Set<string>(raw ? (JSON.parse(raw) as string[]) : []);
    } catch {
      return new Set<string>();
    }
  });
  function toggleSection(labelKey: string) {
    setCollapsed((prev) => {
      const next = new Set(prev);
      if (next.has(labelKey)) next.delete(labelKey);
      else next.add(labelKey);
      return next;
    });
  }
  // Console plateforme : navigation dédiée (cockpit opérateur) ; N°100 —
  // console MAISON pour un compte homenet (une section, l'histoire du
  // produit en 3 items) ; sinon la navigation client métier habituelle.
  // « accounts » n'est rendu qu'en mode client pour l'admin plateforme (en
  // mode plateforme, il est déjà dans sa section).
  const usage = usageOf(user?.usage);
  const sections =
    isAdmin && shellMode === "platform"
      ? NAV_PLATFORM_SECTIONS
      : usage === "homenet"
        ? NAV_HOMENET_SECTIONS
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
  // O — auto-ouverture de la section active, calculée PENDANT LE RENDU
  // (pattern React documenté « ajuster l'état quand une valeur dérivée
  // change », remplace l'ancien effet setState — react-hooks/set-state-in-effect
  // N°57-e) : naviguer vers une vue ROUVRE automatiquement sa section, mais
  // le repli manuel reste maître tant que la section ne change pas (garde
  // par comparaison de la dernière section auto-ouverte — même sémantique
  // que l'ancien lastAutoOpened).
  const [autoOpened, setAutoOpened] = useState<string | null>(null);
  // N°57-f — l'entrée nav « Paramètres » a disparu (accès unique : menu
  // utilisateur) : la section active est simplement celle qui porte la vue
  // métier courante (en zone Paramètres, cette NavList n'est pas rendue —
  // remplacée par la sidebar de zone, N°57-c).
  const activeSectionKey =
    sections.find((s) => s.items.some((item) => item.id === navView))?.labelKey ?? null;
  if (activeSectionKey !== autoOpened) {
    setAutoOpened(activeSectionKey);
    if (activeSectionKey !== null && collapsed.has(activeSectionKey)) {
      const next = new Set(collapsed);
      next.delete(activeSectionKey);
      setCollapsed(next);
    }
  }
  // Persistance du repli (système externe localStorage) : effet PUR sans
  // setState — couvre repli manuel ET auto-ouverture, réagit à chaque
  // changement de « collapsed » (l'écriture initiale est idempotente).
  useEffect(() => {
    try {
      localStorage.setItem("mikcloud-nav-collapsed", JSON.stringify([...collapsed]));
    } catch {
      /* stockage indisponible — état de session uniquement */
    }
  }, [collapsed]);
  // N°57-e — plus de badge « Abonnement » ici : l'entrée nav dédiée a
  // disparu (la facturation vit dans la zone Paramètres). Le statut reste
  // visible passivement via le bandeau du dashboard (expiré / échéance
  // proche) et le mur P5 (PaywallOverlay, autonome).

  // N°195 — MODE RAIL (Réduit/Survol, ou surcouche fermée) : icônes seules,
  // infobulles à droite, séparateurs entre sections (les libellés de section
  // ne tiennent pas dans 64 px — la voix complète vit dans l'infobulle).
  // Préchargement N°130 conservé (survol/focus d'une icône = prefetch de la
  // vue) ; point « live » conservé (sessions/appareils/file de facturation) ;
  // la sélection reste surlignée (nav-active).
  if (rail) {
    return (
      <nav className="flex-1 overflow-y-auto overflow-x-hidden px-2 pb-4" aria-label={t("nav.main")}>
        <ModeSwitch rail />
        {sections.map((section, idx) => {
          const items = section.items.filter(
            (item) => (item.id !== "accounts" || isAdmin) && canView(user?.role, item.id, usage),
          );
          if (items.length === 0) return null;
          return (
            <div key={section.labelKey}>
              {idx > 0 && (
                <div className="mx-2 mb-2 h-px bg-border/60" role="separator" aria-hidden />
              )}
              <ul className="space-y-1">
                {items.map((item) => {
                  const active = item.id === navView;
                  const live =
                    ((item.id === "sessions" || item.id === "devices") && sessionsCount > 0) ||
                    (item.id === "billingRequests" && billingPending > 0);
                  return (
                    <li key={item.id}>
                      <RailTip label={t(item.labelKey)}>
                        <button
                          type="button"
                          onClick={() => setView(item.id)}
                          onMouseEnter={() => prefetchView(item.id, queryClient)}
                          onFocus={() => prefetchView(item.id, queryClient)}
                          aria-current={active ? "page" : undefined}
                          aria-label={t(item.labelKey)}
                          className={cn(
                            "sidebar-nav-item relative flex min-h-11 w-full items-center justify-center rounded-lg transition-all duration-200",
                            active
                              ? "nav-active"
                              : "text-muted-foreground hover:bg-accent/60 hover:text-foreground",
                          )}
                        >
                          <span className="relative flex shrink-0 items-center">
                            <item.icon className="size-4.5" aria-hidden />
                            {live && (
                              <span
                                className="live-dot absolute -right-1.5 -top-1 block size-2 rounded-full bg-primary"
                                aria-hidden
                              />
                            )}
                          </span>
                        </button>
                      </RailTip>
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

  return (
    <nav className="flex-1 space-y-1 overflow-y-auto px-3 pb-4" aria-label={t("nav.main")}>
      <ModeSwitch />
      {sections.map((section) => {
        // N°7 — chaque vue n'apparaît que si le rôle peut l'ouvrir
        // (miroir client des requireRole serveur ; comptes = admin plateforme).
        // N°57-f — plus d'entrée « Paramètres » ici : la zone vit derrière le
        // menu utilisateur (UserCard/menu profil) et la substitution N°57-c.
        const items = section.items.filter(
          (item) => (item.id !== "accounts" || isAdmin) && canView(user?.role, item.id, usage),
        );
        if (items.length === 0) return null;
        // O — état replié explicite de l'utilisateur (localStorage) ; la
        // section de la vue active s'est auto-ouverte au moment de la
        // navigation (voir useEffect ci-dessus) mais reste repliable ensuite.
        const open = !collapsed.has(section.labelKey);
        return (
          <div key={section.labelKey}>
            <button
              type="button"
              onClick={() => toggleSection(section.labelKey)}
              aria-expanded={open}
              aria-label={t(section.labelKey)}
              className="flex w-full items-center gap-1.5 whitespace-nowrap rounded-md px-2.5 py-2 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground/70 transition-colors hover:bg-accent/40 hover:text-foreground"
            >
              <ChevronDown
                className={`size-3.5 shrink-0 transition-transform duration-200 ${open ? "" : "-rotate-90"}`}
                aria-hidden
              />
              {t(section.labelKey)}
            </button>
            {open && (
              <ul className="space-y-0.5 pb-1">
                {items.map((item) => {
                  const active = item.id === navView;
                return (
                  <li key={item.id}>
                    <button
                      type="button"
                      onClick={() => setView(item.id)}
                      // N°130 — préchargement au survol (et au focus clavier) :
                      // chunk de la vue + requêtes principales, pour que le clic
                      // suivant rende immédiatement. Idempotent — aucun effet si
                      // déjà en cache ; le tactile (sans survol) garde le trajet
                      // normal au clic.
                      onMouseEnter={() => prefetchView(item.id, queryClient)}
                      onFocus={() => prefetchView(item.id, queryClient)}
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
                        {(item.id === "sessions" || item.id === "devices") && sessionsCount > 0 && (
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
                      {(item.id === "sessions" || item.id === "devices") && sessionsCount > 0 && (
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
            )}
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
  const shellMode = useHotspotStore((s) => s.shellMode);
  const queryClient = useQueryClient();
  const [profileOpen, setProfileOpen] = useState(false);
  // I — en mode plateforme, « Paramètres » ouvre les paramètres plateforme.
  const isPlatformMode = shellMode === "platform";

  function handleRefresh() {
    // N°78 — refresh ciblé : seules les requêtes ACTIVES (composants montés)
    // sont invalidées — plus la rafale historique de refetchs sur des vues
    // non consultées (polling tranquille, moins de contention sur le
    // 0,1 vCPU Render).
    void queryClient.invalidateQueries({ type: "active" });
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
                <DropdownMenuItem
                  onClick={() => setView(isPlatformMode ? "platformSettings" : settingsLandingView(user?.role, user?.usage))}
                  className="min-h-10"
                >
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
  const syncUsage = useHotspotStore((s) => s.syncUsage);
  const shellMode = useHotspotStore((s) => s.shellMode);
  const isPlatformAdmin = user?.role === "admin" || user?.role === "platform_admin";
  const platformMode = isPlatformAdmin && shellMode === "platform";
  const usage = usageOf(user?.usage);

  // N°100 — auto-réparation de l'usage : la coquille relit /api/auth/me
  // (usage relu sous verrou côté serveur à CHAQUE appel, N°98) une fois par
  // chargement de console. Deux cas réels : (1) le gérant a basculé le
  // compte en console plateforme — le client voit SA nouvelle console au
  // prochain rafraîchissement, sans re-login ; (2) session persistée
  // antérieure à N°98 (user.usage absent) — la coquille se corrige
  // d'elle-même. Les autres champs de /me ne touchent PAS au store ici
  // (rôle/nom rafraîchis par la fiche profil, son consommateur historique).
  const { data: me } = useQuery({
    queryKey: ["/api/auth/me", "shell"],
    queryFn: () => api<{ user: AuthUser & { usage?: string } }>("/api/auth/me"),
    enabled: !!user?.accountId && !platformMode,
    staleTime: Infinity,
    retry: false,
  });
  const meUsage = me?.user?.usage === "homenet" ? "homenet" : me?.user ? "hotspot" : undefined;
  // Synchronisation dans un EFFET (store externe — jamais de setState pendant
  // le rendu) : une seule fois par divergence, la garde de cohérence
  // ci-dessous normalise la vue au cycle suivant.
  useEffect(() => {
    if (meUsage === undefined || !user || meUsage === usage) return;
    // Fusion chirurgicale : seul l'usage change (token/vue/mode intacts).
    syncUsage({ ...user, usage: meUsage });
  }, [meUsage, usage, user, syncUsage]);

  // Cohérence mode ↔ vue : en mode plateforme, une vue client résiduelle
  // (rechargement, palette, lien) retombe sur la vue d'ensemble plateforme —
  // et inversement en mode client (les vues plateforme y sont interdites).
  // N°100 — cohérence CONSOLE ↔ vue : une vue hors de SA console retombe sur
  // son atterrissage (maison pour homenet, dashboard pour hotspot) —
  // rechargement sur /app/vouchers après bascule du compte, signet périmé,
  // localStorage… Le serveur refuserait les appels de toute façon (404
  // requireUsage / 403 requireRole) : l'UI re-normalise, elle ne montre jamais
  // une page morte.
  const clientLanding: ViewId = usage === "homenet" ? "home" : "dashboard";
  useEffect(() => {
    if (platformMode && !isPlatformView(view)) {
      setView("platform");
    } else if (isPlatformAdmin && !platformMode && isPlatformView(view)) {
      setView(clientLanding);
    } else if (!platformMode && !canView(user?.role, view, usage)) {
      setView(clientLanding);
    }
  }, [platformMode, isPlatformAdmin, view, setView, usage, user?.role, clientLanding]);

  // Garde-fou N°7 : une vue interdite au rôle (p.ex. un lien direct restant
  // après un changement de rôle) retombe sur le dashboard — le serveur
  // refuserait les appels de toute façon (403). N°100 : le fallback respecte
  // la console active (maison pour homenet).
  const ActiveView = canView(user?.role, view, usage)
    ? (VIEWS[view] ?? DashboardView)
    : platformMode
      ? PlatformOverviewView
      : usage === "homenet"
        ? HomeView
        : DashboardView;

  // N°57-c — zone Paramètres : quand une vue de la zone est active, la
  // sidebar de sections (bouton Retour + sections filtrées par rôle)
  // REMPLACE NavList dans le même <aside> — le layout reste TOUJOURS à
  // 2 colonnes (fin du « sidebar dans sidebar » de N°57) ; le contenu rend
  // la vue comme tout autre module. Conditions : la vue appartient à la
  // zone ET le rôle peut l'ouvrir (les liens directs interdits ont déjà
  // été re-normalisés par le garde-fou URL d'app-route). En mode plateforme
  // la zone n'existe pas (console dédiée).
  const zoneRender = isSettingsView(view) && canView(user?.role, view, usage) && !platformMode;

  // N°57-c — dernière vue MÉTIER visitée : destination du bouton « Retour »
  // de la sidebar de zone. N°112 — le suivi vit au NIVEAU MODULE (voir
  // zoneReturnView ci-dessus) : l'app-shell est REMONTÉ par Next.js à
  // chaque changement de segment du catch-all /app/[[...vue]] — un useRef
  // repartait de zéro à chaque montage et le Retour retombait sur
  // l'atterrissage de la console au lieu de la dernière vue métier (constaté
  // par instrumentation : 5 montages d'app-shell pour 4 navigations).
  // Défaut = atterrissage de la console courante (N°100 : maison pour
  // homenet) pour une entrée par lien direct. Mis à jour à chaque vue
  // métier — jamais pendant la zone (on garde l'origine, même après un
  // détour par plusieurs sections).
  useEffect(() => {
    if (!isSettingsView(view)) zoneReturnView = view;
  }, [view]);
  const handleZoneBack = () => setView(zoneReturnView ?? clientLanding);

  // ══ N°195 — contrôle de la barre latérale (Étendu / Réduit / Survol) ══
  // Préférence persistée (pattern mikcloud.* N°193) ; lecture sûre : la
  // coquille ne rend qu'après montage (ShellFallback pré-montage, N°100).
  const [sidebarMode, setSidebarMode] = useState<SidebarMode>(readSidebarMode);
  // Mode Survol uniquement : le pointeur est-il sur le rail ? L'ouverture
  // se fait en SURCOUCHE (le contenu ne se décale jamais — pl reste celui
  // du rail, la sidebar flotte au-dessus avec ombre portée).
  const [railHover, setRailHover] = useState(false);
  const railCloseTimer = useRef<number | null>(null);

  // Décompte du mode effectif — « railPersistent » décide du DÉCALAGE du
  // contenu (le mode Survol ne décale jamais, même ouvert en surcouche) ;
  // « expandedNow » décide du RENDU de la sidebar (pleine ou rail).
  const railPersistent = sidebarMode !== "expanded";
  const overlayOpen = sidebarMode === "hover" && railHover;
  const expandedNow = sidebarMode === "expanded" || overlayOpen;

  // Persistance + mémoire du dernier mode rail (destination du repli
  // Ctrl+B / bouton du rail — un amateur de Survol le retrouve intact).
  useEffect(() => {
    if (sidebarMode !== "expanded") lastRailMode = sidebarMode;
    try {
      localStorage.setItem(SIDEBAR_MODE_KEY, sidebarMode);
    } catch {
      /* stockage indisponible — état de session uniquement */
    }
  }, [sidebarMode]);

  // Bascule Étendu ↔ dernier mode rail — le geste de Ctrl+B (le bouton
  // latéral ouvre le sélecteur de mode depuis N°196, il ne bascule plus
  // directement). useCallback : l'écouteur clavier vit au niveau fenêtre.
  const toggleSidebarMode = useCallback(() => {
    setRailHover(false);
    setSidebarMode((m) => (m === "expanded" ? lastRailMode : "expanded"));
  }, []);

  // Ctrl+B / ⌘B — même famille que ⌘K de la palette (topbar-widgets) : le
  // seul conflit potentiel (composant shadcn ui/sidebar) n'est jamais monté
  // dans la console (import nul — vérifié). preventDefault : Ctrl+B n'a pas
  // d'action native utile ici (aucun éditeur riche).
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if ((e.key === "b" || e.key === "B") && (e.ctrlKey || e.metaKey)) {
        e.preventDefault();
        toggleSidebarMode();
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [toggleSidebarMode]);

  // Sélecteur explicite (menu du bouton latéral) : remplace le mode et
  // referme toute surcouche résiduelle.
  function applySidebarMode(m: SidebarMode) {
    setRailHover(false);
    setSidebarMode(m);
  }

  // Mode Survol — ouverture immédiate, fermeture avec GRÂCE (150 ms : un
  // balayage accidentel du rail ne fait pas clignoter la sidebar). Garde à
  // la fermeture : un menu Radix OUVERT (porté hors du rail — p.ex. le menu
  // de la carte utilisateur) retient l'ouverture le temps qu'il se referme ;
  // un état ouvert résiduel se guérit au prochain passage du pointeur.
  // NB (1) pas d'ouverture au FOCUS volontaire (Tab) : l'expansion remplace
  // le bouton sous le curseur entre mousedown et mouseup → un clic pouvait
  // se PERDRE ; le clavier garde Ctrl+B et le bouton du rail (focusables et
  // labellés). NB (2) la garde ne regarde PAS document.activeElement : à la
  // fermeture d'un menu, Radix rend le focus au déclencheur DANS le rail —
  // une garde focus retiendrait alors la surcouche indéfiniment.
  function openRailOverlay() {
    if (railCloseTimer.current !== null) {
      window.clearTimeout(railCloseTimer.current);
      railCloseTimer.current = null;
    }
    setRailHover(true);
  }
  function scheduleRailClose() {
    if (railCloseTimer.current !== null) window.clearTimeout(railCloseTimer.current);
    railCloseTimer.current = window.setTimeout(() => {
      railCloseTimer.current = null;
      if (document.querySelector("[data-radix-popper-content-wrapper]")) return;
      setRailHover(false);
    }, 150);
  }
  // Nettoyage du minuteur au démontage (remontage du catch-all — N°112).
  useEffect(
    () => () => {
      if (railCloseTimer.current !== null) window.clearTimeout(railCloseTimer.current);
    },
    [],
  );

  return (
    <div className="flex min-h-screen">
      {/* PaywallOverlay (P5) — mur total si compte suspendu (PeriodEnd + 30j).
          S'affiche par-dessus toute la console, non refermable. */}
      <PaywallOverlay />

      {/* Sidebar desktop — colonne de marque Aurora. N°57-c : dans la zone
          Paramètres, la sidebar de sections REMPLACE NavList (substitution
          dans le même conteneur — le layout reste à 2 colonnes, la marque,
          la carte utilisateur et le crédit FTCI restent en place).
          N°195 : la colonne devient un RAIL d'icônes (w-16) en modes Réduit
          et Survol — transitions de largeur 200 ms ; en Survol, l'ouverture
          survolée flotte au-dessus du contenu (z-30 > topbar z-20, ombre
          portée) SANS jamais le décaler. overflow-hidden : pendant la
          transition de largeur, les libellés tronqués ne débordent pas.
          Survol : mouseenter ET mousemove — un pointeur DÉJÀ dans le rail
          quand le mode bascule (choix « Survol » au sélecteur pendant que la
          sidebar est ouverte) doit pouvoir ouvrir sans ressortir. */}
      <aside
        onMouseEnter={sidebarMode === "hover" ? openRailOverlay : undefined}
        onMouseMove={sidebarMode === "hover" ? openRailOverlay : undefined}
        onMouseLeave={sidebarMode === "hover" ? scheduleRailClose : undefined}
        className={cn(
          "sidebar-aurora fixed inset-y-0 left-0 z-30 hidden flex-col overflow-hidden transition-[width] duration-200 ease-out lg:flex",
          expandedNow ? "w-64" : "w-16",
          overlayOpen && "shadow-2xl",
        )}
      >
        <BrandHeader rail={!expandedNow} sidebarMode={sidebarMode} onSidebarMode={applySidebarMode} />
        {zoneRender ? (
          <SettingsSidebar rail={!expandedNow} onBack={handleZoneBack} />
        ) : (
          <NavList rail={!expandedNow} />
        )}
        <div className={expandedNow ? "px-3 pb-4" : "px-2 pb-4"}>
          <UserCard rail={!expandedNow} />
          {/* Crédit FTCI : texte long — masqué en rail (l'identité complète
              vit en mode Étendu / surcouche ouverte ; whitespace-nowrap pour
              que la transition ne le replie jamais sur plusieurs lignes). */}
          {expandedNow && (
            <FtciCredit className="mt-3 w-full whitespace-nowrap text-center text-[10px] text-muted-foreground/70" />
          )}
        </div>
      </aside>

      {/* Sidebar mobile (Sheet) — pas de carte utilisateur : le profil
          reste accessible via l'avatar du header sur mobile. N°195 : rendu
          complet permanent (rail={false} implicite) — le mobile vit en
          tiroir, les modes desktop n'ont pas de prise ici. */}
      <Sheet open={sidebarOpen} onOpenChange={setSidebarOpen}>
        <SheetContent side="left" className="sidebar-aurora flex w-72 flex-col gap-0 p-0">
          <SheetHeader className="border-b border-sidebar-border pb-0">
            <SheetTitle className="sr-only">MikCloud</SheetTitle>
            <BrandHeader />
          </SheetHeader>
          {zoneRender ? <SettingsSidebar onBack={handleZoneBack} /> : <NavList />}
          <div className="mt-auto px-4 pb-5 pt-2">
            <FtciCredit className="w-full text-center text-[10px] text-muted-foreground/70" />
          </div>
        </SheetContent>
      </Sheet>

      {/* Contenu principal — le décalage suit le mode PERSISTENT (rail en
          modes Réduit/Survol : la surcouche du Survol ne décale JAMAIS le
          contenu, c'est ce qui la distingue d'un Étendu). Transition de
          padding synchronisée avec celle de la largeur de la colonne. */}
      <div
        className={cn(
          "flex min-w-0 flex-1 flex-col transition-[padding] duration-200 ease-out",
          railPersistent ? "lg:pl-16" : "lg:pl-64",
        )}
      >
        <Topbar />
        <ImpersonationBanner />
        {/* N°152 — annonce de la plateforme : bandeau masquable (la trace
            durable vit dans la cloche). En mode client Y COMPRIS session
            support — le super-admin consulte la vérité du compte visité ;
            le mode plateforme n'est pas un client (liste vide côté serveur). */}
        {!platformMode ? <AnnouncementBanner /> : null}
        {/* N°164 — persistance suspendue (boot résilient) : visible du mode
            plateforme pour agir vite ; la carte Santé porte le détail. */}
        {platformMode ? <PersistenceDegradedBanner /> : null}
        <main className="flex-1" aria-label={viewTitle(view, t)}>
          <div className="mx-auto max-w-7xl space-y-6 p-4 sm:p-6">
            {/* N°57-c — zone Paramètres : le contenu rend la vue comme tout
                autre module (la zone vit dans la sidebar substituée, pas
                dans le contenu) ; transition identique hors zone. */}
            <ViewTransition viewKey={view}>
              <ActiveView />
            </ViewTransition>
          </div>
        </main>
      </div>
    </div>
  );
}
