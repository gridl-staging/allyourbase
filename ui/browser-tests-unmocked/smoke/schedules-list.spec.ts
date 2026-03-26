import {
  test,
  expect,
  probeEndpoint,
  seedSchedule,
  cleanupScheduleByID,
  waitForDashboard,
} from "../fixtures";

/**
 * SMOKE TEST: Schedules - List View
 *
 * Critical Path: Navigate to Schedules → verify seeded schedule row and actions render
 */

test.describe("Smoke: Schedules List", () => {
  const scheduleIDs: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    while (scheduleIDs.length > 0) {
      const scheduleID = scheduleIDs.pop();
      if (!scheduleID) continue;
      await cleanupScheduleByID(request, adminToken, scheduleID).catch(() => {});
    }
  });

  test("seeded schedule appears with cron and management actions", async ({ page, request, adminToken }) => {
    const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/schedules");
    test.skip(
      probeStatus === 503 || probeStatus === 404 || probeStatus === 501,
      `Schedules service unavailable (status ${probeStatus})`,
    );

    const runId = Date.now();
    const scheduleName = `smoke-schedule-${runId}`;
    const jobType = `smoke_job_type_${runId}`;
    const cronExpr = "*/15 * * * *";

    const seededSchedule = await seedSchedule(request, adminToken, {
      name: scheduleName,
      jobType,
      cronExpr,
      timezone: "UTC",
      payload: { runId, source: "schedules-smoke" },
      enabled: true,
    });
    scheduleIDs.push(seededSchedule.id);

    await page.goto("/admin/");
    await waitForDashboard(page);

    await page.locator("aside").getByRole("button", { name: /^Schedules$/i }).click();
    await expect(page.getByRole("heading", { name: /Job Schedules/i })).toBeVisible({ timeout: 15_000 });

    await expect(page.getByRole("columnheader", { name: "Name" })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Job Type" })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Cron" })).toBeVisible();

    const row = page.getByRole("row", { name: new RegExp(scheduleName) }).first();
    await expect(row).toBeVisible({ timeout: 5000 });
    await expect(row).toContainText(jobType);
    await expect(row).toContainText(cronExpr);
    await expect(row).toContainText("UTC");
    await expect(row.getByRole("button", { name: /Disable schedule/i })).toBeVisible();
    await expect(row.getByRole("button", { name: /Edit schedule/i })).toBeVisible();
    await expect(row.getByRole("button", { name: /Delete schedule/i })).toBeVisible();
  });
});
