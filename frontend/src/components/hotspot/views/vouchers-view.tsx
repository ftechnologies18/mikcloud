"use client";

// Vue Vouchers (kind=voucher) — SHELL d'état : génération en série, impression
// de tickets, suivi des statuts et traçabilité des lots (onglet Lots).
// Quick print (F12) : bouton « Réimpression rapide » qui réimprime le dernier
// lot imprimé (localStorage "mikcloud-last-batch", écrit par uc-print-dialog).
//
// Éclatement (refactor présentation) : toute la logique d'état (filtres,
// pagination, queries, mutations, dialogs) vit ICI ; le rendu des deux
// onglets a déménagé dans ./vouchers/ (vouchers-tab, batches-tab,
// confirm-dialogs) — composants de présentation pure alimentés par props.

import { useEffect, useMemo, useRef, useState } from "react";
import { usePathname, useRouter } from "next/navigation";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Layers, Ticket, TicketPlus, Zap } from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { PageHeader } from "@/components/hotspot/page-header";
import { useCurrency, useSettings } from "@/components/hotspot/parts/sd-currency";
import { copyToClipboard } from "@/components/hotspot/parts/uc-clipboard";
import { LAST_BATCH_STORAGE_KEY, UcPrintDialog } from "@/components/hotspot/parts/uc-print-dialog";
import { VoucherTransferDialog } from "@/components/hotspot/parts/voucher-transfer-dialog";
import { VoucherWizardDialog } from "@/components/hotspot/parts/voucher-wizard-dialog";
import { BatchDetailSheet } from "@/components/hotspot/parts/batch-detail-sheet";
import { BatchPrintDialog } from "@/components/hotspot/parts/batch-print-dialog";
import { api, apiDownload } from "@/lib/hotspot/api";
import { STALE_TIME } from "@/lib/hotspot/query";
import { useI18n } from "@/lib/hotspot/i18n";
import { useHotspotStore } from "@/lib/hotspot/store";
import { detailFromPath, viewToPath } from "@/lib/hotspot/view-path";
import { formatCurrency } from "@/lib/hotspot/format";
import type {
  BatchWithStats,
  GenerateVouchersRequest,
  GenerateVouchersResponse,
  HotspotUser,
  PagedBatches,
  PagedUsers,
  Profile,
  Reseller,
  RouterDevice,
  VoucherTemplate,
} from "@/lib/hotspot/types";
import { BATCH_STATUS_OPTIONS, shortBatch, VouchersStats } from "./vouchers/shared";
import { usePageSize } from "@/components/hotspot/parts/page-size-select";
import { VouchersTab } from "./vouchers/vouchers-tab";
import { BatchesTab } from "./vouchers/batches-tab";
import {
  DeleteBatchConfirmDialog,
  DeleteVoucherConfirmDialog,
  RepriseConfirmDialog,
} from "./vouchers/confirm-dialogs";

// N°191 — miroir module des filtres de l'onglet Vouchers : le catch-all
// /app/[[...vue]] REMONTE à chaque changement de params (vérifié au
// navigateur — MÊME à nombre de segments constant) : basculer la loupe
// routeur réinitialiserait recherche + statut + détenteur + profil à
// chaque chip. Piège de séquencement (découvert en vérification
// navigateur) : l'initialisateur du NOUVEL arbre tourne AVANT le cleanup
// de l'ancien — une sauvegarde au démontage arrive TROP TARD. Le miroir
// est donc écrit EN CONTINU (effet sur les filtres), UNIQUEMENT en ère
// loupe (sans détail lot : l'ère détail lot est pilotée par l'effet
// Phase D, le miroir ne doit pas lui survivre — sinon la recherche
// resterait « coincée » sur l'id de lot au retour-navigation) ; il est
// vidé au démontage si la vue a vraiment changé (vraie sortie → la
// prochaine entrée repart propre, comportement inchangé).
let vouchersKeptFilters: {
  searchInput: string;
  search: string;
  statusFilter: string;
  holderFilter: string;
  profileFilter: string;
} | null = null;

