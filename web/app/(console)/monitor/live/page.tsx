import { LiveFeed } from "@/components/monitor/LiveFeed";

/**
 * Live telemetry feed page — SSE-driven real-time transaction events.
 */
export default function LivePage() {
  return (
    <div>
      <h1 style={{ fontSize: "1.75rem", fontWeight: 600, marginBottom: "1.5rem" }}>
        Live Telemetry Feed
      </h1>
      <LiveFeed />
    </div>
  );
}
