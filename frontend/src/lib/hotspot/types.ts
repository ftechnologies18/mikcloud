// Types partagés MikCloud — alignés sur le contrat API Go (voir worklog.md)

import type { TeamRole } from "./roles";

export type { TeamRole } from "./roles";

export type RouterMode = "simulated" | "real" | "agent";
export type RouterStatus = "online" | "offline";
/** Statut RÉSOLU des vouchers (5 états priorisés) : expired > disabled > online > used > active. */
export type VoucherStatus = "active" | "online" | "used" | "expired" | "disabled";
export type UserKind = "regular" | "voucher";
export type ResellerStatus = "active" | "disabled";
// N°19 — dépôt-vente : créance née à la remise + versement encaissé.
export type TransactionType = "credit" | "sale" | "debt" | "settlement";
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
  /** 2FA TOTP active (sécurité S4). */
  totpEnabled?: boolean;
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
  /** Purge/résurgence : nb d'utilisateurs présents sur le routeur mais
   * inconnus du cloud (créés via Winbox ou un autre système). Absent si 0 —
   * ces comptes ne sont pas importés automatiquement (réglage
   * autoImportRouterUsers) : adoption manuelle via l'outil d'import. */
  unknownOnRouter?: number;
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

/** Mode d'expiration cloud (F1, parité Mikhmon) : « none » aucune action, « notify » désactive au routeur, « remove » supprime. */
export type ProfileExpiryMode = "none" | "notify" | "remove";

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
  /** v2 : verrouiller chaque utilisateur au 1er appareil qui se connecte (liaison MAC, anti-partage). */
  lockFirstDevice: boolean;
  /** F13 : prix de vente affiché sur le voucher (0 = même prix que price). */
  sellingPrice: number;
  /** Parité Mikhmon : pool IP RouterOS servi au client hotspot ("" = none). */
  addressPool: string;
  /** Parité Mikhmon : queue simple RouterOS héritée par les utilisateurs ("" = none). */
  parentQueue: string;
  /** Parité Mikhmon : validité fine en minutes (0 = hériter validityDays, compat contrat V2). */
  validityMin: number;
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
  /** Miroir du statut STOCKÉ : true = désactivé manuellement (le badge peut afficher « expiré » — priorité —, le toggle s'y réfère). */
  disabled?: boolean;
  batchId: string;
  resellerId: string;
  resellerName: string;
  /** N°23 — ticket remis au client (Mode Vente / auto_connect) : reprise refusée. */
  soldAt?: string;
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
  /** Parité Mikhmon : Time Limit (limit-uptime) résolu à la génération (minutes ; 0 = héritage historique). */
  timeLimitMin: number;
  /** N (rapprochement doux) — utilisateur absent du dernier read_state du routeur. */
  missingOnRouter?: boolean;
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
  /** Preset de caractères : "" (MikCloud sûr) | abc | ABC | aBc | 5ab | 5AB | 5aB | num. */
  charset?: string;
  /** Commentaire libre inscrit sur le routeur avec le n° de lot (64 car. max). */
  comment?: string;
  /** Quota data par voucher en Mo : undefined = hériter du profil, 0 = illimité, >0 = plafond. */
  dataQuotaMb?: number;
  /** Parité Mikhmon : Time Limit par lot (limit-uptime, minutes ; 0 = hériter du profil). */
  timeLimitMin?: number;
  /** Parité Mikhmon : serveur hotspot RouterOS visé ("" = tous). */
  server?: string;
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
  /** N°8 — stats LIVE « stock vs vendus » (présentes sur la liste, recalculées serveur) : */
  /** vouchers attribués, non remis et toujours actifs (vendables). */
  stockCount?: number;
  /** vouchers remis au client (SoldAt tracé) — total. */
  soldCount?: number;
  /** remises aujourd'hui (badge du jour). */
  soldToday?: number;
  /** tout voucher portant le revendeur — l'écart vs stock+vendus révèle le non déclaré. */
  assignedCount?: number;
  /** recette du jour (prix de vente des remises du jour). */
  revenueToday?: number;
  /** recette totale des remises tracées. */
  revenueTotal?: number;
  /* N°19 — dépôt-vente : mode de paiement + créance. */
  /** prepaid (historique) | deposit (il vend puis verse). */
  paymentMode?: "prepaid" | "deposit";
  /** plafond de créance (dette + stock ≤ plafond, sinon prise de stock refusée). */
  debtCeiling?: number;
  /** créance courante : Σ(remises à crédit) − Σ(versements). */
  debt?: number;
  /** nombre de versements encaissés (confiance progressive). */
  settlementsCount?: number;
  /** date du dernier versement (ISO). */
  lastSettlementAt?: string;
}

