"use client";

// N°68 — Modale « Mot de passe oublié ? » de l'écran de connexion.
// L'utilisateur saisit l'e-mail enregistré à la création de son compte :
//   - e-mail inconnu → le signaler (message du backend, carte qui tremble) ;
//   - e-mail connu → un lien de réinitialisation part (valable 60 minutes,
//     une seule utilisation) — écran de confirmation + rappel anti-spam.
// La consommation du lien vit sur la page publique /reset-password.

import { useState, type FormEvent } from "react";
import { motion, useAnimate, useReducedMotion, type Variants } from "framer-motion";
import { CheckCircle2, KeyRound, Loader2, Mail, MailCheck } from "lucide-react";
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
import { ApiError, api } from "@/lib/hotspot/api";
import { useI18n } from "@/lib/hotspot/i18n";

/* Micro-animations : champs en cascade (cohérent avec login-screen). */
const stagger: Variants = {
  hidden: {},
  show: { transition: { staggerChildren: 0.07, delayChildren: 0.04 } },
};
const rise: Variants = {
  hidden: { opacity: 0, y: 14 },
  show: { opacity: 1, y: 0, transition: { duration: 0.38, ease: "easeOut" } },
};

/* Validation e-mail RFC simple (miroir du backend, message plus tôt). */
const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

/** Durée de validité affichée (contrat backend : 60 minutes). */
const RESET_TTL_MIN = 60;

export interface ForgotPasswordModalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export default function ForgotPasswordModal({ open, onOpenChange }: ForgotPasswordModalProps) {
  const { t, tf } = useI18n();

  const [email, setEmail] = useState("");
  const [loading, setLoading] = useState(false);
  // « form » : saisie ; « sent » : confirmation (le lien est parti).
  const [sentTo, setSentTo] = useState<string | null>(null);
  const [scope, animate] = useAnimate();
  const reduce = useReducedMotion();

  function shakeCard() {
    if (scope.current && !reduce) {
      void animate(scope.current, { x: [0, -10, 10, -6, 6, 0] }, { duration: 0.45, ease: "easeInOut" });
    }
  }

  function handleOpenChange(next: boolean) {
    if (!next) {
      // Réinitialisation différée : laisse sortir l'animation de fermeture.
      setTimeout(() => {
        setEmail("");
        setSentTo(null);
      }, 200);
    }
    onOpenChange(next);
  }

  const emailValid = EMAIL_RE.test(email.trim());
  const canSubmit = emailValid && !loading;

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    if (!canSubmit) return;
    setLoading(true);
    try {
      await api<{ ok: boolean }>("/api/auth/forgot-password", {
        method: "POST",
        body: { email: email.trim() },
      });
      setSentTo(email.trim());
      toast.success(t("forgot.sentTitle", "Lien envoyé"));
    } catch (err) {
      // E-mail inconnu (404 explicite), envoi impossible (502/503) ou quota
      // (429) : le backend porte le message français — on le montre tel quel.
      shakeCard();
      toast.error(err instanceof Error ? err.message : t("forgot.sendFailed", "E-mail non envoyé — réessayez plus tard"));
    } finally {
      setLoading(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-md">
        {sentTo === null ? (
          <>
            <DialogHeader>
              <DialogTitle className="flex items-center gap-2 text-xl">
                <KeyRound className="size-5 text-primary" />
                {t("forgot.title", "Réinitialiser votre mot de passe")}
              </DialogTitle>
              <DialogDescription>{t("forgot.desc")}</DialogDescription>
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
                <Label htmlFor="forgot-email">{t("forgot.email", "Adresse e-mail")}</Label>
                <div className="relative">
                  <Mail className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" aria-hidden />
                  <Input
                    id="forgot-email"
                    type="email"
                    inputMode="email"
                    autoComplete="email"
                    placeholder={t("forgot.emailPlaceholder", "vous@exemple.ci")}
                    className="pl-9"
                    value={email}
                    onChange={(e) => setEmail(e.target.value)}
                    disabled={loading}
                    autoFocus
                    aria-invalid={email.length > 0 && !emailValid}
                  />
                </div>
                {email.length > 0 && !emailValid && (
                  <p className="text-xs text-destructive" role="alert">
                    {t("forgot.emailInvalid", "Adresse e-mail invalide")}
                  </p>
                )}
              </motion.div>

              <motion.div variants={rise} whileHover={{ scale: 1.015 }} whileTap={{ scale: 0.97 }}>
                <Button type="submit" className="w-full shadow-lg shadow-primary/25" disabled={!canSubmit}>
                  {loading ? <Loader2 className="size-4 animate-spin" /> : <MailCheck className="size-4" />}
                  {loading ? t("forgot.sending", "Envoi…") : t("forgot.submit", "Envoyer le lien")}
                </Button>
              </motion.div>
            </motion.form>
          </>
        ) : (
          <div className="flex flex-col items-center gap-4 py-6 text-center" role="status">
            <span className="flex size-14 items-center justify-center rounded-full bg-emerald-500/15 text-emerald-500">
              <CheckCircle2 className="size-7" aria-hidden />
            </span>
            <div className="space-y-1.5">
              <h3 className="text-lg font-semibold">{t("forgot.sentTitle", "Lien envoyé")}</h3>
              <p className="text-sm leading-relaxed text-muted-foreground">
                {tf("forgot.sentDesc", { email: sentTo, minutes: RESET_TTL_MIN })}
              </p>
              <p className="text-xs text-muted-foreground/80">{t("forgot.sentSpamHint")}</p>
            </div>
            <Button
              type="button"
              variant="outline"
              className="w-full"
              onClick={() => handleOpenChange(false)}
            >
              {t("forgot.close", "Fermer")}
            </Button>
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}
