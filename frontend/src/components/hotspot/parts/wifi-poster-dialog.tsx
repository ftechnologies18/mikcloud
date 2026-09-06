"use client";

// N°27 — WiFi Jetable : affiche QR imprimable par site (chevalet de table).
// Réutilise le système d'impression global (.print-area dans globals.css).
//
// N°49 — QR de CONNEXION : le QR encode maintenant le RÉSEAU WiFi lui-même
// (format universel « WIFI:T:...;S:...;P:...;; », scanné par l'appareil photo
// iOS 11+ / Android 10+) : le téléphone propose de rejoindre le réseau, le
// portail captif s'ouvre, et le formulaire inline (N°48) prend le relais
// (numéro → code → en ligne). Zéro page intermédiaire, zéro copie de code.
// L'ancien QR « page web » (/wifi/{slug}) reste disponible en second mode :
// il sert de secours (portail qui ne poppe pas) et aux QR déjà imprimés.
// Échappement : \ ; , : " doivent être backslashés dans S:/P: (spec Android).
//
// Le mode initial dépend du SSID au moment de l'OUVERTURE : le parent passe
// une key par site (wifi-view.tsx) pour remonter le composant à chaque choix.

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

type PosterMode = "wifi" | "web";

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
  publicUrl,
  logoUrl,
  quotaLabel,
  wifiSsid,
  wifiPassword,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  siteName: string;
  publicUrl: string;
  logoUrl?: string;
  quotaLabel: string;
  wifiSsid?: string;
  wifiPassword?: string;
}) {
  const ssid = (wifiSsid ?? "").trim();
  const [mode, setMode] = useState<PosterMode>(ssid ? "wifi" : "web");
  // Rendu asynchrone du QR : {payload encodé, data URL}. L'URL affichée n'est
  // valable que si elle correspond AU payload actif (évite le flash d'un QR
  // d'un autre mode pendant la régénération).
  const [rendered, setRendered] = useState<{ payload: string; url: string }>({
    payload: "",
    url: "",
  });

  const payload = useMemo(() => {
    if (!open) return "";
    if (mode === "wifi") return ssid ? wifiQrPayload(ssid, (wifiPassword ?? "").trim()) : "";
    return publicUrl;
  }, [open, mode, ssid, wifiPassword, publicUrl]);

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

  const missingSsid = mode === "wifi" && !ssid;
  const qr = payload && rendered.payload === payload ? rendered.url : "";

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Affiche QR — {siteName}</DialogTitle>
          <DialogDescription>
            Choisissez le mode, imprimez et posez sur les tables : le client scanne et profite du WiFi offert.
          </DialogDescription>
        </DialogHeader>
        {/* Sélecteur de mode (N°49) : connexion directe au réseau vs page web. */}
        <div className="grid grid-cols-2 gap-1 rounded-lg bg-muted p-1" role="tablist" aria-label="Mode du QR code">
          <Button
            size="sm"
            variant={mode === "wifi" ? "default" : "ghost"}
            aria-selected={mode === "wifi"}
            role="tab"
            onClick={() => setMode("wifi")}
          >
            Connexion WiFi
          </Button>
          <Button
            size="sm"
            variant={mode === "web" ? "default" : "ghost"}
            aria-selected={mode === "web"}
            role="tab"
            onClick={() => setMode("web")}
          >
            Page web
          </Button>
        </div>
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
              <img src={qr} alt={mode === "wifi" ? "QR code de connexion WiFi" : "QR code WiFi"} className="h-52 w-52" />
            ) : (
              <div className="flex h-52 w-52 items-center justify-center">
                <Loader2 className="size-8 animate-spin text-neutral-400" />
              </div>
            )}
            {mode === "wifi" && !missingSsid ? (
              <>
                <p className="max-w-[240px] text-sm font-semibold">
                  Scannez : le WiFi « {ssid} » se connecte tout seul
                </p>
                <p className="text-xs text-neutral-500">
                  La page « WiFi Offert » s&apos;ouvre à l&apos;arrivée — entrez juste votre numéro.
                </p>
              </>
            ) : mode === "web" ? (
              <>
                <p className="max-w-[240px] text-sm font-semibold">
                  Scannez, recevez votre code, connectez-vous
                </p>
                <p className="text-[10px] text-neutral-400">{publicUrl}</p>
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
