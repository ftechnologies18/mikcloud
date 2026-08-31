"use client";

import { useMemo, useState, type FormEvent } from "react";
import { motion, useAnimate, useReducedMotion, type Variants } from "framer-motion";
import { ArrowLeft, ArrowRight, Check, Eye, EyeOff, Loader2, MapPin, Rocket, ShieldCheck, User } from "lucide-react";
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { register } from "@/lib/hotspot/api";
import { useI18n } from "@/lib/hotspot/i18n";
import { useHotspotStore } from "@/lib/hotspot/store";
import { countriesByLang, countryByCode } from "./african-countries";

/* Micro-animations : champs en cascade (cohérent avec login-screen). */
const stagger: Variants = {
  hidden: {},
  show: { transition: { staggerChildren: 0.07, delayChildren: 0.04 } },
};
const rise: Variants = {
  hidden: { opacity: 0, y: 14 },
  show: { opacity: 1, y: 0, transition: { duration: 0.38, ease: "easeOut" } },
};

/* Validation : email RFC simple + WhatsApp 8-15 chiffres. */
const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
function digitsOnly(s: string): string {
  return s.replace(/\D/g, "");
}

export interface SignupModalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export default function SignupModal({ open, onOpenChange }: SignupModalProps) {
  const { t, lang } = useI18n();
  const setAuth = useHotspotStore((s) => s.setAuth);

  // Étape courante : 1 = compte, 2 = profil.
  const [step, setStep] = useState<1 | 2>(1);

