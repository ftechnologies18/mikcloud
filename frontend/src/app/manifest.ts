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
// - `screenshots` : captures RÉELLES de la production — Android remplace la
//   mini-infobar grise par le dialogue d'installation riche dès qu'au moins
//   une capture `narrow` existe ;
// - `shortcuts` : long-press sur l'icône → gestes quotidiens. Les routes
//   gardent leurs redirections d'authentification (pas de session → /login).
//
// N°192 — manifeste REMIS AU GOÛT DU JOUR pour refléter la console actuelle
// (N°184-191) :
// - description : la supervision multi-points d'accès, le portail
//   personnalisable et la loupe routeur remplacent la focalisation « Mode
//   Vente » d'origine — le dialogue d'installation vend le produit TEL
//   QU'IL EST ;
// - raccourci « Sessions » (/app/sessions) : le geste quotidien du
//   gestionnaire depuis la loupe routeur (N°191) — la vue s'ouvre prête à
//   filtrer par point d'accès (les chips vivent au-dessus de la table) ;
// - 5 captures RÉGÉNÉRÉES par ops/pwa-capture.ts (stack locale réelle :
//   backend JSON + seed complet, simulation vivante) dont DEUX écrans
//   console inédits : la loupe routeur côté Sessions (narrow) et le tableau
//   de bord multi-sites (wide). Dimensions VERROUILLÉES : 780×1688 narrow,
//   1280×800 wide — le manifeste les référence en dur.
export default function manifest(): MetadataRoute.Manifest {
  return {
    id: "/",
    name: "MikCloud — Hotspot & Ventes",
    short_name: "MikCloud",
    description:
      "Hotspot MikroTik multi-routeurs : sessions et vouchers par point d'accès, portail captif personnalisable, Mode Vente revendeur.",
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
        name: "Sessions",
        short_name: "Sessions",
        description: "Clients connectés, filtrables par point d'accès.",
        url: "/app/sessions",
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
        src: "/screenshots/console-sessions-narrow.jpg",
        sizes: "780x1688",
        type: "image/jpeg",
        form_factor: "narrow",
        label: "Sessions actives — loupe par point d'accès, badges vivants",
      },
      {
        src: "/screenshots/landing-narrow.jpg",
        sizes: "780x1688",
        type: "image/jpeg",
        form_factor: "narrow",
        label: "Vitrine MikCloud",
      },
      {
        src: "/screenshots/login-narrow.jpg",
        sizes: "780x1688",
        type: "image/jpeg",
        form_factor: "narrow",
        label: "Écran de connexion — console ou Mode Vente",
      },
      {
        src: "/screenshots/console-wide.jpg",
        sizes: "1280x800",
        type: "image/jpeg",
        form_factor: "wide",
        label: "Console — tableau de bord multi-sites",
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
