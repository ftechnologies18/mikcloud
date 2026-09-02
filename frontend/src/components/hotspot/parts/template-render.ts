"use client";

// Rendu des modèles de vouchers (F2) : substitution des variables {{…}} dans le
// bodyHtml côté CLIENT (à l'impression / aperçu) + génération du QR code.
// Les gabarits utilisent des styles INLINE : le rendu doit être identique dans
// l'aperçu (fond blanc) et à l'impression (hors app, pas de Tailwind).

import QRCode from "qrcode";
import { formatCurrency, formatDuration , fmtRouterDuration } from "@/lib/hotspot/format";
import type { HotspotUser, Profile, VoucherFormat, VoucherTemplate } from "@/lib/hotspot/types";

/** Contexte de rendu : identité du tenant + correspondance profileId → profil. */
export interface TemplateRenderContext {
  tenantName: string;
  dnsName?: string;
  logoUrl?: string;
  currency: string;
  /** Profils du compte — sert à résoudre validité / quota / durée par voucher. */
  profiles: Profile[];
}

/** Variables disponibles dans le bodyHtml d'un modèle (contrat F2). */
export const TEMPLATE_VARIABLES = [
  "{{username}}",
  "{{password}}",
  "{{#password}}",
  "{{/password}}",
  "{{profile}}",
  "{{validity}}",
  "{{price}}",
  "{{sellingPrice}}",
  "{{dataLimit}}",
  "{{timeLimit}}",
  "{{qrCode}}",
  "{{logo}}",
  "{{hotspotName}}",
  "{{dnsName}}",
  "{{num}}",
  "{{comment}}",
  "{{currency}}",
] as const;

/**
 * isSamePasswordMode — vrai si le voucher a été généré en mode
 * « mot de passe = identifiant » (parité Mikhmon) : le ticket ne doit alors
 * afficher QUE le code, sans ligne mot de passe redondante.
 */
export function isSamePasswordMode(
  voucher: Pick<HotspotUser, "username" | "password">,
): boolean {
  return voucher.password.length > 0 && voucher.password === voucher.username;
}

/** Bloc conditionnel {{#password}}…{{/password}} — retiré en mode « même mot de
 * passe », déballé (contenu conservé) sinon. */
