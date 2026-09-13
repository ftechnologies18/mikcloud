"use client";

// N°96 — Refonte UX/UI des cartes de protection (vue /app/protection).
//
// Historique : N°83 a fait sortir les modules du 4e onglet « Système » de
// la fiche routeur (la protection est un argument de vente, elle vit à 1
// clic de l'atterrissage) ; N°88 y a ajouté la 4e carte (Bloque-VPN). Le
// contenu grandissait mais la forme restait celle d'un formulaire : 4 cartes
// identiques à plat, l'état ne se lisait qu'en parcourant chaque
// interrupteur, les footnotes honnêtes (N°85/88) formaient des murs de
// texte 11 px, le score « n/4 » vivait en texte brut.
//
// Cette refonte conserve 100 % des comportements (mêmes mutations, toasts,
// invalidations ["/api/routers" + "/api/dashboard"], gardes mode agent,
// éditeur FamilyGuard complet envoyé par le Switch) et change la
// présentation :
//   • ProtectionScoreRing — anneau SVG n/4 coloré par verdict, exposé au
//     héros de la vue (N°96) : l'état du site se lit en une seconde ;
//   • ModuleCard — en-tête commun : icône d'IDENTITÉ par module (ShieldCheck
//     filtrage, Lock anti-piratage, MoonStar couvre-feu, GlobeLock VPN)
//     teintée selon l'état (primaire = actif, neutre = inactif), chip d'état
//     (niveau SafeWiFi / Actif / Inactif) et note « Bon à savoir » en
//     popover — l'honnêteté des footnotes reste, à un clic au lieu d'un mur ;
//   • SafeWiFi — coche de sélection dans chaque option + pastille
//     « Recommandé » sur « Menaces bloquées » (le défaut raisonnable pour
//     tout WiFi public) ;
//   • FamilyGuard — interrupteur séparé du PLANNING (heures + jours +
//     enregistrer) groupé dans un bloc bordé ;
//   • grilles : la vue passe en md:grid-cols-2 — SafeWiFi porte 3 options
//     et FamilyGuard un éditeur complet, 4 colonnes les compressaient.
//
// Le résumé compact de l'onglet Système (ProtectionSummaryCard, fiche
// routeur) est conservé tel quel : même source de calcul (protection.ts),
// mêmes clés, CTA vers cette vue.

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  ArrowRight,
  Baby,
  Check,
  Clock,
  Globe,
  GlobeLock,
  Info,
  Loader2,
  Lock,
  MoonStar,
  Shield,
  ShieldCheck,
  ShieldOff,
} from "lucide-react";
import type { LucideIcon } from "lucide-react";
import type { ReactNode } from "react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { Switch } from "@/components/ui/switch";
import { cn } from "@/lib/utils";
import {
  setRouterAntiVpn,
  setRouterFamilyGuard,
  setRouterSafeWifi,
  setRouterShield,
  type AntiVpnLevel,
  type FamilyGuardWindow,
  type SafeWifiLevel,
  type ShieldLevel,
} from "@/lib/hotspot/api";
import { useI18n } from "@/lib/hotspot/i18n";
import { viewToPath } from "@/lib/hotspot/view-path";
import {
  antiVpnOn,
  familyGuardActiveNow,
  parseFamilyGuardSpec,
  protectionVerdict,
  safeWifiLevelOf,
  shieldOn,
  type ProtectionVerdict,
} from "@/lib/hotspot/protection";
import type { RouterDevice } from "@/lib/hotspot/types";

// ─── Verdict (partagé vue / bandeau / résumé fiche routeur) ───

const VERDICT_STYLES: Record<ProtectionVerdict, string> = {
  protected: "border-primary/30 bg-primary/10 text-primary",
  partial: "border-amber-500/40 bg-amber-500/10 text-amber-700 dark:text-amber-300",
  unprotected: "border-destructive/40 bg-destructive/10 text-destructive",
};

/** Badge de verdict d'un routeur — « Bien protégé / À renforcer / Non
 * protégé ». La couleur suit la gravité (primaire / ambre / destructif,
 * tokens existants — pattern bandeau abonnement). */
