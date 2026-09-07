"use client";

// N°57-d — Hub « Hotspot » (section de la zone Paramètres,
// /app/settings/hotspot). Trois onglets, chacun portant UNE préoccupation
// du service WiFi :
//   • Expérience — règles et personnalisation (expiration, tickets, portail
//     de l'établissement) — réservée au propriétaire (PUT /api/settings,
//     rang 3 : l'onglet est masqué au gérant, miroir canView) ;
//   • Portail — déploiement et aperçu du portail captif (rang 2) ;
//   • Modèles — gabarits d'impression des vouchers (rang 2).
//
// Pattern N°30 (« users/registrations → hub Utilisateurs ») : l'onglet actif
// EST la vue du store — hotspot / portal / templates sont trois ViewIds, la
// couche URL (app-route) met /app/settings/hotspot, /app/settings/hotspot/
// portail ou /app/settings/hotspot/modeles à jour automatiquement, les
// anciennes URLs restent deep-linkables. Le gérant sans rang « settings »
// retombe sur l'onglet Portail (lien direct /app/settings/hotspot).

import dynamic from "next/dynamic";
import { Loader2, Monitor, Printer, Ticket } from "lucide-react";

import { useI18n } from "@/lib/hotspot/i18n";
import { canView } from "@/lib/hotspot/roles";
import { useSettings } from "@/components/hotspot/parts/sd-currency";
import { useHotspotStore } from "@/lib/hotspot/store";
import type { ViewId } from "@/lib/hotspot/types";
import { HotspotExperience } from "@/components/hotspot/parts/hotspot-cards";
import { PageHeader } from "@/components/hotspot/page-header";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";

const TabFallback = (
  <div
    className="flex min-h-[40vh] items-center justify-center"
    role="status"
    aria-live="polite"
  >
    <Loader2 className="size-6 animate-spin text-muted-foreground" aria-hidden="true" />
  </div>
);

// Onglets lourds en chargement différé : l'expérience embarque le composant
// QR (aperçu logo), le portail l'iframe d'aperçu, les modèles l'éditeur +
// le rendu de tickets — chacun son chunk, chargé à la première ouverture.
const PortalContent = dynamic(() => import("./portal-view").then((m) => m.PortalContent), {
  loading: () => TabFallback,
});
const TemplatesContent = dynamic(() => import("./templates-view").then((m) => m.TemplatesContent), {
  loading: () => TabFallback,
});

export default function HotspotView() {
  const { t } = useI18n();
  const view = useHotspotStore((s) => s.view);
  const setView = useHotspotStore((s) => s.setView);
  const role = useHotspotStore((s) => s.user?.role);
  // L'onglet Expérience écrit dans /api/settings (rang 3 serveur) : masqué
  // au gérant, comme le serait la section Général (miroir canView, N°7).
  const withExperience = canView(role, "settings");

  // Vue effective : « hotspot » (Expérience) seulement si le rôle le peut —
  // sinon (lien direct /app/settings/hotspot d'un gérant) on reste sur
  // l'onglet Portail, première préoccupation accessible.
  const active: "hotspot" | "portal" | "templates" =
    view === "templates" ? "templates" : view === "hotspot" && withExperience ? "hotspot" : "portal";

  // Paramètres du tenant — alimentent l'onglet Expérience uniquement
  // (chargé seulement quand l'onglet est actif, pas pour Portail/Modèles).
  const { data: settings, isLoading: settingsLoading } = useSettings();

  return (
    <div className="space-y-4 sm:space-y-6">
      <PageHeader title={t("settings.tabHotspot")} description={t("settings.hotspotDesc")} />

      {/* Onglets de niveau section — la valeur EST le ViewId : le changement
          d'onglet passe par setView, la couche URL met alors
          /app/settings/hotspot[/<onglet>] à jour automatiquement. */}
      <Tabs
        value={active}
        onValueChange={(value) => setView(value as ViewId)}
        className="gap-4 sm:gap-6"
      >
        <TabsList className="glass-chip h-auto w-full justify-start overflow-x-auto rounded-xl p-1 sm:w-fit">
          {withExperience && (
            <TabsTrigger value="hotspot" className="gap-1.5 px-3 py-1.5 text-xs sm:px-4 sm:text-sm">
              <Ticket className="size-3.5" />
              {t("hotspot.tabExperience")}
            </TabsTrigger>
          )}
          <TabsTrigger value="portal" className="gap-1.5 px-3 py-1.5 text-xs sm:px-4 sm:text-sm">
            <Monitor className="size-3.5" />
            {t("nav.portal")}
          </TabsTrigger>
          <TabsTrigger value="templates" className="gap-1.5 px-3 py-1.5 text-xs sm:px-4 sm:text-sm">
            <Printer className="size-3.5" />
            {t("nav.templates")}
          </TabsTrigger>
        </TabsList>
      </Tabs>

      {active === "hotspot" ? (
        settingsLoading || !settings ? (
          TabFallback
        ) : (
          <HotspotExperience settings={settings} />
        )
      ) : active === "portal" ? (
        <PortalContent />
      ) : (
        <TemplatesContent />
      )}
    </div>
  );
}
