"use client";

// N°27 — formulaire public d'inscription (page /join/[token], atteinte en
// scannant le QR code du gérant). Mobile-first absolu : cibles tactiles
// ≥ 44 px, labels liés, erreurs inline (zod, messages i18n), honeypot
// « website » (invisible pour l'humain — les bots qui le remplissent
// reçoivent un succès factice côté backend). Aucune authentification : le
// token du lien fait l'accès.

import { useState, type FormEvent } from "react";
import { Eye, EyeOff, Info, KeyRound, Loader2, UserPlus, Users, Zap } from "lucide-react";
import { z } from "zod";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { ApiError, api } from "@/lib/hotspot/api";
import { useI18n } from "@/lib/hotspot/i18n";
import type { JoinLinkState } from "@/lib/hotspot/types";

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

interface JoinFormProps {
  token: string;
  link: JoinLinkInfo;
  onOutcome: (outcome: SubmitOutcome) => void;
  /** 409 join_link_closed — la page refait le GET et affiche l'état verrouillé réel. */
  onLinkClosed: () => void;
}

export default function JoinForm({ token, link, onOutcome, onLinkClosed }: JoinFormProps) {
  const { t, tf } = useI18n();

  const [fullName, setFullName] = useState("");
  const [phone, setPhone] = useState("");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [message, setMessage] = useState("");
  const [website, setWebsite] = useState(""); // honeypot — jamais affiché
  const [showPassword, setShowPassword] = useState(false);

  const [submitting, setSubmitting] = useState(false);
  const [fieldErrors, setFieldErrors] = useState<Record<string, string | undefined>>({});
  const [formError, setFormError] = useState<string | null>(null);
  const [suggestion, setSuggestion] = useState<string | null>(null);

  /** Schéma zod reconstruit à la validation : les messages dépendent de la langue. */
  function validate() {
    const schema = z
      .object({
        fullName: z.string().trim().min(2, t("joinPage.err.fullName")).max(80, t("joinPage.err.fullName")),
        // « + » et espaces acceptés tels quels — le serveur normalise ; on ne
        // compte que les chiffres (8 à 15), même contrat que le backend.
        phone: z.string().refine((v) => {
          const digits = v.replace(/\D/g, "");
          return digits.length >= 8 && digits.length <= 15;
        }, t("joinPage.err.phone")),
        username: z.string().trim().regex(USERNAME_RE, t("joinPage.err.username")),
        password: z.string().min(6, t("joinPage.err.password")).max(64, t("joinPage.err.password")),
        confirmPassword: z.string(),
        message: z.string().max(300, t("joinPage.err.message")),
      })
      .refine((d) => d.password === d.confirmPassword, {
        message: t("joinPage.err.confirm"),
        path: ["confirmPassword"],
      });
    return schema.safeParse({ fullName, phone, username, password, confirmPassword, message });
  }

  /** Met à jour un champ et efface son erreur inline (retour immédiat). */
  function update(setter: (v: string) => void, field: string, value: string) {
    setter(value);
    setFieldErrors((prev) => (prev[field] ? { ...prev, [field]: undefined } : prev));
    if (field === "username") setSuggestion(null);
    setFormError(null);
  }

  async function handleSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (submitting) return;
    setFormError(null);

    const parsed = validate();
    if (!parsed.success) {
      const next: Record<string, string | undefined> = {};
      for (const issue of parsed.error.issues) {
        const key = String(issue.path[0] ?? "");
        if (!next[key]) next[key] = issue.message;
      }
      setFieldErrors(next);
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
          message: parsed.data.message.trim(),
          website,
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
  const confirmErr = fieldErrors.confirmPassword;
  const messageErr = fieldErrors.message;

  return (
    <Card className="gap-5">
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
        {/* Mode de connexion (distinct des tickets/vouchers) + mention visible */}
        <div className="flex flex-col gap-2">
          <p className="flex items-center gap-1.5 rounded-lg bg-muted/60 px-3 py-2 text-xs font-medium">
            <KeyRound className="size-3.5 shrink-0 text-chart-2" aria-hidden />
            {t("join.modeLabel")}
          </p>
          <p className="flex items-start gap-2 text-sm text-muted-foreground">
            <Info className="mt-0.5 size-4 shrink-0" aria-hidden />
            {t("joinPage.notice")}
          </p>
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
            <Label htmlFor="join-fullName">{t("joinPage.fullName")}</Label>
            <Input
              id="join-fullName"
              className="h-11"
              autoComplete="name"
              maxLength={80}
              placeholder={t("joinPage.fullNamePlaceholder")}
              value={fullName}
              onChange={(e) => update(setFullName, "fullName", e.target.value)}
              disabled={submitting}
              aria-invalid={Boolean(fullNameErr)}
              aria-describedby={fullNameErr ? "join-err-fullName" : undefined}
            />
            {fullNameErr && (
              <p id="join-err-fullName" role="alert" className="text-xs text-destructive">
                {fullNameErr}
              </p>
            )}
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="join-phone">{t("joinPage.phone")}</Label>
            <Input
              id="join-phone"
              className="h-11"
              inputMode="tel"
              autoComplete="tel"
              maxLength={20}
              placeholder={t("joinPage.phonePlaceholder")}
              value={phone}
              onChange={(e) => update(setPhone, "phone", e.target.value)}
              disabled={submitting}
              aria-invalid={Boolean(phoneErr)}
              aria-describedby={phoneErr ? "join-err-phone" : undefined}
            />
            {phoneErr && (
              <p id="join-err-phone" role="alert" className="text-xs text-destructive">
                {phoneErr}
              </p>
            )}
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="join-username">{t("joinPage.username")}</Label>
            <Input
              id="join-username"
              className="h-11"
              autoCapitalize="none"
              autoCorrect="off"
              spellCheck={false}
              autoComplete="username"
              maxLength={32}
              placeholder={t("joinPage.usernamePlaceholder")}
              value={username}
              onChange={(e) => update(setUsername, "username", e.target.value.replace(/\s+/g, ""))}
              disabled={submitting}
              aria-invalid={Boolean(usernameErr)}
              aria-describedby={usernameErr ? "join-username-hint join-err-username" : "join-username-hint"}
            />
            <p id="join-username-hint" className="text-xs text-muted-foreground">
              {t("joinPage.usernameHint")}
            </p>
            {usernameErr && (
              <p id="join-err-username" role="alert" className="text-xs text-destructive">
                {usernameErr}
              </p>
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
            <Label htmlFor="join-password">{t("joinPage.password")}</Label>
            <div className="relative">
              <Input
                id="join-password"
                type={showPassword ? "text" : "password"}
                className="h-11 pr-12"
                autoComplete="new-password"
                maxLength={64}
                placeholder={t("joinPage.passwordPlaceholder")}
                value={password}
                onChange={(e) => update(setPassword, "password", e.target.value)}
                disabled={submitting}
                aria-invalid={Boolean(passwordErr)}
                aria-describedby={passwordErr ? "join-err-password" : undefined}
              />
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
            {passwordErr && (
              <p id="join-err-password" role="alert" className="text-xs text-destructive">
                {passwordErr}
              </p>
            )}
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="join-confirmPassword">{t("joinPage.confirmPassword")}</Label>
            <Input
              id="join-confirmPassword"
              type={showPassword ? "text" : "password"}
              className="h-11"
              autoComplete="new-password"
              maxLength={64}
              placeholder={t("joinPage.confirmPasswordPlaceholder")}
              value={confirmPassword}
              onChange={(e) => update(setConfirmPassword, "confirmPassword", e.target.value)}
              disabled={submitting}
              aria-invalid={Boolean(confirmErr)}
              aria-describedby={confirmErr ? "join-err-confirmPassword" : undefined}
            />
            {confirmErr && (
              <p id="join-err-confirmPassword" role="alert" className="text-xs text-destructive">
                {confirmErr}
              </p>
            )}
          </div>

          <div className="space-y-1.5">
            <div className="flex items-baseline justify-between gap-2">
              <Label htmlFor="join-message">{t("joinPage.message")}</Label>
              <span className="text-[11px] tabular-nums text-muted-foreground">
                {tf("joinPage.messageCounter", { count: message.length })}
              </span>
            </div>
            <Textarea
              id="join-message"
              className="min-h-[88px]"
              maxLength={300}
              placeholder={t("joinPage.messagePlaceholder")}
              value={message}
              onChange={(e) => update(setMessage, "message", e.target.value)}
              disabled={submitting}
              aria-invalid={Boolean(messageErr)}
              aria-describedby={messageErr ? "join-err-message" : undefined}
            />
            {messageErr && (
              <p id="join-err-message" role="alert" className="text-xs text-destructive">
                {messageErr}
              </p>
            )}
          </div>

          <Button type="submit" disabled={submitting} className="h-11 w-full text-base font-semibold shadow-lg shadow-primary/25">
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