export function ProtectionVerdictBadge({ verdict }: { verdict: ProtectionVerdict }) {
  const { t } = useI18n();
  const Icon = verdict === "protected" ? ShieldCheck : verdict === "partial" ? Shield : ShieldOff;
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-xs font-semibold",
        VERDICT_STYLES[verdict],
      )}
    >
      <Icon className="size-3.5 shrink-0" aria-hidden />
      {t(`protection.verdict.${verdict}`)}
    </span>
  );
}

// ─── Anneau de score (N°96) ───

/** Couleur d'arc selon le verdict — miroir VERDICT_STYLES (primaire /
 * ambre / destructif), appliquée à l'attribut SVG stroke via utilitaire
 * Tailwind (stroke-<couleur> résout la variable du thème). */
const SCORE_STROKE: Record<ProtectionVerdict, string> = {
  protected: "stroke-primary",
  partial: "stroke-amber-500",
  unprotected: "stroke-destructive",
};

/** Anneau de score n/4 — la jauge du héros de la vue Protection. Le score
 * compte des MODULES (pas un pourcentage de « sécurité », notion qui
 * n'existerait pas) : la valeur « n/4 » au centre dit la vérité, l'arc la
 * rend visible à 3 mètres. role="img" + libellé complet pour les lecteurs
 * d'écran (l'arc est décoratif, aria-hidden). */
export function ProtectionScoreRing({ score, verdict }: { score: number; verdict: ProtectionVerdict }) {
  const { tf } = useI18n();
  const n = Math.max(0, Math.min(4, score));
  const circumference = 2 * Math.PI * 33;
  return (
    <div
      role="img"
      aria-label={tf("protection.scoreAria", { n })}
      className="relative flex size-20 shrink-0 items-center justify-center"
    >
      <svg viewBox="0 0 80 80" className="size-full -rotate-90" aria-hidden>
        <circle cx="40" cy="40" r="33" fill="none" strokeWidth="7" className="stroke-border" />
        <circle
          cx="40"
          cy="40"
          r="33"
          fill="none"
          strokeWidth="7"
          strokeLinecap="round"
          className={cn("transition-[stroke-dasharray] duration-500 ease-out", SCORE_STROKE[verdict])}
          style={{ strokeDasharray: `${(n / 4) * circumference} ${circumference}` }}
        />
      </svg>
      <span className="absolute inset-0 flex items-center justify-center" aria-hidden>
        <span className="flex items-baseline gap-0.5">
          <span className="text-xl font-bold leading-none tabular-nums">{n}</span>
          <span className="text-[10px] font-medium text-muted-foreground">/4</span>
        </span>
      </span>
    </div>
  );
}

// ─── Briques communes des cartes modules (N°96) ───

/** Chip d'état d'une carte module — « Actif » (primaire, point plein) /
 * « Inactif » (neutre) / nom du niveau SafeWiFi. Lisible à 5 m, remplace
 * la lecture de chaque interrupteur pour scanner la vue. */
function ModuleStateChip({ on, label }: { on: boolean; label: string }) {
  return (
    <span
      className={cn(
        "inline-flex shrink-0 items-center gap-1.5 rounded-full border px-2 py-0.5 text-[11px] font-medium",
        on ? "border-primary/25 bg-primary/10 text-primary" : "border-border bg-muted text-muted-foreground",
      )}
    >
      <span className={cn("size-1.5 rounded-full", on ? "bg-primary" : "bg-muted-foreground/50")} aria-hidden />
      {label}
    </span>
  );
}

/** Note « Bon à savoir » — la footnote honnête d'un module (convergence
 * ≤ 45 s, auto-réparation, limites connues N°85/88) passe du mur de texte
 * 11 px à un popover à un clic : l'information reste, le bruit visuel
 * disparaît. Mêmes clés tools.*.footnote, zéro contenu réécrit. */
function FootnoteNote({ footnote }: { footnote: string }) {
  const { t } = useI18n();
  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="size-7 text-muted-foreground hover:text-foreground"
          aria-label={t("protection.detailsTitle")}
        >
          <Info className="size-4" aria-hidden />
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-80">
        <p className="text-sm font-semibold">{t("protection.detailsTitle")}</p>
        <p className="mt-1.5 text-xs leading-relaxed text-muted-foreground">{footnote}</p>
      </PopoverContent>
    </Popover>
  );
}

