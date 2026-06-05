"use client";

import dynamic from "next/dynamic";
import type { EChartsOption } from "echarts";
import { registerCarbonTheme } from "@/lib/charts/echarts-theme";
import { useUIStore } from "@/store/ui";

// Lazy-load echarts-for-react to keep the initial JS bundle < 300 KB
const ReactECharts = dynamic(() => import("echarts-for-react"), { ssr: false });

// Register Carbon themes on first client render
registerCarbonTheme();

interface MetricsChartProps {
  /** ECharts option object defining the chart. */
  option: EChartsOption;
  /** Chart height in pixels. Defaults to 300. */
  height?: number;
  /** Show loading overlay while data is fetching. */
  loading?: boolean;
  /** Required accessible label describing what the chart shows. */
  ariaLabel: string;
}

/**
 * Carbon-themed ECharts wrapper component.
 * Automatically switches between carbon-white and carbon-dark themes
 * based on the active Carbon theme in the Zustand store.
 */
export function MetricsChart({
  option,
  height = 300,
  loading = false,
  ariaLabel,
}: MetricsChartProps) {
  const theme = useUIStore((s) => s.theme);
  const echartsTheme = theme === "white" ? "carbon-white" : "carbon-dark";

  return (
    <div role="img" aria-label={ariaLabel}>
      <ReactECharts
        option={option}
        theme={echartsTheme}
        style={{ height, width: "100%" }}
        showLoading={loading}
        opts={{ renderer: "canvas" }}
        notMerge
        lazyUpdate
      />
    </div>
  );
}
