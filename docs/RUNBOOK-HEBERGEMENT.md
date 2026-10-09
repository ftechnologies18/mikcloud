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
