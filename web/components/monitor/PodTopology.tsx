"use client";

import { useEffect, useRef, useCallback } from "react";
import type { PodRecord, PodStatus } from "@/types/api";
import { STATUS_COLORS, POD_SHAPE_BASE, CENTRAL_SERVER_SHAPE, LINK_STYLE } from "@/lib/charts/jointjs-styles";

interface PodTopologyProps {
  pods: PodRecord[];
  onPodClick: (gatewayId: string) => void;
}

/**
 * Interactive JointJS pod topology graph.
 * JointJS is dynamically imported inside useEffect to avoid SSR issues.
 * Pod status colors update in-place via cell.attr() without full re-render.
 */
export function PodTopology({ pods, onPodClick }: PodTopologyProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  // Store refs so we can update cells without re-mounting
  const graphRef = useRef<unknown>(null);
  const cellMapRef = useRef<Map<string, unknown>>(new Map());
  const onPodClickRef = useRef(onPodClick);
  onPodClickRef.current = onPodClick;

  // Build or rebuild the graph when pods change
  useEffect(() => {
    if (!containerRef.current || typeof window === "undefined") return;

    let paper: { remove: () => void; setDimensions: (w: number, h: number) => void; on: (e: string, cb: (...args: unknown[]) => void) => void } | undefined;
    let ro: ResizeObserver | undefined;
    let destroyed = false;

    import("@joint/core").then(({ dia, shapes }) => {
      if (destroyed || !containerRef.current) return;

      const graph = new dia.Graph({}, { cellNamespace: shapes });
      graphRef.current = graph;

      const container = containerRef.current;
      paper = new dia.Paper({
        el: container,
        model: graph,
        width: container.clientWidth || 800,
        height: 500,
        gridSize: 10,
        interactive: true,
        background: { color: "transparent" },
      }) as typeof paper;

      // Build cells
      const cellMap = new Map<string, unknown>();
      cellMapRef.current = cellMap;

      // Central Server node
      const centralNode = new shapes.standard.Rectangle({
        position: { x: (container.clientWidth || 800) / 2 - 80, y: 20 },
        size: { width: 160, height: 50 },
        attrs: {
          body: CENTRAL_SERVER_SHAPE.body,
          label: { ...CENTRAL_SERVER_SHAPE.label, text: "Central Server" },
        },
      });
      graph.addCell(centralNode);

      // Pod nodes arranged in a row
      const podSpacing = Math.min(200, ((container.clientWidth || 800) - 40) / Math.max(pods.length, 1));
      pods.forEach((pod, i) => {
        const x = 20 + i * podSpacing;
        const y = 160;
        const status = pod.status;

        const podNode = new shapes.standard.Rectangle({
          id: pod.gateway_id,
          position: { x, y },
          size: { width: 160, height: 60 },
          attrs: {
            body: {
              ...POD_SHAPE_BASE.body,
              stroke: STATUS_COLORS[status as PodStatus] ?? STATUS_COLORS.dead,
            },
            label: {
              ...POD_SHAPE_BASE.label,
              text: `${pod.gateway_id.slice(0, 14)}\n${status.toUpperCase()}`,
            },
          },
        });
        (podNode as { set: (key: string, value: string) => void }).set("gateway_id", pod.gateway_id);
        graph.addCell(podNode);
        cellMap.set(pod.gateway_id, podNode);

        // Link from central server to pod
        const link = new shapes.standard.Link({
          source: { id: centralNode.id },
          target: { id: podNode.id },
          attrs: { line: LINK_STYLE.line },
        });
        graph.addCell(link);
      });

      // Click handler — paper is defined at this point
      const p = paper;
      if (!p) return;
      p.on("cell:pointerclick", (...args: unknown[]) => {
        const cellView = args[0] as { model: { get: (key: string) => unknown } };
        const gatewayId = cellView.model.get("gateway_id") as string | undefined;
        if (gatewayId) onPodClickRef.current(gatewayId);
      });

      // Responsive resize
      ro = new ResizeObserver(() => {
        if (container && p) {
          p.setDimensions(container.clientWidth, 500);
        }
      });
      ro.observe(container);
    });

    return () => {
      destroyed = true;
      ro?.disconnect();
      paper?.remove();
    };
  }, [pods]);

  // Update pod status colors in-place from SSE events (no full re-mount)
  const updatePodStatus = useCallback((gatewayId: string, status: PodStatus) => {
    const cell = cellMapRef.current.get(gatewayId) as { attr: (path: string, value: unknown) => void } | undefined;
    if (cell) {
      cell.attr("body/stroke", STATUS_COLORS[status]);
    }
  }, []);

  // Expose for parent to call from SSE events
  (containerRef as unknown as { updatePodStatus: typeof updatePodStatus }).updatePodStatus = updatePodStatus;

  return (
    <div
      ref={containerRef}
      style={{ width: "100%", minHeight: 500, cursor: "grab" }}
      role="img"
      aria-label={`Pod topology graph showing ${pods.length} daemon pod${pods.length !== 1 ? "s" : ""} connected to the Central Server`}
    />
  );
}