export default function VouchersView() {
  const { t, tf, lang } = useI18n();
  const currency = useCurrency();
  const { data: settings } = useSettings();
  const tenantName = settings?.tenant.name || "MikCloud";
  const queryClient = useQueryClient();

  // Onglet actif : liste des vouchers ou traçabilité des lots
  const [tab, setTab] = useState<"vouchers" | "batches">("vouchers");

  // Filtres de la table (recherche avec debounce ~400 ms)
  const [searchInput, setSearchInput] = useState(() => vouchersKeptFilters?.searchInput ?? "");
  const [search, setSearch] = useState(() => vouchersKeptFilters?.search ?? "");
  const [statusFilter, setStatusFilter] = useState(() => vouchersKeptFilters?.statusFilter ?? "all");
  // N°23 (W3/W4) — détenteur du stock : direct (gérant) / alloué (revendeurs).
  const [holderFilter, setHolderFilter] = useState(() => vouchersKeptFilters?.holderFilter ?? "all");
  const [profileFilter, setProfileFilter] = useState(() => vouchersKeptFilters?.profileFilter ?? "all");
  const [page, setPage] = useState(1);
  // N°193 — sélecteur de pagination (les deux tables de la vue, une
  // préférence mémorisée CHACUNE : densité utile différente entre le stock
  // et la traçabilité des lots). Défaut vouchers normalisé 12 → 10 pour
  // vivre dans l'échelle commune 10/25/50/100.
  const [pageSize, setPageSize] = usePageSize("vouchers");

  // Filtres des lots
  const [batchSearchInput, setBatchSearchInput] = useState("");
  const [batchSearch, setBatchSearch] = useState("");
  const [batchRouterFilter, setBatchRouterFilter] = useState("all");
  const [batchPage, setBatchPage] = useState(1);
  // N°193 — sélecteur de pagination des lots.
  const [batchPageSize, setBatchPageSize] = usePageSize("batches");
  // Refonte « fiche de vie » — filtres canal (provenance), cycle de vie et
  // détenteur LIVE du stock vendable (direct | resellers | id du revendeur).
  // v2 — DÉFAUT « stock » (Vivants) : l'onglet ouvre sur le stock vivant.
  const [batchChannelFilter, setBatchChannelFilter] = useState("all");
  const [batchStatusFilter, setBatchStatusFilter] = useState("stock");
  const [batchHolderFilter, setBatchHolderFilter] = useState("all");
  // v2 — filtres mobiles : un seul bouton « Filtres » ouvre un sheet bottom.
  const [batchFiltersOpen, setBatchFiltersOpen] = useState(false);

  useEffect(() => {
    const timer = setTimeout(() => {
      setBatchSearch(batchSearchInput.trim());
      setBatchPage(1);
    }, 400);
    return () => clearTimeout(timer);
  }, [batchSearchInput]);

  useEffect(() => {
    const timer = setTimeout(() => {
      setSearch(searchInput.trim());
      setPage(1);
    }, 400);
    return () => clearTimeout(timer);
  }, [searchInput]);

  // Révélation des mots de passe (par ligne)
  const [revealed, setRevealed] = useState<Set<string>>(new Set());

  // Générateur — le formulaire vit dans VoucherWizardDialog (3 étapes).
  const [genOpen, setGenOpen] = useState(false);

  // Impression (liste simple — ancien dialog)
  const [printOpen, setPrintOpen] = useState(false);
  const [printVouchers, setPrintVouchers] = useState<HotspotUser[]>([]);
  const [printTitle, setPrintTitle] = useState("");
  /** Lot en cours d'impression (F12) — mémorisé à l'impression par le dialog. */
  const [printBatchId, setPrintBatchId] = useState<string | undefined>(undefined);
  // N°22 — impression unitaire en cours (spinner sur la ligne concernée).
  const [printingVoucherId, setPrintingVoucherId] = useState<string | null>(null);

  // Suppression
  const [deleting, setDeleting] = useState<HotspotUser | null>(null);

  // N°23 (W6) — reprise gérant : reprendre au revendeur un ticket invendu.
  const [reprising, setReprising] = useState<HotspotUser | null>(null);

  const { data: profiles } = useQuery({
    queryKey: ["/api/profiles"],
    queryFn: () => api<Profile[]>("/api/profiles"),
    // N°130 — donnée de référence : ne change qu'à l'écriture.
    staleTime: STALE_TIME.reference,
  });

  const { data: routers, isLoading: routersLoading } = useQuery({
    queryKey: ["/api/routers"],
    queryFn: () => api<RouterDevice[]>("/api/routers"),
    // N°130 — état du parc : bouge aux check-ins agents (~45 s).
    staleTime: STALE_TIME.operational,
  });

  const { data: resellers } = useQuery({
    queryKey: ["/api/resellers"],
    queryFn: () => api<Reseller[]>("/api/resellers"),
    // N°130 — donnée de référence : stock/attribution à l'écriture.
    staleTime: STALE_TIME.reference,
  });

  // Modèles de vouchers (F2) pour le dialog d'impression — la query échoue en 404
  // tant que le backend P0 n'est pas déployé : fallback silencieux sur le ticket standard.
  const { data: templates } = useQuery({
    queryKey: ["/api/templates"],
    queryFn: () => api<VoucherTemplate[]>("/api/templates"),
    // N°130 — unifié sur la constante graduée (même valeur qu'avant).
    staleTime: STALE_TIME.reference,
  });

  // N°191 — LOUPE ROUTEUR : le segment d'URL de la vue porte SOIT le
  // deep-link lot (Phase D : /app/vouchers/<batchId>, un UUID), SOIT la
  // portée routeur « router:<id> » (préfixe réservé — les ids de lot sont
  // des UUID, jamais ce préfixe ; même convention que la clé canonique de
  // l'éditeur Portail, N°190). La loupe VIT dans l'URL (pattern Protection) :
  // rafraîchissement, partage et signet retombent sur la portée ; chaque
  // changement de chip la remplace (applyScope — replace, un réglage pas
  // une navigation). Dérivé de l'URL au premier rendu : aucune
  // synchronisation effet→état.
  const router = useRouter();
  const pathname = usePathname();
  const detailSegment = detailFromPath(pathname, "vouchers");
  const scopeRouterId = detailSegment?.startsWith("router:")
    ? detailSegment.slice("router:".length)
    : null;
  const detailBatchId = detailSegment && !detailSegment.startsWith("router:") ? detailSegment : null;

  function applyScope(routerId: string) {
    router.replace(viewToPath("vouchers", routerId ? `router:${routerId}` : undefined), { scroll: false });
  }

  // N°191 — segment orphelin (routeur supprimé, signet périmé) :
  // re-normalisation vers la racine de la vue — replace, zéro entrée
  // d'historique parasite (miroir Portail N°190). Attends le parc : un
  // routeur pas encore chargé n'est PAS orphelin.
  useEffect(() => {
    if (!scopeRouterId || routersLoading) return;
    if (!routers?.some((r) => r.id === scopeRouterId)) {
      router.replace(viewToPath("vouchers"), { scroll: false });
    }
  }, [scopeRouterId, routersLoading, routers, router]);

  // N°191 — miroir écrit EN CONTINU, en ère loupe uniquement (sans détail
  // lot) : l'initialisateur du nouvel arbre court avant le cleanup de
  // l'ancien, toute sauvegarde différée arriverait trop tard. En ère détail
  // lot, le miroir est VIDE (l'effet Phase D pilote les filtres — le
  // retour-navigation doit retomber propre, pas sur un lot fantôme).
  useEffect(() => {
    vouchersKeptFilters =
      !detailSegment || detailSegment.startsWith("router:")
        ? { searchInput, search, statusFilter, holderFilter, profileFilter }
        : null;
  }, [detailSegment, searchInput, search, statusFilter, holderFilter, profileFilter]);
  useEffect(
    () => () => {
      // Démontage : vue toujours sur vouchers → remontage du catch-all
      // (loupe) : le miroir continu fait son œuvre. Sinon → vraie sortie de
      // vue : vider (la prochaine entrée repart propre).
      if (useHotspotStore.getState().view !== "vouchers") {
        vouchersKeptFilters = null;
      }
    },
    [],
  );

  // Statistiques globales (page large sans filtre) — compteurs calculés côté
  // SERVEUR (N°74) : l'ancien poll pageSize:500 téléchargeait jusqu'à 200
  // objets complets toutes les 20 s (~10-15 Ko gzip) et comptait FAUX dès que
  // le stock dépassait le plafond pageSize 200. La réponse est un objet
  // compact {active, used, expired, allocated, stockValue, total} (~150 o).
  const { data: statsData, isLoading: statsLoading } = useQuery({
    queryKey: ["/api/vouchers", "stats"],
    queryFn: () => api<VouchersStats>("/api/vouchers/stats"),
    refetchInterval: 20_000,
  });

  // N°191 — KPI au niveau de la loupe : ventilation byRouter du serveur
  // (absente sur un backend antérieur → compteurs à 0, dégradé propre).
  const scopedStats = scopeRouterId
    ? statsData?.byRouter?.find((s) => s.routerId === scopeRouterId)
    : undefined;
  const activeCount = scopeRouterId ? (scopedStats?.active ?? 0) : (statsData?.active ?? 0);
  const usedCount = scopeRouterId ? (scopedStats?.used ?? 0) : (statsData?.used ?? 0);
  const expiredCount = scopeRouterId ? (scopedStats?.expired ?? 0) : (statsData?.expired ?? 0);
  // N°23 (W3/W4) — visibilité du stock confié aux revendeurs.
  const allocatedCount = scopeRouterId ? (scopedStats?.allocated ?? 0) : (statsData?.allocated ?? 0);
  const stockValue = scopeRouterId ? (scopedStats?.stockValue ?? 0) : (statsData?.stockValue ?? 0);

  // N°191 — badges du rail : stock vivant (actifs) par routeur, glissé sur
  // la ventilation byRouter du serveur (zéro requête supplémentaire).
  // Chaque routeur du parc porte un badge, MÊME à 0 — mais seulement quand
  // la ventilation existe (backend antérieur → pas de badge plutôt qu'un
  // « 0 » mensonger : inconnu ≠ vide).
  const stockByRouter = useMemo(() => {
    if (!statsData?.byRouter) return undefined;
    const counts: Record<string, number> = {};
    for (const r of routers ?? []) counts[r.id] = 0;
    for (const rs of statsData.byRouter) counts[rs.routerId] = rs.active;
    return counts;
  }, [statsData, routers]);
  const totalActive = statsData?.active ?? 0;
  // Routeur de la loupe (nom pour les libellés) — le parc peut être en
  // chargement : les KPI scopés gardent alors leur sous-texte générique.
  const scopedRouter = routers?.find((r) => r.id === scopeRouterId) ?? null;

  // Liste paginée filtrée — N°191 : le filtre routeur vit CÔTÉ SERVEUR
  // (param routerId de filterUsers, contrat existant) : la loupe ne
  // télécharge pas les pages des autres points d'accès.
  const statusParam = statusFilter === "all" ? undefined : statusFilter;
  const profileParam = profileFilter === "all" ? undefined : profileFilter;
  // N°23 (W3/W4) — détenteur : direct (gérant) / alloué (revendeurs).
  const holderParam = holderFilter === "all" ? undefined : holderFilter;
  const scopeRouterParam = scopeRouterId ?? undefined;

  const { data: pagedData, isLoading, isFetching } = useQuery({
    queryKey: [
      "/api/vouchers",
      "list",
      { search, status: statusParam, profileId: profileParam, holder: holderParam, routerId: scopeRouterParam, page, pageSize },
    ],
    queryFn: () =>
      api<PagedUsers>("/api/vouchers", {
        params: { search, status: statusParam, profileId: profileParam, holder: holderParam, routerId: scopeRouterParam, page, pageSize },
      }),
    refetchInterval: 20_000,
    placeholderData: (previous) => previous,
  });

  const vouchers = pagedData?.data ?? [];
  const totalCount = pagedData?.total ?? 0;

  function invalidateVouchers() {
    void queryClient.invalidateQueries({ queryKey: ["/api/vouchers"] });
    void queryClient.invalidateQueries({ queryKey: ["/api/dashboard"] });
  }

  // Phase D — deep-link /app/vouchers/<batchId> : « détail lot » = onglet
  // vouchers filtré sur le lot (équivalent viewBatchVouchers, sans push —
  // on est déjà sur l'URL). Segment consommé localement : ni le store ni
  // app-route ne changent (fix 192ad9f préservé). À la sortie du détail
  // (Retour navigateur), le filtre lot est levé s'il n'a pas divergé.
  // (La lecture du segment et la loupe routeur vivent plus haut — N°191.)
  const prevDetail = useRef<string | null>(null);
  // Miroir de la recherche lu par l'effet ci-dessous : synchronisé dans un
  // effet (jamais pendant le rendu — règle react-hooks/refs), déclaré
  // AVANT l'effet de détail pour tourner en premier.
  const searchRef = useRef(search);
  useEffect(() => {
    searchRef.current = search;
  });

  useEffect(() => {
    const leaving = prevDetail.current;
    if (detailBatchId) {
      setTab("vouchers");
      setSearchInput(detailBatchId);
      setSearch(detailBatchId);
      setStatusFilter("all");
      setHolderFilter("all");
      setProfileFilter("all");
      setPage(1);
    } else if (leaving && searchRef.current === leaving) {
      setSearchInput("");
      setSearch("");
      setStatusFilter("all");
      setHolderFilter("all");
      setProfileFilter("all");
      setPage(1);
    }
    prevDetail.current = detailBatchId;
  }, [detailBatchId]);

  // N (rapprochement doux) — resynchronisation d'un voucher « absent du
  // routeur » : recréer (user_add en file) ou oublier (retrait du cloud).
  const resyncMutation = useMutation({
    mutationFn: (vars: { voucher: HotspotUser; action: "recreate" | "forget" }) =>
      api<{ ok: boolean }>(`/api/users/${vars.voucher.id}/resync`, {
        method: "POST",
        body: { action: vars.action },
      }),
    onSuccess: (_res, vars) => {
      toast.success(
        vars.action === "recreate"
          ? tf("users.resyncRecreateToast", { name: vars.voucher.username })
          : tf("users.resyncForgetToast", { name: vars.voucher.username }),
      );
      invalidateVouchers();
    },
    onError: (error: Error) => toast.error(error.message),
  });

  function toggleReveal(id: string) {
    setRevealed((previous) => {
      const next = new Set(previous);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  async function copyCode(voucher: HotspotUser) {
    const ok = await copyToClipboard(voucher.username);
    if (ok) toast.success(t("vouchers.codeCopied"));
    else toast.error(t("common.copyImpossible"));
  }

  // N°22 — canal d'impression tracé : les listes masquent les codes des
  // tickets revendeur (anti-vente en direct) ; l'impression récupère les
  // codes COMPLETS via POST /api/vouchers/print — le serveur trace toute
  // remise de codes revendeur dans le journal d'activité. La propriété ne
  // change pas : le ticket reste chez le revendeur (vente auto à la 1ʳᵉ
  // connexion du client créditée chez lui).
  async function fetchPrintVouchers(ids: string[]): Promise<HotspotUser[]> {
    const res = await api<{ vouchers: HotspotUser[]; tracedCount: number }>("/api/vouchers/print", {
      method: "POST",
      body: { ids },
    });
    return res.vouchers;
  }

  async function printOne(voucher: HotspotUser) {
    setPrintingVoucherId(voucher.id);
    try {
      const full = await fetchPrintVouchers([voucher.id]);
      setPrintVouchers(full);
      setPrintTitle(tf("vouchers.printOneTitle", { code: full[0]?.username ?? voucher.username }));
      setPrintBatchId(voucher.batchId || undefined);
      setPrintOpen(true);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t("vouchers.printFailed"));
    } finally {
      setPrintingVoucherId(null);
    }
  }

  const generateMutation = useMutation({
    mutationFn: (payload: GenerateVouchersRequest) =>
      api<GenerateVouchersResponse>("/api/vouchers/generate", { method: "POST", body: payload }),
    onSuccess: (res) => {
      toast.success(
        tf("vouchers.generatedToast", { n: res.vouchers.length, batch: shortBatch(res.batchId) }),
      );
      setGenOpen(false);
      setPrintVouchers(res.vouchers);
      setPrintTitle(tf("vouchers.printBatchTitle", { batch: shortBatch(res.batchId) }));
      setPrintBatchId(res.batchId);
      setPrintOpen(true);
      void queryClient.invalidateQueries({ queryKey: ["/api/vouchers"] });
      void queryClient.invalidateQueries({ queryKey: ["/api/dashboard"] });
      void queryClient.invalidateQueries({ queryKey: ["/api/resellers"] });
      void queryClient.invalidateQueries({ queryKey: ["/api/transactions"] });
      void queryClient.invalidateQueries({ queryKey: ["/api/reports"] });
    },
    onError: (error: Error) => toast.error(error.message),
  });

  const deleteMutation = useMutation({
    mutationFn: (voucher: HotspotUser) => api<{ ok: boolean }>(`/api/vouchers/${voucher.id}`, { method: "DELETE" }),
    onSuccess: (_res, voucher) => {
      toast.success(tf("vouchers.deletedToast", { code: voucher.username }));
      setDeleting(null);
      invalidateVouchers();
    },
    onError: (error: Error) => toast.error(error.message),
  });

  // N°23 (W6) — reprise gérant : retour au stock direct d'un ticket invendu
  // (recrédit prépayé au prix gros côté serveur ; dépôt-vente : aucun).
  const repriseMutation = useMutation({
    mutationFn: (voucher: HotspotUser) =>
      api<{ returned: number; credited: number }>("/api/vouchers/reprise", {
        method: "POST",
        body: { ids: [voucher.id] },
      }),
    onSuccess: (res) => {
      if (res.credited > 0) {
        toast.success(
          tf("vouchers.reprise.doneCredited", { count: res.returned, credit: formatCurrency(res.credited, currency, lang) }),
        );
      } else {
        toast.success(tf("vouchers.reprise.done", { count: res.returned }));
      }
      setReprising(null);
      invalidateVouchers();
      void queryClient.invalidateQueries({ queryKey: ["/api/resellers"] });
      void queryClient.invalidateQueries({ queryKey: ["/api/transactions"] });
    },
    onError: (error: Error) => toast.error(error.message),
  });

  // --- Traçabilité des lots ---
  const batchRouterParam = batchRouterFilter === "all" ? undefined : batchRouterFilter;
  const batchStatusParam = batchStatusFilter === "all" ? undefined : batchStatusFilter;
  const batchHolderParam = batchHolderFilter === "all" ? undefined : batchHolderFilter;
  const batchChannelParam = batchChannelFilter === "all" ? undefined : batchChannelFilter;

  const { data: batchData, isLoading: batchesLoading, isFetching: batchesFetching } = useQuery({
    queryKey: [
      "/api/vouchers",
      "batches",
      {
        search: batchSearch,
        routerId: batchRouterParam,
        status: batchStatusParam,
        holder: batchHolderParam,
        channel: batchChannelParam,
        page: batchPage,
        pageSize: batchPageSize,
      },
    ],
    queryFn: () =>
      api<PagedBatches>("/api/vouchers/batches", {
        params: {
          search: batchSearch,
          routerId: batchRouterParam,
          status: batchStatusParam,
          holder: batchHolderParam,
          channel: batchChannelParam,
          page: batchPage,
          pageSize: batchPageSize,
        },
      }),
    refetchInterval: 30_000,
    placeholderData: (previous) => previous,
  });

  const batches = batchData?.data ?? [];
  // Refonte v2 — « tour de contrôle » : pipeline du stock (totaux sur
  // l'ensemble FILTRÉ, back) consommé par le composant BatchPipeline.
  const batchSummary = batchData?.summary;
  const batchTotal = batchData?.total ?? 0;
  // Premier chargement (aucune donnée en cache) : squelettes pipeline + table.
  const batchesFirstLoading = batchesLoading && !batchData;

  const [deletingBatch, setDeletingBatch] = useState<BatchWithStats | null>(null);
  const [printingBatchId, setPrintingBatchId] = useState<string | null>(null);

  // N°18 — Transfert de stock du lot (distribution revendeur / retour de stock).
  const [transferBatch, setTransferBatch] = useState<BatchWithStats | null>(null);

  // Refonte « fiche de vie » — fiche 360° du lot (drawer) + impression
  // multi-formats (F3) + export CSV.
  const [detailBatch, setDetailBatch] = useState<BatchWithStats | null>(null);
  const [multiPrintBatch, setMultiPrintBatch] = useState<BatchWithStats | null>(null);
  const [multiPrintVouchers, setMultiPrintVouchers] = useState<HotspotUser[] | null>(null);
  const [exportingBatches, setExportingBatches] = useState(false);

  const batchDeleteMutation = useMutation({
    mutationFn: (batch: BatchWithStats) =>
      api<{ ok: boolean; deleted: number }>(`/api/vouchers/batch/${batch.id}/delete`, { method: "POST" }),
    onSuccess: (res, batch) => {
      toast.success(
        tf("vouchers.batches.deletedToast", { batch: shortBatch(batch.id), n: res.deleted }),
      );
      setDeletingBatch(null);
      invalidateVouchers();
    },
    onError: (error: Error) => toast.error(error.message),
  });

  // « Voir les vouchers » : bascule sur l'onglet vouchers filtré par le lot.
  function viewBatchVouchers(batch: BatchWithStats) {
    setSearchInput(batch.id);
    setSearch(batch.id);
    setStatusFilter("all");
    setHolderFilter("all");
    setProfileFilter("all");
    setPage(1);
    setTab("vouchers");
    // Phase D — le « détail lot » devient adressable : /app/vouchers/<batchId>
    // (push → le Retour du navigateur revient à la liste). L'effet de
    // synchronisation ci-dessous ré-applique le filtre à l'arrivée du push.
    router.push(viewToPath("vouchers", batch.id), { scroll: false });
  }

  // Clic sur le #lot d'une ligne de la table vouchers (colonne « Lot ») :
  // filtre la liste sur le lot SANS toucher au filtre détenteur (comportement
  // d'origine — extrait du JSX de la table).
  function filterByBatch(batchId: string) {
    setSearchInput(batchId);
    setSearch(batchId);
    setStatusFilter("all");
    setProfileFilter("all");
    setPage(1);
  }

  // Charge les vouchers ACTIFS d'un lot pour l'impression — parité avec le
  // badge « actifs » du lot (agrégat backend : en ligne = consommé) : les
  // tickets utilisés, expirés, désactivés ou en session ne sortent PAS.
  // Pagination obligatoire : /api/vouchers plafonne pageSize à 200.
  async function fetchActiveBatchVouchers(batchId: string): Promise<PagedUsers> {
    const pageSize = 200;
    const first = await api<PagedUsers>("/api/vouchers", {
      params: { search: batchId, page: 1, pageSize, status: "active" },
    });
    const pages = Math.ceil(first.total / pageSize);
    for (let p = 2; p <= pages; p++) {
      const next = await api<PagedUsers>("/api/vouchers", {
        params: { search: batchId, page: p, pageSize, status: "active" },
      });
      first.data.push(...next.data);
    }
    return first;
  }

  // « Imprimer le lot » (liste simple) : seuls les tickets ACTIFS du lot
  // sont imprimés (utilisés/expirés/désactivés exclus).
  async function printBatch(batch: BatchWithStats) {
    setPrintingBatchId(batch.id);
    try {
      const res = await fetchActiveBatchVouchers(batch.id);
      if (res.total === 0) {
        // Lot purgé (plus aucun voucher en base) ou tous consommés — on distingue.
        const all = await api<PagedUsers>("/api/vouchers", {
          params: { search: batch.id, page: 1, pageSize: 1 },
        });
        if (all.total === 0) {
          toast.info(tf("vouchers.batches.purgedToast", { batch: shortBatch(batch.id) }));
        } else {
          toast.info(tf("vouchers.batches.noActiveToast", { batch: shortBatch(batch.id) }));
        }
        return;
      }
      setPrintVouchers(await fetchPrintVouchers(res.data.map((v) => v.id)));
      setPrintTitle(
        tf("vouchers.batches.printTitleActive", { batch: shortBatch(batch.id), n: res.total }),
      );
      setPrintBatchId(batch.id);
      setPrintOpen(true);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t("vouchers.printFailed"));
    } finally {
      setPrintingBatchId(null);
    }
  }

  // Quick print (F12) : réimprime directement le dernier lot imprimé
  // (localStorage "mikcloud-last-batch") — tickets ACTIFS uniquement.
  // Aucun lot récent → toast informatif.
  async function quickPrint() {
    let lastBatch: string | null = null;
    try {
      lastBatch = window.localStorage.getItem(LAST_BATCH_STORAGE_KEY);
    } catch {
      /* localStorage indisponible */
    }
    if (!lastBatch) {
      toast.info(t("vouchers.quickPrintNone"));
      return;
    }
    setPrintingBatchId(lastBatch);
    try {
      // Même mécanique que printBatch : recherche par identifiant de lot,
      // filtre actifs côté serveur + pagination (plafond 200/page).
      const res = await fetchActiveBatchVouchers(lastBatch);
      if (res.total === 0) {
        const all = await api<PagedUsers>("/api/vouchers", {
          params: { search: lastBatch, page: 1, pageSize: 1 },
        });
        if (all.total === 0) {
          toast.info(tf("vouchers.batches.purgedToast", { batch: shortBatch(lastBatch) }));
        } else {
          toast.info(tf("vouchers.batches.noActiveToast", { batch: shortBatch(lastBatch) }));
        }
        return;
      }
      setPrintVouchers(await fetchPrintVouchers(res.data.map((v) => v.id)));
      setPrintTitle(
        tf("vouchers.batches.printTitleActive", { batch: shortBatch(lastBatch), n: res.total }),
      );
      setPrintBatchId(lastBatch);
      setPrintOpen(true);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t("vouchers.printFailed"));
    } finally {
      setPrintingBatchId(null);
    }
  }

  // Refonte (F3) — impression multi-formats du lot (grille A4 / thermique
  // 58/80 mm) : mêmes règles que printBatch — seuls les tickets ACTIFS sortent
  // (utilisés, expirés, désactivés ou en session exclus).
  async function printMulti(batch: BatchWithStats) {
    setMultiPrintBatch(batch);
    setMultiPrintVouchers(null);
    try {
      const res = await fetchActiveBatchVouchers(batch.id);
      if (res.total === 0) {
        // Lot purgé (plus aucun voucher en base) ou tous consommés — on distingue.
        const all = await api<PagedUsers>("/api/vouchers", {
          params: { search: batch.id, page: 1, pageSize: 1 },
        });
        if (all.total === 0) {
          toast.info(tf("vouchers.batches.purgedToast", { batch: shortBatch(batch.id) }));
        } else {
          toast.info(tf("vouchers.batches.noActiveToast", { batch: shortBatch(batch.id) }));
        }
        setMultiPrintBatch(null);
        return;
      }
      // N°22 — même canal tracé que printBatch : les listes masquent les codes
      // revendeur, l'impression multi-formats les récupère via
      // POST /api/vouchers/print (remise tracée, propriété inchangée).
      setMultiPrintVouchers(await fetchPrintVouchers(res.data.map((v) => v.id)));
    } catch (error) {
      setMultiPrintBatch(null);
      toast.error(error instanceof Error ? error.message : t("vouchers.printFailed"));
    }
  }

  // Refonte (F3) — export CSV des lots : mêmes filtres que la liste, sans
  // pagination (endpoint dédié ; séparateur « ; » + BOM Excel côté back).
  async function exportBatches() {
    setExportingBatches(true);
    try {
      await apiDownload("/api/vouchers/batches/export", "mikcloud-lots.csv", {
        search: batchSearch || undefined,
        routerId: batchRouterParam,
        status: batchStatusParam,
        holder: batchHolderParam,
        channel: batchChannelParam,
      });
      toast.success(t("common.exportDownloaded"));
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t("common.exportFailed"));
    } finally {
      setExportingBatches(false);
    }
  }

  function resetBatchFilters() {
    setBatchSearchInput("");
    setBatchSearch("");
    setBatchRouterFilter("all");
    setBatchChannelFilter("all");
    // v2 — le défaut du filtre statut est « stock » (Vivants).
    setBatchStatusFilter("stock");
    setBatchHolderFilter("all");
    setBatchPage(1);
  }

  // Fiche 360° — actions déléguées aux mécanismes existants (la fiche se ferme).
  function detailToTransfer(batch: BatchWithStats) {
    setDetailBatch(null);
    setTransferBatch(batch);
  }
  function detailToPrintMulti(batch: BatchWithStats) {
    setDetailBatch(null);
    void printMulti(batch);
  }
  function detailToPrintSimple(batch: BatchWithStats) {
    setDetailBatch(null);
    void printBatch(batch);
  }
  function detailToView(batch: BatchWithStats) {
    setDetailBatch(null);
    viewBatchVouchers(batch);
  }
  function detailToDelete(batch: BatchWithStats) {
    setDetailBatch(null);
    setDeletingBatch(batch);
  }

  const hasFilters =
    scopeRouterId !== null ||
    search !== "" ||
    statusFilter !== "all" ||
    profileFilter !== "all" ||
    holderFilter !== "all";
  // Refonte v2 — filtres « fiche de vie » des lots (recherche + site + canal +
  // cycle + détenteur). Le DÉFAUT du statut est désormais « stock » (Vivants) :
  // un filtre n'est « actif » que s'il diverge de ce défaut.
  const hasBatchFilters =
    batchSearch !== "" ||
    batchRouterFilter !== "all" ||
    batchChannelFilter !== "all" ||
    batchStatusFilter !== "stock" ||
    batchHolderFilter !== "all";
  // Compteur pour le badge du bouton « Filtres » mobile.
  const batchActiveFilterCount = [
    batchSearch !== "",
    batchRouterFilter !== "all",
    batchChannelFilter !== "all",
    batchStatusFilter !== "stock",
    batchHolderFilter !== "all",
  ].filter(Boolean).length;

  // Spinners de ligne (mutation en vol sur le voucher concerné).
  const reprisePendingId = repriseMutation.isPending ? (repriseMutation.variables?.id ?? null) : null;
  const resyncPendingId = resyncMutation.isPending ? (resyncMutation.variables?.voucher.id ?? null) : null;

  return (
    <div className="space-y-4 sm:space-y-6">
      <PageHeader
        title={t("vouchers.title")}
        description={t("vouchers.description")}
        actions={
          <>
            <Button variant="outline" className="h-10" onClick={() => void quickPrint()}>
              <Zap className="size-4" />
              {t("vouchers.quickPrint")}
            </Button>
            <Button className="h-10" onClick={() => setGenOpen(true)}>
              <TicketPlus className="size-4" />
              {t("vouchers.generate")}
            </Button>
          </>
        }
      />

      {/* Onglets Vouchers / Lots (traçabilité) */}
      <Tabs value={tab} onValueChange={(value) => setTab(value as "vouchers" | "batches")}>
        <TabsList>
          <TabsTrigger value="vouchers" className="gap-1.5">
            <Ticket className="size-3.5" aria-hidden />
            {t("vouchers.tabVouchers")}
          </TabsTrigger>
          <TabsTrigger value="batches" className="gap-1.5">
            <Layers className="size-3.5" aria-hidden />
            {t("vouchers.tabBatches")}
          </TabsTrigger>
        </TabsList>
      </Tabs>

      {/* Onglet Vouchers — stats, filtres, table, pagination (présentation extraite) */}
      {tab === "vouchers" && (
        <VouchersTab
          statsLoading={statsLoading}
          activeCount={activeCount}
          usedCount={usedCount}
          expiredCount={expiredCount}
          allocatedCount={allocatedCount}
          stockValue={stockValue}
          /* N°191 — loupe routeur (rail + KPI scopés). */
          routers={routers}
          scopeRouter={scopeRouterId ?? ""}
          onScopeRouter={applyScope}
          scopeCounts={stockByRouter}
          scopeTotal={totalActive}
          scopeName={scopedRouter?.name ?? null}
          searchInput={searchInput}
          onSearchInput={setSearchInput}
          statusFilter={statusFilter}
          onStatusFilter={(value) => {
            setStatusFilter(value);
            setPage(1);
          }}
          holderFilter={holderFilter}
          onHolderFilter={(value) => {
            setHolderFilter(value);
            setPage(1);
          }}
          profileFilter={profileFilter}
          onProfileFilter={(value) => {
            setProfileFilter(value);
            setPage(1);
          }}
          profiles={profiles}
          isLoading={isLoading}
          isFetching={isFetching}
          vouchers={vouchers}
          hasFilters={hasFilters}
          revealed={revealed}
          onToggleReveal={toggleReveal}
          printingVoucherId={printingVoucherId}
          reprisePendingId={reprisePendingId}
          resyncPendingId={resyncPendingId}
          onGenerate={() => setGenOpen(true)}
          onCopyCode={(voucher) => void copyCode(voucher)}
          onPrintOne={(voucher) => void printOne(voucher)}
          onReprise={setReprising}
          onResync={(voucher, action) => resyncMutation.mutate({ voucher, action })}
          onDelete={setDeleting}
          onFilterByBatch={filterByBatch}
          totalCount={totalCount}
          page={page}
          onSetPage={setPage}
          pageSize={pageSize}
          onSetPageSize={(size) => {
            setPageSize(size);
            setPage(1);
          }}
        />
      )}

      {/* ----------------------------------------------------------------- */}
      {/* Onglet Lots — traçabilité complète des générations de vouchers     */}
      {/* ----------------------------------------------------------------- */}
      {tab === "batches" && (
        <BatchesTab
          batchSummary={batchSummary}
          firstLoading={batchesFirstLoading}
          batchSearchInput={batchSearchInput}
          onBatchSearchInput={setBatchSearchInput}
          batchRouterFilter={batchRouterFilter}
          onBatchRouterFilter={(value) => {
            setBatchRouterFilter(value);
            setBatchPage(1);
          }}
          batchChannelFilter={batchChannelFilter}
          onBatchChannelFilter={(value) => {
            setBatchChannelFilter(value);
            setBatchPage(1);
          }}
          batchStatusFilter={batchStatusFilter}
          onBatchStatusFilter={(value) => {
            setBatchStatusFilter(value);
            setBatchPage(1);
          }}
          batchHolderFilter={batchHolderFilter}
          onBatchHolderFilter={(value) => {
            setBatchHolderFilter(value);
            setBatchPage(1);
          }}
          routers={routers}
          resellers={resellers}
          filtersOpen={batchFiltersOpen}
          onFiltersOpenChange={setBatchFiltersOpen}
          hasBatchFilters={hasBatchFilters}
          batchActiveFilterCount={batchActiveFilterCount}
          exportingBatches={exportingBatches}
          onExportBatches={() => void exportBatches()}
          onResetFilters={resetBatchFilters}
          batches={batches}
          isFetching={batchesFetching}
          batchTotal={batchTotal}
          batchPage={batchPage}
          onSetBatchPage={setBatchPage}
          batchPageSize={batchPageSize}
          onSetBatchPageSize={(size) => {
            setBatchPageSize(size);
            setBatchPage(1);
          }}
          printingBatchId={printingBatchId}
          multiPrintBatch={multiPrintBatch}
          onGenerate={() => setGenOpen(true)}
          onOpenDetail={setDetailBatch}
          onViewVouchers={viewBatchVouchers}
          onPrintMulti={(batch) => void printMulti(batch)}
          onPrintSimple={(batch) => void printBatch(batch)}
          onTransfer={setTransferBatch}
          onDeleteBatch={setDeletingBatch}
        />
      )}

      {/* Wizard de génération — 3 étapes (Forfait → Codes → Récap) */}
      <VoucherWizardDialog
        open={genOpen}
        onOpenChange={setGenOpen}
        profiles={profiles ?? []}
        routers={routers ?? []}
        resellers={resellers ?? []}
        currency={currency}
        tenantName={tenantName}
        isPending={generateMutation.isPending}
        isError={generateMutation.isError}
        error={generateMutation.error}
        onSubmit={(payload) => generateMutation.mutate(payload)}
      />

      {/* Dialogue d'impression (mode modèle F2 si des templates existent) */}
      <UcPrintDialog
        open={printOpen}
        onOpenChange={setPrintOpen}
        vouchers={printVouchers}
        title={printTitle}
        tenantName={tenantName}
        profiles={profiles ?? []}
        templates={templates ?? []}
        batchId={printBatchId}
      />

      {/* N°18 — Transfert de stock du lot (distribution / retour de stock) */}
      <VoucherTransferDialog
        key={transferBatch?.id ?? "transfer-closed"}
        batch={transferBatch}
        resellers={resellers ?? []}
        currency={currency}
        onOpenChange={(open) => !open && setTransferBatch(null)}
      />

      {/* Refonte — fiche 360° du lot (drawer) : cycle de vie, possession, actions */}
      <BatchDetailSheet
        batch={detailBatch}
        currency={currency}
        onOpenChange={(open) => !open && setDetailBatch(null)}
        onTransfer={detailToTransfer}
        onPrintMulti={detailToPrintMulti}
        onPrintSimple={detailToPrintSimple}
        onViewVouchers={detailToView}
        onDelete={detailToDelete}
      />

      {/* Refonte (F3) — impression multi-formats du lot : grille A4 / thermique 58/80 */}
      <BatchPrintDialog
        open={multiPrintBatch !== null}
        onOpenChange={(open) => {
          if (!open) {
            setMultiPrintBatch(null);
            setMultiPrintVouchers(null);
          }
        }}
        vouchers={multiPrintVouchers}
        title={tf("vouchers.batches.printMultiTitle", { batch: multiPrintBatch ? shortBatch(multiPrintBatch.id) : "" })}
        tenantName={tenantName}
        currency={currency}
      />

      {/* N°23 (W6) — confirmation reprise gérant (retour au stock direct). */}
      <RepriseConfirmDialog
        voucher={reprising}
        isPending={repriseMutation.isPending}
        onOpenChange={(open) => !open && setReprising(null)}
        onConfirm={(voucher) => repriseMutation.mutate(voucher)}
      />

      {/* Confirmation suppression voucher */}
      <DeleteVoucherConfirmDialog
        voucher={deleting}
        isPending={deleteMutation.isPending}
        onOpenChange={(open) => !open && setDeleting(null)}
        onConfirm={(voucher) => deleteMutation.mutate(voucher)}
      />

      {/* Confirmation suppression d'un lot entier */}
      <DeleteBatchConfirmDialog
        batch={deletingBatch}
        isPending={batchDeleteMutation.isPending}
        onOpenChange={(open) => !open && setDeletingBatch(null)}
        onConfirm={(batch) => batchDeleteMutation.mutate(batch)}
      />
    </div>
  );
}
