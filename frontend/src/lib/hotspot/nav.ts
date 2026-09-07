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
import type { ViewId } from "./types";

export interface NavItem {
  id: ViewId;
  labelKey: string;
  icon: LucideIcon;
}

export const NAV_SECTIONS: { labelKey: string; items: NavItem[] }[] = [
  // N°57-g — réorganisation experte de la navigation métier : 4 catégories
  // homogènes dont l'ordre suit le parcours d'usage (surveiller → vendre
  // l'accès → gérer les gens → analyser). L'ancienne section
  // « Exploitation » (5 items : fourre-tout mêlant supervision, clients et
  // produit) et « Facturation & Ventes » (libellé mensonger depuis N°57-e :
  // plus de facturation dans la nav, et le WiFi jetable n'est pas un canal
  // de vente mais un mode d'accès) disparaissent au profit de :
  //   • Supervision — le temps réel (tableau de bord, sessions actives) ;
  //   • Hotspot — LE produit : les trois façons de délivrer de l'accès
  //     (voucher prépayé, profil/forfait, WiFi jetable offert) ;
  //   • Personnes — les deux annuaires humains du business (clients finaux
  //     qui se connectent, revendeurs qui écoulent) ;
  //   • Analyse — comprendre et auditer (rapports, journal, comptes SaaS).
  // Chaque groupe reste repliable (row expandable O).
  {
    labelKey: "nav.section.supervision",
    items: [
      { id: "dashboard", labelKey: "nav.dashboard", icon: LayoutDashboard },
      { id: "sessions", labelKey: "nav.sessions", icon: Radio },
    ],
  },
  {
    labelKey: "nav.section.hotspot",
    items: [
      { id: "vouchers", labelKey: "nav.vouchers", icon: Ticket },
      // N°30 — les inscriptions publiques (N°27) sont fusionnées dans la
      // page Utilisateurs (onglet) : l'entrée dédiée disparaît de la
      // sidebar. ViewId « registrations » reste valide (URL
      // /app/registrations → hub Utilisateurs, onglet Inscriptions).
      // N°57 — « Modèles » vit désormais dans la zone Paramètres
      // (/app/settings/templates) : la sidebar ne montre plus que les
      // modules métier.
      { id: "profiles", labelKey: "nav.profiles", icon: Gauge },
      // N°27 — WiFi jetable : mode d'accès offert des établissements.
      { id: "wifi", labelKey: "wifi.title", icon: Wifi },
    ],
  },
  {
    labelKey: "nav.section.people",
    items: [
      { id: "users", labelKey: "nav.users", icon: Users },
      // N°57-e — l'Abonnement quitte la navigation principale : la
      // facturation de l'espace vit désormais dans la zone Paramètres
      // (/app/settings/subscription, section dédiée de la sidebar de zone).
      // L'ancienne URL /app/subscription reste deep-linkable (re-normalisée).
      { id: "resellers", labelKey: "nav.resellers", icon: Store },
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
  // N°57-f — la section « Système » disparaît : l'entrée « Paramètres »
  // était un doublon du menu utilisateur (UserCard / menu profil topbar),
  // qui reste LE point d'entrée de la zone. La sidebar ne montre plus que
  // les modules métier ; la zone Paramètres vit par substitution de
  // sidebar (N°57-c) une fois ouverte (User menu, URL directe, bandeau
  // d'abonnement du dashboard).
];

/** Liste plate des items de nav pour la console ACTIVE — client par défaut,
 * plateforme quand l'admin y bascule (miroir canView dans les deux cas).
 * N°57-f — la zone Paramètres n'y figure plus : son accès unique est le
 * menu utilisateur (settingsLandingView adapte la destination au rôle). */
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
