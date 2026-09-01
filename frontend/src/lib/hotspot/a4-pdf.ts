// Générateur PDF A4 des vouchers — approche « document figé » : la mise en page
// est dessinée dans un vrai PDF (jsPDF), plus aucune dépendance au moteur
// d'impression du navigateur (zoom, marges, en-têtes, fragments CSS…).
// Impression : ouvrir le PDF téléchargé et lancer l'impression depuis le
// lecteur PDF — pagination pixel-perfect identique sur tous les postes.
//
// Design : bandeau de marque BLEU MARINE par ticket (logo circulaire · nom du
// tenant · PRIX), QR ORNÉ DU LOGO AU CENTRE (badge circulaire, correction
// d'erreur « H » pour rester scannable), code en gros, MDP, profil · quota,
// aide de scan. L'aperçu écran (voucher-a4-print-dialog) reprend exactement
// les mêmes blocs, aux mêmes endroits.

import QRCode from "qrcode";
import { isSamePasswordMode } from "@/components/hotspot/parts/template-render";
import { formatBytes, formatCurrency } from "./format";
import type { Lang } from "./i18n";
import type { HotspotUser } from "./types";

/** Contenu du QR : URL de login pré-remplie si disponible, sinon « code / mot de passe »
 * (code seul en mode « mot de passe = identifiant » — rien de redondant à scanner).
 * Partagé avec l'aperçu écran (QRCodeSVG) — une seule source de vérité. */
export function qrValue(voucher: HotspotUser, hotspotLoginUrl?: string): string {
  if (hotspotLoginUrl) {
    const sep = hotspotLoginUrl.includes("?") ? "&" : "?";
    return `${hotspotLoginUrl}${sep}username=${encodeURIComponent(voucher.username)}&password=${encodeURIComponent(
      voucher.password,
    )}`;
  }
  return isSamePasswordMode(voucher)
    ? voucher.username
    : `${voucher.username} / ${voucher.password}`;
}

export interface VouchersA4PdfInput {
  vouchers: HotspotUser[];
  title: string;
  tenantName: string;
  /** Page de login MikroTik du hotspot — si fournie, le QR ouvre la session pré-remplie. */
  hotspotLoginUrl?: string;
  currency: string;
  lang: Lang;
  /** Libellés déjà traduits côté UI (le générateur reste sans hooks React). */
  labels: {
    password: string; // « MDP : »
    scanHint: string; // « Scannez le QR ou saisissez le code »
    pageOf: (n: number, total: number) => string; // « page n/total »
  };
}

/* ── Géométrie (mm) — calquée sur l'aperçu écran (voir dialog) ── */
const A4_W = 210;
const A4_H = 297;
const MARGIN = 7.5; // marges de feuille
const HEADER_H = 4.2; // bandeau discret tenant / titre · page
const GAP = 1.5; // gouttière entre tickets
const COLS = 5;
const ROWS = 8;
const QR_MM = 12.5; // QR légèrement agrandi (niveau « H ») — toujours scannable

const CELL_W = (A4_W - 2 * MARGIN - (COLS - 1) * GAP) / COLS;
const GRID_TOP = MARGIN + HEADER_H;
const CELL_H = (A4_H - GRID_TOP - MARGIN - (ROWS - 1) * GAP) / ROWS;

/* ── Couleurs de marque — échantillonnées sur /logo.png ── */
const NAVY: [number, number, number] = [2, 19, 78]; // fond du logo (bandeaux)
const INK: [number, number, number] = [24, 24, 24]; // texte MDP
const MUTED: [number, number, number] = [115, 115, 115]; // profil · quota
const FAINT: [number, number, number] = [163, 163, 163]; // aide de scan
const BORDER: [number, number, number] = [64, 64, 64]; // cartouche pointillé

const LOGO_SRC = "/logo.png";

/* ── Images (navigateur uniquement — appelées au clic « Télécharger le PDF » ;
 * fallback propre hors DOM : QR nu sans logo, géométrie inchangée) ── */

