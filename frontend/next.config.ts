import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  /* N°38 — suppression de output: "standalone" : sur Vercel, le builder
     attend .next/next-server.js.nft.json (traçage Node) que le build
     Turbopack + standalone ne produit plus au bon emplacement → ENOENT
     systématique en fin de build (8 déploiements production en échec
     depuis le 06/09 01:00, même avec cache purgé). Standalone n'a aucune
     utilité ici : le frontend est servi par le runtime géré Vercel (aucun
     Dockerfile frontend) et l'E2E Playwright démarre via `next start`.
     Le mode dev (`bun run dev`) n'est pas impacté. */
  /* N°84 — ignoreBuildErrors retiré (héritage du template initial 8cd2e40,
     jamais une décision du projet — N°38 l'avait laissé « intact » sans le
     questionner) : or le webhook Vercel déploie dès le push `main`,
     indépendamment de la CI — avec ce flag, une régression de types
     atteignait la production avant même que le job tsgo de la CI ne
     rougisse. Le build type-check désormais (tsc), en plus du tsgo de la
     CI — triple barrière locale + CI + build. */
  reactStrictMode: false,
  allowedDevOrigins: ["*.space-z.ai", "localhost", "127.0.0.1"],
};

export default nextConfig;
