"use client";

// UX R6 (P3-a) — file d'attente de ventes hors-ligne.
//
// Quand le réseau tombe au comptoir, le revendeur doit pouvoir continuer à
// vendre : la vente est enregistrée localement (IndexedDB) puis REJOUÉE
// automatiquement au retour du réseau. Le backend est idempotent (409
// « déjà remis » sur un voucher déjà vendu, rendu ou expiré) : un replay ne
// peut donc jamais doubler un décompte ni créer de décompte fantôme.
//
// Choix techniques :
// - IndexedDB (pas localStorage) : asynchrone, structured clone, pas de
//   quota JSON ; dégradation douce — si IndexedDB est indisponible (SSR,
//   navigation privée stricte), toutes les fonctions résolvent sans effet :
//   la file n'existe pas, l'UX reste purement en ligne.
// - Zéro dépendance : un wrapper promise minimal suffit (3 opérations).
// - `put` avec keyPath `voucherId` : dédoublonnage naturel — reconfirmer un
//   même ticket hors-ligne remplace l'entrée, n'en ajoute pas une seconde.
// - Aucune fonction ne rejette : une erreur IndexedDB ne doit jamais casser
//   l'UI du comptoir.

export interface QueuedSale {
  voucherId: string;
  username: string;
  profileName: string;
  /** Prix affiché (sellingPrice || price) — purement informatif. */
  price: number;
  /** Date ISO de mise en file (FIFO au replay). */
  queuedAt: string;
}

const DB_NAME = "mikcloud-sell";
const STORE = "pending-sales";
const VERSION = 1;

let dbPromise: Promise<IDBDatabase | null> | null = null;

// N°59 — persistance du stockage (navigator.storage.persist()).
// Sans elle, le navigateur peut ÉVICTIONNER l'IndexedDB sous pression
// disque (Eviction en LRU quand le quota global se resserre) et emporter
// la file des ventes en attente — c'est-à-dire de l'argent réel. Avec elle,
// le stockage de l'app est marqué durable : il n'entre plus dans le calcul
// d'éviction (Chrome : accord automatique si PWA installée ou engagement
// élevé ; sinon promesse false, sans UI ni rejet). Idempotent et
// strictement silencieux : API absente (Safari < 15.2, vieux navigateurs)
// ou refus du navigateur = simple no-op, l'UX ne change jamais.
let persistRequested = false;

export function ensureStoragePersisted(): void {
  if (persistRequested || typeof navigator === "undefined") return;
  persistRequested = true; // une seule tentative par session suffit
  try {
    const maybe: unknown = navigator.storage?.persist?.();
    if (maybe instanceof Promise) maybe.catch(() => {}); // refus ≠ erreur
  } catch {
    /* API absente : dégradation douce */
  }
}

function openDb(): Promise<IDBDatabase | null> {
  if (typeof indexedDB === "undefined") return Promise.resolve(null);
  if (!dbPromise) {
    dbPromise = new Promise((resolve) => {
      try {
        const req = indexedDB.open(DB_NAME, VERSION);
        req.onupgradeneeded = () => {
          const db = req.result;
          if (!db.objectStoreNames.contains(STORE)) {
            db.createObjectStore(STORE, { keyPath: "voucherId" });
          }
        };
        req.onsuccess = () => resolve(req.result);
        req.onerror = () => resolve(null);
        req.onblocked = () => resolve(null);
      } catch {
        resolve(null);
      }
    });
  }
  return dbPromise;
}

function store(db: IDBDatabase, mode: IDBTransactionMode): IDBObjectStore {
  return db.transaction(STORE, mode).objectStore(STORE);
}

/** Met la vente en file (remplace une entrée existante du même voucher). */
export async function queueSale(entry: QueuedSale): Promise<void> {
  // N°59 — au moment précis où des ventes (de l'argent) entrent en zone
  // critique, on (re)demande la persistance du stockage.
  ensureStoragePersisted();
  const db = await openDb();
  if (!db) return;
  await new Promise<void>((resolve) => {
    const req = store(db, "readwrite").put(entry);
    req.onsuccess = () => resolve();
    req.onerror = () => resolve();
  });
}

/** Liste FIFO (plus anciennes d'abord). */
export async function listQueuedSales(): Promise<QueuedSale[]> {
  const db = await openDb();
  if (!db) return [];
  return new Promise((resolve) => {
    const req = store(db, "readonly").getAll();
    req.onsuccess = () => {
      const rows = (req.result ?? []) as QueuedSale[];
      resolve(rows.sort((a, b) => a.queuedAt.localeCompare(b.queuedAt)));
    };
    req.onerror = () => resolve([]);
  });
}

export async function removeQueuedSale(voucherId: string): Promise<void> {
  const db = await openDb();
  if (!db) return;
  await new Promise<void>((resolve) => {
    const req = store(db, "readwrite").delete(voucherId);
    req.onsuccess = () => resolve();
    req.onerror = () => resolve();
  });
}
