"use client";

// N°57-d — Cartes « Expérience hotspot » de la section Hotspot
// (components/hotspot/views/hotspot-view.tsx).
//
// Ces cartes vivaient dans l'onglet Hotspot de l'ancienne vue Paramètres
// (settings-view, pré-N°57-d) : expiration des vouchers, import automatique,
// bouton d'inscription du portail, personnalisation des tickets (DNS+logo),
// bannière et mode hospitalité du portail, guide de connexion routeur.
// L'éclatement des sections les déplace ici SANS AUCUNE modification
// fonctionnelle : mêmes mutations (PUT /api/settings), mêmes contrats,
// mêmes libellés i18n — seule l'adresse change (section Hotspot, onglet
// Expérience, /app/settings/hotspot).
//
// Toutes ces cartes écrivent dans /api/settings (rang 3 serveur) : le hub
// n'affiche l'onglet Expérience qu'aux rôles propriétaire+ (canView
// « settings » — miroir client du requireRole Go).

import { useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  BadgeCheck,
  CalendarClock,
  Eye,
  Globe,
  ImagePlus,
  Image as ImageIcon,
  Images,
  Link as LinkIcon,
  MousePointerClick,
  Router as RouterIcon,
  Store,
  Ticket,
  UserPlus,
  X,
} from "lucide-react";
import { toast } from "sonner";

import { api, apiUpload, updateSettings } from "@/lib/hotspot/api";
import { SETTINGS_QUERY_KEY } from "@/components/hotspot/parts/sd-currency";
import { useI18n } from "@/lib/hotspot/i18n";
import type { AppSettings, ExpiryPolicyMode, PortalPromo, PortalSocial, PromoStats } from "@/lib/hotspot/types";
import { qrWithLogoDataUrl } from "@/components/hotspot/parts/template-render";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";

const MIKROTIK_STEPS = [
  { titleKey: "settings.guide.step1", textKey: "settings.guide.step1Text" },
  { titleKey: "settings.guide.step2", textKey: "settings.guide.step2Text" },
  { titleKey: "settings.guide.step3", textKey: "settings.guide.step3Text" },
];

/** Onglet « Expérience » — grille des cartes de règles du service (N°57-d) :
 * même contenu que l'ancien onglet Hotspot, même ordre. */
export function HotspotExperience({ settings }: { settings: AppSettings }) {
  return (
    <div className="grid grid-cols-1 gap-4 lg:grid-cols-2 lg:gap-6">
      {/* Expiration des vouchers (F1/F5) — politique de nettoyage cloud */}
      <ExpiryCard settings={settings} />

      {/* Import automatique des utilisateurs hors MikCloud (purge P1) —
          comportement de la synchro agent pour les inconnus du cloud */}
      <AutoImportCard settings={settings} />

      {/* Bouton « S'inscrire » du portail captif (N°46) — affichage
          dynamique piloté par le gérant */}
      <PortalJoinCard settings={settings} />

      {/* Vouchers — DNS + logo (F2) */}
      <VoucherCard settings={settings} />

      {/* Bannière du portail captif (N°45) — image tête de page login */}
      <PortalBannerCard settings={settings} />

      {/* Mode hospitalité du portail captif (N°55) — vitrine de
          l'établissement (promos produits R2, bienvenue, réseaux
          sociaux) à la place de la vitrine commerciale */}
      <PortalHospitalityCard settings={settings} />

      {/* Guide connexion routeur réel */}
      <HotspotGuideCard />
    </div>
  );
}

/** Guide de connexion MikroTik — 3 étapes + note (ancienne carte « Guide »
 * de l'onglet Hotspot, déplacée telle quelle). */
export function HotspotGuideCard() {
  const { t } = useI18n();
  return (
    <Card className="gap-4 border-primary/20 bg-primary/5 py-4 sm:py-6 lg:col-span-2">
      <CardHeader className="px-4 sm:px-6">
        <CardTitle className="flex items-center gap-2 text-base">
          <span className="flex size-8 items-center justify-center rounded-lg bg-primary/15 text-primary">
            <RouterIcon className="size-4" />
          </span>
          {t("settings.guide.title")}
        </CardTitle>
        <CardDescription>{t("settings.guide.desc")}</CardDescription>
      </CardHeader>
      <CardContent className="px-4 sm:px-6">
        <ol className="grid gap-4 sm:grid-cols-3">
          {MIKROTIK_STEPS.map((step, index) => (
            <li key={step.titleKey} className="rounded-lg border bg-card p-3">
              <p className="flex items-center gap-2 text-sm font-medium">
                <span className="flex size-6 shrink-0 items-center justify-center rounded-full bg-primary/15 text-xs font-semibold text-primary">
                  {index + 1}
                </span>
                {t(step.titleKey)}
              </p>
              <p className="mt-1.5 text-xs leading-relaxed text-muted-foreground">{t(step.textKey)}</p>
            </li>
          ))}
        </ol>
        <p className="mt-4 text-xs text-muted-foreground">{t("settings.guide.simulatedNote")}</p>
      </CardContent>
    </Card>
  );
}

