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
