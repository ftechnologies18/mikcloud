"use client";

// N°57-b — Shell de la zone Paramètres (sub-nav HORIZONTALE).
//
// Correction du retour utilisateur sur N°57 : la zone portait une sidebar
// verticale interne (split-view) qui empilait DEUX navigations latérales —
// sidebar principale de l'app + sidebar de la zone = 3 colonnes. Pattern
// « sidebar dans sidebar » (anti-pattern UX) : les deux niveaux se
// concurrencent et la sidebar interne empiète sur l'espace du contenu.
//
// La navigation interne de la zone devient HORIZONTALE : une unique rangée
// de pills (les sections de la zone) sous un petit libellé « Paramètres »,
// une bordure fine en dessous, puis le contenu occupe TOUTE la largeur
// restante. Une SEULE sidebar dans toute l'application : la principale.
//
// Le contrat des vues reste INCHANGÉ : le panneau rend la vue active du
// store sans la modifier — chaque vue apporte son PageHeader (titre +
// description + actions), son chargement différé (map VIEWS de l'app-shell)
// et ses données ; la section « Paramètres » (vue settings) conserve ses
// sous-onglets internes Général / Hotspot / Avancé.
//
// Style : tokens du projet (shadcn/oklch — primaire vert émeraude). Pill
// active pleine (fond primaire, texte blanc), pills inactives discrètes
// (texte gris, survol léger), bordure fine sous la rangée, deux graisses
// seulement (normal/medium), aucune dépendance externe. Responsive : la
// rangée défile horizontalement (overflow-x-auto, scrollbar fine globale,
// cibles tactiles ≥ 44 px) quand les pills ne tiennent pas dans l'écran.

import { useI18n } from "@/lib/hotspot/i18n";
import { settingsSectionsFor } from "@/lib/hotspot/settings-sections";
import { useHotspotStore } from "@/lib/hotspot/store";
import type { ViewId } from "@/lib/hotspot/types";
import { cn } from "@/lib/utils";

/** Sub-nav horizontale de la zone — pills des sections, défilement latéral
 * quand elles ne tiennent pas (mobile, tablette étroite). L'item actif =
 * vue courante du store (la navigation pousse l'URL via la synchro
 * store → URL d'app-route, exactement comme la sidebar principale : pas de
 * <Link>, même mécanique). */
function SettingsSubNav({ activeId }: { activeId: ViewId }) {
  const { t } = useI18n();
  const setView = useHotspotStore((s) => s.setView);
  const user = useHotspotStore((s) => s.user);
  const sections = settingsSectionsFor(user?.role);
  if (sections.length === 0) return null;
  return (
    <nav aria-label={t("nav.settings")} className="space-y-2">
      {/* Libellé de zone — ancre minimale (« où suis-je ») sans voler le
          titre des vues : chaque vue apporte son propre PageHeader. */}
      <p className="px-1 text-xs font-medium uppercase tracking-wider text-muted-foreground/70">
        {t("nav.settings")}
      </p>
      {/* Bordure fine SOUS la rangée : délimite la navigation du contenu
          (aucun fond imposé) ; min-h-11 = cible tactile ≥ 44 px. */}
      <div className="-mx-1 flex gap-1.5 overflow-x-auto border-b border-border/60 px-1 pb-3">
        {sections.map((section) => {
          const active = section.id === activeId;
          return (
            <button
              key={section.id}
              type="button"
              onClick={() => setView(section.id)}
              aria-current={active ? "page" : undefined}
              className={cn(
                "flex min-h-11 shrink-0 items-center gap-2 rounded-full px-4 py-2 text-sm outline-none transition-colors duration-150",
                "focus-visible:ring-2 focus-visible:ring-ring",
                active
                  ? "bg-primary font-medium text-primary-foreground"
                  : "font-normal text-muted-foreground hover:bg-muted/60 hover:text-foreground",
              )}
            >
              <section.icon
                className={cn("size-4 shrink-0", active ? "text-primary-foreground" : "text-muted-foreground")}
                aria-hidden
              />
              <span className="whitespace-nowrap">{t(section.labelKey)}</span>
            </button>
          );
        })}
      </div>
    </nav>
  );
}

/** Shell de la zone Paramètres — sub-nav horizontale + contenu pleine
 * largeur. Le children (transition + vue) est fourni par l'app-shell : ce
 * composant est un pur layout, il ne se remonte PAS au changement de
 * section — seule la vue rejoue l'animation, la rangée de pills reste
 * stable (pas de clignotement). */
export function SettingsShell({ children }: { children: React.ReactNode }) {
  const view = useHotspotStore((s) => s.view);
  return (
    <div className="space-y-6">
      <SettingsSubNav activeId={view} />
      {children}
    </div>
  );
}
