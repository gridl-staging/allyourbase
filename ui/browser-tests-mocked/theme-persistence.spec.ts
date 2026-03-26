import { test, expect, bootstrapMockedAdminApp, mockThemePersistenceApis } from "./fixtures";

test.describe("Theme Persistence (Browser Mocked)", () => {
  test.beforeEach(async ({ page }) => {
    await bootstrapMockedAdminApp(page);
    await mockThemePersistenceApis(page);
  });

  test("theme toggle persists dark mode across page reload", async ({ page }) => {
    await page.goto("/admin/");
    const sidebar = page.locator("aside");

    // Verify dashboard loaded
    await expect(page.locator("aside")).toBeVisible({ timeout: 10000 });
    await expect(sidebar).toHaveCSS("background-color", "rgb(255, 255, 255)");

    // Default is light mode — toggle button should offer to switch to dark
    await expect(page.getByRole("button", { name: "Switch to dark mode" })).toBeVisible();

    // Click the theme toggle button to switch to dark mode
    await page.getByRole("button", { name: "Switch to dark mode" }).click();

    // Button label should have flipped to indicate we're now in dark mode
    await expect(page.getByRole("button", { name: "Switch to light mode" })).toBeVisible();
    await expect(sidebar).toHaveCSS("background-color", "rgb(17, 24, 39)");

    // Reload the page
    await page.reload();

    // Verify dashboard loaded again
    await expect(page.locator("aside")).toBeVisible({ timeout: 10000 });

    // Dark mode should persist — button should still offer to switch to light
    await expect(page.getByRole("button", { name: "Switch to light mode" })).toBeVisible();
    await expect(sidebar).toHaveCSS("background-color", "rgb(17, 24, 39)");
  });

  test("toggling back to light mode persists across page reload", async ({ page }) => {
    await page.goto("/admin/");
    const sidebar = page.locator("aside");
    await expect(page.locator("aside")).toBeVisible({ timeout: 10000 });
    await expect(sidebar).toHaveCSS("background-color", "rgb(255, 255, 255)");

    // Toggle to dark mode
    await page.getByRole("button", { name: "Switch to dark mode" }).click();
    await expect(page.getByRole("button", { name: "Switch to light mode" })).toBeVisible();
    await expect(sidebar).toHaveCSS("background-color", "rgb(17, 24, 39)");

    // Toggle back to light mode
    await page.getByRole("button", { name: "Switch to light mode" }).click();
    await expect(page.getByRole("button", { name: "Switch to dark mode" })).toBeVisible();
    await expect(sidebar).toHaveCSS("background-color", "rgb(255, 255, 255)");

    // Reload
    await page.reload();
    await expect(page.locator("aside")).toBeVisible({ timeout: 10000 });

    // Light mode should persist after reload
    await expect(page.getByRole("button", { name: "Switch to dark mode" })).toBeVisible();
    await expect(sidebar).toHaveCSS("background-color", "rgb(255, 255, 255)");
  });
});
