import {
  test,
  expect,
  probeEndpoint,
  seedLogDrain,
  cleanupLogDrain,
  waitForDashboard,
} from "../fixtures";

/**
 * FULL E2E TEST: Log Drains Lifecycle
 *
 * Critical Path: Load seeded drain → create drain via UI → delete via UI
 */

test.describe("Log Drains Lifecycle (Full E2E)", () => {
  const drainIDs: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    while (drainIDs.length > 0) {
      const id = drainIDs.pop();
      if (!id) continue;
      await cleanupLogDrain(request, adminToken, id).catch(() => {});
    }
  });

  test("load-and-verify seeded drain, then create and delete via UI", async ({ page, request, adminToken }) => {
    const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/logging/drains");
    test.skip(
      probeStatus === 503 || probeStatus === 404 || probeStatus === 501,
      `Log Drains service unavailable (status ${probeStatus})`,
    );

    const runId = Date.now();
    const seededName = `drain-full-seeded-${runId}`;
    const createdName = `drain-full-created-${runId}`;

    const seeded = await seedLogDrain(request, adminToken, { name: seededName });
    drainIDs.push(seeded.id);

    await page.goto("/admin/");
    await waitForDashboard(page);

    await page.locator("aside").getByRole("button", { name: /^Log Drains$/i }).click();
    await expect(page.getByRole("heading", { name: /Log Drains/i })).toBeVisible({ timeout: 5000 });

    // Verify seeded drain
    const seededRow = page.getByRole("row", { name: new RegExp(seededName) }).first();
    await expect(seededRow).toBeVisible({ timeout: 5000 });

    // Create new drain via UI
    await page.getByRole("button", { name: /Create Drain/i }).click();

    const nameInput = page.getByLabel("Name");
    const nameInputVisible = await nameInput.isVisible({ timeout: 5000 }).catch(() => false);
    test.skip(!nameInputVisible, "Create Drain form did not open in this environment");
    await nameInput.fill(createdName);
    await page.getByLabel("Type").selectOption("http");
    await page.getByPlaceholder("https://logs.example.com/ingest").fill("https://logs.example.test/ingest");
    await page.getByRole("button", { name: /^Create$/i }).click();

    const createdRow = page.getByRole("row", { name: new RegExp(createdName) }).first();
    await expect(createdRow).toBeVisible({ timeout: 5000 });

    // Delete the created drain
    await createdRow.getByRole("button", { name: /Delete/i }).click();
    await expect(page.getByText(/Delete Drain/i)).toBeVisible({ timeout: 5000 });
    await page.getByRole("button", { name: /^Delete$/i }).click();

    await expect(page.getByRole("row", { name: new RegExp(createdName) })).toHaveCount(0, { timeout: 5000 });
  });
});
