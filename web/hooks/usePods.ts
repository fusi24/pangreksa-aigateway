"use client";

import { useQuery } from "@tanstack/react-query";
import type { PodsResponse } from "@/types/api";

/**
 * Fetches registered daemon pods from /api/pods.
 * Refetches every 15 seconds; also invalidated by SSE pod_heartbeat events.
 */
export function usePods() {
  return useQuery<PodsResponse>({
    queryKey: ["pods"],
    queryFn: async () => {
      const res = await fetch("/api/pods");
      if (!res.ok) throw new Error("Failed to fetch pods");
      return res.json() as Promise<PodsResponse>;
    },
    refetchInterval: 15_000,
  });
}
