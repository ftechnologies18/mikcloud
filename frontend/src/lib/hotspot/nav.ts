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
  Router as RouterIcon,
  ScrollText,
  Settings,
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

/** Liste plate des items de nav visibles pour un rôle donné (miroir canView). */
export function navItemsFor(role: string | undefined, isAdmin: boolean): NavItem[] {
  return NAV_SECTIONS.flatMap((s) => s.items).filter(
    (item) => (item.id !== "accounts" || isAdmin) && canView(role, item.id),
  );
}
