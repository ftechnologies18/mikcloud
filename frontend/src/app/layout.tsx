import type { Metadata, Viewport } from "next";
import { Geist, Geist_Mono } from "next/font/google";
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
  themeColor: "#101012",
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
    <html lang="fr" className="dark" suppressHydrationWarning>
      <body
        className={`${geistSans.variable} ${geistMono.variable} antialiased bg-background text-foreground`}
      >
        {children}
        <Toaster richColors position="top-right" closeButton />
        <PWARegister />
      </body>
    </html>
  );
}
