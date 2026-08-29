// Types partagés MikCloud — alignés sur le contrat API Go (voir worklog.md)

export type RouterMode = "simulated" | "real" | "agent";
export type RouterStatus = "online" | "offline";
export type VoucherStatus = "active" | "used" | "expired" | "disabled";
export type UserKind = "regular" | "voucher";
export type ResellerStatus = "active" | "disabled";
export type TransactionType = "credit" | "sale";
export type SaleChannel = "direct" | "reseller";
export type AccountStatus = "active" | "disabled";

export interface AuthUser {
  id: string;
  name: string;
  username: string;
  role: string;
  /** Compte SaaS auquel l'utilisateur est rattaché (multi-tenant). */
  accountId?: string;
  accountName?: string;
}

export interface LoginResponse {
  token: string;
  user: AuthUser;
}

/** Charge utile d'inscription SaaS (POST /api/auth/register). */
export interface RegisterPayload {
  name: string;
  username: string;
  password: string;
  /** Clé d'invitation — optionnelle, requise si l'inscription est protégée (REGISTER_KEY). */
  key?: string;
}

export interface AuthResponse {
  token: string;
  user: AuthUser;
}

/** Compte client SaaS (GET /api/admin/accounts — admin plateforme uniquement). */
export interface AccountSummary {
  id: string;
  name: string;
  status: AccountStatus;
  createdAt: string;
  /** Identifiant du compte propriétaire (login). */
  owner: string;
  stats: {
    users: number;
    routers: number;
    sessions: number;
    sales30d: number;
    revenue30d: number;
  };
}

export interface RouterDevice {
  id: string;
  name: string;
  host: string;
  port: number;
  username: string;
  mode: RouterMode;
  status: RouterStatus;
  version: string;
  uptimeSec: number;
  cpuLoad: number;
  hotspotUsers: number;
  activeSessions: number;
  createdAt: string;
  /** Mode agent uniquement : aperçu du token (4 caractères). */
  tokenPreview?: string;
  /** Mode agent : dernier check-in (RFC3339, vide si jamais vu). */
  lastSeen?: string;
  /** Page de login du hotspot (optionnel) — cible des QR codes imprimés sur les vouchers. */
  hotspotLoginUrl?: string;
}

/** Réponse de création d'un routeur en mode agent (script + token à copier). */
export interface RouterAgentCreateResponse extends RouterDevice {
  agentToken: string;
  installScript: string;
  message: string;
}

/** État de provisionning d'un routeur agent (GET /api/routers/{id}/provision). */
export interface RouterProvisionInfo {
  mode: string;
  tokenPreview: string;
  provisioned: boolean;
  lastSeen: string;
  online: boolean;
  scheduler: string;
  scriptFile: string;
  note: string;
}

/** Réponse de rotation du token agent (POST /api/routers/{id}/rotate-token). */
export interface RouterRotateTokenResponse {
  agentToken: string;
  installScript: string;
}

export interface RouterStats {
  cpuLoad: number;
  memUsedPct: number;
  freeMemoryMb: number;
  totalMemoryMb: number;
  uptimeSec: number;
  version: string;
  activeSessions: number;
}

export interface RouterTestResult {
  ok: boolean;
  message: string;
  latencyMs: number;
  version: string;
}

export interface Profile {
  id: string;
  name: string;
  rateLimit: string;
  sessionTimeoutMin: number;
  sharedUsers: number;
  validityDays: number;
  price: number;
  dataQuotaMb: number;
  createdAt: string;
}

export interface HotspotUser {
  id: string;
  kind: UserKind;
  username: string;
  password: string;
  profileId: string;
  profileName: string;
  routerId: string;
  routerName: string;
  status: VoucherStatus;
  batchId: string;
  resellerId: string;
  resellerName: string;
  comment: string;
  bytesIn: number;
  bytesOut: number;
  uptimeUsedSec: number;
  createdAt: string;
  expiresAt: string;
  usedAt: string;
  price: number;
  /** Quota data appliqué sur le routeur (limit-bytes-total, en Mo ; 0 = illimité). */
  dataQuotaMb: number;
}

export interface PagedUsers {
  data: HotspotUser[];
  total: number;
  page: number;
  pageSize: number;
}

/** Mode utilisateur des vouchers généré (« User Mode » du User Manager MikroTik). */
export type VoucherUserMode = "userpass" | "same";

export interface GenerateVouchersRequest {
  count: number;
  profileId: string;
  routerId: string;
  prefix?: string;
  codeLength?: number;
  resellerId?: string;
  /** « userpass » (défaut) ou « same » : mot de passe = nom d'utilisateur. */
  userMode?: VoucherUserMode;
  /** Preset de caractères : "" (MikCloud sûr) | abc | ABC | aBc | 5ab | 5AB | 5aB. */
  charset?: string;
  /** Commentaire libre inscrit sur le routeur avec le n° de lot (64 car. max). */
  comment?: string;
  /** Quota data par voucher en Mo : undefined = hériter du profil, 0 = illimité, >0 = plafond. */
  dataQuotaMb?: number;
}

export interface GenerateVouchersResponse {
  batchId: string;
  vouchers: HotspotUser[];
  totalCost: number;
}

export interface HotspotSession {
  id: string;
  userId: string;
  username: string;
  profileName: string;
  routerId: string;
  routerName: string;
  ip: string;
  mac: string;
  startedAt: string;
  uptimeSec: number;
  bytesIn: number;
  bytesOut: number;
}

