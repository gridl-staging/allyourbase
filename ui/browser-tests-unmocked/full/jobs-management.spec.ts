import {
  test,
  expect,
  probeEndpoint,
  seedJob,
  cleanupJobsByType,
  waitForDashboard,
} from "../fixtures";

/**
 * FULL E2E TEST: Jobs Management
 *
 * Critical Path: Load seeded jobs → filter by state → retry failed job → cancel queued job
 */

test.describe("Jobs Management (Full E2E)", () => {
  const seededJobTypes: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    for (const jobType of seededJobTypes.splice(0).reverse()) {
      await cleanupJobsByType(request, adminToken, jobType).catch(() => {});
    }
  });

  test("load-and-verify seeded jobs, then retry failed and cancel queued via UI", async ({ page, request, adminToken }) => {
    const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/jobs");
    test.skip(
      probeStatus === 503 || probeStatus === 404 || probeStatus === 501,
      `Job queue service unavailable (status ${probeStatus})`,
    );

    const runId = Date.now();
    const failedJobType = `jobs_full_failed_${runId}`;
    const queuedJobType = `jobs_full_queued_${runId}`;
    const failedError = `lifecycle-failure-${runId}`;

    await seedJob(request, adminToken, {
      type: failedJobType,
      state: "failed",
      attempts: 2,
      maxAttempts: 5,
      lastError: failedError,
      payload: { source: "jobs-lifecycle" },
    });
    seededJobTypes.push(failedJobType);

    await seedJob(request, adminToken, {
      type: queuedJobType,
      state: "queued",
      attempts: 0,
      maxAttempts: 3,
      payload: { source: "jobs-lifecycle" },
    });
    seededJobTypes.push(queuedJobType);

    await page.goto("/admin/");
    await waitForDashboard(page);

    await page.locator("aside").getByRole("button", { name: /^Jobs$/i }).click();
    await expect(page.getByRole("heading", { name: /Job Queue/i })).toBeVisible({ timeout: 5000 });
    await expect(page.getByText(/Queued:\s*[1-9]/i)).toBeVisible({ timeout: 5000 });
    await expect(page.getByText(/Failed:\s*[1-9]/i)).toBeVisible({ timeout: 5000 });

    const oldestQueuedAgeCard = page.getByText(/Oldest queued age:\s*/i);
    await expect(oldestQueuedAgeCard).toBeVisible({ timeout: 5000 });

    // Verify failed job row
    const failedRow = page.getByRole("row", { name: new RegExp(failedJobType) }).first();
    await expect(failedRow).toBeVisible({ timeout: 5000 });
    await expect(failedRow).toContainText("failed");
    await expect(failedRow).toContainText(failedError);
    await expect(failedRow.getByRole("button", { name: /Retry/i })).toBeVisible();

    // Retry the failed job
    await failedRow.getByRole("button", { name: /Retry/i }).click();
    await expect(page.getByText(/Retried job/i)).toBeVisible({ timeout: 5000 });
    await expect(failedRow).toContainText("queued");
    await expect(oldestQueuedAgeCard).toBeVisible({ timeout: 5000 });

    // Verify queued job row
    const queuedRow = page.getByRole("row", { name: new RegExp(queuedJobType) }).first();
    await expect(queuedRow).toBeVisible({ timeout: 5000 });
    await expect(queuedRow).toContainText("queued");
    await expect(queuedRow.getByRole("button", { name: /Cancel/i })).toBeVisible();

    // Cancel the queued job
    await queuedRow.getByRole("button", { name: /Cancel/i }).click();
    await expect(page.getByText(/Canceled job/i)).toBeVisible({ timeout: 5000 });
    await expect(queuedRow).toContainText("canceled");
    await expect(oldestQueuedAgeCard).toBeVisible({ timeout: 5000 });
  });
});
