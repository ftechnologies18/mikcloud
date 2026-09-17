"use client";

// Dictionnaire anglais MikCloud — extrait de i18n.ts (N°78) : chargé
// UNIQUEMENT pour les utilisateurs EN, via import() dynamique
// (ensureEnDict dans i18n.ts). Les utilisateurs FR ne le téléchargent
// jamais — c'est ~37 Ko gzip qui sortent du bundle initial pour la
// majorité du parc (Côte d'Ivoire).
// N°87 — éclatement du monolithe (2 589 lignes) : les 2 460 clés vivent
// en fragments par domaine dans ./i18n-en/<domaine>.ts (miroir exact de
// i18n-fr/ — mêmes domaines, mêmes clés). La parité FR/EN se vérifie
// désormais fichier à fichier ; l'objet fusionné est strictement identique
// à l'ancien (vérifié clé par clé par script au moment du split).

// Fragments EN par domaine (N°87) — fusion ci-dessous, ordre alphabétique.
import { enAccounts } from "./i18n-en/accounts";
import { enBadge } from "./i18n-en/badge";
import { enBanner } from "./i18n-en/banner";
import { enBillingRequests } from "./i18n-en/billing-requests";
import { enCommon } from "./i18n-en/common";
import { enDashboard } from "./i18n-en/dashboard";
import { enForgot } from "./i18n-en/forgot";
import { enHomeNet } from "./i18n-en/homenet";
import { enHotspot } from "./i18n-en/hotspot";
import { enHourly } from "./i18n-en/hourly";
import { enJoin } from "./i18n-en/join";
import { enJoinPage } from "./i18n-en/join-page";
import { enLogin } from "./i18n-en/login";
import { enLogs } from "./i18n-en/logs";
import { enNav } from "./i18n-en/nav";
import { enNotif } from "./i18n-en/notif";
import { enPaywall } from "./i18n-en/paywall";
import { enPlatform } from "./i18n-en/platform";
import { enPlatformLogs } from "./i18n-en/platform-logs";
import { enPlatformChat } from "./i18n-en/platform-chat";
import { enPlatformSettings } from "./i18n-en/platform-settings";
import { enPlatformTeam } from "./i18n-en/platform-team";
import { enPortal } from "./i18n-en/portal";
import { enPrint } from "./i18n-en/print";
import { enProfile } from "./i18n-en/profile";
import { enProfiles } from "./i18n-en/profiles";
import { enProtection } from "./i18n-en/protection";
import { enPwa } from "./i18n-en/pwa";
import { enReports } from "./i18n-en/reports";
import { enResellers } from "./i18n-en/resellers";
import { enReset } from "./i18n-en/reset";
import { enRouters } from "./i18n-en/routers";
import { enSell } from "./i18n-en/sell";
import { enSessions } from "./i18n-en/sessions";
import { enSettings } from "./i18n-en/settings";
import { enShell } from "./i18n-en/shell";
import { enSignup } from "./i18n-en/signup";
import { enSub } from "./i18n-en/sub";
import { enSubView } from "./i18n-en/sub-view";
import { enTeam } from "./i18n-en/team";
import { enTemplates } from "./i18n-en/templates";
import { enTheme } from "./i18n-en/theme";
import { enTools } from "./i18n-en/tools";
import { enTopbar } from "./i18n-en/topbar";
import { enUsers } from "./i18n-en/users";
import { enVouchers } from "./i18n-en/vouchers";
import { enWifi } from "./i18n-en/wifi";

/** Dictionnaire plat clé → texte anglais (fragments par domaine, N°87). */
export const enDict: Record<string, string> = {
  ...enAccounts,
  ...enBadge,
  ...enBanner,
  ...enBillingRequests,
  ...enCommon,
  ...enDashboard,
  ...enForgot,
  ...enHomeNet,
  ...enHotspot,
  ...enHourly,
  ...enJoin,
  ...enJoinPage,
  ...enLogin,
  ...enLogs,
  ...enNav,
  ...enNotif,
  ...enPaywall,
  ...enPlatform,
  ...enPlatformLogs,
  ...enPlatformChat,
  ...enPlatformSettings,
  ...enPlatformTeam,
  ...enPortal,
  ...enPrint,
  ...enProfile,
  ...enProfiles,
  ...enProtection,
  ...enPwa,
  ...enReports,
  ...enResellers,
  ...enReset,
  ...enRouters,
  ...enSell,
  ...enSessions,
  ...enSettings,
  ...enShell,
  ...enSignup,
  ...enSub,
  ...enSubView,
  ...enTeam,
  ...enTemplates,
  ...enTheme,
  ...enTools,
  ...enTopbar,
  ...enUsers,
  ...enVouchers,
  ...enWifi,
};
