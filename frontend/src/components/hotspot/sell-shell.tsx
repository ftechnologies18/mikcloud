"use client";

// N°8 — Mode Vente (PWA revendeur en tournée).
//
// App VOLONTAIREMENT légère et mobile-first, séparée de la console :
// - le revendeur se connecte par identifiant + PIN (token scopé role=reseller,
//   toutes les routes console le refusent en 403) ;
// - stock = vouchers actifs qui lui sont attribués, non remis ;
// - « Vendu » trace la remise au client (SoldAt/SoldVia → audit anti-vol) ;
// - « Partager » envoie code + mot de passe via Web Share (WhatsApp) ou presse-papiers ;
// - hors ligne : bannière d'état — aucune vente offline fantôme (phase 1).
import { useEffect, useState } from "react";
import Image from "next/image";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { BadgeCheck, Loader2, LogOut, RefreshCw, Share2, ShoppingCart, Store, Wifi, WifiOff } from "lucide-react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { api, ApiError } from "@/lib/hotspot/api";
import { useI18n } from "@/lib/hotspot/i18n";
import { formatCurrency } from "@/lib/hotspot/format";
import { useHotspotStore } from "@/lib/hotspot/store";

interface SellVoucher {
  id: string;
  username: string;
  password: string;
  profileName: string;
  price: number;
  sellingPrice: number;
  dataQuotaMb: number;
  expiresAt: string;
  routerName: string;
  createdAt: string;
}

interface SellMe {
  name: string;
  username: string;
  credit: number;
  stockCount: number;
  soldToday: number;
  revenueToday: number;
  currency: string;
}

function useOnline(): boolean {
  const [online, setOnline] = useState(true);
  useEffect(() => {
    const sync = () => setOnline(navigator.onLine);
    sync();
    window.addEventListener("online", sync);
    window.addEventListener("offline", sync);
    return () => {
      window.removeEventListener("online", sync);
      window.removeEventListener("offline", sync);
    };
  }, []);
  return online;
}

