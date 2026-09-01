# MikCloud — Hotspot API (backend Go)

Backend de la plateforme **MikCloud** : gestion professionnelle de hotspot MikroTik
(vouchers, utilisateurs, profils, sessions temps réel, revendeurs avec portefeuille).

- **Go stdlib + pgx uniquement** → binaire unique, déploiement trivial.
- **Agent HTTP-poll natif** : le routeur se connecte LUI-MÊME au cloud toutes les 45 s
  (connexions 100 % sortantes — Orange CI, CGNAT, Starlink : aucune IP publique requise).
- **Client RouterOS natif** : protocole binaire MikroTik (port 8728) implémenté à la main,
  login RouterOS v6.43+ et fallback challenge MD5 pour les anciens firmwares (mode direct LAN).
- **Mode Simulé** intégré : toute la plateforme est testable sans matériel MikroTik.
- **Persistance double** :
  - `DATABASE_URL` défini (production) → **PostgreSQL/Neon** : schéma relationnel
    (10 tables + index), chargement au démarrage, **synchro différentielle** à chaque
    sauvegarde (seules les lignes modifiées sont écrites, une transaction par Save) ;
  - sinon (développement) → fichier JSON (`data/db.json`, sauvegarde atomique).

## Démarrage local

```bash
go run .          # écoute sur :4000
# ou
go build -o server . && ./server
```

Variables d'environnement :

| Variable    | Défaut               | Description                        |
| ----------- | -------------------- | ---------------------------------- |
| `PORT`      | `4000`               | Port d'écoute HTTP                 |
| `DATA_DIR`  | `data`               | Dossier du fichier `db.json` (mode JSON uniquement) |
| `JWT_SECRET`| `mikcloud-dev-secret` | Secret de signature des tokens   |
| `DATABASE_URL` | — | URL `postgresql://…` → active la persistance PostgreSQL (Neon) |
| `ADMIN_USERNAME` | `admin` | Compte admin de production (avec `ADMIN_PASSWORD`) |
| `ADMIN_PASSWORD` | — | Mot de passe admin de production ; supprime le compte démo si défini |
| `ADMIN_NAME` | `Administrateur MikCloud` | Nom affiché du compte admin |

Démarrage sur base PostgreSQL vide : état de mise en service (compte + admin,
réglages, 3 gabarits de tickets — **aucune donnée de démonstration**).
Compte de développement (mode JSON local, seed démo) : **admin / admin123**.
`POST /api/admin/purge` supprime les données de test par catégories — aucun
endpoint ne régénère de données démo (l'ancien `/api/admin/reset` a été retiré :
il écrasait les données réelles, dont les routeurs agents).

### Neon (production, gratuit)

1. https://neon.tech → projet « mikcloud » (le plan Free 0,5 Go n'expire jamais)
2. Copier la connection string (`postgres://…sslmode=require`)
3. Render → Service mikcloud-api → Environment → `DATABASE_URL`

Au premier démarrage sur une base vide, l'état de mise en service (compte,
admin, réglages, gabarits — zéro démo) est écrit dans Neon. Ensuite l'état réel
est restauré à chaque redéploiement/sleep-wake. Vérifier dans les logs Render :
`store: état chargé depuis PostgreSQL`.

## Endpoints principaux

