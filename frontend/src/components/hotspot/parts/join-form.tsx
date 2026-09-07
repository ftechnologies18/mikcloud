"use client";

// N°62 — formulaire public d'inscription (page /join/[token], atteinte en
// scannant le QR code du gérant) — version SIMPLIFIÉE : 4 champs seulement.
// La confirmation de mot de passe et le message facultatif sont supprimés
// (le backend n'a jamais exigé l'un ni l'autre : le message y est optionnel,
// la confirmation n'existait que côté client). Compensations INTERACTIVES :
// jauge de robustesse + bouton « copier » à la place de la confirmation,
// validation live au blur, barre de progression, coche verte par champ
// valide, focus automatique sur la première erreur à la soumission.
// Mobile-first absolu : cibles tactiles ≥ 44 px, labels liés, erreurs
// inline (zod, messages i18n), honeypot « website » (invisible pour
// l'humain — les bots qui le remplissent reçoivent un succès factice côté
// backend). Aucune authentification : le token du lien fait l'accès.

import { useEffect, useRef, useState, type FormEvent } from "react";
import { motion } from "framer-motion";
import {
  AtSign,
  Check,
  CheckCircle2,
  Copy,
  Eye,
  EyeOff,
  KeyRound,
  Loader2,
  Phone,
  User,
  UserPlus,
  Users,
  Zap,
} from "lucide-react";
import { toast } from "sonner";
import { z } from "zod";

import { copyToClipboard } from "@/components/hotspot/parts/uc-clipboard";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ApiError, api } from "@/lib/hotspot/api";
import { useI18n } from "@/lib/hotspot/i18n";
import type { JoinLinkState } from "@/lib/hotspot/types";
import { cn } from "@/lib/utils";

/** Réponse du GET public /api/join/{token} (handleJoinInfo, backend N°27) :
 * données minimales du lien — JAMAIS le catalogue de profils. */
export interface JoinLinkInfo {
  name: string;
  organization: string;
  state: JoinLinkState;
  expiresAt?: string;
  remaining?: number;
  autoValidate?: boolean;
  profileName?: string;
}

/** Issue d'une soumission POST — la page affiche l'écran correspondant. */
export type SubmitOutcome =
  | { kind: "pending"; username: string; password: string }
  | { kind: "approved"; username: string; password: string; queued: boolean }
  | { kind: "rateLimited" }
  | { kind: "network" };

/** Mêmes règles que le backend : 3–32 caractères [A-Za-z0-9._-]. */
const USERNAME_RE = /^[A-Za-z0-9._-]{3,32}$/;

type FieldKey = "fullName" | "phone" | "username" | "password";

interface JoinFormProps {
  token: string;
  link: JoinLinkInfo;
  onOutcome: (outcome: SubmitOutcome) => void;
  /** 409 join_link_closed — la page refait le GET et affiche l'état verrouillé réel. */
  onLinkClosed: () => void;
  /** N°33 — MAC de l'appareil (page login du routeur, ?mac=), anti-abus par
   * appareil derrière le NAT du hotspot. Optionnelle : jamais bloquante. */
  mac?: string;
}