const PASSWORD_BLOCK_RE = /\{\{#password\}\}([\s\S]*?)\{\{\/password\}\}/g;
// Gabarits hérités des presets d'origine (non réédités, sans bloc conditionnel) :
// la ligne mot de passe exacte est retirée en mode « même mot de passe ».
const LEGACY_PASSWORD_LINE_RES = [
  /<p[^>]*>\s*Mot de passe\s*:\s*\{\{password\}\}\s*<\/p>/gi,
  /<p[^>]*>\s*PASS\s*:\s*\{\{password\}\}\s*<\/p>/gi,
];

// Gabarits personnalisés hérités (ex. « Grille A4 » enregistrée avant F2) :
// étiquette « MOT DE PASSE » et valeur portées par des <div> séparés (ou
// étiquette+valeur dans le même élément). En mode « même mot de passe » le
// code figure déjà via {{username}} — tout élément ne portant QUE le mot de
// passe est retiré pour n'afficher qu'un seul élément sur le ticket.
// Ordre important : les paires étiquette+valeur d'abord, la valeur nue ensuite
// (sinon l'étiquette resterait orpheline).
const LEGACY_PASSWORD_ELEMENT_RES = [
  // Paire : <div …>MOT DE PASSE</div>  <div …>{{password}}</div>
  /<(div|p|span)[^>]*>\s*\b(?:mot\s*de\s*passe|password|mdp|pass)\b\s*:?\s*<\/\1>\s*<(div|p)[^>]*>\s*\{\{password\}\}\s*<\/\2>/gi,
  // Étiquette et valeur dans le MÊME élément : <p …>Mot de passe : {{password}}</p>
  /<(div|p)[^>]*>[^<]*\b(?:mot\s*de\s*passe|password|mdp|pass)\b[^<]*\{\{password\}\}\s*<\/\1>/gi,
  // Valeur SEULE dans son élément (étiquette déjà retirée ou absente) :
  // <div …>{{password}}</div>
  /<(div|p)[^>]*>\s*\{\{password\}\}\s*<\/\1>/gi,
];

/** Élément (div/p/span) portant SEUL {{username}} — cible du gras mode « même
 * mot de passe » (le code est l'unique identité du ticket, il doit se voir). */
const USERNAME_ELEMENT_RE =
  /<(div|p|span)([^>]*)>(\s*)\{\{username\}\}(\s*)<\/\1>/gi;
/** Attribut style existant déjà en gras (ou équivalent) — ne rien changer. */
const STYLE_BOLD_RE = /font-weight\s*:\s*(bold|bolder|[6-9]00)/i;

/** Fusionne font-weight:bold dans une valeur d'attribut style existante. */
function styleWithBold(existing: string): string {
  return `${existing.trim().replace(/;\s*$/, "")};font-weight:bold;`;
}

/**
 * boldUsernameElement — mode « même mot de passe » : garantit que le code
 * ({{username}} seul dans son élément) s'affiche EN GRAS sur tous les modèles,
 * y compris les gabarits personnalisés qui ne l'étaient pas. Idempotent : un
 * élément déjà gras (font-weight:bold/bolder/600+) reste inchangé.
 */
function boldUsernameElement(html: string): string {
  return html.replace(
    USERNAME_ELEMENT_RE,
    (_m, tag: string, attrs: string, pre: string, post: string) => {
      if (STYLE_BOLD_RE.test(attrs)) {
        return `<${tag}${attrs}>${pre}{{username}}${post}</${tag}>`;
      }
      let newAttrs = attrs;
      const dq = /style\s*=\s*"([^"]*)"/i.exec(attrs);
      if (dq) {
        newAttrs = attrs.replace(dq[0], `style="${styleWithBold(dq[1])}"`);
      } else {
        const sq = /style\s*=\s*'([^']*)'/i.exec(attrs);
        if (sq) newAttrs = attrs.replace(sq[0], `style='${styleWithBold(sq[1])}'`);
        else newAttrs = `${attrs} style="font-weight:bold;"`;
      }
      return `<${tag}${newAttrs}>${pre}{{username}}${post}</${tag}>`;
    },
  );
}

/** GIF 1px transparent — placeholder {{logo}} quand aucun logo n'est configuré. */
const TRANSPARENT_PIXEL =
  "data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7";

// ─── QR code + logo du tenant au centre ───────────────────────────────────
// Le logo du client (settings.tenant.logoUrl, importé dans Paramètres) est
// composé AU CENTRE du QR, comme sur les cartes prépayées du commerce.
// Le QR passe en niveau H (30 % de redondance) pour rester parfaitement
// lisible malgré le badge central (scannabilité prouvée par décodage).

/** Taille du bitmap QR généré (px) — suréchantillonné pour l'impression ~300 dpi. */
const QR_SIZE_PX = 300;
/** Diamètre du badge blanc central, en fraction du côté du QR (≤ ~0,35 : sécurité niveau H). */
const QR_BADGE_RATIO = 0.32;
/** Fraction du badge occupée par le logo (aspect préservé, jamais en contact avec les modules). */
const QR_LOGO_FIT = 0.68;

/** Cache des logos décodés (src → Image) — un seul décodage par logo et par lot. */
const logoImageCache = new Map<string, HTMLImageElement>();

function loadImage(src: string): Promise<HTMLImageElement> {
  const cached = logoImageCache.get(src);
  if (cached) return Promise.resolve(cached);
  return new Promise((resolve, reject) => {
    const img = new Image();
    img.onload = () => {
      // Garde-fou mémoire : au-delà de 8 logos décodés, on repart de zéro.
      if (logoImageCache.size >= 8) logoImageCache.clear();
      logoImageCache.set(src, img);
      resolve(img);
    };
    img.onerror = () => reject(new Error("logo-load-failed"));
    img.src = src;
  });
}

