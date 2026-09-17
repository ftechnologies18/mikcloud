// E2E HomeNet — le parcours familial DORÉ (Phase 4 N°102), contre la stack
// réelle (backend Go store JSON + Next.js de production) :
//   1. inscription PUBLIQUE « HomeNet » par l'UI — le sélecteur d'usage
//      N°101 est LA porte d'entrée du foyer, l'atterrissage est SA console
//      (/app/home, invitation honnête tant que la box n'est pas connectée) ;
//   2. le zéro inexpliqué n'existe plus (N°102) : un routeur SANS mode agent
//      → le KPI Appareils dit le geste à faire et l'enseigne ambrée mène aux
//      routeurs ;
//   3. la box découvre le foyer : check-in agent → rapport read_dhcp (bails)
//      → registre → KPI « 3 », l'enseigne disparaît, le KPI ouvre la vue ;
//   4. nom affecté (« TV du salon ») + LA pause dîner (30 min sur le tel de
//      mama) : chip En pause, convergence serveur (device_pause servie puis
//      signée — silence au check-in suivant), rétablissement ;
//   5. gardes d'usage : la famille « devices » n'existe pas pour un compte
//      hotspot (404 serveur, re-normalisation console), et le signet métier
//      d'un foyer retombe sur SA maison.
//
// Autonome : aucun partage d'état avec le bootstrap (ses propres comptes,
// SUFFIX unique par run). La porte d'inscription doit être OUVERTE pour le
// parcours UI sans clé (cf. playwright.config.ts — REGISTER_KEY vide par
// défaut en E2E). Un worker : les tests s'enchaînent dans l'ordre — chacun
// consomme l'état du précédent (compte → box → appareils → pause).
//
// Leçon N°101 (consignée au worklog) respectée : AUCUN appel /agent/cmd en
// dehors de la boucle agent honnête — chaque check-in CONSOMME les commandes
// servies ; on rapporte tout ce qu'on sert.

import { expect, test, type Page } from "@playwright/test";

import { API_BASE, api, apiRaw, REGISTER_KEY } from "./helpers";

const SUFFIX = Date.now().toString(36); // unique entre les réutilisations locales
const PHONE = "07" + String(Date.now()).slice(-8); // anti-abus : numéro unique par run
const PASSWORD = "mot-de-passe-8+";

// Les trois appareils du foyer, tels que la box les rapporte (bails DHCP,
// format F9 : MAC|IP|host|expires|statut, entrées terminées par « ; »).
const MAC_TV = "AA:BB:CC:DD:EE:01";
const MAC_TEL = "AA:BB:CC:DD:EE:02";
const MAC_LAPTOP = "AA:BB:CC:DD:EE:03";
const LEASES = `${MAC_TV}|192.168.88.10|tv-salon|30m|bound;${MAC_TEL}|192.168.88.11|tel-mama|25m|bound;${MAC_LAPTOP}|192.168.88.12|laptop-enfant|20m|bound;`;

/** Session brute « mikcloud-auth » captée après l'inscription UI (la vérité
 * du store persisté, réinjectée telle quelle — zéro reconstruction). */
let familySession: string;
let familyToken: string;
let routerId: string;
let agentToken: string;

// ---------------------------------------------------------------------------
// Boucle agent honnête (miroir des tests Go N°101 — devices_test.go) :
// GET /agent/cmd consomme la file, le rapport POST /agent/result signe.
// ---------------------------------------------------------------------------

interface ServedCmd {
  id: string;
  kind: string;
  /** Lignes du script situées sous le commentaire d'audit de CETTE commande. */
  lines: string[];
}

/** Check-in : renvoie le script servi (commandes « sent », à rapporter). */
async function agentCheckIn(tok: string): Promise<string> {
  const res = await fetch(`${API_BASE}/agent/cmd?token=${tok}`);
  if (res.status !== 200) {
    throw new Error(`check-in : statut ${res.status}`);
  }
  return res.text();
}

/** Inscription de la box (POST /agent/register — le geste du script
 * d'installation) : déclare l'identité, le modèle et la VERSION RouterOS.
 * La garde TLS stricte (P0 #5) refuse tout check-in sous 7.19 — la box de
 * test déclare « 7.20 (stable) » comme celle du gérant. */
async function agentRegister(tok: string): Promise<void> {
  const body = new URLSearchParams({
    token: tok,
    identity: `maison-${SUFFIX}.lan`, // empreinte unique par run (S6)
    model: "RB2011UiAS",
    version: "7.20 (stable)",
    uptime: "3w1d02:15:30",
  });
  const res = await fetch(`${API_BASE}/agent/register`, {
    method: "POST",
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    body: body.toString(),
  });
  if (res.status !== 200) {
    throw new Error(`inscription agent : statut ${res.status}`);
  }
}

