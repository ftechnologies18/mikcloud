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
// (views) — la section Hotspot reste active sur ses trois onglets.
//
// Le contrat des vues est INCHANGÉ : chaque section conserve ses ViewIds
// (store, rôles, map VIEWS de l'app-shell) — seuls les chemins canoniques
// changent (view-path.ts). Les URLs historiques (/app/templates,
// /app/settings/portal…) restent deep-linkables : la normalisation
// d'app-route réécrit vers le chemin canonique.

import type { LucideIcon } from "lucide-react";
import { Bell, Router as RouterIcon, Settings, ShieldCheck, UsersRound, Wifi } from "lucide-react";
import { canView } from "./roles";
import type { ViewId } from "./types";

/** Section de la zone Paramètres — une entrée de la sidebar de zone.
 * `views` regroupe les vues rendues DANS la section (onglets du hub Hotspot
 * inclus) : surlignage actif + filtrage par rôle couvrent le groupe entier. */
export interface SettingsSection {
  /** Vue racine de la section — destination du clic dans la sidebar. */
  id: ViewId;
  /** Clé i18n du libellé (réutilise les clés existantes : zéro traduction neuve). */
  labelKey: string;
  icon: LucideIcon;
  /** Toutes les vues de la section (surlignage actif, zone, garde-fous). */
  views: readonly ViewId[];
}

/** Sections de la zone, dans l'ordre de la sidebar (N°57-d) : identité →
 * service WiFi → sécurité → infrastructure → alertes → équipe. L'accès
 * suit VIEW_MIN_RANK (rôles existants) : « Général », « Sécurité » et
 * « Équipe » restent propriétaire (rang 3) ; « Hotspot » (hors onglet
 * Expérience, masqué au gérant), « Routeurs » et « Notifications » sont
 * gérant+ (rang 2). */
export const SETTINGS_SECTIONS: readonly SettingsSection[] = [
  { id: "settings", labelKey: "settings.tabGeneral", icon: Settings, views: ["settings"] },
  { id: "hotspot", labelKey: "settings.tabHotspot", icon: Wifi, views: ["hotspot", "portal", "templates"] },
  { id: "security", labelKey: "settings.tabAdvanced", icon: ShieldCheck, views: ["security"] },
  { id: "routers", labelKey: "nav.routers", icon: RouterIcon, views: ["routers"] },
  { id: "notifications", labelKey: "nav.notifications", icon: Bell, views: ["notifications"] },
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
 * Portail/Modèles, l'Expérience rang 3 y est simplement masquée), jamais
 * « Général » ni « Sécurité ». */
export function settingsSectionsFor(role: string | undefined): SettingsSection[] {
  return SETTINGS_SECTIONS.filter((s) => s.views.some((v) => canView(role, v)));
}

/** Première section autorisée pour ce rôle — destination du fallback quand
 * une section interdite est demandée directement (lien direct, rechargement
 * après changement de rôle). null = aucune section accessible. */
export function firstSettingsView(role: string | undefined): ViewId | null {
  return settingsSectionsFor(role)[0]?.id ?? null;
}

/** Destination des entrées « Paramètres » (sidebar, palette, menus profil) :
 * le propriétaire atterrit sur Général, le gérant sur sa première section
 * accessible (rang 2 — Hotspot) — jamais sur une vue que le serveur
 * refuserait (403). */
export function settingsLandingView(role: string | undefined): ViewId {
  return firstSettingsView(role) ?? "settings";
}