/**
 * qrWithLogoDataUrl — QR (data URL PNG) avec le logo du tenant composé au
 * centre sur un badge blanc circulaire. Fallback gracieux : QR nu si canvas
 * indisponible (SSR/tests) ou image illisible.
 * Réutilisée par Paramètres pour l'aperçu live « logo au centre du QR ».
 *
 * Rendu MANUEL des modules (QRCode.create + fillRect à échelle entière) :
 * le renderer natif à largeur imposée produit des modules fractionnaires
 * (ex. 300/29 ≈ 10,3 px) baveux, qui dégradent la lecture — l'échelle
 * entière garantit des modules nets, indispensables à l'impression.
 */
export async function qrWithLogoDataUrl(
  payload: string,
  logoUrl?: string,
): Promise<string> {
  const qr = QRCode.create(payload, { errorCorrectionLevel: "H" });
  const cells = qr.modules.size;
  // Échelle entière visant ~300 px (min 4 px/module pour les longs payloads).
  const scale = Math.max(4, Math.floor(QR_SIZE_PX / cells));
  const size = cells * scale;

  /** QR nu de secours (rendu net, échelle entière, sans logo). */
  const plainQr = () =>
    QRCode.toDataURL(payload, { errorCorrectionLevel: "H", margin: 0, scale });

  if (!logoUrl || typeof document === "undefined") return plainQr();
  try {
    const logo = await loadImage(logoUrl);
    const canvas = document.createElement("canvas");
    canvas.width = size;
    canvas.height = size;
    const g = canvas.getContext("2d");
    if (!g) return plainQr();
    g.fillStyle = "#ffffff";
    g.fillRect(0, 0, size, size);
    g.fillStyle = "#000000";
    for (let row = 0; row < cells; row++) {
      for (let col = 0; col < cells; col++) {
        if (qr.modules.get(row, col)) {
          g.fillRect(col * scale, row * scale, scale, scale);
        }
      }
    }
    // Badge blanc circulaire : isole le logo des modules environnants.
    const cx = size / 2;
    const badge = size * QR_BADGE_RATIO;
    g.beginPath();
    g.arc(cx, cx, badge / 2, 0, Math.PI * 2);
    g.fillStyle = "#ffffff";
    g.fill();
    // Logo ajusté (proportions préservées) ; un logo carré reste dans le
    // cercle : 0.68 × √2 ≈ 0.96 < 1.
    const avail = badge * QR_LOGO_FIT;
    const imgScale = Math.min(avail / logo.naturalWidth, avail / logo.naturalHeight);
    const w = logo.naturalWidth * imgScale;
    const h = logo.naturalHeight * imgScale;
    g.imageSmoothingEnabled = true;
    g.imageSmoothingQuality = "high";
    g.drawImage(logo, cx - w / 2, cx - h / 2, w, h);
    return canvas.toDataURL("image/png");
  } catch {
    return plainQr();
  }
}

function escapeHtml(value: string): string {
  return value
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}

function replaceAllLiteral(html: string, token: string, value: string): string {
  // replaceAll avec des chaînes : remplacement littéral (pas de motifs $&…).
  return html.replaceAll(token, value);
}

/**
 * renderTemplate — remplace les variables du bodyHtml pour UN voucher.
 * Asynchrone : le QR code (data URL) est généré via la lib `qrcode`.
 * {{price}} affiche le prix de vente (sellingPrice || price, contrat F13).
 */