export default function JoinForm({ token, link, onOutcome, onLinkClosed, mac }: JoinFormProps) {
  const { t, tf } = useI18n();

  const [fullName, setFullName] = useState("");
  const [phone, setPhone] = useState("");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [website, setWebsite] = useState(""); // honeypot — jamais affiché
  const [showPassword, setShowPassword] = useState(false);
  const [pwCopied, setPwCopied] = useState(false);

  const [submitting, setSubmitting] = useState(false);
  const [fieldErrors, setFieldErrors] = useState<Record<string, string | undefined>>({});
  const [formError, setFormError] = useState<string | null>(null);
  const [suggestion, setSuggestion] = useState<string | null>(null);

  // Retour visuel transitoire du bouton « copier » — minuteur nettoyé au
  // démontage (le formulaire bascule vers l'écran de succès).
  const pwCopyTimer = useRef<number | null>(null);
  useEffect(() => {
    return () => {
      if (pwCopyTimer.current) window.clearTimeout(pwCopyTimer.current);
    };
  }, []);

  /** Schéma zod reconstruit à la validation : les messages dépendent de la langue. */
  function buildSchema() {
    return z.object({
      fullName: z.string().trim().min(2, t("joinPage.err.fullName")).max(80, t("joinPage.err.fullName")),
      // « + » et espaces acceptés tels quels — le serveur normalise ; on ne
      // compte que les chiffres (8 à 15), même contrat que le backend.
      phone: z.string().refine((v) => {
        const digits = v.replace(/\D/g, "");
        return digits.length >= 8 && digits.length <= 15;
      }, t("joinPage.err.phone")),
      username: z.string().trim().regex(USERNAME_RE, t("joinPage.err.username")),
      // N°33 — même plancher que le backend : 8 caractères (denylist et
      // interdiction « = nom d'utilisateur » appliquées côté serveur).
      password: z.string().min(8, t("joinPage.err.password")).max(64, t("joinPage.err.password")),
    });
  }

  /** Validité LIVE de chaque champ (barre de progression + coche) — mêmes
   * règles que le schéma, sans messages : simple retour visuel. */
  const fullNameOk = fullName.trim().length >= 2 && fullName.trim().length <= 80;
  const phoneOk = phone.replace(/\D/g, "").length >= 8 && phone.replace(/\D/g, "").length <= 15;
  const usernameOk = USERNAME_RE.test(username);
  const passwordOk = password.length >= 8 && password.length <= 64;
  const validCount = [fullNameOk, phoneOk, usernameOk, passwordOk].filter(Boolean).length;

  /** Robustesse INDICATIVE du mot de passe (0 = sous le plancher de 8) :
   * longueur + variété de classes. Retour UX uniquement — la politique
   * réelle (plancher, denylist, ≠ nom d'utilisateur) reste côté serveur. */
  function passwordStrength(pw: string): 0 | 1 | 2 | 3 | 4 {
    if (!pw || pw.length < 8) return 0;
    const classes = [/[a-z]/, /[A-Z]/, /[0-9]/, /[^A-Za-z0-9]/].filter((re) => re.test(pw)).length;
    if (pw.length >= 16 && classes >= 3) return 4;
    if (pw.length >= 12 && classes >= 3) return 3;
    if (pw.length >= 10 && classes >= 2) return 2;
    return 1;
  }
  const strength = passwordStrength(password);
  const STRENGTH_META: Record<number, { label: string; fill: string; text: string }> = {
    1: { label: t("joinPage.strength.1"), fill: "bg-destructive", text: "text-destructive" },
    2: { label: t("joinPage.strength.2"), fill: "bg-chart-3", text: "text-chart-3" },
    3: { label: t("joinPage.strength.3"), fill: "bg-chart-2", text: "text-chart-2" },
    4: { label: t("joinPage.strength.4"), fill: "bg-primary", text: "text-primary" },
  };

  /** Met à jour un champ et efface son erreur inline (retour immédiat). */
  function update(setter: (v: string) => void, field: string, value: string) {
    setter(value);
    setFieldErrors((prev) => (prev[field] ? { ...prev, [field]: undefined } : prev));
    if (field === "username") setSuggestion(null);
    setFormError(null);
  }

  /** Validation LIVE au blur (champ non vide) : même schéma zod que la
   * soumission — l'erreur du champ apparaît/disparaît en direct, sans
   * attendre le bouton « Envoyer ». Champ vide : silencieux (pas d'erreur
   * agressive en parcours tabulation). */
  function blurValidate(field: FieldKey) {
    const value = { fullName, phone, username, password }[field];
    if (!value) return;
    const parsed = buildSchema().safeParse({ fullName, phone, username, password });
    const issue = parsed.success
      ? undefined
      : parsed.error.issues.find((i) => String(i.path[0] ?? "") === field);
    setFieldErrors((prev) => ({ ...prev, [field]: issue?.message }));
  }

  /** Copie le mot de passe (compense la confirmation supprimée : l'utilsateur
   * peut le sauvegarder immédiatement) + retour icône/toast transitoire. */
  async function copyPassword() {
    if (!password || submitting) return;
    const ok = await copyToClipboard(password);
    if (!ok) return;
    toast.success(t("joinPage.passwordCopied"));
    setPwCopied(true);
    if (pwCopyTimer.current) window.clearTimeout(pwCopyTimer.current);
    pwCopyTimer.current = window.setTimeout(() => {
      setPwCopied(false);
      pwCopyTimer.current = null;
    }, 1500);
  }

  async function handleSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (submitting) return;
    setFormError(null);

    const parsed = buildSchema().safeParse({ fullName, phone, username, password });
    if (!parsed.success) {
      const next: Record<string, string | undefined> = {};
      for (const issue of parsed.error.issues) {
        const key = String(issue.path[0] ?? "");
        if (!next[key]) next[key] = issue.message;
      }
      setFieldErrors(next);
      // A11y mobile : focus + scroll sur la PREMIÈRE erreur — pas de chasse
      // à l'erreur à l'aveugle dans un écran de formulaire.
      for (const key of ["fullName", "phone", "username", "password"] as FieldKey[]) {
        if (next[key]) {
          document.getElementById(`join-${key}`)?.focus();
          document.getElementById(`join-${key}`)?.scrollIntoView({ block: "center", behavior: "smooth" });
          break;
        }
      }
      return;
    }
    setFieldErrors({});
    setSubmitting(true);
    try {
      const res = await api<{
        status: "pending" | "approved";
        username?: string;
        password?: string;
        queued?: boolean;
      }>(`/api/join/${token}`, {
        method: "POST",
        body: {
          fullName: parsed.data.fullName,
          phone: parsed.data.phone,
          username: parsed.data.username,
          password: parsed.data.password,
          website,
          mac: mac || undefined,
        },
      });
      if (res.status === "approved") {
        // Lien kiosque : identifiants RÉELS renvoyés par le serveur.
        onOutcome({
          kind: "approved",
          username: res.username ?? parsed.data.username,
          password: res.password ?? parsed.data.password,
          queued: Boolean(res.queued),
        });
      } else {
        onOutcome({ kind: "pending", username: parsed.data.username, password: parsed.data.password });
      }
    } catch (err) {
      if (err instanceof ApiError) {
        if (err.code === "username_taken") {
          setFieldErrors((prev) => ({ ...prev, username: t("joinPage.err.usernameTaken") }));
          if (err.suggestion) setSuggestion(err.suggestion);
        } else if (err.code === "phone_pending") {
          setFieldErrors((prev) => ({ ...prev, phone: t("joinPage.err.phonePending") }));
        } else if (err.code === "join_link_closed") {
          // Le lien vient d'être fermé/révoqué/saturé : la page refait le GET
          // et basculera sur la carte d'état verrouillé correspondante.
          onLinkClosed();
        } else if (err.status === 429) {
          onOutcome({ kind: "rateLimited" });
        } else {
          // 400 (règles serveur) et autres : message inline générique.
          setFormError(err.message);
        }
      } else {
        onOutcome({ kind: "network" });
      }
    } finally {
      setSubmitting(false);
    }
  }

  const fullNameErr = fieldErrors.fullName;
  const phoneErr = fieldErrors.phone;
  const usernameErr = fieldErrors.username;
  const passwordErr = fieldErrors.password;

  return (
    <Card className="gap-5 shadow-lg shadow-black/5">
      {/* En-tête du lien : organisation (prioritaire), nom du lien, badges */}
      <CardHeader className="gap-1.5">
        <CardTitle className="text-lg font-semibold break-words">{link.organization || link.name}</CardTitle>
        {link.organization && link.organization !== link.name && (
          <CardDescription className="text-sm break-words">{link.name}</CardDescription>
        )}
        <div className="mt-1 flex flex-wrap items-center gap-2">
          {typeof link.remaining === "number" && (
            <Badge variant="outline" className="gap-1 bg-background/60 px-2 font-normal text-muted-foreground">
              <Users className="size-3" aria-hidden />
              {tf("joinPage.remainingLeft", { count: link.remaining })}
            </Badge>
          )}
          {link.autoValidate && (
            <Badge className="gap-1 border-transparent bg-chart-2/15 px-2 text-chart-2 hover:bg-chart-2/25">
              <Zap className="size-3" aria-hidden />
              {t("joinPage.autoValidate")}
            </Badge>
          )}
          {link.autoValidate && link.profileName && (
            <span className="text-xs text-muted-foreground">
              {tf("joinPage.autoValidateProfile", { name: link.profileName })}
            </span>
          )}
        </div>
      </CardHeader>

      <CardContent className="flex flex-col gap-5">
        {/* Progression du formulaire (retour live : 0 → 4 champs valides). */}
        <div
          role="progressbar"
          aria-valuemin={0}
          aria-valuemax={4}
          aria-valuenow={validCount}
          aria-label={tf("joinPage.progressAria", { count: validCount })}
          className="h-1.5 w-full overflow-hidden rounded-full bg-muted"
        >
          <div
            className="h-full rounded-full bg-chart-2 transition-[width] duration-500 ease-out"
            style={{ width: `${(validCount / 4) * 100}%` }}
          />
        </div>

        {/* Mode de connexion (distinct des tickets/vouchers) + notice de
            validation, fusionnés en un seul bloc compact. */}
        <div className="flex items-start gap-2.5 rounded-xl border border-chart-2/25 bg-chart-2/10 px-3.5 py-3">
          <KeyRound className="mt-0.5 size-4 shrink-0 text-chart-2" aria-hidden />
          <div className="min-w-0">
            <p className="text-xs font-semibold leading-snug">{t("join.modeLabel")}</p>
            <p className="mt-0.5 text-xs leading-relaxed text-muted-foreground">{t("joinPage.notice")}</p>
          </div>
        </div>

        <form onSubmit={handleSubmit} noValidate className="flex flex-col gap-4">
          {/* 400 / erreurs serveur génériques — inline au-dessus des champs */}
          {formError && (
            <div role="alert" className="rounded-lg border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive">
              {formError}
            </div>
          )}

          {/* HONEYPOT — champ invisible pour l'humain. type="text" + .hidden
              (PAS type="hidden") : les bots le remplissent, le backend répond
              alors un succès factice sans rien créer. Jamais rendu à l'écran. */}
          <input
            type="text"
            name="website"
            className="hidden"
            tabIndex={-1}
            autoComplete="off"
            aria-hidden="true"
            value={website}
            onChange={(e) => setWebsite(e.target.value)}
          />

          <div className="space-y-1.5">
            <div className="flex items-center justify-between gap-2">
              <Label htmlFor="join-fullName">{t("joinPage.fullName")}</Label>
              <FieldCheck ok={fullNameOk} />
            </div>
            <div className="relative">
              <User className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground/70" aria-hidden />
              <Input
                id="join-fullName"
                className="h-12 pl-10"
                autoComplete="name"
                enterKeyHint="next"
                maxLength={80}
                placeholder={t("joinPage.fullNamePlaceholder")}
                value={fullName}
                onChange={(e) => update(setFullName, "fullName", e.target.value)}
                onBlur={() => blurValidate("fullName")}
                disabled={submitting}
                aria-invalid={Boolean(fullNameErr)}
                aria-describedby={fullNameErr ? "join-err-fullName" : undefined}
              />
            </div>
            {fullNameErr && (
              <ErrorText id="join-err-fullName">{fullNameErr}</ErrorText>
            )}
          </div>

          <div className="space-y-1.5">
            <div className="flex items-center justify-between gap-2">
              <Label htmlFor="join-phone">{t("joinPage.phone")}</Label>
              <FieldCheck ok={phoneOk} />
            </div>
            <div className="relative">
              <Phone className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground/70" aria-hidden />
              <Input
                id="join-phone"
                className="h-12 pl-10"
                inputMode="tel"
                autoComplete="tel"
                enterKeyHint="next"
                maxLength={20}
                placeholder={t("joinPage.phonePlaceholder")}
                value={phone}
                onChange={(e) => update(setPhone, "phone", e.target.value)}
                onBlur={() => blurValidate("phone")}
                disabled={submitting}
                aria-invalid={Boolean(phoneErr)}
                aria-describedby={phoneErr ? "join-err-phone" : undefined}
              />
            </div>
            {phoneErr && (
              <ErrorText id="join-err-phone">{phoneErr}</ErrorText>
            )}
          </div>

          <div className="space-y-1.5">
            <div className="flex items-center justify-between gap-2">
              <Label htmlFor="join-username">{t("joinPage.username")}</Label>
              <FieldCheck ok={usernameOk} />
            </div>
            <div className="relative">
              <AtSign className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground/70" aria-hidden />
              <Input
                id="join-username"
                className="h-12 pl-10"
                autoCapitalize="none"
                autoCorrect="off"
                spellCheck={false}
                autoComplete="username"
                enterKeyHint="next"
                maxLength={32}
                placeholder={t("joinPage.usernamePlaceholder")}
                value={username}
                onChange={(e) => update(setUsername, "username", e.target.value.replace(/\s+/g, ""))}
                onBlur={() => blurValidate("username")}
                disabled={submitting}
                aria-invalid={Boolean(usernameErr)}
                aria-describedby={usernameErr ? "join-username-hint join-err-username" : "join-username-hint"}
              />
            </div>
            <p id="join-username-hint" className="text-xs text-muted-foreground">
              {t("joinPage.usernameHint")}
            </p>
            {usernameErr && (
              <ErrorText id="join-err-username">{usernameErr}</ErrorText>
            )}
            {usernameErr && suggestion && (
              <Button
                type="button"
                variant="outline"
                size="sm"
                className="h-9 w-fit"
                onClick={() => {
                  setUsername(suggestion);
                  setFieldErrors((prev) => ({ ...prev, username: undefined }));
                  setSuggestion(null);
                }}
                disabled={submitting}
              >
                {tf("joinPage.useSuggestion", { suggestion })}
              </Button>
            )}
          </div>

          <div className="space-y-1.5">
            <div className="flex items-center justify-between gap-2">
              <Label htmlFor="join-password">{t("joinPage.password")}</Label>
              <FieldCheck ok={passwordOk} />
            </div>
            <div className="relative">
              <KeyRound className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground/70" aria-hidden />
              <Input
                id="join-password"
                type={showPassword ? "text" : "password"}
                className="h-12 pl-10 pr-24"
                autoComplete="new-password"
                enterKeyHint="go"
                maxLength={64}
                placeholder={t("joinPage.passwordPlaceholder")}
                value={password}
                onChange={(e) => update(setPassword, "password", e.target.value)}
                disabled={submitting}
                aria-invalid={Boolean(passwordErr)}
                aria-describedby={passwordErr ? "join-err-password" : undefined}
              />
              {/* Copier (compense la confirmation supprimée : sauvegarder le
                  mot de passe dès sa saisie) — visible dès qu'il existe. */}
              {password && (
                <button
                  type="button"
                  onClick={() => void copyPassword()}
                  aria-label={t("joinPage.copyPasswordAria")}
                  disabled={submitting}
                  className="absolute right-12 top-1/2 inline-flex size-11 -translate-y-1/2 items-center justify-center rounded-md text-muted-foreground transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50 disabled:pointer-events-none"
                >
                  {pwCopied ? <Check className="size-4 text-chart-2" aria-hidden /> : <Copy className="size-4" aria-hidden />}
                </button>
              )}
              <button
                type="button"
                onClick={() => setShowPassword((v) => !v)}
                aria-label={t(showPassword ? "join.hidePassword" : "join.showPassword")}
                aria-pressed={showPassword}
                className="absolute right-1 top-1/2 inline-flex size-11 -translate-y-1/2 items-center justify-center rounded-md text-muted-foreground transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50"
              >
                {showPassword ? <EyeOff className="size-4" aria-hidden /> : <Eye className="size-4" aria-hidden />}
              </button>
            </div>
            {/* Jauge de robustesse — purement indicative, la politique réelle
                reste côté serveur ; « trop court » sous le plancher de 8. */}
            {password && (
              <div className="flex items-center gap-2 pt-0.5">
                <div className="flex min-w-0 flex-1 gap-1" aria-hidden="true">
                  {[1, 2, 3, 4].map((i) => (
                    <span
                      key={i}
                      className={cn(
                        "h-1.5 flex-1 rounded-full transition-colors duration-300",
                        i <= strength ? STRENGTH_META[strength].fill : "bg-muted",
                      )}
                    />
                  ))}
                </div>
                <span
                  className={cn(
                    "shrink-0 text-[11px] font-medium tabular-nums",
                    strength === 0 ? "text-destructive" : STRENGTH_META[strength].text,
                  )}
                  aria-live="polite"
                >
                  {strength === 0 ? t("joinPage.strength.tooShort") : STRENGTH_META[strength].label}
                </span>
              </div>
            )}
            {passwordErr && (
              <ErrorText id="join-err-password">{passwordErr}</ErrorText>
            )}
          </div>

          <Button
            type="submit"
            disabled={submitting}
            className="h-12 w-full text-base font-semibold shadow-lg shadow-primary/25 transition-transform active:scale-[0.98]"
          >
            {submitting ? (
              <>
                <Loader2 className="size-4 animate-spin" aria-hidden />
                {t("joinPage.submitting")}
              </>
            ) : (
              <>
                <UserPlus className="size-4" aria-hidden />
                {t("joinPage.submit")}
              </>
            )}
          </Button>
        </form>
      </CardContent>
    </Card>
  );
}

/* ─── Micro-composants (retours visuels interactifs) ─── */

/** Coche verte discrète quand un champ est valide (retour live, décoratif —
 * la progression a déjà le rôle ARIA progressbar). */
function FieldCheck({ ok }: { ok: boolean }) {
  if (!ok) return null;
  return <CheckCircle2 className="size-4 shrink-0 text-chart-2" aria-hidden />;
}

/** Message d'erreur inline animé (apparition douce, ~180 ms). */
function ErrorText({ id, children }: { id: string; children: string }) {
  return (
    <motion.p
      id={id}
      role="alert"
      initial={{ opacity: 0, y: -3 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.18, ease: "easeOut" }}
      className="text-xs text-destructive"
    >
      {children}
    </motion.p>
  );
}
