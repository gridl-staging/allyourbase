/* eslint-disable playwright/no-skipped-test -- unmocked advisor endpoints and dataset shape vary by environment; skips keep tests deterministic without conditional expects */
import {
  test,
  expect,
  probeEndpoint,
  fetchSecurityAdvisorReport,
  fetchPerformanceAdvisorReport,
  waitForDashboard,
} from "../fixtures";
import type { Page } from "@playwright/test";

function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

async function openAdvisor(page: Page, name: RegExp): Promise<void> {
  await page.goto("/admin/");
  await waitForDashboard(page);
  await page.locator("aside").getByRole("button", { name }).click();
}

test.describe("Advisors Lifecycle (Full E2E)", () => {
  test("security advisor filters and finding expansion", async ({ page, request, adminToken }) => {
    const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/advisors/security");
    test.skip(
      probeStatus === 503 || probeStatus === 404 || probeStatus === 501,
      `Security advisor endpoint unavailable (status ${probeStatus})`,
    );

    const report = await fetchSecurityAdvisorReport(request, adminToken);
    const finding = report.findings[0];
    test.skip(!finding, "Security advisor returned no findings; expansion path unavailable");

    await openAdvisor(page, /^Security Advisor$/i);
    await expect(page.getByRole("heading", { name: /Security Advisor/i })).toBeVisible({ timeout: 5000 });

    const panel = page.getByTestId("security-advisor-panel");
    await expect(panel).toBeVisible({ timeout: 5000 });
    await expect(panel.getByLabel("Severity")).toBeVisible();
    await expect(panel.getByLabel("Category")).toBeVisible();
    await expect(panel.getByLabel("Status")).toBeVisible();

    await panel.getByLabel("Severity").selectOption(finding.severity);
    await expect.poll(() => page.url()).toContain(`secSeverity=${finding.severity}`);

    // Verify the filtered result count matches the fixture-predicted count.
    // Each finding renders as a <li> inside the severity section.
    const expectedFilteredCount = report.findings.filter((f) => f.severity === finding.severity).length;
    await expect(panel.getByRole("listitem")).toHaveCount(expectedFilteredCount, { timeout: 5000 });

    const findingButton = panel.getByRole("button", { name: new RegExp(escapeRegExp(finding.title), "i") }).first();
    await expect(findingButton).toBeVisible({ timeout: 5000 });
    await findingButton.click();
    await expect(panel.getByText(finding.description)).toBeVisible({ timeout: 5000 });
    await expect(panel.getByText(finding.remediation)).toBeVisible();
  });

  test("security advisor empty-state consistency", async ({ page, request, adminToken }) => {
    const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/advisors/security");
    test.skip(
      probeStatus === 503 || probeStatus === 404 || probeStatus === 501,
      `Security advisor endpoint unavailable (status ${probeStatus})`,
    );

    const report = await fetchSecurityAdvisorReport(request, adminToken);
    test.skip(report.findings.length > 0, "Security advisor has findings; empty-state path unavailable");

    await openAdvisor(page, /^Security Advisor$/i);
    await expect(page.getByRole("heading", { name: /Security Advisor/i })).toBeVisible({ timeout: 5000 });

    const panel = page.getByTestId("security-advisor-panel");
    await expect(panel.getByText("No findings for current filters.")).toBeVisible({ timeout: 5000 });
  });

  test("performance advisor time-range switching and query detail expansion", async ({
    page,
    request,
    adminToken,
  }) => {
    const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/advisors/performance");
    test.skip(
      probeStatus === 503 || probeStatus === 404 || probeStatus === 501,
      `Performance advisor endpoint unavailable (status ${probeStatus})`,
    );

    const report24h = await fetchPerformanceAdvisorReport(request, adminToken, "24h");
    const query = report24h.queries[0];
    test.skip(!query, "Performance advisor returned no queries; expansion path unavailable");

    await openAdvisor(page, /^Performance Advisor$/i);
    await expect(page.getByRole("heading", { name: /Performance Advisor/i })).toBeVisible({ timeout: 5000 });

    const panel = page.getByTestId("performance-advisor-panel");
    const timeRange = panel.getByLabel("Time range");
    await expect(panel).toBeVisible({ timeout: 5000 });
    await expect(timeRange).toBeVisible();

    await timeRange.selectOption("24h");
    await expect(timeRange).toHaveValue("24h");
    await expect.poll(() => page.url()).toContain("perfRange=24h");

    const tableOrEmpty = panel
      .getByRole("columnheader", { name: /Fingerprint/i })
      .or(panel.getByText("No slow queries"));
    await expect(tableOrEmpty).toBeVisible({ timeout: 5000 });

    // Verify table row count matches the fixture report (capped to PAGE_SIZE=20).
    // getByRole("row") matches header + data rows, so add 1 for the <thead> row.
    const expectedRowCount = Math.min(report24h.queries.length, 20);
    await expect(panel.getByRole("row")).toHaveCount(expectedRowCount + 1, { timeout: 5000 });

    const queryButton = panel.getByRole("button", { name: new RegExp(escapeRegExp(query.fingerprint), "i") }).first();
    await expect(queryButton).toBeVisible({ timeout: 5000 });
    await queryButton.click();
    await expect(
      panel.getByRole("heading", { name: new RegExp(`Query Detail: ${escapeRegExp(query.fingerprint)}`) }),
    ).toBeVisible({ timeout: 5000 });
    await expect(panel.getByText(query.normalizedQuery)).toBeVisible();
  });

  test("performance advisor empty-state consistency", async ({ page, request, adminToken }) => {
    const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/advisors/performance");
    test.skip(
      probeStatus === 503 || probeStatus === 404 || probeStatus === 501,
      `Performance advisor endpoint unavailable (status ${probeStatus})`,
    );

    const report24h = await fetchPerformanceAdvisorReport(request, adminToken, "24h");
    test.skip(report24h.queries.length > 0, "Performance advisor has query rows; empty-state path unavailable");

    await openAdvisor(page, /^Performance Advisor$/i);
    await expect(page.getByRole("heading", { name: /Performance Advisor/i })).toBeVisible({ timeout: 5000 });

    const panel = page.getByTestId("performance-advisor-panel");
    const timeRange = panel.getByLabel("Time range");
    await timeRange.selectOption("24h");
    await expect(timeRange).toHaveValue("24h");
    await expect(panel.getByText("No slow queries")).toBeVisible({ timeout: 5000 });
  });
});
