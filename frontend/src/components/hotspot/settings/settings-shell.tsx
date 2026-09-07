"use client";

// N°57-c — Sidebar de la zone Paramètres : elle REMPLPLACE la sidebar
// principale (jamais côte à côte).
//
// Retour utilisateur sur N°57 (split-view) : la zone portait une sidebar
// interne qui s'empilait sur la sidebar principale = 3 colonnes (« sidebar
// dans sidebar », anti-pattern UX). Le pattern retenu (N°57-c) : quand une
// vue de la zone est active, l'app-shell rend CETTE sidebar À LA PLACE de
// NavList dans le même <aside> — le layout reste TOUJOURS à 2 colonnes
// (sidebar + contenu). Un bouton « Retour » en tête ramène à la navigation
// principale : il rouvre la dernière vue métier visitée (mémorisée par
// l'app-shell), la sidebar principale reprend alors sa place.
//
// N°57-d — la section Hotspot est un HUB : son surlignage actif couvre ses
// trois onglets (hotspot/portail/modèles) — la section reste « allumée »
// pendant toute la navigation interne, exactement comme « Utilisateurs »
// reste actif sur l'onglet Inscriptions (pattern N°30).
//
// Style : mêmes classes que NavList (items `sidebar-nav-item`, état actif
// `nav-active`, icônes lucide) — la substitution est invisible au regard,
// seule la liste change. Aucune dépendance externe.

import { ArrowLeft } from "lucide-react";
import { useI18n } from "@/lib/hotspot/i18n";
import { settingsSectionsFor } from "@/lib/hotspot/settings-sections";
import { useHotspotStore } from "@/lib/hotspot/store";
import { cn } from "@/lib/utils";

export interface SettingsSidebarProps {
  /** Sortie de la zone : rouvre la dernière vue métier visitée (la sidebar
   * principale reprend sa place dans l'app-shell). */
  onBack: () => void;
}

/** Sidebar de la zone Paramètres — bouton Retour + sections (filtrées par
 * rôle). Rendue par l'app-shell DANS le <aside> principal à la place de
 * NavList (desktop) et dans le Sheet mobile (même substitution). L'item
 * actif = vue courante du store ; la navigation pousse l'URL via la synchro
 * store → URL d'app-route, exactement comme la sidebar principale : pas de
 * <Link>, même mécanique. */
export function SettingsSidebar({ onBack }: SettingsSidebarProps) {
  const { t } = useI18n();
  const setView = useHotspotStore((s) => s.setView);
  const view = useHotspotStore((s) => s.view);
  const user = useHotspotStore((s) => s.user);
  const sections = settingsSectionsFor(user?.role);
  if (sections.length === 0) return null;
  return (
    <nav className="flex-1 space-y-1 overflow-y-auto px-3 pb-4" aria-label={t("nav.settings")}>
      {/* Bouton « Retour » — en tête, là où ModeSwitch siège dans la sidebar
          principale : même place, même gabarit (repère stable au swap).
          Mémorisée par l'app-shell, la destination est la dernière vue
          métier visitée avant d'entrer dans la zone. */}
      <button
        type="button"
        onClick={onBack}
        className="sidebar-nav-item relative flex min-h-11 w-full items-center gap-3 rounded-lg px-2.5 py-2 text-sm font-medium text-muted-foreground transition-all duration-200 hover:bg-accent/60 hover:text-foreground"
      >
        <span className="relative flex shrink-0 items-center">
          <ArrowLeft className="size-4.5" aria-hidden />
        </span>
        <span className="flex-1 truncate text-left">{t("settings.zone.back")}</span>
      </button>

      {/* Libellé de zone — ancre « où suis-je » (la liste qui suit remplace
          les sections métier de NavList). */}
      <p className="px-2.5 pb-2 pt-1 text-xs font-medium uppercase tracking-wider text-muted-foreground/70">
        {t("nav.settings")}
      </p>

      <ul className="space-y-0.5">
        {sections.map((section) => {
          // N°57-d — actif sur TOUTES les vues de la section : la section
          // Hotspot reste surlignée sur ses onglets Portail/Modèles.
          const active = section.views.includes(view);
          return (
            <li key={section.id}>
              <button
                type="button"
                onClick={() => setView(section.id)}
                aria-current={active ? "page" : undefined}
                className={cn(
                  "sidebar-nav-item relative flex min-h-11 w-full items-center gap-3 rounded-lg px-2.5 py-2 text-sm font-medium transition-all duration-200",
                  active ? "nav-active" : "text-muted-foreground hover:bg-accent/60 hover:text-foreground",
                )}
              >
                <span className="relative flex shrink-0 items-center">
                  <section.icon className="size-4.5" aria-hidden />
                </span>
                <span className="flex-1 truncate text-left">{t(section.labelKey)}</span>
              </button>
            </li>
          );
        })}
      </ul>
    </nav>
  );
}
