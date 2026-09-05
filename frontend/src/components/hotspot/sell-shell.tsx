"use client";

// N°8 — Mode Vente (PWA revendeur en tournée).
//
// App VOLONTAIREMENT légère et mobile-first, séparée de la console :
// - le revendeur se connecte par identifiant + PIN (token scopé role=reseller,
//   toutes les routes console le refusent en 403) ;
// - stock = vouchers actifs qui lui sont attribués, non remis ;
// - « Vendu » trace la remise au client (SoldAt/SoldVia → audit anti-vol) ;
// - ANTI-FUITE : le code d'un ticket en stock est masqué — il ne devient
//   visible ni partageable (Web Share / presse-papiers) qu'APRÈS
//   confirmation de la vente, via le reçu « Vente confirmée » ;
// - hors ligne : bannière d'état — aucune vente offline fantôme (phase 1) ;
// - UX R3 : recherche code/profil/lot, badge « expire bientôt » (< 48 h),
//   sélection d'un lot entier en retour, et vente des tickets papier déjà
//   imprimés — le code saisi « connecte » le ticket papier : décompte du
//   stock + même traçabilité qu'une vente tactile (confirmation obligatoire).
import { useCallback, useEffect, useMemo, useRef, useState, useSyncExternalStore } from "react";
import Image from "next/image";
import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
  type InfiniteData,
} from "@tanstack/react-query";
import {
  ArrowLeftRight,
  BadgeCheck,
  CheckCheck,
  CheckCircle2,
  ChevronDown,
  Circle,
  CloudUpload,
  Download,
  FileBarChart,
  Layers,
  Loader2,
  LogOut,
  MousePointerClick,
  Printer,
  RefreshCw,
  Search,
  Share2,
  ShoppingCart,
  Store,
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
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { api, apiDownload, ApiError } from "@/lib/hotspot/api";
import {
  SellPrintDialog,
  type SellPrintScope,
} from "@/components/hotspot/parts/sell-print-dialog";
import { useI18n, tf as tfLang } from "@/lib/hotspot/i18n";
import { formatCurrency } from "@/lib/hotspot/format";
import {
  listQueuedSales,
  queueSale,
  removeQueuedSale,
  type QueuedSale,
} from "@/lib/hotspot/offline-queue";
import type { SellDayReport, SellVia } from "@/lib/hotspot/types";
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
  /** Impression revendeur — validité du profil en minutes (ticket papier). */
  validityMin?: number;
}

/** P3-e — une page de stock (réponse de /api/sell/stock?limit=…).
 * Sans `limit`, l'endpoint renvoie le tableau historique complet. */
interface StockPage {
  items: SellVoucher[];
  total: number;
  hasMore: boolean;
}

const STOCK_PAGE_SIZE = 60;

/** Phase D × P3-e — update optimiste du stock PAGINÉ : le filtre traverse
 * toutes les pages chargées (InfiniteData) ; la vue aplatie puis l'effet
 * snapshot persistent ensuite le localStorage — les DEUX caches (TanStack +
 * hors-ligne) restent alignés, sinon le fallback hors-ligne ré-afficherait
 * un ticket déjà vendu/rendu. */
function filterPagedStock(
  paged: InfiniteData<StockPage>,
  ids: ReadonlySet<string>,
): InfiniteData<StockPage> {
  return {
    ...paged,
    pages: paged.pages.map((p) => ({
      ...p,
      items: p.items.filter((v) => !ids.has(v.id)),
    })),
  };
}

