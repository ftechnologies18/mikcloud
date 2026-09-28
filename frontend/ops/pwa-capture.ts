// ops/pwa-capture.ts — N°192 : capture des écrans réels du manifeste PWA.
//
// UNE commande régénère public/screenshots/*.jpg (les captures du dialogue
// d'installation Android/desktop Chrome) à partir de la stack locale :
//
//   cd frontend
//   NEXT_PUBLIC_API_BASE=http://localhost:4000 bun run build   # 1× (build prod)
//   bun ops/pwa-capture.ts
//
// Ce que fait le script :
//  1. Démarre ce qui manque (réutilisation sinon, pattern webServer e2e) :
//     - backend Go en store JSON ÉPHÉMÈRE (/tmp/mikcloud-pwa-capture, effacé
//       au départ → seed déterministe ; RATE_API_PER_MIN=600 comme le runner
//       e2e : le poll de sessions consomme le budget 120/min par défaut ;
//       ADMIN_PASSWORD fixe le mot de passe de l'admin plateforme — le seed
//       en a besoin pour activer la formule, voir ci-dessous) ;
//     - frontend `next start -p 3100` (build de production REQUIS, bâti avec
//       NEXT_PUBLIC_API_BASE=http://localhost:4000 — sinon la console parle
//       en /api relatif et les vues restent vides) ;
//     - serveurs lancés en GROUPE DE PROCESSUS DÉTACHÉ avec logs dans
//       /tmp/mikcloud-pwa-{backend,frontend}.log : le script tue le groupe
//       entier en partant (go run + binaire enfant, bunx + next) — un enfant
//       survivant ne bloque JAMAIS la sortie. En cas de kill -9 du script :
//       pkill -f mikcloud-pwa-capture ; pkill -f "next start -p 3100" ;
//  2. Sème un parc réaliste PAR L'API RÉELLE : gérant, 3 routeurs simulés
//     (Abidjan), 2 profils, ~56 vouchers, un revendeur avec 5 ventes du jour
//     (le tableau de bord n'est jamais à zéro). L'essai couvre 1 routeur
//     (garde plan_router_limit) : le script demande la formule annuelle puis
//     l'active côté plateforme (POST /api/subscription → admin → billing
//     request resolve activate) — le chemin RÉEL d'un client qui monte,
//     jamais une manipulation du store ;
//  3. Fait vivre la simulation (chaque GET /api/sessions déclenche un Tick)
//     jusqu'à ≥ 5 sessions réparties sur ≥ 2 routeurs — la loupe routeur
//     (N°191) a des badges vivants à montrer ;
//  4. Capture 5 écrans : vitrine + connexion (narrow), connexion (wide),
//     console Sessions avec loupe (narrow 390×844 @2x → 780×1688), tableau
//     de bord (wide 1280×800 @1x). Les dimensions DOIVENT rester stables —
//     le manifeste les référence en dur.
//
// Les jetons/captures ne sortent jamais de la machine : le compte sème vit
// dans le store éphémère, jeté à la fin du processus backend.

import { spawn, type ChildProcess } from "node:child_process";
import { open, rm } from "node:fs/promises";
import path from "node:path";
import { chromium } from "@playwright/test";

const API_PORT = 4000;
const FRONT_PORT = 3100;
const API = `http://localhost:${API_PORT}`;
const FRONT = `http://localhost:${FRONT_PORT}`;
const OUT_DIR = path.join(process.cwd(), "public", "screenshots");
const DATA_DIR = "/tmp/mikcloud-pwa-capture";
const ADMIN_PASSWORD = "capture-admin-2026";
const SUFFIX = Date.now().toString(36);

