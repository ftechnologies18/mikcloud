"use client";

// Client API MikCloud.
// - Production (Vercel · mikcloud.ftci.fr) : mode direct —
//   NEXT_PUBLIC_API_BASE=https://xxx.onrender.com (build Vercel)
//   + ALLOWED_ORIGIN côté Render (CORS).
// - Alternative proxy : URLs relatives /api/* transférées par un rewrite
//   vercel.json (zéro CORS — cf. README « Déploiement » pour la bascule).
// - Sandbox : même origine + query param XTransformPort=4000 (passerelle Caddy).

import { useHotspotStore } from "./store";
import type {
  AccountDetail,
  AccountStatus,
  AccountSummary,
  AccountUsage,
  AuthResponse,
  AuthUser,
  BillingRequest,
  BillingRequestsResponse,
  ChatAdminMessage,
  ChatConversationDetail,
  ChatConversationsResponse,
  FleetActionResponse,
  FleetOverview,
  InvoiceRow,
  PlatformActivityRow,
  PlatformOverview,
  PlatformSettingsResponse,
  PlatformSettingsUpdatePayload,
  PurgeAccountRow,
  PurgeResponse,
  PlatformTeamMember,
  RegisterPayload,
  SubscriptionInfo,
  SubscriptionUpdatePayload,
} from "./types";

const API_BASE = (process.env.NEXT_PUBLIC_API_BASE || "").replace(/\/$/, "");
const GATEWAY_PORT = "4000";

export class ApiError extends Error {
  status: number;
  /** Code machine optionnel renvoyé par le backend (ex. subscription_expired). */
  code?: string;
  /** N°27 — suggestion optionnelle du backend (ex. username_taken propose un
   * nom libre) : champ top-level du corps d'erreur, recopié tel quel. */
  suggestion?: string;
  constructor(message: string, status: number, code?: string, suggestion?: string) {
    super(message);
    this.status = status;
    this.code = code;
    this.suggestion = suggestion;
  }
}

interface ApiOptions {
  method?: "GET" | "POST" | "PUT" | "DELETE";
  body?: unknown;
  params?: Record<string, string | number | undefined>;
  /** UX R6 / N°78 — délai max (ms) avant abandon : un réseau mobile peut
   * stall une requête indéfiniment (socket demi-ouvert) — ni succès ni
   * échec. Chaque variante a son DÉFAUT (api/apiAnon 20 s, apiUpload 60 s,
   * apiDownload 120 s) ; passer timeoutMs explicite prime sur le défaut,
   * timeoutMs: 0 désactive (attente infinie). Passé le délai, fetch rejette
   * (DOMException) : l'appelant traite ça comme une erreur réseau
   * (file hors-ligne / fallback cache). */
  timeoutMs?: number;
}

/**
 * N°78 — signal d'annulation avec défaut par variante. AbortSignal.timeout
 * quand il existe (navigateurs modernes), repli manuel AbortController +
 * minuteur sinon. `0`/undefined→fallback, `0` explicite = désactivé.
 */
function timeoutSignal(ms: number | undefined, fallback: number): AbortSignal | undefined {
  const delay = ms === undefined ? fallback : ms;
  if (!delay || !Number.isFinite(delay) || delay <= 0) return undefined;
  if (typeof AbortSignal.timeout === "function") return AbortSignal.timeout(delay);
  const ctrl = new AbortController();
  setTimeout(() => ctrl.abort(), delay);
  return ctrl.signal;
}

function buildUrl(path: string, params?: ApiOptions["params"]): string {
  const url = new URL(`${API_BASE}${path}`, typeof window !== "undefined" ? window.location.origin : "http://localhost");
  const merged: Record<string, string> = {};
  if (params) {
    for (const [k, v] of Object.entries(params)) {
      if (v !== undefined && v !== "" && v !== null) merged[k] = String(v);
    }
  }
  // Mode passerelle sandbox uniquement (localhost) — en prod Vercel, les URLs
  // relatives passent par le rewrite vercel.json : ce paramètre n'a pas lieu
  // d'être et ne doit pas fuiter vers Render.
  const isSandbox =
    typeof window !== "undefined" &&
    (window.location.hostname === "localhost" || window.location.hostname === "127.0.0.1");
  if (!API_BASE && isSandbox) merged["XTransformPort"] = GATEWAY_PORT;
  for (const [k, v] of Object.entries(merged)) url.searchParams.set(k, v);
  return url.toString();
}

