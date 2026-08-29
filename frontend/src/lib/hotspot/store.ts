"use client";

import { create } from "zustand";
import { persist } from "zustand/middleware";
import type { Lang } from "./i18n";
import type { AuthUser, ViewId } from "./types";

interface HotspotState {
  token: string | null;
  user: AuthUser | null;
  view: ViewId;
  sidebarOpen: boolean;
  /** Langue de l'interface (F11) — défaut « fr », persistée avec token/user. */
  lang: Lang;
  setAuth: (token: string, user: AuthUser) => void;
  logout: () => void;
  setView: (view: ViewId) => void;
  setSidebarOpen: (open: boolean) => void;
  setLang: (lang: Lang) => void;
}

export const useHotspotStore = create<HotspotState>()(
  persist(
    (set) => ({
      token: null,
      user: null,
      view: "dashboard",
      sidebarOpen: false,
      lang: "fr",
      setAuth: (token, user) => set({ token, user }),
      // La préférence de langue survit à la déconnexion (lang non réinitialisé).
      logout: () => set({ token: null, user: null, view: "dashboard" }),
      setView: (view) => set({ view, sidebarOpen: false }),
      setSidebarOpen: (sidebarOpen) => set({ sidebarOpen }),
      setLang: (lang) => set({ lang }),
    }),
    {
      name: "mikcloud-auth",
      partialize: (s) => ({ token: s.token, user: s.user, lang: s.lang }),
    },
  ),
);
