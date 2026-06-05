"use client";

import { GlobalTheme } from "@carbon/react";
import { useUIStore } from "@/store/ui";

/**
 * Applies the Carbon global theme (White or Gray 100) based on Zustand store.
 */
export function ThemeProvider({ children }: { children: React.ReactNode }) {
  const theme = useUIStore((s) => s.theme);
  return <GlobalTheme theme={theme}>{children}</GlobalTheme>;
}