export async function api<T>(path: string, opts: ApiOptions = {}): Promise<T> {
  const token = useHotspotStore.getState().token;
  const headers: Record<string, string> = {};
  if (opts.body !== undefined) headers["Content-Type"] = "application/json";
  if (token) headers["Authorization"] = `Bearer ${token}`;

  const method = opts.method ?? "GET";
  const res = await fetch(buildUrl(path, opts.params), {
    method,
    headers,
    body: opts.body !== undefined ? JSON.stringify(opts.body) : undefined,
    // N°130 — P0 audit réactivité : les GET SANS CORPS passent en « no-cache »
    // (stocké + revalidation conditionnelle) au lieu de « no-store » : le
    // backend pose ETag + Cache-Control: no-cache sur les endpoints pollés
    // (writeJSONCacheable — dashboard, sessions, devices, profils, modèles,
    // revendeurs, liste utilisateurs, stats vouchers…) ; le navigateur
    // renvoie If-None-Match et reçoit un 304 SANS CORPS quand rien n'a
    // changé — l'ancien « no-store » re-téléchargeait la payload entière à
    // CHAQUE poll (37 sources de polling côté front). Les mutations (et tout
    // GET portant un corps) restent « no-store » : jamais de réutilisation.
    cache: method === "GET" && opts.body === undefined ? "no-cache" : "no-store",
    signal: timeoutSignal(opts.timeoutMs, 20_000),
  });

  if (res.status === 401) {
    useHotspotStore.getState().logout();
    throw new ApiError("Session expirée, veuillez vous reconnecter.", 401);
  }

  let data: unknown = null;
  try {
    data = await res.json();
  } catch {
    /* réponse non JSON */
  }

  if (!res.ok) {
    const body = data && typeof data === "object" ? (data as { error?: unknown; code?: unknown; suggestion?: unknown }) : null;
    const message =
      (body && typeof body.error === "string" ? body.error : null) ?? `Erreur ${res.status}`;
    const code = body && typeof body.code === "string" ? body.code : undefined;
    const suggestion = body && typeof body.suggestion === "string" ? body.suggestion : undefined;
    throw new ApiError(message, res.status, code, suggestion);
  }

  return data as T;
}

/**
 * apiUpload — variante multipart (N°53) : téléversement de fichiers (images
 * du gérant vers le stockage R2 via POST /api/media). Pas de Content-Type
 * manuel (le navigateur pose le boundary), corps = FormData, auth Bearer
 * identique. L'upload Media n'est PAS concerné par le 401 automatique :
 * l'appelant décide (fallback data URL possible si le stockage est indispo).
 */
export async function apiUpload<T>(path: string, form: FormData, opts: ApiOptions = {}): Promise<T> {
  const token = useHotspotStore.getState().token;
  const headers: Record<string, string> = {};
  if (token) headers["Authorization"] = `Bearer ${token}`;

  const res = await fetch(buildUrl(path), {
    method: "POST",
    headers,
    body: form,
    cache: "no-store",
    signal: timeoutSignal(opts.timeoutMs, 60_000),
  });

  if (res.status === 401) {
    useHotspotStore.getState().logout();
    throw new ApiError("Session expirée, veuillez vous reconnecter.", 401);
  }

  let data: unknown = null;
  try {
    data = await res.json();
  } catch {
    /* réponse non JSON */
  }

  if (!res.ok) {
    const body = data && typeof data === "object" ? (data as { error?: unknown; code?: unknown }) : null;
    const message =
      (body && typeof body.error === "string" ? body.error : null) ?? `Erreur ${res.status}`;
    const code = body && typeof body.code === "string" ? body.code : undefined;
    throw new ApiError(message, res.status, code);
  }

  return data as T;
}

/**
 * apiAnon — variante SANS authentification pour la page publique WiFi jetable
 * (N°27) : pas de header Bearer, pas de logout automatique sur 401 (un
 * visiteur anonyme n'a pas de session à expirer — l'appelant traite l'erreur).
 */
export async function apiAnon<T>(path: string, opts: ApiOptions = {}): Promise<T> {
  const headers: Record<string, string> = {};
  if (opts.body !== undefined) headers["Content-Type"] = "application/json";
  const method = opts.method ?? "GET";
  const res = await fetch(buildUrl(path, opts.params), {
    method,
    headers,
    body: opts.body !== undefined ? JSON.stringify(opts.body) : undefined,
    // N°130 — même politique que api() : GET sans corps → revalidation
    // conditionnelle (ETag/304), le reste sans stockage.
    cache: method === "GET" && opts.body === undefined ? "no-cache" : "no-store",
    signal: timeoutSignal(opts.timeoutMs, 20_000),
  });
  let data: unknown = null;
  try {
    data = await res.json();
  } catch {
    /* réponse non JSON */
  }
  if (!res.ok) {
    const body = data && typeof data === "object" ? (data as { error?: unknown; code?: unknown }) : null;
    const message =
      (body && typeof body.error === "string" ? body.error : null) ?? `Erreur ${res.status}`;
    const code = body && typeof body.code === "string" ? body.code : undefined;
    throw new ApiError(message, res.status, code);
  }
  return data as T;
}

