# MikCloud — Monorepo

**Plateforme SaaS de gestion professionnelle de hotspot MikroTik** (alternative
moderne à Mikhmon) : cloud multi-routeurs, revendeurs avec portefeuille de crédits,
vouchers imprimables et supervision temps réel.

> **État : préparation au lancement commercial.** Données de démonstration
> supprimées du code (aucun seed, aucune donnée inventée), CI active sur chaque
> push, déploiements automatiques Vercel + Render.

```
mikcloud/
├── .github/workflows/  → CI (gofmt/vet/test/build Go + ESLint/build Next.js)
├── backend/    → API Go + PostgreSQL (pgx) — déployée sur Render
│   └── internal/       → api (handlers), auth (JWT), model, store (Neon/JSON),
│                         routeros (protocole binaire), agent, notify
├── frontend/   → Next.js 16 + Tailwind 4 + shadcn/ui — déployé sur Vercel
└── docs/       → CONTRACT-V2.md (source de vérité API), audit, guides
```

| | Stack | Hébergement |
|---|---|---|
| **Backend** | Go 1.27 (net/http), JWT HS256, **PostgreSQL via pgx** (Neon), client natif RouterOS | [Render](https://render.com) (`backend/render.yaml` auto-détecté) |
| **Frontend** | Next.js 16 App Router, React 19, TypeScript strict, TanStack Query, Recharts | [Vercel](https://vercel.com) (1 variable d'env) |
| **Base de données** | PostgreSQL serverless ([Neon](https://neon.tech)) — schéma relationnel, synchro différentielle | — |

## Démarrage local (nouveau développeur)

Prérequis : **Go 1.27+** et **Bun** (ou Node 20+). Aucune autre dépendance.

```bash
# 1. Backend (port 4000)
cd backend
go run .            # ou : bun run dev
# → http://localhost:4000  (1er démarrage : mot de passe admin aléatoire affiché dans les logs)

# 2. Frontend (port 3000)
cd frontend
bun install         # ou npm install
bun run dev         # → http://localhost:3000
```

- **Base vide = état de mise en service** : le système démarre sans aucune
  donnée (le seed de démonstration a été supprimé du code). Au premier
  démarrage, l'administrateur est créé avec un **mot de passe aléatoire,
  affiché une seule fois dans les logs** — plus aucun identifiant par défaut
  n'existe dans le repo (sécurité P0).
- Sans `DATABASE_URL`, le backend persiste dans le fichier JSON
  `backend/data/db.json` (développement) ; avec Neon, la synchro est
  différentielle à chaque sauvegarde.
- En local, renseigner `NEXT_PUBLIC_API_BASE=http://localhost:4000` côté
  frontend (CORS ouvert par défaut quand `ALLOWED_ORIGIN` est vide).

Le frontend appelle le backend via `NEXT_PUBLIC_API_BASE` si défini
(`frontend/.env.example`), sinon en relatif `/api` (passerelle).

## Qualité & CI

`.github/workflows/ci.yml` s'exécute sur chaque push `main` et chaque pull
request :

| Job | Vérifications |
|---|---|
| **Backend Go** | `gofmt` (formatage canonique) · `go vet` (analyse statique) · `go test` · `go build` |
| **Frontend Next.js** | `bun install --frozen-lockfile` · ESLint · `next build` |
| **deploy-render** | après CI verte sur `main` : déclenche le déploiement Render via API (secret `RENDER_API_KEY`) |

Conventions d'édition partagées : [`.editorconfig`](.editorconfig).
Identité des commits : `ftechnologies18 <freelancetechnologies.ci@gmail.com>`.

Avant de pousser :

```bash
cd backend   && gofmt -l . && go vet ./... && go test ./...
cd frontend  && bun run lint
```

> Chaque push `main` = mise en production (Vercel + Render). Vérifier
> localement AVANT de pousser ; toute correction passe par un commit de suivi.

## Déploiement

### 1. Base de données → Neon
1. Créer un compte sur [neon.tech](https://neon.tech) → **New Project**.
2. Copier la **Connection string** (`postgresql://…?sslmode=require`).

### 2. Backend → Render
1. **New → Web Service** → ce repo, *Root Directory* : `backend`
   (le fichier `backend/render.yaml` est auto-détecté : runtime Go, build
   `go build -o server .`, start `./server`).
2. `JWT_SECRET` est générée automatiquement.
3. Ajouter la variable d'environnement `DATABASE_URL` (secret) avec la
   connexion string Neon.
4. Obligatoire : définir `ADMIN_PASSWORD` (fort) — sur une base PostgreSQL
   vide, le service refuse de démarrer sans elle (sécurité P0 : plus aucun
   identifiant par défaut connu du repo).
5. URL obtenue : `https://<service>.onrender.com`.

> Les données vivent dans Neon : l'état est rechargé à chaque démarrage et
> synchronisé à chaque modification (synchro différentielle) — le disque
> éphémère du plan gratuit Render n'est plus un problème. En local sans
> `DATABASE_URL`, le backend utilise le fichier JSON `data/db.json`.

### 3. Frontend → Vercel (mikcloud.ftci.fr)

Le frontend appelle l'API Render **en absolu** (mode direct actif) :
`NEXT_PUBLIC_API_BASE=https://<service>.onrender.com` est définie dans
Vercel → Settings → Environment Variables (Production + Preview), et Render
liste les origines autorisées dans `ALLOWED_ORIGIN` (CORS).

1. **Add New → Project** → ce repo, *Root Directory* : `frontend`.
2. **Domaine** : ajouter `mikcloud.ftci.fr` (Settings → Domains) — déjà fait ✅.
3. **Variables** Vercel : `NEXT_PUBLIC_API_BASE=https://mikcloud.onrender.com`
   (Production + Preview).
4. **CORS** Render : `ALLOWED_ORIGIN=https://mikcloud.ftci.fr`
   (+ `https://<projet>.vercel.app` pour les previews, séparés par des virgules).

**Mode proxy (alternative)** — URLs relatives `/api/*` transférées vers Render
par un rewrite : zéro variable d'environnement, zéro CORS. Pour basculer :
rétablir le bloc `rewrites` de `frontend/vercel.json`
(`"destination": "https://<votre-service>.onrender.com/api/:path*"`),
retirer `NEXT_PUBLIC_API_BASE` de Vercel, vider `ALLOWED_ORIGIN` côté Render
et ajuster les points conditionnés à `NEXT_PUBLIC_API_BASE` (bloc démo de
l'écran de connexion, URL des factures imprimables — cf. `api.ts`).

> Récapitulatif des paramètres — **Vercel** : `NEXT_PUBLIC_API_BASE`
> (mode direct actif) · **Render** : `DATABASE_URL` (Neon), `JWT_SECRET`
> (auto), `ADMIN_USERNAME`/`ADMIN_PASSWORD`, et `ALLOWED_ORIGIN`
> (mode direct).

### Connecter un vrai routeur MikroTik
1. Winbox → **IP → Services** → activer **api** (port 8728).
2. **System → Users** → créer un utilisateur API (groupe *full*).
3. MikCloud → **Routeurs → Ajouter** : IP, port 8728, identifiants, mode **Réel**.

Le client RouterOS parle le **protocole binaire natif** (login v6.43+ et fallback
challenge MD5) — aucun agent à installer sur le routeur. Un **mode Simulé**
intégré permet de démontrer toute la plateforme sans matériel.

## Administration des données

- **Neon = source de vérité en production** (synchro différentielle, une
  transaction par sauvegarde).
- **Aucun endpoint ne régénère de données de démonstration** — le seed a été
  supprimé du code ; toute base vide démarre en état de mise en service.
- **Purge** : Paramètres → *Purge des données* par catégories (`POST
  /api/admin/purge`, compteurs live `GET /api/admin/purge/stats`) ;
  `POST /api/admin/purge-demo` retire chirurgicalement les artefacts hérités
  de l'ancien seed (routeurs simulés, revendeurs res-1…res-5, lots/ventes
  émis par ces routeurs) en préservant les données réelles.

## Fonctionnalités

| Module | Détail |
| --- | --- |
| Tableau de bord | KPIs live, **vue d'ensemble multi-sites** (1 compte = N hotspots : sessions, ventes, revenus par site), sessions 24 h, revenus 14 j, top profils, activité |
| Sessions actives | Table temps réel (poll 5 s), déconnexion (kick) instantanée |
| Utilisateurs | CRUD, filtres, pagination, activation/désactivation, copie identifiants |
| Vouchers | Génération en lot (1–500), préfixe/longueur de code, débit auto du portefeuille revendeur, impression tickets prédécoupés, **traçabilité des lots** (onglet Lots : site émetteur, canal, revendeur, statuts voucher par voucher) |
| Profils | Forfaits : débit (format RouterOS), durée session, validité, quota, prix |
| Revendeurs | Portefeuille de crédits, rechargements, journal des transactions |
| Routeurs | Statut/CPU/uptime, test de connexion, mode simulé ou réel |
| Rapports | **Comptabilité multi-sites** : ventes par jour/semaine/mois et par routeur (part de CA, panier moyen) + activité (revenus, ventes par profil, connexions réelles, statut vouchers) |
| Paramètres | Organisation, devise (FCFA, EUR, USD…), fuseau, purge des données par catégories |

## Documentation

| Fichier | Contenu |
|---|---|
| [`backend/README.md`](backend/README.md) | API : persistance double, agent HTTP-poll, protocole RouterOS |
| [`frontend/README.md`](frontend/README.md) | Configuration Vercel, modes d'appel API (direct / passerelle) |
| [`docs/CONTRACT-V2.md`](docs/CONTRACT-V2.md) | **SOURCE DE VÉRITÉ** — contrats d'API des fonctionnalités (champs, routes, sémantique) |
| [`AUDIT-MIKHMON-V3.md`](AUDIT-MIKHMON-V3.md) | Audit de référence par rapport à Mikhmon |
| [`GUIDE-GESTIONNAIRE.md`](GUIDE-GESTIONNAIRE.md) | Guide utilisateur du gestionnaire |
| [`CHANGELOG.md`](CHANGELOG.md) | Historique des évolutions notables |

## Feuille de route (qualité)

- [ ] Tests automatisés backend Go et frontend — la CI exécute déjà `go test`
- [x] Découpage de `backend/internal/api/handlers.go` (~5 100 lignes) en
      modules plus fins — sans changer le contrat d'API *(fait : routes.go,
      middleware.go, helpers.go et 12 fichiers handlers_<domaine>.go)*
- [x] Même traitement pour `agent_handlers.go` (1 349 l.) et
      `handlers_p1.go` (1 152 l.) *(fait : protocole agent, agent_results.go,
      handlers_provision.go, puis les vagues F6–F10 — ipbindings, commands,
      router_tools, scheduler ; plus aucun fichier > 900 lignes)*
- [ ] Monitoring externe (uptime, alertes) et sauvegardes Neon planifiées

## Licence

© 2026 FTCI — Freelance Technologies Côte d'Ivoire