/** Coquille commune des 4 cartes : en-tête identité (icône teintée selon
 * l'état + titre + bénéfice) avec chip d'état et note, puis les contrôles
 * du module en children. L'API visuelle reste celle de l'app (Card
 * gap-0 py-0, CardContent p-4 sm:p-5 — pattern maison). */
function ModuleCard({
  icon: Icon,
  on,
  title,
  desc,
  stateLabel,
  footnote,
  children,
}: {
  icon: LucideIcon;
  on: boolean;
  title: string;
  desc: string;
  stateLabel: string;
  footnote: string;
  children: ReactNode;
}) {
  return (
    <Card className="gap-0 py-0">
      <CardContent className="flex flex-col p-4 sm:p-5">
        <div className="flex flex-wrap items-start justify-between gap-x-3 gap-y-2">
          <div className="flex min-w-0 flex-1 items-start gap-3">
            <span
              className={cn(
                "flex size-9 shrink-0 items-center justify-center rounded-lg transition-colors",
                on ? "bg-primary/10 text-primary" : "bg-muted text-muted-foreground",
              )}
            >
              <Icon className="size-4.5" aria-hidden />
            </span>
            <div className="min-w-0">
              <h3 className="text-sm font-semibold leading-snug">{title}</h3>
              <p className="mt-0.5 text-xs leading-relaxed text-muted-foreground">{desc}</p>
            </div>
          </div>
          <div className="flex shrink-0 items-center gap-1">
            <ModuleStateChip on={on} label={stateLabel} />
            <FootnoteNote footnote={footnote} />
          </div>
        </div>
        <div className="mt-4">{children}</div>
      </CardContent>
    </Card>
  );
}

// ─── SafeWiFi (N°80) ───

const safeWifiLevels: {
  level: SafeWifiLevel;
  icon: LucideIcon;
  nameKey: string;
  descKey: string;
}[] = [
  { level: "off", icon: ShieldOff, nameKey: "tools.safewifi.levelOff", descKey: "tools.safewifi.levelOffDesc" },
  { level: "threats", icon: ShieldCheck, nameKey: "tools.safewifi.levelThreats", descKey: "tools.safewifi.levelThreatsDesc" },
  { level: "family", icon: Baby, nameKey: "tools.safewifi.levelFamily", descKey: "tools.safewifi.levelFamilyDesc" },
];

