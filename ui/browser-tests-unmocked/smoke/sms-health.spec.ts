import { test, expect } from "../fixtures";

/**
 * SMOKE TEST: SMS Health - Stats Dashboard
 *
 * Critical Path: Navigate to SMS Health → Verify stat cards load
 */

test.describe("Smoke: SMS Health", () => {
  test("admin can navigate to SMS Health page and see stat cards", async ({ page }) => {
    // Act: navigate to admin dashboard
    await page.goto("/admin/");
    await expect(page.getByText("Allyourbase").first()).toBeVisible();

    // Act: click SMS Health in sidebar
    await page.locator("aside").getByRole("button", { name: /SMS Health/i }).click();

    // Assert: heading visible (page-body content, not sidebar label)
    await expect(page.getByRole("heading", { name: /SMS Health/i })).toBeVisible({ timeout: 5000 });

    // Assert: stat card labels visible
    await expect(page.getByText("Today")).toBeVisible();
    await expect(page.getByText("Last 7 Days")).toBeVisible();
    await expect(page.getByText("Last 30 Days")).toBeVisible();

    // Assert: stat row labels visible
    await expect(page.getByText("Sent").first()).toBeVisible();
    await expect(page.getByText("Conversion Rate").first()).toBeVisible();
  });
});
