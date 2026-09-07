// Configuration de navigation partagée — sidebar (app-shell) et palette de
// recherche (topbar) rendent les mêmes sections pour rester cohérents.

import type { LucideIcon } from "lucide-react";
import {
  BarChart3,
  Building2,
  Gauge,
  LayoutDashboard,
  ReceiptText,
  Radio,
  ScrollText,
  Settings,
  ShieldCheck,
  Store,
  Ticket,
  Users,
  UsersRound,
  Wifi,
} from "lucide-react";
import { canView } from "./roles";
import { settingsSectionsFor } from "./settings-sections";
import type { ViewId } from "./types";

export interface NavItem {
  id: ViewId;
  labelKey: string;
  icon: LucideIcon;
}

export const NAV_SECTIONS: { labelKey: string; items: NavItem[] }[] = [
  // O — 4 catégories métier (au lieu de 6 sections plates) : la sidebar est
  // plus compacte et chaque groupe est repliable (row expandable).
  {
    labelKey: "nav.section.exploitation",
    items: [
      { id: "dashboard", labelKey: "nav.dashboard", icon: LayoutDashboard },
      { id: "sessions", labelKey: "nav.sessions", icon: Radio },
      { id: "users", labelKey: "nav.users", icon: Users },
      // N°30 — les inscriptions publiques (N°27) sont fusionnées dans la
      // page Utilisateurs (onglet) : l'entrée dédiée disparaît de la
      // sidebar. ViewId « registrations » reste valide (URL
      // /app/registrations → hub Utilisateurs, onglet Inscriptions).
      { id: "vouchers", labelKey: "nav.vouchers", icon: Ticket },
      // N°57 — « Modèles » vit désormais dans la zone Paramètres
      // (/app/settings/templates) : la sidebar ne montre plus que les
      // modules métier.
      { id: "profiles", labelKey: "nav.profiles", icon: Gauge },
    ],
  },
  {
    labelKey: "nav.section.commercial",
    items: [
      // N°57-e — l'Abonnement quitte la navigation principale : la
      // facturation de l'espace vit désormais dans la zone Paramètres
      // (/app/settings/subscription, section dédiée de la sidebar de zone).
      // L'ancienne URL /app/subscription reste deep-linkable (re-normalisée).
      { id: "resellers", labelKey: "nav.resellers", icon: Store },
      // N°27 — WiFi jetable : mode d'accès offert des établissements.
      { id: "wifi", labelKey: "wifi.title", icon: Wifi },
    ],
  },
  // N°57 — la section Infrastructure disparaît : Routeurs et Portail sont
  // des vues de configuration, elles vivent dans la zone Paramètres
  // (/app/settings/routers, /app/settings/portal).
  {
    labelKey: "nav.section.analysis",
    items: [
      { id: "reports", labelKey: "nav.reports", icon: BarChart3 },
      { id: "logs", labelKey: "nav.logs", icon: ScrollText },
      // « Comptes » n'est visible que de l'admin plateforme (rôle admin) —
      // filtré au rendu (NavList + SearchPalette).
      { id: "accounts", labelKey: "nav.accounts", icon: Building2 },
    ],
  },
  {
    // N°57 — entrée unique de la zone Paramètres : toutes les vues de
    // configuration (général, routeurs, portail, modèles, notifications,
    // équipe) vivent sous /app/settings/<section> — en zone active, la
    // sidebar de sections REMPLPLACE la navigation principale (N°57-c).
    // La destination s'adapte au rôle (cf. settings-sections.ts) : le
    // gérant atterrit sur sa première section accessible, le propriétaire
    // sur la racine de la zone.
    labelKey: "nav.section.system",
    items: [{ id: "settings", labelKey: "nav.settings", icon: Settings }],
  },
];

/** Liste plate des items de nav pour la console ACTIVE — client par défaut,
 * plateforme quand l'admin y bascule (miroir canView dans les deux cas).
 * N°57 — l'entrée « Paramètres » est visible dès qu'UNE section de la zone
 * est accessible au rôle (gérant : routeurs, portail, modèles,
 * notifications ; propriétaire : toutes). */
export function navItemsFor(
  role: string | undefined,
  isAdmin: boolean,
  mode: "platform" | "client" = "client",
): NavItem[] {
  const sections = mode === "platform" ? NAV_PLATFORM_SECTIONS : NAV_SECTIONS;
  return sections
    .flatMap((s) => s.items)
    .filter((item) =>
      item.id === "settings"
        ? settingsSectionsFor(role).length > 0
        : (item.id !== "accounts" || isAdmin) && canView(role, item.id),
    );
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
