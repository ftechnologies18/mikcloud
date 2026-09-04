"use client";

// / — vitrine publique (landing). La navigation est désormais portée par de
// vraies routes : /login (connexion), /app/<vue> (console), /sell (Mode
// Vente) — le bouton Retour du navigateur et les liens directs fonctionnent
// partout (Phase A « Speed App UX »).
//
// Redirections de garde au montage (après rehydratation du store) :
//   session active → /app ou /sell ; PWA standalone sans session → /login.
// Pendant la fenêtre d'évaluation, la vitrine prérendue reste affichée
// (aucun flash) ; en PWA standalone, le masquage CSS pwa-standalone s'applique
// jusqu'au premier rendu React (classe pwa-ready posée ci-dessous).

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { useMounted } from "@/hooks/use-mounted";
import { useHotspotStore } from "@/lib/hotspot/store";
import { useIsStandalone } from "@/hooks/use-standalone";
import LandingPage from "@/components/landing/landing-page";
import SignupModal from "@/components/landing/signup-modal";
import { ShellFallback } from "@/components/hotspot/routes/shell-fallback";

export default function Home() {
  const router = useRouter();
  const token = useHotspotStore((s) => s.token);
  const userRole = useHotspotStore((s) => s.user?.role);
  const isStandalone = useIsStandalone();
  const mounted = useMounted();
  const [signupOpen, setSignupOpen] = useState(false);

  // Fenêtre pré-hydratation PWA : le script inline du layout a posé
  // .pwa-standalone AVANT le premier paint pour masquer le HTML prérendu de
  // la vitrine (globals.css). Dès que React affiche SA première interface,
  // on rend la landing de nouveau visible.
  useEffect(() => {
    document.documentElement.classList.add("pwa-ready");
  }, []);

  useEffect(() => {
    if (!mounted) return;
    if (token) router.replace(userRole === "reseller" ? "/sell" : "/app");
    else if (isStandalone) router.replace("/login");
  }, [mounted, token, userRole, isStandalone, router]);

  // Session active ou PWA standalone : on quitte la vitrine — fallback plein
  // écran pendant la redirection (jamais la vitrine derrière une console).
  if (mounted && (token || isStandalone)) return <ShellFallback />;

  return (
    <main className="min-h-screen">
      <LandingPage
        onSignIn={() => router.push("/login")}
        onSignUp={() => setSignupOpen(true)}
      />
      {/* SignupModal global — ouvrable depuis la vitrine. */}
      <SignupModal open={signupOpen} onOpenChange={setSignupOpen} />
    </main>
  );
}
