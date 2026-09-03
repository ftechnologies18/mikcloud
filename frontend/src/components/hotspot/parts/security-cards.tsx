"use client";

// Cartes de SÉCURITÉ PERSONNELLE partagées (fusion K — anti-redondance) :
// changement de mot de passe et 2FA TOTP. Une seule implémentation, utilisée
// par les DEUX vues de réglages — la vue client (settings-view) et la console
// plateforme (platform-settings-view, où le guard empêche d'accéder à la vue
// client). Le mot de passe de la plateforme utilise la version RICHE
// (bascule de visibilité) ; l'ancienne version plate de la console a été
// retirée. La langue, elle, vit uniquement dans le menu utilisateur + l'onglet
// Général du client — plus de carte dupliquée côté plateforme.

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Copy, Eye, EyeOff, KeyRound, Loader2, Smartphone } from "lucide-react";
import { toast } from "sonner";

import { api } from "@/lib/hotspot/api";
import { useI18n } from "@/lib/hotspot/i18n";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

// Champ mot de passe avec bascule de visibilité.
export function PasswordInput({
  id,
  value,
  onChange,
  placeholder,
  autoComplete,
}: {
  id: string;
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  autoComplete?: string;
}) {
  const { t } = useI18n();
  const [visible, setVisible] = useState(false);
  return (
    <div className="relative">
      <Input
        id={id}
        type={visible ? "text" : "password"}
        value={value}
        onChange={(event) => onChange(event.target.value)}
        placeholder={placeholder}
        autoComplete={autoComplete}
        className="h-10 pr-10"
      />
      <button
        type="button"
        onClick={() => setVisible((v) => !v)}
        aria-label={visible ? t("settings.passwordHide") : t("settings.passwordShow")}
        className="absolute right-1.5 top-1/2 flex size-7 -translate-y-1/2 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
      >
        {visible ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
      </button>
    </div>
  );
}

// Carte Sécurité — changement du mot de passe de connexion de l'utilisateur courant.
// Le backend exige le mot de passe actuel (une session ouverte ne suffit pas).
// descKey — description contextuelle (plateforme : « compte administrateur »).
export function SecurityCard({ descKey }: { descKey?: string }) {
  const { t } = useI18n();
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");

  const changeMutation = useMutation({
    mutationFn: () =>
      api<{ ok: boolean }>("/api/auth/password", {
        method: "POST",
        body: { currentPassword, newPassword },
      }),
    onSuccess: () => {
      toast.success(t("settings.passwordChangedToast"), {
        description: t("settings.passwordChangedToastDesc"),
      });
      setCurrentPassword("");
      setNewPassword("");
      setConfirmPassword("");
    },
    onError: (error: Error) => toast.error(error.message),
  });

  const canSubmit =
    currentPassword !== "" && newPassword !== "" && confirmPassword !== "" && !changeMutation.isPending;

  function submitPassword() {
    if (newPassword.length < 10) {
      toast.error(t("settings.passwordTooShort"));
      return;
    }
    if (newPassword === currentPassword) {
      toast.error(t("settings.passwordSame"));
      return;
    }
    if (newPassword !== confirmPassword) {
      toast.error(t("settings.passwordMismatch"));
      return;
    }
    changeMutation.mutate();
  }

  return (
    <Card className="gap-4 py-4 sm:py-6">
      <CardHeader className="px-4 sm:px-6">
        <CardTitle className="flex items-center gap-2 text-base">
          <span className="flex size-8 items-center justify-center rounded-lg bg-primary/15 text-primary">
            <KeyRound className="size-4" />
          </span>
          {t("settings.security")}
        </CardTitle>
        <CardDescription>{t(descKey ?? "settings.securityDesc")}</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-4 px-4 sm:px-6">
        <div className="grid gap-2">
          <Label htmlFor="pwd-current">{t("settings.currentPassword")}</Label>
          <PasswordInput
            id="pwd-current"
            value={currentPassword}
            onChange={setCurrentPassword}
            autoComplete="current-password"
          />
        </div>
        <div className="grid gap-2">
          <Label htmlFor="pwd-new">{t("settings.newPassword")}</Label>
          <PasswordInput
            id="pwd-new"
            value={newPassword}
            onChange={setNewPassword}
            autoComplete="new-password"
          />
          <p className="text-xs text-muted-foreground">{t("settings.passwordHint")}</p>
        </div>
        <div className="grid gap-2">
          <Label htmlFor="pwd-confirm">{t("settings.confirmPassword")}</Label>
          <PasswordInput
            id="pwd-confirm"
            value={confirmPassword}
            onChange={setConfirmPassword}
            autoComplete="new-password"
          />
        </div>
      </CardContent>
      <CardFooter className="px-4 sm:px-6">
        <Button className="h-10" onClick={submitPassword} disabled={!canSubmit}>
          {changeMutation.isPending ? t("settings.passwordChanging") : t("settings.passwordSubmit")}
        </Button>
      </CardFooter>
    </Card>
  );
}

// Carte 2FA TOTP (sécurité S4) — statut, activation en deux temps
// (setup → saisie du code de l'authenticator → activate) puis désactivation
// exigeant le mot de passe courant. Le secret n'est affiché QUE pendant le
// pairage ; le statut vient de GET /api/auth/me (champ totpEnabled).
export function TwoFactorCard() {
  const { t } = useI18n();
  const queryClient = useQueryClient();
  const meQuery = useQuery({
    queryKey: ["totp-status"],
    queryFn: () => api<{ user: { totpEnabled?: boolean } }>("/api/auth/me"),
  });
  const totpEnabled = meQuery.data?.user?.totpEnabled ?? false;

  const [pairing, setPairing] = useState(false);
  const [secret, setSecret] = useState("");
  const [otpauth, setOtpauth] = useState("");
  const [code, setCode] = useState("");
  const [disablePassword, setDisablePassword] = useState("");

  const setupMutation = useMutation({
    mutationFn: () =>
      api<{ secret: string; otpauth: string }>("/api/auth/2fa/setup", { method: "POST" }),
    onSuccess: (res) => {
      setSecret(res.secret);
      setOtpauth(res.otpauth);
      setPairing(true);
      setCode("");
    },
    onError: (error: Error) => toast.error(error.message),
  });

  const activateMutation = useMutation({
    mutationFn: () =>
      api<{ ok: boolean }>("/api/auth/2fa/activate", { method: "POST", body: { code } }),
    onSuccess: () => {
      toast.success(t("settings.totp.activatedToast"));
      setPairing(false);
      setSecret("");
      setOtpauth("");
      setCode("");
      queryClient.invalidateQueries({ queryKey: ["totp-status"] });
    },
    onError: (error: Error) => toast.error(error.message),
  });

  const disableMutation = useMutation({
    mutationFn: () =>
      api<{ ok: boolean }>("/api/auth/2fa/disable", { method: "POST", body: { password: disablePassword } }),
    onSuccess: () => {
      toast.success(t("settings.totp.disabledToast"));
      setDisablePassword("");
      queryClient.invalidateQueries({ queryKey: ["totp-status"] });
    },
    onError: (error: Error) => toast.error(error.message),
  });

  return (
    <Card className="gap-4 py-4 sm:py-6">
      <CardHeader className="px-4 sm:px-6">
        <CardTitle className="flex items-center gap-2 text-base">
          <span className="flex size-8 items-center justify-center rounded-lg bg-primary/15 text-primary">
            <Smartphone className="size-4" />
          </span>
          {t("settings.totp.title")}
          <Badge variant={totpEnabled ? "default" : "secondary"} className="ml-auto">
            {totpEnabled ? t("settings.totp.statusOn") : t("settings.totp.statusOff")}
          </Badge>
        </CardTitle>
        <CardDescription>{t("settings.totp.desc")}</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-4 px-4 sm:px-6">
        {pairing && !totpEnabled && (
          <div className="grid gap-3 rounded-lg border border-dashed p-4">
            <p className="text-sm font-medium">{t("settings.totp.pairTitle")}</p>
            <p className="text-xs text-muted-foreground">{t("settings.totp.pairHint")}</p>
            <div className="grid gap-1.5">
              <Label htmlFor="totp-secret">{t("settings.totp.secretLabel")}</Label>
              <div className="flex items-center gap-2">
                <Input id="totp-secret" readOnly value={secret} className="font-mono text-xs" />
                <Button
                  type="button"
                  variant="outline"
                  size="icon"
                  aria-label={t("settings.totp.copySecret")}
                  onClick={() => {
                    void navigator.clipboard.writeText(secret);
                    toast.success(t("settings.totp.copiedToast"));
                  }}
                >
                  <Copy className="size-4" />
                </Button>
              </div>
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="totp-code">{t("settings.totp.codeLabel")}</Label>
              <Input
                id="totp-code"
                inputMode="numeric"
                autoComplete="one-time-code"
                placeholder="000000"
                maxLength={6}
                value={code}
                onChange={(e) => setCode(e.target.value.replace(/[^0-9]/g, ""))}
                className="text-center font-mono tracking-[0.4em]"
              />
              <p className="truncate text-xs text-muted-foreground">{otpauth}</p>
            </div>
          </div>
        )}
        {!pairing && !totpEnabled && (
          <p className="text-sm text-muted-foreground">{t("settings.totp.enableHint")}</p>
        )}
        {totpEnabled && (
          <div className="grid gap-2">
            <Label htmlFor="totp-disable-pwd">{t("settings.totp.passwordLabel")}</Label>
            <PasswordInput
              id="totp-disable-pwd"
              value={disablePassword}
              onChange={setDisablePassword}
              autoComplete="current-password"
            />
          </div>
        )}
      </CardContent>
      <CardFooter className="flex flex-wrap gap-2 px-4 sm:px-6">
        {!totpEnabled && !pairing && (
          <Button className="h-10" onClick={() => setupMutation.mutate()} disabled={setupMutation.isPending}>
            {setupMutation.isPending && <Loader2 className="size-4 animate-spin" />}
            {t("settings.totp.enable")}
          </Button>
        )}
        {pairing && (
          <>
            <Button
              className="h-10"
              onClick={() => activateMutation.mutate()}
              disabled={code.length !== 6 || activateMutation.isPending}
            >
              {activateMutation.isPending && <Loader2 className="size-4 animate-spin" />}
              {t("settings.totp.confirmActivate")}
            </Button>
            <Button
              variant="outline"
              className="h-10"
              onClick={() => {
                setPairing(false);
                setSecret("");
                setCode("");
              }}
            >
              {t("settings.totp.cancel")}
            </Button>
          </>
        )}
        {totpEnabled && (
          <Button
            variant="destructive"
            className="h-10"
            onClick={() => disableMutation.mutate()}
            disabled={disablePassword === "" || disableMutation.isPending}
          >
            {disableMutation.isPending && <Loader2 className="size-4 animate-spin" />}
            {t("settings.totp.disable")}
          </Button>
        )}
      </CardFooter>
    </Card>
  );
}