/** Charge /logo.png (logo MikCloud) — null si indisponible. */
async function loadLogo(): Promise<HTMLImageElement | null> {
  if (typeof document === "undefined") return null;
  try {
    return await new Promise<HTMLImageElement>((resolve, reject) => {
      const img = new Image();
      img.onload = () => resolve(img);
      img.onerror = () => reject(new Error("logo load failed"));
      img.src = LOGO_SRC;
    });
  } catch {
    return null;
  }
}

/** Logo recadré en cercle (PNG transparent) — bandeau du ticket + en-tête de
 * page + centre du QR : une seule signature visuelle partout. Exporté pour
 * l'aperçu écran (imageSettings du QRCodeSVG). */
export function circleLogoDataUrl(logo: HTMLImageElement, out = 96): string {
  if (typeof document === "undefined") return "";
  const canvas = document.createElement("canvas");
  canvas.width = out;
  canvas.height = out;
  const ctx = canvas.getContext("2d");
  if (!ctx) return "";
  ctx.beginPath();
  ctx.arc(out / 2, out / 2, out / 2, 0, Math.PI * 2);
  ctx.clip();
  const s = Math.max(out / logo.width, out / logo.height);
  ctx.drawImage(logo, (out - logo.width * s) / 2, (out - logo.height * s) / 2, logo.width * s, logo.height * s);
  return canvas.toDataURL("image/png");
}

/** QR PNG haute résolution (384 px, niveau « H ») avec le logo AU CENTRE :
 * anneau blanc puis logo clippé en cercle (badge = 25 % du QR, sûr avec la
 * correction « H » ≈ 30 % de redondance). */
async function qrWithLogo(value: string, logo: HTMLImageElement | null): Promise<string> {
  const opts = {
    margin: 0,
    errorCorrectionLevel: "H" as const,
    width: 384,
    color: { dark: "#000000", light: "#FFFFFF" },
  };
  if (!logo || typeof document === "undefined") return QRCode.toDataURL(value, opts);

  const size = 384;
  const canvas = document.createElement("canvas");
  canvas.width = size;
  canvas.height = size;
  await QRCode.toCanvas(canvas, value, opts);
  const ctx = canvas.getContext("2d");
  if (!ctx) return canvas.toDataURL("image/png");
  const r = size * 0.125;
  ctx.beginPath();
  ctx.arc(size / 2, size / 2, r + size * 0.014, 0, Math.PI * 2);
  ctx.fillStyle = "#FFFFFF";
  ctx.fill();
  ctx.save();
  ctx.beginPath();
  ctx.arc(size / 2, size / 2, r, 0, Math.PI * 2);
  ctx.clip();
  const s = (r * 2) / Math.min(logo.width, logo.height);
  ctx.drawImage(
    logo,
    size / 2 - (logo.width * s) / 2,
    size / 2 - (logo.height * s) / 2,
    logo.width * s,
    logo.height * s,
  );
  ctx.restore();
  return canvas.toDataURL("image/png");
}

/* ── Texte : jsPDF + polices standards = encodage WinAnsi (latin-1) ── */

/** Normalise pour WinAnsi : NFC (accents composés), ponctuation typographique
 * mappée, tout caractère hors latin-1 remplacé (jamais de « ? » moche). */
function winAnsi(input: string): string {
  return input
    .normalize("NFC")
    .replace(/[\u2018\u2019\u201B]/g, "'")
    .replace(/[\u201C\u201D]/g, '"')
    .replace(/[\u2013\u2014]/g, "-")
    .replace(/\u2026/g, "...")
    .replace(/\u00A0|\u202F|\u2009/g, " ")
    .replace(/[^\x20-\x7E\u00A1-\u00FF]/g, "-");
}

/** Tronque au maxWidth (getTextWidth reflète la police/taille courantes). */
function fitText(doc: import("jspdf").jsPDF, text: string, maxWidth: number): string {
  if (doc.getTextWidth(text) <= maxWidth) return text;
  let s = text;
  while (s.length > 1 && doc.getTextWidth(`${s}...`) > maxWidth) {
    s = s.slice(0, -1);
  }
  return `${s.trimEnd()}...`;
}

