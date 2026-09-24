"use client";

// N°186 — ÉDITEUR UNIFIÉ à sélecteur de contexte (Compte / Site / Routeur).
//
// Suite naturelle de la fusion N°184 (la chaîne ROUTEUR → SITE → COMPTE
// réunie dans un onglet) : il restait DEUX éditeurs pour les MÊMES champs —
// le formulaire riche côté compte (uploads R2, éditeurs de listes, aperçus)
// contre des textareas « une entrée par ligne » dans les dialogs de
// surcharge, et le gérant configurait à l'aveugle (sans voir le portail
// hérité). L'éditeur unifié rebâtit la surface d'édition :
//
//   • UN SÉLECTEUR DE CONTEXTE en tête d'onglet — « Vous personnalisez » :
//     le compte (propriétaire, PUT /api/settings), un site ou un routeur
//     (rang 2, PUT /api/sites/{id} et PUT /api/routers/{id}/portal) ;
//   • LES MÊMES BRIQUES RICHES aux trois niveaux — BannerFields,
//     SlidesFields, ServicesFields, TickerFields, WhatsappFields,
//     HospitalityFields exportées de hotspot-cards (forme structurelle
//     PortalBrandingFields) + une brique « Identité » (nom affiché, logo,
//     lien Wave) propre aux surcharges ;
//   • L'HÉRITAGE RENDU VISIBLE — chaque groupe affiche « Hérité » avec la
//     VALEUR RÉSOLUE de la chaîne (miroir exact de resolvePortalBranding
//     backend : compte → site → routeur) et sa provenance ; « Personnaliser »
//     ouvre le groupe PRÉ-REMPLI avec la valeur héritée, « Réinitialiser
//     (hériter) » le rend au niveau supérieur ;
//   • PARITÉ DE CHAMPS — promos, réseaux sociaux et icônes de services
//     désormais éditables aussi en surcharge (l'ancien dialog textareas ne
//     les offrait pas) ;
//   • MÊME ARMATURE N°140/N°142 — barre d'action sticky, compteur de groupes
//     modifiés, scrollspy, garde beforeunload, Cmd/Ctrl+Entrée, remontée du
//     compteur au hub (garde de sortie d'onglet).
//
// Contrats backend inchangés : sémantique « vide = hérite », remplacement
// ENTIER de la surcharge à chaque écriture (l'éditeur envoie l'état complet
// souhaité — un groupe réinitialisé disparaît du corps), validation partagée
// portal_branding.go. Zéro changement serveur.

import { Fragment, useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  ArrowDownLeft,
  Building2,
  Check,
  CircleAlert,
  Fingerprint,
  Image as ImageIcon,
  Images,
  Landmark,
  ListChecks,
  Loader2,
  Megaphone,
  MessageCircle,
  Palette,
  RotateCcw,
  Router as RouterIcon,
  Store,
  Undo2,
  X,
  ImagePlus,
  type LucideIcon,
} from "lucide-react";
import { toast } from "sonner";

import { updateRouterPortal, updateSite } from "@/lib/hotspot/api";
import { useI18n } from "@/lib/hotspot/i18n";
import {
  parsePortalOverride,
  type AppSettings,
  type RouterDevice,
  type SiteResponse,
} from "@/lib/hotspot/types";
import {
  accountBrandingOf,
  applyOverrideForm,
  emptyBrandingForm,
  emptyGroupValuesOf,
  formFromOverride,
  groupEquals,
  groupIsSet,
  groupValuesOf,
  overridePayloadOf,
  PORTAL_OVERRIDE_GROUPS,
  type OverrideGroupKey,
  type PortalBrandingFields,
  type PortalOverrideForm,
} from "@/lib/hotspot/portal-branding";
import { EmptyState } from "@/components/hotspot/empty-state";
import {
  AccountPortalForm,
  BannerFields,
  HospitalityFields,
  ServicesFields,
  SlidesFields,
  SubSectionHeader,
  TickerFields,
  WhatsappFields,
} from "@/components/hotspot/parts/hotspot-cards";
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
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectLabel,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Separator } from "@/components/ui/separator";
import { cn } from "@/lib/utils";

/* ─── Contexte d'édition (sélecteur) ─── */

/** PortalContext — la CIBLE du sélecteur « Vous personnalisez ». Portée par
 * portal-view (les cartes Sites/Routeurs y basculent aussi) ; la résolution
 * en cible effective (entité trouvée dans les listes) vit côté vue. */
export type PortalContext =
  | { kind: "compte" }
  | { kind: "site"; siteId: string }
  | { kind: "router"; routerId: string };

/** UnifiedTarget — contexte RÉSOLU : l'entité est présente dans les listes
 * (une cible disparue — site supprimé — retombe sur null côté vue). */
export type UnifiedTarget =
  | { kind: "compte" }
  | { kind: "site"; site: SiteResponse }
  | { kind: "router"; router: RouterDevice; site: SiteResponse | null };

/** contextKeyOf — clé stable du contexte (valeur du sélecteur, remontée du
 * compteur de saisie : "compte" | "site:<id>" | "router:<id>"). */
export function contextKeyOf(c: PortalContext | null): string {
  if (!c) return "";
  if (c.kind === "compte") return "compte";
  return c.kind === "site" ? `site:${c.siteId}` : `router:${c.routerId}`;
}

