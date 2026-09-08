"use client";

// N°27 — WiFi Jetable : vue console du mode d'accès offert.
// Le gérant crée ses sites (maquis, resto, salon…), AJUSTE les quotas
// (temps/data/plafonds) ou crée de nouveaux quotas (profils, inline),
// bascule l'offre en 1 clic, imprime l'affiche QR et exploite le registre
// marketing (export CSV des numéros opt-in).

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ExternalLink,
  Loader2,
  Pencil,
  Plus,
  QrCode,
  Trash2,
  Users,
  Wifi,
  WifiOff,
} from "lucide-react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Switch } from "@/components/ui/switch";
import { PageHeader } from "@/components/hotspot/page-header";
import { EmptyState } from "@/components/hotspot/empty-state";
import { WifiPosterDialog } from "@/components/hotspot/parts/wifi-poster-dialog";
import { copyToClipboard } from "@/components/hotspot/parts/uc-clipboard";
// N°63 — dialog création/édition devenu WIZARD 2 étapes (le reste de la vue est inchangé).
import { EMPTY_FORM, WifiSiteWizard, formFromSite, type SiteForm } from "@/components/hotspot/parts/wifi-site-wizard";
import {
  api,
  apiDownload,
  createWifiSite,
  deleteWifiSite,
  fetchWifiGuests,
  fetchWifiSites,
  updateWifiSite,
  wifiGuestsCsvURL,
} from "@/lib/hotspot/api";
import { useI18n } from "@/lib/hotspot/i18n";
import type { Profile, RouterDevice, WifiSite } from "@/lib/hotspot/types";