/**
 * wakeBackend — N°84 : réveil proactif du backend (cold boot Render plan
 * gratuit : hibernation après ~15 min sans trafic, démarrage 30–90 s).
 * Fire-and-forget vers GET / (handleHealth, réponse minuscule) : le but est
 * de DÉCLENCHER le boot pendant que l'utilisateur tape ses identifiants,
 * pas de lire la réponse. Aucune erreur remontée — un échec (offline, CORS,
 * timeout 30 s) est silencieusement ignoré : ce ping est une optimisation,
 * jamais un blocage. Idempotent par garde module (un seul ping par chargement
 * de bundle, les re-rendus ne relancent rien).
 */
const wakeGuard = { done: false };
export function wakeBackend(): void {
  if (wakeGuard.done) return;
  if (typeof window === "undefined") return;
  wakeGuard.done = true;
  void fetch(buildUrl("/", { t: Date.now() }), {
    method: "GET",
    cache: "no-store",
    signal: timeoutSignal(30_000, 30_000),
  }).catch(() => {
    /* silencieux : réveil au mieux, ignoré sinon */
  });
}

/** apiDownload — télécharge un fichier (CSV, PDF…) renvoyé par l'API, avec token.
 * N°78 — délai max 120 s par défaut (gros exports). */
export async function apiDownload(path: string, filename: string, params?: ApiOptions["params"]): Promise<void> {
  const token = useHotspotStore.getState().token;
  const headers: Record<string, string> = {};
  if (token) headers["Authorization"] = `Bearer ${token}`;
  const res = await fetch(buildUrl(path, params), {
    headers,
    cache: "no-store",
    signal: timeoutSignal(undefined, 120_000),
  });
  if (!res.ok) {
    let message = `Erreur ${res.status}`;
    try {
      const body = (await res.json()) as { error?: string };
      if (body.error) message = body.error;
    } catch {
      /* non JSON */
    }
    throw new ApiError(message, res.status);
  }
  const blob = await res.blob();
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
}

/** register — inscription SaaS (rôle owner). Renvoie token + utilisateur comme le login.
 * N°101 — l'usage (hotspot | homenet) est porté par le corps : le sélecteur
 * du formulaire d'inscription choisit la console d'atterrissage. */
export async function register(payload: RegisterPayload): Promise<AuthResponse> {
  return api<AuthResponse>("/api/auth/register", {
    method: "POST",
    body: {
      name: payload.name,
      username: payload.username,
      password: payload.password,
      key: payload.key || undefined,
      // F (signup enrichi) — contact propriétaire + segmentation géographique.
      email: payload.email,
      phone: payload.phone,
      country: payload.country,
      city: payload.city,
      // N°101 — usage du compte (absent = hotspot, comportement historique).
      usage: payload.usage || undefined,
    },
  });
}

/** fetchAccounts — liste des comptes clients SaaS (admin plateforme uniquement, 403 sinon). */
export async function fetchAccounts(): Promise<AccountSummary[]> {
  return api<AccountSummary[]>("/api/admin/accounts");
}

/** setAccountStatus — active ou désactive un compte SaaS. Le compte principal ne peut pas être désactivé (400). */
export async function setAccountStatus(id: string, status: AccountStatus): Promise<{ ok: boolean }> {
  return api<{ ok: boolean }>(`/api/admin/accounts/${id}/status`, { method: "POST", body: { status } });
}

/** setAccountUsage — N°98 : change l'usage d'un compte (hotspot ⇄ homenet).
 * Plateforme uniquement — effet immédiat sur les gardes API (le serveur relit
 * l'usage à chaque requête, pas au login). */
export async function setAccountUsage(id: string, usage: AccountUsage): Promise<{ ok: boolean }> {
  return api<{ ok: boolean }>(`/api/admin/accounts/${id}/usage`, { method: "PUT", body: { usage } });
}

// ---------------------------------------------------------------------------
// N°101 — appareils du foyer (console HomeNet)
// ---------------------------------------------------------------------------

/** renameDevice — affecte le nom familier d'un appareil (« TV du salon »).
 * Un nom vide est légitime (retour à l'anonymat host-name/MAC). */
export async function renameDevice(id: string, name: string): Promise<{ ok: boolean; name: string }> {
  return api<{ ok: boolean; name: string }>(`/api/devices/${id}`, { method: "PUT", body: { name } });
}

/** pauseDevice — pause dîner : coupe l'internet de l'appareil.
 * `paused` pose/ lève la pause ; `minutes` (0 ou absent = illimité) borne la
 * durée — l'échéance est calculée serveur, la coupure appliquée par la box
 * à son prochain check-in (≤ 1 min console ouverte). */
export async function pauseDevice(
  id: string,
  paused: boolean,
  minutes?: number,
): Promise<{ ok: boolean; paused: boolean; pausedUntil: string }> {
  return api<{ ok: boolean; paused: boolean; pausedUntil: string }>(`/api/devices/${id}/pause`, {
    method: "POST",
    body: { paused, minutes: minutes ?? 0 },
  });
}

// ---------------------------------------------------------------------------
// Console plateforme (super-admin MikCloud — multi-comptes)
// ---------------------------------------------------------------------------

