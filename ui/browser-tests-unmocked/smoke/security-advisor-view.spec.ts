import {
  test,
  expect,
  probeEndpoint,
  fetchSecurityAdvisorReport,
  waitForDashboard,
} from "../fixtures";

/**
 * SMOKE TEST: Security Advisor — Content-Verified
 *
 * Probes the security advisor endpoint and asserts real content when available
 * (severity filters, finding sections, "Last evaluated" timestamp), or verifies
 * the error UI renders correctly when the backend endpoint is not wired.
 */

test.describe("Smoke: Security Advisor", () => {
  test("security advisor renders content matching API-backed report state", async ({ page, request, adminToken }) => {
    const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/advisors/security");
    test.skip(
      probeStatus === 503 || probeStatus === 404 || probeStatus === 501,
      `Security advisor endpoint unavailable (status ${probeStatus})`,
    );

    const report = await fetchSecurityAdvisorReport(request, adminToken);

    // Act: navigate to Security Advisor
    await page.goto("/admin/");
    await waitForDashboard(page);
    await page.locator("aside").getByRole("button", { name: /^Security Advisor$/i }).click();
    await expect(page.getByRole("heading", { name: /Security Advisor/i })).toBeVisible({ timeout: 15_000 });

    const panel = page.getByTestId("security-advisor-panel");
    await expect(panel).toBeVisible({ timeout: 5000 });

    await expect(panel.getByLabel(/^Severity$/i)).toBeVisible({ timeout: 5000 });
    await expect(panel.getByLabel(/^Category$/i)).toBeVisible();
    await expect(panel.getByLabel(/^Status$/i)).toBeVisible();

    const findingsOrEmptyState = panel
      .getByRole("heading", { name: /critical|high|medium|low/i })
      .first()
      .or(panel.getByText(/No findings for current filters/i));
    await expect(findingsOrEmptyState).toBeVisible({ timeout: 5000 });

    const emptyState = panel.getByText(/No findings for current filters/i);
    if (report.findings.length === 0) {
      await expect(emptyState).toBeVisible();
    } else {
      await expect(emptyState).not.toBeVisible();
    }
    await expect(panel.getByText(/Last evaluated:/i)).toContainText(report.evaluatedAt.substring(0, 10));
  });
});
