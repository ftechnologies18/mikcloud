"use client";

// N°140 — Refonte UX de l'onglet « Expérience » (hub Hotspot,
// components/hotspot/views/hotspot-view.tsx).
//
// Retour utilisateur : « chaque réglage du portail captif est dans une carte
// séparée avec chacun son bouton d'enregistrement, ce qui complexifie
// l'expérience » + « supprimer la carte "Connecter un vrai routeur
// MikroTik" ». L'onglet portait DIX cartes à FOOTER d'enregistrement : le
// gérant ne savait plus ce qui était sauvé, scrollait un mur de chrome
// répété (10 en-têtes, 10 boutons), et la carte guide MikroTik occupait le
// bas de page pour un branchement qui se fait une fois dans la vue Routeurs.
//
// La refonte :
//   • UN SEUL ENREGISTREMENT — plus aucun bouton par carte. Une barre
//     d'action STICKY (bas d'écran, n'apparaît que sur modification) porte
//     le compteur de groupes modifiés, « Réinitialiser » et « Enregistrer
//     tout ». Le PUT /api/settings part en UN SEUL appel avec tous les
//     champs (le handler Go accepte déjà tout champ présent, nil = inchangé
//     — contrat inchangé, corps défensif plat + tenant{…} conservé).
//   • DEUX CARTES THÉMATIQUES — « Vouchers & tickets imprimés » (politique
//     d'expiration, import des routeurs, DNS + logo + aperçu QR) et
//     « Portail captif » (inscription, bannière, carrousel, services,
//     bandeau animé, WhatsApp, mode d'affichage). Les sous-sections sont
//     séparées par des Separator et portent un POINT « modifié » qui
//     s'allume dès que leur groupe diverge de l'état enregistré ; les
//     pictogrammes des anciennes cartes sont conservés (repères visuels).
//   • NAVIGATION RAPIDE — puces d'ancrage (desktop) sous l'en-tête de
//     l'onglet : saut doux vers chaque sous-section (scroll-mt sous le
//     topbar sticky).
//   • CARTE GUIDE MIKROTIK SUPPRIMÉE — le branchement matériel vit dans la
//     vue Routeurs (et le README) ; la page reste 100 % réglages.
//   • GARDE-FOUS — beforeunload tant que des modifications ne sont pas
//     enregistrées, Cmd/Ctrl+Entrée enregistre, enregistrement bloqué tant
//     qu'une validation locale échoue (délai d'expiration, URL bannière,
//     numéro WhatsApp).
//
// Les comportements de champs sont conservés À L'IDENTIQUE (téléversements
// R2 avec repli data URL, aperçu QR régénéré, aperçu bannière, aperçu du
// lien WhatsApp, analytics de la vitrine, indices et limites) — seule
// l'armature UX change.
//
// N°142 — Durcissement UX (suite du retour « peut encore l'améliorer ? ») :
//   • GARDE DE SORTIE D'ONGLET — le formulaire remonte son compteur de
//     groupes modifiés au hub (onDirtyChange) : changer d'onglet avec des
//     saisies non enregistrées ouvre une confirmation au lieu de tout
//     jeter silencieusement ; « Voir le portail » (onPreviewPortal)
//     traverse la même garde.
//   • NAVIGATION MOBILE + SCROLLSPY — les puces d'ancres, desktop-only au
//     N°140, deviennent une rangée défilante sur téléphone (le gérant de
//     cyber est mobile-first) ; la puce de la section lue se remplit.
//   • RÉINITIALISER CONFIRMÉ — le bouton jetait {n} groupes de saisie sans
//     confirmation ; un AlertDialog demande avant d'effacer.
//   • ERREUR LOCALISÉE — le message d'invalidité de la barre devient un
//     bouton : il mène au premier champ en erreur et lui donne le focus.

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  BadgeCheck,
  Banknote,
  BookOpen,
  CalendarClock,
  Camera,
  Car,
  Check,
  CircleAlert,
  Code,
  Coffee,
  CreditCard,
  Gamepad2,
  Globe,
  Headset,
  Image as ImageIcon,
  Images,
  Laptop,
  Link as LinkIcon,
  ListChecks,
  Loader2,
  Megaphone,
  MessageCircle,
  MonitorSmartphone,
  MousePointerClick,
  Phone,
  Plus,
  Printer,
  RotateCcw,
  Scissors,
  Sparkles,
  Store,
  Ticket,
  UserPlus,
  Utensils,
  Wifi,
  Wrench,
  X,
  Zap,
  ImagePlus,
  Eye,
  type LucideIcon,
} from "lucide-react";
import { toast } from "sonner";

import { api, apiUpload } from "@/lib/hotspot/api";
import { SETTINGS_QUERY_KEY } from "@/components/hotspot/parts/sd-currency";
import { useI18n } from "@/lib/hotspot/i18n";
import type {
  AppSettings,
  PortalPromo,
  PortalService,
  PortalSocial,
  PromoStats,
} from "@/lib/hotspot/types";
import { PORTAL_SERVICE_ICONS } from "@/lib/hotspot/types";
import { qrWithLogoDataUrl } from "@/components/hotspot/parts/template-render";
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
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Separator } from "@/components/ui/separator";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { cn } from "@/lib/utils";

/* ─── Forme locale : TOUT l'état éditable de l'onglet, un seul objet ─── */

interface HotspotForm {
  expiryMode: "keep" | "remove";
  expiryDays: string;
  autoImport: boolean;
  joinButton: boolean;
  dnsName: string;
  logoUrl: string;
  bannerUrl: string;
  slides: string[];
  services: PortalService[];
  ticker: string[];
  waNumber: string;
  waLabel: string;
  portalStyle: string;
  welcome: string;
  promos: PortalPromo[];
  socials: PortalSocial[];
}

/* Décodeurs défensifs — identiques aux anciennes cartes (JSON invalide ou
 * absent du compte = valeur neutre, jamais de page cassée). */
function parseStringArray(raw: string | undefined): string[] {
  try {
    const parsed = JSON.parse(raw || "[]") as string[];
    return Array.isArray(parsed) ? parsed.filter((u) => typeof u === "string" && u) : [];
  } catch {
    return [];
  }
}

function parseServices(raw: string | undefined): PortalService[] {
  try {
    const parsed = JSON.parse(raw || "[]") as PortalService[];
    return Array.isArray(parsed) ? parsed : [];
  } catch {
    return [];
  }
}