// Carte Expiration des vouchers (F1/F5) — politique de nettoyage des expirés.
// Le moteur d'expiration du cloud (Tick) applique la politique automatiquement.
function ExpiryCard({ settings }: { settings: AppSettings }) {
  const { t } = useI18n();
  const queryClient = useQueryClient();
  const [mode, setMode] = useState<ExpiryPolicyMode>(
    settings.tenant.expiryPolicyMode === "remove" ? "remove" : "keep",
  );
  const [days, setDays] = useState(String(settings.tenant.expiryPolicyAfterDays ?? 30));

  const daysNum = parseInt(days, 10);
  const daysValid = Number.isInteger(daysNum) && daysNum >= 1 && daysNum <= 365;

  const saveMutation = useMutation({
    mutationFn: () => {
      const afterDays = mode === "remove" && daysValid ? daysNum : undefined;
      return api<AppSettings>("/api/settings", {
        method: "PUT",
        // Corps défensif : champs plats (forme du handler actuel) + forme imbriquée
        // « tenant » du contrat — le décodeur Go ignore les champs inconnus.
        body: {
          expiryPolicyMode: mode,
          expiryPolicyAfterDays: afterDays,
          tenant: { expiryPolicyMode: mode, expiryPolicyAfterDays: afterDays },
        },
      });
    },
    onSuccess: () => {
      toast.success(t("settings.expirySavedToast"));
      void queryClient.invalidateQueries({ queryKey: SETTINGS_QUERY_KEY });
    },
    onError: (error: Error) => toast.error(error.message),
  });

  return (
    <Card className="gap-4 py-4 sm:py-6">
      <CardHeader className="px-4 sm:px-6">
        <CardTitle className="flex items-center gap-2 text-base">
          <span className="flex size-8 items-center justify-center rounded-lg bg-primary/15 text-primary">
            <CalendarClock className="size-4" />
          </span>
          {t("settings.expiryCard")}
        </CardTitle>
        <CardDescription>{t("settings.expiryCardDesc")}</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-4 px-4 sm:px-6">
        <RadioGroup
          value={mode}
          onValueChange={(value) => setMode(value as ExpiryPolicyMode)}
          className="grid gap-3"
        >
          <label className="flex min-h-14 cursor-pointer items-start gap-3 rounded-lg border p-3 transition-colors has-[[data-state=checked]]:border-primary/50">
            <RadioGroupItem value="keep" className="mt-1" />
            <span className="text-sm">
              {t("settings.expiryKeep")}
              <span className="block text-xs font-normal text-muted-foreground">
                {t("settings.expiryKeepDesc")}
              </span>
            </span>
          </label>
          <label className="flex min-h-14 cursor-pointer items-start gap-3 rounded-lg border p-3 transition-colors has-[[data-state=checked]]:border-primary/50">
            <RadioGroupItem value="remove" className="mt-1" />
            <span className="text-sm">
              {t("settings.expiryRemove")}
              <span className="block text-xs font-normal text-muted-foreground">
                {t("settings.expiryRemoveDesc")}
              </span>
            </span>
          </label>
        </RadioGroup>

        {mode === "remove" && (
          <div className="grid gap-2">
            <Label htmlFor="expiry-days">{t("settings.expiryDays")}</Label>
            <Input
              id="expiry-days"
              type="number"
              min={1}
              max={365}
              value={days}
              onChange={(event) => setDays(event.target.value)}
              className="h-10"
              aria-invalid={!daysValid}
            />
            <p className={daysValid ? "text-xs text-muted-foreground" : "text-xs text-destructive"}>
              {daysValid ? t("settings.expiryDaysHint") : t("settings.expiryDaysInvalid")}
            </p>
          </div>
        )}
      </CardContent>
      <CardFooter className="justify-end px-4 sm:px-6">
        <Button
          className="h-10"
          onClick={() => saveMutation.mutate()}
          disabled={saveMutation.isPending || (mode === "remove" && !daysValid)}
        >
          {saveMutation.isPending ? t("common.saving") : t("common.save")}
        </Button>
      </CardFooter>
    </Card>
  );
}

