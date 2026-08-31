"use client";

import { useState } from "react";
import { QueryProvider } from "@/lib/hotspot/query";
import { useHotspotStore } from "@/lib/hotspot/store";
import LoginScreen from "@/components/hotspot/login-screen";
import AppShell from "@/components/hotspot/app-shell";
import SellShell from "@/components/hotspot/sell-shell";
import LandingPage from "@/components/landing/landing-page";

export default function Home() {
  const token = useHotspotStore((s) => s.token);
  const user = useHotspotStore((s) => s.user);

  // N°8 — token scopé « reseller » (PIN) → Mode Vente, pas la console.
  const isReseller = user?.role === "reseller";

  // Gate landing ↔ login : la landing page est la vitrine publique par défaut
  // (utilisateur non connecté). Le CTA « Se connecter » bascule vers l'écran
  // de connexion. Une fois connecté, on atterrit dans la console (ou le
  // shell de vente pour les revendeurs PIN).
  const [showLogin, setShowLogin] = useState(false);

  return (
    <QueryProvider>
      <main className="min-h-screen">
        {token ? (
          isReseller ? (
            <SellShell />
          ) : (
            <AppShell />
          )
        ) : showLogin ? (
          <LoginScreen onBack={() => setShowLogin(false)} />
        ) : (
          <LandingPage onSignIn={() => setShowLogin(true)} />
        )}
      </main>
    </QueryProvider>
  );
}