  // Étape 1 — compte.
  const [name, setName] = useState("");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);

  // Étape 2 — profil.
  const [email, setEmail] = useState("");
  const [phone, setPhone] = useState("");
  const [country, setCountry] = useState("");
  const [city, setCity] = useState("");

  const [loading, setLoading] = useState(false);
  const [scope, animate] = useAnimate();
  const reduce = useReducedMotion();

  function shakeCard() {
    if (scope.current && !reduce) {
      void animate(scope.current, { x: [0, -10, 10, -6, 6, 0] }, { duration: 0.45, ease: "easeInOut" });
    }
  }

  function resetForm() {
    setName("");
    setUsername("");
    setPassword("");
    setShowPassword(false);
    setEmail("");
    setPhone("");
    setCountry("");
    setCity("");
    setStep(1);
  }

  // Indicatif téléphonique du pays sélectionné (pour le préfixe visuel).
  const selectedCountry = useMemo(() => countryByCode(country), [country]);
  const dialCode = selectedCountry?.dial ?? "";

  // Numéro WhatsApp nettoyé (chiffres uniquement, sans l'indicatif).
  const phoneDigits = digitsOnly(phone);

  // Validations par étape.
  const canSubmitStep1 =
    name.trim().length > 0 &&
    username.trim().length >= 3 &&
    password.length >= 8;
  const emailValid = EMAIL_RE.test(email.trim());
  const phoneValid = phoneDigits.length >= 8 && phoneDigits.length <= 15;
  const canSubmitStep2 = emailValid && phoneValid && country !== "";

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    if (!canSubmitStep2) return;
    setLoading(true);
    try {
      const res = await register({
        name: name.trim(),
        username: username.trim(),
        password,
        email: email.trim(),
        // On envoie le numéro complet (indicatif + chiffres) pour le backend,
        // qui refait un digitsOnly. Si l'utilisateur a saisi l'indicatif dans
        // le champ, on évite le double en ne le préfixant que s'il n'y est pas.
        phone: dialCode && !phoneDigits.startsWith(dialCode) ? dialCode + phoneDigits : phoneDigits,
        country,
        city: city.trim(),
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

  function handleClose(open: boolean) {
    if (!open) {
      // Réinitialise l'étape en quittant.
      setTimeout(() => resetForm(), 200);
    }
    onOpenChange(open);
  }

  const countries = useMemo(() => countriesByLang(lang), [lang]);

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2 text-xl">
            <Rocket className="size-5 text-primary" />
            {step === 1
              ? t("signup.title", "Créer mon compte")
              : t("signup.titleStep2", "Votre profil")}
          </DialogTitle>
          <DialogDescription>
            {step === 1
              ? t("signup.subtitle", "Essai gratuit de 90 jours — 1 routeur inclus, sans carte bancaire.")
              : t("signup.subtitleStep2", "Pour vous aider et segmenter notre offre — nous ne spammons jamais.")}
          </DialogDescription>
        </DialogHeader>

        {/* Indicateur d'étapes */}
        <div className="flex items-center gap-2 px-1 pb-2">
          <div className={`flex items-center gap-1.5 text-xs ${step === 1 ? "text-primary font-medium" : "text-muted-foreground"}`}>
            <span className={`grid size-5 place-items-center rounded-full text-[10px] ${step === 1 ? "bg-primary text-primary-foreground" : "bg-primary/10"}`}>1</span>
            {t("signup.step1", "Compte")}
          </div>
          <div className="h-px flex-1 bg-border" />
          <div className={`flex items-center gap-1.5 text-xs ${step === 2 ? "text-primary font-medium" : "text-muted-foreground"}`}>
            <span className={`grid size-5 place-items-center rounded-full text-[10px] ${step === 2 ? "bg-primary text-primary-foreground" : "bg-primary/10"}`}>2</span>
            {t("signup.step2", "Profil")}
          </div>
        </div>

        {/* ─── ÉTAPE 1 : COMPTE ─── */}
        {step === 1 && (
          <motion.form
            ref={scope}
            variants={stagger}
            initial="hidden"
            animate="show"
            onSubmit={(e) => {
              e.preventDefault();
              if (canSubmitStep1) setStep(2);
            }}
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

            <motion.div variants={rise} className="rounded-lg border border-primary/20 bg-primary/5 px-3 py-2.5 text-xs leading-relaxed text-muted-foreground">
              <ShieldCheck className="mr-1.5 inline size-3.5 text-primary" />
              {t(
                "signup.trialNote",
                "Votre essai de 90 jours démarre immédiatement. 1 routeur inclus, sans carte bancaire.",
              )}
            </motion.div>

            <motion.div variants={rise} whileHover={{ scale: canSubmitStep1 ? 1.015 : 1 }} whileTap={{ scale: canSubmitStep1 ? 0.97 : 1 }}>
              <Button type="submit" className="w-full shadow-lg shadow-primary/25" disabled={!canSubmitStep1}>
                {t("signup.continue", "Continuer")}
                <ArrowRight className="size-4" />
              </Button>
            </motion.div>
          </motion.form>
        )}

        {/* ─── ÉTAPE 2 : PROFIL ─── */}
        {step === 2 && (
          <motion.form
            variants={stagger}
            initial="hidden"
            animate="show"
            onSubmit={handleSubmit}
            className="space-y-4 pt-2"
          >
            <motion.div variants={rise} className="space-y-2">
              <Label htmlFor="signup-email">{t("signup.email", "Email")}</Label>
              <Input
                id="signup-email"
                type="email"
                autoComplete="email"
                placeholder="gerant@promax.ci"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                disabled={loading}
              />
              {email && !emailValid && (
                <p className="text-xs text-destructive">{t("signup.emailInvalid", "Format d'email invalide")}</p>
              )}
            </motion.div>

            <motion.div variants={rise} className="space-y-2">
              <Label htmlFor="signup-phone">{t("signup.phone", "Numéro WhatsApp")}</Label>
              <div className="flex gap-2">
                {dialCode && (
                  <div className="flex items-center gap-1 rounded-md border border-input bg-muted px-3 text-sm text-muted-foreground">
                    <span className="text-xs">+{dialCode}</span>
                  </div>
                )}
                <Input
                  id="signup-phone"
                  type="tel"
                  inputMode="tel"
                  autoComplete="tel-national"
                  placeholder="07 00 00 00 00"
                  className="flex-1"
                  value={phone}
                  onChange={(e) => setPhone(e.target.value)}
                  disabled={loading}
                />
              </div>
              {phone && !phoneValid && (
                <p className="text-xs text-destructive">
                  {t("signup.phoneInvalid", "Numéro invalide (8 à 15 chiffres)")}
                </p>
              )}
              <p className="text-xs text-muted-foreground">
                {t("signup.phoneHint", "Pour le support WhatsApp — nous ne partageons jamais ce numéro")}
              </p>
            </motion.div>

            <motion.div variants={rise} className="space-y-2">
              <Label htmlFor="signup-country">{t("signup.country", "Pays")}</Label>
              <Select value={country} onValueChange={setCountry} disabled={loading}>
                <SelectTrigger id="signup-country">
                  <SelectValue placeholder={t("signup.countryPlaceholder", "Sélectionnez votre pays")} />
                </SelectTrigger>
                <SelectContent className="max-h-72">
                  {countries.map((c) => (
                    <SelectItem key={c.code} value={c.code}>
                      <span className="flex items-center gap-2">
                        <MapPin className="size-3.5 text-muted-foreground" />
                        {c[lang]}
                      </span>
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </motion.div>

            <motion.div variants={rise} className="space-y-2">
              <Label htmlFor="signup-city">{t("signup.city", "Ville d'activité")}</Label>
              <Input
                id="signup-city"
                autoComplete="address-level2"
                placeholder="Abidjan"
                value={city}
                onChange={(e) => setCity(e.target.value)}
                disabled={loading}
              />
            </motion.div>

            <motion.div variants={rise} className="flex items-center gap-2 rounded-lg border border-primary/20 bg-primary/5 px-3 py-2.5 text-xs leading-relaxed text-muted-foreground">
              <User className="mr-1.5 inline size-3.5 text-primary" />
              {t(
                "signup.profileNote",
                "Ces informations restent confidentielles et servent au support et à la segmentation.",
              )}
            </motion.div>

            <motion.div variants={rise} className="flex gap-2">
              <Button
                type="button"
                variant="outline"
                className="flex-1"
                onClick={() => setStep(1)}
                disabled={loading}
              >
                <ArrowLeft className="size-4" />
                {t("signup.back", "Retour")}
              </Button>
              <Button
                type="submit"
                className="flex-1 shadow-lg shadow-primary/25"
                disabled={!canSubmitStep2 || loading}
              >
                {loading && <Loader2 className="size-4 animate-spin" />}
                {!loading && <Check className="size-4" />}
                {t("signup.submit", "Créer mon compte")}
              </Button>
            </motion.div>
          </motion.form>
        )}
      </DialogContent>
    </Dialog>
  );
}
