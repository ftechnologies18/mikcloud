"use client";

import { useState } from "react";
import { motion } from "framer-motion";
import { KeyRound, Loader2, UserPlus, Wifi } from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Separator } from "@/components/ui/separator";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { api, register } from "@/lib/hotspot/api";
import { useHotspotStore } from "@/lib/hotspot/store";
import type { AuthResponse } from "@/lib/hotspot/types";

// Le bloc démo n'existe qu'en mode passerelle sandbox (pas de NEXT_PUBLIC_API_BASE).
// En production (Vercel → Render), il laisse place à la bascule inscription.
const SHOW_DEMO = !process.env.NEXT_PUBLIC_API_BASE;

export default function LoginScreen() {
  const setAuth = useHotspotStore((s) => s.setAuth);

  // Connexion
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [loginLoading, setLoginLoading] = useState(false);

  // Inscription
  const [regName, setRegName] = useState("");
  const [regUsername, setRegUsername] = useState("");
  const [regPassword, setRegPassword] = useState("");
  const [regKey, setRegKey] = useState("");
  const [regLoading, setRegLoading] = useState(false);

  const canLogin = username.trim().length > 0 && password.trim().length > 0 && !loginLoading;
  const canRegister = regName.trim().length > 0 && regUsername.trim().length > 0 && regPassword.length > 0 && !regLoading;

  function applyAuth(res: AuthResponse) {
    setAuth(res.token, res.user);
    toast.success(`Bienvenue, ${res.user.name}`);
  }

  async function handleLogin(e: React.FormEvent) {
    e.preventDefault();
    if (!canLogin) return;
    setLoginLoading(true);
    try {
      const res = await api<AuthResponse>("/api/auth/login", {
        method: "POST",
        body: { username: username.trim(), password },
      });
      applyAuth(res);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Connexion impossible");
    } finally {
      setLoginLoading(false);
    }
  }

  async function handleRegister(e: React.FormEvent) {
    e.preventDefault();
    if (!canRegister) return;
    setRegLoading(true);
    try {
      const res = await register({
        name: regName.trim(),
        username: regUsername.trim(),
        password: regPassword,
        key: regKey.trim() || undefined,
      });
      applyAuth(res);
    } catch (err) {
      // 403 inscription fermée / clé requise, 409 identifiant pris, 400 validations.
      toast.error(err instanceof Error ? err.message : "Inscription impossible");
    } finally {
      setRegLoading(false);
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

            <Tabs defaultValue="login" className="mt-8 gap-5">
              <TabsList className="grid w-full grid-cols-2">
                <TabsTrigger value="login">Se connecter</TabsTrigger>
                <TabsTrigger value="register">
                  <UserPlus className="size-4" />
                  Créer un compte
                </TabsTrigger>
              </TabsList>

              <TabsContent value="login">
                <form onSubmit={handleLogin} className="space-y-4">
                  <div className="space-y-2">
                    <Label htmlFor="login-username">Identifiant</Label>
                    <Input
                      id="login-username"
                      autoComplete="username"
                      placeholder="admin"
                      value={username}
                      onChange={(e) => setUsername(e.target.value)}
                      disabled={loginLoading}
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
                      disabled={loginLoading}
                    />
                  </div>
                  <Button type="submit" className="w-full" disabled={!canLogin}>
                    {loginLoading && <Loader2 className="size-4 animate-spin" />}
                    Se connecter
                  </Button>
                </form>
              </TabsContent>

              <TabsContent value="register">
                <form onSubmit={handleRegister} className="space-y-4">
                  <div className="space-y-2">
                    <Label htmlFor="register-name">Nom complet / entreprise</Label>
                    <Input
                      id="register-name"
                      autoComplete="organization"
                      placeholder="Ex. ProMax Wifi"
                      value={regName}
                      onChange={(e) => setRegName(e.target.value)}
                      disabled={regLoading}
                    />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="register-username">Identifiant</Label>
                    <Input
                      id="register-username"
                      autoComplete="username"
                      placeholder="gerant1"
                      value={regUsername}
                      onChange={(e) => setRegUsername(e.target.value)}
                      disabled={regLoading}
                    />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="register-password">Mot de passe</Label>
                    <Input
                      id="register-password"
                      type="password"
                      autoComplete="new-password"
                      placeholder="••••••••"
                      value={regPassword}
                      onChange={(e) => setRegPassword(e.target.value)}
                      disabled={regLoading}
                    />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="register-key">Clé d&apos;invitation (si requise)</Label>
                    <div className="relative">
                      <KeyRound className="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
                      <Input
                        id="register-key"
                        type="password"
                        autoComplete="off"
                        className="pl-8"
                        placeholder="Optionnelle"
                        value={regKey}
                        onChange={(e) => setRegKey(e.target.value)}
                        disabled={regLoading}
                      />
                    </div>
                  </div>
                  <Button type="submit" className="w-full" disabled={!canRegister}>
                    {regLoading && <Loader2 className="size-4 animate-spin" />}
                    Créer mon compte
                  </Button>
                </form>
              </TabsContent>
            </Tabs>

            {SHOW_DEMO && (
              <>
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
                    disabled={loginLoading}
                  >
                    Utiliser le compte démo
                  </Button>
                </div>
              </>
            )}
          </CardContent>
        </Card>

        <p className="mt-6 text-center text-xs text-muted-foreground">
          © 2025 MikCloud — Connectez vos routeurs MikroTik en toute simplicité
        </p>
      </motion.div>
    </div>
  );
}
