# RUNBOOK — Hébergement backend : pourquoi Render, et jusqu'à quand (N°207)

> Document opérateur MikCloud. Objectif : consigner la décision
> d'hébergement du backend Go avec les **chiffres vérifiés au feu** —
> pour que la question « et si on migrait vers X ? » ne soit jamais
> re-débatue de zéro. Établi le 2026-10-02 après analyse comparative
> fly.io et Stormkit demandée par l'exploitant.

## 0. La carte d'hébergement actuelle (rappel)

| Brique | Plateforme | Plan | Coût | Rôle |
|---|---|---|---|---|
| Frontend Next.js | **Vercel** | Hobby | 0 € | Console gérant + portail invité |
| Backend Go | **Render** (service `mikcloud`) | Free | 0 € | API agents + HTTP-poll + balayages horaires |
| Base PostgreSQL | **Supabase** | (pooler 6543) | 0 € | Données persistantes — 37 tables différentielles |
| Médias (bannières) | **Cloudflare R2** + domaine public `media.ftci.fr` | Free | 0 € | Sorties gratuites par construction (N°205) |

## 1. Les quotas Render gratuit qui nous concernent (vérifiés 2026)

| Quota | Valeur | Situation mikcloud | Marge |
|---|---|---|---|
| Heures d'instance | **750 h/mois** par workspace | 744 h (mois de 31 j) — service **toujours éveillé** | **6 h** |
| Mise en veille | 15 min sans trafic entrant | **Immune** : les agents MikroTik poussent toutes les 45–240 s | — |
| Bande passante sortante | **5 Go/mois**, puis 0,15 $/Go | ~2-3 Go/mois projetés post-N°205 (médias déviés vers R2) | ~2× |
| Build minutes | 500/mois | ~230 min consommées en septembre (~242 builds, médiane 1,1 min) | ~2× |
| RAM / CPU | 512 Mo / 0,1 CPU | Charge Go très en deçà | large |

**La facture de 13 $ de septembre** (voir N°205) était la dette de l'ère
pré-optimisation (bannières 2 Mo × chargements portail servis par le proxy
Render) — pas une fuite courante. Après la sortie des médias vers R2, la
facture attendue est **0 $**.

## 2. Comparatif vérifié — octobre 2026

### 2.1 fly.io

