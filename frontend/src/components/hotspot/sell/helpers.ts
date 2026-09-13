"use client";

// Helpers du Mode Vente (sell-shell) — types de contrat, caches hors-ligne,
// regroupement du stock, vue persistée et utilitaires de présentation.
// Transfert PUR depuis sell-shell.tsx (aucune logique modifiée) : tout ce
// bloc vivait déjà au niveau module, hors du composant.

import { useEffect, useState } from "react";
import type { InfiniteData } from "@tanstack/react-query";
import { MousePointerClick, Printer, Wifi } from "lucide-react";

import { ApiError } from "@/lib/hotspot/api";

export interface SellVoucher {
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
export interface StockPage {
  items: SellVoucher[];
  total: number;
  hasMore: boolean;
}

export const STOCK_PAGE_SIZE = 60;

/** Phase D × P3-e — update optimiste du stock PAGINÉ : le filtre traverse
 * toutes les pages chargées (InfiniteData) ; la vue aplatie puis l'effet
 * snapshot persistent ensuite le localStorage — les DEUX caches (TanStack +
 * hors-ligne) restent alignés, sinon le fallback hors-ligne ré-afficherait
 * un ticket déjà vendu/rendu. */
export function filterPagedStock(
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

export interface SellMe {
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
export interface SellReturnResult {
  returned: number;
  credited: number;
  creditAfter: number;
  codes: string[];
}

/** N°21 — pair de transfert : revendeur actif du même compte (identité minimale). */
export interface SellPeer {
  id: string;
  name: string;
}

/** N°21 — réponse du transfert entre revendeurs (POST /api/sell/transfer). */
export interface SellTransferResult {
  transferred: number;
  credited: number;
  creditAfter: number;
  debited: number;
  target: SellPeer;
  codes: string[];
}

export function useOnline(): boolean {
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
export const NO_BATCH = "__nobatch__";
export const VIEW_STORAGE_KEY = "mikcloud-sell-view";

export interface SellBatchGroup {
  key: string;
  labelId: string;
  createdAt: string; // date de génération du lot (la plus ancienne du groupe)
  vouchers: SellVoucher[];
}

export interface SellProfileGroup {
  profileName: string;
  count: number;
  value: number; // valeur faciale cumulée (sellingPrice || price)
  batches: SellBatchGroup[]; // lot le plus régent en tête
  hasLots: boolean; // au moins un lot identifié → sous-groupes affichés
}

/** UX R1 — vue du stock persistée en localStorage : store externe lu via
 * useSyncExternalStore (pas de setState en effet — règle react-hooks), SSR
 * sûr (snapshot serveur = « profile »), synchro inter-onglets gratuite. */
export function subscribeView(callback: () => void) {
  window.addEventListener("mikcloud-view-change", callback);
  window.addEventListener("storage", callback);
  return () => {
    window.removeEventListener("mikcloud-view-change", callback);
    window.removeEventListener("storage", callback);
  };
}

export function getViewSnapshot(): "profile" | "recent" {
  try {
    return window.localStorage.getItem(VIEW_STORAGE_KEY) === "recent" ? "recent" : "profile";
  } catch {
    return "profile";
  }
}

export function getServerViewSnapshot(): "profile" | "recent" {
  return "profile";
}

export function shortBatchId(id: string): string {
  return id.split("-").pop() || id;
}

// UX R3 — un ticket dont la validité se termine dans moins de 48 h mérite un
// signal visuel : à vendre en priorité, ou à rendre au stock avant qu'il ne
// meure (un voucher expiré sort du stock sans recyclage possible).
export const EXPIRY_SOON_MS = 48 * 60 * 60 * 1000;

// UX R6 (P3-a) — ventes hors-ligne : snapshot du stock et du profil
// revendeur en localStorage à chaque fetch réussi. Hors-ligne, la vue
// continue d'afficher le dernier état connu (sinon, impossible de vendre :
// la liste serait vide) — la file IndexedDB prend le relais côté ventes.
export const STOCK_CACHE_KEY = "mikcloud-stock-cache";
export const ME_CACHE_KEY = "mikcloud-me-cache";

export function readCache<T>(key: string): T | null {
  try {
    const raw = window.localStorage.getItem(key);
    return raw ? (JSON.parse(raw) as T) : null;
  } catch {
    return null;
  }
}

export function writeCache(key: string, data: unknown): void {
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
export function isNetworkError(e: unknown): boolean {
  if (!(e instanceof ApiError)) return true; // TypeError fetch = réseau
  return e.status === 502 || e.status === 503 || e.status === 504;
}

export function expiresSoon(v: SellVoucher): boolean {
  if (!v.expiresAt) return false;
  const ms = new Date(v.expiresAt).getTime() - Date.now();
  return ms > 0 && ms <= EXPIRY_SOON_MS;
}

export function batchExpiringSoon(vouchers: SellVoucher[]): boolean {
  return vouchers.some(expiresSoon);
}

// P3-d — canaux de vente (audit R4) : clé i18n + icône du canal. Une vente
// sans canal tracé (historique pré-R4) était nécessairement tactile.
export const VIA_ORDER = ["sell_mode", "auto_connect", "sell_mode_paper"] as const;
export const VIA_KEYS: Record<string, string> = {
  sell_mode: "sell.viaTactile",
  auto_connect: "sell.viaAuto",
  sell_mode_paper: "sell.viaPaper",
};

export function viaIcon(via?: string) {
  if (via === "auto_connect") return Wifi;
  if (via === "sell_mode_paper") return Printer;
  return MousePointerClick;
}

export function fmtDay(iso: string, lang: string): string {
  const d = new Date(iso.length === 10 ? `${iso}T12:00:00Z` : iso);
  return Number.isNaN(d.getTime())
    ? ""
    : d.toLocaleDateString(lang === "en" ? "en-GB" : "fr-FR", { day: "2-digit", month: "short" });
}

export function groupStock(stock: SellVoucher[]): SellProfileGroup[] {
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
