"use client";

import { useRef, useState } from "react";
import Image from "next/image";
import { Eye, EyeOff, Loader2, ShieldCheck, Store, Ticket, Wifi } from "lucide-react";
import { toast } from "sonner";

import { FtciCredit } from "@/components/ftci-credit";
import { PwaInstallCta } from "@/components/pwa-install-cta";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import ForgotPasswordModal from "@/components/hotspot/parts/forgot-password-modal";
import { ApiError, api } from "@/lib/hotspot/api";
import { useI18n } from "@/lib/hotspot/i18n";
import { useHotspotStore } from "@/lib/hotspot/store";
import type { AuthResponse } from "@/lib/hotspot/types";

// N°78 — framer-motion retiré du chemin critique (écran de connexion =
// bundle initial) : les micro-animations vivent en @keyframes CSS
// (globals.css, classes mik-*) avec animation-delay en cascade pour
// reproduire l'ancien stagger (0,04 s + 0,07 s par enfant).

// Le bloc démo n'existe qu'en mode passerelle sandbox (pas de NEXT_PUBLIC_API_BASE).
// En production (Vercel → Render), il laisse place à la bascule inscription.
const SHOW_DEMO = !process.env.NEXT_PUBLIC_API_BASE;

/* Anneaux de pulsation autour du logo — signal « en ligne ». */
function PulseRings() {
  return (
    <>
      {[0, 1.3].map((delay) => (
        <span
          key={delay}
          aria-hidden
          className="mik-pulse-ring pointer-events-none absolute inset-0 rounded-3xl border border-primary/40"
          style={{ animationDelay: `${delay}s` }}
        />
      ))}
    </>
  );
}