/** fetchPlatformOverview — KPIs globaux du SaaS (tous comptes confondus). */
export async function fetchPlatformOverview(): Promise<PlatformOverview> {
  return api<PlatformOverview>("/api/admin/overview");
}

/* ─── N°117 — parc routeurs global (vue « Parc routeurs », super-admin) ─── */

/** fetchFleetRouters — le parc complet, tous comptes confondus, avec l'état
 * de mise à jour RouterOS de chaque routeur (version installée, version
 * disponible détectée, vérification/installation en vol). */
export async function fetchFleetRouters(): Promise<FleetOverview> {
  return api<FleetOverview>("/api/admin/fleet/routers");
}

/** fleetRouterOSCheck — N°117 — enfile un routeros_check sur chaque routeur
 * agent ciblé (liste explicite = bouton par routeur ; absent = TOUT le parc).
 * Lecture seule : la réponse de chaque routeur arrive à son check-in. */
export async function fleetRouterOSCheck(routerIds?: string[]): Promise<FleetActionResponse> {
  return api<FleetActionResponse>("/api/admin/fleet/routeros-check", {
    method: "POST",
    body: routerIds && routerIds.length > 0 ? { routerIds } : {},
  });
}

/* ─── N°127 — inbox de l'assistant conversationnel (vue « Conversations ») ─── */

/** fetchChatConversations — l'inbox du support : conversations de la
 * vitrine (bot / transmises à un humain / clôturées) + synthèse. */
export async function fetchChatConversations(): Promise<ChatConversationsResponse> {
  return api<ChatConversationsResponse>("/api/admin/chat/conversations");
}

/** fetchChatConversation — le fil complet d'une conversation (marque les
 * messages visiteur non lus comme lus côté serveur). */
export async function fetchChatConversation(id: string): Promise<ChatConversationDetail> {
  return api<ChatConversationDetail>(`/api/admin/chat/conversations/${id}`);
}

/** replyChatConversation — réponse du support : la conversation passe
 * (ou reste) « human » — le bot ne reprend jamais la main ensuite. */
export async function replyChatConversation(
  id: string,
  body: string,
): Promise<{ ok: boolean; message: ChatAdminMessage }> {
  return api<{ ok: boolean; message: ChatAdminMessage }>(
    `/api/admin/chat/conversations/${id}/reply`,
    { method: "POST", body: { body } },
  );
}

/** closeChatConversation — clôture (message de fin côté visiteur, purge
 * automatique 30 jours plus tard). */
export async function closeChatConversation(id: string): Promise<{ ok: boolean }> {
  return api<{ ok: boolean }>(`/api/admin/chat/conversations/${id}/close`, {
    method: "POST",
    body: {},
  });
}

/** fleetRouterOSUpdate — N°117 — installe la mise à jour RouterOS sur les
 * routeurs ciblés. Sans routerIds : uniquement les routeurs avec une mise à
 * jour DÉTECTÉE (état available du dernier check — jamais à l'aveugle : un
 * update redémarre le routeur et coupe le hotspot du client). */
export async function fleetRouterOSUpdate(
  routerIds?: string[],
  latest?: string,
): Promise<FleetActionResponse> {
  return api<FleetActionResponse>("/api/admin/fleet/routeros-update", {
    method: "POST",
    body: routerIds && routerIds.length > 0 ? { routerIds, latest } : { latest },
  });
}

/** createClientAccount — crée un compte client complet (compte + owner).
 * Les identifiants renvoyés doivent être remis au client. N°98 — usage
 * optionnel (défaut « hotspot ») : c'est le chemin de test des comptes
 * HomeNet en Phase 1 (l'inscription publique, elle, reste hotspot seul). */
export async function createClientAccount(payload: {
  name: string;
  username: string;
  password: string;
  usage?: AccountUsage;
}): Promise<{ account: { id: string; name: string; status: string; createdAt: string }; owner: { username: string; role: string } }> {
  return api("/api/admin/accounts", { method: "POST", body: payload });
}

/** fetchPlatformActivity — journal d'activité transverse (tous comptes). */
export async function fetchPlatformActivity(
  params?: { accountId?: string; limit?: number },
): Promise<PlatformActivityRow[]> {
  return api<PlatformActivityRow[]>("/api/admin/activity", {
    params: { accountId: params?.accountId, limit: params?.limit },
  });
}

/** fetchPlatformTeam — membres de l'équipe plateforme (super-admins). */
export async function fetchPlatformTeam(): Promise<PlatformTeamMember[]> {
  return api<PlatformTeamMember[]>("/api/admin/team");
}

/** createPlatformAdmin — ajoute un super-admin plateforme (redondance). */
export async function createPlatformAdmin(payload: {
  name: string;
  username: string;
  password: string;
}): Promise<PlatformTeamMember> {
  return api("/api/admin/team", { method: "POST", body: payload });
}

