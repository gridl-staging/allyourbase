import { test, expect, probeEndpoint, waitForDashboard } from "../fixtures";

/**
 * SMOKE TEST: Materialized Views - List View
 *
 * Critical Path: Navigate to Matviews → verify heading, register action, and table headers
 */

test.describe("Smoke: Materialized Views", () => {
  test("matviews view loads with register action and table structure", async ({ page, request, adminToken }) => {
    const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/matviews");
    test.skip(
      probeStatus === 503 || probeStatus === 404 || probeStatus === 501,
      `Matviews service unavailable (status ${probeStatus})`,
    );

    await page.goto("/admin/");
    await waitForDashboard(page);

    await page.locator("aside").getByRole("button", { name: /^Matviews$/i }).click();
    await expect(page.getByRole("heading", { name: /Materialized Views/i })).toBeVisible({ timeout: 15_000 });

    // Register action should always be available
    await expect(
      page.getByRole("button", { name: /Register Matview/i }),
    ).toBeVisible();

    // Depending on server state, this view can render either a table or an empty-state card.
    const tableNameHeader = page.getByRole("columnheader", { name: /Name/i });
    const emptyState = page.getByText("No materialized views registered");
    await expect(tableNameHeader.or(emptyState)).toBeVisible({ timeout: 5000 });

    if (await tableNameHeader.isVisible().catch(() => false)) {
      await expect(page.getByRole("columnheader", { name: /Refresh/i })).toBeVisible();
    }
  });
});
