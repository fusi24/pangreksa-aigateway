import { test, expect } from "@playwright/test";

/**
 * E2E Flow: Report builder wizard navigation.
 */
test("report builder has 4 steps", async ({ page }) => {
  await page.goto("/reports");
  if (page.url().includes("/login")) { test.skip(); return; }

  await expect(page.getByRole("heading", { level: 1 })).toContainText(/report/i);

  // Should have 4 progress steps
  const steps = page.getByRole("listitem");
  await expect(steps).toHaveCount(4);

  // Click Next to advance
  await page.getByRole("button", { name: /next/i }).click();
  // Step 2 should be visible
  await expect(page.getByText(/scope/i)).toBeVisible();
});
