"use client";

// Client API MikCloud.
// - Production (Vercel · mikcloud.ftci.fr) : URLs relatives /api/* — le proxy
//   vercel.json (rewrites) transfère vers l'API Render. Zéro CORS, zéro réglage.
// - Alternative directe : NEXT_PUBLIC_API_BASE=https://xxx.onrender.com
//   (+ ALLOWED_ORIGIN=https://mikcloud.ftci.fr côté Render).
// - Sandbox : même origine + query param XTransformPort=4000 (passerelle Caddy).

import { useHotspotStore } from "./store";
import type {
  AccountDetail,
  AccountStatus,
  AccountSummary,
  AuthResponse,
  AuthUser,
  BillingRequest,
  BillingRequestsResponse,
  PlatformActivityRow,
  PlatformOverview,
  PlatformSettingsResponse,
  PlatformSettingsUpdatePayload,
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
  constructor(message: string, status: number, code?: string) {
    super(message);
    this.status = status;
    this.code = code;
  }
}

interface ApiOptions {
  method?: "GET" | "POST" | "PUT" | "DELETE";
  body?: unknown;
  params?: Record<string, string | number | undefined>;
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

  const res = await fetch(buildUrl(path, opts.params), {
    method: opts.method ?? "GET",
    headers,
    body: opts.body !== undefined ? JSON.stringify(opts.body) : undefined,
    cache: "no-store",
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

/** apiDownload — télécharge un fichier (CSV, PDF…) renvoyé par l'API, avec token. */
export async function apiDownload(path: string, filename: string, params?: ApiOptions["params"]): Promise<void> {
  const token = useHotspotStore.getState().token;
  const headers: Record<string, string> = {};
  if (token) headers["Authorization"] = `Bearer ${token}`;
  const res = await fetch(buildUrl(path, params), { headers, cache: "no-store" });
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

/** register — inscription SaaS (rôle owner). Renvoie token + utilisateur comme le login. */
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

// ---------------------------------------------------------------------------
// Console plateforme (super-admin MikCloud — multi-comptes)
// ---------------------------------------------------------------------------

/** fetchPlatformOverview — KPIs globaux du SaaS (tous comptes confondus). */
export async function fetchPlatformOverview(): Promise<PlatformOverview> {
  return api<PlatformOverview>("/api/admin/overview");
}

/** createClientAccount — crée un compte client complet (compte + owner).
 * Les identifiants renvoyés doivent être remis au client. */
export async function createClientAccount(payload: {
  name: string;
  username: string;
  password: string;
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
