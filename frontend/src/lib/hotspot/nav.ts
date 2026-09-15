// Configuration de navigation partagée — sidebar (app-shell) et palette de
// recherche (topbar) rendent les mêmes sections pour rester cohérents.

import type { LucideIcon } from "lucide-react";
import {
  BarChart3,
  Building2,
  Gauge,
  LayoutDashboard,
  MonitorSmartphone,
  ReceiptText,
  Radio,
  Router as RouterIcon,
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
  // N°57-g — réorganisation experte de la navigation métier : catégories
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
  //     qui se connectent, revendeurs qui écoulent).
  //
  // N°112 — « Routeurs » quitte la zone Paramètres et rouvre une section
  // Infrastructure DANS la navigation principale (entre Personnes et
  // Analyse). Retour terrain : le gérant vit sur son parc matériel — état
  // en ligne/hors ligne, outils QoS/pool/scheduler, installation agent —
  // et l'exiler derrière le menu utilisateur (N°57) l'enterrait à deux
  // clics du tableau de bord qui pourtant LUI parle des routeurs (cartes
  // d'état, check-in agent). La sidebar se relit en deux pôles : le FRONT
  // métier (Supervision, Hotspot, Personnes — ce qu'on fait chaque jour)
  // puis le DOS technique (Infrastructure, Analyse — la machine et l'audit).
  // La clé « nav.section.infrastructure » existait déjà depuis le pré-N°57
  // (FR/EN) : zéro traduction neuve. Le Portail, lui, reste en zone
  // Paramètres (onglet du hub Hotspot — expérience de marque, pas parc
  // matériel). Chaque groupe reste repliable (row expandable O).
  {
    labelKey: "nav.section.supervision",
    items: [
      { id: "dashboard", labelKey: "nav.dashboard", icon: LayoutDashboard },
      { id: "sessions", labelKey: "nav.sessions", icon: Radio },
      // N°83 — la protection sort de la zone Paramètres : les modules
      // sécurité (SafeWiFi/Shield/FamilyGuard) sont des arguments de vente,
      // pas des réglages — ils vivent à 1 clic du tableau de bord.
      { id: "protection", labelKey: "nav.protection", icon: ShieldCheck },
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
  // N°112 — section Infrastructure : LE parc matériel qui porte le service.
  // Restauration partielle du pré-N°57 (Routeurs + Portail) : seuls les
  // Routeurs reviennent — le Portail reste un onglet du hub Hotspot en
  // zone Paramètres. Position : après le pôle métier, avant l'audit — la
  // machine vit avec l'analyse dans le dos technique de la sidebar.
  {
    labelKey: "nav.section.infrastructure",
    items: [{ id: "routers", labelKey: "nav.routers", icon: RouterIcon }],
  },
  // N°57 — le Portail vit dans la zone Paramètres (onglet du hub Hotspot,
  // /app/settings/portal) — expérience de marque, pas navigation métier.
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
 * plateforme quand l'admin y bascule, MAISON quand le compte est homenet
 * (miroir canView dans les trois cas — même usage, même barrière).
 * N°57-f — la zone Paramètres n'y figure plus : son accès unique est le
 * menu utilisateur (settingsLandingView adapte la destination au rôle ET
 * à l'usage). */
export function navItemsFor(
  role: string | undefined,
  isAdmin: boolean,
  mode: "platform" | "client",
  usage: "hotspot" | "homenet" | "" | undefined = "hotspot",
): NavItem[] {
  const sections =
    mode === "platform"
      ? NAV_PLATFORM_SECTIONS
      : usage === "homenet"
        ? NAV_HOMENET_SECTIONS
        : NAV_SECTIONS;
  return sections
    .flatMap((s) => s.items)
    .filter((item) => (item.id !== "accounts" || isAdmin) && canView(role, item.id, usage));
}

/**
 * N°100 — navigation de la CONSOLE HOMENET (foyer). Une seule section
 * « Votre maison », trois items : l'histoire du produit en un regard —
 * superviser (tableau de bord), voir qui est connecté (appareils), garder
 * la famille tranquille (protection). C'est TOUT ce qu'un foyer a besoin
 * de voir en navigation principale : le MVP mince voulu par la décision
 * produit — pas un deuxième produit complet. Les réglages vivent dans la
 * zone Paramètres (menu utilisateur), la facturation dans l'Abonnement
 * de cette même zone — partagés avec la console hotspot, SANS la section
 * Hotspot (produit des établissements : hub, portail, modèles).
 */
export const NAV_HOMENET_SECTIONS: { labelKey: string; items: NavItem[] }[] = [
  {
    labelKey: "nav.section.home",
    items: [
      { id: "home", labelKey: "nav.home", icon: LayoutDashboard },
      { id: "devices", labelKey: "nav.devices", icon: MonitorSmartphone },
      // La Protection est L'argument massue du foyer (FamilyGuard, couvre-feu,
      // SafeWiFi famille) — elle vit à un clic du tableau de bord maison.
      { id: "protection", labelKey: "nav.protection", icon: ShieldCheck },
      // N°112 — « Routeurs » suit le retour en navigation principale (même
      // décision produit que la console hotspot) : le routeur du foyer
      // n'était atteignable qu'en zone Paramètres — un compte à rebours de
      // clics pour LA boîte qui porte toute la maison. Append en fin de
      // section : l'histoire émotionnelle (superviser → voir qui est
      // connecté → protéger la famille) reste d'abord, le matériel ferme
      // la marche.
      { id: "routers", labelKey: "nav.routers", icon: RouterIcon },
    ],
  },
];

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
      // N°117 — parc routeurs global : chaque routeur de chaque compte client,
      // version installée vs disponible, vérification et mise à jour RouterOS
      // de flotte (le geste super-admin, miroir multi-comptes du N°115).
      { id: "platformFleet", labelKey: "nav.platformFleet", icon: RouterIcon },
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