/** deletePlatformAdmin — retire un super-admin (jamais soi-même, jamais le dernier). */
export async function deletePlatformAdmin(id: string): Promise<{ ok: boolean }> {
  return api<{ ok: boolean }>(`/api/admin/team/${id}`, { method: "DELETE" });
}

/** fetchAccountDetail — fiche détaillée d'un compte client (console plateforme). */
export async function fetchAccountDetail(id: string): Promise<AccountDetail> {
  return api<AccountDetail>(`/api/admin/accounts/${id}`);
}

/** updateAccountSubscription — attribue / renouvelle le plan d'un compte client.
 * Codes 402/400 exploitables : subscription_expired, plan_router_limit, bad_plan… */
export async function updateAccountSubscription(
  id: string,
  payload: SubscriptionUpdatePayload,
): Promise<{ subscription: SubscriptionInfo }> {
  return api(`/api/admin/accounts/${id}/subscription`, { method: "PUT", body: payload });
}

/** deleteClientAccount — supprime un compte client ET toutes ses données (cascade). */
export async function deleteClientAccount(id: string): Promise<{ ok: boolean }> {
  return api<{ ok: boolean }>(`/api/admin/accounts/${id}`, { method: "DELETE" });
}

/** impersonateAccount — ouvre une session support dans la console d'un compte
 * client (bascule à la demande). Renvoie un token scoping ce compte + l'utilisateur
 * enrichi du compte consulté. Erreurs : 404 not_found, 409 account_disabled. */
export async function impersonateAccount(
  id: string,
): Promise<{ token: string; user: AuthUser }> {
  return api<{ token: string; user: AuthUser }>(`/api/admin/accounts/${id}/impersonate`, {
    method: "POST",
  });
}

/** fetchBillingRequests — file des demandes de souscription / renouvellement
 * (console plateforme) : en attente d'abord, puis historique résolu. */
export async function fetchBillingRequests(): Promise<BillingRequestsResponse> {
  return api<BillingRequestsResponse>("/api/admin/billing-requests");
}

/** resolveBillingRequest — traite une demande en attente : « activate »
 * (encaisse, défaut, et active la période) ou « cancel » (rejet). */
export async function resolveBillingRequest(
  id: string,
  payload: { action: "activate" | "cancel"; markPaid?: boolean; note?: string },
): Promise<{ request: BillingRequest }> {
  return api(`/api/admin/billing-requests/${id}/resolve`, { method: "POST", body: payload });
}

/* ─── I (paramètres plateforme) : GET/PUT /api/admin/platform/settings ─── */

/** fetchPlatformSettings — config globale du SaaS (nom, inscriptions). */
export async function fetchPlatformSettings(): Promise<PlatformSettingsResponse> {
  return api<PlatformSettingsResponse>("/api/admin/platform/settings");
}

/** updatePlatformSettings — met à jour le nom affiché et/ou la politique
 * d'inscription (clé d'invitation optionnelle). */
export async function updatePlatformSettings(
  payload: PlatformSettingsUpdatePayload,
): Promise<PlatformSettingsResponse> {
  return api<PlatformSettingsResponse>("/api/admin/platform/settings", {
    method: "PUT",
    body: payload,
  });
}

/* ─── Purge des données FUSIONNÉE (portée globale ou ciblée par compte) ─── */

/** fetchPurgeAccounts — liste des comptes avec leurs compteurs par élément
 * (alimente le sélecteur de PORTÉE et les compteurs de la purge ciblée). */
export async function fetchPurgeAccounts(): Promise<PurgeAccountRow[]> {
  return api<PurgeAccountRow[]>("/api/admin/purge/accounts");
}

/** purgeData — purge UNIFIÉE (POST /api/admin/purge) :
 * accountId vide → portée GLOBALE (les catégories cochées sont supprimées
 * sur TOUS les comptes) ; accountId renseigné → portée CIBLÉE (ce compte
 * seul, les autres ne sont jamais touchés). Scopes : la grille unifiée de
 * 10 catégories (« vouchers », « simulated_routers », « hotspot_users »,
 * « profiles », « batches », « resellers », « sales », « sessions »,
 * « logs », « templates ») ou « all ».
 * options.alsoRouter (défaut false) : purge TOTALE — commande aussi la
 * suppression des comptes sur les routeurs RÉELS (clients déconnectés) ;
 * le backend exige alors options.confirm === "SUPPRIMER" (sinon 400).
 * Les données purgées y sont de toute façon bloquées au ré-import 30 j
 * (tombstones — cf. purged.tombstones / purged.routerRemovals). */
export async function purgeData(
  accountId: string,
  scopes: string[],
  options?: { alsoRouter?: boolean; confirm?: string },
): Promise<PurgeResponse> {
  return api<PurgeResponse>("/api/admin/purge", {
    method: "POST",
    body: {
      ...(accountId ? { accountId } : {}),
      scopes,
      ...(options?.alsoRouter !== undefined ? { alsoRouter: options.alsoRouter } : {}),
      ...(options?.confirm !== undefined ? { confirm: options.confirm } : {}),
    },
  });
}

