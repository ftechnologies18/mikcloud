// Types partagés MikCloud — alignés sur le contrat API Go (voir worklog.md)

import type { TeamRole } from "./roles";

export type { TeamRole } from "./roles";

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

/* ─── N°7 : équipe & rôles (GET/POST /api/team, PUT/DELETE /api/team/{id}) ─── */

/** Membre d'équipe (vue publique — jamais de hash). */
export interface TeamMember {
  id: string;
  name: string;
  username: string;
  role: TeamRole | string;
  createdAt: string;
}

/** Payload de création d'un membre (owner uniquement). */
export interface TeamMemberPayload {
  name: string;
  username: string;
  password?: string;
  role: string;
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
  /** F (signup enrichi) — contact propriétaire + segmentation géographique. */
  email: string;
  /** WhatsApp de préférence, format E.164 sans + (chiffres uniquement). */
  phone: string;
  /** Code ISO 3166-1 alpha-2 (CI, SN, NG…) ou "other". */
  country: string;
  /** Ville d'activité (texte libre). */
  city: string;
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
  /** Santé de l'abonnement SaaS du compte (console plateforme). */
  subscription?: "active" | "expired" | "essai";
  /** F (signup enrichi) — contact propriétaire + segmentation géographique. */
  email?: string;
  phone?: string;
  country?: string;
  city?: string;
  stats: {
    users: number;
    routers: number;
    sessions: number;
    sales30d: number;
    revenue30d: number;
  };
}

/** Abonnement SaaS d'un compte (état réel, calculé côté serveur). */
export interface SubscriptionInfo {
  /** "" (bêta) | essentiel | illimite */
  planId: string;
  /** Libellé court (Bêta, Essentiel, Illimité). */
  planName: string;
  /** Statut effectif : none | active | expired. */
  status: "none" | "active" | "expired";
  periodStart: string;
  periodEnd: string;
  lastAmountFcfa: number;
  /** Routeurs couverts par la période (Essentiel ; 0 = non plafonné). */
  routerSlots: number;
  /** Date du dernier paiement marqué par la plateforme (vide = en attente). */
  lastPaidAt?: string;
  /** Nombre de routeurs actuellement enregistrés du compte. */
  routerCount: number;
  /** Montant indicatif de la période en cours (formules catalogue). */
  amountFcfa: number;
}

/** Fiche détaillée d'un compte client (GET /api/admin/accounts/{id}). */
export interface AccountDetail {
  id: string;
  name: string;
  status: AccountStatus;
  createdAt: string;
  owner: {
    id: string;
    name: string;
    username: string;
    role: string;
    createdAt: string;
    owner: boolean;
  } | null;
  team: {
    id: string;
    name: string;
    username: string;
    role: string;
    createdAt: string;
    owner: boolean;
  }[];
  subscription: SubscriptionInfo;
  stats: {
    users: number;
    routers: number;
    routersOnline: number;
    sessions: number;
    sales30d: number;
    revenue30d: number;
    vouchersAvailable: number;
  };
  routers: {
    id: string;
    name: string;
    mode: string;
    status: string;
    users: number;
    lastSeen: string;
  }[];
  activity: PlatformActivityRow[];
}

/** Corps de PUT /api/admin/accounts/{id}/subscription (attribution plateforme). */
export interface SubscriptionUpdatePayload {
  planId: "essentiel" | "illimite" | "essai";
  /** Durée en mois (défaut : 1 essentiel, 12 illimité). */
  months?: number;
  /** Routeurs couverts (Essentiel ; défaut = quota actuel sinon routeurs enregistrés). */
  routerSlots?: number;
  /** Marquer la période comme payée maintenant. */
  markPaid?: boolean;
  /** Note libre tracée dans le journal. */
  note?: string;
}

/** KPIs globaux du SaaS — console plateforme (admin plateforme uniquement). */
export interface PlatformOverview {
  accounts: { total: number; active: number; disabled: number; new30d: number };
  routers: { total: number; online: number };
  hotspotUsers: number;
  sessions: number;
  sales30d: number;
  revenue30d: number;
  subscriptions: { active: number; expired: number; essai: number };
  growth: { month: string; accounts: number }[];
  topAccounts: {
    id: string;
    name: string;
    status: string;
    revenue30d: number;
    sales30d: number;
    users: number;
    routers: number;
    subscription: string;
  }[];
  registerOpen: boolean;
}