| Critère | Verdict |
|---|---|
| Offre gratuite nouveaux comptes | ❌ **Supprimée** (oct. 2024) : essai 2 h/7 jours, puis PAYG **avec carte bancaire**. Seuls les comptes « Legacy Hobby » (antérieurs à oct. 2024) gardent 3 VM 256 Mo gratuites |
| Coût réel équivalent mikcloud | ~**4–5 $/mois** : VM shared-cpu-1x 256 Mo ≈ 2,19 $ + IPv4 dédié 2 $ + volumes |
| RAM | 256 Mo (moitié du Render gratuit) |
| Always-on | ✅ Vrai (pas de mise en veille forcée) |
| TCP/UDP + réseau privé WireGuard | ✅ (seule capacité que Render ne donnera jamais — pertinente le jour d'un lien direct routeurs↔backend) |
| CI/CD | Dockerfile + fly.toml + GitHub Actions **à créer de zéro** (pas d'auto-déploiement natif) |
| Stabilité des termes | Faible : 3 changements de tarification en 3 ans |

### 2.2 Stormkit.io (cloud)

L'offre **Cloud gratuite existe** (corrigé après vérification navigateur —
la page pricing est rendue côté client et la FAQ publique date de 2023) :

| | Free | Premium | Ultimate |
|---|---|---|---|
| Prix | **0 $/mois** | 20 $/mois | 100 $/mois |
| Build minutes | 300 | 1 000 | 5 000 |
| Bande passante | **100 Go** | 1 To | 5 To |
| Stockage | 100 Go | 1 To | 5 To |
| Invocations fonctions | 500 000 | 1,5 M | 5 M |

**MAIS le modèle d'exécution est rédhibitoire pour mikcloud** — citation
de leur doc officielle (`/docs/deployments/application-runtime`) :

> « Stormkit can run long-running server processes (for example Go HTTP
> servers) by using the Start command setting. **This option is available
> only on self-hosted Stormkit instances.** »

Sur le cloud Stormkit : fonctions serverless **Node.js/TypeScript
uniquement** (AWS Lambda, timeout **15 s**). Le Go natif — même à 100
$/mois — n'y tourne pas. Or le backend mikcloud est :

1. **Un binaire Go longue durée** (`ListenAndServe`, goroutines de fond
   au boot : `RunRetentionSweepForever` — rétention + gel mensuel D3,
   `RunChatSweepForever`, `RunAnnouncementSweepForever`) ;
2. **À état en mémoire sous verrou global** (sessions agents vivantes,
   httpstats, pinlock, signup_abuse, mode simulé Tick) — une Lambda est
   stateless, chaque invocation repart de zéro ;
3. HTTP-poll uniquement (aucun WebSocket — vérifié `main.go`), seul point
   qui aurait été compatible.

Migrer vers Stormkit Cloud = **réécriture complète** (Go → Node/TS
serverless, état externalisé, boucles → Periodic Triggers), pas une
migration.

### 2.3 Stormkit.io (self-hosted)

| Critère | Verdict |
|---|---|
| Go natif longue durée | ✅ `go build -o dist/app` + Start command — exactement la forme mikcloud |
| Process tué après | 10 min **sans requête** — mikcloud pollé toutes les 45–240 s : jamais tué |
| Logiciel | Siège gratuit (self-hosted) |
| Infrastructure | **À fournir soi-même** — VPS Hetzner ≈ 4–5 €/mois minimum |
| Charge opérationnelle | OS, TLS, monitoring, backups, mises à jour de Stormkit lui-même |

### 2.4 Synthèse

| Plateforme | Gratuit ? | Go natif ? | Verdict mikcloud |
|---|---|---|---|
| **Render free** | ✅ 0 $ | ✅ build natif | **Statu quo** — optimal coût/risque |
| fly.io | ❌ ~4-5 $/mois | ✅ | Refusé (coût + pipeline à reconstruire) |
| Stormkit cloud | ✅ 0 $ (100 Go BP) | ❌ Node/TS serverless 15 s | **Impossible** sans réécriture totale |
| Stormkit self-hosted | Logiciel oui, serveur non | ✅ | Option de maturité future (~5 €/mois VPS) |

## 3. La décision

**Rester sur Render gratuit.** La décision est arithmétique :

> Migrer = payer ~4–5 €/mois + plusieurs jours de travail + risque prod
> (5 comptes, ~6 151 utilisateurs), pour un gain de 0 €.

Les quotas généreux de Stormkit (100 Go de bande passante) feraient sens
si notre problème était la bande passante — or ce problème est **résolu
par architecture** depuis le N°205 (médias → R2). Le guetteur restant
n'est pas la bande passante mais **les heures d'instance**.

## 4. Déclencheurs et escalade (règles actives)

| Déclencheur | Règle | Action |
|---|---|---|
| **Matin du 29/10** (et de chaque mois à 29 j) | Heures instance ≥ **745 h** consommées | Upgrade **Starter** (~7 $/mois, prorata ~1-2 $ pour 2 jours) — le chemin le moins cher et le moins risqué, jamais une migration |
| Facture non nulle malgré N°205 | Vérifier `dashboard → Metrics → bandwidth` | Identifier la catégorie qui dérive (agents ? JSON portail ? console ?) avant tout geste |
| Besoin TCP direct routeurs↔backend | — | C'est LE cas d'usage fly.io (réseau privé WireGuard) — rouvrir le dossier à ce moment-là seulement |
| Reprise de contrôle total de l'infra | — | Stormkit self-hosted sur VPS (~5 €/mois) — option de maturité, pas d'actualité |

## 4bis. Plan de continuité — facture Render impayée (établi le 2026-10-02)

**Situation** : la facture de septembre (~13 $, dette de l'ère pré-R2,
cf. N°205) est en attente, sans trésorerie pour la régler immédiatement.
Politique Render vérifiée (FAQ officielle) : sans moyen de paiement →
« disables your services for the duration of the current billing
period » (désactivation, PAS suppression) ; avec moyen de paiement en
échec → suspension réversible après relances. **Dans tous les cas :
aucune donnée n'est perdue** — le code vit sur GitHub, les données dans
Supabase, les médias dans R2. Payer la facture réactive le service.

**Rayon d'explosion vérifié dans le code (2026-10-02) si le backend
tombe** :

| Ce qui CONTINUE de marcher | Preuve technique |
|---|---|
| Login des invités | `login.html` poste vers `$(link-login-only)` en CHAP-MD5 (`md5.js`) — authentification **directement contre le routeur**, jamais via Render |
| Comptes/vouchers existants | Poussés en utilisateurs hotspot **locaux** (`/ip/hotspot/user/add`, gateway.go) — autonomes au routeur |
| Sessions en cours | Gérées par le routeur MikroTik |
| Pages du portail | **Cuites sur les routeurs** (hotspot_files) — chargement local |
| Bannières/logo | Servis par R2 public `media.ftci.fr` (N°205) — hors Render |

| Ce qui S'ARRÊTE | Impact |
|---|---|
| Console gérant | Lecture/écriture impossibles (API down) |
| Nouvelles ventes/vouchers | Création impossible (les existants restent vendables en local si déjà générés) |
| Inscriptions QR `/join/{token}` | Formulaire public injoignable |
| Sync agents (check-in 45-240 s) | Routeurs retentent en autonomie — convergeront au retour de l'API |
| WhatsApp/Telegram, revendeurs | Plateformes inaccessibles |

**Actions ordonnées** :
1. **Aujourd'hui (gratuit)** : courrier au support Render (dashboard →
   Help, ou support@render.com) — dossier honnête : excédent causé par
   les bannières servies via le proxy (corrigé : médias migrés vers R2,
   usage courant ~1 Go/mois < 5 Go inclus), demande de waiver goodwill
   ou d'échéancier. Précedent : le support Render est humain et les
   annulations de premier excès avec cause racine corrigée sont
   plausibles.
