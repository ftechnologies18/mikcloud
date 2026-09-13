"use client";

// N°83 — Cartes de protection : les 3 modules sécurité (SafeWiFi N°80,
// Shield N°81, FamilyGuard N°82) quittent le 4e onglet « Système » de la
// fiche routeur et vivent désormais dans la vue Protection (sidebar
// principale, /app/protection) — un argument de vente ne s'enterre pas
// dans une zone de configuration.
//
// Le code est le MÊME que celui livré en N°80/81/82 (mutations, toasts,
// invalidations, gardes mode agent) : seul le foyer change. L'onglet
// Système de la fiche routeur garde un RÉSUMÉ compact avec lien (aucun
// contrôle dupliqué). Le vocabulaire des clés tools.* est reformulé en
// bénéfices gérant (Mouvement 3 du plan N°83) — les clés restent stables.

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  ArrowRight,
  Baby,
  Clock,
  Globe,
  GlobeLock,
  Loader2,
  MoonStar,
  Shield,
  ShieldCheck,
  ShieldOff,
} from "lucide-react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
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

// ─── SafeWiFi (N°80) ───

const safeWifiLevels: {
  level: SafeWifiLevel;
  icon: typeof Shield;
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

  return (
    <Card className="gap-0 py-0">
      <CardContent className="p-4 sm:p-5">
        <div className="flex items-start gap-2">
          <Shield className="mt-0.5 size-4 shrink-0 text-primary" aria-hidden />
          <div>
            <h3 className="text-sm font-semibold">{t("tools.safewifi.title")}</h3>
            <p className="mt-0.5 text-xs text-muted-foreground">{t("tools.safewifi.desc")}</p>
          </div>
        </div>

        <div className="mt-3 space-y-2" role="radiogroup" aria-label={t("tools.safewifi.title")}>
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
                <span className="min-w-0">
                  <span className="flex flex-wrap items-center gap-2">
                    <span className="text-sm font-medium">{t(nameKey)}</span>
                    {active && <Badge className="h-5 px-1.5 text-[11px]">{t("tools.safewifi.activeBadge")}</Badge>}
                  </span>
                  <span className="mt-0.5 block text-xs text-muted-foreground">{t(descKey)}</span>
                </span>
              </button>
            );
          })}
        </div>

        <p className="mt-3 text-[11px] leading-relaxed text-muted-foreground">{t("tools.safewifi.footnote")}</p>
      </CardContent>
    </Card>
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
  const Icon = on ? ShieldCheck : ShieldOff;

  return (
    <Card className="gap-0 py-0">
      <CardContent className="p-4 sm:p-5">
        <div className="flex items-start gap-2">
          <Shield className="mt-0.5 size-4 shrink-0 text-primary" aria-hidden />
          <div>
            <h3 className="text-sm font-semibold">{t("tools.shield.title")}</h3>
            <p className="mt-0.5 text-xs text-muted-foreground">{t("tools.shield.desc")}</p>
          </div>
        </div>

        <div className="mt-3 flex items-center justify-between gap-3 rounded-lg border p-3">
          <div className="flex min-w-0 items-start gap-2">
            <Icon
              className={cn("mt-0.5 size-4 shrink-0", on ? "text-primary" : "text-muted-foreground")}
              aria-hidden
            />
            <span className="min-w-0">
              <span className="flex flex-wrap items-center gap-2">
                <span className="text-sm font-medium">{t("tools.shield.toggle")}</span>
                {on && <Badge className="h-5 px-1.5 text-[11px]">{t("tools.shield.activeBadge")}</Badge>}
              </span>
              <span className="mt-0.5 block text-xs text-muted-foreground">{t("tools.shield.toggleDesc")}</span>
            </span>
          </div>
          <Switch
            checked={on}
            onCheckedChange={(v) => levelMutation.mutate(v ? "on" : "off")}
            disabled={levelMutation.isPending || router.mode !== "agent"}
            aria-label={t("tools.shield.toggle")}
          />
        </div>

        <p className="mt-3 text-[11px] leading-relaxed text-muted-foreground">{t("tools.shield.footnote")}</p>
      </CardContent>
    </Card>
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
  const Icon = on ? GlobeLock : Globe;

  return (
    <Card className="gap-0 py-0">
      <CardContent className="p-4 sm:p-5">
        <div className="flex items-start gap-2">
          <GlobeLock className="mt-0.5 size-4 shrink-0 text-primary" aria-hidden />
          <div>
            <h3 className="text-sm font-semibold">{t("tools.antivpn.title")}</h3>
            <p className="mt-0.5 text-xs text-muted-foreground">{t("tools.antivpn.desc")}</p>
          </div>
        </div>

        <div className="mt-3 flex items-center justify-between gap-3 rounded-lg border p-3">
          <div className="flex min-w-0 items-start gap-2">
            <Icon
              className={cn("mt-0.5 size-4 shrink-0", on ? "text-primary" : "text-muted-foreground")}
              aria-hidden
            />
            <span className="min-w-0">
              <span className="flex flex-wrap items-center gap-2">
                <span className="text-sm font-medium">{t("tools.antivpn.toggle")}</span>
                {on && <Badge className="h-5 px-1.5 text-[11px]">{t("tools.antivpn.activeBadge")}</Badge>}
              </span>
              <span className="mt-0.5 block text-xs text-muted-foreground">{t("tools.antivpn.toggleDesc")}</span>
            </span>
          </div>
          <Switch
            checked={on}
            onCheckedChange={(v) => levelMutation.mutate(v ? "on" : "off")}
            disabled={levelMutation.isPending || router.mode !== "agent"}
            aria-label={t("tools.antivpn.toggle")}
          />
        </div>

        <p className="mt-3 text-[11px] leading-relaxed text-muted-foreground">{t("tools.antivpn.footnote")}</p>
      </CardContent>
    </Card>
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

  const Icon = activeNow ? MoonStar : Clock;

  return (
    <Card className="gap-0 py-0">
      <CardContent className="p-4 sm:p-5">
        <div className="flex items-start gap-2">
          <Icon className="mt-0.5 size-4 shrink-0 text-primary" aria-hidden />
          <div>
            <h3 className="text-sm font-semibold">{t("tools.familyguard.title")}</h3>
            <p className="mt-0.5 text-xs text-muted-foreground">{t("tools.familyguard.desc")}</p>
          </div>
        </div>

        <div className="mt-3 flex items-center justify-between gap-3 rounded-lg border p-3">
          <div className="flex min-w-0 items-start gap-2">
            <Icon
              className={cn("mt-0.5 size-4 shrink-0", activeNow ? "text-primary" : "text-muted-foreground")}
              aria-hidden
            />
            <span className="min-w-0">
              <span className="flex flex-wrap items-center gap-2">
                <span className="text-sm font-medium">{t("tools.familyguard.toggle")}</span>
                {saved.enabled && (
                  <Badge className="h-5 px-1.5 text-[11px]">{t("tools.familyguard.activeBadge")}</Badge>
                )}
              </span>
              <span className="mt-0.5 block text-xs text-muted-foreground">
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

        <div className="mt-3 grid grid-cols-2 gap-3">
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

        <fieldset className="mt-3">
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
          className="mt-3 w-full"
          disabled={mutation.isPending || router.mode !== "agent" || !editorValid || !dirty}
          onClick={() => mutation.mutate({ enabled: saved.enabled, start, end, days })}
        >
          {mutation.isPending && <Loader2 className="size-3.5 animate-spin" aria-hidden />}
          {t("tools.familyguard.save")}
        </Button>
        {!editorValid && (
          <p className="mt-1.5 text-[11px] text-destructive">{t("tools.familyguard.invalidWindow")}</p>
        )}

        <p className="mt-3 text-[11px] leading-relaxed text-muted-foreground">{t("tools.familyguard.footnote")}</p>
      </CardContent>
    </Card>
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