export function SafeWifiCard({ router }: { router: RouterDevice }) {
  const { t, tf } = useI18n();
  const queryClient = useQueryClient();

  const current: SafeWifiLevel = safeWifiLevelOf(router);

  const levelMutation = useMutation({
    mutationFn: (level: SafeWifiLevel) => setRouterSafeWifi(router.id, level),
    onSuccess: (res) => {
      toast.success(tf("tools.safewifi.appliedToast", { name: router.name }), {
        description: res.message,
      });
      for (const key of ["/api/routers", "/api/dashboard"]) {
        void queryClient.invalidateQueries({ queryKey: [key] });
      }
    },
    onError: (err: Error) => toast.error(err.message),
  });

  // Chip d'état : le niveau courant (« Menaces bloquées » / « Protection
  // familles ») ou « Inactif » — l'état de ce module à 3 niveaux se lit
  // sans ouvrir la carte.
  const stateLabel =
    current === "threats"
      ? t("tools.safewifi.levelThreats")
      : current === "family"
        ? t("tools.safewifi.levelFamily")
        : t("protection.stateOff");

  return (
    <ModuleCard
      icon={ShieldCheck}
      on={current !== "off"}
      title={t("tools.safewifi.title")}
      desc={t("tools.safewifi.desc")}
      stateLabel={stateLabel}
      footnote={t("tools.safewifi.footnote")}
    >
      <div className="space-y-2" role="radiogroup" aria-label={t("tools.safewifi.title")}>
        {safeWifiLevels.map(({ level, icon: Icon, nameKey, descKey }) => {
          const active = current === level;
          return (
            <button
              key={level}
              type="button"
              role="radio"
              aria-checked={active}
              disabled={levelMutation.isPending || router.mode !== "agent"}
              onClick={() => !active && levelMutation.mutate(level)}
              className={cn(
                "flex w-full items-start gap-3 rounded-lg border p-3 text-left transition-colors",
                "min-h-11 disabled:cursor-not-allowed disabled:opacity-50",
                active
                  ? "border-primary bg-primary/5"
                  : "border-border hover:border-muted-foreground/30 hover:bg-muted/60",
              )}
            >
              <Icon
                className={cn("mt-0.5 size-4 shrink-0", active ? "text-primary" : "text-muted-foreground")}
                aria-hidden
              />
              <span className="min-w-0 flex-1">
                <span className="flex flex-wrap items-center gap-2">
                  <span className="text-sm font-medium">{t(nameKey)}</span>
                  {level === "threats" && !active && (
                    <Badge
                      variant="outline"
                      className="h-5 border-amber-500/40 px-1.5 text-[11px] text-amber-700 dark:text-amber-300"
                    >
                      {t("protection.recommended")}
                    </Badge>
                  )}
                </span>
                <span className="mt-0.5 block text-xs leading-relaxed text-muted-foreground">{t(descKey)}</span>
              </span>
              <span
                aria-hidden
                className={cn(
                  "mt-0.5 flex size-4 shrink-0 items-center justify-center rounded-full border transition-colors",
                  active ? "border-primary bg-primary text-primary-foreground" : "border-muted-foreground/30",
                )}
              >
                {active && <Check className="size-3" strokeWidth={3} />}
              </span>
            </button>
          );
        })}
      </div>
    </ModuleCard>
  );
}

// ─── Shield (N°81) ───

export function ShieldCard({ router }: { router: RouterDevice }) {
  const { t, tf } = useI18n();
  const queryClient = useQueryClient();

  const current: ShieldLevel = shieldOn(router) ? "on" : "off";

  const levelMutation = useMutation({
    mutationFn: (level: ShieldLevel) => setRouterShield(router.id, level),
    onSuccess: (res) => {
      toast.success(tf("tools.shield.appliedToast", { name: router.name }), {
        description: res.message,
      });
      for (const key of ["/api/routers", "/api/dashboard"]) {
        void queryClient.invalidateQueries({ queryKey: [key] });
      }
    },
    onError: (err: Error) => toast.error(err.message),
  });

  const on = current === "on";
  const RowIcon = on ? ShieldCheck : ShieldOff;

  return (
    <ModuleCard
      icon={Lock}
      on={on}
      title={t("tools.shield.title")}
      desc={t("tools.shield.desc")}
      stateLabel={on ? t("tools.shield.activeBadge") : t("protection.stateOff")}
      footnote={t("tools.shield.footnote")}
    >
      <div
        className={cn(
          "flex items-center justify-between gap-3 rounded-lg border p-3 transition-colors",
          on ? "border-primary/30 bg-primary/5" : "border-border",
        )}
      >
        <div className="flex min-w-0 items-start gap-2">
          <RowIcon
            className={cn("mt-0.5 size-4 shrink-0", on ? "text-primary" : "text-muted-foreground")}
            aria-hidden
          />
          <span className="min-w-0">
            <span className="block text-sm font-medium">{t("tools.shield.toggle")}</span>
            <span className="mt-0.5 block text-xs leading-relaxed text-muted-foreground">
              {t("tools.shield.toggleDesc")}
            </span>
          </span>
        </div>
        <Switch
          checked={on}
          onCheckedChange={(v) => levelMutation.mutate(v ? "on" : "off")}
          disabled={levelMutation.isPending || router.mode !== "agent"}
          aria-label={t("tools.shield.toggle")}
        />
      </div>
    </ModuleCard>
  );
}

// ─── AntiVPN (N°88) ───

