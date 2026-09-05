import type { Metadata } from "next";

import { WifiGuestPage } from "@/components/wifi/wifi-guest-page";

// N°27 — WiFi Jetable : page publique d'un établissement (QR de table).
// Portail d'accès SANS authentification : téléphone (marketing opt-in) →
// code gratuit avec copie → bascule vers le hotspot. noindex : ce sont des
// pages utilitaires par établissement, pas du contenu à indexer.
export const metadata: Metadata = {
  title: "WiFi Offert — MikCloud",
  robots: { index: false, follow: false },
};

export default async function WifiPublicRoute({
  params,
}: {
  params: Promise<{ slug: string }>;
}) {
  const { slug } = await params;
  return <WifiGuestPage slug={slug} />;
}
