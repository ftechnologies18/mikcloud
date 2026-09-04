"use client";

// N°8 — Mode Vente (PWA revendeur en tournée).
//
// App VOLONTAIREMENT légère et mobile-first, séparée de la console :
// - le revendeur se connecte par identifiant + PIN (token scopé role=reseller,
//   toutes les routes console le refusent en 403) ;
// - stock = vouchers actifs qui lui sont attribués, non remis ;
// - « Vendu » trace la remise au client (SoldAt/SoldVia → audit anti-vol) ;
// - « Partager » envoie code + mot de passe via Web Share (WhatsApp) ou presse-papiers ;
// - hors ligne : bannière d'état — aucune vente offline fantôme (phase 1) ;
// - UX R3 : recherche code/profil/lot, badge « expire bientôt » (< 48 h),
//   sélection d'un lot entier en retour, et vente des tickets papier déjà
//   imprimés — le code saisi « connecte » le ticket papier : décompte du
//   stock + même traçabilité qu'une vente tactile (confirmation obligatoire).
import { useEffect, useMemo, useState, useSyncExternalStore } from "react";
import Image from "next/image";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  BadgeCheck,
  CheckCheck,
  CheckCircle2,
  ChevronDown,
  Circle,
  FileBarChart,
  Layers,
  Loader2,
  LogOut,
  RefreshCw,
  Search,
  Share2,
  ShoppingCart,
  Store,
  Ticket,
  Undo2,
  Wifi,
  WifiOff,
  X,
} from "lucide-react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { api, ApiError } from "@/lib/hotspot/api";
import { useI18n } from "@/lib/hotspot/i18n";
import { formatCurrency } from "@/lib/hotspot/format";
import type { SellDayReport } from "@/lib/hotspot/types";
import { isSamePasswordMode } from "@/components/hotspot/parts/template-render";
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
  /** UX R1 — lot d'origine (peut être absent sur les données historiques). */
  batchId?: string;
}

interface SellMe {
  name: string;
  username: string;
  credit: number;
  stockCount: number;
  soldToday: number;
  revenueToday: number;
  currency: string;
  /** N°19 — dépôt-vente : « à verser » remplace le crédit. */
  paymentMode?: string;
  debt?: number;
  debtCeiling?: number;
}

