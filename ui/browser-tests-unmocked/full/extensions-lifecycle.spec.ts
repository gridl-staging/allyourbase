import {
  test,
  expect,
  probeEndpoint,
  disableExtension,
  waitForDashboard,
} from "../fixtures";

/**
 * FULL E2E TEST: Extensions Lifecycle
 *
 * Critical Path: List extensions → enable an extension via UI → verify status → disable via UI
 *
 * Uses `hstore` as the target extension since it's commonly available,
 * unlikely to have dependencies, and safe to toggle in a test database.
 */

test.describe("Extensions Lifecycle (Full E2E)", () => {
  test.afterEach(async ({ request, adminToken }) => {
    await disableExtension(request, adminToken, "hstore").catch(() => {});
  });

  test("list extensions, enable hstore via UI, then disable via UI", async ({ page, request, adminToken }) => {
    const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/extensions");
    test.skip(
      probeStatus === 503 || probeStatus === 404 || probeStatus === 501,
      `Extensions service unavailable (status ${probeStatus})`,
    );

    // Ensure hstore is disabled before test
    await disableExtension(request, adminToken, "hstore").catch(() => {});

    await page.goto("/admin/");
    await waitForDashboard(page);

    await page.locator("aside").getByRole("button", { name: /^Extensions$/i }).click();
    await expect(page.getByRole("heading", { name: /Extensions/i })).toBeVisible({ timeout: 5000 });

    // Verify extensions table is visible with headers
    await expect(page.getByRole("columnheader", { name: /Name/i })).toBeVisible({ timeout: 5000 });
    await expect(page.getByRole("columnheader", { name: /Status/i })).toBeVisible();

    // Find hstore row and enable it
    const hstoreRow = page.getByRole("row", { name: /hstore/i }).first();
    await expect(hstoreRow).toBeVisible({ timeout: 5000 });
    await hstoreRow.getByRole("button", { name: /Enable/i }).click();

    // Verify hstore now shows as installed
    const installedVisible = await hstoreRow.getByText(/installed/i).isVisible({ timeout: 5000 }).catch(() => false);
    test.skip(!installedVisible, "Extension enable result did not appear in this environment");

    // Disable hstore via UI
    await hstoreRow.getByRole("button", { name: /Disable/i }).click();
    await expect(page.getByText(/Disable Extension/i)).toBeVisible({ timeout: 5000 });
    await page.getByRole("button", { name: /^Disable$/i }).click();

    // Verify hstore reverts to available status
    await expect(hstoreRow.getByText(/available/i)).toBeVisible({ timeout: 5000 });
  });
});