/** Réponse register/login — champ `user` du gérant. */
interface AuthResponse {
  token: string;
  user: { id: string; name: string; username: string; role: string };
}
interface RouterCreated {
  id: string;
}
interface ProfileCreated {
  id: string;
}
interface BatchCreated {
  batchId: string;
}
interface BillingRequest {
  id: string;
  accountId: string;
  status: string;
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

/** Appel API minimal (fetch Node) — échoue bruyamment, jamais de silencieux ;
 * timeout 30 s : une requête pendue doit ABANDONNER, pas figer le script. */
async function api<T>(
  urlPath: string,
  opts: { method?: string; token?: string; body?: unknown } = {},
): Promise<T> {
  const res = await fetch(`${API}${urlPath}`, {
    method: opts.method ?? "GET",
    headers: {
      ...(opts.body !== undefined ? { "Content-Type": "application/json" } : {}),
      ...(opts.token ? { Authorization: `Bearer ${opts.token}` } : {}),
    },
    body: opts.body !== undefined ? JSON.stringify(opts.body) : undefined,
    signal: AbortSignal.timeout(30_000),
  });
  const json: unknown = await res.json().catch(() => ({}));
  if (!res.ok) {
    throw new Error(`${opts.method ?? "GET"} ${urlPath} → ${res.status} ${JSON.stringify(json)}`);
  }
  return json as T;
}

/** Attend qu'une URL réponde 2xx (démarrage backend/frontend). */
async function waitUp(url: string, label: string, timeoutMs: number): Promise<boolean> {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    try {
      const res = await fetch(url, { signal: AbortSignal.timeout(2000) });
      if (res.ok) return true;
    } catch {
      /* pas encore prêt */
    }
    await sleep(300);
  }
  console.error(`✗ ${label} ne répond pas sur ${url}`);
  return false;
}

interface Managed {
  proc: ChildProcess;
  log: Awaited<ReturnType<typeof open>>;
}

interface Stack {
  procs: Managed[];
}

/** Tue le GROUPE détaché entier (parent + enfants) — jamais un orphelin qui
 * retient un pipe ou un port. */
function killGroup(m: Managed): void {
  if (m.proc.pid !== undefined) {
    try {
      process.kill(-m.proc.pid, "SIGTERM");
    } catch {
      /* groupe déjà parti */
    }
  }
}

/** Démarre backend + frontend s'ils ne tournent pas déjà (réutilisation sinon). */
async function ensureStack(stack: Stack): Promise<void> {
  const backendUp = await fetch(`${API}/`)
    .then((r) => r.ok)
    .catch(() => false);
  if (!backendUp) {
    await rm(DATA_DIR, { recursive: true, force: true });
    const log = await open("/tmp/mikcloud-pwa-backend.log", "w");
    const proc = spawn("go", ["run", "."], {
      cwd: path.join(process.cwd(), "..", "backend"),
      env: {
        ...process.env,
        // Store JSON éphémère — jamais DATABASE_URL (garde de production P0).
        DATABASE_URL: "",
        PORT: String(API_PORT),
        DATA_DIR,
        RATE_API_PER_MIN: "600",
        SIGNUP_BURST_MAX: "20",
        SIGNUP_DAILY_MAX: "100",
        ALLOWED_ORIGIN: FRONT,
        // Admin plateforme au mot de passe connu — le seed active la formule
        // annuelle par le chemin réel (billing request → resolve).
        ADMIN_PASSWORD,
      },
      detached: true,
      stdio: ["ignore", log.fd, log.fd],
    });
    proc.unref();
    stack.procs.push({ proc, log });
    if (!(await waitUp(`${API}/`, "backend", 120_000))) {
      throw new Error(
        "backend introuvable — Go 1.27+ est-il installé ? (logs : /tmp/mikcloud-pwa-backend.log)",
      );
    }
    console.log("✓ backend JSON démarré (:4000, store éphémère, logs /tmp/mikcloud-pwa-backend.log)");
  } else {
    console.log("• backend déjà lancé (:4000) — réutilisé");
  }

  const frontUp = await fetch(`${FRONT}/`)
    .then((r) => r.ok)
    .catch(() => false);
  if (!frontUp) {
    const log = await open("/tmp/mikcloud-pwa-frontend.log", "w");
    const proc = spawn("bunx", ["next", "start", "-p", String(FRONT_PORT)], {
      cwd: process.cwd(),
      env: { ...process.env, NEXT_PUBLIC_API_BASE: API },
      detached: true,
      stdio: ["ignore", log.fd, log.fd],
    });
    proc.unref();
    stack.procs.push({ proc, log });
    if (!(await waitUp(`${FRONT}/`, "frontend", 60_000))) {
      throw new Error(
        `frontend introuvable — build requis : NEXT_PUBLIC_API_BASE=${API} bun run build (logs : /tmp/mikcloud-pwa-frontend.log)`,
      );
    }
    console.log("✓ frontend démarré (:3100, logs /tmp/mikcloud-pwa-frontend.log)");
  } else {
    console.log("• frontend déjà lancé (:3100) — réutilisé");
  }
}