/* ─── Réglages du compte : GET/PUT /api/settings ─── */

/* N°140 — updateSettings (sauvegarde partielle autoImport/joinButton) est
 * retiré : l'onglet Expérience de la console enregistre désormais TOUT son
 * formulaire en un seul PUT /api/settings depuis hotspot-cards.tsx (corps
 * défensif plat + tenant{…}, même contrat serveur). */

/* ─── M (facturation client) : historique + facture imprimable ─── */

/** fetchBillingHistory — factures du compte (demandes résolues, plus récentes d'abord). */
export async function fetchBillingHistory(): Promise<InvoiceRow[]> {
  return api<InvoiceRow[]>("/api/billing/history");
}

/** invoiceURL — lien vers la facture HTML print-friendly (ouvrir dans un nouvel onglet). */
export function invoiceURL(id: string): string {
  const base = (process.env.NEXT_PUBLIC_API_BASE || "").replace(/\/$/, "");
  if (base) return `${base}/api/billing/invoice/${id}`;
  // Mode passerelle sandbox : le port du backend transite par le proxy local.
  return `/api/billing/invoice/${id}?XTransformPort=4000`;
}

/* ─── N°27 — WiFi jetable : console (authentifiée) + page publique (anonyme) ─── */

import type {
  WifiClaimResponse,
  WifiConsentResponse,
  WifiGuest,
  WifiSite,
  WifiSiteInfo,
  WifiSitePayload,
  WifiSitesResponse,
  WifiStatusResponse,
} from "./types";

/** fetchWifiSites — sites du compte + stats du jour (console). */
export async function fetchWifiSites(): Promise<WifiSitesResponse> {
  return api<WifiSitesResponse>("/api/wifi/sites");
}

/** createWifiSite — création d'un site (slug dérivé du nom, unique global). */
export async function createWifiSite(payload: WifiSitePayload): Promise<WifiSite> {
  return api<WifiSite>("/api/wifi/sites", { method: "POST", body: payload });
}

/** updateWifiSite — mise à jour complète (quotas, plafonds) + bascule active. */
export async function updateWifiSite(id: string, payload: WifiSitePayload): Promise<WifiSite> {
  return api<WifiSite>(`/api/wifi/sites/${encodeURIComponent(id)}`, { method: "PUT", body: payload });
}

/** deleteWifiSite — suppression du site + de son registre visiteurs. */
export async function deleteWifiSite(id: string): Promise<{ ok: boolean; removedGuests: number }> {
  return api<{ ok: boolean; removedGuests: number }>(`/api/wifi/sites/${encodeURIComponent(id)}`, { method: "DELETE" });
}

/** fetchWifiGuests — registre marketing d'un site (gérant uniquement). */
export async function fetchWifiGuests(siteId: string, optIn?: boolean): Promise<{ guests: WifiGuest[]; count: number }> {
  return api<{ guests: WifiGuest[]; count: number }>("/api/wifi/guests", {
    params: { siteId, optIn: optIn === undefined ? undefined : optIn ? "true" : "false" },
  });
}

/** wifiGuestsCsvURL — URL d'export CSV du registre (apiDownload). */
export function wifiGuestsCsvURL(siteId: string): string {
  return `/api/wifi/guests?siteId=${encodeURIComponent(siteId)}&export=csv`;
}

/* — page publique (SANS auth) — */

/** fetchWifiSiteInfo — branding + quotas du site public. */
export async function fetchWifiSiteInfo(slug: string): Promise<WifiSiteInfo> {
  return apiAnon<WifiSiteInfo>(`/api/wifi/site/${encodeURIComponent(slug)}`);
}

/** claimWifiCode — émission du code gratuit (idempotent par téléphone/jour).
 * N°50 : mac (appareil, quand la page est ouverte depuis le portail) alimente
 * le plafond par appareil ; website = honeypot (toujours vide côté UI).
 * N°69 : optIn = état de l'interrupteur « Me tenir informé » (OFF par défaut
 * — jamais de case pré-cochée, consentement univoque). */
export async function claimWifiCode(
  slug: string,
  body: { phone: string; optIn: boolean; mac?: string; website?: string },
): Promise<WifiClaimResponse> {
  return apiAnon<WifiClaimResponse>(`/api/wifi/site/${encodeURIComponent(slug)}/claim`, {
    method: "POST",
    body,
  });
}

/** wifiConsent — N°69 : bascule du consentement marketing du numéro (le
 * retrait « Ne plus recevoir » de la carte code, un éventuel opt-in).
 * website = honeypot (toujours vide côté UI, même contrat que le claim). */
export async function wifiConsent(
  slug: string,
  body: { phone: string; optIn: boolean; website?: string },
): Promise<WifiConsentResponse> {
  return apiAnon<WifiConsentResponse>(`/api/wifi/site/${encodeURIComponent(slug)}/consent`, {
    method: "POST",
    body,
  });
}

