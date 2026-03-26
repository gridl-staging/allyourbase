import { test, expect, seedLogDrain, cleanupLogDrain, probeEndpoint, waitForDashboard } from "../fixtures";

/**
 * SMOKE TEST: Log Drains
 *
 * Critical Path: Seed a log drain → Navigate to Log Drains → Verify the seeded
 * drain row renders with name and stats columns (Sent, Failed, Dropped).
 */

test.describe("Smoke: Log Drains", () => {
  const drainIDs: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    while (drainIDs.length > 0) {
      const id = drainIDs.pop();
      if (!id) continue;
      await cleanupLogDrain(request, adminToken, id).catch(() => {});
    }
  });

  test("seeded drain renders with stats columns in the table", async ({ page, request, adminToken }) => {
    const status = await probeEndpoint(request, adminToken, "/api/admin/logging/drains");
    test.skip(
      status === 501 || status === 404,
      `Log drains service not configured (status ${status})`,
    );

    const runId = Date.now();
    const drainName = `smoke-drain-${runId}`;
    const seeded = await seedLogDrain(request, adminToken, { name: drainName });
    drainIDs.push(seeded.id);

    await page.goto("/admin/");
    await waitForDashboard(page);

    await page.locator("aside").getByRole("button", { name: /Log Drains/i }).click();
    await expect(page.getByRole("heading", { name: /Log Drains/i })).toBeVisible({ timeout: 15_000 });

    // Verify table column headers including stats columns
    await expect(page.getByRole("columnheader", { name: /Name/i })).toBeVisible({ timeout: 5000 });
    await expect(page.getByRole("columnheader", { name: /Sent/i })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: /Failed/i })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: /Dropped/i })).toBeVisible();

    // Verify seeded drain row renders with name visible
    const drainRow = page.locator("tr").filter({ hasText: drainName }).first();
    await expect(drainRow).toBeVisible({ timeout: 5000 });
    await expect(drainRow.getByRole("cell")).toHaveText([drainName, "0", "0", "0", "Delete"]);

    // Verify create button
    await expect(page.getByRole("button", { name: /Create Drain/i })).toBeVisible();
  });
});
