import type { Metadata, Viewport } from "next";
import { Geist, Geist_Mono } from "next/font/google";
import { ThemeProvider } from "next-themes";
import "./globals.css";
import { Toaster } from "@/components/ui/sonner";
import { PWARegister } from "@/components/pwa-register";
import { QueryProvider } from "@/lib/hotspot/query";

// Perf B1 — préconnexion à l'API Render (mode direct) : le handshake TCP+TLS
// démarre pendant que l'utilisateur parcourt la vitrine / saisit son login,
// économisant ~100-200 ms sur le premier appel API. Rendu uniquement quand
// NEXT_PUBLIC_API_BASE est défini (build Vercel ; la CI construit sans).
const API_BASE = process.env.NEXT_PUBLIC_API_BASE;

const geistSans = Geist({
  variable: "--font-geist-sans",
  subsets: ["latin"],
});

const geistMono = Geist_Mono({
  variable: "--font-geist-mono",
  subsets: ["latin"],
});

export const metadata: Metadata = {
  title: "MikCloud — Gestion Hotspot MikroTik",
  description:
    "Plateforme cloud de gestion professionnelle de hotspot MikroTik : vouchers, utilisateurs, profils, sessions temps réel et revendeurs.",
  keywords: ["MikroTik", "hotspot", "RouterOS", "vouchers", "wifi", "billing"],
  // Favicon via conventions fichiers : src/app/icon.png + apple-icon.png.
  // N°8 — PWA installable (manifest via src/app/manifest.ts + /sw.js).
  appleWebApp: { capable: true, statusBarStyle: "black-translucent", title: "MikCloud" },
};

// N°8 — PWA : classe posée AVANT le premier paint quand l'app est lancée
// depuis l'écran d'accueil (standalone). Combinée à la règle
// `pwa-standalone:not(.pwa-ready)` de globals.css, elle masque le HTML
// prérendu de la vitrine pendant la fenêtre pré-hydratation : la PWA
// s'ouvre sur le fond nuit puis révèle l'écran de connexion (ou la console
// si une session est active) — zéro flash de landing, rendu natif.
const PWA_BOOT_SCRIPT = `try{if(matchMedia("(display-mode: standalone)").matches||navigator.standalone===true){document.documentElement.classList.add("pwa-standalone")}}catch(e){}`;

export const viewport: Viewport = {
  themeColor: [
    { media: "(prefers-color-scheme: dark)", color: "#101815" },
    { media: "(prefers-color-scheme: light)", color: "#f6faf7" },
  ],
  width: "device-width",
  initialScale: 1,
  maximumScale: 1,
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    // Nuit par défaut (identité MikCloud), bascule Jour via next-themes.
    <html lang="fr" suppressHydrationWarning>
      <body
        className={`${geistSans.variable} ${geistMono.variable} antialiased bg-background text-foreground`}
      >
        {/* Premier enfant du body : exécuté pendant le parsing, donc avant
         * tout paint du contenu — la vitrine n'apparaît jamais en PWA. */}
        <script dangerouslySetInnerHTML={{ __html: PWA_BOOT_SCRIPT }} />
        {API_BASE ? (
          <>
            <link rel="preconnect" href={API_BASE} crossOrigin="anonymous" />
            <link rel="dns-prefetch" href={API_BASE} />
          </>
        ) : null}
        <ThemeProvider attribute="class" defaultTheme="dark" enableSystem={false} disableTransitionOnChange>
          {/* QueryProvider au niveau du layout racine : UN seul QueryClient
           * partagé par toutes les routes (/, /login, /app, /sell) — le cache
           * TanStack Query survit aux navigations au lieu d'être recréé à
           * chaque changement de route. */}
          <QueryProvider>
            {children}
          </QueryProvider>
          <Toaster richColors position="top-right" closeButton />
          <PWARegister />
        </ThemeProvider>
      </body>
    </html>
  );
}
