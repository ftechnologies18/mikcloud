"use client";

// N°8 — Mode Vente (PWA revendeur en tournée). — SHELL d'état.
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
//
// Éclatement (présentation extraite) : TOUT l'état, les requêtes, les
// mutations, le replay hors-ligne et les partages vivent ICI ; le rendu
// déménage dans ./sell/ (helpers module-level, voucher-card, stock-section,
// dialogs) — mêmes composants, mêmes props calculées.

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
  FileBarChart,
  LogOut,
  RefreshCw,
  ShoppingCart,
  Undo2,
  Wifi,
  WifiOff,
  X,
} from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { api, apiDownload, ApiError } from "@/lib/hotspot/api";
import { PwaInstallCta } from "@/components/pwa-install-cta";
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
import type { SellDayReport } from "@/lib/hotspot/types";
import { isSamePasswordMode } from "@/components/hotspot/parts/template-render";
import { useHotspotStore } from "@/lib/hotspot/store";
import {
  filterPagedStock,
  getViewSnapshot,
  getServerViewSnapshot,
  groupStock,
  isNetworkError,
  ME_CACHE_KEY,
  readCache,
  shortBatchId,
  STOCK_CACHE_KEY,
  STOCK_PAGE_SIZE,
  subscribeView,
  useOnline,
  VIA_KEYS,
  VIA_ORDER,
  writeCache,
  type SellMe,
  type SellPeer,
  type SellReturnResult,
  type SellTransferResult,
  type SellVoucher,
  type StockPage,
} from "./sell/helpers";
import { StockSection } from "./sell/stock-section";
import {
  DayReportDialog,
  OutboundConfirmDialog,
  ReceiptDialog,
  SaleConfirmDialog,
} from "./sell/dialogs";

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
  // Ids en file (chips « en attente » sur les cartes + bannière).
  const queuedIds = useMemo(() => new Set(queued.map((q) => q.voucherId)), [queued]);

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
  // Spinners de vente (carte + confirmation) : mutation en vol sur CE ticket.
  const sellPendingId = sell.isPending ? (sell.variables?.id ?? null) : null;

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
  // N°39 — l'effet peut se figer si le fetchNextPage est avalé par un
  // refetch concurrent du même queryKey (race TanStack Query v5 : la réponse
  // de la page suivante est jetée quand le refetch réinitialise `pages`,
  // isFetchingNextPage repasse false sans re-rendu intermédiaire → aucune
  // dep ne change → plus jamais re-couru ; symptomatique E2E 08:15 :
  // offset=60 reçu en 200 mais « Aucun ticket ne correspond » affiché).
  // Garde temporisée : tant que l'invariant est insatisfait (recherche
  // active + pages restantes), une tentative est relancée toutes les 300 ms
  // dès que la query est oisive — l'appel est idempotent côté serveur et
  // borné par le nombre de pages du stock.
  useEffect(() => {
    if (!searching || !hasNextPage) return;
    const id = setInterval(() => {
      if (!isFetchingNextPage) void fetchNextPage();
    }, 300);
    return () => clearInterval(id);
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
      window.localStorage.setItem("mikcloud-sell-view", v);
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
  function toggleBatchSelection(b: { vouchers: SellVoucher[] }) {
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

  return (
    <div className="mx-auto flex min-h-dvh w-full max-w-lg flex-col bg-background pwa-safe-bottom">
      {/* En-tête revendeur — N°60 : pwa-safe-top compense l'encoche iOS
          (viewport-fit cover + status bar black-translucent) : le fond de
          l'en-tête couvre la découpe, le contenu reste sous la zone système. */}
      <header className="pwa-safe-top sticky top-0 z-10 border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/80">
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

      {/* N°60 — PWA : le revendeur connecté (token persistant) ne repasse
          jamais par le login : sans CTA ici, il ne verrait JAMAIS le bouton
          d'installation. Caché si déjà installé/standalone ou rejeté. */}
      <PwaInstallCta />

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

      {/* Stock — squelettes, barre, recherche, bannières, vues, pagination
          (présentation extraite dans ./sell/stock-section.tsx). */}
      <StockSection
        isLoading={isLoading}
        stock={stock}
        stockTotal={stockTotal}
        hasNextPage={hasNextPage}
        isFetchingNextPage={isFetchingNextPage}
        onLoadMore={() => void fetchNextPage()}
        view={view}
        onChangeView={changeView}
        query={query}
        onQueryChange={setQuery}
        searching={searching}
        filteredStock={filteredStock}
        filteredGroups={filteredGroups}
        collapsedGroups={collapsedGroups}
        onToggleGroup={toggleGroup}
        returnMode={returnMode}
        selected={selected}
        onToggleSelected={toggleSelected}
        onToggleBatchSelection={toggleBatchSelection}
        queuedIds={queuedIds}
        sellPending={sell.isPending}
        sellPendingId={sellPendingId}
        onSell={setPendingSale}
        onOpenPrint={openPrint}
        currency={currency}
      />

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

      {/* Rapport de fin de journée (présentation extraite — ./sell/dialogs.tsx) */}
      <DayReportDialog
        open={reportOpen}
        onOpenChange={setReportOpen}
        report={report}
        reportLoading={reportLoading}
        currency={currency}
        exporting={exporting}
        onExport={() => void exportJournal()}
        onShare={() => void shareReport()}
      />

      {/* UX R2 — confirmation de vente : récapitulatif du ticket (code,
          profil, prix) + action définitive explicite. En cas d'erreur réseau
          la dialog reste ouverte pour relancer sans re-sélectionner. */}
      <SaleConfirmDialog
        pendingSale={pendingSale}
        currency={currency}
        sellPending={sell.isPending}
        onOpenChange={(open) => {
          if (!open && !sell.isPending) setPendingSale(null);
        }}
        onConfirm={confirmSale}
        onCancel={() => setPendingSale(null)}
      />

      {/* Anti-fuite — reçu « Vente confirmée » : seule porte de sortie du
          code. La vente est déjà tracée (SoldAt) ou en file hors-ligne ;
          « Partager » (Web Share, sinon presse-papiers) remet code + mot de
          passe au client au bon moment — jamais avant. */}
      <ReceiptDialog
        receipt={receipt}
        currency={currency}
        onOpenChange={(open) => {
          if (!open) setReceipt(null);
        }}
        onShare={(voucher) => void share(voucher)}
      />

      {/* N°20/N°21 — confirmation de sortie de stock : retour au gérant OU
          transfert entre revendeurs (fusion UX — même sélection de tickets,
          destination choisie ici, défaut = retour historique). */}
      <OutboundConfirmDialog
        open={returnConfirmOpen}
        onOpenChange={setReturnConfirmOpen}
        currency={currency}
        dest={dest}
        onDestChange={setDest}
        destName={destName}
        peers={peers}
        selectedCount={selected.size}
        selectedWholesale={selectedWholesale}
        isDeposit={isDeposit}
        returnPending={returnStock.isPending}
        transferPending={transferStock.isPending}
        onReturn={() => returnStock.mutate([...selected])}
        onTransfer={() => transferStock.mutate({ ids: [...selected], target: dest })}
      />

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
