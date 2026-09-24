// N°186 — domaine PARTAGÉ du branding du portail captif : la forme d'édition
// (13 champs), les décodeurs défensifs, la résolution de la chaîne
// ROUTEUR → SITE → COMPTE et la construction du corps de surcharge.
//
// Miroir EXACT de la sémantique backend (internal/api/portal_branding.go et
// resolvePortalBranding/applyPortalOverride de portal_serve.go) :
//   • « VIDE = HÉRITE » — un champ vide ne touche rien, une liste vide non
//     plus, un numéro WhatsApp vide non plus ;
//   • la surcharge est REMPLACÉE EN ENTIER à chaque écriture — l'éditeur
//     envoie donc toujours l'état complet souhaité (groupes réinitialisés =
//     champs omis du corps) ;
//   • les bornes de validation sont partagées avec les réglages du compte.
//
// Consommé par le formulaire du compte (hotspot-cards — HotspotForm, via les
// décodeurs et la forme structurelle PortalBrandingFields) et par l'éditeur
// unifié à sélecteur de contexte N°186 (portal-editor — surcharges
// SITE/ROUTEUR) : les MÊMES règles de lecture et d'écriture aux trois
// niveaux, zéro divergence console ↔ serveur.

import type {
  AppSettings,
  PortalOverrideData,
  PortalPromo,
  PortalService,
  PortalSocial,
} from "@/lib/hotspot/types";

/* ─── Formes d'édition ─── */

/** PortalBrandingFields — les 10 champs de branding édités par les briques
 * PARTAGÉES du formulaire riche (bannière, carrousel, services, bandeau,
 * WhatsApp, mode d'affichage). Forme STRUCTURELLE : le HotspotForm du compte
 * (hotspot-cards) et le PortalOverrideForm de l'éditeur N°186 la satisfont
 * tous deux — les briques se rendent ainsi À L'IDENTIQUE aux trois niveaux
 * de la chaîne. */
