"use client";

// N°63 — Wizard 2 étapes pour la création / modification d'un site WiFi
// Jetable (remplace le formulaire plat d'un bloc de la vue wifi-view).
// Étape 1 « Le site » : identité (nom, routeur, profil) — les trois champs
// REQUIS, validés en direct : le bouton « Continuer » ne s'ouvre que sur un
// socle valide. Étape 2 « L'offre » : quotas, protections anti-abus, réseau
// WiFi et switches — précédée d'un RÉCAPITULATIF explicite des choix de
// l'étape 1. Animations : stepper à connecteur animé (✓ sur l'étape
// franchie), transition directionnelle entre étapes (slide gauche/droite,
// inverse au retour), champs en cascade — framer-motion, avec
// useReducedMotion respecté (transitions réduites à néant).

import { useState, type FormEvent } from "react";
import { AnimatePresence, motion, useReducedMotion, type Variants } from "framer-motion";
import { ArrowLeft, ArrowRight, Check, CircleCheck, Clock, Loader2, Radio, Store, Wifi } from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
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
import { useI18n } from "@/lib/hotspot/i18n";
import type { Profile, RouterDevice, WifiSite, WifiSitePayload } from "@/lib/hotspot/types";
import { cn } from "@/lib/utils";

/* État du formulaire — structuralement identique au payload (le parent
 * reste propriétaire de l'état : ouvert/fermé et pré-remplissage inchangés,
 * le wizard n'est que la présentation en 2 étapes). */
export type SiteForm = WifiSitePayload;

export const EMPTY_FORM: SiteForm = {
  name: "",
  routerId: "",
  profileId: "",
  freeTimeMin: 30,
  freeDataMb: 100,
  marketingOptIn: true,
  dailyPerPhone: 1,
  dailyPerMac: 1,
  dailyCap: 100,
  wifiSsid: "",
  wifiPassword: "",
  active: true,
};

export function formFromSite(site: WifiSite): SiteForm {
  return {
    name: site.name,
    routerId: site.routerId,
    profileId: site.profileId,
    freeTimeMin: site.freeTimeMin,
    freeDataMb: site.freeDataMb,
    marketingOptIn: site.marketingOptIn,
    dailyPerPhone: site.dailyPerPhone,
    dailyPerMac: site.dailyPerMac,
    dailyCap: site.dailyCap,
    wifiSsid: site.wifiSsid,
    wifiPassword: site.wifiPassword,
    active: site.active,
  };
}

/* Transitions : slide directionnel + cascade des champs (cohérent avec le
 * stagger du signup-modal). Le sens (custom) suit la navigation : avant =
 * vers la droite, retour = vers la gauche. */
const stepVariants: Variants = {
  enter: (dir: number) => ({ opacity: 0, x: 56 * dir }),
  center: {
    opacity: 1,
    x: 0,
    transition: { duration: 0.32, ease: "easeOut", staggerChildren: 0.05, delayChildren: 0.05 },
  },
  exit: (dir: number) => ({ opacity: 0, x: -56 * dir, transition: { duration: 0.2, ease: "easeIn" } }),
};
const rise: Variants = {
  hidden: { opacity: 0, y: 10 },
  show: { opacity: 1, y: 0, transition: { duration: 0.3, ease: "easeOut" } },
};

interface WifiSiteWizardProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Site en édition (null = création). */
  editing: WifiSite | null;
  /** Valeurs courantes — propriété du PARENT (pré-remplissage ouvert au même endroit qu'avant). */
  form: SiteForm;
  setForm: React.Dispatch<React.SetStateAction<SiteForm>>;
  routers: RouterDevice[];
  profiles: Profile[];
  saving: boolean;
  /** Soumission finale (étape 2) — payload identique à l'ancien dialog plat. */
  onSubmit: (payload: WifiSitePayload) => void;
}

