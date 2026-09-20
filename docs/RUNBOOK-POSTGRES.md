# RUNBOOK — Persistance PostgreSQL (N°162) : incident quota Neon et sortie de crise

> Contexte : le 20/09/2026 vers 21:40 (heure Abidjan), la console Neon
> affiche l'épuisement du quota compute du plan gratuit — le compute est
> **suspendu**, la persistance est morte jusqu'au reset du 1er octobre.
> Ce runbook documente l'incident MESURÉ, pourquoi les optimisations de
> volume n'y pouvaient rien, les options vérifiées et la sortie de crise
> recommandée.

> **STATUT (20/09/2026 soir) : INCIDENT OUVERT** — compute Neon suspendu ;
> service Render UP (état complet en mémoire) ; **autoDeploy Render
> DÉSACTIVÉ** (§3, protection anti-crash) ; migration Supabase Free
> recommandée (§5), décision opérateur en attente.

## 1. L'incident — mesures réelles (API Neon + test direct)

| Mesure (org « FTech CI », projet `Mikcloud`, `aws-eu-central-1`, plan free) | Valeur au 20/09 soir |
|---|---|
| Temps d'éveil du compute (`active_time`, 1er→20/09) | 1 487 959 s ≈ **413 h** (~20,8 h/jour) |
| Compute facturable (`compute_time`) | 396 250 CU-s ≈ **110 CU-h** |
| Plafond du plan gratuit 2026 | **100 CU-h/mois/projet** → **franchi vers le 19-20/09** |
| Période de facturation (reset) | 2026-09-01 → **2026-10-01** (reprise 1er octobre 00:00 UTC) |
| Taille logique de la base | **~25 Mo** (plafond 0,5 Go — le stock n'a JAMAIS été le problème) |

Test direct (connexion au pooler Neon depuis le sandbox) :

```
ÉCHEC CONNEXION: Your account or project has exceeded the quota.
Upgrade your plan to increase limits.
```

État du backend Render au moment de l'incident : **UP**
(HTTP 200 en 0,23 s), dernier déploiement live = N°159 (`66a4812`)
le 19/09 à 05:20 UTC — donc **l'état complet vit en mémoire du backend**,
et la synchro échoue en continu depuis la suspension (tout est marqué
sale — `dirtyAll` — et se re-poussera à la première synchro réussie).

## 2. Pourquoi l'« objectif 0 coût » N°72-77 + N°157/N°159 n'y pouvait rien

Le plafond Neon gratuit 2026 ne facture pas le TRAVAIL mais le **TEMPS
D'ÉVEIL** du compute. MikCloud est précisément conçu pour un compute
toujours éveillé :

- les check-ins des agents (45 s ↔ 180 s) déclenchent `Save()` →
  `Sync()` → trafic continu 24/7 (les routeurs ne dorment pas la nuit) ;
- le keep-alive (fenêtre calme 4 min < autosuspend 5 min,
  `NEON_KEEPALIVE=business` par défaut) achève d'empêcher tout sommeil.

Mesure : ~5,5 CU-h/jour → **~165 CU-h/mois nécessaires** contre 100
offerts. L'écart (~1,65×) est structurel : il faudrait que le compute
dorme ≥ 11 h/jour, incompatible avec des agents qui pointent 24/7.
Aucune optimisation de VOLUME (N°72-77, N°157, N°159) ne change le
temps d'éveil.

⚠️ **Amendement au verdict N°161** : « l'objectif 0 coût tient » reste
vrai en **stock** (table commands bornée N°157 : ~25 Mo réels) et en
**flux CPU** (moteurs de volume démontés N°159), mais il doit être
précisé sur l'hébergement de la persistance : **pas sur le Neon
gratuit** — par changement de pricing de l'hébergeur (le keep-alive
avait été calibré sur l'ancien plafond 191,9 CU-h/mois, ~142 h
théoriques ; Neon est passé à 100 CU-h/projet/mois), non par
régression du produit. L'objectif reste tenable en migrant la
persistance vers un hébergeur dont le gratuit ne facture pas le temps
d'éveil (§4-A).

