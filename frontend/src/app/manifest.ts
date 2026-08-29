import type { MetadataRoute } from "next";

// N°8 — Manifeste PWA : la console ET le Mode Vente s'installent sur l'écran
// d'accueil du revendeur (Android/iOS), plein écran, thème MikCloud.
export default function manifest(): MetadataRoute.Manifest {
  return {
    name: "MikCloud — Hotspot & Ventes",
    short_name: "MikCloud",
    description:
      "Gestion hotspot MikroTik + Mode Vente revendeur : stock de vouchers, remise au client, portefeuille.",
    start_url: "/",
    display: "standalone",
    orientation: "portrait",
    background_color: "#101012",
    theme_color: "#101012",
    lang: "fr",
    categories: ["business", "productivity"],
    icons: [
      { src: "/logo.png", sizes: "512x512", type: "image/png", purpose: "any" },
      { src: "/logo.png", sizes: "512x512", type: "image/png", purpose: "maskable" },
      { src: "/icon.png", sizes: "192x192", type: "image/png", purpose: "any" },
    ],
  };
}
