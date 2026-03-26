import {
  test,
  expect,
  probeEndpoint,
  seedSchedule,
  cleanupScheduleByID,
  waitForDashboard,
} from "../fixtures";

/**
 * FULL E2E TEST: Schedules Lifecycle
 *
 * Critical Path: Load seeded schedule → create schedule via UI → edit via UI → delete via UI
 */

test.describe("Schedules Lifecycle (Full E2E)", () => {
  const scheduleIDs: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    for (const scheduleID of scheduleIDs.splice(0).reverse()) {
      await cleanupScheduleByID(request, adminToken, scheduleID).catch(() => {});
    }
  });

  test("load-and-verify seeded schedule, then create, edit, and delete via UI", async ({ page, request, adminToken }) => {
    const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/schedules");
    test.skip(
      probeStatus === 503 || probeStatus === 404 || probeStatus === 501,
      `Schedules service unavailable (status ${probeStatus})`,
    );

    const runId = Date.now();
    const seededName = `sched-full-seeded-${runId}`;
    const seededJobType = `sched_full_job_${runId}`;
    const createdName = `sched-full-created-${runId}`;
    const createdJobType = `sched_full_new_${runId}`;

    const seeded = await seedSchedule(request, adminToken, {
      name: seededName,
      jobType: seededJobType,
      cronExpr: "0 * * * *",
      timezone: "UTC",
      payload: { source: "lifecycle-test" },
      enabled: true,
    });
    scheduleIDs.push(seeded.id);

    await page.goto("/admin/");
    await waitForDashboard(page);

    await page.locator("aside").getByRole("button", { name: /^Schedules$/i }).click();
    await expect(page.getByRole("heading", { name: /Job Schedules/i })).toBeVisible({ timeout: 5000 });

    // Verify seeded schedule
    const seededRow = page.getByRole("row", { name: new RegExp(seededName) }).first();
    await expect(seededRow).toBeVisible({ timeout: 5000 });
    await expect(seededRow).toContainText(seededJobType);
    await expect(seededRow).toContainText("0 * * * *");
    const seededToggleButton = seededRow.getByRole("button", { name: /Disable schedule/i });
    await expect(seededToggleButton).toBeVisible({ timeout: 5000 });
    await expect(seededToggleButton).toContainText("On");

    await seededToggleButton.click();
    const seededEnableButton = seededRow.getByRole("button", { name: /Enable schedule/i });
    await expect(seededEnableButton).toBeEnabled({ timeout: 5000 });
    await expect(seededEnableButton).toContainText("Off");

    await seededEnableButton.click();
    const seededDisableButton = seededRow.getByRole("button", { name: /Disable schedule/i });
    await expect(seededDisableButton).toBeEnabled({ timeout: 5000 });
    await expect(seededDisableButton).toContainText("On");

    // Create a new schedule via UI
    await page.getByRole("button", { name: /Create Schedule/i }).click();
    await expect(page.getByRole("heading", { name: /Create Schedule/i })).toBeVisible({ timeout: 5000 });

    await page.getByLabel("Name").fill(createdName);
    await page.getByLabel("Job Type").fill(createdJobType);
    await page.getByLabel("Cron Expression").fill("*/30 * * * *");
    await page.getByLabel("Timezone").clear();
    await page.getByLabel("Timezone").fill("UTC");
    await page.getByLabel("Payload JSON").clear();
    await page.getByLabel("Payload JSON").fill('{"source":"ui-create"}');
    await page.getByRole("button", { name: /^Save$/i }).click();

    await expect(page.getByText(/Schedule created/i)).toBeVisible({ timeout: 5000 });

    const createdRow = page.getByRole("row", { name: new RegExp(createdName) }).first();
    await expect(createdRow).toBeVisible({ timeout: 5000 });
    await expect(createdRow).toContainText("*/30 * * * *");

    // Edit the created schedule via UI (change cron)
    await createdRow.getByRole("button", { name: /Edit schedule/i }).click();
    await expect(page.getByRole("heading", { name: /Edit Schedule/i })).toBeVisible({ timeout: 5000 });

    const cronInput = page.getByLabel("Cron Expression");
    await cronInput.clear();
    await cronInput.fill("0 6 * * *");
    await page.getByRole("button", { name: /^Save$/i }).click();

    await expect(page.getByText(/Schedule updated/i)).toBeVisible({ timeout: 5000 });
    await expect(createdRow).toContainText("0 6 * * *");

    // Delete the created schedule via UI
    await createdRow.getByRole("button", { name: /Delete schedule/i }).click();
    await expect(page.getByText(/Delete schedule/i)).toBeVisible({ timeout: 5000 });
    await page.getByRole("button", { name: /^Delete$/i }).click();

    await expect(page.getByText(/Schedule deleted/i)).toBeVisible({ timeout: 5000 });
    await expect(page.getByRole("row", { name: new RegExp(createdName) })).toHaveCount(0, { timeout: 5000 });
  });
});
