/**
 * Carbon Design System token values for JointJS cell styling.
 * These are applied as cell attributes to keep the topology graph
 * visually coherent with the Carbon theme.
 */

import type { PodStatus } from "@/types/api";

/** Base shape attributes for a pod node (Carbon White theme). */
export const POD_SHAPE_BASE = {
  body: {
    rx: 8,
    ry: 8,
    fill: "#f4f4f4",
    stroke: "#8d8d8d",
    strokeWidth: 2,
  },
  label: {
    fontSize: 11,
    fontFamily: "IBM Plex Mono, monospace",
    fill: "#161616",
  },
} as const;

/** Status indicator colors matching Carbon support tokens. */
export const STATUS_COLORS: Record<PodStatus, string> = {
  healthy: "#198038",
  degraded: "#ff832b",
  dead: "#da1e28",
};

/** Central Server node styling (slightly distinct from pod nodes). */
export const CENTRAL_SERVER_SHAPE = {
  body: {
    rx: 4,
    ry: 4,
    fill: "#e8f1ff",
    stroke: "#0f62fe",
    strokeWidth: 3,
  },
  label: {
    fontSize: 12,
    fontFamily: "IBM Plex Sans, sans-serif",
    fill: "#001d6c",
    fontWeight: "bold",
  },
} as const;

/** Infra node styling (Redis, PostgreSQL). */
export const INFRA_SHAPE = {
  body: {
    rx: 4,
    ry: 4,
    fill: "#f2f4f8",
    stroke: "#4d5358",
    strokeWidth: 1,
  },
  label: {
    fontSize: 10,
    fontFamily: "IBM Plex Sans, sans-serif",
    fill: "#4d5358",
  },
} as const;

/** Edge/link styling between nodes. */
export const LINK_STYLE = {
  line: {
    stroke: "#8d8d8d",
    strokeWidth: 1.5,
    targetMarker: { type: "arrow", size: 8 },
  },
} as const;