export async function renderTemplate(
  bodyHtml: string,
  voucher: HotspotUser,
  ctx: TemplateRenderContext,
  num = 1,
): Promise<string> {
  const profile = ctx.profiles.find((p) => p.id === voucher.profileId);
  // QR avec le logo du tenant au centre (badge blanc, niveau H) — voir
  // qrWithLogoDataUrl : fallback QR nu si aucun logo ou canvas indisponible.
  const qrCode = await qrWithLogoDataUrl(
    `${voucher.username}\n${voucher.password}`,
    ctx.logoUrl,
  );

  // Parité Mikhmon : validité affichée au format RouterOS [wdhm] (5h30m, 30d…).
  const validityMin = profile ? (profile.validityMin > 0 ? profile.validityMin : profile.validityDays * 1440) : 0;
  const selling = voucher.sellingPrice || voucher.price;

  let html = bodyHtml;
  // Mode « mot de passe = identifiant » (parité Mikhmon) : les blocs
  // {{#password}}…{{/password}} disparaissent du ticket ; sinon ils sont
  // déballés. Les gabarits hérités sans bloc conditionnel perdent leur ligne
  // mot de passe exacte (presets d'origine), puis tout élément ne portant
  // QUE le mot de passe (étiquette+valeur séparés, valeur nue) est retiré —
  // le code étant déjà affiché via {{username}}, le ticket n'a qu'un seul
  // élément. Garde-fou : sans {{username}} dans le gabarit, {{password}}
  // serait la seule occurrence du code — gabarit laissé intact.
  const sameMode = isSamePasswordMode(voucher);
  html = html.replace(PASSWORD_BLOCK_RE, sameMode ? "" : "$1");
  if (sameMode) {
    for (const legacyRe of LEGACY_PASSWORD_LINE_RES) {
      html = html.replace(legacyRe, "");
    }
    if (/\{\{username\}\}/.test(html)) {
      for (const elementRe of LEGACY_PASSWORD_ELEMENT_RES) {
        html = html.replace(elementRe, "");
      }
      // Le code étant l'unique identité du ticket, il s'affiche EN GRAS sur
      // tous les modèles (idempotent — les gabarits déjà gras sont inchangés).
      html = boldUsernameElement(html);
    }
  }
  html = replaceAllLiteral(html, "{{username}}", escapeHtml(voucher.username));
  html = replaceAllLiteral(html, "{{password}}", escapeHtml(voucher.password));
  html = replaceAllLiteral(html, "{{profile}}", escapeHtml(voucher.profileName));
  html = replaceAllLiteral(html, "{{validity}}", fmtRouterDuration(validityMin));
  html = replaceAllLiteral(html, "{{price}}", escapeHtml(formatCurrency(selling, ctx.currency)));
  html = replaceAllLiteral(
    html,
    "{{sellingPrice}}",
    escapeHtml(formatCurrency(voucher.sellingPrice || selling, ctx.currency)),
  );
  html = replaceAllLiteral(
    html,
    "{{dataLimit}}",
    !profile || profile.dataQuotaMb === 0 ? "Illimité" : `${profile.dataQuotaMb} Mo`,
  );
  // Parité Mikhmon : le quota temps du voucher (limit-uptime du lot) prime,
  // sinon héritage du session-timeout du profil.
  const effectiveTimeLimitMin =
    voucher.timeLimitMin > 0
      ? voucher.timeLimitMin
      : (profile?.sessionTimeoutMin ?? 0);
  html = replaceAllLiteral(
    html,
    "{{timeLimit}}",
    effectiveTimeLimitMin > 0 ? formatDuration(effectiveTimeLimitMin * 60) : "—",
  );
  // Data URL : base64 sans caractères spéciaux HTML — injectable telle quelle dans src="".
  html = replaceAllLiteral(html, "{{qrCode}}", qrCode);
  html = replaceAllLiteral(html, "{{logo}}", ctx.logoUrl || TRANSPARENT_PIXEL);
  html = replaceAllLiteral(html, "{{hotspotName}}", escapeHtml(ctx.tenantName || "MikCloud"));
  html = replaceAllLiteral(html, "{{dnsName}}", escapeHtml(ctx.dnsName ?? ""));
  html = replaceAllLiteral(html, "{{num}}", String(num));
  html = replaceAllLiteral(html, "{{comment}}", escapeHtml(voucher.comment ?? ""));
  html = replaceAllLiteral(html, "{{currency}}", escapeHtml(ctx.currency));
  return html;
}

