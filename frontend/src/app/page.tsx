"use client";

import { useEffect, useState } from "react";
import { QueryProvider } from "@/lib/hotspot/query";
import { useHotspotStore } from "@/lib/hotspot/store";
import { useIsStandalone } from "@/hooks/use-standalone";
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

  // N°8 — PWA installée (mode standalone) : porte d'entrée = écran de
  // connexion, pas la vitrine publique. Le hook est SSR-sûr (snapshot serveur
  // false) : le HTML prérendu reste la landing, puis React bascule vers le
  // login dès le montage — sans erreur d'hydratation ni flash perceptible.
  const isStandalone = useIsStandalone();

  // Vue explicitement choisie par l'utilisateur (CTA « Se connecter » de la
  // vitrine ↔ bouton « Retour » du login). null = aucun choix fait → porte
  // d'entrée par défaut : PWA → login, navigateur → vitrine. La dérivation
  // (plutôt qu'un état copié) garde le comportement correct APRÈS logout :
  // une session expirée en PWA retombe sur le login, jamais sur la vitrine.
  const [explicitView, setExplicitView] = useState<"landing" | "login" | null>(null);
  const view = explicitView ?? (isStandalone ? "login" : "landing");
  const [signupOpen, setSignupOpen] = useState(false);

  // Fenêtre pré-hydratation : le script inline du layout a posé
  // .pwa-standalone AVANT le premier paint pour masquer le HTML prérendu de
  // la vitrine (globals.css). Dès que React affiche SA première interface
  // (login ou console selon la session), on rend la landing de nouveau
  // visible — elle reste accessible via « Retour » depuis le login.
  useEffect(() => {
    document.documentElement.classList.add("pwa-ready");
  }, []);

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
          <LoginScreen onBack={() => setExplicitView("landing")} onSignUp={openSignup} />
        ) : (
          <LandingPage onSignIn={() => setExplicitView("login")} onSignUp={openSignup} />
        )}
      </main>
      {/* SignupModal global — ouvrable depuis la landing ou le login. */}
      <SignupModal open={signupOpen} onOpenChange={setSignupOpen} />
    </QueryProvider>
  );
}
