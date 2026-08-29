"use client";

// Dialog d'impression A4 + QR — 20 tickets par page (grille 4 × 5), pensé pour le flux
// revendeur « imprimer → vendre » : chaque ticket porte un QR pointant vers la page de
// login du hotspot avec le code pré-rempli (connexion en 1 scan) ou, à défaut, le code
// en texte. Impression : chaque .a4-page remplit exactement une feuille A4 — voir la
// section « Impression A4 + QR » de globals.css (body.a4-printing + .print-area.a4-print).

import { useEffect, useMemo } from "react";
import { QRCodeSVG } from "qrcode.react";
import { Printer } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogTitle } from "@/components/ui/dialog";
import { useCurrency } from "@/components/hotspot/parts/sd-currency";
import { useI18n } from "@/lib/hotspot/i18n";
import { formatBytes, formatCurrency } from "@/lib/hotspot/format";
import type { HotspotUser } from "@/lib/hotspot/types";

// 20 tickets par page : 4 colonnes × 5 lignes.
const PER_PAGE = 20;
const QR_SIZE = 56;

interface VoucherA4PrintDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  vouchers: HotspotUser[];
  title: string;
  tenantName: string;
  /** Page de login MikroTik du hotspot — si fournie, le QR ouvre la session pré-remplie. */
  hotspotLoginUrl?: string;
}

/** Contenu du QR : URL de login pré-remplie si disponible, sinon « code / mot de passe ». */
function qrValue(voucher: HotspotUser, hotspotLoginUrl?: string): string {
  if (hotspotLoginUrl) {
    const sep = hotspotLoginUrl.includes("?") ? "&" : "?";
    return `${hotspotLoginUrl}${sep}username=${encodeURIComponent(voucher.username)}&password=${encodeURIComponent(
      voucher.password,
    )}`;
  }
  return `${voucher.username} / ${voucher.password}`;
}

export function VoucherA4PrintDialog({
  open,
  onOpenChange,
  vouchers,
  title,
  tenantName,
  hotspotLoginUrl,
}: VoucherA4PrintDialogProps) {
  const { t, tf } = useI18n();
  const currency = useCurrency();

  // Marque le document pendant l'impression A4 : globals.css n'imprime alors
  // que ce dialog, chaque page A4 étant coupée proprement (break-after: page).
  useEffect(() => {
    if (!open) return;
    document.body.classList.add("a4-printing");
    return () => document.body.classList.remove("a4-printing");
  }, [open]);

  const pages = useMemo(() => {
    const chunks: HotspotUser[][] = [];
    for (let i = 0; i < vouchers.length; i += PER_PAGE) {
      chunks.push(vouchers.slice(i, i + PER_PAGE));
    }
    return chunks;
  }, [vouchers]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="a4-dialog gap-4 sm:max-w-5xl">
        {/* Barre d'outils — masquée à l'impression (.no-print) */}
        <div className="no-print flex flex-wrap items-start justify-between gap-3">
          <div className="min-w-0">
            <DialogTitle className="truncate">{title}</DialogTitle>
            <DialogDescription>
              {vouchers.length > 1
                ? tf("a4.ticketsMany", { n: vouchers.length })
                : tf("a4.ticketsOne", { n: vouchers.length })}
              {pages.length > 1 ? tf("a4.pages", { n: pages.length }) : ""}
              {hotspotLoginUrl ? t("a4.qrLogin") : t("a4.qrText")}
            </DialogDescription>
          </div>
          <div className="flex shrink-0 items-center gap-2">
            <Button variant="outline" className="min-h-10" onClick={() => onOpenChange(false)}>
              {t("a4.close")}
            </Button>
            <Button className="min-h-10" onClick={() => window.print()}>
              <Printer className="size-4" />
              {t("a4.print")}
            </Button>
          </div>
        </div>

        {/* Aperçu A4 — défilement écran uniquement, pleine page à l'impression */}
        <div className="a4-scroll max-h-[70vh] overflow-auto rounded-lg p-4">
          <div className="print-area a4-print mx-auto flex w-[794px] flex-col gap-6 bg-white text-black">
            {pages.length === 0 && (
              <p className="py-12 text-center text-sm text-neutral-500">{t("a4.empty")}</p>
            )}
            {pages.map((pageVouchers, pageIndex) => (
              <div
                key={pageIndex}
                className="a4-page flex h-[1123px] w-full flex-col rounded-lg border border-neutral-200 p-[10mm] shadow-md"
              >
                {/* En-tête discret de page */}
                <div className="mb-1.5 flex items-center justify-between gap-4 text-[9px] uppercase tracking-wider text-neutral-400">
                  <span className="truncate">{tenantName || "MikCloud"}</span>
                  <span className="shrink-0">
                    {title} · {tf("a4.pageStamp", { n: pageIndex + 1, total: pages.length })}
                  </span>
                </div>
                {/* Grille 4 × 5 — les tickets remplissent exactement la page */}
                <div className="grid min-h-0 flex-1 grid-cols-4 grid-rows-5 gap-2.5">
                  {pageVouchers.map((voucher) => (
                    <A4Ticket
                      key={voucher.id}
                      voucher={voucher}
                      tenantName={tenantName}
                      currency={currency}
                      hotspotLoginUrl={hotspotLoginUrl}
                    />
                  ))}
                </div>
              </div>
            ))}
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}

/* ─────────────────────────── Ticket A4 (QR + code) ─────────────────────────── */

function A4Ticket({
  voucher,
  tenantName,
  currency,
  hotspotLoginUrl,
}: {
  voucher: HotspotUser;
  tenantName: string;
  currency: string;
  hotspotLoginUrl?: string;
}) {
  const { t, tf, lang } = useI18n();
  return (
    <div className="a4-ticket flex min-h-0 flex-col items-center justify-between gap-1 rounded-md border border-dashed border-black p-2 text-center break-inside-avoid">
      <div className="w-full">
        <p className="truncate text-[11px] font-bold leading-tight">{tenantName || "MikCloud"}</p>
        <p className="text-[7px] uppercase tracking-[0.3em] text-neutral-500">WIFI</p>
      </div>
      <QRCodeSVG
        value={qrValue(voucher, hotspotLoginUrl)}
        size={QR_SIZE}
        className="shrink-0"
        aria-label={tf("a4.qrAria", { code: voucher.username })}
      />
      <div className="w-full">
        <p className="truncate font-mono text-[15px] font-bold leading-tight">{voucher.username}</p>
        <p className="font-mono text-[10px] leading-tight">{t("a4.password")} {voucher.password}</p>
        <p className="mt-0.5 text-[10px] leading-tight">
          {voucher.profileName}
          {voucher.dataQuotaMb > 0 && ` · ${formatBytes(voucher.dataQuotaMb * 1048576, lang)}`} ·{" "}
          <span className="font-semibold">{formatCurrency(voucher.price, currency)}</span>
        </p>
        <p className="mt-0.5 text-[8px] leading-tight text-neutral-500">{t("a4.scanHint")}</p>
      </div>
    </div>
  );
}
