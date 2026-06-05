"use client";

import { useState } from "react";
import { Tile, InlineNotification } from "@carbon/react";
import { PodTopology } from "@/components/monitor/PodTopology";
import { PodDetailDrawer } from "@/components/monitor/PodDetailDrawer";
import { usePods } from "@/hooks/usePods";
import type { PodRecord } from "@/types/api";

/**
 * Pod Topology page — interactive JointJS graph of all registered daemon pods.
 */
export default function TopologyPage() {
  const { data, isLoading, error } = usePods();
  const [selectedPod, setSelectedPod] = useState<PodRecord | null>(null);

  const pods = data?.pods ?? [];

  return (
    <div>
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: "1rem" }}>
        <h1 style={{ fontSize: "1.75rem", fontWeight: 600 }}>Pod Topology</h1>
        {data && (
          <span style={{ color: "#525252", fontSize: "0.875rem" }}>
            {data.total} pods — {data.healthy} healthy · {data.degraded} degraded · {data.dead} dead
          </span>
        )}
      </div>

      {error && (
        <InlineNotification
          kind="error"
          title="Failed to load pods"
          subtitle={(error as Error).message}
          style={{ marginBottom: "1rem" }}
        />
      )}

      {isLoading ? (
        <Tile style={{ height: 500, display: "flex", alignItems: "center", justifyContent: "center" }}>
          <p style={{ color: "#525252" }}>Loading topology…</p>
        </Tile>
      ) : pods.length === 0 ? (
        <Tile style={{ height: 200, display: "flex", alignItems: "center", justifyContent: "center" }}>
          <p style={{ color: "#525252" }}>No daemon pods registered.</p>
        </Tile>
      ) : (
        <Tile style={{ padding: 0, overflow: "hidden" }}>
          <PodTopology
            pods={pods}
            onPodClick={(gatewayId) => {
              const pod = pods.find((p) => p.gateway_id === gatewayId) ?? null;
              setSelectedPod(pod);
            }}
          />
        </Tile>
      )}

      <PodDetailDrawer
        pod={selectedPod}
        onClose={() => setSelectedPod(null)}
      />
    </div>
  );
}