/**
 * a4GridPlan — plan ADAPTATIF de la grille A4 : au-delà de 24 tickets, la
 * grille passe à 5 colonnes avec un contenu réduit (×0,75) — chaque feuille
 * A4 accueille alors ~25 à 35 tickets (5×5 à 5×7 selon la hauteur réelle des
 * tickets). PLAFOND VOLONTAIRE (consigne client) : jamais de grille plus
 * dense — les lots > 35 sont simplement paginés sur les feuilles suivantes,
 * à taille constante. ESPACEMENTS SERRÉS (3 mm / 2 mm, consigne client :
 * « réduis les espacements pour augmenter la taille de chaque ticket ») —
 * la place gagnée agrandit les cellules : tickets plus grands, texte et QR
 * plus lisibles.
 */
export function a4GridPlan(count: number): {
  cols: number;
  zoom: number;
  gapMm: number;
} {
  if (count <= 12) return { cols: 3, zoom: 1, gapMm: 3 };
  if (count <= 24) return { cols: 4, zoom: 0.75, gapMm: 3 };
  // Lots 25-35 (et au-delà, paginés) : 5 colonnes, réduction ×0,75,
  // espacement 2 mm — plafond ~35 tickets par feuille A4.
  return { cols: 5, zoom: 0.75, gapMm: 2 };
}

/**
 * renderBatch — page imprimable complète pour un lot de vouchers :
 * - a4    : grille ADAPTATIVE (3→5 colonnes selon la quantité, plafond
 *   ~35 tickets/feuille, pagination au-delà — voir a4GridPlan) ;
 * - 58/80 : tickets verticaux séquentiels (largeur fixe 54/76 mm,
 *   saut de page géré par les règles .tpl-* de globals.css).
 */
export async function renderBatch(
  template: Pick<VoucherTemplate, "bodyHtml" | "format">,
  vouchers: HotspotUser[],
  ctx: TemplateRenderContext,
): Promise<string> {
  const tickets = await Promise.all(
    vouchers.map((voucher, index) => renderTemplate(template.bodyHtml, voucher, ctx, index + 1)),
  );

  if (template.format === "a4") {
    const plan = a4GridPlan(vouchers.length);
    const ticketStyle = plan.zoom < 1 ? ` style="zoom:${plan.zoom};"` : "";
    const inner = tickets
      .map((ticket) => `<div class="tpl-ticket"${ticketStyle}>${ticket}</div>`)
      .join("");
    // minmax(0,1fr) : les pistes ne peuvent pas dépasser 1/N de la feuille,
    // même si un gabarit contient du contenu à largeur figée (QR + textes).
    return `<div class="tpl-batch tpl-format-a4" style="display:grid;grid-template-columns:repeat(${plan.cols},minmax(0,1fr));gap:${plan.gapMm}mm;">${inner}</div>`;
  }

  const width = template.format === "58mm" ? "54mm" : "76mm";
  const inner = tickets
    .map((ticket) => `<div class="tpl-ticket" style="width:${width};">${ticket}</div>`)
    .join("");
  return `<div class="tpl-batch tpl-format-${template.format}">${inner}</div>`;
}

// ─── Presets proposés à la création d'un modèle (gabarits Mikhmon adaptés) ───

export interface TemplatePreset {
  id: VoucherFormat;
  label: string;
  name: string;
  format: VoucherFormat;
  bodyHtml: string;
}

