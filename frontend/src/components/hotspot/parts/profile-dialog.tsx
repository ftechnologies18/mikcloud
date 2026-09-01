"use client";

// Dialog PROFIL HOTSPOT (forfait) — création / édition. Regroupe les réglages
// existants (débit, durée, quota, prix) + les champs P0 du contrat :
// - Expiration (F1) : expMode (notify/remove), grâce, verrouillage 1 session ;
// - Tarifs (F13) : prix de vente affiché sur le voucher (sellingPrice).

import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Loader2 } from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { useCurrency } from "@/components/hotspot/parts/sd-currency";
import { api } from "@/lib/hotspot/api";
import { fmtRouterDuration } from "@/lib/hotspot/format";
import { useI18n } from "@/lib/hotspot/i18n";
import type { Profile, ProfileExpiryMode } from "@/lib/hotspot/types";

// Format RouterOS : "2M/2M", "512k/512k", "5M" (insensible à la casse).
const RATE_LIMIT_RE = /^\d+[KM](\/\d+[KM])?$/i;

const GRACE_MAX = 43_200; // 30 jours en minutes (contrat F1)

// Unité de saisie de la durée de session — le serveur stocke TOUJOURS des
// minutes (sessionTimeoutMin) : la conversion se fait ici, à la frappe.
type SessionUnit = "min" | "hour" | "day";

// Facteur de conversion vers minutes selon l'unité choisie.
const SESSION_UNIT_MIN: Record<SessionUnit, number> = { min: 1, hour: 60, day: 1440 };

// Parité Mikhmon : unité de saisie de la VALIDITÉ [wdhm] — le serveur stocke
// des minutes (validityMin), la conversion se fait ici, à la frappe.
type ValidityUnit = "min" | "hour" | "day" | "week";

const VALIDITY_UNIT_MIN: Record<ValidityUnit, number> = {
  min: 1,
  hour: 60,
  day: 1440,
  week: 10080,
};

const VALIDITY_MAX_MIN = 2_628_000; // 5 ans (borne serveur)

// Déduit l'unité la plus lisible pour afficher une durée stockée en minutes
// (multiple de 1440 → jours, multiple de 60 → heures, sinon minutes).
function sessionDisplay(min: number): { sessionTimeoutValue: string; sessionTimeoutUnit: SessionUnit } {
  if (min > 0 && min % 1440 === 0) return { sessionTimeoutValue: String(min / 1440), sessionTimeoutUnit: "day" };
  if (min > 0 && min % 60 === 0) return { sessionTimeoutValue: String(min / 60), sessionTimeoutUnit: "hour" };
  return { sessionTimeoutValue: String(min), sessionTimeoutUnit: "min" };
}

// Déduit l'unité la plus lisible pour la validité (semaines → jours → heures → minutes).
function validityDisplay(min: number): { validityValue: string; validityUnit: ValidityUnit } {
  if (min > 0 && min % 10080 === 0) return { validityValue: String(min / 10080), validityUnit: "week" };
  if (min > 0 && min % 1440 === 0) return { validityValue: String(min / 1440), validityUnit: "day" };
  if (min > 0 && min % 60 === 0) return { validityValue: String(min / 60), validityUnit: "hour" };
  return { validityValue: String(min), validityUnit: "min" };
}

interface ProfileForm {
  name: string;
  rateLimit: string;
  sessionTimeoutValue: string;
  sessionTimeoutUnit: SessionUnit;
  validityValue: string;
  validityUnit: ValidityUnit;
  sharedUsers: string;
  price: string;
  dataQuotaMb: string;
  expMode: ProfileExpiryMode;
  gracePeriodMin: string;
  lockUser: boolean;
  lockFirstDevice: boolean;
  sellingPrice: string;
  addressPool: string;
  parentQueue: string;
}

const DEFAULT_FORM: ProfileForm = {
  name: "",
  rateLimit: "",
  // Défaut 1 heure (équivalent à l'ancien défaut 60 min, plus lisible).
  sessionTimeoutValue: "1",
  sessionTimeoutUnit: "hour",
  validityValue: "1",
  validityUnit: "day",
  sharedUsers: "1",
  price: "0",
  dataQuotaMb: "0",
  expMode: "notify",
  gracePeriodMin: "0",
  lockUser: false,
  lockFirstDevice: false,
  sellingPrice: "0",
  addressPool: "",
  parentQueue: "",
};