/* ─── N°8 : rapport de fin de journée (GET /api/sell/day-report) ─── */

/** P3-d — canal d'une vente (audit R4). `sell_mode` = tactile,
 * `auto_connect` = 1ʳᵉ connexion client, `sell_mode_paper` = papier historique. */
export type SellVia = "sell_mode" | "auto_connect" | "sell_mode_paper";

export interface SellDayReportItem {
  id: string;
  code: string;
  profileName: string;
  price: number;
  soldAt: string;
  routerName: string;
  /** P3-d — canal de la vente (absent sur l'historique pré-R4 → tactile). */
  soldVia?: SellVia;
}

export interface SellDayReport {
  /** YYYY-MM-DD (journée métier UTC). */
  date: string;
  currency: string;
  sold: SellDayReportItem[];
  soldCount: number;
  revenue: number;
  stockCount: number;
  stockValue: number;
  /** N°19 V2 — dépôt-vente : cash du jour à verser + créance totale. */
  toDeposit?: number;
  debtTotal?: number;
  paymentMode?: "prepaid" | "deposit";
  /** P3-d — ventilation des ventes du jour par canal (nombre). */
  byVia?: Partial<Record<SellVia, number>>;
  /** P3-d — retours de stock du jour avec flux cash (recrédit prépayé). */
  returnedCount?: number;
  returnedCredited?: number;
  /** P3-d — versements dépôt-vente déjà encaissés aujourd'hui. */
  settledToday?: number;
}

/* ─── N°19 V2 : créances revendeurs (dashboard) ─── */

export interface ReceivableItem {
  resellerId: string;
  name: string;
  debt: number;
  ceiling: number;
  /** ancienneté de la créance (jours depuis le dernier versement). */
  agingDays: number;
  /** ok | warn (≥ 7 j) | danger (≥ 30 j). */
  level: "ok" | "warn" | "danger";
  /** dette > plafond → Mode Vente du revendeur bloqué. */
  overCeiling: boolean;
}

export interface Receivables {
  totalDebt: number;
  count: number;
  items: ReceivableItem[];
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
  /** Parité Mikhmon : Time Limit (limit-uptime) résolu à la génération (minutes). */
  timeLimitMin: number;
  channel: SaleChannel;
  resellerId: string;
  resellerName: string;
  createdAt: string;
}

/** N°18 — détenteur d'une partie du stock vendable d'un lot (recalculé backend) :
 * resellerId "" = stock direct (gérant). Le lot reste immuable (provenance). */
export interface BatchHolding {
  resellerId: string;
  name: string;
  count: number;
  value: number;
}

/** Refonte onglet Lots — cycle de vie du lot, dérivé backend des stats live :
 * stock (du consommable reste) → consumed (épuisé : utilisé/désactivé)
 * → expired (jamais utilisé, validité envolée) → purged (plus rien en base). */
export type BatchLifecycle = "stock" | "consumed" | "expired" | "purged";

/** Refonte v2 « tour de contrôle » — pipeline d'une ligne de l'onglet Lots
 * (totaux sur l'ensemble FILTRÉ, avant pagination). */
