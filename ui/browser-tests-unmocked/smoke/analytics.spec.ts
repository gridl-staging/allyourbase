import {
  test,
  expect,
  probeEndpoint,
  seedRequestLogEntry,
  cleanupRequestLogsByPath,
  waitForDashboard,
} from "../fixtures";

/**
 * SMOKE TEST: Analytics
 *
 * Critical Path: Navigate to Analytics → Verify page loads with tab structure
 * and content-area elements (subtitle, table headers) visible in the page body.
 */

test.describe("Smoke: Analytics", () => {
  const seededPaths: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    while (seededPaths.length > 0) {
      const path = seededPaths.pop();
      if (!path) continue;
      await cleanupRequestLogsByPath(request, adminToken, path).catch(() => {});
    }
  });

  test("analytics page renders seeded request-log row and filter behavior", async ({ page, request, adminToken }) => {
    const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/analytics/requests");
    test.skip(
      probeStatus === 503 || probeStatus === 501 || probeStatus === 404 || probeStatus === 500,
      `Analytics request-log endpoint unavailable (status ${probeStatus})`,
    );

    const runId = Date.now();
    const primaryPath = `/api/smoke-analytics/${runId}/primary`;
    const secondaryPath = `/api/smoke-analytics/${runId}/secondary`;
    const seedTimestamp = new Date(Date.now() + 5000).toISOString();

    await seedRequestLogEntry(request, adminToken, {
      method: "POST",
      path: primaryPath,
      statusCode: 418,
      durationMs: 321,
      timestampISO: seedTimestamp,
    });
    seededPaths.push(primaryPath);

    await seedRequestLogEntry(request, adminToken, {
      method: "GET",
      path: secondaryPath,
      statusCode: 200,
      durationMs: 45,
      timestampISO: seedTimestamp,
    });
    seededPaths.push(secondaryPath);

    await page.goto("/admin/");
    await waitForDashboard(page);

    await page.locator("aside").getByRole("button", { name: /Analytics/i }).click();
    await expect(page.getByRole("heading", { name: /Analytics/i })).toBeVisible({ timeout: 15_000 });

    await expect(page.getByText("Request logs and query performance insights")).toBeVisible();
    await expect(page.getByRole("button", { name: /Request Logs/i })).toBeVisible();
    await expect(page.getByRole("button", { name: /Query Performance/i })).toBeVisible();

    await expect(page.getByRole("columnheader", { name: /Method/i })).toBeVisible({ timeout: 5000 });
    await expect(page.getByRole("columnheader", { name: /Path/i })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: /Status/i })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: /Duration/i })).toBeVisible();

    const primaryRow = page.locator("tr").filter({ hasText: primaryPath }).first();
    await expect(primaryRow.getByRole("cell", { name: "POST" })).toBeVisible({ timeout: 5000 });
    await expect(primaryRow.getByRole("cell", { name: "418" })).toBeVisible();
    await expect(primaryRow.getByRole("cell", { name: "321ms" })).toBeVisible();

    await page.getByLabel("Method").selectOption("POST");
    await page.getByLabel("Path").fill(primaryPath);
    await page.getByLabel("Status Code").fill("418");
    await page.getByRole("button", { name: /Apply Filters/i }).click();

    await expect(page.locator("tr").filter({ hasText: primaryPath }).first()).toBeVisible({ timeout: 5000 });
    await expect(page.locator("tr").filter({ hasText: secondaryPath })).toHaveCount(0);

    await page.getByRole("button", { name: /Query Performance/i }).click();
    const queryTableOrEmpty = page
      .getByRole("columnheader", { name: /Query/i })
      .or(page.getByText("No query statistics available"));
    await expect(queryTableOrEmpty).toBeVisible({ timeout: 5000 });
  });
});
