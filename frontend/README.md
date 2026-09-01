# MikCloud — Frontend

Application Next.js 16 (App Router, React 19, TypeScript strict) — interface de
gestion hotspot MikroTik : dashboard temps réel, vouchers imprimables,
revendeurs, routeurs, rapports.

## Démarrage

```bash
bun install          # ou npm install
bun run dev          # http://localhost:3000
bun run lint
```

## Configuration

Le frontend appelle l'API Render **en absolu** (mode direct actif en
production). Deux modes :

| Mode | Réglage | Quand l'utiliser |
| --- | --- | --- |
| **Direct (actif)** | Vercel : `NEXT_PUBLIC_API_BASE=https://votre-service.onrender.com` + `ALLOWED_ORIGIN` côté Render | Production `mikcloud.ftci.fr` |
| **Proxy (alternative)** | rien — rétablir le `rewrites` dans `vercel.json` (transfère `/api/*` vers Render) | Si vous préférez zéro CORS / zéro variable |

En local sans variable, l'app appelle `/api/...` en relatif (mode passerelle).

## Déploiement Vercel

Root Directory : `frontend` · Domaine : `mikcloud.ftci.fr` · Variable :
`NEXT_PUBLIC_API_BASE=https://mikcloud.onrender.com` (Production + Preview) —
paire avec `ALLOWED_ORIGIN` côté Render. Détails dans le
[README racine](../README.md).

## Architecture

```
src/
├── app/                      # route unique "/" (SPA) + layout + thème
├── components/hotspot/       # login, shell, 9 vues, composants partagés
│   └── views/                # dashboard, sessions, users, vouchers, profils,
│                             # revendeurs, routeurs, rapports, paramètres
├── lib/hotspot/              # api (fetch + passerelle), store (zustand),
│                             # types (contrat API), format, query provider
└── components/ui/            # shadcn/ui (New York)
```