export interface BatchSummary {
  batches: number;
  stockTickets: number;
  transferable: number;
  stockValue: number;
  expiring7d: number;
  /** v2 — vendables détenus par des revendeurs (étape « Chez revendeurs »). */
  resellerStock: number;
  /** v2 — valeur gros de ce stock chez les revendeurs. */
  resellerStockValue: number;
  /** v2 — sorties de stock sur 7 j glissants (ventes + consommations). */
  sold7d: number;
  /** v2 — valeur faciale du vendable (Σ prix public). */
  stockFace: number;
  /** v2 — marge en attente cumulée (face − gros, jamais négative). */
  marginPending: number;
}

export interface BatchWithStats extends Batch {
  remaining: number;
  active: number;
  used: number;
  expired: number;
  disabled: number;
  /** Refonte — cycle de vie dérivé (stock | consumed | expired | purged). */
  status: BatchLifecycle;
  /** N°18 — stock vendable (actif, jamais remis), toute destination confondue. */
  transferable: number;
  /** Valeur faciale du stock vendable (somme des prix) — aperçu du débit. */
  transferableValue: number;
  /** Tickets transférables expirant dans les 7 jours (garde-fou du dialog). */
  expiring7d: number;
  /** v2 — sorties de stock sur 7 j glissants (ventes + consommations) : vélocité. */
  sold7d: number;
  /** v2 — dernier mouvement de sortie (ISO, absent si aucun mouvement). */
  lastEgressAt?: string;
  /** v2 — jours de dormance (rempli uniquement si transferable > 0) :
   * base = dernière sortie, sinon création du lot si rien n'est jamais sorti. */
  dormantDays: number;
  /** v2 — valeur faciale du stock vendable (Σ prix public). */
  stockFace: number;
  /** v2 — marge en attente (face − gros, jamais négative) : ce que le stock
   * vivant RAPPORTERA à l'écoulement. */
  marginPending: number;
  /** Répartition live du stock vendable par détenteur. */
  holdings?: BatchHolding[];
}

export interface PagedBatches {
  data: BatchWithStats[];
  total: number;
  page: number;
  pageSize: number;
  /** Refonte — bande KPI de l'onglet Lots (additif, présent sur liste filtrée). */
  summary?: BatchSummary;
}

export interface SiteOverview {
  routerId: string;
  routerName: string;
  status: RouterStatus;
  activeSessions: number;
  hotspotUsers: number;
  /** Users EN LIGNE (session live) — la carte « Utilisateurs actifs ». */
  onlineUsers: number;
  activeVouchers: number;
  salesToday: number;
  /** Vouchers activés aujourd'hui (1ʳᵉ connexion) — « Tickets vendus ». */
  soldToday: number;
  revenue30d: number;
}

export type AccountingPeriod = "day" | "week" | "month";

/** Répartition du CA par canal de distribution (direct vs réseau revendeurs). */
export interface ChannelSplit {
  directRevenue: number;
  resellerRevenue: number;
  directSales: number;
  resellerSales: number;
}

/** Fenêtre précédente de même longueur — comparaison Δ%. */
export interface TotalsDelta {
  revenue: number;
  sales: number;
  avgTicket: number;
  margin?: number;
}

export interface AccountingData {
  period: AccountingPeriod;
  routerId: string;
  series: { label: string; revenue: number; sales: number }[];
  byRouter: {
    routerId: string;
    routerName: string;
    revenue: number;
    sales: number;
    share: number;
    /** F13 — coût/total vente du site (backend v2) : marge par site. */
    cost?: number;
    selling?: number;
  }[];
  totals: {
    revenue: number;
    sales: number;
    avgTicket: number;
    /** F13 — coût, total vente et marge (selling − cost). */
    cost?: number;
    selling?: number;
    margin?: number;
  };
  /** v2 — fenêtre précédente équivalente (Δ% des KPI). */
  prev?: TotalsDelta;
  /** v2 — CA direct vs revendeurs (même fenêtre / filtre routeur). */
  channel?: ChannelSplit;
}

