"use client";

import { useEffect, useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { createSseClient } from "@/lib/utils/sse";
import type { TransactionEvent, PodHeartbeatEvent, BudgetAlertEvent } from "@/types/api";

const MAX_FEED_ROWS = 200;

interface LiveFeedState {
  feed: TransactionEvent[];
  paused: boolean;
  connected: boolean;
  togglePause: () => void;
}

/**
 * Manages the SSE live telemetry feed.
 * - Maintains a rolling buffer of the last 200 transaction events
 * - Pausing stops adding to the buffer but keeps the SSE connection open
 * - pod_heartbeat events trigger TanStack Query invalidation for the pods list
 * - budget_alert events trigger invalidation for the budget summary
 */
export function useLiveFeed(): LiveFeedState {
  const [feed, setFeed] = useState<TransactionEvent[]>([]);
  const [paused, setPaused] = useState(false);
  const [connected, setConnected] = useState(false);
  const pausedRef = useRef(false);
  const qc = useQueryClient();

  useEffect(() => {
    const cleanup = createSseClient({
      url: "/api/sse/telemetry",
      onOpen: () => setConnected(true),
      onError: () => setConnected(false),
      onEvent: (type, data) => {
        if (type === "transaction") {
          if (!pausedRef.current) {
            setFeed((prev) =>
              [data as TransactionEvent, ...prev].slice(0, MAX_FEED_ROWS)
            );
          }
        } else if (type === "pod_heartbeat") {
          const evt = data as PodHeartbeatEvent;
          qc.invalidateQueries({ queryKey: ["pods"] });
          // Also update pod status in query cache directly for immediate UI update
          qc.setQueryData<{ pods: Array<{ gateway_id: string; status: string }> }>(
            ["pods"],
            (old) => {
              if (!old) return old;
              return {
                ...old,
                pods: old.pods.map((p) =>
                  p.gateway_id === evt.gateway_id
                    ? { ...p, status: evt.status, last_seen_at: evt.last_seen_at }
                    : p
                ),
              };
            }
          );
        } else if (type === "budget_alert") {
          const _evt = data as BudgetAlertEvent;
          qc.invalidateQueries({ queryKey: ["budget"] });
        }
      },
    });

    return cleanup;
  }, [qc]);

  function togglePause() {
    pausedRef.current = !pausedRef.current;
    setPaused(pausedRef.current);
  }

  return { feed, paused, connected, togglePause };
}
