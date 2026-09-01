# CONTRAT V2 — P0 / P1 / P2 (audit Mikhmon)

> **SOURCE DE VÉRITÉ** pour l'implémentation des 13 fonctionnalités issues de
> l'audit Mikhmon v3. Tout agent (backend comme frontend) DOIT s'y conformer
> strictement : noms de champs JSON, routes, sémantique. Voir AUDIT-MIKHMON-V3.md.

## 0. Conventions générales (inchangées)

- Toutes les routes console sous `/api/…`, auth `Authorization: Bearer <jwt>`.
- Erreurs : `{"error": "message"}` + code HTTP (400/401/403/404).
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

### Settings tenant (ajouts)
```go
DNSName string `json:"dnsName,omitempty"` // ex. wifi.mondomaine.ci
LogoURL string `json:"logoUrl,omitempty"` // data URL image ≤ 300 Ko
```
- `PUT /api/settings` accepte `dnsName` (≤100 chars) et `logoUrl` (data:image/*,
  ≤ 300 Ko — sinon 400 « Logo trop volumineux (300 Ko max) »).

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

- `POST /api/users/{id}/reset-stats` → `{ok:true}` : met à zéro bytesIn/bytesOut/uptimeUsedSec
  (cloud) + agent : queue nouvelle commande `user_reset` `{name}` (script :
  `/ip hotspot user reset-counters [find name=…]`) ; simulated : direct ; real : gateway Run.
- `POST /api/users/{id}/extend` `{days: number ≥ 1 ≤ 3650}` → `HotspotUser`
  - nouvelle `expiresAt = max(now, expiresAt) + days` ;
  - si le statut était `expired` → repasse `active` + agent `user_set {disabled:false}` ;
  - Activity « Utilisateur X prolongé de N j ».
- `GET /api/users/export?search=&status=&routerId=&kind=&profileId=` → CSV
  (colonnes : Utilisateur;Mot de passe;Profil;Statut;Routeur;Créé le;Expire le;Data entrée (Mo);Data sortie (Mo);Prix;Revendeur;Commentaire).
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
  `ValidityMinutes()` (génération, extension F4, resync) ; `validityDays` reste
  renseigné (arrondi supérieur : (validityMin+1439)/1440) pour rétro-compatibilité.
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

## PLAN DE FICHIERS

### Backend (Go)
| Fichier | Action |
|---|---|
| `internal/model/models.go` | + champs Profile/Router/HotspotUser/Sale/Settings ; + VoucherTemplate, UserLog, IPBinding, SchedulerTask, RouterTraffic, IfaceTraffic, TrafficPoint ; + DB fields ; + Cmd* constants (user_reset, ipbinding_add/set/remove, ping, read_dhcp/hosts/cookies/log/scheduler/resources, scheduler_add/set/remove, reboot, shutdown) ; + parité : Profile.AddressPool/ParentQueue/ValidityMin, HotspotUser/Batch.TimeLimitMin, CharsetNum |
| `internal/store/store.go` | Tick : applyExpiry, user logs capture, cleanup, traffic sim, lock-user kick |
| `internal/store/pg.go` | CREATE TABLE voucher_templates/user_logs/ip_bindings/scheduler_tasks/traffic + ALTERs + specs + sync |
| `internal/store/seed.go` | templates×3, profiles sellingPrice, bindings/scheduler/traffic seed |
| `internal/agent/agent.go` | builders nouveaux kinds + read_state v2 (board/freehdd/totalhdd/ifaces) + read_resources + profile address-pool/parent-queue + limit-uptime par lot + server= |
| `internal/api/agent_handlers.go` | applyReadState v2 (logs diff, traffic diff, board/hdd), queue helpers |
| `internal/api/handlers_ext.go` (NOUVEAU) | tous les nouveaux handlers console (templates, user-logs, users actions, ipbindings, ping, tools F9, scheduler, reboot, traffic, commands/{id}) |
| `internal/api/handlers.go` | seulement : enregistrement des nouvelles routes + modifs ponctuelles profiles/settings/reports/vouchers-generate |

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
