// Types partagés SpotCloud — alignés sur le contrat API Go (voir worklog.md)

export type RouterMode = "simulated" | "real";
export type RouterStatus = "online" | "offline";
export type VoucherStatus = "active" | "used" | "expired" | "disabled";
export type UserKind = "regular" | "voucher";
export type ResellerStatus = "active" | "disabled";
export type TransactionType = "credit" | "sale";
export type SaleChannel = "direct" | "reseller";

export interface AuthUser {
  id: string;
  name: string;
  username: string;
  role: string;
}

export interface LoginResponse {
  token: string;
  user: AuthUser;
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
}

export interface PagedUsers {
  data: HotspotUser[];
  total: number;
  page: number;
  pageSize: number;
}

export interface GenerateVouchersRequest {
  count: number;
  profileId: string;
  routerId: string;
  prefix?: string;
  codeLength?: number;
  resellerId?: string;
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
  at: string;
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

export interface AppSettings {
  tenant: {
    name: string;
    currency: string;
    timezone: string;
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
  | "settings";