## 3. Protections posées pendant la fenêtre (20/09 → 1er octobre)

Deux risques pendant la suspension :

1. **Crash Render** = perte de tout l'état post-19/09 (la mémoire est
   seule détentrice) — couvert par le dirtyAll : à la reprise, tout se
   re-pousse ; mais entre-temps, plus aucune durabilité.
2. **Redéploiement** = pire : au boot, `store.New` → `OpenPG` → 10
   pings échoués → **erreur fatale** → le service démarre en
   crash-loop jusqu'au 1er octobre. Or l'auto-déploiement Render
   (trigger commit sur `main`) fait que TOUT push redémarre le
   backend — y compris un push de pure documentation, y compris d'une
   session parallèle.

**Protection posée le 20/09 au soir** (API Render, service `mikcloud`) :

```
PATCH /v1/services/srv-da974o142hec73euul60  {"autoDeploy":"no"}
→ autoDeploy = no, trigger = off
```

→ Les push vers GitHub redeviennent sans danger pour la production.
Les déploiements doivent être déclenchés MANUELLEMENT (dashboard
Render → « Manual Deploy » ou API) tant que la persistance n'est pas
rétablie.

⚠️ **À RÉACTIVER dès la fin de la crise** (§5 étape 6 / §7) :

```
PATCH /v1/services/srv-da974o142hec73euul60  {"autoDeploy":"yes"}
```
(ou dashboard Render → Settings → Auto-Deploy → Enable.)

## 4. Les options vérifiées (console réelle + grilles officielles 2026)

| Option | Coût récurrent | Verdict |
|---|---|---|
| **A. Supabase Free** | **0 $/mois** | ✅ **Recommandée** — viable dans la durée (§5) |
| B. Attendre le 1er octobre | 0 € | Risqué : 10 j de persistance morte + gel des déploiements (§7) |
| C. Neon **Lancement** (0,106 $/CU-h) | ~**18 $/mois** (~11 000 FCFA) avec la consommation mesurée | Correct si l'on reste chez Neon — **Échelle inutile** (§6) |
| D. Render PostgreSQL **Starter** | ~**6-7 $/mois** (1 Go) | La meilleure affaire PAYANTE durable, colocalisée au backend (§6) — naturelle au 1er client payant |

### A. Supabase Free — pourquoi c'est viable dans la durée

- **500 Mo** de base (25 Mo utilisés = **20× de marge**) ;
- **pas de quota d'heures compute** (CPU partagé) : l'instance ne dort
  pas — le profil MikCloud (requêtes par minute, 24/7) y est un atout ;
- pause UNIQUEMENT après **7 jours d'inactivité totale** — impossible
  avec des agents qui pointent 24/7 (c'est l'exact inverse du piège
  Neon : notre trafic constant protège au lieu de facturer) ;
- Postgres standard + pooler : **zéro changement de code** (driver pgx
  v5, `DATABASE_URL` à changer, `ensureSchema` recrée/contrôle les
  tables au boot) ;
- limites à connaître : **5 Go d'egress/mois** (2,96 Go mesurés sur
  Neon en 20 j — mais PRÉ-N°159 : les moteurs démontés représentaient
  ~80 % du trafic → attendu ~1-2 Go/mois → marge à surveiller le
  premier mois, cf. §8) ; 2 projets actifs maximum (1 suffit).

### B. Attendre le 1er octobre — ce que ça implique

Le reset (00:00 UTC le 01/10) réveille le compute ; la première
synchro du backend re-pousse tout l'état mémoire (dirtyAll) : la
persistance repart sans perte SI le backend n'a pas crashé entre
temps. Pendant 10 jours : zéro durabilité, zéro redéploiement
possible (autodeploy off + déploiements manuels interdits), et le
même mur se reproduira vers le **18-20 octobre** (rythme mesuré
~5,5 CU-h/jour). → Ne fait que décaler le problème : à réserver si
l'opérateur veut réfléchir à froid, pas une solution.

