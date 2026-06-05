"use client";

import { useQuery } from "@tanstack/react-query";
import type { BudgetSummaryResponse } from "@/types/api";

/**
 * Fetches budget summary from /api/budget.
 * Refetches every 30 seconds; also invalidated by SSE budget_alert events.
 */
export function useBudget() {
  return useQuery<BudgetSummaryResponse>({
    queryKey: ["budget"],
    queryFn: async () => {
      const res = await fetch("/api/budget");
      if (!res.ok) throw new Error("Failed to fetch budget summary");
      return res.json() as Promise<BudgetSummaryResponse>;
    },
    refetchInterval: 30_000,
  });
}
