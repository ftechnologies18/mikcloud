# Kit du 1er octobre — migration Neon → Supabase + vague de déploiement

Outillage opérationnel de la séquence UNIQUE du 1er octobre
(`docs/RUNBOOK-POSTGRES.md` §11 — la référence canonique). Chaque script
couvre une étape numérotée du runbook ; tout ce qui n'est pas scripté est
une commande explicite ci-dessous.

Ces scripts ne contiennent **aucun secret** : ils lisent les variables
d'environnement, à défaut le coffre local `/home/z/.secrets` (hors dépôt).
Identifiants d'infrastructure embarqués (non secrets, déjà publics dans le
runbook) : service Render `srv-da974o142hec73euul60`, org Neon
`org-blue-forest-04016555`, projet Neon `long-feather-75906741`.

## Table des correspondances (§11 → kit)

| §11 | Étape | Outil |
|---|---|---|
| 1-3 | vérifications (quota levé, rattrapage, cible prête) | `preflight.sh` |
| 4 | migrer les données | dispatch `migrate-neon-supabase` (commande ci-dessous) |
| 5 | basculer l'env Render + ré-activer autoDeploy | `step5-render-flip.sh --exec` |
| 6 | fusionner n163 + n164, lever le sentinel, pousser | commandes git ci-dessous |
| 7 | vérifier le redémarrage unique | `preflight.sh` (post) + logs Render |
| 8 | armer secours + archive froide | `step8-flip-secret.py --exec` puis `step8-arm-standby.sh --exec` |

## Pré-vol (étapes 1-3)

```bash
bash ops/oct1/preflight.sh
```

Lecture seule. Le check **« SQL Neon : quota 53000 levé ? »** est le
GO/NO-GO de la journée ; la **carte Santé admin** (fin du rapport) montre
le rattrapage du syncreur (étape 2 : synchro ok, contact PG récent —
compter ~1-2 h après le reset pour rejouer les deltas accumulés depuis le
20/09). Dépendance SQL : `uv pip install "psycopg[binary]"` (auto-installée
par le script si `uv` est présent).

## Étape 4 — migration des données (~25 Mo, quelques minutes)

```bash
GHP="$(cat /home/z/.secrets/github-token.txt)"
curl -fsS -X POST \
  -H "Authorization: Bearer $GHP" -H "Accept: application/vnd.github+json" \
  -H "Content-Type: application/json" -d '{"ref":"main"}' \
  "https://api.github.com/repos/ftechnologies18/mikcloud/actions/workflows/migrate-neon-supabase/dispatches"
```

Puis suivre le run (onglet Actions). Vert = dump Neon → reset idempotent
Supabase → restore → comptages par table identiques → RLS posé. Le
workflow installe lui-même son client PostgreSQL 18 et sa racine TLS.

## Étape 5 — bascule env Render (AVANT la fusion)

```bash
bash ops/oct1/step5-render-flip.sh          # DRY-RUN d'abord
bash ops/oct1/step5-render-flip.sh --exec   # puis application
```

`DATABASE_URL` ← session pooler Supabase `:5432` (validations dures :
refus du `:6543`, de `sslmode=`, de `pgbouncer=`, de `channel_binding=`),
`NEON_KEEPALIVE=off`, `autoDeploy=yes`. Aucun effet sur le process en
cours — les variables s'appliquent au prochain démarrage. Un snapshot des
variables d'avant-bascule est écrit dans le coffre (rollback).

## Étape 6 — fusion + levée du gel en UNE vague

```bash
cd /path/du/clone && git checkout main && git pull --ff-only
git merge --no-edit n163-zikisso-repair
git merge --no-edit n164-persistence-safety
git rm RENDER-DEPLOY-FROZEN
git commit -m "Levée du gel N°165-b : sentinel RENDER-DEPLOY-FROZEN retiré dans la vague de fusion du 1er octobre (runbook §11 étape 6)"
git push origin main
```

⚠️ La levée du sentinel DOIT voyager dans la même poussée que les fusions
(le job deploy-render détecte les changements `backend/` par diff
`event.before…sha` et lit le sentinel au `sha` — un commit séparé sans
changement backend serait ignoré par la détection monorepo). CI verte →
l'UNIQUE redémarrage s'exécute sur la base Supabase migrée.

## Étape 7 — vérifications

- CI main verte (5 jobs) et déploiement Render `live` sur le commit de fusion ;
- logs Render : « store: état chargé depuis PostgreSQL » (plus de Fatal) ;
- `bash ops/oct1/preflight.sh` — carte Santé mode `postgresql`, synchro ok,
  DATABASE_URL Render → Supabase, sentinel absent ;
- premiers check-ins agents (commandes servies), portail, tickets — la vague
  d'autoréparation Zikisso part au premier read_state complet.

## Étape 8 — armer le secours et l'archive froide

```bash
python3 ops/oct1/step8-flip-secret.py --exec   # 8a : secret DATABASE_URL → Supabase
bash ops/oct1/step8-arm-standby.sh --exec      # 8b : backup on + dispatchs validation
```

Ordre impératif : 8a AVANT 8b (sinon backup exporterait l'ancienne base).
`step8-flip-secret.py` exige PyNaCl (`uv pip install pynacl`).

## Rollback (si problème dans les 24 h)

Le projet Neon contient l'état au 30/09 soir + le rattrapage du 1er au
matin : re-`PATCH`er `DATABASE_URL` Render avec le DSN du snapshot
avant-bascule (coffre `render-env-before-flip-*.json`), `autoDeploy=no`,
redéployer — le boot résilient N°164 absorbe la fenêtre (§11 rollback).
