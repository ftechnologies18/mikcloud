"use client";

// N°186 — contenu « Portail » de la section Hotspot (onglet du hub
// components/hotspot/views/hotspot-view.tsx — /app/settings/hotspot/portail).
//
// Le portail captif se personnalise à TROIS niveaux, chaque routeur servant
// le premier portail défini en remontant sa chaîne ROUTEUR → SITE → COMPTE :
//
//   - COMPTE : la base de la chaîne — « Portail du compte » (rang 3, PUT
//     /api/settings) ;
//   - SITES : regroupements de routeurs par établissement (bâtiment,
//     boutique, campus) portant une identité de portail ;
//   - ROUTEURS : la surcharge INDIVIDUELLE, le niveau le plus fin.
//
// N°186 — ÉDITEUR UNIFIÉ à sélecteur de contexte : UN endroit (en tête
// d'onglet) édite la chaîne ENTIÈRE. Le sélecteur « Vous personnalisez »
// bascule entre le compte, un site ou un routeur ; les MÊMES briques riches
// (uploads R2, éditeurs de listes, aperçus) se rendent aux trois niveaux ;
// chaque groupe affiche la valeur HÉRITÉE résolue et sa provenance. Les
// anciens dialogs de surcharge (textareas « une entrée par ligne ») ont
// disparu : le dialog site ne garde que les champs descriptifs (nom,
// localisation, description), la personnalisation vit dans l'éditeur.
//
// Tout changement (formulaire du compte, assignation, surcharge) rejoint la
// signature de déploiement côté backend → re-déploiement automatique au
// check-in (≤ 45 s). L'aperçu par routeur (N°35-d) reflète la chaîne
// résolue.
//
// Sémantique des surcharges : VIDE = HÉRITE (un champ vide ne touche rien —
// la surcharge ne peut pas masquer un élément que le compte affiche ; pour
// ce cas, vider le champ côté compte et le surcharger ailleurs).

import { useCallback, useMemo, useState } from "react";
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
import {
  PortalUnifiedEditor,
  contextFromKey,
  contextKeyOf,
  type PortalContext,
  type UnifiedTarget,
} from "@/components/hotspot/parts/portal-editor";
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
  updateSite,
} from "@/lib/hotspot/api";
import { useI18n } from "@/lib/hotspot/i18n";
import type { RouterDevice, SiteResponse } from "@/lib/hotspot/types";

// ---------------------------------------------------------------------------
// Dialog site (création / édition — champs descriptifs SEULES)
// ---------------------------------------------------------------------------

/** SiteDialog — création et édition d'un site (nom, localisation,
 * description). N°186 : l'identité de portail ne vit PLUS ici — elle se
 * personnalise dans l'éditeur unifié (le bouton « Portail du site » de la
 * carte y bascule) ; après CRÉATION, l'éditeur s'ouvre directement sur le
 * nouveau site (la personnalisation suit la création, comme dans l'ancien
 * flow). site null = création ; sinon édition. */
