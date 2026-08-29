# MikCloud — Monorepo

**Plateforme SaaS de gestion professionnelle de hotspot MikroTik** (alternative
moderne à Mikhmon) : cloud multi-routeurs, revendeurs avec portefeuille de crédits,
vouchers imprimables et supervision temps réel.

```
mikcloud/
├── .github/workflows/  → CI (gofmt/vet/build Go + ESLint/build Next.js)
├── backend/    → API Go + PostgreSQL (pgx) — à déployer sur Render
└── frontend/   → Next.js 16 + Tailwind 4 + shadcn/ui — à déployer sur Vercel
```

| | Stack | Hébergement |
|---|---|---|
| **Backend** | Go 1.22 (net/http), JWT HS256, **PostgreSQL via pgx** (Neon), client natif RouterOS | [Render](https://render.com) (`backend/render.yaml` auto-détecté) |
| **Frontend** | Next.js 16 App Router, React 19, TypeScript strict, TanStack Query, Recharts | [Vercel](https://vercel.com) (1 variable d'env) |
| **Base de données** | PostgreSQL serverless ([Neon](https://neon.tech)) — schéma relationnel, synchro différentielle | — |

## Démarrage local

```bash
# 1. Backend (port 4000)
cd backend
go run .
# → http://localhost:4000  (compte démo : admin / admin123)

# 2. Frontend (port 3000)
cd frontend
bun install        # ou npm install
bun run dev        # → http://localhost:3000
```

Le frontend appelle le backend via `NEXT_PUBLIC_API_BASE` si défini
(`frontend/.env.example`), sinon en relatif `/api` (passerelle).

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
4. Recommandé : définir `ADMIN_USERNAME` / `ADMIN_PASSWORD` (sinon le compte
   démo `admin/admin123` reste actif).
5. URL obtenue : `https://<service>.onrender.com`.

> Les données vivent dans Neon : l'état est rechargé à chaque démarrage et
> synchronisé à chaque modification (synchro différentielle) — le disque
> éphémère du plan gratuit Render n'est plus un problème. En local sans
> `DATABASE_URL`, le backend utilise le fichier JSON `data/db.json`.

### 3. Frontend → Vercel
1. **Add New → Project** → ce repo, *Root Directory* : `frontend`.
2. Variable d'environnement (Production + Preview) :
   `NEXT_PUBLIC_API_BASE=https://<service>.onrender.com`
3. Déployer — le CORS est déjà ouvert côté Go.

### Connecter un vrai routeur MikroTik
1. Winbox → **IP → Services** → activer **api** (port 8728).
2. **System → Users** → créer un utilisateur API (groupe *full*).
3. MikCloud → **Routeurs → Ajouter** : IP, port 8728, identifiants, mode **Réel**.

Le client RouterOS parle le **protocole binaire natif** (login v6.43+ et fallback
challenge MD5) — aucun agent à installer sur le routeur. Un **mode Simulé**
intégré permet de démontrer toute la plateforme sans matériel.

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
| Rapports | **Comptabilité multi-sites** : ventes par jour/semaine/mois et par routeur (part de CA, panier moyen) + activité (revenus, ventes par profil, trafic, statut vouchers) |
| Paramètres | Organisation, devise (FCFA, EUR, USD…), fuseau, réinitialisation démo |

Documentation détaillée du backend : [`backend/README.md`](backend/README.md).

## Licence

© 2025 MikCloud — ftechnologies18
