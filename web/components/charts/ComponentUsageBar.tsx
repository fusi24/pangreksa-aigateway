"use client";

import type { EChartsOption } from "echarts";
import { MetricsChart } from "./MetricsChart";

interface ComponentUsageSeries {
  name: string;
  data: number[];
}

interface ComponentUsageBarProps {
  /** X-axis time labels (ISO strings or formatted timestamps). */
  categories: string[];
  /** One series per component (Prompt Registry, Skills, MCP, etc.). */
  series: ComponentUsageSeries[];
  height?: number;
  loading?: boolean;
}

/**
 * Stacked bar chart for per-component request volume.
 */
export function ComponentUsageBar({
  categories,
  series,
  height = 300,
  loading = false,
}: ComponentUsageBarProps) {
  const option: EChartsOption = {
    tooltip: { trigger: "axis", axisPointer: { type: "shadow" } },
    legend: { top: 0 },
    grid: { left: "3%", right: "4%", bottom: "3%", containLabel: true },
    xAxis: {
      type: "category",
      data: categories,
    },
    yAxis: {
      type: "value",
      name: "Requests",
    },
    series: series.map((s) => ({
      name: s.name,
      type: "bar",
      stack: "total",
      data: s.data,
      emphasis: { focus: "series" },
    })),
  };

  return (
    <MetricsChart
      option={option}
      height={height}
      loading={loading}
      ariaLabel="Stacked bar chart: request volume by component over time"
    />
  );
}
