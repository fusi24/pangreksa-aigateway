/**
 * SSE client factory with exponential backoff reconnection.
 * Implements SRS-NFR-C-020: auto-reconnect 2s → 4s → 8s … max 60s.
 */

const SSE_EVENT_TYPES = ["transaction", "pod_heartbeat", "budget_alert"] as const;
type SseEventType = (typeof SSE_EVENT_TYPES)[number];

export interface SseClientOptions {
  url: string;
  onEvent: (type: SseEventType, data: unknown) => void;
  onError?: (error: Event) => void;
  onOpen?: () => void;
}

/**
 * Creates a managed SSE client that auto-reconnects with exponential backoff.
 *
 * @param opts - Configuration including URL and event handlers
 * @returns Cleanup function — call to permanently stop the connection
 */
export function createSseClient(opts: SseClientOptions): () => void {
  let es: EventSource | null = null;
  let retryMs = 2000;
  let stopped = false;
  let retryTimer: ReturnType<typeof setTimeout> | null = null;

  function connect(): void {
    if (stopped) return;

    es = new EventSource(opts.url, { withCredentials: true });

    es.onopen = () => {
      retryMs = 2000;
      opts.onOpen?.();
    };

    for (const eventType of SSE_EVENT_TYPES) {
      es.addEventListener(eventType, (e: MessageEvent) => {
        try {
          const data: unknown = JSON.parse(e.data as string);
          opts.onEvent(eventType, data);
        } catch {
          // Malformed event — skip silently
        }
      });
    }

    es.onerror = (e) => {
      opts.onError?.(e);
      es?.close();
      if (!stopped) {
        retryTimer = setTimeout(() => {
          retryMs = Math.min(retryMs * 2, 60_000);
          connect();
        }, retryMs);
      }
    };
  }

  connect();

  return () => {
    stopped = true;
    if (retryTimer !== null) clearTimeout(retryTimer);
    es?.close();
  };
}