/** contextFromKey — décode la valeur du sélecteur (garde d'incohérence : une
 * clé inconnue retombe sur null). */
export function contextFromKey(v: string): PortalContext | null {
  if (v === "compte") return { kind: "compte" };
  if (v.startsWith("site:")) return { kind: "site", siteId: v.slice(5) };
  if (v.startsWith("router:")) return { kind: "router", routerId: v.slice(7) };
  return null;
}

/* ─── Métadonnées des groupes (ancres, pictogrammes, libellés) ─── */

/** GROUP_META — repères visuels COPIÉS du formulaire du compte (N°140) :
 * mêmes pictogrammes, mêmes identifiels d'ancres (les deux formulaires ne
 * sont jamais montés ensemble — le contexte est exclusif). */
const GROUP_META: Record<OverrideGroupKey, { icon: LucideIcon; labelKey: string; anchor: string }> = {
  identity: { icon: Fingerprint, labelKey: "portal.identityCard", anchor: "hot-exp-identity" },
  banner: { icon: ImageIcon, labelKey: "settings.bannerCard", anchor: "hot-exp-banner" },
  slides: { icon: Images, labelKey: "settings.slides.card", anchor: "hot-exp-slides" },
  services: { icon: ListChecks, labelKey: "settings.svc.card", anchor: "hot-exp-services" },
  ticker: { icon: Megaphone, labelKey: "settings.ticker.card", anchor: "hot-exp-ticker" },
  whatsapp: { icon: MessageCircle, labelKey: "settings.wa.card", anchor: "hot-exp-whatsapp" },
  display: { icon: Store, labelKey: "settings.hosp.card", anchor: "hot-exp-mode" },
};

/** GROUP_NAV — libellés des puces d'ancrage (nav rapide). */
const GROUP_NAV: Record<OverrideGroupKey, string> = {
  identity: "portal.navIdentity",
  banner: "settings.exp.navBanner",
  slides: "settings.exp.navSlides",
  services: "settings.exp.navServices",
  ticker: "settings.exp.navTicker",
  whatsapp: "settings.exp.navWhatsapp",
  display: "settings.exp.navMode",
};

/* ─── Éditeur unifié (sélecteur + formulaires) ─── */

