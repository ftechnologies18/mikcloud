"use client";

// N°68 — /reset-password?token=… : page PUBLIQUE de consommation du lien de
// réinitialisation e-maillé (« Mot de passe oublié ? »). Aucun état connecté :
// le secret du parcours est le token du lien (vérifié à la consommation,
// POST /api/auth/reset-password — usage unique, 60 minutes).
//
// Mobile-first (lien ouvert depuis un client mail, souvent un téléphone) :
// colonne centrée max-w-md, cibles tactiles ≥ 44 px, safe-area iOS, bascule de
// langue publique (même coquille que /join/[token]). Lecture client-only du
// token (window) : pas de useSearchParams (évite la boundary Suspense en
// pré-rendu) — pattern du projet.
//
// États : formulaire → succès (session révoquée, retour à /login) ou lien
// invalide/expiré/consommé (carte d'état + retour à la connexion).

import { useState, type FormEvent } from "react";
import { useRouter } from "next/navigation";
import Image from "next/image";
import { motion, useReducedMotion } from "framer-motion";
import { CheckCircle2, Eye, EyeOff, KeyRound, Loader2, LockKeyhole, ShieldAlert } from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { FtciCredit } from "@/components/ftci-credit";
import { api } from "@/lib/hotspot/api";
import { useI18n } from "@/lib/hotspot/i18n";

/** Longueur minimale (miroir de la politique S2 du backend). */
const MIN_PASSWORD = 10;

type Phase = "form" | "success" | "invalid";