2. **Trouver les ~13 $ une fois** (~8 000 FCFA) : recharge de la carte
   en échec, carte prépayée virtuelle, ou proche cartes + remboursement
   mobile money. La dette est UNIQUE : octobre projette ~850 Mo
   (compteur N°72 : 4,1 Mo en 3,6 h le 02/10) < 5 Go inclus → **0 $
   de facture après règlement**.
3. **Si suspension entre-temps** : le WiFi continue (tableau ci-dessus),
   payer → réactivation. Ne rien improviser sous pression.
4. **Échappatoire long terme si Render devient un vrai problème** :
   **Oracle Cloud Always Free** — gratuite à vie, VM Linux, Go natif :
   4 OCPU ARM + 24 Go RAM (retauré 4/24 le 09/10/2026 — N°272,
   enveloppe max Always Free ; réduit de 4/24 Go en juin 2026 — reste
   ~20× la RAM Render) et surtout **10 To de bande passante sortante
   incluse/mois** (2 000× Render). Contraintes : carte bancaire exigée
   à l'inscription pour vérification (prélèvement temporaire ~1-2 $
   remboursé), instances ARM parfois rares à réserver (retenter),
   DevOps à charge (systemd, TLS via Caddy, sauvegardes). fly.io et
   Stormkit ne résolvent PAS le cas « zéro trésorerie » (fly = payant
   dès le 1er jour ; Stormkit cloud n'exécute pas Go ; Stormkit
   self-hosted = serveur à louer).

## 4ter. Résolu le jour même — le mail « 5 GB atteints » du 02/10 (N°209)

**L'incident** : mail Render « You've used all of the 5 GB » reçu le
02/10 après-midi. Diagnostic complet mené au feu (API Render + compteur
N°72 + base production) :

1. **La plus grosse part était la queue de l'ère pré-fix** : le correctif
   R2 (N°205) n'est parti que le **02/10 à 04 h 49 UTC** — le 1er octobre
   et la matinée ont tourné à l'ancien régime (~1,7 Go/jour de bannières
   par le proxy) : ~4,9 Go brûlés avant même le déploiement.
