"use client";

// « Tour de contrôle » du stock (refonte v2 de l'onglet Lots) — une ligne
// compacte qui enchaîne les 3 temps du métier : le STOCK VIVANT (ce qui peut
// être vendu ou transféré, en valeur faciale), ce qui dort CHEZ LES
// REVENDEURS, et la VÉLOCITÉ 7 j (éculés = ventes + consommations, du CA).
// Les deux premières étapes sont des boutons qui filtrent la liste du dessous
// (état actif : ring + fond primary). Les chips d'alerte (marge en attente,
// expirations < 7 j) complètent à droite, conditionnées à > 0.
// Le parent fournit `money` — le même formateur que la liste (formatCurrency).

import { ChevronRight } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Card, CardContent } from "@/components/ui/card";
import { useI18n } from "@/lib/hotspot/i18n";
import type { BatchSummary } from "@/lib/hotspot/types";
import { cn } from "@/lib/utils";

interface BatchPipelineProps {
  summary: BatchSummary;
  statusFilter: string;
  holderFilter: string;
  onStatusFilter: (value: string) => void;
  onHolderFilter: (value: string) => void;
  /** Formateur monétaire du parent (ex. (n) => formatCurrency(n, currency, lang)). */
  money: (amount: number) => string;
}

/** Libellé + valeur + sous-ligne d'une étape du pipeline. */
function Step({
  label,
  value,
  sub,
  active,
  onClick,
  ariaPressed,
}: {
  label: string;
  value: string;
  sub: string;
  active?: boolean;
  onClick?: () => void;
  ariaPressed?: boolean;
}) {
  const body = (
    <>
      <span className="text-[10px] font-medium uppercase tracking-wide text-muted-foreground">{label}</span>
      <span className="text-base font-bold tabular-nums leading-tight md:text-lg">{value}</span>
      <span className="text-[11px] leading-tight text-muted-foreground md:text-xs">{sub}</span>
    </>
  );
  if (!onClick) {
    return <div className="flex min-w-0 flex-col gap-0.5 px-2 py-1.5">{body}</div>;
  }
  return (
    <button
      type="button"
      onClick={onClick}
      aria-pressed={ariaPressed}
      className={cn(
        "flex min-w-0 flex-col gap-0.5 rounded-lg border border-transparent px-2 py-1.5 text-left transition-colors",
        "hover:bg-muted/60 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
        active && "border-primary/40 bg-primary/10 ring-1 ring-primary",
      )}
    >
      {body}
    </button>
  );
}

export function BatchPipeline({
  summary,
  statusFilter,
  holderFilter,
  onStatusFilter,
  onHolderFilter,
  money,
}: BatchPipelineProps) {
  const { t, tf } = useI18n();

  return (
    <Card className="gap-0 py-0">
      <CardContent className="flex flex-wrap items-center gap-x-1.5 gap-y-2 p-3 md:p-4">
        {/* Les 3 étapes du pipeline — chevrons décoratifs entre chaque */}
        <div className="flex min-w-0 flex-1 flex-wrap items-center gap-x-1 gap-y-2">
          <Step
            label={t("vouchers.batches.pipeline.stock")}
            value={money(summary.stockFace)}
            sub={tf("vouchers.batches.pipeline.stockSub", {
              tickets: summary.stockTickets,
              value: money(summary.stockValue),
            })}
            active={statusFilter === "stock"}
            ariaPressed={statusFilter === "stock"}
            onClick={() => onStatusFilter("stock")}
          />
          <ChevronRight aria-hidden className="size-4 shrink-0 text-muted-foreground/60" />
          <Step
            label={t("vouchers.batches.pipeline.resellers")}
            value={String(summary.resellerStock)}
            sub={tf("vouchers.batches.pipeline.resellersSub", { value: money(summary.resellerStockValue) })}
            active={holderFilter === "resellers"}
            ariaPressed={holderFilter === "resellers"}
            // Toggle : re-clic = retour à « tous les détenteurs ».
            onClick={() => onHolderFilter(holderFilter === "resellers" ? "all" : "resellers")}
          />
          <ChevronRight aria-hidden className="size-4 shrink-0 text-muted-foreground/60" />
          {/* Éculés 7 j — lecture seule : c'est le résultat, pas un filtre. */}
          <Step
            label={t("vouchers.batches.pipeline.egress7d")}
            value={String(summary.sold7d)}
            sub={t("vouchers.batches.pipeline.egressSub")}
          />
        </div>

        {/* Chips d'alerte — visibles seulement si pertinentes */}
        {(summary.marginPending > 0 || summary.expiring7d > 0) && (
          <div className="flex w-full flex-wrap items-center gap-2 md:ml-auto md:w-auto">
            {summary.marginPending > 0 && (
              <Badge variant="outline" className="border-primary/40 text-primary">
                {tf("vouchers.batches.pipeline.marginChip", { value: money(summary.marginPending) })}
              </Badge>
            )}
            {summary.expiring7d > 0 && (
              <Badge
                variant="outline"
                className="border-amber-500/40 text-amber-600 dark:text-amber-400"
              >
                {tf("vouchers.batches.pipeline.expiringChip", { n: summary.expiring7d })}
              </Badge>
            )}
          </div>
        )}
      </CardContent>
    </Card>
  );
}