/** Session telle que la PWA la stocke (zustand persist « mikcloud-auth »). */
function authStorageValue(auth: AuthResponse): string {
  return JSON.stringify({
    state: {
      token: auth.token,
      user: auth.user,
      view: "dashboard",
      sidebarOpen: false,
      lang: "fr",
      shellMode: "client",
      ownToken: null,
      ownUser: null,
    },
    version: 0,
  });
}

/** Monte le compte en formule annuelle par le CHEMIN RÉEL : demande client
 * (POST /api/subscription) puis activation plateforme (login admin →
 * billing request → resolve activate). L'essai plafonne à 1 routeur
 * (plan_router_limit) — la loupe N°191 en exige ≥ 2. */
async function upgradeToAnnual(auth: AuthResponse): Promise<void> {
  await api("/api/subscription", {
    method: "POST",
    token: auth.token,
    body: { planId: "hotspot-annuel" },
  });
  const admin = await api<AuthResponse>("/api/auth/login", {
    method: "POST",
    body: { username: "admin", password: ADMIN_PASSWORD },
  });
  const listed = await api<{ requests?: BillingRequest[] } | BillingRequest[]>("/api/admin/billing-requests", {
    token: admin.token,
  });
  const requests = Array.isArray(listed) ? listed : (listed.requests ?? []);
  const pending = requests.find((b) => b.status === "pending");
  if (!pending) throw new Error("aucune demande d'abonnement en attente — activation impossible");
  await api(`/api/admin/billing-requests/${pending.id}/resolve`, {
    method: "POST",
    token: admin.token,
    body: { action: "activate" },
  });
  console.log("✓ formule annuelle activée (billing request réelle, routeurs illimités)");
}

