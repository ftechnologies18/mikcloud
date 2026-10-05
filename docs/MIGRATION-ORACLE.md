# RUNBOOK — Migration backend Render → Oracle Cloud Always Free (N°211)

> Document opérateur MikCloud. Objectif : dérouler PAS À PAS le déménagement
> du backend Go de Render vers une VM **Oracle Cloud Always Free** — **sans
> interruption de service** pour la flotte et les ~6 151 utilisateurs. Établi
> en octobre 2026 (session N°210). Lecteur : exploitant NON-TECHNICIEN —
> chaque geste a sa vérification. Contexte : §4bis/§4ter du
> RUNBOOK-HEBERGEMENT (dette ~13 $ + mail « 5 GB » du 02/10).

## 0. Décision et vue d'ensemble — 0 €/mois définitif

```
        ┌────────────────────────────────────────────┐
        │ GitHub — ftechnologies18/mikcloud          │
        │ code + CI (ci.yml · deploy-oracle.yml)     │
        └──────────────────┬─────────────────────────┘
                           │ ① build arm64 ② scp ③ restart systemd
                           ▼
┌──────────┐  HTTPS  ┌─────────────────────────┐  PostgreSQL  ┌───────────┐
│ Routeurs │────────▶│ VM Oracle « mikcloud-   │─────────────▶│ Supabase  │
│ MikroTik │ api.    │ backend » Ubuntu 24.04  │ session      │ (base     │
│ agents + │ mikcloud│ ARM A1 · 2 OCPU · 12 Go │ pooler :5432 │ primaire, │
│ portail  │ .ftci.fr│ binaire Go + Caddy      │ verify-full  │ inchangée)│
└──────────┘         └───────────┬─────────────┘              └───────────┘
                                 │ pg_dump 03:00 UTC (timer)
                                 ▼
                       ┌─────────────────────┐
                       │ Neon — coffre-fort  │
                       │ snapshot nocturne   │
                       └─────────────────────┘

Navigateurs (gérants + invités) ──HTTPS──▶ Vercel — mikcloud.ftci.fr
(inchangé) ; seule NEXT_PUBLIC_API_BASE → https://api.mikcloud.ftci.fr
change (cuite au build, api.ts:43).
```

Inchangés : médias R2 (`media.ftci.fr`) et base Supabase. Neufs : le
workflow `deploy-oracle.yml` et le rôle coffre-fort de Neon.

**Pourquoi Oracle** (échappatoire déjà identifiée au §4bis du RUNBOOK-HEBERGEMENT) :

| Caractéristique | Render gratuit | Oracle Always Free |
|---|---|---|
| Bande passante sortante | 5 Go/mois puis 0,15 $/Go | **10 To/mois** — la question bande passante (toute l'histoire N°205/N°209) devient nulle |
| RAM / CPU / éveil | 512 Mo / 0,1 CPU, veille 15 min (750 h/mois) | **12 Go / 2 OCPU ARM** Ampere A1 (réduit de 4/24 Go en juin 2026 — ~24× Render), **toujours éveillé** |
| Facture / entrée | 0 $ si < 5 Go, aucune carte | **0 €/mois définitif** — les shapes Always Free ne facturent rien ; carte exigée à l'inscription pour VÉRIFICATION (prélèvement ~1-2 $, remboursé) |
| Charge | Zéro DevOps | systemd, TLS (Caddy), sauvegardes — **absorbés par le kit `deploy/oracle/`** et ce runbook |

Le backend est un **binaire Go STATIQUE** (`CGO_ENABLED=0`) : port `PORT`
(défaut 4000, `backend/main.go:30-38`), écoute `0.0.0.0`, healthcheck
`GET /`, **aucune écriture disque** en mode PostgreSQL, 10 goroutines de
fond, état en mémoire — la base étant partagée, le risque de la migration
porte sur la couche réseau/TLS, **pas sur les données**.

## 1. Pré-requis et création de la VM

### 1.1 Compte Oracle (une fois)

1. Compte sur **oracle.com/cloud/free** ; une **carte bancaire est
   exigée** pour la vérification d'identité (prélèvement ~1-2 $,
   **remboursé**).
