"use client";

// Dialog d'impression MULTI-FORMATS du GÉRANT (refonte onglet Lots, F3) —
// parité avec l'impression revendeur (sell-print-dialog) : grille A4,
// thermique 58 mm, thermique 80 mm. Contrairement au dialog PWA, les tickets
// sont déjà chargés par la vue (fetchActiveBatchVouchers — tickets ACTIFS du
// lot uniquement) : ce composant ne fetch rien, il rend et imprime.
// Le choix du format est mémorisé en localStorage ; les règles d'impression
// (.print-area, .tpl-format-*) sont celles de globals.css — partagées.

import { useEffect, useMemo, useState } from "react";
import { Loader2, Printer } from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogTitle,
} from "@/components/ui/dialog";
import { a4GridPlan } from "@/components/hotspot/parts/template-render";
import { VoucherTicketCard } from "@/components/hotspot/parts/voucher-ticket-card";
import { useI18n } from "@/lib/hotspot/i18n";
import type { HotspotUser } from "@/lib/hotspot/types";

/** Clé localStorage du format d'impression des lots (console gérant). */
const FMT_STORAGE_KEY = "mikcloud-batch-print-fmt";

type PrintFormat = "a4" | "58" | "80";

interface BatchPrintDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Tickets ACTIFS du lot (chargés par la vue) — null = chargement en cours. */
  vouchers: HotspotUser[] | null;
  title: string;
  tenantName: string;
  currency: string;
}

function loadSavedFormat(): PrintFormat {
  if (typeof window === "undefined") return "a4";
  try {
    const saved = window.localStorage.getItem(FMT_STORAGE_KEY);
    return saved === "58" || saved === "80" ? saved : "a4";
  } catch {
    return "a4";
  }
}

const FORMAT_OPTIONS: { value: PrintFormat; labelKey: string }[] = [
  { value: "a4", labelKey: "print.fmtA4" },
  { value: "58", labelKey: "print.fmt58" },
  { value: "80", labelKey: "print.fmt80" },
];

export function BatchPrintDialog({
  open,
  onOpenChange,
  vouchers,
  title,
  tenantName,
  currency,
}: BatchPrintDialogProps) {
  const { t, tf, lang } = useI18n();

  const [fmt, setFmt] = useState<PrintFormat>(loadSavedFormat);

  // Ticket thermique : le papier fait la largeur — @page est remplacé
  // dynamiquement (le @page A4 statique de globals.css ne peut pas être
  // conditionné à une classe CSS). Le style est retiré à la fermeture.
  useEffect(() => {
    if (!open || fmt === "a4") return;
    const style = document.createElement("style");
    style.setAttribute("data-mikcloud-dyn-page", "");
    style.textContent = `@media print { @page { size: ${fmt === "58" ? "58mm" : "80mm"} auto; margin: 2mm; } }`;
    document.head.appendChild(style);
    return () => {
      style.remove();
    };
  }, [open, fmt]);

  function changeFormat(value: PrintFormat) {
    setFmt(value);
    try {
      window.localStorage.setItem(FMT_STORAGE_KEY, value);
    } catch {
      /* localStorage indisponible — choix non mémorisé */
    }
  }

  // Grille A4 adaptative (identique aux dialogs revendeur et liste simple).
  const stdPlan = useMemo(() => a4GridPlan(vouchers?.length ?? 0), [vouchers]);

  const isLoading = vouchers === null;
  const items = vouchers ?? [];

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="gap-4 sm:max-w-3xl">
        <div className="no-print flex flex-wrap items-start justify-between gap-3">
          <div className="min-w-0">
            <DialogTitle className="truncate">{title}</DialogTitle>
            <DialogDescription>
              {isLoading
                ? t("sell.printLoading")
                : tf("print.ticketsReadySimple", { n: items.length, p: items.length > 1 ? "s" : "" })}
            </DialogDescription>
          </div>
          <div className="flex shrink-0 flex-wrap items-center gap-2">
            <div
              className="flex rounded-lg border bg-muted/30 p-0.5"
              role="group"
              aria-label={t("print.formatAria")}
            >
              {FORMAT_OPTIONS.map((opt) => (
                <button
                  key={opt.value}
                  type="button"
                  onClick={() => changeFormat(opt.value)}
                  aria-pressed={fmt === opt.value}
                  className={`min-h-9 rounded-md px-2.5 text-xs font-medium transition-colors ${
                    fmt === opt.value
                      ? "bg-background text-foreground shadow-sm"
                      : "text-muted-foreground hover:text-foreground"
                  }`}
                >
                  {t(opt.labelKey)}
                </button>
              ))}
            </div>
            <Button variant="outline" onClick={() => onOpenChange(false)}>
              {t("common.close")}
            </Button>
            <Button onClick={() => window.print()} disabled={isLoading || items.length === 0}>
              <Printer className="size-4" />
              {t("print.action")}
            </Button>
          </div>
        </div>

        <div className="max-h-[65vh] overflow-y-auto print:max-h-none print:overflow-visible">
          {isLoading ? (
            <div className="rounded-lg border bg-white p-10 text-center text-sm text-neutral-500">
              <Loader2 className="mx-auto size-5 animate-spin" aria-hidden />
              <p className="mt-2">{t("sell.printLoading")}</p>
            </div>
          ) : items.length === 0 ? (
            <div className="rounded-lg border bg-white p-10 text-center text-sm text-neutral-500">
              {t("sell.printEmpty")}
            </div>
          ) : fmt === "a4" ? (
            // Grille A4 — découpe le long des cadres pointillés.
            <div className="print-area rounded-lg bg-white p-4 text-black">
              <div
                className="voucher-print-grid grid"
                style={{
                  gridTemplateColumns: `repeat(${stdPlan.cols}, minmax(0, 1fr))`,
                  gap: `${stdPlan.gapMm}mm`,
                  "--vgrid-cols": stdPlan.cols,
                  "--vgrid-gap": `${stdPlan.gapMm}mm`,
                } as React.CSSProperties}
              >
                {items.map((v) => (
                  <VoucherTicketCard
                    key={v.id}
                    voucher={v}
                    tenantName={tenantName}
                    currency={currency}
                    lang={lang}
                  />
                ))}
              </div>
            </div>
          ) : (
            // Tickets thermiques — un ticket par rouleau (page-break-after via
            // les règles .tpl-format-* de globals.css), cadre = ligne de découpe.
            <div className="print-area rounded-lg bg-white p-4 text-black">
              <div className={`tpl-batch tpl-format-${fmt === "58" ? "58mm" : "80mm"}`}>
                {items.map((v) => (
                  <div
                    key={v.id}
                    className="tpl-ticket mx-auto w-[54mm] max-w-full"
                    style={fmt === "80" ? { width: "76mm" } : undefined}
                  >
                    <VoucherTicketCard
                      voucher={v}
                      tenantName={tenantName}
                      currency={currency}
                      lang={lang}
                    />
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}
