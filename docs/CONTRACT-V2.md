# CONTRAT V2 — P0 / P1 / P2 (audit Mikhmon)

> **SOURCE DE VÉRITÉ** pour l'implémentation des 13 fonctionnalités issues de
> l'audit Mikhmon v3. Tout agent (backend comme frontend) DOIT s'y conformer
> strictement : noms de champs JSON, routes, sémantique. Voir AUDIT-MIKHMON-V3.md.

## 0. Conventions générales (inchangées)

- Toutes les routes console sous `/api/…`, auth `Authorization: Bearer <jwt>`.
- Erreurs : `{"error": "message"}` + code HTTP (400/401/403/404).
- **Sécurité S1 (durcissement pré-lancement, 2026-09-02)** :
  - JWT HS256 24 h, claims `{sub, name, role, acc, ver, iat, exp}` — `ver`
    porte l'époque de session : un token dont `ver` ≠ `SessionEpoch` de
    l'utilisateur est refusé `401` (« Session révoquée — reconnectez-vous ») ;
    un porteur supprimé du store est refusé `401` (« Compte utilisateur
    supprimé — reconnectez-vous »). Révocation immédiate sur changement/
    réinitialisation de mot de passe et changement de rôle. Les revendeurs
    (rôle `reseller`) sont hors périmètre de ce garde. Les tokens sans `ver`
    se décodent `ver=0` (compatibilité migration, tant que SessionEpoch = 0).
  - Limiteur de débit par IP (IP = premier hop XFF, posé par le proxy de
    confiance Render — suivi S1 : le dernier hop est un hop interne Render
    qui tourne et fragmentait les buckets) : `/api/auth/*` 12/min,
    `/api/reseller/login` 5/min, toute autre route `/api/*` 120/min → `429`
    + `Retry-After: 60` ; `/agent/*` et healthcheck hors périmètre.
  - Taille des corps de requête plafonnée à 2 Mio → `413` au-delà (les
    webhooks conservent leur borne propre de 1 Mio).
  - En-têtes de sécurité sur toutes les réponses : `X-Content-Type-Options:
    nosniff`, `Strict-Transport-Security: max-age=31536000; includeSubDomains`,
    `X-Frame-Options: DENY`, `Referrer-Policy: no-referrer`,
    `Cache-Control: no-store`.
- **Sécurité S2 (durcissement P1 anti brute-force, 2026-09-03)** :
  - Politique de mots de passe centralisée sur les 6 points de définition
    (inscription, `POST /api/auth/password`, `POST /api/team`,
    `PUT /api/team/{id}`, création d'un compte client plateforme, création
    d'un admin plateforme) : 10 caractères minimum (runes), 72 octets
    maximum (limite bcrypt), différent du nom d'utilisateur, denylist des
    mots de passe les plus courants → `400` avec le message correspondant.
    Les mots de passe existants plus courts restent valides à la connexion.
  - Verrouillage PIN revendeur par compte : après 5 échecs consécutifs sur
    le même revendeur, `POST /api/reseller/login` renvoie `429` +
    `Retry-After` pendant 15 minutes (clé = ID interne du revendeur,
    insensible à l'IP ; un succès efface l'historique). Réponses d'échec
    inchangées (`400` générique) — aucun oracle d'énumération.
  - Journal des échecs d'authentification : une ligne JSON
    `{"event":"auth_failure",…}` par échec (console, PIN) sur la sortie
    standard du service — horodatage, IP (premier hop XFF), kind, login
    soumis, raison fine (`unknown_user`, `bad_password`, `disabled`,
    `unknown_reseller`, `bad_pin`, `locked`, `reseller_disabled`,
    `account_disabled`) ; réponses HTTP inchangées.
- **Sécurité S3 (hygiène chaîne d'approvisionnement + anti-abus
  inscription, 2026-09-03)** :
  - Quota d'inscription par IP (`POST /api/auth/register`, cf.
    `signup_abuse.go`) : 5 tentatives par fenêtre glissante de 10 minutes
    (anti-burst) et 20 tentatives par fenêtre glissante de 24 heures
    (anti-farm) — TOUTE tentative (même invalide) consomme le quota →
    `429` + `Retry-After` (même contrat que le verrou PIN). Clé = premier
    hop XFF (forgeable en production, cf. S1) : le fermage organisé reste
    rattrapé par le plafond global d'instance (900 req/min) et, pour la
    bêta privée, par `REGISTER_KEY`. État en mémoire (instance unique),
    remis à zéro au redémarrage. N°114 — bornes configurables par
    environnement (`SIGNUP_BURST_MAX` / `SIGNUP_DAILY_MAX`, entiers > 0 ;
    valeur absente/vide/non numérique → repli franc sur 5/20) : le runner
    E2E funellise quatre suites par UNE IP et le retry d'un groupe serial
    rejoue l'inscription déjà passée (6e tentative → 429 fantôme) — la
    config Playwright pose 20/100 (bornes NAT-friendly N°50, miroir du
    pattern `RATE_API_PER_MIN` N°102). Production : env absent → S3.
  - Chaîne d'approvisionnement (côté dépôt GitHub) : job CI `govulncheck`
    (vulnérabilités Go atteignables, rapport non bloquant — job rouge =
    visibilité immédiate, le déploiement n'attend pas une base de CVE
    externe) ; Dependabot hebdomadaire (gomod `/backend`, npm
    `/frontend`, github-actions) avec groupement minor/patch ; secret
    scanning et secret push protection activés au niveau du dépôt.
- **Sécurité S4 (2FA TOTP + sauvegardes testées, 2026-09-03)** :
  - 2FA TOTP RFC 6238 (HMAC-SHA1, 30 s, 6 chiffres, fenêtre ±1,
    constant-time) : `POST /api/auth/2fa/setup` → `{secret, otpauth}`
    (secret en attente, jamais sérialisé en JSON ensuite), `/activate`
    `{code}` → active après vérification d'un premier code, `/disable`
    `{password}` → désactive en exigeant le mot de passe courant. Au
    login, si la 2FA est active : sans `code` → `401` + code machine
    `totp_required` ; code erroné → `400` générique + journal
    `auth_failure` (raison `bad_totp`). Statut exposé via `totpEnabled`
    (login + `GET /api/auth/me`). Colonnes idempotentes
    `admin_users.totp_secret` / `totp_enabled`. La 2FA n'est pas écrasée
    par une réinitialisation de mot de passe ; secours opérateur :
    RUNBOOK-SECRETS.md §4.
  - Sauvegardes chiffrées : `backend/cmd/mikbackup` (export
    row_to_json → AES-256-GCM ; `restore-check` = réinsertion complète en
    miroir `s4check_*` via json_populate_record, comptages comparés,
    nettoyage). Workflow `backup.yml` hebdomadaire : chaque export est
    suivi d'un test de restauration, l'artefact chiffré (90 j) est publié
    uniquement si la vérification passe.
- **Sécurité S5 (dédoublonnage email/WhatsApp, 2026-09-03)** :
  - Un même email ou un même numéro WhatsApp ne peut créer qu'UN SEUL
    compte — appliqué aux DEUX points de création : `POST /api/auth/register`
    (auto-inscription publique) et `POST /api/admin/accounts` (console
    plateforme). Objectif : bloquer le fermage « manuel » d'essais de 90
    jours (client sous paywall P3 qui relance un essai en changeant nom et
    username). → `409` avec message distinct (« Un compte existe déjà avec
    cet email » / « … avec ce numéro WhatsApp »), emails comparés trim +
    insensible à la casse, WhatsApp comparé en chiffres normalisés
    (E.164 sans « + », fait en amont). Comptes désactivés inclus (un client
    banni ne revient pas avec ses coordonnées) ; la suppression d'un compte
    libère ses coordonnées. Limite assumée : formes de numéro différentes
    (« 0701020304 » vs « 2250701020304 ») restent distinctes — bornées par
    le quota d'inscription par IP (S3).