### C. Si payer Neon — la carte de la console

La capture du 20/09 (Facturation → Plan de mise à niveau) montre :
**Lancement** 0,106 $/CU-h, autoscale 16 CU, « ordinateur toujours
actif » (badge « le plus populaire ») vs **Échelle** 0,222 $/CU-h,
jusqu'à 56 CU, SOC 2/HIPAA, SLA 99,95 %, allowlist IP.

- **Choisir LANCEMENT** si l'on reste chez Neon — jamais Échelle
  (conformité entreprise sans objet pour MikCloud, 2× le prix) ;
- coût mesuré : ~165 CU-h/mois × 0,106 $ ≈ **17,5-18 $/mois**
  (+ ~0,05 $ de stockage) — tarification à l'usage, sans minimum
  mensuel affiché (« commencez gratuitement, payez à l'utilisation ») ;
- effet immédiat : le compute se réveille dès l'upgrade, la synchro
  dirtyAll rétablit la persistance SANS migration ni perte — d'où son
  usage comme **pont de migration** (§5 étape 1) : quelques jours
  d'usage = ~1-3 $ au passage.

### D. Render PostgreSQL — la vraie bonne affaire payante

- free : 1 Go mais **expire 30 jours après création** (14 j de grâce
  puis suppression) → éliminé pour la durée ;
- **Starter ~6-7 $/mois** (1 Go) : colocalisé avec le backend
  (latence minimale, même console/facture), **~3× moins cher** que
  Neon Lancement → c'est LA cible naturelle « au premier client
  payant » (remplace aussi le plan Render free du backend à cette
  occasion — cf. RUNBOOK-KEEPALIVE §Option B).

## 5. Plan recommandé — migrer vers Supabase Free (coût de passage ~1-3 $)

> Pourquoi un pont payant : le `pg_dump` de l'historique exige un
> compute Neon réveillé — or il est suspendu. Réveiller quelques jours
> via Lancement coûte ~0,58 $/jour et RÉTABLIT la persistance dès la
> minute qui suit (fin du risque de perte), le temps de migrer.

1. **Upgrade Neon → Lancement** (console → Facturation → Lancement →
   carte bancaire, ~5 min) → le compute se réveille → la synchro
   dirtyAll du backend re-pousse l'état → vérifier la carte « Santé de
   la persistance » (retour au vert) et les horodatages des tables
   chaudes.
2. **Créer le projet Supabase** (supabase.com, plan Free, région
   **Francfort / eu-central-1** — même voisinage que l'actuel) →
   Project Settings → Database → relever l'URL du **session pooler**
   (port 5432) `postgresql://postgres.<ref>:<mdp>@aws-0-<région>...
   .pooler.supabase.com:5432/postgres` (ne PAS utiliser le port 6543).
3. **Copier les données** (depuis le sandbox de guidage, avec
   l'accord de l'opérateur) : `pg_dump` Neon → restore Supabase —
   34 tables, ~25 Mo → quelques minutes. Alternative zéro outil :
   pointer temporairement le backend sur Supabase et utiliser
   l'export/recharge admin — le pg_dump reste la voie propre.
4. **Basculer le backend** : Render → Environment → `DATABASE_URL`
   = URL session pooler Supabase (garder `?sslmode=require`) + ajouter
   `NEON_KEEPALIVE=off` (le keep-alive n'a plus d'objet — Supabase ne
   suspend pas à 5 min) → « Manual Deploy » → au boot :
   `ensureSchema` + Load depuis Supabase → vérifier la carte Santé,
   un check-in d'agent et l'horodatage des tables.
5. **Valider 24-48 h** (carte Santé, check-ins, egress dans le
   dashboard Supabase) → supprimer le projet Neon (ou le garder vide
   en repli quelques jours) → **retour à 0 $/mois récurrent**.
6. **Réactiver l'auto-déploiement** Render (§3) — la crise est close.

## 6. Réponse à « si je devais payer, lequel sur la capture ? »

**Lancement** — sans hésitation. Échelle double le prix unitaire
(0,222 $/CU-h) pour des garanties d'entreprise (SOC 2, HIPAA, SLA
99,95 %, allowlist IP) sans objet pour MikCloud. Mais si la question
devient « payer durablement », la bonne réponse n'est pas sur la
capture : **Render PostgreSQL Starter (~6-7 $/mois)** fait la même
chose pour ~3× moins cher, colocalisé au backend (§4-D).

## 7. Si l'opérateur choisit d'attendre le 1er octobre

- ne rien redéployer (autodeploy déjà off ; PAS de Manual Deploy) ;
- le 01/10 après 00:00 UTC : vérifier la carte « Santé de la
  persistance » (le dirtyAll doit tout re-pousser en quelques
  cycles) puis **décider AVANT le ~18-20 octobre** (le mur se
  reproduira au même rythme : ~5,5 CU-h/jour mesurés) ;
- réactiver l'autoDeploy dès la persistance confirmée (§3).

## 8. Surveillance après migration Supabase (premier mois)

| Indicateur | Où | Seuil d'alerte |
|---|---|---|
| Taille de la base | dashboard Supabase → Settings → Database | > 350 Mo (70 % des 500 Mo gratuits) |
| Egress mensuel | dashboard Supabase → Reports/Usage | > 3,5 Go (70 % des 5 Go gratuits) |
| Carte « Santé de la persistance » | MikCloud (admin) | tout échec > 5 min |
| Synchro / check-ins | logs Render | retour des « store: synchro PostgreSQL différée échouée » |

Si un seuil est franchi au fil de la croissance du parc : Render
PostgreSQL Starter (~6-7 $/mois, §4-D) — idéalement au moment où le
premier client payant finance le basculement (et le backend Render
Starter avec, cf. RUNBOOK-KEEPALIVE).

## 9. Récapitulatif des actions déjà effectuées (20/09 au soir)

- diagnostic mesuré par API Neon (consumption org) + test de
  connexion direct (message de quota) + état du service Render (API) ;
- **autoDeploy Render désactivé** (patch API) — à réactiver en fin de
  crise (§3) ;
- aucune modification de code : la sortie de crise est purement
  opérateur (migration d'hébergeur ou upgrade), documentée ici.

## 10. Amendement N°164 — modèle de COHABITATION (décision opérateur du 21/09)

Le plan du §5 se termine par « supprimer le projet Neon ». **Décision finale
de l'opérateur : Neon est conservé comme secours** — le duo cohabite avec un
rôle chacun, à coût total 0 € :

| Rôle | Service | Cadence |
|---|---|---|
| **Production** (écritures 24/7 du backend) | Supabase Free | permanent |
| **Secours vivant** (copy restaurée) | Neon Free | 1×/jour (restore `standby-restore.yml`) |
| **Archive froide chiffrée** (AES-256-GCM) | artefacts GitHub | 1×/semaine (`backup.yml`, 90 j de rétention) |

Pourquoi pas une double-écriture simultanée : le moteur de synchro est
mono-primaire par conception, et surtout écrire en continu sur Neon
réveillerait son compute en continu — l'incident N°162 reconstitué. Le
secours doit être un restaurateur quotidien (batch), jamais un second
écrivain.

**Math du quota** : 1 réveil Neon/jour de 5-10 min ≈ 1-2 CU-h/mois
(plafond 100). Soutenable indéfiniment. NE JAMAIS passer le restore en
horaire (~30-60 CU-h/mois).

**RPO écrit noir sur blanc** : le secours a jusqu'à **24 h de retard** sur
la production. En cas de perte simultanée de la production ET du process
Render : l'état des **routeurs/utilisateurs hotspot est reconstituable par
les agents** (le routeur détient la vérité opérationnelle) ; l'**historique
de ventes/journal/annonces** dépend du backup (RPO 24 h). Cette distinction
est assumée.

**Bascule de secours** (production morte > quelques heures) : reprendre le
dump le plus frais (artefact `mikcloud-backup` OU base Neon elle-même) →
`pg_restore`/psql vers un nouveau projet Supabase (ou upgrade immédiat) →
pointer `DATABASE_URL` → déployer. Fenêtre ~15-30 min. Le boot résilient
N°164 couvre le démarrage pendant la fenêtre base-morte.

**Secrets GitHub à définir** (Settings → Secrets → Actions) :
- `SUPABASE_DATABASE_URL` — DSN production (session pooler `:5432`) ;
- `NEON_STANDBY_DATABASE_URL` — DSN secours, **endpoint DIRECT Neon**
  (SANS `-pooler` dans le nom d'hôte : le DDL massif sur pooler est
  déconseillé) ;
- (`BACKUP_KEY`/`DATABASE_URL` existent déjà pour `backup.yml` — après la
  bascule du 1er octobre, mettre `DATABASE_URL` à la valeur Supabase pour
  que l'archive hebdomadaire suive la production.)

## 11. Séquence du 1er octobre — migration + vague de déploiement (UNE SEULE)

Le redémarrage du service casse la mémoire (état orphelin depuis le 20/09) :
**tout se joue en une seule vague**, jamais avant le retour du quota Neon
(reset 1er octobre ~00:00 UTC).

1. **Vérifier le réveil Neon** : `psql "<DSN Neon>"` doit répondre (sinon
   attendre — le reset s'applique au fil des heures).
2. **Laisser le rattrapage se faire** (~1-2 h) : le syncreur en échec
   depuis le 20/09 rejoue les deltas accumulés. Vérifier : carte Santé →
   synchro OK + `commands.done_at` récents.
3. **Préparer Supabase** : opérateur fournit le DSN (Settings → Database →
   Connection string → URI, mode **Session pooler, port 5432** — l'endpoint
   direct `db.<ref>.supabase.co` est IPv6-only, Render sort en IPv4).
4. **Migrer les données** : `pg_dump --no-owner --no-privileges <Neon> |
   psql <Supabase>` puis contrôle d'intégrité (comptages par table —
   même méthode que `standby-restore.yml`). (~25 Mo : quelques minutes.)
5. **Fusionner les branches** : `n163-zikisso-repair` (correctif Zikisso :
   vérité du lot + autoréparation) puis `n164-persistence-safety` (boot
   résilient + garde anti-écrasement + carte Santé/bannière dégradée) dans
   `main`. CI verte.
6. **Basculer Render** : `DATABASE_URL` = DSN Supabase + `NEON_KEEPALIVE=off`
   (inutile et nuisible sur Supabase) ; réactiver l'autoDeploy (§3) ; le
   push de la fusion déploie la vague complète — UNIQUE redémarrement.
7. **Vérifier** : boot « état chargé depuis PostgreSQL » dans les logs
   Render, carte Santé verte (mode postgresql, synchro OK), agents qui
   checkent (commandes), portail client, tickets — la vague
   d'autoréparation Zikisso se déclenche au premier read_state complet.
8. **Armer le secours** : poser les secrets `SUPABASE_DATABASE_URL` +
   `NEON_STANDBY_DATABASE_URL` (§10), déclencher `standby-restore.yml`
   manuellement (workflow_dispatch) pour valider le premier restore, puis
   laisser le cron quotidien faire.

Rollback (si la production Supabase pose problème dans les 24 h) : le projet
Neon contient l'état au 30/09 au soir + le rattrapage du 1er au matin ;
repointer `DATABASE_URL` vers Neon et redéployer — le boot résilient N°164
absorbe la fenêtre de bascule sans Fatal.