2. **Une fuite résiduelle** : le compteur post-déploiement montrait
   encore **medias = 83,7 Mo en 8,5 h** — N°205 ne réécrivait que
   logo + bannière ; les **slides du carrousel** (N°136) et les
   **imageUrl des promos** (N°54) restaient servis par le proxy
   (compte pilote : une slide de 403 Ko × chaque chargement de portail).

**Le correctif N°209** (commit 6c5b5d6, LIVE 12 h 57 UTC) :
- réécriture de TOUTES les images vers R2 public (slides + promos,
  contrat d'identité stricte — rien à réécrire ⇒ string inchangée) ;
- bump d'empreinte v2 → v3 : re-déploiement unique des pages portail
  sur les routeurs (vague de convergence ~3 Mo observée au compteur) ;
- **le proxy devient un aiguilleur** : 302 vers R2 public, clé validée
  avant redirection (zéro redirection ouverte) — toute requête résiduelle
  coûte ~300 octets au lieu de ~230 Ko.

**Vérifié en production** : proxy → 302 + 0 octet transféré (la même
requête streamait 287 836 o avant) ; cible R2 → 200 ; compteur post-fix :
**medias 0 o**. Projection : ~150-300 Mo/mois tout compris < 5 Go →
**facture 0 $ dès novembre**. Octobre terminera à ~5,2 Go → surcoût
≈ 0,03-0,08 $ (la dette réelle reste les ~13 $ de septembre).

**Leçon consignée** : quand on déplace une catégorie de trafic hors d'un
tuyau facturé, l'inventaire des sources doit être EXHAUSTIF (logo,
bannière, slides, promos, favicon…) — le compteur par catégorie (N°72)
est ce qui a permis de détecter la fuite résiduelle en 15 minutes ;
le garder vert est le garde-fou permanent.

## 5. Si une migration a lieu un jour quand même

- **Garder le domaine API identique** (bascule DNS uniquement) : les
  routeurs ont l'URL et le walled-garden **en dur** dans leur config —
  changer de domaine imposerait de reconfigurer toute la flotte.
- Les vérifications de N°202 (pipeline standby-restore) restent le modèle :
  restaurer sur le nouveau main et compter les 37 tables avant bascule.
- Les secrets (ADMIN_PASSWORD, Supabase pooler, R2, Telegram…) sont listés
  dans `docs/RUNBOOK-SECRETS.md` — rotation complète recommandée à
  l'occasion d'un changement de plateforme.

## 6. Pièges de recherche consignés (leçons de vérification)

1. **curl ne rend pas le JavaScript** : la page pricing Stormkit est une
   coquille vide côté serveur (95 Ko sans les chiffres) — un runbook basé
   sur curl seul conclurait « pas d'offre gratuite » à tort. Vérifier au
   navigateur headless (rendu complet) avant de trancher.
2. **Les FAQ de blog périment** : la FAQ Stormkit 2023 (« Why there is no
   free tier? ») contredit l'offre 2026 — dater les sources avant de citer.
3. **« Supporte le Go » ≠ « Go sur leur cloud »** : Stormkit, fly.io et
   autres annoncent des runtimes multi-langages — vérifier TOUJOURS si le
   runtime visé tourne sur l'offre gratuite cloud, sur le payant, ou
   uniquement en self-hosted. Les trois réponses diffèrent parfois.
4. **Les quotas évoluent** : Render est passé de 100 Go à 5 Go de bande
   passante gratuite en 2026 ; fly.io a supprimé son gratuit en 2024.
   Ce runbook est daté — re-vérifier les chiffres avant toute décision
   future.

Références : N°205 (diagnostic facture + R2), N°72-77/157/159 (optimisation
volume), N°202 (pipeline standby-restore), `docs/RUNBOOK-SECRETS.md`
(rotation des secrets), `docs/RUNBOOK-WALLED-GARDEN.md` (domaines en dur
sur les routeurs).

## §7 — Migration Oracle : le plan d'exécution existe (2026-10-02, N°211)

L'option Oracle du §4bis est désormais outillée de bout en bout : runbook
pas-à-pas (`docs/MIGRATION-ORACLE.md`), kit `deploy/oracle/` (bootstrap,
systemd, Caddy, CA Supabase, coffre-fort Neon) et workflow CI/CD
`deploy-oracle.yml`. Stratégie de domaine en deux temps (custom domain
Render → bascule DNS) pour une coupure ZÉRO. À dérouler quand
l'exploitant décide de quitter Render — les déclencheurs du §4 restent
la référence pour le timing.

