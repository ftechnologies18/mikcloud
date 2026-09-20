"use client";

// N°164 — bannière de persistance dégradée (mode plateforme uniquement) :
// quand le backend a démarré sans PostgreSQL (quota suspendu, panne, bascule),
// il vit en mémoire seule et une récupération automatique tourne en arrière
// plan. Le super-admin voit le bandeau ROUGE tant que la récupération n'a pas
// abouti — le détail (tentatives, dernière erreur, horodatage) vit dans la
// carte Santé de la console plateforme (Paramètres plateforme → Maintenance).
// Contrat : GET /api/admin/sync-status (rang 3) — champ « degraded ».
// Non masquable : une persistance suspendue n'est pas une information
// optionnelle.

import { useQuery } from "@tanstack/react-query";
import { DatabaseZap } from "lucide-react";

import { api } from "@/lib/hotspot/api";
import { useI18n } from "@/lib/hotspot/i18n";

interface DegradedBlock {
  degraded: boolean;
  since?: string;
  recoveredAt?: string;
  recoveryTries: number;
  lastError?: string;
}

export function PersistenceDegradedBanner() {
  const { t } = useI18n();

  // Même cadence que la carte Santé (15 s) : le passage rouge → rien à la
  // récupération doit être visible sans rechargement manuel.
  const { data } = useQuery({
    queryKey: ["/api/admin/sync-status", "degraded-banner"],
    queryFn: () => api<{ degraded?: DegradedBlock | null }>("/api/admin/sync-status"),
    refetchInterval: 15_000,
    retry: false, // 403 (mode client) ou panne : pas de spam de retry
  });

  if (!data?.degraded?.degraded) {
    return null;
  }

  return (
    <div
      className="border-b border-destructive/25 bg-destructive/10"
      role="alert"
      aria-live="polite"
    >
      <div className="mx-auto flex max-w-7xl items-center gap-2 px-4 py-1.5 sm:px-6">
        <DatabaseZap className="size-4 shrink-0 text-destructive" aria-hidden />
        <p className="min-w-0 flex-1 truncate text-xs font-medium text-destructive sm:text-sm">
          {t("banner.persistenceDegraded")}
        </p>
      </div>
    </div>
  );
}
