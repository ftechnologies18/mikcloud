// Bootstrap E2E (project « setup ») — sème un état complet PAR L'API RÉELLE :
// inscription gérant → routeur simulé → profil → lot de 70 (pagination) +
// lot de 3 → revendeur prépayé avec PIN → transferts → une vente tactile
// tracée (le rapport du jour n'est jamais vide). Aucun endpoint de console
// en GET (ils alimentent la simulation d'activité : le stock resterait stable
// mais l'état ne doit rien devoir au hasard).
//
// Le ticket cherché par les tests UI en 2ᵉ page est capturé DEPUIS l'endpoint
// paginé lui-même (offset=60) — par construction, il y figure.

import { expect, test } from "@playwright/test";

import { api, REGISTER_KEY, writeState } from "./helpers";

const SUFFIX = Date.now().toString(36); // unique entre les réutilisations locales
const PHONE = "07" + String(Date.now()).slice(-8); // anti-abus : numéro unique par run

test("bootstrap — compte, stock paginé, revendeur PIN, une vente tracée", async () => {
  // 1. Inscription gérant (inscriptions fermées sans clé — fail-closed).
  const reg = await api("/api/auth/register", {
    method: "POST",
    body: {
      name: "Gérant E2E",
      username: `gerant-${SUFFIX}`,
      password: "mot-de-passe-8+",
      key: REGISTER_KEY,
      email: `e2e-${SUFFIX}@example.ci`,
      phone: PHONE,
      country: "CI",
      city: "Abidjan",
    },
  });
  const ownerToken = reg.token as string;
  expect(ownerToken).toBeTruthy();

  // 2. Routeur simulé + profil tarifaire.
  const router = await api("/api/routers", {
    method: "POST",
    token: ownerToken,
    body: { name: "site-e2e", host: "10.5.50.1", mode: "simulated" },
  });
  const profile = await api("/api/profiles", {
    method: "POST",
    token: ownerToken,
    body: { name: "1 Heure", price: 200, sellingPrice: 200, validityDays: 7, sessionTimeoutMin: 60 },
  });
  expect(router.id).toBeTruthy();
  expect(profile.id).toBeTruthy();

  // 3. Deux lots : 70 tickets (→ pagination : > 60) + 3 tickets.
  const big = await api("/api/vouchers/generate", {
    method: "POST",
    token: ownerToken,
    body: { count: 70, profileId: profile.id, routerId: router.id },
  });
  const small = await api("/api/vouchers/generate", {
    method: "POST",
    token: ownerToken,
    body: { count: 3, profileId: profile.id, routerId: router.id },
  });
  expect(big.batchId).toBeTruthy();
  expect(small.batchId).toBeTruthy();

  // 4. Revendeur prépayé avec PIN + transfert des deux lots.
  const reseller = await api("/api/resellers", {
    method: "POST",
    token: ownerToken,
    body: { name: "Ulrich E2E", username: `ulrich-${SUFFIX}`, pin: "2468", credit: 50_000 },
  });
  await api(`/api/vouchers/batch/${big.batchId}/transfer`, {
    method: "POST",
    token: ownerToken,
    body: { resellerId: reseller.id },
  });
  await api(`/api/vouchers/batch/${small.batchId}/transfer`, {
    method: "POST",
    token: ownerToken,
    body: { resellerId: reseller.id },
  });

  // 5. Connexion revendeur (PIN) — token scopé Mode Vente.
  const login = await api("/api/reseller/login", {
    method: "POST",
    body: { username: `ulrich-${SUFFIX}`, pin: "2468" },
  });
  const resellerToken = login.token as string;
  expect(resellerToken).toBeTruthy();

  // 6. Une vente déjà tracée via l'API (vente tactile) — le rapport du jour
  // contient au moins cette ligne, jamais un décompte fantôme.
  const stockPage1 = await api("/api/sell/stock?limit=60&offset=0", { token: resellerToken });
  const items = stockPage1.items as Array<{ id: string; username: string }>;
  expect(items.length).toBe(60); // 73 tickets actifs — page 1 pleine
  await api(`/api/sell/${items[0].id}/sold`, { method: "POST", token: resellerToken });

  // 7. Un code de la 2ᵉ page (par construction : l'endpoint paginé le renvoie).
  const stockPage2 = await api("/api/sell/stock?limit=60&offset=60", { token: resellerToken });
  const page2 = stockPage2.items as Array<{ username: string }>;
  expect(page2.length).toBeGreaterThan(0);

  await writeState({
    resellerUsername: `ulrich-${SUFFIX}`,
    pin: "2468",
    token: resellerToken,
    resellerId: reseller.id as string,
    resellerName: "Ulrich E2E",
    pageTwoCode: page2[page2.length - 1].username,
  });
});