function parseWhatsapp(raw: string | undefined): { number: string; label: string } {
  try {
    const parsed = JSON.parse(raw || "{}") as { number?: unknown; label?: unknown };
    return {
      number: typeof parsed.number === "string" ? parsed.number : "",
      label: typeof parsed.label === "string" ? parsed.label : "",
    };
  } catch {
    return { number: "", label: "" };
  }
}

function parsePromos(raw: string | undefined): PortalPromo[] {
  try {
    const parsed = JSON.parse(raw || "[]") as PortalPromo[];
    return Array.isArray(parsed) ? parsed : [];
  } catch {
    return [];
  }
}

function parseSocials(raw: string | undefined): PortalSocial[] {
  try {
    const parsed = JSON.parse(raw || "[]") as PortalSocial[];
    return Array.isArray(parsed) ? parsed : [];
  } catch {
    return [];
  }
}

/** État initial — même lecture défensive que les anciennes cartes (champ
 * absent du JSON = défaut effectif historique, pas « vide »). */
function initialForm(settings: AppSettings): HotspotForm {
  return {
    expiryMode: settings.tenant.expiryPolicyMode === "remove" ? "remove" : "keep",
    expiryDays: String(settings.tenant.expiryPolicyAfterDays ?? 30),
    autoImport: settings.tenant.autoImportRouterUsers ?? settings.autoImportRouterUsers ?? true,
    joinButton: settings.tenant.joinButton ?? true,
    dnsName: settings.tenant.dnsName ?? "",
    logoUrl: settings.tenant.logoUrl ?? "",
    bannerUrl: settings.tenant.bannerUrl ?? "",
    slides: parseStringArray(settings.tenant.portalSlides),
    services: parseServices(settings.tenant.portalServices),
    ticker: parseStringArray(settings.tenant.portalTicker),
    ...(() => {
      const wa = parseWhatsapp(settings.tenant.portalWhatsapp);
      return { waNumber: wa.number, waLabel: wa.label };
    })(),
    portalStyle: settings.tenant.portalStyle ?? "",
    welcome: settings.tenant.portalWelcome ?? "",
    promos: parsePromos(settings.tenant.portalPromos),
    socials: parseSocials(settings.tenant.portalSocials),
  };
}

/** Groupes « modifiés » — un point par sous-section de la page. */
interface DirtyGroups {
  expiry: boolean;
  autoImport: boolean;
  voucher: boolean;
  join: boolean;
  banner: boolean;
  slides: boolean;
  services: boolean;
  ticker: boolean;
  whatsapp: boolean;
  hospitality: boolean;
}

const sameJson = (a: unknown, b: unknown) => JSON.stringify(a) === JSON.stringify(b);

function computeDirty(form: HotspotForm, base: HotspotForm): DirtyGroups {
  return {
    expiry: form.expiryMode !== base.expiryMode || form.expiryDays !== base.expiryDays,
    autoImport: form.autoImport !== base.autoImport,
    voucher: form.dnsName !== base.dnsName || form.logoUrl !== base.logoUrl,
    join: form.joinButton !== base.joinButton,
    banner: form.bannerUrl !== base.bannerUrl,
    slides: !sameJson(form.slides, base.slides),
    services: !sameJson(form.services, base.services),
    ticker: !sameJson(form.ticker, base.ticker),
    whatsapp: form.waNumber !== base.waNumber || form.waLabel !== base.waLabel,
    hospitality:
      form.portalStyle !== base.portalStyle ||
      form.welcome !== base.welcome ||
      !sameJson(form.promos, base.promos) ||
      !sameJson(form.socials, base.socials),
  };
}

/* Ancres de la navigation rapide — module scope : partagées par les puces
 * et le scrollspy (la puce de la section lue se remplit au scroll). */
const ANCHORS = [
  { id: "hot-exp-vouchers", labelKey: "settings.exp.navVouchers" },
  { id: "hot-exp-join", labelKey: "settings.exp.navJoin" },
  { id: "hot-exp-banner", labelKey: "settings.exp.navBanner" },
  { id: "hot-exp-slides", labelKey: "settings.exp.navSlides" },
  { id: "hot-exp-services", labelKey: "settings.exp.navServices" },
  { id: "hot-exp-ticker", labelKey: "settings.exp.navTicker" },
  { id: "hot-exp-whatsapp", labelKey: "settings.exp.navWhatsapp" },
  { id: "hot-exp-mode", labelKey: "settings.exp.navMode" },
];

/* ─── Onglet « Expérience » — un formulaire, deux cartes, une barre ─── */

