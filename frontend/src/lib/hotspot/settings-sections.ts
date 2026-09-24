// N°57 — Zone Paramètres : regroupement des vues back-office.
//
// Le gérant reste concentré sur les modules métier (ventes, vouchers,
// sessions, utilisateurs…) : les vues de configuration quittent la
// navigation principale et vivent désormais sous /app/settings/<section>,
// rendues dans une sidebar dédiée qui REMPLPLACE la navigation principale
// quand la zone est active (N°57-c — bouton Retour + sections,
// components/hotspot/settings/settings-shell.tsx).
//
// N°57-d — réorganisation experte des sections : Général / Hotspot /
// Sécurité / Routeurs / Notifications / Équipe. L'ancienne vue « Paramètres »
// (3 onglets internes) est ÉCLATÉE : Général et Sécurité deviennent des
// sections à part entière, la section Hotspot devient un HUB à onglets
// intégrant le Portail et les Modèles (pattern N°30 « users/registrations →
// hub » : chaque onglet reste un ViewId deep-linkable, /app/settings/hotspot/
// <portail|modeles>). Le clic sur une section ouvre sa vue racine
// (section.id) ; le surlignage actif couvre TOUTES les vues de la section
// (views) — la section Hotspot reste active sur ses deux onglets.
//
// N°112 — « Routeurs » QUITTE la zone et retourne dans la navigation
// principale (section Infrastructure, lib/hotspot/nav.ts) : le parc matériel
// est une préoccupation opérationnelle quotidienne, pas un réglage
// back-office. La zone se recentre sur l'identité et la gouvernance de
// l'espace — 6 sections : Général / Hotspot / Sécurité / Notifications /
// Abonnement / Équipe. L'URL /app/settings/routers reste deep-linkable
// (LEGACY_SLUG_VIEWS → re-normalisée vers /app/routers).
//
// Le contrat des vues est INCHANGÉ : chaque section conserve ses ViewIds
// (store, rôles, map VIEWS de l'app-shell) — seuls les chemins canoniques
// changent (view-path.ts). Les URLs historiques (/app/templates,
// /app/settings/portal…) restent deep-linkables : la normalisation
// d'app-route réécrit vers le chemin canonique.
//
// N°57-e — l’Abonnement rejoint la zone : la facturation de l’espace
// (formule, échéance, renouvellement, factures) vit désormais
// sous /app/settings/subscription, comme les autres préoccupations
// back-office. L’ancienne vue racine /app/subscription reste deep-linkable
// (re-normalisation app-route). Position : AVANT Équipe — l’ordre se lit
// « identité → service → sécurité → alertes → facturation → équipe » ; le
// gérant (rang 2, lecture seule du GET /api/subscription) conserve son
// atterrissage Hotspot (la section ne devient jamais SA première section
// accessible).
//
// N°184 — fusion « Expérience → Portail » : le hub Hotspot passe à DEUX
// onglets. La chaîne de personnalisation du portail captif (N°182 :
// ROUTEUR → SITE → COMPTE) vivait coupée en deux — les surcharges site/
// routeur dans l’onglet Portail, la base COMPTE dans l’onglet Expérience.
// Désormais l’onglet Portail porte la chaîne ENTIÈRE (portail du compte
// [rang 3, masqué au gérant] puis sites puis routeurs), et l’onglet
// « Vouchers & tickets » (ex-Modèles) regroupe la politique des tickets
// (expiration, import auto, DNS/logo/QR — ex-Expérience, rang 3 masqué)
// avec les gabarits d’impression. La racine de section devient la vue
// « portal » — le gérant y atterrissait déjà (l’Expérience lui était
// masquée) ; l’ancienne URL /app/settings/hotspot reste deep-linkable
// (LEGACY_SLUG_VIEWS → portal).

import type { LucideIcon } from "lucide-react";
import { Bell, CreditCard, Settings, ShieldCheck, UsersRound, Wifi } from "lucide-react";
import { canView } from "./roles";
import type { ViewId } from "./types";

/** Usage du compte de session (N°100) — la zone Paramètres est PARTAGÉE par
 * les deux consoles, mais sa composition change : la section Hotspot
 * (hub portail/vouchers & tickets) est le produit des ÉTABLISSEMENTS — un
 * foyer n'a ni portail captif ni gabarits de vouchers, ses endpoints sont
 * d'ailleurs 404 pour lui (requireUsage, N°98). */
type Usage = "hotspot" | "homenet" | "" | undefined;

/** Section de la zone Paramètres — une entrée de la sidebar de zone.
 * `views` regroupe les vues rendues DANS la section (onglets du hub Hotspot
 * inclus) : surlignage actif + filtrage par rôle couvrent le groupe entier.
 * N°100 — `hotspotOnly` : section réservée à la console hotspot (produit
 * des établissements) — absente de la zone d'un compte homenet. */