## §8 — Docker sur Ftechci : terrain multi-services (Phase A, N°279)

> Décision exploitant « Go phase A » (2026-10-09) : la VM est le serveur
> multi-services de l'entreprise (N°271) — Docker prépare l'isolation des
> prochains back-ends. Le backend mikcloud (systemd) et PostgreSQL
> (primaire Tier1) ne sont PAS concernés.

### État posé (workflow `ops-docker`, mode `install` idempotent)
- **Docker 29.1.3 + Compose 2.40.3** — découverts DÉJÀ PRÉSENTS sur la
  VM (l'install a été idempotente, zéro conflit) ;
- **`/etc/docker/daemon.json`** : rotation des logs conteneurs
  **3 × 10 Mo** (leçon disque N°275 — un log qui creuse = incident) +
  `live-restore: true` (un restart du daemon ne tue pas les conteneurs) ;
- **Smoke test validé** : pull arm64, publication `127.0.0.1:4010`,
  HTTP 200, DNAT iptables OK, egress bridge OK, conteneur jetable purgé.

### Conteneur préexistant (CONFIRMÉ exploitant, N°282)
- `sect-api` (ghcr.io/udevrard7/sect/sect-api) — healthy, publié sur
  **127.0.0.1:8090**, policy de restart. **Installé par l'exploitant
  lui-même pendant l'installation de son second back-end** (confirmation
  N°282 — c'est CE second back-end) ; il vit hors du canal CI mikcloud
  et nous ne le touchons pas. Le monitor DR (famille n°10) le compte
  (« docker: actif (N conteneurs) »).
- Suggestion (sans urgence, décision exploitant) : à sa prochaine
  recréation, l'aligner sur la convention n°4 ci-dessous (limites
  `--memory`/`--cpus` — il partage la VM avec PostgreSQL).

### Conventions pour tout nouveau service (Phase B incluse)
1. **Publication TOUJOURS sur 127.0.0.1** (`-p 127.0.0.1:P:P`) — Caddy
   reste l'unique porte 80/443 ; la security list OCI n'expose que
   22/80/443 mais on ne compte PAS dessus ;
2. **Config par `--env-file /etc/mikcloud/<service>.env`** (600 root) —
   jamais de secrets dans l'image ni en argument (`ps`) ;
3. **Rollback** : garder l'image précédente taguée, swap + restart
   (~10 s) ;
4. **Limites cgroups** (`--memory`, `--cpus`) dès qu'un service
   partage la VM avec PostgreSQL — un service qui fuit ne doit JAMAIS
   affamer le primaire ;
5. **NE JAMAIS conteneuriser PostgreSQL** ni les timers/scripts DR —
   la chaîne WAL/archive_command/timers vit sur l'hôte
   (cf. docs/DB-HYBRIDE.md).

### Outil d'exploitation
`ops-docker.yml` (workflow_dispatch) : `install` (idempotent) /
`status` (lecture seule) / `prune` (**confirm=MIK-DOCKER-PRUNE** —
JAMAIS `--volumes`, les volumes portent des données de services).

