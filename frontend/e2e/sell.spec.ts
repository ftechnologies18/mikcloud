// E2E Mode Vente — les parcours du revendeur dans le vrai navigateur, contre
// la stack réelle (backend Go + Next.js de production). Un worker : les tests
// partagent l'état semé par le bootstrap et s'enchaînent dans l'ordre —
// chaque mutation (vente) sert au test suivant (rapport de journée).
//
// Sessions : le login PIN UI est testé explicitement dans le 1ᵉʳ test ;
// les tests suivants injectent la session revendeur dans le localStorage
// (addInitScript) — /api/reseller/login est limité à 5 req/min/IP (S2).

import { readFileSync } from "node:fs";
import { expect, test, type Page } from "@playwright/test";

import { readState, sessionStorageValue, type BootstrapState } from "./helpers";

let state: BootstrapState;

/** Ouvre /sell avec la session revendeur déjà injectée. */
async function gotoAsReseller(page: Page): Promise<void> {
  await page.addInitScript(
    ([key, value]) => {
      window.localStorage.setItem(key, value as string);
    },
    ["mikcloud-auth", sessionStorageValue(state)],
  );
  await page.goto("/sell");
}

test.describe.serial("Mode Vente — PWA revendeur", () => {
  test.beforeAll(async () => {
    state = await readState();
  });

  test("connexion PIN → le comptoir s'ouvre avec les groupes de stock", async ({ page }) => {
    // Parcours de connexion UI réel (onglet « Vente », identifiant + PIN).
    await page.goto("/login");
    await page.getByRole("tab", { name: "Vente" }).click();
    await page.getByLabel(/Identifiant/).fill(state.resellerUsername);
    await page.getByLabel(/PIN/).fill(state.pin);
    await page.getByRole("button", { name: "Ouvrir le Mode Vente" }).click();
    await expect(page.getByText("Mode Vente ·")).toBeVisible();
    await expect(page.getByRole("main", { name: "En stock" })).toBeVisible();
    // Le stock est groupé par profil (UX R1) et la bannière R5 explique
    // la vente automatique à la connexion.
    await expect(page.getByText("1 Heure").first()).toBeVisible();
    await expect(page.getByText("Vente automatique à la connexion")).toBeVisible();
  });

  test("pagination du stock — la 1ʳᵉ page se charge, « Afficher plus » complète", async ({ page }) => {
    await gotoAsReseller(page);
    // 72 tickets en stock → la page 1 (60) s'affiche avec le bouton.
    const loadMore = page.getByRole("button", { name: /Afficher plus/ });
    await expect(loadMore).toBeVisible();
    await expect(loadMore).toContainText("60 sur 7");
    await loadMore.click();
    // Toutes les pages chargées : le bouton disparaît, le compteur grimpe.
    await expect(loadMore).toHaveCount(0);
    await expect(page.getByText(/[7][0-2] ticket\(s\) en stock/)).toBeVisible();
  });

  test("recherche exhaustive — un ticket de la 2ᵉ page est retrouvé (code masqué)", async ({ page }) => {
    await gotoAsReseller(page);
    // Le contexte neuf n'a chargé que la page 1 : chercher un code de la
    // page 2 doit forcer le chargement du reste (invariant R3 : la recherche
    // porte sur TOUT le stock, jamais « aucun résultat » à tort).
    await page.getByLabel("Rechercher un code, un profil, un lot…").fill(state.pageTwoCode);
    // Anti-fuite — le code cherché ne s'affiche PAS : la carte reste muette
    // tant que la vente n'est pas confirmée ; c'est le bouton « Vendu »
    // (unique — le filtre ne laisse qu'une carte) qui prouve le résultat.
    await expect(page.getByRole("button", { name: "Vendu" })).toHaveCount(1, { timeout: 15_000 });
    await expect(page.locator("main").getByText(state.pageTwoCode)).toHaveCount(0);
    await expect(page.getByText(/Aucun ticket ne correspond/)).toHaveCount(0);
  });

  test("vente tactile — code masqué avant confirmation, reçu partageable après", async ({ page }) => {
    await gotoAsReseller(page);
    await page.getByLabel("Rechercher un code, un profil, un lot…").fill(state.pageTwoCode);
    // R2 — le misclick ne vend pas : Annuler laisse le stock intact, et le
    // récapitulatif pré-confirmation ne révèle pas le code (anti-fuite).
    await page.getByRole("button", { name: "Vendu" }).click();
    const dialog = page.getByRole("dialog", { name: "Vendre ce ticket ?" });
    await expect(dialog).toBeVisible();
    await expect(dialog.getByText(state.pageTwoCode)).toHaveCount(0);
    await dialog.getByRole("button", { name: "Annuler" }).click();
    await expect(dialog).toHaveCount(0);
    await expect(page.getByRole("button", { name: "Vendu" })).toHaveCount(1);
    // Confirmation → la vente est tracée, le ticket quitte le comptoir et le
    // reçu « Vente confirmée » devient la SEULE porte de sortie du code.
    await page.getByRole("button", { name: "Vendu" }).click();
    await page
      .getByRole("dialog", { name: "Vendre ce ticket ?" })
      .getByRole("button", { name: "Confirmer la vente" })
      .click();
    await expect(page.getByText("Voucher remis au client")).toBeVisible();
    const receipt = page.getByRole("dialog", { name: "Vente confirmée" });
    await expect(receipt).toBeVisible();
    await expect(receipt.getByText(state.pageTwoCode)).toBeVisible();
    // Partage : Web Share absent de Chromium headless → fallback presse-
    // papiers ; le presse-papiers contient bien le code.
    await page.context().grantPermissions(["clipboard-read", "clipboard-write"]);
    await receipt.getByRole("button", { name: "Partager" }).click();
    await expect(page.getByText("Code copié dans le presse-papiers")).toBeVisible();
    const clip = await page.evaluate(() => navigator.clipboard.readText());
    expect(clip).toContain(state.pageTwoCode);
  });

  test("rapport de journée — ventilation par canal + export CSV comptable", async ({ page }) => {
    await gotoAsReseller(page);
    await page.getByRole("button", { name: "Rapport de fin de journée" }).click();
    const report = page.getByRole("dialog", { name: "Rapport de fin de journée" });
    await expect(report).toBeVisible();
    // P3-d — stats + ventilation par canal (2 ventes : 1 API + 1 UI, tactile).
    await expect(report.getByText("Recette")).toBeVisible();
    await expect(report.getByText(/Tactile · \d+/)).toBeVisible();
    await expect(report.getByText(state.pageTwoCode)).toBeVisible();
    // P3-d — export comptable : le téléchargement aboutit et le journal
    // contient l'entête, la vente du test et les totaux du jour.
    const [download] = await Promise.all([
      page.waitForEvent("download"),
      report.getByRole("button", { name: "Exporter CSV" }).click(),
    ]);
    expect(download.suggestedFilename()).toMatch(/^journal-caisse-\d{4}-\d{2}-\d{2}.*\.csv$/);
    const body = readFileSync(await download.path(), "utf-8");
    expect(body).toContain("Journal de caisse");
    expect(body).toContain(state.pageTwoCode);
    expect(body).toContain("Total ventes ;2 ; ;400 ;");
    expect(body.startsWith("\uFEFF")).toBe(true); // BOM UTF-8 (Excel)
  });
});
