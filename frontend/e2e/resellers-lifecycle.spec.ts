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
// N°22 — chaque test du fichier s'enregistre avec SON numéro (unicité compte).
const PHONE_MASQ = "05" + String(Date.now()).slice(-8);

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

test("N°22 — codes revendeur masqués côté gérant, impression tracée, code verrouillé", async () => {
  // 1. Compte dédié + routeur simulé + profil tarifaire.
  const reg = await api("/api/auth/register", {
    method: "POST",
    body: {
      name: "Gérant Masq",
      username: `gerant-masq-${SUFFIX}`,
      password: "mot-de-passe-8+",
      key: REGISTER_KEY,
      email: `masq-${SUFFIX}@example.ci`,
      phone: PHONE_MASQ,
      country: "CI",
      city: "Abidjan",
    },
  });
  const token = reg.token as string;
  expect(token).toBeTruthy();

  const router = await api("/api/routers", {
    method: "POST",
    token,
    body: { name: "site-masq", host: "10.5.50.1", mode: "simulated" },
  });
  const profile = await api("/api/profiles", {
    method: "POST",
    token,
    body: { name: "1 Heure", price: 200, sellingPrice: 200, validityDays: 7, sessionTimeoutMin: 60 },
  });
  expect(router.id).toBeTruthy();
  expect(profile.id).toBeTruthy();

  // 2. Lot de 2 dont UN SEUL ticket transféré au revendeur (l'autre reste en
  //    stock direct) : les deux traversent toutes les vérifications.
  const reseller = await api("/api/resellers", {
    method: "POST",
    token,
    body: { name: "Vendeur Masq", username: `masq-${SUFFIX}`, pin: "2468", credit: 50_000 },
  });
  const batch = await api("/api/vouchers/generate", {
    method: "POST",
    token,
    body: { count: 2, profileId: profile.id, routerId: router.id },
  });
  await api(`/api/vouchers/batch/${batch.batchId}/transfer`, {
    method: "POST",
    token,
    body: { resellerId: reseller.id, quantity: 1 },
  });

  // 3. Liste gérant : le ticket transféré sort MASQUÉ, le direct en clair.
  const list = await api(`/api/vouchers?search=${batch.batchId}&pageSize=10`, { token });
  const rows = list.data as Array<{
    id: string;
    username: string;
    password: string;
    resellerId: string;
    resellerName: string;
  }>;
  expect(rows.length).toBe(2);
  const resRow = rows.find((r) => r.resellerId !== "");
  const directRow = rows.find((r) => r.resellerId === "");
  expect(resRow).toBeTruthy();
  expect(directRow).toBeTruthy();
  expect(resRow!.username).toBe("••••••");
  expect(resRow!.password).toBe("");
  expect(resRow!.resellerName).toBe("Vendeur Masq");
  const directCode = directRow!.username;
  expect(directCode).not.toBe("••••••");

  // 4. Canal d'impression tracé : codes COMPLETS rendus + trace revendeur.
  const printed = await api("/api/vouchers/print", {
    method: "POST",
    token,
    body: { ids: [resRow!.id, directRow!.id] },
  });
  const printedVouchers = printed.vouchers as Array<{ id: string; username: string }>;
  const printedRes = printedVouchers.find((v) => v.id === resRow!.id);
  expect(printedRes).toBeTruthy();
  expect(printedRes!.username).not.toBe("••••••");
  expect(String(printedRes!.username).length).toBeGreaterThan(0);
  expect(printed.tracedCount).toBe(1);

  // 5. Le code d'un ticket attribué n'est pas réinscriptible depuis la console.
  const locked = await apiRaw(`/api/users/${resRow!.id}`, {
    method: "PUT",
    token,
    body: { username: "HACKED" },
  });
  expect(locked.status).toBe(403);
  expect(locked.json.code).toBe("reseller_voucher_locked");

  // 6. Recherche par le VRAI code (ticket papier qui revient) : la ligne
  //    ressort — mais TOUJOURS masquée en sortie.
  const found = await api(`/api/vouchers?search=${directCode}`, { token });
  expect(found.total).toBe(1);
  const foundRow = (found.data as Array<{ username: string; resellerId: string }>)[0];
  expect(foundRow.resellerId).toBe("");
  expect(foundRow.username).toBe(directCode);

  const foundRes = await api(`/api/vouchers?search=${(printedRes as { username: string }).username}`, { token });
  const resHit = (foundRes.data as Array<{ username: string; resellerId: string }>).find(
    (r) => r.resellerId !== "",
  );
  expect(resHit).toBeTruthy();
  expect(resHit!.username).toBe("••••••");
});