export interface PortalBrandingFields {
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

/** PortalOverrideForm — TOUT l'état éditable d'une personnalisation de
 * portail : les 10 champs de branding + l'identité propre au niveau (nom
 * affiché, logo, lien marchand Wave). Vide = hérite. */
export interface PortalOverrideForm extends PortalBrandingFields {
  displayName: string;
  logoUrl: string;
  waveLink: string;
}

/* ─── Groupes de l'éditeur unifié ─── */

/** Groupes pédagogiques de l'éditeur : un groupe hérite ou est personnalisé
 * EN BLOC. « personnalisé » = au moins un de ses champs porteur de valeur
 * (même prédicat que la sémantique « vide = hérite » du backend). */
export type OverrideGroupKey =
  | "identity"
  | "banner"
  | "slides"
  | "services"
  | "ticker"
  | "whatsapp"
  | "display";

export const PORTAL_OVERRIDE_GROUPS: readonly OverrideGroupKey[] = [
  "identity",
  "banner",
  "slides",
  "services",
  "ticker",
  "whatsapp",
  "display",
];

/* ─── Décodeurs défensifs (JSON invalide ou absent = valeur neutre) ─── */
/* Déplacés de hotspot-cards (N°186) : ils servent désormais AUSSI à décoder
 * les valeurs héritées du compte dans l'éditeur de surcharge. Comportement
 * conservé À L'IDENTIQUE. */

export function parseStringArray(raw: string | undefined): string[] {
  try {
    const parsed = JSON.parse(raw || "[]") as string[];
    return Array.isArray(parsed) ? parsed.filter((u) => typeof u === "string" && u) : [];
  } catch {
    return [];
  }
}

export function parseServices(raw: string | undefined): PortalService[] {
  try {
    const parsed = JSON.parse(raw || "[]") as PortalService[];
    return Array.isArray(parsed) ? parsed : [];
  } catch {
    return [];
  }
}

export function parseWhatsapp(raw: string | undefined): { number: string; label: string } {
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

export function parsePromos(raw: string | undefined): PortalPromo[] {
  try {
    const parsed = JSON.parse(raw || "[]") as PortalPromo[];
    return Array.isArray(parsed) ? parsed : [];
  } catch {
    return [];
  }
}

export function parseSocials(raw: string | undefined): PortalSocial[] {
  try {
    const parsed = JSON.parse(raw || "[]") as PortalSocial[];
    return Array.isArray(parsed) ? parsed : [];
  } catch {
    return [];
  }
}

/* ─── Résolution de la chaîne (miroir de resolvePortalBranding) ─── */

/** emptyBrandingForm — forme vierge : tout hérite, rien n'est personnalisé. */
export function emptyBrandingForm(): PortalOverrideForm {
  return {
    displayName: "",
    logoUrl: "",
    bannerUrl: "",
    waveLink: "",
    portalStyle: "",
    welcome: "",
    slides: [],
    services: [],
    ticker: [],
    promos: [],
    socials: [],
    waNumber: "",
    waLabel: "",
  };
}

/** accountBrandingOf — le branding EFFECTIF du compte (base de la chaîne) :
 * les réglages du tenant décodés dans la forme d'édition. Même lecture
 * défensive que l'ancien initialForm de hotspot-cards (champ absent du JSON =
 * défaut effectif, pas « vide »). */
export function accountBrandingOf(settings: AppSettings): PortalOverrideForm {
  const wa = parseWhatsapp(settings.tenant.portalWhatsapp);
  return {
    displayName: settings.tenant.name ?? "",
    logoUrl: settings.tenant.logoUrl ?? "",
    bannerUrl: settings.tenant.bannerUrl ?? "",
    waveLink: settings.tenant.waveLink ?? "",
    portalStyle: settings.tenant.portalStyle ?? "",
    welcome: settings.tenant.portalWelcome ?? "",
    slides: parseStringArray(settings.tenant.portalSlides),
    services: parseServices(settings.tenant.portalServices),
    ticker: parseStringArray(settings.tenant.portalTicker),
    promos: parsePromos(settings.tenant.portalPromos),
    socials: parseSocials(settings.tenant.portalSocials),
    waNumber: wa.number,
    waLabel: wa.label,
  };
}

/** applyOverrideForm — applique une surcharge décodée sur un branding de
 * base, champ par champ : seules les valeurs NON VIDES gagnent (miroir
 * exact d'applyPortalOverride — la surcharge ne peut pas masquer un élément
 * que le niveau inférieur affiche). */
export function applyOverrideForm(
  base: PortalOverrideForm,
  ov: PortalOverrideData,
): PortalOverrideForm {
  return {
    ...base,
    displayName: ov.displayName ? ov.displayName : base.displayName,
    logoUrl: ov.logoUrl ? ov.logoUrl : base.logoUrl,
    bannerUrl: ov.bannerUrl ? ov.bannerUrl : base.bannerUrl,
    waveLink: ov.waveLink ? ov.waveLink : base.waveLink,
    portalStyle: ov.portalStyle ? ov.portalStyle : base.portalStyle,
    welcome: ov.portalWelcome ? ov.portalWelcome : base.welcome,
    slides: ov.portalSlides && ov.portalSlides.length > 0 ? [...ov.portalSlides] : base.slides,
    services:
      ov.portalServices && ov.portalServices.length > 0
        ? ov.portalServices.map((s) => ({ icon: s.icon ?? "fa-wifi", label: s.label }))
        : base.services,
    ticker: ov.portalTicker && ov.portalTicker.length > 0 ? [...ov.portalTicker] : base.ticker,
    promos:
      ov.portalPromos && ov.portalPromos.length > 0
        ? ov.portalPromos.map((p) => ({
            id: p.id,
            title: p.title,
            desc: p.desc ?? "",
            imageUrl: p.imageUrl ?? "",
            priceLabel: p.priceLabel ?? "",
            link: p.link,
          }))
        : base.promos,
    socials:
      ov.portalSocials && ov.portalSocials.length > 0
        ? ov.portalSocials.map((s) => ({ label: s.label, url: s.url }))
        : base.socials,
    waNumber: ov.portalWhatsapp?.number ? ov.portalWhatsapp.number : base.waNumber,
    waLabel: ov.portalWhatsapp?.number ? (ov.portalWhatsapp.label ?? "") : base.waLabel,
  };
}

/** formFromOverride — état initial de l'éditeur pour une surcharge
 * persistée (JSON canonique) : les champs absents restent vides (= hérite). */
export function formFromOverride(ov: PortalOverrideData): PortalOverrideForm {
  return applyOverrideForm(emptyBrandingForm(), ov);
}

/* ─── Prédicats par groupe ─── */

/** groupIsSet — le groupe porte-t-il une personnalification ? Même
 * sémantique que le backend : au moins un champ porteur de valeur (compte
 * tenu des trim/normalisations d'encodage). */
export function groupIsSet(f: PortalOverrideForm, key: OverrideGroupKey): boolean {
  switch (key) {
    case "identity":
      return f.displayName.trim() !== "" || f.logoUrl.trim() !== "" || f.waveLink.trim() !== "";
    case "banner":
      return f.bannerUrl.trim() !== "";
    case "slides":
      return f.slides.some((u) => u.trim() !== "");
    case "services":
      return f.services.some((s) => s.label.trim() !== "");
    case "ticker":
      return f.ticker.some((m) => m.trim() !== "");
    case "whatsapp":
      return f.waNumber.replace(/[^0-9]/g, "") !== "";
    case "display":
      return (
        f.portalStyle !== "" ||
        f.welcome.trim() !== "" ||
        f.promos.some((p) => p.title.trim() !== "") ||
        f.socials.some((s) => s.label.trim() !== "" || s.url.trim() !== "")
      );
  }
}

const sameJson = (a: unknown, b: unknown) => JSON.stringify(a) === JSON.stringify(b);

/** groupEquals — compare les champs D'UN groupe entre deux états (compteur
 * « modifié » et barre d'action N°140 de l'éditeur). */
export function groupEquals(a: PortalOverrideForm, b: PortalOverrideForm, key: OverrideGroupKey): boolean {
  switch (key) {
    case "identity":
      return a.displayName === b.displayName && a.logoUrl === b.logoUrl && a.waveLink === b.waveLink;
    case "banner":
      return a.bannerUrl === b.bannerUrl;
    case "slides":
      return sameJson(a.slides, b.slides);
    case "services":
      return sameJson(a.services, b.services);
    case "ticker":
      return sameJson(a.ticker, b.ticker);
    case "whatsapp":
      return a.waNumber === b.waNumber && a.waLabel === b.waLabel;
    case "display":
      return (
        a.portalStyle === b.portalStyle &&
        a.welcome === b.welcome &&
        sameJson(a.promos, b.promos) &&
        sameJson(a.socials, b.socials)
      );
  }
}

/* ─── Manipulation de groupes (personnaliser / réinitialiser) ─── */

/** groupValuesOf — les champs D'UN groupe, extraits d'une forme source (pour
 * initialiser la personnalisation depuis la valeur HÉRITÉE : l'utilisateur
 * part de ce qui est servi aujourd'hui et ajuste). */
export function groupValuesOf(f: PortalOverrideForm, key: OverrideGroupKey): Partial<PortalOverrideForm> {
  switch (key) {
    case "identity":
      return { displayName: f.displayName, logoUrl: f.logoUrl, waveLink: f.waveLink };
    case "banner":
      return { bannerUrl: f.bannerUrl };
    case "slides":
      return { slides: [...f.slides] };
    case "services":
      return { services: f.services.map((s) => ({ ...s })) };
    case "ticker":
      return { ticker: [...f.ticker] };
    case "whatsapp":
      return { waNumber: f.waNumber, waLabel: f.waLabel };
    case "display":
      return {
        portalStyle: f.portalStyle,
        welcome: f.welcome,
        promos: f.promos.map((p) => ({ ...p })),
        socials: f.socials.map((s) => ({ ...s })),
      };
  }
}

/** emptyGroupValuesOf — les champs d'un groupe remis à vide (= hérite à
 * nouveau, le groupe disparaît du corps envoyé). */
export function emptyGroupValuesOf(key: OverrideGroupKey): Partial<PortalOverrideForm> {
  switch (key) {
    case "identity":
      return { displayName: "", logoUrl: "", waveLink: "" };
    case "banner":
      return { bannerUrl: "" };
    case "slides":
      return { slides: [] };
    case "services":
      return { services: [] };
    case "ticker":
      return { ticker: [] };
    case "whatsapp":
      return { waNumber: "", waLabel: "" };
    case "display":
      return { portalStyle: "", welcome: "", promos: [], socials: [] };
  }
}

/* ─── Corps de surcharge (contrat PUT /api/sites/{id} et
     PUT /api/routers/{id}/portal — remplacement ENTIER à chaque écriture,
     champs vides omis = hérite) ─── */

/** overridePayloadOf — construit le corps de surcharge correspondant à
 * l'état de l'éditeur : seuls les champs porteurs de valeur partent (miroir
 * enrichi de l'ancien formToPayload : promos, réseaux et icônes de services
 * inclus — la parité de champs du formulaire riche s'applique aux trois
 * niveaux). */
export function overridePayloadOf(f: PortalOverrideForm): Record<string, unknown> {
  const payload: Record<string, unknown> = {};
  const displayName = f.displayName.trim();
  if (displayName) payload.displayName = displayName;
  const logoUrl = f.logoUrl.trim();
  if (logoUrl) payload.logoUrl = logoUrl;
  const bannerUrl = f.bannerUrl.trim();
  if (bannerUrl) payload.bannerUrl = bannerUrl;
  const waveLink = f.waveLink.trim();
  if (waveLink) payload.waveLink = waveLink;
  if (f.portalStyle) payload.portalStyle = f.portalStyle;
  const welcome = f.welcome.trim();
  if (welcome) payload.portalWelcome = welcome;
  const slides = f.slides.map((u) => u.trim()).filter((u) => u !== "");
  if (slides.length > 0) payload.portalSlides = slides;
  const services = f.services.filter((s) => s.label.trim() !== "");
  if (services.length > 0) payload.portalServices = services;
  const ticker = f.ticker.map((m) => m.trim()).filter((m) => m !== "");
  if (ticker.length > 0) payload.portalTicker = ticker;
  const promos = f.promos.filter((p) => p.title.trim() !== "");
  if (promos.length > 0) payload.portalPromos = promos;
  const socials = f.socials.filter((s) => s.label.trim() !== "" || s.url.trim() !== "");
  if (socials.length > 0) payload.portalSocials = socials;
  const waDigits = f.waNumber.replace(/[^0-9]/g, "");
  if (waDigits) payload.portalWhatsapp = { number: f.waNumber.trim(), label: f.waLabel.trim() };
  return payload;
}
