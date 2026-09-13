// Constantes, options et helpers partagés entre le shell vouchers-view et
// ses onglets extraits (vouchers-tab, batches-tab, confirm-dialogs).
// Fichier délibérément sans dépendance React : données pures seulement.

export const PAGE_SIZE = 12;
export const BATCH_PAGE_SIZE = 10;

export const STATUS_OPTIONS = [
  { value: "all", labelKey: "common.allStatuses" },
  { value: "active", labelKey: "common.statusActive" },
  { value: "online", labelKey: "common.statusOnline" },
  { value: "used", labelKey: "common.statusUsed" },
  { value: "expired", labelKey: "common.statusExpired" },
  { value: "disabled", labelKey: "common.statusDisabled" },
];

// Refonte v2 — cycle de vie filtrable, DÉFAUT « Vivants » (le stock vivant
// d'abord) ; « Tous » reste disponible en fin de liste.
export const BATCH_STATUS_OPTIONS = [
  { value: "stock", labelKey: "vouchers.batches.life.vivants" },
  { value: "consumed", labelKey: "vouchers.batches.life.consumed" },
  { value: "expired", labelKey: "vouchers.batches.life.expired" },
  { value: "purged", labelKey: "vouchers.batches.life.purged" },
  { value: "all", labelKey: "common.allStatuses" },
];

export function shortBatch(batchId: string): string {
  return batchId.split("-").pop() || batchId;
}

// VouchersStats — N°74 — compteurs de stock renvoyés par GET /api/vouchers/stats
// (calcul serveur sur l'ensemble du stock, plus de plafond pageSize 200).
export type VouchersStats = {
  active: number;
  used: number;
  expired: number;
  disabled: number;
  allocated: number;
  stockValue: number;
  total: number;
};
