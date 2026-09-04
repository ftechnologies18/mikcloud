"use client";

// Route /sell — Mode Vente (portail revendeur, session scoping « reseller »).
// Garde : session requise ; seuls les revendeurs y accèdent — un gestionnaire
// est renvoyé vers la console (/app). Mêmes règles que l'ancien rendu
// conditionnel de page.tsx.

import { useEffect } from "react";
import { useRouter } from "next/navigation";
import SellShell from "@/components/hotspot/sell-shell";
import { useMounted } from "@/hooks/use-mounted";
import { useHotspotStore } from "@/lib/hotspot/store";
import { ShellFallback } from "./shell-fallback";

export default function SellRoute() {
  const router = useRouter();
  const mounted = useMounted();

  const token = useHotspotStore((s) => s.token);
  const userRole = useHotspotStore((s) => s.user?.role);

  useEffect(() => {
    if (!mounted) return;
    if (!token) router.replace("/login");
    else if (userRole !== "reseller") router.replace("/app");
  }, [mounted, token, userRole, router]);

  if (!mounted || !token || userRole !== "reseller") return <ShellFallback />;
  return <SellShell />;
}
