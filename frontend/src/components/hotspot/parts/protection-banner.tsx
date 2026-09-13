"use client";

// N°83 — Bandeau Protection du tableau de bord : le rappel quotidien de la
// valeur de l'abonnement. Chaque connexion montre au gérant que son WiFi
// est protégé — ou ce qu'il manque pour l'être (CTA vers la vue Protection).
//
// Données : GET /api/routers (cache partagé queryKey ["/api/routers"],
// rafraîchi 60 s — l'état de protection change rarement, les mutations des
// cartes invalident la clé de toute façon). Aucun bandeau sans routeur en
// mode agent : rien à protéger, aucun bruit visuel pour les comptes vides.

import { useQuery } from "@tanstack/react-query";
import { ShieldAlert, ShieldCheck, ShieldHalf } from "lucide-react";

import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { api } from "@/lib/hotspot/api";
import { useI18n } from "@/lib/hotspot/i18n";
import { protectionScore } from "@/lib/hotspot/protection";
import { useHotspotStore } from "@/lib/hotspot/store";
import type { RouterDevice } from "@/lib/hotspot/types";

export function ProtectionBanner() {
  const { t, tf } = useI18n();
  const setView = useHotspotStore((s) => s.setView);

  const { data: routers } = useQuery({
    queryKey: ["/api/routers"],
    queryFn: () => api<RouterDevice[]>("/api/routers"),
    refetchInterval: 60_000,
  });

  // Rien à protéger (aucun routeur, ou aucun en mode agent) : pas de bandeau.
  const agentRouters = (routers ?? []).filter((r) => r.mode === "agent");
  if (agentRouters.length === 0) return null;

  // N°88 : 4 modules de protection par routeur (SafeWiFi, Shield,
  // FamilyGuard, AntiVPN) — le total suit la source unique protectionScore.
  const total = agentRouters.length * 4;
  const on = agentRouters.reduce((n, r) => n + protectionScore(r), 0);
  const verdict = on === total ? "protected" : on === 0 ? "unprotected" : "partial";

  const styles = {
    protected: {
      icon: ShieldCheck,
      box: "border-primary/25 bg-primary/5",
      iconBox: "bg-primary/10 text-primary",
      title: "text-foreground",
    },
    partial: {
      icon: ShieldHalf,
      box: "border-amber-500/25 bg-amber-500/5",
      iconBox: "bg-amber-500/15 text-amber-600 dark:text-amber-400",
      title: "text-amber-700 dark:text-amber-300",
    },
    unprotected: {
      icon: ShieldAlert,
      box: "border-amber-500/40 bg-amber-500/10",
      iconBox: "bg-amber-500/15 text-amber-600 dark:text-amber-400",
      title: "text-amber-700 dark:text-amber-300",
    },
  }[verdict];
  const Icon = styles.icon;

  return (
    <div
      role="status"
      className={cn(
        "flex flex-col gap-3 rounded-xl border px-4 py-3 sm:flex-row sm:items-center sm:justify-between",
        styles.box,
      )}
    >
      <div className="flex min-w-0 items-start gap-3">
        <span
          className={cn("mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg", styles.iconBox)}
        >
          <Icon className="size-4" aria-hidden />
        </span>
        <div className="min-w-0">
          <p className={cn("text-sm font-semibold", styles.title)}>
            {t(`protection.banner.${verdict}Title`)}
          </p>
          <p className="mt-0.5 text-xs leading-relaxed text-muted-foreground">
            {tf("protection.banner.count", {
              on,
              total,
              routers: agentRouters.length,
              s: agentRouters.length > 1 ? "s" : "",
            })}
          </p>
        </div>
      </div>
      <Button
        size="sm"
        variant={verdict === "protected" ? "outline" : "default"}
        className="shrink-0 self-start sm:self-center"
        onClick={() => setView("protection")}
      >
        {t("protection.banner.cta")}
      </Button>
    </div>
  );
}