/** Réduit la police (start → min, pas 0.2 pt) pour faire tenir le texte ;
 * la troncature n'intervient qu'en dernier recours (noms de tenant longs). */
function fitFontSize(doc: import("jspdf").jsPDF, text: string, maxWidth: number, start: number, min: number): number {
  let size = start;
  doc.setFontSize(size);
  while (size > min && doc.getTextWidth(text) > maxWidth) {
    size = Math.max(min, size - 0.2);
    doc.setFontSize(size);
  }
  return size;
}

/** Texte centré sur cx, avec troncature. */
function centerText(doc: import("jspdf").jsPDF, raw: string, cx: number, y: number, maxWidth: number) {
  doc.text(fitText(doc, winAnsi(raw), maxWidth), cx, y, { align: "center" });
}

/* ── Ticket ── */
function drawTicket(
  doc: import("jspdf").jsPDF,
  voucher: HotspotUser,
  x: number,
  y: number,
  qrDataUrl: string,
  logoCircle: string | null,
  opts: { tenantName: string; currency: string; lang: Lang; labels: VouchersA4PdfInput["labels"] },
) {
  const { tenantName, currency, lang, labels } = opts;
  const cx = x + CELL_W / 2;
  const pad = 1.4;
  const innerW = CELL_W - 2 * pad;

  // Cartouche pointillé (comme l'aperçu : bordure à découper).
  doc.setLineWidth(0.18);
  doc.setDrawColor(...BORDER);
  doc.setLineDashPattern([0.7, 0.55], 0);
  doc.roundedRect(x + 0.15, y + 0.15, CELL_W - 0.3, CELL_H - 0.3, 1.4, 1.4, "S");
  doc.setLineDashPattern([], 0);

  // ── Bandeau de marque (navy, coins hauts arrondis) ──
  const bandH = 5.4;
  const bx = x + 0.15;
  const by = y + 0.15;
  const bw = CELL_W - 0.3;
  const r = 1.3;
  doc.setFillColor(...NAVY);
  doc.roundedRect(bx, by, bw, bandH, r, r, "F");
  doc.rect(bx, by + bandH / 2, bw, bandH / 2, "F"); // aplatit le bas du bandeau

  const baseY = by + bandH / 2 + 0.92; // centrage optique des textes
  const rightPad = 1.7;

  // PRIX à droite (blanc, gras) — mesuré d'abord pour borner le nom du tenant.
  doc.setFont("helvetica", "bold");
  doc.setFontSize(6.4);
  doc.setTextColor(255, 255, 255);
  const priceTxt = fitText(doc, winAnsi(formatCurrency(voucher.price, currency, lang)), innerW * 0.42);
  const priceW = doc.getTextWidth(priceTxt);
  doc.text(priceTxt, bx + bw - rightPad, baseY, { align: "right" });

  // Logo circulaire à gauche du bandeau.
  let tenantX = bx + 1.7;
  if (logoCircle) {
    const lr = 1.55;
    doc.addImage(logoCircle, "PNG", bx + 1.7, by + bandH / 2 - lr, lr * 2, lr * 2);
    tenantX = bx + 1.7 + lr * 2 + 1.1;
  }

  // Nom du tenant (blanc, gras) : rétrécissement progressif puis troncature.
  const tenantMax = Math.max(bx + bw - rightPad - priceW - 1.8 - tenantX, 7);
  const tenantTxt = winAnsi(tenantName || "MikCloud");
  fitFontSize(doc, tenantTxt, tenantMax, 6.4, 5.2);
  doc.text(fitText(doc, tenantTxt, tenantMax), tenantX, baseY, { align: "left" });

  // ── QR orné du logo (PNG 384 px → 12,5 mm : net à l'impression) ──
  const qrX = cx - QR_MM / 2;
  const qrY = y + 6.9;
  doc.addImage(qrDataUrl, "PNG", qrX, qrY, QR_MM, QR_MM);

  // ── Code (identifiant) en navy, mot de passe si distinct ──
  const belowQr = qrY + QR_MM;
  doc.setTextColor(...NAVY);
  doc.setFont("courier", "bold");
  doc.setFontSize(8.4);
  centerText(doc, voucher.username, cx, belowQr + 3.1, innerW);
  if (!isSamePasswordMode(voucher)) {
    doc.setFont("courier", "normal");
    doc.setFontSize(6.2);
    doc.setTextColor(...INK);
    centerText(doc, `${labels.password} ${voucher.password}`, cx, belowQr + 5.9, innerW);
  }

  // Profil · quota (le PRIX vit dans le bandeau, jamais victime de la place).
  const meta = [voucher.profileName, voucher.dataQuotaMb > 0 ? formatBytes(voucher.dataQuotaMb * 1048576, lang) : ""]
    .filter(Boolean)
    .join(" · ");
  doc.setFont("helvetica", "normal");
  doc.setFontSize(5.4);
  doc.setTextColor(...MUTED);
  centerText(doc, meta, cx, belowQr + 8.5, innerW);

  // Aide de scan.
  doc.setTextColor(...FAINT);
  doc.setFontSize(4.8);
  centerText(doc, labels.scanHint, cx, belowQr + 11.4, innerW);
}

