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
  // mot de passe exacte (presets d'origine).
  const sameMode = isSamePasswordMode(voucher);
  html = html.replace(PASSWORD_BLOCK_RE, sameMode ? "" : "$1");
  if (sameMode) {
    for (const legacyRe of LEGACY_PASSWORD_LINE_RES) {
      html = html.replace(legacyRe, "");
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
 * renderBatch — page imprimable complète pour un lot de vouchers :
 * - a4    : grille 3 colonnes avec espacement ;
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
    const inner = tickets.map((ticket) => `<div class="tpl-ticket">${ticket}</div>`).join("");
    // minmax(0,1fr) : les pistes ne peuvent pas dépasser 1/3 de la feuille,
    // même si un gabarit contient du contenu à largeur figée (QR + textes).
    return `<div class="tpl-batch tpl-format-a4" style="display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:5mm;">${inner}</div>`;
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
    bodyHtml: `<div style="border:2px dashed #000;border-radius:10px;padding:12px 10px;text-align:center;font-family:Arial,Helvetica,sans-serif;background:#fff;color:#000;">
  <div style="display:flex;align-items:center;justify-content:center;gap:6px;">
    <img src="{{logo}}" alt="" style="height:20px;width:auto;">
    <span style="font-size:14px;font-weight:bold;">{{hotspotName}}</span>
  </div>
  <p style="margin:2px 0 8px;font-size:9px;letter-spacing:2px;color:#555;">WIFI HOTSPOT</p>
  <img src="{{qrCode}}" alt="QR code" style="display:block;margin:0 auto 8px;width:84px;height:84px;">
  <p style="margin:0;font-size:17px;font-weight:bold;font-family:'Courier New',monospace;letter-spacing:1px;">{{username}}</p>
  {{#password}}<p style="margin:3px 0 8px;font-size:12px;font-family:'Courier New',monospace;">Mot de passe : {{password}}</p>{{/password}}
  <p style="margin:0 0 2px;font-size:11px;color:#222;">{{profile}} · {{validity}}</p>
  <p style="margin:0 0 6px;font-size:13px;font-weight:bold;">{{price}}</p>
  <p style="margin:0;font-size:9px;color:#666;">{{dnsName}}</p>
  <p style="margin:8px 0 0;padding-top:5px;border-top:1px solid #ccc;font-size:8px;color:#888;">N° {{num}} — Gardez ce ticket pour vous connecter</p>
</div>`,
  },
  {
    id: "58mm",
    label: "Ticket 58 mm",
    name: "Ticket thermique 58 mm",
    format: "58mm",
    bodyHtml: `<div style="padding:6px 2px;text-align:center;font-family:Arial,Helvetica,sans-serif;background:#fff;color:#000;">
  <p style="margin:0;font-size:12px;font-weight:bold;">{{hotspotName}}</p>
  <p style="margin:1px 0 6px;font-size:8px;letter-spacing:1px;color:#555;">WIFI HOTSPOT · {{dnsName}}</p>
  <img src="{{qrCode}}" alt="QR code" style="display:block;margin:0 auto 6px;width:70px;height:70px;">
  <p style="margin:0;font-size:13px;font-weight:bold;font-family:'Courier New',monospace;">{{username}}</p>
  {{#password}}<p style="margin:1px 0 5px;font-size:11px;font-family:'Courier New',monospace;">PASS : {{password}}</p>{{/password}}
  <p style="margin:0 0 3px;font-size:10px;">{{profile}} · {{validity}}</p>
  <p style="margin:0 0 4px;font-size:12px;font-weight:bold;">{{price}}</p>
  <p style="margin:6px 0 0;border-top:1px dashed #999;padding-top:4px;font-size:8px;color:#666;">N° {{num}} — Gardez ce ticket</p>
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
  <p style="margin:2px 0 8px;font-size:10px;letter-spacing:2px;color:#555;">WIFI HOTSPOT · {{dnsName}}</p>
  <img src="{{qrCode}}" alt="QR code" style="display:block;margin:0 auto 8px;width:96px;height:96px;">
  <p style="margin:0;font-size:18px;font-weight:bold;font-family:'Courier New',monospace;letter-spacing:1px;">{{username}}</p>
  {{#password}}<p style="margin:3px 0 8px;font-size:14px;font-family:'Courier New',monospace;">Mot de passe : {{password}}</p>{{/password}}
  <p style="margin:0 0 3px;font-size:12px;color:#222;">{{profile}} · {{validity}} · {{timeLimit}}</p>
  <p style="margin:0 0 6px;font-size:15px;font-weight:bold;">{{price}}</p>
  <p style="margin:8px 0 0;border-top:1px dashed #999;padding-top:5px;font-size:9px;color:#666;">N° {{num}} · {{comment}} — Gardez ce ticket pour vous connecter</p>
</div>`,
  },
];
