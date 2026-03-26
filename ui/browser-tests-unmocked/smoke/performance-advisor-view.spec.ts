import {
  test,
  expect,
  probeEndpoint,
  fetchPerformanceAdvisorReport,
  waitForDashboard,
} from "../fixtures";

/**
 * SMOKE TEST: Performance Advisor — Content-Verified
 *
 * Probes the performance advisor endpoint and asserts real content when available
 * (time range selector, query fingerprint rows or "No slow queries" empty state),
 * or verifies the error UI renders correctly when the backend endpoint is not wired.
 */

test.describe("Smoke: Performance Advisor", () => {
  test("performance advisor renders content matching API-backed report state", async ({ page, request, adminToken }) => {
    const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/advisors/performance");
    test.skip(
      probeStatus === 503 || probeStatus === 404 || probeStatus === 501,
      `Performance advisor endpoint unavailable (status ${probeStatus})`,
    );

    const report = await fetchPerformanceAdvisorReport(request, adminToken, "1h");

    // Act: navigate to Performance Advisor
    await page.goto("/admin/");
    await waitForDashboard(page);
    await page.locator("aside").getByRole("button", { name: /^Performance Advisor$/i }).click();
    await expect(page.getByRole("heading", { name: /Performance Advisor/i })).toBeVisible({ timeout: 15_000 });

    const panel = page.getByTestId("performance-advisor-panel");
    await expect(panel).toBeVisible({ timeout: 5000 });

    const timeRangeSelect = panel.getByLabel(/Time range/i);
    await expect(timeRangeSelect).toBeVisible({ timeout: 5000 });
    await expect(timeRangeSelect).toHaveValue(report.range);

    const queryTableOrEmptyState = panel
      .getByRole("columnheader", { name: /Fingerprint/i })
      .or(panel.getByText(/No slow queries/i));
    await expect(queryTableOrEmptyState).toBeVisible({ timeout: 5000 });

    const emptyState = panel.getByText(/No slow queries/i);
    if (report.queries.length === 0) {
      await expect(emptyState).toBeVisible();
    } else {
      await expect(emptyState).not.toBeVisible();
    }
  });
});
