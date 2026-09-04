// Playwright — tests E2E du Mode Vente (PWA revendeur) contre la stack réelle :
// backend Go (store JSON éphémère, port 4000) + frontend Next.js (port 3000).
//
// Deux projects :
//   - « setup » : bootstrap par API (compte, routeur, profil, lots, revendeur,
//     une vente tracée) — écrit e2e/.auth-state.json ;
//   - « sell »  : les parcours navigateur (login PIN, pagination du stock,
//     recherche exhaustive, vente tactile R2, rapport + export CSV) — dépend
//     de « setup ».
//
// Le frontend est testé en build de production (NEXT_PUBLIC_API_BASE pointe
// directement vers l'API — pas de passerelle ; CORS autorisé côté backend via
// ALLOWED_ORIGIN). Un seul worker : les tests partagent l'état du backend.

import { defineConfig } from "@playwright/test";

const FRONT_PORT = 3000;
const API_PORT = 4000;

export default defineConfig({
  testDir: "./e2e",
  timeout: 90_000,
  expect: { timeout: 10_000 },
  fullyParallel: false,
  workers: 1,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [["list"], ["html", { open: "never" }]] : [["list"]],
  use: {
    baseURL: `http://localhost:${FRONT_PORT}`,
    trace: "retain-on-failure",
    locale: "fr-FR",
  },
  projects: [
    { name: "setup", testMatch: /bootstrap\.spec\.ts/ },
    { name: "sell", testMatch: /sell\.spec\.ts/, dependencies: ["setup"] },
    { name: "resellers", testMatch: /resellers-lifecycle\.spec\.ts/ },
  ],
  webServer: [
    {
      command: "go run .",
      cwd: "../backend",
      port: API_PORT,
      reuseExistingServer: !process.env.CI,
      timeout: 120_000,
      env: {
        ...process.env,
        // Mode dev FORCÉ : l'environnement hôte peut porter DATABASE_URL
        // (le backend refuserait de démarrer — garde de production P0).
        DATABASE_URL: "",
        PORT: String(API_PORT),
        DATA_DIR: "/tmp/mikcloud-e2e-api",
        REGISTER_KEY: "cle-e2e",
        ALLOWED_ORIGIN: `http://localhost:${FRONT_PORT}`,
      },
    },
    {
      command: "bun run start",
      port: FRONT_PORT,
      reuseExistingServer: !process.env.CI,
      timeout: 120_000,
      env: {
        ...process.env,
        NEXT_PUBLIC_API_BASE: `http://localhost:${API_PORT}`,
      },
    },
  ],
});