/** Entrée du journal d'activité transverse (tous comptes — plateforme). */
export interface PlatformActivityRow {
  id: string;
  accountId: string;
  accountName: string;
  type: string;
  message: string;
  at: string;
  actorId?: string;
  actorName?: string;
}

/** Demande de souscription / renouvellement d'abonnement (verrou du cycle de
 * facturation) — créée par POST /api/subscription côté client. */
export interface BillingRequest {
  id: string;
  accountId: string;
  planId: string;
  planName: string;
  amountFcfa: number;
  periodLabel: string;
  routerCount: number;
  /** Référence de paiement publique (MC-XXXXXXXX) — appariement webhook Wave. */
  ref: string;
  status: "pending" | "done" | "cancelled";
  createdAt: string;
  resolvedAt?: string;
  resolvedBy?: string;
  note?: string;
  /** manual (plateforme) | wave (webhook) — présent sur les demandes traitées. */
  paidVia?: string;
}

/** Demande enrichie du nom/statut du compte client (liste plateforme). */
export interface BillingRequestRow extends BillingRequest {
  accountName: string;
  accountStatus: string;
}

export interface BillingRequestsResponse {
  requests: BillingRequestRow[];
  pending: number;
}

/** Membre de l'équipe plateforme (super-admins MikCloud). */
export interface PlatformTeamMember {
  id: string;
  name: string;
  username: string;
  role: string;
  createdAt: string;
  self: boolean;
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
  /** F8 : modèle de la carte (ex. « RB2011UiAS ») — vide si non rapporté. */
  boardName?: string;
  /** F8 : espace disque libre en Mo (0 = inconnu). */
  freeHddMb?: number;
  /** F8 : espace disque total en Mo (0 = inconnu). */
  totalHddMb?: number;
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

/** Mode d'expiration cloud (F1) : « notify » désactive au routeur, « remove » supprime. */
export type ProfileExpiryMode = "notify" | "remove";

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
  /** F1 : comportement à l'expiration (défaut « notify »). */
  expMode: ProfileExpiryMode;
  /** F1 : période de grâce en minutes avant expiration effective (0 = immédiat, max 43200). */
  gracePeriodMin: number;
  /** F1 : verrouiller l'utilisateur à 1 session à la fois. */
  lockUser: boolean;
  /** F13 : prix de vente affiché sur le voucher (0 = même prix que price). */
  sellingPrice: number;
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
  /** F13 : prix de vente copié du profil à la génération ({{price}} du voucher = sellingPrice || price). */
  sellingPrice?: number;
  /** Quota data appliqué sur le routeur (limit-bytes-total, en Mo ; 0 = illimité). */
  dataQuotaMb: number;
}

export interface PagedUsers {
  data: HotspotUser[];
  total: number;
  page: number;
  pageSize: number;
}

// ─── F2 — Modèles (templates) de vouchers ───

export type VoucherFormat = "a4" | "58mm" | "80mm";

/** Modèle de voucher — le bodyHtml est rendu côté client à l'impression (F2). */
export interface VoucherTemplate {
  id: string;
  name: string;
  format: VoucherFormat;
  bodyHtml: string;
  isDefault: boolean;
  createdAt: string;
}

// ─── F3 — Journal utilisateurs ───

export type UserLogAction = "login" | "logout" | "expire" | "kick";

export interface UserLog {
  id: string;
  userId: string;
  username: string;
  action: UserLogAction;
  routerId: string;
  routerName: string;
  ip: string;
  mac: string;
  at: string;
}

