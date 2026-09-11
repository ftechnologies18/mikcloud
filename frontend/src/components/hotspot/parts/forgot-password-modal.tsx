"use client";

// N°68 — Modale « Mot de passe oublié ? » de l'écran de connexion.
// L'utilisateur saisit l'e-mail enregistré à la création de son compte :
//   - e-mail inconnu → le signaler (message du backend, carte qui tremble) ;
//   - e-mail connu → un lien de réinitialisation part (valable 60 minutes,
//     une seule utilisation) — écran de confirmation + rappel anti-spam.
// La consommation du lien vit sur la page publique /reset-password.

import { useRef, useState, type FormEvent } from "react";
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

/* N°78 — micro-animations en CSS pur (classes mik-* de globals.css,
 * cascade par animation-delay — cohérent avec login-screen) : framer-motion
 * ne fait plus partie du bundle initial de l'écran de connexion. */

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
  const formRef = useRef<HTMLFormElement>(null);

  function shakeCard() {
    const el = formRef.current;
    if (!el) return;
    el.classList.remove("mik-shake");
    void el.offsetWidth; // reflow : réarme l'animation sur le même nœud
    el.classList.add("mik-shake");
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

            <form
              ref={formRef}
              onSubmit={handleSubmit}
              className="space-y-4 pt-2"
            >
              <div className="mik-rise space-y-2" style={{ animationDelay: "0.04s" }}>
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
              </div>

              <div className="mik-rise mik-press" style={{ animationDelay: "0.11s" }}>
                <Button type="submit" className="w-full shadow-lg shadow-primary/25" disabled={!canSubmit}>
                  {loading ? <Loader2 className="size-4 animate-spin" /> : <MailCheck className="size-4" />}
                  {loading ? t("forgot.sending", "Envoi…") : t("forgot.submit", "Envoyer le lien")}
                </Button>
              </div>
            </form>
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