// Carte Import automatique (purge P1) — comportement de la synchronisation
// agent pour les utilisateurs présents sur les routeurs mais inconnus du
// cloud (créés via Winbox ou un autre système). Défaut affiché = ACTIVÉ
// quand le champ est absent/undefined (comportement historique de découverte).
// Quand désactivé : jamais importés automatiquement — listés dans la santé
// du routeur (unknownOnRouter), adoption manuelle via l'outil d'import.
function AutoImportCard({ settings }: { settings: AppSettings }) {
  const { t } = useI18n();
  const queryClient = useQueryClient();
  // Lecture défensive : le champ peut vivre dans tenant{…} ou à plat selon
  // la version du backend déployée — absent = true (comportement historique).
  const [enabled, setEnabled] = useState<boolean>(
    settings.tenant.autoImportRouterUsers ?? settings.autoImportRouterUsers ?? true,
  );

  const saveMutation = useMutation({
    mutationFn: () => updateSettings({ autoImportRouterUsers: enabled }),
    onSuccess: () => {
      toast.success(t("settings.autoImport.savedToast"));
      void queryClient.invalidateQueries({ queryKey: SETTINGS_QUERY_KEY });
    },
    onError: (error: Error) => toast.error(error.message),
  });

  return (
    <Card className="gap-4 py-4 sm:py-6">
      <CardHeader className="px-4 sm:px-6">
        <CardTitle className="flex items-center gap-2 text-base">
          <span className="flex size-8 items-center justify-center rounded-lg bg-primary/15 text-primary">
            <UserPlus className="size-4" />
          </span>
          {t("settings.autoImport.title")}
        </CardTitle>
        <CardDescription>{t("settings.autoImport.desc")}</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-3 px-4 sm:px-6">
        <div className="flex items-center justify-between gap-3 rounded-lg border p-3">
          <Label htmlFor="auto-import-router-users" className="cursor-pointer text-sm">
            {t("settings.autoImport.switchLabel")}
          </Label>
          <Switch
            id="auto-import-router-users"
            checked={enabled}
            onCheckedChange={setEnabled}
            aria-label={t("settings.autoImport.aria")}
            className="shrink-0"
            disabled={saveMutation.isPending}
          />
        </div>
        {/* Les deux comportements sont décrits — celui du réglage courant
            est mis en avant, l'autre reste lisible (montre la conséquence
            du basculement avant d'enregistrer). */}
        <p
          className={
            enabled
              ? "text-xs leading-relaxed text-foreground"
              : "text-xs leading-relaxed text-muted-foreground"
          }
        >
          {t("settings.autoImport.enabledDesc")}
        </p>
        <p
          className={
            !enabled
              ? "text-xs leading-relaxed text-foreground"
              : "text-xs leading-relaxed text-muted-foreground"
          }
        >
          {t("settings.autoImport.disabledDesc")}
        </p>
      </CardContent>
      <CardFooter className="justify-end px-4 sm:px-6">
        <Button className="h-10" onClick={() => saveMutation.mutate()} disabled={saveMutation.isPending}>
          {saveMutation.isPending ? t("common.saving") : t("common.save")}
        </Button>
      </CardFooter>
    </Card>
  );
}

// Carte Bouton « S'inscrire » (N°46) — réglage dynamique de l'affichage du
// bouton d'inscription sur la page de connexion du portail captif. Le portail
// lit ce réglage dans la config (fallback inliné + fetch live) : activé → le
// bouton Mikhmon « Scanner un QR Code » devient « S'inscrire » (lien join
// ?mac= pré-injectée) ; désactivé → aucun bouton d'inscription.
function PortalJoinCard({ settings }: { settings: AppSettings }) {
  const { t } = useI18n();
  const queryClient = useQueryClient();
  // Lecture défensive : le champ vit dans tenant{…} (forme du contrat) —
  // absent (nil côté Go) = true (défaut effectif, comportement historique).
  const [enabled, setEnabled] = useState<boolean>(settings.tenant.joinButton ?? true);

  const saveMutation = useMutation({
    mutationFn: () => updateSettings({ joinButton: enabled }),
    onSuccess: () => {
      toast.success(t("settings.joinButton.savedToast"));
      void queryClient.invalidateQueries({ queryKey: SETTINGS_QUERY_KEY });
    },
    onError: (error: Error) => toast.error(error.message),
  });

  return (
    <Card className="gap-4 py-4 sm:py-6">
      <CardHeader className="px-4 sm:px-6">
        <CardTitle className="flex items-center gap-2 text-base">
          <span className="flex size-8 items-center justify-center rounded-lg bg-primary/15 text-primary">
            <BadgeCheck className="size-4" />
          </span>
          {t("settings.joinButton.title")}
        </CardTitle>
        <CardDescription>{t("settings.joinButton.desc")}</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-3 px-4 sm:px-6">
        <div className="flex items-center justify-between gap-3 rounded-lg border p-3">
          <Label htmlFor="portal-join-button" className="cursor-pointer text-sm">
            {t("settings.joinButton.switchLabel")}
          </Label>
          <Switch
            id="portal-join-button"
            checked={enabled}
            onCheckedChange={setEnabled}
            aria-label={t("settings.joinButton.aria")}
            className="shrink-0"
            disabled={saveMutation.isPending}
          />
        </div>
        {/* Les deux comportements sont décrits — celui du réglage courant
            est mis en avant, l'autre reste lisible (montre la conséquence
            du basculement avant d'enregistrer). */}
        <p
          className={
            enabled
              ? "text-xs leading-relaxed text-foreground"
              : "text-xs leading-relaxed text-muted-foreground"
          }
        >
          {t("settings.joinButton.enabledDesc")}
        </p>
        <p
          className={
            !enabled
              ? "text-xs leading-relaxed text-foreground"
              : "text-xs leading-relaxed text-muted-foreground"
          }
        >
          {t("settings.joinButton.disabledDesc")}
        </p>
      </CardContent>
      <CardFooter className="justify-end px-4 sm:px-6">
        <Button className="h-10" onClick={() => saveMutation.mutate()} disabled={saveMutation.isPending}>
          {saveMutation.isPending ? t("common.saving") : t("common.save")}
        </Button>
      </CardFooter>
    </Card>
  );
}