export function PortalUnifiedEditor({
  context,
  target,
  withAccount,
  settings,
  settingsLoading,
  sites,
  routers,
  onSelectContext,
  onDirtyChange,
}: {
  /** Contexte SÉLECTIONNÉ (valeur du sélecteur — peut être momentanément
   * irrésolu pendant les refetchs). */
  context: PortalContext | null;
  /** Contexte RÉSOLU (entité présente) — pilote le formulaire rendu. */
  target: UnifiedTarget | null;
  /** Rang 3 : l'option « Compte » du sélecteur (PUT /api/settings). */
  withAccount: boolean;
  /** Réglages du compte — base de la chaîne (GET ouvert au gérant : les
   * valeurs héritées restent visibles au rang 2, fin du réglage à
   * l'aveugle). */
  settings: AppSettings | undefined;
  settingsLoading: boolean;
  sites: SiteResponse[];
  routers: RouterDevice[];
  /** Changement de contexte demandé par le sélecteur — la GARDE (saisie en
   * cours) vit côté portal-view, qui voit le compteur des DEUX formulaires. */
  onSelectContext: (key: string) => void;
  /** Remonte le compteur de groupes modifiés (garde N°142). */
  onDirtyChange?: (count: number) => void;
}) {
  const { t, tf } = useI18n();
  const queryClient = useQueryClient();

  // Branding EFFECTIF du compte — base des héritages (défensif si les
  // réglages ne sont pas encore chargés : les résumés se remplissent à
  // l'arrivée, la saisie n'est jamais bloquée).
  const accountForm = useMemo(
    () => (settings ? accountBrandingOf(settings) : emptyBrandingForm()),
    [settings],
  );

  // Sauvegarde SITE — PUT /api/sites/{id} avec la SEULE surcharge (champs
  // descriptifs en pointeurs nil = inchangés côté Go) ; invalidation des
  // listes pour rafraîchir badges et régimes.
  const saveSite = useCallback(
    async (siteId: string, payload: Record<string, unknown>) => {
      await updateSite(siteId, { portalOverride: payload });
      await queryClient.invalidateQueries({ queryKey: ["/api/sites"] });
      void queryClient.invalidateQueries({ queryKey: ["/api/activity"] });
    },
    [queryClient],
  );

  // Sauvegarde ROUTEUR — PUT /api/routers/{id}/portal (corps complet).
  const saveRouter = useCallback(
    async (routerId: string, payload: Record<string, unknown>) => {
      await updateRouterPortal(routerId, payload);
      await queryClient.invalidateQueries({ queryKey: ["/api/routers"] });
      void queryClient.invalidateQueries({ queryKey: ["/api/activity"] });
    },
    [queryClient],
  );

  const hasOptions = withAccount || sites.length > 0 || routers.length > 0;

  return (
    <section className="space-y-4 sm:space-y-6" aria-label={t("portal.editorSection")}>
      {/* — SÉLECTEUR DE CONTEXTE — « Vous personnalisez » : UN endroit pour
          éditer la chaîne ENTIÈRE. Les cartes Sites/Routeurs ci-dessous y
          basculent aussi (bouton « Personnaliser »). */}
      {hasOptions ? (
        <div id="portal-editor" className="scroll-mt-24 space-y-2">
          <div className="flex flex-wrap items-center gap-2">
            <Label htmlFor="portal-context" className="text-sm font-medium">
              {t("portal.ctxLabel")}
            </Label>
            <Select value={contextKeyOf(context)} onValueChange={onSelectContext}>
              <SelectTrigger id="portal-context" className="h-10 w-full sm:w-[22rem]">
                <SelectValue placeholder={t("portal.ctxPlaceholder")} />
              </SelectTrigger>
              <SelectContent>
                {withAccount ? (
                  <>
                    <SelectLabel>{t("portal.ctxGroupAccount")}</SelectLabel>
                    <SelectItem value="compte">
                      <span className="inline-flex items-center gap-2">
                        <Landmark className="size-4 text-primary" aria-hidden="true" />
                        {tf("portal.ctxAccountItem", { name: settings?.tenant.name ?? "…" })}
                      </span>
                    </SelectItem>
                  </>
                ) : null}
                {sites.length > 0 ? (
                  <>
                    <SelectLabel>{t("portal.sites")}</SelectLabel>
                    {sites.map((s) => (
                      <SelectItem key={s.id} value={`site:${s.id}`}>
                        <span className="inline-flex items-center gap-2">
                          <Building2 className="size-4 text-primary" aria-hidden="true" />
                          {s.name}
                          {s.hasOverride ? (
                            <span className="text-xs text-muted-foreground">
                              · {t("portal.groupPersonalized")}
                            </span>
                          ) : null}
                        </span>
                      </SelectItem>
                    ))}
                  </>
                ) : null}
                {routers.length > 0 ? (
                  <>
                    <SelectLabel>{t("portal.routerSection")}</SelectLabel>
                    {routers.map((r) => (
                      <SelectItem key={r.id} value={`router:${r.id}`}>
                        <span className="inline-flex items-center gap-2">
                          <RouterIcon className="size-4 text-primary" aria-hidden="true" />
                          {r.name}
                          {r.portalOverride ? (
                            <span className="text-xs text-muted-foreground">
                              · {t("portal.groupPersonalized")}
                            </span>
                          ) : null}
                        </span>
                      </SelectItem>
                    ))}
                  </>
                ) : null}
              </SelectContent>
            </Select>
          </div>
          <p className="text-xs text-muted-foreground">
            {target === null
              ? t("portal.editorHintNone")
              : target.kind === "compte"
                ? t("portal.editorHintAccount")
                : target.kind === "site"
                  ? tf("portal.editorHintSite", { n: target.site.routerCount })
                  : tf("portal.editorHintRouter", {
                      source: target.site?.name ?? t("portal.ctxGroupAccount"),
                    })}
          </p>
        </div>
      ) : null}

      {/* — FORMULE du contexte — compte : le formulaire riche N°184 tel
          quel (barre d'action et gardes propres) ; site/routeur :
          l'éditeur de surcharge ; rien de sélectionné : guide de démarrage
          (gérant sans choix encore posé). */}
      {target === null ? (
        hasOptions ? (
          <div className="rounded-xl border border-dashed p-6">
            <EmptyState
              icon={Palette}
              title={t("portal.editorEmptyTitle")}
              description={t("portal.editorEmptyHint")}
            />
          </div>
        ) : null
      ) : target.kind === "compte" ? (
        settingsLoading || !settings ? (
          <div
            className="flex h-24 items-center justify-center"
            role="status"
            aria-live="polite"
          >
            <Loader2 className="size-5 animate-spin text-muted-foreground" />
          </div>
        ) : (
          <AccountPortalForm settings={settings} onDirtyChange={onDirtyChange} />
        )
      ) : target.kind === "site" ? (
        <OverrideEditor
          key={`site:${target.site.id}`}
          title={tf("portal.editorSiteTitle", { name: target.site.name })}
          desc={t("portal.editorOverrideDesc")}
          saved={parsePortalOverride(target.site.portalOverride)}
          inherited={accountForm}
          siteSavedForm={null}
          siteName={null}
          save={(payload) => saveSite(target.site.id, payload)}
          savedToastKey="portal.editorSiteSaved"
          onDirtyChange={onDirtyChange}
        />
      ) : (
        <OverrideEditor
          key={`router:${target.router.id}`}
          title={tf("portal.editorRouterTitle", { name: target.router.name })}
          desc={t("portal.editorOverrideDesc")}
          saved={parsePortalOverride(target.router.portalOverride)}
          inherited={applyOverrideForm(
            accountForm,
            parsePortalOverride(target.site?.portalOverride),
          )}
          siteSavedForm={
            target.site ? formFromOverride(parsePortalOverride(target.site.portalOverride)) : null
          }
          siteName={target.site?.name ?? null}
          save={(payload) => saveRouter(target.router.id, payload)}
          savedToastKey="portal.overrideSaved"
          onDirtyChange={onDirtyChange}
        />
      )}
    </section>
  );
}

/* ─── Éditeur de surcharge (site / routeur) ─── */

