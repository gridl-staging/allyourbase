import { test, expect, waitForDashboard } from "../fixtures";

/**
 * SMOKE TEST: New Table CTA navigation
 *
 * Critical Path: Admin clicks New Table in sidebar → SQL Editor opens
 */

test.describe("Smoke: Create Table Nav Update", () => {
  test("clicking New Table opens SQL Editor", async ({ page }) => {
    // Navigate to admin dashboard
    await page.goto("/admin/");
    await waitForDashboard(page);

    const sidebar = page.locator("aside");

    // Navigate to SQL Editor via the first-run CTA in Tables section
    await sidebar.getByRole("button", { name: /^New Table$/i }).click();

    const sqlInput = page.getByLabel("SQL query");
    await expect(sqlInput).toBeVisible({ timeout: 5000 });
  });
});