/** Découpe le script par commentaires d'audit « # mikcloud cmd <id> <kind> ». */
function servedCmds(script: string): ServedCmd[] {
  const cmds: ServedCmd[] = [];
  for (const line of script.split("\n")) {
    if (line.includes("mikcloud cmd ")) {
      const parts = line.trim().split(/\s+/);
      // parts : # mikcloud cmd <id> <kind> …
      if (parts.length >= 5) {
        cmds.push({ id: parts[3], kind: parts[4], lines: [] });
      }
    } else if (cmds.length > 0) {
      cmds[cmds.length - 1].lines.push(line);
    }
  }
  return cmds;
}

/** Rapport d'exécution d'une commande (formulaire, comme l'agent réel). */
async function agentReport(tok: string, cmdID: string, fields: Record<string, string>): Promise<void> {
  const body = new URLSearchParams({ token: tok, cmd: cmdID, ...fields });
  const res = await fetch(`${API_BASE}/agent/result`, {
    method: "POST",
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    body: body.toString(),
  });
  if (res.status !== 200) {
    throw new Error(`rapport ${cmdID} : statut ${res.status}`);
  }
}

/** Rapporte la lecture read_dhcp avec les baux donnés (inventaire du foyer). */
async function reportReadDhcp(tok: string, script: string, data: string): Promise<void> {
  for (const cmd of servedCmds(script)) {
    if (cmd.kind === "read_dhcp") {
      await agentReport(tok, cmd.id, { status: "ok", data });
    }
  }
}

/** Routeur honnête : chaque device_pause servie rapporte rules = nombre de
 * règles IPv4 réellement posables par SON script (une par MAC — le compte
 * exact qui signe, post-mortem N°93 : un compte faux ne signe pas). */
async function reportServedDevicePauses(tok: string, script: string): Promise<number> {
  let n = 0;
  for (const cmd of servedCmds(script)) {
    if (cmd.kind === "device_pause") {
      const rules = cmd.lines.filter((l) => l.includes("/ip firewall filter add")).length;
      await agentReport(tok, cmd.id, { status: "ok", rules: String(rules) });
      n++;
    }
  }
  return n;
}

// ---------------------------------------------------------------------------
// Sessions UI — injection localStorage (le login UI est testé une fois, à
// l'inscription ; les tests suivants réinjectent la session du foyer).
// ---------------------------------------------------------------------------

async function gotoAsFamily(page: Page, path = "/app/home"): Promise<void> {
  await page.addInitScript(
    ([key, value]) => {
      window.localStorage.setItem(key, value as string);
    },
    ["mikcloud-auth", familySession],
  );
  await page.goto(path);
}

/** Session minimale pour un compte créé par l'API (miroir du store persisté). */
function sessionValueFor(token: string, user: Record<string, unknown>): string {
  return JSON.stringify({
    state: { token, user, lang: "fr", shellMode: "client", ownToken: null, ownUser: null },
    version: 0,
  });
}

// ---------------------------------------------------------------------------

