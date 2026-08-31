"use client";

import { create } from "zustand";
import { persist } from "zustand/middleware";
import type { Lang } from "./i18n";
import type { AuthUser, ViewId } from "./types";

/** Mode de navigation — la console plateforme (propriétaire du SaaS) est
 * distincte de la console client ; l'admin plateforme bascule vers la console
 * de n'importe quel client via une session support (impersonation). */
export type ShellMode = "platform" | "client";

interface HotspotState {
  token: string | null;
  user: AuthUser | null;
  view: ViewId;
  sidebarOpen: boolean;
  /** Langue de l'interface (F11) — défaut « fr », persistée avec token/user. */
  lang: Lang;
  /** Console active pour l'admin plateforme (ignoré pour les autres rôles). */
  shellMode: ShellMode;
  /** Session support en cours : token + user D'ORIGINE de l'admin plateforme
   * (l'admin consulte la console d'un client avec un token scoping ce compte).
   * null = l'admin navigue sur SA console plateforme. */
  ownToken: string | null;
  ownUser: AuthUser | null;
  setAuth: (token: string, user: AuthUser) => void;
  logout: () => void;
  setView: (view: ViewId) => void;
  setSidebarOpen: (open: boolean) => void;
  setLang: (lang: Lang) => void;
  /** Bascule manuelle de console (vue par défaut du mode appliquée). */
  setShellMode: (mode: ShellMode) => void;
  /** Ouvre la session support : mémorise la session plateforme, active le
   * token scoping le compte client et atterrit sur le dashboard client. */
  impersonate: (token: string, user: AuthUser) => void;
  /** Quitte la session support : restaure la session plateforme. */
  exitImpersonation: () => void;
}

const VIEW_DEFAULTS: Record<ShellMode, ViewId> = {
  platform: "platform",
  client: "dashboard",
};

export const useHotspotStore = create<HotspotState>()(
  persist(
    (set, get) => ({
      token: null,
      user: null,
      view: "dashboard",
      sidebarOpen: false,
      lang: "fr",
      shellMode: "client",
      ownToken: null,
      ownUser: null,
      // L'admin plateforme (propriétaire du SaaS) atterrit sur sa CONSOLE
      // PLATEFORME à chaque connexion — pas sur un dashboard client.
      setAuth: (token, user) =>
        set({
          token,
          user,
          ownToken: null,
          ownUser: null,
          shellMode: user.role === "admin" || user.role === "platform_admin" ? "platform" : "client",
          view: user.role === "admin" || user.role === "platform_admin" ? "platform" : "dashboard",
        }),
      // La préférence de langue survit à la déconnexion (lang non réinitialisé).
      logout: () =>
        set({ token: null, user: null, view: "dashboard", shellMode: "client", ownToken: null, ownUser: null }),
      setView: (view) => set({ view, sidebarOpen: false }),
      setSidebarOpen: (sidebarOpen) => set({ sidebarOpen }),
      setLang: (lang) => set({ lang }),
      setShellMode: (mode) => set({ shellMode: mode, view: VIEW_DEFAULTS[mode], sidebarOpen: false }),
      impersonate: (token, user) =>
        set({
          ownToken: get().token,
          ownUser: get().user,
          token,
          user,
          shellMode: "client",
          view: "dashboard",
          sidebarOpen: false,
        }),
      exitImpersonation: () =>
        set((s) =>
          s.ownToken && s.ownUser
            ? { token: s.ownToken, user: s.ownUser, ownToken: null, ownUser: null, shellMode: "platform", view: "platform", sidebarOpen: false }
            : { shellMode: "platform", view: "platform", sidebarOpen: false },
        ),
    }),
    {
      name: "mikcloud-auth",
      partialize: (s) => ({
        token: s.token,
        user: s.user,
        lang: s.lang,
        shellMode: s.shellMode,
        ownToken: s.ownToken,
        ownUser: s.ownUser,
      }),
    },
  ),
);