/** fetchWifiStatus — état du ticket du jour + offres payantes (bascule). */
export async function fetchWifiStatus(slug: string, phone: string): Promise<WifiStatusResponse> {
  return apiAnon<WifiStatusResponse>(`/api/wifi/site/${encodeURIComponent(slug)}/status`, {
    params: { phone },
  });
}

/* — N°35-d : portail captif (console gérant) — */

/** RedeployRouterPortalResponse — réponse du POST /api/routers/{id}/redeploy-portal. */
export interface RedeployRouterPortalResponse {
  ok: boolean;
  message: string;
}

/** redeployRouterPortal — force le re-déploiement du portail captif sur un
 * routeur agent. Vide la signature HotspotFilesSig côté backend → l'agent
 * re-déploie automatiquement au prochain check-in (≤ 45 s). */
export async function redeployRouterPortal(routerId: string): Promise<RedeployRouterPortalResponse> {
  return api<RedeployRouterPortalResponse>(
    `/api/routers/${encodeURIComponent(routerId)}/redeploy-portal`,
    { method: "POST" },
  );
}

/** RepairWalledGardenResponse — réponse du POST /api/routers/{id}/repair-walled-garden. */
export interface RepairWalledGardenResponse {
  ok: boolean;
  message: string;
}

/** repairRouterWalledGarden — N°49 : force la ré-application du walled-garden
 * d'inscription publique sur un routeur agent (règles page + DNS, deux
 * tables). Vide la signature et l'horodatage côté backend → l'agent réapplique
 * automatiquement au prochain check-in (≤ 45 s). Idempotent : seules les
 * règles marquées mikcloud-wg sont remplacées. */
export async function repairRouterWalledGarden(routerId: string): Promise<RepairWalledGardenResponse> {
  return api<RepairWalledGardenResponse>(
    `/api/routers/${encodeURIComponent(routerId)}/repair-walled-garden`,
    { method: "POST" },
  );
}

/* — N°80 : SafeWiFi (protection DNS du WiFi public) — */

/** Niveau de protection SafeWiFi d'un site (cf. RouterDevice.safeWifiLevel). */
export type SafeWifiLevel = "off" | "threats" | "family";

/** SetRouterSafeWifiResponse — réponse du PUT /api/routers/{id}/safewifi. */
export interface SetRouterSafeWifiResponse {
  ok: boolean;
  level: SafeWifiLevel;
  message: string;
}

/** setRouterSafeWifi — N°80 : change le niveau de protection DNS du WiFi
 * public du site. La commande safewifi est servie au check-in suivant du
 * routeur (≤ 45 s, console ouverte = attention N°75) ; le retour « ok »
 * vérifié (compte de règles marquées rapporté) confirme l'application. */
export async function setRouterSafeWifi(
  routerId: string,
  level: SafeWifiLevel,
): Promise<SetRouterSafeWifiResponse> {
  return api<SetRouterSafeWifiResponse>(
    `/api/routers/${encodeURIComponent(routerId)}/safewifi`,
    { method: "PUT", body: { level } },
  );
}

/* — N°81 : Shield (bouclier réseau du WiFi public) — */

/** Niveau du bouclier Shield d'un site (cf. RouterDevice.shieldLevel). */
export type ShieldLevel = "off" | "on";

/** SetRouterShieldResponse — réponse du PUT /api/routers/{id}/shield. */
export interface SetRouterShieldResponse {
  ok: boolean;
  level: ShieldLevel;
  message: string;
}

/** setRouterShield — N°81 : active ou désactive le bouclier réseau du
 * WiFi public du site. La commande shield est servie au check-in suivant
 * du routeur (≤ 45 s) ; le retour « ok » vérifié (compte de règles
 * marquées == 5 × hotspots rapportés) confirme l'application. */
export async function setRouterShield(
  routerId: string,
  level: ShieldLevel,
): Promise<SetRouterShieldResponse> {
  return api<SetRouterShieldResponse>(
    `/api/routers/${encodeURIComponent(routerId)}/shield`,
    { method: "PUT", body: { level } },
  );
}

/* — N°82 : FamilyGuard (couvre-feu internet du WiFi public) — */

/** Fenêtre du couvre-feu FamilyGuard d'un site (cf. RouterDevice.familyGuardSpec). */
export interface FamilyGuardWindow {
  /** false : configuré mais désactivé (la fenêtre est conservée). */
  enabled: boolean;
  /** Début "HH:MM" (inclus), heure d'Abidjan (GMT). */
  start: string;
  /** Fin "HH:MM" (exclue). */
  end: string;
  /** "1111111" — lundi→dimanche, '1' = la fenêtre démarre ce jour. */
  days: string;
}

/** SetRouterFamilyGuardResponse — réponse du PUT /api/routers/{id}/familyguard. */
export interface SetRouterFamilyGuardResponse {
  ok: boolean;
  enabled: boolean;
  message: string;
}