### Phase B — RÉALISÉE (N°281, 09/10) : le backend mikcloud tourne SOUS DOCKER
- **Transport actif : docker** — conteneur `mikcloud-server`
  (`--network host`, publication directe sur `127.0.0.1:4000` → Caddy
  et le monitor DR inchangés ; `--env-file /etc/mikcloud/mikcloud.env`
  → blindage DSN N°274 intact ; `--memory 4g --cpus 3` → PostgreSQL à
  l'abri ; `--restart unless-stopped`). Image distroless du binaire
  statique existant, buildée sur la VM à chaque deploy.
- **Unité systemd `mikcloud` arrêtée + désactivée mais CONSERVÉE**
  (unité + binaire) — rollback = dispatch `deploy-oracle` avec
  `deploy_mode=systemd` ; rollback d'image instantané = tag `previous`.
- **`deploy-oracle.yml` est transport-aware** : input `deploy_mode`
  (`auto` = suit le transport actif — les pushs auto-deploy ne changent
  de rien ; `docker` / `systemd` = gestes de bascule explicites).
- **Monitor DR** : `backend: 200 (docker)` ou `(systemd)` — le signal
  reste HTTP 200 sur 127.0.0.1:4000, transport-agnostique (N°281).
- Healthcheck post-deploy mesuré : **10 s** (PG local) contre ~1 min à
  l'ère Johannesburg.

### Leçon du 09/10 (N°282) : poweroff invité ↔ OCI « RUNNING fantôme »
- Le « hang » du 09/10 n'en était pas un : **poweroff ACPI volontaire
  de l'exploitant** (« Power key pressed short », 05:13:23) pendant
  l'installation du second back-end → guest éteint proprement à
  05:13:28, état d'instance OCI resté **RUNNING ~77 min** sans rien
  relancer, jusqu'au RESET dur via `ops-vm-diag` (06:24, API verte
  06:31). Zéro signal noyau sur les 10 boots de la nuit.
- **Règle d'exploitation** : ne JAMAIS éteindre le guest depuis
  l'intérieur (`shutdown -h`, power key console) — préférer
  **`sudo reboot`** (tous les reboots propres de la nuit sont revenus
  seuls) ou l'action **Stop/Start de la console OCI** (réconcilie
  l'état hyperviseur). Si un poweroff invité arrive malgré tout :
  vérifier l'état OCI juste après, **Start** manuel si RUNNING fantôme ;
  rattrapage = `ops-vm-diag` (START/RESET + capture console PATIENTE).
- Angle mort assumé : le monitor Telegram vit sur la VM — un guest
  éteint ne peut pas alerter lui-même. D'où la règle ci-dessus.

## §9 — WireGuard sur Ftechci : accès privé + full-tunnel (N°284, voie A hôte-native)

> Décision exploitant « Go — full tunnel Wi-Fi + accès services, complet
> et évolutif, bonnes pratiques, réseaux Always Free » (2026-10-09).
> **Voie A retenue : wg-quick hôte pur** — aucune UI tierce, aucun
> conteneur NET_ADMIN ; les IP sources des clients (10.8.0.0/24)
> arrivent RÉELLES sur les services de l'hôte → l'accès services se
> règle d'une seule règle INPUT scoping `wg0`, et les services internes
> futurs n'ont jamais besoin d'un port public.

### Architecture posée (premier install vert, run 38000922564)
- **`wg-quick@wg0`** : `10.8.0.1/24` (+ ULA `fd00:8::1/64`), UDP 51820,
  clé serveur sous `/etc/wireguard/` (700/600, fingerprint `fd9ef476471a…`) ;
- **PostUp/PostDown iptables GARDES** (`-C || -I/-A`) : INPUT 1 + FORWARD
  1/2 `wg0`, MASQUERADE `10.8.0.0/24` → interface de sortie réelle
  **`enp0s6`** (résolue à l'install — PAS `eth0` : un guide générique
  aurait posé un MASQUERADE mort) ; cohabite avec les chaînes Docker ;
- **sysctl `net.ipv4.ip_forward=1`** persisté
  `/etc/sysctl.d/99-mik-wireguard.conf` ;
- **`/opt/wireguard/`** (700 root) : `endpoint` (IP publique
  enregistrée), `peers/` (confs clients 600), `wg-peer.sh`
  (add/remove/qr/reip/list) ;
- **PresharedKey par peer** (renfort symétrique) +
  `PersistentKeepalive 25` (clients derrière NAT opérateur) ;
- **Full-tunnel clients** : `AllowedIPs = 0.0.0.0/0, ::/0`, DNS 1.1.1.1,
  MTU 1420 — l'IPv6 sans route upstream est **blackholé dans le tunnel**
  (zéro fuite ; si des sites v6-only manquent un jour : activer l'IPv6
  du VCN = geste console séparé).

### Modèle de sécurité (les 5 règles)
1. **Clés jamais dans les logs** (dépôt PUBLIC) : confs clients 600
   root, récupérées par SSH (`sudo cat` ou `sudo qrencode -t ansiutf8
   < /opt/wireguard/peers/<nom>.conf`) ;
