// Configuration de navigation partagée — sidebar (app-shell) et palette de
// recherche (topbar) rendent les mêmes sections pour rester cohérents.

import type { LucideIcon } from "lucide-react";
import {
  BarChart3,
  Bell,
  Building2,
  Gauge,
  LayoutDashboard,
  Printer,
  Radio,
  ReceiptText,
  Router as RouterIcon,
  ScrollText,
  Settings,
  ShieldCheck,
  Store,
  Ticket,
  Users,
  UsersRound,
} from "lucide-react";
import { canView } from "./roles";
import type { ViewId } from "./types";

export interface NavItem {
  id: ViewId;
  labelKey: string;
  icon: LucideIcon;
}

export const NAV_SECTIONS: { labelKey: string; items: NavItem[] }[] = [
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
      // « Comptes » n'est visible que de l'admin plateforme (rôle admin) —
      // filtré au rendu (NavList + SearchPalette).
      { id: "accounts", labelKey: "nav.accounts", icon: Building2 },
      { id: "team", labelKey: "nav.team", icon: UsersRound },
      { id: "notifications", labelKey: "nav.notifications", icon: Bell },
      { id: "settings", labelKey: "nav.settings", icon: Settings },
    ],
  },
];

/** Liste plate des items de nav pour la console ACTIVE — client par défaut,
 * plateforme quand l'admin y bascule (miroir canView dans les deux cas). */
export function navItemsFor(
  role: string | undefined,
  isAdmin: boolean,
  mode: "platform" | "client" = "client",
): NavItem[] {
  const sections = mode === "platform" ? NAV_PLATFORM_SECTIONS : NAV_SECTIONS;
  return sections
    .flatMap((s) => s.items)
    .filter((item) => (item.id !== "accounts" || isAdmin) && canView(role, item.id));
}

/**
 * Navigation de la CONSOLE PLATEFORME — cockpit du propriétaire du SaaS
 * (super-admin MikCloud) : pilotage global, comptes clients, journal
 * transverse, équipe plateforme. Les clients ne la voient jamais.
 */
export const NAV_PLATFORM_SECTIONS: { labelKey: string; items: NavItem[] }[] = [
  {
    labelKey: "nav.section.platform",
    items: [
      { id: "platform", labelKey: "nav.platform", icon: ShieldCheck },
      { id: "accounts", labelKey: "nav.accounts", icon: Building2 },
      // File des demandes de renouvellement (facturation) — badge en attente.
      { id: "billingRequests", labelKey: "nav.billingRequests", icon: ReceiptText },
      { id: "platformLogs", labelKey: "nav.platformLogs", icon: ScrollText },
      { id: "platformTeam", labelKey: "nav.platformTeam", icon: UsersRound },
      // I (paramètres plateforme) — config globale du SaaS (nom, inscriptions,
      // sécurité, langue, purge globale).
      { id: "platformSettings", labelKey: "nav.platformSettings", icon: Settings },
    ],
  },
];
