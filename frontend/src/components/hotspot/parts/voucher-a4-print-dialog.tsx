"use client";

// Dialog d'impression A4 + QR — approche PDF : le bouton génère un VRAI
// document PDF (jsPDF — voir lib/hotspot/a4-pdf.ts) figé à 40 tickets par
// page (grille 5 × 8). L'impression se fait depuis le lecteur PDF : la mise
// en page ne dépend plus du moteur d'impression du navigateur (zoom, marges,
// en-têtes…). Le QR pointe vers la page de login du hotspot avec le code
// pré-rempli (connexion en 1 scan) ou, à défaut, porte le code en texte.

import { useMemo, useState } from "react";
import { QRCodeSVG } from "qrcode.react";
import { FileDown, Loader2 } from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogTitle } from "@/components/ui/dialog";
import { isSamePasswordMode } from "@/components/hotspot/parts/template-render";
import { useCurrency } from "@/components/hotspot/parts/sd-currency";
import { buildVouchersA4Pdf, downloadBlob, qrValue } from "@/lib/hotspot/a4-pdf";
import { useI18n } from "@/lib/hotspot/i18n";
import { formatBytes, formatCurrency } from "@/lib/hotspot/format";
import type { HotspotUser } from "@/lib/hotspot/types";

// 40 tickets par page : 5 colonnes × 8 lignes — tickets compactés (QR 42 px ≈ 11 mm,
// toujours scannable). La même géométrie est reprise au mm dans le générateur PDF.
const COLS = 5;
const ROWS = 8;
const PER_PAGE = COLS * ROWS;
const QR_SIZE = 42;

interface VoucherA4PrintDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  vouchers: HotspotUser[];
  title: string;
  tenantName: string;
  /** Page de login MikroTik du hotspot — si fournie, le QR ouvre la session pré-remplie. */
  hotspotLoginUrl?: string;
}

export function VoucherA4PrintDialog({
  open,
  onOpenChange,
  vouchers,
  title,
  tenantName,
  hotspotLoginUrl,
}: VoucherA4PrintDialogProps) {
  const { t, tf, lang } = useI18n();
  const currency = useCurrency();
  const [pdfBusy, setPdfBusy] = useState(false);

  const pages = useMemo(() => {
    const chunks: HotspotUser[][] = [];
    for (let i = 0; i < vouchers.length; i += PER_PAGE) {
      chunks.push(vouchers.slice(i, i + PER_PAGE));
    }
    return chunks;
  }, [vouchers]);

  const generatePdf = async () => {
    if (pdfBusy || vouchers.length === 0) return;
    setPdfBusy(true);
    try {
      const { blob, filename } = await buildVouchersA4Pdf({
        vouchers,
        title,
        tenantName,
        hotspotLoginUrl,
        currency,
        lang,
        labels: {
          password: t("a4.password"),
          scanHint: t("a4.scanHint"),
          pageOf: (n: number, total: number) => tf("a4.pageStamp", { n, total }),
        },
      });
      downloadBlob(blob, filename);
      toast.success(t("a4.pdfReady"));
    } catch {
      toast.error(t("a4.pdfError"));
    } finally {
      setPdfBusy(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="gap-4 sm:max-w-5xl">
        {/* Barre d'outils (écran) — le document imprimé est le PDF téléchargé */}
        <div className="flex flex-wrap items-start justify-between gap-3">
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
            <Button className="min-h-10" disabled={pdfBusy || vouchers.length === 0} onClick={generatePdf}>
              {pdfBusy ? <Loader2 className="size-4 animate-spin" /> : <FileDown className="size-4" />}
              {pdfBusy ? t("a4.pdfBusy") : t("a4.pdf")}
            </Button>
          </div>
        </div>

        {/* Aperçu écran — mise en page identique au PDF généré (géométrie calquée) */}
        <div className="a4-scroll max-h-[70vh] overflow-auto rounded-lg p-4">
          <div className="mx-auto flex w-[794px] flex-col gap-6 bg-white text-black">
            {pages.length === 0 && (
              <p className="py-12 text-center text-sm text-neutral-500">{t("a4.empty")}</p>
            )}
            {pages.map((pageVouchers, pageIndex) => (
              <div
                key={pageIndex}
                className="a4-page flex h-[1123px] w-full flex-col rounded-lg border border-neutral-200 p-[7.5mm] shadow-md"
              >
                {/* En-tête discret de page */}
                <div className="mb-1 flex items-center justify-between gap-4 text-[8px] uppercase tracking-wider text-neutral-400">
                  <span className="truncate">{tenantName || "MikCloud"}</span>
                  <span className="shrink-0">
                    {title} · {tf("a4.pageStamp", { n: pageIndex + 1, total: pages.length })}
                  </span>
                </div>
                {/* Grille 5 × 8 — les tickets remplissent exactement la page */}
                <div className="grid min-h-0 flex-1 grid-cols-5 grid-rows-8 gap-[1.5mm]">
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
    <div className="a4-ticket flex min-h-0 flex-col items-center justify-between gap-0.5 rounded-md border border-dashed border-black p-1.5 text-center break-inside-avoid">
      <div className="w-full">
        <p className="truncate text-[10px] font-bold leading-tight">{tenantName || "MikCloud"}</p>
        <p className="text-[6px] uppercase tracking-[0.3em] text-neutral-500">WIFI</p>
      </div>
      <QRCodeSVG
        value={qrValue(voucher, hotspotLoginUrl)}
        size={QR_SIZE}
        className="shrink-0"
        aria-label={tf("a4.qrAria", { code: voucher.username })}
      />
      <div className="w-full">
        <p className="truncate font-mono text-[12px] font-bold leading-tight">{voucher.username}</p>
        {/* Mode « mot de passe = identifiant » : le code seul sur le ticket. */}
        {!isSamePasswordMode(voucher) && (
          <p className="font-mono text-[9px] leading-tight">{t("a4.password")} {voucher.password}</p>
        )}
        <p className="mt-0.5 text-[9px] leading-tight">
          {voucher.profileName}
          {voucher.dataQuotaMb > 0 && ` · ${formatBytes(voucher.dataQuotaMb * 1048576, lang)}`} ·{" "}
          <span className="font-semibold">{formatCurrency(voucher.price, currency)}</span>
        </p>
        <p className="mt-0.5 text-[7px] leading-tight text-neutral-500">{t("a4.scanHint")}</p>
      </div>
    </div>
  );
}
