"use client";

import { create } from "zustand";

export type TimeRange = "1h" | "6h" | "24h" | "7d" | "30d" | "custom";
export type CarbonTheme = "white" | "g100";

interface UIState {
  timeRange: TimeRange;
  customFrom: Date | null;
  customTo: Date | null;
  sideNavCollapsed: boolean;
  theme: CarbonTheme;
  setTimeRange: (r: TimeRange) => void;
  setCustomRange: (from: Date, to: Date) => void;
  toggleSideNav: () => void;
  toggleTheme: () => void;
}

/**
 * Global UI state store for time-range selection, theme, and sidebar state.
 */
export const useUIStore = create<UIState>((set) => ({
  timeRange: "24h",
  customFrom: null,
  customTo: null,
  sideNavCollapsed: false,
  theme: "white",

  setTimeRange: (timeRange) => set({ timeRange }),

  setCustomRange: (customFrom, customTo) =>
    set({ customFrom, customTo, timeRange: "custom" }),

  toggleSideNav: () =>
    set((s) => ({ sideNavCollapsed: !s.sideNavCollapsed })),

  toggleTheme: () =>
    set((s) => ({ theme: s.theme === "white" ? "g100" : "white" })),
}));
