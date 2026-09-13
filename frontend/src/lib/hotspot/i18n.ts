"use client";

// i18n MikCloud (F11 — CONTRACT-V2) : dictionnaire PLAT fr/en (~430 clés),
// résolution maison légère (next-intl n'est pas utilisé). La langue vit dans
// le store zustand (champ `lang`, persisté avec token/user dans localStorage
// « mikcloud-auth ») ; le hook useI18n() expose t() (traduction simple),
// tf() (interpolation {variable}) et setLang().
//
// Règles :
// - t(lang, key) : la clé manquante en anglais retombe sur le français
//   (sécurité), puis sur `fallback`, puis sur la clé elle-même.
// - tf(lang, key, vars) : remplace chaque {nom} présent dans `vars`.
// - Les données serveur (noms, messages d'erreur API) ne passent PAS ici.

import { useEffect, useReducer } from "react";

import { useHotspotStore } from "./store";

// Fragments FR par domaine (N°87) — fusion ci-dessous, ordre alphabétique.
import { frAccounts } from "./i18n-fr/accounts";
import { frBadge } from "./i18n-fr/badge";
import { frBanner } from "./i18n-fr/banner";
import { frBillingRequests } from "./i18n-fr/billing-requests";
import { frCommon } from "./i18n-fr/common";
import { frDashboard } from "./i18n-fr/dashboard";
import { frForgot } from "./i18n-fr/forgot";
import { frHomeNet } from "./i18n-fr/homenet";
import { frHotspot } from "./i18n-fr/hotspot";
import { frHourly } from "./i18n-fr/hourly";
import { frJoin } from "./i18n-fr/join";
import { frJoinPage } from "./i18n-fr/join-page";
import { frLogin } from "./i18n-fr/login";
import { frLogs } from "./i18n-fr/logs";
import { frNav } from "./i18n-fr/nav";
import { frNotif } from "./i18n-fr/notif";
import { frPaywall } from "./i18n-fr/paywall";
import { frPlatform } from "./i18n-fr/platform";
import { frPlatformLogs } from "./i18n-fr/platform-logs";
import { frPlatformSettings } from "./i18n-fr/platform-settings";
import { frPlatformTeam } from "./i18n-fr/platform-team";
import { frPortal } from "./i18n-fr/portal";
import { frPrint } from "./i18n-fr/print";
import { frProfile } from "./i18n-fr/profile";
import { frProfiles } from "./i18n-fr/profiles";
import { frProtection } from "./i18n-fr/protection";
import { frPwa } from "./i18n-fr/pwa";
import { frReports } from "./i18n-fr/reports";
import { frResellers } from "./i18n-fr/resellers";
import { frReset } from "./i18n-fr/reset";
import { frRouters } from "./i18n-fr/routers";
import { frSell } from "./i18n-fr/sell";
import { frSessions } from "./i18n-fr/sessions";
import { frSettings } from "./i18n-fr/settings";
import { frShell } from "./i18n-fr/shell";
import { frSignup } from "./i18n-fr/signup";
import { frSub } from "./i18n-fr/sub";
import { frSubView } from "./i18n-fr/sub-view";
import { frTeam } from "./i18n-fr/team";
import { frTemplates } from "./i18n-fr/templates";
import { frTheme } from "./i18n-fr/theme";
import { frTools } from "./i18n-fr/tools";
import { frTopbar } from "./i18n-fr/topbar";
import { frUsers } from "./i18n-fr/users";
import { frVouchers } from "./i18n-fr/vouchers";
import { frWifi } from "./i18n-fr/wifi";

export type Lang = "fr" | "en";

// N°87 — éclatement du monolithe (2 702 lignes, plus gros fichier du
// projet) : le dictionnaire FR (2 460 clés) vit désormais en fragments
// par domaine dans ./i18n-fr/<domaine>.ts, regroupés ici par fusion.
// Règle : une clé préfixée « foo. » vit dans i18n-fr/foo.ts (miroir
// anglais : i18n-en/foo.ts) — ajouter une clé ne touche plus qu'un petit
// fichier et la parité FR/EN se lit fichier à fichier. L'objet fusionné
// est strictement identique à l'ancien (mêmes clés, mêmes valeurs —
// vérifié clé par clé par script au moment du split) ; seul l'ordre
// d'insertion change (regroupement par domaine), sans effet : la
// résolution ne fait que des lookups directs.

// ─── Dictionnaire FR (fragments par domaine, N°87) ───