export default function WifiView() {
  const { t } = useI18n();
  const queryClient = useQueryClient();

  // N°54 — refetch 30 s : le compteur « X / Y offerts aujourd'hui » reste
  // vivant pendant que le gérant garde la vue ouverte.
  const { data, isLoading } = useQuery({
    queryKey: ["/api/wifi/sites"],
    queryFn: fetchWifiSites,
    refetchInterval: 30_000,
  });
  const { data: routers } = useQuery({
    queryKey: ["/api/routers"],
    queryFn: () => api<RouterDevice[]>("/api/routers"),
  });
  const { data: profiles } = useQuery({
    queryKey: ["/api/profiles"],
    queryFn: () => api<Profile[]>("/api/profiles"),
  });

  const [dialogOpen, setDialogOpen] = useState(false);
  const [editing, setEditing] = useState<WifiSite | null>(null);
  const [form, setForm] = useState<SiteForm>(EMPTY_FORM);
  // N°63 — remonte le wizard à chaque ouverture (étape 1 vierge) via la clé.
  const [wizardNonce, setWizardNonce] = useState(0);
  const [guestsFor, setGuestsFor] = useState<WifiSite | null>(null);
  const [posterFor, setPosterFor] = useState<WifiSite | null>(null);
  const [saving, setSaving] = useState(false);

  const sites = data?.sites ?? [];
  const stats = data?.stats ?? {};
  const origin = typeof window !== "undefined" ? window.location.origin : "";
  const publicUrlOf = (site: WifiSite) => `${origin}/wifi/${site.slug}`;

  const openCreate = () => {
    setEditing(null);
    // Pré-sélection : premier routeur + premier profil à 0 F (sinon premier profil).
    const freeProfile = profiles?.find((p) => p.price === 0) ?? profiles?.[0];
    setForm({
      ...EMPTY_FORM,
      routerId: routers?.[0]?.id ?? "",
      profileId: freeProfile?.id ?? "",
    });
    setWizardNonce((n) => n + 1);
    setDialogOpen(true);
  };

  const openEdit = (site: WifiSite) => {
    setEditing(site);
    setForm(formFromSite(site));
    setWizardNonce((n) => n + 1);
    setDialogOpen(true);
  };

  const saveMutation = useMutation({
    mutationFn: async (payload: SiteForm) => {
      if (editing) return updateWifiSite(editing.id, payload);
      return createWifiSite(payload);
    },
    onSuccess: () => {
      toast.success(t("wifi.saved"));
      setDialogOpen(false);
      queryClient.invalidateQueries({ queryKey: ["/api/wifi/sites"] });
    },
    onError: (err: Error) => toast.error(err.message),
  });

  const toggleMutation = useMutation({
    mutationFn: (site: WifiSite) =>
      updateWifiSite(site.id, {
        name: site.name,
        routerId: site.routerId,
        profileId: site.profileId,
        freeTimeMin: site.freeTimeMin,
        freeDataMb: site.freeDataMb,
        marketingOptIn: site.marketingOptIn,
        dailyPerPhone: site.dailyPerPhone,
        dailyPerMac: site.dailyPerMac,
        dailyCap: site.dailyCap,
        wifiSsid: site.wifiSsid,
        wifiPassword: site.wifiPassword,
        active: !site.active,
      }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["/api/wifi/sites"] }),
    onError: (err: Error) => toast.error(err.message),
  });

  const deleteMutation = useMutation({
    mutationFn: (site: WifiSite) => deleteWifiSite(site.id),
    onSuccess: () => {
      toast.success(t("wifi.deleted"));
      queryClient.invalidateQueries({ queryKey: ["/api/wifi/sites"] });
    },
    onError: (err: Error) => toast.error(err.message),
  });

  const onSubmit = async (payload: SiteForm) => {
    // Les trois requis (nom, routeur, profil) sont déjà garantis par l'étape 1
    // du wizard — le payload part tel quel (nom trimmé par le wizard).
    setSaving(true);
    try {
      await saveMutation.mutateAsync(payload);
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="space-y-6">
      <PageHeader
        title={t("wifi.title")}
        description={t("wifi.subtitle")}
        actions={
          <Button onClick={openCreate} aria-label={t("wifi.create")}>
            <Plus className="size-4" aria-hidden="true" /> {t("wifi.create")}
          </Button>
        }
      />

      {isLoading ? (
        <div className="flex min-h-[40vh] items-center justify-center" role="status" aria-live="polite">
          <Loader2 className="size-6 animate-spin text-muted-foreground" aria-hidden="true" />
        </div>
      ) : sites.length === 0 ? (
        <EmptyState
          icon={Wifi}
          title={t("wifi.empty")}
          description={t("wifi.emptyHint")}
          action={
            <Button onClick={openCreate}>
              <Plus className="size-4" aria-hidden="true" /> {t("wifi.create")}
            </Button>
          }
        />
      ) : (
        <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          {sites.map((site) => {
            const st = stats[site.id] ?? { guestsToday: 0, optInTotal: 0 };
            // N°54 — compteur/jauge du plafond journalier : le gérant voit
            // d'un coup d'œil le budget offert restant (ambre ≥ 80 %, rouge épuisé).
            const capRatio = site.dailyCap > 0 ? st.guestsToday / site.dailyCap : 1;
            const capTone = capRatio >= 1 ? "bg-destructive" : capRatio >= 0.8 ? "bg-amber-500" : "bg-primary";
            return (
              <Card key={site.id} className="overflow-hidden">
                <CardContent className="space-y-4 p-4">
                  <div className="flex items-start justify-between gap-2">
                    <div className="min-w-0">
                      <p className="truncate text-base font-semibold">{site.name}</p>
                      <p className="truncate text-xs text-muted-foreground">/wifi/{site.slug}</p>
                    </div>
                    <Badge variant={site.active ? "default" : "secondary"} className="shrink-0">
                      {site.active ? <Wifi className="size-3" aria-hidden="true" /> : <WifiOff className="size-3" aria-hidden="true" />}
                      {site.active ? "ON" : "OFF"}
                    </Badge>
                  </div>

                  <div className="grid grid-cols-2 gap-2 text-sm">
                    <div className="rounded-lg bg-muted/50 p-2">
                      <p className="text-xs text-muted-foreground">{t("wifi.quota")}</p>
                      <p className="font-medium">
                        {site.freeTimeMin > 0 ? `${site.freeTimeMin} min` : site.profileName}
                        {site.freeDataMb > 0 ? ` · ${site.freeDataMb} Mo` : ""}
                      </p>
                    </div>
                    <div className="rounded-lg bg-muted/50 p-2">
                      <p className="text-xs text-muted-foreground">{t("wifi.stats.capLabel")}</p>
                      <p className="font-medium">
                        {st.guestsToday} / {site.dailyCap}{" "}
                        <span className="text-xs font-normal text-muted-foreground">{t("wifi.stats.today")}</span>
                      </p>
                      <div
                        aria-hidden="true"
                        className="mt-1.5 h-1.5 overflow-hidden rounded-full bg-background"
                        role="presentation"
                      >
                        <div
                          className={`h-full rounded-full ${capTone}`}
                          style={{ width: `${Math.min(100, Math.round(capRatio * 100))}%` }}
                        />
                      </div>
                    </div>
                  </div>

                  <div className="flex items-center justify-between rounded-lg border p-2">
                    <div className="flex items-center gap-2">
                      <Switch
                        checked={site.active}
                        onCheckedChange={() => toggleMutation.mutate(site)}
                        aria-label={t("wifi.active")}
                      />
                      <span className="text-sm">{t("wifi.active")}</span>
                    </div>
                    <span className="text-xs text-muted-foreground">
                      {st.optInTotal} {t("wifi.stats.optin")}
                    </span>
                  </div>

                  <div className="flex flex-wrap gap-2">
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={async () => {
                        const ok = await copyToClipboard(publicUrlOf(site));
                        if (ok) toast.success(t("wifi.copyUrl"));
                      }}
                    >
                      <ExternalLink className="size-4" aria-hidden="true" /> {t("wifi.copyUrl")}
                    </Button>
                    <Button size="sm" variant="outline" onClick={() => setPosterFor(site)}>
                      <QrCode className="size-4" aria-hidden="true" /> {t("wifi.poster")}
                    </Button>
                    <Button size="sm" variant="outline" onClick={() => setGuestsFor(site)}>
                      <Users className="size-4" aria-hidden="true" /> {t("wifi.guests")}
                    </Button>
                    <Button size="sm" variant="outline" onClick={() => openEdit(site)}>
                      <Pencil className="size-4" aria-hidden="true" /> {t("wifi.edit")}
                    </Button>
                    <Button
                      size="sm"
                      variant="ghost"
                      className="text-destructive hover:text-destructive"
                      onClick={() => {
                        if (window.confirm(t("wifi.deleteConfirm"))) deleteMutation.mutate(site);
                      }}
                    >
                      <Trash2 className="size-4" aria-hidden="true" /> {t("wifi.delete")}
                    </Button>
                  </div>
                </CardContent>
              </Card>
            );
          })}
        </div>
      )}

      {/* N°63 — création / édition en WIZARD 2 étapes (identité → offre) :
          le formulaire plat d'un bloc est remplacé, payload identique. */}
      <WifiSiteWizard
        key={wizardNonce}
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        editing={editing}
        form={form}
        setForm={setForm}
        routers={routers ?? []}
        profiles={profiles ?? []}
        saving={saving}
        onSubmit={(payload) => void onSubmit(payload)}
      />

      {/* Registre marketing (table + export CSV). */}
      <GuestsDialog site={guestsFor} onClose={() => setGuestsFor(null)} />

      {/* Affiche QR imprimable. */}
      <WifiPosterDialog
        key={posterFor?.id ?? "none"}
        open={Boolean(posterFor)}
        onOpenChange={(o) => !o && setPosterFor(null)}
        siteName={posterFor?.name ?? ""}
        wifiSsid={posterFor?.wifiSsid ?? ""}
        wifiPassword={posterFor?.wifiPassword ?? ""}
        quotaLabel={
          posterFor
            ? `${posterFor.freeTimeMin > 0 ? `${posterFor.freeTimeMin} min` : posterFor.profileName}${
                posterFor.freeDataMb > 0 ? ` · ${posterFor.freeDataMb} Mo` : ""
              }`
            : ""
        }
      />
    </div>
  );
}

