import { test, expect } from "@playwright/test";

/**
 * E2E Flow: Dashboard page structure.
 * Verifies the Carbon Shell renders and navigation works.
 * Requires an authenticated session (cookie pre-set via fixtures in CI).
 */
test.describe("Dashboard (authenticated)", () => {
  test.beforeEach(async ({ context }) => {
    // In CI: inject a pre-signed test session cookie
    const testCookieValue = process.env["E2E_SESSION_COOKIE"] ?? "";
    if (testCookieValue) {
      await context.addCookies([{
        name: process.env["SESSION_COOKIE_NAME"] ?? "pangreksa_session",
        value: testCookieValue,
        domain: "localhost",
        path: "/",
        httpOnly: true,
        secure: false,
        sameSite: "Strict",
      }]);
    }
  });

  test("shell renders with navigation", async ({ page }) => {
    await page.goto("/dashboard");
    // If redirected to login (no test session), skip remaining assertions
    if (page.url().includes("/login")) {
      test.skip();
      return;
    }
    await expect(page.getByText("Pangreksa")).toBeVisible();
    await expect(page.getByText("Observability")).toBeVisible();
  });

  test("dashboard page has heading", async ({ page }) => {
    await page.goto("/dashboard");
    if (page.url().includes("/login")) { test.skip(); return; }
    await expect(page.getByRole("heading", { level: 1 })).toContainText(/dashboard/i);
  });
});
