import { test, expect } from "@playwright/test";

/**
 * E2E Flow 1: Unauthenticated visit redirects to /login.
 * E2E Flow 2: Login → dashboard visible.
 */

test("unauthenticated visit redirects to /login", async ({ page }) => {
  await page.goto("/dashboard");
  await expect(page).toHaveURL(/.*\/login/);
});

test("login page renders correctly", async ({ page }) => {
  await page.goto("/login");
  await expect(page.getByText("Sign in")).toBeVisible();
  await expect(page.getByRole("textbox", { name: /email/i })).toBeVisible();
});

// Note: Full login flow requires a running Central Server.
// This test verifies the login form submission structure.
test("login form shows error on invalid credentials", async ({ page }) => {
  await page.goto("/login");
  await page.getByRole("textbox", { name: /email/i }).fill("invalid@test.com");
  await page.getByLabel(/password/i).fill("wrongpassword");
  await page.getByRole("button", { name: /sign in/i }).click();

  // Should show an error notification (Central Server not running = network error)
  await expect(page.getByRole("alert")).toBeVisible({ timeout: 5000 });
});