// Carte Vouchers (F2) — nom DNS du hotspot + logo affichés sur les tickets
// (variables {{dnsName}} et {{logo}} des modèles).
function VoucherCard({ settings }: { settings: AppSettings }) {
  const { t } = useI18n();
  const queryClient = useQueryClient();
  const [dnsName, setDnsName] = useState(settings.tenant.dnsName ?? "");
  const [logoUrl, setLogoUrl] = useState(settings.tenant.logoUrl ?? "");
  const fileInputRef = useRef<HTMLInputElement>(null);

  // Aperçu live « logo au centre du QR » — même fonction de composition que
  // les tickets réels ; se régénère dès que le logo importé change.
  const [qrPreview, setQrPreview] = useState("");
  useEffect(() => {
    let cancelled = false;
    qrWithLogoDataUrl("MIKCLOUD\nDEMO-2026", logoUrl || undefined)
      .then((url) => {
        if (!cancelled) setQrPreview(url);
      })
      .catch(() => {
        if (!cancelled) setQrPreview("");
      });
    return () => {
      cancelled = true;
    };
  }, [logoUrl]);

  // Logo : image ≤ 300 Ko encodée en data URL (contrat F2).
  function handleLogoFile(event: React.ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0];
    // Permet de re-sélectionner le même fichier après une erreur.
    event.target.value = "";
    if (!file) return;
    if (!file.type.startsWith("image/")) {
      toast.error(t("settings.logoNotImage"));
      return;
    }
    if (file.size > 300 * 1024) {
      toast.error(t("settings.logoTooBig"));
      return;
    }
    const reader = new FileReader();
    reader.onload = () => {
      if (typeof reader.result === "string") setLogoUrl(reader.result);
    };
    reader.onerror = () => toast.error(t("settings.logoReadError"));
    reader.readAsDataURL(file);
  }

  const saveMutation = useMutation({
    mutationFn: () =>
      api<AppSettings>("/api/settings", {
        method: "PUT",
        // Corps défensif : champs plats + forme imbriquée « tenant » (cf. ExpiryCard).
        body: {
          dnsName: dnsName.trim(),
          logoUrl,
          tenant: { dnsName: dnsName.trim(), logoUrl },
        },
      }),
    onSuccess: () => {
      toast.success(t("settings.voucherSavedToast"));
      void queryClient.invalidateQueries({ queryKey: SETTINGS_QUERY_KEY });
    },
    onError: (error: Error) => toast.error(error.message),
  });

  return (
    <Card className="gap-4 py-4 sm:py-6">
      <CardHeader className="px-4 sm:px-6">
        <CardTitle className="flex items-center gap-2 text-base">
          <span className="flex size-8 items-center justify-center rounded-lg bg-primary/15 text-primary">
            <Ticket className="size-4" />
          </span>
          {t("settings.voucherCard")}
        </CardTitle>
        <CardDescription>{t("settings.voucherCardDesc")}</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-4 px-4 sm:px-6 sm:grid-cols-2">
        <div className="grid gap-2">
          <Label htmlFor="voucher-dns">{t("settings.dnsName")}</Label>
          <div className="relative">
            <Globe className="absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" aria-hidden />
            <Input
              id="voucher-dns"
              className="h-10 pl-9"
              placeholder="wifi.mondomaine.ci"
              value={dnsName}
              onChange={(event) => setDnsName(event.target.value)}
              maxLength={100}
            />
          </div>
          <p className="text-xs text-muted-foreground">
            {t("settings.dnsNameHintPre")}
            <code className="font-mono text-[11px]">{"{{dnsName}}"}</code>
            {t("settings.dnsNameHintPost")}
          </p>
        </div>

        <div className="grid gap-2">
          <Label>{t("settings.logo")}</Label>
          <div className="flex items-center gap-3">
            <Avatar className="size-14 rounded-xl border bg-white">
              {logoUrl ? (
                <AvatarImage src={logoUrl} alt={t("settings.logoAlt")} className="object-contain" />
              ) : null}
              <AvatarFallback className="rounded-xl bg-muted text-muted-foreground">
                <ImageIcon className="size-5" aria-hidden />
              </AvatarFallback>
            </Avatar>
            <div className="flex flex-wrap gap-2">
              <Button
                type="button"
                variant="outline"
                className="h-10"
                onClick={() => fileInputRef.current?.click()}
              >
                <ImagePlus className="size-4" />
                {logoUrl ? t("settings.change") : t("settings.upload")}
              </Button>
              {logoUrl && (
                <Button
                  type="button"
                  variant="ghost"
                  className="h-10 text-destructive hover:text-destructive"
                  onClick={() => setLogoUrl("")}
                >
                  <X className="size-4" />
                  {t("settings.remove")}
                </Button>
              )}
            </div>
            <input
              ref={fileInputRef}
              type="file"
              accept="image/*"
              className="sr-only"
              onChange={handleLogoFile}
              aria-label={t("settings.logoInputAria")}
            />
          </div>
          <p className="text-xs text-muted-foreground">
            {t("settings.logoHintPre")}
            <code className="font-mono text-[11px]">{"{{logo}}"}</code>
            {t("settings.logoHintPost")}
          </p>
          <p className="text-xs font-medium">{t("settings.qrPreviewTitle")}</p>
          <div className="flex items-center gap-3 rounded-lg border bg-white p-3">
            {qrPreview ? (
              <img
                src={qrPreview}
                alt={t("settings.qrPreviewAlt")}
                className="size-20 shrink-0"
              />
            ) : (
              <Skeleton className="size-20 shrink-0" />
            )}
            <p className="text-xs text-muted-foreground">{t("settings.qrPreviewHint")}</p>
          </div>
        </div>
      </CardContent>
      <CardFooter className="justify-end px-4 sm:px-6">
        <Button
          className="h-10"
          onClick={() => saveMutation.mutate()}
          disabled={saveMutation.isPending || dnsName.trim().length > 100}
        >
          {saveMutation.isPending ? t("common.saving") : t("common.save")}
        </Button>
      </CardFooter>
    </Card>
  );
}