export default function ResetPasswordPage() {
  const router = useRouter();
  const { t, lang, setLang } = useI18n();
  const reduce = useReducedMotion();

  // Token du lien — lecture client-only (cf. en-tête).
  const [token] = useState(() =>
    typeof window === "undefined" ? "" : (new URLSearchParams(window.location.search).get("token") ?? ""),
  );

  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [show, setShow] = useState(false);
  const [loading, setLoading] = useState(false);
  const [phase, setPhase] = useState<Phase>(token === "" ? "invalid" : "form");
  // Message d'erreur du backend quand le lien est refusé (expiré, utilisé…).
  const [invalidReason, setInvalidReason] = useState("");

  const longEnough = password.length >= MIN_PASSWORD;
  const canSubmit = longEnough && password === confirm && !loading;

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    if (!canSubmit) return;
    setLoading(true);
    try {
      await api<{ ok: boolean }>("/api/auth/reset-password", {
        method: "POST",
        body: { token, password },
      });
      setPhase("success");
    } catch (err) {
      // 400 : lien expiré / déjà utilisé / inconnu, ou politique refusée.
      // Message français porté par le backend — affiché tel quel.
      const msg = err instanceof Error ? err.message : "";
      if (msg) {
        setInvalidReason(msg);
        setPhase("invalid");
      } else {
        toast.error(t("forgot.sendFailed", "E-mail non envoyé — réessayez plus tard"));
      }
    } finally {
      setLoading(false);
    }
  }

  return (
    <main className="bg-glow flex min-h-screen flex-col px-4 pb-[max(2rem,env(safe-area-inset-bottom))] pt-[max(0.75rem,env(safe-area-inset-top))] sm:px-6">
      {/* Bascule de langue publique — autonymes FR/EN. */}
      <div className="mx-auto flex w-full max-w-md justify-end">
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="h-10 min-w-11 px-3 text-xs font-semibold text-muted-foreground"
          aria-label={t("joinPage.langAria", "Changer de langue")}
          onClick={() => setLang(lang === "fr" ? "en" : "fr")}
        >
          {lang === "fr" ? "EN" : "FR"}
        </Button>
      </div>

      {/* my-auto : centre sur grand écran, dégrade en haut si débordement. */}
      <motion.div
        initial={reduce ? undefined : { opacity: 0, y: 14 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.4, ease: "easeOut" }}
        className="mx-auto my-auto w-full max-w-md pt-3"
      >
        <div className="mb-5 flex flex-col items-center gap-2 text-center">
          <Image
            src="/logo.png"
            alt="MikCloud"
            width={64}
            height={64}
            priority
            className="rounded-xl shadow-lg shadow-primary/20"
          />
          <p className="text-xs font-medium tracking-wide text-muted-foreground">{t("reset.brand")}</p>
        </div>

        {phase === "form" && (
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2 text-xl">
                <KeyRound className="size-5 text-primary" />
                {t("reset.title", "Nouveau mot de passe")}
              </CardTitle>
              <CardDescription>{t("reset.desc")}</CardDescription>
            </CardHeader>
            <CardContent>
              <form onSubmit={handleSubmit} className="space-y-4">
                <div className="space-y-2">
                  <Label htmlFor="reset-password">{t("reset.newPassword", "Nouveau mot de passe")}</Label>
                  <div className="relative">
                    <Input
                      id="reset-password"
                      type={show ? "text" : "password"}
                      autoComplete="new-password"
                      placeholder="••••••••"
                      className="pr-10"
                      value={password}
                      onChange={(e) => setPassword(e.target.value)}
                      disabled={loading}
                      minLength={MIN_PASSWORD}
                      required
                      autoFocus
                    />
                    <button
                      type="button"
                      onClick={() => setShow((v) => !v)}
                      aria-label={t("settings.passwordShow", "Afficher le mot de passe")}
                      className="absolute right-2.5 top-1/2 -translate-y-1/2 text-muted-foreground transition-colors hover:text-foreground"
                    >
                      {show ? <EyeOff className="size-4" aria-hidden /> : <Eye className="size-4" aria-hidden />}
                    </button>
                  </div>
                </div>
                <div className="space-y-2">
                  <Label htmlFor="reset-confirm">{t("reset.confirmPassword", "Confirmer le mot de passe")}</Label>
                  <Input
                    id="reset-confirm"
                    type={show ? "text" : "password"}
                    autoComplete="new-password"
                    placeholder="••••••••"
                    value={confirm}
                    onChange={(e) => setConfirm(e.target.value)}
                    disabled={loading}
                    minLength={MIN_PASSWORD}
                    required
                    aria-invalid={confirm.length > 0 && password !== confirm}
                  />
                  {confirm.length > 0 && password !== confirm && (
                    <p className="text-xs text-destructive" role="alert">
                      {t("reset.mismatch", "Les deux mots de passe ne correspondent pas.")}
                    </p>
                  )}
                  {password.length > 0 && !longEnough && (
                    <p className="text-xs text-muted-foreground">{t("reset.passwordHint")}</p>
                  )}
                </div>
                <Button type="submit" className="h-11 w-full shadow-lg shadow-primary/25" disabled={!canSubmit}>
                  {loading ? <Loader2 className="size-4 animate-spin" /> : <LockKeyhole className="size-4" />}
                  {loading ? t("reset.submitting", "Enregistrement…") : t("reset.submit", "Définir mon mot de passe")}
                </Button>
              </form>
            </CardContent>
          </Card>
        )}

        {phase === "success" && (
          <Card role="status">
            <CardContent className="flex flex-col items-center gap-4 py-10 text-center">
              <span className="flex size-14 items-center justify-center rounded-full bg-emerald-500/15 text-emerald-500">
                <CheckCircle2 className="size-7" aria-hidden />
              </span>
              <div className="space-y-1.5">
                <h2 className="text-lg font-semibold">{t("reset.successTitle", "Mot de passe modifié")}</h2>
                <p className="text-sm leading-relaxed text-muted-foreground">{t("reset.successDesc")}</p>
              </div>
              <Button type="button" className="h-11 w-full" onClick={() => router.push("/login")}>
                {t("reset.login", "Se connecter")}
              </Button>
            </CardContent>
          </Card>
        )}

        {phase === "invalid" && (
          <Card>
            <CardContent className="flex flex-col items-center gap-4 py-10 text-center">
              <span className="flex size-14 items-center justify-center rounded-full bg-destructive/10 text-destructive">
                <ShieldAlert className="size-7" aria-hidden />
              </span>
              <div className="space-y-1.5">
                <h2 className="text-lg font-semibold">{t("reset.invalidTitle", "Lien invalide")}</h2>
                <p className="text-sm leading-relaxed text-muted-foreground">{t("reset.invalidDesc")}</p>
                {/* Raison fine du backend (expiré / déjà utilisé) si connue. */}
                {invalidReason && (
                  <p className="glass-chip rounded-lg px-3 py-1.5 text-left text-xs font-medium break-words text-foreground/90">
                    {invalidReason}
                  </p>
                )}
              </div>
              <Button
                type="button"
                variant="outline"
                className="h-11 w-full"
                onClick={() => router.push("/login")}
              >
                {t("reset.requestNew", "Retour à la connexion")}
              </Button>
            </CardContent>
          </Card>
        )}

        <p className="mt-6 text-center">
          <FtciCredit className="text-xs text-muted-foreground/80" />
        </p>
      </motion.div>
    </main>
  );
}
