"use client";

// Dialog d'impression des vouchers — tickets noirs sur fond blanc dans une zone .print-area
// (globals.css masque tout le reste à l'impression, .no-print cache la barre d'outils).

import { Printer } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogTitle } from "@/components/ui/dialog";
import { useCurrency } from "@/components/hotspot/parts/sd-currency";
import { formatCurrency } from "@/lib/hotspot/format";
import type { HotspotUser, Profile } from "@/lib/hotspot/types";

interface UcPrintDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  vouchers: HotspotUser[];
  title: string;
  description?: string;
  tenantName: string;
  profiles: Profile[];
}

export function UcPrintDialog({
  open,
  onOpenChange,
  vouchers,
  title,
  description,
  tenantName,
  profiles,
}: UcPrintDialogProps) {
  const currency = useCurrency();

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="gap-4 sm:max-w-3xl">
        <div className="no-print flex flex-wrap items-start justify-between gap-3">
          <div className="min-w-0">
            <DialogTitle className="truncate">{title}</DialogTitle>
            <DialogDescription>
              {description ?? `${vouchers.length} ticket${vouchers.length > 1 ? "s" : ""} prêt${
                vouchers.length > 1 ? "s" : ""
              } à imprimer — découpez le long des cadres.`}
            </DialogDescription>
          </div>
          <div className="flex shrink-0 items-center gap-2">
            <Button variant="outline" onClick={() => onOpenChange(false)}>
              Fermer
            </Button>
            <Button onClick={() => window.print()}>
              <Printer className="size-4" />
              Imprimer
            </Button>
          </div>
        </div>

        <div className="max-h-[65vh] overflow-y-auto print:max-h-none print:overflow-visible">
          <div className="print-area rounded-lg bg-white p-4 text-black">
            <div className="grid grid-cols-2 gap-5 sm:grid-cols-3">
              {vouchers.map((voucher) => {
                const validityDays = profiles.find((p) => p.id === voucher.profileId)?.validityDays;
                return (
                  <div
                    key={voucher.id}
                    className="flex flex-col items-center gap-1 rounded-lg border-2 border-dashed border-black p-3 text-center break-inside-avoid"
                  >
                    <p className="text-sm font-bold leading-tight">{tenantName || "SpotCloud"}</p>
                    <p className="text-[10px] uppercase tracking-widest text-neutral-500">WiFi Hotspot</p>
                    <p className="mt-1 text-lg font-bold font-mono tracking-wider">{voucher.username}</p>
                    <p className="font-mono text-sm">Mot de passe : {voucher.password}</p>
                    <p className="text-xs text-neutral-700">
                      {voucher.profileName}
                      {validityDays ? ` · validité ${validityDays} j` : ""}
                    </p>
                    <p className="text-xs font-semibold">{formatCurrency(voucher.price, currency)}</p>
                    <p className="mt-1 w-full border-t border-neutral-300 pt-1 text-[10px] text-neutral-500">
                      Gardez ce ticket pour vous connecter
                    </p>
                  </div>
                );
              })}
            </div>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
