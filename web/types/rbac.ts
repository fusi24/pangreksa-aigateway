/**
 * RBAC permission strings as a string literal union.
 * Sourced from SRS-FR-C-023.
 */
export type ConsolePermission =
  | "console.observability.read"
  | "console.monitor.read"
  | "console.telemetry.read"
  | "gateway.prompt_registry.read"
  | "gateway.prompt_registry.write"
  | "console.reports.read"
  | "console.reports.generate"
  | "console.admin.read"
  | "console.admin.write";

/** Maps each console route prefix to the permission required to access it. */
export const ROUTE_PERMISSION_MAP: Array<[string, ConsolePermission]> = [
  ["/dashboard", "console.observability.read"],
  ["/monitor", "console.monitor.read"],
  ["/telemetry", "console.telemetry.read"],
  ["/config", "gateway.prompt_registry.read"],
  ["/reports", "console.reports.read"],
  ["/settings", "console.admin.read"],
];