test("N°23 — reprise gérant (W6) : recrédit prépayé, gardes de destruction (W1), boucle fermée", async () => {
  // Numéro dédié (anti-abus : un numéro unique par compte et par run).
  const PHONE_REP = "01" + String(Date.now()).slice(-8);

  // 1. Compte dédié + routeur simulé + profil tarifaire.
  const reg = await api("/api/auth/register", {
    method: "POST",
    body: {
      name: "Gérant Reprise",
      username: `gerant-rep-${SUFFIX}`,
      password: "mot-de-passe-8+",
      key: REGISTER_KEY,
      email: `rep-${SUFFIX}@example.ci`,
      phone: PHONE_REP,
      country: "CI",
      city: "Abidjan",
    },
  });
  const token = reg.token as string;
  expect(token).toBeTruthy();

  const router = await api("/api/routers", {
    method: "POST",
    token,
    body: { name: "site-rep", host: "10.5.50.1", mode: "simulated" },
  });
  const profile = await api("/api/profiles", {
    method: "POST",
    token,
    body: { name: "1 Heure", price: 300, sellingPrice: 300, validityDays: 7, sessionTimeoutMin: 60 },
  });
  expect(router.id).toBeTruthy();
  expect(profile.id).toBeTruthy();

  // 2. Revendeur prépayé + lot de 3 transféré (stock revendeur).
  const reseller = await api("/api/resellers", {
    method: "POST",
    token,
    body: { name: "Vendeur Reprise", username: `rep-v-${SUFFIX}`, pin: "3579", credit: 10_000 },
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

  // 3. W1 — aucune porte de destruction : unitaire 403, bulk 409, lot 409.
  const login = await api("/api/reseller/login", {
    method: "POST",
    body: { username: `rep-v-${SUFFIX}`, pin: "3579" },
  });
  const resToken = login.token as string;
  expect(resToken).toBeTruthy();
  const stock = await api("/api/sell/stock?limit=60&offset=0", { token: resToken });
  const ids = (stock.items as Array<{ id: string }>).map((it) => it.id);
  expect(ids.length).toBe(3);

  const delUnit = await apiRaw(`/api/vouchers/${ids[0]}`, { method: "DELETE", token });
  expect(delUnit.status).toBe(403);
  expect(delUnit.json.code).toBe("reseller_voucher_locked");

  const delBulk = await apiRaw("/api/users/bulk", {
    method: "POST",
    token,
    body: { ids, action: "delete" },
  });
  expect(delBulk.status).toBe(409);
  expect(String(delBulk.json.error)).toContain("revendeur");

  const delBatch = await apiRaw(`/api/vouchers/batch/${batch.batchId}/delete`, { method: "POST", token });
  expect(delBatch.status).toBe(409);
  expect(String(delBatch.json.error)).toContain("reprise");

  // 4. W6 — reprise gérant de 2 tickets : recrédit prépayé au prix gros.
  const reprise = await api("/api/vouchers/reprise", {
    method: "POST",
    token,
    body: { ids: [ids[0], ids[1]] },
  });
  expect(reprise.returned).toBe(2);
  expect(reprise.credited).toBe(600); // 2 × 300 — le portefeuille suit le retour
  const details = (reprise.details as string[]) ?? [];
  expect(details.some((d) => d.includes("Vendeur Reprise"))).toBe(true);

  // 5. Le crédit du revendeur suit l'argent : 10 000 − 900 (débit au
  //    transfert, prépayé) + 600 (recrédit de la reprise) = 9 700.
  const resellers = (await api("/api/resellers", { token })) as unknown as Array<{
    id: string;
    credit: number;
  }>;
  const res = resellers.find((r) => r.id === reseller.id);
  expect(res?.credit).toBe(9_700);

  // 6. W3/W4 — le filtre détenteur sépare le stock : 2 repris côté direct
  //    (codes en clair), 1 reste alloué (masqué).
  const direct = await api(`/api/vouchers?search=${batch.batchId}&holder=direct&pageSize=10`, { token });
  expect(direct.total).toBe(2);
  const allocated = await api(`/api/vouchers?search=${batch.batchId}&holder=reseller&pageSize=10`, { token });
  expect(allocated.total).toBe(1);
  const allocatedRow = (allocated.data as Array<{ username: string; resellerName: string }>)[0];
  expect(allocatedRow.username).toBe("••••••");
  expect(allocatedRow.resellerName).toBe("Vendeur Reprise");

  // 7. Boucle fermée — reprise du dernier, puis les portes de destruction
  //    s'ouvrent (unitaire 200, lot 200).
  const reprise2 = await api("/api/vouchers/reprise", {
    method: "POST",
    token,
    body: { ids: [ids[2]] },
  });
  expect(reprise2.credited).toBe(300);
  const resellers2 = (await api("/api/resellers", { token })) as unknown as Array<{
    id: string;
    credit: number;
  }>;
  const res2 = resellers2.find((r) => r.id === reseller.id);
  expect(res2?.credit).toBe(10_000);

  const directRows = (direct.data as Array<{ id: string; username: string }>).slice();
  const codes = ((reprise.codes as string[]) ?? []).concat((reprise2.codes as string[]) ?? []);
  const victim = directRows.find((r) => codes.includes(r.username));
  expect(victim).toBeTruthy();
  const delUnitOk = await apiRaw(`/api/vouchers/${victim!.id}`, { method: "DELETE", token });
  expect(delUnitOk.status).toBe(200);
  const delBatchOk = await apiRaw(`/api/vouchers/batch/${batch.batchId}/delete`, { method: "POST", token });
  expect(delBatchOk.status).toBe(200);
});