2. ⚠️ **Région d'origine = Paris (eu-paris-1)** — la plus proche d'Abidjan
   et de Supabase (eu-west-1) ; choix **définitif**, à L'INSCRIPTION.
   > **Erratum (03/10/2026, N°215)** — le tenancy réel a finalement été
   > créé en **af-johannesburg-1** (choisi à l'inscription). Écart ASSUMÉ :
   > +~300 ms d'aller-retour vers Supabase eu-west-1 (absorbé : syncs
   > différentielles N°210, pooler persistant, logins invités directs au
   > routeur) et Johannesburg ne possède qu'UN Availability Domain (pas de
   > repli AD en cas de « Out of capacity »). Paris reste la recommandation
   > pour tout NOUVEAU tenancy. La VM lancée sur ce tenancy y est
   > référencée comme `yMUP:AF-JOHANNESBURG-1-AD-1`.
   >
   > **Analyse latence DB (03/10/2026, N°224)** — pourquoi les ~300 ms ne
   > touchent AUCUNE requête utilisateur : le store mikcloud garde **la
   > mémoire comme moteur de calcul** (handlers sur `*model.DB` sous
   > verrou, tête de `pg.go`) et Postgres comme **miroir différentiel
   > asynchrone** — marquage sale + réveil du syncreur (`store.go`) :
   > debounce 500 ms, **1 transaction max toutes les 3 s**, flush hors
   > verrou global sur photographie CloneDeep, flush final au SIGTERM
   > (arrêt propre = zéro perte). Coûts réels : boot plus lent (~37
   > tables × RTT) et fenêtre de perte ~3,5 s de lignes modifiées
   > **uniquement** sur crash brutal. L'instrumentation N°71 (syncstats)
   > livrera le lag réel après atterrissage — mesurer avant toute
   > chirurgie DB.
   >
   > **Rapprochement impossible côté Supabase** (vérifié sur la doc
   > officielle le 03/10/2026) : aucune région africaine au catalogue
   > (Amériques, Londres/Zurich, Asie, Océanie, São Paulo) et un projet
   > ne se déplace pas en place (nouveau projet + dump/restore).
   > **À l'inverse, Abidjan→JNB ≈ 60-80 ms bat Abidjan→Francfurt ≈
   > 110 ms** : la migration AMÉLIORE la latence réelle des routeurs et
   > de la console ; seul le saut DB part en asynchrone. Si un jour le
   > lag DB mesuré dérange : l'alternative est un backend européen
   > (Cloud Run), pas un déménagement Supabase.

### 1.2 Créer l'instance (clics console)

Console Oracle → menu ☰ → **Compute → Instances → Create instance** :

| Champ | Valeur à saisir |
|---|---|
| Name | `mikcloud-backend` |
| Placement | Paris (eu-paris-1), Availability Domain au choix |
| Image | **Canonical Ubuntu 24.04** (passe en aarch64 avec le shape) |
| Shape | **VM.Standard.A1.Flex** — 2 OCPU, 12 Go — libellé « Always Free eligible » à vérifier |
| Clé SSH | **Generate a key pair** (ed25519) → TÉLÉCHARGER la clé privée `.key` : seul moyen d'entrer dans la VM |
| Boot volume | **50 Go** |
| Créer | → noter l'**ADRESSE IP PUBLIQUE** affichée ensuite |

### 1.3 Ouvrir les ports dans la VCN (Security List)

Console → **Networking → Virtual Cloud Networks** → la VCN de l'instance →
**Security Lists → Default Security List** → *Add Ingress Rules* : **22**
(idéalement restreint à l'IP de l'administrateur), **80** et **443** en
`0.0.0.0/0` — le 80 est requis par le challenge HTTP-01 de Let's Encrypt.

### 1.4 Première connexion — deux pièges de l'étape

```bash
ssh -i <clé_privée> ubuntu@<IP_PUBLIQUE>
```

- **« Out of capacity »** : **la friction n°1 d'Oracle** — retenter plus
  tard, changer d'Availability Domain, voire de région.
- **Pare-feu local Ubuntu** : les images Ubuntu-Oracle DROP tout par
  défaut en iptables, même Security List ouverte — `bootstrap.sh` (§2)
  ouvre et persiste ; commande manuelle : §10, piège 2.

## 2. Bootstrap de la VM — UNE commande

Sur la VM, en SSH, copier-coller (l'argument est le domaine futur de l'API) :

```bash
curl -fsSL https://raw.githubusercontent.com/ftechnologies18/mikcloud/main/deploy/oracle/bootstrap.sh | sudo bash -s -- api.mikcloud.ftci.fr
```

Le script est **idempotent** (re-lançable sans casse) et fait, dans l'ordre :

1. `apt` : mises à jour + paquets (client PostgreSQL pour `pg_dump`/`psql`,
   `ca-certificates`, `netfilter-persistent`…) ;
