"use client";

// Helpers et états partagés des Outils routeur — constantes de validation
// (MAC, intervalle RouterOS), formatage court, enveloppe des outils F9/F10
// (fetchToolEnvelope) et les 4 composants d'état (non supporté, erreur,
// squelette, bannière en file). Transfert pur depuis router-tools.tsx.

import { Loader2, RefreshCw, TriangleAlert } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { EmptyState } from "@/components/hotspot/empty-state";
import { api, ApiError } from "@/lib/hotspot/api";
import { localeOf, useI18n } from "@/lib/hotspot/i18n";
import type { Lang } from "@/lib/hotspot/i18n";
import { useChartPalette } from "@/lib/hotspot/chart-theme";
import type { ToolEnvelope } from "@/lib/hotspot/types";

// ─── Constantes ───

// Palette thématée (nuit/jour) — injectée par TrafficTab via useChartPalette().

export const MAC_RE = /^([0-9A-Fa-f]{2}:){5}[0-9A-Fa-f]{2}$/;
export const INTERVAL_RE = /^\d+[smhdw]$/; // format RouterOS : 45s, 5m, 1h, 2d, 1w
export const MAX_SAMPLES = 60; // ~5 minutes d'historique par interface (poll 5 s)

export const sleep = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms));

/** Heure locale courte « 14:05:32 ». */
export function shortClock(iso: string, lang: Lang): string {
  const d = new Date(iso);
  if (isNaN(d.getTime())) return iso;
  return d.toLocaleTimeString(localeOf(lang), { hour: "2-digit", minute: "2-digit", second: "2-digit" });
}

/** Version ultra compacte pour l'axe Y du graphique (« 20 M », « 500 K »). */
export function shortBits(v: number): string {
  if (v >= 1_000_000_000) return `${Math.round(v / 1_000_000_000)} G`;
  if (v >= 1_000_000) return `${Math.round(v / 1_000_000)} M`;
  if (v >= 1_000) return `${Math.round(v / 1_000)} K`;
  return String(Math.round(v));
}

/** Latence « 12,3 ms » (entier au-delà de 100 ms). */
export function fmtMs(v: number): string {
  return `${v >= 100 ? Math.round(v) : v.toFixed(1).replace(".", ",")} ms`;
}

// ─── Enveloppe des outils (F9/F10) ───

/**
 * Récupère une enveloppe {queued, data, updatedAt} — tolère aussi un simple
 * tableau (défense contre les variantes de forme du backend).
 */
export async function fetchToolEnvelope<T>(path: string): Promise<ToolEnvelope<T>> {
  const raw = await api<ToolEnvelope<T> | T[]>(path);
  if (Array.isArray(raw)) return { queued: false, data: raw, updatedAt: "" };
  if (raw && typeof raw === "object") {
    return {
      queued: raw.queued ?? false,
      data: raw.data ?? [],
      updatedAt: raw.updatedAt ?? "",
    };
  }
  return { queued: false, data: [], updatedAt: "" };
}

// ─── États partagés ───

export function UnsupportedState() {
  const { t } = useI18n();
  return (
    <EmptyState
      icon={TriangleAlert}
      title={t("tools.unsupportedTitle")}
      description={t("tools.unsupportedDesc")}
    />
  );
}

export function ToolError({ error, onRetry }: { error: Error | null; onRetry: () => void }) {
  const { t } = useI18n();
  if (error instanceof ApiError && error.status === 400) return <UnsupportedState />;
  return (
    <EmptyState
      icon={TriangleAlert}
      title={t("tools.loadError")}
      description={error?.message ?? t("tools.loadErrorDefault")}
      action={
        <Button variant="outline" onClick={onRetry}>
          <RefreshCw className="size-4" />
          {t("common.retry")}
        </Button>
      }
    />
  );
}

export function ToolSkeleton({ rows = 4 }: { rows?: number }) {
  return (
    <div className="space-y-2.5 rounded-lg border p-4">
      {Array.from({ length: rows }).map((_, i) => (
        <div key={i} className="flex items-center gap-3">
          <Skeleton className="h-4 flex-1" />
          <Skeleton className="hidden h-4 w-24 sm:block" />
          <Skeleton className="h-4 w-14" />
        </div>
      ))}
    </div>
  );
}

/** Bandeau « file d'attente agent » commun (F9/F10). */
export function QueuedBanner() {
  const { t } = useI18n();
  return (
    <p
      className="flex items-center gap-2 rounded-lg border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-sm text-amber-700 dark:text-amber-400"
      role="status"
      aria-live="polite"
    >
      <Loader2 className="size-4 shrink-0 animate-spin" />
      {t("tools.queued")}
    </p>
  );
}
