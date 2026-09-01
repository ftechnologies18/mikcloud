// Générateur PDF A4 des vouchers — approche « document figé » : la mise en page
// est dessinée dans un vrai PDF (jsPDF), plus aucune dépendance au moteur
// d'impression du navigateur (zoom, marges, en-têtes, fragments CSS…).
// Impression : ouvrir le PDF téléchargé et lancer l'impression depuis le
// lecteur PDF — pagination pixel-perfect identique sur tous les postes.
//
// Layout : A4 portrait (210 × 297 mm), 40 tickets par page (grille 5 × 8),
// mêmes contenus que l'aperçu écran (tenant, WIFI, QR, code, MDP, profil,
// quota, prix, aide de scan).

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
const QR_MM = 12; // ≈ l'aperçu écran (42 px) — scannable à 300 dpi

const CELL_W = (A4_W - 2 * MARGIN - (COLS - 1) * GAP) / COLS;
const GRID_TOP = MARGIN + HEADER_H;
const CELL_H = (A4_H - GRID_TOP - MARGIN - (ROWS - 1) * GAP) / ROWS;

/* Couleurs (gris neutres, à l'identique de l'aperçu) */
const INK: [number, number, number] = [10, 10, 10];
const MUTED: [number, number, number] = [115, 115, 115]; // neutral-500
const FAINT: [number, number, number] = [163, 163, 163]; // neutral-400
const BORDER: [number, number, number] = [64, 64, 64];

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

  // Tenant + micro-label WIFI.
  doc.setTextColor(...INK);
  doc.setFont("helvetica", "bold");
  doc.setFontSize(6.6);
  centerText(doc, tenantName || "MikCloud", cx, y + 3.6, innerW);
  doc.setFont("helvetica", "normal");
  doc.setTextColor(...FAINT);
  doc.setFontSize(4.2);
  doc.setCharSpace(0.55);
  doc.text("WIFI", cx, y + 6.4, { align: "center" });
  doc.setCharSpace(0);

  // QR (PNG 256 px → 12 mm : net à l'impression).
  const qrX = cx - QR_MM / 2;
  const qrY = y + 7.6;
  doc.addImage(qrDataUrl, "PNG", qrX, qrY, QR_MM, QR_MM);

  // Code (identifiant) + mot de passe si distinct.
  const belowQr = qrY + QR_MM;
  doc.setTextColor(...INK);
  doc.setFont("courier", "bold");
  doc.setFontSize(8.2);
  centerText(doc, voucher.username, cx, belowQr + 3, innerW);
  if (!isSamePasswordMode(voucher)) {
    doc.setFont("courier", "normal");
    doc.setFontSize(6.2);
    centerText(doc, `${labels.password} ${voucher.password}`, cx, belowQr + 5.8, innerW);
  }

  // Profil · quota, puis PRIX sur sa ligne propre (en gras) : le prix est
  // l'information critique du flux revendeur — jamais victime de la troncature.
  const meta = [voucher.profileName, voucher.dataQuotaMb > 0 ? formatBytes(voucher.dataQuotaMb * 1048576, lang) : ""]
    .filter(Boolean)
    .join(" · ");
  doc.setFont("helvetica", "normal");
  doc.setFontSize(5.4);
  centerText(doc, meta, cx, belowQr + 8.3, innerW);
  doc.setFont("helvetica", "bold");
  doc.setFontSize(5.8);
  centerText(doc, formatCurrency(voucher.price, currency, lang), cx, belowQr + 10.7, innerW);

  // Aide de scan.
  doc.setTextColor(...MUTED);
  doc.setFont("helvetica", "normal");
  doc.setFontSize(4.8);
  centerText(doc, labels.scanHint, cx, belowQr + 13, innerW);
}

/* ── Document ── */
export async function buildVouchersA4Pdf(input: VouchersA4PdfInput): Promise<{ blob: Blob; filename: string }> {
  const { vouchers, title, tenantName, hotspotLoginUrl, currency, lang, labels } = input;
  const [{ jsPDF }] = await Promise.all([import("jspdf")]);

  // QR générés en parallèle (PNG 256 px, marge 0 — le cartouche fait office de zone silencieuse).
  const qrUrls = await Promise.all(
    vouchers.map((v) =>
      QRCode.toDataURL(qrValue(v, hotspotLoginUrl), {
        margin: 0,
        errorCorrectionLevel: "M",
        width: 256,
        color: { dark: "#000000", light: "#FFFFFF" },
      }),
    ),
  );

  const doc = new jsPDF({ unit: "mm", format: "a4", orientation: "portrait", compress: true });
  const totalPages = Math.max(1, Math.ceil(vouchers.length / (COLS * ROWS)));

  for (let page = 0; page < totalPages; page++) {
    if (page > 0) doc.addPage("a4", "portrait");

    // Bandeau discret de page — à l'identique de l'aperçu.
    doc.setFont("helvetica", "normal");
    doc.setTextColor(...FAINT);
    doc.setFontSize(5.4);
    doc.setCharSpace(0.18);
    doc.text(winAnsi((tenantName || "MikCloud").toUpperCase()), MARGIN, MARGIN + 2.4, { align: "left" });
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
      drawTicket(doc, pageVouchers[i], x, y, qrUrls[page * COLS * ROWS + i], {
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
