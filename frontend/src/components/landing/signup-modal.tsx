"use client";

import { useState, type FormEvent } from "react";
import { motion, useAnimate, useReducedMotion, type Variants } from "framer-motion";
import { Eye, EyeOff, Loader2, Rocket, ShieldCheck } from "lucide-react";
import { toast } from "sonner";

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { register } from "@/lib/hotspot/api";
import { useI18n } from "@/lib/hotspot/i18n";
import { useHotspotStore } from "@/lib/hotspot/store";

/* Micro-animations : champs en cascade (cohérent avec login-screen). */
const stagger: Variants = {
  hidden: {},
  show: { transition: { staggerChildren: 0.07, delayChildren: 0.04 } },
};
const rise: Variants = {
  hidden: { opacity: 0, y: 14 },
  show: { opacity: 1, y: 0, transition: { duration: 0.38, ease: "easeOut" } },
};

export interface SignupModalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export default function SignupModal({ open, onOpenChange }: SignupModalProps) {
  const { t } = useI18n();
  const setAuth = useHotspotStore((s) => s.setAuth);

  const [name, setName] = useState("");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [loading, setLoading] = useState(false);
  const [scope, animate] = useAnimate();
  const reduce = useReducedMotion();

  function shakeCard() {
    if (scope.current && !reduce) {
      void animate(scope.current, { x: [0, -10, 10, -6, 6, 0] }, { duration: 0.45, ease: "easeInOut" });
    }
  }

  const canSubmit =
    name.trim().length > 0 &&
    username.trim().length >= 3 &&
    password.length >= 8 &&
    !loading;

  function resetForm() {
    setName("");
    setUsername("");
    setPassword("");
    setShowPassword(false);
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    if (!canSubmit) return;
    setLoading(true);
    try {
      const res = await register({
        name: name.trim(),
        username: username.trim(),
        password,
      });
      setAuth(res.token, res.user);
      toast.success(t("signup.success", "Compte créé — bienvenue sur MikCloud !"));
      onOpenChange(false);
      resetForm();
    } catch (err) {
      shakeCard();
      toast.error(err instanceof Error ? err.message : t("signup.failed", "Inscription impossible"));
    } finally {
      setLoading(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2 text-xl">
            <Rocket className="size-5 text-primary" />
            {t("signup.title", "Créer mon compte")}
          </DialogTitle>
          <DialogDescription>
            {t(
              "signup.subtitle",
              "Essai gratuit de 90 jours — 1 routeur inclus, sans carte bancaire.",
            )}
          </DialogDescription>
        </DialogHeader>

        <motion.form
          ref={scope}
          variants={stagger}
          initial="hidden"
          animate="show"
          onSubmit={handleSubmit}
          className="space-y-4 pt-2"
        >
          <motion.div variants={rise} className="space-y-2">
            <Label htmlFor="signup-name">{t("signup.name", "Nom du compte")}</Label>
            <Input
              id="signup-name"
              autoComplete="organization"
              placeholder="ProMax Wifi"
              value={name}
              onChange={(e) => setName(e.target.value)}
              disabled={loading}
            />
          </motion.div>

          <motion.div variants={rise} className="space-y-2">
            <Label htmlFor="signup-username">{t("signup.username", "Identifiant")}</Label>
            <Input
              id="signup-username"
              autoComplete="username"
              placeholder="gerant1"
              value={username}
              onChange={(e) => setUsername(e.target.value.toLowerCase())}
              disabled={loading}
            />
            <p className="text-xs text-muted-foreground">
              {t("signup.usernameHint", "3 à 32 caractères : a-z, 0-9, tirets et tirets bas")}
            </p>
          </motion.div>

          <motion.div variants={rise} className="space-y-2">
            <Label htmlFor="signup-password">{t("signup.password", "Mot de passe")}</Label>
            <div className="relative">
              <Input
                id="signup-password"
                type={showPassword ? "text" : "password"}
                autoComplete="new-password"
                placeholder="••••••••"
                className="pr-10"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                disabled={loading}
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
            <p className="text-xs text-muted-foreground">
              {t("signup.passwordHint", "Minimum 8 caractères")}
            </p>
          </motion.div>

          <motion.div
            variants={rise}
            className="rounded-lg border border-primary/20 bg-primary/5 px-3 py-2.5 text-xs leading-relaxed text-muted-foreground"
          >
            <ShieldCheck className="mr-1.5 inline size-3.5 text-primary" />
            {t(
              "signup.trialNote",
              "Votre essai de 90 jours démarre immédiatement. 1 routeur inclus, sans carte bancaire. À l'expiration, choisissez votre formule (Essentiel ou Illimité).",
            )}
          </motion.div>

          <motion.div variants={rise} whileHover={{ scale: 1.015 }} whileTap={{ scale: 0.97 }}>
            <Button type="submit" className="w-full shadow-lg shadow-primary/25" disabled={!canSubmit}>
              {loading && <Loader2 className="size-4 animate-spin" />}
              {t("signup.submit", "Créer mon compte")}
            </Button>
          </motion.div>
        </motion.form>
      </DialogContent>
    </Dialog>
  );
}
