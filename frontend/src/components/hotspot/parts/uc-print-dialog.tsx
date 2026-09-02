"use client";

// Dialog d'impression des vouchers — tickets noirs sur fond blanc dans une zone
// .print-area (globals.css : à l'impression, le dialog devient le document en flux
// normal et tout le reste est masqué ; .no-print cache la barre d'outils).
//
// Mode modèle (F2) : si des VoucherTemplates sont fournis, une barre d'outils
// permet de choisir le modèle (« Ticket standard MikCloud » = rendu historique
// conservé tel quel) ; l'aperçu est rendu via renderBatch (QR + variables) et le
// choix est mémorisé dans localStorage("mikcloud-tpl").
//
// Quick print (F12) : le prop optionnel `batchId` identifie le lot en cours ;
// à chaque impression il est mémorisé dans localStorage("mikcloud-last-batch")
// pour la réimpression rapide depuis la vue Vouchers.

import {
  useEffect,
  useMemo,
  useState,
  type CSSProperties,
} from "react";
import { Loader2, Printer } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogTitle } from "@/components/ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useCurrency, useSettings } from "@/components/hotspot/parts/sd-currency";
import {
  a4GridPlan,
  isSamePasswordMode,
  renderBatch,
} from "@/components/hotspot/parts/template-render";
import { useI18n } from "@/lib/hotspot/i18n";
import { formatBytes, formatCurrency , fmtRouterDuration } from "@/lib/hotspot/format";
import type { HotspotUser, Profile, VoucherTemplate } from "@/lib/hotspot/types";

/** Clé localStorage du dernier modèle choisi pour l'impression. */
const TPL_STORAGE_KEY = "mikcloud-tpl";
/** Clé localStorage du dernier lot imprimé (quick print, F12). */
export const LAST_BATCH_STORAGE_KEY = "mikcloud-last-batch";
/** Valeur spéciale du Select = rendu historique MikCloud (sans modèle). */
const TPL_STANDARD = "standard";

interface UcPrintDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  vouchers: HotspotUser[];
  title: string;
  description?: string;
  tenantName: string;
  profiles: Profile[];
  /** Modèles de vouchers (F2) — active le sélecteur de modèle si non vide. */
  templates?: VoucherTemplate[];
  /** Lot en cours (F12) — mémorisé à l'impression pour la réimpression rapide. */
  batchId?: string;
}