const fr: Record<string, string> = {
  ...frAccounts,
  ...frBadge,
  ...frBanner,
  ...frBillingRequests,
  ...frCommon,
  ...frDashboard,
  ...frForgot,
  ...frHomeNet,
  ...frHotspot,
  ...frHourly,
  ...frJoin,
  ...frJoinPage,
  ...frLogin,
  ...frLogs,
  ...frNav,
  ...frNotif,
  ...frPaywall,
  ...frPlatform,
  ...frPlatformLogs,
  ...frPlatformSettings,
  ...frPlatformTeam,
  ...frPortal,
  ...frPrint,
  ...frProfile,
  ...frProfiles,
  ...frProtection,
  ...frPwa,
  ...frReports,
  ...frResellers,
  ...frReset,
  ...frRouters,
  ...frSell,
  ...frSessions,
  ...frSettings,
  ...frShell,
  ...frSignup,
  ...frSub,
  ...frSubView,
  ...frTeam,
  ...frTemplates,
  ...frTheme,
  ...frTools,
  ...frTopbar,
  ...frUsers,
  ...frVouchers,
  ...frWifi,
};

// ─── Dictionnaire EN — chargement asynchrone (N°78) ───
// Le dictionnaire anglais vit dans un module séparé (i18n-en.ts) chargé
// par import() dynamique UNIQUEMENT quand la langue EN est active : les
// utilisateurs FR ne paient plus ~37 Ko gzip dans le bundle initial.
// Pendant le (très court) chargement, t() replie sur le français — le
// contrat historique « clé EN manquante → français » est conservé.

let enDict: Record<string, string> | null = null;
let enLoadPromise: Promise<void> | null = null;

/**
 * Charge le dictionnaire EN au premier besoin (promesse mémoïsée).
 * En cas d'échec réseau, la promesse est réarmée et t() continue de
 * rejeter silencieusement sur le français.
 */
export function ensureEnDict(): Promise<void> {
  if (enDict || typeof window === "undefined") return Promise.resolve();
  if (!enLoadPromise) {
    enLoadPromise = import("./i18n-en")
      .then((m) => {
        enDict = m.enDict;
      })
      .catch(() => {
        enLoadPromise = null;
      });
  }
  return enLoadPromise;
}


// ─── Résolution ───

/**
 * Traduction simple. Ordre de résolution en anglais : enDict[key] (chargé
 * asynchronement — pendant le chargement ou en cas d'échec, on retombe
 * sur le français) → fr[key] (sécurité : une clé absente du dictionnaire
 * anglais retombe sur le français) → fallback → la clé elle-même. En
 * français : fr[key] → fallback → la clé.
 */
export function t(lang: Lang, key: string, fallback?: string): string {
  if (lang === "en") {
    const v = enDict?.[key];
    if (v !== undefined) return v;
    const f = fr[key];
    if (f !== undefined) return f;
    return fallback ?? key;
  }
  return fr[key] ?? fallback ?? key;
}

/**
 * Traduction avec interpolation : chaque occurrence de {nom} présente dans
 * `vars` est remplacée par sa valeur. Les placeholders absents de `vars`
 * (ex. les littéraux {{price}} des modèles de vouchers) sont conservés
 * tels quels.
 */
export function tf(lang: Lang, key: string, vars: Record<string, string | number>): string {
  const raw = t(lang, key);
  return raw.replace(/\{(\w+)\}/g, (match, name: string) =>
    name in vars ? String(vars[name]) : match,
  );
}

/** Locale Intl associée à la langue. */
export function localeOf(lang: Lang): string {
  return lang === "fr" ? "fr-FR" : "en-GB";
}

/**
 * Hook i18n : langue courante + traducteurs liés. La langue vit dans le
 * store zustand (persistée) — le changement est immédiat, sans rechargement.
 * Le dictionnaire EN est amorcé ici : si la langue active est EN, son
 * import dynamique est déclenché et l'arrivée du module provoque un
 * re-rendu (bump de compteur) pour afficher les textes anglais.
 */
export function useI18n(): {
  lang: Lang;
  setLang: (lang: Lang) => void;
  t: (key: string, fallback?: string) => string;
  tf: (key: string, vars: Record<string, string | number>) => string;
} {
  const lang = useHotspotStore((s) => s.lang);
  const setLang = useHotspotStore((s) => s.setLang);
  // Re-rendu quand le dictionnaire EN arrive (import dynamique asynchrone).
  const [, bumpEn] = useReducer((x: number) => x + 1, 0);
  useEffect(() => {
    if (lang === "en") void ensureEnDict().then(bumpEn);
  }, [lang, bumpEn]);
  return {
    lang,
    setLang,
    t: (key, fallback) => t(lang, key, fallback),
    tf: (key, vars) => tf(lang, key, vars),
  };
}

