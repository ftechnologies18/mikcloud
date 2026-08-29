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

Le proxy `vercel.json` (rewrites `/api/*` → Render) fait tout le travail en
production : **aucune variable n'est requise**. Deux modes :

| Mode | Réglage | Quand l'utiliser |
| --- | --- | --- |
| **Proxy (défaut)** | rien — `vercel.json` transfère `/api/*` vers Render | Production `mikcloud.ftci.fr` (zéro CORS) |
| **Direct** | `.env.local` : `NEXT_PUBLIC_API_BASE=https://votre-service.onrender.com` + `ALLOWED_ORIGIN` côté Render | Si vous préférez contourner le proxy |

En local sans variable, l'app appelle `/api/...` en relatif (mode passerelle).

## Déploiement Vercel

Root Directory : `frontend` · Domaine : `mikcloud.ftci.fr` · Aucune variable
requise (proxy `vercel.json` auto-détecté). Vérifier l'URL Render dans le
`rewrites → destination` de `vercel.json` si votre service porte un autre nom.
Détails dans le [README racine](../README.md).

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