/** Sème le parc : gérant, 3 routeurs, profils, vouchers, ventes du jour. */
async function seed(): Promise<AuthResponse> {
  const auth = await api<AuthResponse>("/api/auth/register", {
    method: "POST",
    body: {
      name: "Awa Koné",
      username: `awa-${SUFFIX}`,
      password: "capture-pwa-2026",
      key: "",
      email: `awa-${SUFFIX}@example.ci`,
      phone: `07${String(Date.now()).slice(-8)}`,
      country: "CI",
      city: "Abidjan",
    },
  });

  // Routeur 1 couvert par l'essai, puis montée en formule annuelle pour les
  // deux autres (la loupe routeur N°191 a besoin d'un parc ≥ 2).
  const ebrie = await api<RouterCreated>("/api/routers", {
    method: "POST",
    token: auth.token,
    body: { name: "Résidence Ébrié", host: "10.5.50.1", mode: "simulated" },
  });
  await upgradeToAnnual(auth);
  const yopougon = await api<RouterCreated>("/api/routers", {
    method: "POST",
    token: auth.token,
    body: { name: "Cyber Yopougon", host: "10.5.50.1", mode: "simulated" },
  });
  const plateau = await api<RouterCreated>("/api/routers", {
    method: "POST",
    token: auth.token,
    body: { name: "Maquis Plateau", host: "10.5.50.1", mode: "simulated" },
  });

  const hourly = await api<ProfileCreated>("/api/profiles", {
    method: "POST",
    token: auth.token,
    body: { name: "1 Heure", price: 200, sellingPrice: 200, validityDays: 7, sessionTimeoutMin: 60 },
  });
  const daily = await api<ProfileCreated>("/api/profiles", {
    method: "POST",
    token: auth.token,
    body: { name: "Journalier", price: 500, sellingPrice: 500, validityDays: 14, sessionTimeoutMin: 1440 },
  });

  // Stock par routeur — le lot du Maquis part chez le revendeur (vendu dans
  // la journée), les deux autres alimentent la simulation de sessions.
  await api<BatchCreated>("/api/vouchers/generate", {
    method: "POST",
    token: auth.token,
    body: { count: 24, profileId: hourly.id, routerId: ebrie.id },
  });
  await api<BatchCreated>("/api/vouchers/generate", {
    method: "POST",
    token: auth.token,
    body: { count: 18, profileId: daily.id, routerId: yopougon.id },
  });
  const maquis = await api<BatchCreated>("/api/vouchers/generate", {
    method: "POST",
    token: auth.token,
    body: { count: 14, profileId: hourly.id, routerId: plateau.id },
  });

  const reseller = await api<{ id: string }>("/api/resellers", {
    method: "POST",
    token: auth.token,
    body: { name: "Ulrich Kofi", username: `ulrich-${SUFFIX}`, pin: "2468", credit: 50_000 },
  });
  await api(`/api/vouchers/batch/${maquis.batchId}/transfer`, {
    method: "POST",
    token: auth.token,
    body: { resellerId: reseller.id },
  });

  // 5 ventes du jour — le tableau de bord montre stock vivant + revenus.
  const login = await api<AuthResponse>("/api/reseller/login", {
    method: "POST",
    body: { username: `ulrich-${SUFFIX}`, pin: "2468" },
  });
  const stock = await api<{ items: Array<{ id: string }> }>("/api/sell/stock?limit=60&offset=0", {
    token: login.token,
  });
  for (const item of stock.items.slice(0, 5)) {
    await api(`/api/sell/${item.id}/sold`, { method: "POST", token: login.token });
  }
  console.log("✓ parc semé : 3 routeurs, 2 profils, 56 vouchers, 5 ventes du jour");

  // Fait vivre la simulation : chaque lecture déclenche un Tick (~30 % de
  // nouvelle session, ~12 % de fin). Objectif : ≥ 5 sessions sur ≥ 2 routeurs
  // — les chips de la loupe portent des badges vivants (N°191).
  const deadline = Date.now() + 90_000;
  for (;;) {
    const sessions = await api<Array<{ routerId: string }>>("/api/sessions", {
      token: auth.token,
    });
    const routersAlive = new Set(sessions.map((s) => s.routerId)).size;
    if (sessions.length >= 5 && routersAlive >= 2) {
      console.log(`✓ simulation vivante : ${sessions.length} sessions sur ${routersAlive} routeurs`);
      return auth;
    }
    if (Date.now() > deadline) {
      throw new Error(
        `simulation trop lente (${sessions.length} sessions / ${routersAlive} routeurs après 90 s)`,
      );
    }
    await sleep(600);
  }
}

/** Contexte mobile « narrow » — 390×844 @2x → JPEG 780×1688 (manifeste).
 * reducedMotion : les révélations d'entrée framer-motion (vitrine, CTA)
 * aboutissent INSTANTANÉMENT — sinon la capture attrape un texte à mi-révélation
 * (constaté : libellé du bouton héro semi-transparent, N°192). */
function mobileContextOpts() {
  return {
    viewport: { width: 390, height: 844 },
    deviceScaleFactor: 2,
    isMobile: true,
    hasTouch: true,
    locale: "fr-FR",
    timezoneId: "Africa/Abidjan",
    reducedMotion: "reduce",
  } as const;
}

/** Contexte desktop « wide » — 1280×800 @1x (manifeste). */
function desktopContextOpts() {
  return {
    viewport: { width: 1280, height: 800 },
    deviceScaleFactor: 1,
    locale: "fr-FR",
    timezoneId: "Africa/Abidjan",
    reducedMotion: "reduce",
  } as const;
}