export const TEMPLATE_PRESETS: TemplatePreset[] = [
  {
    id: "a4",
    label: "Grille A4",
    name: "Grille A4",
    format: "a4",
    bodyHtml: `<div style="border:2px dashed #16a34a;border-radius:10px;padding:12px 10px;text-align:center;font-family:Arial,Helvetica,sans-serif;background:#fff;color:#000;">
  <div style="display:flex;align-items:center;justify-content:center;gap:6px;">
    <img src="{{logo}}" alt="" style="height:20px;width:auto;">
    <span style="font-size:15px;font-weight:bold;">{{hotspotName}}</span>
  </div>
  <p style="margin:2px 0 8px;font-size:9px;letter-spacing:2px;color:#16a34a;">WIFI HOTSPOT</p>
  <img src="{{qrCode}}" alt="QR code" style="display:block;margin:0 auto 8px;width:84px;height:84px;">
  <p style="margin:0;font-size:19px;font-weight:bold;font-family:'Courier New',monospace;letter-spacing:1px;">{{username}}</p>
  {{#password}}<p style="margin:3px 0 8px;font-size:12px;font-family:'Courier New',monospace;">Mot de passe : {{password}}</p>{{/password}}
  <p style="margin:0 0 2px;font-size:12px;color:#1f2937;">{{profile}} · {{validity}}</p>
  <p style="margin:0 0 6px;font-size:14px;font-weight:bold;color:#047857;">{{price}}</p>
  <p style="margin:0;font-size:9px;color:#4b5563;">{{dnsName}}</p>
  <p style="margin:8px 0 0;padding-top:5px;border-top:1px solid #ccc;font-size:9px;color:#6b7280;">N° {{num}} — Gardez ce ticket pour vous connecter</p>
</div>`,
  },
  {
    id: "58mm",
    label: "Ticket 58 mm",
    name: "Ticket thermique 58 mm",
    format: "58mm",
    bodyHtml: `<div style="padding:6px 2px;text-align:center;font-family:Arial,Helvetica,sans-serif;background:#fff;color:#000;">
  <p style="margin:0;font-size:12px;font-weight:bold;">{{hotspotName}}</p>
  <p style="margin:1px 0 6px;font-size:8px;letter-spacing:1px;color:#16a34a;">WIFI HOTSPOT · {{dnsName}}</p>
  <img src="{{qrCode}}" alt="QR code" style="display:block;margin:0 auto 6px;width:70px;height:70px;">
  <p style="margin:0;font-size:14px;font-weight:bold;font-family:'Courier New',monospace;">{{username}}</p>
  {{#password}}<p style="margin:1px 0 5px;font-size:11px;font-family:'Courier New',monospace;">PASS : {{password}}</p>{{/password}}
  <p style="margin:0 0 3px;font-size:11px;color:#1f2937;">{{profile}} · {{validity}}</p>
  <p style="margin:0 0 4px;font-size:12px;font-weight:bold;color:#047857;">{{price}}</p>
  <p style="margin:6px 0 0;border-top:1px dashed #999;padding-top:4px;font-size:9px;color:#6b7280;">N° {{num}} — Gardez ce ticket</p>
</div>`,
  },
  {
    id: "80mm",
    label: "Ticket 80 mm",
    name: "Ticket thermique 80 mm",
    format: "80mm",
    bodyHtml: `<div style="padding:8px 4px;text-align:center;font-family:Arial,Helvetica,sans-serif;background:#fff;color:#000;">
  <div style="display:flex;align-items:center;justify-content:center;gap:8px;">
    <img src="{{logo}}" alt="" style="height:26px;width:auto;">
    <span style="font-size:17px;font-weight:bold;">{{hotspotName}}</span>
  </div>
  <p style="margin:2px 0 8px;font-size:10px;letter-spacing:2px;color:#16a34a;">WIFI HOTSPOT · {{dnsName}}</p>
  <img src="{{qrCode}}" alt="QR code" style="display:block;margin:0 auto 8px;width:96px;height:96px;">
  <p style="margin:0;font-size:19px;font-weight:bold;font-family:'Courier New',monospace;letter-spacing:1px;">{{username}}</p>
  {{#password}}<p style="margin:3px 0 8px;font-size:14px;font-family:'Courier New',monospace;">Mot de passe : {{password}}</p>{{/password}}
  <p style="margin:0 0 3px;font-size:13px;color:#1f2937;">{{profile}} · {{validity}} · {{timeLimit}}</p>
  <p style="margin:0 0 6px;font-size:15px;font-weight:bold;color:#047857;">{{price}}</p>
  <p style="margin:8px 0 0;border-top:1px dashed #999;padding-top:5px;font-size:9px;color:#6b7280;">N° {{num}} · {{comment}} — Gardez ce ticket pour vous connecter</p>
</div>`,
  },
];
