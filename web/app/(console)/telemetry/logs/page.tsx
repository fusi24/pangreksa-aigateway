import { LogExplorer } from "@/components/telemetry/LogExplorer";

/**
 * Log Explorer page — paginated structured log viewer with virtual scroll.
 */
export default function LogsPage() {
  return (
    <div>
      <h1 style={{ fontSize: "1.75rem", fontWeight: 600, marginBottom: "1.5rem" }}>
        Log Explorer
      </h1>
      <LogExplorer />
    </div>
  );
}