// Carte Bannière portail (N°45) — image affichée en tête de la page de login
// du portail captif (routeurs agent) et de la page visiteur WiFi. Deux sources
// acceptées par le backend : data URL (téléversement, ≤ 500 Ko) ou URL https
// (Cloudflare R2 et tout hébergeur d'images). Vide = portail sans bannière.
function PortalBannerCard({ settings }: { settings: AppSettings }) {
  const { t } = useI18n();
  const queryClient = useQueryClient();
  const [bannerUrl, setBannerUrl] = useState(settings.tenant.bannerUrl ?? "");
  const [uploading, setUploading] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);

  // N°53 — Téléversement : l'image part dans le stockage cloud R2 (POST
  // /api/media, ≤ 2 Mo, type sniffé côté serveur) et le champ reçoit une URL
  // https PERMANENTE servie par le même hôte que l'API — donc joignable
  // pré-authentification par le portail captif (walled-garden N°48). Repli
  // dégradé si le stockage est indisponible (sandbox, R2 non configuré) :
  // data URL intégrée ≤ 500 Ko, contrat N°45 inchangé — le gérant n'est
  // jamais bloqué.
  async function handleBannerFile(event: React.ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0];
    // Permet de re-sélectionner le même fichier après une erreur.
    event.target.value = "";
    if (!file) return;
    if (!file.type.startsWith("image/")) {
      toast.error(t("settings.logoNotImage"));
      return;
    }
    if (file.size > 2 * 1024 * 1024) {
      toast.error(t("settings.bannerTooBig"));
      return;
    }
    setUploading(true);
    try {
      const form = new FormData();
      form.append("file", file);
      const res = await apiUpload<{ url: string }>("/api/media", form, { timeoutMs: 60_000 });
      setBannerUrl(res.url);
      toast.success(t("settings.bannerUploadOk"));
    } catch {
      // Repli N°45 : image intégrée au compte (≤ 500 Ko).
      if (file.size <= 500 * 1024) {
        const reader = new FileReader();
        reader.onload = () => {
          if (typeof reader.result === "string") setBannerUrl(reader.result);
        };
        reader.onerror = () => toast.error(t("settings.logoReadError"));
        reader.readAsDataURL(file);
        toast.info(t("settings.bannerUploadFail"));
      } else {
        toast.error(t("settings.bannerUploadFailBig"));
      }
    } finally {
      setUploading(false);
    }
  }

  // Validation souple de l'URL collée : https:// requis (le portail et la
  // page WiFi sont servies en https — mixed content interdit), data URL
  // tolérée (elle vient du téléversement). Backend : même contrat (400 sinon).
  const bannerUrlInvalid =
    bannerUrl.trim() !== "" &&
    !bannerUrl.trim().startsWith("https://") &&
    !bannerUrl.trim().startsWith("data:image/");

  const saveMutation = useMutation({
    mutationFn: () =>
      api<AppSettings>("/api/settings", {
        method: "PUT",
        // Corps défensif : champs plats + forme imbriquée « tenant » (cf. VoucherCard).
        body: {
          bannerUrl: bannerUrl.trim(),
          tenant: { bannerUrl: bannerUrl.trim() },
        },
      }),
    onSuccess: () => {
      toast.success(t("settings.bannerSavedToast"));
      void queryClient.invalidateQueries({ queryKey: SETTINGS_QUERY_KEY });
    },
    onError: (error: Error) => toast.error(error.message),
  });

  return (
    <Card className="gap-4 py-4 sm:py-6">
      <CardHeader className="px-4 sm:px-6">
        <CardTitle className="flex items-center gap-2 text-base">
          <span className="flex size-8 items-center justify-center rounded-lg bg-primary/15 text-primary">
            <Images className="size-4" />
          </span>
          {t("settings.bannerCard")}
        </CardTitle>
        <CardDescription>{t("settings.bannerCardDesc")}</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-4 px-4 sm:px-6">
        <div className="grid gap-2">
          <Label htmlFor="banner-url">{t("settings.bannerUrl")}</Label>
          <div className="flex flex-wrap items-center gap-2">
            <Input
              id="banner-url"
              className="h-10 min-w-0 flex-1"
              placeholder={t("settings.bannerUrlPlaceholder")}
              value={bannerUrl.startsWith("data:image/") ? "" : bannerUrl}
              onChange={(event) => setBannerUrl(event.target.value)}
              aria-invalid={bannerUrlInvalid || undefined}
            />
            <Button
              type="button"
              variant="outline"
              className="h-10"
              onClick={() => fileInputRef.current?.click()}
              disabled={uploading}
            >
              <ImagePlus className="size-4" />
              {uploading ? t("settings.uploading") : t("settings.upload")}
            </Button>
            {bannerUrl && (
              <Button
                type="button"
                variant="ghost"
                className="h-10 text-destructive hover:text-destructive"
                onClick={() => setBannerUrl("")}
              >
                <X className="size-4" />
                {t("settings.remove")}
              </Button>
            )}
            <input
              ref={fileInputRef}
              type="file"
              accept="image/*"
              className="sr-only"
              onChange={handleBannerFile}
              aria-label={t("settings.bannerInputAria")}
            />
          </div>
          <p className="text-xs text-muted-foreground">{t("settings.bannerUrlHint")}</p>
        </div>

        {/* Aperçu live — même rendu que la page du portail (objet cover, coins arrondis) */}
        {bannerUrl && !bannerUrlInvalid && (
          <div className="grid gap-2">
            <p className="text-xs font-medium">{t("settings.bannerPreviewAlt")}</p>
            <div className="overflow-hidden rounded-xl border bg-muted">
              <img
                src={bannerUrl}
                alt={t("settings.bannerPreviewAlt")}
                className="h-32 w-full object-cover"
              />
            </div>
          </div>
        )}
      </CardContent>
      <CardFooter className="justify-end px-4 sm:px-6">
        <Button
          className="h-10"
          onClick={() => saveMutation.mutate()}
          disabled={saveMutation.isPending || bannerUrlInvalid}
        >
          {saveMutation.isPending ? t("common.saving") : t("common.save")}
        </Button>
      </CardFooter>
    </Card>
  );
}

