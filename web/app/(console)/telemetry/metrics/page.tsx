"use client";

import { useState } from "react";
import { useMutation } from "@tanstack/react-query";
import {
  Grid,
  Column,
  Tile,
  TextArea,
  Button,
  InlineNotification,
  ContentSwitcher,
  Switch,
} from "@carbon/react";
import { MetricsChart } from "@/components/charts/MetricsChart";
import type { EChartsOption } from "echarts";

interface PrometheusResult {
  status: string;
  data: {
    resultType: string;
    result: Array<{
      metric: Record<string, string>;
      values: Array<[number, string]>;
    }>;
  };
}

const PRESET_QUERIES = [
  { label: "Goroutines", query: "go_goroutines", unit: "" },
  { label: "GC Pause", query: "go_gc_duration_seconds_sum", unit: "s" },
  { label: "Memory (RSS)", query: "process_resident_memory_bytes", unit: "bytes" },
  { label: "CPU Usage", query: "rate(process_cpu_seconds_total[5m])", unit: "" },
];

/**
 * Prometheus metrics explorer page.
 * Supports preset metric tiles and a free-form PromQL input.
 */
export default function MetricsExplorerPage() {
  const [promql, setPromql] = useState("");
  const [viewMode, setViewMode] = useState<"chart" | "table">("chart");
  const [queryResult, setQueryResult] = useState<PrometheusResult | null>(null);
  const [queryError, setQueryError] = useState<string | null>(null);

  const now = Math.floor(Date.now() / 1000);
  const oneHourAgo = now - 3600;

  const runQuery = useMutation({
    mutationFn: async (query: string) => {
      const params = new URLSearchParams({
        query,
        start: String(oneHourAgo),
        end: String(now),
        step: "60",
      });
      const res = await fetch(`/api/telemetry/metrics?${params.toString()}`);
      if (!res.ok) throw new Error("Query failed");
      return res.json() as Promise<PrometheusResult>;
    },
    onSuccess: (data) => {
      setQueryResult(data);
      setQueryError(null);
    },
    onError: (err: Error) => {
      setQueryError(err.message);
    },
  });

  function buildChartOption(result: PrometheusResult): EChartsOption {
    if (!result.data?.result?.length) return {};

    const series = result.data.result.map((r) => ({
      name: JSON.stringify(r.metric),
      type: "line" as const,
      data: r.values.map(([ts, val]: [number, string]) => [ts * 1000, parseFloat(val)]),
      smooth: true,
    }));

    return {
      tooltip: { trigger: "axis" },
      legend: {},
      xAxis: { type: "time" },
      yAxis: { type: "value" },
      series,
    };
  }

  return (
    <div>
      <h1 style={{ fontSize: "1.75rem", fontWeight: 600, marginBottom: "1.5rem" }}>
        Metrics Explorer
      </h1>

      {/* Preset metric tiles */}
      <Grid style={{ marginBottom: "2rem" }}>
        {PRESET_QUERIES.map((preset) => (
          <Column key={preset.label} sm={2} md={2} lg={4}>
            <Tile
              style={{ cursor: "pointer", padding: "1rem" }}
              onClick={() => {
                setPromql(preset.query);
                runQuery.mutate(preset.query);
              }}
            >
              <div style={{ fontWeight: 600, marginBottom: "0.5rem" }}>{preset.label}</div>
              <code style={{ fontSize: "0.75rem", color: "#525252", fontFamily: "IBM Plex Mono, monospace" }}>
                {preset.query}
              </code>
            </Tile>
          </Column>
        ))}
      </Grid>

      {/* Free-form PromQL */}
      <Tile style={{ marginBottom: "1rem" }}>
        <h3 style={{ marginBottom: "1rem" }}>Custom PromQL Query</h3>
        <TextArea
          id="promql-input"
          labelText="PromQL expression"
          placeholder="e.g. rate(http_requests_total[5m])"
          value={promql}
          onChange={(e) => setPromql(e.target.value)}
          rows={3}
        />
        <div style={{ display: "flex", gap: "1rem", marginTop: "1rem", alignItems: "center" }}>
          <Button
            size="md"
            onClick={() => { if (promql.trim()) runQuery.mutate(promql.trim()); }}
            disabled={runQuery.isPending || !promql.trim()}
          >
            {runQuery.isPending ? "Running…" : "Run Query"}
          </Button>

          <ContentSwitcher
            selectedIndex={viewMode === "chart" ? 0 : 1}
            onChange={({ index }) => setViewMode(index === 0 ? "chart" : "table")}
            size="sm"
          >
            <Switch name="chart" text="Chart" />
            <Switch name="table" text="Table" />
          </ContentSwitcher>
        </div>
      </Tile>

      {queryError && (
        <InlineNotification kind="error" title="Query failed" subtitle={queryError} />
      )}

      {queryResult && viewMode === "chart" && (
        <Tile>
          <MetricsChart
            option={buildChartOption(queryResult)}
            height={400}
            ariaLabel="PromQL query result chart"
          />
        </Tile>
      )}

      {queryResult && viewMode === "table" && (
        <Tile>
          <pre style={{ fontFamily: "IBM Plex Mono, monospace", fontSize: "0.75rem", overflowX: "auto" }}>
            {JSON.stringify(queryResult.data?.result, null, 2)}
          </pre>
        </Tile>
      )}
    </div>
  );
}