export interface DashboardData {
  kpis: {
    activeSessions: number;
    totalUsers: number;
    activeVouchers: number;
    /** Vouchers activés aujourd'hui (1ʳᵉ connexion) — « Tickets vendus ». */
    soldToday: number;
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
  /** N°19 V2 — créances revendeurs (dépôt-vente) : présent si des créances existent. */
  receivables?: Receivables;
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
  /** Connexions RÉELLES par jour (journal logins) — remplace la courbe de trafic synthétique, supprimée. */
  loginsByDay: { day: string; count: number }[];
  voucherStatus: { active: number; used: number; expired: number; disabled: number };
  totals: { revenue: number; sales: number; avgTicket: number };
  /** v2 — fenêtre précédente de même longueur (Δ% des KPI). */
  prev?: TotalsDelta;
  /** v2 — CA direct vs revendeurs sur la fenêtre. */
  channel?: ChannelSplit;
  /** v2 — top 5 revendeurs par CA de la fenêtre. */
  topResellers?: { name: string; sales: number; revenue: number }[];
  /** v2 — sessions réelles de la fenêtre : comptage + trafic cumulé. */
  sessions?: { count: number; bytesIn: number; bytesOut: number };
  /** F13 : bloc marge (30 jours glissants) — optionnel tant que le backend P2 n'est pas déployé. */
  margin?: ReportsMargin;
}

/** Parité Mikhmon : ressource routeur (read_resources) — pool d'adresses, file parent ou serveur hotspot. */
export interface RouterResource {
  kind: "pool" | "queue" | "server";
  name: string;
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
  /** v2 — fenêtre des 30 jours précédents (Δ% CA / marge). */
  prev?: { revenue: number; cost: number; margin: number };
  /** v2 — marge quotidienne (le plus ancien d'abord, 30 points). */
  byDay?: { day: string; margin: number }[];
  /** v2 — profitabilité par site (tri : meilleure marge d'abord). */
  byRouter?: { routerName: string; revenue: number; cost: number; margin: number }[];
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
    /** Purge/résurgence : true (défaut) = les utilisateurs présents sur les
     * routeurs mais inconnus du cloud sont importés automatiquement à chaque
     * synchronisation agent ; false = jamais importés automatiquement (visibles
     * dans la santé du routeur, adoption manuelle via l'outil d'import). */
    autoImportRouterUsers?: boolean;
  };
  plan: {
    name: string;
    maxRouters: string;
    maxUsers: string;
  };
  /** État d'abonnement SaaS (optionnel : absents des réponses d'avant la facturation). */
  subscription?: Subscription;
  /** Forme PLATE du réglage (repli si le backend ne l'imbrique pas dans tenant) —
   * même sémantique que tenant.autoImportRouterUsers. */
  autoImportRouterUsers?: boolean;
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
/** Répercussion des frais de paiement (stratégie validée) : montants d'une
 * période par moyen — base (prix catalogue net cible), liste (carte, frais
 * GeniusPay inclus via gross-up), Wave (remise mobile money −3 % incluse). */
export interface PlanPricing {
  planId: string;
  baseFcfa: number;
  listFcfa: number;
  waveFcfa: number;
}

export interface SubscriptionView {
  subscription: Subscription;
  /** Statut effectif calculé serveur : une période échue passe en « expired », puis « suspended » après 30j de grâce. */
  status: "active" | "expired" | "suspended" | "none";
  routerCount: number;
  currentAmountFcfa: number;
  plans: SaasPlan[];
  waveConfigured: boolean;
  /** Montants par moyen pour chaque formule (répercussion des frais, serveur). */
  pricing?: PlanPricing[];
}

/** Réponse POST /api/subscription — DEMANDE de souscription (verrou facturation) :
 * l'abonnement n'est PAS activé côté client (renvoyé inchangé) ; la demande
 * est tracée dans le journal et l'activation est effectuée par la plateforme
 * après encaissement (PUT /api/admin/accounts/{id}/subscription). */
export interface SubscribeResponse {
  subscription: Subscription;
  /** Montant à régler pour le moyen PAR DÉFAUT (Wave — remise mobile money incluse). */
  amountFcfa: number;
  /** Net cible plateforme (prix catalogue, hors frais) — base du calcul. */
  baseAmountFcfa?: number;
  /** Prix de liste carte (frais inclus) — encaissé si le client bascule en carte. */
  listAmountFcfa?: number;
  routerCount: number;
  periodLabel: string;
  /** Lien de paiement Wave de la PLATEFORME, pré-composé avec le montant ("" si non configuré). */
  waveLink: string;
  /** Toujours true : la demande attend l'encaissement puis l'activation plateforme. */
  pending: boolean;
}

/** Réponse POST /api/subscription/pay — paiement Wave initié via GeniusPay :
 * le client est redirigé vers paymentUrl, la confirmation arrive par webhook. */
export interface PayInitiateResponse {
  /** Référence de la demande de facturation (MC-XXXXXXXX). */
  ref: string;
  /** Référence de la transaction marchande GeniusPay (MTX-…). */
  gatewayRef: string;
  /** Lien de paiement Wave à ouvrir (checkout GeniusPay en repli). */
  paymentUrl: string;
  amountFcfa: number;
  status: "pending";
}

/** Réponse GET /api/subscription/pay/status — filet de sécurité : statut réel
 * de la transaction consulté chez GeniusPay, activation finalisée si besoin. */
export interface PayStatusResponse {
  status: "none" | "pending" | "done" | "cancelled";
  ref?: string;
  gatewayRef?: string;
}

/** Abonnement RÉCURRENT par carte (Stripe via GeniusPay) — vue GET
 * /api/subscription/stripe. Statut "none" = aucun prélèvement automatique. */
export interface StripeSubView {
  status: "none" | "pending" | "trialing" | "active" | "past_due" | "paused" | "cancelled" | "expired";
  uuid?: string;
  planId?: string;
  planName?: string;
  /** Cycle de prélèvement : monthly (Essentiel) | yearly (Illimité). */
  cycle?: "monthly" | "yearly";
  amountFcfa?: number;
  /** Prochaine échéance (AAAA-MM-JJ) renseignée par GeniusPay. */
  nextBilling?: string;
  lastRenewalAt?: string;
  /** Abonnement MikCloud du compte (période active). */
  subscription?: Pick<Subscription, "planId" | "status" | "periodStart" | "periodEnd">;
}

/** Réponse POST /api/subscription/stripe — abonnement récurrent créé :
 * paymentUrl = paiement EN LIGNE de la première période (carte si proposé,
 * sinon Wave) ; redirectUrl = page Stripe Checkout (si GeniusPay la renvoie) ;
 * les échéances suivantes sont débitées automatiquement par GeniusPay. */
export interface StripeCreateResponse {
  uuid: string;
  status: string;
  nextBilling?: string;
  redirectUrl?: string;
  paymentUrl?: string;
  /** Référence de la demande de facturation de la première période (MC-…). */
  ref?: string;
  planId: string;
  planName: string;
  amountFcfa: number;
  cycle: "monthly" | "yearly";
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
  | "subscription"
  | "users"
  | "registrations"
  | "vouchers"
  | "templates"
  | "profiles"
  | "resellers"
  | "wifi"
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

/* ─── Purge des données fusionnée (portée globale ou ciblée par compte) ─── */

/** Compteurs par élément d'un compte (GET /api/admin/purge/accounts).
 * Mêmes règles que les stats globales : les entités attachées aux routeurs
 * simulés partent en cascade — exclues de leur catégorie. */
export interface AccountPurgeStats {
  simulatedRouters: number;
  /** Comptes client hotspot (kind != voucher). */
  hotspotUsers: number;
  /** Tickets (kind == voucher). */
  vouchers: number;
  profiles: number;
  batches: number;
  resellers: number;
  transactions: number;
  sales: number;
  sessions: number;
  logs: number;
  templates: number;
}

/** Ligne de la liste des compteurs par compte (sélecteur de portée). */
export interface PurgeAccountRow {
  id: string;
  name: string;
  /** Login du propriétaire (premier owner/admin du compte). */
  owner: string;
  /** active | disabled. */
  status: string;
  stats: AccountPurgeStats;
}

/** Réponse de POST /api/admin/purge (portée globale OU ciblée) — quantités
 * réellement supprimées. purged.tombstones : marqueurs anti-ré-import posés
 * (blocage 30 j de la synchro agent) ; purged.routerRemovals : comptes
 * commandés en suppression sur les routeurs réels (0 si alsoRouter = false). */
export interface PurgeResponse {
  ok: boolean;
  /** Bilan lisible (journal + toast). */
  summary: string;
  purged: Partial<Record<"routers" | "hotspotUsers" | "vouchers" | "profiles" | "batches" | "resellers" | "transactions" | "sales" | "sessions" | "logs" | "templates" | "tombstones" | "routerRemovals", number>>;
}

/* ─── M (facturation client) : GET /api/billing/history ─── */

/** Ligne d'historique de facturation (demande résolue du compte). */
export interface InvoiceRow {
  id: string;
  /** Numéro de facture séquentiel annuel (MC-2026-0001). */
  invoiceNo: string;
  planName: string;
  amountFcfa: number;
  periodLabel: string;
  routerCount: number;
  /** Référence de paiement (MC-XXXXXXXX). */
  ref: string;
  paidVia: "wave" | "manual" | string;
  issuedAt: string;
}

/* ─── N°27 — Inscriptions publiques par QR code (liens + demandes) ─── */

/** État dérivé d'un lien d'inscription : revoked > expired > exhausted > active. */
export type JoinLinkState = "active" | "revoked" | "expired" | "exhausted";

/** Lien d'inscription publique — encodé en QR côté console (URL /join/{token}). */
export interface JoinLink {
  id: string;
  accountId: string;
  name: string;
  token: string;
  /** Pré-attribution optionnelle : profil et routeur imposés par le lien. */
  profileId?: string;
  profileName?: string;
  routerId?: string;
  routerName?: string;
  /** Mode kiosque : profil + routeur pré-attribués requis, compte créé dès la soumission. */
  autoValidate: boolean;
  /** Nombre max de soumissions — 0 = illimité. */
  maxUses: number;
  uses: number;
  expiresAt?: string;
  revoked: boolean;
  createdBy?: string;
  createdByName?: string;
  createdAt: string;
}

/** Lien enrichi de l'état dérivé (réponse GET/POST/PUT /api/join-links). */
export interface JoinLinkView extends JoinLink {
  state: JoinLinkState;
}

/** Demande d'inscription publique (page /join/{token}). Le mot de passe choisi
 * n'est renvoyé NON VIDE que pour status="pending" (vidé après décision). */
export interface RegistrationRequest {
  id: string;
  accountId: string;
  linkId: string;
  linkName: string;
  fullName: string;
  phone: string;
  desiredUsername: string;
  password: string;
  /** Message libre éventuel laissé par le demandeur. */
  message?: string;
  status: "pending" | "approved" | "rejected";
  rejectionReason?: string;
  reviewedByName?: string;
  reviewedAt?: string;
  /** Utilisateur hotspot créé à l'approbation. */
  userId?: string;
  createdAt: string;
}

/** Réponse GET /api/registrations — compteurs globaux du compte + items
 * (plafond 300, tri createdAt desc ; status vide = toutes). */
export interface RegistrationsResponse {
  counts: { pending: number; approved: number; rejected: number };
  items: RegistrationRequest[];
}

/** Corps de POST /api/join-links (profil/routeur optionnels ; maxUses 0–100000). */
export interface JoinLinkCreatePayload {
  name: string;
  profileId?: string;
  routerId?: string;
  autoValidate: boolean;
  maxUses: number;
  /** RFC3339 futur (fin de journée locale côté console). */
  expiresAt?: string;
}

/** Corps de PUT /api/join-links/{id} — révoquer / réactiver. */
export interface JoinLinkUpdatePayload {
  revoked: boolean;
}

/** Corps de POST /api/registrations/{id}/approve. password vide = généré côté serveur. */
export interface RegistrationApprovePayload {
  profileId: string;
  routerId: string;
  username: string;
  password?: string;
}

/** Réponse de l'approbation — queued=true si la commande user_add est en file
 * agent (routeur réel) ; le mot de passe final vit dans user.password. */
export interface RegistrationApproveResponse {
  request?: RegistrationRequest;
  user: HotspotUser;
  queued?: boolean;
  commandId?: string;
}

/* ─── N°28 — WiFi Jetable : sites publics + registre marketing ─── */

/** Site d'un établissement (maquis, resto, salon…) offrant le WiFi jetable. */
export interface WifiSite {
  id: string;
  accountId: string;
  name: string;
  slug: string;
  routerId: string;
  routerName: string;
  profileId: string;
  profileName: string;
  /** Minutes offertes (0 = hériter du profil). */
  freeTimeMin: number;
  /** Mo offerts (0 = hériter du profil). */
  freeDataMb: number;
  marketingOptIn: boolean;
  dailyPerPhone: number;
  dailyCap: number;
  active: boolean;
  createdAt: string;
}

/** Entrée du registre : un code délivré (marketing + anti-abus). */
export interface WifiGuest {
  id: string;
  accountId: string;
  siteId: string;
  siteName: string;
  phone: string;
  optIn: boolean;
  voucherId: string;
  code: string;
  day: string;
  createdAt: string;
}

/** GET /api/wifi/sites — liste + statistiques du jour. */
export interface WifiSitesResponse {
  sites: WifiSite[];
  day: string;
  stats: Record<string, { guestsToday: number; optInTotal: number }>;
}

/** Corps de création / mise à jour d'un site (quotas ajustables par le gérant). */
export interface WifiSitePayload {
  name: string;
  routerId: string;
  profileId: string;
  freeTimeMin: number;
  freeDataMb: number;
  marketingOptIn: boolean;
  dailyPerPhone: number;
  dailyCap: number;
  active: boolean;
}

/** GET /api/wifi/site/{slug} — branding public (aucune donnée sensible). */
export interface WifiSiteInfo {
  slug: string;
  name: string;
  tenantName?: string;
  logoUrl?: string;
  freeTimeMin: number;
  freeDataMb: number;
  profileName?: string;
  marketingOptIn: boolean;
  active: boolean;
  suspended?: boolean;
  offers?: WifiOffer[];
}

/** Offre payante affichée à la bascule (profil à prix > 0). */
export interface WifiOffer {
  id: string;
  name: string;
  price: number;
  validityMinutes: number;
  dataQuotaMb: number;
  timeLimitMin: number;
}

/** POST /api/wifi/site/{slug}/claim — code délivré au visiteur. */
export interface WifiClaimResponse {
  duplicate: boolean;
  code: string;
  loginUrl: string;
  timeLimitMin: number;
  dataQuotaMb: number;
  profileName?: string;
  siteName?: string;
}

/** GET /api/wifi/site/{slug}/status — état du ticket du jour. */
export interface WifiStatusResponse {
  state: "none" | "active" | "exhausted";
  active: boolean;
  code?: string;
  loginUrl?: string;
  timeLimitMin?: number;
  dataQuotaMb?: number;
  offers?: WifiOffer[];
}
