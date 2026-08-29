"use client";

import { useTheme } from "next-themes";

/**
 * Palette Recharts thématée « Aurora Emerald ».
 * Les SVG Recharts exigent des couleurs réelles (pas des var()), on fournit
 * donc deux jeux hex complets qui suivent le mode Nuit/Jour de next-themes.
 */
export interface ChartPalette {
  /** Trame de fond (quadrillage) */
  grid: string;
  /** Texte des axes */
  axis: string;
  /** Séries — émeraude, teal, ambre, rose, lime */
  series: [string, string, string, string, string];
  /** Zone de remplissage area (hex + alpha) */
  areaFill: string;
  /** Fond du curseur barres */
  cursorFill: string;
}

const NIGHT: ChartPalette = {
  grid: "#2b3a32",
  axis: "#8fa69b",
  series: ["#34d399", "#2dd4bf", "#fbbf24", "#fb7185", "#a3e635"],
  areaFill: "#34d39926",
  cursorFill: "#34d399",
};

const DAY: ChartPalette = {
  grid: "#dde8e0",
  axis: "#4f5f58",
  series: ["#059669", "#0d9488", "#d97706", "#e11d48", "#65a30d"],
  areaFill: "#05966922",
  cursorFill: "#059669",
};

/** Couleurs de graphiques adaptées au thème résolu (nuit par défaut). */
export function useChartPalette(): ChartPalette {
  const { resolvedTheme } = useTheme();
  return resolvedTheme === "light" ? DAY : NIGHT;
}