export function WifiSiteWizard({
  open,
  onOpenChange,
  editing,
  form,
  setForm,
  routers,
  profiles,
  saving,
  onSubmit,
}: WifiSiteWizardProps) {
  const { t, tf } = useI18n();
  const reduce = useReducedMotion();

  // Étape courante + sens de la transition. Remis à 1 par REMONTAGE : le
  // parent key le wizard sur un nonce incrémenté à chaque ouverture (le
  // formulaire lui-même vit dans le parent — rien à réinitialiser ici).
  const [step, setStep] = useState<1 | 2>(1);
  const [direction, setDirection] = useState<1 | -1>(1);
  // Erreur inline du nom (validation au blur — champ requis le plus fragile).
  const [nameErr, setNameErr] = useState<string | null>(null);

  function goStep(next: 1 | 2) {
    setDirection(next > step ? 1 : -1);
    setStep(next);
  }

  /* ─── Validité LIVE de l'étape 1 (les trois requis) ─── */
  const nameOk = form.name.trim().length >= 2;
  const routerOk = form.routerId !== "";
  const profileOk = form.profileId !== "";
  const step1Ok = nameOk && routerOk && profileOk;

  function handleStep1Submit(e: FormEvent) {
    e.preventDefault();
    if (!step1Ok || saving) {
      if (!nameOk) setNameErr(t("wifi.wiz.err.name"));
      return;
    }
    goStep(2);
  }

  function handleStep2Submit(e: FormEvent) {
    e.preventDefault();
    if (saving) return;
    onSubmit({ ...form, name: form.name.trim() });
  }

  /* Récapitulatif de l'étape 1 affiché en tête de l'étape 2 (explicite :
   * le gérant voit le socle qu'il a posé sans revenir en arrière). */
  const routerName = routers.find((r) => r.id === form.routerId)?.name;
  const profile = profiles.find((p) => p.id === form.profileId);

  const variants = reduce
    ? { enter: { opacity: 0 }, center: { opacity: 1 }, exit: { opacity: 0 } }
    : stepVariants;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{editing ? t("wifi.edit") : t("wifi.create")}</DialogTitle>
          <DialogDescription>
            {t(step === 1 ? "wifi.wiz.step1Desc" : "wifi.wiz.step2Desc")}{" "}
            <span className="tabular-nums font-medium text-foreground/70">
              {tf("wifi.wiz.stepOf", { n: step })}
            </span>
          </DialogDescription>
        </DialogHeader>

        {/* Stepper — deux jalons, connecteur qui se remplit, ✓ sur l'étape franchie. */}
        <div className="flex items-center gap-2.5 px-1" aria-hidden="true">
          <StepBadge n={1} label={t("wifi.wiz.step1")} state={step === 1 ? "active" : step === 2 ? "done" : "todo"} />
          <div className="h-1 flex-1 overflow-hidden rounded-full bg-muted">
            <div
              className="h-full rounded-full bg-primary transition-[width] duration-500 ease-out"
              style={{ width: step === 2 ? "100%" : "0%" }}
            />
          </div>
          <StepBadge n={2} label={t("wifi.wiz.step2")} state={step === 2 ? "active" : "todo"} />
        </div>

        <AnimatePresence mode="wait" custom={direction} initial={false}>
          <motion.div
            key={step}
            custom={direction}
            variants={variants}
            initial="enter"
            animate="center"
            exit="exit"
          >
            {step === 1 ? (
              /* ─── ÉTAPE 1 — LE SITE (identité : les trois requis) ─── */
              <motion.form variants={staggerParent} initial="hidden" animate="show" onSubmit={handleStep1Submit} className="space-y-4 pt-1">
                <motion.div variants={rise} className="space-y-1.5">
                  <div className="flex items-center justify-between gap-2">
                    <Label htmlFor="wifi-name">{t("wifi.name")}</Label>
                    <FieldCheck ok={nameOk} />
                  </div>
                  <Input
                    id="wifi-name"
                    value={form.name}
                    placeholder={t("wifi.namePh")}
                    maxLength={60}
                    autoComplete="off"
                    disabled={saving}
                    aria-invalid={Boolean(nameErr)}
                    aria-describedby={nameErr ? "wifi-wiz-err-name" : undefined}
                    onChange={(e) => {
                      setForm((f) => ({ ...f, name: e.target.value }));
                      if (nameErr && e.target.value.trim().length >= 2) setNameErr(null);
                    }}
                    onBlur={() => {
                      if (form.name && form.name.trim().length < 2) setNameErr(t("wifi.wiz.err.name"));
                    }}
                  />
                  {nameErr && (
                    <p id="wifi-wiz-err-name" role="alert" className="text-xs text-destructive">
                      {nameErr}
                    </p>
                  )}
                </motion.div>

                <motion.div variants={rise} className="space-y-1.5">
                  <div className="flex items-center justify-between gap-2">
                    <Label>{t("wifi.router")}</Label>
                    <FieldCheck ok={routerOk} />
                  </div>
                  <Select
                    value={form.routerId}
                    onValueChange={(v) => setForm((f) => ({ ...f, routerId: v }))}
                    disabled={saving}
                  >
                    <SelectTrigger aria-label={t("wifi.router")}>
                      <SelectValue placeholder={t("wifi.router")} />
                    </SelectTrigger>
                    <SelectContent>
                      {routers.map((r) => (
                        <SelectItem key={r.id} value={r.id}>
                          {r.name}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  {routers.length === 0 && (
                    <p className="text-xs text-chart-3">{t("wifi.wiz.noRouter")}</p>
                  )}
                </motion.div>

                <motion.div variants={rise} className="space-y-1.5">
                  <div className="flex items-center justify-between gap-2">
                    <Label>{t("wifi.profile")}</Label>
                    <FieldCheck ok={profileOk} />
                  </div>
                  <Select
                    value={form.profileId}
                    onValueChange={(v) => setForm((f) => ({ ...f, profileId: v }))}
                    disabled={saving}
                  >
                    <SelectTrigger aria-label={t("wifi.profile")}>
                      <SelectValue placeholder={t("wifi.profile")} />
                    </SelectTrigger>
                    <SelectContent>
                      {profiles.map((p) => (
                        <SelectItem key={p.id} value={p.id}>
                          {p.name} · {p.price} F
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  <p className="text-xs text-muted-foreground">{t("wifi.profileHint")}</p>
                  {profiles.length === 0 && (
                    <p className="text-xs text-chart-3">{t("wifi.wiz.noProfile")}</p>
                  )}
                </motion.div>

                <motion.div variants={rise}>
                  <Button type="submit" className="w-full shadow-lg shadow-primary/25" disabled={!step1Ok || saving}>
                    {t("wifi.wiz.continue")}
                    <ArrowRight className="size-4" aria-hidden />
                  </Button>
                </motion.div>
              </motion.form>
            ) : (
              /* ─── ÉTAPE 2 — L'OFFRE (quotas, anti-abus, réseau, switches) ─── */
              <motion.form variants={staggerParent} initial="hidden" animate="show" onSubmit={handleStep2Submit} className="space-y-4 pt-1">
                {/* Récapitulatif de l'étape 1 — explicite, animé en cascade. */}
                <motion.div variants={rise} className="flex flex-wrap gap-1.5" aria-label={t("wifi.wiz.recap")}>
                  <RecapChip icon={Store} label={form.name.trim()} />
                  {routerName && <RecapChip icon={Radio} label={routerName} />}
                  {profile && (
                    <RecapChip icon={Clock} label={`${profile.name}${profile.price > 0 ? ` · ${profile.price} F` : ""}`} />
                  )}
                </motion.div>

                <motion.div variants={rise} className="grid gap-4 sm:grid-cols-2">
                  <div className="space-y-1.5">
                    <Label htmlFor="wifi-time">{t("wifi.freeTime")}</Label>
                    <Input
                      id="wifi-time"
                      type="number"
                      min={0}
                      value={form.freeTimeMin}
                      disabled={saving}
                      onChange={(e) => setForm((f) => ({ ...f, freeTimeMin: Number(e.target.value) || 0 }))}
                    />
                    <p className="text-xs text-muted-foreground">{t("wifi.freeTimeHint")}</p>
                  </div>
                  <div className="space-y-1.5">
                    <Label htmlFor="wifi-data">{t("wifi.freeData")}</Label>
                    <Input
                      id="wifi-data"
                      type="number"
                      min={0}
                      value={form.freeDataMb}
                      disabled={saving}
                      onChange={(e) => setForm((f) => ({ ...f, freeDataMb: Number(e.target.value) || 0 }))}
                    />
                    <p className="text-xs text-muted-foreground">{t("wifi.freeDataHint")}</p>
                  </div>
                  <div className="space-y-1.5">
                    <Label htmlFor="wifi-perphone">{t("wifi.perPhone")}</Label>
                    <Input
                      id="wifi-perphone"
                      type="number"
                      min={1}
                      max={10}
                      value={form.dailyPerPhone}
                      disabled={saving}
                      onChange={(e) => setForm((f) => ({ ...f, dailyPerPhone: Number(e.target.value) || 1 }))}
                    />
                    <p className="text-xs text-muted-foreground">{t("wifi.perPhoneHint")}</p>
                  </div>
                  <div className="space-y-1.5">
                    <Label htmlFor="wifi-permac">{t("wifi.perMac")}</Label>
                    <Input
                      id="wifi-permac"
                      type="number"
                      min={1}
                      max={10}
                      value={form.dailyPerMac}
                      disabled={saving}
                      onChange={(e) => setForm((f) => ({ ...f, dailyPerMac: Number(e.target.value) || 1 }))}
                    />
                    <p className="text-xs text-muted-foreground">{t("wifi.perMacHint")}</p>
                  </div>
                  <div className="space-y-1.5 sm:col-span-2">
                    <Label htmlFor="wifi-cap">{t("wifi.dailyCap")}</Label>
                    <Input
                      id="wifi-cap"
                      type="number"
                      min={1}
                      max={1000}
                      value={form.dailyCap}
                      disabled={saving}
                      onChange={(e) => setForm((f) => ({ ...f, dailyCap: Number(e.target.value) || 100 }))}
                    />
                    <p className="text-xs text-muted-foreground">{t("wifi.dailyCapHint")}</p>
                  </div>
                </motion.div>

                {/* N°49 — Réseau WiFi : alimente le QR de CONNEXION de l'affiche. */}
                <motion.div variants={rise} className="grid gap-4 sm:grid-cols-2">
                  <div className="space-y-1.5">
                    <Label htmlFor="wifi-ssid">{t("wifi.ssid")}</Label>
                    <Input
                      id="wifi-ssid"
                      value={form.wifiSsid}
                      placeholder="CYBER-ESPACE"
                      maxLength={32}
                      autoComplete="off"
                      disabled={saving}
                      onChange={(e) => setForm((f) => ({ ...f, wifiSsid: e.target.value }))}
                    />
                    <p className="text-xs text-muted-foreground">{t("wifi.ssidHint")}</p>
                  </div>
                  <div className="space-y-1.5">
                    <Label htmlFor="wifi-pass">{t("wifi.wifiPass")}</Label>
                    <Input
                      id="wifi-pass"
                      value={form.wifiPassword}
                      placeholder={t("wifi.wifiPassPh")}
                      maxLength={63}
                      autoComplete="off"
                      disabled={saving}
                      onChange={(e) => setForm((f) => ({ ...f, wifiPassword: e.target.value }))}
                    />
                    <p className="text-xs text-muted-foreground">{t("wifi.wifiPassHint")}</p>
                  </div>
                </motion.div>

                <motion.div variants={rise} className="flex items-center justify-between rounded-lg border p-3">
                  <div>
                    <Label htmlFor="wifi-optin">{t("wifi.optIn")}</Label>
                    <p className="text-xs text-muted-foreground">{t("wifi.optInHint")}</p>
                  </div>
                  <Switch
                    id="wifi-optin"
                    checked={form.marketingOptIn}
                    disabled={saving}
                    onCheckedChange={(v) => setForm((f) => ({ ...f, marketingOptIn: v }))}
                  />
                </motion.div>

                <motion.div variants={rise} className="flex items-center justify-between rounded-lg border p-3">
                  <div className="flex items-center gap-2">
                    <Wifi className="size-4 text-muted-foreground" aria-hidden />
                    <Label htmlFor="wifi-active">{t("wifi.active")}</Label>
                  </div>
                  <Switch
                    id="wifi-active"
                    checked={form.active}
                    disabled={saving}
                    onCheckedChange={(v) => setForm((f) => ({ ...f, active: v }))}
                  />
                </motion.div>

                <motion.div variants={rise} className="flex gap-2">
                  <Button type="button" variant="outline" className="flex-1" onClick={() => goStep(1)} disabled={saving}>
                    <ArrowLeft className="size-4" aria-hidden />
                    {t("wifi.wiz.back")}
                  </Button>
                  <Button type="submit" className="flex-1 shadow-lg shadow-primary/25" disabled={saving}>
                    {saving ? <Loader2 className="size-4 animate-spin" aria-hidden /> : <Check className="size-4" aria-hidden />}
                    {editing ? t("common.save") : t("wifi.create")}
                  </Button>
                </motion.div>
              </motion.form>
            )}
          </motion.div>
        </AnimatePresence>
      </DialogContent>
    </Dialog>
  );
}

/* ─── Sous-composants du stepper / récap ─── */

/** Cascade des champs à l'intérieur d'une étape (le parent gère le slide). */
const staggerParent: Variants = {
  hidden: {},
  show: { transition: { staggerChildren: 0.05, delayChildren: 0.04 } },
};

/** Jalon d'étape : pastille numérotée (active = pleine, franchie = ✓ teal,
 * à venir = grise) + libellé. Décoratif : la description du dialog porte
 * déjà « Étape n/2 » en texte. */
function StepBadge({ n, label, state }: { n: 1 | 2; label: string; state: "active" | "done" | "todo" }) {
  return (
    <span className="flex items-center gap-1.5">
      <span
        className={cn(
          "grid size-6 shrink-0 place-items-center rounded-full text-[11px] font-semibold transition-colors duration-300",
          state === "active" && "bg-primary text-primary-foreground",
          state === "done" && "bg-chart-2/15 text-chart-2",
          state === "todo" && "bg-muted text-muted-foreground",
        )}
      >
        {state === "done" ? <Check className="size-3.5" aria-hidden /> : n}
      </span>
      <span
        className={cn(
          "text-xs whitespace-nowrap transition-colors duration-300",
          state === "active" ? "font-medium text-foreground" : "text-muted-foreground",
        )}
      >
        {label}
      </span>
    </span>
  );
}

/** Coche discrète de champ requis valide (retour live, décoratif). */
function FieldCheck({ ok }: { ok: boolean }) {
  if (!ok) return null;
  return <CircleCheck className="size-4 shrink-0 text-chart-2" aria-hidden />;
}

/** Puce du récapitulatif d'étape 1. */
function RecapChip({ icon: Icon, label }: { icon: typeof Store; label: string }) {
  if (!label) return null;
  return (
    <span className="inline-flex max-w-full items-center gap-1.5 rounded-lg border bg-muted/40 px-2.5 py-1.5 text-xs font-medium">
      <Icon className="size-3.5 shrink-0 text-muted-foreground" aria-hidden />
      <span className="truncate">{label}</span>
    </span>
  );
}
