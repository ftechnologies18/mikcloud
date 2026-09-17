"use client";

// N°143 — Écran de connexion porté par « Miko », la mascotte flat design.
// Le formulaire historique (onglets Radix Console / Mode Vente) devient un
// TOGGLE CLAY Admin / Revendeur à pastille glissante : même contrat métier
// (Admin → POST /api/auth/login + étape 2FA ; Revendeur → POST
// /api/reseller/login par PIN), mais le personnage au-dessus de la carte
// vit chaque interaction — pupilles qui suivent l'identifiant, mains sur
// les yeux pendant les secrets, œillo quand on affiche le mot de passe,
// étonnement à l'échec, joie au succès, bulle 2FA qui se remplit.
// Tout l'historique fonctionnel est conservé : réveil proactif du backend
// (N°84) + filet cold-boot, 2FA TOTP (S4), « Mot de passe oublié ? » (N°68),
// inscription (SignupModal), CTA PWA (N°60), crédit FTCI (N°94).
// Discipline N°78 : zéro framer-motion ici — les animations vivent en CSS
// (globals.css, classes mik-*), le personnage inclus (login-mascot.tsx).

import { useEffect, useRef, useState } from "react";
import Image from "next/image";
import { Eye, EyeOff, Loader2, ShieldCheck, Store, Ticket, Wifi } from "lucide-react";
import { toast } from "sonner";

import { FtciCredit } from "@/components/ftci-credit";
import { PwaInstallCta } from "@/components/pwa-install-cta";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import ForgotPasswordModal from "@/components/hotspot/parts/forgot-password-modal";
import LoginMascot, { type MascotMood } from "@/components/hotspot/login-mascot";
import { ApiError, api, wakeBackend } from "@/lib/hotspot/api";
import { useI18n } from "@/lib/hotspot/i18n";
import { useHotspotStore } from "@/lib/hotspot/store";
import type { AuthResponse } from "@/lib/hotspot/types";
import { cn } from "@/lib/utils";

// N°84 — filet cold boot : le backend Render du plan gratuit hiberne après
// ~15 min sans trafic et met 30–90 s à redémarrer. Sur la requête
// authentifiante, un échec RÉSEAU (timeout/connexion — pas une réponse HTTP
// d'erreur, qui reste traitée par la logique normale) déclenche UNE seconde
// tentative patiente (75 s) avec un message explicite. Le login est sûr à
// rejouer : aucune écriture métier, au pire deux sessions JWT sont créées
// (la première expire naturellement). Les autres POST (génération de
// vouchers, e-mails…) ne DOIVENT PAS utiliser ce filet : un timeout peut
// masquer un traitement serveur réussi → double effet de bord.
async function withColdBootRetry<T>(
  attempt: (timeoutMs: number) => Promise<T>,
  onWake: () => void,
): Promise<T> {
  try {
    return await attempt(20_000);
  } catch (err) {
    if (err instanceof ApiError) throw err; // erreur serveur réelle (401, totp_required…)
    onWake();
    return await attempt(75_000);
  }
}

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
          {/* N°94 — mention « © 2025 MikCloud — Connectez vos routeurs… »
              retirée : seul subsiste le crédit FTCI, ancré au même emplacement. */}
          <p className="mik-rise mt-8" style={{ animationDelay: "0.46s" }}>
            <FtciCredit className="text-xs text-muted-foreground/80" />
          </p>
        </div>
      </div>
    </aside>
  );
}

/* Rôles du toggle N°143 — Admin (console) / Revendeur (Mode Vente). */
type LoginMode = "admin" | "reseller";
type ActiveField = null | "username" | "password" | "totp" | "sellUsername" | "sellPin";

