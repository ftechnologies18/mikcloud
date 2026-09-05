"use client";

// N°27 — WiFi Jetable : page PUBLIQUE du visiteur (scan QR → téléphone →
// code → connexion hotspot → bascule payante 1 clic). Mobile-first, français
// épinglé (audience CI), aucune dépendance à une session console.
// La copie du code utilise le repli execCommand (contextes non sécurisés du
// captive portal). Le téléphone du jour est mémorisé en localStorage :
// re-scan = statut immédiat (idempotence gérée côté backend).

import { useCallback, useEffect, useState } from "react";
import { motion } from "framer-motion";
import {
  ArrowRight,
  Check,
  Copy,
  Loader2,
  RefreshCw,
  Smartphone,
  Wifi,
  WifiOff,
} from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { copyToClipboard } from "@/components/hotspot/parts/uc-clipboard";
import {
  claimWifiCode,
  fetchWifiSiteInfo,
  fetchWifiStatus,
} from "@/lib/hotspot/api";
import { ApiError } from "@/lib/hotspot/api";
import type { WifiOffer, WifiSiteInfo } from "@/lib/hotspot/types";

const FCFA = (n: number) => `${n.toLocaleString("fr-FR")} F`;

function storageKey(slug: string) {
  return `mikcloud-wifi-phone-${slug}`;
}

