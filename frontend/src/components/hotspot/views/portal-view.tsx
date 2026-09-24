"use client";

// N°184 — contenu « Portail » de la section Hotspot (onglet du hub
// components/hotspot/views/hotspot-view.tsx — /app/settings/hotspot/portail).
//
// REFONTE « option 3 » validée par l'opérateur (N°182) : le portail captif se
// personnalise à TROIS niveaux, chaque routeur servant le premier portail
// défini en remontant sa chaîne ROUTEUR → SITE → COMPTE :
//
//   - COMPTE (N°184, ex-onglet Expérience) : le formulaire « Portail du
//     compte » en tête de l'onglet — la BASE de la chaîne, au même endroit
//     que ses surcharges (rang 3, masqué au gérant) ;
//   - SITES : regroupements de routeurs par établissement (bâtiment,
//     boutique, campus) portant une identité de portail (nom affiché, logo,
//     bannière, Wave, style, WhatsApp, ticker…) ;
//   - ROUTEURS : un Select de site par routeur + une surcharge INDIVIDUELLE
//     du portail (le niveau le plus fin).
//
// Tout changement (formulaire du compte, assignation, surcharge) rejoint la
// signature de déploiement côté backend → re-déploiement automatique au
// check-in (≤ 45 s), exactement comme un changement de branding du compte
// (N°135). L'aperçu par routeur (existant N°35-d) reflète la chaîne résolue.
//
// Sémantique des surcharges : VIDE = HÉRITE (un champ vide ne touche rien —
// la surcharge ne peut pas masquer un élément que le compte affiche ; pour
// ce cas, vider le champ côté compte et le surcharger ailleurs).

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  AlertTriangle,
  Building2,
  CheckCircle2,
  Clock,
  Eye,
  Loader2,
  MapPin,
  Monitor,
  Palette,
  Pencil,
  Plus,
  RefreshCw,
  Router as RouterIcon,
  Trash2,
} from "lucide-react";
import { toast } from "sonner";

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
import { ScrollArea } from "@/components/ui/scroll-area";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";

import { EmptyState } from "@/components/hotspot/empty-state";
import { AccountPortalForm } from "@/components/hotspot/parts/hotspot-cards";
import { useSettings } from "@/components/hotspot/parts/sd-currency";
import {
  AccountActivity,
  api,
  assignRouterSite,
  createSite,
  deleteSite,
  fetchAccountActivity,
  fetchRouterPortalPreview,
  fetchSites,
  redeployRouterPortal,
  updateRouterPortal,
  updateSite,
} from "@/lib/hotspot/api";
import { useI18n } from "@/lib/hotspot/i18n";
import {
  parsePortalOverride,
  type PortalOverrideData,
  type RouterDevice,
  type SiteResponse,
} from "@/lib/hotspot/types";

// ---------------------------------------------------------------------------
// Formulaire de surcharge (partagé site / routeur)
// ---------------------------------------------------------------------------

/** OverrideForm — état local du formulaire de surcharge. Les listes sont
 * éditées en « une entrée par ligne » (textarea) : simple, lisible, borné. */
interface OverrideForm {
  displayName: string;
  logoUrl: string;
  bannerUrl: string;
  waveLink: string;
  portalStyle: "" | "commercial" | "hospitality";
  portalWelcome: string;
  waNumber: string;
  waLabel: string;
  tickerText: string;
  servicesText: string;
  slidesText: string;
}

/** formFromOverride — décode une surcharge persistée (JSON canonique) vers
 * l'état local du formulaire. Vide/absente → formulaire vierge. */
function formFromOverride(raw?: string): OverrideForm {
  const ov: PortalOverrideData = parsePortalOverride(raw);
  return {
    displayName: ov.displayName ?? "",
    logoUrl: ov.logoUrl ?? "",
    bannerUrl: ov.bannerUrl ?? "",
    waveLink: ov.waveLink ?? "",
    portalStyle: (ov.portalStyle as OverrideForm["portalStyle"]) ?? "",
    portalWelcome: ov.portalWelcome ?? "",
    waNumber: ov.portalWhatsapp?.number ?? "",
    waLabel: ov.portalWhatsapp?.label ?? "",
    tickerText: (ov.portalTicker ?? []).join("\n"),
    servicesText: (ov.portalServices ?? []).map((s) => s.label).join("\n"),
    slidesText: (ov.portalSlides ?? []).join("\n"),
  };
}

