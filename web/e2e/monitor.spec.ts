import { test, expect } from "@playwright/test";

/**
 * E2E Flow: Live monitoring feed — pause/resume.
 */
test("live feed page renders with pause control", async ({ page }) => {
  await page.goto("/monitor/live");
  if (page.url().includes("/login")) { test.skip(); return; }

  await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
  // Pause button should be present
  const pauseBtn = page.getByRole("button", { name: /pause/i });
  await expect(pauseBtn).toBeVisible();

  // Click pause — button should change to Resume
  await pauseBtn.click();
  await expect(page.getByRole("button", { name: /resume/i })).toBeVisible();
});
