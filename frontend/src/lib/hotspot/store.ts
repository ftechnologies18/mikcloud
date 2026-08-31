"use client";

import { create } from "zustand";
import { persist } from "zustand/middleware";
import type { Lang } from "./i18n";
import type { AuthUser, ViewId } from "./types";

/** Mode de navigation — la console plateforme (propriétaire du SaaS) est
 * distincte de la console client ; l'admin plateforme bascule entre les deux. */
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
  setAuth: (token: string, user: AuthUser) => void;
  logout: () => void;
  setView: (view: ViewId) => void;
  setSidebarOpen: (open: boolean) => void;
  setLang: (lang: Lang) => void;
  /** Bascule Plateforme ↔ Ma console (vue par défaut du mode appliquée). */
  setShellMode: (mode: ShellMode) => void;
}

const VIEW_DEFAULTS: Record<ShellMode, ViewId> = {
  platform: "platform",
  client: "dashboard",
};

export const useHotspotStore = create<HotspotState>()(
  persist(
    (set) => ({
      token: null,
      user: null,
      view: "dashboard",
      sidebarOpen: false,
      lang: "fr",
      shellMode: "client",
      // L'admin plateforme (propriétaire du SaaS) atterrit sur sa CONSOLE
      // PLATEFORME à chaque connexion — pas sur un dashboard client.
      setAuth: (token, user) =>
        set({
          token,
          user,
          shellMode: user.role === "admin" || user.role === "platform_admin" ? "platform" : "client",
          view: user.role === "admin" || user.role === "platform_admin" ? "platform" : "dashboard",
        }),
      // La préférence de langue survit à la déconnexion (lang non réinitialisé).
      logout: () => set({ token: null, user: null, view: "dashboard", shellMode: "client" }),
      setView: (view) => set({ view, sidebarOpen: false }),
      setSidebarOpen: (sidebarOpen) => set({ sidebarOpen }),
      setLang: (lang) => set({ lang }),
      setShellMode: (mode) => set({ shellMode: mode, view: VIEW_DEFAULTS[mode], sidebarOpen: false }),
    }),
    {
      name: "mikcloud-auth",
      partialize: (s) => ({ token: s.token, user: s.user, lang: s.lang }),
    },
  ),
);