interface SellMe {
  name: string;
  username: string;
  credit: number;
  stockCount: number;
  soldToday: number;
  revenueToday: number;
  currency: string;
  /** Impression revendeur — branding des tickets (nom du hotspot / portail). */
  tenantName?: string;
  dnsName?: string;
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

/** N°21 — pair de transfert : revendeur actif du même compte (identité minimale). */
interface SellPeer {
  id: string;
  name: string;
}

/** N°21 — réponse du transfert entre revendeurs (POST /api/sell/transfer). */
interface SellTransferResult {
  transferred: number;
  credited: number;
  creditAfter: number;
  debited: number;
  target: SellPeer;
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

// UX R6 (P3-a) — ventes hors-ligne : snapshot du stock et du profil
// revendeur en localStorage à chaque fetch réussi. Hors-ligne, la vue
// continue d'afficher le dernier état connu (sinon, impossible de vendre :
// la liste serait vide) — la file IndexedDB prend le relais côté ventes.
const STOCK_CACHE_KEY = "mikcloud-stock-cache";
const ME_CACHE_KEY = "mikcloud-me-cache";

function readCache<T>(key: string): T | null {
  try {
    const raw = window.localStorage.getItem(key);
    return raw ? (JSON.parse(raw) as T) : null;
  } catch {
    return null;
  }
}

function writeCache(key: string, data: unknown): void {
  try {
    window.localStorage.setItem(key, JSON.stringify(data));
  } catch {
    /* quota dépassé / stockage indisponible — le cache est un confort, pas une garantie */
  }
}

// UX R6 — une erreur « réseau » (fetch TypeError, ou 502/503/504 de la
// passerelle quand le backend est injoignable) peut partir en file locale ;
// une erreur HTTP métier (409 « déjà remis », 401 session…) reste une erreur
// affichée — on ne met jamais en file ce que le serveur a réellement refusé.
function isNetworkError(e: unknown): boolean {
  if (!(e instanceof ApiError)) return true; // TypeError fetch = réseau
  return e.status === 502 || e.status === 503 || e.status === 504;
}

function expiresSoon(v: SellVoucher): boolean {
  if (!v.expiresAt) return false;
  const ms = new Date(v.expiresAt).getTime() - Date.now();
  return ms > 0 && ms <= EXPIRY_SOON_MS;
}

function batchExpiringSoon(vouchers: SellVoucher[]): boolean {
  return vouchers.some(expiresSoon);
}

// P3-d — canaux de vente (audit R4) : clé i18n + icône du canal. Une vente
// sans canal tracé (historique pré-R4) était nécessairement tactile.
const VIA_ORDER = ["sell_mode", "auto_connect", "sell_mode_paper"] as const;
const VIA_KEYS: Record<string, string> = {
  sell_mode: "sell.viaTactile",
  auto_connect: "sell.viaAuto",
  sell_mode_paper: "sell.viaPaper",
};

function viaIcon(via?: string) {
  if (via === "auto_connect") return Wifi;
  if (via === "sell_mode_paper") return Printer;
  return MousePointerClick;
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
  // N°21 — fusion retour/transfert : la destination se choisit dans le dialog
  // de confirmation — « manager » (retour de stock historique) ou l'id d'un
  // pair (transfert entre revendeurs). Réinitialisée à chaque ouverture.
  const [dest, setDest] = useState("manager");
  // Impression revendeur — portée courante : tout le stock ou un seul lot
  // (le dialog relit le stock complet avant impression). printSession est un
  // nonce : chaque ouverture REMONTE le dialog → stock toujours relu à neuf
  // (un ticket vendu entre deux ouvertures ne réapparaît pas).
  const [printScope, setPrintScope] = useState<SellPrintScope | null>(null);
  const [printSession, setPrintSession] = useState(0);
  const openPrint = (scope: SellPrintScope) => {
    setPrintScope(scope);
    setPrintSession((n) => n + 1);
  };
  // UX R1 — vue du stock : « profile » (regroupé profil → lot, défaut) ou
  // « recent » (liste plate historique, plus récents d'abord) — persistée.
  const view = useSyncExternalStore(subscribeView, getViewSnapshot, getServerViewSnapshot);
  const [collapsedGroups, setCollapsedGroups] = useState<Set<string>>(new Set());
  // UX R2 — ticket en attente de confirmation : un misclick ne doit pas
  // marquer un ticket « vendu » (trace anti-vol SoldAt immuable, créance
  // dépôt-vente créée immédiatement — la vente est définitive par design).
  const [pendingSale, setPendingSale] = useState<SellVoucher | null>(null);
  // Anti-fuite — reçu de vente : la vente vient d'être confirmée (ou mise en
  // file hors-ligne) ; c'est LE moment où le code devient visible et
  // partageable. Tant qu'une vente n'est pas confirmée, aucun code ne
  // s'affiche nulle part dans le comptoir — plus aucun partage anticipé
  // susceptible de contourner la trace SoldAt/créance.
  const [receipt, setReceipt] = useState<{ voucher: SellVoucher; offline: boolean } | null>(null);
  // UX R3 — recherche locale (code, profil, référence de lot) : filtre la
  // liste affichée, sans nouvelle requête (le stock est déjà chargé).
  const [query, setQuery] = useState("");

  const { data: me } = useQuery({
    queryKey: ["/api/sell/me"],
    // UX R6 — hors-ligne : dernier snapshot connu (localStorage) plutôt qu'un
    // écran vide ; retry: false — le refetch 30 s suffit au retour du réseau.
    queryFn: async () => {
      try {
        const data = await api<SellMe>("/api/sell/me", { timeoutMs: 10_000 });
        writeCache(ME_CACHE_KEY, data);
        return data;
      } catch (e) {
        const cached = readCache<SellMe>(ME_CACHE_KEY);
        if (cached && isNetworkError(e)) return cached;
        throw e;
      }
    },
    refetchInterval: 30_000,
    retry: false,
  });

  // P3-e — stock paginé (useInfiniteQuery) : le comptoir ne charge que la
  // première page de tickets (60) puis « Afficher plus » ; un gros stock ne
  // plombe plus le premier rendu ni le payload mobile. Le contrat backend est
  // additif (sans `limit` → tableau historique) ; hors-ligne, le snapshot
  // localStorage sert de page unique (hasMore=false) — le comptoir reste
  // vendable sans couverture.
  const {
    data: stockPages,
    isLoading,
    refetch,
    isRefetching,
    fetchNextPage,
    hasNextPage,
    isFetchingNextPage,
  } = useInfiniteQuery({
    queryKey: ["/api/sell/stock", "paged"],
    queryFn: async ({ pageParam }) => {
      const offset = pageParam as number;
      try {
        const data = await api<StockPage>("/api/sell/stock", {
          params: { limit: STOCK_PAGE_SIZE, offset },
          timeoutMs: 10_000,
        });
        return data;
      } catch (e) {
        // UX R6 — fallback hors-ligne : uniquement sur la première page (le
        // snapshot sert d'état complet ; on ne fabrique jamais une page 2).
        if (offset === 0 && isNetworkError(e)) {
          const cached = readCache<SellVoucher[]>(STOCK_CACHE_KEY);
          if (cached) return { items: cached, total: cached.length, hasMore: false };
        }
        throw e;
      }
    },
    initialPageParam: 0,
    getNextPageParam: (last, pages) =>
      last.hasMore ? pages.reduce((n, p) => n + p.items.length, 0) : undefined,
    refetchInterval: 30_000,
    retry: false,
  });

  // Vue aplatie : tout le reste du composant (groupes, recherche, sélection)
  // consomme le même tableau qu'avant — la pagination est un détail de charge.
  const stock = useMemo(
    () => (stockPages ? stockPages.pages.flatMap((p) => p.items) : undefined),
    [stockPages],
  );

  // UX R6 — snapshot hors-ligne : l'état complet chargé (toutes pages) est
  // persisté à chaque changement, pas seulement la première page.
  useEffect(() => {
    if (stock) writeCache(STOCK_CACHE_KEY, stock);
  }, [stock]);

  // Rapport de fin de journée — chargé uniquement quand le dialog est ouvert.
  const { data: report, isLoading: reportLoading } = useQuery({
    queryKey: ["/api/sell/day-report"],
    queryFn: () => api<SellDayReport>("/api/sell/day-report"),
    enabled: reportOpen,
  });

  // N°21 — pairs de transfert : revendeurs actifs du même compte, chargés
  // uniquement quand le dialog de confirmation est ouvert. Identité minimale
  // (id + nom) — aucune donnée financière d'un pair n'est exposée ici.
  const { data: peers } = useQuery({
    queryKey: ["/api/sell/peers"],
    queryFn: () => api<SellPeer[]>("/api/sell/peers", { timeoutMs: 10_000 }),
    enabled: returnConfirmOpen,
    retry: false,
  });
  // Garde de cohérence : si la destination choisie disparaît (pair désactivé
  // ou supprimé pendant la sélection), on revient au retour gérant — jamais
  // d'action sur une destination fantôme.
  useEffect(() => {
    if (returnConfirmOpen && dest !== "manager" && peers && !peers.some((p) => p.id === dest)) {
      setDest("manager");
    }
  }, [returnConfirmOpen, dest, peers]);
  const destName = (peers ?? []).find((p) => p.id === dest)?.name ?? "";

  // P3-d — export comptable « journal de caisse » (CSV Excel, téléchargement
  // authentifié via apiDownload — le lien direct ne porterait pas le token).
  const [exporting, setExporting] = useState(false);
  async function exportJournal() {
    if (!report || exporting) return;
    setExporting(true);
    try {
      await apiDownload(
        "/api/sell/day-report.csv",
        `journal-caisse-${report.date}-${me?.username || "revendeur"}.csv`,
      );
      toast.success(t("sell.dayReportExportToast"));
    } catch (e) {
      toast.error(e instanceof ApiError ? e.message : t("sell.error"));
    } finally {
      setExporting(false);
    }
  }

  // UX R6 — miroir de la file IndexedDB pour le rendu (bannière + chips).
  const [queued, setQueued] = useState<QueuedSale[]>([]);
  const refreshQueue = useCallback(() => {
    void listQueuedSales().then(setQueued);
  }, []);
  useEffect(() => {
    refreshQueue();
  }, [refreshQueue]);

  const sell = useMutation({
    // UX R5/R6 — la vente POSTe sans corps (SoldVia=sell_mode). Erreur
    // réseau → la vente part en file locale (IndexedDB) et sera REJOUÉE au
    // retour du réseau (409-safe, cf. replay plus bas) ; erreur métier →
    // R2 : la dialog reste ouverte, on ne file pas un refus du serveur.
    // Phase D (UI optimiste) — le ticket quitte le stock AFFICHÉ dès le
    // clic : les DEUX caches sont mis à jour (TanStack + snapshots
    // localStorage UX R6) — sinon, hors-ligne, l'invalidation retomberait
    // sur le snapshot qui contient ENCORE le ticket vendu. CA/crédit ne
    // sont PAS devinés (sémantique différente selon prépayé/dépôt-vente) :
    // servis par l'invalidation (ou le replay) au retour du réseau.
    mutationFn: async ({ id, voucher }: { id: string; voucher: SellVoucher }) => {
      try {
        await api<{ ok: boolean }>(`/api/sell/${id}/sold`, { method: "POST", timeoutMs: 10_000 });
        return { offline: false };
      } catch (e) {
        if (!isNetworkError(e)) throw e;
        await queueSale({
          voucherId: id,
          username: voucher.username,
          profileName: voucher.profileName,
          price: voucher.sellingPrice || voucher.price,
          queuedAt: new Date().toISOString(),
        });
        return { offline: true };
      }
    },
    onMutate: async ({ id }) => {
      await Promise.all([
        qc.cancelQueries({ queryKey: ["/api/sell/stock"] }),
        qc.cancelQueries({ queryKey: ["/api/sell/me"] }),
      ]);
      // Phase D × P3-e — le stock vit en pages (InfiniteData sous
      // ["/api/sell/stock","paged"]) : l'update optimiste filtre toutes les
      // pages ; le snapshot hors-ligne suit via l'effet sur la vue aplatie
      // (source de vérité unique — pas d'écriture manuelle du snapshot).
      const paged = qc.getQueryData<InfiniteData<StockPage>>(["/api/sell/stock", "paged"]);
      const me = qc.getQueryData<SellMe>(["/api/sell/me"]);
      const nextPaged = paged ? filterPagedStock(paged, new Set([id])) : undefined;
      const nextMe = me
        ? { ...me, stockCount: Math.max(0, me.stockCount - 1), soldToday: me.soldToday + 1 }
        : undefined;
      if (nextPaged) qc.setQueryData(["/api/sell/stock", "paged"], nextPaged);
      if (nextMe) qc.setQueryData<SellMe>(["/api/sell/me"], nextMe);
      return { paged, me };
    },
    onSuccess: (res, vars) => {
      if (res.offline) toast.info(t("sell.queuedToast"));
      else toast.success(t("sell.soldToast"));
      setPendingSale(null);
      // Anti-fuite : le code n'est révélé qu'ici, vente tracée (ou file
      // hors-ligne — la sync est un détail technique, la remise au client
      // est déjà décidée par le revendeur).
      setReceipt({ voucher: vars.voucher, offline: res.offline });
      refreshQueue();
      qc.invalidateQueries({ queryKey: ["/api/sell/stock"] });
      qc.invalidateQueries({ queryKey: ["/api/sell/me"] });
    },
    onError: (e: Error, _vars, ctx) => {
      // Erreur métier uniquement (une erreur réseau part en file dans
      // mutationFn) : restauration du cache paginé — le snapshot hors-ligne
      // se réécrit seul via l'effet sur la vue aplatie.
      if (ctx?.paged) qc.setQueryData(["/api/sell/stock", "paged"], ctx.paged);
      if (ctx?.me) qc.setQueryData(["/api/sell/me"], ctx.me);
      toast.error(e instanceof ApiError ? e.message : t("sell.error"));
    },
  });

  // UX R6 — replay de la file : au retour du réseau (useOnline réagit aux
  // événements online/offline → l'effet re-court), au montage, puis toutes
  // les 60 s tant qu'il reste des ventes en file. Le backend est idempotent
  // (409 « déjà remis ») : un replay ne peut jamais doubler un décompte — un
  // 409 signifie que la vente a déjà été tracée par un autre chemin, ou que
  // le ticket a été rendu/expiré entre-temps : l'entrée est retirée de la
  // file, jamais de décompte fantôme.
  const replayingRef = useRef(false);
  useEffect(() => {
    if (!online) return;
    let cancelled = false;
    const run = async () => {
      if (replayingRef.current) return;
      replayingRef.current = true;
      try {
        const q = await listQueuedSales();
        for (const item of q) {
          if (cancelled) return;
          try {
            await api<{ ok: boolean }>(`/api/sell/${item.voucherId}/sold`, {
              method: "POST",
              timeoutMs: 10_000,
            });
            await removeQueuedSale(item.voucherId);
            toast.success(tfLang(useHotspotStore.getState().lang, "sell.syncSoldToast", { code: item.username }));
          } catch (e) {
            // 409 : déjà vendu (autre chemin) ou ticket rendu/expiré —
            // 404 : ticket inexistant (données nettoyées côté gérant).
            // Dans les deux cas la file est résolue sans décompte fantôme.
            if (e instanceof ApiError && (e.status === 409 || e.status === 404)) {
              await removeQueuedSale(item.voucherId);
              toast.info(
                tfLang(useHotspotStore.getState().lang, "sell.syncConflictToast", { code: item.username }),
              );
            } else {
              return; // réseau/session toujours indisponible — prochain tick
            }
          }
        }
      } finally {
        replayingRef.current = false;
      }
      if (cancelled) return;
      setQueued(await listQueuedSales());
      qc.invalidateQueries({ queryKey: ["/api/sell/stock"] });
      qc.invalidateQueries({ queryKey: ["/api/sell/me"] });
    };
    void run();
    const iv = setInterval(() => {
      if (!cancelled) void run();
    }, 60_000);
    return () => {
      cancelled = true;
      clearInterval(iv);
    };
  }, [online, qc]);

  // N°21 — sorties de stock (retour N°20 + transfert entre revendeurs) : la
  // mise à jour optimiste est partagée — les tickets quittent le stock affiché
  // dès la confirmation ; le crédit éventuel (prépayé) est patché avec la
  // valeur réelle calculée par le serveur (onSuccess), jamais devinée.
  async function outboundOptimistic(ids: string[]) {
    await Promise.all([
      qc.cancelQueries({ queryKey: ["/api/sell/stock"] }),
      qc.cancelQueries({ queryKey: ["/api/sell/me"] }),
    ]);
    const paged = qc.getQueryData<InfiniteData<StockPage>>(["/api/sell/stock", "paged"]);
    const me = qc.getQueryData<SellMe>(["/api/sell/me"]);
    if (paged) qc.setQueryData(["/api/sell/stock", "paged"], filterPagedStock(paged, new Set(ids)));
    if (me) {
      qc.setQueryData<SellMe>(["/api/sell/me"], {
        ...me,
        stockCount: Math.max(0, me.stockCount - ids.length),
      });
    }
    return { paged, me };
  }

  function outboundRollback(ctx?: { paged?: InfiniteData<StockPage>; me?: SellMe }) {
    if (ctx?.paged) qc.setQueryData(["/api/sell/stock", "paged"], ctx.paged);
    if (ctx?.me) qc.setQueryData(["/api/sell/me"], ctx.me);
  }

  function outboundFinish() {
    setReturnConfirmOpen(false);
    setReturnMode(false);
    setSelected(new Set());
    qc.invalidateQueries({ queryKey: ["/api/sell/stock"] });
    qc.invalidateQueries({ queryKey: ["/api/sell/me"] });
    qc.invalidateQueries({ queryKey: ["/api/sell/day-report"] });
  }

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
    onMutate: (ids) => outboundOptimistic(ids),
    onSuccess: (res, _ids, ctx) => {
      if (res.credited > 0) {
        toast.success(tf("sell.returnDoneCreditToast", { count: res.returned, amount: formatCurrency(res.credited, currency, lang) }));
        if (ctx?.me) {
          qc.setQueryData<SellMe>(["/api/sell/me"], { ...ctx.me, credit: ctx.me.credit + res.credited });
        }
      } else {
        toast.success(tf("sell.returnDoneToast", { count: res.returned }));
      }
      outboundFinish();
    },
    onError: (e: Error, _ids, ctx) => {
      outboundRollback(ctx);
      toast.error(e instanceof ApiError ? e.message : t("sell.error"));
    },
  });