async function main(): Promise<void> {
  const stack: Stack = { procs: [] };
  try {
    await ensureStack(stack);
    const auth = await seed();

    const browser = await chromium.launch();
    try {
      // 1. Vitrine — narrow.
      const vitrine = await browser.newContext(mobileContextOpts());
      const vitrinePage = await vitrine.newPage();
      await vitrinePage.goto(`${FRONT}/`, { waitUntil: "load", timeout: 60_000 });
      await vitrinePage.waitForTimeout(1500); // animations d'entrée
      await vitrinePage.screenshot({
        type: "jpeg",
        quality: 82,
        path: path.join(OUT_DIR, "landing-narrow.jpg"),
      });
      await vitrine.close();
      console.log("✓ landing-narrow.jpg");

      // 2. Connexion — narrow + wide (funnel commun console / Mode Vente).
      const loginNarrow = await browser.newContext(mobileContextOpts());
      const loginNarrowPage = await loginNarrow.newPage();
      await loginNarrowPage.goto(`${FRONT}/login`, { waitUntil: "load", timeout: 60_000 });
      await loginNarrowPage.getByText("Se connecter").first().waitFor();
      await loginNarrowPage.waitForTimeout(1200);
      await loginNarrowPage.screenshot({
        type: "jpeg",
        quality: 82,
        path: path.join(OUT_DIR, "login-narrow.jpg"),
      });
      await loginNarrow.close();
      console.log("✓ login-narrow.jpg");

      const loginWide = await browser.newContext(desktopContextOpts());
      const loginWidePage = await loginWide.newPage();
      await loginWidePage.goto(`${FRONT}/login`, { waitUntil: "load", timeout: 60_000 });
      await loginWidePage.getByText("Se connecter").first().waitFor();
      await loginWidePage.waitForTimeout(1200);
      await loginWidePage.screenshot({
        type: "jpeg",
        quality: 82,
        path: path.join(OUT_DIR, "login-wide.jpg"),
      });
      await loginWide.close();
      console.log("✓ login-wide.jpg");

      // 3. Console — Sessions avec la loupe routeur (N°191), narrow. La
      // session du gérant est injectée dans localStorage AVANT le premier
      // script (pattern e2e : zéro parcours de login à rejouer).
      const sessions = await browser.newContext(mobileContextOpts());
      await sessions.addInitScript(
        (value: string) => localStorage.setItem("mikcloud-auth", value),
        authStorageValue(auth),
      );
      const sessionsPage = await sessions.newPage();
      await sessionsPage.goto(`${FRONT}/app/sessions`, { waitUntil: "load" });
      await sessionsPage.getByRole("button", { name: /Tous les routeurs/ }).waitFor();
      await sessionsPage.getByRole("heading", { name: "Sessions actives" }).waitFor();
      await sessionsPage.locator("tbody tr").first().waitFor();
      await sessionsPage.waitForTimeout(2500); // KPI, durées, badges
      await sessionsPage.screenshot({
        type: "jpeg",
        quality: 82,
        path: path.join(OUT_DIR, "console-sessions-narrow.jpg"),
      });
      await sessions.close();
      console.log("✓ console-sessions-narrow.jpg");

      // 4. Console — tableau de bord, wide (KPI + graphiques + activité).
      const dash = await browser.newContext(desktopContextOpts());
      await dash.addInitScript(
        (value: string) => localStorage.setItem("mikcloud-auth", value),
        authStorageValue(auth),
      );
      const dashPage = await dash.newPage();
      await dashPage.goto(`${FRONT}/app`, { waitUntil: "load" });
      await dashPage.getByRole("heading", { name: "Tableau de bord" }).waitFor();
      await dashPage.locator(".recharts-surface").first().waitFor();
      await dashPage.waitForTimeout(2500);
      await dashPage.screenshot({
        type: "jpeg",
        quality: 82,
        path: path.join(OUT_DIR, "console-wide.jpg"),
      });
      await dash.close();
      console.log("✓ console-wide.jpg");
    } finally {
      await browser.close();
    }
    console.log("\n5 captures écrites dans public/screenshots/ — vérifiez les");
    console.log("dimensions (780×1688 / 1280×800) avant de servir le manifeste.");
  } finally {
    // Tue uniquement ce que CE script a démarré (réutilisation respectée).
    for (const m of stack.procs) killGroup(m);
    await sleep(500);
    for (const m of stack.procs) await m.log.close();
  }
}

main().catch((error: unknown) => {
  console.error(error);
  process.exit(1);
});
