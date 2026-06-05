"use client";

import { Column, Tile, InlineNotification } from "@carbon/react";
import { MetricsChart } from "@/components/charts/MetricsChart";
import { ComponentUsageBar } from "@/components/charts/ComponentUsageBar";
import { useMetrics } from "@/hooks/useMetrics";
import { fmtUsd, fmtMs, fmtTokens } from "@/lib/utils/format";
import type { EChartsOption } from "echarts";
import type { MetricPoint } from "@/types/api";

/** Extracts time labels and values from a named series. */
function getSeries(
  seriesData: Array<{ metric: string; points: MetricPoint[] }>,
  metric: string
): { categories: string[]; values: number[] } {
  const series = seriesData.find((s) => s.metric === metric);
  if (!series) return { categories: [], values: [] };
  return {
    categories: series.points.map((p) => {
      const d = new Date(p.ts);
      return `${d.getHours().toString().padStart(2, "0")}:${d.getMinutes().toString().padStart(2, "0")}`;
    }),
    values: series.points.map((p) => p.value),
  };
}

function lineOption(
  categories: string[],
  values: number[],
  name: string,
  yAxisName?: string
): EChartsOption {
  return {
    tooltip: { trigger: "axis" },
    grid: { left: "3%", right: "4%", bottom: "3%", containLabel: true },
    xAxis: { type: "category", data: categories, boundaryGap: false },
    yAxis: yAxisName !== undefined
      ? { type: "value", name: yAxisName }
      : { type: "value" },
    series: [{ name, type: "line", data: values, smooth: true, areaStyle: { opacity: 0.1 } }],
  };
}

function stackedBarTokensOption(
  categories: string[],
  inputValues: number[],
  outputValues: number[]
): EChartsOption {
  return {
    tooltip: { trigger: "axis", axisPointer: { type: "shadow" } },
    legend: { data: ["Input tokens", "Output tokens"] },
    grid: { left: "3%", right: "4%", bottom: "3%", containLabel: true },
    xAxis: { type: "category", data: categories },
    yAxis: { type: "value" },
    series: [
      { name: "Input tokens", type: "bar", stack: "total", data: inputValues },
      { name: "Output tokens", type: "bar", stack: "total", data: outputValues },
    ],
  };
}

/**
 * Renders all 6 metric chart panels using data from the useMetrics hook.
 */
