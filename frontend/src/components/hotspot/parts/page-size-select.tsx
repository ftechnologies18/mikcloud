"use client";

// N°193 — Sélecteur de pagination (Page size) : le nombre de résultats
// maximum par page, choisi par l'opérateur et MÉMORISÉ PAR VUE
// (localStorage « mikcloud.pageSize.<vue> »). Une préférence PAR vue, pas
// globale : la densité utile n'est pas la même selon la table — 100 tickets
// d'un coup en gestion de stock, 10 sessions sur un téléphone en tournée.
// Le même composant / la même échelle servent toutes les tables paginées
// de la console (Users, Vouchers, Lots, Journal, Sessions).
//
// Échelle unique [10, 25, 50, 100] : 100 reste sous TOUS les plafonds API
// (users/vouchers 200, lots/journal 100 — queryInt côté backend), donc le
// sélecteur ne peut JAMAIS demander une page que le serveur refuserait en
// silence (un pageSize hors bornes est silencieusement ramené au plafond
// par queryInt : le compte « sur {total} » et les bornes de page seraient
// désynchronisés sans que rien ne rougeoye).
//
// Deux défauts historiques normalisés au passage (documentés CHANGELOG) :
// vouchers 12 → 10, journal 20 → 25 — pour que le défaut de CHAQUE table
// vive dans l'échelle commune (le sélecteur affiche toujours une valeur
// présente dans sa liste).

import { useCallback, useState } from "react";

import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useI18n } from "@/lib/hotspot/i18n";

/** Échelle commune à toutes les tables paginées (voir en-tête de fichier). */
export const PAGE_SIZE_OPTIONS = [10, 25, 50, 100] as const;

const STORAGE_PREFIX = "mikcloud.pageSize.";

/**
 * Taille de page d'une vue, persistée par vue dans le navigateur.
 * La valeur restaurée est VALIDÉE contre l'échelle : une préférence écrite
 * par une version antérieure (ou à la main) hors échelle retombe sur le
 * défaut de la vue — jamais d'état incohérent avec le sélecteur.
 *
 * Retourne `[pageSize, setPageSize]` ; setPageSize persiste en mémoire et
 * en localStorage (silencieux si le stockage est indisponible — navigation
 * privée stricte : la préférence vit alors le temps de la session).
 *
 * NOTE — l'initialisateur lit localStorage : ces vues ne montent qu'en
 * client (auth client-side, écran de chargement avant), et la garde
 * `typeof window` couvre le rendu serveur du shell.
 */
export function usePageSize(viewKey: string, fallback: number = 10) {
  const [pageSize, setPageSizeState] = useState<number>(() => {
    if (typeof window === "undefined") return fallback;
    try {
      const raw = window.localStorage.getItem(STORAGE_PREFIX + viewKey);
      if (!raw) return fallback;
      const parsed = Number(raw);
      return (PAGE_SIZE_OPTIONS as readonly number[]).includes(parsed)
        ? parsed
        : fallback;
    } catch {
      return fallback;
    }
  });

  const setPageSize = useCallback(
    (size: number) => {
      setPageSizeState(size);
      try {
        window.localStorage.setItem(STORAGE_PREFIX + viewKey, String(size));
      } catch {
        /* stockage indisponible — préférence en mémoire seulement */
      }
    },
    [viewKey],
  );

  return [pageSize, setPageSize] as const;
}

/**
 * Le sélecteur « N / page » des barres de pagination. Le déclencheur affiche
 * la valeur courante (ex. « 25 / page ») ; l'attribut title porte la
 * description pédagogique (« Indique le nombre de résultats maximum par
 * page ») et l'aria-label le nom accessible. Le changement de taille NE
 * RESET PAS la page ici : chaque vue le fait dans son handler (repère
 * « setPage(1) » à côté du setPageSize — cohérent avec les filtres).
 */
export function PageSizeSelect({
  value,
  onChange,
}: {
  value: number;
  onChange: (size: number) => void;
}) {
  const { t } = useI18n();
  return (
    <Select
      value={String(value)}
      onValueChange={(raw) => onChange(Number(raw))}
    >
      <SelectTrigger
        size="sm"
        className="h-10 w-24"
        aria-label={t("common.pageSizeLabel")}
        title={t("common.pageSizeHint")}
      >
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {PAGE_SIZE_OPTIONS.map((option) => (
          <SelectItem key={option} value={String(option)}>
            {option} / {t("common.perPageUnit")}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}