2. **PostgreSQL reste sur 127.0.0.1** (armure N°274 INTACTE) — le VPN
   n'atteint que ce qui écoute sur wg0/0.0.0.0 ; pour PG : SSH (direct
   ou à travers le tunnel), jamais d'écoute PG sur le VPN ;
3. **Le tunnel est crypto-silencieux** (aucune réponse aux paquets non
   authentifiés) — c'est la seule surface publique ajoutée ;
4. **Caddy reste l'unique porte 80/443** — les futurs services internes
   = bind wg0/0.0.0.0 sur port NON public (protégés par l'absence de
   Security List + la règle INPUT wg0) ;
5. **C'est le primaire DR** — tout geste est réversible : `prune`
   (gardé `MIK-WG-PRUNE`) démonte tout (⚠ détruit les clés : tous les
   clients à recréer ensuite).

### Cycle de vie des peers (évolutif, zéro interruption)
- **Ajout** : dispatch `ops-wg` `mode=peer-add` `peer_name=telephone`
  (nom [a-z0-9-], 1-32) → conf écrite → en SSH : `sudo qrencode -t
  ansiutf8 < /opt/wireguard/peers/telephone.conf` → scan par l'app
  WireGuard (iOS/Android/macOS/Windows) → full-tunnel actif ;
- **Listing/état** : `mode=status` (peers, handshakes, transferts,
  dérive IP publique, posture iptables/sysctl, coexistence backend) ;
- **Révocation** : `mode=peer-remove` (bloc + conf supprimés via
  `wg syncconf` — les autres peers ne perdent PAS le tunnel) ;