  // N°21 — transfert de stock entre revendeurs : même sélection, destination
  // un pair (revendeur actif du même compte). L'argent suit le stock à prix
  // gros : prépayé → portefeuille recrédité du prix facial ; dépôt-vente →
  // aucun mouvement pour vous (la créance du pair naît à la remise client).
  const transferStock = useMutation({
    mutationFn: ({ ids, target }: { ids: string[]; target: string }) =>
      api<SellTransferResult>("/api/sell/transfer", {
        method: "POST",
        body: { ids, targetResellerId: target },
      }),
    onMutate: ({ ids }) => outboundOptimistic(ids),
    onSuccess: (res, _vars, ctx) => {
      if (res.credited > 0) {
        toast.success(
          tf("sell.transferDoneCreditToast", {
            count: res.transferred,
            name: res.target.name,
            amount: formatCurrency(res.credited, currency, lang),
          }),
        );
        if (ctx?.me) {
          qc.setQueryData<SellMe>(["/api/sell/me"], { ...ctx.me, credit: ctx.me.credit + res.credited });
        }
      } else {
        toast.success(tf("sell.transferDoneToast", { count: res.transferred, name: res.target.name }));
      }
      outboundFinish();
    },
    onError: (e: Error, _vars, ctx) => {
      outboundRollback(ctx);
      toast.error(e instanceof ApiError ? e.message : t("sell.transferError"));
    },
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

  // P3-e — la recherche porte sur TOUT le stock : si des pages restent à
  // charger quand le vendeur cherche, elles sont chargées automatiquement
  // (l'effet re-court à chaque page arrivée jusqu'à hasMore=false — borné).
  // Sans ça, « Aucun ticket ne correspond » pourrait mentir sur un stock
  // partiellement chargé : l'invariant R3 de recherche exhaustive prime.
  useEffect(() => {
    if (searching && hasNextPage && !isFetchingNextPage) {
      void fetchNextPage();
    }
  }, [searching, hasNextPage, isFetchingNextPage, fetchNextPage]);

  const currency = me?.currency || "FCFA";
  const isDeposit = me?.paymentMode === "deposit";
  const hasStock = !!stock && stock.length > 0;
  // P3-e — total réel du stock (le serveur le renvoie avec chaque page) :
  // sert au bouton « Afficher plus » (X sur Y).
  const stockTotal = stockPages?.pages[0]?.total ?? stock?.length ?? 0;
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
  // cas d'erreur réseau la vente part en file hors-ligne (UX R6) et la dialog
  // se ferme ; les erreurs métier laissent la dialog ouverte (relance sans
  // retaper).
  function confirmSale() {
    if (pendingSale) sell.mutate({ id: pendingSale.id, voucher: pendingSale });
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
    setDest("manager");
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
    const viaLabel = (via?: string) => t(VIA_KEYS[via ?? "sell_mode"] ?? "sell.viaTactile");
    const viaLine = VIA_ORDER.filter((v) => (report.byVia?.[v] ?? 0) > 0)
      .map((v) => `${viaLabel(v)} ${report.byVia?.[v]}`)
      .join(" · ");
    const lines = [
      tf("sell.dayReportTextHeader", { date: dateLabel }),
      me ? `${me.name} (${me.username})` : "",
      `${t("sell.dayReportSold")} : ${report.soldCount} — ${formatCurrency(report.revenue, currency, lang)}`,
      // P3-d — ventilation par canal (le gérant voit d'où viennent les ventes).
      ...(viaLine ? [viaLine] : []),
      // N°19 V2 — dépôt-vente : le rapport annonce le versement attendu.
      ...(report.paymentMode === "deposit"
        ? [
            tf("sell.dayReportToDepositText", { amount: formatCurrency(report.toDeposit ?? 0, currency, lang) }),
            tf("sell.dayReportDebtText", { amount: formatCurrency(report.debtTotal ?? 0, currency, lang) }),
            ...(report.settledToday && report.settledToday > 0
              ? [tf("sell.dayReportSettled", { amount: formatCurrency(report.settledToday, currency, lang) })]
              : []),
          ]
        : []),
      // P3-d — retours du jour avec flux cash (recrédit prépayé).
      ...((report.returnedCredited ?? 0) > 0
        ? [
            tf("sell.dayReportReturned", {
              count: report.returnedCount ?? 0,
              amount: formatCurrency(report.returnedCredited ?? 0, currency, lang),
            }),
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
                {/* UX R6 — vendu hors-ligne, en attente de replay serveur. */}
                {queued.some((q) => q.voucherId === v.id) && (
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
            <Button
              className="mt-3 w-full"
              onClick={() => setPendingSale(v)}
              disabled={sell.isPending}
            >
              {sell.isPending && sell.variables?.id === v.id ? (
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
              <div className="flex items-center gap-2">
                {!returnMode && (
                  <Button
                    variant="outline"
                    size="sm"
                    className="min-h-9 shrink-0"
                    onClick={() => openPrint({ kind: "all" })}
                    disabled={stock.length === 0}
                  >
                    <Printer className="size-4" />
                    <span className="hidden min-[420px]:inline">{t("sell.printAll")}</span>
                  </Button>
                )}
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

            {/* UX R5 — la saisie papier (R3) est retirée : elle compliquait
                l'UX (3ᵉ mode de vente, codes à taper — risque d'homoglyphes)
                et est devenue redondante. Le ticket papier remis au client se
                vend tout seul à sa 1ʳᵉ connexion hotspot (auto_connect,
                idempotent) ; la vente tactile reste le geste « je
                comptabilise maintenant ». La bannière garde le flux visible —
                le vendeur comprend pourquoi son stock baisse « tout seul ». */}
            {!returnMode && (
              <div
                role="note"
                aria-label={t("sell.autoSaleTitle")}
                className="flex items-start gap-2.5 rounded-xl border bg-muted/20 p-3"
              >
                <Wifi aria-hidden className="mt-0.5 size-4 shrink-0 text-primary" />
                <div className="min-w-0">
                  <p className="text-sm font-semibold">{t("sell.autoSaleTitle")}</p>
                  <p className="mt-0.5 text-xs text-muted-foreground">{t("sell.autoSaleDesc")}</p>
                </div>
              </div>
            )}

            {/* UX R6 — file de ventes hors-ligne : tout ce qui est parti sans
                réseau attend ici, et repartira tout seul au retour du signal
                (replay 409-safe — jamais de double décompte). */}
            {queued.length > 0 && (
              <div
                role="status"
                aria-live="polite"
                className="flex items-start gap-2.5 rounded-xl border border-amber-500/40 bg-amber-500/10 p-3"
              >
                <CloudUpload aria-hidden className="mt-0.5 size-4 shrink-0 text-amber-700 dark:text-amber-300" />
                <div className="min-w-0">
                  <p className="text-sm font-semibold text-amber-700 dark:text-amber-300">
                    {t("sell.queueBannerTitle")}
                  </p>
                  <p className="mt-0.5 text-xs text-muted-foreground">
                    {tf("sell.queueBannerDesc", { count: queued.length })}
                  </p>
                </div>
              </div>
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
                                {!returnMode && (
                                  <button
                                    type="button"
                                    onClick={() =>
                                      openPrint({
                                        kind: "batch",
                                        batchId: b.key === NO_BATCH ? null : b.key,
                                        label: b.key === NO_BATCH ? t("sell.lotNone") : b.labelId,
                                      })
                                    }
                                    aria-label={t("sell.printLotAria")}
                                    className="flex size-8 shrink-0 items-center justify-center rounded-md border bg-background text-muted-foreground transition-colors hover:bg-muted/60 hover:text-foreground"
                                  >
                                    <Printer className="size-3.5" aria-hidden />
                                  </button>
                                )}
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

            {/* P3-e — pagination du stock : les pages suivantes se chargent
                sur demande (un gros stock ne casse plus le premier rendu). */}
            {hasNextPage && (
              <div className="flex flex-col items-center gap-1 py-1">
                <Button
                  variant="outline"
                  onClick={() => void fetchNextPage()}
                  disabled={isFetchingNextPage}
                  className="min-h-11 w-full max-w-xs"
                >
                  {isFetchingNextPage ? (
                    <Loader2 className="size-4 animate-spin" aria-hidden />
                  ) : (
                    <ChevronDown className="size-4" aria-hidden />
                  )}
                  {tf("sell.loadMore", {
                    shown: stock?.length ?? 0,
                    total: stockTotal,
                  })}
                </Button>
                {!searching && (
                  <p className="text-[11px] text-muted-foreground">
                    {t("sell.loadMoreHint")}
                  </p>
                )}
              </div>
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
              onClick={() => {
                setDest("manager");
                setReturnConfirmOpen(true);
              }}
              disabled={returnStock.isPending || transferStock.isPending}
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

              {/* P3-d — ventilation par canal : le comptoir comprend d'où viennent
                  les ventes (tactile / auto à la connexion / papier historique). */}
              {report.byVia && (report.byVia.sell_mode ?? 0) + (report.byVia.auto_connect ?? 0) + (report.byVia.sell_mode_paper ?? 0) > 0 && (
                <div className="flex flex-wrap items-center gap-1.5" role="list" aria-label={t("sell.dayReportByVia")}>
                  {VIA_ORDER.map((via) => {
                    const n = report.byVia?.[via] ?? 0;
                    if (!n) return null;
                    const Icon = viaIcon(via);
                    return (
                      <Badge key={via} variant="secondary" className="gap-1 text-[11px]" role="listitem">
                        <Icon aria-hidden className="size-3" />
                        {t(VIA_KEYS[via])} · {n}
                      </Badge>
                    );
                  })}
                </div>
              )}

              {report.paymentMode === "deposit" && (
                <div className="rounded-lg border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-sm">
                  <p className="font-medium text-amber-700 dark:text-amber-300">
                    {tf("sell.dayReportToDepositText", { amount: formatCurrency(report.toDeposit ?? 0, currency, lang) })}
                  </p>
                  <p className="text-xs text-amber-600/80 dark:text-amber-400/80">
                    {tf("sell.dayReportDebtText", { amount: formatCurrency(report.debtTotal ?? 0, currency, lang) })}
                  </p>
                  {/* P3-d — versements déjà encaissés par le gérant aujourd'hui :
                      le reste à verser est donc toDeposit − settledToday. */}
                  {(report.settledToday ?? 0) > 0 && (
                    <p className="text-xs font-medium text-amber-700 dark:text-amber-300">
                      {tf("sell.dayReportSettled", { amount: formatCurrency(report.settledToday ?? 0, currency, lang) })}
                    </p>
                  )}
                </div>
              )}

              {/* P3-d — retours du jour avec flux cash (recrédit prépayé).
                  En dépôt-vente un retour ne déplace pas d'argent : il n'est
                  pas listé — le journal reste un vrai journal de caisse. */}
              {(report.returnedCredited ?? 0) > 0 && (
                <p className="text-xs text-muted-foreground">
                  {tf("sell.dayReportReturned", {
                    count: report.returnedCount ?? 0,
                    amount: formatCurrency(report.returnedCredited ?? 0, currency, lang),
                  })}
                </p>
              )}

              <div className="max-h-64 overflow-y-auto rounded-lg border" aria-label={t("sell.dayReportDetail")}>
                {report.sold.length === 0 ? (
                  <p className="px-3 py-8 text-center text-sm text-muted-foreground">{t("sell.dayReportEmpty")}</p>
                ) : (
                  report.sold.map((s) => {
                    const Icon = viaIcon(s.soldVia);
                    return (
                      <div
                        key={s.id}
                        className="flex items-center justify-between gap-3 border-b px-3 py-2 last:border-b-0"
                      >
                        <div className="min-w-0">
                          <p className="flex items-center gap-1.5 truncate font-mono text-sm font-semibold">
                            <span title={t(VIA_KEYS[s.soldVia ?? "sell_mode"])} className="flex shrink-0">
                              <Icon
                                aria-hidden
                                className="size-3 text-muted-foreground"
                              />
                            </span>
                            {s.code}
                          </p>
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
                    );
                  })
                )}
              </div>
            </>
          )}

          <DialogFooter>
            {/* P3-d — export comptable : journal de caisse Excel (CSV « ; », BOM). */}
            <Button
              variant="outline"
              onClick={() => void exportJournal()}
              disabled={reportLoading || !report || exporting}
            >
              {exporting ? <Loader2 className="size-4 animate-spin" /> : <Download className="size-4" />}
              {t("sell.dayReportExport")}
            </Button>
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
                {/* Anti-fuite : même le récapitulatif pré-confirmation reste
                    muet sur le code — il n'apparaîtra que dans le reçu. */}
                <p className="mt-1 font-mono text-sm tracking-widest text-muted-foreground" aria-label={t("sell.codeAfterConfirm")}>••••••</p>
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

      {/* Anti-fuite — reçu « Vente confirmée » : seule porte de sortie du
          code. La vente est déjà tracée (SoldAt) ou en file hors-ligne ;
          « Partager » (Web Share, sinon presse-papiers) remet code + mot de
          passe au client au bon moment — jamais avant. */}
      <Dialog
        open={receipt !== null}
        onOpenChange={(open) => {
          if (!open) setReceipt(null);
        }}
      >
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <BadgeCheck className="size-4 text-primary" aria-hidden />
              {t("sell.receiptTitle")}
            </DialogTitle>
            <DialogDescription>{t("sell.receiptDesc")}</DialogDescription>
          </DialogHeader>

          {receipt && (
            <>
              <div className="rounded-lg border bg-muted/30 p-3">
                <div className="flex items-center justify-between gap-3">
                  <p className="font-semibold">{receipt.voucher.profileName}</p>
                  <p className="shrink-0 font-bold text-primary tabular-nums">
                    {formatCurrency(receipt.voucher.sellingPrice || receipt.voucher.price, currency, lang)}
                  </p>
                </div>
                <div className={`mt-3 grid gap-2 font-mono text-sm ${isSamePasswordMode(receipt.voucher) ? "grid-cols-1" : "grid-cols-2"}`}>
                  <div>
                    <p className="text-[10px] tracking-wide text-muted-foreground uppercase">{t("sell.code")}</p>
                    <p className="mt-0.5 text-base font-semibold select-all">{receipt.voucher.username}</p>
                  </div>
                  {!isSamePasswordMode(receipt.voucher) && (
                    <div>
                      <p className="text-[10px] tracking-wide text-muted-foreground uppercase">{t("sell.password")}</p>
                      <p className="mt-0.5 text-base font-semibold select-all">{receipt.voucher.password}</p>
                    </div>
                  )}
                </div>
              </div>

              {receipt.offline && (
                <p
                  role="status"
                  className="flex items-start gap-2 rounded-lg border border-amber-500/40 bg-amber-500/10 px-3 py-2 text-xs text-amber-700 dark:text-amber-300"
                >
                  <CloudUpload aria-hidden className="mt-0.5 size-3.5 shrink-0" />
                  {t("sell.receiptOffline")}
                </p>
              )}

              <DialogFooter>
                <Button variant="outline" onClick={() => setReceipt(null)}>
                  {t("common.close")}
                </Button>
                <Button onClick={() => void share(receipt.voucher)}>
                  <Share2 className="size-4" />
                  {t("sell.share")}
                </Button>
              </DialogFooter>
            </>
          )}
        </DialogContent>
      </Dialog>

      {/* N°20/N°21 — confirmation de sortie de stock : retour au gérant OU
          transfert entre revendeurs (fusion UX — même sélection de tickets,
          destination choisie ici, défaut = retour historique). */}
      <Dialog open={returnConfirmOpen} onOpenChange={setReturnConfirmOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              {dest === "manager" ? (
                <Undo2 className="size-4 text-primary" aria-hidden />
              ) : (
                <ArrowLeftRight className="size-4 text-primary" aria-hidden />
              )}
              {dest === "manager"
                ? t("sell.returnConfirmTitle")
                : tf("sell.transferConfirmTitle", { name: destName })}
            </DialogTitle>
            <DialogDescription>
              {dest === "manager" ? (
                isDeposit ? (
                  tf("sell.returnConfirmDescDeposit", { count: selected.size })
                ) : (
                  tf("sell.returnConfirmDescPrepaid", {
                    count: selected.size,
                    amount: formatCurrency(selectedWholesale, currency, lang),
                  })
                )
              ) : isDeposit ? (
                tf("sell.transferConfirmDescDeposit", { count: selected.size, name: destName })
              ) : (
                tf("sell.transferConfirmDescPrepaid", {
                  count: selected.size,
                  name: destName,
                  amount: formatCurrency(selectedWholesale, currency, lang),
                })
              )}
            </DialogDescription>
          </DialogHeader>
          {/* Destination : gérant (retour de stock) ou pair (transfert). */}
          <div className="space-y-1.5">
            <Label htmlFor="outbound-dest">{t("sell.outboundDest")}</Label>
            <Select value={dest} onValueChange={setDest}>
              <SelectTrigger id="outbound-dest" className="h-10 w-full">
                <SelectValue aria-label={t("sell.outboundDest")} />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="manager">{t("sell.outboundDestManager")}</SelectItem>
                {(peers ?? []).map((p) => (
                  <SelectItem key={p.id} value={p.id}>
                    {tf("sell.outboundDestPeer", { name: p.name })}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            {dest !== "manager" && (
              <p className="text-xs text-muted-foreground">{t("sell.transferHint")}</p>
            )}
          </div>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setReturnConfirmOpen(false)}
              disabled={returnStock.isPending || transferStock.isPending}
            >
              {t("common.cancel")}
            </Button>
            <Button
              onClick={() =>
                dest === "manager"
                  ? returnStock.mutate([...selected])
                  : transferStock.mutate({ ids: [...selected], target: dest })
              }
              disabled={returnStock.isPending || transferStock.isPending || selected.size === 0}
            >
              {returnStock.isPending || transferStock.isPending ? (
                <Loader2 className="size-4 animate-spin" />
              ) : dest === "manager" ? (
                <Undo2 className="size-4" />
              ) : (
                <ArrowLeftRight className="size-4" />
              )}
              {dest === "manager" ? t("sell.returnAction") : t("sell.transferAction")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      {/* Impression revendeur — stock complet ou lot unique (A4 / thermique). */}
      <SellPrintDialog
        key={printSession}
        open={printScope !== null}
        onOpenChange={(open) => {
          if (!open) setPrintScope(null);
        }}
        scope={printScope ?? { kind: "all" }}
        tenantName={me?.tenantName ?? ""}
        currency={currency}
      />
    </div>
  );
}
