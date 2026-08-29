# MikCloud — Hotspot API (backend Go)

Backend de la plateforme **MikCloud** : gestion professionnelle de hotspot MikroTik
(vouchers, utilisateurs, profils, sessions temps réel, revendeurs avec portefeuille).

- **100 % stdlib Go** (zéro dépendance externe) → binaire unique, déploiement trivial.
- **Client RouterOS natif** : protocole binaire MikroTik (port 8728) implémenté à la main,
  login RouterOS v6.43+ et fallback challenge MD5 pour les anciens firmwares.
- **Mode Simulé** intégré : toute la plateforme est testable sans matériel MikroTik.
- **Persistance** : fichier JSON (`data/db.json`, sauvegarde atomique).

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
| `DATA_DIR`  | `data`               | Dossier du fichier `db.json`       |
| `JWT_SECRET`| `mikcloud-dev-secret` | Secret de signature des tokens   |

Compte démo (seed) : **admin / admin123**. `POST /api/admin/reset` régénère les données de démo.

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
| GET     | `/api/sessions` / DELETE `/:id`     | Sessions actives / kick              |
| GET/POST/PUT/DELETE | `/api/resellers` (+ `/:id/credit`) | Revendeurs & portefeuille |
| GET     | `/api/accounting?period=day\|week\|month&routerId=` | **Comptabilité multi-sites** (ventes/CA par jour, semaine, mois et par routeur) |
| GET     | `/api/reports?days=7\|14\|30`       | Rapports commerciaux & trafic        |
| GET/PUT | `/api/settings`                     | Organisation, devise, fuseau         |
| POST    | `/api/admin/reset`                  | Réinitialisation des données démo    |

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
3. Définir `JWT_SECRET` (généré automatiquement par `render.yaml`).
4. L'API sera servie sur `https://<votre-service>.onrender.com`.

> ⚠️ Le plan gratuit Render a un **système de fichiers éphèmère** : `db.json` est
> réinitialisé à chaque redéploiement. Pour la production : monter un disque persistant
> (décommenter `disk:` dans `render.yaml`, plan payant) ou brancher PostgreSQL
> (remplacer `internal/store` par un driver `pgx` — l'architecture est isolée derrière
> le package store).

## Docker

```bash
docker build -t mikcloud-api .
docker run -d -p 4000:4000 -v mikcloud-data:/app/data mikcloud-api
```
