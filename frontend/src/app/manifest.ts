import type { MetadataRoute } from "next";

// N°8 — Manifeste PWA : la console ET le Mode Vente s'installent sur l'écran
// d'accueil du revendeur (Android/iOS), plein écran, thème MikCloud.
//
// N°60 — « Installation riche » (audit PWA, action 2) :
// - `id` : identité STABLE de l'app. Sans lui, un futur changement de
//   start_url/scope créerait une seconde icône chez les revendeurs déjà
//   installés ;
// - `launch_handler` : focus sur l'instance existante plutôt qu'une nouvelle
//   fenêtre quand un lien MikCloud est ouvert depuis WhatsApp/le navigateur ;
// - `screenshots` : 3 captures RÉELLES de la production (login mobile,
//   vitrine mobile, login desktop) — Android remplace la mini-infobar grise
//   par le dialogue d'installation riche dès qu'au moins une capture
//   `narrow` existe ;
// - `shortcuts` : long-press sur l'icône → « Mode Vente » (/sell, le geste
//   quotidien du revendeur) et « Console » (/app). Les routes gardent leurs
//   redirections d'authentification (pas de session → /login).
export default function manifest(): MetadataRoute.Manifest {
  return {
    id: "/",
    name: "MikCloud — Hotspot & Ventes",
    short_name: "MikCloud",
    description:
      "Gestion hotspot MikroTik + Mode Vente revendeur : stock de vouchers, remise au client, portefeuille.",
    start_url: "/",
    scope: "/",
    display: "standalone",
    orientation: "portrait",
    background_color: "#101012",
    theme_color: "#101012",
    lang: "fr",
    categories: ["business", "productivity"],
    launch_handler: { client_mode: "focus-existing" },
    icons: [
      { src: "/logo.png", sizes: "512x512", type: "image/png", purpose: "any" },
      { src: "/logo.png", sizes: "512x512", type: "image/png", purpose: "maskable" },
      { src: "/icon.png", sizes: "192x192", type: "image/png", purpose: "any" },
    ],
    shortcuts: [
      {
        name: "Mode Vente",
        short_name: "Vente",
        description: "Comptoir revendeur : stock de vouchers et remise au client.",
        url: "/sell",
        icons: [{ src: "/icon.png", sizes: "192x192", type: "image/png" }],
      },
      {
        name: "Console",
        short_name: "Console",
        description: "Supervision hotspot : tableau de bord et sessions actives.",
        url: "/app",
        icons: [{ src: "/icon.png", sizes: "192x192", type: "image/png" }],
      },
    ],
    screenshots: [
      {
        src: "/screenshots/login-narrow.jpg",
        sizes: "780x1688",
        type: "image/jpeg",
        form_factor: "narrow",
        label: "Écran de connexion — console ou Mode Vente",
      },
      {
        src: "/screenshots/landing-narrow.jpg",
        sizes: "780x1688",
        type: "image/jpeg",
        form_factor: "narrow",
        label: "Vitrine MikCloud",
      },
      {
        src: "/screenshots/login-wide.jpg",
        sizes: "1280x800",
        type: "image/jpeg",
        form_factor: "wide",
        label: "Connexion — vue desktop",
      },
    ],
  };
}
