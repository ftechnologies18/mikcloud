"use client";

// Route /login — écran de connexion (vitrine → console, PWA standalone).
// Garde inverse : une session active est redirigée vers son espace
// (revendeur → /sell, sinon /app). Le bouton « Retour » ramène à la
// vitrine (/) via le routeur — une vraie entrée d'historique, le bouton
// Retour du navigateur fonctionne aussi.

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import LoginScreen from "@/components/hotspot/login-screen";
import SignupModal from "@/components/landing/signup-modal";
import { useMounted } from "@/hooks/use-mounted";
import { useHotspotStore } from "@/lib/hotspot/store";
import { ShellFallback } from "./shell-fallback";

export default function LoginRoute() {
  const router = useRouter();
  const mounted = useMounted();

  const token = useHotspotStore((s) => s.token);
  const userRole = useHotspotStore((s) => s.user?.role);
  const [signupOpen, setSignupOpen] = useState(false);

  useEffect(() => {
    if (!mounted || !token) return;
    router.replace(userRole === "reseller" ? "/sell" : "/app");
  }, [mounted, token, userRole, router]);

  if (!mounted || token) return <ShellFallback />;
  return (
    <main className="min-h-screen">
      <LoginScreen onBack={() => router.push("/")} onSignUp={() => setSignupOpen(true)} />
      {/* SignupModal global — ouvrable depuis l'écran de connexion. */}
      <SignupModal open={signupOpen} onOpenChange={setSignupOpen} />
    </main>
  );
}
