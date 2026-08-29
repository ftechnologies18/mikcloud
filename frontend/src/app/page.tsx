"use client";

import { QueryProvider } from "@/lib/hotspot/query";
import { useHotspotStore } from "@/lib/hotspot/store";
import LoginScreen from "@/components/hotspot/login-screen";
import AppShell from "@/components/hotspot/app-shell";
import SellShell from "@/components/hotspot/sell-shell";

export default function Home() {
  const token = useHotspotStore((s) => s.token);
  const user = useHotspotStore((s) => s.user);

  // N°8 — token scopé « reseller » (PIN) → Mode Vente, pas la console.
  const isReseller = user?.role === "reseller";

  return (
    <QueryProvider>
      <main className="min-h-screen">
        {token ? (isReseller ? <SellShell /> : <AppShell />) : <LoginScreen />}
      </main>
    </QueryProvider>
  );
}