- **Sécurité S6 (détection d'identité routeur dupliquée, 2026-09-03)** :
  - Anti-fermage d'essai côté PROTOCOLE AGENT : l'empreinte RouterOS
    (System Identity + board-name, normalisées trim/minuscules) déclarée au
    `POST /agent/register` est comparée aux routeurs ACTIFS (`LastSeen` <
    24 h) des AUTRES comptes. Conflit → `409` + code machine
    `router_identity_conflict` + flag persistant `routers.identity_conflict`
    (colonne idempotente). Le flag est porté par le modèle (jamais exposé à
    la console client).
  - `GET /agent/cmd` : un routeur flaggé ne reçoit AUCUNE commande
    (`409`, texte `# mikcloud: identite de routeur deja active…`) tant que
    le porteur reste actif — le fermage ne doit rien produire. Levée
    AUTOMATIQUE à chaque check-in dès que le porteur disparaît (suppression
    du routeur fantôme par le support, impersonation) ou dort (`LastSeen` >
    24 h) ; la levée est tracée dans le journal d'activité du compte.
  - Exclusions : empreintes génériques (identity vide ou « mikrotik » —
    défaut RouterOS) et MÊME compte (re-register, rotate-token, doublons
    logiques = gestion interne). Au register sans conflit, l'identity
    COURANTE écrase `Host` (un renommage RouterOS est désormais répercuté —
    avant : figé au premier register) et `BoardName` est rempli.
  - Limites assumées (documentées) : identity forgeable par qui contrôle le
    routeur (barrière contre le fermage paresseux de masse + traçabilité
    complète dans le journal d'activité) ; fenêtre 24 h = compromis contre
    le faux positif « routeur revendu » (le support débloque en supprimant
    le fantôme). Traçage : ligne d'activité « Inscription agent REFUSÉE »
    sur le compte cible + journal serveur.
- Isolation multi-tenant : toute entité portée par `accountID` ; helpers existants
  `accountScope(r)`, `findRouterScoped`, etc.
- 3 modes routeur : `simulated` | `real` | `agent`. **Matrice de support des
  nouvelles fonctionnalités** :

| Fonction | simulated | real | agent |
|---|---|---|---|
| Expiration cloud (F1) | ✅ Tick direct | ✅ via gateway (set/remove) | ✅ commandes user_set/user_remove |
| User logs (F3) | ✅ Tick | ⛔ non supporté (erreur claire) | ✅ diff applyReadState |
| Templates / marge (F2,F13) | ✅ cloud pur | ✅ | ✅ |
| Export/reset/extend/cleanup (F4,F5) | ✅ direct | ✅ reset via gateway | ✅ commandes |
| Trafic temps réel (F6) | ✅ Tick simule | ⛔ (erreur claire) | ✅ read_state v2 |
| IP bindings (F7) | ✅ cloud CRUD | ⛔ (erreur claire) | ✅ commandes |
| Status étendu + ping (F8) | ✅ simulé | ⛔ (erreur claire) | ✅ read_state v2 + cmd ping |
| DHCP/hosts/cookies/log (F9) | ✅ généré à la volée | ⛔ (erreur claire) | ✅ commandes read_* |
| Ressources routeur (pools/queues/servers) | ✅ généré à la volée | ⛔ (erreur claire) | ✅ commande read_resources |
| Scheduler/reboot/shutdown (F10) | ✅ cloud CRUD | ⛔ (erreur claire) | ✅ commandes |

> En mode `real`, répondre `400` avec message : « Non supporté en mode API directe — utilisez le mode agent ».

- CSV : séparateur `;`, BOM UTF-8 (`\ufeff`), Content-Type `text/csv; charset=utf-8`,
  header `Content-Disposition: attachment; filename="…"` (suivre le pattern de
  `handleAccountingExport`).
- **Purge des données FUSIONNÉE (zone sensible — console plateforme, 2026-09-03,
  fusion anti-redondance)** : la purge GLOBALE et la purge CIBLÉE par compte
  partagent UN SEUL moteur (`purgeScopes`) et UN SEUL endpoint d'exécution ;
  la grille de 10 catégories est IDENTIQUE dans les deux portées. Mêmes
  garanties structurelles (jamais les routeurs réels, comptes, équipe,
  réglages, abonnement, facturation ; ne régénère rien ; réaffectation de
  slices non-nil → synchro différentielle Neon) :
  - `GET /api/admin/purge/stats` → compteurs GLOBAUX (tous comptes confondus)
    `{simulatedRouters, vouchers, hotspotUsers, profiles, batches, resellers,
    transactions, sales, sessions, logs, templates, realRouters}` — les
    entités des routeurs simulés sont exclues de leur catégorie (cascade) ;
    tickets et comptes client comptés séparément.
  - `GET /api/admin/purge/accounts` → tableau
    `[{id, name, owner, status, stats:{…}}]` — mêmes compteurs PAR COMPTE
    (alimente le sélecteur de portée de l'UI) ; tri par nom.
  - `POST /api/admin/purge` `{scopes:[…], accountId?}` → `{ok, summary,
    purged}` — portée UNIFIÉE :
    - `accountId` ABSENT/VIDE → purge GLOBALE (les catégories cochées sont
      supprimées sur TOUS les comptes — comportement historique) ;
    - `accountId` RENSEIGNÉ → purge CIBLÉE (seules les données de CE compte
      sont supprimées ; les autres ne sont JAMAIS touchés ; `404` si inconnu ;
      l'ancien `POST /api/admin/purge/account` est FUSIONNÉ ici).
    Scopes : **`vouchers`** (tickets kind=voucher, LOTS conservés),
    `simulated_routers` (cascade : utilisateurs, tickets, sessions, trafic,
    commandes, bindings, schedulers, lots, ventes attachés), `hotspot_users`
    (comptes client, hors tickets), `profiles`, `batches` (+ leurs vouchers
    restants), `resellers` (+ leurs transactions), `sales`, `sessions`,
    `logs`, `templates` ; `all` = les 10 catégories. `400` scope inconnu /
    sélection absente (purge destructive : sélection explicite exigée),
    `404` compte introuvable. Ligne d'activité écrite APRÈS la purge —
    journal de l'admin en portée globale, journal du compte ciblé en portée
    ciblée (« … (par la plateforme) »). Réservé rôle admin plateforme.

---

## F1 — Expiration cloud (expmode + grâce + verrouillage) [P0]

### Modèle `Profile` (champs ajoutés)
```go
ExpMode        string `json:"expMode"`        // "none" (parité Mikhmon « None ») | "notify" (défaut) | "remove"
GracePeriodMin int    `json:"gracePeriodMin"` // 0 = immédiat
LockUser       bool   `json:"lockUser"`       // verrouiller : 1 session à la fois
```
- `POST/PUT /api/profiles` acceptent ces champs (validation : expMode ∈ {none,notify,remove},
  gracePeriodMin ∈ [0,43200]).
- Réponses GET /api/profiles : champs toujours présents.
- expMode `none` : AUCUNE expiration cloud — le voucher reste `active` jusqu'à
  épuisement du temps/data sur le routeur (`applyExpiry` le saute ; les modes
  Mikhmon « remc »/« ntfc » sont couverts par le nettoyage cloud F5).

### Moteur (fonction partagée `applyExpiry(db, now)` appelée par Tick ET avant chaque
lecture de /api/users, /api/dashboard, /api/vouchers — sous verrou)
Pour chaque compte, pour chaque voucher `status == "active"` dont
`expiresAt != ""` :
1. `expiredAt := expiresAt + gracePeriodMin minutes`
2. si `now > expiredAt` : `status = "expired"` + **UserLog** `{action:"expire"}` +
   enforcement routeur :
   - expMode `remove` → agent : queue `user_remove` (names=[username]) ;
     simulated : rien de plus (l'utilisateur cloud reste en historique) ;
     real : gateway RemoveUser.
   - expMode `notify` → agent : queue `user_set` `{oldName, disabled:true}` ;
     simulated/real : rien (le statut cloud suffit).
3. `LockUser` : si le profil du voucher a lockUser et que >1 sessions actives pour
   cet utilisateur → kick les plus anciennes (cloud + agent queue `kick`) —
   implémenté dans Tick (simulation) et applyReadState (agent : à chaque check-in,
   si un user lockUser a 2+ sessions actives → queue kick des anciennes).

### Nettoyage cloud (F5, même moteur)
`Settings.Tenant` gagne :
```go
ExpiryPolicyMode      string `json:"expiryPolicyMode"`      // "keep" (défaut) | "remove"
ExpiryPolicyAfterDays int    `json:"expiryPolicyAfterDays"` // défaut 30
```
- Si `expiryPolicyMode == "remove"` : dans `applyExpiry`, tout utilisateur
  `status == "expired"` dont `expiresAt` date de plus de `afterDays` jours est
  **supprimé du cloud** (+ Activity « Nettoyage : N utilisateurs expirés supprimés »).
- `PUT /api/settings` accepte ces champs.

---

## F2 — Éditeur de templates de vouchers [P0]

### Modèle `VoucherTemplate`
```go
type VoucherTemplate struct {
    ID        string `json:"id"`
    AccountID string `json:"accountId"`
    Name      string `json:"name"`     // 1-60 chars
    Format    string `json:"format"`   // "a4" | "58mm" | "80mm"
    BodyHTML  string `json:"bodyHtml"` // ≤ 20 000 chars
    IsDefault bool   `json:"isDefault"`
    CreatedAt string `json:"createdAt"`
}
```

### Routes (auth console)
- `GET /api/templates` → `VoucherTemplate[]` (tri : default d'abord, puis createdAt)
- `POST /api/templates` `{name, format, bodyHtml, isDefault?}` → 201 `VoucherTemplate`
  - si `isDefault` → unset les autres du compte.
- `PUT /api/templates/{id}` `{name?, format?, bodyHtml?, isDefault?}` → `VoucherTemplate`
- `DELETE /api/templates/{id}` → 200 `{ok:true}` — interdit si c'est le dernier du compte (400).

### Variables du bodyHtml (remplacées côté CLIENT à l'impression)
`{{username}} {{password}} {{profile}} {{validity}} {{price}} {{sellingPrice}}
{{dataLimit}} {{timeLimit}} {{qrCode}} {{logo}} {{hotspotName}} {{dnsName}} {{num}}
{{comment}} {{currency}}`

**Bloc conditionnel `{{#password}}…{{/password}}`** : retiré du rendu quand le
voucher est en mode « mot de passe = identifiant » (`password === username`,
parité Mikhmon — le ticket n'affiche que le code), déballé (contenu conservé)
sinon. Les gabarits hérités des presets d'origine, sans bloc, voient leur ligne
mot de passe exacte (`<p>Mot de passe : {{password}}</p>` / `<p>PASS :
{{password}}</p>`) retirée automatiquement en mode « même mot de passe » ;
un gabarit personnalisé sans bloc conserve son affichage (choix du gérant).
Le ticket standard MikCloud (hors modèle) et le A4+QR appliquent la même règle
(code seul ; QR de secours A4 = code seul).

### Settings tenant (ajouts)
```go
DNSName string `json:"dnsName,omitempty"` // ex. wifi.mondomaine.ci
LogoURL string `json:"logoUrl,omitempty"` // data URL image ≤ 300 Ko
// N°45 — bannière du portail captif : data URL image ≤ 500 Ko OU URL https
// (Cloudflare R2). Affichée en tête de la page login du portail (routeurs
// agent) et exposée par GET /api/wifi/site/{slug}/info (page visiteur).
BannerURL string `json:"bannerUrl,omitempty"`
```
- `PUT /api/settings` accepte `dnsName` (≤100 chars) et `logoUrl` (data:image/*,
  ≤ 300 Ko — sinon 400 « Logo trop volumineux (300 Ko max) »).
- `PUT /api/settings` accepte `bannerUrl` (N°45) : `data:image/*` ≤ 500 Ko
  (sinon 400 « Bannière trop volumineuse (500 Ko max) ») **ou** URL
  `https://…` (Cloudflare R2 et tout hébergeur externe — http/ftp/relative/
  javascript: sont refusés en 400, mixed content impossible sur le portail).
  Vide = bannière retirée. Le templating du portail expose le marqueur
  `{{MIKCLOUD_BANNER_URL}}` et le champ `bannerUrl` du bloc config JSON ;
  `login.html` insère l'image (id `mikcloud-banner`) en tête de la colonne
  de connexion quand la valeur est non vide (retrait auto si l'image 404).
- `PUT /api/settings` accepte `joinButton` (N°46, booléen, formes plate +
  `tenant{…}`) : bouton « S'inscrire » du portail captif. `false` = AUCUN
  bouton d'inscription sur la page de login (le reliquat Mikhmon « Scanner un
  QR Code » est retiré du DOM) ; true/absent (nil) = le bouton s'affiche
  quand un lien d'inscription publique actif est lié au routeur. Persisté
  Neon (`settings.join_button BOOLEAN NOT NULL DEFAULT TRUE`, DDL idempotent).
  Le templating du portail expose la valeur effective via le champ
  `joinEnabled` du bloc config JSON (SANS omitempty : toujours explicite) et
  `GET /api/wifi/site/{slug}/portal` le renvoie aussi — le réglage s'applique
  sans re-déploiement sur les portails déjà déployés (fetch live prime).
  Prérequis d'affichage : `APP_PUBLIC_URL` doit être défini sur Render
  (origine publique du frontend) pour que `cfg.joinUrl` soit construit.

### Seed (compte principal + tout nouveau compte)
3 templates par défaut (contenus HTML fidèles à Mikhmon, adaptés MikCloud) :
1. « Grille A4 » (format a4, défaut) — 3 colonnes, ticket pointillé, QR code,
   variables de base.
2. « Ticket thermique 58 mm » (58mm) — ticket compact 58 mm de large.
3. « Ticket thermique 80 mm » (80mm) — ticket large 80 mm.
Les 3 utilisent des styles INLINE (pas de classes Tailwind — l'impression est hors app).

---

## F3 — Journal utilisateurs (login/logout) [P0]

### Modèle `UserLog`
```go
type UserLog struct {
    ID         string `json:"id"`
    AccountID  string `json:"accountId"`
    UserID     string `json:"userId"`
    Username   string `json:"username"`
    Action     string `json:"action"` // "login" | "logout" | "expire" | "kick"
    RouterID   string `json:"routerId"`
    RouterName string `json:"routerName"`
    IP         string `json:"ip"`
    MAC        string `json:"mac"`
    At         string `json:"at"`
}
```
- Captures : Tick (session créée → login ; session terminée → logout ; kick existant → kick ;
  expiry engine → expire) ; applyReadState agent (diff sessions avant/après → login/logout,
  en comparant par username ; IP depuis l'entrée session).
- Rétention 90 jours (purge dans Tick).

### Routes
- `GET /api/user-logs?search=&routerId=&action=&page=&pageSize=` →
  `{ "data": UserLog[], "total": number, "page": number, "pageSize": number }`
  (pageSize ≤ 100, défaut 20 ; tri At desc ; search sur username/IP).
- `GET /api/user-logs/export?search=&routerId=&action=` → CSV download
  (colonnes : Date;Utilisateur;Action;Routeur;IP;MAC).

---

## F4 — Actions utilisateurs : reset stats / prolonger / exporter / nettoyer [P0]

### Statuts résolus (5 états priorisés) — listes, export, donut dashboard, stats par lot

Le statut RENVOYÉ et FILTRÉ par `GET /api/users` / `GET /api/vouchers` / export CSV est le
statut résolu (model.ResolvedStatus), par priorité décroissante :

1. `expired` — voucher : validité (`expiresAt`) dépassée **OU** quota temps épuisé
   (`uptimeUsedSec >= timeLimitMin` quand `timeLimitMin > 0`) — calculé, gagne sur tout ;
   `expiresAt` vide = voucher jamais connecté (ancrage au 1er login) → pas d'échéance par date ;
2. `disabled` — désactivation manuelle (statut stocké) ;
3. `online` — session live au dernier read_state (≤ 45 s de latence) ; garde : seules les
   sessions des routeurs vus depuis < 3 min sont prises en compte (pas de « en ligne » figé) ;
4. `used` — déjà connecté au moins une fois, hors ligne ;
5. `active` — jamais connecté (disponible).

- Persistance dynamique (mode agent) : au 1er login détecté (diff sessions), le voucher passe
  `status="used"` + `usedAt` horodaté ; à chaque logout détecté, l'uptime de session s'ajoute à
  `uptimeUsedSec` (le routeur applique lui-même la coupure limit-uptime ; le cloud reflète).
- Ancrage de la validité au 1er login (variante opérateur, `model.AnchorVoucherValidity`) :
  `expiresAt` est posé au PREMIER LOGIN = login + `ValidityMinutes()` du profil courant (agent
  ET session simulée) ; à la génération (unitaire et par lot) il reste vide — un ticket jamais
  connecté reste « actif » en stock indéfiniment. Changement de profil : voucher jamais
  connecté → `expiresAt` reste vide (la nouvelle validité s'appliquera au 1er login) ; voucher
  connecté → recalcul depuis maintenant (inchangé). `extend` (F4) sur un voucher jamais
  connecté → 400 explicite en unitaire, no-op en bulk : la validité du stock se règle via le
  profil. Parité routeur : le profil est lu à l'authentification (comportement MikroTik).
- Les agrégats (donut dashboard, stats par lot) replient `online` dans `used` (en ligne = consommé
  en cours) — les buckets restent active/used/expired/disabled.
- La liste ajoute `disabled` (booléen, miroir du statut stocké) : le badge peut afficher
  « expiré » (priorité 1) tandis que le toggle activer/désactiver s'y réfère.
- `EffectiveStatus` (vente, compteurs « disponibles ») : renvoie aussi `expired` pour un voucher
  utilisé dont la validité/quota est épuisé, et `used` pour un voucher réactivé après connexion.
- `reset-stats` (unitaire ET bulk, toutes branches) : remet aussi `usedAt=""` et `used → active`
  (retour « jamais connecté ») en plus des compteurs.

- `POST /api/users/{id}/reset-stats` → `{ok:true}` : met à zéro bytesIn/bytesOut/uptimeUsedSec
  (cloud) + agent : queue nouvelle commande `user_reset` `{name}` (script :
  `/ip hotspot user reset-counters [find name=…]`) ; simulated : direct ; real : gateway Run.

> **Sémantique des compteurs de trafic (verrouillée)** — RouterOS compte du point de vue du
> ROUTEUR (doc officielle help.mikrotik.com — HotSpot) : `bytesIn` (`bytes-in`) = bytes
> **uploadés** par le client ; `bytesOut` (`bytes-out`) = bytes **téléchargés** par le client.
> Le backend transporte ces compteurs BRUTS (somme invariante pour les quotas) ; l'étiquetage
> client (upload/download) est interdit hors du module front
> `frontend/src/lib/hotspot/traffic-semantics.ts` (`upBytes`/`downBytes`). Le simulateur
> (store tick sessions) respecte la même réalité : download ≫ upload.
- `POST /api/users/{id}/extend` `{days: number ≥ 1 ≤ 3650}` → `HotspotUser`
  - nouvelle `expiresAt = max(now, expiresAt) + days` ;
  - si le statut était `expired` → repasse `active` + agent `user_set {disabled:false}` ;
  - Activity « Utilisateur X prolongé de N j ».
- `GET /api/users/export?search=&status=&routerId=&kind=&profileId=` → CSV
  (colonnes : Utilisateur;Mot de passe;Profil;Statut;Routeur;Créé le;Expire le;Upload (Mo);Download (Mo);Prix;Revendeur;Commentaire).
  Utilise le MÊME filtrage que handleUsersList.
- `POST /api/users/cleanup` `{mode:"expired"}` → `{ok:true, removed:number}` — supprime
  du cloud TOUS les utilisateurs `expired` du compte (+ Activity). Mode réel/agent :
  queue `user_remove` avec la liste des noms (≤ 50 par commande, plusieurs commandes si besoin).

---

## F6 — Moniteur de trafic temps réel [P1]

### Modèle (nouvelles collections DB)
```go
type IfaceTraffic struct {
    Name    string `json:"name"`
    RxBytes int64  `json:"rxBytes"` // compteurs cumulés
    TxBytes int64  `json:"txBytes"`
    RxBps   int64  `json:"rxBps"`   // débit calculé
    TxBps   int64  `json:"txBps"`
}
type RouterTraffic struct {
    RouterID   string         `json:"routerId"`
    AccountID  string         `json:"accountId"`
    UpdatedAt  string         `json:"updatedAt"`
    Interfaces []IfaceTraffic `json:"interfaces"`
    History    []TrafficPoint `json:"history"` // 60 derniers points, t par interface ? NON :
}
```
Historique simple (somme toutes interfaces) + détail par interface courant :
```go
type TrafficPoint struct {
    T     string `json:"t"`     // RFC3339
    RxBps int64  `json:"rxBps"` // somme interfaces
    TxBps int64  `json:"txBps"`
}
```

- **Simulated** : Tick maintient 3 interfaces par routeur simulé (`ether1`, `wlan1`,
  `hotspot`) avec marche aléatoire réaliste (0,5–50 Mbps) + ajoute un TrafficPoint
  toutes les ~5 s (cap 60).
- **Agent** : `read_state` v2 rapporte en plus `ifaces=name:rx:tx;…` (compteurs
  cumulés `/interface print` → `rx-byte`,`tx-byte`). applyReadState : diff avec les
  compteurs précédents (delta temps vs delta octets) → RxBps/TxBps + point historique
  (cap 60). Interfaces : max 8, filtrer `running`.
- **real** : 400 « Non supporté en mode API directe ».

### Route
- `GET /api/routers/{id}/traffic` → `RouterTraffic` (ou `{routerId, interfaces: [], history: []}` si absent).

---

## F7 — IP Bindings [P1]

### Modèle `IPBinding`
```go
type IPBinding struct {
    ID        string `json:"id"`
    AccountID string `json:"accountId"`
    RouterID  string `json:"routerId"`
    MAC       string `json:"mac"`      // "AA:BB:CC:DD:EE:FF"
    Address   string `json:"address"`  // IP optionnelle
    Comment   string `json:"comment"`
    Type      string `json:"type"`     // "bypassed" | "blocked"
    Disabled  bool   `json:"disabled"`
    CreatedAt string `json:"createdAt"`
}
```

### Routes
- `GET /api/routers/{id}/ipbindings` → `IPBinding[]`
- `POST /api/routers/{id}/ipbindings` `{mac, address?, comment?, type?}` (défaut type
  bypassed ; MAC validée regex `^([0-9A-Fa-f]{2}:){5}[0-9A-Fa-f]{2}$`) → 201
- `PUT /api/ipbindings/{id}` `{disabled?, comment?, address?}` → `IPBinding`
- `DELETE /api/ipbindings/{id}` → `{ok:true}`
- Agent : à la création → commande `ipbinding_add {mac,address,comment,type}` ;
  update → `ipbinding_set {mac, disabled?|address?}` ; delete → `ipbinding_remove {mac}`.
  Scripts RouterOS : `/ip hotspot ip-binding add|set|remove` sur `[find mac-address=…]`.
- Simulated : CRUD cloud pur. Seed : 2 bypassed + 1 blocked sur chaque routeur simulé.

---

## F8 — Status étendu + ping [P1]

### Modèle `Router` (ajouts)
```go
BoardName  string `json:"boardName,omitempty"`
FreeHddMb  int    `json:"freeHddMb,omitempty"`
TotalHddMb int    `json:"totalHddMb,omitempty"`
```
- Agent : read_state v2 rapporte `board`, `freehdd`, `totalhdd` (Mo).
- Simulated : valeurs plausibles au seed (« RB2011UiAS», 4000/… par routeur).

### Route ping
- `POST /api/routers/{id}/ping` `{target: string}` (IP ou hostname ≤ 253 chars) →
  - simulated : réponse immédiate
    `{queued:false, ok:true, target, sent:4, received:4, lossPct:0, minMs, avgMs, maxMs}`
    (valeurs aléatoires plausibles ; 10 % de perte aléatoire) ;
  - agent : `{queued:true, commandId}` — commande `ping {target}` (script
    `/ping address=… count=4 as-value` → rapport sent/received/min/avg/max) ;
  - real : 400 non supporté.
- `GET /api/commands/{id}` (NOUVELLE route générique, auth console, scopée compte) →
  `{id, kind, status, result}` — le front poll toutes les 2 s.

---

## F9 — DHCP leases / Hôtes / Cookies / Journal routeur [P1]

Modèle de réponse commun :
```json
{ "queued": false, "data": [ … ], "updatedAt": "…" }
```
- `queued:true` tant que la commande agent n'est pas `done` (le front re-poll).
- **Simulated** : données générées à la volée (déterministes par routeur : seed du
  rand sur routerID — 5-15 baux DHCP, 8-20 hôtes, 0-4 cookies, 20 lignes de log),
  `queued:false` immédiat.
- **Agent** : commandes `read_dhcp`, `read_hosts`, `read_cookies`, `read_log` →
  le résultat est mis en cache dans `Command.Result` (champ `data` = JSON string) ;
  si une commande du même kind est `done` depuis < 120 s → renvoyer le cache, sinon
  en filer une nouvelle.

### Routes + formes des lignes
- `GET /api/routers/{id}/dhcp` → data: `[{ip, mac, host, expires, status}]`
- `GET /api/routers/{id}/hosts` → data: `[{mac, ip, server, uptime, authorized}]`
  (`authorized` boolean, uptime en secondes)
- `GET /api/routers/{id}/cookies` → data: `[{user, mac, expires}]`
- `GET /api/routers/{id}/log` → data: `[{time, topics, message}]` (50 dernières lignes hotspot)
- `GET /api/routers/{id}/resources` → data: `[{kind, name}]` avec kind ∈ {pool, queue, server}
  (parité Mikhmon : alimente Address Pool / Parent Queue / Server des formulaires).
  Commande `read_resources` (idem cache 120 s, inscrite dans staleSentReadKinds) :
  pools `/ip pool`, queues simple NON dynamiques, serveurs `/ip hotspot` — cap 60
  entrées par kind, rapport `kind|name;`.

Scripts RouterOS (dans les builders) — chaque entrée séparée par `|`, champs par `:`,
liste par `;` (même mécanique que users/sessions existant) :
- dhcp : `/ip dhcp-server lease print` sans paging → mac|address|host|expires-after|status
- hosts : `/ip hotspot host print` → mac|address|server|uptime|authorized? (bypassed→true)
- cookies : `/ip hotspot cookie print` → user|mac-address|expires-in
- log : `/log print where topics~"hotspot"` → time|topics|message (échapper | et ; via
  substitution en `_` côté script si besoin — TOLÉRANCE : le parseur remplace | et ; restants).

---

## F10 — Scheduler + reboot/shutdown [P1]

### Modèle `SchedulerTask` (persisté, source cloud)
```go
type SchedulerTask struct {
    ID        string `json:"id"`
    AccountID string `json:"accountId"`
    RouterID  string `json:"routerId"`
    Name      string `json:"name"`
    Interval  string `json:"interval"`  // affichage humain ex. "45s", "1h" (à la RouterOS)
    OnEvent   string `json:"onEvent"`
    Disabled  bool   `json:"disabled"`
    CreatedAt string `json:"createdAt"`
}
```

### Routes
- `GET /api/routers/{id}/scheduler` →
  - simulated : `SchedulerTask[]` depuis la DB ;
  - agent : même enveloppe `{queued, data, updatedAt}` que F9 avec `data` =
    `[{name, interval, onEvent, disabled}]` (commande `read_scheduler`) ;
  - **UNIFICATION** : la réponse est TOUJOURS `{queued:boolean, data:[…], updatedAt}` —
    simulated → queued:false + données DB. (Le front gère les deux cas.)
- `POST /api/routers/{id}/scheduler` `{name, interval, onEvent}` → crée (simulated :
  DB + 201 ; agent : commande `scheduler_add` → `{queued:true}`). Validation :
  name ≤ 48 chars sans espaces, interval format RouterOS (`^\d+[smhdw]$`).
- `POST /api/scheduler/{taskId}/toggle` → bascule disabled (simulated : DB ;
  agent : commande `scheduler_set {name, disabled}` — taskId = name pour agent ? NON :
  pour agent, l'UI liste les tâches ROUTEUR (nom), le toggle envoie
  `POST /api/routers/{id}/scheduler-toggle {name, disabled}`).
- `POST /api/routers/{id}/scheduler-remove` `{name}` (agent + simulated par nom).
- `POST /api/routers/{id}/reboot` → simulated : uptimeSec=0, sessions du routeur
  supprimées, Activity « Redémarrage… » ; agent : commande `reboot` (script
  `/system reboot` — rapport AVANT l'exécution via on-error… NON : rapporter
  immédiatement ok puis exécuter) ; réponse `{ok:true}` (ou `{queued:true}` agent).
- `POST /api/routers/{id}/shutdown` → idem (`/system shutdown`).
- Seed simulated : tâches `mikcloud-agent` (45s), `daily-backup` (1d) par routeur simulé.

---

## N°115 — Mise à jour RouterOS depuis la console [P1]

**Retour utilisateur** : « je souhaite donner la possibilité à mes clients de
mettre à jour leurs routeurs vers la dernière version RouterOS depuis
MikCloud ». Parité Mikhmon « Update RouterOS » — le gérant garde son parc à
jour sans Winbox. La VÉRIFICATION interroge les serveurs MikroTik DEPUIS le
routeur (`/system package update check-for-updates` : le canal du routeur —
stable par défaut — fait foi, pas une version codée en dur côté cloud) ;
l'INSTALLATION télécharge, installe puis REDÉMARRE (sémantique v7 de
`/system package update install`). Parc concerné : agents RouterOS ≥ 7.19
(garde TLS existante de `/agent/cmd` — un routeur plus ancien ne reçoit
AUCUNE commande, la mise à jour doit se faire une première fois en Winbox ;
version inconnue → tolérée le temps du premier read_state).

### Routes
- `POST /api/routers/{id}/routeros-check` →
  - simulated : réponse immédiate `{queued:false, ok, state, status,
    latestVersion, installedVersion, channel}` — déterministe (la version
    posée à la création est toujours en retard sur la dernière stable
    simulée ; après update → `latest`) ;
  - agent : DÉDUP (une vérification à la fois — le second clic récupère la
    commande EN COURS, pas d'accumulation) + commande `routeros_check` en
    file → `{queued:true, commandId, message}` ; le front poll
    `GET /api/commands/{id}` (pattern ping F8, 2 s / 90 s max — le
    check-in vient toutes les 45 s et le script routeur patiente lui-même
    jusqu'à ~30 s que MikroTik réponde : boucle `:while` sur le status
    « Checking… », garde `[:typeof] = "num"` du find-qui-ne-trouve-pas).
- `POST /api/routers/{id}/routeros-update` `{latest?}` (corps OPTIONNEL —
  `decodeBodyTolerant` : un POST nu ne doit pas échouer) →
  - simulated : application immédiate (version posée, uptime à zéro,
    sessions coupées et journalisées logout — miroir exact du reboot F10) ;
  - agent : DÉDUP STRICTE (jamais deux installations en parallèle — le
    second clic récupère la commande en vol avec `already:true`) + commande
    `routeros_update` en file ; le rapport ok part AVANT l'exécution
    (pattern reboot F10 : le téléchargement — minutes — puis le
    redémarrage coupent le routeur, le `/tool fetch` bloquant termine le
    premier) ; un échec de téléchargement est rapporté après coup
    (`:do on-error` — le routeur ne redémarre pas dans ce cas).
- Validation : `latest` optionnel, `^[0-9][0-9A-Za-z.\-]{0,31}$` (défense
  en profondeur — la valeur est embarquée dans le script .rsc du rapport de
  lancement ET assainie côté générateur `sanitizeRouterOSVersion` : premier
  caractère chiffre, `[0-9A-Za-z.-]`, coupe au premier caractère étranger,
  ≤ 32).

### Normalisation du rapport (`normalizeRouterOSCheck`)
Le rapport agent arrive en valeurs formulaire (`rosStatus` = le status
RouterOS BRUT — la clé `status` est celle du protocole ok/error du rapport,
jamais réutilisée). Le front attend `state` + `status` +
`latestVersion`/`installedVersion`/`channel`. Dérivation de l'état, par
ordre : « up to date » → `latest` ; « new version » → `available` ;
« error » → `error` ; REPLI sur la comparaison `installed != latest`
(un libellé de build inconnu ne doit pas masquer une mise à jour évidente) ;
sinon `unknown` — le status brut est préservé et affiché honnêtement (borné
à 160 chars : un firmware exotique ne gonfle pas l'historique).

### Confirmation de version — zéro mécanique dédiée
La version finale revient d'elle-même : le `read_state` de fraîcheur
re-enfilé après le rapport ok rapporte la NOUVELLE version
post-redémarrage, et `applyReadState` trace le changement (journal
« RouterOS de «X» mis à jour : A → B » — couvre AUSSI une mise à jour posée
à la main en Winbox). Le lancement lui-même est journalisé au rapport de la
commande (« Mise à jour RouterOS lancée sur «X» vers Y — téléchargement
puis redémarrage, le portail coupe pendant l'opération »). Aucune nouvelle
colonne : `Router.Version` (read_state) reste la vérité. Le check en revanche
ne journalise RIEN (lecture d'outil : chaque clic « vérifier » en produirait
une ligne de bruit) et n'enfile aucun read_state (aucune écriture).

### Zombie & convergence
`routeros_check` rejoint `staleSentReadKinds` (re-exécution sans effet de
bord — le check est idempotent). `routeros_update` N'EN FAIT PAS partie :
une écriture « sent » muette n'est JAMAIS rejouée automatiquement (un
redémarrage peut être en cours — le double lancement est précisément ce que
la dédup stricte interdit) ; la trace vit dans le journal et l'historique
des commandes.

### Front — carte « Mise à jour RouterOS » (onglet Système, sous les infos)
`ros-update-card.tsx` : version installée + bouton « Vérifier les mises à
jour » → panneau d'état (à jour = vert, disponible = ambre avec
`installé → dispo [canal]` + status brut en italique + bouton « Mettre à
jour vers X », erreur = rouge, inconnu = neutre honnête) → AlertDialog de
confirmation (avertissement FORT : coupure totale 2 à 5 min, sessions Wi-Fi
coupées, note agent ≤ 45 s) → panneau d'installation piloté SANS état
dérivé : la fiche routeur est « vivante » (poll 15 s), le panneau reste
« Installation en cours… » tant que la version n'a pas atteint la cible
(ou changé depuis une base connue — une base INCONNUE ne conclut jamais
sur une version ≠ cible : l'arrivée d'une version périmée pendant le
téléchargement ne doit pas faire passer le panneau pour terminé), puis
bascule « Mise à jour installée A → B ». Une vérification fraîche remplace
le panneau d'installation (le check est l'action la plus récente du gérant
et porte la vérité du serveur). i18n : 27 clés `tools.ros.*` FR/EN.
Mode `real` : carte désactivée (note standard `tools.realNote`).

---

## N°117 — Mise à jour RouterOS de FLOTTE : le super-admin pilote le parc de TOUS les clients [P1]

**Retour utilisateur** : « ajouter une fonctionnalité pour le super admin
afin que celui-ci, depuis la console, lance une mise à jour du parc de
routeurs de tous les clients MikCloud ». Suite multi-comptes du N°115 (le
gérant met à jour SON routeur) : la vue « Parc routeurs » de la console
plateforme liste chaque routeur de chaque compte client — version installée,
version disponible détectée, état — puis deux gestes de flotte.

**SÉCURITÉ — jamais à l'aveugle** : un update RouterOS REDÉMARRE le routeur
et coupe le hotspot du client. La cible par défaut de « Mettre à jour le
parc » n'est PAS « tous les routeurs » mais « tous les routeurs avec une
mise à jour DÉTECTÉE » (état `available` du dernier check abouti) ; la
barrière s'applique AUSSI en ciblage explicite (un routeur à jour ou jamais
vérifié n'est jamais re-redémarré pour rien) ; la confirmation du front
affiche le compte EXACT (nombre de routeurs, nombre de comptes touchés)
avant le geste ; dédup stricte par routeur (jamais deux installations en
parallèle — pattern N°115).

### Routes (super-admin : `requireRole(3)` + `isPlatformAdmin`)
- `GET /api/admin/fleet/routers` → le parc complet, tous comptes confondus :
  par routeur, compte/nom/mode/statut/version/lastSeen + état RouterOS
  DÉRIVÉ (`rosState`, `rosLatest`, `rosStatus` borné 160, `checkedAt`,
  `checking`, `updating`, `updateError`) + `summary` (total/agent/
  simulated/real/online/checking/updating/latest/available).
- `POST /api/admin/fleet/routeros-check` `{routerIds?}` → enfile un
  `routeros_check` sur chaque routeur AGENT ciblé (liste explicite = bouton
  par routeur ; absent = TOUT le parc). Lecture seule. Dédup par routeur
  (un check en vol n'est pas re-enfilé). Simulés : rien à enfiler (état
  calculé à la volée au GET), `real` : non supporté (matrice §0).
- `POST /api/admin/fleet/routeros-update` `{routerIds?, latest?}` (corps
  OPTIONNEL — `decodeBodyTolerant`) → agents : commande `routeros_update`
  avec la cible du dernier check (`latest` du corps en repli, validé
  `^[0-9][0-9A-Za-z.\-]{0,31}$`) ; simulés : application immédiate (miroir
  du chemin simulated N°115 — version, uptime à zéro, sessions coupées et
  journalisées logout). Réponse `{queued, applied, skipped, message}`.

### Dérivation d'état — ZÉRO nouveau schéma
`fleetRouterOSStateOf` parcourt l'historique des commandes du routeur : la
dernière `routeros_check` aboutie porte l'état normalisé
(`normalizeRouterOSCheck`, N°115) dans son `Result` ; les commandes en vol
portent `checking`/`updating`. Les routeurs SIMULÉS n'ont pas d'agent : état
calculé à la volée (version posée vs dernière stable simulée — miroir du
chemin simulated N°115). La version installée reste `Router.Version`
(read_state) ; la version finale revient d'elle-même au premier read_state
post-redémarrage (mécanique N°115 inchangée).

### Journal — une entrée par COMPTE, pas par routeur
Un geste de flotte ne doit pas inonder le journal : `logActivityBy` pose UNE
entrée par compte client concerné (« Mise à jour RouterOS de flotte lancée
par la plateforme : «A», «B», «C» + N autres en file d'installation… » —
noms bornés à 3, acteur = le super-admin). Les commandes sont enfilées sous
le COMPTE CLIENT du routeur (`queueCommandLocked acc = rr.AccountID`) : le
gérant concerné voit l'opération dans SON journal, le rapport agent remonte
par le chemin standard N°115.

### Front — vue « Parc routeurs » (console plateforme)
`platform-fleet-view.tsx` : 4 KPI (total, en ligne, mises à jour disponibles
en ambre, installations en cours) + gestes (« Vérifier tout le parc »
outline, « Mettre à jour le parc (N) » — désactivé à 0 avec note
pédagogique) + la liste du parc (compte, routeur, badges mode/ligne,
`installé → dispo` mono, badge d'état + dernière vérification `timeAgo`,
actions par routeur : check + update) — `max-h-[32rem] overflow-y-auto`
(règle maison des longues listes), poll 10 s pendant les vols sinon 30 s
(forme fonctionnelle — pas de fermeture sur `data`), AlertDialog de
confirmation forte (nombre exact, comptes touchés, coupure 2 à 5 min par
routeur, jamais deux fois le même). Câblage : ViewId `platformFleet`,
slug `/app/platform-fleet`, `PLATFORM_VIEWS` (garde rôle), nav plateforme
(après « Vue d'ensemble »), `viewTitle`, dynamic import. i18n : clés
`platform.fleet.*` + `nav.platformFleet` FR/EN. Mode `real` : ligne sans
action (note standard).

---

## N°125 — Firmware RouterBOARD : « le bootloader qui attendait son redémarrage » [P1]

**Retour utilisateur** : « tout le parc est à la dernière version 7.24.4.
Cependant j'ai remarqué depuis Winbox que la mise à jour du routeur ne
change pas automatiquement le firmware routeurboard. » Comportement
RouterOS PAR DÉFAUT, pas un bug : le firmware RouterBOARD (bootloader) est
un monde SÉPARÉ du RouterOS — il ne s'applique qu'au REDÉMARRAGE et
seulement si `auto-upgrade=yes` (désactivé d'usine). Un parc mis à jour
via le N°115/N°117 (ou Winbox) se retrouve donc « RouterOS à jour, firmware
en attente » : Winbox le montre dans *System → Routerboard*
(`current-firmware` ≠ `upgrade-firmware`).

### Modèle

- **AUCUNE nouvelle colonne** : l'état firmware voyage dans les RÉSULTATS
  de commandes (`routeros_check` le révèle, `routerboard_firmware`
  l'applique) — la version RouterOS reste dans `Router.Version`
  (read_state), le firmware n'a pas besoin d'être télémétré toutes les
  ~2 min pour un geste rare ;
- **CHECK ÉTENDU** — `buildRouterOSCheck` lit en plus
  `/system routerboard get current-firmware / upgrade-firmware /
  auto-upgrade`, chaque lecture dans son `:do on-error` (un CHR ou un
  vieux build sans `/system routerboard` ne tue pas la commande : les
  champs restent vides, le cloud n'expose pas la ligne). Le rapport
  dynamique F8 emporte `fwCurrent/fwStaged/fwAuto` ; la normalisation
  expose `firmwareCurrent/firmwareStaged` (bornés 32) et `firmwareAuto`
  (booléen) — absents si le routeur n'a rien rapporté ;
- **UPDATE AUTO-SYNC** — `buildRouterOSUpdate` pose
  `/system routerboard settings set auto-upgrade=yes` AVANT
  `/system package update install` (isolé on-error) : le MÊME redémarrage
  applique RouterOS ET firmware — les prochaines mises à jour ne laissent
  plus le bootloader en attente ;
- **COMMANDE `routerboard_firmware`** (kind N°125) — applique le firmware
  EN ATTENTE sur un parc déjà à jour côté RouterOS. GARDE CÔTÉ ROUTEUR
  (le cloud ne se fie jamais au front) : le script relit
  current/upgrade-firmware et ne redémarre QUE si un firmware attend
  réellement (`fwStg != "" && fwStg != fwCur`) — sinon rapport ok
  `applied=false` SANS coupure (une coupure de 2 à 5 min doit avoir une
  raison). Vrai appliquage : auto-upgrade posé, staging
  `/system routerboard upgrade`, rapport ok AVANT `/system reboot`
  (pattern reboot F10 — le fetch bloquant termine premier), reboot.
  L'échec du staging est rapporté en erreur (le routeur ne redémarre pas).

### API

```
POST /api/routers/{id}/routerboard-firmware   (rôle 2)
  simulated → {ok:true, already:true, version}   — le firmware simulé suit
              toujours le RouterOS (auto-upgrade simulé) : rien à appliquer
  agent     → {queued:true, commandId, message}  — dédup CROISÉE stricte :
              jamais en parallèle d'un routeros_update NI d'un autre
              routerboard_firmware (le second clic récupère la commande en
              vol, `already:true`)
```

Rapport (`handleAgentResult`) : `applied=true` → journal « Firmware
RouterBOARD lancé sur «X» : A → B — redémarrage (2 à 5 min), la version
RouterOS ne change pas » ; `applied=false` → « déjà synchronisé — aucun
redémarrage nécessaire ». `queueReadStateFreshLocked` (fraîcheur
post-écriture) : le read_state post-redémarrage ramène l'uptime — la
version RouterOS ne change PAS, c'est l'uptime qui prouve le retour.

### Front

- **Carte « Mise à jour RouterOS »** (onglet Système) : le panneau de
  vérification affiche la ligne firmware dans CHAQUE état (icône puce) —
  en attente (`7.24.2 → 7.24.4, appliqué au redémarrage du routeur`) avec
  le bouton **« Appliquer le firmware »** quand le RouterOS est À JOUR
  (état `latest` : le geste n'entre pas en concurrence avec une mise à
  jour RouterOS qui l'emporterait de toute façon), ou la note « Sera
  appliqué par la mise à jour RouterOS » en état `available` (un seul
  redémarrage pour les deux). Synchronisé → « Firmware RouteBOARD X · à
  jour » discret. Absent (CHR) → pas de ligne ;
- **AlertDialog forte** : « Le firmware en attente A → B sera appliqué et
  le routeur REDÉMARRERA (la version RouterOS ne change pas) » + coupure
  totale 2 à 5 min + note agent ≤ 45 s ;
- **Panneau vivant** : « Application du firmware… » tant que la fiche
  (poll 15 s) ne voit pas l'uptime RETOMBER (retour du routeur — miroir
  InstallPanel, mais la version ne bouge pas : c'est l'uptime qui
  bascule) → « Firmware appliqué · A → B en place, routeur redémarré ».
  Une base d'uptime NULLE au lancement ne conclut jamais (panneau
  honnête, une vérification fraîche le remplace). Poll bref de la
  commande (95 s) : un échec de staging remonte en toast au lieu de
  laisser le panneau « en cours » sans raison ;
- **Vue flotte (N°117)** : ligne firmware par routeur (dérivée du dernier
  check abouti) — `fwCurrent → fwStaged` en ambre si en attente ; simulés :
  toujours synchronisés. 18 clés i18n `tools.ros.fw*` FR/EN + 1 clé flotte.

### Tests

Agent : `TestRouterOSCheckScriptShape` (lectures firmware isolées ≥ 7,
rapport étendu), `TestRouterOSUpdateScriptShape` (auto-upgrade AVANT
l'install), `TestRouterboardFirmwareScriptShape` (garde anti-redémarrage
inutile, ordre garde → staging → rapport → reboot, échec rapporté).
API : `TestRouterOSCheckSimulatedFirmware`, `TestRouterboardFirmwareAgentFlow`
(file + dédups + rapport brut routeur + journaux + read_state re-enfilé),
`TestRouterboardFirmwareSimulated`, `TestNormalizeRouterOSCheckFirmware`
(bornage, booléen, absence CHR tolérée).

---

## N°97 — Docteur du pool d'adresses IP du hotspot [P1]

### Contexte
Épuisement du pool IP aux heures de pointe : le portail affiche
`cannot assign ip address - no more free addresses from pool` aux CLIENTS
PAYANTS. Causes : pool /24 trop petit (254 IP partagées avec les appareils
non connectés qui reçoivent une IP AVANT login), hôtes « zombies » conservés
indéfiniment (login-timeout absent par défaut), address-per-mac=2 par défaut.

### Route
- `POST /api/routers/{id}/pool-doctor` (rôle ≥ manager, garde compte
  expiré) — corps optionnel `{ "extend": true|false }` (défaut false) :
  - simulated : diagnostic synthétique (PoolCap=254, PoolHosts=sessions+40 %,
    PoolRanges, PoolDoctorAt ; extend=true applique VRAIMENT le geste —
    capacité + ~2 037 adresses, même arithmétique que le rapport agent)
    → `{ok, poolCap, poolHosts, usagePct, message}` ;
  - agent : enfile la commande `pool_doctor` avec
    `{recycle: true, extend}` → `{queued, commandId, message}` — le
    recyclage est TOUJOURS inclus (aucun subnet touché), l'extension est
    l'opt-in explicite ;
  - real : 400 (matrice §0).

### Commande agent `pool_doctor` (idempotente, script .rsc) — N°108
**Correctif N°108 (constat production ProMax WIFI — l'extension « ne
fonctionne pas »)** : `address-pool` et `addresses-per-mac` sont des
propriétés du **SERVEUR** hotspot (`/ip hotspot`) — le menu profil
(`/ip hotspot profile`) ne les a JAMAIS eues. Le script N°97 lisait/écrivait
les deux sur le profil : chaque `get` échouait silencieusement (on-error →
chaîne vide → contresens « profil sans pool ») et le `set` de l'extension
échouait à chaque fois — le pool n'a JAMAIS été étendu (seuls l'IP
secondaire, l'entrée network et le NAT étaient posés, autour d'un pool
orphelin `mikcloud-pool` référencé par personne).
- DIAGNOSTIC (toujours) : rapporte `pools=nom|ranges;…`,
  `servers=n|profil|interface|login-timeout|idle-timeout|keepalive|address-pool|addresses-per-mac;…`
  (8 champs — les deux derniers RELUS sur le serveur : la vérité de ce qui
  a VRAIMENT collé), `dhcp=n|interface|pool|lease-time;…` (4 champs),
  `hosts`, `active` — la liste `profiles` du N°97 est retirée (propriétés
  inexistantes sur ce menu) ; le cloud en tire PoolCap (capacité des pools
  RÉFÉRENCÉS : pool du SERVEUR hotspot **OU** pool du DHCP posé sur son
  interface — cas ProMax WIFI « DHCP du bridge » ; formats `a-b` et CIDR,
  dédoublonnés), PoolHosts, PoolRanges, PoolDoctorAt. Tolérant aux rapports
  pré-N°108 (serveurs à 6 champs : la branche DHCP suffit) ;
- RECYCLAGE (payload recycle) : trois écrits ISOLÉS sur les bons menus —
  `/ip hotspot set [find] login-timeout=5m idle-timeout=10m
  keepalive-timeout=2m`, `/ip hotspot set [find] addresses-per-mac=1`,
  `/ip dhcp-server set [find where interface=<if hotspot>]
  lease-time=10m` (un bail long brûle l'IP d'un appareil parti pendant des
  heures — l'isolation par bloc laisse un RouterOS ancien refuser
  `addresses-per-mac` sans faire échouer les timeouts) ;
- EXTENSION (payload extend) : par serveur hotspot — (a) le serveur a un
  `address-pool` → le range dédié `10.77.0.10-10.77.7.254` est AJOUTÉ à CE
  pool ; (b) sinon, le serveur DHCP de la même interface a un pool → le
  range est ajouté au pool **DU DHCP** (la capacité vient de là, cas
  ProMax WIFI) ; (c) sinon, pool dédié `mikcloud-pool` posé **SUR LE
  SERVEUR** (`/ip hotspot set <id> address-pool=mikcloud-pool`). Dans tous
  les cas : IP secondaire `10.77.0.1/21` sur l'interface hotspot, entrée
  `/ip hotspot network add address=10.77.0.0/21 masquerade=yes`, entrée
  `/ip dhcp-server network add address=10.77.0.0/21 gateway=10.77.0.1`
  (sans elle, le DHCP n'offre pas proprement le nouveau range) et règle NAT
  `mikcloud-pool-nat` (chain=srcnat src-address=10.77.0.0/21
  action=masquerade) — tout marqué/idempotent, clients connectés non
  déconnectés. Ménage inclus : le pool orphelin `mikcloud-pool` laissé par
  le N°97 est retiré s'il n'est référencé par aucun serveur hotspot ni
  DHCP (garde double avant `/ip pool remove`).
- Convergence au démarrage cloud : `UPDATE routers SET pool_doctor_at=''
  WHERE mode='agent' AND pool_doctor_at<>''` (lecture seule, une commande
  par routeur par démarrage — la sémantique du rapport a changé, la
  capacité doit être re-mesurée après déploiement).
- Auto-diagnostic du check-in : tout routeur agent dont PoolCap est nul ou
  dont le diagnostic dépasse 7 jours reçoit un pool_doctor en DIAGNOSTIC
  PUR (recycle/extend OFF — le cloud ne modifie jamais la configuration de
  son propre chef). Vague différée 97 (fermeture du batch, ne bloque rien).

### Mesure continue + alerte
- `read_state` rapporte `hosts` à chaque chunk → Router.PoolHosts.
- Moniteur 30 s : occupation = PoolHosts/PoolCap — `high` ≥ 80 %, `full`
  ≥ 95 % — notification `pool_alert` (Telegram/WhatsApp/e-mail) à chaque
  transition, anti-spam mémorisé (NotificationSettings.PoolAlertState,
  JSON en colonne notif_settings.pool_alert_state). Capacité inconnue
  (PoolCap=0) → aucune alerte.

### Modèle (nouvelles colonnes routers)
`PoolCap int`, `PoolHosts int`, `PoolRanges text`,
`PoolDoctorAt text` — migrations idempotentes, `omitempty` (absent tant
que jamais diagnostiqué).

## N°103 — Qualité de ligne : mesure passive du débit FAI [P1]

### Contexte
Chaque site MikCloud a un FAI et un forfait différents ; aucun routeur ne peut
exposer « le débit du forfait » (paramètre commercial opérateur). La capacité
EFFECTIVE se mesure passivement : l'enveloppe des débits observés sur
l'interface WAN en fenêtres ~2 min (celles de read_state) converge vers le
plafond réel de la ligne. Les agrégats survivent aux redéploiements (histogrammes
fusionnables), le p95 filtre les pics isolés, le max borne le plancher honnête.

### Détection WAN (script read_state, N°103)
`read_state` rapporte en plus `wan=<iface>` : première route par défaut ACTIVE
(`/ip route find where dst-address="0.0.0.0/0"` → `gateway-status`
« reachable via <iface> »). Vide si aucune (l'endpoint ne rattache alors AUCUNE
interface — aucun WAN deviné). Appliqué sur `Router.WanIface` (lecture seule
côté console : vérité routeur, jamais une saisie). Simulé : `ether1` par
convention (le WanIface est posé au Tick).

### Modèle (nouvelles collections/colonnes)
```go
type LineQualityDay struct { // une ligne par (routeur, jour UTC, interface)
    ID, AccountID, RouterID    string
    Day   string `json:"day"`   // "2006-01-02" (UTC == Abidjan, arbitrage N°82)
    Iface string `json:"iface"` // WAN détecté pour la mesure FAI
    Samples   int   `json:"samples"`
    RxMaxBps  int64 `json:"rxMaxBps"` // plus haute fenêtre ~2 min du jour
    TxMaxBps  int64 `json:"txMaxBps"`
    RxHist    string `json:"rxHist"`  // histogramme « c0,…,c15 » (seaux log, fusionnables)
    TxHist    string `json:"txHist"`
    UpdatedAt string `json:"updatedAt"`
}
```
Table `line_quality` (+ index account/router). Router : `WanIface text`,
`LineDownBps bigint`, `LineUpBps bigint` (capacité DÉCLARÉE par le gérant,
bits/s, 0 = non renseignée — PAR ROUTEUR, jamais globale). Rétention 90 j
(PruneLineQuality, moteur commun applyExpiry).

Alimentation : chaque fenêtre de mesure (read_state côté agent, Tick côté simulé)
verse ses débits par interface via `AccumulateLineQuality` (~720 échantillons/jour).
La PREMIÈRE mesure (pas de référence) ne compte pas.

### Route
- `GET /api/routers/{id}/line-quality` (toute l'équipe connectée, comme /traffic ;
  real → 400) :
```json
{
  "routerId": "r-…", "wanIface": "ether1",
  "configured": { "downBps": 110000000, "upBps": 20000000 },
  "days": [ { "day": "2026-09-14", "samples": 312, "rxMaxBps": 96000000,
              "txMaxBps": 18000000, "rxP95Bps": 75000000, "txP95Bps": 10000000 } ],
  "measured": { "downBps": 96000000, "upBps": 19000000,
                "p95DownBps": 75000000, "p95UpBps": 10000000,
                "days": 3, "confident": true },
  "live": { "rxBps": 42000000, "txBps": 9000000, "at": "…" }
}
```
  `days` : jusqu'à 14 jours de l'interface WAN, du plus récent au plus ancien.
  `measured` : enveloppe sur les jours ÉCLOS qualifiés uniquement (samples ≥ 50,
  hors jour courant ; `confident` à partir de 3 jours) — une ligne peu chargée
  n'est jamais « mesurée » à son étiquette (le plancher observé seul est rendu).
  `live` : débit courant de l'interface WAN (même source que l'onglet Trafic).
- `PUT /api/routers/{id}` : `lineDownBps` / `lineUpBps` (bits/s, bornes
  0–10 Gbps, 0 = effacer la déclaration).

## N°104 — QoS Manager : plafond agrégat du hotspot automatisé [P1]

### Contexte
Suite de N°103 : la mesure donne la capacité, le QoS Manager en fait une file.
Une SEULE file statique « mikcloud-qos » cible le sous-réseau hotspot
(max-limit upload/download, types PCQ PAR DÉFAUT de RouterOS :
`pcq-upload-default`/`pcq-download-default`, pcq-rate=0) ; les files
dynamiques des utilisateurs (rate-limit des user profiles) deviennent ses
ENFANTS via `parent-queue` — l'arbitrage HTB natif remplace l'ordre fragile
de la liste des files : le plafond agrégat s'applique VRAIMENT. Paramètres
duaux orientés upload d'abord (convention RouterOS). Valeurs émises en bps
bruts (`max-limit=19000000/95000000`), relecture routeur formatée (« 19M »)
normalisée en bps par le cloud (`RosRateBps`) : la vérification est bit à
bit, jamais textuelle.

N°110 — la cible accepte désormais une LISTE (1 à 4 CIDR IPv4 séparés par
des virgules, ex. `192.168.10.0/24,10.77.0.0/21`) : un hotspot dont le pool
a été étendu (docteur N°108 — range dédié 10.77.0.0/21) vit sur des
sous-réseaux DISJOINTS qu'aucun préfixe unique ne couvre ; RouterOS accepte
plusieurs cibles dans UNE file — le plafond agrégat reste UN. La
VÉRIFICATION compare des ENSEMBLES (une relecture réordonnée reste
conforme) ; côté builder, tout élément invalide fait retomber TOUTE la
cible sur le défaut franc 192.168.88.0/24.

### Commandes agent (kinds)
- `queue_ensure` — create-or-set idempotent + RELECTURE de vérification
  (target|max-limit|queue|disabled). Signature posée seulement si la
  relecture correspond au payload ET si l'état désiré est toujours courant.
- `queue_read` — toutes les files (dynamiques incluses, cap 60) :
  name|target|max-limit|queue|disabled|bytes|rate|dynamic → stats + dérive
  (le drapeau `dynamic` N°106 étiquette honnêtement les files des
  utilisateurs et garde le ménage loin d'elles ; un rapport ancien sans la
  colonne reste toléré — l'heuristique des noms `<…>` fait foi).
- `queue_remove` — détache d'abord les profils qui référencent la file
  (`set [find parent-queue=X] parent-queue=none`), retire la file, prouve
  la disparition (compte restant rapporté). Payload `name` optionnel
  (N°106 « ménage à distance ») : absent = file agrégat mikcloud-qos
  (convergence QoS) ; présent = retrait d'une file LEGACY posée à la main
  (HOTSPOT-Total…). Nom validé strictement (1-64 : lettres, chiffres,
  espaces internes, `- _ .`) : un payload corrompu fait ÉCHOUER la commande,
  jamais de repli vers une autre file.

### Modèle (colonnes routers)
`QoSEnabled bool`, `QoSTarget text` (1 à 4 CIDR IPv4, virgules — N°110),
`QoSMaxUpBps/QoSMaxDownBps bigint` (max-limit appliqué — burst = max×20/19,
seuil = 80 % du max : DÉRIVÉS, jamais persistés), `QoSSig text` (config
appliquée, hash cible+limites+sel qos-v1), `QoSAppliedAt text`.

### Convergence (pattern walled_garden)
- Check-in : QoS active + sig différente/stale → `queue_ensure` (vague
  différée 104, fermeture) ; désactivée mais file posée (QoSAppliedAt) →
  `queue_remove` ; sig fraîche → monitoring `queue_read` 30 min.
- Retour VÉRIFIÉ du ensure → sig + appliedAt + rattachement des profils
  (ParentQueue = mikcloud-qos, machinerie profile_set — mono-routeur agent
  par compte uniquement, arbitrage documenté : le champ est account-level).
- `queue_read` : file absente/divergente → sig vidée → re-file (auto-
  réparation). Auto-ré-assertion complète toutes les 6 h (qosRefresh).

### Routes
- `GET /api/routers/{id}/qos` (lecture, comme /traffic ; real → 400) :
```json
{
  "queueName": "mikcloud-qos", "queueTypes": "pcq-upload-default/pcq-download-default",
  "burstTime": "10s/10s",
  "status": { "enabled": true, "target": "192.168.10.0/24",
              "maxUpBps": 19000000, "maxDownBps": 95000000,
              "burstUpBps": 20000000, "burstDownBps": 100000000,
              "thrUpBps": 15200000, "thrDownBps": 76000000,
              "applied": true, "appliedAt": "…", "removalPending": false },
  "recommendation": { "source": "declared|measured|none",
                      "capacityDownBps": 100000000, "capacityUpBps": 20000000,
                      "maxDownBps": 95000000, "maxUpBps": 19000000,
                      "burstDownBps": 100000000, "burstUpBps": 20000000,
                      "thrDownBps": 76000000, "thrUpBps": 15200000,
                      "measuredDays": 3 },
  "queues": { "queued": false, "updatedAt": "…",
              "data": [ { "name": "mikcloud-qos", "target": "192.168.10.0/24",
                          "maxLimit": "19M/95M", "maxUpBps": 19000000, "maxDownBps": 95000000,
                          "queue": "pcq-upload-default/pcq-download-default",
                          "disabled": false, "dynamic": false,
                          "bytesUp": 123456, "bytesDown": 654321,
                          "rateUpBps": 9500000, "rateDownBps": 72000000 } ] }
}
```
  `recommendation` : capacité déclarée (N°103) prioritaire, sinon enveloppe
  mesurée (confidente), sinon `none` — jamais inventée. max = 95 %, burst =
  capacité, seuil = 80 % du max (anti-bufferbloat : le shaper routeur est LE
  goulot). `queues` : cache d'un queue_read done < 120 s (mécanique outils
  F9) sinon lecture à la demande ; simulé → lignes déterministes.
- `PUT /api/routers/{id}/qos` (rang 2) `{enabled, target, maxUpBps,
  maxDownBps}` : pose l'état désiré (champs absents = inchangés ; target =
  1 à 4 CIDR IPv4 séparés par des virgules, chacun canonisé, doublons
  dédoublonnés — N°110 ; limites 1 Mbps–10 Gbps) et enfile immédiatement
  queue_ensure / queue_remove (agent). La convergence complète suit au
  check-in (≤ 45 s).
- `DELETE /api/routers/{id}/qos` (rang 2) : désactivation — la config est
  conservée (ré-allumage), les profils sont détachés, la file retirée.
- `DELETE /api/routers/{id}/queues/{name}` (rang 2, N°106 « ménage à
  distance ») : retrait d'une file statique LEGACY (posée à la main avant
  le QoS Manager) SANS être sur site — détache les profils qui la
  référencent (mono-routeur agent), enfile un queue_remove nommé,
  disparition prouvée au retour (compte restant 0), l'état QoS du routeur
  reste INTACT. Gardes-fous : `mikcloud-qos` refusé (400 — passer par
  « Désactiver la QoS »), noms dynamiques `<…>` refusés (400), charset
  strict (400), re-clic pendant le vol = idempotent (pas d'accumulation,
  retraits de noms distincts coexistent). Simulé → no-op convergent. Le
  cache `queues` de GET /qos est invalidé par tout queue_remove terminé
  postérieur à la dernière lecture (la table montre la vérité routeur,
  jamais un cliché antérieur au geste).

---

## N°106 — Mode bridage : quota data à l'atterrissage en douceur [P1]

### Contexte
Un forfait « 1 h / 1 Go » dont le quota data épuisé COUPE la connexion
(comportement natif RouterOS `limit-bytes-total`) punit le client au pire
moment. Le mode bridage (« soft-landing ») maintient la connexion : le débit
est réduit jusqu'à l'expiration du TEMPS (`limit-uptime` inchangé — c'est lui
qui coupe, jamais le quota). RouterOS n'a aucun réglage natif pour ça (le menu
`/ip hotspot active` est informationnel) : la construction éprouvée =
file simple STATIQUE `mikthrottle-<user>` ciblant l'IP du client, posée
AU-DESSUS de la file dynamique du profil (`place-before` — évaluation en
ordre, premier-match gagnant), retirée au départ de la dernière session.

### Modèle (nouvelles colonnes)
- `Profile` gagne : `QuotaMode string json:"quotaMode"` — `"cut"` (défaut,
  comportement historique) ou `"throttle"` ; `""` (pré-N°106) = cut.
  `ThrottleRate string json:"throttleRate"` — format RouterOS simple
  (`512k`, `1M`, `512k/2M` ; regex `^\d+[kKmMgG]?(/\d+[kKmMgG]?)?$`).
  En mode throttle : quota data > 0 Mo ET débit valide OBLIGATOIRES
  (refus 400 sinon — jamais une config à moitié posée).
- `Session` gagne : `Throttled bool json:"throttled,omitempty"` — une file
  `mikthrottle-<user>` existe sur le routeur (vérité routeur).
- `Router` gagne : `QuotaSchedOK bool json:"quotaSchedOK,omitempty"` —
  scheduler `mikcloud-quota` confirmé déployé (pattern watcher N°77).
Migrations idempotentes : `profiles.quota_mode/throttle_rate`,
`sessions.throttled`, `routers.quota_sched_ok`.

### Marqueur de quota (commentaire routeur)
En mode throttle, le quota NE devient PAS un `limit-bytes-total`. Il vit en
TÊTE du commentaire utilisateur : `mikq:<octets>,<débit>` (ex.
`mikq:1073741824,512k/512k custom · mikcloud:b1`). La virgule et le `/` ne
figurent pas parmi les séparateurs neutralisés par l'import (| ; & = % +) ;
posé en tête, le marqueur survit à la troncature d'import à 60 caractères et
au suffixe `mikcloud_lock:` du verrou « 1er appareil ». Les overrides de
quota PAR LOT restent supportés : le marqueur porte la valeur effective.

### Enforcement routeur (100 % sortant, autonome)
- `on-login` du profil (throttle) : à CHAQUE connexion, cumul utilisateur
  (bytes-in + bytes-out de `/ip hotspot user`, jamais ceux de la session)
  ≥ quota du marqueur → file posée pour l'IP de la session (remove-then-add,
  ancrée EN TÊTE de liste — N°113). Couvre la fenêtre de re-login (sinon :
  plein débit jusqu'au tick). FUSIONNÉ au script du verrou « 1er appareil »
  si les deux actifs (un seul champ on-login par profil — blocs
  `:do{}on-error{}` indépendants).
- `on-logout` du profil (throttle) : dernière session partie → file retirée
  (anti-héritage d'IP par le prochain occupant du bail DHCP).
- scheduler `mikcloud-quota` (tick 20 s, remove-then-add idempotent, posé par
  InstallScript ET par la commande `quota_ensure`) : (1) balayage des
  orphelins — toute file `mikthrottle-*` sans session active est retirée
  (couvre aussi le reboot routeur : les files statiques survivent, pas les
  sessions) ; (2) ≤ 250 sessions actives par tick : marqueur relu, cumul
  comparé — ≥ quota → file posée EN TÊTE ; < quota → file retirée (le
  reset-counters F4 rouvre le plein débit au tick suivant). Aucun octet émis
  vers le cloud : le routeur applique la politique même coupé du cloud.
- Convergence : `ensureQuotaThrottleLocked` au check-in ne file `quota_ensure`
  QUE si le compte possède ≥ 1 profil throttle (économie de veille N°75
  entière sinon). `QuotaSchedOK` posé au retour « ok » uniquement
  (pattern watcher). Commande idempotente (`staleSentReadKinds`).
- Scripts du profil : la ligne `set` de `profileEnsureLine` aligne ELLE AUSSI
  `on-login` (combiné verrou+quota) et `on-logout` (N°113) — le `add` seul
  ne suffit pas : sur un profil PRÉEXISTANT l'add échoue et le set est la
  seule écriture.

### Rapport read_state
`throttle=user1,user2,…` (noms des files `mikthrottle-` présentes dans
`/queue simple`, le suffixe du nom EST le username) — rapporté par le chunk
final, comme `sessions`. Chaque session live reçoit `Throttled` depuis cette
liste (jamais un calcul cloud). Rapport sans le champ (pré-N°106) = faux.

### Routes
- `POST/PUT /api/profiles` : `quotaMode` (`cut|throttle`, défaut/vide = cut),
  `throttleRate` (requis en throttle). La validation porte sur l'état FUTUR
  du profil en PUT (les champs non fournis héritent de l'existant).
- `GET /api/sessions` : champ `throttled` par session (miroir du modèle).
- `POST /api/users`, `POST /api/vouchers/generate` : le payload agent
  embarque `quotaMode` (via profileRef) + `throttleRate` — le cloud reste la
  source de vérité, le resync utilisateur (`user_resync`) reconstitue le
  marqueur.

### Limites documentées (v2)
- `shared-users` > 1 : une seule file par utilisateur (dernière IP) —
  recommander 1 appareil simultané sur les profils à quota.
- Latence de détection : ~intervalle (20 s) × débit du profil de
  sur-consommation possible avant bridage (le on-login ferme la fenêtre de
  re-login).
- Compteurs de LA FILE de bridage remis à zéro à chaque tick (pose
  remove-then-add) : purement cosmétique — la décision se fait sur les
  compteurs CUMULÉS utilisateur, et le read_state ne rapporte que les NOMS
  des files `mikthrottle-`.
- Le trafic d'un utilisateur bridé n'entre plus dans le plafond agrégat
  `mikcloud-qos` (N°104) quand celui-ci est une simple queue plate : la file
  `mikthrottle-` (plus spécifique, en tête) matche en premier — le contrat de
  BRIDAGE prime sur l'agrégat (compromis documenté ; en mode HTB
  parent-queue l'agrégat reste respecté).
- Un routeur avec une règle firewall `fasttrack-connection` générique qui
  matche le trafic hotspot authentifié contourne TOUTES les simple queues
  (manuel RouterOS) : si même le débit DE BASE du forfait ne s'applique pas,
  vérifier/retirer la règle fasttrack (diagnostic terrain — les compteurs
  hotspot continuent de compter, le fasttrack ne casse ni l'auth ni les
  quotas temps).

### N°113 — Correctif terrain : « le bridage qui ne bridait rien »
Test N°106 réel (routeur client cybere-space sc, voucher `testa` — 15 min /
50 Mo / bridage 512k/1M) : **200+ Mo consommés sans AUCUN bridage** jusqu'à
l'expiration du temps. Autopsie (sources : manuel RouterOS
manual.mikrotik.com + sorties réelles du forum MikroTik) :
1. **LA CAUSE RACINE — ancre morte** : la file dynamique hotspot se nomme
   `<user>` AVEC CHEVRONS (sortie réelle : `name="<hotspot-user3>"`), et
   les simple queues s'évaluent en ORDRE STRICT premier-match-gagnant
   (manuel : « each packet must go through every queue until it reaches one
   queue whose conditions fit »). L'ancre `find where name=$qu` (nom NU)
   ne trouvait JAMAIS la dynamique → la file `mikthrottle-testa` était
   ajoutée en BAS de liste, SOUS `<testa>` → **pas un seul paquet vu par le
   bridage**. Correctif : pose v2 = remove-then-add + `place-before` LA
   PREMIÈRE file de la liste (repli `/queue simple move` en tête si le
   place-before échoue — l'opération historiquement supportée pour placer
   une statique avant les dynamiques, pattern des forums 2007+). Au-dessus
   de la dynamique `<user>`, du plafond `mikcloud-qos` (N°104) et de toute
   file opérateur — seules des files aux cibles DISJOINTES peuvent rester
   au-dessus (aucun effet sur le trafic de cet utilisateur).
2. **Le set qui effaçait le on-login** : `profileSetLine` écrasait
   `on-login` avec le verrou SEUL (ou vide) et n'alignait JAMAIS
   `on-logout` — le on-login de bridage posé par le `add` de la MÊME
   commande était effacé aussitôt ; sur un profil préexistant (add en
   échec), le set était la SEULE écriture : AUCUN script de bridage.
   Correctif : le set porte le on-login COMBINÉ (verrou+quota) et le
   on-logout (vidés en mode cut — alignement complet).
3. **Le tick figé sans version** : le script vit dans le `on-event` du
   scheduler ROUTEUR — `QuotaSchedOK` vrai bloquait tout re-déploiement :
   les routeurs déjà équipés du tick v1 ne recevraient JAMAIS le correctif.
   Correctif : `Router.QuotaSchedVer` (génération confirmée, portée par le
   payload de `quota_ensure` et posée au retour « ok » — pattern sel
   `safeWifiRulesVersion` N°80). Migration `routers.quota_sched_ver` ; 0 =
   pré-N°113 → re-file automatique au premier check-in après déploiement.

### N°116 — Correctif terrain v3 : « le tick armé qui restait silencieux »
Re-test N°113 (routeur cybere-space sc, RouterOS 7.24.1, voucher `4327` —
15 min / 50 Mo / bridage 1M/512k, créé PAR MikCloud) : **219 Mo consommés
sans AUCUN bridage**, encore. Autopsie AVEC les données de production
(Neon : payloads, résultats, `read_scheduler`, `queue_read`, read_state) :
1. **Tout était ARMÉ** : tick v2 déployé et confirmé (`QuotaSchedVer=2`,
   scheduler `mikcloud-quota` 20 s, on-event v2 relu mot à mot depuis le
   rapport `read_scheduler`), marqueur `mikq:52428800,1M/512k` présent
   (payload du voucher_batch vérifié), `profile_set` avec le on-login
   combiné livré 6 min avant le test — et `throttle=""` dans CHAQUE
   read_state de la fenêtre de bridage : **la file n'a JAMAIS existé, pas
   même mal placée en bas de liste**.
2. **LA CAUSE RATTEE — la source de décision** : la comparaison v2
   reposait sur les SEULS compteurs UTILISATEUR
   (`/ip hotspot user get <id> bytes-in/out`). C'est l'opération du script
   jamais prouvée sur ce routeur : les compteurs SESSION
   (`/ip hotspot active` — rapportés par le read_state toutes les ~2 min)
   sont fiables, mais si les compteurs UTILISATEUR sont illisibles ou
   figés sur RouterOS 7.x, chaque lecture protégée retombe à 0 →
   `0 + 0 >= quota` est FAUX à CHAQUE tick → branche retrait → **rien,
   silencieusement, pour toujours**. (L'hypothèse fasttrack est ÉLIMINÉE
   par les compteurs : la file dynamique `<hotspot-4327>` comptait les
   mêmes octets que la session — le trafic traverse bien les simple
   queues.)
3. **Correctif v3 — décision = MAX des deux sources, JAMAIS leur somme** :
   - compteurs cumulés UTILISATEUR (protégés on-error — survivent aux
     reconnexions QUAND le RouterOS les tient) ;
   - octets de la PLUS GROSSE session ACTIVE de l'utilisateur
     (`/ip hotspot active` — source prouvée fiable sur le terrain).
     Pourquoi max : si les compteurs utilisateur sont live, ils INCLUENT
     la session en cours (somme = double comptage, bridage trop tôt) ;
     s'ils sont figés au logout, le max sous-estime d'au plus un reste de
     quota (dégradation bornée) ; s'ils sont absents, le max retombe
     exactement sur la session — **le bridage en cours de session
     fonctionne, seule la fenêtre de re-login peut se rouvrir**
     (dégradation documentée, mesurable — cf. la sonde).
4. **Sonde `qcounters` (ok|err|na)** : le chunk final du read_state tente
   la lecture d'un compteur utilisateur (typeof non-nil, protégé on-error)
   et rapporte le résultat. Le champ atterrit dans le résultat de la
   commande (historique queryable à distance) : `ok` = au moins un
   compteur utilisateur lisible ; `err` = parc sondé, aucun lisible ;
   `na` = aucun utilisateur. La décision v3 ne DÉPEND PAS de la sonde
   (dégradation gracieuse) — elle mesure le mode réel du parc.
5. **Convergence** : `QuotaTickVersion = 3` — le payload de `quota_ensure`
   porte la génération, `ensureQuotaThrottleLocked` re-file tout routeur
   dont `QuotaSchedVer < 3` : le parc re-converge au premier check-in
   après déploiement, SANS geste opérateur (pattern N°113).
6. **Limites documentées** : (a) si `qcounters=err`, la fenêtre de
   re-login d'un voucher épuisé peut se rouvrir (plein débit jusqu'au
   tick qui voit la nouvelle session dépasser le quota — v4 possible :
   persistance du consommé au logout) ; (b) le marqueur `mikq:` n'est
   posé QUE sur les utilisateurs créés par MikCloud (`user_add` /
   `voucher_batch`) : un voucher créé dans Winbox ou Mikhmon sous un
   profil throttle ne porte pas le marqueur — aucun bridage pour lui
   (le quota par lot reste une création MikCloud).

### N°127 — Assistant conversationnel public (chatbot) + inbox support

La section « Questions fréquentes » de la vitrine est remplacée par un
assistant conversationnel : le bot répond depuis une base d'intents FAQ
(FR/EN), et la conversation peut être transmise à un humain qui répond
depuis la console plateforme (vue « Conversations »).

**Modèle** : `ChatConversation` (`id` "chat-…", `token_hash` SHA-256 hex
du secret visiteur — JAMAIS le secret en clair, `lang` fr|en, `status`
`bot|human|closed`, `created_ip`, `created_at`, `updated_at`, `unread`)
et `ChatMessage` (`id` "cmsg-…", `conversation_id`, `sender`
`visitor|bot|agent`, `body`, `at`). Le fil est APPEND-ONLY : jamais
réordonné ni inséré au milieu — les clients suivent par OFFSET (nombre de
messages connus), pas par horloge.

**Routes publiques** (sans JWT — le secret visiteur fait l'auth ; quota IP
`a.chat` 30/10 min + 400/24 h sur les trois POST ; corps ≤ 1 000 car.) :

| Route | Corps / Query | Réponse |
|---|---|---|
| `POST /api/chat/session` | `{lang}` ("fr" défaut) | `201 {id, token, status, total, messages[]}` — message de bienvenue |
| `POST /api/chat/message` | `{token, body, offset, lang?}` | `{status, total, messages[]}` — fil depuis `offset` (inclut le message visiteur confirmé + la réponse bot) |
| `GET /api/chat/messages` | `?token=&offset=` | `{status, total, messages[]}` — polling visiteur |
| `POST /api/chat/handoff` | `{token, offset, lang?}` | `{status, total, messages[]}` — transmission explicite |

Statuts : `bot` (l'assistant répond), `human` (transmis — le bot se tait,
les messages visiteur incrémentent `unread`), `closed` (clôturée par le
support, message de fin automatique côté visiteur). L'intent « humain »
reconnu dans un message (mots-clés : humain, conseiller, support…) a le
même effet qu'un handoff explicite. Mauvais token → `404`.

**Langue (N°128)** : la conversation suit le visiteur — `message` et
`handoff` acceptent un champ optionnel `lang` (`"fr"`/`"en"`) ; le
backend aligne `conversation.lang` dessus AVANT de générer la réponse
bot ou le message de transmission (le bot répond dans la langue
affichée, l'inbox support voit la préférence à jour). Champ absent ou
invalide → aucun changement (rétro-compatible).

**Routes console plateforme** (`requireRole(3)`) :

| Route | Effet |
|---|---|
| `GET /api/admin/chat/conversations` | `{conversations[], summary}` — lignes sans le hash du secret (statut, langue, dernier message, non-lus, activité), tri human → bot → closed puis récent |
| `GET /api/admin/chat/conversations/{id}` | `{conversation, total, messages[]}` — ouvrir le fil remet `unread` à 0 |
| `POST /api/admin/chat/conversations/{id}/reply` | `{body}` (≤ 2 000 car.) → `{ok, message}` — la conversation passe (ou reste) `human` : après une intervention humaine le bot ne reprend jamais la main ; répondre à une clôturée la rouvre |
| `POST /api/admin/chat/conversations/{id}/close` | → `{ok}` — statut `closed` + message de fin |

**Rétention** (`chatPruneLocked`, exécutée sous le verrou à la création de
session) : conversations `closed` de plus de 30 jours et `bot` sans
activité depuis 7 jours purgées avec leurs messages ; les conversations
`human` ne sont jamais purgées automatiquement ; garde-fou mémoire à
2 000 conversations. Limite connue : une conversation très longue n'est
pas plafonnée en messages (le volume est borné par la purge par statut).

**Clôture automatique + rétention périodique (N°129)** : une conversation
vivante (`bot` OU `human`) sans nouveau message depuis **15 minutes** est
fermée par l'assistant — `chatAutoCloseLocked`, opération ATOMIQUE sous le
verrou du store (aucune race entre le test d'inactivité, un message
visiteur et la clôture), message de fin dédié dans la langue de la
conversation (distinct de la clôture support). L'horloge est le DERNIER
MESSAGE du fil (`updated_at`) : pour une conversation `bot` c'est
exactement le dernier message du visiteur (l'assistant répond dans la
même seconde) ; pour une conversation `human`, la réponse d'un conseiller
relance le délai — le visiteur garde un quart d'heure pour lire et
répondre. Deux déclencheurs : le balayage de fond `RunChatSweepForever`
(goroutine main.go, **chaque minute**, rattrapage au démarrage, filet
anti-panique N°74 — `chat_sweep.go`) et la lecture
`GET /api/admin/chat/conversations` (l'inbox console ouverte, les fils
morts apparaissent déjà fermés). Un message du visiteur sur une
conversation clôturée (support OU automatique) la ROUVRE : `human` si un
conseiller était déjà intervenu (`chatAgentEverRepliedLocked` — le bot ne
reprend jamais la main après un humain), sinon `bot` (l'assistant répond
à nouveau) ; la réponse d'un conseiller rouvre aussi (comportement
existant). Le widget de la vitrine propose quant à lui une nouvelle
conversation au statut `closed`. La rétention `chatPruneLocked` tourne
désormais AUSSI au balayage périodique (chaque minute) — plus seulement
à la création de session : la purge est garantie même sans nouveau
visiteur. Console : la vue « Conversations » affiche les deux règles
(auto-close 15 min + rétention 30 j / 7 j) sous l'en-tête, i18n FR/EN.

---

### N°123 — Badges annuels retirés, essai Hotspot 60 jours, migration des essais actifs

Retour utilisateur : « Supprime les (2 mois offerts) sur les cartes
annuelles. Essai Hotspot réduit à 60 jours, mettre à jour les clients
actifs. »

**Badges** : les formules annuelles (`hotspot-annuel`, `homenet-annuel`)
n'ont PLUS de `badge` (champ vide, omis du JSON de `GET /api/plans` et
`GET /api/subscription`) ; les mensuelles gardent « Sans engagement ».

**Essai** : `trialPeriodEnd` = 30 jours HomeNet / **60 jours** Hotspot
(avant N°123 : 3 mois) ; `trialDefaultMonths` = 1 mois Maison / **2 mois**
Hotspot (défaut d'attribution plateforme sans durée explicite) ; la
création de compte par la plateforme (`POST /api/admin/accounts`) pose le
MÊME essai segmenté que l'inscription publique (avant : 3 mois fixes
quelle que soit l'usage du compte).

**Migration idempotente** (`store.migrateActiveTrialCap`, exécutée au
chargement PG/JSON/Reload après `migrateUsageScopedPlans`) : pour chaque
compte de statut `active` dont `Subscription.PlanID == "essai"`, si
`PeriodEnd > PeriodStart + durée segmentée` (60 j hotspot / 30 j homenet),
alors `PeriodEnd = PeriodStart + durée`. La migration ne fait que
raccourcir (jamais allonger) ; les abonnements payés, les comptes
désactivés et les périodes non expirantes (`PeriodEnd` vide) ne sont pas
touchés. Un essai déjà entamé au-delà de la durée cible devient `expired`
(lecture seule) puis `suspended` après la grâce de 30 jours — comportement
voulu de la réduction.

---

### N°122 — Tarifs segmentés Hotspot / HomeNet : « Deux modes, un nuage »

Retour utilisateur : le catalogue unique (Essentiel 1 250 F/mois/routeur,
Illimité 12 000 F/an) facturait pareil un cybercafé qui monétise son WiFi et
un foyer qui se protège. Décision produit :

| Formule | Prix | Couverture | Mode |
|---|---|---|---|
| `hotspot-mensuel` | 2 500 F / mois | par routeur (`PerRouter`) | hotspot |
| `hotspot-annuel` | 25 000 F / an | routeurs illimités | hotspot |
| `homenet-mensuel` | 1 250 F / mois | par routeur (`PerRouter`) | homenet |
| `homenet-annuel` | 12 000 F / an | routeurs illimités | homenet |

**Essai segmenté** : 3 mois (~90 jours) en Hotspot, **30 jours** en HomeNet
(`trialPeriodEnd`, posé à l'inscription selon l'usage ; défaut plateforme
`trialDefaultMonths` : 1 mois Maison, 3 mois Hotspot).

**Résolution — source unique** (`model.ResolvePlan(id, usage)`) :
- identifiant segmenté exact → la formule ;
- identifiants HISTORIQUES `essentiel`/`illimite` (abonnements antérieurs)
  → la formule du MÊME MODE que le compte (usage vide/inconnu → hotspot) ;
- `essai` et inconnus → pas de formule.

**Application** (`applySubscriptionLocked`) : mensuelle = prix × slots ×
mois ; annuelle = forfait pro-ratisé `prix × mois / 12` (division entière —
12 mois = prix catalogue exact). L'identifiant STOCKÉ est NORMALISÉ vers
l'identifiant segmenté ; l'empilement compare les identifiants normalisés.

**Migration idempotente** (`store.migrateUsageScopedPlans`, exécutée au
chargement PG/JSON/Reload) : réécrit `SettingsByAccount[].Subscription.PlanID`
et `BillingRequests[].PlanID` historiques vers les identifiants segmentés du
mode du compte (périodes et `LastAmountFcfa` conservés : le RENOUVELLEMENT
applique le nouveau tarif — décision prix, pas de rattrapage rétroactif) ;
libellés compat `Settings.Plan` rafraîchis.

**Garde de mode (serveur)** : `POST /api/subscription` et
`POST /api/subscription/stripe` refusent la formule d'un autre mode
(400, code `wrong_mode`) ; `GET /api/subscription` renvoie UNIQUEMENT les
formules du mode du compte + le champ `usage` ; `guardAccountRouterLimit`
plafonne les mensuelles par routeur (`IsPerRouterPlanID`, essai à part) ;
webhook Wave, `finalizeBillingSuccess`, activation plateforme et resync
carte alignés sur `IsAnnualPlanID`/`IsPerRouterPlanID`.

**Limites connues (consignées)** :
- Les prélèvements carte GeniusPaySubs créés AVANT le N°122 gardent leur
  identifiant et leur MONTANT souscrit chez GeniusPay (tarif de création) :
  la facture applique la formule résolue mais le débit reste l'ancien
  montant — résilier et re-créer l'abonnement carte pour aligner.
- Une demande de facturation en attente au moment du déploiement conserve
  le montant calculé à la demande ; l'activation (décision plateforme)
  applique le tarif segmenté courant — l'opérateur voit l'écart dans le
  journal (« abonnement X activé (N FCFA) »).

**Frontend** : catalogue consommé par PÉRIODE (`mois`/`an`), jamais par id ;
`SubscriptionView.usage` pilote les caractéristiques (Hotspot vs HomeNet) ;
dialog plateforme : options par usage du compte, preview miroir (mensuelle =
prix × slots × mois, annuelle = `Math.floor(prix × mois / 12)`) ; vitrine :
sélecteur clay Hotspot/Maison (`aria-pressed`), 3 formules par mode, essai
30/90 jours affiché, FR/EN.

---

### N°118 — Convention débit : « le libellé qui inversait le sens »
Re-test N°116 RÉUSSI (voucher `7388` sur CYBER S.C : file `mikthrottle-7388`
en position 0, `max-limit=1M/512k`, download plombé à 512,9 kbps live,
dynamique gelée à 53,7 Mo) — mais retour utilisateur : « dans le formulaire
de création de profil, au niveau du bridage, le download est placé avant
l'upload, d'où 1M/512 dans le test — pareil pour la QoS (plafond agrégat) ».

**La chaîne de la confusion** (aucun bug de données — un bug de
COMMUNICATION du sens) :
1. RouterOS lit `max-limit`/`rate-limit` en ordre **montant/descendant**
   (upload/download) — preuve terrain N°116 : `1M/512k` plombait le
   download à 512,9 kbps (2ᵉ valeur = descendant).
2. Le libellé du Studio Forfait annonçait « Limite de débit
   **(descendant/montant)** » — l'inverse exact. L'opérateur tape le 1M
   (download voulu) en premier → la chaîne part verbatim au routeur →
   le routeur l'applique comme MONTANT. Le test N°116 a donc bridé
   down 512k/up 1M au lieu de down 1M/up 512k.
3. `formatRateLimit` (aperçu live du wizard, liste des profils, presets
   bridage) décorait la 1ʳᵉ valeur d'une flèche ↓ — confirmant
   visuellement le mauvais sens.
4. La carte QoS affichait « Plafond descendant » AVANT « Plafond montant »
   (saisie + forfait FAI déclaré + recommandation) — mêmes valeurs
   correctes (champs séparés, backend `maxUp/maxDown` déjà dans le bon
   ordre), mais ordre incohérent avec la table des files qui affiche
   déjà « Plafond (montant/descendant) ».

**Correctif (frontend uniquement, aucune donnée transformée au passage)** :
- `formatRateLimit` : `1M/10M` → « 1M ↑ / 10M ↓ » (montant d'abord).
- Studio Forfait : libellé « Limite de débit **(montant/descendant)** »,
  hints/toasts explicites (FR+EN), « Débit de bridage **(montant/descendant)** ».
- Carte QoS : « Plafond montant » AVANT « Plafond descendant » (saisie),
  forfait FAI déclaré « montant / descendant » (saisie + affichage +
  exemple), recommandation ↑ d'abord.
- Convention UNIQUE dans toute la console : **une paire de débit se lit
  et se saisit dans l'ordre RouterOS montant/descendant** — WYSIWYG avec
  Winbox (l'opérateur vérifie ses files dans Winbox : ce qu'il tape dans
  MikCloud est ce qu'il voit là-bas).

**Rattrapage de la donnée existante** (geste opérateur ponctuel, pas une
migration) : le profil `Test` (seul profil throttle du parc) portait
`throttle_rate=1M/512k` saisi avec l'ancien libellé (intention :
down 1M / up 512k) — corrigé en base vers `512k/1M`. Les `rate_limit`
existants (1M/10M, 512k/6M…) étaient déjà tapés RouterOS-style (petit
montant d'abord) : inchangés. Un profil édité doit être re-sauvegardé
pour re-pousser son `profile_set` (le marqueur `mikq:` des vouchers
EXISTANTS conserve le débit de leur création).

---

## F13 — Marge : prix de vente vs coût [P2]

### Modèle
- `Profile` gagne : `SellingPrice int json:"sellingPrice"` (0 = même prix que Price).
- `HotspotUser` gagne : `SellingPrice int json:"sellingPrice"` (copié du profil à la
  génération ; affichage voucher `{{price}}` = sellingPrice || price).
- `Sale` gagne : `Cost int json:"cost"` (= price×count), `SellingTotal int json:"selling"`
  (= (sellingPrice||price)×count). **`Amount` garde sa sémantique actuelle** (= price×count).

### Rapports
- `GET /api/reports` (réponse étendue, rétro-compatible) gagne :
```json
"margin": {
  "revenue": 0, "cost": 0, "margin": 0, "marginPct": 0,
  "byProfile": [{ "name": "", "sold": 0, "revenue": 0, "cost": 0, "margin": 0 }]
}
```
  (période = 30 jours glissants, cohérente avec les autres blocs.)
- `GET /api/accounting/export` : 2 colonnes ajoutées `Coût (FCFA)` et `Marge (FCFA)`.

---

## F11/F12 — i18n FR/EN & Quick print [P2, FRONTEND UNIQUEMENT]

### i18n (approche maison légère — next-intl n'est PAS utilisé)
- `src/lib/hotspot/i18n.ts` : dictionnaire `fr` (existant, extraction) + `en` (traduction),
  ~250 clés aplaties `nav.dashboard`, `users.title`…
  > N°87 — éclatement : les dictionnaires vivent désormais en fragments par domaine
  > (`src/lib/hotspot/i18n-fr/<domaine>.ts` et `i18n-en/<domaine>.ts`, 45 domaines,
  > 2 460 clés chacun) fusionnés par les agrégateurs `i18n.ts` / `i18n-en.ts` —
  > API publique, règles de résolution et lazy-load EN (N°78) inchangés.
- Hook `useI18n()` : langue depuis `useHotspotStore` (nouveau champ `lang` persisté
  localStorage via zustand persist — attention à ne pas casser l'existant).
- Sélecteur : carte « Langue » dans Paramètres (RadioGroup FR/EN) — langue par défaut : fr.
- TOUTES les chaînes visibles des vues/shell/dialogues/toasts passent par `t()`.
  Les données serveur (noms, messages d'erreur API) restent telles quelles.

### Quick print
- `localStorage "mikcloud-last-batch"` = dernier batchId imprimé (écrit à l'impression).
- Vouchers view : bouton header « Réimpression rapide » (icône Zap) → ouvre directement
  le dialog d'impression du dernier lot (fetch batch vouchers + print).

---

## PARITÉ MIKHMON — User Profiles & Generate (extension post-audit)

> Suite de l'audit : alignement fin des formulaires Profile / Generate sur Mikhmon v3
> (adduserprofile.php + generateuser.php). Extensions ADDITIVES uniquement — le
> contrat V2 ci-dessus reste valable tel quel (expiresAt, reports, templates).

### Modèle `Profile` (ajouts)
```go
AddressPool string `json:"addressPool"` // nom RouterOS /ip pool ("" = none au routeur)
ParentQueue string `json:"parentQueue"` // queue simple /queue simple ("" = none)
ValidityMin int    `json:"validityMin"` // validité fine en minutes (0 = hériter validityDays×1440)
```
- **Source de vérité validité** : `Profile.ValidityMinutes()` = validityMin si > 0,
  sinon validityDays×1440. `expiresAt` (contrat V2) est TOUJOURS calculé via
  `ValidityMinutes()` — posé au PREMIER LOGIN (ancrage, vide avant), extension F4
  (voucher déjà connecté), recalcul au changement de profil (voucher connecté) ;
  `validityDays` reste renseigné (arrondi supérieur : (validityMin+1439)/1440) pour
  rétro-compatibilité.
- Validations create/update : validityMin ∈ [0, 2628000] ; addressPool/parentQueue
  = noms RouterOS transmis tels quels (TrimSpace) ; synchronisés sur TOUS les
  routeurs agents à chaque user_add/voucher_batch via profile_set (`none` si vide).
- expMode `none` : voir F1.
- Neon : colonnes `profiles.address_pool`, `profiles.parent_queue`,
  `profiles.validity_min` (ALTER TABLE idempotents au boot).

### Génération de vouchers (extension `POST /api/vouchers/generate`)
```go
TimeLimitMin int    `json:"timeLimitMin"` // limit-uptime PAR LOT (≤0/omis = sessionTimeoutMin du profil)
Server       string `json:"server"`       // serveur hotspot RouterOS cible (""/omis = all)
```
- `timeLimitMin` ∈ [0, 2628000], tracé sur `HotspotUser.timeLimitMin` ET
  `batches.time_limit_min` ; poussé au routeur (`limit-uptime`), priorité payload >
  profil. Le correctif rétroactif (`profileUserLimitLine`) ne cible QUE les users
  `limit-uptime=0s` — les quotas par lot ne sont jamais écrasés.
- `server` : ≤ 64 chars sans guillemets/retours, poussé tel quel (`server=` ; « all »
  = omis au routeur, décision routeur par défaut).
- Validations alignées Mikhmon : `codeLength` ∈ [3,10] (min abaissé 4→3) ; `prefix`
  ≤ 6 chars, OPTIONNEL — vide = AUCUN préfixe (le ticket porte le code généré seul,
  pas de valeur par défaut ; correctif post-test live, l'ancien fallback "SC-" est
  supprimé) ; charset preset `num` (chiffres purs, alphabet digitSafe
  sans 0/1 — plus lisible à l'impression que 0-9 Mikhmon). Pas de confusion avec la
  variable template `{{num}}` (n° de voucher).
- `{{timeLimit}}` des templates reflète le quota propre du voucher.

### Frontend
- `src/lib/hotspot/use-router-resources.ts` (hook partagé) : agrège
  `GET /api/routers/{id}/resources` en décompactant l'enveloppe F9 `{queued, data,
  updatedAt}` (re-poll 5 s tant qu'un check-in agent est attendu, sinon 15 s ;
  fusion dédupliquée multi-routeurs ; routeurs `real` exclus — cf. matrice §0).
  Retourne aussi `queued`/`updatedAt` pour l'état « en attente ».
- `profile-dialog.tsx` : sélecteur « **Charger depuis un routeur** »
  (« Tous les routeurs (fusion) » par défaut, sinon un routeur non réel précis →
  les datalists reflètent les valeurs RÉELLES de ce MikroTik) + bannière d'attente
  si la commande read_resources est en file ; validité valeur+unité
  (min/h/j/semaines) avec aperçu RouterOS `fmtRouterDuration()` (ex. `5h30m`,
  `4w3d`) ; datalists Address Pool / Parent Queue (saisie libre conservée —
  profils MikCloud multi-routeurs, Mikhmon est mono-routeur) ; nom de profil
  auto-formaté Mikhmon (espaces → tirets).
- `vouchers-view.tsx` : sélecteur Server (routeur sélectionné, « all » = omis),
  Time Limit par lot (hériter/illimité/presets), charset num, récap GetValidPrice
  (Validité RouterOS / Prix de vente / Verrou 1er appareil / Expired Mode).

---

## N°18 — Transfert de stock : redistribution des lots déjà générés (gérant/propriétaire)

Maillon manquant du circuit de distribution : l'attribution revendeur n'existait qu'à la
GÉNÉRATION (`channel=reseller`). Un lot « direct » généré à l'avance ne pouvait pas être
remis à un revendeur après coup (seule option : re-générer → doublons routeur).

### Endpoint

`POST /api/vouchers/batch/{batchId}/transfer` — `requireRole(2)` (gérant, propriétaire,
super-admin plateforme consulté), `guardAccountWrite` (compte expiré → refus).

Corps : `{ "resellerId": "<id>" | "direct", "quantity": number?, "excludeExpiringDays": number? }`

Réponse : `{ "transferred": int, "debited": int, "credited": int, "creditAfter": int,
"refunds": [{resellerId, resellerName, amount, creditAfter}], "vouchers": HotspotUser[] }`
— `vouchers` = tickets transférés (impression A4 immédiate côté front).

### Règles d'or

1. **Changement de propriété, jamais duplication** : seuls `ResellerID/ResellerName`
   sont mutés — zéro write RouterOS (fonctionne même routeur hors ligne).
2. **Seul le stock VENDABLE part** : statut effectif `active` (`EffectiveStatus`) et
   `soldAt` vide. Un ticket remis à un client ne bouge plus (anti-fraude) ; used/expired/
   disabled restent dans leur attribution d'origine (audit). Les tickets déjà chez la
   destination sont ignorés (no-op).
3. **Transfert partiel** : `quantity ≤ transférable` ; rotation « plus récemment généré
   en premier » (les vieux restent au comptoir). `quantity` 0/omis = tout.
4. **L'argent suit le transfert** : entrée chez un revendeur = débit du portefeuille du
   prix facial (u.Price, cohérent avec la génération) + `Transaction` type `sale` ;
   sortie d'un revendeur = recrédit + `Transaction` type `credit` (retour de stock ;
   ré-affectation A→B combine les deux). Solde insuffisant → 400 (même règle que la
   génération). **AUCUNE ligne `Sale` créée** : les Sales restent liés à la génération —
   dashboard/rapports/compta ne double-comptent pas.
5. **Traçabilité** : `Activity` horodatée avec l'acteur (« Transfert du lot … » /
   « Retour de stock du lot … ») + une Transaction par mouvement.

Garde-fou expiration : `excludeExpiringDays>0` exclut les tickets dont `expiresAt` ANCRÉ
tombe dans la fenêtre (tickets reconnectés une fois puis ré-activés — `usedAt` remis à
« » — ou importés avec échéance). Les tickets frais (non ancrés, `expiresAt` vide) ne
PEUVENT pas expirer en stock : leur validité démarre au premier login — rien à exclure.

### Lot immuable + possession live

`Batch.Channel/ResellerID` décrivent la GÉNÉRATION (provenance) et ne sont JAMAIS mutés.
`GET /api/vouchers/batches` enrichit chaque lot de champs recalculés à la lecture depuis
les vouchers : `transferable` (stock vendable), `transferableValue` (valeur faciale),
`expiring7d` (transférables à échéance ancrée ≤ 7 j), `holdings[]`
(`{resellerId, name, count, value}` — `resellerId: ""` = stock direct ; tri direct
d'abord puis quantité décroissante). Le front affiche « Chez : … » quand la possession
diverge de la provenance, et plafonne le transfert à `transferable − déjà chez la cible`.

### Refonte onglet Lots — « fiche de vie » (ADDITIF, contrat liste intact)

`GET /api/vouchers/batches` gagne un champ `status` par lot (cycle de vie DÉRIVÉ des
stats live, jamais stocké) : `stock` (du consommable reste : active>0) → `consumed`
(épuisé : used>0 ou disabled>0, plus de stock) → `expired` (jamais utilisé, validité
envolée) → `purged` (plus rien en base). Nouveaux filtres query (tous optionnels,
valeurs `all`/vide = ignorés) : `status` (une des 4 valeurs ci-dessus), `channel`
(`direct|reseller` — PROVENANCE, pas la possession), `holder` (détenteur LIVE du
stock vendable : `direct` ou l'ID du revendeur — un lot sans stock vendable
n'appartient à personne). La réponse gagne `summary` (totaux sur l'ensemble FILTRÉ,
avant pagination) : `{batches, stockTickets, transferable, stockValue, expiring7d}` —
le gérant filtre par détenteur et lit « son » stock. Le lot porte aussi
`DataQuotaMb/TimeLimitMin` (hérités de la génération) déjà exposés.

`GET /api/vouchers/batches/export` (requireRole 2) — export CSV des lots, MÊMES
filtres que la liste, sans pagination : séparateur `;` + BOM UTF-8 (convention
`mikcloud-comptabilite`), une ligne par lot — cycle de vie, stock vendable,
détail des statuts et possession (`Stock direct (13) · David (6)`).

### V2 onglet Lots — « tour de contrôle du stock » (ADDITIF)

Vocabulaire métier unifié (front) : `used`+`disabled` = « éculés » (vendus/consommés —
du chiffre d'affaires, jamais une perte) ; `expired` = « expirés » (perte sèche) ;
vendable = actif jamais remis au client.

Filtre `holder` gagne la valeur `resellers` (n'importe quel revendeur — pipeline).
Chaque lot gagne (ADDITIF, calculés à la lecture, jamais stockés) :
`sold7d` (sorties de stock sur 7 j glissants — vente `SoldAt` OU consommation
`UsedAt`, un ticket ne compte qu'UNE fois), `lastEgressAt` (dernier mouvement de
sortie), `dormantDays` (jours depuis la dernière sortie, sinon création du lot —
rempli uniquement si `transferable > 0`, seuil d'alerte front 7 j), `stockFace`
(valeur faciale du vendable, Σ prix public avec repli `price`), `marginPending`
(marge en attente = face − gros, jamais négative).
`summary` gagne : `resellerStock` + `resellerStockValue` (vendables détenus par
des revendeurs), `sold7d`, `stockFace`, `marginPending` (totaux sur l'ensemble
FILTRÉ).
CSV : 3 colonnes additives en queue d'en-tête avant `Possession` —
`Ecoules 7j ;Dormance j ;Marge en attente`.

---

## N°19 — Modes de paiement revendeur : prépayé / dépôt-vente (vend puis verse)

Deux modes cohabitent **PAR revendeur** (`Reseller.PaymentMode`, défaut `prepaid` — zéro changement pour les existants) :

- **prepaid** (historique) : le crédit est débité À LA PRISE de stock ; vente reconnue à la génération (Sale + Transaction `sale`).
- **deposit** (dépôt-vente) : la prise de stock (génération **et** transfert N°18) est GRATUITE et bornée par le **plafond de créance**
  (`Reseller.DebtCeiling` > 0 obligatoire) : `dette + stock à crédit + nouveau stock ≤ plafond`, sinon 400 « Plafond de créance dépassé ».
  La créance naît à la REMISE au client (`POST /api/sell/:id/sold` → Transaction `debt`, prix gros `u.Price`) et se règle par
  `POST /api/resellers/:id/settle {amount, note?}` (requireRole 2) → Transaction `settlement` + ligne Sale (reconnaissance à l'encaissement).

Règles comptables : (1) une vente = UNE écriture — en dépôt-vente, AUCUNE écriture à la génération ni au transfert (le dashboard,
les rapports et la compta consomment `db.Sales` sans double-compter) ; (2) le marqueur par-voucher `HotspotUser.CreditSale` est posé
à chaque attribution selon le mode de la destination et SURVIT aux changements de mode — seul le stock pris à crédit crée une créance.
Anti-vol ACTIF : en dépôt-vente, `dette > plafond` bloque le Mode Vente (403 « versement requis ») jusqu'au versement.
Bascule de mode : prépayé → dépôt-vente exige un plafond ; dépôt-vente → prépayé exige une dette soldée.
Réponses enrichies : liste revendeurs `+debt, settlementsCount, lastSettlementAt` ; `/api/sell/me` `+paymentMode, debt, debtCeiling`.

**V2 — visibilité & recouvrement** : `GET /api/dashboard` `+receivables` `{totalDebt, count, items[] {resellerId, name, debt, ceiling, agingDays, level ok/warn(≥7 j)/danger(≥30 j), overCeiling}}` — widget « Créances revendeurs » (visible si count > 0).
`GET /api/sell/day-report` `+paymentMode, toDeposit` (cash du jour à verser), `+debtTotal` — bannière amber dans le rapport + lignes du texte partagé.
Reçu de versement partageable (WhatsApp/presse-papiers) après encaissement ; indication de confiance (≥ 3 versements, dette soldée → suggérer d'augmenter le plafond).
Migration : ALTER idempotents au boot (`resellers.payment_mode`, `resellers.debt_ceiling`, `hotspot_users.credit_sale`).

---

## UX R3/R4 — Mode Vente : vente papier tracée + vente automatique à la connexion

- `POST /api/sell/:id/sold` gagne un **corps optionnel** `{"via":"paper"}` — additif et
  rétrocompatible (les PWA installées POSTent sans corps) : `via:"paper"` → trace
  `SoldVia="sell_mode_paper"` (ticket papier imprimé vendu par saisie du code) ;
  défaut (tactile) inchangé `SoldVia="sell_mode"`.
- **Vente automatique à la 1ʳᵉ connexion** : au premier login hotspot détecté d'un
  voucher du stock revendeur (`ResellerID != ""`, jamais vendu, non désactivé), la
  vente est tracée automatiquement — `SoldAt=now`, `SoldVia="auto_connect"`, créance
  dépôt-vente au prix gros (Transaction `debt`, règle N°19) + entrée Activity. Le
  revendeur n'a plus rien à taper : il remet le ticket, le client se connecte, le
  stock se décompte et le rapport de journée se met à jour (poll 30 s).
- **Idempotence & refus** : un voucher déjà vendu (tactile, papier ou auto) n'est
  jamais recompté (`409` « déjà remis ») ; `POST /sold` refuse désormais explicitement
  un voucher expiré ou consommé (`409` « Voucher expiré ou consommé » — durcissement :
  entre l'affichage du stock et la confirmation, la validité peut tomber) ; le retour
  de stock N°20 refuse de même tout ticket vendu, expiré ou consommé (inchangé).

## B2 — Core Web Vitals « Speed App UX » (télémétrie RUM, 2026-09-04)

Mesure de la latence RÉELLE perçue par les usagers (vitrine anonyme incluse)
pour piloter les optimisations et arbitrer l'autosuspend Neon (Phase C).
Isolée du store métier : `internal/telemetry` (ring mémoire + Neon par lots
asynchrones) — aucun impact sur `model.DB` ni la synchro différentielle.

### POST /api/vitals — PUBLIC (whitelisté, beacon)

- **Entrée** : `text/plain` contenant du JSON (requête simple → JAMAIS de
  preflight CORS), envoyé par `navigator.sendBeacon` (fallback fetch keepalive) :
  ```json
  { "path": "/app/users", "sid": "a1b2c3d4", "nav": "navigate",
    "metrics": [ { "name": "LCP", "value": 2340.5, "rating": "good" } ] }
  ```
  - `name` ∈ `LCP | CLS | INP | FCP | TTFB` ; `value` ≥ 0, fini, ≤ 600 000
    (ms ; CLS sans unité) ; `rating` ∈ `good | needs-improvement | poor | ""` ;
  - `path` ≤ 128, commence par `/` ; `sid` ≤ 64 (session de mesure
    sessionStorage — sans cookie ni PII) ; `nav` ∈ navigate / reload /
    back-forward / back-forward-cache / prerender / restore / "" ;
  - ≤ 8 métriques par requête ; corps ≤ 8 Kio → `400` sinon ;
- **Sortie** : `204 No Content` (le beacon ne lit jamais la réponse) ;
- **Sécurité** : whitelist publique dans le middleware d'auth (la vitrine
  anonyme est précisément la page à mesurer), couverture par le limiteur
  général (120/min/IP + plafond global 900/min) ; IP et User-Agent bruts
  JAMAIS stockés (IP = limiteur mémoire uniquement ; `device` = hint
  `mobile|desktop` dérivé serveur du UA).

### GET /api/vitals/summary — plateforme uniquement (isPlatformAdmin)

- `?window=heures` (1-168, défaut 24) ;
- Sortie :
  ```json
  { "window": 24, "samples": 1234,
    "metrics": { "LCP": { "n": 300, "p50": 2100.4, "p75": 3300.1, "p95": 5200.0,
                          "good": 140, "needsImprovement": 90, "poor": 70 } },
    "paths":   [ { "path": "/app/dashboard", "n": 210,
                   "p75": { "LCP": 3100.2, "INP": 180.5, "TTFB": 640.0 } } ],
    "devices": { "mobile": { "n": 980, "p75": { "LCP": 3400.0, "TTFB": 720.0 } } } }
  ```
- **Déduplication** : web-vitals re-rapporte LCP/CLS/INP quand la valeur
  évolue ; seuls les DERNIERS rapports par (sid, path, métrique) comptent
  dans les agrégats (recommandation Google pour les p75) ; quantiles
  « nearest-rank » ; p75 par groupe omis sous 5 échantillons.

### Stockage (`internal/telemetry/vitals.go`)

- **Mémoire** : ring 20 000 échantillons — source des agrégats ; historique
  rechargé depuis Neon au boot (48 h max) → les agrégats survivent aux
  redéploiements Render ;
- **Neon** : table `web_vitals` (`id BIGSERIAL`, `sampled_at TIMESTAMPTZ`,
  `metric`, `value DOUBLE PRECISION`, `rating`, `path`, `device`, `nav`,
  `sid` + index `sampled_at DESC`) — DDL idempotente en tâche de fond 45 s
  après boot (le démarrage n'attend JAMAIS Neon) ; insertions par LOT
  asynchrones (10 min ou 200 échantillons, file bornée 2 000, pertes
  loguées) — un incident Neon n'impacte jamais une requête API ;
- **Frontend** : `src/components/perf/vitals-reporter.tsx` monté dans le
  layout racine (mesure /, /login, /app, /sell) ; `web-vitals` (~1,5 Ko
  gzip) importé dynamiquement à l'idle — hors du chemin critique qu'il
  mesure ; `sid` en sessionStorage.

---

## PURGE & RÉSURGENCE — tombstones anti-ré-import, réglage d'import auto, purge totale (2026-09-04)

**Problème** (audit) : la purge admin supprime du cloud (mémoire + Neon) mais
PAS des routeurs réels — le read_state agent (≤ 45 s) ré-importait tout ce que
le routeur garde encore : résurgence, et un voucher purgé revenait en
« utilisateur régulier » fantôme (sans lot ni vente).

**Tombstones** : la purge pose un marqueur par username purgé (minuscules,
compte, TTL 30 jours). `applyReadState` refuse de ré-importer (ni user, ni
session, ni journal, ni vente auto) un username tombstoné et ne le compte PAS
comme « inconnu ». Levée : expiration (30 j) OU création volontaire du même
username dans MikCloud. Annulation des commandes EN FILE qui recréeraient les
entités purgées (`user_add` du nom, `voucher_batch` du compte) ; celles déjà
envoyées restent — le tombstone bloque leur effet au read_state suivant.
Table Neon `purge_tombstones` (id, account_id, username, purged_at, expires_at).

**POST /api/admin/purge** — requête enrichie :
- `alsoRouter` (bool, défaut false) : purge TOTALE — enfile des commandes
  `user_remove` (lots de 50, payload `{"names":[…]}`) pour les routeurs AGENT
  qui détenaient les comptes purgés. Les routeurs REAL (passerelle) ne sont
  pas commandables après purge (les identifiants cloud sont effacés) : leurs
  comptes restants restent visibles via `unknownOnRouter`.
- `confirm` (string) : EXIGÉ égal à `"SUPPRIMER"` si `alsoRouter=true`
  (400 sinon) — double garde client + serveur.
Réponse : `purged.tombstones` (marqueurs posés) et `purged.routerRemovals`
(comptes commandés en suppression routeur).

**Settings compte** — `autoImportRouterUsers` (bool, défaut ON, accepté plat
et dans `tenant{…}` via PUT /api/settings) : ON = les comptes créés hors
MikCloud (Winbox…) sont importés automatiquement au read_state (comportement
historique) ; OFF = jamais importés — comptés dans le NOUVEAU champ volatile
de l'objet routeur `unknownOnRouter` (adoptables via l'outil d'import).
Colonne Neon `settings.auto_import_router_users` (NOT NULL DEFAULT TRUE).

**Front** : avertissement purge quand `realRouters > 0` (les données ne
disparaissent pas des routeurs ; ré-import bloqué 30 j), option purge totale
avec case + saisie « SUPPRIMER », toast bilan enrichi (tombstones,
routerRemovals), interrupteur d'import auto dans les réglages du compte,
badge discret « N hors MikCloud » sur la carte routeur.

**Tests** : `purge_tombstones_test.go` (tombstones posés, blocage read_state,
expiration, levée, import auto OFF, annulation de file, purge totale 400/200).

---

## N°26 — Lots éteints : disparition automatique (lot tous-expirés)

Sweep `sweepDeadBatches(db)` (api, appelé en fin d'`enforceExpired` — donc
après `applyExpiry`, sous verrou, persisté par le `Save` de chaque appelant :
dashboard, listes users/vouchers, ops, check-in agent 45 s).

Critère (par lot, vouchers uniquement) : ≥ 1 ticket ET tous les tickets
présents `status == "expired"` ET aucun ticket avec `ResellerID != ""`
(N°23/W1). Effets : suppression des tickets, de la ligne `db.Batches`, des
sessions associées ; tombstones de purge (TTL 30 j) pour chaque username ;
`CmdUserRemove` (paquets de 50) pour chaque routeur AGENT détenteur ;
entrée d'activité par compte. Ventes/transactions/journaux INTACTS.
Aucun changement de contrat API ni de schéma — les listes reflètent la
disparition (un lot éteint n'apparaît plus, même sous `status=tous`).

## N°27 — Inscriptions publiques par QR (campus/écoles/administration/entreprise)

Deux nouvelles entités (tables `join_links`, `registration_requests` —
migration purement additive, aucune colonne ajoutée à `hotspot_users`) :

**Liens d'invitation (console, rôle 2)** — `GET /api/join-links` →
`{items:[JoinLinkView]}` ; `POST /api/join-links {name, profileId?, routerId?,
autoValidate, maxUses, expiresAt?}` (autoValidate exige profil+routeur ;
maxUses 0 = illimité) ; `PUT /api/join-links/{id} {revoked}` (révocable
INSTANTANÉMENT) ; `DELETE /api/join-links/{id}`. `JoinLinkView` = lien +
`state` dérivé (`active|revoked|expired|exhausted` — expired/exhausted sont
des états calculés, pas stockés). Token = `RandomCode(32)` (alphabet sans
ambiguïtés, ~155 bits), stocké côté serveur : le lien FAIT l'authentification
de la page publique.

**Page publique (whitelist middleware `/api/join/` — PAS `/api/join-links`)** :
- `GET /api/join/{token}` → `{name, organization, state, expiresAt?,
  remaining?, autoValidate?, profileName?}` — minimal : JAMAIS le catalogue
  de profils. 404 `join_link_unknown` si token inconnu.
- `POST /api/join/{token}` `{fullName, phone, username, password, message?,
  website?}` — `website` = honeypot (réponse 200 factice, rien n'est créé).
  Quotas : rate-limit « join » 10/min/IP (main.go) + `signupLimiter` réutilisé
  sur la soumission (5/10 min, 20/24 h — 429 + Retry-After). Validations :
  nom 2–80, téléphone normalisé 8–15 chiffres (dédoublonné contre les
  demandes pending : 409 `phone_pending`), username 3–32 `[A-Za-z0-9._-]`
  (409 `username_taken` + `suggestion`), mot de passe 6–64. 200 →
  `{status:"pending"}` ou, lien kiosque, `{status:"approved", username,
  password, queued?}` (création immédiate via `createHotspotUser`).

**File de validation (console, rôle 2)** — `GET /api/registrations?status=`
→ `{counts:{pending,approved,rejected}, items:[RegistrationRequest]}` (max
300, tri desc) ; `POST /api/registrations/{id}/approve
{profileId, routerId, username, password?}` (password vide → généré ; 409
`username_taken`+`suggestion` si repris entre-temps ; 409 « Demande déjà
traitée ») → `{request, user, queued?, commandId?}` ; `POST
/…/{id}/reject {reason}` (1–300 requis) ; `DELETE /api/registrations/{id}`.
L'approbation passe par `createHotspotUser` (cœur extrait de
`handleUserCreate`) : kind `regular`, validité = maintenant + validité du
profil (démarre À L'APPROBATION), file agent `user_add`, tombstone levé,
journal. Mode de connexion « Nom d'utilisateur & Mot de passe » (codes
distincts au choix) — distinct des vouchers (N°25). Minimisation : le mot de
passe de la demande est VIDÉ à l'approbation comme au refus ; les demandes
refusées sont purgées à 30 jours (`sweepStaleRegistrations`, hook
`enforceExpired`). `RegistrationRequest.password` n'est non-vide QUE pour
les demandes pending.

**Tests** : `handlers_join_test.go` (cycle complet console↔public,
garde-fous de lien, validations + honeypot, kiosque + file agent, scoping
inter-comptes, sweep 30 j).

---

## N°50 — WiFi jetable : garde-fous anti-abus du claim (appareil + honeypot + IP)

### Modèle (ajouts)
```go
WifiSite.DailyPerMac int    // N°50 — tickets max / appareil (MAC) / jour (1–10, défaut 1)
WifiGuest.Mac string        // N°50 — MAC normalisée du claim portail (audit + plafond)
WifiGuest.IP string         // N°50 — IP client (premier hop XFF, audit)
```
Migration boot idempotente : `wifi_sites.daily_per_mac INTEGER NOT NULL DEFAULT 1`,
`wifi_guests.mac TEXT`, `wifi_guests.ip TEXT` (mécanique N°47/N°49).

### Règles du claim (POST /api/wifi/site/{slug}/claim)
1. **Quota anti-fermage IP** (limiter dédié, NAT-friendly : 20/10 min +
   100/24 h, `newSignupLimiterLimits`) — consommé par toute tentative ;
   429 + `Retry-After` au seuil.
2. **Honeypot `website`** — champ invisible (page /wifi ET formulaire inline
   du portail) : rempli ⇒ succès FACTICE 200 (même forme JSON, code
   aléatoire jamais créé, aucune écriture) — aucun indice sur le filtre.
3. **Plafond par appareil** — `mac` (normalisée via `normalizeJoinMac`,
   injectée par le portail `$(mac-esc)`) : comptée sur le registre du jour
   du site ; `>= DailyPerMac` ⇒ 429 `device_cap`. Positionné APRÈS
   l'idempotence téléphone (le re-claim du même numéro renvoie le même
   code, plafond atteint ou non) et AVANT `phone_cap`. Claims sans MAC
   (page /wifi scannée hors portail) ou MAC invalide : plafond ignoré.
4. Plafonds métier inchangés : `phone_cap` (téléphone/jour) puis
   `site_cap` (budget site/jour).
5. Empreintes `Mac`/`IP` stockées dans `WifiGuest` + colonnes
   « appareil »/« ip » de l'export CSV (audit gérant).

### Payload console (create/update site)
`dailyPerMac` (int 1–10, 0 ⇒ 1). Champs UI : « Tickets max / appareil /
jour » + hint (i18n fr/en). Registre JSON : `mac`/`ip` optionnels (vides
pour les lignes antérieures au N°50).

**Tests** : `TestWifiClaimHoneypot` (succès factice sans écriture puis
claim honnête OK), `TestWifiClaimDeviceCap` (plafond 429 `device_cap`,
idempotence prioritaire, sans MAC / MAC invalide, empreintes tracées).

---

## N°29 — Walled-garden d'inscription publique automatisé par l'agent (runbook N°27-D)

Objectif : le scan du QR `/join/{token}` fonctionne depuis le WiFi du hotspot SANS
configuration manuelle des routeurs — le runbook `docs/RUNBOOK-WALLED-GARDEN.md`
est appliqué par le système lui-même, pour les routeurs neufs ET ceux déjà en ligne.

- **Nouvelle commande agent `walled_garden`** (kind `CmdWalledGarden`, payload
  `{domains: […], sig: …}`) : script idempotent — remplace UNIQUEMENT les règles
  portant le commentaire-marqueur `mikcloud-wg` (les règles personnelles du
  gérant sont préservées), pose une règle `allow` par domaine
  (`/ip hotspot walled-garden add action=allow dst-host=…`) + 2 règles DNS
  udp/tcp 53 (`walled-garden ip`), et se rapporte avec `domains=N`.
- **Domaines calculés par le déploiement** (`walledGardenDomains`) : hôte API
  (`MIKCLOUD_BASE_URL` sinon hôte de la requête agent), origine page
  (`APP_PUBLIC_URL` + `ALLOWED_ORIGIN` — les origines CORS sont exactement les
  origines navigateur à autoriser), hôte de la requête courante ; dé-dupliqués,
  triés (signature stable), 10 max, assainis (`agent.SanitizeWGDomain` :
  `[a-z0-9._-]` + port numérique, 253 car. max).
- **Routeurs DÉJÀ EN LIGNE** : à chaque check-in (`handleAgentCmd`),
  `ensureWalledGardenLocked` compare la signature de la configuration courante à
  `Router.WalledGardenSig` (nouvelle colonne `routers.walled_garden_sig`) —
  différente et aucune commande en vol → mise en file, servie dans CE check-in.
  La signature n'est posée qu'au retour « ok » (`handleAgentResult`) : un échec
  est retenté au check-in suivant, un changement de config re-file
  automatiquement. Un parc en ligne se met à niveau tout seul en ≤ 45 s.
- **Routeurs neufs** : le script d'INSTALLATION intègre le même bloc (généré
  par `walledGardenInstallBlock`, corps multi-lignes — règle du parseur
  console) ; les mises à jour ultérieures passent par le check-in, pas par un
  recollage.
- Journal : « Walled-garden d'inscription publique appliqué sur … ».
- Tests : `TestSanitizeWGDomain` (anti-injection), `TestWalledGardenScript`
  (règles + rapport `domains=N` + injection neutralisée),
  `TestWalledGardenInstallBlock` (multi-lignes, absent si aucun domaine),
  `TestNormalizeWGHost`, `TestWalledGardenDomains` (tri/dé-dup),
  `TestEnsureWalledGardenLocked` (exactly-once, re-file sur changement, retry
  sur échec).

### N°49 — walled-garden AUTO-RÉPARANT + réparation à la demande (addendum au N°29/N°48)

Contexte : le N°48 (v2, sel `wg-v2-api`) a couvert le HTTPS (règles ip « api »)
et réparé les routeurs existants UNE fois. Constat prod CyberSC : une liste
vidée/amputée LOCALEMENT après coup (ménage Mikhmon, restauration, script
partiel) repassait en panne silencieuse — la sig posée bloquant tout re-file.

- **Nouveau champ `Router.WalledGardenAppliedAt`** (RFC3339, colonne
  `routers.walled_garden_applied_at`, DDL idempotent) : posé avec la signature
  au retour « ok » de la commande (`handleAgentResult`), jamais à la mise en
  file.
- **Re-file élargi (`ensureWalledGardenLocked`)** : la commande est (re)filée
  si la sig diffère (N°29, sel N°48) OU si la sig est identique mais
  `walledGardenFresh(router)` est faux : horodatage ABSENT (routeur configuré
  avant le N°49 — réparation immédiate au premier check-in) ou plus vieux que
  `walledGardenRefresh` = 6 h (réparation périodique d'une liste vidée
  localement). Aucune commande en vol reste prioritaire (pas de doublon).
- **Nouvel endpoint console** : `POST /api/routers/{id}/repair-walled-garden`
  → `{ok, message}` ; 404 routeur inconnu, 400 `not_agent` (modes
  simulated/real), 402 `subscription_expired`. Vide sig + horodatage → re-file
  au check-in suivant (≤ 45 s). Bouton « Réparer le walled-garden » dans le
  menu d'un routeur agent (vue Routeurs).
- Tests : `TestEnsureWalledGardenAutoRepair` (trois leviers),
  `TestBuildWalledGardenCoversBothTables` (page host + api ip, commande +
  installation, removes compris),
  `TestRepairWalledGardenOK/NotFound/NotAgent`.

---

## N°53 — Stockage média Cloudflare R2 (images du gérant)

### Principe
Les images du gérant (bannière du portail N°45, promos hospitalité à venir)
quittent la base (data URL ≤ 500 Ko) pour un stockage objet durable :
compartiment R2 **mikcloud-media**. Canal retenu : **API REST Cloudflare**
(Bearer `R2_API_TOKEN`), PAS l'API S3 — zéro SDK Go, un seul secret, volumes
minuscules (≤ 2 Mo).

### Env (Render, backend)
`R2_ACCOUNT_ID` (compte Cloudflare), `R2_API_TOKEN` (jeton API R2:Edit),
`R2_BUCKET` (défaut `mikcloud-media`). Non configuré ⇒ upload 503
`media_unconfigured` (le frontend retombe sur la data URL N°45) et lecture 404.

### Routes
- `POST /api/media` (auth gérant `requireRole(3)`, multipart `file`) —
  type SNIFFÉ dans le contenu (`http.DetectContentType` : jpg/png/webp/gif),
  ≤ 2 Mo (`MaxBytesReader`), clé `media/{compte}/{année}/{hex32}.{ext}`
  (128 bits), PUT REST, réponse 201 `{url, key, size, type}`.
- `GET /api/media/{key...}` (PUBLIC, limiter « api » 120/min/IP) — clé
  validée par regex stricte (aucune traversal, uniquement des objets déposés
  par l'upload authentifié), Content-Type déduit de l'extension (jamais des
  en-têtes stockés), `X-Content-Type-Options: nosniff`,
  `Cache-Control: public, max-age=31536000, immutable` (clé unique à jamais),
  404 typé si l'objet n'existe pas.

### Walled-garden (portail captif)
Les URL renvoyées sont sur le MÊME hôte que `apiBase`
(`https://mikcloud.onrender.com/api/media/…`) : déjà joignable pré-auth
(claim N°47, fetch live N°48) — AUCUNE entrée walled-garden nouvelle.

### Console (bannière, settings-view)
Le bouton « Téléverser » envoie l'image vers R2 (`apiUpload` multipart,
≤ 2 Mo, i18n fr/en) et remplit le champ avec l'URL permanente ; repli
dégradé automatique : stockage indisponible ⇒ data URL intégrée ≤ 500 Ko
(contrat N°45 inchangé), sinon message d'erreur. `bannerUrl` reste la
seule donnée persistée — aucun changement de schéma, aucune migration Neon.

## N°55 — Mode hospitalité du portail (vitrine de l'établissement)

### Modèle (ajouts Tenant)
```go
Tenant.PortalStyle  string // "" | "commercial" | "hospitality" ("" = défaut commercial)
Tenant.PortalWelcome string // message de bienvenue (≤ 200 car.)
Tenant.PortalPromos string // JSON [{title,desc,imageUrl,priceLabel}] ≤ 6 (validé API)
Tenant.PortalSocials string // JSON [{label,url}] ≤ 4 (https, validé API)
```
Migration boot idempotente : `settings.portal_style`, `portal_welcome`,
`portal_promos`, `portal_socials` (TEXT NOT NULL DEFAULT '').

### Règles
- `PUT /api/settings` : reçoit `portalStyle` (string), `portalWelcome`
  (string), `portalPromos` ([]{title,desc,imageUrl,priceLabel}),
  `portalSocials` ([]{label,url}) — plats + nested `tenant{…}`, plat prime.
  Validation serveur stricte puis sérialisation JSON ; client ne contrôle que
  ces champs. Liste vide = retrait. `nil` = inchangé.
- `PortalConfig` (fallback inliné + fetch live) : champs `portalStyle`,
  `portalWelcome`, `portalPromos[]`, `portalSocials[]` — décodage tolérant
  (`portalHospitality`, JSON invalide ⇒ listes vides).
- Portail `login.html` : `applyHospitality(cfg)` — hospitality = masquage par
  classe (`mikcloud-hosp-hidden`) de slider/grille/Wave/services/offers +
  injection vitrine (bienvenue, promos avec images R2, socials) ; commercial =
  retrait des injections et dé-masquage (réversible, idempotent).

## N°56 — Analytics du portail (impressions / clics par promo)

### Modèle
```go
Tenant.PortalKey  string // clé publique du portail (16 hex, générée ensureSettings)
model.PromoEvent  // {ID, AccountID, PromoID, Kind, ClientKey, Day, CreatedAt}
db.PromoEvents    // []PromoEvent — journal borné (rétention 90 j, ≤ 12 000 lignes)
```
Migration boot idempotente : table `promo_events` (PK `id`, index
`account_id,day` + `account_id,promo_id`) + colonne `settings.portal_key`.

### Endpoints
- `POST /api/portal/track` — **PUBLIC** (whitelist `authMiddleware` exacte +
  CORS ouverte dans `corsMiddleware`, pattern portail WiFi). Corps
  `{key, promoId, kind: impression|click, clientKey?}`. Réponse **204 dans
  tous les cas** (succès, doublon, clé inconnue, promo absente, quota) :
  aucun oracle. Résolution du compte par `tenant.portalKey` (non secrète,
  aucun droit de lecture). `clientKey` = MAC normalisée (portail) ou repli
  `ip:<IP>` serveur.
- `GET /api/promos/stats` — console (`requireRole(2)`) : `{today, weekFrom,
  promos: [{id,title,impressions:{day,week,total},clicks:{day,week,total}}],
  totals}` — semaine = 7 jours glissants UTC ; lecture mémoire sous verrou.

### Règles anti-gonflement (contractuelles)
1. **Dédup** : ID d'événement DÉTERMINISTE
   `e + sha256(acc|promo|kind|clientKey|day)[:20]` — un appareil = une ligne
   par promo/jour/type ; re-POST = no-op.
2. **Existence** : seuls les `promoId` présents dans `tenant.portalPromos`
   (id posé `p…` ou déterministe `h…` — même règle que
   `portalHospitality`) sont acceptés.
3. **Quota IP** : limiter dédié NAT-friendly 300/10 min + 3000/24 h.
4. **Plafonds** : 3 000 événements/compte/jour ; rétention 90 jours ;
   journal ≤ 12 000 lignes (`prunePromoEvents`, suppressions répercutées en
   Neon par la diff syncTable).

### IDs de promos (analytics)
- `PUT /api/settings` : id aléatoire `p` + hex posé au 1er enregistrement,
  **conservé** si bien formé (`promoIDValid`, `^[ph][0-9a-f]{7,12}$`) — le
  round-trip console ne régénère JAMAIS (sinon compteurs remis à zéro).
  Id malformé → 400.
- Lignes héritées (sans id, pré-N°56) : id déterministe
  `h + sha256(title|desc|img|price)[:8]` calculé à la LECTURE
  (`portalHospitality`, `promoIDsOf`) — tracking immédiat sans ré-save.
- Nouveau champ promo **`link`** (https ≤ 300) : carte cliquable
  (`a.hosp-card`, target _blank) ; ouverture = événement `click`.
- Portail : cartes `data-promo-id`, `mikTrack()` fetch keepalive silencieux,
  `mikWatchPromos()` après rendu, dédup client `mikTracked`, `clientKey` =
  `clientMac`. Console : champ Lien par promo + panneau « Analyse de la
  vitrine » (headline semaine + vues/clics par ligne).

## PLAN DE FICHIERS

### Backend (Go)
| Fichier | Action |
|---|---|
| `internal/model/models.go` | + champs Profile/Router/HotspotUser/Sale/Settings ; + VoucherTemplate, UserLog, IPBinding, SchedulerTask, RouterTraffic, IfaceTraffic, TrafficPoint ; + DB fields ; + Cmd* constants (user_reset, ipbinding_add/set/remove, ping, read_dhcp/hosts/cookies/log/scheduler/resources, scheduler_add/set/remove, reboot, shutdown) ; + parité : Profile.AddressPool/ParentQueue/ValidityMin, HotspotUser/Batch.TimeLimitMin, CharsetNum |
| `internal/store/store.go` | Tick : applyExpiry, user logs capture, cleanup, traffic sim, lock-user kick |
| `internal/store/pg.go` | CREATE TABLE voucher_templates/user_logs/ip_bindings/scheduler_tasks/traffic + ALTERs + specs + sync |
| `internal/store/seed.go` | templates×3, profiles sellingPrice, bindings/scheduler/traffic seed |
| `internal/agent/agent.go` | builders nouveaux kinds + read_state v2 (board/freehdd/totalhdd/ifaces) + read_resources + profile address-pool/parent-queue + limit-uptime par lot + server= |
| `internal/api/agent_handlers.go` | protocole agent (register/cmd/result) + file de commandes (queueCommandLocked, requeue des lectures périmées, helpers de scripts) |
| `internal/api/agent_results.go` | application des résultats agent : applyReadState v2 (logs diff, traffic diff, board/hdd), markVoucherUsed, accumulateUptime, normalizePingResult |
| `internal/api/handlers_provision.go` | provisionning console agent : provision / rotate-token / refresh / import + applyImportHotspot |
| `internal/api/handlers_ipbindings.go` | F7 — IP bindings (CRUD cloud + commandes agent ipbinding_add/set/remove) |
| `internal/api/handlers_commands.go` | F8 — ping routeur + GET /api/commands/{id} (résultat normalisé) |
| `internal/api/handlers_router_tools.go` | F9 — outils routeur (dhcp/hosts/cookies/log/resources ; cache agent 120 s, simulation déterministe) |
| `internal/api/handlers_scheduler.go` | F10 — scheduler (CRUD + read_scheduler en cache) et reboot/shutdown |
| `internal/api/handlers_templates.go` | F2 — modèles (templates) de vouchers (CRUD scopé au compte, formats a4/58mm/80mm, sanitize HTML, unicité du modèle par défaut) |
| `internal/api/handlers_userlogs.go` | F3 — journal utilisateurs (liste paginée + export CSV « ; » BOM) |
| `internal/api/handlers_users_ops.go` | F4/F5 — actions utilisateurs (reset stats, extend, export CSV, bulk) + nettoyage des expirés (cleanup) ; enforcement F1 (enforceExpired) et filtres sessions live (onlineSessions, onlineKey) dans helpers.go |
| `internal/api/routes.go` | table de routage HTTP complète (mux) + handleHealth + fallback 404 JSON — ancien cœur de `handlers.go`, découpé par domaine : auth, dashboard, routers, profiles, users, vouchers, sessions, resellers, reports, accounting, settings, subscription (+ middleware.go, helpers.go) |

### Frontend (TSX)
| Fichier | Action |
|---|---|
| `src/lib/hotspot/types.ts` | tous les nouveaux types + ViewId « templates » « logs » |
| `src/lib/hotspot/i18n.ts` (NOUVEAU) | dictionnaires fr/en + useI18n |
| `src/lib/hotspot/use-router-resources.ts` (NOUVEAU) | hook ressources routeur (datalists profil + sélecteur Server du générateur) |
| `src/lib/hotspot/format.ts` | + fmtRouterDuration (format RouterOS w/d/h/m) |
| `src/components/hotspot/views/templates-view.tsx` (NOUVEAU) | liste + éditeur + aperçu |
| `src/components/hotspot/views/logs-view.tsx` (NOUVEAU) | journal utilisateurs |
| `src/components/hotspot/parts/template-render.ts` (NOUVEAU) | rendu variables + QR (lib `qrcode`) |
| `src/components/hotspot/parts/uc-print-dialog.tsx` | mode template (sélecteur + aperçu + print CSS par format) |
| `src/components/hotspot/parts/profile-dialog.tsx` | champs expMode/grace/lock/sellingPrice + parité (validité wdhm, datalists pool/queue) |
| `src/components/hotspot/views/vouchers-view.tsx` | parité Generate : Server, Time Limit par lot, charset num, codeLength 3-10, récap GetValidPrice |
| `src/components/hotspot/views/users-view.tsx` | actions reset/extend, export CSV, cleanup |
| `src/components/hotspot/views/profiles-view.tsx` | colonnes/badges nouveaux champs |
| `src/components/hotspot/views/routers-view.tsx` + `parts/router-tools.tsx` (NOUVEAU) | onglets trafic/bindings/outils/système |
| `src/components/hotspot/views/reports-view.tsx` | onglet Marge |
| `src/components/hotspot/views/settings-view.tsx` | cartes Expiration, Voucher (dns+logo), Langue |
| `src/components/hotspot/app-shell.tsx` | nav « Modèles » + « Journal » |

## TESTS (obligatoires avant de rendre la main)
- Backend : `cd /home/z/mikcloud/backend && gofmt -l . && go vet ./... && go build -o /tmp/mikcloud-test .`
- Backend smoke : démarrer `/tmp/mikcloud-test` (PORT=4000, dossier data temp),
  `curl` login + 1 appel par nouvelle route (voir worklog pour la méthode daemonize).
- Frontend : `cd /home/z/mikcloud/frontend && bun run lint` (0 erreur).
- Aucun test automatisé à écrire. NE PAS toucher aux workflows CI.
