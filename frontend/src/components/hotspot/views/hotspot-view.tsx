"use client";

// N°184 — Hub « Hotspot » (section de la zone Paramètres,
// /app/settings/hotspot). DEUX onglets, chacun portant UNE préoccupation
// du service WiFi :
//   • Portail — la chaîne ENTIÈRE ROUTEUR → SITE → COMPTE (N°182) au même
//     endroit : portail du compte (rang 3, masqué au gérant), sites,
//     routeurs, journal des déploiements (rang 2) ;
//   • Vouchers & tickets — la préoccupation « ticket » du réglage à
//     l'impression : politique & identité (rang 3, masqué au gérant) puis
//     gabarits d'impression (rang 2).
//
// Historique : le hub N°57-d portait TROIS onglets (Expérience / Portail /
// Modèles) — la chaîne de personnalisation N°182 vivait coupée en deux (les
// surcharges site/routeur dans « Portail », la base COMPTE dans
// « Expérience », un bouton « Voir le portail » faisait le pont entre les
// deux). La fusion la réunifie : le ViewId « hotspot » disparaît (l'ancienne
// URL reste deep-linkable → onglet Portail), les deux ViewIds restants
// (portal/templates) rendent ce même hub (pattern N°30).
//
// N°142 — garde de sortie d'onglet : les formulaires du compte (portail et
// tickets) portent une saisie volatile (barre d'action du N°140). Chacun
// remonte son compteur (onDirtyChange) ; toute sortie d'onglet demandée
// passe par requestView — confirmation AlertDialog si saisie en cours,
// navigation directe sinon. Les onglets étant contrôlés (value={active}),
// un refus ne change rien visuellement.

import { useCallback, useState } from "react";
import dynamic from "next/dynamic";
import { Loader2, Monitor, Ticket } from "lucide-react";

import { useI18n } from "@/lib/hotspot/i18n";
import { canView } from "@/lib/hotspot/roles";
import { useHotspotStore } from "@/lib/hotspot/store";
import type { ViewId } from "@/lib/hotspot/types";
import { PageHeader } from "@/components/hotspot/page-header";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
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

// Onglets lourds en chargement différé : le portail embarque l'iframe
// d'aperçu et le formulaire du compte (composant QR), les tickets l'éditeur
// de gabarits + le rendu de tickets — chacun son chunk, chargé à la première
// ouverture.
const PortalContent = dynamic(() => import("./portal-view").then((m) => m.PortalContent), {
  loading: () => TabFallback,
});
const TemplatesContent = dynamic(() => import("./templates-view").then((m) => m.TemplatesContent), {
  loading: () => TabFallback,
});

export default function HotspotView() {
  const { t, tf } = useI18n();
  const view = useHotspotStore((s) => s.view);
  const setView = useHotspotStore((s) => s.setView);
  const role = useHotspotStore((s) => s.user?.role);
  // Les formulaires du compte écrivent dans /api/settings (rang 3 serveur) :
  // masqués au gérant, comme la section Général (miroir canView, N°7).
  const withAccount = canView(role, "settings");

  // Vue effective : « portal » (Portail) par défaut — un lien direct
  // /app/settings/hotspot d'un gérant y atterrissait déjà (l'Expérience lui
  // était masquée) ; le propriétaire y retrouve sa carte « Portail du
  // compte » en tête de chaîne.
  const active: "portal" | "templates" = view === "templates" ? "templates" : "portal";

  // N°142/N°184 — compteurs de groupes modifiés remontés par les DEUX
  // formulaires du compte (stables pour l'effet de remontée). Seul
  // l'onglet ACTIF est monté : la sortie d'onglet ne consulte que son
  // compteur. Le remontage d'un formulaire re-zérote son compteur : aucune
  // garde fantôme après une sortie confirmée.
  const [portalDirty, setPortalDirty] = useState(0);
  const [ticketsDirty, setTicketsDirty] = useState(0);
  const [pendingView, setPendingView] = useState<ViewId | null>(null);
  const handlePortalDirty = useCallback((count: number) => setPortalDirty(count), []);
  const handleTicketsDirty = useCallback((count: number) => setTicketsDirty(count), []);

  // Toute sortie d'onglet passe ici : propre → navigation directe ; saisie
  // en cours → confirmation (Rester / Quitter).
  const requestView = (value: string) => {
    const next = value as ViewId;
    const dirty = active === "portal" ? portalDirty : ticketsDirty;
    if (next !== active && dirty > 0) {
      setPendingView(next);
      return;
    }
    setView(next);
  };
  const confirmLeave = () => {
    const next = pendingView;
    setPendingView(null);
    if (next) setView(next);
  };

  return (
    <div className="space-y-4 sm:space-y-6">
      <PageHeader title={t("settings.tabHotspot")} description={t("settings.hotspotDesc")} />

      {/* Onglets de niveau section — la valeur EST le ViewId : le changement
          d'onglet passe par requestView (garde N°142), la couche URL met
          alors /app/settings/hotspot/<portail|modeles> à jour automatiquement. */}
      <Tabs
        value={active}
        onValueChange={requestView}
        className="gap-4 sm:gap-6"
      >
        <TabsList className="glass-chip h-auto w-full justify-start overflow-x-auto rounded-xl p-1 sm:w-fit">
          <TabsTrigger value="portal" className="gap-1.5 px-3 py-1.5 text-xs sm:px-4 sm:text-sm">
            <Monitor className="size-3.5" />
            {t("nav.portal")}
          </TabsTrigger>
          <TabsTrigger value="templates" className="gap-1.5 px-3 py-1.5 text-xs sm:px-4 sm:text-sm">
            <Ticket className="size-3.5" />
            {t("hotspot.tabVouchers")}
          </TabsTrigger>
        </TabsList>
      </Tabs>

      {active === "portal" ? (
        <PortalContent withAccount={withAccount} onDirtyChange={handlePortalDirty} />
      ) : (
        <TemplatesContent withPolicy={withAccount} onDirtyChange={handleTicketsDirty} />
      )}

      {/* N°142 — confirmation de sortie d'onglet : annuler restaure l'onglet
          d'origine (onglets contrôlés), confirmer navigue et assume la perte
          de la saisie non enregistrée. */}
      <AlertDialog
        open={pendingView !== null}
        onOpenChange={(open) => {
          if (!open) setPendingView(null);
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t("settings.exp.tabGuardTitle")}</AlertDialogTitle>
            <AlertDialogDescription>
              {tf("settings.exp.tabGuardDesc", {
                n: active === "portal" ? portalDirty : ticketsDirty,
              })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t("settings.exp.tabGuardStay")}</AlertDialogCancel>
            <AlertDialogAction onClick={confirmLeave}>
              {t("settings.exp.tabGuardLeave")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
