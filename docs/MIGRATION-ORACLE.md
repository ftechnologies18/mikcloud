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
6. **Re-provisionner les routeurs UN PAR UN** : console → Infrastructure →
   routeur → copier le script d'installation (il porte désormais
   `api.mikcloud.ftci.fr`) → coller dans Winbox → vérifier que le check-in
   revient. Un à la fois, validé, puis le suivant. Un routeur non
   re-provisionné continue sur `onrender.com` (toujours servi) — d'où la
   décommission après zéro trafic résiduel seulement.
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
(`ci.yml:217-226`). À poser au plus tard AVANT la suspension Render
(§9) ; dès T2 c'est déjà pertinent : sans lui, un push backend déploierait
sur Render ET sur la VM en parallèle. Après T2, `deploy-oracle.yml` est
LE chemin backend ; Vercel déploie le frontend via son webhook.

## 7. Coffre-fort Neon — snapshot nocturne

Neon (l'ancienne base de secours) est recyclée en **coffre-fort** : chaque
nuit à **03:00 UTC** (= 03:00 à Abidjan), le timer `mikcloud-backup.timer`
exécute `backup-neon.sh` :

```
pg_dump --no-owner --no-privileges --clean --if-exists "$DATABASE_URL_SESSION" | psql "$NEON_DATABASE_URL" -v ON_ERROR_STOP=1
```

- source : le DSN Supabase **session pooler `:5432`** — le transactionnel
  `:6543` ne fonctionne PAS pour `pg_dump` ;
- `--clean --if-exists` rend le snapshot rejouable nuit après nuit ;
- le script **journalise** (`journalctl -u mikcloud-backup`) et **propage
  le code retour** : un échec se voit en échec systemd, pas en silence.

**Limite** : Neon gratuit = 0,5 Go — la base fait quelques Mo (37 tables
différentielles), marge large ; vérifier la première exécution (§8-11).
C'est un filet, PAS une standby — la primaire reste Supabase.

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

Références : N°211 (audit : état mémoire, URLs gravées, 27 variables),
N°207-209 (`docs/RUNBOOK-HEBERGEMENT.md` — §4bis facture, §4ter incident
5 Go, §5 règles de migration), N°205 (R2), N°172 (PUT env-vars), N°165
(sentinel), N°166 (CA Supabase), `docs/RUNBOOK-POSTGRES.md` §11-12,
`docs/RUNBOOK-SECRETS.md` §2 (rotation), `docs/RUNBOOK-KEEPALIVE.md`
(Option A), `docs/RUNBOOK-WALLED-GARDEN.md` (domaines en dur).