/** lines — découpe un textarea en lignes nettoyées (vides ignorées, bornées). */
function lines(text: string, max: number): string[] {
  const out = text
    .split("\n")
    .map((l) => l.trim())
    .filter((l) => l !== "")
    .slice(0, max);
  return out;
}

/** formToPayload — construit le corps de surcharge pour l'API. Les champs
 * vides ne partent PAS (sémantique vide = hérite côté backend). */
function formToPayload(f: OverrideForm): Record<string, unknown> {
  const payload: Record<string, unknown> = {};
  if (f.displayName.trim()) payload.displayName = f.displayName.trim();
  if (f.logoUrl.trim()) payload.logoUrl = f.logoUrl.trim();
  if (f.bannerUrl.trim()) payload.bannerUrl = f.bannerUrl.trim();
  if (f.waveLink.trim()) payload.waveLink = f.waveLink.trim();
  if (f.portalStyle) payload.portalStyle = f.portalStyle;
  if (f.portalWelcome.trim()) payload.portalWelcome = f.portalWelcome.trim();
  const ticker = lines(f.tickerText, 5);
  if (ticker.length > 0) payload.portalTicker = ticker;
  const services = lines(f.servicesText, 6);
  if (services.length > 0) payload.portalServices = services.map((label) => ({ label }));
  const slides = lines(f.slidesText, 3);
  if (slides.length > 0) payload.portalSlides = slides;
  if (f.waNumber.trim()) {
    payload.portalWhatsapp = { number: f.waNumber.trim(), label: f.waLabel.trim() };
  }
  return payload;
}

/** OverrideFields — les champs de surcharge (visibles dans les dialogs site
 * et routeur). Chaque champ porte la mention « vide = hérite ». */
