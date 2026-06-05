"use client";

import { useQuery } from "@tanstack/react-query";
import { formatISO, subHours } from "date-fns";
import { useUIStore } from "@/store/ui";
import type { MetricsResponse } from "@/types/api";
import type { TimeRange } from "@/store/ui";

function resolveRange(
  timeRange: TimeRange,
  customFrom: Date | null,
  customTo: Date | null
): { from: string; to: string } {
  const now = new Date();

  if (timeRange === "custom" && customFrom && customTo) {
    return { from: formatISO(customFrom), to: formatISO(customTo) };
  }

  const hoursMap: Record<Exclude<TimeRange, "custom">, number> = {
    "1h": 1,
    "6h": 6,
    "24h": 24,
    "7d": 168,
    "30d": 720,
  };

  const hours = hoursMap[timeRange as Exclude<TimeRange, "custom">] ?? 24;
  return { from: formatISO(subHours(now, hours)), to: formatISO(now) };
}

/**
 * Fetches aggregated metrics from the BFF /api/metrics route.
 * Automatically re-queries when the global time-range selector changes.
 *
 * @param filters - Optional additional query filters (user_id, gateway_id, etc.)
 */
export function useMetrics(filters?: Record<string, string>) {
  const { timeRange, customFrom, customTo } = useUIStore();
  const { from, to } = resolveRange(timeRange, customFrom, customTo);

  return useQuery<MetricsResponse>({
    queryKey: ["metrics", { from, to, ...filters }],
    queryFn: async () => {
      const params = new URLSearchParams({ from, to, ...filters });
      const res = await fetch(`/api/metrics?${params.toString()}`);
      if (!res.ok) throw new Error("Failed to fetch metrics");
      return res.json() as Promise<MetricsResponse>;
    },
    refetchInterval: 30_000,
  });
}
