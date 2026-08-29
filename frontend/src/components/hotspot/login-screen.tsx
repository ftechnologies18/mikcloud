"use client";

import { useState } from "react";
import { motion } from "framer-motion";
import { Loader2, Wifi } from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Separator } from "@/components/ui/separator";
import { api } from "@/lib/hotspot/api";
import { useHotspotStore } from "@/lib/hotspot/store";
import type { LoginResponse } from "@/lib/hotspot/types";

export default function LoginScreen() {
  const setAuth = useHotspotStore((s) => s.setAuth);
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [loading, setLoading] = useState(false);

  const canSubmit = username.trim().length > 0 && password.trim().length > 0 && !loading;

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!canSubmit) return;
    setLoading(true);
    try {
      const res = await api<LoginResponse>("/api/auth/login", {
        method: "POST",
        body: { username: username.trim(), password },
      });
      setAuth(res.token, res.user);
      toast.success(`Bienvenue, ${res.user.name}`);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Connexion impossible");
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="bg-glow relative flex min-h-screen items-center justify-center px-4 py-10">
      <motion.div
        initial={{ opacity: 0, y: 24 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.45, ease: "easeOut" }}
        className="w-full max-w-sm"
      >
        <Card className="border-border/70 shadow-xl shadow-black/20">
          <CardContent className="p-6 sm:p-8">
            <div className="flex flex-col items-center text-center">
              <div className="flex size-14 items-center justify-center rounded-2xl bg-gradient-to-br from-primary to-chart-2 shadow-lg shadow-primary/20">
                <Wifi className="size-7 text-white" />
              </div>
              <h1 className="mt-4 text-2xl font-semibold tracking-tight">MikCloud</h1>
              <p className="mt-1 text-sm text-muted-foreground">Gestion Hotspot Cloud MikroTik</p>
            </div>

            <form onSubmit={handleSubmit} className="mt-8 space-y-4">
              <div className="space-y-2">
                <Label htmlFor="login-username">Identifiant</Label>
                <Input
                  id="login-username"
                  autoComplete="username"
                  placeholder="admin"
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                  disabled={loading}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="login-password">Mot de passe</Label>
                <Input
                  id="login-password"
                  type="password"
                  autoComplete="current-password"
                  placeholder="••••••••"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  disabled={loading}
                />
              </div>
              <Button type="submit" className="w-full" disabled={!canSubmit}>
                {loading && <Loader2 className="size-4 animate-spin" />}
                Se connecter
              </Button>
            </form>

            <Separator className="my-6" />

            <div className="flex items-center justify-between gap-3 rounded-lg bg-muted/50 px-3 py-2.5">
              <p className="text-xs text-muted-foreground">
                Démo : <span className="font-medium text-foreground">admin / admin123</span>
              </p>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                className="h-8 text-xs"
                onClick={() => {
                  setUsername("admin");
                  setPassword("admin123");
                }}
                disabled={loading}
              >
                Utiliser le compte démo
              </Button>
            </div>
          </CardContent>
        </Card>

        <p className="mt-6 text-center text-xs text-muted-foreground">
          © 2025 MikCloud — Connectez vos routeurs MikroTik en toute simplicité
        </p>
      </motion.div>
    </div>
  );
}
