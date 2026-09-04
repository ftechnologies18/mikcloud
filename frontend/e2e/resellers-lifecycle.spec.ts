// E2E — cycle de vie d'un revendeur (audit purge/revendeurs V1→V5), par l'API
// RÉELLE (surface publique, store éphémère) :
//   - V1 : DELETE refusé 409 tant que le revendeur porte du stock vendable
//     puis du crédit restant — motifs détaillés dans le corps structuré ;
//   - V2 : une fois soldé, le DELETE cascade l'historique de transactions
//     (compteur transactionsPurged retourné au gérant) ;
//   - V4 : après DELETE, le token Mode Vente du revendeur est révoqué (403).
//
// Le scénario nettoie lui-même ses positions — retour de stock (revendeur),
// débit manuel du crédit (gérant) : exactement les actions que l'UI propose
// pour régulariser avant suppression. Aucun partage d'état avec le bootstrap.

import { expect, test } from "@playwright/test";

import { api, apiRaw, REGISTER_KEY } from "./helpers";

const SUFFIX = Date.now().toString(36); // unique entre les réutilisations locales
const PHONE = "07" + String(Date.now()).slice(-8); // anti-abus : numéro unique par run

test("cycle de vie revendeur — gardes 409, cascade 200, révocation 403", async () => {
  // 1. Compte dédié + routeur simulé + profil tarifaire.
  const reg = await api("/api/auth/register", {
    method: "POST",
    body: {
      name: "Gérant Cycle",
      username: `gerant-cyc-${SUFFIX}`,
      password: "mot-de-passe-8+",
      key: REGISTER_KEY,
      email: `cyc-${SUFFIX}@example.ci`,
      phone: PHONE,
      country: "CI",
      city: "Abidjan",
    },
  });
  const token = reg.token as string;
  expect(token).toBeTruthy();

  const router = await api("/api/routers", {
    method: "POST",
    token,
    body: { name: "site-cyc", host: "10.5.50.1", mode: "simulated" },
  });
  const profile = await api("/api/profiles", {
    method: "POST",
    token,
    body: { name: "1 Heure", price: 200, sellingPrice: 200, validityDays: 7, sessionTimeoutMin: 60 },
  });
  expect(router.id).toBeTruthy();
  expect(profile.id).toBeTruthy();

  // 2. Revendeur prépayé crédité + stock attribué (lot de 3).
  const reseller = await api("/api/resellers", {
    method: "POST",
    token,
    body: { name: "Vendeur Cycle", username: `cyc-${SUFFIX}`, pin: "1357", credit: 50_000 },
  });
  const batch = await api("/api/vouchers/generate", {
    method: "POST",
    token,
    body: { count: 3, profileId: profile.id, routerId: router.id },
  });
  await api(`/api/vouchers/batch/${batch.batchId}/transfer`, {
    method: "POST",
    token,
    body: { resellerId: reseller.id },
  });

  // 3. V1 — DELETE refusé : du stock attribué vendable (motif détaillé).
  const guardStock = await apiRaw(`/api/resellers/${reseller.id}`, { method: "DELETE", token });
  expect(guardStock.status).toBe(409);
  expect(String(guardStock.json.error)).toContain("en stock");
  expect(guardStock.json.stock).toBe(3);

  // 4. Le revendeur rend son stock — retour recrédité (Mode Vente).
  const login = await api("/api/reseller/login", {
    method: "POST",
    body: { username: `cyc-${SUFFIX}`, pin: "1357" },
  });
  const resToken = login.token as string;
  expect(resToken).toBeTruthy();
  const stock = await api("/api/sell/stock?limit=60&offset=0", { token: resToken });
  const ids = (stock.items as Array<{ id: string }>).map((it) => it.id);
  expect(ids.length).toBe(3);
  const ret = await api("/api/sell/return", { method: "POST", token: resToken, body: { ids } });
  expect(ret.returned).toBe(3);
  expect(ret.credited).toBe(600);

  // 5. V1 — DELETE à nouveau refusé : du crédit restant.
  const guardCredit = await apiRaw(`/api/resellers/${reseller.id}`, { method: "DELETE", token });
  expect(guardCredit.status).toBe(409);
  expect(String(guardCredit.json.error)).toContain("crédit restant");
  expect(guardCredit.json.credit).toBe(50_000);

  // 6. Débit manuel du crédit (action gérant) → revendeur soldé.
  await api(`/api/resellers/${reseller.id}/credit`, {
    method: "POST",
    token,
    body: { amount: -50_000, note: "Solde avant suppression" },
  });

  // 7. V2 — DELETE autorisé : cascade de l'historique de transactions
  //    (transfert, retour de stock, débit) — compteur retourné.
  const del = await apiRaw(`/api/resellers/${reseller.id}`, { method: "DELETE", token });
  expect(del.status).toBe(200);
  expect(del.json.ok).toBe(true);
  expect(del.json.transactionsPurged).toBe(3);
  expect(del.json.vouchersDetached).toBe(0); // déjà détachés par le retour

  // 8. V4 — le token Mode Vente survivant est révoqué de fait (403).
  const revoked = await apiRaw("/api/sell/stock?limit=10&offset=0", { token: resToken });
  expect(revoked.status).toBe(403);
  expect(String(revoked.json.error)).toContain("revendeur supprimé");
});
