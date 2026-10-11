"use client";

import { useTheme } from "next-themes";

/**
 * Palette Recharts thématée « V10 Charbon & Corail ».
 * Les SVG Recharts exigent des couleurs réelles (pas des var()), on fournit
 * donc deux jeux hex complets qui suivent le mode Nuit/Jour de next-themes.
 * Séries : bleu (données principales), cyan, émeraude (succès — héritage
 * MikCloud), ambre, rose — fidèle au design V10 (aire dégradée + glow bleu).
 */
export interface ChartPalette {
  /** Trame de fond (quadrillage) */
  grid: string;
  /** Texte des axes */
  axis: string;
  /** Séries — bleu, cyan, émeraude, ambre, rose */
  series: [string, string, string, string, string];
  /** Zone de remplissage area (hex + alpha) */
  areaFill: string;
  /** Fond du curseur barres */
  cursorFill: string;
}

const NIGHT: ChartPalette = {
  grid: "#232838",
  axis: "#8b91a5",
  series: ["#3B82F6", "#22D3EE", "#10B981", "#F59E0B", "#FB7185"],
  areaFill: "#3B82F626",
  cursorFill: "#3B82F6",
};

const DAY: ChartPalette = {
  grid: "#e4e7ee",
  axis: "#6b7280",
  series: ["#3B82F6", "#06B6D4", "#059669", "#D97706", "#E11D48"],
  areaFill: "#3B82F62e",
  cursorFill: "#3B82F6",
};

/** Couleurs de graphiques adaptées au thème résolu (nuit par défaut). */
export function useChartPalette(): ChartPalette {
  const { resolvedTheme } = useTheme();
  return resolvedTheme === "light" ? DAY : NIGHT;
}