export interface Reseller {
  id: string;
  name: string;
  username: string;
  phone: string;
  credit: number;
  vouchersSold: number;
  revenue: number;
  status: ResellerStatus;
  createdAt: string;
}

export interface Transaction {
  id: string;
  type: TransactionType;
  resellerId: string;
  resellerName: string;
  amount: number;
  note: string;
  at: string;
}

export interface Activity {
  id: string;
  type: "router" | "user" | "voucher" | "reseller" | "session" | "system";
  message: string;
  at: string;
}

export interface Sale {
  id: string;
  amount: number;
  profileName: string;
  count: number;
  channel: SaleChannel;
  resellerName: string;
  routerId: string;
  routerName: string;
  batchId: string;
  at: string;
}

export interface Batch {
  id: string;
  profileId: string;
  profileName: string;
  routerId: string;
  routerName: string;
  count: number;
  unitPrice: number;
  totalCost: number;
  /** Quota data porté par chaque voucher du lot (Mo, 0 = illimité). */
  dataQuotaMb: number;
  channel: SaleChannel;
  resellerId: string;
  resellerName: string;
  createdAt: string;
}

export interface BatchWithStats extends Batch {
  remaining: number;
  active: number;
  used: number;
  expired: number;
  disabled: number;
}

export interface PagedBatches {
  data: BatchWithStats[];
  total: number;
  page: number;
  pageSize: number;
}

export interface SiteOverview {
  routerId: string;
  routerName: string;
  status: RouterStatus;
  activeSessions: number;
  hotspotUsers: number;
  activeVouchers: number;
  salesToday: number;
  revenue30d: number;
}

export type AccountingPeriod = "day" | "week" | "month";

export interface AccountingData {
  period: AccountingPeriod;
  routerId: string;
  series: { label: string; revenue: number; sales: number }[];
  byRouter: { routerId: string; routerName: string; revenue: number; sales: number; share: number }[];
  totals: { revenue: number; sales: number; avgTicket: number };
}

export interface DashboardData {
  kpis: {
    activeSessions: number;
    totalUsers: number;
    activeVouchers: number;
    salesToday: number;
    revenue30d: number;
    routersOnline: number;
    routersTotal: number;
    onlineNow: number;
  };
  sites: SiteOverview[];
  sessionsTimeline: { t: string; value: number }[];
  revenueByDay: { day: string; value: number }[];
  topProfiles: { name: string; users: number; total: number }[];
  recentActivity: Activity[];
}

export interface ReportsData {
  revenueByDay: { day: string; value: number }[];
  salesByProfile: { name: string; count: number; revenue: number }[];
  trafficByDay: { day: string; bytesIn: number; bytesOut: number }[];
  voucherStatus: { active: number; used: number; expired: number; disabled: number };
  totals: { revenue: number; sales: number; avgTicket: number };
}

/* ─── Notifications (GET/PUT /api/notifications, POST /api/notifications/test) ─── */

/** Réglages du module Notifications. Les secrets ne sont jamais renvoyés : les
 * champs « *Set » indiquent uniquement qu'une valeur est déjà configurée côté serveur. */
export interface NotifSettings {
  /** Interrupteur général : aucune notification n'est envoyée s'il est désactivé. */
  enabled: boolean;
  // Telegram
  telegramEnabled: boolean;
  telegramBotTokenSet: boolean;
  telegramChatId: string;
  // WhatsApp Cloud API (Meta)
  whatsappEnabled: boolean;
  whatsappTokenSet: boolean;
  whatsappPhoneId: string;
  /** Numéro destinataire, format international sans « + » ni espaces (ex. 2250700000000). */
  whatsappTo: string;
  // Email SMTP
  emailEnabled: boolean;
  smtpHost: string;
  smtpPort: number;
  smtpUser: string;
  smtpPassSet: boolean;
  emailTo: string;
  // Règles d'alerte
  /** Routeur considéré hors ligne après N secondes sans check-in (défaut 135 = 3 × 45 s). */
  offlineAfterSec: number;
  /** Alerte quand le stock de vouchers actifs passe sous ce seuil (défaut 25). */
  lowStockThreshold: number;
  /** Rapport quotidien — ventes du jour, nouveaux utilisateurs, routeurs en ligne, stock restant. */
  dailyReport: boolean;
  /** Heure d'envoi du rapport, 0-23 UTC (= heure d'Abidjan, GMT+0). */
  reportHour: number;
}

/** Canal de notification (corps de POST /api/notifications/test). */
export type NotifChannel = "telegram" | "whatsapp" | "email";

/** Entrée d'historique (GET /api/notifications/log — 50 plus récentes d'abord). */
export interface NotifLogEntry {
  id: string;
  channel: NotifChannel | "system";
  kind: "router_offline" | "router_back" | "low_stock" | "daily_report" | "test" | "settings";
  title: string;
  status: "sent" | "error";
  error: string;
  at: string; // RFC3339
}

export interface AppSettings {
  tenant: {
    name: string;
    currency: string;
    timezone: string;
    /** Lien marchand Wave CI (pay.wave.com) — composé avec /amount/<montant>/. */
    waveLink?: string;
  };
  plan: {
    name: string;
    maxRouters: string;
    maxUsers: string;
  };
}

export type ViewId =
  | "dashboard"
  | "sessions"
  | "users"
  | "vouchers"
  | "profiles"
  | "resellers"
  | "routers"
  | "reports"
  | "accounts"
  | "notifications"
  | "settings";
