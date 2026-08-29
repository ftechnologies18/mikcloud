"use client";

// Client API MikCloud.
// - En production (Vercel → Render) : NEXT_PUBLIC_API_BASE=https://xxx.onrender.com
// - Dans la sandbox : même origine + query param XTransformPort=4000 (passerelle Caddy).

import { useHotspotStore } from "./store";

const API_BASE = (process.env.NEXT_PUBLIC_API_BASE || "").replace(/\/$/, "");
const GATEWAY_PORT = "4000";

export class ApiError extends Error {
  status: number;
  constructor(message: string, status: number) {
    super(message);
    this.status = status;
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
  if (!API_BASE) merged["XTransformPort"] = GATEWAY_PORT; // mode passerelle sandbox
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
    const message =
      (data && typeof data === "object" && "error" in data && typeof (data as { error: unknown }).error === "string"
        ? (data as { error: string }).error
        : null) ?? `Erreur ${res.status}`;
    throw new ApiError(message, res.status);
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