function SiteDialog({
  site,
  onClose,
  onCreated,
  t,
}: {
  site: SiteResponse | null;
  onClose: () => void;
  /** Après création : bascule l'éditeur unifié sur le portail du nouveau
   * site (appelé une fois les sites rafraîchis — la cible doit exister
   * dans la liste pour se résoudre). */
  onCreated?: (siteId: string) => void;
  t: (key: string, fallback?: string) => string;
}) {
  const queryClient = useQueryClient();
  const [name, setName] = useState(site?.name ?? "");
  const [location, setLocation] = useState(site?.location ?? "");
  const [description, setDescription] = useState(site?.description ?? "");

  const mutation = useMutation({
    mutationFn: async () => {
      // Champs descriptifs SEULS : portalOverride nil = inchangé côté Go
      // (l'éditeur unifié est l'unique point d'écriture de la surcharge).
      const body = { name: name.trim(), location: location.trim(), description: description.trim() };
      if (site) return updateSite(site.id, body);
      return createSite(body);
    },
    onSuccess: async (data) => {
      toast.success(site ? t("portal.siteUpdated") : t("portal.siteCreated"), {
        description: site ? t("portal.siteSaved") : t("portal.siteCreatedDesc"),
      });
      // Attend le refetch des sites AVANT de basculer l'éditeur.
      await queryClient.invalidateQueries({ queryKey: ["/api/sites"] });
      void queryClient.invalidateQueries({ queryKey: ["/api/activity"] });
      if (!site) onCreated?.(data.id);
      onClose();
    },
    onError: (err: Error) => toast.error(err.message),
  });

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-lg">
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
          <p className="rounded-lg border bg-muted/40 p-3 text-xs leading-relaxed text-muted-foreground">
            {t("portal.siteDialogBrandingHint")}
          </p>
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

/** Contenu de l'onglet « Portail » du hub Hotspot — N°186 : l'éditeur
 * unifié à sélecteur de contexte en tête (compte / site / routeur — LA
 * chaîne entière au même endroit), puis les sections Sites et Routeurs
 * (vue d'ensemble + navigation vers l'éditeur), journal des déploiements
 * en pied. */
export function PortalContent({
  withAccount,
  onDirtyChange,
}: {
  /** N°184/N°186 — l'option « Compte » du sélecteur (PUT /api/settings,
   * rang 3) : masquée au gérant — miroir canView côté hub. */
  withAccount: boolean;
  /** Remonte le compteur de saisie du formulaire ACTIF (compte OU
   * surcharge) au hub — garde de sortie d'onglet (N°142). Stable
   * (useCallback côté hub). */
  onDirtyChange?: (count: number) => void;
}) {
  const { t, tf } = useI18n();
  const queryClient = useQueryClient();

  // Réglages du compte : base de la chaîne — alimentent le formulaire du
  // compte ET les valeurs héritées des surcharges (GET ouvert au gérant :
  // le rang 2 voit ce qu'il surcharge, fin du réglage à l'aveugle).
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

  // — N°186 : contexte de l'éditeur unifié —
  // Défaut : le compte pour le propriétaire (le sommet de sa chaîne) ; le
  // gérant choisit (site ou routeur) — les valeurs héritées restent
  // visibles dans les résumés de groupes.
  const [context, setContext] = useState<PortalContext | null>(() =>
    withAccount ? { kind: "compte" } : null,
  );
  // Compteur de saisie du formulaire ACTIF (compte OU surcharge — un seul
  // monté à la fois) : garde de changement de contexte ET remontée au hub.
  const [formDirty, setFormDirty] = useState(0);
  const [pendingContext, setPendingContext] = useState<PortalContext | null>(null);

  const handleEditorDirty = useCallback(
    (count: number) => {
      setFormDirty(count);
      onDirtyChange?.(count);
    },
    [onDirtyChange],
  );

  // Cible RÉSOLUE : l'entité doit exister dans les listes (une cible
  // disparue — site supprimé, routeur retiré — retombe sur null). TOUT EST
  // DÉRIVÉ, sans effet : le sélecteur reçoit le contexte seulement s'il
  // résout (sinon placeholder), l'éditeur rend son état vide, et les
  // formulaires signalent 0 à leur DÉMONTAGE (compteur jamais fantôme).
  const target = useMemo<UnifiedTarget | null>(() => {
    if (!context) return null;
    if (context.kind === "compte") return withAccount ? { kind: "compte" } : null;
    if (context.kind === "site") {
      const site = sites.find((s) => s.id === context.siteId);
      return site ? { kind: "site", site } : null;
    }
    const router = routers.find((r) => r.id === context.routerId);
    if (!router) return null;
    return { kind: "router", router, site: sites.find((s) => s.id === router.siteId) ?? null };
  }, [context, withAccount, sites, routers]);

  const scrollToEditor = useCallback(() => {
    // requestAnimationFrame : laisse le formulaire de la nouvelle cible se
    // monter avant d'ancrer le défilement.
    window.requestAnimationFrame(() => {
      document.getElementById("portal-editor")?.scrollIntoView({
        behavior: "smooth",
        block: "start",
      });
    });
  }, []);

  const applyContext = useCallback(
    (next: PortalContext | null, scroll = true) => {
      setContext(next);
      if (scroll) scrollToEditor();
    },
    [scrollToEditor],
  );

  // Changement de contexte demandé (sélecteur OU bouton d'une carte) —
  // garde N°142 : saisie en cours → confirmation au lieu de tout jeter.
  // Fonction simple (recréée au rendu, hors chemins mémoïsés) : lit l'état
  // le plus frais au moment du clic.
  const requestContext = (next: PortalContext | null) => {
    if (contextKeyOf(next) === contextKeyOf(context)) {
      scrollToEditor();
      return;
    }
    if (formDirty > 0) {
      setPendingContext(next);
      return;
    }
    applyContext(next);
  };

  const confirmContext = () => {
    const next = pendingContext;
    setPendingContext(null);
    if (next) applyContext(next);
  };

  const handleSelectContext = (key: string) => {
    requestContext(contextFromKey(key));
  };

  // États locaux : dialogs.
  const [siteDialogFor, setSiteDialogFor] = useState<SiteResponse | null | undefined>(undefined); // undefined fermé, null création
  const [deleteFor, setDeleteFor] = useState<SiteResponse | null>(null);
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
      {/* N°182/N°186 — pédagogie de la chaîne ROUTEUR → SITE → COMPTE : la
          note se lit dans l'ordre de la page (sélecteur → base → surcharges). */}
      <div className="flex items-start gap-3 rounded-xl border bg-muted/40 p-4 text-sm">
        <Palette className="mt-0.5 size-4 shrink-0 text-primary" aria-hidden="true" />
        <p className="leading-relaxed text-muted-foreground">{t("portal.chainNote")}</p>
      </div>

      {/* — N°186 : ÉDITEUR UNIFIÉ à sélecteur de contexte — LA chaîne
          entière s'édite ici : compte (rang 3), site ou routeur. Les
          formulaires portent leur propre barre d'enregistrement (N°140) et
          leurs gardes (N°142) ; les sauvegardes invalident les listes —
          les badges de régime ci-dessous restent exacts. */}
      <PortalUnifiedEditor
        context={target ? context : null}
        target={target}
        withAccount={withAccount}
        settings={settingsQuery.data}
        settingsLoading={settingsQuery.isLoading}
        sites={sites}
        routers={routers}
        onSelectContext={handleSelectContext}
        onDirtyChange={handleEditorDirty}
      />

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
                    {/* N°186 — la personnalisation vit dans l'éditeur
                        unifié : ce bouton Y bascule (contexte site). */}
                    <Button
                      type="button"
                      size="sm"
                      className="gap-1.5"
                      onClick={() => requestContext({ kind: "site", siteId: site.id })}
                    >
                      <Palette className="size-3.5" />
                      {t("portal.siteCustomize")}
                    </Button>
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
                      {/* N°186 — la personnalisation vit dans l'éditeur
                          unifié (ancien dialog textareas supprimé). */}
                      <Button
                        type="button"
                        size="sm"
                        className="gap-1.5"
                        onClick={() => requestContext({ kind: "router", routerId: router.id })}
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

      {/* Dialog site (création/édition descriptive) */}
      {siteDialogFor !== undefined ? (
        <SiteDialog
          site={siteDialogFor}
          onClose={() => setSiteDialogFor(undefined)}
          onCreated={(siteId) => applyContext({ kind: "site", siteId })}
          t={t}
        />
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

      {/* N°186 — confirmation de changement de contexte (miroir de la garde
          de sortie d'onglet N°142 : la saisie du formulaire ACTIF — compte
          ou surcharge — serait perdue). */}
      <AlertDialog
        open={pendingContext !== null}
        onOpenChange={(open) => {
          if (!open) setPendingContext(null);
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t("portal.ctxGuardTitle")}</AlertDialogTitle>
            <AlertDialogDescription>
              {tf("portal.ctxGuardDesc", { n: formDirty })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t("settings.exp.tabGuardStay")}</AlertDialogCancel>
            <AlertDialogAction onClick={confirmContext}>
              {t("settings.exp.tabGuardLeave")}
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
