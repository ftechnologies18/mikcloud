"use client";

// Carte ticket du comptoir Mode Vente — UX R1 : identique dans les deux vues
// (groupée et plate). Anti-fuite : le code reste masqué tant que la vente
// n'est pas confirmée ; en mode retour la carte devient une case à cocher
// (pas de vente — anti-misclick N°20). Présentation pure : l'état vit dans
// sell-shell.tsx.

import { BadgeCheck, CheckCircle2, Circle, Loader2 } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { isSamePasswordMode } from "@/components/hotspot/parts/template-render";
import { useI18n } from "@/lib/hotspot/i18n";
import { formatCurrency } from "@/lib/hotspot/format";
import { expiresSoon, type SellVoucher } from "./helpers";

interface VoucherCardProps {
  voucher: SellVoucher;
  currency: string;
  /** N°20 — mode retour : la carte devient sélectionnable, vente coupée. */
  returnMode: boolean;
  isSelected: boolean;
  /** UX R6 — ce ticket a une vente en file hors-ligne (badge « en attente »). */
  isQueued: boolean;
  /** Mutation de vente en vol (globale) + sur CE ticket (spinner). */
  sellPending: boolean;
  sellPendingId: string | null;
  onToggle: (id: string) => void;
  onSell: (voucher: SellVoucher) => void;
}

export function VoucherCard({
  voucher: v,
  currency,
  returnMode,
  isSelected,
  isQueued,
  sellPending,
  sellPendingId,
  onToggle,
  onSell,
}: VoucherCardProps) {
  const { t, lang } = useI18n();
  const price = v.sellingPrice || v.price;
  return (
    <Card
      key={v.id}
      className={`gap-0 py-0 transition-shadow ${returnMode && isSelected ? "ring-2 ring-primary" : ""}`}
    >
      <CardContent
        className={`p-4 ${returnMode ? "cursor-pointer select-none" : ""}`}
        onClick={returnMode ? () => onToggle(v.id) : undefined}
        role={returnMode ? "checkbox" : undefined}
        aria-checked={returnMode ? isSelected : undefined}
        tabIndex={returnMode ? 0 : undefined}
        onKeyDown={
          returnMode
            ? (e) => {
                if (e.key === " " || e.key === "Enter") {
                  e.preventDefault();
                  onToggle(v.id);
                }
              }
            : undefined
        }
      >
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <div className="flex items-center gap-2">
              <p className="truncate font-semibold">{v.profileName}</p>
              {v.dataQuotaMb > 0 && (
                <Badge variant="secondary" className="text-[10px]">
                  {Math.round(v.dataQuotaMb / 1024)} Go
                </Badge>
              )}
              {expiresSoon(v) && (
                <Badge className="border-amber-500/40 bg-amber-500/10 text-[10px] text-amber-700 dark:text-amber-300">
                  {t("sell.expiresSoon")}
                </Badge>
              )}
              {/* UX R6 — vendu hors-ligne, en attente de replay serveur. */}
              {isQueued && (
                <Badge className="border-amber-500/40 bg-amber-500/10 text-[10px] text-amber-700 dark:text-amber-300">
                  {t("sell.chipPending")}
                </Badge>
              )}
            </div>
            <p className="mt-0.5 text-xs text-muted-foreground">
              {v.routerName} ·{" "}
              {v.expiresAt
                ? `${t("sell.expires")} ${new Date(v.expiresAt).toLocaleDateString(lang === "en" ? "en-GB" : "fr-FR", {
                    day: "2-digit",
                    month: "short",
                  })}`
                : t("sell.expiresOnFirstLogin")}
            </p>
          </div>
          {returnMode ? (
            <span aria-hidden className="shrink-0 text-primary">
              {isSelected ? <CheckCircle2 className="size-6" /> : <Circle className="size-6 text-muted-foreground/40" />}
            </span>
          ) : (
            <p className="shrink-0 text-lg font-bold text-primary tabular-nums">
              {formatCurrency(price, currency, lang)}
            </p>
          )}
        </div>

        {/* Anti-fuite — code masqué tant que la vente n'est pas confirmée :
            rien ne peut être lu, copié ou partagé avant le reçu. */}
        <div className={`mt-3 grid gap-2 rounded-lg bg-muted/50 p-3 font-mono text-sm ${isSamePasswordMode(v) ? "grid-cols-1" : "grid-cols-2"}`}>
          <div>
            <p className="text-[10px] tracking-wide text-muted-foreground uppercase">{t("sell.code")}</p>
            <p className="mt-0.5 font-semibold tracking-widest" aria-label={t("sell.codeAfterConfirm")}>••••••</p>
          </div>
          {/* Mode « mot de passe = identifiant » : le code seul. */}
          {!isSamePasswordMode(v) && (
            <div>
              <p className="text-[10px] tracking-wide text-muted-foreground uppercase">{t("sell.password")}</p>
              <p className="mt-0.5 font-semibold tracking-widest">••••••</p>
            </div>
          )}
        </div>
        <p className="mt-1.5 text-[11px] text-muted-foreground">{t("sell.codeAfterConfirm")}</p>

        {/* N°20 — en mode retour : pas de vente (anti-misclick). Le
            partage n'existe plus sur la carte : il vit dans le reçu,
            APRÈS confirmation — le code ne quitte jamais l'app avant. */}
        {!returnMode && (
          <Button className="mt-3 w-full" onClick={() => onSell(v)} disabled={sellPending}>
            {sellPendingId === v.id ? (
              <Loader2 className="size-4 animate-spin" />
            ) : (
              <BadgeCheck className="size-4" />
            )}
            {t("sell.sellBtn")}
          </Button>
        )}
      </CardContent>
    </Card>
  );
}
