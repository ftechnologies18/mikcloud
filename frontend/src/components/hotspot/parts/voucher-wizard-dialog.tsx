"use client";

// Wizard de création de vouchers — 3 étapes animées (Forfait → Codes → Récap).
// Remplace l'ancien formulaire à onglets (Général/Limites) en guidant pas à pas :
//   1. Forfait  : profil (cartes), quantité (stepper + raccourcis), routeur (+ serveur) ;
//   2. Codes    : aperçu live d'échantillons (alphabets fidèles au backend — models.go
//                 CharsetAlphabets, sans 0/1/I/L/O), type de connexion, jeu de caractères,
//                 longueur (slider), préfixe, limites avancées (Time Limit / quota data,
//                 parité Mikhmon) avec repères FCFA cliquables ;
//   3. Récap    : revendeur (jauge de crédit animée), récapitulatif complet, ticket
//                 miniature en direct, coût total animé → Générer.
// Animations (transitions d'étape, pop des sélections, compteur) : section
// « Wizard vouchers » de globals.css — respecte prefers-reduced-motion.
// Le payload construit est identique à l'ancien submitGenerate (contrat backend inchangé).

import { useEffect, useMemo, useRef, useState } from "react";
import {
  AlertTriangle,
  Check,
  ChevronLeft,
  ChevronRight,
  ClipboardCheck,
  Clock,
  Database,
  Fingerprint,
  Gauge,
  Hash,
  Layers,
  Loader2,
  Lock,
  QrCode,
  RefreshCcw,
  Route,
  ShieldCheck,
  Sparkles,
  Ticket,
  Wallet,
  Wifi,
  Zap,
} from "lucide-react";

import { Badge } from "@/components/ui/badge";
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
import { Separator } from "@/components/ui/separator";
import { Slider } from "@/components/ui/slider";
import { useI18n, localeOf } from "@/lib/hotspot/i18n";
import {
  formatBytes,
  formatCurrency,
  formatDuration,
  fmtRouterDuration,
} from "@/lib/hotspot/format";
import { useRouterResources } from "@/lib/hotspot/use-router-resources";
import type {
  GenerateVouchersRequest,
  Profile,
  Reseller,
  RouterDevice,
} from "@/lib/hotspot/types";
import { cn } from "@/lib/utils";

/* ────────────────────────────── Constantes ────────────────────────────── */

const CODE_LENGTH_MIN = 3;
const CODE_LENGTH_MAX = 10;
const COUNT_CHIPS = [10, 25, 50, 100, 200];

// Miroir de backend/internal/model/models.go (CharsetAlphabets) — aperçu fidèle :
// tous les alphabets excluent 0/1/I/L/O (lisibilité sur ticket imprimé).
const CHARSET_ALPHABETS: Record<string, string> = {
  "": "23456789ABCDEFGHJKMNPQRSTUVWXYZ", // « mikcloud » (défaut sûr)
  abc: "abcdefghijkmnpqrstuvwxyz",
  ABC: "ABCDEFGHJKMNPQRSTUVWXYZ",
  aBc: "abcdefghijkmnpqrstuvwxyzABCDEFGHJKMNPQRSTUVWXYZ",
  "5ab": "23456789abcdefghijkmnpqrstuvwxyz",
  "5AB": "23456789ABCDEFGHJKMNPQRSTUVWXYZ",
  "5aB": "23456789abcdefghijkmnpqrstuvwxyzABCDEFGHJKMNPQRSTUVWXYZ",
  num: "23456789",
};

// Pilules de charset : preset à la User Manager + sous-titre localisé.
const CHARSET_PILLS = [
  { value: "mikcloud", name: "MikCloud", subKey: "vouchers.wizard.charsetSub.mikcloud", star: true },
  { value: "abc", name: "abcd", subKey: "vouchers.wizard.charsetSub.abc" },
  { value: "ABC", name: "ABCD", subKey: "vouchers.wizard.charsetSub.ABC" },
  { value: "aBc", name: "aBcD", subKey: "vouchers.wizard.charsetSub.aBc" },
  { value: "5ab", name: "5ab2c34d", subKey: "vouchers.wizard.charsetSub.5ab" },
  { value: "5AB", name: "5AB2C34D", subKey: "vouchers.wizard.charsetSub.5AB" },
  { value: "5aB", name: "5aB2c34D", subKey: "vouchers.wizard.charsetSub.5aB" },
  { value: "num", name: "1234", subKey: "vouchers.wizard.charsetSub.num" },
];

// Quotas courants (1 Go = 1024 Mo) — parité avec l'ancien formulaire.
const QUOTA_OPTIONS = [
  { value: "0", mb: 0 },
  { value: "512", mb: 512 },
  { value: "1024", mb: 1024 },
  { value: "2048", mb: 2048 },
  { value: "5120", mb: 5120 },
  { value: "10240", mb: 10240 },
  { value: "20480", mb: 20480 },
  { value: "51200", mb: 51200 },
];

// Time Limit (limit-uptime) par lot — minutes, libellés RouterOS au rendu.
const TIME_LIMIT_OPTIONS = [
  { value: "0", min: 0 },
  { value: "30", min: 30 },
  { value: "60", min: 60 },
  { value: "120", min: 120 },
  { value: "180", min: 180 },
  { value: "300", min: 300 },
  { value: "720", min: 720 },
  { value: "1440", min: 1440 },
  { value: "2880", min: 2880 },
  { value: "4320", min: 4320 },
  { value: "10080", min: 10080 },
  { value: "43200", min: 43200 },
];

// Repères de prix FCFA — cliquables : appliquent le quota correspondant.
const QUOTA_PRICE_HINTS: [number, number][] = [
  [1024, 100],
  [2048, 200],
  [5120, 500],
  [10240, 1000],
  [30720, 3000],
];

// Libellés du mode d'expiration (récap profil), parité avec l'ancien formulaire.
const EXP_MODE_LABEL_KEY: Record<string, string> = {
  none: "profiles.dialog.expNone",
  notify: "profiles.dialog.expNotify",
  remove: "profiles.dialog.expRemove",
};

const STEPS = [
  { n: 1, titleKey: "vouchers.wizard.step1", icon: Ticket },
  { n: 2, titleKey: "vouchers.wizard.step2", icon: Sparkles },
  { n: 3, titleKey: "vouchers.wizard.step3", icon: ClipboardCheck },
] as const;