function OverrideEditor({
  title,
  desc,
  saved,
  inherited,
  siteSavedForm,
  siteName,
  save,
  savedToastKey,
  onDirtyChange,
}: {
  /** Titre de la carte (« Portail du site « X » »). */
  title: string;
  desc: string;
  /** Surcharge PERSISTÉE (JSON canonique décodé) — état de référence. */
  saved: ReturnType<typeof parsePortalOverride>;
  /** Branding RÉSOLU au-dessus de ce niveau (compte, ou compte + site pour
   * un routeur) : valeurs héritées affichées et pré-remplissages. */
  inherited: PortalOverrideForm;
  /** Surcharge persistée du SITE parent (niveau routeur seulement) — sert
   * à indiquer la PROVENANCE de chaque groupe hérité. */
  siteSavedForm: PortalOverrideForm | null;
  siteName: string | null;
  /** Écrit la surcharge (remplacement ENTIER — corps complet). */
  save: (payload: Record<string, unknown>) => Promise<unknown>;
  savedToastKey: string;
  onDirtyChange?: (count: number) => void;
}) {
  const { t, tf } = useI18n();

  const [form, setForm] = useState<PortalOverrideForm>(() => formFromOverride(saved));
  const [baseline, setBaseline] = useState<PortalOverrideForm>(() => formFromOverride(saved));
  // Groupes OUVERTS en édition : au départ, ceux que la surcharge persistée
  // porte déjà (« personnalisé ») ; les autres restent « hérité ».
  const [active, setActive] = useState<ReadonlySet<OverrideGroupKey>>(
    () => new Set(PORTAL_OVERRIDE_GROUPS.filter((g) => groupIsSet(formFromOverride(saved), g))),
  );

  const patch = useCallback((p: Partial<PortalOverrideForm>) => setForm((f) => ({ ...f, ...p })), []);
  // Variante fonctionnelle — téléversements async (slides, promos) : part
  // toujours de l'état le plus frais.
  const patchWith = useCallback(
    (fn: (f: PortalOverrideForm) => PortalOverrideForm) => setForm(fn),
    [],
  );
  // Adaptateur des briques partagées (vue branding seule).
  const patchBrandingWith = useCallback(
    (fn: (f: PortalBrandingFields) => PortalBrandingFields) =>
      patchWith((f) => ({ ...f, ...fn(f) })),
    [patchWith],
  );

  const dirtyGroups = useMemo(() => {
    const out = {} as Record<OverrideGroupKey, boolean>;
    for (const g of PORTAL_OVERRIDE_GROUPS) out[g] = !groupEquals(form, baseline, g);
    return out;
  }, [form, baseline]);
  const dirtyCount = PORTAL_OVERRIDE_GROUPS.filter((g) => dirtyGroups[g]).length;

  // N°142 — remontée du compteur (garde de sortie d'onglet côté hub). Le
  // DÉMONTAGE signale 0 (miroir du formulaire du compte) : un changement de
  // contexte ou une cible disparue ne laisse AUCUN compteur fantôme.
  useEffect(() => {
    onDirtyChange?.(dirtyCount);
    return () => onDirtyChange?.(0);
  }, [dirtyCount, onDirtyChange]);

  // N°142 — scrollspy compact (même mécanique que le formulaire du compte :
  // puce de la dernière section lue, règle du bas de page).
  const anchors = useMemo(
    () => PORTAL_OVERRIDE_GROUPS.map((g) => ({ id: GROUP_META[g].anchor, labelKey: GROUP_NAV[g] })),
    [],
  );
  const [activeAnchor, setActiveAnchor] = useState(anchors[0].id);
  const chipNavRef = useRef<HTMLElement | null>(null);
  useEffect(() => {
    let raf = 0;
    const measure = () => {
      cancelAnimationFrame(raf);
      raf = requestAnimationFrame(() => {
        const doc = document.documentElement;
        const maxScroll = doc.scrollHeight - window.innerHeight;
        if (maxScroll > 0 && window.scrollY >= maxScroll - 60) {
          setActiveAnchor(anchors[anchors.length - 1].id);
          return;
        }
        const line = 140;
        let current = anchors[0].id;
        for (const anchor of anchors) {
          const el = document.getElementById(anchor.id);
          if (el && el.getBoundingClientRect().top <= line) current = anchor.id;
        }
        setActiveAnchor(current);
      });
    };
    measure();
    window.addEventListener("scroll", measure, { passive: true });
    window.addEventListener("resize", measure);
    return () => {
      window.removeEventListener("scroll", measure);
      window.removeEventListener("resize", measure);
      cancelAnimationFrame(raf);
    };
  }, [anchors]);
  useEffect(() => {
    // N°142-bis — suivre la puce active ne touche qu'au défilement
    // HORIZONTAL de la rangée (la page ne bouge jamais).
    const nav = chipNavRef.current;
    const chip = nav?.querySelector<HTMLElement>(`[data-chip="${activeAnchor}"]`);
    if (!nav || !chip) return;
    const navRect = nav.getBoundingClientRect();
    const chipRect = chip.getBoundingClientRect();
    const target = chipRect.left - navRect.left - (nav.clientWidth - chip.offsetWidth) / 2;
    const left = Math.max(0, nav.scrollLeft + target);
    if (Math.abs(left - nav.scrollLeft) >= 1) nav.scrollTo({ left, behavior: "smooth" });
  }, [activeAnchor]);

  // Validations locales — miroir du formulaire du compte (et des bornes
  // partagées portal_branding.go).
  const bannerInvalid =
    form.bannerUrl.trim() !== "" &&
    !form.bannerUrl.trim().startsWith("https://") &&
    !form.bannerUrl.trim().startsWith("data:image/");
  const waDigits = form.waNumber.replace(/[^0-9]/g, "");
  const waInvalid = waDigits !== "" && (waDigits.length < 8 || waDigits.length > 15);
  const canSave = !bannerInvalid && !waInvalid;

  const firstError = bannerInvalid
    ? { section: "hot-exp-banner", input: "banner-url" }
    : waInvalid
      ? { section: "hot-exp-whatsapp", input: "wa-number" }
      : null;
  const goToFirstError = () => {
    if (!firstError) return;
    document.getElementById(firstError.section)?.scrollIntoView({ behavior: "smooth", block: "start" });
    window.setTimeout(() => {
      document.getElementById(firstError.input)?.focus({ preventScroll: true });
    }, 450);
  };

  const saveMutation = useMutation({
    mutationFn: (payload: Record<string, unknown>) => save(payload),
    onSuccess: (_data, payload) => {
      if (Object.keys(payload).length === 0) {
        // Retrait complet : retour à l'héritage total du niveau supérieur.
        setForm(emptyBrandingForm());
        setBaseline(emptyBrandingForm());
        setActive(new Set());
        toast.success(t("portal.overrideRemoved"));
      } else {
        // La saisie devient la nouvelle référence (points « modifié »
        // éteints immédiatement, sans attendre le refetch).
        setBaseline({ ...form });
        toast.success(t(savedToastKey));
      }
    },
    onError: (error: Error) => toast.error(error.message),
  });
  const { mutate: saveAll, isPending: saving } = saveMutation;

  // Garde-fou navigation (beforeunload tant que des groupes divergent).
  useEffect(() => {
    if (dirtyCount === 0) return;
    const handler = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", handler);
    return () => window.removeEventListener("beforeunload", handler);
  }, [dirtyCount]);

  // Cmd/Ctrl+Entrée enregistre.
  useEffect(() => {
    const handler = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key === "Enter") {
        if (dirtyCount > 0 && canSave && !saving) saveAll(overridePayloadOf(form));
      }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [dirtyCount, canSave, saving, saveAll, form]);

  // Réinitialiser (confirmé) : la saisie revient à l'état enregistré —
  // y compris les groupes ouverts (un groupe sauvé se rouvre, un groupe
  // ouvert pour rien se referme).
  const [confirmReset, setConfirmReset] = useState(false);
  const resetForm = () => {
    setForm({ ...baseline });
    setActive(new Set(PORTAL_OVERRIDE_GROUPS.filter((g) => groupIsSet(baseline, g))));
  };

  // Retrait complet (confirmé) : corps VIDE = la surcharge disparaît, le
  // niveau supérieur s'applique à nouveau partout.
  const [confirmRemove, setConfirmRemove] = useState(false);
  const hasOverride = PORTAL_OVERRIDE_GROUPS.some(
    (g) => groupIsSet(baseline, g) || groupIsSet(form, g),
  );

  /* Personnaliser un groupe : OUVERTURE pré-remplie avec la valeur HÉRITÉE
   * (l'utilisateur part de ce qui est servi aujourd'hui et ajuste) —
   * enregistrer fige alors ce réglage à CE niveau. */
  const personalize = (g: OverrideGroupKey) => {
    setActive((prev) => {
      const next = new Set(prev);
      next.add(g);
      return next;
    });
    setForm((f) => ({ ...f, ...groupValuesOf(inherited, g) }));
  };

  /* Rendre un groupe au niveau supérieur : champs vidés (= hérite), le
   * groupe disparaît du corps envoyé au prochain enregistrement. */
  const inheritGroup = (g: OverrideGroupKey) => {
    setActive((prev) => {
      const next = new Set(prev);
      next.delete(g);
      return next;
    });
    setForm((f) => ({ ...f, ...emptyGroupValuesOf(g) }));
  };

  const jumpTo = (id: string) =>
    document.getElementById(id)?.scrollIntoView({ behavior: "smooth", block: "start" });

  // Provenance d'un groupe hérité (niveau routeur : le site parent peut
  // porter le groupe — sinon c'est le compte).
  const sourceIsSite = (g: OverrideGroupKey) => !!siteSavedForm && groupIsSet(siteSavedForm, g);

  const renderBrick = (g: OverrideGroupKey): ReactNode => {
    switch (g) {
      case "identity":
        return (
          <IdentityFields form={form} patch={patch} dirty={dirtyGroups.identity} inherited={inherited} />
        );
      case "banner":
        return (
          <BannerFields form={form} patch={patch} dirty={dirtyGroups.banner} invalid={bannerInvalid} />
        );
      case "slides":
        return (
          <SlidesFields
            form={form}
            patch={patch}
            patchWith={patchBrandingWith}
            dirty={dirtyGroups.slides}
          />
        );
      case "services":
        return <ServicesFields form={form} patch={patch} dirty={dirtyGroups.services} />;
      case "ticker":
        return <TickerFields form={form} patch={patch} dirty={dirtyGroups.ticker} />;
      case "whatsapp":
        return (
          <WhatsappFields form={form} patch={patch} dirty={dirtyGroups.whatsapp} invalid={waInvalid} />
        );
      case "display":
        return (
          <HospitalityFields
            form={form}
            patch={patch}
            patchWith={patchBrandingWith}
            dirty={dirtyGroups.display}
          />
        );
    }
  };

  return (
    <div className="space-y-4 sm:space-y-6">
      {/* Navigation rapide (miroir du formulaire du compte — mobile : rangée
          défilante, desktop : enroulée, puce de la section lue remplie). */}
      <div className="space-y-2">
        <nav
          ref={chipNavRef}
          className="flex items-center gap-1.5 overflow-x-auto pb-0.5 [scrollbar-width:none] sm:flex-wrap sm:overflow-visible [&::-webkit-scrollbar]:hidden"
          aria-label={t("settings.exp.jumpAria")}
        >
          {anchors.map((anchor) => {
            const activeChip = anchor.id === activeAnchor;
            return (
              <button
                key={anchor.id}
                type="button"
                data-chip={anchor.id}
                onClick={() => jumpTo(anchor.id)}
                aria-current={activeChip ? "true" : undefined}
                className={cn(
                  "shrink-0 whitespace-nowrap rounded-full px-3 py-1 text-xs font-medium transition-colors",
                  activeChip
                    ? "bg-primary text-primary-foreground"
                    : "glass-chip text-muted-foreground hover:text-foreground",
                )}
              >
                {t(anchor.labelKey)}
              </button>
            );
          })}
        </nav>
        <p className="text-xs text-muted-foreground">{desc}</p>
      </div>

      {/* Carte de la surcharge — même armature N°140 que le formulaire du
          compte : en-tête pictogramme, sous-sections séparées par des
          Separator, point « modifié ». */}
      <Card className="gap-4 py-4 sm:py-6">
        <CardHeader className="flex flex-row items-start justify-between gap-2 px-4 sm:px-6">
          <div className="grid gap-1.5">
            <CardTitle className="flex items-center gap-2 text-base">
              <span className="flex size-8 items-center justify-center rounded-lg bg-primary/15 text-primary">
                <Palette className="size-4" />
              </span>
              {title}
            </CardTitle>
            <CardDescription>{t("portal.editorOverrideCardDesc")}</CardDescription>
          </div>
          {hasOverride ? (
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="gap-1.5 text-destructive hover:text-destructive"
              onClick={() => setConfirmRemove(true)}
              disabled={saving}
            >
              <Undo2 className="size-3.5" />
              {t("portal.removeOverride")}
            </Button>
          ) : null}
        </CardHeader>
        <CardContent className="grid gap-6 px-4 sm:px-6">
          {PORTAL_OVERRIDE_GROUPS.map((g, i) => (
            <Fragment key={g}>
              {i > 0 ? <Separator /> : null}
              {active.has(g) ? (
                <div className="grid gap-2">
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <Badge className="gap-1 bg-primary/10 text-primary hover:bg-primary/10">
                      <Palette className="size-3" aria-hidden="true" />
                      {t("portal.groupPersonalized")}
                    </Badge>
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      className="h-7 gap-1 px-2 text-xs text-muted-foreground hover:text-foreground"
                      onClick={() => inheritGroup(g)}
                    >
                      <Undo2 className="size-3.5" />
                      {t("portal.groupReset")}
                    </Button>
                  </div>
                  {renderBrick(g)}
                </div>
              ) : (
                <InheritedGroup
                  group={g}
                  values={inherited}
                  sourceLabel={
                    sourceIsSite(g) && siteName
                      ? tf("portal.inheritedFromSite", { name: siteName })
                      : t("portal.inheritedFromAccount")
                  }
                  dirty={dirtyGroups[g]}
                  onPersonalize={() => personalize(g)}
                />
              )}
            </Fragment>
          ))}
        </CardContent>
      </Card>

      {/* Barre d'action unique (N°140) — sticky bas d'écran, visible dès la
          première modification. */}
      {dirtyCount > 0 && (
        <div className="sticky bottom-4 z-30 mik-rise">
          <div
            className="flex flex-col gap-2 rounded-xl border bg-card/95 p-3 pl-4 shadow-lg shadow-black/10 backdrop-blur sm:flex-row sm:flex-wrap sm:items-center sm:gap-x-3 sm:gap-y-2"
            role="status"
            aria-live="polite"
          >
            <div className="flex min-w-0 items-center gap-2.5">
              <span className="relative flex size-2 shrink-0" aria-hidden>
                <span className="absolute inline-flex size-full animate-ping rounded-full bg-primary opacity-60" />
                <span className="relative inline-flex size-2 rounded-full bg-primary" />
              </span>
              <p className="min-w-0 flex-1 text-sm font-medium">
                {tf("settings.exp.changes", { n: dirtyCount })}
              </p>
              {!canSave && (
                <button
                  type="button"
                  onClick={goToFirstError}
                  className="flex min-w-0 shrink-0 items-center gap-1 rounded text-xs text-destructive transition-colors hover:text-destructive/80 sm:hidden"
                >
                  <CircleAlert className="size-3.5 shrink-0" aria-hidden />
                  <span className="sr-only">{t("settings.exp.invalid")}</span>
                </button>
              )}
            </div>
            {!canSave && (
              <button
                type="button"
                onClick={goToFirstError}
                className="hidden items-center gap-1.5 rounded text-left text-xs text-destructive underline underline-offset-2 transition-colors hover:text-destructive/80 sm:flex"
              >
                <CircleAlert className="size-3.5 shrink-0" aria-hidden />
                {t("settings.exp.invalid")}
              </button>
            )}
            <div className="flex shrink-0 items-center gap-2 sm:ml-auto">
              <Button
                type="button"
                variant="outline"
                className="h-10 flex-1 sm:flex-none"
                onClick={() => setConfirmReset(true)}
                disabled={saving}
              >
                <RotateCcw className="size-4" />
                {t("settings.exp.reset")}
              </Button>
              <Button
                type="button"
                className="h-10 flex-1 sm:flex-none"
                onClick={() => saveAll(overridePayloadOf(form))}
                disabled={saving || !canSave}
              >
                {saving ? <Loader2 className="size-4 animate-spin" /> : <Check className="size-4" />}
                {saving ? t("common.saving") : t("portal.save")}
              </Button>
            </div>
          </div>
        </div>
      )}

      {/* Réinitialiser (confirmé) — jette les saisies non enregistrées. */}
      <AlertDialog open={confirmReset} onOpenChange={setConfirmReset}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{tf("settings.exp.resetTitle", { n: dirtyCount })}</AlertDialogTitle>
            <AlertDialogDescription>{t("settings.exp.resetDesc")}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-white hover:bg-destructive/90"
              onClick={() => {
                resetForm();
                setConfirmReset(false);
              }}
            >
              {t("settings.exp.reset")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* Retrait complet (confirmé) — le niveau supérieur reprend tous les
          groupes, re-déploiement au check-in suivant. */}
      <AlertDialog open={confirmRemove} onOpenChange={setConfirmRemove}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t("portal.removeOverrideTitle")}</AlertDialogTitle>
            <AlertDialogDescription>{t("portal.removeOverrideDesc")}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={saving}>{t("common.cancel")}</AlertDialogCancel>
            <AlertDialogAction
              disabled={saving}
              className="gap-1.5 bg-destructive text-white hover:bg-destructive/90"
              onClick={() => {
                setConfirmRemove(false);
                saveAll({});
              }}
            >
              {saving ? <Loader2 className="size-4 animate-spin" /> : <Undo2 className="size-4" />}
              {t("portal.removeOverride")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

/* ─── Groupe hérité (carte compacte : provenance + valeur résolue) ─── */

function InheritedGroup({
  group,
  values,
  sourceLabel,
  dirty,
  onPersonalize,
}: {
  group: OverrideGroupKey;
  values: PortalOverrideForm;
  sourceLabel: string;
  dirty: boolean;
  onPersonalize: () => void;
}) {
  const { t } = useI18n();
  const meta = GROUP_META[group];
  const Icon = meta.icon;
  return (
    <div id={meta.anchor} className="scroll-mt-24 grid gap-3 rounded-xl border bg-muted/30 p-4">
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div className="min-w-0">
          <h3 className="flex items-center gap-2 text-sm font-semibold">
            <Icon className="size-4 shrink-0 text-primary" aria-hidden="true" />
            {t(meta.labelKey)}
            <Badge variant="outline" className="gap-1 font-normal">
              <Undo2 className="size-3" aria-hidden="true" />
              {t("portal.groupInherited")}
            </Badge>
            {dirty && (
              <span className="size-1.5 shrink-0 animate-pulse rounded-full bg-primary" aria-hidden />
            )}
          </h3>
          <p className="mt-0.5 flex items-center gap-1 text-xs text-muted-foreground">
            <ArrowDownLeft className="size-3 shrink-0" aria-hidden="true" />
            {sourceLabel}
          </p>
        </div>
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="gap-1.5"
          onClick={onPersonalize}
        >
          <Palette className="size-3.5" />
          {t("portal.groupPersonalize")}
        </Button>
      </div>
      <GroupSummary group={group} values={values} />
    </div>
  );
}

/* ─── Résumé d'un groupe hérité (valeur RÉSOLUE, lecture seule) ─── */

function GroupSummary({ group, values }: { group: OverrideGroupKey; values: PortalOverrideForm }) {
  const { t, tf } = useI18n();
  const items: ReactNode[] = [];

  switch (group) {
    case "identity":
      if (values.logoUrl) {
        items.push(
          <Avatar key="logo" className="size-8 rounded-lg border bg-white">
            <AvatarImage src={values.logoUrl} alt={t("settings.logoAlt")} className="object-contain" />
            <AvatarFallback className="rounded-lg bg-muted text-muted-foreground">
              <ImageIcon className="size-4" aria-hidden="true" />
            </AvatarFallback>
          </Avatar>,
        );
      }
      if (values.displayName) {
        items.push(
          <span key="name" className="font-medium text-foreground">
            {values.displayName}
          </span>,
        );
      }
      if (values.waveLink) {
        items.push(
          <span key="wave" className="inline-flex items-center gap-1">
            <Store className="size-3" aria-hidden="true" />
            {t("portal.summaryWave")}
          </span>,
        );
      }
      break;
    case "banner":
      if (values.bannerUrl) {
        items.push(
          <img
            key="banner"
            src={values.bannerUrl}
            alt=""
            className="h-10 rounded-md object-cover"
            onError={(event) => {
              event.currentTarget.style.opacity = "0.3";
            }}
          />,
        );
      }
      break;
    case "slides":
      if (values.slides.length > 0) {
        items.push(
          <span key="count" className="inline-flex items-center gap-1">
            <Images className="size-3" aria-hidden="true" />
            {tf("portal.summarySlides", { n: values.slides.length })}
          </span>,
        );
        if (values.slides[0]) {
          items.push(
            <img
              key="slide"
              src={values.slides[0]}
              alt=""
              className="h-10 w-16 rounded-md object-cover"
              onError={(event) => {
                event.currentTarget.style.opacity = "0.3";
              }}
            />,
          );
        }
      }
      break;
    case "services":
      if (values.services.length > 0) {
        items.push(
          <span key="svc" className="min-w-0 truncate">
            {values.services.map((s) => s.label).join(" · ")}
          </span>,
        );
      }
      break;
    case "ticker":
      if (values.ticker.length > 0) {
        items.push(
          <span key="ticker" className="min-w-0 truncate">
            {values.ticker.join(" · ")}
          </span>,
        );
      }
      break;
    case "whatsapp":
      if (values.waNumber) {
        items.push(
          <span key="wa" className="inline-flex items-center gap-1 font-mono">
            <MessageCircle className="size-3" aria-hidden="true" />
            {values.waNumber}
            {values.waLabel ? ` — ${values.waLabel}` : ""}
          </span>,
        );
      }
      break;
    case "display":
      items.push(
        <span key="style" className="font-medium text-foreground">
          {values.portalStyle === "hospitality"
            ? t("portal.styleHospitality")
            : t("portal.styleCommercial")}
        </span>,
      );
      if (values.welcome) {
        items.push(
          <span key="welcome" className="min-w-0 truncate">
            « {values.welcome} »
          </span>,
        );
      }
      if (values.promos.length > 0) {
        items.push(
          <span key="promos">{tf("portal.summaryPromos", { n: values.promos.length })}</span>,
        );
      }
      if (values.socials.length > 0) {
        items.push(
          <span key="socials">{tf("portal.summarySocials", { n: values.socials.length })}</span>,
        );
      }
      break;
  }

  if (items.length === 0) {
    return <p className="text-xs text-muted-foreground">{t("portal.summaryNothing")}</p>;
  }
  return (
    <div className="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1.5 text-xs text-muted-foreground">
      {items}
    </div>
  );
}

/* ─── Brique « Identité » (propres aux surcharges : nom affiché, logo,
     lien marchand Wave) ─── */

function IdentityFields({
  form,
  patch,
  dirty,
  inherited,
}: {
  form: PortalOverrideForm;
  patch: (p: Partial<PortalOverrideForm>) => void;
  dirty: boolean;
  /** Valeurs héritées — servent de placeholders (l'utilisateur voit ce que
   * le niveau supérieur affiche pendant qu'il saisit sa version). */
  inherited: PortalOverrideForm;
}) {
  const { t } = useI18n();
  const fileInputRef = useRef<HTMLInputElement>(null);

  // Logo : image ≤ 300 Ko encodée en data URL (contrat partagé du logo,
  // même garde que la carte tickets du compte).
  function handleLogoFile(event: React.ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0];
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
      if (typeof reader.result === "string") patch({ logoUrl: reader.result });
    };
    reader.onerror = () => toast.error(t("settings.logoReadError"));
    reader.readAsDataURL(file);
  }

  return (
    <section className="grid gap-3" aria-labelledby="hot-exp-identity">
      <SubSectionHeader
        id="hot-exp-identity"
        icon={Fingerprint}
        title={t("portal.identityCard")}
        desc={t("portal.identityCardDesc")}
        dirty={dirty}
      />
      <div className="grid gap-4 sm:grid-cols-2">
        <div className="grid gap-2">
          <Label htmlFor="ov-display-name">{t("portal.displayName")}</Label>
          <Input
            id="ov-display-name"
            value={form.displayName}
            onChange={(event) => patch({ displayName: event.target.value })}
            maxLength={80}
            placeholder={inherited.displayName || t("portal.identityNamePlaceholder")}
          />
        </div>
        <div className="grid gap-2">
          <Label htmlFor="ov-wave">{t("portal.waveLink")}</Label>
          <Input
            id="ov-wave"
            value={form.waveLink}
            onChange={(event) => patch({ waveLink: event.target.value })}
            maxLength={300}
            placeholder={inherited.waveLink || "https://pay.wave.com/m/…/c/ci/"}
          />
        </div>
      </div>
      <div className="grid gap-2">
        <Label>{t("portal.logoUrl")}</Label>
        <div className="flex items-center gap-3">
          <Avatar className="size-14 rounded-xl border bg-white">
            {form.logoUrl ? (
              <AvatarImage src={form.logoUrl} alt={t("settings.logoAlt")} className="object-contain" />
            ) : null}
            <AvatarFallback className="rounded-xl bg-muted text-muted-foreground">
              <ImageIcon className="size-5" aria-hidden="true" />
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
              {form.logoUrl ? t("settings.change") : t("settings.upload")}
            </Button>
            {form.logoUrl ? (
              <Button
                type="button"
                variant="ghost"
                className="h-10 text-destructive hover:text-destructive"
                onClick={() => patch({ logoUrl: "" })}
              >
                <X className="size-4" />
                {t("settings.remove")}
              </Button>
            ) : null}
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
        <p className="text-xs text-muted-foreground">{t("portal.identityLogoHint")}</p>
      </div>
    </section>
  );
}
