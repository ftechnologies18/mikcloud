"use client";

import { QueryProvider } from "@/lib/hotspot/query";
import { useHotspotStore } from "@/lib/hotspot/store";
import LoginScreen from "@/components/hotspot/login-screen";
import AppShell from "@/components/hotspot/app-shell";

export default function Home() {
  const token = useHotspotStore((s) => s.token);

  return (
    <QueryProvider>
      <main className="min-h-screen">{token ? <AppShell /> : <LoginScreen />}</main>
    </QueryProvider>
  );
}