export interface PagedUserLogs {
  data: UserLog[];
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
  /** F13 : coût (= price × count) — Amount conserve sa sémantique de coût d'origine. */
  cost?: number;
  /** F13 : total vente (= (sellingPrice || price) × count). */
  sellingTotal?: number;
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

/* ─── N°10 : affluence par tranche horaire (GET /api/stats/hourly?days=7|14|30) ─── */

/** Une ligne de la heatmap : un jour local, 24 compteurs de connexions. */
export interface HourlyStatsRow {
  date: string; // "YYYY-MM-DD" (fuseau du compte)
  hours: number[]; // 24 valeurs
}

/** Agrégats horaires réels — connexions depuis les UserLogs, CA depuis les Sales. */
export interface HourlyStats {
  timezone: string;
  days: number;
  rows: HourlyStatsRow[]; // oldest → today
  loginsByHour: number[]; // total connexions/heure sur la fenêtre (24)
  salesByHour: number[]; // CA cumulé/heure sur la fenêtre (24)
  maxCell: number;
  totalLogins: number;
  totalSales: number;
  peakHour: number; // 0-23, heure locale de pic
  generatedAt: string;
}

export interface ReportsData {
  revenueByDay: { day: string; value: number }[];
  salesByProfile: { name: string; count: number; revenue: number }[];
  trafficByDay: { day: string; bytesIn: number; bytesOut: number }[];
  voucherStatus: { active: number; used: number; expired: number; disabled: number };
  totals: { revenue: number; sales: number; avgTicket: number };
  /** F13 : bloc marge (30 jours glissants) — optionnel tant que le backend P2 n'est pas déployé. */
  margin?: ReportsMargin;
}

/** Marge par profil (F13). */
export interface ReportsMarginByProfile {
  name: string;
  sold: number;
  revenue: number;
  cost: number;
  margin: number;
}

/** Bloc marge des rapports (F13) : prix de vente vs coût. */
export interface ReportsMargin {
  revenue: number;
  cost: number;
  margin: number;
  marginPct: number;
  byProfile: ReportsMarginByProfile[];
}

export type ExpiryPolicyMode = "keep" | "remove";

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
    /** F2 : nom DNS du hotspot affiché sur les vouchers (ex. wifi.mondomaine.ci). */
    dnsName?: string;
    /** F2 : logo du tenant (data URL image ≤ 300 Ko) affiché sur les vouchers. */
    logoUrl?: string;
    /** F1/F5 : politique de nettoyage des utilisateurs expirés (défaut « keep »). */
    expiryPolicyMode?: ExpiryPolicyMode;
    /** F1/F5 : suppression après N jours (1-365, défaut 30) quand mode = remove. */
    expiryPolicyAfterDays?: number;
  };
  plan: {
    name: string;
    maxRouters: string;
    maxUsers: string;
  };
  /** État d'abonnement SaaS (optionnel : absents des réponses d'avant la facturation). */
  subscription?: Subscription;
}

/** Formule d'abonnement MikCloud (catalogue GET /api/plans). */
export interface SaasPlan {
  id: "essentiel" | "illimite" | string;
  name: string;
  priceFcfa: number;
  period: "mois" | "an";
  /** true : prix × nombre de routeurs enregistrés (formule Essentiel). */
  perRouter: boolean;
  /** true : routeurs illimités (formule Illimité). */
  unlimited: boolean;
  tagline: string;
  badge?: string;
}

/** État d'abonnement SaaS d'un compte. planId vide = ère bêta. */
export interface Subscription {
  planId: string;
  status: "active" | "expired" | "";
  periodStart: string; // RFC3339
  periodEnd: string; // RFC3339, "" = non expirant
  lastAmountFcfa: number;
}

/** Réponse GET /api/subscription — état + catalogue + assiette de facturation. */
export interface SubscriptionView {
  subscription: Subscription;
  /** Statut effectif calculé serveur : une période échue passe en « expired », puis « suspended » après 30j de grâce. */
  status: "active" | "expired" | "suspended" | "none";
  routerCount: number;
  currentAmountFcfa: number;
  plans: SaasPlan[];
  waveConfigured: boolean;
}

/** Réponse POST /api/subscription — DEMANDE de souscription (verrou facturation) :
 * l'abonnement n'est PAS activé côté client (renvoyé inchangé) ; la demande
 * est tracée dans le journal et l'activation est effectuée par la plateforme
 * après encaissement (PUT /api/admin/accounts/{id}/subscription). */
export interface SubscribeResponse {
  subscription: Subscription;
  amountFcfa: number;
  routerCount: number;
  periodLabel: string;
  /** Lien de paiement Wave de la PLATEFORME, pré-composé avec le montant ("" si non configuré). */
  waveLink: string;
  /** Toujours true : la demande attend l'encaissement puis l'activation plateforme. */
  pending: boolean;
}

// ─── F6 — Trafic temps réel ───

