import { describe, it, expect, beforeEach } from "vitest";
import { useUIStore } from "@/store/ui";

describe("useUIStore", () => {
  beforeEach(() => {
    // Reset store to initial state before each test
    useUIStore.setState({
      timeRange: "24h",
      customFrom: null,
      customTo: null,
      sideNavCollapsed: false,
      theme: "white",
    });
  });

  it("has correct initial state", () => {
    const state = useUIStore.getState();
    expect(state.timeRange).toBe("24h");
    expect(state.theme).toBe("white");
    expect(state.sideNavCollapsed).toBe(false);
  });

  it("setTimeRange updates the range", () => {
    useUIStore.getState().setTimeRange("7d");
    expect(useUIStore.getState().timeRange).toBe("7d");
  });

  it("toggleTheme switches between white and g100", () => {
    useUIStore.getState().toggleTheme();
    expect(useUIStore.getState().theme).toBe("g100");
    useUIStore.getState().toggleTheme();
    expect(useUIStore.getState().theme).toBe("white");
  });

  it("toggleSideNav flips collapsed state", () => {
    useUIStore.getState().toggleSideNav();
    expect(useUIStore.getState().sideNavCollapsed).toBe(true);
    useUIStore.getState().toggleSideNav();
    expect(useUIStore.getState().sideNavCollapsed).toBe(false);
  });

  it("setCustomRange sets custom dates and timeRange to custom", () => {
    const from = new Date("2026-01-01");
    const to = new Date("2026-01-31");
    useUIStore.getState().setCustomRange(from, to);
    const state = useUIStore.getState();
    expect(state.timeRange).toBe("custom");
    expect(state.customFrom).toEqual(from);
    expect(state.customTo).toEqual(to);
  });
});