/** setRouterFamilyGuard — N°82 : programme (ou désactive) le couvre-feu
 * internet du WiFi public du site. L'état désiré (en fenêtre ou non) est
 * recalculé PAR LE CLOUD à chaque check-in : la bascule s'applique au
 * point de contact suivant (≤ 45 s) ; le retour « ok » vérifié (règles
 * marquées == 1 × hotspots rapportés) confirme l'application. */
export async function setRouterFamilyGuard(
  routerId: string,
  window: FamilyGuardWindow,
): Promise<SetRouterFamilyGuardResponse> {
  return api<SetRouterFamilyGuardResponse>(
    `/api/routers/${encodeURIComponent(routerId)}/familyguard`,
    { method: "PUT", body: window },
  );
}

/* — N°88 : AntiVPN (bloque-VPN du WiFi public) — */

/** Niveau du bloque-VPN d'un site (cf. RouterDevice.antiVpnLevel). */
export type AntiVpnLevel = "off" | "on";

/** SetRouterAntiVpnResponse — réponse du PUT /api/routers/{id}/antivpn. */
export interface SetRouterAntiVpnResponse {
  ok: boolean;
  level: AntiVpnLevel;
  message: string;
}

/** setRouterAntiVpn — N°88 : active ou désactive le bloque-VPN du WiFi
 * public du site (VPN et tunnels standards coupés pour les clients).
 * La commande antivpn est servie au check-in suivant du routeur (≤ 45 s) ;
 * le retour « ok » vérifié (compte de règles marquées == 4 × hotspots
 * rapportés) confirme l'application. */
export async function setRouterAntiVpn(
  routerId: string,
  level: AntiVpnLevel,
): Promise<SetRouterAntiVpnResponse> {
  return api<SetRouterAntiVpnResponse>(
    `/api/routers/${encodeURIComponent(routerId)}/antivpn`,
    { method: "PUT", body: { level } },
  );
}

/** fetchRouterPortalPreview — récupère le HTML personnalisé de login.html
 * pour un routeur agent, à injecter dans une iframe srcDoc (aperçu console).
 * Retourne le HTML brut (text/html). */
export async function fetchRouterPortalPreview(routerId: string): Promise<string> {
  const token = useHotspotStore.getState().token;
  const headers: Record<string, string> = { Accept: "text/html" };
  if (token) headers["Authorization"] = `Bearer ${token}`;
  const res = await fetch(buildUrl(`/api/routers/${encodeURIComponent(routerId)}/portal-preview`), {
    method: "GET",
    headers,
    cache: "no-store",
    signal: timeoutSignal(undefined, 20_000),
  });
  if (res.status === 401) {
    useHotspotStore.getState().logout();
    throw new ApiError("Session expirée, veuillez vous reconnecter.", 401);
  }
  if (!res.ok) {
    let message = `Erreur ${res.status}`;
    try {
      const body = await res.json();
      if (body && typeof body.error === "string") message = body.error;
    } catch {
      /* non-JSON */
    }
    throw new ApiError(message, res.status);
  }
  return res.text();
}

/** AccountActivity — entrée du journal d'activité (GET /api/activity). */
export interface AccountActivity {
  id: string;
  type: string;
  message: string;
  at: string;
  actorId?: string;
  actorName?: string;
  /** N°151 — items d'annonce dans la boîte (/api/bell) : niveau + copie. */
  level?: "info" | "warning" | "critical";
  title?: string;
  body?: string;
}

/** fetchAccountActivity — journal d'activité du compte (filtrable côté client
 * sur le type et le message). Limit 1-200. */
export async function fetchAccountActivity(limit = 100): Promise<AccountActivity[]> {
  return api<AccountActivity[]>("/api/activity", { params: { limit: String(limit) } });
}

/* ─── N°151 — boîte de notifications (cloche, GET/POST /api/bell) ─── */

/** Réponse GET /api/bell — items filtrés RBAC + read-state serveur. */
export interface BellResponse {
  items: AccountActivity[];
  /** Dernier acquit de CET utilisateur (vide = première visite, tout lu). */
  seenAt: string;
  /** Entrées visibles plus récentes que seenAt — calculé SERVEUR : le badge
   * est cohérent multi-appareils et par membre de l'équipe (fin du
   * localStorage mikcloud:activity-seen par navigateur). */
  unread: number;
}

/** fetchBell — la boîte de notifications de la cloche (rang 2+). */
export async function fetchBell(limit = 20): Promise<BellResponse> {
  return api<BellResponse>("/api/bell", { params: { limit: String(limit) } });
}

/** markBellSeen — acquitte la cloche (POST /api/bell/seen). L'acquit optionnel
 * `at` sert à la migration de l'ancien localStorage : l'utilisateur garde son
 * avancement ; le serveur le borne à maintenant et ne recule jamais. */
export async function markBellSeen(at?: string): Promise<{ seenAt: string }> {
  return api<{ seenAt: string }>("/api/bell/seen", {
    method: "POST",
    body: at ? { at } : {},
  });
}