export function HotspotExperience({
  settings,
  onDirtyChange,
  onPreviewPortal,
}: {
  settings: AppSettings;
  /** Remonte le nombre de groupes modifiés au hub — garde de sortie d'onglet (N°142). */
  onDirtyChange?: (count: number) => void;
  /** « Voir le portail » — navigue vers l'onglet Portail via la garde du hub (N°142). */
  onPreviewPortal?: () => void;
}) {
  const { t, tf } = useI18n();
  const queryClient = useQueryClient();
  const [form, setForm] = useState<HotspotForm>(() => initialForm(settings));
  // État de référence « enregistré » : sert au point modifié, au bouton
  // Réinitialiser et à la bascule propre après enregistrement (le refetch
  // d'arrière-plan ne PIETINE jamais la saisie — l'état vit ici, pas dans
  // les props).
  const [baseline, setBaseline] = useState<HotspotForm>(() => initialForm(settings));

  const patch = useCallback((p: Partial<HotspotForm>) => setForm((f) => ({ ...f, ...p })), []);
  // Variante FONCTIONNELLE — pour les téléversements async (slides, promos) :
  // comme les setXxx((list) => …) des anciennes cartes, elle part TOUJOURS
  // de l'état le plus frais, même si deux téléversements se chevauchent.
  const patchWith = useCallback((fn: (f: HotspotForm) => HotspotForm) => setForm(fn), []);

  const dirty = useMemo(() => computeDirty(form, baseline), [form, baseline]);
  const dirtyCount = useMemo(() => Object.values(dirty).filter(Boolean).length, [dirty]);

  // N°142 — le hub garde le compte pour confirmer les sorties d'onglet
  // (le callback est stable — useCallback côté hub).
  useEffect(() => {
    onDirtyChange?.(dirtyCount);
  }, [dirtyCount, onDirtyChange]);

  // N°142 — scrollspy : la puce de la dernière section dont l'en-tête a
  // passé la ligne de lecture (topbar sticky + respiration) se remplit ;
  // la rangée défilante (mobile) la suit pour rester cadrée.
  const [activeAnchor, setActiveAnchor] = useState(ANCHORS[0].id);
  const chipNavRef = useRef<HTMLElement | null>(null);
  useEffect(() => {
    let raf = 0;
    const measure = () => {
      cancelAnimationFrame(raf);
      raf = requestAnimationFrame(() => {
        // Bas de page atteint (à 60 px près) : la DERNIÈRE section est
        // celle qu'on lit — sans cette règle, les sections dont l'en-tête
        // ne peut pas monter au-dessus de la ligne (page plus courte que
        // la cible d'ancrage : le scroll est clampé) resteraient muettes.
        const doc = document.documentElement;
        const maxScroll = doc.scrollHeight - window.innerHeight;
        if (maxScroll > 0 && window.scrollY >= maxScroll - 60) {
          setActiveAnchor(ANCHORS[ANCHORS.length - 1].id);
          return;
        }
        const line = 140;
        let current = ANCHORS[0].id;
        for (const anchor of ANCHORS) {
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
  }, []);
  useEffect(() => {
    // N°142-bis — correction : suivre la puce active NE DOIT toucher qu'au
    // défilement HORIZONTAL de la rangée. scrollIntoView(block:"nearest")
    // remontait la PAGE pour révéler la rangée dès qu'elle quittait le
    // viewport — l'utilisateur ne pouvait plus descendre (aspiration en
    // haut à chaque changement de section). Ici on ne règle que
    // scrollLeft : la page ne bouge jamais.
    const nav = chipNavRef.current;
    const chip = nav?.querySelector<HTMLElement>(`[data-chip="${activeAnchor}"]`);
    if (!nav || !chip) return;
    const navRect = nav.getBoundingClientRect();
    const chipRect = chip.getBoundingClientRect();
    const target = chipRect.left - navRect.left - (nav.clientWidth - chip.offsetWidth) / 2;
    const left = Math.max(0, nav.scrollLeft + target);
    if (Math.abs(left - nav.scrollLeft) >= 1) nav.scrollTo({ left, behavior: "smooth" });
  }, [activeAnchor]);

  // N°142 — Réinitialiser demande confirmation (le bouton jette {n} groupes
  // de saisie d'un coup — plus de misclick fatal).
  const [confirmReset, setConfirmReset] = useState(false);

  /* Validations locales — miroir des garde-fous des anciennes cartes.
   * L'enregistrement reste bloqué tant qu'un champ triche. */
  const daysNum = parseInt(form.expiryDays, 10);
  const daysValid = Number.isInteger(daysNum) && daysNum >= 1 && daysNum <= 365;
  const expiryInvalid = form.expiryMode === "remove" && !daysValid;
  const bannerInvalid =
    form.bannerUrl.trim() !== "" &&
    !form.bannerUrl.trim().startsWith("https://") &&
    !form.bannerUrl.trim().startsWith("data:image/");
  const waDigits = form.waNumber.replace(/[^0-9]/g, "");
  const waInvalid = waDigits !== "" && (waDigits.length < 8 || waDigits.length > 15);
  const canSave = !expiryInvalid && !bannerInvalid && !waInvalid;

  // N°142 — l'erreur de la barre devient un raccourci : mène au premier
  // champ fautif (ordre de lecture de la page) et lui donne le focus une
  // fois le scroll doux posé.
  const firstError = expiryInvalid
    ? { section: "hot-exp-vouchers", input: "expiry-days" }
    : bannerInvalid
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
    mutationFn: () => {
      // Sérialisation À L'IDENTIQUE des anciennes cartes, fusionnée en un
      // seul corps : champs plats + forme imbriquée tenant{…} (le plat
      // prime côté backend, le décodeur Go ignore les champs inconnus).
      // expiryPolicyAfterDays omis en mode « conserver » (undefined = pas
      // de JSON = nil = inchangé côté serveur — comportement ExpiryCard).
      const afterDays = form.expiryMode === "remove" && daysValid ? daysNum : undefined;
      const flat = {
        expiryPolicyMode: form.expiryMode,
        expiryPolicyAfterDays: afterDays,
        autoImportRouterUsers: form.autoImport,
        joinButton: form.joinButton,
        dnsName: form.dnsName.trim(),
        logoUrl: form.logoUrl,
        bannerUrl: form.bannerUrl.trim(),
        portalSlides: form.slides,
        portalServices: form.services,
        portalTicker: form.ticker,
        portalWhatsapp: { number: form.waNumber, label: form.waLabel },
        portalStyle: form.portalStyle,
        portalWelcome: form.welcome,
        portalPromos: form.promos,
        portalSocials: form.socials,
      };
      return api<AppSettings>("/api/settings", {
        method: "PUT",
        body: { ...flat, tenant: { ...flat } },
      });
    },
    onSuccess: () => {
      toast.success(t("settings.exp.savedToast"));
      // La saisie devient la nouvelle référence : les points « modifié »
      // s'éteignent immédiatement, sans attendre le refetch.
      setBaseline({ ...form });
      void queryClient.invalidateQueries({ queryKey: SETTINGS_QUERY_KEY });
    },
    onError: (error: Error) => toast.error(error.message),
  });
  const { mutate: saveAll, isPending: saving } = saveMutation;

  /* Garde-fou navigation : ne pas perdre 10 groupes de saisie sur un
   * clic accidentel hors de la page. */
  useEffect(() => {
    if (dirtyCount === 0) return;
    const handler = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", handler);
    return () => window.removeEventListener("beforeunload", handler);
  }, [dirtyCount]);

  /* Cmd/Ctrl+Entrée enregistre — raccourci des consoles modernes. */
  useEffect(() => {
    const handler = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key === "Enter") {
        if (dirtyCount > 0 && canSave && !saving) saveAll();
      }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [dirtyCount, canSave, saving, saveAll]);

  const resetForm = () => setForm({ ...baseline });

  /* Ancres — scroll-mt-24 sous le topbar sticky de l'app-shell (la liste
   * vit au module scope : partagée avec le scrollspy N°142). */
  const jumpTo = (id: string) =>
    document.getElementById(id)?.scrollIntoView({ behavior: "smooth", block: "start" });

  return (
    <div className="space-y-4 sm:space-y-6">
      {/* Navigation rapide + rappel du modèle d'enregistrement unique.
          Mobile : rangée défilante (le gérant de cyber est mobile-first) ;
          la puce de la section lue se remplit (scrollspy) et la rangée la
          suit pour rester cadrée. Desktop : rangée enroulée inchangée. */}
      <div className="space-y-2">
        <nav
          ref={chipNavRef}
          className="flex items-center gap-1.5 overflow-x-auto pb-0.5 [scrollbar-width:none] sm:flex-wrap sm:overflow-visible [&::-webkit-scrollbar]:hidden"
          aria-label={t("settings.exp.jumpAria")}
        >
          {ANCHORS.map((anchor) => {
            const active = anchor.id === activeAnchor;
            return (
              <button
                key={anchor.id}
                type="button"
                data-chip={anchor.id}
                onClick={() => jumpTo(anchor.id)}
                aria-current={active ? "true" : undefined}
                className={cn(
                  "shrink-0 whitespace-nowrap rounded-full px-3 py-1 text-xs font-medium transition-colors",
                  active
                    ? "bg-primary text-primary-foreground"
                    : "glass-chip text-muted-foreground hover:text-foreground",
                )}
              >
                {t(anchor.labelKey)}
              </button>
            );
          })}
        </nav>
        <div className="flex flex-wrap items-center justify-between gap-2">
          <p className="text-xs text-muted-foreground">{t("settings.exp.hint")}</p>
          {onPreviewPortal && (
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="gap-1.5 text-xs"
              onClick={onPreviewPortal}
            >
              <Eye className="size-3.5" aria-hidden />
              {t("settings.exp.previewPortal")}
            </Button>
          )}
        </div>
      </div>

      {/* ══ CARTE 1 — Vouchers & tickets imprimés ══ */}
      <Card className="gap-4 py-4 sm:py-6">
        <CardHeader className="px-4 sm:px-6">
          <CardTitle className="flex items-center gap-2 text-base">
            <span className="flex size-8 items-center justify-center rounded-lg bg-primary/15 text-primary">
              <Ticket className="size-4" />
            </span>
            {t("settings.exp.cardVouchers")}
          </CardTitle>
          <CardDescription>{t("settings.exp.cardVouchersDesc")}</CardDescription>
        </CardHeader>
        <CardContent className="grid gap-6 px-4 sm:px-6">
          <ExpiryFields form={form} patch={patch} dirty={dirty.expiry} daysValid={daysValid} />
          <Separator />
          <AutoImportFields form={form} patch={patch} dirty={dirty.autoImport} />
          <Separator />
          <VoucherFields form={form} patch={patch} dirty={dirty.voucher} />
        </CardContent>
      </Card>

      {/* ══ CARTE 2 — Portail captif : ce que voient vos invités ══ */}
      <Card className="gap-4 py-4 sm:py-6">
        <CardHeader className="px-4 sm:px-6">
          <CardTitle className="flex items-center gap-2 text-base">
            <span className="flex size-8 items-center justify-center rounded-lg bg-primary/15 text-primary">
              <MonitorSmartphone className="size-4" />
            </span>
            {t("settings.exp.cardPortal")}
          </CardTitle>
          <CardDescription>{t("settings.exp.cardPortalDesc")}</CardDescription>
        </CardHeader>
        <CardContent className="grid gap-6 px-4 sm:px-6">
          <JoinFields form={form} patch={patch} dirty={dirty.join} />
          <Separator />
          <BannerFields form={form} patch={patch} dirty={dirty.banner} invalid={bannerInvalid} />
          <Separator />
          <SlidesFields form={form} patch={patch} patchWith={patchWith} dirty={dirty.slides} />
          <Separator />
          <ServicesFields form={form} patch={patch} dirty={dirty.services} />
          <Separator />
          <TickerFields form={form} patch={patch} dirty={dirty.ticker} />
          <Separator />
          <WhatsappFields form={form} patch={patch} dirty={dirty.whatsapp} invalid={waInvalid} />
          <Separator />
          <HospitalityFields form={form} patch={patch} patchWith={patchWith} dirty={dirty.hospitality} />
        </CardContent>
      </Card>

      {/* ══ BARRE D'ACTION UNIQUE — sticky bas d'écran, visible dès la
          première modification (mik-rise), disparaît une fois propre. */}
      {dirtyCount > 0 && (
        <div className="sticky bottom-4 z-30 mik-rise">
          <div
            className="flex flex-col gap-2 rounded-xl border bg-card/95 p-3 pl-4 shadow-lg shadow-black/10 backdrop-blur sm:flex-row sm:flex-wrap sm:items-center sm:gap-x-3 sm:gap-y-2"
            role="status"
            aria-live="polite"
          >
            {/* Compteur — pleine largeur en mobile (une ligne, jamais de
                retour à la 3e ligne), rangée flex en desktop. */}
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
                onClick={() => saveAll()}
                disabled={saving || !canSave}
              >
                {saving ? <Loader2 className="size-4 animate-spin" /> : <Check className="size-4" />}
                {saving ? t("common.saving") : t("settings.exp.saveAll")}
              </Button>
            </div>
          </div>
        </div>
      )}

      {/* N°142 — Réinitialiser demande confirmation : le bouton jette
          TOUTES les saisies non enregistrées d'un coup. */}
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
    </div>
  );
}

/* ─── Briques communes des sous-sections ─── */

/** En-tête de sous-section : pictogramme (repère des anciennes cartes) +
 * libellé + point « modifié » (pulse discret quand le groupe diverge de
 * l'état enregistré) + description. L'id EST l'ancre de la nav rapide. */
function SubSectionHeader({
  id,
  icon: Icon,
  title,
  desc,
  dirty,
}: {
  id: string;
  icon: LucideIcon;
  title: string;
  desc?: string;
  dirty: boolean;
}) {
  return (
    <div id={id} className="scroll-mt-24 gap-0.5">
      <h3 className="flex items-center gap-2 text-sm font-semibold">
        <Icon className="size-4 shrink-0 text-primary" aria-hidden />
        {title}
        {dirty && (
          <span className="size-1.5 shrink-0 animate-pulse rounded-full bg-primary" aria-hidden />
        )}
      </h3>
      {desc && <p className="text-xs leading-relaxed text-muted-foreground">{desc}</p>}
    </div>
  );
}

/** Ligne interrupteur — libellé à gauche, Switch à droite (zone tactile
 * confortable), puis les DEUX comportements décrits : celui du réglage
 * courant est mis en avant, l'autre reste lisible (conséquence du
 * basculement). */
function SwitchRow({
  id,
  label,
  ariaLabel,
  checked,
  onCheckedChange,
}: {
  id: string;
  label: string;
  ariaLabel: string;
  checked: boolean;
  onCheckedChange: (value: boolean) => void;
}) {
  return (
    <div className="flex items-center justify-between gap-3 rounded-lg border p-3">
      <Label htmlFor={id} className="cursor-pointer text-sm">
        {label}
      </Label>
      <Switch
        id={id}
        checked={checked}
        onCheckedChange={onCheckedChange}
        aria-label={ariaLabel}
        className="shrink-0"
      />
    </div>
  );
}

function DualStateDesc({
  enabled,
  enabledText,
  disabledText,
}: {
  enabled: boolean;
  enabledText: string;
  disabledText: string;
}) {
  return (
    <>
      <p className={cn("text-xs leading-relaxed", enabled ? "text-foreground" : "text-muted-foreground")}>
        {enabledText}
      </p>
      <p className={cn("text-xs leading-relaxed", !enabled ? "text-foreground" : "text-muted-foreground")}>
        {disabledText}
      </p>
    </>
  );
}

/* ─── Sous-sections (contrôlées : form + patch) ─── */

interface SectionProps {
  form: HotspotForm;
  patch: (p: Partial<HotspotForm>) => void;
  /** Mise à jour fonctionnelle (téléversements async — cf. HotspotExperience). */
  patchWith?: (fn: (f: HotspotForm) => HotspotForm) => void;
  dirty: boolean;
}

/** Expiration des vouchers (F1/F5) — politique de nettoyage des expirés,
 * appliquée automatiquement par le moteur d'expiration du cloud. */
function ExpiryFields({ form, patch, dirty, daysValid }: SectionProps & { daysValid: boolean }) {
  const { t } = useI18n();
  return (
    <section className="grid gap-3" aria-labelledby="hot-exp-vouchers">
      <SubSectionHeader
        id="hot-exp-vouchers"
        icon={CalendarClock}
        title={t("settings.expiryCard")}
        desc={t("settings.expiryCardDesc")}
        dirty={dirty}
      />
      <RadioGroup
        value={form.expiryMode}
        onValueChange={(value) => patch({ expiryMode: value === "remove" ? "remove" : "keep" })}
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
      {form.expiryMode === "remove" && (
        <div className="grid gap-2">
          <Label htmlFor="expiry-days">{t("settings.expiryDays")}</Label>
          <Input
            id="expiry-days"
            type="number"
            min={1}
            max={365}
            value={form.expiryDays}
            onChange={(event) => patch({ expiryDays: event.target.value })}
            className="h-10"
            aria-invalid={!daysValid || undefined}
          />
          <p className={daysValid ? "text-xs text-muted-foreground" : "text-xs text-destructive"}>
            {daysValid ? t("settings.expiryDaysHint") : t("settings.expiryDaysInvalid")}
          </p>
        </div>
      )}
    </section>
  );
}

/** Import automatique (purge P1) — comportement de la synchro agent pour
 * les utilisateurs inconnus du cloud (créés via Winbox…). */
function AutoImportFields({ form, patch, dirty }: SectionProps) {
  const { t } = useI18n();
  return (
    <section className="grid gap-3" aria-labelledby="hot-exp-autoimport">
      <SubSectionHeader
        id="hot-exp-autoimport"
        icon={UserPlus}
        title={t("settings.autoImport.title")}
        desc={t("settings.autoImport.desc")}
        dirty={dirty}
      />
      <SwitchRow
        id="auto-import-router-users"
        label={t("settings.autoImport.switchLabel")}
        ariaLabel={t("settings.autoImport.aria")}
        checked={form.autoImport}
        onCheckedChange={(value) => patch({ autoImport: value })}
      />
      <DualStateDesc
        enabled={form.autoImport}
        enabledText={t("settings.autoImport.enabledDesc")}
        disabledText={t("settings.autoImport.disabledDesc")}
      />
    </section>
  );
}

/** Identité des tickets (F2) — DNS + logo (variables {{dnsName}}/{{logo}}),
 * avec aperçu live « logo au centre du QR » (même composition que les
 * tickets réels, régénéré à chaque changement de logo). */
function VoucherFields({ form, patch, dirty }: SectionProps) {
  const { t } = useI18n();
  const fileInputRef = useRef<HTMLInputElement>(null);
  const [qrPreview, setQrPreview] = useState("");
  useEffect(() => {
    let cancelled = false;
    qrWithLogoDataUrl("MIKCLOUD\nDEMO-2026", form.logoUrl || undefined)
      .then((url) => {
        if (!cancelled) setQrPreview(url);
      })
      .catch(() => {
        if (!cancelled) setQrPreview("");
      });
    return () => {
      cancelled = true;
    };
  }, [form.logoUrl]);

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
      if (typeof reader.result === "string") patch({ logoUrl: reader.result });
    };
    reader.onerror = () => toast.error(t("settings.logoReadError"));
    reader.readAsDataURL(file);
  }

  return (
    <section className="grid gap-3" aria-labelledby="hot-exp-voucher">
      <SubSectionHeader
        id="hot-exp-voucher"
        icon={Globe}
        title={t("settings.voucherCard")}
        desc={t("settings.voucherCardDesc")}
        dirty={dirty}
      />
      <div className="grid gap-4 sm:grid-cols-2">
        <div className="grid gap-2">
          <Label htmlFor="voucher-dns">{t("settings.dnsName")}</Label>
          <div className="relative">
            <Globe className="absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" aria-hidden />
            <Input
              id="voucher-dns"
              className="h-10 pl-9"
              placeholder="wifi.mondomaine.ci"
              value={form.dnsName}
              onChange={(event) => patch({ dnsName: event.target.value })}
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
              {form.logoUrl ? (
                <AvatarImage src={form.logoUrl} alt={t("settings.logoAlt")} className="object-contain" />
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
                {form.logoUrl ? t("settings.change") : t("settings.upload")}
              </Button>
              {form.logoUrl && (
                <Button
                  type="button"
                  variant="ghost"
                  className="h-10 text-destructive hover:text-destructive"
                  onClick={() => patch({ logoUrl: "" })}
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
              <img src={qrPreview} alt={t("settings.qrPreviewAlt")} className="size-20 shrink-0" />
            ) : (
              <Skeleton className="size-20 shrink-0" />
            )}
            <p className="text-xs text-muted-foreground">{t("settings.qrPreviewHint")}</p>
          </div>
        </div>
      </div>
    </section>
  );
}

/** Bouton « S'inscrire » du portail (N°46) — affichage dynamique piloté
 * par le gérant (config figée + fetch live côté portail). */
function JoinFields({ form, patch, dirty }: SectionProps) {
  const { t } = useI18n();
  return (
    <section className="grid gap-3" aria-labelledby="hot-exp-join">
      <SubSectionHeader
        id="hot-exp-join"
        icon={BadgeCheck}
        title={t("settings.joinButton.title")}
        desc={t("settings.joinButton.desc")}
        dirty={dirty}
      />
      <SwitchRow
        id="portal-join-button"
        label={t("settings.joinButton.switchLabel")}
        ariaLabel={t("settings.joinButton.aria")}
        checked={form.joinButton}
        onCheckedChange={(value) => patch({ joinButton: value })}
      />
      <DualStateDesc
        enabled={form.joinButton}
        enabledText={t("settings.joinButton.enabledDesc")}
        disabledText={t("settings.joinButton.disabledDesc")}
      />
    </section>
  );
}

/** Bannière du portail (N°45) — image de tête de la page de connexion.
 * Téléversement R2 (URL https permanente) avec repli data URL ≤ 500 Ko
 * si le stockage est indisponible — le gérant n'est jamais bloqué. */
function BannerFields({ form, patch, dirty, invalid }: SectionProps & { invalid: boolean }) {
  const { t } = useI18n();
  const [uploading, setUploading] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);

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
      const data = new FormData();
      data.append("file", file);
      const res = await apiUpload<{ url: string }>("/api/media", data, { timeoutMs: 60_000 });
      patch({ bannerUrl: res.url });
      toast.success(t("settings.bannerUploadOk"));
    } catch {
      // Repli N°45 : image intégrée au compte (≤ 500 Ko).
      if (file.size <= 500 * 1024) {
        const reader = new FileReader();
        reader.onload = () => {
          if (typeof reader.result === "string") patch({ bannerUrl: reader.result });
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

  return (
    <section className="grid gap-3" aria-labelledby="hot-exp-banner">
      <SubSectionHeader
        id="hot-exp-banner"
        icon={ImageIcon}
        title={t("settings.bannerCard")}
        desc={t("settings.bannerCardDesc")}
        dirty={dirty}
      />
      <div className="grid gap-2">
        <Label htmlFor="banner-url">{t("settings.bannerUrl")}</Label>
        <div className="flex flex-wrap items-center gap-2">
          <Input
            id="banner-url"
            className="h-10 min-w-0 flex-1"
            placeholder={t("settings.bannerUrlPlaceholder")}
            value={form.bannerUrl.startsWith("data:image/") ? "" : form.bannerUrl}
            onChange={(event) => patch({ bannerUrl: event.target.value })}
            aria-invalid={invalid || undefined}
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
          {form.bannerUrl && (
            <Button
              type="button"
              variant="ghost"
              className="h-10 text-destructive hover:text-destructive"
              onClick={() => patch({ bannerUrl: "" })}
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
        <p className={cn("text-xs", invalid ? "text-destructive" : "text-muted-foreground")}>
          {invalid ? t("settings.bannerInvalidUrl") : t("settings.bannerUrlHint")}
        </p>
      </div>

      {/* Aperçu live — même rendu que la page du portail (cover, arrondi). */}
      {form.bannerUrl && !invalid && (
        <div className="grid gap-2">
          <p className="text-xs font-medium">{t("settings.bannerPreviewAlt")}</p>
          <div className="overflow-hidden rounded-xl border bg-muted">
            <img
              src={form.bannerUrl}
              alt={t("settings.bannerPreviewAlt")}
              className="h-32 w-full object-cover"
            />
          </div>
        </div>
      )}
    </section>
  );
}

/** Slides du carrousel commercial (N°136) — ≤ 3 visuels R2 remplaçant
 * les images génériques du portail (mode commercial). */
function SlidesFields({
  form,
  patch,
  patchWith,
  dirty,
}: SectionProps) {
  const { t, tf } = useI18n();
  const [uploadingIdx, setUploadingIdx] = useState<number | null>(null);
  const [pendingIdx, setPendingIdx] = useState<number>(0);
  const slides = form.slides;

  async function handleSlideFile(idx: number, file?: File) {
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
      const data = new FormData();
      data.append("file", file);
      const res = await apiUpload<{ url: string }>("/api/media", data, { timeoutMs: 60_000 });
      patchWith?.((f) => ({
        ...f,
        slides: (() => {
          // Append (idx = prochaine place) ou remplacement (slot existant) —
          // aucun trou possible : les entrées vides sont retirées.
          const next = [...f.slides];
          next[idx] = res.url;
          return next.filter((u) => u !== "");
        })(),
      }));
      toast.success(t("settings.bannerUploadOk"));
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t("settings.hosp.imageFail"));
    } finally {
      setUploadingIdx(null);
    }
  }

  return (
    <section className="grid gap-3" aria-labelledby="hot-exp-slides">
      <SubSectionHeader
        id="hot-exp-slides"
        icon={Images}
        title={t("settings.slides.card")}
        desc={t("settings.slides.cardDesc")}
        dirty={dirty}
      />
      {slides.length === 0 && (
        <p className="text-xs text-muted-foreground">{t("settings.slides.empty")}</p>
      )}
      {[0, 1, 2].map((idx) => {
        const url = slides[idx];
        if (!url) return null;
        return (
          <div key={idx} className="flex items-center gap-3 rounded-lg border p-3">
            <img
              src={url}
              alt={tf("settings.slides.slot", { n: idx + 1 })}
              className="h-12 w-20 rounded-md object-cover"
              onError={(event) => {
                event.currentTarget.style.opacity = "0.3";
              }}
            />
            <div className="min-w-0 flex-1">
              <p className="text-sm font-medium">{tf("settings.slides.slot", { n: idx + 1 })}</p>
              <p className="truncate text-xs text-muted-foreground">{url}</p>
            </div>
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={uploadingIdx === idx}
              onClick={() => document.getElementById(`slide-file-${idx}`)?.click()}
            >
              <ImagePlus className="size-4" />
              {uploadingIdx === idx ? t("settings.uploading") : t("settings.upload")}
            </Button>
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => patch({ slides: slides.filter((_, i) => i !== idx) })}
            >
              <X className="size-4" />
              <span className="sr-only">{t("settings.slides.remove")}</span>
            </Button>
            <input
              id={`slide-file-${idx}`}
              type="file"
              accept="image/*"
              className="hidden"
              onChange={(event) => {
                const file = event.target.files?.[0];
                event.target.value = "";
                void handleSlideFile(idx, file);
              }}
            />
          </div>
        );
      })}
      <div className="flex flex-wrap items-center gap-2">
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={slides.length >= 3}
          onClick={() => {
            // Ouvre le sélecteur pour le prochain slot libre.
            setPendingIdx(slides.length);
            document.getElementById("slide-file-new")?.click();
          }}
        >
          <ImagePlus className="size-4" />
          {t("settings.slides.add")}
        </Button>
        <span className="text-xs text-muted-foreground">{slides.length}/3</span>
      </div>
      <p className="text-xs text-muted-foreground">{t("settings.slides.hint")}</p>
      <input
        id="slide-file-new"
        type="file"
        accept="image/*"
        className="hidden"
        onChange={(event) => {
          const file = event.target.files?.[0];
          event.target.value = "";
          void handleSlideFile(pendingIdx, file);
        }}
      />
    </section>
  );
}

/** Services du portail (N°137) — section « Nos Services » (mode
 * commercial) : ≤ 6 lignes {icône curée + libellé}. */
function ServicesFields({ form, patch, dirty }: SectionProps) {
  const { t } = useI18n();
  const services = form.services;

  return (
    <section className="grid gap-3" aria-labelledby="hot-exp-services">
      <SubSectionHeader
        id="hot-exp-services"
        icon={ListChecks}
        title={t("settings.svc.card")}
        desc={t("settings.svc.cardDesc")}
        dirty={dirty}
      />
      <div className="grid gap-2">
        <div className="flex items-center justify-between">
          <Label>{t("settings.svc.list")}</Label>
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={services.length >= 6}
            onClick={() => patch({ services: [...services, { icon: "fa-wifi", label: "" }] })}
          >
            <Plus className="size-4" />
            {t("settings.svc.add")}
          </Button>
        </div>
        {services.length === 0 && (
          <p className="text-xs text-muted-foreground">{t("settings.svc.empty")}</p>
        )}
        {services.map((service, idx) => (
          <div key={idx} className="flex flex-wrap items-center gap-2">
            <Select
              value={service.icon}
              onValueChange={(value) =>
                patch({ services: services.map((it, i) => (i === idx ? { ...it, icon: value } : it)) })
              }
            >
              <SelectTrigger className="h-9 w-[11.5rem]" aria-label={t("settings.svc.icon")}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {PORTAL_SERVICE_ICONS.map((icon) => {
                  const Icon = PORTAL_SERVICE_LUCIDE[icon] ?? ListChecks;
                  return (
                    <SelectItem key={icon} value={icon}>
                      <span className="inline-flex items-center gap-2">
                        <Icon className="size-4 text-primary" aria-hidden="true" />
                        {t(`settings.svc.icon.${icon.slice(3)}`)}
                      </span>
                    </SelectItem>
                  );
                })}
              </SelectContent>
            </Select>
            <Input
              className="h-9 min-w-0 flex-1"
              maxLength={60}
              placeholder={t("settings.svc.name")}
              value={service.label}
              onChange={(event) =>
                patch({
                  services: services.map((it, i) => (i === idx ? { ...it, label: event.target.value } : it)),
                })
              }
            />
            <Button
              type="button"
              variant="ghost"
              size="sm"
              className="text-destructive hover:text-destructive"
              onClick={() => patch({ services: services.filter((_, i) => i !== idx) })}
            >
              <X className="size-4" />
            </Button>
          </div>
        ))}
      </div>
      <p className="text-xs text-muted-foreground">{t("settings.svc.hint")}</p>
    </section>
  );
}

// PORTAL_SERVICE_LUCIDE — N°137 — équivalents lucide des icônes curées : la
// console n'embarque PAS Font Awesome (c'est le portail qui rend les fa-*),
// elle montre son propre jeu d'icônes pour guider le choix du gérant. La
// valeur persistée reste la classe fa-* (whitelist serveur).
const PORTAL_SERVICE_LUCIDE: Record<string, LucideIcon> = {
  "fa-wifi": Wifi,
  "fa-globe": Globe,
  "fa-laptop": Laptop,
  "fa-tools": Wrench,
  "fa-code": Code,
  "fa-print": Printer,
  "fa-credit-card": CreditCard,
  "fa-money-bill-wave": Banknote,
  "fa-phone": Phone,
  "fa-headset": Headset,
  "fa-gamepad": Gamepad2,
  "fa-mug-hot": Coffee,
  "fa-utensils": Utensils,
  "fa-car": Car,
  "fa-bolt": Zap,
  "fa-store": Store,
  "fa-camera": Camera,
  "fa-scissors": Scissors,
  "fa-book": BookOpen,
  "fa-spa": Sparkles,
};

/** Bandeau animé du portail (N°138) — messages Typed.js sous le logo,
 * ≤ 5 messages de texte brut (80 car.). */
function TickerFields({ form, patch, dirty }: SectionProps) {
  const { t } = useI18n();
  const msgs = form.ticker;

  return (
    <section className="grid gap-3" aria-labelledby="hot-exp-ticker">
      <SubSectionHeader
        id="hot-exp-ticker"
        icon={Megaphone}
        title={t("settings.ticker.card")}
        desc={t("settings.ticker.cardDesc")}
        dirty={dirty}
      />
      <div className="grid gap-2">
        <div className="flex items-center justify-between">
          <Label>{t("settings.ticker.list")}</Label>
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={msgs.length >= 5}
            onClick={() => patch({ ticker: [...msgs, ""] })}
          >
            <Plus className="size-4" />
            {t("settings.ticker.add")}
          </Button>
        </div>
        {msgs.length === 0 && (
          <p className="text-xs text-muted-foreground">{t("settings.ticker.empty")}</p>
        )}
        {msgs.map((msg, idx) => (
          <div key={idx} className="flex items-center gap-2">
            <Input
              className="h-9 min-w-0 flex-1"
              maxLength={80}
              placeholder={t("settings.ticker.name")}
              value={msg}
              onChange={(event) =>
                patch({ ticker: msgs.map((it, i) => (i === idx ? event.target.value : it)) })
              }
            />
            <Button
              type="button"
              variant="ghost"
              size="sm"
              className="text-destructive hover:text-destructive"
              onClick={() => patch({ ticker: msgs.filter((_, i) => i !== idx) })}
            >
              <X className="size-4" />
              <span className="sr-only">{t("settings.slides.remove")}</span>
            </Button>
          </div>
        ))}
      </div>
      <p className="text-xs text-muted-foreground">{t("settings.ticker.hint")}</p>
    </section>
  );
}

/** Support WhatsApp du portail (N°139) — le numéro que les invités
 * cliquent (footer login/logout/error), repli : support MikCloud. */
function WhatsappFields({ form, patch, dirty, invalid }: SectionProps & { invalid: boolean }) {
  const { t } = useI18n();
  // Aperçu du lien réellement servi (chiffres seuls — même normalisation
  // que le serveur).
  const digits = form.waNumber.replace(/[^0-9]/g, "");
  const preview = digits ? `https://wa.me/${digits}` : "https://wa.me/2250150491807";

  return (
    <section className="grid gap-3" aria-labelledby="hot-exp-whatsapp">
      <SubSectionHeader
        id="hot-exp-whatsapp"
        icon={MessageCircle}
        title={t("settings.wa.card")}
        desc={t("settings.wa.cardDesc")}
        dirty={dirty}
      />
      <div className="grid gap-4 sm:grid-cols-2">
        <div className="grid gap-2">
          <Label htmlFor="wa-number">{t("settings.wa.number")}</Label>
          <Input
            id="wa-number"
            className="h-9"
            type="tel"
            inputMode="tel"
            autoComplete="off"
            maxLength={20}
            placeholder={t("settings.wa.numberPh")}
            value={form.waNumber}
            onChange={(event) => patch({ waNumber: event.target.value })}
            aria-invalid={invalid || undefined}
          />
          {invalid && <p className="text-xs text-destructive">{t("settings.exp.waInvalid")}</p>}
        </div>
        <div className="grid gap-2">
          <Label htmlFor="wa-label">{t("settings.wa.label")}</Label>
          <Input
            id="wa-label"
            className="h-9"
            maxLength={30}
            placeholder={t("settings.wa.labelPh")}
            value={form.waLabel}
            onChange={(event) => patch({ waLabel: event.target.value })}
          />
        </div>
      </div>
      <p className="break-all font-mono text-xs text-muted-foreground" data-testid="wa-preview">
        {t("settings.wa.preview")} : {preview}
      </p>
      <p className="text-xs text-muted-foreground">{t("settings.wa.hint")}</p>
    </section>
  );
}

/** Mode d'affichage du portail (N°55) — commercial (grille tarifaire +
 * Wave, défaut) ou hospitalité (vitrine : bienvenue, promos R2, réseaux
 * sociaux). Inclut l'analyse de la vitrine (N°56). */
function HospitalityFields({ form, patch, patchWith, dirty }: SectionProps) {
  const { t } = useI18n();
  const promos = form.promos;
  const socials = form.socials;
  const [uploadingIdx, setUploadingIdx] = useState<number | null>(null);

  // N°56 — analytics de la vitrine : impressions/clics par promo.
  const statsQuery = useQuery({
    queryKey: ["promo-stats"],
    queryFn: () => api<PromoStats>("/api/promos/stats"),
    enabled: form.portalStyle === "hospitality" && promos.length > 0,
    staleTime: 30_000,
  });
  const stats = statsQuery.data;

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
      const data = new FormData();
      data.append("file", file);
      const res = await apiUpload<{ url: string }>("/api/media", data, { timeoutMs: 60_000 });
      patchWith?.((f) => ({
        ...f,
        promos: f.promos.map((it, i) => (i === idx ? { ...it, imageUrl: res.url } : it)),
      }));
      toast.success(t("settings.bannerUploadOk"));
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t("settings.hosp.imageFail"));
    } finally {
      setUploadingIdx(null);
    }
  }

  return (
    <section className="grid gap-3" aria-labelledby="hot-exp-mode">
      <SubSectionHeader
        id="hot-exp-mode"
        icon={Store}
        title={t("settings.hosp.card")}
        desc={t("settings.hosp.cardDesc")}
        dirty={dirty}
      />
      <div className="grid gap-2">
        <Label htmlFor="portal-style">{t("settings.hosp.mode")}</Label>
        <Select
          value={form.portalStyle === "hospitality" ? "hospitality" : "commercial"}
          onValueChange={(value) => patch({ portalStyle: value === "hospitality" ? "hospitality" : "" })}
        >
          <SelectTrigger id="portal-style" className="h-10 sm:w-72">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="commercial">{t("settings.hosp.modeCommercial")}</SelectItem>
            <SelectItem value="hospitality">{t("settings.hosp.modeHospitality")}</SelectItem>
          </SelectContent>
        </Select>
        <p className="text-xs text-muted-foreground">{t("settings.hosp.modeHint")}</p>
      </div>

      {form.portalStyle === "hospitality" && (
        <>
          <div className="grid gap-2">
            <Label htmlFor="portal-welcome">{t("settings.hosp.welcome")}</Label>
            <Textarea
              id="portal-welcome"
              className="min-h-[70px]"
              maxLength={200}
              value={form.welcome}
              onChange={(event) => patch({ welcome: event.target.value })}
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
                onClick={() =>
                  patch({ promos: [...promos, { title: "", desc: "", imageUrl: "", priceLabel: "" }] })
                }
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
                      onClick={() =>
                        patch({ promos: promos.map((it, i) => (i === idx ? { ...it, imageUrl: "" } : it)) })
                      }
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
                    onClick={() => patch({ promos: promos.filter((_, i) => i !== idx) })}
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
                    patch({ promos: promos.map((it, i) => (i === idx ? { ...it, title: event.target.value } : it)) })
                  }
                />
                <Input
                  className="h-9"
                  maxLength={160}
                  placeholder={t("settings.hosp.promoDesc")}
                  value={promo.desc}
                  onChange={(event) =>
                    patch({ promos: promos.map((it, i) => (i === idx ? { ...it, desc: event.target.value } : it)) })
                  }
                />
                <Input
                  className="h-9"
                  maxLength={30}
                  placeholder={t("settings.hosp.promoPrice")}
                  value={promo.priceLabel}
                  onChange={(event) =>
                    patch({
                      promos: promos.map((it, i) => (i === idx ? { ...it, priceLabel: event.target.value } : it)),
                    })
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
                      patch({ promos: promos.map((it, i) => (i === idx ? { ...it, link: event.target.value } : it)) })
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
                  <span className="font-medium text-foreground min-w-0 truncate max-w-full sm:max-w-[16rem]">
                    {p.title || p.id}
                  </span>
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
                onClick={() => patch({ socials: [...socials, { label: "", url: "" }] })}
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
                    patch({
                      socials: socials.map((it, i) => (i === idx ? { ...it, label: event.target.value } : it)),
                    })
                  }
                />
                <Input
                  className="h-9 min-w-0 flex-1"
                  maxLength={200}
                  placeholder="https://…"
                  value={social.url}
                  onChange={(event) =>
                    patch({
                      socials: socials.map((it, i) => (i === idx ? { ...it, url: event.target.value } : it)),
                    })
                  }
                />
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  className="text-destructive hover:text-destructive"
                  onClick={() => patch({ socials: socials.filter((_, i) => i !== idx) })}
                >
                  <X className="size-4" />
                </Button>
              </div>
            ))}
          </div>
        </>
      )}
    </section>
  );
}