export interface SettingsSection {
  /** Vue racine de la section — destination du clic dans la sidebar. */
  id: ViewId;
  /** Clé i18n du libellé (réutilise les clés existantes : zéro traduction neuve). */
  labelKey: string;
  icon: LucideIcon;
  /** Toutes les vues de la section (surlignage actif, zone, garde-fous). */
  views: readonly ViewId[];
  /** N°100 — la section n'existe que pour les comptes hotspot (défaut non). */
  hotspotOnly?: boolean;
}

/** Sections de la zone, dans l'ordre de la sidebar (N°57-d) : identité →
 * service WiFi → sécurité → alertes → facturation → équipe (N°57-e :
 * Abonnement avant Équipe ; N°112 : Routeurs parti en navigation
 * principale). L'accès suit VIEW_MIN_RANK (rôles existants) : « Général »,
 * « Sécurité » et « Équipe » restent propriétaire (rang 3) ; « Hotspot »
 * (hors onglet Expérience, masqué au gérant) et « Notifications » sont
 * gérant+ (rang 2) ; « Abonnement » est en lecture pour tous les rôles
 * authentifiés (GET /api/subscription sans restriction serveur — les
 * ACTIONS de renouvellement/paiement restent rang 3, gardées côté Go). */
export const SETTINGS_SECTIONS: readonly SettingsSection[] = [
  { id: "settings", labelKey: "settings.tabGeneral", icon: Settings, views: ["settings"] },
  // N°100 — section marquée « hotspot » : cachée aux comptes homenet
  // (canView refuse déjà portal/templates pour ces comptes — ce marqueur
  // ne fait qu'écourter le filtrage, la barrière réelle est VIEW_MIN_RANK +
  // HOMENET_VIEWS dans roles.ts).
  // N°184 — racine « portal » (fusion Expérience → Portail) : la chaîne
  // ROUTEUR → SITE → COMPTE vit dans UN onglet, les tickets dans l'autre.
  { id: "portal", labelKey: "settings.tabHotspot", icon: Wifi, views: ["portal", "templates"], hotspotOnly: true },
  { id: "security", labelKey: "settings.tabAdvanced", icon: ShieldCheck, views: ["security"] },
  // N°112 — plus de section « routers » ici : la vue Routeurs vit en
  // navigation principale (section Infrastructure de nav.ts, /app/routers).
  { id: "notifications", labelKey: "nav.notifications", icon: Bell, views: ["notifications"] },
  // N°57-e — Abonnement : formule, échéance, renouvellement, factures.
  { id: "subscription", labelKey: "nav.subscription", icon: CreditCard, views: ["subscription"] },
  { id: "team", labelKey: "nav.team", icon: UsersRound, views: ["team"] },
];

/** ViewIds appartenant à la zone Paramètres (toutes sections confondues). */
const ZONE_VIEWS: ReadonlySet<string> = new Set<string>(
  SETTINGS_SECTIONS.flatMap((s) => s.views),
);

/** Cette vue vit-elle dans la zone Paramètres ? */
export function isSettingsView(view: string): boolean {
  return ZONE_VIEWS.has(view);
}

/** Sections visibles pour ce rôle (miroir canView — l'UI masque ce que le
 * serveur refuserait de toute façon, pattern N°7). Une section apparaît dès
 * qu'UNE de ses vues est accessible : le gérant voit « Hotspot » (onglets
 * Portail et Vouchers & tickets — les formulaires du compte, rang 3, y sont
 * simplement masqués), jamais « Général » ni « Sécurité ». N°100 — `usage`
 * retire la section Hotspot de la zone d'un foyer (les vues portal/templates
 * lui sont de toute façon refusées par canView : la section serait vide). */
export function settingsSectionsFor(role: string | undefined, usage: Usage = "hotspot"): SettingsSection[] {
  return SETTINGS_SECTIONS.filter(
    (s) => !(usage === "homenet" && s.hotspotOnly) && s.views.some((v) => canView(role, v, usage)),
  );
}

/** Première section autorisée pour ce rôle — destination du fallback quand
 * une section interdite est demandée directement (lien direct, rechargement
 * après changement de rôle). null = aucune section accessible. N°100 — la
 * destination respecte l'usage. N°112 — sans la section Routeurs (partie en
 * navigation principale), le gérant d'un foyer atterrit sur « Notifications »,
 * le gérant d'un établissement sur le hub Hotspot — N°184 : la racine de la
 * section est désormais l'onglet Portail, où le gérant atterrissait déjà. */
export function firstSettingsView(role: string | undefined, usage: Usage = "hotspot"): ViewId | null {
  return settingsSectionsFor(role, usage)[0]?.id ?? null;
}

/** Destination des entrées « Paramètres » (sidebar, palette, menus profil) :
 * le propriétaire atterrit sur Général, le gérant sur sa première section
 * accessible (rang 2 — Hotspot, onglet Portail depuis N°184) — jamais sur
 * une vue que le serveur refuserait (403). N°100 — pour un foyer (gérant ou
 * propriétaire) la première section est toujours saine (Notifications pour
 * le gérant, Général pour le propriétaire — jamais le hub Hotspot). */
export function settingsLandingView(role: string | undefined, usage: Usage = "hotspot"): ViewId {
  return firstSettingsView(role, usage) ?? "settings";
}
