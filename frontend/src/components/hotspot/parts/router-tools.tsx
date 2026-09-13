"use client";

// Outils routeur (vague P1 — CONTRACT-V2) : F6 trafic temps réel, F7 IP bindings,
// F9 DHCP/hôtes/cookies/journal, F8+F10 système (infos, ping, scheduler,
// reboot/shutdown). Le dialogue est monté conditionnellement par routers-view
// (pattern Next 16 : dialogs montés avec leur état initial, aucun setState en effect).
//
// Éclatement N°92 : ce fichier est l'ENTRY du panneau (RouterToolsPanel) ;
// les onglets et cartes vivent dans ./router-tools/ — composants autonomes,
// zéro déplacement d'état.

import { useQuery } from "@tanstack/react-query";
import { Activity, Cpu, Network, Wrench } from "lucide-react";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { StatusBadge } from "@/components/hotspot/status-badge";
import { api } from "@/lib/hotspot/api";
import { useI18n } from "@/lib/hotspot/i18n";
import type { RouterDevice } from "@/lib/hotspot/types";
import { TrafficTab } from "./router-tools/traffic-tab";
import { IpBindingsTab } from "./router-tools/bindings-tab";
import { ToolsTab } from "./router-tools/tools-tab";
import { SystemTab } from "./router-tools/system-tab";


// Retour utilisateur N°57-d : plus de fenêtre modale pour inspecter un
// routeur — le panneau vit DANS la fiche routeur (page directe
// /app/settings/routers/<id>, bouton Retour aux cartes). Même contenu
// (onglets Trafic / Liaisons IP / Outils / Système), même routeur « vivant »
// (poll 15 s partagé avec la liste), seul le conteneur change : un bloc
// plein largeur en flux de page au lieu d'un overlay.
export function RouterToolsPanel({ router: snapshot }: { router: RouterDevice }) {
  const { t, tf } = useI18n();
  // Routeur « vivant » : la vue Routeurs poll déjà ["/api/routers"] toutes les 15 s
  // (même queryKey → cache partagé). Si le routeur passe hors ligne ou disparaît
  // pendant l'affichage, les onglets dégradent proprement (états d'erreur/toasts).
  const { data: routers } = useQuery({
    queryKey: ["/api/routers"],
    queryFn: () => api<RouterDevice[]>("/api/routers"),
    refetchInterval: 15_000,
  });
  const router = routers?.find((r) => r.id === snapshot.id) ?? snapshot;

  return (
    <div className="flex flex-col gap-0">
      <div className="shrink-0 border-b px-1 pb-4">
        <p className="flex flex-wrap items-center gap-2 text-sm font-semibold">
          <Wrench className="size-4 shrink-0 text-primary" aria-hidden />
          {tf("tools.dialogTitle", { name: router.name })}
        </p>
        <span className="mt-1 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
          <StatusBadge status={router.mode} />
          <StatusBadge status={router.status} dot />
          <span>{t("tools.dialogDesc")}</span>
        </span>
      </div>

      <Tabs defaultValue="traffic" className="flex min-h-0 flex-1 flex-col gap-0">
        <div className="shrink-0 overflow-x-auto py-3">
          <TabsList className="w-full min-w-max sm:w-fit">
            <TabsTrigger value="traffic">
              <Activity className="size-4" />
              {t("tools.tabTraffic")}
            </TabsTrigger>
            <TabsTrigger value="bindings">
              <Network className="size-4" />
              {t("tools.tabBindings")}
            </TabsTrigger>
            <TabsTrigger value="tools">
              <Wrench className="size-4" />
              {t("tools.tabTools")}
            </TabsTrigger>
            <TabsTrigger value="system">
              <Cpu className="size-4" />
              {t("tools.tabSystem")}
            </TabsTrigger>
          </TabsList>
        </div>

        <div className="min-h-0 flex-1 pb-1">
          <TabsContent value="traffic" className="mt-0">
            <TrafficTab router={router} />
          </TabsContent>
          <TabsContent value="bindings" className="mt-0">
            <IpBindingsTab router={router} />
          </TabsContent>
          <TabsContent value="tools" className="mt-0">
            <ToolsTab router={router} />
          </TabsContent>
          <TabsContent value="system" className="mt-0">
            <SystemTab router={router} />
          </TabsContent>
        </div>
      </Tabs>
    </div>
  );
}
