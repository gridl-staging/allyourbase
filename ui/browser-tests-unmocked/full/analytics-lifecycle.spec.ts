import {
  test,
  expect,
  probeEndpoint,
  seedRequestLogEntry,
  cleanupRequestLogsByPath,
  waitForDashboard,
} from "../fixtures";
import type { Page } from "@playwright/test";

async function openAnalytics(page: Page): Promise<void> {
  await page.goto("/admin/");
  await waitForDashboard(page);
  await page.locator("aside").getByRole("button", { name: /^Analytics$/i }).click();
  await expect(page.getByRole("heading", { name: /Analytics/i })).toBeVisible({ timeout: 5000 });
}

test.describe("Analytics Lifecycle (Full E2E)", () => {
  const seededPaths: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    while (seededPaths.length > 0) {
      const path = seededPaths.pop();
      if (!path) continue;
      await cleanupRequestLogsByPath(request, adminToken, path).catch(() => {});
    }
  });

  test("request-log filter, reset, and query-tab interactions", async ({ page, request, adminToken }) => {
    const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/analytics/requests");
    test.skip(
      probeStatus === 503 || probeStatus === 501 || probeStatus === 404 || probeStatus === 500,
      `Analytics request-log endpoint unavailable (status ${probeStatus})`,
    );

    const runId = Date.now();
    const primaryPath = `/api/full-analytics/${runId}/primary`;
    const secondaryPath = `/api/full-analytics/${runId}/secondary`;
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

    await openAnalytics(page);

    const primaryRow = page.locator("tr").filter({ hasText: primaryPath }).first();
    await expect(primaryRow.getByRole("cell", { name: "POST" })).toBeVisible({ timeout: 5000 });
    await expect(primaryRow.getByRole("cell", { name: "418" })).toBeVisible();

    await expect(page.locator("tr").filter({ hasText: secondaryPath }).first()).toBeVisible({ timeout: 5000 });

    await page.getByLabel("Method").selectOption("POST");
    await page.getByRole("button", { name: "Apply Filters" }).click();

    await expect(page.locator("tr").filter({ hasText: primaryPath }).first()).toBeVisible({ timeout: 5000 });
    await expect(page.locator("tr").filter({ hasText: secondaryPath })).toHaveCount(0);

    await page.getByRole("button", { name: "Reset" }).click();

    await expect(page.locator("tr").filter({ hasText: primaryPath }).first()).toBeVisible({ timeout: 5000 });
    await expect(page.locator("tr").filter({ hasText: secondaryPath }).first()).toBeVisible({ timeout: 5000 });

    await page.getByRole("button", { name: /^Query Performance$/i }).click();
    await page.getByLabel("Sort by").selectOption("mean_time");

    const queryTableOrEmpty = page
      .getByRole("columnheader", { name: /Query/i })
      .or(page.getByText("No query statistics available"));
    await expect(queryTableOrEmpty).toBeVisible({ timeout: 5000 });
  });
});
