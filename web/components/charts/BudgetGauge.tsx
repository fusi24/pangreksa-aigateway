"use client";

import type { EChartsOption } from "echarts";
import { MetricsChart } from "./MetricsChart";

interface BudgetGaugeProps {
  ruleName: string;
  spendUsd: number;
  limitUsd: number;
}

/**
 * ECharts gauge showing budget consumption percentage.
 * Color thresholds: green < 75%, yellow 75–89%, orange 90–99%, red >= 100%.
 */
export function BudgetGauge({ ruleName, spendUsd, limitUsd }: BudgetGaugeProps) {
  const pct = limitUsd > 0 ? Math.min((spendUsd / limitUsd) * 100, 100) : 0;

  const color =
    pct >= 100 ? "#da1e28"
    : pct >= 90 ? "#ff832b"
    : pct >= 75 ? "#f1c21b"
    : "#198038";

  const option: EChartsOption = {
    series: [
      {
        type: "gauge",
        min: 0,
        max: 100,
        data: [{ value: Math.round(pct * 10) / 10, name: ruleName }],
        axisLine: {
          lineStyle: {
            color: [[pct / 100, color], [1, "#e0e0e0"]],
            width: 18,
          },
        },
        pointer: { show: false },
        axisTick: { show: false },
        splitLine: { show: false },
        axisLabel: { show: false },
        detail: {
          formatter: "{value:.1f}%",
          fontSize: 22,
          fontWeight: "bold",
          color: color,
          offsetCenter: [0, "30%"],
        },
        title: {
          fontSize: 12,
          color: "#525252",
          offsetCenter: [0, "55%"],
        },
      },
    ],
  };

  return (
    <MetricsChart
      option={option}
      height={180}
      ariaLabel={`Budget gauge: ${ruleName} at ${pct.toFixed(1)}% consumed ($${spendUsd.toFixed(2)} of $${limitUsd.toFixed(2)})`}
    />
  );
}
