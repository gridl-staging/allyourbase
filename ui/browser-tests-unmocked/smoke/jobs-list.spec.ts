import {
  test,
  expect,
  probeEndpoint,
  seedJob,
  cleanupJobsByType,
  waitForDashboard,
} from "../fixtures";

/**
 * SMOKE TEST: Jobs - List View
 *
 * Critical Path: Navigate to Jobs → verify seeded job row renders with retry action
 */

test.describe("Smoke: Jobs List", () => {
  const seededJobTypes: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    while (seededJobTypes.length > 0) {
      const jobType = seededJobTypes.pop();
      if (!jobType) continue;
      await cleanupJobsByType(request, adminToken, jobType).catch(() => {});
    }
  });

  test("seeded failed job appears with retry action", async ({ page, request, adminToken }) => {
    const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/jobs");
    test.skip(
      probeStatus === 503 || probeStatus === 404 || probeStatus === 501,
      `Job queue service unavailable (status ${probeStatus})`,
    );

    const runId = Date.now();
    const jobType = `smoke_job_${runId}`;
    const lastError = `smoke-failure-${runId}`;

    await seedJob(request, adminToken, {
      type: jobType,
      state: "failed",
      attempts: 1,
      maxAttempts: 3,
      lastError,
      payload: { runId, source: "jobs-smoke" },
    });
    seededJobTypes.push(jobType);

    await page.goto("/admin/");
    await waitForDashboard(page);

    await page.locator("aside").getByRole("button", { name: /^Jobs$/i }).click();
    await expect(page.getByRole("heading", { name: /Job Queue/i })).toBeVisible({ timeout: 15_000 });

    await expect(page.getByRole("columnheader", { name: "State" })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Type" })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Last Error" })).toBeVisible();

    const row = page.getByRole("row", { name: new RegExp(jobType) }).first();
    await expect(row).toBeVisible({ timeout: 5000 });
    await expect(row).toContainText("failed");
    await expect(row).toContainText(lastError);
    await expect(row.getByRole("button", { name: /Retry job/i })).toBeVisible();
  });
});
