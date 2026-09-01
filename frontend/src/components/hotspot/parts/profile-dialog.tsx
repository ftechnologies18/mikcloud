"use client";

// « Studio Forfait » — dialog PROFIL HOTSPOT (création / édition) repensé :
// bento 2 colonnes avec CARTE FORFAIT en APERÇU LIVE (dégradé aurora, la
// signature MikCloud) et contrôles interactifs (chips de débit / quota,
// stepper d'appareils, cartes radio d'expiration, toggles d'unités).
// Les animations Framer Motion restent discrètes et respectent
// prefers-reduced-motion.
//
// Logique métier INCHANGÉE : mêmes champs, mêmes bornes de validation, même
// payload API. Champs P0 du contrat : expiration F1 (expMode/grâce/verrous),
// tarifs F13 (prix de vente sur voucher), parité Mikhmon (validité [wdhm],
// address-pool / parent-queue assistés par les ressources routeur F9).

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  motion,
  useAnimationControls,
  useReducedMotion,
  type Variants,
} from "framer-motion";
import {
  ArrowDownUp,
  Bell,
  CalendarClock,
  Check,
  ChevronDown,
  Database,
  Eye,
  Gauge,
  Infinity as InfinityIcon,
  Loader2,
  Lock,
  Minus,
  MonitorSmartphone,
  Plus,
  Router as RouterIcon,
  ShieldCheck,
  Smartphone,
  Timer,
  Trash2,
  TrendingUp,
  Wifi,
  X,
  type LucideIcon,
} from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import {
  Dialog,
  DialogContent,
  DialogDescription,
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
import {
  ToggleGroup,
  ToggleGroupItem,
} from "@/components/ui/toggle-group";
import { useCurrency } from "@/components/hotspot/parts/sd-currency";
import { cn } from "@/lib/utils";
import { api } from "@/lib/hotspot/api";
import {
  formatBytes,
  formatCurrency,
  formatDuration,
  formatRateLimit,
  fmtRouterDuration,
} from "@/lib/hotspot/format";
import { useI18n } from "@/lib/hotspot/i18n";
import { useRouterResources } from "@/lib/hotspot/use-router-resources";
import type { Profile, ProfileExpiryMode, RouterDevice } from "@/lib/hotspot/types";

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

// Presets de débit (format RouterOS) — un tap remplit le champ.
const RATE_PRESETS = ["512k/512k", "1M/1M", "2M/2M", "5M/5M", "10M/10M"];

// Presets de quota de données (Mo) — 0 = illimité.
const QUOTA_PRESETS = [0, 1024, 5120, 10240, 20480, 51200];

// Graduations du stepper d'appareils simultanés.
const DEVICE_CHIPS = [1, 2, 3, 5, 10];

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

// ─── Animations (discrètes, staggeré au montage) ────────────────────────────

const containerVariants: Variants = {
  hidden: {},
  show: { transition: { staggerChildren: 0.055, delayChildren: 0.08 } },
};

const itemVariants: Variants = {
  hidden: { opacity: 0, y: 14 },
  show: { opacity: 1, y: 0, transition: { duration: 0.3, ease: "easeOut" } },
};

// ─── Titre de section (icône en tuile + libellé) ────────────────────────────

function SectionTitle({ icon: Icon, title }: { icon: LucideIcon; title: string }) {
  return (
    <div className="flex items-center gap-2">
      <span className="grid size-7 shrink-0 place-items-center rounded-lg bg-accent text-accent-foreground" aria-hidden>
        <Icon className="size-4" />
      </span>
      <h3 className="text-sm font-semibold tracking-tight">{title}</h3>
    </div>
  );
}

// Carte générique d'un groupe d'options cliquables (modèles, expiration).
function OptionCard({
  selected,
  onClick,
  icon: Icon,
  title,
  desc,
  testId,
}: {
  selected: boolean;
  onClick: () => void;
  icon: LucideIcon;
  title: string;
  desc: string;
  testId?: string;
}) {
  return (
    <motion.button
      type="button"
      role="radio"
      aria-checked={selected}
      data-testid={testId}
      whileTap={{ scale: 0.97 }}
      onClick={onClick}
      className={cn(
        "rounded-xl border p-3 text-left transition-all duration-150",
        selected
          ? "border-primary bg-primary/5 shadow-sm ring-1 ring-primary"
          : "border-border hover:border-primary/40 hover:bg-accent/40",
      )}
    >
      <div className="flex items-center gap-2">
        <span
          className={cn(
            "grid size-6 shrink-0 place-items-center rounded-md transition-colors",
            selected ? "bg-primary text-primary-foreground" : "bg-muted text-muted-foreground",
          )}
          aria-hidden
        >
          <Icon className="size-3.5" />
        </span>
        <span className="truncate text-sm font-medium">{title}</span>
      </div>
      <p className="mt-1.5 text-xs leading-snug text-muted-foreground">{desc}</p>
    </motion.button>
  );
}

// ─── Dialog principal ────────────────────────────────────────────────────────

export function ProfileEditDialog({ open, onOpenChange, profile }: ProfileEditDialogProps) {
  const { t, tf, lang } = useI18n();
  const currency = useCurrency();
  const queryClient = useQueryClient();
  const [form, setForm] = useState<ProfileForm>(() =>
    profile ? formFromProfile(profile) : DEFAULT_FORM,
  );
  const [attempted, setAttempted] = useState(false);
  const shakeControls = useAnimationControls();
  const prefersReducedMotion = useReducedMotion();

  // Parité Mikhmon : ressources routeur (pools / files / serveurs) pour assister
  // la saisie address-pool / parent-queue — datalists (la saisie libre reste
  // possible, les noms sont transmis tels quels au routeur). Sélecteur
  // « Charger depuis un routeur » : « all » = fusion dédupliquée de tous les
  // routeurs non réels, sinon les valeurs RÉELLES du routeur choisi (commande
  // agent read_resources, réponse après check-in ≤ 45 s).
  const { data: routers } = useQuery({
    queryKey: ["/api/routers"],
    queryFn: () => api<RouterDevice[]>("/api/routers"),
  });
  const [sourceRouterId, setSourceRouterId] = useState("all");
  const resources = useRouterResources(routers, sourceRouterId === "all" ? undefined : sourceRouterId);
  const eligibleRouters = (routers ?? []).filter((r) => r.mode !== "real");
  const resourcesEmpty =
    (resources.data?.pools.length ?? 0) === 0 &&
    (resources.data?.queues.length ?? 0) === 0 &&
    (resources.data?.servers.length ?? 0) === 0;

  // Le bloc RouterOS s'ouvre d'office si le profil édité porte un pool/queue.
  const [advancedOpen, setAdvancedOpen] = useState(
    () => Boolean(profile?.addressPool || profile?.parentQueue),
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
  const nameMissing = form.name.trim() === "";

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

  // Secousse douce du formulaire quand la soumission est refusée — guide
  // l'œil vers l'erreur signalée en toast, sans casser le focus clavier.
  async function shakeForm() {
    if (prefersReducedMotion) return;
    await shakeControls.start({
      x: [0, -10, 10, -6, 6, -2, 0],
      transition: { duration: 0.4, ease: "easeInOut" },
    });
  }

  function submitProfile() {
    setAttempted(true);
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
      void shakeForm();
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

  // ── Valeurs d'aperçu (toujours sûres à afficher) ──
  const previewName = form.name.trim() || t("profiles.dialog.previewNameFallback");
  const previewPrice = Number.isFinite(priceNum) && priceNum >= 0 ? priceNum : 0;
  const sellingDiffers = sellingNum > 0 && sellingNum !== previewPrice;
  const margin = sellingNum > previewPrice ? sellingNum - previewPrice : 0;
  const previewRate = form.rateLimit.trim() && !rateInvalid ? formatRateLimit(form.rateLimit.trim()) : "—";
  const previewSession = Number.isInteger(sessionNum) && sessionNum >= 1 ? formatDuration(sessionNum * 60) : "—";
  const previewValidity = Number.isInteger(validityNum) && validityNum >= 1 ? fmtRouterDuration(validityNum) : "—";
  const previewDevices =
    form.lockUser
      ? t("profiles.oneAtATime")
      : Number.isInteger(devicesNum) && devicesNum >= 1
        ? tf("profiles.devicesCount", { n: devicesNum })
        : "—";
  const previewQuota = Number.isInteger(quotaNum) && quotaNum > 0
    ? formatBytes(quotaNum * 1048576, lang)
    : t("profiles.unlimited");
  const expiryVisual: Record<ProfileExpiryMode, { icon: LucideIcon; labelKey: string }> = {
    none: { icon: InfinityIcon, labelKey: "profiles.dialog.expNoneTitle" },
    notify: { icon: Bell, labelKey: "profiles.dialog.expNotifyTitle" },
    remove: { icon: Trash2, labelKey: "profiles.dialog.expRemoveTitle" },
  };
  const ExpiryIcon = expiryVisual[form.expMode].icon;

  // Stepper d'appareils : garde-fous NaN / bornes.
  const devicesCur = Number.isInteger(devicesNum) ? devicesNum : 0;
  const deviceDec = () => setForm((f) => ({ ...f, sharedUsers: String(Math.max(1, devicesCur - 1)) }));
  const deviceInc = () => setForm((f) => ({ ...f, sharedUsers: String(Math.min(10, devicesCur + 1)) }));

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex h-[92dvh] max-w-[calc(100vw-1rem)] flex-col gap-0 overflow-hidden p-0 sm:h-auto sm:max-h-[min(88vh,780px)] sm:max-w-4xl">
        {/* ─── En-tête ─── */}
        <div className="border-b bg-gradient-to-r from-accent/50 via-transparent to-transparent px-5 py-4 sm:px-6">
          <div className="flex items-center gap-3 pr-8">
            <span className="tile-aurora grid size-10 shrink-0 place-items-center rounded-xl text-white" aria-hidden>
              <Gauge className="size-5" />
            </span>
            <div className="min-w-0">
              <DialogTitle className="truncate text-base font-semibold leading-tight">
                {profile ? tf("profiles.dialog.editTitle", { name: profile.name }) : t("profiles.dialog.newTitle")}
              </DialogTitle>
              <DialogDescription className="mt-0.5 line-clamp-2 text-xs leading-snug text-muted-foreground">
                {profile ? t("profiles.dialog.editDesc") : t("profiles.dialog.newDesc")}
              </DialogDescription>
            </div>
          </div>
        </div>

        {/* ─── Corps défilant : formulaire + aperçu live ─── */}
        <div className="min-h-0 flex-1 overflow-y-auto px-5 py-5 sm:px-6">
          <div className="mx-auto grid w-full max-w-3xl gap-6 lg:grid-cols-[minmax(0,1fr)_286px] lg:items-start">
            <motion.div animate={shakeControls} className="order-2 min-w-0 lg:order-1">
              <motion.form
                id="profile-form"
                variants={containerVariants}
                initial="hidden"
                animate="show"
                onSubmit={(event) => {
                  event.preventDefault();
                  submitProfile();
                }}
                className="grid gap-5"
              >
                {/* ─── Identité & débit ─── */}
                <motion.section variants={itemVariants} className="grid gap-3">
                  <SectionTitle icon={Wifi} title={t("profiles.dialog.secIdentity")} />
                  <div className="grid gap-2">
                    <Label htmlFor="profile-name">{t("common.name")}</Label>
                    <Input
                      id="profile-name"
                      placeholder={t("profiles.dialog.namePlaceholder")}
                      value={form.name}
                      onChange={(event) =>
                        // Parité Mikhmon (remSpace) : le nom de profil RouterOS ne
                        // contient pas d'espaces — ils deviennent des tirets.
                        setForm((f) => ({ ...f, name: event.target.value.replace(/\s+/g, "-") }))
                      }
                      disabled={saveMutation.isPending}
                      aria-invalid={attempted && nameMissing}
                      className={cn(attempted && nameMissing && "border-destructive focus-visible:ring-destructive/30")}
                    />
                  </div>
                  <div className="grid gap-2">
                    <Label htmlFor="profile-rate">{t("profiles.dialog.rate")}</Label>
                    <div className="relative">
                      <Input
                        id="profile-rate"
                        placeholder="2M/2M"
                        value={form.rateLimit}
                        onChange={(event) => setForm((f) => ({ ...f, rateLimit: event.target.value }))}
                        disabled={saveMutation.isPending}
                        aria-invalid={rateInvalid}
                        className={cn(
                          "font-mono pr-9",
                          rateInvalid && "border-destructive focus-visible:ring-destructive/30",
                        )}
                      />
                      {form.rateLimit.trim() !== "" && (
                        <span
                          className="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2"
                          aria-hidden
                        >
                          {rateInvalid ? (
                            <X className="size-4 text-destructive" />
                          ) : (
                            <Check className="size-4 text-primary" />
                          )}
                        </span>
                      )}
                    </div>
                    <div className="flex flex-wrap gap-1.5" role="group" aria-label={t("profiles.dialog.rate")}>
                      {RATE_PRESETS.map((rate) => (
                        <Button
                          key={rate}
                          type="button"
                          variant="outline"
                          size="sm"
                          className={cn(
                            "h-7 rounded-full px-2.5 font-mono text-xs",
                            form.rateLimit === rate
                              ? "border-primary bg-primary/10 text-primary"
                              : "text-muted-foreground",
                          )}
                          onClick={() => setForm((f) => ({ ...f, rateLimit: rate }))}
                          disabled={saveMutation.isPending}
                        >
                          {rate}
                        </Button>
                      ))}
                    </div>
                    <p className={rateInvalid ? "text-xs text-destructive" : "text-xs text-muted-foreground"}>
                      {rateInvalid ? t("profiles.dialog.rateInvalid") : t("profiles.dialog.rateHint")}
                    </p>
                  </div>
                </motion.section>

                {/* ─── Durées (session + validité) ─── */}
                <motion.section variants={itemVariants} className="grid gap-3">
                  <SectionTitle icon={Timer} title={t("profiles.dialog.secDurations")} />
                  <div className="grid gap-3 sm:grid-cols-2">
                    <div className="grid content-start gap-2 rounded-xl border p-3">
                      <Label htmlFor="profile-session">{t("profiles.dialog.session")}</Label>
                      <div className="flex gap-2">
                        <Input
                          id="profile-session"
                          type="number"
                          min={1}
                          value={form.sessionTimeoutValue}
                          onChange={(event) =>
                            setForm((f) => ({ ...f, sessionTimeoutValue: event.target.value }))
                          }
                          disabled={saveMutation.isPending}
                          className="min-w-0 flex-1"
                        />
                        <ToggleGroup
                          type="single"
                          variant="outline"
                          size="sm"
                          className="shrink-0"
                          value={form.sessionTimeoutUnit}
                          onValueChange={(value) =>
                            value && setForm((f) => ({ ...f, sessionTimeoutUnit: value as SessionUnit }))
                          }
                          aria-label={t("profiles.dialog.session")}
                        >
                          <ToggleGroupItem value="min">{t("profiles.dialog.unitMin")}</ToggleGroupItem>
                          <ToggleGroupItem value="hour">{t("profiles.dialog.unitHour")}</ToggleGroupItem>
                          <ToggleGroupItem value="day">{t("profiles.dialog.unitDay")}</ToggleGroupItem>
                        </ToggleGroup>
                      </div>
                      {form.sessionTimeoutUnit !== "min" && Number.isInteger(sessionNum) && (
                        <p className="text-xs tabular-nums text-muted-foreground">
                          {tf("profiles.dialog.sessionEq", { n: sessionNum })}
                        </p>
                      )}
                      <p className="text-[11px] leading-snug text-muted-foreground">
                        {t("profiles.dialog.sessionHint")}
                      </p>
                    </div>
                    <div className="grid content-start gap-2 rounded-xl border p-3">
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
                        <ToggleGroup
                          type="single"
                          variant="outline"
                          size="sm"
                          className="shrink-0"
                          value={form.validityUnit}
                          onValueChange={(value) =>
                            value && setForm((f) => ({ ...f, validityUnit: value as ValidityUnit }))
                          }
                          aria-label={t("profiles.dialog.validity")}
                        >
                          <ToggleGroupItem value="min">{t("profiles.dialog.unitMin")}</ToggleGroupItem>
                          <ToggleGroupItem value="hour">{t("profiles.dialog.unitHour")}</ToggleGroupItem>
                          <ToggleGroupItem value="day">{t("profiles.dialog.unitDay")}</ToggleGroupItem>
                          <ToggleGroupItem value="week">{t("profiles.dialog.unitWeek")}</ToggleGroupItem>
                        </ToggleGroup>
                      </div>
                      {form.validityUnit !== "min" && Number.isInteger(validityNum) && (
                        <p className="text-xs tabular-nums text-muted-foreground">
                          {tf("profiles.dialog.validityEq", { d: fmtRouterDuration(validityNum) })}
                        </p>
                      )}
                    </div>
                  </div>
                </motion.section>

                {/* ─── Appareils & données ─── */}
                <motion.section variants={itemVariants} className="grid gap-3">
                  <SectionTitle icon={Smartphone} title={t("profiles.dialog.secCaps")} />
                  <div className="grid gap-3 sm:grid-cols-2">
                    <div className="grid content-start gap-2 rounded-xl border p-3">
                      <Label htmlFor="profile-devices">{t("profiles.dialog.devices")}</Label>
                      {/* Stepper sur sa rangée propre : les chips de graduation ne
                          tiennent JAMAIS à côté dans la colonne formulaire
                          (~239 px utiles à lg) — sur la même ligne elles
                          débordaient de la carte et chevauchaient la carte
                          « Quota de données ». */}
                      <div className="flex items-center gap-2">
                        <Button
                          type="button"
                          variant="outline"
                          size="icon"
                          className="size-8 shrink-0"
                          onClick={deviceDec}
                          disabled={saveMutation.isPending || devicesCur <= 1}
                          aria-label={t("profiles.dialog.devicesMinus")}
                        >
                          <Minus className="size-4" />
                        </Button>
                        <motion.span
                          key={form.sharedUsers}
                          initial={{ scale: 0.8, opacity: 0 }}
                          animate={{ scale: 1, opacity: 1 }}
                          transition={{ duration: 0.15, ease: "easeOut" }}
                          className="w-10 text-center text-lg font-semibold tabular-nums"
                          aria-live="polite"
                        >
                          {form.sharedUsers || "–"}
                        </motion.span>
                        <Button
                          type="button"
                          variant="outline"
                          size="icon"
                          className="size-8 shrink-0"
                          onClick={deviceInc}
                          disabled={saveMutation.isPending || devicesCur >= 10}
                          aria-label={t("profiles.dialog.devicesPlus")}
                        >
                          <Plus className="size-4" />
                        </Button>
                        {/* Compteur live à droite du stepper (lit à l'écran via
                            l'aperçu, aria-hidden pour éviter le doublon SR). */}
                        <span className="ml-auto text-xs tabular-nums text-muted-foreground" aria-hidden>
                          {Number.isInteger(devicesNum) && devicesNum >= 1 && devicesNum <= 10
                            ? tf("profiles.devicesCount", { n: devicesNum })
                            : "—"}
                        </span>
                      </div>
                      <div className="flex flex-wrap gap-1.5" role="group" aria-label={t("profiles.dialog.devices")}>
                        {DEVICE_CHIPS.map((n) => (
                          <Button
                            key={n}
                            type="button"
                            variant="outline"
                            size="sm"
                            className={cn(
                              "h-7 w-9 rounded-full px-0 text-xs tabular-nums",
                              form.sharedUsers === String(n)
                                ? "border-primary bg-primary/10 text-primary"
                                : "text-muted-foreground",
                            )}
                            onClick={() => setForm((f) => ({ ...f, sharedUsers: String(n) }))}
                            disabled={saveMutation.isPending}
                          >
                            {n}
                          </Button>
                        ))}
                      </div>
                      {form.lockUser && (
                        <p className="text-[11px] leading-snug text-amber-600 dark:text-amber-400">
                          {t("profiles.dialog.lockUserDevices")}
                        </p>
                      )}
                    </div>
                    <div className="grid content-start gap-2 rounded-xl border p-3">
                      <Label htmlFor="profile-quota">{t("profiles.dialog.quota")}</Label>
                      <div className="flex flex-wrap gap-1.5" role="group" aria-label={t("profiles.dialog.quota")}>
                        {QUOTA_PRESETS.map((mb) => (
                          <Button
                            key={mb}
                            type="button"
                            variant="outline"
                            size="sm"
                            className={cn(
                              "h-7 rounded-full px-2.5 text-xs tabular-nums",
                              quotaNum === mb
                                ? "border-primary bg-primary/10 text-primary"
                                : "text-muted-foreground",
                            )}
                            onClick={() => setForm((f) => ({ ...f, dataQuotaMb: String(mb) }))}
                            disabled={saveMutation.isPending}
                          >
                            {mb === 0 ? t("profiles.unlimited") : tf("profiles.dialog.quotaGb", { n: mb / 1024 })}
                          </Button>
                        ))}
                      </div>
                      <div className="flex items-center gap-2">
                        <Input
                          id="profile-quota"
                          type="number"
                          min={0}
                          value={form.dataQuotaMb}
                          onChange={(event) => setForm((f) => ({ ...f, dataQuotaMb: event.target.value }))}
                          disabled={saveMutation.isPending}
                          className="min-w-0 flex-1"
                        />
                        <span className="shrink-0 text-xs text-muted-foreground">Mo</span>
                      </div>
                      <p className="text-[11px] leading-snug text-muted-foreground">{t("profiles.dialog.quotaHint")}</p>
                    </div>
                  </div>
                </motion.section>

                {/* ─── Tarifs ─── */}
                <motion.section variants={itemVariants} className="grid gap-3">
                  <SectionTitle icon={TrendingUp} title={t("profiles.dialog.pricing")} />
                  <div className="grid gap-3 sm:grid-cols-2">
                    <div className="grid gap-2">
                      <Label htmlFor="profile-price">{tf("profiles.dialog.price", { currency })}</Label>
                      <Input
                        id="profile-price"
                        type="number"
                        min={0}
                        value={form.price}
                        onChange={(event) => setForm((f) => ({ ...f, price: event.target.value }))}
                        disabled={saveMutation.isPending}
                      />
                      <p className="text-[11px] leading-snug text-muted-foreground">{t("profiles.dialog.priceHint")}</p>
                    </div>
                    <div className="grid gap-2">
                      <div className="flex items-center justify-between gap-2">
                        <Label htmlFor="profile-selling">{tf("profiles.dialog.selling", { currency })}</Label>
                        {margin > 0 && (
                          <motion.span
                            initial={{ opacity: 0, scale: 0.85 }}
                            animate={{ opacity: 1, scale: 1 }}
                            transition={{ duration: 0.2, ease: "easeOut" }}
                            className="inline-flex items-center gap-1 rounded-full bg-primary/10 px-2 py-0.5 text-[11px] font-medium tabular-nums text-primary"
                          >
                            <TrendingUp className="size-3" aria-hidden />
                            {tf("profiles.dialog.margin", { m: formatCurrency(margin, currency, lang) })}
                          </motion.span>
                        )}
                      </div>
                      <Input
                        id="profile-selling"
                        type="number"
                        min={0}
                        value={form.sellingPrice}
                        onChange={(event) => setForm((f) => ({ ...f, sellingPrice: event.target.value }))}
                        disabled={saveMutation.isPending}
                      />
                      <p className="text-[11px] leading-snug text-muted-foreground">
                        {t("profiles.dialog.sellingHintPre")}
                        <code className="font-mono text-[11px]">{"{{price}}"}</code>
                        {t("profiles.dialog.sellingHintPost")}
                      </p>
                    </div>
                  </div>
                </motion.section>

                {/* ─── Expiration ─── */}
                <motion.section variants={itemVariants} className="grid gap-3">
                  <SectionTitle icon={Bell} title={t("profiles.dialog.expiry")} />
                  <div
                    role="radiogroup"
                    aria-label={t("profiles.dialog.expMode")}
                    className="grid gap-2 sm:grid-cols-3"
                  >
                    <OptionCard
                      selected={form.expMode === "none"}
                      onClick={() => setForm((f) => ({ ...f, expMode: "none" }))}
                      icon={InfinityIcon}
                      title={t("profiles.dialog.expNoneTitle")}
                      desc={t("profiles.dialog.expNoneDesc")}
                    />
                    <OptionCard
                      selected={form.expMode === "notify"}
                      onClick={() => setForm((f) => ({ ...f, expMode: "notify" }))}
                      icon={Bell}
                      title={t("profiles.dialog.expNotifyTitle")}
                      desc={t("profiles.dialog.expNotifyDesc")}
                    />
                    <OptionCard
                      selected={form.expMode === "remove"}
                      onClick={() => setForm((f) => ({ ...f, expMode: "remove" }))}
                      icon={Trash2}
                      title={t("profiles.dialog.expRemoveTitle")}
                      desc={t("profiles.dialog.expRemoveDesc")}
                    />
                  </div>
                  {form.expMode !== "none" && (
                    <div className="grid gap-2 sm:max-w-xs">
                      <Label htmlFor="profile-grace">{t("profiles.dialog.grace")}</Label>
                      <Input
                        id="profile-grace"
                        type="number"
                        min={0}
                        max={GRACE_MAX}
                        value={form.gracePeriodMin}
                        onChange={(event) => setForm((f) => ({ ...f, gracePeriodMin: event.target.value }))}
                        disabled={saveMutation.isPending}
                      />
                      <p className="text-[11px] leading-snug text-muted-foreground">
                        {tf("profiles.dialog.graceHint", { n: GRACE_MAX })}
                      </p>
                    </div>
                  )}
                </motion.section>

                {/* ─── Sécurité & anti-partage ─── */}
                <motion.section variants={itemVariants} className="grid gap-2">
                  <SectionTitle icon={ShieldCheck} title={t("profiles.dialog.secSecurity")} />
                  <div
                    className={cn(
                      "flex items-center justify-between gap-3 rounded-xl border p-3 transition-colors",
                      form.lockUser ? "border-primary/50 bg-primary/5" : "hover:bg-accent/30",
                    )}
                  >
                    <div className="flex min-w-0 items-center gap-3">
                      <span
                        className={cn(
                          "grid size-8 shrink-0 place-items-center rounded-lg",
                          form.lockUser ? "bg-primary/10 text-primary" : "bg-muted text-muted-foreground",
                        )}
                        aria-hidden
                      >
                        <Lock className="size-4" />
                      </span>
                      <div className="min-w-0">
                        <Label htmlFor="profile-lock" className="text-sm">
                          {t("profiles.dialog.lock")}
                        </Label>
                        <p className="text-[11px] leading-snug text-muted-foreground">
                          {t("profiles.dialog.lockHint")}
                        </p>
                      </div>
                    </div>
                    <Switch
                      id="profile-lock"
                      checked={form.lockUser}
                      onCheckedChange={(checked) => setForm((f) => ({ ...f, lockUser: checked }))}
                      disabled={saveMutation.isPending}
                    />
                  </div>
                  <div
                    className={cn(
                      "flex items-center justify-between gap-3 rounded-xl border p-3 transition-colors",
                      form.lockFirstDevice ? "border-primary/50 bg-primary/5" : "hover:bg-accent/30",
                    )}
                  >
                    <div className="flex min-w-0 items-center gap-3">
                      <span
                        className={cn(
                          "grid size-8 shrink-0 place-items-center rounded-lg",
                          form.lockFirstDevice ? "bg-primary/10 text-primary" : "bg-muted text-muted-foreground",
                        )}
                        aria-hidden
                      >
                        <MonitorSmartphone className="size-4" />
                      </span>
                      <div className="min-w-0">
                        <Label htmlFor="profile-lock-device" className="text-sm">
                          {t("profiles.dialog.lockDevice")}
                        </Label>
                        <p className="text-[11px] leading-snug text-muted-foreground">
                          {t("profiles.dialog.lockDeviceHint")}
                        </p>
                      </div>
                    </div>
                    <Switch
                      id="profile-lock-device"
                      checked={form.lockFirstDevice}
                      onCheckedChange={(checked) => setForm((f) => ({ ...f, lockFirstDevice: checked }))}
                      disabled={saveMutation.isPending}
                    />
                  </div>
                </motion.section>

                {/* ─── RouterOS avancé (parité Mikhmon, replié par défaut) ─── */}
                <motion.section variants={itemVariants}>
                  <Collapsible open={advancedOpen} onOpenChange={setAdvancedOpen}>
                    <CollapsibleTrigger asChild>
                      <Button
                        type="button"
                        variant="outline"
                        className="w-full justify-between rounded-xl px-3 font-normal"
                        aria-expanded={advancedOpen}
                      >
                        <span className="flex items-center gap-2 text-sm">
                          <RouterIcon className="size-4 text-muted-foreground" aria-hidden />
                          {t("profiles.dialog.routeros")}
                        </span>
                        <ChevronDown
                          className={cn("size-4 text-muted-foreground transition-transform", advancedOpen && "rotate-180")}
                          aria-hidden
                        />
                      </Button>
                    </CollapsibleTrigger>
                    <CollapsibleContent>
                      <div className="mt-2 grid gap-3 rounded-xl border p-3">
                        <div className="grid gap-2">
                          <Label htmlFor="profile-router-source">{t("profiles.dialog.routerSource")}</Label>
                          <Select value={sourceRouterId} onValueChange={setSourceRouterId}>
                            <SelectTrigger
                              id="profile-router-source"
                              className="h-10 w-full"
                              aria-label={t("profiles.dialog.routerSource")}
                            >
                              <SelectValue />
                            </SelectTrigger>
                            <SelectContent>
                              <SelectItem value="all">{t("profiles.dialog.routerSourceAll")}</SelectItem>
                              {eligibleRouters.map((router) => (
                                <SelectItem key={router.id} value={router.id}>
                                  {router.name}
                                </SelectItem>
                              ))}
                            </SelectContent>
                          </Select>
                          {resources.data?.queued && resourcesEmpty && (
                            <p className="text-xs text-muted-foreground">
                              {t("profiles.dialog.routerSourcePending")}
                            </p>
                          )}
                        </div>
                        <div className="grid gap-3 sm:grid-cols-2">
                          <div className="grid gap-2">
                            <Label htmlFor="profile-pool">{t("profiles.dialog.pool")}</Label>
                            <Input
                              id="profile-pool"
                              placeholder="pool-hotspot-1"
                              className="font-mono"
                              list="profile-pool-options"
                              value={form.addressPool}
                              onChange={(event) => setForm((f) => ({ ...f, addressPool: event.target.value }))}
                              disabled={saveMutation.isPending}
                            />
                            <datalist id="profile-pool-options">
                              {resources.data?.pools.map((p) => (
                                <option key={p} value={p} />
                              ))}
                            </datalist>
                            <p className="text-[11px] leading-snug text-muted-foreground">
                              {t("profiles.dialog.poolHint")}
                            </p>
                          </div>
                          <div className="grid gap-2">
                            <Label htmlFor="profile-queue">{t("profiles.dialog.queue")}</Label>
                            <Input
                              id="profile-queue"
                              placeholder="q-parent-vip"
                              className="font-mono"
                              list="profile-queue-options"
                              value={form.parentQueue}
                              onChange={(event) => setForm((f) => ({ ...f, parentQueue: event.target.value }))}
                              disabled={saveMutation.isPending}
                            />
                            <datalist id="profile-queue-options">
                              {resources.data?.queues.map((q) => (
                                <option key={q} value={q} />
                              ))}
                            </datalist>
                            <p className="text-[11px] leading-snug text-muted-foreground">
                              {t("profiles.dialog.queueHint")}
                            </p>
                          </div>
                        </div>
                      </div>
                    </CollapsibleContent>
                  </Collapsible>
                </motion.section>
              </motion.form>
            </motion.div>

            {/* ─── Aperçu live (carte forfait aurora) ─── */}
            <motion.aside
              initial={{ opacity: 0, y: 14 }}
              animate={{ opacity: 1, y: 0, transition: { duration: 0.3, delay: 0.18, ease: "easeOut" } }}
              className="order-1 self-start lg:order-2 lg:sticky lg:top-0"
              aria-label={t("profiles.dialog.previewLabel")}
            >
              <p className="mb-2 flex items-center gap-1.5 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">
                <Eye className="size-3.5" aria-hidden />
                {t("profiles.dialog.previewLabel")}
              </p>
              <div className="bg-aurora relative overflow-hidden rounded-2xl p-5 text-white shadow-lg">
                {/* Halos décoratifs + liseré intérieur */}
                <div aria-hidden className="pointer-events-none absolute -right-8 -top-10 size-32 rounded-full bg-white/15 blur-2xl" />
                <div aria-hidden className="pointer-events-none absolute -bottom-12 -left-6 size-28 rounded-full bg-white/10 blur-xl" />
                <div aria-hidden className="pointer-events-none absolute inset-0 rounded-2xl ring-1 ring-inset ring-white/20" />
                <div className="relative">
                  <p className="text-[11px] font-medium uppercase tracking-wider text-white/70">
                    {t("profiles.dialog.previewKind")}
                  </p>
                  <motion.p
                    key={previewName}
                    initial={{ opacity: 0, y: 4 }}
                    animate={{ opacity: 1, y: 0 }}
                    transition={{ duration: 0.2, ease: "easeOut" }}
                    className="mt-0.5 truncate text-lg font-bold leading-tight"
                  >
                    {previewName}
                  </motion.p>
                  <div className="mt-3 flex items-end gap-2">
                    <motion.span
                      key={previewPrice}
                      initial={{ opacity: 0, y: 4 }}
                      animate={{ opacity: 1, y: 0 }}
                      transition={{ duration: 0.2, ease: "easeOut" }}
                      className="text-2xl font-bold tabular-nums"
                    >
                      {formatCurrency(previewPrice, currency, lang)}
                    </motion.span>
                    <span className="pb-0.5 text-xs text-white/70">{t("profiles.perPlan")}</span>
                  </div>
                  {sellingDiffers && (
                    <motion.span
                      key={sellingNum}
                      initial={{ opacity: 0, y: 4 }}
                      animate={{ opacity: 1, y: 0 }}
                      transition={{ duration: 0.2, ease: "easeOut" }}
                      className="mt-1.5 inline-flex rounded-full bg-white/15 px-2 py-0.5 text-[11px] font-medium tabular-nums"
                    >
                      {tf("profiles.saleAt", { price: formatCurrency(sellingNum, currency, lang) })}
                    </motion.span>
                  )}
                  <div className="mt-4 grid grid-cols-2 gap-x-3 gap-y-2.5 border-t border-white/20 pt-3.5 text-xs">
                    <div className="min-w-0">
                      <p className="flex items-center gap-1.5 text-white/70">
                        <ArrowDownUp className="size-3.5 shrink-0" aria-hidden />
                        {t("profiles.bandwidth")}
                      </p>
                      <p className="mt-0.5 truncate font-semibold tabular-nums">{previewRate}</p>
                    </div>
                    <div className="min-w-0">
                      <p className="flex items-center gap-1.5 text-white/70">
                        <Timer className="size-3.5 shrink-0" aria-hidden />
                        {t("profiles.session")}
                      </p>
                      <p className="mt-0.5 truncate font-semibold tabular-nums">{previewSession}</p>
                    </div>
                    <div className="min-w-0">
                      <p className="flex items-center gap-1.5 text-white/70">
                        <CalendarClock className="size-3.5 shrink-0" aria-hidden />
                        {t("profiles.validity")}
                      </p>
                      <p className="mt-0.5 truncate font-semibold tabular-nums">{previewValidity}</p>
                    </div>
                    <div className="min-w-0">
                      <p className="flex items-center gap-1.5 text-white/70">
                        <Smartphone className="size-3.5 shrink-0" aria-hidden />
                        {form.lockUser ? t("profiles.sessionsLabel") : t("profiles.devices")}
                      </p>
                      <p className="mt-0.5 truncate font-semibold tabular-nums">{previewDevices}</p>
                    </div>
                    <div className="min-w-0">
                      <p className="flex items-center gap-1.5 text-white/70">
                        <Database className="size-3.5 shrink-0" aria-hidden />
                        {t("profiles.quota")}
                      </p>
                      <p className="mt-0.5 truncate font-semibold tabular-nums">{previewQuota}</p>
                    </div>
                    <div className="min-w-0">
                      <p className="flex items-center gap-1.5 text-white/70">
                        <CalendarClock className="size-3.5 shrink-0" aria-hidden />
                        {t("profiles.expiration")}
                      </p>
                      <p className="mt-0.5 flex items-center gap-1 truncate font-semibold">
                        <ExpiryIcon className="size-3.5 shrink-0" aria-hidden />
                        {t(expiryVisual[form.expMode].labelKey)}
                      </p>
                    </div>
                  </div>
                  {(form.lockUser || form.lockFirstDevice) && (
                    <div className="mt-3 flex flex-wrap gap-1.5 border-t border-white/20 pt-3">
                      {form.lockUser && (
                        <span className="inline-flex items-center gap-1 rounded-full bg-white/15 px-2 py-0.5 text-[11px] font-medium">
                          <Lock className="size-3" aria-hidden />
                          {t("profiles.oneAtATime")}
                        </span>
                      )}
                      {form.lockFirstDevice && (
                        <span className="inline-flex items-center gap-1 rounded-full bg-white/15 px-2 py-0.5 text-[11px] font-medium">
                          <MonitorSmartphone className="size-3" aria-hidden />
                          {t("profiles.dialog.lockDeviceShort")}
                        </span>
                      )}
                    </div>
                  )}
                </div>
              </div>
              <p className="mt-2 text-xs leading-snug text-muted-foreground">
                {t("profiles.dialog.previewCaption")}
              </p>
            </motion.aside>
          </div>
        </div>

        {/* ─── Pied collant ─── */}
        <div className="flex items-center gap-3 border-t bg-muted/30 px-5 py-3.5 sm:px-6">
          {saveMutation.isError && (
            <p className="hidden min-w-0 flex-1 truncate text-sm text-destructive sm:block" role="alert">
              {saveMutation.error.message}
            </p>
          )}
          <div className="ml-auto flex items-center gap-2">
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
              disabled={saveMutation.isPending}
            >
              {t("common.cancel")}
            </Button>
            <Button type="submit" form="profile-form" disabled={saveMutation.isPending} className="min-w-36">
              {saveMutation.isPending && <Loader2 className="size-4 animate-spin" aria-hidden />}
              {profile ? t("common.save") : t("profiles.dialog.createSubmit")}
            </Button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