// Carte Portail hospitalité (N°55) — le gérant choisit le MODE d'affichage du
// portail captif : « commercial » (grille tarifaire + Wave, défaut) ou
// « hospitality » (vitrine de son établissement : message de bienvenue, promos
// produits avec images stockées dans R2 via N°53, liens réseaux sociaux).
// Les listes sont éditées structurées ici et VALIDÉES/sérialisées côté
// backend (≤ 6 promos, ≤ 4 liens, URLs https) — cf. handlers_settings.go.
function PortalHospitalityCard({ settings }: { settings: AppSettings }) {
  const { t } = useI18n();
  const queryClient = useQueryClient();
  const [style, setStyle] = useState(settings.tenant.portalStyle ?? "");
  const [welcome, setWelcome] = useState(settings.tenant.portalWelcome ?? "");
  const [promos, setPromos] = useState<PortalPromo[]>(() => {
    try {
      const parsed = JSON.parse(settings.tenant.portalPromos || "[]") as PortalPromo[];
      return Array.isArray(parsed) ? parsed : [];
    } catch {
      return [];
    }
  });
  const [socials, setSocials] = useState<PortalSocial[]>(() => {
    try {
      const parsed = JSON.parse(settings.tenant.portalSocials || "[]") as PortalSocial[];
      return Array.isArray(parsed) ? parsed : [];
    } catch {
      return [];
    }
  });
  const [uploadingIdx, setUploadingIdx] = useState<number | null>(null);

  // N°56 — analytics de la vitrine : impressions/clics par promo. Chargé
  // quand la carte est ouverte en mode hospitalité ; rafraîchi après chaque
  // enregistrement (invalidation SETTINGS_QUERY_KEY + refetch à l'ouverture).
  const statsQuery = useQuery({
    queryKey: ["promo-stats"],
    queryFn: () => api<PromoStats>("/api/promos/stats"),
    enabled: style === "hospitality" && promos.length > 0,
    staleTime: 30_000,
  });
  const stats = statsQuery.data;

  const saveMutation = useMutation({
    mutationFn: () =>
      api<AppSettings>("/api/settings", {
        method: "PUT",
        // Corps défensif (pattern VoucherCard) : champs plats + forme
        // imbriquée tenant{…} — le plat prime côté backend.
        body: {
          portalStyle: style,
          portalWelcome: welcome,
          portalPromos: promos,
          portalSocials: socials,
          tenant: { portalStyle: style, portalWelcome: welcome, portalPromos: promos, portalSocials: socials },
        },
      }),
    onSuccess: () => {
      toast.success(t("settings.hosp.savedToast"));
      void queryClient.invalidateQueries({ queryKey: SETTINGS_QUERY_KEY });
    },
    onError: (error: Error) => toast.error(error.message),
  });

  // Image d'une promo : téléversée vers R2 (N°53) — URL permanente https.
  async function handlePromoImage(idx: number, file?: File) {
    if (!file) return;
    if (!file.type.startsWith("image/")) {
      toast.error(t("settings.logoNotImage"));
      return;
    }
    if (file.size > 2 * 1024 * 1024) {
      toast.error(t("settings.bannerTooBig"));
      return;
    }
    setUploadingIdx(idx);
    try {
      const form = new FormData();
      form.append("file", file);
      const res = await apiUpload<{ url: string }>("/api/media", form, { timeoutMs: 60_000 });
      setPromos((list) => list.map((it, i) => (i === idx ? { ...it, imageUrl: res.url } : it)));
      toast.success(t("settings.bannerUploadOk"));
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t("settings.hosp.imageFail"));
    } finally {
      setUploadingIdx(null);
    }
  }

  return (
    <Card className="gap-4 py-4 sm:py-6">
      <CardHeader className="px-4 sm:px-6">
        <CardTitle className="flex items-center gap-2 text-base">
          <span className="flex size-8 items-center justify-center rounded-lg bg-primary/15 text-primary">
            <Store className="size-4" />
          </span>
          {t("settings.hosp.card")}
        </CardTitle>
        <CardDescription>{t("settings.hosp.cardDesc")}</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-4 px-4 sm:px-6">
        <div className="grid gap-2">
          <Label htmlFor="portal-style">{t("settings.hosp.mode")}</Label>
          <Select
            value={style === "hospitality" ? "hospitality" : "commercial"}
            onValueChange={(value) => setStyle(value === "hospitality" ? "hospitality" : "")}
          >
            <SelectTrigger id="portal-style" className="h-10">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="commercial">{t("settings.hosp.modeCommercial")}</SelectItem>
              <SelectItem value="hospitality">{t("settings.hosp.modeHospitality")}</SelectItem>
            </SelectContent>
          </Select>
          <p className="text-xs text-muted-foreground">{t("settings.hosp.modeHint")}</p>
        </div>

        {style === "hospitality" && (
          <>
            <div className="grid gap-2">
              <Label htmlFor="portal-welcome">{t("settings.hosp.welcome")}</Label>
              <Textarea
                id="portal-welcome"
                className="min-h-[70px]"
                maxLength={200}
                value={welcome}
                onChange={(event) => setWelcome(event.target.value)}
                placeholder={t("settings.hosp.welcomePlaceholder")}
              />
            </div>

            <div className="grid gap-2">
              <div className="flex items-center justify-between">
                <Label>{t("settings.hosp.promos")}</Label>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  disabled={promos.length >= 6}
                  onClick={() => setPromos((list) => [...list, { title: "", desc: "", imageUrl: "", priceLabel: "" }])}
                >
                  <ImagePlus className="size-4" />
                  {t("settings.hosp.addPromo")}
                </Button>
              </div>
              {promos.length === 0 && (
                <p className="text-xs text-muted-foreground">{t("settings.hosp.promosEmpty")}</p>
              )}
              {promos.map((promo, idx) => (
                <div key={idx} className="grid gap-2 rounded-lg border p-3">
                  <div className="flex items-center gap-2">
                    {promo.imageUrl ? (
                      <img
                        src={promo.imageUrl}
                        alt={promo.title || `promo-${idx + 1}`}
                        className="size-10 rounded-md object-cover"
                        onError={(event) => {
                          event.currentTarget.style.opacity = "0.3";
                        }}
                      />
                    ) : (
                      <span className="flex size-10 items-center justify-center rounded-md border bg-muted text-muted-foreground">
                        <ImageIcon className="size-4" />
                      </span>
                    )}
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      disabled={uploadingIdx === idx}
                      onClick={() => document.getElementById(`promo-file-${idx}`)?.click()}
                    >
                      <ImagePlus className="size-4" />
                      {uploadingIdx === idx ? t("settings.uploading") : t("settings.upload")}
                    </Button>
                    {promo.imageUrl && (
                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        className="text-destructive hover:text-destructive"
                        onClick={() => setPromos((list) => list.map((it, i) => (i === idx ? { ...it, imageUrl: "" } : it)))}
                      >
                        <X className="size-4" />
                      </Button>
                    )}
                    <input
                      id={`promo-file-${idx}`}
                      type="file"
                      accept="image/*"
                      className="sr-only"
                      onChange={(event) => {
                        const file = event.target.files?.[0];
                        event.target.value = "";
                        void handlePromoImage(idx, file);
                      }}
                      aria-label={t("settings.bannerInputAria")}
                    />
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      className="ml-auto text-destructive hover:text-destructive"
                      onClick={() => setPromos((list) => list.filter((_, i) => i !== idx))}
                    >
                      <X className="size-4" />
                      {t("settings.remove")}
                    </Button>
                  </div>
                  <Input
                    className="h-9"
                    maxLength={60}
                    placeholder={t("settings.hosp.promoTitle")}
                    value={promo.title}
                    onChange={(event) =>
                      setPromos((list) => list.map((it, i) => (i === idx ? { ...it, title: event.target.value } : it)))
                    }
                  />
                  <Input
                    className="h-9"
                    maxLength={160}
                    placeholder={t("settings.hosp.promoDesc")}
                    value={promo.desc}
                    onChange={(event) =>
                      setPromos((list) => list.map((it, i) => (i === idx ? { ...it, desc: event.target.value } : it)))
                    }
                  />
                  <Input
                    className="h-9"
                    maxLength={30}
                    placeholder={t("settings.hosp.promoPrice")}
                    value={promo.priceLabel}
                    onChange={(event) =>
                      setPromos((list) => list.map((it, i) => (i === idx ? { ...it, priceLabel: event.target.value } : it)))
                    }
                  />
                  <div className="relative">
                    <LinkIcon className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
                    <Input
                      className="h-9 pl-8"
                      maxLength={300}
                      inputMode="url"
                      placeholder={t("settings.hosp.promoLink")}
                      value={promo.link ?? ""}
                      onChange={(event) =>
                        setPromos((list) => list.map((it, i) => (i === idx ? { ...it, link: event.target.value } : it)))
                      }
                    />
                  </div>
                </div>
              ))}
            </div>

            {/* N°56 — analytics : « votre menu vu N fois cette semaine ». Les
                compteurs démarrent dès les premières connexions au portail. */}
            {promos.length > 0 && (
              <div className="grid gap-2 rounded-lg border bg-muted/30 p-3">
                <div className="flex items-center gap-2">
                  <Eye className="size-4 text-primary" />
                  <span className="text-sm font-semibold">{t("settings.hosp.statsTitle")}</span>
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    className="ml-auto h-8 px-2 text-xs"
                    disabled={statsQuery.isFetching}
                    onClick={() => void statsQuery.refetch()}
                  >
                    {statsQuery.isFetching ? t("common.refreshing") : t("common.refresh")}
                  </Button>
                </div>
                {stats && (
                  <p className="text-sm text-primary font-medium">
                    {t("settings.hosp.statsHeadline").replace("{n}", String(stats.totals.impression.week))}
                  </p>
                )}
                {stats && stats.totals.impression.week === 0 && (
                  <p className="text-xs text-muted-foreground">{t("settings.hosp.statsEmpty")}</p>
                )}
                {stats?.promos.map((p) => (
                  <div key={p.id} className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs">
                    <span className="font-medium text-foreground min-w-0 truncate max-w-full sm:max-w-[16rem]">{p.title || p.id}</span>
                    <span className="inline-flex items-center gap-1 text-muted-foreground">
                      <Eye className="size-3" />
                      {t("settings.hosp.statsViews").replace("{w}", String(p.impressions.week)).replace("{t}", String(p.impressions.total))}
                    </span>
                    {p.clicks.week > 0 && (
                      <span className="inline-flex items-center gap-1 text-muted-foreground">
                        <MousePointerClick className="size-3" />
                        {t("settings.hosp.statsClicks").replace("{w}", String(p.clicks.week)).replace("{t}", String(p.clicks.total))}
                      </span>
                    )}
                  </div>
                ))}
                <p className="text-xs text-muted-foreground">{t("settings.hosp.statsHint")}</p>
              </div>
            )}

            <div className="grid gap-2">
              <div className="flex items-center justify-between">
                <Label>{t("settings.hosp.socials")}</Label>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  disabled={socials.length >= 4}
                  onClick={() => setSocials((list) => [...list, { label: "", url: "" }])}
                >
                  <UserPlus className="size-4" />
                  {t("settings.hosp.addSocial")}
                </Button>
              </div>
              {socials.length === 0 && (
                <p className="text-xs text-muted-foreground">{t("settings.hosp.socialsEmpty")}</p>
              )}
              {socials.map((social, idx) => (
                <div key={idx} className="flex flex-wrap items-center gap-2">
                  <Input
                    className="h-9 w-36"
                    maxLength={30}
                    placeholder={t("settings.hosp.socialLabel")}
                    value={social.label}
                    onChange={(event) =>
                      setSocials((list) => list.map((it, i) => (i === idx ? { ...it, label: event.target.value } : it)))
                    }
                  />
                  <Input
                    className="h-9 min-w-0 flex-1"
                    maxLength={200}
                    placeholder="https://…"
                    value={social.url}
                    onChange={(event) =>
                      setSocials((list) => list.map((it, i) => (i === idx ? { ...it, url: event.target.value } : it)))
                    }
                  />
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    className="text-destructive hover:text-destructive"
                    onClick={() => setSocials((list) => list.filter((_, i) => i !== idx))}
                  >
                    <X className="size-4" />
                  </Button>
                </div>
              ))}
            </div>
          </>
        )}

        <Button
          type="button"
          className="w-full sm:w-auto"
          disabled={saveMutation.isPending}
          onClick={() => saveMutation.mutate()}
        >
          {saveMutation.isPending ? t("common.saving") : t("common.save")}
        </Button>
      </CardContent>
    </Card>
  );
}
