"use client";

// Carte du TICKET STANDARD MikCloud — rendu partagé par le dialog d'impression
// du gérant (uc-print-dialog, grille A4) et celui du revendeur
// (sell-print-dialog, grille A4 + tickets thermiques). Un seul endroit pour
// faire évoluer le design du ticket papier.
//
// NB : currency/lang arrivent en PROPS (pas useCurrency) — la PWA revendeur
// n'a pas accès à /api/settings (403 session scopée revendeur) ; le gérant
// passe les valeurs qu'il a déjà en mémoire.

import type { CSSProperties } from "react";

import {
  formatBytes,
  formatCurrency,
  fmtRouterDuration,
} from "@/lib/hotspot/format";
import { isSamePasswordMode } from "@/components/hotspot/parts/template-render";
import { useI18n } from "@/lib/hotspot/i18n";
import type { Lang } from "@/lib/hotspot/i18n";

/** Forme minimale partagée par HotspotUser (console) et SellVoucher (revendeur). */
export interface TicketVoucher {
  username: string;
  password: string;
  profileName: string;
  price: number;
  dataQuotaMb?: number;
}

interface VoucherTicketCardProps {
  voucher: TicketVoucher;
  /** Nom du hotspot (en-tête du ticket) — settings du compte. */
  tenantName: string;
  /** Devise du compte (prix du ticket). */
  currency: string;
  /** Langue d'affichage des libellés (formatage des octets). */
  lang?: Lang;
  /** Validité du profil en minutes (0 = non affichée). */
  validityMin?: number;
  className?: string;
  style?: CSSProperties;
}

export function VoucherTicketCard({
  voucher,
  tenantName,
  currency,
  lang,
  validityMin = 0,
  className = "",
  style,
}: VoucherTicketCardProps) {
  const { t, lang: hookLang } = useI18n();
  const effectiveLang = lang ?? hookLang;
  return (
    // N°145 — durcissement mobile PWA : w-full + min-w-0 (le ticket suit sa
    // cellule, même minmax(0,1fr) écrasée en aperçu mobile) et break-words sur
    // chaque ligne de texte : un identifiant/tenant non sécable ne peut plus
    // déborder du cadre pointillé — il repasse à la ligne.
    <div
      className={`flex w-full min-w-0 flex-col items-center gap-1 rounded-lg border-2 border-dashed border-black p-3 text-center break-inside-avoid ${className}`}
      style={style}
    >
      <p className="w-full break-words text-sm font-bold leading-tight">{tenantName || "MikCloud"}</p>
      <p className="text-[10px] uppercase tracking-widest text-emerald-700">
        {t("print.wifiHotspot")}
      </p>
      <p className="mt-1 w-full break-words text-xl font-bold font-mono tracking-wider">{voucher.username}</p>
      {/* Mode « mot de passe = identifiant » : le code seul sur le ticket. */}
      {!isSamePasswordMode(voucher) && (
        <p className="w-full break-words font-mono text-sm">
          {t("print.passwordLabel")} {voucher.password}
        </p>
      )}
      <p className="w-full break-words text-xs text-neutral-700">
        {voucher.profileName}
        {validityMin ? ` · ${fmtRouterDuration(validityMin)}` : ""}
        {(voucher.dataQuotaMb ?? 0) > 0
          ? ` · ${formatBytes((voucher.dataQuotaMb ?? 0) * 1048576, effectiveLang)}`
          : ""}
      </p>
      <p className="break-words text-sm font-bold text-emerald-700">
        {formatCurrency(voucher.price, currency, effectiveLang)}
      </p>
      <p className="mt-1 w-full border-t border-neutral-300 pt-1 text-[10px] text-neutral-600">
        {t("print.keepTicket")}
      </p>
    </div>
  );
}