2. **iptables** : ouvre 80/443 + persistance (piège §1.4) ;
3. **Caddy** depuis le dépôt officiel (TLS Let's Encrypt automatique) ;
4. utilisateur système `mikcloud` + `/opt/mikcloud` +
   `/etc/mikcloud/mikcloud.env` créé depuis `env.example` (placeholders, §3) ;
5. installe l'unit systemd, le `Caddyfile` et le timer de sauvegarde ;
6. **CA Supabase** : télécharge `backend/certs/supabase-prod-ca-2021.crt`
   depuis le dépôt → `/usr/local/share/ca-certificates/` +
   `update-ca-certificates`.

**Piège critique n°1 de la migration** : la connexion Supabase exige
`sslmode=verify-full` contre la PKI **privée** « Supabase Root 2021 CA »,
absente des magasins publics (déjà le comportement de l'image Docker,
`backend/Dockerfile:22-23` — cf. RUNBOOK-POSTGRES §12). Sans elle : `x509:
certificate signed by unknown authority` au boot.

Contenu du kit `deploy/oracle/` (fichiers créés avec ce runbook) :

| Fichier | Rôle |
|---|---|
| `mikcloud.service` | systemd : `User=mikcloud`, `EnvironmentFile=/etc/mikcloud/mikcloud.env`, `ExecStart=/opt/mikcloud/mikcloud-server`, `Restart=always`, durcissement `ProtectSystem=strict` + `PrivateTmp` + `StateDirectory=mikcloud` (viable : zéro écriture disque) |
| `Caddyfile` | `reverse_proxy 127.0.0.1:4000`, `encode`, en-têtes |
| `env.example` | Les **27 variables** documentées — PLACEHOLDERS, **jamais de vraies valeurs** |
| `mikcloud-backup.service` + `.timer` | Sauvegarde nocturne `OnCalendar=*-*-* 03:00 UTC` |
| `backup-neon.sh` | Le corps de la sauvegarde (§7) |

## 3. Variables d'environnement

Les **vraies valeurs vivent uniquement dans `/etc/mikcloud/mikcloud.env`**
(`chmod 640 root:mikcloud`) et dans les 3 secrets GitHub (§6) — **aucun
secret dans le dépôt**. Liste exhaustive des 27 variables (défauts inclus) :
`deploy/oracle/env.example`. Les critiques :

| Variable | Valeur sur la VM | Rappel |
|---|---|---|
| `DATABASE_URL` | DSN Supabase **session pooler `:5432`**, **SANS `sslmode=`** | L'app ajoute `verify-full` elle-même ; le `:6543` casse `pg_dump`/pgx (42P05) — validations `ops/oct1/step5-render-flip.sh:79-90` |
| `JWT_SECRET` | (valeur Render) | **Obligatoire** : sans elle le binaire REFUSE de démarrer (`backend/main.go:50`) |
| `ADMIN_USERNAME` / `ADMIN_PASSWORD` | (valeurs Render) | Sécurité P0 (`render.yaml:20-26`) |
| `ALLOWED_ORIGIN` | **INCHANGÉE** | Le CORS autorise l'origine du FRONTEND (`https://mikcloud.ftci.fr`), qui ne bouge pas |
| `APP_PUBLIC_URL` | `https://mikcloud.ftci.fr` | Liens `/join/{token}` du portail |
| `MIKCLOUD_BASE_URL` | `https://api.mikcloud.ftci.fr` | **La clé de la bascule** — §5 |
| `NEON_DATABASE_URL` | DSN du projet Neon | Utilisée par `backup-neon.sh` (§7) |

Valeurs actuelles lisibles dans le dashboard Render → Environment ;
inventaire complet : `docs/RUNBOOK-SECRETS.md`. Après édition :

```bash
sudo nano /etc/mikcloud/mikcloud.env     # édition
sudo systemctl restart mikcloud           # les variables ne s'appliquent QU'AU DÉMARRAGE
journalctl -u mikcloud -n 50 --no-pager  # vérifier le boot propre
```

(Même sémantique que Render : variable posée sans redémarrage = sans
effet — leçon N°172 du `step5-render-flip.sh`.)

## 4. Premier démarrage et test AVANT bascule

### 4.1 Poser le binaire (compilé pour ARM64 Linux)

```bash
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o dist/mikcloud-server .
```

- **Option A (recommandée)** : GitHub → **Actions → deploy-oracle.yml →
  Run workflow** — compile, `scp`, redémarre. Secrets §6 absents → échec
  **explicite** « complète l'installation manuelle » (un déploiement prod
  raté doit être VISIBLE) ;
- **Option B (manuelle)** : commande ci-dessus depuis `backend/`, puis :

```bash
scp -i <clé> dist/mikcloud-server ubuntu@<IP>:/tmp/mikcloud-server
ssh -i <clé> ubuntu@<IP> "sudo install -o mikcloud -g mikcloud -m 755 /tmp/mikcloud-server /opt/mikcloud/mikcloud-server && sudo systemctl restart mikcloud"
```

### 4.2 Vérifications SANS toucher au DNS

| Vérification | Commande (sur la VM) | Attendu |
|---|---|---|
| Services actifs | `systemctl status mikcloud caddy` | `active (running)` |
| Boot propre | `journalctl -u mikcloud -n 50 --no-pager` | mode PostgreSQL, connexion Supabase OK (CA installée), pas de `x509` |
| Healthcheck direct | `curl -fsS http://127.0.0.1:4000/` | `200` |

Depuis **votre machine**, sans attendre le DNS (teste pare-feu → Caddy,
indépendamment de Cloudflare) :

```bash
curl -i --resolve api.mikcloud.ftci.fr:80:<IP_PUBLIQUE_VM> http://api.mikcloud.ftci.fr/
```

Attendu : **HTTP 308** (redirection Caddy vers HTTPS) — la chaîne réseau
répond. Le **certificat Let's Encrypt ne peut pas encore exister** (le
challenge HTTP-01 se résout via le DNS public, encore sur Render) : il
sera émis **automatiquement ~1 min après la bascule** (§8-6) ; d'ici là,
des erreurs d'acquisition dans `journalctl -u caddy` sont du bruit NORMAL.
La VM tourne et parle à Supabase mais **personne ne l'utilise** : la prod
est encore sur Render. (`GOMEMLIMIT=400MiB`, plafond GC du conteneur 512
Mo — `Dockerfile:34` — n'a plus d'objet avec 12 Go.)

## 5. Stratégie de domaine en DEUX temps (le cœur du runbook)