export function AntiVpnCard({ router }: { router: RouterDevice }) {
  const { t, tf } = useI18n();
  const queryClient = useQueryClient();

  const current: AntiVpnLevel = antiVpnOn(router) ? "on" : "off";

  const levelMutation = useMutation({
    mutationFn: (level: AntiVpnLevel) => setRouterAntiVpn(router.id, level),
    onSuccess: (res) => {
      toast.success(tf("tools.antivpn.appliedToast", { name: router.name }), {
        description: res.message,
      });
      for (const key of ["/api/routers", "/api/dashboard"]) {
        void queryClient.invalidateQueries({ queryKey: [key] });
      }
    },
    onError: (err: Error) => toast.error(err.message),
  });

  const on = current === "on";
  const RowIcon = on ? GlobeLock : Globe;

  return (
    <ModuleCard
      icon={GlobeLock}
      on={on}
      title={t("tools.antivpn.title")}
      desc={t("tools.antivpn.desc")}
      stateLabel={on ? t("tools.antivpn.activeBadge") : t("protection.stateOff")}
      footnote={t("tools.antivpn.footnote")}
    >
      <div
        className={cn(
          "flex items-center justify-between gap-3 rounded-lg border p-3 transition-colors",
          on ? "border-primary/30 bg-primary/5" : "border-border",
        )}
      >
        <div className="flex min-w-0 items-start gap-2">
          <RowIcon
            className={cn("mt-0.5 size-4 shrink-0", on ? "text-primary" : "text-muted-foreground")}
            aria-hidden
          />
          <span className="min-w-0">
            <span className="block text-sm font-medium">{t("tools.antivpn.toggle")}</span>
            <span className="mt-0.5 block text-xs leading-relaxed text-muted-foreground">
              {t("tools.antivpn.toggleDesc")}
            </span>
          </span>
        </div>
        <Switch
          checked={on}
          onCheckedChange={(v) => levelMutation.mutate(v ? "on" : "off")}
          disabled={levelMutation.isPending || router.mode !== "agent"}
          aria-label={t("tools.antivpn.toggle")}
        />
      </div>
    </ModuleCard>
  );
}

// ─── FamilyGuard (N°82) ───

/** Clés i18n des jours (lundi → dimanche, ordre du spec). */
const familyGuardDayKeys = [
  { short: "tools.familyguard.dayMon", full: "tools.familyguard.dayMonFull" },
  { short: "tools.familyguard.dayTue", full: "tools.familyguard.dayTueFull" },
  { short: "tools.familyguard.dayWed", full: "tools.familyguard.dayWedFull" },
  { short: "tools.familyguard.dayThu", full: "tools.familyguard.dayThuFull" },
  { short: "tools.familyguard.dayFri", full: "tools.familyguard.dayFriFull" },
  { short: "tools.familyguard.daySat", full: "tools.familyguard.daySatFull" },
  { short: "tools.familyguard.daySun", full: "tools.familyguard.daySunFull" },
];