interface ProfileEditDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Profil en cours d'édition — null = création. */
  profile: Profile | null;
}

function formFromProfile(profile: Profile): ProfileForm {
  return {
    name: profile.name,
    rateLimit: profile.rateLimit,
    ...sessionDisplay(profile.sessionTimeoutMin),
    // Parité Mikhmon : validité fine (minutes) si présente, sinon jours historiques.
    ...validityDisplay(profile.validityMin > 0 ? profile.validityMin : profile.validityDays * 1440),
    sharedUsers: String(profile.sharedUsers),
    price: String(profile.price),
    dataQuotaMb: String(profile.dataQuotaMb),
    expMode: profile.expMode ?? "notify",
    gracePeriodMin: String(profile.gracePeriodMin ?? 0),
    lockUser: profile.lockUser ?? false,
    lockFirstDevice: profile.lockFirstDevice ?? false,
    sellingPrice: String(profile.sellingPrice ?? 0),
    addressPool: profile.addressPool ?? "",
    parentQueue: profile.parentQueue ?? "",
  };
}

// Le parent monte ce dialogue conditionnellement (voir profiles-view) : l'état
// du formulaire s'initialise donc au montage, sans effet de synchronisation.
export function ProfileEditDialog({ open, onOpenChange, profile }: ProfileEditDialogProps) {
  const { t, tf } = useI18n();
  const currency = useCurrency();
  const queryClient = useQueryClient();
  const [form, setForm] = useState<ProfileForm>(() =>
    profile ? formFromProfile(profile) : DEFAULT_FORM,
  );

  const saveMutation = useMutation({
    mutationFn: (payload: { id: string | null; body: Record<string, unknown> }) =>
      payload.id
        ? api<Profile>(`/api/profiles/${payload.id}`, { method: "PUT", body: payload.body })
        : api<Profile>("/api/profiles", { method: "POST", body: payload.body }),
    onSuccess: (_saved, variables) => {
      toast.success(variables.id ? t("profiles.dialog.updatedToast") : t("profiles.dialog.createdToast"));
      onOpenChange(false);
      void queryClient.invalidateQueries({ queryKey: ["/api/profiles"] });
      void queryClient.invalidateQueries({ queryKey: ["/api/dashboard"] });
      void queryClient.invalidateQueries({ queryKey: ["/api/reports"] });
    },
    onError: (error: Error) => toast.error(error.message),
  });

  const rateInvalid = form.rateLimit.trim() !== "" && !RATE_LIMIT_RE.test(form.rateLimit.trim());

  // Valeur saisie dans l'unité choisie, convertie en minutes (NaN si vide).
  const sessionNum =
    parseInt(form.sessionTimeoutValue, 10) * SESSION_UNIT_MIN[form.sessionTimeoutUnit];
  const validityNum = parseInt(form.validityValue, 10) * VALIDITY_UNIT_MIN[form.validityUnit];
  const devicesNum = parseInt(form.sharedUsers, 10);
  const priceNum = Number(form.price);
  const quotaNum = parseInt(form.dataQuotaMb, 10);
  const graceNum = parseInt(form.gracePeriodMin, 10);
  const sellingNum = Number(form.sellingPrice);

  const formValid =
    form.name.trim() !== "" &&
    form.rateLimit.trim() !== "" &&
    !rateInvalid &&
    Number.isInteger(sessionNum) &&
    sessionNum >= 1 &&
    Number.isInteger(validityNum) &&
    validityNum >= 1 &&
    validityNum <= VALIDITY_MAX_MIN &&
    Number.isInteger(devicesNum) &&
    devicesNum >= 1 &&
    devicesNum <= 10 &&
    Number.isFinite(priceNum) &&
    priceNum >= 0 &&
    Number.isInteger(quotaNum) &&
    quotaNum >= 0 &&
    Number.isInteger(graceNum) &&
    graceNum >= 0 &&
    graceNum <= GRACE_MAX &&
    Number.isFinite(sellingNum) &&
    sellingNum >= 0;

  function submitProfile() {
    if (!formValid || saveMutation.isPending) {
      if (form.name.trim() === "") toast.error(t("common.nameRequired"));
      else if (form.rateLimit.trim() === "" || rateInvalid)
        toast.error(t("profiles.dialog.rateToast"));
      else if (!Number.isInteger(sessionNum) || sessionNum < 1)
        toast.error(t("profiles.dialog.sessionToast"));
      else if (!Number.isInteger(validityNum) || validityNum < 1 || validityNum > VALIDITY_MAX_MIN)
        toast.error(t("profiles.dialog.validityToast"));
      else if (!Number.isInteger(devicesNum) || devicesNum < 1 || devicesNum > 10)
        toast.error(t("profiles.dialog.devicesToast"));
      else if (!Number.isFinite(priceNum) || priceNum < 0) toast.error(t("profiles.dialog.priceToast"));
      else if (!Number.isInteger(quotaNum) || quotaNum < 0) toast.error(t("profiles.dialog.quotaToast"));
      else if (!Number.isInteger(graceNum) || graceNum < 0 || graceNum > GRACE_MAX)
        toast.error(tf("profiles.dialog.graceToast", { n: GRACE_MAX }));
      else if (!Number.isFinite(sellingNum) || sellingNum < 0)
        toast.error(t("profiles.dialog.sellingToast"));
      return;
    }
    saveMutation.mutate({
      id: profile?.id ?? null,
      body: {
        name: form.name.trim(),
        rateLimit: form.rateLimit.trim(),
        sessionTimeoutMin: sessionNum,
        sharedUsers: devicesNum,
        // Parité Mikhmon : validité fine en minutes (le serveur maintient
        // validityDays en cohérence, arrondi supérieur).
        validityMin: validityNum,
        price: Math.round(priceNum),
        dataQuotaMb: quotaNum,
        expMode: form.expMode,
        gracePeriodMin: graceNum,
        lockUser: form.lockUser,
        lockFirstDevice: form.lockFirstDevice,
        sellingPrice: Math.round(sellingNum),
        addressPool: form.addressPool.trim(),
        parentQueue: form.parentQueue.trim(),
      },
    });
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>
            {profile ? tf("profiles.dialog.editTitle", { name: profile.name }) : t("profiles.dialog.newTitle")}
          </DialogTitle>
          <DialogDescription>
            {profile ? t("profiles.dialog.editDesc") : t("profiles.dialog.newDesc")}
          </DialogDescription>
        </DialogHeader>

        <form
          className="grid gap-4"
          onSubmit={(event) => {
            event.preventDefault();
            submitProfile();
          }}
        >
          {/* ─── Forfait ─── */}
          <div className="grid gap-4">
            <div className="grid gap-2">
              <Label htmlFor="profile-name">{t("common.name")}</Label>
              <Input
                id="profile-name"
                placeholder={t("profiles.dialog.namePlaceholder")}
                value={form.name}
                onChange={(event) => setForm((f) => ({ ...f, name: event.target.value }))}
                disabled={saveMutation.isPending}
              />
            </div>

            <div className="grid gap-2">
              <Label htmlFor="profile-rate">{t("profiles.dialog.rate")}</Label>
              <Input
                id="profile-rate"
                placeholder="2M/2M"
                value={form.rateLimit}
                onChange={(event) => setForm((f) => ({ ...f, rateLimit: event.target.value }))}
                disabled={saveMutation.isPending}
                aria-invalid={rateInvalid}
                className="font-mono"
              />
              <p className={rateInvalid ? "text-xs text-destructive" : "text-xs text-muted-foreground"}>
                {rateInvalid ? t("profiles.dialog.rateInvalid") : t("profiles.dialog.rateHint")}
              </p>
            </div>

            <div className="grid gap-4 sm:grid-cols-2">
              <div className="grid gap-2">
                <Label htmlFor="profile-session">{t("profiles.dialog.session")}</Label>
                <div className="flex gap-2">
                  <Input
                    id="profile-session"
                    type="number"
                    min={1}
                    value={form.sessionTimeoutValue}
                    onChange={(event) => setForm((f) => ({ ...f, sessionTimeoutValue: event.target.value }))}
                    disabled={saveMutation.isPending}
                    className="min-w-0 flex-1"
                  />
                  <Select
                    value={form.sessionTimeoutUnit}
                    onValueChange={(value) =>
                      setForm((f) => ({ ...f, sessionTimeoutUnit: value as SessionUnit }))
                    }
                  >
                    <SelectTrigger
                      className="w-[6.75rem] shrink-0"
                      aria-label={t("profiles.dialog.session")}
                    >
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="min">{t("profiles.dialog.sessionUnitMin")}</SelectItem>
                      <SelectItem value="hour">{t("profiles.dialog.sessionUnitHour")}</SelectItem>
                      <SelectItem value="day">{t("profiles.dialog.sessionUnitDay")}</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                {form.sessionTimeoutUnit !== "min" && Number.isInteger(sessionNum) && (
                  <p className="text-xs text-muted-foreground">
                    {tf("profiles.dialog.sessionEq", { n: sessionNum })}
                  </p>
                )}
                <p className="text-xs text-muted-foreground">
                  {t("profiles.dialog.sessionHint")}
                </p>
              </div>
              <div className="grid gap-2">
                <Label htmlFor="profile-validity">{t("profiles.dialog.validity")}</Label>
                <div className="flex gap-2">
                  <Input
                    id="profile-validity"
                    type="number"
                    min={1}
                    value={form.validityValue}
                    onChange={(event) => setForm((f) => ({ ...f, validityValue: event.target.value }))}
                    disabled={saveMutation.isPending}
                    className="min-w-0 flex-1"
                  />
                  <Select
                    value={form.validityUnit}
                    onValueChange={(value) =>
                      setForm((f) => ({ ...f, validityUnit: value as ValidityUnit }))
                    }
                  >
                    <SelectTrigger
                      className="w-[6.75rem] shrink-0"
                      aria-label={t("profiles.dialog.validity")}
                    >
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="min">{t("profiles.dialog.validityUnitMin")}</SelectItem>
                      <SelectItem value="hour">{t("profiles.dialog.validityUnitHour")}</SelectItem>
                      <SelectItem value="day">{t("profiles.dialog.validityUnitDay")}</SelectItem>
                      <SelectItem value="week">{t("profiles.dialog.validityUnitWeek")}</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                {form.validityUnit !== "min" && Number.isInteger(validityNum) && (
                  <p className="text-xs text-muted-foreground">
                    {tf("profiles.dialog.validityEq", { d: fmtRouterDuration(validityNum) })}
                  </p>
                )}
              </div>
            </div>

            <div className="grid grid-cols-2 gap-4">
              <div className="grid gap-2">
                <Label htmlFor="profile-devices">{t("profiles.dialog.devices")}</Label>
                <Input
                  id="profile-devices"
                  type="number"
                  min={1}
                  max={10}
                  value={form.sharedUsers}
                  onChange={(event) => setForm((f) => ({ ...f, sharedUsers: event.target.value }))}
                  disabled={saveMutation.isPending}
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="profile-quota">{t("profiles.dialog.quota")}</Label>
                <Input
                  id="profile-quota"
                  type="number"
                  min={0}
                  value={form.dataQuotaMb}
                  onChange={(event) => setForm((f) => ({ ...f, dataQuotaMb: event.target.value }))}
                  disabled={saveMutation.isPending}
                />
                <p className="text-xs text-muted-foreground">{t("profiles.dialog.quotaHint")}</p>
              </div>
            </div>
          </div>

          {/* ─── RouterOS (parité Mikhmon) : address-pool / parent-queue ─── */}
          <fieldset className="grid gap-4 rounded-lg border p-4" disabled={saveMutation.isPending}>
            <legend className="px-1.5 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
              {t("profiles.dialog.routeros")}
            </legend>
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="grid gap-2">
                <Label htmlFor="profile-pool">{t("profiles.dialog.pool")}</Label>
                <Input
                  id="profile-pool"
                  placeholder="pool-hotspot-1"
                  className="font-mono"
                  value={form.addressPool}
                  onChange={(event) => setForm((f) => ({ ...f, addressPool: event.target.value }))}
                />
                <p className="text-xs text-muted-foreground">{t("profiles.dialog.poolHint")}</p>
              </div>
              <div className="grid gap-2">
                <Label htmlFor="profile-queue">{t("profiles.dialog.queue")}</Label>
                <Input
                  id="profile-queue"
                  placeholder="q-parent-vip"
                  className="font-mono"
                  value={form.parentQueue}
                  onChange={(event) => setForm((f) => ({ ...f, parentQueue: event.target.value }))}
                />
                <p className="text-xs text-muted-foreground">{t("profiles.dialog.queueHint")}</p>
              </div>
            </div>
          </fieldset>

          {/* ─── Tarifs ─── */}
          <fieldset className="grid gap-4 rounded-lg border p-4" disabled={saveMutation.isPending}>
            <legend className="px-1.5 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
              {t("profiles.dialog.pricing")}
            </legend>
            <div className="grid grid-cols-2 gap-4">
              <div className="grid gap-2">
                <Label htmlFor="profile-price">{tf("profiles.dialog.price", { currency })}</Label>
                <Input
                  id="profile-price"
                  type="number"
                  min={0}
                  value={form.price}
                  onChange={(event) => setForm((f) => ({ ...f, price: event.target.value }))}
                />
                <p className="text-xs text-muted-foreground">{t("profiles.dialog.priceHint")}</p>
              </div>
              <div className="grid gap-2">
                <Label htmlFor="profile-selling">{tf("profiles.dialog.selling", { currency })}</Label>
                <Input
                  id="profile-selling"
                  type="number"
                  min={0}
                  value={form.sellingPrice}
                  onChange={(event) => setForm((f) => ({ ...f, sellingPrice: event.target.value }))}
                />
                <p className="text-xs text-muted-foreground">
                  {t("profiles.dialog.sellingHintPre")}
                  <code className="font-mono text-[11px]">{"{{price}}"}</code>
                  {t("profiles.dialog.sellingHintPost")}
                </p>
              </div>
            </div>
          </fieldset>

          {/* ─── Expiration ─── */}
          <fieldset className="grid gap-4 rounded-lg border p-4" disabled={saveMutation.isPending}>
            <legend className="px-1.5 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
              {t("profiles.dialog.expiry")}
            </legend>
            <div className="grid gap-2 sm:grid-cols-2">
              <div className="grid gap-2">
                <Label htmlFor="profile-expmode">{t("profiles.dialog.expMode")}</Label>
                <Select
                  value={form.expMode}
                  onValueChange={(value) =>
                    setForm((f) => ({ ...f, expMode: value as ProfileExpiryMode }))
                  }
                >
                  <SelectTrigger id="profile-expmode" className="h-10 w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="none">{t("profiles.dialog.expNone")}</SelectItem>
                    <SelectItem value="notify">{t("profiles.dialog.expNotify")}</SelectItem>
                    <SelectItem value="remove">{t("profiles.dialog.expRemove")}</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className="grid gap-2">
                <Label htmlFor="profile-grace">{t("profiles.dialog.grace")}</Label>
                <Input
                  id="profile-grace"
                  type="number"
                  min={0}
                  max={GRACE_MAX}
                  value={form.gracePeriodMin}
                  onChange={(event) => setForm((f) => ({ ...f, gracePeriodMin: event.target.value }))}
                />
                <p className="text-xs text-muted-foreground">
                  {tf("profiles.dialog.graceHint", { n: GRACE_MAX })}
                </p>
              </div>
            </div>
            <div className="flex items-center justify-between gap-4 rounded-md bg-muted/40 px-3 py-2.5">
              <div className="min-w-0">
                <Label htmlFor="profile-lock" className="text-sm">
                  {t("profiles.dialog.lock")}
                </Label>
                <p className="text-xs text-muted-foreground">{t("profiles.dialog.lockHint")}</p>
              </div>
              <Switch
                id="profile-lock"
                checked={form.lockUser}
                onCheckedChange={(checked) => setForm((f) => ({ ...f, lockUser: checked }))}
              />
            </div>
          </fieldset>

          {/* ─── Sécurité (v2) : verrou « 1er appareil » — anti-partage ─── */}
          <fieldset className="grid gap-4 rounded-lg border p-4" disabled={saveMutation.isPending}>
            <legend className="px-1.5 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
              {t("profiles.dialog.security")}
            </legend>
            <div className="flex items-center justify-between gap-4 rounded-md bg-muted/40 px-3 py-2.5">
              <div className="min-w-0">
                <Label htmlFor="profile-lock-device" className="text-sm">
                  {t("profiles.dialog.lockDevice")}
                </Label>
                <p className="text-xs text-muted-foreground">{t("profiles.dialog.lockDeviceHint")}</p>
              </div>
              <Switch
                id="profile-lock-device"
                checked={form.lockFirstDevice}
                onCheckedChange={(checked) => setForm((f) => ({ ...f, lockFirstDevice: checked }))}
              />
            </div>
          </fieldset>

          {saveMutation.isError && (
            <p className="text-sm text-destructive" role="alert">
              {saveMutation.error.message}
            </p>
          )}

          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)} disabled={saveMutation.isPending}>
              {t("common.cancel")}
            </Button>
            <Button type="submit" disabled={!formValid || saveMutation.isPending}>
              {saveMutation.isPending && <Loader2 className="size-4 animate-spin" />}
              {profile ? t("common.save") : t("profiles.dialog.createSubmit")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