export default function LoginScreen({ onBack, onSignUp }: { onBack?: () => void; onSignUp?: () => void }) {
  const { t, tf } = useI18n();
  const setAuth = useHotspotStore((s) => s.setAuth);

  // N°84 — réveil proactif du backend : ping silencieux au premier montage
  // de l'écran (garde module dans wakeBackend, les re-rendus ne relancent
  // rien). Sur Render plan gratuit, le serveur démarre PENDANT que
  // l'utilisateur tape ses identifiants — la soumission part sur un serveur
  // déjà chaud, et le filet withColdBootRetry couvre le reste.
  useEffect(() => {
    wakeBackend();
  }, []);

  // ─── Connexion Admin ───
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [loginLoading, setLoginLoading] = useState(false);
  const [showPassword, setShowPassword] = useState(false);
  // Sécurité S4 — 2FA TOTP : le backend répond 401 + code « totp_required »
  // quand l'utilisateur a activé la double authentification ; l'écran passe
  // alors en mode second étape (saisie du code à 6 chiffres).
  const [awaitingTotp, setAwaitingTotp] = useState(false);
  const [totpCode, setTotpCode] = useState("");

  // ─── Mode Vente (revendeur, identifiant + PIN) ───
  const [sellUsername, setSellUsername] = useState("");
  const [sellPin, setSellPin] = useState("");
  const [sellLoading, setSellLoading] = useState(false);

  // N°68 — « Mot de passe oublié ? » : modale de demande de lien e-mail.
  const [forgotOpen, setForgotOpen] = useState(false);

  // ─── N°143 — Miko, la mascotte ───
  const [mode, setMode] = useState<LoginMode>("admin");
  const [activeField, setActiveField] = useState<ActiveField>(null);
  const [gaze, setGaze] = useState({ x: 0, y: 0.1 });
  // Humeur transitoire (échec / succès) : prime sur l'humeur dérivée.
  const [flash, setFlash] = useState<null | "shocked" | "happy">(null);
  const [waving, setWaving] = useState(false);
  const [bubble, setBubble] = useState<{ key: number; text: string } | null>(null);

  const bubbleTimer = useRef<number | null>(null);
  const flashTimer = useRef<number | null>(null);
  // Répliques « une fois par session » : le voile des secrets et l'œillo.
  const coverHintShown = useRef(false);
  const peekHintShown = useRef(false);

  // Nettoyage des minuteurs au démontage (redirection après succès).
  useEffect(
    () => () => {
      if (bubbleTimer.current) window.clearTimeout(bubbleTimer.current);
      if (flashTimer.current) window.clearTimeout(flashTimer.current);
    },
    [],
  );

  /** Miko parle : bulle de dialogue auto-effacée (~2,6 s). */
  function say(text: string) {
    if (bubbleTimer.current) window.clearTimeout(bubbleTimer.current);
    setBubble({ key: performance.now(), text });
    bubbleTimer.current = window.setTimeout(() => setBubble(null), 2600);
  }

  /** Humeur éphémère (échec / succès) puis retour au calme. */
  function flashMood(m: "shocked" | "happy") {
    setFlash(m);
    if (flashTimer.current) window.clearTimeout(flashTimer.current);
    flashTimer.current = window.setTimeout(() => setFlash(null), m === "happy" ? 2200 : 1900);
  }

  // Salut du montage (une fois, à la langue du premier rendu).
  const hello = t("login.mascot.hello");
  useEffect(() => {
    const id = window.setTimeout(() => setBubble({ key: 1, text: hello }), 700);
    return () => window.clearTimeout(id);
  }, [hello]);

  /** Les pupilles suivent le caret de l'identifiant (progression 0→1). */
  function trackCaret(e: { currentTarget: HTMLInputElement }) {
    const el = e.currentTarget;
    const pos = el.selectionStart ?? el.value.length;
    const ratio = Math.min(Math.max(pos / 14, 0), 1);
    setGaze({ x: (ratio * 2 - 1) * 0.72, y: 0.34 });
  }

  function focusUsername(field: "username" | "sellUsername") {
    return {
      onFocus: (e: React.FocusEvent<HTMLInputElement>) => {
        setActiveField(field);
        trackCaret(e);
      },
      onBlur: blurField,
    };
  }

  function focusSecret(field: "password" | "sellPin") {
    return {
      onFocus: () => {
        setActiveField(field);
        setGaze({ x: 0, y: 0.75 });
        if (!coverHintShown.current) {
          coverHintShown.current = true;
          say(t("login.mascot.cover"));
        }
      },
      onBlur: blurField,
    };
  }

  function blurField() {
    setActiveField(null);
    setGaze({ x: 0, y: 0.1 });
  }

  /** Bascule Admin ↔ Revendeur : tenue de Miko + salut + micro-fiche. */
  function switchMode(next: LoginMode) {
    if (next === mode || loginLoading || sellLoading) return;
    setMode(next);
    setActiveField(null);
    setGaze({ x: 0, y: 0.1 });
    setWaving(true);
    window.setTimeout(() => setWaving(false), 950);
    say(t(next === "admin" ? "login.mascot.modeAdmin" : "login.mascot.modeReseller"));
  }

  // Micro-feedback d'erreur : la carte de verre tremble (N°78 — relance CSS
  // par retrait/retour forcé/rajout de la classe sur le MÊME nœud DOM, le
  // focus des champs est préservé). N°143 : Miko s'étonne en plus.
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

  // ─── État dérivé de Miko ───
  const covering = activeField === "password" || activeField === "sellPin";
  const peeking = covering && activeField === "password" && showPassword;
  const mood: MascotMood =
    flash ?? ((loginLoading || sellLoading) ? "excited" : awaitingTotp ? "curious" : "idle");

  /** Œil d'affichage du mot de passe : Miko garde le focus sur le champ
   *  (mousedown empêché) et ne triche qu'un petit œil — une fois. */
  function toggleShowPassword() {
    const next = !showPassword;
    setShowPassword(next);
    if (next && activeField === "password" && !peekHintShown.current) {
      peekHintShown.current = true;
      say(t("login.mascot.peek"));
    }
  }

  function failMascot() {
    shakeCard();
    flashMood("shocked");
    say(t("login.mascot.error"));
  }

  // N°8 — connexion revendeur par PIN : token scopé role=reseller → SellShell.
  async function handleSellLogin(e: React.FormEvent) {
    e.preventDefault();
    if (!canSell) return;
    setSellLoading(true);
    try {
      const res = await withColdBootRetry(
        (timeoutMs) =>
          api<{ token: string; reseller: { id: string; name: string; username: string } }>(
            "/api/reseller/login",
            { method: "POST", timeoutMs, body: { username: sellUsername.trim(), pin: sellPin } },
          ),
        () => toast.info(t("login.serverWaking")),
      );
      setAuth(res.token, {
        id: res.reseller.id,
        name: res.reseller.name,
        username: res.reseller.username,
        role: "reseller",
      });
      flashMood("happy");
      say(t("login.mascot.success"));
      toast.success(tf("login.welcome", { name: res.reseller.name }));
    } catch (err) {
      failMascot();
      // N°84 — un échec réseau brut (timeout cold boot, hors-ligne) affiche
      // un message humain, pas le DOMException du navigateur (« signal
      // timed out ») : la cible gérant n'a pas à décoder un message interne.
      toast.error(err instanceof ApiError ? err.message : t("login.networkError"));
    } finally {
      setSellLoading(false);
    }
  }

  function applyAuth(res: AuthResponse) {
    setAuth(res.token, res.user);
    flashMood("happy");
    say(t("login.mascot.success"));
    toast.success(tf("login.welcome", { name: res.user.name }));
  }

  async function handleLogin(e: React.FormEvent) {
    e.preventDefault();
    if (!canLogin) return;
    setLoginLoading(true);
    try {
      const res = await withColdBootRetry(
        (timeoutMs) =>
          api<AuthResponse>("/api/auth/login", {
            method: "POST",
            timeoutMs,
            body: {
              username: username.trim(),
              password,
              // Second étape TOTP (S4) : le code n'est envoyé qu'une fois le
              // backend passé en mode « totp_required ».
              ...(awaitingTotp ? { code: totpCode.trim() } : {}),
            },
          }),
        () => toast.info(t("login.serverWaking")),
      );
      applyAuth(res);
    } catch (err) {
      if (err instanceof ApiError && err.code === "totp_required") {
        setAwaitingTotp(true);
        say(t("login.mascot.totp"));
        toast.info(t("login.totpPrompt"));
      } else {
        failMascot();
        // N°84 — cf. handleSellLogin : message humain pour l'échec réseau.
        toast.error(err instanceof ApiError ? err.message : t("login.networkError"));
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
          {/* En-tête branding compact (mobile / tablette) — N°143 : réduit
              au strict nécessaire, la mascotte porte l'identité visuelle. */}
          <div
            className="mik-rise mb-4 flex flex-col items-center text-center lg:hidden"
            style={{ animationDuration: "0.45s" }}
          >
            <div className="flex items-center gap-3">
              <Image
                src="/logo.png"
                alt={t("login.logoAlt")}
                width={44}
                height={44}
                priority
                className="rounded-xl shadow-lg shadow-primary/20"
              />
              <div className="text-left">
                <h1 className="text-xl font-semibold tracking-tight">MikCloud</h1>
                <p className="text-xs text-muted-foreground">{t("login.tagline")}</p>
              </div>
            </div>
          </div>

          {/* ─── Miko, la mascotte (N°143) ───
              Ses mains reposent sur le bord de la carte (chevauchement
              ~14 px au-dessus du verre) ; la bulle de dialogue flotte à
              gauche de son antenne. pointer-events-none : rien d'interactif. */}
          <div
            className={cn(
              "pointer-events-none relative z-20 -mb-3.5 flex w-full justify-center",
              flash === "shocked" && "mik-shake",
            )}
          >
            <div
              className={cn("mik-mascot-in relative", (loginLoading || sellLoading) && "mik-mascot-cheer")}
              style={{ animationDelay: "0.02s" }}
            >
              {bubble && (
                <div
                  key={bubble.key}
                  aria-hidden="true"
                  className="mik-bubble absolute -top-2 -left-2 z-30 w-max max-w-[178px] rounded-xl border border-border bg-card px-3 py-1.5 text-[11px] font-medium leading-snug text-foreground shadow-lg sm:-left-8"
                >
                  {bubble.text}
                </div>
              )}
              <LoginMascot
                mode={mode}
                mood={mood}
                gaze={gaze}
                covering={covering}
                peeking={peeking}
                waving={waving}
                totpActive={awaitingTotp}
                totpDots={totpCode.length}
                label={t("login.mascot.alt")}
                className="w-[204px] sm:w-[232px]"
              />
            </div>
          </div>

          {/* Carte de verre */}
          <div
            ref={cardRef}
            className="mik-card-in glass-card w-full rounded-2xl p-6 sm:p-8"
            style={{ animationDelay: "0.08s" }}
          >
            <div className="mb-5 hidden lg:block">
              <h2 className="text-xl font-semibold tracking-tight">{t("login.form.title")}</h2>
              <p className="mt-1 text-sm text-muted-foreground">{t("login.form.subtitle")}</p>
            </div>

            {/* ─── Toggle Admin / Revendeur (N°143) ───
                Bac en creux + pastille clay glissante (mik-role-toggle /
                mik-role-thumb, globals.css) ; remplace les onglets Radix —
                même destinations : Admin → console, Revendeur → Mode Vente. */}
            <div
              className="mik-role-toggle relative mb-4 grid grid-cols-2 rounded-2xl p-1.5"
              role="group"
              aria-label={t("login.role.switchLabel")}
            >
              <span
                aria-hidden="true"
                className="mik-role-thumb absolute bottom-1.5 left-1.5 top-1.5 w-[calc(50%-0.375rem)] rounded-xl"
                style={{
                  transform:
                    mode === "admin" ? "translateX(0)" : "translateX(calc(100% + 0.375rem))",
                }}
              />
              <button
                type="button"
                onClick={() => switchMode("admin")}
                aria-pressed={mode === "admin"}
                className={cn(
                  "relative z-10 flex items-center justify-center gap-2 rounded-xl py-2.5 text-sm font-medium transition-colors duration-200 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/60",
                  mode === "admin" ? "text-accent-foreground" : "text-muted-foreground hover:text-foreground",
                )}
              >
                <ShieldCheck className="size-4" aria-hidden />
                {t("login.role.admin")}
              </button>
              <button
                type="button"
                onClick={() => switchMode("reseller")}
                aria-pressed={mode === "reseller"}
                className={cn(
                  "relative z-10 flex items-center justify-center gap-2 rounded-xl py-2.5 text-sm font-medium transition-colors duration-200 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/60",
                  mode === "reseller"
                    ? "text-accent-foreground"
                    : "text-muted-foreground hover:text-foreground",
                )}
              >
                <Store className="size-4" aria-hidden />
                {t("login.role.reseller")}
              </button>
            </div>
            <p className="mb-5 text-center text-xs text-muted-foreground">
              {t(mode === "admin" ? "login.role.adminDesc" : "login.role.resellerDesc")}
            </p>

            {mode === "admin" ? (
              /* ─── Formulaire Admin (console de gestion) ─── */
              <form key="mik-form-admin" onSubmit={handleLogin} className="space-y-4">
                <div className="mik-rise space-y-2" style={{ animationDelay: "0.04s" }}>
                  <Label htmlFor="login-username">{t("login.username")}</Label>
                  <Input
                    id="login-username"
                    autoComplete="username"
                    placeholder="admin"
                    value={username}
                    {...focusUsername("username")}
                    onChange={(e) => {
                      setUsername(e.target.value);
                      trackCaret(e);
                    }}
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
                      {...focusSecret("password")}
                      onChange={(e) => setPassword(e.target.value)}
                      disabled={loginLoading}
                    />
                    <button
                      type="button"
                      onClick={toggleShowPassword}
                      onMouseDown={(e) => e.preventDefault()} // le champ garde le focus : Miko ne baisse pas les mains
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
                      onFocus={() => {
                        setActiveField("totp");
                        setGaze({ x: 0.62, y: -0.55 });
                      }}
                      onBlur={blurField}
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
            ) : (
              /* ─── Formulaire Revendeur (Mode Vente, N°8) ─── */
              <form key="mik-form-reseller" onSubmit={handleSellLogin} className="space-y-4">
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
                    {...focusUsername("sellUsername")}
                    onChange={(e) => {
                      setSellUsername(e.target.value);
                      trackCaret(e);
                    }}
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
                    {...focusSecret("sellPin")}
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
            )}

            {onSignUp && (
              <p className="mik-rise mt-5 text-center text-sm text-muted-foreground" style={{ animationDelay: "0.32s" }}>
                {t("login.noAccount", "Pas encore de compte ?")}{" "}
                <button onClick={onSignUp} className="font-medium text-primary hover:underline">
                  {t("login.createAccount", "Créer mon compte")}
                </button>
              </p>
            )}

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
                    setGaze({ x: 0.25, y: 0.34 });
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

          {/* N°94 — idem mobile : une seule ligne de pied, le crédit FTCI. */}
          <p className="mik-fade mt-6 text-center lg:hidden" style={{ animationDelay: "0.55s" }}>
            <FtciCredit className="text-xs text-muted-foreground/80" />
          </p>
        </div>
      </section>
    </div>
  );
}