export function FamilyGuardCard({ router }: { router: RouterDevice }) {
  const { t, tf } = useI18n();
  const queryClient = useQueryClient();

  const saved = parseFamilyGuardSpec(router.familyGuardSpec);
  const activeNow = familyGuardActiveNow(saved, new Date());

  // Éditeur local : les champs sont initialisés depuis le spec persisté,
  // « Enregistrer » les pousse tels quels (le Switch envoie l'état
  // complet courant — enabled + champs de l'éditeur).
  const [start, setStart] = useState(saved.start);
  const [end, setEnd] = useState(saved.end);
  const [days, setDays] = useState(saved.days);

  const dirty =
    start !== saved.start || end !== saved.end || days !== saved.days;

  const mutation = useMutation({
    mutationFn: (w: FamilyGuardWindow) => setRouterFamilyGuard(router.id, w),
    onSuccess: (res, w) => {
      toast.success(tf("tools.familyguard.appliedToast", { name: router.name }), {
        description: res.message,
      });
      if (w.enabled && !activeNow) {
        // information honnête : activé mais hors fenêtre pour l'instant.
        toast.info(tf("tools.familyguard.idle", { start: w.start, end: w.end }));
      }
      for (const key of ["/api/routers", "/api/dashboard"]) {
        void queryClient.invalidateQueries({ queryKey: [key] });
      }
    },
    onError: (err: Error) => toast.error(err.message),
  });

  const timeOk = (s: string) => /^([01]\d|2[0-3]):[0-5]\d$/.test(s);
  const editorValid =
    timeOk(start) && timeOk(end) && start !== end && /^[01]{7}$/.test(days) && days.includes("1");

  const RowIcon = activeNow ? MoonStar : Clock;

  return (
    <ModuleCard
      icon={MoonStar}
      on={saved.enabled}
      title={t("tools.familyguard.title")}
      desc={t("tools.familyguard.desc")}
      stateLabel={saved.enabled ? t("tools.familyguard.activeBadge") : t("protection.stateOff")}
      footnote={t("tools.familyguard.footnote")}
    >
      {/* Interrupteur : l'état vivant (en cours / programmé) se lit dans
          la description — la chip d'en-tête porte le statut du module. */}
      <div
        className={cn(
          "flex items-center justify-between gap-3 rounded-lg border p-3 transition-colors",
          saved.enabled ? "border-primary/30 bg-primary/5" : "border-border",
        )}
      >
        <div className="flex min-w-0 items-start gap-2">
          <RowIcon
            className={cn(
              "mt-0.5 size-4 shrink-0",
              saved.enabled ? "text-primary" : "text-muted-foreground",
            )}
            aria-hidden
          />
          <span className="min-w-0">
            <span className="block text-sm font-medium">{t("tools.familyguard.toggle")}</span>
            <span className="mt-0.5 block text-xs leading-relaxed text-muted-foreground">
              {saved.enabled
                ? activeNow
                  ? tf("tools.familyguard.liveNow", { end: saved.end })
                  : tf("tools.familyguard.idle", { start: saved.start, end: saved.end })
                : t("tools.familyguard.toggleDesc")}
            </span>
          </span>
        </div>
        <Switch
          checked={saved.enabled}
          onCheckedChange={(v) =>
            mutation.mutate({ enabled: v, start, end, days })
          }
          disabled={mutation.isPending || router.mode !== "agent" || !editorValid}
          aria-label={t("tools.familyguard.toggle")}
        />
      </div>

      {/* Planning groupé (N°96) : heures + jours + enregistrement dans un
          bloc bordé distinct de l'interrupteur — l'édition ne se confond
          plus avec l'activation. */}
      <div className="mt-3 space-y-3 rounded-lg border bg-muted/30 p-3">
        <p className="text-xs font-medium text-muted-foreground">{t("protection.scheduleTitle")}</p>
        <div className="grid grid-cols-2 gap-3">
          <div className="space-y-1.5">
            <Label htmlFor={`fg-start-${router.id}`} className="text-xs text-muted-foreground">
              {t("tools.familyguard.startLabel")}
            </Label>
            <Input
              id={`fg-start-${router.id}`}
              type="time"
              value={start}
              onChange={(e) => setStart(e.target.value)}
              disabled={mutation.isPending || router.mode !== "agent"}
              className="h-9"
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor={`fg-end-${router.id}`} className="text-xs text-muted-foreground">
              {t("tools.familyguard.endLabel")}
            </Label>
            <Input
              id={`fg-end-${router.id}`}
              type="time"
              value={end}
              onChange={(e) => setEnd(e.target.value)}
              disabled={mutation.isPending || router.mode !== "agent"}
              className="h-9"
            />
          </div>
        </div>
        <fieldset>
          <legend className="text-xs text-muted-foreground">{t("tools.familyguard.daysLabel")}</legend>
          <div className="mt-1.5 flex flex-wrap gap-1.5" role="group" aria-label={t("tools.familyguard.daysLabel")}>
            {familyGuardDayKeys.map((k, i) => {
              const on = days[i] === "1";
              return (
                <button
                  key={k.short}
                  type="button"
                  aria-pressed={on}
                  aria-label={t(k.full)}
                  disabled={mutation.isPending || router.mode !== "agent"}
                  onClick={() =>
                    setDays((d) => {
                      const chars = d.split("");
                      chars[i] = on ? "0" : "1";
                      return chars.join("");
                    })
                  }
                  className={cn(
                    "size-8 rounded-md border text-xs font-medium transition-colors",
                    "min-h-8 disabled:cursor-not-allowed disabled:opacity-50",
                    on
                      ? "border-primary bg-primary/10 text-primary"
                      : "border-border text-muted-foreground hover:border-muted-foreground/30 hover:bg-muted/60",
                  )}
                >
                  {t(k.short)}
                </button>
              );
            })}
          </div>
        </fieldset>
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="w-full"
          disabled={mutation.isPending || router.mode !== "agent" || !editorValid || !dirty}
          onClick={() => mutation.mutate({ enabled: saved.enabled, start, end, days })}
        >
          {mutation.isPending && <Loader2 className="size-3.5 animate-spin" aria-hidden />}
          {t("tools.familyguard.save")}
        </Button>
        {!editorValid && (
          <p className="text-[11px] text-destructive">{t("tools.familyguard.invalidWindow")}</p>
        )}
      </div>
    </ModuleCard>
  );
}

