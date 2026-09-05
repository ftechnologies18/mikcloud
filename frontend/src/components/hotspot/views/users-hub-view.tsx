"use client";

// Hub « Utilisateurs » (fusion N°30) — les inscriptions publiques par QR
// (N°27) ne sont plus une page séparée : la page Utilisateurs porte deux
// onglets, « Comptes » (comptes hotspot réguliers) et « Inscriptions »
// (demandes + liens & QR). L'onglet actif EST la vue du store : les deux
// URLs historiques /app/users et /app/registrations restent valides et
// deep-linkent le bon onglet, sans dupliquer les vues côté app-shell.
// L'onglet Inscriptions reste soumis au rang manager+ (miroir canView de
// la requireRole API) : masqué pour un rôle insuffisant, l'URL
// /app/registrations retombe alors sur « Comptes ».

import dynamic from "next/dynamic";
import { UserPlus, Users } from "lucide-react";

import { LoadingRows } from "@/components/hotspot/loading";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useI18n } from "@/lib/hotspot/i18n";
import { canView } from "@/lib/hotspot/roles";
import { useHotspotStore } from "@/lib/hotspot/store";
import type { ViewId } from "@/lib/hotspot/types";

const AccountsView = dynamic(() => import("./users-view"), {
  loading: () => <LoadingRows withTitle />,
});
const RegistrationsView = dynamic(() => import("./registrations-view"), {
  loading: () => <LoadingRows withTitle />,
});

export default function UsersHubView() {
  const { t } = useI18n();
  const view = useHotspotStore((s) => s.view);
  const setView = useHotspotStore((s) => s.setView);
  const role = useHotspotStore((s) => s.user?.role);
  const withRegistrations = canView(role, "registrations");
  // Vue effective : « registrations » seulement si le rôle le permet —
  // sinon (lien direct /app/registrations sans rang) on reste sur Comptes.
  const active: "users" | "registrations" =
    withRegistrations && view === "registrations" ? "registrations" : "users";

  return (
    <div className="space-y-4 sm:space-y-6">
      {/* Onglets de niveau page — la valeur EST le ViewId : le changement
          d'onglet passe par setView, la couche URL (app-route) met alors
          /app/users ou /app/registrations à jour automatiquement. */}
      <Tabs
        value={active}
        onValueChange={(value) => setView(value as ViewId)}
      >
        <TabsList>
          <TabsTrigger value="users" className="gap-1.5">
            <Users className="size-3.5" aria-hidden />
            {t("users.hub.accounts")}
          </TabsTrigger>
          {withRegistrations && (
            <TabsTrigger value="registrations" className="gap-1.5">
              <UserPlus className="size-3.5" aria-hidden />
              {t("users.hub.registrations")}
            </TabsTrigger>
          )}
        </TabsList>
      </Tabs>
      {active === "registrations" ? <RegistrationsView /> : <AccountsView />}
    </div>
  );
}