| Méthode | Chemin                              | Description                          |
| ------- | ----------------------------------- | ------------------------------------ |
| POST    | `/api/auth/login`                   | Connexion (JWT Bearer)               |
| GET     | `/api/dashboard`                    | KPIs, **vue multi-sites**, chronologies, activité |
| GET/POST/PUT/DELETE | `/api/routers` (+ `/:id/test`, `/:id/stats`) | Routeurs MikroTik |
| GET/POST/PUT/DELETE | `/api/profiles`    | Forfaits hotspot                     |
| GET/POST/PUT/DELETE | `/api/users` (+ enable/disable) | Utilisateurs & vouchers |
| POST    | `/api/vouchers/generate`            | Génération en lot (débit revendeur, lot tracé) |
| GET     | `/api/vouchers/batches`             | **Lots tracés** (stats live par statut) |
| POST    | `/api/vouchers/batch/:batchId/delete` | Suppression d'un lot entier        |
| POST    | `/api/vouchers/batch/:batchId/transfer` | **Transfert de stock (N°18)** : redistribue le stock vendable d'un lot vers un revendeur (débit portefeuille) ou vers le stock direct (retour, recrédit) — anti-fraude : les vendus ne bougent jamais |
| GET     | `/api/sessions` / DELETE `/:id`     | Sessions actives / kick              |
| GET/POST/PUT/DELETE | `/api/resellers` (+ `/:id/credit`, `/:id/settle`) | Revendeurs & portefeuille (liste enrichie : stock/vendus/attribués/**dette** live) ; **N°19** : mode de paiement par revendeur — prépayé ou dépôt-vente (plafond de créance), `settle` = encaissement d'un versement (créance −, cash au dashboard) |
| GET     | `/api/sell/me` / `/api/sell/stock` / `/api/sell/day-report` | Mode Vente revendeur (profil tournée, stock, **rapport de fin de journée**) |
| POST    | `/api/sell/:id/sold`                | Remise au client (traçabilité anti-vol N°8) |
| GET     | `/api/accounting?period=day\|week\|month&routerId=` | **Comptabilité multi-sites** (ventes/CA par jour, semaine, mois et par routeur) |
| GET     | `/api/reports?days=7\|14\|30`       | Rapports commerciaux & trafic        |
| GET/PUT | `/api/settings`                     | Organisation, devise, fuseau         |
| GET     | `/api/admin/purge/stats`            | Compteurs par catégorie de purge     |
| POST    | `/api/admin/purge`                  | Purge des données par catégories (`{"scopes":[…]}`, `"all"` accepté) |
| POST    | `/api/admin/wipe`                   | Purge totale (alias scope `all`)     |
| POST    | `/api/admin/reload`                 | Rechargement de l'état depuis la base |

## Connecter un vrai routeur MikroTik (mode Réel)

1. Winbox → **IP → Services** → activer **api** (port 8728).
2. **System → Users** → créer un utilisateur API avec le groupe **full**.
3. Dans MikCloud → Routeurs → *Ajouter* : IP du routeur, port 8728, identifiants, mode **Réel**.

Le client dialogue alors en direct avec le routeur : création/suppression d'utilisateurs
hotspot, activation/désactivation, sessions actives et déconnexion, supervision CPU/mémoire.

## Déploiement sur Render

1. Pousser ce dossier sur GitHub/GitLab.
2. Render → **New → Web Service** → connecter le dépôt.
   Render détecte `render.yaml` (runtime Go, plan gratuit).
3. Définir `DATABASE_URL` (connexion string Neon) et, recommandé,
   `ADMIN_USERNAME` / `ADMIN_PASSWORD`.
4. L'API sera servie sur `https://<votre-service>.onrender.com`.

> Les données vivent dans **Neon** (PostgreSQL serverless) : l'état est
> rechargé à chaque démarrage et synchronisé à chaque modification — le
> disque éphémère du plan gratuit Render n'est plus un problème.
>
> Détail du fonctionnement (`internal/store/pg.go`) : la mémoire reste le
> moteur de calcul (handlers inchangés), PostgreSQL est la source de vérité
> durable. Chaque `Save()` compare des empreintes par ligne (FNV-1a) et
> n'écrit que les différences en une transaction (upserts multi-lignes
> groupés, suppressions par lots) — suffisamment léger pour le polling 5 s
> du tableau de bord.

## Docker

```bash
docker build -t mikcloud-api .
docker run -d -p 4000:4000 -v mikcloud-data:/app/data mikcloud-api
```
