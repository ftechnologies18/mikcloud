"use client";

// N°27 — WiFi Jetable : affiche QR imprimable par site (chevalet de table).
// Le QR encode l'URL PUBLIQUE /wifi/{slug} — le client passe toujours par la
// capture du téléphone (marketing) avant d'atteindre le hotspot.
// Réutilise le système d'impression global (.print-area dans globals.css).

import { useEffect, useState } from "react";
import QRCode from "qrcode";
import { Loader2, Printer } from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

export function WifiPosterDialog({
  open,
  onOpenChange,
  siteName,
  publicUrl,
  logoUrl,
  quotaLabel,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  siteName: string;
  publicUrl: string;
  logoUrl?: string;
  quotaLabel: string;
}) {
  const [qr, setQr] = useState<string>("");

  useEffect(() => {
    if (!open || !publicUrl) return;
    let alive = true;
    QRCode.toDataURL(publicUrl, {
      width: 520,
      margin: 1,
      errorCorrectionLevel: "H",
      color: { dark: "#022c22", light: "#ffffff" },
    })
      .then((url) => {
        if (alive) setQr(url);
      })
      .catch(() => {
        if (alive) setQr("");
      });
    return () => {
      alive = false;
    };
  }, [open, publicUrl]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Affiche QR — {siteName}</DialogTitle>
          <DialogDescription>
            Imprimez et posez sur les tables : le client scanne, laisse son numéro et reçoit son code.
          </DialogDescription>
        </DialogHeader>
        <div className="print-area rounded-lg bg-white p-4 text-black">
          <div className="flex flex-col items-center gap-3 text-center">
            {logoUrl ? (
              <img src={logoUrl} alt="" className="h-12 w-auto rounded object-contain" />
            ) : null}
            <p className="text-2xl font-black tracking-tight">WiFi Offert</p>
            <p className="text-sm font-medium">{siteName}</p>
            {qr ? (
              <img src={qr} alt="QR code WiFi" className="h-52 w-52" />
            ) : (
              <div className="flex h-52 w-52 items-center justify-center">
                <Loader2 className="size-8 animate-spin text-neutral-400" />
              </div>
            )}
            <p className="max-w-[240px] text-sm font-semibold">
              Scannez, recevez votre code, connectez-vous
            </p>
            <p className="text-xs text-neutral-500">{quotaLabel}</p>
            <p className="text-[10px] text-neutral-400">{publicUrl}</p>
          </div>
        </div>
        <DialogFooter>
          <Button onClick={() => window.print()}>
            <Printer className="size-4" aria-hidden="true" /> Imprimer
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