test.describe.serial("HomeNet — le parcours familial", () => {
  test("inscription publique « HomeNet » → atterrissage SA console", async ({ page }) => {
    // Le sélecteur d'usage (N°101) ouvre l'inscription au foyer : deux cartes
    // radio en tête du formulaire, hotspot par défaut (zéro changement pour
    // les clients existants — ici on choisit explicitement la maison).
    await page.goto("/");
    await page.getByRole("button", { name: "Créer mon compte" }).first().click();
    const dialog = page.getByRole("dialog");
    await expect(dialog).toBeVisible();
    await dialog.getByRole("radio", { name: /HomeNet/ }).click();

    // Étape 1 — compte.
    await page.locator("#signup-name").fill("Famille Yopougon");
    await page.locator("#signup-username").fill(`famille-${SUFFIX}`);
    await page.locator("#signup-password").fill(PASSWORD);
    await dialog.getByRole("button", { name: "Continuer" }).click();

    // Étape 2 — profil (le pays ouvre la liste radix ; clic d'option réel).
    await page.locator("#signup-email").fill(`famille-${SUFFIX}@example.ci`);
    await page.locator("#signup-phone").fill(PHONE);
    await page.locator("#signup-country").click();
    await page.getByRole("option", { name: /Côte d'Ivoire/ }).click();
    await page.locator("#signup-city").fill("Abidjan");
    await page.locator("#signup-privacy").click();
    await dialog.getByRole("button", { name: "Créer mon compte" }).click();

    // La session est posée → la vitrine cède la place à la console, et la
    // console d'un FOYER, c'est « Votre maison » — pas le dashboard métier.
    await expect(page).toHaveURL(/\/app\/home$/);
    await expect(page.getByRole("heading", { name: "Votre maison" })).toBeVisible();
    // L'invitation honnête (N°100) : aucune box connectée, aucun faux zéro.
    await expect(page.getByText("Votre maison attend son routeur")).toBeVisible();

    // La session réelle du store (usage homenet inclus) alimente les tests
    // suivants — vérité du persisté, jamais une reconstruction.
    familySession = (await page.evaluate(() => window.localStorage.getItem("mikcloud-auth"))) as string;
    const parsed = JSON.parse(familySession) as { state: { token: string } };
    familyToken = parsed.state.token;
    expect(familyToken).toBeTruthy();
  });

  test("routeur sans mode agent — le zéro inexpliqué n'existe plus (N°102)", async ({ page }) => {
    // Une box existe MAIS pas en mode agent : l'inventaire (et la pause
    // dîner) ne peut pas vivre. Le KPI dit le geste, l'enseigne y mène.
    // (Le plan essai couvre UNE box : c'est celle-là qu'on connectera en
    // mode agent au test suivant — le geste exact que l'enseigne enseigne.)
    const router = await api("/api/routers", {
      method: "POST",
      token: familyToken,
      body: { name: "MAISON YOPOUGON", host: "192.168.88.1", mode: "simulated" },
    });
    routerId = router.id as string;
    expect(routerId).toBeTruthy();

    await gotoAsFamily(page);
    await expect(page.getByRole("button", { name: /Appareils en ligne/ })).toContainText("0");
    await expect(page.getByText("connectez votre box en mode agent")).toBeVisible();
    // L'enseigne ambrée : la vérité terrain (bails DHCP de la box agent) et
    // la sortie (Voir mes routeurs — N°112 : la section Infrastructure de la
    // navigation principale héberge la fiche box).
    await expect(page.getByText(/découverts par votre box MikroTik/)).toBeVisible();
    await page.getByRole("button", { name: "Voir mes routeurs" }).click();
    // N°112 — la fiche box vit désormais dans la section Infrastructure de la
    // navigation principale (/app/routers), plus dans la zone Paramètres
    // (/app/settings/routers reste deep-linkable — re-normalisation legacy).
    await expect(page).toHaveURL(/\/app\/routers$/);
  });

  test("la box découvre le foyer — baux DHCP → registre → KPI → vue Appareils", async ({ page }) => {
    // La famille connecte SA box en mode agent (PUT — le plan essai couvre
    // une seule box, c'est la sienne) : un token agent est forgé, la box
    // passera en ligne à son premier check-in.
    const updated = await api(`/api/routers/${routerId}`, {
      method: "PUT",
      token: familyToken,
      body: { mode: "agent" },
    });
    agentToken = updated.agentToken as string;
    expect(agentToken).toBeTruthy();

    // Le script d'installation inscrit la box (identité + version) avant le
    // premier check-in — sinon la garde TLS répond 426 (version inconnue).
    await agentRegister(agentToken);

    // Premier check-in : le cadenceur foyer enfile read_dhcp (et le
    // convergeur une device_pause à ensemble vide). Routeur honnête : tout
    // ce qui est servi est rapporté — les bails nourrissent le registre.
    const script = await agentCheckIn(agentToken);
    await reportReadDhcp(agentToken, script, LEASES);
    await reportServedDevicePauses(agentToken, script);

    await gotoAsFamily(page);
    // L'enseigne disparaît (une box agent existe), le KPI compte les bails.
    await expect(page.getByText(/découverts par votre box MikroTik/)).toHaveCount(0);
    await expect(page.getByText("connectez votre box en mode agent")).toHaveCount(0);
    const kpi = page.getByRole("button", { name: /Appareils en ligne/ });
    await expect(kpi).toContainText("3");
    await expect(kpi).toContainText("sur votre WiFi, maintenant");

    // N°102 — le KPI est LA porte de la vue Appareils (raccourci pause
    // dîner : un parent pressé clique le compteur, choisit, coupe).
    await kpi.click();
    await expect(page).toHaveURL(/\/app\/devices$/);
    for (const host of ["tv-salon", "tel-mama", "laptop-enfant"]) {
      await expect(page.getByRole("row", { name: new RegExp(host) })).toBeVisible();
    }
  });

  test("nom affecté + pause dîner — convergence serveur puis rétablissement", async ({ page }) => {
    await gotoAsFamily(page, "/app/devices");

    // — Le nom affecté : « TV du salon » (registre cloud, 48 runes) —
    const tvRow = page.getByRole("row", { name: /tv-salon/ });
    await tvRow.getByRole("button", { name: "Renommer" }).click();
    const renameDialog = page.getByRole("dialog", { name: "Nommer l'appareil" });
    // Le champ n'a pas de <label> lié (placeholder « TV du salon ») — c'est
    // l'unique textbox du dialog.
    await renameDialog.getByRole("textbox").fill("TV du salon");
    await renameDialog.getByRole("button", { name: "Enregistrer" }).click();
    await expect(page.getByRole("row", { name: /TV du salon/ })).toBeVisible();

    // — LA pause dîner : 30 minutes sur le tel de mama —
    const telRow = page.getByRole("row", { name: /tel-mama/ });
    await telRow.getByRole("button", { name: "Mettre en pause" }).click();
    await page.getByRole("menuitem", { name: "30 minutes" }).click();
    await expect(page.getByText(/Internet de tel-mama coupé pendant 30 minutes/)).toBeVisible();
    await expect(telRow).toContainText("En pause");

    // — Convergence : la box sert la device_pause (une règle par MAC, en
    // tête de chaîne) puis le rapport EXACT signe — le check-in suivant ne
    // re-file RIEN (même signature : vérité routeur, pattern N°93/101). —
    const script = await agentCheckIn(agentToken);
    expect(script).toContain(`src-mac-address="${MAC_TEL}"`);
    expect(await reportServedDevicePauses(agentToken, script)).toBe(1);
    const silence = await agentCheckIn(agentToken);
    for (const cmd of servedCmds(silence)) {
      expect(cmd.kind, `commande re-servie après convergence : ${cmd.kind}`).not.toBe("device_pause");
    }

    // — Le parent change d'avis : internet rétabli, l'appareil reprend —
    await telRow.getByRole("button", { name: "Rétablir internet" }).click();
    await expect(page.getByText(/Internet de tel-mama rétabli/)).toBeVisible();
    await expect(telRow).toContainText("En ligne");
  });

  test("gardes d'usage — le miroir serveur et la re-normalisation console", async ({ page }) => {
    // Un établissement (compte hotspot) ne connaît PAS la famille devices :
    // 404 serveur (garde N°98/101), et sa console n'affiche jamais la
    // sidebar « Votre maison » (coquille N°100).
    //
    // L'établissement voisin s'inscrit depuis SA connexion : un run CI
    // complet consomme 6 inscriptions (bootstrap 1 + revendeurs 3 + foyer 2)
    // contre un quota anti-ferme de 5/10 min PAR IP — le quota est borné par
    // le PREMIER HOP X-Forwarded-For (comportement documenté du limiteur S3),
    // le voisin déclare donc la sienne. Réalisme ET quota honnête.
    const regBody = {
      name: "Cyber E2E",
      username: `cyber-${SUFFIX}`,
      password: PASSWORD,
      // La clé passe porte fermée (CI manuelle), est ignorée porte ouverte.
      key: REGISTER_KEY,
      email: `cyber-${SUFFIX}@example.ci`,
      phone: "05" + String(Date.now()).slice(-8),
      country: "CI",
      city: "Abidjan",
    };
    const regRes = await fetch(`${API_BASE}/api/auth/register`, {
      method: "POST",
      headers: { "Content-Type": "application/json", "X-Forwarded-For": "10.9.8.7" },
      body: JSON.stringify(regBody),
    });
    const reg = (await regRes.json()) as Record<string, unknown>;
    expect(regRes.status, JSON.stringify(reg)).toBe(201);
    const guard = await apiRaw("/api/devices", { token: reg.token as string });
    expect(guard.status).toBe(404);

    // Lien direct /app/devices d'un compte hotspot : re-normalisé vers SON
    // atterrissage (dashboard métier) — miroir des 404 serveur.
    const hotspotSession = sessionValueFor(reg.token as string, reg.user as Record<string, unknown>);
    await page.addInitScript(
      ([key, value]) => {
        window.localStorage.setItem(key, value as string);
      },
      ["mikcloud-auth", hotspotSession],
    );
    await page.goto("/app/devices");
    await expect(page).toHaveURL(/\/app\/dashboard$/);
    await expect(page.getByText("Votre maison")).toHaveCount(0);

    // Et le foyer qui tape un signet métier (/app/vouchers) retombe sur SA
    // maison — la garde est générale (N°100), pas une liste de vues cachées.
    await page.addInitScript(
      ([key, value]) => {
        window.localStorage.setItem(key, value as string);
      },
      ["mikcloud-auth", familySession],
    );
    await page.goto("/app/vouchers");
    await expect(page).toHaveURL(/\/app\/home$/);
    await expect(page.getByRole("heading", { name: "Votre maison" })).toBeVisible();
  });
});