- **Changement d'IP publique** : `sudo /opt/wireguard/wg-peer.sh reip
  <nouvelle-ip>` (conf clients mises à jour, clés conservées) ;
- **Monitor DR famille 11** : « wireguard: UP (N peers) » au heartbeat
  06:00 UTC ; alerte si wg0 DOWN quand installé.

### Gestes exploitant OCI (console, hors canal CI — gratuits)
1. **Security List** : Ingress **UDP 51820** src `0.0.0.0/0` (ou CIDR
   restreint si IP sortante semi-stable) ;
2. **RÉSERVER l'IP publique** — une IP éphémère change au stop/start
   (le resize N°272 en a fait un) ; si l'IP change malgré tout : `reip` ;
3. **Depuis l'appareil** : tester le full-tunnel (IP publique vue =
   Marseille) puis l'accès services (https://mikcloud.ftci.fr à travers
   le tunnel ; SSH à travers le tunnel pour l'admin).

## §10 — Routeurs peers WireGuard : le renfort du mode agent (N°285)

> Décision « renforcer le mode de connexion des routeurs MikroTik pour
> MikCloud » (2026-10-09). Constat : le mode `agent` (check-in HTTPS
> sortant 45 s, TLS strict, token haché) est le SEUL canal de contrôle et
> le mode `real` (API RouterOS 8728 directe) exige un routeur joignable
> publiquement — inutilisable derrière le CGNAT des opérateurs. WireGuard
> (déjà posé côté VM, §9) ajoute par routeur un **deuxième chemin direct
> chiffré**, opt-in et révocable.

### La doctrine (les 3 invariants)
1. **Le mode agent reste LE SOCLE** — le tunnel n'est JAMAIS sur le chemin
   critique du check-in : aucune réécriture de scheduler, aucun DNS static,
   aucune dépendance circulaire. Un tunnel mort dégrade le RENFORT, jamais
   le contrôle (le routeur reste géré au pas 45 s). C'est l'anti-orphan
   N°230 appliqué au tunnel : configurer le tunnel REQUIERT le canal qui
   survit à sa panne.
2. **La clé privée ne quitte jamais le routeur** — `wg_keygen` fait générer
   la paire SUR l'appareil (RouterOS ≥ 7.15, paquet wireguard natif) et ne
   rapporte que la clé publique. La PSK (renfort symétrique, pattern §9)
   transite une seule fois, sur le canal TLS strict de l'agent — même
   surface que le token agent lui-même.
3. **Le tunnel est un chemin de GESTION, pas une porte** — allowed-address
   du peer routeur = `10.8.0.1/32` uniquement (l'hôte MikCloud). Ni le LAN
   du routeur ni l'internet n'est atteignable à travers lui ; le routeur
   n'expose TOUJOURS zéro port public.

### Cycle de vie (les 5 étapes, console + dispatch)
| # | Acteur | Geste | Résultat |
|---|---|---|---|
| 1 | Gérant | Fiche routeur → carte « Tunnel WireGuard » → **Activer** | commande `wg_keygen` filée ; au check-in ≤ 45 s le routeur crée `mikcloud-wg` et rapporte sa clé publique (état `ready`) |
| 2 | Exploitant | dispatch `ops-wg` `mode=peer-add-router` `peer_name=<suggéré>` `router_pubkey=<clé>` | peer embarqué côté VM (PSK neuve) + **livret** `/opt/wireguard/peers/<nom>.router.txt` (600 root : address/server_pub/psk/endpoint) |
| 3 | Exploitant | en SSH : `sudo cat …/<nom>.router.txt` | les 4 valeurs à coller dans la console (jamais dans un log/chat) |
| 4 | Gérant | console → coller les 4 valeurs → **Livrer** | `wg_setup` filée ; au check-in le routeur configure peer + adresse et échoe `peers=1` → état **`active`** |
| 5 | Gérant | **Tester le tunnel** | dial direct depuis la VM : `10.8.0.N:8728` (API RouterOS) puis `8291` (Winbox) — « tunnel UP mais API fermée » est rapporté honnêtement |
| — | Gérant/Exploitant | **Désactiver** = `wg-disable` (démontage routeur) + dispatch `peer-remove` (révocation serveur) | les deux côtés sont propres ; la paire de clés du routeur est détruite |

### Ce que ça change (les « possibilités » du routeur)
- **Diagnostic direct** : la VM sait si le routeur est joignable en direct
  (latence réelle, indépendante du check-in) — c'est déjà intégré au
  bouton « Tester » de la fiche (verdict agent + verdict tunnel).
- **Porte vers le pilotage direct** : avec l'API RouterOS (8728, activée
  par défaut) joignable à travers le tunnel, la gateway `real` EXISTANTE
  (`internal/routeros`, protocole binaire) peut un jour piloter ce routeur
  sans IP publique ni 45 s d'attente — l'activation par routeur (creds
  RouterOS côté cloud) est l'évolution suivante, volontairement HORS
  N°285 pour limiter le rayon.
- **Admin d'urgence** : Winbox/SSH à travers le tunnel (routeur sans
  aucun port public), pattern §9.
- **Évolutions notées (pas dans N°285)** : check-in privé via tunnel
  (DNS static — à proscrire tant que le failover n'existe pas dans le
  scheduler), read_state v3 rapportant l'état WG (bump de version de
  script = vague de re-déploiement, à budgéter), gateway `real` par
  tunnel (creds à demander au gérant).

### Discipline et pièges consignés
- **Les logs ne portent que du non-secret** (noms, adresses tunnel,
  fingerprints) : le livret `.router.txt` est 600 root, lu par SSH
  (`sudo cat`), JAMAIS collé dans un chat (dépôt public) ;
- **Anti-collision d'adresses** : le cloud refuse deux routeurs avec la
  même `10.8.0.N` (pool GLOBAL à la VM — deux comptes différents aussi) ;
- **Un routeur sans wireguard (ROS < 7.15)** rapporte une erreur claire,
  l'état passe `error` et AUCUNE commande ne se re-file toute seule
  (opt-in : c'est le gérant qui relance) ;
- **La PSK vit chiffrée au repos** (secretbox, pattern Password P0 #6) et
  ne sort JAMAIS (ni `json:"-"`, ni `sanitizeRouter` — double barrière) ;
- **wg-peer.sh v2 est rétro-compatible** : le mode `peer-add-router`
  re-pose le script à chaque dispatch (upgrade à chaud du v1 N°284,
  peers full-tunnel existants intacts) ; `remove` nettoie conf client ET
  livret ; `reip` met à jour les deux formes.
