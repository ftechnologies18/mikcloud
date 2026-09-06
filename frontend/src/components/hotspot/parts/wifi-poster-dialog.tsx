"use client";

// N°27 — WiFi Jetable : affiche QR imprimable par site (chevalet de table).
// Réutilise le système d'impression global (.print-area dans globals.css).
//
// N°49 — QR de CONNEXION : le QR encode le RÉSEAU WiFi lui-même
// (format universel « WIFI:T:...;S:...;P:...;; », scanné par l'appareil photo
// iOS 11+ / Android 10+) : le téléphone propose de rejoindre le réseau, le
// portail captif s'ouvre, et le formulaire inline (N°48) prend le relais
// (numéro → code → en ligne). Zéro page intermédiaire, zéro copie de code.
//
// N°49-b — QR UNIQUE : l'ancien QR « page web » (/wifi/{slug}) quitte
// l'affiche — deux QR côte à côte = hésitation au scan. La page /wifi/{slug}
// reste EN LIGNE (affiches déjà imprimées, secours, vitrine). Le repli
// « portail qui ne poppe pas » est repris par une ligne imprimée : le client
// ouvre son navigateur et le hotspot MikroTik redirige vers le portail
// (HTTP non authentifié). Échappement : \ ; , : " backslashés dans S:/P:
// (spec Android).

import { useEffect, useMemo, useState } from "react";
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

/** Échappe les caractères réservés du format WIFI: (\ ; , : "). */
function escWifi(s: string) {
  return s.replace(/([\\;,:"])/g, "\\$1");
}

/** Payload QR de connexion — réseau ouvert (nopass) ou WPA. */
function wifiQrPayload(ssid: string, password: string) {
  const type = password ? "WPA" : "nopass";
  const pass = password ? `;P:${escWifi(password)}` : "";
  return `WIFI:T:${type};S:${escWifi(ssid)}${pass};;`;
}

export function WifiPosterDialog({
  open,
  onOpenChange,
  siteName,
  logoUrl,
  quotaLabel,
  wifiSsid,
  wifiPassword,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  siteName: string;
  logoUrl?: string;
  quotaLabel: string;
  wifiSsid?: string;
  wifiPassword?: string;
}) {
  const ssid = (wifiSsid ?? "").trim();
  // Rendu asynchrone du QR : {payload encodé, data URL}. L'URL affichée n'est
  // valable que si elle correspond AU payload actif (évite le flash d'un QR
  // périmé pendant la régénération).
  const [rendered, setRendered] = useState<{ payload: string; url: string }>({
    payload: "",
    url: "",
  });

  const payload = useMemo(() => {
    if (!open || !ssid) return "";
    return wifiQrPayload(ssid, (wifiPassword ?? "").trim());
  }, [open, ssid, wifiPassword]);

  useEffect(() => {
    if (!payload) return;
    let alive = true;
    QRCode.toDataURL(payload, {
      width: 520,
      margin: 1,
      errorCorrectionLevel: "H",
      color: { dark: "#022c22", light: "#ffffff" },
    })
      .then((url) => {
        if (alive) setRendered({ payload, url });
      })
      .catch(() => {
        if (alive) setRendered({ payload, url: "" });
      });
    return () => {
      alive = false;
    };
  }, [payload]);

  const missingSsid = !ssid;
  const qr = payload && rendered.payload === payload ? rendered.url : "";

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Affiche QR — {siteName}</DialogTitle>
          <DialogDescription>
            Imprimez et posez sur les tables : le client scanne, le WiFi se connecte tout seul.
          </DialogDescription>
        </DialogHeader>
        <div className="print-area rounded-lg bg-white p-4 text-black">
          <div className="flex flex-col items-center gap-3 text-center">
            {logoUrl ? (
              <img src={logoUrl} alt="" className="h-12 w-auto rounded object-contain" />
            ) : null}
            <p className="text-2xl font-black tracking-tight">WiFi Offert</p>
            <p className="text-sm font-medium">{siteName}</p>
            {missingSsid ? (
              // SSID non renseigné : l'affiche de connexion n'est pas générable.
              <div className="flex h-52 w-52 flex-col items-center justify-center gap-2 rounded-lg border border-dashed p-4">
                <p className="text-sm font-semibold">QR de connexion indisponible</p>
                <p className="text-xs text-neutral-500">
                  Renseignez le « SSID du réseau WiFi » dans les réglages de ce site, puis rouvrez l&apos;affiche.
                </p>
              </div>
            ) : qr ? (
              <img src={qr} alt="QR code de connexion WiFi" className="h-52 w-52" />
            ) : (
              <div className="flex h-52 w-52 items-center justify-center">
                <Loader2 className="size-8 animate-spin text-neutral-400" />
              </div>
            )}
            {!missingSsid ? (
              <>
                <p className="max-w-[240px] text-sm font-semibold">
                  Scannez : le WiFi « {ssid} » se connecte tout seul
                </p>
                <p className="text-xs text-neutral-500">
                  La page « WiFi Offert » s&apos;ouvre à l&apos;arrivée — entrez juste votre numéro.
                </p>
                {/* N°49-b — remplace le QR « page web » retiré : si le portail
                    ne poppe pas, ouvrir le navigateur suffit (le hotspot
                    MikroTik redirige le HTTP non authentifié vers le portail). */}
                <p className="text-[10px] text-neutral-400">
                  La page ne s&apos;ouvre pas ? Ouvrez simplement votre navigateur.
                </p>
              </>
            ) : null}
            <p className="text-xs text-neutral-500">{quotaLabel}</p>
          </div>
        </div>
        <DialogFooter>
          <Button onClick={() => window.print()} disabled={missingSsid}>
            <Printer className="size-4" aria-hidden="true" /> Imprimer
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