export function UcPrintDialog({
  open,
  onOpenChange,
  vouchers,
  title,
  description,
  tenantName,
  profiles,
  templates,
  batchId,
}: UcPrintDialogProps) {
  const { t, tf, lang } = useI18n();
  const currency = useCurrency();
  const { data: settings } = useSettings();
  const dnsName = settings?.tenant.dnsName ?? "";
  const logoUrl = settings?.tenant.logoUrl ?? "";

  // "" = choix automatique (dernier utilisé → défaut du compte → standard).
  const [tplChoice, setTplChoice] = useState<string>("");

  // Aperçu du lot rendu via le modèle (async : génération des QR codes).
  // « batch » conserve une empreinte du rendu : tant qu'elle diffère de l'état
  // courant, l'aperçu est considéré en cours de génération.
  const [batch, setBatch] = useState<{ key: string; html: string } | null>(null);

  const hasTemplates = (templates?.length ?? 0) > 0;
  // Modèle effectivement sélectionné (null = ticket standard).
  const selectedTemplate = useMemo<VoucherTemplate | null>(() => {
    if (!hasTemplates || !templates) return null;
    if (tplChoice === TPL_STANDARD) return null;
    if (tplChoice) return templates.find((t) => t.id === tplChoice) ?? null;
    if (typeof window !== "undefined") {
      const saved = window.localStorage.getItem(TPL_STORAGE_KEY);
      if (saved === TPL_STANDARD) return null;
      if (saved) {
        const found = templates.find((t) => t.id === saved);
        if (found) return found;
      }
    }
    return templates.find((t) => t.isDefault) ?? null;
  }, [hasTemplates, templates, tplChoice]);

  // Empreinte du rendu attendu (modèle + lot) — l'aperçu en cours vaut null tant
  // que le rendu asynchrone correspondant n'est pas arrivé.
  const batchKey = selectedTemplate
    ? `${selectedTemplate.id}|${selectedTemplate.bodyHtml.length}|${vouchers.length}|${vouchers[0]?.id ?? ""}`
    : null;
  const batchHtml = batch && batch.key === batchKey ? batch.html : null;
  const batchFailed = batchHtml !== null && batchHtml === "";

  // Grille A4 adaptative du rendu standard : au-delà de 12 tickets, 4→6
  // colonnes + zoom (a4GridPlan) pour caser le maximum de tickets par feuille.
  const stdPlan = a4GridPlan(vouchers.length);

  function handleTemplateChange(value: string) {
    setTplChoice(value);
    try {
      window.localStorage.setItem(TPL_STORAGE_KEY, value);
    } catch {
      /* localStorage indisponible (navigation privée…) — choix non mémorisé */
    }
  }

  /** Imprime le lot et mémorise le dernier lot imprimé (F12 quick print). */
  function handlePrint() {
    if (batchId) {
      try {
        window.localStorage.setItem(LAST_BATCH_STORAGE_KEY, batchId);
      } catch {
        /* localStorage indisponible — quick print non mémorisé */
      }
    }
    window.print();
  }

  // Aperçu du lot rendu via le modèle (asynchrone : génération des QR codes).
  useEffect(() => {
    if (!open || !selectedTemplate) return;
    let cancelled = false;
    const key = batchKey ?? "";
    renderBatch(selectedTemplate, vouchers, {
      tenantName,
      dnsName,
      logoUrl,
      currency,
      profiles,
    })
      .then((html) => {
        if (!cancelled) setBatch({ key, html });
      })
      .catch(() => {
        if (!cancelled) setBatch({ key, html: "" });
      });
    return () => {
      cancelled = true;
    };
  }, [open, selectedTemplate, vouchers, tenantName, dnsName, logoUrl, currency, profiles, batchKey]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="gap-4 sm:max-w-3xl">
        <div className="no-print flex flex-wrap items-start justify-between gap-3">
          <div className="min-w-0">
            <DialogTitle className="truncate">{title}</DialogTitle>
            <DialogDescription>
              {description ??
                tf("print.ticketsReady", {
                  n: vouchers.length,
                  p: vouchers.length > 1 ? "s" : "",
                })}
            </DialogDescription>
          </div>
          <div className="flex shrink-0 flex-wrap items-center gap-2">
            {hasTemplates && (
              <Select
                value={selectedTemplate?.id ?? TPL_STANDARD}
                onValueChange={handleTemplateChange}
              >
                <SelectTrigger
                  className="h-10 w-full sm:w-56"
                  aria-label={t("print.templateAria")}
                >
                  <SelectValue placeholder={t("print.template")} />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={TPL_STANDARD}>{t("print.standard")}</SelectItem>
                  {templates?.map((template) => (
                    <SelectItem key={template.id} value={template.id}>
                      {template.name}
                      {template.isDefault ? t("print.defaultSuffix") : ""}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            )}
            <Button variant="outline" onClick={() => onOpenChange(false)}>
              {t("common.close")}
            </Button>
            <Button onClick={handlePrint}>
              <Printer className="size-4" />
              {t("print.action")}
            </Button>
          </div>
        </div>

        <div className="max-h-[65vh] overflow-y-auto print:max-h-none print:overflow-visible">
          {selectedTemplate ? (
            // Aperçu du modèle — le wrapper .tpl-batch (dans batchHtml) porte la
            // classe .tpl-format-{format} pilotant les sauts de page à l'impression.
            <div className="print-area rounded-lg bg-white p-4 text-black">
              {batchFailed ? (
                <p className="p-6 text-center text-sm text-neutral-500">{t("print.previewFailed")}</p>
              ) : batchHtml !== null ? (
                <div dangerouslySetInnerHTML={{ __html: batchHtml }} />
              ) : (
                <p className="flex items-center justify-center gap-2 p-10 text-sm text-neutral-500">
                  <Loader2 className="size-4 animate-spin" aria-hidden />
                  {t("print.generating")}
                </p>
              )}
            </div>
          ) : (
            <div className="print-area rounded-lg bg-white p-4 text-black">
              {/* Grille ADAPTATIVE (stdPlan) : les variables --vgrid-* alimentent
                  la règle d'impression !important de globals.css (aperçu = print). */}
              <div
                className="voucher-print-grid grid"
                style={{
                  gridTemplateColumns: `repeat(${stdPlan.cols}, minmax(0, 1fr))`,
                  gap: `${stdPlan.gapMm}mm`,
                  "--vgrid-cols": stdPlan.cols,
                  "--vgrid-gap": `${stdPlan.gapMm}mm`,
                } as CSSProperties}
              >
                {vouchers.map((voucher) => {
                  const prof = profiles.find((p) => p.id === voucher.profileId);
                  const validityMin = prof ? (prof.validityMin > 0 ? prof.validityMin : prof.validityDays * 1440) : 0;
                  return (
                    <div
                      key={voucher.id}
                      className="flex flex-col items-center gap-1 rounded-lg border-2 border-dashed border-black p-3 text-center break-inside-avoid"
                      style={stdPlan.zoom < 1 ? { zoom: stdPlan.zoom } : undefined}
                    >
                      <p className="text-sm font-bold leading-tight">{tenantName || "MikCloud"}</p>
                      <p className="text-[10px] uppercase tracking-widest text-neutral-500">
                        {t("print.wifiHotspot")}
                      </p>
                      <p className="mt-1 text-lg font-bold font-mono tracking-wider">{voucher.username}</p>
                      {/* Mode « mot de passe = identifiant » : le code seul sur le ticket. */}
                      {!isSamePasswordMode(voucher) && (
                        <p className="font-mono text-sm">
                          {t("print.passwordLabel")} {voucher.password}
                        </p>
                      )}
                      <p className="text-xs text-neutral-700">
                        {voucher.profileName}
                        {validityMin ? ` · ${fmtRouterDuration(validityMin)}` : ""}
                        {(voucher.dataQuotaMb ?? 0) > 0
                          ? ` · ${formatBytes(voucher.dataQuotaMb * 1048576, lang)}`
                          : ""}
                      </p>
                      <p className="text-xs font-semibold">
                        {formatCurrency(voucher.price, currency, lang)}
                      </p>
                      <p className="mt-1 w-full border-t border-neutral-300 pt-1 text-[10px] text-neutral-500">
                        {t("print.keepTicket")}
                      </p>
                    </div>
                  );
                })}
              </div>
            </div>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}