export function WifiGuestPage({ slug }: { slug: string }) {
  const [info, setInfo] = useState<WifiSiteInfo | null>(null);
  const [loadingInfo, setLoadingInfo] = useState(true);
  const [fatal, setFatal] = useState("");

  const [phone, setPhone] = useState("");
  const [optIn, setOptIn] = useState(true);
  const [claiming, setClaiming] = useState(false);

  const [code, setCode] = useState("");
  const [loginUrl, setLoginUrl] = useState("");
  const [timeMin, setTimeMin] = useState(0);
  const [dataMb, setDataMb] = useState(0);
  const [exhausted, setExhausted] = useState(false);
  const [offers, setOffers] = useState<WifiOffer[]>([]);
  const [copied, setCopied] = useState(false);

  const remember = useCallback(
    (p: string) => {
      try {
        localStorage.setItem(storageKey(slug), p);
      } catch {
        /* stockage indisponible */
      }
    },
    [slug],
  );

  const recall = useCallback((): string => {
    try {
      return localStorage.getItem(storageKey(slug)) ?? "";
    } catch {
      return "";
    }
  }, [slug]);

  // Statut du ticket du jour pour un téléphone donné (re-scan du QR).
  const refreshStatus = useCallback(
    async (p: string) => {
      const st = await fetchWifiStatus(slug, p);
      setOffers(st.offers ?? []);
      if (st.state === "active" && st.code) {
        setCode(st.code);
        setLoginUrl(st.loginUrl ?? "");
        setTimeMin(st.timeLimitMin ?? 0);
        setDataMb(st.dataQuotaMb ?? 0);
        setExhausted(false);
      } else if (st.state === "exhausted") {
        setExhausted(true);
        setCode("");
        setLoginUrl("");
      }
      return st.state;
    },
    [slug],
  );

  useEffect(() => {
    let alive = true;
    (async () => {
      try {
        const i = await fetchWifiSiteInfo(slug);
        if (!alive) return;
        setInfo(i);
        const saved = recall();
        if (saved && i.active && !i.suspended) {
          await refreshStatus(saved).catch(() => undefined);
        }
      } catch {
        if (alive) setFatal("Site WiFi introuvable.");
      } finally {
        if (alive) setLoadingInfo(false);
      }
    })();
    return () => {
      alive = false;
    };
  }, [slug, recall, refreshStatus]);

  const onClaim = async (e: React.FormEvent) => {
    e.preventDefault();
    const digits = phone.replace(/\D/g, "");
    if (digits.length < 8 || digits.length > 15) {
      toast.error("Numéro invalide — incluez l'indicatif (ex. 225 07 07 08 09 09)");
      return;
    }
    setClaiming(true);
    try {
      const res = await claimWifiCode(slug, { phone: digits, optIn });
      remember(digits);
      setCode(res.code);
      setLoginUrl(res.loginUrl);
      setTimeMin(res.timeLimitMin);
      setDataMb(res.dataQuotaMb);
      setExhausted(false);
      setOffers(info?.offers ?? []);
      if (res.duplicate) toast.info("Vous avez déjà votre code pour aujourd'hui");
    } catch (err) {
      if (err instanceof ApiError) {
        if (err.code === "phone_cap") {
          remember(digits);
          const state = await refreshStatus(digits).catch(() => "none");
          if (state === "none") toast.error(err.message);
        } else {
          toast.error(err.message);
        }
      } else {
        toast.error("Connexion au service impossible — réessayez");
      }
    } finally {
      setClaiming(false);
    }
  };

  const onCopy = async () => {
    const ok = await copyToClipboard(code);
    if (ok) {
      setCopied(true);
      toast.success("Code copié");
      setTimeout(() => setCopied(false), 2000);
    }
  };

  // ─── Rendus d'état ───
  if (loadingInfo) {
    return (
      <WifiFrame>
        <div className="flex justify-center py-16" role="status" aria-live="polite">
          <Loader2 className="size-8 animate-spin text-white" aria-hidden="true" />
        </div>
      </WifiFrame>
    );
  }
  if (fatal || !info) {
    return (
      <WifiFrame>
        <Card className="border-white/10 bg-white/95">
          <CardContent className="flex flex-col items-center gap-3 p-8 text-center">
            <WifiOff className="size-10 text-destructive" aria-hidden="true" />
            <p className="font-semibold">{fatal || "Site WiFi introuvable."}</p>
          </CardContent>
        </Card>
      </WifiFrame>
    );
  }
  if (info.suspended) {
    return (
      <WifiFrame>
        <Card className="border-white/10 bg-white/95">
          <CardContent className="flex flex-col items-center gap-3 p-8 text-center">
            <WifiOff className="size-10 text-amber-500" aria-hidden="true" />
            <p className="font-semibold">Service WiFi momentanément suspendu</p>
            <p className="text-sm text-muted-foreground">Contactez le personnel de l&apos;établissement.</p>
          </CardContent>
        </Card>
      </WifiFrame>
    );
  }
  if (!info.active) {
    return (
      <WifiFrame>
        <Card className="border-white/10 bg-white/95">
          <CardContent className="flex flex-col items-center gap-3 p-8 text-center">
            <WifiOff className="size-10 text-amber-500" aria-hidden="true" />
            <p className="font-semibold">WiFi offert en pause</p>
            <p className="text-sm text-muted-foreground">Le WiFi gratuit est désactivé pour le moment.</p>
          </CardContent>
        </Card>
      </WifiFrame>
    );
  }

  const quotaLabel = `${timeMin > 0 ? `${timeMin} min` : info.profileName ?? ""}${
    dataMb > 0 ? ` · ${dataMb} Mo` : ""
  }`;

  return (
    <WifiFrame>
      {/* En-tête branding */}
      <div className="mb-5 flex flex-col items-center gap-2 text-center">
        {info.logoUrl ? (
          <img src={info.logoUrl} alt="" className="h-12 w-auto rounded bg-white/90 object-contain p-1" />
        ) : null}
        <h1 className="bg-gradient-to-r from-emerald-300 to-teal-200 bg-clip-text text-3xl font-black tracking-tight text-transparent">
          WiFi Offert
        </h1>
        <p className="text-sm font-medium text-emerald-100/90">{info.name}</p>
        {info.tenantName ? <p className="text-xs text-emerald-100/60">{info.tenantName}</p> : null}
      </div>

      {code ? (
        // ─── Carte code : révélée après claim / re-scan ───
        <motion.div initial={{ opacity: 0, y: 12 }} animate={{ opacity: 1, y: 0 }}>
          <Card className="border-white/10 bg-white/95">
            <CardContent className="space-y-4 p-6 text-center">
              <p className="text-sm font-medium text-muted-foreground">Votre code WiFi</p>
              <div className="flex items-center justify-center gap-2">
                <p className="select-all font-mono text-4xl font-black tracking-[0.3em] text-emerald-700">
                  {code}
                </p>
              </div>
              {quotaLabel ? (
                <p className="text-xs text-muted-foreground">
                  Offert : {quotaLabel}
                  {info.marketingOptIn ? " · Merci !" : ""}
                </p>
              ) : null}
              <div className="grid gap-2">
                <Button size="lg" className="h-12 text-base" onClick={onCopy}>
                  {copied ? (
                    <Check className="size-5" aria-hidden="true" />
                  ) : (
                    <Copy className="size-5" aria-hidden="true" />
                  )}
                  {copied ? "Copié !" : "Copier le code"}
                </Button>
                {loginUrl ? (
                  <Button size="lg" className="h-12 text-base" onClick={() => window.open(loginUrl, "_blank")}>
                    <Smartphone className="size-5" aria-hidden="true" />
                    Se connecter au WiFi
                    <ArrowRight className="size-5" aria-hidden="true" />
                  </Button>
                ) : null}
              </div>
              {loginUrl ? (
                <p className="text-xs text-muted-foreground">
                  Le portail du hotspot s&apos;ouvre avec votre code pré-rempli — validez et naviguez.
                </p>
              ) : (
                <p className="text-xs text-muted-foreground">
                  Saisissez ce code sur la page de connexion du WiFi.
                </p>
              )}
              <button
                className="mx-auto flex items-center gap-1.5 text-xs text-muted-foreground underline-offset-2 hover:underline"
                onClick={() => {
                  setCode("");
                  setLoginUrl("");
                  setExhausted(false);
                }}
              >
                <RefreshCw className="size-3" aria-hidden="true" /> Voir l&apos;écran d&apos;accueil
              </button>
            </CardContent>
          </Card>
        </motion.div>
      ) : exhausted ? (
        // ─── Bascule 1 clic : quota épuisé → offres payantes ───
        <motion.div initial={{ opacity: 0, y: 12 }} animate={{ opacity: 1, y: 0 }}>
          <Card className="border-white/10 bg-white/95">
            <CardContent className="space-y-4 p-6 text-center">
              <p className="text-base font-semibold">Quota offert épuisé</p>
              <p className="text-sm text-muted-foreground">
                Votre WiFi gratuit du jour est terminé. Passez à une offre payante :
              </p>
              <div className="space-y-2">
                {(offers ?? []).map((o) => (
                  <div
                    key={o.id}
                    className="flex items-center justify-between rounded-xl border bg-muted/40 p-3 text-left"
                  >
                    <div>
                      <p className="font-semibold">{o.name}</p>
                      <p className="text-xs text-muted-foreground">
                        {o.validityMinutes >= 1440
                          ? `${Math.round(o.validityMinutes / 1440)} j`
                          : `${Math.round(o.validityMinutes / 60)} h`}
                        {o.dataQuotaMb > 0 ? ` · ${o.dataQuotaMb} Mo` : ""}
                      </p>
                    </div>
                    <p className="text-lg font-black text-emerald-700">{FCFA(o.price)}</p>
                  </div>
                ))}
                {(!offers || offers.length === 0) && (
                  <p className="text-sm text-muted-foreground">Demandez les tarifs au comptoir.</p>
                )}
              </div>
              <p className="text-xs text-muted-foreground">
                Achetez votre ticket au comptoir, puis connectez-vous avec le code reçu.
              </p>
            </CardContent>
          </Card>
        </motion.div>
      ) : (
        // ─── Formulaire téléphone (marketing opt-in) ───
        <motion.div initial={{ opacity: 0, y: 12 }} animate={{ opacity: 1, y: 0 }}>
          <Card className="border-white/10 bg-white/95">
            <CardContent className="p-6">
              <form className="space-y-4" onSubmit={onClaim}>
                <div className="space-y-1.5">
                  <Label htmlFor="wifi-phone">Votre numéro de téléphone</Label>
                  <Input
                    id="wifi-phone"
                    type="tel"
                    inputMode="tel"
                    autoComplete="tel"
                    placeholder="225 07 07 08 09 09"
                    value={phone}
                    onChange={(e) => setPhone(e.target.value)}
                    className="h-12 text-base"
                  />
                  <p className="text-xs text-muted-foreground">
                    Avec l&apos;indicatif — ex. 225 pour la Côte d&apos;Ivoire.
                  </p>
                </div>
                {info.marketingOptIn ? (
                  <label className="flex items-start gap-2 text-sm" htmlFor="wifi-consent">
                    <input
                      id="wifi-consent"
                      type="checkbox"
                      checked={optIn}
                      onChange={(e) => setOptIn(e.target.checked)}
                      className="mt-0.5 size-4"
                    />
                    <span className="text-muted-foreground">
                      J&apos;accepte de recevoir les offres et actualités de l&apos;établissement
                      (désinscription à tout moment).
                    </span>
                  </label>
                ) : null}
                <Button type="submit" size="lg" className="h-12 w-full text-base" disabled={claiming}>
                  {claiming ? (
                    <Loader2 className="size-5 animate-spin" aria-hidden="true" />
                  ) : (
                    <Wifi className="size-5" aria-hidden="true" />
                  )}
                  Recevoir mon code WiFi
                </Button>
                <p className="text-center text-xs text-muted-foreground">
                  Gratuit dans la limite du quota de l&apos;établissement — au-delà, offres payantes.
                </p>
              </form>
            </CardContent>
          </Card>
        </motion.div>
      )}

      <p className="mt-6 text-center text-[11px] text-white/50">
        Propulsé par MikCloud — WiFi jetable pour restaurants, cafés, salons et maquis.
      </p>
    </WifiFrame>
  );
}

/** Cadre Aurora (dégradé emerald du thème) plein écran, centré, mobile-first. */
function WifiFrame({ children }: { children: React.ReactNode }) {
  return (
    <div
      className="flex min-h-screen flex-col items-center justify-center bg-[#022c22] px-4 py-10"
      style={{
        backgroundImage:
          "radial-gradient(ellipse 80% 50% at 20% -10%, rgba(16,185,129,0.35), transparent), radial-gradient(ellipse 60% 40% at 90% 20%, rgba(45,212,191,0.25), transparent)",
      }}
    >
      <main className="w-full max-w-md">{children}</main>
    </div>
  );
}