/** N°20 — réponse du retour de stock (POST /api/sell/return). */
interface SellReturnResult {
  returned: number;
  credited: number;
  creditAfter: number;
  codes: string[];
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


// UX R1 — regroupement du stock : profil → lot. Purement présentationnel (la
// référence de lot est déjà tracée sur chaque voucher à la génération) —
// aucune route ni entité nouvelle, l'app revendeur reste légère.
const NO_BATCH = "__nobatch__";
const VIEW_STORAGE_KEY = "mikcloud-sell-view";

interface SellBatchGroup {
  key: string;
  labelId: string;
  createdAt: string; // date de génération du lot (la plus ancienne du groupe)
  vouchers: SellVoucher[];
}

interface SellProfileGroup {
  profileName: string;
  count: number;
  value: number; // valeur faciale cumulée (sellingPrice || price)
  batches: SellBatchGroup[]; // lot le plus régent en tête
  hasLots: boolean; // au moins un lot identifié → sous-groupes affichés
}

/** UX R1 — vue du stock persistée en localStorage : store externe lu via
 * useSyncExternalStore (pas de setState en effet — règle react-hooks), SSR
 * sûr (snapshot serveur = « profile »), synchro inter-onglets gratuite. */
function subscribeView(callback: () => void) {
  window.addEventListener("mikcloud-view-change", callback);
  window.addEventListener("storage", callback);
  return () => {
    window.removeEventListener("mikcloud-view-change", callback);
    window.removeEventListener("storage", callback);
  };
}

function getViewSnapshot(): "profile" | "recent" {
  try {
    return window.localStorage.getItem(VIEW_STORAGE_KEY) === "recent" ? "recent" : "profile";
  } catch {
    return "profile";
  }
}

function getServerViewSnapshot(): "profile" | "recent" {
  return "profile";
}

function shortBatchId(id: string): string {
  return id.split("-").pop() || id;
}

// UX R3 — un ticket dont la validité se termine dans moins de 48 h mérite un
// signal visuel : à vendre en priorité, ou à rendre au stock avant qu'il ne
// meure (un voucher expiré sort du stock sans recyclage possible).
const EXPIRY_SOON_MS = 48 * 60 * 60 * 1000;

function expiresSoon(v: SellVoucher): boolean {
  if (!v.expiresAt) return false;
  const ms = new Date(v.expiresAt).getTime() - Date.now();
  return ms > 0 && ms <= EXPIRY_SOON_MS;
}

function batchExpiringSoon(vouchers: SellVoucher[]): boolean {
  return vouchers.some(expiresSoon);
}

function fmtDay(iso: string, lang: string): string {
  const d = new Date(iso.length === 10 ? `${iso}T12:00:00Z` : iso);
  return Number.isNaN(d.getTime())
    ? ""
    : d.toLocaleDateString(lang === "en" ? "en-GB" : "fr-FR", { day: "2-digit", month: "short" });
}

function groupStock(stock: SellVoucher[]): SellProfileGroup[] {
  const byProfile = new Map<string, SellVoucher[]>();
  for (const v of stock) {
    const list = byProfile.get(v.profileName);
    if (list) list.push(v);
    else byProfile.set(v.profileName, [v]);
  }
  const groups: SellProfileGroup[] = [];
  for (const [profileName, vouchers] of byProfile) {
    const byBatch = new Map<string, SellBatchGroup>();
    for (const v of vouchers) {
      const key = v.batchId || NO_BATCH;
      let b = byBatch.get(key);
      if (!b) {
        b = { key, labelId: v.batchId ? shortBatchId(v.batchId) : "", createdAt: v.createdAt, vouchers: [] };
        byBatch.set(key, b);
      }
      if (v.createdAt < b.createdAt) b.createdAt = v.createdAt;
      b.vouchers.push(v);
    }
    const batches = [...byBatch.values()].sort((a, b) => b.createdAt.localeCompare(a.createdAt));
    for (const b of batches) b.vouchers.sort((a, c) => c.createdAt.localeCompare(a.createdAt));
    groups.push({
      profileName,
      count: vouchers.length,
      value: vouchers.reduce((sum, v) => sum + (v.sellingPrice || v.price), 0),
      batches,
      hasLots: batches.some((b) => b.key !== NO_BATCH),
    });
  }
  // Logique de comptoir : le profil avec le plus de tickets en tête (ordre
  // stable par nom à égalité) — le vendeur retrouve d'abord son best-seller.
  return groups.sort((a, b) => b.count - a.count || a.profileName.localeCompare(b.profileName));
}

export default function SellShell() {
  const { t, tf, lang } = useI18n();
  const qc = useQueryClient();
  const logout = useHotspotStore((s) => s.logout);
  const online = useOnline();
  const [reportOpen, setReportOpen] = useState(false);
  // N°20 — retour de stock : mode sélection → confirmation → POST /api/sell/return.
  const [returnMode, setReturnMode] = useState(false);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [returnConfirmOpen, setReturnConfirmOpen] = useState(false);
  // UX R1 — vue du stock : « profile » (regroupé profil → lot, défaut) ou
  // « recent » (liste plate historique, plus récents d'abord) — persistée.
  const view = useSyncExternalStore(subscribeView, getViewSnapshot, getServerViewSnapshot);
  const [collapsedGroups, setCollapsedGroups] = useState<Set<string>>(new Set());
  // UX R2 — ticket en attente de confirmation : un misclick ne doit pas
  // marquer un ticket « vendu » (trace anti-vol SoldAt immuable, créance
  // dépôt-vente créée immédiatement — la vente est définitive par design).
  const [pendingSale, setPendingSale] = useState<SellVoucher | null>(null);
  // UX R3 — origine de la vente en attente : papier (code saisi) ou tactile.
  const [pendingVia, setPendingVia] = useState<"paper" | undefined>(undefined);
  // UX R3 — recherche locale (code, profil, référence de lot) : filtre la
  // liste affichée, sans nouvelle requête (le stock est déjà chargé).
  const [query, setQuery] = useState("");
  // UX R3 — vente d'un ticket papier imprimé : le code tapé retrouve le
  // voucher dans le stock actif, puis passe par la confirmation R2.
  const [physicalCode, setPhysicalCode] = useState("");

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

  // Rapport de fin de journée — chargé uniquement quand le dialog est ouvert.
  const { data: report, isLoading: reportLoading } = useQuery({
    queryKey: ["/api/sell/day-report"],
    queryFn: () => api<SellDayReport>("/api/sell/day-report"),
    enabled: reportOpen,
  });

  const sell = useMutation({
    // UX R3 — via=paper : la vente d'un ticket papier imprimé est tracée
    // à part (SoldVia=sell_mode_paper) ; la vente tactile POSTe sans corps.
    mutationFn: ({ id, via }: { id: string; via?: "paper" }) =>
      api<{ ok: boolean }>(`/api/sell/${id}/sold`, { method: "POST", body: via ? { via } : undefined }),
    onSuccess: () => {
      toast.success(t("sell.soldToast"));
      setPendingSale(null);
      setPendingVia(undefined);
      setPhysicalCode(""); // UX R3 — le code papier saisi est consommé
      qc.invalidateQueries({ queryKey: ["/api/sell/stock"] });
      qc.invalidateQueries({ queryKey: ["/api/sell/me"] });
    },
    onError: (e: Error) => toast.error(e instanceof ApiError ? e.message : t("sell.error")),
  });

  // N°20 — retour de stock : les tickets choisis repartent dans le stock
  // direct du compte (gérant OU propriétaire — le backend ne dépend pas d'un
  // gérant ; prépayé : portefeuille recrédité du prix gros, dépôt-vente : stock seul).
  const returnStock = useMutation({
    // NB : `api()` sérialise déjà le corps en JSON — passer l'objet brut.
    // (Un `JSON.stringify` ici produisait un corps doublement encodé — une
    // chaîne JSON au lieu d'un objet — refusé par le backend en 400
    // « Corps de requête invalide ». Cause du bug remonté par Ulrich.)
    mutationFn: (ids: string[]) =>
      api<SellReturnResult>("/api/sell/return", { method: "POST", body: { ids } }),
    onSuccess: (res) => {
      if (res.credited > 0) {
        toast.success(tf("sell.returnDoneCreditToast", { count: res.returned, amount: formatCurrency(res.credited, currency, lang) }));
      } else {
        toast.success(tf("sell.returnDoneToast", { count: res.returned }));
      }
      setReturnConfirmOpen(false);
      setReturnMode(false);
      setSelected(new Set());
      qc.invalidateQueries({ queryKey: ["/api/sell/stock"] });
      qc.invalidateQueries({ queryKey: ["/api/sell/me"] });
      qc.invalidateQueries({ queryKey: ["/api/sell/day-report"] });
    },
    onError: (e: Error) => toast.error(e instanceof ApiError ? e.message : t("sell.error")),
  });

  // UX R1 — regroupement memoïsé : recalculé uniquement au refetch du stock.
  const groupedStock = useMemo(() => (stock ? groupStock(stock) : []), [stock]);

  // UX R3 — recherche : filtre la liste plate et regroupe la sélection (mêmes
  // groupes profil → lot, seuls les lots/tickets correspondants restent).
  const searching = query.trim().length > 0;
  const filteredStock = useMemo(() => {
    if (!stock) return [];
    const needle = query.trim().toLowerCase();
    if (!needle) return stock;
    return stock.filter(
      (v) =>
        v.username.toLowerCase().includes(needle) ||
        v.profileName.toLowerCase().includes(needle) ||
        (v.batchId ? shortBatchId(v.batchId).toLowerCase().includes(needle) : false),
    );
  }, [stock, query]);
  const filteredGroups = useMemo(
    () => (searching ? groupStock(filteredStock) : groupedStock),
    [searching, filteredStock, groupedStock],
  );

  const currency = me?.currency || "FCFA";
  const isDeposit = me?.paymentMode === "deposit";
  const hasStock = !!stock && stock.length > 0;
  // Valeur GROSSISTE de la sélection (u.Price — ce qui est recrédité en prépayé).
  const selectedVouchers = (stock ?? []).filter((v) => selected.has(v.id));
  const selectedWholesale = selectedVouchers.reduce((sum, v) => sum + v.price, 0);

  // UX R1 — bascule de vue : écriture localStorage + notification des
  // abonnés (même onglet — « storage » ne se déclenche que dans les autres).
  function changeView(v: "profile" | "recent") {
    try {
      window.localStorage.setItem(VIEW_STORAGE_KEY, v);
    } catch {
      /* stockage indisponible — vue de session uniquement */
    }
    window.dispatchEvent(new Event("mikcloud-view-change"));
  }

  // UX R2 — la vente n'est déclenchée qu'après confirmation explicite ; en
  // cas d'erreur réseau la dialog reste ouverte (relance sans retaper).
  function confirmSale() {
    if (pendingSale) sell.mutate({ id: pendingSale.id, via: pendingVia });
  }

  // UX R3 — ticket papier « connecté » : le revendeur a imprimé des tickets
  // de son stock ; quand il en remet un au client, il tape le code imprimé —
  // le voucher est retrouvé dans SON stock actif et suit exactement le même
  // chemin qu'une vente tactile (confirmation R2, trace SoldAt, créance
  // dépôt-vente). Code inexistant/vendu/expiré → refus explicite : jamais de
  // décompte fantôme, le stock ne baisse qu'à la vente réellement confirmée.
  function sellPhysical() {
    const code = physicalCode.trim();
    if (!code) return;
    const hit = (stock ?? []).find((v) => v.username.toLowerCase() === code.toLowerCase());
    if (!hit) {
      toast.error(t("sell.physicalNotFound"));
      return;
    }
    setPendingVia("paper");
    setPendingSale(hit);
  }

  function toggleGroup(key: string) {
    setCollapsedGroups((prev) => {
      const next = new Set(prev);
      if (next.has(key)) {
        next.delete(key);
      } else {
        next.add(key);
      }
      return next;
    });
  }

  function toggleReturnMode() {
    setReturnMode((on) => !on);
    setSelected(new Set());
    setReturnConfirmOpen(false);
  }

  function toggleSelected(id: string) {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
      }
      return next;
    });
  }

  // UX R3 — mode retour : tout un lot en un geste (les tickets d'un lot
  // expirent ensemble — les rendre un par un n'a pas de sens au comptoir).
  function toggleBatchSelection(b: SellBatchGroup) {
    setSelected((prev) => {
      const next = new Set(prev);
      const allIn = b.vouchers.every((v) => next.has(v.id));
      for (const v of b.vouchers) {
        if (allIn) next.delete(v.id);
        else next.add(v.id);
      }
      return next;
    });
  }

  // Clôture : le rapport textuel se partage (WhatsApp) ou se copie — le
  // revendeur l'envoie au gérant en fin de tournée.
  async function shareReport() {
    if (!report) return;
    const locale = lang === "en" ? "en-GB" : "fr-FR";
    // T12:00:00Z évite que le fuseau local décale la journée métier (UTC).
    const dateLabel = new Date(`${report.date}T12:00:00Z`).toLocaleDateString(locale, {
      weekday: "long",
      day: "2-digit",
      month: "long",
      year: "numeric",
    });
    const timeHM = (iso: string) =>
      new Date(iso).toLocaleTimeString(locale, { hour: "2-digit", minute: "2-digit" });
    const lines = [
      tf("sell.dayReportTextHeader", { date: dateLabel }),
      me ? `${me.name} (${me.username})` : "",
      `${t("sell.dayReportSold")} : ${report.soldCount} — ${formatCurrency(report.revenue, currency, lang)}`,
      // N°19 V2 — dépôt-vente : le rapport annonce le versement attendu.
      ...(report.paymentMode === "deposit"
        ? [
            tf("sell.dayReportToDepositText", { amount: formatCurrency(report.toDeposit ?? 0, currency, lang) }),
            tf("sell.dayReportDebtText", { amount: formatCurrency(report.debtTotal ?? 0, currency, lang) }),
          ]
        : []),
      `${t("sell.dayReportStock")} : ${report.stockCount}`,
      t("sell.dayReportDetail"),
      ...report.sold.map(
        (s) => `• ${timeHM(s.soldAt)} · ${s.code} · ${s.profileName} — ${formatCurrency(s.price, currency, lang)}`,
      ),
    ].filter(Boolean);
    const text = lines.join("\n");
    try {
      if (navigator.share) {
        await navigator.share({ title: "MikCloud", text });
      } else {
        await navigator.clipboard.writeText(text);
        toast.success(t("sell.dayReportShared"));
      }
    } catch {
      /* partage annulé par l'utilisateur */
    }
  }

  async function share(v: SellVoucher) {
    const price = v.sellingPrice || v.price;
    // Mode « mot de passe = identifiant » : le partage ne mentionne que le code.
    const text = isSamePasswordMode(v)
      ? tf("sell.shareTextCodeOnly", {
          profile: v.profileName,
          code: v.username,
          price: formatCurrency(price, currency, lang),
        })
      : tf("sell.shareText", {
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

  // UX R1 — carte ticket (identique dans les deux vues : groupée et plate).
  const renderVoucherCard = (v: SellVoucher) => {
    const price = v.sellingPrice || v.price;
    const isSelected = selected.has(v.id);
    return (
      <Card
        key={v.id}
        className={`gap-0 py-0 transition-shadow ${returnMode && isSelected ? "ring-2 ring-primary" : ""}`}
      >
        <CardContent
          className={`p-4 ${returnMode ? "cursor-pointer select-none" : ""}`}
          onClick={returnMode ? () => toggleSelected(v.id) : undefined}
          role={returnMode ? "checkbox" : undefined}
          aria-checked={returnMode ? isSelected : undefined}
          tabIndex={returnMode ? 0 : undefined}
          onKeyDown={
            returnMode
              ? (e) => {
                  if (e.key === " " || e.key === "Enter") {
                    e.preventDefault();
                    toggleSelected(v.id);
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

          <div className={`mt-3 grid gap-2 rounded-lg bg-muted/50 p-3 font-mono text-sm ${isSamePasswordMode(v) ? "grid-cols-1" : "grid-cols-2"}`}>
            <div>
              <p className="text-[10px] tracking-wide text-muted-foreground uppercase">{t("sell.code")}</p>
              <p className="mt-0.5 font-semibold">{v.username}</p>
            </div>
            {/* Mode « mot de passe = identifiant » : le code seul. */}
            {!isSamePasswordMode(v) && (
              <div>
                <p className="text-[10px] tracking-wide text-muted-foreground uppercase">{t("sell.password")}</p>
                <p className="mt-0.5 font-semibold">{v.password}</p>
              </div>
            )}
          </div>

          {/* N°20 — en mode retour : pas de vente/partage (anti-misclick). */}
          {!returnMode && (
            <div className="mt-3 flex gap-2">
              <Button
                className="flex-1"
                onClick={() => {
                  setPendingVia(undefined);
                  setPendingSale(v);
                }}
                disabled={sell.isPending}
              >
                {sell.isPending && sell.variables?.id === v.id ? (
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
          )}
        </CardContent>
      </Card>
    );
  };

  return (
    <div className="mx-auto flex min-h-dvh w-full max-w-lg flex-col bg-background">
      {/* En-tête revendeur */}
      <header className="sticky top-0 z-10 border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/80">
        <div className="flex items-center gap-3 px-4 py-3">
          <Image src="/logo.png" alt="MikCloud" width={36} height={36} className="rounded-lg" priority />
          <div className="min-w-0 flex-1">
            <p className="truncate text-sm font-semibold">{me?.name ?? "…"}</p>
            <p className="text-xs text-muted-foreground">
              {t("sell.mode")} ·{" "}
              {me?.paymentMode === "deposit" ? (
                <>
                  {t("sell.toDeposit")}{" "}
                  <span className={(me.debt ?? 0) > 0 ? "font-semibold text-amber-600 dark:text-amber-400" : ""}>
                    {formatCurrency(me.debt ?? 0, currency, lang)}
                  </span>
                </>
              ) : (
                <>
                  {t("sell.credit")} {me ? formatCurrency(me.credit, currency, lang) : "—"}
                </>
              )}
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

      {/* État réseau + clôture de journée */}
      <div
        className={`flex items-center justify-center gap-2 px-4 py-1.5 text-xs ${online ? "text-muted-foreground" : "bg-amber-500/15 text-amber-600 dark:text-amber-400"}`}
        role="status"
      >
        {online ? <Wifi className="size-3" /> : <WifiOff className="size-3" />}
        {online ? t("sell.online") : t("sell.offline")}
      </div>

      {/* Clôture de journée + retour de stock (N°20) */}
      <div className="grid grid-cols-2 gap-px border-b bg-border">
        <button
          type="button"
          onClick={() => setReportOpen(true)}
          className="flex min-h-11 items-center justify-center gap-2 bg-muted/30 px-3 text-sm font-medium text-muted-foreground transition-colors hover:bg-muted/60 hover:text-foreground"
        >
          <FileBarChart className="size-4" aria-hidden />
          {t("sell.dayReport")}
        </button>
        <button
          type="button"
          onClick={toggleReturnMode}
          disabled={!hasStock}
          aria-pressed={returnMode}
          className={`flex min-h-11 items-center justify-center gap-2 px-3 text-sm font-medium transition-colors disabled:opacity-50 ${
            returnMode
              ? "bg-primary text-primary-foreground"
              : "bg-muted/30 text-muted-foreground hover:bg-muted/60 hover:text-foreground"
          }`}
        >
          {returnMode ? <X className="size-4" aria-hidden /> : <Undo2 className="size-4" aria-hidden />}
          {t("sell.returnBtn")}
        </button>
      </div>
      {returnMode && (
        <p className="bg-primary/5 px-4 py-1.5 text-center text-xs text-muted-foreground" role="status">
          {t("sell.returnSelectHint")}
        </p>
      )}

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
          <>
            {/* UX R1 — barre du stock : comptage + bascule de vue. */}
            <div className="flex items-center justify-between gap-3">
              <p className="text-sm text-muted-foreground">
                {tf("sell.stockCountLabel", { count: searching ? filteredStock.length : stock.length })}
              </p>
              <div className="flex rounded-lg border bg-muted/30 p-0.5" role="group" aria-label={t("sell.viewLabel")}>
                <button
                  type="button"
                  onClick={() => changeView("profile")}
                  aria-pressed={view === "profile"}
                  className={`min-h-9 rounded-md px-3 text-xs font-medium transition-colors ${
                    view === "profile" ? "bg-background text-foreground shadow-sm" : "text-muted-foreground hover:text-foreground"
                  }`}
                >
                  {t("sell.viewProfile")}
                </button>
                <button
                  type="button"
                  onClick={() => changeView("recent")}
                  aria-pressed={view === "recent"}
                  className={`min-h-9 rounded-md px-3 text-xs font-medium transition-colors ${
                    view === "recent" ? "bg-background text-foreground shadow-sm" : "text-muted-foreground hover:text-foreground"
                  }`}
                >
                  {t("sell.viewRecent")}
                </button>
              </div>
            </div>

            {/* UX R3 — recherche locale : code, profil ou référence de lot.
                Les groupes correspondants se déplient automatiquement. */}
            <div className="relative">
              <Search aria-hidden className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                type="text"
                inputMode="search"
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder={t("sell.searchPlaceholder")}
                aria-label={t("sell.searchPlaceholder")}
                autoCapitalize="none"
                autoCorrect="off"
                spellCheck={false}
                className="h-10 pl-9 pr-9"
              />
              {searching && (
                <button
                  type="button"
                  onClick={() => setQuery("")}
                  aria-label={t("sell.searchClear")}
                  className="absolute right-1.5 top-1/2 -translate-y-1/2 rounded-md p-1.5 text-muted-foreground transition-colors hover:text-foreground"
                >
                  <X className="size-4" aria-hidden />
                </button>
              )}
            </div>

            {/* UX R3 — ticket papier déjà imprimé : la saisie du code
                « connecte » le ticket papier au système — le stock se décompte
                exactement comme une vente tactile (confirmation incluse). */}
            {!returnMode && (
              <section aria-label={t("sell.physicalTitle")} className="rounded-xl border border-dashed bg-muted/20 p-3">
                <div className="flex items-center gap-2">
                  <Ticket aria-hidden className="size-4 shrink-0 text-primary" />
                  <p className="text-sm font-semibold">{t("sell.physicalTitle")}</p>
                </div>
                <p className="mt-1 text-xs text-muted-foreground">{t("sell.physicalDesc")}</p>
                <form
                  className="mt-2 flex gap-2"
                  onSubmit={(e) => {
                    e.preventDefault();
                    sellPhysical();
                  }}
                >
                  <Input
                    value={physicalCode}
                    onChange={(e) => setPhysicalCode(e.target.value)}
                    placeholder={t("sell.physicalPlaceholder")}
                    aria-label={t("sell.physicalPlaceholder")}
                    autoCapitalize="none"
                    autoCorrect="off"
                    spellCheck={false}
                    className="h-10 min-w-0 flex-1 font-mono"
                  />
                  <Button type="submit" variant="outline" className="h-10 shrink-0" disabled={sell.isPending}>
                    <BadgeCheck className="size-4" aria-hidden />
                    {t("sell.sellBtn")}
                  </Button>
                </form>
              </section>
            )}

            {searching && filteredStock.length === 0 ? (
              <Card>
                <CardContent className="p-6 text-center text-sm text-muted-foreground">
                  {t("sell.searchNoResult")}
                </CardContent>
              </Card>
            ) : view === "recent" ? (
              filteredStock.map((v) => renderVoucherCard(v))
            ) : (
              filteredGroups.map((g) => {
                const groupKey = `profil:${g.profileName}`;
                // UX R3 — en recherche, tous les groupes correspondants sont
                // dépliés (le filtre remplace l'état de repliage mémorisé).
                const isCollapsed = !searching && collapsedGroups.has(groupKey);
                return (
                  <section key={g.profileName} aria-label={g.profileName} className="space-y-3">
                    <button
                      type="button"
                      onClick={() => toggleGroup(groupKey)}
                      aria-expanded={!isCollapsed}
                      className="flex min-h-11 w-full items-center gap-2 rounded-xl border bg-muted/30 px-3 py-2 text-left transition-colors hover:bg-muted/60"
                    >
                      <ChevronDown
                        aria-hidden
                        className={`size-4 shrink-0 text-muted-foreground transition-transform ${isCollapsed ? "-rotate-90" : ""}`}
                      />
                      <span className="min-w-0 flex-1">
                        <span className="block truncate text-sm font-semibold">{g.profileName}</span>
                        <span className="block text-[11px] text-muted-foreground">
                          {tf("sell.groupMeta", { lots: g.batches.length })} · {formatCurrency(g.value, currency, lang)}
                        </span>
                      </span>
                      <Badge variant="secondary" className="shrink-0 tabular-nums">
                        {g.count}
                      </Badge>
                    </button>

                    {!isCollapsed &&
                      (g.hasLots ? (
                        g.batches.map((b) => {
                          const batchAllIn = returnMode && b.vouchers.every((v) => selected.has(v.id));
                          return (
                            <div key={b.key} className="space-y-3">
                              <div className="flex items-center gap-1.5 px-1">
                                <Layers aria-hidden className="size-3 shrink-0 text-muted-foreground" />
                                <p className="min-w-0 flex-1 truncate text-[11px] font-medium uppercase tracking-wide text-muted-foreground">
                                  {b.key === NO_BATCH ? t("sell.lotNone") : tf("sell.lotLabel", { id: b.labelId })} ·{" "}
                                  {fmtDay(b.createdAt, lang)} · {tf("sell.lotCount", { count: b.vouchers.length })}
                                  {batchExpiringSoon(b.vouchers) && (
                                    <span className="text-amber-600 dark:text-amber-400">
                                      {" "}· {t("sell.lotExpiring")}
                                    </span>
                                  )}
                                </p>
                                {returnMode && (
                                  <button
                                    type="button"
                                    onClick={() => toggleBatchSelection(b)}
                                    aria-label={t("sell.lotSelectAllAria")}
                                    aria-pressed={batchAllIn}
                                    className={`flex min-h-8 shrink-0 items-center gap-1 rounded-md border px-2 text-[11px] font-medium transition-colors ${
                                      batchAllIn
                                        ? "border-primary bg-primary text-primary-foreground"
                                        : "bg-background text-muted-foreground hover:bg-muted/60 hover:text-foreground"
                                    }`}
                                  >
                                    <CheckCheck className="size-3.5" aria-hidden />
                                    {t("sell.lotSelectAll")}
                                  </button>
                                )}
                              </div>
                              {b.vouchers.map((v) => renderVoucherCard(v))}
                            </div>
                          );
                        })
                      ) : (
                        g.batches.flatMap((b) => b.vouchers).map((v) => renderVoucherCard(v))
                      ))}
                  </section>
                );
              })
            )}
          </>
        )}
      </main>

      {/* N°20 — barre d'action du mode retour (sticky au-dessus du footer). */}
      {returnMode && selected.size > 0 && (
        <div className="sticky bottom-0 z-10 border-t bg-background/95 px-4 py-3 backdrop-blur supports-[backdrop-filter]:bg-background/80">
          <div className="mb-2 flex items-center justify-between gap-3">
            <p className="text-sm font-medium">{tf("sell.returnSelected", { count: selected.size })}</p>
            <p className="text-sm text-muted-foreground">
              {t("sell.returnWholesale")} :{" "}
              <span className="font-semibold text-foreground tabular-nums">
                {formatCurrency(selectedWholesale, currency, lang)}
              </span>
            </p>
          </div>
          <div className="flex gap-2">
            <Button variant="outline" className="flex-1" onClick={toggleReturnMode}>
              {t("common.cancel")}
            </Button>
            <Button
              className="flex-1"
              onClick={() => setReturnConfirmOpen(true)}
              disabled={returnStock.isPending}
            >
              <Undo2 className="size-4" />
              {t("sell.returnAction")}
            </Button>
          </div>
        </div>
      )}

      <footer className="mt-auto border-t px-4 py-3 text-center text-[11px] text-muted-foreground">
        <ShoppingCart className="mr-1 inline size-3" />
        {t("sell.footer")}
      </footer>

      {/* Rapport de fin de journée */}
      <Dialog open={reportOpen} onOpenChange={setReportOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <FileBarChart className="size-4 text-primary" aria-hidden />
              {t("sell.dayReport")}
            </DialogTitle>
            <DialogDescription>
              {report
                ? new Date(`${report.date}T12:00:00Z`).toLocaleDateString(lang === "en" ? "en-GB" : "fr-FR", {
                    weekday: "long",
                    day: "2-digit",
                    month: "long",
                    year: "numeric",
                  })
                : t("sell.dayReportDesc")}
            </DialogDescription>
          </DialogHeader>

          {reportLoading || !report ? (
            <div className="space-y-2">
              <div className="grid grid-cols-3 gap-2">
                {Array.from({ length: 3 }).map((_, i) => (
                  <Skeleton key={i} className="h-16 rounded-lg" />
                ))}
              </div>
              <Skeleton className="h-40 rounded-lg" />
            </div>
          ) : (
            <>
              <div className="grid grid-cols-3 gap-2">
                <div className="rounded-lg border bg-muted/30 p-3 text-center">
                  <p className="text-lg font-bold tabular-nums">{report.soldCount}</p>
                  <p className="text-[11px] text-muted-foreground">{t("sell.dayReportSold")}</p>
                </div>
                <div className="rounded-lg border bg-muted/30 p-3 text-center">
                  <p className="text-lg font-bold text-primary tabular-nums">
                    {formatCurrency(report.revenue, currency, lang)}
                  </p>
                  <p className="text-[11px] text-muted-foreground">{t("sell.dayReportRevenue")}</p>
                </div>
                <div className="rounded-lg border bg-muted/30 p-3 text-center">
                  <p className="text-lg font-bold tabular-nums">{report.stockCount}</p>
                  <p className="text-[11px] text-muted-foreground">{t("sell.dayReportStock")}</p>
                </div>
              </div>

              {report.paymentMode === "deposit" && (
                <div className="rounded-lg border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-sm">
                  <p className="font-medium text-amber-700 dark:text-amber-300">
                    {tf("sell.dayReportToDepositText", { amount: formatCurrency(report.toDeposit ?? 0, currency, lang) })}
                  </p>
                  <p className="text-xs text-amber-600/80 dark:text-amber-400/80">
                    {tf("sell.dayReportDebtText", { amount: formatCurrency(report.debtTotal ?? 0, currency, lang) })}
                  </p>
                </div>
              )}

              <div className="max-h-64 overflow-y-auto rounded-lg border" aria-label={t("sell.dayReportDetail")}>
                {report.sold.length === 0 ? (
                  <p className="px-3 py-8 text-center text-sm text-muted-foreground">{t("sell.dayReportEmpty")}</p>
                ) : (
                  report.sold.map((s) => (
                    <div
                      key={s.id}
                      className="flex items-center justify-between gap-3 border-b px-3 py-2 last:border-b-0"
                    >
                      <div className="min-w-0">
                        <p className="truncate font-mono text-sm font-semibold">{s.code}</p>
                        <p className="truncate text-xs text-muted-foreground">
                          {s.profileName} ·{" "}
                          {new Date(s.soldAt).toLocaleTimeString(lang === "en" ? "en-GB" : "fr-FR", {
                            hour: "2-digit",
                            minute: "2-digit",
                          })}
                        </p>
                      </div>
                      <p className="shrink-0 text-sm font-semibold text-primary tabular-nums">
                        {formatCurrency(s.price, currency, lang)}
                      </p>
                    </div>
                  ))
                )}
              </div>
            </>
          )}

          <DialogFooter>
            <Button variant="outline" onClick={() => setReportOpen(false)}>
              {t("common.close")}
            </Button>
            <Button onClick={() => void shareReport()} disabled={reportLoading || !report}>
              <Share2 className="size-4" />
              {t("sell.dayReportShare")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* UX R2 — confirmation de vente : récapitulatif du ticket (code,
          profil, prix) + action définitive explicite. En cas d'erreur réseau
          la dialog reste ouverte pour relancer sans re-sélectionner. */}
      <Dialog
        open={pendingSale !== null}
        onOpenChange={(open) => {
          if (!open && !sell.isPending) setPendingSale(null);
        }}
      >
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <BadgeCheck className="size-4 text-primary" aria-hidden />
              {t("sell.sellConfirmTitle")}
            </DialogTitle>
            <DialogDescription>{t("sell.sellConfirmDesc")}</DialogDescription>
          </DialogHeader>

          {pendingSale && (
            <>
              <div className="rounded-lg border bg-muted/30 p-3">
                <div className="flex items-center justify-between gap-3">
                  <p className="font-semibold">{pendingSale.profileName}</p>
                  <p className="shrink-0 font-bold text-primary tabular-nums">
                    {formatCurrency(pendingSale.sellingPrice || pendingSale.price, currency, lang)}
                  </p>
                </div>
                <p className="mt-1 font-mono text-sm text-muted-foreground">{pendingSale.username}</p>
              </div>

              <DialogFooter>
                <Button variant="outline" onClick={() => setPendingSale(null)} disabled={sell.isPending}>
                  {t("common.cancel")}
                </Button>
                <Button onClick={confirmSale} disabled={sell.isPending}>
                  {sell.isPending ? <Loader2 className="size-4 animate-spin" /> : <BadgeCheck className="size-4" />}
                  {t("sell.sellConfirmAction")}
                </Button>
              </DialogFooter>
            </>
          )}
        </DialogContent>
      </Dialog>

      {/* N°20 — confirmation du retour de stock. */}
      <Dialog open={returnConfirmOpen} onOpenChange={setReturnConfirmOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <Undo2 className="size-4 text-primary" aria-hidden />
              {t("sell.returnConfirmTitle")}
            </DialogTitle>
            <DialogDescription>
              {isDeposit
                ? tf("sell.returnConfirmDescDeposit", { count: selected.size })
                : tf("sell.returnConfirmDescPrepaid", {
                    count: selected.size,
                    amount: formatCurrency(selectedWholesale, currency, lang),
                  })}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setReturnConfirmOpen(false)} disabled={returnStock.isPending}>
              {t("common.cancel")}
            </Button>
            <Button
              onClick={() => returnStock.mutate([...selected])}
              disabled={returnStock.isPending || selected.size === 0}
            >
              {returnStock.isPending ? <Loader2 className="size-4 animate-spin" /> : <Undo2 className="size-4" />}
              {t("sell.returnAction")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