export default function SellShell() {
  const { t, tf, lang } = useI18n();
  const qc = useQueryClient();
  const logout = useHotspotStore((s) => s.logout);
  const online = useOnline();

  const { data: me } = useQuery({
    queryKey: ["/api/sell/me"],
    queryFn: () => api<SellMe>("/api/sell/me"),
    refetchInterval: 30_000,
  });

  const { data: stock, isLoading, refetch, isRefetching } = useQuery({
    queryKey: ["/api/sell/stock"],
    queryFn: () => api<SellVoucher[]>("/api/sell/stock"),
    refetchInterval: 30_000,
  });

  const sell = useMutation({
    mutationFn: (id: string) => api<{ ok: boolean }>(`/api/sell/${id}/sold`, { method: "POST" }),
    onSuccess: () => {
      toast.success(t("sell.soldToast"));
      qc.invalidateQueries({ queryKey: ["/api/sell/stock"] });
      qc.invalidateQueries({ queryKey: ["/api/sell/me"] });
    },
    onError: (e: Error) => toast.error(e instanceof ApiError ? e.message : t("sell.error")),
  });

  const currency = me?.currency || "FCFA";

  async function share(v: SellVoucher) {
    const price = v.sellingPrice || v.price;
    const text = tf("sell.shareText", {
      profile: v.profileName,
      code: v.username,
      pass: v.password,
      price: formatCurrency(price, currency, lang),
    });
    try {
      if (navigator.share) {
        await navigator.share({ title: "MikCloud", text });
      } else {
        await navigator.clipboard.writeText(text);
        toast.success(t("sell.copied"));
      }
    } catch {
      /* partage annulé par l'utilisateur */
    }
  }

  return (
    <div className="mx-auto flex min-h-dvh w-full max-w-lg flex-col bg-background">
      {/* En-tête revendeur */}
      <header className="sticky top-0 z-10 border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/80">
        <div className="flex items-center gap-3 px-4 py-3">
          <Image src="/logo.png" alt="MikCloud" width={36} height={36} className="rounded-lg" priority />
          <div className="min-w-0 flex-1">
            <p className="truncate text-sm font-semibold">{me?.name ?? "…"}</p>
            <p className="text-xs text-muted-foreground">
              {t("sell.mode")} · {t("sell.credit")} {me ? formatCurrency(me.credit, currency, lang) : "—"}
            </p>
          </div>
          <Button
            size="icon"
            variant="ghost"
            onClick={() => refetch()}
            aria-label={t("common.refresh")}
            disabled={isRefetching}
          >
            <RefreshCw className={`size-4 ${isRefetching ? "animate-spin" : ""}`} />
          </Button>
          <Button
            size="icon"
            variant="ghost"
            className="text-destructive hover:text-destructive"
            onClick={logout}
            aria-label={t("shell.logout")}
          >
            <LogOut className="size-4" />
          </Button>
        </div>

        {/* Stats du jour */}
        <div className="grid grid-cols-3 gap-px border-t bg-border">
          <div className="bg-background px-3 py-2 text-center">
            <p className="text-lg font-bold tabular-nums">{me?.stockCount ?? "—"}</p>
            <p className="text-[11px] text-muted-foreground">{t("sell.stock")}</p>
          </div>
          <div className="bg-background px-3 py-2 text-center">
            <p className="text-lg font-bold tabular-nums">{me?.soldToday ?? "—"}</p>
            <p className="text-[11px] text-muted-foreground">{t("sell.soldToday")}</p>
          </div>
          <div className="bg-background px-3 py-2 text-center">
            <p className="text-lg font-bold text-primary tabular-nums">
              {me ? formatCurrency(me.revenueToday, currency, lang) : "—"}
            </p>
            <p className="text-[11px] text-muted-foreground">{t("sell.revenueToday")}</p>
          </div>
        </div>
      </header>

      {/* État réseau */}
      <div
        className={`flex items-center justify-center gap-2 px-4 py-1.5 text-xs ${online ? "text-muted-foreground" : "bg-amber-500/15 text-amber-600 dark:text-amber-400"}`}
        role="status"
      >
        {online ? <Wifi className="size-3" /> : <WifiOff className="size-3" />}
        {online ? t("sell.online") : t("sell.offline")}
      </div>

      {/* Stock */}
      <main className="flex-1 space-y-3 p-4" aria-label={t("sell.stock")}>
        {isLoading ? (
          Array.from({ length: 4 }).map((_, i) => <Skeleton key={i} className="h-28 w-full" />)
        ) : !stock || stock.length === 0 ? (
          <Card>
            <CardContent className="flex flex-col items-center gap-3 p-8 text-center">
              <span className="flex size-12 items-center justify-center rounded-xl bg-primary/10 text-primary">
                <Store className="size-6" />
              </span>
              <p className="font-medium">{t("sell.empty")}</p>
              <p className="text-sm text-muted-foreground">{t("sell.emptyDesc")}</p>
            </CardContent>
          </Card>
        ) : (
          stock.map((v) => {
            const price = v.sellingPrice || v.price;
            return (
              <Card key={v.id} className="gap-0 py-0">
                <CardContent className="p-4">
                  <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0">
                      <div className="flex items-center gap-2">
                        <p className="truncate font-semibold">{v.profileName}</p>
                        {v.dataQuotaMb > 0 && (
                          <Badge variant="secondary" className="text-[10px]">
                            {Math.round(v.dataQuotaMb / 1024)} Go
                          </Badge>
                        )}
                      </div>
                      <p className="mt-0.5 text-xs text-muted-foreground">
                        {v.routerName} · {t("sell.expires")}{" "}
                        {new Date(v.expiresAt).toLocaleDateString(lang === "en" ? "en-GB" : "fr-FR", {
                          day: "2-digit",
                          month: "short",
                        })}
                      </p>
                    </div>
                    <p className="shrink-0 text-lg font-bold text-primary tabular-nums">
                      {formatCurrency(price, currency, lang)}
                    </p>
                  </div>

                  <div className="mt-3 grid grid-cols-2 gap-2 rounded-lg bg-muted/50 p-3 font-mono text-sm">
                    <div>
                      <p className="text-[10px] tracking-wide text-muted-foreground uppercase">{t("sell.code")}</p>
                      <p className="mt-0.5 font-semibold">{v.username}</p>
                    </div>
                    <div>
                      <p className="text-[10px] tracking-wide text-muted-foreground uppercase">{t("sell.password")}</p>
                      <p className="mt-0.5 font-semibold">{v.password}</p>
                    </div>
                  </div>

                  <div className="mt-3 flex gap-2">
                    <Button className="flex-1" onClick={() => sell.mutate(v.id)} disabled={sell.isPending}>
                      {sell.isPending && sell.variables === v.id ? (
                        <Loader2 className="size-4 animate-spin" />
                      ) : (
                        <BadgeCheck className="size-4" />
                      )}
                      {t("sell.sellBtn")}
                    </Button>
                    <Button variant="outline" size="icon" onClick={() => void share(v)} aria-label={t("sell.share")}>
                      <Share2 className="size-4" />
                    </Button>
                  </div>
                </CardContent>
              </Card>
            );
          })
        )}
      </main>

      <footer className="mt-auto border-t px-4 py-3 text-center text-[11px] text-muted-foreground">
        <ShoppingCart className="mr-1 inline size-3" />
        {t("sell.footer")}
      </footer>
    </div>
  );
}