**Pourquoi deux temps** : les scripts agents **gravent l'URL à
l'installation** (issus de `GET /api/routers/{id}/provision`,
`handlers_provision.go:92` ; schedulers en dur, `agent.go:351-398`), et
l'`apiBase` des pages portail suit `MIKCLOUD_BASE_URL` (empreinte N°210,
`hotspot_files.go:129-136` : la poser re-déploie les pages au check-in
suivant, cadence 45-240 s). On fait donc converger toute la flotte vers le
**domaine** (T1), puis on retourne le domaine vers la VM (T2) — la bascule
finale devient INVISIBLE pour tous.

### 5.1 T1 — le domaine répond sur Render (transition sans coupure)

`mikcloud.onrender.com` continue de marcher pendant TOUTE la manœuvre.

1. **Custom domain côté Render** : dashboard → service mikcloud → Settings
   → **Custom Domains → Add** → `api.mikcloud.ftci.fr`. Render affiche les
   enregistrements à créer.
2. **Cloudflare** (ftci.fr y est hébergé — `media.ftci.fr` y est déjà en
   custom domain R2) : DNS → créer les enregistrements affichés (CNAME
   vers la cible indiquée + vérification). ⚠️ **Nuage GRIS (DNS-only)**
   obligatoire — l'orange casserait le challenge HTTP-01 de Caddy au T2.
3. Vérifier : `https://api.mikcloud.ftci.fr/` → `200` **servi par Render**
   (certificat Render émis après validation DNS).
4. **Poser `MIKCLOUD_BASE_URL=https://api.mikcloud.ftci.fr` sur Render**
   (dashboard → Environment → Add) puis redémarrer (Manual Deploy) :
   l'`apiBase` cuite change → re-déploiement des pages portail — **une
   vague unique de quelques Mo, ATTENDUE** (empreinte N°210). Alternative
   scriptée : pattern PUT du `step5-render-flip.sh:150-177` (N°172).
5. `ALLOWED_ORIGIN` : **INCHANGÉE** (le CORS autorise l'origine du
   frontend, qui ne bouge pas).
