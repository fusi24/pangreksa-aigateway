import { describe, it, expect } from "vitest";
// rbac.ts uses 'server-only' — we need to mock it in tests
import { vi } from "vitest";

vi.mock("server-only", () => ({}));

const { hasPermission, requirePermission } = await import("@/lib/auth/rbac");

const baseSession = {
  sub: "user-123",
  email: "admin@test.com",
  org_id: "org-456",
  central_token: "jwt-token",
  permissions: ["console.observability.read", "console.monitor.read"] as import("@/types/rbac").ConsolePermission[],
};

describe("hasPermission", () => {
  it("returns true when permission is present", () => {
    expect(hasPermission(baseSession, "console.observability.read")).toBe(true);
  });

  it("returns false when permission is absent", () => {
    expect(hasPermission(baseSession, "console.admin.write")).toBe(false);
  });
});

describe("requirePermission", () => {
  it("does not throw when permission is present", () => {
    expect(() => requirePermission(baseSession, "console.monitor.read")).not.toThrow();
  });

  it("throws when session is null", () => {
    expect(() => requirePermission(null, "console.observability.read")).toThrow("Unauthenticated");
  });

  it("throws when permission is absent", () => {
    expect(() => requirePermission(baseSession, "console.admin.write")).toThrow("Forbidden");
  });
});
