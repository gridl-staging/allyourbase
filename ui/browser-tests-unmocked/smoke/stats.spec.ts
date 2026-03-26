import {
  test,
  expect,
  probeEndpoint,
  fetchAdminStatsSnapshot,
  waitForDashboard,
} from "../fixtures";

function formatExpectedUptime(seconds: number): string {
  const days = Math.floor(seconds / 86400);
  const hours = Math.floor((seconds % 86400) / 3600);
  const mins = Math.floor((seconds % 3600) / 60);
  if (days > 0) return `${days}d ${hours}h ${mins}m`;
  if (hours > 0) return `${hours}h ${mins}m`;
  return `${mins}m`;
}

function formatExpectedBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

/**
 * SMOKE TEST: Stats Overview
 *
 * Critical Path: Navigate to Stats → Verify stat cards render with real values
 * (Uptime, Go Version, Goroutines, Memory) in the page body.
 */

test.describe("Smoke: Stats", () => {
  test("stats page renders system metrics and memory stats", async ({ page, request, adminToken }) => {
    const status = await probeEndpoint(request, adminToken, "/api/admin/stats/");
    test.skip(
      status === 501 || status === 404,
      `Stats endpoint not available (status ${status})`,
    );

    const snapshot = await fetchAdminStatsSnapshot(request, adminToken);

    const expectedUptime = formatExpectedUptime(snapshot.uptime_seconds);
    const expectedAlloc = formatExpectedBytes(snapshot.memory_alloc);
    const expectedSys = formatExpectedBytes(snapshot.memory_sys);

    await page.route("**/api/admin/stats*", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(snapshot),
      });
    });

    await page.goto("/admin/");
    await waitForDashboard(page);

    await page.locator("aside").getByRole("button", { name: /Stats/i }).click();
    await expect(page.getByRole("heading", { name: /Stats/i })).toBeVisible({ timeout: 15_000 });

    await expect(page.getByTestId("stats-card-uptime")).toContainText(expectedUptime, {
      timeout: 5000,
    });
    await expect(page.getByTestId("stats-card-go-version")).toContainText(snapshot.go_version);
    await expect(page.getByTestId("stats-card-goroutines")).toContainText(String(snapshot.goroutines));
    await expect(page.getByTestId("stats-card-gc-cycles")).toContainText(String(snapshot.gc_cycles));
    await expect(page.getByTestId("stats-card-alloc")).toContainText(expectedAlloc);
    await expect(page.getByTestId("stats-card-sys")).toContainText(expectedSys);

    if (snapshot.db_pool_max !== undefined) {
      await expect(page.getByTestId("stats-card-total")).toContainText(String(snapshot.db_pool_total ?? 0));
      await expect(page.getByTestId("stats-card-idle")).toContainText(String(snapshot.db_pool_idle ?? 0));
      await expect(page.getByTestId("stats-card-in-use")).toContainText(String(snapshot.db_pool_in_use ?? 0));
      await expect(page.getByTestId("stats-card-max")).toContainText(String(snapshot.db_pool_max));
    }
  });
});