function GuestsDialog({ site, onClose }: { site: WifiSite | null; onClose: () => void }) {
  const { t } = useI18n();
  const [optInOnly, setOptInOnly] = useState(false);
  const { data, isLoading } = useQuery({
    queryKey: ["/api/wifi/guests", site?.id, optInOnly],
    queryFn: () => fetchWifiGuests(site!.id, optInOnly ? true : undefined),
    enabled: Boolean(site),
  });
  const guests = useMemo(() => data?.guests ?? [], [data]);

  return (
    <Dialog open={Boolean(site)} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-h-[90vh] sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>
            {t("wifi.guests")} — {site?.name}
          </DialogTitle>
          <DialogDescription>{t("wifi.optInHint")}</DialogDescription>
        </DialogHeader>
        <div className="flex items-center justify-between gap-2">
          <label className="flex items-center gap-2 text-sm">
            <Switch checked={optInOnly} onCheckedChange={setOptInOnly} aria-label={t("wifi.guest.optin")} />
            {t("wifi.guest.optin")}
          </label>
          <Button
            size="sm"
            variant="outline"
            onClick={() => site && apiDownload(wifiGuestsCsvURL(site.id), "wifi-guests.csv")}
          >
            {t("wifi.csv")}
          </Button>
        </div>
        <div className="max-h-72 overflow-y-auto rounded-lg border" style={{ scrollbarWidth: "thin" }}>
          {isLoading ? (
            <div className="flex justify-center py-10">
              <Loader2 className="size-5 animate-spin text-muted-foreground" aria-hidden="true" />
            </div>
          ) : guests.length === 0 ? (
            <p className="py-10 text-center text-sm text-muted-foreground">{t("wifi.empty")}</p>
          ) : (
            <table className="w-full text-sm">
              <thead className="sticky top-0 bg-muted text-left text-xs uppercase text-muted-foreground">
                <tr>
                  <th className="p-2 font-medium">{t("wifi.guest.date")}</th>
                  <th className="p-2 font-medium">{t("wifi.guest.phone")}</th>
                  <th className="p-2 font-medium">{t("wifi.guest.optin")}</th>
                  <th className="p-2 font-medium">{t("wifi.guest.code")}</th>
                </tr>
              </thead>
              <tbody>
                {guests.map((g) => (
                  <tr key={g.id} className="border-t">
                    <td className="p-2 text-xs text-muted-foreground">
                      {new Date(g.createdAt).toLocaleString("fr-FR", { dateStyle: "short", timeStyle: "short" })}
                    </td>
                    <td className="p-2 font-mono">+{g.phone}</td>
                    <td className="p-2">
                      {/* N°69 — la date de preuve (optInAt) accompagne le ✓ :
                          le gérant voit QUAND le consentement a été posé. */}
                      {g.optIn ? (
                        <span title={g.optInAt ? new Date(g.optInAt).toLocaleString("fr-FR") : undefined}>
                          ✓{g.optInAt ? ` ${new Date(g.optInAt).toLocaleDateString("fr-FR")}` : ""}
                        </span>
                      ) : (
                        "—"
                      )}
                    </td>
                    <td className="p-2 font-mono">{g.code}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}
