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
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { PageHeader } from "@/components/hotspot/page-header";
import { EmptyState } from "@/components/hotspot/empty-state";
import { WifiPosterDialog } from "@/components/hotspot/parts/wifi-poster-dialog";
import { copyToClipboard } from "@/components/hotspot/parts/uc-clipboard";
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
import type { Profile, RouterDevice, WifiSite, WifiSitePayload } from "@/lib/hotspot/types";

interface SiteForm {
  name: string;
  routerId: string;
  profileId: string;
  freeTimeMin: number;
  freeDataMb: number;
  marketingOptIn: boolean;
  dailyPerPhone: number;
  dailyCap: number;
  active: boolean;
}

function formFromSite(site: WifiSite): SiteForm {
  return {
    name: site.name,
    routerId: site.routerId,
    profileId: site.profileId,
    freeTimeMin: site.freeTimeMin,
    freeDataMb: site.freeDataMb,
    marketingOptIn: site.marketingOptIn,
    dailyPerPhone: site.dailyPerPhone,
    dailyCap: site.dailyCap,
    active: site.active,
  };
}

const EMPTY_FORM: SiteForm = {
  name: "",
  routerId: "",
  profileId: "",
  freeTimeMin: 30,
  freeDataMb: 100,
  marketingOptIn: true,
  dailyPerPhone: 1,
  dailyCap: 100,
  active: true,
};

export default function WifiView() {
  const { t } = useI18n();
  const queryClient = useQueryClient();

  const { data, isLoading } = useQuery({ queryKey: ["/api/wifi/sites"], queryFn: fetchWifiSites });
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
    setDialogOpen(true);
  };

  const openEdit = (site: WifiSite) => {
    setEditing(site);
    setForm(formFromSite(site));
    setDialogOpen(true);
  };

  const saveMutation = useMutation({
    mutationFn: async (payload: WifiSitePayload) => {
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
        dailyCap: site.dailyCap,
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

  const onSubmit = async () => {
    if (form.name.trim().length < 2 || !form.routerId || !form.profileId) {
      toast.error(t("wifi.name"));
      return;
    }
    setSaving(true);
    try {
      await saveMutation.mutateAsync({ ...form, name: form.name.trim() });
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
                      <p className="text-xs text-muted-foreground">{t("wifi.guests")}</p>
                      <p className="font-medium">
                        {st.guestsToday} {t("wifi.stats.today")}
                      </p>
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

      {/* Dialog création / édition — quotas AJUSTABLES par le gérant. */}
      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>{editing ? t("wifi.edit") : t("wifi.create")}</DialogTitle>
            <DialogDescription>{t("wifi.profileHint")}</DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-1.5">
              <Label htmlFor="wifi-name">{t("wifi.name")}</Label>
              <Input
                id="wifi-name"
                value={form.name}
                placeholder={t("wifi.namePh")}
                maxLength={60}
                onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
              />
            </div>
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="space-y-1.5">
                <Label>{t("wifi.router")}</Label>
                <Select
                  value={form.routerId}
                  onValueChange={(v) => setForm((f) => ({ ...f, routerId: v }))}
                >
                  <SelectTrigger aria-label={t("wifi.router")}>
                    <SelectValue placeholder={t("wifi.router")} />
                  </SelectTrigger>
                  <SelectContent>
                    {(routers ?? []).map((r) => (
                      <SelectItem key={r.id} value={r.id}>
                        {r.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-1.5">
                <Label>{t("wifi.profile")}</Label>
                <Select
                  value={form.profileId}
                  onValueChange={(v) => setForm((f) => ({ ...f, profileId: v }))}
                >
                  <SelectTrigger aria-label={t("wifi.profile")}>
                    <SelectValue placeholder={t("wifi.profile")} />
                  </SelectTrigger>
                  <SelectContent>
                    {(profiles ?? []).map((p) => (
                      <SelectItem key={p.id} value={p.id}>
                        {p.name} · {p.price} F
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="wifi-time">{t("wifi.freeTime")}</Label>
                <Input
                  id="wifi-time"
                  type="number"
                  min={0}
                  value={form.freeTimeMin}
                  onChange={(e) => setForm((f) => ({ ...f, freeTimeMin: Number(e.target.value) || 0 }))}
                />
                <p className="text-xs text-muted-foreground">{t("wifi.freeTimeHint")}</p>
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="wifi-data">{t("wifi.freeData")}</Label>
                <Input
                  id="wifi-data"
                  type="number"
                  min={0}
                  value={form.freeDataMb}
                  onChange={(e) => setForm((f) => ({ ...f, freeDataMb: Number(e.target.value) || 0 }))}
                />
                <p className="text-xs text-muted-foreground">{t("wifi.freeDataHint")}</p>
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="wifi-perphone">{t("wifi.perPhone")}</Label>
                <Input
                  id="wifi-perphone"
                  type="number"
                  min={1}
                  max={10}
                  value={form.dailyPerPhone}
                  onChange={(e) => setForm((f) => ({ ...f, dailyPerPhone: Number(e.target.value) || 1 }))}
                />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="wifi-cap">{t("wifi.dailyCap")}</Label>
                <Input
                  id="wifi-cap"
                  type="number"
                  min={1}
                  max={1000}
                  value={form.dailyCap}
                  onChange={(e) => setForm((f) => ({ ...f, dailyCap: Number(e.target.value) || 100 }))}
                />
              </div>
            </div>
            <div className="flex items-center justify-between rounded-lg border p-3">
              <div>
                <Label htmlFor="wifi-optin">{t("wifi.optIn")}</Label>
                <p className="text-xs text-muted-foreground">{t("wifi.optInHint")}</p>
              </div>
              <Switch
                id="wifi-optin"
                checked={form.marketingOptIn}
                onCheckedChange={(v) => setForm((f) => ({ ...f, marketingOptIn: v }))}
              />
            </div>
            <div className="flex items-center justify-between rounded-lg border p-3">
              <Label htmlFor="wifi-active">{t("wifi.active")}</Label>
              <Switch
                id="wifi-active"
                checked={form.active}
                onCheckedChange={(v) => setForm((f) => ({ ...f, active: v }))}
              />
            </div>
          </div>
          <DialogFooter>
            <Button onClick={onSubmit} disabled={saving}>
              {saving ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : null}
              {editing ? t("common.save") : t("wifi.create")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Registre marketing (table + export CSV). */}
      <GuestsDialog site={guestsFor} onClose={() => setGuestsFor(null)} />

      {/* Affiche QR imprimable. */}
      <WifiPosterDialog
        open={Boolean(posterFor)}
        onOpenChange={(o) => !o && setPosterFor(null)}
        siteName={posterFor?.name ?? ""}
        publicUrl={posterFor ? publicUrlOf(posterFor) : ""}
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
                    <td className="p-2">{g.optIn ? "✓" : "—"}</td>
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