// ─── Résumé compact (onglet Système de la fiche routeur) ───

/** État lisible d'une protection pour le résumé : nom du module + valeur
 * courte (badge Actif / niveau / fenêtre « HH:MM → HH:MM » / —). */
export function ProtectionSummaryCard({ router }: { router: RouterDevice }) {
  const { t, tf } = useI18n();
  const nav = useRouter();

  const fg = parseFamilyGuardSpec(router.familyGuardSpec);
  const verdict = protectionVerdict(router);
  const score =
    (safeWifiLevelOf(router) !== "off" ? 1 : 0) +
    (shieldOn(router) ? 1 : 0) +
    (fg.enabled ? 1 : 0) +
    (antiVpnOn(router) ? 1 : 0);

  const rows: { nameKey: string; state: string; on: boolean }[] = [
    {
      nameKey: "tools.safewifi.title",
      state:
        safeWifiLevelOf(router) === "threats"
          ? t("tools.safewifi.levelThreats")
          : safeWifiLevelOf(router) === "family"
            ? t("tools.safewifi.levelFamily")
            : "—",
      on: safeWifiLevelOf(router) !== "off",
    },
    { nameKey: "tools.shield.title", state: shieldOn(router) ? t("tools.shield.activeBadge") : "—", on: shieldOn(router) },
    {
      nameKey: "tools.familyguard.title",
      state: fg.enabled ? `${fg.start} → ${fg.end}` : "—",
      on: fg.enabled,
    },
    { nameKey: "tools.antivpn.title", state: antiVpnOn(router) ? t("tools.antivpn.activeBadge") : "—", on: antiVpnOn(router) },
  ];

  return (
    <Card className="gap-0 py-0">
      <CardContent className="p-4 sm:p-5">
        <div className="flex items-start gap-2">
          <ShieldCheck className="mt-0.5 size-4 shrink-0 text-primary" aria-hidden />
          <div>
            <h3 className="text-sm font-semibold">{t("protection.summary.title")}</h3>
            <p className="mt-0.5 text-xs text-muted-foreground">{t("protection.summary.desc")}</p>
          </div>
        </div>

        <div className="mt-3 flex flex-wrap items-center justify-between gap-2 rounded-lg border p-3">
          <ProtectionVerdictBadge verdict={verdict} />
          <span className="text-xs font-medium text-muted-foreground tabular-nums">
            {tf("protection.ofModules", { n: score })}
          </span>
        </div>

        <ul className="mt-2 divide-y">
          {rows.map((row) => (
            <li key={row.nameKey} className="flex items-center justify-between gap-3 py-2">
              <span className="min-w-0 truncate text-sm">{t(row.nameKey)}</span>
              <span
                className={cn(
                  "shrink-0 text-xs font-medium tabular-nums",
                  row.on ? "text-primary" : "text-muted-foreground",
                )}
              >
                {row.state}
              </span>
            </li>
          ))}
        </ul>

        <Button
          type="button"
          variant="outline"
          size="sm"
          className="mt-3 w-full"
          onClick={() => nav.push(viewToPath("protection", router.id), { scroll: false })}
        >
          {t("protection.summary.cta")}
          <ArrowRight className="size-3.5" aria-hidden />
        </Button>
      </CardContent>
    </Card>
  );
}
