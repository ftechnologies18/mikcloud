// Playwright — tests E2E du Mode Vente (PWA revendeur) contre la stack réelle :
// backend Go (store JSON éphémère, port 4000) + frontend Next.js (port 3000).
//
// Projects :
//   - « setup »     : bootstrap par API (compte, routeur, profil, lots,
//                    revendeur, une vente tracée) — écrit e2e/.auth-state.json ;
//   - « sell »      : les parcours navigateur du Mode Vente (login PIN,
//                    pagination du stock, recherche exhaustive, vente tactile
//                    R2, rapport + export CSV) — dépend de « setup » ;
//   - « resellers » : cycle de vie d'un revendeur par l'API (gardes 409,
//                    cascade, révocation) — autonome ;
//   - « homenet »   : le parcours familial doré (N°102, Phase 4) — inscription
//                    publique « Ma maison », découverte par baux DHCP, pause
//                    dîner et convergence, gardes d'usage — autonome.
//
// Le frontend est testé en build de production (NEXT_PUBLIC_API_BASE pointe
// directement vers l'API — pas de passerelle ; CORS autorisé côté backend via
// ALLOWED_ORIGIN). Un seul worker : les tests partagent l'état du backend.
//
// N°102 — surcharges locales (CI inchangée : valeurs par défaut = historique) :
//   - E2E_FRONT_PORT / E2E_API_PORT : quand le port 3000 (ou 4000) est déjà
//     occupé sur la machine de dev (un `next dev` d'un autre projet, p.ex.),
//     le webServer « réutiliserait » le mauvais serveur ;
//   - E2E_REGISTER_KEY : vide par défaut → la porte d'inscription est
//     OUVERTE, condition du parcours UI publique (l'inscription « Ma maison »
//     n'envoie pas de clé — le sélecteur d'usage N°101 est LA porte d'entrée
//     du foyer). Une valeur referme la porte en mode bêta privée : le
//     bootstrap envoie toujours REGISTER_KEY (ignorée porte ouverte, exigée
//     porte fermée) — le refus sans clé est couvert par les tests Go.

import { defineConfig } from "@playwright/test";

const FRONT_PORT = Number(process.env.E2E_FRONT_PORT ?? 3000);
const API_PORT = Number(process.env.E2E_API_PORT ?? 4000);
const REGISTER_KEY = process.env.E2E_REGISTER_KEY ?? "";

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
    { name: "homenet", testMatch: /homenet\.spec\.ts/ },
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
        // N°102 — vide par défaut : porte d'inscription OUVERTE (parcours UI
        // publique du spec homenet). E2E_REGISTER_KEY referme en bêta privée.
        REGISTER_KEY,
        // N°102 — le runner partage UNE IP pour quatre suites : sans marge,
        // le plafond S1-A2 (120 req/min/IP, preflights OPTIONS compris)
        // coupait la dernière suite schedulée en 429. Production : 120 (env
        // absent côté Render — cf. main.go).
        RATE_API_PER_MIN: "600",
        // N°114 — même maladie, même remède pour le quota S3 d'inscription :
        // un run complet consomme 5 inscriptions (bootstrap 1 + revendeurs 3
        // + foyer 1) = EXACTEMENT le plafond burst 5/10 min ; le retry d'un
        // groupe serial REJOUE l'inscription déjà passée → 6e tentative →
        // 429 fantôme qui masquait l'échec réel. Bornes NAT-friendly N°50.
        // Production : env absent → bornes S3 (5/10 min + 20/24 h).
        SIGNUP_BURST_MAX: "20",
        SIGNUP_DAILY_MAX: "100",
        ALLOWED_ORIGIN: `http://localhost:${FRONT_PORT}`,
      },
    },
    {
      // next start respecte -p ; le port suit E2E_FRONT_PORT (3000 par défaut).
      command: `bunx next start -p ${FRONT_PORT}`,
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
