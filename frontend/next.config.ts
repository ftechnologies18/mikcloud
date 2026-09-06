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
  typescript: {
    ignoreBuildErrors: true,
  },
  reactStrictMode: false,
  allowedDevOrigins: ["*.space-z.ai", "localhost", "127.0.0.1"],
};

export default nextConfig;
