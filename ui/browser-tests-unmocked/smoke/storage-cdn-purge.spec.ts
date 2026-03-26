import { test, expect, waitForDashboard } from "../fixtures";

/**
 * SMOKE TEST: Storage CDN Purge
 *
 * Critical Path: Navigate to Storage → Open CDN Purge → Submit targeted purge
 *   → Wait for /api/admin/storage/cdn/purge response
 *   → Assert success (202) or error (503/404) feedback is visible.
 */

test.describe("Smoke: Storage CDN Purge", () => {
  test("targeted URL purge produces correct feedback based on backend response", async ({ page }) => {
    // Navigate to admin dashboard
    await page.goto("/admin/");
    await waitForDashboard(page);

    // Navigate to Storage section
    const storageButton = page.locator("aside").getByRole("button", { name: /^Storage$/i });
    await storageButton.click();
    await expect(page.getByRole("button", { name: "Upload", exact: true })).toBeVisible({ timeout: 5000 });

    // Open CDN purge section
    await page.getByRole("button", { name: "CDN Purge" }).click();
    await expect(page.getByText("CDN Cache Purge")).toBeVisible();

    // Enter a test URL and set up response interception before clicking submit
    await page.getByPlaceholder("Enter URLs to purge, one URL per line").fill("https://example.com/test.txt");

    const responsePromise = page.waitForResponse(
      (resp) => resp.url().includes("/api/admin/storage/cdn/purge") && resp.request().method() === "POST",
      { timeout: 15000 },
    );

    await page.getByRole("button", { name: "Purge URLs" }).click();

    // Wait for the actual API response
    const response = await responsePromise;
    const status = response.status();

    if (status === 202) {
      // Success path: assert the success toast contains purge-specific text
      const successToast = page.getByTestId("toast").filter({ hasText: /Purged \d+ URLs? via/ });
      await expect(successToast).toBeVisible({ timeout: 5000 });
      await expect(successToast).toHaveClass(/bg-green-50/);
    } else {
      // Error path (503 storage not enabled, 404 route not registered, etc.):
      // assert an error toast appears with a non-empty message
      const errorToast = page.getByTestId("toast").filter({ hasText: /.+/ }).first();
      await expect(errorToast).toBeVisible({ timeout: 5000 });
      await expect(errorToast).toHaveClass(/bg-red-50/);
    }
  });
});
