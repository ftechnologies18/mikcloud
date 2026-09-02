"use client";

import { useState } from "react";
import Image from "next/image";
import { motion, useAnimate, useReducedMotion, type Variants } from "framer-motion";
import { Eye, EyeOff, Loader2, ShieldCheck, Store, Ticket, Wifi } from "lucide-react";
import { toast } from "sonner";

import { FtciCredit } from "@/components/ftci-credit";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { api } from "@/lib/hotspot/api";
import { useI18n } from "@/lib/hotspot/i18n";
import { useHotspotStore } from "@/lib/hotspot/store";
import type { AuthResponse } from "@/lib/hotspot/types";

// Le bloc démo n'existe qu'en mode passerelle sandbox (pas de NEXT_PUBLIC_API_BASE).
// En production (Vercel → Render), il laisse place à la bascule inscription.
const SHOW_DEMO = !process.env.NEXT_PUBLIC_API_BASE;

/* Micro-animations : champs en cascade à chaque changement d'onglet. */
const stagger: Variants = {
  hidden: {},
  show: { transition: { staggerChildren: 0.07, delayChildren: 0.04 } },
};
const rise: Variants = {
  hidden: { opacity: 0, y: 14 },
  show: { opacity: 1, y: 0, transition: { duration: 0.38, ease: "easeOut" } },
};

/* Anneaux de pulsation autour du logo — signal « en ligne ». */
function PulseRings() {
  const reduce = useReducedMotion();
  if (reduce) return null;
  return (
    <>
      {[0, 1.3].map((delay) => (
        <motion.span
          key={delay}
          aria-hidden
          className="pointer-events-none absolute inset-0 rounded-3xl border border-primary/40"
          initial={{ scale: 1, opacity: 0.55 }}
          animate={{ scale: 1.85, opacity: 0 }}
          transition={{ duration: 2.8, repeat: Infinity, delay, ease: "easeOut" }}
        />
      ))}
    </>
  );
}