6. **Migrer les routeurs UN PAR UN — À DISTANCE, sans Winbox** (N°230) :
   les scripts agents gravent l'URL dans le `on-event` de leurs schedulers
   (`mikcloud-agent` 45 s, `mikcloud-watch` 20 s — le cadenceur de quota
   est 100 % local, aucune URL). La commande `agent_migrate` les réécrit
   via le canal de commande existant, sous trois filets : pré-flight de la
   nouvelle URL (`/agent/register` heartbeat AVANT tout retrait), pont
   anti-orphelin (`mikcloud-agent-b` posé et vérifié avant de retirer
   l'ancien), ménage du pont seulement si le canonique est vérifié
   (présence + hôte dans l'on-event). Par routeur :

   ```bash
   curl -X POST -H "Authorization: Bearer $TOKEN" \
     https://api.mikcloud.ftci.fr/api/routers/{id}/migrate-url
   # puis suivre : GET /api/commands/{commandId} → status=done, ok
   ```

   Vérifier ensuite le check-in (console → Infrastructure → lastSeen) ;
   un routeur non migré continue sur `onrender.com` (toujours servi) —
   d'où la décommission après zéro trafic résiduel seulement. Le
   walled-garden et les pages portail convergent EUX-MÊMES au check-in
   (sig N°48/49 + empreinte N°210 — `MIKCLOUD_BASE_URL` change la sig,
   l'ensure re-file automatiquement).
7. **Vercel** : `NEXT_PUBLIC_API_BASE=https://api.mikcloud.ftci.fr` →
   Redeploy (URL cuite au build, `frontend/…/api.ts:43` — mode direct
   actif, pas de proxy `vercel.json`).
8. **Keep-alive** : `keepalive.yml` (désactivé depuis le 13/09) et le
   monitor UptimeRobot (Option A du `docs/RUNBOOK-KEEPALIVE.md`) visaient
   l'hibernation Render — **une VM ne dort jamais** et les agents la
   sollicitent de toute façon. Monitor UptimeRobot sur
   `https://api.mikcloud.ftci.fr/` (optionnel — les agents suffisent) ;
   `keepalive.yml` à supprimer ou repointer au ménage final.

### 5.2 T2 — la bascule DNS (une seule minute)

Cloudflare → DNS → remplacer le CNAME `api` par un **A record vers l'IP
publique de la VM** — toujours **DNS-only (gris)**, TTL auto (~300 s).
Pour la flotte convergée en T1, rien ne change : le domaine résout
désormais vers la VM. Déroulé et validations : §8.

## 6. CI/CD — déploiements GitHub Actions

### 6.1 Les 3 secrets (dépôt → Settings → Secrets and variables → Actions)

`ORACLE_HOST` (IP publique de la VM), `ORACLE_USER` (utilisateur SSH de
déploiement, ex. `ubuntu`) et `ORACLE_SSH_KEY` (la **clé privée** ed25519
complète — contenu du `.key` de §1.2).

### 6.2 Le workflow `deploy-oracle.yml`

Déclenché par un push `main` touchant `backend/**` ou `deploy/oracle/**`,
plus `workflow_dispatch` (bouton manuel) : checkout → `setup-go` → build
arm64 (§4.1) → `scp` → `ssh … systemctl restart mikcloud`. Secrets
absents → échec **explicite** « complète l'installation manuelle » —
différence volontaire avec `deploy-render` (silencieux quand
`RENDER_API_KEY` manque, `ci.yml:233-236`) : un déploiement prod raté ne
doit plus jamais passer inaperçu (leçon N°36-b, `ci.yml:15-23`).

### 6.3 Le sentinel `RENDER-DEPLOY-FROZEN`

Le job `deploy-render` de `ci.yml` (déclenchement par API, service
`srv-da974o142hec73euul60`) est gelé tant que le fichier
`RENDER-DEPLOY-FROZEN` existe à la racine du dépôt — mécanisme N°165
(`ci.yml:217-226`). **POSÉ au T2 (N°244)** avec un second lecteur :
l'étape « Sync variables Render → VM » de `deploy-oracle.yml` est gelée
par le MÊME sentinel (post-T2 la VM est la source de vérité — l'env y est
rotée par `t2-secrets.yml`, un sync réintroduirait les anciennes valeurs
Render et casserait le backend au redémarrage suivant). Sans sentinel,
un push backend déploierait sur Render ET sur la VM en parallèle (piège
n°12). Après T2, `deploy-oracle.yml` est LE chemin backend ; Vercel
déploie le frontend via son webhook. Levée du gel : supprimer le fichier
dans un commit de reprise consignée (rollback Render §9.2 uniquement).

## 7. Coffre-fort Neon — snapshot nocturne

Neon (l'ancienne base de secours) est recyclée en **coffre-fort** : chaque
nuit à **03:00 UTC** (= 03:00 à Abidjan), le timer `mikcloud-backup.timer`
exécute `backup-neon.sh` :

```
# Phase 1 — dump (schéma public uniquement) vers fichier temporaire
pg_dump --schema=public --no-owner --no-privileges --clean --if-exists "$DATABASE_URL_SESSION" > "$DUMP_FILE"
# Phase 2 — filtre défense en profondeur puis restore depuis le fichier
sed -E '/…supabase_vault…/d' "$DUMP_FILE" | psql "$NEON_DATABASE_URL" -v ON_ERROR_STOP=1
```

- source : le DSN Supabase **session pooler `:5432`** — le transactionnel
  `:6543` ne fonctionne PAS pour `pg_dump` ;
- `--schema=public` (N°243-d) : le coffre-fort ne snapshot que **les données
  applicatives MikCloud** (schéma `public`). Le dump « base entière »
  embarquait les schémas de la plateforme Supabase (`auth`, `storage`,
  `realtime`, `vault`, `graphql`, `neon_auth`…) qui ne sont PAS restaurables
  hors Supabase et bloquaient le restore (run 2 : extension
  `supabase_vault` ; run 3 : clause SUSET `SET log_min_messages` de
  `realtime.list_changes`, refusée au rôle non-superuser Neon). Le schéma
  `public` est auto-suffisant : `gen_random_uuid()` natif (PG13+), zéro
  référence croisée, zéro extension requise ;
- **dump puis restore en deux phases** (N°243-d) : le pipeline direct
  `pg_dump | psql` s'est montré faillible (4 échecs / 4 à ~61 s,
  « SSL SYSCALL error: EOF », connexion source coupée au milieu de la
  première grosse COPY ; réfuté par tests : ni l'inactivité, ni le
  consommateur lent, ni la double connexion longue ne tuent seuls) — le
  découplage par fichier isole chaque phase, reprise jusqu'à 3 tentatives
  espacées de 30 s (le pire cas ~7 min est couvert par le
  `TimeoutStartSec=900` de l'unité systemd) ;
- `--clean --if-exists` rend le snapshot rejouable nuit après nuit ;
- le script **journalise** (`journalctl -u mikcloud-backup`) et **propage
  le code retour** : un échec se voit en échec systemd, pas en silence.

**Limite** : Neon gratuit = 0,5 Go — le schéma applicatif fait ~22 Mo
mesurés (40 tables), marge large. C'est un filet, PAS une standby — la
primaire reste Supabase.

## 8. Bascule finale — checklist pas-à-pas (T2)

Pré-requis : T1 (§5.1) terminé — custom domain validé, tous les routeurs
re-provisionnés, Vercel reconstruit sur le domaine.

| # | Action | Validation — NE PAS PASSER À LA SUIVANTE sans ✓ |
|---|---|---|
| 1 | VM : `systemctl status mikcloud` + `journalctl -u mikcloud -n 50` | `active (running)`, synchro Supabase OK |
| 2 | Compter les **37 tables différentielles** vues par le backend (log de chargement) — règles de migration du **§5 du RUNBOOK-HEBERGEMENT** | 37/37 |
| 3 | Sync-status (console, ou `GET /api/admin/sync-status` en local) | mode **postgresql**, `lastChangedBytes` ~0 en régime établi (N°210) |
| 4 | **Rotation des secrets pendant la fenêtre** (§5 du RUNBOOK-HEBERGEMENT + `docs/RUNBOOK-SECRETS.md`) : JWT_SECRET, ADMIN_PASSWORD, DSN — nouvelles valeurs sur la VM | fait AVANT le DNS |
| 5 | **Cloudflare** : CNAME `api` → **A record = IP VM**, nuage GRIS, TTL auto | enregistrement sauvegardé |
| 6 | Attendre ≤ 5 min, puis `curl -fsS https://api.mikcloud.ftci.fr/` | `200` + cadenas (certificat Let's Encrypt par Caddy, ~1 min) |
| 7 | Console → Infrastructure | check-in des agents visible, pour CHAQUE routeur |
| 8 | Téléphone sur le WiFi invité : page du portail | page servie, **bannière chargée depuis R2** (`media.ftci.fr`) |
| 9 | Vendre un **voucher test** | vente enregistrée, voucher utilisable |
| 10 | Scanner le **QR `/join/{token}`** d'une invitation | inscription aboutie |
| 11 | Première sauvegarde : `sudo systemctl start mikcloud-backup.service` puis `systemctl status mikcloud-backup` | exit 0 ; tables visibles dans la console Neon |
| 12 | Guetteurs : `df -h` ; **alerte budget Oracle à 1 €** (Billing → Budgets — les shapes Always Free ne facturent rien : elle ne doit JAMAIS sonner) ; UptimeRobot | tous verts |
| 13 | Surveiller 48 h : check-ins, ventes, sync-status, `journalctl -u mikcloud -e` | rien d'anormal → §9 |

Si une étape échoue : le service Render est TOUJOURS debout et le DNS
revient en arrière en minutes (§9) — aucune perte de données possible
(code GitHub, données Supabase, médias R2).

## 8-bis. T2 direct vers le pont E5 — sans attendre la capture A1 (N°244)

**Contexte** : la facture Render de septembre (~16 € — §9.1, l'héritage
pré-migration) est impayable (app en essai, zéro revenu) ; le support
Render a confirmé la **suspension du service au 16/10**. La chasse A1
(N°232/241) reste vide après 190+ sondes : T2 ne peut PAS attendre une
capture. Il n'a d'ailleurs JAMAIS eu besoin de l'attendre — le **pont
E5** (IP réservée `84.12.85.241`, backend déployé par `deploy-oracle.yml`
à chaque push, env synchronisée) est une machine de production complète
qui tourne à vide. L'élégance de l'IP réservée « à vie » : **un seul
geste DNS au total** — la capture A1/micro suivante déplace l'IP vers la
nouvelle machine (`land-a1.yml`/`land-micro.yml`), le DNS ne rechange
plus jamais.

**Déroulé de la fenêtre T2** (chaque phase rejouable sans impact — le
pont est idle pré-flip) :

| # | Qui | Action | Référence |
|---|---|---|---|
| 1 | agent | Commit T2 : sentinel `RENDER-DEPLOY-FROZEN` (gel déploiement Render N°165 + gel du sync §6.3) + workflow `t2-secrets.yml` + utilitaire `cmd/mikderive` | N°244 |
| 2 | agent | Dispatch `t2-secrets` phase JWT : épinglage `CREDENTIALS_KEY` (HKDF de l'ancien JWT_SECRET via mikderive — sinon les credentials RouterOS chiffrés deviennent indéchiffrables) + JWT_SECRET neuf + healthcheck | §8-4 adapté |
| 3 | exploitant | **Supabase** : Project Settings → Database → **Reset database password** (le reset n'est pas scriptable — verdict §2.8 du RUNBOOK-SECRETS) | geste a |
| 4 | exploitant | **GitHub** : Settings → Secrets → Actions → **`SUPABASE_DSN_NEW`** = la chaîne « Connection pooling » affichée (jamais le chat — doctrine §0) | geste b |
| 5 | agent | Dispatch `t2-secrets` phase DSN : normalisation session `:5432`, garde anti-oubli (DSN identique = reset pas fait), test `SELECT 1`, `mikcloud.env` + `backup.env` (le timer 03:00 UTC lit backup.env), restart, **snapshot test Supabase→Neon**, secrets GitHub `DATABASE_URL`+`SUPABASE_DATABASE_URL` (backup.yml et standby-restore.yml sinon morts), env Render + redéploiement (standby sain) | §8-4 adapté |
| 6 | exploitant | **Cloudflare** : enregistrement `api` → **A record `84.12.85.241`**, nuage GRIS, TTL auto — LE geste de bascule (~1 min, propagation ≤ 300 s) | §5.2/§8-5 |
| 7 | agent | Validations post-flip : `https://api.mikcloud.ftci.fr/` 200 + cadenas. ⚠ N°245-b (mesuré au T2) : le certificat NE s'émet PAS « tout seul en ~1 min » — les échecs d'acquisition d'AVANT le flip (domaine pointé vers Render) ont mis certmagic en reprise exponentielle (jusqu'à 24 h) et le flip ne déclenche AUCUNE nouvelle tentative : ERR_SSL_PROTOCOL_ERROR des heures durant, alors que l'émission réussirait désormais. Parade immédiate : dispatch `t2-caddy-rescue.yml` (relance Caddy → émission immédiate — piège n°17). Puis : check-in des agents, portail sur téléphone, vente voucher test, QR `/join` test, timer backup confirmé pour la nuit | §8-6→12, N°245-b |
| 8 | agent | 48 h de surveillance (check-ins, ventes, sync-status, journal) puis constat : Render sera suspendu le 16/10 sans que rien ne change pour nous | §8-13/§9 |

**Exécution (nuit du 05/10, UTC — N°245)** : étapes 1-2 accomplies le 04/10
(commit `610647f`, phase JWT run `37243863434` SUCCESS) ; gestes 3-4 à 00:08 ;
étape 5 phase DSN : run `37246345785` mort en broken pipe APRÈS application de
l'env (étapes 7-9 jamais jouées) → correctif N°245-a (`resume_after_apply` +
keepalives) puis reprise run `37252639942` SUCCESS (snapshot test, secrets
GitHub, env Render + redéploiement standby) ; étape 6 flip DNS ~00:25 ;
**incident TLS post-flip** (backoff certmagic + apostrophe française —
N°245-b/c, pièges 17-18) sauvé à 02:01 par `t2-caddy-rescue.yml` run
`37253717736` (certificat Let's Encrypt PRODUCTION émis en ~5 s, HTTPS 200) ;
étape 7 : validations infra OK (200 + cadenas + chaîne `http→308→https`) —
validations métier (check-in des agents, portail sur téléphone, vente voucher
test, QR `/join`, timer 03:00 UTC) à confirmer pendant les 48 h de l'étape 8.
Le secret `SUPABASE_DSN_NEW` a été supprimé après application complète.

**ADMIN_PASSWORD** : pas de nouvelle rotation au T2 — déjà rotée
proprement le 01/10 (RUNBOOK-SECRETS §2.8, par API, jamais exposée) et
synchronisée sur le pont par l'ancien sync.

**La chaîne d'échéances qui en découle** :
- **16/10** : suspension Render — sans effet (T2 fait avant) ; le
  rollback Render devient « payer + resume » (l'env Render est tenue à
  jour par `t2-secrets.yml`, le chemin reste documenté §9.2) ;
- **31/10** : crédits du pont E5 épuisés — l'atterrissage A1/micro doit
  avoir eu lieu avant (la chasse survit au pont : cron GitHub nocturne +
  timer autonome installé par `land-micro.yml` sur toute machine atterrie) ;
- A1 et micro = shapes **Always Free** : stables indéfiniment après
  atterrissage — l'IP réservée déménage sans toucher au DNS.

**Si le 25-28/10 arrive sans capture A1 NI micro** : session de décision
obligatoire (le pont meurt le 31/10 — un problème connu à l'avance vaut
mieux qu'une interruption subie).

## 9. Décommission Render et rollback

### 9.1 Décommission (à T2 + 48 h de stabilité, PAS avant)

1. **Vérifier qu'AUCUN agent n'arrive plus sur `onrender.com`** : dashboard
   Render → Metrics/Logs (trafic ~0), journal d'activité console, logs
   Vercel. Un routeur oublié en T1 se voit ici — le re-provisionner.
2. **Poser le sentinel** : fichier `RENDER-DEPLOY-FROZEN` à la racine du
   dépôt, commiter, pousser — `deploy-render` gelé (§6.3).
3. **Suspendre le service** (facturé 0, conservé en rollback) :

```bash
curl -X POST -H "Authorization: Bearer $RENDER_API_KEY" \
  https://api.render.com/v1/services/srv-da974o142hec73euul60/suspend
```

   (clé dans le coffre — résolution au pattern du `step5-render-flip.sh:64-69` ;
   équivalent clic : dashboard → Settings → Suspend. Réactivation : `POST
   …/resume`.)
4. **La facture de septembre (~13 $) reste à traiter par le courrier
   support** — la migration ne l'annule pas (§4bis du RUNBOOK-HEBERGEMENT).
5. **NE JAMAIS supprimer le service Render avant plusieurs semaines de
   stabilité** — c'est le plan B intégral (§9.2).

### 9.2 Rollback (minutes, tant que le service Render existe)

| Situation | Geste | Délai |
|---|---|---|
| Problème après T2 | Cloudflare : A record → CNAME vers Render (le custom domain y est toujours) ; si suspendu : `POST …/resume` (ou dashboard) | minutes (TTL 300 s) |
| Render redevient primaire | retirer `RENDER-DEPLOY-FROZEN` dans un commit → le déploiement CI Render se ré-arme (N°165) | immédiat |
| Vercel | `NEXT_PUBLIC_API_BASE=https://mikcloud.onrender.com` + Redeploy | ~1 min |

## 10. Pièges consignés (à relire avant chaque geste)

| # | Piège | Symptôme | Parade |
|---|---|---|---|
| 1 | Capacité A1 | « Out of capacity » à la création | Retenter / autre AD / région — friction n°1, rien à corriger |
| 2 | iptables local Ubuntu-Oracle | 80/443 ouverts en Security List mais timeout | Pare-feu LOCAL qui droppe — `bootstrap.sh` ouvre+persiste ; manuel : `iptables -I INPUT -p tcp --dport 80 -j ACCEPT` (+ 443) puis `netfilter-persistent save` |
| 3 | **CA Supabase privée** | `x509: certificate signed by unknown authority` au boot | `certs/supabase-prod-ca-2021.crt` → `/usr/local/share/ca-certificates/` + `update-ca-certificates` (Dockerfile:22-23) — fait par `bootstrap.sh` |
| 4 | `pg_dump` sur `:6543` | erreurs prepared statements | DSN **session pooler `:5432`** uniquement (step5:83-88) |
| 5 | `sslmode` dans le DSN | connexion cassée | L'app ajoute `verify-full` elle-même — DSN SANS `sslmode=` (step5:85-86) |
| 6 | Nuage Cloudflare ORANGE | échec HTTP-01 / boucles de redirection | Enregistrement `api` **DNS-only (gris)** — T1 et T2 |
| 7 | Variable posée sans effet | `mikcloud.env` édité, rien ne change | S'applique AU REDÉMARRAGE — `systemctl restart mikcloud` (sémantique N°172) |
| 8 | Vague portail après `MIKCLOUD_BASE_URL` | journal d'activité + quelques Mo | **Attendu** (empreinte N°210) — vague unique |
| 9 | Restart = perte d'état mémoire | sessions agents, compteur N°72, pinlock, signup_abuse, mode simulé | Attendu (audit N°210) : les agents re-check-innent en autonomie ; éviter l'heure de pointe |
| 10 | `JWT_SECRET` absente | le binaire refuse de démarrer | La poser dans `mikcloud.env` (main.go:50) |
| 11 | Limite Neon 0,5 Go | sauvegarde en échec | La base fait quelques Mo ; vérifier le statut du timer après la 1ʳᵉ exécution |
| 12 | `deploy-render` toujours actif | un push backend déploie sur les DEUX plateformes | Sentinel `RENDER-DEPLOY-FROZEN` (§6.3/§9.1) |
| 13 | Aucun secret dans le dépôt | — | `env.example` = placeholders ; vraies valeurs dans `/etc/mikcloud/mikcloud.env` (640 root:mikcloud) + 3 secrets GitHub |
| 14 | **MTU 9000 des images Oracle** (N°228) | chargement DB au boot **×4 plus lent** (4 min au lieu de 1 ; transferts Supabase étouffés, octets coincés en Recv-Q), services qui « marchent au ralenti » | **PRÉVENTIF : `bootstrap.sh` [0b/8] (N°231)** détecte l'interface par défaut, la passe à 1500 et pose `/etc/netplan/99-mtu.yaml` (fusion lexicale, sans toucher au fichier cloud-init) ; en curatif : `ip link set <dev> mtu 1500` + netplan |
| 15 | Verrou apt du premier boot (N°228) | cloud-init exécute bootstrap.sh pendant qu'unattended-upgrades tient `/var/lib/apt/lists/lock` → `E: Could not get lock` → script mort, 80/443 jamais ouverts | `bootstrap.sh` [0/8] attend le verrou (max 10 min) ; en curatif : relancer bootstrap.sh (idempotent) via SSH |
| 16 | Healthcheck trop impatient (N°228) | `curl 127.0.0.1:4000` 3 s après le restart → refus de connexion : le chargement des ~37 tables Supabase prend ~1 min (JNB→Londres) | Le workflow retry jusqu'à 10 min (boucle 10 s) ; en manuel : attendre « en écoute sur le port 4000 » dans `journalctl -u mikcloud` |
| 17 | **Backoff certmagic après échecs ACME pré-flip** (N°245-b) | Post-flip DNS : `ERR_SSL_PROTOCOL_ERROR` / handshake TLS « alert internal error » alors que le port 80 répond son 308 — pendant des HEURES, sans auto-guérison | Les échecs d'émission d'AVANT le flip (domaine pointé ailleurs) placent Caddy/certmagic en reprise exponentielle (jusqu'à 24 h) ; le flip ne déclenche AUCUNE nouvelle tentative. `systemctl restart caddy` re-tente immédiatement → `t2-caddy-rescue.yml` (diagnostic journaux + relance + vérif bout-en-bout depuis le runner) — re-dispatchable, réutilisable après tout atterrissage A1/micro |
| 18 | **Apostrophe française dans une commande SSH mono-quotée** (N°245-c) | L'étape meurt en `syntax error near unexpected token (` AVANT d'exécuter quoi que ce soit côté pont (mesuré : run 37252683556 — le « l'instant » de « Émission re-tentée à l'instant » fermait la quote du `ssh '…'`, la relance n'a jamais eu lieu) | Commandes distantes en heredoc : `ssh … bash -s <<'REMOTE'` — le corps est LITTÉRAL (apostrophes et `$(…)` compris, l'expansion se fait côté pont) ; JAMAIS de texte français (ou d'apostrophe) dans un `ssh '…'` |

Références : N°211 (audit : état mémoire, URLs gravées, 27 variables),
N°207-209 (`docs/RUNBOOK-HEBERGEMENT.md` — §4bis facture, §4ter incident
5 Go, §5 règles de migration), N°205 (R2), N°172 (PUT env-vars), N°165
(sentinel), N°166 (CA Supabase), `docs/RUNBOOK-POSTGRES.md` §11-12,
`docs/RUNBOOK-SECRETS.md` §2 (rotation), `docs/RUNBOOK-KEEPALIVE.md`
(Option A), `docs/RUNBOOK-WALLED-GARDEN.md` (domaines en dur).
