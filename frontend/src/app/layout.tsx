import type { Metadata, Viewport } from "next";
import { Geist, Geist_Mono } from "next/font/google";
import { ThemeProvider } from "next-themes";
import "./globals.css";
import { Toaster } from "@/components/ui/sonner";
import { PWARegister } from "@/components/pwa-register";

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
        <ThemeProvider attribute="class" defaultTheme="dark" enableSystem={false} disableTransitionOnChange>
          {children}
          <Toaster richColors position="top-right" closeButton />
          <PWARegister />
        </ThemeProvider>
      </body>
    </html>
  );
}