/* Panneau branding animé — colonne gauche (desktop uniquement). */
function BrandPanel() {
  const { t } = useI18n();
  const reduce = useReducedMotion();

  const features = [
    { icon: Wifi, title: "login.hero.f1.title", desc: "login.hero.f1.desc" },
    { icon: Ticket, title: "login.hero.f2.title", desc: "login.hero.f2.desc" },
    { icon: ShieldCheck, title: "login.hero.f3.title", desc: "login.hero.f3.desc" },
    { icon: Store, title: "login.hero.f4.title", desc: "login.hero.f4.desc" },
  ];

  return (
    <aside className="login-brand relative hidden flex-col justify-between overflow-hidden p-10 lg:flex xl:p-14">
      {/* Décor : grille technique + orbes dérivants */}
      <div aria-hidden className="login-grid absolute inset-0" />
      <motion.div
        aria-hidden
        className="absolute -left-28 top-[10%] size-[26rem] rounded-full bg-primary/20 blur-3xl"
        animate={reduce ? undefined : { x: [0, 36, -12, 0], y: [0, 28, -8, 0] }}
        transition={{ duration: 22, repeat: Infinity, ease: "easeInOut" }}
      />
      <motion.div
        aria-hidden
        className="absolute -right-24 bottom-[6%] size-[22rem] rounded-full bg-emerald-500/10 blur-3xl"
        animate={reduce ? undefined : { x: [0, -30, 10, 0], y: [0, -24, 6, 0] }}
        transition={{ duration: 18, repeat: Infinity, ease: "easeInOut", delay: 1.5 }}
      />

      <motion.div
        variants={stagger}
        initial="hidden"
        animate="show"
        className="relative z-10 flex h-full flex-col justify-between gap-10"
      >
        {/* Badge plateforme */}
        <motion.div variants={rise}>
          <span className="glass-chip inline-flex items-center gap-2 rounded-full px-3.5 py-1.5 text-xs font-medium tracking-wide text-primary">
            <span className="live-dot size-1.5 rounded-full bg-primary" aria-hidden />
            {t("login.hero.badge")}
          </span>
        </motion.div>

        {/* Identité produit */}
        <motion.div variants={stagger} className="max-w-lg">
          <motion.div variants={rise} className="relative w-fit">
            <PulseRings />
            <Image
              src="/logo.png"
              alt={t("login.logoAlt")}
              width={104}
              height={104}
              priority
              className="relative z-10 rounded-2xl shadow-2xl shadow-primary/25"
            />
          </motion.div>
          <motion.h1
            variants={rise}
            className="mt-7 bg-gradient-to-br from-primary via-emerald-300 to-teal-200 bg-clip-text text-4xl font-semibold tracking-tight text-transparent xl:text-5xl"
          >
            MikCloud
          </motion.h1>
          <motion.p variants={rise} className="mt-3 text-lg font-medium text-foreground/90">
            {t("login.hero.title")}
          </motion.p>
          <motion.p variants={rise} className="mt-2 text-sm leading-relaxed text-muted-foreground">
            {t("login.hero.subtitle")}
          </motion.p>
        </motion.div>

        {/* Atouts + copyright */}
        <motion.div variants={stagger}>
          <motion.ul variants={stagger} className="grid gap-3 xl:grid-cols-2">
            {features.map((f) => (
              <motion.li
                key={f.title}
                variants={rise}
                whileHover={{ y: -3 }}
                transition={{ type: "spring", stiffness: 320, damping: 22 }}
                className="glass-chip flex items-start gap-3 rounded-xl p-3.5"
              >
                <span className="mt-0.5 flex size-9 shrink-0 items-center justify-center rounded-lg bg-primary/15 text-primary">
                  <f.icon className="size-4.5" aria-hidden />
                </span>
                <span>
                  <span className="block text-sm font-medium">{t(f.title)}</span>
                  <span className="mt-0.5 block text-xs leading-relaxed text-muted-foreground">{t(f.desc)}</span>
                </span>
              </motion.li>
            ))}
          </motion.ul>
          <motion.p variants={rise} className="mt-8 text-xs text-muted-foreground/70">
            {t("login.footer")}
          </motion.p>
          <motion.p variants={rise} className="mt-2">
            <FtciCredit className="text-xs text-muted-foreground/80" />
          </motion.p>
        </motion.div>
      </motion.div>
    </aside>
  );
}

