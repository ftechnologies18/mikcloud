"use client";

// N°191 — « Loupe routeur » : rail de sélection de portée partagé par les
// vues Sessions et Vouchers. UN composant, DEUX sémantiques de compte (les
// badges vivent par la vue via `counts` : clients connectés côté Sessions,
// stock vivant côté Vouchers) — la portée ELLE-MÊME vit dans l'URL de la vue
// (« router:<id> », pattern Protection N°83 / Portail N°190) : ce composant
// ne fait que rendre et cliquer, il ne connaît ni l'URL ni les requêtes.
//
// Règles :
// - parc < 2 routeurs : le rail disparaît (un seul point d'accès = la vue
//   EST déjà sa loupe, un filtre qui ne filtre rien) ;
// - parc en chargement : squelette de chips (géométrie stable, pas de saut) ;
// - chips = boutons aria-pressed dans un groupe labellisé (clavier natif) ;
// - point de statut : en ligne = chart-1 pulsant, hors ligne = destructive
//   (mêmes couleurs que StatusBadge, conventions de la console) ;
// - badges plafonnés à « 99+ » (le rail reste compact, le compte exact vit
//   dans l'infobulle `countTitle`).

import { Router } from "lucide-react";

import { useI18n } from "@/lib/hotspot/i18n";
import { cn } from "@/lib/utils";
import type { RouterDevice } from "@/lib/hotspot/types";

export interface RouterScopeRailProps {
  /** Parc à afficher — undefined = chargement (squelette) ; < 2 = rail masqué. */
  routers: RouterDevice[] | undefined;
  /** Routeur sélectionné ("" = tous les routeurs). */
  value: string;
  /** Nouvelle portée ("" = retour à tous) — la vue décide où elle vit (URL). */
  onChange: (routerId: string) => void;
  /** Comptes vivants par routeur (badge) — sémantique propre à la vue ;
   *  undefined = pas de badges (comptes non calculables). */
  counts?: Record<string, number>;
  /** Compte du chip « tous » (même sémantique que `counts`). */
  total?: number;
  /** Infobulle d'un badge — name = null pour le chip « tous ». Chaque vue
   *  formule sa sémantique (« sessions actives » vs « tickets actifs »). */
  countTitle: (name: string | null, count: number) => string;
}

/** Badge compact : plafonné à 99+, largeur stable (tabular + min-w). */
function badgeText(n: number): string {
  return n > 99 ? "99+" : String(n);
}

export function RouterScopeRail({ routers, value, onChange, counts, total, countTitle }: RouterScopeRailProps) {
  const { t } = useI18n();

  // Un seul point d'accès (ou zéro) : rien à choisir — la vue reste telle
  // quelle (une loupe sur l'unique routeur n'apporte aucune information).
  if (routers !== undefined && routers.length < 2) return null;

  const chip =
    "inline-flex h-10 shrink-0 snap-start items-center gap-2 rounded-full border px-3.5 text-sm font-medium transition-colors";
  const badge = (selected: boolean) =>
    cn(
      "min-w-6 rounded-full px-1.5 py-0 text-center text-[11px] font-semibold leading-5 tabular-nums",
      selected ? "bg-primary-foreground/15 text-primary-foreground" : "bg-muted text-muted-foreground",
    );

  return (
    <div
      role="group"
      aria-label={t("common.routerScopeLabel")}
      className="flex snap-x items-center gap-2 overflow-x-auto pb-1"
    >
      {routers === undefined ? (
        // Parc en chargement — trois chips fantômes gardent la hauteur.
        <>
          <span className="h-10 w-24 shrink-0 animate-pulse rounded-full bg-muted" aria-hidden />
          <span className="h-10 w-36 shrink-0 animate-pulse rounded-full bg-muted" aria-hidden />
          <span className="h-10 w-28 shrink-0 animate-pulse rounded-full bg-muted" aria-hidden />
        </>
      ) : (
        <>
          <button
            type="button"
            aria-pressed={value === ""}
            title={total !== undefined ? countTitle(null, total) : undefined}
            onClick={() => onChange("")}
            className={cn(
              chip,
              value === ""
                ? "border-primary bg-primary text-primary-foreground"
                : "bg-background text-foreground hover:bg-muted",
            )}
          >
            <Router className="size-4 shrink-0 opacity-80" aria-hidden />
            {t("common.allRouters")}
            {total !== undefined && <span className={badge(value === "")}>{badgeText(total)}</span>}
          </button>
          {routers.map((device) => {
            const selected = device.id === value;
            const count = counts?.[device.id];
            return (
              <button
                key={device.id}
                type="button"
                aria-pressed={selected}
                title={count !== undefined ? countTitle(device.name, count) : undefined}
                onClick={() => onChange(device.id)}
                className={cn(
                  chip,
                  selected
                    ? "border-primary bg-primary text-primary-foreground"
                    : "bg-background text-foreground hover:bg-muted",
                )}
              >
                <span
                  className={cn(
                    "size-2 shrink-0 rounded-full",
                    device.status === "online" ? "animate-pulse bg-chart-1" : "bg-destructive",
                  )}
                  aria-hidden
                />
                <span className="max-w-40 truncate">{device.name}</span>
                {count !== undefined && <span className={badge(selected)}>{badgeText(count)}</span>}
              </button>
            );
          })}
        </>
      )}
    </div>
  );
}
