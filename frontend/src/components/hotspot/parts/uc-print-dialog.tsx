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
import { a4GridPlan, renderBatch } from "@/components/hotspot/parts/template-render";
import { VoucherTicketCard } from "@/components/hotspot/parts/voucher-ticket-card";
import { useI18n } from "@/lib/hotspot/i18n";
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

  // N°22 — impression pour le compte des revendeurs : le lot imprimé peut
  // contenir des tickets attribués (ResellerID). L'impression reste le service
  // légitime du gérant (le revendeur n'a pas toujours d'imprimante) : le
  // bandeau le rappelle, la remise des codes est tracée côté serveur
  // (POST /api/vouchers/print ou journal de génération/transfert) et la
  // propriété ne change pas — la vente auto crédite le revendeur à la 1ʳᵉ
  // connexion du client.
  const resellerCount = vouchers.reduce((n, v) => (v.resellerId ? n + 1 : n), 0);
  const resellerNames = useMemo(() => {
    const names: string[] = [];
    for (const v of vouchers) {
      if (!v.resellerId) continue;
      const name = v.resellerName || v.resellerId;
      if (!names.includes(name)) names.push(name);
    }
    return names;
  }, [vouchers]);

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

  // Grille A4 adaptative du rendu standard : au-delà de 24 tickets, 5
  // colonnes + réduction ×0,70 (a4GridPlan) — plafond ~35 tickets par feuille
  // A4, les lots plus gros sont paginés à taille constante.
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

        {/* N°22 — bandeau « impression pour le compte des revendeurs »
            (hors impression : .no-print). */}
        {resellerCount > 0 && (
          <p
            role="note"
            className="no-print rounded-lg border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-xs font-medium text-amber-700 dark:text-amber-400"
          >
            {tf("print.resellerNotice", { n: resellerCount, names: resellerNames.join(", ") })}
          </p>
        )}

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
                    <VoucherTicketCard
                      key={voucher.id}
                      voucher={voucher}
                      tenantName={tenantName}
                      currency={currency}
                      lang={lang}
                      validityMin={validityMin}
                      style={stdPlan.zoom < 1 ? { zoom: stdPlan.zoom } : undefined}
                    />
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
