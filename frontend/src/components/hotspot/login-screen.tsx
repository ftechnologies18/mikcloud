"use client";

import { useState } from "react";
import Image from "next/image";
import { motion } from "framer-motion";
import { KeyRound, Loader2, Store, UserPlus } from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Separator } from "@/components/ui/separator";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { api, register } from "@/lib/hotspot/api";
import { useI18n } from "@/lib/hotspot/i18n";
import { useHotspotStore } from "@/lib/hotspot/store";
import type { AuthResponse } from "@/lib/hotspot/types";

// Le bloc démo n'existe qu'en mode passerelle sandbox (pas de NEXT_PUBLIC_API_BASE).
// En production (Vercel → Render), il laisse place à la bascule inscription.
const SHOW_DEMO = !process.env.NEXT_PUBLIC_API_BASE;

export default function LoginScreen() {
  const { t, tf } = useI18n();
  const setAuth = useHotspotStore((s) => s.setAuth);

  // Connexion
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [loginLoading, setLoginLoading] = useState(false);

  // N°8 — Mode Vente (revendeur, identifiant + PIN)
  const [sellUsername, setSellUsername] = useState("");
  const [sellPin, setSellPin] = useState("");
  const [sellLoading, setSellLoading] = useState(false);

  // Inscription
  const [regName, setRegName] = useState("");
  const [regUsername, setRegUsername] = useState("");
  const [regPassword, setRegPassword] = useState("");
  const [regKey, setRegKey] = useState("");
  const [regLoading, setRegLoading] = useState(false);

  const canLogin = username.trim().length > 0 && password.trim().length > 0 && !loginLoading;
  const canRegister = regName.trim().length > 0 && regUsername.trim().length > 0 && regPassword.length > 0 && !regLoading;
  const canSell = sellUsername.trim().length > 0 && sellPin.length >= 4 && !sellLoading;

  // N°8 — connexion revendeur par PIN : token scopé role=reseller → SellShell.
  async function handleSellLogin(e: React.FormEvent) {
    e.preventDefault();
    if (!canSell) return;
    setSellLoading(true);
    try {
      const res = await api<{ token: string; reseller: { id: string; name: string; username: string } }>(
        "/api/reseller/login",
        { method: "POST", body: { username: sellUsername.trim(), pin: sellPin } },
      );
      setAuth(res.token, {
        id: res.reseller.id,
        name: res.reseller.name,
        username: res.reseller.username,
        role: "reseller",
      });
      toast.success(tf("login.welcome", { name: res.reseller.name }));
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("login.failed"));
    } finally {
      setSellLoading(false);
    }
  }

  function applyAuth(res: AuthResponse) {
    setAuth(res.token, res.user);
    toast.success(tf("login.welcome", { name: res.user.name }));
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
      toast.error(err instanceof Error ? err.message : t("login.failed"));
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
      toast.error(err instanceof Error ? err.message : t("login.registerFailed"));
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
              <Image
                src="/logo.png"
                alt={t("login.logoAlt")}
                width={112}
                height={112}
                priority
                className="rounded-2xl shadow-lg shadow-primary/20"
              />
              <h1 className="mt-4 text-2xl font-semibold tracking-tight">MikCloud</h1>
              <p className="mt-1 text-sm text-muted-foreground">{t("login.tagline")}</p>
            </div>

            <Tabs defaultValue="login" className="mt-8 gap-5">
              <TabsList className="grid w-full grid-cols-3">
                <TabsTrigger value="login">{t("login.tabLogin")}</TabsTrigger>
                <TabsTrigger value="register">
                  <UserPlus className="size-4" />
                  {t("login.tabRegister")}
                </TabsTrigger>
                <TabsTrigger value="sell">
                  <Store className="size-4" />
                  {t("login.tabSell")}
                </TabsTrigger>
              </TabsList>

              <TabsContent value="login">
                <form onSubmit={handleLogin} className="space-y-4">
                  <div className="space-y-2">
                    <Label htmlFor="login-username">{t("login.username")}</Label>
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
                    <Label htmlFor="login-password">{t("login.password")}</Label>
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
                    {t("login.tabLogin")}
                  </Button>
                </form>
              </TabsContent>

              <TabsContent value="register">
                <form onSubmit={handleRegister} className="space-y-4">
                  <div className="space-y-2">
                    <Label htmlFor="register-name">{t("login.register.name")}</Label>
                    <Input
                      id="register-name"
                      autoComplete="organization"
                      placeholder="ProMax Wifi"
                      value={regName}
                      onChange={(e) => setRegName(e.target.value)}
                      disabled={regLoading}
                    />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="register-username">{t("login.username")}</Label>
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
                    <Label htmlFor="register-password">{t("login.password")}</Label>
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
                    <Label htmlFor="register-key">{t("login.register.key")}</Label>
                    <div className="relative">
                      <KeyRound className="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
                      <Input
                        id="register-key"
                        type="password"
                        autoComplete="off"
                        className="pl-8"
                        placeholder={t("login.register.keyPlaceholder")}
                        value={regKey}
                        onChange={(e) => setRegKey(e.target.value)}
                        disabled={regLoading}
                      />
                    </div>
                  </div>
                  <Button type="submit" className="w-full" disabled={!canRegister}>
                    {regLoading && <Loader2 className="size-4 animate-spin" />}
                    {t("login.register.submit")}
                  </Button>
                </form>
              </TabsContent>
              <TabsContent value="sell">
                <form onSubmit={handleSellLogin} className="space-y-4">
                  <p className="rounded-lg bg-primary/5 px-3 py-2 text-xs text-muted-foreground">
                    {t("login.sellHint")}
                  </p>
                  <div className="space-y-2">
                    <Label htmlFor="sell-username">{t("login.sellUsername")}</Label>
                    <Input
                      id="sell-username"
                      autoComplete="username"
                      placeholder="ange.kessie"
                      value={sellUsername}
                      onChange={(e) => setSellUsername(e.target.value)}
                      disabled={sellLoading}
                    />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="sell-pin">{t("login.sellPin")}</Label>
                    <Input
                      id="sell-pin"
                      type="password"
                      inputMode="numeric"
                      pattern="[0-9]*"
                      maxLength={6}
                      autoComplete="one-time-code"
                      placeholder="••••"
                      value={sellPin}
                      onChange={(e) => setSellPin(e.target.value.replace(/\D/g, ""))}
                      disabled={sellLoading}
                    />
                  </div>
                  <Button type="submit" className="w-full" disabled={!canSell}>
                    {sellLoading && <Loader2 className="size-4 animate-spin" />}
                    <Store className="size-4" />
                    {t("login.sellSubmit")}
                  </Button>
                </form>
              </TabsContent>
            </Tabs>

            {SHOW_DEMO && (
              <>
                <Separator className="my-6" />
                <div className="flex items-center justify-between gap-3 rounded-lg bg-muted/50 px-3 py-2.5">
                  <p className="text-xs text-muted-foreground">
                    {t("login.demoPrefix")}
                    <span className="font-medium text-foreground">admin / admin123</span>
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
                    {t("login.useDemo")}
                  </Button>
                </div>
              </>
            )}
          </CardContent>
        </Card>

        <p className="mt-6 text-center text-xs text-muted-foreground">{t("login.footer")}</p>
      </motion.div>
    </div>
  );
}