/* Panneau branding animé — colonne gauche (desktop uniquement). */
function BrandPanel() {
  const { t } = useI18n();

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
      <div
        aria-hidden
        className="mik-drift-a absolute -left-28 top-[10%] size-[26rem] rounded-full bg-primary/20 blur-3xl"
      />
      <div
        aria-hidden
        className="mik-drift-b absolute -right-24 bottom-[6%] size-[22rem] rounded-full bg-emerald-500/10 blur-3xl"
      />

      <div className="relative z-10 flex h-full flex-col justify-between gap-10">
        {/* Badge plateforme */}
        <div className="mik-rise" style={{ animationDelay: "0.04s" }}>
          <span className="glass-chip inline-flex items-center gap-2 rounded-full px-3.5 py-1.5 text-xs font-medium tracking-wide text-primary">
            <span className="live-dot size-1.5 rounded-full bg-primary" aria-hidden />
            {t("login.hero.badge")}
          </span>
        </div>

        {/* Identité produit */}
        <div className="max-w-lg">
          <div className="mik-rise relative w-fit" style={{ animationDelay: "0.11s" }}>
            <PulseRings />
            <Image
              src="/logo.png"
              alt={t("login.logoAlt")}
              width={104}
              height={104}
              priority
              className="relative z-10 rounded-2xl shadow-2xl shadow-primary/25"
            />
          </div>
          <h1
            className="mik-rise mt-7 bg-gradient-to-br from-primary via-emerald-300 to-teal-200 bg-clip-text text-4xl font-semibold tracking-tight text-transparent xl:text-5xl"
            style={{ animationDelay: "0.18s" }}
          >
            MikCloud
          </h1>
          <p
            className="mik-rise mt-3 text-lg font-medium text-foreground/90"
            style={{ animationDelay: "0.25s" }}
          >
            {t("login.hero.title")}
          </p>
          <p
            className="mik-rise mt-2 text-sm leading-relaxed text-muted-foreground"
            style={{ animationDelay: "0.32s" }}
          >
            {t("login.hero.subtitle")}
          </p>
        </div>

        {/* Atouts + copyright */}
        <div>
          <ul className="grid gap-3 xl:grid-cols-2">
            {features.map((f, i) => (
              <li
                key={f.title}
                className="mik-rise mik-hover-lift glass-chip flex items-start gap-3 rounded-xl p-3.5"
                style={{ animationDelay: `${0.11 + i * 0.07}s` }}
              >
                <span className="mt-0.5 flex size-9 shrink-0 items-center justify-center rounded-lg bg-primary/15 text-primary">
                  <f.icon className="size-4.5" aria-hidden />
                </span>
                <span>
                  <span className="block text-sm font-medium">{t(f.title)}</span>
                  <span className="mt-0.5 block text-xs leading-relaxed text-muted-foreground">{t(f.desc)}</span>
                </span>
              </li>
            ))}
          </ul>
          <p className="mik-rise mt-8 text-xs text-muted-foreground/70" style={{ animationDelay: "0.46s" }}>
            {t("login.footer")}
          </p>
          <p className="mik-rise mt-2" style={{ animationDelay: "0.53s" }}>
            <FtciCredit className="text-xs text-muted-foreground/80" />
          </p>
        </div>
      </div>
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
  // Sécurité S4 — 2FA TOTP : le backend répond 401 + code « totp_required »
  // quand l'utilisateur a activé la double authentification ; l'écran passe
  // alors en mode second étape (saisie du code à 6 chiffres).
  const [awaitingTotp, setAwaitingTotp] = useState(false);
  const [totpCode, setTotpCode] = useState("");

  // N°8 — Mode Vente (revendeur, identifiant + PIN)
  const [sellUsername, setSellUsername] = useState("");
  const [sellPin, setSellPin] = useState("");
  const [sellLoading, setSellLoading] = useState(false);

  // N°68 — « Mot de passe oublié ? » : modale de demande de lien e-mail.
  const [forgotOpen, setForgotOpen] = useState(false);

  // Micro-feedback d'erreur : la carte de verre tremble (N°78 — relance CSS
  // par retrait/retour forcé/rajout de la classe sur le MÊME nœud DOM, le
  // focus des champs est préservé).
  const cardRef = useRef<HTMLDivElement>(null);
  function shakeCard() {
    const el = cardRef.current;
    if (!el) return;
    el.classList.remove("mik-shake");
    void el.offsetWidth; // reflow : réarme l'animation sans démonter la carte
    el.classList.add("mik-shake");
  }

  const canLogin =
    username.trim().length > 0 &&
    password.trim().length > 0 &&
    (!awaitingTotp || totpCode.trim().length === 6) &&
    !loginLoading;
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
        body: {
          username: username.trim(),
          password,
          // Second étape TOTP (S4) : le code n'est envoyé qu'une fois le
          // backend passé en mode « totp_required ».
          ...(awaitingTotp ? { code: totpCode.trim() } : {}),
        },
      });
      applyAuth(res);
    } catch (err) {
      if (err instanceof ApiError && err.code === "totp_required") {
        setAwaitingTotp(true);
        toast.info(t("login.totpPrompt"));
      } else {
        shakeCard();
        toast.error(err instanceof Error ? err.message : t("login.failed"));
      }
    } finally {
      setLoginLoading(false);
    }
  }

  return (
    // N°60 — pwa-safe-top : en PWA iOS (status bar black-translucent +
    // viewport-fit cover), le contenu coule sous l'encoche — le padding
    // compense. En navigateur classique, env() vaut 0 : aucun effet.
    <div className="pwa-safe-top grid min-h-screen lg:grid-cols-[1.08fr_1fr]">
      <BrandPanel />

      {/* Colonne formulaire */}
      <section className="bg-glow relative flex min-h-screen flex-col items-center justify-center overflow-hidden px-4 py-10 sm:px-8">
        {/* Modale « Mot de passe oublié ? » (N°68) — survit au-dessus de tout. */}
        <ForgotPasswordModal open={forgotOpen} onOpenChange={setForgotOpen} />
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
        <div
          aria-hidden
          className="mik-orb-pulse absolute -bottom-24 -right-32 size-[26rem] rounded-full bg-primary/10 blur-3xl"
        />

        <div className="relative z-10 flex w-full max-w-md flex-1 flex-col items-center justify-center">
          {/* En-tête branding compact (mobile / tablette) */}
          <div
            className="mik-rise mb-8 flex flex-col items-center text-center lg:hidden"
            style={{ animationDuration: "0.45s" }}
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
          </div>

          {/* Carte de verre */}
          <div
            ref={cardRef}
            className="mik-card-in glass-card w-full rounded-2xl p-6 sm:p-8"
            style={{ animationDelay: "0.08s" }}
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
                <form onSubmit={handleLogin} className="space-y-4">
                  <div className="mik-rise space-y-2" style={{ animationDelay: "0.04s" }}>
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
                  <div className="mik-rise space-y-2" style={{ animationDelay: "0.11s" }}>
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
                    {/* N°68 — lien d'entrée du parcours de réinitialisation. */}
                    <div className="flex justify-end">
                      <button
                        type="button"
                        onClick={() => setForgotOpen(true)}
                        className="text-xs font-medium text-muted-foreground transition-colors hover:text-primary hover:underline focus-visible:outline-none focus-visible:underline"
                      >
                        {t("login.forgot", "Mot de passe oublié ?")}
                      </button>
                    </div>
                  </div>
                  {awaitingTotp && (
                    <div className="mik-rise space-y-2" style={{ animationDelay: "0.18s" }}>
                      <Label htmlFor="login-totp">{t("login.totpCode")}</Label>
                      <Input
                        id="login-totp"
                        inputMode="numeric"
                        autoComplete="one-time-code"
                        placeholder="000000"
                        maxLength={6}
                        value={totpCode}
                        onChange={(e) => setTotpCode(e.target.value.replace(/[^0-9]/g, ""))}
                        disabled={loginLoading}
                        className="text-center font-mono tracking-[0.4em]"
                      />
                      <p className="text-xs text-muted-foreground">{t("login.totpHint")}</p>
                    </div>
                  )}
                  <div className="mik-rise mik-press" style={{ animationDelay: "0.25s" }}>
                    <Button type="submit" className="w-full shadow-lg shadow-primary/25" disabled={!canLogin}>
                      {loginLoading && <Loader2 className="size-4 animate-spin" />}
                      {awaitingTotp ? t("login.totpSubmit") : t("login.tabLogin")}
                    </Button>
                  </div>
                </form>
              </TabsContent>

              {onSignUp && (
                <p className="mik-rise text-center text-sm text-muted-foreground" style={{ animationDelay: "0.32s" }}>
                  {t("login.noAccount", "Pas encore de compte ?")}{" "}
                  <button onClick={onSignUp} className="font-medium text-primary hover:underline">
                    {t("login.createAccount", "Créer mon compte")}
                  </button>
                </p>
              )}

              <TabsContent value="sell">
                <form onSubmit={handleSellLogin} className="space-y-4">
                  <p
                    className="mik-rise glass-chip rounded-lg px-3 py-2 text-xs text-muted-foreground"
                    style={{ animationDelay: "0.04s" }}
                  >
                    {t("login.sellHint")}
                  </p>
                  <div className="mik-rise space-y-2" style={{ animationDelay: "0.11s" }}>
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
                  <div className="mik-rise space-y-2" style={{ animationDelay: "0.18s" }}>
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
                  <div className="mik-rise mik-press" style={{ animationDelay: "0.25s" }}>
                    <Button type="submit" className="w-full shadow-lg shadow-primary/25" disabled={!canSell}>
                      {sellLoading && <Loader2 className="size-4 animate-spin" />}
                      <Store className="size-4" />
                      {t("login.sellSubmit")}
                    </Button>
                  </div>
                </form>
              </TabsContent>
            </Tabs>

            {SHOW_DEMO && (
              <div className="mik-fade glass-chip mt-6 flex items-center justify-between gap-3 rounded-xl px-3 py-2.5" style={{ animationDelay: "0.4s" }}>
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
              </div>
            )}
          </div>

          {/* N°60 — PWA : la barre d'installation vit sous la carte de
              connexion (funnel commun console + Mode Vente — la mini-infobar
              Chrome est suspendue par le script du layout, c'est CE CTA qui
              la remplace). Disparaît dès que l'app est installée/standalone. */}
          <div className="mik-rise w-full" style={{ animationDelay: "0.35s", animationDuration: "0.4s" }}>
            <PwaInstallCta />
          </div>

          <p
            className="mik-fade mt-6 text-center text-xs text-muted-foreground lg:hidden"
            style={{ animationDelay: "0.55s" }}
          >
            {t("login.footer")}
          </p>
          <p className="mik-fade mt-2 text-center lg:hidden" style={{ animationDelay: "0.6s" }}>
            <FtciCredit className="text-xs text-muted-foreground/80" />
          </p>
        </div>
      </section>
    </div>
  );
}