/* ──────────────────────────── Helper interne ──────────────────────────── */

/** Compteur animé (ease-out cubic ~380 ms) — coût total, crédit revendeur. */
function AnimatedNumber({ value, format }: { value: number; format: (n: number) => string }) {
  const [display, setDisplay] = useState(value);
  const prevRef = useRef(value);

  useEffect(() => {
    const from = prevRef.current;
    prevRef.current = value;
    if (from === value) return;
    const start = performance.now();
    const duration = 380;
    let raf = 0;
    const tick = (now: number) => {
      const p = Math.min(1, (now - start) / duration);
      const eased = 1 - Math.pow(1 - p, 3);
      setDisplay(Math.round(from + (value - from) * eased));
      if (p < 1) raf = requestAnimationFrame(tick);
    };
    raf = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(raf);
  }, [value]);

  return <span className="tabular-nums">{format(display)}</span>;
}

/* ─────────────────────────────── Composant ─────────────────────────────── */

interface VoucherWizardDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  profiles: Profile[];
  routers: RouterDevice[];
  resellers: Reseller[];
  currency: string;
  tenantName: string;
  isPending: boolean;
  isError: boolean;
  error: Error | null;
  onSubmit: (payload: GenerateVouchersRequest) => void;
}

export function VoucherWizardDialog({
  open,
  onOpenChange,
  profiles,
  routers,
  resellers,
  currency,
  tenantName,
  isPending,
  isError,
  error,
  onSubmit,
}: VoucherWizardDialogProps) {
  const { t, tf, lang } = useI18n();

  // Formatage local (repères FCFA + quotas), comme l'ancien formulaire.
  const fmtQuota = (mb: number) =>
    mb < 1024 ? tf("vouchers.gen.quota.mb", { n: mb }) : tf("vouchers.gen.quota.gb", { n: mb / 1024 });
  const fmtFcfa = (amount: number) =>
    `${new Intl.NumberFormat(localeOf(lang)).format(amount)} F`;
  const fmtMoney = (amount: number) => formatCurrency(amount, currency, lang);

  // Étape courante + direction (transition avant/arrière).
  const [step, setStep] = useState(1);
  const [dir, setDir] = useState<"fwd" | "back">("fwd");

  // État du formulaire — conservé entre deux ouvertures (comme avant).
  const [profileId, setProfileId] = useState("");
  const [routerId, setRouterId] = useState("");
  const [count, setCount] = useState("10");
  const [server, setServer] = useState("all");
  // N°25 — VERROU : le mode « nom d'utilisateur = mot de passe » est imposé.
  // L'ancien choix userpass/same a été retiré — l'API force aussi "same" côté serveur.
  const [charset, setCharset] = useState("mikcloud");
  const [codeLength, setCodeLength] = useState(6);
  const [prefix, setPrefix] = useState("");
  const [comment, setComment] = useState("");
  const [resellerId, setResellerId] = useState("none");
  const [timeLimit, setTimeLimit] = useState("inherit");
  const [quotaMb, setQuotaMb] = useState("inherit");
  const [advancedOpen, setAdvancedOpen] = useState(false);

  // Échantillons de codes (aperçu) — re-tirés via seed.
  const [seed, setSeed] = useState(0);
  const [spin, setSpin] = useState(false);

  // Retour à l'étape 1 à la FERMETURE (reset en handler, pas en effet — les
  // champs du formulaire restent mémorisés d'une ouverture à l'autre).
  function handleOpenChange(next: boolean) {
    onOpenChange(next);
    if (!next) {
      setStep(1);
      setDir("fwd");
    }
  }

  // Ressources du routeur sélectionné (serveurs hotspot — parité Mikhmon).
  const resources = useRouterResources(routers, routerId || undefined);
  const servers = resources.data?.servers ?? [];

  /* ── Dérivés (coût, crédit, quotas) ── */

  const countNum = parseInt(count, 10);
  const countValid = Number.isInteger(countNum) && countNum >= 1 && countNum <= 500;
  const profile = profiles.find((p) => p.id === profileId);
  const router = routers.find((r) => r.id === routerId);
  const reseller = resellers.find((r) => r.id === resellerId);

  const unitPrice = profile?.price ?? 0;
  const totalCost = (countValid ? countNum : 0) * unitPrice;
  const creditAfter = reseller ? reseller.credit - totalCost : null;
  const insufficient = creditAfter !== null && creditAfter < 0;
  // Ratio de crédit restant après génération (jauge).
  const creditRatio =
    reseller && reseller.credit > 0 && creditAfter !== null
      ? Math.max(0, Math.min(100, (creditAfter / reseller.credit) * 100))
      : null;

  const effectiveQuotaMb = quotaMb === "inherit" ? (profile?.dataQuotaMb ?? 0) : Number(quotaMb);
  const quotaLabel =
    effectiveQuotaMb > 0
      ? formatBytes(effectiveQuotaMb * 1048576, lang)
      : quotaMb === "inherit" && !profile
        ? t("vouchers.quotaWillInherit")
        : t("vouchers.quotaUnlimitedShort");

  const timeLimitLabel =
    timeLimit === "inherit"
      ? profile
        ? profile.sessionTimeoutMin > 0
          ? fmtRouterDuration(profile.sessionTimeoutMin)
          : t("vouchers.gen.timeLimitUnlimited")
        : "—"
      : Number(timeLimit) === 0
        ? t("vouchers.gen.timeLimitUnlimited")
        : fmtRouterDuration(Number(timeLimit));

  const advancedTouched = timeLimit !== "inherit" || quotaMb !== "inherit";

  /* ── Validité par étape ── */

  const step1Valid = countValid && !!profile && !!router;
  const stepValid = step === 1 ? step1Valid : true;
  const canSubmit = step1Valid && !insufficient && !isPending;

  const nextHint = !countValid
    ? t("vouchers.wizard.hintQty")
    : !profile
      ? t("vouchers.wizard.hintProfile")
      : !router
        ? t("vouchers.wizard.hintRouter")
        : insufficient
          ? t("vouchers.wizard.hintCredit")
          : "";

  /* ── Navigation ── */

  const headingRef = useRef<HTMLParagraphElement>(null);
  // Horodatage du dernier changement d'étape — les actions (Continuer / Générer)
  // sont ignorées pendant la fenêtre de transition : le clic qui provoque le
  // passage à l'étape suivante ne doit jamais « traverser » vers l'action du
  // pied de page qui prend sa place (Générer), même en cas de rejeu du clic.
  const lastNavRef = useRef(0);
  const NAV_GRACE_MS = 300;
  function go(next: number) {
    setDir(next > step ? "fwd" : "back");
    setStep(next);
    lastNavRef.current = Date.now();
  }
  // Focus du titre d'étape (accessibilité clavier / lecteurs d'écran).
  useEffect(() => {
    if (open) headingRef.current?.focus();
  }, [step, open]);

  function tryNext() {
    if (Date.now() - lastNavRef.current < NAV_GRACE_MS) return;
    if (!stepValid) return;
    go(Math.min(3, step + 1));
  }

  function submit() {
    if (Date.now() - lastNavRef.current < NAV_GRACE_MS) return;
    if (!canSubmit) return;
    // Payload identique à l'ancien submitGenerate (contrat backend inchangé).
    onSubmit({
      count: countNum,
      profileId,
      routerId,
      server: server !== "all" ? server : undefined,
      prefix: prefix.trim() || undefined,
      codeLength,
      resellerId: resellerId === "none" ? undefined : resellerId,
      userMode: "same", // N°25 — verrou : code unique (nom = mot de passe)
      charset: charset !== "mikcloud" ? charset : undefined,
      comment: comment.trim() || undefined,
      dataQuotaMb: quotaMb === "inherit" ? undefined : Number(quotaMb),
      timeLimitMin: timeLimit === "inherit" ? undefined : Number(timeLimit),
    });
  }

  function reshuffle() {
    setSeed((s) => s + 1);
    setSpin(true);
    window.setTimeout(() => setSpin(false), 500);
  }

  /* ── Échantillons de codes (aperçu live, fidèle aux alphabets serveur) ── */

  const samples = useMemo(() => {
    const alphabet = CHARSET_ALPHABETS[charset === "mikcloud" ? "" : charset] ?? CHARSET_ALPHABETS[""];
    const rnd = () =>
      Array.from(
        { length: codeLength },
        () => alphabet[Math.floor(Math.random() * alphabet.length)],
      ).join("");
    return Array.from({ length: 3 }, () => ({
      code: rnd(),
      pass: "", // N°25 — code unique : le mot de passe EST le code
    }));
    // seed volontaire dans les deps : re-tirage à la demande.
  }, [charset, codeLength, seed]);

  const profileValidity = (p: Profile) =>
    fmtRouterDuration(p.validityMin > 0 ? p.validityMin : p.validityDays * 1440);
  const profileSalePrice = (p: Profile) => (p.sellingPrice > 0 ? p.sellingPrice : p.price);

  const modeBadge = (r: RouterDevice) =>
    r.mode === "agent"
      ? t("vouchers.wizard.modeAgent")
      : r.mode === "simulated"
        ? t("vouchers.wizard.modeSimulated")
        : t("vouchers.wizard.modeReal");

  const formatRecap = tf("vouchers.wizard.recapFormatValue", {
    n: codeLength,
    charset: charset === "mikcloud" ? "MikCloud" : CHARSET_PILLS.find((c) => c.value === charset)?.name ?? charset,
    mode: t("vouchers.wizard.modeSameShort"),
  });

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="gap-0 overflow-hidden p-0 sm:max-w-2xl">
        {/* ── En-tête dégradé + stepper ── */}
        <div className="relative border-b bg-gradient-to-br from-primary/15 via-primary/5 to-transparent px-6 pb-4 pt-6">
          <div
            aria-hidden
            className="pointer-events-none absolute -right-10 -top-16 size-44 rounded-full bg-primary/15 blur-3xl"
          />
          <div
            aria-hidden
            className="pointer-events-none absolute -left-14 -bottom-20 size-40 rounded-full bg-primary/10 blur-3xl"
          />
          <div className="relative">
            <DialogTitle className="text-xl font-bold">{t("vouchers.generateTitle")}</DialogTitle>
            <DialogDescription>{t("vouchers.generateDesc")}</DialogDescription>

            {/* Stepper — cercles + connecteurs animés */}
            <ol className="mt-5 flex items-center gap-2" aria-label={t("vouchers.wizard.stepsAria")}>
              {STEPS.map((s, i) => {
                const Icon = s.icon;
                const done = step > s.n;
                const current = step === s.n;
                return (
                  <li
                    key={s.n}
                    className="flex flex-1 items-center gap-2"
                    aria-current={current ? "step" : undefined}
                  >
                    <button
                      type="button"
                      onClick={() => done && go(s.n)}
                      disabled={!done}
                      className={cn(
                        "group flex min-w-0 items-center gap-2 rounded-full focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary",
                        done && "cursor-pointer",
                      )}
                      aria-label={tf("vouchers.wizard.stepAria", { n: s.n, title: t(s.titleKey) })}
                    >
                      <span
                        className={cn(
                          "flex size-8 shrink-0 items-center justify-center rounded-full border-2 text-xs font-bold transition-all duration-300",
                          current &&
                            "scale-110 border-primary bg-primary text-primary-foreground shadow-lg shadow-primary/30",
                          done && "border-primary/40 bg-primary/15 text-primary",
                          !current && !done && "border-border bg-background text-muted-foreground",
                        )}
                      >
                        {done ? (
                          <Check className="wizard-pop size-4" aria-hidden />
                        ) : (
                          <Icon className="size-4" aria-hidden />
                        )}
                      </span>
                      <span
                        className={cn(
                          "hidden truncate text-sm font-semibold sm:block",
                          current ? "text-foreground" : "text-muted-foreground",
                        )}
                      >
                        {t(s.titleKey)}
                      </span>
                    </button>
                    {i < STEPS.length - 1 && (
                      <span aria-hidden className="relative h-1 flex-1 overflow-hidden rounded-full bg-border">
                        <span
                          className="absolute inset-0 origin-left rounded-full bg-gradient-to-r from-primary to-primary/50 transition-transform duration-500 ease-out"
                          style={{ transform: `scaleX(${step > s.n ? 1 : 0})` }}
                        />
                      </span>
                    )}
                  </li>
                );
              })}
            </ol>
          </div>
        </div>

        {/* ── Corps : étape courante (transition animée) ── */}
        <form
          className="grid grid-rows-[1fr_auto]"
          onSubmit={(event) => {
            event.preventDefault();
            if (step < 3) tryNext();
            else submit();
          }}
        >
          <div className="max-h-[58vh] overflow-y-auto px-5 py-5 sm:px-6">
            <div key={step} className={dir === "back" ? "wizard-step-back" : "wizard-step-fwd"}>
              <p
                ref={headingRef}
                tabIndex={-1}
                className="mb-4 flex items-center gap-2 text-sm font-semibold text-primary focus:outline-none"
              >
                {step === 1 && <Ticket className="size-4" aria-hidden />}
                {step === 2 && <Sparkles className="size-4" aria-hidden />}
                {step === 3 && <ClipboardCheck className="size-4" aria-hidden />}
                {tf("vouchers.wizard.stepTitle", { n: step, title: t(STEPS[step - 1].titleKey) })}
              </p>

              {/* ════════ ÉTAPE 1 — FORFAIT ════════ */}
              {step === 1 && (
                <div className="grid gap-5">
                  {/* Profils — cartes sélectionnables */}
                  <section aria-label={t("vouchers.wizard.chooseProfile")}>
                    {profiles.length === 0 ? (
                      <p className="rounded-xl border border-dashed p-4 text-sm text-muted-foreground">
                        {t("vouchers.wizard.noProfiles")}
                      </p>
                    ) : (
                      <div
                        role="radiogroup"
                        aria-label={t("vouchers.wizard.chooseProfile")}
                        className="grid max-h-64 gap-2 overflow-y-auto pr-1 sm:grid-cols-2"
                      >
                        {profiles.map((p) => {
                          const selected = p.id === profileId;
                          return (
                            <button
                              key={p.id}
                              type="button"
                              role="radio"
                              aria-checked={selected}
                              onClick={() => setProfileId(p.id)}
                              className={cn(
                                "wizard-card relative rounded-xl border-2 p-3 text-left transition-all duration-200 hover:-translate-y-0.5 hover:shadow-md focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary",
                                selected
                                  ? "border-primary bg-gradient-to-br from-primary/10 to-transparent shadow-md shadow-primary/10"
                                  : "border-border bg-card",
                              )}
                            >
                              {selected && (
                                <span
                                  className="wizard-pop absolute right-2 top-2 flex size-5 items-center justify-center rounded-full bg-primary text-primary-foreground"
                                  aria-hidden
                                >
                                  <Check className="size-3" />
                                </span>
                              )}
                              <p className="truncate pr-6 text-sm font-bold">{p.name}</p>
                              <p className="mt-0.5 text-lg font-extrabold text-primary">
                                {fmtMoney(profileSalePrice(p))}
                                {p.sellingPrice > 0 && p.sellingPrice !== p.price && (
                                  <span className="ml-1.5 text-xs font-medium text-muted-foreground">
                                    · {tf("vouchers.wizard.costShort", { n: fmtMoney(p.price) })}
                                  </span>
                                )}
                              </p>
                              <div className="mt-2 flex flex-wrap gap-1">
                                <Badge variant="secondary" className="gap-1 text-[10px]">
                                  <Clock className="size-3" aria-hidden />
                                  {formatDuration(p.sessionTimeoutMin * 60)}
                                </Badge>
                                {p.dataQuotaMb > 0 && (
                                  <Badge variant="secondary" className="gap-1 text-[10px]">
                                    <Database className="size-3" aria-hidden />
                                    {formatBytes(p.dataQuotaMb * 1048576, lang)}
                                  </Badge>
                                )}
                                <Badge variant="secondary" className="gap-1 text-[10px]">
                                  <ShieldCheck className="size-3" aria-hidden />
                                  {profileValidity(p)}
                                </Badge>
                              </div>
                            </button>
                          );
                        })}
                      </div>
                    )}
                  </section>

                  {/* Quantité — stepper + raccourcis */}
                  <section aria-label={t("vouchers.wizard.quantity")} className="grid gap-2">
                    <Label htmlFor="wiz-count" className="text-sm font-semibold">
                      {t("vouchers.wizard.quantity")}
                    </Label>
                    <div className="flex flex-wrap items-center gap-2">
                      <div className="flex items-center gap-1.5">
                        <Button
                          type="button"
                          variant="outline"
                          size="icon"
                          className="size-11 shrink-0 rounded-xl text-lg font-bold"
                          onClick={() => setCount(String(Math.max(1, (countValid ? countNum : 1) - 1)))}
                          disabled={isPending || !countValid || countNum <= 1}
                          aria-label={t("vouchers.wizard.qtyMinus")}
                        >
                          −
                        </Button>
                        <Input
                          id="wiz-count"
                          type="number"
                          inputMode="numeric"
                          min={1}
                          max={500}
                          value={count}
                          onChange={(event) => setCount(event.target.value.replace(/[^\d]/g, ""))}
                          disabled={isPending}
                          aria-invalid={!countValid}
                          className="h-11 w-20 rounded-xl text-center text-lg font-extrabold"
                        />
                        <Button
                          type="button"
                          variant="outline"
                          size="icon"
                          className="size-11 shrink-0 rounded-xl text-lg font-bold"
                          onClick={() => setCount(String(Math.min(500, (countValid ? countNum : 0) + 1)))}
                          disabled={isPending || !countValid || countNum >= 500}
                          aria-label={t("vouchers.wizard.qtyPlus")}
                        >
                          +
                        </Button>
                      </div>
                      <div className="flex flex-wrap gap-1.5">
                        {COUNT_CHIPS.map((chip) => (
                          <button
                            key={chip}
                            type="button"
                            onClick={() => setCount(String(chip))}
                            aria-pressed={countValid && countNum === chip}
                            className={cn(
                              "min-h-11 rounded-xl border px-3 text-sm font-semibold transition-all focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary",
                              countValid && countNum === chip
                                ? "wizard-pop border-primary bg-primary/10 text-primary"
                                : "border-border bg-background text-muted-foreground hover:border-primary/40 hover:text-foreground",
                            )}
                          >
                            {chip}
                          </button>
                        ))}
                      </div>
                    </div>
                    <p className="text-xs text-muted-foreground">{t("vouchers.countHint")}</p>
                  </section>

                  {/* Routeur — cartes + point d'état */}
                  <section aria-label={t("vouchers.wizard.chooseRouter")} className="grid gap-2">
                    <Label className="text-sm font-semibold">{t("vouchers.wizard.chooseRouter")}</Label>
                    {routers.length === 0 ? (
                      <p className="rounded-xl border border-dashed p-4 text-sm text-muted-foreground">
                        {t("vouchers.wizard.noRouters")}
                      </p>
                    ) : (
                      <div
                        role="radiogroup"
                        aria-label={t("vouchers.wizard.chooseRouter")}
                        className="grid gap-2 sm:grid-cols-2"
                      >
                        {routers.map((r) => {
                          const selected = r.id === routerId;
                          const online = r.status === "online";
                          return (
                            <button
                              key={r.id}
                              type="button"
                              role="radio"
                              aria-checked={selected}
                              onClick={() => {
                                setRouterId(r.id);
                                setServer("all");
                              }}
                              className={cn(
                                "wizard-card flex min-h-11 items-center gap-3 rounded-xl border-2 px-3 py-2.5 text-left transition-all duration-200 hover:-translate-y-0.5 hover:shadow-md focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary",
                                selected
                                  ? "border-primary bg-gradient-to-br from-primary/10 to-transparent shadow-md shadow-primary/10"
                                  : "border-border bg-card",
                              )}
                            >
                              <span
                                className={cn(
                                  "flex size-9 shrink-0 items-center justify-center rounded-lg",
                                  selected ? "bg-primary/15 text-primary" : "bg-muted text-muted-foreground",
                                )}
                                aria-hidden
                              >
                                <Wifi className="size-4" />
                              </span>
                              <span className="min-w-0 flex-1">
                                <span className="block truncate text-sm font-bold">{r.name}</span>
                                <span className="mt-0.5 flex items-center gap-1.5 text-xs text-muted-foreground">
                                  <span
                                    aria-hidden
                                    className={cn(
                                      "size-1.5 rounded-full",
                                      online ? "animate-pulse bg-emerald-500" : "bg-muted-foreground/40",
                                    )}
                                  />
                                  {modeBadge(r)} · {online ? t("vouchers.wizard.online") : t("vouchers.wizard.offline")}
                                </span>
                              </span>
                              {selected && (
                                <span
                                  className="wizard-pop flex size-5 shrink-0 items-center justify-center rounded-full bg-primary text-primary-foreground"
                                  aria-hidden
                                >
                                  <Check className="size-3" />
                                </span>
                              )}
                            </button>
                          );
                        })}
                      </div>
                    )}
                  </section>

                  {/* Serveur hotspot (si le routeur en expose — parité Mikhmon) */}
                  {servers.length > 0 && (
                    <div className="grid gap-2">
                      <Label htmlFor="wiz-server" className="text-sm font-semibold">
                        {t("vouchers.gen.server")}
                      </Label>
                      <Select value={server} onValueChange={setServer} disabled={isPending}>
                        <SelectTrigger id="wiz-server" className="h-11 w-full rounded-xl">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="all">{t("vouchers.gen.serverAll")}</SelectItem>
                          {servers.map((srv) => (
                            <SelectItem key={srv} value={srv}>
                              {srv}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                      <p className="text-xs text-muted-foreground">{t("vouchers.gen.serverHint")}</p>
                    </div>
                  )}
                </div>
              )}

              {/* ════════ ÉTAPE 2 — CODES ════════ */}
              {step === 2 && (
                <div className="grid gap-5">
                  {/* Aperçu live des codes */}
                  <section
                    aria-label={t("vouchers.wizard.previewTitle")}
                    className="rounded-xl border bg-gradient-to-br from-muted/70 to-background p-4"
                  >
                    <div className="flex items-center justify-between gap-2">
                      <Label className="text-sm font-semibold">{t("vouchers.wizard.previewTitle")}</Label>
                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        className="min-h-10 gap-1.5 text-xs"
                        onClick={reshuffle}
                      >
                        <RefreshCcw className={cn("size-3.5", spin && "animate-spin")} aria-hidden />
                        {t("vouchers.wizard.previewRefresh")}
                      </Button>
                    </div>
                    <div className="mt-1 flex flex-wrap gap-2">
                      {samples.map((s, i) => (
                        <span
                          key={`${seed}-${i}`}
                          className="wizard-pop rounded-lg border bg-background px-3 py-2 font-mono text-sm font-bold shadow-sm"
                          style={{ animationDelay: `${i * 70}ms` }}
                        >
                          <span className="text-primary">{prefix}</span>
                          {s.code}
                        </span>
                      ))}
                    </div>
                    <p className="mt-2 text-xs text-muted-foreground">{t("vouchers.wizard.previewHint")}</p>
                  </section>

                  {/* N°25 — Type de connexion VERROUILLÉ : un seul code par ticket
                      (il sert aussi de mot de passe). Le choix a été retiré. */}
                  <section aria-label={t("vouchers.wizard.connectionMode")} className="grid gap-2">
                    <Label className="text-sm font-semibold">{t("vouchers.wizard.connectionMode")}</Label>
                    <div className="flex items-start gap-3 rounded-xl border-2 border-primary/25 bg-gradient-to-br from-primary/10 to-transparent p-3">
                      <span
                        className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-primary/15 text-primary"
                        aria-hidden
                      >
                        <Fingerprint className="size-4" />
                      </span>
                      <span className="min-w-0">
                        <span className="flex flex-wrap items-center gap-2">
                          <span className="text-sm font-bold">{t("vouchers.gen.userModeSame")}</span>
                          <Badge variant="outline" className="gap-1 border-primary/25 bg-primary/10 text-primary">
                            <Lock className="size-3" aria-hidden />
                            {t("vouchers.wizard.modeLocked")}
                          </Badge>
                        </span>
                        <span className="mt-0.5 block text-xs text-muted-foreground">
                          {t("vouchers.wizard.modeSameDesc")}
                        </span>
                      </span>
                    </div>
                  </section>

                  {/* Jeu de caractères — pilules */}
                  <section aria-label={t("vouchers.gen.charset")} className="grid gap-2">
                    <Label className="text-sm font-semibold">{t("vouchers.gen.charset")}</Label>
                    <div role="radiogroup" aria-label={t("vouchers.gen.charset")} className="grid grid-cols-2 gap-2 sm:grid-cols-4">
                      {CHARSET_PILLS.map((pill) => {
                        const selected = charset === pill.value;
                        return (
                          <button
                            key={pill.value}
                            type="button"
                            role="radio"
                            aria-checked={selected}
                            onClick={() => setCharset(pill.value)}
                            title={t(pill.subKey)}
                            className={cn(
                              "wizard-card rounded-xl border-2 px-2 py-2.5 text-center transition-all duration-200 hover:-translate-y-0.5 hover:shadow-md focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary",
                              selected
                                ? "border-primary bg-primary/10"
                                : "border-border bg-card",
                            )}
                          >
                            <span
                              className={cn(
                                "block font-mono text-sm font-bold",
                                selected ? "text-primary" : "text-foreground",
                              )}
                            >
                              {pill.name}
                              {pill.star && " ★"}
                            </span>
                            <span className="mt-0.5 block text-[10px] leading-tight text-muted-foreground">
                              {t(pill.subKey)}
                            </span>
                          </button>
                        );
                      })}
                    </div>
                  </section>

                  {/* Longueur — slider */}
                  <section aria-label={t("vouchers.codeLength")} className="grid gap-2">
                    <div className="flex items-center justify-between">
                      <Label htmlFor="wiz-length" className="text-sm font-semibold">
                        {t("vouchers.codeLength")}
                      </Label>
                      <span className="wizard-pop rounded-lg bg-primary/10 px-2.5 py-1 font-mono text-sm font-bold text-primary">
                        {tf("vouchers.codeLengthUnit", { n: codeLength })}
                      </span>
                    </div>
                    <Slider
                      id="wiz-length"
                      min={CODE_LENGTH_MIN}
                      max={CODE_LENGTH_MAX}
                      step={1}
                      value={[codeLength]}
                      onValueChange={(values) => setCodeLength(values[0] ?? codeLength)}
                      disabled={isPending}
                      className="mt-1"
                      aria-label={t("vouchers.codeLength")}
                    />
                    <div aria-hidden className="flex justify-between px-0.5 text-[10px] font-medium text-muted-foreground">
                      {Array.from({ length: CODE_LENGTH_MAX - CODE_LENGTH_MIN + 1 }, (_, i) => (
                        <span key={i} className={cn(codeLength === i + CODE_LENGTH_MIN && "font-bold text-primary")}>
                          {i + CODE_LENGTH_MIN}
                        </span>
                      ))}
                    </div>
                  </section>

                  {/* Préfixe + commentaire */}
                  <div className="grid gap-4 sm:grid-cols-2">
                    <div className="grid gap-2">
                      <Label htmlFor="wiz-prefix" className="text-sm font-semibold">
                        {t("vouchers.prefix")}
                      </Label>
                      <Input
                        id="wiz-prefix"
                        value={prefix}
                        onChange={(event) => setPrefix(event.target.value)}
                        disabled={isPending}
                        maxLength={6}
                        placeholder={t("vouchers.wizard.prefixPlaceholder")}
                        className="h-11 rounded-xl font-mono uppercase"
                      />
                      <p className="text-xs text-muted-foreground">{t("vouchers.wizard.prefixMax")}</p>
                    </div>
                    <div className="grid gap-2">
                      <Label htmlFor="wiz-comment" className="text-sm font-semibold">
                        {t("vouchers.gen.comment")}
                      </Label>
                      <Input
                        id="wiz-comment"
                        value={comment}
                        onChange={(event) => setComment(event.target.value)}
                        disabled={isPending}
                        maxLength={64}
                        placeholder={t("vouchers.gen.commentPlaceholder")}
                        className="h-11 rounded-xl"
                      />
                      <p className="text-xs text-muted-foreground">
                        {tf("vouchers.wizard.commentCounter", { n: comment.length, max: 64 })}
                      </p>
                    </div>
                  </div>

                  {/* Limites avancées (parité Mikhmon) — repliable */}
                  <Collapsible open={advancedOpen} onOpenChange={setAdvancedOpen}>
                    <CollapsibleTrigger asChild>
                      <Button
                        type="button"
                        variant="outline"
                        className={cn(
                          "min-h-11 w-full justify-between rounded-xl",
                          advancedTouched && "border-primary/50",
                        )}
                      >
                        <span className="flex items-center gap-2">
                          <Gauge className="size-4 text-primary" aria-hidden />
                          {t("vouchers.wizard.advanced")}
                          {advancedTouched && (
                            <Badge className="wizard-pop gap-1 text-[10px]" aria-hidden>
                              <Check className="size-3" />
                              {t("vouchers.wizard.advancedBadge")}
                            </Badge>
                          )}
                        </span>
                        <ChevronRight
                          className={cn("size-4 transition-transform duration-200", advancedOpen && "rotate-90")}
                          aria-hidden
                        />
                      </Button>
                    </CollapsibleTrigger>
                    <CollapsibleContent className="wizard-expand grid gap-4 pt-3">
                      <div className="grid gap-2">
                        <Label htmlFor="wiz-timelimit" className="text-sm font-semibold">
                          {t("vouchers.gen.timeLimit")}
                        </Label>
                        <Select value={timeLimit} onValueChange={setTimeLimit} disabled={isPending}>
                          <SelectTrigger id="wiz-timelimit" className="h-11 w-full rounded-xl">
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            <SelectItem value="inherit">
                              {t("vouchers.gen.timeLimitInherit")}
                              {profile
                                ? ` (${
                                    profile.sessionTimeoutMin > 0
                                      ? fmtRouterDuration(profile.sessionTimeoutMin)
                                      : t("vouchers.gen.timeLimitUnlimited")
                                  })`
                                : ""}
                            </SelectItem>
                            {TIME_LIMIT_OPTIONS.map((opt) => (
                              <SelectItem key={opt.value} value={opt.value}>
                                {opt.min === 0
                                  ? t("vouchers.gen.timeLimitUnlimited")
                                  : fmtRouterDuration(opt.min)}
                              </SelectItem>
                            ))}
                          </SelectContent>
                        </Select>
                        <p className="text-xs text-muted-foreground">{t("vouchers.gen.timeLimitHint")}</p>
                      </div>

                      <div className="grid gap-2">
                        <Label htmlFor="wiz-quota" className="text-sm font-semibold">
                          {t("vouchers.gen.quotaLabel")}
                        </Label>
                        <Select value={quotaMb} onValueChange={setQuotaMb} disabled={isPending}>
                          <SelectTrigger id="wiz-quota" className="h-11 w-full rounded-xl">
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            <SelectItem value="inherit">
                              {t("vouchers.gen.quotaInherit")}
                              {profile
                                ? ` (${
                                    profile.dataQuotaMb > 0
                                      ? formatBytes(profile.dataQuotaMb * 1048576, lang)
                                      : t("vouchers.quotaUnlimitedShort")
                                  })`
                                : ""}
                            </SelectItem>
                            {QUOTA_OPTIONS.map((quota) => (
                              <SelectItem key={quota.value} value={quota.value}>
                                {quota.mb === 0 ? t("vouchers.gen.quotaUnlimited") : fmtQuota(quota.mb)}
                              </SelectItem>
                            ))}
                          </SelectContent>
                        </Select>
                        <p className="text-xs text-muted-foreground">{t("vouchers.gen.quotaHint")}</p>

                        {/* Repères FCFA — cliquables : appliquent le quota */}
                        <div className="rounded-xl border bg-background p-3">
                          <p className="text-sm font-medium">{t("vouchers.gen.priceHintsTitle")}</p>
                          <div className="mt-2 flex flex-wrap gap-1.5">
                            {QUOTA_PRICE_HINTS.map(([quota, price]) => {
                              const active = quotaMb === String(quota);
                              return (
                                <button
                                  key={quota}
                                  type="button"
                                  onClick={() => setQuotaMb(String(quota))}
                                  aria-pressed={active}
                                  title={tf("vouchers.wizard.hintApplies", { quota: fmtQuota(quota) })}
                                  className={cn(
                                    "wizard-card rounded-lg border px-2.5 py-1.5 font-mono text-xs font-semibold transition-all focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary",
                                    active
                                      ? "wizard-pop border-primary bg-primary/10 text-primary"
                                      : "border-border bg-background text-muted-foreground hover:border-primary/40 hover:text-foreground",
                                  )}
                                >
                                  {fmtQuota(quota)} = {fmtFcfa(price)}
                                </button>
                              );
                            })}
                          </div>
                          <p className="mt-2 text-xs text-muted-foreground">{t("vouchers.wizard.hintAppliesHint")}</p>
                        </div>
                      </div>
                    </CollapsibleContent>
                  </Collapsible>
                </div>
              )}

              {/* ════════ ÉTAPE 3 — RÉCAP ════════ */}
              {step === 3 && (
                <div className="grid gap-5">
                  {/* Revendeur + jauge de crédit */}
                  <section aria-label={t("common.reseller")} className="grid gap-2">
                    <Label htmlFor="wiz-reseller" className="text-sm font-semibold">
                      {t("common.reseller")}
                    </Label>
                    <Select value={resellerId} onValueChange={setResellerId} disabled={isPending}>
                      <SelectTrigger id="wiz-reseller" className="h-11 w-full rounded-xl">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="none">{t("vouchers.noReseller")}</SelectItem>
                        {resellers.map((r) => (
                          <SelectItem key={r.id} value={r.id}>
                            {tf("vouchers.creditLine", {
                              name: r.name,
                              credit: fmtMoney(r.credit),
                            })}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>

                    {reseller && creditAfter !== null && (
                      <div
                        className={cn(
                          "wizard-card mt-1 rounded-xl border p-3",
                          insufficient ? "border-destructive/50 bg-destructive/5" : "border-border bg-muted/40",
                        )}
                      >
                        <div className="flex items-center justify-between text-sm">
                          <span className="flex items-center gap-1.5 font-medium">
                            <Wallet className="size-4 text-primary" aria-hidden />
                            {t("vouchers.wizard.creditGauge")}
                          </span>
                          <span className={cn("font-bold", insufficient && "text-destructive")}>
                            <AnimatedNumber value={creditAfter} format={fmtMoney} />
                          </span>
                        </div>
                        {creditRatio !== null && (
                          <div className="mt-2" aria-hidden>
                            <div className="h-2 overflow-hidden rounded-full bg-border">
                              <div
                                className={cn(
                                  "h-full rounded-full transition-all duration-500 ease-out",
                                  insufficient ? "bg-destructive" : "bg-primary",
                                )}
                                style={{ width: `${creditRatio}%` }}
                              />
                            </div>
                          </div>
                        )}
                        <div className="mt-1.5 flex justify-between text-xs text-muted-foreground">
                          <span>
                            {t("vouchers.wizard.creditBefore")} : {fmtMoney(reseller.credit)}
                          </span>
                          <span>
                            {t("vouchers.wizard.creditAfterGen")} :{" "}
                            <span className={cn("font-semibold", insufficient && "text-destructive")}>
                              {fmtMoney(creditAfter)}
                            </span>
                          </span>
                        </div>
                        {insufficient && (
                          <p className="mt-2 flex items-center gap-1.5 text-sm font-medium text-destructive" role="alert">
                            <AlertTriangle className="size-4 shrink-0" aria-hidden />
                            {tf("vouchers.insufficient", { name: reseller.name })}
                          </p>
                        )}
                      </div>
                    )}
                  </section>

                  {/* Récap + aperçu ticket */}
                  <div className="grid gap-4 lg:grid-cols-[1fr_230px]">
                    <section aria-label={t("vouchers.wizard.step3")} className="grid gap-2 self-start rounded-xl border bg-card p-3">
                      {(
                        [
                          { icon: Ticket, label: t("vouchers.gen.profileLabel"), value: profile ? `${profile.name} · ${fmtMoney(profileSalePrice(profile))}` : "—" },
                          { icon: Route, label: t("vouchers.gen.routerLabel"), value: router ? router.name + (server !== "all" ? ` · ${server}` : "") : "—" },
                          { icon: Layers, label: t("vouchers.wizard.recapQuantity"), value: countValid ? tf("vouchers.wizard.qtyTickets", { n: countNum }) : "—" },
                          { icon: Hash, label: t("vouchers.wizard.recapFormat"), value: formatRecap },
                          { icon: Gauge, label: t("vouchers.wizard.recapLimits"), value: `${t("vouchers.wizard.recapTime")} ${timeLimitLabel} · ${t("vouchers.wizard.recapData")} ${quotaLabel}` },
                          { icon: Wallet, label: t("common.reseller"), value: reseller ? reseller.name : t("vouchers.noReseller") },
                        ]
                      ).map((row) => {
                        const Icon = row.icon;
                        return (
                          <div key={row.label} className="flex items-start gap-2.5 px-1 py-1.5">
                            <span className="mt-0.5 flex size-7 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary" aria-hidden>
                              <Icon className="size-3.5" />
                            </span>
                            <span className="min-w-0">
                              <span className="block text-xs text-muted-foreground">{row.label}</span>
                              <span className="block truncate text-sm font-semibold">{row.value}</span>
                            </span>
                          </div>
                        );
                      })}
                      {profile && (
                        <p className="mt-1 border-t px-1 pt-2 text-xs text-muted-foreground">
                          {t("vouchers.gen.recapValidity")} : <span className="font-medium text-foreground">{profileValidity(profile)}</span>
                          {" · "}
                          {t("vouchers.gen.recapExpiry")} :{" "}
                          <span className="font-medium text-foreground">
                            {EXP_MODE_LABEL_KEY[profile.expMode] ? t(EXP_MODE_LABEL_KEY[profile.expMode]) : profile.expMode}
                          </span>
                        </p>
                      )}
                    </section>

                    {/* Ticket miniature en direct */}
                    <section aria-label={t("vouchers.wizard.ticketPreview")} className="grid gap-2">
                      <p className="text-center text-xs font-medium text-muted-foreground">
                        {t("vouchers.wizard.ticketPreview")}
                      </p>
                      <div className="mx-auto w-full max-w-[230px] rounded-xl border-2 border-dashed border-primary/40 bg-background p-3 text-center shadow-sm">
                        <p className="truncate text-[10px] font-bold uppercase tracking-widest">{tenantName}</p>
                        <p className="text-[8px] uppercase tracking-[0.3em] text-muted-foreground">WIFI</p>
                        <p className="wizard-pop mx-auto mt-2 w-fit rounded-md bg-muted px-2 py-1 font-mono text-base font-extrabold tracking-wide">
                          {prefix}
                          {samples[0]?.code}
                        </p>
                        <Separator className="my-2" />
                        <p className="truncate text-xs font-semibold">{profile?.name ?? "—"}</p>
                        <p className="text-[10px] text-muted-foreground">{quotaLabel}</p>
                        <div className="mt-1.5 flex items-center justify-center gap-1.5" aria-hidden>
                          <QrCode className="size-8 text-foreground/60" />
                          <span className="text-sm font-extrabold text-primary">
                            {profile ? fmtMoney(profileSalePrice(profile)) : "—"}
                          </span>
                        </div>
                      </div>
                    </section>
                  </div>

                  {/* Coût total animé */}
                  <div className="flex items-end justify-between gap-3 rounded-xl border border-primary/25 bg-gradient-to-br from-primary/10 to-transparent px-4 py-3">
                    <div>
                      <p className="text-xs font-medium text-muted-foreground">{t("vouchers.totalCost")}</p>
                      <p className="text-2xl font-extrabold text-primary">
                        <AnimatedNumber value={totalCost} format={fmtMoney} />
                      </p>
                    </div>
                    <p className="pb-1 text-xs text-muted-foreground">
                      {tf("vouchers.costDetail", {
                        n: countValid ? countNum : 0,
                        price: fmtMoney(unitPrice),
                      })}
                      {profile && tf("vouchers.gen.profileSuffix", { name: profile.name })}
                    </p>
                  </div>

                  {isError && error && (
                    <p className="flex items-center gap-1.5 text-sm text-destructive" role="alert">
                      <AlertTriangle className="size-4 shrink-0" aria-hidden />
                      {error.message}
                    </p>
                  )}
                </div>
              )}
            </div>
          </div>

          {/* ── Pied : coût live + navigation ── */}
          <div className="flex flex-wrap items-center justify-between gap-3 border-t bg-muted/30 px-5 py-4 sm:px-6">
            <div className="min-w-0">
              <p className="text-xs text-muted-foreground">{t("vouchers.totalCost")}</p>
              <p className="text-lg font-extrabold leading-tight text-primary">
                <AnimatedNumber value={totalCost} format={fmtMoney} />
                <span className="ml-1.5 text-xs font-medium text-muted-foreground">
                  {tf("vouchers.costDetail", {
                    n: countValid ? countNum : 0,
                    price: fmtMoney(unitPrice),
                  })}
                </span>
              </p>
              {step < 3 && nextHint && (
                <p className="mt-0.5 flex items-center gap-1 text-xs font-medium text-amber-600 dark:text-amber-400">
                  <AlertTriangle className="size-3 shrink-0" aria-hidden />
                  {nextHint}
                </p>
              )}
            </div>
            <div className="flex shrink-0 items-center gap-2">
              {step > 1 && (
                <Button
                  type="button"
                  variant="ghost"
                  className="min-h-11 gap-1"
                  onClick={() => go(step - 1)}
                  disabled={isPending}
                >
                  <ChevronLeft className="size-4" aria-hidden />
                  {t("vouchers.wizard.back")}
                </Button>
              )}
              {step < 3 ? (
                <Button
                  type="button"
                  className="min-h-11 min-w-32 gap-1 rounded-xl shadow-lg shadow-primary/25"
                  onClick={tryNext}
                  disabled={!stepValid || isPending}
                >
                  {t("vouchers.wizard.continue")}
                  <ChevronRight className="size-4" aria-hidden />
                </Button>
              ) : (
                <Button
                  type="submit"
                  className="min-h-11 min-w-44 gap-1.5 rounded-xl shadow-lg shadow-primary/25"
                  disabled={!canSubmit}
                >
                  {isPending ? (
                    <Loader2 className="size-4 animate-spin" aria-hidden />
                  ) : (
                    <Zap className="size-4" aria-hidden />
                  )}
                  {tf("vouchers.wizard.generateN", { n: countValid ? countNum : 0 })}
                </Button>
              )}
            </div>
          </div>
        </form>
      </DialogContent>
    </Dialog>
  );
}
