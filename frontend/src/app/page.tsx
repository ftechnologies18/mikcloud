"use client";

import { useState } from "react";
import { QueryProvider } from "@/lib/hotspot/query";
import { useHotspotStore } from "@/lib/hotspot/store";
import LoginScreen from "@/components/hotspot/login-screen";
import AppShell from "@/components/hotspot/app-shell";
import SellShell from "@/components/hotspot/sell-shell";
import LandingPage from "@/components/landing/landing-page";
import SignupModal from "@/components/landing/signup-modal";

export default function Home() {
  const token = useHotspotStore((s) => s.token);
  const user = useHotspotStore((s) => s.user);

  // N°8 — token scopé « reseller » (PIN) → Mode Vente, pas la console.
  const isReseller = user?.role === "reseller";

  // Gate landing ↔ login : la landing page est la vitrine publique par défaut
  // (utilisateur non connecté). Le CTA « Se connecter » bascule vers l'écran
  // de connexion. Une fois connecté, on atterrit dans la console (ou le
  // shell de vente pour les revendeurs PIN).
  const [view, setView] = useState<"landing" | "login">("landing");
  const [signupOpen, setSignupOpen] = useState(false);

  const openSignup = () => setSignupOpen(true);

  return (
    <QueryProvider>
      <main className="min-h-screen">
        {token ? (
          isReseller ? (
            <SellShell />
          ) : (
            <AppShell />
          )
        ) : view === "login" ? (
          <LoginScreen onBack={() => setView("landing")} onSignUp={openSignup} />
        ) : (
          <LandingPage onSignIn={() => setView("login")} onSignUp={openSignup} />
        )}
      </main>
      {/* SignupModal global — ouvrable depuis la landing ou le login. */}
      <SignupModal open={signupOpen} onOpenChange={setSignupOpen} />
    </QueryProvider>
  );
}