export function DashboardCharts() {
  const { data, isLoading, error } = useMetrics();

  if (error) {
    return (
      <Column sm={4} md={8} lg={16}>
        <InlineNotification
          kind="error"
          title="Failed to load metrics"
          subtitle={(error as Error).message}
        />
      </Column>
    );
  }

  const seriesData = data?.series ?? [];
  const summary = data?.summary;

  const latencyP95 = getSeries(seriesData, "latency_p95_ms");
  const latencyP99 = getSeries(seriesData, "latency_p99_ms");
  const rpm = getSeries(seriesData, "rpm");
  const inputTokens = getSeries(seriesData, "input_tokens");
  const outputTokens = getSeries(seriesData, "output_tokens");
  const costUsd = getSeries(seriesData, "cost_usd");
  const errorRate = getSeries(seriesData, "error_rate");

  const multiLatencyOption: EChartsOption = {
    tooltip: { trigger: "axis" },
    legend: { data: ["P95 latency", "P99 latency"] },
    grid: { left: "3%", right: "4%", bottom: "3%", containLabel: true },
    xAxis: { type: "category", data: latencyP95.categories, boundaryGap: false },
    yAxis: { type: "value", name: "ms" },
    series: [
      { name: "P95 latency", type: "line", data: latencyP95.values, smooth: true },
      { name: "P99 latency", type: "line", data: latencyP99.values, smooth: true },
    ],
  };

  const costOption: EChartsOption = {
    ...lineOption(costUsd.categories, costUsd.values, "Cost (USD)", "USD/hr"),
    series: [
      {
        name: "Cost (USD)",
        type: "line",
        data: costUsd.values,
        smooth: true,
        markLine: {
          data: [{ type: "average", name: "Avg" }],
          lineStyle: { color: "#ff832b", type: "dashed" },
          label: { formatter: "Avg: {c}" },
        },
      },
    ],
  };

  return (
    <>
      {/* Row 1: KPI tiles */}
      <Column sm={2} md={2} lg={3}>
        <Tile style={{ textAlign: "center", padding: "1.5rem" }}>
          <div style={{ fontSize: "2rem", fontWeight: 700, color: "#0f62fe" }}>
            {summary ? summary.total_requests.toLocaleString() : "—"}
          </div>
          <div style={{ color: "#525252", fontSize: "0.875rem" }}>Total Requests</div>
        </Tile>
      </Column>
      <Column sm={2} md={2} lg={3}>
        <Tile style={{ textAlign: "center", padding: "1.5rem" }}>
          <div style={{ fontSize: "2rem", fontWeight: 700, color: "#198038" }}>
            {summary ? fmtUsd(summary.total_cost_usd) : "—"}
          </div>
          <div style={{ color: "#525252", fontSize: "0.875rem" }}>Total Cost</div>
        </Tile>
      </Column>
      <Column sm={2} md={2} lg={3}>
        <Tile style={{ textAlign: "center", padding: "1.5rem" }}>
          <div style={{ fontSize: "2rem", fontWeight: 700, color: "#8a3ffc" }}>
            {summary ? fmtMs(Math.round(summary.avg_latency_p95_ms)) : "—"}
          </div>
          <div style={{ color: "#525252", fontSize: "0.875rem" }}>Avg P95 Latency</div>
        </Tile>
      </Column>
      <Column sm={2} md={2} lg={3}>
        <Tile style={{ textAlign: "center", padding: "1.5rem" }}>
          <div style={{ fontSize: "2rem", fontWeight: 700, color: summary && summary.error_rate_pct > 5 ? "#da1e28" : "#161616" }}>
            {summary ? `${summary.error_rate_pct.toFixed(1)}%` : "—"}
          </div>
          <div style={{ color: "#525252", fontSize: "0.875rem" }}>Error Rate</div>
        </Tile>
      </Column>

      {/* Row 2: Latency + Throughput */}
      <Column sm={4} md={4} lg={8} style={{ marginTop: "1.5rem" }}>
        <Tile>
          <h4 style={{ marginBottom: "1rem" }}>Latency (P95 / P99)</h4>
          <MetricsChart
            option={multiLatencyOption}
            loading={isLoading}
            ariaLabel="Line chart showing P95 and P99 request latency in milliseconds over time"
          />
        </Tile>
      </Column>
      <Column sm={4} md={4} lg={8} style={{ marginTop: "1.5rem" }}>
        <Tile>
          <h4 style={{ marginBottom: "1rem" }}>Throughput (RPM)</h4>
          <MetricsChart
            option={lineOption(rpm.categories, rpm.values, "Requests/min", "RPM")}
            loading={isLoading}
            ariaLabel="Area chart showing requests per minute over time"
          />
        </Tile>
      </Column>

      {/* Row 3: Token consumption + Cost */}
      <Column sm={4} md={4} lg={8} style={{ marginTop: "1.5rem" }}>
        <Tile>
          <h4 style={{ marginBottom: "1rem" }}>
            Token Consumption (Total: {summary ? fmtTokens(summary.total_input_tokens + summary.total_output_tokens) : "—"})
          </h4>
          <MetricsChart
            option={stackedBarTokensOption(
              inputTokens.categories,
              inputTokens.values,
              outputTokens.values
            )}
            loading={isLoading}
            ariaLabel="Stacked bar chart showing input and output token consumption over time"
          />
        </Tile>
      </Column>
      <Column sm={4} md={4} lg={8} style={{ marginTop: "1.5rem" }}>
        <Tile>
          <h4 style={{ marginBottom: "1rem" }}>Cost / Budget</h4>
          <MetricsChart
            option={costOption}
            loading={isLoading}
            ariaLabel="Line chart showing spend rate over time with average threshold marker"
          />
        </Tile>
      </Column>

      {/* Row 4: Component usage + Error rate */}
      <Column sm={4} md={4} lg={8} style={{ marginTop: "1.5rem" }}>
        <Tile>
          <h4 style={{ marginBottom: "1rem" }}>Component Usage</h4>
          <ComponentUsageBar
            categories={inputTokens.categories}
            series={[
              { name: "Prompt Registry", data: getSeries(seriesData, "component_prompt").values },
              { name: "Skill Registry", data: getSeries(seriesData, "component_skill").values },
              { name: "MCP Registry", data: getSeries(seriesData, "component_mcp").values },
              { name: "Guardrails", data: getSeries(seriesData, "component_guardrail").values },
              { name: "Budget Policy", data: getSeries(seriesData, "component_budget").values },
            ]}
            loading={isLoading}
          />
        </Tile>
      </Column>
      <Column sm={4} md={4} lg={8} style={{ marginTop: "1.5rem" }}>
        <Tile>
          <h4 style={{ marginBottom: "1rem" }}>Error Rate (%)</h4>
          <MetricsChart
            option={{
              tooltip: { trigger: "axis" },
              grid: { left: "3%", right: "4%", bottom: "3%", containLabel: true },
              xAxis: { type: "category", data: errorRate.categories, boundaryGap: false },
              yAxis: { type: "value", name: "%", max: 100 },
              visualMap: {
                show: false,
                pieces: [
                  { lte: 1, color: "#198038" },
                  { gt: 1, lte: 5, color: "#f1c21b" },
                  { gt: 5, color: "#da1e28" },
                ],
              },
              series: [{
                name: "Error rate",
                type: "line",
                data: errorRate.values,
                smooth: true,
                areaStyle: { opacity: 0.2 },
              }],
            }}
            loading={isLoading}
            ariaLabel="Line chart showing error rate percentage over time with color thresholds"
          />
        </Tile>
      </Column>
    </>
  );
}
