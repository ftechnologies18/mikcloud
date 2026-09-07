// N°57 — Zone Paramètres : regroupement des vues back-office.
//
// Le gérant reste concentré sur les modules métier (ventes, vouchers,
// sessions, utilisateurs…) : les vues de configuration quittent la
// navigation principale et vivent désormais sous /app/settings/<section>,
// rendues dans un shell dédié (sub-nav horizontale de sections + contenu
// pleine largeur — N°57-b, components/hotspot/settings/settings-shell.tsx).
//
// Le contrat des vues est INCHANGÉ : chaque section conserve son ViewId
// (store, rôles, map VIEWS de l'app-shell) — seul le chemin canonique
// change (view-path.ts). Les URLs historiques (/app/templates…) restent
// deep-linkables : la normalisation d'app-route réécrit vers le chemin
// canonique (pattern N°30 « registrations → hub Utilisateurs »).

import type { LucideIcon } from "lucide-react";
import { Bell, Monitor, Printer, Router as RouterIcon, Settings, UsersRound } from "lucide-react";
import { canView } from "./roles";
import type { ViewId } from "./types";

/** Section de la zone Paramètres — une section = une vue existante. */
export interface SettingsSection {
  /** Vue rendue dans le panneau (ViewId du store — contrat inchangé). */
  id: ViewId;
  /** Clé i18n du libellé (réutilise les clés nav existantes : zéro traduction neuve). */
  labelKey: string;
  icon: LucideIcon;
}

/** Sections de la zone, dans l'ordre de la sidebar : configuration de base →
 * infrastructure → impression → alertes → équipe. L'accès de chaque section
 * suit VIEW_MIN_RANK (rôles existants) : « Général » et « Équipe » restent
 * propriétaire (rang 3), les autres gérant+ (rang 2). */
export const SETTINGS_SECTIONS: readonly SettingsSection[] = [
  { id: "settings", labelKey: "nav.settings", icon: Settings },
  { id: "routers", labelKey: "nav.routers", icon: RouterIcon },
  { id: "portal", labelKey: "nav.portal", icon: Monitor },
  { id: "templates", labelKey: "nav.templates", icon: Printer },
  { id: "notifications", labelKey: "nav.notifications", icon: Bell },
  { id: "team", labelKey: "nav.team", icon: UsersRound },
];

/** ViewIds appartenant à la zone Paramètres. */
const ZONE_VIEWS: ReadonlySet<string> = new Set<string>(SETTINGS_SECTIONS.map((s) => s.id));

/** Cette vue vit-elle dans la zone Paramètres ? */
export function isSettingsView(view: string): boolean {
  return ZONE_VIEWS.has(view);
}

/** Sections visibles pour ce rôle (miroir canView — l'UI masque ce que le
 * serveur refuserait de toute façon, pattern N°7). */
export function settingsSectionsFor(role: string | undefined): SettingsSection[] {
  return SETTINGS_SECTIONS.filter((s) => canView(role, s.id));
}

/** Première section autorisée pour ce rôle — destination du fallback quand
 * une section interdite est demandée directement (lien direct, rechargement
 * après changement de rôle). null = aucune section accessible. */
export function firstSettingsView(role: string | undefined): ViewId | null {
  return settingsSectionsFor(role)[0]?.id ?? null;
}

/** Destination des entrées « Paramètres » (sidebar, palette, menus profil) :
 * le propriétaire atterrit sur la racine de la zone (vue Général), le gérant
 * sur sa première section accessible (rang 2) — jamais sur une vue que le
 * serveur refuserait (403). */
export function settingsLandingView(role: string | undefined): ViewId {
  return firstSettingsView(role) ?? "settings";
}
