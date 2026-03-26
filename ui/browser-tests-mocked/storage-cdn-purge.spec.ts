import { test, expect, bootstrapMockedAdminApp, mockStorageCDNApis } from "./fixtures";

test.describe("Storage CDN Purge (Browser Mocked)", () => {
  test.beforeEach(async ({ page }) => {
    await bootstrapMockedAdminApp(page);
  });

  test("targeted URL purge: submits URLs and shows success toast", async ({ page }) => {
    const apis = await mockStorageCDNApis(page);

    await page.goto("/admin/");
    const storageButton = page.locator("aside").getByRole("button", { name: /^Storage$/i });
    await storageButton.click();
    await expect(page.getByRole("button", { name: "Upload", exact: true })).toBeVisible({ timeout: 5000 });
    await expect(page.getByText('No files in "default"')).toBeVisible({ timeout: 5000 });
    await expect(page.getByText(/^0 files$/)).toBeVisible();

    // Open CDN purge section
    await page.getByRole("button", { name: "CDN Purge" }).click();
    await expect(page.getByText("CDN Cache Purge")).toBeVisible();

    // Enter URLs and submit
    await page.getByPlaceholder("Enter URLs to purge, one URL per line").fill(
      "https://cdn.example.com/img1.png\nhttps://cdn.example.com/img2.png",
    );
    await page.getByRole("button", { name: "Purge URLs" }).click();

    // Assert success toast
    const toast = page.getByTestId("toast").filter({ hasText: "Purged 2 URLs via cloudflare" });
    await expect(toast).toBeVisible({ timeout: 5000 });
    await expect(toast).toHaveClass(/bg-green-50/);

    // Verify API was called with correct body
    await expect.poll(() => apis.purgeCalls, { timeout: 3000 }).toBe(1);
    expect(apis.lastPurgeBody).toEqual({
      urls: ["https://cdn.example.com/img1.png", "https://cdn.example.com/img2.png"],
    });
  });

  test("purge_all: confirmation step then success toast", async ({ page }) => {
    await mockStorageCDNApis(page);

    await page.goto("/admin/");
    const storageButton = page.locator("aside").getByRole("button", { name: /^Storage$/i });
    await storageButton.click();
    await expect(page.getByRole("button", { name: "Upload", exact: true })).toBeVisible({ timeout: 5000 });

    // Open CDN purge section
    await page.getByRole("button", { name: "CDN Purge" }).click();
    await expect(page.getByText("CDN Cache Purge")).toBeVisible();

    // Click Purge All — should show confirmation
    await page.getByRole("button", { name: "Purge All" }).click();
    await expect(page.getByText("Are you sure? This invalidates the entire CDN cache.")).toBeVisible();

    // Confirm
    await page.getByRole("button", { name: "Confirm" }).click();

    // Assert success toast
    const toast = page.getByTestId("toast").filter({ hasText: "Full cache purge submitted" });
    await expect(toast).toBeVisible({ timeout: 5000 });
    await expect(toast).toHaveClass(/bg-green-50/);
  });

  test("400 validation error: displays backend error message", async ({ page }) => {
    await mockStorageCDNApis(page, {
      purgeResponder: () => ({
        status: 400,
        body: { code: 400, message: "choose exactly one mode" },
      }),
    });

    await page.goto("/admin/");
    const storageButton = page.locator("aside").getByRole("button", { name: /^Storage$/i });
    await storageButton.click();
    await expect(page.getByRole("button", { name: "Upload", exact: true })).toBeVisible({ timeout: 5000 });

    // Open CDN purge section and submit URLs
    await page.getByRole("button", { name: "CDN Purge" }).click();
    await page.getByPlaceholder("Enter URLs to purge, one URL per line").fill("https://cdn.example.com/test.txt");
    await page.getByRole("button", { name: "Purge URLs" }).click();

    // Assert error toast with backend message
    const toast = page.getByTestId("toast").filter({ hasText: "choose exactly one mode" });
    await expect(toast).toBeVisible({ timeout: 5000 });
    await expect(toast).toHaveClass(/bg-red-50/);
  });

  test("429 rate-limit error: displays rate-limit message", async ({ page }) => {
    await mockStorageCDNApis(page, {
      purgeResponder: () => ({
        status: 429,
        body: { code: 429, message: "cdn purge_all rate limit exceeded" },
      }),
    });

    await page.goto("/admin/");
    const storageButton = page.locator("aside").getByRole("button", { name: /^Storage$/i });
    await storageButton.click();
    await expect(page.getByRole("button", { name: "Upload", exact: true })).toBeVisible({ timeout: 5000 });

    // Open CDN purge section and trigger purge all
    await page.getByRole("button", { name: "CDN Purge" }).click();
    await page.getByRole("button", { name: "Purge All" }).click();
    await expect(page.getByText("Are you sure?")).toBeVisible();
    await page.getByRole("button", { name: "Confirm" }).click();

    // Assert error toast with rate-limit message
    const toast = page.getByTestId("toast").filter({ hasText: "cdn purge_all rate limit exceeded" });
    await expect(toast).toBeVisible({ timeout: 5000 });
    await expect(toast).toHaveClass(/bg-red-50/);
  });
});