function OverrideFields({
  form,
  patch,
  t,
}: {
  form: OverrideForm;
  patch: (p: Partial<OverrideForm>) => void;
  t: (key: string, fallback?: string) => string;
}) {
  return (
    <div className="space-y-4">
      <div className="grid gap-4 sm:grid-cols-2">
        <div className="space-y-2">
          <Label htmlFor="ov-name">{t("portal.displayName")}</Label>
          <Input
            id="ov-name"
            value={form.displayName}
            onChange={(e) => patch({ displayName: e.target.value })}
            maxLength={80}
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="ov-style">{t("portal.style")}</Label>
          <Select
            value={form.portalStyle || "inherit"}
            onValueChange={(v) => patch({ portalStyle: v === "inherit" ? "" : (v as OverrideForm["portalStyle"]) })}
          >
            <SelectTrigger id="ov-style">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="inherit">{t("portal.styleInherit")}</SelectItem>
              <SelectItem value="commercial">{t("portal.styleCommercial")}</SelectItem>
              <SelectItem value="hospitality">{t("portal.styleHospitality")}</SelectItem>
            </SelectContent>
          </Select>
        </div>
      </div>
      <div className="space-y-2">
        <Label htmlFor="ov-logo">{t("portal.logoUrl")}</Label>
        <Input
          id="ov-logo"
          value={form.logoUrl}
          onChange={(e) => patch({ logoUrl: e.target.value })}
          placeholder="data:image/png;base64,…"
        />
      </div>
      <div className="space-y-2">
        <Label htmlFor="ov-banner">{t("portal.bannerUrl")}</Label>
        <Input
          id="ov-banner"
          value={form.bannerUrl}
          onChange={(e) => patch({ bannerUrl: e.target.value })}
          placeholder="data:image/… ou https://…"
        />
      </div>
      <div className="space-y-2">
        <Label htmlFor="ov-wave">{t("portal.waveLink")}</Label>
        <Input
          id="ov-wave"
          value={form.waveLink}
          onChange={(e) => patch({ waveLink: e.target.value })}
          placeholder="https://pay.wave.com/m/…/c/ci/"
        />
      </div>
      <div className="space-y-2">
        <Label htmlFor="ov-welcome">{t("portal.welcome")}</Label>
        <Textarea
          id="ov-welcome"
          value={form.portalWelcome}
          onChange={(e) => patch({ portalWelcome: e.target.value })}
          maxLength={200}
          rows={2}
        />
      </div>
      <div className="grid gap-4 sm:grid-cols-2">
        <div className="space-y-2">
          <Label htmlFor="ov-wa">{t("portal.whatsappNumber")}</Label>
          <Input
            id="ov-wa"
            value={form.waNumber}
            onChange={(e) => patch({ waNumber: e.target.value })}
            placeholder="2250708091012"
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="ov-wa-label">{t("portal.whatsappLabel")}</Label>
          <Input
            id="ov-wa-label"
            value={form.waLabel}
            onChange={(e) => patch({ waLabel: e.target.value })}
            maxLength={30}
          />
        </div>
      </div>
      <div className="space-y-2">
        <Label htmlFor="ov-ticker">{t("portal.ticker")}</Label>
        <Textarea
          id="ov-ticker"
          value={form.tickerText}
          onChange={(e) => patch({ tickerText: e.target.value })}
          rows={3}
        />
      </div>
      <div className="space-y-2">
        <Label htmlFor="ov-services">{t("portal.services")}</Label>
        <Textarea
          id="ov-services"
          value={form.servicesText}
          onChange={(e) => patch({ servicesText: e.target.value })}
          rows={3}
        />
      </div>
      <div className="space-y-2">
        <Label htmlFor="ov-slides">{t("portal.slides")}</Label>
        <Textarea
          id="ov-slides"
          value={form.slidesText}
          onChange={(e) => patch({ slidesText: e.target.value })}
          rows={3}
        />
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Dialog site (création / édition)
// ---------------------------------------------------------------------------

/** SiteDialog — création et édition d'un site (champs descriptifs + identité
 * de portail). site null = création ; sinon édition. */
function SiteDialog({
  site,
  onClose,
  t,
}: {
  site: SiteResponse | null;
  onClose: () => void;
  t: (key: string, fallback?: string) => string;
}) {
  const queryClient = useQueryClient();
  const [name, setName] = useState(site?.name ?? "");
  const [location, setLocation] = useState(site?.location ?? "");
  const [description, setDescription] = useState(site?.description ?? "");
  const [form, setForm] = useState<OverrideForm>(() => formFromOverride(site?.portalOverride));
  const patch = (p: Partial<OverrideForm>) => setForm((f) => ({ ...f, ...p }));

  const mutation = useMutation({
    mutationFn: async () => {
      const override = formToPayload(form);
      const body = { name: name.trim(), location: location.trim(), description: description.trim(), portalOverride: override };
      if (site) return updateSite(site.id, body);
      return createSite(body);
    },
    onSuccess: (_data, _vars, ctx) => {
      toast.success(site ? t("portal.siteUpdated") : t("portal.siteCreated"), {
        description: t("portal.siteSaved"),
      });
      queryClient.invalidateQueries({ queryKey: ["/api/sites"] });
      queryClient.invalidateQueries({ queryKey: ["/api/activity"] });
      void ctx;
      onClose();
    },
    onError: (err: Error) => toast.error(err.message),
  });

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>{site ? t("portal.siteEdit") : t("portal.newSite")}</DialogTitle>
          <DialogDescription>{t("portal.sitesHint")}</DialogDescription>
        </DialogHeader>
        <div className="space-y-5">
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="site-name">{t("portal.siteName")}</Label>
              <Input
                id="site-name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                maxLength={60}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="site-loc">{t("portal.siteLocation")}</Label>
              <Input
                id="site-loc"
                value={location}
                onChange={(e) => setLocation(e.target.value)}
                maxLength={120}
              />
            </div>
          </div>
          <div className="space-y-2">
            <Label htmlFor="site-desc">{t("portal.siteDescription")}</Label>
            <Textarea
              id="site-desc"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              maxLength={200}
              rows={2}
            />
          </div>
          <div className="space-y-3 rounded-xl border bg-muted/40 p-4">
            <div className="flex items-center gap-2 text-sm font-medium">
              <Palette className="size-4 text-primary" aria-hidden="true" />
              {t("portal.identitySection")}
            </div>
            <OverrideFields form={form} patch={patch} t={t} />
          </div>
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose} disabled={mutation.isPending}>
            {t("portal.cancel")}
          </Button>
          <Button type="button" onClick={() => mutation.mutate()} disabled={mutation.isPending || !name.trim()}>
            {mutation.isPending ? <Loader2 className="size-4 animate-spin" /> : null}
            {t("portal.save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

// ---------------------------------------------------------------------------
// Dialog surcharge individuelle du portail d'un routeur
// ---------------------------------------------------------------------------

/** RouterPortalDialog — surcharge INDIVIDUELLE du portail d'un routeur (le
 * niveau le plus fin de la chaîne). « Réinitialiser » renvoie un corps vide :
 * le routeur hérite de son site puis du compte. */
function RouterPortalDialog({
  router,
  onClose,
  t,
}: {
  router: RouterDevice;
  onClose: () => void;
  t: (key: string, fallback?: string) => string;
}) {
  const queryClient = useQueryClient();
  const [form, setForm] = useState<OverrideForm>(() => formFromOverride(router.portalOverride));
  const patch = (p: Partial<OverrideForm>) => setForm((f) => ({ ...f, ...p }));

  const mutation = useMutation({
    mutationFn: (payload: Record<string, unknown>) => updateRouterPortal(router.id, payload),
    onSuccess: () => {
      toast.success(t("portal.overrideSaved"));
      queryClient.invalidateQueries({ queryKey: ["/api/routers"] });
      queryClient.invalidateQueries({ queryKey: ["/api/activity"] });
      onClose();
    },
    onError: (err: Error) => toast.error(err.message),
  });

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t("portal.customizeTitle", `Portail de «${router.name}»`)}</DialogTitle>
          <DialogDescription>{t("portal.customizeHint")}</DialogDescription>
        </DialogHeader>
        <OverrideFields form={form} patch={patch} t={t} />
        <DialogFooter className="flex-row justify-between gap-2 sm:justify-between">
          <Button
            type="button"
            variant="outline"
            onClick={() => mutation.mutate({})}
            disabled={mutation.isPending}
          >
            {t("portal.resetOverride")}
          </Button>
          <div className="flex gap-2">
            <Button type="button" variant="outline" onClick={onClose} disabled={mutation.isPending}>
              {t("portal.cancel")}
            </Button>
            <Button
              type="button"
              onClick={() => mutation.mutate(formToPayload(form))}
              disabled={mutation.isPending}
            >
              {mutation.isPending ? <Loader2 className="size-4 animate-spin" /> : null}
              {t("portal.save")}
            </Button>
          </div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

// ---------------------------------------------------------------------------
// Contenu principal (onglet Portail du hub Hotspot)
// ---------------------------------------------------------------------------

/** Régime de portail d'un routeur : personnalisé (surcharge routeur) /
 * site (assigné à un site) / compte (unifié). */
function portalRegime(router: RouterDevice, sites: SiteResponse[]) {
  if (router.portalOverride) return "custom" as const;
  if (router.siteId) {
    const site = sites.find((s) => s.id === router.siteId);
    if (site) return "site" as const;
  }
  return "account" as const;
}

/** Contenu de l'onglet « Portail » du hub Hotspot — N°184 : la chaîne
 * ENTIÈRE au même endroit : portail du compte (propriétaire seulement),
 * sections Sites puis Routeurs, journal des déploiements en pied. */
export function PortalContent({
  withAccount,
  onDirtyChange,
}: {
  /** N°184 — affiche le formulaire « Portail du compte » (base de la
   * chaîne) : rang 3 (PUT /api/settings), masqué au gérant — miroir
   * canView côté hub. */
  withAccount: boolean;
  /** Remonte le compteur de saisie du formulaire au hub — garde de sortie
   * d'onglet (N°142). Stable (useCallback côté hub). */
  onDirtyChange?: (count: number) => void;
}) {
  const { t, tf } = useI18n();
  const queryClient = useQueryClient();

  // N°184 — réglages du compte : alimentent le formulaire « Portail du
  // compte » (chargé seulement pour le propriétaire, pas pour le gérant).
  const settingsQuery = useSettings();

  // Liste des routeurs et des sites du compte.
  const routersQuery = useQuery<RouterDevice[]>({
    queryKey: ["/api/routers"],
    queryFn: () => api<RouterDevice[]>("/api/routers"),
  });
  const sitesQuery = useQuery<SiteResponse[]>({
    queryKey: ["/api/sites"],
    queryFn: fetchSites,
  });

  // Journal d'activité (pour filtrer les événements portail).
  const activityQuery = useQuery<AccountActivity[]>({
    queryKey: ["/api/activity"],
    queryFn: () => fetchAccountActivity(100),
    refetchInterval: 30_000, // refresh auto 30 s
  });

  const routers = (routersQuery.data ?? []).filter((r) => r.mode === "agent");
  const sites = sitesQuery.data ?? [];

  // Filtre le journal sur les événements portail.
  const portalActivity = (activityQuery.data ?? []).filter(
    (a) =>
      a.type === "router" &&
      (/Portail captif déployé/i.test(a.message) ||
        /Re-déploiement du portail/i.test(a.message) ||
        /Portail du routeur/i.test(a.message)),
  );

  // États locaux : dialogs.
  const [siteDialogFor, setSiteDialogFor] = useState<SiteResponse | null | undefined>(undefined); // undefined fermé, null création
  const [deleteFor, setDeleteFor] = useState<SiteResponse | null>(null);
  const [customizeFor, setCustomizeFor] = useState<RouterDevice | null>(null);
  const [previewFor, setPreviewFor] = useState<RouterDevice | null>(null);
  const [redeployFor, setRedeployFor] = useState<RouterDevice | null>(null);

  // Mutation re-déploiement.
  const redeployMutation = useMutation({
    mutationFn: (routerId: string) => redeployRouterPortal(routerId),
    onSuccess: (data) => {
      toast.success(t("portal.redeployQueued"), { description: data.message });
      setRedeployFor(null);
      queryClient.invalidateQueries({ queryKey: ["/api/routers"] });
      queryClient.invalidateQueries({ queryKey: ["/api/activity"] });
    },
    onError: (err: Error) => toast.error(err.message),
  });

  // Mutation assignation de site (Select par routeur).
  const assignMutation = useMutation({
    mutationFn: ({ routerId, siteId }: { routerId: string; siteId: string }) =>
      assignRouterSite(routerId, siteId),
    onSuccess: (data) => {
      toast.success(data.message);
      queryClient.invalidateQueries({ queryKey: ["/api/routers"] });
      queryClient.invalidateQueries({ queryKey: ["/api/sites"] });
      queryClient.invalidateQueries({ queryKey: ["/api/activity"] });
    },
    onError: (err: Error) => toast.error(err.message),
  });

  // Mutation suppression de site (détache les routeurs).
  const deleteMutation = useMutation({
    mutationFn: (siteId: string) => deleteSite(siteId),
    onSuccess: (data) => {
      toast.success(t("portal.siteDeleted"), { description: data.message });
      setDeleteFor(null);
      queryClient.invalidateQueries({ queryKey: ["/api/sites"] });
      queryClient.invalidateQueries({ queryKey: ["/api/routers"] });
      queryClient.invalidateQueries({ queryKey: ["/api/activity"] });
    },
    onError: (err: Error) => toast.error(err.message),
  });

  const deleting = useMemo(
    () => (deleteFor ? (sites.find((s) => s.id === deleteFor.id)?.routerCount ?? 0) : 0),
    [deleteFor, sites],
  );

  return (
    <div className="space-y-6">
      {/* N°182/N°184 — pédagogie de la chaîne ROUTEUR → SITE → COMPTE : la
          base se règle désormais CI-DESSOUS (formulaire du compte,
          propriétaire) — la note se lit dans l'ordre de la page. */}
      <div className="flex items-start gap-3 rounded-xl border bg-muted/40 p-4 text-sm">
        <Palette className="mt-0.5 size-4 shrink-0 text-primary" aria-hidden="true" />
        <p className="leading-relaxed text-muted-foreground">{t("portal.chainNote")}</p>
      </div>

      {/* — SECTION PORTAIL DU COMPTE (base de la chaîne, N°184) —
          réservée au propriétaire (PUT /api/settings, rang 3) : masquée au
          gérant, comme l'était l'onglet Expérience (miroir canView). Le
          formulaire porte sa propre barre d'enregistrement (N°140) et ses
          gardes (N°142) ; la sauvegarde invalide les réglages partagés —
          les badges de régime ci-dessous restent exacts. */}
      {withAccount &&
        (settingsQuery.isLoading || !settingsQuery.data ? (
          <div className="flex h-24 items-center justify-center text-muted-foreground" role="status" aria-live="polite">
            <Loader2 className="size-5 animate-spin" />
          </div>
        ) : (
          <AccountPortalForm settings={settingsQuery.data} onDirtyChange={onDirtyChange} />
        ))}

      {/* — SECTION SITES — */}
      <section className="space-y-3" aria-label={t("portal.sites")}>
        <div className="flex flex-wrap items-center justify-between gap-2">
          <div>
            <h2 className="text-lg font-semibold tracking-tight">{t("portal.sites")}</h2>
            <p className="text-sm text-muted-foreground">{t("portal.sitesHint")}</p>
          </div>
          <Button type="button" size="sm" className="gap-1.5" onClick={() => setSiteDialogFor(null)}>
            <Plus className="size-3.5" />
            {t("portal.newSite")}
          </Button>
        </div>
        {sitesQuery.isLoading ? (
          <div className="flex h-24 items-center justify-center text-muted-foreground">
            <Loader2 className="size-5 animate-spin" />
          </div>
        ) : sites.length === 0 ? (
          <p className="rounded-xl border border-dashed p-4 text-sm text-muted-foreground">
            {t("portal.noSites")}
          </p>
        ) : (
          <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
            {sites.map((site) => (
              <Card key={site.id}>
                <CardContent className="space-y-4 p-4">
                  <div className="flex items-start justify-between gap-2">
                    <div className="min-w-0">
                      <div className="flex items-center gap-2">
                        <Building2 className="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
                        <span className="truncate font-medium">{site.name}</span>
                      </div>
                      {site.location ? (
                        <p className="mt-1 flex items-center gap-1 text-xs text-muted-foreground">
                          <MapPin className="size-3" aria-hidden="true" />
                          {site.location}
                        </p>
                      ) : null}
                      {site.description ? (
                        <p className="mt-1 line-clamp-2 text-xs text-muted-foreground">{site.description}</p>
                      ) : null}
                    </div>
                    {site.hasOverride ? (
                      <Badge className="gap-1 bg-emerald-100 text-emerald-700 hover:bg-emerald-100">
                        <Palette className="size-3" />
                        {t("portal.sitePortalCustom")}
                      </Badge>
                    ) : (
                      <Badge variant="outline">{t("portal.sitePortalAccount")}</Badge>
                    )}
                  </div>
                  <p className="text-xs text-muted-foreground">
                    {tf("portal.siteRouters", { count: site.routerCount })}
                  </p>
                  <div className="flex flex-wrap gap-2">
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      className="gap-1.5"
                      onClick={() => setSiteDialogFor(site)}
                    >
                      <Pencil className="size-3.5" />
                      {t("portal.siteEdit")}
                    </Button>
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      className="gap-1.5 text-destructive hover:text-destructive"
                      onClick={() => setDeleteFor(site)}
                      disabled={deleteMutation.isPending}
                    >
                      <Trash2 className="size-3.5" />
                      {t("portal.siteDelete")}
                    </Button>
                  </div>
                </CardContent>
              </Card>
            ))}
          </div>
        )}
      </section>

      {/* — SECTION ROUTEURS — */}
      <section className="space-y-3" aria-label={t("portal.routerSection")}>
        <h2 className="text-lg font-semibold tracking-tight">{t("portal.routerSection")}</h2>
        {routers.length === 0 ? (
          <EmptyState
            icon={Monitor}
            title={t("portal.empty")}
            description={t("portal.emptyHint")}
          />
        ) : (
          <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
            {routers.map((router) => {
              const regime = portalRegime(router, sites);
              const siteName = sites.find((s) => s.id === router.siteId)?.name;
              return (
                <Card key={router.id}>
                  <CardContent className="space-y-4 p-4">
                    <div className="flex items-start justify-between gap-2">
                      <div className="min-w-0">
                        <div className="flex items-center gap-2">
                          <RouterIcon className="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
                          <span className="truncate font-medium">{router.name}</span>
                        </div>
                        <p className="mt-1 text-xs text-muted-foreground">
                          {router.mode} · {router.status}
                        </p>
                        <div className="mt-1.5">
                          <SigBadge status={sigStatusOf(router)} t={t} />
                        </div>
                      </div>
                      {regime === "custom" ? (
                        <Badge className="gap-1 bg-amber-100 text-amber-700 hover:bg-amber-100">
                          {t("portal.regimeCustom")}
                        </Badge>
                      ) : regime === "site" ? (
                        <Badge className="gap-1 bg-teal-100 text-teal-700 hover:bg-teal-100">
                          {t("portal.regimeSite")} · {siteName}
                        </Badge>
                      ) : (
                        <Badge variant="outline">{t("portal.regimeAccount")}</Badge>
                      )}
                    </div>

                    <div className="space-y-2">
                      <Label className="text-xs" htmlFor={`site-${router.id}`}>
                        {t("portal.routerSite")}
                      </Label>
                      <Select
                        value={router.siteId || "none"}
                        onValueChange={(v) =>
                          assignMutation.mutate({ routerId: router.id, siteId: v === "none" ? "" : v })
                        }
                        disabled={assignMutation.isPending}
                      >
                        <SelectTrigger id={`site-${router.id}`} className="w-full">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="none">{t("portal.routerNoSite")}</SelectItem>
                          {sites.map((s) => (
                            <SelectItem key={s.id} value={s.id}>
                              {s.name}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </div>

                    <div className="flex flex-wrap gap-2">
                      <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        className="gap-1.5"
                        onClick={() => setCustomizeFor(router)}
                      >
                        <Palette className="size-3.5" />
                        {t("portal.customize")}
                      </Button>
                      <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        className="gap-1.5"
                        onClick={() => setPreviewFor(router)}
                      >
                        <Eye className="size-3.5" />
                        {t("portal.preview")}
                      </Button>
                      <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        className="gap-1.5"
                        onClick={() => setRedeployFor(router)}
                        disabled={redeployMutation.isPending}
                      >
                        <RefreshCw className="size-3.5" />
                        {t("portal.redeploy")}
                      </Button>
                    </div>
                  </CardContent>
                </Card>
              );
            })}
          </div>
        )}
      </section>

      {/* Journal des déploiements */}
      <div className="space-y-3">
        <h2 className="text-lg font-semibold tracking-tight">{t("portal.journal")}</h2>
        {portalActivity.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t("portal.journalEmpty")}</p>
        ) : (
          <ScrollArea className="max-h-96 rounded-lg border">
            <ul className="divide-y">
              {portalActivity.map((a) => (
                <li key={a.id} className="flex items-start gap-3 p-3 text-sm">
                  <div className="mt-0.5">
                    {/Re-déploiement/i.test(a.message) ? (
                      <RefreshCw className="size-4 text-amber-500" />
                    ) : (
                      <CheckCircle2 className="size-4 text-emerald-500" />
                    )}
                  </div>
                  <div className="min-w-0 flex-1">
                    <p className="text-sm">{a.message}</p>
                    <p className="mt-0.5 text-xs text-muted-foreground">
                      {a.at}
                      {a.actorName ? ` · ${a.actorName}` : ""}
                    </p>
                  </div>
                </li>
              ))}
            </ul>
          </ScrollArea>
        )}
      </div>

      {/* Dialog site (création/édition) */}
      {siteDialogFor !== undefined ? (
        <SiteDialog site={siteDialogFor} onClose={() => setSiteDialogFor(undefined)} t={t} />
      ) : null}

      {/* Dialog personnalisation routeur */}
      {customizeFor ? (
        <RouterPortalDialog router={customizeFor} onClose={() => setCustomizeFor(null)} t={t} />
      ) : null}

      {/* AlertDialog suppression site */}
      <AlertDialog open={deleteFor !== null} onOpenChange={(open) => !open && setDeleteFor(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {tf("portal.siteDeleteTitle", { name: deleteFor?.name ?? "" })}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {tf("portal.siteDeleteConfirm", { count: deleting })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleteMutation.isPending}>
              {t("portal.cancel")}
            </AlertDialogCancel>
            <AlertDialogAction
              disabled={deleteMutation.isPending}
              onClick={() => deleteFor && deleteMutation.mutate(deleteFor.id)}
              className="gap-1.5"
            >
              {deleteMutation.isPending ? (
                <Loader2 className="size-4 animate-spin" />
              ) : (
                <Trash2 className="size-4" />
              )}
              {t("portal.siteDelete")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* Dialog aperçu */}
      <PreviewDialog router={previewFor} onClose={() => setPreviewFor(null)} t={t} tf={tf} />

      {/* AlertDialog re-déploiement */}
      <AlertDialog open={redeployFor !== null} onOpenChange={(open) => !open && setRedeployFor(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t("portal.redeployConfirmTitle")}</AlertDialogTitle>
            <AlertDialogDescription>
              {tf("portal.redeployConfirm", { name: redeployFor?.name ?? "" })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={redeployMutation.isPending}>
              {t("portal.cancel")}
            </AlertDialogCancel>
            <AlertDialogAction
              disabled={redeployMutation.isPending}
              onClick={() => redeployFor && redeployMutation.mutate(redeployFor.id)}
              className="gap-1.5"
            >
              {redeployMutation.isPending ? (
                <Loader2 className="size-4 animate-spin" />
              ) : (
                <RefreshCw className="size-4" />
              )}
              {t("portal.redeploy")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Aperçu (N°35-d, inchangé) et badge de sig — conservés du portail d'origine.
// ---------------------------------------------------------------------------

/** SigBadge — badge de statut de la signature du portail. */
function SigBadge({
  status,
  t,
}: {
  status: "deployed" | "pending" | "never";
  t: (key: string, fallback?: string) => string;
}) {
  switch (status) {
    case "deployed":
      return (
        <Badge className="gap-1 bg-emerald-100 text-emerald-700 hover:bg-emerald-100">
          <CheckCircle2 className="size-3" />
          {t("portal.sigDeployed")}
        </Badge>
      );
    case "pending":
      return (
        <Badge className="gap-1 bg-amber-100 text-amber-700 hover:bg-amber-100">
          <Clock className="size-3" />
          {t("portal.sigPending")}
        </Badge>
      );
    case "never":
      return (
        <Badge variant="outline" className="gap-1">
          <AlertTriangle className="size-3" />
          {t("portal.sigNever")}
        </Badge>
      );
  }
}

/** sigStatus — statut dérivé de la signature du portail (conservé N°35-d). */
function sigStatusOf(router: RouterDevice): "deployed" | "pending" | "never" {
  const sig = (router as RouterDevice & { hotspotFilesSig?: string }).hotspotFilesSig;
  if (!sig) return "never";
  return "deployed";
}

/** PreviewDialog — dialog avec iframe srcDoc affichant le HTML personnalisé
 * du portail pour le routeur sélectionné (N°35-d — reflète la chaîne N°182
 * puisque l'endpoint passe par la même résolution côté backend). */
function PreviewDialog({
  router,
  onClose,
  t,
  tf,
}: {
  router: RouterDevice | null;
  onClose: () => void;
  t: (key: string, fallback?: string) => string;
  tf: (key: string, vars: Record<string, string | number>) => string;
}) {
  const [html, setHtml] = useState<string>("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string>("");

  // Fetch le HTML quand le dialog s'ouvre.
  useQuery({
    queryKey: ["portal-preview", router?.id],
    queryFn: async () => {
      if (!router) return "";
      setLoading(true);
      setError("");
      try {
        const htmlContent = await fetchRouterPortalPreview(router.id);
        setHtml(htmlContent);
        return htmlContent;
      } catch (err) {
        const msg = err instanceof Error ? err.message : String(err);
        setError(msg);
        setHtml("");
        return "";
      } finally {
        setLoading(false);
      }
    },
    enabled: router !== null,
  });

  return (
    <Dialog open={router !== null} onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-h-[90vh] max-w-4xl overflow-hidden">
        <DialogHeader>
          <DialogTitle>{tf("portal.previewTitle", { name: router?.name ?? "" })}</DialogTitle>
          <DialogDescription>{t("portal.previewHint")}</DialogDescription>
        </DialogHeader>
        <div className="h-[600px] w-full overflow-hidden rounded-lg border">
          {loading ? (
            <div className="flex h-full items-center justify-center text-muted-foreground">
              <Loader2 className="size-6 animate-spin" />
            </div>
          ) : error ? (
            <div className="flex h-full items-center justify-center p-4 text-center text-sm text-destructive">
              {error}
            </div>
          ) : (
            <iframe
              srcDoc={html}
              title="Portal preview"
              sandbox="allow-scripts allow-same-origin"
              className="h-full w-full border-0"
            />
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}
