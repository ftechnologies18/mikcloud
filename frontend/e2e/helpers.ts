// Helpers partagés des tests E2E Mode Vente.

import { readFile, writeFile } from "node:fs/promises";
import path from "node:path";

export const API_BASE = "http://localhost:4000";
export const REGISTER_KEY = "cle-e2e";

const STATE_FILE = path.join(__dirname, ".auth-state.json");

/** État écrit par le bootstrap (project « setup ») et lu par les tests UI. */
export interface BootstrapState {
  resellerUsername: string;
  pin: string;
  /** Token revendeur — le PIN est limité à 5 connexions/min/IP (sécurité S2) :
   * UN SEUL login API au bootstrap ; les tests UI injectent la session
   * directement dans le localStorage de l'app. */
  token: string;
  resellerId: string;
  resellerName: string;
  /** Un code ticket situé en 2ᵉ page du stock paginé (tri anti-chronologique). */
  pageTwoCode: string;
}

export async function writeState(state: BootstrapState): Promise<void> {
  await writeFile(STATE_FILE, JSON.stringify(state), "utf-8");
}

export async function readState(): Promise<BootstrapState> {
  return JSON.parse(await readFile(STATE_FILE, "utf-8")) as BootstrapState;
}

/** Session telle que la PWA la stocke (zustand persist « mikcloud-auth »,
 * cf. login-screen setAuth) — injectée via page.addInitScript avant le
 * chargement de l'app, chaque test démarre connecté sans marteler le login. */
export function sessionStorageValue(state: BootstrapState): string {
  return JSON.stringify({
    state: {
      token: state.token,
      user: {
        id: state.resellerId,
        name: state.resellerName,
        username: state.resellerUsername,
        role: "reseller",
      },
      lang: "fr",
      shellMode: "client",
      ownToken: null,
      ownUser: null,
    },
    version: 0,
  });
}

/** Appel API minimal (fetch Node) — échoue bruyamment : le bootstrap ne doit
 * jamais continuer sur un silencieux (un état moitié semé = tests incohérents). */
export async function api(
  path: string,
  opts: { method?: string; token?: string; body?: unknown } = {},
): Promise<Record<string, unknown>> {
  const res = await fetch(`${API_BASE}${path}`, {
    method: opts.method ?? "GET",
    headers: {
      ...(opts.body !== undefined ? { "Content-Type": "application/json" } : {}),
      ...(opts.token ? { Authorization: `Bearer ${opts.token}` } : {}),
    },
    body: opts.body !== undefined ? JSON.stringify(opts.body) : undefined,
  });
  const json: unknown = await res.json().catch(() => ({}));
  if (!res.ok) {
    throw new Error(`${opts.method ?? "GET"} ${path} → ${res.status} ${JSON.stringify(json)}`);
  }
  return json as Record<string, unknown>;
}

/** Variante de api() pour les cas d'erreur ATTENDUS (garde-fous 409, révocation
 * 403…) : retourne le statut et le corps sans lever — le test décide. */
export async function apiRaw(
  path: string,
  opts: { method?: string; token?: string; body?: unknown } = {},
): Promise<{ status: number; json: Record<string, unknown> }> {
  const res = await fetch(`${API_BASE}${path}`, {
    method: opts.method ?? "GET",
    headers: {
      ...(opts.body !== undefined ? { "Content-Type": "application/json" } : {}),
      ...(opts.token ? { Authorization: `Bearer ${opts.token}` } : {}),
    },
    body: opts.body !== undefined ? JSON.stringify(opts.body) : undefined,
  });
  const json: unknown = await res.json().catch(() => ({}));
  return { status: res.status, json: json as Record<string, unknown> };
}
