"use client";

// Devise active du tenant (paramètres) — partagé par les vues Revendeurs / Rapports.

import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/hotspot/api";
import type { AppSettings } from "@/lib/hotspot/types";

export const SETTINGS_QUERY_KEY = ["/api/settings"] as const;

export function useSettings() {
  return useQuery({
    queryKey: SETTINGS_QUERY_KEY,
    queryFn: () => api<AppSettings>("/api/settings"),
    staleTime: 60_000,
  });
}

export function useCurrency(): string {
  const { data } = useSettings();
  return data?.tenant.currency || "FCFA";
}
