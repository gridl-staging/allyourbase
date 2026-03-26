import { test, expect, waitForDashboard } from "../fixtures";

/**
 * FULL TEST: Dark Mode Persistence
 *
 * Verifies dark mode persists through real dashboard navigation:
 * 1. Toggle to dark mode
 * 2. Navigate between pages
 * 3. Verify dark mode stays active
 * 4. Reload and verify persistence
 */

test.describe("Full: Dark Mode Persistence", () => {
  test("dark mode persists through dashboard navigation and page reload", async ({ page }) => {
    // Act: Navigate to dashboard
    await page.goto("/admin/");
    const sidebar = page.locator("aside");
    await waitForDashboard(page);
    await expect(sidebar).toHaveCSS("background-color", "rgb(255, 255, 255)");

    // Step 1: Start in light mode — toggle button should say "Switch to dark mode"
    const darkToggle = page.getByRole("button", { name: "Switch to dark mode" });
    await expect(darkToggle).toBeVisible({ timeout: 5000 });

    // Step 2: Click to switch to dark mode
    await darkToggle.click();

    // Step 3: Verify toggle now says "Switch to light mode" (confirming dark mode active)
    const lightToggle = page.getByRole("button", { name: "Switch to light mode" });
    await expect(lightToggle).toBeVisible();
    await expect(sidebar).toHaveCSS("background-color", "rgb(17, 24, 39)");

    // Step 4: Navigate to SQL Editor via sidebar
    await sidebar.getByRole("button", { name: /^SQL Editor$/i }).click();
    const sqlInput = page.getByLabel("SQL query");
    await expect(sqlInput).toBeVisible({ timeout: 5000 });

    // Step 5: Verify still in dark mode after navigation
    await expect(lightToggle).toBeVisible();
    await expect(sidebar).toHaveCSS("background-color", "rgb(17, 24, 39)");

    // Step 6: Navigate to a different section — Storage
    await sidebar.getByRole("button", { name: /^Storage$/i }).click();
    await expect(page.getByRole("heading", { name: /Storage/i }).or(page.getByText(/bucket/i).first())).toBeVisible({ timeout: 5000 });

    // Step 7: Still dark mode after second navigation
    await expect(lightToggle).toBeVisible();
    await expect(sidebar).toHaveCSS("background-color", "rgb(17, 24, 39)");

    // Step 8: Reload the page completely
    await page.reload();
    await waitForDashboard(page);

    // Step 9: Verify dark mode persisted through reload
    await expect(page.getByRole("button", { name: "Switch to light mode" })).toBeVisible({ timeout: 5000 });
    await expect(sidebar).toHaveCSS("background-color", "rgb(17, 24, 39)");

    // Cleanup: toggle back to light mode so other tests are unaffected
    await page.getByRole("button", { name: "Switch to light mode" }).click();
    await expect(page.getByRole("button", { name: "Switch to dark mode" })).toBeVisible();
    await expect(sidebar).toHaveCSS("background-color", "rgb(255, 255, 255)");
  });
});
