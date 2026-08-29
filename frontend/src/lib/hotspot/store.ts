"use client";

import { create } from "zustand";
import { persist } from "zustand/middleware";
import type { AuthUser, ViewId } from "./types";

interface HotspotState {
  token: string | null;
  user: AuthUser | null;
  view: ViewId;
  sidebarOpen: boolean;
  setAuth: (token: string, user: AuthUser) => void;
  logout: () => void;
  setView: (view: ViewId) => void;
  setSidebarOpen: (open: boolean) => void;
}

export const useHotspotStore = create<HotspotState>()(
  persist(
    (set) => ({
      token: null,
      user: null,
      view: "dashboard",
      sidebarOpen: false,
      setAuth: (token, user) => set({ token, user }),
      logout: () => set({ token: null, user: null, view: "dashboard" }),
      setView: (view) => set({ view, sidebarOpen: false }),
      setSidebarOpen: (sidebarOpen) => set({ sidebarOpen }),
    }),
    {
      name: "mikcloud-auth",
      partialize: (s) => ({ token: s.token, user: s.user }),
    },
  ),
);