/* ── Document ── */
export async function buildVouchersA4Pdf(input: VouchersA4PdfInput): Promise<{ blob: Blob; filename: string }> {
  const { vouchers, title, tenantName, hotspotLoginUrl, currency, lang, labels } = input;
  const [{ jsPDF }] = await Promise.all([import("jspdf")]);

  // Logo circulaire (bandeaux + en-tête de page) et QR ornés, en parallèle.
  const logo = await loadLogo();
  const logoCircle = logo ? circleLogoDataUrl(logo) : null;
  const qrUrls = await Promise.all(vouchers.map((v) => qrWithLogo(qrValue(v, hotspotLoginUrl), logo)));

  const doc = new jsPDF({ unit: "mm", format: "a4", orientation: "portrait", compress: true });
  const totalPages = Math.max(1, Math.ceil(vouchers.length / (COLS * ROWS)));

  for (let page = 0; page < totalPages; page++) {
    if (page > 0) doc.addPage("a4", "portrait");

    // Bandeau discret de page — logo + tenant à gauche, titre · page à droite.
    if (logoCircle) doc.addImage(logoCircle, "PNG", MARGIN, MARGIN + 0.45, 3.3, 3.3);
    doc.setFont("helvetica", "normal");
    doc.setTextColor(...FAINT);
    doc.setFontSize(5.4);
    doc.setCharSpace(0.18);
    doc.text(
      winAnsi((tenantName || "MikCloud").toUpperCase()),
      MARGIN + (logoCircle ? 4.3 : 0),
      MARGIN + 2.4,
      { align: "left" },
    );
    const stamp = `${winAnsi(title)} · ${labels.pageOf(page + 1, totalPages)}`;
    doc.text(fitText(doc, winAnsi(stamp), A4_W - 2 * MARGIN - 40), A4_W - MARGIN, MARGIN + 2.4, {
      align: "right",
    });
    doc.setCharSpace(0);

    // Grille 5 × 8.
    const pageVouchers = vouchers.slice(page * COLS * ROWS, (page + 1) * COLS * ROWS);
    for (let i = 0; i < pageVouchers.length; i++) {
      const col = i % COLS;
      const row = Math.floor(i / COLS);
      const x = MARGIN + col * (CELL_W + GAP);
      const y = GRID_TOP + row * (CELL_H + GAP);
      drawTicket(doc, pageVouchers[i], x, y, qrUrls[page * COLS * ROWS + i], logoCircle, {
        tenantName,
        currency,
        lang,
        labels,
      });
    }
  }

  const d = new Date();
  const pad2 = (n: number) => String(n).padStart(2, "0");
  const filename = `mikcloud-vouchers-a4-${d.getFullYear()}${pad2(d.getMonth() + 1)}${pad2(d.getDate())}-${pad2(
    d.getHours(),
  )}${pad2(d.getMinutes())}.pdf`;

  return { blob: doc.output("blob"), filename };
}

/** Déclenche le téléchargement d'un blob (nettoyage différé de l'URL). */
export function downloadBlob(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 10_000);
}
