"use client";

// Dialog d'impression du REVENDEUR (Mode Vente /sell) — le revendeur imprime
// son stock pour le vendre au comptoir (« vente auto » : le ticket papier se
// vend tout seul à la première connexion du client, SoldAt tracé par le back).
//
// Trois formats :
// - a4 : grille MikCloud adaptative (a4GridPlan) — découpe au ciseau ;
// - 58 : tickets thermiques 58 mm — un ticket par rouleau (saut de page) ;
// - 80 : tickets thermiques 80 mm.
//
// Le stock est TOUJOURS relu en ENTIER (/api/sell/stock sans `limit` → tableau
// complet, contrat historique) : le lot imprimé est complet même si la PWA
// n'a chargé que les premières pages du stock paginé. Le choix du format est
// mémorisé en localStorage. Les règles d'impression (.print-area, .tpl-format-*)
// sont celles de globals.css — partagées avec l'impression du gérant.

import { useEffect, useMemo, useState } from "react";
import { Loader2, Printer } from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogTitle,
} from "@/components/ui/dialog";
import { a4GridPlan } from "@/components/hotspot/parts/template-render";
import { VoucherTicketCard } from "@/components/hotspot/parts/voucher-ticket-card";
import { api } from "@/lib/hotspot/api";
import { useI18n } from "@/lib/hotspot/i18n";

/** Clé localStorage du format d'impression revendeur. */
const FMT_STORAGE_KEY = "mikcloud-sell-print-fmt";

type PrintFormat = "a4" | "58" | "80";

/** Portée d'impression demandée par le shell : tout le stock ou un seul lot. */
export type SellPrintScope =
  | { kind: "all" }
  | { kind: "batch"; batchId: string | null; label: string };

interface SellPrintDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  scope: SellPrintScope;
  /** Branding du compte (tenant) — en-tête des tickets. */
  tenantName: string;
  currency: string;
}

/** Stock relu en entier — forme minimale utilisée pour l'impression
 * (structurellement compatible avec SellVoucher du shell). */
interface SellStockItem {
  id: string;
  username: string;
  password: string;
  profileName: string;
  price: number;
  sellingPrice: number;
  dataQuotaMb: number;
  batchId?: string;
  validityMin?: number;
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

export function SellPrintDialog({
  open,
  onOpenChange,
  scope,
  tenantName,
  currency,
}: SellPrintDialogProps) {
  const { t, tf, lang } = useI18n();

  const [fmt, setFmt] = useState<PrintFormat>(loadSavedFormat);
  const [items, setItems] = useState<SellStockItem[] | null>(null);

  // Empreinte du chargement (ouverture + portée) — évite de relire le stock à
  // chaque rendu (les callbacks t/onOpenChange changent d'identité au rendu).
  const loadKey = `${open}|${scope.kind}|${scope.kind === "batch" ? scope.batchId : ""}`;
  const [loadedKey, setLoadedKey] = useState<string | null>(null);

  useEffect(() => {
    if (!open || loadedKey === loadKey) return;
    let cancelled = false;
    api<SellStockItem[]>("/api/sell/stock")
      .then((all) => {
        if (cancelled) return;
        const list =
          scope.kind === "batch"
            ? all.filter((v) => (v.batchId ?? null) === scope.batchId)
            : all;
        setLoadedKey(loadKey);
        setItems(list);
      })
      .catch(() => {
        if (cancelled) return;
        setLoadedKey(loadKey); // ne pas retenter en boucle
        setItems([]);
        toast.error(t("sell.printError"));
        onOpenChange(false);
      });
    return () => {
      cancelled = true;
    };
  }, [open, loadKey, loadedKey, scope, t, onOpenChange]);

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

  // Grille A4 adaptative (identique au dialog du gérant).
  const stdPlan = useMemo(() => a4GridPlan(items?.length ?? 0), [items]);

  // En chargement dès que la portée change (l'empreinte n'est pas à jour) —
  // pas de setState direct dans l'effet (règle react-hooks).
  const isLoading = items === null || loadedKey !== loadKey;

  const title =
    scope.kind === "batch"
      ? tf("sell.printTitleLot", { label: scope.label })
      : t("sell.printTitle");

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="gap-4 sm:max-w-3xl">
        <div className="no-print flex flex-wrap items-start justify-between gap-3">
          <div className="min-w-0">
            <DialogTitle className="truncate">{title}</DialogTitle>
            <DialogDescription>
              {isLoading
                ? t("sell.printLoading")
                : tf("print.ticketsReadySimple", {
                    n: items?.length ?? 0,
                    p: (items?.length ?? 0) > 1 ? "s" : "",
                  })}
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
            <Button onClick={() => window.print()} disabled={isLoading || items === null || items.length === 0}>
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
          ) : items === null || items.length === 0 ? (
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
                    validityMin={v.validityMin ?? 0}
                    style={stdPlan.zoom < 1 ? { zoom: stdPlan.zoom } : undefined}
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
                      validityMin={v.validityMin ?? 0}
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
