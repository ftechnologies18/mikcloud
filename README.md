# MikCloud — Monorepo

**Plateforme SaaS de gestion professionnelle de hotspot MikroTik** (alternative
moderne à Mikhmon) : cloud multi-routeurs, revendeurs avec portefeuille de crédits,
vouchers imprimables et supervision temps réel.

```
mikcloud/
├── backend/    → API Go (100 % stdlib) — à déployer sur Render
└── frontend/   → Next.js 16 + Tailwind 4 + shadcn/ui — à déployer sur Vercel
```

| | Stack | Hébergement |
|---|---|---|
| **Backend** | Go (net/http uniquement), JWT HS256, persistance JSON atomique, client natif RouterOS | [Render](https://render.com) (`backend/render.yaml` auto-détecté) |
| **Frontend** | Next.js 16 App Router, React 19, TypeScript strict, TanStack Query, Recharts | [Vercel](https://vercel.com) (1 variable d'env) |

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

### Backend → Render
1. **New → Web Service** → ce repo, *Root Directory* : `backend`
   (le fichier `backend/render.yaml` est auto-détecté : runtime Go, build
   `go build -o server .`, start `./server`).
2. `JWT_SECRET` est générée automatiquement.
3. URL obtenue : `https://<service>.onrender.com`.

> Plan gratuit Render = disque éphémère (données réinitialisées à chaque
> redéploiement). Production : monter un disque persistant ou brancher
> PostgreSQL (accès données isolé dans `backend/internal/store`).

### Frontend → Vercel
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
| Tableau de bord | KPIs live, sessions 24 h, revenus 14 j, top profils, activité |
| Sessions actives | Table temps réel (poll 5 s), déconnexion (kick) instantanée |
| Utilisateurs | CRUD, filtres, pagination, activation/désactivation, copie identifiants |
| Vouchers | Génération en lot (1–500), préfixe/longueur de code, débit auto du portefeuille revendeur, impression tickets prédécoupés |
| Profils | Forfaits : débit (format RouterOS), durée session, validité, quota, prix |
| Revendeurs | Portefeuille de crédits, rechargements, journal des transactions |
| Routeurs | Statut/CPU/uptime, test de connexion, mode simulé ou réel |
| Rapports | Revenus, ventes par profil, trafic réseau, statut vouchers (7/14/30 j) |
| Paramètres | Organisation, devise (FCFA, EUR, USD…), fuseau, réinitialisation démo |

Documentation détaillée du backend : [`backend/README.md`](backend/README.md).

## Licence

© 2025 MikCloud — ftechnologies18