export interface IfaceTraffic {
  name: string;
  /** Compteurs cumulés. */
  rxBytes: number;
  txBytes: number;
  /** Débit calculé. */
  rxBps: number;
  txBps: number;
}

export interface TrafficPoint {
  /** RFC3339. */
  t: string;
  rxBps: number;
  txBps: number;
}

export interface RouterTraffic {
  routerId: string;
  accountId: string;
  updatedAt: string;
  interfaces: IfaceTraffic[];
  /** 60 derniers points (somme toutes interfaces). */
  history: TrafficPoint[];
}

// ─── F7 — IP Bindings ───

export type IPBindingType = "bypassed" | "blocked";

export interface IPBinding {
  id: string;
  routerId: string;
  mac: string;
  address: string;
  comment: string;
  type: IPBindingType;
  disabled: boolean;
  createdAt: string;
}

// ─── F8 — Status étendu + ping ───

export interface PingResult {
  queued: boolean;
  /** Résultat direct (simulated) — absent si queued. */
  ok?: boolean;
  target?: string;
  sent?: number;
  received?: number;
  lossPct?: number;
  minMs?: number;
  avgMs?: number;
  maxMs?: number;
  /** Résultat différé (agent) : identifiant de commande à poller via GET /api/commands/{id}. */
  commandId?: string;
}

export interface CommandStatus {
  id: string;
  kind: string;
  status: string;
  result: unknown;
}

// ─── F9 — Outils routeur (DHCP / hôtes / cookies / journal) ───

export interface DhcpLeaseRow {
  ip: string;
  mac: string;
  host: string;
  expires: string;
  status: string;
}

export interface HotspotHostRow {
  mac: string;
  ip: string;
  server: string;
  /** Durée de connexion en secondes. */
  uptime: number;
  authorized: boolean;
}

export interface HotspotCookieRow {
  user: string;
  mac: string;
  expires: string;
}

export interface RouterLogRow {
  time: string;
  topics: string;
  message: string;
}

/** Enveloppe commune des outils routeur (F9/F10) : queued=true tant que la commande agent n'est pas done. */
export interface ToolEnvelope<T> {
  queued: boolean;
  data: T[];
  updatedAt: string;
}

/** Lignes des 4 outils F9. */
export interface ToolRows {
  dhcp: DhcpLeaseRow[];
  hosts: HotspotHostRow[];
  cookies: HotspotCookieRow[];
  log: RouterLogRow[];
}

// ─── F10 — Scheduler ───

export interface SchedulerTask {
  id: string;
  routerId: string;
  name: string;
  /** Affichage humain à la RouterOS, ex. « 45s », « 1d ». */
  interval: string;
  onEvent: string;
  disabled: boolean;
  createdAt: string;
}

/**
 * Ligne scheduler unifiée (F10) : simulated renvoie des SchedulerTask complètes,
 * l'agent renvoie {name, interval, onEvent, disabled} — les deux partagent ces
 * quatre champs d'affichage.
 */
export interface SchedulerRow {
  id?: string;
  name: string;
  interval: string;
  onEvent: string;
  disabled: boolean;
  createdAt?: string;
}

export type ViewId =
  | "dashboard"
  | "sessions"
  | "users"
  | "vouchers"
  | "templates"
  | "profiles"
  | "resellers"
  | "routers"
  | "reports"
  | "logs"
  | "platform"
  | "platformLogs"
  | "platformTeam"
  | "platformSettings"
  | "billingRequests"
  | "accounts"
  | "notifications"
  | "settings"
  | "team";

/* ─── I (paramètres plateforme) : GET/PUT /api/admin/platform/settings ─── */

/** Réponse GET /api/admin/platform/settings. */
export interface PlatformSettingsResponse {
  platform: {
    name: string;
    registerOpen: boolean;
    /** true si une clé d'invitation est définie (jamais renvoyée en clair). */
    registerKeySet: boolean;
  };
  /** Source du contrôle des inscriptions : "database" (console) ou "env" (REGISTER_KEY verrouillée). */
  registerSource: "database" | "env";
}

/** Corps de PUT /api/admin/platform/settings (tous champs optionnels). */
export interface PlatformSettingsUpdatePayload {
  name?: string;
  registerOpen?: boolean;
  /** Nouvelle clé d'invitation — "" supprime la clé. */
  registerKey?: string;
}
