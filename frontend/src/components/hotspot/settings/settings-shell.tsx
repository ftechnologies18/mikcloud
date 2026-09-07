"use client";

// N°57 — Shell de la zone Paramètres (split-view).
//
// Layout : sidebar de sections à gauche (desktop ≥ lg) + panneau de contenu
// à droite ; sur mobile/tablette la sidebar devient un bandeau horizontal
// défilant de sections. Le panneau rend la vue active du store SANS LA
// MODIFIER : chaque vue apporte son PageHeader (titre + description), son
// chargement différé (map VIEWS de l'app-shell) et ses données — le contrat
// des vues est inchangé.
//
// Style minimaliste épuré, adapté aux TOKENS du projet (shadcn/oklch —
// mêmes variables que le reste de la console) : bordures fines, pas de fond
// imposé sur le panneau, deux graisses seulement (normal/medium), accents
// réservés à l'état actif (icône primaire). Aucune dépendance externe.

import { useI18n } from "@/lib/hotspot/i18n";
import { settingsSectionsFor } from "@/lib/hotspot/settings-sections";
import { useHotspotStore } from "@/lib/hotspot/store";
import type { ViewId } from "@/lib/hotspot/types";
import { cn } from "@/lib/utils";

/** Sidebar desktop — sections de la zone, item actif = vue courante du
 * store (la navigation pousse l'URL via la synchro store → URL d'app-route,
 * exactement comme la sidebar principale : pas de <Link>, même mécanique). */
function SettingsSidebar({ activeId }: { activeId: ViewId }) {
  const { t } = useI18n();
  const setView = useHotspotStore((s) => s.setView);
  const user = useHotspotStore((s) => s.user);
  const sections = settingsSectionsFor(user?.role);
  if (sections.length === 0) return null;
  return (
    <nav aria-label={t("nav.settings")} className="hidden w-56 shrink-0 lg:block">
      {/* Sticky : la liste suit le défilement du panneau (contenu long des
          sections Modèles/Routeurs) ; bornée au viewport, défilement propre
          si l'écran est court. */}
      <div className="lg:sticky lg:top-20 lg:max-h-[calc(100vh-6.5rem)] lg:overflow-y-auto">
        <p className="px-3 pb-2 text-xs font-medium uppercase tracking-wider text-muted-foreground/70">
          {t("nav.settings")}
        </p>
        <ul className="space-y-0.5">
          {sections.map((section) => {
            const active = section.id === activeId;
            return (
              <li key={section.id}>
                <button
                  type="button"
                  onClick={() => setView(section.id)}
                  aria-current={active ? "page" : undefined}
                  className={cn(
                    "flex min-h-11 w-full items-center gap-3 rounded-lg px-3 py-2 text-sm outline-none transition-colors duration-150",
                    "focus-visible:ring-2 focus-visible:ring-ring",
                    active
                      ? "bg-muted font-medium text-foreground"
                      : "font-normal text-muted-foreground hover:bg-muted/60 hover:text-foreground",
                  )}
                >
                  <section.icon
                    className={cn("size-4.5 shrink-0", active ? "text-primary" : "text-muted-foreground")}
                    aria-hidden
                  />
                  <span className="truncate text-left">{t(section.labelKey)}</span>
                </button>
              </li>
            );
          })}
        </ul>
      </div>
    </nav>
  );
}

/** Bandeau horizontal défilant des sections — mobile/tablette (< lg) :
 * remplace la sidebar (cible tactile ≥ 44 px, défilement latéral sans wrap). */
function SettingsTabsRow({ activeId }: { activeId: ViewId }) {
  const { t } = useI18n();
  const setView = useHotspotStore((s) => s.setView);
  const user = useHotspotStore((s) => s.user);
  const sections = settingsSectionsFor(user?.role);
  if (sections.length === 0) return null;
  return (
    <div role="navigation" aria-label={t("nav.settings")} className="lg:hidden">
      <p className="px-1 pb-2 text-xs font-medium uppercase tracking-wider text-muted-foreground/70">
        {t("nav.settings")}
      </p>
      <div className="-mx-1 flex gap-1.5 overflow-x-auto px-1 pb-1">
        {sections.map((section) => {
          const active = section.id === activeId;
          return (
            <button
              key={section.id}
              type="button"
              onClick={() => setView(section.id)}
              aria-current={active ? "page" : undefined}
              className={cn(
                "flex min-h-11 shrink-0 items-center gap-2 rounded-full border px-4 py-2 text-sm outline-none transition-colors duration-150",
                "focus-visible:ring-2 focus-visible:ring-ring",
                active
                  ? "border-border bg-muted font-medium text-foreground"
                  : "border-border/60 bg-transparent font-normal text-muted-foreground hover:bg-muted/50 hover:text-foreground",
              )}
            >
              <section.icon
                className={cn("size-4 shrink-0", active ? "text-primary" : "text-muted-foreground")}
                aria-hidden
              />
              <span className="whitespace-nowrap">{t(section.labelKey)}</span>
            </button>
          );
        })}
      </div>
    </div>
  );
}

/** Shell split-view — enveloppe la vue active de la zone Paramètres.
 * Le children (transition + vue) est fourni par l'app-shell : ce composant
 * est un pur layout, il ne remonte PAS au changement de section — seule la
 * zone de contenu rejoue l'animation, la sidebar reste stable. */
export function SettingsShell({ children }: { children: React.ReactNode }) {
  const view = useHotspotStore((s) => s.view);
  return (
    <div className="flex flex-col gap-6 lg:flex-row lg:gap-0">
      <SettingsSidebar activeId={view} />
      <SettingsTabsRow activeId={view} />
      {/* Panneau — bordure fine gauche (desktop) pour structurer, pas de fond
          imposé ; largeur bornée pour la lisibilité des formulaires tout en
          laissant respirer les grilles de cartes (Modèles, Routeurs). */}
      <div className="min-w-0 max-w-5xl flex-1 lg:border-l lg:border-border/60 lg:pl-8">
        {children}
      </div>
    </div>
  );
}