export default function LoginScreen({ onBack, onSignUp }: { onBack?: () => void; onSignUp?: () => void }) {
  const { t, tf } = useI18n();
  const setAuth = useHotspotStore((s) => s.setAuth);

  // Connexion
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [loginLoading, setLoginLoading] = useState(false);
  const [showPassword, setShowPassword] = useState(false);

  // N°8 — Mode Vente (revendeur, identifiant + PIN)
  const [sellUsername, setSellUsername] = useState("");
  const [sellPin, setSellPin] = useState("");
  const [sellLoading, setSellLoading] = useState(false);


  // Micro-feedback d'erreur : la carte de verre tremble (sans remonter les onglets).
  const [scope, animate] = useAnimate();
  function shakeCard() {
    if (scope.current) {
      void animate(scope.current, { x: [0, -10, 10, -6, 6, 0] }, { duration: 0.45, ease: "easeInOut" });
    }
  }

  const canLogin = username.trim().length > 0 && password.trim().length > 0 && !loginLoading;
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
      shakeCard();
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
      shakeCard();
      toast.error(err instanceof Error ? err.message : t("login.failed"));
    } finally {
      setLoginLoading(false);
    }
  }

  return (
    <div className="grid min-h-screen lg:grid-cols-[1.08fr_1fr]">
      <BrandPanel />

      {/* Colonne formulaire */}
      <section className="bg-glow relative flex min-h-screen flex-col items-center justify-center overflow-hidden px-4 py-10 sm:px-8">
        {/* Lien retour vers la landing page (si appelé depuis la landing) */}
        {onBack && (
          <button
            onClick={onBack}
            className="absolute left-4 top-4 z-20 inline-flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm text-muted-foreground transition-colors hover:bg-accent hover:text-foreground sm:left-8 sm:top-8"
          >
            <span aria-hidden>←</span>
            {t("common.back", "Retour")}
          </button>
        )}
        {/* Orbe discret côté formulaire */}
        <motion.div
          aria-hidden
          className="absolute -bottom-24 -right-32 size-[26rem] rounded-full bg-primary/10 blur-3xl"
          animate={{ scale: [1, 1.12, 1], opacity: [0.7, 1, 0.7] }}
          transition={{ duration: 14, repeat: Infinity, ease: "easeInOut" }}
        />

        <div className="relative z-10 flex w-full max-w-md flex-1 flex-col items-center justify-center">
          {/* En-tête branding compact (mobile / tablette) */}
          <motion.div
            initial={{ opacity: 0, y: 18 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ duration: 0.45, ease: "easeOut" }}
            className="mb-8 flex flex-col items-center text-center lg:hidden"
          >
            <div className="relative w-fit">
              <PulseRings />
              <Image
                src="/logo.png"
                alt={t("login.logoAlt")}
                width={88}
                height={88}
                priority
                className="relative z-10 rounded-2xl shadow-xl shadow-primary/20"
              />
            </div>
            <h1 className="mt-4 text-2xl font-semibold tracking-tight">MikCloud</h1>
            <p className="mt-1 text-sm text-muted-foreground">{t("login.tagline")}</p>
          </motion.div>

          {/* Carte de verre */}
          <motion.div
            ref={scope}
            initial={{ opacity: 0, y: 26, scale: 0.985 }}
            animate={{ opacity: 1, y: 0, scale: 1 }}
            transition={{ duration: 0.5, ease: "easeOut", delay: 0.08 }}
            className="glass-card w-full rounded-2xl p-6 sm:p-8"
          >
            <div className="mb-6 hidden lg:block">
              <h2 className="text-xl font-semibold tracking-tight">{t("login.form.title")}</h2>
              <p className="mt-1 text-sm text-muted-foreground">{t("login.form.subtitle")}</p>
            </div>

            <Tabs defaultValue="login" className="gap-5">
              <TabsList className="glass-chip grid w-full grid-cols-2">
                <TabsTrigger value="login" className="px-1.5 text-xs sm:px-2.5 sm:text-sm">
                  {t("login.tabLogin")}
                </TabsTrigger>
                <TabsTrigger value="sell" className="px-1.5 text-xs sm:px-2.5 sm:text-sm">
                  <span className="hidden sm:inline">{t("login.tabSell")}</span>
                  <span className="sm:hidden">{t("login.tabSellShort")}</span>
                </TabsTrigger>
              </TabsList>

              <TabsContent value="login">
                <motion.form variants={stagger} initial="hidden" animate="show" onSubmit={handleLogin} className="space-y-4">
                  <motion.div variants={rise} className="space-y-2">
                    <Label htmlFor="login-username">{t("login.username")}</Label>
                    <Input
                      id="login-username"
                      autoComplete="username"
                      placeholder="admin"
                      value={username}
                      onChange={(e) => setUsername(e.target.value)}
                      disabled={loginLoading}
                    />
                  </motion.div>
                  <motion.div variants={rise} className="space-y-2">
                    <Label htmlFor="login-password">{t("login.password")}</Label>
                    <div className="relative">
                      <Input
                        id="login-password"
                        type={showPassword ? "text" : "password"}
                        autoComplete="current-password"
                        placeholder="••••••••"
                        className="pr-10"
                        value={password}
                        onChange={(e) => setPassword(e.target.value)}
                        disabled={loginLoading}
                      />
                      <button
                        type="button"
                        onClick={() => setShowPassword((v) => !v)}
                        aria-label={t(showPassword ? "login.hidePassword" : "login.showPassword")}
                        className="absolute right-2.5 top-1/2 -translate-y-1/2 text-muted-foreground transition-colors hover:text-foreground"
                      >
                        {showPassword ? <EyeOff className="size-4" aria-hidden /> : <Eye className="size-4" aria-hidden />}
                      </button>
                    </div>
                  </motion.div>
                  <motion.div variants={rise} whileHover={{ scale: 1.015 }} whileTap={{ scale: 0.97 }}>
                    <Button type="submit" className="w-full shadow-lg shadow-primary/25" disabled={!canLogin}>
                      {loginLoading && <Loader2 className="size-4 animate-spin" />}
                      {t("login.tabLogin")}
                    </Button>
                  </motion.div>
                </motion.form>
              </TabsContent>

              {onSignUp && (
                <motion.p variants={rise} className="text-center text-sm text-muted-foreground">
                  {t("login.noAccount", "Pas encore de compte ?")}{" "}
                  <button onClick={onSignUp} className="font-medium text-primary hover:underline">
                    {t("login.createAccount", "Créer mon compte")}
                  </button>
                </motion.p>
              )}

              <TabsContent value="sell">
                <motion.form variants={stagger} initial="hidden" animate="show" onSubmit={handleSellLogin} className="space-y-4">
                  <motion.p variants={rise} className="glass-chip rounded-lg px-3 py-2 text-xs text-muted-foreground">
                    {t("login.sellHint")}
                  </motion.p>
                  <motion.div variants={rise} className="space-y-2">
                    <Label htmlFor="sell-username">{t("login.sellUsername")}</Label>
                    <Input
                      id="sell-username"
                      autoComplete="username"
                      placeholder="ange.kessie"
                      value={sellUsername}
                      onChange={(e) => setSellUsername(e.target.value)}
                      disabled={sellLoading}
                    />
                  </motion.div>
                  <motion.div variants={rise} className="space-y-2">
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
                  </motion.div>
                  <motion.div variants={rise} whileHover={{ scale: 1.015 }} whileTap={{ scale: 0.97 }}>
                    <Button type="submit" className="w-full shadow-lg shadow-primary/25" disabled={!canSell}>
                      {sellLoading && <Loader2 className="size-4 animate-spin" />}
                      <Store className="size-4" />
                      {t("login.sellSubmit")}
                    </Button>
                  </motion.div>
                </motion.form>
              </TabsContent>
            </Tabs>

            {SHOW_DEMO && (
              <motion.div
                initial={{ opacity: 0 }}
                animate={{ opacity: 1 }}
                transition={{ delay: 0.4, duration: 0.4 }}
                className="glass-chip mt-6 flex items-center justify-between gap-3 rounded-xl px-3 py-2.5"
              >
                <p className="text-xs text-muted-foreground">
                  {t("login.demoPrefix")}
                  <span className="font-medium text-foreground">admin / {t("login.demoPasswordHint", "mot de passe défini par la plateforme")}</span>
                </p>
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  className="h-8 text-xs"
                  onClick={() => {
                    setUsername("admin");
                  }}
                  disabled={loginLoading}
                >
                  {t("login.useDemo")}
                </Button>
              </motion.div>
            )}
          </motion.div>

          <motion.p
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            transition={{ delay: 0.55, duration: 0.4 }}
            className="mt-6 text-center text-xs text-muted-foreground lg:hidden"
          >
            {t("login.footer")}
          </motion.p>
          <motion.p
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            transition={{ delay: 0.6, duration: 0.4 }}
            className="mt-2 text-center lg:hidden"
          >
            <FtciCredit className="text-xs text-muted-foreground/80" />
          </motion.p>
        </div>
      </section>
    </div>
  );
}
