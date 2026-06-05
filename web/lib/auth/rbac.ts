import "server-only";

import type { ConsolePermission } from "@/types/rbac";
import type { SessionPayload } from "./session";

/**
 * Returns true if the session has the requested permission.
 *
 * @param session - Parsed session payload from JWT
 * @param permission - Permission string to check
 */
export function hasPermission(
  session: SessionPayload,
  permission: ConsolePermission
): boolean {
  return session.permissions.includes(permission);
}

/**
 * Throws if the session is missing or lacks the required permission.
 * Use in RSC pages to guard server-side data fetching.
 *
 * @param session - Parsed session payload (or null if unauthenticated)
 * @param permission - Permission string required
 * @throws {Error} If session is null or permission is absent
 */
export function requirePermission(
  session: SessionPayload | null,
  permission: ConsolePermission
): asserts session is SessionPayload {
  if (!session) {
    throw new Error("Unauthenticated");
  }
  if (!hasPermission(session, permission)) {
    throw new Error(`Forbidden: requires ${permission}`);
  }
}
